import 'package:flutter/material.dart';
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
