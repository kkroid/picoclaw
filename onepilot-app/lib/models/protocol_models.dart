enum ProjectStatus { running, stopped, error }

ProjectStatus projectStatusFromJson(Object? value) {
  switch ('$value') {
    case 'running':
      return ProjectStatus.running;
    case 'error':
    case 'unhealthy':
      return ProjectStatus.error;
    default:
      return ProjectStatus.stopped;
  }
}

String projectStatusLabel(ProjectStatus status) {
  switch (status) {
    case ProjectStatus.running:
      return '运行中';
    case ProjectStatus.error:
      return '异常';
    case ProjectStatus.stopped:
      return '已停止';
  }
}

class SystemInfo {
  const SystemInfo({required this.version, required this.projectCount});

  final String version;
  final int projectCount;

  factory SystemInfo.fromJson(Map<String, Object?> json) {
    return SystemInfo(
      version: json['version']?.toString() ?? 'unknown',
      projectCount: _asInt(json['project_count']),
    );
  }
}

class Project {
  const Project({
    required this.id,
    required this.name,
    this.workspace = '',
    this.os = '',
    this.status = ProjectStatus.stopped,
    this.port,
    this.pid,
    this.lastActive,
  });

  final String id;
  final String name;
  final String workspace;
  final String os;
  final ProjectStatus status;
  final int? port;
  final int? pid;
  final DateTime? lastActive;

  factory Project.fromJson(Map<String, Object?> json) {
    return Project(
      id: json['id']?.toString() ?? '',
      name: json['name']?.toString() ?? '',
      workspace: json['workspace']?.toString() ?? '',
      os: json['os']?.toString() ?? '',
      status: projectStatusFromJson(json['status']),
      port: _asNullableInt(json['port']),
      pid: _asNullableInt(json['pid']),
      lastActive: _asDate(json['last_active']),
    );
  }
}

enum ConversationStatus { active, running, completed, failed, interrupted }
enum ConversationSource { thread, task }

ConversationStatus conversationStatusFromJson(Object? value) {
  switch ('$value') {
    case 'running':
      return ConversationStatus.running;
    case 'completed':
      return ConversationStatus.completed;
    case 'failed':
      return ConversationStatus.failed;
    case 'interrupted':
      return ConversationStatus.interrupted;
    default:
      return ConversationStatus.active;
  }
}

String conversationStatusLabel(ConversationStatus status) {
  switch (status) {
    case ConversationStatus.running:
      return '进行中';
    case ConversationStatus.completed:
      return '已完成';
    case ConversationStatus.failed:
      return '失败';
    case ConversationStatus.interrupted:
      return '中断';
    case ConversationStatus.active:
      return '待处理';
  }
}

ConversationSource conversationSourceFromJson(Object? value) {
  return '$value' == 'task' ? ConversationSource.task : ConversationSource.thread;
}

class Conversation {
  const Conversation({
    required this.id,
    required this.title,
    this.model = '',
    this.mode = '',
    this.source = ConversationSource.thread,
    this.status = ConversationStatus.active,
    this.createdAt,
    this.updatedAt,
    this.lastMessagePreview = '',
    this.messages = const <Message>[],
  });

  final String id;
  final String title;
  final String model;
  final String mode;
  final ConversationSource source;
  final ConversationStatus status;
  final DateTime? createdAt;
  final DateTime? updatedAt;
  final String lastMessagePreview;
  final List<Message> messages;

  factory Conversation.fromJson(Map<String, Object?> json) {
    final messagesJson = json['messages'];
    return Conversation(
      id: json['id']?.toString() ?? '',
      title: json['title']?.toString() ?? '',
      model: json['model']?.toString() ?? '',
      mode: json['mode']?.toString() ?? '',
      source: conversationSourceFromJson(json['source']),
      status: conversationStatusFromJson(json['status']),
      createdAt: _asDate(json['created_at']),
      updatedAt: _asDate(json['updated_at']),
      lastMessagePreview: json['last_message_preview']?.toString() ?? '',
      messages: messagesJson is List
          ? messagesJson.whereType<Map<String, Object?>>().map(Message.fromJson).toList()
          : const <Message>[],
    );
  }

  Conversation copyWith({ConversationStatus? status, List<Message>? messages}) {
    return Conversation(
      id: id,
      title: title,
      model: model,
      mode: mode,
      source: source,
      status: status ?? this.status,
      createdAt: createdAt,
      updatedAt: updatedAt,
      lastMessagePreview: lastMessagePreview,
      messages: messages ?? this.messages,
    );
  }
}

enum MessageRole { user, assistant, system }
enum MessageSegmentKind { agentMessage, reasoning, toolCall, toolResult }

MessageRole messageRoleFromJson(Object? value) {
  switch ('$value') {
    case 'assistant':
      return MessageRole.assistant;
    case 'system':
      return MessageRole.system;
    default:
      return MessageRole.user;
  }
}

MessageSegmentKind segmentKindFromJson(Object? value) {
  switch ('$value') {
    case 'reasoning':
      return MessageSegmentKind.reasoning;
    case 'tool_call':
      return MessageSegmentKind.toolCall;
    case 'tool_result':
      return MessageSegmentKind.toolResult;
    default:
      return MessageSegmentKind.agentMessage;
  }
}

class Usage {
  const Usage({this.inputTokens, this.outputTokens, this.cachedTokens, this.costUsd});

  final int? inputTokens;
  final int? outputTokens;
  final int? cachedTokens;
  final double? costUsd;

  factory Usage.fromJson(Map<String, Object?> json) {
    return Usage(
      inputTokens: _asNullableInt(json['input_tokens']),
      outputTokens: _asNullableInt(json['output_tokens']),
      cachedTokens: _asNullableInt(json['cached_tokens']),
      costUsd: _asNullableDouble(json['cost_usd']),
    );
  }
}

class MessageSegment {
  const MessageSegment({required this.kind, required this.content, this.turnId});

  final MessageSegmentKind kind;
  final String content;
  final String? turnId;
}

class Message {
  const Message({
    required this.role,
    required this.content,
    this.timestamp,
    this.usage,
    this.segments = const <MessageSegment>[],
  });

  final MessageRole role;
  final String content;
  final DateTime? timestamp;
  final Usage? usage;
  final List<MessageSegment> segments;

  factory Message.fromJson(Map<String, Object?> json) {
    final usageJson = json['usage'];
    return Message(
      role: messageRoleFromJson(json['role']),
      content: json['content']?.toString() ?? '',
      timestamp: _asDate(json['timestamp']),
      usage: usageJson is Map<String, Object?> ? Usage.fromJson(usageJson) : null,
    );
  }

  Message copyWith({String? content, Usage? usage, List<MessageSegment>? segments}) {
    return Message(
      role: role,
      content: content ?? this.content,
      timestamp: timestamp,
      usage: usage ?? this.usage,
      segments: segments ?? this.segments,
    );
  }
}

enum FileNodeType { file, directory, image }

FileNodeType fileNodeTypeFromJson(Object? value, String name) {
  if ('$value' == 'directory') {
    return FileNodeType.directory;
  }
  final lower = name.toLowerCase();
  if (lower.endsWith('.png') || lower.endsWith('.jpg') || lower.endsWith('.jpeg') || lower.endsWith('.webp')) {
    return FileNodeType.image;
  }
  return FileNodeType.file;
}

class FileNode {
  const FileNode({
    required this.path,
    required this.name,
    required this.type,
    this.size = 0,
    this.modified,
    this.children = const <FileNode>[],
  });

  final String path;
  final String name;
  final FileNodeType type;
  final int size;
  final DateTime? modified;
  final List<FileNode> children;

  factory FileNode.fromJson(Map<String, Object?> json) {
    final name = json['name']?.toString() ?? json['path']?.toString() ?? '';
    final childrenJson = json['children'];
    return FileNode(
      path: json['path']?.toString() ?? name,
      name: name,
      type: fileNodeTypeFromJson(json['type'], name),
      size: _asInt(json['size']),
      modified: _asDate(json['modified']),
      children: childrenJson is List
          ? childrenJson.whereType<Map<String, Object?>>().map(FileNode.fromJson).toList()
          : const <FileNode>[],
    );
  }
}

class ConnectionSettings {
  const ConnectionSettings({
    this.host = '',
    this.port = 8080,
    this.token = '',
    this.lastProjectId,
    this.lastConversationId,
    this.themeMode = 'system',
  });

  final String host;
  final int port;
  final String token;
  final String? lastProjectId;
  final String? lastConversationId;
  final String themeMode;

  bool get isConfigured => host.trim().isNotEmpty && port > 0;
  Uri get baseUri => Uri.parse('http://$host:$port/api/v1');

  Uri wsUri(String projectId) {
    return Uri.parse('ws://$host:$port/ws?project_id=${Uri.encodeComponent(projectId)}');
  }

  ConnectionSettings copyWith({
    String? host,
    int? port,
    String? token,
    String? lastProjectId,
    String? lastConversationId,
    String? themeMode,
  }) {
    return ConnectionSettings(
      host: host ?? this.host,
      port: port ?? this.port,
      token: token ?? this.token,
      lastProjectId: lastProjectId ?? this.lastProjectId,
      lastConversationId: lastConversationId ?? this.lastConversationId,
      themeMode: themeMode ?? this.themeMode,
    );
  }
}

class TurnResponse {
  const TurnResponse({required this.turnId, required this.status});

  final String turnId;
  final String status;

  factory TurnResponse.fromJson(Map<String, Object?> json) {
    return TurnResponse(turnId: json['turn_id']?.toString() ?? '', status: json['status']?.toString() ?? 'queued');
  }
}

int _asInt(Object? value) => _asNullableInt(value) ?? 0;

int? _asNullableInt(Object? value) {
  if (value is int) {
    return value;
  }
  return int.tryParse('$value');
}

double? _asNullableDouble(Object? value) {
  if (value is num) {
    return value.toDouble();
  }
  return double.tryParse('$value');
}

DateTime? _asDate(Object? value) {
  final text = value?.toString();
  if (text == null || text.isEmpty) {
    return null;
  }
  return DateTime.tryParse(text);
}
