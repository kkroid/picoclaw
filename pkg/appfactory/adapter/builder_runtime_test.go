package adapter

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	appprepare "github.com/sipeed/picoclaw/pkg/appfactory/prepare"
	appruns "github.com/sipeed/picoclaw/pkg/appfactory/runs"
)

func writeBuilderRuntimeWorkspaceFiles(t *testing.T, workspacePath string, files map[string]string) {
	t.Helper()
	for relativePath, content := range files {
		fullPath := filepath.Join(workspacePath, filepath.FromSlash(relativePath))
		if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
			t.Fatalf("MkdirAll(%s) error = %v", relativePath, err)
		}
		if err := os.WriteFile(fullPath, []byte(content), 0o600); err != nil {
			t.Fatalf("WriteFile(%s) error = %v", relativePath, err)
		}
	}
}

func createBuilderRuntimeBookkeepingWorkspace(t *testing.T, files map[string]string) string {
	t.Helper()
	workspacePath := t.TempDir()
	workspaceFiles := map[string]string{
		"lib/models/entry.dart": strings.Join([]string{
			"class BookkeepingEntry {",
			"  const BookkeepingEntry({required this.occurredOn});",
			"  final DateTime occurredOn;",
			"}",
			"",
		}, "\n"),
	}
	for relativePath, content := range files {
		workspaceFiles[relativePath] = content
	}
	writeBuilderRuntimeWorkspaceFiles(t, workspacePath, workspaceFiles)
	return workspacePath
}

func createBuilderRuntimeRelationRichWorkspace(t *testing.T, files map[string]string) string {
	t.Helper()
	workspacePath := t.TempDir()
	workspaceFiles := map[string]string{
		"lib/models/project.dart": strings.Join([]string{
			"enum ProjectStatus { active, paused, done }",
			"class Project {",
			"  const Project({required this.projectId, required this.title, required this.status, this.color});",
			"  final String projectId;",
			"  final String title;",
			"  final ProjectStatus status;",
			"  final String? color;",
			"}",
		}, "\n") + "\n",
		"lib/models/task.dart": strings.Join([]string{
			"enum TaskStatus { todo, doing, done }",
			"class Task {",
			"  const Task({required this.taskId, required this.projectId, required this.title, required this.status, this.dueOn, this.note});",
			"  final String taskId;",
			"  final String projectId;",
			"  final String title;",
			"  final TaskStatus status;",
			"  final DateTime? dueOn;",
			"  final String? note;",
			"}",
		}, "\n") + "\n",
		"lib/models/tag.dart": strings.Join([]string{
			"class Tag {",
			"  const Tag({required this.tagId, required this.name, this.color});",
			"  final String tagId;",
			"  final String name;",
			"  final String? color;",
			"}",
		}, "\n") + "\n",
		"lib/models/task_tag_link.dart": strings.Join([]string{
			"class TaskTagLink {",
			"  const TaskTagLink({required this.linkId, required this.taskId, required this.tagId});",
			"  final String linkId;",
			"  final String taskId;",
			"  final String tagId;",
			"}",
		}, "\n") + "\n",
		"lib/models/dashboard_summary.dart": strings.Join([]string{
			"class DashboardSummary {",
			"  const DashboardSummary({required this.projectId, required this.openTaskCount, required this.doneTaskCount, required this.taggedTaskCount});",
			"  final String projectId;",
			"  final int openTaskCount;",
			"  final int doneTaskCount;",
			"  final int taggedTaskCount;",
			"}",
		}, "\n") + "\n",
		"lib/repositories/record_repository.dart": strings.Join([]string{
			"abstract class RecordRepository {",
			"  Future<List<dynamic>> loadProjects();",
			"  Future<List<dynamic>> loadTasks();",
			"  Future<List<dynamic>> loadTags();",
			"  Future<List<dynamic>> loadTaskTagLinks();",
			"}",
		}, "\n") + "\n",
	}
	for relativePath, content := range files {
		workspaceFiles[relativePath] = content
	}
	writeBuilderRuntimeWorkspaceFiles(t, workspacePath, workspaceFiles)
	return workspacePath
}

func createBuilderRuntimeCustomCollectionModelWorkspace(t *testing.T, taskModelContent string) string {
	t.Helper()
	workspacePath := t.TempDir()
	writeBuilderRuntimeWorkspaceFiles(t, workspacePath, map[string]string{
		"lib/models/task.dart":                  taskModelContent,
		"lib/repositories/task_repository.dart": "abstract class TaskRepository {}\n",
		"lib/controllers/task_collection_controller.dart": strings.Join([]string{
			"import '../models/task.dart';",
			"import '../repositories/task_repository.dart';",
			"",
			"class TaskCollectionController {",
			"  TaskCollectionController({required TaskRepository taskRepository});",
			"  List<Task> get records => const [];",
			"}",
		}, "\n") + "\n",
		"lib/views/task_collection_page.dart": strings.Join([]string{
			"class TaskCollectionPage {",
			"  const TaskCollectionPage({required this.controller, required this.onCreateTask, required this.onOpenTaskDetail});",
			"  final TaskCollectionController controller;",
			"  final Future<void> Function() onCreateTask;",
			"  final Future<void> Function(Task task) onOpenTaskDetail;",
			"}",
		}, "\n") + "\n",
	})
	return workspacePath
}

func createBuilderRuntimeWorkspaceWithDomainModel(t *testing.T, domainModel string, files map[string]string) string {
	t.Helper()
	jobRoot := t.TempDir()
	workspacePath := filepath.Join(jobRoot, "workspace")
	writeBuilderRuntimeWorkspaceFiles(t, workspacePath, files)
	preparePath := filepath.Join(jobRoot, "prepare")
	if err := os.MkdirAll(preparePath, 0o755); err != nil {
		t.Fatalf("MkdirAll(prepare) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(preparePath, "domain-model.json"), []byte(domainModel), 0o600); err != nil {
		t.Fatalf("WriteFile(domain-model.json) error = %v", err)
	}
	return workspacePath
}

func createBuilderRuntimeCustomMutationSurfaceWorkspace(t *testing.T) string {
	t.Helper()
	workspacePath := t.TempDir()
	writeBuilderRuntimeWorkspaceFiles(t, workspacePath, map[string]string{
		"lib/repositories/task_repository.dart": "abstract class TaskRepository {}\n",
		"lib/controllers/task_form_controller.dart": strings.Join([]string{
			"class TaskFormController {",
			"  const TaskFormController({required this.taskRepository});",
			"  final TaskRepository taskRepository;",
			"}",
		}, "\n") + "\n",
		"lib/views/task_form_page.dart": strings.Join([]string{
			"class TaskFormPage {",
			"  const TaskFormPage({required this.taskRepository, this.initialTask});",
			"  final TaskRepository taskRepository;",
			"  final Object? initialTask;",
			"}",
		}, "\n") + "\n",
	})
	return workspacePath
}

func createBuilderRuntimeCustomOverviewSurfaceWorkspace(t *testing.T) string {
	t.Helper()
	workspacePath := t.TempDir()
	writeBuilderRuntimeWorkspaceFiles(t, workspacePath, map[string]string{
		"lib/repositories/task_repository.dart": "abstract class TaskRepository {}\n",
		"lib/controllers/task_overview_controller.dart": strings.Join([]string{
			"class TaskOverviewController {",
			"  const TaskOverviewController({required this.taskRepository});",
			"  final TaskRepository taskRepository;",
			"}",
		}, "\n") + "\n",
		"lib/views/task_overview_page.dart": strings.Join([]string{
			"class TaskOverviewPage {",
			"  const TaskOverviewPage({required this.controller, required this.onCreateTask, required this.onViewAllTasks});",
			"  final TaskOverviewController controller;",
			"  final Future<void> Function() onCreateTask;",
			"  final Future<void> Function() onViewAllTasks;",
			"}",
		}, "\n") + "\n",
	})
	return workspacePath
}

func createBuilderRuntimeCustomDetailSurfaceWorkspace(t *testing.T) string {
	t.Helper()
	workspacePath := t.TempDir()
	writeBuilderRuntimeWorkspaceFiles(t, workspacePath, map[string]string{
		"lib/repositories/task_repository.dart": "abstract class TaskRepository {}\n",
		"lib/controllers/task_collection_controller.dart": strings.Join([]string{
			"class TaskCollectionController {",
			"  TaskCollectionController({required TaskRepository taskRepository});",
			"  Future<void> refresh() async {}",
			"  Future<void> updateRecord(dynamic record) async {}",
			"}",
		}, "\n") + "\n",
		"lib/views/task_form_page.dart": strings.Join([]string{
			"class TaskFormPage {",
			"  const TaskFormPage({required this.taskRepository, this.initialTask});",
			"  final TaskRepository taskRepository;",
			"  final Object? initialTask;",
			"}",
		}, "\n") + "\n",
		"lib/views/task_detail_page.dart": strings.Join([]string{
			"class TaskDetailPage {",
			"  const TaskDetailPage({required this.task, required this.onEdit});",
			"  final Object task;",
			"  final Future<void> Function(Object task) onEdit;",
			"}",
		}, "\n") + "\n",
	})
	return workspacePath
}

func TestBuilderRuntimeOpenLitePrimaryRecordSignalsFollowCustomCollectionModel(t *testing.T) {
	workspacePath := createBuilderRuntimeCustomCollectionModelWorkspace(t, strings.Join([]string{
		"class Task {",
		"  const Task({required this.title});",
		"  final String title;",
		"}",
	}, "\n")+"\n")

	if got := builderRuntimeOpenLitePrimaryRecordModelPath(workspacePath); got != "lib/models/task.dart" {
		t.Fatalf("builderRuntimeOpenLitePrimaryRecordModelPath() = %q, want lib/models/task.dart", got)
	}
	if got := builderRuntimeOpenLitePrimaryRecordTypeName(workspacePath); got != "Task" {
		t.Fatalf("builderRuntimeOpenLitePrimaryRecordTypeName() = %q, want Task", got)
	}
	if builderRuntimeOpenLiteShouldDropTitleField(workspacePath) {
		t.Fatal("builderRuntimeOpenLiteShouldDropTitleField() should keep title when custom collection model still exports title")
	}
	if !builderRuntimeOpenLiteShouldDropCategoryField(workspacePath) {
		t.Fatal("builderRuntimeOpenLiteShouldDropCategoryField() should drop category when custom collection model omits category")
	}
	if builderRuntimeOpenLiteRecordModelHasStatus(workspacePath) {
		t.Fatal("builderRuntimeOpenLiteRecordModelHasStatus() should report false when custom collection model omits status")
	}
}

func TestBuilderRuntimeOpenLitePrimaryRecordSignalsDetectTitlelessCustomCollectionModel(t *testing.T) {
	workspacePath := createBuilderRuntimeCustomCollectionModelWorkspace(t, strings.Join([]string{
		"class Task {",
		"  const Task({required this.note});",
		"  final String note;",
		"}",
	}, "\n")+"\n")

	if got := builderRuntimeOpenLitePrimaryRecordTypeName(workspacePath); got != "Task" {
		t.Fatalf("builderRuntimeOpenLitePrimaryRecordTypeName() = %q, want Task", got)
	}
	if !builderRuntimeOpenLiteShouldDropTitleField(workspacePath) {
		t.Fatal("builderRuntimeOpenLiteShouldDropTitleField() should detect missing title on custom collection model")
	}
}

func TestBuilderRuntimeOpenLitePrimaryRecordSignalsPreferCurrentCollectionModelOverStaleRecordPath(t *testing.T) {
	workspacePath := createBuilderRuntimeCustomCollectionModelWorkspace(t, strings.Join([]string{
		"class Task {",
		"  const Task({required this.title});",
		"  final String title;",
		"}",
	}, "\n")+"\n")
	writeBuilderRuntimeWorkspaceFiles(t, workspacePath, map[string]string{
		"lib/models/record.dart": strings.Join([]string{
			"class AppRecord {",
			"  const AppRecord({required this.updatedAt});",
			"  final DateTime updatedAt;",
			"}",
		}, "\n") + "\n",
	})

	if got := builderRuntimeOpenLitePrimaryRecordModelPath(workspacePath); got != "lib/models/task.dart" {
		t.Fatalf("builderRuntimeOpenLitePrimaryRecordModelPath() = %q, want lib/models/task.dart even when stale record.dart still exists", got)
	}
	if got := builderRuntimeOpenLitePrimaryRecordTypeName(workspacePath); got != "Task" {
		t.Fatalf("builderRuntimeOpenLitePrimaryRecordTypeName() = %q, want Task when collection surface already points at task.dart", got)
	}
	if builderRuntimeOpenLiteShouldDropTitleField(workspacePath) {
		t.Fatal("builderRuntimeOpenLiteShouldDropTitleField() should keep title when current collection model still exports title, even if stale record.dart remains")
	}
}

func TestBuilderRuntimeOpenLiteCanonicalNoFilterListPageUsesCustomCollectionClassName(t *testing.T) {
	workspacePath := createBuilderRuntimeCustomCollectionModelWorkspace(t, strings.Join([]string{
		"class Task {",
		"  const Task({required this.title});",
		"  final String title;",
		"}",
	}, "\n")+"\n")

	content := builderRuntimeOpenLiteCanonicalNoFilterListPage(workspacePath)
	for _, want := range []string{
		"class TaskCollectionPage extends StatelessWidget {",
		"  const TaskCollectionPage({",
		"  final TaskCollectionController controller;",
		"  final Future<void> Function() onCreateTask;",
		"  final Future<void> Function(Task task) onOpenTaskDetail;",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("builderRuntimeOpenLiteCanonicalNoFilterListPage() missing custom collection marker %q: %q", want, content)
		}
	}
	if strings.Contains(content, "  const RecordListPage({") {
		t.Fatalf("builderRuntimeOpenLiteCanonicalNoFilterListPage() should not keep stale default constructor name: %q", content)
	}
}

func TestBuilderRuntimeOpenLiteCanonicalNoFilterListPageUsesFieldSemanticsAndCustomStatusCopy(t *testing.T) {
	workspacePath := createBuilderRuntimeCustomCollectionModelWorkspace(t, strings.Join([]string{
		"enum TaskStatus { todo, doing, done }",
		"class Task {",
		"  const Task({required this.headline, required this.group, required this.status});",
		"  final String headline;",
		"  final String group;",
		"  final TaskStatus status;",
		"}",
	}, "\n")+"\n")
	writeBuilderRuntimeWorkspaceFiles(t, workspacePath, map[string]string{
		"lib/template/open_lite_copy.dart": strings.Join([]string{
			"import '../models/task.dart';",
			"",
			"const openLiteCopy = OpenLiteCopy();",
			"",
			"class OpenLiteCopy {",
			"  const OpenLiteCopy();",
			"",
			"  String get listPageTitle => '任务列表';",
			"  String get listEmptyLabel => '暂无任务';",
			"",
			"  String taskStatusLabel(TaskStatus status) {",
			"    switch (status) {",
			"      case TaskStatus.todo:",
			"        return '待办';",
			"      case TaskStatus.doing:",
			"        return '进行中';",
			"      case TaskStatus.done:",
			"        return '已完成';",
			"    }",
			"  }",
			"}",
		}, "\n") + "\n",
	})

	content := builderRuntimeOpenLiteCanonicalNoFilterListPage(workspacePath)
	for _, want := range []string{
		"record.headline,",
		"record.group",
		"openLiteCopy.taskStatusLabel(record.status)",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("builderRuntimeOpenLiteCanonicalNoFilterListPage() missing field semantics marker %q: %q", want, content)
		}
	}
	for _, forbidden := range []string{
		"record.title",
		"record.category",
		"openLiteCopy.statusLabel(record.status)",
	} {
		if strings.Contains(content, forbidden) {
			t.Fatalf("builderRuntimeOpenLiteCanonicalNoFilterListPage() should not keep stale field semantics marker %q: %q", forbidden, content)
		}
	}
}

func TestBuilderRuntimeOpenLiteCollectionSurfaceRegistryTracksFieldSemantics(t *testing.T) {
	workspacePath := createBuilderRuntimeCustomCollectionModelWorkspace(t, strings.Join([]string{
		"enum TaskStatus { todo, doing, done }",
		"class Task {",
		"  const Task({required this.taskId, required this.headline, required this.group, required this.status, this.note});",
		"  final String taskId;",
		"  final String headline;",
		"  final String group;",
		"  final TaskStatus status;",
		"  final String? note;",
		"}",
	}, "\n")+"\n")
	writeBuilderRuntimeWorkspaceFiles(t, workspacePath, map[string]string{
		"lib/template/open_lite_copy.dart": strings.Join([]string{
			"import '../models/task.dart';",
			"",
			"const openLiteCopy = OpenLiteCopy();",
			"",
			"class OpenLiteCopy {",
			"  const OpenLiteCopy();",
			"",
			"  String taskStatusLabel(TaskStatus status) {",
			"    switch (status) {",
			"      case TaskStatus.todo:",
			"        return '待办';",
			"      case TaskStatus.doing:",
			"        return '进行中';",
			"      case TaskStatus.done:",
			"        return '已完成';",
			"    }",
			"  }",
			"}",
		}, "\n") + "\n",
	})

	registry := builderRuntimeOpenLiteCollectionSurfaceRegistry(workspacePath)
	if got := registry.view.fieldSemantics.identifierField; got != "taskId" {
		t.Fatalf("builderRuntimeOpenLiteCollectionSurfaceRegistry().view.fieldSemantics.identifierField = %q, want taskId", got)
	}
	if got := registry.view.fieldSemantics.primaryTextField; got != "headline" {
		t.Fatalf("builderRuntimeOpenLiteCollectionSurfaceRegistry().view.fieldSemantics.primaryTextField = %q, want headline", got)
	}
	if got := registry.view.fieldSemantics.secondaryTextField; got != "group" {
		t.Fatalf("builderRuntimeOpenLiteCollectionSurfaceRegistry().view.fieldSemantics.secondaryTextField = %q, want group", got)
	}
	if got := registry.view.fieldSemantics.statusField; got != "status" {
		t.Fatalf("builderRuntimeOpenLiteCollectionSurfaceRegistry().view.fieldSemantics.statusField = %q, want status", got)
	}
	if got := registry.view.fieldSemantics.statusEnumType; got != "TaskStatus" {
		t.Fatalf("builderRuntimeOpenLiteCollectionSurfaceRegistry().view.fieldSemantics.statusEnumType = %q, want TaskStatus", got)
	}
	if got := registry.view.fieldSemantics.statusCopyLabelMethodName; got != "taskStatusLabel" {
		t.Fatalf("builderRuntimeOpenLiteCollectionSurfaceRegistry().view.fieldSemantics.statusCopyLabelMethodName = %q, want taskStatusLabel", got)
	}
	if got := registry.view.fieldSemantics.noteField; got != "note" {
		t.Fatalf("builderRuntimeOpenLiteCollectionSurfaceRegistry().view.fieldSemantics.noteField = %q, want note", got)
	}
}

func TestBuilderRuntimeOpenLiteCollectionFieldSemanticsPreferDomainModelRoles(t *testing.T) {
	workspacePath := createBuilderRuntimeWorkspaceWithDomainModel(t, strings.Join([]string{
		"{",
		"  \"entities\": [",
		"    {",
		"      \"source\": \"local_storage\",",
		"      \"fields\": [",
		"        {\"name\": \"movie_record_id\", \"role\": \"identifier\"},",
		"        {\"name\": \"movie_title\", \"role\": \"primary_text\"},",
		"        {\"name\": \"genre\", \"role\": \"secondary_text\"},",
		"        {\"name\": \"watch_status\", \"role\": \"status\"},",
		"        {\"name\": \"next_due_at\", \"role\": \"due_date\"},",
		"        {\"name\": \"review\", \"role\": \"note\"}",
		"      ]",
		"    }",
		"  ]",
		"}",
	}, "\n"), map[string]string{
		"lib/models/record.dart": strings.Join([]string{
			"enum MovieRecordWatchStatus { planned, watching, watched }",
			"class MovieRecord {",
			"  const MovieRecord({required this.movieRecordId, required this.movieTitle, required this.genre, required this.watchStatus, required this.nextDueAt, required this.review});",
			"  final String movieRecordId;",
			"  final String movieTitle;",
			"  final String genre;",
			"  final MovieRecordWatchStatus watchStatus;",
			"  final DateTime nextDueAt;",
			"  final String review;",
			"}",
		}, "\n") + "\n",
	})

	semantics := builderRuntimeOpenLiteCollectionFieldSemantics(workspacePath)
	checks := map[string]string{
		"identifierField":    semantics.identifierField,
		"primaryTextField":   semantics.primaryTextField,
		"secondaryTextField": semantics.secondaryTextField,
		"statusField":        semantics.statusField,
		"statusEnumType":     semantics.statusEnumType,
		"timeField":          semantics.timeField,
		"noteField":          semantics.noteField,
	}
	wants := map[string]string{
		"identifierField":    "movieRecordId",
		"primaryTextField":   "movieTitle",
		"secondaryTextField": "genre",
		"statusField":        "watchStatus",
		"statusEnumType":     "MovieRecordWatchStatus",
		"timeField":          "nextDueAt",
		"noteField":          "review",
	}
	for name, want := range wants {
		if got := checks[name]; got != want {
			t.Fatalf("builderRuntimeOpenLiteCollectionFieldSemantics().%s = %q, want %q", name, got, want)
		}
	}
}

func TestBuilderRuntimeOpenLiteGenericOverviewUsesSchemaStatusEnum(t *testing.T) {
	workspacePath := t.TempDir()
	writeBuilderRuntimeWorkspaceFiles(t, workspacePath, map[string]string{
		"lib/models/record.dart": strings.Join([]string{
			"enum CourseAssignmentStatus { todo, inProgress, done }",
			"class CourseAssignment {",
			"  const CourseAssignment({required this.assignmentId, required this.assignmentTitle, required this.status});",
			"  final String assignmentId;",
			"  final String assignmentTitle;",
			"  final CourseAssignmentStatus status;",
			"}",
		}, "\n") + "\n",
	})

	content := builderRuntimeOpenLiteCanonicalGenericOverviewPage(workspacePath)
	for _, want := range []string{
		"import '../models/record.dart';",
		"r.status == CourseAssignmentStatus.done",
		"title: Text(r.assignmentTitle)",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("builderRuntimeOpenLiteCanonicalGenericOverviewPage() missing schema marker %q: %q", want, content)
		}
	}
	if strings.Contains(content, "RecordStatus.done") {
		t.Fatalf("builderRuntimeOpenLiteCanonicalGenericOverviewPage() should not hard-code RecordStatus.done: %q", content)
	}
}

func TestBuilderRuntimeOpenLiteGenericOverviewOmitsDoneCountWithoutStatus(t *testing.T) {
	workspacePath := t.TempDir()
	writeBuilderRuntimeWorkspaceFiles(t, workspacePath, map[string]string{
		"lib/models/record.dart": strings.Join([]string{
			"class PetVaccineRecord {",
			"  const PetVaccineRecord({required this.vaccineRecordId, required this.petName, required this.vaccineName});",
			"  final String vaccineRecordId;",
			"  final String petName;",
			"  final String vaccineName;",
			"}",
		}, "\n") + "\n",
	})

	content := builderRuntimeOpenLiteCanonicalGenericOverviewPage(workspacePath)
	for _, forbidden := range []string{"doneCount", "doneFilterLabel", "import '../models/record.dart';"} {
		if strings.Contains(content, forbidden) {
			t.Fatalf("builderRuntimeOpenLiteCanonicalGenericOverviewPage() should omit status-only marker %q: %q", forbidden, content)
		}
	}
	if !strings.Contains(content, "const _SummaryCard({required this.totalCount});") {
		t.Fatalf("builderRuntimeOpenLiteCanonicalGenericOverviewPage() should use statusless SummaryCard constructor: %q", content)
	}
}

func TestBuilderRuntimeOpenLiteGenericInspectionUsesDomainRoleNote(t *testing.T) {
	workspacePath := createBuilderRuntimeWorkspaceWithDomainModel(t, strings.Join([]string{
		"{",
		"  \"entities\": [",
		"    {",
		"      \"source\": \"local_storage\",",
		"      \"fields\": [",
		"        {\"name\": \"movie_record_id\", \"role\": \"identifier\"},",
		"        {\"name\": \"movie_title\", \"role\": \"primary_text\"},",
		"        {\"name\": \"genre\", \"role\": \"secondary_text\"},",
		"        {\"name\": \"watch_status\", \"role\": \"status\"},",
		"        {\"name\": \"review\", \"role\": \"note\"}",
		"      ]",
		"    }",
		"  ]",
		"}",
	}, "\n"), map[string]string{
		"lib/models/record.dart": strings.Join([]string{
			"enum MovieRecordWatchStatus { planned, watching, watched }",
			"class MovieRecord {",
			"  const MovieRecord({required this.movieRecordId, required this.movieTitle, required this.genre, required this.watchStatus, required this.review});",
			"  final String movieRecordId;",
			"  final String movieTitle;",
			"  final String genre;",
			"  final MovieRecordWatchStatus watchStatus;",
			"  final String review;",
			"}",
		}, "\n") + "\n",
	})

	content := builderRuntimeOpenLiteCanonicalGenericInspectionPageWithDelete(workspacePath, true)
	for _, want := range []string{
		"_InfoTile(label: openLiteCopy.detailCategoryLabel, value: record.genre)",
		"_InfoTile(label: openLiteCopy.detailNoteLabel, value: record.review.trim().isEmpty ? openLiteCopy.emptyNoteLabel : record.review.trim(), multiline: true)",
		"this.multiline = false",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("builderRuntimeOpenLiteCanonicalGenericInspectionPageWithDelete() missing role-aware detail marker %q: %q", want, content)
		}
	}
	if strings.Contains(content, "record.category") || strings.Contains(content, "record.note") {
		t.Fatalf("builderRuntimeOpenLiteCanonicalGenericInspectionPageWithDelete() should not fall back to literal category/note fields: %q", content)
	}
}

func TestBuilderRuntimeOpenLiteGenericAppEntryDeleteUsesSchemaIdentifier(t *testing.T) {
	workspacePath := t.TempDir()
	writeBuilderRuntimeWorkspaceFiles(t, workspacePath, map[string]string{
		"lib/models/record.dart": strings.Join([]string{
			"class CourseAssignment {",
			"  const CourseAssignment({required this.assignmentId, required this.assignmentTitle});",
			"  final String assignmentId;",
			"  final String assignmentTitle;",
			"}",
		}, "\n") + "\n",
	})

	content := builderRuntimeOpenLiteCanonicalGenericAppEntryWithDelete(workspacePath, true)
	if !strings.Contains(content, "await _overviewController.deleteRecord(record.assignmentId);") {
		t.Fatalf("builderRuntimeOpenLiteCanonicalGenericAppEntryWithDelete() should delete by schema identifier: %q", content)
	}
	if strings.Contains(content, "record.recordId") {
		t.Fatalf("builderRuntimeOpenLiteCanonicalGenericAppEntryWithDelete() should not hard-code recordId: %q", content)
	}
}

func TestBuilderRuntimeOpenLiteCanonicalNoFilterListControllerUsesCustomCollectionSurfaceContract(t *testing.T) {
	workspacePath := createBuilderRuntimeCustomCollectionModelWorkspace(t, strings.Join([]string{
		"class Task {",
		"  const Task({required this.title});",
		"  final String title;",
		"}",
	}, "\n")+"\n")

	content := builderRuntimeOpenLiteCanonicalNoFilterListController(workspacePath)
	for _, want := range []string{
		"import '../models/task.dart';",
		"import '../repositories/task_repository.dart';",
		"class TaskCollectionController extends ChangeNotifier {",
		"TaskCollectionController({required TaskRepository taskRepository})",
		"final TaskRepository _repository;",
		"List<Task> _records = [];",
		"Future<void> addRecord(Task record) async {",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("builderRuntimeOpenLiteCanonicalNoFilterListController() missing custom collection controller marker %q: %q", want, content)
		}
	}
	for _, forbidden := range []string{
		"import '../models/record.dart';",
		"import '../repositories/record_repository.dart';",
		"class RecordListController extends ChangeNotifier {",
		"List<TodoItem> _records = [];",
	} {
		if strings.Contains(content, forbidden) {
			t.Fatalf("builderRuntimeOpenLiteCanonicalNoFilterListController() should not keep stale default list controller marker %q: %q", forbidden, content)
		}
	}
}

func TestBuilderRuntimeOpenLiteCanonicalNoFilterListControllerUsesCustomRepositoryMethodNames(t *testing.T) {
	workspacePath := t.TempDir()
	writeBuilderRuntimeWorkspaceFiles(t, workspacePath, map[string]string{
		"lib/models/task.dart": strings.Join([]string{
			"class Task {",
			"  const Task({required this.title});",
			"  final String title;",
			"}",
		}, "\n") + "\n",
		"lib/repositories/task_repository.dart": strings.Join([]string{
			"import '../models/task.dart';",
			"",
			"abstract class TaskRepository {",
			"  Future<List<Task>> loadTasks();",
			"  Future<void> addTask(Task task);",
			"  Future<void> updateTask(Task task);",
			"}",
		}, "\n") + "\n",
		"lib/controllers/task_collection_controller.dart": strings.Join([]string{
			"import '../repositories/task_repository.dart';",
			"",
			"class TaskCollectionController {",
			"  TaskCollectionController({required this.taskRepository});",
			"  final TaskRepository taskRepository;",
			"}",
		}, "\n") + "\n",
		"lib/views/task_collection_page.dart": strings.Join([]string{
			"import '../controllers/task_collection_controller.dart';",
			"import '../models/task.dart';",
			"",
			"class TaskCollectionPage {",
			"  const TaskCollectionPage({required this.controller, required this.onCreateTask, required this.onOpenTaskDetail});",
			"  final TaskCollectionController controller;",
			"  final Future<void> Function() onCreateTask;",
			"  final Future<void> Function(Task task) onOpenTaskDetail;",
			"}",
		}, "\n") + "\n",
	})

	content := builderRuntimeOpenLiteCanonicalNoFilterListController(workspacePath)
	for _, want := range []string{
		"_records = await _repository.loadTasks();",
		"await _repository.addTask(record);",
		"await _repository.updateTask(record);",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("builderRuntimeOpenLiteCanonicalNoFilterListController() missing custom repository method marker %q: %q", want, content)
		}
	}
	for _, forbidden := range []string{
		"_records = await _repository.loadRecords();",
		"await _repository.addRecord(record);",
		"await _repository.updateRecord(record);",
	} {
		if strings.Contains(content, forbidden) {
			t.Fatalf("builderRuntimeOpenLiteCanonicalNoFilterListController() should not keep stale default repository method marker %q: %q", forbidden, content)
		}
	}
}

func TestBuilderRuntimePrimaryDetailViewCandidatePrefersPrimaryModelAlignedCustomDetailView(t *testing.T) {
	workspacePath := createBuilderRuntimeCustomCollectionModelWorkspace(t, strings.Join([]string{
		"class Task {",
		"  const Task({required this.title});",
		"  final String title;",
		"}",
	}, "\n")+"\n")
	writeBuilderRuntimeWorkspaceFiles(t, workspacePath, map[string]string{
		"lib/views/record_detail_page.dart": strings.Join([]string{
			"class RecordDetailPage {",
			"  const RecordDetailPage({required this.record, required this.onEdit});",
			"  final Object record;",
			"  final Future<void> Function(Object record) onEdit;",
			"}",
		}, "\n") + "\n",
		"lib/views/task_detail_page.dart": strings.Join([]string{
			"class TaskDetailPage {",
			"  const TaskDetailPage({required this.task, required this.onEdit});",
			"  final Object task;",
			"  final Future<void> Function(Object task) onEdit;",
			"}",
		}, "\n") + "\n",
	})

	candidate := builderRuntimePrimaryDetailViewCandidate(workspacePath)
	if candidate.path != "lib/views/task_detail_page.dart" {
		t.Fatalf("builderRuntimePrimaryDetailViewCandidate().path = %q, want lib/views/task_detail_page.dart", candidate.path)
	}
	if candidate.className != "TaskDetailPage" {
		t.Fatalf("builderRuntimePrimaryDetailViewCandidate().className = %q, want TaskDetailPage", candidate.className)
	}
	if candidate.recordParam != "task" {
		t.Fatalf("builderRuntimePrimaryDetailViewCandidate().recordParam = %q, want task", candidate.recordParam)
	}
}

func TestBuilderRuntimePrimaryDetailViewCandidatePrefersUniqueSpecificRecordParamWithoutPrimaryModel(t *testing.T) {
	workspacePath := t.TempDir()
	writeBuilderRuntimeWorkspaceFiles(t, workspacePath, map[string]string{
		"lib/views/record_detail_page.dart": strings.Join([]string{
			"class RecordDetailPage {",
			"  const RecordDetailPage({required this.record, required this.onEdit});",
			"  final Object record;",
			"  final Future<void> Function(Object record) onEdit;",
			"}",
		}, "\n") + "\n",
		"lib/views/task_detail_page.dart": strings.Join([]string{
			"class TaskDetailPage {",
			"  const TaskDetailPage({required this.task, required this.onEdit});",
			"  final Object task;",
			"  final Future<void> Function(Object task) onEdit;",
			"}",
		}, "\n") + "\n",
	})

	candidate := builderRuntimePrimaryDetailViewCandidate(workspacePath)
	if candidate.path != "lib/views/task_detail_page.dart" {
		t.Fatalf("builderRuntimePrimaryDetailViewCandidate().path = %q, want lib/views/task_detail_page.dart when it is the only non-generic recordParam candidate", candidate.path)
	}
	if candidate.recordParam != "task" {
		t.Fatalf("builderRuntimePrimaryDetailViewCandidate().recordParam = %q, want task when no primary model exists", candidate.recordParam)
	}
}

func TestBuilderRuntimePrimaryCollectionSurfaceCandidateAlignsCustomCollectionSurface(t *testing.T) {
	workspacePath := createBuilderRuntimeCustomCollectionModelWorkspace(t, strings.Join([]string{
		"class Task {",
		"  const Task({required this.title});",
		"  final String title;",
		"}",
	}, "\n")+"\n")

	surface := builderRuntimePrimaryCollectionSurfaceCandidate(workspacePath)
	if got := surface.view.className; got != "TaskCollectionPage" {
		t.Fatalf("builderRuntimePrimaryCollectionSurfaceCandidate().view.className = %q, want TaskCollectionPage", got)
	}
	if got := surface.controller.className; got != "TaskCollectionController" {
		t.Fatalf("builderRuntimePrimaryCollectionSurfaceCandidate().controller.className = %q, want TaskCollectionController", got)
	}
	if got := surface.repository.repositoryType; got != "TaskRepository" {
		t.Fatalf("builderRuntimePrimaryCollectionSurfaceCandidate().repository.repositoryType = %q, want TaskRepository", got)
	}
	if got := surface.view.detailCallbackName; got != "onOpenTaskDetail" {
		t.Fatalf("builderRuntimePrimaryCollectionSurfaceCandidate().view.detailCallbackName = %q, want onOpenTaskDetail", got)
	}
}

func TestBuilderRuntimePrimaryCollectionSurfaceCandidatePrefersViewAlignedCustomControllerOverStaleDefaultDeclarationHint(t *testing.T) {
	workspacePath := t.TempDir()
	writeBuilderRuntimeWorkspaceFiles(t, workspacePath, map[string]string{
		"lib/models/task.dart":                            "class Task {\n  const Task({required this.title});\n  final String title;\n}\n",
		"lib/repositories/task_repository.dart":           "abstract class TaskRepository {}\n",
		"lib/controllers/record_list_controller.dart":     "import '../repositories/task_repository.dart';\nclass RecordListController {\n  RecordListController({required TaskRepository repository});\n}\n",
		"lib/controllers/task_collection_controller.dart": "import '../repositories/task_repository.dart';\nclass TaskCollectionController {\n  TaskCollectionController({required TaskRepository taskRepository});\n  Future<void> refresh() async {}\n  Future<void> updateRecord(dynamic record) async {}\n}\n",
		"lib/views/task_collection_page.dart":             "import '../controllers/task_collection_controller.dart';\nimport '../models/task.dart';\nclass TaskCollectionPage {\n  const TaskCollectionPage({required this.controller, required this.onOpenTaskDetail});\n  final TaskCollectionController controller;\n  final Future<void> Function(Task task) onOpenTaskDetail;\n}\n",
	})
	content := strings.Join([]string{
		"import 'controllers/record_list_controller.dart';",
		"import 'views/task_collection_page.dart';",
		"",
		"class TaskLiteApp extends StatelessWidget {",
		"  const TaskLiteApp({super.key, required this.repository});",
		"  final TaskRepository repository;",
		"  @override",
		"  Widget build(BuildContext context) {",
		"    final listController = RecordListController(repository: repository);",
		"    return MaterialApp(",
		"      home: TaskCollectionPage(",
		"        controller: listController,",
		"        onOpenTaskDetail: (task) async {},",
		"      ),",
		"    );",
		"  }",
		"}",
	}, "\n") + "\n"

	surface := builderRuntimePrimaryCollectionSurfaceCandidate(workspacePath, content)
	if got := surface.view.className; got != "TaskCollectionPage" {
		t.Fatalf("builderRuntimePrimaryCollectionSurfaceCandidate().view.className = %q, want TaskCollectionPage", got)
	}
	if got := surface.controller.className; got != "TaskCollectionController" {
		t.Fatalf("builderRuntimePrimaryCollectionSurfaceCandidate().controller.className = %q, want TaskCollectionController when current view contract points to the custom controller", got)
	}
	if got := surface.controller.path; got != "lib/controllers/task_collection_controller.dart" {
		t.Fatalf("builderRuntimePrimaryCollectionSurfaceCandidate().controller.path = %q, want lib/controllers/task_collection_controller.dart when stale default declaration hints are present", got)
	}
}

func TestBuilderRuntimeOpenLiteCollectionSurfaceRegistryAlignsCustomCollectionSurface(t *testing.T) {
	workspacePath := createBuilderRuntimeCustomCollectionModelWorkspace(t, strings.Join([]string{
		"class Task {",
		"  const Task({required this.title});",
		"  final String title;",
		"}",
	}, "\n")+"\n")

	registry := builderRuntimeOpenLiteCollectionSurfaceRegistry(workspacePath)
	if got := registry.controller.resolvedPath; got != "lib/controllers/task_collection_controller.dart" {
		t.Fatalf("builderRuntimeOpenLiteCollectionSurfaceRegistry().controller.resolvedPath = %q, want lib/controllers/task_collection_controller.dart", got)
	}
	if got := registry.controller.resolvedClassName; got != "TaskCollectionController" {
		t.Fatalf("builderRuntimeOpenLiteCollectionSurfaceRegistry().controller.resolvedClassName = %q, want TaskCollectionController", got)
	}
	if got := registry.controller.repositoryImportPath; got != "../repositories/task_repository.dart" {
		t.Fatalf("builderRuntimeOpenLiteCollectionSurfaceRegistry().controller.repositoryImportPath = %q, want ../repositories/task_repository.dart", got)
	}
	if got := registry.controller.modelImportPath; got != "../models/task.dart" {
		t.Fatalf("builderRuntimeOpenLiteCollectionSurfaceRegistry().controller.modelImportPath = %q, want ../models/task.dart", got)
	}
	if got := registry.controller.constructorContract.repositoryParam; got != "taskRepository" {
		t.Fatalf("builderRuntimeOpenLiteCollectionSurfaceRegistry().controller.constructorContract.repositoryParam = %q, want taskRepository", got)
	}
	if got := registry.controller.fallbackMode; got != builderRuntimeOpenLiteSurfaceRegistryFallbackWorkspaceCandidate {
		t.Fatalf("builderRuntimeOpenLiteCollectionSurfaceRegistry().controller.fallbackMode = %q, want workspace_candidate", got)
	}
	if got := registry.controller.resolutionSource; got != builderRuntimeOpenLiteSurfaceRegistryResolutionWorkspaceCandidate {
		t.Fatalf("builderRuntimeOpenLiteCollectionSurfaceRegistry().controller.resolutionSource = %q, want workspace_candidate", got)
	}
	if got := registry.view.resolvedPath; got != "lib/views/task_collection_page.dart" {
		t.Fatalf("builderRuntimeOpenLiteCollectionSurfaceRegistry().view.resolvedPath = %q, want lib/views/task_collection_page.dart", got)
	}
	if got := registry.view.resolvedClassName; got != "TaskCollectionPage" {
		t.Fatalf("builderRuntimeOpenLiteCollectionSurfaceRegistry().view.resolvedClassName = %q, want TaskCollectionPage", got)
	}
	if got := registry.view.controllerImportPath; got != "../controllers/task_collection_controller.dart" {
		t.Fatalf("builderRuntimeOpenLiteCollectionSurfaceRegistry().view.controllerImportPath = %q, want ../controllers/task_collection_controller.dart", got)
	}
	if got := registry.view.modelImportPath; got != "../models/task.dart" {
		t.Fatalf("builderRuntimeOpenLiteCollectionSurfaceRegistry().view.modelImportPath = %q, want ../models/task.dart", got)
	}
	if got := registry.view.constructorContract.detailCallbackName; got != "onOpenTaskDetail" {
		t.Fatalf("builderRuntimeOpenLiteCollectionSurfaceRegistry().view.constructorContract.detailCallbackName = %q, want onOpenTaskDetail", got)
	}
	if got := registry.view.constructorContract.createCallbackName; got != "onCreateTask" {
		t.Fatalf("builderRuntimeOpenLiteCollectionSurfaceRegistry().view.constructorContract.createCallbackName = %q, want onCreateTask", got)
	}
	if got := registry.view.fallbackMode; got != builderRuntimeOpenLiteSurfaceRegistryFallbackWorkspaceCandidate {
		t.Fatalf("builderRuntimeOpenLiteCollectionSurfaceRegistry().view.fallbackMode = %q, want workspace_candidate", got)
	}
	if got := registry.view.resolutionSource; got != builderRuntimeOpenLiteSurfaceRegistryResolutionWorkspaceCandidate {
		t.Fatalf("builderRuntimeOpenLiteCollectionSurfaceRegistry().view.resolutionSource = %q, want workspace_candidate", got)
	}
	if !registry.view.capabilityFlags.supportsCreate {
		t.Fatal("builderRuntimeOpenLiteCollectionSurfaceRegistry().view.capabilityFlags.supportsCreate should be true for custom collection page")
	}
	if !registry.view.capabilityFlags.supportsDetail {
		t.Fatal("builderRuntimeOpenLiteCollectionSurfaceRegistry().view.capabilityFlags.supportsDetail should be true for custom collection page")
	}
}

func TestBuilderRuntimePrimaryOverviewSurfaceCandidatePrefersViewAlignedCustomControllerOverStaleDefaultDeclarationHint(t *testing.T) {
	workspacePath := t.TempDir()
	writeBuilderRuntimeWorkspaceFiles(t, workspacePath, map[string]string{
		"lib/repositories/task_repository.dart":         "abstract class TaskRepository {}\n",
		"lib/controllers/home_controller.dart":          "import '../repositories/task_repository.dart';\nclass HomeController {\n  HomeController({required TaskRepository repository});\n}\n",
		"lib/controllers/task_overview_controller.dart": "import '../repositories/task_repository.dart';\nclass TaskOverviewController {\n  TaskOverviewController({required TaskRepository taskRepository});\n}\n",
		"lib/views/task_overview_page.dart":             "import '../controllers/task_overview_controller.dart';\nclass TaskOverviewPage { const TaskOverviewPage({required this.controller, required this.onCreateTask, required this.onViewAllTasks}); final TaskOverviewController controller; final Future<void> Function() onCreateTask; final Future<void> Function() onViewAllTasks; }\n",
	})
	content := strings.Join([]string{
		"import 'controllers/home_controller.dart';",
		"import 'views/task_overview_page.dart';",
		"",
		"Future<void> _noop() async {}",
		"",
		"class TaskLiteApp extends StatelessWidget {",
		"  const TaskLiteApp({super.key, required this.repository});",
		"  final TaskRepository repository;",
		"  @override",
		"  Widget build(BuildContext context) {",
		"    return MaterialApp(",
		"      home: TaskOverviewPage(",
		"        controller: HomeController(repository: repository),",
		"        onCreateTask: _noop,",
		"        onViewAllTasks: _noop,",
		"      ),",
		"    );",
		"  }",
		"}",
	}, "\n") + "\n"

	surface := builderRuntimePrimaryOverviewSurfaceCandidate(workspacePath, content)
	if got := surface.view.className; got != "TaskOverviewPage" {
		t.Fatalf("builderRuntimePrimaryOverviewSurfaceCandidate().view.className = %q, want TaskOverviewPage", got)
	}
	if got := surface.controller.className; got != "TaskOverviewController" {
		t.Fatalf("builderRuntimePrimaryOverviewSurfaceCandidate().controller.className = %q, want TaskOverviewController when current view contract points to the custom controller", got)
	}
	if got := surface.controller.path; got != "lib/controllers/task_overview_controller.dart" {
		t.Fatalf("builderRuntimePrimaryOverviewSurfaceCandidate().controller.path = %q, want lib/controllers/task_overview_controller.dart when stale default HomeController hints are present", got)
	}
}

func TestBuilderRuntimePrimaryMutationSurfaceCandidateAlignsCustomMutationSurface(t *testing.T) {
	workspacePath := createBuilderRuntimeCustomMutationSurfaceWorkspace(t)

	surface := builderRuntimePrimaryMutationSurfaceCandidate(workspacePath)
	if got := surface.view.className; got != "TaskFormPage" {
		t.Fatalf("builderRuntimePrimaryMutationSurfaceCandidate().view.className = %q, want TaskFormPage", got)
	}
	if got := surface.view.repositoryParam; got != "taskRepository" {
		t.Fatalf("builderRuntimePrimaryMutationSurfaceCandidate().view.repositoryParam = %q, want taskRepository", got)
	}
	if got := surface.view.initialParam; got != "initialTask" {
		t.Fatalf("builderRuntimePrimaryMutationSurfaceCandidate().view.initialParam = %q, want initialTask", got)
	}
	if got := surface.controller.className; got != "TaskFormController" {
		t.Fatalf("builderRuntimePrimaryMutationSurfaceCandidate().controller.className = %q, want TaskFormController", got)
	}
	if got := surface.controller.repositoryParam; got != "taskRepository" {
		t.Fatalf("builderRuntimePrimaryMutationSurfaceCandidate().controller.repositoryParam = %q, want taskRepository", got)
	}
	if got := surface.repository.repositoryType; got != "TaskRepository" {
		t.Fatalf("builderRuntimePrimaryMutationSurfaceCandidate().repository.repositoryType = %q, want TaskRepository", got)
	}
}

func TestBuilderRuntimePrimaryMutationSurfaceCandidatePrefersViewAlignedCustomControllerOverStaleDefaultDeclarationHint(t *testing.T) {
	workspacePath := t.TempDir()
	writeBuilderRuntimeWorkspaceFiles(t, workspacePath, map[string]string{
		"lib/repositories/task_repository.dart":       "abstract class TaskRepository {}\n",
		"lib/controllers/record_form_controller.dart": "import '../repositories/task_repository.dart';\nclass RecordFormController {\n  RecordFormController({required TaskRepository repository});\n}\n",
		"lib/controllers/task_form_controller.dart":   "import '../repositories/task_repository.dart';\nclass TaskFormController {\n  TaskFormController({required TaskRepository taskRepository});\n}\n",
		"lib/views/task_form_page.dart":               "import '../repositories/task_repository.dart';\nclass TaskFormPage { const TaskFormPage({required this.taskRepository, this.initialTask}); final TaskRepository taskRepository; final Object? initialTask; }\n",
	})
	content := strings.Join([]string{
		"import 'controllers/record_form_controller.dart';",
		"import 'views/task_form_page.dart';",
		"",
		"class TaskLiteApp extends StatelessWidget {",
		"  const TaskLiteApp({super.key, required this.repository, required this.task});",
		"  final TaskRepository repository;",
		"  final Object task;",
		"  @override",
		"  Widget build(BuildContext context) {",
		"    final formController = RecordFormController(repository: repository);",
		"    return TaskFormPage(taskRepository: repository, initialTask: task);",
		"  }",
		"}",
	}, "\n") + "\n"

	surface := builderRuntimePrimaryMutationSurfaceCandidate(workspacePath, content)
	if got := surface.view.className; got != "TaskFormPage" {
		t.Fatalf("builderRuntimePrimaryMutationSurfaceCandidate().view.className = %q, want TaskFormPage", got)
	}
	if got := surface.controller.className; got != "TaskFormController" {
		t.Fatalf("builderRuntimePrimaryMutationSurfaceCandidate().controller.className = %q, want TaskFormController when current view contract points to the custom controller", got)
	}
	if got := surface.controller.path; got != "lib/controllers/task_form_controller.dart" {
		t.Fatalf("builderRuntimePrimaryMutationSurfaceCandidate().controller.path = %q, want lib/controllers/task_form_controller.dart when stale default RecordFormController hints are present", got)
	}
}

func TestBuilderRuntimePrimaryOverviewSurfaceCandidateAlignsCustomOverviewSurface(t *testing.T) {
	workspacePath := createBuilderRuntimeCustomOverviewSurfaceWorkspace(t)

	surface := builderRuntimePrimaryOverviewSurfaceCandidate(workspacePath)
	if got := surface.view.className; got != "TaskOverviewPage" {
		t.Fatalf("builderRuntimePrimaryOverviewSurfaceCandidate().view.className = %q, want TaskOverviewPage", got)
	}
	if got := surface.view.controllerParam; got != "controller" {
		t.Fatalf("builderRuntimePrimaryOverviewSurfaceCandidate().view.controllerParam = %q, want controller", got)
	}
	if got := surface.controller.className; got != "TaskOverviewController" {
		t.Fatalf("builderRuntimePrimaryOverviewSurfaceCandidate().controller.className = %q, want TaskOverviewController", got)
	}
	if got := surface.controller.repositoryParam; got != "taskRepository" {
		t.Fatalf("builderRuntimePrimaryOverviewSurfaceCandidate().controller.repositoryParam = %q, want taskRepository", got)
	}
	if got := surface.repository.repositoryType; got != "TaskRepository" {
		t.Fatalf("builderRuntimePrimaryOverviewSurfaceCandidate().repository.repositoryType = %q, want TaskRepository", got)
	}
}

func TestBuilderRuntimeOpenLiteOverviewSurfaceRegistryAlignsCustomOverviewSurface(t *testing.T) {
	workspacePath := createBuilderRuntimeCustomOverviewSurfaceWorkspace(t)

	registry := builderRuntimeOpenLiteOverviewSurfaceRegistry(workspacePath)
	if got := registry.controller.resolvedPath; got != "lib/controllers/task_overview_controller.dart" {
		t.Fatalf("builderRuntimeOpenLiteOverviewSurfaceRegistry().controller.resolvedPath = %q, want lib/controllers/task_overview_controller.dart", got)
	}
	if got := registry.controller.resolvedClassName; got != "TaskOverviewController" {
		t.Fatalf("builderRuntimeOpenLiteOverviewSurfaceRegistry().controller.resolvedClassName = %q, want TaskOverviewController", got)
	}
	if got := registry.controller.constructorContract.repositoryParam; got != "taskRepository" {
		t.Fatalf("builderRuntimeOpenLiteOverviewSurfaceRegistry().controller.constructorContract.repositoryParam = %q, want taskRepository", got)
	}
	if got := registry.controller.fallbackMode; got != builderRuntimeOpenLiteSurfaceRegistryFallbackWorkspaceCandidate {
		t.Fatalf("builderRuntimeOpenLiteOverviewSurfaceRegistry().controller.fallbackMode = %q, want workspace_candidate", got)
	}
	if got := registry.controller.resolutionSource; got != builderRuntimeOpenLiteSurfaceRegistryResolutionWorkspaceCandidate {
		t.Fatalf("builderRuntimeOpenLiteOverviewSurfaceRegistry().controller.resolutionSource = %q, want workspace_candidate", got)
	}
	if got := registry.view.resolvedPath; got != "lib/views/task_overview_page.dart" {
		t.Fatalf("builderRuntimeOpenLiteOverviewSurfaceRegistry().view.resolvedPath = %q, want lib/views/task_overview_page.dart", got)
	}
	if got := registry.view.resolvedClassName; got != "TaskOverviewPage" {
		t.Fatalf("builderRuntimeOpenLiteOverviewSurfaceRegistry().view.resolvedClassName = %q, want TaskOverviewPage", got)
	}
	if got := registry.view.constructorContract.createCallbackName; got != "onCreateTask" {
		t.Fatalf("builderRuntimeOpenLiteOverviewSurfaceRegistry().view.constructorContract.createCallbackName = %q, want onCreateTask", got)
	}
	if got := registry.view.constructorContract.viewAllCallbackName; got != "onViewAllTasks" {
		t.Fatalf("builderRuntimeOpenLiteOverviewSurfaceRegistry().view.constructorContract.viewAllCallbackName = %q, want onViewAllTasks", got)
	}
	if got := registry.view.fallbackMode; got != builderRuntimeOpenLiteSurfaceRegistryFallbackWorkspaceCandidate {
		t.Fatalf("builderRuntimeOpenLiteOverviewSurfaceRegistry().view.fallbackMode = %q, want workspace_candidate", got)
	}
	if got := registry.view.resolutionSource; got != builderRuntimeOpenLiteSurfaceRegistryResolutionWorkspaceCandidate {
		t.Fatalf("builderRuntimeOpenLiteOverviewSurfaceRegistry().view.resolutionSource = %q, want workspace_candidate", got)
	}
}

func TestBuilderRuntimeOpenLiteOverviewSurfaceRegistryMarksLegacyTemplateFallbackWithoutConcreteSurface(t *testing.T) {
	registry := builderRuntimeOpenLiteOverviewSurfaceRegistry(t.TempDir())
	if got := registry.controller.fallbackMode; got != builderRuntimeOpenLiteSurfaceRegistryFallbackLegacyTemplate {
		t.Fatalf("builderRuntimeOpenLiteOverviewSurfaceRegistry().controller.fallbackMode = %q, want legacy_template", got)
	}
	if got := registry.controller.resolutionSource; got != builderRuntimeOpenLiteSurfaceRegistryResolutionLegacyTemplate {
		t.Fatalf("builderRuntimeOpenLiteOverviewSurfaceRegistry().controller.resolutionSource = %q, want legacy_template", got)
	}
	if got := registry.view.fallbackMode; got != builderRuntimeOpenLiteSurfaceRegistryFallbackLegacyTemplate {
		t.Fatalf("builderRuntimeOpenLiteOverviewSurfaceRegistry().view.fallbackMode = %q, want legacy_template", got)
	}
	if got := registry.view.resolutionSource; got != builderRuntimeOpenLiteSurfaceRegistryResolutionLegacyTemplate {
		t.Fatalf("builderRuntimeOpenLiteOverviewSurfaceRegistry().view.resolutionSource = %q, want legacy_template", got)
	}
}

func TestBuilderRuntimePrimaryDetailSurfaceCandidateAlignsCustomDetailSurface(t *testing.T) {
	workspacePath := createBuilderRuntimeCustomDetailSurfaceWorkspace(t)

	surface := builderRuntimePrimaryDetailSurfaceCandidate(workspacePath)
	if got := surface.view.className; got != "TaskDetailPage" {
		t.Fatalf("builderRuntimePrimaryDetailSurfaceCandidate().view.className = %q, want TaskDetailPage", got)
	}
	if got := surface.view.recordParam; got != "task" {
		t.Fatalf("builderRuntimePrimaryDetailSurfaceCandidate().view.recordParam = %q, want task", got)
	}
	if got := surface.view.editCallbackName; got != "onEdit" {
		t.Fatalf("builderRuntimePrimaryDetailSurfaceCandidate().view.editCallbackName = %q, want onEdit", got)
	}
	if got := surface.mutation.view.className; got != "TaskFormPage" {
		t.Fatalf("builderRuntimePrimaryDetailSurfaceCandidate().mutation.view.className = %q, want TaskFormPage", got)
	}
	if got := surface.controller.className; got != "TaskCollectionController" {
		t.Fatalf("builderRuntimePrimaryDetailSurfaceCandidate().controller.className = %q, want TaskCollectionController", got)
	}
}

func TestBuilderRuntimePrimaryDetailSurfaceCandidatePrefersStructuredCustomSurfaceHintsOverStaleDefaultNoise(t *testing.T) {
	workspacePath := createBuilderRuntimeCustomDetailSurfaceWorkspace(t)
	writeBuilderRuntimeWorkspaceFiles(t, workspacePath, map[string]string{
		"lib/controllers/record_list_controller.dart": strings.Join([]string{
			"class RecordListController {",
			"  RecordListController({required TaskRepository repository});",
			"}",
		}, "\n") + "\n",
		"lib/views/record_form_page.dart": strings.Join([]string{
			"class RecordFormPage {",
			"  const RecordFormPage({required this.repository, this.initialRecord});",
			"  final TaskRepository repository;",
			"  final Object? initialRecord;",
			"}",
		}, "\n") + "\n",
	})
	content := strings.Join([]string{
		"import 'controllers/record_list_controller.dart';",
		"import 'views/record_form_page.dart';",
		"import 'views/task_detail_page.dart';",
		"",
		"class TaskLiteApp extends StatelessWidget {",
		"  const TaskLiteApp({super.key, required this.repository, required this.task});",
		"  final TaskRepository repository;",
		"  final Object task;",
		"  @override",
		"  Widget build(BuildContext context) {",
		"    final listController = RecordListController(repository: repository);",
		"    return TaskDetailPage(",
		"      task: task,",
		"      onEdit: (task) async {},",
		"    );",
		"  }",
		"}",
	}, "\n") + "\n"

	surface := builderRuntimePrimaryDetailSurfaceCandidate(workspacePath, content)
	if got := surface.view.className; got != "TaskDetailPage" {
		t.Fatalf("builderRuntimePrimaryDetailSurfaceCandidate().view.className = %q, want TaskDetailPage", got)
	}
	if got := surface.mutation.view.className; got != "TaskFormPage" {
		t.Fatalf("builderRuntimePrimaryDetailSurfaceCandidate().mutation.view.className = %q, want TaskFormPage when current detail surface points at task-domain edit flow", got)
	}
	if got := surface.controller.className; got != "TaskCollectionController" {
		t.Fatalf("builderRuntimePrimaryDetailSurfaceCandidate().controller.className = %q, want TaskCollectionController when stale default record_form/list noise is present", got)
	}
}

func TestBuilderRuntimeOpenLiteDetailSurfaceRegistryAlignsCustomDetailSurface(t *testing.T) {
	workspacePath := createBuilderRuntimeCustomDetailSurfaceWorkspace(t)

	registry := builderRuntimeOpenLiteDetailSurfaceRegistry(workspacePath)
	if got := registry.view.resolvedPath; got != "lib/views/task_detail_page.dart" {
		t.Fatalf("builderRuntimeOpenLiteDetailSurfaceRegistry().view.resolvedPath = %q, want lib/views/task_detail_page.dart", got)
	}
	if got := registry.view.resolvedClassName; got != "TaskDetailPage" {
		t.Fatalf("builderRuntimeOpenLiteDetailSurfaceRegistry().view.resolvedClassName = %q, want TaskDetailPage", got)
	}
	if got := registry.view.constructorContract.recordParam; got != "task" {
		t.Fatalf("builderRuntimeOpenLiteDetailSurfaceRegistry().view.constructorContract.recordParam = %q, want task", got)
	}
	if got := registry.view.constructorContract.editCallbackName; got != "onEdit" {
		t.Fatalf("builderRuntimeOpenLiteDetailSurfaceRegistry().view.constructorContract.editCallbackName = %q, want onEdit", got)
	}
	if got := registry.view.fallbackMode; got != builderRuntimeOpenLiteSurfaceRegistryFallbackWorkspaceCandidate {
		t.Fatalf("builderRuntimeOpenLiteDetailSurfaceRegistry().view.fallbackMode = %q, want workspace_candidate", got)
	}
	if got := registry.view.resolutionSource; got != builderRuntimeOpenLiteSurfaceRegistryResolutionWorkspaceCandidate {
		t.Fatalf("builderRuntimeOpenLiteDetailSurfaceRegistry().view.resolutionSource = %q, want workspace_candidate", got)
	}
	if got := registry.mutation.resolvedPath; got != "lib/views/task_form_page.dart" {
		t.Fatalf("builderRuntimeOpenLiteDetailSurfaceRegistry().mutation.resolvedPath = %q, want lib/views/task_form_page.dart", got)
	}
	if got := registry.mutation.resolvedClassName; got != "TaskFormPage" {
		t.Fatalf("builderRuntimeOpenLiteDetailSurfaceRegistry().mutation.resolvedClassName = %q, want TaskFormPage", got)
	}
	if got := registry.mutation.fallbackMode; got != builderRuntimeOpenLiteSurfaceRegistryFallbackWorkspaceCandidate {
		t.Fatalf("builderRuntimeOpenLiteDetailSurfaceRegistry().mutation.fallbackMode = %q, want workspace_candidate", got)
	}
	if got := registry.mutation.resolutionSource; got != builderRuntimeOpenLiteSurfaceRegistryResolutionWorkspaceCandidate {
		t.Fatalf("builderRuntimeOpenLiteDetailSurfaceRegistry().mutation.resolutionSource = %q, want workspace_candidate", got)
	}
	if got := registry.controller.resolvedPath; got != "lib/controllers/task_collection_controller.dart" {
		t.Fatalf("builderRuntimeOpenLiteDetailSurfaceRegistry().controller.resolvedPath = %q, want lib/controllers/task_collection_controller.dart", got)
	}
	if got := registry.controller.resolvedClassName; got != "TaskCollectionController" {
		t.Fatalf("builderRuntimeOpenLiteDetailSurfaceRegistry().controller.resolvedClassName = %q, want TaskCollectionController", got)
	}
	if got := registry.controller.fallbackMode; got != builderRuntimeOpenLiteSurfaceRegistryFallbackWorkspaceCandidate {
		t.Fatalf("builderRuntimeOpenLiteDetailSurfaceRegistry().controller.fallbackMode = %q, want workspace_candidate", got)
	}
	if got := registry.controller.resolutionSource; got != builderRuntimeOpenLiteSurfaceRegistryResolutionWorkspaceCandidate {
		t.Fatalf("builderRuntimeOpenLiteDetailSurfaceRegistry().controller.resolutionSource = %q, want workspace_candidate", got)
	}
}

func TestBuilderRuntimeOpenLiteMutationSurfaceRegistryTracksMutationCapabilities(t *testing.T) {
	workspacePath := t.TempDir()
	writeBuilderRuntimeWorkspaceFiles(t, workspacePath, map[string]string{
		"lib/models/task.dart":                  "class Task { const Task({required this.title, this.note = ''}); final String title; final String note; }\n",
		"lib/repositories/task_repository.dart": "abstract class TaskRepository {}\n",
		"lib/controllers/task_form_controller.dart": strings.Join([]string{
			"import 'package:flutter/material.dart';",
			"",
			"class TaskFormController {",
			"  final TextEditingController titleController = TextEditingController();",
			"  final TextEditingController noteController = TextEditingController();",
			"}",
		}, "\n") + "\n",
		"lib/views/task_form_page.dart": strings.Join([]string{
			"class TaskFormPage {",
			"  const TaskFormPage({required this.taskRepository, this.initialTask});",
			"  final TaskRepository taskRepository;",
			"  final Task? initialTask;",
			"}",
		}, "\n") + "\n",
	})

	registry := builderRuntimeOpenLiteMutationSurfaceRegistry(workspacePath)
	if got := registry.view.resolvedPath; got != "lib/views/task_form_page.dart" {
		t.Fatalf("builderRuntimeOpenLiteMutationSurfaceRegistry().view.resolvedPath = %q, want lib/views/task_form_page.dart", got)
	}
	if got := registry.view.constructorContract.initialParam; got != "initialTask" {
		t.Fatalf("builderRuntimeOpenLiteMutationSurfaceRegistry().view.constructorContract.initialParam = %q, want initialTask", got)
	}
	if got := registry.view.fallbackMode; got != builderRuntimeOpenLiteSurfaceRegistryFallbackWorkspaceCandidate {
		t.Fatalf("builderRuntimeOpenLiteMutationSurfaceRegistry().view.fallbackMode = %q, want workspace_candidate", got)
	}
	if got := registry.view.resolutionSource; got != builderRuntimeOpenLiteSurfaceRegistryResolutionWorkspaceCandidate {
		t.Fatalf("builderRuntimeOpenLiteMutationSurfaceRegistry().view.resolutionSource = %q, want workspace_candidate", got)
	}
	if !registry.view.capabilityFlags.supportsEdit {
		t.Fatal("builderRuntimeOpenLiteMutationSurfaceRegistry().view.capabilityFlags.supportsEdit should be true when title field and initial param both exist")
	}
	if got := registry.controller.resolvedPath; got != "lib/controllers/task_form_controller.dart" {
		t.Fatalf("builderRuntimeOpenLiteMutationSurfaceRegistry().controller.resolvedPath = %q, want lib/controllers/task_form_controller.dart", got)
	}
	if got := registry.controller.fallbackMode; got != builderRuntimeOpenLiteSurfaceRegistryFallbackWorkspaceCandidate {
		t.Fatalf("builderRuntimeOpenLiteMutationSurfaceRegistry().controller.fallbackMode = %q, want workspace_candidate", got)
	}
	if got := registry.controller.resolutionSource; got != builderRuntimeOpenLiteSurfaceRegistryResolutionWorkspaceCandidate {
		t.Fatalf("builderRuntimeOpenLiteMutationSurfaceRegistry().controller.resolutionSource = %q, want workspace_candidate", got)
	}
	if !registry.controller.capabilityFlags.hasTitleField {
		t.Fatal("builderRuntimeOpenLiteMutationSurfaceRegistry().controller.capabilityFlags.hasTitleField should detect titleController")
	}
	if !registry.controller.capabilityFlags.hasNoteField {
		t.Fatal("builderRuntimeOpenLiteMutationSurfaceRegistry().controller.capabilityFlags.hasNoteField should detect noteController")
	}
}

func TestBuilderRuntimeOpenLiteMutationSurfaceRegistryTracksControllerDrivenViewConstructorContract(t *testing.T) {
	workspacePath := t.TempDir()
	writeBuilderRuntimeWorkspaceFiles(t, workspacePath, map[string]string{
		"lib/repositories/task_repository.dart": "abstract class TaskRepository {}\n",
		"lib/controllers/task_form_controller.dart": strings.Join([]string{
			"import '../repositories/task_repository.dart';",
			"",
			"class TaskFormController {",
			"  const TaskFormController({required this.taskRepository});",
			"  final TaskRepository taskRepository;",
			"}",
		}, "\n") + "\n",
		"lib/views/task_form_page.dart": strings.Join([]string{
			"import '../controllers/task_form_controller.dart';",
			"",
			"class TaskFormPage {",
			"  const TaskFormPage({required this.formController});",
			"  final TaskFormController formController;",
			"}",
		}, "\n") + "\n",
	})

	registry := builderRuntimeOpenLiteMutationSurfaceRegistry(workspacePath)
	if got := registry.view.resolvedPath; got != "lib/views/task_form_page.dart" {
		t.Fatalf("builderRuntimeOpenLiteMutationSurfaceRegistry().view.resolvedPath = %q, want lib/views/task_form_page.dart", got)
	}
	if got := registry.view.constructorContract.controllerParam; got != "formController" {
		t.Fatalf("builderRuntimeOpenLiteMutationSurfaceRegistry().view.constructorContract.controllerParam = %q, want formController", got)
	}
	if got := registry.view.constructorContract.controllerType; got != "TaskFormController" {
		t.Fatalf("builderRuntimeOpenLiteMutationSurfaceRegistry().view.constructorContract.controllerType = %q, want TaskFormController", got)
	}
	if got := registry.view.fallbackMode; got != builderRuntimeOpenLiteSurfaceRegistryFallbackWorkspaceCandidate {
		t.Fatalf("builderRuntimeOpenLiteMutationSurfaceRegistry().view.fallbackMode = %q, want workspace_candidate", got)
	}
	if got := registry.view.resolutionSource; got != builderRuntimeOpenLiteSurfaceRegistryResolutionWorkspaceCandidate {
		t.Fatalf("builderRuntimeOpenLiteMutationSurfaceRegistry().view.resolutionSource = %q, want workspace_candidate", got)
	}
}

func createBuilderRuntimeProjectTaskTagOverviewWorkspace(t *testing.T) string {
	t.Helper()
	return createBuilderRuntimeRelationRichWorkspace(t, map[string]string{
		"lib/controllers/home_controller.dart": strings.Join([]string{
			"class HomeController {",
			"  HomeController({required dynamic repository});",
			"  Future<void> initialize() async {}",
			"  Future<void> refresh() async {}",
			"  void dispose() {}",
			"}",
		}, "\n") + "\n",
		"lib/controllers/record_form_controller.dart": strings.Join([]string{
			"class RecordFormController {",
			"  RecordFormController({required dynamic repository, List<dynamic> projects = const [], List<dynamic> tags = const [], dynamic initialTask, List<String>? initialTagIds});",
			"  void dispose() {}",
			"}",
		}, "\n") + "\n",
		"lib/controllers/record_list_controller.dart": strings.Join([]string{
			"class RecordListController {",
			"  RecordListController({required dynamic repository});",
			"  Future<void> refresh() async {}",
			"  void dispose() {}",
			"  List<dynamic> get projects => const [];",
			"  List<dynamic> get tags => const [];",
			"  List<dynamic> get visibleTasks => const [];",
			"  bool get isLoading => false;",
			"  bool get hasFilter => false;",
			"}",
		}, "\n") + "\n",
		"lib/views/home_page.dart": strings.Join([]string{
			"class HomePage {",
			"  const HomePage({required this.controller, required this.onCreateTask, required this.onViewAllTasks});",
			"  final dynamic controller;",
			"  final Future<void> Function() onCreateTask;",
			"  final Future<void> Function() onViewAllTasks;",
			"}",
		}, "\n") + "\n",
		"lib/views/record_form_page.dart": strings.Join([]string{
			"class RecordFormPage {",
			"  const RecordFormPage({required this.controller});",
			"  final dynamic controller;",
			"}",
		}, "\n") + "\n",
		"lib/views/record_list_page.dart": strings.Join([]string{
			"import '../models/task.dart';",
			"import '../controllers/record_list_controller.dart';",
			"class RecordListPage {",
			"  const RecordListPage({required this.controller, required this.onCreateRecord, required this.onOpenTaskDetail});",
			"  final dynamic controller;",
			"  final Future<void> Function() onCreateRecord;",
			"  final Future<void> Function(Task task) onOpenTaskDetail;",
			"}",
		}, "\n") + "\n",
		"lib/views/record_detail_page.dart": strings.Join([]string{
			"import '../models/project.dart';",
			"import '../models/tag.dart';",
			"import '../models/task.dart';",
			"class RecordDetailPage {",
			"  const RecordDetailPage({required this.task, required this.projects, required this.tags, required this.taskTags, required this.onEdit});",
			"  final Task task;",
			"  final List<Project> projects;",
			"  final List<Tag> tags;",
			"  final List<String> taskTags;",
			"  final Future<void> Function(Task task) onEdit;",
			"}",
		}, "\n") + "\n",
		"lib/template/open_lite_copy.dart": "class _OpenLiteCopy { String get appTitle => 'app'; }\nconst openLiteCopy = _OpenLiteCopy();\n",
		"lib/repositories/record_repository.dart": strings.Join([]string{
			"abstract class RecordRepository {",
			"  Future<void> init();",
			"  Future<List<dynamic>> loadProjects();",
			"  Future<List<dynamic>> loadTags();",
			"  Future<List<dynamic>> loadTaskTagLinks();",
			"}",
			"class HiveRecordRepository extends RecordRepository {",
			"  @override",
			"  Future<void> init() async {}",
			"  @override",
			"  Future<List<dynamic>> loadProjects() async => const [];",
			"  @override",
			"  Future<List<dynamic>> loadTags() async => const [];",
			"  @override",
			"  Future<List<dynamic>> loadTaskTagLinks() async => const [];",
			"}",
		}, "\n") + "\n",
	})
}

func createBuilderRuntimeInventoryRelationRichWorkspace(t *testing.T, files map[string]string) string {
	t.Helper()
	workspacePath := t.TempDir()
	workspaceFiles := map[string]string{
		"lib/models/inventory_sheet.dart": strings.Join([]string{
			"enum InventorySheetStatus { draft, checking, closed }",
			"class InventorySheet {",
			"  const InventorySheet({required this.sheetId, required this.warehouseId, required this.status, required this.countedOn, this.note});",
			"  final String sheetId;",
			"  final String warehouseId;",
			"  final InventorySheetStatus status;",
			"  final DateTime countedOn;",
			"  final String? note;",
			"}",
		}, "\n") + "\n",
		"lib/models/line_item.dart": strings.Join([]string{
			"class LineItem {",
			"  const LineItem({required this.lineItemId, required this.sheetId, required this.skuId, required this.expectedQty, required this.countedQty, required this.varianceQty});",
			"  final String lineItemId;",
			"  final String sheetId;",
			"  final String skuId;",
			"  final int expectedQty;",
			"  final int countedQty;",
			"  final int varianceQty;",
			"}",
		}, "\n") + "\n",
		"lib/models/sku.dart": strings.Join([]string{
			"class Sku {",
			"  const Sku({required this.skuId, required this.name, required this.category, required this.reorderThreshold});",
			"  final String skuId;",
			"  final String name;",
			"  final String category;",
			"  final int reorderThreshold;",
			"}",
		}, "\n") + "\n",
		"lib/models/warehouse.dart": strings.Join([]string{
			"class Warehouse {",
			"  const Warehouse({required this.warehouseId, required this.name, this.location});",
			"  final String warehouseId;",
			"  final String name;",
			"  final String? location;",
			"}",
		}, "\n") + "\n",
		"lib/models/dashboard_summary.dart": strings.Join([]string{
			"class DashboardSummary {",
			"  const DashboardSummary({required this.warehouseId, required this.openSheetCount, required this.lowStockSkuCount, required this.varianceLineItemCount});",
			"  final String warehouseId;",
			"  final int openSheetCount;",
			"  final int lowStockSkuCount;",
			"  final int varianceLineItemCount;",
			"}",
		}, "\n") + "\n",
		"lib/repositories/record_repository.dart": strings.Join([]string{
			"abstract class RecordRepository {",
			"  Future<List<dynamic>> loadInventorySheets();",
			"  Future<List<dynamic>> loadLineItems();",
			"  Future<List<dynamic>> loadSkus();",
			"  Future<List<dynamic>> loadWarehouses();",
			"}",
		}, "\n") + "\n",
	}
	for relativePath, content := range files {
		workspaceFiles[relativePath] = content
	}
	writeBuilderRuntimeWorkspaceFiles(t, workspacePath, workspaceFiles)
	return workspacePath
}

func TestBuildBuilderRuntimePromptAvoidsDeprecatedFlutterAPIs(t *testing.T) {
	run := runRecord{
		GoalSummary:   "repair flutter ui",
		WorkspacePath: t.TempDir(),
		TaskBundle: []appruns.TaskBundleItem{{
			TaskID:      "task-home",
			TaskType:    appruns.BuilderRuntimeTaskTypeSingleFileEdit,
			TargetPaths: []string{"lib/views/home_page.dart"},
		}},
	}
	roundInput := appruns.RoundInput{
		TaskBundle: run.TaskBundle,
		AllowedPaths: []string{
			"lib/**",
		},
	}
	route := appruns.BuilderRuntimeTaskRoute{
		TaskID:      "task-home",
		TaskType:    appruns.BuilderRuntimeTaskTypeSingleFileEdit,
		RouteSource: "default_model",
	}

	prompt, err := buildBuilderRuntimePrompt(BuilderRuntimePatchRequest{Run: run, RoundInput: roundInput, Route: route})
	if err != nil {
		t.Fatalf("buildBuilderRuntimePrompt() error = %v", err)
	}
	if !strings.Contains(prompt, "keep withOpacity() unless the current workspace already compiles with withValues()") {
		t.Fatalf("prompt missing deprecated Flutter API guidance: %q", prompt)
	}
	if !strings.Contains(prompt, "free of deprecated_member_use") {
		t.Fatalf("prompt missing flutter analyze hard-gate guidance: %q", prompt)
	}
}

func TestBuildBuilderRuntimePromptAllowsWithValuesWhenCapabilityEnabled(t *testing.T) {
	t.Setenv("APPFACTORY_FLUTTER_ENABLE_WITH_VALUES", "true")
	run := runRecord{
		GoalSummary:   "repair flutter ui",
		WorkspacePath: t.TempDir(),
		TaskBundle: []appruns.TaskBundleItem{{
			TaskID:      "task-home",
			TaskType:    appruns.BuilderRuntimeTaskTypeSingleFileEdit,
			TargetPaths: []string{"lib/views/home_page.dart"},
		}},
	}
	roundInput := appruns.RoundInput{
		TaskBundle: run.TaskBundle,
		AllowedPaths: []string{
			"lib/**",
		},
	}
	route := appruns.BuilderRuntimeTaskRoute{
		TaskID:      "task-home",
		TaskType:    appruns.BuilderRuntimeTaskTypeSingleFileEdit,
		RouteSource: "default_model",
	}

	prompt, err := buildBuilderRuntimePrompt(BuilderRuntimePatchRequest{Run: run, RoundInput: roundInput, Route: route})
	if err != nil {
		t.Fatalf("buildBuilderRuntimePrompt() error = %v", err)
	}
	if !strings.Contains(prompt, "withValues() is enabled for this run") {
		t.Fatalf("prompt missing withValues capability note: %q", prompt)
	}
}

func TestBuildBuilderRuntimePromptForbidsLocalizedFrameworkIdentifiers(t *testing.T) {
	run := runRecord{
		GoalSummary:   "repair record list page",
		WorkspacePath: t.TempDir(),
		TaskBundle: []appruns.TaskBundleItem{{
			TaskID:      "task-list-page",
			TaskType:    appruns.BuilderRuntimeTaskTypeDualFileWiring,
			TargetPaths: []string{"lib/views/record_list_page.dart"},
		}},
	}
	roundInput := appruns.RoundInput{
		TaskBundle:   run.TaskBundle,
		AllowedPaths: []string{"lib/**"},
	}
	route := appruns.BuilderRuntimeTaskRoute{
		TaskID:      "task-list-page",
		TaskType:    appruns.BuilderRuntimeTaskTypeDualFileWiring,
		RouteSource: "default_model",
	}

	prompt, err := buildBuilderRuntimePrompt(BuilderRuntimePatchRequest{Run: run, RoundInput: roundInput, Route: route})
	if err != nil {
		t.Fatalf("buildBuilderRuntimePrompt() error = %v", err)
	}
	if !strings.Contains(prompt, "Do not translate, localize, or mix non-ASCII characters into identifiers") {
		t.Fatalf("prompt missing identifier-localization guard: %q", prompt)
	}
	if !strings.Contains(prompt, "CrossAxisAlignment") {
		t.Fatalf("prompt missing concrete Flutter identifier example: %q", prompt)
	}
	if !strings.Contains(prompt, "Restrict non-ASCII text to user-facing string literals and comments") {
		t.Fatalf("prompt missing non-ASCII boundary guidance: %q", prompt)
	}
}

func TestBuildBuilderRuntimePromptUsesDartSyntaxRepairGuidance(t *testing.T) {
	workspacePath := t.TempDir()
	viewPath := filepath.Join(workspacePath, "lib", "views", "record_form_page.dart")
	if err := os.MkdirAll(filepath.Dir(viewPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(viewPath) error = %v", err)
	}
	if err := os.WriteFile(viewPath, []byte("import 'package:flutter/material.dart';\n\nclass RecordFormPage extends StatelessWidget {\n  const RecordFormPage({super.key});\n\n  @override\n  Widget build(BuildContext context) => const SizedBox.shrink();\n}\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(viewPath) error = %v", err)
	}
	run := runRecord{
		GoalSummary:   "repair record form page",
		WorkspacePath: workspacePath,
		TaskBundle: []appruns.TaskBundleItem{{
			TaskID:      "task-form-page",
			TaskType:    appruns.BuilderRuntimeTaskTypeSingleFileEdit,
			TargetPaths: []string{"lib/views/record_form_page.dart"},
		}},
	}
	roundInput := appruns.RoundInput{
		TaskBundle:   run.TaskBundle,
		AllowedPaths: []string{"lib/**"},
	}
	route := appruns.BuilderRuntimeTaskRoute{
		TaskID:      "task-form-page",
		TaskType:    appruns.BuilderRuntimeTaskTypeSingleFileEdit,
		RouteSource: "default_model",
	}
	prompt, err := buildBuilderRuntimePrompt(BuilderRuntimePatchRequest{
		Run:          run,
		RoundInput:   roundInput,
		Route:        route,
		RepairOnly:   true,
		PreviousErr:  "task file lib/views/record_form_page.dart failed Dart syntax validation: dart format rejected file",
		PreviousBody: `{"patch_id":"bad","operations":[{"type":"write_file","path":"lib/views/record_form_page.dart","content":"class RecordForm．Page {}"}]}`,
		FailureContext: strings.Join([]string{
			"task file lib/views/record_form_page.dart failed Dart syntax validation",
			"line 1, column 18: Illegal character '65294'",
		}, "\n"),
	})
	if err != nil {
		t.Fatalf("buildBuilderRuntimePrompt() error = %v", err)
	}
	if !strings.Contains(prompt, "Dart syntax repair mode is active") {
		t.Fatalf("prompt missing Dart syntax repair guidance: %q", prompt)
	}
	if !strings.Contains(prompt, "ASCII punctuation only") {
		t.Fatalf("prompt missing ASCII punctuation guard: %q", prompt)
	}
	if !strings.Contains(prompt, "State<RecordFormPage>") {
		t.Fatalf("prompt missing concrete generic-shape guidance: %q", prompt)
	}
	if strings.Contains(prompt, "Previous invalid response:") {
		t.Fatalf("prompt leaked previous invalid response block: %q", prompt)
	}
	if strings.Contains(prompt, "RecordForm．Page") {
		t.Fatalf("prompt leaked corrupted identifier from previous patch body: %q", prompt)
	}
}

func TestBuildBuilderRuntimePromptUsesMainEntryRepairGuidance(t *testing.T) {
	workspacePath := t.TempDir()
	mainPath := filepath.Join(workspacePath, "lib", "main.dart")
	homePath := filepath.Join(workspacePath, "lib", "views", "home_page.dart")
	if err := os.MkdirAll(filepath.Dir(homePath), 0o755); err != nil {
		t.Fatalf("MkdirAll(homePath) error = %v", err)
	}
	if err := os.WriteFile(mainPath, []byte("import 'package:flutter/material.dart';\n\nvoid main() {\n  runApp(const Placeholder());\n}\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(mainPath) error = %v", err)
	}
	if err := os.WriteFile(homePath, []byte("import 'package:flutter/material.dart';\n\nclass HomePage extends StatelessWidget {\n  const HomePage({super.key});\n\n  @override\n  Widget build(BuildContext context) => const SizedBox.shrink();\n}\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(homePath) error = %v", err)
	}
	run := runRecord{
		GoalSummary:   "repair app entry",
		WorkspacePath: workspacePath,
		TaskBundle: []appruns.TaskBundleItem{{
			TaskID:      "task-bind-app-entry",
			TaskType:    appruns.BuilderRuntimeTaskTypeSingleFileEdit,
			TargetPaths: []string{"lib/main.dart"},
		}},
	}
	roundInput := appruns.RoundInput{
		TaskBundle:   run.TaskBundle,
		AllowedPaths: []string{"lib/**"},
	}
	route := appruns.BuilderRuntimeTaskRoute{
		TaskID:      "task-bind-app-entry",
		TaskType:    appruns.BuilderRuntimeTaskTypeSingleFileEdit,
		RouteSource: "default_model",
	}
	prompt, err := buildBuilderRuntimePrompt(BuilderRuntimePatchRequest{
		Run:         run,
		RoundInput:  roundInput,
		Route:       route,
		RepairOnly:  true,
		PreviousErr: "task file lib/main.dart still retains Flutter default counter-demo shell MyHomePage in app entry",
		FailureContext: strings.Join([]string{
			"task file lib/main.dart still retains Flutter default counter-demo shell MyHomePage in app entry",
			"task file lib/main.dart still nests multiple MaterialApp roots in app entry",
		}, "\n"),
	})
	if err != nil {
		t.Fatalf("buildBuilderRuntimePrompt() error = %v", err)
	}
	if !strings.Contains(prompt, "keep exactly one MaterialApp") {
		t.Fatalf("prompt missing single-MaterialApp guidance: %q", prompt)
	}
	if !strings.Contains(prompt, "do not declare local fallback pages like MyHomePage") {
		t.Fatalf("prompt missing local fallback page guard: %q", prompt)
	}
	if !strings.Contains(prompt, "Do not invent named parameters such as recordRepository") {
		t.Fatalf("prompt missing constructor-signature repair guard: %q", prompt)
	}
}

func TestNormalizeBuilderRuntimePatchRepairsRawNewlinesInsideJSONString(t *testing.T) {
	raw := strings.Join([]string{
		`{`,
		`  "patch_id": "repair-domain-field-remaps",`,
		`  "operations": [`,
		`    {`,
		`      "replace_block": {`,
		`        "path": "lib/views/home_page.dart",`,
		"        \"old_content\": \"value: 'before",
		"after',\",",
		"        \"new_content\": \"value: 'next",
		"step',\"",
		`      }`,
		`    }`,
		`  ]`,
		`}`,
	}, "\n")

	patch, normalized, drift, err := normalizeBuilderRuntimePatch(raw, "round-1", nil)
	if err != nil {
		t.Fatalf("normalizeBuilderRuntimePatch() error = %v", err)
	}
	if !normalized {
		t.Fatal("normalizeBuilderRuntimePatch() should report normalized JSON")
	}
	if drift == 0 {
		t.Fatal("normalizeBuilderRuntimePatch() drift = 0, want repaired drift")
	}
	if len(patch.Operations) != 1 {
		t.Fatalf("len(patch.Operations) = %d, want 1", len(patch.Operations))
	}
	if patch.Operations[0].OldContent != "value: 'before\nafter'," {
		t.Fatalf("old_content = %q, want raw newline preserved after repair", patch.Operations[0].OldContent)
	}
	if patch.Operations[0].NewContent != "value: 'next\nstep'," {
		t.Fatalf("new_content = %q, want raw newline preserved after repair", patch.Operations[0].NewContent)
	}
}

func TestNormalizeBuilderRuntimePatchSupportsWriteFilePathShorthand(t *testing.T) {
	raw := `{
	  "patch_id": "patch-create-record-model",
	  "operations": [
	    {
	      "write_file": "lib/models/record.dart",
	      "new_content": "class AppRecord {}\n"
	    }
	  ]
	}`

	patch, normalized, drift, err := normalizeBuilderRuntimePatch(raw, "round-1", []string{"lib/models/record.dart"})
	if err != nil {
		t.Fatalf("normalizeBuilderRuntimePatch() error = %v", err)
	}
	if !normalized {
		t.Fatal("normalizeBuilderRuntimePatch() should report normalized shorthand write_file payload")
	}
	if drift == 0 {
		t.Fatal("normalizeBuilderRuntimePatch() drift = 0, want shorthand normalization drift")
	}
	if len(patch.Operations) != 1 {
		t.Fatalf("len(patch.Operations) = %d, want 1", len(patch.Operations))
	}
	if patch.Operations[0].Type != "write_file" {
		t.Fatalf("operation type = %q, want write_file", patch.Operations[0].Type)
	}
	if patch.Operations[0].Path != "lib/models/record.dart" {
		t.Fatalf("operation path = %q, want lib/models/record.dart", patch.Operations[0].Path)
	}
	if patch.Operations[0].Content != "class AppRecord {}\n" {
		t.Fatalf("operation content = %q, want shorthand new_content copied into content", patch.Operations[0].Content)
	}
}

func TestNormalizeBuilderRuntimePatchRepairsTrailingCommaBeforeObjectClose(t *testing.T) {
	raw := strings.Join([]string{
		`{`,
		`  "patch_id": "repair-trailing-comma",`,
		`  "operations": [`,
		`    {`,
		`      "write_file": {`,
		`        "path": "lib/views/record_form_page.dart",`,
		`        "new_content": "class RecordFormPage {}\n"`,
		`      },`,
		`    }`,
		`  ]`,
		`}`,
	}, "\n")

	patch, normalized, drift, err := normalizeBuilderRuntimePatch(raw, "round-1", []string{"lib/views/record_form_page.dart"})
	if err != nil {
		t.Fatalf("normalizeBuilderRuntimePatch() error = %v", err)
	}
	if !normalized {
		t.Fatal("normalizeBuilderRuntimePatch() should report normalized JSON after trailing comma repair")
	}
	if drift == 0 {
		t.Fatal("normalizeBuilderRuntimePatch() drift = 0, want trailing comma repair drift")
	}
	if len(patch.Operations) != 1 {
		t.Fatalf("len(patch.Operations) = %d, want 1", len(patch.Operations))
	}
	if patch.Operations[0].Path != "lib/views/record_form_page.dart" {
		t.Fatalf("patch.Operations[0].Path = %q, want lib/views/record_form_page.dart", patch.Operations[0].Path)
	}
}

func TestNormalizeBuilderRuntimePatchExtractsPatchJSONAfterEarlierDartFence(t *testing.T) {
	raw := strings.Join([]string{
		"I tried this first, but it is not the final patch:",
		"```dart",
		"void main() {",
		"  runApp(const WrongApp());",
		"}",
		"```",
		"Final patch:",
		"```json",
		`{"patch_id":"patch-bind-main","operations":[{"type":"write_file","path":"lib/main.dart","content":"void main() {}\n"}]}`,
		"```",
	}, "\n")

	patch, normalized, drift, err := normalizeBuilderRuntimePatch(raw, "round-1", []string{"lib/main.dart"})
	if err != nil {
		t.Fatalf("normalizeBuilderRuntimePatch() error = %v", err)
	}
	if !normalized {
		t.Fatal("normalizeBuilderRuntimePatch() should report normalized mixed prose/code/json response")
	}
	if drift == 0 {
		t.Fatal("normalizeBuilderRuntimePatch() drift = 0, want repaired drift for mixed response")
	}
	if len(patch.Operations) != 1 {
		t.Fatalf("len(patch.Operations) = %d, want 1", len(patch.Operations))
	}
	if patch.Operations[0].Path != "lib/main.dart" {
		t.Fatalf("patch.Operations[0].Path = %q, want lib/main.dart", patch.Operations[0].Path)
	}
	if patch.Operations[0].Content != "void main() {}\n" {
		t.Fatalf("patch.Operations[0].Content = %q, want JSON patch content", patch.Operations[0].Content)
	}
}

func TestNormalizeBuilderRuntimePatchRecoversSingleTargetDartFenceAsWriteFile(t *testing.T) {
	raw := strings.Join([]string{
		"To complete the task, replace lib/main.dart with the following content.",
		"```dart",
		"import 'package:flutter/material.dart';",
		"",
		"void main() {",
		"  runApp(const MyApp());",
		"}",
		"",
		"class MyApp extends StatelessWidget {",
		"  const MyApp({super.key});",
		"",
		"  @override",
		"  Widget build(BuildContext context) {",
		"    return const MaterialApp(home: Placeholder());",
		"  }",
		"}",
		"```",
	}, "\n")

	patch, normalized, drift, err := normalizeBuilderRuntimePatch(raw, "round-1", []string{"lib/main.dart"})
	if err != nil {
		t.Fatalf("normalizeBuilderRuntimePatch() error = %v", err)
	}
	if !normalized {
		t.Fatal("normalizeBuilderRuntimePatch() should report recovered single-target Dart fence as normalized")
	}
	if drift == 0 {
		t.Fatal("normalizeBuilderRuntimePatch() drift = 0, want recovery drift for fenced Dart fallback")
	}
	if len(patch.Operations) != 1 {
		t.Fatalf("len(patch.Operations) = %d, want 1", len(patch.Operations))
	}
	if patch.Operations[0].Type != "write_file" {
		t.Fatalf("patch.Operations[0].Type = %q, want write_file", patch.Operations[0].Type)
	}
	if patch.Operations[0].Path != "lib/main.dart" {
		t.Fatalf("patch.Operations[0].Path = %q, want lib/main.dart", patch.Operations[0].Path)
	}
	for _, want := range []string{"import 'package:flutter/material.dart';", "void main() {", "runApp(const MyApp());", "class MyApp extends StatelessWidget"} {
		if !strings.Contains(patch.Operations[0].Content, want) {
			t.Fatalf("patch.Operations[0].Content missing recovered Dart marker %q: %q", want, patch.Operations[0].Content)
		}
	}
}

func TestNormalizeBuilderRuntimePatchRecoversGenericAppEntryFenceOverLongerPlaceholderFence(t *testing.T) {
	raw := strings.Join([]string{
		"Draft one:",
		"```dart",
		"import 'package:flutter/material.dart';",
		"",
		"void main() {",
		"  runApp(const PlaceholderApp());",
		"}",
		"",
		"class PlaceholderApp extends StatelessWidget {",
		"  const PlaceholderApp({super.key});",
		"",
		"  @override",
		"  Widget build(BuildContext context) {",
		"    return const MaterialApp(home: Placeholder());",
		"  }",
		"}",
		"",
		"class ExtraPlaceholderDiagnostics extends StatelessWidget {",
		"  const ExtraPlaceholderDiagnostics({super.key});",
		"",
		"  @override",
		"  Widget build(BuildContext context) => const SizedBox.shrink();",
		"}",
		"```",
		"Final app-entry patch:",
		"```dart",
		"import 'package:flutter/material.dart';",
		"import 'views/task_overview_page.dart';",
		"",
		"void main() {",
		"  runApp(const TaskTrackerApp());",
		"}",
		"",
		"class TaskTrackerApp extends StatelessWidget {",
		"  const TaskTrackerApp({super.key});",
		"",
		"  @override",
		"  Widget build(BuildContext context) {",
		"    return const MaterialApp(home: TaskOverviewPage());",
		"  }",
		"}",
		"```",
	}, "\n")

	patch, normalized, drift, err := normalizeBuilderRuntimePatch(raw, "round-1", []string{"lib/main.dart"})
	if err != nil {
		t.Fatalf("normalizeBuilderRuntimePatch() error = %v", err)
	}
	if !normalized {
		t.Fatal("normalizeBuilderRuntimePatch() should recover generic app-entry Dart fence as normalized")
	}
	if drift == 0 {
		t.Fatal("normalizeBuilderRuntimePatch() drift = 0, want recovery drift for generic app-entry fence")
	}
	if len(patch.Operations) != 1 {
		t.Fatalf("len(patch.Operations) = %d, want 1", len(patch.Operations))
	}
	content := patch.Operations[0].Content
	if !strings.Contains(content, "import 'views/task_overview_page.dart';") {
		t.Fatalf("patch.Operations[0].Content should preserve custom view import: %q", content)
	}
	if !strings.Contains(content, "TaskOverviewPage") {
		t.Fatalf("patch.Operations[0].Content should recover custom surface constructor: %q", content)
	}
	if strings.Contains(content, "home: Placeholder()") {
		t.Fatalf("patch.Operations[0].Content should not prefer longer placeholder fence over custom app entry: %q", content)
	}
}

func TestNormalizeBuilderRuntimePatchWithWorkspacePrefersRegistryMatchedSingleTargetAppEntryFence(t *testing.T) {
	workspacePath := t.TempDir()
	writeBuilderRuntimeWorkspaceFiles(t, workspacePath, map[string]string{
		"lib/models/task.dart":                  "class Task { const Task({required this.title}); final String title; }\n",
		"lib/repositories/task_repository.dart": "abstract class TaskRepository {}\n",
		"lib/controllers/task_collection_controller.dart": strings.Join([]string{
			"import '../models/task.dart';",
			"import '../repositories/task_repository.dart';",
			"",
			"class TaskCollectionController {",
			"  TaskCollectionController({required this.taskRepository});",
			"  final TaskRepository taskRepository;",
			"  List<Task> get records => const [];",
			"}",
		}, "\n") + "\n",
		"lib/views/task_collection_page.dart": strings.Join([]string{
			"import '../controllers/task_collection_controller.dart';",
			"import '../models/task.dart';",
			"",
			"class TaskCollectionPage {",
			"  const TaskCollectionPage({required this.controller, required this.onCreateTask, required this.onOpenTaskDetail});",
			"  final TaskCollectionController controller;",
			"  final Future<void> Function() onCreateTask;",
			"  final Future<void> Function(Task task) onOpenTaskDetail;",
			"}",
		}, "\n") + "\n",
	})
	raw := strings.Join([]string{
		"Option A:",
		"```dart",
		"import 'package:flutter/material.dart';",
		"import 'views/task_overview_page.dart';",
		"",
		"void main() {",
		"  runApp(const TaskTrackerApp());",
		"}",
		"",
		"class TaskTrackerApp extends StatelessWidget {",
		"  const TaskTrackerApp({super.key});",
		"",
		"  @override",
		"  Widget build(BuildContext context) {",
		"    return MaterialApp(",
		"      theme: ThemeData(useMaterial3: true),",
		"      home: const TaskOverviewPage(),",
		"    );",
		"  }",
		"}",
		"```",
		"Option B:",
		"```dart",
		"import 'package:flutter/material.dart';",
		"import 'views/task_collection_page.dart';",
		"",
		"Future<void> _noop() async {}",
		"Future<void> _noopTask(Object task) async {}",
		"",
		"void main() {",
		"  runApp(const TaskTrackerApp());",
		"}",
		"",
		"class TaskTrackerApp extends StatelessWidget {",
		"  const TaskTrackerApp({super.key});",
		"",
		"  @override",
		"  Widget build(BuildContext context) {",
		"    return MaterialApp(",
		"      home: TaskCollectionPage(",
		"        controller: Object(),",
		"        onCreateTask: _noop,",
		"        onOpenTaskDetail: _noopTask,",
		"      ),",
		"    );",
		"  }",
		"}",
		"```",
	}, "\n")

	patch, normalized, drift, err := normalizeBuilderRuntimePatchWithWorkspace(workspacePath, raw, "round-1", []string{"lib/main.dart"})
	if err != nil {
		t.Fatalf("normalizeBuilderRuntimePatchWithWorkspace() error = %v", err)
	}
	if !normalized {
		t.Fatal("normalizeBuilderRuntimePatchWithWorkspace() should report recovered single-target Dart fence as normalized")
	}
	if drift == 0 {
		t.Fatal("normalizeBuilderRuntimePatchWithWorkspace() drift = 0, want recovery drift for registry-matched app-entry fence")
	}
	if len(patch.Operations) != 1 {
		t.Fatalf("len(patch.Operations) = %d, want 1", len(patch.Operations))
	}
	content := patch.Operations[0].Content
	if !strings.Contains(content, "import 'views/task_collection_page.dart';") {
		t.Fatalf("patch.Operations[0].Content should prefer registry-matched collection import: %q", content)
	}
	if !strings.Contains(content, "TaskCollectionPage(") {
		t.Fatalf("patch.Operations[0].Content should prefer registry-matched collection surface constructor: %q", content)
	}
	if strings.Contains(content, "task_overview_page.dart") || strings.Contains(content, "TaskOverviewPage") {
		t.Fatalf("patch.Operations[0].Content should not keep non-registry overview fence when workspace registry points to collection surface: %q", content)
	}
}

func TestNormalizeBuilderRuntimePatchStripsJSONCommentsOutsideStrings(t *testing.T) {
	raw := strings.Join([]string{
		`{`,
		`  "patch_id": "repair-commented-json",`,
		`  "operations": [`,
		`    {`,
		`      "type": "write_file",`,
		`      "path": "lib/views/record_form_page.dart",`,
		`      "content": "const url = 'https://example.com';\nconst marker = '/* keep */';\n"`,
		`    }, /* model note: keep only the patch */`,
		`    {`,
		`      "type": "write_file",`,
		`      "path": "lib/views/record_list_page.dart",`,
		`      "content": "class RecordListPage {}\n"`,
		`    } // final patch`,
		`  ]`,
		`}`,
	}, "\n")

	patch, normalized, drift, err := normalizeBuilderRuntimePatch(raw, "round-1", []string{"lib/views/record_form_page.dart", "lib/views/record_list_page.dart"})
	if err != nil {
		t.Fatalf("normalizeBuilderRuntimePatch() error = %v", err)
	}
	if !normalized {
		t.Fatal("normalizeBuilderRuntimePatch() should report normalized JSON after stripping comments")
	}
	if drift == 0 {
		t.Fatal("normalizeBuilderRuntimePatch() drift = 0, want comment-strip repair drift")
	}
	if len(patch.Operations) != 2 {
		t.Fatalf("len(patch.Operations) = %d, want 2", len(patch.Operations))
	}
	if patch.Operations[0].Path != "lib/views/record_form_page.dart" {
		t.Fatalf("patch.Operations[0].Path = %q, want lib/views/record_form_page.dart", patch.Operations[0].Path)
	}
	for _, want := range []string{"https://example.com", "/* keep */"} {
		if !strings.Contains(patch.Operations[0].Content, want) {
			t.Fatalf("patch.Operations[0].Content missing preserved string marker %q: %q", want, patch.Operations[0].Content)
		}
	}
	if patch.Operations[1].Path != "lib/views/record_list_page.dart" {
		t.Fatalf("patch.Operations[1].Path = %q, want lib/views/record_list_page.dart", patch.Operations[1].Path)
	}
	if patch.Operations[1].Content != "class RecordListPage {}\n" {
		t.Fatalf("patch.Operations[1].Content = %q, want preserved list page content", patch.Operations[1].Content)
	}
}

func TestBuildBuilderRuntimePromptIncludesHumanNotes(t *testing.T) {
	run := runRecord{
		GoalSummary:   "repair flutter ui",
		HumanNotes:    json.RawMessage(`[{"note_id":"note-canary","summary":"prefer stable public jobs repair canary","scope":"engineering"}]`),
		WorkspacePath: t.TempDir(),
		TaskBundle: []appruns.TaskBundleItem{{
			TaskID:      "task-home",
			TaskType:    appruns.BuilderRuntimeTaskTypeAnalyzeRepair,
			TargetPaths: []string{"lib/main.dart"},
		}},
	}
	roundInput := appruns.RoundInput{
		TaskBundle: run.TaskBundle,
		AllowedPaths: []string{
			"lib/**",
		},
	}
	route := appruns.BuilderRuntimeTaskRoute{
		TaskID:      "task-home",
		TaskType:    appruns.BuilderRuntimeTaskTypeAnalyzeRepair,
		RouteSource: "default_model",
	}

	prompt, err := buildBuilderRuntimePrompt(BuilderRuntimePatchRequest{Run: run, RoundInput: roundInput, Route: route})
	if err != nil {
		t.Fatalf("buildBuilderRuntimePrompt() error = %v", err)
	}
	if !strings.Contains(prompt, "Human notes JSON: [{\"note_id\":\"note-canary\"") {
		t.Fatalf("prompt missing human notes payload: %q", prompt)
	}
}

func TestBuildBuilderRuntimePromptIncludesPrepareDomainModel(t *testing.T) {
	jobRoot := t.TempDir()
	workspacePath := filepath.Join(jobRoot, "workspace")
	preparePath := filepath.Join(jobRoot, "prepare")
	if err := os.MkdirAll(workspacePath, 0o755); err != nil {
		t.Fatalf("MkdirAll(workspacePath) error = %v", err)
	}
	if err := os.MkdirAll(preparePath, 0o755); err != nil {
		t.Fatalf("MkdirAll(preparePath) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(preparePath, "domain-model.json"), []byte(`{"entities":[{"fields":[{"name":"weight"},{"name":"recorded_at"},{"name":"note"}]}]}`), 0o600); err != nil {
		t.Fatalf("WriteFile(domain-model.json) error = %v", err)
	}
	if err := os.MkdirAll(filepath.Join(workspacePath, "lib", "models"), 0o755); err != nil {
		t.Fatalf("MkdirAll(modelDir) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(workspacePath, "lib", "models", "record.dart"), []byte("class AppRecord {\n  const AppRecord({required this.recordId, required this.weight});\n\n  final String recordId;\n  final double weight;\n}\n\nenum RecordStatus {\n  draft,\n}\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(record.dart) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(workspacePath, "lib", "models", "dashboard_summary.dart"), []byte("class DashboardSummary {\n  const DashboardSummary();\n}\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(dashboard_summary.dart) error = %v", err)
	}
	run := runRecord{
		GoalSummary:   "repair flutter analyze failures",
		WorkspacePath: workspacePath,
		TaskBundle: []appruns.TaskBundleItem{{
			TaskID:      "repair-check-flutter-analyze",
			TaskType:    appruns.BuilderRuntimeTaskTypeAnalyzeRepair,
			TargetPaths: []string{"lib/controllers/record_form_controller.dart"},
		}},
	}
	roundInput := appruns.RoundInput{TaskBundle: run.TaskBundle, AllowedPaths: []string{"lib/**"}}
	route := appruns.BuilderRuntimeTaskRoute{TaskID: "repair-check-flutter-analyze", TaskType: appruns.BuilderRuntimeTaskTypeAnalyzeRepair, RouteSource: "task_route"}
	prompt, err := buildBuilderRuntimePrompt(BuilderRuntimePatchRequest{Run: run, RoundInput: roundInput, Route: route})
	if err != nil {
		t.Fatalf("buildBuilderRuntimePrompt() error = %v", err)
	}
	if !strings.Contains(prompt, "Prepare domain model JSON:") {
		t.Fatalf("prompt missing prepare domain model payload: %q", prompt)
	}
	if !strings.Contains(prompt, "source of truth for final entity and summary fields") {
		t.Fatalf("prompt missing domain model source-of-truth guidance: %q", prompt)
	}
	if !strings.Contains(prompt, "Current exported Dart model types:") {
		t.Fatalf("prompt missing exported model type context: %q", prompt)
	}
	if !strings.Contains(prompt, "lib/models/record.dart: AppRecord, RecordStatus") {
		t.Fatalf("prompt missing record export names: %q", prompt)
	}
	if !strings.Contains(prompt, "lib/models/dashboard_summary.dart: DashboardSummary") {
		t.Fatalf("prompt missing dashboard summary export names: %q", prompt)
	}
	if !strings.Contains(prompt, "Do not delete currently exported enums, classes, or mixins") {
		t.Fatalf("prompt missing exported type deletion guidance: %q", prompt)
	}
	if !strings.Contains(prompt, "Legacy exported helper types that may be removed") {
		t.Fatalf("prompt missing removable legacy export guidance: %q", prompt)
	}
	if !strings.Contains(prompt, "RecordStatus") {
		t.Fatalf("prompt missing removable RecordStatus guidance: %q", prompt)
	}
	if !strings.Contains(prompt, "Current Dart model API snapshot:") {
		t.Fatalf("prompt missing model api snapshot: %q", prompt)
	}
	if !strings.Contains(prompt, "fields=recordId, weight") {
		t.Fatalf("prompt missing model field snapshot: %q", prompt)
	}
	if !strings.Contains(prompt, "enum_values=draft") {
		t.Fatalf("prompt missing enum snapshot: %q", prompt)
	}
	if !strings.Contains(prompt, "Current forbidden schema tokens:") {
		t.Fatalf("prompt missing forbidden schema token list: %q", prompt)
	}
	if !strings.Contains(prompt, "RecordListFilter") {
		t.Fatalf("prompt missing derived forbidden tokens: %q", prompt)
	}
}

func TestExecuteBuilderRuntimeEditRetriesTruncatedPatchParseFailureBeforeSchemaRepair(t *testing.T) {
	jobRoot := t.TempDir()
	workspace := filepath.Join(jobRoot, "workspace")
	repoPath := filepath.Join(workspace, "lib", "repositories", "entry_repository.dart")
	if err := os.MkdirAll(filepath.Dir(repoPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(repoPath) error = %v", err)
	}
	original := strings.Join([]string{
		"abstract class EntryRepository {",
		"  Future<List<String>> loadEntries();",
		"}",
		"",
	}, "\n")
	if err := os.WriteFile(repoPath, []byte(original), 0o600); err != nil {
		t.Fatalf("WriteFile(repo) error = %v", err)
	}
	run := runRecord{
		RunID:         "run-entry-repo-truncated-parse-retry",
		GoalSummary:   "修复 entry repository 并保持当前领域语义。",
		WorkspacePath: workspace,
		AllowedPaths:  []string{"lib/**"},
		TaskBundle: []appruns.TaskBundleItem{{
			TaskID:      "task-entry-repo",
			TaskType:    appruns.BuilderRuntimeTaskTypeSingleFileEdit,
			TargetPaths: []string{"lib/repositories/entry_repository.dart"},
		}},
		BuilderRuntime: &appruns.BuilderRuntimePlan{
			Enabled: true,
			DefaultModel: appruns.BuilderRuntimeModelRef{
				Primary: "gemma4-26b-local",
			},
			TaskRoutes: []appruns.BuilderRuntimeTaskRoute{{
				TaskID:      "task-entry-repo",
				TaskType:    appruns.BuilderRuntimeTaskTypeSingleFileEdit,
				RouteSource: "task_route",
				Model: appruns.BuilderRuntimeModelRef{
					Primary: "gemma4-26b-local",
				},
			}},
		},
		LogPath: filepath.Join(t.TempDir(), "builder-runtime-entry-repo-truncated.log"),
	}
	roundInput := appruns.RoundInput{
		RoundID:        "round-entry-repo-truncated",
		Attempt:        1,
		TaskBundle:     run.TaskBundle,
		AllowedPaths:   run.AllowedPaths,
		BuilderRuntime: run.BuilderRuntime,
	}
	runner := &Runner{PatchGenerator: &stubBuilderRuntimePatchGenerator{responses: []BuilderRuntimePatchResponse{
		{
			ModelAlias: "gemma4-26b-local",
			Content:    "{\"patch_id\":\"round-entry-repo-truncated\",\"operations\":[{\"type\":\"write_file\",\"path\":\"lib/repositories/entry_repository.dart\",\"content\":\"abstract class EntryRepository {\\n",
		},
		{
			ModelAlias: "gemma4-26b-local",
			Content:    "{\"patch_id\":\"round-entry-repo-truncated-retry\",\"operations\":[{\"type\":\"write_file\",\"path\":\"lib/repositories/entry_repository.dart\",\"content\":\"abstract class EntryRepository {\\n  Future<List<String>> loadEntries();\\n}\\n\\nclass HiveEntryRepository implements EntryRepository {\\n  @override\\n  Future<List<String>> loadEntries() async {\\n    return ['new'];\\n  }\\n}\\n\"}]}",
		},
	}}}
	step := ExecutionStep{StepID: "thin-prepare", Stage: appruns.StageThinPrepare}

	result, err := runner.executeBuilderRuntimeEdit(context.Background(), noopRunnerBackend{}, run.RunID, step, run, roundInput)
	if err != nil {
		t.Fatalf("executeBuilderRuntimeEdit() error = %v, want truncated parse retry success", err)
	}
	if result == nil || result.Stats == nil {
		t.Fatalf("result = %+v, want stats", result)
	}
	if result.Stats.Attempts != 2 {
		t.Fatalf("stats.Attempts = %d, want 2", result.Stats.Attempts)
	}
	if result.Stats.ParseFailureCount != 1 {
		t.Fatalf("stats.ParseFailureCount = %d, want 1", result.Stats.ParseFailureCount)
	}
	generator, ok := runner.PatchGenerator.(*stubBuilderRuntimePatchGenerator)
	if !ok {
		t.Fatalf("PatchGenerator = %T, want *stubBuilderRuntimePatchGenerator", runner.PatchGenerator)
	}
	if len(generator.requests) != 2 {
		t.Fatalf("len(generator.requests) = %d, want 2", len(generator.requests))
	}
	if generator.requests[1].RepairOnly {
		t.Fatalf("retry request RepairOnly = true, want false")
	}
	if generator.requests[1].PreviousErr != "" {
		t.Fatalf("retry request PreviousErr = %q, want empty", generator.requests[1].PreviousErr)
	}
	if generator.requests[1].PreviousBody != "" {
		t.Fatalf("retry request PreviousBody = %q, want empty", generator.requests[1].PreviousBody)
	}
	content, err := os.ReadFile(repoPath)
	if err != nil {
		t.Fatalf("ReadFile(repo) error = %v", err)
	}
	if !strings.Contains(string(content), "return ['new'];") {
		t.Fatalf("repo content = %q, want retried write_file content", string(content))
	}
}

func TestExecuteBuilderRuntimeEditRetriesTruncatedSchemaRepairParseFailure(t *testing.T) {
	jobRoot := t.TempDir()
	workspace := filepath.Join(jobRoot, "workspace")
	repoPath := filepath.Join(workspace, "lib", "repositories", "entry_repository.dart")
	if err := os.MkdirAll(filepath.Dir(repoPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(repoPath) error = %v", err)
	}
	original := strings.Join([]string{
		"abstract class EntryRepository {",
		"  Future<List<String>> loadEntries();",
		"}",
		"",
	}, "\n")
	if err := os.WriteFile(repoPath, []byte(original), 0o600); err != nil {
		t.Fatalf("WriteFile(repo) error = %v", err)
	}
	run := runRecord{
		RunID:         "run-entry-repo-schema-retry",
		GoalSummary:   "修复 entry repository 并保持当前领域语义。",
		WorkspacePath: workspace,
		AllowedPaths:  []string{"lib/**"},
		TaskBundle: []appruns.TaskBundleItem{{
			TaskID:      "task-entry-repo",
			TaskType:    appruns.BuilderRuntimeTaskTypeSingleFileEdit,
			TargetPaths: []string{"lib/repositories/entry_repository.dart"},
		}},
		BuilderRuntime: &appruns.BuilderRuntimePlan{
			Enabled: true,
			DefaultModel: appruns.BuilderRuntimeModelRef{
				Primary: "gemma4-26b-local",
			},
			TaskRoutes: []appruns.BuilderRuntimeTaskRoute{{
				TaskID:      "task-entry-repo",
				TaskType:    appruns.BuilderRuntimeTaskTypeSingleFileEdit,
				RouteSource: "task_route",
				Model: appruns.BuilderRuntimeModelRef{
					Primary: "gemma4-26b-local",
				},
			}},
		},
		LogPath: filepath.Join(t.TempDir(), "builder-runtime-entry-repo-schema-retry.log"),
	}
	roundInput := appruns.RoundInput{
		RoundID:        "round-entry-repo-schema-retry",
		Attempt:        1,
		TaskBundle:     run.TaskBundle,
		AllowedPaths:   run.AllowedPaths,
		BuilderRuntime: run.BuilderRuntime,
	}
	runner := &Runner{PatchGenerator: &stubBuilderRuntimePatchGenerator{responses: []BuilderRuntimePatchResponse{
		{
			ModelAlias: "gemma4-26b-local",
			Content:    "",
		},
		{
			ModelAlias: "gemma4-26b-local",
			Content:    "{\"patch_id\":\"round-entry-repo-schema-retry\",\"operations\":[{\"type\":\"write_file\",\"path\":\"lib/repositories/entry_repository.dart\",\"content\":\"abstract class EntryRepository {\\n",
		},
		{
			ModelAlias: "gemma4-26b-local",
			Content:    "{\"patch_id\":\"round-entry-repo-schema-retry-repair\",\"operations\":[{\"type\":\"write_file\",\"path\":\"lib/repositories/entry_repository.dart\",\"content\":\"abstract class EntryRepository {\\n  Future<List<String>> loadEntries();\\n}\\n\\nclass HiveEntryRepository implements EntryRepository {\\n  @override\\n  Future<List<String>> loadEntries() async {\\n    return ['new'];\\n  }\\n}\\n\"}]}",
		},
	}}}
	step := ExecutionStep{StepID: "thin-prepare", Stage: appruns.StageThinPrepare}

	result, err := runner.executeBuilderRuntimeEdit(context.Background(), noopRunnerBackend{}, run.RunID, step, run, roundInput)
	if err != nil {
		t.Fatalf("executeBuilderRuntimeEdit() error = %v, want schema repair parse retry success", err)
	}
	if result == nil || result.Stats == nil {
		t.Fatalf("result = %+v, want stats", result)
	}
	if result.Stats.Attempts != 3 {
		t.Fatalf("stats.Attempts = %d, want 3", result.Stats.Attempts)
	}
	if result.Stats.ParseFailureCount != 2 {
		t.Fatalf("stats.ParseFailureCount = %d, want 2", result.Stats.ParseFailureCount)
	}
	generator, ok := runner.PatchGenerator.(*stubBuilderRuntimePatchGenerator)
	if !ok {
		t.Fatalf("PatchGenerator = %T, want *stubBuilderRuntimePatchGenerator", runner.PatchGenerator)
	}
	if len(generator.requests) != 3 {
		t.Fatalf("len(generator.requests) = %d, want 3", len(generator.requests))
	}
	if !generator.requests[1].RepairOnly {
		t.Fatalf("first schema repair request RepairOnly = false, want true")
	}
	if !generator.requests[2].RepairOnly {
		t.Fatalf("retried schema repair request RepairOnly = false, want true")
	}
	if generator.requests[2].PreviousBody != "" {
		t.Fatalf("retried schema repair PreviousBody = %q, want empty", generator.requests[2].PreviousBody)
	}
	if !strings.Contains(generator.requests[2].PreviousErr, "unexpected end of JSON input") {
		t.Fatalf("retried schema repair PreviousErr = %q, want parse failure context", generator.requests[2].PreviousErr)
	}
	content, err := os.ReadFile(repoPath)
	if err != nil {
		t.Fatalf("ReadFile(repo) error = %v", err)
	}
	if !strings.Contains(string(content), "return ['new'];") {
		t.Fatalf("repo content = %q, want repaired write_file content", string(content))
	}
}

func TestBuildBuilderRuntimePromptUsesExportRenameFailureContext(t *testing.T) {
	workspacePath := t.TempDir()
	modelDir := filepath.Join(workspacePath, "lib", "models")
	if err := os.MkdirAll(modelDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(modelDir) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(modelDir, "record.dart"), []byte("class AppRecord {\n  const AppRecord();\n}\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(record.dart) error = %v", err)
	}
	run := runRecord{
		GoalSummary:   "repair flutter analyze failures",
		WorkspacePath: workspacePath,
		TaskBundle: []appruns.TaskBundleItem{{
			TaskID:      "task-generic-domain-models",
			TaskType:    appruns.BuilderRuntimeTaskTypeDualFileWiring,
			TargetPaths: []string{"lib/models/record.dart"},
		}},
	}
	roundInput := appruns.RoundInput{TaskBundle: run.TaskBundle, AllowedPaths: []string{"lib/**"}}
	route := appruns.BuilderRuntimeTaskRoute{TaskID: "task-generic-domain-models", TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, RouteSource: "upgrade_model"}
	prompt, err := buildBuilderRuntimePrompt(BuilderRuntimePatchRequest{
		Run:            run,
		RoundInput:     roundInput,
		Route:          route,
		FailureContext: "patch renamed exported Dart model types without coordinated dependent updates: lib/models/record.dart -> AppRecord => WeightRecord",
	})
	if err != nil {
		t.Fatalf("buildBuilderRuntimePrompt() error = %v", err)
	}
	if !strings.Contains(prompt, "preserve the current exported names from lib/models/*.dart exactly") {
		t.Fatalf("prompt missing export-rename retry guidance: %q", prompt)
	}
	if !strings.Contains(prompt, "restore that export in place") {
		t.Fatalf("prompt missing dropped export retry guidance: %q", prompt)
	}
	if !strings.Contains(prompt, "Do not add extra exported helper classes") {
		t.Fatalf("prompt missing extra exported helper guard: %q", prompt)
	}
}

func TestBuildBuilderRuntimePromptUsesForbiddenTokenFailureContextAndPreviousPatch(t *testing.T) {
	workspacePath := t.TempDir()
	modelDir := filepath.Join(workspacePath, "lib", "models")
	if err := os.MkdirAll(modelDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(modelDir) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(modelDir, "record.dart"), []byte("class AppRecord {\n  const AppRecord({required this.recordId});\n\n  final String recordId;\n}\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(record.dart) error = %v", err)
	}
	run := runRecord{
		GoalSummary:   "repair flutter analyze failures",
		WorkspacePath: workspacePath,
		TaskBundle: []appruns.TaskBundleItem{{
			TaskID:      "task-generic-domain-models",
			TaskType:    appruns.BuilderRuntimeTaskTypeDualFileWiring,
			TargetPaths: []string{"lib/models/record.dart"},
		}},
	}
	roundInput := appruns.RoundInput{TaskBundle: run.TaskBundle, AllowedPaths: []string{"lib/**"}}
	route := appruns.BuilderRuntimeTaskRoute{TaskID: "task-generic-domain-models", TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, RouteSource: "upgrade_model"}
	prompt, err := buildBuilderRuntimePrompt(BuilderRuntimePatchRequest{
		Run:            run,
		RoundInput:     roundInput,
		Route:          route,
		PreviousErr:    "patch content reintroduced symbols absent from current models",
		PreviousBody:   `{"patch_id":"bad","operations":[{"write_file":{"path":"lib/models/record.dart","content":"class AppRecord { final String title; }"}}]}`,
		FailureContext: "patch content reintroduced symbols absent from current models: lib/models/record.dart -> title:, status:, RecordStatus",
	})
	if err != nil {
		t.Fatalf("buildBuilderRuntimePrompt() error = %v", err)
	}
	if !strings.Contains(prompt, "Previous rejected patch:") {
		t.Fatalf("prompt missing previous rejected patch block: %q", prompt)
	}
	if !strings.Contains(prompt, "class AppRecord { final String title; }") {
		t.Fatalf("prompt missing previous patch body content: %q", prompt)
	}
	if !strings.Contains(prompt, "The listed forbidden schema tokens must not appear anywhere in this retry patch") {
		t.Fatalf("prompt missing forbidden token guidance: %q", prompt)
	}
	if !strings.Contains(prompt, "RecordStatus, status:, title:") {
		t.Fatalf("prompt missing forbidden token list: %q", prompt)
	}
}

func TestBuildBuilderRuntimePromptIncludesControllerRepairGuidance(t *testing.T) {
	workspacePath := t.TempDir()
	modelDir := filepath.Join(workspacePath, "lib", "models")
	if err := os.MkdirAll(modelDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(modelDir) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(modelDir, "record.dart"), []byte("class AppRecord {\n  const AppRecord({required this.recordId, required this.weight, required this.recordedAt, this.note});\n\n  final String recordId;\n  final double weight;\n  final DateTime recordedAt;\n  final String? note;\n}\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(record.dart) error = %v", err)
	}
	run := runRecord{
		GoalSummary:   "repair flutter analyze failures",
		WorkspacePath: workspacePath,
		TaskBundle: []appruns.TaskBundleItem{{
			TaskID:      "repair-check-flutter-analyze",
			TaskType:    appruns.BuilderRuntimeTaskTypeAnalyzeRepair,
			TargetPaths: []string{"lib/controllers/record_form_controller.dart", "lib/controllers/record_list_controller.dart"},
			AllocationTransition: &appruns.TaskAllocationTransition{
				SurfaceRefs: []string{builderRuntimeMutationSurfaceRef, builderRuntimeCollectionSurfaceRef},
			},
		}},
	}
	roundInput := appruns.RoundInput{TaskBundle: run.TaskBundle, AllowedPaths: []string{"lib/**"}}
	route := appruns.BuilderRuntimeTaskRoute{TaskID: "repair-check-flutter-analyze", TaskType: appruns.BuilderRuntimeTaskTypeAnalyzeRepair, RouteSource: "upgrade_model"}
	prompt, err := buildBuilderRuntimePrompt(BuilderRuntimePatchRequest{
		Run:            run,
		RoundInput:     roundInput,
		Route:          route,
		FailureContext: "patch content reintroduced symbols absent from current models: lib/controllers/record_form_controller.dart -> categories, categoryController, setCategory(; lib/controllers/record_list_controller.dart -> RecordListFilter",
	})
	if err != nil {
		t.Fatalf("buildBuilderRuntimePrompt() error = %v", err)
	}
	if !strings.Contains(prompt, "rebuild the controller around weight, recordedAt, and note editing only") {
		t.Fatalf("prompt missing record_form controller repair guidance: %q", prompt)
	}
	if !strings.Contains(prompt, "without RecordListFilter or any status-based filtering buckets") {
		t.Fatalf("prompt missing record_list controller repair guidance: %q", prompt)
	}
}

func TestBuildBuilderRuntimePromptIncludesControllerRepairGuidanceForCustomWeightPrimaryModel(t *testing.T) {
	workspacePath := t.TempDir()
	writeBuilderRuntimeWorkspaceFiles(t, workspacePath, map[string]string{
		"lib/models/weight_record.dart": strings.Join([]string{
			"class WeightRecord {",
			"  const WeightRecord({required this.recordId, required this.weight, required this.recordedAt, this.note});",
			"  final String recordId;",
			"  final double weight;",
			"  final DateTime recordedAt;",
			"  final String? note;",
			"}",
		}, "\n") + "\n",
		"lib/controllers/weight_record_collection_controller.dart": strings.Join([]string{
			"class WeightRecordCollectionController {",
			"  const WeightRecordCollectionController();",
			"}",
		}, "\n") + "\n",
		"lib/views/weight_record_collection_page.dart": strings.Join([]string{
			"class WeightRecordCollectionPage {",
			"  const WeightRecordCollectionPage({required this.controller, required this.onOpenWeightRecordDetail});",
			"  final WeightRecordCollectionController controller;",
			"  final Future<void> Function(WeightRecord record) onOpenWeightRecordDetail;",
			"}",
		}, "\n") + "\n",
	})
	run := runRecord{
		GoalSummary:   "repair flutter analyze failures",
		WorkspacePath: workspacePath,
		TaskBundle: []appruns.TaskBundleItem{{
			TaskID:      "repair-check-flutter-analyze",
			TaskType:    appruns.BuilderRuntimeTaskTypeAnalyzeRepair,
			TargetPaths: []string{"lib/controllers/weight_record_form_controller.dart", "lib/controllers/weight_record_collection_controller.dart"},
			AllocationTransition: &appruns.TaskAllocationTransition{
				SurfaceRefs: []string{builderRuntimeMutationSurfaceRef, builderRuntimeCollectionSurfaceRef},
			},
		}},
	}
	roundInput := appruns.RoundInput{TaskBundle: run.TaskBundle, AllowedPaths: []string{"lib/**"}}
	route := appruns.BuilderRuntimeTaskRoute{TaskID: "repair-check-flutter-analyze", TaskType: appruns.BuilderRuntimeTaskTypeAnalyzeRepair, RouteSource: "upgrade_model"}
	prompt, err := buildBuilderRuntimePrompt(BuilderRuntimePatchRequest{
		Run:            run,
		RoundInput:     roundInput,
		Route:          route,
		FailureContext: "patch content reintroduced symbols absent from current models: lib/controllers/weight_record_form_controller.dart -> categories, categoryController, setCategory(; lib/controllers/weight_record_collection_controller.dart -> RecordListFilter",
	})
	if err != nil {
		t.Fatalf("buildBuilderRuntimePrompt() error = %v", err)
	}
	if !strings.Contains(prompt, "rebuild the controller around weight, recordedAt, and note editing only") {
		t.Fatalf("prompt missing custom weight-model controller repair guidance: %q", prompt)
	}
	if !strings.Contains(prompt, "without RecordListFilter or any status-based filtering buckets") {
		t.Fatalf("prompt missing custom weight-model collection controller guidance: %q", prompt)
	}
}

func TestBuildBuilderRuntimePromptIncludesPageAndRepositoryRepairGuidance(t *testing.T) {
	workspacePath := t.TempDir()
	modelDir := filepath.Join(workspacePath, "lib", "models")
	if err := os.MkdirAll(modelDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(modelDir) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(modelDir, "record.dart"), []byte("class AppRecord {\n  const AppRecord({required this.recordId, required this.weight, required this.recordedAt, this.note});\n\n  final String recordId;\n  final double weight;\n  final DateTime recordedAt;\n  final String? note;\n}\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(record.dart) error = %v", err)
	}
	run := runRecord{
		GoalSummary:   "repair flutter analyze failures",
		WorkspacePath: workspacePath,
		TaskBundle: []appruns.TaskBundleItem{{
			TaskID:      "repair-check-flutter-analyze",
			TaskType:    appruns.BuilderRuntimeTaskTypeAnalyzeRepair,
			TargetPaths: []string{"lib/views/record_form_page.dart", "lib/views/record_detail_page.dart", "lib/views/record_list_page.dart", "lib/views/home_page.dart", "lib/repositories/record_repository.dart", "lib/main.dart", "lib/template/open_lite_copy.dart"},
			AllocationTransition: &appruns.TaskAllocationTransition{
				SurfaceRefs: []string{builderRuntimeMutationSurfaceRef, builderRuntimeOverviewSurfaceRef, builderRuntimeCollectionSurfaceRef, builderRuntimeInspectionSurfaceRef},
			},
		}},
	}
	roundInput := appruns.RoundInput{TaskBundle: run.TaskBundle, AllowedPaths: []string{"lib/**"}}
	route := appruns.BuilderRuntimeTaskRoute{TaskID: "repair-check-flutter-analyze", TaskType: appruns.BuilderRuntimeTaskTypeAnalyzeRepair, RouteSource: "upgrade_model"}
	prompt, err := buildBuilderRuntimePrompt(BuilderRuntimePatchRequest{
		Run:            run,
		RoundInput:     roundInput,
		Route:          route,
		FailureContext: "lib/views/record_form_page.dart -> selectedDate; lib/views/record_detail_page.dart -> title; lib/views/record_list_page.dart -> RecordListFilter; lib/views/home_page.dart -> totalCount; lib/repositories/record_repository.dart -> id; lib/main.dart -> updatedAt; lib/template/open_lite_copy.dart -> RecordStatus",
	})
	if err != nil {
		t.Fatalf("buildBuilderRuntimePrompt() error = %v", err)
	}
	if !strings.Contains(prompt, "construct AppRecord(recordId, weight, recordedAt, note)") {
		t.Fatalf("prompt missing form flow guidance: %q", prompt)
	}
	if !strings.Contains(prompt, "render weight, recordedAt, note, latestWeight, recordCount, and trend") {
		t.Fatalf("prompt missing page repair guidance: %q", prompt)
	}
	if !strings.Contains(prompt, "use recordId as the record identifier and recordedAt as the time field") {
		t.Fatalf("prompt missing repository/main repair guidance: %q", prompt)
	}
	if !strings.Contains(prompt, "remove any status label helpers") {
		t.Fatalf("prompt missing open_lite_copy guidance: %q", prompt)
	}
}

func TestBuildBuilderRuntimePromptIncludesValidationFailureContext(t *testing.T) {
	run := runRecord{
		GoalSummary:   "repair flutter analyze failures",
		WorkspacePath: t.TempDir(),
		TaskBundle: []appruns.TaskBundleItem{{
			TaskID:      "task-home",
			TaskType:    appruns.BuilderRuntimeTaskTypeAnalyzeRepair,
			TargetPaths: []string{"lib/main.dart"},
		}},
	}
	roundInput := appruns.RoundInput{
		TaskBundle: run.TaskBundle,
		AllowedPaths: []string{
			"lib/**",
		},
	}
	route := appruns.BuilderRuntimeTaskRoute{
		TaskID:      "task-home",
		TaskType:    appruns.BuilderRuntimeTaskTypeAnalyzeRepair,
		RouteSource: "default_model",
	}

	prompt, err := buildBuilderRuntimePrompt(BuilderRuntimePatchRequest{Run: run, RoundInput: roundInput, Route: route, FailureContext: "check=check-flutter-analyze\nlib/views/home_page.dart:12:3: Error: Undefined name 'AppRecord'.\nundefined_getter: AppRecord.title"})
	if err != nil {
		t.Fatalf("buildBuilderRuntimePrompt() error = %v", err)
	}
	if !strings.Contains(prompt, "Validation failure context:") {
		t.Fatalf("prompt missing validation failure context header: %q", prompt)
	}
	if !strings.Contains(prompt, "undefined_getter: AppRecord.title") {
		t.Fatalf("prompt missing validation failure context payload: %q", prompt)
	}
	if !strings.Contains(prompt, "Current directly failing files JSON: [\"lib/views/home_page.dart\"]") {
		t.Fatalf("prompt missing direct failing files payload: %q", prompt)
	}
	if !strings.Contains(prompt, "Current directly failing files JSON is a hard coverage contract for analyze/test repair") {
		t.Fatalf("prompt missing direct failure hard coverage guidance: %q", prompt)
	}
	if !strings.Contains(prompt, "Prioritize fixing the concrete files named in validation failure context") {
		t.Fatalf("prompt missing failure-file prioritization guidance: %q", prompt)
	}
}

func TestBuildBuilderRuntimePromptIncludesBookkeepingRepairGuidance(t *testing.T) {
	workspacePath := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspacePath, "lib", "repositories"), 0o755); err != nil {
		t.Fatalf("MkdirAll(repoDir) error = %v", err)
	}
	if err := os.MkdirAll(filepath.Join(workspacePath, "lib", "models"), 0o755); err != nil {
		t.Fatalf("MkdirAll(modelDir) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(workspacePath, "lib", "repositories", "entry_repository.dart"), []byte(strings.Join([]string{
		"abstract class EntryRepository {}",
		"",
		"class InMemoryEntryRepository implements EntryRepository {}",
		"",
	}, "\n")), 0o600); err != nil {
		t.Fatalf("WriteFile(entry_repository.dart) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(workspacePath, "lib", "models", "entry.dart"), []byte(strings.Join([]string{
		"class BookkeepingEntry {",
		"  const BookkeepingEntry({required this.occurredOn, this.note});",
		"  final DateTime occurredOn;",
		"  final String? note;",
		"}",
	}, "\n")), 0o600); err != nil {
		t.Fatalf("WriteFile(entry.dart) error = %v", err)
	}
	run := runRecord{
		GoalSummary:   "repair bookkeeping analyze failures",
		WorkspacePath: workspacePath,
		TaskBundle: []appruns.TaskBundleItem{{
			TaskID:      "repair-check-flutter-analyze",
			TaskType:    appruns.BuilderRuntimeTaskTypeAnalyzeRepair,
			TargetPaths: []string{"lib/repositories/entry_repository.dart", "lib/views/home_page.dart", "test/widget_test.dart"},
			AllocationTransition: &appruns.TaskAllocationTransition{
				SurfaceRefs: []string{builderRuntimeOverviewSurfaceRef},
			},
		}},
	}
	roundInput := appruns.RoundInput{
		TaskBundle: run.TaskBundle,
		AllowedPaths: []string{
			"lib/**",
			"test/**",
		},
	}
	route := appruns.BuilderRuntimeTaskRoute{
		TaskID:      "repair-check-flutter-analyze",
		TaskType:    appruns.BuilderRuntimeTaskTypeAnalyzeRepair,
		RouteSource: "task_route",
	}
	failureContext := strings.Join([]string{
		"check=check-flutter-analyze",
		"lib/repositories/entry_repository.dart:31:60: Error: argument_type_not_assignable",
		"lib/views/home_page.dart:107:38: Error: undefined_getter",
		"error • The function 'InMemoryEntryRepository' isn't defined • test/widget_test.dart:10:34 • undefined_function",
	}, "\n")

	prompt, err := buildBuilderRuntimePrompt(BuilderRuntimePatchRequest{Run: run, RoundInput: roundInput, Route: route, FailureContext: failureContext})
	if err != nil {
		t.Fatalf("buildBuilderRuntimePrompt() error = %v", err)
	}
	for _, want := range []string{
		"Do not declare Box<Map<String, dynamic>> if the entries key stores a whole List<Map<String, dynamic>>",
		"never emit nested defaults like <List<Map<String, dynamic>>>[] or any List<List<Map<String, dynamic>>> shape",
		"BookkeepingEntry uses occurredOn and nullable note",
		"do not use BookkeepingEntry as a type argument unless the imported symbol is actually a Dart type",
		"prefer a full-file write_file for that file instead of leaving it untouched while only changing shared dependencies",
		"The current workspace already defines these failure-referenced Dart helper symbols and this patch must preserve or restore them: InMemoryEntryRepository -> lib/repositories/entry_repository.dart",
		"first restore or preserve the existing helper definitions for: InMemoryEntryRepository",
		"the same patch must define that concrete helper class in the final workspace instead of leaving the reference unresolved",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing bookkeeping repair guidance %q: %q", want, prompt)
		}
	}
}

func TestBuildBuilderRuntimePromptWarnsAboutStatusAndUpdatedAtCleanup(t *testing.T) {
	run := runRecord{
		GoalSummary:   "repair generic schema drift",
		WorkspacePath: t.TempDir(),
		TaskBundle: []appruns.TaskBundleItem{{
			TaskID:      "repair-check-flutter-analyze",
			TaskType:    appruns.BuilderRuntimeTaskTypeAnalyzeRepair,
			TargetPaths: []string{"lib/models/record.dart", "lib/models/dashboard_summary.dart", "lib/controllers/record_list_controller.dart", "lib/views/record_list_page.dart"},
		}},
	}
	roundInput := appruns.RoundInput{
		TaskBundle: run.TaskBundle,
		AllowedPaths: []string{
			"lib/**",
		},
	}
	route := appruns.BuilderRuntimeTaskRoute{
		TaskID:      "repair-check-flutter-analyze",
		TaskType:    appruns.BuilderRuntimeTaskTypeAnalyzeRepair,
		RouteSource: "default_model",
	}

	prompt, err := buildBuilderRuntimePrompt(BuilderRuntimePatchRequest{Run: run, RoundInput: roundInput, Route: route})
	if err != nil {
		t.Fatalf("buildBuilderRuntimePrompt() error = %v", err)
	}
	if !strings.Contains(prompt, "selectedStatus state") {
		t.Fatalf("prompt missing selectedStatus cleanup guidance: %q", prompt)
	}
	if !strings.Contains(prompt, "repository sort/order call") {
		t.Fatalf("prompt missing repository sort cleanup guidance: %q", prompt)
	}
	if !strings.Contains(prompt, "do not reference RecordStatus, RecordListFilter, openLiteCopy.statusLabel") {
		t.Fatalf("prompt missing generic status symbol ban: %q", prompt)
	}
	if !strings.Contains(prompt, "lib/main.dart callsite") {
		t.Fatalf("prompt missing lib/main.dart callsite sync guidance: %q", prompt)
	}
}

func TestBuildBuilderRuntimePromptGuardsRelationRichAnalyzeRepairAgainstDependencyAndAPIDrift(t *testing.T) {
	workspacePath := t.TempDir()
	writeBuilderRuntimeWorkspaceFiles(t, workspacePath, map[string]string{
		"lib/models/project.dart":           "class Project {}\n",
		"lib/models/task.dart":              "class Task {}\n",
		"lib/models/tag.dart":               "class Tag {}\n",
		"lib/models/task_tag_link.dart":     "class TaskTagLink {}\n",
		"lib/models/dashboard_summary.dart": "class DashboardSummary {}\n",
		"lib/repositories/record_repository.dart": strings.Join([]string{
			"import '../models/task_tag_link.dart';",
			"",
			"abstract class RecordRepository {",
			"  Future<List<TaskTagLink>> loadTaskTagLinks();",
			"}",
		}, "\n") + "\n",
	})
	run := runRecord{
		GoalSummary:   "repair relation-rich flutter analyze drift",
		WorkspacePath: workspacePath,
		TaskBundle: []appruns.TaskBundleItem{{
			TaskID:      "repair-check-flutter-analyze",
			TaskType:    appruns.BuilderRuntimeTaskTypeAnalyzeRepair,
			TargetPaths: []string{"lib/main.dart", "lib/controllers/record_form_controller.dart", "lib/repositories/record_repository.dart", "test/widget_test.dart"},
		}},
	}
	roundInput := appruns.RoundInput{
		TaskBundle:   run.TaskBundle,
		AllowedPaths: []string{"lib/**", "test/**"},
	}
	route := appruns.BuilderRuntimeTaskRoute{
		TaskID:      "repair-check-flutter-analyze",
		TaskType:    appruns.BuilderRuntimeTaskTypeAnalyzeRepair,
		RouteSource: "task_route",
	}
	failureContext := strings.Join([]string{
		"check=check-flutter-analyze",
		"info • The imported package 'provider' isn't a dependency of the importing package • lib/main.dart:2:8 • depend_on_referenced_packages",
		"error • Target of URI doesn't exist: 'package:uuid/uuid.dart' • lib/controllers/record_form_controller.dart:1:8 • uri_does_not_exist",
		"error • Undefined name '_navigatorKey' • lib/main.dart:30:11 • undefined_identifier",
		"error • The method 'setDate' isn't defined for the type 'RecordFormController' • lib/views/record_form_page.dart:44:19 • undefined_method",
		"error • The method 'loadTasksTagLinks' isn't defined for the type 'RecordRepository' • lib/repositories/record_repository.dart:88:21 • undefined_method",
	}, "\n")

	prompt, err := buildBuilderRuntimePrompt(BuilderRuntimePatchRequest{Run: run, RoundInput: roundInput, Route: route, FailureContext: failureContext})
	if err != nil {
		t.Fatalf("buildBuilderRuntimePrompt() error = %v", err)
	}
	for _, want := range []string{
		"Do not introduce new package imports or APIs from packages absent from current pubspec.yaml",
		"Do not introduce package:provider/provider.dart, package:uuid/uuid.dart",
		"either restore that API on the owning file or update every current caller in validation-repair target_paths to the final API in the same patch",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing relation-rich analyze-repair guard %q: %q", want, prompt)
		}
	}
}

func TestBuildBuilderRuntimePromptPrefersMinimalCleanupForAnalyzeClosureNoise(t *testing.T) {
	run := runRecord{
		GoalSummary:   "repair flutter analyze closure noise",
		WorkspacePath: t.TempDir(),
		TaskBundle: []appruns.TaskBundleItem{{
			TaskID:      "repair-check-flutter-analyze",
			TaskType:    appruns.BuilderRuntimeTaskTypeAnalyzeRepair,
			TargetPaths: []string{"lib/views/record_list_page.dart", "test/widget_test.dart"},
		}},
	}
	roundInput := appruns.RoundInput{
		TaskBundle:   run.TaskBundle,
		AllowedPaths: []string{"lib/**", "test/**"},
	}
	route := appruns.BuilderRuntimeTaskRoute{
		TaskID:      "repair-check-flutter-analyze",
		TaskType:    appruns.BuilderRuntimeTaskTypeAnalyzeRepair,
		RouteSource: "task_route",
	}
	failureContext := strings.Join([]string{
		"check=check-flutter-analyze",
		"warning • Unused import: '../models/tag.dart' • lib/views/record_list_page.dart:5:8 • unused_import",
		"warning • Unused import: 'package:flutter_open_lite/models/task.dart' • test/widget_test.dart:6:8 • unused_import",
		"warning • The declaration 'pumpAndSettle' isn't referenced • test/widget_test.dart:70:16 • unused_element",
	}, "\n")

	prompt, err := buildBuilderRuntimePrompt(BuilderRuntimePatchRequest{Run: run, RoundInput: roundInput, Route: route, FailureContext: failureContext})
	if err != nil {
		t.Fatalf("buildBuilderRuntimePrompt() error = %v", err)
	}
	for _, want := range []string{
		"Current validation failure is closure-only analyzer noise such as unused_import/unused_element.",
		"Prefer the smallest cleanup that removes unused imports, duplicate placeholder helpers, and other unreferenced private declarations.",
		"Do not rewrite unrelated architecture, navigation, or data flow when the failure context is only closure noise.",
		"When the only remaining analyzer findings in current views/tests are unused imports, delete the now-unused import lines directly and keep the existing view/test structure unchanged.",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing closure-noise repair guidance %q: %q", want, prompt)
		}
	}
}

func TestBuildBuilderRuntimePromptPreservesFrameworkHelperImportsDuringAnalyzeRepair(t *testing.T) {
	run := runRecord{
		GoalSummary:   "repair flutter analyze framework helper imports",
		WorkspacePath: t.TempDir(),
		TaskBundle: []appruns.TaskBundleItem{{
			TaskID:      "repair-check-flutter-analyze",
			TaskType:    appruns.BuilderRuntimeTaskTypeAnalyzeRepair,
			TargetPaths: []string{"lib/controllers/home_controller.dart"},
		}},
	}
	roundInput := appruns.RoundInput{
		TaskBundle:   run.TaskBundle,
		AllowedPaths: []string{"lib/**", "test/**"},
	}
	route := appruns.BuilderRuntimeTaskRoute{
		TaskID:      "repair-check-flutter-analyze",
		TaskType:    appruns.BuilderRuntimeTaskTypeAnalyzeRepair,
		RouteSource: "task_route",
	}
	failureContext := strings.Join([]string{
		"check=check-flutter-analyze",
		"error • The method 'debugPrint' isn't defined for the type 'HomeController' • lib/controllers/home_controller.dart:31:7 • undefined_method",
	}, "\n")

	prompt, err := buildBuilderRuntimePrompt(BuilderRuntimePatchRequest{Run: run, RoundInput: roundInput, Route: route, FailureContext: failureContext})
	if err != nil {
		t.Fatalf("buildBuilderRuntimePrompt() error = %v", err)
	}
	if !strings.Contains(prompt, "Do not leave a flutter/foundation.dart show-list import that keeps ChangeNotifier but omits debugPrint") {
		t.Fatalf("prompt missing framework helper import guidance: %q", prompt)
	}
}

func TestBuildBuilderRuntimePromptKeepsRelationRichRepositoryImportsAtHeaderDuringAnalyzeRepair(t *testing.T) {
	workspacePath := t.TempDir()
	writeBuilderRuntimeWorkspaceFiles(t, workspacePath, map[string]string{
		"lib/models/project.dart":           "class Project {}\n",
		"lib/models/task.dart":              "class Task {}\n",
		"lib/models/tag.dart":               "class Tag {}\n",
		"lib/models/task_tag_link.dart":     "class TaskTagLink {}\n",
		"lib/models/dashboard_summary.dart": "class DashboardSummary {}\n",
		"lib/repositories/record_repository.dart": strings.Join([]string{
			"import 'package:hive_flutter/hive_flutter.dart';",
			"",
			"import '../models/dashboard_summary.dart';",
			"import '../models/project.dart';",
			"import '../models/tag.dart';",
			"import '../models/task.dart';",
			"import '../models/task_tag_link.dart';",
			"",
			"abstract class RecordRepository {",
			"  Future<void> init();",
			"  Future<List<TaskTagLink>> loadTaskTagLinks();",
			"}",
		}, "\n") + "\n",
	})
	run := runRecord{
		GoalSummary:   "repair relation-rich repository analyze drift",
		WorkspacePath: workspacePath,
		TaskBundle: []appruns.TaskBundleItem{{
			TaskID:      "repair-check-flutter-analyze",
			TaskType:    appruns.BuilderRuntimeTaskTypeAnalyzeRepair,
			TargetPaths: []string{"lib/repositories/record_repository.dart"},
		}},
	}
	roundInput := appruns.RoundInput{
		TaskBundle:   run.TaskBundle,
		AllowedPaths: []string{"lib/**", "test/**"},
	}
	route := appruns.BuilderRuntimeTaskRoute{
		TaskID:      "repair-check-flutter-analyze",
		TaskType:    appruns.BuilderRuntimeTaskTypeAnalyzeRepair,
		RouteSource: "task_route",
	}
	failureContext := strings.Join([]string{
		"check=check-flutter-analyze",
		"error • Undefined function '_safe' • lib/repositories/record_repository.dart:56:17 • undefined_function",
		"error • Expected to find ';' • lib/repositories/record_repository.dart:100:5 • expected_token",
	}, "\n")

	prompt, err := buildBuilderRuntimePrompt(BuilderRuntimePatchRequest{Run: run, RoundInput: roundInput, Route: route, FailureContext: failureContext})
	if err != nil {
		t.Fatalf("buildBuilderRuntimePrompt() error = %v", err)
	}
	for _, want := range []string{
		"Keep every import for current repository-scope target files in the file header only.",
		"Do not paste a second import block below an existing class or method body during repair.",
		"Do not invent helper calls such as _safe<T>(...) unless that helper already exists in the current file context.",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing relation-rich repository repair guidance %q: %q", want, prompt)
		}
	}
}

func TestBuildBuilderRuntimePromptKeepsInventoryRelationRichRepositoryContractDuringAnalyzeRepair(t *testing.T) {
	workspacePath := createBuilderRuntimeInventoryRelationRichWorkspace(t, map[string]string{
		"lib/repositories/record_repository.dart": strings.Join([]string{
			"import 'package:hive_flutter/hive_flutter.dart';",
			"",
			"import '../models/dashboard_summary.dart';",
			"import '../models/inventory_sheet.dart';",
			"import '../models/line_item.dart';",
			"import '../models/sku.dart';",
			"import '../models/warehouse.dart';",
			"",
			"abstract class RecordRepository {",
			"  Future<void> init();",
			"  Future<List<InventorySheet>> loadInventorySheets();",
			"}",
		}, "\n") + "\n",
	})
	run := runRecord{
		GoalSummary:   "repair inventory relation-rich repository drift",
		WorkspacePath: workspacePath,
		TaskBundle: []appruns.TaskBundleItem{{
			TaskID:      "repair-check-flutter-analyze",
			TaskType:    appruns.BuilderRuntimeTaskTypeAnalyzeRepair,
			TargetPaths: []string{"lib/repositories/record_repository.dart"},
		}},
	}
	roundInput := appruns.RoundInput{TaskBundle: run.TaskBundle, AllowedPaths: []string{"lib/**", "test/**"}}
	route := appruns.BuilderRuntimeTaskRoute{TaskID: "repair-check-flutter-analyze", TaskType: appruns.BuilderRuntimeTaskTypeAnalyzeRepair, RouteSource: "task_route"}
	failureContext := strings.Join([]string{
		"check=check-flutter-analyze",
		"error • Undefined type 'async' • lib/repositories/record_repository.dart:88:62 • undefined_class",
		"error • The name 'DashboardSummary' is already defined • lib/repositories/record_repository.dart:181:7 • duplicate_definition",
	}, "\n")

	prompt, err := buildBuilderRuntimePrompt(BuilderRuntimePatchRequest{Run: run, RoundInput: roundInput, Route: route, FailureContext: failureContext})
	if err != nil {
		t.Fatalf("buildBuilderRuntimePrompt() error = %v", err)
	}
	for _, want := range []string{
		"Do not redefine DashboardSummary, InventorySheet, LineItem, Sku, or Warehouse inside current repository-scope target files.",
		"Do not introduce misspelled keys such as lowStockSukuCount or placeholder collection types such as List<async>.",
		"Keep every import for current repository-scope target files in the file header only.",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing inventory relation-rich repository guidance %q: %q", want, prompt)
		}
	}
}

func TestBuildBuilderRuntimePromptUsesRepositoryScopeForRelationRichRepositoryGuidance(t *testing.T) {
	workspacePath := t.TempDir()
	writeBuilderRuntimeWorkspaceFiles(t, workspacePath, map[string]string{
		"lib/models/project.dart":           "class Project {}\n",
		"lib/models/task.dart":              "class Task {}\n",
		"lib/models/tag.dart":               "class Tag {}\n",
		"lib/models/task_tag_link.dart":     "class TaskTagLink {}\n",
		"lib/models/dashboard_summary.dart": "class DashboardSummary {}\n",
	})
	run := runRecord{
		GoalSummary:   "repair relation-rich repository scope drift",
		WorkspacePath: workspacePath,
		TaskBundle: []appruns.TaskBundleItem{{
			TaskID:      "repair-check-flutter-analyze",
			TaskType:    appruns.BuilderRuntimeTaskTypeAnalyzeRepair,
			TargetPaths: []string{"lib/repositories/custom_repository.dart"},
		}},
	}
	roundInput := appruns.RoundInput{TaskBundle: run.TaskBundle, AllowedPaths: []string{"lib/**", "test/**"}}
	route := appruns.BuilderRuntimeTaskRoute{TaskID: "repair-check-flutter-analyze", TaskType: appruns.BuilderRuntimeTaskTypeAnalyzeRepair, RouteSource: "task_route"}
	failureContext := strings.Join([]string{
		"check=check-flutter-analyze",
		"error • Undefined function '_safe' • lib/repositories/custom_repository.dart:56:17 • undefined_function",
	}, "\n")

	prompt, err := buildBuilderRuntimePrompt(BuilderRuntimePatchRequest{Run: run, RoundInput: roundInput, Route: route, FailureContext: failureContext})
	if err != nil {
		t.Fatalf("buildBuilderRuntimePrompt() error = %v", err)
	}
	if !strings.Contains(prompt, "For current repository-scope target files under lib/repositories/*.dart in the current relation-rich task graph") {
		t.Fatalf("prompt should use repository-scope guidance instead of record_repository.dart trigger: %q", prompt)
	}
	if strings.Contains(prompt, "For lib/repositories/record_repository.dart in the current relation-rich task graph") {
		t.Fatalf("prompt should not use record_repository.dart as public repository trigger: %q", prompt)
	}
}

func TestFocusBuilderRuntimeTasksUsesOnlyCurrentRouteTaskOutsideClosureRepair(t *testing.T) {
	focus := focusBuilderRuntimeTasks([]appruns.TaskBundleItem{
		{TaskID: "repair-check-flutter-analyze", Category: appruns.TaskCategoryValidation, TaskType: appruns.BuilderRuntimeTaskTypeAnalyzeRepair, TargetPaths: []string{"lib/controllers/record_form_controller.dart"}},
		{TaskID: "task-generic-domain-models", Category: appruns.TaskCategoryDomain, TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, TargetPaths: []string{"lib/models/record.dart"}},
		{TaskID: "task-generic-validation-closure", Category: appruns.TaskCategoryValidation, TaskType: appruns.BuilderRuntimeTaskTypeClosureRepair, TargetPaths: []string{"lib/**"}},
	}, "repair-check-flutter-analyze", appruns.BuilderRuntimeTaskTypeAnalyzeRepair)
	if len(focus) != 1 {
		t.Fatalf("len(focus) = %d, want 1", len(focus))
	}
	if focus[0].TaskID != "repair-check-flutter-analyze" {
		t.Fatalf("focus[0].TaskID = %q, want repair-check-flutter-analyze", focus[0].TaskID)
	}
	if strings.Contains(focus[0].TaskID, "task-generic-domain-models") {
		t.Fatalf("focus should not retain unrelated domain task: %+v", focus)
	}
}

func TestFocusBuilderRuntimeTasksRetainsBundleForClosureRepair(t *testing.T) {
	focus := focusBuilderRuntimeTasks([]appruns.TaskBundleItem{
		{TaskID: "repair-check-flutter-analyze", Category: appruns.TaskCategoryValidation, TaskType: appruns.BuilderRuntimeTaskTypeAnalyzeRepair, TargetPaths: []string{"lib/controllers/record_form_controller.dart"}},
		{TaskID: "task-generic-domain-models", Category: appruns.TaskCategoryDomain, TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, TargetPaths: []string{"lib/models/record.dart"}},
		{TaskID: "task-generic-validation-closure", Category: appruns.TaskCategoryValidation, TaskType: appruns.BuilderRuntimeTaskTypeClosureRepair, TargetPaths: []string{"lib/**"}},
	}, "task-generic-validation-closure", appruns.BuilderRuntimeTaskTypeClosureRepair)
	if len(focus) != 2 {
		t.Fatalf("len(focus) = %d, want 2", len(focus))
	}
	if focus[0].TaskID != "repair-check-flutter-analyze" {
		t.Fatalf("focus[0].TaskID = %q, want repair-check-flutter-analyze", focus[0].TaskID)
	}
	if focus[1].TaskID != "task-generic-domain-models" {
		t.Fatalf("focus[1].TaskID = %q, want task-generic-domain-models", focus[1].TaskID)
	}
}

func TestBuildBuilderRuntimePromptUsesRouteTaskAsCurrentAllocationTask(t *testing.T) {
	run := runRecord{
		GoalSummary:   "repair flutter analyze failures",
		WorkspacePath: t.TempDir(),
		TaskBundle: []appruns.TaskBundleItem{
			{TaskID: "repair-check-flutter-analyze", Category: appruns.TaskCategoryValidation, TaskType: appruns.BuilderRuntimeTaskTypeAnalyzeRepair, TargetPaths: []string{"lib/controllers/record_form_controller.dart"}},
			{TaskID: "task-generic-domain-models", Category: appruns.TaskCategoryDomain, TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, RouteHint: appruns.TaskRouteHintStrongModel, RiskLevel: appruns.TaskRiskLevelHigh, TargetPaths: []string{"lib/models/record.dart", "lib/models/dashboard_summary.dart"}},
		},
	}
	roundInput := appruns.RoundInput{TaskBundle: run.TaskBundle, AllowedPaths: []string{"lib/**"}}
	route := appruns.BuilderRuntimeTaskRoute{TaskID: "repair-check-flutter-analyze", TaskType: appruns.BuilderRuntimeTaskTypeAnalyzeRepair, RouteSource: "task_route"}

	prompt, err := buildBuilderRuntimePrompt(BuilderRuntimePatchRequest{Run: run, RoundInput: roundInput, Route: route, FailureContext: "check=check-flutter-analyze\nlib/controllers/record_form_controller.dart:14:39: error"})
	if err != nil {
		t.Fatalf("buildBuilderRuntimePrompt() error = %v", err)
	}
	if !strings.Contains(prompt, "Current allocation task JSON: {\"task_id\":\"repair-check-flutter-analyze\"") {
		t.Fatalf("prompt should prioritize route task as current allocation task: %q", prompt)
	}
}

func TestBuildBuilderRuntimePromptUsesScopedAllowedPathsForAnalyzeRepair(t *testing.T) {
	run := runRecord{
		GoalSummary:   "repair flutter analyze failures",
		WorkspacePath: t.TempDir(),
		TaskBundle: []appruns.TaskBundleItem{
			{TaskID: "repair-check-flutter-analyze", Category: appruns.TaskCategoryValidation, TaskType: appruns.BuilderRuntimeTaskTypeAnalyzeRepair, TargetPaths: []string{"lib/controllers/record_form_controller.dart"}},
			{TaskID: "task-generic-domain-models", Category: appruns.TaskCategoryDomain, TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, TargetPaths: []string{"lib/models/record.dart", "lib/models/dashboard_summary.dart"}},
		},
	}
	roundInput := appruns.RoundInput{TaskBundle: run.TaskBundle, AllowedPaths: []string{"lib/**", "test/**"}}
	route := appruns.BuilderRuntimeTaskRoute{TaskID: "repair-check-flutter-analyze", TaskType: appruns.BuilderRuntimeTaskTypeAnalyzeRepair, RouteSource: "task_route"}

	prompt, err := buildBuilderRuntimePrompt(BuilderRuntimePatchRequest{Run: run, RoundInput: roundInput, Route: route})
	if err != nil {
		t.Fatalf("buildBuilderRuntimePrompt() error = %v", err)
	}
	if !strings.Contains(prompt, "Allowed paths JSON: [\"lib/controllers/record_form_controller.dart\",\"lib/models/dashboard_summary.dart\",\"lib/models/record.dart\"]") {
		t.Fatalf("prompt should narrow allowed paths to scoped concrete repair targets: %q", prompt)
	}
	if !strings.Contains(prompt, "Current validation-repair target_paths JSON: [\"lib/controllers/record_form_controller.dart\"]") {
		t.Fatalf("prompt should include concrete validation-repair target paths: %q", prompt)
	}
	if strings.Contains(prompt, "Allowed paths JSON: [\"lib/**\",\"test/**\"]") {
		t.Fatalf("prompt should not keep broad allowed paths during analyze repair: %q", prompt)
	}
}

func TestBuildBuilderRuntimePromptUsesRouteTargetPathsForNonRepairTasks(t *testing.T) {
	run := runRecord{
		GoalSummary:   "create home page",
		WorkspacePath: t.TempDir(),
		TaskBundle: []appruns.TaskBundleItem{
			{TaskID: "task-create-copy", Category: appruns.TaskCategoryContent, TaskType: appruns.BuilderRuntimeTaskTypeSingleFileEdit, TargetPaths: []string{"lib/template/open_lite_copy.dart"}},
			{TaskID: "task-create-home-page", Category: appruns.TaskCategoryScreen, TaskType: appruns.BuilderRuntimeTaskTypeSingleFileEdit, TargetPaths: []string{"lib/views/home_page.dart"}, Dependencies: []string{"task-create-copy"}},
		},
	}
	roundInput := appruns.RoundInput{TaskBundle: run.TaskBundle, AllowedPaths: []string{"lib/**", "test/**"}}
	route := appruns.BuilderRuntimeTaskRoute{TaskID: "task-create-home-page", TaskType: appruns.BuilderRuntimeTaskTypeSingleFileEdit, RouteSource: "task_route"}

	prompt, err := buildBuilderRuntimePrompt(BuilderRuntimePatchRequest{Run: run, RoundInput: roundInput, Route: route})
	if err != nil {
		t.Fatalf("buildBuilderRuntimePrompt() error = %v", err)
	}
	if !strings.Contains(prompt, "Allowed paths JSON: [\"lib/views/home_page.dart\"]") {
		t.Fatalf("prompt should narrow allowed paths to current route target: %q", prompt)
	}
	if strings.Contains(prompt, "Allowed paths JSON: [\"lib/**\",\"test/**\"]") {
		t.Fatalf("prompt should not keep broad allowed paths for non-repair tasks: %q", prompt)
	}
	if strings.Contains(prompt, "Treat current focus tasks as a coordinated implementation slice") {
		t.Fatalf("prompt should not coordinate multiple focus tasks for single route task: %q", prompt)
	}
}

func TestBuilderRuntimeFileContextPathsUsesRepairTaskTargetsForAnalyzeRepair(t *testing.T) {
	paths := builderRuntimeFileContextPaths([]appruns.TaskBundleItem{
		{TaskID: "repair-check-flutter-analyze", Category: appruns.TaskCategoryValidation, TaskType: appruns.BuilderRuntimeTaskTypeAnalyzeRepair, TargetPaths: []string{"lib/controllers/record_form_controller.dart", "lib/views/home_page.dart"}},
		{TaskID: "task-generic-domain-models", Category: appruns.TaskCategoryDomain, TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, TargetPaths: []string{"lib/models/record.dart", "lib/models/dashboard_summary.dart"}},
	}, "repair-check-flutter-analyze", appruns.BuilderRuntimeTaskTypeAnalyzeRepair)
	want := []string{"lib/controllers/record_form_controller.dart", "lib/views/home_page.dart", "lib/models/dashboard_summary.dart", "lib/models/record.dart"}
	if len(paths) != len(want) {
		t.Fatalf("len(paths) = %d, want %d; paths=%v", len(paths), len(want), paths)
	}
	for idx, item := range want {
		if paths[idx] != item {
			t.Fatalf("paths[%d] = %q, want %q", idx, paths[idx], item)
		}
	}
}

func TestBuilderRuntimeFileContextPathsKeepsFullTaskPathsForNonRepair(t *testing.T) {
	paths := builderRuntimeFileContextPaths([]appruns.TaskBundleItem{
		{TaskID: "repair-check-flutter-analyze", Category: appruns.TaskCategoryValidation, TaskType: appruns.BuilderRuntimeTaskTypeAnalyzeRepair, TargetPaths: []string{"lib/controllers/record_form_controller.dart"}},
		{TaskID: "task-generic-domain-models", Category: appruns.TaskCategoryDomain, TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, TargetPaths: []string{"lib/models/record.dart", "lib/models/dashboard_summary.dart"}},
	}, "task-generic-domain-models", appruns.BuilderRuntimeTaskTypeDualFileWiring)
	want := []string{"lib/models/dashboard_summary.dart", "lib/models/record.dart"}
	if len(paths) != len(want) {
		t.Fatalf("len(paths) = %d, want %d; paths=%v", len(paths), len(want), paths)
	}
	for idx, item := range want {
		if paths[idx] != item {
			t.Fatalf("paths[%d] = %q, want %q", idx, paths[idx], item)
		}
	}
}

func TestBuilderRuntimeFileContextPathsUsesDependenciesForNonRepair(t *testing.T) {
	paths := builderRuntimeFileContextPaths([]appruns.TaskBundleItem{
		{TaskID: "task-create-record-model", Category: appruns.TaskCategoryDomain, TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, TargetPaths: []string{"lib/models/record.dart"}},
		{TaskID: "task-create-repository", Category: appruns.TaskCategoryStorage, TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, Dependencies: []string{"task-create-record-model"}, TargetPaths: []string{"lib/repositories/record_repository.dart"}},
		{TaskID: "task-create-home-controller", Category: appruns.TaskCategorySummary, TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, Dependencies: []string{"task-create-repository"}, TargetPaths: []string{"lib/controllers/home_controller.dart"}},
	}, "task-create-home-controller", appruns.BuilderRuntimeTaskTypeDualFileWiring)
	want := []string{"lib/models/record.dart", "lib/repositories/record_repository.dart", "lib/controllers/home_controller.dart"}
	if len(paths) != len(want) {
		t.Fatalf("len(paths) = %d, want %d; paths=%v", len(paths), len(want), paths)
	}
	for idx, item := range want {
		if paths[idx] != item {
			t.Fatalf("paths[%d] = %q, want %q", idx, paths[idx], item)
		}
	}
}

func TestBuilderRuntimeFileContextPathsAddsModelContextForTemplateCopy(t *testing.T) {
	paths := builderRuntimeFileContextPaths([]appruns.TaskBundleItem{
		{TaskID: "task-create-record-model", Category: appruns.TaskCategoryDomain, TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, TargetPaths: []string{"lib/models/record.dart"}},
		{TaskID: "task-create-summary-model", Category: appruns.TaskCategorySummary, TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, Dependencies: []string{"task-create-record-model"}, TargetPaths: []string{"lib/models/dashboard_summary.dart"}},
		{TaskID: "task-create-copy", Category: appruns.TaskCategoryContent, TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, Dependencies: []string{"task-create-record-model"}, TargetPaths: []string{"lib/template/domain_copy.dart"}},
	}, "task-create-copy", appruns.BuilderRuntimeTaskTypeDualFileWiring)
	want := []string{"lib/models/record.dart", "lib/template/domain_copy.dart", "lib/models/dashboard_summary.dart"}
	if len(paths) != len(want) {
		t.Fatalf("len(paths) = %d, want %d; paths=%v", len(paths), len(want), paths)
	}
	for idx, item := range want {
		if paths[idx] != item {
			t.Fatalf("paths[%d] = %q, want %q", idx, paths[idx], item)
		}
	}
}

func TestBuildBuilderRuntimeFileContextUsesReferenceTemplateAndExistingDependencies(t *testing.T) {
	workspacePath := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspacePath, "lib", "models"), 0o755); err != nil {
		t.Fatalf("MkdirAll(modelDir) error = %v", err)
	}
	if err := os.MkdirAll(filepath.Join(workspacePath, "lib", "repositories"), 0o755); err != nil {
		t.Fatalf("MkdirAll(repoDir) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(workspacePath, "lib", "models", "record.dart"), []byte("class AppRecord {}\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(record.dart) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(workspacePath, "lib", "repositories", "record_repository.dart"), []byte("class RecordRepository {}\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(record_repository.dart) error = %v", err)
	}
	referenceRoot := t.TempDir()
	referencePath := filepath.Join(referenceRoot, "lib", "controllers", "home_controller.dart")
	if err := os.MkdirAll(filepath.Dir(referencePath), 0o755); err != nil {
		t.Fatalf("MkdirAll(referenceDir) error = %v", err)
	}
	if err := os.WriteFile(referencePath, []byte("class HomeControllerTemplate {}\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(reference home controller) error = %v", err)
	}
	run := runRecord{
		WorkspacePath: workspacePath,
		TemplateReferenceFiles: map[string]string{
			"lib/controllers/home_controller.dart": referencePath,
		},
		TaskBundle: []appruns.TaskBundleItem{
			{TaskID: "task-create-record-model", TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, TargetPaths: []string{"lib/models/record.dart"}},
			{TaskID: "task-create-repository", TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, Dependencies: []string{"task-create-record-model"}, TargetPaths: []string{"lib/repositories/record_repository.dart"}},
			{TaskID: "task-create-home-controller", TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, Dependencies: []string{"task-create-repository"}, TargetPaths: []string{"lib/controllers/home_controller.dart"}},
		},
	}
	contextText, err := buildBuilderRuntimeFileContext(run, "task-create-home-controller", appruns.BuilderRuntimeTaskTypeDualFileWiring)
	if err != nil {
		t.Fatalf("buildBuilderRuntimeFileContext() error = %v", err)
	}
	if !strings.Contains(contextText, "EXISTING FILE: lib/models/record.dart") {
		t.Fatalf("context missing existing model file label: %q", contextText)
	}
	if !strings.Contains(contextText, "EXISTING FILE: lib/repositories/record_repository.dart") {
		t.Fatalf("context missing existing repository file label: %q", contextText)
	}
	if !strings.Contains(contextText, "REFERENCE TEMPLATE: lib/controllers/home_controller.dart") {
		t.Fatalf("context missing reference template label: %q", contextText)
	}
	if strings.Contains(contextText, "<missing>") {
		t.Fatalf("context should not use legacy missing marker in reference-template mode: %q", contextText)
	}
}

func TestBuildBuilderRuntimePromptUsesReferenceTemplateCreateGuidance(t *testing.T) {
	jobRoot := t.TempDir()
	workspacePath := filepath.Join(jobRoot, "workspace")
	preparePath := filepath.Join(jobRoot, "prepare")
	referencePath := filepath.Join(jobRoot, "template", "lib", "controllers", "home_controller.dart")
	if err := os.MkdirAll(filepath.Join(workspacePath, "lib", "models"), 0o755); err != nil {
		t.Fatalf("MkdirAll(modelDir) error = %v", err)
	}
	if err := os.MkdirAll(preparePath, 0o755); err != nil {
		t.Fatalf("MkdirAll(prepareDir) error = %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(referencePath), 0o755); err != nil {
		t.Fatalf("MkdirAll(referenceDir) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(preparePath, "domain-model.json"), []byte(`{"entities":[{"fields":[{"name":"record_id"},{"name":"weight"},{"name":"recorded_at"},{"name":"note"}]}]}`), 0o600); err != nil {
		t.Fatalf("WriteFile(domain-model.json) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(workspacePath, "lib", "models", "record.dart"), []byte("class AppRecord {\n  const AppRecord({required this.recordId});\n  final String recordId;\n}\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(record.dart) error = %v", err)
	}
	if err := os.WriteFile(referencePath, []byte("class HomeControllerTemplate {}\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(referencePath) error = %v", err)
	}
	run := runRecord{
		GoalSummary:   "create home controller for weight records",
		WorkspacePath: workspacePath,
		TemplateReferenceFiles: map[string]string{
			"lib/controllers/home_controller.dart": referencePath,
		},
		TaskBundle: []appruns.TaskBundleItem{
			{TaskID: "task-create-record-model", TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, TargetPaths: []string{"lib/models/record.dart"}},
			{TaskID: "task-create-home-controller", TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, Dependencies: []string{"task-create-record-model"}, TargetPaths: []string{"lib/controllers/home_controller.dart"}},
		},
	}
	roundInput := appruns.RoundInput{TaskBundle: run.TaskBundle, AllowedPaths: []string{"lib/**"}}
	route := appruns.BuilderRuntimeTaskRoute{TaskID: "task-create-home-controller", TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, RouteSource: "default_model"}
	prompt, err := buildBuilderRuntimePrompt(BuilderRuntimePatchRequest{Run: run, RoundInput: roundInput, Route: route})
	if err != nil {
		t.Fatalf("buildBuilderRuntimePrompt() error = %v", err)
	}
	if !strings.Contains(prompt, "Reference-template mode is active") {
		t.Fatalf("prompt missing reference-template guidance: %q", prompt)
	}
	if !strings.Contains(prompt, "REFERENCE TEMPLATE OMITTED") {
		t.Fatalf("prompt missing omitted-template guidance: %q", prompt)
	}
	if !strings.Contains(prompt, "create the workspace file from scratch with write_file") {
		t.Fatalf("prompt missing create-from-scratch guidance: %q", prompt)
	}
	if !strings.Contains(prompt, "Current task should converge on one primary workspace file: lib/controllers/home_controller.dart") {
		t.Fatalf("prompt missing single-target guidance: %q", prompt)
	}
	if !strings.Contains(prompt, "REFERENCE TEMPLATE: lib/controllers/home_controller.dart") {
		t.Fatalf("prompt missing reference template file context: %q", prompt)
	}
	if !strings.Contains(prompt, "Current forbidden schema tokens: <none yet from workspace files>") {
		t.Fatalf("prompt missing early-task forbidden-token fallback guidance: %q", prompt)
	}
}

func TestBuildBuilderRuntimePromptUsesCompactPromptForMissingSingleTargetModelCreate(t *testing.T) {
	jobRoot := t.TempDir()
	workspacePath := filepath.Join(jobRoot, "workspace")
	preparePath := filepath.Join(jobRoot, "prepare")
	referencePath := filepath.Join(jobRoot, "template", "lib", "models", "record.dart")
	if err := os.MkdirAll(filepath.Join(workspacePath, "lib", "models"), 0o755); err != nil {
		t.Fatalf("MkdirAll(modelDir) error = %v", err)
	}
	if err := os.MkdirAll(preparePath, 0o755); err != nil {
		t.Fatalf("MkdirAll(prepareDir) error = %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(referencePath), 0o755); err != nil {
		t.Fatalf("MkdirAll(referenceDir) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(preparePath, "domain-model.json"), []byte(`{"entities":[{"name":"weight_record","fields":[{"name":"record_id"},{"name":"weight"},{"name":"recorded_at"},{"name":"note"}]}]}`), 0o600); err != nil {
		t.Fatalf("WriteFile(domain-model.json) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(workspacePath, "lib", "models", "dashboard_summary.dart"), []byte("class DashboardSummary {\n  const DashboardSummary({required this.latestWeight});\n\n  final double latestWeight;\n}\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(dashboard_summary.dart) error = %v", err)
	}
	if err := os.WriteFile(referencePath, []byte("class AppRecordTemplate {}\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(referencePath) error = %v", err)
	}
	run := runRecord{
		GoalSummary:   "create record model for weight tracker",
		WorkspacePath: workspacePath,
		TemplateReferenceFiles: map[string]string{
			"lib/models/record.dart": referencePath,
		},
		TaskBundle: []appruns.TaskBundleItem{
			{TaskID: "task-create-record-model", Title: "创建领域记录模型", TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, RouteHint: appruns.TaskRouteHintStrongModel, TargetPaths: []string{"lib/models/record.dart"}, AllocationTransition: &appruns.TaskAllocationTransition{EntityRefs: []string{"entity-weight-record"}, OwnedPaths: []string{"lib/models/record.dart"}, SuccessEvidence: []string{"领域记录模型已创建"}}},
			{TaskID: "task-create-summary-model", Title: "创建首页摘要模型", TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, Dependencies: []string{"task-create-record-model"}, TargetPaths: []string{"lib/models/dashboard_summary.dart"}},
		},
	}
	roundInput := appruns.RoundInput{TaskBundle: run.TaskBundle, AllowedPaths: []string{"lib/**"}}
	route := appruns.BuilderRuntimeTaskRoute{TaskID: "task-create-record-model", TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, RouteSource: "task_route"}

	prompt, err := buildBuilderRuntimePrompt(BuilderRuntimePatchRequest{Run: run, RoundInput: roundInput, Route: route})
	if err != nil {
		t.Fatalf("buildBuilderRuntimePrompt() error = %v", err)
	}
	if !strings.Contains(prompt, "This is a single-file model-create task.") {
		t.Fatalf("prompt should use compact single-file model-create guidance: %q", prompt)
	}
	if !strings.Contains(prompt, "Reference-template mode is active. The reference template is structural guidance only") {
		t.Fatalf("prompt missing compact reference-template guidance: %q", prompt)
	}
	if !strings.Contains(prompt, "Current task summary JSON:") {
		t.Fatalf("prompt missing compact task summary: %q", prompt)
	}
	if !strings.Contains(prompt, "REFERENCE TEMPLATE: lib/models/record.dart") {
		t.Fatalf("prompt missing target reference template context: %q", prompt)
	}
	if strings.Contains(prompt, "Current exported Dart model types:") {
		t.Fatalf("compact model-create prompt should omit exported model context: %q", prompt)
	}
	if strings.Contains(prompt, "Current topology does not include delete behavior") {
		t.Fatalf("compact model-create prompt should omit cross-surface topology guidance: %q", prompt)
	}
	if strings.Contains(prompt, "If you change shared model fields, constructor parameters") {
		t.Fatalf("compact model-create prompt should omit broad dependent-file coordination guidance: %q", prompt)
	}
}

func TestBuildBuilderRuntimePromptKeepsFullPromptForExistingSingleTargetModelFile(t *testing.T) {
	jobRoot := t.TempDir()
	workspacePath := filepath.Join(jobRoot, "workspace")
	preparePath := filepath.Join(jobRoot, "prepare")
	if err := os.MkdirAll(filepath.Join(workspacePath, "lib", "models"), 0o755); err != nil {
		t.Fatalf("MkdirAll(modelDir) error = %v", err)
	}
	if err := os.MkdirAll(preparePath, 0o755); err != nil {
		t.Fatalf("MkdirAll(prepareDir) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(preparePath, "domain-model.json"), []byte(`{"entities":[{"fields":[{"name":"record_id"},{"name":"weight"}]}]}`), 0o600); err != nil {
		t.Fatalf("WriteFile(domain-model.json) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(workspacePath, "lib", "models", "record.dart"), []byte("class AppRecord {\n  const AppRecord({required this.recordId, required this.weight});\n\n  final String recordId;\n  final double weight;\n}\n\nenum RecordStatus { draft }\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(record.dart) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(workspacePath, "lib", "models", "dashboard_summary.dart"), []byte("class DashboardSummary {\n  const DashboardSummary();\n}\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(dashboard_summary.dart) error = %v", err)
	}
	run := runRecord{
		GoalSummary:   "refine record model",
		WorkspacePath: workspacePath,
		TaskBundle:    []appruns.TaskBundleItem{{TaskID: "task-create-record-model", Title: "创建领域记录模型", TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, RouteHint: appruns.TaskRouteHintStrongModel, TargetPaths: []string{"lib/models/record.dart"}}},
	}
	roundInput := appruns.RoundInput{TaskBundle: run.TaskBundle, AllowedPaths: []string{"lib/**"}}
	route := appruns.BuilderRuntimeTaskRoute{TaskID: "task-create-record-model", TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, RouteSource: "task_route"}

	prompt, err := buildBuilderRuntimePrompt(BuilderRuntimePatchRequest{Run: run, RoundInput: roundInput, Route: route})
	if err != nil {
		t.Fatalf("buildBuilderRuntimePrompt() error = %v", err)
	}
	if strings.Contains(prompt, "This is a single-file model-create task.") {
		t.Fatalf("existing model file should keep the full prompt path: %q", prompt)
	}
	if !strings.Contains(prompt, "Current exported Dart model types:") {
		t.Fatalf("full prompt should keep exported model context: %q", prompt)
	}
	if !strings.Contains(prompt, "If you change shared model fields, constructor parameters") {
		t.Fatalf("full prompt should keep dependent-file coordination guidance: %q", prompt)
	}
}

func TestBuildBuilderRuntimePromptUsesCompactPromptForMissingSingleTargetSummaryModelCreate(t *testing.T) {
	jobRoot := t.TempDir()
	workspacePath := filepath.Join(jobRoot, "workspace")
	preparePath := filepath.Join(jobRoot, "prepare")
	referencePath := filepath.Join(jobRoot, "template", "lib", "models", "dashboard_summary.dart")
	if err := os.MkdirAll(filepath.Join(workspacePath, "lib", "models"), 0o755); err != nil {
		t.Fatalf("MkdirAll(modelDir) error = %v", err)
	}
	if err := os.MkdirAll(preparePath, 0o755); err != nil {
		t.Fatalf("MkdirAll(prepareDir) error = %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(referencePath), 0o755); err != nil {
		t.Fatalf("MkdirAll(referenceDir) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(preparePath, "domain-model.json"), []byte(`{"entities":[{"name":"weight_record","fields":[{"name":"record_id"},{"name":"weight"},{"name":"recorded_at"},{"name":"note"}]},{"name":"weight_summary","fields":[{"name":"latest_weight"},{"name":"trend_label"},{"name":"record_count"}]}]}`), 0o600); err != nil {
		t.Fatalf("WriteFile(domain-model.json) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(workspacePath, "lib", "models", "record.dart"), []byte(strings.Join([]string{
		"class AppRecord {",
		"  const AppRecord({required this.recordId, required this.weight, required this.recordedAt, this.note});",
		"  final String recordId;",
		"  final double weight;",
		"  final DateTime recordedAt;",
		"  final String? note;",
		"}",
	}, "\n")), 0o600); err != nil {
		t.Fatalf("WriteFile(record.dart) error = %v", err)
	}
	if err := os.WriteFile(referencePath, []byte("class DashboardSummaryTemplate {}\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(referencePath) error = %v", err)
	}
	run := runRecord{
		GoalSummary:   "create summary model for weight tracker",
		WorkspacePath: workspacePath,
		TemplateReferenceFiles: map[string]string{
			"lib/models/dashboard_summary.dart": referencePath,
		},
		TaskBundle: []appruns.TaskBundleItem{
			{TaskID: "task-create-record-model", Title: "创建领域记录模型", TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, RouteHint: appruns.TaskRouteHintStrongModel, TargetPaths: []string{"lib/models/record.dart"}, AllocationTransition: &appruns.TaskAllocationTransition{EntityRefs: []string{"entity-weight-record"}, OwnedPaths: []string{"lib/models/record.dart"}, SuccessEvidence: []string{"领域记录模型已创建"}}},
			{TaskID: "task-create-summary-model", Title: "创建首页摘要模型", TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, RouteHint: appruns.TaskRouteHintStrongModel, Dependencies: []string{"task-create-record-model"}, TargetPaths: []string{"lib/models/dashboard_summary.dart"}, AllocationTransition: &appruns.TaskAllocationTransition{EntityRefs: []string{"entity-weight-summary"}, OwnedPaths: []string{"lib/models/dashboard_summary.dart"}, SuccessEvidence: []string{"首页摘要模型已创建"}}},
		},
	}
	roundInput := appruns.RoundInput{TaskBundle: run.TaskBundle, AllowedPaths: []string{"lib/**"}}
	route := appruns.BuilderRuntimeTaskRoute{TaskID: "task-create-summary-model", TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, RouteSource: "task_route"}

	prompt, err := buildBuilderRuntimePrompt(BuilderRuntimePatchRequest{Run: run, RoundInput: roundInput, Route: route})
	if err != nil {
		t.Fatalf("buildBuilderRuntimePrompt() error = %v", err)
	}
	if !strings.Contains(prompt, "This is a single-file summary-model create task.") {
		t.Fatalf("prompt should use compact single-file summary-model guidance: %q", prompt)
	}
	if !strings.Contains(prompt, "Keep the exported model type named DashboardSummary") {
		t.Fatalf("prompt missing summary-model export guidance: %q", prompt)
	}
	if !strings.Contains(prompt, "Use aggregate scalar summary fields only.") {
		t.Fatalf("prompt missing scalar summary guidance: %q", prompt)
	}
	if !strings.Contains(prompt, "Do not introduce generic seed summary fields such as totalCount, inboxCount, inProgressCount, doneCount, status, or updatedAt") {
		t.Fatalf("prompt missing summary forbidden seed-field guidance: %q", prompt)
	}
	if !strings.Contains(prompt, "Reference-template mode is active. The reference template is structural guidance only") {
		t.Fatalf("prompt missing compact reference-template guidance: %q", prompt)
	}
	if !strings.Contains(prompt, "REFERENCE TEMPLATE: lib/models/dashboard_summary.dart") {
		t.Fatalf("prompt missing summary reference template context: %q", prompt)
	}
	if strings.Contains(prompt, "If you change shared model fields, constructor parameters") {
		t.Fatalf("compact summary-model prompt should omit broad dependent-file coordination guidance: %q", prompt)
	}
	if strings.Contains(prompt, "update every dependent controller, repository, widget, and test in the current file context") {
		t.Fatalf("compact summary-model prompt should omit broad open-lite schema remap guidance: %q", prompt)
	}
}

func TestBuildBuilderRuntimePromptKeepsFullPromptForExistingSingleTargetSummaryModelFile(t *testing.T) {
	jobRoot := t.TempDir()
	workspacePath := filepath.Join(jobRoot, "workspace")
	preparePath := filepath.Join(jobRoot, "prepare")
	if err := os.MkdirAll(filepath.Join(workspacePath, "lib", "models"), 0o755); err != nil {
		t.Fatalf("MkdirAll(modelDir) error = %v", err)
	}
	if err := os.MkdirAll(preparePath, 0o755); err != nil {
		t.Fatalf("MkdirAll(prepareDir) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(preparePath, "domain-model.json"), []byte(`{"entities":[{"name":"weight_record","fields":[{"name":"record_id"},{"name":"weight"},{"name":"recorded_at"}]},{"name":"weight_summary","fields":[{"name":"latest_weight"},{"name":"trend_label"}]}]}`), 0o600); err != nil {
		t.Fatalf("WriteFile(domain-model.json) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(workspacePath, "lib", "models", "record.dart"), []byte("class AppRecord {\n  const AppRecord({required this.recordedAt});\n  final DateTime recordedAt;\n}\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(record.dart) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(workspacePath, "lib", "models", "dashboard_summary.dart"), []byte("class DashboardSummary {\n  const DashboardSummary({required this.latestWeight});\n  final double latestWeight;\n}\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(dashboard_summary.dart) error = %v", err)
	}
	run := runRecord{
		GoalSummary:   "refine summary model",
		WorkspacePath: workspacePath,
		TaskBundle: []appruns.TaskBundleItem{
			{TaskID: "task-create-record-model", Title: "创建领域记录模型", TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, TargetPaths: []string{"lib/models/record.dart"}},
			{TaskID: "task-create-summary-model", Title: "创建首页摘要模型", TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, RouteHint: appruns.TaskRouteHintStrongModel, Dependencies: []string{"task-create-record-model"}, TargetPaths: []string{"lib/models/dashboard_summary.dart"}},
		},
	}
	roundInput := appruns.RoundInput{TaskBundle: run.TaskBundle, AllowedPaths: []string{"lib/**"}}
	route := appruns.BuilderRuntimeTaskRoute{TaskID: "task-create-summary-model", TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, RouteSource: "task_route"}

	prompt, err := buildBuilderRuntimePrompt(BuilderRuntimePatchRequest{Run: run, RoundInput: roundInput, Route: route})
	if err != nil {
		t.Fatalf("buildBuilderRuntimePrompt() error = %v", err)
	}
	if strings.Contains(prompt, "This is a single-file summary-model create task.") {
		t.Fatalf("existing summary model file should keep the full prompt path: %q", prompt)
	}
	if !strings.Contains(prompt, "Current exported Dart model types:") {
		t.Fatalf("full prompt should keep exported model context for existing summary model: %q", prompt)
	}
	if !strings.Contains(prompt, "If you change shared model fields, constructor parameters") {
		t.Fatalf("full prompt should keep dependent-file coordination guidance for existing summary model: %q", prompt)
	}
}

func TestBuildBuilderRuntimePromptUsesCompactPromptForMissingSingleTargetOverviewBinding(t *testing.T) {
	jobRoot := t.TempDir()
	workspacePath := filepath.Join(jobRoot, "workspace")
	preparePath := filepath.Join(jobRoot, "prepare")
	referencePath := filepath.Join(jobRoot, "template", "lib", "views", "home_page.dart")
	if err := os.MkdirAll(filepath.Join(workspacePath, "lib", "models"), 0o755); err != nil {
		t.Fatalf("MkdirAll(modelDir) error = %v", err)
	}
	if err := os.MkdirAll(filepath.Join(workspacePath, "lib", "controllers"), 0o755); err != nil {
		t.Fatalf("MkdirAll(controllerDir) error = %v", err)
	}
	if err := os.MkdirAll(filepath.Join(workspacePath, "lib", "template"), 0o755); err != nil {
		t.Fatalf("MkdirAll(templateDir) error = %v", err)
	}
	if err := os.MkdirAll(preparePath, 0o755); err != nil {
		t.Fatalf("MkdirAll(prepareDir) error = %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(referencePath), 0o755); err != nil {
		t.Fatalf("MkdirAll(referenceDir) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(preparePath, "domain-model.json"), []byte(`{"entities":[{"name":"weight_record","fields":[{"name":"record_id"},{"name":"weight"},{"name":"recorded_at"},{"name":"note"}]},{"name":"weight_summary","fields":[{"name":"latest_weight"},{"name":"trend_label"}]}]}`), 0o600); err != nil {
		t.Fatalf("WriteFile(domain-model.json) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(workspacePath, "lib", "models", "record.dart"), []byte(strings.Join([]string{
		"class AppRecord {",
		"  const AppRecord({required this.recordId, required this.weight, required this.recordedAt, this.note});",
		"  final String recordId;",
		"  final double weight;",
		"  final DateTime recordedAt;",
		"  final String? note;",
		"}",
	}, "\n")), 0o600); err != nil {
		t.Fatalf("WriteFile(record.dart) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(workspacePath, "lib", "models", "dashboard_summary.dart"), []byte(strings.Join([]string{
		"class DashboardSummary {",
		"  const DashboardSummary({required this.latestWeight, required this.trendLabel});",
		"  final double latestWeight;",
		"  final String trendLabel;",
		"}",
	}, "\n")), 0o600); err != nil {
		t.Fatalf("WriteFile(dashboard_summary.dart) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(workspacePath, "lib", "controllers", "home_controller.dart"), []byte(strings.Join([]string{
		"import '../models/dashboard_summary.dart';",
		"import '../models/record.dart';",
		"",
		"class HomeController {",
		"  HomeController({required this.summary, required this.records});",
		"  final DashboardSummary summary;",
		"  final List<AppRecord> records;",
		"}",
	}, "\n")), 0o600); err != nil {
		t.Fatalf("WriteFile(home_controller.dart) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(workspacePath, "lib", "template", "open_lite_copy.dart"), []byte(strings.Join([]string{
		"class OpenLiteCopy {",
		"  String homeTitle() => 'Weight tracker';",
		"}",
	}, "\n")), 0o600); err != nil {
		t.Fatalf("WriteFile(open_lite_copy.dart) error = %v", err)
	}
	if err := os.WriteFile(referencePath, []byte("class HomePageTemplate {}\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(referencePath) error = %v", err)
	}
	run := runRecord{
		GoalSummary:   "bind overview surface for weight tracker",
		WorkspacePath: workspacePath,
		TemplateReferenceFiles: map[string]string{
			"lib/views/home_page.dart": referencePath,
		},
		TaskBundle: []appruns.TaskBundleItem{
			{TaskID: "task-create-record-model", TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, TargetPaths: []string{"lib/models/record.dart"}},
			{TaskID: "task-create-summary-model", TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, Dependencies: []string{"task-create-record-model"}, TargetPaths: []string{"lib/models/dashboard_summary.dart"}},
			{TaskID: "task-create-home-controller", TaskType: appruns.BuilderRuntimeTaskTypeSingleFileEdit, TargetPaths: []string{"lib/controllers/home_controller.dart"}},
			{TaskID: "task-create-copy", TaskType: appruns.BuilderRuntimeTaskTypeSingleFileEdit, TargetPaths: []string{"lib/template/open_lite_copy.dart"}},
			{TaskID: "task-bind-overview-surface", Title: "绑定概览承载单元", TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, RouteHint: appruns.TaskRouteHintStrongModel, Dependencies: []string{"task-create-summary-model", "task-create-home-controller", "task-create-copy"}, TargetPaths: []string{"lib/views/home_page.dart"}, AllocationTransition: &appruns.TaskAllocationTransition{SurfaceRefs: []string{"surface-overview"}, EntityRefs: []string{"entity-weight-record", "entity-weight-summary"}, OwnedPaths: []string{"lib/views/home_page.dart"}, SuccessEvidence: []string{"概览承载单元已绑定"}}},
		},
	}
	roundInput := appruns.RoundInput{TaskBundle: run.TaskBundle, AllowedPaths: []string{"lib/**"}}
	route := appruns.BuilderRuntimeTaskRoute{TaskID: "task-bind-overview-surface", TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, RouteSource: "task_route"}

	prompt, err := buildBuilderRuntimePrompt(BuilderRuntimePatchRequest{Run: run, RoundInput: roundInput, Route: route})
	if err != nil {
		t.Fatalf("buildBuilderRuntimePrompt() error = %v", err)
	}
	if !strings.Contains(prompt, "This is a single-file overview-surface binding task.") {
		t.Fatalf("prompt should use compact single-file overview guidance: %q", prompt)
	}
	if !strings.Contains(prompt, "Emit exactly one write_file operation for lib/views/home_page.dart") {
		t.Fatalf("prompt missing compact write_file guidance: %q", prompt)
	}
	if !strings.Contains(prompt, "Reference-template mode is active. The reference template is structural guidance only") {
		t.Fatalf("prompt missing compact reference-template guidance: %q", prompt)
	}
	if !strings.Contains(prompt, "derive any overall record count from controller.records.length") {
		t.Fatalf("prompt missing compact count derivation guidance: %q", prompt)
	}
	if !strings.Contains(prompt, "Current primary record model does not define updatedAt") {
		t.Fatalf("prompt missing compact updatedAt prohibition: %q", prompt)
	}
	if !strings.Contains(prompt, "REFERENCE TEMPLATE: lib/views/home_page.dart") {
		t.Fatalf("prompt missing compact home_page reference template context: %q", prompt)
	}
	if strings.Contains(prompt, "If you change shared model fields, constructor parameters") {
		t.Fatalf("compact overview prompt should omit broad dependent-file coordination guidance: %q", prompt)
	}
	if strings.Contains(prompt, "Current topology does not include delete behavior") {
		t.Fatalf("compact overview prompt should omit unrelated topology guidance: %q", prompt)
	}
}

func TestBuildBuilderRuntimePromptUsesCompactOverviewGuidanceForSurfaceRefWithoutDefaultFileName(t *testing.T) {
	jobRoot := t.TempDir()
	workspacePath := filepath.Join(jobRoot, "workspace")
	preparePath := filepath.Join(jobRoot, "prepare")
	referenceRoot := filepath.Join(jobRoot, "template")
	if err := os.MkdirAll(filepath.Join(workspacePath, "lib", "models"), 0o755); err != nil {
		t.Fatalf("MkdirAll(modelDir) error = %v", err)
	}
	if err := os.MkdirAll(filepath.Join(workspacePath, "lib", "controllers"), 0o755); err != nil {
		t.Fatalf("MkdirAll(controllerDir) error = %v", err)
	}
	if err := os.MkdirAll(preparePath, 0o755); err != nil {
		t.Fatalf("MkdirAll(prepareDir) error = %v", err)
	}
	if err := os.MkdirAll(filepath.Join(referenceRoot, "lib", "views"), 0o755); err != nil {
		t.Fatalf("MkdirAll(referenceViewDir) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(preparePath, "domain-model.json"), []byte(`{"entities":[{"name":"weight_record","fields":[{"name":"record_id"},{"name":"weight"},{"name":"recorded_at"},{"name":"note"}]},{"name":"weight_summary","fields":[{"name":"latest_weight"},{"name":"trend_label"}]}]}`), 0o600); err != nil {
		t.Fatalf("WriteFile(domain-model.json) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(workspacePath, "lib", "models", "record.dart"), []byte("class AppRecord {\n  const AppRecord({required this.recordedAt});\n  final DateTime recordedAt;\n}\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(record.dart) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(workspacePath, "lib", "models", "dashboard_summary.dart"), []byte("class DashboardSummary {\n  const DashboardSummary({required this.latestWeight});\n  final double latestWeight;\n}\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(dashboard_summary.dart) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(workspacePath, "lib", "controllers", "task_overview_controller.dart"), []byte("class TaskOverviewController {}\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(task_overview_controller.dart) error = %v", err)
	}
	referencePath := filepath.Join(referenceRoot, "lib", "views", "task_overview_page.dart")
	if err := os.WriteFile(referencePath, []byte("class TaskOverviewPage {}\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(task_overview_page.dart) error = %v", err)
	}
	run := runRecord{
		GoalSummary:   "bind overview surface for weight tracker",
		WorkspacePath: workspacePath,
		TemplateReferenceFiles: map[string]string{
			"lib/views/task_overview_page.dart": referencePath,
		},
		TaskBundle: []appruns.TaskBundleItem{
			{TaskID: "task-create-record-model", TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, TargetPaths: []string{"lib/models/record.dart"}},
			{TaskID: "task-create-summary-model", TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, Dependencies: []string{"task-create-record-model"}, TargetPaths: []string{"lib/models/dashboard_summary.dart"}},
			{TaskID: "task-create-home-controller", TaskType: appruns.BuilderRuntimeTaskTypeSingleFileEdit, TargetPaths: []string{"lib/controllers/task_overview_controller.dart"}},
			{TaskID: "task-bind-overview-surface", Title: "绑定概览承载单元", TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, RouteHint: appruns.TaskRouteHintStrongModel, Dependencies: []string{"task-create-summary-model", "task-create-home-controller"}, TargetPaths: []string{"lib/views/task_overview_page.dart"}, AllocationTransition: &appruns.TaskAllocationTransition{SurfaceRefs: []string{builderRuntimeOverviewSurfaceRef}, EntityRefs: []string{"entity-weight-record", "entity-weight-summary"}, OwnedPaths: []string{"lib/views/task_overview_page.dart"}, SuccessEvidence: []string{"概览承载单元已绑定"}}},
		},
	}
	roundInput := appruns.RoundInput{TaskBundle: run.TaskBundle, AllowedPaths: []string{"lib/**"}}
	route := appruns.BuilderRuntimeTaskRoute{TaskID: "task-bind-overview-surface", TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, RouteSource: "task_route"}

	prompt, err := buildBuilderRuntimePrompt(BuilderRuntimePatchRequest{Run: run, RoundInput: roundInput, Route: route})
	if err != nil {
		t.Fatalf("buildBuilderRuntimePrompt() error = %v", err)
	}
	if !strings.Contains(prompt, "This is a single-file overview-surface binding task.") {
		t.Fatalf("prompt should use compact single-file overview guidance for custom overview surface: %q", prompt)
	}
	if !strings.Contains(prompt, "Emit exactly one write_file operation for lib/views/task_overview_page.dart") {
		t.Fatalf("prompt missing compact write_file guidance for custom overview file: %q", prompt)
	}
	if !strings.Contains(prompt, "REFERENCE TEMPLATE: lib/views/task_overview_page.dart") {
		t.Fatalf("prompt missing compact custom overview reference template context: %q", prompt)
	}
	if strings.Contains(prompt, "If you change shared model fields, constructor parameters") {
		t.Fatalf("compact overview prompt should omit broad dependent-file coordination guidance for custom overview surface: %q", prompt)
	}
}

func TestBuildBuilderRuntimePromptKeepsFullPromptForExistingSingleTargetOverviewFile(t *testing.T) {
	jobRoot := t.TempDir()
	workspacePath := filepath.Join(jobRoot, "workspace")
	preparePath := filepath.Join(jobRoot, "prepare")
	if err := os.MkdirAll(filepath.Join(workspacePath, "lib", "models"), 0o755); err != nil {
		t.Fatalf("MkdirAll(modelDir) error = %v", err)
	}
	if err := os.MkdirAll(filepath.Join(workspacePath, "lib", "controllers"), 0o755); err != nil {
		t.Fatalf("MkdirAll(controllerDir) error = %v", err)
	}
	if err := os.MkdirAll(filepath.Join(workspacePath, "lib", "views"), 0o755); err != nil {
		t.Fatalf("MkdirAll(viewDir) error = %v", err)
	}
	if err := os.MkdirAll(preparePath, 0o755); err != nil {
		t.Fatalf("MkdirAll(prepareDir) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(preparePath, "domain-model.json"), []byte(`{"entities":[{"name":"weight_record","fields":[{"name":"record_id"},{"name":"weight"},{"name":"recorded_at"},{"name":"note"}]},{"name":"weight_summary","fields":[{"name":"latest_weight"},{"name":"trend_label"}]}]}`), 0o600); err != nil {
		t.Fatalf("WriteFile(domain-model.json) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(workspacePath, "lib", "models", "record.dart"), []byte("class AppRecord {\n  const AppRecord({required this.recordedAt});\n  final DateTime recordedAt;\n}\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(record.dart) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(workspacePath, "lib", "models", "dashboard_summary.dart"), []byte("class DashboardSummary {\n  const DashboardSummary({required this.latestWeight});\n  final double latestWeight;\n}\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(dashboard_summary.dart) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(workspacePath, "lib", "controllers", "home_controller.dart"), []byte("class HomeController {}\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(home_controller.dart) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(workspacePath, "lib", "views", "home_page.dart"), []byte("class HomePage {}\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(home_page.dart) error = %v", err)
	}
	run := runRecord{
		GoalSummary:   "bind overview surface for weight tracker",
		WorkspacePath: workspacePath,
		TaskBundle: []appruns.TaskBundleItem{
			{TaskID: "task-create-record-model", TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, TargetPaths: []string{"lib/models/record.dart"}},
			{TaskID: "task-create-summary-model", TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, Dependencies: []string{"task-create-record-model"}, TargetPaths: []string{"lib/models/dashboard_summary.dart"}},
			{TaskID: "task-create-home-controller", TaskType: appruns.BuilderRuntimeTaskTypeSingleFileEdit, TargetPaths: []string{"lib/controllers/home_controller.dart"}},
			{TaskID: "task-bind-overview-surface", Title: "绑定概览承载单元", TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, RouteHint: appruns.TaskRouteHintStrongModel, Dependencies: []string{"task-create-summary-model", "task-create-home-controller"}, TargetPaths: []string{"lib/views/home_page.dart"}, AllocationTransition: &appruns.TaskAllocationTransition{SurfaceRefs: []string{"surface-overview"}, EntityRefs: []string{"entity-weight-record", "entity-weight-summary"}, OwnedPaths: []string{"lib/views/home_page.dart"}}},
		},
	}
	roundInput := appruns.RoundInput{TaskBundle: run.TaskBundle, AllowedPaths: []string{"lib/**"}}
	route := appruns.BuilderRuntimeTaskRoute{TaskID: "task-bind-overview-surface", TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, RouteSource: "task_route"}

	prompt, err := buildBuilderRuntimePrompt(BuilderRuntimePatchRequest{Run: run, RoundInput: roundInput, Route: route})
	if err != nil {
		t.Fatalf("buildBuilderRuntimePrompt() error = %v", err)
	}
	if strings.Contains(prompt, "This is a single-file overview-surface binding task.") {
		t.Fatalf("existing home_page should keep full prompt instead of compact overview guidance: %q", prompt)
	}
	if !strings.Contains(prompt, "If you change shared model fields, constructor parameters") {
		t.Fatalf("full prompt should keep dependent-file coordination guidance for existing home_page: %q", prompt)
	}
	if !strings.Contains(prompt, "For current overview-surface target files, keep summary cards, total-count copy, recent-record metadata") {
		t.Fatalf("full prompt should keep overview-surface field guidance: %q", prompt)
	}
}

func TestBuildBuilderRuntimeFileContextOmitsReferenceTemplateBodyForClosureRepair(t *testing.T) {
	workspacePath := t.TempDir()
	referenceRoot := t.TempDir()
	referencePath := filepath.Join(referenceRoot, "lib", "views", "home_page.dart")
	if err := os.MkdirAll(filepath.Dir(referencePath), 0o755); err != nil {
		t.Fatalf("MkdirAll(referenceDir) error = %v", err)
	}
	referenceContent := "class HomePageTemplate {\n  Widget build(BuildContext context) => const Placeholder();\n}\n"
	if err := os.WriteFile(referencePath, []byte(referenceContent), 0o600); err != nil {
		t.Fatalf("WriteFile(referencePath) error = %v", err)
	}
	run := runRecord{
		WorkspacePath: workspacePath,
		TemplateReferenceFiles: map[string]string{
			"lib/views/home_page.dart": referencePath,
		},
		TaskBundle: []appruns.TaskBundleItem{
			{TaskID: "task-create-home-page", TaskType: appruns.BuilderRuntimeTaskTypeSingleFileEdit, TargetPaths: []string{"lib/views/home_page.dart"}},
			{TaskID: "task-validation-closure", TaskType: appruns.BuilderRuntimeTaskTypeClosureRepair, Dependencies: []string{"task-create-home-page"}, TargetPaths: []string{"lib/**"}},
		},
	}
	contextText, err := buildBuilderRuntimeFileContext(run, "task-validation-closure", appruns.BuilderRuntimeTaskTypeClosureRepair)
	if err != nil {
		t.Fatalf("buildBuilderRuntimeFileContext() error = %v", err)
	}
	if !strings.Contains(contextText, "REFERENCE TEMPLATE OMITTED: lib/views/home_page.dart") {
		t.Fatalf("context missing omitted template marker: %q", contextText)
	}
	if !strings.Contains(contextText, "template_source="+referencePath) {
		t.Fatalf("context missing template source pointer: %q", contextText)
	}
	if strings.Contains(contextText, referenceContent) {
		t.Fatalf("context should omit full reference template body during closure repair: %q", contextText)
	}
}

func TestBuildBuilderRuntimePromptMarksSingleTargetNonRepairFileReadOnly(t *testing.T) {
	run := runRecord{
		GoalSummary:   "创建 relation-rich 任务列表页",
		WorkspacePath: createBuilderRuntimeRelationRichWorkspace(t, nil),
		TaskBundle: []appruns.TaskBundleItem{{
			TaskID:      "task-bind-collection-surface",
			TaskType:    appruns.BuilderRuntimeTaskTypeDualFileWiring,
			TargetPaths: []string{"lib/views/record_list_page.dart"},
		}},
	}
	roundInput := appruns.RoundInput{TaskBundle: run.TaskBundle, AllowedPaths: []string{"lib/**"}}
	route := appruns.BuilderRuntimeTaskRoute{
		TaskID:      "task-bind-collection-surface",
		TaskType:    appruns.BuilderRuntimeTaskTypeDualFileWiring,
		RouteSource: "default_model",
	}

	prompt, err := buildBuilderRuntimePrompt(BuilderRuntimePatchRequest{Run: run, RoundInput: roundInput, Route: route})
	if err != nil {
		t.Fatalf("buildBuilderRuntimePrompt() error = %v", err)
	}
	if !strings.Contains(prompt, "Emit operations only for lib/views/record_list_page.dart in this task") {
		t.Fatalf("prompt missing single-target non-repair read-only guidance: %q", prompt)
	}
}

func TestBuildBuilderRuntimePromptConstrainsRelationRichModelSliceToPlainJsonClasses(t *testing.T) {
	run := runRecord{
		GoalSummary:   "创建 relation-rich 领域模型",
		WorkspacePath: t.TempDir(),
		TaskBundle: []appruns.TaskBundleItem{{
			TaskID:   "task-create-relation-models",
			TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring,
			TargetPaths: []string{
				"lib/models/project.dart",
				"lib/models/task.dart",
				"lib/models/tag.dart",
				"lib/models/task_tag_link.dart",
			},
		}},
	}
	roundInput := appruns.RoundInput{TaskBundle: run.TaskBundle, AllowedPaths: []string{"lib/**"}}
	route := appruns.BuilderRuntimeTaskRoute{
		TaskID:      "task-create-relation-models",
		TaskType:    appruns.BuilderRuntimeTaskTypeDualFileWiring,
		RouteSource: "default_model",
	}

	prompt, err := buildBuilderRuntimePrompt(BuilderRuntimePatchRequest{Run: run, RoundInput: roundInput, Route: route})
	if err != nil {
		t.Fatalf("buildBuilderRuntimePrompt() error = %v", err)
	}
	for _, want := range []string{"emit plain Dart data classes with enums plus fromJson/toJson helpers only", "Do not import package:hive/hive.dart", "do not add part '*.g.dart'"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing relation-rich model constraint %q: %q", want, prompt)
		}
	}
}

func TestBuildBuilderRuntimePromptConstrainsInventoryRelationRichModelSliceToPlainJsonClasses(t *testing.T) {
	run := runRecord{
		GoalSummary:   "创建库存 relation-rich 领域模型",
		WorkspacePath: t.TempDir(),
		TaskBundle: []appruns.TaskBundleItem{{
			TaskID:   "task-create-relation-models",
			TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring,
			TargetPaths: []string{
				"lib/models/inventory_sheet.dart",
				"lib/models/line_item.dart",
				"lib/models/sku.dart",
				"lib/models/warehouse.dart",
			},
		}},
	}
	roundInput := appruns.RoundInput{TaskBundle: run.TaskBundle, AllowedPaths: []string{"lib/**"}}
	route := appruns.BuilderRuntimeTaskRoute{
		TaskID:      "task-create-relation-models",
		TaskType:    appruns.BuilderRuntimeTaskTypeDualFileWiring,
		RouteSource: "default_model",
	}

	prompt, err := buildBuilderRuntimePrompt(BuilderRuntimePatchRequest{Run: run, RoundInput: roundInput, Route: route})
	if err != nil {
		t.Fatalf("buildBuilderRuntimePrompt() error = %v", err)
	}
	for _, want := range []string{"lib/models/inventory_sheet.dart", "emit plain Dart data classes with enums plus fromJson/toJson helpers only", "Do not import package:hive/hive.dart"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing inventory relation-rich model constraint %q: %q", want, prompt)
		}
	}
}

func TestBuildBuilderRuntimePromptOmitsReferenceTemplateBodyForClosureRepair(t *testing.T) {
	jobRoot := t.TempDir()
	workspacePath := filepath.Join(jobRoot, "workspace")
	referencePath := filepath.Join(jobRoot, "template", "lib", "views", "home_page.dart")
	if err := os.MkdirAll(filepath.Dir(referencePath), 0o755); err != nil {
		t.Fatalf("MkdirAll(referenceDir) error = %v", err)
	}
	referenceContent := "class HomePageTemplate {\n  Widget build(BuildContext context) => const Placeholder();\n}\n"
	if err := os.WriteFile(referencePath, []byte(referenceContent), 0o600); err != nil {
		t.Fatalf("WriteFile(referencePath) error = %v", err)
	}
	run := runRecord{
		GoalSummary:   "close validation gap without full template replay",
		WorkspacePath: workspacePath,
		TemplateReferenceFiles: map[string]string{
			"lib/views/home_page.dart": referencePath,
		},
		TaskBundle: []appruns.TaskBundleItem{
			{TaskID: "task-create-home-page", TaskType: appruns.BuilderRuntimeTaskTypeSingleFileEdit, TargetPaths: []string{"lib/views/home_page.dart"}},
			{TaskID: "task-validation-closure", Category: appruns.TaskCategoryValidation, TaskType: appruns.BuilderRuntimeTaskTypeClosureRepair, Dependencies: []string{"task-create-home-page"}, TargetPaths: []string{"lib/**"}},
		},
	}
	roundInput := appruns.RoundInput{TaskBundle: run.TaskBundle, AllowedPaths: []string{"lib/**"}}
	route := appruns.BuilderRuntimeTaskRoute{TaskID: "task-validation-closure", TaskType: appruns.BuilderRuntimeTaskTypeClosureRepair, RouteSource: "task_route"}
	prompt, err := buildBuilderRuntimePrompt(BuilderRuntimePatchRequest{Run: run, RoundInput: roundInput, Route: route})
	if err != nil {
		t.Fatalf("buildBuilderRuntimePrompt() error = %v", err)
	}
	if !strings.Contains(prompt, "REFERENCE TEMPLATE OMITTED: lib/views/home_page.dart") {
		t.Fatalf("prompt missing omitted template marker: %q", prompt)
	}
	if strings.Contains(prompt, referenceContent) {
		t.Fatalf("prompt should omit full reference template body during closure repair: %q", prompt)
	}
}

func TestValidateBuilderRuntimeTargetScopeRejectsOutsideRepairTargets(t *testing.T) {
	err := validateBuilderRuntimeTargetScope([]appruns.WorkspacePatchOperation{
		{Type: "write_file", Path: "lib/controllers/record_form_controller.dart"},
		{Type: "write_file", Path: "lib/views/home_page.dart"},
	}, []string{"lib/controllers/record_form_controller.dart"})
	if err == nil {
		t.Fatalf("validateBuilderRuntimeTargetScope() error = nil, want scope violation")
	}
	if !strings.Contains(err.Error(), "lib/views/home_page.dart") {
		t.Fatalf("scope violation should mention outside path, got %q", err)
	}
}

func TestValidateBuilderRuntimeDirectFailureCoverageRejectsUntouchedAnalyzeTargets(t *testing.T) {
	err := validateBuilderRuntimeDirectFailureCoverage(
		appruns.BuilderRuntimeTaskTypeAnalyzeRepair,
		[]appruns.WorkspacePatchOperation{{Type: "write_file", Path: "lib/repositories/entry_repository.dart"}},
		[]string{"lib/repositories/entry_repository.dart", "lib/views/home_page.dart", "test/widget_test.dart"},
	)
	if err == nil {
		t.Fatalf("validateBuilderRuntimeDirectFailureCoverage() error = nil, want incomplete repair violation")
	}
	for _, want := range []string{"lib/views/home_page.dart", "test/widget_test.dart"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("coverage violation should mention %q, got %q", want, err)
		}
	}
}

func TestValidateBuilderRuntimeDirectFailureCoverageAllowsFullAnalyzeCoverage(t *testing.T) {
	err := validateBuilderRuntimeDirectFailureCoverage(
		appruns.BuilderRuntimeTaskTypeAnalyzeRepair,
		[]appruns.WorkspacePatchOperation{
			{Type: "write_file", Path: "lib/repositories/entry_repository.dart"},
			{Type: "write_file", Path: "lib/views/home_page.dart"},
			{Type: "write_file", Path: "test/widget_test.dart"},
		},
		[]string{"lib/repositories/entry_repository.dart", "lib/views/home_page.dart", "test/widget_test.dart"},
	)
	if err != nil {
		t.Fatalf("validateBuilderRuntimeDirectFailureCoverage() error = %v, want nil", err)
	}
}

func TestValidateBuilderRuntimeTaskTargetCoverageRejectsUntouchedNonRepairTargets(t *testing.T) {
	err := validateBuilderRuntimeTaskTargetCoverage(
		appruns.BuilderRuntimeTaskTypeDualFileWiring,
		[]appruns.WorkspacePatchOperation{{Type: "write_file", Path: "lib/main.dart"}},
		[]string{"lib/main.dart", "lib/views/home_page.dart"},
	)
	if err == nil {
		t.Fatalf("validateBuilderRuntimeTaskTargetCoverage() error = nil, want incomplete task coverage violation")
	}
	if !strings.Contains(err.Error(), "lib/views/home_page.dart") {
		t.Fatalf("task coverage violation should mention missing target, got %q", err)
	}
}

func TestValidateBuilderRuntimeTaskTargetCoverageAllowsFullNonRepairCoverage(t *testing.T) {
	err := validateBuilderRuntimeTaskTargetCoverage(
		appruns.BuilderRuntimeTaskTypeDualFileWiring,
		[]appruns.WorkspacePatchOperation{
			{Type: "write_file", Path: "lib/main.dart"},
			{Type: "write_file", Path: "lib/views/home_page.dart"},
		},
		[]string{"lib/main.dart", "lib/views/home_page.dart"},
	)
	if err != nil {
		t.Fatalf("validateBuilderRuntimeTaskTargetCoverage() error = %v, want nil", err)
	}
}

func TestValidateBuilderRuntimeSemanticConsistencyRejectsRemovedGenericSymbols(t *testing.T) {
	workspacePath := t.TempDir()
	jobRoot := t.TempDir()
	workspacePath = filepath.Join(jobRoot, "workspace")
	modelDir := filepath.Join(workspacePath, "lib", "models")
	prepareDir := filepath.Join(jobRoot, "prepare")
	if err := os.MkdirAll(prepareDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(prepareDir) error = %v", err)
	}
	if err := os.MkdirAll(modelDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(modelDir) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(prepareDir, "domain-model.json"), []byte(`{"entities":[{"fields":[{"name":"weight"},{"name":"recorded_at"},{"name":"note"}]}]}`), 0o600); err != nil {
		t.Fatalf("WriteFile(domain-model.json) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(modelDir, "record.dart"), []byte(strings.Join([]string{
		"class AppRecord {",
		"  final String id;",
		"  final double weight;",
		"  final DateTime recordedAt;",
		"  final String note;",
		"}",
	}, "\n")), 0o600); err != nil {
		t.Fatalf("WriteFile(record.dart) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(modelDir, "dashboard_summary.dart"), []byte(strings.Join([]string{
		"class DashboardSummary {",
		"  final double latestWeight;",
		"  final int recordCount;",
		"  final String trend;",
		"}",
	}, "\n")), 0o600); err != nil {
		t.Fatalf("WriteFile(dashboard_summary.dart) error = %v", err)
	}
	err := validateBuilderRuntimeSemanticConsistency(runRecord{WorkspacePath: workspacePath}, []appruns.WorkspacePatchOperation{{
		Type:    "write_file",
		Path:    "lib/views/record_list_page.dart",
		Content: "Text(openLiteCopy.statusLabel(record.status))\n",
	}}, "")
	if err == nil {
		t.Fatalf("validateBuilderRuntimeSemanticConsistency() error = nil, want semantic conflict")
	}
	if !strings.Contains(err.Error(), "record.status") && !strings.Contains(err.Error(), ".status") {
		t.Fatalf("semantic conflict should mention removed status symbol, got %q", err)
	}
}

func TestValidateBuilderRuntimeSemanticConsistencyRejectsModelExportRename(t *testing.T) {
	workspacePath := t.TempDir()
	modelDir := filepath.Join(workspacePath, "lib", "models")
	if err := os.MkdirAll(modelDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(modelDir) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(modelDir, "record.dart"), []byte(strings.Join([]string{
		"class AppRecord {",
		"  const AppRecord();",
		"}",
	}, "\n")), 0o600); err != nil {
		t.Fatalf("WriteFile(record.dart) error = %v", err)
	}
	err := validateBuilderRuntimeSemanticConsistency(runRecord{WorkspacePath: workspacePath}, []appruns.WorkspacePatchOperation{{
		Type:    "write_file",
		Path:    "lib/models/record.dart",
		Content: "class WeightRecord {\n  const WeightRecord();\n}\n",
	}}, "")
	if err == nil {
		t.Fatalf("validateBuilderRuntimeSemanticConsistency() error = nil, want export rename conflict")
	}
	if !strings.Contains(err.Error(), "patch renamed exported Dart model types") {
		t.Fatalf("export rename conflict missing from error: %q", err)
	}
	if !strings.Contains(err.Error(), "AppRecord") || !strings.Contains(err.Error(), "WeightRecord") {
		t.Fatalf("export rename conflict should mention old/new types, got %q", err)
	}
}

func TestValidateBuilderRuntimeSemanticConsistencyAllowsRemovingLegacyStatusExport(t *testing.T) {
	jobRoot := t.TempDir()
	workspacePath := filepath.Join(jobRoot, "workspace")
	modelDir := filepath.Join(workspacePath, "lib", "models")
	prepareDir := filepath.Join(jobRoot, "prepare")
	if err := os.MkdirAll(modelDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(modelDir) error = %v", err)
	}
	if err := os.MkdirAll(prepareDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(prepareDir) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(prepareDir, "domain-model.json"), []byte(`{"entities":[{"fields":[{"name":"record_id"},{"name":"weight"},{"name":"recorded_at"},{"name":"note"}]}]}`), 0o600); err != nil {
		t.Fatalf("WriteFile(domain-model.json) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(modelDir, "record.dart"), []byte("enum RecordStatus {\n  inbox,\n}\n\nclass AppRecord {\n  const AppRecord();\n}\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(record.dart) error = %v", err)
	}
	err := validateBuilderRuntimeSemanticConsistency(runRecord{WorkspacePath: workspacePath}, []appruns.WorkspacePatchOperation{{
		Type:    "write_file",
		Path:    "lib/models/record.dart",
		Content: "class AppRecord {\n  const AppRecord();\n}\n",
	}}, "")
	if err != nil {
		t.Fatalf("validateBuilderRuntimeSemanticConsistency() error = %v, want removable legacy export allowed", err)
	}
}

func TestValidateBuilderRuntimeSemanticConsistencyAllowsFlutterTitleParameter(t *testing.T) {
	jobRoot := t.TempDir()
	workspacePath := filepath.Join(jobRoot, "workspace")
	prepareDir := filepath.Join(jobRoot, "prepare")
	modelDir := filepath.Join(workspacePath, "lib", "models")
	if err := os.MkdirAll(prepareDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(prepareDir) error = %v", err)
	}
	if err := os.MkdirAll(modelDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(modelDir) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(prepareDir, "domain-model.json"), []byte(`{"entities":[{"fields":[{"name":"record_id"},{"name":"weight"},{"name":"recorded_at"},{"name":"note"}]}]}`), 0o600); err != nil {
		t.Fatalf("WriteFile(domain-model.json) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(modelDir, "record.dart"), []byte("class AppRecord {\n  const AppRecord();\n}\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(record.dart) error = %v", err)
	}
	err := validateBuilderRuntimeSemanticConsistency(runRecord{WorkspacePath: workspacePath}, []appruns.WorkspacePatchOperation{{
		Type:    "write_file",
		Path:    "lib/views/home_page.dart",
		Content: "class HomePageWidget {\n  void build() {\n    AppBar(title: const Text('体重记录 App'));\n  }\n}\n",
	}}, "")
	if err != nil {
		t.Fatalf("validateBuilderRuntimeSemanticConsistency() error = %v, want flutter title parameter allowed", err)
	}
}

func TestValidateBuilderRuntimeSemanticConsistencyRejectsRemovingFailureReferencedHelperSymbol(t *testing.T) {
	workspacePath := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspacePath, "lib", "repositories"), 0o755); err != nil {
		t.Fatalf("MkdirAll(repoDir) error = %v", err)
	}
	if err := os.MkdirAll(filepath.Join(workspacePath, "test"), 0o755); err != nil {
		t.Fatalf("MkdirAll(testDir) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(workspacePath, "lib", "repositories", "entry_repository.dart"), []byte(strings.Join([]string{
		"abstract class EntryRepository {}",
		"",
		"class InMemoryEntryRepository implements EntryRepository {}",
		"",
	}, "\n")), 0o600); err != nil {
		t.Fatalf("WriteFile(entry_repository.dart) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(workspacePath, "test", "widget_test.dart"), []byte("void main() {}\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(widget_test.dart) error = %v", err)
	}
	err := validateBuilderRuntimeSemanticConsistency(runRecord{
		WorkspacePath: workspacePath,
		TaskBundle: []appruns.TaskBundleItem{{
			TaskID:      "repair-check-flutter-analyze",
			TaskType:    appruns.BuilderRuntimeTaskTypeAnalyzeRepair,
			TargetPaths: []string{"lib/repositories/entry_repository.dart", "test/widget_test.dart"},
		}},
	}, []appruns.WorkspacePatchOperation{
		{Type: "write_file", Path: "lib/repositories/entry_repository.dart", Content: "abstract class EntryRepository {}\n"},
		{Type: "write_file", Path: "test/widget_test.dart", Content: "void main() {\n  // simplified scaffold\n}\n"},
	}, "test/widget_test.dart:11:34: Error: The function 'InMemoryEntryRepository' isn't defined")
	if err == nil {
		t.Fatalf("validateBuilderRuntimeSemanticConsistency() error = nil, want helper symbol preservation violation")
	}
	if !strings.Contains(err.Error(), "InMemoryEntryRepository -> lib/repositories/entry_repository.dart") {
		t.Fatalf("semantic consistency error = %q, want removed helper symbol details", err)
	}
}

func TestValidateBuilderRuntimeSemanticConsistencyRejectsUnresolvedFailureReferencedHelperUsage(t *testing.T) {
	workspacePath := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspacePath, "test"), 0o755); err != nil {
		t.Fatalf("MkdirAll(testDir) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(workspacePath, "test", "widget_test.dart"), []byte("void main() {}\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(widget_test.dart) error = %v", err)
	}
	err := validateBuilderRuntimeSemanticConsistency(runRecord{
		WorkspacePath: workspacePath,
		TaskBundle: []appruns.TaskBundleItem{{
			TaskID:      "repair-check-flutter-analyze",
			TaskType:    appruns.BuilderRuntimeTaskTypeAnalyzeRepair,
			TargetPaths: []string{"lib/repositories/entry_repository.dart", "test/widget_test.dart"},
		}},
	}, []appruns.WorkspacePatchOperation{
		{Type: "write_file", Path: "test/widget_test.dart", Content: "void main() {\n  final repo = InMemoryEntryRepository();\n  print(repo);\n}\n"},
	}, "test/widget_test.dart:11:34: Error: The function 'InMemoryEntryRepository' isn't defined")
	if err == nil {
		t.Fatalf("validateBuilderRuntimeSemanticConsistency() error = nil, want unresolved helper usage violation")
	}
	if !strings.Contains(err.Error(), "patch still references unresolved helper symbols") || !strings.Contains(err.Error(), "InMemoryEntryRepository") {
		t.Fatalf("semantic consistency error = %q, want unresolved helper usage details", err)
	}
}

func TestValidateBuilderRuntimeSemanticConsistencyAllowsRemovingLocalFailureReferencedHelperShim(t *testing.T) {
	workspacePath := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspacePath, "lib", "views"), 0o755); err != nil {
		t.Fatalf("MkdirAll(views) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(workspacePath, "lib", "views", "record_form_page.dart"), []byte(strings.Join([]string{
		"class RecordFormPage {",
		"  String build() => openlyLiteCopy;",
		"}",
		"",
		"String get openlyLiteCopy => '';",
	}, "\n")+"\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(record_form_page.dart) error = %v", err)
	}
	err := validateBuilderRuntimeSemanticConsistency(runRecord{
		WorkspacePath: workspacePath,
		TaskBundle: []appruns.TaskBundleItem{{
			TaskID:      "repair-check-flutter-analyze",
			TaskType:    appruns.BuilderRuntimeTaskTypeAnalyzeRepair,
			TargetPaths: []string{"lib/views/record_form_page.dart"},
		}},
	}, []appruns.WorkspacePatchOperation{{
		Type:    "write_file",
		Path:    "lib/views/record_form_page.dart",
		Content: "class RecordFormPage {\n  String build() => '';\n}\n",
	}}, "lib/views/record_form_page.dart:74:74: Error: Undefined name 'openlyLiteCopy'")
	if err != nil {
		t.Fatalf("validateBuilderRuntimeSemanticConsistency() error = %v, want local helper shim removal allowed", err)
	}
}

func TestValidateBuilderRuntimeSemanticConsistencyRejectsRemovedFailureReferencedMember(t *testing.T) {
	workspacePath := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspacePath, "lib", "repositories"), 0o755); err != nil {
		t.Fatalf("MkdirAll(repositories) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(workspacePath, "lib", "repositories", "record_repository.dart"), []byte(strings.Join([]string{
		"abstract class RecordRepository {",
		"  Future<void> loadTaskTagLinks();",
		"}",
	}, "\n")+"\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(record_repository.dart) error = %v", err)
	}
	err := validateBuilderRuntimeSemanticConsistency(runRecord{
		WorkspacePath: workspacePath,
		TaskBundle: []appruns.TaskBundleItem{{
			TaskID:      "repair-check-flutter-analyze",
			TaskType:    appruns.BuilderRuntimeTaskTypeAnalyzeRepair,
			TargetPaths: []string{"lib/repositories/record_repository.dart"},
		}},
	}, []appruns.WorkspacePatchOperation{{
		Type:    "write_file",
		Path:    "lib/repositories/record_repository.dart",
		Content: "abstract class RecordRepository {}\n",
	}}, "lib/repositories/record_repository.dart:10:3: Error: The method 'loadTaskTagLinks' isn't defined for the type 'RecordRepository'")
	if err == nil {
		t.Fatalf("validateBuilderRuntimeSemanticConsistency() error = nil, want removed member violation")
	}
	if !strings.Contains(err.Error(), "loadTaskTagLinks -> lib/repositories/record_repository.dart") {
		t.Fatalf("semantic consistency error = %q, want removed member details", err)
	}
}

func TestValidateBuilderRuntimeSemanticConsistencyRejectsUnresolvedFailureReferencedMemberUsage(t *testing.T) {
	workspacePath := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspacePath, "lib"), 0o755); err != nil {
		t.Fatalf("MkdirAll(lib) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(workspacePath, "lib", "main.dart"), []byte("void main() {}\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(main.dart) error = %v", err)
	}
	err := validateBuilderRuntimeSemanticConsistency(runRecord{
		WorkspacePath: workspacePath,
		TaskBundle: []appruns.TaskBundleItem{{
			TaskID:      "repair-check-flutter-analyze",
			TaskType:    appruns.BuilderRuntimeTaskTypeAnalyzeRepair,
			TargetPaths: []string{"lib/main.dart"},
		}},
	}, []appruns.WorkspacePatchOperation{{
		Type:    "write_file",
		Path:    "lib/main.dart",
		Content: "void main() {\n  _navigatorKey.currentState?.pushNamed('/');\n}\n",
	}}, "lib/main.dart:30:11: Error: Undefined name '_navigatorKey'")
	if err == nil {
		t.Fatalf("validateBuilderRuntimeSemanticConsistency() error = nil, want unresolved member usage violation")
	}
	if !strings.Contains(err.Error(), "patch still references unresolved helper symbols") || !strings.Contains(err.Error(), "_navigatorKey") {
		t.Fatalf("semantic consistency error = %q, want unresolved member usage details", err)
	}
}

func TestValidateBuilderRuntimeSemanticConsistencyRejectsUndeclaredPackageImports(t *testing.T) {
	workspacePath := t.TempDir()
	writeBuilderRuntimeWorkspaceFiles(t, workspacePath, map[string]string{
		"pubspec.yaml": strings.Join([]string{
			"name: relation_tracker",
			"dependencies:",
			"  flutter:",
			"    sdk: flutter",
			"  hive_flutter: ^1.1.0",
		}, "\n") + "\n",
	})
	err := validateBuilderRuntimeSemanticConsistency(runRecord{WorkspacePath: workspacePath}, []appruns.WorkspacePatchOperation{{
		Type: "write_file",
		Path: "lib/controllers/record_form_controller.dart",
		Content: strings.Join([]string{
			"import 'package:flutter/material.dart';",
			"import 'package:uuid/uuid.dart';",
			"import 'package:hive_flutter/hive_flutter.dart';",
			"",
			"class RecordFormController {}",
		}, "\n") + "\n",
	}}, "")
	if err == nil {
		t.Fatalf("validateBuilderRuntimeSemanticConsistency() error = nil, want undeclared package import violation")
	}
	if !strings.Contains(err.Error(), "patch introduced package imports not declared in pubspec.yaml") || !strings.Contains(err.Error(), "uuid") {
		t.Fatalf("semantic consistency error = %q, want undeclared package import details", err)
	}
	if strings.Contains(err.Error(), "hive_flutter") {
		t.Fatalf("semantic consistency error = %q, declared dependency should not be flagged", err)
	}
}

func TestValidateBuilderRuntimeSemanticConsistencyAllowsDeclaredAndLocalPackageImports(t *testing.T) {
	workspacePath := t.TempDir()
	writeBuilderRuntimeWorkspaceFiles(t, workspacePath, map[string]string{
		"pubspec.yaml": strings.Join([]string{
			"name: relation_tracker",
			"dependencies:",
			"  flutter:",
			"    sdk: flutter",
			"  hive_flutter: ^1.1.0",
		}, "\n") + "\n",
	})
	err := validateBuilderRuntimeSemanticConsistency(runRecord{WorkspacePath: workspacePath}, []appruns.WorkspacePatchOperation{{
		Type: "write_file",
		Path: "lib/main.dart",
		Content: strings.Join([]string{
			"import 'package:flutter/material.dart';",
			"import 'package:hive_flutter/hive_flutter.dart';",
			"import 'package:relation_tracker/repositories/record_repository.dart';",
			"",
			"void main() {}",
		}, "\n") + "\n",
	}}, "")
	if err != nil {
		t.Fatalf("validateBuilderRuntimeSemanticConsistency() error = %v, want declared/local package imports allowed", err)
	}
}

func TestValidateBuilderRuntimeSemanticConsistencyRejectsInvalidBookkeepingRepositoryValueShape(t *testing.T) {
	workspacePath := createBuilderRuntimeBookkeepingWorkspace(t, nil)
	err := validateBuilderRuntimeSemanticConsistency(runRecord{WorkspacePath: workspacePath}, []appruns.WorkspacePatchOperation{{
		Type: "write_file",
		Path: "lib/repositories/local_store.dart",
		Content: strings.Join([]string{
			"class HiveEntryRepository {",
			"  late Box<List<Map<String, dynamic>>> _box;",
			"  Future<void> load() async {",
			"    final rawEntries = _box.get('entries', defaultValue: <List<Map<String, dynamic>>>[]) as List<Map<String, dynamic>>;",
			"    print(rawEntries);",
			"  }",
			"}",
		}, "\n"),
	}}, "")
	if err == nil {
		t.Fatalf("validateBuilderRuntimeSemanticConsistency() error = nil, want bookkeeping repository shape violation")
	}
	if !strings.Contains(err.Error(), "invalid bookkeeping repository value shapes") {
		t.Fatalf("semantic consistency error = %q, want bookkeeping repository shape details", err)
	}
}

func TestValidateBuilderRuntimeSemanticConsistencySkipsBookkeepingGuardsOutsideBookkeepingDomain(t *testing.T) {
	workspacePath := t.TempDir()
	err := validateBuilderRuntimeSemanticConsistency(runRecord{WorkspacePath: workspacePath}, []appruns.WorkspacePatchOperation{{
		Type: "write_file",
		Path: "lib/repositories/local_store.dart",
		Content: strings.Join([]string{
			"class HiveEntryRepository {",
			"  late Box<List<Map<String, dynamic>>> _box;",
			"  Future<void> load() async {",
			"    final rawEntries = _box.get('entries', defaultValue: <List<Map<String, dynamic>>>[]) as List<Map<String, dynamic>>;",
			"    print(rawEntries);",
			"  }",
			"}",
		}, "\n"),
	}}, "")
	if err != nil {
		t.Fatalf("validateBuilderRuntimeSemanticConsistency() error = %v, want non-bookkeeping workspace to skip bookkeeping-only guards", err)
	}
}

func TestValidateBuilderRuntimeSemanticConsistencyRejectsInvalidBookkeepingFieldReferences(t *testing.T) {
	workspacePath := createBuilderRuntimeBookkeepingWorkspace(t, map[string]string{
		"lib/models/summary.dart": strings.Join([]string{
			"class Summary {",
			"  const Summary({required this.balance});",
			"  final double balance;",
			"}",
			"",
		}, "\n"),
	})
	err := validateBuilderRuntimeSemanticConsistency(runRecord{WorkspacePath: workspacePath}, []appruns.WorkspacePatchOperation{{
		Type: "write_file",
		Path: "lib/views/home_page.dart",
		Content: strings.Join([]string{
			"class HomePage {",
			"  String render(dynamic summary, dynamic entry) {",
			"    return '${summary.entryCount}-${entry.date.year}';",
			"  }",
			"}",
		}, "\n"),
	}}, "")
	if err == nil {
		t.Fatalf("validateBuilderRuntimeSemanticConsistency() error = nil, want bookkeeping field reference violation")
	}
	if !strings.Contains(err.Error(), "summary.entryCount") || !strings.Contains(err.Error(), "entry.date") {
		t.Fatalf("semantic consistency error = %q, want bookkeeping field reference details", err)
	}
}

func TestValidateBuilderRuntimeSemanticConsistencyAllowsBookkeepingFieldReferencesWhenModelPatchAddsFields(t *testing.T) {
	workspacePath := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspacePath, "lib", "models"), 0o755); err != nil {
		t.Fatalf("MkdirAll(models) error = %v", err)
	}
	err := validateBuilderRuntimeSemanticConsistency(runRecord{WorkspacePath: workspacePath}, []appruns.WorkspacePatchOperation{
		{
			Type: "write_file",
			Path: "lib/models/entry.dart",
			Content: strings.Join([]string{
				"class BookkeepingEntry {",
				"  const BookkeepingEntry({required this.date});",
				"  final DateTime date;",
				"}",
			}, "\n"),
		},
		{
			Type: "write_file",
			Path: "lib/models/summary.dart",
			Content: strings.Join([]string{
				"class Summary {",
				"  const Summary({required this.entryCount});",
				"  final int entryCount;",
				"}",
			}, "\n"),
		},
		{
			Type: "write_file",
			Path: "lib/views/home_page.dart",
			Content: strings.Join([]string{
				"class HomePage {",
				"  String render(dynamic summary, dynamic entry) {",
				"    return '${summary.entryCount}-${entry.date.year}';",
				"  }",
				"}",
			}, "\n"),
		},
	}, "")
	if err != nil {
		t.Fatalf("validateBuilderRuntimeSemanticConsistency() error = %v, want model-consistent bookkeeping field references allowed", err)
	}
}

func TestValidateBuilderRuntimeSemanticConsistencyRejectsUnknownBookkeepingSnapshotFieldsInViewAndTest(t *testing.T) {
	workspacePath := createBuilderRuntimeBookkeepingWorkspace(t, map[string]string{
		"lib/models/entry.dart": strings.Join([]string{
			"class BookkeepingEntry {",
			"  const BookkeepingEntry({required this.occurredOn, this.note, required this.category, required this.amount});",
			"  final DateTime occurredOn;",
			"  final String? note;",
			"  final String category;",
			"  final double amount;",
			"}",
			"",
		}, "\n"),
		"lib/models/summary.dart": strings.Join([]string{
			"class Summary {",
			"  const Summary({required this.balance, required this.incomeTotal, required this.expenseTotal});",
			"  final double balance;",
			"  final double incomeTotal;",
			"  final double expenseTotal;",
			"}",
			"",
		}, "\n"),
	})
	err := validateBuilderRuntimeSemanticConsistency(runRecord{WorkspacePath: workspacePath}, []appruns.WorkspacePatchOperation{
		{
			Type: "write_file",
			Path: "lib/views/home_page.dart",
			Content: strings.Join([]string{
				"class HomePage {",
				"  String render(dynamic summary, dynamic entry) {",
				"    return '${summary.totalCount}-${entry.title}';",
				"  }",
				"}",
			}, "\n"),
		},
		{
			Type: "write_file",
			Path: "test/widget_test.dart",
			Content: strings.Join([]string{
				"void main() {",
				"  print(summary.totalCount);",
				"  print(entry.title);",
				"}",
			}, "\n"),
		},
	}, "")
	if err == nil {
		t.Fatalf("validateBuilderRuntimeSemanticConsistency() error = nil, want bookkeeping snapshot violation")
	}
	for _, want := range []string{"summary.totalCount", "entry.title"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("semantic consistency error = %q, want %q", err, want)
		}
	}
}

func TestValidateBuilderRuntimeSemanticConsistencyAllowsKnownBookkeepingSnapshotFieldsInViewAndTest(t *testing.T) {
	workspacePath := createBuilderRuntimeBookkeepingWorkspace(t, map[string]string{
		"lib/models/entry.dart": strings.Join([]string{
			"class BookkeepingEntry {",
			"  const BookkeepingEntry({required this.occurredOn, this.note, required this.category, required this.amount});",
			"  final DateTime occurredOn;",
			"  final String? note;",
			"  final String category;",
			"  final double amount;",
			"}",
			"",
		}, "\n"),
		"lib/models/summary.dart": strings.Join([]string{
			"class Summary {",
			"  const Summary({required this.balance, required this.incomeTotal, required this.expenseTotal});",
			"  final double balance;",
			"  final double incomeTotal;",
			"  final double expenseTotal;",
			"}",
			"",
		}, "\n"),
	})
	err := validateBuilderRuntimeSemanticConsistency(runRecord{WorkspacePath: workspacePath}, []appruns.WorkspacePatchOperation{
		{
			Type: "write_file",
			Path: "lib/views/home_page.dart",
			Content: strings.Join([]string{
				"class HomePage {",
				"  String render(dynamic summary, dynamic entry) {",
				"    return '${summary.balance}-${entry.category}-${entry.amount}';",
				"  }",
				"}",
			}, "\n"),
		},
		{
			Type: "write_file",
			Path: "test/widget_test.dart",
			Content: strings.Join([]string{
				"import 'package:bookkeeping_lite/models/entry.dart';",
				"void main() {",
				"  print(entry.occurredOn);",
				"  print(summary.incomeTotal);",
				"}",
			}, "\n"),
		},
	}, "")
	if err != nil {
		t.Fatalf("validateBuilderRuntimeSemanticConsistency() error = %v, want known bookkeeping snapshot fields allowed", err)
	}
}

func TestValidateBuilderRuntimeSemanticConsistencyRejectsBookkeepingEntryUsageWithoutImport(t *testing.T) {
	workspacePath := createBuilderRuntimeBookkeepingWorkspace(t, nil)
	err := validateBuilderRuntimeSemanticConsistency(runRecord{WorkspacePath: workspacePath}, []appruns.WorkspacePatchOperation{{
		Type: "write_file",
		Path: "test/widget_test.dart",
		Content: strings.Join([]string{
			"import 'package:flutter_test/flutter_test.dart';",
			"import 'package:bookkeeping_lite/repositories/entry_repository.dart';",
			"",
			"class InMemoryEntryRepository extends EntryRepository {",
			"  final List<BookkeepingEntry> entries = [];",
			"}",
		}, "\n"),
	}}, "")
	if err == nil {
		t.Fatalf("validateBuilderRuntimeSemanticConsistency() error = nil, want missing entry model import violation")
	}
	if !strings.Contains(err.Error(), "uses BookkeepingEntry without importing the entry model") {
		t.Fatalf("semantic consistency error = %q, want missing entry model import details", err)
	}
}

func TestValidateBuilderRuntimeSemanticConsistencyAllowsBookkeepingEntryUsageWithImport(t *testing.T) {
	workspacePath := createBuilderRuntimeBookkeepingWorkspace(t, nil)
	err := validateBuilderRuntimeSemanticConsistency(runRecord{WorkspacePath: workspacePath}, []appruns.WorkspacePatchOperation{{
		Type: "write_file",
		Path: "test/widget_test.dart",
		Content: strings.Join([]string{
			"import 'package:flutter_test/flutter_test.dart';",
			"import 'package:bookkeeping_lite/models/entry.dart';",
			"import 'package:bookkeeping_lite/repositories/entry_repository.dart';",
			"",
			"class InMemoryEntryRepository extends EntryRepository {",
			"  final List<BookkeepingEntry> entries = [];",
			"}",
		}, "\n"),
	}}, "")
	if err != nil {
		t.Fatalf("validateBuilderRuntimeSemanticConsistency() error = %v, want imported BookkeepingEntry allowed", err)
	}
}

func TestValidateBuilderRuntimeSemanticConsistencyRejectsUndefinedInMemoryEntryRepositoryInWidgetTest(t *testing.T) {
	workspacePath := createBuilderRuntimeBookkeepingWorkspace(t, nil)
	err := validateBuilderRuntimeSemanticConsistency(runRecord{WorkspacePath: workspacePath}, []appruns.WorkspacePatchOperation{{
		Type: "write_file",
		Path: "test/widget_test.dart",
		Content: strings.Join([]string{
			"import 'package:flutter_test/flutter_test.dart';",
			"import 'package:bookkeeping_lite/main.dart';",
			"",
			"void main() {",
			"  testWidgets('smoke', (tester) async {",
			"    await tester.pumpWidget(BookkeepingApp(repository: InMemoryEntryRepository()));",
			"  });",
			"}",
		}, "\n"),
	}}, "")
	if err == nil {
		t.Fatalf("validateBuilderRuntimeSemanticConsistency() error = nil, want undefined InMemoryEntryRepository violation")
	}
	if !strings.Contains(err.Error(), "instantiates InMemoryEntryRepository even though the helper is not defined") {
		t.Fatalf("semantic consistency error = %q, want undefined InMemoryEntryRepository details", err)
	}
}

func TestValidateBuilderRuntimeSemanticConsistencyAllowsDefinedInMemoryEntryRepositoryInRepository(t *testing.T) {
	workspacePath := createBuilderRuntimeBookkeepingWorkspace(t, nil)
	err := validateBuilderRuntimeSemanticConsistency(runRecord{WorkspacePath: workspacePath}, []appruns.WorkspacePatchOperation{
		{
			Type: "write_file",
			Path: "lib/repositories/local_store.dart",
			Content: strings.Join([]string{
				"abstract class EntryRepository {}",
				"class InMemoryEntryRepository implements EntryRepository {}",
			}, "\n"),
		},
		{
			Type: "write_file",
			Path: "test/widget_test.dart",
			Content: strings.Join([]string{
				"import 'package:flutter_test/flutter_test.dart';",
				"import 'package:bookkeeping_lite/main.dart';",
				"import 'package:bookkeeping_lite/repositories/local_store.dart';",
				"",
				"void main() {",
				"  testWidgets('smoke', (tester) async {",
				"    await tester.pumpWidget(BookkeepingApp(repository: InMemoryEntryRepository()));",
				"  });",
				"}",
			}, "\n"),
		},
	}, "")
	if err != nil {
		t.Fatalf("validateBuilderRuntimeSemanticConsistency() error = %v, want defined InMemoryEntryRepository allowed", err)
	}
}

func TestShouldRetryBuilderRuntimeWithUpgradeOnAnalyzeRepairSemanticConflict(t *testing.T) {
	should := shouldRetryBuilderRuntimeWithUpgrade(runRecord{
		BuilderRuntime: &appruns.BuilderRuntimePlan{
			UpgradeModel: appruns.BuilderRuntimeModelRef{Primary: "upgrade-model"},
		},
	}, &appruns.BuilderRuntimeExecutionStats{TaskType: appruns.BuilderRuntimeTaskTypeAnalyzeRepair, SelectedModel: "local-model", Attempts: 1}, "semantic_conflict")
	if !should {
		t.Fatalf("shouldRetryBuilderRuntimeWithUpgrade() = false, want true for analyze repair semantic conflict")
	}
}

func TestShouldRetryBuilderRuntimeSemanticConflictRepairOnAnalyzeHelperSymbolConflict(t *testing.T) {
	should := shouldRetryBuilderRuntimeSemanticConflictRepair(
		appruns.BuilderRuntimeTaskTypeAnalyzeRepair,
		&appruns.BuilderRuntimeExecutionStats{Attempts: 1},
		"gemma4-26b-local",
		`{"patch_id":"repair","operations":[]}`,
		errors.New("patch removed required helper symbols referenced by current validation failures: recordRepository -> lib/views/record_form_page.dart"),
	)
	if !should {
		t.Fatal("shouldRetryBuilderRuntimeSemanticConflictRepair() = false, want true for analyze helper-symbol semantic conflict")
	}
}

func TestShouldRetryBuilderRuntimeWithUpgradeOnUpgradeModelSemanticConflictFallback(t *testing.T) {
	should := shouldRetryBuilderRuntimeWithUpgrade(runRecord{
		BuilderRuntime: &appruns.BuilderRuntimePlan{
			UpgradeModel: appruns.BuilderRuntimeModelRef{Primary: "qwen2.5-coder-32b-local", Fallbacks: []string{"gpt-4o-mini"}},
		},
	}, &appruns.BuilderRuntimeExecutionStats{TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, SelectedModel: "qwen2.5-coder-32b-local", UpgradeApplied: true, Attempts: 1}, "semantic_conflict")
	if !should {
		t.Fatalf("shouldRetryBuilderRuntimeWithUpgrade() = false, want true for upgrade-model semantic conflict fallback")
	}
}

func TestShouldRetryBuilderRuntimeTransientModelRequestFailureOnRateLimit(t *testing.T) {
	rateLimitErr := errors.New("builder runtime model request failed: qwen3-coder-480b: API request failed: Status: 429 Body: {\"status\":429,\"title\":\"Too Many Requests\"}")
	if !shouldRetryBuilderRuntimeTransientModelRequestFailure(1, rateLimitErr) {
		t.Fatal("shouldRetryBuilderRuntimeTransientModelRequestFailure() = false, want true for rate limit failure")
	}
	serviceUnavailableErr := errors.New("builder runtime model request failed: qwen3-coder-480b: API request failed: Status: 503 Body: {\"status\":503,\"title\":\"Service Unavailable\"}")
	if !shouldRetryBuilderRuntimeTransientModelRequestFailure(1, serviceUnavailableErr) {
		t.Fatal("shouldRetryBuilderRuntimeTransientModelRequestFailure() = false, want true for transient 503 failure")
	}
	if shouldRetryBuilderRuntimeTransientModelRequestFailure(2, rateLimitErr) {
		t.Fatal("shouldRetryBuilderRuntimeTransientModelRequestFailure() = true at requestAttempts=2, want false")
	}
	authErr := errors.New("API request failed: Status: 401 Body: unauthorized")
	if shouldRetryBuilderRuntimeTransientModelRequestFailure(1, authErr) {
		t.Fatal("shouldRetryBuilderRuntimeTransientModelRequestFailure() = true for auth failure, want false")
	}
}

func TestBuildBuilderRuntimePromptIncludesSchemaRepairInstructions(t *testing.T) {
	run := runRecord{
		GoalSummary:   "repair flutter ui",
		WorkspacePath: t.TempDir(),
		TaskBundle: []appruns.TaskBundleItem{{
			TaskID:      "task-home",
			TaskType:    appruns.BuilderRuntimeTaskTypeSingleFileEdit,
			TargetPaths: []string{"lib/views/home_page.dart"},
		}},
	}
	roundInput := appruns.RoundInput{
		TaskBundle: run.TaskBundle,
		AllowedPaths: []string{
			"lib/**",
		},
	}
	route := appruns.BuilderRuntimeTaskRoute{
		TaskID:      "task-home",
		TaskType:    appruns.BuilderRuntimeTaskTypeSingleFileEdit,
		RouteSource: "upgrade_model",
	}

	prompt, err := buildBuilderRuntimePrompt(BuilderRuntimePatchRequest{
		Run:          run,
		RoundInput:   roundInput,
		Route:        route,
		RepairOnly:   true,
		PreviousErr:  "replace_block requires new_content",
		PreviousBody: `{"patch_id":"bad","operations":[{"replace_block":{"path":"lib/views/home_page.dart"}}]}`,
	})
	if err != nil {
		t.Fatalf("buildBuilderRuntimePrompt() error = %v", err)
	}
	if !strings.Contains(prompt, "Schema repair mode") {
		t.Fatalf("prompt missing schema repair mode: %q", prompt)
	}
	if !strings.Contains(prompt, "Previous parser error: replace_block requires new_content") {
		t.Fatalf("prompt missing previous parser error: %q", prompt)
	}
	if !strings.Contains(prompt, "Previous invalid response:") {
		t.Fatalf("prompt missing previous invalid response block: %q", prompt)
	}
	if !strings.Contains(prompt, "Never emit replace_block without new_content") {
		t.Fatalf("prompt missing replace_block repair guidance: %q", prompt)
	}
	if !strings.Contains(prompt, "do not emit another ambiguous replace_block") {
		t.Fatalf("prompt missing ambiguous replace_block repair guidance: %q", prompt)
	}
}

func TestBuildBuilderRuntimePromptSchemaRepairHandlesEmptyPreviousResponse(t *testing.T) {
	run := runRecord{
		GoalSummary:   "repair home page",
		WorkspacePath: t.TempDir(),
		TaskBundle: []appruns.TaskBundleItem{{
			TaskID:      "task-home",
			TaskType:    appruns.BuilderRuntimeTaskTypeSingleFileEdit,
			TargetPaths: []string{"lib/views/home_page.dart"},
		}},
	}
	roundInput := appruns.RoundInput{
		TaskBundle: run.TaskBundle,
		AllowedPaths: []string{
			"lib/**",
		},
	}
	route := appruns.BuilderRuntimeTaskRoute{
		TaskID:      "task-home",
		TaskType:    appruns.BuilderRuntimeTaskTypeSingleFileEdit,
		RouteSource: "upgrade_model",
	}

	prompt, err := buildBuilderRuntimePrompt(BuilderRuntimePatchRequest{
		Run:         run,
		RoundInput:  roundInput,
		Route:       route,
		RepairOnly:  true,
		PreviousErr: "unexpected end of JSON input",
	})
	if err != nil {
		t.Fatalf("buildBuilderRuntimePrompt() error = %v", err)
	}
	if !strings.Contains(prompt, "previous response body was empty or truncated") {
		t.Fatalf("prompt missing empty-response repair guidance: %q", prompt)
	}
	if strings.Contains(prompt, "Previous invalid response:") {
		t.Fatalf("prompt should not include previous invalid response block when body is empty: %q", prompt)
	}
}

func TestBuildBuilderRuntimePromptWarnsAboutAmbiguousReplaceBlockAnchors(t *testing.T) {
	run := runRecord{
		GoalSummary:   "repair repository",
		WorkspacePath: t.TempDir(),
		TaskBundle: []appruns.TaskBundleItem{{
			TaskID:      "task-entry-repo",
			TaskType:    appruns.BuilderRuntimeTaskTypeSingleFileEdit,
			TargetPaths: []string{"lib/repositories/entry_repository.dart"},
		}},
	}
	roundInput := appruns.RoundInput{
		TaskBundle: run.TaskBundle,
		AllowedPaths: []string{
			"lib/**",
		},
	}
	route := appruns.BuilderRuntimeTaskRoute{
		TaskID:      "task-entry-repo",
		TaskType:    appruns.BuilderRuntimeTaskTypeSingleFileEdit,
		RouteSource: "default_model",
	}

	prompt, err := buildBuilderRuntimePrompt(BuilderRuntimePatchRequest{Run: run, RoundInput: roundInput, Route: route})
	if err != nil {
		t.Fatalf("buildBuilderRuntimePrompt() error = %v", err)
	}
	if !strings.Contains(prompt, "Never use replace_block with an anchor or old_content that may match multiple regions") {
		t.Fatalf("prompt missing ambiguous anchor warning: %q", prompt)
	}
	if !strings.Contains(prompt, "lib/repositories/*.dart") {
		t.Fatalf("prompt missing repository write_file guidance: %q", prompt)
	}
}

func TestBuilderRuntimeRepairModelAliasesPrioritizesPreferredAlias(t *testing.T) {
	got := builderRuntimeRepairModelAliases(appruns.BuilderRuntimeModelRef{
		Primary:   "qwen2.5-coder-32b-local",
		Fallbacks: []string{"gpt-4o-mini", "qwen2.5-coder-32b-local"},
	}, "gpt-4o-mini")
	if len(got) != 2 || got[0] != "gpt-4o-mini" || got[1] != "qwen2.5-coder-32b-local" {
		t.Fatalf("builderRuntimeRepairModelAliases() = %#v, want [gpt-4o-mini qwen2.5-coder-32b-local]", got)
	}
}

func TestSelectBuilderRuntimeRoutePrefersAnalyzeRepairTask(t *testing.T) {
	route, upgraded := selectBuilderRuntimeRoute(runRecord{
		TaskBundle: []appruns.TaskBundleItem{
			{TaskID: "repair-check-flutter-analyze", Category: appruns.TaskCategoryValidation, TaskType: appruns.BuilderRuntimeTaskTypeAnalyzeRepair, TargetPaths: []string{"lib/controllers/record_form_controller.dart"}},
			{TaskID: "task-generic-domain-models", Category: appruns.TaskCategoryDomain, TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, RouteHint: appruns.TaskRouteHintStrongModel, RiskLevel: appruns.TaskRiskLevelHigh, TargetPaths: []string{"lib/models/record.dart"}},
		},
		BuilderRuntime: &appruns.BuilderRuntimePlan{
			Enabled:      true,
			DefaultModel: appruns.BuilderRuntimeModelRef{Primary: "default-model"},
			TaskRoutes: []appruns.BuilderRuntimeTaskRoute{{
				TaskType:    appruns.BuilderRuntimeTaskTypeAnalyzeRepair,
				RouteSource: "task_route",
				Model:       appruns.BuilderRuntimeModelRef{Primary: "repair-model"},
			}},
		},
	})
	if upgraded {
		t.Fatalf("upgraded = true, want false")
	}
	if route.TaskID != "repair-check-flutter-analyze" {
		t.Fatalf("route.TaskID = %q, want repair-check-flutter-analyze", route.TaskID)
	}
	if route.TaskType != appruns.BuilderRuntimeTaskTypeAnalyzeRepair {
		t.Fatalf("route.TaskType = %q, want analyze_repair", route.TaskType)
	}
	if route.Model.Primary != "repair-model" {
		t.Fatalf("route.Model.Primary = %q, want repair-model", route.Model.Primary)
	}
}

func TestSelectBuilderRuntimeRouteSkipsValidatedTasksByDependencyOrder(t *testing.T) {
	route, upgraded := selectBuilderRuntimeRoute(runRecord{
		TaskBundle: []appruns.TaskBundleItem{
			{TaskID: "task-create-record-model", TaskType: appruns.BuilderRuntimeTaskTypeSingleFileEdit, TargetPaths: []string{"lib/models/record.dart"}},
			{TaskID: "task-create-repository", TaskType: appruns.BuilderRuntimeTaskTypeSingleFileEdit, Dependencies: []string{"task-create-record-model"}, TargetPaths: []string{"lib/repositories/record_repository.dart"}},
			{TaskID: "task-create-home-controller", TaskType: appruns.BuilderRuntimeTaskTypeSingleFileEdit, Dependencies: []string{"task-create-repository"}, TargetPaths: []string{"lib/controllers/home_controller.dart"}},
		},
		RoundState: &appruns.RoundState{TaskStatuses: map[string]appruns.BuilderRuntimeTaskStatus{
			"task-create-record-model": appruns.BuilderRuntimeTaskStatusValidated,
			"task-create-repository":   appruns.BuilderRuntimeTaskStatusCreated,
		}},
		BuilderRuntime: &appruns.BuilderRuntimePlan{
			Enabled:      true,
			DefaultModel: appruns.BuilderRuntimeModelRef{Primary: "default-model"},
			TaskRoutes: []appruns.BuilderRuntimeTaskRoute{{
				TaskID:      "task-create-repository",
				TaskType:    appruns.BuilderRuntimeTaskTypeSingleFileEdit,
				RouteSource: "task_route",
				Model:       appruns.BuilderRuntimeModelRef{Primary: "repo-model"},
			}},
		},
	})
	if upgraded {
		t.Fatalf("upgraded = true, want false")
	}
	if route.TaskID != "task-create-repository" {
		t.Fatalf("route.TaskID = %q, want task-create-repository", route.TaskID)
	}
	if route.Model.Primary != "repo-model" {
		t.Fatalf("route.Model.Primary = %q, want repo-model", route.Model.Primary)
	}
}

func TestPreferredBuilderRuntimeTaskBlocksOnUnvalidatedDependencies(t *testing.T) {
	selected := preferredBuilderRuntimeTask([]appruns.TaskBundleItem{
		{TaskID: "task-create-record-model", TaskType: appruns.BuilderRuntimeTaskTypeSingleFileEdit, TargetPaths: []string{"lib/models/record.dart"}},
		{TaskID: "task-create-repository", TaskType: appruns.BuilderRuntimeTaskTypeSingleFileEdit, Dependencies: []string{"task-create-record-model"}, TargetPaths: []string{"lib/repositories/record_repository.dart"}},
	}, &appruns.RoundState{TaskStatuses: map[string]appruns.BuilderRuntimeTaskStatus{
		"task-create-record-model": appruns.BuilderRuntimeTaskStatusCreated,
	}})
	if selected.TaskID != "task-create-record-model" {
		t.Fatalf("selected.TaskID = %q, want task-create-record-model", selected.TaskID)
	}
}

func TestValidateBuilderRuntimeTaskOutputsRejectsUnresolvedLocalImport(t *testing.T) {
	workspacePath := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspacePath, "lib", "controllers"), 0o755); err != nil {
		t.Fatalf("MkdirAll(controllerDir) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(workspacePath, "lib", "controllers", "home_controller.dart"), []byte("import '../repositories/record_repository.dart';\nclass HomeController {}\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(home_controller.dart) error = %v", err)
	}
	err := validateBuilderRuntimeTaskOutputs(runRecord{
		WorkspacePath: workspacePath,
		TaskBundle: []appruns.TaskBundleItem{{
			TaskID:      "task-create-home-controller",
			TaskType:    appruns.BuilderRuntimeTaskTypeSingleFileEdit,
			TargetPaths: []string{"lib/controllers/home_controller.dart"},
		}},
	}, "task-create-home-controller")
	if err == nil {
		t.Fatal("validateBuilderRuntimeTaskOutputs() error = nil, want unresolved local import")
	}
	if !strings.Contains(err.Error(), "unresolved local import/export/part") {
		t.Fatalf("validateBuilderRuntimeTaskOutputs() error = %q, want unresolved import detail", err)
	}
}

func TestValidateBuilderRuntimeTaskOutputsRejectsCounterDemoShellInMain(t *testing.T) {
	workspacePath := t.TempDir()
	writeBuilderRuntimeWorkspaceFiles(t, workspacePath, map[string]string{
		"lib/main.dart": strings.Join([]string{
			"import 'package:flutter/material.dart';",
			"",
			"void main() {",
			"  runApp(const MaterialApp(home: MyHomePage()));",
			"}",
			"",
			"class MyHomePage extends StatelessWidget {",
			"  const MyHomePage({super.key});",
			"",
			"  @override",
			"  Widget build(BuildContext context) => const Placeholder();",
			"}",
		}, "\n") + "\n",
	})
	err := validateBuilderRuntimeTaskOutputs(runRecord{
		WorkspacePath: workspacePath,
		TaskBundle: []appruns.TaskBundleItem{{
			TaskID:      "task-bind-app-entry",
			TaskType:    appruns.BuilderRuntimeTaskTypeSingleFileEdit,
			TargetPaths: []string{"lib/main.dart"},
		}},
	}, "task-bind-app-entry")
	if err == nil {
		t.Fatal("validateBuilderRuntimeTaskOutputs() error = nil, want counter-demo shell violation")
	}
	if !strings.Contains(err.Error(), "counter-demo shell MyHomePage") {
		t.Fatalf("validateBuilderRuntimeTaskOutputs() error = %q, want MyHomePage counter-demo detail", err)
	}
}

func TestValidateBuilderRuntimeTaskOutputsRejectsNestedMaterialAppInMain(t *testing.T) {
	workspacePath := t.TempDir()
	writeBuilderRuntimeWorkspaceFiles(t, workspacePath, map[string]string{
		"lib/main.dart": strings.Join([]string{
			"import 'package:flutter/material.dart';",
			"",
			"void main() {",
			"  runApp(const WeightTrackerApp());",
			"}",
			"",
			"class WeightTrackerApp extends StatelessWidget {",
			"  const WeightTrackerApp({super.key});",
			"",
			"  @override",
			"  Widget build(BuildContext context) {",
			"    return MaterialApp(",
			"      home: NestedShell(),",
			"    );",
			"  }",
			"}",
			"",
			"class NestedShell extends StatelessWidget {",
			"  @override",
			"  Widget build(BuildContext context) {",
			"    return MaterialApp(",
			"      home: const Scaffold(body: Center(child: Text('ok'))),",
			"    );",
			"  }",
			"}",
		}, "\n") + "\n",
	})
	err := validateBuilderRuntimeTaskOutputs(runRecord{
		WorkspacePath: workspacePath,
		TaskBundle: []appruns.TaskBundleItem{{
			TaskID:      "task-bind-app-entry",
			TaskType:    appruns.BuilderRuntimeTaskTypeSingleFileEdit,
			TargetPaths: []string{"lib/main.dart"},
		}},
	}, "task-bind-app-entry")
	if err == nil {
		t.Fatal("validateBuilderRuntimeTaskOutputs() error = nil, want nested MaterialApp violation")
	}
	if !strings.Contains(err.Error(), "nested MaterialApp") {
		t.Fatalf("validateBuilderRuntimeTaskOutputs() error = %q, want nested MaterialApp detail", err)
	}
}

func TestValidateBuilderRuntimeTaskOutputsRejectsMissingRequiredViewNamedParametersInMain(t *testing.T) {
	workspacePath := t.TempDir()
	writeBuilderRuntimeWorkspaceFiles(t, workspacePath, map[string]string{
		"lib/main.dart": strings.Join([]string{
			"import 'package:flutter/material.dart';",
			"",
			"import 'views/home_page.dart';",
			"",
			"void main() {",
			"  runApp(const WeightTrackerApp());",
			"}",
			"",
			"class WeightTrackerApp extends StatelessWidget {",
			"  const WeightTrackerApp({super.key});",
			"",
			"  @override",
			"  Widget build(BuildContext context) {",
			"    return const MaterialApp(home: HomePage());",
			"  }",
			"}",
		}, "\n") + "\n",
		"lib/views/home_page.dart": strings.Join([]string{
			"import 'package:flutter/material.dart';",
			"",
			"class HomePage extends StatelessWidget {",
			"  const HomePage({",
			"    super.key,",
			"    required this.controller,",
			"    required this.onCreateRecord,",
			"    required this.onViewAllRecords,",
			"    required this.onOpenRecordDetail,",
			"  });",
			"",
			"  final Object controller;",
			"  final Future<void> Function() onCreateRecord;",
			"  final Future<void> Function() onViewAllRecords;",
			"  final Future<void> Function(Object record) onOpenRecordDetail;",
			"",
			"  @override",
			"  Widget build(BuildContext context) => const Placeholder();",
			"}",
		}, "\n") + "\n",
	})
	err := validateBuilderRuntimeTaskOutputs(runRecord{
		WorkspacePath: workspacePath,
		TaskBundle: []appruns.TaskBundleItem{{
			TaskID:      "task-bind-app-entry",
			TaskType:    appruns.BuilderRuntimeTaskTypeSingleFileEdit,
			TargetPaths: []string{"lib/main.dart", "lib/views/home_page.dart"},
		}},
	}, "task-bind-app-entry")
	if err == nil {
		t.Fatal("validateBuilderRuntimeTaskOutputs() error = nil, want missing required named parameter violation")
	}
	if !strings.Contains(err.Error(), "omits required named parameters") || !strings.Contains(err.Error(), "HomePage") {
		t.Fatalf("validateBuilderRuntimeTaskOutputs() error = %q, want missing HomePage named parameters detail", err)
	}
}

func TestValidateBuilderRuntimeTaskOutputsRejectsUnsupportedViewNamedParametersInMain(t *testing.T) {
	workspacePath := t.TempDir()
	writeBuilderRuntimeWorkspaceFiles(t, workspacePath, map[string]string{
		"lib/main.dart": strings.Join([]string{
			"import 'package:flutter/material.dart';",
			"",
			"import 'views/record_form_page.dart';",
			"",
			"void main() {",
			"  runApp(const WeightTrackerApp());",
			"}",
			"",
			"class WeightTrackerApp extends StatelessWidget {",
			"  const WeightTrackerApp({super.key});",
			"",
			"  @override",
			"  Widget build(BuildContext context) {",
			"    final repository = Object();",
			"    return MaterialApp(home: RecordFormPage(recordRepository: repository));",
			"  }",
			"}",
		}, "\n") + "\n",
		"lib/views/record_form_page.dart": strings.Join([]string{
			"import 'package:flutter/material.dart';",
			"",
			"class RecordFormPage extends StatelessWidget {",
			"  const RecordFormPage({super.key, this.initialRecord});",
			"",
			"  final Object? initialRecord;",
			"",
			"  @override",
			"  Widget build(BuildContext context) => const Placeholder();",
			"}",
		}, "\n") + "\n",
	})
	err := validateBuilderRuntimeTaskOutputs(runRecord{
		WorkspacePath: workspacePath,
		TaskBundle: []appruns.TaskBundleItem{{
			TaskID:      "task-bind-app-entry",
			TaskType:    appruns.BuilderRuntimeTaskTypeSingleFileEdit,
			TargetPaths: []string{"lib/main.dart", "lib/views/record_form_page.dart"},
		}},
	}, "task-bind-app-entry")
	if err == nil {
		t.Fatal("validateBuilderRuntimeTaskOutputs() error = nil, want unsupported named parameter violation")
	}
	if !strings.Contains(err.Error(), "passes unsupported named parameters") || !strings.Contains(err.Error(), "recordRepository") {
		t.Fatalf("validateBuilderRuntimeTaskOutputs() error = %q, want invalid RecordFormPage named parameter detail", err)
	}
}

func TestValidateBuilderRuntimeTaskOutputsRejectsMissingRequiredViewNamedParametersInMainForCustomViewFileName(t *testing.T) {
	workspacePath := t.TempDir()
	writeBuilderRuntimeWorkspaceFiles(t, workspacePath, map[string]string{
		"lib/main.dart": strings.Join([]string{
			"import 'package:flutter/material.dart';",
			"",
			"import 'views/task_overview_page.dart';",
			"",
			"void main() {",
			"  runApp(const TodoApp());",
			"}",
			"",
			"class TodoApp extends StatelessWidget {",
			"  const TodoApp({super.key});",
			"",
			"  @override",
			"  Widget build(BuildContext context) {",
			"    return const MaterialApp(home: TaskOverviewPage());",
			"  }",
			"}",
		}, "\n") + "\n",
		"lib/views/task_overview_page.dart": strings.Join([]string{
			"import 'package:flutter/material.dart';",
			"",
			"class TaskOverviewPage extends StatelessWidget {",
			"  const TaskOverviewPage({",
			"    super.key,",
			"    required this.controller,",
			"    required this.onCreateTask,",
			"  });",
			"",
			"  final Object controller;",
			"  final Future<void> Function() onCreateTask;",
			"",
			"  @override",
			"  Widget build(BuildContext context) => const Placeholder();",
			"}",
		}, "\n") + "\n",
	})
	err := validateBuilderRuntimeTaskOutputs(runRecord{
		WorkspacePath: workspacePath,
		TaskBundle: []appruns.TaskBundleItem{{
			TaskID:      "task-bind-app-entry",
			TaskType:    appruns.BuilderRuntimeTaskTypeSingleFileEdit,
			TargetPaths: []string{"lib/main.dart", "lib/views/task_overview_page.dart"},
		}},
	}, "task-bind-app-entry")
	if err == nil {
		t.Fatal("validateBuilderRuntimeTaskOutputs() error = nil, want missing required named parameter violation for custom view file name")
	}
	if !strings.Contains(err.Error(), "omits required named parameters") || !strings.Contains(err.Error(), "TaskOverviewPage") || !strings.Contains(err.Error(), "lib/views/task_overview_page.dart") {
		t.Fatalf("validateBuilderRuntimeTaskOutputs() error = %q, want custom overview view named parameter detail", err)
	}
}

func TestIsBuilderRuntimeRepairableTaskOutputValidationFailureRecognizesMainViewConstructorDrift(t *testing.T) {
	for _, err := range []error{
		errors.New("task file lib/main.dart omits required named parameters controller, onCreateRecord when instantiating HomePage from lib/views/home_page.dart"),
		errors.New("task file lib/main.dart passes unsupported named parameters recordRepository to RecordFormPage; derive app-entry wiring from lib/views/record_form_page.dart"),
	} {
		if !isBuilderRuntimeRepairableTaskOutputValidationFailure(err) {
			t.Fatalf("isBuilderRuntimeRepairableTaskOutputValidationFailure(%q) = false, want true", err)
		}
	}
}

func TestShouldRetryBuilderRuntimeSemanticConflictAfterValidationRepairAllowsOneExtraSemanticRepair(t *testing.T) {
	semanticErr := errors.New("patch introduced package imports not declared in pubspec.yaml: lib/controllers/record_list_controller.dart -> uuid")
	if !shouldRetryBuilderRuntimeSemanticConflictAfterValidationRepair(&appruns.BuilderRuntimeExecutionStats{Attempts: 3}, "gemma4-26b-local", `{"patch_id":"repair-record-list-controller"}`, semanticErr) {
		t.Fatal("shouldRetryBuilderRuntimeSemanticConflictAfterValidationRepair() = false at attempts=3, want true for one extra semantic repair")
	}
	if shouldRetryBuilderRuntimeSemanticConflictAfterValidationRepair(&appruns.BuilderRuntimeExecutionStats{Attempts: 4}, "gemma4-26b-local", `{"patch_id":"repair-record-list-controller"}`, semanticErr) {
		t.Fatal("shouldRetryBuilderRuntimeSemanticConflictAfterValidationRepair() = true at attempts=4, want false")
	}
}

func TestValidateBuilderRuntimeTaskOutputsAcceptsResolvedLocalImport(t *testing.T) {
	workspacePath := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspacePath, "lib", "controllers"), 0o755); err != nil {
		t.Fatalf("MkdirAll(controllerDir) error = %v", err)
	}
	if err := os.MkdirAll(filepath.Join(workspacePath, "lib", "repositories"), 0o755); err != nil {
		t.Fatalf("MkdirAll(repositoryDir) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(workspacePath, "lib", "controllers", "home_controller.dart"), []byte("import '../repositories/record_repository.dart';\nclass HomeController {}\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(home_controller.dart) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(workspacePath, "lib", "repositories", "record_repository.dart"), []byte("class RecordRepository {}\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(record_repository.dart) error = %v", err)
	}
	if err := validateBuilderRuntimeTaskOutputs(runRecord{
		WorkspacePath: workspacePath,
		TaskBundle: []appruns.TaskBundleItem{{
			TaskID:      "task-create-home-controller",
			TaskType:    appruns.BuilderRuntimeTaskTypeSingleFileEdit,
			TargetPaths: []string{"lib/controllers/home_controller.dart"},
		}},
	}, "task-create-home-controller"); err != nil {
		t.Fatalf("validateBuilderRuntimeTaskOutputs() error = %v, want resolved import accepted", err)
	}
}

func TestValidateBuilderRuntimeTaskOutputsRejectsObviousDartSyntaxError(t *testing.T) {
	workspacePath := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspacePath, "lib", "views"), 0o755); err != nil {
		t.Fatalf("MkdirAll(viewDir) error = %v", err)
	}
	content := "import 'package:flutter/material.dart';\nclass HomePage extends StatelessWidget {\n  const HomePage({super.key});\n\n  @override\n  Widget build(BuildContext context) {\n    return const Text('broken);\n  }\n}\n"
	if err := os.WriteFile(filepath.Join(workspacePath, "lib", "views", "home_page.dart"), []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile(home_page.dart) error = %v", err)
	}
	err := validateBuilderRuntimeTaskOutputs(runRecord{
		WorkspacePath: workspacePath,
		TaskBundle: []appruns.TaskBundleItem{{
			TaskID:      "task-create-home-page",
			TaskType:    appruns.BuilderRuntimeTaskTypeSingleFileEdit,
			TargetPaths: []string{"lib/views/home_page.dart"},
		}},
	}, "task-create-home-page")
	if err == nil {
		t.Fatal("validateBuilderRuntimeTaskOutputs() error = nil, want syntax validation failure")
	}
	if !strings.Contains(err.Error(), "failed Dart syntax validation") {
		t.Fatalf("validateBuilderRuntimeTaskOutputs() error = %q, want syntax validation detail", err)
	}
	if !strings.Contains(err.Error(), "unterminated string literal") && !strings.Contains(err.Error(), "dart format rejected file") {
		t.Fatalf("validateBuilderRuntimeTaskOutputs() error = %q, want formatter or heuristic syntax hint", err)
	}
}

func TestValidateBuilderRuntimeTaskOutputsAcceptsBalancedDartSyntaxWithoutFormatter(t *testing.T) {
	workspacePath := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspacePath, "lib", "views"), 0o755); err != nil {
		t.Fatalf("MkdirAll(viewDir) error = %v", err)
	}
	content := "import 'package:flutter/material.dart';\nclass HomePage extends StatelessWidget {\n  const HomePage({super.key});\n\n  @override\n  Widget build(BuildContext context) {\n    return const Text('ok');\n  }\n}\n"
	if err := os.WriteFile(filepath.Join(workspacePath, "lib", "views", "home_page.dart"), []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile(home_page.dart) error = %v", err)
	}
	if err := validateBuilderRuntimeTaskOutputs(runRecord{
		WorkspacePath: workspacePath,
		TaskBundle: []appruns.TaskBundleItem{{
			TaskID:      "task-create-home-page",
			TaskType:    appruns.BuilderRuntimeTaskTypeSingleFileEdit,
			TargetPaths: []string{"lib/views/home_page.dart"},
		}},
	}, "task-create-home-page"); err != nil {
		t.Fatalf("validateBuilderRuntimeTaskOutputs() error = %v, want balanced syntax accepted", err)
	}
}

func TestValidateBuilderRuntimeTaskOutputsRejectsSeedPlaceholderMainWhenOverviewIsPruned(t *testing.T) {
	workspacePath := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspacePath, "lib"), 0o755); err != nil {
		t.Fatalf("MkdirAll(lib) error = %v", err)
	}
	content := "import 'package:flutter/material.dart';\n\nvoid main() {\n  runApp(const MaterialApp(home: AppFactorySeedHomePage()));\n}\n\nclass AppFactorySeedHomePage extends StatelessWidget {\n  const AppFactorySeedHomePage({super.key});\n\n  @override\n  Widget build(BuildContext context) {\n    return const Scaffold(body: Center(child: Text('Seed workspace ready')));\n  }\n}\n"
	if err := os.WriteFile(filepath.Join(workspacePath, "lib", "main.dart"), []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile(main.dart) error = %v", err)
	}
	err := validateBuilderRuntimeTaskOutputs(runRecord{
		WorkspacePath: workspacePath,
		TaskBundle: []appruns.TaskBundleItem{
			{TaskID: "task-create-repository", TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, TargetPaths: []string{"lib/repositories/record_repository.dart"}},
			{TaskID: "task-create-list-controller", TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, TargetPaths: []string{"lib/controllers/record_list_controller.dart"}, AllocationTransition: &appruns.TaskAllocationTransition{SemanticIntentRefs: []string{"ac-list"}}},
			{TaskID: "task-bind-collection-surface", TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, TargetPaths: []string{"lib/views/record_list_page.dart"}, AllocationTransition: &appruns.TaskAllocationTransition{SemanticIntentRefs: []string{"ac-list"}}},
			{TaskID: "task-bind-app-entry", TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, TargetPaths: []string{"lib/main.dart"}, Dependencies: []string{"task-create-repository", "task-create-list-controller", "task-bind-collection-surface"}, AllocationTransition: &appruns.TaskAllocationTransition{SemanticIntentRefs: []string{"ac-navigation"}}},
		},
	}, "task-bind-app-entry")
	if err == nil {
		t.Fatal("validateBuilderRuntimeTaskOutputs() error = nil, want seed placeholder app entry rejection")
	}
	if !strings.Contains(err.Error(), "seed placeholder app entry") {
		t.Fatalf("validateBuilderRuntimeTaskOutputs() error = %q, want seed placeholder detail", err)
	}
}

func TestValidateBuilderRuntimeTaskOutputsRejectsUnexpectedListFilterWhenTopologyPrunesFilter(t *testing.T) {
	workspacePath := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspacePath, "lib", "controllers"), 0o755); err != nil {
		t.Fatalf("MkdirAll(controllerDir) error = %v", err)
	}
	content := "class RecordListController {\n  Object? _selectedFilter;\n\n  void setFilter(Object? filter) {}\n}\n"
	if err := os.WriteFile(filepath.Join(workspacePath, "lib", "controllers", "record_list_controller.dart"), []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile(record_list_controller.dart) error = %v", err)
	}
	err := validateBuilderRuntimeTaskOutputs(runRecord{
		WorkspacePath: workspacePath,
		TaskBundle: []appruns.TaskBundleItem{{
			TaskID:      "task-create-list-controller",
			TaskType:    appruns.BuilderRuntimeTaskTypeDualFileWiring,
			TargetPaths: []string{"lib/controllers/record_list_controller.dart"},
			AllocationTransition: &appruns.TaskAllocationTransition{
				SemanticIntentRefs: []string{"ac-list"},
			},
		}},
	}, "task-create-list-controller")
	if err == nil {
		t.Fatal("validateBuilderRuntimeTaskOutputs() error = nil, want unexpected filter rejection")
	}
	if !strings.Contains(err.Error(), "does not include ac-filter") {
		t.Fatalf("validateBuilderRuntimeTaskOutputs() error = %q, want no-filter detail", err)
	}
}

func TestValidateBuilderRuntimeTaskOutputsRejectsDeleteBehaviorWhenTopologyPrunesDeleteFromListController(t *testing.T) {
	workspacePath := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspacePath, "lib", "controllers"), 0o755); err != nil {
		t.Fatalf("MkdirAll(controllerDir) error = %v", err)
	}
	content := "class RecordListController {\n  Future<void> deleteRecord(String taskId) async {}\n}\n"
	if err := os.WriteFile(filepath.Join(workspacePath, "lib", "controllers", "record_list_controller.dart"), []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile(record_list_controller.dart) error = %v", err)
	}
	err := validateBuilderRuntimeTaskOutputs(runRecord{
		WorkspacePath: workspacePath,
		TaskBundle: []appruns.TaskBundleItem{{
			TaskID:      "task-create-list-controller",
			TaskType:    appruns.BuilderRuntimeTaskTypeDualFileWiring,
			TargetPaths: []string{"lib/controllers/record_list_controller.dart"},
			AllocationTransition: &appruns.TaskAllocationTransition{
				SemanticIntentRefs: []string{"ac-list"},
			},
		}},
	}, "task-create-list-controller")
	if err == nil {
		t.Fatal("validateBuilderRuntimeTaskOutputs() error = nil, want unexpected delete rejection")
	}
	if !strings.Contains(err.Error(), "does not include delete behavior") {
		t.Fatalf("validateBuilderRuntimeTaskOutputs() error = %q, want no-delete detail", err)
	}
}

func TestValidateBuilderRuntimeTaskOutputsRejectsDeleteBehaviorWhenTopologyPrunesDeleteFromRepository(t *testing.T) {
	workspacePath := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspacePath, "lib", "repositories"), 0o755); err != nil {
		t.Fatalf("MkdirAll(repositoryDir) error = %v", err)
	}
	content := "abstract class RecordRepository {\n  Future<void> deleteRecord(String taskId);\n}\n"
	if err := os.WriteFile(filepath.Join(workspacePath, "lib", "repositories", "record_repository.dart"), []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile(record_repository.dart) error = %v", err)
	}
	err := validateBuilderRuntimeTaskOutputs(runRecord{
		WorkspacePath: workspacePath,
		TaskBundle: []appruns.TaskBundleItem{{
			TaskID:      "task-create-repository",
			TaskType:    appruns.BuilderRuntimeTaskTypeDualFileWiring,
			TargetPaths: []string{"lib/repositories/record_repository.dart"},
			AllocationTransition: &appruns.TaskAllocationTransition{
				SemanticIntentRefs: []string{"ac-persistence"},
			},
		}},
	}, "task-create-repository")
	if err == nil {
		t.Fatal("validateBuilderRuntimeTaskOutputs() error = nil, want repository delete rejection")
	}
	if !strings.Contains(err.Error(), "does not include delete behavior") {
		t.Fatalf("validateBuilderRuntimeTaskOutputs() error = %q, want no-delete detail", err)
	}
}

func TestValidateBuilderRuntimeTaskOutputsUsesDockerFormatterWhenExecutorImageProvided(t *testing.T) {
	workspacePath := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspacePath, "lib", "views"), 0o755); err != nil {
		t.Fatalf("MkdirAll(viewDir) error = %v", err)
	}
	content := "import 'package:flutter/material.dart';\nclass HomePage extends StatelessWidget {\n  const HomePage({super.key});\n\n  @override\n  Widget build(BuildContext context) {\n    return const Text('ok');\n  }\n}\n"
	if err := os.WriteFile(filepath.Join(workspacePath, "lib", "views", "home_page.dart"), []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile(home_page.dart) error = %v", err)
	}
	originalBuilder := builderRuntimeDartFormatterCommandBuilder
	t.Cleanup(func() {
		builderRuntimeDartFormatterCommandBuilder = originalBuilder
	})
	called := false
	builderRuntimeDartFormatterCommandBuilder = func(ctx context.Context, run runRecord, relPath, absPath string) (*exec.Cmd, error) {
		called = true
		if run.ExecutorImage != "picoclaw/appfactory-builder:local" {
			t.Fatalf("ExecutorImage = %q, want docker-backed formatter path", run.ExecutorImage)
		}
		if relPath != "lib/views/home_page.dart" {
			t.Fatalf("relPath = %q, want lib/views/home_page.dart", relPath)
		}
		return exec.CommandContext(ctx, "/bin/sh", "-lc", "true"), nil
	}
	if err := validateBuilderRuntimeTaskOutputs(runRecord{
		ExecutorImage: "picoclaw/appfactory-builder:local",
		WorkspacePath: workspacePath,
		TaskBundle: []appruns.TaskBundleItem{{
			TaskID:      "task-create-home-page",
			TaskType:    appruns.BuilderRuntimeTaskTypeSingleFileEdit,
			TargetPaths: []string{"lib/views/home_page.dart"},
		}},
	}, "task-create-home-page"); err != nil {
		t.Fatalf("validateBuilderRuntimeTaskOutputs() error = %v, want docker formatter path accepted", err)
	}
	if !called {
		t.Fatal("builderRuntimeDartFormatterCommandBuilder was not called")
	}
}

func TestBuildBuilderRuntimeDartFormatterCommandFallsBackToHostWithoutExecutorImage(t *testing.T) {
	command, err := buildBuilderRuntimeDartFormatterCommand(context.Background(), runRecord{}, "lib/views/home_page.dart", "/tmp/home_page.dart")
	if err != nil && !errors.Is(err, errBuilderRuntimeDartFormatterUnavailable) {
		t.Fatalf("buildBuilderRuntimeDartFormatterCommand() error = %v, want host formatter command or formatter unavailable", err)
	}
	if err == nil {
		if command == nil {
			t.Fatal("buildBuilderRuntimeDartFormatterCommand() command = nil, want host dart command")
		}
		if !strings.Contains(command.Path, "dart") {
			t.Fatalf("command.Path = %q, want host dart formatter", command.Path)
		}
	}
}

func TestBuildBuilderRuntimePromptUsesAllocationTransitionForCoupledRules(t *testing.T) {
	run := runRecord{
		GoalSummary:   "rewrite generic template wording into domain wording",
		WorkspacePath: t.TempDir(),
		TaskBundle: []appruns.TaskBundleItem{{
			TaskID:   "task-domain-copy",
			TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring,
			AllocationTransition: &appruns.TaskAllocationTransition{
				AllocationID:       "task-domain-copy",
				SurfaceRefs:        []string{"surface-overview", "surface-collection", "surface-inspection"},
				ScreenRefs:         []string{"screen-home", "screen-list", "screen-detail"},
				EntityRefs:         []string{"entity-record", "entity-dashboard-summary"},
				SemanticIntentRefs: []string{"mrp-domain-wording", "check-profile-open-lite-domain-branding", "check-profile-open-lite-domain-language"},
				OwnedPaths:         []string{"lib/template/open_lite_copy.dart", "android/app/src/main/res/values/strings.xml", "test/widget_test.dart"},
				SuccessEvidence:    []string{"copy 与 branding 同步切换到领域文案", "widget_test 断言与当前领域文案保持同步"},
			},
			TargetPaths: []string{"lib/template/open_lite_copy.dart", "android/app/src/main/res/values/strings.xml", "test/widget_test.dart"},
		}},
	}
	roundInput := appruns.RoundInput{
		TaskBundle: run.TaskBundle,
		AllowedPaths: []string{
			"lib/**",
			"android/app/src/main/res/values/strings.xml",
			"test/**",
		},
	}
	route := appruns.BuilderRuntimeTaskRoute{
		TaskID:      "task-domain-copy",
		TaskType:    appruns.BuilderRuntimeTaskTypeDualFileWiring,
		RouteSource: "default_model",
	}

	prompt, err := buildBuilderRuntimePrompt(BuilderRuntimePatchRequest{Run: run, RoundInput: roundInput, Route: route})
	if err != nil {
		t.Fatalf("buildBuilderRuntimePrompt() error = %v", err)
	}
	if strings.Contains(prompt, "Treat allocation_transition.owned_paths as a coordinated change-set") {
		t.Fatalf("prompt should no longer contain owned_paths coordination guidance (migrated to task-allocation blocked_by): %q", prompt)
	}
	if !strings.Contains(prompt, "Treat allocation_transition.surface_refs as the primary interaction surfaces for this patch") {
		t.Fatalf("prompt missing surface refs guidance: %q", prompt)
	}
	if !strings.Contains(prompt, "Current allocation surface_refs JSON: [\"surface-overview\",\"surface-collection\",\"surface-inspection\"]") {
		t.Fatalf("prompt missing surface refs payload: %q", prompt)
	}
	if strings.Contains(prompt, "Current allocation screen_refs JSON:") {
		t.Fatalf("prompt should prefer surface refs over screen refs: %q", prompt)
	}
	if !strings.Contains(prompt, "Treat allocation_transition.entity_refs as the primary domain entities for this patch") {
		t.Fatalf("prompt missing entity refs guidance: %q", prompt)
	}
	if !strings.Contains(prompt, "Current allocation entity_refs JSON: [\"entity-record\",\"entity-dashboard-summary\"]") {
		t.Fatalf("prompt missing entity refs payload: %q", prompt)
	}
	if !strings.Contains(prompt, "Current allocation semantic_intent_refs JSON: [\"mrp-domain-wording\"") {
		t.Fatalf("prompt missing semantic intent refs payload: %q", prompt)
	}
	if !strings.Contains(prompt, "Current allocation success_evidence JSON: [\"copy 与 branding 同步切换到领域文案\"") {
		t.Fatalf("prompt missing success evidence payload: %q", prompt)
	}
	if !strings.Contains(prompt, "Current allocation owned_paths JSON: [\"lib/template/open_lite_copy.dart\"") {
		t.Fatalf("prompt missing owned paths payload: %q", prompt)
	}
	if !strings.Contains(prompt, "Current allocation task JSON: {\"task_id\":\"task-domain-copy\"") {
		t.Fatalf("prompt missing current allocation task payload: %q", prompt)
	}
	if strings.Contains(prompt, "Task bundle JSON:") {
		t.Fatalf("prompt should not dump full task bundle anymore: %q", prompt)
	}
	if strings.Contains(prompt, "If lib/template/open_lite_copy.dart is in task target_paths") {
		t.Fatalf("prompt still contains file-specific open_lite_copy patch rule: %q", prompt)
	}
	if !strings.Contains(prompt, "wrap testWidgets calls inside void main() { ... }") {
		t.Fatalf("prompt missing valid Dart test structure guidance: %q", prompt)
	}
	if !strings.Contains(prompt, "Do not invent Key(...) values") {
		t.Fatalf("prompt missing widget test finder realism guidance: %q", prompt)
	}
	if !strings.Contains(prompt, "Do not call enterText on DropdownButtonFormField") {
		t.Fatalf("prompt missing widget test input interaction guidance: %q", prompt)
	}
	if !strings.Contains(prompt, "keep imports and helper declarations minimal") {
		t.Fatalf("prompt missing widget test import/helper cleanup guidance: %q", prompt)
	}
	if !strings.Contains(prompt, "Call await tester.pumpAndSettle() directly") {
		t.Fatalf("prompt missing direct pumpAndSettle guidance: %q", prompt)
	}
	if !strings.Contains(prompt, "Bad state: No element") {
		t.Fatalf("prompt missing widget test no-element repair guidance: %q", prompt)
	}
}

func TestBuildBuilderRuntimePromptConsumesSimpleDomainCompilerRefs(t *testing.T) {
	testCases := []struct {
		name                string
		requirementText     string
		requirementSource   string
		taskID              string
		expectedSurfaceRefs string
		expectedEntityRefs  string
	}{
		{
			name:                "generic todo list controller",
			requirementText:     "做一个待办事项 app，需要首页摘要、新建待办、任务列表和详情页，优先保证本地可用。",
			requirementSource:   "inline:builder-runtime-generic-todo",
			taskID:              "task-create-list-controller",
			expectedSurfaceRefs: `["surface-collection","surface-inspection"]`,
			expectedEntityRefs:  `["entity-todo-item"]`,
		},
		{
			name:                "bookkeeping flow wiring",
			requirementText:     "做一个简单记账 app，需要首页概览、记一笔和账单列表。",
			requirementSource:   "inline:builder-runtime-bookkeeping",
			taskID:              "task-flow-wiring",
			expectedSurfaceRefs: `["surface-overview","surface-collection","surface-mutation"]`,
			expectedEntityRefs:  `["entity-entry","entity-summary"]`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			run, roundInput := buildCompiledPromptTestRun(t, tc.requirementText, tc.requirementSource)
			routeTask := mustFindPromptTaskBundleItem(t, run.TaskBundle, tc.taskID)
			route := appruns.BuilderRuntimeTaskRoute{
				TaskID:      tc.taskID,
				TaskType:    routeTask.EffectiveTaskType(),
				RouteSource: "task_route",
			}

			prompt, err := buildBuilderRuntimePrompt(BuilderRuntimePatchRequest{Run: run, RoundInput: roundInput, Route: route})
			if err != nil {
				t.Fatalf("buildBuilderRuntimePrompt() error = %v", err)
			}
			if tc.expectedSurfaceRefs != "" {
				if !strings.Contains(prompt, "Treat allocation_transition.surface_refs as the primary interaction surfaces for this patch") {
					t.Fatalf("prompt missing surface refs guidance: %q", prompt)
				}
				if !strings.Contains(prompt, "Current allocation surface_refs JSON: "+tc.expectedSurfaceRefs) {
					t.Fatalf("prompt missing expected surface refs payload %s: %q", tc.expectedSurfaceRefs, prompt)
				}
				if strings.Contains(prompt, "Current allocation screen_refs JSON:") {
					t.Fatalf("prompt should prefer surface refs over screen refs: %q", prompt)
				}
			} else if strings.Contains(prompt, "Current allocation surface_refs JSON:") {
				t.Fatalf("prompt unexpectedly included surface refs payload: %q", prompt)
			}
			if strings.Contains(prompt, "Current allocation legacy screen_refs JSON:") {
				t.Fatalf("prompt unexpectedly included screen refs payload: %q", prompt)
			}
			if !strings.Contains(prompt, "Treat allocation_transition.entity_refs as the primary domain entities for this patch") {
				t.Fatalf("prompt missing entity refs guidance: %q", prompt)
			}
			if !strings.Contains(prompt, "Current allocation entity_refs JSON: "+tc.expectedEntityRefs) {
				t.Fatalf("prompt missing expected entity refs payload %s: %q", tc.expectedEntityRefs, prompt)
			}
			if !strings.Contains(prompt, fmt.Sprintf(`Current allocation task JSON: {"task_id":"%s"`, tc.taskID)) {
				t.Fatalf("prompt missing current allocation task payload for %s: %q", tc.taskID, prompt)
			}
		})
	}
}

func TestBuildBuilderRuntimePromptIgnoresLegacyScreenRefs(t *testing.T) {
	run := runRecord{
		WorkspacePath: t.TempDir(),
		TaskBundle: []appruns.TaskBundleItem{{
			TaskID:      "task-legacy-screen-flow",
			Title:       "legacy screen flow",
			Category:    appruns.TaskCategoryFlow,
			TaskType:    appruns.BuilderRuntimeTaskTypeDualFileWiring,
			TargetPaths: []string{"lib/main.dart"},
			AllocationTransition: &appruns.TaskAllocationTransition{
				AllocationID:       "task-legacy-screen-flow",
				ScreenRefs:         []string{"screen-home", "screen-list"},
				EntityRefs:         []string{"entity-record"},
				SemanticIntentRefs: []string{"ac-navigation"},
			},
		}},
	}
	roundInput := BuildRoundInputForTest(run)
	route := appruns.BuilderRuntimeTaskRoute{
		TaskID:      "task-legacy-screen-flow",
		TaskType:    appruns.BuilderRuntimeTaskTypeDualFileWiring,
		RouteSource: "task_route",
	}

	prompt, err := buildBuilderRuntimePrompt(BuilderRuntimePatchRequest{Run: run, RoundInput: roundInput, Route: route})
	if err != nil {
		t.Fatalf("buildBuilderRuntimePrompt() error = %v", err)
	}
	if strings.Contains(prompt, "allocation_transition.screen_refs") {
		t.Fatalf("prompt should not surface legacy screen refs guidance or payload anymore: %q", prompt)
	}
	if !strings.Contains(prompt, "Treat allocation_transition.entity_refs as the primary domain entities for this patch") {
		t.Fatalf("prompt missing entity refs guidance: %q", prompt)
	}
}

func TestBuildBuilderRuntimePromptConstrainsOpenLiteCopyHelpersToCurrentModels(t *testing.T) {
	run, roundInput := buildCompiledPromptTestRun(t, "做一个待办事项 app，需要首页摘要、新建待办、任务列表和详情页，优先保证本地可用。", "inline:builder-runtime-open-lite-copy")
	for index := range run.TaskBundle {
		for pathIndex, path := range run.TaskBundle[index].TargetPaths {
			if path == "lib/template/open_lite_copy.dart" {
				run.TaskBundle[index].TargetPaths[pathIndex] = "lib/template/domain_copy.dart"
			}
		}
		if run.TaskBundle[index].AllocationTransition != nil {
			for pathIndex, path := range run.TaskBundle[index].AllocationTransition.OwnedPaths {
				if path == "lib/template/open_lite_copy.dart" {
					run.TaskBundle[index].AllocationTransition.OwnedPaths[pathIndex] = "lib/template/domain_copy.dart"
				}
			}
		}
	}
	roundInput.TaskBundle = run.TaskBundle
	recordPath := filepath.Join(run.WorkspacePath, "lib", "models", "record.dart")
	if err := os.MkdirAll(filepath.Dir(recordPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(recordPath) error = %v", err)
	}
	if err := os.WriteFile(recordPath, []byte(strings.Join([]string{
		"class AppRecord {",
		"  const AppRecord({required this.taskId, required this.title, required this.category, required this.status, this.note});",
		"  final String taskId;",
		"  final String title;",
		"  final String category;",
		"  final String status;",
		"  final String? note;",
		"}",
	}, "\n")), 0o600); err != nil {
		t.Fatalf("WriteFile(record.dart) error = %v", err)
	}
	summaryPath := filepath.Join(run.WorkspacePath, "lib", "models", "dashboard_summary.dart")
	if err := os.WriteFile(summaryPath, []byte(strings.Join([]string{
		"class DashboardSummary {",
		"  const DashboardSummary({required this.inboxCount, required this.inProgressCount, required this.doneCount});",
		"  final int inboxCount;",
		"  final int inProgressCount;",
		"  final int doneCount;",
		"}",
	}, "\n")), 0o600); err != nil {
		t.Fatalf("WriteFile(dashboard_summary.dart) error = %v", err)
	}
	routeTask := mustFindPromptTaskBundleItem(t, run.TaskBundle, "task-create-copy")
	route := appruns.BuilderRuntimeTaskRoute{
		TaskID:      "task-create-copy",
		TaskType:    routeTask.EffectiveTaskType(),
		RouteSource: "task_route",
	}

	prompt, err := buildBuilderRuntimePrompt(BuilderRuntimePatchRequest{Run: run, RoundInput: roundInput, Route: route})
	if err != nil {
		t.Fatalf("buildBuilderRuntimePrompt() error = %v", err)
	}
	if !strings.Contains(prompt, "EXISTING FILE: lib/models/dashboard_summary.dart") {
		t.Fatalf("prompt missing summary model file context for open_lite_copy: %q", prompt)
	}
	if !strings.Contains(prompt, "For current template copy files under lib/template/*.dart, expose only copy helpers justified by the current record.dart, dashboard_summary.dart") {
		t.Fatalf("prompt missing open_lite_copy model-bound guidance: %q", prompt)
	}
	if !strings.Contains(prompt, "Current summary model does not define totalCount") {
		t.Fatalf("prompt missing totalCount removal guidance for open_lite_copy: %q", prompt)
	}
	if !strings.Contains(prompt, "Do not emit summaryCountLabel, recentRecordsCountLabel, listCountLabel") {
		t.Fatalf("prompt missing count-helper removal guidance for open_lite_copy: %q", prompt)
	}
	if !strings.Contains(prompt, "Never emit the identifier totalCount anywhere in the current template copy file") {
		t.Fatalf("prompt missing totalCount identifier prohibition for open_lite_copy: %q", prompt)
	}
}

func TestBuildBuilderRuntimePromptConstrainsRepositoryToCurrentRecordFields(t *testing.T) {
	run, roundInput := buildCompiledPromptTestRun(t, "做一个待办事项 app，需要首页摘要、新建待办、任务列表和详情页，优先保证本地可用。", "inline:builder-runtime-repository-fields")
	recordPath := filepath.Join(run.WorkspacePath, "lib", "models", "record.dart")
	if err := os.MkdirAll(filepath.Dir(recordPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(recordPath) error = %v", err)
	}
	if err := os.WriteFile(recordPath, []byte(strings.Join([]string{
		"class AppRecord {",
		"  const AppRecord({required this.taskId, required this.title, required this.category, required this.status, this.note});",
		"  final String taskId;",
		"  final String title;",
		"  final String category;",
		"  final String status;",
		"  final String? note;",
		"}",
	}, "\n")), 0o600); err != nil {
		t.Fatalf("WriteFile(record.dart) error = %v", err)
	}
	routeTask := mustFindPromptTaskBundleItem(t, run.TaskBundle, "task-create-repository")
	route := appruns.BuilderRuntimeTaskRoute{
		TaskID:      "task-create-repository",
		TaskType:    routeTask.EffectiveTaskType(),
		RouteSource: "task_route",
	}

	prompt, err := buildBuilderRuntimePrompt(BuilderRuntimePatchRequest{Run: run, RoundInput: roundInput, Route: route})
	if err != nil {
		t.Fatalf("buildBuilderRuntimePrompt() error = %v", err)
	}
	if !strings.Contains(prompt, "For current repository-scope target files under lib/repositories/*.dart, keep persistence, serialization, sorting, and query helpers aligned to the exact field names exported by the current primary record model in context") {
		t.Fatalf("prompt missing repository field-alignment guidance: %q", prompt)
	}
	if !strings.Contains(prompt, "keep the current repository file on the template-style Hive map persistence path") {
		t.Fatalf("prompt missing map-backed Hive guidance for repository task: %q", prompt)
	}
	if !strings.Contains(prompt, "Do not introduce Hive.registerAdapter(...) or Box<RecordType>") {
		t.Fatalf("prompt missing adapter prohibition for repository task: %q", prompt)
	}
	if !strings.Contains(prompt, "Current primary record model does not define updatedAt") {
		t.Fatalf("prompt missing updatedAt removal guidance for repository task: %q", prompt)
	}
	if !strings.Contains(prompt, "Do not read, write, sort, serialize, or map by updatedAt inside the current repository file") {
		t.Fatalf("prompt missing repository updatedAt prohibition: %q", prompt)
	}
}

func TestBuildBuilderRuntimePromptConstrainsRepositoryToCustomPrimaryModelFields(t *testing.T) {
	workspacePath := t.TempDir()
	writeBuilderRuntimeWorkspaceFiles(t, workspacePath, map[string]string{
		"lib/models/task.dart": strings.Join([]string{
			"class Task {",
			"  const Task({required this.taskId, required this.title, this.note});",
			"  final String taskId;",
			"  final String title;",
			"  final String? note;",
			"}",
		}, "\n") + "\n",
		"lib/controllers/task_collection_controller.dart": strings.Join([]string{
			"class TaskCollectionController {",
			"  const TaskCollectionController();",
			"}",
		}, "\n") + "\n",
		"lib/views/task_collection_page.dart": strings.Join([]string{
			"class TaskCollectionPage {",
			"  const TaskCollectionPage({required this.controller, required this.onOpenTaskDetail});",
			"  final TaskCollectionController controller;",
			"  final Future<void> Function(Task task) onOpenTaskDetail;",
			"}",
		}, "\n") + "\n",
	})
	run := runRecord{
		GoalSummary:   "repair repository analyze failures",
		WorkspacePath: workspacePath,
		TaskBundle: []appruns.TaskBundleItem{{
			TaskID:      "task-create-repository",
			TaskType:    appruns.BuilderRuntimeTaskTypeDualFileWiring,
			TargetPaths: []string{"lib/repositories/task_repository.dart"},
		}},
	}
	roundInput := appruns.RoundInput{TaskBundle: run.TaskBundle, AllowedPaths: []string{"lib/**"}}
	route := appruns.BuilderRuntimeTaskRoute{TaskID: "task-create-repository", TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, RouteSource: "task_route"}
	prompt, err := buildBuilderRuntimePrompt(BuilderRuntimePatchRequest{
		Run:            run,
		RoundInput:     roundInput,
		Route:          route,
		FailureContext: "patch content reintroduced symbols absent from current models: lib/repositories/task_repository.dart -> id, updatedAt",
	})
	if err != nil {
		t.Fatalf("buildBuilderRuntimePrompt() error = %v", err)
	}
	if !strings.Contains(prompt, "For current repository-scope target files under lib/repositories/*.dart, keep persistence, serialization, sorting, and query helpers aligned to the exact field names exported by the current primary record model in context") {
		t.Fatalf("prompt missing repository field-alignment guidance for custom primary model: %q", prompt)
	}
	if !strings.Contains(prompt, "Current primary record model does not define updatedAt") {
		t.Fatalf("prompt missing updatedAt guidance for custom primary model: %q", prompt)
	}
	if !strings.Contains(prompt, "Do not read, write, sort, serialize, or map by updatedAt inside the current repository file") {
		t.Fatalf("prompt missing repository updatedAt prohibition for custom primary model: %q", prompt)
	}
}

func TestBuildBuilderRuntimePromptConstrainsRepositoryWhenTopologyPrunesDelete(t *testing.T) {
	run := runRecord{
		GoalSummary:   "build a todo app with list and form flows but without delete",
		WorkspacePath: t.TempDir(),
		TaskBundle: []appruns.TaskBundleItem{
			{TaskID: "task-create-repository", TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, TargetPaths: []string{"lib/repositories/record_repository.dart"}},
			{TaskID: "task-create-form-controller", TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, TargetPaths: []string{"lib/controllers/record_form_controller.dart"}, AllocationTransition: &appruns.TaskAllocationTransition{SemanticIntentRefs: []string{"ac-form"}}},
			{TaskID: "task-create-list-controller", TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, TargetPaths: []string{"lib/controllers/record_list_controller.dart"}, AllocationTransition: &appruns.TaskAllocationTransition{SemanticIntentRefs: []string{"ac-list"}}},
			{TaskID: "task-bind-collection-surface", TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, TargetPaths: []string{"lib/views/record_list_page.dart"}, AllocationTransition: &appruns.TaskAllocationTransition{SemanticIntentRefs: []string{"ac-list"}, SurfaceRefs: []string{"surface-collection"}}},
			{TaskID: "task-bind-mutation-surface", TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, TargetPaths: []string{"lib/views/record_form_page.dart"}, AllocationTransition: &appruns.TaskAllocationTransition{SemanticIntentRefs: []string{"ac-form"}, SurfaceRefs: []string{"surface-mutation"}}},
		},
	}
	roundInput := appruns.RoundInput{TaskBundle: run.TaskBundle, AllowedPaths: []string{"lib/**"}}
	route := appruns.BuilderRuntimeTaskRoute{TaskID: "task-create-repository", TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, RouteSource: "default_model"}

	prompt, err := buildBuilderRuntimePrompt(BuilderRuntimePatchRequest{Run: run, RoundInput: roundInput, Route: route})
	if err != nil {
		t.Fatalf("buildBuilderRuntimePrompt() error = %v", err)
	}
	if !strings.Contains(prompt, "Current topology does not include delete behavior.") {
		t.Fatalf("prompt missing no-delete topology guidance for repository task: %q", prompt)
	}
	if !strings.Contains(prompt, "repository delete helpers") {
		t.Fatalf("prompt missing repository delete prohibition detail: %q", prompt)
	}
}

func TestBuildBuilderRuntimePromptConstrainsHomePageToCurrentRecordAndSummaryFields(t *testing.T) {
	run, roundInput := buildCompiledPromptTestRun(t, "做一个待办事项 app，需要首页摘要、新建待办、任务列表和详情页，优先保证本地可用。", "inline:builder-runtime-home-page-fields")
	recordPath := filepath.Join(run.WorkspacePath, "lib", "models", "record.dart")
	if err := os.MkdirAll(filepath.Dir(recordPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(recordPath) error = %v", err)
	}
	if err := os.WriteFile(recordPath, []byte(strings.Join([]string{
		"class AppRecord {",
		"  const AppRecord({required this.taskId, required this.title, required this.category, required this.status, this.note});",
		"  final String taskId;",
		"  final String title;",
		"  final String category;",
		"  final String status;",
		"  final String? note;",
		"}",
	}, "\n")), 0o600); err != nil {
		t.Fatalf("WriteFile(record.dart) error = %v", err)
	}
	summaryPath := filepath.Join(run.WorkspacePath, "lib", "models", "dashboard_summary.dart")
	if err := os.MkdirAll(filepath.Dir(summaryPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(summaryPath) error = %v", err)
	}
	if err := os.WriteFile(summaryPath, []byte(strings.Join([]string{
		"class DashboardSummary {",
		"  const DashboardSummary({required this.inboxCount, required this.inProgressCount, required this.doneCount});",
		"  final int inboxCount;",
		"  final int inProgressCount;",
		"  final int doneCount;",
		"}",
	}, "\n")), 0o600); err != nil {
		t.Fatalf("WriteFile(dashboard_summary.dart) error = %v", err)
	}
	routeTask := mustFindPromptTaskBundleItem(t, run.TaskBundle, "task-bind-overview-surface")
	route := appruns.BuilderRuntimeTaskRoute{
		TaskID:      "task-bind-overview-surface",
		TaskType:    routeTask.EffectiveTaskType(),
		RouteSource: "task_route",
	}

	prompt, err := buildBuilderRuntimePrompt(BuilderRuntimePatchRequest{Run: run, RoundInput: roundInput, Route: route})
	if err != nil {
		t.Fatalf("buildBuilderRuntimePrompt() error = %v", err)
	}
	if !strings.Contains(prompt, "For current overview-surface target files, keep summary cards, total-count copy, recent-record metadata") {
		t.Fatalf("prompt missing home_page model-alignment guidance: %q", prompt)
	}
	if !strings.Contains(prompt, "Current summary model does not define totalCount") {
		t.Fatalf("prompt missing totalCount prohibition for home_page: %q", prompt)
	}
	if !strings.Contains(prompt, "In current overview-surface target files, derive any overall record count from controller.records.length") {
		t.Fatalf("prompt missing home_page count derivation guidance: %q", prompt)
	}
	if !strings.Contains(prompt, "Current primary record model does not define updatedAt") {
		t.Fatalf("prompt missing updatedAt prohibition for home_page: %q", prompt)
	}
	if !strings.Contains(prompt, "In current overview-surface target files, use the actual time field") {
		t.Fatalf("prompt missing home_page date fallback guidance: %q", prompt)
	}
}

func TestBuildBuilderRuntimePromptConstrainsListAndDetailPagesToCurrentTimeField(t *testing.T) {
	run, roundInput := buildCompiledPromptTestRun(t, "做一个待办事项 app，需要首页摘要、新建待办、任务列表和详情页，优先保证本地可用。", "inline:builder-runtime-list-detail-fields")
	recordPath := filepath.Join(run.WorkspacePath, "lib", "models", "record.dart")
	if err := os.MkdirAll(filepath.Dir(recordPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(recordPath) error = %v", err)
	}
	if err := os.WriteFile(recordPath, []byte(strings.Join([]string{
		"class AppRecord {",
		"  const AppRecord({required this.taskId, required this.title, required this.category, required this.status, this.note});",
		"  final String taskId;",
		"  final String title;",
		"  final String category;",
		"  final String status;",
		"  final String? note;",
		"}",
	}, "\n")), 0o600); err != nil {
		t.Fatalf("WriteFile(record.dart) error = %v", err)
	}
	routeTask := mustFindPromptTaskBundleItem(t, run.TaskBundle, "task-bind-collection-surface")
	route := appruns.BuilderRuntimeTaskRoute{
		TaskID:      "task-bind-collection-surface",
		TaskType:    routeTask.EffectiveTaskType(),
		RouteSource: "task_route",
	}

	prompt, err := buildBuilderRuntimePrompt(BuilderRuntimePatchRequest{Run: run, RoundInput: roundInput, Route: route})
	if err != nil {
		t.Fatalf("buildBuilderRuntimePrompt() error = %v", err)
	}
	if !strings.Contains(prompt, "For current collection/inspection-surface target files") {
		t.Fatalf("prompt missing list/detail time-field guidance: %q", prompt)
	}
	if !strings.Contains(prompt, "Current primary record model does not define updatedAt") {
		t.Fatalf("prompt missing updatedAt prohibition for list/detail pages: %q", prompt)
	}
	if !strings.Contains(prompt, "omit updatedAt-based date text and date tiles") {
		t.Fatalf("prompt missing list/detail date fallback guidance: %q", prompt)
	}
}

func TestBuildBuilderRuntimePromptDerivesSurfaceGuidanceFromOverlappingContextTaskDuringAnalyzeRepair(t *testing.T) {
	testCases := []struct {
		name     string
		paths    []string
		wantText string
	}{
		{
			name:     "overview repair slice",
			paths:    []string{"lib/views/home_page.dart"},
			wantText: "For current overview-surface target files, keep summary cards, total-count copy, recent-record metadata",
		},
		{
			name:     "collection repair slice",
			paths:    []string{"lib/views/record_list_page.dart"},
			wantText: "For current collection/inspection-surface target files",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			run, roundInput := buildCompiledPromptTestRun(t, "做一个待办事项 app，需要首页摘要、新建待办、任务列表和详情页，优先保证本地可用。", "inline:builder-runtime-repair-surface-overlap")
			repairTask := appruns.TaskBundleItem{
				TaskID:      "repair-check-flutter-analyze",
				Category:    appruns.TaskCategoryValidation,
				TaskType:    appruns.BuilderRuntimeTaskTypeAnalyzeRepair,
				TargetPaths: tc.paths,
			}
			run.TaskBundle = buildValidationRepairTaskBundle(run.TaskBundle, repairTask)
			roundInput.TaskBundle = run.TaskBundle
			route := appruns.BuilderRuntimeTaskRoute{
				TaskID:      repairTask.TaskID,
				TaskType:    appruns.BuilderRuntimeTaskTypeAnalyzeRepair,
				RouteSource: "task_route",
			}

			prompt, err := buildBuilderRuntimePrompt(BuilderRuntimePatchRequest{Run: run, RoundInput: roundInput, Route: route})
			if err != nil {
				t.Fatalf("buildBuilderRuntimePrompt() error = %v", err)
			}
			if !strings.Contains(prompt, tc.wantText) {
				t.Fatalf("prompt missing surface guidance %q: %q", tc.wantText, prompt)
			}
		})
	}
}

func TestBuildBuilderRuntimePromptDoesNotInferSurfaceGuidanceFromFixedPathsDuringAnalyzeRepair(t *testing.T) {
	testCases := []struct {
		name       string
		paths      []string
		unwantText string
	}{
		{
			name:       "home page path alone",
			paths:      []string{"lib/views/home_page.dart"},
			unwantText: "For current overview-surface target files, keep summary cards, total-count copy, recent-record metadata",
		},
		{
			name:       "record list path alone",
			paths:      []string{"lib/views/record_list_page.dart"},
			unwantText: "For current collection/inspection-surface target files",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			run, _ := buildCompiledPromptTestRun(t, "做一个待办事项 app，需要首页摘要、新建待办、任务列表和详情页，优先保证本地可用。", "inline:builder-runtime-no-fixed-path-fallback")
			repairTask := appruns.TaskBundleItem{
				TaskID:      "repair-check-flutter-analyze",
				Category:    appruns.TaskCategoryValidation,
				TaskType:    appruns.BuilderRuntimeTaskTypeAnalyzeRepair,
				TargetPaths: tc.paths,
			}
			run.TaskBundle = []appruns.TaskBundleItem{
				repairTask,
				{
					TaskID:      "task-generic-domain-models",
					TaskType:    appruns.BuilderRuntimeTaskTypeDualFileWiring,
					TargetPaths: []string{"lib/models/record.dart", "lib/models/dashboard_summary.dart"},
				},
				{
					TaskID:      "task-fixed-view-without-surface",
					TaskType:    appruns.BuilderRuntimeTaskTypeDualFileWiring,
					TargetPaths: tc.paths,
				},
			}
			roundInput := BuildRoundInputForTest(run)
			route := appruns.BuilderRuntimeTaskRoute{
				TaskID:      repairTask.TaskID,
				TaskType:    appruns.BuilderRuntimeTaskTypeAnalyzeRepair,
				RouteSource: "task_route",
			}

			prompt, err := buildBuilderRuntimePrompt(BuilderRuntimePatchRequest{Run: run, RoundInput: roundInput, Route: route})
			if err != nil {
				t.Fatalf("buildBuilderRuntimePrompt() error = %v", err)
			}
			if strings.Contains(prompt, tc.unwantText) {
				t.Fatalf("prompt should not infer surface guidance from fixed paths alone: %q", prompt)
			}
		})
	}
}

func TestBuildBuilderRuntimePromptAvoidsGenericRecordSurfaceHeuristicsForRelationRichOverview(t *testing.T) {
	run, roundInput := buildCompiledPromptTestRun(t, "做一个项目任务协同 app，需要项目看板、任务列表、任务编辑和标签绑定，支持按项目和标签筛选任务。", "inline:builder-runtime-relation-rich-overview")
	routeTask := mustFindPromptTaskBundleItem(t, run.TaskBundle, "task-bind-overview-surface")
	route := appruns.BuilderRuntimeTaskRoute{
		TaskID:      "task-bind-overview-surface",
		TaskType:    routeTask.EffectiveTaskType(),
		RouteSource: "task_route",
	}

	prompt, err := buildBuilderRuntimePrompt(BuilderRuntimePatchRequest{Run: run, RoundInput: roundInput, Route: route})
	if err != nil {
		t.Fatalf("buildBuilderRuntimePrompt() error = %v", err)
	}
	if strings.Contains(prompt, "For current overview-surface target files, keep summary cards, total-count copy, recent-record metadata") {
		t.Fatalf("prompt should not inject generic record overview heuristics into relation-rich overview surface: %q", prompt)
	}
	if strings.Contains(prompt, "dashboard_summary.dart and record.dart models in context") {
		t.Fatalf("prompt should not assume generic record/dashboard summary pair for relation-rich overview surface: %q", prompt)
	}
}

func TestBuildBuilderRuntimePromptConstrainsAndroidBuildConfigFlutterSource(t *testing.T) {
	run, roundInput := buildCompiledPromptTestRun(t, "做一个待办事项 app，需要首页摘要、新建待办、任务列表和详情页，优先保证本地可用。", "inline:builder-runtime-android-build-config")
	routeTask := mustFindPromptTaskBundleItem(t, run.TaskBundle, "task-create-android-build-config")
	route := appruns.BuilderRuntimeTaskRoute{
		TaskID:      "task-create-android-build-config",
		TaskType:    routeTask.EffectiveTaskType(),
		RouteSource: "task_route",
	}

	prompt, err := buildBuilderRuntimePrompt(BuilderRuntimePatchRequest{Run: run, RoundInput: roundInput, Route: route})
	if err != nil {
		t.Fatalf("buildBuilderRuntimePrompt() error = %v", err)
	}
	if !strings.Contains(prompt, "preserve flutter { source = \"../..\" } exactly") {
		t.Fatalf("prompt missing android build config flutter source guidance: %q", prompt)
	}
	if !strings.Contains(prompt, "Do not emit malformed Flutter source path literals") {
		t.Fatalf("prompt missing malformed flutter source prohibition: %q", prompt)
	}
	if !strings.Contains(prompt, "defaultOpenLiteApplicationId") {
		t.Fatalf("prompt missing android build config namespace/applicationId guidance: %q", prompt)
	}
}

func TestNormalizeBuilderRuntimePatchForWorkspaceRemovesUnsupportedUpdatedAtSorts(t *testing.T) {
	workspacePath := t.TempDir()
	recordPath := filepath.Join(workspacePath, "lib", "models", "record.dart")
	if err := os.MkdirAll(filepath.Dir(recordPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(recordPath) error = %v", err)
	}
	if err := os.WriteFile(recordPath, []byte(strings.Join([]string{
		"class AppRecord {",
		"  const AppRecord({required this.taskId, required this.title});",
		"  final String taskId;",
		"  final String title;",
		"}",
	}, "\n")), 0o600); err != nil {
		t.Fatalf("WriteFile(record.dart) error = %v", err)
	}
	patch := appruns.WorkspacePatch{Operations: []appruns.WorkspacePatchOperation{{
		Type: "write_file",
		Path: "lib/repositories/local_store.dart",
		Content: strings.Join([]string{
			"final records = rows",
			"    .map(AppRecord.fromJson)",
			"    .toList()",
			"  ..sort((left, right) => right.updatedAt.compareTo(left.updatedAt));",
			"records.sort((left, right) => right.updatedAt.compareTo(left.updatedAt));",
		}, "\n"),
	}}}

	normalizeBuilderRuntimePatchForWorkspace(workspacePath, &patch)

	content := patch.Operations[0].Content
	if strings.Contains(content, ".updatedAt") {
		t.Fatalf("normalizeBuilderRuntimePatchForWorkspace() content still references updatedAt: %q", content)
	}
	if strings.Contains(content, "records.sort(") {
		t.Fatalf("normalizeBuilderRuntimePatchForWorkspace() should remove standalone updatedAt sort: %q", content)
	}
	if !strings.Contains(content, ".toList();") {
		t.Fatalf("normalizeBuilderRuntimePatchForWorkspace() should preserve list materialization when removing cascade sort: %q", content)
	}
}

func TestNormalizeBuilderRuntimePatchForWorkspaceRewritesUpdatedAtSortsToCurrentTimeField(t *testing.T) {
	workspacePath := t.TempDir()
	recordPath := filepath.Join(workspacePath, "lib", "models", "record.dart")
	if err := os.MkdirAll(filepath.Dir(recordPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(recordPath) error = %v", err)
	}
	if err := os.WriteFile(recordPath, []byte(strings.Join([]string{
		"class AppRecord {",
		"  const AppRecord({required this.taskId, required this.recordedAt});",
		"  final String taskId;",
		"  final DateTime recordedAt;",
		"}",
	}, "\n")), 0o600); err != nil {
		t.Fatalf("WriteFile(record.dart) error = %v", err)
	}
	patch := appruns.WorkspacePatch{Operations: []appruns.WorkspacePatchOperation{{
		Type:    "write_file",
		Path:    "lib/repositories/local_store.dart",
		Content: "records.sort((left, right) => right.updatedAt.compareTo(left.updatedAt));\n",
	}}}

	normalizeBuilderRuntimePatchForWorkspace(workspacePath, &patch)

	content := patch.Operations[0].Content
	if strings.Contains(content, ".updatedAt") {
		t.Fatalf("normalizeBuilderRuntimePatchForWorkspace() content still references updatedAt: %q", content)
	}
	if !strings.Contains(content, "right.recordedAt.compareTo(left.recordedAt)") {
		t.Fatalf("normalizeBuilderRuntimePatchForWorkspace() should rewrite sort to recordedAt: %q", content)
	}
}

func TestNormalizeBuilderRuntimePatchForWorkspaceRewritesTypedHiveRepositoryToMapBackedPersistence(t *testing.T) {
	workspacePath := t.TempDir()
	recordPath := filepath.Join(workspacePath, "lib", "models", "record.dart")
	if err := os.MkdirAll(filepath.Dir(recordPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(recordPath) error = %v", err)
	}
	if err := os.WriteFile(recordPath, []byte(strings.Join([]string{
		"class TodoItem {",
		"  const TodoItem({required this.taskId, required this.title, required this.category, required this.status, this.note});",
		"  final String taskId;",
		"  final String title;",
		"  final String category;",
		"  final String status;",
		"  final String? note;",
		"  Map<String, dynamic> toMap() {",
		"    return {'task_id': taskId, 'title': title, 'category': category, 'status': status, 'note': note};",
		"  }",
		"  factory TodoItem.fromMap(Map<String, dynamic> map) {",
		"    return TodoItem(taskId: map['task_id'] as String, title: map['title'] as String, category: map['category'] as String, status: map['status'] as String, note: map['note'] as String?);",
		"  }",
		"}",
	}, "\n")), 0o600); err != nil {
		t.Fatalf("WriteFile(record.dart) error = %v", err)
	}
	patch := appruns.WorkspacePatch{Operations: []appruns.WorkspacePatchOperation{{
		Type: "write_file",
		Path: "lib/repositories/local_store.dart",
		Content: strings.Join([]string{
			"import 'package:hive_flutter/hive_flutter.dart';",
			"",
			"import '../models/record.dart';",
			"",
			"abstract class RecordRepository {",
			"  Future<void> init();",
			"  Future<List<TodoItem>> loadRecords();",
			"  Future<void> saveRecords(List<TodoItem> records);",
			"}",
			"",
			"class HiveRecordRepository extends RecordRepository {",
			"  static const String _boxName = 'todo_items';",
			"  static const String _recordsKey = 'items';",
			"",
			"  Box<TodoItem>? _box;",
			"",
			"  @override",
			"  Future<void> init() async {",
			"    await Hive.initFlutter();",
			"    Hive.registerAdapter(TodoItemAdapter());",
			"    _box = await Hive.openBox<TodoItem>(_boxName);",
			"  }",
			"",
			"  Box<TodoItem> get _safeBox {",
			"    final box = _box;",
			"    if (box == null) {",
			"      throw StateError('repository not initialized');",
			"    }",
			"    return box;",
			"  }",
			"",
			"  @override",
			"  Future<List<TodoItem>> loadRecords() async {",
			"    final rawRecords = _safeBox.values.toList();",
			"    return rawRecords",
			"      ..sort((left, right) => left.title.compareTo(right.title));",
			"  }",
			"",
			"  @override",
			"  Future<void> saveRecords(List<TodoItem> records) async {",
			"    await _safeBox.clear();",
			"    for (final record in records) {",
			"      await _safeBox.add(record);",
			"    }",
			"  }",
			"}",
		}, "\n"),
	}}}

	normalizeBuilderRuntimePatchForWorkspace(workspacePath, &patch)

	content := patch.Operations[0].Content
	if strings.Contains(content, "registerAdapter(") {
		t.Fatalf("normalizeBuilderRuntimePatchForWorkspace() should remove registerAdapter from repository content: %q", content)
	}
	if strings.Contains(content, "Box<TodoItem>") {
		t.Fatalf("normalizeBuilderRuntimePatchForWorkspace() should rewrite typed Hive box to Box<dynamic>: %q", content)
	}
	if !strings.Contains(content, "Box<dynamic>? _box;") {
		t.Fatalf("normalizeBuilderRuntimePatchForWorkspace() should rewrite repository storage to Box<dynamic>: %q", content)
	}
	if !strings.Contains(content, "_safeBox.get(_recordsKey, defaultValue: <dynamic>[]) as List<dynamic>") {
		t.Fatalf("normalizeBuilderRuntimePatchForWorkspace() should load persisted map payloads from _recordsKey: %q", content)
	}
	if !strings.Contains(content, "TodoItem.fromMap(Map<String, dynamic>.from(item as Map))") {
		t.Fatalf("normalizeBuilderRuntimePatchForWorkspace() should reconstruct records via TodoItem.fromMap: %q", content)
	}
	if !strings.Contains(content, "records.map((record) => record.toMap()).toList()") {
		t.Fatalf("normalizeBuilderRuntimePatchForWorkspace() should persist records via toMap list payloads: %q", content)
	}
	if !strings.Contains(content, "left.title.compareTo(right.title)") {
		t.Fatalf("normalizeBuilderRuntimePatchForWorkspace() should keep title-based sorting when no time field exists: %q", content)
	}
	if !strings.Contains(content, "item.taskId == record.taskId") {
		t.Fatalf("normalizeBuilderRuntimePatchForWorkspace() should preserve taskId-based update lookup: %q", content)
	}
	if !strings.Contains(content, "item.taskId == taskId") {
		t.Fatalf("normalizeBuilderRuntimePatchForWorkspace() should preserve taskId-based delete lookup: %q", content)
	}
}

func TestNormalizeBuilderRuntimePatchForWorkspaceRewritesConcreteRecordRepositoryToCanonicalMapBackedRepository(t *testing.T) {
	workspacePath := t.TempDir()
	recordPath := filepath.Join(workspacePath, "lib", "models", "record.dart")
	if err := os.MkdirAll(filepath.Dir(recordPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(recordPath) error = %v", err)
	}
	if err := os.WriteFile(recordPath, []byte(strings.Join([]string{
		"class WeightRecord {",
		"  const WeightRecord({required this.recordId, required this.weight, required this.recordedAt, this.note = ''});",
		"  final String recordId;",
		"  final double weight;",
		"  final DateTime recordedAt;",
		"  final String note;",
		"  Map<String, dynamic> toMap() {",
		"    return {'record_id': recordId, 'weight': weight, 'recorded_at': recordedAt.toIso8601String(), 'note': note};",
		"  }",
		"  factory WeightRecord.fromMap(Map<String, dynamic> map) {",
		"    return WeightRecord(recordId: map['record_id'] as String, weight: (map['weight'] as num).toDouble(), recordedAt: DateTime.parse(map['recorded_at'] as String), note: map['note'] as String? ?? '');",
		"  }",
		"}",
	}, "\n")), 0o600); err != nil {
		t.Fatalf("WriteFile(record.dart) error = %v", err)
	}
	patch := appruns.WorkspacePatch{Operations: []appruns.WorkspacePatchOperation{{
		Type: "write_file",
		Path: "lib/repositories/record_repository.dart",
		Content: strings.Join([]string{
			"import 'package:hive_flutter/hive_flutter.dart';",
			"",
			"import '../models/record.dart';",
			"",
			"class RecordRepository {",
			"  static const String _boxName = 'weight_records';",
			"  static const String _recordsKey = 'records';",
			"",
			"  Box<dynamic>? _box;",
			"",
			"  Future<void> init() async {",
			"    await Hive.initFlutter();",
			"    _box = await Hive.openBox<dynamic>(_boxName);",
			"  }",
			"",
			"  Future<List<WeightRecord>> loadRecords() async {",
			"    final rawRecords = _box?.get(_recordsKey, defaultValue: <dynamic>[]) as List<dynamic>? ?? <dynamic>[];",
			"    return rawRecords.map((item) => WeightRecord.fromMap(Map<String, dynamic>.from(item as Map))).toList();",
			"  }",
			"",
			"  Future<void> saveRecords(List<WeightRecord> records) async {",
			"    await _box?.put(_recordsKey, records.map((record) => record.toMap()).toList());",
			"  }",
			"}",
		}, "\n"),
	}}}

	normalizeBuilderRuntimePatchForWorkspace(workspacePath, &patch)

	content := patch.Operations[0].Content
	for _, want := range []string{
		"abstract class RecordRepository {",
		"class HiveRecordRepository extends RecordRepository {",
		"class InMemoryRecordRepository extends RecordRepository {",
		"Box<dynamic>? _box;",
		"WeightRecord.fromMap(Map<String, dynamic>.from(item as Map))",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("normalizeBuilderRuntimePatchForWorkspace() content missing %q: %q", want, content)
		}
	}
	if strings.Contains(content, "\nclass RecordRepository {\n") {
		t.Fatalf("normalizeBuilderRuntimePatchForWorkspace() should not keep concrete RecordRepository implementation wrapper: %q", content)
	}
}

func TestNormalizeBuilderRuntimePatchForWorkspaceWithTaskBundleRemovesRepositoryDeleteWhenTopologyPrunesDelete(t *testing.T) {
	workspacePath := t.TempDir()
	recordPath := filepath.Join(workspacePath, "lib", "models", "record.dart")
	if err := os.MkdirAll(filepath.Dir(recordPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(recordPath) error = %v", err)
	}
	if err := os.WriteFile(recordPath, []byte(strings.Join([]string{
		"class TodoItem {",
		"  const TodoItem({required this.taskId, required this.title, required this.category, required this.status, this.note});",
		"  final String taskId;",
		"  final String title;",
		"  final String category;",
		"  final String status;",
		"  final String? note;",
		"  Map<String, dynamic> toMap() {",
		"    return {'task_id': taskId, 'title': title, 'category': category, 'status': status, 'note': note};",
		"  }",
		"  factory TodoItem.fromMap(Map<String, dynamic> map) {",
		"    return TodoItem(taskId: map['task_id'] as String, title: map['title'] as String, category: map['category'] as String, status: map['status'] as String, note: map['note'] as String?);",
		"  }",
		"}",
	}, "\n")), 0o600); err != nil {
		t.Fatalf("WriteFile(record.dart) error = %v", err)
	}
	patch := appruns.WorkspacePatch{Operations: []appruns.WorkspacePatchOperation{{
		Type: "write_file",
		Path: "lib/repositories/record_repository.dart",
		Content: strings.Join([]string{
			"import 'package:hive_flutter/hive_flutter.dart';",
			"",
			"import '../models/record.dart';",
			"",
			"abstract class RecordRepository {",
			"  Future<void> init();",
			"  Future<List<TodoItem>> loadRecords();",
			"  Future<void> saveRecords(List<TodoItem> records);",
			"",
			"  Future<void> addRecord(TodoItem record) async {",
			"    final records = await loadRecords();",
			"    records.add(record);",
			"    await saveRecords(records);",
			"  }",
			"",
			"  Future<void> updateRecord(TodoItem record) async {",
			"    final records = await loadRecords();",
			"    final index = records.indexWhere((item) => item.taskId == record.taskId);",
			"    if (index < 0) {",
			"      throw StateError('record not found');",
			"    }",
			"    records[index] = record;",
			"    await saveRecords(records);",
			"  }",
			"",
			"  Future<void> deleteRecord(String taskId) async {",
			"    final records = await loadRecords();",
			"    records.removeWhere((item) => item.taskId == taskId);",
			"    await saveRecords(records);",
			"  }",
			"}",
			"",
			"class HiveRecordRepository extends RecordRepository {",
			"  static const String _boxName = 'flutter_open_lite';",
			"  static const String _recordsKey = 'records';",
			"",
			"  Box<dynamic>? _box;",
			"",
			"  @override",
			"  Future<void> init() async {",
			"    await Hive.initFlutter();",
			"    _box = await Hive.openBox<dynamic>(_boxName);",
			"  }",
			"",
			"  @override",
			"  Future<List<TodoItem>> loadRecords() async {",
			"    final rawRecords = _box?.get(_recordsKey, defaultValue: <dynamic>[]) as List<dynamic>? ?? <dynamic>[];",
			"    return rawRecords.map((item) => TodoItem.fromMap(Map<String, dynamic>.from(item as Map))).toList();",
			"  }",
			"",
			"  @override",
			"  Future<void> saveRecords(List<TodoItem> records) async {",
			"    await _box?.put(_recordsKey, records.map((record) => record.toMap()).toList());",
			"  }",
			"}",
		}, "\n"),
	}}}
	taskBundle := []appruns.TaskBundleItem{{
		TaskID:      "task-create-repository",
		TaskType:    appruns.BuilderRuntimeTaskTypeDualFileWiring,
		TargetPaths: []string{"lib/repositories/record_repository.dart"},
	}}

	normalizeBuilderRuntimePatchForWorkspaceWithTaskBundle(workspacePath, taskBundle, &patch)

	content := patch.Operations[0].Content
	if strings.Contains(content, "deleteRecord") {
		t.Fatalf("normalizeBuilderRuntimePatchForWorkspaceWithTaskBundle() should remove deleteRecord when topology prunes delete: %q", content)
	}
	if !strings.Contains(content, "Future<void> addRecord(TodoItem record) async") {
		t.Fatalf("normalizeBuilderRuntimePatchForWorkspaceWithTaskBundle() should preserve addRecord: %q", content)
	}
	if !strings.Contains(content, "Future<void> updateRecord(TodoItem record) async") {
		t.Fatalf("normalizeBuilderRuntimePatchForWorkspaceWithTaskBundle() should preserve updateRecord: %q", content)
	}
}

func TestNormalizeBuilderRuntimePatchForWorkspaceWithTaskBundleAddsCollectionCreateEntryFromTopology(t *testing.T) {
	workspacePath := t.TempDir()
	writeBuilderRuntimeWorkspaceFiles(t, workspacePath, map[string]string{
		"lib/views/home_page.dart":                    "class HomePage {}\n",
		"lib/views/record_form_page.dart":             "class RecordFormPage {}\n",
		"lib/controllers/record_list_controller.dart": "class RecordListController { Future<void> refresh() async {} }\n",
		"lib/controllers/record_form_controller.dart": "import 'package:flutter/material.dart';\nimport '../models/record.dart';\nclass RecordFormController extends ChangeNotifier {\n  final TextEditingController titleController = TextEditingController();\n  final TextEditingController noteController = TextEditingController();\n  final TextEditingController categoryController = TextEditingController();\n  RecordStatus get selectedStatus => RecordStatus.inbox;\n}\n",
		"lib/models/record.dart":                      "enum RecordStatus { inbox, inProgress, done }\nclass AppRecord {\n  const AppRecord({required this.title, required this.category, required this.status, this.note = ''});\n  final String title;\n  final String category;\n  final RecordStatus status;\n  final String note;\n}\n",
	})
	patch := appruns.WorkspacePatch{Operations: []appruns.WorkspacePatchOperation{{
		Type: "write_file",
		Path: "lib/views/record_list_page.dart",
		Content: strings.Join([]string{
			"import 'package:flutter/material.dart';",
			"",
			"import '../controllers/record_list_controller.dart';",
			"import '../models/record.dart';",
			"import '../template/open_lite_copy.dart';",
			"",
			"class RecordListPage extends StatelessWidget {",
			"  const RecordListPage({",
			"    super.key,",
			"    required this.controller,",
			"    required this.onOpenRecordDetail,",
			"  });",
			"",
			"  final RecordListController controller;",
			"  final Future<void> Function(AppRecord record) onOpenRecordDetail;",
			"",
			"  @override",
			"  Widget build(BuildContext context) {",
			"    return AnimatedBuilder(",
			"      animation: controller,",
			"      builder: (context, _) {",
			"        final records = controller.records;",
			"        final visibleRecords = controller.visibleRecords;",
			"        return Scaffold(",
			"          appBar: AppBar(title: Text(openLiteCopy.listPageTitle)),",
			"          body: records.isEmpty",
			"              ? Center(child: Text(openLiteCopy.listEmptyLabel))",
			"              : ListView(children: const []),",
			"        );",
			"      },",
			"    );",
			"  }",
			"}",
		}, "\n"),
	}}}
	taskBundle := []appruns.TaskBundleItem{
		{TaskID: "task-bind-collection-surface", TargetPaths: []string{"lib/views/record_list_page.dart", "lib/controllers/record_list_controller.dart"}},
		{TaskID: "task-bind-mutation-surface", TargetPaths: []string{"lib/views/record_form_page.dart", "lib/controllers/record_form_controller.dart"}},
	}

	normalizeBuilderRuntimePatchForWorkspaceWithTaskBundle(workspacePath, taskBundle, &patch)

	content := patch.Operations[0].Content
	if !strings.Contains(content, "required this.onCreateRecord,") {
		t.Fatalf("normalizeBuilderRuntimePatchForWorkspaceWithTaskBundle() should add create callback parameter: %q", content)
	}
	if !strings.Contains(content, "final Future<void> Function() onCreateRecord;") {
		t.Fatalf("normalizeBuilderRuntimePatchForWorkspaceWithTaskBundle() should add create callback field: %q", content)
	}
	if !strings.Contains(content, "FloatingActionButton.extended(") {
		t.Fatalf("normalizeBuilderRuntimePatchForWorkspaceWithTaskBundle() should expose collection-root create action: %q", content)
	}
	if strings.Contains(content, "RecordListFilter") {
		t.Fatalf("normalizeBuilderRuntimePatchForWorkspaceWithTaskBundle() should drop unexpected filter flow for no-home topology: %q", content)
	}
}

func TestNormalizeBuilderRuntimePatchForWorkspaceWithTaskBundleAddsCollectionCreateEntryFromWorkspaceForCustomPathsWithoutSurfaceRefs(t *testing.T) {
	workspacePath := t.TempDir()
	writeBuilderRuntimeWorkspaceFiles(t, workspacePath, map[string]string{
		"lib/models/task.dart":                            "enum TaskStatus { todo, doing, done }\nclass Task {\n  const Task({required this.title, required this.status, this.note = ''});\n  final String title;\n  final TaskStatus status;\n  final String note;\n}\n",
		"lib/repositories/task_repository.dart":           "abstract class TaskRepository {}\n",
		"lib/controllers/task_collection_controller.dart": "import 'package:flutter/foundation.dart';\nimport '../models/task.dart';\nimport '../repositories/task_repository.dart';\nclass TaskCollectionController extends ChangeNotifier {\n  TaskCollectionController({required TaskRepository taskRepository});\n  List<Task> get records => const [];\n  List<Task> get visibleRecords => records;\n}\n",
		"lib/controllers/task_form_controller.dart":       "import 'package:flutter/material.dart';\nimport '../models/task.dart';\nclass TaskFormController extends ChangeNotifier {\n  final TextEditingController titleController = TextEditingController();\n  final TextEditingController noteController = TextEditingController();\n  TaskStatus get selectedStatus => TaskStatus.todo;\n}\n",
		"lib/views/task_form_page.dart":                   "class TaskFormPage {}\n",
	})
	patch := appruns.WorkspacePatch{Operations: []appruns.WorkspacePatchOperation{{
		Type: "write_file",
		Path: "lib/views/task_collection_page.dart",
		Content: strings.Join([]string{
			"import 'package:flutter/material.dart';",
			"",
			"import '../controllers/task_collection_controller.dart';",
			"import '../models/task.dart';",
			"import '../template/open_lite_copy.dart';",
			"",
			"class TaskCollectionPage extends StatelessWidget {",
			"  const TaskCollectionPage({",
			"    super.key,",
			"    required this.controller,",
			"    required this.onOpenTaskDetail,",
			"  });",
			"",
			"  final TaskCollectionController controller;",
			"  final Future<void> Function(Task task) onOpenTaskDetail;",
			"",
			"  @override",
			"  Widget build(BuildContext context) {",
			"    return AnimatedBuilder(",
			"      animation: controller,",
			"      builder: (context, _) {",
			"        final records = controller.records;",
			"        final visibleRecords = controller.visibleRecords;",
			"        return Scaffold(",
			"          appBar: AppBar(title: Text(openLiteCopy.listPageTitle)),",
			"          body: records.isEmpty",
			"              ? Center(child: Text(openLiteCopy.listEmptyLabel))",
			"              : ListView(children: const []),",
			"        );",
			"      },",
			"    );",
			"  }",
			"}",
		}, "\n"),
	}}}
	taskBundle := []appruns.TaskBundleItem{
		{TaskID: "task-bind-collection-surface", TargetPaths: []string{"lib/views/task_collection_page.dart", "lib/controllers/task_collection_controller.dart"}},
		{TaskID: "task-bind-mutation-surface", TargetPaths: []string{"lib/views/task_form_page.dart", "lib/controllers/task_form_controller.dart"}},
	}

	normalizeBuilderRuntimePatchForWorkspaceWithTaskBundle(workspacePath, taskBundle, &patch)

	content := patch.Operations[0].Content
	for _, want := range []string{
		"required this.onCreateTask,",
		"final Future<void> Function() onCreateTask;",
		"FloatingActionButton.extended(",
		"onPressed: onCreateTask,",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("normalizeBuilderRuntimePatchForWorkspaceWithTaskBundle() should use workspace create-entry fallback for custom paths without surface refs and include %q: %q", want, content)
		}
	}
}

func TestNormalizeBuilderRuntimeOpenLiteListPageCreateEntryAvoidsDoubleComma(t *testing.T) {
	original := strings.Join([]string{
		"class RecordListPage extends StatelessWidget {",
		"  const RecordListPage({",
		"    super.key,",
		"    required this.controller,",
		"    required this.onOpenRecordDetail,",
		"  });",
		"",
		"  final Object controller;",
		"  final Future<void> Function(Object record) onOpenRecordDetail;",
		"",
		"  Widget build(BuildContext context) {",
		"    return Scaffold(",
		"      appBar: AppBar(title: const Text('List')),",
		"      body: const SizedBox.shrink(),",
		"    );",
		"  }",
		"}",
	}, "\n")

	updated := normalizeBuilderRuntimeOpenLiteListPageCreateEntry(true, original)
	if !strings.Contains(updated, "FloatingActionButton.extended(") {
		t.Fatalf("normalizeBuilderRuntimeOpenLiteListPageCreateEntry() should add collection-root create entry: %q", updated)
	}
	if strings.Contains(updated, "\n        ,\n") {
		t.Fatalf("normalizeBuilderRuntimeOpenLiteListPageCreateEntry() should not leave an isolated comma line: %q", updated)
	}
	if strings.Contains(updated, "body: const SizedBox.shrink(),\n,\n") {
		t.Fatalf("normalizeBuilderRuntimeOpenLiteListPageCreateEntry() should avoid double commas after an existing trailing comma: %q", updated)
	}
}

func TestNormalizeBuilderRuntimeOpenLiteListPageCreateCallbackUsesTaskDomainCreateCallback(t *testing.T) {
	original := strings.Join([]string{
		"class TaskCollectionPage extends StatelessWidget {",
		"  const TaskCollectionPage({",
		"    super.key,",
		"    required this.controller,",
		"    required this.onOpenTaskDetail,",
		"  });",
		"",
		"  final Object controller;",
		"  final Future<void> Function(Task task) onOpenTaskDetail;",
		"}",
	}, "\n")

	updated := normalizeBuilderRuntimeOpenLiteListPageCreateCallback(original)
	if !strings.Contains(updated, "required this.onCreateTask,") {
		t.Fatalf("normalizeBuilderRuntimeOpenLiteListPageCreateCallback() should inject task-domain create callback parameter: %q", updated)
	}
	if !strings.Contains(updated, "final Future<void> Function() onCreateTask;") {
		t.Fatalf("normalizeBuilderRuntimeOpenLiteListPageCreateCallback() should inject task-domain create callback field: %q", updated)
	}
	if strings.Contains(updated, "onCreateRecord") {
		t.Fatalf("normalizeBuilderRuntimeOpenLiteListPageCreateCallback() should not fall back to record-domain create callback for task collection page: %q", updated)
	}
	if !strings.Contains(updated, "final Future<void> Function(Task task) onOpenTaskDetail;") {
		t.Fatalf("normalizeBuilderRuntimeOpenLiteListPageCreateCallback() should preserve task detail callback signature: %q", updated)
	}
}

func TestNormalizeBuilderRuntimeOpenLiteListPageCreateEntryUsesTaskDomainCallbackAndCopyLabel(t *testing.T) {
	original := strings.Join([]string{
		"import '../template/open_lite_copy.dart';",
		"",
		"class TaskCollectionPage extends StatelessWidget {",
		"  const TaskCollectionPage({",
		"    super.key,",
		"    required this.controller,",
		"    required this.onCreateTask,",
		"    required this.onOpenTaskDetail,",
		"  });",
		"",
		"  final Object controller;",
		"  final Future<void> Function() onCreateTask;",
		"  final Future<void> Function(Task task) onOpenTaskDetail;",
		"",
		"  Widget build(BuildContext context) {",
		"    return Scaffold(",
		"      body: const SizedBox.shrink(),",
		"    );",
		"  }",
		"}",
	}, "\n")

	updated := normalizeBuilderRuntimeOpenLiteListPageCreateEntry(true, original)
	if !strings.Contains(updated, "onPressed: onCreateTask,") {
		t.Fatalf("normalizeBuilderRuntimeOpenLiteListPageCreateEntry() should use task-domain create callback when present: %q", updated)
	}
	if !strings.Contains(updated, "label: Text(openLiteCopy.createPrimaryActionLabel),") {
		t.Fatalf("normalizeBuilderRuntimeOpenLiteListPageCreateEntry() should reuse template create label when openLiteCopy is available: %q", updated)
	}
	if strings.Contains(updated, "const Text('Create')") {
		t.Fatalf("normalizeBuilderRuntimeOpenLiteListPageCreateEntry() should not hard-code Create label when template copy is available: %q", updated)
	}
}

func TestNormalizeBuilderRuntimeViewContentAddsCreateEntryToCustomCollectionPage(t *testing.T) {
	workspacePath := t.TempDir()
	original := strings.Join([]string{
		"import 'package:flutter/material.dart';",
		"import '../template/open_lite_copy.dart';",
		"import '../models/task.dart';",
		"import '../controllers/task_collection_controller.dart';",
		"",
		"class TaskCollectionPage extends StatelessWidget {",
		"  const TaskCollectionPage({",
		"    super.key,",
		"    required this.controller,",
		"    required this.onOpenTaskDetail,",
		"  });",
		"",
		"  final TaskCollectionController controller;",
		"  final Future<void> Function(Task task) onOpenTaskDetail;",
		"",
		"  @override",
		"  Widget build(BuildContext context) {",
		"    return Scaffold(",
		"      appBar: AppBar(title: Text(openLiteCopy.listPageTitle)),",
		"      body: const SizedBox.shrink(),",
		"    );",
		"  }",
		"}",
	}, "\n")

	updated := normalizeBuilderRuntimeViewContent(workspacePath, false, true, original)
	if !strings.Contains(updated, "required this.onCreateTask,") {
		t.Fatalf("normalizeBuilderRuntimeViewContent() should inject task-domain create callback for custom collection page: %q", updated)
	}
	if !strings.Contains(updated, "final Future<void> Function() onCreateTask;") {
		t.Fatalf("normalizeBuilderRuntimeViewContent() should add task-domain create callback field for custom collection page: %q", updated)
	}
	if !strings.Contains(updated, "onPressed: onCreateTask,") {
		t.Fatalf("normalizeBuilderRuntimeViewContent() should wire floating action button to task-domain create callback: %q", updated)
	}
	if !strings.Contains(updated, "label: Text(openLiteCopy.createPrimaryActionLabel),") {
		t.Fatalf("normalizeBuilderRuntimeViewContent() should reuse template create label for custom collection page: %q", updated)
	}
}

func TestNormalizeBuilderRuntimeViewContentDoesNotCanonicalizeListPageIntoFormPage(t *testing.T) {
	workspacePath := t.TempDir()
	writeBuilderRuntimeWorkspaceFiles(t, workspacePath, map[string]string{
		"lib/controllers/record_form_controller.dart": "import 'package:flutter/material.dart';\nclass RecordFormController extends ChangeNotifier {}\n",
		"lib/repositories/record_repository.dart":     "abstract class RecordRepository {}\n",
		"lib/models/record.dart":                      "enum RecordStatus { inbox }\nclass AppRecord { const AppRecord({required this.status}); final RecordStatus status; }\n",
	})
	original := strings.Join([]string{
		"import 'package:flutter/material.dart';",
		"",
		"class RecordListPage extends StatelessWidget {",
		"  const RecordListPage({super.key, required this.controller, required this.onOpenRecordDetail});",
		"",
		"  final Listenable controller;",
		"  final Future<void> Function(dynamic record) onOpenRecordDetail;",
		"",
		"  @override",
		"  Widget build(BuildContext context) {",
		"    return Scaffold(",
		"      body: const SizedBox.shrink(),",
		"      floatingActionButton: FloatingActionButton(",
		"        onPressed: () {},",
		"        child: const Icon(Icons.add),",
		"      ),",
		"    );",
		"  }",
		"}",
	}, "\n")

	updated := normalizeBuilderRuntimeViewContent(workspacePath, false, true, original)
	if !strings.Contains(updated, "class RecordListPage") {
		t.Fatalf("normalizeBuilderRuntimeViewContent() should preserve list page identity: %q", updated)
	}
	if strings.Contains(updated, "class RecordFormPage") {
		t.Fatalf("normalizeBuilderRuntimeViewContent() should not rewrite record_list_page into RecordFormPage: %q", updated)
	}
}

func TestNormalizeBuilderRuntimePatchForWorkspaceWithTaskBundleWiresMainCreateCallbackFromTopology(t *testing.T) {
	workspacePath := t.TempDir()
	writeBuilderRuntimeWorkspaceFiles(t, workspacePath, map[string]string{
		"lib/views/home_page.dart":                    "class HomePage {}\n",
		"lib/views/record_form_page.dart":             "class RecordFormPage {}\n",
		"lib/controllers/record_list_controller.dart": "import '../repositories/record_repository.dart';\nclass RecordListController {\n  RecordListController({required RecordRepository repository});\n  Future<void> refresh() async {}\n}\n",
		"lib/controllers/record_form_controller.dart": "import 'package:flutter/material.dart';\nimport '../models/record.dart';\nclass RecordFormController extends ChangeNotifier {\n  final TextEditingController titleController = TextEditingController();\n  final TextEditingController noteController = TextEditingController();\n  final TextEditingController categoryController = TextEditingController();\n  RecordStatus get selectedStatus => RecordStatus.inbox;\n}\n",
		"lib/models/record.dart":                      "enum RecordStatus { inbox, inProgress, done }\nclass AppRecord {\n  const AppRecord({required this.title, required this.category, required this.status, this.note = ''});\n  final String title;\n  final String category;\n  final RecordStatus status;\n  final String note;\n}\n",
		"lib/repositories/record_repository.dart":     "abstract class RecordRepository {}\nclass HiveRecordRepository extends RecordRepository { Future<void> init() async {} }\n",
	})
	patch := appruns.WorkspacePatch{Operations: []appruns.WorkspacePatchOperation{{
		Type: "write_file",
		Path: "lib/main.dart",
		Content: strings.Join([]string{
			"import 'package:flutter/material.dart';",
			"",
			"import 'controllers/record_list_controller.dart';",
			"import 'repositories/record_repository.dart';",
			"import 'views/record_list_page.dart';",
			"",
			"Future<void> main() async {",
			"  final repository = HiveRecordRepository();",
			"  await repository.init();",
			"  runApp(TodoApp(repository: repository));",
			"}",
			"",
			"class TodoApp extends StatelessWidget {",
			"  const TodoApp({super.key, required this.repository});",
			"",
			"  final RecordRepository repository;",
			"",
			"  @override",
			"  Widget build(BuildContext context) {",
			"    final listController = RecordListController(repository: repository);",
			"    return MaterialApp(",
			"      home: RecordListPage(",
			"        controller: listController,",
			"        onOpenRecordDetail: (record) async {},",
			"      ),",
			"    );",
			"  }",
			"}",
		}, "\n"),
	}}}
	taskBundle := []appruns.TaskBundleItem{
		{TaskID: "task-bind-collection-surface", TargetPaths: []string{"lib/views/record_list_page.dart", "lib/controllers/record_list_controller.dart"}},
		{TaskID: "task-bind-mutation-surface", TargetPaths: []string{"lib/views/record_form_page.dart", "lib/controllers/record_form_controller.dart"}},
		{TaskID: "task-bind-app-entry", TargetPaths: []string{"lib/main.dart"}},
	}

	normalizeBuilderRuntimePatchForWorkspaceWithTaskBundle(workspacePath, taskBundle, &patch)

	content := patch.Operations[0].Content
	if !strings.Contains(content, "onCreateRecord:") {
		t.Fatalf("normalizeBuilderRuntimePatchForWorkspaceWithTaskBundle() should wire create callback into app entry: %q", content)
	}
	if !strings.Contains(content, "RecordFormPage(") || !strings.Contains(content, "recordRepository: repository") {
		t.Fatalf("normalizeBuilderRuntimePatchForWorkspaceWithTaskBundle() should route create callback into record form page: %q", content)
	}
	if !strings.Contains(content, "navigatorKey: _navigatorKey,") {
		t.Fatalf("normalizeBuilderRuntimePatchForWorkspaceWithTaskBundle() should add navigator key for no-home collection root flow: %q", content)
	}
}

func TestNormalizeBuilderRuntimePatchForWorkspaceWithTaskBundlePrunesDependentOperationsFromMainOnlyPatch(t *testing.T) {
	workspacePath := t.TempDir()
	patch := appruns.WorkspacePatch{Operations: []appruns.WorkspacePatchOperation{
		{Type: "write_file", Path: "lib/main.dart", Content: "void main() {}\n"},
		{Type: "write_file", Path: "lib/views/record_list_page.dart", Content: "class RecordListPage {}\n"},
		{Type: "write_file", Path: "test/widget_test.dart", Content: "void main() {}\n"},
	}}
	taskBundle := []appruns.TaskBundleItem{{TaskID: "task-bind-app-entry", TargetPaths: []string{"lib/main.dart"}}}

	normalizeBuilderRuntimePatchForWorkspaceWithTaskBundle(workspacePath, taskBundle, &patch)

	if len(patch.Operations) != 1 {
		t.Fatalf("len(patch.Operations) = %d, want 1 after pruning main-only dependent edits", len(patch.Operations))
	}
	if patch.Operations[0].Path != "lib/main.dart" {
		t.Fatalf("patch.Operations[0].Path = %q, want lib/main.dart", patch.Operations[0].Path)
	}
}

func TestNormalizeBuilderRuntimeOpenLiteMainContentRepairsInvalidSeedColorLiteral(t *testing.T) {
	workspacePath := t.TempDir()
	content := strings.Join([]string{
		"import 'package:flutter/material.dart';",
		"import 'package:hive_flutter/hive_flutter.dart';",
		"",
		"Future<void> main() async {",
		"  WidgetsFlutterBinding.ensureInitialized();",
		"  await Hive.initFlutter();",
		"  runApp(const TodoApp());",
		"}",
		"",
		"class TodoApp extends StatelessWidget {",
		"  const TodoApp({super.key});",
		"",
		"  @override",
		"  Widget build(BuildContext context) {",
		"    return MaterialApp(",
		"      title: 'Todo Lite',",
		"      theme: ThemeData(",
		"        colorScheme: ColorScheme.fromSeed(seedColor: const Color(0xFF156rag)),",
		"        useMaterial3: true,",
		"      ),",
		"      home: const Placeholder(),",
		"    );",
		"  }",
		"}",
		"",
	}, "\n")

	normalized := normalizeBuilderRuntimeOpenLiteMainContent(workspacePath, false, content)
	if strings.Contains(normalized, "0xFF156rag") {
		t.Fatalf("normalizeBuilderRuntimeOpenLiteMainContent() should remove malformed seedColor literal: %q", normalized)
	}
	if strings.Count(normalized, "ColorScheme.fromSeed(seedColor: const Color(0xFF1565C0))") != 1 {
		t.Fatalf("normalizeBuilderRuntimeOpenLiteMainContent() should inject canonical seedColor exactly once: %q", normalized)
	}
	if !strings.Contains(normalized, "useMaterial3: true") {
		t.Fatalf("normalizeBuilderRuntimeOpenLiteMainContent() should preserve useMaterial3 after seedColor repair: %q", normalized)
	}
}

func TestNormalizeBuilderRuntimeOpenLiteMainContentRepairsRunAppTypoAndNavigatorPushWithoutGeneric(t *testing.T) {
	workspacePath := t.TempDir()
	writeBuilderRuntimeWorkspaceFiles(t, workspacePath, map[string]string{
		"lib/views/record_form_page.dart":             "class RecordFormPage {}\n",
		"lib/views/record_list_page.dart":             "class RecordListPage {}\n",
		"lib/controllers/record_list_controller.dart": "import '../repositories/record_repository.dart';\nclass RecordListController {\n  RecordListController({required RecordRepository repository});\n  Future<void> refresh() async {}\n  Future<void> updateRecord(dynamic record) async {}\n}\n",
		"lib/repositories/record_repository.dart":     "abstract class RecordRepository {}\nclass HiveRecordRepository extends RecordRepository { Future<void> init() async {} }\n",
	})
	content := strings.Join([]string{
		"import 'package:flutter/material.dart';",
		"",
		"import 'controllers/record_list_controller.dart';",
		"import 'repositories/record_repository.dart';",
		"import 'views/record_form_page.dart';",
		"import 'views/record_list_page.dart';",
		"",
		"Future<void> main() async {",
		"  WidgetsFlutterBinding.ensureInitialized();",
		"  final repository = HiveRecordRepository();",
		"  await repository.init();",
		"  runrunApp(TodoApp(repository: repository));",
		"}",
		"",
		"class TodoApp extends StatelessWidget {",
		"  TodoApp({super.key, required this.repository});",
		"",
		"  final RecordRepository repository;",
		"",
		"  @override",
		"  Widget build(BuildContext context) {",
		"    final listController = RecordList",
		"        Controller(repository: repository);",
		"    return MaterialApp(",
		"      home: RecordListPage(",
		"        controller: listController,",
		"        onCreateRecord: () async {",
		"          await Navigator.of(context).push(",
		"            MaterialPageRoute(",
		"              builder: (context) => RecordFormPage(",
		"                recordRepository: repository,",
		"              ),",
		"            ),",
		"          );",
		"          await listController.refresh();",
		"        },",
		"        onOpenRecordDetail: (record) async {",
		"          await Navigator.of(context).push(",
		"            MaterialPageRoute(",
		"              builder: (context) => RecordFormPage(",
		"                recordRepository: repository,",
		"                initialRecord: record,",
		"              ),",
		"            ),",
		"          );",
		"          await listController.refresh();",
		"        },",
		"      ),",
		"    );",
		"  }",
		"}",
		"",
	}, "\n")

	normalized := normalizeBuilderRuntimeOpenLiteMainContent(workspacePath, true, content)
	if strings.Contains(normalized, "runrunApp(") {
		t.Fatalf("normalizeBuilderRuntimeOpenLiteMainContent() should repair runApp typo: %q", normalized)
	}
	if !strings.Contains(normalized, "runApp(TodoApp(repository: repository));") {
		t.Fatalf("normalizeBuilderRuntimeOpenLiteMainContent() should preserve repository wiring after runApp typo repair: %q", normalized)
	}
	if !strings.Contains(normalized, "navigatorKey: _navigatorKey,") {
		t.Fatalf("normalizeBuilderRuntimeOpenLiteMainContent() should add navigator key when rewriting navigator pushes: %q", normalized)
	}
	if strings.Contains(normalized, "final listController = RecordList\n") {
		t.Fatalf("normalizeBuilderRuntimeOpenLiteMainContent() should collapse split list controller declarations before wiring callbacks: %q", normalized)
	}
	if strings.Count(normalized, "final listController = RecordListController(repository: repository);") != 1 {
		t.Fatalf("normalizeBuilderRuntimeOpenLiteMainContent() should leave exactly one canonical list controller declaration: %q", normalized)
	}
	if strings.Contains(normalized, "Navigator.of(context).push(") {
		t.Fatalf("normalizeBuilderRuntimeOpenLiteMainContent() should replace genericless navigator pushes that capture the outer context: %q", normalized)
	}
	if strings.Count(normalized, "_navigatorKey.currentState!.push<dynamic>(") != 2 {
		t.Fatalf("normalizeBuilderRuntimeOpenLiteMainContent() should route both create/detail pushes through navigator key with canonical dynamic push calls: %q", normalized)
	}
}

func TestNormalizeBuilderRuntimeOpenLiteMainContentRepairsTypedNavigatorPushDoubleOpenParen(t *testing.T) {
	workspacePath := t.TempDir()
	writeBuilderRuntimeWorkspaceFiles(t, workspacePath, map[string]string{
		"lib/models/record.dart":                      "class WeightRecord {}\n",
		"lib/views/record_form_page.dart":             "class RecordFormPage {}\n",
		"lib/views/record_list_page.dart":             "class RecordListPage {}\n",
		"lib/controllers/record_list_controller.dart": "import '../repositories/record_repository.dart';\nclass RecordListController {\n  RecordListController({required RecordRepository repository});\n  Future<void> refresh() async {}\n}\n",
		"lib/repositories/record_repository.dart":     "abstract class RecordRepository {}\nclass HiveRecordRepository extends RecordRepository { Future<void> init() async {} }\n",
	})
	content := strings.Join([]string{
		"import 'package:flutter/material.dart';",
		"",
		"import 'controllers/record_list_controller.dart';",
		"import 'models/record.dart';",
		"import 'repositories/record_repository.dart';",
		"import 'views/record_form_page.dart';",
		"import 'views/record_list_page.dart';",
		"",
		"Future<void> main() async {",
		"  WidgetsFlutterBinding.ensureInitialized();",
		"  final repository = HiveRecordRepository();",
		"  await repository.init();",
		"  runApp(TodoApp(repository: repository));",
		"}",
		"",
		"class TodoApp extends StatelessWidget {",
		"  TodoApp({super.key, required this.repository});",
		"",
		"  final RecordRepository repository;",
		"",
		"  @override",
		"  Widget build(BuildContext context) {",
		"    final listController = RecordListController(repository: repository);",
		"    return MaterialApp(",
		"      home: RecordListPage(",
		"        controller: listController,",
		"        onOpenRecordDetail: (record) async {",
		"          final updatedRecord = await Navigator.of(context).push<WeightRecord?>((",
		"            MaterialPageRoute(",
		"              builder: (context) => RecordFormPage(",
		"                recordRepository: repository,",
		"                initialRecord: record,",
		"              ),",
		"            ),",
		"          );",
		"          if (updatedRecord != null) {",
		"            await listController.refresh();",
		"          }",
		"        },",
		"      ),",
		"    );",
		"  }",
		"}",
		"",
	}, "\n")

	normalized := normalizeBuilderRuntimeOpenLiteMainContent(workspacePath, false, content)
	if strings.Contains(normalized, "Navigator.of(context).push<WeightRecord?>((") {
		t.Fatalf("normalizeBuilderRuntimeOpenLiteMainContent() should remove duplicated opening parenthesis in typed push call: %q", normalized)
	}
	if !strings.Contains(normalized, "_navigatorKey.currentState!.push<dynamic>(") {
		t.Fatalf("normalizeBuilderRuntimeOpenLiteMainContent() should route the repaired detail callback through navigator key using the canonical push form: %q", normalized)
	}
}

func TestNormalizeBuilderRuntimeOpenLiteMainContentCanonicalizesExistingFormCallbacksToRefresh(t *testing.T) {
	workspacePath := t.TempDir()
	writeBuilderRuntimeWorkspaceFiles(t, workspacePath, map[string]string{
		"lib/models/record.dart":                      "enum RecordStatus { inbox, inProgress, done }\nclass AppRecord {}\n",
		"lib/views/record_form_page.dart":             "class RecordFormPage {}\n",
		"lib/views/record_list_page.dart":             "class RecordListPage {}\n",
		"lib/controllers/record_list_controller.dart": "import '../repositories/record_repository.dart';\nclass RecordListController {\n  RecordListController({required RecordRepository repository});\n  Future<void> refresh() async {}\n  Future<void> updateRecord(dynamic record) async {}\n}\n",
		"lib/repositories/record_repository.dart":     "abstract class RecordRepository {}\nclass HiveRecordRepository extends RecordRepository { Future<void> init() async {} }\n",
	})
	content := strings.Join([]string{
		"import 'package:flutter/material.dart';",
		"",
		"import 'controllers/record_list_controller.dart';",
		"import 'models/record.dart';",
		"import 'repositories/record_repository.dart';",
		"import 'template/open_lite_copy.dart';",
		"import 'views/record_form_page.dart';",
		"import 'views/record_list_page.dart';",
		"",
		"Future<void> main() async {",
		"  WidgetsFlutterBinding.ensureInitialized();",
		"  final repository = HiveRecordRepository();",
		"  await repository.init();",
		"  runApp(TodoLiteApp(repository: repository));",
		"}",
		"",
		"class TodoLiteApp extends StatelessWidget {",
		"  TodoLiteApp({super.key, required this.repository});",
		"",
		"  final RecordRepository repository;",
		"",
		"  final GlobalKey<NavigatorState> _navigatorKey = GlobalKey<NavigatorState>();",
		"",
		"  @override",
		"  Widget build(BuildContext context) {",
		"    final listController = RecordListController(repository: repository);",
		"    return MaterialApp(",
		"      navigatorKey: _navigatorKey,",
		"      title: openLiteCopy.appTitle,",
		"      home: RecordListPage(",
		"        controller: listController,",
		"        onCreateRecord: () async {",
		"          final newRecord = await _navigatorKey.currentState!.push(",
		"            MaterialPageRoute(",
		"              builder: (context) => RecordFormPage(",
		"                recordRepository: repository,",
		"              ),",
		"            ),",
		"          );",
		"          if (newRecord != null && newRecord is AppRecord) {",
		"            // Navigation handled by controller refresh in list page",
		"          }",
		"        },",
		"        onOpenRecordDetail: (record) async {",
		"          await _navigatorKey.currentState!.push(",
		"            MaterialPageRoute(",
		"              builder: (context) => RecordFormPage(",
		"                recordRepository: repository,",
		"                initialRecord: record,",
		"              ),",
		"            ),",
		"          );",
		"        },",
		"      ),",
		"    );",
		"  }",
		"}",
		"",
	}, "\n")

	normalized := normalizeBuilderRuntimeOpenLiteMainContent(workspacePath, true, content)
	if strings.Contains(normalized, "newRecord is AppRecord") {
		t.Fatalf("normalizeBuilderRuntimeOpenLiteMainContent() should replace stale create callback guards with refresh-based flow: %q", normalized)
	}
	if strings.Count(normalized, "await listController.refresh();") != 2 {
		t.Fatalf("normalizeBuilderRuntimeOpenLiteMainContent() should refresh list controller after create and edit returns: %q", normalized)
	}
	if !strings.Contains(normalized, "final createdRecord = await _navigatorKey.currentState!.push<dynamic>(") {
		t.Fatalf("normalizeBuilderRuntimeOpenLiteMainContent() should canonicalize create callback around record form navigation: %q", normalized)
	}
	if !strings.Contains(normalized, "final updatedRecord = await _navigatorKey.currentState!.push<dynamic>(") {
		t.Fatalf("normalizeBuilderRuntimeOpenLiteMainContent() should canonicalize detail callback around record form navigation: %q", normalized)
	}
	if strings.Contains(normalized, "// Navigation handled by controller refresh in list page") {
		t.Fatalf("normalizeBuilderRuntimeOpenLiteMainContent() should remove stale create callback comments after canonical rewrite: %q", normalized)
	}
}

func TestNormalizeBuilderRuntimeOpenLiteMainContentCanonicalizesExistingFormCallbacksToRefreshForCustomMutationView(t *testing.T) {
	workspacePath := t.TempDir()
	writeBuilderRuntimeWorkspaceFiles(t, workspacePath, map[string]string{
		"lib/models/record.dart":                      "enum RecordStatus { inbox, inProgress, done }\nclass AppRecord {}\n",
		"lib/views/task_form_page.dart":               "class TaskFormPage { const TaskFormPage({super.key, required this.taskRepository, this.initialTask}); final TaskRepository taskRepository; final Object? initialTask; }\n",
		"lib/views/record_list_page.dart":             "class RecordListPage {}\n",
		"lib/controllers/record_list_controller.dart": "import '../repositories/task_repository.dart';\nclass RecordListController {\n  RecordListController({required TaskRepository repository});\n  Future<void> refresh() async {}\n  Future<void> updateRecord(dynamic record) async {}\n}\n",
		"lib/repositories/task_repository.dart":       "abstract class TaskRepository {}\nclass HiveTaskRepository extends TaskRepository { Future<void> init() async {} }\n",
	})
	content := strings.Join([]string{
		"import 'package:flutter/material.dart';",
		"",
		"import 'controllers/record_list_controller.dart';",
		"import 'models/record.dart';",
		"import 'repositories/task_repository.dart';",
		"import 'template/open_lite_copy.dart';",
		"import 'views/record_form_page.dart';",
		"import 'views/task_form_page.dart';",
		"import 'views/record_list_page.dart';",
		"",
		"Future<void> main() async {",
		"  WidgetsFlutterBinding.ensureInitialized();",
		"  final repository = HiveTaskRepository();",
		"  await repository.init();",
		"  runApp(TodoLiteApp(repository: repository));",
		"}",
		"",
		"class TodoLiteApp extends StatelessWidget {",
		"  TodoLiteApp({super.key, required this.repository});",
		"",
		"  final TaskRepository repository;",
		"",
		"  final GlobalKey<NavigatorState> _navigatorKey = GlobalKey<NavigatorState>();",
		"",
		"  @override",
		"  Widget build(BuildContext context) {",
		"    final listController = RecordListController(repository: repository);",
		"    return MaterialApp(",
		"      navigatorKey: _navigatorKey,",
		"      title: openLiteCopy.appTitle,",
		"      home: RecordListPage(",
		"        controller: listController,",
		"        onCreateRecord: () async {",
		"          final newRecord = await _navigatorKey.currentState!.push(",
		"            MaterialPageRoute(",
		"              builder: (context) => TaskFormPage(",
		"                taskRepository: repository,",
		"              ),",
		"            ),",
		"          );",
		"          if (newRecord != null && newRecord is AppRecord) {",
		"            // stale create callback body",
		"          }",
		"        },",
		"        onOpenRecordDetail: (record) async {",
		"          await _navigatorKey.currentState!.push(",
		"            MaterialPageRoute(",
		"              builder: (context) => TaskFormPage(",
		"                taskRepository: repository,",
		"                initialTask: record,",
		"              ),",
		"            ),",
		"          );",
		"        },",
		"      ),",
		"    );",
		"  }",
		"}",
		"",
	}, "\n")

	normalized := normalizeBuilderRuntimeOpenLiteMainContent(workspacePath, true, content)
	if strings.Contains(normalized, "newRecord is AppRecord") {
		t.Fatalf("normalizeBuilderRuntimeOpenLiteMainContent() should replace stale create callback guards for custom mutation view: %q", normalized)
	}
	if strings.Count(normalized, "await listController.refresh();") != 2 {
		t.Fatalf("normalizeBuilderRuntimeOpenLiteMainContent() should refresh list controller after custom create and edit returns: %q", normalized)
	}
	for _, want := range []string{
		"builder: (context) => TaskFormPage(",
		"taskRepository: repository,",
		"initialTask: record,",
		"final createdRecord = await _navigatorKey.currentState!.push<dynamic>(",
		"final updatedRecord = await _navigatorKey.currentState!.push<dynamic>(",
	} {
		if !strings.Contains(normalized, want) {
			t.Fatalf("normalizeBuilderRuntimeOpenLiteMainContent() should keep custom mutation view wiring %q: %q", want, normalized)
		}
	}
	if strings.Contains(normalized, "import 'views/record_form_page.dart';") {
		t.Fatalf("normalizeBuilderRuntimeOpenLiteMainContent() should drop stale default mutation view import when a custom mutation view is active: %q", normalized)
	}
	if strings.Contains(normalized, "// stale create callback body") {
		t.Fatalf("normalizeBuilderRuntimeOpenLiteMainContent() should remove stale custom create callback comments after canonical rewrite: %q", normalized)
	}
}

func TestNormalizeBuilderRuntimeOpenLiteMainContentCanonicalizesExistingFormCallbacksToRefreshForCustomCollectionController(t *testing.T) {
	workspacePath := t.TempDir()
	writeBuilderRuntimeWorkspaceFiles(t, workspacePath, map[string]string{
		"lib/models/record.dart":                          "enum RecordStatus { inbox, inProgress, done }\nclass AppRecord {}\n",
		"lib/views/task_form_page.dart":                   "class TaskFormPage { const TaskFormPage({super.key, required this.taskRepository, this.initialTask}); final TaskRepository taskRepository; final Object? initialTask; }\n",
		"lib/views/record_list_page.dart":                 "class RecordListPage {}\n",
		"lib/controllers/task_collection_controller.dart": "import '../repositories/task_repository.dart';\nclass TaskCollectionController {\n  TaskCollectionController({required TaskRepository taskRepository});\n  Future<void> refresh() async {}\n  Future<void> updateRecord(dynamic record) async {}\n}\n",
		"lib/repositories/task_repository.dart":           "abstract class TaskRepository {}\nclass HiveTaskRepository extends TaskRepository { Future<void> init() async {} }\n",
	})
	content := strings.Join([]string{
		"import 'package:flutter/material.dart';",
		"",
		"import 'controllers/task_collection_controller.dart';",
		"import 'models/record.dart';",
		"import 'repositories/task_repository.dart';",
		"import 'template/open_lite_copy.dart';",
		"import 'views/task_form_page.dart';",
		"import 'views/record_list_page.dart';",
		"",
		"Future<void> main() async {",
		"  WidgetsFlutterBinding.ensureInitialized();",
		"  final repository = HiveTaskRepository();",
		"  await repository.init();",
		"  runApp(TodoLiteApp(repository: repository));",
		"}",
		"",
		"class TodoLiteApp extends StatelessWidget {",
		"  TodoLiteApp({super.key, required this.repository});",
		"",
		"  final TaskRepository repository;",
		"",
		"  final GlobalKey<NavigatorState> _navigatorKey = GlobalKey<NavigatorState>();",
		"",
		"  @override",
		"  Widget build(BuildContext context) {",
		"    return MaterialApp(",
		"      navigatorKey: _navigatorKey,",
		"      title: openLiteCopy.appTitle,",
		"      home: RecordListPage(",
		"        controller: TaskCollectionController(taskRepository: repository),",
		"        onCreateRecord: () async {",
		"          final newRecord = await _navigatorKey.currentState!.push(",
		"            MaterialPageRoute(",
		"              builder: (context) => TaskFormPage(",
		"                taskRepository: repository,",
		"              ),",
		"            ),",
		"          );",
		"          if (newRecord != null && newRecord is AppRecord) {",
		"            // stale create callback body",
		"          }",
		"        },",
		"        onOpenRecordDetail: (record) async {",
		"          await _navigatorKey.currentState!.push(",
		"            MaterialPageRoute(",
		"              builder: (context) => TaskFormPage(",
		"                taskRepository: repository,",
		"                initialTask: record,",
		"              ),",
		"            ),",
		"          );",
		"        },",
		"      ),",
		"    );",
		"  }",
		"}",
		"",
	}, "\n")

	normalized := normalizeBuilderRuntimeOpenLiteMainContent(workspacePath, true, content)
	for _, want := range []string{
		"final listController = TaskCollectionController(taskRepository: repository);",
		"controller: listController,",
		"await listController.refresh();",
		"final updatedRecord = await _navigatorKey.currentState!.push<dynamic>(",
	} {
		if !strings.Contains(normalized, want) {
			t.Fatalf("normalizeBuilderRuntimeOpenLiteMainContent() should normalize custom collection controller wiring %q: %q", want, normalized)
		}
	}
	if strings.Count(normalized, "await listController.refresh();") != 2 {
		t.Fatalf("normalizeBuilderRuntimeOpenLiteMainContent() should refresh custom collection controller after both create and edit returns: %q", normalized)
	}
	if strings.Contains(normalized, "controller: TaskCollectionController(taskRepository: repository),") {
		t.Fatalf("normalizeBuilderRuntimeOpenLiteMainContent() should replace inline custom collection controller wiring with listController variable: %q", normalized)
	}
}

func TestNormalizeBuilderRuntimeOpenLiteMainContentDropsStaleDefaultCollectionControllerImportForCustomSurface(t *testing.T) {
	workspacePath := t.TempDir()
	writeBuilderRuntimeWorkspaceFiles(t, workspacePath, map[string]string{
		"lib/models/record.dart":                          "enum RecordStatus { inbox, inProgress, done }\nclass AppRecord {}\n",
		"lib/views/record_form_page.dart":                 "class RecordFormPage {}\n",
		"lib/views/record_list_page.dart":                 "class RecordListPage {}\n",
		"lib/controllers/task_collection_controller.dart": "import '../repositories/task_repository.dart';\nclass TaskCollectionController {\n  TaskCollectionController({required TaskRepository taskRepository});\n  Future<void> refresh() async {}\n  Future<void> updateRecord(dynamic record) async {}\n}\n",
		"lib/repositories/task_repository.dart":           "abstract class TaskRepository {}\nclass HiveTaskRepository extends TaskRepository { Future<void> init() async {} }\n",
	})
	content := strings.Join([]string{
		"import 'package:flutter/material.dart';",
		"",
		"import 'controllers/record_list_controller.dart';",
		"import 'controllers/task_collection_controller.dart';",
		"import 'models/record.dart';",
		"import 'repositories/task_repository.dart';",
		"import 'views/record_form_page.dart';",
		"import 'views/record_list_page.dart';",
		"",
		"Future<void> main() async {",
		"  WidgetsFlutterBinding.ensureInitialized();",
		"  final repository = HiveTaskRepository();",
		"  await repository.init();",
		"  runApp(TodoLiteApp(repository: repository));",
		"}",
		"",
		"class TodoLiteApp extends StatelessWidget {",
		"  TodoLiteApp({super.key, required this.repository});",
		"",
		"  final TaskRepository repository;",
		"",
		"  @override",
		"  Widget build(BuildContext context) {",
		"    return MaterialApp(",
		"      home: RecordListPage(",
		"        controller: TaskCollectionController(taskRepository: repository),",
		"      ),",
		"    );",
		"  }",
		"}",
	}, "\n") + "\n"

	normalized := normalizeBuilderRuntimeOpenLiteMainContent(workspacePath, false, content)
	if !strings.Contains(normalized, "import 'controllers/task_collection_controller.dart';") {
		t.Fatalf("normalizeBuilderRuntimeOpenLiteMainContent() should keep active custom collection controller import: %q", normalized)
	}
	if strings.Contains(normalized, "import 'controllers/record_list_controller.dart';") {
		t.Fatalf("normalizeBuilderRuntimeOpenLiteMainContent() should drop stale default collection controller import once custom collection controller is active: %q", normalized)
	}
}

func TestNormalizeBuilderRuntimeOpenLiteMainContentDropsStaleDefaultCollectionViewImportForCustomSurface(t *testing.T) {
	workspacePath := t.TempDir()
	writeBuilderRuntimeWorkspaceFiles(t, workspacePath, map[string]string{
		"lib/models/task.dart":                            "class Task {}\n",
		"lib/views/record_list_page.dart":                 "class RecordListPage {}\n",
		"lib/views/task_collection_page.dart":             "import '../controllers/task_collection_controller.dart';\nimport '../models/task.dart';\nclass TaskCollectionPage { const TaskCollectionPage({required this.controller, required this.onOpenTaskDetail}); final TaskCollectionController controller; final Future<void> Function(Task task) onOpenTaskDetail; }\n",
		"lib/controllers/task_collection_controller.dart": "import '../repositories/task_repository.dart';\nclass TaskCollectionController {\n  TaskCollectionController({required TaskRepository taskRepository});\n  Future<void> refresh() async {}\n  Future<void> updateRecord(dynamic record) async {}\n}\n",
		"lib/repositories/task_repository.dart":           "abstract class TaskRepository {}\nclass HiveTaskRepository extends TaskRepository { Future<void> init() async {} }\n",
	})
	content := strings.Join([]string{
		"import 'package:flutter/material.dart';",
		"",
		"import 'controllers/task_collection_controller.dart';",
		"import 'repositories/task_repository.dart';",
		"import 'views/record_list_page.dart';",
		"import 'views/task_collection_page.dart';",
		"",
		"Future<void> main() async {",
		"  WidgetsFlutterBinding.ensureInitialized();",
		"  final repository = HiveTaskRepository();",
		"  await repository.init();",
		"  runApp(TodoLiteApp(repository: repository));",
		"}",
		"",
		"class TodoLiteApp extends StatelessWidget {",
		"  TodoLiteApp({super.key, required this.repository});",
		"",
		"  final TaskRepository repository;",
		"",
		"  @override",
		"  Widget build(BuildContext context) {",
		"    return MaterialApp(",
		"      home: TaskCollectionPage(",
		"        controller: TaskCollectionController(taskRepository: repository),",
		"        onOpenTaskDetail: (task) async {},",
		"      ),",
		"    );",
		"  }",
		"}",
	}, "\n") + "\n"

	normalized := normalizeBuilderRuntimeOpenLiteMainContent(workspacePath, false, content)
	if !strings.Contains(normalized, "import 'views/task_collection_page.dart';") {
		t.Fatalf("normalizeBuilderRuntimeOpenLiteMainContent() should keep active custom collection view import: %q", normalized)
	}
	if strings.Contains(normalized, "import 'views/record_list_page.dart';") {
		t.Fatalf("normalizeBuilderRuntimeOpenLiteMainContent() should drop stale default collection view import once custom collection view is active: %q", normalized)
	}
}

func TestNormalizeBuilderRuntimeOpenLiteMainContentRewritesStaleDefaultListControllerDeclarationForCustomSurface(t *testing.T) {
	workspacePath := t.TempDir()
	writeBuilderRuntimeWorkspaceFiles(t, workspacePath, map[string]string{
		"lib/models/task.dart":                            "class Task {}\n",
		"lib/views/task_collection_page.dart":             "import '../controllers/task_collection_controller.dart';\nimport '../models/task.dart';\nclass TaskCollectionPage { const TaskCollectionPage({required this.controller, required this.onOpenTaskDetail}); final TaskCollectionController controller; final Future<void> Function(Task task) onOpenTaskDetail; }\n",
		"lib/controllers/record_list_controller.dart":     "import '../repositories/task_repository.dart';\nclass RecordListController {\n  RecordListController({required TaskRepository repository});\n}\n",
		"lib/controllers/task_collection_controller.dart": "import '../repositories/task_repository.dart';\nclass TaskCollectionController {\n  TaskCollectionController({required TaskRepository taskRepository});\n  Future<void> refresh() async {}\n  Future<void> updateRecord(dynamic record) async {}\n}\n",
		"lib/repositories/task_repository.dart":           "abstract class TaskRepository {}\nclass HiveTaskRepository extends TaskRepository { Future<void> init() async {} }\n",
	})
	content := strings.Join([]string{
		"import 'package:flutter/material.dart';",
		"",
		"import 'controllers/record_list_controller.dart';",
		"import 'controllers/task_collection_controller.dart';",
		"import 'repositories/task_repository.dart';",
		"import 'views/task_collection_page.dart';",
		"",
		"Future<void> main() async {",
		"  WidgetsFlutterBinding.ensureInitialized();",
		"  final repository = HiveTaskRepository();",
		"  await repository.init();",
		"  runApp(TodoLiteApp(repository: repository));",
		"}",
		"",
		"class TodoLiteApp extends StatelessWidget {",
		"  TodoLiteApp({super.key, required this.repository});",
		"",
		"  final TaskRepository repository;",
		"",
		"  @override",
		"  Widget build(BuildContext context) {",
		"    final listController = RecordListController(repository: repository);",
		"    return MaterialApp(",
		"      home: TaskCollectionPage(",
		"        controller: listController,",
		"        onOpenTaskDetail: (task) async {},",
		"      ),",
		"    );",
		"  }",
		"}",
	}, "\n") + "\n"

	normalized := normalizeBuilderRuntimeOpenLiteMainContent(workspacePath, false, content)
	if !strings.Contains(normalized, "final listController = TaskCollectionController(taskRepository: repository);") {
		t.Fatalf("normalizeBuilderRuntimeOpenLiteMainContent() should rewrite stale default listController declaration to the active custom collection controller: %q", normalized)
	}
	if strings.Contains(normalized, "final listController = RecordListController(repository: repository);") {
		t.Fatalf("normalizeBuilderRuntimeOpenLiteMainContent() should remove stale default listController declaration once a custom collection controller is active: %q", normalized)
	}
	if strings.Contains(normalized, "import 'controllers/record_list_controller.dart';") {
		t.Fatalf("normalizeBuilderRuntimeOpenLiteMainContent() should drop stale default collection controller import after rewriting the listController declaration: %q", normalized)
	}
}

func TestNormalizeBuilderRuntimeOpenLiteMainContentRewritesStaleDefaultListControllerDeclarationForBoardLikeCustomSurface(t *testing.T) {
	workspacePath := t.TempDir()
	writeBuilderRuntimeWorkspaceFiles(t, workspacePath, map[string]string{
		"lib/models/task.dart":                        "class Task {}\n",
		"lib/views/task_board_page.dart":              "import '../controllers/task_board_controller.dart';\nimport '../models/task.dart';\nclass TaskBoardPage { const TaskBoardPage({required this.controller, required this.onOpenTaskDetail}); final TaskBoardController controller; final Future<void> Function(Task task) onOpenTaskDetail; }\n",
		"lib/controllers/record_list_controller.dart": "import '../repositories/task_repository.dart';\nclass RecordListController {\n  RecordListController({required TaskRepository repository});\n}\n",
		"lib/controllers/task_board_controller.dart":  "import '../repositories/task_repository.dart';\nclass TaskBoardController {\n  TaskBoardController({required TaskRepository taskRepository});\n  Future<void> refresh() async {}\n  Future<void> updateRecord(dynamic record) async {}\n}\n",
		"lib/repositories/task_repository.dart":       "abstract class TaskRepository {}\nclass HiveTaskRepository extends TaskRepository { Future<void> init() async {} }\n",
	})
	content := strings.Join([]string{
		"import 'package:flutter/material.dart';",
		"",
		"import 'controllers/record_list_controller.dart';",
		"import 'controllers/task_board_controller.dart';",
		"import 'repositories/task_repository.dart';",
		"import 'views/task_board_page.dart';",
		"",
		"Future<void> main() async {",
		"  WidgetsFlutterBinding.ensureInitialized();",
		"  final repository = HiveTaskRepository();",
		"  await repository.init();",
		"  runApp(TodoLiteApp(repository: repository));",
		"}",
		"",
		"class TodoLiteApp extends StatelessWidget {",
		"  TodoLiteApp({super.key, required this.repository});",
		"",
		"  final TaskRepository repository;",
		"",
		"  @override",
		"  Widget build(BuildContext context) {",
		"    final listController = RecordListController(repository: repository);",
		"    return MaterialApp(",
		"      home: TaskBoardPage(",
		"        controller: listController,",
		"        onOpenTaskDetail: (task) async {},",
		"      ),",
		"    );",
		"  }",
		"}",
	}, "\n") + "\n"

	normalized := normalizeBuilderRuntimeOpenLiteMainContent(workspacePath, false, content)
	if !strings.Contains(normalized, "final listController = TaskBoardController(taskRepository: repository);") {
		t.Fatalf("normalizeBuilderRuntimeOpenLiteMainContent() should rewrite stale default listController declaration to the active board-like custom collection controller: %q", normalized)
	}
	if strings.Contains(normalized, "final listController = RecordListController(repository: repository);") {
		t.Fatalf("normalizeBuilderRuntimeOpenLiteMainContent() should remove stale default listController declaration once a board-like custom collection controller is active: %q", normalized)
	}
	if strings.Contains(normalized, "import 'controllers/record_list_controller.dart';") {
		t.Fatalf("normalizeBuilderRuntimeOpenLiteMainContent() should drop stale default collection controller import after rewriting the board-like listController declaration: %q", normalized)
	}
}

func TestNormalizeBuilderRuntimeOpenLiteEnsureListControllerVariableRewritesStaleDefaultDeclarationForCustomController(t *testing.T) {
	workspacePath := t.TempDir()
	writeBuilderRuntimeWorkspaceFiles(t, workspacePath, map[string]string{
		"lib/models/task.dart":                            "class Task {}\n",
		"lib/repositories/task_repository.dart":           "abstract class TaskRepository {}\n",
		"lib/controllers/record_list_controller.dart":     "import '../repositories/task_repository.dart';\nclass RecordListController {\n  RecordListController({required TaskRepository repository});\n}\n",
		"lib/controllers/task_collection_controller.dart": "import '../repositories/task_repository.dart';\nclass TaskCollectionController {\n  TaskCollectionController({required TaskRepository taskRepository});\n  Future<void> refresh() async {}\n  Future<void> updateRecord(dynamic record) async {}\n}\n",
		"lib/views/task_collection_page.dart":             "import '../controllers/task_collection_controller.dart';\nimport '../models/task.dart';\nclass TaskCollectionPage { const TaskCollectionPage({required this.controller, required this.onOpenTaskDetail}); final TaskCollectionController controller; final Future<void> Function(Task task) onOpenTaskDetail; }\n",
	})
	content := strings.Join([]string{
		"import 'controllers/record_list_controller.dart';",
		"import 'controllers/task_collection_controller.dart';",
		"import 'views/task_collection_page.dart';",
		"",
		"class TodoLiteApp extends StatelessWidget {",
		"  const TodoLiteApp({super.key, required this.repository});",
		"  final TaskRepository repository;",
		"  @override",
		"  Widget build(BuildContext context) {",
		"    final listController = RecordListController(repository: repository);",
		"    return MaterialApp(",
		"      home: TaskCollectionPage(",
		"        controller: listController,",
		"        onOpenTaskDetail: (task) async {},",
		"      ),",
		"    );",
		"  }",
		"}",
	}, "\n") + "\n"

	view := builderRuntimePrimaryCollectionViewCandidate(workspacePath, content)
	controller := builderRuntimePrimaryCollectionControllerCandidate(workspacePath, view.path, view.className, view.controllerParam, view.controllerType, content)
	if controller.className != "TaskCollectionController" || controller.repositoryParam != "taskRepository" {
		t.Fatalf("builderRuntimePrimaryCollectionControllerCandidate() = %+v, want TaskCollectionController(taskRepository)", controller)
	}
	normalized := normalizeBuilderRuntimeOpenLiteEnsureListControllerVariable(content, controller)
	if !strings.Contains(normalized, "final listController = TaskCollectionController(taskRepository: repository);") {
		t.Fatalf("normalizeBuilderRuntimeOpenLiteEnsureListControllerVariable() should rewrite stale default listController declaration to the active custom controller: %q", normalized)
	}
}

func TestNormalizeBuilderRuntimeOpenLiteMainContentInjectsCustomOverviewControllerFromSurfaceCandidate(t *testing.T) {
	workspacePath := createBuilderRuntimeCustomOverviewSurfaceWorkspace(t)
	writeBuilderRuntimeWorkspaceFiles(t, workspacePath, map[string]string{
		"lib/repositories/task_repository.dart": "abstract class TaskRepository {}\nclass HiveTaskRepository extends TaskRepository { Future<void> init() async {} }\n",
	})
	content := strings.Join([]string{
		"import 'package:flutter/material.dart';",
		"",
		"import 'controllers/task_overview_controller.dart';",
		"import 'repositories/task_repository.dart';",
		"import 'views/task_overview_page.dart';",
		"",
		"Future<void> _noop() async {}",
		"",
		"void main() {",
		"  final repository = HiveTaskRepository();",
		"  runApp(TaskTrackerApp(repository: repository));",
		"}",
		"",
		"class TaskTrackerApp extends StatelessWidget {",
		"  const TaskTrackerApp({super.key, required this.repository});",
		"",
		"  final TaskRepository repository;",
		"",
		"  @override",
		"  Widget build(BuildContext context) {",
		"    return MaterialApp(",
		"      home: TaskOverviewPage(",
		"        onCreateTask: _noop,",
		"        onViewAllTasks: _noop,",
		"      ),",
		"    );",
		"  }",
		"}",
	}, "\n") + "\n"

	normalized := normalizeBuilderRuntimeOpenLiteMainContent(workspacePath, false, content)

	if !strings.Contains(normalized, "controller: TaskOverviewController(taskRepository: repository),") {
		t.Fatalf("normalizeBuilderRuntimeOpenLiteMainContent() should inject custom overview controller from surface candidate: %q", normalized)
	}
	if strings.Contains(normalized, "controller: Object()") {
		t.Fatalf("normalizeBuilderRuntimeOpenLiteMainContent() should not fall back to Object() when custom overview surface is available: %q", normalized)
	}
}

func TestNormalizeBuilderRuntimeOpenLiteMainContentInjectsBoardLikeOverviewControllerFromSurfaceCandidate(t *testing.T) {
	workspacePath := t.TempDir()
	writeBuilderRuntimeWorkspaceFiles(t, workspacePath, map[string]string{
		"lib/repositories/task_repository.dart":          "abstract class TaskRepository {}\nclass HiveTaskRepository extends TaskRepository { Future<void> init() async {} }\n",
		"lib/controllers/task_dashboard_controller.dart": "import '../repositories/task_repository.dart';\nclass TaskDashboardController { const TaskDashboardController({required this.taskRepository}); final TaskRepository taskRepository; }\n",
		"lib/views/task_dashboard_page.dart":             "import '../controllers/task_dashboard_controller.dart';\nclass TaskDashboardPage { const TaskDashboardPage({required this.controller, required this.onCreateTask, required this.onViewAllTasks}); final TaskDashboardController controller; final Future<void> Function() onCreateTask; final Future<void> Function() onViewAllTasks; }\n",
	})
	content := strings.Join([]string{
		"import 'package:flutter/material.dart';",
		"",
		"import 'controllers/task_dashboard_controller.dart';",
		"import 'repositories/task_repository.dart';",
		"import 'views/task_dashboard_page.dart';",
		"",
		"Future<void> _noop() async {}",
		"",
		"void main() {",
		"  final repository = HiveTaskRepository();",
		"  runApp(TaskTrackerApp(repository: repository));",
		"}",
		"",
		"class TaskTrackerApp extends StatelessWidget {",
		"  const TaskTrackerApp({super.key, required this.repository});",
		"",
		"  final TaskRepository repository;",
		"",
		"  @override",
		"  Widget build(BuildContext context) {",
		"    return MaterialApp(",
		"      home: TaskDashboardPage(",
		"        onCreateTask: _noop,",
		"        onViewAllTasks: _noop,",
		"      ),",
		"    );",
		"  }",
		"}",
	}, "\n") + "\n"

	normalized := normalizeBuilderRuntimeOpenLiteMainContent(workspacePath, false, content)

	if !strings.Contains(normalized, "controller: TaskDashboardController(taskRepository: repository),") {
		t.Fatalf("normalizeBuilderRuntimeOpenLiteMainContent() should inject board-like custom overview controller from surface candidate: %q", normalized)
	}
	if strings.Contains(normalized, "controller: Object()") {
		t.Fatalf("normalizeBuilderRuntimeOpenLiteMainContent() should not fall back to Object() when board-like custom overview surface is available: %q", normalized)
	}
}

func TestNormalizeBuilderRuntimeOpenLiteMainContentDropsStaleDefaultOverviewImportForCustomSurface(t *testing.T) {
	workspacePath := createBuilderRuntimeCustomOverviewSurfaceWorkspace(t)
	writeBuilderRuntimeWorkspaceFiles(t, workspacePath, map[string]string{
		"lib/repositories/task_repository.dart": "abstract class TaskRepository {}\nclass HiveTaskRepository extends TaskRepository { Future<void> init() async {} }\n",
	})
	content := strings.Join([]string{
		"import 'package:flutter/material.dart';",
		"",
		"import 'controllers/task_overview_controller.dart';",
		"import 'repositories/task_repository.dart';",
		"import 'views/home_page.dart';",
		"import 'views/task_overview_page.dart';",
		"",
		"Future<void> _noop() async {}",
		"",
		"void main() {",
		"  final repository = HiveTaskRepository();",
		"  runApp(TaskTrackerApp(repository: repository));",
		"}",
		"",
		"class TaskTrackerApp extends StatelessWidget {",
		"  const TaskTrackerApp({super.key, required this.repository});",
		"",
		"  final TaskRepository repository;",
		"",
		"  @override",
		"  Widget build(BuildContext context) {",
		"    return MaterialApp(",
		"      home: TaskOverviewPage(",
		"        onCreateTask: _noop,",
		"        onViewAllTasks: _noop,",
		"      ),",
		"    );",
		"  }",
		"}",
	}, "\n") + "\n"

	normalized := normalizeBuilderRuntimeOpenLiteMainContent(workspacePath, false, content)
	if !strings.Contains(normalized, "import 'views/task_overview_page.dart';") {
		t.Fatalf("normalizeBuilderRuntimeOpenLiteMainContent() should keep active custom overview import: %q", normalized)
	}
	if strings.Contains(normalized, "import 'views/home_page.dart';") {
		t.Fatalf("normalizeBuilderRuntimeOpenLiteMainContent() should drop stale default overview import once custom overview surface is active: %q", normalized)
	}
}

func TestNormalizeBuilderRuntimeOpenLiteMainContentDropsStaleDefaultOverviewControllerImportForCustomSurface(t *testing.T) {
	workspacePath := createBuilderRuntimeCustomOverviewSurfaceWorkspace(t)
	writeBuilderRuntimeWorkspaceFiles(t, workspacePath, map[string]string{
		"lib/repositories/task_repository.dart": "abstract class TaskRepository {}\nclass HiveTaskRepository extends TaskRepository { Future<void> init() async {} }\n",
	})
	content := strings.Join([]string{
		"import 'package:flutter/material.dart';",
		"",
		"import 'controllers/home_controller.dart';",
		"import 'controllers/task_overview_controller.dart';",
		"import 'repositories/task_repository.dart';",
		"import 'views/task_overview_page.dart';",
		"",
		"Future<void> _noop() async {}",
		"",
		"void main() {",
		"  final repository = HiveTaskRepository();",
		"  runApp(TaskTrackerApp(repository: repository));",
		"}",
		"",
		"class TaskTrackerApp extends StatelessWidget {",
		"  const TaskTrackerApp({super.key, required this.repository});",
		"",
		"  final TaskRepository repository;",
		"",
		"  @override",
		"  Widget build(BuildContext context) {",
		"    return MaterialApp(",
		"      home: TaskOverviewPage(",
		"        onCreateTask: _noop,",
		"        onViewAllTasks: _noop,",
		"      ),",
		"    );",
		"  }",
		"}",
	}, "\n") + "\n"

	normalized := normalizeBuilderRuntimeOpenLiteMainContent(workspacePath, false, content)
	if !strings.Contains(normalized, "import 'controllers/task_overview_controller.dart';") {
		t.Fatalf("normalizeBuilderRuntimeOpenLiteMainContent() should keep active custom overview controller import: %q", normalized)
	}
	if strings.Contains(normalized, "import 'controllers/home_controller.dart';") {
		t.Fatalf("normalizeBuilderRuntimeOpenLiteMainContent() should drop stale default overview controller import once custom overview surface is active: %q", normalized)
	}
}

func TestNormalizeBuilderRuntimeOpenLiteMainDetailCallbackCanonicalizesPlaceholderToCustomDetailSurface(t *testing.T) {
	workspacePath := createBuilderRuntimeCustomDetailSurfaceWorkspace(t)
	writeBuilderRuntimeWorkspaceFiles(t, workspacePath, map[string]string{
		"lib/repositories/task_repository.dart": "abstract class TaskRepository {}\n",
	})
	content := strings.Join([]string{
		"import 'package:flutter/material.dart';",
		"",
		"import 'controllers/task_collection_controller.dart';",
		"import 'repositories/task_repository.dart';",
		"import 'views/record_list_page.dart';",
		"",
		"class TodoLiteApp extends StatelessWidget {",
		"  const TodoLiteApp({super.key, required this.repository});",
		"  final TaskRepository repository;",
		"  @override",
		"  Widget build(BuildContext context) {",
		"    return MaterialApp(",
		"      home: RecordListPage(",
		"        controller: TaskCollectionController(taskRepository: repository),",
		"        onOpenRecordDetail: (record) async {},",
		"      ),",
		"    );",
		"  }",
		"}",
	}, "\n") + "\n"

	normalized := normalizeBuilderRuntimeOpenLiteMainDetailCallback(workspacePath, content)
	for _, want := range []string{
		"import 'views/task_detail_page.dart';",
		"import 'views/task_form_page.dart';",
		"final listController = TaskCollectionController(taskRepository: repository);",
		"controller: listController,",
		"builder: (context) => TaskDetailPage(",
		"task: record,",
		"onEdit: (updatedRecord) async {",
		"builder: (context) => TaskFormPage(",
		"taskRepository: repository,",
		"initialTask: updatedRecord,",
		"listController.updateRecord(editedRecord);",
	} {
		if !strings.Contains(normalized, want) {
			t.Fatalf("normalizeBuilderRuntimeOpenLiteMainDetailCallback() should wire constructible custom detail surface marker %q: %q", want, normalized)
		}
	}
	if strings.Contains(normalized, "onOpenRecordDetail: (record) async {},") {
		t.Fatalf("normalizeBuilderRuntimeOpenLiteMainDetailCallback() should not keep placeholder callback once custom detail surface is constructible: %q", normalized)
	}
}

func TestNormalizeBuilderRuntimeOpenLiteMainDetailCallbackUsesCustomCollectionCallbackNameWithCustomDetailSurface(t *testing.T) {
	workspacePath := createBuilderRuntimeCustomDetailSurfaceWorkspace(t)
	writeBuilderRuntimeWorkspaceFiles(t, workspacePath, map[string]string{
		"lib/repositories/task_repository.dart": "abstract class TaskRepository {}\n",
		"lib/views/task_collection_page.dart": strings.Join([]string{
			"import '../controllers/task_collection_controller.dart';",
			"",
			"class TaskCollectionPage {",
			"  const TaskCollectionPage({required this.controller, required this.onOpenTaskDetail});",
			"  final TaskCollectionController controller;",
			"  final Future<void> Function(Object task) onOpenTaskDetail;",
			"}",
		}, "\n") + "\n",
	})
	content := strings.Join([]string{
		"import 'package:flutter/material.dart';",
		"",
		"import 'controllers/task_collection_controller.dart';",
		"import 'repositories/task_repository.dart';",
		"import 'views/record_list_page.dart';",
		"",
		"class TodoLiteApp extends StatelessWidget {",
		"  const TodoLiteApp({super.key, required this.repository});",
		"  final TaskRepository repository;",
		"  @override",
		"  Widget build(BuildContext context) {",
		"    return MaterialApp(",
		"      home: RecordListPage(",
		"        controller: TaskCollectionController(taskRepository: repository),",
		"        onOpenTaskDetail: (record) async {},",
		"      ),",
		"    );",
		"  }",
		"}",
	}, "\n") + "\n"

	normalized := normalizeBuilderRuntimeOpenLiteMainDetailCallback(workspacePath, content)
	for _, want := range []string{
		"import 'views/task_detail_page.dart';",
		"import 'views/task_form_page.dart';",
		"final listController = TaskCollectionController(taskRepository: repository);",
		"controller: listController,",
		"onOpenTaskDetail: (record) async {",
		"builder: (context) => TaskDetailPage(",
		"builder: (context) => TaskFormPage(",
	} {
		if !strings.Contains(normalized, want) {
			t.Fatalf("normalizeBuilderRuntimeOpenLiteMainDetailCallback() should keep custom collection callback name with custom detail surface marker %q: %q", want, normalized)
		}
	}
	if strings.Contains(normalized, "onOpenRecordDetail:") {
		t.Fatalf("normalizeBuilderRuntimeOpenLiteMainDetailCallback() should not rewrite custom collection callback back to onOpenRecordDetail when custom detail surface is active: %q", normalized)
	}
	if strings.Contains(normalized, "onOpenTaskDetail: (record) async {},") {
		t.Fatalf("normalizeBuilderRuntimeOpenLiteMainDetailCallback() should not keep placeholder callback once custom detail surface is constructible under custom callback naming: %q", normalized)
	}
}

func TestNormalizeBuilderRuntimeOpenLiteMainDetailCallbackRecognizesCustomEditCallbackName(t *testing.T) {
	workspacePath := createBuilderRuntimeCustomDetailSurfaceWorkspace(t)
	writeBuilderRuntimeWorkspaceFiles(t, workspacePath, map[string]string{
		"lib/repositories/task_repository.dart": "abstract class TaskRepository {}\n",
		"lib/views/task_detail_page.dart": strings.Join([]string{
			"class TaskDetailPage {",
			"  const TaskDetailPage({required this.task, required this.onModifyTask});",
			"  final Object task;",
			"  final Future<void> Function(Object task) onModifyTask;",
			"}",
		}, "\n") + "\n",
	})
	content := strings.Join([]string{
		"import 'package:flutter/material.dart';",
		"",
		"import 'controllers/task_collection_controller.dart';",
		"import 'repositories/task_repository.dart';",
		"import 'views/record_list_page.dart';",
		"",
		"class TodoLiteApp extends StatelessWidget {",
		"  const TodoLiteApp({super.key, required this.repository});",
		"  final TaskRepository repository;",
		"  @override",
		"  Widget build(BuildContext context) {",
		"    return MaterialApp(",
		"      home: RecordListPage(",
		"        controller: TaskCollectionController(taskRepository: repository),",
		"        onOpenRecordDetail: (record) async {},",
		"      ),",
		"    );",
		"  }",
		"}",
	}, "\n") + "\n"

	normalized := normalizeBuilderRuntimeOpenLiteMainDetailCallback(workspacePath, content)
	for _, want := range []string{
		"import 'views/task_detail_page.dart';",
		"import 'views/task_form_page.dart';",
		"final listController = TaskCollectionController(taskRepository: repository);",
		"builder: (context) => TaskDetailPage(",
		"task: record,",
		"onModifyTask: (updatedRecord) async {",
		"builder: (context) => TaskFormPage(",
		"initialTask: updatedRecord,",
	} {
		if !strings.Contains(normalized, want) {
			t.Fatalf("normalizeBuilderRuntimeOpenLiteMainDetailCallback() should support custom edit callback marker %q: %q", want, normalized)
		}
	}
	if strings.Contains(normalized, "onOpenRecordDetail: (record) async {},") {
		t.Fatalf("normalizeBuilderRuntimeOpenLiteMainDetailCallback() should not keep placeholder callback when custom detail edit callback name is recognized: %q", normalized)
	}
}

func TestNormalizeBuilderRuntimeOpenLiteMainDetailCallbackDropsStaleDefaultDetailImportForCustomSurface(t *testing.T) {
	workspacePath := createBuilderRuntimeCustomDetailSurfaceWorkspace(t)
	writeBuilderRuntimeWorkspaceFiles(t, workspacePath, map[string]string{
		"lib/repositories/task_repository.dart": "abstract class TaskRepository {}\n",
	})
	content := strings.Join([]string{
		"import 'package:flutter/material.dart';",
		"",
		"import 'controllers/task_collection_controller.dart';",
		"import 'repositories/task_repository.dart';",
		"import 'views/record_detail_page.dart';",
		"import 'views/record_list_page.dart';",
		"",
		"class TodoLiteApp extends StatelessWidget {",
		"  const TodoLiteApp({super.key, required this.repository});",
		"  final TaskRepository repository;",
		"  @override",
		"  Widget build(BuildContext context) {",
		"    return MaterialApp(",
		"      home: RecordListPage(",
		"        controller: TaskCollectionController(taskRepository: repository),",
		"        onOpenRecordDetail: (record) async {},",
		"      ),",
		"    );",
		"  }",
		"}",
	}, "\n") + "\n"

	normalized := normalizeBuilderRuntimeOpenLiteMainDetailCallback(workspacePath, content)
	if !strings.Contains(normalized, "import 'views/task_detail_page.dart';") {
		t.Fatalf("normalizeBuilderRuntimeOpenLiteMainDetailCallback() should add the active custom detail import: %q", normalized)
	}
	if strings.Contains(normalized, "import 'views/record_detail_page.dart';") {
		t.Fatalf("normalizeBuilderRuntimeOpenLiteMainDetailCallback() should drop stale default detail import once custom detail surface is active: %q", normalized)
	}
}

func TestNormalizeBuilderRuntimeOpenLiteMainContentDropsStaleDefaultDetailImportForConcreteCustomDetailCallback(t *testing.T) {
	workspacePath := createBuilderRuntimeCustomDetailSurfaceWorkspace(t)
	writeBuilderRuntimeWorkspaceFiles(t, workspacePath, map[string]string{
		"lib/repositories/task_repository.dart": "abstract class TaskRepository {}\n",
	})
	content := strings.Join([]string{
		"import 'package:flutter/material.dart';",
		"",
		"import 'controllers/task_collection_controller.dart';",
		"import 'repositories/task_repository.dart';",
		"import 'views/record_detail_page.dart';",
		"import 'views/task_detail_page.dart';",
		"import 'views/record_list_page.dart';",
		"",
		"void main() {",
		"  runApp(TodoLiteApp(repository: repository));",
		"}",
		"",
		"class TodoLiteApp extends StatelessWidget {",
		"  const TodoLiteApp({super.key, required this.repository});",
		"  final TaskRepository repository;",
		"  @override",
		"  Widget build(BuildContext context) {",
		"    return MaterialApp(",
		"      home: RecordListPage(",
		"        controller: TaskCollectionController(taskRepository: repository),",
		"        onOpenRecordDetail: (record) async {",
		"          await Navigator.of(context).push(",
		"            MaterialPageRoute(",
		"              builder: (context) => TaskDetailPage(",
		"                task: record,",
		"                onEdit: (updatedTask) async {},",
		"              ),",
		"            ),",
		"          );",
		"        },",
		"      ),",
		"    );",
		"  }",
		"}",
	}, "\n") + "\n"

	normalized := normalizeBuilderRuntimeOpenLiteMainContent(workspacePath, false, content)
	if !strings.Contains(normalized, "import 'views/task_detail_page.dart';") {
		t.Fatalf("normalizeBuilderRuntimeOpenLiteMainContent() should keep the active custom detail import when a concrete callback already uses it: %q", normalized)
	}
	if strings.Contains(normalized, "import 'views/record_detail_page.dart';") {
		t.Fatalf("normalizeBuilderRuntimeOpenLiteMainContent() should drop stale default detail import even when the custom detail callback is already concrete: %q", normalized)
	}
}

func TestNormalizeBuilderRuntimeOpenLiteMainDetailCallbackCanonicalizesSameNamedControllerArgs(t *testing.T) {
	workspacePath := createBuilderRuntimeCustomDetailSurfaceWorkspace(t)
	writeBuilderRuntimeWorkspaceFiles(t, workspacePath, map[string]string{
		"lib/views/task_detail_page.dart": strings.Join([]string{
			"class TaskDetailPage {",
			"  const TaskDetailPage({required this.task, required this.projects, required this.tags, required this.onEdit});",
			"  final Object task;",
			"  final List<Object> projects;",
			"  final List<Object> tags;",
			"  final Future<void> Function(Object task) onEdit;",
			"}",
		}, "\n") + "\n",
		"lib/controllers/task_collection_controller.dart": strings.Join([]string{
			"import '../repositories/task_repository.dart';",
			"class TaskCollectionController {",
			"  TaskCollectionController({required TaskRepository taskRepository});",
			"  Future<void> refresh() async {}",
			"  Future<void> updateRecord(dynamic record) async {}",
			"  List<Object> get projects => const [];",
			"  List<Object> get tags => const [];",
			"}",
		}, "\n") + "\n",
		"lib/repositories/task_repository.dart": "abstract class TaskRepository {}\n",
	})
	content := strings.Join([]string{
		"import 'package:flutter/material.dart';",
		"",
		"import 'controllers/task_collection_controller.dart';",
		"import 'repositories/task_repository.dart';",
		"import 'views/record_list_page.dart';",
		"",
		"class TodoLiteApp extends StatelessWidget {",
		"  const TodoLiteApp({super.key, required this.repository});",
		"  final TaskRepository repository;",
		"  @override",
		"  Widget build(BuildContext context) {",
		"    return MaterialApp(",
		"      home: RecordListPage(",
		"        controller: TaskCollectionController(taskRepository: repository),",
		"        onOpenRecordDetail: (record) async {},",
		"      ),",
		"    );",
		"  }",
		"}",
	}, "\n") + "\n"

	normalized := normalizeBuilderRuntimeOpenLiteMainDetailCallback(workspacePath, content)
	for _, want := range []string{
		"projects: listController.projects,",
		"tags: listController.tags,",
		"onEdit: (updatedRecord) async {",
	} {
		if !strings.Contains(normalized, want) {
			t.Fatalf("normalizeBuilderRuntimeOpenLiteMainDetailCallback() should wire same-named controller argument %q: %q", want, normalized)
		}
	}
	if strings.Contains(normalized, "onOpenRecordDetail: (record) async {},") {
		t.Fatalf("normalizeBuilderRuntimeOpenLiteMainDetailCallback() should replace placeholder callback when extra args come from listController getters: %q", normalized)
	}
}

func TestNormalizeBuilderRuntimeOpenLiteMainDetailCallbackCanonicalizesRepositoryDerivedTaskTags(t *testing.T) {
	workspacePath := createBuilderRuntimeCustomDetailSurfaceWorkspace(t)
	writeBuilderRuntimeWorkspaceFiles(t, workspacePath, map[string]string{
		"lib/views/task_detail_page.dart": strings.Join([]string{
			"class TaskDetailPage {",
			"  const TaskDetailPage({required this.task, required this.projects, required this.tags, required this.taskTags, required this.onEdit});",
			"  final Object task;",
			"  final List<Object> projects;",
			"  final List<Object> tags;",
			"  final List<String> taskTags;",
			"  final Future<void> Function(Object task) onEdit;",
			"}",
		}, "\n") + "\n",
		"lib/controllers/task_collection_controller.dart": strings.Join([]string{
			"import '../repositories/task_repository.dart';",
			"class TaskCollectionController {",
			"  TaskCollectionController({required TaskRepository taskRepository});",
			"  Future<void> refresh() async {}",
			"  Future<void> updateRecord(dynamic record) async {}",
			"  List<Object> get projects => const [];",
			"  List<Object> get tags => const [];",
			"}",
		}, "\n") + "\n",
		"lib/repositories/task_repository.dart": strings.Join([]string{
			"abstract class TaskRepository {",
			"  Future<List<dynamic>> loadTaskTagLinks();",
			"}",
		}, "\n") + "\n",
	})
	content := strings.Join([]string{
		"import 'package:flutter/material.dart';",
		"",
		"import 'controllers/task_collection_controller.dart';",
		"import 'repositories/task_repository.dart';",
		"import 'views/record_list_page.dart';",
		"",
		"class TodoLiteApp extends StatelessWidget {",
		"  const TodoLiteApp({super.key, required this.repository});",
		"  final TaskRepository repository;",
		"  @override",
		"  Widget build(BuildContext context) {",
		"    return MaterialApp(",
		"      home: RecordListPage(",
		"        controller: TaskCollectionController(taskRepository: repository),",
		"        onOpenRecordDetail: (record) async {},",
		"      ),",
		"    );",
		"  }",
		"}",
	}, "\n") + "\n"

	normalized := normalizeBuilderRuntimeOpenLiteMainDetailCallback(workspacePath, content)
	for _, want := range []string{
		"projects: listController.projects,",
		"tags: listController.tags,",
		"final taskTags = (await repository.loadTaskTagLinks())",
		".where((link) => link.taskId == record.taskId)",
		".map((link) => link.tagId as String)",
		"taskTags: taskTags,",
	} {
		if !strings.Contains(normalized, want) {
			t.Fatalf("normalizeBuilderRuntimeOpenLiteMainDetailCallback() should wire repository-derived detail argument %q: %q", want, normalized)
		}
	}
	if strings.Contains(normalized, "onOpenRecordDetail: (record) async {},") {
		t.Fatalf("normalizeBuilderRuntimeOpenLiteMainDetailCallback() should replace placeholder callback when taskTags can be derived from repository links: %q", normalized)
	}
}

func TestNormalizeBuilderRuntimeOpenLiteMainDetailCallbackCanonicalizesRepositoryDerivedCollectionLoaders(t *testing.T) {
	workspacePath := createBuilderRuntimeCustomDetailSurfaceWorkspace(t)
	writeBuilderRuntimeWorkspaceFiles(t, workspacePath, map[string]string{
		"lib/views/task_detail_page.dart": strings.Join([]string{
			"class TaskDetailPage {",
			"  const TaskDetailPage({required this.task, required this.projects, required this.tags, required this.taskTags, required this.onEdit});",
			"  final Object task;",
			"  final List<Object> projects;",
			"  final List<Object> tags;",
			"  final List<String> taskTags;",
			"  final Future<void> Function(Object task) onEdit;",
			"}",
		}, "\n") + "\n",
		"lib/controllers/task_collection_controller.dart": strings.Join([]string{
			"import '../repositories/task_repository.dart';",
			"class TaskCollectionController {",
			"  TaskCollectionController({required TaskRepository taskRepository});",
			"  Future<void> refresh() async {}",
			"  Future<void> updateRecord(dynamic record) async {}",
			"}",
		}, "\n") + "\n",
		"lib/repositories/task_repository.dart": strings.Join([]string{
			"abstract class TaskRepository {",
			"  Future<List<dynamic>> loadProjects();",
			"  Future<List<dynamic>> loadTags();",
			"  Future<List<dynamic>> loadTaskTagLinks();",
			"}",
		}, "\n") + "\n",
	})
	content := strings.Join([]string{
		"import 'package:flutter/material.dart';",
		"",
		"import 'controllers/task_collection_controller.dart';",
		"import 'repositories/task_repository.dart';",
		"import 'views/record_list_page.dart';",
		"",
		"class TodoLiteApp extends StatelessWidget {",
		"  const TodoLiteApp({super.key, required this.repository});",
		"  final TaskRepository repository;",
		"  @override",
		"  Widget build(BuildContext context) {",
		"    return MaterialApp(",
		"      home: RecordListPage(",
		"        controller: TaskCollectionController(taskRepository: repository),",
		"        onOpenRecordDetail: (record) async {},",
		"      ),",
		"    );",
		"  }",
		"}",
	}, "\n") + "\n"

	normalized := normalizeBuilderRuntimeOpenLiteMainDetailCallback(workspacePath, content)
	for _, want := range []string{
		"final projects = await repository.loadProjects();",
		"final tags = await repository.loadTags();",
		"final taskTags = (await repository.loadTaskTagLinks())",
		"projects: projects,",
		"tags: tags,",
		"taskTags: taskTags,",
		"builder: (context) => TaskDetailPage(",
	} {
		if !strings.Contains(normalized, want) {
			t.Fatalf("normalizeBuilderRuntimeOpenLiteMainDetailCallback() should wire repository-derived collection loader marker %q: %q", want, normalized)
		}
	}
	if strings.Contains(normalized, "onOpenRecordDetail: (record) async {},") {
		t.Fatalf("normalizeBuilderRuntimeOpenLiteMainDetailCallback() should replace placeholder callback when repository collection loaders satisfy custom detail args: %q", normalized)
	}
}

func TestNormalizeBuilderRuntimeOpenLiteMainDetailCallbackCanonicalizesInventoryRepositoryDerivedArgs(t *testing.T) {
	workspacePath := createBuilderRuntimeInventoryRelationRichWorkspace(t, map[string]string{
		"lib/controllers/record_list_controller.dart": strings.Join([]string{
			"import '../repositories/record_repository.dart';",
			"class RecordListController {",
			"  RecordListController({required RecordRepository repository});",
			"  Future<void> refresh() async {}",
			"  Future<void> updateRecord(dynamic record) async {}",
			"}",
		}, "\n") + "\n",
		"lib/views/record_form_page.dart": strings.Join([]string{
			"import '../repositories/record_repository.dart';",
			"class RecordFormPage {",
			"  const RecordFormPage({required this.repository, this.initialSheet});",
			"  final RecordRepository repository;",
			"  final Object? initialSheet;",
			"}",
		}, "\n") + "\n",
		"lib/views/record_detail_page.dart": strings.Join([]string{
			"class RecordDetailPage {",
			"  const RecordDetailPage({required this.sheet, required this.warehouse, required this.items, required this.skus, required this.onEdit});",
			"  final Object sheet;",
			"  final Object? warehouse;",
			"  final List<dynamic> items;",
			"  final List<dynamic> skus;",
			"  final Future<void> Function(Object sheet) onEdit;",
			"}",
		}, "\n") + "\n",
		"lib/repositories/record_repository.dart": strings.Join([]string{
			"abstract class RecordRepository {",
			"  Future<List<dynamic>> loadInventorySheets();",
			"  Future<List<dynamic>> loadLineItems();",
			"  Future<List<dynamic>> loadSkus();",
			"  Future<List<dynamic>> loadWarehouses();",
			"}",
		}, "\n") + "\n",
	})
	content := strings.Join([]string{
		"import 'package:flutter/material.dart';",
		"",
		"import 'controllers/record_list_controller.dart';",
		"import 'repositories/record_repository.dart';",
		"import 'views/record_list_page.dart';",
		"",
		"class InventoryLiteApp extends StatelessWidget {",
		"  const InventoryLiteApp({super.key, required this.repository});",
		"  final RecordRepository repository;",
		"  @override",
		"  Widget build(BuildContext context) {",
		"    return MaterialApp(",
		"      home: RecordListPage(",
		"        controller: RecordListController(repository: repository),",
		"        onOpenRecordDetail: (record) async {},",
		"      ),",
		"    );",
		"  }",
		"}",
	}, "\n") + "\n"

	normalized := normalizeBuilderRuntimeOpenLiteMainDetailCallback(workspacePath, content)
	for _, want := range []string{
		"dynamic warehouse;",
		"for (final candidate in await repository.loadWarehouses()) {",
		"if (candidate.warehouseId == record.warehouseId) {",
		"final items = (await repository.loadLineItems())",
		".where((candidate) => candidate.sheetId == record.sheetId)",
		"final skus = await repository.loadSkus();",
		"warehouse: warehouse,",
		"items: items,",
		"skus: skus,",
	} {
		if !strings.Contains(normalized, want) {
			t.Fatalf("normalizeBuilderRuntimeOpenLiteMainDetailCallback() should wire inventory repository-derived detail argument %q: %q", want, normalized)
		}
	}
	if strings.Contains(normalized, "onOpenRecordDetail: (record) async {},") {
		t.Fatalf("normalizeBuilderRuntimeOpenLiteMainDetailCallback() should replace placeholder callback when inventory detail args can be derived from repository: %q", normalized)
	}
}

func TestNormalizeBuilderRuntimeOpenLiteMainDetailCallbackCanonicalizesRepositoryDerivedSingularRelation(t *testing.T) {
	workspacePath := createBuilderRuntimeRelationRichWorkspace(t, map[string]string{
		"lib/controllers/record_list_controller.dart": strings.Join([]string{
			"import '../repositories/record_repository.dart';",
			"class RecordListController {",
			"  RecordListController({required RecordRepository repository});",
			"  Future<void> refresh() async {}",
			"  Future<void> updateRecord(dynamic record) async {}",
			"}",
		}, "\n") + "\n",
		"lib/views/record_form_page.dart": strings.Join([]string{
			"import '../repositories/record_repository.dart';",
			"class RecordFormPage {",
			"  const RecordFormPage({required this.repository, this.initialTask});",
			"  final RecordRepository repository;",
			"  final Object? initialTask;",
			"}",
		}, "\n") + "\n",
		"lib/views/record_list_page.dart": strings.Join([]string{
			"import '../models/task.dart';",
			"class RecordListPage {",
			"  const RecordListPage({required this.controller, required this.onCreateRecord, required this.onOpenTaskDetail});",
			"  final dynamic controller;",
			"  final Future<void> Function() onCreateRecord;",
			"  final Future<void> Function(Task task) onOpenTaskDetail;",
			"}",
		}, "\n") + "\n",
		"lib/views/record_detail_page.dart": strings.Join([]string{
			"class RecordDetailPage {",
			"  const RecordDetailPage({required this.task, required this.project, required this.onEdit});",
			"  final Object task;",
			"  final Object? project;",
			"  final Future<void> Function(Object task) onEdit;",
			"}",
		}, "\n") + "\n",
	})
	content := strings.Join([]string{
		"import 'package:flutter/material.dart';",
		"",
		"import 'controllers/record_list_controller.dart';",
		"import 'repositories/record_repository.dart';",
		"import 'views/record_list_page.dart';",
		"",
		"class TodoLiteApp extends StatelessWidget {",
		"  const TodoLiteApp({super.key, required this.repository});",
		"  final RecordRepository repository;",
		"  @override",
		"  Widget build(BuildContext context) {",
		"    return MaterialApp(",
		"      home: RecordListPage(",
		"        controller: RecordListController(repository: repository),",
		"        onOpenTaskDetail: (record) async {},",
		"      ),",
		"    );",
		"  }",
		"}",
	}, "\n") + "\n"

	normalized := normalizeBuilderRuntimeOpenLiteMainDetailCallback(workspacePath, content)
	for _, want := range []string{
		"builder: (context) => RecordDetailPage(",
		"project: project,",
		"candidate.projectId == record.projectId",
	} {
		if !strings.Contains(normalized, want) {
			t.Fatalf("normalizeBuilderRuntimeOpenLiteMainDetailCallback() should wire repository-derived singular relation marker %q: %q", want, normalized)
		}
	}
	if strings.Contains(normalized, "onOpenTaskDetail: (record) async {},") {
		t.Fatalf("normalizeBuilderRuntimeOpenLiteMainDetailCallback() should replace placeholder callback when repository can derive a singular relation object: %q", normalized)
	}
}

func TestBuilderRuntimeOpenLiteMainDetailResolvedArgumentExpressionsDerivesSingularProjectFromRepository(t *testing.T) {
	workspacePath := createBuilderRuntimeRelationRichWorkspace(t, map[string]string{
		"lib/controllers/record_list_controller.dart": strings.Join([]string{
			"import '../repositories/record_repository.dart';",
			"class RecordListController {",
			"  RecordListController({required RecordRepository repository});",
			"  Future<void> refresh() async {}",
			"  Future<void> updateRecord(dynamic record) async {}",
			"}",
		}, "\n") + "\n",
		"lib/views/record_list_page.dart": strings.Join([]string{
			"import '../models/task.dart';",
			"class RecordListPage {",
			"  const RecordListPage({required this.controller, required this.onCreateRecord, required this.onOpenTaskDetail});",
			"  final dynamic controller;",
			"  final Future<void> Function() onCreateRecord;",
			"  final Future<void> Function(Task task) onOpenTaskDetail;",
			"}",
		}, "\n") + "\n",
		"lib/views/record_detail_page.dart": strings.Join([]string{
			"class RecordDetailPage {",
			"  const RecordDetailPage({required this.task, required this.project, required this.onEdit});",
			"  final Object task;",
			"  final Object? project;",
			"  final Future<void> Function(Object task) onEdit;",
			"}",
		}, "\n") + "\n",
	})
	surface := builderRuntimePrimaryDetailSurfaceCandidate(workspacePath)
	if surface.view.path == "" || surface.view.className == "" {
		t.Fatalf("detail surface view was not resolved: %+v", surface.view)
	}
	if surface.controller.path == "" || surface.controller.className == "" {
		t.Fatalf("detail surface controller was not resolved: %+v", surface.controller)
	}
	collectionSurface := builderRuntimePrimaryCollectionSurfaceCandidate(workspacePath)
	if collectionSurface.view.detailCallbackName != "onOpenTaskDetail" {
		t.Fatalf("builderRuntimePrimaryCollectionSurfaceCandidate().view.detailCallbackName = %q, want onOpenTaskDetail", collectionSurface.view.detailCallbackName)
	}
	if collectionSurface.view.detailCallbackType != "Future<void> Function(Task task)" {
		t.Fatalf("builderRuntimePrimaryCollectionSurfaceCandidate().view.detailCallbackType = %q, want Future<void> Function(Task task)", collectionSurface.view.detailCallbackType)
	}
	if modelPath := builderRuntimeOpenLitePrimaryRecordModelPath(workspacePath); modelPath != "lib/models/task.dart" {
		t.Fatalf("builderRuntimeOpenLitePrimaryRecordModelPath() = %q, want lib/models/task.dart", modelPath)
	}
	if modelContent := builderRuntimeOpenLitePrimaryRecordModelContent(workspacePath); !strings.Contains(modelContent, "projectId") {
		t.Fatalf("builderRuntimeOpenLitePrimaryRecordModelContent() should expose projectId for singular relation lookup: %q", modelContent)
	}
	resolved := builderRuntimeOpenLiteMainDetailResolvedArgumentExpressions(workspacePath, surface.view, surface.controller)
	project, ok := resolved["project"]
	if !ok {
		t.Fatalf("resolved detail args missing project singular relation for view=%+v controller=%+v: %+v", surface.view, surface.controller, resolved)
	}
	if project.expression != "project" {
		t.Fatalf("resolved project expression = %q, want project", project.expression)
	}
	if len(project.prelude) == 0 || !strings.Contains(strings.Join(project.prelude, "\n"), "candidate.projectId == record.projectId") {
		t.Fatalf("resolved project prelude = %q, want projectId-based repository lookup", strings.Join(project.prelude, "\n"))
	}
}

func TestBuilderRuntimeOpenLiteMainDetailResolvedArgumentExpressionsDerivesTypeAwareProjectAliasesFromRepository(t *testing.T) {
	workspacePath := createBuilderRuntimeRelationRichWorkspace(t, map[string]string{
		"lib/controllers/record_list_controller.dart": strings.Join([]string{
			"import '../repositories/record_repository.dart';",
			"class RecordListController {",
			"  RecordListController({required RecordRepository repository});",
			"  Future<void> refresh() async {}",
			"  Future<void> updateRecord(dynamic record) async {}",
			"}",
		}, "\n") + "\n",
		"lib/views/record_list_page.dart": strings.Join([]string{
			"import '../models/task.dart';",
			"class RecordListPage {",
			"  const RecordListPage({required this.controller, required this.onCreateRecord, required this.onOpenTaskDetail});",
			"  final dynamic controller;",
			"  final Future<void> Function() onCreateRecord;",
			"  final Future<void> Function(Task task) onOpenTaskDetail;",
			"}",
		}, "\n") + "\n",
		"lib/views/record_detail_page.dart": strings.Join([]string{
			"import '../models/project.dart';",
			"class RecordDetailPage {",
			"  const RecordDetailPage({required this.task, required this.availableProjects, required this.currentProject, required this.onEdit});",
			"  final Object task;",
			"  final List<Project> availableProjects;",
			"  final Project? currentProject;",
			"  final Future<void> Function(Object task) onEdit;",
			"}",
		}, "\n") + "\n",
	})
	surface := builderRuntimePrimaryDetailSurfaceCandidate(workspacePath)
	resolved := builderRuntimeOpenLiteMainDetailResolvedArgumentExpressions(workspacePath, surface.view, surface.controller)
	availableProjects, ok := resolved["availableProjects"]
	if !ok {
		t.Fatalf("resolved detail args missing availableProjects alias: %+v", resolved)
	}
	if availableProjects.expression != "availableProjects" {
		t.Fatalf("resolved availableProjects expression = %q, want availableProjects", availableProjects.expression)
	}
	if len(availableProjects.prelude) == 0 || !strings.Contains(strings.Join(availableProjects.prelude, "\n"), "final availableProjects = await repository.loadProjects();") {
		t.Fatalf("resolved availableProjects prelude = %q, want type-aware loadProjects prelude", strings.Join(availableProjects.prelude, "\n"))
	}
	currentProject, ok := resolved["currentProject"]
	if !ok {
		t.Fatalf("resolved detail args missing currentProject alias: %+v", resolved)
	}
	if currentProject.expression != "currentProject" {
		t.Fatalf("resolved currentProject expression = %q, want currentProject", currentProject.expression)
	}
	if len(currentProject.prelude) == 0 || !strings.Contains(strings.Join(currentProject.prelude, "\n"), "candidate.projectId == record.projectId") {
		t.Fatalf("resolved currentProject prelude = %q, want projectId-based type-aware relation lookup", strings.Join(currentProject.prelude, "\n"))
	}
}

func TestNormalizeBuilderRuntimeOpenLiteMainDetailCallbackCanonicalizesTypeAwareRepositoryDerivedAliases(t *testing.T) {
	workspacePath := createBuilderRuntimeRelationRichWorkspace(t, map[string]string{
		"lib/controllers/record_list_controller.dart": strings.Join([]string{
			"import '../repositories/record_repository.dart';",
			"class RecordListController {",
			"  RecordListController({required RecordRepository repository});",
			"  Future<void> refresh() async {}",
			"  Future<void> updateRecord(dynamic record) async {}",
			"}",
		}, "\n") + "\n",
		"lib/views/record_form_page.dart": strings.Join([]string{
			"import '../repositories/record_repository.dart';",
			"class RecordFormPage {",
			"  const RecordFormPage({required this.repository, this.initialTask});",
			"  final RecordRepository repository;",
			"  final Object? initialTask;",
			"}",
		}, "\n") + "\n",
		"lib/views/record_list_page.dart": strings.Join([]string{
			"import '../models/task.dart';",
			"class RecordListPage {",
			"  const RecordListPage({required this.controller, required this.onCreateRecord, required this.onOpenTaskDetail});",
			"  final dynamic controller;",
			"  final Future<void> Function() onCreateRecord;",
			"  final Future<void> Function(Task task) onOpenTaskDetail;",
			"}",
		}, "\n") + "\n",
		"lib/views/record_detail_page.dart": strings.Join([]string{
			"import '../models/project.dart';",
			"class RecordDetailPage {",
			"  const RecordDetailPage({required this.task, required this.availableProjects, required this.currentProject, required this.onEdit});",
			"  final Object task;",
			"  final List<Project> availableProjects;",
			"  final Project? currentProject;",
			"  final Future<void> Function(Object task) onEdit;",
			"}",
		}, "\n") + "\n",
	})
	content := strings.Join([]string{
		"import 'package:flutter/material.dart';",
		"",
		"import 'controllers/record_list_controller.dart';",
		"import 'repositories/record_repository.dart';",
		"import 'views/record_list_page.dart';",
		"",
		"class TodoLiteApp extends StatelessWidget {",
		"  const TodoLiteApp({super.key, required this.repository});",
		"  final RecordRepository repository;",
		"  @override",
		"  Widget build(BuildContext context) {",
		"    return MaterialApp(",
		"      home: RecordListPage(",
		"        controller: RecordListController(repository: repository),",
		"        onOpenTaskDetail: (record) async {},",
		"      ),",
		"    );",
		"  }",
		"}",
	}, "\n") + "\n"

	normalized := normalizeBuilderRuntimeOpenLiteMainDetailCallback(workspacePath, content)
	for _, want := range []string{
		"final availableProjects = await repository.loadProjects();",
		"dynamic currentProject;",
		"for (final candidate in await repository.loadProjects()) {",
		"if (candidate.projectId == record.projectId) {",
		"availableProjects: availableProjects,",
		"currentProject: currentProject,",
	} {
		if !strings.Contains(normalized, want) {
			t.Fatalf("normalizeBuilderRuntimeOpenLiteMainDetailCallback() should wire type-aware repository-derived alias %q: %q", want, normalized)
		}
	}
	if strings.Contains(normalized, "onOpenTaskDetail: (record) async {},") {
		t.Fatalf("normalizeBuilderRuntimeOpenLiteMainDetailCallback() should replace placeholder callback when repository can derive type-aware detail aliases: %q", normalized)
	}
}

func TestNormalizeBuilderRuntimeOpenLiteMainDetailCallbackCanonicalizesControllerLookupArgs(t *testing.T) {
	workspacePath := createBuilderRuntimeInventoryRelationRichWorkspace(t, map[string]string{
		"lib/controllers/record_list_controller.dart": strings.Join([]string{
			"import '../repositories/record_repository.dart';",
			"class RecordListController {",
			"  RecordListController({required RecordRepository repository});",
			"  Future<void> refresh() async {}",
			"  Future<void> updateRecord(dynamic record) async {}",
			"  dynamic warehouseFor(String warehouseId) => null;",
			"}",
		}, "\n") + "\n",
		"lib/views/record_list_page.dart": strings.Join([]string{
			"import '../controllers/record_list_controller.dart';",
			"import '../models/inventory_sheet.dart';",
			"class RecordListPage {",
			"  const RecordListPage({required this.controller, required this.onOpenRecordDetail});",
			"  final RecordListController controller;",
			"  final Future<void> Function(InventorySheet record) onOpenRecordDetail;",
			"}",
		}, "\n") + "\n",
		"lib/views/record_form_page.dart": strings.Join([]string{
			"import '../repositories/record_repository.dart';",
			"class RecordFormPage {",
			"  const RecordFormPage({required this.repository, this.initialSheet});",
			"  final RecordRepository repository;",
			"  final Object? initialSheet;",
			"}",
		}, "\n") + "\n",
		"lib/views/record_detail_page.dart": strings.Join([]string{
			"class RecordDetailPage {",
			"  const RecordDetailPage({required this.sheet, required this.warehouse, required this.onEdit});",
			"  final Object sheet;",
			"  final Object? warehouse;",
			"  final Future<void> Function(Object sheet) onEdit;",
			"}",
		}, "\n") + "\n",
		"lib/repositories/record_repository.dart": "abstract class RecordRepository {}\n",
	})
	content := strings.Join([]string{
		"import 'package:flutter/material.dart';",
		"",
		"import 'controllers/record_list_controller.dart';",
		"import 'repositories/record_repository.dart';",
		"import 'views/record_list_page.dart';",
		"",
		"class InventoryLiteApp extends StatelessWidget {",
		"  const InventoryLiteApp({super.key, required this.repository});",
		"  final RecordRepository repository;",
		"  @override",
		"  Widget build(BuildContext context) {",
		"    return MaterialApp(",
		"      home: RecordListPage(",
		"        controller: RecordListController(repository: repository),",
		"        onOpenRecordDetail: (record) async {},",
		"      ),",
		"    );",
		"  }",
		"}",
	}, "\n") + "\n"

	normalized := normalizeBuilderRuntimeOpenLiteMainDetailCallback(workspacePath, content)
	for _, want := range []string{
		"import 'views/record_detail_page.dart';",
		"warehouse: listController.warehouseFor(record.warehouseId),",
		"onEdit: (updatedRecord) async {",
	} {
		if !strings.Contains(normalized, want) {
			t.Fatalf("normalizeBuilderRuntimeOpenLiteMainDetailCallback() should wire controller lookup detail argument %q: %q", want, normalized)
		}
	}
	if strings.Contains(normalized, "final warehouse =") {
		t.Fatalf("normalizeBuilderRuntimeOpenLiteMainDetailCallback() should prefer controller lookup over repository-derived warehouse prelude: %q", normalized)
	}
	if strings.Contains(normalized, "onOpenRecordDetail: (record) async {},") {
		t.Fatalf("normalizeBuilderRuntimeOpenLiteMainDetailCallback() should replace placeholder callback when controller lookup can derive warehouse: %q", normalized)
	}
}

func TestNormalizeBuilderRuntimeOpenLiteMainDetailCallbackCanonicalizesTypeAwareControllerLookupAlias(t *testing.T) {
	workspacePath := createBuilderRuntimeInventoryRelationRichWorkspace(t, map[string]string{
		"lib/controllers/record_list_controller.dart": strings.Join([]string{
			"import '../repositories/record_repository.dart';",
			"class RecordListController {",
			"  RecordListController({required RecordRepository repository});",
			"  Future<void> refresh() async {}",
			"  Future<void> updateRecord(dynamic record) async {}",
			"  dynamic warehouseFor(String warehouseId) => null;",
			"}",
		}, "\n") + "\n",
		"lib/views/record_list_page.dart": strings.Join([]string{
			"import '../controllers/record_list_controller.dart';",
			"import '../models/inventory_sheet.dart';",
			"class RecordListPage {",
			"  const RecordListPage({required this.controller, required this.onOpenRecordDetail});",
			"  final RecordListController controller;",
			"  final Future<void> Function(InventorySheet record) onOpenRecordDetail;",
			"}",
		}, "\n") + "\n",
		"lib/views/record_form_page.dart": strings.Join([]string{
			"import '../repositories/record_repository.dart';",
			"class RecordFormPage {",
			"  const RecordFormPage({required this.repository, this.initialSheet});",
			"  final RecordRepository repository;",
			"  final Object? initialSheet;",
			"}",
		}, "\n") + "\n",
		"lib/views/record_detail_page.dart": strings.Join([]string{
			"import '../models/warehouse.dart';",
			"class RecordDetailPage {",
			"  const RecordDetailPage({required this.sheet, required this.currentWarehouse, required this.onEdit});",
			"  final Object sheet;",
			"  final Warehouse? currentWarehouse;",
			"  final Future<void> Function(Object sheet) onEdit;",
			"}",
		}, "\n") + "\n",
		"lib/repositories/record_repository.dart": "abstract class RecordRepository {}\n",
	})
	content := strings.Join([]string{
		"import 'package:flutter/material.dart';",
		"",
		"import 'controllers/record_list_controller.dart';",
		"import 'repositories/record_repository.dart';",
		"import 'views/record_list_page.dart';",
		"",
		"class InventoryLiteApp extends StatelessWidget {",
		"  const InventoryLiteApp({super.key, required this.repository});",
		"  final RecordRepository repository;",
		"  @override",
		"  Widget build(BuildContext context) {",
		"    return MaterialApp(",
		"      home: RecordListPage(",
		"        controller: RecordListController(repository: repository),",
		"        onOpenRecordDetail: (record) async {},",
		"      ),",
		"    );",
		"  }",
		"}",
	}, "\n") + "\n"

	normalized := normalizeBuilderRuntimeOpenLiteMainDetailCallback(workspacePath, content)
	for _, want := range []string{
		"import 'views/record_detail_page.dart';",
		"currentWarehouse: listController.warehouseFor(record.warehouseId),",
		"onEdit: (updatedRecord) async {",
	} {
		if !strings.Contains(normalized, want) {
			t.Fatalf("normalizeBuilderRuntimeOpenLiteMainDetailCallback() should wire type-aware controller lookup alias %q: %q", want, normalized)
		}
	}
	if strings.Contains(normalized, "dynamic currentWarehouse;") {
		t.Fatalf("normalizeBuilderRuntimeOpenLiteMainDetailCallback() should prefer controller lookup alias without repository-style prelude: %q", normalized)
	}
	if strings.Contains(normalized, "onOpenRecordDetail: (record) async {},") {
		t.Fatalf("normalizeBuilderRuntimeOpenLiteMainDetailCallback() should replace placeholder callback when controller can derive a type-aware alias: %q", normalized)
	}
}

func TestNormalizeBuilderRuntimeOpenLiteMainDetailCallbackPreservesPlaceholderWhenCustomDetailNeedsExtraData(t *testing.T) {
	workspacePath := t.TempDir()
	writeBuilderRuntimeWorkspaceFiles(t, workspacePath, map[string]string{
		"lib/views/task_detail_page.dart":                 "class TaskDetailPage { const TaskDetailPage({super.key, required this.task, required this.projects, required this.onEdit}); final Object task; final List<Object> projects; final Future<void> Function(Object task) onEdit; }\n",
		"lib/views/task_form_page.dart":                   "class TaskFormPage { const TaskFormPage({super.key, required this.taskRepository, this.initialTask}); final TaskRepository taskRepository; final Object? initialTask; }\n",
		"lib/controllers/task_collection_controller.dart": "import '../repositories/task_repository.dart';\nclass TaskCollectionController {\n  TaskCollectionController({required TaskRepository taskRepository});\n  Future<void> refresh() async {}\n  Future<void> updateRecord(dynamic record) async {}\n}\n",
		"lib/repositories/task_repository.dart":           "abstract class TaskRepository {}\n",
	})
	content := strings.Join([]string{
		"import 'package:flutter/material.dart';",
		"",
		"import 'controllers/task_collection_controller.dart';",
		"import 'repositories/task_repository.dart';",
		"import 'views/record_list_page.dart';",
		"",
		"class TodoLiteApp extends StatelessWidget {",
		"  const TodoLiteApp({super.key, required this.repository});",
		"  final TaskRepository repository;",
		"  @override",
		"  Widget build(BuildContext context) {",
		"    return MaterialApp(",
		"      home: RecordListPage(",
		"        controller: TaskCollectionController(taskRepository: repository),",
		"        onOpenRecordDetail: (record) async {},",
		"      ),",
		"    );",
		"  }",
		"}",
	}, "\n") + "\n"

	normalized := normalizeBuilderRuntimeOpenLiteMainDetailCallback(workspacePath, content)
	if normalized != content {
		t.Fatalf("normalizeBuilderRuntimeOpenLiteMainDetailCallback() should preserve placeholder when custom detail view still needs extra constructor data: %q", normalized)
	}
}

func TestNormalizeBuilderRuntimeOpenLiteMainContentRepairsStatefulWrapperAnalyzeDrift(t *testing.T) {
	workspacePath := t.TempDir()
	content := strings.Join([]string{
		"import 'package:flutter/material.dart';",
		"",
		"import 'controllers/home_controller.dart';",
		"import 'controllers/record_list_controller.dart';",
		"import 'models/record.dart';",
		"import 'repositories/record_repository.dart';",
		"import 'views/record_detail_page.dart';",
		"import 'views/record_form_page.dart';",
		"import 'views/record_list_page.dart';",
		"",
		"Future<void> main() async {",
		"  final repository = HiveRecordRepository();",
		"  runApp(WeightTrackerApp(repository: repository));",
		"}",
		"",
		"class WeightTrackerApp extends StatelessWidget {",
		"  WeightTrackerApp({super.key, required this.repository});",
		"",
		"  final RecordRepository repository;",
		"",
		"  final GlobalKey<NavigatorState> _navigatorKey = GlobalKey<NavigatorState>();",
		"",
		"  @override",
		"  Widget build(BuildContext context) {",
		"    return MaterialApp(home: HomePageWrapper(repository: repository, navigatorKey: _navigatorKey));",
		"  }",
		"}",
		"",
		"class HomePageWrapper extends StatefulWidget {",
		"  const HomePageWrapper({super.key, required this.repository, required this.navigatorKey});",
		"",
		"  final RecordRepository repository;",
		"  final GlobalKey<NavigatorState> navigatorKey;",
		"",
		"  @override",
		"  _HomePageWrapperState createState() => _HomePageWrapperState();",
		"}",
		"",
		"class _HomePageWrapperState extends State<HomePageWrapper> {",
		"  late final HomeController _homeController;",
		"",
		"  @override",
		"  Widget build(BuildContext context) {",
		"    return RecordListPage(",
		"      controller: RecordListController(repository: widget.repository),",
		"      onCreateRecord: _navigateToFormPage,",
		"      onOpenRecordDetail: _navigateToDetailPage,",
		"    );",
		"  }",
		"",
		"  Future<void> _navigateToFormPage({WeightRecord? initialRecord}) async {}",
		"",
		"  Future<void> _navigateToDetailPage(WeightRecord record) async {",
		"    await widget.navigatorKey.currentState!.push<WeightRecord>(",
		"      MaterialPageRoute(",
		"        builder: (context) => RecordDetailPage(",
		"          record: record,",
		"          onEdit: (updatedRecord) async => updatedRecord,",
		"          onDelete: (deletedRecord) async {",
		"            await _homeController.deleteRecord(deletedRecord.recordId);",
		"            return null;",
		"          },",
		"        ),",
		"      ),",
		"    );",
		"  }",
		"}",
	}, "\n") + "\n"

	normalized := normalizeBuilderRuntimeOpenLiteMainContent(workspacePath, false, content)

	if !strings.Contains(normalized, "State<HomePageWrapper> createState() => _HomePageWrapperState();") {
		t.Fatalf("normalizeBuilderRuntimeOpenLiteMainContent() should repair private createState return type drift: %q", normalized)
	}
	if strings.Contains(normalized, "return null;") {
		t.Fatalf("normalizeBuilderRuntimeOpenLiteMainContent() should remove null return from Future<void> onDelete callback: %q", normalized)
	}
	if !strings.Contains(normalized, "await _homeController.deleteRecord(deletedRecord.recordId);") {
		t.Fatalf("normalizeBuilderRuntimeOpenLiteMainContent() should preserve onDelete body while removing null return: %q", normalized)
	}
	if !strings.Contains(normalized, "onDelete: (deletedRecord) async {") {
		t.Fatalf("normalizeBuilderRuntimeOpenLiteMainContent() should preserve onDelete callback shape: %q", normalized)
	}
}

func TestNormalizeBuilderRuntimeOpenLiteMainTodoStatefulDriftPrunesCustomFormControllerFieldLifecycle(t *testing.T) {
	content := strings.Join([]string{
		"import 'controllers/task_form_controller.dart';",
		"",
		"class HomePageWrapper extends StatefulWidget {",
		"  const HomePageWrapper({super.key, required this.taskRepository});",
		"  final Object taskRepository;",
		"  @override",
		"  _HomePageWrapperState createState() => _HomePageWrapperState();",
		"}",
		"",
		"class _HomePageWrapperState extends State<HomePageWrapper> {",
		"  late final TaskFormController formController;",
		"",
		"  @override",
		"  void initState() {",
		"    super.initState();",
		"    formController = TaskFormController(taskRepository: widget.taskRepository);",
		"  }",
		"",
		"  @override",
		"  void dispose() {",
		"    formController.dispose();",
		"    super.dispose();",
		"  }",
		"}",
	}, "\n") + "\n"

	normalized := normalizeBuilderRuntimeOpenLiteMainTodoStatefulDrift(content)

	for _, forbidden := range []string{"late final TaskFormController formController;", "formController = TaskFormController(taskRepository: widget.taskRepository);", "formController.dispose();"} {
		if strings.Contains(normalized, forbidden) {
			t.Fatalf("normalizeBuilderRuntimeOpenLiteMainTodoStatefulDrift() should prune stale custom form controller lifecycle %q: %q", forbidden, normalized)
		}
	}
	if strings.Contains(normalized, "import 'controllers/task_form_controller.dart';") {
		t.Fatalf("normalizeBuilderRuntimeOpenLiteMainTodoStatefulDrift() should drop unused custom form controller import after pruning lifecycle: %q", normalized)
	}
	if !strings.Contains(normalized, "super.dispose();") {
		t.Fatalf("normalizeBuilderRuntimeOpenLiteMainTodoStatefulDrift() should preserve surrounding dispose body after pruning custom form controller lifecycle: %q", normalized)
	}
}

func TestNormalizeBuilderRuntimeOpenLiteMainTodoStatefulDriftPrunesUnusedCustomListControllerDeclaration(t *testing.T) {
	content := strings.Join([]string{
		"import 'controllers/task_collection_controller.dart';",
		"",
		"class TodoLiteApp extends StatelessWidget {",
		"  const TodoLiteApp({super.key, required this.repository});",
		"  final Object repository;",
		"  @override",
		"  Widget build(BuildContext context) {",
		"    final listController = TaskCollectionController(taskRepository: repository);",
		"    return const Placeholder();",
		"  }",
		"}",
	}, "\n") + "\n"

	normalized := normalizeBuilderRuntimeOpenLiteMainTodoStatefulDrift(content)

	if strings.Contains(normalized, "final listController = TaskCollectionController(taskRepository: repository);") {
		t.Fatalf("normalizeBuilderRuntimeOpenLiteMainTodoStatefulDrift() should prune unused custom listController declaration: %q", normalized)
	}
	if strings.Contains(normalized, "import 'controllers/task_collection_controller.dart';") {
		t.Fatalf("normalizeBuilderRuntimeOpenLiteMainTodoStatefulDrift() should drop unused custom list controller import after pruning declaration: %q", normalized)
	}
	if !strings.Contains(normalized, "return const Placeholder();") {
		t.Fatalf("normalizeBuilderRuntimeOpenLiteMainTodoStatefulDrift() should preserve surrounding build content after pruning unused custom listController declaration: %q", normalized)
	}
}

func TestNormalizeBuilderRuntimeOpenLiteMainContentRepairsMissingHomePageCallbacks(t *testing.T) {
	workspacePath := t.TempDir()
	homePath := filepath.Join(workspacePath, "lib", "views", "home_page.dart")
	if err := os.MkdirAll(filepath.Dir(homePath), 0o755); err != nil {
		t.Fatalf("MkdirAll(homePath) error = %v", err)
	}
	if err := os.WriteFile(homePath, []byte(strings.Join([]string{
		"import 'package:flutter/material.dart';",
		"",
		"class HomePage extends StatelessWidget {",
		"  const HomePage({",
		"    super.key,",
		"    required this.controller,",
		"    required this.onCreateRecord,",
		"    required this.onViewAllRecords,",
		"    required this.onOpenRecordDetail,",
		"  });",
		"",
		"  final Object controller;",
		"  final Future<void> Function() onCreateRecord;",
		"  final Future<void> Function() onViewAllRecords;",
		"  final Future<void> Function(Object record) onOpenRecordDetail;",
		"",
		"  @override",
		"  Widget build(BuildContext context) => const Placeholder();",
		"}",
	}, "\n")+"\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(home_page.dart) error = %v", err)
	}
	content := strings.Join([]string{
		"import 'package:flutter/material.dart';",
		"",
		"import 'views/home_page.dart';",
		"",
		"Future<void> main() async {",
		"  runApp(MyApp());",
		"}",
		"",
		"class MyApp extends StatelessWidget {",
		"  @override",
		"  Widget build(BuildContext context) {",
		"    return MaterialApp(",
		"      home: HomePage(",
		"        controller: Object(),",
		"      ),",
		"    );",
		"  }",
		"}",
	}, "\n") + "\n"

	normalized := normalizeBuilderRuntimeOpenLiteMainContent(workspacePath, false, content)

	for _, want := range []string{"onCreateRecord: _noop", "onViewAllRecords: _noop", "onOpenRecordDetail: _noopRecord"} {
		if !strings.Contains(normalized, want) {
			t.Fatalf("normalizeBuilderRuntimeOpenLiteMainContent() should inject missing HomePage callback %q: %q", want, normalized)
		}
	}
	if !strings.Contains(normalized, "Future<void> _noop() async {}") {
		t.Fatalf("normalizeBuilderRuntimeOpenLiteMainContent() should add top-level noop callback helper: %q", normalized)
	}
	if !strings.Contains(normalized, "Future<void> _noopRecord(Object record) async {}") {
		t.Fatalf("normalizeBuilderRuntimeOpenLiteMainContent() should add top-level noop record callback helper: %q", normalized)
	}
	if strings.Count(normalized, "Future<void> _noop() async {}") != 1 {
		t.Fatalf("normalizeBuilderRuntimeOpenLiteMainContent() should insert noop helper exactly once: %q", normalized)
	}
}

func TestNormalizeBuilderRuntimeOpenLiteMainContentRepairsMissingCustomHomeDetailCallbackWithRecordNoop(t *testing.T) {
	workspacePath := t.TempDir()
	homePath := filepath.Join(workspacePath, "lib", "views", "home_page.dart")
	if err := os.MkdirAll(filepath.Dir(homePath), 0o755); err != nil {
		t.Fatalf("MkdirAll(homePath) error = %v", err)
	}
	if err := os.WriteFile(homePath, []byte(strings.Join([]string{
		"import 'package:flutter/material.dart';",
		"",
		"class HomePage extends StatelessWidget {",
		"  const HomePage({",
		"    super.key,",
		"    required this.controller,",
		"    required this.onCreateRecord,",
		"    required this.onViewAllRecords,",
		"    required this.onOpenTaskCardDetail,",
		"  });",
		"",
		"  final Object controller;",
		"  final Future<void> Function() onCreateRecord;",
		"  final Future<void> Function() onViewAllRecords;",
		"  final Future<void> Function(Object record) onOpenTaskCardDetail;",
		"",
		"  @override",
		"  Widget build(BuildContext context) => const Placeholder();",
		"}",
	}, "\n")+"\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(home_page.dart) error = %v", err)
	}
	content := strings.Join([]string{
		"import 'package:flutter/material.dart';",
		"",
		"import 'views/home_page.dart';",
		"",
		"Future<void> main() async {",
		"  runApp(MyApp());",
		"}",
		"",
		"class MyApp extends StatelessWidget {",
		"  @override",
		"  Widget build(BuildContext context) {",
		"    return MaterialApp(",
		"      home: HomePage(",
		"        controller: Object(),",
		"      ),",
		"    );",
		"  }",
		"}",
	}, "\n") + "\n"

	normalized := normalizeBuilderRuntimeOpenLiteMainContent(workspacePath, false, content)

	if !strings.Contains(normalized, "onOpenTaskCardDetail: _noopRecord") {
		t.Fatalf("normalizeBuilderRuntimeOpenLiteMainContent() should inject custom home detail callback with record noop helper: %q", normalized)
	}
	if strings.Contains(normalized, "onOpenTaskCardDetail: _noop,") {
		t.Fatalf("normalizeBuilderRuntimeOpenLiteMainContent() should not downcast custom home detail callback to zero-arg noop: %q", normalized)
	}
	if !strings.Contains(normalized, "Future<void> _noopRecord(Object record) async {}") {
		t.Fatalf("normalizeBuilderRuntimeOpenLiteMainContent() should keep top-level noop record helper for custom home detail callback: %q", normalized)
	}
}

func TestNormalizeBuilderRuntimeOpenLiteMainContentReusesCustomHomeDetailHelperForMissingCallback(t *testing.T) {
	workspacePath := t.TempDir()
	homePath := filepath.Join(workspacePath, "lib", "views", "home_page.dart")
	if err := os.MkdirAll(filepath.Dir(homePath), 0o755); err != nil {
		t.Fatalf("MkdirAll(homePath) error = %v", err)
	}
	if err := os.WriteFile(homePath, []byte(strings.Join([]string{
		"import 'package:flutter/material.dart';",
		"",
		"class HomePage extends StatelessWidget {",
		"  const HomePage({",
		"    super.key,",
		"    required this.controller,",
		"    required this.onCreateRecord,",
		"    required this.onViewAllRecords,",
		"    required this.onOpenTaskCardDetail,",
		"  });",
		"",
		"  final Object controller;",
		"  final Future<void> Function() onCreateRecord;",
		"  final Future<void> Function() onViewAllRecords;",
		"  final Future<void> Function(Object record) onOpenTaskCardDetail;",
		"",
		"  @override",
		"  Widget build(BuildContext context) => const Placeholder();",
		"}",
	}, "\n")+"\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(home_page.dart) error = %v", err)
	}
	content := strings.Join([]string{
		"import 'package:flutter/material.dart';",
		"",
		"import 'views/home_page.dart';",
		"",
		"Future<void> _noop() async {}",
		"Future<void> _openTaskCardDetail(Object record) async {}",
		"",
		"Future<void> main() async {",
		"  runApp(MyApp());",
		"}",
		"",
		"class MyApp extends StatelessWidget {",
		"  @override",
		"  Widget build(BuildContext context) {",
		"    return MaterialApp(",
		"      home: HomePage(",
		"        controller: Object(),",
		"      ),",
		"    );",
		"  }",
		"}",
	}, "\n") + "\n"

	normalized := normalizeBuilderRuntimeOpenLiteMainContent(workspacePath, false, content)

	if !strings.Contains(normalized, "onOpenTaskCardDetail: _openTaskCardDetail") {
		t.Fatalf("normalizeBuilderRuntimeOpenLiteMainContent() should reuse existing custom home detail helper when filling missing callback: %q", normalized)
	}
	if strings.Contains(normalized, "onOpenTaskCardDetail: _noopRecord") {
		t.Fatalf("normalizeBuilderRuntimeOpenLiteMainContent() should not discard existing custom home detail helper in favor of noop record fallback: %q", normalized)
	}
}

func TestNormalizeBuilderRuntimeOpenLiteMainContentReusesCustomHomeCreateHelperForMissingCallback(t *testing.T) {
	workspacePath := createBuilderRuntimeCustomOverviewSurfaceWorkspace(t)
	content := strings.Join([]string{
		"import 'package:flutter/material.dart';",
		"",
		"import 'repositories/task_repository.dart';",
		"import 'views/task_overview_page.dart';",
		"",
		"Future<void> _noop() async {}",
		"Future<void> _openCreateTask() async {}",
		"",
		"void main() {",
		"  final repository = HiveTaskRepository();",
		"  runApp(TaskTrackerApp(repository: repository));",
		"}",
		"",
		"class TaskTrackerApp extends StatelessWidget {",
		"  const TaskTrackerApp({super.key, required this.repository});",
		"",
		"  final TaskRepository repository;",
		"",
		"  @override",
		"  Widget build(BuildContext context) {",
		"    return MaterialApp(",
		"      home: TaskOverviewPage(",
		"        controller: TaskOverviewController(taskRepository: repository),",
		"        onViewAllTasks: _noop,",
		"      ),",
		"    );",
		"  }",
		"}",
	}, "\n") + "\n"

	normalized := normalizeBuilderRuntimeOpenLiteMainContent(workspacePath, false, content)

	if !strings.Contains(normalized, "onCreateTask: _openCreateTask") {
		t.Fatalf("normalizeBuilderRuntimeOpenLiteMainContent() should reuse existing custom home create helper when filling missing callback: %q", normalized)
	}
	if strings.Contains(normalized, "onCreateTask: _noop") {
		t.Fatalf("normalizeBuilderRuntimeOpenLiteMainContent() should not discard existing custom home create helper in favor of noop fallback: %q", normalized)
	}
}

func TestNormalizeBuilderRuntimeOpenLiteMainContentReusesCustomHomeViewAllHelperForMissingCallback(t *testing.T) {
	workspacePath := createBuilderRuntimeCustomOverviewSurfaceWorkspace(t)
	content := strings.Join([]string{
		"import 'package:flutter/material.dart';",
		"",
		"import 'repositories/task_repository.dart';",
		"import 'views/task_overview_page.dart';",
		"",
		"Future<void> _noop() async {}",
		"Future<void> _openViewAllTasks() async {}",
		"",
		"void main() {",
		"  final repository = HiveTaskRepository();",
		"  runApp(TaskTrackerApp(repository: repository));",
		"}",
		"",
		"class TaskTrackerApp extends StatelessWidget {",
		"  const TaskTrackerApp({super.key, required this.repository});",
		"",
		"  final TaskRepository repository;",
		"",
		"  @override",
		"  Widget build(BuildContext context) {",
		"    return MaterialApp(",
		"      home: TaskOverviewPage(",
		"        controller: TaskOverviewController(taskRepository: repository),",
		"        onCreateTask: _noop,",
		"      ),",
		"    );",
		"  }",
		"}",
	}, "\n") + "\n"

	normalized := normalizeBuilderRuntimeOpenLiteMainContent(workspacePath, false, content)

	if !strings.Contains(normalized, "onViewAllTasks: _openViewAllTasks") {
		t.Fatalf("normalizeBuilderRuntimeOpenLiteMainContent() should reuse existing custom home view-all helper when filling missing callback: %q", normalized)
	}
	if strings.Contains(normalized, "onViewAllTasks: _noop") {
		t.Fatalf("normalizeBuilderRuntimeOpenLiteMainContent() should not discard existing custom home view-all helper in favor of noop fallback: %q", normalized)
	}
}

func TestNormalizeBuilderRuntimeOpenLiteMainContentReusesCustomHomeDetailNavigateHelperForMissingCallback(t *testing.T) {
	workspacePath := t.TempDir()
	homePath := filepath.Join(workspacePath, "lib", "views", "home_page.dart")
	if err := os.MkdirAll(filepath.Dir(homePath), 0o755); err != nil {
		t.Fatalf("MkdirAll(homePath) error = %v", err)
	}
	if err := os.WriteFile(homePath, []byte(strings.Join([]string{
		"import 'package:flutter/material.dart';",
		"",
		"class HomePage extends StatelessWidget {",
		"  const HomePage({",
		"    super.key,",
		"    required this.controller,",
		"    required this.onCreateRecord,",
		"    required this.onViewAllRecords,",
		"    required this.onOpenTaskCardDetail,",
		"  });",
		"",
		"  final Object controller;",
		"  final Future<void> Function() onCreateRecord;",
		"  final Future<void> Function() onViewAllRecords;",
		"  final Future<void> Function(Object record) onOpenTaskCardDetail;",
		"",
		"  @override",
		"  Widget build(BuildContext context) => const Placeholder();",
		"}",
	}, "\n")+"\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(home_page.dart) error = %v", err)
	}
	content := strings.Join([]string{
		"import 'package:flutter/material.dart';",
		"",
		"import 'views/home_page.dart';",
		"",
		"Future<void> _noop() async {}",
		"Future<void> _navigateToTaskCardDetail(Object record) async {}",
		"",
		"Future<void> main() async {",
		"  runApp(MyApp());",
		"}",
		"",
		"class MyApp extends StatelessWidget {",
		"  @override",
		"  Widget build(BuildContext context) {",
		"    return MaterialApp(",
		"      home: HomePage(",
		"        controller: Object(),",
		"      ),",
		"    );",
		"  }",
		"}",
	}, "\n") + "\n"

	normalized := normalizeBuilderRuntimeOpenLiteMainContent(workspacePath, false, content)

	if !strings.Contains(normalized, "onOpenTaskCardDetail: _navigateToTaskCardDetail") {
		t.Fatalf("normalizeBuilderRuntimeOpenLiteMainContent() should reuse existing custom home detail navigate helper when filling missing callback: %q", normalized)
	}
	if strings.Contains(normalized, "onOpenTaskCardDetail: _noopRecord") {
		t.Fatalf("normalizeBuilderRuntimeOpenLiteMainContent() should not discard existing custom home detail navigate helper in favor of noop record fallback: %q", normalized)
	}
}

func TestNormalizeBuilderRuntimeOpenLiteMainContentUsesCustomRepositoryFileName(t *testing.T) {
	workspacePath := t.TempDir()
	writeBuilderRuntimeWorkspaceFiles(t, workspacePath, map[string]string{
		"lib/repositories/task_repository.dart": strings.Join([]string{
			"abstract class TaskRepository { Future<void> init(); }",
			"class HiveTaskRepository extends TaskRepository { @override Future<void> init() async {} }",
		}, "\n") + "\n",
	})
	content := strings.Join([]string{
		"import 'package:flutter/material.dart';",
		"import 'package:hive_flutter/hive_flutter.dart';",
		"",
		"import 'repositories/record_repository.dart';",
		"",
		"Future<void> main() async {",
		"  WidgetsFlutterBinding.ensureInitialized();",
		"  await Hive.initFlutter();",
		"  runApp(const TaskApp());",
		"}",
		"",
		"class TaskApp extends StatelessWidget {",
		"  const TaskApp({super.key});",
		"",
		"  @override",
		"  Widget build(BuildContext context) => const MaterialApp(home: Placeholder());",
		"}",
	}, "\n") + "\n"

	normalized := normalizeBuilderRuntimeOpenLiteMainContent(workspacePath, false, content)
	for _, want := range []string{
		"import 'repositories/task_repository.dart';",
		"final repository = HiveTaskRepository();",
		"runApp(TaskApp(repository: repository));",
		"const TaskApp({super.key, required this.repository});",
		"final TaskRepository repository;",
	} {
		if !strings.Contains(normalized, want) {
			t.Fatalf("normalizeBuilderRuntimeOpenLiteMainContent() should preserve custom repository file/type %q: %q", want, normalized)
		}
	}
	if strings.Contains(normalized, "record_repository.dart") {
		t.Fatalf("normalizeBuilderRuntimeOpenLiteMainContent() should remove legacy record_repository import when custom repository file exists: %q", normalized)
	}
}

func TestNormalizeBuilderRuntimeWidgetTestContentPrefersInMemoryRecordRepository(t *testing.T) {
	workspacePath := t.TempDir()
	writeBuilderRuntimeWorkspaceFiles(t, workspacePath, map[string]string{
		"lib/main.dart": strings.Join([]string{
			"import 'package:flutter/material.dart';",
			"",
			"import 'repositories/record_repository.dart';",
			"",
			"void main() {",
			"  runApp(TodoLiteApp(repository: InMemoryRecordRepository()));",
			"}",
			"",
			"class TodoLiteApp extends StatelessWidget {",
			"  const TodoLiteApp({super.key, required this.repository});",
			"",
			"  final RecordRepository repository;",
			"",
			"  @override",
			"  Widget build(BuildContext context) => const Placeholder();",
			"}",
		}, "\n") + "\n",
		"lib/models/record.dart":                      "enum RecordStatus { inbox, inProgress, done }\nclass AppRecord {}\n",
		"lib/controllers/record_form_controller.dart": "import 'package:flutter/material.dart';\nimport '../models/record.dart';\nclass RecordFormController extends ChangeNotifier {\n  final TextEditingController titleController = TextEditingController();\n  final TextEditingController noteController = TextEditingController();\n  final TextEditingController categoryController = TextEditingController();\n  RecordStatus get selectedStatus => RecordStatus.inbox;\n}\n",
		"lib/repositories/record_repository.dart":     "abstract class RecordRepository { Future<void> init(); }\nclass HiveRecordRepository extends RecordRepository { @override Future<void> init() async {} }\nclass InMemoryRecordRepository extends RecordRepository { @override Future<void> init() async {} }\n",
	})

	normalized := normalizeBuilderRuntimeWidgetTestContent(workspacePath, true, "void main() {}\n")
	if !strings.Contains(normalized, "final repository = InMemoryRecordRepository();") {
		t.Fatalf("normalizeBuilderRuntimeWidgetTestContent() should prefer in-memory repository for widget tests when available: %q", normalized)
	}
	if strings.Contains(normalized, "final repository = HiveRecordRepository();") {
		t.Fatalf("normalizeBuilderRuntimeWidgetTestContent() should not keep Hive repository in widget tests when in-memory helper exists: %q", normalized)
	}
	if !strings.Contains(normalized, "await tester.pumpAndSettle();") {
		t.Fatalf("normalizeBuilderRuntimeWidgetTestContent() should settle UI transitions instead of relying on fixed-duration pumps: %q", normalized)
	}
	if !strings.Contains(normalized, "await tester.pumpWidget(TodoLiteApp(repository: repository));") {
		t.Fatalf("normalizeBuilderRuntimeWidgetTestContent() should keep the current app widget constructor synchronized with lib/main.dart: %q", normalized)
	}
}

func TestNormalizeBuilderRuntimeWidgetTestContentPrefersInMemoryCustomRepository(t *testing.T) {
	workspacePath := t.TempDir()
	writeBuilderRuntimeWorkspaceFiles(t, workspacePath, map[string]string{
		"lib/main.dart": strings.Join([]string{
			"import 'package:flutter/material.dart';",
			"",
			"import 'repositories/task_repository.dart';",
			"",
			"void main() {",
			"  runApp(TaskLiteApp(repository: InMemoryTaskRepository()));",
			"}",
			"",
			"class TaskLiteApp extends StatelessWidget {",
			"  const TaskLiteApp({super.key, required this.repository});",
			"",
			"  final TaskRepository repository;",
			"",
			"  @override",
			"  Widget build(BuildContext context) => const Placeholder();",
			"}",
		}, "\n") + "\n",
		"lib/models/record.dart":                      "enum RecordStatus { inbox, inProgress, done }\nclass AppRecord {}\n",
		"lib/controllers/record_form_controller.dart": "import 'package:flutter/material.dart';\nimport '../models/record.dart';\nclass RecordFormController extends ChangeNotifier {\n  final TextEditingController titleController = TextEditingController();\n  final TextEditingController noteController = TextEditingController();\n  final TextEditingController categoryController = TextEditingController();\n  RecordStatus get selectedStatus => RecordStatus.inbox;\n}\n",
		"lib/repositories/task_repository.dart": strings.Join([]string{
			"abstract class TaskRepository { Future<void> init(); }",
			"class HiveTaskRepository extends TaskRepository { @override Future<void> init() async {} }",
			"class InMemoryTaskRepository extends TaskRepository { @override Future<void> init() async {} }",
		}, "\n") + "\n",
	})

	normalized := normalizeBuilderRuntimeWidgetTestContent(workspacePath, true, "void main() {}\n")
	if !strings.Contains(normalized, "final repository = InMemoryTaskRepository();") {
		t.Fatalf("normalizeBuilderRuntimeWidgetTestContent() should prefer in-memory custom repository for widget tests when available: %q", normalized)
	}
	if strings.Contains(normalized, "final repository = HiveTaskRepository();") {
		t.Fatalf("normalizeBuilderRuntimeWidgetTestContent() should not keep Hive custom repository in widget tests when in-memory helper exists: %q", normalized)
	}
	if !strings.Contains(normalized, "import 'package:flutter_open_lite/repositories/task_repository.dart';") {
		t.Fatalf("normalizeBuilderRuntimeWidgetTestContent() should import the discovered custom repository path in widget tests: %q", normalized)
	}
	if strings.Contains(normalized, "import 'package:flutter_open_lite/repositories/record_repository.dart';") {
		t.Fatalf("normalizeBuilderRuntimeWidgetTestContent() should not keep legacy record repository import when custom repository path exists: %q", normalized)
	}
	if !strings.Contains(normalized, "await tester.pumpWidget(TaskLiteApp(repository: repository));") {
		t.Fatalf("normalizeBuilderRuntimeWidgetTestContent() should keep custom app widget constructor synchronized with lib/main.dart: %q", normalized)
	}
}

func TestNormalizeBuilderRuntimeWidgetTestContentSkipsStatusFilterWhenRecordModelHasNoStatus(t *testing.T) {
	workspacePath := t.TempDir()
	writeBuilderRuntimeWorkspaceFiles(t, workspacePath, map[string]string{
		"lib/main.dart": strings.Join([]string{
			"import 'package:flutter/material.dart';",
			"",
			"import 'repositories/task_repository.dart';",
			"",
			"void main() {",
			"  runApp(TaskLiteApp(repository: InMemoryTaskRepository()));",
			"}",
			"",
			"class TaskLiteApp extends StatelessWidget {",
			"  const TaskLiteApp({super.key, required this.repository});",
			"",
			"  final TaskRepository repository;",
			"",
			"  @override",
			"  Widget build(BuildContext context) => const Placeholder();",
			"}",
		}, "\n") + "\n",
		"lib/models/record.dart": strings.Join([]string{
			"class TodoItem {",
			"  const TodoItem({required this.title, required this.category, this.note = ''});",
			"  final String title;",
			"  final String category;",
			"  final String note;",
			"}",
		}, "\n") + "\n",
		"lib/controllers/task_form_controller.dart": strings.Join([]string{
			"import 'package:flutter/material.dart';",
			"class TaskFormController extends ChangeNotifier {",
			"  final TextEditingController titleController = TextEditingController();",
			"  final TextEditingController noteController = TextEditingController();",
			"  final TextEditingController categoryController = TextEditingController();",
			"}",
		}, "\n") + "\n",
		"lib/repositories/task_repository.dart": strings.Join([]string{
			"abstract class TaskRepository { Future<void> init(); }",
			"class HiveTaskRepository extends TaskRepository { @override Future<void> init() async {} }",
			"class InMemoryTaskRepository extends TaskRepository { @override Future<void> init() async {} }",
		}, "\n") + "\n",
	})

	normalized := normalizeBuilderRuntimeWidgetTestContent(workspacePath, true, "void main() {}\n")
	if !strings.Contains(normalized, "await tester.pumpWidget(TaskLiteApp(repository: repository));") {
		t.Fatalf("normalizeBuilderRuntimeWidgetTestContent() should still canonicalize widget tests for custom mutation controller paths without status: %q", normalized)
	}
	if strings.Contains(normalized, "doneFilterLabel") {
		t.Fatalf("normalizeBuilderRuntimeWidgetTestContent() should not inject status filter assertions when the current record model has no status field: %q", normalized)
	}
	if !strings.Contains(normalized, "await tester.tap(find.text(openLiteCopy.editSubmitLabel));") {
		t.Fatalf("normalizeBuilderRuntimeWidgetTestContent() should keep the edit submit flow when status filtering is absent: %q", normalized)
	}
}

func TestNormalizeBuilderRuntimeWidgetTestContentSkipsEditFlowWhenMutationViewHasNoInitialParam(t *testing.T) {
	workspacePath := t.TempDir()
	writeBuilderRuntimeWorkspaceFiles(t, workspacePath, map[string]string{
		"lib/main.dart": strings.Join([]string{
			"import 'package:flutter/material.dart';",
			"",
			"import 'repositories/task_repository.dart';",
			"",
			"void main() {",
			"  runApp(TaskLiteApp(repository: InMemoryTaskRepository()));",
			"}",
			"",
			"class TaskLiteApp extends StatelessWidget {",
			"  const TaskLiteApp({super.key, required this.repository});",
			"",
			"  final TaskRepository repository;",
			"",
			"  @override",
			"  Widget build(BuildContext context) => const Placeholder();",
			"}",
		}, "\n") + "\n",
		"lib/models/record.dart": strings.Join([]string{
			"class TodoItem {",
			"  const TodoItem({required this.title, required this.category, this.note = ''});",
			"  final String title;",
			"  final String category;",
			"  final String note;",
			"}",
		}, "\n") + "\n",
		"lib/views/task_form_page.dart": strings.Join([]string{
			"class TaskFormPage {",
			"  const TaskFormPage({super.key, required this.taskRepository});",
			"  final TaskRepository taskRepository;",
			"}",
		}, "\n") + "\n",
		"lib/controllers/task_form_controller.dart": strings.Join([]string{
			"import 'package:flutter/material.dart';",
			"class TaskFormController extends ChangeNotifier {",
			"  final TextEditingController titleController = TextEditingController();",
			"  final TextEditingController noteController = TextEditingController();",
			"  final TextEditingController categoryController = TextEditingController();",
			"}",
		}, "\n") + "\n",
		"lib/repositories/task_repository.dart": strings.Join([]string{
			"abstract class TaskRepository { Future<void> init(); }",
			"class HiveTaskRepository extends TaskRepository { @override Future<void> init() async {} }",
			"class InMemoryTaskRepository extends TaskRepository { @override Future<void> init() async {} }",
		}, "\n") + "\n",
	})

	normalized := normalizeBuilderRuntimeWidgetTestContent(workspacePath, true, "void main() {}\n")
	if strings.Contains(normalized, "openLiteCopy.editPageTitle") {
		t.Fatalf("normalizeBuilderRuntimeWidgetTestContent() should not inject edit page assertions when the mutation view lacks an initial* param: %q", normalized)
	}
	if strings.Contains(normalized, "openLiteCopy.editSubmitLabel") {
		t.Fatalf("normalizeBuilderRuntimeWidgetTestContent() should not inject edit submit flow when the mutation view lacks an initial* param: %q", normalized)
	}
	if strings.Contains(normalized, "find.text('测试任务').first") {
		t.Fatalf("normalizeBuilderRuntimeWidgetTestContent() should not tap a created record for edit when the mutation view has no edit capability: %q", normalized)
	}
	if !strings.Contains(normalized, "expect(find.text('测试任务'), findsOneWidget);") {
		t.Fatalf("normalizeBuilderRuntimeWidgetTestContent() should keep create/read assertions when edit capability is absent: %q", normalized)
	}
}

func TestNormalizeBuilderRuntimeWidgetTestContentSkipsNoteFieldWhenMutationControllerLacksNoteController(t *testing.T) {
	workspacePath := t.TempDir()
	writeBuilderRuntimeWorkspaceFiles(t, workspacePath, map[string]string{
		"lib/main.dart": strings.Join([]string{
			"import 'package:flutter/material.dart';",
			"",
			"import 'repositories/task_repository.dart';",
			"",
			"void main() {",
			"  runApp(TaskLiteApp(repository: InMemoryTaskRepository()));",
			"}",
			"",
			"class TaskLiteApp extends StatelessWidget {",
			"  const TaskLiteApp({super.key, required this.repository});",
			"",
			"  final TaskRepository repository;",
			"",
			"  @override",
			"  Widget build(BuildContext context) => const Placeholder();",
			"}",
		}, "\n") + "\n",
		"lib/models/record.dart": strings.Join([]string{
			"class TodoItem {",
			"  const TodoItem({required this.title, required this.category});",
			"  final String title;",
			"  final String category;",
			"}",
		}, "\n") + "\n",
		"lib/controllers/task_form_controller.dart": strings.Join([]string{
			"import 'package:flutter/material.dart';",
			"class TaskFormController extends ChangeNotifier {",
			"  final TextEditingController titleController = TextEditingController();",
			"  final TextEditingController categoryController = TextEditingController();",
			"}",
		}, "\n") + "\n",
		"lib/repositories/task_repository.dart": strings.Join([]string{
			"abstract class TaskRepository { Future<void> init(); }",
			"class HiveTaskRepository extends TaskRepository { @override Future<void> init() async {} }",
			"class InMemoryTaskRepository extends TaskRepository { @override Future<void> init() async {} }",
		}, "\n") + "\n",
	})

	normalized := normalizeBuilderRuntimeWidgetTestContent(workspacePath, true, "void main() {}\n")
	if !strings.Contains(normalized, "await tester.pumpWidget(TaskLiteApp(repository: repository));") {
		t.Fatalf("normalizeBuilderRuntimeWidgetTestContent() should still canonicalize widget tests when the mutation controller keeps title-only editing: %q", normalized)
	}
	if strings.Contains(normalized, "note-field") {
		t.Fatalf("normalizeBuilderRuntimeWidgetTestContent() should not inject note-field edits when the mutation controller lacks noteController: %q", normalized)
	}
	if !strings.Contains(normalized, "await tester.tap(find.text(openLiteCopy.editSubmitLabel));") {
		t.Fatalf("normalizeBuilderRuntimeWidgetTestContent() should keep edit submit flow when noteController is absent but title editing remains supported: %q", normalized)
	}
}

func TestNormalizeBuilderRuntimeWidgetTestContentDegradesToCreateOnlySmokeWhenMutationControllerLacksTitleController(t *testing.T) {
	workspacePath := t.TempDir()
	writeBuilderRuntimeWorkspaceFiles(t, workspacePath, map[string]string{
		"lib/main.dart": strings.Join([]string{
			"import 'package:flutter/material.dart';",
			"",
			"import 'repositories/task_repository.dart';",
			"",
			"void main() {",
			"  runApp(TaskLiteApp(repository: InMemoryTaskRepository()));",
			"}",
			"",
			"class TaskLiteApp extends StatelessWidget {",
			"  const TaskLiteApp({super.key, required this.repository});",
			"",
			"  final TaskRepository repository;",
			"",
			"  @override",
			"  Widget build(BuildContext context) => const Placeholder();",
			"}",
		}, "\n") + "\n",
		"lib/models/record.dart": strings.Join([]string{
			"class WeightEntry {",
			"  const WeightEntry({required this.weight, this.note = ''});",
			"  final double weight;",
			"  final String note;",
			"}",
		}, "\n") + "\n",
		"lib/controllers/task_form_controller.dart": strings.Join([]string{
			"import 'package:flutter/material.dart';",
			"class TaskFormController extends ChangeNotifier {",
			"  final TextEditingController weightController = TextEditingController();",
			"  final TextEditingController noteController = TextEditingController();",
			"}",
		}, "\n") + "\n",
		"lib/repositories/task_repository.dart": strings.Join([]string{
			"abstract class TaskRepository { Future<void> init(); }",
			"class HiveTaskRepository extends TaskRepository { @override Future<void> init() async {} }",
			"class InMemoryTaskRepository extends TaskRepository { @override Future<void> init() async {} }",
		}, "\n") + "\n",
	})

	normalized := normalizeBuilderRuntimeWidgetTestContent(workspacePath, true, "void main() {}\n")
	for _, want := range []string{
		"await tester.pumpWidget(TaskLiteApp(repository: repository));",
		"await tester.tap(find.text(openLiteCopy.createPrimaryActionLabel));",
		"expect(find.text(openLiteCopy.createPageTitle), findsOneWidget);",
		"expect(find.text(openLiteCopy.createSubmitLabel), findsOneWidget);",
	} {
		if !strings.Contains(normalized, want) {
			t.Fatalf("normalizeBuilderRuntimeWidgetTestContent() missing title-less create-only smoke marker %q: %q", want, normalized)
		}
	}
	for _, forbidden := range []string{"title-field", "测试任务", "editPageTitle", "editSubmitLabel", "note-field"} {
		if strings.Contains(normalized, forbidden) {
			t.Fatalf("normalizeBuilderRuntimeWidgetTestContent() should drop title-based create/edit assertions for title-less mutation controller, found %q in %q", forbidden, normalized)
		}
	}
}

func TestNormalizeBuilderRuntimePatchForWorkspaceRemovesUnsupportedTotalCountFromOpenLiteCopy(t *testing.T) {
	workspacePath := t.TempDir()
	summaryPath := filepath.Join(workspacePath, "lib", "models", "dashboard_summary.dart")
	if err := os.MkdirAll(filepath.Dir(summaryPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(summaryPath) error = %v", err)
	}
	if err := os.WriteFile(summaryPath, []byte(strings.Join([]string{
		"class DashboardSummary {",
		"  const DashboardSummary({required this.inboxCount, required this.inProgressCount, required this.doneCount});",
		"  final int inboxCount;",
		"  final int inProgressCount;",
		"  final int doneCount;",
		"}",
	}, "\n")), 0o600); err != nil {
		t.Fatalf("WriteFile(dashboard_summary.dart) error = %v", err)
	}
	patch := appruns.WorkspacePatch{Operations: []appruns.WorkspacePatchOperation{{
		Type: "write_file",
		Path: "lib/template/domain_copy.dart",
		Content: strings.Join([]string{
			"class OpenLiteCopy {",
			"  String listCountLabel(int visibleCount, int totalCount) => '当前展示 $visibleCount / $totalCount 条待办';",
			"}",
		}, "\n"),
	}}}

	normalizeBuilderRuntimePatchForWorkspace(workspacePath, &patch)

	content := patch.Operations[0].Content
	if strings.Contains(content, "totalCount") {
		t.Fatalf("normalizeBuilderRuntimePatchForWorkspace() should remove totalCount from open_lite_copy when summary model lacks it: %q", content)
	}
	if !strings.Contains(content, "String listCountLabel(int visibleCount)") {
		t.Fatalf("normalizeBuilderRuntimePatchForWorkspace() should rewrite listCountLabel signature: %q", content)
	}
	if !strings.Contains(content, "当前展示 $visibleCount 条待办") {
		t.Fatalf("normalizeBuilderRuntimePatchForWorkspace() should rewrite listCountLabel body: %q", content)
	}
}

func TestNormalizeBuilderRuntimePatchForWorkspacePreservesTotalCountWhenSummaryDefinesIt(t *testing.T) {
	workspacePath := t.TempDir()
	summaryPath := filepath.Join(workspacePath, "lib", "models", "dashboard_summary.dart")
	if err := os.MkdirAll(filepath.Dir(summaryPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(summaryPath) error = %v", err)
	}
	if err := os.WriteFile(summaryPath, []byte(strings.Join([]string{
		"class DashboardSummary {",
		"  const DashboardSummary({required this.totalCount});",
		"  final int totalCount;",
		"}",
	}, "\n")), 0o600); err != nil {
		t.Fatalf("WriteFile(dashboard_summary.dart) error = %v", err)
	}
	original := "class OpenLiteCopy {\n  String listCountLabel(int visibleCount, int totalCount) => '当前展示 $visibleCount / $totalCount 条待办';\n}\n"
	patch := appruns.WorkspacePatch{Operations: []appruns.WorkspacePatchOperation{{
		Type:    "write_file",
		Path:    "lib/template/open_lite_copy.dart",
		Content: original,
	}}}

	normalizeBuilderRuntimePatchForWorkspace(workspacePath, &patch)

	if patch.Operations[0].Content != original {
		t.Fatalf("normalizeBuilderRuntimePatchForWorkspace() should preserve totalCount when summary model defines it: %q", patch.Operations[0].Content)
	}
}

func TestNormalizeBuilderRuntimePatchForWorkspaceRewritesAdditionalOpenLiteTotalCountVariants(t *testing.T) {
	workspacePath := t.TempDir()
	summaryPath := filepath.Join(workspacePath, "lib", "models", "dashboard_summary.dart")
	if err := os.MkdirAll(filepath.Dir(summaryPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(summaryPath) error = %v", err)
	}
	if err := os.WriteFile(summaryPath, []byte(strings.Join([]string{
		"class DashboardSummary {",
		"  const DashboardSummary({required this.inboxCount, required this.inProgressCount, required this.doneCount});",
		"  final int inboxCount;",
		"  final int inProgressCount;",
		"  final int doneCount;",
		"}",
	}, "\n")), 0o600); err != nil {
		t.Fatalf("WriteFile(dashboard_summary.dart) error = %v", err)
	}
	patch := appruns.WorkspacePatch{Operations: []appruns.WorkspacePatchOperation{{
		Type: "write_file",
		Path: "lib/template/open_lite_copy.dart",
		Content: strings.Join([]string{
			"class OpenLiteCopy {",
			"  String summaryCountLabel(int totalCount) => '$totalCount 条摘要';",
			"  String recentRecordsCountLabel(int totalCount) {",
			"    return '${totalCount} 条';",
			"  }",
			"  String totalCountLabel(int totalCount) => '共 $totalCount 条';",
			"}",
		}, "\n"),
	}}}

	normalizeBuilderRuntimePatchForWorkspace(workspacePath, &patch)

	content := patch.Operations[0].Content
	if strings.Contains(content, "totalCount") {
		t.Fatalf("normalizeBuilderRuntimePatchForWorkspace() should remove all totalCount variants from open_lite_copy: %q", content)
	}
	for _, want := range []string{
		"String summaryCountLabel(int count)",
		"String recentRecordsCountLabel(int count)",
		"return '${count} 条';",
		"String countLabel(int count)",
		"共 $count 条",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("normalizeBuilderRuntimePatchForWorkspace() content missing %q: %q", want, content)
		}
	}
}

func TestNormalizeBuilderRuntimePatchForWorkspaceRemovesOpenLiteTotalCountWithoutSummaryFileWhenDomainModelForbidsIt(t *testing.T) {
	jobRoot := t.TempDir()
	workspacePath := filepath.Join(jobRoot, "workspace")
	if err := os.MkdirAll(filepath.Join(workspacePath, "lib", "template"), 0o755); err != nil {
		t.Fatalf("MkdirAll(workspace) error = %v", err)
	}
	preparePath := filepath.Join(jobRoot, "prepare")
	if err := os.MkdirAll(preparePath, 0o755); err != nil {
		t.Fatalf("MkdirAll(prepare) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(preparePath, "domain-model.json"), []byte(strings.Join([]string{
		"{",
		"  \"entities\": [",
		"    {",
		"      \"fields\": [",
		"        {\"name\": \"task_id\"},",
		"        {\"name\": \"title\"},",
		"        {\"name\": \"category\"},",
		"        {\"name\": \"status\"},",
		"        {\"name\": \"note\"}",
		"      ]",
		"    }",
		"  ]",
		"}",
	}, "\n")), 0o600); err != nil {
		t.Fatalf("WriteFile(domain-model.json) error = %v", err)
	}
	patch := appruns.WorkspacePatch{Operations: []appruns.WorkspacePatchOperation{{
		Type: "write_file",
		Path: "lib/template/open_lite_copy.dart",
		Content: strings.Join([]string{
			"class OpenLiteCopy {",
			"  String listCountLabel(int visibleCount, int totalCount) => '当前展示 $visibleCount / $totalCount 条记录';",
			"}",
		}, "\n"),
	}}}

	normalizeBuilderRuntimePatchForWorkspace(workspacePath, &patch)

	content := patch.Operations[0].Content
	if strings.Contains(content, "totalCount") {
		t.Fatalf("normalizeBuilderRuntimePatchForWorkspace() should remove totalCount when domain model forbids it even without summary file: %q", content)
	}
	if !strings.Contains(content, "String listCountLabel(int visibleCount)") {
		t.Fatalf("normalizeBuilderRuntimePatchForWorkspace() should rewrite listCountLabel signature without summary file: %q", content)
	}
	if !strings.Contains(content, "当前展示 $visibleCount 条记录") {
		t.Fatalf("normalizeBuilderRuntimePatchForWorkspace() should rewrite listCountLabel body without summary file: %q", content)
	}
}

func TestNormalizeBuilderRuntimePatchForWorkspaceRemovesStatuslessOpenLiteCopyHelpersWithoutTotalCount(t *testing.T) {
	jobRoot := t.TempDir()
	workspacePath := filepath.Join(jobRoot, "workspace")
	if err := os.MkdirAll(filepath.Join(workspacePath, "lib", "template"), 0o755); err != nil {
		t.Fatalf("MkdirAll(workspace) error = %v", err)
	}
	preparePath := filepath.Join(jobRoot, "prepare")
	if err := os.MkdirAll(preparePath, 0o755); err != nil {
		t.Fatalf("MkdirAll(prepare) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(preparePath, "domain-model.json"), []byte(strings.Join([]string{
		"{",
		"  \"entities\": [",
		"    {",
		"      \"fields\": [",
		"        {\"name\": \"weight\"},",
		"        {\"name\": \"recorded_at\"},",
		"        {\"name\": \"note\"}",
		"      ]",
		"    }",
		"  ]",
		"}",
	}, "\n")), 0o600); err != nil {
		t.Fatalf("WriteFile(domain-model.json) error = %v", err)
	}
	patch := appruns.WorkspacePatch{Operations: []appruns.WorkspacePatchOperation{{
		Type: "write_file",
		Path: "lib/template/open_lite_copy.dart",
		Content: strings.Join([]string{
			"import '../models/record.dart';",
			"",
			"class OpenLiteCopy {",
			"  String get listFilterTitle => '筛选';",
			"  String get inboxFilterLabel => '待整理';",
			"  String get inProgressFilterLabel => '进行中';",
			"  String get doneFilterLabel => '已完成';",
			"  String get detailStatusLabel => '状态';",
			"  String statusLabel(RecordStatus status) {",
			"    switch (status) {",
			"      case RecordStatus.inbox:",
			"        return '待整理';",
			"      case RecordStatus.inProgress:",
			"        return '进行中';",
			"      case RecordStatus.done:",
			"        return '已完成';",
			"    }",
			"  }",
			"}",
		}, "\n"),
	}}}

	normalizeBuilderRuntimePatchForWorkspace(workspacePath, &patch)

	content := patch.Operations[0].Content
	for _, forbidden := range []string{"RecordStatus", "statusLabel(", "detailStatusLabel", "inboxFilterLabel", "inProgressFilterLabel", "doneFilterLabel", "../models/record.dart"} {
		if strings.Contains(content, forbidden) {
			t.Fatalf("normalizeBuilderRuntimePatchForWorkspace() should remove statusless open_lite_copy helper %q: %q", forbidden, content)
		}
	}
	if !strings.Contains(content, "String get listFilterTitle") {
		t.Fatalf("normalizeBuilderRuntimePatchForWorkspace() should preserve unrelated copy getters: %q", content)
	}
}

func TestNormalizeBuilderRuntimePatchForWorkspaceRetargetsGenericOpenLiteCopyStatusTypeToCurrentModel(t *testing.T) {
	workspacePath := createBuilderRuntimeCustomCollectionModelWorkspace(t, strings.Join([]string{
		"enum TaskStatus { inbox, inProgress, done }",
		"class Task {",
		"  const Task({required this.headline, required this.group, required this.status});",
		"  final String headline;",
		"  final String group;",
		"  final TaskStatus status;",
		"}",
	}, "\n")+"\n")
	patch := appruns.WorkspacePatch{Operations: []appruns.WorkspacePatchOperation{{
		Type: "write_file",
		Path: "lib/template/open_lite_copy.dart",
		Content: strings.Join([]string{
			"import '../models/record.dart';",
			"",
			"class OpenLiteCopy {",
			"  String get titleFieldLabel => '标题';",
			"  String get categoryFieldLabel => '分类';",
			"  String get detailCategoryLabel => '分类';",
			"  String statusLabel(RecordStatus status) {",
			"    switch (status) {",
			"      case RecordStatus.inbox:",
			"        return '待整理';",
			"      case RecordStatus.inProgress:",
			"        return '进行中';",
			"      case RecordStatus.done:",
			"        return '已完成';",
			"    }",
			"  }",
			"}",
		}, "\n"),
	}}}

	normalizeBuilderRuntimePatchForWorkspace(workspacePath, &patch)

	content := patch.Operations[0].Content
	for _, want := range []string{"import '../models/task.dart';", "String statusLabel(TaskStatus status) {", "case TaskStatus.inbox:"} {
		if !strings.Contains(content, want) {
			t.Fatalf("normalizeBuilderRuntimePatchForWorkspace() missing retargeted generic copy marker %q: %q", want, content)
		}
	}
	for _, forbidden := range []string{"import '../models/record.dart';", "RecordStatus", "titleFieldLabel", "categoryFieldLabel", "detailCategoryLabel"} {
		if strings.Contains(content, forbidden) {
			t.Fatalf("normalizeBuilderRuntimePatchForWorkspace() should not keep stale generic copy marker %q: %q", forbidden, content)
		}
	}
}

func TestNormalizeBuilderRuntimePatchForWorkspaceRetargetsGenericOpenLiteCopyStatusAPIAndMembersToCurrentContract(t *testing.T) {
	workspacePath := createBuilderRuntimeCustomCollectionModelWorkspace(t, strings.Join([]string{
		"enum TaskStatus { todo, doing, done }",
		"class Task {",
		"  const Task({required this.headline, required this.group, required this.status});",
		"  final String headline;",
		"  final String group;",
		"  final TaskStatus status;",
		"}",
	}, "\n")+"\n")
	writeBuilderRuntimeWorkspaceFiles(t, workspacePath, map[string]string{
		"lib/template/open_lite_copy.dart": strings.Join([]string{
			"import '../models/task.dart';",
			"",
			"class OpenLiteCopy {",
			"  String taskStatusLabel(TaskStatus status) {",
			"    switch (status) {",
			"      case TaskStatus.todo:",
			"        return '待办';",
			"      case TaskStatus.doing:",
			"        return '进行中';",
			"      case TaskStatus.done:",
			"        return '已完成';",
			"    }",
			"  }",
			"}",
		}, "\n") + "\n",
	})
	patch := appruns.WorkspacePatch{Operations: []appruns.WorkspacePatchOperation{{
		Type: "write_file",
		Path: "lib/template/open_lite_copy.dart",
		Content: strings.Join([]string{
			"import '../models/record.dart';",
			"",
			"class OpenLiteCopy {",
			"  String statusLabel(RecordStatus status) {",
			"    switch (status) {",
			"      case RecordStatus.inbox:",
			"        return '待整理';",
			"      case RecordStatus.inProgress:",
			"        return '进行中';",
			"      case RecordStatus.done:",
			"        return '已完成';",
			"    }",
			"  }",
			"}",
		}, "\n"),
	}}}

	normalizeBuilderRuntimePatchForWorkspace(workspacePath, &patch)

	content := patch.Operations[0].Content
	for _, want := range []string{
		"import '../models/task.dart';",
		"String taskStatusLabel(TaskStatus status) {",
		"case TaskStatus.todo:",
		"case TaskStatus.doing:",
		"case TaskStatus.done:",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("normalizeBuilderRuntimePatchForWorkspace() missing custom status contract marker %q: %q", want, content)
		}
	}
	for _, forbidden := range []string{
		"import '../models/record.dart';",
		"String statusLabel(",
		"RecordStatus",
		"TaskStatus.inbox",
		"TaskStatus.inProgress",
	} {
		if strings.Contains(content, forbidden) {
			t.Fatalf("normalizeBuilderRuntimePatchForWorkspace() should not keep stale generic status contract marker %q: %q", forbidden, content)
		}
	}
}

func TestNormalizeBuilderRuntimePatchForWorkspaceRemovesTitleAndCategoryOpenLiteCopyHelpersWhenDomainModelForbidsThem(t *testing.T) {
	jobRoot := t.TempDir()
	workspacePath := filepath.Join(jobRoot, "workspace")
	if err := os.MkdirAll(filepath.Join(workspacePath, "lib", "template"), 0o755); err != nil {
		t.Fatalf("MkdirAll(workspace) error = %v", err)
	}
	preparePath := filepath.Join(jobRoot, "prepare")
	if err := os.MkdirAll(preparePath, 0o755); err != nil {
		t.Fatalf("MkdirAll(prepare) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(preparePath, "domain-model.json"), []byte(strings.Join([]string{
		"{",
		"  \"entities\": [",
		"    {",
		"      \"fields\": [",
		"        {\"name\": \"record_id\"},",
		"        {\"name\": \"weight\"},",
		"        {\"name\": \"recorded_at\"},",
		"        {\"name\": \"note\"}",
		"      ]",
		"    }",
		"  ]",
		"}",
	}, "\n")), 0o600); err != nil {
		t.Fatalf("WriteFile(domain-model.json) error = %v", err)
	}
	patch := appruns.WorkspacePatch{Operations: []appruns.WorkspacePatchOperation{{
		Type: "write_file",
		Path: "lib/template/open_lite_copy.dart",
		Content: strings.Join([]string{
			"class OpenLiteCopy {",
			"  String get titleFieldLabel => '标题';",
			"  String get titleFieldRequiredError => '请输入标题';",
			"  String get categoryFieldLabel => '分类';",
			"  String get detailCategoryLabel => '分类';",
			"  String get noteFieldLabel => '备注';",
			"  String get dateFieldLabel => '记录时间';",
			"}",
		}, "\n"),
	}}}

	normalizeBuilderRuntimePatchForWorkspace(workspacePath, &patch)

	content := patch.Operations[0].Content
	for _, forbidden := range []string{"titleFieldLabel", "titleFieldRequiredError", "categoryFieldLabel", "detailCategoryLabel"} {
		if strings.Contains(content, forbidden) {
			t.Fatalf("normalizeBuilderRuntimePatchForWorkspace() should remove title/category open_lite_copy helper %q: %q", forbidden, content)
		}
	}
	for _, required := range []string{"noteFieldLabel", "dateFieldLabel"} {
		if !strings.Contains(content, required) {
			t.Fatalf("normalizeBuilderRuntimePatchForWorkspace() should preserve unrelated copy getter %q: %q", required, content)
		}
	}
}

func TestBuilderRuntimeForbiddenTokensFromDomainModelAllowsRoleBasedGenericCopyHelpers(t *testing.T) {
	workspacePath := createBuilderRuntimeWorkspaceWithDomainModel(t, strings.Join([]string{
		"{",
		"  \"entities\": [",
		"    {",
		"      \"source\": \"local_storage\",",
		"      \"fields\": [",
		"        {\"name\": \"movie_record_id\", \"role\": \"identifier\"},",
		"        {\"name\": \"movie_title\", \"role\": \"primary_text\"},",
		"        {\"name\": \"genre\", \"role\": \"secondary_text\"},",
		"        {\"name\": \"watch_status\", \"role\": \"status\"}",
		"      ]",
		"    }",
		"  ]",
		"}",
	}, "\n"), map[string]string{
		"lib/template/open_lite_copy.dart": "class OpenLiteCopy {}\n",
	})

	forbidden, err := builderRuntimeForbiddenTokensFromDomainModel(workspacePath)
	if err != nil {
		t.Fatalf("builderRuntimeForbiddenTokensFromDomainModel() error = %v", err)
	}
	for _, allowed := range []string{"titleFieldLabel", "categoryFieldLabel", "detailCategoryLabel", "statusLabel(", "doneFilterLabel", "doneCount"} {
		if slices.Contains(forbidden, allowed) {
			t.Fatalf("builderRuntimeForbiddenTokensFromDomainModel() should allow role-based helper %q, forbidden=%v", allowed, forbidden)
		}
	}
	for _, forbiddenToken := range []string{"RecordStatus", ".status"} {
		if !slices.Contains(forbidden, forbiddenToken) {
			t.Fatalf("builderRuntimeForbiddenTokensFromDomainModel() should still forbid stale model token %q, forbidden=%v", forbiddenToken, forbidden)
		}
	}
}

func TestNormalizeBuilderRuntimePatchForWorkspaceCanonicalizesRelationRichOpenLiteCopy(t *testing.T) {
	workspacePath := createBuilderRuntimeProjectTaskTagOverviewWorkspace(t)
	stale := strings.Join([]string{
		"import '../models/task.dart';",
		"",
		"class _OpenLiteCopy {",
		"  String get appTitle => '/jobs regression relation-rich project-task-tag';",
		"  String get categoryFieldLabel => '所属项目';",
		"  String get detailCategoryLabel => '所属项目';",
		"  String listCountLabel(int visibleCount, int totalCount) => '当前展示 $visibleCount / $totalCount 条任务';",
		"  String statusLabel(TaskStatus status) {",
		"    switch (status) {",
		"      case TaskStatus.todo:",
		"        return '待开始';",
		"      case TaskStatus.doing:",
		"        return '进行中';",
		"      case TaskStatus.done:",
		"        return '已完成';",
		"    }",
		"  }",
		"}",
		"",
		"const openLiteCopy = _OpenLiteCopy();",
	}, "\n") + "\n"
	expected := builderRuntimeOpenLiteCanonicalRelationRichCopyContent(workspacePath, stale)
	if strings.TrimSpace(expected) == "" {
		t.Fatal("builderRuntimeOpenLiteCanonicalRelationRichCopyContent() returned empty canonical copy")
	}
	patch := appruns.WorkspacePatch{Operations: []appruns.WorkspacePatchOperation{{
		Type:    "write_file",
		Path:    "lib/template/open_lite_copy.dart",
		Content: stale,
	}}}

	normalizeBuilderRuntimePatchForWorkspace(workspacePath, &patch)

	content := patch.Operations[0].Content
	if content != expected {
		t.Fatalf("normalizeBuilderRuntimePatchForWorkspace() should canonicalize relation-rich open_lite_copy\nwant:\n%s\n\ngot:\n%s", expected, content)
	}
	for _, forbidden := range []string{"categoryFieldLabel", "detailCategoryLabel", "statusLabel(", "totalCount"} {
		if strings.Contains(content, forbidden) {
			t.Fatalf("normalizeBuilderRuntimePatchForWorkspace() should remove stale relation-rich copy helper %q: %q", forbidden, content)
		}
	}
	for _, required := range []string{"projectFieldLabel", "detailProjectLabel", "taskStatusLabel", "projectFilterLabel", "tagFilterLabel", "listCountLabel(int visibleCount)"} {
		if !strings.Contains(content, required) {
			t.Fatalf("normalizeBuilderRuntimePatchForWorkspace() canonical relation-rich copy missing %q: %q", required, content)
		}
	}
}

func TestNormalizeBuilderRuntimePatchForWorkspacePreservesCanonicalRelationRichOpenLiteCopy(t *testing.T) {
	workspacePath := createBuilderRuntimeProjectTaskTagOverviewWorkspace(t)
	seed := strings.Join([]string{
		"class _OpenLiteCopy {",
		"  String get appTitle => 'Project Task Tag';",
		"}",
		"",
		"const openLiteCopy = _OpenLiteCopy();",
	}, "\n") + "\n"
	canonical := builderRuntimeOpenLiteCanonicalRelationRichCopyContent(workspacePath, seed)
	if strings.TrimSpace(canonical) == "" {
		t.Fatal("builderRuntimeOpenLiteCanonicalRelationRichCopyContent() returned empty canonical copy")
	}
	patch := appruns.WorkspacePatch{Operations: []appruns.WorkspacePatchOperation{{
		Type:    "write_file",
		Path:    "lib/template/open_lite_copy.dart",
		Content: canonical,
	}}}

	normalizeBuilderRuntimePatchForWorkspace(workspacePath, &patch)

	if patch.Operations[0].Content != canonical {
		t.Fatalf("normalizeBuilderRuntimePatchForWorkspace() should preserve canonical relation-rich open_lite_copy\nwant:\n%s\n\ngot:\n%s", canonical, patch.Operations[0].Content)
	}
}

func TestNormalizeBuilderRuntimePatchForWorkspaceCanonicalizesRelationRichFormController(t *testing.T) {
	workspacePath := createBuilderRuntimeProjectTaskTagOverviewWorkspace(t)
	stale := strings.Join([]string{
		"import 'package:flutter/material.dart';",
		"",
		"import '../models/project.dart';",
		"import '../models/tag.dart';",
		"import '../models/task.dart';",
		"import '../repositories/record_repository.dart';",
		"",
		"class RecordFormController extends ChangeNotifier {",
		"  RecordFormController({required this.recordRepository})",
		"      : titleController = TextEditingController(),",
		"        noteController = TextEditingController(),",
		"        categoryController = TextEditingController(text: _categories.first);",
		"",
		"  static const List<String> _categories = <String>[",
		"    '项目',",
		"    '任务',",
		"    '标签',",
		"  ];",
		"",
		"  final RecordRepository recordRepository;",
		"  final TextEditingController titleController;",
		"  final TextEditingController noteController;",
		"  final TextEditingController categoryController;",
		"",
		"  void setCategory(String category) {",
		"    categoryController.text = category;",
		"    notifyListeners();",
		"  }",
		"}",
	}, "\n") + "\n"
	expected := builderRuntimeOpenLiteCanonicalRelationRichFormControllerContent(workspacePath, stale)
	if strings.TrimSpace(expected) == "" {
		t.Fatal("builderRuntimeOpenLiteCanonicalRelationRichFormControllerContent() returned empty canonical form controller")
	}
	patch := appruns.WorkspacePatch{Operations: []appruns.WorkspacePatchOperation{{
		Type:    "write_file",
		Path:    "lib/controllers/record_form_controller.dart",
		Content: stale,
	}}}

	normalizeBuilderRuntimePatchForWorkspace(workspacePath, &patch)

	content := patch.Operations[0].Content
	if content != expected {
		t.Fatalf("normalizeBuilderRuntimePatchForWorkspace() should canonicalize relation-rich record_form_controller\nwant:\n%s\n\ngot:\n%s", expected, content)
	}
	for _, forbidden := range []string{"categoryController", "_categories", "setCategory("} {
		if strings.Contains(content, forbidden) {
			t.Fatalf("normalizeBuilderRuntimePatchForWorkspace() should remove stale relation-rich form controller helper %q: %q", forbidden, content)
		}
	}
	for _, required := range []string{"projectController", "setProject(", "toggleTag(", "selectedTagIds", "TaskTagLink"} {
		if !strings.Contains(content, required) {
			t.Fatalf("normalizeBuilderRuntimePatchForWorkspace() canonical relation-rich form controller missing %q: %q", required, content)
		}
	}
}

func TestNormalizeBuilderRuntimePatchForWorkspacePreservesCanonicalRelationRichFormController(t *testing.T) {
	workspacePath := createBuilderRuntimeProjectTaskTagOverviewWorkspace(t)
	canonical := builderRuntimeOpenLiteCanonicalRelationRichFormControllerContent(workspacePath, "class RecordFormController extends ChangeNotifier {}\n")
	if strings.TrimSpace(canonical) == "" {
		t.Fatal("builderRuntimeOpenLiteCanonicalRelationRichFormControllerContent() returned empty canonical form controller")
	}
	patch := appruns.WorkspacePatch{Operations: []appruns.WorkspacePatchOperation{{
		Type:    "write_file",
		Path:    "lib/controllers/record_form_controller.dart",
		Content: canonical,
	}}}

	normalizeBuilderRuntimePatchForWorkspace(workspacePath, &patch)

	if patch.Operations[0].Content != canonical {
		t.Fatalf("normalizeBuilderRuntimePatchForWorkspace() should preserve canonical relation-rich record_form_controller\nwant:\n%s\n\ngot:\n%s", canonical, patch.Operations[0].Content)
	}
}

func TestNormalizeBuilderRuntimePatchForWorkspaceCanonicalizesRelationRichCustomFormController(t *testing.T) {
	workspacePath := createBuilderRuntimeProjectTaskTagOverviewWorkspace(t)
	stale := strings.Join([]string{
		"import 'package:flutter/material.dart';",
		"",
		"import '../models/project.dart';",
		"import '../models/tag.dart';",
		"import '../models/task.dart';",
		"import '../repositories/record_repository.dart';",
		"",
		"class TaskFormController extends ChangeNotifier {",
		"  TaskFormController({required this.repository})",
		"      : titleController = TextEditingController(),",
		"        noteController = TextEditingController(),",
		"        categoryController = TextEditingController(text: _categories.first);",
		"",
		"  static const List<String> _categories = <String>['项目', '任务', '标签'];",
		"",
		"  final RecordRepository repository;",
		"  final TextEditingController titleController;",
		"  final TextEditingController noteController;",
		"  final TextEditingController categoryController;",
		"",
		"  void setCategory(String category) {",
		"    categoryController.text = category;",
		"    notifyListeners();",
		"  }",
		"}",
	}, "\n") + "\n"
	patch := appruns.WorkspacePatch{Operations: []appruns.WorkspacePatchOperation{{
		Type:    "write_file",
		Path:    "lib/controllers/task_form_controller.dart",
		Content: stale,
	}}}
	taskBundle := []appruns.TaskBundleItem{{
		TaskID:      "task-create-form-controller",
		TaskType:    appruns.BuilderRuntimeTaskTypeDualFileWiring,
		TargetPaths: []string{"lib/controllers/task_form_controller.dart"},
	}}

	normalizeBuilderRuntimePatchForWorkspaceWithTaskBundle(workspacePath, taskBundle, &patch)

	content := patch.Operations[0].Content
	for _, forbidden := range []string{"categoryController", "_categories", "setCategory(", "class RecordFormController"} {
		if strings.Contains(content, forbidden) {
			t.Fatalf("normalizeBuilderRuntimePatchForWorkspaceWithTaskBundle() should remove stale custom form controller drift %q: %q", forbidden, content)
		}
	}
	for _, required := range []string{"class TaskFormController extends ChangeNotifier", "projectController", "setProject(", "toggleTag(", "selectedTagIds", "TaskTagLink"} {
		if !strings.Contains(content, required) {
			t.Fatalf("normalizeBuilderRuntimePatchForWorkspaceWithTaskBundle() canonical custom form controller missing %q: %q", required, content)
		}
	}
}

func TestNormalizeBuilderRuntimePatchForWorkspaceCanonicalizesRelationRichCustomFormControllerWithCustomRepository(t *testing.T) {
	workspacePath := createBuilderRuntimeProjectTaskTagOverviewWorkspace(t)
	writeBuilderRuntimeWorkspaceFiles(t, workspacePath, map[string]string{
		"lib/repositories/task_repository.dart": strings.Join([]string{
			"abstract class TaskRepository {",
			"  Future<List<dynamic>> loadProjects();",
			"  Future<List<dynamic>> loadTasks();",
			"  Future<List<dynamic>> loadTags();",
			"  Future<List<dynamic>> loadTaskTagLinks();",
			"  Future<void> saveTasks(List<dynamic> tasks);",
			"  Future<void> saveTaskTagLinks(List<dynamic> links);",
			"}",
		}, "\n") + "\n",
	})
	stale := strings.Join([]string{
		"import 'package:flutter/material.dart';",
		"",
		"import '../models/project.dart';",
		"import '../models/tag.dart';",
		"import '../models/task.dart';",
		"import '../repositories/task_repository.dart';",
		"",
		"class TaskFormController extends ChangeNotifier {",
		"  TaskFormController({required this.taskRepository})",
		"      : titleController = TextEditingController(),",
		"        noteController = TextEditingController(),",
		"        categoryController = TextEditingController(text: _categories.first);",
		"",
		"  static const List<String> _categories = <String>['项目', '任务', '标签'];",
		"",
		"  final TaskRepository taskRepository;",
		"  final TextEditingController titleController;",
		"  final TextEditingController noteController;",
		"  final TextEditingController categoryController;",
		"",
		"  void setCategory(String category) {",
		"    categoryController.text = category;",
		"    notifyListeners();",
		"  }",
		"}",
	}, "\n") + "\n"
	patch := appruns.WorkspacePatch{Operations: []appruns.WorkspacePatchOperation{{
		Type:    "write_file",
		Path:    "lib/controllers/task_form_controller.dart",
		Content: stale,
	}}}
	taskBundle := []appruns.TaskBundleItem{{
		TaskID:      "task-create-form-controller",
		TaskType:    appruns.BuilderRuntimeTaskTypeDualFileWiring,
		TargetPaths: []string{"lib/controllers/task_form_controller.dart"},
	}}

	normalizeBuilderRuntimePatchForWorkspaceWithTaskBundle(workspacePath, taskBundle, &patch)

	content := patch.Operations[0].Content
	for _, want := range []string{"import '../repositories/task_repository.dart';", "class TaskFormController extends ChangeNotifier", "required this.taskRepository", "final TaskRepository taskRepository;", "selectedTagIds", "toggleTag("} {
		if !strings.Contains(content, want) {
			t.Fatalf("normalizeBuilderRuntimePatchForWorkspaceWithTaskBundle() canonical custom form controller missing %q: %q", want, content)
		}
	}
	if strings.Contains(content, "RecordRepository") {
		t.Fatalf("normalizeBuilderRuntimePatchForWorkspaceWithTaskBundle() should not keep default RecordRepository type for custom form controller: %q", content)
	}
	if strings.Contains(content, "categoryController") || strings.Contains(content, "setCategory(") {
		t.Fatalf("normalizeBuilderRuntimePatchForWorkspaceWithTaskBundle() should replace stale custom form controller drift with canonical content: %q", content)
	}
}

func TestNormalizeBuilderRuntimePatchForWorkspaceRewritesHomePageTotalCountToControllerRecordCount(t *testing.T) {
	workspacePath := t.TempDir()
	summaryPath := filepath.Join(workspacePath, "lib", "models", "dashboard_summary.dart")
	if err := os.MkdirAll(filepath.Dir(summaryPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(summaryPath) error = %v", err)
	}
	if err := os.WriteFile(summaryPath, []byte(strings.Join([]string{
		"class DashboardSummary {",
		"  const DashboardSummary({required this.inboxCount, required this.inProgressCount, required this.doneCount});",
		"  final int inboxCount;",
		"  final int inProgressCount;",
		"  final int doneCount;",
		"}",
	}, "\n")), 0o600); err != nil {
		t.Fatalf("WriteFile(dashboard_summary.dart) error = %v", err)
	}
	patch := appruns.WorkspacePatch{Operations: []appruns.WorkspacePatchOperation{{
		Type:    "write_file",
		Path:    "lib/views/home_page.dart",
		Content: "class HomePage {\n  String render(dynamic summary, dynamic controller) => '${summary.totalCount}-${summary.totalCount}';\n}\n",
	}}}

	normalizeBuilderRuntimePatchForWorkspace(workspacePath, &patch)

	content := patch.Operations[0].Content
	if strings.Contains(content, "totalCount") {
		t.Fatalf("normalizeBuilderRuntimePatchForWorkspace() content still references totalCount: %q", content)
	}
	if strings.Count(content, "controller.records.length") != 2 {
		t.Fatalf("normalizeBuilderRuntimePatchForWorkspace() should rewrite both totalCount references to controller.records.length: %q", content)
	}
}

func TestNormalizeBuilderRuntimePatchForWorkspaceRewritesHomePageTotalCountWithoutSummaryFileWhenDomainModelForbidsIt(t *testing.T) {
	jobRoot := t.TempDir()
	workspacePath := filepath.Join(jobRoot, "workspace")
	if err := os.MkdirAll(filepath.Join(workspacePath, "lib", "views"), 0o755); err != nil {
		t.Fatalf("MkdirAll(workspace views) error = %v", err)
	}
	preparePath := filepath.Join(jobRoot, "prepare")
	if err := os.MkdirAll(preparePath, 0o755); err != nil {
		t.Fatalf("MkdirAll(prepare) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(preparePath, "domain-model.json"), []byte(strings.Join([]string{
		"{",
		"  \"entities\": [",
		"    {",
		"      \"fields\": [",
		"        {\"name\": \"task_id\"},",
		"        {\"name\": \"title\"},",
		"        {\"name\": \"category\"},",
		"        {\"name\": \"status\"},",
		"        {\"name\": \"note\"}",
		"      ]",
		"    }",
		"  ]",
		"}",
	}, "\n")), 0o600); err != nil {
		t.Fatalf("WriteFile(domain-model.json) error = %v", err)
	}
	patch := appruns.WorkspacePatch{Operations: []appruns.WorkspacePatchOperation{{
		Type:    "write_file",
		Path:    "lib/views/home_page.dart",
		Content: "class HomePage {\n  String render(dynamic summary, dynamic controller) => '${summary.totalCount}-${summary.totalCount}';\n}\n",
	}}}

	normalizeBuilderRuntimePatchForWorkspace(workspacePath, &patch)

	content := patch.Operations[0].Content
	if strings.Contains(content, "summary.totalCount") {
		t.Fatalf("normalizeBuilderRuntimePatchForWorkspace() should rewrite summary.totalCount when domain model forbids it without summary file: %q", content)
	}
	if strings.Count(content, "controller.records.length") != 2 {
		t.Fatalf("normalizeBuilderRuntimePatchForWorkspace() should rewrite both totalCount references without summary file: %q", content)
	}
}

func TestNormalizeBuilderRuntimePatchForWorkspaceRewritesRecordListPageCountCallWithoutSummaryFileWhenDomainModelForbidsIt(t *testing.T) {
	jobRoot := t.TempDir()
	workspacePath := filepath.Join(jobRoot, "workspace")
	if err := os.MkdirAll(filepath.Join(workspacePath, "lib", "views"), 0o755); err != nil {
		t.Fatalf("MkdirAll(workspace views) error = %v", err)
	}
	preparePath := filepath.Join(jobRoot, "prepare")
	if err := os.MkdirAll(preparePath, 0o755); err != nil {
		t.Fatalf("MkdirAll(prepare) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(preparePath, "domain-model.json"), []byte(strings.Join([]string{
		"{",
		"  \"entities\": [",
		"    {",
		"      \"fields\": [",
		"        {\"name\": \"task_id\"},",
		"        {\"name\": \"title\"},",
		"        {\"name\": \"category\"},",
		"        {\"name\": \"status\"},",
		"        {\"name\": \"note\"}",
		"      ]",
		"    }",
		"  ]",
		"}",
	}, "\n")), 0o600); err != nil {
		t.Fatalf("WriteFile(domain-model.json) error = %v", err)
	}
	patch := appruns.WorkspacePatch{Operations: []appruns.WorkspacePatchOperation{{
		Type:    "write_file",
		Path:    "lib/views/record_list_page.dart",
		Content: "Text(openLiteCopy.listCountLabel(visibleRecords.length, records.length));\n",
	}}}

	normalizeBuilderRuntimePatchForWorkspace(workspacePath, &patch)

	content := patch.Operations[0].Content
	if strings.Contains(content, "listCountLabel(visibleRecords.length, records.length)") {
		t.Fatalf("normalizeBuilderRuntimePatchForWorkspace() should rewrite listCountLabel call when domain model forbids totalCount: %q", content)
	}
	if !strings.Contains(content, "listCountLabel(visibleRecords.length)") {
		t.Fatalf("normalizeBuilderRuntimePatchForWorkspace() should keep single visibleCount argument for listCountLabel: %q", content)
	}
}

func TestNormalizeBuilderRuntimePatchForWorkspaceRewritesRecordListPageBuilderOutputToSafeStatusAndCountUsage(t *testing.T) {
	jobRoot := t.TempDir()
	workspacePath := filepath.Join(jobRoot, "workspace")
	if err := os.MkdirAll(filepath.Join(workspacePath, "lib", "views"), 0o755); err != nil {
		t.Fatalf("MkdirAll(workspace views) error = %v", err)
	}
	if err := os.MkdirAll(filepath.Join(workspacePath, "lib", "controllers"), 0o755); err != nil {
		t.Fatalf("MkdirAll(workspace controllers) error = %v", err)
	}
	preparePath := filepath.Join(jobRoot, "prepare")
	if err := os.MkdirAll(preparePath, 0o755); err != nil {
		t.Fatalf("MkdirAll(prepare) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(preparePath, "domain-model.json"), []byte(strings.Join([]string{
		"{",
		"  \"entities\": [",
		"    {",
		"      \"fields\": [",
		"        {\"name\": \"task_id\"},",
		"        {\"name\": \"title\"},",
		"        {\"name\": \"category\"},",
		"        {\"name\": \"status\"},",
		"        {\"name\": \"note\"}",
		"      ]",
		"    }",
		"  ]",
		"}",
	}, "\n")), 0o600); err != nil {
		t.Fatalf("WriteFile(domain-model.json) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(workspacePath, "lib", "controllers", "record_list_controller.dart"), []byte(strings.Join([]string{
		"import '../models/record.dart';",
		"class RecordListController {",
		"  TodoItemStatus? _selectedFilter;",
		"  TodoItemStatus? get selectedFilter => _selectedFilter;",
		"  void setFilter(TodoItemStatus? filter) {}",
		"}",
	}, "\n")), 0o600); err != nil {
		t.Fatalf("WriteFile(record_list_controller.dart) error = %v", err)
	}
	patch := appruns.WorkspacePatch{Operations: []appruns.WorkspacePatchOperation{{
		Type: "write_file",
		Path: "lib/views/record_list_page.dart",
		Content: strings.Join([]string{
			"import '../repositories/record_repository.dart';",
			"import 'package:flutter/material.dart';",
			"",
			"import '../controllers/record_list_controller.dart';",
			"import '../models/record.dart';",
			"import '../template/open_lite_copy.dart';",
			"",
			"class RecordListPage extends StatelessWidget {",
			"  @override",
			"  Widget build(BuildContext context) {",
			"    final visibleRecords = controller.visibleRecords;",
			"    final records = controller.records;",
			"    return Text(",
			"      openLiteCopy.listCountLabel(visibleRecords.length, records.length),",
			"      style: Theme.of(context).textTheme.bodyMedium,",
			"    );",
			"  }",
			"}",
			"",
			"class _FilterChip extends StatelessWidget {",
			"  const _FilterChip({required this.filter, required this.controller, required this.label});",
			"  final RecordListFilter filter;",
			"  final RecordListController controller;",
			"  final String label;",
			"  @override",
			"  Widget build(BuildContext context) {",
			"    return ChoiceChip(",
			"      key: Key('record-filter-${filter.name}'),",
			"      label: Text(label),",
			"      selected: controller.selectedFilter == filter,",
			"      onSelected: (_) => controller.setFilter(filter),",
			"    );",
			"  }",
			"}",
			"",
			"void clearFilter(RecordListController controller) {",
			"  controller.setFilter(RecordListFilter.all);",
			"  final inbox = RecordListFilter.inbox;",
			"  final progress = RecordListFilter.inProgress;",
			"  final done = RecordListFilter.done;",
			"  print('$inbox$progress$done');",
			"}",
		}, "\n"),
	}}}

	normalizeBuilderRuntimePatchForWorkspace(workspacePath, &patch)

	content := patch.Operations[0].Content
	if strings.Contains(content, "RecordListFilter") {
		t.Fatalf("normalizeBuilderRuntimePatchForWorkspace() should remove RecordListFilter references from record_list_page: %q", content)
	}
	if strings.Contains(content, "../repositories/record_repository.dart") {
		t.Fatalf("normalizeBuilderRuntimePatchForWorkspace() should remove unused repository import from record_list_page: %q", content)
	}
	if strings.Contains(content, ")).textTheme.bodyMedium") {
		t.Fatalf("normalizeBuilderRuntimePatchForWorkspace() should preserve Text style syntax when rewriting listCountLabel call: %q", content)
	}
	if !strings.Contains(content, "openLiteCopy.listCountLabel(visibleRecords.length),") {
		t.Fatalf("normalizeBuilderRuntimePatchForWorkspace() should keep a single visibleCount argument in multiline builder output: %q", content)
	}
	if !strings.Contains(content, "style: Theme.of(context).textTheme.bodyMedium,") {
		t.Fatalf("normalizeBuilderRuntimePatchForWorkspace() should preserve Text style after listCountLabel rewrite: %q", content)
	}
	if !strings.Contains(content, "final TodoItemStatus? filter;") {
		t.Fatalf("normalizeBuilderRuntimePatchForWorkspace() should rewrite RecordListFilter field type to current controller status enum: %q", content)
	}
	for _, want := range []string{"TodoItemStatus.inbox", "TodoItemStatus.inProgress", "TodoItemStatus.done", "controller.setFilter(null)", "record-filter-all"} {
		if !strings.Contains(content, want) {
			t.Fatalf("normalizeBuilderRuntimePatchForWorkspace() content missing %q: %q", want, content)
		}
	}
}

func TestNormalizeBuilderRuntimePatchForWorkspaceRewritesRecordListPageBuilderOutputToSafeStatusAndCountUsageWithCustomCollectionController(t *testing.T) {
	jobRoot := t.TempDir()
	workspacePath := filepath.Join(jobRoot, "workspace")
	if err := os.MkdirAll(filepath.Join(workspacePath, "lib", "views"), 0o755); err != nil {
		t.Fatalf("MkdirAll(workspace views) error = %v", err)
	}
	if err := os.MkdirAll(filepath.Join(workspacePath, "lib", "controllers"), 0o755); err != nil {
		t.Fatalf("MkdirAll(workspace controllers) error = %v", err)
	}
	preparePath := filepath.Join(jobRoot, "prepare")
	if err := os.MkdirAll(preparePath, 0o755); err != nil {
		t.Fatalf("MkdirAll(prepare) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(preparePath, "domain-model.json"), []byte(strings.Join([]string{
		"{",
		"  \"entities\": [",
		"    {",
		"      \"fields\": [",
		"        {\"name\": \"task_id\"},",
		"        {\"name\": \"title\"},",
		"        {\"name\": \"category\"},",
		"        {\"name\": \"status\"},",
		"        {\"name\": \"note\"}",
		"      ]",
		"    }",
		"  ]",
		"}",
	}, "\n")), 0o600); err != nil {
		t.Fatalf("WriteFile(domain-model.json) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(workspacePath, "lib", "controllers", "task_collection_controller.dart"), []byte(strings.Join([]string{
		"import '../models/record.dart';",
		"class TaskCollectionController {",
		"  TaskCollectionController({required this.taskRepository});",
		"  final Object taskRepository;",
		"  TodoItemStatus? _selectedFilter;",
		"  TodoItemStatus? get selectedFilter => _selectedFilter;",
		"  void setFilter(TodoItemStatus? filter) {}",
		"}",
	}, "\n")), 0o600); err != nil {
		t.Fatalf("WriteFile(task_collection_controller.dart) error = %v", err)
	}
	patch := appruns.WorkspacePatch{Operations: []appruns.WorkspacePatchOperation{{
		Type: "write_file",
		Path: "lib/views/record_list_page.dart",
		Content: strings.Join([]string{
			"import '../repositories/record_repository.dart';",
			"import 'package:flutter/material.dart';",
			"",
			"import '../controllers/task_collection_controller.dart';",
			"import '../models/record.dart';",
			"import '../template/open_lite_copy.dart';",
			"",
			"class RecordListPage extends StatelessWidget {",
			"  @override",
			"  Widget build(BuildContext context) {",
			"    final visibleRecords = controller.visibleRecords;",
			"    final records = controller.records;",
			"    return Text(",
			"      openLiteCopy.listCountLabel(visibleRecords.length, records.length),",
			"      style: Theme.of(context).textTheme.bodyMedium,",
			"    );",
			"  }",
			"}",
			"",
			"class _FilterChip extends StatelessWidget {",
			"  const _FilterChip({required this.filter, required this.controller, required this.label});",
			"  final RecordListFilter filter;",
			"  final TaskCollectionController controller;",
			"  final String label;",
			"  @override",
			"  Widget build(BuildContext context) {",
			"    return ChoiceChip(",
			"      key: Key('record-filter-${filter.name}'),",
			"      label: Text(label),",
			"      selected: controller.selectedFilter == filter,",
			"      onSelected: (_) => controller.setFilter(filter),",
			"    );",
			"  }",
			"}",
			"",
			"void clearFilter(TaskCollectionController controller) {",
			"  controller.setFilter(RecordListFilter.all);",
			"  final inbox = RecordListFilter.inbox;",
			"  final progress = RecordListFilter.inProgress;",
			"  final done = RecordListFilter.done;",
			"  print('$inbox$progress$done');",
			"}",
		}, "\n"),
	}}}

	normalizeBuilderRuntimePatchForWorkspace(workspacePath, &patch)

	content := patch.Operations[0].Content
	if strings.Contains(content, "RecordListFilter") {
		t.Fatalf("normalizeBuilderRuntimePatchForWorkspace() should remove RecordListFilter references from record_list_page with custom collection controller: %q", content)
	}
	if !strings.Contains(content, "final TodoItemStatus? filter;") {
		t.Fatalf("normalizeBuilderRuntimePatchForWorkspace() should rewrite RecordListFilter field type using custom collection controller status enum: %q", content)
	}
	for _, want := range []string{"TodoItemStatus.inbox", "TodoItemStatus.inProgress", "TodoItemStatus.done", "controller.setFilter(null)", "record-filter-all"} {
		if !strings.Contains(content, want) {
			t.Fatalf("normalizeBuilderRuntimePatchForWorkspace() custom collection controller content missing %q: %q", want, content)
		}
	}
}

func TestNormalizeBuilderRuntimeRecordListPageContentCanonicalizesNoFilterCustomCollectionPage(t *testing.T) {
	workspacePath := t.TempDir()
	writeBuilderRuntimeWorkspaceFiles(t, workspacePath, map[string]string{
		"lib/models/record.dart": strings.Join([]string{
			"class TodoItem {",
			"  const TodoItem({required this.title});",
			"  final String title;",
			"}",
		}, "\n") + "\n",
		"lib/models/task.dart": strings.Join([]string{
			"class Task {",
			"  const Task({required this.title});",
			"  final String title;",
			"}",
		}, "\n") + "\n",
		"lib/repositories/task_repository.dart": "abstract class TaskRepository {}\n",
		"lib/controllers/task_collection_controller.dart": strings.Join([]string{
			"import '../models/task.dart';",
			"import '../repositories/task_repository.dart';",
			"",
			"class TaskCollectionController {",
			"  TaskCollectionController({required TaskRepository taskRepository}) : _taskRepository = taskRepository;",
			"",
			"  final TaskRepository _taskRepository;",
			"",
			"  List<Task> get records => const [];",
			"}",
		}, "\n") + "\n",
		"lib/views/task_collection_page.dart": strings.Join([]string{
			"import 'package:flutter/material.dart';",
			"",
			"import '../controllers/task_collection_controller.dart';",
			"import '../models/task.dart';",
			"",
			"class TaskCollectionPage extends StatelessWidget {",
			"  const TaskCollectionPage({",
			"    super.key,",
			"    required this.controller,",
			"    required this.onCreateTask,",
			"    required this.onOpenTaskDetail,",
			"  });",
			"",
			"  final TaskCollectionController controller;",
			"  final Future<void> Function() onCreateTask;",
			"  final Future<void> Function(Task task) onOpenTaskDetail;",
			"",
			"  @override",
			"  Widget build(BuildContext context) => const SizedBox.shrink();",
			"}",
		}, "\n") + "\n",
	})

	original := strings.Join([]string{
		"import 'package:flutter/material.dart';",
		"",
		"import '../controllers/task_collection_controller.dart';",
		"import '../models/task.dart';",
		"import '../template/open_lite_copy.dart';",
		"",
		"class RecordListPage extends StatelessWidget {",
		"  const RecordListPage({",
		"    super.key,",
		"    required this.controller,",
		"    required this.onOpenTaskDetail,",
		"  });",
		"",
		"  final TaskCollectionController controller;",
		"  final Future<void> Function(Task task) onOpenTaskDetail;",
		"",
		"  @override",
		"  Widget build(BuildContext context) {",
		"    final visibleRecords = controller.records;",
		"    final records = controller.records;",
		"    return Scaffold(",
		"      appBar: AppBar(title: Text(openLiteCopy.listPageTitle)),",
		"      body: Column(",
		"        children: [",
		"          Text(openLiteCopy.listCountLabel(visibleRecords.length, records.length)),",
		"          ChoiceChip(",
		"            label: const Text('All'),",
		"            selected: controller.selectedFilter == RecordListFilter.all,",
		"            onSelected: (_) => controller.setFilter(RecordListFilter.all),",
		"          ),",
		"        ],",
		"      ),",
		"    );",
		"  }",
		"}",
	}, "\n") + "\n"

	normalized := normalizeBuilderRuntimeRecordListPageContent(workspacePath, false, true, original)

	for _, want := range []string{
		"class TaskCollectionPage extends StatelessWidget {",
		"import '../controllers/task_collection_controller.dart';",
		"import '../models/task.dart';",
		"required this.onCreateTask,",
		"final TaskCollectionController controller;",
		"final Future<void> Function() onCreateTask;",
		"final Future<void> Function(Task task) onOpenTaskDetail;",
		"onPressed: () => onCreateTask(),",
		"label: Text(openLiteCopy.createPrimaryActionLabel),",
	} {
		if !strings.Contains(normalized, want) {
			t.Fatalf("normalizeBuilderRuntimeRecordListPageContent() missing custom no-filter page marker %q: %q", want, normalized)
		}
	}
	for _, unwanted := range []string{"RecordListFilter", "selectedFilter", "setFilter(", "record.category", "statusLabel(", "class RecordListPage extends StatelessWidget {", "onCreateRecord", "onOpenRecordDetail"} {
		if strings.Contains(normalized, unwanted) {
			t.Fatalf("normalizeBuilderRuntimeRecordListPageContent() should remove stale no-filter page drift %q: %q", unwanted, normalized)
		}
	}
}

func TestNormalizeBuilderRuntimeRecordListPageContentCanonicalizesCustomCollectionPathWithNonListLikeClassName(t *testing.T) {
	workspacePath := createBuilderRuntimeRelationRichWorkspace(t, map[string]string{
		"lib/repositories/task_repository.dart": "abstract class TaskRepository {}\n",
		"lib/controllers/task_collection_controller.dart": strings.Join([]string{
			"import '../models/project.dart';",
			"import '../models/tag.dart';",
			"import '../models/task.dart';",
			"import '../repositories/task_repository.dart';",
			"",
			"class TaskCollectionController {",
			"  TaskCollectionController({required this.taskRepository});",
			"  final TaskRepository taskRepository;",
			"  List<Task> get visibleTasks => const [];",
			"  List<Project> get projects => const [];",
			"  List<Tag> get tags => const [];",
			"  TaskStatus? get selectedFilter => null;",
			"  bool get hasFilter => false;",
			"  void clearFilters() {}",
			"  void setProjectId(String? projectId) {}",
			"  void setTagId(String? tagId) {}",
			"  void setStatus(TaskStatus? status) {}",
			"}",
		}, "\n") + "\n",
		"lib/views/task_collection_page.dart": strings.Join([]string{
			"import 'package:flutter/material.dart';",
			"",
			"import '../controllers/task_collection_controller.dart';",
			"import '../models/task.dart';",
			"",
			"class TasksPage extends StatelessWidget {",
			"  const TasksPage({",
			"    super.key,",
			"    required this.controller,",
			"    required this.onCreateTask,",
			"    required this.onOpenTaskDetail,",
			"  });",
			"",
			"  final TaskCollectionController controller;",
			"  final Future<void> Function() onCreateTask;",
			"  final Future<void> Function(Task task) onOpenTaskDetail;",
			"",
			"  @override",
			"  Widget build(BuildContext context) => const SizedBox.shrink();",
			"}",
		}, "\n") + "\n",
	})

	original := strings.Join([]string{
		"import 'package:flutter/material.dart';",
		"",
		"class TasksPage extends StatelessWidget {",
		"  const TasksPage({super.key});",
		"",
		"  @override",
		"  Widget build(BuildContext context) => const Placeholder();",
		"}",
	}, "\n") + "\n"

	normalized := normalizeBuilderRuntimeRecordListPageContent(workspacePath, true, false, original)

	for _, want := range []string{
		"class TasksPage extends StatelessWidget {",
		"import '../controllers/task_collection_controller.dart';",
		"import '../models/task.dart';",
		"controller.visibleTasks",
		"_RelationRichFilterStrip",
		"_TaskCard",
		"controller.setProjectId(",
		"controller.setTagId(",
		"controller.setStatus(",
	} {
		if !strings.Contains(normalized, want) {
			t.Fatalf("normalizeBuilderRuntimeRecordListPageContent() missing custom path/non-list class marker %q: %q", want, normalized)
		}
	}
	for _, unwanted := range []string{"Widget build(BuildContext context) => const Placeholder();", "class RecordListPage extends StatelessWidget {", "onOpenRecordDetail"} {
		if strings.Contains(normalized, unwanted) {
			t.Fatalf("normalizeBuilderRuntimeRecordListPageContent() should remove stale custom path/non-list class drift %q: %q", unwanted, normalized)
		}
	}
}

func TestNormalizeBuilderRuntimeRecordListPageContentCanonicalizesCustomCollectionViewWithoutListLikePathOrClass(t *testing.T) {
	workspacePath := createBuilderRuntimeRelationRichWorkspace(t, map[string]string{
		"lib/repositories/task_repository.dart": "abstract class TaskRepository {}\n",
		"lib/controllers/task_collection_controller.dart": strings.Join([]string{
			"import '../models/project.dart';",
			"import '../models/tag.dart';",
			"import '../models/task.dart';",
			"import '../repositories/task_repository.dart';",
			"",
			"class TaskCollectionController {",
			"  TaskCollectionController({required this.taskRepository});",
			"  final TaskRepository taskRepository;",
			"  List<Task> get visibleTasks => const [];",
			"  List<Project> get projects => const [];",
			"  List<Tag> get tags => const [];",
			"  TaskStatus? get selectedFilter => null;",
			"  bool get hasFilter => false;",
			"  void clearFilters() {}",
			"  void setProjectId(String? projectId) {}",
			"  void setTagId(String? tagId) {}",
			"  void setStatus(TaskStatus? status) {}",
			"}",
		}, "\n") + "\n",
		"lib/views/task_board_page.dart": strings.Join([]string{
			"import 'package:flutter/material.dart';",
			"",
			"import '../controllers/task_collection_controller.dart';",
			"import '../models/task.dart';",
			"",
			"class TaskBoardPage extends StatelessWidget {",
			"  const TaskBoardPage({",
			"    super.key,",
			"    required this.controller,",
			"    required this.onCreateTask,",
			"    required this.onOpenTaskDetail,",
			"  });",
			"",
			"  final TaskCollectionController controller;",
			"  final Future<void> Function() onCreateTask;",
			"  final Future<void> Function(Task task) onOpenTaskDetail;",
			"",
			"  @override",
			"  Widget build(BuildContext context) => const SizedBox.shrink();",
			"}",
		}, "\n") + "\n",
	})

	original := strings.Join([]string{
		"import 'package:flutter/material.dart';",
		"",
		"class TaskBoardPage extends StatelessWidget {",
		"  const TaskBoardPage({super.key});",
		"",
		"  @override",
		"  Widget build(BuildContext context) => const Placeholder();",
		"}",
	}, "\n") + "\n"

	normalized := normalizeBuilderRuntimeRecordListPageContent(workspacePath, true, false, original)

	for _, want := range []string{
		"class TaskBoardPage extends StatelessWidget {",
		"import '../controllers/task_collection_controller.dart';",
		"import '../models/task.dart';",
		"controller.visibleTasks",
		"_RelationRichFilterStrip",
		"_TaskCard",
		"controller.setProjectId(",
		"controller.setTagId(",
		"controller.setStatus(",
	} {
		if !strings.Contains(normalized, want) {
			t.Fatalf("normalizeBuilderRuntimeRecordListPageContent() missing custom board collection marker %q: %q", want, normalized)
		}
	}
	for _, unwanted := range []string{"Widget build(BuildContext context) => const Placeholder();", "class RecordListPage extends StatelessWidget {", "onOpenRecordDetail"} {
		if strings.Contains(normalized, unwanted) {
			t.Fatalf("normalizeBuilderRuntimeRecordListPageContent() should remove stale custom board collection drift %q: %q", unwanted, normalized)
		}
	}
}

func TestNormalizeBuilderRuntimeRecordListPageContentCanonicalizesRelationRichCustomCollectionPage(t *testing.T) {
	workspacePath := createBuilderRuntimeRelationRichWorkspace(t, map[string]string{
		"lib/repositories/task_repository.dart": "abstract class TaskRepository {}\n",
		"lib/controllers/task_collection_controller.dart": strings.Join([]string{
			"import '../models/project.dart';",
			"import '../models/tag.dart';",
			"import '../models/task.dart';",
			"import '../repositories/task_repository.dart';",
			"",
			"class TaskCollectionController {",
			"  TaskCollectionController({required this.taskRepository});",
			"  final TaskRepository taskRepository;",
			"  List<Task> get visibleTasks => const [];",
			"  List<Project> get projects => const [];",
			"  List<Tag> get tags => const [];",
			"  TaskStatus? get selectedFilter => null;",
			"  bool get hasFilter => false;",
			"  void clearFilters() {}",
			"  void setProjectId(String? projectId) {}",
			"  void setTagId(String? tagId) {}",
			"  void setStatus(TaskStatus? status) {}",
			"}",
		}, "\n") + "\n",
		"lib/views/task_collection_page.dart": strings.Join([]string{
			"import 'package:flutter/material.dart';",
			"",
			"import '../controllers/task_collection_controller.dart';",
			"import '../models/task.dart';",
			"",
			"class TaskCollectionPage extends StatelessWidget {",
			"  const TaskCollectionPage({",
			"    super.key,",
			"    required this.controller,",
			"    required this.onCreateTask,",
			"    required this.onOpenTaskDetail,",
			"  });",
			"",
			"  final TaskCollectionController controller;",
			"  final Future<void> Function() onCreateTask;",
			"  final Future<void> Function(Task task) onOpenTaskDetail;",
			"",
			"  @override",
			"  Widget build(BuildContext context) => const Placeholder();",
			"}",
		}, "\n") + "\n",
	})
	original := strings.Join([]string{
		"import 'package:flutter/material.dart';",
		"",
		"import '../controllers/task_collection_controller.dart';",
		"import '../models/task.dart';",
		"",
		"class TaskCollectionPage extends StatelessWidget {",
		"  const TaskCollectionPage({",
		"    super.key,",
		"    required this.controller,",
		"    required this.onCreateTask,",
		"    required this.onOpenTaskDetail,",
		"  });",
		"",
		"  final TaskCollectionController controller;",
		"  final Future<void> Function() onCreateTask;",
		"  final Future<void> Function(Task task) onOpenTaskDetail;",
		"",
		"  @override",
		"  Widget build(BuildContext context) => const Placeholder();",
		"}",
	}, "\n") + "\n"

	normalized := normalizeBuilderRuntimeRecordListPageContent(workspacePath, true, false, original)

	for _, want := range []string{
		"class TaskCollectionPage extends StatelessWidget {",
		"import '../controllers/task_collection_controller.dart';",
		"import '../models/task.dart';",
		"controller.visibleTasks",
		"_RelationRichFilterStrip",
		"_TaskCard",
		"openLiteCopy.listCountLabel(tasks.length)",
		"controller.setProjectId(",
		"controller.setTagId(",
		"controller.setStatus(",
	} {
		if !strings.Contains(normalized, want) {
			t.Fatalf("normalizeBuilderRuntimeRecordListPageContent() missing relation-rich custom collection marker %q: %q", want, normalized)
		}
	}
	for _, forbidden := range []string{
		"Widget build(BuildContext context) => const Placeholder();",
		"class RecordListPage extends StatelessWidget {",
		"onOpenRecordDetail",
	} {
		if strings.Contains(normalized, forbidden) {
			t.Fatalf("normalizeBuilderRuntimeRecordListPageContent() should replace stale relation-rich custom collection page drift %q: %q", forbidden, normalized)
		}
	}
}

func TestNormalizeBuilderRuntimePatchForWorkspaceCanonicalizesRelationRichModelSlice(t *testing.T) {
	workspacePath := t.TempDir()
	writeBuilderRuntimeWorkspaceFiles(t, workspacePath, map[string]string{
		"lib/main.dart": "void main() {}\n",
	})
	patch := appruns.WorkspacePatch{Operations: []appruns.WorkspacePatchOperation{
		{
			Type: "write_file",
			Path: "lib/models/project.dart",
			Content: strings.Join([]string{
				"import 'package:hive/hive.dart;",
				"",
				"part 'project.g.dart';",
				"",
				"@HiveType(typeId: 0)",
				"class Project extends HiveObject {",
				"  @HiveField(0)",
				"  final String projectId;",
				"}",
			}, "\n") + "\n",
		},
		{
			Type: "write_file",
			Path: "lib/models/task.dart",
			Content: strings.Join([]string{
				"import 'package:hive/hive.dart;",
				"",
				"part 'task.g.dart';",
				"",
				"@HiveType(typeId: 2)",
				"class Task extends HiveObject {",
				"  @HiveField(0)",
				"  final String taskId;",
				"}",
			}, "\n") + "\n",
		},
		{
			Type: "write_file",
			Path: "lib/models/tag.dart",
			Content: strings.Join([]string{
				"import 'package:hive/hive.dart;",
				"",
				"part 'tag.g.dart';",
				"",
				"@HiveType(typeId: 1)",
				"class Tag extends HiveObject {",
				"  @HiveField(0)",
				"  final String tagId;",
				"}",
			}, "\n") + "\n",
		},
		{
			Type: "write_file",
			Path: "lib/models/task_tag_link.dart",
			Content: strings.Join([]string{
				"import 'package:hive/hive.dart;",
				"",
				"part 'task_tag_link.g.dart';",
				"",
				"@HiveType(typeId: 3)",
				"class TaskTagLink extends HiveObject {",
				"  @HiveField(0)",
				"  final String linkId;",
				"}",
			}, "\n") + "\n",
		},
		{
			Type: "write_file",
			Path: "lib/models/dashboard_summary.dart",
			Content: strings.Join([]string{
				"class DashboardSummary {",
				"  const DashboardSummary({required this.projectId, required this.openTaskCount, required this.doneTaskCount, required this.taggedTaskCount});",
				"  final String projectId;",
				"  final int openTaskCount;",
				"  final int doneTaskCount;",
				"  final int taggedTaskCount;",
				"  factory DashboardSummary.fromJson(Map<String, dynamic> json) => DashboardSummary(projectId: json['project_id'] as String, openTaskCount: json['open_task_count'] as int, doneTaskCount: json['done_task_count'] as int, taggedTaskCount: json['tagged_task_count'] as int);",
				"  Map<String, dynamic> toJson() => {'project_id': projectId, 'open_task_count': openTaskCount, 'done_task_count': done_task_count, 'tagged_task_count': taggedTaskCount};",
				"}",
			}, "\n") + "\n",
		},
	}}
	taskBundle := []appruns.TaskBundleItem{{
		TaskID:   "task-create-relation-models",
		TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring,
		TargetPaths: []string{
			"lib/models/project.dart",
			"lib/models/task.dart",
			"lib/models/tag.dart",
			"lib/models/task_tag_link.dart",
			"lib/models/dashboard_summary.dart",
		},
	}}

	normalizeBuilderRuntimePatchForWorkspaceWithTaskBundle(workspacePath, taskBundle, &patch)

	contentByPath := make(map[string]string, len(patch.Operations))
	for _, operation := range patch.Operations {
		contentByPath[operation.Path] = operation.Content
		for _, unwanted := range []string{"package:hive/hive.dart", "@HiveType", "@HiveField", "HiveObject", "part '"} {
			if strings.Contains(operation.Content, unwanted) {
				t.Fatalf("normalizeBuilderRuntimePatchForWorkspaceWithTaskBundle() should remove relation-rich model drift %q from %s: %q", unwanted, operation.Path, operation.Content)
			}
		}
	}
	for path, want := range map[string][]string{
		"lib/models/project.dart":           {"factory Project.fromJson(Map<String, dynamic> json)", "Map<String, dynamic> toJson()", "enum ProjectStatus {"},
		"lib/models/task.dart":              {"factory Task.fromJson(Map<String, dynamic> json)", "'due_on': dueOn?.toIso8601String()", "enum TaskStatus {"},
		"lib/models/tag.dart":               {"factory Tag.fromJson(Map<String, dynamic> json)", "Map<String, dynamic> toJson()"},
		"lib/models/task_tag_link.dart":     {"factory TaskTagLink.fromJson(Map<String, dynamic> json)", "Map<String, dynamic> toJson()"},
		"lib/models/dashboard_summary.dart": {"factory DashboardSummary.fromJson(Map<String, dynamic> json)", "'done_task_count': doneTaskCount", "'tagged_task_count': taggedTaskCount"},
	} {
		content := contentByPath[path]
		if strings.TrimSpace(content) == "" {
			t.Fatalf("normalizeBuilderRuntimePatchForWorkspaceWithTaskBundle() missing canonicalized content for %s", path)
		}
		for _, marker := range want {
			if !strings.Contains(content, marker) {
				t.Fatalf("normalizeBuilderRuntimePatchForWorkspaceWithTaskBundle() missing canonical relation-rich model marker %q in %s: %q", marker, path, content)
			}
		}
	}
}

func TestNormalizeBuilderRuntimePatchForWorkspaceCanonicalizesInventoryRelationRichModelSlice(t *testing.T) {
	workspacePath := t.TempDir()
	writeBuilderRuntimeWorkspaceFiles(t, workspacePath, map[string]string{
		"lib/main.dart": "void main() {}\n",
	})
	patch := appruns.WorkspacePatch{Operations: []appruns.WorkspacePatchOperation{
		{
			Type: "write_file",
			Path: "lib/models/inventory_sheet.dart",
			Content: strings.Join([]string{
				"import 'package:hive/hive.dart;",
				"",
				"part 'inventory_sheet.g.dart';",
				"",
				"@HiveType(typeId: 0)",
				"class InventorySheet extends HiveObject {",
				"  @HiveField(0)",
				"  final String sheetId;",
				"}",
			}, "\n") + "\n",
		},
		{
			Type: "write_file",
			Path: "lib/models/line_item.dart",
			Content: strings.Join([]string{
				"import 'package:hive/hive.dart;",
				"",
				"part 'line_item.g.dart';",
				"",
				"@HiveType(typeId: 1)",
				"class LineItem extends HiveObject {",
				"  @HiveField(0)",
				"  final String lineItemId;",
				"}",
			}, "\n") + "\n",
		},
		{
			Type: "write_file",
			Path: "lib/models/sku.dart",
			Content: strings.Join([]string{
				"import 'package:hive/hive.dart;",
				"",
				"part 'sku.g.dart';",
				"",
				"@HiveType(typeId: 2)",
				"class Sku extends HiveObject {",
				"  @HiveField(0)",
				"  final String skuId;",
				"}",
			}, "\n") + "\n",
		},
		{
			Type: "write_file",
			Path: "lib/models/warehouse.dart",
			Content: strings.Join([]string{
				"import 'package:hive/hive.dart;",
				"",
				"part 'warehouse.g.dart';",
				"",
				"@HiveType(typeId: 3)",
				"class Warehouse extends HiveObject {",
				"  @HiveField(0)",
				"  final String warehouseId;",
				"}",
			}, "\n") + "\n",
		},
	}}
	taskBundle := []appruns.TaskBundleItem{{
		TaskID:   "task-create-relation-models",
		TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring,
		TargetPaths: []string{
			"lib/models/inventory_sheet.dart",
			"lib/models/line_item.dart",
			"lib/models/sku.dart",
			"lib/models/warehouse.dart",
		},
	}}

	normalizeBuilderRuntimePatchForWorkspaceWithTaskBundle(workspacePath, taskBundle, &patch)

	contentByPath := make(map[string]string, len(patch.Operations))
	for _, operation := range patch.Operations {
		contentByPath[operation.Path] = operation.Content
		for _, unwanted := range []string{"package:hive/hive.dart", "@HiveType", "@HiveField", "HiveObject", "part '"} {
			if strings.Contains(operation.Content, unwanted) {
				t.Fatalf("normalizeBuilderRuntimePatchForWorkspaceWithTaskBundle() should remove inventory relation-rich model drift %q from %s: %q", unwanted, operation.Path, operation.Content)
			}
		}
	}
	for path, want := range map[string][]string{
		"lib/models/inventory_sheet.dart": {"factory InventorySheet.fromJson(Map<String, dynamic> json)", "Map<String, dynamic> toJson()", "enum InventorySheetStatus {"},
		"lib/models/line_item.dart":       {"factory LineItem.fromJson(Map<String, dynamic> json)", "Map<String, dynamic> toJson()", "'variance_qty': varianceQty"},
		"lib/models/sku.dart":             {"factory Sku.fromJson(Map<String, dynamic> json)", "Map<String, dynamic> toJson()", "'reorder_threshold': reorderThreshold"},
		"lib/models/warehouse.dart":       {"factory Warehouse.fromJson(Map<String, dynamic> json)", "Map<String, dynamic> toJson()"},
	} {
		content := contentByPath[path]
		if strings.TrimSpace(content) == "" {
			t.Fatalf("normalizeBuilderRuntimePatchForWorkspaceWithTaskBundle() missing canonicalized content for %s", path)
		}
		for _, marker := range want {
			if !strings.Contains(content, marker) {
				t.Fatalf("normalizeBuilderRuntimePatchForWorkspaceWithTaskBundle() missing canonical inventory relation-rich model marker %q in %s: %q", marker, path, content)
			}
		}
	}
}

func TestNormalizeBuilderRuntimePatchForWorkspaceCanonicalizesInventoryRelationRichSummaryModel(t *testing.T) {
	workspacePath := t.TempDir()
	writeBuilderRuntimeWorkspaceFiles(t, workspacePath, map[string]string{
		"lib/main.dart":                   "void main() {}\n",
		"lib/models/inventory_sheet.dart": "class InventorySheet {}\n",
		"lib/models/line_item.dart":       "class LineItem {}\n",
		"lib/models/sku.dart":             "class Sku {}\n",
		"lib/models/warehouse.dart":       "class Warehouse {}\n",
	})
	patch := appruns.WorkspacePatch{Operations: []appruns.WorkspacePatchOperation{{
		Type: "write_file",
		Path: "lib/models/dashboard_summary.dart",
		Content: strings.Join([]string{
			"import 'package:hive/hive.dart;",
			"",
			"part 'dashboard_summary.g.dart';",
			"",
			"@HiveType(typeId: 4)",
			"class DashboardSummary extends HiveObject {",
			"  @HiveField(0)",
			"  final String warehouseId;",
			"}",
		}, "\n") + "\n",
	}}}
	taskBundle := []appruns.TaskBundleItem{{
		TaskID:   "task-create-summary-model",
		TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring,
		TargetPaths: []string{
			"lib/models/dashboard_summary.dart",
		},
	}}

	normalizeBuilderRuntimePatchForWorkspaceWithTaskBundle(workspacePath, taskBundle, &patch)

	content := patch.Operations[0].Content
	for _, unwanted := range []string{"package:hive/hive.dart", "@HiveType", "@HiveField", "HiveObject", "part '"} {
		if strings.Contains(content, unwanted) {
			t.Fatalf("normalizeBuilderRuntimePatchForWorkspaceWithTaskBundle() should remove inventory summary drift %q: %q", unwanted, content)
		}
	}
	for _, marker := range []string{"factory DashboardSummary.fromJson(Map<String, dynamic> json)", "final int lowStockSkuCount;", "'variance_line_item_count': varianceLineItemCount"} {
		if !strings.Contains(content, marker) {
			t.Fatalf("normalizeBuilderRuntimePatchForWorkspaceWithTaskBundle() missing canonical inventory summary marker %q: %q", marker, content)
		}
	}
}

func TestNormalizeBuilderRuntimePatchForWorkspaceCanonicalizesRelationRichRepository(t *testing.T) {
	workspacePath := createBuilderRuntimeRelationRichWorkspace(t, nil)
	patch := appruns.WorkspacePatch{Operations: []appruns.WorkspacePatchOperation{{
		Type: "write_file",
		Path: "lib/repositories/record_repository.dart",
		Content: strings.Join([]string{
			"import 'package:hive_flutter/hive_flutter.dart';",
			"import '../models/project.dart';",
			"import '../models/task.dart';",
			"import '../models/tag.dart';",
			"import '../models/task_tag_link.dart';",
			"import '../models/dashboard_summary.dart';",
			"",
			"abstract class RecordRepository {",
			"  Future<List<TaskTagLink>> loadTaskTagLinks();",
			"  Future<List<DashboardSummary>> loadSummaries();",
			"}",
			"",
			"class HiveRecordRepository extends RecordRepository {",
			"  @toOverride",
			"  Future<List<Tag>> loadTags() async => const [];",
			"}",
			"",
			"class InMemoryRecordRepository extends RecordRepository {",
			"  final List<Dashboard<dynamic>> _summaries = [];",
			"  Future<DashboardSummary?> loadDashboardSummary(String projectId) async => null;",
			"}",
		}, "\n") + "\n",
	}}}
	taskBundle := []appruns.TaskBundleItem{{
		TaskID:      "task-create-repository",
		TaskType:    appruns.BuilderRuntimeTaskTypeDualFileWiring,
		TargetPaths: []string{"lib/repositories/record_repository.dart"},
	}}

	normalizeBuilderRuntimePatchForWorkspaceWithTaskBundle(workspacePath, taskBundle, &patch)

	content := patch.Operations[0].Content
	for _, unwanted := range []string{"@toOverride", "Dashboard<dynamic>", "loadDashboardSummary(", "loadTasksTagLinks("} {
		if strings.Contains(content, unwanted) {
			t.Fatalf("normalizeBuilderRuntimePatchForWorkspaceWithTaskBundle() should remove relation-rich repository drift %q: %q", unwanted, content)
		}
	}
	for _, want := range []string{"class HiveRecordRepository extends RecordRepository", "class InMemoryRecordRepository extends RecordRepository", "Future<List<Project>> loadProjects();", "Future<void> addTaskTagLink(TaskTagLink link) async"} {
		if !strings.Contains(content, want) {
			t.Fatalf("normalizeBuilderRuntimePatchForWorkspaceWithTaskBundle() missing canonical relation-rich repository contract %q: %q", want, content)
		}
	}
}

func TestNormalizeBuilderRuntimePatchForWorkspaceCanonicalizesRelationRichRepositoryRepairImportReplay(t *testing.T) {
	workspacePath := createBuilderRuntimeRelationRichWorkspace(t, nil)
	patch := appruns.WorkspacePatch{Operations: []appruns.WorkspacePatchOperation{{
		Type: "write_file",
		Path: "lib/repositories/record_repository.dart",
		Content: strings.Join([]string{
			"import 'package:hive_flutter/hive_flutter.dart';",
			"",
			"import '../models/dashboard_summary.dart';",
			"import '../models/project.dart';",
			"import '../models/tag.dart';",
			"import '../models/task.dart';",
			"import '../models/task_tag_link.dart';",
			"",
			"abstract class RecordRepository {",
			"  Future<void> init();",
			"  Future<List<Project>> loadProjects();",
			"  Future<void> saveProjects(List<Project> projects);",
			"  Future<List<Task>> loadTasks();",
			"  Future<void> saveTasks(List<Task> tasks);",
			"  Future<List<Tag>> loadTags();",
			"  Future<void> saveTags(List<Tag> tags);",
			"  Future<List<TaskTagLink>> loadTaskTagLinks();",
			"  Future<void> saveTaskTagLinks(List<TaskTagLink> links);",
			"  Future<List<DashboardSummary>> loadSummaries();",
			"  Future<void> saveSummaries(List<DashboardSummary> summaries);",
			"}",
			"",
			"class HiveRecordRepository extends RecordRepository {",
			"  Box<dynamic>? _box;",
			"",
			"  @override",
			"  Future<List<TaskTagLink>> loadTaskTagLinks() async {",
			"    final raw = _safe<dynamic>('task_tag_links', defaultValue: <dynamic>[]) as List<dynamic>;",
			"    return raw.map((item) => TaskTagLink.fromJson(Map<String, dynamic>.from(item as Map))).toList();",
			"  }",
			"",
			"  import 'package:hive_flutter/hive_flutter.dart';",
			"  import '../models/dashboard_summary.dart';",
			"}",
			"",
			"class InMemoryRecordRepository extends RecordRepository {}",
		}, "\n") + "\n",
	}}}
	taskBundle := []appruns.TaskBundleItem{{
		TaskID:      "repair-check-flutter-analyze",
		TaskType:    appruns.BuilderRuntimeTaskTypeAnalyzeRepair,
		TargetPaths: []string{"lib/repositories/record_repository.dart"},
	}}

	normalizeBuilderRuntimePatchForWorkspaceWithTaskBundle(workspacePath, taskBundle, &patch)

	content := patch.Operations[0].Content
	for _, unwanted := range []string{"_safe<dynamic>(", "  import 'package:hive_flutter/hive_flutter.dart';", "  import '../models/dashboard_summary.dart';"} {
		if strings.Contains(content, unwanted) {
			t.Fatalf("normalizeBuilderRuntimePatchForWorkspaceWithTaskBundle() should canonicalize relation-rich repository repair drift %q: %q", unwanted, content)
		}
	}
	for _, want := range []string{"class HiveRecordRepository extends RecordRepository", "class InMemoryRecordRepository extends RecordRepository", "final raw = _safeBox.get(_linksKey, defaultValue: <dynamic>[]) as List<dynamic>;", "await _safeBox.put(_summariesKey, summaries.map((summary) => summary.toJson()).toList());"} {
		if !strings.Contains(content, want) {
			t.Fatalf("normalizeBuilderRuntimePatchForWorkspaceWithTaskBundle() missing canonical relation-rich repository repair marker %q: %q", want, content)
		}
	}
}

func TestNormalizeBuilderRuntimePatchForWorkspaceCanonicalizesInventoryRelationRichRepository(t *testing.T) {
	workspacePath := createBuilderRuntimeInventoryRelationRichWorkspace(t, nil)
	patch := appruns.WorkspacePatch{Operations: []appruns.WorkspacePatchOperation{{
		Type: "write_file",
		Path: "lib/repositories/record_repository.dart",
		Content: strings.Join([]string{
			"import 'package:hive_flutter/hive_flutter.dart';",
			"import '../models/inventory_sheet.dart';",
			"import '../models/line_item.dart';",
			"import '../models/sku.dart';",
			"import '../models/warehouse.dart';",
			"",
			"abstract class RecordRepository {",
			"  Future<List<InventorySheet>> loadInventorySheets();",
			"  Future<DashboardSummary?> loadDashboardSummary(String warehouseId);",
			"}",
			"",
			"class HiveRecordRepository implements RecordRepository {",
			"  Future<List<Sku>> loadSkus() async {",
			"    final raw = _safeBox.get('skus', defaultValue: <dynamic>[]) as List<async>;",
			"    return [];",
			"  }",
			"}",
			"",
			"class InMemoryRecordRepository implements RecordRepository {",
			"  // Implementation omitted for brevity in this patch.",
			"}",
			"",
			"class DashboardSummary {",
			"  final String warehouseId;",
			"  DashboardSummary({required this.warehouseId});",
			"}",
		}, "\n") + "\n",
	}}}
	taskBundle := []appruns.TaskBundleItem{{
		TaskID:      "task-create-repository",
		TaskType:    appruns.BuilderRuntimeTaskTypeDualFileWiring,
		TargetPaths: []string{"lib/repositories/record_repository.dart"},
	}}

	normalizeBuilderRuntimePatchForWorkspaceWithTaskBundle(workspacePath, taskBundle, &patch)

	content := patch.Operations[0].Content
	for _, unwanted := range []string{"List<async>", "class DashboardSummary {", "omitted for brevity", "implements RecordRepository"} {
		if strings.Contains(content, unwanted) {
			t.Fatalf("normalizeBuilderRuntimePatchForWorkspaceWithTaskBundle() should remove inventory relation-rich repository drift %q: %q", unwanted, content)
		}
	}
	for _, want := range []string{"import '../models/dashboard_summary.dart';", "Future<List<InventorySheet>> loadInventorySheets();", "Future<List<DashboardSummary>> loadSummaries();", "Future<DashboardSummary?> loadDashboardSummary(String warehouseId) async {", "class HiveRecordRepository extends RecordRepository", "class InMemoryRecordRepository extends RecordRepository", "DashboardSummary _buildDashboardSummary("} {
		if !strings.Contains(content, want) {
			t.Fatalf("normalizeBuilderRuntimePatchForWorkspaceWithTaskBundle() missing canonical inventory relation-rich repository contract %q: %q", want, content)
		}
	}
}

func TestNormalizeBuilderRuntimePatchForWorkspaceCanonicalizesRelationRichCustomCollectionController(t *testing.T) {
	workspacePath := createBuilderRuntimeRelationRichWorkspace(t, map[string]string{
		"lib/controllers/task_collection_controller.dart": strings.Join([]string{
			"import 'package:flutter/foundation.dart' show ChangeNotifier;",
			"",
			"import '../repositories/record_repository.dart';",
			"",
			"class TaskCollectionController extends ChangeNotifier {",
			"  TaskCollectionController({required RecordRepository repository});",
			"}",
		}, "\n") + "\n",
	})
	patch := appruns.WorkspacePatch{Operations: []appruns.WorkspacePatchOperation{{
		Type: "write_file",
		Path: "lib/controllers/task_collection_controller.dart",
		Content: strings.Join([]string{
			"import 'package:flutter/foundation.dart' show ChangeNotifier;",
			"",
			"import '../models/project.dart';",
			"import '../models/tag.dart';",
			"import '../models/task.dart';",
			"import '../models/task_tag_link.dart';",
			"import '../repositories/record_repository.dart';",
			"",
			"class TaskCollectionController extends ChangeNotifier {",
			"  TaskCollectionController({required RecordRepository repository}) : _repository = repository {",
			"    debugPrint('stale controller');",
			"  }",
			"",
			"  final RecordRepository _repository;",
			"}",
		}, "\n") + "\n",
	}}}
	taskBundle := []appruns.TaskBundleItem{{
		TaskID:      "task-create-list-controller",
		TaskType:    appruns.BuilderRuntimeTaskTypeDualFileWiring,
		TargetPaths: []string{"lib/controllers/task_collection_controller.dart"},
	}}

	normalizeBuilderRuntimePatchForWorkspaceWithTaskBundle(workspacePath, taskBundle, &patch)

	content := patch.Operations[0].Content
	for _, want := range []string{"class TaskCollectionController extends ChangeNotifier", "TaskCollectionController({required RecordRepository repository})", "Future<void> refresh() async => _refresh();"} {
		if !strings.Contains(content, want) {
			t.Fatalf("normalizeBuilderRuntimePatchForWorkspaceWithTaskBundle() missing custom collection controller canonical marker %q: %q", want, content)
		}
	}
	if strings.Contains(content, "class RecordListController") {
		t.Fatalf("normalizeBuilderRuntimePatchForWorkspaceWithTaskBundle() should not fall back to default RecordListController class name for custom controller path: %q", content)
	}
	if strings.Contains(content, "debugPrint('stale controller')") {
		t.Fatalf("normalizeBuilderRuntimePatchForWorkspaceWithTaskBundle() should replace stale custom collection controller drift with canonical content: %q", content)
	}
}

func TestNormalizeBuilderRuntimePatchForWorkspaceCanonicalizesRelationRichCustomCollectionControllerWithCustomRepository(t *testing.T) {
	workspacePath := createBuilderRuntimeRelationRichWorkspace(t, map[string]string{
		"lib/repositories/task_repository.dart": strings.Join([]string{
			"abstract class TaskRepository {",
			"  Future<List<dynamic>> loadProjects();",
			"  Future<List<dynamic>> loadTasks();",
			"  Future<List<dynamic>> loadTags();",
			"  Future<List<dynamic>> loadTaskTagLinks();",
			"}",
		}, "\n") + "\n",
		"lib/controllers/task_collection_controller.dart": strings.Join([]string{
			"import 'package:flutter/foundation.dart' show ChangeNotifier;",
			"",
			"import '../repositories/task_repository.dart';",
			"",
			"class TaskCollectionController extends ChangeNotifier {",
			"  TaskCollectionController({required TaskRepository taskRepository});",
			"}",
		}, "\n") + "\n",
	})
	patch := appruns.WorkspacePatch{Operations: []appruns.WorkspacePatchOperation{{
		Type: "write_file",
		Path: "lib/controllers/task_collection_controller.dart",
		Content: strings.Join([]string{
			"import 'package:flutter/foundation.dart' show ChangeNotifier;",
			"",
			"import '../models/project.dart';",
			"import '../models/tag.dart';",
			"import '../models/task.dart';",
			"import '../models/task_tag_link.dart';",
			"import '../repositories/task_repository.dart';",
			"",
			"class TaskCollectionController extends ChangeNotifier {",
			"  TaskCollectionController({required TaskRepository taskRepository}) : _repository = taskRepository {",
			"    debugPrint('stale controller');",
			"  }",
			"",
			"  final TaskRepository _repository;",
			"}",
		}, "\n") + "\n",
	}}}
	taskBundle := []appruns.TaskBundleItem{{
		TaskID:      "task-create-list-controller",
		TaskType:    appruns.BuilderRuntimeTaskTypeDualFileWiring,
		TargetPaths: []string{"lib/controllers/task_collection_controller.dart"},
	}}

	normalizeBuilderRuntimePatchForWorkspaceWithTaskBundle(workspacePath, taskBundle, &patch)

	content := patch.Operations[0].Content
	for _, want := range []string{"import '../repositories/task_repository.dart';", "class TaskCollectionController extends ChangeNotifier", "TaskCollectionController({required TaskRepository taskRepository})", "final TaskRepository _repository;", "Future<void> refresh() async => _refresh();"} {
		if !strings.Contains(content, want) {
			t.Fatalf("normalizeBuilderRuntimePatchForWorkspaceWithTaskBundle() missing custom repository controller canonical marker %q: %q", want, content)
		}
	}
	if strings.Contains(content, "RecordRepository") {
		t.Fatalf("normalizeBuilderRuntimePatchForWorkspaceWithTaskBundle() should not keep default RecordRepository type for custom repository controller path: %q", content)
	}
	if strings.Contains(content, "debugPrint('stale controller')") {
		t.Fatalf("normalizeBuilderRuntimePatchForWorkspaceWithTaskBundle() should replace stale custom collection controller drift with canonical content: %q", content)
	}
}

func TestNormalizeBuilderRuntimePatchForWorkspaceCanonicalizesExplicitCustomCollectionControllerWithDefaultNoisePresent(t *testing.T) {
	workspacePath := createBuilderRuntimeRelationRichWorkspace(t, map[string]string{
		"lib/repositories/task_repository.dart": strings.Join([]string{
			"abstract class TaskRepository {",
			"  Future<List<dynamic>> loadProjects();",
			"  Future<List<dynamic>> loadTasks();",
			"  Future<List<dynamic>> loadTags();",
			"  Future<List<dynamic>> loadTaskTagLinks();",
			"}",
		}, "\n") + "\n",
		"lib/controllers/record_list_controller.dart": strings.Join([]string{
			"import '../repositories/task_repository.dart';",
			"class RecordListController {",
			"  RecordListController({required TaskRepository repository});",
			"}",
		}, "\n") + "\n",
		"lib/controllers/task_collection_controller.dart": strings.Join([]string{
			"import 'package:flutter/foundation.dart' show ChangeNotifier;",
			"",
			"import '../repositories/task_repository.dart';",
			"",
			"class TaskCollectionController extends ChangeNotifier {",
			"  TaskCollectionController({required TaskRepository taskRepository});",
			"}",
		}, "\n") + "\n",
	})
	patch := appruns.WorkspacePatch{Operations: []appruns.WorkspacePatchOperation{{
		Type: "write_file",
		Path: "lib/controllers/task_collection_controller.dart",
		Content: strings.Join([]string{
			"import 'package:flutter/foundation.dart' show ChangeNotifier;",
			"",
			"import '../models/project.dart';",
			"import '../models/tag.dart';",
			"import '../models/task.dart';",
			"import '../models/task_tag_link.dart';",
			"import '../repositories/task_repository.dart';",
			"",
			"class TaskCollectionController extends ChangeNotifier {",
			"  TaskCollectionController({required TaskRepository taskRepository}) : _repository = taskRepository {",
			"    debugPrint('stale controller with default noise');",
			"  }",
			"",
			"  final TaskRepository _repository;",
			"}",
		}, "\n") + "\n",
	}}}
	taskBundle := []appruns.TaskBundleItem{{
		TaskID:      "task-create-list-controller",
		TaskType:    appruns.BuilderRuntimeTaskTypeDualFileWiring,
		TargetPaths: []string{"lib/controllers/task_collection_controller.dart"},
	}}

	normalizeBuilderRuntimePatchForWorkspaceWithTaskBundle(workspacePath, taskBundle, &patch)

	content := patch.Operations[0].Content
	for _, want := range []string{"import '../repositories/task_repository.dart';", "class TaskCollectionController extends ChangeNotifier", "TaskCollectionController({required TaskRepository taskRepository})", "final TaskRepository _repository;", "Future<void> refresh() async => _refresh();"} {
		if !strings.Contains(content, want) {
			t.Fatalf("normalizeBuilderRuntimePatchForWorkspaceWithTaskBundle() missing explicit custom collection controller canonical marker %q with default noise present: %q", want, content)
		}
	}
	if strings.Contains(content, "debugPrint('stale controller with default noise')") {
		t.Fatalf("normalizeBuilderRuntimePatchForWorkspaceWithTaskBundle() should canonicalize explicit custom collection controller target even when default RecordListController noise exists: %q", content)
	}
	if strings.Contains(content, "class RecordListController") {
		t.Fatalf("normalizeBuilderRuntimePatchForWorkspaceWithTaskBundle() should not drift back to default RecordListController when explicit custom controller target is present: %q", content)
	}
}

func TestNormalizeBuilderRuntimeRecordListControllerContentCanonicalizesNoFilterCustomCollectionController(t *testing.T) {
	workspacePath := t.TempDir()
	content := strings.Join([]string{
		"import 'package:flutter/foundation.dart' show ChangeNotifier;",
		"",
		"import '../repositories/task_repository.dart';",
		"",
		"class TaskCollectionController extends ChangeNotifier {",
		"  TaskCollectionController({required TaskRepository taskRepository}) : _repository = taskRepository;",
		"",
		"  final TaskRepository _repository;",
		"  Object? _selectedFilter;",
		"",
		"  void setFilter(Object? filter) {",
		"    _selectedFilter = filter;",
		"  }",
		"}",
	}, "\n") + "\n"
	writeBuilderRuntimeWorkspaceFiles(t, workspacePath, map[string]string{
		"lib/models/record.dart":                          "class TodoItem { const TodoItem({required this.title, required this.category, this.note = ''}); final String title; final String category; final String note; }\n",
		"lib/controllers/task_collection_controller.dart": content,
		"lib/repositories/task_repository.dart": strings.Join([]string{
			"abstract class TaskRepository {",
			"  Future<List<TodoItem>> loadRecords();",
			"  Future<void> addRecord(TodoItem record);",
			"  Future<void> updateRecord(TodoItem record);",
			"}",
		}, "\n") + "\n",
	})

	normalized := normalizeBuilderRuntimeRecordListControllerContent(workspacePath, false, false, content)
	for _, forbidden := range []string{"_selectedFilter", "setFilter(", "Object? _selectedFilter"} {
		if strings.Contains(normalized, forbidden) {
			t.Fatalf("normalizeBuilderRuntimeRecordListControllerContent() should remove no-filter drift %q from custom collection controller: %q", forbidden, normalized)
		}
	}
	for _, want := range []string{
		"import '../repositories/task_repository.dart';",
		"class TaskCollectionController extends ChangeNotifier {",
		"TaskCollectionController({required TaskRepository taskRepository})",
		"final TaskRepository _repository;",
		"Future<void> refresh() async {",
		"Future<void> addRecord(TodoItem record) async {",
		"Future<void> updateRecord(TodoItem record) async {",
	} {
		if !strings.Contains(normalized, want) {
			t.Fatalf("normalizeBuilderRuntimeRecordListControllerContent() missing no-filter canonical marker %q: %q", want, normalized)
		}
	}
	if strings.Contains(normalized, "class RecordListController") {
		t.Fatalf("normalizeBuilderRuntimeRecordListControllerContent() should keep custom collection controller class name in no-filter topology: %q", normalized)
	}
}

func TestNormalizeBuilderRuntimeHomeControllerContentRestoresDebugPrintImportForRelationRichController(t *testing.T) {
	workspacePath := createBuilderRuntimeRelationRichWorkspace(t, nil)
	content := strings.Join([]string{
		"import 'package:flutter/foundation.dart' show ChangeNotifier;",
		"",
		"import '../models/dashboard_summary.dart';",
		"import '../models/project.dart';",
		"import '../repositories/record_repository.dart';",
		"",
		"class HomeController extends ChangeNotifier {",
		"  HomeController({required RecordRepository repository}) : _repository = repository;",
		"",
		"  final RecordRepository _repository;",
		"  bool _isLoading = true;",
		"  List<Project> _projects = const [];",
		"  List<DashboardSummary> _summaries = const [];",
		"",
		"  bool get isLoading => _isLoading;",
		"  List<Project> get projects => List.unmodifiable(_projects);",
		"  List<DashboardSummary> get summaries => List.unmodifiable(_summaries);",
		"",
		"  Future<void> initialize() async {",
		"    await refresh();",
		"  }",
		"",
		"  Future<void> refresh() async {",
		"    try {",
		"      final projects = await _repository.loadProjects();",
		"      final summaries = await _repository.loadSummaries();",
		"      _projects = projects;",
		"      _summaries = summaries;",
		"    } catch (error) {",
		"      debugPrint('Failed to refresh relation-rich home dashboard: $error');",
		"    } finally {",
		"      _isLoading = false;",
		"      notifyListeners();",
		"    }",
		"  }",
		"",
		"  DashboardSummary? getSummaryForProject(String projectId) {",
		"    for (final summary in _summaries) {",
		"      if (summary.projectId == projectId) {",
		"        return summary;",
		"      }",
		"    }",
		"    return null;",
		"  }",
		"}",
	}, "\n") + "\n"

	normalized := normalizeBuilderRuntimeHomeControllerContent(workspacePath, content)

	if !strings.Contains(normalized, "import 'package:flutter/foundation.dart' show ChangeNotifier, debugPrint;") {
		t.Fatalf("normalizeBuilderRuntimeHomeControllerContent() should restore debugPrint import when relation-rich controller still calls debugPrint: %q", normalized)
	}
	if !strings.Contains(normalized, "debugPrint('Failed to refresh relation-rich home dashboard: $error');") {
		t.Fatalf("normalizeBuilderRuntimeHomeControllerContent() should preserve debugPrint call after import repair: %q", normalized)
	}
}

func TestNormalizeBuilderRuntimePatchForWorkspaceCanonicalizesRelationRichCustomHomeControllerWithCustomRepository(t *testing.T) {
	workspacePath := createBuilderRuntimeRelationRichWorkspace(t, map[string]string{
		"lib/repositories/task_repository.dart": strings.Join([]string{
			"abstract class TaskRepository {",
			"  Future<List<dynamic>> loadProjects();",
			"  Future<List<dynamic>> loadSummaries();",
			"}",
		}, "\n") + "\n",
	})
	stale := strings.Join([]string{
		"import 'package:flutter/foundation.dart' show ChangeNotifier;",
		"",
		"import '../models/dashboard_summary.dart';",
		"import '../models/project.dart';",
		"import '../repositories/task_repository.dart';",
		"",
		"class TaskHomeController extends ChangeNotifier {",
		"  TaskHomeController({required TaskRepository taskRepository}) : _repository = taskRepository;",
		"",
		"  final TaskRepository _repository;",
		"",
		"  Future<void> refresh() async {",
		"    debugPrint('stale home controller');",
		"  }",
		"}",
	}, "\n") + "\n"
	patch := appruns.WorkspacePatch{Operations: []appruns.WorkspacePatchOperation{{
		Type:    "write_file",
		Path:    "lib/controllers/task_home_controller.dart",
		Content: stale,
	}}}
	taskBundle := []appruns.TaskBundleItem{{
		TaskID:      "task-create-home-controller",
		TaskType:    appruns.BuilderRuntimeTaskTypeDualFileWiring,
		TargetPaths: []string{"lib/controllers/task_home_controller.dart"},
	}}

	normalizeBuilderRuntimePatchForWorkspaceWithTaskBundle(workspacePath, taskBundle, &patch)

	content := patch.Operations[0].Content
	for _, want := range []string{"import '../repositories/task_repository.dart';", "class TaskHomeController extends ChangeNotifier", "TaskHomeController({required TaskRepository taskRepository}) : _repository = taskRepository;", "final TaskRepository _repository;", "debugPrint('Failed to refresh relation-rich home dashboard: $error');"} {
		if !strings.Contains(content, want) {
			t.Fatalf("normalizeBuilderRuntimePatchForWorkspaceWithTaskBundle() missing custom home controller canonical marker %q: %q", want, content)
		}
	}
	if strings.Contains(content, "RecordRepository") {
		t.Fatalf("normalizeBuilderRuntimePatchForWorkspaceWithTaskBundle() should not keep default RecordRepository type for custom home controller path: %q", content)
	}
	if strings.Contains(content, "debugPrint('stale home controller')") {
		t.Fatalf("normalizeBuilderRuntimePatchForWorkspaceWithTaskBundle() should replace stale custom home controller drift with canonical content: %q", content)
	}
}

func TestNormalizeBuilderRuntimePatchForWorkspaceCanonicalizesRelationRichCustomOverviewControllerWithCustomRepository(t *testing.T) {
	workspacePath := createBuilderRuntimeRelationRichWorkspace(t, map[string]string{
		"lib/repositories/task_repository.dart": strings.Join([]string{
			"abstract class TaskRepository {",
			"  Future<List<dynamic>> loadProjects();",
			"  Future<List<dynamic>> loadSummaries();",
			"}",
		}, "\n") + "\n",
	})
	stale := strings.Join([]string{
		"import 'package:flutter/foundation.dart' show ChangeNotifier;",
		"",
		"import '../models/dashboard_summary.dart';",
		"import '../models/project.dart';",
		"import '../repositories/task_repository.dart';",
		"",
		"class TaskOverviewController extends ChangeNotifier {",
		"  TaskOverviewController({required TaskRepository taskRepository}) : _repository = taskRepository;",
		"",
		"  final TaskRepository _repository;",
		"",
		"  Future<void> refresh() async {",
		"    debugPrint('stale overview controller');",
		"  }",
		"}",
	}, "\n") + "\n"
	patch := appruns.WorkspacePatch{Operations: []appruns.WorkspacePatchOperation{{
		Type:    "write_file",
		Path:    "lib/controllers/task_overview_controller.dart",
		Content: stale,
	}}}
	taskBundle := []appruns.TaskBundleItem{{
		TaskID:      "task-create-overview-controller",
		TaskType:    appruns.BuilderRuntimeTaskTypeDualFileWiring,
		TargetPaths: []string{"lib/controllers/task_overview_controller.dart"},
	}}

	normalizeBuilderRuntimePatchForWorkspaceWithTaskBundle(workspacePath, taskBundle, &patch)

	content := patch.Operations[0].Content
	for _, want := range []string{"import '../repositories/task_repository.dart';", "class TaskOverviewController extends ChangeNotifier", "TaskOverviewController({required TaskRepository taskRepository}) : _repository = taskRepository;", "final TaskRepository _repository;", "debugPrint('Failed to refresh relation-rich home dashboard: $error');"} {
		if !strings.Contains(content, want) {
			t.Fatalf("normalizeBuilderRuntimePatchForWorkspaceWithTaskBundle() missing custom overview controller canonical marker %q: %q", want, content)
		}
	}
	if strings.Contains(content, "RecordRepository") {
		t.Fatalf("normalizeBuilderRuntimePatchForWorkspaceWithTaskBundle() should not keep default RecordRepository type for custom overview controller path: %q", content)
	}
	if strings.Contains(content, "debugPrint('stale overview controller')") {
		t.Fatalf("normalizeBuilderRuntimePatchForWorkspaceWithTaskBundle() should replace stale custom overview controller drift with canonical content: %q", content)
	}
}

func TestNormalizeBuilderRuntimePatchForWorkspaceCanonicalizesRelationRichCustomOverviewPageWithCustomController(t *testing.T) {
	workspacePath := createBuilderRuntimeRelationRichWorkspace(t, map[string]string{
		"lib/repositories/task_repository.dart": strings.Join([]string{
			"abstract class TaskRepository {",
			"  Future<List<dynamic>> loadProjects();",
			"  Future<List<dynamic>> loadSummaries();",
			"}",
		}, "\n") + "\n",
		"lib/controllers/task_overview_controller.dart": strings.Join([]string{
			"class TaskOverviewController extends ChangeNotifier {",
			"  TaskOverviewController({required this.taskRepository});",
			"  final TaskRepository taskRepository;",
			"}",
		}, "\n") + "\n",
	})
	stale := strings.Join([]string{
		"import 'package:flutter/material.dart';",
		"",
		"import '../controllers/task_overview_controller.dart';",
		"import '../models/dashboard_summary.dart';",
		"import '../models/project.dart';",
		"import '../template/open_lite_copy.dart';",
		"",
		"class TaskOverviewPage extends StatelessWidget {",
		"  const TaskOverviewPage({required this.controller, required this.onCreateTask, required this.onViewAllTasks});",
		"",
		"  final TaskOverviewController controller;",
		"  final Future<void> Function() onCreateTask;",
		"  final Future<void> Function() onViewAllTasks;",
		"",
		"  @override",
		"  Widget build(BuildContext context) {",
		"    return const Text('stale overview page');",
		"  }",
		"}",
	}, "\n") + "\n"
	patch := appruns.WorkspacePatch{Operations: []appruns.WorkspacePatchOperation{{
		Type:    "write_file",
		Path:    "lib/views/task_overview_page.dart",
		Content: stale,
	}}}
	taskBundle := []appruns.TaskBundleItem{{
		TaskID:      "task-bind-overview-surface",
		TaskType:    appruns.BuilderRuntimeTaskTypeDualFileWiring,
		TargetPaths: []string{"lib/views/task_overview_page.dart"},
	}}

	normalizeBuilderRuntimePatchForWorkspaceWithTaskBundle(workspacePath, taskBundle, &patch)

	content := patch.Operations[0].Content
	for _, want := range []string{"import '../controllers/task_overview_controller.dart';", "class TaskOverviewPage extends StatelessWidget", "final TaskOverviewController controller;", "final Future<void> Function() onCreateTask;", "final Future<void> Function() onViewAllTasks;", "_OverviewActions(", "openLiteCopy.homeSummaryTitle"} {
		if !strings.Contains(content, want) {
			t.Fatalf("normalizeBuilderRuntimePatchForWorkspaceWithTaskBundle() missing custom overview page canonical marker %q: %q", want, content)
		}
	}
	if strings.Contains(content, "class HomePage extends StatelessWidget") {
		t.Fatalf("normalizeBuilderRuntimePatchForWorkspaceWithTaskBundle() should not keep default HomePage class for custom overview page path: %q", content)
	}
	if strings.Contains(content, "final HomeController controller;") {
		t.Fatalf("normalizeBuilderRuntimePatchForWorkspaceWithTaskBundle() should not keep default HomeController field for custom overview page path: %q", content)
	}
	if strings.Contains(content, "stale overview page") {
		t.Fatalf("normalizeBuilderRuntimePatchForWorkspaceWithTaskBundle() should replace stale custom overview page drift with canonical content: %q", content)
	}
}

func TestNormalizeBuilderRuntimePatchForWorkspaceCanonicalizesRelationRichCustomDetailPage(t *testing.T) {
	workspacePath := createBuilderRuntimeRelationRichWorkspace(t, map[string]string{
		"lib/views/task_detail_page.dart": strings.Join([]string{
			"import 'package:flutter/material.dart';",
			"",
			"import '../models/project.dart';",
			"import '../models/tag.dart';",
			"import '../models/task.dart';",
			"import '../template/open_lite_copy.dart';",
			"",
			"class TaskDetailPage extends StatelessWidget {",
			"  const TaskDetailPage({required this.task, required this.projects, required this.tags, required this.taskTags, required this.onEdit});",
			"",
			"  final Task task;",
			"  final List<Project> projects;",
			"  final List<Tag> tags;",
			"  final List<String> taskTags;",
			"  final Future<void> Function(Task task) onEdit;",
			"",
			"  @override",
			"  Widget build(BuildContext context) {",
			"    return const Text('stale detail page');",
			"  }",
			"}",
		}, "\n") + "\n",
	})
	stale := strings.Join([]string{
		"import 'package:flutter/material.dart';",
		"",
		"import '../models/project.dart';",
		"import '../models/tag.dart';",
		"import '../models/task.dart';",
		"import '../template/open_lite_copy.dart';",
		"",
		"class TaskDetailPage extends StatelessWidget {",
		"  const TaskDetailPage({required this.task, required this.projects, required this.tags, required this.taskTags, required this.onEdit});",
		"",
		"  final Task task;",
		"  final List<Project> projects;",
		"  final List<Tag> tags;",
		"  final List<String> taskTags;",
		"  final Future<void> Function(Task task) onEdit;",
		"",
		"  @override",
		"  Widget build(BuildContext context) {",
		"    return const Text('stale detail page');",
		"  }",
		"}",
	}, "\n") + "\n"
	patch := appruns.WorkspacePatch{Operations: []appruns.WorkspacePatchOperation{{
		Type:    "write_file",
		Path:    "lib/views/task_detail_page.dart",
		Content: stale,
	}}}
	taskBundle := []appruns.TaskBundleItem{{
		TaskID:      "task-bind-detail-surface",
		TaskType:    appruns.BuilderRuntimeTaskTypeDualFileWiring,
		TargetPaths: []string{"lib/views/task_detail_page.dart"},
	}}

	normalizeBuilderRuntimePatchForWorkspaceWithTaskBundle(workspacePath, taskBundle, &patch)

	content := patch.Operations[0].Content
	for _, want := range []string{"class TaskDetailPage extends StatefulWidget", "State<TaskDetailPage> createState()", "final Task task;", "final List<Project> projects;", "final List<Tag> tags;", "final List<String> taskTags;", "final Future<void> Function(Task task) onEdit;", "openLiteCopy.detailPageTitle"} {
		if !strings.Contains(content, want) {
			t.Fatalf("normalizeBuilderRuntimePatchForWorkspaceWithTaskBundle() missing custom detail page canonical marker %q: %q", want, content)
		}
	}
	if strings.Contains(content, "class RecordDetailPage extends StatefulWidget") {
		t.Fatalf("normalizeBuilderRuntimePatchForWorkspaceWithTaskBundle() should not keep default RecordDetailPage class for custom detail path: %q", content)
	}
	if strings.Contains(content, "stale detail page") {
		t.Fatalf("normalizeBuilderRuntimePatchForWorkspaceWithTaskBundle() should replace stale custom detail page drift with canonical content: %q", content)
	}
}

func TestNormalizeBuilderRuntimePatchForWorkspaceCanonicalizesRelationRichCustomDetailPageAliasConstructorContract(t *testing.T) {
	workspacePath := createBuilderRuntimeRelationRichWorkspace(t, map[string]string{
		"lib/views/task_detail_page.dart": strings.Join([]string{
			"import 'package:flutter/material.dart';",
			"",
			"import '../models/project.dart';",
			"import '../models/tag.dart';",
			"import '../models/task.dart';",
			"import '../template/open_lite_copy.dart';",
			"",
			"class TaskDetailPage extends StatelessWidget {",
			"  const TaskDetailPage({required this.task, required this.availableProjects, required this.availableTags, required this.linkedTaskTags, required this.onEdit});",
			"",
			"  final Task task;",
			"  final List<Project> availableProjects;",
			"  final List<Tag> availableTags;",
			"  final List<String> linkedTaskTags;",
			"  final Future<void> Function(Task task) onEdit;",
			"",
			"  @override",
			"  Widget build(BuildContext context) {",
			"    return const Text('stale detail alias page');",
			"  }",
			"}",
		}, "\n") + "\n",
	})
	patch := appruns.WorkspacePatch{Operations: []appruns.WorkspacePatchOperation{{
		Type: "write_file",
		Path: "lib/views/task_detail_page.dart",
		Content: strings.Join([]string{
			"import 'package:flutter/material.dart';",
			"",
			"import '../models/project.dart';",
			"import '../models/tag.dart';",
			"import '../models/task.dart';",
			"import '../template/open_lite_copy.dart';",
			"",
			"class TaskDetailPage extends StatelessWidget {",
			"  const TaskDetailPage({required this.task, required this.availableProjects, required this.availableTags, required this.linkedTaskTags, required this.onEdit});",
			"",
			"  final Task task;",
			"  final List<Project> availableProjects;",
			"  final List<Tag> availableTags;",
			"  final List<String> linkedTaskTags;",
			"  final Future<void> Function(Task task) onEdit;",
			"",
			"  @override",
			"  Widget build(BuildContext context) {",
			"    return const Text('stale detail alias page');",
			"  }",
			"}",
		}, "\n") + "\n",
	}}}
	taskBundle := []appruns.TaskBundleItem{{
		TaskID:      "task-bind-detail-surface",
		TaskType:    appruns.BuilderRuntimeTaskTypeDualFileWiring,
		TargetPaths: []string{"lib/views/task_detail_page.dart"},
	}}

	normalizeBuilderRuntimePatchForWorkspaceWithTaskBundle(workspacePath, taskBundle, &patch)

	content := patch.Operations[0].Content
	for _, want := range []string{"required this.availableProjects", "required this.availableTags", "required this.linkedTaskTags", "final List<Project> availableProjects;", "final List<Tag> availableTags;", "final List<String> linkedTaskTags;", "widget.availableProjects", "widget.availableTags", "widget.linkedTaskTags"} {
		if !strings.Contains(content, want) {
			t.Fatalf("normalizeBuilderRuntimePatchForWorkspaceWithTaskBundle() missing alias detail constructor marker %q: %q", want, content)
		}
	}
	if strings.Contains(content, "required this.projects") || strings.Contains(content, "required this.tags") || strings.Contains(content, "required this.taskTags") {
		t.Fatalf("normalizeBuilderRuntimePatchForWorkspaceWithTaskBundle() should not keep default detail constructor names when alias contract is active: %q", content)
	}
	if strings.Contains(content, "stale detail alias page") {
		t.Fatalf("normalizeBuilderRuntimePatchForWorkspaceWithTaskBundle() should replace stale alias detail page drift with canonical content: %q", content)
	}
}

func TestNormalizeBuilderRuntimeRecordListPageContentDropsUnusedRelationRichImports(t *testing.T) {
	workspacePath := createBuilderRuntimeRelationRichWorkspace(t, nil)
	content := strings.Join([]string{
		"import 'package:flutter/material.dart';",
		"",
		"import '../controllers/record_list_controller.dart';",
		"import '../models/tag.dart';",
		"import '../models/task.dart';",
		"import '../template/open_lite_copy.dart';",
		"",
		"class RecordListPage extends StatelessWidget {",
		"  const RecordListPage({super.key, required this.controller, required this.onCreateRecord, required this.onOpenTaskDetail});",
		"",
		"  final RecordListController controller;",
		"  final Future<void> Function() onCreateRecord;",
		"  final Future<void> Function(Task task) onOpenTaskDetail;",
		"",
		"  @override",
		"  Widget build(BuildContext context) {",
		"    final task = controller.visibleTasks.first;",
		"    return Scaffold(",
		"      appBar: AppBar(title: Text(openLiteCopy.listPageTitle)),",
		"      body: ListTile(",
		"        title: Text(task.title),",
		"        onTap: () => onOpenTaskDetail(task),",
		"      ),",
		"    );",
		"  }",
		"}",
	}, "\n") + "\n"

	normalized := normalizeBuilderRuntimeRecordListPageContent(workspacePath, true, false, content)

	if strings.Contains(normalized, "import '../models/tag.dart';") {
		t.Fatalf("normalizeBuilderRuntimeRecordListPageContent() should drop unused relation-rich tag import: %q", normalized)
	}
	if !strings.Contains(normalized, "import '../models/task.dart';") {
		t.Fatalf("normalizeBuilderRuntimeRecordListPageContent() should preserve task import when Task is still referenced: %q", normalized)
	}
}

func TestNormalizeBuilderRuntimeWidgetTestContentPreservesRelationRichImports(t *testing.T) {
	workspacePath := createBuilderRuntimeRelationRichWorkspace(t, nil)
	content := strings.Join([]string{
		"import 'package:flutter/material.dart';",
		"import 'package:flutter_test/flutter_test.dart';",
		"import 'package:flutter_open_lite/main.dart';",
		"import 'package:flutter_open_lite/repositories/record_repository.dart';",
		"import 'package:flutter_open_lite/models/project.dart';",
		"import 'package:flutter_open_lite/models/task.dart';",
		"import 'package:flutter_open_lite/template/open_lite_copy.dart';",
		"",
		"void main() {",
		"  testWidgets('relation-rich smoke', (WidgetTester tester) async {",
		"    final repository = InMemoryRecordRepository(",
		"      projects: [Project(projectId: 'p1', title: 'Project Alpha', status: ProjectStatus.active)],",
		"    );",
		"    await tester.pumpWidget(AppFactoryApp(repository: repository));",
		"    await tester.pumpAndSettle();",
		"    expect(find.text(openLiteCopy.appTitle), findsOneWidget);",
		"  });",
		"}",
	}, "\n") + "\n"

	normalized := normalizeBuilderRuntimeWidgetTestContent(workspacePath, false, content)

	if normalized != content {
		t.Fatalf("normalizeBuilderRuntimeWidgetTestContent() should preserve relation-rich widget test content once canonicalization is removed:\nwant:\n%s\n got:\n%s", content, normalized)
	}
}

func TestNormalizeBuilderRuntimeWidgetTestContentDoesNotCanonicalizeRelationRichSmokeFlow(t *testing.T) {
	workspacePath := createBuilderRuntimeRelationRichWorkspace(t, map[string]string{
		"lib/main.dart": strings.Join([]string{
			"import 'package:flutter/material.dart';",
			"",
			"import 'repositories/record_repository.dart';",
			"",
			"void main() {",
			"  final repository = InMemoryRecordRepository();",
			"  runApp(AppFactoryApp(repository: repository));",
			"}",
			"",
			"class AppFactoryApp extends StatelessWidget {",
			"  const AppFactoryApp({super.key, required this.repository});",
			"",
			"  final RecordRepository repository;",
			"",
			"  @override",
			"  Widget build(BuildContext context) {",
			"    return const MaterialApp(home: SizedBox.shrink());",
			"  }",
			"}",
		}, "\n") + "\n",
		"lib/repositories/record_repository.dart": builderRuntimeCanonicalRelationRichRecordRepository(builderRuntimeRelationRichProjectTaskTagProfile),
	})
	content := strings.Join([]string{
		"import 'package:flutter/material.dart';",
		"import 'package:flutter_test/flutter_test.dart';",
		"import 'package:flutter_open_lite/main.dart';",
		"import 'package:flutter_open_lite/repositories/record_repository.dart';",
		"import 'package:flutter_open_lite/models/project.dart';",
		"import 'package:flutter_open_lite/models/task.dart';",
		"import 'package:flutter_open_lite/models/tag.dart';",
		"import 'package:flutter_open_lite/models/task_tag_link.dart';",
		"import 'package:flutter_open_lite/models/dashboard_summary.dart';",
		"",
		"void main() {",
		"  late InMemoryRecordRepository repository;",
		"",
		"  setUp(() {",
		"    repository = InMemoryRecordRepository(",
		"      projects: [Project(projectId: 'p1', title: 'Project 1', status: ProjectStatus.active)],",
		"      tasks: [Task(taskId: 't1', projectId: 'p1', title: 'Task 1', status: TaskStatus.todo)],",
		"      tags: [Tag(tagId: 'tag1', name: 'Tag 1')],",
		"      links: [TaskTagLink(linkId: 'l1', taskId: 't1', tagId: 'tag1')],",
		"      summaries: [DashboardSummary(projectId: 'p1', openTaskCount: 1, doneTaskCount: 0, taggedTaskCount: 1)],",
		"    );",
		"  });",
		"",
		"  testWidgets('validate /jobs regression bookkeeping demo flow', (WidgetTester tester) async {",
		"    await tester.pumpWidget(AppFactoryApp(repository: repository));",
		"    await tester.pumpAndSettle();",
		"    expect(find.text('/jobs regression bookkeeping demo'), findsNothing);",
		"    await tester.tap(find.text('查看任务列表'));",
		"    await tester.pumpAnd/Settle();",
		"    await tester.tap(find.text('#Tag 1'));",
		"    await tester.pumpAndSettle();",
		"    await tester.enterText(find.byKey(const Key('title-field')), 'New Task');",
		"  });",
		"}",
	}, "\n") + "\n"

	normalized := normalizeBuilderRuntimeWidgetTestContent(workspacePath, false, content)

	if normalized != content {
		t.Fatalf("normalizeBuilderRuntimeWidgetTestContent() should leave relation-rich smoke drift untouched after canonicalization removal:\nwant:\n%s\n got:\n%s", content, normalized)
	}
}

func TestNormalizeBuilderRuntimeWidgetTestContentDoesNotCanonicalizeInventoryRelationRichSmoke(t *testing.T) {
	workspacePath := createBuilderRuntimeInventoryRelationRichWorkspace(t, nil)
	content := strings.Join([]string{
		"import 'package:flutter/material.dart';",
		"import 'package:flutter_test/flutter_test.dart';",
		"import 'package:flutter_open_lite/main.dart';",
		"import 'package:flutter_open_lite/repositories/record_repository.dart';",
		"",
		"void main() {",
		"  testWidgets('inventory flow supports overview, list filtering, and line item maintenance', (WidgetTester tester) async {",
		"    final repository = InMemoryRecordRepository(",
		"      warehouses: [Warehouse(warehouseId: 'wh-1', name: '仓库 A', location: '位置 A')],",
		"      skus: [Sku(skuId: 'sku-1', name: 'SKU 1', category: '分类 1', reorderThreshold: 10)],",
		"    );",
		"",
		"    await tester.pumpWidget(AppFactoryApp(repository: repository));",
		"    await tester.pumpAndSettle();",
		"",
		"    expect(find.text('库存看板摘要'), findsOneWidget);",
		"    await tester.tap(find.text('查看全部'));",
		"    await tester.pumpAndSettle();",
		"  });",
		"}",
		"",
		"extension WidgetTesterX on WidgetTester {}",
	}, "\n") + "\n"

	normalized := normalizeBuilderRuntimeWidgetTestContent(workspacePath, false, content)

	if normalized != content {
		t.Fatalf("normalizeBuilderRuntimeWidgetTestContent() should leave inventory relation-rich smoke drift untouched after canonicalization removal:\nwant:\n%s\n got:\n%s", content, normalized)
	}
}

func TestNormalizeBuilderRuntimeViewContentRepairsOpenLiteCopyHelperTypos(t *testing.T) {
	workspacePath := createBuilderRuntimeInventoryRelationRichWorkspace(t, nil)
	content := strings.Join([]string{
		"import 'package:flutter/material.dart';",
		"",
		"import '../models/inventory_sheet.dart';",
		"import '../models/line_item.dart';",
		"import '../models/sku.dart';",
		"import '../models/warehouse.dart';",
		"import '../template/open_lite_copy.dart';",
		"",
		"class RecordDetailPage extends StatelessWidget {",
		"  const RecordDetailPage({",
		"    super.key,",
		"    required this.sheet,",
		"    required this.warehouse,",
		"    required this.items,",
		"    required this.skus,",
		"    required this.onEdit,",
		"  });",
		"",
		"  final InventorySheet sheet;",
		"  final Warehouse? warehouse;",
		"  final List<LineItem> items;",
		"  final List<Sku> skus;",
		"  final Future<void> Function(InventorySheet sheet) onEdit;",
		"",
		"  @override",
		"  Widget build(BuildContext context) {",
		"    return Scaffold(",
		"      appBar: AppBar(",
		"        title: Text(openLiteCopy.detailPageTitle),",
		"        actions: [",
		"          TextButton(",
		"            onPressed: () => onEdit(sheet),",
		"            child: Text(openCCopy.editActionLabel),",
		"          ),",
		"        ],",
		"      ),",
		"      body: ListView(",
		"        children: [",
		"          _InfoTile(label: openLiteCopy.detailCategoryLabel, value: warehouse?.name ?? sheet.warehouseId),",
		"          _InfoTile(label: openLiteKey.detailStatusLabel, value: openLiteCopy.statusLabel(sheet.status)),",
		"          Text(openLiteCopy.lineItemTitle),",
		"        ],",
		"      ),",
		"    );",
		"  }",
		"}",
		"",
		"class _InfoTile extends StatelessWidget {",
		"  const _InfoTile({required this.label, required this.value});",
		"  final String label;",
		"  final String value;",
		"",
		"  @override",
		"  Widget build(BuildContext context) => Text('$label$value');",
		"}",
	}, "\n") + "\n"

	normalized := normalizeBuilderRuntimeViewContent(workspacePath, false, false, content)

	for _, unwanted := range []string{"openCCopy.", "openLiteKey."} {
		if strings.Contains(normalized, unwanted) {
			t.Fatalf("normalizeBuilderRuntimeViewContent() should repair openLiteCopy helper typo %q: %q", unwanted, normalized)
		}
	}
	for _, want := range []string{"openLiteCopy.editActionLabel", "openLiteCopy.detailStatusLabel", "openLiteCopy.detailCategoryLabel", "openLiteCopy.lineItemTitle"} {
		if !strings.Contains(normalized, want) {
			t.Fatalf("normalizeBuilderRuntimeViewContent() missing repaired helper reference %q: %q", want, normalized)
		}
	}
}

func TestNormalizeBuilderRuntimeViewContentRewritesDropdownInitialValueInsideDropdownButtonFormField(t *testing.T) {
	workspacePath := createBuilderRuntimeRelationRichWorkspace(t, nil)
	content := strings.Join([]string{
		"import 'package:flutter/material.dart';",
		"",
		"import '../controllers/record_list_controller.dart';",
		"",
		"class RecordListPage extends StatelessWidget {",
		"  const RecordListPage({",
		"    super.key,",
		"    required this.controller,",
		"    required this.onOpenRecordDetail,",
		"  });",
		"",
		"  final RecordListController controller;",
		"  final Future<void> Function(String taskId) onOpenRecordDetail;",
		"",
		"  @override",
		"  Widget build(BuildContext context) {",
		"    return Column(",
		"      children: [",
		"        DropdownButtonFormField<String>(",
		"          initialValue: controller.selectedProjectId,",
		"          items: const <DropdownMenuItem<String>>[],",
		"          onChanged: controller.selectProject,",
		"        ),",
		"        DropdownButtonFormField<String>(",
		"          initialValue: controller.selectedTagId,",
		"          items: const <DropdownMenuItem<String>>[],",
		"          onChanged: controller.selectTag,",
		"        ),",
		"      ],",
		"    );",
		"  }",
		"}",
	}, "\n") + "\n"

	normalized := normalizeBuilderRuntimeViewContent(workspacePath, false, false, content)

	if strings.Contains(normalized, "initialValue:") {
		t.Fatalf("normalizeBuilderRuntimeViewContent() should rewrite DropdownButtonFormField initialValue to value: %q", normalized)
	}
	if strings.Count(normalized, "value: controller.") != 2 {
		t.Fatalf("normalizeBuilderRuntimeViewContent() should preserve dropdown selections with value: %q", normalized)
	}
}

func TestNormalizeBuilderRuntimePatchForWorkspaceWithTaskBundlePrunesDependentOperationsFromSingleTargetNonRepairPatch(t *testing.T) {
	workspacePath := createBuilderRuntimeRelationRichWorkspace(t, nil)
	patch := appruns.WorkspacePatch{Operations: []appruns.WorkspacePatchOperation{
		{Type: "write_file", Path: "lib/views/record_list_page.dart", Content: "class RecordListPage {}\n"},
		{Type: "write_file", Path: "lib/controllers/record_list_controller.dart", Content: "class RecordListController {}\n"},
		{Type: "write_file", Path: "lib/template/open_lite_pop.dart", Content: "const openLiteCopy = Object();\n"},
	}}
	taskBundle := []appruns.TaskBundleItem{{
		TaskID:      "task-bind-collection-surface",
		TaskType:    appruns.BuilderRuntimeTaskTypeDualFileWiring,
		TargetPaths: []string{"lib/views/record_list_page.dart"},
	}}

	normalizeBuilderRuntimePatchForWorkspaceWithTaskBundle(workspacePath, taskBundle, &patch)

	if len(patch.Operations) != 1 {
		t.Fatalf("len(patch.Operations) = %d, want 1 after pruning single-target dependent edits", len(patch.Operations))
	}
	if patch.Operations[0].Path != "lib/views/record_list_page.dart" {
		t.Fatalf("patch.Operations[0].Path = %q, want lib/views/record_list_page.dart", patch.Operations[0].Path)
	}
}

func TestNormalizeBuilderRuntimePatchForWorkspaceWithTaskBundlePrunesRelationRichRepositorySiblingOperations(t *testing.T) {
	workspacePath := createBuilderRuntimeRelationRichWorkspace(t, nil)
	patch := appruns.WorkspacePatch{Operations: []appruns.WorkspacePatchOperation{
		{Type: "write_file", Path: "lib/repositories/record_repository.dart", Content: "abstract class RecordRepository {\n  Future<void> init();\n  Future<void> updateSummaries(List<dynamic> summaries);\n}\n"},
		{Type: "write_file", Path: "lib/repositories/hive_record_repository.dart", Content: "class HiveRecordRepository implements RecordRepository {}\n"},
		{Type: "write_file", Path: "lib/repositories/in_memory_record_repository.dart", Content: "class InMemoryRecordRepository implements RecordRepository {}\n"},
	}}
	taskBundle := []appruns.TaskBundleItem{{
		TaskID:      "task-create-repository",
		TaskType:    appruns.BuilderRuntimeTaskTypeDualFileWiring,
		TargetPaths: []string{"lib/repositories/record_repository.dart"},
	}}

	normalizeBuilderRuntimePatchForWorkspaceWithTaskBundle(workspacePath, taskBundle, &patch)

	if len(patch.Operations) != 1 {
		t.Fatalf("len(patch.Operations) = %d, want 1 after pruning repository sibling edits", len(patch.Operations))
	}
	if patch.Operations[0].Path != "lib/repositories/record_repository.dart" {
		t.Fatalf("patch.Operations[0].Path = %q, want lib/repositories/record_repository.dart", patch.Operations[0].Path)
	}
}

func TestNormalizeBuilderRuntimePatchForWorkspaceWithTaskBundlePrunesCustomRepositorySiblingOperations(t *testing.T) {
	workspacePath := t.TempDir()
	patch := appruns.WorkspacePatch{Operations: []appruns.WorkspacePatchOperation{
		{Type: "write_file", Path: "lib/repositories/task_repository.dart", Content: "abstract class TaskRepository {\n  Future<void> init();\n}\n"},
		{Type: "write_file", Path: "lib/repositories/hive_task_repository.dart", Content: "class HiveTaskRepository implements TaskRepository {}\n"},
		{Type: "write_file", Path: "lib/repositories/in_memory_task_repository.dart", Content: "class InMemoryTaskRepository implements TaskRepository {}\n"},
	}}
	taskBundle := []appruns.TaskBundleItem{{
		TaskID:      "task-create-repository",
		TaskType:    appruns.BuilderRuntimeTaskTypeDualFileWiring,
		TargetPaths: []string{"lib/repositories/task_repository.dart"},
	}}

	normalizeBuilderRuntimePatchForWorkspaceWithTaskBundle(workspacePath, taskBundle, &patch)

	if len(patch.Operations) != 1 {
		t.Fatalf("len(patch.Operations) = %d, want 1 after pruning custom repository sibling edits", len(patch.Operations))
	}
	if patch.Operations[0].Path != "lib/repositories/task_repository.dart" {
		t.Fatalf("patch.Operations[0].Path = %q, want lib/repositories/task_repository.dart", patch.Operations[0].Path)
	}
}

func TestNormalizeBuilderRuntimePatchForWorkspaceWithTaskBundlePrunesCustomCollectionSiblingOperationsWithoutSurfaceRefs(t *testing.T) {
	workspacePath := t.TempDir()
	controllerPath := filepath.Join(workspacePath, "lib", "controllers", "task_collection_controller.dart")
	if err := os.MkdirAll(filepath.Dir(controllerPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(controllerPath) error = %v", err)
	}
	if err := os.WriteFile(controllerPath, []byte("import '../repositories/task_repository.dart';\nclass TaskCollectionController {\n  TaskCollectionController({required TaskRepository taskRepository});\n}\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(task_collection_controller.dart) error = %v", err)
	}
	patch := appruns.WorkspacePatch{Operations: []appruns.WorkspacePatchOperation{
		{Type: "write_file", Path: "lib/views/task_collection_page.dart", Content: "class TaskCollectionPage {}\n"},
		{Type: "write_file", Path: "lib/controllers/task_collection_controller.dart", Content: "class TaskCollectionController {}\n"},
		{Type: "write_file", Path: "lib/template/open_lite_copy.dart", Content: "const openLiteCopy = Object();\n"},
		{Type: "write_file", Path: "lib/template/open_lite_pop.dart", Content: "const openLitePop = Object();\n"},
	}}
	taskBundle := []appruns.TaskBundleItem{{
		TaskID:      "task-bind-collection-surface",
		TaskType:    appruns.BuilderRuntimeTaskTypeDualFileWiring,
		TargetPaths: []string{"lib/views/task_collection_page.dart"},
	}}

	normalizeBuilderRuntimePatchForWorkspaceWithTaskBundle(workspacePath, taskBundle, &patch)

	if len(patch.Operations) != 1 {
		t.Fatalf("len(patch.Operations) = %d, want 1 after pruning custom collection sibling edits", len(patch.Operations))
	}
	if patch.Operations[0].Path != "lib/views/task_collection_page.dart" {
		t.Fatalf("patch.Operations[0].Path = %q, want lib/views/task_collection_page.dart", patch.Operations[0].Path)
	}
}

func TestNormalizeBuilderRuntimePatchForWorkspaceWithTaskBundlePrunesCustomOverviewSiblingOperationsWithoutSurfaceRefs(t *testing.T) {
	workspacePath := t.TempDir()
	patch := appruns.WorkspacePatch{Operations: []appruns.WorkspacePatchOperation{
		{Type: "write_file", Path: "lib/views/task_home_page.dart", Content: "class TaskHomePage {}\n"},
		{Type: "write_file", Path: "lib/controllers/task_home_controller.dart", Content: "class TaskHomeController {}\n"},
		{Type: "write_file", Path: "lib/template/open_lite_copy.dart", Content: "const openLiteCopy = Object();\n"},
	}}
	taskBundle := []appruns.TaskBundleItem{{
		TaskID:      "task-bind-overview-surface",
		TaskType:    appruns.BuilderRuntimeTaskTypeDualFileWiring,
		TargetPaths: []string{"lib/views/task_home_page.dart"},
	}}

	normalizeBuilderRuntimePatchForWorkspaceWithTaskBundle(workspacePath, taskBundle, &patch)

	if len(patch.Operations) != 1 {
		t.Fatalf("len(patch.Operations) = %d, want 1 after pruning custom overview sibling edits", len(patch.Operations))
	}
	if patch.Operations[0].Path != "lib/views/task_home_page.dart" {
		t.Fatalf("patch.Operations[0].Path = %q, want lib/views/task_home_page.dart", patch.Operations[0].Path)
	}
}

func TestNormalizeBuilderRuntimePatchForWorkspaceWithTaskBundlePrunesCustomMutationSiblingOperationsWithoutSurfaceRefs(t *testing.T) {
	workspacePath := t.TempDir()
	patch := appruns.WorkspacePatch{Operations: []appruns.WorkspacePatchOperation{
		{Type: "write_file", Path: "lib/views/task_form_page.dart", Content: "class TaskFormPage {}\n"},
		{Type: "write_file", Path: "lib/controllers/task_form_controller.dart", Content: "class TaskFormController {}\n"},
	}}
	taskBundle := []appruns.TaskBundleItem{{
		TaskID:      "task-bind-mutation-surface",
		TaskType:    appruns.BuilderRuntimeTaskTypeDualFileWiring,
		TargetPaths: []string{"lib/views/task_form_page.dart"},
	}}

	normalizeBuilderRuntimePatchForWorkspaceWithTaskBundle(workspacePath, taskBundle, &patch)

	if len(patch.Operations) != 1 {
		t.Fatalf("len(patch.Operations) = %d, want 1 after pruning custom mutation sibling edits", len(patch.Operations))
	}
	if patch.Operations[0].Path != "lib/views/task_form_page.dart" {
		t.Fatalf("patch.Operations[0].Path = %q, want lib/views/task_form_page.dart", patch.Operations[0].Path)
	}
}

func TestNormalizeBuilderRuntimePatchForWorkspaceWithTaskBundlePrunesCustomDetailSiblingOperationsWithoutSurfaceRefs(t *testing.T) {
	workspacePath := t.TempDir()
	patch := appruns.WorkspacePatch{Operations: []appruns.WorkspacePatchOperation{
		{Type: "write_file", Path: "lib/views/task_detail_page.dart", Content: "class TaskDetailPage {}\n"},
		{Type: "write_file", Path: "lib/template/open_lite_copy.dart", Content: "const openLiteCopy = Object();\n"},
	}}
	taskBundle := []appruns.TaskBundleItem{{
		TaskID:      "task-bind-detail-surface",
		TaskType:    appruns.BuilderRuntimeTaskTypeDualFileWiring,
		TargetPaths: []string{"lib/views/task_detail_page.dart"},
	}}

	normalizeBuilderRuntimePatchForWorkspaceWithTaskBundle(workspacePath, taskBundle, &patch)

	if len(patch.Operations) != 1 {
		t.Fatalf("len(patch.Operations) = %d, want 1 after pruning custom detail helper edits", len(patch.Operations))
	}
	if patch.Operations[0].Path != "lib/views/task_detail_page.dart" {
		t.Fatalf("patch.Operations[0].Path = %q, want lib/views/task_detail_page.dart", patch.Operations[0].Path)
	}
	if strings.Contains(patch.Operations[0].Content, "openLiteCopy") {
		t.Fatalf("detail target operation should not be replaced by helper content: %q", patch.Operations[0].Content)
	}
}

func TestNormalizeBuilderRuntimePatchForWorkspaceRewritesRecordFormPageWithoutHomeControllerAndDateAPI(t *testing.T) {
	workspacePath := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspacePath, "lib", "controllers"), 0o755); err != nil {
		t.Fatalf("MkdirAll(controllers) error = %v", err)
	}
	if err := os.MkdirAll(filepath.Join(workspacePath, "lib", "repositories"), 0o755); err != nil {
		t.Fatalf("MkdirAll(repositories) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(workspacePath, "lib", "controllers", "record_form_controller.dart"), []byte(strings.Join([]string{
		"import '../repositories/record_repository.dart';",
		"class RecordFormController {",
		"  Future<void> submit(RecordRepository recordRepository) async {}",
		"}",
	}, "\n")), 0o600); err != nil {
		t.Fatalf("WriteFile(record_form_controller.dart) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(workspacePath, "lib", "repositories", "record_repository.dart"), []byte("abstract class RecordRepository {}\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(record_repository.dart) error = %v", err)
	}
	patch := appruns.WorkspacePatch{Operations: []appruns.WorkspacePatchOperation{{
		Type: "write_file",
		Path: "lib/views/record_form_page.dart",
		Content: strings.Join([]string{
			"import 'package:flutter/material.dart';",
			"",
			"import '../controllers/record_form_controller.dart';",
			"import '../controllers/home_controller.dart';",
			"import '../models/record.dart';",
			"import '../template/open_lite_copy.dart';",
			"",
			"class RecordFormPage extends StatefulWidget {",
			"  const RecordFormPage({",
			"    super.key,",
			"    required this.homeController,",
			"    this.initialRecord,",
			"  });",
			"",
			"  final HomeController homeController;",
			"  final AppRecord? initialRecord;",
			"",
			"  @override",
			"  State<RecordFormPage> createState() => _RecordFormPageState();",
			"}",
			"",
			"class _RecordFormPageState extends State<RecordFormPage> {",
			"  @override",
			"  void dispose() {",
			"    super.dispose();",
			"  }",
			"",
			"  Future<void> _pickDate() async {",
			"    final pickedDate = await showDatePicker(",
			"      context: context,",
			"      initialDate: _controller.selectedDate,",
			"      firstDate: DateTime(2020),",
			"      lastDate: DateTime(2100),",
			"    );",
			"    if (pickedDate != null) {",
			"      _controller.setDate(pickedDate);",
			"    }",
			"  }",
			"",
			"  Future<void> _save() async {",
			"    final savedRecord = await _controller.submit(widget.homeController);",
			"  }",
			"",
			"  @override",
			"  Widget build(BuildContext context) {",
			"    return ListView(",
			"      children: [",
			"        const SizedBox(height: 16),",
			"        ListTile(",
			"          contentPadding: const EdgeInsets.symmetric(horizontal: 12),",
			"          shape: RoundedRectangleBorder(",
			"            borderRadius: BorderRadius.circular(12),",
			"            side: BorderSide(color: Theme.of(context).dividerColor),",
			"          ),",
			"          title: Text(openLiteCopy.dateFieldLabel),",
			"          subtitle: Text(",
			"            '${_controller.selectedDate.year}-${_controller.selectedDate.month.toString().padLeft(2, '0')}-${_controller.selectedDate.day.toString().padLeft(2, '0')}',",
			"          ),",
			"          trailing: IconButton(",
			"            onPressed: _pickDate,",
			"            icon: const Icon(Icons.calendar_today),",
			"          ),",
			"        ),",
			"        const SizedBox(height: 16),",
			"        TextFormField(),",
			"      ],",
			"    );",
			"  }",
			"}",
		}, "\n"),
	}}}

	normalizeBuilderRuntimePatchForWorkspace(workspacePath, &patch)

	content := patch.Operations[0].Content
	if strings.Contains(content, "../controllers/home_controller.dart") {
		t.Fatalf("normalizeBuilderRuntimePatchForWorkspace() should remove unresolved home_controller import: %q", content)
	}
	if strings.Contains(content, "HomeController") {
		t.Fatalf("normalizeBuilderRuntimePatchForWorkspace() should rewrite HomeController references when only RecordRepository exists: %q", content)
	}
	if !strings.Contains(content, "import '../repositories/record_repository.dart';") {
		t.Fatalf("normalizeBuilderRuntimePatchForWorkspace() should import record_repository.dart: %q", content)
	}
	if !strings.Contains(content, "final RecordRepository recordRepository;") {
		t.Fatalf("normalizeBuilderRuntimePatchForWorkspace() should rewrite form page dependency to RecordRepository: %q", content)
	}
	if !strings.Contains(content, "submit(widget.recordRepository)") {
		t.Fatalf("normalizeBuilderRuntimePatchForWorkspace() should submit via widget.recordRepository: %q", content)
	}
	if strings.Contains(content, "selectedDate") || strings.Contains(content, "setDate(") || strings.Contains(content, "_pickDate(") {
		t.Fatalf("normalizeBuilderRuntimePatchForWorkspace() should drop date picker API when controller does not expose it: %q", content)
	}
	if strings.Contains(content, "\n  }\n  }\n\n  Future<void> _save() async {") {
		t.Fatalf("normalizeBuilderRuntimePatchForWorkspace() should not leave an orphan closing brace after removing _pickDate(): %q", content)
	}
	if !strings.Contains(content, "super.dispose();\n  }\n\n  Future<void> _save() async {") {
		t.Fatalf("normalizeBuilderRuntimePatchForWorkspace() should keep dispose() and preserve surrounding method structure: %q", content)
	}
}

func TestNormalizeBuilderRuntimePatchForWorkspaceRewritesCustomMutationPageWithoutHomeControllerAndDateAPI(t *testing.T) {
	workspacePath := createBuilderRuntimeCustomMutationSurfaceWorkspace(t)
	patch := appruns.WorkspacePatch{Operations: []appruns.WorkspacePatchOperation{{
		Type: "write_file",
		Path: "lib/views/task_form_page.dart",
		Content: strings.Join([]string{
			"import 'package:flutter/material.dart';",
			"",
			"import '../controllers/task_form_controller.dart';",
			"import '../controllers/home_controller.dart';",
			"import '../models/task.dart';",
			"import '../template/open_lite_copy.dart';",
			"",
			"class TaskFormPage extends StatefulWidget {",
			"  const TaskFormPage({",
			"    super.key,",
			"    required this.homeController,",
			"    this.initialTask,",
			"  });",
			"",
			"  final HomeController homeController;",
			"  final Object? initialTask;",
			"",
			"  @override",
			"  State<TaskFormPage> createState() => _TaskFormPageState();",
			"}",
			"",
			"class _TaskFormPageState extends State<TaskFormPage> {",
			"  @override",
			"  void dispose() {",
			"    super.dispose();",
			"  }",
			"",
			"  Future<void> _pickDate() async {",
			"    final pickedDate = await showDatePicker(",
			"      context: context,",
			"      initialDate: _controller.selectedDate,",
			"      firstDate: DateTime(2020),",
			"      lastDate: DateTime(2100),",
			"    );",
			"    if (pickedDate != null) {",
			"      _controller.setDate(pickedDate);",
			"    }",
			"  }",
			"",
			"  Future<void> _save() async {",
			"    final savedTask = await _controller.submit(widget.homeController);",
			"  }",
			"",
			"  @override",
			"  Widget build(BuildContext context) {",
			"    return ListView(",
			"      children: [",
			"        const SizedBox(height: 16),",
			"        ListTile(",
			"          contentPadding: const EdgeInsets.symmetric(horizontal: 12),",
			"          shape: RoundedRectangleBorder(",
			"            borderRadius: BorderRadius.circular(12),",
			"            side: BorderSide(color: Theme.of(context).dividerColor),",
			"          ),",
			"          title: Text(openLiteCopy.dateFieldLabel),",
			"          subtitle: Text(",
			"            '${_controller.selectedDate.year}-${_controller.selectedDate.month.toString().padLeft(2, '0')}-${_controller.selectedDate.day.toString().padLeft(2, '0')}',",
			"          ),",
			"          trailing: IconButton(",
			"            onPressed: _pickDate,",
			"            icon: const Icon(Icons.calendar_today),",
			"          ),",
			"        ),",
			"        const SizedBox(height: 16),",
			"        TextFormField(),",
			"      ],",
			"    );",
			"  }",
			"}",
		}, "\n"),
	}}}

	normalizeBuilderRuntimePatchForWorkspace(workspacePath, &patch)

	content := patch.Operations[0].Content
	if strings.Contains(content, "../controllers/home_controller.dart") {
		t.Fatalf("normalizeBuilderRuntimePatchForWorkspace() should remove unresolved home_controller import from custom mutation page: %q", content)
	}
	if strings.Contains(content, "HomeController") {
		t.Fatalf("normalizeBuilderRuntimePatchForWorkspace() should rewrite HomeController references on custom mutation page when current workspace uses TaskRepository: %q", content)
	}
	if !strings.Contains(content, "import '../repositories/task_repository.dart';") {
		t.Fatalf("normalizeBuilderRuntimePatchForWorkspace() should import task_repository.dart for custom mutation page: %q", content)
	}
	if !strings.Contains(content, "final TaskRepository taskRepository;") {
		t.Fatalf("normalizeBuilderRuntimePatchForWorkspace() should rewrite custom mutation page dependency to TaskRepository: %q", content)
	}
	if !strings.Contains(content, "submit(widget.taskRepository)") {
		t.Fatalf("normalizeBuilderRuntimePatchForWorkspace() should submit via widget.taskRepository on custom mutation page: %q", content)
	}
	if strings.Contains(content, "selectedDate") || strings.Contains(content, "setDate(") || strings.Contains(content, "_pickDate(") {
		t.Fatalf("normalizeBuilderRuntimePatchForWorkspace() should drop date picker API on custom mutation page when controller does not expose it: %q", content)
	}
	if strings.Contains(content, "\n  }\n  }\n\n  Future<void> _save() async {") {
		t.Fatalf("normalizeBuilderRuntimePatchForWorkspace() should not leave an orphan closing brace after removing _pickDate() from custom mutation page: %q", content)
	}
	if !strings.Contains(content, "super.dispose();\n  }\n\n  Future<void> _save() async {") {
		t.Fatalf("normalizeBuilderRuntimePatchForWorkspace() should keep dispose() and preserve surrounding method structure for custom mutation page: %q", content)
	}
}

func TestNormalizeBuilderRuntimePatchForWorkspaceCanonicalizesRelationRichCustomMutationPageWithCustomController(t *testing.T) {
	workspacePath := createBuilderRuntimeProjectTaskTagOverviewWorkspace(t)
	writeBuilderRuntimeWorkspaceFiles(t, workspacePath, map[string]string{
		"lib/controllers/task_form_controller.dart": strings.Join([]string{
			"import 'package:flutter/material.dart';",
			"",
			"import '../models/task.dart';",
			"",
			"class TaskFormController extends ChangeNotifier {",
			"  TaskFormController();",
			"  final TextEditingController titleController = TextEditingController();",
			"  final TextEditingController noteController = TextEditingController();",
			"  final TextEditingController projectController = TextEditingController();",
			"  List<dynamic> get projects => const [];",
			"  List<dynamic> get tags => const [];",
			"  List<String> get selectedTagIds => const [];",
			"  TaskStatus get selectedStatus => TaskStatus.todo;",
			"  DateTime? get selectedDate => null;",
			"  bool get isEditing => false;",
			"  void setProject(String projectId) {}",
			"  void toggleTag(String tagId) {}",
			"  void setStatus(TaskStatus status) {}",
			"  void setDate(DateTime? date) {}",
			"  Future<dynamic> submit() async => Object();",
			"}",
		}, "\n") + "\n",
		"lib/views/task_form_page.dart": strings.Join([]string{
			"import 'package:flutter/material.dart';",
			"",
			"import '../controllers/task_form_controller.dart';",
			"",
			"class TaskFormPage extends StatefulWidget {",
			"  const TaskFormPage({",
			"    super.key,",
			"    required this.formController,",
			"  });",
			"",
			"  final TaskFormController formController;",
			"",
			"  @override",
			"  State<TaskFormPage> createState() => _TaskFormPageState();",
			"}",
			"",
			"class _TaskFormPageState extends State<TaskFormPage> {",
			"  @override",
			"  Widget build(BuildContext context) => const SizedBox.shrink();",
			"}",
		}, "\n") + "\n",
	})
	patch := appruns.WorkspacePatch{Operations: []appruns.WorkspacePatchOperation{{
		Type: "write_file",
		Path: "lib/views/task_form_page.dart",
		Content: strings.Join([]string{
			"import 'package:flutter/material.dart';",
			"",
			"import '../controllers/task_form_controller.dart';",
			"",
			"class TaskFormPage extends StatefulWidget {",
			"  const TaskFormPage({",
			"    super.key,",
			"    required this.formController,",
			"  });",
			"",
			"  final TaskFormController formController;",
			"",
			"  @override",
			"  State<TaskFormPage> createState() => _TaskFormPageState();",
			"}",
			"",
			"class _TaskFormPageState extends State<TaskFormPage> {",
			"  @override",
			"  Widget build(BuildContext context) {",
			"    return const Placeholder();",
			"  }",
			"}",
		}, "\n"),
	}}}

	normalizeBuilderRuntimePatchForWorkspace(workspacePath, &patch)

	content := patch.Operations[0].Content
	for _, want := range []string{
		"class TaskFormPage extends StatefulWidget {",
		"required this.formController,",
		"final TaskFormController formController;",
		"FilterChip(",
		"widget.formController.selectedTagIds.contains(tag.tagId)",
		"widget.formController.toggleTag(tag.tagId)",
		"widget.formController.setStatus(TaskStatus.todo)",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("normalizeBuilderRuntimePatchForWorkspace() canonical custom mutation page missing %q: %q", want, content)
		}
	}
	for _, forbidden := range []string{
		"class RecordFormPage extends StatefulWidget {",
		"required this.controller,",
		"final RecordFormController controller;",
	} {
		if strings.Contains(content, forbidden) {
			t.Fatalf("normalizeBuilderRuntimePatchForWorkspace() should rewrite relation-rich custom mutation page away from default form contract %q: %q", forbidden, content)
		}
	}
}

func TestNormalizeBuilderRuntimeOpenLiteFormPageContentPreservesDueOnDatePicker(t *testing.T) {
	workspacePath := t.TempDir()
	writeBuilderRuntimeWorkspaceFiles(t, workspacePath, map[string]string{
		"lib/controllers/home_controller.dart": "class HomeController {}\n",
		"lib/controllers/record_form_controller.dart": strings.Join([]string{
			"class RecordFormController {",
			"  RecordFormController({required dynamic repository});",
			"  DateTime? get selectedDueOn => null;",
			"  void setDueOn(DateTime? dueOn) {}",
			"}",
		}, "\n") + "\n",
		"lib/repositories/record_repository.dart": "abstract class RecordRepository {}\n",
	})
	content := strings.Join([]string{
		"import 'package:flutter/material.dart';",
		"",
		"import '../controllers/record_form_controller.dart';",
		"import '../repositories/record_repository.dart';",
		"",
		"class RecordFormPage extends StatefulWidget {",
		"  const RecordFormPage({super.key, required this.recordRepository});",
		"",
		"  final RecordRepository recordRepository;",
		"",
		"  @override",
		"  State<RecordFormPage> createState() => _RecordFormPageState();",
		"}",
		"",
		"class _RecordFormPageState extends State<RecordFormPage> {",
		"  final GlobalKey<FormState> _formKey = GlobalKey<FormState>();",
		"  late final RecordFormController _controller;",
		"",
		"  @override",
		"  void initState() {",
		"    super.initState();",
		"    _controller = RecordFormController(repository: widget.recordRepository);",
		"  }",
		"",
		"  Future<void> _pickDate() async {",
		"    final pickedDate = await showDatePicker(",
		"      context: context,",
		"      initialDate: _controller.selectedDueOn ?? DateTime(2026, 4, 17),",
		"      firstDate: DateTime(2020),",
		"      lastDate: DateTime(2100),",
		"    );",
		"    if (pickedDate != null) {",
		"      _controller.setDueOn(pickedDate);",
		"    }",
		"  }",
		"",
		"  Future<void> _save() async {}",
		"",
		"  @override",
		"  Widget build(BuildContext context) {",
		"    return Form(",
		"      key: _formKey,",
		"      child: ListView(",
		"        children: [",
		"          TextFormField(key: const Key('title-field')),",
		"          InkWell(",
		"            onTap: _pickDate,",
		"            child: Text(",
		"              _controller.selectedDueOn == null",
		"                  ? '未设置'",
		"                  : _controller.selectedDueOn!.toIso8601String(),",
		"            ),",
		"          ),",
		"          TextFormField(key: const Key('note-field')),",
		"        ],",
		"      ),",
		"    );",
		"  }",
		"}",
	}, "\n") + "\n"

	normalized := normalizeBuilderRuntimeOpenLiteFormPageContent(workspacePath, content)

	for _, want := range []string{"Future<void> _pickDate() async {", "setDueOn(", "selectedDueOn", "onTap: _pickDate,"} {
		if !strings.Contains(normalized, want) {
			t.Fatalf("normalizeBuilderRuntimeOpenLiteFormPageContent() should preserve relation-rich dueOn date picker marker %q: %q", want, normalized)
		}
	}
}

func TestNormalizeBuilderRuntimePatchForWorkspaceRemovesRepositoryUpdatedAtForRelationRichPrimaryModel(t *testing.T) {
	workspacePath := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspacePath, "lib", "models"), 0o755); err != nil {
		t.Fatalf("MkdirAll(models) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(workspacePath, "lib", "models", "task.dart"), []byte(strings.Join([]string{
		"class Task {",
		"  final String taskId;",
		"  final DateTime? dueOn;",
		"",
		"  Task({required this.taskId, this.dueOn});",
		"}",
	}, "\n")), 0o600); err != nil {
		t.Fatalf("WriteFile(task.dart) error = %v", err)
	}
	patch := appruns.WorkspacePatch{Operations: []appruns.WorkspacePatchOperation{{
		Type: "write_file",
		Path: "lib/repositories/record_repository.dart",
		Content: strings.Join([]string{
			"import '../models/task.dart';",
			"class HiveRecordRepository {",
			"  Future<List<Task>> loadTasks() async {",
			"    return <Task>[]",
			"      ..sort((left, right) => right.updatedAt.compareTo(left.updatedAt));",
			"  }",
			"}",
		}, "\n"),
	}}}

	normalizeBuilderRuntimePatchForWorkspace(workspacePath, &patch)

	content := patch.Operations[0].Content
	if strings.Contains(content, "updatedAt") {
		t.Fatalf("normalizeBuilderRuntimePatchForWorkspace() should remove updatedAt when relation-rich primary model lacks that field: %q", content)
	}
	if strings.Contains(content, "dueOn.compareTo") {
		t.Fatalf("normalizeBuilderRuntimePatchForWorkspace() should not replace updatedAt with nullable dueOn sort: %q", content)
	}
	if strings.Contains(content, ".sort(") {
		t.Fatalf("normalizeBuilderRuntimePatchForWorkspace() should drop invalid repository sort when no non-null time field exists: %q", content)
	}
}

func TestNormalizeBuilderRuntimePatchForWorkspaceRepositoryNormalizePrefersImportedModelOverStaleRecordPath(t *testing.T) {
	workspacePath := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspacePath, "lib", "models"), 0o755); err != nil {
		t.Fatalf("MkdirAll(models) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(workspacePath, "lib", "models", "task.dart"), []byte(strings.Join([]string{
		"class Task {",
		"  final String taskId;",
		"  final DateTime? dueOn;",
		"",
		"  Task({required this.taskId, this.dueOn});",
		"}",
	}, "\n")), 0o600); err != nil {
		t.Fatalf("WriteFile(task.dart) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(workspacePath, "lib", "models", "record.dart"), []byte(strings.Join([]string{
		"class AppRecord {",
		"  const AppRecord({required this.updatedAt});",
		"  final DateTime updatedAt;",
		"}",
	}, "\n")), 0o600); err != nil {
		t.Fatalf("WriteFile(record.dart) error = %v", err)
	}
	patch := appruns.WorkspacePatch{Operations: []appruns.WorkspacePatchOperation{{
		Type: "write_file",
		Path: "lib/repositories/record_repository.dart",
		Content: strings.Join([]string{
			"import '../models/task.dart';",
			"class HiveRecordRepository {",
			"  Future<List<Task>> loadTasks() async {",
			"    return <Task>[]",
			"      ..sort((left, right) => right.updatedAt.compareTo(left.updatedAt));",
			"  }",
			"}",
		}, "\n"),
	}}}

	normalizeBuilderRuntimePatchForWorkspace(workspacePath, &patch)

	content := patch.Operations[0].Content
	if strings.Contains(content, "updatedAt") {
		t.Fatalf("normalizeBuilderRuntimePatchForWorkspace() should not keep updatedAt when repository imports task.dart and stale record.dart is unrelated: %q", content)
	}
	if strings.Contains(content, "dueOn.compareTo") {
		t.Fatalf("normalizeBuilderRuntimePatchForWorkspace() should not replace updatedAt with nullable dueOn sort when repository imports task.dart: %q", content)
	}
	if strings.Contains(content, ".sort(") {
		t.Fatalf("normalizeBuilderRuntimePatchForWorkspace() should drop invalid repository sort when imported primary model lacks a non-null time field: %q", content)
	}
}

func TestNormalizeBuilderRuntimePatchForWorkspaceRewritesArbitraryViewPathWithoutFixedHomeName(t *testing.T) {
	workspacePath := t.TempDir()
	summaryPath := filepath.Join(workspacePath, "lib", "models", "dashboard_summary.dart")
	if err := os.MkdirAll(filepath.Dir(summaryPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(summaryPath) error = %v", err)
	}
	if err := os.WriteFile(summaryPath, []byte(strings.Join([]string{
		"class DashboardSummary {",
		"  const DashboardSummary({required this.inboxCount, required this.inProgressCount, required this.doneCount});",
		"  final int inboxCount;",
		"  final int inProgressCount;",
		"  final int doneCount;",
		"}",
	}, "\n")), 0o600); err != nil {
		t.Fatalf("WriteFile(dashboard_summary.dart) error = %v", err)
	}
	recordPath := filepath.Join(workspacePath, "lib", "models", "record.dart")
	if err := os.WriteFile(recordPath, []byte(strings.Join([]string{
		"class AppRecord {",
		"  const AppRecord({required this.recordedAt});",
		"  final DateTime recordedAt;",
		"}",
	}, "\n")), 0o600); err != nil {
		t.Fatalf("WriteFile(record.dart) error = %v", err)
	}
	patch := appruns.WorkspacePatch{Operations: []appruns.WorkspacePatchOperation{{
		Type:    "write_file",
		Path:    "lib/views/overview_surface.dart",
		Content: "class OverviewSurface {\n  String render(dynamic summary, dynamic controller, dynamic record) => '${summary.totalCount}-${record.updatedAt.toIso8601String()}';\n}\n",
	}}}

	normalizeBuilderRuntimePatchForWorkspace(workspacePath, &patch)

	content := patch.Operations[0].Content
	if strings.Contains(content, "summary.totalCount") || strings.Contains(content, ".updatedAt") {
		t.Fatalf("normalizeBuilderRuntimePatchForWorkspace() should rewrite arbitrary view path content without fixed home_page name: %q", content)
	}
	if !strings.Contains(content, "controller.records.length") {
		t.Fatalf("normalizeBuilderRuntimePatchForWorkspace() should rewrite totalCount on arbitrary view path: %q", content)
	}
	if !strings.Contains(content, "record.recordedAt.toIso8601String()") {
		t.Fatalf("normalizeBuilderRuntimePatchForWorkspace() should rewrite updatedAt on arbitrary view path: %q", content)
	}
}

func TestNormalizeBuilderRuntimePatchForWorkspacePreservesWithOpacityByDefault(t *testing.T) {
	workspacePath := t.TempDir()
	patch := appruns.WorkspacePatch{Operations: []appruns.WorkspacePatchOperation{{
		Type:    "write_file",
		Path:    "lib/views/home_page.dart",
		Content: "class HomePage {\n  String colorize(dynamic color) => color.withOpacity(0.14).toString();\n}\n",
	}}}

	normalizeBuilderRuntimePatchForWorkspace(workspacePath, &patch)

	content := patch.Operations[0].Content
	if !strings.Contains(content, ".withOpacity(0.14)") {
		t.Fatalf("normalizeBuilderRuntimePatchForWorkspace() should preserve withOpacity when compatibility is uncertain: %q", content)
	}
	if strings.Contains(content, ".withValues(alpha: 0.14)") {
		t.Fatalf("normalizeBuilderRuntimePatchForWorkspace() should not upgrade withOpacity to withValues(alpha: ...): %q", content)
	}
}

func TestNormalizeBuilderRuntimePatchForWorkspaceDowngradesWithValuesToWithOpacityByDefault(t *testing.T) {
	workspacePath := t.TempDir()
	patch := appruns.WorkspacePatch{Operations: []appruns.WorkspacePatchOperation{{
		Type:    "write_file",
		Path:    "lib/views/home_page.dart",
		Content: "class HomePage {\n  String colorize(dynamic color) => color.withValues(alpha: 0.14).toString();\n}\n",
	}}}

	normalizeBuilderRuntimePatchForWorkspace(workspacePath, &patch)

	content := patch.Operations[0].Content
	if strings.Contains(content, ".withValues(alpha: 0.14)") {
		t.Fatalf("normalizeBuilderRuntimePatchForWorkspace() should remove alpha-only withValues when capability is disabled: %q", content)
	}
	if !strings.Contains(content, ".withOpacity(0.14)") {
		t.Fatalf("normalizeBuilderRuntimePatchForWorkspace() should downgrade alpha-only withValues to withOpacity: %q", content)
	}
	if strings.Contains(content, "withOpacity(0.14, ") {
		t.Fatalf("normalizeBuilderRuntimePatchForWorkspace() should keep downgraded withOpacity syntax valid: %q", content)
	}

	complexPatch := appruns.WorkspacePatch{Operations: []appruns.WorkspacePatchOperation{{
		Type:    "write_file",
		Path:    "lib/views/home_page.dart",
		Content: "class HomePage {\n  String colorize(dynamic color) => color.withValues(alpha: 0.14, red: 0.2).toString();\n}\n",
	}}}

	normalizeBuilderRuntimePatchForWorkspace(workspacePath, &complexPatch)

	complexContent := complexPatch.Operations[0].Content
	if !strings.Contains(complexContent, ".withValues(alpha: 0.14, red: 0.2)") {
		t.Fatalf("normalizeBuilderRuntimePatchForWorkspace() should preserve multi-argument withValues calls: %q", complexContent)
	}
	if strings.Contains(complexContent, ".withOpacity(0.14, red: 0.2)") {
		t.Fatalf("normalizeBuilderRuntimePatchForWorkspace() should not create invalid withOpacity syntax from multi-argument withValues: %q", complexContent)
	}
}

func TestNormalizeBuilderRuntimePatchForWorkspacePreservesWithValuesWhenCapabilityEnabled(t *testing.T) {
	t.Setenv("APPFACTORY_FLUTTER_ENABLE_WITH_VALUES", "1")
	workspacePath := t.TempDir()
	patch := appruns.WorkspacePatch{Operations: []appruns.WorkspacePatchOperation{{
		Type:    "write_file",
		Path:    "lib/views/home_page.dart",
		Content: "class HomePage {\n  String colorize(dynamic color) => color.withValues(alpha: 0.14).toString();\n}\n",
	}}}

	normalizeBuilderRuntimePatchForWorkspace(workspacePath, &patch)

	content := patch.Operations[0].Content
	if !strings.Contains(content, ".withValues(alpha: 0.14)") {
		t.Fatalf("normalizeBuilderRuntimePatchForWorkspace() should preserve withValues when capability is enabled: %q", content)
	}
}

func TestNormalizeBuilderRuntimePatchForWorkspaceRewritesHomePageUpdatedAtToCurrentTimeField(t *testing.T) {
	workspacePath := t.TempDir()
	recordPath := filepath.Join(workspacePath, "lib", "models", "record.dart")
	if err := os.MkdirAll(filepath.Dir(recordPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(recordPath) error = %v", err)
	}
	if err := os.WriteFile(recordPath, []byte(strings.Join([]string{
		"class AppRecord {",
		"  const AppRecord({required this.recordedAt});",
		"  final DateTime recordedAt;",
		"}",
	}, "\n")), 0o600); err != nil {
		t.Fatalf("WriteFile(record.dart) error = %v", err)
	}
	patch := appruns.WorkspacePatch{Operations: []appruns.WorkspacePatchOperation{{
		Type:    "write_file",
		Path:    "lib/views/home_page.dart",
		Content: "class HomePage {\n  String render(dynamic record) => record.updatedAt.toIso8601String();\n}\n",
	}}}

	normalizeBuilderRuntimePatchForWorkspace(workspacePath, &patch)

	content := patch.Operations[0].Content
	if strings.Contains(content, ".updatedAt") {
		t.Fatalf("normalizeBuilderRuntimePatchForWorkspace() content still references updatedAt: %q", content)
	}
	if !strings.Contains(content, "record.recordedAt.toIso8601String()") {
		t.Fatalf("normalizeBuilderRuntimePatchForWorkspace() should rewrite updatedAt to recordedAt in home_page: %q", content)
	}
}

func TestNormalizeBuilderRuntimePatchForWorkspaceRemovesHomePageUpdatedAtWidgetWhenNoTimeFieldExists(t *testing.T) {
	workspacePath := t.TempDir()
	recordPath := filepath.Join(workspacePath, "lib", "models", "record.dart")
	if err := os.MkdirAll(filepath.Dir(recordPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(recordPath) error = %v", err)
	}
	if err := os.WriteFile(recordPath, []byte(strings.Join([]string{
		"class AppRecord {",
		"  const AppRecord({required this.taskId});",
		"  final String taskId;",
		"}",
	}, "\n")), 0o600); err != nil {
		t.Fatalf("WriteFile(record.dart) error = %v", err)
	}
	patch := appruns.WorkspacePatch{Operations: []appruns.WorkspacePatchOperation{{
		Type: "write_file",
		Path: "lib/views/home_page.dart",
		Content: strings.Join([]string{
			"class HomePage {",
			"  Widget build(dynamic record) {",
			"    return Row(",
			"      children: [",
			"        Text(",
			"          '${record.updatedAt.month.toString().padLeft(2, '0')}-${record.updatedAt.day.toString().padLeft(2, '0')}',",
			"          style: const TextStyle(fontWeight: FontWeight.w600),",
			"        ),",
			"      ],",
			"    );",
			"  }",
			"}",
		}, "\n"),
	}}}

	normalizeBuilderRuntimePatchForWorkspace(workspacePath, &patch)

	content := patch.Operations[0].Content
	if strings.Contains(content, ".updatedAt") {
		t.Fatalf("normalizeBuilderRuntimePatchForWorkspace() should remove updatedAt widget when no time field exists: %q", content)
	}
	if !strings.Contains(content, "const SizedBox.shrink(),") {
		t.Fatalf("normalizeBuilderRuntimePatchForWorkspace() should replace the home_page date widget with SizedBox.shrink(): %q", content)
	}
}

func TestNormalizeBuilderRuntimePatchForWorkspaceRemovesHomePageUpdatedAtToLocalWidgetWhenNoTimeFieldExists(t *testing.T) {
	workspacePath := t.TempDir()
	recordPath := filepath.Join(workspacePath, "lib", "models", "record.dart")
	if err := os.MkdirAll(filepath.Dir(recordPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(recordPath) error = %v", err)
	}
	if err := os.WriteFile(recordPath, []byte(strings.Join([]string{
		"class TodoItem {",
		"  const TodoItem({required this.taskId});",
		"  final String taskId;",
		"}",
	}, "\n")), 0o600); err != nil {
		t.Fatalf("WriteFile(record.dart) error = %v", err)
	}
	patch := appruns.WorkspacePatch{Operations: []appruns.WorkspacePatchOperation{{
		Type: "write_file",
		Path: "lib/views/home_page.dart",
		Content: strings.Join([]string{
			"class HomePage {",
			"  Widget build(dynamic record) {",
			"    return Row(",
			"      children: [",
			"        Text(",
			"          '${record.updatedAt?.toLocal().toString().split(\" \")[0]}',",
			"          style: const TextStyle(fontWeight: FontWeight.w600),",
			"        ),",
			"      ],",
			"    );",
			"  }",
			"}",
		}, "\n"),
	}}}

	normalizeBuilderRuntimePatchForWorkspace(workspacePath, &patch)

	content := patch.Operations[0].Content
	if strings.Contains(content, ".updatedAt") {
		t.Fatalf("normalizeBuilderRuntimePatchForWorkspace() should remove toLocal updatedAt widget when no time field exists: %q", content)
	}
	if !strings.Contains(content, "const SizedBox.shrink(),") {
		t.Fatalf("normalizeBuilderRuntimePatchForWorkspace() should replace the toLocal home_page date widget with SizedBox.shrink(): %q", content)
	}
}

func TestNormalizeBuilderRuntimePatchForWorkspaceRewritesRecordListPageUpdatedAtToCurrentTimeField(t *testing.T) {
	workspacePath := t.TempDir()
	recordPath := filepath.Join(workspacePath, "lib", "models", "record.dart")
	if err := os.MkdirAll(filepath.Dir(recordPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(recordPath) error = %v", err)
	}
	if err := os.WriteFile(recordPath, []byte(strings.Join([]string{
		"class AppRecord {",
		"  const AppRecord({required this.recordedAt});",
		"  final DateTime recordedAt;",
		"}",
	}, "\n")), 0o600); err != nil {
		t.Fatalf("WriteFile(record.dart) error = %v", err)
	}
	patch := appruns.WorkspacePatch{Operations: []appruns.WorkspacePatchOperation{{
		Type:    "write_file",
		Path:    "lib/views/record_list_page.dart",
		Content: "class RecordListPage {\n  String render(dynamic record) => record.updatedAt.toIso8601String();\n}\n",
	}}}

	normalizeBuilderRuntimePatchForWorkspace(workspacePath, &patch)

	content := patch.Operations[0].Content
	if strings.Contains(content, ".updatedAt") {
		t.Fatalf("normalizeBuilderRuntimePatchForWorkspace() content still references updatedAt in record_list_page: %q", content)
	}
	if !strings.Contains(content, "record.recordedAt.toIso8601String()") {
		t.Fatalf("normalizeBuilderRuntimePatchForWorkspace() should rewrite record_list_page to recordedAt: %q", content)
	}
}

func TestNormalizeBuilderRuntimePatchForWorkspaceRemovesRecordListPageUpdatedAtWidgetWhenNoTimeFieldExists(t *testing.T) {
	workspacePath := t.TempDir()
	recordPath := filepath.Join(workspacePath, "lib", "models", "record.dart")
	if err := os.MkdirAll(filepath.Dir(recordPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(recordPath) error = %v", err)
	}
	if err := os.WriteFile(recordPath, []byte(strings.Join([]string{
		"class AppRecord {",
		"  const AppRecord({required this.taskId});",
		"  final String taskId;",
		"}",
	}, "\n")), 0o600); err != nil {
		t.Fatalf("WriteFile(record.dart) error = %v", err)
	}
	patch := appruns.WorkspacePatch{Operations: []appruns.WorkspacePatchOperation{{
		Type: "write_file",
		Path: "lib/views/record_list_page.dart",
		Content: strings.Join([]string{
			"class RecordListPage {",
			"  Widget build(dynamic record) {",
			"    return Row(",
			"      children: [",
			"        Text(",
			"          '${record.updatedAt.year}-${record.updatedAt.month.toString().padLeft(2, '0')}-${record.updatedAt.day.toString().padLeft(2, '0')}',",
			"          style: const TextStyle(fontWeight: FontWeight.w600),",
			"        ),",
			"      ],",
			"    );",
			"  }",
			"}",
		}, "\n"),
	}}}

	normalizeBuilderRuntimePatchForWorkspace(workspacePath, &patch)

	content := patch.Operations[0].Content
	if strings.Contains(content, ".updatedAt") {
		t.Fatalf("normalizeBuilderRuntimePatchForWorkspace() should remove updatedAt widget from record_list_page when no time field exists: %q", content)
	}
	if !strings.Contains(content, "const SizedBox.shrink(),") {
		t.Fatalf("normalizeBuilderRuntimePatchForWorkspace() should replace the record_list_page date widget with SizedBox.shrink(): %q", content)
	}
}

func TestNormalizeBuilderRuntimePatchForWorkspaceRemovesRecordDetailPageUpdatedAtTileWhenNoTimeFieldExists(t *testing.T) {
	workspacePath := t.TempDir()
	recordPath := filepath.Join(workspacePath, "lib", "models", "record.dart")
	if err := os.MkdirAll(filepath.Dir(recordPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(recordPath) error = %v", err)
	}
	if err := os.WriteFile(recordPath, []byte(strings.Join([]string{
		"class AppRecord {",
		"  const AppRecord({required this.taskId});",
		"  final String taskId;",
		"}",
	}, "\n")), 0o600); err != nil {
		t.Fatalf("WriteFile(record.dart) error = %v", err)
	}
	patch := appruns.WorkspacePatch{Operations: []appruns.WorkspacePatchOperation{{
		Type: "write_file",
		Path: "lib/views/record_detail_page.dart",
		Content: strings.Join([]string{
			"class RecordDetailPage {",
			"  Widget build() {",
			"    return ListView(",
			"      children: [",
			"        _InfoTile(",
			"          label: openLiteCopy.detailDateLabel,",
			"          value:",
			"              '${_record.updatedAt.year}-${_record.updatedAt.month.toString().padLeft(2, '0')}-${_record.updatedAt.day.toString().padLeft(2, '0')}',",
			"        ),",
			"      ],",
			"    );",
			"  }",
			"}",
		}, "\n"),
	}}}

	normalizeBuilderRuntimePatchForWorkspace(workspacePath, &patch)

	content := patch.Operations[0].Content
	if strings.Contains(content, ".updatedAt") {
		t.Fatalf("normalizeBuilderRuntimePatchForWorkspace() should remove updatedAt tile from record_detail_page when no time field exists: %q", content)
	}
	if !strings.Contains(content, "const SizedBox.shrink(),") {
		t.Fatalf("normalizeBuilderRuntimePatchForWorkspace() should replace the record_detail_page date tile with SizedBox.shrink(): %q", content)
	}
}

func TestNormalizeBuilderRuntimeRecordDetailPageContentUsesCustomPrimaryRecordModelTimeField(t *testing.T) {
	workspacePath := t.TempDir()
	writeBuilderRuntimeWorkspaceFiles(t, workspacePath, map[string]string{
		"lib/models/task.dart": strings.Join([]string{
			"class Task {",
			"  const Task({required this.occurredOn});",
			"  final DateTime occurredOn;",
			"}",
		}, "\n") + "\n",
		"lib/views/task_collection_page.dart": strings.Join([]string{
			"import '../models/task.dart';",
			"",
			"class TaskCollectionPage {",
			"  const TaskCollectionPage({required this.controller, required this.onOpenTaskDetail});",
			"  final Object controller;",
			"  final Future<void> Function(Task task) onOpenTaskDetail;",
			"}",
		}, "\n") + "\n",
	})
	content := strings.Join([]string{
		"class TaskDetailPage {",
		"  Widget build() {",
		"    return ListView(",
		"      children: [",
		"        _InfoTile(",
		"          label: openLiteCopy.detailDateLabel,",
		"          value:",
		"              '${_record.updatedAt.year}-${_record.updatedAt.month.toString().padLeft(2, '0')}-${_record.updatedAt.day.toString().padLeft(2, '0')}',",
		"        ),",
		"      ],",
		"    );",
		"  }",
		"}",
	}, "\n")

	normalized := normalizeBuilderRuntimeRecordDetailPageContent(workspacePath, content)
	if strings.Contains(normalized, ".updatedAt") {
		t.Fatalf("normalizeBuilderRuntimeRecordDetailPageContent() should not keep updatedAt when custom primary model exposes occurredOn: %q", normalized)
	}
	if !strings.Contains(normalized, ".occurredOn") {
		t.Fatalf("normalizeBuilderRuntimeRecordDetailPageContent() should rewrite updatedAt to occurredOn for custom primary model: %q", normalized)
	}
}

func TestNormalizeBuilderRuntimePatchForWorkspaceRemovesDetailTileWithoutFixedDetailName(t *testing.T) {
	workspacePath := t.TempDir()
	recordPath := filepath.Join(workspacePath, "lib", "models", "record.dart")
	if err := os.MkdirAll(filepath.Dir(recordPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(recordPath) error = %v", err)
	}
	if err := os.WriteFile(recordPath, []byte(strings.Join([]string{
		"class AppRecord {",
		"  const AppRecord({required this.taskId});",
		"  final String taskId;",
		"}",
	}, "\n")), 0o600); err != nil {
		t.Fatalf("WriteFile(record.dart) error = %v", err)
	}
	patch := appruns.WorkspacePatch{Operations: []appruns.WorkspacePatchOperation{{
		Type: "write_file",
		Path: "lib/views/inspection_surface.dart",
		Content: strings.Join([]string{
			"class InspectionSurface {",
			"  Widget build() {",
			"    return ListView(",
			"      children: [",
			"        _InfoTile(",
			"          label: openLiteCopy.detailDateLabel,",
			"          value:",
			"              '${_record.updatedAt.year}-${_record.updatedAt.month.toString().padLeft(2, '0')}-${_record.updatedAt.day.toString().padLeft(2, '0')}',",
			"        ),",
			"      ],",
			"    );",
			"  }",
			"}",
		}, "\n"),
	}}}

	normalizeBuilderRuntimePatchForWorkspace(workspacePath, &patch)

	content := patch.Operations[0].Content
	if strings.Contains(content, ".updatedAt") {
		t.Fatalf("normalizeBuilderRuntimePatchForWorkspace() should remove detail tile updatedAt on arbitrary view path: %q", content)
	}
	if !strings.Contains(content, "const SizedBox.shrink(),") {
		t.Fatalf("normalizeBuilderRuntimePatchForWorkspace() should replace arbitrary inspection view date tile with SizedBox.shrink(): %q", content)
	}
}

func TestNormalizeBuilderRuntimePatchForWorkspaceRemovesDetailTileWithoutFixedRecordVariableName(t *testing.T) {
	workspacePath := t.TempDir()
	recordPath := filepath.Join(workspacePath, "lib", "models", "record.dart")
	if err := os.MkdirAll(filepath.Dir(recordPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(recordPath) error = %v", err)
	}
	if err := os.WriteFile(recordPath, []byte(strings.Join([]string{
		"class AppRecord {",
		"  const AppRecord({required this.taskId});",
		"  final String taskId;",
		"}",
	}, "\n")), 0o600); err != nil {
		t.Fatalf("WriteFile(record.dart) error = %v", err)
	}
	patch := appruns.WorkspacePatch{Operations: []appruns.WorkspacePatchOperation{{
		Type: "write_file",
		Path: "lib/views/inspection_surface.dart",
		Content: strings.Join([]string{
			"class InspectionSurface {",
			"  Widget build() {",
			"    return ListView(",
			"      children: [",
			"        _InfoTile(",
			"          label: openLiteCopy.detailDateLabel,",
			"          value:",
			"              '${task.updatedAt.year}-${task.updatedAt.month.toString().padLeft(2, '0')}-${task.updatedAt.day.toString().padLeft(2, '0')}',",
			"        ),",
			"      ],",
			"    );",
			"  }",
			"}",
		}, "\n"),
	}}}

	normalizeBuilderRuntimePatchForWorkspace(workspacePath, &patch)

	content := patch.Operations[0].Content
	if strings.Contains(content, ".updatedAt") {
		t.Fatalf("normalizeBuilderRuntimePatchForWorkspace() should remove detail tile updatedAt when the record variable name is not _record: %q", content)
	}
	if !strings.Contains(content, "const SizedBox.shrink(),") {
		t.Fatalf("normalizeBuilderRuntimePatchForWorkspace() should replace arbitrary detail variable date tile with SizedBox.shrink(): %q", content)
	}
}

func TestNormalizeBuilderRuntimePatchForWorkspaceRepairsAndroidBuildGradleFlutterSource(t *testing.T) {
	workspacePath := t.TempDir()
	patch := appruns.WorkspacePatch{Operations: []appruns.WorkspacePatchOperation{{
		Type: "write_file",
		Path: "android/app/build.gradle.kts",
		Content: strings.Join([]string{
			"val defaultOpenLiteApplicationId = \"com.picoclaw.appfactory.flutter_open_lite\"",
			"",
			"android {",
			"    namespace = defaultOpenLiteApplication",
			"}",
			"",
			"flutter {",
			"    source = \"..\"..",
			"}",
		}, "\n"),
	}}}

	normalizeBuilderRuntimePatchForWorkspace(workspacePath, &patch)

	content := patch.Operations[0].Content
	if strings.Contains(content, `source = ".."..`) {
		t.Fatalf("normalizeBuilderRuntimePatchForWorkspace() should repair malformed flutter source path: %q", content)
	}
	if !strings.Contains(content, `source = "../.."`) {
		t.Fatalf("normalizeBuilderRuntimePatchForWorkspace() should preserve template flutter source path: %q", content)
	}
	if regexp.MustCompile(`(?m)^\s*namespace\s*=\s*defaultOpenLiteApplication\s*$`).MatchString(content) {
		t.Fatalf("normalizeBuilderRuntimePatchForWorkspace() should repair malformed namespace identifier: %q", content)
	}
	if !strings.Contains(content, `namespace = defaultOpenLiteApplicationId`) {
		t.Fatalf("normalizeBuilderRuntimePatchForWorkspace() should preserve template namespace identifier: %q", content)
	}
}

func mustFindPromptTaskBundleItem(t *testing.T, tasks []appruns.TaskBundleItem, taskID string) appruns.TaskBundleItem {
	t.Helper()
	for _, task := range tasks {
		if strings.TrimSpace(task.TaskID) == strings.TrimSpace(taskID) {
			return task
		}
	}
	t.Fatalf("task %q not found in task bundle %#v", taskID, tasks)
	return appruns.TaskBundleItem{}
}

func buildCompiledPromptTestRun(t *testing.T, requirementText, requirementSource string) (runRecord, appruns.RoundInput) {
	t.Helper()
	bundle, err := appprepare.Compile(appprepare.Request{
		RequirementText:   requirementText,
		RequirementSource: requirementSource,
		Now: func() time.Time {
			return time.Date(2026, 4, 10, 12, 0, 0, 0, time.UTC)
		},
	})
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	jobRoot := t.TempDir()
	workspacePath := filepath.Join(jobRoot, "workspace")
	prepareDir := filepath.Join(jobRoot, "prepare")
	if err := os.MkdirAll(workspacePath, 0o755); err != nil {
		t.Fatalf("MkdirAll(workspacePath) error = %v", err)
	}
	if err := appprepare.WriteBundle(prepareDir, bundle); err != nil {
		t.Fatalf("WriteBundle(%s) error = %v", prepareDir, err)
	}
	run := runRecord{
		GoalSummary:      bundle.BuilderInput.GoalSummary,
		PlanningPolicy:   bundle.BuilderInput.PlanningPolicy,
		HumanNotes:       append(json.RawMessage(nil), bundle.BuilderInput.HumanNotes...),
		TaskBundle:       append([]appruns.TaskBundleItem(nil), bundle.BuilderInput.TaskBundle...),
		AcceptanceChecks: append([]appruns.AcceptanceCheck(nil), bundle.BuilderInput.AcceptanceChecks...),
		AllowedPaths:     append([]string(nil), bundle.BuilderInput.AllowedPaths...),
		ProtectedPaths:   append([]string(nil), bundle.BuilderInput.ProtectedPaths...),
		KnowledgePack:    append([]appruns.ProfileSkill(nil), bundle.BuilderInput.KnowledgePack...),
		WorkspacePath:    workspacePath,
		ArtifactDir:      filepath.Join(jobRoot, "artifacts"),
	}
	return run, BuildRoundInputForTest(run)
}

func TestBuildBuilderRuntimePromptIncludesDartIdentifierGuidanceFromDomainModel(t *testing.T) {
	jobRoot := t.TempDir()
	workspacePath := filepath.Join(jobRoot, "workspace")
	prepareDir := filepath.Join(jobRoot, "prepare")
	if err := os.MkdirAll(prepareDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(prepareDir) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(prepareDir, "domain-model.json"), []byte(`{"entities":[{"fields":[{"name":"record_id"},{"name":"recorded_at"}]}]}`), 0o600); err != nil {
		t.Fatalf("WriteFile(domain-model.json) error = %v", err)
	}
	run := runRecord{
		GoalSummary:   "stabilize weight models",
		WorkspacePath: workspacePath,
		TaskBundle: []appruns.TaskBundleItem{{
			TaskID:      "task-models",
			TaskType:    appruns.BuilderRuntimeTaskTypeDualFileWiring,
			TargetPaths: []string{"lib/models/record.dart", "lib/models/dashboard_summary.dart"},
		}},
	}
	roundInput := appruns.RoundInput{
		TaskBundle:   run.TaskBundle,
		AllowedPaths: []string{"lib/**"},
	}
	route := appruns.BuilderRuntimeTaskRoute{
		TaskID:      "task-models",
		TaskType:    appruns.BuilderRuntimeTaskTypeDualFileWiring,
		RouteSource: "default_model",
	}
	prompt, err := buildBuilderRuntimePrompt(BuilderRuntimePatchRequest{Run: run, RoundInput: roundInput, Route: route})
	if err != nil {
		t.Fatalf("buildBuilderRuntimePrompt() error = %v", err)
	}
	if !strings.Contains(prompt, "fields/getters/parameters stay lowerCamelCase") {
		t.Fatalf("prompt missing Dart identifier mapping guidance: %q", prompt)
	}
	if !strings.Contains(prompt, "Do not rename exported types in lib/models/*.dart") {
		t.Fatalf("prompt missing exported model type stability guidance: %q", prompt)
	}
}

func TestBuildBuilderRuntimePromptKeepsCurrentRoundChecksWithoutFullTaskBundle(t *testing.T) {
	run := runRecord{
		GoalSummary:   "repair widget wiring",
		WorkspacePath: t.TempDir(),
		TaskBundle: []appruns.TaskBundleItem{{
			TaskID:      "task-flow",
			TaskType:    appruns.BuilderRuntimeTaskTypeDualFileWiring,
			TargetPaths: []string{"lib/controllers/home_controller.dart", "test/widget_test.dart"},
		}},
	}
	roundInput := appruns.RoundInput{
		TaskBundle: run.TaskBundle,
		AcceptanceChecks: []appruns.AcceptanceCheck{{
			CheckID:         "check-profile-open-lite-domain-language",
			Label:           "确认领域字段与文案已进入 open-lite 工作区",
			SuccessCriteria: "领域文案与字段命名已经落地",
		}},
		AllowedPaths: []string{"lib/**", "test/**"},
	}
	route := appruns.BuilderRuntimeTaskRoute{
		TaskID:      "task-flow",
		TaskType:    appruns.BuilderRuntimeTaskTypeDualFileWiring,
		RouteSource: "default_model",
	}

	prompt, err := buildBuilderRuntimePrompt(BuilderRuntimePatchRequest{Run: run, RoundInput: roundInput, Route: route})
	if err != nil {
		t.Fatalf("buildBuilderRuntimePrompt() error = %v", err)
	}
	if !strings.Contains(prompt, "Current round acceptance checks JSON: [{\"check_id\":\"check-profile-open-lite-domain-language\"") {
		t.Fatalf("prompt missing current round acceptance checks payload: %q", prompt)
	}
	if !strings.Contains(prompt, "Allowed paths JSON: [\"lib/controllers/home_controller.dart\",\"test/widget_test.dart\"]") {
		t.Fatalf("prompt missing allowed paths payload: %q", prompt)
	}
	if strings.Contains(prompt, "Task bundle JSON:") {
		t.Fatalf("prompt should not contain full task bundle dump: %q", prompt)
	}
	if !strings.Contains(prompt, "Current allocation task JSON: {\"task_id\":\"task-flow\"") {
		t.Fatalf("prompt missing current allocation task json: %q", prompt)
	}
}

func TestBuildBuilderRuntimePromptRestrictsPrunedGenericFlows(t *testing.T) {
	run := runRecord{
		GoalSummary:   "build a list-plus-drawer todo app without home detail filter or delete",
		WorkspacePath: t.TempDir(),
		TaskBundle: []appruns.TaskBundleItem{
			{TaskID: "task-create-repository", TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, TargetPaths: []string{"lib/repositories/record_repository.dart"}},
			{TaskID: "task-create-form-controller", TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, TargetPaths: []string{"lib/controllers/record_form_controller.dart"}, AllocationTransition: &appruns.TaskAllocationTransition{SemanticIntentRefs: []string{"ac-form"}}},
			{TaskID: "task-create-list-controller", TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, TargetPaths: []string{"lib/controllers/record_list_controller.dart"}, AllocationTransition: &appruns.TaskAllocationTransition{SemanticIntentRefs: []string{"ac-list"}}},
			{TaskID: "task-bind-collection-surface", TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, TargetPaths: []string{"lib/views/record_list_page.dart"}, AllocationTransition: &appruns.TaskAllocationTransition{SemanticIntentRefs: []string{"ac-list"}, SurfaceRefs: []string{"surface-collection"}}},
			{TaskID: "task-bind-mutation-surface", TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, TargetPaths: []string{"lib/views/record_form_page.dart"}, AllocationTransition: &appruns.TaskAllocationTransition{SemanticIntentRefs: []string{"ac-form"}, SurfaceRefs: []string{"surface-mutation"}}},
			{TaskID: "task-bind-app-entry", TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, TargetPaths: []string{"lib/main.dart"}, Dependencies: []string{"task-create-repository", "task-create-form-controller", "task-create-list-controller", "task-bind-collection-surface", "task-bind-mutation-surface"}, AllocationTransition: &appruns.TaskAllocationTransition{SemanticIntentRefs: []string{"ac-navigation", "ac-form"}, SurfaceRefs: []string{"surface-collection", "surface-mutation"}, OwnedPaths: []string{"lib/main.dart"}}},
			{TaskID: "task-create-test", TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, TargetPaths: []string{"test/widget_test.dart"}, Dependencies: []string{"task-bind-app-entry"}, AllocationTransition: &appruns.TaskAllocationTransition{SemanticIntentRefs: []string{"ac-navigation", "ac-form"}, SurfaceRefs: []string{"surface-collection", "surface-mutation"}, OwnedPaths: []string{"test/widget_test.dart"}}},
		},
	}
	roundInput := appruns.RoundInput{
		TaskBundle:   run.TaskBundle,
		AllowedPaths: []string{"lib/**", "test/**"},
	}
	route := appruns.BuilderRuntimeTaskRoute{
		TaskID:      "task-create-test",
		TaskType:    appruns.BuilderRuntimeTaskTypeDualFileWiring,
		RouteSource: "default_model",
	}
	prompt, err := buildBuilderRuntimePrompt(BuilderRuntimePatchRequest{Run: run, RoundInput: roundInput, Route: route})
	if err != nil {
		t.Fatalf("buildBuilderRuntimePrompt() error = %v", err)
	}
	for _, want := range []string{
		"Current topology does not include an overview/home flow.",
		"Current topology does not include a standalone detail flow.",
		"Current topology does not include filtering.",
		"Current topology does not include delete behavior.",
		"keep the list as an unfiltered collection",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing pruned-topology guidance %q: %q", want, prompt)
		}
	}
}

func TestBuildBuilderRuntimePromptRestrictsPrunedGenericFlowsFromSurfaceRefsWithoutDefaultFileNames(t *testing.T) {
	run := runRecord{
		GoalSummary:   "build a list-plus-drawer todo app without home detail filter or delete",
		WorkspacePath: t.TempDir(),
		TaskBundle: []appruns.TaskBundleItem{
			{TaskID: "task-create-repository", TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, TargetPaths: []string{"lib/repositories/task_repository.dart"}},
			{TaskID: "task-create-form-controller", TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, TargetPaths: []string{"lib/controllers/task_form_controller.dart"}, AllocationTransition: &appruns.TaskAllocationTransition{SemanticIntentRefs: []string{"ac-form"}, SurfaceRefs: []string{builderRuntimeMutationSurfaceRef}}},
			{TaskID: "task-create-list-controller", TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, TargetPaths: []string{"lib/controllers/task_collection_controller.dart"}, AllocationTransition: &appruns.TaskAllocationTransition{SemanticIntentRefs: []string{"ac-list"}, SurfaceRefs: []string{builderRuntimeCollectionSurfaceRef}}},
			{TaskID: "task-bind-collection-surface", TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, TargetPaths: []string{"lib/views/task_collection_page.dart"}, AllocationTransition: &appruns.TaskAllocationTransition{SemanticIntentRefs: []string{"ac-list"}, SurfaceRefs: []string{builderRuntimeCollectionSurfaceRef}}},
			{TaskID: "task-bind-mutation-surface", TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, TargetPaths: []string{"lib/views/task_form_page.dart"}, AllocationTransition: &appruns.TaskAllocationTransition{SemanticIntentRefs: []string{"ac-form"}, SurfaceRefs: []string{builderRuntimeMutationSurfaceRef}}},
			{TaskID: "task-bind-app-entry", TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, TargetPaths: []string{"lib/main.dart"}, Dependencies: []string{"task-create-repository", "task-create-form-controller", "task-create-list-controller", "task-bind-collection-surface", "task-bind-mutation-surface"}, AllocationTransition: &appruns.TaskAllocationTransition{SemanticIntentRefs: []string{"ac-navigation", "ac-form"}, SurfaceRefs: []string{builderRuntimeCollectionSurfaceRef, builderRuntimeMutationSurfaceRef}, OwnedPaths: []string{"lib/main.dart"}}},
			{TaskID: "task-create-test", TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, TargetPaths: []string{"test/widget_test.dart"}, Dependencies: []string{"task-bind-app-entry"}, AllocationTransition: &appruns.TaskAllocationTransition{SemanticIntentRefs: []string{"ac-navigation", "ac-form"}, SurfaceRefs: []string{builderRuntimeCollectionSurfaceRef, builderRuntimeMutationSurfaceRef}, OwnedPaths: []string{"test/widget_test.dart"}}},
		},
	}
	roundInput := appruns.RoundInput{
		TaskBundle:   run.TaskBundle,
		AllowedPaths: []string{"lib/**", "test/**"},
	}
	route := appruns.BuilderRuntimeTaskRoute{
		TaskID:      "task-create-test",
		TaskType:    appruns.BuilderRuntimeTaskTypeDualFileWiring,
		RouteSource: "default_model",
	}
	prompt, err := buildBuilderRuntimePrompt(BuilderRuntimePatchRequest{Run: run, RoundInput: roundInput, Route: route})
	if err != nil {
		t.Fatalf("buildBuilderRuntimePrompt() error = %v", err)
	}
	for _, want := range []string{
		"Current topology does not include an overview/home flow.",
		"Current topology does not include a standalone detail flow.",
		"Current topology does not include filtering.",
		"Current topology does not include delete behavior.",
		"keep the list as an unfiltered collection",
		"Because the current topology boots directly into the collection surface",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing surface-ref-driven pruned-topology guidance %q: %q", want, prompt)
		}
	}
}

func TestBuildBuilderRuntimePromptOmitsStandaloneDetailWarningForCustomDetailPathWithoutSurfaceRefs(t *testing.T) {
	run := runRecord{
		GoalSummary:   "build a list-plus-detail todo app without home filter or delete",
		WorkspacePath: t.TempDir(),
		TaskBundle: []appruns.TaskBundleItem{
			{TaskID: "task-create-repository", TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, TargetPaths: []string{"lib/repositories/task_repository.dart"}},
			{TaskID: "task-bind-collection-surface", TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, TargetPaths: []string{"lib/views/record_list_page.dart", "lib/controllers/record_list_controller.dart"}},
			{TaskID: "task-bind-detail-surface", TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, TargetPaths: []string{"lib/views/task_detail_page.dart"}},
			{TaskID: "task-create-test", TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, TargetPaths: []string{"test/widget_test.dart"}, Dependencies: []string{"task-bind-collection-surface", "task-bind-detail-surface"}},
		},
	}
	roundInput := appruns.RoundInput{
		TaskBundle:   run.TaskBundle,
		AllowedPaths: []string{"lib/**", "test/**"},
	}
	route := appruns.BuilderRuntimeTaskRoute{
		TaskID:      "task-create-test",
		TaskType:    appruns.BuilderRuntimeTaskTypeDualFileWiring,
		RouteSource: "default_model",
	}
	prompt, err := buildBuilderRuntimePrompt(BuilderRuntimePatchRequest{Run: run, RoundInput: roundInput, Route: route})
	if err != nil {
		t.Fatalf("buildBuilderRuntimePrompt() error = %v", err)
	}
	if strings.Contains(prompt, "Current topology does not include a standalone detail flow.") {
		t.Fatalf("buildBuilderRuntimePrompt() should not warn about missing detail flow when task bundle already contains a custom detail view path: %q", prompt)
	}
	if !strings.Contains(prompt, "Current topology does not include an overview/home flow.") {
		t.Fatalf("buildBuilderRuntimePrompt() should still keep unrelated topology pruning guidance: %q", prompt)
	}
}

func TestBuildBuilderRuntimePromptOmitsStandaloneDetailWarningForInspectionPathWithoutSurfaceRefs(t *testing.T) {
	run := runRecord{
		GoalSummary:   "build a list-plus-inspection todo app without home filter or delete",
		WorkspacePath: t.TempDir(),
		TaskBundle: []appruns.TaskBundleItem{
			{TaskID: "task-create-repository", TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, TargetPaths: []string{"lib/repositories/task_repository.dart"}},
			{TaskID: "task-bind-collection-surface", TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, TargetPaths: []string{"lib/views/record_list_page.dart", "lib/controllers/record_list_controller.dart"}},
			{TaskID: "task-bind-inspection-surface", TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, TargetPaths: []string{"lib/views/inspection_surface.dart"}},
			{TaskID: "task-create-test", TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, TargetPaths: []string{"test/widget_test.dart"}, Dependencies: []string{"task-bind-collection-surface", "task-bind-inspection-surface"}},
		},
	}
	roundInput := appruns.RoundInput{
		TaskBundle:   run.TaskBundle,
		AllowedPaths: []string{"lib/**", "test/**"},
	}
	route := appruns.BuilderRuntimeTaskRoute{
		TaskID:      "task-create-test",
		TaskType:    appruns.BuilderRuntimeTaskTypeDualFileWiring,
		RouteSource: "default_model",
	}
	prompt, err := buildBuilderRuntimePrompt(BuilderRuntimePatchRequest{Run: run, RoundInput: roundInput, Route: route})
	if err != nil {
		t.Fatalf("buildBuilderRuntimePrompt() error = %v", err)
	}
	if strings.Contains(prompt, "Current topology does not include a standalone detail flow.") {
		t.Fatalf("buildBuilderRuntimePrompt() should not warn about missing detail flow when task bundle already contains an inspection-named detail view path: %q", prompt)
	}
	if !strings.Contains(prompt, "Current topology does not include an overview/home flow.") {
		t.Fatalf("buildBuilderRuntimePrompt() should still keep unrelated topology pruning guidance for inspection-named detail paths: %q", prompt)
	}
}

func TestBuildBuilderRuntimePromptUsesWorkspaceFallbackForCustomCollectionPathWithoutSurfaceRefs(t *testing.T) {
	run := runRecord{
		GoalSummary:   "build a collection-root todo app without home filter or delete",
		WorkspacePath: t.TempDir(),
		TaskBundle: []appruns.TaskBundleItem{
			{TaskID: "task-create-repository", TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, TargetPaths: []string{"lib/repositories/task_repository.dart"}},
			{TaskID: "task-bind-collection-surface", TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, TargetPaths: []string{"lib/views/task_collection_page.dart", "lib/controllers/task_collection_controller.dart"}},
			{TaskID: "task-bind-mutation-surface", TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, TargetPaths: []string{"lib/views/task_form_page.dart", "lib/controllers/task_form_controller.dart"}},
			{TaskID: "task-create-test", TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, TargetPaths: []string{"test/widget_test.dart"}, Dependencies: []string{"task-bind-collection-surface", "task-bind-mutation-surface"}},
		},
	}
	roundInput := appruns.RoundInput{
		TaskBundle:   run.TaskBundle,
		AllowedPaths: []string{"lib/**", "test/**"},
	}
	route := appruns.BuilderRuntimeTaskRoute{
		TaskID:      "task-create-test",
		TaskType:    appruns.BuilderRuntimeTaskTypeDualFileWiring,
		RouteSource: "default_model",
	}
	prompt, err := buildBuilderRuntimePrompt(BuilderRuntimePatchRequest{Run: run, RoundInput: roundInput, Route: route})
	if err != nil {
		t.Fatalf("buildBuilderRuntimePrompt() error = %v", err)
	}
	for _, want := range []string{
		"Current topology does not include an overview/home flow.",
		"Because the current topology boots directly into the collection surface",
		"keep the list as an unfiltered collection",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("buildBuilderRuntimePrompt() should use workspace fallback for custom collection paths without surface refs and include %q: %q", want, prompt)
		}
	}
}

func TestBuildBuilderRuntimePromptDoesNotInferCollectionFallbackWhenMutationSurfaceRefIsExplicit(t *testing.T) {
	run := runRecord{
		GoalSummary:   "build a mutation-only todo flow without overview collection filter or delete",
		WorkspacePath: t.TempDir(),
		TaskBundle: []appruns.TaskBundleItem{
			{TaskID: "task-create-repository", TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, TargetPaths: []string{"lib/repositories/task_repository.dart"}},
			{TaskID: "task-bind-implicit-collection-path", TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, TargetPaths: []string{"lib/views/task_collection_page.dart"}},
			{TaskID: "task-bind-mutation-surface", TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, TargetPaths: []string{"lib/views/task_form_page.dart"}, AllocationTransition: &appruns.TaskAllocationTransition{SurfaceRefs: []string{builderRuntimeMutationSurfaceRef}}},
			{TaskID: "task-create-test", TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, TargetPaths: []string{"test/widget_test.dart"}, Dependencies: []string{"task-bind-mutation-surface"}},
		},
	}
	roundInput := appruns.RoundInput{
		TaskBundle:   run.TaskBundle,
		AllowedPaths: []string{"lib/**", "test/**"},
	}
	route := appruns.BuilderRuntimeTaskRoute{
		TaskID:      "task-create-test",
		TaskType:    appruns.BuilderRuntimeTaskTypeDualFileWiring,
		RouteSource: "default_model",
	}
	prompt, err := buildBuilderRuntimePrompt(BuilderRuntimePatchRequest{Run: run, RoundInput: roundInput, Route: route})
	if err != nil {
		t.Fatalf("buildBuilderRuntimePrompt() error = %v", err)
	}
	if strings.Contains(prompt, "Because the current topology boots directly into the collection surface") {
		t.Fatalf("buildBuilderRuntimePrompt() should not infer collection-root guidance from likely custom collection paths once mutation surface refs make topology explicit: %q", prompt)
	}
	if !strings.Contains(prompt, "Current topology does not include an overview/home flow.") {
		t.Fatalf("buildBuilderRuntimePrompt() should still keep unrelated pruning guidance when only mutation surface refs are explicit: %q", prompt)
	}
}

func TestBuilderRuntimeNeedsCollectionCreateEntryUsesSurfaceRefsWithoutDefaultFileNames(t *testing.T) {
	taskBundle := []appruns.TaskBundleItem{
		{TaskID: "task-create-list-controller", TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, TargetPaths: []string{"lib/controllers/task_collection_controller.dart"}, AllocationTransition: &appruns.TaskAllocationTransition{SurfaceRefs: []string{builderRuntimeCollectionSurfaceRef}}},
		{TaskID: "task-bind-collection-surface", TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, TargetPaths: []string{"lib/views/task_collection_page.dart"}, AllocationTransition: &appruns.TaskAllocationTransition{SurfaceRefs: []string{builderRuntimeCollectionSurfaceRef}}},
		{TaskID: "task-bind-mutation-surface", TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, TargetPaths: []string{"lib/views/task_form_page.dart"}, AllocationTransition: &appruns.TaskAllocationTransition{SurfaceRefs: []string{builderRuntimeMutationSurfaceRef}}},
	}
	if !builderRuntimeNeedsCollectionCreateEntry(taskBundle, t.TempDir()) {
		t.Fatalf("builderRuntimeNeedsCollectionCreateEntry() should use surface refs for custom file names")
	}
}

func TestBuilderRuntimeNeedsCollectionCreateEntrySkipsWhenOverviewSurfaceExistsViaSurfaceRefs(t *testing.T) {
	taskBundle := []appruns.TaskBundleItem{
		{TaskID: "task-bind-overview-surface", TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, TargetPaths: []string{"lib/views/task_home_page.dart"}, AllocationTransition: &appruns.TaskAllocationTransition{SurfaceRefs: []string{builderRuntimeOverviewSurfaceRef}}},
		{TaskID: "task-bind-collection-surface", TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, TargetPaths: []string{"lib/views/task_collection_page.dart"}, AllocationTransition: &appruns.TaskAllocationTransition{SurfaceRefs: []string{builderRuntimeCollectionSurfaceRef}}},
		{TaskID: "task-bind-mutation-surface", TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, TargetPaths: []string{"lib/views/task_form_page.dart"}, AllocationTransition: &appruns.TaskAllocationTransition{SurfaceRefs: []string{builderRuntimeMutationSurfaceRef}}},
	}
	if builderRuntimeNeedsCollectionCreateEntry(taskBundle, t.TempDir()) {
		t.Fatalf("builderRuntimeNeedsCollectionCreateEntry() should respect overview surface refs for custom file names")
	}
}

func TestBuilderRuntimeNeedsCollectionCreateEntryFallsBackToWorkspaceForCustomPathsWithoutSurfaceRefs(t *testing.T) {
	workspacePath := t.TempDir()
	writeBuilderRuntimeWorkspaceFiles(t, workspacePath, map[string]string{
		"lib/repositories/task_repository.dart":           "abstract class TaskRepository {}\n",
		"lib/controllers/task_collection_controller.dart": "import '../repositories/task_repository.dart';\nclass TaskCollectionController {\n  TaskCollectionController({required TaskRepository taskRepository});\n  Future<void> refresh() async {}\n}\n",
		"lib/controllers/task_form_controller.dart":       "class TaskFormController {}\n",
		"lib/views/task_form_page.dart":                   "class TaskFormPage {}\n",
	})
	taskBundle := []appruns.TaskBundleItem{
		{TaskID: "task-bind-collection-surface", TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, TargetPaths: []string{"lib/views/task_collection_page.dart", "lib/controllers/task_collection_controller.dart"}},
		{TaskID: "task-bind-mutation-surface", TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, TargetPaths: []string{"lib/views/task_form_page.dart", "lib/controllers/task_form_controller.dart"}},
	}
	if !builderRuntimeNeedsCollectionCreateEntry(taskBundle, workspacePath) {
		t.Fatalf("builderRuntimeNeedsCollectionCreateEntry() should fall back to workspace detection for custom paths without surface refs")
	}
}

func TestBuilderRuntimeNeedsCollectionCreateEntryUsesLikelyCustomPathsBeforeWorkspaceExists(t *testing.T) {
	workspacePath := t.TempDir()
	taskBundle := []appruns.TaskBundleItem{
		{TaskID: "task-bind-collection-surface", TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, TargetPaths: []string{"lib/views/task_collection_page.dart", "lib/controllers/task_collection_controller.dart"}},
		{TaskID: "task-bind-mutation-surface", TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, TargetPaths: []string{"lib/views/task_form_page.dart", "lib/controllers/task_form_controller.dart"}},
	}
	if !builderRuntimeNeedsCollectionCreateEntry(taskBundle, workspacePath) {
		t.Fatalf("builderRuntimeNeedsCollectionCreateEntry() should treat likely custom collection/mutation paths as collection-root even before workspace files exist")
	}
}

func TestBuilderRuntimeNeedsCollectionCreateEntrySkipsWhenCustomOverviewExistsWithoutSurfaceRefs(t *testing.T) {
	workspacePath := t.TempDir()
	writeBuilderRuntimeWorkspaceFiles(t, workspacePath, map[string]string{
		"lib/views/task_home_page.dart":                   "class TaskHomePage {}\n",
		"lib/controllers/task_home_controller.dart":       "class TaskHomeController {}\n",
		"lib/controllers/task_collection_controller.dart": "class TaskCollectionController { Future<void> refresh() async {} }\n",
		"lib/controllers/task_form_controller.dart":       "class TaskFormController {}\n",
	})
	taskBundle := []appruns.TaskBundleItem{
		{TaskID: "task-bind-collection-surface", TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, TargetPaths: []string{"lib/views/task_collection_page.dart", "lib/controllers/task_collection_controller.dart"}},
		{TaskID: "task-bind-mutation-surface", TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, TargetPaths: []string{"lib/views/task_form_page.dart", "lib/controllers/task_form_controller.dart"}},
	}
	if builderRuntimeNeedsCollectionCreateEntry(taskBundle, workspacePath) {
		t.Fatalf("builderRuntimeNeedsCollectionCreateEntry() should respect custom overview workspace detection without surface refs")
	}
}

func TestValidateBuilderRuntimeTopologyScopedTaskOutputUsesSurfaceRefsWithoutDefaultFileNames(t *testing.T) {
	run := runRecord{
		WorkspacePath: t.TempDir(),
		TaskBundle: []appruns.TaskBundleItem{
			{TaskID: "task-bind-collection-surface", TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, TargetPaths: []string{"lib/views/task_collection_page.dart"}, AllocationTransition: &appruns.TaskAllocationTransition{SurfaceRefs: []string{builderRuntimeCollectionSurfaceRef}}},
			{TaskID: "task-bind-mutation-surface", TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, TargetPaths: []string{"lib/views/task_form_page.dart"}, AllocationTransition: &appruns.TaskAllocationTransition{SurfaceRefs: []string{builderRuntimeMutationSurfaceRef}}},
		},
	}
	err := validateBuilderRuntimeTopologyScopedTaskOutput(run, "lib/main.dart", []byte(strings.Join([]string{
		"import 'package:flutter/material.dart';",
		"class App extends StatelessWidget {",
		"  const App({super.key});",
		"  @override",
		"  Widget build(BuildContext context) {",
		"    return const MaterialApp(home: Placeholder());",
		"  }",
		"}",
	}, "\n")))
	if err == nil {
		t.Fatalf("validateBuilderRuntimeTopologyScopedTaskOutput() should require collection-surface wiring when surface refs exist on custom file names")
	}
	if !strings.Contains(err.Error(), "did not wire the current collection surface into app entry") {
		t.Fatalf("validateBuilderRuntimeTopologyScopedTaskOutput() error = %v, want collection-surface wiring failure", err)
	}
}

func TestValidateBuilderRuntimeTopologyScopedTaskOutputAcceptsCustomCollectionSurfaceInAppEntry(t *testing.T) {
	workspacePath := t.TempDir()
	writeBuilderRuntimeWorkspaceFiles(t, workspacePath, map[string]string{
		"lib/views/task_collection_page.dart": strings.Join([]string{
			"import 'package:flutter/material.dart';",
			"",
			"class TaskCollectionPage extends StatelessWidget {",
			"  const TaskCollectionPage({super.key});",
			"",
			"  @override",
			"  Widget build(BuildContext context) => const SizedBox.shrink();",
			"}",
		}, "\n") + "\n",
	})
	run := runRecord{
		WorkspacePath: workspacePath,
		TaskBundle: []appruns.TaskBundleItem{
			{TaskID: "task-bind-collection-surface", TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, TargetPaths: []string{"lib/views/task_collection_page.dart"}, AllocationTransition: &appruns.TaskAllocationTransition{SurfaceRefs: []string{builderRuntimeCollectionSurfaceRef}}},
		},
	}
	err := validateBuilderRuntimeTopologyScopedTaskOutput(run, "lib/main.dart", []byte(strings.Join([]string{
		"import 'package:flutter/material.dart';",
		"import 'views/task_collection_page.dart';",
		"",
		"void main() {",
		"  runApp(const TaskLiteApp());",
		"}",
		"",
		"class TaskLiteApp extends StatelessWidget {",
		"  const TaskLiteApp({super.key});",
		"",
		"  @override",
		"  Widget build(BuildContext context) {",
		"    return const MaterialApp(home: TaskCollectionPage());",
		"  }",
		"}",
	}, "\n")))
	if err != nil {
		t.Fatalf("validateBuilderRuntimeTopologyScopedTaskOutput() error = %v, want custom collection surface app entry to satisfy wiring validation", err)
	}
}

func TestValidateBuilderRuntimeTopologyScopedTaskOutputAllowsPlaceholderDetailCallbackForCustomDetailPathWithoutSurfaceRefs(t *testing.T) {
	run := runRecord{
		WorkspacePath: t.TempDir(),
		TaskBundle: []appruns.TaskBundleItem{
			{TaskID: "task-bind-collection-surface", TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, TargetPaths: []string{"lib/views/record_list_page.dart", "lib/controllers/record_list_controller.dart"}},
			{TaskID: "task-bind-detail-surface", TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, TargetPaths: []string{"lib/views/task_detail_page.dart"}},
		},
	}
	err := validateBuilderRuntimeTopologyScopedTaskOutput(run, "lib/main.dart", []byte(strings.Join([]string{
		"import 'package:flutter/material.dart';",
		"class App extends StatelessWidget {",
		"  const App({super.key});",
		"  @override",
		"  Widget build(BuildContext context) {",
		"    return MaterialApp(",
		"      home: RecordListPage(",
		"        controller: Object(),",
		"        onOpenRecordDetail: (record) async {},",
		"      ),",
		"    );",
		"  }",
		"}",
	}, "\n")))
	if err != nil {
		t.Fatalf("validateBuilderRuntimeTopologyScopedTaskOutput() error = %v, want custom detail view path to satisfy detail-surface fallback without surface refs", err)
	}
}

func TestValidateBuilderRuntimeTopologyScopedTaskOutputAcceptsInspectionPathWithoutSurfaceRefs(t *testing.T) {
	run := runRecord{
		WorkspacePath: t.TempDir(),
		TaskBundle: []appruns.TaskBundleItem{
			{TaskID: "task-bind-collection-surface", TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, TargetPaths: []string{"lib/views/record_list_page.dart", "lib/controllers/record_list_controller.dart"}},
			{TaskID: "task-bind-inspection-surface", TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, TargetPaths: []string{"lib/views/inspection_surface.dart"}},
		},
	}
	err := validateBuilderRuntimeTopologyScopedTaskOutput(run, "lib/main.dart", []byte(strings.Join([]string{
		"import 'package:flutter/material.dart';",
		"class App extends StatelessWidget {",
		"  const App({super.key});",
		"  @override",
		"  Widget build(BuildContext context) {",
		"    return MaterialApp(",
		"      home: RecordListPage(",
		"        controller: Object(),",
		"        onOpenRecordDetail: (record) async {},",
		"      ),",
		"    );",
		"  }",
		"}",
	}, "\n")))
	if err != nil {
		t.Fatalf("validateBuilderRuntimeTopologyScopedTaskOutput() error = %v, want inspection-named detail view path to satisfy detail-surface fallback without surface refs", err)
	}
}

func TestPruneUnexpectedNoFilterCollectionControllerOperationsUsesSurfaceRefsWithoutDefaultFileNames(t *testing.T) {
	workspacePath := t.TempDir()
	controllerPath := filepath.Join(workspacePath, "lib", "controllers", "task_collection_controller.dart")
	if err := os.MkdirAll(filepath.Dir(controllerPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(controllerPath) error = %v", err)
	}
	if err := os.WriteFile(controllerPath, []byte("class TaskCollectionController {}\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(task_collection_controller.dart) error = %v", err)
	}
	taskBundle := []appruns.TaskBundleItem{
		{TaskID: "task-create-list-controller", TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, TargetPaths: []string{"lib/controllers/task_collection_controller.dart"}, AllocationTransition: &appruns.TaskAllocationTransition{SurfaceRefs: []string{builderRuntimeCollectionSurfaceRef}}},
		{TaskID: "task-bind-collection-surface", TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, TargetPaths: []string{"lib/views/task_collection_page.dart"}, AllocationTransition: &appruns.TaskAllocationTransition{SurfaceRefs: []string{builderRuntimeCollectionSurfaceRef}}},
	}
	operations := []appruns.WorkspacePatchOperation{
		{Type: "write_file", Path: "lib/views/task_collection_page.dart", Content: "class TaskCollectionPage {}\n"},
		{Type: "write_file", Path: "lib/controllers/task_collection_controller.dart", Content: "class TaskCollectionController {}\n"},
	}
	filtered := pruneUnexpectedNoFilterCollectionControllerOperations(workspacePath, taskBundle, operations)
	if len(filtered) != 1 {
		t.Fatalf("len(filtered) = %d, want 1", len(filtered))
	}
	if filtered[0].Path != "lib/views/task_collection_page.dart" {
		t.Fatalf("filtered[0].Path = %q, want collection view path", filtered[0].Path)
	}
}

func TestPruneUnexpectedNoFilterCollectionControllerOperationsUsesLikelyCustomPathsWithoutSurfaceRefs(t *testing.T) {
	workspacePath := t.TempDir()
	controllerPath := filepath.Join(workspacePath, "lib", "controllers", "task_collection_controller.dart")
	if err := os.MkdirAll(filepath.Dir(controllerPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(controllerPath) error = %v", err)
	}
	if err := os.WriteFile(controllerPath, []byte("class TaskCollectionController {}\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(task_collection_controller.dart) error = %v", err)
	}
	taskBundle := []appruns.TaskBundleItem{
		{TaskID: "task-create-list-controller", TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, TargetPaths: []string{"lib/controllers/task_collection_controller.dart"}},
		{TaskID: "task-bind-collection-surface", TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, TargetPaths: []string{"lib/views/task_collection_page.dart"}},
	}
	operations := []appruns.WorkspacePatchOperation{
		{Type: "write_file", Path: "lib/views/task_collection_page.dart", Content: "class TaskCollectionPage {}\n"},
		{Type: "write_file", Path: "lib/controllers/task_collection_controller.dart", Content: "class TaskCollectionController {}\n"},
	}
	filtered := pruneUnexpectedNoFilterCollectionControllerOperations(workspacePath, taskBundle, operations)
	if len(filtered) != 1 {
		t.Fatalf("len(filtered) = %d, want 1 for likely custom collection paths without surface refs", len(filtered))
	}
	if filtered[0].Path != "lib/views/task_collection_page.dart" {
		t.Fatalf("filtered[0].Path = %q, want likely custom collection view path", filtered[0].Path)
	}
}

func TestPruneUnexpectedDependentOperationsFromSingleTargetNonRepairPatchUsesSurfaceRefsWithoutDefaultFileNames(t *testing.T) {
	taskBundle := []appruns.TaskBundleItem{
		{TaskID: "task-create-list-controller", TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, TargetPaths: []string{"lib/controllers/task_collection_controller.dart"}, AllocationTransition: &appruns.TaskAllocationTransition{SurfaceRefs: []string{builderRuntimeCollectionSurfaceRef}}},
		{TaskID: "task-bind-collection-surface", TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, TargetPaths: []string{"lib/views/task_collection_page.dart"}, AllocationTransition: &appruns.TaskAllocationTransition{SurfaceRefs: []string{builderRuntimeCollectionSurfaceRef}}},
	}
	operations := []appruns.WorkspacePatchOperation{
		{Type: "write_file", Path: "lib/views/task_collection_page.dart", Content: "class TaskCollectionPage {}\n"},
		{Type: "write_file", Path: "lib/controllers/task_collection_controller.dart", Content: "class TaskCollectionController {}\n"},
		{Type: "write_file", Path: "lib/template/open_lite_copy.dart", Content: "const copy = 1;\n"},
		{Type: "write_file", Path: "lib/template/open_lite_pop.dart", Content: "const pop = 1;\n"},
		{Type: "write_file", Path: "lib/models/task.dart", Content: "class Task {}\n"},
	}
	filtered := pruneUnexpectedDependentOperationsFromSingleTargetNonRepairPatch("", taskBundle, operations, []string{"lib/views/task_collection_page.dart"})
	if len(filtered) != 2 {
		t.Fatalf("len(filtered) = %d, want 2", len(filtered))
	}
	if filtered[0].Path != "lib/views/task_collection_page.dart" {
		t.Fatalf("filtered[0].Path = %q, want target collection view path", filtered[0].Path)
	}
	if filtered[1].Path != "lib/models/task.dart" {
		t.Fatalf("filtered[1].Path = %q, want unrelated model path to remain", filtered[1].Path)
	}
}

func TestPruneUnexpectedDependentOperationsFromSingleTargetNonRepairPatchUsesLikelyCustomCollectionPathsWithoutSurfaceRefs(t *testing.T) {
	taskBundle := []appruns.TaskBundleItem{
		{TaskID: "task-create-list-controller", TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, TargetPaths: []string{"lib/controllers/task_collection_controller.dart"}},
		{TaskID: "task-bind-collection-surface", TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, TargetPaths: []string{"lib/views/task_collection_page.dart"}},
	}
	operations := []appruns.WorkspacePatchOperation{
		{Type: "write_file", Path: "lib/views/task_collection_page.dart", Content: "class TaskCollectionPage {}\n"},
		{Type: "write_file", Path: "lib/controllers/task_collection_controller.dart", Content: "class TaskCollectionController {}\n"},
		{Type: "write_file", Path: "lib/template/open_lite_copy.dart", Content: "const copy = 1;\n"},
		{Type: "write_file", Path: "lib/template/open_lite_pop.dart", Content: "const pop = 1;\n"},
		{Type: "write_file", Path: "lib/models/task.dart", Content: "class Task {}\n"},
	}
	filtered := pruneUnexpectedDependentOperationsFromSingleTargetNonRepairPatch("", taskBundle, operations, []string{"lib/views/task_collection_page.dart"})
	if len(filtered) != 2 {
		t.Fatalf("len(filtered) = %d, want 2 for likely custom collection paths without surface refs", len(filtered))
	}
	if filtered[0].Path != "lib/views/task_collection_page.dart" {
		t.Fatalf("filtered[0].Path = %q, want target likely custom collection view path", filtered[0].Path)
	}
	if filtered[1].Path != "lib/models/task.dart" {
		t.Fatalf("filtered[1].Path = %q, want unrelated model path to remain", filtered[1].Path)
	}
}

func TestPruneUnexpectedDependentOperationsFromSingleTargetNonRepairPatchPrunesDetailHelpersFromSurfaceRefsWithoutDefaultFileNames(t *testing.T) {
	taskBundle := []appruns.TaskBundleItem{
		{TaskID: "task-bind-detail-surface", TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, TargetPaths: []string{"lib/views/task_detail_page.dart"}, AllocationTransition: &appruns.TaskAllocationTransition{SurfaceRefs: []string{builderRuntimeInspectionSurfaceRef}}},
	}
	operations := []appruns.WorkspacePatchOperation{
		{Type: "write_file", Path: "lib/views/task_detail_page.dart", Content: "class TaskDetailPage {}\n"},
		{Type: "write_file", Path: "lib/template/open_lite_copy.dart", Content: "const copy = 1;\n"},
		{Type: "write_file", Path: "lib/models/task.dart", Content: "class Task {}\n"},
	}
	filtered := pruneUnexpectedDependentOperationsFromSingleTargetNonRepairPatch("", taskBundle, operations, []string{"lib/views/task_detail_page.dart"})
	if len(filtered) != 2 {
		t.Fatalf("len(filtered) = %d, want 2", len(filtered))
	}
	if filtered[0].Path != "lib/views/task_detail_page.dart" {
		t.Fatalf("filtered[0].Path = %q, want target detail view path", filtered[0].Path)
	}
	if filtered[1].Path != "lib/models/task.dart" {
		t.Fatalf("filtered[1].Path = %q, want unrelated model path to remain", filtered[1].Path)
	}
}

func TestValidateBuilderRuntimeTopologyScopedTaskOutputRejectsUnexpectedHomeControllerInMutationSurfaceWithoutDefaultFileNames(t *testing.T) {
	run := runRecord{
		WorkspacePath: t.TempDir(),
		TaskBundle: []appruns.TaskBundleItem{
			{TaskID: "task-bind-mutation-surface", TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, TargetPaths: []string{"lib/controllers/task_form_controller.dart", "lib/views/task_form_page.dart"}, AllocationTransition: &appruns.TaskAllocationTransition{SurfaceRefs: []string{builderRuntimeMutationSurfaceRef}}},
		},
	}
	err := validateBuilderRuntimeTopologyScopedTaskOutput(run, "lib/controllers/task_form_controller.dart", []byte("class TaskFormController {\n  final HomeController homeController;\n  TaskFormController(this.homeController);\n}\n"))
	if err == nil {
		t.Fatalf("validateBuilderRuntimeTopologyScopedTaskOutput() should reject HomeController dependency for custom mutation controller path")
	}
	if !strings.Contains(err.Error(), "still depends on HomeController") {
		t.Fatalf("validateBuilderRuntimeTopologyScopedTaskOutput() error = %v, want HomeController topology failure", err)
	}
}

func TestValidateBuilderRuntimeTopologyScopedTaskOutputRejectsMissingCreateEntryInCollectionSurfaceWithoutDefaultFileNames(t *testing.T) {
	run := runRecord{
		WorkspacePath: t.TempDir(),
		TaskBundle: []appruns.TaskBundleItem{
			{TaskID: "task-bind-collection-surface", TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, TargetPaths: []string{"lib/views/task_collection_page.dart"}, AllocationTransition: &appruns.TaskAllocationTransition{SurfaceRefs: []string{builderRuntimeCollectionSurfaceRef}}},
			{TaskID: "task-bind-mutation-surface", TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, TargetPaths: []string{"lib/views/task_form_page.dart"}, AllocationTransition: &appruns.TaskAllocationTransition{SurfaceRefs: []string{builderRuntimeMutationSurfaceRef}}},
		},
	}
	err := validateBuilderRuntimeTopologyScopedTaskOutput(run, "lib/views/task_collection_page.dart", []byte(strings.Join([]string{
		"import 'package:flutter/material.dart';",
		"class TaskCollectionPage extends StatelessWidget {",
		"  const TaskCollectionPage({super.key});",
		"  @override",
		"  Widget build(BuildContext context) {",
		"    return const Scaffold(body: Text('list'));",
		"  }",
		"}",
	}, "\n")))
	if err == nil {
		t.Fatalf("validateBuilderRuntimeTopologyScopedTaskOutput() should require create entry for custom collection view path")
	}
	if !strings.Contains(err.Error(), "did not expose a create entry on the collection root") {
		t.Fatalf("validateBuilderRuntimeTopologyScopedTaskOutput() error = %v, want collection create-entry failure", err)
	}
}

func TestValidateBuilderRuntimeTopologyScopedTaskOutputUsesWorkspaceFallbackForCustomCollectionPathWithoutSurfaceRefs(t *testing.T) {
	workspacePath := t.TempDir()
	writeBuilderRuntimeWorkspaceFiles(t, workspacePath, map[string]string{
		"lib/repositories/task_repository.dart":           "abstract class TaskRepository {}\n",
		"lib/controllers/task_collection_controller.dart": "import '../repositories/task_repository.dart';\nclass TaskCollectionController {\n  TaskCollectionController({required TaskRepository taskRepository});\n  Future<void> refresh() async {}\n}\n",
		"lib/controllers/task_form_controller.dart":       "class TaskFormController {}\n",
		"lib/views/task_form_page.dart":                   "class TaskFormPage {}\n",
	})
	run := runRecord{
		WorkspacePath: workspacePath,
		TaskBundle: []appruns.TaskBundleItem{
			{TaskID: "task-bind-collection-surface", TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, TargetPaths: []string{"lib/views/task_collection_page.dart", "lib/controllers/task_collection_controller.dart"}},
			{TaskID: "task-bind-mutation-surface", TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, TargetPaths: []string{"lib/views/task_form_page.dart", "lib/controllers/task_form_controller.dart"}},
		},
	}
	err := validateBuilderRuntimeTopologyScopedTaskOutput(run, "lib/views/task_collection_page.dart", []byte(strings.Join([]string{
		"import 'package:flutter/material.dart';",
		"class TaskCollectionPage extends StatelessWidget {",
		"  const TaskCollectionPage({super.key});",
		"  @override",
		"  Widget build(BuildContext context) {",
		"    return const Scaffold(body: Text('list'));",
		"  }",
		"}",
	}, "\n")))
	if err == nil {
		t.Fatalf("validateBuilderRuntimeTopologyScopedTaskOutput() should require create entry for custom collection view path even without surface refs when workspace topology is collection+mutation only")
	}
	if !strings.Contains(err.Error(), "did not expose a create entry on the collection root") {
		t.Fatalf("validateBuilderRuntimeTopologyScopedTaskOutput() error = %v, want workspace-backed collection create-entry failure", err)
	}
}

func TestValidateBuilderRuntimeTopologyScopedTaskOutputRejectsRecordDetailReferenceInCollectionSurfaceWithoutDefaultFileNames(t *testing.T) {
	run := runRecord{
		WorkspacePath: t.TempDir(),
		TaskBundle: []appruns.TaskBundleItem{
			{TaskID: "task-bind-collection-surface", TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, TargetPaths: []string{"lib/views/task_collection_page.dart"}, AllocationTransition: &appruns.TaskAllocationTransition{SurfaceRefs: []string{builderRuntimeCollectionSurfaceRef}}},
		},
	}
	err := validateBuilderRuntimeTopologyScopedTaskOutput(run, "lib/views/task_collection_page.dart", []byte(strings.Join([]string{
		"import 'package:flutter/material.dart';",
		"class TaskCollectionPage extends StatelessWidget {",
		"  const TaskCollectionPage({super.key});",
		"  @override",
		"  Widget build(BuildContext context) {",
		"    return Scaffold(",
		"      body: TextButton(",
		"        onPressed: () => Navigator.push(context, MaterialPageRoute(builder: (_) => const RecordDetailPage())),",
		"        child: const Text('open'),",
		"      ),",
		"    );",
		"  }",
		"}",
	}, "\n")))
	if err == nil {
		t.Fatalf("validateBuilderRuntimeTopologyScopedTaskOutput() should reject RecordDetailPage reference for custom collection view path when detail surface is absent")
	}
	if !strings.Contains(err.Error(), "RecordDetailPage") {
		t.Fatalf("validateBuilderRuntimeTopologyScopedTaskOutput() error = %v, want RecordDetailPage topology failure", err)
	}
}

func TestValidateBuilderRuntimeTopologyScopedTaskOutputRejectsCustomDetailReferenceInCollectionSurfaceWithoutDetailSurface(t *testing.T) {
	run := runRecord{
		WorkspacePath: t.TempDir(),
		TaskBundle: []appruns.TaskBundleItem{
			{TaskID: "task-bind-collection-surface", TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, TargetPaths: []string{"lib/views/task_collection_page.dart"}, AllocationTransition: &appruns.TaskAllocationTransition{SurfaceRefs: []string{builderRuntimeCollectionSurfaceRef}}},
		},
	}
	err := validateBuilderRuntimeTopologyScopedTaskOutput(run, "lib/views/task_collection_page.dart", []byte(strings.Join([]string{
		"import 'package:flutter/material.dart';",
		"class TaskCollectionPage extends StatelessWidget {",
		"  const TaskCollectionPage({super.key});",
		"  @override",
		"  Widget build(BuildContext context) {",
		"    return Scaffold(",
		"      body: TextButton(",
		"        onPressed: () => Navigator.push(context, MaterialPageRoute(builder: (_) => const TaskDetailPage())),",
		"        child: const Text('open'),",
		"      ),",
		"    );",
		"  }",
		"}",
	}, "\n")))
	if err == nil {
		t.Fatalf("validateBuilderRuntimeTopologyScopedTaskOutput() should reject custom detail page reference for collection view path when detail surface is absent")
	}
	if !strings.Contains(err.Error(), "TaskDetailPage") {
		t.Fatalf("validateBuilderRuntimeTopologyScopedTaskOutput() error = %v, want custom detail topology failure", err)
	}
}

func TestValidateBuilderRuntimeTopologyScopedTaskOutputReportsCustomDetailCallbackNameWhenPlaceholderRemains(t *testing.T) {
	workspacePath := t.TempDir()
	writeBuilderRuntimeWorkspaceFiles(t, workspacePath, map[string]string{
		"lib/repositories/task_repository.dart": "abstract class TaskRepository {}\n",
		"lib/controllers/task_collection_controller.dart": strings.Join([]string{
			"import '../repositories/task_repository.dart';",
			"class TaskCollectionController {",
			"  TaskCollectionController({required TaskRepository taskRepository});",
			"}",
		}, "\n") + "\n",
		"lib/views/task_collection_page.dart": strings.Join([]string{
			"import '../models/task.dart';",
			"class TaskCollectionPage {",
			"  const TaskCollectionPage({required this.controller, required this.onOpenTaskDetail});",
			"  final TaskCollectionController controller;",
			"  final Future<void> Function(Task task) onOpenTaskDetail;",
			"}",
		}, "\n") + "\n",
		"lib/models/task.dart": strings.Join([]string{
			"class Task {",
			"  const Task({required this.taskId});",
			"  final String taskId;",
			"}",
		}, "\n") + "\n",
	})
	run := runRecord{
		WorkspacePath: workspacePath,
		TaskBundle: []appruns.TaskBundleItem{
			{TaskID: "task-bind-collection-surface", TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, TargetPaths: []string{"lib/views/task_collection_page.dart"}, AllocationTransition: &appruns.TaskAllocationTransition{SurfaceRefs: []string{builderRuntimeCollectionSurfaceRef}}},
		},
	}
	err := validateBuilderRuntimeTopologyScopedTaskOutput(run, "lib/main.dart", []byte(strings.Join([]string{
		"import 'package:flutter/material.dart';",
		"import 'controllers/task_collection_controller.dart';",
		"import 'repositories/task_repository.dart';",
		"import 'views/task_collection_page.dart';",
		"class TaskLiteApp extends StatelessWidget {",
		"  const TaskLiteApp({super.key, required this.repository});",
		"  final TaskRepository repository;",
		"  @override",
		"  Widget build(BuildContext context) {",
		"    return MaterialApp(",
		"      home: TaskCollectionPage(",
		"        controller: TaskCollectionController(taskRepository: repository),",
		"        onOpenTaskDetail: (record) async {},",
		"      ),",
		"    );",
		"  }",
		"}",
	}, "\n")))
	if err == nil {
		t.Fatalf("validateBuilderRuntimeTopologyScopedTaskOutput() should reject custom detail callback placeholder when detail surface is absent")
	}
	if !strings.Contains(err.Error(), "onOpenTaskDetail") {
		t.Fatalf("validateBuilderRuntimeTopologyScopedTaskOutput() error = %v, want custom detail callback name in topology failure", err)
	}
}

func TestValidateBuilderRuntimeTopologyScopedTaskOutputAcceptsCreateEntryInCollectionSurfaceWithTaskDomainCallback(t *testing.T) {
	run := runRecord{
		WorkspacePath: t.TempDir(),
		TaskBundle: []appruns.TaskBundleItem{
			{TaskID: "task-bind-collection-surface", TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, TargetPaths: []string{"lib/views/task_collection_page.dart"}, AllocationTransition: &appruns.TaskAllocationTransition{SurfaceRefs: []string{builderRuntimeCollectionSurfaceRef}}},
			{TaskID: "task-bind-mutation-surface", TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, TargetPaths: []string{"lib/views/task_form_page.dart"}, AllocationTransition: &appruns.TaskAllocationTransition{SurfaceRefs: []string{builderRuntimeMutationSurfaceRef}}},
		},
	}
	err := validateBuilderRuntimeTopologyScopedTaskOutput(run, "lib/views/task_collection_page.dart", []byte(strings.Join([]string{
		"import 'package:flutter/material.dart';",
		"class TaskCollectionPage extends StatelessWidget {",
		"  const TaskCollectionPage({super.key, required this.onCreateTask});",
		"  final Future<void> Function() onCreateTask;",
		"  @override",
		"  Widget build(BuildContext context) {",
		"    return Scaffold(",
		"      floatingActionButton: FloatingActionButton(onPressed: onCreateTask),",
		"      body: const Text('list'),",
		"    );",
		"  }",
		"}",
	}, "\n")))
	if err != nil {
		t.Fatalf("validateBuilderRuntimeTopologyScopedTaskOutput() error = %v, want task-domain create entry to satisfy collection-root requirement", err)
	}
}

func TestBuildBuilderRuntimePromptMarksMainOnlyTaskDependentsReadOnly(t *testing.T) {
	run := runRecord{
		GoalSummary:   "build a list-plus-drawer todo app without home detail filter or delete",
		WorkspacePath: t.TempDir(),
		TaskBundle: []appruns.TaskBundleItem{
			{TaskID: "task-create-repository", TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, TargetPaths: []string{"lib/repositories/record_repository.dart"}},
			{TaskID: "task-create-form-controller", TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, TargetPaths: []string{"lib/controllers/record_form_controller.dart"}, AllocationTransition: &appruns.TaskAllocationTransition{SemanticIntentRefs: []string{"ac-form"}}},
			{TaskID: "task-create-list-controller", TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, TargetPaths: []string{"lib/controllers/record_list_controller.dart"}, AllocationTransition: &appruns.TaskAllocationTransition{SemanticIntentRefs: []string{"ac-list"}}},
			{TaskID: "task-bind-collection-surface", TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, TargetPaths: []string{"lib/views/record_list_page.dart"}, AllocationTransition: &appruns.TaskAllocationTransition{SemanticIntentRefs: []string{"ac-list"}, SurfaceRefs: []string{"surface-collection"}}},
			{TaskID: "task-bind-mutation-surface", TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, TargetPaths: []string{"lib/views/record_form_page.dart"}, AllocationTransition: &appruns.TaskAllocationTransition{SemanticIntentRefs: []string{"ac-form"}, SurfaceRefs: []string{"surface-mutation"}}},
			{TaskID: "task-bind-app-entry", TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, TargetPaths: []string{"lib/main.dart"}, Dependencies: []string{"task-create-repository", "task-create-form-controller", "task-create-list-controller", "task-bind-collection-surface", "task-bind-mutation-surface"}, AllocationTransition: &appruns.TaskAllocationTransition{SemanticIntentRefs: []string{"ac-navigation", "ac-form"}, SurfaceRefs: []string{"surface-collection", "surface-mutation"}, OwnedPaths: []string{"lib/main.dart"}}},
		},
	}
	roundInput := appruns.RoundInput{
		TaskBundle:   run.TaskBundle,
		AllowedPaths: []string{"lib/**", "test/**"},
	}
	route := appruns.BuilderRuntimeTaskRoute{
		TaskID:      "task-bind-app-entry",
		TaskType:    appruns.BuilderRuntimeTaskTypeDualFileWiring,
		RouteSource: "default_model",
	}

	prompt, err := buildBuilderRuntimePrompt(BuilderRuntimePatchRequest{Run: run, RoundInput: roundInput, Route: route})
	if err != nil {
		t.Fatalf("buildBuilderRuntimePrompt() error = %v", err)
	}
	if !strings.Contains(prompt, "Emit operations only for lib/main.dart in this task") {
		t.Fatalf("prompt missing main-only read-only dependency guidance: %q", prompt)
	}
}

func TestBuildBuilderRuntimePromptIncludesCoordinatedFocusTasks(t *testing.T) {
	run := runRecord{
		GoalSummary:   "rewrite generic template into weight tracker app",
		WorkspacePath: t.TempDir(),
		TaskBundle: []appruns.TaskBundleItem{
			{
				TaskID:      "task-generic-domain-models",
				Category:    appruns.TaskCategoryDomain,
				TaskType:    appruns.BuilderRuntimeTaskTypeDualFileWiring,
				RouteHint:   appruns.TaskRouteHintStrongModel,
				RiskLevel:   appruns.TaskRiskLevelHigh,
				TargetPaths: []string{"lib/models/record.dart", "lib/models/dashboard_summary.dart"},
			},
			{
				TaskID:      "task-generic-summary-remap",
				Category:    appruns.TaskCategorySummary,
				TaskType:    appruns.BuilderRuntimeTaskTypeDualFileWiring,
				RouteHint:   appruns.TaskRouteHintStrongModel,
				RiskLevel:   appruns.TaskRiskLevelHigh,
				TargetPaths: []string{"lib/controllers/home_controller.dart", "lib/views/home_page.dart", "lib/models/dashboard_summary.dart"},
			},
			{
				TaskID:      "task-generic-validation-closure",
				Category:    appruns.TaskCategoryValidation,
				TaskType:    appruns.BuilderRuntimeTaskTypeClosureRepair,
				TargetPaths: []string{"lib/**", "test/**"},
			},
		},
	}
	roundInput := appruns.RoundInput{
		TaskBundle: run.TaskBundle,
		AllowedPaths: []string{
			"lib/**",
		},
	}
	route := appruns.BuilderRuntimeTaskRoute{
		TaskID:      "task-generic-validation-closure",
		TaskType:    appruns.BuilderRuntimeTaskTypeClosureRepair,
		RouteSource: "task_route",
	}

	prompt, err := buildBuilderRuntimePrompt(BuilderRuntimePatchRequest{Run: run, RoundInput: roundInput, Route: route})
	if err != nil {
		t.Fatalf("buildBuilderRuntimePrompt() error = %v", err)
	}
	if !strings.Contains(prompt, "Current focus tasks JSON: [{\"task_id\":\"task-generic-domain-models\"") {
		t.Fatalf("prompt missing focus task bundle: %q", prompt)
	}
	if !strings.Contains(prompt, "task-generic-summary-remap") {
		t.Fatalf("prompt missing summary remap task in focus bundle: %q", prompt)
	}
	if !strings.Contains(prompt, "Treat current focus tasks as a coordinated implementation slice") {
		t.Fatalf("prompt missing coordinated focus-task guidance: %q", prompt)
	}
	if !strings.Contains(prompt, "If you change shared model fields, constructor parameters, enum values, widget constructor signatures, controller public APIs, or summary metrics") {
		t.Fatalf("prompt missing shared-model dependency sync guidance: %q", prompt)
	}
	if !strings.Contains(prompt, "do not leave stale references to title, category, status, updatedAt, totalCount, inboxCount, inProgressCount, or doneCount") {
		t.Fatalf("prompt missing explicit open-lite stale-field cleanup guidance: %q", prompt)
	}
	if !strings.Contains(prompt, "Current allocation route_hint requires domain-specific implementation") {
		t.Fatalf("prompt missing explicit strong route-hint guidance: %q", prompt)
	}
	if !strings.Contains(prompt, "Current allocation task JSON: {\"task_id\":\"task-generic-domain-models\"") {
		t.Fatalf("prompt missing current allocation task payload: %q", prompt)
	}
	if strings.Contains(prompt, "Task bundle JSON:") {
		t.Fatalf("prompt should not contain full task bundle dump: %q", prompt)
	}
}

func TestDominantBuilderRuntimeTaskPrefersStrongSemanticTaskOverClosure(t *testing.T) {
	dominant := dominantBuilderRuntimeTask([]appruns.TaskBundleItem{
		{
			TaskID:    "task-generic-validation-closure",
			Category:  appruns.TaskCategoryValidation,
			TaskType:  appruns.BuilderRuntimeTaskTypeClosureRepair,
			RouteHint: appruns.TaskRouteHintDefaultModel,
		},
		{
			TaskID:    "task-generic-domain-models",
			Category:  appruns.TaskCategoryDomain,
			TaskType:  appruns.BuilderRuntimeTaskTypeDualFileWiring,
			RouteHint: appruns.TaskRouteHintStrongModel,
			RiskLevel: appruns.TaskRiskLevelHigh,
		},
	})
	if dominant.TaskID != "task-generic-domain-models" {
		t.Fatalf("dominant task = %q, want task-generic-domain-models", dominant.TaskID)
	}
}

func TestExecuteBuilderRuntimeEditAppliesGenericBrandingPreProjection(t *testing.T) {
	workspace := t.TempDir()
	copyPath := filepath.Join(workspace, "lib", "template", "open_lite_copy.dart")
	if err := os.MkdirAll(filepath.Dir(copyPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(copy) error = %v", err)
	}
	if err := os.WriteFile(copyPath, []byte("String get appTitle => 'Open Lite Seed';\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(copy) error = %v", err)
	}
	modelPath := filepath.Join(workspace, "lib", "models", "record.dart")
	if err := os.MkdirAll(filepath.Dir(modelPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(model) error = %v", err)
	}
	if err := os.WriteFile(modelPath, []byte("class Record {}\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(model) error = %v", err)
	}
	stringsPath := filepath.Join(workspace, "android", "app", "src", "main", "res", "values", "strings.xml")
	if err := os.MkdirAll(filepath.Dir(stringsPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(strings) error = %v", err)
	}
	if err := os.WriteFile(stringsPath, []byte("<resources><string name=\"app_name\">Open Lite Seed</string></resources>\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(strings) error = %v", err)
	}
	humanNotes, err := json.Marshal([]map[string]string{{
		"note_id": "note-weight-domain",
		"summary": "Builder 后续必须把中性 record 骨架收口为体重记录 app，而不是停在 open-lite 默认文案。",
	}})
	if err != nil {
		t.Fatalf("Marshal(humanNotes) error = %v", err)
	}
	run := runRecord{
		RunID:         "run-1",
		GoalSummary:   "将体重记录需求整理成基于 flutter-open-lite 的多页面 Android MVP 输入包。",
		HumanNotes:    humanNotes,
		WorkspacePath: workspace,
		AllowedPaths: []string{
			"lib/**",
			"android/app/src/main/res/values/strings.xml",
		},
		TaskBundle: []appruns.TaskBundleItem{{
			TaskID:      "task-generic-domain-copy",
			TaskType:    appruns.BuilderRuntimeTaskTypeDualFileWiring,
			TargetPaths: []string{"lib/template/open_lite_copy.dart", "android/app/src/main/res/values/strings.xml", "lib/models/record.dart"},
		}},
		BuilderRuntime: &appruns.BuilderRuntimePlan{
			Enabled: true,
		},
		LogPath: filepath.Join(t.TempDir(), "builder-runtime.log"),
	}
	roundInput := appruns.RoundInput{
		RoundID:      "round-1",
		Attempt:      1,
		TaskBundle:   run.TaskBundle,
		AllowedPaths: run.AllowedPaths,
		BuilderRuntime: &appruns.BuilderRuntimePlan{
			Enabled: true,
		},
	}
	runner := &Runner{
		PatchGenerator: &stubBuilderRuntimePatchGenerator{response: BuilderRuntimePatchResponse{Content: `{"patch_id":"round-1-patch","operations":[{"type":"write_file","path":"lib/models/record.dart","content":"class Record { final double weight; const Record(this.weight); }\n"}]}`}},
	}
	step := ExecutionStep{StepID: "thin-prepare", Stage: appruns.StageThinPrepare}
	backend := noopRunnerBackend{}
	route := appruns.BuilderRuntimeTaskRoute{TaskID: "task-generic-domain-copy", TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, RouteSource: "task_route", Model: appruns.BuilderRuntimeModelRef{Primary: "qwen2.5-coder-14b-local"}}
	run.BuilderRuntime.TaskRoutes = []appruns.BuilderRuntimeTaskRoute{route}
	roundInput.BuilderRuntime = run.BuilderRuntime

	result, err := runner.executeBuilderRuntimeEdit(context.Background(), backend, run.RunID, step, run, roundInput)
	if err != nil {
		t.Fatalf("executeBuilderRuntimeEdit() error = %v", err)
	}
	copyContent, err := os.ReadFile(copyPath)
	if err != nil {
		t.Fatalf("ReadFile(copy) error = %v", err)
	}
	if strings.Contains(string(copyContent), "Open Lite Seed") || !strings.Contains(string(copyContent), "体重记录 App") {
		t.Fatalf("copy content = %q, want projected generic branding", string(copyContent))
	}
	androidContent, err := os.ReadFile(stringsPath)
	if err != nil {
		t.Fatalf("ReadFile(strings) error = %v", err)
	}
	if strings.Contains(string(androidContent), "Open Lite Seed") || !strings.Contains(string(androidContent), "体重记录 App") {
		t.Fatalf("android content = %q, want projected generic branding", string(androidContent))
	}
	if result == nil || result.Patch == nil || len(result.Patch.Operations) < 2 {
		t.Fatalf("result patch = %+v, want merged deterministic + model operations", result)
	}
}

func TestExecuteBuilderRuntimeEditRejectsModelExportRenameDuringDualFileWiring(t *testing.T) {
	jobRoot := t.TempDir()
	workspace := filepath.Join(jobRoot, "workspace")
	modelPath := filepath.Join(workspace, "lib", "models", "record.dart")
	if err := os.MkdirAll(filepath.Dir(modelPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(model) error = %v", err)
	}
	if err := os.WriteFile(modelPath, []byte("class AppRecord {\n  const AppRecord();\n}\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(model) error = %v", err)
	}
	copyPath := filepath.Join(workspace, "lib", "template", "open_lite_copy.dart")
	if err := os.MkdirAll(filepath.Dir(copyPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(copy) error = %v", err)
	}
	if err := os.WriteFile(copyPath, []byte("String get appTitle => 'Open Lite Seed';\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(copy) error = %v", err)
	}
	stringsPath := filepath.Join(workspace, "android", "app", "src", "main", "res", "values", "strings.xml")
	if err := os.MkdirAll(filepath.Dir(stringsPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(strings) error = %v", err)
	}
	if err := os.WriteFile(stringsPath, []byte("<resources><string name=\"app_name\">Open Lite Seed</string></resources>\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(strings) error = %v", err)
	}
	run := runRecord{
		RunID:         "run-rename-check",
		GoalSummary:   "将体重记录需求整理成基于 flutter-open-lite 的多页面 Android MVP 输入包。",
		WorkspacePath: workspace,
		AllowedPaths: []string{
			"lib/**",
			"android/app/src/main/res/values/strings.xml",
		},
		TaskBundle: []appruns.TaskBundleItem{{
			TaskID:      "task-generic-domain-models",
			TaskType:    appruns.BuilderRuntimeTaskTypeDualFileWiring,
			TargetPaths: []string{"lib/models/record.dart", "lib/models/dashboard_summary.dart"},
		}},
		BuilderRuntime: &appruns.BuilderRuntimePlan{Enabled: true},
		LogPath:        filepath.Join(t.TempDir(), "builder-runtime.log"),
	}
	roundInput := appruns.RoundInput{
		RoundID:      "round-1",
		Attempt:      1,
		TaskBundle:   run.TaskBundle,
		AllowedPaths: run.AllowedPaths,
		BuilderRuntime: &appruns.BuilderRuntimePlan{
			Enabled: true,
		},
	}
	route := appruns.BuilderRuntimeTaskRoute{TaskID: "task-generic-domain-models", TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, RouteSource: "task_route", Model: appruns.BuilderRuntimeModelRef{Primary: "qwen2.5-coder-32b-local"}}
	run.BuilderRuntime.TaskRoutes = []appruns.BuilderRuntimeTaskRoute{route}
	roundInput.BuilderRuntime = run.BuilderRuntime
	runner := &Runner{
		PatchGenerator: &stubBuilderRuntimePatchGenerator{response: BuilderRuntimePatchResponse{ModelAlias: "qwen2.5-coder-32b-local", Content: `{"patch_id":"round-1-patch","operations":[{"type":"write_file","path":"lib/models/record.dart","content":"class WeightRecord {\n  const WeightRecord();\n}\n"}]}`}},
	}
	step := ExecutionStep{StepID: "thin-prepare", Stage: appruns.StageThinPrepare}
	_, err := runner.executeBuilderRuntimeEdit(context.Background(), noopRunnerBackend{}, run.RunID, step, run, roundInput)
	if err == nil {
		t.Fatal("executeBuilderRuntimeEdit() error = nil, want semantic conflict")
	}
	if !strings.Contains(err.Error(), "schema-conflicting symbols") && !strings.Contains(err.Error(), "exported Dart model types") {
		t.Fatalf("executeBuilderRuntimeEdit() error = %v, want model export rename conflict", err)
	}
}

func TestExecuteBuilderRuntimeEditFallsBackAfterUpgradeModelSemanticConflict(t *testing.T) {
	jobRoot := t.TempDir()
	workspace := filepath.Join(jobRoot, "workspace")
	recordPath := filepath.Join(workspace, "lib", "models", "record.dart")
	summaryPath := filepath.Join(workspace, "lib", "models", "dashboard_summary.dart")
	if err := os.MkdirAll(filepath.Dir(recordPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(model) error = %v", err)
	}
	if err := os.WriteFile(recordPath, []byte("class AppRecord {\n  const AppRecord();\n}\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(record) error = %v", err)
	}
	if err := os.WriteFile(summaryPath, []byte("class DashboardSummary {\n  const DashboardSummary();\n}\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(summary) error = %v", err)
	}
	run := runRecord{
		RunID:         "run-upgrade-fallback",
		GoalSummary:   "将体重记录需求整理成基于 flutter-open-lite 的多页面 Android MVP 输入包。",
		WorkspacePath: workspace,
		AllowedPaths:  []string{"lib/**"},
		TaskBundle: []appruns.TaskBundleItem{{
			TaskID:      "task-generic-domain-models",
			TaskType:    appruns.BuilderRuntimeTaskTypeDualFileWiring,
			TargetPaths: []string{"lib/models/record.dart", "lib/models/dashboard_summary.dart"},
		}},
		BuilderRuntime: &appruns.BuilderRuntimePlan{
			Enabled:      true,
			UpgradeModel: appruns.BuilderRuntimeModelRef{Primary: "qwen2.5-coder-32b-local", Fallbacks: []string{"gpt-4o-mini"}},
		},
		LogPath: filepath.Join(t.TempDir(), "builder-runtime.log"),
	}
	roundInput := appruns.RoundInput{
		RoundID:      "round-1",
		Attempt:      1,
		TaskBundle:   run.TaskBundle,
		AllowedPaths: run.AllowedPaths,
		BuilderRuntime: &appruns.BuilderRuntimePlan{
			Enabled:      true,
			UpgradeModel: appruns.BuilderRuntimeModelRef{Primary: "qwen2.5-coder-32b-local", Fallbacks: []string{"gpt-4o-mini"}},
		},
	}
	route := appruns.BuilderRuntimeTaskRoute{TaskID: "task-generic-domain-models", TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, RouteSource: "upgrade_threshold", Model: appruns.BuilderRuntimeModelRef{Primary: "qwen2.5-coder-32b-local", Fallbacks: []string{"gpt-4o-mini"}}}
	run.BuilderRuntime.TaskRoutes = []appruns.BuilderRuntimeTaskRoute{route}
	roundInput.BuilderRuntime = run.BuilderRuntime
	runner := &Runner{
		PatchGenerator: &stubBuilderRuntimePatchGenerator{responses: []BuilderRuntimePatchResponse{
			{ModelAlias: "qwen2.5-coder-32b-local", Content: `{"patch_id":"round-1-patch","operations":[{"type":"write_file","path":"lib/models/record.dart","content":"class WeightRecord {\n  const WeightRecord();\n}\n"},{"type":"write_file","path":"lib/models/dashboard_summary.dart","content":"class WeightSummary {\n  const WeightSummary();\n}\n"}]}`},
			{ModelAlias: "gpt-4o-mini", Content: `{"patch_id":"round-2-patch","operations":[{"type":"write_file","path":"lib/models/record.dart","content":"class AppRecord {\n  const AppRecord();\n}\n"},{"type":"write_file","path":"lib/models/dashboard_summary.dart","content":"class DashboardSummary {\n  const DashboardSummary();\n}\n"}]}`},
		}},
	}
	step := ExecutionStep{StepID: "thin-prepare", Stage: appruns.StageThinPrepare}
	result, err := runner.executeBuilderRuntimeEdit(context.Background(), noopRunnerBackend{}, run.RunID, step, run, roundInput)
	if err != nil {
		t.Fatalf("executeBuilderRuntimeEdit() error = %v, want fallback success", err)
	}
	if result == nil || result.Stats == nil {
		t.Fatalf("result = %+v, want stats", result)
	}
	if result.Stats.Attempts != 2 {
		t.Fatalf("stats.Attempts = %d, want 2", result.Stats.Attempts)
	}
	if result.Stats.SelectedModel != "gpt-4o-mini" {
		t.Fatalf("stats.SelectedModel = %q, want gpt-4o-mini", result.Stats.SelectedModel)
	}
	if len(result.Stats.ModelSequence) != 2 || result.Stats.ModelSequence[1] != "gpt-4o-mini" {
		t.Fatalf("stats.ModelSequence = %#v, want fallback model recorded", result.Stats.ModelSequence)
	}
	if generator, ok := runner.PatchGenerator.(*stubBuilderRuntimePatchGenerator); !ok {
		t.Fatalf("PatchGenerator = %T, want *stubBuilderRuntimePatchGenerator", runner.PatchGenerator)
	} else {
		if len(generator.requests) != 2 {
			t.Fatalf("len(generator.requests) = %d, want 2", len(generator.requests))
		}
		if !strings.Contains(generator.requests[1].FailureContext, "patch renamed exported Dart model types") {
			t.Fatalf("fallback failure context = %q, want export rename conflict", generator.requests[1].FailureContext)
		}
		if !strings.Contains(generator.requests[1].PreviousBody, "WeightRecord") {
			t.Fatalf("fallback previous body = %q, want prior invalid patch body", generator.requests[1].PreviousBody)
		}
	}
	content, err := os.ReadFile(recordPath)
	if err != nil {
		t.Fatalf("ReadFile(record) error = %v", err)
	}
	if !strings.Contains(string(content), "class AppRecord") {
		t.Fatalf("record content = %q, want AppRecord preserved after fallback", string(content))
	}
}

func TestExecuteBuilderRuntimeEditRetriesTransientModelRequestFailure(t *testing.T) {
	jobRoot := t.TempDir()
	workspace := filepath.Join(jobRoot, "workspace")
	recordPath := filepath.Join(workspace, "lib", "models", "record.dart")
	if err := os.MkdirAll(filepath.Dir(recordPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(record) error = %v", err)
	}
	if err := os.WriteFile(recordPath, []byte("class WeightRecord {\n  const WeightRecord();\n}\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(record) error = %v", err)
	}
	run := runRecord{
		RunID:         "run-transient-model-request-retry",
		GoalSummary:   "创建体重记录模型并在限流后重试。",
		WorkspacePath: workspace,
		AllowedPaths:  []string{"lib/**"},
		TaskBundle: []appruns.TaskBundleItem{{
			TaskID:      "task-create-record-model",
			TaskType:    appruns.BuilderRuntimeTaskTypeDualFileWiring,
			TargetPaths: []string{"lib/models/record.dart"},
		}},
		BuilderRuntime: &appruns.BuilderRuntimePlan{
			Enabled: true,
			TaskRoutes: []appruns.BuilderRuntimeTaskRoute{{
				TaskID:      "task-create-record-model",
				TaskType:    appruns.BuilderRuntimeTaskTypeDualFileWiring,
				RouteSource: "task_route",
				Model:       appruns.BuilderRuntimeModelRef{Primary: "qwen3-coder-480b"},
			}},
		},
		LogPath: filepath.Join(t.TempDir(), "builder-runtime-transient-retry.log"),
	}
	roundInput := appruns.RoundInput{
		RoundID:        "round-transient-model-request-retry",
		Attempt:        1,
		TaskBundle:     run.TaskBundle,
		AllowedPaths:   run.AllowedPaths,
		BuilderRuntime: run.BuilderRuntime,
	}
	previousDelay := builderRuntimeTransientModelRequestRetryDelay
	builderRuntimeTransientModelRequestRetryDelay = 0
	defer func() {
		builderRuntimeTransientModelRequestRetryDelay = previousDelay
	}()
	runner := &Runner{PatchGenerator: &sequenceBuilderRuntimePatchGenerator{results: []builderRuntimePatchAttemptResult{
		{err: errors.New("builder runtime model request failed: qwen3-coder-480b: API request failed: Status: 429 Body: {\"status\":429,\"title\":\"Too Many Requests\"}")},
		{response: BuilderRuntimePatchResponse{ModelAlias: "qwen3-coder-480b", Content: `{"patch_id":"retry-record-model","operations":[{"type":"write_file","path":"lib/models/record.dart","content":"class WeightRecord {\n  const WeightRecord();\n}\n"}]}`}},
	}}}
	step := ExecutionStep{StepID: "thin-prepare", Stage: appruns.StageThinPrepare}

	result, err := runner.executeBuilderRuntimeEdit(context.Background(), noopRunnerBackend{}, run.RunID, step, run, roundInput)
	if err != nil {
		t.Fatalf("executeBuilderRuntimeEdit() error = %v, want transient retry success", err)
	}
	if result == nil || result.Stats == nil {
		t.Fatalf("result = %+v, want stats", result)
	}
	if result.Stats.Attempts != 2 {
		t.Fatalf("stats.Attempts = %d, want 2", result.Stats.Attempts)
	}
	if result.Stats.SelectedModel != "qwen3-coder-480b" {
		t.Fatalf("stats.SelectedModel = %q, want qwen3-coder-480b", result.Stats.SelectedModel)
	}
	if generator, ok := runner.PatchGenerator.(*sequenceBuilderRuntimePatchGenerator); !ok {
		t.Fatalf("PatchGenerator = %T, want *sequenceBuilderRuntimePatchGenerator", runner.PatchGenerator)
	} else {
		if len(generator.requests) != 2 {
			t.Fatalf("len(generator.requests) = %d, want 2", len(generator.requests))
		}
		for index, request := range generator.requests {
			if got := request.ModelAliases; len(got) != 1 || got[0] != "qwen3-coder-480b" {
				t.Fatalf("request %d ModelAliases = %#v, want [qwen3-coder-480b]", index, got)
			}
		}
	}
	content, err := os.ReadFile(recordPath)
	if err != nil {
		t.Fatalf("ReadFile(record) error = %v", err)
	}
	if !strings.Contains(string(content), "class WeightRecord") {
		t.Fatalf("record content = %q, want retried patch content", string(content))
	}
}

func TestExecuteBuilderRuntimeEditRetriesTransientTaskOutputValidationRepairRequest(t *testing.T) {
	workspace := filepath.Join(t.TempDir(), "workspace")
	mainPath := filepath.Join(workspace, "lib", "main.dart")
	if err := os.MkdirAll(filepath.Dir(mainPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(mainPath) error = %v", err)
	}
	baseline := "import 'package:flutter/material.dart';\n\nvoid main() {\n  runApp(const Placeholder());\n}\n"
	if err := os.WriteFile(mainPath, []byte(baseline), 0o600); err != nil {
		t.Fatalf("WriteFile(mainPath) error = %v", err)
	}
	run := runRecord{
		RunID:         "run-task-output-validation-transient-retry",
		GoalSummary:   "修复 app entry 的 counter demo shell，并在 validation repair request 遇到瞬时 503 后重试。",
		WorkspacePath: workspace,
		AllowedPaths:  []string{"lib/**"},
		TaskBundle: []appruns.TaskBundleItem{{
			TaskID:      "task-bind-app-entry",
			TaskType:    appruns.BuilderRuntimeTaskTypeSingleFileEdit,
			TargetPaths: []string{"lib/main.dart"},
		}},
		BuilderRuntime: &appruns.BuilderRuntimePlan{
			Enabled: true,
			TaskRoutes: []appruns.BuilderRuntimeTaskRoute{{
				TaskID:      "task-bind-app-entry",
				TaskType:    appruns.BuilderRuntimeTaskTypeSingleFileEdit,
				RouteSource: "task_route",
				Model:       appruns.BuilderRuntimeModelRef{Primary: "qwen3-coder-480b"},
			}},
		},
		LogPath: filepath.Join(t.TempDir(), "builder-runtime-task-output-validation-transient-retry.log"),
	}
	roundInput := appruns.RoundInput{
		RoundID:        "round-task-output-validation-transient-retry",
		Attempt:        1,
		TaskBundle:     run.TaskBundle,
		AllowedPaths:   run.AllowedPaths,
		BuilderRuntime: run.BuilderRuntime,
	}
	previousDelay := builderRuntimeTransientModelRequestRetryDelay
	builderRuntimeTransientModelRequestRetryDelay = 0
	defer func() {
		builderRuntimeTransientModelRequestRetryDelay = previousDelay
	}()
	runner := &Runner{PatchGenerator: &sequenceBuilderRuntimePatchGenerator{results: []builderRuntimePatchAttemptResult{
		{response: BuilderRuntimePatchResponse{ModelAlias: "qwen3-coder-480b", Content: `{"patch_id":"round-counter-demo-invalid-main","operations":[{"type":"write_file","path":"lib/main.dart","content":"import 'package:flutter/material.dart';\n\nvoid main() {\n  runApp(const MaterialApp(home: MyHomePage()));\n}\n\nclass MyHomePage extends StatelessWidget {\n  const MyHomePage({super.key});\n\n  @override\n  Widget build(BuildContext context) => const Placeholder();\n}\n"}]}`}},
		{err: errors.New("builder runtime model request failed: qwen3-coder-480b: API request failed: Status: 503 Body: {\"status\":503,\"title\":\"Service Unavailable\"}")},
		{response: BuilderRuntimePatchResponse{ModelAlias: "qwen3-coder-480b", Content: `{"patch_id":"round-counter-demo-repair-main","operations":[{"type":"write_file","path":"lib/main.dart","content":"import 'package:flutter/material.dart';\n\nvoid main() {\n  runApp(const MaterialApp(home: Placeholder()));\n}\n"}]}`}},
	}}}
	step := ExecutionStep{StepID: "thin-prepare", Stage: appruns.StageThinPrepare}

	result, err := runner.executeBuilderRuntimeEdit(context.Background(), noopRunnerBackend{}, run.RunID, step, run, roundInput)
	if err != nil {
		t.Fatalf("executeBuilderRuntimeEdit() error = %v, want validation repair transient retry success", err)
	}
	if result == nil || result.Stats == nil {
		t.Fatalf("result = %+v, want stats", result)
	}
	if result.Stats.Attempts != 3 {
		t.Fatalf("stats.Attempts = %d, want 3", result.Stats.Attempts)
	}
	generator, ok := runner.PatchGenerator.(*sequenceBuilderRuntimePatchGenerator)
	if !ok {
		t.Fatalf("PatchGenerator = %T, want *sequenceBuilderRuntimePatchGenerator", runner.PatchGenerator)
	}
	if len(generator.requests) != 3 {
		t.Fatalf("len(generator.requests) = %d, want 3", len(generator.requests))
	}
	for index := 1; index < 3; index++ {
		if !generator.requests[index].RepairOnly {
			t.Fatalf("request %d RepairOnly = false, want true for validation repair retry", index)
		}
		if generator.requests[index].PreviousBody != "" {
			t.Fatalf("request %d PreviousBody = %q, want empty", index, generator.requests[index].PreviousBody)
		}
		if !strings.Contains(generator.requests[index].PreviousErr, "counter-demo shell MyHomePage") {
			t.Fatalf("request %d PreviousErr = %q, want MyHomePage validation failure", index, generator.requests[index].PreviousErr)
		}
	}
	content, err := os.ReadFile(mainPath)
	if err != nil {
		t.Fatalf("ReadFile(main.dart) error = %v", err)
	}
	if strings.Contains(string(content), "MyHomePage") {
		t.Fatalf("main.dart content = %q, want counter-demo shell removed", string(content))
	}
	if !strings.Contains(string(content), "runApp(const MaterialApp(home: Placeholder()));") {
		t.Fatalf("main.dart content = %q, want repaired app entry", string(content))
	}
}

func TestExecuteBuilderRuntimeEditRepairsAmbiguousReplaceBlockApplyFailure(t *testing.T) {
	jobRoot := t.TempDir()
	workspace := filepath.Join(jobRoot, "workspace")
	repoPath := filepath.Join(workspace, "lib", "repositories", "entry_repository.dart")
	if err := os.MkdirAll(filepath.Dir(repoPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(repo) error = %v", err)
	}
	original := strings.Join([]string{
		"abstract class EntryRepository {",
		"  Future<List<String>> loadEntries();",
		"}",
		"",
		"class HiveEntryRepository implements EntryRepository {",
		"  @override",
		"  Future<List<String>> loadEntries() async {",
		"    return ['old'];",
		"  }",
		"}",
		"",
		"class InMemoryEntryRepository implements EntryRepository {",
		"  @override",
		"  Future<List<String>> loadEntries() async {",
		"    return ['cache'];",
		"  }",
		"}",
		"",
	}, "\n")
	if err := os.WriteFile(repoPath, []byte(original), 0o600); err != nil {
		t.Fatalf("WriteFile(repo) error = %v", err)
	}
	run := runRecord{
		RunID:         "run-entry-repo-repair",
		GoalSummary:   "修复 entry repository 并保持当前领域语义。",
		WorkspacePath: workspace,
		AllowedPaths:  []string{"lib/**"},
		TaskBundle: []appruns.TaskBundleItem{{
			TaskID:      "task-entry-repo",
			TaskType:    appruns.BuilderRuntimeTaskTypeSingleFileEdit,
			TargetPaths: []string{"lib/repositories/entry_repository.dart"},
		}},
		BuilderRuntime: &appruns.BuilderRuntimePlan{
			Enabled:      true,
			DefaultModel: appruns.BuilderRuntimeModelRef{Primary: "qwen2.5-coder-32b-local", Fallbacks: []string{"gpt-4o-mini"}},
			TaskRoutes: []appruns.BuilderRuntimeTaskRoute{{
				TaskID:      "task-entry-repo",
				TaskType:    appruns.BuilderRuntimeTaskTypeSingleFileEdit,
				RouteSource: "task_route",
				Model:       appruns.BuilderRuntimeModelRef{Primary: "qwen2.5-coder-32b-local", Fallbacks: []string{"gpt-4o-mini"}},
			}},
		},
		LogPath: filepath.Join(t.TempDir(), "builder-runtime-entry-repo.log"),
	}
	roundInput := appruns.RoundInput{
		RoundID:        "round-entry-repo",
		Attempt:        1,
		TaskBundle:     run.TaskBundle,
		AllowedPaths:   run.AllowedPaths,
		BuilderRuntime: run.BuilderRuntime,
	}
	runner := &Runner{PatchGenerator: &stubBuilderRuntimePatchGenerator{responses: []BuilderRuntimePatchResponse{
		{
			ModelAlias: "gpt-4o-mini",
			Content:    `{"patch_id":"round-entry-repo-1","operations":[{"type":"replace_block","path":"lib/repositories/entry_repository.dart","anchor":"loadEntries() async {","new_content":"  Future<List<String>> loadEntries() async {\n    return ['new'];\n  }\n"}]}`,
		},
		{
			ModelAlias: "gpt-4o-mini",
			Content:    "{\"patch_id\":\"round-entry-repo-2\",\"operations\":[{\"type\":\"write_file\",\"path\":\"lib/repositories/entry_repository.dart\",\"content\":\"abstract class EntryRepository {\\n  Future<List<String>> loadEntries();\\n}\\n\\nclass HiveEntryRepository implements EntryRepository {\\n  @override\\n  Future<List<String>> loadEntries() async {\\n    return ['new'];\\n  }\\n}\\n\\nclass InMemoryEntryRepository implements EntryRepository {\\n  @override\\n  Future<List<String>> loadEntries() async {\\n    return ['cache'];\\n  }\\n}\\n\"}]}",
		},
	}}}
	step := ExecutionStep{StepID: "thin-prepare", Stage: appruns.StageThinPrepare}

	result, err := runner.executeBuilderRuntimeEdit(context.Background(), noopRunnerBackend{}, run.RunID, step, run, roundInput)
	if err != nil {
		t.Fatalf("executeBuilderRuntimeEdit() error = %v, want patch-apply repair success", err)
	}
	if result == nil || result.Stats == nil {
		t.Fatalf("result = %+v, want stats", result)
	}
	if result.Stats.Attempts != 2 {
		t.Fatalf("stats.Attempts = %d, want 2", result.Stats.Attempts)
	}
	generator, ok := runner.PatchGenerator.(*stubBuilderRuntimePatchGenerator)
	if !ok {
		t.Fatalf("PatchGenerator = %T, want *stubBuilderRuntimePatchGenerator", runner.PatchGenerator)
	}
	if len(generator.requests) != 2 {
		t.Fatalf("len(generator.requests) = %d, want 2", len(generator.requests))
	}
	if !generator.requests[1].RepairOnly {
		t.Fatalf("repair request RepairOnly = false, want true")
	}
	if got := generator.requests[1].ModelAliases; len(got) == 0 || got[0] != "gpt-4o-mini" {
		t.Fatalf("repair request ModelAliases = %#v, want gpt-4o-mini first", got)
	}
	if !strings.Contains(generator.requests[1].PreviousErr, "replace_block expects exactly one match") {
		t.Fatalf("repair request PreviousErr = %q, want ambiguous replace_block error", generator.requests[1].PreviousErr)
	}
	if !strings.Contains(generator.requests[1].FailureContext, "replace_block expects exactly one match") {
		t.Fatalf("repair request FailureContext = %q, want ambiguous replace_block error", generator.requests[1].FailureContext)
	}
	content, err := os.ReadFile(repoPath)
	if err != nil {
		t.Fatalf("ReadFile(repo) error = %v", err)
	}
	if !strings.Contains(string(content), "return ['new'];") {
		t.Fatalf("repo content = %q, want repaired write_file content", string(content))
	}
}

func TestExecuteBuilderRuntimeEditRepairsBlankPatchBodyViaSchemaRepair(t *testing.T) {
	jobRoot := t.TempDir()
	workspace := filepath.Join(jobRoot, "workspace")
	repoPath := filepath.Join(workspace, "lib", "repositories", "entry_repository.dart")
	if err := os.MkdirAll(filepath.Dir(repoPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(repo) error = %v", err)
	}
	original := strings.Join([]string{
		"abstract class EntryRepository {",
		"  Future<List<String>> loadEntries();",
		"}",
		"",
		"class HiveEntryRepository implements EntryRepository {",
		"  @override",
		"  Future<List<String>> loadEntries() async {",
		"    return ['old'];",
		"  }",
		"}",
		"",
		"class InMemoryEntryRepository implements EntryRepository {",
		"  @override",
		"  Future<List<String>> loadEntries() async {",
		"    return ['cache'];",
		"  }",
		"}",
		"",
	}, "\n")
	if err := os.WriteFile(repoPath, []byte(original), 0o600); err != nil {
		t.Fatalf("WriteFile(repo) error = %v", err)
	}
	run := runRecord{
		RunID:         "run-entry-repo-blank-parse-repair",
		GoalSummary:   "修复 entry repository 并保持当前领域语义。",
		WorkspacePath: workspace,
		AllowedPaths:  []string{"lib/**"},
		TaskBundle: []appruns.TaskBundleItem{{
			TaskID:      "task-entry-repo",
			TaskType:    appruns.BuilderRuntimeTaskTypeSingleFileEdit,
			TargetPaths: []string{"lib/repositories/entry_repository.dart"},
		}},
		BuilderRuntime: &appruns.BuilderRuntimePlan{
			Enabled: true,
			DefaultModel: appruns.BuilderRuntimeModelRef{
				Primary: "gemma4-26b-local",
			},
			TaskRoutes: []appruns.BuilderRuntimeTaskRoute{{
				TaskID:      "task-entry-repo",
				TaskType:    appruns.BuilderRuntimeTaskTypeSingleFileEdit,
				RouteSource: "task_route",
				Model: appruns.BuilderRuntimeModelRef{
					Primary: "gemma4-26b-local",
				},
			}},
		},
		LogPath: filepath.Join(t.TempDir(), "builder-runtime-entry-repo-blank.log"),
	}
	roundInput := appruns.RoundInput{
		RoundID:        "round-entry-repo-blank",
		Attempt:        1,
		TaskBundle:     run.TaskBundle,
		AllowedPaths:   run.AllowedPaths,
		BuilderRuntime: run.BuilderRuntime,
	}
	runner := &Runner{PatchGenerator: &stubBuilderRuntimePatchGenerator{responses: []BuilderRuntimePatchResponse{
		{
			ModelAlias: "gemma4-26b-local",
			Content:    "",
		},
		{
			ModelAlias: "gemma4-26b-local",
			Content:    "{\"patch_id\":\"round-entry-repo-blank-repair\",\"operations\":[{\"type\":\"write_file\",\"path\":\"lib/repositories/entry_repository.dart\",\"content\":\"abstract class EntryRepository {\\n  Future<List<String>> loadEntries();\\n}\\n\\nclass HiveEntryRepository implements EntryRepository {\\n  @override\\n  Future<List<String>> loadEntries() async {\\n    return ['new'];\\n  }\\n}\\n\\nclass InMemoryEntryRepository implements EntryRepository {\\n  @override\\n  Future<List<String>> loadEntries() async {\\n    return ['cache'];\\n  }\\n}\\n\"}]}",
		},
	}}}
	step := ExecutionStep{StepID: "thin-prepare", Stage: appruns.StageThinPrepare}

	result, err := runner.executeBuilderRuntimeEdit(context.Background(), noopRunnerBackend{}, run.RunID, step, run, roundInput)
	if err != nil {
		t.Fatalf("executeBuilderRuntimeEdit() error = %v, want blank-body schema repair success", err)
	}
	if result == nil || result.Stats == nil {
		t.Fatalf("result = %+v, want stats", result)
	}
	if result.Stats.Attempts != 2 {
		t.Fatalf("stats.Attempts = %d, want 2", result.Stats.Attempts)
	}
	if result.Stats.ParseFailureCount != 1 {
		t.Fatalf("stats.ParseFailureCount = %d, want 1", result.Stats.ParseFailureCount)
	}
	generator, ok := runner.PatchGenerator.(*stubBuilderRuntimePatchGenerator)
	if !ok {
		t.Fatalf("PatchGenerator = %T, want *stubBuilderRuntimePatchGenerator", runner.PatchGenerator)
	}
	if len(generator.requests) != 2 {
		t.Fatalf("len(generator.requests) = %d, want 2", len(generator.requests))
	}
	if !generator.requests[1].RepairOnly {
		t.Fatalf("repair request RepairOnly = false, want true")
	}
	if generator.requests[1].PreviousBody != "" {
		t.Fatalf("repair request PreviousBody = %q, want empty body forwarded", generator.requests[1].PreviousBody)
	}
	if !strings.Contains(generator.requests[1].PreviousErr, "unexpected end of JSON input") {
		t.Fatalf("repair request PreviousErr = %q, want parse failure context", generator.requests[1].PreviousErr)
	}
	if got := generator.requests[1].ModelAliases; len(got) == 0 || got[0] != "gemma4-26b-local" {
		t.Fatalf("repair request ModelAliases = %#v, want preferred alias first", got)
	}
	content, err := os.ReadFile(repoPath)
	if err != nil {
		t.Fatalf("ReadFile(repo) error = %v", err)
	}
	if !strings.Contains(string(content), "return ['new'];") {
		t.Fatalf("repo content = %q, want repaired write_file content", string(content))
	}
}

func TestExecuteBuilderRuntimeEditRepairsDirectFailureCoverageGap(t *testing.T) {
	jobRoot := t.TempDir()
	workspace := filepath.Join(jobRoot, "workspace")
	entryRepoPath := filepath.Join(workspace, "lib", "repositories", "entry_repository.dart")
	listPagePath := filepath.Join(workspace, "lib", "views", "entry_list_page.dart")
	homePagePath := filepath.Join(workspace, "lib", "views", "home_page.dart")
	widgetTestPath := filepath.Join(workspace, "test", "widget_test.dart")
	for _, filePath := range []string{entryRepoPath, listPagePath, homePagePath, widgetTestPath} {
		if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
			t.Fatalf("MkdirAll(%s) error = %v", filePath, err)
		}
	}
	if err := os.WriteFile(entryRepoPath, []byte("class EntryRepository {}\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(entry repo) error = %v", err)
	}
	if err := os.WriteFile(listPagePath, []byte("class EntryListPage {}\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(list page) error = %v", err)
	}
	if err := os.WriteFile(homePagePath, []byte("class HomePage {}\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(home page) error = %v", err)
	}
	if err := os.WriteFile(widgetTestPath, []byte("void main() {}\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(widget test) error = %v", err)
	}
	run := runRecord{
		RunID:         "run-direct-failure-coverage-repair",
		GoalSummary:   "修复 bookkeeping analyze repair 漏文件问题。",
		WorkspacePath: workspace,
		AllowedPaths:  []string{"lib/**", "test/**"},
		TaskBundle: []appruns.TaskBundleItem{{
			TaskID:      "repair-check-flutter-analyze",
			TaskType:    appruns.BuilderRuntimeTaskTypeAnalyzeRepair,
			TargetPaths: []string{"lib/repositories/entry_repository.dart", "lib/views/entry_list_page.dart", "lib/views/home_page.dart", "test/widget_test.dart"},
		}},
		BuilderRuntime: &appruns.BuilderRuntimePlan{
			Enabled: true,
			TaskRoutes: []appruns.BuilderRuntimeTaskRoute{{
				TaskID:      "repair-check-flutter-analyze",
				TaskType:    appruns.BuilderRuntimeTaskTypeAnalyzeRepair,
				RouteSource: "task_route",
				Model:       appruns.BuilderRuntimeModelRef{Primary: "gemma4-26b-local", Fallbacks: []string{"gpt-4o-mini"}},
			}},
		},
		LogPath: filepath.Join(t.TempDir(), "builder-runtime-coverage-repair.log"),
	}
	roundInput := appruns.RoundInput{
		RoundID:        "round-analyze-repair",
		Attempt:        2,
		TaskBundle:     run.TaskBundle,
		AllowedPaths:   run.AllowedPaths,
		BuilderRuntime: run.BuilderRuntime,
	}
	runner := &Runner{PatchGenerator: &stubBuilderRuntimePatchGenerator{responses: []BuilderRuntimePatchResponse{
		{
			ModelAlias: "gemma4-26b-local",
			Content:    `{"patch_id":"round-analyze-repair-1","operations":[{"type":"write_file","path":"lib/repositories/entry_repository.dart","content":"class EntryRepository {\n  Future<void> save() async {}\n}\n"},{"type":"write_file","path":"lib/views/entry_list_page.dart","content":"class EntryListPage {\n  const EntryListPage();\n}\n"}]}`,
		},
		{
			ModelAlias: "gemma4-26b-local",
			Content:    `{"patch_id":"round-analyze-repair-2","operations":[{"type":"write_file","path":"lib/views/home_page.dart","content":"class HomePage {\n  const HomePage();\n}\n"},{"type":"write_file","path":"test/widget_test.dart","content":"void main() {\n  // repaired test scaffold\n}\n"}]}`,
		},
	}}}
	step := ExecutionStep{StepID: "check-flutter-analyze", Stage: appruns.StageCheap}

	result, err := runner.executeBuilderRuntimeEditWithFailureContext(context.Background(), noopRunnerBackend{}, run.RunID, step, run, roundInput, "flutter analyze failed in lib/repositories/entry_repository.dart, lib/views/entry_list_page.dart, lib/views/home_page.dart, test/widget_test.dart")
	if err != nil {
		t.Fatalf("executeBuilderRuntimeEditWithFailureContext() error = %v, want direct coverage repair success", err)
	}
	if result == nil || result.Stats == nil {
		t.Fatalf("result = %+v, want stats", result)
	}
	if result.Stats.Attempts != 2 {
		t.Fatalf("stats.Attempts = %d, want 2", result.Stats.Attempts)
	}
	generator, ok := runner.PatchGenerator.(*stubBuilderRuntimePatchGenerator)
	if !ok {
		t.Fatalf("PatchGenerator = %T, want *stubBuilderRuntimePatchGenerator", runner.PatchGenerator)
	}
	if len(generator.requests) != 2 {
		t.Fatalf("len(generator.requests) = %d, want 2", len(generator.requests))
	}
	if !generator.requests[1].RepairOnly {
		t.Fatalf("coverage repair request RepairOnly = false, want true")
	}
	if !strings.Contains(generator.requests[1].PreviousErr, "patch did not touch every directly failing repair target") {
		t.Fatalf("coverage repair request PreviousErr = %q, want direct coverage violation", generator.requests[1].PreviousErr)
	}
	if !strings.Contains(generator.requests[1].FailureContext, "lib/views/home_page.dart") || !strings.Contains(generator.requests[1].FailureContext, "test/widget_test.dart") {
		t.Fatalf("coverage repair request FailureContext = %q, want missing direct failure targets", generator.requests[1].FailureContext)
	}
	if !strings.Contains(generator.requests[1].PreviousBody, "entry_list_page.dart") {
		t.Fatalf("coverage repair request PreviousBody = %q, want prior partial patch body", generator.requests[1].PreviousBody)
	}
	if len(generator.requests[1].RoundInput.TaskBundle) != 1 || len(generator.requests[1].RoundInput.TaskBundle[0].TargetPaths) != 2 {
		t.Fatalf("coverage repair target_paths = %#v, want narrowed missing-file slice", generator.requests[1].RoundInput.TaskBundle)
	}
	if generator.requests[1].RoundInput.TaskBundle[0].TargetPaths[0] != "lib/views/home_page.dart" || generator.requests[1].RoundInput.TaskBundle[0].TargetPaths[1] != "test/widget_test.dart" {
		t.Fatalf("coverage repair target_paths = %v, want [lib/views/home_page.dart test/widget_test.dart]", generator.requests[1].RoundInput.TaskBundle[0].TargetPaths)
	}
	for _, check := range []struct {
		path   string
		needle string
	}{
		{entryRepoPath, "class EntryRepository"},
		{listPagePath, "const EntryListPage"},
		{homePagePath, "const HomePage"},
		{widgetTestPath, "repaired test scaffold"},
	} {
		content, readErr := os.ReadFile(check.path)
		if readErr != nil {
			t.Fatalf("ReadFile(%s) error = %v", check.path, readErr)
		}
		if !strings.Contains(string(content), check.needle) {
			t.Fatalf("content(%s) = %q, want %q", check.path, string(content), check.needle)
		}
	}
}

func TestRetryBuilderRuntimeDirectFailureCoverageRepairFallsBackToPatchApplyRepair(t *testing.T) {
	jobRoot := t.TempDir()
	workspace := filepath.Join(jobRoot, "workspace")
	mainPath := filepath.Join(workspace, "lib", "main.dart")
	controllerPath := filepath.Join(workspace, "lib", "controllers", "record_list_controller.dart")
	if err := os.MkdirAll(filepath.Dir(mainPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(main) error = %v", err)
	}
	if err := os.WriteFile(mainPath, []byte("import 'package:flutter/material.dart';\n\nvoid main() {}\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(main) error = %v", err)
	}
	run := runRecord{
		RunID:         "run-direct-failure-apply-repair",
		GoalSummary:   "修复 analyze repair 合并 patch 后的 replace_block 漂移。",
		WorkspacePath: workspace,
		AllowedPaths:  []string{"lib/**"},
		TaskBundle: []appruns.TaskBundleItem{{
			TaskID:      "repair-check-flutter-analyze",
			TaskType:    appruns.BuilderRuntimeTaskTypeAnalyzeRepair,
			TargetPaths: []string{"lib/main.dart", "lib/controllers/record_list_controller.dart"},
		}},
		BuilderRuntime: &appruns.BuilderRuntimePlan{
			Enabled: true,
			TaskRoutes: []appruns.BuilderRuntimeTaskRoute{{
				TaskID:      "repair-check-flutter-analyze",
				TaskType:    appruns.BuilderRuntimeTaskTypeAnalyzeRepair,
				RouteSource: "task_route",
				Model:       appruns.BuilderRuntimeModelRef{Primary: "qwen2.5-coder-32b-local", Fallbacks: []string{"gpt-4o-mini"}},
			}},
		},
		LogPath: filepath.Join(t.TempDir(), "builder-runtime-direct-failure-apply-repair.log"),
	}
	roundInput := appruns.RoundInput{
		RoundID:        "round-direct-failure-apply-repair",
		Attempt:        2,
		TaskBundle:     run.TaskBundle,
		AllowedPaths:   run.AllowedPaths,
		BuilderRuntime: run.BuilderRuntime,
	}
	route := run.BuilderRuntime.TaskRoutes[0]
	invalidPatch := &appruns.WorkspacePatch{
		PatchID: "repair-check-flutter-analyze-invalid",
		Operations: []appruns.WorkspacePatchOperation{{
			Type:       "replace_block",
			Path:       "lib/main.dart",
			OldContent: "import 'package:flutter_open_lite/main.dart' as AppRecord;",
			NewContent: "import 'package:flutter_open_lite/models/record.dart';\n",
		}},
	}
	invalidContent := `{"patch_id":"repair-check-flutter-analyze-invalid","operations":[{"type":"replace_block","path":"lib/main.dart","old_content":"import 'package:flutter_open_lite/main.dart' as AppRecord;","new_content":"import 'package:flutter_open_lite/models/record.dart';\n"}]}`
	coverageErr := validateBuilderRuntimeDirectFailureCoverage(route.TaskType, invalidPatch.Operations, []string{"lib/main.dart", "lib/controllers/record_list_controller.dart"})
	if coverageErr == nil {
		t.Fatal("validateBuilderRuntimeDirectFailureCoverage() error = nil, want missing target failure")
	}
	runner := &Runner{PatchGenerator: &stubBuilderRuntimePatchGenerator{responses: []BuilderRuntimePatchResponse{
		{
			ModelAlias: "gpt-4o-mini",
			Content:    `{"patch_id":"repair-check-flutter-analyze-coverage","operations":[{"type":"write_file","path":"lib/controllers/record_list_controller.dart","content":"class RecordListController {}\n"}]}`,
		},
		{
			ModelAlias: "gpt-4o-mini",
			Content:    `{"patch_id":"repair-check-flutter-analyze-apply","operations":[{"type":"write_file","path":"lib/main.dart","content":"import 'package:flutter/material.dart';\n\nvoid main() {}\n"},{"type":"write_file","path":"lib/controllers/record_list_controller.dart","content":"class RecordListController {}\n"}]}`,
		},
	}}}
	stats := &appruns.BuilderRuntimeExecutionStats{}
	step := ExecutionStep{StepID: "check-flutter-analyze", Stage: appruns.StageCheap}

	result, err := runner.retryBuilderRuntimeDirectFailureCoverageRepair(context.Background(), noopRunnerBackend{}, run.RunID, step, run, roundInput, route, stats, "gpt-4o-mini", invalidContent, invalidPatch, coverageErr, "flutter analyze failed: lib/main.dart, lib/controllers/record_list_controller.dart")
	if err != nil {
		t.Fatalf("retryBuilderRuntimeDirectFailureCoverageRepair() error = %v, want patch-apply repair success", err)
	}
	if result == nil || result.Stats == nil {
		t.Fatalf("result = %+v, want stats", result)
	}
	if result.Stats.Attempts != 2 {
		t.Fatalf("stats.Attempts = %d, want 2", result.Stats.Attempts)
	}
	generator, ok := runner.PatchGenerator.(*stubBuilderRuntimePatchGenerator)
	if !ok {
		t.Fatalf("PatchGenerator = %T, want *stubBuilderRuntimePatchGenerator", runner.PatchGenerator)
	}
	if len(generator.requests) != 2 {
		t.Fatalf("len(generator.requests) = %d, want 2", len(generator.requests))
	}
	if !strings.Contains(generator.requests[1].PreviousErr, "replace_block expects exactly one match") {
		t.Fatalf("patch-apply repair PreviousErr = %q, want replace_block ambiguity", generator.requests[1].PreviousErr)
	}
	if !strings.Contains(generator.requests[1].PreviousBody, "\"lib/main.dart\"") || !strings.Contains(generator.requests[1].PreviousBody, "\"lib/controllers/record_list_controller.dart\"") {
		t.Fatalf("patch-apply repair PreviousBody = %q, want combined patch json", generator.requests[1].PreviousBody)
	}
	mainContent, err := os.ReadFile(mainPath)
	if err != nil {
		t.Fatalf("ReadFile(main) error = %v", err)
	}
	if string(mainContent) != "import 'package:flutter/material.dart';\n\nvoid main() {}\n" {
		t.Fatalf("main content = %q, want repaired main.dart", string(mainContent))
	}
	controllerContent, err := os.ReadFile(controllerPath)
	if err != nil {
		t.Fatalf("ReadFile(controller) error = %v", err)
	}
	if string(controllerContent) != "class RecordListController {}\n" {
		t.Fatalf("controller content = %q, want repaired controller", string(controllerContent))
	}
}

func TestExecuteBuilderRuntimeEditRepairsTaskOutputValidationFailure(t *testing.T) {
	workspace := filepath.Join(t.TempDir(), "workspace")
	viewPath := filepath.Join(workspace, "lib", "views", "record_list_page.dart")
	run := runRecord{
		RunID:         "run-task-output-validation-repair",
		GoalSummary:   "修复集合承载单元的 Dart 语法错误。",
		WorkspacePath: workspace,
		AllowedPaths:  []string{"lib/**"},
		TaskBundle: []appruns.TaskBundleItem{{
			TaskID:      "task-bind-collection-surface",
			TaskType:    appruns.BuilderRuntimeTaskTypeDualFileWiring,
			TargetPaths: []string{"lib/views/record_list_page.dart"},
		}},
		BuilderRuntime: &appruns.BuilderRuntimePlan{
			Enabled: true,
			TaskRoutes: []appruns.BuilderRuntimeTaskRoute{{
				TaskID:      "task-bind-collection-surface",
				TaskType:    appruns.BuilderRuntimeTaskTypeDualFileWiring,
				RouteSource: "task_route",
				Model:       appruns.BuilderRuntimeModelRef{Primary: "gemma4-26b-local", Fallbacks: []string{"gpt-4o-mini"}},
			}},
		},
		LogPath: filepath.Join(t.TempDir(), "builder-runtime-task-output-validation-repair.log"),
	}
	roundInput := appruns.RoundInput{
		RoundID:        "round-task-output-validation-repair",
		Attempt:        1,
		TaskBundle:     run.TaskBundle,
		AllowedPaths:   run.AllowedPaths,
		BuilderRuntime: run.BuilderRuntime,
	}
	originalBuilder := builderRuntimeDartFormatterCommandBuilder
	t.Cleanup(func() {
		builderRuntimeDartFormatterCommandBuilder = originalBuilder
	})
	builderRuntimeDartFormatterCommandBuilder = func(ctx context.Context, run runRecord, relPath, absPath string) (*exec.Cmd, error) {
		content, err := os.ReadFile(absPath)
		if err != nil {
			return nil, err
		}
		if strings.Contains(string(content), "CrossেরAlignment") {
			return exec.CommandContext(ctx, "/bin/sh", "-lc", `printf 'Could not format because the source could not be parsed:

line 8, column 31 of lib/views/record_list_page.dart: Illegal character '\''2503'\''.
' >&2; exit 1`), nil
		}
		return exec.CommandContext(ctx, "/bin/sh", "-lc", "true"), nil
	}
	runner := &Runner{PatchGenerator: &stubBuilderRuntimePatchGenerator{responses: []BuilderRuntimePatchResponse{
		{
			ModelAlias: "gemma4-26b-local",
			Content:    `{"patch_id":"round-1-invalid-view","operations":[{"type":"write_file","path":"lib/views/record_list_page.dart","content":"import 'package:flutter/material.dart';\n\nclass RecordListPage extends StatelessWidget {\n  const RecordListPage({super.key});\n\n  @override\n  Widget build(BuildContext context) {\n    return Column(\n      crossAxisAlignment: CrossেরAlignment.start,\n      children: const [Text('bad')],\n    );\n  }\n}\n"}]}`,
		},
		{
			ModelAlias: "gemma4-26b-local",
			Content:    `{"patch_id":"round-1-repair-view","operations":[{"type":"write_file","path":"lib/views/record_list_page.dart","content":"import 'package:flutter/material.dart';\n\nclass RecordListPage extends StatelessWidget {\n  const RecordListPage({super.key});\n\n  @override\n  Widget build(BuildContext context) {\n    return Column(\n      crossAxisAlignment: CrossAxisAlignment.start,\n      children: const [Text('ok')],\n    );\n  }\n}\n"}]}`,
		},
	}}}
	step := ExecutionStep{StepID: "thin-prepare", Stage: appruns.StageThinPrepare}

	result, err := runner.executeBuilderRuntimeEdit(context.Background(), noopRunnerBackend{}, run.RunID, step, run, roundInput)
	if err != nil {
		t.Fatalf("executeBuilderRuntimeEdit() error = %v, want task-output validation repair success", err)
	}
	if result == nil || result.Stats == nil {
		t.Fatalf("result = %+v, want stats", result)
	}
	if result.Stats.Attempts != 2 {
		t.Fatalf("stats.Attempts = %d, want 2", result.Stats.Attempts)
	}
	generator, ok := runner.PatchGenerator.(*stubBuilderRuntimePatchGenerator)
	if !ok {
		t.Fatalf("PatchGenerator = %T, want *stubBuilderRuntimePatchGenerator", runner.PatchGenerator)
	}
	if len(generator.requests) != 2 {
		t.Fatalf("len(generator.requests) = %d, want 2", len(generator.requests))
	}
	if !generator.requests[1].RepairOnly {
		t.Fatalf("validation repair request RepairOnly = false, want true")
	}
	if !strings.Contains(generator.requests[1].PreviousErr, "failed Dart syntax validation") {
		t.Fatalf("validation repair request PreviousErr = %q, want Dart syntax validation failure", generator.requests[1].PreviousErr)
	}
	if generator.requests[1].PreviousBody != "" {
		t.Fatalf("validation repair request PreviousBody = %q, want empty body to avoid replaying corrupted patch", generator.requests[1].PreviousBody)
	}
	if !strings.Contains(generator.requests[1].FailureContext, "Illegal character '2503'") {
		t.Fatalf("validation repair request FailureContext = %q, want formatter failure detail", generator.requests[1].FailureContext)
	}
	if len(generator.requests[1].RoundInput.TaskBundle) != 1 || len(generator.requests[1].RoundInput.TaskBundle[0].TargetPaths) != 1 {
		t.Fatalf("validation repair target_paths = %#v, want narrowed single-file repair slice", generator.requests[1].RoundInput.TaskBundle)
	}
	if generator.requests[1].RoundInput.TaskBundle[0].TargetPaths[0] != "lib/views/record_list_page.dart" {
		t.Fatalf("validation repair target_paths = %v, want lib/views/record_list_page.dart", generator.requests[1].RoundInput.TaskBundle[0].TargetPaths)
	}
	content, err := os.ReadFile(viewPath)
	if err != nil {
		t.Fatalf("ReadFile(record_list_page.dart) error = %v", err)
	}
	if strings.Contains(string(content), "CrossেরAlignment") {
		t.Fatalf("view content = %q, want localized identifier removed", string(content))
	}
	if !strings.Contains(string(content), "CrossAxisAlignment.start") {
		t.Fatalf("view content = %q, want repaired Flutter identifier", string(content))
	}
}

func TestExecuteBuilderRuntimeEditRepairsUnresolvedLocalImportTaskOutputValidationFailure(t *testing.T) {
	workspace := filepath.Join(t.TempDir(), "workspace")
	viewPath := filepath.Join(workspace, "lib", "views", "record_list_page.dart")
	controllerPath := filepath.Join(workspace, "lib", "controllers", "record_list_controller.dart")
	if err := os.MkdirAll(filepath.Dir(viewPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(viewPath) error = %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(controllerPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(controllerPath) error = %v", err)
	}
	baseline := strings.Join([]string{
		"import 'package:flutter/material.dart';",
		"",
		"class RecordListPage extends StatelessWidget {",
		"  const RecordListPage({super.key});",
		"",
		"  @override",
		"  Widget build(BuildContext context) => const SizedBox.shrink();",
		"}",
	}, "\n") + "\n"
	if err := os.WriteFile(viewPath, []byte(baseline), 0o600); err != nil {
		t.Fatalf("WriteFile(viewPath) error = %v", err)
	}
	if err := os.WriteFile(controllerPath, []byte(strings.Join([]string{
		"class RecordListController {",
		"  const RecordListController();",
		"}",
	}, "\n")+"\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(controllerPath) error = %v", err)
	}
	run := runRecord{
		RunID:         "run-task-output-validation-local-import-repair",
		GoalSummary:   "修复集合承载单元的本地导入漂移。",
		WorkspacePath: workspace,
		AllowedPaths:  []string{"lib/**"},
		TaskBundle: []appruns.TaskBundleItem{{
			TaskID:      "task-bind-collection-surface",
			TaskType:    appruns.BuilderRuntimeTaskTypeDualFileWiring,
			TargetPaths: []string{"lib/views/record_list_page.dart"},
		}},
		BuilderRuntime: &appruns.BuilderRuntimePlan{
			Enabled: true,
			TaskRoutes: []appruns.BuilderRuntimeTaskRoute{{
				TaskID:      "task-bind-collection-surface",
				TaskType:    appruns.BuilderRuntimeTaskTypeDualFileWiring,
				RouteSource: "task_route",
				Model:       appruns.BuilderRuntimeModelRef{Primary: "gemma4-26b-local", Fallbacks: []string{"gpt-4o-mini"}},
			}},
		},
		LogPath: filepath.Join(t.TempDir(), "builder-runtime-task-output-validation-local-import-repair.log"),
	}
	roundInput := appruns.RoundInput{
		RoundID:        "round-task-output-validation-local-import-repair",
		Attempt:        1,
		TaskBundle:     run.TaskBundle,
		AllowedPaths:   run.AllowedPaths,
		BuilderRuntime: run.BuilderRuntime,
	}
	generator := &inspectingBuilderRuntimePatchGenerator{
		responses: []BuilderRuntimePatchResponse{
			{
				ModelAlias: "gemma4-26b-local",
				Content:    `{"patch_id":"round-local-import-invalid-view","operations":[{"type":"write_file","path":"lib/views/record_list_page.dart","content":"import 'package:flutter/material.dart';\n\nimport '../controllers/record_list_page.dart';\n\nclass RecordListPage extends StatelessWidget {\n  const RecordListPage({super.key, required this.controller});\n\n  final RecordListController controller;\n\n  @override\n  Widget build(BuildContext context) => const SizedBox.shrink();\n}\n"}]}`,
			},
			{
				ModelAlias: "gemma4-26b-local",
				Content:    `{"patch_id":"round-local-import-repair-view","operations":[{"type":"write_file","path":"lib/views/record_list_page.dart","content":"import 'package:flutter/material.dart';\n\nimport '../controllers/record_list_controller.dart';\n\nclass RecordListPage extends StatelessWidget {\n  const RecordListPage({super.key, required this.controller});\n\n  final RecordListController controller;\n\n  @override\n  Widget build(BuildContext context) => const SizedBox.shrink();\n}\n"}]}`,
			},
		},
		inspect: func(request BuilderRuntimePatchRequest, index int) error {
			if index != 1 {
				return nil
			}
			content, err := os.ReadFile(viewPath)
			if err != nil {
				return err
			}
			if got := string(content); got != baseline {
				return fmt.Errorf("workspace content before repair = %q, want restored baseline", got)
			}
			if request.PreviousBody != "" {
				return fmt.Errorf("repair request PreviousBody = %q, want empty", request.PreviousBody)
			}
			if !strings.Contains(request.PreviousErr, "unresolved local import/export/part") {
				return fmt.Errorf("repair request PreviousErr = %q, want unresolved local import detail", request.PreviousErr)
			}
			if !strings.Contains(request.PreviousErr, "../controllers/record_list_page.dart") {
				return fmt.Errorf("repair request PreviousErr = %q, want wrong controller import path", request.PreviousErr)
			}
			return nil
		},
	}
	runner := &Runner{PatchGenerator: generator}
	step := ExecutionStep{StepID: "thin-prepare", Stage: appruns.StageThinPrepare}

	result, err := runner.executeBuilderRuntimeEdit(context.Background(), noopRunnerBackend{}, run.RunID, step, run, roundInput)
	if err != nil {
		t.Fatalf("executeBuilderRuntimeEdit() error = %v, want unresolved-local-import validation repair success", err)
	}
	if result == nil || result.Stats == nil {
		t.Fatalf("result = %+v, want stats", result)
	}
	if result.Stats.Attempts != 2 {
		t.Fatalf("stats.Attempts = %d, want 2", result.Stats.Attempts)
	}
	if len(generator.requests) != 2 {
		t.Fatalf("len(generator.requests) = %d, want 2", len(generator.requests))
	}
	if !generator.requests[1].RepairOnly {
		t.Fatalf("validation repair request RepairOnly = false, want true")
	}
	if got := generator.requests[1].RoundInput.TaskBundle[0].TargetPaths; len(got) != 1 || got[0] != "lib/views/record_list_page.dart" {
		t.Fatalf("validation repair target_paths = %v, want [lib/views/record_list_page.dart]", got)
	}
	content, err := os.ReadFile(viewPath)
	if err != nil {
		t.Fatalf("ReadFile(record_list_page.dart) error = %v", err)
	}
	if strings.Contains(string(content), "record_list_page.dart") {
		t.Fatalf("view content = %q, want wrong local import removed", string(content))
	}
	if !strings.Contains(string(content), "../controllers/record_list_controller.dart") {
		t.Fatalf("view content = %q, want repaired local import", string(content))
	}
}

func TestExecuteBuilderRuntimeEditRepairsCounterDemoShellTaskOutputValidationFailure(t *testing.T) {
	workspace := filepath.Join(t.TempDir(), "workspace")
	mainPath := filepath.Join(workspace, "lib", "main.dart")
	if err := os.MkdirAll(filepath.Dir(mainPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(mainPath) error = %v", err)
	}
	baseline := "import 'package:flutter/material.dart';\n\nvoid main() {\n  runApp(const Placeholder());\n}\n"
	if err := os.WriteFile(mainPath, []byte(baseline), 0o600); err != nil {
		t.Fatalf("WriteFile(mainPath) error = %v", err)
	}
	run := runRecord{
		RunID:         "run-task-output-validation-counter-demo",
		GoalSummary:   "修复 app entry 的 counter demo shell 漂移。",
		WorkspacePath: workspace,
		AllowedPaths:  []string{"lib/**"},
		TaskBundle: []appruns.TaskBundleItem{{
			TaskID:      "task-bind-app-entry",
			TaskType:    appruns.BuilderRuntimeTaskTypeSingleFileEdit,
			TargetPaths: []string{"lib/main.dart"},
		}},
		BuilderRuntime: &appruns.BuilderRuntimePlan{
			Enabled: true,
			TaskRoutes: []appruns.BuilderRuntimeTaskRoute{{
				TaskID:      "task-bind-app-entry",
				TaskType:    appruns.BuilderRuntimeTaskTypeSingleFileEdit,
				RouteSource: "task_route",
				Model:       appruns.BuilderRuntimeModelRef{Primary: "gemma4-26b-local", Fallbacks: []string{"gpt-4o-mini"}},
			}},
		},
		LogPath: filepath.Join(t.TempDir(), "builder-runtime-task-output-validation-counter-demo.log"),
	}
	roundInput := appruns.RoundInput{
		RoundID:        "round-task-output-validation-counter-demo",
		Attempt:        1,
		TaskBundle:     run.TaskBundle,
		AllowedPaths:   run.AllowedPaths,
		BuilderRuntime: run.BuilderRuntime,
	}
	generator := &inspectingBuilderRuntimePatchGenerator{
		responses: []BuilderRuntimePatchResponse{
			{
				ModelAlias: "gemma4-26b-local",
				Content:    `{"patch_id":"round-counter-demo-invalid-main","operations":[{"type":"write_file","path":"lib/main.dart","content":"import 'package:flutter/material.dart';\n\nvoid main() {\n  runApp(const MaterialApp(home: MyHomePage()));\n}\n\nclass MyHomePage extends StatelessWidget {\n  const MyHomePage({super.key});\n\n  @override\n  Widget build(BuildContext context) => const Placeholder();\n}\n"}]}`,
			},
			{
				ModelAlias: "gemma4-26b-local",
				Content:    `{"patch_id":"round-counter-demo-repair-main","operations":[{"type":"write_file","path":"lib/main.dart","content":"import 'package:flutter/material.dart';\n\nvoid main() {\n  runApp(const MaterialApp(home: Placeholder()));\n}\n"}]}`,
			},
		},
		inspect: func(request BuilderRuntimePatchRequest, index int) error {
			if index != 1 {
				return nil
			}
			content, err := os.ReadFile(mainPath)
			if err != nil {
				return err
			}
			if got := string(content); got != baseline {
				return fmt.Errorf("workspace content before repair = %q, want restored baseline", got)
			}
			if request.PreviousBody != "" {
				return fmt.Errorf("repair request PreviousBody = %q, want empty", request.PreviousBody)
			}
			if !strings.Contains(request.PreviousErr, "counter-demo shell MyHomePage") {
				return fmt.Errorf("repair request PreviousErr = %q, want MyHomePage counter-demo detail", request.PreviousErr)
			}
			return nil
		},
	}
	runner := &Runner{PatchGenerator: generator}
	step := ExecutionStep{StepID: "thin-prepare", Stage: appruns.StageThinPrepare}

	result, err := runner.executeBuilderRuntimeEdit(context.Background(), noopRunnerBackend{}, run.RunID, step, run, roundInput)
	if err != nil {
		t.Fatalf("executeBuilderRuntimeEdit() error = %v, want task-output validation repair success", err)
	}
	if result == nil || result.Stats == nil {
		t.Fatalf("result = %+v, want stats", result)
	}
	if result.Stats.Attempts != 2 {
		t.Fatalf("stats.Attempts = %d, want 2", result.Stats.Attempts)
	}
	content, err := os.ReadFile(mainPath)
	if err != nil {
		t.Fatalf("ReadFile(main.dart) error = %v", err)
	}
	if strings.Contains(string(content), "MyHomePage") {
		t.Fatalf("main.dart content = %q, want counter-demo shell removed", string(content))
	}
	if !strings.Contains(string(content), "runApp(const MaterialApp(home: Placeholder()));") {
		t.Fatalf("main.dart content = %q, want repaired app entry", string(content))
	}
}

func TestExecuteBuilderRuntimeEditRepairsMainViewConstructorTaskOutputValidationFailure(t *testing.T) {
	workspace := filepath.Join(t.TempDir(), "workspace")
	mainPath := filepath.Join(workspace, "lib", "main.dart")
	homePath := filepath.Join(workspace, "lib", "views", "home_page.dart")
	if err := os.MkdirAll(filepath.Dir(homePath), 0o755); err != nil {
		t.Fatalf("MkdirAll(homePath) error = %v", err)
	}
	baseline := "import 'package:flutter/material.dart';\n\nvoid main() {\n  runApp(const Placeholder());\n}\n"
	if err := os.WriteFile(mainPath, []byte(baseline), 0o600); err != nil {
		t.Fatalf("WriteFile(mainPath) error = %v", err)
	}
	if err := os.WriteFile(homePath, []byte(strings.Join([]string{
		"import 'package:flutter/material.dart';",
		"",
		"class HomePage extends StatelessWidget {",
		"  const HomePage({",
		"    super.key,",
		"    required this.controller,",
		"    required this.onCreateRecord,",
		"    required this.onViewAllRecords,",
		"    required this.onOpenRecordDetail,",
		"  });",
		"",
		"  final Object controller;",
		"  final Future<void> Function() onCreateRecord;",
		"  final Future<void> Function() onViewAllRecords;",
		"  final Future<void> Function(Object record) onOpenRecordDetail;",
		"",
		"  @override",
		"  Widget build(BuildContext context) => const Placeholder();",
		"}",
	}, "\n")+"\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(homePath) error = %v", err)
	}
	run := runRecord{
		RunID:         "run-task-output-validation-main-callsite",
		GoalSummary:   "修复 app entry 的页面构造器漂移。",
		WorkspacePath: workspace,
		AllowedPaths:  []string{"lib/**"},
		TaskBundle: []appruns.TaskBundleItem{{
			TaskID:      "task-bind-app-entry",
			TaskType:    appruns.BuilderRuntimeTaskTypeSingleFileEdit,
			TargetPaths: []string{"lib/main.dart"},
		}},
		BuilderRuntime: &appruns.BuilderRuntimePlan{
			Enabled: true,
			TaskRoutes: []appruns.BuilderRuntimeTaskRoute{{
				TaskID:      "task-bind-app-entry",
				TaskType:    appruns.BuilderRuntimeTaskTypeSingleFileEdit,
				RouteSource: "task_route",
				Model:       appruns.BuilderRuntimeModelRef{Primary: "gemma4-26b-local", Fallbacks: []string{"gpt-4o-mini"}},
			}},
		},
		LogPath: filepath.Join(t.TempDir(), "builder-runtime-task-output-validation-main-callsite.log"),
	}
	roundInput := appruns.RoundInput{
		RoundID:        "round-task-output-validation-main-callsite",
		Attempt:        1,
		TaskBundle:     run.TaskBundle,
		AllowedPaths:   run.AllowedPaths,
		BuilderRuntime: run.BuilderRuntime,
	}
	generator := &inspectingBuilderRuntimePatchGenerator{
		responses: []BuilderRuntimePatchResponse{
			{
				ModelAlias: "gemma4-26b-local",
				Content:    `{"patch_id":"round-main-invalid-callsite","operations":[{"type":"write_file","path":"lib/main.dart","content":"import 'package:flutter/material.dart';\n\nimport 'views/home_page.dart';\n\nvoid main() {\n  runApp(const WeightTrackerApp());\n}\n\nclass WeightTrackerApp extends StatelessWidget {\n  const WeightTrackerApp({super.key});\n\n  @override\n  Widget build(BuildContext context) {\n    return const MaterialApp(home: HomePage());\n  }\n}\n"}]}`,
			},
			{
				ModelAlias: "gemma4-26b-local",
				Content:    `{"patch_id":"round-main-repair-callsite","operations":[{"type":"write_file","path":"lib/main.dart","content":"import 'package:flutter/material.dart';\n\nimport 'views/home_page.dart';\n\nFuture<void> _noop() async {}\nFuture<void> _noopRecord(Object record) async {}\n\nvoid main() {\n  runApp(\n    MaterialApp(\n      home: HomePage(\n        controller: Object(),\n        onCreateRecord: _noop,\n        onViewAllRecords: _noop,\n        onOpenRecordDetail: _noopRecord,\n      ),\n    ),\n  );\n}\n"}]}`,
			},
		},
	}
	runner := &Runner{PatchGenerator: generator}
	step := ExecutionStep{StepID: "thin-prepare", Stage: appruns.StageThinPrepare}

	result, err := runner.executeBuilderRuntimeEdit(context.Background(), noopRunnerBackend{}, run.RunID, step, run, roundInput)
	if err != nil {
		t.Fatalf("executeBuilderRuntimeEdit() error = %v, want task-output validation repair success", err)
	}
	if result == nil || result.Stats == nil {
		t.Fatalf("result = %+v, want stats", result)
	}
	if result.Stats.Attempts != 1 {
		t.Fatalf("stats.Attempts = %d, want 1 after main normalize repairs constructor drift inline", result.Stats.Attempts)
	}
	if len(generator.requests) != 1 {
		t.Fatalf("len(generator.requests) = %d, want 1 because main normalize should avoid validation repair round", len(generator.requests))
	}
	content, err := os.ReadFile(mainPath)
	if err != nil {
		t.Fatalf("ReadFile(main.dart) error = %v", err)
	}
	for _, want := range []string{"controller: Object()", "onCreateRecord: _noop", "onViewAllRecords: _noop", "onOpenRecordDetail: _noopRecord"} {
		if !strings.Contains(string(content), want) {
			t.Fatalf("main.dart content = %q, want normalized constructor argument %q", string(content), want)
		}
	}
	if !strings.Contains(string(content), "onOpenRecordDetail: _noopRecord") {
		t.Fatalf("main.dart content = %q, want repaired HomePage constructor wiring", string(content))
	}
}

func TestExecuteBuilderRuntimeEditRepairsMainViewConstructorTaskOutputValidationFailureForCustomViewFileName(t *testing.T) {
	workspace := filepath.Join(t.TempDir(), "workspace")
	mainPath := filepath.Join(workspace, "lib", "main.dart")
	overviewPath := filepath.Join(workspace, "lib", "views", "task_overview_page.dart")
	if err := os.MkdirAll(filepath.Dir(overviewPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(overviewPath) error = %v", err)
	}
	baseline := "import 'package:flutter/material.dart';\n\nvoid main() {\n  runApp(const Placeholder());\n}\n"
	if err := os.WriteFile(mainPath, []byte(baseline), 0o600); err != nil {
		t.Fatalf("WriteFile(mainPath) error = %v", err)
	}
	if err := os.WriteFile(overviewPath, []byte(strings.Join([]string{
		"import 'package:flutter/material.dart';",
		"",
		"class TaskOverviewPage extends StatelessWidget {",
		"  const TaskOverviewPage({",
		"    super.key,",
		"    required this.controller,",
		"    required this.onCreateTask,",
		"    required this.onViewAllTasks,",
		"  });",
		"",
		"  final Object controller;",
		"  final Future<void> Function() onCreateTask;",
		"  final Future<void> Function() onViewAllTasks;",
		"",
		"  @override",
		"  Widget build(BuildContext context) => const Placeholder();",
		"}",
	}, "\n")+"\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(task_overview_page.dart) error = %v", err)
	}
	run := runRecord{
		RunID:         "run-task-output-validation-main-callsite-custom-view",
		GoalSummary:   "修复 custom overview app entry 的页面构造器漂移。",
		WorkspacePath: workspace,
		AllowedPaths:  []string{"lib/**"},
		TaskBundle: []appruns.TaskBundleItem{{
			TaskID:      "task-bind-app-entry",
			TaskType:    appruns.BuilderRuntimeTaskTypeSingleFileEdit,
			TargetPaths: []string{"lib/main.dart"},
		}},
		BuilderRuntime: &appruns.BuilderRuntimePlan{
			Enabled: true,
			TaskRoutes: []appruns.BuilderRuntimeTaskRoute{{
				TaskID:      "task-bind-app-entry",
				TaskType:    appruns.BuilderRuntimeTaskTypeSingleFileEdit,
				RouteSource: "task_route",
				Model:       appruns.BuilderRuntimeModelRef{Primary: "gemma4-26b-local", Fallbacks: []string{"gpt-4o-mini"}},
			}},
		},
		LogPath: filepath.Join(t.TempDir(), "builder-runtime-task-output-validation-main-callsite-custom-view.log"),
	}
	roundInput := appruns.RoundInput{
		RoundID:        "round-task-output-validation-main-callsite-custom-view",
		Attempt:        1,
		TaskBundle:     run.TaskBundle,
		AllowedPaths:   run.AllowedPaths,
		BuilderRuntime: run.BuilderRuntime,
	}
	generator := &inspectingBuilderRuntimePatchGenerator{
		responses: []BuilderRuntimePatchResponse{{
			ModelAlias: "gemma4-26b-local",
			Content:    `{"patch_id":"round-main-invalid-callsite-custom-view","operations":[{"type":"write_file","path":"lib/main.dart","content":"import 'package:flutter/material.dart';\n\nimport 'views/task_overview_page.dart';\n\nvoid main() {\n  runApp(const TaskTrackerApp());\n}\n\nclass TaskTrackerApp extends StatelessWidget {\n  const TaskTrackerApp({super.key});\n\n  @override\n  Widget build(BuildContext context) {\n    return const MaterialApp(home: TaskOverviewPage());\n  }\n}\n"}]}`,
		}, {
			ModelAlias: "gemma4-26b-local",
			Content:    `{"patch_id":"round-main-repair-callsite-custom-view","operations":[{"type":"write_file","path":"lib/main.dart","content":"import 'package:flutter/material.dart';\n\nimport 'views/task_overview_page.dart';\n\nFuture<void> _noop() async {}\n\nvoid main() {\n  runApp(\n    MaterialApp(\n      home: TaskOverviewPage(\n        controller: Object(),\n        onCreateTask: _noop,\n        onViewAllTasks: _noop,\n      ),\n    ),\n  );\n}\n"}]}`,
		}},
	}
	runner := &Runner{PatchGenerator: generator}
	step := ExecutionStep{StepID: "thin-prepare", Stage: appruns.StageThinPrepare}

	result, err := runner.executeBuilderRuntimeEdit(context.Background(), noopRunnerBackend{}, run.RunID, step, run, roundInput)
	if err != nil {
		t.Fatalf("executeBuilderRuntimeEdit() error = %v, want task-output validation repair success for custom view file name", err)
	}
	if result == nil || result.Stats == nil {
		t.Fatalf("result = %+v, want stats", result)
	}
	if result.Stats.Attempts != 1 {
		t.Fatalf("stats.Attempts = %d, want 1 after main normalize repairs custom view constructor drift inline", result.Stats.Attempts)
	}
	if len(generator.requests) != 1 {
		t.Fatalf("len(generator.requests) = %d, want 1 because main normalize should avoid validation repair round for custom view file name", len(generator.requests))
	}
	content, err := os.ReadFile(mainPath)
	if err != nil {
		t.Fatalf("ReadFile(main.dart) error = %v", err)
	}
	for _, want := range []string{"controller: Object()", "onCreateTask: _noop", "onViewAllTasks: _noop"} {
		if !strings.Contains(string(content), want) {
			t.Fatalf("main.dart content = %q, want normalized constructor argument %q", string(content), want)
		}
	}
}

func TestNormalizeBuilderRuntimeOpenLiteMainHomePageConstructorCallsUsesOverviewRegistryForCustomViewAllCallbackName(t *testing.T) {
	workspacePath := t.TempDir()
	writeBuilderRuntimeWorkspaceFiles(t, workspacePath, map[string]string{
		"lib/views/task_overview_page.dart": strings.Join([]string{
			"import 'package:flutter/material.dart';",
			"",
			"class TaskOverviewPage extends StatelessWidget {",
			"  const TaskOverviewPage({",
			"    super.key,",
			"    required this.controller,",
			"    required this.onCreateTask,",
			"    required this.onBrowseTasks,",
			"  });",
			"",
			"  final Object controller;",
			"  final Future<void> Function() onCreateTask;",
			"  final Future<void> Function() onBrowseTasks;",
			"",
			"  @override",
			"  Widget build(BuildContext context) => const SizedBox.shrink();",
			"}",
		}, "\n") + "\n",
	})

	content := strings.Join([]string{
		"import 'package:flutter/material.dart';",
		"",
		"import 'views/task_overview_page.dart';",
		"",
		"Future<void> _noop() async {}",
		"Future<void> _openTaskList() async {}",
		"",
		"void main() {",
		"  runApp(",
		"    MaterialApp(",
		"      home: TaskOverviewPage(",
		"        controller: Object(),",
		"        onCreateTask: _noop,",
		"      ),",
		"    ),",
		"  );",
		"}",
	}, "\n") + "\n"

	normalized := normalizeBuilderRuntimeOpenLiteMainHomePageConstructorCalls(workspacePath, content)
	if !strings.Contains(normalized, "onBrowseTasks: _openTaskList,") {
		t.Fatalf("normalizeBuilderRuntimeOpenLiteMainHomePageConstructorCalls() should reuse overview registry for custom view-all callback name: %q", normalized)
	}
	if strings.Contains(normalized, "onBrowseTasks: _noop,") {
		t.Fatalf("normalizeBuilderRuntimeOpenLiteMainHomePageConstructorCalls() should not degrade custom view-all callback to generic _noop when _openTaskList exists: %q", normalized)
	}
}

func TestNormalizeBuilderRuntimeOpenLiteMainCollectionCreateFlowUsesCollectionRegistryCallbackName(t *testing.T) {
	workspacePath := t.TempDir()
	writeBuilderRuntimeWorkspaceFiles(t, workspacePath, map[string]string{
		"lib/repositories/task_repository.dart": "abstract class TaskRepository {}\n",
		"lib/controllers/task_collection_controller.dart": strings.Join([]string{
			"class TaskCollectionController {",
			"  TaskCollectionController({required this.taskRepository});",
			"  final TaskRepository taskRepository;",
			"  Future<void> refresh() async {}",
			"}",
		}, "\n") + "\n",
		"lib/controllers/task_form_controller.dart": "class TaskFormController {}\n",
		"lib/views/task_collection_page.dart": strings.Join([]string{
			"class TaskCollectionPage {",
			"  const TaskCollectionPage({required this.controller, required this.onCreateTask, required this.onOpenTaskDetail});",
			"  final TaskCollectionController controller;",
			"  final Future<void> Function() onCreateTask;",
			"  final Future<void> Function(Object task) onOpenTaskDetail;",
			"}",
		}, "\n") + "\n",
		"lib/views/task_form_page.dart": strings.Join([]string{
			"class TaskFormPage {",
			"  const TaskFormPage({required this.taskRepository, this.initialTask});",
			"  final TaskRepository taskRepository;",
			"  final Object? initialTask;",
			"}",
		}, "\n") + "\n",
	})

	content := strings.Join([]string{
		"import 'package:flutter/material.dart';",
		"",
		"import 'views/task_collection_page.dart';",
		"",
		"class TaskTrackerApp extends StatelessWidget {",
		"  const TaskTrackerApp({super.key, required this.repository});",
		"  final TaskRepository repository;",
		"",
		"  @override",
		"  Widget build(BuildContext context) {",
		"    final listController = TaskCollectionController(taskRepository: repository);",
		"    return MaterialApp(",
		"      home: TaskCollectionPage(",
		"        controller: listController,",
		"      ),",
		"    );",
		"  }",
		"}",
	}, "\n") + "\n"

	normalized := normalizeBuilderRuntimeOpenLiteMainCollectionCreateFlow(workspacePath, true, content, "TaskTrackerApp")
	if !strings.Contains(normalized, "onCreateTask: () async {") {
		t.Fatalf("normalizeBuilderRuntimeOpenLiteMainCollectionCreateFlow() should inject custom create callback name from collection registry: %q", normalized)
	}
	if strings.Contains(normalized, "onCreateRecord:") {
		t.Fatalf("normalizeBuilderRuntimeOpenLiteMainCollectionCreateFlow() should not fall back to onCreateRecord when current collection surface uses onCreateTask: %q", normalized)
	}
}

func TestNormalizeBuilderRuntimeOpenLiteMainRecordFormCallbacksUsesCollectionRegistryDetailCallbackName(t *testing.T) {
	workspacePath := t.TempDir()
	writeBuilderRuntimeWorkspaceFiles(t, workspacePath, map[string]string{
		"lib/repositories/task_repository.dart": "abstract class TaskRepository {}\n",
		"lib/controllers/task_collection_controller.dart": strings.Join([]string{
			"class TaskCollectionController {",
			"  TaskCollectionController({required this.taskRepository});",
			"  final TaskRepository taskRepository;",
			"  Future<void> refresh() async {}",
			"}",
		}, "\n") + "\n",
		"lib/views/task_collection_page.dart": strings.Join([]string{
			"class TaskCollectionPage {",
			"  const TaskCollectionPage({required this.controller, required this.onCreateTask, required this.onOpenTaskDetail});",
			"  final TaskCollectionController controller;",
			"  final Future<void> Function() onCreateTask;",
			"  final Future<void> Function(Object task) onOpenTaskDetail;",
			"}",
		}, "\n") + "\n",
		"lib/views/task_form_page.dart": strings.Join([]string{
			"class TaskFormPage {",
			"  const TaskFormPage({required this.taskRepository, this.initialTask});",
			"  final TaskRepository taskRepository;",
			"  final Object? initialTask;",
			"}",
		}, "\n") + "\n",
	})

	content := strings.Join([]string{
		"import 'package:flutter/material.dart';",
		"",
		"import 'views/task_form_page.dart';",
		"",
		"class TaskTrackerApp extends StatelessWidget {",
		"  const TaskTrackerApp({super.key});",
		"",
		"  @override",
		"  Widget build(BuildContext context) {",
		"    final listController = TaskCollectionController(taskRepository: repository);",
		"    return TaskCollectionPage(",
		"      controller: listController,",
		"      onCreateTask: () async {",
		"        final createdRecord = await _navigatorKey.currentState!.push<dynamic>(",
		"          MaterialPageRoute(builder: (context) => TaskFormPage(taskRepository: repository)),",
		"        );",
		"        if (createdRecord != null) {",
		"          await listController.refresh();",
		"        }",
		"      },",
		"      onOpenTaskDetail: (record) async {",
		"        final updatedRecord = await _navigatorKey.currentState!.push<dynamic>(",
		"          MaterialPageRoute(builder: (context) => TaskFormPage(taskRepository: repository, initialTask: record)),",
		"        );",
		"        if (updatedRecord != null) {",
		"          listController.updateRecord(updatedRecord);",
		"        }",
		"      },",
		"    );",
		"  }",
		"}",
	}, "\n") + "\n"

	normalized := normalizeBuilderRuntimeOpenLiteMainRecordFormCallbacks(workspacePath, true, content, "TaskTrackerApp")
	if !strings.Contains(normalized, "onOpenTaskDetail: (record) async {") {
		t.Fatalf("normalizeBuilderRuntimeOpenLiteMainRecordFormCallbacks() should preserve custom detail callback name from collection registry: %q", normalized)
	}
	if strings.Contains(normalized, "onOpenRecordDetail:") {
		t.Fatalf("normalizeBuilderRuntimeOpenLiteMainRecordFormCallbacks() should not rewrite custom detail callback back to onOpenRecordDetail: %q", normalized)
	}
	if !strings.Contains(normalized, "await listController.refresh();") {
		t.Fatalf("normalizeBuilderRuntimeOpenLiteMainRecordFormCallbacks() should still refresh list controller after detail edit repair: %q", normalized)
	}
}

func TestBuilderRuntimeOpenLiteNeedsCollectionCreateEntryFromWorkspaceSkipsWhenCustomOverviewViewExists(t *testing.T) {
	workspace := t.TempDir()
	files := map[string]string{
		"lib/main.dart": strings.Join([]string{
			"import 'package:flutter/material.dart';",
			"",
			"import 'views/task_overview_page.dart';",
			"",
			"void main() {",
			"  runApp(const MaterialApp(home: TaskOverviewPage()));",
			"}",
		}, "\n") + "\n",
		"lib/views/task_overview_page.dart": strings.Join([]string{
			"import 'package:flutter/material.dart';",
			"",
			"class TaskOverviewPage extends StatelessWidget {",
			"  const TaskOverviewPage({super.key});",
			"",
			"  @override",
			"  Widget build(BuildContext context) => const Placeholder();",
			"}",
		}, "\n") + "\n",
		"lib/views/record_list_page.dart": "class RecordListPage {}\n",
		"lib/views/record_form_page.dart": "class RecordFormPage {}\n",
	}
	for relPath, content := range files {
		absPath := filepath.Join(workspace, filepath.FromSlash(relPath))
		if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
			t.Fatalf("MkdirAll(%s) error = %v", relPath, err)
		}
		if err := os.WriteFile(absPath, []byte(content), 0o600); err != nil {
			t.Fatalf("WriteFile(%s) error = %v", relPath, err)
		}
	}
	if builderRuntimeOpenLiteNeedsCollectionCreateEntryFromWorkspace(workspace) {
		t.Fatal("builderRuntimeOpenLiteNeedsCollectionCreateEntryFromWorkspace() = true, want false when main.dart already wires a custom overview view")
	}
}

func TestBuilderRuntimeOpenLiteNeedsCollectionCreateEntryFromWorkspaceSkipsWhenCustomOverviewSurfaceExistsWithoutMain(t *testing.T) {
	workspace := t.TempDir()
	files := map[string]string{
		"lib/views/task_home_page.dart": strings.Join([]string{
			"import 'package:flutter/material.dart';",
			"",
			"class TaskHomePage extends StatelessWidget {",
			"  const TaskHomePage({super.key});",
			"",
			"  @override",
			"  Widget build(BuildContext context) => const Placeholder();",
			"}",
		}, "\n") + "\n",
		"lib/controllers/task_home_controller.dart":       "class TaskHomeController {}\n",
		"lib/controllers/task_collection_controller.dart": "class TaskCollectionController { Future<void> refresh() async {} }\n",
		"lib/controllers/task_form_controller.dart":       "class TaskFormController {}\n",
	}
	for relPath, content := range files {
		absPath := filepath.Join(workspace, filepath.FromSlash(relPath))
		if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
			t.Fatalf("MkdirAll(%s) error = %v", relPath, err)
		}
		if err := os.WriteFile(absPath, []byte(content), 0o600); err != nil {
			t.Fatalf("WriteFile(%s) error = %v", relPath, err)
		}
	}
	if builderRuntimeOpenLiteNeedsCollectionCreateEntryFromWorkspace(workspace) {
		t.Fatal("builderRuntimeOpenLiteNeedsCollectionCreateEntryFromWorkspace() = true, want false when custom overview surface already exists without main.dart")
	}
}

func TestBuilderRuntimeOpenLiteNeedsCollectionCreateEntryFromWorkspaceUsesCustomCollectionAndMutationControllers(t *testing.T) {
	workspace := t.TempDir()
	files := map[string]string{
		"lib/controllers/task_collection_controller.dart": strings.Join([]string{
			"import '../repositories/task_repository.dart';",
			"class TaskCollectionController {",
			"  TaskCollectionController({required this.taskRepository});",
			"  final TaskRepository taskRepository;",
			"  Future<void> refresh() async {}",
			"}",
		}, "\n") + "\n",
		"lib/controllers/task_form_controller.dart": strings.Join([]string{
			"class TaskFormController {",
			"  TaskFormController({required this.taskRepository});",
			"  final Object taskRepository;",
			"  final Object titleController = Object();",
			"  final Object noteController = Object();",
			"  final Object categoryController = Object();",
			"}",
		}, "\n") + "\n",
		"lib/repositories/task_repository.dart": "abstract class TaskRepository {}\n",
	}
	for relPath, content := range files {
		absPath := filepath.Join(workspace, filepath.FromSlash(relPath))
		if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
			t.Fatalf("MkdirAll(%s) error = %v", relPath, err)
		}
		if err := os.WriteFile(absPath, []byte(content), 0o600); err != nil {
			t.Fatalf("WriteFile(%s) error = %v", relPath, err)
		}
	}
	if !builderRuntimeOpenLiteNeedsCollectionCreateEntryFromWorkspace(workspace) {
		t.Fatal("builderRuntimeOpenLiteNeedsCollectionCreateEntryFromWorkspace() = false, want true for custom collection + mutation controller paths")
	}
}

func TestBuilderRuntimeOpenLiteNeedsCollectionCreateEntryFromWorkspaceUsesLikelyCustomCollectionControllerPath(t *testing.T) {
	workspace := t.TempDir()
	files := map[string]string{
		"lib/controllers/task_collection_controller.dart": strings.Join([]string{
			"enum TaskFilter { all, done }",
			"class TaskCollectionController {",
			"  TaskCollectionController();",
			"  TaskFilter? _selectedFilter;",
			"  Future<void> refresh() async {}",
			"}",
		}, "\n") + "\n",
		"lib/controllers/task_form_controller.dart": strings.Join([]string{
			"class TaskFormController {",
			"  TaskFormController();",
			"  final Object titleController = Object();",
			"  final Object noteController = Object();",
			"  final Object categoryController = Object();",
			"}",
		}, "\n") + "\n",
	}
	for relPath, content := range files {
		absPath := filepath.Join(workspace, filepath.FromSlash(relPath))
		if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
			t.Fatalf("MkdirAll(%s) error = %v", relPath, err)
		}
		if err := os.WriteFile(absPath, []byte(content), 0o600); err != nil {
			t.Fatalf("WriteFile(%s) error = %v", relPath, err)
		}
	}
	if !builderRuntimeOpenLiteNeedsCollectionCreateEntryFromWorkspace(workspace) {
		t.Fatal("builderRuntimeOpenLiteNeedsCollectionCreateEntryFromWorkspace() = false, want true for likely custom collection controller path without repository param")
	}
}

func TestBuilderRuntimeOpenLiteSelectedFilterTypeNameUsesLikelyCustomCollectionControllerPath(t *testing.T) {
	workspace := t.TempDir()
	files := map[string]string{
		"lib/controllers/task_collection_controller.dart": strings.Join([]string{
			"enum TaskFilter { all, done }",
			"class TaskCollectionController {",
			"  TaskCollectionController();",
			"  TaskFilter? _selectedFilter;",
			"}",
		}, "\n") + "\n",
	}
	for relPath, content := range files {
		absPath := filepath.Join(workspace, filepath.FromSlash(relPath))
		if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
			t.Fatalf("MkdirAll(%s) error = %v", relPath, err)
		}
		if err := os.WriteFile(absPath, []byte(content), 0o600); err != nil {
			t.Fatalf("WriteFile(%s) error = %v", relPath, err)
		}
	}
	if got := builderRuntimeOpenLiteSelectedFilterTypeName(workspace); got != "TaskFilter" {
		t.Fatalf("builderRuntimeOpenLiteSelectedFilterTypeName() = %q, want TaskFilter for likely custom collection controller path", got)
	}
}

func TestBuilderRuntimeOpenLiteSupportsTodoMutationFlowUsesCustomMutationControllerPath(t *testing.T) {
	workspace := t.TempDir()
	files := map[string]string{
		"lib/controllers/task_form_controller.dart": strings.Join([]string{
			"import '../models/record.dart';",
			"class TaskFormController {",
			"  TaskFormController({required this.taskRepository});",
			"  final Object taskRepository;",
			"  final Object titleController = Object();",
			"  final Object noteController = Object();",
			"  final Object categoryController = Object();",
			"  TodoItemStatus get selectedStatus => TodoItemStatus.todo;",
			"}",
		}, "\n") + "\n",
		"lib/models/record.dart": strings.Join([]string{
			"enum TodoItemStatus { todo, done }",
			"class Record {",
			"  const Record({required this.status});",
			"  final TodoItemStatus status;",
			"}",
		}, "\n") + "\n",
	}
	for relPath, content := range files {
		absPath := filepath.Join(workspace, filepath.FromSlash(relPath))
		if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
			t.Fatalf("MkdirAll(%s) error = %v", relPath, err)
		}
		if err := os.WriteFile(absPath, []byte(content), 0o600); err != nil {
			t.Fatalf("WriteFile(%s) error = %v", relPath, err)
		}
	}
	if !builderRuntimeOpenLiteSupportsTodoMutationFlow(workspace) {
		t.Fatal("builderRuntimeOpenLiteSupportsTodoMutationFlow() = false, want true for custom mutation controller path")
	}
}

func TestBuilderRuntimeOpenLiteSupportsTodoMutationFlowAllowsCustomMutationControllerPathWithoutStatus(t *testing.T) {
	workspace := t.TempDir()
	files := map[string]string{
		"lib/controllers/task_form_controller.dart": strings.Join([]string{
			"class TaskFormController {",
			"  TaskFormController({required this.taskRepository});",
			"  final Object taskRepository;",
			"  final Object titleController = Object();",
			"  final Object noteController = Object();",
			"  final Object categoryController = Object();",
			"}",
		}, "\n") + "\n",
		"lib/models/record.dart": strings.Join([]string{
			"class Record {",
			"  const Record({required this.title, required this.category, this.note = ''});",
			"  final String title;",
			"  final String category;",
			"  final String note;",
			"}",
		}, "\n") + "\n",
	}
	for relPath, content := range files {
		absPath := filepath.Join(workspace, filepath.FromSlash(relPath))
		if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
			t.Fatalf("MkdirAll(%s) error = %v", relPath, err)
		}
		if err := os.WriteFile(absPath, []byte(content), 0o600); err != nil {
			t.Fatalf("WriteFile(%s) error = %v", relPath, err)
		}
	}
	if !builderRuntimeOpenLiteSupportsTodoMutationFlow(workspace) {
		t.Fatal("builderRuntimeOpenLiteSupportsTodoMutationFlow() = false, want true for custom mutation controller path without status workflow")
	}
}

func TestExecuteBuilderRuntimeEditRestoresWorkspaceBeforeTaskOutputValidationRepair(t *testing.T) {
	workspace := filepath.Join(t.TempDir(), "workspace")
	viewPath := filepath.Join(workspace, "lib", "views", "record_list_page.dart")
	if err := os.MkdirAll(filepath.Dir(viewPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(viewPath) error = %v", err)
	}
	baseline := "import 'package:flutter/material.dart';\n\nclass RecordListPage extends StatelessWidget {\n  const RecordListPage({super.key});\n\n  @override\n  Widget build(BuildContext context) {\n    return const Text('baseline');\n  }\n}\n"
	if err := os.WriteFile(viewPath, []byte(baseline), 0o600); err != nil {
		t.Fatalf("WriteFile(viewPath) error = %v", err)
	}
	run := runRecord{
		RunID:         "run-task-output-validation-restore",
		GoalSummary:   "修复集合承载单元的 Dart 语法错误。",
		WorkspacePath: workspace,
		AllowedPaths:  []string{"lib/**"},
		TaskBundle: []appruns.TaskBundleItem{{
			TaskID:      "task-bind-collection-surface",
			TaskType:    appruns.BuilderRuntimeTaskTypeDualFileWiring,
			TargetPaths: []string{"lib/views/record_list_page.dart"},
		}},
		BuilderRuntime: &appruns.BuilderRuntimePlan{
			Enabled: true,
			TaskRoutes: []appruns.BuilderRuntimeTaskRoute{{
				TaskID:      "task-bind-collection-surface",
				TaskType:    appruns.BuilderRuntimeTaskTypeDualFileWiring,
				RouteSource: "task_route",
				Model:       appruns.BuilderRuntimeModelRef{Primary: "gemma4-26b-local", Fallbacks: []string{"gpt-4o-mini"}},
			}},
		},
		LogPath: filepath.Join(t.TempDir(), "builder-runtime-task-output-validation-restore.log"),
	}
	roundInput := appruns.RoundInput{
		RoundID:        "round-task-output-validation-restore",
		Attempt:        1,
		TaskBundle:     run.TaskBundle,
		AllowedPaths:   run.AllowedPaths,
		BuilderRuntime: run.BuilderRuntime,
	}
	originalBuilder := builderRuntimeDartFormatterCommandBuilder
	t.Cleanup(func() {
		builderRuntimeDartFormatterCommandBuilder = originalBuilder
	})
	builderRuntimeDartFormatterCommandBuilder = func(ctx context.Context, run runRecord, relPath, absPath string) (*exec.Cmd, error) {
		content, err := os.ReadFile(absPath)
		if err != nil {
			return nil, err
		}
		if strings.Contains(string(content), "CrossদেরAlignment") {
			return exec.CommandContext(ctx, "/bin/sh", "-lc", `printf 'Could not format because the source could not be parsed:\n\nline 8, column 31 of lib/views/record_list_page.dart: Illegal character '\''2503'\''.\n' >&2; exit 1`), nil
		}
		return exec.CommandContext(ctx, "/bin/sh", "-lc", "true"), nil
	}
	generator := &inspectingBuilderRuntimePatchGenerator{
		responses: []BuilderRuntimePatchResponse{
			{
				ModelAlias: "gemma4-26b-local",
				Content:    `{"patch_id":"round-restore-invalid-view","operations":[{"type":"write_file","path":"lib/views/record_list_page.dart","content":"import 'package:flutter/material.dart';\n\nclass RecordListPage extends StatelessWidget {\n  const RecordListPage({super.key});\n\n  @override\n  Widget build(BuildContext context) {\n    return Column(\n      crossAxisAlignment: CrossদেরAlignment.start,\n      children: const [Text('bad')],\n    );\n  }\n}\n"}]}`,
			},
			{
				ModelAlias: "gemma4-26b-local",
				Content:    `{"patch_id":"round-restore-repair-view","operations":[{"type":"write_file","path":"lib/views/record_list_page.dart","content":"import 'package:flutter/material.dart';\n\nclass RecordListPage extends StatelessWidget {\n  const RecordListPage({super.key});\n\n  @override\n  Widget build(BuildContext context) {\n    return Column(\n      crossAxisAlignment: CrossAxisAlignment.start,\n      children: const [Text('ok')],\n    );\n  }\n}\n"}]}`,
			},
		},
		inspect: func(request BuilderRuntimePatchRequest, index int) error {
			if index != 1 {
				return nil
			}
			content, err := os.ReadFile(viewPath)
			if err != nil {
				return err
			}
			if got := string(content); got != baseline {
				return fmt.Errorf("workspace content before repair = %q, want restored baseline", got)
			}
			if request.PreviousBody != "" {
				return fmt.Errorf("repair request PreviousBody = %q, want empty", request.PreviousBody)
			}
			return nil
		},
	}
	runner := &Runner{PatchGenerator: generator}
	step := ExecutionStep{StepID: "thin-prepare", Stage: appruns.StageThinPrepare}

	result, err := runner.executeBuilderRuntimeEdit(context.Background(), noopRunnerBackend{}, run.RunID, step, run, roundInput)
	if err != nil {
		t.Fatalf("executeBuilderRuntimeEdit() error = %v, want task-output validation repair success", err)
	}
	if result == nil || result.Stats == nil {
		t.Fatalf("result = %+v, want stats", result)
	}
	if result.Stats.Attempts != 2 {
		t.Fatalf("stats.Attempts = %d, want 2", result.Stats.Attempts)
	}
	content, err := os.ReadFile(viewPath)
	if err != nil {
		t.Fatalf("ReadFile(record_list_page.dart) error = %v", err)
	}
	if !strings.Contains(string(content), "CrossAxisAlignment.start") {
		t.Fatalf("view content = %q, want repaired Flutter identifier", string(content))
	}
}

func TestExecuteBuilderRuntimeEditRepairsSemanticConflictAfterTaskOutputValidationRepair(t *testing.T) {
	jobRoot := t.TempDir()
	workspace := filepath.Join(jobRoot, "workspace")
	preparePath := filepath.Join(jobRoot, "prepare")
	repoPath := filepath.Join(workspace, "lib", "repositories", "record_repository.dart")
	if err := os.MkdirAll(preparePath, 0o755); err != nil {
		t.Fatalf("MkdirAll(preparePath) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(preparePath, "domain-model.json"), []byte(`{"entities":[{"fields":[{"name":"project_id"},{"name":"title"},{"name":"status"},{"name":"color"}]},{"fields":[{"name":"task_id"},{"name":"project_id"},{"name":"status"},{"name":"due_on"},{"name":"note"}]},{"fields":[{"name":"tag_id"},{"name":"name"},{"name":"color"}]},{"fields":[{"name":"link_id"},{"name":"task_id"},{"name":"tag_id"}]},{"fields":[{"name":"project_id"},{"name":"open_task_count"},{"name":"done_task_count"},{"name":"tagged_task_count"}]}]}`), 0o600); err != nil {
		t.Fatalf("WriteFile(domain-model.json) error = %v", err)
	}
	writeBuilderRuntimeWorkspaceFiles(t, workspace, map[string]string{
		"lib/models/project.dart": strings.Join([]string{
			"class Project {",
			"  const Project({required this.projectId});",
			"  final String projectId;",
			"  factory Project.fromJson(Map<String, dynamic> json) => Project(projectId: json['project_id'] as String);",
			"  Map<String, dynamic> toJson() => {'project_id': projectId};",
			"}",
			"",
		}, "\n"),
		"lib/models/task.dart": strings.Join([]string{
			"enum TaskStatus { todo, doing, done }",
			"class Task {",
			"  const Task({required this.taskId, required this.projectId, required this.status});",
			"  final String taskId;",
			"  final String projectId;",
			"  final TaskStatus status;",
			"  factory Task.fromJson(Map<String, dynamic> json) => Task(taskId: json['task_id'] as String, projectId: json['project_id'] as String, status: TaskStatus.todo);",
			"  Map<String, dynamic> toJson() => {'task_id': taskId, 'project_id': projectId, 'status': status.name};",
			"}",
			"",
		}, "\n"),
		"lib/models/tag.dart": strings.Join([]string{
			"class Tag {",
			"  const Tag({required this.tagId});",
			"  final String tagId;",
			"  factory Tag.fromJson(Map<String, dynamic> json) => Tag(tagId: json['tag_id'] as String);",
			"  Map<String, dynamic> toJson() => {'tag_id': tagId};",
			"}",
			"",
		}, "\n"),
		"lib/models/task_tag_link.dart": strings.Join([]string{
			"class TaskTagLink {",
			"  const TaskTagLink({required this.linkId, required this.taskId, required this.tagId});",
			"  final String linkId;",
			"  final String taskId;",
			"  final String tagId;",
			"  factory TaskTagLink.fromJson(Map<String, dynamic> json) => TaskTagLink(linkId: json['link_id'] as String, taskId: json['task_id'] as String, tagId: json['tag_id'] as String);",
			"  Map<String, dynamic> toJson() => {'link_id': linkId, 'task_id': taskId, 'tag_id': tagId};",
			"}",
			"",
		}, "\n"),
		"lib/models/dashboard_summary.dart": strings.Join([]string{
			"class DashboardSummary {",
			"  const DashboardSummary({required this.projectId, required this.openTaskCount, required this.doneTaskCount, required this.taggedTaskCount});",
			"  final String projectId;",
			"  final int openTaskCount;",
			"  final int doneTaskCount;",
			"  final int taggedTaskCount;",
			"  factory DashboardSummary.fromJson(Map<String, dynamic> json) => DashboardSummary(projectId: json['project_id'] as String, openTaskCount: json['open_task_count'] as int, doneTaskCount: json['done_task_count'] as int, taggedTaskCount: json['tagged_task_count'] as int);",
			"  Map<String, dynamic> toJson() => {'project_id': projectId, 'open_task_count': openTaskCount, 'done_task_count': doneTaskCount, 'tagged_task_count': taggedTaskCount};",
			"}",
			"",
		}, "\n"),
		"lib/repositories/record_repository.dart": strings.Join([]string{
			"import '../models/project.dart';",
			"import '../models/task.dart';",
			"import '../models/tag.dart';",
			"import '../models/task_tag_link.dart';",
			"import '../models/dashboard_summary.dart';",
			"",
			"abstract class RecordRepository {",
			"  Future<void> init();",
			"  Future<List<Project>> loadProjects();",
			"  Future<void> saveProjects(List<Project> projects);",
			"  Future<List<Task>> loadTasks();",
			"  Future<void> saveTasks(List<Task> tasks);",
			"  Future<List<Tag>> loadTags();",
			"  Future<void> saveTags(List<Tag> tags);",
			"  Future<List<TaskTagLink>> loadTaskTagLinks();",
			"  Future<void> saveTaskTagLinks(List<TaskTagLink> links);",
			"  Future<List<DashboardSummary>> loadSummaries();",
			"  Future<void> saveSummaries(List<DashboardSummary> summaries);",
			"}",
			"",
		}, "\n"),
	})
	run := runRecord{
		RunID:         "run-task-output-semantic-repair",
		GoalSummary:   "修复关系型 repository 的 Dart 语法与语义漂移。",
		WorkspacePath: workspace,
		AllowedPaths:  []string{"lib/**"},
		TaskBundle: []appruns.TaskBundleItem{{
			TaskID:      "task-create-repository",
			TaskType:    appruns.BuilderRuntimeTaskTypeSingleFileEdit,
			TargetPaths: []string{"lib/repositories/record_repository.dart"},
		}},
		BuilderRuntime: &appruns.BuilderRuntimePlan{
			Enabled: true,
			TaskRoutes: []appruns.BuilderRuntimeTaskRoute{{
				TaskID:      "task-create-repository",
				TaskType:    appruns.BuilderRuntimeTaskTypeSingleFileEdit,
				RouteSource: "task_route",
				Model:       appruns.BuilderRuntimeModelRef{Primary: "gemma4-26b-local", Fallbacks: []string{"gpt-4o-mini"}},
			}},
		},
		LogPath: filepath.Join(t.TempDir(), "builder-runtime-task-output-semantic-repair.log"),
	}
	roundInput := appruns.RoundInput{
		RoundID:        "round-task-output-semantic-repair",
		Attempt:        1,
		TaskBundle:     run.TaskBundle,
		AllowedPaths:   run.AllowedPaths,
		BuilderRuntime: run.BuilderRuntime,
	}
	originalBuilder := builderRuntimeDartFormatterCommandBuilder
	t.Cleanup(func() {
		builderRuntimeDartFormatterCommandBuilder = originalBuilder
	})
	builderRuntimeDartFormatterCommandBuilder = func(ctx context.Context, run runRecord, relPath, absPath string) (*exec.Cmd, error) {
		content, err := os.ReadFile(absPath)
		if err != nil {
			return nil, err
		}
		if strings.Contains(string(content), "load..'links'") {
			return exec.CommandContext(ctx, "/bin/sh", "-lc", `printf 'Could not format because the source could not be parsed:\n\nline 18, column 31 of lib/repositories/record_repository.dart: Expected an identifier.\n' >&2; exit 1`), nil
		}
		return exec.CommandContext(ctx, "/bin/sh", "-lc", "true"), nil
	}
	runner := &Runner{PatchGenerator: &stubBuilderRuntimePatchGenerator{responses: []BuilderRuntimePatchResponse{
		{
			ModelAlias: "gemma4-26b-local",
			Content:    `{"patch_id":"repo-invalid-syntax","operations":[{"type":"write_file","path":"lib/repositories/record_repository.dart","content":"import '../models/project.dart';\nimport '../models/task.dart';\nimport '../models/tag.dart';\nimport '../models/task_tag_link.dart';\nimport '../models/dashboard_summary.dart';\n\nabstract class RecordRepository {\n  Future<void> init();\n  Future<List<TaskTagLink>> loadTaskTagLinks();\n  Future<void> saveTaskTagLinks(List<TaskTagLink> links);\n\n  Future<void> addTaskTagLink(TaskTagLink link) async {\n    final links = await load..'links' = await loadTaskTagLinks();\n    links.add(link);\n    await saveTaskTagLinks(links);\n  }\n}\n"}]}`,
		},
		{
			ModelAlias: "gemma4-26b-local",
			Content:    `{"patch_id":"repo-semantic-conflict","operations":[{"type":"write_file","path":"lib/repositories/record_repository.dart","content":"import '../models/project.dart';\nimport '../models/task.dart';\nimport '../models/tag.dart';\nimport '../models/task_tag_link.dart';\nimport '../models/dashboard_summary.dart';\n\nabstract class RecordRepository {\n  Future<void> init();\n  Future<List<DashboardSummary>> loadSummaries();\n  Future<void> saveSummaries(List<DashboardSummary> summaries);\n  Future<DashboardSummary?> loadDashboardSummary(String projectId);\n}\n\nclass InMemoryRecordRepository implements RecordRepository {\n  @override\n  Future<void> init() async {}\n\n  @override\n  Future<List<DashboardSummary>> loadSummaries() async => const [];\n\n  @override\n  Future<void> saveSummaries(List<DashboardSummary> summaries) async {}\n\n  @override\n  Future<DashboardSummary?> loadDashboardSummary(String projectId) async {\n    const doneCount = 1;\n    return DashboardSummary(projectId: projectId, openTaskCount: 0, doneTaskCount: doneCount, taggedTaskCount: 0);\n  }\n}\n"}]}`,
		},
		{
			ModelAlias: "gemma4-26b-local",
			Content:    `{"patch_id":"repo-semantic-repair","operations":[{"type":"write_file","path":"lib/repositories/record_repository.dart","content":"import '../models/project.dart';\nimport '../models/task.dart';\nimport '../models/tag.dart';\nimport '../models/task_tag_link.dart';\nimport '../models/dashboard_summary.dart';\n\nabstract class RecordRepository {\n  Future<void> init();\n  Future<List<Project>> loadProjects();\n  Future<void> saveProjects(List<Project> projects);\n  Future<List<Task>> loadTasks();\n  Future<void> saveTasks(List<Task> tasks);\n  Future<List<Tag>> loadTags();\n  Future<void> saveTags(List<Tag> tags);\n  Future<List<TaskTagLink>> loadTaskTagLinks();\n  Future<void> saveTaskTagLinks(List<TaskTagLink> links);\n  Future<List<DashboardSummary>> loadSummaries();\n  Future<void> saveSummaries(List<DashboardSummary> summaries);\n}\n"}]}`,
		},
	}}}
	step := ExecutionStep{StepID: "thin-prepare", Stage: appruns.StageThinPrepare}

	result, err := runner.executeBuilderRuntimeEdit(context.Background(), noopRunnerBackend{}, run.RunID, step, run, roundInput)
	if err != nil {
		t.Fatalf("executeBuilderRuntimeEdit() error = %v, want task-output semantic repair success", err)
	}
	if result == nil || result.Stats == nil {
		t.Fatalf("result = %+v, want stats", result)
	}
	if result.Stats.Attempts != 1 {
		t.Fatalf("stats.Attempts = %d, want 1", result.Stats.Attempts)
	}
	generator, ok := runner.PatchGenerator.(*stubBuilderRuntimePatchGenerator)
	if !ok {
		t.Fatalf("PatchGenerator = %T, want *stubBuilderRuntimePatchGenerator", runner.PatchGenerator)
	}
	if len(generator.requests) != 1 {
		t.Fatalf("len(generator.requests) = %d, want 1", len(generator.requests))
	}
	content, err := os.ReadFile(repoPath)
	if err != nil {
		t.Fatalf("ReadFile(record_repository.dart) error = %v", err)
	}
	for _, unwanted := range []string{"@toOverride", "Dashboard<dynamic>", "loadDashboardSummary(", "load..'links'", "doneCount"} {
		if strings.Contains(string(content), unwanted) {
			t.Fatalf("repository content should not contain %q after canonicalization: %q", unwanted, string(content))
		}
	}
	for _, want := range []string{"class HiveRecordRepository extends RecordRepository", "class InMemoryRecordRepository extends RecordRepository", "Future<List<Project>> loadProjects();", "Future<void> addTaskTagLink(TaskTagLink link) async"} {
		if !strings.Contains(string(content), want) {
			t.Fatalf("repository content missing %q after canonicalization: %q", want, string(content))
		}
	}
}

func TestExecuteBuilderRuntimeEditAllowsAnalyzeRepairModelContextEditsOutsideDirectFailures(t *testing.T) {
	jobRoot := t.TempDir()
	workspace := filepath.Join(jobRoot, "workspace")
	homePagePath := filepath.Join(workspace, "lib", "views", "home_page.dart")
	widgetTestPath := filepath.Join(workspace, "test", "widget_test.dart")
	summaryPath := filepath.Join(workspace, "lib", "models", "summary.dart")
	for _, filePath := range []string{homePagePath, widgetTestPath, summaryPath} {
		if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
			t.Fatalf("MkdirAll(%s) error = %v", filePath, err)
		}
	}
	if err := os.WriteFile(homePagePath, []byte("class HomePage {}\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(home page) error = %v", err)
	}
	if err := os.WriteFile(widgetTestPath, []byte("void main() {}\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(widget test) error = %v", err)
	}
	if err := os.WriteFile(summaryPath, []byte("class Summary {\n  final double income;\n  const Summary(this.income);\n}\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(summary) error = %v", err)
	}
	run := runRecord{
		RunID:         "run-analyze-repair-model-context-scope",
		GoalSummary:   "修复 analyze repair 并允许联动模型文件。",
		WorkspacePath: workspace,
		AllowedPaths:  []string{"lib/**", "test/**"},
		RoundState:    &appruns.RoundState{CurrentTaskID: "repair-check-flutter-analyze"},
		TaskBundle: []appruns.TaskBundleItem{
			{
				TaskID:      "repair-check-flutter-analyze",
				TaskType:    appruns.BuilderRuntimeTaskTypeAnalyzeRepair,
				TargetPaths: []string{"lib/views/home_page.dart", "test/widget_test.dart"},
			},
			{
				TaskID:      "task-summary-model",
				TaskType:    appruns.BuilderRuntimeTaskTypeDualFileWiring,
				TargetPaths: []string{"lib/models/summary.dart"},
			},
		},
		BuilderRuntime: &appruns.BuilderRuntimePlan{
			Enabled: true,
			TaskRoutes: []appruns.BuilderRuntimeTaskRoute{{
				TaskID:      "repair-check-flutter-analyze",
				TaskType:    appruns.BuilderRuntimeTaskTypeAnalyzeRepair,
				RouteSource: "task_route",
				Model:       appruns.BuilderRuntimeModelRef{Primary: "gemma4-26b-local", Fallbacks: []string{"gpt-4o-mini"}},
			}},
		},
		LogPath: filepath.Join(t.TempDir(), "builder-runtime-model-context-scope.log"),
	}
	roundInput := appruns.RoundInput{
		RoundID:        "round-analyze-repair-model-context-scope",
		Attempt:        1,
		TaskBundle:     run.TaskBundle,
		AllowedPaths:   run.AllowedPaths,
		BuilderRuntime: run.BuilderRuntime,
	}
	runner := &Runner{PatchGenerator: &stubBuilderRuntimePatchGenerator{responses: []BuilderRuntimePatchResponse{{
		ModelAlias: "gemma4-26b-local",
		Content:    `{"patch_id":"round-analyze-repair-model-context-scope-1","operations":[{"type":"write_file","path":"lib/views/home_page.dart","content":"class HomePage {\n  const HomePage();\n}\n"},{"type":"write_file","path":"test/widget_test.dart","content":"void main() {\n  // repaired analyze target\n}\n"},{"type":"write_file","path":"lib/models/summary.dart","content":"class Summary {\n  final double incomeTotal;\n  const Summary(this.incomeTotal);\n}\n"}]}`,
	}}}}
	step := ExecutionStep{StepID: "check-flutter-analyze", Stage: appruns.StageCheap}

	result, err := runner.executeBuilderRuntimeEditWithFailureContext(context.Background(), noopRunnerBackend{}, run.RunID, step, run, roundInput, "flutter analyze failed in lib/views/home_page.dart and test/widget_test.dart while summary model fields drifted")
	if err != nil {
		t.Fatalf("executeBuilderRuntimeEditWithFailureContext() error = %v, want analyze repair to allow model-context edit", err)
	}
	if result == nil || result.Stats == nil {
		t.Fatalf("result = %+v, want stats", result)
	}
	if result.Stats.Attempts != 1 {
		t.Fatalf("stats.Attempts = %d, want 1", result.Stats.Attempts)
	}
	for _, check := range []struct {
		path   string
		needle string
	}{
		{homePagePath, "const HomePage"},
		{widgetTestPath, "repaired analyze target"},
		{summaryPath, "incomeTotal"},
	} {
		content, readErr := os.ReadFile(check.path)
		if readErr != nil {
			t.Fatalf("ReadFile(%s) error = %v", check.path, readErr)
		}
		if !strings.Contains(string(content), check.needle) {
			t.Fatalf("content(%s) = %q, want %q", check.path, string(content), check.needle)
		}
	}
}

func TestExecuteBuilderRuntimeEditRepairsPatchThatRemovesFailureReferencedHelper(t *testing.T) {
	jobRoot := t.TempDir()
	workspace := filepath.Join(jobRoot, "workspace")
	repoPath := filepath.Join(workspace, "lib", "repositories", "record_repository.dart")
	formPath := filepath.Join(workspace, "lib", "views", "record_form_page.dart")
	for _, filePath := range []string{repoPath, formPath} {
		if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
			t.Fatalf("MkdirAll(%s) error = %v", filePath, err)
		}
	}
	if err := os.WriteFile(repoPath, []byte(strings.Join([]string{
		"abstract class RecordRepository {}",
		"",
		"class InMemoryRecordRepository implements RecordRepository {}",
		"",
	}, "\n")), 0o600); err != nil {
		t.Fatalf("WriteFile(repo) error = %v", err)
	}
	baselineForm := strings.Join([]string{
		"import 'package:flutter/material.dart';",
		"",
		"import '../repositories/record_repository.dart';",
		"",
		"class RecordFormPage extends StatelessWidget {",
		"  const RecordFormPage({super.key, required this.recordRepository});",
		"",
		"  final RecordRepository recordRepository;",
		"",
		"  @override",
		"  Widget build(BuildContext context) => const SizedBox.shrink();",
		"}",
	}, "\n") + "\n"
	if err := os.WriteFile(formPath, []byte(baselineForm), 0o600); err != nil {
		t.Fatalf("WriteFile(form) error = %v", err)
	}
	run := runRecord{
		RunID:         "run-direct-failure-helper-preserve",
		GoalSummary:   "修复 analyze repair 并保留当前 view helper。",
		WorkspacePath: workspace,
		AllowedPaths:  []string{"lib/**"},
		TaskBundle: []appruns.TaskBundleItem{{
			TaskID:      "repair-check-flutter-analyze",
			TaskType:    appruns.BuilderRuntimeTaskTypeAnalyzeRepair,
			TargetPaths: []string{"lib/views/record_form_page.dart"},
		}},
		BuilderRuntime: &appruns.BuilderRuntimePlan{
			Enabled: true,
			TaskRoutes: []appruns.BuilderRuntimeTaskRoute{{
				TaskID:      "repair-check-flutter-analyze",
				TaskType:    appruns.BuilderRuntimeTaskTypeAnalyzeRepair,
				RouteSource: "task_route",
				Model:       appruns.BuilderRuntimeModelRef{Primary: "gemma4-26b-local", Fallbacks: []string{"gpt-4o-mini"}},
			}},
		},
		LogPath: filepath.Join(t.TempDir(), "builder-runtime-helper-preserve.log"),
	}
	roundInput := appruns.RoundInput{
		RoundID:        "round-analyze-repair",
		Attempt:        2,
		TaskBundle:     run.TaskBundle,
		AllowedPaths:   run.AllowedPaths,
		BuilderRuntime: run.BuilderRuntime,
	}
	runner := &Runner{PatchGenerator: &stubBuilderRuntimePatchGenerator{responses: []BuilderRuntimePatchResponse{
		{
			ModelAlias: "gemma4-26b-local",
			Content:    `{"patch_id":"round-analyze-repair-1","operations":[{"type":"write_file","path":"lib/views/record_form_page.dart","content":"import 'package:flutter/material.dart';\n\nclass RecordFormPage extends StatelessWidget {\n  const RecordFormPage({super.key});\n\n  @override\n  Widget build(BuildContext context) => const SizedBox.shrink();\n}\n"}]}`,
		},
		{
			ModelAlias: "gemma4-26b-local",
			Content:    `{"patch_id":"round-analyze-repair-2","operations":[{"type":"write_file","path":"lib/views/record_form_page.dart","content":"import 'package:flutter/material.dart';\n\nimport '../repositories/record_repository.dart';\n\nclass RecordFormPage extends StatelessWidget {\n  const RecordFormPage({super.key, required this.recordRepository});\n\n  final RecordRepository recordRepository;\n\n  @override\n  Widget build(BuildContext context) => const SizedBox.shrink();\n}\n"}]}`,
		},
	}}}
	step := ExecutionStep{StepID: "check-flutter-analyze", Stage: appruns.StageCheap}

	result, err := runner.executeBuilderRuntimeEditWithFailureContext(context.Background(), noopRunnerBackend{}, run.RunID, step, run, roundInput, "flutter analyze failed in lib/views/record_form_page.dart\n  error • The getter 'recordRepository' isn't defined for the type 'RecordFormPage' • lib/views/record_form_page.dart:8:20 • undefined_getter")
	if err != nil {
		t.Fatalf("executeBuilderRuntimeEditWithFailureContext() error = %v, want helper-preservation semantic repair success", err)
	}
	if result == nil || result.Stats == nil {
		t.Fatalf("result = %+v, want stats", result)
	}
	if result.Stats.Attempts != 2 {
		t.Fatalf("stats.Attempts = %d, want 2", result.Stats.Attempts)
	}
	generator, ok := runner.PatchGenerator.(*stubBuilderRuntimePatchGenerator)
	if !ok {
		t.Fatalf("PatchGenerator = %T, want *stubBuilderRuntimePatchGenerator", runner.PatchGenerator)
	}
	if len(generator.requests) != 2 {
		t.Fatalf("len(generator.requests) = %d, want 2", len(generator.requests))
	}
	if !generator.requests[1].RepairOnly {
		t.Fatalf("semantic repair request RepairOnly = false, want true")
	}
	if !strings.Contains(generator.requests[1].PreviousErr, "patch removed required helper symbols referenced by current validation failures") {
		t.Fatalf("semantic repair PreviousErr = %q, want helper-preservation semantic conflict", generator.requests[1].PreviousErr)
	}
	content, err := os.ReadFile(formPath)
	if err != nil {
		t.Fatalf("ReadFile(form) error = %v", err)
	}
	if !strings.Contains(string(content), "required this.recordRepository") {
		t.Fatalf("form content = %q, want repaired recordRepository parameter", string(content))
	}
	if !strings.Contains(string(content), "final RecordRepository recordRepository;") {
		t.Fatalf("form content = %q, want preserved recordRepository field", string(content))
	}
}

func TestExecuteBuilderRuntimeEditRepairsUndefinedHelperSymbolsViaOwnerFileSemanticRepair(t *testing.T) {
	jobRoot := t.TempDir()
	workspace := filepath.Join(jobRoot, "workspace")
	controllerPath := filepath.Join(workspace, "lib", "controllers", "record_form_controller.dart")
	formPath := filepath.Join(workspace, "lib", "views", "record_form_page.dart")
	for _, filePath := range []string{controllerPath, formPath} {
		if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
			t.Fatalf("MkdirAll(%s) error = %v", filePath, err)
		}
	}
	if err := os.WriteFile(controllerPath, []byte(strings.Join([]string{
		"import 'package:flutter/material.dart';",
		"",
		"class RecordFormController extends ChangeNotifier {",
		"  final TextEditingController _weightController = TextEditingController();",
		"  final TextEditingController _noteController = TextEditingController();",
		"",
		"  @override",
		"  void dispose() {",
		"    _weightController.dispose();",
		"    _noteController.dispose();",
		"    super.dispose();",
		"  }",
		"}",
	}, "\n")+"\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(controller) error = %v", err)
	}
	if err := os.WriteFile(formPath, []byte(strings.Join([]string{
		"import 'package:flutter/material.dart';",
		"",
		"class RecordFormPage extends StatelessWidget {",
		"  const RecordFormPage({super.key});",
		"",
		"  @override",
		"  Widget build(BuildContext context) => const SizedBox.shrink();",
		"}",
	}, "\n")+"\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(form) error = %v", err)
	}
	run := runRecord{
		RunID:         "run-analyze-repair-owner-file-semantic-repair",
		GoalSummary:   "修复 analyze repair 的 helper owner 文件漂移。",
		WorkspacePath: workspace,
		AllowedPaths:  []string{"lib/**"},
		TaskBundle: []appruns.TaskBundleItem{{
			TaskID:      "repair-check-flutter-analyze",
			TaskType:    appruns.BuilderRuntimeTaskTypeAnalyzeRepair,
			TargetPaths: []string{"lib/views/record_form_page.dart"},
		}},
		BuilderRuntime: &appruns.BuilderRuntimePlan{
			Enabled: true,
			TaskRoutes: []appruns.BuilderRuntimeTaskRoute{{
				TaskID:      "repair-check-flutter-analyze",
				TaskType:    appruns.BuilderRuntimeTaskTypeAnalyzeRepair,
				RouteSource: "task_route",
				Model:       appruns.BuilderRuntimeModelRef{Primary: "gemma4-26b-local", Fallbacks: []string{"gpt-4o-mini"}},
			}},
		},
		LogPath: filepath.Join(t.TempDir(), "builder-runtime-owner-file-semantic-repair.log"),
	}
	roundInput := appruns.RoundInput{
		RoundID:        "round-analyze-repair-owner-file-semantic-repair",
		Attempt:        2,
		TaskBundle:     run.TaskBundle,
		AllowedPaths:   run.AllowedPaths,
		BuilderRuntime: run.BuilderRuntime,
	}
	runner := &Runner{PatchGenerator: &stubBuilderRuntimePatchGenerator{responses: []BuilderRuntimePatchResponse{
		{
			ModelAlias: "gemma4-26b-local",
			Content:    `{"patch_id":"round-analyze-owner-invalid-page","operations":[{"type":"write_file","path":"lib/views/record_form_page.dart","content":"import 'package:flutter/material.dart';\n\nimport '../controllers/record_form_controller.dart';\n\nclass RecordFormPage extends StatelessWidget {\n  const RecordFormPage({super.key, required this.controller});\n\n  final RecordFormController controller;\n\n  @override\n  Widget build(BuildContext context) {\n    return Column(\n      children: [\n        TextField(controller: controller.weightController),\n        TextField(controller: controller.noteController),\n      ],\n    );\n  }\n}\n"}]}`,
		},
		{
			ModelAlias: "gemma4-26b-local",
			Content:    `{"patch_id":"round-analyze-owner-controller-repair","operations":[{"type":"write_file","path":"lib/controllers/record_form_controller.dart","content":"import 'package:flutter/material.dart';\n\nclass RecordFormController extends ChangeNotifier {\n  final TextEditingController _weightController = TextEditingController();\n  final TextEditingController _noteController = TextEditingController();\n\n  TextEditingController get weightController => _weightController;\n  TextEditingController get noteController => _noteController;\n\n  @override\n  void dispose() {\n    _weightController.dispose();\n    _noteController.dispose();\n    super.dispose();\n  }\n}\n"}]}`,
		},
	}}}
	step := ExecutionStep{StepID: "check-flutter-analyze", Stage: appruns.StageCheap}

	result, err := runner.executeBuilderRuntimeEditWithFailureContext(
		context.Background(),
		noopRunnerBackend{},
		run.RunID,
		step,
		run,
		roundInput,
		strings.Join([]string{
			"check=check-flutter-analyze",
			"error • The getter 'weightController' isn't defined for the type 'RecordFormController' • lib/views/record_form_page.dart:11:42 • undefined_getter",
			"error • The getter 'noteController' isn't defined for the type 'RecordFormController' • lib/views/record_form_page.dart:12:42 • undefined_getter",
		}, "\n"),
	)
	if err != nil {
		t.Fatalf("executeBuilderRuntimeEditWithFailureContext() error = %v, want owner-file semantic repair success", err)
	}
	if result == nil || result.Stats == nil {
		t.Fatalf("result = %+v, want stats", result)
	}
	if result.Stats.Attempts != 2 {
		t.Fatalf("stats.Attempts = %d, want 2", result.Stats.Attempts)
	}
	generator, ok := runner.PatchGenerator.(*stubBuilderRuntimePatchGenerator)
	if !ok {
		t.Fatalf("PatchGenerator = %T, want *stubBuilderRuntimePatchGenerator", runner.PatchGenerator)
	}
	if len(generator.requests) != 2 {
		t.Fatalf("len(generator.requests) = %d, want 2", len(generator.requests))
	}
	if !generator.requests[1].RepairOnly {
		t.Fatalf("semantic repair request RepairOnly = false, want true")
	}
	if !strings.Contains(generator.requests[1].PreviousErr, "lib/controllers/record_form_controller.dart -> noteController") {
		t.Fatalf("semantic repair PreviousErr = %q, want owner file path for noteController", generator.requests[1].PreviousErr)
	}
	if !strings.Contains(generator.requests[1].PreviousErr, "lib/controllers/record_form_controller.dart -> weightController") {
		t.Fatalf("semantic repair PreviousErr = %q, want owner file path for weightController", generator.requests[1].PreviousErr)
	}
	if got := generator.requests[1].RoundInput.TaskBundle[0].TargetPaths; len(got) != 1 || got[0] != "lib/controllers/record_form_controller.dart" {
		t.Fatalf("semantic repair target_paths = %v, want [lib/controllers/record_form_controller.dart]", got)
	}
	controllerContent, err := os.ReadFile(controllerPath)
	if err != nil {
		t.Fatalf("ReadFile(controller) error = %v", err)
	}
	if !strings.Contains(string(controllerContent), "TextEditingController get weightController => _weightController;") {
		t.Fatalf("controller content = %q, want public weightController getter", string(controllerContent))
	}
	if !strings.Contains(string(controllerContent), "TextEditingController get noteController => _noteController;") {
		t.Fatalf("controller content = %q, want public noteController getter", string(controllerContent))
	}
	formContent, err := os.ReadFile(formPath)
	if err != nil {
		t.Fatalf("ReadFile(form) error = %v", err)
	}
	if !strings.Contains(string(formContent), "controller.weightController") || !strings.Contains(string(formContent), "controller.noteController") {
		t.Fatalf("form content = %q, want final page to keep repaired controller accessors", string(formContent))
	}
}

func TestExecuteBuilderRuntimeEditRejectsPartialTargetCoverageForNonRepairTask(t *testing.T) {
	jobRoot := t.TempDir()
	workspace := filepath.Join(jobRoot, "workspace")
	mainPath := filepath.Join(workspace, "lib", "main.dart")
	homePath := filepath.Join(workspace, "lib", "views", "home_page.dart")
	for _, filePath := range []string{mainPath, homePath} {
		if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
			t.Fatalf("MkdirAll(%s) error = %v", filePath, err)
		}
	}
	if err := os.WriteFile(mainPath, []byte("void main() {}\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(main) error = %v", err)
	}
	if err := os.WriteFile(homePath, []byte("class HomePage {}\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(home) error = %v", err)
	}
	run := runRecord{
		RunID:         "run-partial-non-repair-coverage",
		GoalSummary:   "搭建首页骨架。",
		WorkspacePath: workspace,
		AllowedPaths:  []string{"lib/**"},
		RoundState:    &appruns.RoundState{CurrentTaskID: "task-screen-scaffold"},
		TaskBundle: []appruns.TaskBundleItem{{
			TaskID:      "task-screen-scaffold",
			TaskType:    appruns.BuilderRuntimeTaskTypeDualFileWiring,
			TargetPaths: []string{"lib/main.dart", "lib/views/home_page.dart"},
		}},
		BuilderRuntime: &appruns.BuilderRuntimePlan{
			Enabled: true,
			TaskRoutes: []appruns.BuilderRuntimeTaskRoute{{
				TaskID:      "task-screen-scaffold",
				TaskType:    appruns.BuilderRuntimeTaskTypeDualFileWiring,
				RouteSource: "task_route",
				Model:       appruns.BuilderRuntimeModelRef{Primary: "gemma4-26b-local"},
			}},
		},
		LogPath: filepath.Join(t.TempDir(), "builder-runtime-partial-non-repair.log"),
	}
	roundInput := appruns.RoundInput{
		RoundID:        "round-partial-non-repair",
		Attempt:        1,
		TaskBundle:     run.TaskBundle,
		AllowedPaths:   run.AllowedPaths,
		BuilderRuntime: run.BuilderRuntime,
	}
	runner := &Runner{PatchGenerator: &stubBuilderRuntimePatchGenerator{responses: []BuilderRuntimePatchResponse{{
		ModelAlias: "gemma4-26b-local",
		Content:    `{"patch_id":"patch-partial-non-repair","operations":[{"type":"write_file","path":"lib/main.dart","content":"void main() {\n  // partial scaffold\n}\n"}]}`,
	}}}}
	step := ExecutionStep{StepID: "thin-prepare", Stage: appruns.StageThinPrepare}

	result, err := runner.executeBuilderRuntimeEdit(context.Background(), noopRunnerBackend{}, run.RunID, step, run, roundInput)
	if err == nil {
		t.Fatalf("executeBuilderRuntimeEdit() error = nil, want task target coverage rejection")
	}
	var execErr *builderRuntimeExecutionError
	if !errors.As(err, &execErr) {
		t.Fatalf("error = %T, want *builderRuntimeExecutionError", err)
	}
	if execErr.signature != "builder_runtime_task_output_invalid" {
		t.Fatalf("signature = %q, want builder_runtime_task_output_invalid", execErr.signature)
	}
	if result == nil || result.Stats == nil || result.Stats.Attempts < 1 {
		t.Fatalf("result stats = %+v, want at least one rejected attempt", result)
	}
	if !strings.Contains(err.Error(), "lib/views/home_page.dart") {
		t.Fatalf("error = %q, want missing non-repair target path", err)
	}
}

func TestExecuteBuilderRuntimeEditRepairsPartialTargetCoverageForNonRepairTask(t *testing.T) {
	jobRoot := t.TempDir()
	workspace := filepath.Join(jobRoot, "workspace")
	mainPath := filepath.Join(workspace, "lib", "main.dart")
	homePath := filepath.Join(workspace, "lib", "views", "home_page.dart")
	formPath := filepath.Join(workspace, "lib", "views", "entry_form_page.dart")
	listPath := filepath.Join(workspace, "lib", "views", "entry_list_page.dart")
	for _, filePath := range []string{mainPath, homePath, formPath, listPath} {
		if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
			t.Fatalf("MkdirAll(%s) error = %v", filePath, err)
		}
	}
	for path, content := range map[string]string{
		mainPath: "void main() {}\n",
		homePath: "class HomePage {}\n",
		formPath: "class EntryFormPage {}\n",
		listPath: "class EntryListPage {}\n",
	} {
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatalf("WriteFile(%s) error = %v", path, err)
		}
	}
	run := runRecord{
		RunID:         "run-partial-non-repair-coverage-repair",
		GoalSummary:   "搭建首页与表单列表骨架。",
		WorkspacePath: workspace,
		AllowedPaths:  []string{"lib/**"},
		RoundState:    &appruns.RoundState{CurrentTaskID: "task-screen-scaffold"},
		TaskBundle: []appruns.TaskBundleItem{{
			TaskID:      "task-screen-scaffold",
			TaskType:    appruns.BuilderRuntimeTaskTypeDualFileWiring,
			TargetPaths: []string{"lib/main.dart", "lib/views/entry_form_page.dart", "lib/views/entry_list_page.dart", "lib/views/home_page.dart"},
		}},
		BuilderRuntime: &appruns.BuilderRuntimePlan{
			Enabled: true,
			TaskRoutes: []appruns.BuilderRuntimeTaskRoute{{
				TaskID:      "task-screen-scaffold",
				TaskType:    appruns.BuilderRuntimeTaskTypeDualFileWiring,
				RouteSource: "task_route",
				Model:       appruns.BuilderRuntimeModelRef{Primary: "gemma4-26b-local", Fallbacks: []string{"gpt-4o-mini"}},
			}},
		},
		LogPath: filepath.Join(t.TempDir(), "builder-runtime-partial-non-repair-repair.log"),
	}
	roundInput := appruns.RoundInput{
		RoundID:        "round-partial-non-repair-repair",
		Attempt:        1,
		TaskBundle:     run.TaskBundle,
		AllowedPaths:   run.AllowedPaths,
		BuilderRuntime: run.BuilderRuntime,
	}
	runner := &Runner{PatchGenerator: &stubBuilderRuntimePatchGenerator{responses: []BuilderRuntimePatchResponse{
		{
			ModelAlias: "gemma4-26b-local",
			Content:    `{"patch_id":"patch-partial-non-repair-1","operations":[{"type":"write_file","path":"lib/main.dart","content":"import 'package:flutter/material.dart';\n\nimport 'views/home_page.dart';\n\nvoid main() {\n  runApp(MaterialApp(home: HomePage()));\n}\n"},{"type":"write_file","path":"lib/views/home_page.dart","content":"class HomePage {\n  const HomePage();\n}\n"}]}`,
		},
		{
			ModelAlias: "gemma4-26b-local",
			Content:    `{"patch_id":"patch-partial-non-repair-2","operations":[{"type":"write_file","path":"lib/views/entry_form_page.dart","content":"class EntryFormPage {\n  const EntryFormPage();\n}\n"},{"type":"write_file","path":"lib/views/entry_list_page.dart","content":"class EntryListPage {\n  const EntryListPage();\n}\n"}]}`,
		},
	}}}
	step := ExecutionStep{StepID: "thin-prepare", Stage: appruns.StageThinPrepare}

	result, err := runner.executeBuilderRuntimeEdit(context.Background(), noopRunnerBackend{}, run.RunID, step, run, roundInput)
	if err != nil {
		t.Fatalf("executeBuilderRuntimeEdit() error = %v, want task target coverage repair success", err)
	}
	if result == nil || result.Stats == nil {
		t.Fatalf("result = %+v, want stats", result)
	}
	if result.Stats.Attempts != 2 {
		t.Fatalf("stats.Attempts = %d, want 2", result.Stats.Attempts)
	}
	generator, ok := runner.PatchGenerator.(*stubBuilderRuntimePatchGenerator)
	if !ok {
		t.Fatalf("PatchGenerator = %T, want *stubBuilderRuntimePatchGenerator", runner.PatchGenerator)
	}
	if len(generator.requests) != 2 {
		t.Fatalf("len(generator.requests) = %d, want 2", len(generator.requests))
	}
	if !generator.requests[1].RepairOnly {
		t.Fatalf("repair request RepairOnly = false, want true")
	}
	if !strings.Contains(generator.requests[1].PreviousErr, "patch did not touch every current task target path") {
		t.Fatalf("repair request PreviousErr = %q, want task target coverage violation", generator.requests[1].PreviousErr)
	}
	if len(generator.requests[1].RoundInput.TaskBundle) != 1 || len(generator.requests[1].RoundInput.TaskBundle[0].TargetPaths) != 2 {
		t.Fatalf("repair target_paths = %#v, want narrowed missing-file slice", generator.requests[1].RoundInput.TaskBundle)
	}
	if generator.requests[1].RoundInput.TaskBundle[0].TargetPaths[0] != "lib/views/entry_form_page.dart" || generator.requests[1].RoundInput.TaskBundle[0].TargetPaths[1] != "lib/views/entry_list_page.dart" {
		t.Fatalf("repair target_paths = %v, want [lib/views/entry_form_page.dart lib/views/entry_list_page.dart]", generator.requests[1].RoundInput.TaskBundle[0].TargetPaths)
	}
	for _, check := range []struct {
		path   string
		needle string
	}{
		{mainPath, "HomePage()"},
		{homePath, "const HomePage"},
		{formPath, "const EntryFormPage"},
		{listPath, "const EntryListPage"},
	} {
		content, readErr := os.ReadFile(check.path)
		if readErr != nil {
			t.Fatalf("ReadFile(%s) error = %v", check.path, readErr)
		}
		if !strings.Contains(string(content), check.needle) {
			t.Fatalf("content(%s) = %q, want %q", check.path, string(content), check.needle)
		}
	}
}

func TestExecuteBuilderRuntimeEditRepairsSemanticConflictBeforeNonRepairCoverageRepair(t *testing.T) {
	jobRoot := t.TempDir()
	workspace := filepath.Join(jobRoot, "workspace")
	mainPath := filepath.Join(workspace, "lib", "main.dart")
	homePath := filepath.Join(workspace, "lib", "views", "home_page.dart")
	formPath := filepath.Join(workspace, "lib", "views", "entry_form_page.dart")
	listPath := filepath.Join(workspace, "lib", "views", "entry_list_page.dart")
	entryModelPath := filepath.Join(workspace, "lib", "models", "entry.dart")
	summaryModelPath := filepath.Join(workspace, "lib", "models", "summary.dart")
	for _, filePath := range []string{mainPath, homePath, formPath, listPath, entryModelPath, summaryModelPath} {
		if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
			t.Fatalf("MkdirAll(%s) error = %v", filePath, err)
		}
	}
	for path, content := range map[string]string{
		mainPath:         "void main() {}\n",
		homePath:         "class HomePage {}\n",
		formPath:         "class EntryFormPage {}\n",
		listPath:         "class EntryListPage {}\n",
		entryModelPath:   "class BookkeepingEntry {\n  final DateTime occurredOn;\n  final String? note;\n  final String category;\n  final bool isExpense;\n  final double amount;\n  const BookkeepingEntry({required this.occurredOn, this.note, required this.category, required this.isExpense, required this.amount});\n}\n",
		summaryModelPath: "class Summary {\n  final double balance;\n  final double incomeTotal;\n  final double expenseTotal;\n  const Summary({required this.balance, required this.incomeTotal, required this.expenseTotal});\n}\n",
	} {
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatalf("WriteFile(%s) error = %v", path, err)
		}
	}
	run := runRecord{
		RunID:         "run-non-repair-semantic-repair",
		GoalSummary:   "搭建首页与表单列表骨架。",
		WorkspacePath: workspace,
		AllowedPaths:  []string{"lib/**"},
		RoundState:    &appruns.RoundState{CurrentTaskID: "task-screen-scaffold"},
		TaskBundle: []appruns.TaskBundleItem{{
			TaskID:      "task-screen-scaffold",
			TaskType:    appruns.BuilderRuntimeTaskTypeDualFileWiring,
			TargetPaths: []string{"lib/main.dart", "lib/views/entry_form_page.dart", "lib/views/entry_list_page.dart", "lib/views/home_page.dart"},
		}},
		BuilderRuntime: &appruns.BuilderRuntimePlan{
			Enabled: true,
			TaskRoutes: []appruns.BuilderRuntimeTaskRoute{{
				TaskID:      "task-screen-scaffold",
				TaskType:    appruns.BuilderRuntimeTaskTypeDualFileWiring,
				RouteSource: "task_route",
				Model:       appruns.BuilderRuntimeModelRef{Primary: "gemma4-26b-local", Fallbacks: []string{"gpt-4o-mini"}},
			}},
		},
		LogPath: filepath.Join(t.TempDir(), "builder-runtime-semantic-repair-non-repair.log"),
	}
	roundInput := appruns.RoundInput{
		RoundID:        "round-non-repair-semantic-repair",
		Attempt:        1,
		TaskBundle:     run.TaskBundle,
		AllowedPaths:   run.AllowedPaths,
		BuilderRuntime: run.BuilderRuntime,
	}
	runner := &Runner{PatchGenerator: &stubBuilderRuntimePatchGenerator{responses: []BuilderRuntimePatchResponse{
		{
			ModelAlias: "gemma4-26b-local",
			Content:    `{"patch_id":"patch-semantic-conflict-1","operations":[{"type":"write_file","path":"lib/main.dart","content":"import 'package:flutter/material.dart';\n\nimport 'views/home_page.dart';\n\nvoid main() {\n  runApp(MaterialApp(home: HomePage()));\n}\n"},{"type":"write_file","path":"lib/views/home_page.dart","content":"class HomePage {\n  String render(dynamic summary, dynamic entry) {\n    return '${summary.entryCount}-${entry.date.year}';\n  }\n}\n"}]}`,
		},
		{
			ModelAlias: "gemma4-26b-local",
			Content:    `{"patch_id":"patch-semantic-conflict-2","operations":[{"type":"write_file","path":"lib/views/home_page.dart","content":"class HomePage {\n  String render(dynamic summary, dynamic entry) {\n    return '${summary.balance}-${entry.occurredOn.year}';\n  }\n}\n"}]}`,
		},
		{
			ModelAlias: "gemma4-26b-local",
			Content:    `{"patch_id":"patch-semantic-conflict-3","operations":[{"type":"write_file","path":"lib/views/entry_form_page.dart","content":"class EntryFormPage {\n  const EntryFormPage();\n}\n"},{"type":"write_file","path":"lib/views/entry_list_page.dart","content":"class EntryListPage {\n  const EntryListPage();\n}\n"}]}`,
		},
	}}}
	step := ExecutionStep{StepID: "thin-prepare", Stage: appruns.StageThinPrepare}

	result, err := runner.executeBuilderRuntimeEdit(context.Background(), noopRunnerBackend{}, run.RunID, step, run, roundInput)
	if err != nil {
		t.Fatalf("executeBuilderRuntimeEdit() error = %v, want semantic repair then coverage repair success", err)
	}
	if result == nil || result.Stats == nil {
		t.Fatalf("result = %+v, want stats", result)
	}
	if result.Stats.Attempts != 3 {
		t.Fatalf("stats.Attempts = %d, want 3", result.Stats.Attempts)
	}
	generator, ok := runner.PatchGenerator.(*stubBuilderRuntimePatchGenerator)
	if !ok {
		t.Fatalf("PatchGenerator = %T, want *stubBuilderRuntimePatchGenerator", runner.PatchGenerator)
	}
	if len(generator.requests) != 3 {
		t.Fatalf("len(generator.requests) = %d, want 3", len(generator.requests))
	}
	if !generator.requests[1].RepairOnly {
		t.Fatalf("semantic repair request RepairOnly = false, want true")
	}
	if got := generator.requests[1].RoundInput.TaskBundle[0].TargetPaths; len(got) != 1 || got[0] != "lib/views/home_page.dart" {
		t.Fatalf("semantic repair target_paths = %v, want [lib/views/home_page.dart]", got)
	}
	if !generator.requests[2].RepairOnly {
		t.Fatalf("coverage repair request RepairOnly = false, want true")
	}
	if got := generator.requests[2].RoundInput.TaskBundle[0].TargetPaths; len(got) != 2 || got[0] != "lib/views/entry_form_page.dart" || got[1] != "lib/views/entry_list_page.dart" {
		t.Fatalf("coverage repair target_paths = %v, want [lib/views/entry_form_page.dart lib/views/entry_list_page.dart]", got)
	}
	for _, check := range []struct {
		path   string
		needle string
	}{
		{mainPath, "HomePage()"},
		{homePath, "entry.occurredOn.year"},
		{formPath, "const EntryFormPage"},
		{listPath, "const EntryListPage"},
	} {
		content, readErr := os.ReadFile(check.path)
		if readErr != nil {
			t.Fatalf("ReadFile(%s) error = %v", check.path, readErr)
		}
		if !strings.Contains(string(content), check.needle) {
			t.Fatalf("content(%s) = %q, want %q", check.path, string(content), check.needle)
		}
	}
}

func TestExecuteBuilderRuntimeEditRepairsUndeclaredPackageImportSemanticConflict(t *testing.T) {
	jobRoot := t.TempDir()
	workspace := filepath.Join(jobRoot, "workspace")
	mainPath := filepath.Join(workspace, "lib", "main.dart")
	writeBuilderRuntimeWorkspaceFiles(t, workspace, map[string]string{
		"pubspec.yaml": strings.Join([]string{
			"name: weight_tracker",
			"dependencies:",
			"  flutter:",
			"    sdk: flutter",
			"  hive_flutter: ^1.1.0",
		}, "\n") + "\n",
		"lib/main.dart": "void main() {}\n",
	})
	run := runRecord{
		RunID:         "run-main-undeclared-package-semantic-repair",
		GoalSummary:   "绑定应用入口。",
		WorkspacePath: workspace,
		AllowedPaths:  []string{"lib/**"},
		RoundState:    &appruns.RoundState{CurrentTaskID: "task-bind-app-entry"},
		TaskBundle: []appruns.TaskBundleItem{{
			TaskID:      "task-bind-app-entry",
			TaskType:    appruns.BuilderRuntimeTaskTypeDualFileWiring,
			TargetPaths: []string{"lib/main.dart"},
		}},
		BuilderRuntime: &appruns.BuilderRuntimePlan{
			Enabled: true,
			TaskRoutes: []appruns.BuilderRuntimeTaskRoute{{
				TaskID:      "task-bind-app-entry",
				TaskType:    appruns.BuilderRuntimeTaskTypeDualFileWiring,
				RouteSource: "task_route",
				Model:       appruns.BuilderRuntimeModelRef{Primary: "gemma4-26b-local", Fallbacks: []string{"gpt-4o-mini"}},
			}},
		},
		LogPath: filepath.Join(t.TempDir(), "builder-runtime-main-undeclared-package-semantic-repair.log"),
	}
	roundInput := appruns.RoundInput{
		RoundID:        "round-main-undeclared-package-semantic-repair",
		Attempt:        1,
		TaskBundle:     run.TaskBundle,
		AllowedPaths:   run.AllowedPaths,
		BuilderRuntime: run.BuilderRuntime,
	}
	runner := &Runner{PatchGenerator: &stubBuilderRuntimePatchGenerator{responses: []BuilderRuntimePatchResponse{
		{
			ModelAlias: "gemma4-26b-local",
			Content:    `{"patch_id":"patch-bad-main-provider","operations":[{"type":"write_file","path":"lib/main.dart","content":"import 'package:flutter/material.dart';\nimport 'package:provider/provider.dart';\n\nvoid main() {\n  runApp(const MaterialApp(home: SizedBox.shrink()));\n}\n"}]}`,
		},
		{
			ModelAlias: "gemma4-26b-local",
			Content:    `{"patch_id":"patch-repair-main-provider","operations":[{"type":"write_file","path":"lib/main.dart","content":"import 'package:flutter/material.dart';\n\nvoid main() {\n  runApp(const MaterialApp(home: SizedBox.shrink()));\n}\n"}]}`,
		},
	}}}
	step := ExecutionStep{StepID: "thin-prepare", Stage: appruns.StageThinPrepare}

	result, err := runner.executeBuilderRuntimeEdit(context.Background(), noopRunnerBackend{}, run.RunID, step, run, roundInput)
	if err != nil {
		t.Fatalf("executeBuilderRuntimeEdit() error = %v, want semantic repair success for undeclared package imports", err)
	}
	if result == nil || result.Stats == nil {
		t.Fatalf("result = %+v, want stats", result)
	}
	if result.Stats.Attempts != 2 {
		t.Fatalf("stats.Attempts = %d, want 2", result.Stats.Attempts)
	}
	generator, ok := runner.PatchGenerator.(*stubBuilderRuntimePatchGenerator)
	if !ok {
		t.Fatalf("PatchGenerator = %T, want *stubBuilderRuntimePatchGenerator", runner.PatchGenerator)
	}
	if len(generator.requests) != 2 {
		t.Fatalf("len(generator.requests) = %d, want 2", len(generator.requests))
	}
	if !generator.requests[1].RepairOnly {
		t.Fatalf("semantic repair request RepairOnly = false, want true")
	}
	if !strings.Contains(generator.requests[1].PreviousErr, "patch introduced package imports not declared in pubspec.yaml") {
		t.Fatalf("semantic repair PreviousErr = %q, want undeclared package import violation", generator.requests[1].PreviousErr)
	}
	if got := generator.requests[1].RoundInput.TaskBundle[0].TargetPaths; len(got) != 1 || got[0] != "lib/main.dart" {
		t.Fatalf("semantic repair target_paths = %v, want [lib/main.dart]", got)
	}
	content, err := os.ReadFile(mainPath)
	if err != nil {
		t.Fatalf("ReadFile(main.dart) error = %v", err)
	}
	if strings.Contains(string(content), "provider/provider.dart") {
		t.Fatalf("main.dart still contains stray provider import after repair: %q", string(content))
	}
	if !strings.Contains(string(content), "runApp(const MaterialApp(home: SizedBox.shrink()));") {
		t.Fatalf("main.dart content = %q, want repaired app entry", string(content))
	}
}

func TestExecuteBuilderRuntimeEditAllowsMultiRoundCoverageRepairAfterSemanticRepair(t *testing.T) {
	jobRoot := t.TempDir()
	workspace := filepath.Join(jobRoot, "workspace")
	entryFormControllerPath := filepath.Join(workspace, "lib", "controllers", "entry_form_controller.dart")
	controllerListPath := filepath.Join(workspace, "lib", "controllers", "entry_list_controller.dart")
	homeControllerPath := filepath.Join(workspace, "lib", "controllers", "home_controller.dart")
	repositoryPath := filepath.Join(workspace, "lib", "repositories", "entry_repository.dart")
	widgetTestPath := filepath.Join(workspace, "test", "widget_test.dart")
	entryModelPath := filepath.Join(workspace, "lib", "models", "entry.dart")
	summaryModelPath := filepath.Join(workspace, "lib", "models", "summary.dart")
	for _, filePath := range []string{entryFormControllerPath, controllerListPath, homeControllerPath, repositoryPath, widgetTestPath, entryModelPath, summaryModelPath} {
		if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
			t.Fatalf("MkdirAll(%s) error = %v", filePath, err)
		}
	}
	for path, content := range map[string]string{
		entryFormControllerPath: "class EntryFormController {}\n",
		controllerListPath:      "class EntryListController {}\n",
		homeControllerPath:      "class HomeController {}\n",
		repositoryPath:          "class EntryRepository {}\n",
		widgetTestPath:          "void main() {}\n",
		entryModelPath:          "class BookkeepingEntry {\n  final DateTime occurredOn;\n  const BookkeepingEntry({required this.occurredOn});\n}\n",
		summaryModelPath:        "class Summary {\n  final double balance;\n  const Summary({required this.balance});\n}\n",
	} {
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatalf("WriteFile(%s) error = %v", path, err)
		}
	}
	run := runRecord{
		RunID:         "run-non-repair-semantic-multi-coverage-repair",
		GoalSummary:   "接通记一笔与首页刷新主流程。",
		WorkspacePath: workspace,
		AllowedPaths:  []string{"lib/**", "test/**"},
		RoundState:    &appruns.RoundState{CurrentTaskID: "task-flow-wiring"},
		TaskBundle: []appruns.TaskBundleItem{{
			TaskID:      "task-flow-wiring",
			TaskType:    appruns.BuilderRuntimeTaskTypeDualFileWiring,
			TargetPaths: []string{"lib/controllers/entry_form_controller.dart", "lib/controllers/entry_list_controller.dart", "lib/controllers/home_controller.dart", "lib/repositories/entry_repository.dart", "test/widget_test.dart"},
		}},
		BuilderRuntime: &appruns.BuilderRuntimePlan{
			Enabled: true,
			TaskRoutes: []appruns.BuilderRuntimeTaskRoute{{
				TaskID:      "task-flow-wiring",
				TaskType:    appruns.BuilderRuntimeTaskTypeDualFileWiring,
				RouteSource: "task_route",
				Model:       appruns.BuilderRuntimeModelRef{Primary: "gemma4-26b-local", Fallbacks: []string{"gpt-4o-mini"}},
			}},
		},
		LogPath: filepath.Join(t.TempDir(), "builder-runtime-semantic-multi-coverage-repair.log"),
	}
	roundInput := appruns.RoundInput{
		RoundID:        "round-non-repair-semantic-multi-coverage-repair",
		Attempt:        1,
		TaskBundle:     run.TaskBundle,
		AllowedPaths:   run.AllowedPaths,
		BuilderRuntime: run.BuilderRuntime,
	}
	runner := &Runner{PatchGenerator: &stubBuilderRuntimePatchGenerator{responses: []BuilderRuntimePatchResponse{
		{
			ModelAlias: "gemma4-26b-local",
			Content:    `{"patch_id":"patch-flow-1","operations":[{"type":"write_file","path":"lib/controllers/entry_form_controller.dart","content":"import '../models/entry.dart';\nclass EntryFormController {\n  BookkeepingEntry bind(BookkeepingEntry entry) => entry;\n}\n"},{"type":"write_file","path":"test/widget_test.dart","content":"void main() {\n  BookkeepingEntry? entry;\n}\n"}]}`,
		},
		{
			ModelAlias: "gemma4-26b-local",
			Content:    `{"patch_id":"patch-flow-2","operations":[{"type":"write_file","path":"test/widget_test.dart","content":"import 'package:bookkeeping_lite/models/entry.dart';\nvoid main() {\n  BookkeepingEntry? entry;\n}\n"}]}`,
		},
		{
			ModelAlias: "gemma4-26b-local",
			Content:    `{"patch_id":"patch-flow-3","operations":[{"type":"write_file","path":"lib/controllers/home_controller.dart","content":"class HomeController {\n  const HomeController();\n}\n"},{"type":"write_file","path":"lib/repositories/entry_repository.dart","content":"class EntryRepository {\n  const EntryRepository();\n}\n"}]}`,
		},
		{
			ModelAlias: "gemma4-26b-local",
			Content:    `{"patch_id":"patch-flow-4","operations":[{"type":"write_file","path":"lib/controllers/entry_list_controller.dart","content":"class EntryListController {\n  const EntryListController();\n}\n"}]}`,
		},
	}}}
	step := ExecutionStep{StepID: "thin-prepare", Stage: appruns.StageThinPrepare}

	result, err := runner.executeBuilderRuntimeEdit(context.Background(), noopRunnerBackend{}, run.RunID, step, run, roundInput)
	if err != nil {
		t.Fatalf("executeBuilderRuntimeEdit() error = %v, want semantic repair then multi-round coverage repair success", err)
	}
	if result == nil || result.Stats == nil {
		t.Fatalf("result = %+v, want stats", result)
	}
	if result.Stats.Attempts != 4 {
		t.Fatalf("stats.Attempts = %d, want 4", result.Stats.Attempts)
	}
	generator, ok := runner.PatchGenerator.(*stubBuilderRuntimePatchGenerator)
	if !ok {
		t.Fatalf("PatchGenerator = %T, want *stubBuilderRuntimePatchGenerator", runner.PatchGenerator)
	}
	if len(generator.requests) != 4 {
		t.Fatalf("len(generator.requests) = %d, want 4", len(generator.requests))
	}
	if got := generator.requests[1].RoundInput.TaskBundle[0].TargetPaths; len(got) != 1 || got[0] != "test/widget_test.dart" {
		t.Fatalf("semantic repair target_paths = %v, want [test/widget_test.dart]", got)
	}
	if got := generator.requests[2].RoundInput.TaskBundle[0].TargetPaths; !reflect.DeepEqual(got, []string{"lib/controllers/entry_list_controller.dart", "lib/controllers/home_controller.dart", "lib/repositories/entry_repository.dart"}) {
		t.Fatalf("first coverage repair target_paths = %v, want [lib/controllers/entry_list_controller.dart lib/controllers/home_controller.dart lib/repositories/entry_repository.dart]", got)
	}
	if got := generator.requests[3].RoundInput.TaskBundle[0].TargetPaths; len(got) != 1 || got[0] != "lib/controllers/entry_list_controller.dart" {
		t.Fatalf("second coverage repair target_paths = %v, want [lib/controllers/entry_list_controller.dart]", got)
	}
	for _, check := range []struct {
		path   string
		needle string
	}{
		{entryFormControllerPath, "bind(BookkeepingEntry entry) => entry"},
		{controllerListPath, "const EntryListController"},
		{homeControllerPath, "const HomeController"},
		{repositoryPath, "const EntryRepository"},
		{widgetTestPath, "import 'package:bookkeeping_lite/models/entry.dart';"},
	} {
		content, readErr := os.ReadFile(check.path)
		if readErr != nil {
			t.Fatalf("ReadFile(%s) error = %v", check.path, readErr)
		}
		if !strings.Contains(string(content), check.needle) {
			t.Fatalf("content(%s) = %q, want %q", check.path, string(content), check.needle)
		}
	}
}

func TestExecuteBuilderRuntimeEditFailsWhenPatchRequestTimesOut(t *testing.T) {
	previousTimeout := builderRuntimePatchRequestTimeout
	previousHeartbeatInterval := builderRuntimePatchProgressHeartbeatInterval
	builderRuntimePatchRequestTimeout = 40 * time.Millisecond
	builderRuntimePatchProgressHeartbeatInterval = 10 * time.Millisecond
	defer func() {
		builderRuntimePatchRequestTimeout = previousTimeout
		builderRuntimePatchProgressHeartbeatInterval = previousHeartbeatInterval
	}()

	jobRoot := t.TempDir()
	workspace := filepath.Join(jobRoot, "workspace")
	modelPath := filepath.Join(workspace, "lib", "models", "record.dart")
	if err := os.MkdirAll(filepath.Dir(modelPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(model) error = %v", err)
	}
	if err := os.WriteFile(modelPath, []byte("class AppRecord {\n  const AppRecord();\n}\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(model) error = %v", err)
	}
	run := runRecord{
		RunID:         "run-timeout",
		GoalSummary:   "将体重记录需求整理成基于 flutter-open-lite 的多页面 Android MVP 输入包。",
		WorkspacePath: workspace,
		AllowedPaths:  []string{"lib/**"},
		TaskBundle: []appruns.TaskBundleItem{{
			TaskID:      "task-create-record-model",
			TaskType:    appruns.BuilderRuntimeTaskTypeSingleFileEdit,
			TargetPaths: []string{"lib/models/record.dart"},
		}},
		BuilderRuntime: &appruns.BuilderRuntimePlan{
			Enabled: true,
			TaskRoutes: []appruns.BuilderRuntimeTaskRoute{{
				TaskID:      "task-create-record-model",
				TaskType:    appruns.BuilderRuntimeTaskTypeSingleFileEdit,
				RouteSource: "task_route",
				Model:       appruns.BuilderRuntimeModelRef{Primary: "qwen2.5-coder-14b-local"},
			}},
		},
		LogPath: filepath.Join(t.TempDir(), "builder-runtime-timeout.log"),
	}
	roundInput := appruns.RoundInput{
		RoundID:        "round-1",
		Attempt:        1,
		TaskBundle:     run.TaskBundle,
		AllowedPaths:   run.AllowedPaths,
		BuilderRuntime: run.BuilderRuntime,
	}
	runner := &Runner{PatchGenerator: blockingBuilderRuntimePatchGenerator{}}
	step := ExecutionStep{StepID: "thin-prepare", Stage: appruns.StageThinPrepare}
	backend := &recordingRunnerBackend{}

	_, err := runner.executeBuilderRuntimeEdit(context.Background(), backend, run.RunID, step, run, roundInput)
	if err == nil {
		t.Fatal("executeBuilderRuntimeEdit() error = nil, want timeout")
	}
	if !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("executeBuilderRuntimeEdit() error = %v, want timeout detail", err)
	}
	var sawStarted bool
	var sawWaiting bool
	var sawFailed bool
	for _, heartbeat := range backend.heartbeats {
		switch heartbeat.EventType {
		case "run_patch_generation_started":
			sawStarted = true
		case "run_patch_generation_waiting":
			sawWaiting = true
			if !strings.Contains(heartbeat.Summary, "lib/models/record.dart") {
				t.Fatalf("waiting heartbeat summary = %q, want target path", heartbeat.Summary)
			}
			if !strings.Contains(heartbeat.Summary, "qwen2.5-coder-14b-local") {
				t.Fatalf("waiting heartbeat summary = %q, want model alias", heartbeat.Summary)
			}
		case "run_patch_generation_failed":
			sawFailed = true
			if !strings.Contains(heartbeat.Summary, "timed out") {
				t.Fatalf("failed heartbeat summary = %q, want timeout detail", heartbeat.Summary)
			}
		}
	}
	if !sawStarted {
		t.Fatalf("heartbeat events = %#v, want run_patch_generation_started", backend.heartbeats)
	}
	if !sawWaiting {
		t.Fatalf("heartbeat events = %#v, want run_patch_generation_waiting", backend.heartbeats)
	}
	if !sawFailed {
		t.Fatalf("heartbeat events = %#v, want run_patch_generation_failed", backend.heartbeats)
	}
}

func TestExecuteBuilderRuntimeEditPrunesDependentOperationsFromMainOnlyPatch(t *testing.T) {
	workspace := t.TempDir()
	run := runRecord{
		RunID:         "run-main-only-dependent-prune",
		GoalSummary:   "接线 open-lite 的主入口。",
		WorkspacePath: workspace,
		AllowedPaths:  []string{"lib/**"},
		RoundState:    &appruns.RoundState{CurrentTaskID: "task-bind-app-entry"},
		TaskBundle: []appruns.TaskBundleItem{{
			TaskID:      "task-bind-app-entry",
			TaskType:    appruns.BuilderRuntimeTaskTypeDualFileWiring,
			TargetPaths: []string{"lib/main.dart"},
		}},
		BuilderRuntime: &appruns.BuilderRuntimePlan{
			Enabled: true,
			TaskRoutes: []appruns.BuilderRuntimeTaskRoute{{
				TaskID:      "task-bind-app-entry",
				TaskType:    appruns.BuilderRuntimeTaskTypeDualFileWiring,
				RouteSource: "task_route",
				Model:       appruns.BuilderRuntimeModelRef{Primary: "gemma4-26b-local"},
			}},
		},
		LogPath: filepath.Join(t.TempDir(), "builder-runtime-main-only-dependent-prune.log"),
	}
	roundInput := appruns.RoundInput{
		RoundID:        "round-1",
		Attempt:        1,
		TaskBundle:     run.TaskBundle,
		AllowedPaths:   run.AllowedPaths,
		BuilderRuntime: run.BuilderRuntime,
	}
	runner := &Runner{PatchGenerator: &stubBuilderRuntimePatchGenerator{response: BuilderRuntimePatchResponse{
		ModelAlias: "gemma4-26b-local",
		Content:    `{"patch_id":"patch-main-only-dependent-prune","operations":[{"type":"write_file","path":"lib/main.dart","content":"void main() {}\n"},{"type":"write_file","path":"lib/views/record_list_page.dart","content":"class RecordListPage {}\n"}]}`,
	}}}
	step := ExecutionStep{StepID: "thin-prepare", Stage: appruns.StageThinPrepare}

	result, err := runner.executeBuilderRuntimeEdit(context.Background(), noopRunnerBackend{}, run.RunID, step, run, roundInput)
	if err != nil {
		t.Fatalf("executeBuilderRuntimeEdit() error = %v, want main-only patch pruning success", err)
	}
	if result == nil || result.Patch == nil {
		t.Fatalf("result = %+v, want applied patch", result)
	}
	if len(result.Patch.Operations) != 1 {
		t.Fatalf("len(result.Patch.Operations) = %d, want 1 after pruning dependent edits", len(result.Patch.Operations))
	}
	if result.Patch.Operations[0].Path != "lib/main.dart" {
		t.Fatalf("result.Patch.Operations[0].Path = %q, want lib/main.dart", result.Patch.Operations[0].Path)
	}
	if _, err := os.Stat(filepath.Join(workspace, "lib", "views", "record_list_page.dart")); !os.IsNotExist(err) {
		t.Fatalf("record_list_page.dart should remain untouched, stat error = %v", err)
	}
}

func TestExecuteBuilderRuntimeEditSupportsMultiRoundDirectCoverageRepair(t *testing.T) {
	workspace := t.TempDir()
	entryRepoPath := filepath.Join(workspace, "lib", "repositories", "entry_repository.dart")
	listPagePath := filepath.Join(workspace, "lib", "views", "entry_list_page.dart")
	homePagePath := filepath.Join(workspace, "lib", "views", "home_page.dart")
	mainPath := filepath.Join(workspace, "lib", "main.dart")
	entryModelPath := filepath.Join(workspace, "lib", "models", "entry.dart")
	widgetTestPath := filepath.Join(workspace, "test", "widget_test.dart")
	controllerListPath := filepath.Join(workspace, "lib", "controllers", "entry_list_controller.dart")
	homeControllerPath := filepath.Join(workspace, "lib", "controllers", "home_controller.dart")
	for _, dir := range []string{
		filepath.Dir(entryRepoPath),
		filepath.Dir(listPagePath),
		filepath.Dir(homePagePath),
		filepath.Dir(mainPath),
		filepath.Dir(entryModelPath),
		filepath.Dir(widgetTestPath),
		filepath.Dir(controllerListPath),
		filepath.Dir(homeControllerPath),
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("MkdirAll(%s) error = %v", dir, err)
		}
	}
	for path, content := range map[string]string{
		entryRepoPath:      "abstract class EntryRepository {}\n",
		listPagePath:       "class EntryListPage {}\n",
		homePagePath:       "class HomePage {}\n",
		mainPath:           "void main() {}\n",
		entryModelPath:     "class BookkeepingEntry {}\n",
		widgetTestPath:     "void main() {}\n",
		controllerListPath: "class EntryListController {}\n",
		homeControllerPath: "class HomeController {}\n",
	} {
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatalf("WriteFile(%s) error = %v", path, err)
		}
	}
	run := runRecord{
		RunID:         "run-multi-round-direct-failure-coverage-repair",
		GoalSummary:   "修复 analyze repair 多轮漏文件问题。",
		WorkspacePath: workspace,
		AllowedPaths:  []string{"lib/**", "test/**"},
		TaskBundle: []appruns.TaskBundleItem{{
			TaskID:      "repair-check-flutter-analyze",
			TaskType:    appruns.BuilderRuntimeTaskTypeAnalyzeRepair,
			TargetPaths: []string{"lib/controllers/entry_list_controller.dart", "lib/controllers/home_controller.dart", "lib/main.dart", "lib/models/entry.dart", "lib/repositories/entry_repository.dart", "lib/views/entry_list_page.dart", "lib/views/home_page.dart", "test/widget_test.dart"},
		}},
		BuilderRuntime: &appruns.BuilderRuntimePlan{
			Enabled: true,
			TaskRoutes: []appruns.BuilderRuntimeTaskRoute{{
				TaskID:      "repair-check-flutter-analyze",
				TaskType:    appruns.BuilderRuntimeTaskTypeAnalyzeRepair,
				RouteSource: "task_route",
				Model:       appruns.BuilderRuntimeModelRef{Primary: "gemma4-26b-local", Fallbacks: []string{"gpt-4o-mini"}},
			}},
		},
		LogPath: filepath.Join(t.TempDir(), "builder-runtime-multi-coverage-repair.log"),
	}
	roundInput := appruns.RoundInput{
		RoundID:        "round-analyze-repair",
		Attempt:        2,
		TaskBundle:     run.TaskBundle,
		AllowedPaths:   run.AllowedPaths,
		BuilderRuntime: run.BuilderRuntime,
	}
	runner := &Runner{PatchGenerator: &stubBuilderRuntimePatchGenerator{responses: []BuilderRuntimePatchResponse{
		{
			ModelAlias: "gemma4-26b-local",
			Content:    `{"patch_id":"round-analyze-repair-1","operations":[{"type":"write_file","path":"lib/repositories/entry_repository.dart","content":"class EntryRepository {\n  Future<void> save() async {}\n}\n"},{"type":"write_file","path":"lib/views/entry_list_page.dart","content":"class EntryListPage {\n  const EntryListPage();\n}\n"}]}`,
		},
		{
			ModelAlias: "gemma4-26b-local",
			Content:    `{"patch_id":"round-analyze-repair-2","operations":[{"type":"write_file","path":"lib/controllers/entry_list_controller.dart","content":"class EntryListController {\n  const EntryListController();\n}\n"},{"type":"write_file","path":"lib/controllers/home_controller.dart","content":"class HomeController {\n  const HomeController();\n}\n"}]}`,
		},
		{
			ModelAlias: "gemma4-26b-local",
			Content:    `{"patch_id":"round-analyze-repair-3","operations":[{"type":"write_file","path":"lib/main.dart","content":"void main() {\n  // repaired main entry\n}\n"},{"type":"write_file","path":"test/widget_test.dart","content":"void main() {\n  // repaired multi-round test scaffold\n}\n"}]}`,
		},
		{
			ModelAlias: "gemma4-26b-local",
			Content:    `{"patch_id":"round-analyze-repair-4","operations":[{"type":"write_file","path":"lib/models/entry.dart","content":"class BookkeepingEntry {\n  const BookkeepingEntry();\n}\n"},{"type":"write_file","path":"lib/views/home_page.dart","content":"class HomePage {\n  const HomePage();\n}\n"}]}`,
		},
	}}}
	step := ExecutionStep{StepID: "check-flutter-analyze", Stage: appruns.StageCheap}

	result, err := runner.executeBuilderRuntimeEditWithFailureContext(context.Background(), noopRunnerBackend{}, run.RunID, step, run, roundInput, "flutter analyze failed in lib/controllers/entry_list_controller.dart, lib/controllers/home_controller.dart, lib/main.dart, lib/models/entry.dart, lib/repositories/entry_repository.dart, lib/views/entry_list_page.dart, lib/views/home_page.dart, test/widget_test.dart")
	if err != nil {
		t.Fatalf("executeBuilderRuntimeEditWithFailureContext() error = %v, want multi-round direct coverage repair success", err)
	}
	if result == nil || result.Stats == nil {
		t.Fatalf("result = %+v, want stats", result)
	}
	if result.Stats.Attempts != 4 {
		t.Fatalf("stats.Attempts = %d, want 4", result.Stats.Attempts)
	}
	generator, ok := runner.PatchGenerator.(*stubBuilderRuntimePatchGenerator)
	if !ok {
		t.Fatalf("PatchGenerator = %T, want *stubBuilderRuntimePatchGenerator", runner.PatchGenerator)
	}
	if len(generator.requests) != 4 {
		t.Fatalf("len(generator.requests) = %d, want 4", len(generator.requests))
	}
	if len(generator.requests[2].RoundInput.TaskBundle) != 1 {
		t.Fatalf("third coverage repair target_paths = %#v, want single narrowed task bundle", generator.requests[2].RoundInput.TaskBundle)
	}
	wantThirdTargets := []string{"lib/main.dart", "lib/models/entry.dart", "lib/views/home_page.dart", "test/widget_test.dart"}
	if !reflect.DeepEqual(generator.requests[2].RoundInput.TaskBundle[0].TargetPaths, wantThirdTargets) {
		t.Fatalf("third coverage repair target_paths = %v, want %v", generator.requests[2].RoundInput.TaskBundle[0].TargetPaths, wantThirdTargets)
	}
	if len(generator.requests[3].RoundInput.TaskBundle) != 1 {
		t.Fatalf("fourth coverage repair target_paths = %#v, want single narrowed task bundle", generator.requests[3].RoundInput.TaskBundle)
	}
	wantFourthTargets := []string{"lib/models/entry.dart", "lib/views/home_page.dart"}
	if !reflect.DeepEqual(generator.requests[3].RoundInput.TaskBundle[0].TargetPaths, wantFourthTargets) {
		t.Fatalf("fourth coverage repair target_paths = %v, want %v", generator.requests[3].RoundInput.TaskBundle[0].TargetPaths, wantFourthTargets)
	}
	for _, check := range []struct {
		path   string
		needle string
	}{
		{entryRepoPath, "class EntryRepository"},
		{listPagePath, "const EntryListPage"},
		{controllerListPath, "const EntryListController"},
		{homeControllerPath, "const HomeController"},
		{mainPath, "repaired main entry"},
		{entryModelPath, "const BookkeepingEntry"},
		{homePagePath, "const HomePage"},
		{widgetTestPath, "repaired multi-round test scaffold"},
	} {
		content, readErr := os.ReadFile(check.path)
		if readErr != nil {
			t.Fatalf("ReadFile(%s) error = %v", check.path, readErr)
		}
		if !strings.Contains(string(content), check.needle) {
			t.Fatalf("content(%s) = %q, want %q", check.path, string(content), check.needle)
		}
	}
}

func TestGenerateBuilderRuntimePatchFallsBackAfterPrimaryTimeout(t *testing.T) {
	previousTimeout := builderRuntimePatchRequestTimeout
	builderRuntimePatchRequestTimeout = 10 * time.Millisecond
	defer func() {
		builderRuntimePatchRequestTimeout = previousTimeout
	}()

	generator := &aliasAwareBuilderRuntimePatchGenerator{
		responses: map[string]BuilderRuntimePatchResponse{
			"gpt-4o-mini": {
				ModelAlias: "gpt-4o-mini",
				Content:    `{"patch_id":"round-1-patch","operations":[{"type":"write_file","path":"lib/models/record.dart","content":"class AppRecord {}\n"}]}`,
			},
		},
		blocking: map[string]bool{"qwen2.5-coder-32b-local": true},
	}
	runner := &Runner{PatchGenerator: generator}

	response, err := runner.generateBuilderRuntimePatch(context.Background(), BuilderRuntimePatchRequest{
		ModelAliases: []string{"qwen2.5-coder-32b-local", "gpt-4o-mini"},
	})
	if err != nil {
		t.Fatalf("generateBuilderRuntimePatch() error = %v, want fallback success", err)
	}
	if response.ModelAlias != "gpt-4o-mini" {
		t.Fatalf("response.ModelAlias = %q, want gpt-4o-mini", response.ModelAlias)
	}
	if len(generator.requests) != 2 {
		t.Fatalf("len(generator.requests) = %d, want 2", len(generator.requests))
	}
	if got := generator.requests[0].ModelAliases; len(got) != 1 || got[0] != "qwen2.5-coder-32b-local" {
		t.Fatalf("first request aliases = %#v, want [qwen2.5-coder-32b-local]", got)
	}
	if got := generator.requests[1].ModelAliases; len(got) != 1 || got[0] != "gpt-4o-mini" {
		t.Fatalf("second request aliases = %#v, want [gpt-4o-mini]", got)
	}
	if !generator.deadlineExceeded["qwen2.5-coder-32b-local"] {
		t.Fatal("primary alias did not observe deadline exceeded")
	}
	if generator.deadlineExceeded["gpt-4o-mini"] {
		t.Fatal("fallback alias unexpectedly observed deadline exceeded")
	}
}

func TestGenerateBuilderRuntimePatchFallsBackAfterPrimaryEmptyResponse(t *testing.T) {
	generator := &aliasAwareBuilderRuntimePatchGenerator{
		responses: map[string]BuilderRuntimePatchResponse{
			"qwen2.5-coder-32b-local": {
				ModelAlias: "qwen2.5-coder-32b-local",
				Content:    "   \n\t  ",
			},
			"gpt-4o-mini": {
				ModelAlias: "gpt-4o-mini",
				Content:    `{"patch_id":"round-1-patch","operations":[{"type":"write_file","path":"lib/models/record.dart","content":"class AppRecord {}\n"}]}`,
			},
		},
	}
	runner := &Runner{PatchGenerator: generator}

	response, err := runner.generateBuilderRuntimePatch(context.Background(), BuilderRuntimePatchRequest{
		ModelAliases: []string{"qwen2.5-coder-32b-local", "gpt-4o-mini"},
	})
	if err != nil {
		t.Fatalf("generateBuilderRuntimePatch() error = %v, want fallback success", err)
	}
	if response.ModelAlias != "gpt-4o-mini" {
		t.Fatalf("response.ModelAlias = %q, want gpt-4o-mini", response.ModelAlias)
	}
	if len(generator.requests) != 2 {
		t.Fatalf("len(generator.requests) = %d, want 2", len(generator.requests))
	}
	if got := generator.requests[0].ModelAliases; len(got) != 1 || got[0] != "qwen2.5-coder-32b-local" {
		t.Fatalf("first request aliases = %#v, want [qwen2.5-coder-32b-local]", got)
	}
	if got := generator.requests[1].ModelAliases; len(got) != 1 || got[0] != "gpt-4o-mini" {
		t.Fatalf("second request aliases = %#v, want [gpt-4o-mini]", got)
	}
}

func TestGenerateBuilderRuntimePatchReturnsRequestFailureWhenFallbackErrorsAfterEmptyResponse(t *testing.T) {
	previousTimeout := builderRuntimePatchRequestTimeout
	builderRuntimePatchRequestTimeout = 10 * time.Millisecond
	defer func() {
		builderRuntimePatchRequestTimeout = previousTimeout
	}()

	generator := &aliasAwareBuilderRuntimePatchGenerator{
		responses: map[string]BuilderRuntimePatchResponse{
			"qwen2.5-coder-32b-local": {
				ModelAlias: "qwen2.5-coder-32b-local",
				Content:    "   \n\t  ",
			},
		},
		blocking: map[string]bool{"gpt-4o-mini": true},
	}
	runner := &Runner{PatchGenerator: generator}

	_, err := runner.generateBuilderRuntimePatch(context.Background(), BuilderRuntimePatchRequest{
		ModelAliases: []string{"qwen2.5-coder-32b-local", "gpt-4o-mini"},
	})
	if err == nil {
		t.Fatal("generateBuilderRuntimePatch() error = nil, want aggregated model request failure")
	}
	if !strings.Contains(err.Error(), "builder runtime model request failed") {
		t.Fatalf("generateBuilderRuntimePatch() error = %q, want model request failure", err)
	}
	if !strings.Contains(err.Error(), "qwen2.5-coder-32b-local: builder runtime model returned empty patch content") {
		t.Fatalf("generateBuilderRuntimePatch() error = %q, want empty primary alias detail", err)
	}
	if !strings.Contains(err.Error(), "gpt-4o-mini: builder runtime patch request timed out") {
		t.Fatalf("generateBuilderRuntimePatch() error = %q, want fallback timeout detail", err)
	}
	if len(generator.requests) != 2 {
		t.Fatalf("len(generator.requests) = %d, want 2", len(generator.requests))
	}
}

type noopRunnerBackend struct{}

func (noopRunnerBackend) GetRun(context.Context, string) (appruns.RunRecord, error) {
	return appruns.RunRecord{}, nil
}

func (noopRunnerBackend) Heartbeat(context.Context, string, appruns.Heartbeat) error {
	return nil
}

func (noopRunnerBackend) Complete(context.Context, string, appruns.BuildOutput) error {
	return nil
}

func (noopRunnerBackend) Fail(context.Context, string, appruns.FailureReport) error {
	return nil
}

func (noopRunnerBackend) IndexArtifacts(context.Context, string, appruns.ArtifactManifest) error {
	return nil
}

func (noopRunnerBackend) IndexMetrics(context.Context, string, appruns.Metrics) error {
	return nil
}

type recordingRunnerBackend struct {
	noopRunnerBackend
	heartbeats []appruns.Heartbeat
}

func (backend *recordingRunnerBackend) Heartbeat(_ context.Context, _ string, heartbeat appruns.Heartbeat) error {
	backend.heartbeats = append(backend.heartbeats, heartbeat)
	return nil
}

type stubBuilderRuntimePatchGenerator struct {
	response  BuilderRuntimePatchResponse
	responses []BuilderRuntimePatchResponse
	index     int
	requests  []BuilderRuntimePatchRequest
}

func (generator *stubBuilderRuntimePatchGenerator) GeneratePatch(ctx context.Context, request BuilderRuntimePatchRequest) (BuilderRuntimePatchResponse, error) {
	generator.requests = append(generator.requests, request)
	if len(generator.responses) > 0 {
		if generator.index >= len(generator.responses) {
			return generator.responses[len(generator.responses)-1], nil
		}
		response := generator.responses[generator.index]
		generator.index++
		return response, nil
	}
	return generator.response, nil
}

type sequenceBuilderRuntimePatchGenerator struct {
	results  []builderRuntimePatchAttemptResult
	index    int
	requests []BuilderRuntimePatchRequest
}

func (generator *sequenceBuilderRuntimePatchGenerator) GeneratePatch(ctx context.Context, request BuilderRuntimePatchRequest) (BuilderRuntimePatchResponse, error) {
	generator.requests = append(generator.requests, request)
	if len(generator.results) == 0 {
		return BuilderRuntimePatchResponse{}, fmt.Errorf("no sequence patch results configured")
	}
	if generator.index >= len(generator.results) {
		result := generator.results[len(generator.results)-1]
		return result.response, result.err
	}
	result := generator.results[generator.index]
	generator.index++
	return result.response, result.err
}

type inspectingBuilderRuntimePatchGenerator struct {
	responses []BuilderRuntimePatchResponse
	index     int
	requests  []BuilderRuntimePatchRequest
	inspect   func(request BuilderRuntimePatchRequest, index int) error
}

func (generator *inspectingBuilderRuntimePatchGenerator) GeneratePatch(ctx context.Context, request BuilderRuntimePatchRequest) (BuilderRuntimePatchResponse, error) {
	if generator.inspect != nil {
		if err := generator.inspect(request, generator.index); err != nil {
			return BuilderRuntimePatchResponse{}, err
		}
	}
	generator.requests = append(generator.requests, request)
	if len(generator.responses) == 0 {
		return BuilderRuntimePatchResponse{}, fmt.Errorf("no stub responses configured")
	}
	if generator.index >= len(generator.responses) {
		return generator.responses[len(generator.responses)-1], nil
	}
	response := generator.responses[generator.index]
	generator.index++
	return response, nil
}

type blockingBuilderRuntimePatchGenerator struct{}

func (blockingBuilderRuntimePatchGenerator) GeneratePatch(ctx context.Context, request BuilderRuntimePatchRequest) (BuilderRuntimePatchResponse, error) {
	<-ctx.Done()
	return BuilderRuntimePatchResponse{}, ctx.Err()
}

type aliasAwareBuilderRuntimePatchGenerator struct {
	responses        map[string]BuilderRuntimePatchResponse
	blocking         map[string]bool
	requests         []BuilderRuntimePatchRequest
	deadlineExceeded map[string]bool
}

func (generator *aliasAwareBuilderRuntimePatchGenerator) GeneratePatch(ctx context.Context, request BuilderRuntimePatchRequest) (BuilderRuntimePatchResponse, error) {
	generator.requests = append(generator.requests, request)
	if generator.deadlineExceeded == nil {
		generator.deadlineExceeded = map[string]bool{}
	}
	alias := ""
	if len(request.ModelAliases) > 0 {
		alias = request.ModelAliases[0]
	}
	if generator.blocking[alias] {
		<-ctx.Done()
		generator.deadlineExceeded[alias] = errors.Is(ctx.Err(), context.DeadlineExceeded)
		return BuilderRuntimePatchResponse{}, ctx.Err()
	}
	response, ok := generator.responses[alias]
	if !ok {
		return BuilderRuntimePatchResponse{}, fmt.Errorf("unexpected alias: %s", alias)
	}
	return response, nil
}
