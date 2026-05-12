import 'package:shared_preferences/shared_preferences.dart';

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
