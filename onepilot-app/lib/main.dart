import 'package:flutter/material.dart';

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
