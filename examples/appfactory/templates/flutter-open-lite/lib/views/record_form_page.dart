import 'package:flutter/material.dart';

import '../controllers/record_form_controller.dart';
import '../controllers/home_controller.dart';
import '../models/record.dart';
import '../template/open_lite_copy.dart';

class RecordFormPage extends StatefulWidget {
  const RecordFormPage({
    super.key,
    required this.homeController,
    this.initialRecord,
  });

  final HomeController homeController;
  final AppRecord? initialRecord;

  @override
  State<RecordFormPage> createState() => _RecordFormPageState();
}

class _RecordFormPageState extends State<RecordFormPage> {
  final GlobalKey<FormState> _formKey = GlobalKey<FormState>();
  late final RecordFormController _controller;

  @override
  void initState() {
    super.initState();
    _controller = RecordFormController(initialRecord: widget.initialRecord);
  }

  @override
  void dispose() {
    _controller.dispose();
    super.dispose();
  }

  Future<void> _pickDate() async {
    final pickedDate = await showDatePicker(
      context: context,
      initialDate: _controller.selectedDate,
      firstDate: DateTime(2020),
      lastDate: DateTime(2100),
    );
    if (pickedDate != null) {
      _controller.setDate(pickedDate);
    }
  }

  Future<void> _save() async {
    if (!_formKey.currentState!.validate()) {
      return;
    }
    try {
      final savedRecord = await _controller.submit(widget.homeController);
      if (!mounted) {
        return;
      }
      Navigator.of(context).pop(savedRecord);
    } on FormatException catch (error) {
      if (!mounted) {
        return;
      }
      ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(content: Text(error.message)),
      );
    }
  }

  @override
  Widget build(BuildContext context) {
    return AnimatedBuilder(
      animation: _controller,
      builder: (context, _) {
        return Scaffold(
          appBar: AppBar(
            title: Text(
              _controller.isEditing ? openLiteCopy.editPageTitle : openLiteCopy.createPageTitle,
            ),
          ),
          body: Form(
            key: _formKey,
            child: ListView(
              padding: const EdgeInsets.all(20),
              children: [
                TextFormField(
                  key: const Key('title-field'),
                  controller: _controller.titleController,
                  decoration: InputDecoration(
                    labelText: openLiteCopy.titleFieldLabel,
                  ),
                  validator: (value) {
                    if ((value ?? '').trim().isEmpty) {
                      return openLiteCopy.titleFieldRequiredError;
                    }
                    return null;
                  },
                ),
                const SizedBox(height: 16),
                DropdownButtonFormField<String>(
                  value: _controller.categoryController.text,
                  items: _controller.categories
                      .map(
                        (category) => DropdownMenuItem<String>(
                          value: category,
                          child: Text(category),
                        ),
                      )
                      .toList(),
                  decoration: InputDecoration(
                    labelText: openLiteCopy.categoryFieldLabel,
                  ),
                  onChanged: (value) {
                    if (value != null) {
                      _controller.setCategory(value);
                    }
                  },
                ),
                const SizedBox(height: 16),
                SegmentedButton<RecordStatus>(
                  segments: const [
                    ButtonSegment<RecordStatus>(
                      value: RecordStatus.inbox,
                      label: Text('待整理'),
                    ),
                    ButtonSegment<RecordStatus>(
                      value: RecordStatus.inProgress,
                      label: Text('进行中'),
                    ),
                    ButtonSegment<RecordStatus>(
                      value: RecordStatus.done,
                      label: Text('已完成'),
                    ),
                  ],
                  selected: <RecordStatus>{_controller.selectedStatus},
                  onSelectionChanged: (selection) {
                    _controller.setStatus(selection.first);
                  },
                ),
                const SizedBox(height: 16),
                ListTile(
                  contentPadding: const EdgeInsets.symmetric(horizontal: 12),
                  shape: RoundedRectangleBorder(
                    borderRadius: BorderRadius.circular(12),
                    side: BorderSide(color: Theme.of(context).dividerColor),
                  ),
                  title: Text(openLiteCopy.dateFieldLabel),
                  subtitle: Text(
                    '${_controller.selectedDate.year}-${_controller.selectedDate.month.toString().padLeft(2, '0')}-${_controller.selectedDate.day.toString().padLeft(2, '0')}',
                  ),
                  trailing: IconButton(
                    onPressed: _pickDate,
                    icon: const Icon(Icons.calendar_today),
                  ),
                ),
                const SizedBox(height: 16),
                TextFormField(
                  key: const Key('note-field'),
                  controller: _controller.noteController,
                  decoration: InputDecoration(
                    labelText: openLiteCopy.noteFieldLabel,
                    hintText: openLiteCopy.noteFieldHint,
                  ),
                  minLines: 2,
                  maxLines: 3,
                ),
                const SizedBox(height: 24),
                FilledButton(
                  onPressed: _save,
                  child: Text(
                    _controller.isEditing ? openLiteCopy.editSubmitLabel : openLiteCopy.createSubmitLabel,
                  ),
                ),
              ],
            ),
          ),
        );
      },
    );
  }
}