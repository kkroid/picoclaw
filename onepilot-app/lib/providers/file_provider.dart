import 'dart:convert';

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
