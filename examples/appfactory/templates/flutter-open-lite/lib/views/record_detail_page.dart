import 'package:flutter/material.dart';

import '../models/record.dart';
import '../template/open_lite_copy.dart';

class RecordDetailPage extends StatefulWidget {
  const RecordDetailPage({
    super.key,
    required this.record,
    required this.onEdit,
    required this.onDelete,
  });

  final AppRecord record;
  final Future<AppRecord?> Function(AppRecord record) onEdit;
  final Future<void> Function(AppRecord record) onDelete;

  @override
  State<RecordDetailPage> createState() => _RecordDetailPageState();
}

class _RecordDetailPageState extends State<RecordDetailPage> {
  late AppRecord _record;

  @override
  void initState() {
    super.initState();
    _record = widget.record;
  }

  Future<void> _editRecord() async {
    final updatedRecord = await widget.onEdit(_record);
    if (updatedRecord == null || !mounted) {
      return;
    }
    setState(() {
      _record = updatedRecord;
    });
  }

  Future<void> _deleteRecord() async {
    final confirmed = await showDialog<bool>(
      context: context,
      builder: (context) {
        return AlertDialog(
          title: Text(openLiteCopy.deleteDialogTitle),
          content: Text(openLiteCopy.deleteDialogMessage),
          actions: [
            TextButton(
              onPressed: () => Navigator.of(context).pop(false),
              child: const Text('取消'),
            ),
            FilledButton(
              onPressed: () => Navigator.of(context).pop(true),
              child: Text(openLiteCopy.deleteActionLabel),
            ),
          ],
        );
      },
    );
    if (confirmed != true) {
      return;
    }
    await widget.onDelete(_record);
    if (!mounted) {
      return;
    }
    Navigator.of(context).pop();
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(
        title: Text(openLiteCopy.detailPageTitle),
        actions: [
          TextButton(
            onPressed: _editRecord,
            child: Text(openLiteCopy.editActionLabel),
          ),
          TextButton(
            onPressed: _deleteRecord,
            child: Text(openLiteCopy.deleteActionLabel),
          ),
        ],
      ),
      body: ListView(
        padding: const EdgeInsets.all(20),
        children: [
          _DetailCard(
            title: _record.title,
            subtitle: '${_record.category} · ${openLiteCopy.statusLabel(_record.status)}',
          ),
          const SizedBox(height: 16),
          _InfoTile(label: openLiteCopy.detailCategoryLabel, value: _record.category),
          _InfoTile(label: openLiteCopy.detailStatusLabel, value: openLiteCopy.statusLabel(_record.status)),
          _InfoTile(
            label: openLiteCopy.detailDateLabel,
            value:
                '${_record.updatedAt.year}-${_record.updatedAt.month.toString().padLeft(2, '0')}-${_record.updatedAt.day.toString().padLeft(2, '0')}',
          ),
          _InfoTile(
            label: openLiteCopy.detailNoteLabel,
            value: _record.note.isEmpty ? openLiteCopy.emptyNoteLabel : _record.note,
            multiline: true,
          ),
        ],
      ),
    );
  }
}

class _DetailCard extends StatelessWidget {
  const _DetailCard({required this.title, required this.subtitle});

  final String title;
  final String subtitle;

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.all(20),
      decoration: BoxDecoration(
        borderRadius: BorderRadius.circular(24),
        color: const Color(0xFF1565C0),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text(
            title,
            style: const TextStyle(
              color: Colors.white,
              fontSize: 24,
              fontWeight: FontWeight.w700,
            ),
          ),
          const SizedBox(height: 8),
          Text(
            subtitle,
            style: const TextStyle(color: Colors.white70),
          ),
        ],
      ),
    );
  }
}

class _InfoTile extends StatelessWidget {
  const _InfoTile({required this.label, required this.value, this.multiline = false});

  final String label;
  final String value;
  final bool multiline;

  @override
  Widget build(BuildContext context) {
    return Container(
      margin: const EdgeInsets.only(bottom: 12),
      padding: const EdgeInsets.all(16),
      decoration: BoxDecoration(
        color: Colors.white,
        borderRadius: BorderRadius.circular(18),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text(label, style: Theme.of(context).textTheme.labelMedium),
          const SizedBox(height: 6),
          Text(
            value,
            style: TextStyle(height: multiline ? 1.4 : 1.2),
          ),
        ],
      ),
    );
  }
}
