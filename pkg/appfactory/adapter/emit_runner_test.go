package adapter

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	appprepare "github.com/sipeed/picoclaw/pkg/appfactory/prepare"
	appruns "github.com/sipeed/picoclaw/pkg/appfactory/runs"
)

const (
	emitRunnerBindingOverview   = "surface-overview"
	emitRunnerBindingCollection = "surface-collection"
	emitRunnerBindingMutation   = "surface-mutation"
	emitRunnerBindingInspection = "surface-inspection"
	emitRunnerBindingAppEntry   = "app-entry"
	emitRunnerBindingCopy       = "domain-copy"
	emitRunnerBindingBranding   = "android-branding"
	emitRunnerBindingStorage    = "storage-boundary"
	emitRunnerBindingWidgetTest = "widget-test"
)

func writeEmitRunnerTemplateSlotMap(t *testing.T, preparePath string, slots ...appprepare.TemplateSlot) {
	t.Helper()
	if len(slots) == 0 {
		slots = defaultEmitRunnerTemplateSlots()
	}
	slotMap := appprepare.TemplateSlotMap{
		SchemaVersion: "0.1.0",
		TemplateID:    "flutter-open-lite",
		Slots:         append([]appprepare.TemplateSlot(nil), slots...),
	}
	content, err := json.Marshal(slotMap)
	if err != nil {
		t.Fatalf("Marshal(template-slot-map.json) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(preparePath, "template-slot-map.json"), content, 0o600); err != nil {
		t.Fatalf("WriteFile(template-slot-map.json) error = %v", err)
	}
}

func defaultEmitRunnerTemplateSlots() []appprepare.TemplateSlot {
	return []appprepare.TemplateSlot{
		emitRunnerTemplateSlot(emitRunnerBindingOverview, "open-lite-overview-slot", "summary", "replace", true, "lib/views/home_page.dart", "lib/controllers/home_controller.dart", "lib/models/dashboard_summary.dart"),
		emitRunnerTemplateSlot(emitRunnerBindingCollection, "open-lite-collection-slot", "list", "replace", true, "lib/views/record_list_page.dart", "lib/controllers/record_list_controller.dart"),
		emitRunnerTemplateSlot(emitRunnerBindingMutation, "open-lite-mutation-slot", "form", "replace", true, "lib/views/record_form_page.dart", "lib/controllers/record_form_controller.dart"),
		emitRunnerTemplateSlot(emitRunnerBindingInspection, "open-lite-inspection-slot", "detail", "replace", true, "lib/views/record_detail_page.dart"),
		emitRunnerTemplateSlot(emitRunnerBindingAppEntry, "open-lite-app-entry-slot", "app_entry", "replace", true, "lib/main.dart"),
		emitRunnerTemplateSlot(emitRunnerBindingCopy, "open-lite-copy-slot", "copy", "synchronize", true, "lib/template/open_lite_copy.dart", "lib/views/home_page.dart", "lib/views/record_form_page.dart", "lib/views/record_detail_page.dart", "lib/views/record_list_page.dart"),
		emitRunnerTemplateSlot(emitRunnerBindingBranding, "open-lite-branding-slot", "branding", "synchronize", true, "android/app/src/main/res/values/strings.xml", "android/app/build.gradle.kts"),
		emitRunnerTemplateSlot(emitRunnerBindingStorage, "open-lite-storage-slot", "storage", "extend", true, "lib/repositories/record_repository.dart", "pubspec.yaml"),
		emitRunnerTemplateSlot(emitRunnerBindingWidgetTest, "open-lite-widget-test-slot", "test", "synchronize", true, "test/widget_test.dart"),
	}
}

func emitRunnerTemplateSlot(bindingID, slotID, slotKind, overridePolicy string, emitEligible bool, targetPaths ...string) appprepare.TemplateSlot {
	return appprepare.TemplateSlot{
		BindingID:      bindingID,
		SlotID:         slotID,
		SlotKind:       slotKind,
		TargetPaths:    append([]string(nil), targetPaths...),
		OverridePolicy: overridePolicy,
		EmitEligible:   emitEligible,
	}
}

func TestTryDeterministicEmitCopySuccess(t *testing.T) {
	jobRoot := t.TempDir()
	workspace := filepath.Join(jobRoot, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatalf("MkdirAll(workspace) error = %v", err)
	}
	preparePath := filepath.Join(jobRoot, "prepare")
	if err := os.MkdirAll(preparePath, 0o755); err != nil {
		t.Fatalf("MkdirAll(prepare) error = %v", err)
	}
	dm := appprepare.DomainModel{
		DomainCopy: appprepare.DomainCopy{Title: "体重追踪"},
		Entities:   []appprepare.DataEntity{{EntityID: "entity-weight-record", Name: "WeightRecord"}},
	}
	dmBytes, _ := json.Marshal(dm)
	if err := os.WriteFile(filepath.Join(preparePath, "domain-model.json"), dmBytes, 0o600); err != nil {
		t.Fatalf("WriteFile(domain-model.json) error = %v", err)
	}
	writeEmitRunnerTemplateSlotMap(t, preparePath)
	// 创建 copy 目标目录
	if err := os.MkdirAll(filepath.Join(workspace, "lib", "template"), 0o755); err != nil {
		t.Fatalf("MkdirAll(lib/template) error = %v", err)
	}

	run := runRecord{
		WorkspacePath: workspace,
		AllowedPaths:  []string{"lib/**"},
		TaskBundle: []appruns.TaskBundleItem{{
			TaskID:      "task-copy",
			RouteHint:   appruns.TaskRouteHintDeterministic,
			TargetPaths: []string{"lib/template/open_lite_copy.dart"},
		}},
	}
	roundInput := appruns.RoundInput{RoundID: "round-1", TaskBundle: run.TaskBundle}

	result, err := tryDeterministicEmit(run, roundInput, "emit-patch-1")
	if err != nil {
		t.Fatalf("tryDeterministicEmit() error = %v", err)
	}
	if !result.Handled {
		t.Fatal("expected Handled=true for emit-eligible copy task")
	}
	if result.Patch == nil || len(result.Patch.Operations) != 1 {
		t.Fatalf("expected 1 operation, got %d", len(result.Patch.Operations))
	}
	if result.Patch.Operations[0].Path != "lib/template/open_lite_copy.dart" {
		t.Fatalf("operation path = %q, want lib/template/open_lite_copy.dart", result.Patch.Operations[0].Path)
	}
	// 验证文件已写入 workspace
	content, err := os.ReadFile(filepath.Join(workspace, "lib", "template", "open_lite_copy.dart"))
	if err != nil {
		t.Fatalf("ReadFile(open_lite_copy.dart) error = %v", err)
	}
	if !strings.Contains(string(content), "体重追踪") {
		t.Fatalf("emitted copy content missing app title, got: %s", string(content))
	}
}

func TestTryDeterministicEmitBrandSuccess(t *testing.T) {
	jobRoot := t.TempDir()
	workspace := filepath.Join(jobRoot, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatalf("MkdirAll(workspace) error = %v", err)
	}
	preparePath := filepath.Join(jobRoot, "prepare")
	if err := os.MkdirAll(preparePath, 0o755); err != nil {
		t.Fatalf("MkdirAll(prepare) error = %v", err)
	}
	dm := appprepare.DomainModel{
		DomainCopy: appprepare.DomainCopy{Title: "库存管家"},
		Entities:   []appprepare.DataEntity{{EntityID: "entity-inventory-sheet", Name: "InventorySheet"}},
	}
	dmBytes, _ := json.Marshal(dm)
	if err := os.WriteFile(filepath.Join(preparePath, "domain-model.json"), dmBytes, 0o600); err != nil {
		t.Fatalf("WriteFile(domain-model.json) error = %v", err)
	}
	writeEmitRunnerTemplateSlotMap(t, preparePath)
	// 创建 strings.xml 目标目录
	stringsDir := filepath.Join(workspace, "android", "app", "src", "main", "res", "values")
	if err := os.MkdirAll(stringsDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(strings dir) error = %v", err)
	}

	run := runRecord{
		WorkspacePath: workspace,
		AllowedPaths:  []string{"android/**"},
		TaskBundle: []appruns.TaskBundleItem{{
			TaskID:      "task-brand",
			RouteHint:   appruns.TaskRouteHintDeterministic,
			TargetPaths: []string{"android/app/src/main/res/values/strings.xml"},
		}},
	}
	roundInput := appruns.RoundInput{RoundID: "round-2", TaskBundle: run.TaskBundle}

	result, err := tryDeterministicEmit(run, roundInput, "emit-patch-2")
	if err != nil {
		t.Fatalf("tryDeterministicEmit() error = %v", err)
	}
	if !result.Handled {
		t.Fatal("expected Handled=true for emit-eligible brand task")
	}
	if result.Patch == nil || len(result.Patch.Operations) != 1 {
		t.Fatalf("expected 1 operation, got %d", len(result.Patch.Operations))
	}
	content, err := os.ReadFile(filepath.Join(stringsDir, "strings.xml"))
	if err != nil {
		t.Fatalf("ReadFile(strings.xml) error = %v", err)
	}
	if !strings.Contains(string(content), "库存管家") {
		t.Fatalf("emitted strings.xml missing app title, got: %s", string(content))
	}
}

func TestTryDeterministicEmitAndroidBuildConfigSuccess(t *testing.T) {
	jobRoot := t.TempDir()
	workspace := filepath.Join(jobRoot, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatalf("MkdirAll(workspace) error = %v", err)
	}
	preparePath := filepath.Join(jobRoot, "prepare")
	if err := os.MkdirAll(preparePath, 0o755); err != nil {
		t.Fatalf("MkdirAll(prepare) error = %v", err)
	}
	dm := appprepare.DomainModel{
		DomainCopy: appprepare.DomainCopy{Title: "项目任务协同 App"},
	}
	dmBytes, _ := json.Marshal(dm)
	if err := os.WriteFile(filepath.Join(preparePath, "domain-model.json"), dmBytes, 0o600); err != nil {
		t.Fatalf("WriteFile(domain-model.json) error = %v", err)
	}
	writeEmitRunnerTemplateSlotMap(t, preparePath)
	buildGradleDir := filepath.Join(workspace, "android", "app")
	if err := os.MkdirAll(buildGradleDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(android/app) error = %v", err)
	}

	run := runRecord{
		WorkspacePath: workspace,
		AllowedPaths:  []string{"android/**"},
		TaskBundle: []appruns.TaskBundleItem{{
			TaskID:      "task-build-config",
			RouteHint:   appruns.TaskRouteHintDeterministic,
			TargetPaths: []string{"android/app/build.gradle.kts"},
		}},
	}
	roundInput := appruns.RoundInput{RoundID: "round-build-config", TaskBundle: run.TaskBundle}

	result, err := tryDeterministicEmit(run, roundInput, "emit-patch-build-config")
	if err != nil {
		t.Fatalf("tryDeterministicEmit() error = %v", err)
	}
	if !result.Handled {
		t.Fatal("expected Handled=true for emit-eligible android build config task")
	}
	if result.Patch == nil || len(result.Patch.Operations) != 1 {
		t.Fatalf("expected 1 operation, got %d", len(result.Patch.Operations))
	}
	content, err := os.ReadFile(filepath.Join(buildGradleDir, "build.gradle.kts"))
	if err != nil {
		t.Fatalf("ReadFile(build.gradle.kts) error = %v", err)
	}
	for _, marker := range []string{"defaultOpenLiteApplicationId", `id("dev.flutter.flutter-gradle-plugin")`, `source = "../.."`} {
		if !strings.Contains(string(content), marker) {
			t.Fatalf("emitted build.gradle.kts missing %q, got: %s", marker, string(content))
		}
	}
}

func TestTryDeterministicEmitRelationRichModelsSuccess(t *testing.T) {
	testCases := []struct {
		name        string
		domainModel appprepare.DomainModel
		targetPaths []string
		wantMarkers map[string][]string
	}{
		{
			name:        "project-task-tag-model-slice",
			domainModel: appprepare.DomainModel{Entities: []appprepare.DataEntity{{EntityID: "entity-project", Name: "Project"}, {EntityID: "entity-task", Name: "Task"}, {EntityID: "entity-tag", Name: "Tag"}}},
			targetPaths: []string{"lib/models/project.dart", "lib/models/task.dart", "lib/models/tag.dart", "lib/models/task_tag_link.dart"},
			wantMarkers: map[string][]string{
				"lib/models/project.dart":       {"enum ProjectStatus {", "factory Project.fromJson(Map<String, dynamic> json)"},
				"lib/models/task.dart":          {"enum TaskStatus {", "'due_on': dueOn?.toIso8601String()"},
				"lib/models/tag.dart":           {"class Tag {", "Map<String, dynamic> toJson()"},
				"lib/models/task_tag_link.dart": {"class TaskTagLink {", "factory TaskTagLink.fromJson(Map<String, dynamic> json)"},
			},
		},
		{
			name:        "project-task-tag-summary-model",
			domainModel: appprepare.DomainModel{Entities: []appprepare.DataEntity{{EntityID: "entity-project", Name: "Project"}, {EntityID: "entity-task", Name: "Task"}, {EntityID: "entity-tag", Name: "Tag"}}},
			targetPaths: []string{"lib/models/dashboard_summary.dart"},
			wantMarkers: map[string][]string{
				"lib/models/dashboard_summary.dart": {"final int doneTaskCount;", "'tagged_task_count': taggedTaskCount"},
			},
		},
		{
			name:        "inventory-model-slice",
			domainModel: appprepare.DomainModel{Entities: []appprepare.DataEntity{{EntityID: "entity-inventory-sheet", Name: "InventorySheet"}, {EntityID: "entity-line-item", Name: "LineItem"}, {EntityID: "entity-sku", Name: "Sku"}}},
			targetPaths: []string{"lib/models/inventory_sheet.dart", "lib/models/line_item.dart", "lib/models/sku.dart", "lib/models/warehouse.dart"},
			wantMarkers: map[string][]string{
				"lib/models/inventory_sheet.dart": {"enum InventorySheetStatus {", "'counted_on': countedOn.toIso8601String()"},
				"lib/models/line_item.dart":       {"class LineItem {", "'variance_qty': varianceQty"},
				"lib/models/sku.dart":             {"class Sku {", "'reorder_threshold': reorderThreshold"},
				"lib/models/warehouse.dart":       {"class Warehouse {", "'location': location"},
			},
		},
		{
			name:        "inventory-summary-model",
			domainModel: appprepare.DomainModel{Entities: []appprepare.DataEntity{{EntityID: "entity-inventory-sheet", Name: "InventorySheet"}, {EntityID: "entity-line-item", Name: "LineItem"}, {EntityID: "entity-sku", Name: "Sku"}}},
			targetPaths: []string{"lib/models/dashboard_summary.dart"},
			wantMarkers: map[string][]string{
				"lib/models/dashboard_summary.dart": {"final int lowStockSkuCount;", "'variance_line_item_count': varianceLineItemCount"},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			jobRoot := t.TempDir()
			workspace := filepath.Join(jobRoot, "workspace")
			if err := os.MkdirAll(filepath.Join(workspace, "lib", "models"), 0o755); err != nil {
				t.Fatalf("MkdirAll(lib/models) error = %v", err)
			}
			preparePath := filepath.Join(jobRoot, "prepare")
			if err := os.MkdirAll(preparePath, 0o755); err != nil {
				t.Fatalf("MkdirAll(prepare) error = %v", err)
			}
			dmBytes, _ := json.Marshal(tc.domainModel)
			if err := os.WriteFile(filepath.Join(preparePath, "domain-model.json"), dmBytes, 0o600); err != nil {
				t.Fatalf("WriteFile(domain-model.json) error = %v", err)
			}
			writeEmitRunnerTemplateSlotMap(t, preparePath)

			run := runRecord{
				WorkspacePath: workspace,
				AllowedPaths:  []string{"lib/**"},
				TaskBundle: []appruns.TaskBundleItem{{
					TaskID:      "task-relation-models",
					RouteHint:   appruns.TaskRouteHintDeterministic,
					TargetPaths: tc.targetPaths,
				}},
			}
			roundInput := appruns.RoundInput{RoundID: "round-relation-models", TaskBundle: run.TaskBundle}

			result, err := tryDeterministicEmit(run, roundInput, "emit-patch-relation-models")
			if err != nil {
				t.Fatalf("tryDeterministicEmit() error = %v", err)
			}
			if !result.Handled {
				t.Fatal("expected Handled=true for relation-rich models task")
			}
			if got := len(result.Patch.Operations); got != len(tc.targetPaths) {
				t.Fatalf("operations len = %d, want %d", got, len(tc.targetPaths))
			}
			for path, wantMarkers := range tc.wantMarkers {
				content, err := os.ReadFile(filepath.Join(workspace, filepath.FromSlash(path)))
				if err != nil {
					t.Fatalf("ReadFile(%s) error = %v", path, err)
				}
				for _, marker := range wantMarkers {
					if !strings.Contains(string(content), marker) {
						t.Fatalf("emitted %s missing marker %q, got: %s", path, marker, string(content))
					}
				}
			}
		})
	}
}

func TestEmitGenericModelAndRepositoryUseSchemaIdentifierFields(t *testing.T) {
	dm := appprepare.DomainModel{
		TemplateID: "flutter-open-lite",
		Entities: []appprepare.DataEntity{
			{
				EntityID: "entity-course-assignment",
				Name:     "课程作业",
				Source:   "local_storage",
				Fields: []appprepare.DataField{
					{Name: "assignment_id", Type: "string", Role: "identifier", Required: true},
					{Name: "assignment_title", Type: "string", Role: "primary_text", Required: true},
					{Name: "due_date", Type: "date", Role: "due_date", Required: true},
					{Name: "status", Type: "enum[todo,inProgress,done]", Role: "status", Required: true},
					{Name: "effort_hours", Type: "number", Role: "duration", Required: false},
				},
			},
			{
				EntityID: "entity-course-assignment-summary",
				Name:     "作业摘要",
				Source:   "derived",
				Fields: []appprepare.DataField{
					{Name: "due_soon_count", Type: "int", Role: "metric_source", Required: true},
					{Name: "overdue_count", Type: "int", Role: "metric_source", Required: true},
					{Name: "done_count", Type: "int", Role: "metric_source", Required: true},
				},
			},
		},
	}

	recordContent := emitGenericRecordModel(dm, &dm.Entities[0])
	for _, marker := range []string{
		"enum CourseAssignmentStatus {",
		"String? assignmentId,",
		"assignmentId = assignmentId ?? DateTime.now().microsecondsSinceEpoch.toString()",
		"final DateTime dueDate;",
		"final double effortHours;",
	} {
		if !strings.Contains(recordContent, marker) {
			t.Fatalf("record model missing %q:\n%s", marker, recordContent)
		}
	}
	if strings.Contains(recordContent, "recordId") {
		t.Fatalf("record model should not invent recordId for schema identifier fields:\n%s", recordContent)
	}

	repositoryContent := emitGenericRepositoryContent("", dm)
	for _, marker := range []string{
		"Future<void> addRecord(CourseAssignment record)",
		"await _recordBox!.put(record.assignmentId, record);",
		"item.assignmentId == record.assignmentId",
		"record.assignmentId == recordId",
		"return CourseAssignmentSummary(",
		"dueSoonCount: records.where",
		"overdueCount: records.where",
		"doneCount: records.where",
	} {
		if !strings.Contains(repositoryContent, marker) {
			t.Fatalf("repository missing %q:\n%s", marker, repositoryContent)
		}
	}
}

func TestTryDeterministicEmitGenericSchemaSurfacesUseDomainFields(t *testing.T) {
	jobRoot := t.TempDir()
	workspace := filepath.Join(jobRoot, "workspace")
	for _, dir := range []string{
		filepath.Join(workspace, "lib", "models"),
		filepath.Join(workspace, "lib", "template"),
		filepath.Join(workspace, "lib", "repositories"),
		filepath.Join(workspace, "lib", "controllers"),
		filepath.Join(workspace, "lib", "views"),
		filepath.Join(workspace, "test"),
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("MkdirAll(%s) error = %v", dir, err)
		}
	}
	preparePath := filepath.Join(jobRoot, "prepare")
	if err := os.MkdirAll(preparePath, 0o755); err != nil {
		t.Fatalf("MkdirAll(prepare) error = %v", err)
	}
	dm := appprepare.DomainModel{
		TemplateID: "flutter-open-lite",
		DomainCopy: appprepare.DomainCopy{Title: "课程作业"},
		Entities: []appprepare.DataEntity{
			{
				EntityID: "entity-course-assignment",
				Name:     "课程作业",
				Source:   "local_storage",
				Fields: []appprepare.DataField{
					{Name: "assignment_id", Type: "string", Role: "identifier", Required: true, Description: "作业唯一标识"},
					{Name: "course_name", Type: "string", Role: "secondary_text", Required: true, Description: "课程名称"},
					{Name: "assignment_title", Type: "string", Role: "primary_text", Required: true, Description: "作业标题"},
					{Name: "due_date", Type: "date", Role: "due_date", Required: true, Description: "截止日期"},
					{Name: "status", Type: "enum[todo,inProgress,done]", Role: "status", Required: true, Description: "作业状态"},
					{Name: "note", Type: "string", Role: "note", Required: false, Description: "补充备注"},
				},
			},
			{
				EntityID: "entity-course-assignment-summary",
				Name:     "作业摘要",
				Source:   "derived",
				Fields: []appprepare.DataField{
					{Name: "due_soon_count", Type: "int", Role: "metric_source", Required: true},
					{Name: "overdue_count", Type: "int", Role: "metric_source", Required: true},
					{Name: "done_count", Type: "int", Role: "metric_source", Required: true},
				},
			},
		},
	}
	dmBytes, _ := json.Marshal(dm)
	if err := os.WriteFile(filepath.Join(preparePath, "domain-model.json"), dmBytes, 0o600); err != nil {
		t.Fatalf("WriteFile(domain-model.json) error = %v", err)
	}
	writeEmitRunnerTemplateSlotMap(t, preparePath)
	planningContextBytes, _ := json.Marshal(appprepare.PlanningContext{TemplateID: "flutter-open-lite"})
	if err := os.WriteFile(filepath.Join(preparePath, "planning-context.json"), planningContextBytes, 0o600); err != nil {
		t.Fatalf("WriteFile(planning-context.json) error = %v", err)
	}

	runTask := func(taskID string, bindingRefs, targetPaths []string) {
		t.Helper()
		task := appruns.TaskBundleItem{
			TaskID:      taskID,
			RouteHint:   appruns.TaskRouteHintDeterministic,
			TargetPaths: append([]string(nil), targetPaths...),
		}
		if len(bindingRefs) > 0 {
			task.AllocationTransition = &appruns.TaskAllocationTransition{BindingRefs: append([]string(nil), bindingRefs...)}
		}
		run := runRecord{
			WorkspacePath: workspace,
			AllowedPaths:  []string{"lib/**", "test/**"},
			TaskBundle:    []appruns.TaskBundleItem{task},
		}
		roundInput := appruns.RoundInput{RoundID: "round-" + taskID, TaskBundle: run.TaskBundle}
		result, err := tryDeterministicEmit(run, roundInput, "emit-patch-"+taskID)
		if err != nil {
			t.Fatalf("tryDeterministicEmit(%s) error = %v", taskID, err)
		}
		if !result.Handled {
			t.Fatalf("tryDeterministicEmit(%s) Handled=false", taskID)
		}
	}

	runTask("task-model", nil, []string{"lib/models/record.dart", "lib/models/dashboard_summary.dart"})
	runTask("task-copy", []string{emitRunnerBindingCopy}, []string{"lib/template/open_lite_copy.dart"})
	runTask("task-storage", []string{emitRunnerBindingStorage}, []string{"lib/repositories/record_repository.dart"})
	runTask("task-overview", []string{emitRunnerBindingOverview}, []string{"lib/views/home_page.dart", "lib/controllers/home_controller.dart"})
	runTask("task-list", []string{emitRunnerBindingCollection}, []string{"lib/views/record_list_page.dart", "lib/controllers/record_list_controller.dart"})
	runTask("task-form", []string{emitRunnerBindingMutation}, []string{"lib/views/record_form_page.dart", "lib/controllers/record_form_controller.dart"})
	runTask("task-detail", []string{emitRunnerBindingInspection}, []string{"lib/views/record_detail_page.dart"})
	runTask("task-main", []string{emitRunnerBindingAppEntry}, []string{"lib/main.dart"})
	runTask("task-test", []string{emitRunnerBindingWidgetTest}, []string{"test/widget_test.dart"})

	assertFileMarkers := func(path string, wantMarkers, forbiddenMarkers []string) {
		t.Helper()
		content, err := os.ReadFile(filepath.Join(workspace, filepath.FromSlash(path)))
		if err != nil {
			t.Fatalf("ReadFile(%s) error = %v", path, err)
		}
		for _, marker := range wantMarkers {
			if !strings.Contains(string(content), marker) {
				t.Fatalf("%s missing marker %q:\n%s", path, marker, string(content))
			}
		}
		for _, marker := range forbiddenMarkers {
			if strings.Contains(string(content), marker) {
				t.Fatalf("%s should not contain %q:\n%s", path, marker, string(content))
			}
		}
	}

	assertFileMarkers("lib/models/record.dart", []string{"class CourseAssignment", "final String assignmentId;", "final String assignmentTitle;", "final DateTime dueDate;", "enum CourseAssignmentStatus"}, []string{"recordId", "RecordStatus"})
	assertFileMarkers("lib/repositories/record_repository.dart", []string{"Future<List<CourseAssignment>> loadRecords()", "record.assignmentId", "return CourseAssignmentSummary(", "dueSoonCount: records.where", "overdueCount: records.where", "doneCount: records.where"}, []string{"record.recordId"})
	assertFileMarkers("lib/views/home_page.dart", []string{"r.assignmentTitle", "CourseAssignmentStatus.done"}, []string{"RecordStatus.done", "r.title"})
	assertFileMarkers("lib/template/open_lite_copy.dart", []string{"String get titleFieldLabel => '作业标题';", "String get categoryFieldLabel => '课程名称';", "String get dateFieldLabel => '截止日期';", "String get detailStatusLabel => '作业状态';"}, []string{"Open Lite Seed"})
	assertFileMarkers("lib/views/record_list_page.dart", []string{"record.assignmentTitle", "record.courseName", "_formatDate(record.dueDate)", "openLiteCopy.statusLabel(record.status)"}, []string{"record.title", "record.category"})
	assertFileMarkers("lib/views/record_form_page.dart", []string{"CourseAssignment(assignmentTitle: title", "courseName: _secondaryController.text.trim()", "status: _selectedStatus", "dueDate: _selectedDate", "DropdownButtonFormField<CourseAssignmentStatus>", "key: const Key('date-field')", "openLiteCopy.categoryFieldLabel", "openLiteCopy.dateFieldLabel", "note: _noteController.text.trim()"}, []string{"Record(title"})
	assertFileMarkers("lib/views/record_detail_page.dart", []string{"record.assignmentTitle", "record.courseName", "_formatDate(record.dueDate)", "openLiteCopy.statusLabel(record.status)"}, []string{"record.title", "record.category"})
	assertFileMarkers("test/widget_test.dart", []string{"import 'package:flutter_open_lite/models/record.dart';", "InMemoryRecordRepository(seedRecords: [CourseAssignment(assignmentTitle: '测试记录')])", "expect(find.text('测试记录'), findsOneWidget);"}, []string{"InMemoryRecordRepository();"})
}

func TestTryDeterministicEmitRelationRichRepositorySuccess(t *testing.T) {
	testCases := []struct {
		name            string
		workspacePath   string
		preparePath     string
		wantMarkers     []string
		forbiddenMarker string
	}{
		{
			name:          "project-task-tag-repository",
			workspacePath: createBuilderRuntimeProjectTaskTagOverviewWorkspace(t),
			wantMarkers: []string{
				"Future<List<Project>> loadProjects();",
				"Future<List<TaskTagLink>> loadTaskTagLinks();",
				"Future<List<DashboardSummary>> loadSummaries();",
				"class HiveRecordRepository extends RecordRepository",
			},
			forbiddenMarker: "loadTasksTagLinks(",
		},
		{
			name:          "inventory-repository",
			workspacePath: createBuilderRuntimeInventoryRelationRichWorkspace(t, nil),
			wantMarkers: []string{
				"Future<List<InventorySheet>> loadInventorySheets();",
				"Future<List<LineItem>> loadLineItems();",
				"Future<DashboardSummary?> loadDashboardSummary(String warehouseId) async {",
				"class HiveRecordRepository extends RecordRepository",
			},
			forbiddenMarker: "low_stock_suku_count",
		},
	}

	for index := range testCases {
		tc := testCases[index]
		t.Run(tc.name, func(t *testing.T) {
			jobRoot := filepath.Dir(tc.workspacePath)
			preparePath := filepath.Join(jobRoot, "prepare")
			if err := os.MkdirAll(preparePath, 0o755); err != nil {
				t.Fatalf("MkdirAll(prepare) error = %v", err)
			}
			var dm appprepare.DomainModel
			switch tc.name {
			case "project-task-tag-repository":
				dm = appprepare.DomainModel{Entities: []appprepare.DataEntity{{EntityID: "entity-project", Name: "Project"}, {EntityID: "entity-task", Name: "Task"}, {EntityID: "entity-tag", Name: "Tag"}}}
			default:
				dm = appprepare.DomainModel{Entities: []appprepare.DataEntity{{EntityID: "entity-inventory-sheet", Name: "InventorySheet"}, {EntityID: "entity-line-item", Name: "LineItem"}, {EntityID: "entity-sku", Name: "Sku"}}}
			}
			dmBytes, _ := json.Marshal(dm)
			if err := os.WriteFile(filepath.Join(preparePath, "domain-model.json"), dmBytes, 0o600); err != nil {
				t.Fatalf("WriteFile(domain-model.json) error = %v", err)
			}
			writeEmitRunnerTemplateSlotMap(t, preparePath)

			run := runRecord{
				WorkspacePath: tc.workspacePath,
				AllowedPaths:  []string{"lib/**"},
				TaskBundle: []appruns.TaskBundleItem{{
					TaskID:      "task-create-repository",
					RouteHint:   appruns.TaskRouteHintDeterministic,
					TargetPaths: []string{"lib/repositories/record_repository.dart"},
				}},
			}
			roundInput := appruns.RoundInput{RoundID: "round-repository", TaskBundle: run.TaskBundle}

			result, err := tryDeterministicEmit(run, roundInput, "emit-patch-repository")
			if err != nil {
				t.Fatalf("tryDeterministicEmit() error = %v", err)
			}
			if !result.Handled {
				t.Fatal("expected Handled=true for relation-rich repository task")
			}
			content, err := os.ReadFile(filepath.Join(tc.workspacePath, "lib", "repositories", "record_repository.dart"))
			if err != nil {
				t.Fatalf("ReadFile(record_repository.dart) error = %v", err)
			}
			for _, marker := range tc.wantMarkers {
				if !strings.Contains(string(content), marker) {
					t.Fatalf("emitted repository missing marker %q, got: %s", marker, string(content))
				}
			}
			if tc.forbiddenMarker != "" && strings.Contains(string(content), tc.forbiddenMarker) {
				t.Fatalf("emitted repository should remove stale marker %q, got: %s", tc.forbiddenMarker, string(content))
			}
		})
	}
}

func TestTryDeterministicEmitRelationRichMainSuccess(t *testing.T) {
	testCases := []struct {
		name          string
		workspacePath string
		domainModel   appprepare.DomainModel
		wantMarkers   []string
	}{
		{
			name:          "project-task-tag-main",
			workspacePath: createBuilderRuntimeProjectTaskTagOverviewWorkspace(t),
			domainModel:   appprepare.DomainModel{Entities: []appprepare.DataEntity{{EntityID: "entity-project", Name: "Project"}, {EntityID: "entity-task", Name: "Task"}, {EntityID: "entity-tag", Name: "Tag"}}},
			wantMarkers: []string{
				"runApp(AppFactoryApp(repository: repository));",
				"RecordFormPage(controller: widget.controller)",
				"onCreateTask: _openCreateTask,",
				"onViewAllTasks: _openViewAllTasks,",
				"onCreateRecord: _openCreateTask,",
				"onOpenTaskDetail: _openTaskDetail,",
			},
		},
		{
			name: "inventory-main",
			workspacePath: createBuilderRuntimeInventoryRelationRichWorkspace(t, map[string]string{
				"lib/controllers/home_controller.dart":        "class HomeController { HomeController({required dynamic repository}); Future<void> initialize() async {} Future<void> refresh() async {} void dispose() {} }\n",
				"lib/controllers/record_form_controller.dart": "class RecordFormController { RecordFormController({required dynamic repository, List<dynamic> warehouses = const [], List<dynamic> skus = const [], dynamic initialSheet, List<dynamic>? initialItems}); void dispose() {} }\n",
				"lib/controllers/record_list_controller.dart": "class RecordListController { RecordListController({required dynamic repository}); Future<void> refresh() async {} void dispose() {} List<dynamic> get warehouses => const []; List<dynamic> get sheets => const []; dynamic warehouseFor(String id) => null; }\n",
				"lib/views/home_page.dart":                    "class HomePage { const HomePage({required this.controller, required this.onCreateRecord, required this.onViewAllRecords}); final dynamic controller; final Future<void> Function() onCreateRecord; final Future<void> Function() onViewAllRecords; }\n",
				"lib/views/record_form_page.dart":             "class RecordFormPage { const RecordFormPage({required this.controller}); final dynamic controller; }\n",
				"lib/views/record_list_page.dart":             "class RecordListPage { const RecordListPage({required this.controller, required this.onCreateRecord, required this.onOpenRecordDetail}); final dynamic controller; final Future<void> Function() onCreateRecord; final Future<void> Function(dynamic sheet) onOpenRecordDetail; }\n",
				"lib/views/record_detail_page.dart":           "class RecordDetailPage { const RecordDetailPage({required this.sheet, required this.warehouse, required this.items, required this.skus, required this.onEdit}); final dynamic sheet; final dynamic warehouse; final List<dynamic> items; final List<dynamic> skus; final Future<void> Function(dynamic sheet) onEdit; }\n",
				"lib/template/open_lite_copy.dart":            "class _OpenLiteCopy { String get appTitle => 'inventory'; }\nconst openLiteCopy = _OpenLiteCopy();\n",
				"lib/repositories/record_repository.dart":     "abstract class RecordRepository { Future<void> init(); Future<List<dynamic>> loadWarehouses(); Future<List<dynamic>> loadSkus(); Future<List<dynamic>> loadLineItems(); } class HiveRecordRepository extends RecordRepository { @override Future<void> init() async {} @override Future<List<dynamic>> loadWarehouses() async => const []; @override Future<List<dynamic>> loadSkus() async => const []; @override Future<List<dynamic>> loadLineItems() async => const []; }\n",
			}),
			domainModel: appprepare.DomainModel{Entities: []appprepare.DataEntity{{EntityID: "entity-inventory-sheet", Name: "InventorySheet"}, {EntityID: "entity-line-item", Name: "LineItem"}, {EntityID: "entity-sku", Name: "Sku"}}},
			wantMarkers: []string{
				"runApp(AppFactoryApp(repository: repository));",
				"onCreateRecord: _openCreateRecord,",
				"onViewAllRecords: _openRecordList,",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			jobRoot := filepath.Dir(tc.workspacePath)
			preparePath := filepath.Join(jobRoot, "prepare")
			if err := os.MkdirAll(preparePath, 0o755); err != nil {
				t.Fatalf("MkdirAll(prepare) error = %v", err)
			}
			dmBytes, _ := json.Marshal(tc.domainModel)
			if err := os.WriteFile(filepath.Join(preparePath, "domain-model.json"), dmBytes, 0o600); err != nil {
				t.Fatalf("WriteFile(domain-model.json) error = %v", err)
			}
			writeEmitRunnerTemplateSlotMap(t, preparePath)
			planningContextBytes, _ := json.Marshal(appprepare.PlanningContext{TemplateID: "flutter-open-lite"})
			if err := os.WriteFile(filepath.Join(preparePath, "planning-context.json"), planningContextBytes, 0o600); err != nil {
				t.Fatalf("WriteFile(planning-context.json) error = %v", err)
			}

			run := runRecord{
				WorkspacePath: tc.workspacePath,
				AllowedPaths:  []string{"lib/**"},
				TaskBundle: []appruns.TaskBundleItem{{
					TaskID:      "task-bind-app-entry",
					RouteHint:   appruns.TaskRouteHintDeterministic,
					TargetPaths: []string{"lib/main.dart"},
				}},
			}
			roundInput := appruns.RoundInput{RoundID: "round-main", TaskBundle: run.TaskBundle}

			result, err := tryDeterministicEmit(run, roundInput, "emit-patch-main")
			if err != nil {
				t.Fatalf("tryDeterministicEmit() error = %v", err)
			}
			if !result.Handled {
				t.Fatal("expected Handled=true for relation-rich main task")
			}
			content, err := os.ReadFile(filepath.Join(tc.workspacePath, "lib", "main.dart"))
			if err != nil {
				t.Fatalf("ReadFile(main.dart) error = %v", err)
			}
			for _, marker := range tc.wantMarkers {
				if !strings.Contains(string(content), marker) {
					t.Fatalf("emitted main.dart missing marker %q, got: %s", marker, string(content))
				}
			}
		})
	}
}

func TestTryDeterministicEmitTestSuccess(t *testing.T) {
	jobRoot := t.TempDir()
	workspace := filepath.Join(jobRoot, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatalf("MkdirAll(workspace) error = %v", err)
	}
	preparePath := filepath.Join(jobRoot, "prepare")
	if err := os.MkdirAll(preparePath, 0o755); err != nil {
		t.Fatalf("MkdirAll(prepare) error = %v", err)
	}
	dm := appprepare.DomainModel{
		DomainCopy: appprepare.DomainCopy{Title: "笔记管理"},
		Entities:   []appprepare.DataEntity{{EntityID: "entity-note", Name: "Note"}},
	}
	dmBytes, _ := json.Marshal(dm)
	if err := os.WriteFile(filepath.Join(preparePath, "domain-model.json"), dmBytes, 0o600); err != nil {
		t.Fatalf("WriteFile(domain-model.json) error = %v", err)
	}
	writeEmitRunnerTemplateSlotMap(t, preparePath)
	// 创建 test 目标目录
	if err := os.MkdirAll(filepath.Join(workspace, "test"), 0o755); err != nil {
		t.Fatalf("MkdirAll(test) error = %v", err)
	}

	run := runRecord{
		WorkspacePath: workspace,
		AllowedPaths:  []string{"test/**"},
		TaskBundle: []appruns.TaskBundleItem{{
			TaskID:      "task-test",
			RouteHint:   appruns.TaskRouteHintDeterministic,
			TargetPaths: []string{"test/widget_test.dart"},
		}},
	}
	roundInput := appruns.RoundInput{RoundID: "round-3", TaskBundle: run.TaskBundle}

	result, err := tryDeterministicEmit(run, roundInput, "emit-patch-3")
	if err != nil {
		t.Fatalf("tryDeterministicEmit() error = %v", err)
	}
	if !result.Handled {
		t.Fatal("expected Handled=true for emit-eligible test task")
	}
	content, err := os.ReadFile(filepath.Join(workspace, "test", "widget_test.dart"))
	if err != nil {
		t.Fatalf("ReadFile(widget_test.dart) error = %v", err)
	}
	if !strings.Contains(string(content), "flutter_test") || !strings.Contains(string(content), "testWidgets") {
		t.Fatalf("emitted widget_test.dart missing expected content, got: %s", string(content))
	}
}

func TestTryDeterministicEmitTestRelationRichMinimalDomainModelHandled(t *testing.T) {
	testCases := []struct {
		name        string
		domainModel appprepare.DomainModel
		wantMarkers []string
	}{
		{
			name: "project-task-tag",
			domainModel: appprepare.DomainModel{
				Entities: []appprepare.DataEntity{
					{EntityID: "entity-project", Name: "Project"},
					{EntityID: "entity-task", Name: "Task"},
					{EntityID: "entity-tag", Name: "Tag"},
				},
			},
			wantMarkers: []string{
				"AppFactoryApp(repository: repository)",
				"models/project.dart",
				"models/task.dart",
				"models/tag.dart",
			},
		},
		{
			name: "inventory-sheet-line-item",
			domainModel: appprepare.DomainModel{
				Entities: []appprepare.DataEntity{
					{EntityID: "entity-inventory-sheet", Name: "InventorySheet"},
					{EntityID: "entity-line-item", Name: "LineItem"},
					{EntityID: "entity-sku", Name: "Sku"},
				},
			},
			wantMarkers: []string{
				"AppFactoryApp(repository: repository)",
				"models/inventory_sheet.dart",
				"models/line_item.dart",
				"models/sku.dart",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			jobRoot := t.TempDir()
			workspace := filepath.Join(jobRoot, "workspace")
			if err := os.MkdirAll(workspace, 0o755); err != nil {
				t.Fatalf("MkdirAll(workspace) error = %v", err)
			}
			preparePath := filepath.Join(jobRoot, "prepare")
			if err := os.MkdirAll(preparePath, 0o755); err != nil {
				t.Fatalf("MkdirAll(prepare) error = %v", err)
			}
			dmBytes, _ := json.Marshal(tc.domainModel)
			if err := os.WriteFile(filepath.Join(preparePath, "domain-model.json"), dmBytes, 0o600); err != nil {
				t.Fatalf("WriteFile(domain-model.json) error = %v", err)
			}
			writeEmitRunnerTemplateSlotMap(t, preparePath)
			if err := os.MkdirAll(filepath.Join(workspace, "test"), 0o755); err != nil {
				t.Fatalf("MkdirAll(test) error = %v", err)
			}

			run := runRecord{
				WorkspacePath: workspace,
				AllowedPaths:  []string{"test/**"},
				TaskBundle: []appruns.TaskBundleItem{{
					TaskID:      "task-test",
					RouteHint:   appruns.TaskRouteHintDeterministic,
					TargetPaths: []string{"test/widget_test.dart"},
				}},
			}
			roundInput := appruns.RoundInput{RoundID: "round-test-relation-rich", TaskBundle: run.TaskBundle}

			result, err := tryDeterministicEmit(run, roundInput, "emit-patch-test-relation-rich")
			if err != nil {
				t.Fatalf("tryDeterministicEmit() error = %v", err)
			}
			if !result.Handled {
				t.Fatal("expected Handled=true for minimal relation-rich test task")
			}
			content, err := os.ReadFile(filepath.Join(workspace, "test", "widget_test.dart"))
			if err != nil {
				t.Fatalf("ReadFile(widget_test.dart) error = %v", err)
			}
			for _, marker := range tc.wantMarkers {
				if !strings.Contains(string(content), marker) {
					t.Fatalf("emitted widget_test.dart missing marker %q, got: %s", marker, string(content))
				}
			}
		})
	}
}

func TestTryDeterministicEmitMultipleTargets(t *testing.T) {
	jobRoot := t.TempDir()
	workspace := filepath.Join(jobRoot, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatalf("MkdirAll(workspace) error = %v", err)
	}
	preparePath := filepath.Join(jobRoot, "prepare")
	if err := os.MkdirAll(preparePath, 0o755); err != nil {
		t.Fatalf("MkdirAll(prepare) error = %v", err)
	}
	dm := appprepare.DomainModel{
		DomainCopy: appprepare.DomainCopy{Title: "任务管理"},
		Entities: []appprepare.DataEntity{
			{EntityID: "entity-project", Name: "Project"},
			{EntityID: "entity-task", Name: "Task"},
			{EntityID: "entity-tag", Name: "Tag"},
		},
	}
	dmBytes, _ := json.Marshal(dm)
	if err := os.WriteFile(filepath.Join(preparePath, "domain-model.json"), dmBytes, 0o600); err != nil {
		t.Fatalf("WriteFile(domain-model.json) error = %v", err)
	}
	writeEmitRunnerTemplateSlotMap(t, preparePath)
	// 创建目标目录
	if err := os.MkdirAll(filepath.Join(workspace, "lib", "template"), 0o755); err != nil {
		t.Fatalf("MkdirAll(lib/template) error = %v", err)
	}
	stringsDir := filepath.Join(workspace, "android", "app", "src", "main", "res", "values")
	if err := os.MkdirAll(stringsDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(strings dir) error = %v", err)
	}

	run := runRecord{
		WorkspacePath: workspace,
		AllowedPaths:  []string{"lib/**", "android/**"},
		TaskBundle: []appruns.TaskBundleItem{{
			TaskID:    "task-combined",
			RouteHint: appruns.TaskRouteHintDeterministic,
			TargetPaths: []string{
				"lib/template/open_lite_copy.dart",
				"android/app/src/main/res/values/strings.xml",
			},
		}},
	}
	roundInput := appruns.RoundInput{RoundID: "round-4", TaskBundle: run.TaskBundle}

	result, err := tryDeterministicEmit(run, roundInput, "emit-patch-4")
	if err != nil {
		t.Fatalf("tryDeterministicEmit() error = %v", err)
	}
	if !result.Handled {
		t.Fatal("expected Handled=true for multi-target emit task")
	}
	if len(result.Patch.Operations) != 2 {
		t.Fatalf("expected 2 operations, got %d", len(result.Patch.Operations))
	}
}

func TestTryDeterministicEmitNonDeterministicTaskSkips(t *testing.T) {
	run := runRecord{
		WorkspacePath: t.TempDir(),
		TaskBundle: []appruns.TaskBundleItem{{
			TaskID:      "task-model",
			RouteHint:   appruns.TaskRouteHintDefaultModel,
			TargetPaths: []string{"lib/models/record.dart"},
		}},
	}
	roundInput := appruns.RoundInput{RoundID: "round-5", TaskBundle: run.TaskBundle}

	result, err := tryDeterministicEmit(run, roundInput, "emit-patch-5")
	if err != nil {
		t.Fatalf("tryDeterministicEmit() error = %v", err)
	}
	if result.Handled {
		t.Fatal("expected Handled=false for non-deterministic task")
	}
}

func TestTryDeterministicEmitNoDomainModelFails(t *testing.T) {
	jobRoot := t.TempDir()
	workspace := filepath.Join(jobRoot, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatalf("MkdirAll(workspace) error = %v", err)
	}
	// 没有创建 prepare/domain-model.json

	run := runRecord{
		WorkspacePath: workspace,
		TaskBundle: []appruns.TaskBundleItem{{
			TaskID:      "task-copy",
			RouteHint:   appruns.TaskRouteHintDeterministic,
			TargetPaths: []string{"lib/template/open_lite_copy.dart"},
		}},
	}
	roundInput := appruns.RoundInput{RoundID: "round-6", TaskBundle: run.TaskBundle}

	_, err := tryDeterministicEmit(run, roundInput, "emit-patch-6")
	if err == nil {
		t.Fatal("expected error when domain-model.json is missing")
	}
	if !strings.Contains(err.Error(), "domain model") {
		t.Fatalf("error = %v, want domain model related error", err)
	}
}

func TestTryDeterministicEmitNoMatchingEmitterReturnsUnhandled(t *testing.T) {
	jobRoot := t.TempDir()
	workspace := filepath.Join(jobRoot, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatalf("MkdirAll(workspace) error = %v", err)
	}
	preparePath := filepath.Join(jobRoot, "prepare")
	if err := os.MkdirAll(preparePath, 0o755); err != nil {
		t.Fatalf("MkdirAll(prepare) error = %v", err)
	}
	dm := appprepare.DomainModel{
		DomainCopy: appprepare.DomainCopy{Title: "体重追踪"},
		Entities:   []appprepare.DataEntity{{EntityID: "entity-weight-record", Name: "WeightRecord"}},
	}
	dmBytes, _ := json.Marshal(dm)
	if err := os.WriteFile(filepath.Join(preparePath, "domain-model.json"), dmBytes, 0o600); err != nil {
		t.Fatalf("WriteFile(domain-model.json) error = %v", err)
	}
	writeEmitRunnerTemplateSlotMap(t, preparePath)

	run := runRecord{
		WorkspacePath: workspace,
		TaskBundle: []appruns.TaskBundleItem{{
			TaskID:      "task-unknown",
			RouteHint:   appruns.TaskRouteHintDeterministic,
			TargetPaths: []string{"lib/models/unknown.dart"}, // generic 模型生成器不覆盖未知目标路径
		}},
	}
	roundInput := appruns.RoundInput{RoundID: "round-7", TaskBundle: run.TaskBundle}

	result, err := tryDeterministicEmit(run, roundInput, "emit-patch-7")
	if err != nil {
		t.Fatalf("tryDeterministicEmit() error = %v", err)
	}
	if result.Handled {
		t.Fatal("expected Handled=false when no emitter matches target paths")
	}
}

func TestTryDeterministicEmitUsesSlotKindRoutingInsteadOfTargetPaths(t *testing.T) {
	jobRoot := t.TempDir()
	workspace := filepath.Join(jobRoot, "workspace")
	if err := os.MkdirAll(filepath.Join(workspace, "lib", "template"), 0o755); err != nil {
		t.Fatalf("MkdirAll(lib/template) error = %v", err)
	}
	if err := os.MkdirAll(filepath.Join(workspace, "test"), 0o755); err != nil {
		t.Fatalf("MkdirAll(test) error = %v", err)
	}
	preparePath := filepath.Join(jobRoot, "prepare")
	if err := os.MkdirAll(preparePath, 0o755); err != nil {
		t.Fatalf("MkdirAll(prepare) error = %v", err)
	}
	dm := appprepare.DomainModel{
		DomainCopy: appprepare.DomainCopy{Title: "任务档案"},
		Entities:   []appprepare.DataEntity{{EntityID: "entity-task", Name: "Task"}},
	}
	dmBytes, _ := json.Marshal(dm)
	if err := os.WriteFile(filepath.Join(preparePath, "domain-model.json"), dmBytes, 0o600); err != nil {
		t.Fatalf("WriteFile(domain-model.json) error = %v", err)
	}
	writeEmitRunnerTemplateSlotMap(t, preparePath,
		emitRunnerTemplateSlot(emitRunnerBindingCopy, "copy-bound-to-test-slot", "test", "synchronize", true, "lib/template/open_lite_copy.dart", "test/widget_test.dart"),
	)

	run := runRecord{
		WorkspacePath: workspace,
		AllowedPaths:  []string{"lib/**", "test/**"},
		TaskBundle: []appruns.TaskBundleItem{{
			TaskID:      "task-copy-prefers-slot-kind",
			RouteHint:   appruns.TaskRouteHintDeterministic,
			TargetPaths: []string{"lib/template/open_lite_copy.dart", "test/widget_test.dart"},
			AllocationTransition: &appruns.TaskAllocationTransition{
				BindingRefs: []string{emitRunnerBindingCopy},
			},
		}},
	}
	roundInput := appruns.RoundInput{RoundID: "round-slot-kind", TaskBundle: run.TaskBundle}

	result, err := tryDeterministicEmit(run, roundInput, "emit-patch-slot-kind")
	if err != nil {
		t.Fatalf("tryDeterministicEmit() error = %v", err)
	}
	if !result.Handled {
		t.Fatal("expected Handled=true when slot kind routes to test emitter")
	}
	if len(result.Patch.Operations) != 1 {
		t.Fatalf("operations len = %d, want 1", len(result.Patch.Operations))
	}
	if result.Patch.Operations[0].Path != "test/widget_test.dart" {
		t.Fatalf("operation path = %q, want test/widget_test.dart", result.Patch.Operations[0].Path)
	}
	if _, err := os.Stat(filepath.Join(workspace, "lib", "template", "open_lite_copy.dart")); !os.IsNotExist(err) {
		t.Fatalf("open_lite_copy.dart should not be emitted when slot kind is test, stat err = %v", err)
	}
}

func TestTryDeterministicEmitFallsBackWhenSlotIsNotEmitEligible(t *testing.T) {
	jobRoot := t.TempDir()
	workspace := filepath.Join(jobRoot, "workspace")
	if err := os.MkdirAll(filepath.Join(workspace, "lib", "template"), 0o755); err != nil {
		t.Fatalf("MkdirAll(lib/template) error = %v", err)
	}
	preparePath := filepath.Join(jobRoot, "prepare")
	if err := os.MkdirAll(preparePath, 0o755); err != nil {
		t.Fatalf("MkdirAll(prepare) error = %v", err)
	}
	dm := appprepare.DomainModel{
		DomainCopy: appprepare.DomainCopy{Title: "体重追踪"},
		Entities:   []appprepare.DataEntity{{EntityID: "entity-weight-record", Name: "WeightRecord"}},
	}
	dmBytes, _ := json.Marshal(dm)
	if err := os.WriteFile(filepath.Join(preparePath, "domain-model.json"), dmBytes, 0o600); err != nil {
		t.Fatalf("WriteFile(domain-model.json) error = %v", err)
	}
	writeEmitRunnerTemplateSlotMap(t, preparePath,
		emitRunnerTemplateSlot(emitRunnerBindingCopy, "copy-disabled-slot", "copy", "synchronize", false, "lib/template/open_lite_copy.dart"),
	)

	run := runRecord{
		WorkspacePath: workspace,
		AllowedPaths:  []string{"lib/**"},
		TaskBundle: []appruns.TaskBundleItem{{
			TaskID:      "task-copy-disabled",
			RouteHint:   appruns.TaskRouteHintDeterministic,
			TargetPaths: []string{"lib/template/open_lite_copy.dart"},
			AllocationTransition: &appruns.TaskAllocationTransition{
				BindingRefs: []string{emitRunnerBindingCopy},
			},
		}},
	}
	roundInput := appruns.RoundInput{RoundID: "round-slot-disabled", TaskBundle: run.TaskBundle}

	result, err := tryDeterministicEmit(run, roundInput, "emit-patch-slot-disabled")
	if err != nil {
		t.Fatalf("tryDeterministicEmit() error = %v", err)
	}
	if result.Handled {
		t.Fatal("expected Handled=false when matched slot is not emit eligible")
	}
	if _, err := os.Stat(filepath.Join(workspace, "lib", "template", "open_lite_copy.dart")); !os.IsNotExist(err) {
		t.Fatalf("open_lite_copy.dart should not be written when slot is not emit eligible, stat err = %v", err)
	}
}

func TestSelectBuilderRuntimeRoutePreservesDeterministicEmptyModel(t *testing.T) {
	run := runRecord{
		TaskBundle: []appruns.TaskBundleItem{{
			TaskID:      "task-copy",
			TaskType:    appruns.BuilderRuntimeTaskTypeDualFileWiring,
			RouteHint:   appruns.TaskRouteHintDeterministic,
			TargetPaths: []string{"lib/template/open_lite_copy.dart"},
		}},
		BuilderRuntime: &appruns.BuilderRuntimePlan{
			Enabled:      true,
			DefaultModel: appruns.BuilderRuntimeModelRef{Primary: "qwen2.5-coder-14b-local"},
			TaskRoutes: []appruns.BuilderRuntimeTaskRoute{{
				TaskID:      "task-copy",
				TaskType:    appruns.BuilderRuntimeTaskTypeDualFileWiring,
				RouteSource: "route_hint",
			}},
		},
	}

	route, upgraded := selectBuilderRuntimeRoute(run)
	if upgraded {
		t.Fatal("expected no upgrade for deterministic route")
	}
	if route.RouteSource != "route_hint" {
		t.Fatalf("route_source = %q, want route_hint", route.RouteSource)
	}
	if route.Model.Primary != "" {
		t.Fatalf("model.primary = %q, want empty (deterministic tasks skip model)", route.Model.Primary)
	}
}

func TestShouldUseBuilderRuntimeAcceptsDeterministicRoute(t *testing.T) {
	runner := &Runner{
		PatchGenerator: &stubBuilderRuntimePatchGenerator{},
	}
	run := runRecord{
		TaskBundle: []appruns.TaskBundleItem{{
			TaskID:    "task-copy",
			RouteHint: appruns.TaskRouteHintDeterministic,
		}},
		BuilderRuntime: &appruns.BuilderRuntimePlan{
			Enabled: true,
			TaskRoutes: []appruns.BuilderRuntimeTaskRoute{{
				TaskID:      "task-copy",
				RouteSource: "route_hint",
			}},
		},
	}
	if !runner.shouldUseBuilderRuntime(run) {
		t.Fatal("shouldUseBuilderRuntime() = false, want true for deterministic route_hint")
	}
}

// TestEmitOutputIdempotentUnderCanonicalization 验证 emit 产物经过 canonicalization 后保持不变。
// 这是 M5.1 的核心保证：emit 路径完全替代 canonicalization，两者产物一致。
func TestEmitOutputIdempotentUnderCanonicalization(t *testing.T) {
	// 构造 project-task-tag workspace
	jobRoot := t.TempDir()
	workspace := filepath.Join(jobRoot, "workspace")
	preparePath := filepath.Join(jobRoot, "prepare")
	for _, dir := range []string{
		filepath.Join(workspace, "lib", "template"),
		filepath.Join(workspace, "lib", "models"),
		filepath.Join(workspace, "lib", "views"),
		filepath.Join(workspace, "lib", "controllers"),
		preparePath,
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("MkdirAll error: %v", err)
		}
	}

	dm := appprepare.DomainModel{
		DomainCopy: appprepare.DomainCopy{Title: "项目任务协同"},
		Entities: []appprepare.DataEntity{
			{EntityID: "entity-project", Name: "Project"},
			{EntityID: "entity-task", Name: "Task"},
			{EntityID: "entity-tag", Name: "Tag"},
		},
	}
	dmBytes, _ := json.Marshal(dm)
	if err := os.WriteFile(filepath.Join(preparePath, "domain-model.json"), dmBytes, 0o600); err != nil {
		t.Fatalf("WriteFile(domain-model.json) error = %v", err)
	}
	writeEmitRunnerTemplateSlotMap(t, preparePath)

	// 放置 model 文件使 profile 检测为 project-task-tag
	modelStubs := map[string]string{
		"lib/models/project.dart":       "class Project {}",
		"lib/models/task.dart":          "class Task {}",
		"lib/models/tag.dart":           "class Tag {}",
		"lib/models/task_tag_link.dart": "class TaskTagLink {}",
	}
	for path, content := range modelStubs {
		if err := os.WriteFile(filepath.Join(workspace, filepath.FromSlash(path)), []byte(content), 0o600); err != nil {
			t.Fatalf("WriteFile(%s) error = %v", path, err)
		}
	}

	// 执行 emit
	allTargets := []string{
		"lib/template/open_lite_copy.dart",
		"lib/views/home_page.dart",
		"lib/controllers/home_controller.dart",
		"lib/views/record_list_page.dart",
		"lib/controllers/record_list_controller.dart",
		"lib/views/record_form_page.dart",
		"lib/controllers/record_form_controller.dart",
		"lib/views/record_detail_page.dart",
	}
	run := runRecord{
		RunID:         "run-idempotent",
		WorkspacePath: workspace,
		AllowedPaths:  []string{"lib/**", "test/**", "android/**"},
		TaskBundle: []appruns.TaskBundleItem{{
			TaskID:      "task-all",
			RouteHint:   appruns.TaskRouteHintDeterministic,
			TargetPaths: allTargets,
		}},
	}
	roundInput := appruns.RoundInput{
		RoundID:    "round-idempotent",
		TaskBundle: run.TaskBundle,
	}

	result, err := tryDeterministicEmit(run, roundInput, "emit-patch-idempotent")
	if err != nil {
		t.Fatalf("tryDeterministicEmit() error = %v", err)
	}
	if !result.Handled {
		t.Fatal("expected Handled=true")
	}

	// 收集 emit 产出的文件内容
	emittedFiles := make(map[string]string)
	for _, op := range result.Patch.Operations {
		emittedFiles[filepath.ToSlash(op.Path)] = op.Content
	}

	// 对 view 文件验证幂等性：通过 normalizeBuilderRuntimeViewContent 后不应变化
	viewFiles := []string{
		"lib/views/home_page.dart",
		"lib/views/record_list_page.dart",
		"lib/views/record_form_page.dart",
		"lib/views/record_detail_page.dart",
	}
	for _, path := range viewFiles {
		content, ok := emittedFiles[path]
		if !ok {
			t.Fatalf("emit did not produce %s", path)
		}
		normalized := normalizeBuilderRuntimeViewContent(workspace, true, false, content)
		if normalized != content {
			t.Errorf("normalizeBuilderRuntimeViewContent changed emit output for %s\n--- emit ---\n%s\n--- normalized ---\n%s",
				path, content[:min(200, len(content))], normalized[:min(200, len(normalized))])
		}
	}

	// 对 controller 文件验证幂等性
	controllerChecks := []struct {
		path      string
		normalize func(string) string
	}{
		{"lib/controllers/home_controller.dart", func(c string) string {
			return normalizeBuilderRuntimeHomeControllerContent(workspace, c)
		}},
		{"lib/controllers/record_form_controller.dart", func(c string) string {
			return normalizeBuilderRuntimeRecordFormControllerContent(workspace, c)
		}},
		{"lib/controllers/record_list_controller.dart", func(c string) string {
			return normalizeBuilderRuntimeRecordListControllerContent(workspace, true, false, c)
		}},
	}
	for _, check := range controllerChecks {
		content, ok := emittedFiles[check.path]
		if !ok {
			t.Fatalf("emit did not produce %s", check.path)
		}
		normalized := check.normalize(content)
		if normalized != content {
			t.Errorf("canonicalization changed emit output for %s\n--- emit ---\n%s\n--- normalized ---\n%s",
				check.path, content[:min(200, len(content))], normalized[:min(200, len(normalized))])
		}
	}

	// copy 文件幂等性
	if copyContent, ok := emittedFiles["lib/template/open_lite_copy.dart"]; ok {
		normalized := normalizeBuilderRuntimeOpenLiteCopyHelperReferences(copyContent)
		if normalized != copyContent {
			t.Errorf("normalizeBuilderRuntimeOpenLiteCopyHelperReferences changed emit copy output")
		}
	}
}

func TestExecuteBuilderRuntimeEditDeterministicEmitBypassesLLM(t *testing.T) {
	jobRoot := t.TempDir()
	workspace := filepath.Join(jobRoot, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatalf("MkdirAll(workspace) error = %v", err)
	}
	preparePath := filepath.Join(jobRoot, "prepare")
	if err := os.MkdirAll(preparePath, 0o755); err != nil {
		t.Fatalf("MkdirAll(prepare) error = %v", err)
	}
	dm := appprepare.DomainModel{
		DomainCopy: appprepare.DomainCopy{Title: "体重追踪"},
		Entities:   []appprepare.DataEntity{{EntityID: "entity-weight", Name: "WeightRecord"}},
	}
	dmBytes, _ := json.Marshal(dm)
	if err := os.WriteFile(filepath.Join(preparePath, "domain-model.json"), dmBytes, 0o600); err != nil {
		t.Fatalf("WriteFile(domain-model.json) error = %v", err)
	}
	writeEmitRunnerTemplateSlotMap(t, preparePath)
	if err := os.MkdirAll(filepath.Join(workspace, "lib", "template"), 0o755); err != nil {
		t.Fatalf("MkdirAll(lib/template) error = %v", err)
	}

	// stubPatchGenerator 会记录是否被调用——deterministic 路径不应调用它
	generator := &stubBuilderRuntimePatchGenerator{
		response: BuilderRuntimePatchResponse{Content: `{"patch_id":"should-not-be-used","operations":[]}`},
	}
	runner := &Runner{PatchGenerator: generator}

	run := runRecord{
		RunID:         "run-emit",
		WorkspacePath: workspace,
		AllowedPaths:  []string{"lib/**"},
		TaskBundle: []appruns.TaskBundleItem{{
			TaskID:      "task-copy",
			TaskType:    appruns.BuilderRuntimeTaskTypeDualFileWiring,
			RouteHint:   appruns.TaskRouteHintDeterministic,
			TargetPaths: []string{"lib/template/open_lite_copy.dart"},
		}},
		BuilderRuntime: &appruns.BuilderRuntimePlan{
			Enabled: true,
			TaskRoutes: []appruns.BuilderRuntimeTaskRoute{{
				TaskID:      "task-copy",
				TaskType:    appruns.BuilderRuntimeTaskTypeDualFileWiring,
				RouteSource: "route_hint",
				// Model 留空——确定性任务不需要
			}},
		},
		LogPath: filepath.Join(t.TempDir(), "builder-runtime.log"),
	}
	roundInput := appruns.RoundInput{
		RoundID:    "round-emit",
		Attempt:    1,
		TaskBundle: run.TaskBundle,
		BuilderRuntime: &appruns.BuilderRuntimePlan{
			Enabled: true,
			TaskRoutes: []appruns.BuilderRuntimeTaskRoute{{
				TaskID:      "task-copy",
				TaskType:    appruns.BuilderRuntimeTaskTypeDualFileWiring,
				RouteSource: "route_hint",
			}},
		},
	}
	step := ExecutionStep{StepID: "thin-prepare", Stage: appruns.StageThinPrepare}
	backend := noopRunnerBackend{}

	result, err := runner.executeBuilderRuntimeEdit(context.Background(), backend, run.RunID, step, run, roundInput)
	if err != nil {
		t.Fatalf("executeBuilderRuntimeEdit() error = %v", err)
	}
	if result == nil || result.Patch == nil {
		t.Fatal("expected non-nil patch from deterministic emit")
	}
	if result.Stats.Mode != "deterministic_emit" {
		t.Fatalf("stats.mode = %q, want deterministic_emit", result.Stats.Mode)
	}
	// 验证文件已写入
	content, err := os.ReadFile(filepath.Join(workspace, "lib", "template", "open_lite_copy.dart"))
	if err != nil {
		t.Fatalf("ReadFile(open_lite_copy.dart) error = %v", err)
	}
	if !strings.Contains(string(content), "体重追踪") {
		t.Fatalf("emitted content missing app title, got: %s", string(content))
	}
}

// TestEmitOutputIdempotentUnderCanonicalizationInventory 验证 inventory profile 的 emit 产物幂等性。
func TestEmitOutputIdempotentUnderCanonicalizationInventory(t *testing.T) {
	jobRoot := t.TempDir()
	workspace := filepath.Join(jobRoot, "workspace")
	preparePath := filepath.Join(jobRoot, "prepare")
	for _, dir := range []string{
		filepath.Join(workspace, "lib", "template"),
		filepath.Join(workspace, "lib", "models"),
		filepath.Join(workspace, "lib", "views"),
		filepath.Join(workspace, "lib", "controllers"),
		preparePath,
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("MkdirAll error: %v", err)
		}
	}

	dm := appprepare.DomainModel{
		DomainCopy: appprepare.DomainCopy{Title: "库存盘点工作台"},
		Entities: []appprepare.DataEntity{
			{EntityID: "entity-inventory-sheet", Name: "InventorySheet"},
			{EntityID: "entity-line-item", Name: "LineItem"},
			{EntityID: "entity-sku", Name: "Sku"},
			{EntityID: "entity-warehouse", Name: "Warehouse"},
		},
	}
	dmBytes, _ := json.Marshal(dm)
	if err := os.WriteFile(filepath.Join(preparePath, "domain-model.json"), dmBytes, 0o600); err != nil {
		t.Fatalf("WriteFile(domain-model.json) error = %v", err)
	}
	writeEmitRunnerTemplateSlotMap(t, preparePath)

	modelStubs := map[string]string{
		"lib/models/inventory_sheet.dart": "class InventorySheet {}",
		"lib/models/line_item.dart":       "class LineItem {}",
		"lib/models/sku.dart":             "class Sku {}",
		"lib/models/warehouse.dart":       "class Warehouse {}",
	}
	for path, content := range modelStubs {
		if err := os.WriteFile(filepath.Join(workspace, filepath.FromSlash(path)), []byte(content), 0o600); err != nil {
			t.Fatalf("WriteFile(%s) error = %v", path, err)
		}
	}

	allTargets := []string{
		"lib/template/open_lite_copy.dart",
		"lib/views/home_page.dart",
		"lib/controllers/home_controller.dart",
		"lib/views/record_list_page.dart",
		"lib/controllers/record_list_controller.dart",
		"lib/views/record_form_page.dart",
		"lib/controllers/record_form_controller.dart",
		"lib/views/record_detail_page.dart",
	}
	run := runRecord{
		RunID:         "run-idempotent-inv",
		WorkspacePath: workspace,
		AllowedPaths:  []string{"lib/**", "test/**", "android/**"},
		TaskBundle: []appruns.TaskBundleItem{{
			TaskID:      "task-all",
			RouteHint:   appruns.TaskRouteHintDeterministic,
			TargetPaths: allTargets,
		}},
	}
	roundInput := appruns.RoundInput{
		RoundID:    "round-idempotent-inv",
		TaskBundle: run.TaskBundle,
	}

	result, err := tryDeterministicEmit(run, roundInput, "emit-patch-idempotent-inv")
	if err != nil {
		t.Fatalf("tryDeterministicEmit() error = %v", err)
	}
	if !result.Handled {
		t.Fatal("expected Handled=true")
	}

	emittedFiles := make(map[string]string)
	for _, op := range result.Patch.Operations {
		emittedFiles[filepath.ToSlash(op.Path)] = op.Content
	}

	viewFiles := []string{
		"lib/views/home_page.dart",
		"lib/views/record_list_page.dart",
		"lib/views/record_form_page.dart",
		"lib/views/record_detail_page.dart",
	}
	for _, path := range viewFiles {
		content, ok := emittedFiles[path]
		if !ok {
			t.Fatalf("emit did not produce %s", path)
		}
		normalized := normalizeBuilderRuntimeViewContent(workspace, true, false, content)
		if normalized != content {
			// 逐行比较找出所有不同行（最多 3 个）
			emitLines := strings.Split(content, "\n")
			normLines := strings.Split(normalized, "\n")
			diffCount := 0
			for i := 0; i < len(emitLines) || i < len(normLines); i++ {
				var el, nl string
				if i < len(emitLines) {
					el = emitLines[i]
				}
				if i < len(normLines) {
					nl = normLines[i]
				}
				if el != nl {
					t.Errorf("normalizeBuilderRuntimeViewContent changed emit output for %s at line %d\n--- emit line ---\n%q\n--- norm line ---\n%q",
						path, i+1, el, nl)
					diffCount++
					if diffCount >= 3 {
						break
					}
				}
			}
			if len(emitLines) != len(normLines) {
				t.Errorf("  line count changed: emit=%d, norm=%d", len(emitLines), len(normLines))
			}
		}
	}

	controllerChecks := []struct {
		path      string
		normalize func(string) string
	}{
		{"lib/controllers/home_controller.dart", func(c string) string {
			return normalizeBuilderRuntimeHomeControllerContent(workspace, c)
		}},
		{"lib/controllers/record_form_controller.dart", func(c string) string {
			return normalizeBuilderRuntimeRecordFormControllerContent(workspace, c)
		}},
		{"lib/controllers/record_list_controller.dart", func(c string) string {
			return normalizeBuilderRuntimeRecordListControllerContent(workspace, true, false, c)
		}},
	}
	for _, check := range controllerChecks {
		content, ok := emittedFiles[check.path]
		if !ok {
			t.Fatalf("emit did not produce %s", check.path)
		}
		normalized := check.normalize(content)
		if normalized != content {
			t.Errorf("canonicalization changed emit output for %s\n--- emit ---\n%s\n--- normalized ---\n%s",
				check.path, content[:min(200, len(content))], normalized[:min(200, len(normalized))])
		}
	}

	if copyContent, ok := emittedFiles["lib/template/open_lite_copy.dart"]; ok {
		normalized := normalizeBuilderRuntimeOpenLiteCopyHelperReferences(copyContent)
		if normalized != copyContent {
			t.Errorf("normalizeBuilderRuntimeOpenLiteCopyHelperReferences changed emit copy output")
		}
	}
}
