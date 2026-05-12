import 'dart:async';

import 'package:flutter/foundation.dart';

import '../models/protocol_models.dart';
import '../services/api_client.dart';
import '../services/settings_store.dart';
import '../services/ws_client.dart';
import 'connection_provider.dart';
import 'project_provider.dart';

class ConversationProvider extends ChangeNotifier {
  ConversationProvider({required this.apiClient, required this.wsClient, required this.settingsStore});

  final OnePilotApiClient apiClient;
  final OnePilotWsClient wsClient;
  final ConnectionSettingsStore settingsStore;

  ConnectionProvider? _connection;
  ProjectProvider? _projects;
  StreamSubscription<RealtimeEvent>? _subscription;

  List<Conversation> conversations = const <Conversation>[];
  Conversation? activeConversation;
  List<Message> messages = const <Message>[];
  List<String> statusMessages = const <String>[];
  bool loading = false;
  bool running = false;
  String? errorMessage;

  void attach(ConnectionProvider connection, ProjectProvider projects) {
    _connection = connection;
    _projects = projects;
  }

  Future<void> resetForProjectSwitch() async {
    final conversationId = activeConversation?.id;
    if (conversationId != null) {
      wsClient.unsubscribe(conversationId);
    }
    await _subscription?.cancel();
    _subscription = null;
    await wsClient.close();
    conversations = const <Conversation>[];
    activeConversation = null;
    messages = const <Message>[];
    statusMessages = const <String>[];
    running = false;
    loading = false;
    errorMessage = null;
    notifyListeners();
  }

  Future<void> loadConversations() async {
    final connection = _connection;
    final project = _projects?.selectedProject;
    if (connection == null || project == null) {
      return;
    }
    loading = true;
    errorMessage = null;
    notifyListeners();
    try {
      conversations = await apiClient.getConversations(connection.settings, project.id);
    } catch (error) {
      errorMessage = error.toString();
    }
    loading = false;
    notifyListeners();
  }

  Future<void> createConversation(String title) async {
    final connection = _connection;
    final project = _projects?.selectedProject;
    if (connection == null || project == null || title.trim().isEmpty) {
      return;
    }
    final created = await apiClient.createConversation(connection.settings, project.id, title.trim());
    conversations = <Conversation>[created, ...conversations];
    await openConversation(created);
  }

  Future<void> openConversation(Conversation conversation) async {
    final connection = _connection;
    final project = _projects?.selectedProject;
    if (connection == null || project == null) {
      return;
    }
    final previousConversationId = activeConversation?.id;
    if (previousConversationId != null && previousConversationId != conversation.id) {
      wsClient.unsubscribe(previousConversationId);
    }
    await _subscription?.cancel();
    _subscription = null;
    await wsClient.close();
    activeConversation = conversation;
    loading = true;
    errorMessage = null;
    notifyListeners();
    try {
      final detail = await apiClient.getConversation(connection.settings, project.id, conversation.id);
      activeConversation = detail;
      messages = detail.messages;
      await connection.remember(conversationId: detail.id);
      _subscription = wsClient.connect(connection.settings, project.id).listen(_handleRealtimeEvent, onError: (error) {
        errorMessage = error.toString();
        _recordStatus('WebSocket 错误: $error');
        notifyListeners();
      });
      wsClient.subscribe(detail.id);
      _recordStatus('已订阅对话 ${detail.id}');
    } catch (error) {
      errorMessage = error.toString();
    }
    loading = false;
    notifyListeners();
  }

  void leaveConversation() {
    final conversationId = activeConversation?.id;
    if (conversationId != null) {
      wsClient.unsubscribe(conversationId);
    }
    _subscription?.cancel();
    _subscription = null;
    unawaited(wsClient.close());
    running = false;
  }

  Future<void> sendMessage(String message) async {
    final connection = _connection;
    final project = _projects?.selectedProject;
    var conversation = activeConversation;
    if (connection == null || project == null || message.trim().isEmpty) {
      return;
    }
    if (conversation == null) {
      final created = await apiClient.createConversation(connection.settings, project.id, message.trim().substring(0, message.trim().length > 50 ? 50 : message.trim().length));
      conversations = <Conversation>[created, ...conversations];
      await openConversation(created);
      conversation = activeConversation;
    }
    if (conversation == null) {
      return;
    }
    messages = <Message>[...messages, Message(role: MessageRole.user, content: message.trim(), timestamp: DateTime.now())];
    running = true;
    notifyListeners();
    await apiClient.sendTurn(connection.settings, project.id, conversation.id, message.trim());
  }

  Future<void> emergencyStop() async {
    final connection = _connection;
    final project = _projects?.selectedProject;
    final conversation = activeConversation;
    if (connection == null || project == null || conversation == null) {
      return;
    }
    await apiClient.emergencyStop(connection.settings, project.id, conversation.id);
    running = false;
    notifyListeners();
  }

  void _handleRealtimeEvent(RealtimeEvent event) {
    switch (event.type) {
      case 'connected':
        _recordStatus('WebSocket 已连接');
        break;
      case 'delta':
        _appendDelta(event);
        running = true;
        break;
      case 'turn_completed':
        _applyUsage(event.usage);
        _recordStatus(event.usage == null ? 'Turn 已完成' : 'Turn 已完成 · ${event.usage!.inputTokens ?? 0} in / ${event.usage!.outputTokens ?? 0} out');
        running = false;
        break;
      case 'conversation_update':
        _applyConversationStatus(event.status);
        _recordStatus('对话状态: ${event.status}');
        running = event.status == 'running';
        break;
      case 'replay_truncated':
        final conversation = activeConversation;
        if (conversation != null) {
          _recordStatus('历史被截断，正在刷新对话');
          unawaited(_refreshActiveConversation(conversation));
        }
        break;
      case 'instance_status':
        _recordStatus('实例状态: ${event.status}');
        break;
    }
    notifyListeners();
  }

  Future<void> _refreshActiveConversation(Conversation conversation) async {
    final connection = _connection;
    final project = _projects?.selectedProject;
    if (connection == null || project == null) {
      return;
    }
    try {
      final detail = await apiClient.getConversation(connection.settings, project.id, conversation.id);
      activeConversation = detail;
      messages = detail.messages;
      notifyListeners();
    } catch (error) {
      errorMessage = error.toString();
      notifyListeners();
    }
  }

  void _applyUsage(Usage? usage) {
    if (usage == null || messages.isEmpty) {
      return;
    }
    final index = messages.lastIndexWhere((message) => message.role == MessageRole.assistant);
    if (index < 0) {
      return;
    }
    final updated = messages[index].copyWith(usage: usage);
    messages = <Message>[...messages.take(index), updated, ...messages.skip(index + 1)];
  }

  void _applyConversationStatus(String status) {
    final conversation = activeConversation;
    if (conversation == null || status.isEmpty) {
      return;
    }
    activeConversation = conversation.copyWith(status: conversationStatusFromJson(status));
  }

  void _recordStatus(String message) {
    statusMessages = <String>[...statusMessages, message];
    if (statusMessages.length > 5) {
      statusMessages = statusMessages.sublist(statusMessages.length - 5);
    }
  }

  void _appendDelta(RealtimeEvent event) {
    if (event.kind == MessageSegmentKind.toolResult) {
      return;
    }
    final segment = MessageSegment(kind: event.kind, content: event.delta, turnId: event.turnId);
    if (messages.isEmpty || messages.last.role != MessageRole.assistant) {
      messages = <Message>[...messages, Message(role: MessageRole.assistant, content: event.kind == MessageSegmentKind.agentMessage ? event.delta : '', segments: <MessageSegment>[segment])];
      return;
    }
    final last = messages.last;
    final updated = last.copyWith(
      content: event.kind == MessageSegmentKind.agentMessage ? '${last.content}${event.delta}' : last.content,
      segments: <MessageSegment>[...last.segments, segment],
    );
    messages = <Message>[...messages.take(messages.length - 1), updated];
  }

  @override
  void dispose() {
    _subscription?.cancel();
    wsClient.close();
    super.dispose();
  }
}
