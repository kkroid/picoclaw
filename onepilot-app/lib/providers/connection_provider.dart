import 'package:flutter/foundation.dart';

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
