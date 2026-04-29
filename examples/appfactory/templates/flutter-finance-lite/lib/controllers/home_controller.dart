import 'package:flutter/foundation.dart' show ChangeNotifier;

import '../models/entry.dart';
import '../models/summary.dart' as bookkeeping;
import '../repositories/entry_repository.dart';

class HomeController extends ChangeNotifier {
  HomeController({required EntryRepository repository}) : _repository = repository;

  final EntryRepository _repository;

  bool _isLoading = true;
  List<BookkeepingEntry> _entries = const [];

  bool get isLoading => _isLoading;
  List<BookkeepingEntry> get entries => List<BookkeepingEntry>.unmodifiable(_entries);
  bookkeeping.Summary get summary => bookkeeping.Summary.fromEntries(_entries);

  Future<void> initialize() async {
    await refresh();
  }

  Future<void> refresh() async {
    _isLoading = true;
    notifyListeners();
    _entries = await _repository.loadEntries();
    _isLoading = false;
    notifyListeners();
  }

  Future<void> addEntry(BookkeepingEntry entry) async {
    await _repository.addEntry(entry);
    await refresh();
  }
}