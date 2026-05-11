package emitter

import (
	"strings"
	"testing"

	appprepare "github.com/sipeed/oneappfactory/pkg/appfactory/prepare"
)

func TestEmitAppEntryProjectTaskTag(t *testing.T) {
	dm := appprepare.DomainModel{
		DomainCopy: appprepare.DomainCopy{Title: "项目任务协同 App"},
		Entities: []appprepare.DataEntity{
			{EntityID: "entity-project", Name: "Project"},
			{EntityID: "entity-task", Name: "Task"},
			{EntityID: "entity-tag", Name: "Tag"},
		},
	}
	pc := appprepare.PlanningContext{TemplateID: "flutter-open-lite"}
	result, ok := EmitAppEntry(dm, pc)
	if !ok {
		t.Fatal("EmitAppEntry() returned false for project-task-tag profile")
	}
	if result.FilePath != "lib/main.dart" {
		t.Fatalf("FilePath = %q, want lib/main.dart", result.FilePath)
	}
	for _, marker := range []string{
		"Future<void> main() async {",
		"runApp(AppFactoryApp(repository: repository));",
		"class AppFactoryApp extends StatefulWidget",
		"final RecordRepository repository;",
		"Future<List<Project>> _loadProjects() async {",
		"Future<List<Tag>> _loadTags() async {",
		"Future<List<String>> _loadTaskTagIDs(String taskId) async {",
		"Future<void> _openTaskForm({Task? initialTask}) async {",
		"RecordFormController(",
		"projects: projects,",
		"tags: tags,",
		"initialTagIds: initialTagIDs,",
		"Future<void> _openTaskDetail(Task task) async {",
		"RecordDetailPage(",
		"taskTags: taskTagIDs,",
		"onEdit: _openEditTask,",
		"Future<void> _openTaskList() async {",
		"onCreateRecord: _openCreateTask,",
		"onOpenTaskDetail: _openTaskDetail,",
		"onCreateTask: _openCreateTask,",
		"onViewAllTasks: _openViewAllTasks,",
		"return RecordFormPage(controller: widget.controller);",
	} {
		if !strings.Contains(result.Content, marker) {
			t.Fatalf("content missing project-task-tag marker: %s", marker)
		}
	}
	for _, absent := range []string{
		"runApp(FlutterOpenLiteApp(repository: repository));",
		"class FlutterOpenLiteApp extends StatelessWidget",
		"class FlutterOpenLiteShell extends StatefulWidget",
	} {
		if strings.Contains(result.Content, absent) {
			t.Fatalf("content should not contain stale project-task-tag marker: %s", absent)
		}
	}
}

func TestEmitAppEntryInventory(t *testing.T) {
	dm := appprepare.DomainModel{
		DomainCopy: appprepare.DomainCopy{Title: "库存管家"},
		Entities: []appprepare.DataEntity{
			{EntityID: "entity-inventory-sheet", Name: "InventorySheet"},
			{EntityID: "entity-line-item", Name: "LineItem"},
			{EntityID: "entity-sku", Name: "Sku"},
		},
	}
	pc := appprepare.PlanningContext{TemplateID: "flutter-open-lite"}
	result, ok := EmitAppEntry(dm, pc)
	if !ok {
		t.Fatal("EmitAppEntry() returned false for inventory profile")
	}
	for _, marker := range []string{
		"Future<void> main() async {",
		"runApp(AppFactoryApp(repository: repository));",
		"class AppFactoryApp extends StatefulWidget",
		"final RecordRepository repository;",
		"Future<void> _ensureListDataLoaded() async {",
		"Future<List<Warehouse>> _loadWarehouses() async {",
		"Future<List<Sku>> _loadSkus() async {",
		"Future<List<LineItem>> _loadLineItemsForSheet(String sheetId) async {",
		"Future<void> _openCreateRecord() async {",
		"warehouses: warehouses,",
		"skus: skus,",
		"Future<void> _openEditRecord(InventorySheet sheet) async {",
		"initialSheet: sheet,",
		"initialItems: items,",
		"Future<void> _openRecordDetail(InventorySheet sheet) async {",
		"RecordDetailPage(",
		"sheet: sheet,",
		"warehouse: warehouse,",
		"items: items,",
		"skus: skus,",
		"onEdit: _openEditRecord,",
		"onCreateRecord: _openCreateRecord,",
		"onOpenRecordDetail: _openRecordDetail,",
		"onViewAllRecords: _openRecordList,",
		"return RecordFormPage(controller: widget.controller);",
	} {
		if !strings.Contains(result.Content, marker) {
			t.Fatalf("content missing inventory marker: %s", marker)
		}
	}
	for _, absent := range []string{
		"AppFactorySeedApp",
		"AppFactorySeedHomePage",
		"Seed workspace ready",
	} {
		if strings.Contains(result.Content, absent) {
			t.Fatalf("content should not contain stale inventory marker: %s", absent)
		}
	}
}

func TestEmitAppEntryUnsupportedTemplateReturnsFalse(t *testing.T) {
	dm := appprepare.DomainModel{
		Entities: []appprepare.DataEntity{
			{EntityID: "entity-project", Name: "Project"},
			{EntityID: "entity-task", Name: "Task"},
			{EntityID: "entity-tag", Name: "Tag"},
		},
	}
	pc := appprepare.PlanningContext{TemplateID: "flutter-finance-lite"}
	if _, ok := EmitAppEntry(dm, pc); ok {
		t.Fatal("EmitAppEntry() returned true for unsupported template, want false")
	}
}

func TestEmitAppEntryGenericReturnsFalse(t *testing.T) {
	dm := appprepare.DomainModel{
		DomainCopy: appprepare.DomainCopy{Title: "体重追踪"},
		Entities:   []appprepare.DataEntity{{EntityID: "entity-weight-record", Name: "WeightRecord"}},
	}
	pc := appprepare.PlanningContext{TemplateID: "flutter-open-lite"}
	if _, ok := EmitAppEntry(dm, pc); ok {
		t.Fatal("EmitAppEntry() returned true for generic profile, want false")
	}
}

func TestEmitAppEntryStructure(t *testing.T) {
	profiles := []appprepare.DomainModel{
		{Entities: []appprepare.DataEntity{{EntityID: "entity-project", Name: "Project"}, {EntityID: "entity-task", Name: "Task"}, {EntityID: "entity-tag", Name: "Tag"}}},
		{Entities: []appprepare.DataEntity{{EntityID: "entity-inventory-sheet", Name: "InventorySheet"}, {EntityID: "entity-line-item", Name: "LineItem"}, {EntityID: "entity-sku", Name: "Sku"}}},
	}
	pc := appprepare.PlanningContext{TemplateID: "flutter-open-lite"}
	for _, dm := range profiles {
		result, ok := EmitAppEntry(dm, pc)
		if !ok {
			t.Fatal("EmitAppEntry() returned false")
		}
		if !strings.HasPrefix(result.Content, "import 'package:flutter/material.dart';") {
			t.Fatal("content should start with flutter/material import")
		}
		if !strings.HasSuffix(result.Content, "}\n") {
			t.Fatal("content should end with newline")
		}
	}
}
