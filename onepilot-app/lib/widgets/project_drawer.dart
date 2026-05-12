import 'package:flutter/material.dart';
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
