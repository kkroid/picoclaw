import 'package:flutter/material.dart';

import '../controllers/record_list_controller.dart';
import '../controllers/home_controller.dart';
import '../models/record.dart';
import '../template/open_lite_copy.dart';

class HomePage extends StatelessWidget {
  const HomePage({
    super.key,
    required this.controller,
    required this.recordListController,
    required this.onCreateRecord,
    required this.onViewAllRecords,
    required this.onOpenRecordDetail,
  });

  final HomeController controller;
  final RecordListController recordListController;
  final Future<void> Function() onCreateRecord;
  final Future<void> Function() onViewAllRecords;
  final Future<void> Function(AppRecord record) onOpenRecordDetail;

  @override
  Widget build(BuildContext context) {
    return AnimatedBuilder(
      animation: controller,
      builder: (context, _) {
        final summary = controller.summary;
        final recentRecords = recordListController.records.take(3).toList();
        return Scaffold(
          appBar: AppBar(
            title: Text(openLiteCopy.appTitle),
          ),
          body: controller.isLoading
              ? const Center(child: CircularProgressIndicator())
              : ListView(
                  padding: const EdgeInsets.all(20),
                  children: [
                    Container(
                      padding: const EdgeInsets.all(20),
                      decoration: BoxDecoration(
                        color: const Color(0xFF1565C0),
                        borderRadius: BorderRadius.circular(24),
                      ),
                      child: Column(
                        crossAxisAlignment: CrossAxisAlignment.start,
                        children: [
                          Text(
                            openLiteCopy.homeSummaryTitle,
                            style: TextStyle(color: Colors.white70, fontSize: 16),
                          ),
                          const SizedBox(height: 8),
                          Text(
                            openLiteCopy.summaryCountLabel(summary.totalCount),
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
                                  label: openLiteCopy.inboxFilterLabel,
                                  value: '${summary.inboxCount}',
                                ),
                              ),
                              const SizedBox(width: 12),
                              Expanded(
                                child: _SummaryChip(
                                  label: openLiteCopy.inProgressFilterLabel,
                                  value: '${summary.inProgressCount}',
                                ),
                              ),
                              const SizedBox(width: 12),
                              Expanded(
                                child: _SummaryChip(
                                  label: openLiteCopy.doneFilterLabel,
                                  value: '${summary.doneCount}',
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
                            onPressed: onCreateRecord,
                            child: Text(openLiteCopy.createPrimaryActionLabel),
                          ),
                        ),
                        const SizedBox(width: 12),
                        Expanded(
                          child: OutlinedButton(
                            onPressed: onViewAllRecords,
                            child: Text(openLiteCopy.viewAllActionLabel),
                          ),
                        ),
                      ],
                    ),
                    const SizedBox(height: 24),
                    Row(
                      mainAxisAlignment: MainAxisAlignment.spaceBetween,
                      children: [
                        Text(
                          openLiteCopy.recentRecordsTitle,
                          style: TextStyle(fontSize: 20, fontWeight: FontWeight.w700),
                        ),
                        Text(
                          openLiteCopy.recentRecordsCountLabel(summary.totalCount),
                          style: Theme.of(context).textTheme.bodySmall,
                        ),
                      ],
                    ),
                    const SizedBox(height: 12),
                    if (recentRecords.isEmpty)
                      const _EmptyState()
                    else
                      ...recentRecords.map(
                        (record) => _RecordCard(
                          record: record,
                          onTap: () => onOpenRecordDetail(record),
                        ),
                      ),
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
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text(openLiteCopy.homeEmptyTitle, style: TextStyle(fontSize: 18, fontWeight: FontWeight.w700)),
          SizedBox(height: 8),
          Text(openLiteCopy.homeEmptyDescription),
        ],
      ),
    );
  }
}

class _RecordCard extends StatelessWidget {
  const _RecordCard({required this.record, required this.onTap});

  final AppRecord record;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    return InkWell(
      onTap: onTap,
      borderRadius: BorderRadius.circular(20),
      child: Container(
        margin: const EdgeInsets.only(bottom: 12),
        padding: const EdgeInsets.all(16),
        decoration: BoxDecoration(
          color: Colors.white,
          borderRadius: BorderRadius.circular(20),
        ),
        child: Row(
          children: [
            CircleAvatar(
              backgroundColor: _statusTone(record.status).withValues(alpha: 0.12),
              child: Icon(
                _statusIcon(record.status),
                color: _statusTone(record.status),
              ),
            ),
            const SizedBox(width: 12),
            Expanded(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text(record.title, style: const TextStyle(fontWeight: FontWeight.w700)),
                  const SizedBox(height: 4),
                  Text(
                    '${record.category} · ${openLiteCopy.statusLabel(record.status)}',
                    style: Theme.of(context).textTheme.bodySmall,
                  ),
                ],
              ),
            ),
            Text(
              '${record.updatedAt.month.toString().padLeft(2, '0')}-${record.updatedAt.day.toString().padLeft(2, '0')}',
              style: const TextStyle(fontWeight: FontWeight.w600),
            ),
          ],
        ),
      ),
    );
  }
}

IconData _statusIcon(RecordStatus status) {
  switch (status) {
    case RecordStatus.inbox:
      return Icons.inbox_outlined;
    case RecordStatus.inProgress:
      return Icons.play_circle_outline;
    case RecordStatus.done:
      return Icons.check_circle_outline;
  }
}

Color _statusTone(RecordStatus status) {
  switch (status) {
    case RecordStatus.inbox:
      return const Color(0xFF5C6BC0);
    case RecordStatus.inProgress:
      return const Color(0xFFEF6C00);
    case RecordStatus.done:
      return const Color(0xFF2E7D32);
  }
}