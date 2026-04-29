import 'package:flutter/material.dart';

import '../controllers/record_list_controller.dart';
import '../models/record.dart';
import '../template/open_lite_copy.dart';

class RecordListPage extends StatelessWidget {
  const RecordListPage({
    super.key,
    required this.controller,
    required this.onOpenRecordDetail,
  });

  final RecordListController controller;
  final Future<void> Function(AppRecord record) onOpenRecordDetail;

  @override
  Widget build(BuildContext context) {
    return AnimatedBuilder(
      animation: controller,
      builder: (context, _) {
        final records = controller.records;
        final visibleRecords = controller.visibleRecords;
        return Scaffold(
          appBar: AppBar(title: Text(openLiteCopy.listPageTitle)),
          body: records.isEmpty
              ? Center(child: Text(openLiteCopy.listEmptyLabel))
              : ListView(
                  padding: const EdgeInsets.all(20),
                  children: [
                    Text(
                      openLiteCopy.listFilterTitle,
                      style: TextStyle(fontSize: 18, fontWeight: FontWeight.w700),
                    ),
                    const SizedBox(height: 12),
                    Wrap(
                      spacing: 8,
                      runSpacing: 8,
                      children: [
                        _FilterChip(
                          filter: RecordListFilter.all,
                          controller: controller,
                          label: openLiteCopy.allFilterLabel,
                        ),
                        _FilterChip(
                          filter: RecordListFilter.inbox,
                          controller: controller,
                          label: openLiteCopy.inboxFilterLabel,
                        ),
                        _FilterChip(
                          filter: RecordListFilter.inProgress,
                          controller: controller,
                          label: openLiteCopy.inProgressFilterLabel,
                        ),
                        _FilterChip(
                          filter: RecordListFilter.done,
                          controller: controller,
                          label: openLiteCopy.doneFilterLabel,
                        ),
                      ],
                    ),
                    const SizedBox(height: 16),
                    Text(
                      openLiteCopy.listCountLabel(visibleRecords.length, records.length),
                      style: Theme.of(context).textTheme.bodyMedium,
                    ),
                    const SizedBox(height: 16),
                    if (visibleRecords.isEmpty)
                      _FilteredEmptyState(
                        onClearFilter: () => controller.setFilter(RecordListFilter.all),
                      )
                    else
                      ...List<Widget>.generate(visibleRecords.length, (index) {
                        final record = visibleRecords[index];
                        return Padding(
                          padding: EdgeInsets.only(bottom: index == visibleRecords.length - 1 ? 0 : 12),
                          child: _RecordRow(
                            record: record,
                            onTap: () => onOpenRecordDetail(record),
                          ),
                        );
                      }),
                  ],
                ),
        );
      },
    );
  }
}

class _FilterChip extends StatelessWidget {
  const _FilterChip({
    required this.filter,
    required this.controller,
    required this.label,
  });

  final RecordListFilter filter;
  final RecordListController controller;
  final String label;

  @override
  Widget build(BuildContext context) {
    return ChoiceChip(
      key: Key('record-filter-${filter.name}'),
      label: Text(label),
      selected: controller.selectedFilter == filter,
      onSelected: (_) => controller.setFilter(filter),
    );
  }
}

class _FilteredEmptyState extends StatelessWidget {
  const _FilteredEmptyState({required this.onClearFilter});

  final VoidCallback onClearFilter;

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.all(20),
      decoration: BoxDecoration(
        color: Colors.white,
        borderRadius: BorderRadius.circular(18),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text(
            openLiteCopy.filteredEmptyTitle,
            style: TextStyle(fontSize: 18, fontWeight: FontWeight.w700),
          ),
          const SizedBox(height: 8),
          Text(openLiteCopy.filteredEmptyDescription),
          const SizedBox(height: 16),
          OutlinedButton(
            onPressed: onClearFilter,
            child: Text(openLiteCopy.clearFilterActionLabel),
          ),
        ],
      ),
    );
  }
}

class _RecordRow extends StatelessWidget {
  const _RecordRow({required this.record, required this.onTap});

  final AppRecord record;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    return Material(
      color: Colors.white,
      borderRadius: BorderRadius.circular(18),
      child: InkWell(
        onTap: onTap,
        borderRadius: BorderRadius.circular(18),
        child: Padding(
          padding: const EdgeInsets.all(16),
          child: Row(
            children: [
              Expanded(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Text(
                      record.title,
                      style: const TextStyle(fontWeight: FontWeight.w700),
                    ),
                    const SizedBox(height: 4),
                    Text(
                      '${record.category} · ${openLiteCopy.statusLabel(record.status)}',
                    ),
                  ],
                ),
              ),
              Text(
                '${record.updatedAt.year}-${record.updatedAt.month.toString().padLeft(2, '0')}-${record.updatedAt.day.toString().padLeft(2, '0')}',
                style: const TextStyle(fontWeight: FontWeight.w600),
              ),
            ],
          ),
        ),
      ),
    );
  }
}
