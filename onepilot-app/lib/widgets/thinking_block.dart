import 'package:flutter/material.dart';

class ThinkingBlock extends StatefulWidget {
  const ThinkingBlock({super.key, required this.text});

  final String text;

  @override
  State<ThinkingBlock> createState() => _ThinkingBlockState();
}

class _ThinkingBlockState extends State<ThinkingBlock> {
  bool expanded = false;

  @override
  Widget build(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.only(top: 8),
      child: InkWell(
        onTap: () => setState(() => expanded = !expanded),
        child: DecoratedBox(
          decoration: BoxDecoration(color: Theme.of(context).colorScheme.surface, borderRadius: BorderRadius.circular(6)),
          child: Padding(
            padding: const EdgeInsets.all(8),
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                const Text('Thinking'),
                if (expanded) Text(widget.text),
              ],
            ),
          ),
        ),
      ),
    );
  }
}
