import 'package:flutter/material.dart';

import '../controllers/entry_list_controller.dart';
import '../controllers/home_controller.dart';
import '../models/entry.dart';

class HomePage extends StatelessWidget {
  const HomePage({
    super.key,
    required this.controller,
    required this.entryListController,
    required this.onCreateEntry,
    required this.onViewLedger,
  });

  final HomeController controller;
  final EntryListController entryListController;
  final Future<void> Function() onCreateEntry;
  final Future<void> Function() onViewLedger;

  @override
  Widget build(BuildContext context) {
    return AnimatedBuilder(
      animation: controller,
      builder: (context, _) {
        final summary = controller.summary;
        final entries = entryListController.entries.take(3).toList();
        return Scaffold(
          appBar: AppBar(
            title: const Text('记账助手'),
          ),
          body: controller.isLoading
              ? const Center(child: CircularProgressIndicator())
              : ListView(
                  padding: const EdgeInsets.all(20),
                  children: [
                    Container(
                      padding: const EdgeInsets.all(20),
                      decoration: BoxDecoration(
                        color: const Color(0xFF1F7A5C),
                        borderRadius: BorderRadius.circular(24),
                      ),
                      child: Column(
                        crossAxisAlignment: CrossAxisAlignment.start,
                        children: [
                          const Text(
                            '本月结余',
                            style: TextStyle(color: Colors.white70, fontSize: 16),
                          ),
                          const SizedBox(height: 8),
                          Text(
                            _formatCurrency(summary.balance),
                            style: const TextStyle(
                              color: Colors.white,
                              fontSize: 34,
                              fontWeight: FontWeight.w700,
                            ),
                          ),
                          const SizedBox(height: 16),
                          Row(
                            children: [
                              Expanded(
                                child: _SummaryChip(
                                  label: '收入',
                                  value: _formatCurrency(summary.income),
                                ),
                              ),
                              const SizedBox(width: 12),
                              Expanded(
                                child: _SummaryChip(
                                  label: '支出',
                                  value: _formatCurrency(summary.expense),
                                ),
                              ),
                            ],
                          ),
                        ],
                      ),
                    ),
                    const SizedBox(height: 20),
                    Row(
                      children: [
                        Expanded(
                          child: FilledButton(
                            onPressed: onCreateEntry,
                            child: const Text('记一笔'),
                          ),
                        ),
                        const SizedBox(width: 12),
                        Expanded(
                          child: OutlinedButton(
                            onPressed: onViewLedger,
                            child: const Text('查看账单'),
                          ),
                        ),
                      ],
                    ),
                    const SizedBox(height: 24),
                    Row(
                      mainAxisAlignment: MainAxisAlignment.spaceBetween,
                      children: [
                        const Text(
                          '最近账单',
                          style: TextStyle(fontSize: 20, fontWeight: FontWeight.w700),
                        ),
                        Text(
                          '${summary.entryCount} 条记录',
                          style: Theme.of(context).textTheme.bodySmall,
                        ),
                      ],
                    ),
                    const SizedBox(height: 12),
                    if (entries.isEmpty)
                      const _EmptyState()
                    else
                      ...entries.map(_EntryCard.new),
                  ],
                ),
        );
      },
    );
  }
}

class _SummaryChip extends StatelessWidget {
  const _SummaryChip({required this.label, required this.value});

  final String label;
  final String value;

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.all(14),
      decoration: BoxDecoration(
        color: Colors.white.withValues(alpha: 0.14),
        borderRadius: BorderRadius.circular(18),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text(label, style: const TextStyle(color: Colors.white70)),
          const SizedBox(height: 6),
          Text(
            value,
            style: const TextStyle(color: Colors.white, fontWeight: FontWeight.w600),
          ),
        ],
      ),
    );
  }
}

class _EmptyState extends StatelessWidget {
  const _EmptyState();

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.all(24),
      decoration: BoxDecoration(
        color: Colors.white,
        borderRadius: BorderRadius.circular(20),
      ),
      child: const Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text('还没有账单记录', style: TextStyle(fontSize: 18, fontWeight: FontWeight.w700)),
          SizedBox(height: 8),
          Text('先记下一笔支出或收入，首页概览会自动刷新。'),
        ],
      ),
    );
  }
}

class _EntryCard extends StatelessWidget {
  const _EntryCard(this.entry);

  final BookkeepingEntry entry;

  @override
  Widget build(BuildContext context) {
    return Container(
      margin: const EdgeInsets.only(bottom: 12),
      padding: const EdgeInsets.all(16),
      decoration: BoxDecoration(
        color: Colors.white,
        borderRadius: BorderRadius.circular(20),
      ),
      child: Row(
        children: [
          CircleAvatar(
            backgroundColor: entry.isExpense
                ? const Color(0xFFFFEEE7)
                : const Color(0xFFE7F7EF),
            child: Icon(
              entry.isExpense ? Icons.arrow_upward : Icons.arrow_downward,
              color: entry.isExpense ? const Color(0xFFB04B2D) : const Color(0xFF1F7A5C),
            ),
          ),
          const SizedBox(width: 12),
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text(entry.note.isEmpty ? entry.category : entry.note),
                const SizedBox(height: 4),
                Text(
                  '${entry.category} · ${entry.date.year}-${entry.date.month.toString().padLeft(2, '0')}-${entry.date.day.toString().padLeft(2, '0')}',
                  style: Theme.of(context).textTheme.bodySmall,
                ),
              ],
            ),
          ),
          Text(
            entry.amount.toStringAsFixed(2),
            style: TextStyle(
              fontWeight: FontWeight.w700,
              color: entry.isExpense ? const Color(0xFFB04B2D) : const Color(0xFF1F7A5C),
            ),
          ),
        ],
      ),
    );
  }
}

String _formatCurrency(double value) {
  return '¥${value.toStringAsFixed(2)}';
}