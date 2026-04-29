import 'package:flutter/material.dart';

import '../models/entry.dart';
import 'home_controller.dart';

class EntryFormController extends ChangeNotifier {
  EntryFormController()
      : amountController = TextEditingController(),
        noteController = TextEditingController(),
        categoryController = TextEditingController(text: _categories.first);

  static const List<String> _categories = <String>[
    '餐饮',
    '交通',
    '居家',
    '娱乐',
    '工资',
    '其他',
  ];

  final TextEditingController amountController;
  final TextEditingController noteController;
  final TextEditingController categoryController;

  EntryType _selectedType = EntryType.expense;
  DateTime _selectedDate = DateTime.now();

  List<String> get categories => _categories;
  EntryType get selectedType => _selectedType;
  DateTime get selectedDate => _selectedDate;

  void setType(EntryType type) {
    _selectedType = type;
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

  Future<void> submit(HomeController homeController) async {
    final amount = double.tryParse(amountController.text.trim());
    if (amount == null || amount <= 0) {
      throw const FormatException('请输入有效金额');
    }
    final category = categoryController.text.trim();
    if (category.isEmpty) {
      throw const FormatException('请选择分类');
    }

    await homeController.addEntry(
      BookkeepingEntry(
        id: DateTime.now().microsecondsSinceEpoch.toString(),
        amount: amount,
        category: category,
        date: DateTime(
          _selectedDate.year,
          _selectedDate.month,
          _selectedDate.day,
        ),
        note: noteController.text.trim(),
        type: _selectedType,
      ),
    );
  }

  @override
  void dispose() {
    amountController.dispose();
    noteController.dispose();
    categoryController.dispose();
    super.dispose();
  }
}