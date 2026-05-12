import 'package:flutter/material.dart';
import 'package:flutter_markdown/flutter_markdown.dart';

import '../models/protocol_models.dart';
import 'thinking_block.dart';

class ChatBubble extends StatelessWidget {
  const ChatBubble({super.key, required this.message});

  final Message message;

  @override
  Widget build(BuildContext context) {
    final isUser = message.role == MessageRole.user;
    final reasoning = message.segments.where((segment) => segment.kind == MessageSegmentKind.reasoning).toList();
    final toolCalls = message.segments.where((segment) => segment.kind == MessageSegmentKind.toolCall).toList();
    return Align(
      alignment: isUser ? Alignment.centerRight : Alignment.centerLeft,
      child: Container(
        constraints: const BoxConstraints(maxWidth: 560),
        margin: const EdgeInsets.symmetric(vertical: 6, horizontal: 12),
        padding: const EdgeInsets.all(12),
        decoration: BoxDecoration(
          color: isUser ? Theme.of(context).colorScheme.primary : Theme.of(context).colorScheme.surfaceContainerHighest,
          borderRadius: BorderRadius.circular(8),
        ),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            MarkdownBody(data: message.content.isEmpty ? ' ' : message.content, selectable: true),
            for (final item in reasoning) ThinkingBlock(text: item.content),
            for (final item in toolCalls)
              Padding(
                padding: const EdgeInsets.only(top: 6),
                child: Text('正在执行: ${item.content}', style: Theme.of(context).textTheme.bodySmall),
              ),
            if (message.usage != null)
              Padding(
                padding: const EdgeInsets.only(top: 6),
                child: Text('${message.usage!.inputTokens ?? 0} in / ${message.usage!.outputTokens ?? 0} out', style: Theme.of(context).textTheme.labelSmall),
              ),
          ],
        ),
      ),
    );
  }
}
