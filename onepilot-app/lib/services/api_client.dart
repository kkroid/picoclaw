import 'dart:convert';
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
