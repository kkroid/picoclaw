import 'dart:async';
import 'dart:convert';

import 'package:web_socket_channel/web_socket_channel.dart';

import '../models/protocol_models.dart';

class RealtimeEvent {
  const RealtimeEvent({required this.type, this.conversationId, this.turnId, this.delta = '', this.kind = MessageSegmentKind.agentMessage, this.status = '', this.usage});

  final String type;
  final String? conversationId;
  final String? turnId;
  final String delta;
  final MessageSegmentKind kind;
  final String status;
  final Usage? usage;

  factory RealtimeEvent.fromJson(Map<String, Object?> json) {
    final usageJson = json['usage'];
    return RealtimeEvent(
      type: json['type']?.toString() ?? '',
      conversationId: json['conversation_id']?.toString(),
      turnId: json['turn_id']?.toString(),
      delta: json['delta']?.toString() ?? json['message']?.toString() ?? '',
      kind: segmentKindFromJson(json['kind']),
      status: json['status']?.toString() ?? '',
      usage: usageJson is Map<String, Object?> ? Usage.fromJson(usageJson) : null,
    );
  }
}

abstract class OnePilotWsClient {
  Stream<RealtimeEvent> connect(ConnectionSettings settings, String projectId);
  void subscribe(String conversationId);
  void unsubscribe(String conversationId);
  Future<void> close();
}

class WebSocketOnePilotWsClient implements OnePilotWsClient {
  WebSocketChannel? _channel;
  final _events = StreamController<RealtimeEvent>.broadcast();

  @override
  Stream<RealtimeEvent> connect(ConnectionSettings settings, String projectId) {
    close();
    _channel = WebSocketChannel.connect(settings.wsUri(projectId));
    _channel!.stream.listen((payload) {
      if (payload is String) {
        final decoded = jsonDecode(payload) as Map<String, Object?>;
        _events.add(RealtimeEvent.fromJson(decoded));
      }
    }, onError: _events.addError);
    return _events.stream;
  }

  @override
  void subscribe(String conversationId) {
    _send(<String, Object?>{'type': 'subscribe', 'conversation_id': conversationId});
  }

  @override
  void unsubscribe(String conversationId) {
    _send(<String, Object?>{'type': 'unsubscribe', 'conversation_id': conversationId});
  }

  void _send(Map<String, Object?> payload) {
    _channel?.sink.add(jsonEncode(payload));
  }

  @override
  Future<void> close() async {
    await _channel?.sink.close();
    _channel = null;
  }
}

class FakeOnePilotWsClient implements OnePilotWsClient {
  final controller = StreamController<RealtimeEvent>.broadcast();
  final List<String> subscriptions = <String>[];

  @override
  Stream<RealtimeEvent> connect(ConnectionSettings settings, String projectId) => controller.stream;

  @override
  void subscribe(String conversationId) {
    subscriptions.add(conversationId);
  }

  @override
  void unsubscribe(String conversationId) {
    subscriptions.remove(conversationId);
  }

  @override
  Future<void> close() async {}

  void emit(RealtimeEvent event) {
    controller.add(event);
  }
}
