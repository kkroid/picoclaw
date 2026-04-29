package emitter

import (
	"strings"
	"testing"

	appprepare "github.com/sipeed/picoclaw/pkg/appfactory/prepare"
)

func TestEmitInspectionProjectTaskTag(t *testing.T) {
	dm := appprepare.DomainModel{
		DomainCopy: appprepare.DomainCopy{Title: "项目管理"},
		Entities: []appprepare.DataEntity{
			{EntityID: "entity-project", Name: "Project"},
			{EntityID: "entity-task", Name: "Task"},
			{EntityID: "entity-tag", Name: "Tag"},
		},
	}
	result, ok := EmitInspection(dm)
	if !ok {
		t.Fatal("EmitInspection() returned false for project-task-tag profile")
	}
	if result.DetailPagePath != "lib/views/record_detail_page.dart" {
		t.Fatalf("DetailPagePath = %q, want lib/views/record_detail_page.dart", result.DetailPagePath)
	}
	// DetailPage 关键结构验证
	for _, marker := range []string{
		"class RecordDetailPage extends StatefulWidget",
		"required this.task,",
		"required this.projects,",
		"required this.tags,",
		"required this.taskTags,",
		"required this.onEdit,",
		"final Task task;",
		"final List<Project> projects;",
		"final List<Tag> tags;",
		"final List<String> taskTags;",
		"Future<void> Function(Task task) onEdit;",
		"_RecordDetailPageState",
		"late Task _task;",
		"_editTask",
		"_projectTitleFor",
		"_selectedTags",
		"openLiteCopy.detailPageTitle",
		"openLiteCopy.editActionLabel",
		"openLiteCopy.detailProjectLabel",
		"openLiteCopy.detailStatusLabel",
		"openLiteCopy.detailDateLabel",
		"openLiteCopy.detailNoteLabel",
		"openLiteCopy.emptyNoteLabel",
		"openLiteCopy.taskStatusLabel",
		"openLiteCopy.tagFilterLabel",
		"_DetailCard",
		"_InfoTile",
		"_formatDate",
		"multiline: true",
	} {
		if !strings.Contains(result.DetailPageContent, marker) {
			t.Errorf("DetailPage missing marker: %q", marker)
		}
	}
	// 不应有 inventory 特有元素
	for _, absent := range []string{"_HeaderCard", "_LineItemCard", "_EmptyLineItemState", "InventorySheet", "warehouse", "skuId", "_skuNameFor"} {
		if strings.Contains(result.DetailPageContent, absent) {
			t.Errorf("DetailPage should not contain inventory marker: %q", absent)
		}
	}
}

func TestEmitInspectionInventory(t *testing.T) {
	dm := appprepare.DomainModel{
		DomainCopy: appprepare.DomainCopy{Title: "库存管家"},
		Entities: []appprepare.DataEntity{
			{EntityID: "entity-inventory-sheet", Name: "InventorySheet"},
			{EntityID: "entity-line-item", Name: "LineItem"},
			{EntityID: "entity-sku", Name: "SKU"},
		},
	}
	result, ok := EmitInspection(dm)
	if !ok {
		t.Fatal("EmitInspection() returned false for inventory profile")
	}
	// DetailPage 关键结构验证
	for _, marker := range []string{
		"class RecordDetailPage extends StatelessWidget",
		"required this.sheet,",
		"required this.warehouse,",
		"required this.items,",
		"required this.skus,",
		"required this.onEdit,",
		"final InventorySheet sheet;",
		"final Warehouse? warehouse;",
		"final List<LineItem> items;",
		"final List<Sku> skus;",
		"Future<void> Function(InventorySheet sheet) onEdit;",
		"_skuNameFor",
		"_HeaderCard",
		"_InfoTile",
		"_LineItemCard",
		"_EmptyLineItemState",
		"_formatInventoryDetailDate",
		"openLiteCopy.detailPageTitle",
		"openLiteCopy.editActionLabel",
		"openLiteCopy.detailCategoryLabel",
		"openLiteCopy.detailStatusLabel",
		"openLiteCopy.detailDateLabel",
		"openLiteCopy.detailNoteLabel",
		"openLiteCopy.emptyNoteLabel",
		"openLiteCopy.statusLabel(sheet.status)",
		"openLiteCopy.warehouseLocationLabel",
		"openLiteCopy.lineItemTitle",
		"openLiteCopy.expectedQtyLabel",
		"openLiteCopy.countedQtyLabel",
		"openLiteCopy.varianceQtyLabel",
	} {
		if !strings.Contains(result.DetailPageContent, marker) {
			t.Errorf("DetailPage missing marker: %q", marker)
		}
	}
	// 不应有 project-task-tag 特有元素
	for _, absent := range []string{"_DetailCard", "taskTags", "Task task;", "taskStatusLabel", "_projectTitleFor"} {
		if strings.Contains(result.DetailPageContent, absent) {
			t.Errorf("DetailPage should not contain project-task-tag marker: %q", absent)
		}
	}
}

func TestEmitInspectionGenericReturnsFalse(t *testing.T) {
	dm := appprepare.DomainModel{
		DomainCopy: appprepare.DomainCopy{Title: "体重追踪"},
		Entities:   []appprepare.DataEntity{{EntityID: "entity-weight-record", Name: "WeightRecord"}},
	}
	_, ok := EmitInspection(dm)
	if ok {
		t.Fatal("EmitInspection() returned true for generic profile, want false")
	}
}

func TestEmitInspectionNoEntitiesReturnsFalse(t *testing.T) {
	dm := appprepare.DomainModel{
		DomainCopy: appprepare.DomainCopy{Title: "空应用"},
	}
	_, ok := EmitInspection(dm)
	if ok {
		t.Fatal("EmitInspection() returned true for empty entities, want false")
	}
}

func TestEmitInspectionProjectTaskTagStructure(t *testing.T) {
	dm := appprepare.DomainModel{
		DomainCopy: appprepare.DomainCopy{Title: "任务管理"},
		Entities: []appprepare.DataEntity{
			{EntityID: "entity-project", Name: "Project"},
			{EntityID: "entity-task", Name: "Task"},
			{EntityID: "entity-tag", Name: "Tag"},
		},
	}
	result, ok := EmitInspection(dm)
	if !ok {
		t.Fatal("EmitInspection() returned false")
	}
	if !strings.HasPrefix(result.DetailPageContent, "import 'package:flutter/material.dart';") {
		t.Error("DetailPage should start with flutter/material.dart import")
	}
	if !strings.HasSuffix(result.DetailPageContent, "\n") {
		t.Error("DetailPage should end with newline")
	}
}

func TestEmitInspectionInventoryStructure(t *testing.T) {
	dm := appprepare.DomainModel{
		DomainCopy: appprepare.DomainCopy{Title: "库存管家"},
		Entities: []appprepare.DataEntity{
			{EntityID: "entity-inventory-sheet", Name: "InventorySheet"},
			{EntityID: "entity-line-item", Name: "LineItem"},
			{EntityID: "entity-sku", Name: "SKU"},
		},
	}
	result, ok := EmitInspection(dm)
	if !ok {
		t.Fatal("EmitInspection() returned false")
	}
	if !strings.HasPrefix(result.DetailPageContent, "import 'package:flutter/material.dart';") {
		t.Error("DetailPage should start with flutter/material.dart import")
	}
	if !strings.HasSuffix(result.DetailPageContent, "\n") {
		t.Error("DetailPage should end with newline")
	}
}
