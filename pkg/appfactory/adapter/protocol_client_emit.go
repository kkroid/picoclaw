package adapter

import (
	"os"
	"path/filepath"
	"strings"

	appprepare "github.com/sipeed/oneappfactory/pkg/appfactory/prepare"
	appruns "github.com/sipeed/oneappfactory/pkg/appfactory/runs"
)

func deterministicProtocolClientOperations(workspacePath string, dm appprepare.DomainModel) []appruns.WorkspacePatchOperation {
	if !isProtocolClientDomainModel(dm) {
		return nil
	}
	files := protocolClientFileContents()
	ops := make([]appruns.WorkspacePatchOperation, 0, len(files)+8)
	for _, path := range sortedProtocolClientFilePaths(files) {
		ops = append(ops, appruns.WorkspacePatchOperation{Type: "write_file", Path: path, Content: files[path]})
	}
	for _, legacyPath := range protocolClientLegacyOpenLitePaths() {
		if workspaceFileExists(workspacePath, legacyPath) {
			ops = append(ops, appruns.WorkspacePatchOperation{Type: "delete_file", Path: legacyPath})
		}
	}
	return ops
}

func isProtocolClientDomainModel(dm appprepare.DomainModel) bool {
	if dm.ProtocolContract != nil && len(dm.ProtocolContract.Endpoints) > 0 {
		return true
	}
	for _, capability := range dm.CapabilityFlags {
		switch strings.TrimSpace(capability) {
		case "rest-api", "websocket-streaming", "connection-settings":
			return true
		}
	}
	return false
}

func sortedProtocolClientFilePaths(files map[string]string) []string {
	preferredOrder := []string{
		"pubspec.yaml",
		"android/app/build.gradle.kts",
		"android/app/src/main/AndroidManifest.xml",
		"android/app/src/main/res/values/strings.xml",
		"lib/models/protocol_models.dart",
		"lib/services/settings_store.dart",
		"lib/services/api_client.dart",
		"lib/services/ws_client.dart",
		"lib/providers/connection_provider.dart",
		"lib/providers/project_provider.dart",
		"lib/providers/conversation_provider.dart",
		"lib/providers/file_provider.dart",
		"lib/widgets/connection_indicator.dart",
		"lib/widgets/project_drawer.dart",
		"lib/widgets/chat_bubble.dart",
		"lib/widgets/thinking_block.dart",
		"lib/widgets/file_tree_tile.dart",
		"lib/screens/settings/connection_page.dart",
		"lib/screens/conversations/list_page.dart",
		"lib/screens/conversations/detail_page.dart",
		"lib/screens/files/browser_page.dart",
		"lib/screens/files/file_preview_page.dart",
		"lib/screens/home.dart",
		"lib/app.dart",
		"lib/main.dart",
		"test/widget_test.dart",
	}
	paths := make([]string, 0, len(files))
	seen := map[string]struct{}{}
	for _, path := range preferredOrder {
		if _, ok := files[path]; ok {
			paths = append(paths, path)
			seen[path] = struct{}{}
		}
	}
	for path := range files {
		if _, ok := seen[path]; !ok {
			paths = append(paths, path)
		}
	}
	return paths
}

func workspaceFileExists(workspacePath, relPath string) bool {
	info, err := os.Stat(filepath.Join(workspacePath, filepath.FromSlash(relPath)))
	return err == nil && !info.IsDir()
}

func protocolClientLegacyOpenLitePaths() []string {
	return []string{
		"lib/models/record.dart",
		"lib/models/dashboard_summary.dart",
		"lib/repositories/record_repository.dart",
		"lib/controllers/home_controller.dart",
		"lib/controllers/record_list_controller.dart",
		"lib/controllers/record_form_controller.dart",
		"lib/views/home_page.dart",
		"lib/views/record_list_page.dart",
		"lib/views/record_detail_page.dart",
		"lib/views/record_form_page.dart",
		"lib/template/open_lite_copy.dart",
	}
}

func protocolClientFileContents() map[string]string {
	return map[string]string{
		"pubspec.yaml":                                protocolPubspec(),
		"android/app/build.gradle.kts":                protocolAndroidBuildGradleKTS(),
		"android/app/src/main/AndroidManifest.xml":    protocolAndroidManifest(),
		"android/app/src/main/res/values/strings.xml": protocolAndroidStrings(),
		"lib/main.dart":                               protocolMainDart(),
		"lib/app.dart":                                protocolAppDart(),
		"lib/models/protocol_models.dart":             protocolModelsDart(),
		"lib/services/settings_store.dart":            protocolSettingsStoreDart(),
		"lib/services/api_client.dart":                protocolApiClientDart(),
		"lib/services/ws_client.dart":                 protocolWsClientDart(),
		"lib/providers/connection_provider.dart":      protocolConnectionProviderDart(),
		"lib/providers/project_provider.dart":         protocolProjectProviderDart(),
		"lib/providers/conversation_provider.dart":    protocolConversationProviderDart(),
		"lib/providers/file_provider.dart":            protocolFileProviderDart(),
		"lib/screens/home.dart":                       protocolHomeDart(),
		"lib/screens/settings/connection_page.dart":   protocolConnectionPageDart(),
		"lib/screens/conversations/list_page.dart":    protocolConversationListPageDart(),
		"lib/screens/conversations/detail_page.dart":  protocolConversationDetailPageDart(),
		"lib/screens/files/browser_page.dart":         protocolFileBrowserPageDart(),
		"lib/screens/files/file_preview_page.dart":    protocolFilePreviewPageDart(),
		"lib/widgets/project_drawer.dart":             protocolProjectDrawerDart(),
		"lib/widgets/chat_bubble.dart":                protocolChatBubbleDart(),
		"lib/widgets/thinking_block.dart":             protocolThinkingBlockDart(),
		"lib/widgets/connection_indicator.dart":       protocolConnectionIndicatorDart(),
		"lib/widgets/file_tree_tile.dart":             protocolFileTreeTileDart(),
		"test/widget_test.dart":                       protocolWidgetTestDart(),
	}
}

func protocolPubspec() string {
	return `name: flutter_open_lite
description: "A protocol-client Flutter MVP generated by AppFactory."
publish_to: 'none'
version: 1.0.0+1

environment:
  sdk: ^3.7.2

dependencies:
  flutter:
    sdk: flutter
  cupertino_icons: ^1.0.8
  http: ^1.2.0
  web_socket_channel: ^2.4.0
  flutter_markdown: ^0.7.0
  shared_preferences: ^2.2.0
  provider: ^6.1.0

dev_dependencies:
  flutter_test:
    sdk: flutter
  flutter_lints: ^5.0.0

flutter:
  uses-material-design: true
  assets:
    - assets/
`
}

func protocolAndroidManifest() string {
	return `<manifest xmlns:android="http://schemas.android.com/apk/res/android">
    <uses-permission android:name="android.permission.INTERNET" />
    <application
        android:label="@string/app_name"
        android:name="${applicationName}"
        android:icon="@mipmap/ic_launcher"
        android:usesCleartextTraffic="true">
        <activity
            android:name=".MainActivity"
            android:exported="true"
            android:launchMode="singleTop"
            android:taskAffinity=""
            android:theme="@style/LaunchTheme"
            android:configChanges="orientation|keyboardHidden|keyboard|screenSize|smallestScreenSize|locale|layoutDirection|fontScale|screenLayout|density|uiMode"
            android:hardwareAccelerated="true"
            android:windowSoftInputMode="adjustResize">
            <meta-data
              android:name="io.flutter.embedding.android.NormalTheme"
              android:resource="@style/NormalTheme"
              />
            <intent-filter>
                <action android:name="android.intent.action.MAIN"/>
                <category android:name="android.intent.category.LAUNCHER"/>
            </intent-filter>
        </activity>
        <meta-data
            android:name="flutterEmbedding"
            android:value="2" />
    </application>
    <queries>
        <intent>
            <action android:name="android.intent.action.PROCESS_TEXT"/>
            <data android:mimeType="text/plain"/>
        </intent>
    </queries>
</manifest>
`
}

func protocolAndroidStrings() string {
	return `<resources>
    <string name="app_name">OnePilot</string>
</resources>
`
}

func protocolAndroidBuildGradleKTS() string {
	return `val defaultOpenLiteApplicationId = "com.appfactory.onepilot"

plugins {
  id("com.android.application")
  id("kotlin-android")
  id("dev.flutter.flutter-gradle-plugin")
}

android {
  namespace = defaultOpenLiteApplicationId
  compileSdk = flutter.compileSdkVersion
  ndkVersion = "27.0.12077973"

  compileOptions {
    sourceCompatibility = JavaVersion.VERSION_11
    targetCompatibility = JavaVersion.VERSION_11
  }

  kotlinOptions {
    jvmTarget = JavaVersion.VERSION_11.toString()
  }

  defaultConfig {
    applicationId = defaultOpenLiteApplicationId
    minSdk = flutter.minSdkVersion
    targetSdk = flutter.targetSdkVersion
    versionCode = flutter.versionCode
    versionName = flutter.versionName
    ndk {
      abiFilters.add("arm64-v8a")
    }
  }

  buildTypes {
    release {
      signingConfig = signingConfigs.getByName("debug")
    }
  }
}

flutter {
  source = "../.."
}
`
}

func protocolMainDart() string {
	return `import 'package:flutter/material.dart';

import 'app.dart';
import 'services/api_client.dart';
import 'services/settings_store.dart';
import 'services/ws_client.dart';

Future<void> main() async {
  WidgetsFlutterBinding.ensureInitialized();
  final settingsStore = SharedPreferencesConnectionSettingsStore();
  final apiClient = HttpOnePilotApiClient();
  final wsClient = WebSocketOnePilotWsClient();
  runApp(OnePilotApp(
    apiClient: apiClient,
    wsClient: wsClient,
    settingsStore: settingsStore,
  ));
}
`
}

func protocolAppDart() string {
	return `import 'package:flutter/material.dart';
import 'package:provider/provider.dart';

import 'providers/connection_provider.dart';
import 'providers/conversation_provider.dart';
import 'providers/file_provider.dart';
import 'providers/project_provider.dart';
import 'screens/home.dart';
import 'screens/settings/connection_page.dart';
import 'services/api_client.dart';
import 'services/settings_store.dart';
import 'services/ws_client.dart';

class OnePilotApp extends StatelessWidget {
  const OnePilotApp({
    super.key,
    required this.apiClient,
    required this.wsClient,
    required this.settingsStore,
  });

  final OnePilotApiClient apiClient;
  final OnePilotWsClient wsClient;
  final ConnectionSettingsStore settingsStore;

  @override
  Widget build(BuildContext context) {
    return MultiProvider(
      providers: [
        ChangeNotifierProvider(
          create: (_) => ConnectionProvider(
            apiClient: apiClient,
            settingsStore: settingsStore,
          )..restore(),
        ),
        ChangeNotifierProxyProvider<ConnectionProvider, ProjectProvider>(
          create: (_) => ProjectProvider(apiClient: apiClient, settingsStore: settingsStore),
          update: (_, connection, projectProvider) => (projectProvider ?? ProjectProvider(apiClient: apiClient, settingsStore: settingsStore))
            ..attachConnection(connection),
        ),
        ChangeNotifierProxyProvider2<ConnectionProvider, ProjectProvider, ConversationProvider>(
          create: (_) => ConversationProvider(apiClient: apiClient, wsClient: wsClient, settingsStore: settingsStore),
          update: (_, connection, projects, conversationProvider) => (conversationProvider ?? ConversationProvider(apiClient: apiClient, wsClient: wsClient, settingsStore: settingsStore))
            ..attach(connection, projects),
        ),
        ChangeNotifierProxyProvider2<ConnectionProvider, ProjectProvider, FileProvider>(
          create: (_) => FileProvider(apiClient: apiClient),
          update: (_, connection, projects, fileProvider) => (fileProvider ?? FileProvider(apiClient: apiClient))
            ..attach(connection, projects),
        ),
      ],
      child: MaterialApp(
        title: 'OnePilot',
        theme: ThemeData(
          colorScheme: ColorScheme.fromSeed(seedColor: const Color(0xFF006D77)),
          useMaterial3: true,
        ),
        darkTheme: ThemeData(
          colorScheme: ColorScheme.fromSeed(seedColor: const Color(0xFF83C5BE), brightness: Brightness.dark),
          useMaterial3: true,
        ),
        home: const _AppRoot(),
      ),
    );
  }
}

class _AppRoot extends StatelessWidget {
  const _AppRoot();

  @override
  Widget build(BuildContext context) {
    return Consumer<ConnectionProvider>(
      builder: (context, connection, _) {
        if (!connection.settings.isConfigured || connection.status == ConnectionStatus.disconnected) {
          return const ConnectionPage();
        }
        return const HomePage();
      },
    );
  }
}
`
}

func protocolModelsDart() string {
	return `enum ProjectStatus { running, stopped, error }

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
`
}

func protocolSettingsStoreDart() string {
	return `import 'package:shared_preferences/shared_preferences.dart';

import '../models/protocol_models.dart';

abstract class ConnectionSettingsStore {
  Future<ConnectionSettings> load();
  Future<void> save(ConnectionSettings settings);
}

class SharedPreferencesConnectionSettingsStore implements ConnectionSettingsStore {
  @override
  Future<ConnectionSettings> load() async {
    final prefs = await SharedPreferences.getInstance();
    return ConnectionSettings(
      host: prefs.getString('connection_host') ?? '',
      port: prefs.getInt('connection_port') ?? 8080,
      token: prefs.getString('connection_token') ?? '',
      lastProjectId: prefs.getString('last_project_id'),
      lastConversationId: prefs.getString('last_conversation_id'),
      themeMode: prefs.getString('theme_mode') ?? 'system',
    );
  }

  @override
  Future<void> save(ConnectionSettings settings) async {
    final prefs = await SharedPreferences.getInstance();
    await prefs.setString('connection_host', settings.host);
    await prefs.setInt('connection_port', settings.port);
    await prefs.setString('connection_token', settings.token);
    await prefs.setString('theme_mode', settings.themeMode);
    if (settings.lastProjectId != null) {
      await prefs.setString('last_project_id', settings.lastProjectId!);
    }
    if (settings.lastConversationId != null) {
      await prefs.setString('last_conversation_id', settings.lastConversationId!);
    }
  }
}

class MemoryConnectionSettingsStore implements ConnectionSettingsStore {
  MemoryConnectionSettingsStore([this._settings = const ConnectionSettings()]);

  ConnectionSettings _settings;

  @override
  Future<ConnectionSettings> load() async => _settings;

  @override
  Future<void> save(ConnectionSettings settings) async {
    _settings = settings;
  }
}
`
}

func protocolApiClientDart() string {
	return `import 'dart:convert';
import 'dart:typed_data';

import 'package:http/http.dart' as http;

import '../models/protocol_models.dart';

class FileContent {
  const FileContent({required this.contentType, required this.bytes});

  final String contentType;
  final Uint8List bytes;

  bool get isImage => contentType.startsWith('image/');
  String get text => utf8.decode(bytes, allowMalformed: true);
}

abstract class OnePilotApiClient {
  Future<SystemInfo> getSystemInfo(ConnectionSettings settings);
  Future<List<Project>> getProjects(ConnectionSettings settings);
  Future<Project> getProject(ConnectionSettings settings, String projectId);
  Future<List<Conversation>> getConversations(ConnectionSettings settings, String projectId);
  Future<Conversation> createConversation(ConnectionSettings settings, String projectId, String title);
  Future<Conversation> getConversation(ConnectionSettings settings, String projectId, String conversationId);
  Future<TurnResponse> sendTurn(ConnectionSettings settings, String projectId, String conversationId, String message);
  Future<void> emergencyStop(ConnectionSettings settings, String projectId, String conversationId);
  Future<FileNode> getFileTree(ConnectionSettings settings, String projectId, String path);
  Future<FileContent> readFile(ConnectionSettings settings, String projectId, String path);
  Future<void> writeFile(ConnectionSettings settings, String projectId, String path, String content);
}

class HttpOnePilotApiClient implements OnePilotApiClient {
  HttpOnePilotApiClient({http.Client? client}) : _client = client ?? http.Client();

  final http.Client _client;

  Uri _uri(ConnectionSettings settings, String path, [Map<String, String?> query = const <String, String?>{}]) {
    final filtered = <String, String>{};
    for (final entry in query.entries) {
      if (entry.value != null && entry.value!.isNotEmpty) {
        filtered[entry.key] = entry.value!;
      }
    }
    return settings.baseUri.replace(path: '${settings.baseUri.path}$path', queryParameters: filtered.isEmpty ? null : filtered);
  }

  Map<String, String> _headers(ConnectionSettings settings, [String? projectId]) {
    final headers = <String, String>{'Content-Type': 'application/json'};
    if (settings.token.isNotEmpty) {
      headers['Authorization'] = 'Bearer ${settings.token}';
    }
    if (projectId != null && projectId.isNotEmpty) {
      headers['X-Project-Id'] = projectId;
    }
    return headers;
  }

  Future<Object?> _json(http.Response response) async {
    final decoded = jsonDecode(response.body) as Map<String, Object?>;
    if (response.statusCode >= 400 || decoded.containsKey('error')) {
      final error = decoded['error'];
      if (error is Map<String, Object?>) {
        throw Exception(error['message']?.toString() ?? error['code']?.toString() ?? '请求失败');
      }
      throw Exception('请求失败: ${response.statusCode}');
    }
    return decoded['data'];
  }

  @override
  Future<SystemInfo> getSystemInfo(ConnectionSettings settings) async {
    // GET /system/info
    final response = await _client.get(_uri(settings, '/system/info'), headers: _headers(settings));
    return SystemInfo.fromJson(await _json(response) as Map<String, Object?>);
  }

  @override
  Future<List<Project>> getProjects(ConnectionSettings settings) async {
    // GET /projects
    final response = await _client.get(_uri(settings, '/projects'), headers: _headers(settings));
    final data = await _json(response) as List<Object?>;
    return data.whereType<Map<String, Object?>>().map(Project.fromJson).toList();
  }

  @override
  Future<Project> getProject(ConnectionSettings settings, String projectId) async {
    // GET /projects/{id}
    final response = await _client.get(_uri(settings, '/projects/$projectId'), headers: _headers(settings, projectId));
    return Project.fromJson(await _json(response) as Map<String, Object?>);
  }

  @override
  Future<List<Conversation>> getConversations(ConnectionSettings settings, String projectId) async {
    // GET /conversations
    final response = await _client.get(_uri(settings, '/conversations', {'project_id': projectId, 'limit': '50'}), headers: _headers(settings, projectId));
    final data = await _json(response) as List<Object?>;
    return data.whereType<Map<String, Object?>>().map(Conversation.fromJson).toList();
  }

  @override
  Future<Conversation> createConversation(ConnectionSettings settings, String projectId, String title) async {
    // POST /conversations
    final response = await _client.post(
      _uri(settings, '/conversations', {'project_id': projectId}),
      headers: _headers(settings, projectId),
      body: jsonEncode(<String, Object?>{'title': title, 'source': 'thread'}),
    );
    return Conversation.fromJson(await _json(response) as Map<String, Object?>);
  }

  @override
  Future<Conversation> getConversation(ConnectionSettings settings, String projectId, String conversationId) async {
    // GET /conversations/{id}
    final response = await _client.get(_uri(settings, '/conversations/$conversationId', {'project_id': projectId}), headers: _headers(settings, projectId));
    return Conversation.fromJson(await _json(response) as Map<String, Object?>);
  }

  @override
  Future<TurnResponse> sendTurn(ConnectionSettings settings, String projectId, String conversationId, String message) async {
    // POST /conversations/{id}/turns
    final response = await _client.post(
      _uri(settings, '/conversations/$conversationId/turns', {'project_id': projectId}),
      headers: _headers(settings, projectId),
      body: jsonEncode(<String, Object?>{'message': message}),
    );
    return TurnResponse.fromJson(await _json(response) as Map<String, Object?>);
  }

  @override
  Future<void> emergencyStop(ConnectionSettings settings, String projectId, String conversationId) async {
    // POST /conversations/{id}/emergency-stop
    final response = await _client.post(_uri(settings, '/conversations/$conversationId/emergency-stop', {'project_id': projectId}), headers: _headers(settings, projectId));
    await _json(response);
  }

  @override
  Future<FileNode> getFileTree(ConnectionSettings settings, String projectId, String path) async {
    // GET /files/tree
    final response = await _client.get(_uri(settings, '/files/tree', {'project_id': projectId, 'path': path}), headers: _headers(settings, projectId));
    return FileNode.fromJson(await _json(response) as Map<String, Object?>);
  }

  @override
  Future<FileContent> readFile(ConnectionSettings settings, String projectId, String path) async {
    // GET /files/read
    final response = await _client.get(_uri(settings, '/files/read', {'project_id': projectId, 'path': path}), headers: _headers(settings, projectId));
    if (response.statusCode >= 400) {
      throw Exception('读取文件失败: ${response.statusCode}');
    }
    return FileContent(contentType: response.headers['content-type'] ?? 'text/plain', bytes: response.bodyBytes);
  }

  @override
  Future<void> writeFile(ConnectionSettings settings, String projectId, String path, String content) async {
    // PUT /files/write
    final headers = _headers(settings, projectId);
    headers['Content-Type'] = 'text/plain; charset=utf-8';
    final response = await _client.put(
      _uri(settings, '/files/write', {'project_id': projectId, 'path': path}),
      headers: headers,
      body: content,
    );
    await _json(response);
  }
}
`
}

func protocolWsClientDart() string {
	return `import 'dart:async';
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
`
}

func protocolConnectionProviderDart() string {
	return `import 'package:flutter/foundation.dart';

import '../models/protocol_models.dart';
import '../services/api_client.dart';
import '../services/settings_store.dart';

enum ConnectionStatus { disconnected, connecting, connected, error }

class ConnectionProvider extends ChangeNotifier {
  ConnectionProvider({required this.apiClient, required this.settingsStore});

  final OnePilotApiClient apiClient;
  final ConnectionSettingsStore settingsStore;

  ConnectionSettings settings = const ConnectionSettings();
  ConnectionStatus status = ConnectionStatus.disconnected;
  SystemInfo? systemInfo;
  String? errorMessage;

  Future<void> restore() async {
    settings = await settingsStore.load();
    if (settings.isConfigured) {
      status = ConnectionStatus.connected;
    }
    notifyListeners();
  }

  Future<void> connect({required String host, required int port, String token = ''}) async {
    status = ConnectionStatus.connecting;
    errorMessage = null;
    notifyListeners();
    final nextSettings = settings.copyWith(host: host.trim(), port: port, token: token.trim());
    try {
      systemInfo = await apiClient.getSystemInfo(nextSettings);
      settings = nextSettings;
      await settingsStore.save(settings);
      status = ConnectionStatus.connected;
    } catch (error) {
      settings = nextSettings;
      errorMessage = error.toString();
      status = ConnectionStatus.error;
    }
    notifyListeners();
  }

  Future<void> remember({String? projectId, String? conversationId, String? themeMode}) async {
    settings = settings.copyWith(lastProjectId: projectId, lastConversationId: conversationId, themeMode: themeMode);
    await settingsStore.save(settings);
    notifyListeners();
  }
}
`
}

func protocolProjectProviderDart() string {
	return `import 'package:flutter/foundation.dart';

import '../models/protocol_models.dart';
import '../services/api_client.dart';
import '../services/settings_store.dart';
import 'connection_provider.dart';

class ProjectProvider extends ChangeNotifier {
  ProjectProvider({required this.apiClient, required this.settingsStore});

  final OnePilotApiClient apiClient;
  final ConnectionSettingsStore settingsStore;

  ConnectionProvider? _connection;
  List<Project> projects = const <Project>[];
  Project? selectedProject;
  bool loading = false;
  String? errorMessage;

  void attachConnection(ConnectionProvider connection) {
    if (_connection == connection) {
      return;
    }
    _connection = connection;
    if (connection.status == ConnectionStatus.connected && projects.isEmpty) {
      loadProjects();
    }
  }

  Future<void> loadProjects() async {
    final connection = _connection;
    if (connection == null || !connection.settings.isConfigured) {
      return;
    }
    loading = true;
    errorMessage = null;
    notifyListeners();
    try {
      projects = await apiClient.getProjects(connection.settings);
      selectedProject = projects.firstWhere(
        (project) => project.id == connection.settings.lastProjectId,
        orElse: () => projects.isNotEmpty ? projects.first : const Project(id: '', name: ''),
      );
      if (selectedProject?.id.isEmpty ?? true) {
        selectedProject = projects.isNotEmpty ? projects.first : null;
      }
      if (selectedProject != null) {
        await connection.remember(projectId: selectedProject!.id);
      }
    } catch (error) {
      errorMessage = error.toString();
    }
    loading = false;
    notifyListeners();
  }

  Future<void> selectProject(Project project) async {
    selectedProject = project;
    await _connection?.remember(projectId: project.id);
    notifyListeners();
  }
}
`
}

func protocolConversationProviderDart() string {
	return `import 'dart:async';

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
`
}

func protocolFileProviderDart() string {
	return `import 'dart:convert';

import 'package:flutter/foundation.dart';

import '../models/protocol_models.dart';
import '../services/api_client.dart';
import 'connection_provider.dart';
import 'project_provider.dart';

class FileProvider extends ChangeNotifier {
  FileProvider({required this.apiClient});

  final OnePilotApiClient apiClient;

  ConnectionProvider? _connection;
  ProjectProvider? _projects;
  FileNode? root;
  FileNode? activeNode;
  FileContent? activeContent;
  String currentPath = '';
  bool loading = false;
  bool saving = false;
  String? errorMessage;
  String? saveMessage;

  void attach(ConnectionProvider connection, ProjectProvider projects) {
    _connection = connection;
    _projects = projects;
  }

  void resetForProjectSwitch() {
    root = null;
    activeNode = null;
    activeContent = null;
    currentPath = '';
    loading = false;
    saving = false;
    errorMessage = null;
    saveMessage = null;
    notifyListeners();
  }

  Future<void> loadTree([String path = '']) async {
    final connection = _connection;
    final project = _projects?.selectedProject;
    if (connection == null || project == null) {
      return;
    }
    loading = true;
    errorMessage = null;
    notifyListeners();
    try {
      root = await apiClient.getFileTree(connection.settings, project.id, path);
      currentPath = path;
      activeNode = null;
      activeContent = null;
    } catch (error) {
      errorMessage = error.toString();
    }
    loading = false;
    notifyListeners();
  }

  Future<void> readFile(FileNode node) async {
    final connection = _connection;
    final project = _projects?.selectedProject;
    if (connection == null || project == null) {
      return;
    }
    loading = true;
    errorMessage = null;
    notifyListeners();
    try {
      activeNode = node;
      activeContent = await apiClient.readFile(connection.settings, project.id, node.path);
      saveMessage = null;
    } catch (error) {
      errorMessage = error.toString();
    }
    loading = false;
    notifyListeners();
  }

  Future<void> writeTextFile(String text) async {
    final connection = _connection;
    final project = _projects?.selectedProject;
    final node = activeNode;
    if (connection == null || project == null || node == null) {
      return;
    }
    saving = true;
    errorMessage = null;
    saveMessage = null;
    notifyListeners();
    try {
      await apiClient.writeFile(connection.settings, project.id, node.path, text);
      activeContent = FileContent(contentType: activeContent?.contentType ?? 'text/plain', bytes: Uint8List.fromList(utf8.encode(text)));
      saveMessage = '已保存 ${node.path}';
    } catch (error) {
      errorMessage = error.toString();
    }
    saving = false;
    notifyListeners();
  }
}
`
}

func protocolConnectionIndicatorDart() string {
	return `import 'package:flutter/material.dart';

import '../providers/connection_provider.dart';

class ConnectionIndicator extends StatelessWidget {
  const ConnectionIndicator({super.key, required this.status, required this.hostPort});

  final ConnectionStatus status;
  final String hostPort;

  @override
  Widget build(BuildContext context) {
    final connected = status == ConnectionStatus.connected;
    return Row(
      mainAxisSize: MainAxisSize.min,
      children: [
        Icon(Icons.circle, color: connected ? Colors.green : Colors.red, size: 12),
        const SizedBox(width: 6),
        Text(hostPort, overflow: TextOverflow.ellipsis),
      ],
    );
  }
}
`
}

func protocolProjectDrawerDart() string {
	return `import 'package:flutter/material.dart';
import 'package:provider/provider.dart';

import '../models/protocol_models.dart';
import '../providers/conversation_provider.dart';
import '../providers/file_provider.dart';
import '../providers/project_provider.dart';

class ProjectDrawer extends StatelessWidget {
  const ProjectDrawer({super.key});

  @override
  Widget build(BuildContext context) {
    return Drawer(
      child: Consumer3<ProjectProvider, ConversationProvider, FileProvider>(
        builder: (context, projects, conversations, files, _) {
          return ListView(
            children: [
              const DrawerHeader(child: Text('项目列表')),
              if (projects.loading) const LinearProgressIndicator(),
              for (final project in projects.projects)
                ListTile(
                  selected: projects.selectedProject?.id == project.id,
                  leading: Icon(Icons.circle, color: _statusColor(project.status), size: 14),
                  title: Text(project.name),
                  subtitle: Text(projectStatusLabel(project.status)),
                  onTap: () async {
                    await projects.selectProject(project);
                    await conversations.resetForProjectSwitch();
                    await conversations.loadConversations();
                    files.resetForProjectSwitch();
                    await files.loadTree();
                    if (context.mounted) {
                      Navigator.of(context).pop();
                    }
                  },
                ),
              const Divider(),
              const ListTile(leading: Icon(Icons.settings), title: Text('连接配置')),
            ],
          );
        },
      ),
    );
  }

  Color _statusColor(ProjectStatus status) {
    switch (status) {
      case ProjectStatus.running:
        return Colors.green;
      case ProjectStatus.error:
        return Colors.red;
      case ProjectStatus.stopped:
        return Colors.grey;
    }
  }
}
`
}

func protocolChatBubbleDart() string {
	return `import 'package:flutter/material.dart';
import 'package:flutter_markdown/flutter_markdown.dart';

import '../models/protocol_models.dart';
import 'thinking_block.dart';

class ChatBubble extends StatelessWidget {
  const ChatBubble({super.key, required this.message});

  final Message message;

  @override
  Widget build(BuildContext context) {
    final isUser = message.role == MessageRole.user;
    final reasoning = message.segments.where((segment) => segment.kind == MessageSegmentKind.reasoning).toList();
    final toolCalls = message.segments.where((segment) => segment.kind == MessageSegmentKind.toolCall).toList();
    return Align(
      alignment: isUser ? Alignment.centerRight : Alignment.centerLeft,
      child: Container(
        constraints: const BoxConstraints(maxWidth: 560),
        margin: const EdgeInsets.symmetric(vertical: 6, horizontal: 12),
        padding: const EdgeInsets.all(12),
        decoration: BoxDecoration(
          color: isUser ? Theme.of(context).colorScheme.primary : Theme.of(context).colorScheme.surfaceContainerHighest,
          borderRadius: BorderRadius.circular(8),
        ),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            MarkdownBody(data: message.content.isEmpty ? ' ' : message.content, selectable: true),
            for (final item in reasoning) ThinkingBlock(text: item.content),
            for (final item in toolCalls)
              Padding(
                padding: const EdgeInsets.only(top: 6),
                child: Text('正在执行: ${item.content}', style: Theme.of(context).textTheme.bodySmall),
              ),
            if (message.usage != null)
              Padding(
                padding: const EdgeInsets.only(top: 6),
                child: Text('${message.usage!.inputTokens ?? 0} in / ${message.usage!.outputTokens ?? 0} out', style: Theme.of(context).textTheme.labelSmall),
              ),
          ],
        ),
      ),
    );
  }
}
`
}

func protocolThinkingBlockDart() string {
	return `import 'package:flutter/material.dart';

class ThinkingBlock extends StatefulWidget {
  const ThinkingBlock({super.key, required this.text});

  final String text;

  @override
  State<ThinkingBlock> createState() => _ThinkingBlockState();
}

class _ThinkingBlockState extends State<ThinkingBlock> {
  bool expanded = false;

  @override
  Widget build(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.only(top: 8),
      child: InkWell(
        onTap: () => setState(() => expanded = !expanded),
        child: DecoratedBox(
          decoration: BoxDecoration(color: Theme.of(context).colorScheme.surface, borderRadius: BorderRadius.circular(6)),
          child: Padding(
            padding: const EdgeInsets.all(8),
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                const Text('Thinking'),
                if (expanded) Text(widget.text),
              ],
            ),
          ),
        ),
      ),
    );
  }
}
`
}

func protocolFileTreeTileDart() string {
	return `import 'package:flutter/material.dart';

import '../models/protocol_models.dart';

class FileTreeTile extends StatelessWidget {
  const FileTreeTile({super.key, required this.node, required this.onOpen});

  final FileNode node;
  final ValueChanged<FileNode> onOpen;

  @override
  Widget build(BuildContext context) {
    final icon = switch (node.type) {
      FileNodeType.directory => Icons.folder,
      FileNodeType.image => Icons.image,
      FileNodeType.file => Icons.description,
    };
    return ListTile(
      leading: Icon(icon),
      title: Text(node.name),
      subtitle: Text('${node.size} B'),
      onTap: () => onOpen(node),
    );
  }
}
`
}

func protocolConnectionPageDart() string {
	return `import 'package:flutter/material.dart';
import 'package:provider/provider.dart';

import '../../providers/connection_provider.dart';

class ConnectionPage extends StatefulWidget {
  const ConnectionPage({super.key});

  @override
  State<ConnectionPage> createState() => _ConnectionPageState();
}

class _ConnectionPageState extends State<ConnectionPage> {
  final hostController = TextEditingController();
  final portController = TextEditingController(text: '8080');
  final tokenController = TextEditingController();

  @override
  void didChangeDependencies() {
    super.didChangeDependencies();
    final settings = context.read<ConnectionProvider>().settings;
    if (hostController.text.isEmpty) {
      hostController.text = settings.host;
      portController.text = settings.port.toString();
      tokenController.text = settings.token;
    }
  }

  @override
  void dispose() {
    hostController.dispose();
    portController.dispose();
    tokenController.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(title: const Text('连接配置')),
      body: Consumer<ConnectionProvider>(
        builder: (context, connection, _) {
          return ListView(
            padding: const EdgeInsets.all(16),
            children: [
              TextField(key: const Key('host-field'), controller: hostController, decoration: const InputDecoration(labelText: 'PC host')),
              const SizedBox(height: 12),
              TextField(key: const Key('port-field'), controller: portController, keyboardType: TextInputType.number, decoration: const InputDecoration(labelText: '端口')),
              const SizedBox(height: 12),
              TextField(key: const Key('token-field'), controller: tokenController, decoration: const InputDecoration(labelText: '可选 token')),
              const SizedBox(height: 20),
              FilledButton.icon(
                key: const Key('connect-button'),
                onPressed: connection.status == ConnectionStatus.connecting
                    ? null
                    : () => connection.connect(host: hostController.text, port: int.tryParse(portController.text) ?? 8080, token: tokenController.text),
                icon: const Icon(Icons.lan),
                label: const Text('连接'),
              ),
              if (connection.status == ConnectionStatus.connecting) const LinearProgressIndicator(),
              if (connection.errorMessage != null) Padding(padding: const EdgeInsets.only(top: 12), child: Text(connection.errorMessage!, style: TextStyle(color: Theme.of(context).colorScheme.error))),
            ],
          );
        },
      ),
    );
  }
}
`
}

func protocolHomeDart() string {
	return `import 'package:flutter/material.dart';
import 'package:provider/provider.dart';

import '../providers/connection_provider.dart';
import '../providers/conversation_provider.dart';
import '../providers/file_provider.dart';
import '../providers/project_provider.dart';
import '../screens/conversations/list_page.dart';
import '../screens/files/browser_page.dart';
import '../screens/settings/connection_page.dart';
import '../widgets/connection_indicator.dart';
import '../widgets/project_drawer.dart';

class HomePage extends StatefulWidget {
  const HomePage({super.key});

  @override
  State<HomePage> createState() => _HomePageState();
}

class _HomePageState extends State<HomePage> {
  int tabIndex = 0;

  @override
  void initState() {
    super.initState();
    WidgetsBinding.instance.addPostFrameCallback((_) {
      final projects = context.read<ProjectProvider>();
      final conversations = context.read<ConversationProvider>();
      final files = context.read<FileProvider>();
      _loadInitialData(projects, conversations, files);
    });
  }

  Future<void> _loadInitialData(ProjectProvider projects, ConversationProvider conversations, FileProvider files) async {
    await projects.loadProjects();
    await conversations.loadConversations();
    await files.loadTree();
  }

  @override
  Widget build(BuildContext context) {
    final connection = context.watch<ConnectionProvider>();
    final projects = context.watch<ProjectProvider>();
    final currentProject = projects.selectedProject?.name ?? '未选择项目';
    return Scaffold(
      drawer: const ProjectDrawer(),
      appBar: AppBar(
        title: Text(currentProject),
        actions: [
          ConnectionIndicator(status: connection.status, hostPort: '${connection.settings.host}:${connection.settings.port}'),
          IconButton(icon: const Icon(Icons.settings), onPressed: () => Navigator.of(context).push(MaterialPageRoute<void>(builder: (_) => const ConnectionPage()))),
        ],
      ),
      body: tabIndex == 0 ? const ConversationListPage() : const FileBrowserPage(),
      bottomNavigationBar: NavigationBar(
        selectedIndex: tabIndex,
        onDestinationSelected: (value) => setState(() => tabIndex = value),
        destinations: const [
          NavigationDestination(icon: Icon(Icons.chat_bubble_outline), selectedIcon: Icon(Icons.chat_bubble), label: '对话'),
          NavigationDestination(icon: Icon(Icons.folder_outlined), selectedIcon: Icon(Icons.folder), label: '文件'),
        ],
      ),
    );
  }
}
`
}

func protocolConversationListPageDart() string {
	return `import 'package:flutter/material.dart';
import 'package:provider/provider.dart';

import '../../models/protocol_models.dart';
import '../../providers/conversation_provider.dart';
import 'detail_page.dart';

class ConversationListPage extends StatelessWidget {
  const ConversationListPage({super.key});

  @override
  Widget build(BuildContext context) {
    return Consumer<ConversationProvider>(
      builder: (context, conversations, _) {
        return Scaffold(
          body: RefreshIndicator(
            onRefresh: conversations.loadConversations,
            child: conversations.conversations.isEmpty
                ? ListView(children: const [SizedBox(height: 180), Center(child: Text('暂无对话，点击右下角开始'))])
                : ListView.builder(
                    itemCount: conversations.conversations.length,
                    itemBuilder: (context, index) {
                      final conversation = conversations.conversations[index];
                      return ListTile(
                        title: Text(conversation.title),
                        subtitle: Text('${conversationStatusLabel(conversation.status)} · ${conversation.lastMessagePreview}'),
                        trailing: Text(conversation.source == ConversationSource.task ? 'task' : 'thread'),
                        onTap: () async {
                          await conversations.openConversation(conversation);
                          if (context.mounted) {
                            await Navigator.of(context).push(MaterialPageRoute<void>(builder: (_) => const ConversationDetailPage()));
                          }
                        },
                      );
                    },
                  ),
          ),
          floatingActionButton: FloatingActionButton(
            key: const Key('new-conversation-button'),
            onPressed: () async {
              final title = await _askTitle(context);
              if (title != null && title.trim().isNotEmpty) {
                await conversations.createConversation(title);
                if (context.mounted) {
                  await Navigator.of(context).push(MaterialPageRoute<void>(builder: (_) => const ConversationDetailPage()));
                }
              }
            },
            child: const Icon(Icons.add),
          ),
        );
      },
    );
  }

  Future<String?> _askTitle(BuildContext context) async {
    final controller = TextEditingController(text: '新对话');
    return showDialog<String>(
      context: context,
      builder: (context) => AlertDialog(
        title: const Text('新建对话'),
        content: TextField(controller: controller, autofocus: true),
        actions: [
          TextButton(onPressed: () => Navigator.of(context).pop(), child: const Text('取消')),
          FilledButton(onPressed: () => Navigator.of(context).pop(controller.text), child: const Text('创建')),
        ],
      ),
    );
  }
}
`
}

func protocolConversationDetailPageDart() string {
	return `import 'package:flutter/material.dart';
import 'package:provider/provider.dart';

import '../../providers/conversation_provider.dart';
import '../../widgets/chat_bubble.dart';

class ConversationDetailPage extends StatefulWidget {
  const ConversationDetailPage({super.key});

  @override
  State<ConversationDetailPage> createState() => _ConversationDetailPageState();
}

class _ConversationDetailPageState extends State<ConversationDetailPage> {
  final inputController = TextEditingController();
  late ConversationProvider conversations;

  @override
  void didChangeDependencies() {
    super.didChangeDependencies();
    conversations = context.read<ConversationProvider>();
  }

  @override
  void dispose() {
    conversations.leaveConversation();
    inputController.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    return Consumer<ConversationProvider>(
      builder: (context, conversations, _) {
        final title = conversations.activeConversation?.title ?? '对话详情';
        return Scaffold(
          appBar: AppBar(
            title: Text(title),
            actions: [
              IconButton(
                key: const Key('emergency-stop-button'),
                onPressed: conversations.running ? () => _confirmEmergencyStop(context, conversations) : null,
                icon: const Icon(Icons.stop_circle),
              ),
            ],
          ),
          body: Column(
            children: [
              if (conversations.loading) const LinearProgressIndicator(),
              for (final status in conversations.statusMessages)
                Padding(
                  padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 2),
                  child: Align(alignment: Alignment.centerLeft, child: Text(status, style: Theme.of(context).textTheme.bodySmall)),
                ),
              Expanded(
                child: ListView.builder(
                  padding: const EdgeInsets.symmetric(vertical: 12),
                  itemCount: conversations.messages.length,
                  itemBuilder: (context, index) => ChatBubble(message: conversations.messages[index]),
                ),
              ),
              SafeArea(
                child: Padding(
                  padding: const EdgeInsets.all(8),
                  child: Row(
                    children: [
                      Expanded(
                        child: TextField(
                          key: const Key('message-field'),
                          controller: inputController,
                          minLines: 1,
                          maxLines: 4,
                          decoration: const InputDecoration(hintText: '输入消息...'),
                        ),
                      ),
                      IconButton(
                        key: const Key('send-message-button'),
                        icon: const Icon(Icons.send),
                        onPressed: () async {
                          final text = inputController.text;
                          inputController.clear();
                          await conversations.sendMessage(text);
                        },
                      ),
                    ],
                  ),
                ),
              ),
            ],
          ),
        );
      },
    );
  }

  Future<void> _confirmEmergencyStop(BuildContext context, ConversationProvider conversations) async {
    final confirmed = await showDialog<bool>(
      context: context,
      builder: (context) => AlertDialog(
        title: const Text('确认急停'),
        content: const Text('确定要急停当前对话？'),
        actions: [
          TextButton(onPressed: () => Navigator.of(context).pop(false), child: const Text('取消')),
          FilledButton(onPressed: () => Navigator.of(context).pop(true), child: const Text('急停')),
        ],
      ),
    );
    if (confirmed == true) {
      await conversations.emergencyStop();
    }
  }
}
`
}

func protocolFileBrowserPageDart() string {
	return `import 'package:flutter/material.dart';
import 'package:provider/provider.dart';

import '../../models/protocol_models.dart';
import '../../providers/file_provider.dart';
import '../../widgets/file_tree_tile.dart';
import 'file_preview_page.dart';

class FileBrowserPage extends StatelessWidget {
  const FileBrowserPage({super.key});

  @override
  Widget build(BuildContext context) {
    return Consumer<FileProvider>(
      builder: (context, files, _) {
        final children = files.root?.children ?? const <FileNode>[];
        return Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Padding(
              padding: const EdgeInsets.all(12),
              child: Text(files.currentPath.isEmpty ? '项目根目录' : files.currentPath),
            ),
            if (files.loading) const LinearProgressIndicator(),
            if (files.errorMessage != null) Padding(padding: const EdgeInsets.all(12), child: Text(files.errorMessage!)),
            Expanded(
              child: children.isEmpty
                  ? const Center(child: Text('暂无文件'))
                  : ListView(
                      children: [
                        if (files.currentPath.isNotEmpty)
                          ListTile(
                            leading: const Icon(Icons.arrow_upward),
                            title: const Text('..'),
                            onTap: () => files.loadTree(_parentPath(files.currentPath)),
                          ),
                        for (final node in children)
                          FileTreeTile(
                            node: node,
                            onOpen: (node) async {
                              if (node.type == FileNodeType.directory) {
                                await files.loadTree(node.path);
                              } else {
                                await files.readFile(node);
                                if (context.mounted) {
                                  await Navigator.of(context).push(MaterialPageRoute<void>(builder: (_) => FilePreviewPage(node: node)));
                                }
                              }
                            },
                          ),
                      ],
                    ),
            ),
          ],
        );
      },
    );
  }

  String _parentPath(String path) {
    final index = path.lastIndexOf('/');
    return index < 0 ? '' : path.substring(0, index);
  }
}
`
}

func protocolFilePreviewPageDart() string {
	return `import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:provider/provider.dart';

import '../../models/protocol_models.dart';
import '../../providers/file_provider.dart';
import '../../services/api_client.dart';

class FilePreviewPage extends StatefulWidget {
  const FilePreviewPage({super.key, required this.node});

  final FileNode node;

  @override
  State<FilePreviewPage> createState() => _FilePreviewPageState();
}

class _FilePreviewPageState extends State<FilePreviewPage> {
  final textController = TextEditingController();
  FileContent? lastContent;

  @override
  void dispose() {
    textController.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final files = context.watch<FileProvider>();
    final content = files.activeContent;
    if (content != null && !content.isImage && content != lastContent) {
      textController.text = content.text;
      lastContent = content;
    }
    return Scaffold(
      appBar: AppBar(
        title: Text(widget.node.name),
        actions: [
          if (content != null && !content.isImage)
            IconButton(
              key: const Key('copy-file-button'),
              icon: const Icon(Icons.copy),
              onPressed: () => Clipboard.setData(ClipboardData(text: content.text)),
            ),
          if (content != null && !content.isImage)
            IconButton(
              key: const Key('save-file-button'),
              icon: files.saving ? const SizedBox(width: 18, height: 18, child: CircularProgressIndicator(strokeWidth: 2)) : const Icon(Icons.save),
              onPressed: files.saving
                  ? null
                  : () async {
                      await files.writeTextFile(textController.text);
                      if (context.mounted) {
                        ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(files.saveMessage ?? '已保存')));
                      }
                    },
            ),
        ],
      ),
      body: content == null
          ? const Center(child: CircularProgressIndicator())
          : content.isImage
              ? InteractiveViewer(child: Center(child: Image.memory(content.bytes)))
              : Padding(
                  padding: const EdgeInsets.all(16),
                  child: TextField(
                    key: const Key('file-content-field'),
                    controller: textController,
                    maxLines: null,
                    expands: true,
                    textAlignVertical: TextAlignVertical.top,
                    decoration: InputDecoration(
                      border: const OutlineInputBorder(),
                      errorText: files.errorMessage,
                    ),
                    style: const TextStyle(fontFamily: 'monospace'),
                  ),
                ),
    );
  }
}
`
}

func protocolWidgetTestDart() string {
	return `import 'dart:typed_data';

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
`
}
