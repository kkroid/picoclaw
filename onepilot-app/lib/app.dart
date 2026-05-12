import 'package:flutter/material.dart';
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
