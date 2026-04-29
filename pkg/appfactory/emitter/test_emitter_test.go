package emitter

import (
	"strings"
	"testing"

	appprepare "github.com/sipeed/picoclaw/pkg/appfactory/prepare"
)

func TestEmitTestProjectTaskTag(t *testing.T) {
	dm := appprepare.DomainModel{
		DomainCopy: appprepare.DomainCopy{Title: "项目任务协同 App"},
		Entities: []appprepare.DataEntity{
			{EntityID: "entity-project"},
			{EntityID: "entity-task"},
			{EntityID: "entity-tag"},
			{EntityID: "entity-task-tag-link"},
			{EntityID: "entity-dashboard-summary", Source: "derived"},
		},
	}
	result, ok := EmitTest(dm, TestEmitConfig{})
	if !ok {
		t.Fatal("EmitTest returned false for project-task-tag")
	}
	if result.FilePath != "test/widget_test.dart" {
		t.Fatalf("FilePath = %q, want test/widget_test.dart", result.FilePath)
	}
	for _, marker := range []string{
		"import 'package:flutter_open_lite/main.dart';",
		"import 'package:flutter_open_lite/models/dashboard_summary.dart';",
		"import 'package:flutter_open_lite/models/project.dart';",
		"import 'package:flutter_open_lite/models/tag.dart';",
		"import 'package:flutter_open_lite/models/task.dart';",
		"import 'package:flutter_open_lite/models/task_tag_link.dart';",
		"relation-rich app supports create and inspect task flow",
		"InMemoryRecordRepository(",
		"Project(projectId: 'project-alpha'",
		"Task(taskId: 'task-seed'",
		"Tag(tagId: 'tag-priority'",
		"TaskTagLink(linkId: 'task-seed_tag-priority'",
		"DashboardSummary(projectId: 'project-alpha'",
		"AppFactoryApp(repository: repository)",
		"openLiteCopy.appTitle",
		"openLiteCopy.createPrimaryActionLabel",
		"openLiteCopy.createSubmitLabel",
		"openLiteCopy.viewAllActionLabel",
		"openLiteCopy.listPageTitle",
		"openLiteCopy.detailPageTitle",
		"'新增任务'",
		"'补充说明'",
	} {
		if !strings.Contains(result.Content, marker) {
			t.Fatalf("content missing project-task-tag marker: %s", marker)
		}
	}
}

func TestEmitTestInventorySheetLineItem(t *testing.T) {
	dm := appprepare.DomainModel{
		DomainCopy: appprepare.DomainCopy{Title: "库存盘点工作台"},
		Entities: []appprepare.DataEntity{
			{EntityID: "entity-inventory-sheet"},
			{EntityID: "entity-line-item"},
			{EntityID: "entity-sku"},
			{EntityID: "entity-warehouse"},
		},
	}
	result, ok := EmitTest(dm, TestEmitConfig{})
	if !ok {
		t.Fatal("EmitTest returned false for inventory")
	}
	for _, marker := range []string{
		"import 'package:flutter_open_lite/models/inventory_sheet.dart';",
		"import 'package:flutter_open_lite/models/line_item.dart';",
		"import 'package:flutter_open_lite/models/sku.dart';",
		"import 'package:flutter_open_lite/models/warehouse.dart';",
		"inventory relation-rich app supports overview and inspection flow",
		"InventorySheet(",
		"LineItem(",
		"Sku(skuId: 'sku-001'",
		"Warehouse(warehouseId: 'warehouse-east'",
		"AppFactoryApp(repository: repository)",
		"openLiteCopy.homeSummaryTitle",
		"'华东一号仓'",
		"openLiteCopy.createPageTitle",
		"add-line-item-button",
		"tester.pageBack()",
		"scrollUntilVisible(",
		"openLiteCopy.varianceQtyLabel",
		"'SKU-001'",
	} {
		if !strings.Contains(result.Content, marker) {
			t.Fatalf("content missing inventory marker: %s", marker)
		}
	}
	// inventory 不应包含 project-task-tag 特有内容
	for _, absent := range []string{
		"dashboard_summary.dart",
		"project.dart",
		"tag.dart",
		"task.dart",
		"task_tag_link.dart",
	} {
		if strings.Contains(result.Content, absent) {
			t.Fatalf("content should not contain: %s", absent)
		}
	}
}

func TestEmitTestRelationRichProfilesDoNotRequireOptionalDomainFields(t *testing.T) {
	cases := []struct {
		name        string
		entities    []appprepare.DataEntity
		wantMarkers []string
		wantAbsent  []string
	}{
		{
			name: "project-task-tag",
			entities: []appprepare.DataEntity{
				{EntityID: "entity-project"},
				{EntityID: "entity-task"},
				{EntityID: "entity-tag"},
			},
			wantMarkers: []string{
				"AppFactoryApp(repository: repository)",
				"models/project.dart",
				"models/task.dart",
				"models/tag.dart",
				"models/task_tag_link.dart",
			},
			wantAbsent: []string{
				"MyApp(repository: repository)",
			},
		},
		{
			name: "inventory-sheet-line-item",
			entities: []appprepare.DataEntity{
				{EntityID: "entity-inventory-sheet"},
				{EntityID: "entity-line-item"},
				{EntityID: "entity-sku"},
			},
			wantMarkers: []string{
				"AppFactoryApp(repository: repository)",
				"models/inventory_sheet.dart",
				"models/line_item.dart",
				"models/sku.dart",
				"models/warehouse.dart",
			},
			wantAbsent: []string{
				"MyApp(repository: repository)",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result, ok := EmitTest(appprepare.DomainModel{Entities: tc.entities}, TestEmitConfig{})
			if !ok {
				t.Fatal("EmitTest returned false for minimal relation-rich domain model")
			}
			if result.FilePath != "test/widget_test.dart" {
				t.Fatalf("FilePath = %q, want test/widget_test.dart", result.FilePath)
			}
			for _, marker := range tc.wantMarkers {
				if !strings.Contains(result.Content, marker) {
					t.Fatalf("content missing marker: %s", marker)
				}
			}
			for _, absent := range tc.wantAbsent {
				if strings.Contains(result.Content, absent) {
					t.Fatalf("content should not contain: %s", absent)
				}
			}
		})
	}
}

func TestEmitTestGeneric(t *testing.T) {
	dm := appprepare.DomainModel{
		DomainCopy: appprepare.DomainCopy{Title: "体重记录 App"},
		Entities: []appprepare.DataEntity{
			{EntityID: "entity-weight-record"},
		},
	}
	result, ok := EmitTest(dm, TestEmitConfig{})
	if !ok {
		t.Fatal("EmitTest returned false for generic")
	}
	for _, marker := range []string{
		"import 'package:flutter_open_lite/main.dart';",
		"import 'package:flutter_open_lite/repositories/record_repository.dart';",
		"import 'package:flutter_open_lite/template/open_lite_copy.dart';",
		"open lite flow supports create, read, update",
		"InMemoryRecordRepository();",
		"await repository.init();",
		"MyApp(repository: repository)",
		"openLiteCopy.listPageTitle",
		"openLiteCopy.createPrimaryActionLabel",
		"openLiteCopy.createSubmitLabel",
		"openLiteCopy.editPageTitle",
		"openLiteCopy.editSubmitLabel",
		"openLiteCopy.doneFilterLabel",
		"'测试任务'",
		"'更新后的测试任务'",
	} {
		if !strings.Contains(result.Content, marker) {
			t.Fatalf("content missing generic marker: %s", marker)
		}
	}
	// generic 不应包含 relation-rich model imports
	for _, absent := range []string{
		"models/project.dart",
		"models/task.dart",
		"models/inventory_sheet.dart",
	} {
		if strings.Contains(result.Content, absent) {
			t.Fatalf("content should not contain: %s", absent)
		}
	}
}

func TestEmitTestCustomConfig(t *testing.T) {
	dm := appprepare.DomainModel{
		DomainCopy: appprepare.DomainCopy{Title: "自定义 App"},
		Entities: []appprepare.DataEntity{
			{EntityID: "entity-project"},
			{EntityID: "entity-task"},
			{EntityID: "entity-tag"},
		},
	}
	cfg := TestEmitConfig{
		PackageName:  "my_custom_package",
		AppClassName: "CustomApp",
	}
	result, ok := EmitTest(dm, cfg)
	if !ok {
		t.Fatal("EmitTest returned false")
	}
	if !strings.Contains(result.Content, "import 'package:my_custom_package/main.dart';") {
		t.Fatal("content missing custom package name")
	}
	if !strings.Contains(result.Content, "CustomApp(repository: repository)") {
		t.Fatal("content missing custom app class name")
	}
}

func TestEmitTestGenericCustomRepositoryType(t *testing.T) {
	dm := appprepare.DomainModel{
		DomainCopy: appprepare.DomainCopy{Title: "待办 App"},
		Entities: []appprepare.DataEntity{
			{EntityID: "entity-todo"},
		},
	}
	cfg := TestEmitConfig{
		RepositoryType: "HiveRecordRepository",
	}
	result, ok := EmitTest(dm, cfg)
	if !ok {
		t.Fatal("EmitTest returned false")
	}
	if !strings.Contains(result.Content, "HiveRecordRepository();") {
		t.Fatal("content missing custom repository type")
	}
}

func TestEmitTestEmptyEntitiesReturnsFalse(t *testing.T) {
	dm := appprepare.DomainModel{
		DomainCopy: appprepare.DomainCopy{Title: "空实体"},
	}
	_, ok := EmitTest(dm, TestEmitConfig{})
	if ok {
		t.Fatal("EmitTest should return false for empty entities")
	}
}

func TestEmitTestStructureIntegrity(t *testing.T) {
	profiles := []struct {
		name     string
		entities []appprepare.DataEntity
	}{
		{"generic", []appprepare.DataEntity{{EntityID: "entity-record"}}},
		{"project-task-tag", []appprepare.DataEntity{{EntityID: "entity-project"}, {EntityID: "entity-task"}, {EntityID: "entity-tag"}}},
		{"inventory", []appprepare.DataEntity{{EntityID: "entity-inventory-sheet"}, {EntityID: "entity-line-item"}, {EntityID: "entity-sku"}}},
	}
	for _, p := range profiles {
		t.Run(p.name, func(t *testing.T) {
			dm := appprepare.DomainModel{
				DomainCopy: appprepare.DomainCopy{Title: "测试"},
				Entities:   p.entities,
			}
			result, ok := EmitTest(dm, TestEmitConfig{})
			if !ok {
				t.Fatal("EmitTest returned false")
			}
			if !strings.HasPrefix(result.Content, "import 'package:flutter/material.dart';") {
				t.Fatal("content should start with flutter/material import")
			}
			if !strings.HasSuffix(result.Content, "}\n") {
				t.Fatal("content should end with closing brace and newline")
			}
			if !strings.Contains(result.Content, "void main() {") {
				t.Fatal("content missing void main()")
			}
			if !strings.Contains(result.Content, "testWidgets(") {
				t.Fatal("content missing testWidgets(")
			}
		})
	}
}
