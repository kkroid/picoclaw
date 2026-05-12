import 'package:flutter/material.dart';
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
