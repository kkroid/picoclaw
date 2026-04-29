import 'package:flutter/material.dart';

import '../models/record.dart';
import 'home_controller.dart';

class RecordFormController extends ChangeNotifier {
  RecordFormController({AppRecord? initialRecord})
      : titleController = TextEditingController(),
        noteController = TextEditingController(),
        categoryController = TextEditingController(text: _categories.first),
        _initialRecord = initialRecord {
    if (initialRecord != null) {
      titleController.text = initialRecord.title;
      noteController.text = initialRecord.note;
      categoryController.text = initialRecord.category;
      _selectedStatus = initialRecord.status;
      _selectedDate = initialRecord.updatedAt;
    }
  }

  static const List<String> _categories = <String>[
    '例行事项',
    '健康记录',
    '库存事项',
    '灵感备忘',
    '其他',
  ];

  final TextEditingController titleController;
  final TextEditingController noteController;
  final TextEditingController categoryController;
  final AppRecord? _initialRecord;

  RecordStatus _selectedStatus = RecordStatus.inbox;
  DateTime _selectedDate = DateTime.now();

  List<String> get categories => _categories;
  bool get isEditing => _initialRecord != null;
  RecordStatus get selectedStatus => _selectedStatus;
  DateTime get selectedDate => _selectedDate;

  void setStatus(RecordStatus status) {
    _selectedStatus = status;
    notifyListeners();
  }

  void setDate(DateTime date) {
    _selectedDate = date;
    notifyListeners();
  }

  void setCategory(String category) {
    categoryController.text = category;
    notifyListeners();
  }

  Future<AppRecord> submit(HomeController homeController) async {
    final title = titleController.text.trim();
    if (title.isEmpty) {
      throw const FormatException('请输入记录标题');
    }
    final category = categoryController.text.trim();
    if (category.isEmpty) {
      throw const FormatException('请选择分类');
    }

    final normalizedDate = DateTime(
      _selectedDate.year,
      _selectedDate.month,
      _selectedDate.day,
    );

    final existingRecord = _initialRecord;
    if (existingRecord != null) {
      final updatedRecord = existingRecord.copyWith(
        title: title,
        category: category,
        updatedAt: normalizedDate,
        note: noteController.text.trim(),
        status: _selectedStatus,
      );
      await homeController.updateRecord(updatedRecord);
      return updatedRecord;
    }

    final createdRecord = AppRecord(
      id: DateTime.now().microsecondsSinceEpoch.toString(),
      title: title,
      category: category,
      updatedAt: normalizedDate,
      note: noteController.text.trim(),
      status: _selectedStatus,
    );
    await homeController.addRecord(createdRecord);
    return createdRecord;
  }

  @override
  void dispose() {
    titleController.dispose();
    noteController.dispose();
    categoryController.dispose();
    super.dispose();
  }
}