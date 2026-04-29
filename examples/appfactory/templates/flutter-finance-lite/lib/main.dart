import 'package:flutter/material.dart';

import 'controllers/entry_list_controller.dart';
import 'controllers/home_controller.dart';
import 'repositories/entry_repository.dart';
import 'views/entry_form_page.dart';
import 'views/entry_list_page.dart';
import 'views/home_page.dart';

Future<void> main() async {
  WidgetsFlutterBinding.ensureInitialized();
  final repository = HiveEntryRepository();
  await repository.init();
  runApp(BookkeepingApp(repository: repository));
}

class BookkeepingApp extends StatelessWidget {
  const BookkeepingApp({super.key, required this.repository});

  final EntryRepository repository;

  @override
  Widget build(BuildContext context) {
    return MaterialApp(
      title: '记账助手',
      theme: ThemeData(
        colorScheme: ColorScheme.fromSeed(
          seedColor: const Color(0xFF1F7A5C),
          brightness: Brightness.light,
        ),
        scaffoldBackgroundColor: const Color(0xFFF5F7F2),
        useMaterial3: true,
      ),
      home: BookkeepingShell(repository: repository),
    );
  }
}

class BookkeepingShell extends StatefulWidget {
  const BookkeepingShell({super.key, required this.repository});

  final EntryRepository repository;

  @override
  State<BookkeepingShell> createState() => _BookkeepingShellState();
}

class _BookkeepingShellState extends State<BookkeepingShell> {
  late final HomeController _homeController;
  late final EntryListController _entryListController;
  late final Future<void> _initialization;

  @override
  void initState() {
    super.initState();
    _homeController = HomeController(repository: widget.repository);
    _entryListController = EntryListController(homeController: _homeController);
    _initialization = _homeController.initialize();
  }

  @override
  void dispose() {
    _entryListController.dispose();
    _homeController.dispose();
    super.dispose();
  }

  Future<void> _openEntryForm() async {
    await Navigator.of(context).push<void>(
      MaterialPageRoute(
        builder: (_) => EntryFormPage(homeController: _homeController),
      ),
    );
    if (mounted) {
      setState(() {});
    }
  }

  Future<void> _openEntryList() async {
    await Navigator.of(context).push<void>(
      MaterialPageRoute(
        builder: (_) => EntryListPage(controller: _entryListController),
      ),
    );
  }

  @override
  Widget build(BuildContext context) {
    return FutureBuilder<void>(
      future: _initialization,
      builder: (context, snapshot) {
        if (snapshot.connectionState != ConnectionState.done) {
          return const Scaffold(
            body: Center(child: CircularProgressIndicator()),
          );
        }

        return HomePage(
          controller: _homeController,
          entryListController: _entryListController,
          onCreateEntry: _openEntryForm,
          onViewLedger: _openEntryList,
        );
      },
    );
  }
}
