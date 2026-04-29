import 'package:hive_flutter/hive_flutter.dart';

import '../models/entry.dart';

abstract class EntryRepository {
  Future<void> init();
  Future<List<BookkeepingEntry>> loadEntries();
  Future<void> saveEntries(List<BookkeepingEntry> entries);

  Future<void> addEntry(BookkeepingEntry entry) async {
    final entries = await loadEntries();
    entries.add(entry);
    await saveEntries(entries);
  }
}

class HiveEntryRepository extends EntryRepository {
  static const String _boxName = 'bookkeeping_lite';
  static const String _entriesKey = 'entries';

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
  Future<List<BookkeepingEntry>> loadEntries() async {
    final rawEntries = _safeBox.get(_entriesKey, defaultValue: <dynamic>[]) as List<dynamic>;
    return rawEntries
        .map((item) => BookkeepingEntry.fromMap(Map<String, dynamic>.from(item as Map)))
        .toList()
      ..sort((left, right) => right.date.compareTo(left.date));
  }

  @override
  Future<void> saveEntries(List<BookkeepingEntry> entries) async {
    await _safeBox.put(
      _entriesKey,
      entries.map((entry) => entry.toMap()).toList(),
    );
  }
}

class InMemoryEntryRepository extends EntryRepository {
  InMemoryEntryRepository({List<BookkeepingEntry>? seedEntries})
      : _entries = List<BookkeepingEntry>.from(seedEntries ?? const []);

  final List<BookkeepingEntry> _entries;

  @override
  Future<void> init() async {}

  @override
  Future<List<BookkeepingEntry>> loadEntries() async {
    final entries = List<BookkeepingEntry>.from(_entries);
    entries.sort((left, right) => right.date.compareTo(left.date));
    return entries;
  }

  @override
  Future<void> saveEntries(List<BookkeepingEntry> entries) async {
    _entries
      ..clear()
      ..addAll(entries);
  }
}