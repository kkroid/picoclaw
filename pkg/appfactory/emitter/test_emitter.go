package emitter

import (
	"strings"

	appprepare "github.com/sipeed/picoclaw/pkg/appfactory/prepare"
)

// TestEmitConfig 提供 TestEmitter 的运行时配置。
// 零值表示使用默认值。
type TestEmitConfig struct {
	// PackageName 是 Dart 包名，默认 "flutter_open_lite"。
	PackageName string
	// AppClassName 是应用入口 Widget 类名。
	// relation-rich 默认 "AppFactoryApp"，generic 默认 "MyApp"。
	AppClassName string
	// RepositoryType 是 generic profile 使用的 repository 类型名，默认 "InMemoryRecordRepository"。
	RepositoryType string
}

// TestEmitResult 表示 TestEmitter 的输出。
type TestEmitResult struct {
	Content  string
	FilePath string
}

// EmitTest 从 DomainModel 确定性生成 test/widget_test.dart。
// 返回 (result, true) 表示成功；(zero, false) 表示当前 DomainModel 不在可 emit 范围内。
func EmitTest(dm appprepare.DomainModel, cfg TestEmitConfig) (TestEmitResult, bool) {
	profile := identifyCopyProfile(dm)
	if profile == "" {
		return TestEmitResult{}, false
	}
	packageName := cfg.PackageName
	if packageName == "" {
		packageName = "flutter_open_lite"
	}
	content := renderTestForProfile(profile, packageName, cfg)
	return TestEmitResult{
		Content:  content,
		FilePath: "test/widget_test.dart",
	}, true
}

// renderTestForProfile 按 profile 渲染 widget_test.dart 的完整内容。
func renderTestForProfile(profile copyProfile, packageName string, cfg TestEmitConfig) string {
	switch profile {
	case copyProfileProjectTaskTag:
		return renderProjectTaskTagTest(packageName, resolveAppClassName(cfg.AppClassName, "AppFactoryApp"))
	case copyProfileInventorySheetLineItem:
		return renderInventorySheetLineItemTest(packageName, resolveAppClassName(cfg.AppClassName, "AppFactoryApp"))
	default:
		return renderGenericTest(packageName, resolveAppClassName(cfg.AppClassName, "MyApp"), resolveRepositoryType(cfg.RepositoryType))
	}
}

func resolveAppClassName(configured, fallback string) string {
	if configured != "" {
		return configured
	}
	return fallback
}

func resolveRepositoryType(configured string) string {
	if configured != "" {
		return configured
	}
	return "InMemoryRecordRepository"
}

func renderGenericTest(packageName, appClassName, repositoryType string) string {
	return strings.Join([]string{
		"import 'package:flutter/material.dart';",
		"import 'package:flutter_test/flutter_test.dart';",
		"",
		"import 'package:" + packageName + "/main.dart';",
		"import 'package:" + packageName + "/repositories/record_repository.dart';",
		"import 'package:" + packageName + "/template/open_lite_copy.dart';",
		"",
		"void main() {",
		"  testWidgets('open lite flow supports create, read, update', (WidgetTester tester) async {",
		"    final repository = " + repositoryType + "();",
		"    await repository.init();",
		"",
		"    await tester.pumpWidget(" + appClassName + "(repository: repository));",
		"    await tester.pumpAndSettle();",
		"",
		"    expect(find.text(openLiteCopy.listPageTitle), findsOneWidget);",
		"    expect(find.text(openLiteCopy.createPrimaryActionLabel), findsOneWidget);",
		"",
		"    await tester.tap(find.text(openLiteCopy.createPrimaryActionLabel));",
		"    await tester.pumpAndSettle();",
		"",
		"    await tester.enterText(find.byKey(const Key('title-field')), '测试任务');",
		"    await tester.enterText(find.byKey(const Key('note-field')), '这是一个测试备注');",
		"    await tester.tap(find.text(openLiteCopy.createSubmitLabel));",
		"    await tester.pumpAndSettle();",
		"",
		"    expect(find.text('测试任务'), findsOneWidget);",
		"",
		"    await tester.tap(find.text('测试任务').first);",
		"    await tester.pumpAndSettle();",
		"",
		"    expect(find.text(openLiteCopy.editPageTitle), findsOneWidget);",
		"    await tester.enterText(find.byKey(const Key('title-field')), '更新后的测试任务');",
		"    await tester.enterText(find.byKey(const Key('note-field')), '这是一个更新后的测试备注');",
		"    await tester.tap(find.text(openLiteCopy.doneFilterLabel));",
		"    await tester.pumpAndSettle();",
		"    await tester.tap(find.text(openLiteCopy.editSubmitLabel));",
		"    await tester.pumpAndSettle();",
		"",
		"    expect(find.text('更新后的测试任务'), findsOneWidget);",
		"    expect(find.textContaining(openLiteCopy.doneFilterLabel), findsWidgets);",
		"  });",
		"}",
	}, "\n") + "\n"
}

func renderProjectTaskTagTest(packageName, appClassName string) string {
	return strings.Join([]string{
		"import 'package:flutter/material.dart';",
		"import 'package:flutter_test/flutter_test.dart';",
		"",
		"import 'package:" + packageName + "/main.dart';",
		"import 'package:" + packageName + "/models/dashboard_summary.dart';",
		"import 'package:" + packageName + "/models/project.dart';",
		"import 'package:" + packageName + "/models/tag.dart';",
		"import 'package:" + packageName + "/models/task.dart';",
		"import 'package:" + packageName + "/models/task_tag_link.dart';",
		"import 'package:" + packageName + "/repositories/record_repository.dart';",
		"import 'package:" + packageName + "/template/open_lite_copy.dart';",
		"",
		"void main() {",
		"  testWidgets('relation-rich app supports create and inspect task flow', (WidgetTester tester) async {",
		"    final repository = InMemoryRecordRepository(",
		"      projects: [",
		"        Project(projectId: 'project-alpha', title: 'Project Alpha', status: ProjectStatus.active),",
		"      ],",
		"      tasks: [",
		"        Task(taskId: 'task-seed', projectId: 'project-alpha', title: '现有任务', status: TaskStatus.todo, note: '已有备注'),",
		"      ],",
		"      tags: [",
		"        Tag(tagId: 'tag-priority', name: '高优先级'),",
		"      ],",
		"      links: [",
		"        TaskTagLink(linkId: 'task-seed_tag-priority', taskId: 'task-seed', tagId: 'tag-priority'),",
		"      ],",
		"      summaries: [",
		"        DashboardSummary(projectId: 'project-alpha', openTaskCount: 1, doneTaskCount: 0, taggedTaskCount: 1),",
		"      ],",
		"    );",
		"",
		"    await tester.pumpWidget(" + appClassName + "(repository: repository));",
		"    await tester.pumpAndSettle();",
		"",
		"    expect(find.text(openLiteCopy.appTitle), findsOneWidget);",
		"    expect(find.text('Project Alpha'), findsOneWidget);",
		"    expect(find.text(openLiteCopy.createPrimaryActionLabel), findsOneWidget);",
		"",
		"    await tester.tap(find.text(openLiteCopy.createPrimaryActionLabel));",
		"    await tester.pumpAndSettle();",
		"",
		"    await tester.enterText(find.byKey(const Key('title-field')), '新增任务');",
		"    await tester.enterText(find.byKey(const Key('note-field')), '补充说明');",
		"    await tester.tap(find.text(openLiteCopy.createSubmitLabel));",
		"    await tester.pumpAndSettle();",
		"",
		"    await tester.tap(find.text(openLiteCopy.viewAllActionLabel));",
		"    await tester.pumpAndSettle();",
		"",
		"    expect(find.text(openLiteCopy.listPageTitle), findsOneWidget);",
		"    expect(find.text('新增任务'), findsOneWidget);",
		"",
		"    await tester.tap(find.text('新增任务').first);",
		"    await tester.pumpAndSettle();",
		"",
		"    expect(find.text(openLiteCopy.detailPageTitle), findsOneWidget);",
		"    expect(find.text('新增任务'), findsOneWidget);",
		"    expect(find.text('补充说明'), findsOneWidget);",
		"  });",
		"}",
	}, "\n") + "\n"
}

func renderInventorySheetLineItemTest(packageName, appClassName string) string {
	return strings.Join([]string{
		"import 'package:flutter/material.dart';",
		"import 'package:flutter_test/flutter_test.dart';",
		"",
		"import 'package:" + packageName + "/main.dart';",
		"import 'package:" + packageName + "/models/inventory_sheet.dart';",
		"import 'package:" + packageName + "/models/line_item.dart';",
		"import 'package:" + packageName + "/models/sku.dart';",
		"import 'package:" + packageName + "/models/warehouse.dart';",
		"import 'package:" + packageName + "/repositories/record_repository.dart';",
		"import 'package:" + packageName + "/template/open_lite_copy.dart';",
		"",
		"void main() {",
		"  testWidgets('inventory relation-rich app supports overview and inspection flow', (WidgetTester tester) async {",
		"    final repository = InMemoryRecordRepository(",
		"      inventorySheets: [",
		"        InventorySheet(",
		"          sheetId: 'sheet-seed',",
		"          warehouseId: 'warehouse-east',",
		"          status: InventorySheetStatus.checking,",
		"          countedOn: DateTime(2026, 4, 14),",
		"          note: '夜班复盘完成',",
		"        ),",
		"      ],",
		"      lineItems: [",
		"        LineItem(",
		"          lineItemId: 'line-seed',",
		"          sheetId: 'sheet-seed',",
		"          skuId: 'sku-001',",
		"          expectedQty: 18,",
		"          countedQty: 15,",
		"          varianceQty: -3,",
		"        ),",
		"      ],",
		"      skus: [",
		"        Sku(skuId: 'sku-001', name: 'SKU-001', category: '辅料', reorderThreshold: 16),",
		"      ],",
		"      warehouses: [",
		"        Warehouse(warehouseId: 'warehouse-east', name: '华东一号仓', location: 'A1 冷链区'),",
		"      ],",
		"    );",
		"",
		"    await tester.pumpWidget(" + appClassName + "(repository: repository));",
		"    await tester.pumpAndSettle();",
		"",
		"    expect(find.text(openLiteCopy.appTitle), findsOneWidget);",
		"    expect(find.text(openLiteCopy.homeSummaryTitle), findsOneWidget);",
		"    expect(find.text('华东一号仓'), findsOneWidget);",
		"",
		"    await tester.tap(find.text(openLiteCopy.createPrimaryActionLabel));",
		"    await tester.pumpAndSettle();",
		"    expect(find.text(openLiteCopy.createPageTitle), findsOneWidget);",
		"    expect(find.byKey(const Key('add-line-item-button')), findsOneWidget);",
		"",
		"    await tester.pageBack();",
		"    await tester.pumpAndSettle();",
		"",
		"    await tester.tap(find.text(openLiteCopy.viewAllActionLabel));",
		"    await tester.pumpAndSettle();",
		"",
		"    expect(find.text(openLiteCopy.listPageTitle), findsOneWidget);",
		"    expect(find.widgetWithText(ListTile, '华东一号仓'), findsOneWidget);",
		"",
		"    await tester.tap(find.widgetWithText(ListTile, '华东一号仓'));",
		"    await tester.pumpAndSettle();",
		"",
		"    expect(find.text(openLiteCopy.detailPageTitle), findsOneWidget);",
		"    await tester.scrollUntilVisible(",
		"      find.text('SKU-001'),",
		"      200,",
		"      scrollable: find.byType(Scrollable).first,",
		"    );",
		"    await tester.pumpAndSettle();",
		"    expect(find.text('SKU-001'), findsOneWidget);",
		"    expect(find.textContaining(openLiteCopy.varianceQtyLabel), findsWidgets);",
		"  });",
		"}",
	}, "\n") + "\n"
}
