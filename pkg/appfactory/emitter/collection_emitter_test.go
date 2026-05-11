package emitter

import (
	"strings"
	"testing"

	appprepare "github.com/sipeed/oneappfactory/pkg/appfactory/prepare"
)

func TestEmitCollectionProjectTaskTag(t *testing.T) {
	dm := appprepare.DomainModel{
		DomainCopy: appprepare.DomainCopy{Title: "项目管理"},
		Entities: []appprepare.DataEntity{
			{EntityID: "entity-project", Name: "Project"},
			{EntityID: "entity-task", Name: "Task"},
			{EntityID: "entity-tag", Name: "Tag"},
		},
	}
	result, ok := EmitCollection(dm)
	if !ok {
		t.Fatal("EmitCollection() returned false for project-task-tag profile")
	}
	if result.ListPagePath != "lib/views/record_list_page.dart" {
		t.Fatalf("ListPagePath = %q, want lib/views/record_list_page.dart", result.ListPagePath)
	}
	if result.ListControllerPath != "lib/controllers/record_list_controller.dart" {
		t.Fatalf("ListControllerPath = %q, want lib/controllers/record_list_controller.dart", result.ListControllerPath)
	}
	// ListPage 关键结构验证
	for _, marker := range []string{
		"class RecordListPage extends StatelessWidget",
		"required this.onCreateRecord",
		"required this.onOpenTaskDetail",
		"controller.visibleTasks",
		"controller.hasFilter",
		"controller.clearFilters",
		"_RelationRichFilterStrip",
		"_TaskCard",
		"_EmptyState",
		"openLiteCopy.listPageTitle",
		"openLiteCopy.listCountLabel(tasks.length)",
		"openLiteCopy.clearFilterActionLabel",
		"openLiteCopy.createPrimaryActionLabel",
		"_projectTitleFor(controller.projects, task.projectId)",
		"controller.setProjectId(",
		"controller.setTagId(",
		"controller.setStatus(",
		"openLiteCopy.taskStatusLabel(status)",
	} {
		if !strings.Contains(result.ListPageContent, marker) {
			t.Errorf("ListPage missing marker: %q", marker)
		}
	}
	// 不应有 inventory 特有元素
	for _, absent := range []string{"_InventoryFilterStrip", "_InventorySheetCard", "onOpenRecordDetail", "warehouse", "lowStock"} {
		if strings.Contains(result.ListPageContent, absent) {
			t.Errorf("ListPage should not contain inventory marker: %q", absent)
		}
	}
	// ListController 关键结构验证
	for _, marker := range []string{
		"class RecordListController extends ChangeNotifier",
		"List<Task> get visibleTasks",
		"List<Task> get filteredTasks",
		"void setProjectId(",
		"void setTagId(",
		"void setStatus(TaskStatus? status)",
		"void clearFilters()",
		"Future<void> refresh() async",
		"_repository.loadProjects()",
		"_repository.loadTasks()",
		"_repository.loadTags()",
		"_repository.loadTaskTagLinks()",
		"bool get hasFilter",
	} {
		if !strings.Contains(result.ListControllerContent, marker) {
			t.Errorf("ListController missing marker: %q", marker)
		}
	}
	for _, absent := range []string{"warehouse", "InventorySheet", "lowStock", "lineItems"} {
		if strings.Contains(result.ListControllerContent, absent) {
			t.Errorf("ListController should not contain inventory marker: %q", absent)
		}
	}
}

func TestEmitCollectionInventory(t *testing.T) {
	dm := appprepare.DomainModel{
		DomainCopy: appprepare.DomainCopy{Title: "库存管家"},
		Entities: []appprepare.DataEntity{
			{EntityID: "entity-inventory-sheet", Name: "InventorySheet"},
			{EntityID: "entity-line-item", Name: "LineItem"},
			{EntityID: "entity-sku", Name: "SKU"},
		},
	}
	result, ok := EmitCollection(dm)
	if !ok {
		t.Fatal("EmitCollection() returned false for inventory profile")
	}
	// ListPage 关键结构验证
	for _, marker := range []string{
		"class RecordListPage extends StatelessWidget",
		"required this.onCreateRecord",
		"required this.onOpenRecordDetail",
		"controller.visibleSheets",
		"controller.hasFilter",
		"_InventoryFilterStrip",
		"_InventorySheetCard",
		"openLiteCopy.listPageTitle",
		"openLiteCopy.listCountLabel(sheets.length)",
		"controller.warehouseFor(sheet.warehouseId)",
		"controller.lineItemsForSheet(sheet.sheetId)",
		"controller.isLowStockSheet(sheet)",
		"controller.setWarehouseId(",
		"controller.setLowStockOnly",
		"openLiteCopy.statusLabel(status)",
	} {
		if !strings.Contains(result.ListPageContent, marker) {
			t.Errorf("ListPage missing marker: %q", marker)
		}
	}
	// 不应有 project-task-tag 特有元素
	for _, absent := range []string{"_RelationRichFilterStrip", "_TaskCard", "onOpenTaskDetail", "task.projectId"} {
		if strings.Contains(result.ListPageContent, absent) {
			t.Errorf("ListPage should not contain project-task-tag marker: %q", absent)
		}
	}
	// ListController 关键结构验证
	for _, marker := range []string{
		"class RecordListController extends ChangeNotifier",
		"List<InventorySheet> get visibleSheets",
		"List<InventorySheet> get filteredSheets",
		"void setWarehouseId(String? warehouseId)",
		"void setLowStockOnly(bool value)",
		"void setStatus(InventorySheetStatus? status)",
		"void clearFilters()",
		"bool isLowStockSheet(InventorySheet sheet)",
		"Warehouse? warehouseFor(String warehouseId)",
		"List<LineItem> lineItemsForSheet(String sheetId)",
		"_repository.loadInventorySheets()",
		"_repository.loadLineItems()",
		"_repository.loadSkus()",
		"_repository.loadWarehouses()",
		"bool _lowStockOnly = false",
	} {
		if !strings.Contains(result.ListControllerContent, marker) {
			t.Errorf("ListController missing marker: %q", marker)
		}
	}
}

func TestEmitCollectionGenericReturnsFalse(t *testing.T) {
	dm := appprepare.DomainModel{
		DomainCopy: appprepare.DomainCopy{Title: "体重追踪"},
		Entities:   []appprepare.DataEntity{{EntityID: "entity-weight-record", Name: "WeightRecord"}},
	}
	_, ok := EmitCollection(dm)
	if ok {
		t.Fatal("EmitCollection() returned true for generic profile, want false")
	}
}

func TestEmitCollectionNoEntitiesReturnsFalse(t *testing.T) {
	dm := appprepare.DomainModel{
		DomainCopy: appprepare.DomainCopy{Title: "空应用"},
	}
	_, ok := EmitCollection(dm)
	if ok {
		t.Fatal("EmitCollection() returned true for empty entities, want false")
	}
}

func TestEmitCollectionProjectTaskTagStructure(t *testing.T) {
	dm := appprepare.DomainModel{
		DomainCopy: appprepare.DomainCopy{Title: "任务管理"},
		Entities: []appprepare.DataEntity{
			{EntityID: "entity-project", Name: "Project"},
			{EntityID: "entity-task", Name: "Task"},
			{EntityID: "entity-tag", Name: "Tag"},
		},
	}
	result, ok := EmitCollection(dm)
	if !ok {
		t.Fatal("EmitCollection() returned false")
	}
	if !strings.HasPrefix(result.ListPageContent, "import 'package:flutter/material.dart';") {
		t.Error("ListPage should start with flutter/material.dart import")
	}
	if !strings.HasSuffix(result.ListPageContent, "\n") {
		t.Error("ListPage should end with newline")
	}
	if !strings.HasPrefix(result.ListControllerContent, "import 'package:flutter/foundation.dart'") {
		t.Error("ListController should start with flutter/foundation.dart import")
	}
	if !strings.HasSuffix(result.ListControllerContent, "\n") {
		t.Error("ListController should end with newline")
	}
}

func TestEmitCollectionInventoryStructure(t *testing.T) {
	dm := appprepare.DomainModel{
		DomainCopy: appprepare.DomainCopy{Title: "库存管家"},
		Entities: []appprepare.DataEntity{
			{EntityID: "entity-inventory-sheet", Name: "InventorySheet"},
			{EntityID: "entity-line-item", Name: "LineItem"},
			{EntityID: "entity-sku", Name: "SKU"},
		},
	}
	result, ok := EmitCollection(dm)
	if !ok {
		t.Fatal("EmitCollection() returned false")
	}
	if !strings.HasPrefix(result.ListPageContent, "import 'package:flutter/material.dart';") {
		t.Error("ListPage should start with flutter/material.dart import")
	}
	if !strings.HasSuffix(result.ListPageContent, "\n") {
		t.Error("ListPage should end with newline")
	}
	if !strings.HasPrefix(result.ListControllerContent, "import 'package:flutter/foundation.dart'") {
		t.Error("ListController should start with flutter/foundation.dart import")
	}
	if !strings.HasSuffix(result.ListControllerContent, "\n") {
		t.Error("ListController should end with newline")
	}
}
