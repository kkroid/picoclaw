import 'package:hive_flutter/hive_flutter.dart';

import '../models/record.dart';

abstract class RecordRepository {
  Future<void> init();
  Future<List<AppRecord>> loadRecords();
  Future<void> saveRecords(List<AppRecord> records);

  Future<void> addRecord(AppRecord record) async {
    final records = await loadRecords();
    records.add(record);
    await saveRecords(records);
  }

  Future<void> updateRecord(AppRecord record) async {
    final records = await loadRecords();
    final index = records.indexWhere((item) => item.id == record.id);
    if (index < 0) {
      throw StateError('record not found');
    }
    records[index] = record;
    await saveRecords(records);
  }

  Future<void> deleteRecord(String recordID) async {
    final records = await loadRecords();
    records.removeWhere((item) => item.id == recordID);
    await saveRecords(records);
  }
}

class HiveRecordRepository extends RecordRepository {
  static const String _boxName = 'flutter_open_lite';
  static const String _recordsKey = 'records';

  Box<dynamic>? _box;

  @override
  Future<void> init() async {
    await Hive.initFlutter();
    _box = await Hive.openBox<dynamic>(_boxName);
  }

  Box<dynamic> get _safeBox {
    final box = _box;
    if (box == null) {
      throw StateError('repository not initialized');
    }
    return box;
  }

  @override
  Future<List<AppRecord>> loadRecords() async {
    final rawRecords = _safeBox.get(_recordsKey, defaultValue: <dynamic>[]) as List<dynamic>;
    return rawRecords
        .map((item) => AppRecord.fromMap(Map<String, dynamic>.from(item as Map)))
        .toList()
      ..sort((left, right) => right.updatedAt.compareTo(left.updatedAt));
  }

  @override
  Future<void> saveRecords(List<AppRecord> records) async {
    await _safeBox.put(
      _recordsKey,
      records.map((record) => record.toMap()).toList(),
    );
  }
}

class InMemoryRecordRepository extends RecordRepository {
  InMemoryRecordRepository({List<AppRecord>? seedRecords})
      : _records = List<AppRecord>.from(seedRecords ?? const []);

  final List<AppRecord> _records;

  @override
  Future<void> init() async {}

  @override
  Future<List<AppRecord>> loadRecords() async {
    final records = List<AppRecord>.from(_records);
    records.sort((left, right) => right.updatedAt.compareTo(left.updatedAt));
    return records;
  }

  @override
  Future<void> saveRecords(List<AppRecord> records) async {
    _records
      ..clear()
      ..addAll(records);
  }
}