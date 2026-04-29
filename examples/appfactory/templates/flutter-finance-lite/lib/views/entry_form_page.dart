import 'package:flutter/material.dart';

import '../controllers/entry_form_controller.dart';
import '../controllers/home_controller.dart';
import '../models/entry.dart';

class EntryFormPage extends StatefulWidget {
  const EntryFormPage({super.key, required this.homeController});

  final HomeController homeController;

  @override
  State<EntryFormPage> createState() => _EntryFormPageState();
}

class _EntryFormPageState extends State<EntryFormPage> {
  final GlobalKey<FormState> _formKey = GlobalKey<FormState>();
  late final EntryFormController _controller;

  @override
  void initState() {
    super.initState();
    _controller = EntryFormController();
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
      await _controller.submit(widget.homeController);
      if (!mounted) {
        return;
      }
      Navigator.of(context).pop();
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
          appBar: AppBar(title: const Text('记一笔')),
          body: Form(
            key: _formKey,
            child: ListView(
              padding: const EdgeInsets.all(20),
              children: [
                SegmentedButton<EntryType>(
                  segments: const [
                    ButtonSegment<EntryType>(
                      value: EntryType.expense,
                      label: Text('支出'),
                    ),
                    ButtonSegment<EntryType>(
                      value: EntryType.income,
                      label: Text('收入'),
                    ),
                  ],
                  selected: <EntryType>{_controller.selectedType},
                  onSelectionChanged: (selection) {
                    _controller.setType(selection.first);
                  },
                ),
                const SizedBox(height: 16),
                TextFormField(
                  key: const Key('amount-field'),
                  controller: _controller.amountController,
                  keyboardType: const TextInputType.numberWithOptions(decimal: true),
                  decoration: const InputDecoration(
                    labelText: '金额',
                    prefixText: '¥ ',
                    border: OutlineInputBorder(),
                  ),
                  validator: (value) {
                    final amount = double.tryParse((value ?? '').trim());
                    if (amount == null || amount <= 0) {
                      return '请输入有效金额';
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
                  decoration: const InputDecoration(
                    labelText: '分类',
                    border: OutlineInputBorder(),
                  ),
                  onChanged: (value) {
                    if (value != null) {
                      _controller.setCategory(value);
                    }
                  },
                ),
                const SizedBox(height: 16),
                ListTile(
                  contentPadding: const EdgeInsets.symmetric(horizontal: 12),
                  shape: RoundedRectangleBorder(
                    borderRadius: BorderRadius.circular(12),
                    side: BorderSide(color: Theme.of(context).dividerColor),
                  ),
                  title: const Text('记账日期'),
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
                  decoration: const InputDecoration(
                    labelText: '备注',
                    hintText: '例如：早餐、地铁、工资到账',
                    border: OutlineInputBorder(),
                  ),
                  minLines: 2,
                  maxLines: 3,
                ),
                const SizedBox(height: 24),
                FilledButton(
                  onPressed: _save,
                  child: const Text('保存'),
                ),
              ],
            ),
          ),
        );
      },
    );
  }
}