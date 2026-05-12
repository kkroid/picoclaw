import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:provider/provider.dart';

import '../../models/protocol_models.dart';
import '../../providers/file_provider.dart';
import '../../services/api_client.dart';

class FilePreviewPage extends StatefulWidget {
  const FilePreviewPage({super.key, required this.node});

  final FileNode node;

  @override
  State<FilePreviewPage> createState() => _FilePreviewPageState();
}

class _FilePreviewPageState extends State<FilePreviewPage> {
  final textController = TextEditingController();
  FileContent? lastContent;

  @override
  void dispose() {
    textController.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final files = context.watch<FileProvider>();
    final content = files.activeContent;
    if (content != null && !content.isImage && content != lastContent) {
      textController.text = content.text;
      lastContent = content;
    }
    return Scaffold(
      appBar: AppBar(
        title: Text(widget.node.name),
        actions: [
          if (content != null && !content.isImage)
            IconButton(
              key: const Key('copy-file-button'),
              icon: const Icon(Icons.copy),
              onPressed: () => Clipboard.setData(ClipboardData(text: content.text)),
            ),
          if (content != null && !content.isImage)
            IconButton(
              key: const Key('save-file-button'),
              icon: files.saving ? const SizedBox(width: 18, height: 18, child: CircularProgressIndicator(strokeWidth: 2)) : const Icon(Icons.save),
              onPressed: files.saving
                  ? null
                  : () async {
                      await files.writeTextFile(textController.text);
                      if (context.mounted) {
                        ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(files.saveMessage ?? '已保存')));
                      }
                    },
            ),
        ],
      ),
      body: content == null
          ? const Center(child: CircularProgressIndicator())
          : content.isImage
              ? InteractiveViewer(child: Center(child: Image.memory(content.bytes)))
              : Padding(
                  padding: const EdgeInsets.all(16),
                  child: TextField(
                    key: const Key('file-content-field'),
                    controller: textController,
                    maxLines: null,
                    expands: true,
                    textAlignVertical: TextAlignVertical.top,
                    decoration: InputDecoration(
                      border: const OutlineInputBorder(),
                      errorText: files.errorMessage,
                    ),
                    style: const TextStyle(fontFamily: 'monospace'),
                  ),
                ),
    );
  }
}
