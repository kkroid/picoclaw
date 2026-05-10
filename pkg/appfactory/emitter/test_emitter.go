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
	content := renderTestForProfile(profile, packageName, dm, cfg)
	return TestEmitResult{
		Content:  content,
		FilePath: "test/widget_test.dart",
	}, true
}

// renderTestForProfile 按 profile 渲染 widget_test.dart 的完整内容。
func renderTestForProfile(profile copyProfile, packageName string, dm appprepare.DomainModel, cfg TestEmitConfig) string {
	switch profile {
	case copyProfileProjectTaskTag:
		return renderProjectTaskTagTest(packageName, resolveAppClassName(cfg.AppClassName, "AppFactoryApp"))
	case copyProfileInventorySheetLineItem:
		return renderInventorySheetLineItemTest(packageName, resolveAppClassName(cfg.AppClassName, "AppFactoryApp"))
	default:
		return renderGenericTest(dm, packageName, resolveAppClassName(cfg.AppClassName, "MyApp"), resolveRepositoryType(cfg.RepositoryType))
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

func renderGenericTest(dm appprepare.DomainModel, packageName, appClassName, repositoryType string) string {
	contract := genericTestRecordContract(dm)
	if contract.modelClassName == "" || contract.primaryField.name == "" {
		return strings.Join([]string{
			"import 'package:flutter_test/flutter_test.dart';",
			"",
			"import 'package:" + packageName + "/main.dart';",
			"import 'package:" + packageName + "/repositories/record_repository.dart';",
			"import 'package:" + packageName + "/template/open_lite_copy.dart';",
			"",
			"void main() {",
			"  testWidgets('open lite app renders primary flow', (WidgetTester tester) async {",
			"    final repository = " + repositoryType + "();",
			"    await repository.init();",
			"",
			"    await tester.pumpWidget(" + appClassName + "(repository: repository));",
			"    await tester.pumpAndSettle();",
			"",
			"    expect(find.text(openLiteCopy.appTitle), findsOneWidget);",
			"    expect(find.text(openLiteCopy.createPrimaryActionLabel), findsOneWidget);",
			"  });",
			"}",
		}, "\n") + "\n"
	}
	repositoryConstructor := repositoryType + "(seedRecords: [" + contract.modelClassName + "(" + strings.Join(contract.seedArgs, ", ") + ")])"
	lines := []string{
		"import 'package:flutter/foundation.dart';",
		"import 'package:flutter_test/flutter_test.dart';",
		"",
		"import 'package:" + packageName + "/main.dart';",
		"import 'package:" + packageName + "/models/record.dart';",
		"import 'package:" + packageName + "/repositories/record_repository.dart';",
		"import 'package:" + packageName + "/template/open_lite_copy.dart';",
		"",
		"void main() {",
		"  testWidgets('open lite app supports schema-driven create and inspect flow', (WidgetTester tester) async {",
		"    final repository = " + repositoryConstructor + ";",
		"    await repository.init();",
		"",
		"    await tester.pumpWidget(" + appClassName + "(repository: repository));",
		"    await tester.pumpAndSettle();",
		"",
		"    expect(find.text(openLiteCopy.appTitle), findsOneWidget);",
		"    expect(find.text(openLiteCopy.createPrimaryActionLabel), findsOneWidget);",
		"    expect(find.text('" + contract.seedDisplay + "'), findsOneWidget);",
	}
	for _, summaryGetter := range genericTestSummaryLabelGetters(dm) {
		lines = append(lines, "    expect(find.textContaining(openLiteCopy."+summaryGetter+"), findsWidgets);")
	}
	lines = append(lines,
		"",
		"    await tester.tap(find.text(openLiteCopy.createPrimaryActionLabel));",
		"    await tester.pumpAndSettle();",
		"",
		"    expect(find.text(openLiteCopy.createPageTitle), findsOneWidget);",
		"    expect(find.text(openLiteCopy.titleFieldLabel), findsOneWidget);",
		"    await tester.enterText(find.byKey(const Key('title-field')), '"+contract.createInput+"');",
	)
	if contract.secondaryField.name != "" {
		lines = append(lines,
			"    expect(find.text(openLiteCopy.categoryFieldLabel), findsOneWidget);",
			"    await tester.enterText(find.byKey(const Key('secondary-field')), '测试分类');",
		)
	}
	if contract.statusField.name != "" {
		lines = append(lines, "    expect(find.text(openLiteCopy.detailStatusLabel), findsOneWidget);")
	}
	if contract.timeField.name != "" {
		lines = append(lines, "    expect(find.text(openLiteCopy.dateFieldLabel), findsOneWidget);")
	}
	for _, field := range contract.numericFields {
		lines = append(lines,
			"    expect(find.text(openLiteCopy."+genericTestNumericFieldLabelGetter(field)+"), findsOneWidget);",
			"    await tester.enterText(find.byKey(const Key('"+genericTestNumericFieldKey(field)+"')), '"+genericTestCreateValue(field)+"');",
		)
	}
	for _, field := range contract.booleanFields {
		lines = append(lines,
			"    expect(find.text(openLiteCopy."+genericCopyBooleanFieldLabelGetter(field.name)+"), findsOneWidget);",
			"    await tester.tap(find.byKey(const Key('"+genericTestBooleanFieldKey(field)+"')));",
		)
	}
	if contract.noteField.name != "" {
		lines = append(lines,
			"    expect(find.text(openLiteCopy.noteFieldLabel), findsOneWidget);",
			"    await tester.enterText(find.byKey(const Key('note-field')), '测试备注');",
		)
	}
	lines = append(lines,
		"    await tester.tap(find.text(openLiteCopy.createSubmitLabel));",
		"    await tester.pumpAndSettle();",
		"",
		"    expect(find.text('"+contract.createdDisplay+"'), findsOneWidget);",
		"",
		"    await tester.tap(find.text(openLiteCopy.viewAllActionLabel));",
		"    await tester.pumpAndSettle();",
		"",
		"    expect(find.text(openLiteCopy.listPageTitle), findsOneWidget);",
		"    expect(find.text('"+contract.createdDisplay+"'), findsOneWidget);",
	)
	for _, field := range contract.booleanFields {
		lines = append(lines, "    expect(find.textContaining(openLiteCopy."+genericCopyBooleanFieldLabelGetter(field.name)+"), findsWidgets);")
	}
	if contract.noteField.name != "" || len(contract.booleanFields) > 0 || len(contract.numericFields) > 0 || contract.secondaryField.name != "" {
		lines = append(lines,
			"",
			"    await tester.tap(find.text('"+contract.createdDisplay+"').first);",
			"    await tester.pumpAndSettle();",
			"",
			"    expect(find.text(openLiteCopy.detailPageTitle), findsOneWidget);",
			"    expect(find.text('"+contract.createdDisplay+"'), findsOneWidget);",
		)
		if contract.noteField.name != "" {
			lines = append(lines, "    expect(find.text('测试备注'), findsOneWidget);")
		}
		for _, field := range contract.booleanFields {
			lines = append(lines,
				"    expect(find.text(openLiteCopy."+genericCopyBooleanDetailLabelGetter(field.name)+"), findsOneWidget);",
				"    expect(find.text(openLiteCopy.booleanValueLabel(true)), findsWidgets);",
			)
		}
	}
	lines = append(lines,
		"  });",
		"}",
	)
	return strings.Join(lines, "\n") + "\n"
}

type genericTestRecordSpec struct {
	modelClassName string
	primaryField   genericTestField
	secondaryField genericTestField
	statusField    genericTestField
	timeField      genericTestField
	numericFields  []genericTestField
	booleanFields  []genericTestField
	noteField      genericTestField
	seedArgs       []string
	seedDisplay    string
	createInput    string
	createdDisplay string
}

type genericTestField struct {
	name      string
	fieldType string
	role      string
}

func genericTestRecordContract(dm appprepare.DomainModel) genericTestRecordSpec {
	for i := range dm.Entities {
		entity := dm.Entities[i]
		if genericTestIsSummaryEntity(entity) {
			continue
		}
		className := genericTestEntityClassName(entity)
		primaryField := genericTestPrimaryField(entity.Fields)
		if className != "" && primaryField.name != "" {
			contract := genericTestRecordSpec{modelClassName: className, primaryField: primaryField}
			contract.seedDisplay = genericTestDisplayValue(primaryField, "seed")
			contract.createInput = genericTestInputValue(primaryField)
			contract.createdDisplay = genericTestDisplayValue(primaryField, "created")
			contract.seedArgs = append(contract.seedArgs, primaryField.name+": "+genericTestSeedValue(primaryField))
			for _, field := range genericTestFields(entity.Fields) {
				if field.name == primaryField.name || genericTestIsIdentifierField(field.name) {
					continue
				}
				switch {
				case strings.EqualFold(field.role, "secondary_text") && genericTestIsStringType(field.fieldType):
					contract.secondaryField = field
					contract.seedArgs = append(contract.seedArgs, field.name+": '测试分类'")
				case strings.EqualFold(field.role, "status"):
					contract.statusField = field
				case genericTestIsTimeType(field.fieldType):
					contract.timeField = field
				case genericTestIsNumericType(field.fieldType) && genericTestNumericFieldRole(field) != "":
					field.role = genericTestNumericFieldRole(field)
					contract.numericFields = append(contract.numericFields, field)
					contract.seedArgs = append(contract.seedArgs, field.name+": "+genericTestSeedValue(field))
				case genericTestIsBooleanType(field.fieldType):
					contract.booleanFields = append(contract.booleanFields, field)
					contract.seedArgs = append(contract.seedArgs, field.name+": true")
				case strings.EqualFold(field.role, "note") && genericTestIsStringType(field.fieldType):
					contract.noteField = field
					contract.seedArgs = append(contract.seedArgs, field.name+": '测试备注'")
				}
			}
			return contract
		}
	}
	return genericTestRecordSpec{}
}

func genericTestIsSummaryEntity(entity appprepare.DataEntity) bool {
	entityID := strings.ToLower(strings.TrimSpace(entity.EntityID))
	return strings.Contains(entityID, "dashboard") || strings.Contains(entityID, "summary")
}

func genericTestEntityClassName(entity appprepare.DataEntity) string {
	name := strings.TrimSpace(entity.Name)
	if genericTestHasASCII(name) {
		if candidate := genericTestSnakeToPascal(name); candidate != "" {
			return candidate
		}
	}
	entityID := strings.TrimSpace(entity.EntityID)
	entityID = strings.TrimPrefix(entityID, "entity-")
	entityID = strings.ReplaceAll(entityID, "-", "_")
	return genericTestSnakeToPascal(entityID)
}

func genericTestHasASCII(value string) bool {
	for _, r := range strings.ToLower(value) {
		if r >= 'a' && r <= 'z' {
			return true
		}
	}
	return false
}

func genericTestFields(fields []appprepare.DataField) []genericTestField {
	result := make([]genericTestField, 0, len(fields))
	for _, field := range fields {
		dartName := genericTestSnakeToCamel(field.Name)
		if dartName == "" {
			continue
		}
		result = append(result, genericTestField{
			name:      dartName,
			fieldType: genericTestNormalizeFieldType(field.Type),
			role:      strings.ToLower(strings.TrimSpace(field.Role)),
		})
	}
	return result
}

func genericTestPrimaryField(fields []appprepare.DataField) genericTestField {
	for _, field := range genericTestFields(fields) {
		if strings.EqualFold(strings.TrimSpace(field.role), "primary_text") {
			return field
		}
	}
	for _, field := range genericTestFields(fields) {
		if field.name == "" || genericTestIsIdentifierField(field.name) {
			continue
		}
		if genericTestIsStringType(field.fieldType) {
			return field
		}
	}
	return genericTestField{}
}

func genericTestNormalizeFieldType(fieldType string) string {
	trimmed := strings.ToLower(strings.TrimSpace(fieldType))
	if strings.HasPrefix(trimmed, "enum[") {
		return "enum"
	}
	switch trimmed {
	case "int", "integer":
		return "int"
	case "number", "double", "float":
		return "double"
	case "bool", "boolean":
		return "bool"
	case "date", "datetime":
		return "DateTime"
	default:
		return "String"
	}
}

func genericTestIsStringType(fieldType string) bool {
	return genericTestNormalizeFieldType(fieldType) == "String"
}

func genericTestIsTimeType(fieldType string) bool {
	return genericTestNormalizeFieldType(fieldType) == "DateTime"
}

func genericTestIsNumericType(fieldType string) bool {
	switch genericTestNormalizeFieldType(fieldType) {
	case "int", "double", "num":
		return true
	default:
		return false
	}
}

func genericTestIsBooleanType(fieldType string) bool {
	return genericTestNormalizeFieldType(fieldType) == "bool"
}

func genericTestSeedValue(field genericTestField) string {
	switch genericTestNormalizeFieldType(field.fieldType) {
	case "int":
		return "42"
	case "double", "num":
		return "4.5"
	case "bool":
		return "true"
	default:
		if strings.EqualFold(field.role, "primary_text") {
			return "'测试记录'"
		}
		return "'测试值'"
	}
}

func genericTestInputValue(field genericTestField) string {
	switch genericTestNormalizeFieldType(field.fieldType) {
	case "int":
		return "43"
	case "double", "num":
		return "4.6"
	default:
		return "新增记录"
	}
}

func genericTestCreateValue(field genericTestField) string {
	switch genericTestNormalizeFieldType(field.fieldType) {
	case "int":
		return "43"
	default:
		return "4.6"
	}
}

func genericTestDisplayValue(field genericTestField, mode string) string {
	if mode == "created" {
		switch genericTestNormalizeFieldType(field.fieldType) {
		case "int":
			return "43"
		case "double", "num":
			return "4.6"
		default:
			return "新增记录"
		}
	}
	switch genericTestNormalizeFieldType(field.fieldType) {
	case "int":
		return "42"
	case "double", "num":
		return "4.5"
	default:
		return "测试记录"
	}
}

func genericTestNumericFieldRole(field genericTestField) string {
	role := strings.ToLower(strings.TrimSpace(field.role))
	switch role {
	case "rating", "duration", "amount", "number", "quantity":
		return role
	}
	lowerName := strings.ToLower(strings.TrimSpace(field.name))
	switch {
	case strings.Contains(lowerName, "rating") || strings.Contains(lowerName, "score"):
		return "rating"
	case strings.Contains(lowerName, "duration") || strings.Contains(lowerName, "minutes") || strings.Contains(lowerName, "hours"):
		return "duration"
	case strings.Contains(lowerName, "amount") || strings.Contains(lowerName, "price") || strings.Contains(lowerName, "cost") || strings.Contains(lowerName, "weight"):
		return "amount"
	default:
		return ""
	}
}

func genericTestNumericFieldLabelGetter(field genericTestField) string {
	switch strings.ToLower(strings.TrimSpace(field.role)) {
	case "rating":
		return "ratingFieldLabel"
	case "duration":
		return "durationFieldLabel"
	default:
		return "amountFieldLabel"
	}
}

func genericTestNumericFieldKey(field genericTestField) string {
	role := strings.ToLower(strings.TrimSpace(field.role))
	if role != "" {
		return role + "-field"
	}
	return genericTestKebabFieldKey(field.name)
}

func genericTestBooleanFieldKey(field genericTestField) string {
	return genericTestKebabFieldKey(field.name)
}

func genericTestKebabFieldKey(name string) string {
	snakeName := genericTestCamelToSnake(name)
	if snakeName == "" {
		return "field"
	}
	return strings.ReplaceAll(snakeName, "_", "-") + "-field"
}

func genericTestCamelToSnake(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	var builder strings.Builder
	for index, r := range trimmed {
		if r >= 'A' && r <= 'Z' {
			if index > 0 {
				builder.WriteByte('_')
			}
			builder.WriteRune(r + ('a' - 'A'))
			continue
		}
		builder.WriteRune(r)
	}
	return builder.String()
}

func genericTestSummaryLabelGetters(dm appprepare.DomainModel) []string {
	getters := make([]string, 0)
	for _, entity := range dm.Entities {
		if !genericTestIsSummaryEntity(entity) {
			continue
		}
		for _, field := range entity.Fields {
			getter := genericCopySummaryMetricLabelGetter(field.Name)
			if getter != "" {
				getters = append(getters, getter)
			}
		}
	}
	return getters
}

func genericTestSnakeToCamel(value string) string {
	parts := strings.Split(strings.TrimSpace(value), "_")
	for i, part := range parts {
		if part == "" {
			continue
		}
		if i == 0 {
			parts[i] = strings.ToLower(part[:1]) + part[1:]
		} else {
			parts[i] = strings.ToUpper(part[:1]) + part[1:]
		}
	}
	return strings.Join(parts, "")
}

func genericTestSnakeToPascal(value string) string {
	parts := strings.Split(strings.TrimSpace(value), "_")
	for i, part := range parts {
		if part == "" {
			continue
		}
		parts[i] = strings.ToUpper(part[:1]) + part[1:]
	}
	return strings.Join(parts, "")
}

func genericTestIsIdentifierField(name string) bool {
	trimmedName := strings.TrimSpace(name)
	return trimmedName == "id" || strings.HasSuffix(trimmedName, "Id") || strings.HasSuffix(trimmedName, "ID")
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
