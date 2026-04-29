import 'package:flutter/foundation.dart' show ChangeNotifier;

import '../models/record.dart';
import 'home_controller.dart';

enum RecordListFilter {
  all,
  inbox,
  inProgress,
  done,
}

class RecordListController extends ChangeNotifier {
  RecordListController({required HomeController homeController})
      : _homeController = homeController {
    _homeController.addListener(_forwardChange);
  }

  final HomeController _homeController;
  RecordListFilter _selectedFilter = RecordListFilter.all;

  List<AppRecord> get records => _homeController.records;
  RecordListFilter get selectedFilter => _selectedFilter;
  List<AppRecord> get visibleRecords {
    switch (_selectedFilter) {
      case RecordListFilter.all:
        return records;
      case RecordListFilter.inbox:
        return records.where((record) => record.status == RecordStatus.inbox).toList();
      case RecordListFilter.inProgress:
        return records.where((record) => record.status == RecordStatus.inProgress).toList();
      case RecordListFilter.done:
        return records.where((record) => record.status == RecordStatus.done).toList();
    }
  }

  bool get hasActiveFilter => _selectedFilter != RecordListFilter.all;

  void setFilter(RecordListFilter filter) {
    if (_selectedFilter == filter) {
      return;
    }
    _selectedFilter = filter;
    notifyListeners();
  }

  void _forwardChange() {
    notifyListeners();
  }

  @override
  void dispose() {
    _homeController.removeListener(_forwardChange);
    super.dispose();
  }
}