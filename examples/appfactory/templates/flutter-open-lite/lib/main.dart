import 'package:flutter/material.dart';

import 'controllers/record_list_controller.dart';
import 'controllers/home_controller.dart';
import 'models/record.dart';
import 'repositories/record_repository.dart';
import 'template/open_lite_copy.dart';
import 'views/record_detail_page.dart';
import 'views/record_form_page.dart';
import 'views/record_list_page.dart';
import 'views/home_page.dart';

Future<void> main() async {
  WidgetsFlutterBinding.ensureInitialized();
  final repository = HiveRecordRepository();
  await repository.init();
  runApp(FlutterOpenLiteApp(repository: repository));
}

class FlutterOpenLiteApp extends StatelessWidget {
  const FlutterOpenLiteApp({super.key, required this.repository});

  final RecordRepository repository;

  @override
  Widget build(BuildContext context) {
    return MaterialApp(
      title: openLiteCopy.appTitle,
      theme: ThemeData(
        colorScheme: ColorScheme.fromSeed(
          seedColor: const Color(0xFF1565C0),
          brightness: Brightness.light,
        ),
        scaffoldBackgroundColor: const Color(0xFFF4F7FB),
        inputDecorationTheme: const InputDecorationTheme(
          border: OutlineInputBorder(),
        ),
        useMaterial3: true,
      ),
      home: FlutterOpenLiteShell(repository: repository),
    );
  }
}

class FlutterOpenLiteShell extends StatefulWidget {
  const FlutterOpenLiteShell({super.key, required this.repository});

  final RecordRepository repository;

  @override
  State<FlutterOpenLiteShell> createState() => _FlutterOpenLiteShellState();
}

class _FlutterOpenLiteShellState extends State<FlutterOpenLiteShell> {
  late final HomeController _homeController;
  late final RecordListController _recordListController;
  late final Future<void> _initialization;

  @override
  void initState() {
    super.initState();
    _homeController = HomeController(repository: widget.repository);
    _recordListController = RecordListController(homeController: _homeController);
    _initialization = _homeController.initialize();
  }

  @override
  void dispose() {
    _recordListController.dispose();
    _homeController.dispose();
    super.dispose();
  }

  Future<void> _openRecordForm() async {
    await Navigator.of(context).push<void>(
      MaterialPageRoute(
        builder: (_) => RecordFormPage(homeController: _homeController),
      ),
    );
    if (mounted) {
      setState(() {});
    }
  }

  Future<AppRecord?> _editRecord(AppRecord record) async {
    final updatedRecord = await Navigator.of(context).push<AppRecord>(
      MaterialPageRoute(
        builder: (_) => RecordFormPage(
          homeController: _homeController,
          initialRecord: record,
        ),
      ),
    );
    if (mounted) {
      setState(() {});
    }
    return updatedRecord;
  }

  Future<void> _deleteRecord(AppRecord record) async {
    await _homeController.deleteRecord(record.id);
    if (mounted) {
      setState(() {});
    }
  }

  Future<void> _openRecordList() async {
    await Navigator.of(context).push<void>(
      MaterialPageRoute(
        builder: (_) => RecordListPage(
          controller: _recordListController,
          onOpenRecordDetail: _openRecordDetail,
        ),
      ),
    );
  }

  Future<void> _openRecordDetail(AppRecord record) async {
    await Navigator.of(context).push<void>(
      MaterialPageRoute(
        builder: (_) => RecordDetailPage(
          record: record,
          onEdit: _editRecord,
          onDelete: _deleteRecord,
        ),
      ),
    );
    if (mounted) {
      setState(() {});
    }
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
          recordListController: _recordListController,
          onCreateRecord: _openRecordForm,
          onViewAllRecords: _openRecordList,
          onOpenRecordDetail: _openRecordDetail,
        );
      },
    );
  }
}
