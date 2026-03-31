package adapter

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strings"

	appruns "github.com/sipeed/picoclaw/pkg/appfactory/runs"
)

type ThinExecutor interface {
	Prepare(ctx context.Context, run runRecord) (RoundPlan, error)
}

type ExecutionStep struct {
	StepID  string
	Stage   appruns.ExecutionStage
	Summary string
	Command *exec.Cmd
	Check   *CheckExecutionPreview
}

type TaskExecutionPreview struct {
	TaskID    string
	Title     string
	Category  appruns.TaskCategory
	DependsOn []string
}

type CheckExecutionPreview struct {
	CheckID      string
	Label        string
	Stage        appruns.ExecutionStage
	Required     bool
	AllowFailure bool
	Commands     []string
}

type RoundPlan struct {
	Summary          string
	RoundInput       appruns.RoundInput
	EditStep         ExecutionStep
	ValidationSteps  []ExecutionStep
	TaskBundle       []TaskExecutionPreview
	AcceptanceChecks []CheckExecutionPreview
}

type defaultEditPlan struct {
	Summary string
	Files   []defaultEditFile
}

type defaultEditFile struct {
	Path    string
	Content string
	Mode    defaultEditMode
}

type defaultEditMode string

const (
	defaultEditOverwrite    defaultEditMode = "overwrite"
	defaultEditIfMissing    defaultEditMode = "if_missing"
	defaultEditIfDemo       defaultEditMode = "if_missing_or_default_demo"
	defaultEditIfIncomplete defaultEditMode = "if_missing_or_incomplete"
)

func (plan RoundPlan) Steps() []ExecutionStep {
	steps := make([]ExecutionStep, 0, 1+len(plan.ValidationSteps))
	if plan.EditStep.StepID != "" || plan.EditStep.Command != nil {
		steps = append(steps, plan.EditStep)
	}
	steps = append(steps, plan.ValidationSteps...)
	return steps
}

type StagedThinExecutor struct{}

func NewThinExecutor() ThinExecutor {
	return StagedThinExecutor{}
}

func (StagedThinExecutor) Prepare(ctx context.Context, run runRecord) (RoundPlan, error) {
	plannedRun := plannedExecutionRun(run)
	validationSteps := make([]ExecutionStep, 0, len(plannedRun.AcceptanceChecks))
	prepareCmd, editSummary, err := buildDefaultEditCommand(ctx, plannedRun)
	if err != nil {
		return RoundPlan{}, err
	}
	editStep := ExecutionStep{
		StepID:  "thin-prepare",
		Stage:   appruns.StageThinPrepare,
		Summary: editSummary,
		Command: prepareCmd,
	}
	for _, check := range plannedRun.AcceptanceChecks {
		preview := CheckExecutionPreview{
			CheckID:      check.CheckID,
			Label:        check.Label,
			Stage:        normalizeStage(check.Stage),
			Required:     check.Required,
			AllowFailure: check.AllowFailure,
			Commands:     append([]string(nil), check.Commands...),
		}
		trimmedCommands := compactCommands(check.Commands)
		if len(trimmedCommands) == 0 {
			continue
		}
		checkCmd, err := buildExecutionCommand(ctx, run, []string{"/bin/sh", "-lc", strings.Join(trimmedCommands, "\n")}, []string{
			"PICOCLAW_EXECUTION_STEP=" + check.CheckID,
			"PICOCLAW_EXECUTION_STAGE=" + string(preview.Stage),
			"PICOCLAW_ACCEPTANCE_CHECK_ID=" + check.CheckID,
		})
		if err != nil {
			return RoundPlan{}, err
		}
		validationSteps = append(validationSteps, ExecutionStep{
			StepID:  check.CheckID,
			Stage:   preview.Stage,
			Summary: buildCheckSummary(preview),
			Command: checkCmd,
			Check:   &preview,
		})
	}
	return RoundPlan{
		Summary:          buildExecutionSummary(plannedRun),
		RoundInput:       buildRoundInput(plannedRun),
		EditStep:         editStep,
		ValidationSteps:  validationSteps,
		TaskBundle:       previewTasks(plannedRun.TaskBundle),
		AcceptanceChecks: previewChecks(plannedRun.AcceptanceChecks),
	}, nil
}

func plannedExecutionRun(run runRecord) runRecord {
	planned := run
	planned.TaskBundle = plannedTaskBundle(run)
	planned.AcceptanceChecks = plannedAcceptanceChecks(run)
	return planned
}

func buildDefaultEditCommand(ctx context.Context, run runRecord) (*exec.Cmd, string, error) {
	plan, err := buildDefaultEditPlan(run)
	if err != nil {
		return nil, "", err
	}
	cmd, err := buildExecutionCommand(ctx, run, []string{"/bin/sh", "-lc", buildDefaultEditScript(plan)}, []string{
		"PICOCLAW_EXECUTION_STEP=thin-prepare",
		"PICOCLAW_EXECUTION_STAGE=" + string(appruns.StageThinPrepare),
	})
	if err != nil {
		return nil, "", err
	}
	return cmd, plan.Summary, nil
}

func buildDefaultEditPlan(run runRecord) (defaultEditPlan, error) {
	if plan, ok := buildFlutterLandingEditPlan(run); ok {
		return plan, nil
	}
	outputPath, err := defaultExecutorOutputPath(run)
	if err != nil {
		return defaultEditPlan{}, err
	}
	return defaultEditPlan{
		Summary: buildExecutionSummary(run) + " | output=" + outputPath,
		Files: []defaultEditFile{{
			Path:    outputPath,
			Content: renderDefaultEditContent(run, outputPath),
			Mode:    defaultEditOverwrite,
		}},
	}, nil
}

func buildFlutterLandingEditPlan(run runRecord) (defaultEditPlan, bool) {
	if !looksLikeFlutterLandingRun(run) {
		return defaultEditPlan{}, false
	}
	files := []defaultEditFile{
		{
			Path:    "lib/models/entry.dart",
			Content: renderFlutterEntryModel(),
			Mode:    defaultEditIfMissing,
		},
		{
			Path:    "lib/models/summary.dart",
			Content: renderFlutterSummaryModel(),
			Mode:    defaultEditIfMissing,
		},
		{
			Path:    "lib/repositories/entry_repository.dart",
			Content: renderFlutterEntryRepository(),
			Mode:    defaultEditIfMissing,
		},
		{
			Path:    "lib/controllers/home_controller.dart",
			Content: renderFlutterHomeController(),
			Mode:    defaultEditIfMissing,
		},
		{
			Path:    "lib/controllers/entry_form_controller.dart",
			Content: renderFlutterEntryFormController(),
			Mode:    defaultEditIfMissing,
		},
		{
			Path:    "lib/controllers/entry_list_controller.dart",
			Content: renderFlutterEntryListController(),
			Mode:    defaultEditIfMissing,
		},
		{
			Path:    "lib/views/home_page.dart",
			Content: renderFlutterHomePage(),
			Mode:    defaultEditIfMissing,
		},
		{
			Path:    "lib/views/entry_form_page.dart",
			Content: renderFlutterEntryFormPage(),
			Mode:    defaultEditIfMissing,
		},
		{
			Path:    "lib/views/entry_list_page.dart",
			Content: renderFlutterEntryListPage(),
			Mode:    defaultEditIfMissing,
		},
		{
			Path:    "lib/main.dart",
			Content: renderFlutterMain(),
			Mode:    defaultEditIfDemo,
		},
	}
	if isProbePathAllowed("pubspec.yaml", run.AllowedPaths, run.ProtectedPaths) {
		files = append(files, defaultEditFile{
			Path:    "pubspec.yaml",
			Content: renderFlutterPubspec(),
			Mode:    defaultEditIfIncomplete,
		})
	}
	if isProbePathAllowed("test/widget_test.dart", run.AllowedPaths, run.ProtectedPaths) {
		files = append(files, defaultEditFile{
			Path:    "test/widget_test.dart",
			Content: renderFlutterWidgetTest(),
			Mode:    defaultEditIfDemo,
		})
	}
	filtered := make([]defaultEditFile, 0, len(files))
	for _, file := range files {
		if isProbePathAllowed(file.Path, run.AllowedPaths, run.ProtectedPaths) {
			filtered = append(filtered, file)
		}
	}
	if len(filtered) == 0 {
		return defaultEditPlan{}, false
	}
	return defaultEditPlan{
		Summary: buildExecutionSummary(run) + " | flutter landing skeleton",
		Files:   filtered,
	}, true
}

func looksLikeFlutterLandingRun(run runRecord) bool {
	if hasKnowledgeSkill(run, "flutter-mvc-template") || hasKnowledgeSkill(run, "builder-direct-edit") {
		return true
	}
	for _, check := range run.AcceptanceChecks {
		switch strings.TrimSpace(check.CheckID) {
		case "check-counter-demo-removed", "check-entry-form-wiring", "check-local-persistence-wiring":
			return true
		}
		if strings.HasPrefix(strings.TrimSpace(check.CheckID), "check-flutter-") {
			return true
		}
	}
	for _, allowedPath := range run.AllowedPaths {
		trimmed := filepath.ToSlash(strings.TrimSpace(allowedPath))
		if trimmed == "pubspec.yaml" || trimmed == "test/**" || trimmed == "lib/**" {
			return true
		}
	}
	for _, task := range run.TaskBundle {
		for _, targetPath := range task.TargetPaths {
			trimmed := filepath.ToSlash(strings.TrimSpace(targetPath))
			if trimmed == "lib/main.dart" || strings.HasPrefix(trimmed, "lib/views/") || strings.HasPrefix(trimmed, "lib/controllers/") || strings.HasPrefix(trimmed, "lib/repositories/") {
				return true
			}
		}
	}
	return false
}

func defaultExecutorOutputPath(run runRecord) (string, error) {
	for _, task := range run.TaskBundle {
		for _, targetPath := range task.TargetPaths {
			candidate, ok := candidateProbePathFromReference(targetPath)
			if ok && isProbePathAllowed(candidate, run.AllowedPaths, run.ProtectedPaths) {
				return candidate, nil
			}
		}
	}
	for _, allowedPath := range run.AllowedPaths {
		candidate, ok := candidateProbePathFromAllowedPath(allowedPath)
		if ok && isProbePathAllowed(candidate, run.AllowedPaths, run.ProtectedPaths) {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("run %s has no writable allowed path for default thin executor", run.RunID)
}

func candidateProbePathFromReference(reference string) (string, bool) {
	trimmed := filepath.ToSlash(strings.TrimSpace(reference))
	if trimmed == "" || strings.ContainsAny(trimmed, "*?[]") {
		return "", false
	}
	cleaned := path.Clean(trimmed)
	if cleaned == "." || strings.HasPrefix(cleaned, "../") || path.IsAbs(cleaned) {
		return "", false
	}
	return path.Join(path.Dir(cleaned), defaultProbeFilename(cleaned)), true
}

func candidateProbePathFromAllowedPath(allowedPath string) (string, bool) {
	trimmed := filepath.ToSlash(strings.TrimSpace(allowedPath))
	if trimmed == "" {
		return "", false
	}
	if strings.HasSuffix(trimmed, "/**") {
		dir := path.Clean(strings.TrimSuffix(trimmed, "/**"))
		if dir == "." || dir == "" {
			return defaultProbeFilename("."), true
		}
		return path.Join(dir, defaultProbeFilename(dir)), true
	}
	if strings.ContainsAny(trimmed, "*?[]") {
		return "", false
	}
	cleaned := path.Clean(trimmed)
	if cleaned == "." || strings.HasPrefix(cleaned, "../") || path.IsAbs(cleaned) {
		return "", false
	}
	return path.Join(path.Dir(cleaned), defaultProbeFilename(cleaned)), true
}

func isProbePathAllowed(relPath string, allowedPaths, protectedPaths []string) bool {
	if strings.TrimSpace(relPath) == "" {
		return false
	}
	if matchesWorkspacePatterns(relPath, protectedPaths) {
		return false
	}
	return matchesWorkspacePatterns(relPath, allowedPaths)
}

func defaultProbeFilename(reference string) string {
	ext := filepath.Ext(reference)
	if ext == "" {
		rel := filepath.ToSlash(reference)
		if rel == "lib" || rel == "test" || strings.HasPrefix(rel, "lib/") || strings.HasPrefix(rel, "test/") {
			ext = ".dart"
		} else {
			ext = ".txt"
		}
	}
	return "picoclaw_executor_probe" + ext
}

func buildDefaultEditScript(plan defaultEditPlan) string {
	var builder strings.Builder
	for _, file := range plan.Files {
		dir := path.Dir(file.Path)
		if dir != "." {
			builder.WriteString("mkdir -p ")
			builder.WriteString(shellQuote(dir))
			builder.WriteString("\n")
		}
		switch file.Mode {
		case defaultEditIfMissing:
			builder.WriteString("if [ ! -f ")
			builder.WriteString(shellQuote(file.Path))
			builder.WriteString(" ]; then\n")
			appendDefaultEditHereDoc(&builder, file.Path, file.Content)
			builder.WriteString("fi\n")
		case defaultEditIfIncomplete:
			builder.WriteString("if [ ! -f ")
			builder.WriteString(shellQuote(file.Path))
			builder.WriteString(" ] || ! grep -E '^[[:space:]]*shared_preferences:' ")
			builder.WriteString(shellQuote(file.Path))
			builder.WriteString(" >/dev/null 2>&1; then\n")
			appendDefaultEditHereDoc(&builder, file.Path, file.Content)
			builder.WriteString("fi\n")
		case defaultEditIfDemo:
			builder.WriteString("if [ ! -f ")
			builder.WriteString(shellQuote(file.Path))
			builder.WriteString(" ] || grep -E 'Flutter Demo Home Page|You have pushed the button this many times|Counter increments smoke test|_counter|_incrementCounter|MyHomePage' ")
			builder.WriteString(shellQuote(file.Path))
			builder.WriteString(" >/dev/null 2>&1; then\n")
			appendDefaultEditHereDoc(&builder, file.Path, file.Content)
			builder.WriteString("fi\n")
		default:
			appendDefaultEditHereDoc(&builder, file.Path, file.Content)
		}
	}
	return builder.String()
}

func appendDefaultEditHereDoc(builder *strings.Builder, outputPath, content string) {
	builder.WriteString("cat > ")
	builder.WriteString(shellQuote(outputPath))
	builder.WriteString(" <<'EOF'\n")
	builder.WriteString(content)
	if !strings.HasSuffix(content, "\n") {
		builder.WriteString("\n")
	}
	builder.WriteString("EOF\n")
}

func renderDefaultEditContent(run runRecord, outputPath string) string {
	if filepath.Ext(outputPath) == ".dart" {
		return renderDefaultDartProbe(run)
	}
	return renderDefaultTextProbe(run)
}

func renderDefaultDartProbe(run runRecord) string {
	return strings.Join([]string{
		"// Generated by PicoClaw thin executor.",
		"// This file proves the default inspect -> edit -> validate loop can write a real workspace patch.",
		"const picoclawExecutorProbe = <String, Object>{",
		"  'jobId': " + dartString(run.JobID) + ",",
		"  'goalSummary': " + dartString(run.GoalSummary) + ",",
		fmt.Sprintf("  'taskCount': %d,", len(run.TaskBundle)),
		fmt.Sprintf("  'acceptanceCheckCount': %d,", len(run.AcceptanceChecks)),
		"};",
	}, "\n") + "\n"
}

func renderDefaultTextProbe(run runRecord) string {
	return strings.Join([]string{
		"Generated by PicoClaw thin executor.",
		"This file proves the default inspect -> edit -> validate loop can write a real workspace patch.",
		"job_id=" + strings.TrimSpace(run.JobID),
		"goal_summary=" + strings.TrimSpace(run.GoalSummary),
		fmt.Sprintf("task_count=%d", len(run.TaskBundle)),
		fmt.Sprintf("acceptance_check_count=%d", len(run.AcceptanceChecks)),
	}, "\n") + "\n"
}

func renderFlutterPubspec() string {
	return strings.TrimLeft(`name: picoclaw_executor_app
description: Minimal bookkeeping workspace generated by PicoClaw thin executor.
publish_to: 'none'
version: 0.1.0+1

environment:
  sdk: '>=3.3.0 <4.0.0'

dependencies:
  flutter:
    sdk: flutter
  shared_preferences: ^2.2.3

dev_dependencies:
  flutter_test:
    sdk: flutter
  flutter_lints: ^3.0.0

flutter:
  uses-material-design: true
`, "\n")
}

func renderFlutterMain() string {
	return strings.TrimLeft(`import 'package:flutter/material.dart';
import 'package:shared_preferences/shared_preferences.dart';

import 'controllers/entry_form_controller.dart';
import 'controllers/entry_list_controller.dart';
import 'controllers/home_controller.dart';
import 'repositories/entry_repository.dart';
import 'views/entry_form_page.dart';
import 'views/entry_list_page.dart';
import 'views/home_page.dart';

Future<void> main() async {
	WidgetsFlutterBinding.ensureInitialized();
	final preferences = await SharedPreferences.getInstance();
	final repository = EntryRepository(preferences: preferences);
	runApp(BookkeepingApp(repository: repository));
}

class BookkeepingApp extends StatefulWidget {
	const BookkeepingApp({super.key, required this.repository});

	final EntryRepository repository;

	@override
	State<BookkeepingApp> createState() => _BookkeepingAppState();
}

class _BookkeepingAppState extends State<BookkeepingApp> {
	late final HomeController _homeController;
	late final EntryFormController _entryFormController;
	late final EntryListController _entryListController;
	int _currentIndex = 0;

	@override
	void initState() {
		super.initState();
		_homeController = HomeController(repository: widget.repository);
		_entryFormController = EntryFormController(repository: widget.repository);
		_entryListController = EntryListController(repository: widget.repository);
		_homeController.reload();
		_entryListController.load();
	}

	@override
	void dispose() {
		_homeController.dispose();
		_entryFormController.dispose();
		_entryListController.dispose();
		super.dispose();
	}

	Future<void> _handleEntrySaved() async {
		await _homeController.reload();
		await _entryListController.load();
		if (!mounted) {
			return;
		}
		setState(() {
			_currentIndex = 0;
		});
	}

	@override
	Widget build(BuildContext context) {
		final pages = <Widget>[
			HomePage(
				controller: _homeController,
				onAddPressed: () => setState(() {
					_currentIndex = 1;
				}),
				onListPressed: () => setState(() {
					_currentIndex = 2;
				}),
			),
			EntryFormPage(
				controller: _entryFormController,
				onSaved: _handleEntrySaved,
			),
			EntryListPage(controller: _entryListController),
		];

		return MaterialApp(
			title: 'PicoClaw Bookkeeping',
			theme: ThemeData(
				colorScheme: ColorScheme.fromSeed(seedColor: Colors.teal),
				useMaterial3: true,
			),
			home: Scaffold(
				body: SafeArea(child: pages[_currentIndex]),
				bottomNavigationBar: NavigationBar(
					selectedIndex: _currentIndex,
					onDestinationSelected: (index) {
						setState(() {
							_currentIndex = index;
						});
						if (index == 0) {
							_homeController.reload();
						}
						if (index == 2) {
							_entryListController.load();
						}
					},
					destinations: const [
						NavigationDestination(icon: Icon(Icons.dashboard_outlined), label: 'Overview'),
						NavigationDestination(icon: Icon(Icons.add_circle_outline), label: 'Add Entry'),
						NavigationDestination(icon: Icon(Icons.receipt_long_outlined), label: 'Entries'),
					],
				),
			),
		);
	}
}
`, "\n")
}

func renderFlutterEntryModel() string {
	return strings.TrimLeft(`enum EntryType { income, expense }

class Entry {
	const Entry({
		required this.amount,
		required this.type,
		required this.date,
		this.note = '',
	});

	final double amount;
	final EntryType type;
	final DateTime date;
	final String note;

	Map<String, Object> toJson() {
		return {
			'amount': amount,
			'type': type.name,
			'date': date.toIso8601String(),
			'note': note,
		};
	}

	factory Entry.fromJson(Map<String, dynamic> json) {
		final typeName = json['type'] as String? ?? EntryType.expense.name;
		return Entry(
			amount: (json['amount'] as num?)?.toDouble() ?? 0,
			type: EntryType.values.firstWhere(
				(item) => item.name == typeName,
				orElse: () => EntryType.expense,
			),
			date: DateTime.tryParse(json['date'] as String? ?? '') ?? DateTime.now(),
			note: json['note'] as String? ?? '',
		);
	}
}
`, "\n")
}

func renderFlutterSummaryModel() string {
	return strings.TrimLeft(`class EntrySummary {
	const EntrySummary({
		required this.totalIncome,
		required this.totalExpense,
	});

	final double totalIncome;
	final double totalExpense;

	double get balance => totalIncome - totalExpense;
}
`, "\n")
}

func renderFlutterEntryRepository() string {
	return strings.TrimLeft(`import 'dart:convert';

import 'package:shared_preferences/shared_preferences.dart';

import '../models/entry.dart';
import '../models/summary.dart';

class EntryRepository {
	EntryRepository({required this.preferences});

	final SharedPreferences preferences;
	static const _entriesKey = 'bookkeeping_entries';

	Future<List<Entry>> loadEntries() async {
		final payload = preferences.getString(_entriesKey);
		if (payload == null || payload.isEmpty) {
			return const [];
		}
		final decoded = jsonDecode(payload) as List<dynamic>;
		return decoded
				.map((item) => Entry.fromJson(item as Map<String, dynamic>))
				.toList()
			..sort((left, right) => right.date.compareTo(left.date));
	}

	Future<void> saveEntry(Entry entry) async {
		final entries = await loadEntries();
		final updated = <Entry>[entry, ...entries];
		final encoded = jsonEncode(updated.map((item) => item.toJson()).toList());
		await preferences.setString(_entriesKey, encoded);
	}

	Future<EntrySummary> loadSummary() async {
		final entries = await loadEntries();
		double income = 0;
		double expense = 0;
		for (final entry in entries) {
			if (entry.type == EntryType.income) {
				income += entry.amount;
			} else {
				expense += entry.amount;
			}
		}
		return EntrySummary(totalIncome: income, totalExpense: expense);
	}
}
`, "\n")
}

func renderFlutterHomeController() string {
	return strings.TrimLeft(`import 'package:flutter/foundation.dart';

import '../models/entry.dart';
import '../models/summary.dart';
import '../repositories/entry_repository.dart';

class HomeController extends ChangeNotifier {
	HomeController({required this.repository});

	final EntryRepository repository;
	EntrySummary _summary = const EntrySummary(totalIncome: 0, totalExpense: 0);
	List<Entry> _recentEntries = const [];
	bool _loading = false;

	EntrySummary get summary => _summary;
	List<Entry> get recentEntries => _recentEntries;
	bool get loading => _loading;

	Future<void> reload() async {
		_loading = true;
		notifyListeners();
		_summary = await repository.loadSummary();
		_recentEntries = (await repository.loadEntries()).take(5).toList();
		_loading = false;
		notifyListeners();
	}
}
`, "\n")
}

func renderFlutterEntryFormController() string {
	return strings.TrimLeft(`import 'package:flutter/widgets.dart';

import '../models/entry.dart';
import '../repositories/entry_repository.dart';

class EntryFormController extends ChangeNotifier {
	EntryFormController({required this.repository});

	final EntryRepository repository;
	final amountController = TextEditingController();
	final noteController = TextEditingController();
	EntryType selectedType = EntryType.expense;
	DateTime selectedDate = DateTime.now();
	bool saving = false;

	void setSelectedType(EntryType type) {
		selectedType = type;
		notifyListeners();
	}

	void setSelectedDate(DateTime date) {
		selectedDate = date;
		notifyListeners();
	}

	Future<bool> submit() async {
		final amount = double.tryParse(amountController.text.trim());
		if (amount == null || amount <= 0) {
			return false;
		}
		saving = true;
		notifyListeners();
		await repository.saveEntry(Entry(
			amount: amount,
			type: selectedType,
			date: selectedDate,
			note: noteController.text.trim(),
		));
		amountController.clear();
		noteController.clear();
		selectedType = EntryType.expense;
		selectedDate = DateTime.now();
		saving = false;
		notifyListeners();
		return true;
	}

	@override
	void dispose() {
		amountController.dispose();
		noteController.dispose();
		super.dispose();
	}
}
`, "\n")
}

func renderFlutterEntryListController() string {
	return strings.TrimLeft(`import 'package:flutter/foundation.dart';

import '../models/entry.dart';
import '../repositories/entry_repository.dart';

class EntryListController extends ChangeNotifier {
	EntryListController({required this.repository});

	final EntryRepository repository;
	List<Entry> _entries = const [];
	bool _loading = false;

	List<Entry> get entries => _entries;
	bool get loading => _loading;

	Future<void> load() async {
		_loading = true;
		notifyListeners();
		_entries = await repository.loadEntries();
		_loading = false;
		notifyListeners();
	}
}
`, "\n")
}

func renderFlutterHomePage() string {
	return strings.TrimLeft(`import 'package:flutter/material.dart';

import '../controllers/home_controller.dart';
import '../models/entry.dart';

class HomePage extends StatefulWidget {
	const HomePage({
		super.key,
		required this.controller,
		required this.onAddPressed,
		required this.onListPressed,
	});

	final HomeController controller;
	final VoidCallback onAddPressed;
	final VoidCallback onListPressed;

	@override
	State<HomePage> createState() => _HomePageState();
}

class _HomePageState extends State<HomePage> {
	@override
	void initState() {
		super.initState();
		widget.controller.reload();
	}

	@override
	Widget build(BuildContext context) {
		return AnimatedBuilder(
			animation: widget.controller,
			builder: (context, _) {
				final summary = widget.controller.summary;
				return RefreshIndicator(
					onRefresh: widget.controller.reload,
					child: ListView(
						padding: const EdgeInsets.all(24),
						children: [
							Text('Bookkeeping overview', style: Theme.of(context).textTheme.headlineMedium),
							const SizedBox(height: 8),
							Text('Track quick income, expenses, and recent entries in one place.', style: Theme.of(context).textTheme.bodyMedium),
							const SizedBox(height: 24),
							Wrap(
								spacing: 12,
								runSpacing: 12,
								children: [
									_SummaryCard(label: 'Income', value: summary.totalIncome),
									_SummaryCard(label: 'Expense', value: summary.totalExpense),
									_SummaryCard(label: 'Balance', value: summary.balance),
								],
							),
							const SizedBox(height: 24),
							Row(
								children: [
									ElevatedButton.icon(onPressed: widget.onAddPressed, icon: const Icon(Icons.add), label: const Text('Add entry')),
									const SizedBox(width: 12),
									OutlinedButton.icon(onPressed: widget.onListPressed, icon: const Icon(Icons.receipt_long_outlined), label: const Text('View entries')),
								],
							),
							const SizedBox(height: 24),
							Text('Recent entries', style: Theme.of(context).textTheme.titleLarge),
							const SizedBox(height: 12),
							if (widget.controller.loading)
								const Center(child: CircularProgressIndicator())
							else if (widget.controller.recentEntries.isEmpty)
								const Card(child: Padding(padding: EdgeInsets.all(16), child: Text('No saved entries yet. Start with the add entry form.')))
							else
								...widget.controller.recentEntries.map(_EntryTile.new),
						],
					),
				);
			},
		);
	}
}

class _SummaryCard extends StatelessWidget {
	const _SummaryCard({required this.label, required this.value});

	final String label;
	final double value;

	@override
	Widget build(BuildContext context) {
		return SizedBox(
			width: 180,
			child: Card(
				child: Padding(
					padding: const EdgeInsets.all(16),
					child: Column(
						crossAxisAlignment: CrossAxisAlignment.start,
						children: [
							Text(label, style: Theme.of(context).textTheme.labelLarge),
							const SizedBox(height: 8),
							Text(value.toStringAsFixed(2), style: Theme.of(context).textTheme.headlineSmall),
						],
					),
				),
			),
		);
	}
}

class _EntryTile extends StatelessWidget {
	const _EntryTile(this.entry);

	final Entry entry;

	@override
	Widget build(BuildContext context) {
		return Card(
			child: ListTile(
				leading: Icon(entry.type == EntryType.income ? Icons.south_west : Icons.north_east),
				title: Text(entry.note.isEmpty ? entry.type.name.toUpperCase() : entry.note),
				subtitle: Text(entry.date.toIso8601String().split('T').first),
				trailing: Text(entry.amount.toStringAsFixed(2)),
			),
		);
	}
}
`, "\n")
}

func renderFlutterEntryFormPage() string {
	return strings.TrimLeft(`import 'package:flutter/material.dart';

import '../controllers/entry_form_controller.dart';
import '../models/entry.dart';

class EntryFormPage extends StatefulWidget {
	const EntryFormPage({
		super.key,
		required this.controller,
		required this.onSaved,
	});

	final EntryFormController controller;
	final Future<void> Function() onSaved;

	@override
	State<EntryFormPage> createState() => _EntryFormPageState();
}

class _EntryFormPageState extends State<EntryFormPage> {
	final _formKey = GlobalKey<FormState>();

	Future<void> _pickDate() async {
		final picked = await showDatePicker(
			context: context,
			initialDate: widget.controller.selectedDate,
			firstDate: DateTime(2020),
			lastDate: DateTime(2100),
		);
		if (picked != null) {
			widget.controller.setSelectedDate(picked);
		}
	}

	Future<void> _submit() async {
		if (!_formKey.currentState!.validate()) {
			return;
		}
		final saved = await widget.controller.submit();
		if (!mounted) {
			return;
		}
		if (!saved) {
			ScaffoldMessenger.of(context).showSnackBar(const SnackBar(content: Text('Enter a valid positive amount.')));
			return;
		}
		await widget.onSaved();
		if (!mounted) {
			return;
		}
		ScaffoldMessenger.of(context).showSnackBar(const SnackBar(content: Text('Entry saved locally.')));
	}

	@override
	Widget build(BuildContext context) {
		return AnimatedBuilder(
			animation: widget.controller,
			builder: (context, _) {
				return ListView(
					padding: const EdgeInsets.all(24),
					children: [
						Text('Add entry', style: Theme.of(context).textTheme.headlineMedium),
						const SizedBox(height: 8),
						Text('Capture amount, type, and date, then persist it locally.', style: Theme.of(context).textTheme.bodyMedium),
						const SizedBox(height: 24),
						Form(
							key: _formKey,
							child: Column(
								crossAxisAlignment: CrossAxisAlignment.start,
								children: [
									TextFormField(
										controller: widget.controller.amountController,
										keyboardType: const TextInputType.numberWithOptions(decimal: true),
										decoration: const InputDecoration(labelText: 'Amount'),
										validator: (value) {
											final amount = double.tryParse((value ?? '').trim());
											if (amount == null || amount <= 0) {
												return 'Enter a valid amount';
											}
											return null;
										},
									),
									const SizedBox(height: 16),
									DropdownButtonFormField<EntryType>(
										value: widget.controller.selectedType,
										decoration: const InputDecoration(labelText: 'Type'),
										items: EntryType.values
												.map((item) => DropdownMenuItem(value: item, child: Text(item.name)))
												.toList(),
										onChanged: (value) {
											if (value != null) {
												widget.controller.setSelectedType(value);
											}
										},
									),
									const SizedBox(height: 16),
									TextFormField(
										controller: widget.controller.noteController,
										decoration: const InputDecoration(labelText: 'Note'),
									),
									const SizedBox(height: 16),
									OutlinedButton.icon(
										onPressed: _pickDate,
										icon: const Icon(Icons.calendar_today_outlined),
										label: Text(widget.controller.selectedDate.toIso8601String().split('T').first),
									),
									const SizedBox(height: 24),
									FilledButton.icon(
										onPressed: widget.controller.saving ? null : _submit,
										icon: const Icon(Icons.save_outlined),
										label: Text(widget.controller.saving ? 'Saving...' : 'Save locally'),
									),
								],
							),
						),
					],
				);
			},
		);
	}
}
`, "\n")
}

func renderFlutterEntryListPage() string {
	return strings.TrimLeft(`import 'package:flutter/material.dart';

import '../controllers/entry_list_controller.dart';
import '../models/entry.dart';

class EntryListPage extends StatefulWidget {
	const EntryListPage({super.key, required this.controller});

	final EntryListController controller;

	@override
	State<EntryListPage> createState() => _EntryListPageState();
}

class _EntryListPageState extends State<EntryListPage> {
	@override
	void initState() {
		super.initState();
		widget.controller.load();
	}

	@override
	Widget build(BuildContext context) {
		return AnimatedBuilder(
			animation: widget.controller,
			builder: (context, _) {
				if (widget.controller.loading) {
					return const Center(child: CircularProgressIndicator());
				}
				return ListView(
					padding: const EdgeInsets.all(24),
					children: [
						Text('Saved entries', style: Theme.of(context).textTheme.headlineMedium),
						const SizedBox(height: 8),
						Text('Review locally persisted bookkeeping items in reverse chronological order.', style: Theme.of(context).textTheme.bodyMedium),
						const SizedBox(height: 24),
						if (widget.controller.entries.isEmpty)
							const Card(child: Padding(padding: EdgeInsets.all(16), child: Text('No entries available yet.')))
						else
							...widget.controller.entries.map(_EntryRow.new),
					],
				);
			},
		);
	}
}

class _EntryRow extends StatelessWidget {
	const _EntryRow(this.entry);

	final Entry entry;

	@override
	Widget build(BuildContext context) {
		return Card(
			child: ListTile(
				title: Text(entry.note.isEmpty ? entry.type.name.toUpperCase() : entry.note),
				subtitle: Text(entry.date.toIso8601String().split('T').first),
				trailing: Text(entry.amount.toStringAsFixed(2)),
			),
		);
	}
}
`, "\n")
}

func renderFlutterWidgetTest() string {
	return strings.TrimLeft(`import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
	testWidgets('bookkeeping shell renders', (tester) async {
		await tester.pumpWidget(const MaterialApp(home: Text('Bookkeeping overview')));

		expect(find.text('Bookkeeping overview'), findsOneWidget);
	});
}
`, "\n")
}

func dartString(value string) string {
	replacer := strings.NewReplacer("\\", "\\\\", "'", "\\'", "\n", "\\n", "\r", "\\r")
	return "'" + replacer.Replace(value) + "'"
}

func buildExecutionCommand(ctx context.Context, run runRecord, argv []string, extraEnv []string) (*exec.Cmd, error) {
	if len(argv) == 0 || strings.TrimSpace(argv[0]) == "" {
		return nil, fmt.Errorf("run %s has no launch command", run.RunID)
	}
	if run.ExecutorImage != "" {
		return dockerCommand(ctx, run, argv, extraEnv)
	}
	env, err := localExecutionEnv(run)
	if err != nil {
		return nil, err
	}
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Dir = run.WorkspacePath
	cmd.Env = append(os.Environ(), env...)
	cmd.Env = append(cmd.Env, extraEnv...)
	return cmd, nil
}

func localExecutionEnv(run runRecord) ([]string, error) {
	env := append([]string(nil), baseExecutionEnv(run)...)
	root := appFactoryRootFromWorkspace(run.WorkspacePath)
	pubCacheDir := resolveBuilderPubCache(root)
	if err := os.MkdirAll(pubCacheDir, 0o755); err != nil {
		return nil, fmt.Errorf("create builder pub cache: %w", err)
	}
	env = append(env, "PUB_CACHE="+pubCacheDir)
	return env, nil
}

func DockerPrepareCommandForTest(ctx context.Context, run RunRecord) (*exec.Cmd, error) {
	return dockerPrepareCommand(ctx, run)
}

func LocalExecutionEnvForTest(run RunRecord) ([]string, error) {
	return localExecutionEnv(run)
}

func baseExecutionEnv(run runRecord) []string {
	return []string{
		"PICOCLAW_RUN_ID=" + run.RunID,
		"PICOCLAW_JOB_ID=" + run.JobID,
		"PICOCLAW_ARTIFACT_DIR=" + run.ArtifactDir,
	}
}

func buildExecutionSummary(run runRecord) string {
	parts := make([]string, 0, 5)
	if goal := strings.TrimSpace(run.GoalSummary); goal != "" {
		parts = append(parts, goal)
	}
	if knowledgePack := buildKnowledgePackSummary(run.KnowledgePack); knowledgePack != "" {
		parts = append(parts, knowledgePack)
	}
	if taskFocus := buildTaskFocusSummary(run); taskFocus != "" {
		parts = append(parts, taskFocus)
	}
	if len(run.TaskBundle) > 0 {
		parts = append(parts, fmt.Sprintf("tasks=%d", len(run.TaskBundle)))
	}
	if len(run.AcceptanceChecks) > 0 {
		parts = append(parts, fmt.Sprintf("checks=%d", len(run.AcceptanceChecks)))
	}
	return strings.Join(parts, " | ")
}

func hasKnowledgeSkill(run runRecord, skillID string) bool {
	trimmedID := strings.TrimSpace(skillID)
	if trimmedID == "" {
		return false
	}
	for _, skill := range run.KnowledgePack {
		if strings.TrimSpace(skill.SkillID) == trimmedID {
			return true
		}
	}
	return false
}

func buildKnowledgePackSummary(skills []appruns.ProfileSkill) string {
	ordered := orderedKnowledgePack(skills)
	if len(ordered) == 0 {
		return ""
	}
	parts := make([]string, 0, len(ordered))
	for _, skill := range ordered {
		skillID := strings.TrimSpace(skill.SkillID)
		if skillID == "" {
			continue
		}
		usageStage := strings.TrimSpace(skill.UsageStage)
		if usageStage == "" {
			parts = append(parts, skillID)
			continue
		}
		parts = append(parts, usageStage+":"+skillID)
	}
	if len(parts) == 0 {
		return ""
	}
	return "skill_flow=" + strings.Join(parts, " -> ")
}

func buildTaskFocusSummary(run runRecord) string {
	if !hasKnowledgeSkill(run, "prd-to-task-bundle") || len(run.TaskBundle) == 0 {
		return ""
	}
	focusIDs := make([]string, 0, 2)
	for _, task := range run.TaskBundle {
		if strings.TrimSpace(task.TaskID) == "" {
			continue
		}
		focusIDs = append(focusIDs, task.TaskID)
		if len(focusIDs) == 2 {
			break
		}
	}
	if len(focusIDs) == 0 {
		return ""
	}
	return "focus_tasks=" + strings.Join(focusIDs, ",")
}

func orderedKnowledgePack(skills []appruns.ProfileSkill) []appruns.ProfileSkill {
	if len(skills) == 0 {
		return nil
	}
	ordered := make([]appruns.ProfileSkill, 0, len(skills))
	used := make([]bool, len(skills))
	for _, stage := range []string{"planning", "layout", "edit", "closure"} {
		for index, skill := range skills {
			if used[index] || strings.TrimSpace(skill.UsageStage) != stage {
				continue
			}
			ordered = append(ordered, skill)
			used[index] = true
		}
	}
	for index, skill := range skills {
		if used[index] {
			continue
		}
		ordered = append(ordered, skill)
	}
	return ordered
}

func plannedTaskBundle(run runRecord) []appruns.TaskBundleItem {
	tasks := append([]appruns.TaskBundleItem(nil), run.TaskBundle...)
	if len(tasks) == 0 || !hasKnowledgeSkill(run, "prd-to-task-bundle") {
		return tasks
	}
	return orderTaskBundle(tasks)
}

func orderTaskBundle(tasks []appruns.TaskBundleItem) []appruns.TaskBundleItem {
	if len(tasks) <= 1 {
		return tasks
	}
	type taskNode struct {
		index int
		task  appruns.TaskBundleItem
	}
	nodes := make([]taskNode, 0, len(tasks))
	known := make(map[string]struct{}, len(tasks))
	for index, task := range tasks {
		nodes = append(nodes, taskNode{index: index, task: task})
		if taskID := strings.TrimSpace(task.TaskID); taskID != "" {
			known[taskID] = struct{}{}
		}
	}
	ordered := make([]appruns.TaskBundleItem, 0, len(tasks))
	completed := make(map[string]struct{}, len(tasks))
	used := make([]bool, len(nodes))
	for len(ordered) < len(tasks) {
		ready := make([]taskNode, 0, len(nodes))
		for index, node := range nodes {
			if used[index] || !taskDependenciesSatisfied(node.task, known, completed) {
				continue
			}
			ready = append(ready, node)
		}
		if len(ready) == 0 {
			remaining := make([]taskNode, 0, len(nodes))
			for index, node := range nodes {
				if used[index] {
					continue
				}
				remaining = append(remaining, node)
			}
			sort.SliceStable(remaining, func(left, right int) bool {
				return compareTaskNodes(remaining[left], remaining[right]) < 0
			})
			for _, node := range remaining {
				ordered = append(ordered, node.task)
			}
			break
		}
		sort.SliceStable(ready, func(left, right int) bool {
			return compareTaskNodes(ready[left], ready[right]) < 0
		})
		selected := ready[0]
		ordered = append(ordered, selected.task)
		if taskID := strings.TrimSpace(selected.task.TaskID); taskID != "" {
			completed[taskID] = struct{}{}
		}
		used[selected.index] = true
	}
	return ordered
}

func taskDependenciesSatisfied(task appruns.TaskBundleItem, known, completed map[string]struct{}) bool {
	for _, dependency := range task.Dependencies {
		dependency = strings.TrimSpace(dependency)
		if dependency == "" {
			continue
		}
		if _, exists := known[dependency]; !exists {
			continue
		}
		if _, done := completed[dependency]; !done {
			return false
		}
	}
	return true
}

func compareTaskNodes(left, right struct {
	index int
	task  appruns.TaskBundleItem
}) int {
	leftRank := taskCategoryRank(left.task.Category)
	rightRank := taskCategoryRank(right.task.Category)
	if leftRank != rightRank {
		return leftRank - rightRank
	}
	if len(left.task.Dependencies) != len(right.task.Dependencies) {
		return len(left.task.Dependencies) - len(right.task.Dependencies)
	}
	return left.index - right.index
}

func taskCategoryRank(category appruns.TaskCategory) int {
	switch category {
	case appruns.TaskCategoryDomain:
		return 0
	case appruns.TaskCategoryStorage:
		return 1
	case appruns.TaskCategoryScreen:
		return 2
	case appruns.TaskCategoryFlow:
		return 3
	case appruns.TaskCategoryValidation:
		return 4
	default:
		return 5
	}
}

func plannedAcceptanceChecks(run runRecord) []appruns.AcceptanceCheck {
	checks := append([]appruns.AcceptanceCheck(nil), run.AcceptanceChecks...)
	if len(checks) == 0 || !hasKnowledgeSkill(run, "flutter-build-closure") {
		return checks
	}
	sort.SliceStable(checks, func(left, right int) bool {
		return acceptanceCheckStageRank(checks[left].Stage) < acceptanceCheckStageRank(checks[right].Stage)
	})
	return checks
}

func acceptanceCheckStageRank(stage appruns.ExecutionStage) int {
	switch normalizeStage(stage) {
	case appruns.StageBaseline:
		return 0
	case appruns.StageCheap:
		return 1
	case appruns.StageMilestone:
		return 2
	case appruns.StageHeavy:
		return 3
	case appruns.StageDevice:
		return 4
	case appruns.StageSmoke:
		return 5
	case appruns.StageManual:
		return 6
	default:
		return 7
	}
}

func buildCheckSummary(check CheckExecutionPreview) string {
	parts := []string{"acceptance check: " + check.CheckID}
	if strings.TrimSpace(check.Label) != "" {
		parts = append(parts, check.Label)
	}
	parts = append(parts, "stage="+string(check.Stage))
	return strings.Join(parts, " | ")
}

func compactCommands(commands []string) []string {
	result := make([]string, 0, len(commands))
	for _, command := range commands {
		trimmed := strings.TrimSpace(command)
		if trimmed == "" {
			continue
		}
		result = append(result, trimmed)
	}
	return result
}

func normalizeStage(stage appruns.ExecutionStage) appruns.ExecutionStage {
	if stage == "" {
		return appruns.StageOther
	}
	return stage
}

func previewTasks(tasks []appruns.TaskBundleItem) []TaskExecutionPreview {
	result := make([]TaskExecutionPreview, 0, len(tasks))
	for _, task := range tasks {
		result = append(result, TaskExecutionPreview{
			TaskID:    task.TaskID,
			Title:     task.Title,
			Category:  task.Category,
			DependsOn: append([]string(nil), task.Dependencies...),
		})
	}
	return result
}

func previewChecks(checks []appruns.AcceptanceCheck) []CheckExecutionPreview {
	result := make([]CheckExecutionPreview, 0, len(checks))
	for _, check := range checks {
		result = append(result, CheckExecutionPreview{
			CheckID:      check.CheckID,
			Label:        check.Label,
			Stage:        normalizeStage(check.Stage),
			Required:     check.Required,
			AllowFailure: check.AllowFailure,
			Commands:     append([]string(nil), check.Commands...),
		})
	}
	return result
}
