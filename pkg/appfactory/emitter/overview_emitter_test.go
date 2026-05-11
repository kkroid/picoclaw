package emitter

import (
	"strings"
	"testing"

	appprepare "github.com/sipeed/oneappfactory/pkg/appfactory/prepare"
)

func TestEmitOverviewProjectTaskTag(t *testing.T) {
	dm := appprepare.DomainModel{
		DomainCopy: appprepare.DomainCopy{Title: "项目管理"},
		Entities: []appprepare.DataEntity{
			{EntityID: "entity-project", Name: "Project"},
			{EntityID: "entity-task", Name: "Task"},
			{EntityID: "entity-tag", Name: "Tag"},
		},
	}
	result, ok := EmitOverview(dm)
	if !ok {
		t.Fatal("EmitOverview() returned false for project-task-tag profile")
	}
	if result.HomePagePath != "lib/views/home_page.dart" {
		t.Fatalf("HomePagePath = %q, want lib/views/home_page.dart", result.HomePagePath)
	}
	if result.HomeControllerPath != "lib/controllers/home_controller.dart" {
		t.Fatalf("HomeControllerPath = %q, want lib/controllers/home_controller.dart", result.HomeControllerPath)
	}
	// HomePage 关键结构验证
	for _, marker := range []string{
		"class HomePage extends StatelessWidget",
		"required this.onCreateTask",
		"required this.onViewAllTasks",
		"controller.projects",
		"getSummaryForProject(project.projectId)",
		"_ProjectSummaryCard",
		"openLiteCopy.todoFilterLabel",
		"openLiteCopy.doneFilterLabel",
		"openLiteCopy.tagFilterLabel",
		"openLiteCopy.homeSummaryTitle",
		"_OverviewActions",
		"_OverviewEmptyState",
	} {
		if !strings.Contains(result.HomePageContent, marker) {
			t.Errorf("HomePage missing marker: %q", marker)
		}
	}
	// 不应有 inventory 特有元素
	for _, absent := range []string{"_OverviewHero", "warehouse", "onCreateRecord", "recentRecordsTitle"} {
		if strings.Contains(result.HomePageContent, absent) {
			t.Errorf("HomePage should not contain inventory marker: %q", absent)
		}
	}
	// HomeController 关键结构验证
	for _, marker := range []string{
		"class HomeController extends ChangeNotifier",
		"List<Project> get projects",
		"_repository.loadProjects()",
		"_repository.loadSummaries()",
		"getSummaryForProject(String projectId)",
		"summary.projectId == projectId",
	} {
		if !strings.Contains(result.HomeControllerContent, marker) {
			t.Errorf("HomeController missing marker: %q", marker)
		}
	}
	for _, absent := range []string{"warehouse", "totalOpenSheetCount", "totalLowStockSkuCount"} {
		if strings.Contains(result.HomeControllerContent, absent) {
			t.Errorf("HomeController should not contain inventory marker: %q", absent)
		}
	}
}

func TestEmitOverviewInventory(t *testing.T) {
	dm := appprepare.DomainModel{
		DomainCopy: appprepare.DomainCopy{Title: "库存管家"},
		Entities: []appprepare.DataEntity{
			{EntityID: "entity-inventory-sheet", Name: "InventorySheet"},
			{EntityID: "entity-line-item", Name: "LineItem"},
			{EntityID: "entity-sku", Name: "SKU"},
		},
	}
	result, ok := EmitOverview(dm)
	if !ok {
		t.Fatal("EmitOverview() returned false for inventory profile")
	}
	// HomePage 关键结构验证
	for _, marker := range []string{
		"class HomePage extends StatelessWidget",
		"required this.onCreateRecord",
		"required this.onViewAllRecords",
		"controller.warehouses",
		"getSummaryForWarehouse(warehouse.warehouseId)",
		"_WarehouseSummaryCard",
		"_OverviewHero",
		"_HeroStat",
		"controller.totalOpenSheetCount",
		"openLiteCopy.recentRecordsTitle",
		"openLiteCopy.warehouseLocationLabel",
		"Color(0xFF1565C0)",
	} {
		if !strings.Contains(result.HomePageContent, marker) {
			t.Errorf("HomePage missing marker: %q", marker)
		}
	}
	// 不应有 project-task-tag 特有元素
	for _, absent := range []string{"onCreateTask", "onViewAllTasks", "_ProjectSummaryCard", "project.projectId"} {
		if strings.Contains(result.HomePageContent, absent) {
			t.Errorf("HomePage should not contain project-task-tag marker: %q", absent)
		}
	}
	// HomeController 关键结构验证
	for _, marker := range []string{
		"class HomeController extends ChangeNotifier",
		"List<Warehouse> get warehouses",
		"_repository.loadWarehouses()",
		"_repository.loadSummaries()",
		"getSummaryForWarehouse(String warehouseId)",
		"totalOpenSheetCount",
		"totalLowStockSkuCount",
		"totalVarianceLineItemCount",
		"summary.warehouseId == warehouseId",
	} {
		if !strings.Contains(result.HomeControllerContent, marker) {
			t.Errorf("HomeController missing marker: %q", marker)
		}
	}
}

func TestEmitOverviewGenericReturnsFalse(t *testing.T) {
	dm := appprepare.DomainModel{
		DomainCopy: appprepare.DomainCopy{Title: "体重追踪"},
		Entities:   []appprepare.DataEntity{{EntityID: "entity-weight-record", Name: "WeightRecord"}},
	}
	_, ok := EmitOverview(dm)
	if ok {
		t.Fatal("EmitOverview() returned true for generic profile, want false")
	}
}

func TestEmitOverviewNoEntitiesReturnsFalse(t *testing.T) {
	dm := appprepare.DomainModel{
		DomainCopy: appprepare.DomainCopy{Title: "空应用"},
	}
	_, ok := EmitOverview(dm)
	if ok {
		t.Fatal("EmitOverview() returned true for empty entities, want false")
	}
}

func TestEmitOverviewProjectTaskTagMatchesCanonical(t *testing.T) {
	dm := appprepare.DomainModel{
		DomainCopy: appprepare.DomainCopy{Title: "任务管理"},
		Entities: []appprepare.DataEntity{
			{EntityID: "entity-project", Name: "Project"},
			{EntityID: "entity-task", Name: "Task"},
			{EntityID: "entity-tag", Name: "Tag"},
		},
	}
	result, ok := EmitOverview(dm)
	if !ok {
		t.Fatal("EmitOverview() returned false")
	}
	// 验证输出以 Dart import 开头，以换行结尾
	if !strings.HasPrefix(result.HomePageContent, "import 'package:flutter/material.dart';") {
		t.Error("HomePage should start with flutter/material.dart import")
	}
	if !strings.HasSuffix(result.HomePageContent, "\n") {
		t.Error("HomePage should end with newline")
	}
	if !strings.HasPrefix(result.HomeControllerContent, "import 'package:flutter/foundation.dart'") {
		t.Error("HomeController should start with flutter/foundation.dart import")
	}
	if !strings.HasSuffix(result.HomeControllerContent, "\n") {
		t.Error("HomeController should end with newline")
	}
}

func TestEmitOverviewInventoryMatchesCanonical(t *testing.T) {
	dm := appprepare.DomainModel{
		DomainCopy: appprepare.DomainCopy{Title: "仓库管理"},
		Entities: []appprepare.DataEntity{
			{EntityID: "entity-inventory-sheet", Name: "InventorySheet"},
			{EntityID: "entity-line-item", Name: "LineItem"},
			{EntityID: "entity-sku", Name: "SKU"},
		},
	}
	result, ok := EmitOverview(dm)
	if !ok {
		t.Fatal("EmitOverview() returned false")
	}
	if !strings.HasPrefix(result.HomePageContent, "import 'package:flutter/material.dart';") {
		t.Error("HomePage should start with flutter/material.dart import")
	}
	if !strings.HasPrefix(result.HomeControllerContent, "import 'package:flutter/foundation.dart'") {
		t.Error("HomeController should start with flutter/foundation.dart import")
	}
	// inventory 特有：Hero 面板用深蓝色
	if !strings.Contains(result.HomePageContent, "0xFF1565C0") {
		t.Error("inventory HomePage should contain blue hero color")
	}
	// inventory 特有：3 个汇总 getter
	controllerGetters := []string{"totalOpenSheetCount", "totalLowStockSkuCount", "totalVarianceLineItemCount"}
	for _, getter := range controllerGetters {
		if !strings.Contains(result.HomeControllerContent, getter) {
			t.Errorf("inventory HomeController missing getter: %q", getter)
		}
	}
}
