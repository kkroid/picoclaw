package prepare

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestCompileInventorySheetLineItemRequirement(t *testing.T) {
	bundle, err := Compile(Request{
		RequirementText:   "做一个库存单协同 app，需要库存看板、库存单列表、库存单编辑和库存单详情，支持在库存单里维护多个明细项，并按仓库和低库存状态筛选。",
		RequirementSource: "inline:relation-rich-inventory-test",
		Now: func() time.Time {
			return time.Date(2026, 4, 10, 17, 0, 0, 0, time.UTC)
		},
	})
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	if bundle.PRD.ID != "prd-inventory-sheet-line-item-open-lite" {
		t.Fatalf("PRD.ID = %q, want prd-inventory-sheet-line-item-open-lite", bundle.PRD.ID)
	}
	if bundle.PRD.Title != "库存单协同 App" {
		t.Fatalf("PRD.Title = %q, want 库存单协同 App", bundle.PRD.Title)
	}
	if bundle.BuilderInput.TemplateID != "flutter-open-lite" {
		t.Fatalf("BuilderInput.TemplateID = %q, want flutter-open-lite", bundle.BuilderInput.TemplateID)
	}
	if len(bundle.PRD.DataEntities) != 5 {
		t.Fatalf("PRD.DataEntities len = %d, want 5", len(bundle.PRD.DataEntities))
	}
	if bundle.PRD.DataEntities[1].EntityID != "entity-line-item" {
		t.Fatalf("line item entity id = %q, want entity-line-item", bundle.PRD.DataEntities[1].EntityID)
	}
	if bundle.PRD.ExecutionContract == nil {
		t.Fatal("PRD.ExecutionContract = nil, want execution contract")
	}
	assertSurfaceRelationSchema(t, bundle.PRD.ExecutionContract)
	if len(bundle.PRD.ExecutionContract.SurfaceContracts) != 4 {
		t.Fatalf("ExecutionContract.SurfaceContracts len = %d, want 4", len(bundle.PRD.ExecutionContract.SurfaceContracts))
	}
	listControllerTask := findTaskBundleItem(t, bundle.BuilderInput.TaskBundle, "task-create-list-controller")
	if listControllerTask.AllocationTransition == nil {
		t.Fatal("list controller allocation_transition = nil, want populated relation-rich transition")
	}
	androidBrandingTask := findTaskBundleItem(t, bundle.BuilderInput.TaskBundle, "task-create-android-branding")
	if androidBrandingTask.RouteHint != "deterministic" {
		t.Fatalf("android branding route_hint = %q, want deterministic", androidBrandingTask.RouteHint)
	}
	androidBuildConfigTask := findTaskBundleItem(t, bundle.BuilderInput.TaskBundle, "task-create-android-build-config")
	if androidBuildConfigTask.RouteHint != "deterministic" {
		t.Fatalf("android build config route_hint = %q, want deterministic", androidBuildConfigTask.RouteHint)
	}
	for _, taskID := range []string{
		"task-create-relation-models",
		"task-create-summary-model",
		"task-create-repository",
		"task-create-home-controller",
		"task-create-form-controller",
		"task-create-list-controller",
		"task-create-copy",
		taskBindOverviewSurfaceID,
		taskBindCollectionSurfaceID,
		taskBindMutationSurfaceID,
		taskBindInspectionSurfaceID,
		taskBindAppEntryID,
		"task-create-test",
		"task-create-android-branding",
		"task-create-android-build-config",
	} {
		task := findTaskBundleItem(t, bundle.BuilderInput.TaskBundle, taskID)
		if task.RouteHint != "deterministic" {
			t.Fatalf("%s route_hint = %q, want deterministic", taskID, task.RouteHint)
		}
	}
	if !reflect.DeepEqual(listControllerTask.AllocationTransition.SurfaceRefs, []string{genericSurfaceCollectionID, genericSurfaceInspectionID}) {
		t.Fatalf("list controller surface_refs = %v, want [%s %s]", listControllerTask.AllocationTransition.SurfaceRefs, genericSurfaceCollectionID, genericSurfaceInspectionID)
	}
	if len(listControllerTask.AllocationTransition.ScreenRefs) != 0 {
		t.Fatalf("list controller screen_refs = %v, want omitted compatibility refs", listControllerTask.AllocationTransition.ScreenRefs)
	}
	if !containsString(listControllerTask.AllocationTransition.EntityRefs, "entity-line-item") || !containsString(listControllerTask.AllocationTransition.EntityRefs, "entity-sku") {
		t.Fatalf("list controller entity_refs = %v, want include line-item/sku", listControllerTask.AllocationTransition.EntityRefs)
	}
	if !containsString(listControllerTask.AllocationTransition.RelationGroupRefs, "relation-sheet-line-item") || !containsString(listControllerTask.AllocationTransition.RelationGroupRefs, "relation-line-item-sku") {
		t.Fatalf("list controller relation_group_refs = %v, want include both relation refs", listControllerTask.AllocationTransition.RelationGroupRefs)
	}
	if !containsString(listControllerTask.AllocationTransition.SharedOwnershipRefs, "shared-filter-state") {
		t.Fatalf("list controller shared_ownership_refs = %v, want include shared-filter-state", listControllerTask.AllocationTransition.SharedOwnershipRefs)
	}

	var taskAllocation TaskAllocation
	mustDecodeJSON(t, bundle.Files[taskAllocationFileName], &taskAllocation)
	listControllerUnit := findAllocationUnit(t, taskAllocation.Units, "task-create-list-controller")
	if !reflect.DeepEqual(listControllerUnit.SurfaceRefs, []string{genericSurfaceCollectionID, genericSurfaceInspectionID}) {
		t.Fatalf("task allocation surface_refs = %v, want [%s %s]", listControllerUnit.SurfaceRefs, genericSurfaceCollectionID, genericSurfaceInspectionID)
	}
	if len(listControllerUnit.ScreenRefs) != 0 {
		t.Fatalf("task allocation screen_refs = %v, want omitted compatibility refs", listControllerUnit.ScreenRefs)
	}
	if !containsString(listControllerUnit.RelationGroupRefs, "relation-sheet-line-item") || !containsString(listControllerUnit.RelationGroupRefs, "relation-line-item-sku") {
		t.Fatalf("task allocation relation_group_refs = %v, want include both relation refs", listControllerUnit.RelationGroupRefs)
	}
	if !containsString(listControllerUnit.SharedOwnershipRefs, "shared-filter-state") {
		t.Fatalf("task allocation shared_ownership_refs = %v, want include shared-filter-state", listControllerUnit.SharedOwnershipRefs)
	}

	var planningContext PlanningContext
	mustDecodeJSON(t, bundle.Files[planningContextFileName], &planningContext)
	if planningContext.TemplateID != "flutter-open-lite" {
		t.Fatalf("PlanningContext.TemplateID = %q, want flutter-open-lite", planningContext.TemplateID)
	}

	projection := mustFindTaskProjectionItem(t, bundle.PRD.ExecutionContract.TaskProjection, "task-create-list-controller")
	if !containsString(projection.SurfaceRefs, genericSurfaceCollectionID) || !containsString(projection.SurfaceRefs, genericSurfaceInspectionID) {
		t.Fatalf("task projection surface_refs = %v, want include %s and %s", projection.SurfaceRefs, genericSurfaceCollectionID, genericSurfaceInspectionID)
	}
	if !containsString(projection.EntityRefs, "entity-line-item") || !containsString(projection.EntityRefs, "entity-sku") {
		t.Fatalf("task projection entity_refs = %v, want include line-item/sku", projection.EntityRefs)
	}
	if !containsString(projection.RelationGroupRefs, "relation-sheet-line-item") || !containsString(projection.RelationGroupRefs, "relation-line-item-sku") {
		t.Fatalf("task projection relation_group_refs = %v, want include both relation refs", projection.RelationGroupRefs)
	}
	if !containsString(projection.SharedOwnershipRefs, "shared-filter-state") {
		t.Fatalf("task projection shared_ownership_refs = %v, want include shared-filter-state", projection.SharedOwnershipRefs)
	}

	var builderInput map[string]any
	if err := json.Unmarshal(bundle.Files[builderInputFileName], &builderInput); err != nil {
		t.Fatalf("Unmarshal(builder-input.json) error = %v", err)
	}
	taskBundle, ok := builderInput["task_bundle"].([]any)
	if !ok || len(taskBundle) == 0 {
		t.Fatalf("task_bundle = %v, want non-empty list", builderInput["task_bundle"])
	}
	foundRelationTransition := false
	for _, raw := range taskBundle {
		item, ok := raw.(map[string]any)
		if !ok || item["task_id"] != "task-create-list-controller" {
			continue
		}
		transition, ok := item["allocation_transition"].(map[string]any)
		if !ok {
			t.Fatalf("allocation_transition = %T, want map[string]any", item["allocation_transition"])
		}
		if _, ok := transition["screen_refs"]; ok {
			t.Fatalf("allocation_transition.screen_refs = %v, want omitted compatibility refs", transition["screen_refs"])
		}
		relationRefs, ok := transition["relation_group_refs"].([]any)
		if !ok || len(relationRefs) == 0 {
			t.Fatalf("relation_group_refs = %v, want non-empty list", transition["relation_group_refs"])
		}
		sharedRefs, ok := transition["shared_ownership_refs"].([]any)
		if !ok || len(sharedRefs) == 0 {
			t.Fatalf("shared_ownership_refs = %v, want non-empty list", transition["shared_ownership_refs"])
		}
		foundRelationTransition = true
	}
	if !foundRelationTransition {
		t.Fatal("missing inventory relation-rich allocation_transition in builder-input task_bundle")
	}
	if !strings.Contains(string(bundle.Files[fitReportFileName]), "复杂") {
		t.Fatalf("template fit report should mention complex-domain risk: %s", string(bundle.Files[fitReportFileName]))
	}
}

func TestCompileInventorySheetLineItemRequirementUsesRealBuildChecksWhenExecutorImageProvided(t *testing.T) {
	bundle, err := Compile(Request{
		RequirementText: "做一个库存单协同 app，需要库存看板、库存单列表、库存单编辑和库存单详情，支持在库存单里维护多个明细项，并按仓库和低库存状态筛选。",
		ExecutorImage:   "oneappfactory/builder:local",
	})
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	if bundle.BuilderInput.ExecutorImage != "oneappfactory/builder:local" {
		t.Fatalf("ExecutorImage = %q, want oneappfactory/builder:local", bundle.BuilderInput.ExecutorImage)
	}
	if len(bundle.BuilderInput.AcceptanceChecks) != 7 {
		t.Fatalf("AcceptanceChecks len = %d, want 7", len(bundle.BuilderInput.AcceptanceChecks))
	}
	for index, checkID := range []string{"check-flutter-pub-get", "check-counter-demo-removed", "check-entry-form-wiring", "check-local-persistence-wiring", "check-flutter-analyze", "check-flutter-test", "check-flutter-build-apk"} {
		if bundle.BuilderInput.AcceptanceChecks[index].CheckID != checkID {
			t.Fatalf("acceptance_check[%d] = %q, want %q", index, bundle.BuilderInput.AcceptanceChecks[index].CheckID, checkID)
		}
	}
	if len(bundle.BuilderInput.AllowedPaths) != 6 || bundle.BuilderInput.AllowedPaths[4] != "android/app/build.gradle.kts" || bundle.BuilderInput.AllowedPaths[5] != "android/app/src/main/res/values/strings.xml" {
		t.Fatalf("AllowedPaths = %v, want Android override points in inventory relation-rich real-build path", bundle.BuilderInput.AllowedPaths)
	}
	if len(bundle.BuilderInput.ProtectedPaths) != 8 || bundle.BuilderInput.ProtectedPaths[0] != "android/app/src/main/AndroidManifest.xml" {
		t.Fatalf("ProtectedPaths = %v, want narrowed protected paths in inventory relation-rich real-build path", bundle.BuilderInput.ProtectedPaths)
	}
	for _, check := range bundle.BuilderInput.AcceptanceChecks {
		if check.CheckID == "check-oneappfactory-thin-fallback-probe" || check.CheckID == "check-oneappfactory-thin-fallback-metadata" {
			t.Fatalf("unexpected OneAppFactory fallback check in inventory relation-rich real-build path: %q", check.CheckID)
		}
	}
}
