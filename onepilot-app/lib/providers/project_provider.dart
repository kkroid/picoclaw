import 'package:flutter/foundation.dart';

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
