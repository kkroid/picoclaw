import 'package:flutter/foundation.dart' show ChangeNotifier;

import '../models/dashboard_summary.dart';
import '../models/record.dart';
import '../repositories/record_repository.dart';

class HomeController extends ChangeNotifier {
  HomeController({required RecordRepository repository}) : _repository = repository;

  final RecordRepository _repository;

  bool _isLoading = true;
  List<AppRecord> _records = const [];

  bool get isLoading => _isLoading;
  List<AppRecord> get records => List<AppRecord>.unmodifiable(_records);
  DashboardSummary get summary => DashboardSummary.fromRecords(_records);

  Future<void> initialize() async {
    await refresh();
  }

  Future<void> refresh() async {
    _isLoading = true;
    notifyListeners();
    _records = await _repository.loadRecords();
    _isLoading = false;
    notifyListeners();
  }

  Future<void> addRecord(AppRecord record) async {
    await _repository.addRecord(record);
    await refresh();
  }

  Future<void> updateRecord(AppRecord record) async {
    await _repository.updateRecord(record);
    await refresh();
  }

  Future<void> deleteRecord(String recordID) async {
    await _repository.deleteRecord(recordID);
    await refresh();
  }
}