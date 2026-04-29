import '../models/entry.dart';
import 'home_controller.dart';

class EntryListController {
  const EntryListController({required HomeController homeController})
      : _homeController = homeController;

  final HomeController _homeController;

  List<BookkeepingEntry> get entries => _homeController.entries;

  void dispose() {}
}