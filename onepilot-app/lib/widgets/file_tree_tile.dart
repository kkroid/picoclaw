import 'package:flutter/material.dart';

import '../models/protocol_models.dart';

class FileTreeTile extends StatelessWidget {
  const FileTreeTile({super.key, required this.node, required this.onOpen});

  final FileNode node;
  final ValueChanged<FileNode> onOpen;

  @override
  Widget build(BuildContext context) {
    final icon = switch (node.type) {
      FileNodeType.directory => Icons.folder,
      FileNodeType.image => Icons.image,
      FileNodeType.file => Icons.description,
    };
    return ListTile(
      leading: Icon(icon),
      title: Text(node.name),
      subtitle: Text('${node.size} B'),
      onTap: () => onOpen(node),
    );
  }
}
