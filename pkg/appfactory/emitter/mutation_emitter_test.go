package emitter

import (
	"strings"
	"testing"

	appprepare "github.com/sipeed/picoclaw/pkg/appfactory/prepare"
)

func TestEmitMutationProjectTaskTag(t *testing.T) {
	dm := appprepare.DomainModel{
		DomainCopy: appprepare.DomainCopy{Title: "项目管理"},
		Entities: []appprepare.DataEntity{
			{EntityID: "entity-project", Name: "Project"},
			{EntityID: "entity-task", Name: "Task"},
			{EntityID: "entity-tag", Name: "Tag"},
		},
	}
	result, ok := EmitMutation(dm)
	if !ok {
		t.Fatal("EmitMutation() returned false for project-task-tag profile")
	}
	if result.FormPagePath != "lib/views/record_form_page.dart" {
		t.Fatalf("FormPagePath = %q, want lib/views/record_form_page.dart", result.FormPagePath)
	}
	if result.FormControllerPath != "lib/controllers/record_form_controller.dart" {
		t.Fatalf("FormControllerPath = %q, want lib/controllers/record_form_controller.dart", result.FormControllerPath)
	}
	// FormPage 关键结构验证
	for _, marker := range []string{
		"class RecordFormPage extends StatefulWidget",
		"required this.controller,",
		"final RecordFormController controller;",
		"_RecordFormPageState",
		"GlobalKey<FormState> _formKey",
		"_pickDate",
		"_save",
		"widget.controller.isEditing",
		"widget.controller.titleController",
		"widget.controller.projectController",
		"widget.controller.selectedTagIds",
		"widget.controller.selectedStatus",
		"widget.controller.selectedDate",
		"widget.controller.setStatus(TaskStatus.",
		"widget.controller.setProject(",
		"widget.controller.toggleTag(",
		"widget.controller.setDate(",
		"widget.controller.noteController",
		"openLiteCopy.editPageTitle",
		"openLiteCopy.createPageTitle",
		"openLiteCopy.titleFieldLabel",
		"openLiteCopy.projectFieldLabel",
		"openLiteCopy.dateFieldLabel",
		"openLiteCopy.noteFieldLabel",
		"openLiteCopy.editSubmitLabel",
		"openLiteCopy.createSubmitLabel",
		"ChoiceChip(",
		"FilterChip(",
		"DropdownButtonFormField<String>(",
		"FilledButton(",
		"Navigator.of(context).pop(savedTask)",
	} {
		if !strings.Contains(result.FormPageContent, marker) {
			t.Errorf("FormPage missing marker: %q", marker)
		}
	}
	// 不应有 inventory 特有元素
	for _, absent := range []string{"_LineItemEditor", "_LineItemEmptyState", "warehouseController", "InventorySheetStatus", "selectedCountedOn"} {
		if strings.Contains(result.FormPageContent, absent) {
			t.Errorf("FormPage should not contain inventory marker: %q", absent)
		}
	}
	// FormController 关键结构验证
	for _, marker := range []string{
		"class RecordFormController extends ChangeNotifier",
		"required this.repository,",
		"this.projects = const [],",
		"this.tags = const [],",
		"Task? initialTask,",
		"List<String>? initialTagIds,",
		"final TextEditingController titleController;",
		"final TextEditingController noteController;",
		"final TextEditingController projectController;",
		"TaskStatus _selectedStatus;",
		"DateTime? _selectedDate;",
		"void setStatus(TaskStatus status)",
		"void setDate(DateTime? date)",
		"void setProject(String projectId)",
		"void toggleTag(String tagId)",
		"Future<Task> submit() async",
		"repository.loadTasks()",
		"repository.saveTasks(",
		"repository.loadTaskTagLinks()",
		"repository.saveTaskTagLinks(",
		"TaskTagLink(",
	} {
		if !strings.Contains(result.FormControllerContent, marker) {
			t.Errorf("FormController missing marker: %q", marker)
		}
	}
	for _, absent := range []string{"warehouse", "InventorySheet", "lineItem", "countedOn", "_nextDraftSeed"} {
		if strings.Contains(result.FormControllerContent, absent) {
			t.Errorf("FormController should not contain inventory marker: %q", absent)
		}
	}
}

func TestEmitMutationInventory(t *testing.T) {
	dm := appprepare.DomainModel{
		DomainCopy: appprepare.DomainCopy{Title: "库存管家"},
		Entities: []appprepare.DataEntity{
			{EntityID: "entity-inventory-sheet", Name: "InventorySheet"},
			{EntityID: "entity-line-item", Name: "LineItem"},
			{EntityID: "entity-sku", Name: "SKU"},
		},
	}
	result, ok := EmitMutation(dm)
	if !ok {
		t.Fatal("EmitMutation() returned false for inventory profile")
	}
	// FormPage 关键结构验证
	for _, marker := range []string{
		"class RecordFormPage extends StatefulWidget",
		"required this.controller,",
		"final RecordFormController controller;",
		"widget.controller.selectedCountedOn",
		"widget.controller.setCountedOn(",
		"widget.controller.warehouses",
		"widget.controller.warehouseController",
		"widget.controller.selectedStatus",
		"widget.controller.setStatus(InventorySheetStatus.",
		"widget.controller.setWarehouse(",
		"widget.controller.lineItems",
		"widget.controller.updateLineItem(",
		"widget.controller.removeLineItem(",
		"widget.controller.addLineItem()",
		"widget.controller.skus",
		"widget.controller.noteController",
		"_LineItemEditor",
		"_LineItemEmptyState",
		"_formatInventoryDate",
		"openLiteCopy.editPageTitle",
		"openLiteCopy.createPageTitle",
		"openLiteCopy.categoryFieldLabel",
		"openLiteCopy.dateFieldLabel",
		"openLiteCopy.noteFieldLabel",
		"openLiteCopy.lineItemTitle",
		"openLiteCopy.skuLabel",
		"openLiteCopy.expectedQtyLabel",
		"openLiteCopy.countedQtyLabel",
		"openLiteCopy.varianceQtyLabel",
		"Navigator.of(context).pop(savedSheet)",
	} {
		if !strings.Contains(result.FormPageContent, marker) {
			t.Errorf("FormPage missing marker: %q", marker)
		}
	}
	// 不应有 project-task-tag 特有元素
	for _, absent := range []string{"FilterChip(", "toggleTag", "task.projectId", "TaskStatus."} {
		if strings.Contains(result.FormPageContent, absent) {
			t.Errorf("FormPage should not contain project-task-tag marker: %q", absent)
		}
	}
	// FormController 关键结构验证
	for _, marker := range []string{
		"class RecordFormController extends ChangeNotifier",
		"required this.repository,",
		"List<Warehouse> warehouses",
		"List<Sku> skus",
		"InventorySheet? initialSheet",
		"List<LineItem>? initialItems",
		"final TextEditingController warehouseController;",
		"final TextEditingController countedOnController;",
		"final TextEditingController noteController;",
		"InventorySheetStatus _selectedStatus;",
		"int _nextDraftSeed = 0;",
		"void setStatus(InventorySheetStatus status)",
		"void setWarehouse(String warehouseId)",
		"void setCountedOn(DateTime value)",
		"void addLineItem(",
		"void updateLineItem(",
		"void removeLineItem(",
		"Future<InventorySheet> submit() async",
		"repository.loadLineItems()",
		"repository.saveLineItems(",
		"_formatInventoryDate",
	} {
		if !strings.Contains(result.FormControllerContent, marker) {
			t.Errorf("FormController missing marker: %q", marker)
		}
	}
	for _, absent := range []string{"TaskStatus", "toggleTag", "projectId", "taskTagLink"} {
		if strings.Contains(result.FormControllerContent, absent) {
			t.Errorf("FormController should not contain project-task-tag marker: %q", absent)
		}
	}
}

func TestEmitMutationGenericReturnsFalse(t *testing.T) {
	dm := appprepare.DomainModel{
		DomainCopy: appprepare.DomainCopy{Title: "体重追踪"},
		Entities:   []appprepare.DataEntity{{EntityID: "entity-weight-record", Name: "WeightRecord"}},
	}
	_, ok := EmitMutation(dm)
	if ok {
		t.Fatal("EmitMutation() returned true for generic profile, want false")
	}
}

func TestEmitMutationNoEntitiesReturnsFalse(t *testing.T) {
	dm := appprepare.DomainModel{
		DomainCopy: appprepare.DomainCopy{Title: "空应用"},
	}
	_, ok := EmitMutation(dm)
	if ok {
		t.Fatal("EmitMutation() returned true for empty entities, want false")
	}
}

func TestEmitMutationProjectTaskTagStructure(t *testing.T) {
	dm := appprepare.DomainModel{
		DomainCopy: appprepare.DomainCopy{Title: "任务管理"},
		Entities: []appprepare.DataEntity{
			{EntityID: "entity-project", Name: "Project"},
			{EntityID: "entity-task", Name: "Task"},
			{EntityID: "entity-tag", Name: "Tag"},
		},
	}
	result, ok := EmitMutation(dm)
	if !ok {
		t.Fatal("EmitMutation() returned false")
	}
	if !strings.HasPrefix(result.FormPageContent, "import 'package:flutter/material.dart';") {
		t.Error("FormPage should start with flutter/material.dart import")
	}
	if !strings.HasSuffix(result.FormPageContent, "\n") {
		t.Error("FormPage should end with newline")
	}
	if !strings.HasPrefix(result.FormControllerContent, "import 'package:flutter/material.dart';") {
		t.Error("FormController should start with flutter/material.dart import")
	}
	if !strings.HasSuffix(result.FormControllerContent, "\n") {
		t.Error("FormController should end with newline")
	}
}

func TestEmitMutationInventoryStructure(t *testing.T) {
	dm := appprepare.DomainModel{
		DomainCopy: appprepare.DomainCopy{Title: "库存管家"},
		Entities: []appprepare.DataEntity{
			{EntityID: "entity-inventory-sheet", Name: "InventorySheet"},
			{EntityID: "entity-line-item", Name: "LineItem"},
			{EntityID: "entity-sku", Name: "SKU"},
		},
	}
	result, ok := EmitMutation(dm)
	if !ok {
		t.Fatal("EmitMutation() returned false")
	}
	if !strings.HasPrefix(result.FormPageContent, "import 'package:flutter/material.dart';") {
		t.Error("FormPage should start with flutter/material.dart import")
	}
	if !strings.HasSuffix(result.FormPageContent, "\n") {
		t.Error("FormPage should end with newline")
	}
	if !strings.HasPrefix(result.FormControllerContent, "import 'package:flutter/material.dart';") {
		t.Error("FormController should start with flutter/material.dart import")
	}
	if !strings.HasSuffix(result.FormControllerContent, "\n") {
		t.Error("FormController should end with newline")
	}
}
