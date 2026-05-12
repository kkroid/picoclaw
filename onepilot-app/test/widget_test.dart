import 'dart:typed_data';

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';

import 'package:flutter_open_lite/app.dart';
import 'package:flutter_open_lite/models/protocol_models.dart';
import 'package:flutter_open_lite/services/api_client.dart';
import 'package:flutter_open_lite/services/settings_store.dart';
import 'package:flutter_open_lite/services/ws_client.dart';

void main() {
  testWidgets('OnePilot protocol client covers connection, conversation stream, and files', (WidgetTester tester) async {
    final apiClient = _FakeApiClient();
    final wsClient = FakeOnePilotWsClient();
    final settingsStore = MemoryConnectionSettingsStore();

    await tester.pumpWidget(OnePilotApp(apiClient: apiClient, wsClient: wsClient, settingsStore: settingsStore));
    await tester.pumpAndSettle();

    expect(find.text('连接配置'), findsOneWidget);
    await tester.enterText(find.byKey(const Key('host-field')), '127.0.0.1');
    await tester.enterText(find.byKey(const Key('port-field')), '18800');
    await tester.tap(find.byKey(const Key('connect-button')));
    await tester.pumpAndSettle();

    expect(find.text('App A 后端'), findsWidgets);
    expect(find.text('对话'), findsOneWidget);
    expect(find.text('文件'), findsOneWidget);
    expect(find.text('修复登录模块空指针'), findsOneWidget);

    await tester.tap(find.text('修复登录模块空指针'));
    await tester.pumpAndSettle();
    expect(find.textContaining('历史消息'), findsOneWidget);

    await tester.enterText(find.byKey(const Key('message-field')), '继续');
    await tester.tap(find.byKey(const Key('send-message-button')));
    await tester.pump();
    await tester.tap(find.byKey(const Key('emergency-stop-button')));
    await tester.pumpAndSettle();
    expect(find.text('确认急停'), findsOneWidget);
    await tester.tap(find.text('急停'));
    await tester.pumpAndSettle();
    expect(apiClient.emergencyStopCount, 1);
    wsClient.emit(const RealtimeEvent(type: 'delta', conversationId: 'thr_abc123', turnId: 'turn_1', delta: '流式回复', kind: MessageSegmentKind.agentMessage));
    wsClient.emit(const RealtimeEvent(type: 'delta', conversationId: 'thr_abc123', turnId: 'turn_1', delta: '需要分析', kind: MessageSegmentKind.reasoning));
    wsClient.emit(const RealtimeEvent(type: 'delta', conversationId: 'thr_abc123', turnId: 'turn_1', delta: 'read_file(auth.go)', kind: MessageSegmentKind.toolCall));
    wsClient.emit(const RealtimeEvent(type: 'turn_completed', conversationId: 'thr_abc123', turnId: 'turn_1', usage: Usage(inputTokens: 4, outputTokens: 8)));
    await tester.pumpAndSettle();

    expect(find.text('继续'), findsOneWidget);
    expect(find.textContaining('流式回复'), findsOneWidget);
    expect(find.text('Thinking'), findsOneWidget);
    expect(find.textContaining('正在执行: read_file(auth.go)'), findsOneWidget);
    expect(find.textContaining('4 in / 8 out'), findsWidgets);

    await tester.pageBack();
    await tester.pumpAndSettle();
    expect(wsClient.subscriptions, isEmpty);
    await tester.tap(find.text('文件'));
    await tester.pumpAndSettle();
    expect(find.text('main.go'), findsOneWidget);
    await tester.tap(find.text('main.go'));
    await tester.pumpAndSettle();
    expect(find.byKey(const Key('file-content-field')), findsOneWidget);
    await tester.enterText(find.byKey(const Key('file-content-field')), 'package main\nfunc main() {}');
    await tester.tap(find.byKey(const Key('save-file-button')));
    await tester.pumpAndSettle();
    expect(apiClient.writtenFiles['main.go'], contains('func main'));
  });
}

class _FakeApiClient implements OnePilotApiClient {
  int emergencyStopCount = 0;
  final Map<String, String> writtenFiles = <String, String>{};

  @override
  Future<SystemInfo> getSystemInfo(ConnectionSettings settings) async => const SystemInfo(version: '1.0.0', projectCount: 1);

  @override
  Future<List<Project>> getProjects(ConnectionSettings settings) async => const [Project(id: 'app-a', name: 'App A 后端', status: ProjectStatus.running, workspace: '/workspace/app-a')];

  @override
  Future<Project> getProject(ConnectionSettings settings, String projectId) async => const Project(id: 'app-a', name: 'App A 后端', status: ProjectStatus.running);

  @override
  Future<List<Conversation>> getConversations(ConnectionSettings settings, String projectId) async => const [Conversation(id: 'thr_abc123', title: '修复登录模块空指针', status: ConversationStatus.running, lastMessagePreview: '历史消息', source: ConversationSource.thread)];

  @override
  Future<Conversation> createConversation(ConnectionSettings settings, String projectId, String title) async => Conversation(id: 'thr_new', title: title, status: ConversationStatus.active);

  @override
  Future<Conversation> getConversation(ConnectionSettings settings, String projectId, String conversationId) async {
    return const Conversation(
      id: 'thr_abc123',
      title: '修复登录模块空指针',
      status: ConversationStatus.running,
      messages: [Message(role: MessageRole.assistant, content: '历史消息')],
    );
  }

  @override
  Future<TurnResponse> sendTurn(ConnectionSettings settings, String projectId, String conversationId, String message) async => const TurnResponse(turnId: 'turn_1', status: 'queued');

  @override
  Future<void> emergencyStop(ConnectionSettings settings, String projectId, String conversationId) async {
    emergencyStopCount += 1;
  }

  @override
  Future<FileNode> getFileTree(ConnectionSettings settings, String projectId, String path) async {
    return const FileNode(path: '', name: 'root', type: FileNodeType.directory, children: [FileNode(path: 'main.go', name: 'main.go', type: FileNodeType.file, size: 42)]);
  }

  @override
  Future<FileContent> readFile(ConnectionSettings settings, String projectId, String path) async => FileContent(contentType: 'text/plain', bytes: Uint8List.fromList('package main'.codeUnits));

  @override
  Future<void> writeFile(ConnectionSettings settings, String projectId, String path, String content) async {
    writtenFiles[path] = content;
  }
}
