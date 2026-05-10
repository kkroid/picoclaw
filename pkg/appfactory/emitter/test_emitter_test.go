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
		"open lite app renders primary flow",
		"InMemoryRecordRepository();",
		"await repository.init();",
		"MyApp(repository: repository)",
		"openLiteCopy.appTitle",
		"openLiteCopy.createPrimaryActionLabel",
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

func TestEmitTestGenericUsesSchemaSeedRecord(t *testing.T) {
	dm := appprepare.DomainModel{
		DomainCopy: appprepare.DomainCopy{Title: "课程作业 App"},
		Entities: []appprepare.DataEntity{
			{
				EntityID: "entity-course-assignment",
				Name:     "课程作业",
				Fields: []appprepare.DataField{
					{Name: "assignment_id", Type: "string", Role: "identifier", Required: true},
					{Name: "assignment_title", Type: "string", Role: "primary_text", Required: true},
					{Name: "due_date", Type: "date", Role: "due_date", Required: true},
				},
			},
		},
	}
	result, ok := EmitTest(dm, TestEmitConfig{AppClassName: "AppFactoryApp"})
	if !ok {
		t.Fatal("EmitTest returned false for generic schema")
	}
	for _, marker := range []string{
		"import 'package:flutter/foundation.dart';",
		"import 'package:flutter_open_lite/models/record.dart';",
		"InMemoryRecordRepository(seedRecords: [CourseAssignment(assignmentTitle: '测试记录')])",
		"AppFactoryApp(repository: repository)",
		"open lite app supports schema-driven create and inspect flow",
		"expect(find.text('测试记录'), findsOneWidget);",
		"await tester.enterText(find.byKey(const Key('title-field')), '新增记录');",
		"expect(find.text(openLiteCopy.dateFieldLabel), findsOneWidget);",
		"await tester.tap(find.text(openLiteCopy.viewAllActionLabel));",
	} {
		if !strings.Contains(result.Content, marker) {
			t.Fatalf("content missing schema generic marker %q: %s", marker, result.Content)
		}
	}
	if strings.Contains(result.Content, "recordId") {
		t.Fatalf("generic schema test should not hard-code recordId: %s", result.Content)
	}
	if strings.Contains(result.Content, "package:flutter/material.dart") {
		t.Fatalf("generic schema test should not import unused material.dart: %s", result.Content)
	}
}

func TestEmitTestGenericCoversFieldDrivenWidgets(t *testing.T) {
	dm := appprepare.DomainModel{
		DomainCopy: appprepare.DomainCopy{Title: "行李打包清单 App"},
		Entities: []appprepare.DataEntity{
			{
				EntityID: "entity-packing-item",
				Name:     "PackingItem",
				Fields: []appprepare.DataField{
					{Name: "packing_item_id", Type: "string", Role: "identifier", Required: true},
					{Name: "item_name", Type: "string", Role: "primary_text", Required: true},
					{Name: "category", Type: "string", Role: "secondary_text"},
					{Name: "is_packed", Type: "bool", Role: "flag"},
					{Name: "note", Type: "string", Role: "note"},
				},
			},
			{
				EntityID: "entity-dashboard-summary",
				Source:   "derived",
				Fields: []appprepare.DataField{
					{Name: "total_count", Type: "int"},
					{Name: "packed_count", Type: "int"},
				},
			},
		},
	}
	result, ok := EmitTest(dm, TestEmitConfig{AppClassName: "AppFactoryApp"})
	if !ok {
		t.Fatal("EmitTest returned false for generic field-driven schema")
	}
	for _, marker := range []string{
		"InMemoryRecordRepository(seedRecords: [PackingItem(itemName: '测试记录', category: '测试分类', isPacked: true, note: '测试备注')])",
		"expect(find.textContaining(openLiteCopy.summaryTotalCountLabel), findsWidgets);",
		"expect(find.textContaining(openLiteCopy.summaryPackedCountLabel), findsWidgets);",
		"await tester.enterText(find.byKey(const Key('secondary-field')), '测试分类');",
		"expect(find.text(openLiteCopy.isPackedFieldLabel), findsOneWidget);",
		"await tester.tap(find.byKey(const Key('is-packed-field')));",
		"await tester.enterText(find.byKey(const Key('note-field')), '测试备注');",
		"expect(find.textContaining(openLiteCopy.isPackedFieldLabel), findsWidgets);",
		"expect(find.text(openLiteCopy.detailIsPackedLabel), findsOneWidget);",
		"expect(find.text(openLiteCopy.booleanValueLabel(true)), findsWidgets);",
	} {
		if !strings.Contains(result.Content, marker) {
			t.Fatalf("content missing field-driven marker %q: %s", marker, result.Content)
		}
	}
}

func TestEmitTestGenericCoversNumericWidgets(t *testing.T) {
	dm := appprepare.DomainModel{
		DomainCopy: appprepare.DomainCopy{Title: "观影清单 App"},
		Entities: []appprepare.DataEntity{
			{
				EntityID: "entity-movie-entry",
				Name:     "MovieEntry",
				Fields: []appprepare.DataField{
					{Name: "movie_id", Type: "string", Role: "identifier", Required: true},
					{Name: "movie_title", Type: "string", Role: "primary_text", Required: true},
					{Name: "rating", Type: "number", Role: "rating"},
				},
			},
			{
				EntityID: "entity-dashboard-summary",
				Source:   "derived",
				Fields: []appprepare.DataField{
					{Name: "average_rating", Type: "double"},
				},
			},
		},
	}
	result, ok := EmitTest(dm, TestEmitConfig{})
	if !ok {
		t.Fatal("EmitTest returned false for generic numeric schema")
	}
	for _, marker := range []string{
		"InMemoryRecordRepository(seedRecords: [MovieEntry(movieTitle: '测试记录', rating: 4.5)])",
		"expect(find.textContaining(openLiteCopy.summaryAverageRatingLabel), findsWidgets);",
		"expect(find.text(openLiteCopy.ratingFieldLabel), findsOneWidget);",
		"await tester.enterText(find.byKey(const Key('rating-field')), '4.6');",
	} {
		if !strings.Contains(result.Content, marker) {
			t.Fatalf("content missing numeric marker %q: %s", marker, result.Content)
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
			if !strings.HasPrefix(result.Content, "import 'package:") {
				t.Fatal("content should start with a Dart package import")
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
