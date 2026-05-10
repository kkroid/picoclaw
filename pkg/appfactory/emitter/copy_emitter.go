// Package emitter 提供确定性代码生成器，用于替代 LLM 生成路径。
// M3 阶段首批上线的 Emitter 覆盖 copy、branding 和 test 三个表面。
package emitter

import (
	"strings"

	appprepare "github.com/sipeed/picoclaw/pkg/appfactory/prepare"
)

// CopyEmitResult 表示 CopyEmitter 的输出。
type CopyEmitResult struct {
	Content  string
	FilePath string
}

// copyProfile 表示 copy 模板的领域变体。
type copyProfile string

const (
	copyProfileGeneric                copyProfile = "generic"
	copyProfileProjectTaskTag         copyProfile = "project-task-tag"
	copyProfileInventorySheetLineItem copyProfile = "inventory-sheet-line-item"
)

// EmitCopy 从 DomainModel 确定性生成 open_lite_copy.dart。
// 返回 (result, true) 表示成功；(zero, false) 表示当前 DomainModel 不在可 emit 范围内。
func EmitCopy(dm appprepare.DomainModel) (CopyEmitResult, bool) {
	profile := identifyCopyProfile(dm)
	if profile == "" {
		return CopyEmitResult{}, false
	}
	appTitle := strings.TrimSpace(dm.DomainCopy.Title)
	if appTitle == "" {
		return CopyEmitResult{}, false
	}
	content := renderCopyForProfile(profile, dm, appTitle)
	return CopyEmitResult{
		Content:  content,
		FilePath: "lib/template/open_lite_copy.dart",
	}, true
}

// identifyCopyProfile 从 DomainModel 的 entity_id 集合判断领域 profile。
func identifyCopyProfile(dm appprepare.DomainModel) copyProfile {
	ids := make(map[string]bool, len(dm.Entities))
	for _, e := range dm.Entities {
		ids[e.EntityID] = true
	}
	// relation-rich: project-task-tag
	if ids["entity-project"] && ids["entity-task"] && ids["entity-tag"] {
		return copyProfileProjectTaskTag
	}
	// relation-rich: inventory-sheet-line-item
	if ids["entity-inventory-sheet"] && ids["entity-line-item"] && ids["entity-sku"] {
		return copyProfileInventorySheetLineItem
	}
	// generic：有任意实体即可 emit
	if len(dm.Entities) > 0 {
		return copyProfileGeneric
	}
	return ""
}

// renderCopyForProfile 按 profile 渲染 open_lite_copy.dart 的完整内容。
func renderCopyForProfile(profile copyProfile, dm appprepare.DomainModel, appTitle string) string {
	switch profile {
	case copyProfileProjectTaskTag:
		return renderProjectTaskTagCopy(appTitle)
	case copyProfileInventorySheetLineItem:
		return renderInventorySheetLineItemCopy(appTitle)
	default:
		return renderGenericCopy(dm, appTitle)
	}
}

func renderGenericCopy(dm appprepare.DomainModel, appTitle string) string {
	entity := genericCopyPrimaryEntity(dm)
	summaryEntity := genericCopySummaryEntity(dm)
	entityName := genericCopyEntityName(entity, "记录")
	summaryName := entityName + "摘要"
	if summaryEntity != nil && strings.TrimSpace(summaryEntity.Name) != "" {
		summaryName = strings.TrimSpace(summaryEntity.Name)
	}
	primaryField := genericCopyFieldByRole(entity, "primary_text", "title", "name")
	secondaryField := genericCopyFieldByRole(entity, "secondary_text", "category", "group")
	statusField := genericCopyFieldByRole(entity, "status")
	timeField := genericCopyFieldByRole(entity, "due_date", "date", "time")
	ratingField := genericCopyFieldByRole(entity, "rating")
	durationField := genericCopyFieldByRole(entity, "duration")
	amountField := genericCopyFieldByRole(entity, "amount", "number", "quantity")
	noteField := genericCopyFieldByRole(entity, "note")
	primaryLabel := genericCopyFieldLabel(primaryField, "标题")
	secondaryLabel := genericCopyFieldLabel(secondaryField, "分类")
	statusLabel := genericCopyFieldLabel(statusField, "状态")
	timeLabel := genericCopyFieldLabel(timeField, "更新时间")
	ratingLabel := genericCopyFieldLabel(ratingField, "评分")
	durationLabel := genericCopyFieldLabel(durationField, "时长")
	amountLabel := genericCopyFieldLabel(amountField, "数值")
	noteLabel := genericCopyFieldLabel(noteField, "备注")
	lines := []string{
		"const openLiteCopy = OpenLiteCopy();",
		"",
		"class OpenLiteCopy {",
		"  const OpenLiteCopy();",
		"",
		"  String get appTitle => " + dartSingleQuotedString(appTitle) + ";",
		"",
		"  String get homeSummaryTitle => " + dartSingleQuotedString(summaryName) + ";",
		"",
		"  String summaryCountLabel(int count) => '$count 条" + dartSingleQuotedContent(entityName) + "';",
		"",
		"  String get createPrimaryActionLabel => " + dartSingleQuotedString("新建"+entityName) + ";",
		"",
		"  String get viewAllActionLabel => '查看全部';",
		"",
		"  String get recentRecordsTitle => " + dartSingleQuotedString("最近"+entityName) + ";",
		"",
		"  String get recentRecordsLabel => recentRecordsTitle;",
		"",
		"  String recentRecordsCountLabel(int count) => '$count 条" + dartSingleQuotedContent(entityName) + "';",
		"",
		"  String get homeEmptyTitle => " + dartSingleQuotedString("还没有"+entityName) + ";",
		"",
		"  String get homeEmptyDescription => " + dartSingleQuotedString("先创建一条"+entityName+"，首页摘要和列表会自动刷新。") + ";",
		"",
		"  String get listPageTitle => " + dartSingleQuotedString("全部"+entityName) + ";",
		"",
		"  String get listFilterTitle => '状态筛选';",
		"",
		"  String listCountLabel(int visibleCount, int totalCount) =>",
		"      '当前展示 $visibleCount / $totalCount 条" + dartSingleQuotedContent(entityName) + "';",
		"",
		"  String get filteredEmptyTitle => " + dartSingleQuotedString("当前筛选下还没有"+entityName+"。") + ";",
		"",
		"  String get filteredEmptyDescription => " + dartSingleQuotedString("可以切回全部，或者先新增一条符合当前状态的"+entityName+"。") + ";",
		"",
		"  String get clearFilterActionLabel => '清除筛选';",
		"",
		"  String get listEmptyLabel => " + dartSingleQuotedString("暂无"+entityName+"，请先从首页新增一条。") + ";",
		"",
		"  String get createPageTitle => " + dartSingleQuotedString("新建"+entityName) + ";",
		"",
		"  String get editPageTitle => " + dartSingleQuotedString("编辑"+entityName) + ";",
		"",
		"  String get titleFieldLabel => " + dartSingleQuotedString(primaryLabel) + ";",
		"",
		"  String get titleLabel => titleFieldLabel;",
		"",
		"  String get titleFieldRequiredError => " + dartSingleQuotedString("请输入"+primaryLabel) + ";",
		"",
		"  String get categoryFieldLabel => " + dartSingleQuotedString(secondaryLabel) + ";",
		"",
		"  String get dateFieldLabel => " + dartSingleQuotedString(timeLabel) + ";",
		"",
		"  String get noteFieldLabel => " + dartSingleQuotedString(noteLabel) + ";",
		"",
		"  String get noteLabel => noteFieldLabel;",
		"",
		"  String get noteFieldHint => " + dartSingleQuotedString("补充"+entityName+"的说明或下一步动作") + ";",
		"",
		"  String get createSubmitLabel => '保存';",
		"",
		"  String get editSubmitLabel => '更新';",
		"",
		"  String get detailPageTitle => " + dartSingleQuotedString(entityName+"详情") + ";",
		"",
		"  String get editActionLabel => '编辑';",
		"",
		"  String get deleteActionLabel => '删除';",
		"",
		"  String get deleteDialogTitle => " + dartSingleQuotedString("删除"+entityName) + ";",
		"",
		"  String get deleteDialogMessage => '删除后不可恢复，确认继续吗？';",
		"",
		"  String get detailCategoryLabel => categoryFieldLabel;",
		"",
		"  String get detailProjectLabel => detailCategoryLabel;",
		"",
		"  String get detailDateLabel => dateFieldLabel;",
		"",
	}
	if ratingField != nil {
		lines = append(lines,
			"  String get ratingFieldLabel => "+dartSingleQuotedString(ratingLabel)+";",
			"",
			"  String get detailRatingLabel => ratingFieldLabel;",
			"",
		)
	}
	if durationField != nil {
		lines = append(lines,
			"  String get durationFieldLabel => "+dartSingleQuotedString(durationLabel)+";",
			"",
			"  String get detailDurationLabel => durationFieldLabel;",
			"",
		)
	}
	if amountField != nil {
		lines = append(lines,
			"  String get amountFieldLabel => "+dartSingleQuotedString(amountLabel)+";",
			"",
			"  String get detailAmountLabel => amountFieldLabel;",
			"",
		)
	}
	if summaryEntity != nil {
		for _, field := range summaryEntity.Fields {
			getterName := genericCopySummaryMetricLabelGetter(field.Name)
			if getterName == "" {
				continue
			}
			lines = append(lines,
				"  String get "+getterName+" => "+dartSingleQuotedString(genericCopyFieldLabel(&field, field.Name))+";",
				"",
			)
		}
	}
	lines = append(lines,
		"  String get detailNoteLabel => noteFieldLabel;",
		"",
		"  String get emptyNoteLabel => '暂无备注';",
	)
	if statusField != nil {
		lines = append(lines,
			"",
			"  String get detailStatusLabel => "+dartSingleQuotedString(statusLabel)+";",
			"",
			"  String get allFilterLabel => '全部';",
			"",
			"  String get inboxFilterLabel => '待整理';",
			"",
			"  String get inProgressFilterLabel => '进行中';",
			"",
			"  String get doneFilterLabel => '已完成';",
			"",
			"  String statusLabel(dynamic status) {",
			"    final statusName = status is Enum ? status.name : status.toString();",
			"    switch (statusName) {",
			"      case 'inbox':",
			"      case 'todo':",
			"      case 'pending':",
			"      case 'planned':",
			"      case 'needsWater':",
			"        return inboxFilterLabel;",
			"      case 'inProgress':",
			"      case 'watching':",
			"        return inProgressFilterLabel;",
			"      case 'done':",
			"      case 'completed':",
			"      case 'watched':",
			"      case 'watered':",
			"        return doneFilterLabel;",
			"      default:",
			"        return statusName;",
			"    }",
			"  }",
		)
	}
	lines = append(lines, "}")
	return strings.Join(lines, "\n") + "\n"
}

func genericCopyPrimaryEntity(dm appprepare.DomainModel) *appprepare.DataEntity {
	for index := range dm.Entities {
		if strings.TrimSpace(dm.Entities[index].Source) != "derived" {
			return &dm.Entities[index]
		}
	}
	if len(dm.Entities) == 0 {
		return nil
	}
	return &dm.Entities[0]
}

func genericCopySummaryEntity(dm appprepare.DomainModel) *appprepare.DataEntity {
	for index := range dm.Entities {
		if strings.TrimSpace(dm.Entities[index].Source) == "derived" {
			return &dm.Entities[index]
		}
	}
	return nil
}

func genericCopyEntityName(entity *appprepare.DataEntity, fallback string) string {
	if entity == nil {
		return fallback
	}
	if name := strings.TrimSpace(entity.Name); name != "" {
		return name
	}
	return fallback
}

func genericCopyFieldByRole(entity *appprepare.DataEntity, roles ...string) *appprepare.DataField {
	if entity == nil {
		return nil
	}
	for _, role := range roles {
		for index := range entity.Fields {
			if strings.EqualFold(strings.TrimSpace(entity.Fields[index].Role), strings.TrimSpace(role)) {
				return &entity.Fields[index]
			}
		}
	}
	for _, role := range roles {
		for index := range entity.Fields {
			name := strings.ToLower(strings.TrimSpace(entity.Fields[index].Name))
			if name == strings.ToLower(strings.TrimSpace(role)) || strings.Contains(name, strings.ToLower(strings.TrimSpace(role))) {
				return &entity.Fields[index]
			}
		}
	}
	return nil
}

func genericCopyFieldLabel(field *appprepare.DataField, fallback string) string {
	if field == nil {
		return fallback
	}
	if description := strings.TrimSpace(field.Description); description != "" {
		return description
	}
	if name := strings.TrimSpace(field.Name); name != "" {
		return name
	}
	return fallback
}

func genericCopySummaryMetricLabelGetter(fieldName string) string {
	camelName := genericCopySnakeToCamel(fieldName)
	if camelName == "" {
		return ""
	}
	return "summary" + strings.ToUpper(camelName[:1]) + camelName[1:] + "Label"
}

func genericCopySnakeToCamel(value string) string {
	parts := strings.Split(strings.TrimSpace(value), "_")
	for index, part := range parts {
		if part == "" {
			continue
		}
		if index == 0 {
			parts[index] = strings.ToLower(part[:1]) + part[1:]
			continue
		}
		parts[index] = strings.ToUpper(part[:1]) + part[1:]
	}
	return strings.Join(parts, "")
}

func dartSingleQuotedString(value string) string {
	return "'" + dartSingleQuotedContent(value) + "'"
}

func dartSingleQuotedContent(value string) string {
	replacer := strings.NewReplacer("\\", "\\\\", "$", "\\$", "'", "\\'", "\r", " ", "\n", " ")
	return replacer.Replace(strings.TrimSpace(value))
}

func renderProjectTaskTagCopy(appTitle string) string {
	return strings.Join([]string{
		"import '../models/task.dart';",
		"",
		"const openLiteCopy = OpenLiteCopy();",
		"",
		"class OpenLiteCopy {",
		"  const OpenLiteCopy();",
		"",
		"  String get appTitle => '" + appTitle + "';",
		"",
		"  String get homeSummaryTitle => '项目看板摘要';",
		"",
		"  String summaryCountLabel(int count) => '覆盖 $count 个项目';",
		"",
		"  String get createPrimaryActionLabel => '新建任务';",
		"",
		"  String get viewAllActionLabel => '查看任务列表';",
		"",
		"  String get recentRecordsTitle => '重点任务';",
		"",
		"  String recentRecordsCountLabel(int count) => '$count 条重点任务';",
		"",
		"  String get homeEmptyTitle => '还没有项目任务数据';",
		"",
		"  String get homeEmptyDescription => '先创建项目、任务和标签，首页会自动汇总项目进度与重点标签。';",
		"",
		"  String get listPageTitle => '任务列表';",
		"",
		"  String get listFilterTitle => '项目 / 标签 / 状态筛选';",
		"",
		"  String listCountLabel(int visibleCount) => '当前筛选结果 $visibleCount 条任务';",
		"",
		"  String get filteredEmptyTitle => '当前筛选条件下还没有任务';",
		"",
		"  String get filteredEmptyDescription => '可以切换筛选条件，或者先创建一条匹配当前条件的任务。';",
		"",
		"  String get clearFilterActionLabel => '清除筛选';",
		"",
		"  String get listEmptyLabel => '暂无任务，先创建一条任务。';",
		"",
		"  String get createPageTitle => '新建任务';",
		"",
		"  String get editPageTitle => '编辑任务';",
		"",
		"  String get titleFieldLabel => '任务标题';",
		"",
		"  String get titleFieldRequiredError => '请输入任务标题';",
		"",
		"  String get projectFieldLabel => '所属项目';",
		"",
		"  String get dateFieldLabel => '截止日期';",
		"",
		"  String get noteFieldLabel => '备注';",
		"",
		"  String get noteFieldHint => '补充任务背景、交付物或下一步动作';",
		"",
		"  String get createSubmitLabel => '保存任务';",
		"",
		"  String get editSubmitLabel => '更新任务';",
		"",
		"  String get detailPageTitle => '任务详情';",
		"",
		"  String get editActionLabel => '编辑';",
		"",
		"  String get deleteActionLabel => '删除';",
		"",
		"  String get deleteDialogTitle => '删除任务';",
		"",
		"  String get deleteDialogMessage => '删除后不可恢复，确认继续吗？';",
		"",
		"  String get detailProjectLabel => '所属项目';",
		"",
		"  String get detailStatusLabel => '任务状态';",
		"",
		"  String get detailDateLabel => '截止日期';",
		"",
		"  String get detailNoteLabel => '备注';",
		"",
		"  String get emptyNoteLabel => '暂无备注';",
		"",
		"  String get allFilterLabel => '全部';",
		"",
		"  String get todoFilterLabel => '待办';",
		"",
		"  String get doingFilterLabel => '进行中';",
		"",
		"  String get doneFilterLabel => '已完成';",
		"",
		"  String get tagFilterLabel => '标签';",
		"",
		"  String get projectFilterLabel => '项目';",
		"",
		"  String get statusFilterLabel => '状态';",
		"",
		"  String taskStatusLabel(TaskStatus status) {",
		"    switch (status) {",
		"      case TaskStatus.todo:",
		"        return todoFilterLabel;",
		"      case TaskStatus.doing:",
		"        return doingFilterLabel;",
		"      case TaskStatus.done:",
		"        return doneFilterLabel;",
		"    }",
		"  }",
		"}",
	}, "\n") + "\n"
}

func renderInventorySheetLineItemCopy(appTitle string) string {
	return strings.Join([]string{
		"import '../models/inventory_sheet.dart';",
		"",
		"const openLiteCopy = OpenLiteCopy();",
		"",
		"class OpenLiteCopy {",
		"  const OpenLiteCopy();",
		"",
		"  String get appTitle => '" + appTitle + "';",
		"",
		"  String get homeSummaryTitle => '库存看板摘要';",
		"",
		"  String summaryCountLabel(int count) => '覆盖 $count 个仓库';",
		"",
		"  String get createPrimaryActionLabel => '新建库存单';",
		"",
		"  String get viewAllActionLabel => '查看库存列表';",
		"",
		"  String get recentRecordsTitle => '仓库盘点概览';",
		"",
		"  String recentRecordsCountLabel(int count) => '$count 张库存单';",
		"",
		"  String get homeEmptyTitle => '还没有库存盘点数据';",
		"",
		"  String get homeEmptyDescription => '先创建库存单，首页会自动汇总各仓盘点状态与差异项。';",
		"",
		"  String get listPageTitle => '库存单列表';",
		"",
		"  String get listFilterTitle => '仓库 / 状态筛选';",
		"",
		"  String listCountLabel(int visibleCount) => '当前筛选结果 $visibleCount 张库存单';",
		"",
		"  String get filteredEmptyTitle => '当前筛选条件下还没有库存单';",
		"",
		"  String get filteredEmptyDescription => '可以清除筛选条件，或创建一张符合当前条件的库存单。';",
		"",
		"  String get clearFilterActionLabel => '清除筛选';",
		"",
		"  String get listEmptyLabel => '暂无库存单，先创建一张库存单。';",
		"",
		"  String get createPageTitle => '新建库存单';",
		"",
		"  String get editPageTitle => '编辑库存单';",
		"",
		"  String get categoryFieldLabel => '盘点仓库';",
		"",
		"  String get dateFieldLabel => '盘点日期';",
		"",
		"  String get noteFieldLabel => '盘点备注';",
		"",
		"  String get noteFieldHint => '补充盘点说明、异常项或交接信息';",
		"",
		"  String get lineItemTitle => '盘点明细';",
		"",
		"  String get skuLabel => 'SKU';",
		"",
		"  String get expectedQtyLabel => '系统数量';",
		"",
		"  String get countedQtyLabel => '实盘数量';",
		"",
		"  String get varianceQtyLabel => '差异数量';",
		"",
		"  String get createSubmitLabel => '保存库存单';",
		"",
		"  String get editSubmitLabel => '更新库存单';",
		"",
		"  String get detailPageTitle => '库存单详情';",
		"",
		"  String get editActionLabel => '编辑';",
		"",
		"  String get detailCategoryLabel => '盘点仓库';",
		"",
		"  String get warehouseLocationLabel => '仓库位置';",
		"",
		"  String get detailStatusLabel => '盘点状态';",
		"",
		"  String get detailDateLabel => '盘点日期';",
		"",
		"  String get detailNoteLabel => '盘点备注';",
		"",
		"  String get emptyNoteLabel => '暂无备注';",
		"",
		"  String get allFilterLabel => '全部';",
		"",
		"  String get inboxFilterLabel => '草稿';",
		"",
		"  String get inProgressFilterLabel => '盘点中';",
		"",
		"  String get doneFilterLabel => '已完成';",
		"",
		"  String statusLabel(InventorySheetStatus status) {",
		"    switch (status) {",
		"      case InventorySheetStatus.draft:",
		"        return inboxFilterLabel;",
		"      case InventorySheetStatus.checking:",
		"        return inProgressFilterLabel;",
		"      case InventorySheetStatus.closed:",
		"        return doneFilterLabel;",
		"    }",
		"  }",
		"}",
	}, "\n") + "\n"
}
