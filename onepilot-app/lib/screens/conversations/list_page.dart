import 'package:flutter/material.dart';
import 'package:provider/provider.dart';

import '../../models/protocol_models.dart';
import '../../providers/conversation_provider.dart';
import 'detail_page.dart';

class ConversationListPage extends StatelessWidget {
  const ConversationListPage({super.key});

  @override
  Widget build(BuildContext context) {
    return Consumer<ConversationProvider>(
      builder: (context, conversations, _) {
        return Scaffold(
          body: RefreshIndicator(
            onRefresh: conversations.loadConversations,
            child: conversations.conversations.isEmpty
                ? ListView(children: const [SizedBox(height: 180), Center(child: Text('暂无对话，点击右下角开始'))])
                : ListView.builder(
                    itemCount: conversations.conversations.length,
                    itemBuilder: (context, index) {
                      final conversation = conversations.conversations[index];
                      return ListTile(
                        title: Text(conversation.title),
                        subtitle: Text('${conversationStatusLabel(conversation.status)} · ${conversation.lastMessagePreview}'),
                        trailing: Text(conversation.source == ConversationSource.task ? 'task' : 'thread'),
                        onTap: () async {
                          await conversations.openConversation(conversation);
                          if (context.mounted) {
                            await Navigator.of(context).push(MaterialPageRoute<void>(builder: (_) => const ConversationDetailPage()));
                          }
                        },
                      );
                    },
                  ),
          ),
          floatingActionButton: FloatingActionButton(
            key: const Key('new-conversation-button'),
            onPressed: () async {
              final title = await _askTitle(context);
              if (title != null && title.trim().isNotEmpty) {
                await conversations.createConversation(title);
                if (context.mounted) {
                  await Navigator.of(context).push(MaterialPageRoute<void>(builder: (_) => const ConversationDetailPage()));
                }
              }
            },
            child: const Icon(Icons.add),
          ),
        );
      },
    );
  }

  Future<String?> _askTitle(BuildContext context) async {
    final controller = TextEditingController(text: '新对话');
    return showDialog<String>(
      context: context,
      builder: (context) => AlertDialog(
        title: const Text('新建对话'),
        content: TextField(controller: controller, autofocus: true),
        actions: [
          TextButton(onPressed: () => Navigator.of(context).pop(), child: const Text('取消')),
          FilledButton(onPressed: () => Navigator.of(context).pop(controller.text), child: const Text('创建')),
        ],
      ),
    );
  }
}
