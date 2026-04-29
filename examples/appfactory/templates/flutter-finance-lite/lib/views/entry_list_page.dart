import 'package:flutter/material.dart';

import '../controllers/entry_list_controller.dart';
import '../models/entry.dart';

class EntryListPage extends StatelessWidget {
  const EntryListPage({super.key, required this.controller});

  final EntryListController controller;

  @override
  Widget build(BuildContext context) {
    final entries = controller.entries;
    return Scaffold(
      appBar: AppBar(title: const Text('全部账单')),
      body: entries.isEmpty
          ? const Center(child: Text('暂无账单，请先从首页新增一笔记录。'))
          : ListView.separated(
              padding: const EdgeInsets.all(20),
              itemBuilder: (context, index) {
                final entry = entries[index];
                return _EntryRow(entry: entry);
              },
              separatorBuilder: (_, __) => const SizedBox(height: 12),
              itemCount: entries.length,
            ),
    );
  }
}

class _EntryRow extends StatelessWidget {
  const _EntryRow({required this.entry});

  final BookkeepingEntry entry;

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.all(16),
      decoration: BoxDecoration(
        color: Colors.white,
        borderRadius: BorderRadius.circular(18),
      ),
      child: Row(
        children: [
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text(
                  entry.note.isEmpty ? entry.category : entry.note,
                  style: const TextStyle(fontWeight: FontWeight.w700),
                ),
                const SizedBox(height: 4),
                Text(
                  '${entry.category} · ${entry.date.year}-${entry.date.month.toString().padLeft(2, '0')}-${entry.date.day.toString().padLeft(2, '0')}',
                ),
              ],
            ),
          ),
          Text(
            '${entry.type == EntryType.expense ? '-' : '+'}${entry.amount.toStringAsFixed(2)}',
            style: TextStyle(
              fontWeight: FontWeight.w700,
              color: entry.type == EntryType.expense
                  ? const Color(0xFFB04B2D)
                  : const Color(0xFF1F7A5C),
            ),
          ),
        ],
      ),
    );
  }
}