package emitter

import (
	"strings"
	"testing"

	appprepare "github.com/sipeed/oneappfactory/pkg/appfactory/prepare"
)

func TestEmitCopyProjectTaskTag(t *testing.T) {
	dm := appprepare.DomainModel{
		DomainCopy: appprepare.DomainCopy{Title: "项目任务协同 App"},
		Entities: []appprepare.DataEntity{
			{EntityID: "entity-project", Name: "项目"},
			{EntityID: "entity-task", Name: "任务"},
			{EntityID: "entity-tag", Name: "标签"},
			{EntityID: "entity-task-tag-link", Name: "任务标签关联"},
			{EntityID: "entity-project-summary", Name: "项目看板摘要", Source: "derived"},
		},
	}
	result, ok := EmitCopy(dm)
	if !ok {
		t.Fatal("EmitCopy returned false for project-task-tag domain model")
	}
	if result.FilePath != "lib/template/open_lite_copy.dart" {
		t.Fatalf("FilePath = %q, want lib/template/open_lite_copy.dart", result.FilePath)
	}
	// 验证 appTitle 来自 DomainCopy
	if !strings.Contains(result.Content, "String get appTitle => '项目任务协同 App';") {
		t.Fatal("content missing appTitle from DomainCopy.Title")
	}
	// 验证 project-task-tag 特有内容
	for _, marker := range []string{
		"import '../models/task.dart';",
		"taskStatusLabel(TaskStatus status)",
		"case TaskStatus.todo:",
		"case TaskStatus.doing:",
		"case TaskStatus.done:",
		"String get projectFieldLabel",
		"String get tagFilterLabel",
		"String get projectFilterLabel",
		"String get statusFilterLabel",
	} {
		if !strings.Contains(result.Content, marker) {
			t.Fatalf("content missing project-task-tag marker: %s", marker)
		}
	}
	// 验证不包含 generic/inventory 特有内容
	for _, absent := range []string{
		"import '../models/record.dart';",
		"import '../models/inventory_sheet.dart';",
		"RecordStatus",
		"InventorySheetStatus",
	} {
		if strings.Contains(result.Content, absent) {
			t.Fatalf("content should not contain: %s", absent)
		}
	}
}

func TestEmitCopyInventorySheetLineItem(t *testing.T) {
	dm := appprepare.DomainModel{
		DomainCopy: appprepare.DomainCopy{Title: "库存盘点工作台"},
		Entities: []appprepare.DataEntity{
			{EntityID: "entity-inventory-sheet", Name: "库存单"},
			{EntityID: "entity-line-item", Name: "明细项"},
			{EntityID: "entity-sku", Name: "SKU"},
			{EntityID: "entity-warehouse", Name: "仓库"},
			{EntityID: "entity-low-stock-summary", Name: "低库存摘要", Source: "derived"},
		},
	}
	result, ok := EmitCopy(dm)
	if !ok {
		t.Fatal("EmitCopy returned false for inventory domain model")
	}
	if !strings.Contains(result.Content, "String get appTitle => '库存盘点工作台';") {
		t.Fatal("content missing appTitle from DomainCopy.Title")
	}
	for _, marker := range []string{
		"import '../models/inventory_sheet.dart';",
		"statusLabel(InventorySheetStatus status)",
		"case InventorySheetStatus.draft:",
		"case InventorySheetStatus.checking:",
		"case InventorySheetStatus.closed:",
		"String get lineItemTitle",
		"String get skuLabel",
		"String get expectedQtyLabel",
		"String get countedQtyLabel",
		"String get varianceQtyLabel",
		"String get warehouseLocationLabel",
	} {
		if !strings.Contains(result.Content, marker) {
			t.Fatalf("content missing inventory marker: %s", marker)
		}
	}
	for _, absent := range []string{
		"import '../models/record.dart';",
		"import '../models/task.dart';",
		"RecordStatus",
		"TaskStatus",
	} {
		if strings.Contains(result.Content, absent) {
			t.Fatalf("content should not contain: %s", absent)
		}
	}
}

func TestEmitCopyGeneric(t *testing.T) {
	dm := appprepare.DomainModel{
		DomainCopy: appprepare.DomainCopy{Title: "体重记录 App"},
		Entities: []appprepare.DataEntity{
			{EntityID: "entity-weight-record", Name: "体重记录", Fields: []appprepare.DataField{
				{Name: "recorded_at", Type: "date", Role: "date", Description: "记录日期"},
				{Name: "weight", Type: "number", Role: "primary_text", Description: "体重"},
				{Name: "note", Type: "string", Role: "note", Description: "记录备注"},
			}},
			{EntityID: "entity-weight-summary", Name: "体重概览摘要", Source: "derived"},
		},
	}
	result, ok := EmitCopy(dm)
	if !ok {
		t.Fatal("EmitCopy returned false for generic domain model")
	}
	if !strings.Contains(result.Content, "String get appTitle => '体重记录 App';") {
		t.Fatal("content missing appTitle from DomainCopy.Title")
	}
	for _, marker := range []string{
		"String get homeSummaryTitle => '体重概览摘要';",
		"String get createPrimaryActionLabel => '新建体重记录';",
		"String get titleFieldLabel => '体重';",
		"String get dateFieldLabel => '记录日期';",
		"String get noteFieldLabel => '记录备注';",
		"int visibleCount, int totalCount",
	} {
		if !strings.Contains(result.Content, marker) {
			t.Fatalf("content missing generic marker: %s", marker)
		}
	}
	for _, absent := range []string{"RecordStatus", "statusLabel(dynamic status)", "detailStatusLabel", "doneFilterLabel"} {
		if strings.Contains(result.Content, absent) {
			t.Fatalf("generic copy without status should not contain %s: %s", absent, result.Content)
		}
	}
}

func TestEmitCopyGenericWithStatus(t *testing.T) {
	dm := appprepare.DomainModel{
		DomainCopy: appprepare.DomainCopy{Title: "观影清单 App"},
		Entities: []appprepare.DataEntity{
			{EntityID: "entity-movie-record", Name: "观影记录", Fields: []appprepare.DataField{
				{Name: "movie_title", Type: "string", Role: "primary_text", Description: "电影名称"},
				{Name: "genre", Type: "string", Role: "secondary_text", Description: "电影类型"},
				{Name: "watch_status", Type: "enum[planned,watching,watched]", Role: "status", Description: "观看状态"},
				{Name: "rating", Type: "double", Role: "rating", Description: "个人评分"},
				{Name: "review", Type: "string", Role: "note", Description: "观影短评"},
			}},
		},
	}
	result, ok := EmitCopy(dm)
	if !ok {
		t.Fatal("EmitCopy returned false for generic domain model with status")
	}
	for _, marker := range []string{
		"String get titleFieldLabel => '电影名称';",
		"String get categoryFieldLabel => '电影类型';",
		"String get ratingFieldLabel => '个人评分';",
		"String get detailRatingLabel => ratingFieldLabel;",
		"String get detailStatusLabel => '观看状态';",
		"String statusLabel(dynamic status)",
		"case 'planned':",
		"case 'watching':",
		"case 'watched':",
	} {
		if !strings.Contains(result.Content, marker) {
			t.Fatalf("content missing status generic marker: %s", marker)
		}
	}
}

func TestEmitCopyGenericWithDuration(t *testing.T) {
	dm := appprepare.DomainModel{
		DomainCopy: appprepare.DomainCopy{Title: "训练日志 App"},
		Entities: []appprepare.DataEntity{
			{EntityID: "entity-workout-session", Name: "训练记录", Fields: []appprepare.DataField{
				{Name: "workout_name", Type: "string", Role: "primary_text", Description: "训练名称"},
				{Name: "duration_minutes", Type: "int", Role: "duration", Description: "训练时长分钟数"},
				{Name: "note", Type: "string", Role: "note", Description: "补充备注"},
			}},
			{EntityID: "entity-workout-summary", Name: "训练摘要", Source: "derived", Fields: []appprepare.DataField{
				{Name: "total_duration_minutes", Type: "int", Description: "累计训练分钟"},
			}},
		},
	}
	result, ok := EmitCopy(dm)
	if !ok {
		t.Fatal("EmitCopy returned false for generic domain model with duration")
	}
	for _, marker := range []string{
		"String get durationFieldLabel => '训练时长分钟数';",
		"String get detailDurationLabel => durationFieldLabel;",
		"String get summaryTotalDurationMinutesLabel => '累计训练分钟';",
	} {
		if !strings.Contains(result.Content, marker) {
			t.Fatalf("content missing duration generic marker: %s", marker)
		}
	}
}

func TestEmitCopyGenericWithBoolean(t *testing.T) {
	dm := appprepare.DomainModel{
		DomainCopy: appprepare.DomainCopy{Title: "行李清单 App"},
		Entities: []appprepare.DataEntity{
			{EntityID: "entity-packing-item", Name: "行李物品", Fields: []appprepare.DataField{
				{Name: "item_name", Type: "string", Role: "primary_text", Description: "物品名称"},
				{Name: "is_packed", Type: "bool", Role: "flag", Description: "是否已打包"},
			}},
		},
	}
	result, ok := EmitCopy(dm)
	if !ok {
		t.Fatal("EmitCopy returned false for generic domain model with boolean")
	}
	for _, marker := range []string{
		"String booleanValueLabel(bool value) => value ? '是' : '否';",
		"String booleanFieldValueLabel(String label, bool value) => '$label: ${booleanValueLabel(value)}';",
		"String get isPackedFieldLabel => '是否已打包';",
		"String get detailIsPackedLabel => isPackedFieldLabel;",
	} {
		if !strings.Contains(result.Content, marker) {
			t.Fatalf("content missing boolean generic marker: %s", marker)
		}
	}
}

func TestEmitCopyCustomAppTitle(t *testing.T) {
	// 验证 appTitle 是参数化的，不是固定值
	dm := appprepare.DomainModel{
		DomainCopy: appprepare.DomainCopy{Title: "自定义标题测试"},
		Entities: []appprepare.DataEntity{
			{EntityID: "entity-project", Name: "项目"},
			{EntityID: "entity-task", Name: "任务"},
			{EntityID: "entity-tag", Name: "标签"},
		},
	}
	result, ok := EmitCopy(dm)
	if !ok {
		t.Fatal("EmitCopy returned false")
	}
	if !strings.Contains(result.Content, "String get appTitle => '自定义标题测试';") {
		t.Fatal("appTitle not parameterized from DomainCopy.Title")
	}
}

func TestEmitCopyEmptyEntitiesReturnsFalse(t *testing.T) {
	dm := appprepare.DomainModel{
		DomainCopy: appprepare.DomainCopy{Title: "空实体"},
	}
	_, ok := EmitCopy(dm)
	if ok {
		t.Fatal("EmitCopy should return false for empty entities")
	}
}

func TestEmitCopyEmptyTitleReturnsFalse(t *testing.T) {
	dm := appprepare.DomainModel{
		DomainCopy: appprepare.DomainCopy{Title: ""},
		Entities: []appprepare.DataEntity{
			{EntityID: "entity-record", Name: "记录"},
		},
	}
	_, ok := EmitCopy(dm)
	if ok {
		t.Fatal("EmitCopy should return false for empty title")
	}
}

func TestEmitCopyMatchesCanonicalProjectTaskTag(t *testing.T) {
	// 验证 CopyEmitter 输出与现有 canonical copy 完全一致
	dm := appprepare.DomainModel{
		DomainCopy: appprepare.DomainCopy{Title: "项目任务协同 App"},
		Entities: []appprepare.DataEntity{
			{EntityID: "entity-project"},
			{EntityID: "entity-task"},
			{EntityID: "entity-tag"},
		},
	}
	result, ok := EmitCopy(dm)
	if !ok {
		t.Fatal("EmitCopy returned false")
	}
	// 验证结构完整性：以 import 开头，以 } + 换行结尾
	if !strings.HasPrefix(result.Content, "import '../models/task.dart';") {
		t.Fatal("content should start with import statement")
	}
	if !strings.HasSuffix(result.Content, "}\n") {
		t.Fatal("content should end with closing brace and newline")
	}
	// 验证类声明
	if !strings.Contains(result.Content, "class OpenLiteCopy {") {
		t.Fatal("content missing class declaration")
	}
	if !strings.Contains(result.Content, "const openLiteCopy = OpenLiteCopy();") {
		t.Fatal("content missing singleton constant")
	}
}

func TestEmitCopyMatchesCanonicalInventory(t *testing.T) {
	dm := appprepare.DomainModel{
		DomainCopy: appprepare.DomainCopy{Title: "库存盘点工作台"},
		Entities: []appprepare.DataEntity{
			{EntityID: "entity-inventory-sheet"},
			{EntityID: "entity-line-item"},
			{EntityID: "entity-sku"},
			{EntityID: "entity-warehouse"},
		},
	}
	result, ok := EmitCopy(dm)
	if !ok {
		t.Fatal("EmitCopy returned false")
	}
	if !strings.HasPrefix(result.Content, "import '../models/inventory_sheet.dart';") {
		t.Fatal("content should start with inventory_sheet import")
	}
	// 验证不包含 delete 相关 getter（inventory copy 不含删除功能）
	if strings.Contains(result.Content, "deleteActionLabel") {
		t.Fatal("inventory copy should not contain deleteActionLabel")
	}
}

func TestIdentifyCopyProfilePriority(t *testing.T) {
	// 验证 profile 判定不会因为额外实体干扰
	dm := appprepare.DomainModel{
		Entities: []appprepare.DataEntity{
			{EntityID: "entity-project"},
			{EntityID: "entity-task"},
			{EntityID: "entity-tag"},
			{EntityID: "entity-custom-extra"},
		},
	}
	profile := identifyCopyProfile(dm)
	if profile != copyProfileProjectTaskTag {
		t.Fatalf("profile = %q, want project-task-tag", profile)
	}
}
