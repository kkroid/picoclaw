import 'package:flutter/material.dart';
import 'package:provider/provider.dart';

import '../../providers/conversation_provider.dart';
import '../../widgets/chat_bubble.dart';

class ConversationDetailPage extends StatefulWidget {
  const ConversationDetailPage({super.key});

  @override
  State<ConversationDetailPage> createState() => _ConversationDetailPageState();
}

class _ConversationDetailPageState extends State<ConversationDetailPage> {
  final inputController = TextEditingController();
  late ConversationProvider conversations;

  @override
  void didChangeDependencies() {
    super.didChangeDependencies();
    conversations = context.read<ConversationProvider>();
  }

  @override
  void dispose() {
    conversations.leaveConversation();
    inputController.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    return Consumer<ConversationProvider>(
      builder: (context, conversations, _) {
        final title = conversations.activeConversation?.title ?? '对话详情';
        return Scaffold(
          appBar: AppBar(
            title: Text(title),
            actions: [
              IconButton(
                key: const Key('emergency-stop-button'),
                onPressed: conversations.running ? () => _confirmEmergencyStop(context, conversations) : null,
                icon: const Icon(Icons.stop_circle),
              ),
            ],
          ),
          body: Column(
            children: [
              if (conversations.loading) const LinearProgressIndicator(),
              for (final status in conversations.statusMessages)
                Padding(
                  padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 2),
                  child: Align(alignment: Alignment.centerLeft, child: Text(status, style: Theme.of(context).textTheme.bodySmall)),
                ),
              Expanded(
                child: ListView.builder(
                  padding: const EdgeInsets.symmetric(vertical: 12),
                  itemCount: conversations.messages.length,
                  itemBuilder: (context, index) => ChatBubble(message: conversations.messages[index]),
                ),
              ),
              SafeArea(
                child: Padding(
                  padding: const EdgeInsets.all(8),
                  child: Row(
                    children: [
                      Expanded(
                        child: TextField(
                          key: const Key('message-field'),
                          controller: inputController,
                          minLines: 1,
                          maxLines: 4,
                          decoration: const InputDecoration(hintText: '输入消息...'),
                        ),
                      ),
                      IconButton(
                        key: const Key('send-message-button'),
                        icon: const Icon(Icons.send),
                        onPressed: () async {
                          final text = inputController.text;
                          inputController.clear();
                          await conversations.sendMessage(text);
                        },
                      ),
                    ],
                  ),
                ),
              ),
            ],
          ),
        );
      },
    );
  }

  Future<void> _confirmEmergencyStop(BuildContext context, ConversationProvider conversations) async {
    final confirmed = await showDialog<bool>(
      context: context,
      builder: (context) => AlertDialog(
        title: const Text('确认急停'),
        content: const Text('确定要急停当前对话？'),
        actions: [
          TextButton(onPressed: () => Navigator.of(context).pop(false), child: const Text('取消')),
          FilledButton(onPressed: () => Navigator.of(context).pop(true), child: const Text('急停')),
        ],
      ),
    );
    if (confirmed == true) {
      await conversations.emergencyStop();
    }
  }
}
