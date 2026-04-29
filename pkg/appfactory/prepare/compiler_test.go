package prepare

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	appruns "github.com/sipeed/picoclaw/pkg/appfactory/runs"
)

func TestCompileBookkeepingRequirement(t *testing.T) {
	bundle, err := Compile(Request{
		RequirementText:   "做一个简单记账 app，不考虑上架，只考虑功能实现，需要首页概览、记一笔和账单列表。",
		RequirementSource: "inline:test",
		Now: func() time.Time {
			return time.Date(2026, 3, 26, 8, 0, 0, 0, time.UTC)
		},
	})
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	if bundle.PRD.ID != "prd-bookkeeping-lite" {
		t.Fatalf("PRD.ID = %q, want prd-bookkeeping-lite", bundle.PRD.ID)
	}
	if bundle.PRD.Title != "轻量记账 App" {
		t.Fatalf("PRD.Title = %q, want 轻量记账 App", bundle.PRD.Title)
	}
	if bundle.BuilderInput.TemplateID != "flutter-finance-lite" {
		t.Fatalf("BuilderInput.TemplateID = %q, want flutter-finance-lite", bundle.BuilderInput.TemplateID)
	}
	if bundle.BuilderInput.PlanningPolicy.PolicyVersion != "phase1-boundary-v1" {
		t.Fatalf("PlanningPolicy.PolicyVersion = %q, want phase1-boundary-v1", bundle.BuilderInput.PlanningPolicy.PolicyVersion)
	}
	if len(bundle.BuilderInput.PlanningPolicy.Stages) != 5 {
		t.Fatalf("PlanningPolicy.Stages len = %d, want 5", len(bundle.BuilderInput.PlanningPolicy.Stages))
	}
	if bundle.BuilderInput.PlanningPolicy.Stages[4].Route != appruns.PlanningStageRouteDeterministic {
		t.Fatalf("final planning stage route = %q, want deterministic", bundle.BuilderInput.PlanningPolicy.Stages[4].Route)
	}
	if !strings.HasPrefix(bundle.BuilderInput.PreparedPRDSubjectVersion, "prd-bookkeeping-lite@0.1.0@sha256:") {
		t.Fatalf("PreparedPRDSubjectVersion = %q, want PRD compile-source version", bundle.BuilderInput.PreparedPRDSubjectVersion)
	}
	if want := TemplateCompileSourceVersion("flutter-finance-lite", "v0.1.0"); bundle.BuilderInput.PreparedTemplateSubjectVersion != want {
		t.Fatalf("PreparedTemplateSubjectVersion = %q, want %q", bundle.BuilderInput.PreparedTemplateSubjectVersion, want)
	}
	if !strings.Contains(string(bundle.Files[fitReportFileName]), "命中模板注册表条目：flutter-finance-lite") {
		t.Fatalf("template fit report should include registry selection reason: %s", string(bundle.Files[fitReportFileName]))
	}
	if len(bundle.BuilderInput.TaskBundle) != 5 {
		t.Fatalf("TaskBundle len = %d, want 5", len(bundle.BuilderInput.TaskBundle))
	}
	if len(bundle.BuilderInput.KnowledgePack) != 4 {
		t.Fatalf("KnowledgePack len = %d, want 4", len(bundle.BuilderInput.KnowledgePack))
	}
	if bundle.BuilderInput.KnowledgePack[0].SkillID != "prd-to-task-bundle" {
		t.Fatalf("first skill = %q, want prd-to-task-bundle", bundle.BuilderInput.KnowledgePack[0].SkillID)
	}
	if bundle.BuilderInput.TaskBundle[0].TaskID != "task-domain-models" {
		t.Fatalf("first task id = %q, want task-domain-models", bundle.BuilderInput.TaskBundle[0].TaskID)
	}
	if bundle.BuilderInput.TaskBundle[0].AllocationTransition == nil {
		t.Fatal("first task allocation_transition = nil, want derived transition mapping")
	}
	if bundle.BuilderInput.TaskBundle[0].AllocationTransition.AllocationID != "task-domain-models" {
		t.Fatalf("first task allocation_id = %q, want task-domain-models", bundle.BuilderInput.TaskBundle[0].AllocationTransition.AllocationID)
	}
	if len(bundle.BuilderInput.TaskBundle[0].AllocationTransition.OwnedPaths) != 2 {
		t.Fatalf("first task owned_paths = %v, want two owned paths", bundle.BuilderInput.TaskBundle[0].AllocationTransition.OwnedPaths)
	}
	if len(bundle.PRD.SurfaceList) != 3 {
		t.Fatalf("SurfaceList len = %d, want 3", len(bundle.PRD.SurfaceList))
	}
	for index, surfaceID := range []string{genericSurfaceOverviewID, genericSurfaceMutationID, genericSurfaceCollectionID} {
		if bundle.PRD.SurfaceList[index].SurfaceID != surfaceID {
			t.Fatalf("SurfaceList[%d].SurfaceID = %q, want %q", index, bundle.PRD.SurfaceList[index].SurfaceID, surfaceID)
		}
	}
	if len(bundle.PRD.ScreenList) != 0 {
		t.Fatalf("ScreenList = %v, want omitted compatibility output", bundle.PRD.ScreenList)
	}
	for _, surface := range bundle.PRD.SurfaceList {
		if surface.LegacyScreenRef != "" {
			t.Fatalf("surface %q legacy_screen_ref = %q, want omitted compatibility output", surface.SurfaceID, surface.LegacyScreenRef)
		}
	}
	for _, feature := range bundle.PRD.FeatureList {
		if len(feature.RelatedSurfaceRefs) == 0 {
			t.Fatalf("feature %q related_surface_refs = %v, want surface-first refs", feature.FeatureID, feature.RelatedSurfaceRefs)
		}
		if len(feature.RelatedScreens) != 0 {
			t.Fatalf("feature %q related_screens = %v, want compatibility screens omitted", feature.FeatureID, feature.RelatedScreens)
		}
	}
	for _, flow := range bundle.PRD.UserFlows {
		for _, step := range flow.Steps {
			if step.SurfaceRef == "" {
				t.Fatalf("user flow %q step %q surface_ref is empty, want surface-first projection", flow.FlowID, step.StepID)
			}
			if step.ScreenRef != "" {
				t.Fatalf("user flow %q step %q screen_ref = %q, want compatibility screen_ref omitted", flow.FlowID, step.StepID, step.ScreenRef)
			}
		}
	}
	surfaceTask := findTaskBundleItem(t, bundle.BuilderInput.TaskBundle, "task-bind-core-surfaces")
	if surfaceTask.AllocationTransition == nil {
		t.Fatal("core surfaces allocation_transition = nil, want surface refs")
	}
	for _, surfaceID := range []string{genericSurfaceOverviewID, genericSurfaceCollectionID, genericSurfaceMutationID} {
		if !containsString(surfaceTask.AllocationTransition.SurfaceRefs, surfaceID) {
			t.Fatalf("core surfaces surface_refs = %v, want include %q", surfaceTask.AllocationTransition.SurfaceRefs, surfaceID)
		}
	}
	if len(surfaceTask.AllocationTransition.ScreenRefs) != 0 {
		t.Fatalf("core surfaces screen_refs = %v, want omitted compatibility refs", surfaceTask.AllocationTransition.ScreenRefs)
	}
	for _, entityID := range []string{"entity-entry", "entity-summary"} {
		if !containsString(surfaceTask.AllocationTransition.EntityRefs, entityID) {
			t.Fatalf("core surfaces entity_refs = %v, want include %q", surfaceTask.AllocationTransition.EntityRefs, entityID)
		}
	}
	if bundle.BuilderInput.TaskBundle[0].Category != "domain" {
		t.Fatalf("first task category = %q, want domain", bundle.BuilderInput.TaskBundle[0].Category)
	}
	if bundle.BuilderInput.TaskBundle[1].Category != "storage" {
		t.Fatalf("second task category = %q, want storage", bundle.BuilderInput.TaskBundle[1].Category)
	}
	if bundle.BuilderInput.TaskBundle[2].Category != "screen" {
		t.Fatalf("third task category = %q, want screen", bundle.BuilderInput.TaskBundle[2].Category)
	}
	if bundle.BuilderInput.TaskBundle[3].Category != "flow" {
		t.Fatalf("fourth task category = %q, want flow", bundle.BuilderInput.TaskBundle[3].Category)
	}
	if bundle.BuilderInput.TaskBundle[4].Category != "validation" {
		t.Fatalf("fifth task category = %q, want validation", bundle.BuilderInput.TaskBundle[4].Category)
	}
	if len(bundle.BuilderInput.TaskBundle[4].Dependencies) != 1 || bundle.BuilderInput.TaskBundle[4].Dependencies[0] != "task-flow-wiring" {
		t.Fatalf("validation dependencies = %v, want [task-flow-wiring]", bundle.BuilderInput.TaskBundle[4].Dependencies)
	}
	for _, name := range []string{requirementFileName, prdMarkdownFileName, prdJSONFileName, prdApprovalFileName, templateApprovalFileName, fitReportFileName, planFileName, constraintsFileName, planningContextFileName, domainModelFileName, templateSlotMapFileName, taskAllocationFileName, acceptancePlanFileName, builderInputFileName} {
		if _, ok := bundle.Files[name]; !ok {
			t.Fatalf("expected generated file %s", name)
		}
	}
	var prd map[string]any
	if err := json.Unmarshal(bundle.Files[prdJSONFileName], &prd); err != nil {
		t.Fatalf("Unmarshal(PRD.json) error = %v", err)
	}
	if prd["status"] != "draft" {
		t.Fatalf("status = %v, want draft", prd["status"])
	}
	if _, ok := prd["screen_list"]; ok {
		t.Fatalf("PRD.json screen_list = %v, want omitted compatibility output", prd["screen_list"])
	}
	if bytes.Contains(bundle.Files[prdJSONFileName], []byte(`"legacy_screen_ref"`)) {
		t.Fatalf("PRD.json = %s, want legacy_screen_ref omitted", string(bundle.Files[prdJSONFileName]))
	}
	var input map[string]any
	if err := json.Unmarshal(bundle.Files[builderInputFileName], &input); err != nil {
		t.Fatalf("Unmarshal(builder-input.json) error = %v", err)
	}
	if input["job_id"] != "job-bookkeeping-lite" {
		t.Fatalf("job_id = %v, want job-bookkeeping-lite", input["job_id"])
	}
	if subjectVersion, _ := input["prepared_prd_subject_version"].(string); !strings.HasPrefix(subjectVersion, "prd-bookkeeping-lite@0.1.0@sha256:") {
		t.Fatalf("prepared_prd_subject_version = %v, want PRD compile-source version", input["prepared_prd_subject_version"])
	}
	if want := TemplateCompileSourceVersion("flutter-finance-lite", "v0.1.0"); input["prepared_template_subject_version"] != want {
		t.Fatalf("prepared_template_subject_version = %v, want %q", input["prepared_template_subject_version"], want)
	}
	planningPolicy, ok := input["planning_policy"].(map[string]any)
	if !ok {
		t.Fatalf("planning_policy type = %T, want map[string]any", input["planning_policy"])
	}
	if planningPolicy["policy_version"] != "phase1-boundary-v1" {
		t.Fatalf("planning_policy.policy_version = %v, want phase1-boundary-v1", planningPolicy["policy_version"])
	}
	stages, ok := planningPolicy["stages"].([]any)
	if !ok || len(stages) != 5 {
		t.Fatalf("planning_policy.stages = %v, want 5 stages", planningPolicy["stages"])
	}
	taskBundle, ok := input["task_bundle"].([]any)
	if !ok || len(taskBundle) == 0 {
		t.Fatalf("task_bundle = %v, want non-empty list", input["task_bundle"])
	}
	firstTask, ok := taskBundle[0].(map[string]any)
	if !ok {
		t.Fatalf("task_bundle[0] = %T, want map[string]any", taskBundle[0])
	}
	allocationTransition, ok := firstTask["allocation_transition"].(map[string]any)
	if !ok {
		t.Fatalf("allocation_transition = %T, want map[string]any", firstTask["allocation_transition"])
	}
	if allocationTransition["allocation_id"] != "task-domain-models" {
		t.Fatalf("allocation_transition.allocation_id = %v, want task-domain-models", allocationTransition["allocation_id"])
	}
	var foundCopyBindingTransition bool
	for _, rawTask := range taskBundle {
		item, ok := rawTask.(map[string]any)
		if !ok || item["task_id"] != "task-bind-core-surfaces" {
			continue
		}
		transition, ok := item["allocation_transition"].(map[string]any)
		if !ok {
			t.Fatalf("task-bind-core-surfaces allocation_transition = %T, want map[string]any", item["allocation_transition"])
		}
		bindingRefs, ok := transition["binding_refs"].([]any)
		if !ok {
			t.Fatalf("task-bind-core-surfaces binding_refs = %T, want []any", transition["binding_refs"])
		}
		bindingRefStrings := make([]string, 0, len(bindingRefs))
		for _, rawRef := range bindingRefs {
			ref, ok := rawRef.(string)
			if !ok {
				t.Fatalf("task-bind-core-surfaces binding_refs item = %T, want string", rawRef)
			}
			bindingRefStrings = append(bindingRefStrings, ref)
		}
		if !strings.Contains(strings.Join(bindingRefStrings, ","), publicBindingDomainCopy) {
			t.Fatalf("task-bind-core-surfaces binding_refs = %v, want include %s", bindingRefs, publicBindingDomainCopy)
		}
		foundCopyBindingTransition = true
	}
	if !foundCopyBindingTransition {
		t.Fatal("missing task-bind-core-surfaces allocation_transition in builder-input task_bundle")
	}
	contextFiles, ok := input["context_files"].(map[string]any)
	if !ok {
		t.Fatalf("context_files type = %T, want map[string]any", input["context_files"])
	}
	if contextFiles["prd_markdown_path"] != prdMarkdownFileName {
		t.Fatalf("prd_markdown_path = %v, want %s", contextFiles["prd_markdown_path"], prdMarkdownFileName)
	}
	supportingFiles, ok := contextFiles["supporting_files"].([]any)
	if !ok || len(supportingFiles) != 8 {
		t.Fatalf("supporting_files = %v, want [requirement.md prd-approval.json template-approval.json planning-context.json domain-model.json template-slot-map.json task-allocation.json acceptance-plan.json]", contextFiles["supporting_files"])
	}
	if supportingFiles[1] != prdApprovalFileName || supportingFiles[2] != templateApprovalFileName || supportingFiles[3] != planningContextFileName || supportingFiles[4] != domainModelFileName || supportingFiles[5] != templateSlotMapFileName || supportingFiles[6] != taskAllocationFileName || supportingFiles[7] != acceptancePlanFileName {
		t.Fatalf("supporting_files = %v, want approval snapshots appended", supportingFiles)
	}
	var planningContext PlanningContext
	if err := json.Unmarshal(bundle.Files[planningContextFileName], &planningContext); err != nil {
		t.Fatalf("Unmarshal(planning-context.json) error = %v", err)
	}
	if planningContext.JobID != bundle.BuilderInput.JobID {
		t.Fatalf("PlanningContext.JobID = %q, want %q", planningContext.JobID, bundle.BuilderInput.JobID)
	}
	if planningContext.PlanningPolicy.PolicyVersion != "phase1-boundary-v1" {
		t.Fatalf("PlanningContext.PolicyVersion = %q, want phase1-boundary-v1", planningContext.PlanningPolicy.PolicyVersion)
	}
	if planningContext.PlanningPolicyVersion != "phase1-boundary-v1" {
		t.Fatalf("PlanningContext.PlanningPolicyVersion = %q, want phase1-boundary-v1", planningContext.PlanningPolicyVersion)
	}
	if planningContext.PlanningModelSnapshot.RequirementStructuring != string(appruns.PlanningStageRoutePlanningModel) {
		t.Fatalf("PlanningContext.RequirementStructuring = %q, want planning_model", planningContext.PlanningModelSnapshot.RequirementStructuring)
	}
	if planningContext.ExecutionRouteSnapshot.DefaultRouteHint != appruns.TaskRouteHintDefaultModel {
		t.Fatalf("PlanningContext.DefaultRouteHint = %q, want default_model", planningContext.ExecutionRouteSnapshot.DefaultRouteHint)
	}
	var flowAllocation TaskAllocation
	mustDecodeJSON(t, bundle.Files[taskAllocationFileName], &flowAllocation)
	flowUnit := findAllocationUnit(t, flowAllocation.Units, "task-flow-wiring")
	for _, surfaceID := range []string{genericSurfaceOverviewID, genericSurfaceCollectionID, genericSurfaceMutationID} {
		if !containsString(flowUnit.SurfaceRefs, surfaceID) {
			t.Fatalf("task-flow-wiring surface_refs = %v, want include %q", flowUnit.SurfaceRefs, surfaceID)
		}
	}
	if len(flowUnit.ScreenRefs) != 0 {
		t.Fatalf("task-flow-wiring screen_refs = %v, want omitted compatibility refs", flowUnit.ScreenRefs)
	}
	for _, entityID := range []string{"entity-entry", "entity-summary"} {
		if !containsString(flowUnit.EntityRefs, entityID) {
			t.Fatalf("task-flow-wiring entity_refs = %v, want include %q", flowUnit.EntityRefs, entityID)
		}
	}
	projection := mustFindTaskProjectionItem(t, bundle.PRD.ExecutionContract.TaskProjection, "task-flow-wiring")
	for _, surfaceID := range []string{genericSurfaceOverviewID, genericSurfaceCollectionID, genericSurfaceMutationID} {
		if !containsString(projection.SurfaceRefs, surfaceID) {
			t.Fatalf("task projection surface_refs = %v, want include %q", projection.SurfaceRefs, surfaceID)
		}
	}
	for _, entityID := range []string{"entity-entry", "entity-summary"} {
		if !containsString(projection.EntityRefs, entityID) {
			t.Fatalf("task projection entity_refs = %v, want include %q", projection.EntityRefs, entityID)
		}
	}
	contractJSON, err := json.Marshal(bundle.PRD.ExecutionContract)
	if err != nil {
		t.Fatalf("Marshal(ExecutionContract) error = %v", err)
	}
	if strings.Contains(string(contractJSON), `"screen_refs"`) {
		t.Fatalf("execution contract JSON = %s, want screen_refs omitted", string(contractJSON))
	}
	if got := planningContext.ExecutionRouteSnapshot.RouteHintCounts[string(appruns.TaskRouteHintDefaultModel)]; got != 5 {
		t.Fatalf("PlanningContext.default_model count = %d, want 5", got)
	}
	if got := planningContext.ExecutionRouteSnapshot.RouteHintCounts[string(appruns.TaskRouteHintStrongModel)]; got != 0 {
		t.Fatalf("PlanningContext.strong_model count = %d, want 0", got)
	}
	if len(planningContext.ExecutionRouteSnapshot.TaskRoutes) != 5 {
		t.Fatalf("PlanningContext.TaskRoutes len = %d, want 5", len(planningContext.ExecutionRouteSnapshot.TaskRoutes))
	}
	if planningContext.ExecutionRouteSnapshot.UpgradePolicy.Source != "config/config.example.json:appfactory.builder_runtime.upgrade_threshold" {
		t.Fatalf("PlanningContext.UpgradePolicy.Source = %q, want config example baseline", planningContext.ExecutionRouteSnapshot.UpgradePolicy.Source)
	}
	if planningContext.ExecutionRouteSnapshot.UpgradePolicy.MaxAttemptsBeforeUpgrade != 2 {
		t.Fatalf("PlanningContext.MaxAttemptsBeforeUpgrade = %d, want 2", planningContext.ExecutionRouteSnapshot.UpgradePolicy.MaxAttemptsBeforeUpgrade)
	}
	if !planningContext.ExecutionRouteSnapshot.StartUpgradeCheck.WouldUpgradeFromStart {
		t.Fatalf("PlanningContext.StartUpgradeCheck = %+v, want would_upgrade_from_start=true", planningContext.ExecutionRouteSnapshot.StartUpgradeCheck)
	}
	if len(planningContext.ExecutionRouteSnapshot.StartUpgradeCheck.TriggeredReasons) != 1 || planningContext.ExecutionRouteSnapshot.StartUpgradeCheck.TriggeredReasons[0] != "max_files_before_upgrade" {
		t.Fatalf("PlanningContext.TriggeredReasons = %v, want [max_files_before_upgrade]", planningContext.ExecutionRouteSnapshot.StartUpgradeCheck.TriggeredReasons)
	}
	var domainModel DomainModel
	if err := json.Unmarshal(bundle.Files[domainModelFileName], &domainModel); err != nil {
		t.Fatalf("Unmarshal(domain-model.json) error = %v", err)
	}
	if domainModel.DomainName != "轻量记账 App" {
		t.Fatalf("DomainModel.DomainName = %q, want 轻量记账 App", domainModel.DomainName)
	}
	if len(domainModel.Entities) != 2 {
		t.Fatalf("DomainModel.Entities len = %d, want 2", len(domainModel.Entities))
	}
	if len(domainModel.SemanticAcceptanceRules) != 5 {
		t.Fatalf("DomainModel.SemanticAcceptanceRules len = %d, want 5", len(domainModel.SemanticAcceptanceRules))
	}
	if bundle.PRD.ExecutionContract == nil {
		t.Fatal("PRD.ExecutionContract = nil, want compiler-projected contract")
	}
	assertSurfaceRelationSchema(t, bundle.PRD.ExecutionContract)
	if len(bundle.PRD.ExecutionContract.SurfaceContracts) != 3 {
		t.Fatalf("ExecutionContract.SurfaceContracts len = %d, want 3", len(bundle.PRD.ExecutionContract.SurfaceContracts))
	}
	for index, want := range []string{genericSurfaceOverviewID, genericSurfaceMutationID, genericSurfaceCollectionID} {
		if bundle.PRD.ExecutionContract.SurfaceContracts[index].TemplateBindingRef != want {
			t.Fatalf("ExecutionContract.SurfaceContracts[%d].TemplateBindingRef = %q, want %q", index, bundle.PRD.ExecutionContract.SurfaceContracts[index].TemplateBindingRef, want)
		}
	}
	var templateSlotMap TemplateSlotMap
	if err := json.Unmarshal(bundle.Files[templateSlotMapFileName], &templateSlotMap); err != nil {
		t.Fatalf("Unmarshal(template-slot-map.json) error = %v", err)
	}
	if templateSlotMap.TemplateID != bundle.BuilderInput.TemplateID {
		t.Fatalf("TemplateSlotMap.TemplateID = %q, want %q", templateSlotMap.TemplateID, bundle.BuilderInput.TemplateID)
	}
	if templateSlotMap.TemplateSubjectVersion != TemplateCompileSourceVersion("flutter-finance-lite", "v0.1.0") {
		t.Fatalf("TemplateSlotMap.TemplateSubjectVersion = %q, want %q", templateSlotMap.TemplateSubjectVersion, TemplateCompileSourceVersion("flutter-finance-lite", "v0.1.0"))
	}
	if len(templateSlotMap.Slots) != 8 {
		t.Fatalf("TemplateSlotMap.Slots len = %d, want 8", len(templateSlotMap.Slots))
	}
	if !strings.HasPrefix(templateSlotMap.Slots[1].SlotID, "finance-lite-") || templateSlotMap.Slots[1].BindingID != genericSurfaceCollectionID || templateSlotMap.Slots[1].TargetPaths[0] != "lib/views/entry_list_page.dart" {
		t.Fatalf("collection slot = %+v, want finance-lite private ledger projection", templateSlotMap.Slots[1])
	}
	if templateSlotMap.Slots[3].BindingID != publicBindingAppEntry || templateSlotMap.Slots[3].SlotKind != "app_entry" || templateSlotMap.Slots[3].TargetPaths[0] != "lib/main.dart" {
		t.Fatalf("app-entry slot = %+v, want finance-lite app_entry main.dart slot", templateSlotMap.Slots[3])
	}
	if !strings.HasPrefix(templateSlotMap.Slots[4].SlotID, "finance-lite-") || templateSlotMap.Slots[4].BindingID != publicBindingDomainCopy || templateSlotMap.Slots[4].OverridePolicy != "synchronize" {
		t.Fatalf("domain-copy slot = %+v, want finance-lite private synchronize copy slot", templateSlotMap.Slots[4])
	}
	var taskAllocation TaskAllocation
	if err := json.Unmarshal(bundle.Files[taskAllocationFileName], &taskAllocation); err != nil {
		t.Fatalf("Unmarshal(task-allocation.json) error = %v", err)
	}
	if len(taskAllocation.Units) != 5 {
		t.Fatalf("TaskAllocation.Units len = %d, want 5", len(taskAllocation.Units))
	}
	if taskAllocation.Units[0].AllocationID != "task-domain-models" || taskAllocation.Units[0].Wave != 0 {
		t.Fatalf("first allocation = %+v, want task-domain-models wave 0", taskAllocation.Units[0])
	}
	if taskAllocation.Units[1].Wave != 1 {
		t.Fatalf("second allocation wave = %d, want 1", taskAllocation.Units[1].Wave)
	}
	if taskAllocation.Units[2].Lane != "screen" {
		t.Fatalf("third allocation lane = %q, want screen", taskAllocation.Units[2].Lane)
	}
	if len(taskAllocation.Units[0].BindingRefs) == 0 || taskAllocation.Units[0].BindingRefs[0] != publicBindingDomainModel {
		t.Fatalf("first allocation binding_refs = %v, want include %s", taskAllocation.Units[0].BindingRefs, publicBindingDomainModel)
	}
	var acceptancePlan AcceptancePlan
	if err := json.Unmarshal(bundle.Files[acceptancePlanFileName], &acceptancePlan); err != nil {
		t.Fatalf("Unmarshal(acceptance-plan.json) error = %v", err)
	}
	if len(acceptancePlan.SemanticChecks) != 7 {
		t.Fatalf("AcceptancePlan.SemanticChecks len = %d, want 7", len(acceptancePlan.SemanticChecks))
	}
	if len(acceptancePlan.BehaviorChecks) != 2 {
		t.Fatalf("AcceptancePlan.BehaviorChecks len = %d, want 2", len(acceptancePlan.BehaviorChecks))
	}
	if len(acceptancePlan.StructureChecks) != 6 {
		t.Fatalf("AcceptancePlan.StructureChecks len = %d, want 6", len(acceptancePlan.StructureChecks))
	}
	if len(acceptancePlan.DeliveryChecks) != 0 {
		t.Fatalf("AcceptancePlan.DeliveryChecks len = %d, want 0 for non-real-build sample", len(acceptancePlan.DeliveryChecks))
	}
	var foundCopyReview bool
	for _, item := range acceptancePlan.SemanticChecks {
		if item.CheckID != "mrp-copy" {
			continue
		}
		foundCopyReview = true
		if item.SourceType != "manual_review_point" {
			t.Fatalf("mrp-copy source_type = %q, want manual_review_point", item.SourceType)
		}
		if !strings.Contains(strings.Join(item.BindingRefs, ","), publicBindingDomainCopy) {
			t.Fatalf("mrp-copy binding_refs = %v, want include %s", item.BindingRefs, publicBindingDomainCopy)
		}
	}
	if !foundCopyReview {
		t.Fatal("missing mrp-copy semantic review projection")
	}
	var prdApproval appruns.ApprovalRecord
	if err := json.Unmarshal(bundle.Files[prdApprovalFileName], &prdApproval); err != nil {
		t.Fatalf("Unmarshal(prd-approval.json) error = %v", err)
	}
	if prdApproval.Status != appruns.ApprovalStatusApproved || prdApproval.ApprovalType != appruns.ApprovalTypePRD {
		t.Fatalf("prd approval = %+v, want approved prd record", prdApproval)
	}
	if !strings.HasPrefix(prdApproval.SubjectVersion, "prd-bookkeeping-lite@0.1.0@sha256:") || strings.Count(prdApproval.SubjectVersion, "@sha256:") != 2 {
		t.Fatalf("prd approval subject_version = %q, want PRD approval version bound to PRD.md, PRD.json and requirement.md", prdApproval.SubjectVersion)
	}
	var templateApproval appruns.ApprovalRecord
	if err := json.Unmarshal(bundle.Files[templateApprovalFileName], &templateApproval); err != nil {
		t.Fatalf("Unmarshal(template-approval.json) error = %v", err)
	}
	if templateApproval.Status != appruns.ApprovalStatusApproved || templateApproval.ApprovalType != appruns.ApprovalTypeTemplate {
		t.Fatalf("template approval = %+v, want approved template record", templateApproval)
	}
	if !strings.HasPrefix(templateApproval.SubjectVersion, "selected-template@flutter-finance-lite@v0.1.0@sha256:") {
		t.Fatalf("template approval subject_version = %q, want content-bound template approval version", templateApproval.SubjectVersion)
	}
}

func TestWriteBundle(t *testing.T) {
	bundle, err := Compile(Request{RequirementText: "做一个简单记账 app，优先验证 Builder 技术链路。"})
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	outputDir := filepath.Join(t.TempDir(), "bundle")
	if err := WriteBundle(outputDir, bundle); err != nil {
		t.Fatalf("WriteBundle() error = %v", err)
	}
	for _, name := range bundle.FileNames() {
		if _, err := os.Stat(filepath.Join(outputDir, name)); err != nil {
			t.Fatalf("expected %s to exist: %v", name, err)
		}
	}
}

func TestRecompilePreservesPreparedPRDVersion(t *testing.T) {
	original, err := Compile(Request{
		RequirementText: "做一个简单记账 app，不考虑上架，只考虑功能实现，需要首页概览、记一笔和账单列表。",
		JobID:           "job-bookkeeping-lite",
		PRDID:           "prd-bookkeeping-lite",
		TemplateID:      "flutter-finance-lite",
	})
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	prepared := original.PRD
	prepared.Version = "0.2.0"
	prepared.UpdatedAt = "2026-03-31T00:00:00Z"

	recompiled, err := Recompile(RecompileRequest{
		RequirementText: string(original.Files[requirementFileName]),
		PRD:             prepared,
		JobID:           original.BuilderInput.JobID,
		TemplateID:      original.BuilderInput.TemplateID,
		ExecutorImage:   original.BuilderInput.ExecutorImage,
	})
	if err != nil {
		t.Fatalf("Recompile() error = %v", err)
	}
	if recompiled.PRD.Version != "0.2.0" {
		t.Fatalf("PRD.Version = %q, want 0.2.0", recompiled.PRD.Version)
	}
	if !strings.HasPrefix(recompiled.BuilderInput.PreparedPRDSubjectVersion, "prd-bookkeeping-lite@0.2.0@sha256:") {
		t.Fatalf("PreparedPRDSubjectVersion = %q, want prd-bookkeeping-lite@0.2.0@sha256:*", recompiled.BuilderInput.PreparedPRDSubjectVersion)
	}
	var decoded PRD
	if err := json.Unmarshal(recompiled.Files[prdJSONFileName], &decoded); err != nil {
		t.Fatalf("Unmarshal(recompiled PRD.json) error = %v", err)
	}
	if decoded.Version != "0.2.0" {
		t.Fatalf("decoded PRD version = %q, want 0.2.0", decoded.Version)
	}
}

func TestRecompileReadsLegacyScreenCompatButDropsItFromOutput(t *testing.T) {
	original, err := Compile(Request{
		RequirementText: "做一个待办事项 app，需要首页摘要、新建待办、任务列表和详情页，优先保证本地可用。",
		JobID:           "job-todo-lite-sample-001",
		PRDID:           "prd-todo-lite-sample-001",
		TemplateID:      "flutter-open-lite",
	})
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}

	prepared := original.PRD
	prepared.SurfaceList = nil
	prepared.ScreenList = nil
	legacyScreenBySurface := make(map[string]string, len(original.PRD.SurfaceList))
	legacyScreenList := make([]Screen, 0, len(original.PRD.SurfaceList))
	for _, surface := range original.PRD.SurfaceList {
		legacyScreenRef := legacyScreenRefForGenericSurfaceID(surface.SurfaceID)
		if legacyScreenRef == "" {
			t.Fatalf("surface %q missing compatibility screen ref fixture", surface.SurfaceID)
		}
		legacyScreenBySurface[surface.SurfaceID] = legacyScreenRef
		legacyScreenList = append(legacyScreenList, Screen{
			ScreenID:        legacyScreenRef,
			Name:            surface.Label,
			Purpose:         surface.Purpose,
			PrimaryFeatures: append([]string(nil), surface.PrimaryFeatureRefs...),
		})
	}
	prepared.ScreenList = legacyScreenList
	for index := range prepared.FeatureList {
		feature := &prepared.FeatureList[index]
		legacyScreens := make([]string, 0, len(feature.RelatedSurfaceRefs))
		for _, surfaceRef := range feature.RelatedSurfaceRefs {
			if legacyScreen := strings.TrimSpace(legacyScreenBySurface[surfaceRef]); legacyScreen != "" {
				legacyScreens = append(legacyScreens, legacyScreen)
			}
		}
		feature.RelatedSurfaceRefs = nil
		feature.RelatedScreens = legacyScreens
	}
	for flowIndex := range prepared.UserFlows {
		for stepIndex := range prepared.UserFlows[flowIndex].Steps {
			step := &prepared.UserFlows[flowIndex].Steps[stepIndex]
			step.ScreenRef = legacyScreenBySurface[step.SurfaceRef]
			step.SurfaceRef = ""
		}
	}

	recompiled, err := Recompile(RecompileRequest{
		RequirementText: string(original.Files[requirementFileName]),
		PRD:             prepared,
		JobID:           original.BuilderInput.JobID,
		TemplateID:      original.BuilderInput.TemplateID,
		ExecutorImage:   original.BuilderInput.ExecutorImage,
	})
	if err != nil {
		t.Fatalf("Recompile() error = %v", err)
	}

	if len(recompiled.PRD.SurfaceList) == 0 {
		t.Fatal("PRD.SurfaceList is empty, want compatibility screen_list projected back to surfaces")
	}
	if len(recompiled.PRD.ScreenList) != 0 {
		t.Fatalf("PRD.ScreenList = %v, want compatibility screen_list omitted after normalization", recompiled.PRD.ScreenList)
	}
	for _, surface := range recompiled.PRD.SurfaceList {
		if surface.LegacyScreenRef != "" {
			t.Fatalf("surface %q legacy_screen_ref = %q, want omitted compatibility output after normalization", surface.SurfaceID, surface.LegacyScreenRef)
		}
	}
	for _, feature := range recompiled.PRD.FeatureList {
		if len(feature.RelatedSurfaceRefs) == 0 {
			t.Fatalf("feature %q related_surface_refs = %v, want derived surface refs", feature.FeatureID, feature.RelatedSurfaceRefs)
		}
		if len(feature.RelatedScreens) != 0 {
			t.Fatalf("feature %q related_screens = %v, want compatibility screens omitted after normalization", feature.FeatureID, feature.RelatedScreens)
		}
	}
	for _, flow := range recompiled.PRD.UserFlows {
		for _, step := range flow.Steps {
			if step.SurfaceRef == "" {
				t.Fatalf("user flow %q step %q surface_ref is empty, want compatibility screen_ref projected to surface_ref", flow.FlowID, step.StepID)
			}
			if step.ScreenRef != "" {
				t.Fatalf("user flow %q step %q screen_ref = %q, want compatibility screen_ref omitted after normalization", flow.FlowID, step.StepID, step.ScreenRef)
			}
		}
	}
	contractJSON, err := json.Marshal(recompiled.PRD.ExecutionContract)
	if err != nil {
		t.Fatalf("Marshal(execution contract) error = %v", err)
	}
	var recompiledPRD map[string]any
	if err := json.Unmarshal(recompiled.Files[prdJSONFileName], &recompiledPRD); err != nil {
		t.Fatalf("Unmarshal(recompiled PRD.json) error = %v", err)
	}
	if _, ok := recompiledPRD["screen_list"]; ok {
		t.Fatalf("PRD.json = %s, want top-level screen_list omitted", string(recompiled.Files[prdJSONFileName]))
	}
	if bytes.Contains(recompiled.Files[prdJSONFileName], []byte(`"legacy_screen_ref"`)) {
		t.Fatalf("PRD.json = %s, want legacy_screen_ref omitted", string(recompiled.Files[prdJSONFileName]))
	}
	prdMarkdown := string(recompiled.Files[prdMarkdownFileName])
	if !strings.Contains(prdMarkdown, "## 交互承载单元") {
		t.Fatalf("PRD.md = %s, want surface-first interaction section", prdMarkdown)
	}
	if strings.Contains(prdMarkdown, "## 页面列表") {
		t.Fatalf("PRD.md = %s, want legacy 页面列表 section omitted", prdMarkdown)
	}
	if strings.Contains(string(contractJSON), `"entry_screen_ref"`) {
		t.Fatalf("execution contract JSON = %s, want entry_screen_ref omitted", string(contractJSON))
	}
	if strings.Contains(string(contractJSON), `"screen_ref"`) {
		t.Fatalf("execution contract JSON = %s, want screen_ref omitted", string(contractJSON))
	}
}

func TestSurfaceBuildersDoNotGenerateLegacyScreenRefs(t *testing.T) {
	genericFeatures := []Feature{
		{FeatureID: "feature-home-overview", RelatedSurfaceRefs: []string{genericSurfaceOverviewID}},
		{FeatureID: "feature-record-list", RelatedSurfaceRefs: []string{genericSurfaceCollectionID}},
		{FeatureID: "feature-record-form", RelatedSurfaceRefs: []string{genericSurfaceMutationID}},
		{FeatureID: "feature-record-delete", RelatedSurfaceRefs: []string{genericSurfaceInspectionID}},
	}

	testCases := []struct {
		name     string
		surfaces []InteractionSurface
	}{
		{name: "generic", surfaces: buildGenericSurfaceList(genericFeatures)},
		{name: "relation-rich", surfaces: buildRelationRichSurfaceList()},
		{name: "inventory", surfaces: buildInventorySurfaceList()},
		{name: "bookkeeping", surfaces: buildBookkeepingSurfaceList()},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			if len(testCase.surfaces) == 0 {
				t.Fatal("surface list is empty, want generated surfaces")
			}
			for _, surface := range testCase.surfaces {
				if surface.LegacyScreenRef != "" {
					t.Fatalf("surface %q legacy_screen_ref = %q, want source builders surface-first by default", surface.SurfaceID, surface.LegacyScreenRef)
				}
			}
		})
	}
}

func TestBuildTaskAllocationMapsLegacyScreenRefsToSurfaceRefs(t *testing.T) {
	spec := domainSpec{
		JobID:      "job-legacy-screen-allocation",
		PRDID:      "prd-legacy-screen-allocation",
		TemplateID: "flutter-open-lite",
		SurfaceList: []InteractionSurface{
			{SurfaceID: genericSurfaceOverviewID, Label: "概览承载单元", Purpose: "概览", LegacyScreenRef: "screen-home"},
			{SurfaceID: genericSurfaceCollectionID, Label: "集合浏览承载单元", Purpose: "列表", LegacyScreenRef: "screen-list"},
		},
	}
	tasks := []appruns.TaskBundleItem{{
		TaskID:      "task-legacy-screen-only",
		Title:       "legacy screen task",
		Category:    appruns.TaskCategoryScreen,
		TaskType:    appruns.BuilderRuntimeTaskTypeDualFileWiring,
		TargetPaths: []string{"lib/main.dart"},
		AllocationTransition: &appruns.TaskAllocationTransition{
			AllocationID: "task-legacy-screen-only",
			ScreenRefs:   []string{"screen-home", "screen-list"},
		},
	}}

	allocation := buildTaskAllocation(tasks, spec)
	if len(allocation.Units) != 1 {
		t.Fatalf("TaskAllocation.Units len = %d, want 1", len(allocation.Units))
	}
	unit := allocation.Units[0]
	if !reflect.DeepEqual(unit.SurfaceRefs, []string{genericSurfaceOverviewID, genericSurfaceCollectionID}) {
		t.Fatalf("task allocation surface_refs = %v, want [%s %s]", unit.SurfaceRefs, genericSurfaceOverviewID, genericSurfaceCollectionID)
	}
	if len(unit.ScreenRefs) != 0 {
		t.Fatalf("task allocation screen_refs = %v, want omitted compatibility refs", unit.ScreenRefs)
	}
	allocationJSON, err := json.Marshal(allocation)
	if err != nil {
		t.Fatalf("Marshal(task allocation) error = %v", err)
	}
	if strings.Contains(string(allocationJSON), `"screen_refs"`) {
		t.Fatalf("task allocation JSON = %s, want screen_refs omitted", string(allocationJSON))
	}
}

func TestCompileBookkeepingRequirementWithRealBuild(t *testing.T) {
	bundle, err := Compile(Request{
		RequirementText: "做一个简单记账 app，不考虑上架，只考虑功能实现，需要首页概览、记一笔和账单列表。",
		RealBuild:       true,
	})
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	if bundle.BuilderInput.ExecutorImage != "picoclaw/appfactory-builder:local" {
		t.Fatalf("ExecutorImage = %q, want picoclaw/appfactory-builder:local", bundle.BuilderInput.ExecutorImage)
	}
	if len(bundle.BuilderInput.AcceptanceChecks) != 7 {
		t.Fatalf("AcceptanceChecks len = %d, want 7", len(bundle.BuilderInput.AcceptanceChecks))
	}
	if bundle.BuilderInput.AcceptanceChecks[0].Commands[0] != "flutter pub get" {
		t.Fatalf("first command = %q, want flutter pub get", bundle.BuilderInput.AcceptanceChecks[0].Commands[0])
	}
	if bundle.BuilderInput.AcceptanceChecks[1].CheckID != "check-counter-demo-removed" {
		t.Fatalf("second check id = %q, want check-counter-demo-removed", bundle.BuilderInput.AcceptanceChecks[1].CheckID)
	}
	if bundle.BuilderInput.GoalSummary != "在 flutter-finance-lite 模板基础上完成一个可用的离线记账 MVP，必须彻底替换默认 counter demo，至少实现收支概览、记账动作、账单集合浏览和本地持久化，并通过 analyze、test 和 debug APK 构建验证。" {
		t.Fatalf("GoalSummary = %q", bundle.BuilderInput.GoalSummary)
	}
	planText := string(bundle.Files[planFileName])
	if !strings.Contains(planText, "关联需求：feature-local-data, feature-home-summary") {
		t.Fatalf("implementation plan missing related requirements section: %s", planText)
	}
	if !strings.Contains(planText, "风险提示：如果实体字段频繁变化，后续承载单元和测试都会跟着返工") {
		t.Fatalf("implementation plan missing risk section: %s", planText)
	}
	var profile map[string]any
	if err := json.Unmarshal(bundle.BuilderInput.CommandProfile, &profile); err != nil {
		t.Fatalf("Unmarshal(command_profile) error = %v", err)
	}
	allowedCommands, ok := profile["allowed_commands"].([]any)
	if !ok || len(allowedCommands) != 2 || allowedCommands[0] != "flutter" || allowedCommands[1] != "grep" {
		t.Fatalf("allowed_commands = %v, want [flutter grep]", profile["allowed_commands"])
	}
	if len(bundle.BuilderInput.AllowedPaths) != 6 || bundle.BuilderInput.AllowedPaths[0] != "lib/**" || bundle.BuilderInput.AllowedPaths[4] != "android/app/build.gradle.kts" || bundle.BuilderInput.AllowedPaths[5] != "android/app/src/main/res/values/strings.xml" {
		t.Fatalf("AllowedPaths = %v, want Flutter profile allowed paths", bundle.BuilderInput.AllowedPaths)
	}
	if len(bundle.BuilderInput.ProtectedPaths) != 8 || bundle.BuilderInput.ProtectedPaths[0] != "android/app/src/main/AndroidManifest.xml" {
		t.Fatalf("ProtectedPaths = %v, want Flutter profile protected paths", bundle.BuilderInput.ProtectedPaths)
	}
	if len(bundle.BuilderInput.KnowledgePack) != 4 || bundle.BuilderInput.KnowledgePack[3].SkillID != "flutter-build-closure" {
		t.Fatalf("KnowledgePack = %+v, want Flutter skill pack through builder input", bundle.BuilderInput.KnowledgePack)
	}
}

func TestCompileBookkeepingRequirementIncludesFlutterStructuralChecksByDefault(t *testing.T) {
	bundle, err := Compile(Request{
		RequirementText: "做一个简单记账 app，不考虑上架，只考虑功能实现，需要首页概览、记一笔和账单列表。",
	})
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	if bundle.BuilderInput.ExecutorImage != "" {
		t.Fatalf("ExecutorImage = %q, want empty on default compile path", bundle.BuilderInput.ExecutorImage)
	}
	if len(bundle.BuilderInput.AcceptanceChecks) != 6 {
		t.Fatalf("AcceptanceChecks len = %d, want 6", len(bundle.BuilderInput.AcceptanceChecks))
	}
	for index, checkID := range []string{"check-context-ready", "check-bookkeeping-scope", "check-plan-ready", "check-structural-template-files-ready", "check-legacy-thin-fallback-probe", "check-legacy-thin-fallback-metadata"} {
		if bundle.BuilderInput.AcceptanceChecks[index].CheckID != checkID {
			t.Fatalf("acceptance_check[%d] = %q, want %q", index, bundle.BuilderInput.AcceptanceChecks[index].CheckID, checkID)
		}
	}
	if bundle.BuilderInput.TaskBundle[1].TargetPaths[0] != "lib/repositories/entry_repository.dart" {
		t.Fatalf("storage target_paths = %v, want repository landing path", bundle.BuilderInput.TaskBundle[1].TargetPaths)
	}
	if bundle.BuilderInput.TaskBundle[2].TargetPaths[0] != "lib/main.dart" {
		t.Fatalf("screen target_paths = %v, want main.dart landing path", bundle.BuilderInput.TaskBundle[2].TargetPaths)
	}
	if bundle.BuilderInput.TaskBundle[3].TargetPaths[0] != "lib/controllers/home_controller.dart" {
		t.Fatalf("flow target_paths = %v, want controller landing path", bundle.BuilderInput.TaskBundle[3].TargetPaths)
	}
	if bundle.BuilderInput.TaskBundle[3].Title != "接通记账动作、概览刷新与账单集合回显主流程" {
		t.Fatalf("flow task title = %q, want bookkeeping flow title", bundle.BuilderInput.TaskBundle[3].Title)
	}
	if bundle.PRD.FeatureList[2].Summary != "按时间倒序展示账单集合，并支持核对金额和分类。" {
		t.Fatalf("ledger summary = %q, want bookkeeping ledger summary", bundle.PRD.FeatureList[2].Summary)
	}
	if bundle.PRD.FeatureList[1].Summary != "支持录入金额、类型、分类、日期和备注，并提交新增账单。" {
		t.Fatalf("entry form summary = %q, want bookkeeping entry summary", bundle.PRD.FeatureList[1].Summary)
	}
	if bundle.PRD.FeatureList[3].FeatureID != "feature-local-data" {
		t.Fatalf("fourth feature id = %q, want feature-local-data", bundle.PRD.FeatureList[3].FeatureID)
	}
	if bundle.PRD.AcceptanceCriteria[2].Description != "账单集合浏览承载单元按时间倒序显示账单，并展示金额和分类。" {
		t.Fatalf("ledger acceptance criterion = %q, want bookkeeping ledger acceptance", bundle.PRD.AcceptanceCriteria[2].Description)
	}
	if bundle.PRD.AcceptanceCriteria[3].CriterionID != "ac-persistence" {
		t.Fatalf("persistence acceptance criterion = %q, want ac-persistence", bundle.PRD.AcceptanceCriteria[3].CriterionID)
	}
	fields := bundle.PRD.DataEntities[0].Fields
	if len(fields) != 6 || fields[0].Name != "entry_id" || fields[1].Name != "entry_type" || fields[2].Name != "amount" || fields[3].Name != "category" || fields[4].Name != "occurred_on" || fields[5].Name != "note" {
		t.Fatalf("bookkeeping entity fields = %+v, want stable entry mapping fields", fields)
	}
}

func TestCompileGenericRequirementUsesRegistrySelection(t *testing.T) {
	bundle, err := Compile(Request{RequirementText: "做一个简单 Android MVP，只要一个主页面和一次核心操作。"})
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	if bundle.BuilderInput.TemplateID != "flutter-open-lite" {
		t.Fatalf("TemplateID = %q, want flutter-open-lite", bundle.BuilderInput.TemplateID)
	}
	fitReport := string(bundle.Files[fitReportFileName])
	if !strings.Contains(fitReport, "命中模板注册表条目：flutter-open-lite") {
		t.Fatalf("template fit report missing registry entry reason: %s", fitReport)
	}
	if !strings.Contains(fitReport, "Pinned Ref: v0.1.0") || !strings.Contains(fitReport, "Health Status: healthy") {
		t.Fatalf("template fit report missing pinned ref or health status: %s", fitReport)
	}
	if !strings.Contains(fitReport, "summary-card") || !strings.Contains(fitReport, "detail") || !strings.Contains(fitReport, "local-storage") {
		t.Fatalf("template fit report missing capability coverage: %s", fitReport)
	}
	if !strings.Contains(fitReport, "analyze、test 与 debug APK build structural checks") {
		t.Fatalf("template fit report missing structural check narrative: %s", fitReport)
	}
	if strings.Contains(fitReport, "generic real-check 还没有切到真实 Flutter analyze/test/build 主链") {
		t.Fatalf("template fit report still contains outdated generic real-check gap: %s", fitReport)
	}
	if len(bundle.BuilderInput.AcceptanceChecks) != 6 {
		t.Fatalf("AcceptanceChecks len = %d, want 6", len(bundle.BuilderInput.AcceptanceChecks))
	}
	if len(bundle.BuilderInput.TaskBundle) != 15 {
		t.Fatalf("TaskBundle len = %d, want 15", len(bundle.BuilderInput.TaskBundle))
	}
	if bundle.PRD.FeatureList[1].Summary != "承接记录集合浏览、状态筛选与结果检查入口。" {
		t.Fatalf("record list summary = %q, want collection+filter summary", bundle.PRD.FeatureList[1].Summary)
	}
	collectionSurfaceTask := findTaskBundleItem(t, bundle.BuilderInput.TaskBundle, taskBindCollectionSurfaceID)
	if len(collectionSurfaceTask.TargetPaths) != 1 || collectionSurfaceTask.TargetPaths[0] != "lib/views/record_list_page.dart" {
		t.Fatalf("collection surface target_paths = %v, want record_list_page single-file target", collectionSurfaceTask.TargetPaths)
	}
	listControllerTask := findTaskBundleItem(t, bundle.BuilderInput.TaskBundle, "task-create-list-controller")
	if listControllerTask.Title != "创建集合浏览与筛选控制器" {
		t.Fatalf("list controller task title = %q, want filter-aware list controller title", listControllerTask.Title)
	}
}

func TestCompileGenericRequirementUsesOpenLiteRealBuildChecksWhenExecutorImageProvided(t *testing.T) {
	bundle, err := Compile(Request{
		RequirementText: "做一个待办事项 app，需要首页摘要、新建待办、任务列表和详情页，优先保证本地可用。",
		ExecutorImage:   "picoclaw/appfactory-builder:local",
	})
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	if bundle.BuilderInput.TemplateID != "flutter-open-lite" {
		t.Fatalf("TemplateID = %q, want flutter-open-lite", bundle.BuilderInput.TemplateID)
	}
	if bundle.BuilderInput.ExecutorImage != "picoclaw/appfactory-builder:local" {
		t.Fatalf("ExecutorImage = %q, want picoclaw/appfactory-builder:local", bundle.BuilderInput.ExecutorImage)
	}
	if len(bundle.BuilderInput.AcceptanceChecks) != 9 {
		t.Fatalf("AcceptanceChecks len = %d, want 9", len(bundle.BuilderInput.AcceptanceChecks))
	}
	for index, checkID := range []string{"check-flutter-pub-get", "check-open-lite-counter-demo-removed", "check-open-lite-record-flow-wiring", "check-open-lite-local-persistence-wiring", "check-profile-open-lite-domain-branding", "check-profile-open-lite-domain-language", "check-flutter-analyze", "check-flutter-test", "check-flutter-build-apk"} {
		if bundle.BuilderInput.AcceptanceChecks[index].CheckID != checkID {
			t.Fatalf("acceptance_check[%d] = %q, want %q", index, bundle.BuilderInput.AcceptanceChecks[index].CheckID, checkID)
		}
	}
	commandProfile := string(bundle.BuilderInput.CommandProfile)
	if !strings.Contains(commandProfile, "flutter-builder-p0") || !strings.Contains(commandProfile, "flutter") {
		t.Fatalf("command_profile = %s, want flutter builder profile", commandProfile)
	}
	if len(bundle.BuilderInput.AllowedPaths) != 6 || bundle.BuilderInput.AllowedPaths[4] != "android/app/build.gradle.kts" || bundle.BuilderInput.AllowedPaths[5] != "android/app/src/main/res/values/strings.xml" {
		t.Fatalf("AllowedPaths = %v, want Android override points in real-build path", bundle.BuilderInput.AllowedPaths)
	}
	if len(bundle.BuilderInput.ProtectedPaths) != 8 || bundle.BuilderInput.ProtectedPaths[0] != "android/app/src/main/AndroidManifest.xml" {
		t.Fatalf("ProtectedPaths = %v, want narrowed protected paths in real-build path", bundle.BuilderInput.ProtectedPaths)
	}
	for _, check := range bundle.BuilderInput.AcceptanceChecks {
		if check.CheckID == "check-legacy-thin-fallback-probe" || check.CheckID == "check-legacy-thin-fallback-metadata" {
			t.Fatalf("unexpected legacy fallback check in real-build path: %q", check.CheckID)
		}
	}
}

func TestCompileWeightTrackerRealBuildChecksIncludeDomainSemanticGuards(t *testing.T) {
	requirementData, err := os.ReadFile("/home/kkroid/github/picoclaw/examples/appfactory/generic/weight-tracker/requirement.md")
	if err != nil {
		t.Fatalf("ReadFile(weight-tracker requirement) error = %v", err)
	}

	bundle, err := Compile(Request{
		RequirementText: string(requirementData),
		ExecutorImage:   "picoclaw/appfactory-builder:local",
	})
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}

	foundBrandingCheck := false
	foundDomainLanguageCheck := false
	for _, check := range bundle.BuilderInput.AcceptanceChecks {
		switch check.CheckID {
		case "check-profile-open-lite-domain-branding":
			foundBrandingCheck = true
			if !strings.Contains(check.Commands[0], "Open Lite Seed|open_lite_seed") {
				t.Fatalf("branding check command = %q, want Open Lite Seed guard", check.Commands[0])
			}
		case "check-profile-open-lite-domain-language":
			foundDomainLanguageCheck = true
			if !strings.Contains(check.Commands[0], "体重|weight|recorded_at|latest_weight|trend") {
				t.Fatalf("domain language check command = %q, want weight semantic guard", check.Commands[0])
			}
		}
	}
	if !foundBrandingCheck || !foundDomainLanguageCheck {
		t.Fatalf("AcceptanceChecks = %+v, want branding and weight semantic guards", bundle.BuilderInput.AcceptanceChecks)
	}
}

func TestCompileGenericExampleRequirementsPreserveDomainSemantics(t *testing.T) {
	testCases := []struct {
		name                    string
		reqPath                 string
		wantTitle               string
		wantEntityID            string
		wantFieldNames          []string
		wantListSummaryContains string
		wantFormSummaryContains string
		wantOverviewAcceptance  string
	}{
		{name: "todo-lite", reqPath: "/home/kkroid/github/picoclaw/examples/appfactory/generic/todo-lite/requirement.md", wantTitle: "待办事项 App", wantEntityID: "entity-todo-item", wantFieldNames: []string{"task_id", "title", "category", "status", "note"}, wantListSummaryContains: "待办", wantFormSummaryContains: "标题、分类、状态和备注", wantOverviewAcceptance: "待整理、进行中、已完成待办"},
		{name: "habit-checkin", reqPath: "/home/kkroid/github/picoclaw/examples/appfactory/generic/habit-checkin/requirement.md", wantTitle: "习惯打卡 App", wantEntityID: "entity-habit-record", wantFieldNames: []string{"habit_record_id", "title", "category", "status", "checked_at", "note"}, wantListSummaryContains: "习惯记录", wantFormSummaryContains: "习惯记录", wantOverviewAcceptance: "总记录数和完成状态分布"},
		{name: "weight-tracker", reqPath: "/home/kkroid/github/picoclaw/examples/appfactory/generic/weight-tracker/requirement.md", wantTitle: "体重记录 App", wantEntityID: "entity-weight-record", wantFieldNames: []string{"record_id", "weight", "recorded_at", "note"}, wantListSummaryContains: "体重记录", wantFormSummaryContains: "体重、日期和备注", wantOverviewAcceptance: "最新体重、记录数量和最近趋势"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			requirementData, err := os.ReadFile(testCase.reqPath)
			if err != nil {
				t.Fatalf("ReadFile(%s) error = %v", testCase.reqPath, err)
			}

			bundle, err := Compile(Request{RequirementText: string(requirementData)})
			if err != nil {
				t.Fatalf("Compile() error = %v", err)
			}

			if bundle.BuilderInput.TemplateID != "flutter-open-lite" {
				t.Fatalf("TemplateID = %q, want flutter-open-lite", bundle.BuilderInput.TemplateID)
			}

			if bundle.PRD.Title != testCase.wantTitle {
				t.Fatalf("PRD.Title = %q, want %q", bundle.PRD.Title, testCase.wantTitle)
			}

			if len(bundle.PRD.ScreenList) != 0 {
				t.Fatalf("ScreenList = %v, want omitted compatibility output", bundle.PRD.ScreenList)
			}
			if len(bundle.PRD.SurfaceList) != 4 {
				t.Fatalf("SurfaceList len = %d, want 4", len(bundle.PRD.SurfaceList))
			}
			for _, surface := range bundle.PRD.SurfaceList {
				if surface.LegacyScreenRef != "" {
					t.Fatalf("surface %q legacy_screen_ref = %q, want omitted compatibility output", surface.SurfaceID, surface.LegacyScreenRef)
				}
			}

			if len(bundle.PRD.DataEntities) != 2 {
				t.Fatalf("DataEntities len = %d, want 2", len(bundle.PRD.DataEntities))
			}
			if bundle.PRD.DataEntities[0].EntityID != testCase.wantEntityID {
				t.Fatalf("record entity id = %q, want %q", bundle.PRD.DataEntities[0].EntityID, testCase.wantEntityID)
			}
			if !strings.Contains(bundle.PRD.DataEntities[1].Name, "摘要") {
				t.Fatalf("summary entity name = %q, want semantic summary entity", bundle.PRD.DataEntities[1].Name)
			}

			fields := bundle.PRD.DataEntities[0].Fields
			wantFields := testCase.wantFieldNames
			if len(fields) != len(wantFields) {
				t.Fatalf("record field len = %d, want %d", len(fields), len(wantFields))
			}
			for index, fieldName := range wantFields {
				if fields[index].Name != fieldName {
					t.Fatalf("record field[%d] = %q, want %q", index, fields[index].Name, fieldName)
				}
			}

			if bundle.PRD.FeatureList[2].FeatureID != "feature-record-form" {
				t.Fatalf("form feature id = %q, want feature-record-form", bundle.PRD.FeatureList[2].FeatureID)
			}
			if !strings.Contains(bundle.PRD.FeatureList[1].Summary, testCase.wantListSummaryContains) {
				t.Fatalf("record list summary = %q, want contain %q", bundle.PRD.FeatureList[1].Summary, testCase.wantListSummaryContains)
			}
			if !strings.Contains(bundle.PRD.FeatureList[2].Summary, testCase.wantFormSummaryContains) {
				t.Fatalf("record form summary = %q, want contain %q", bundle.PRD.FeatureList[2].Summary, testCase.wantFormSummaryContains)
			}
			if bundle.PRD.FeatureList[3].FeatureID != "feature-record-delete" {
				t.Fatalf("delete feature id = %q, want feature-record-delete", bundle.PRD.FeatureList[3].FeatureID)
			}
			if !strings.Contains(bundle.PRD.AcceptanceCriteria[0].Description, testCase.wantOverviewAcceptance) {
				t.Fatalf("overview acceptance = %q, want contain %q", bundle.PRD.AcceptanceCriteria[0].Description, testCase.wantOverviewAcceptance)
			}

			foundHomePageTask := false
			for _, task := range bundle.BuilderInput.TaskBundle {
				if task.TaskID == taskBindOverviewSurfaceID {
					foundHomePageTask = true
					if len(task.TargetPaths) != 1 || task.TargetPaths[0] != "lib/views/home_page.dart" {
						t.Fatalf("overview surface target_paths = %v, want single home_page target", task.TargetPaths)
					}
				}
			}
			if !foundHomePageTask {
				t.Fatalf("task bundle missing %s: %+v", taskBindOverviewSurfaceID, bundle.BuilderInput.TaskBundle)
			}

			wantTaskIDs := []string{"task-create-record-model", "task-create-summary-model", "task-create-copy", taskBindOverviewSurfaceID, "task-create-test", "task-create-android-branding", "task-create-android-build-config"}
			for _, taskID := range wantTaskIDs {
				found := false
				for _, task := range bundle.BuilderInput.TaskBundle {
					if task.TaskID == taskID {
						found = true
						break
					}
				}
				if !found {
					t.Fatalf("task bundle missing %q: %+v", taskID, bundle.BuilderInput.TaskBundle)
				}
			}

			var foundDomainCopy bool
			for _, task := range bundle.BuilderInput.TaskBundle {
				if task.TaskID == "task-create-copy" {
					foundDomainCopy = true
					if len(task.TargetPaths) != 1 || task.TargetPaths[0] != "lib/template/open_lite_copy.dart" {
						t.Fatalf("domain copy target_paths = %v, want single copy override point", task.TargetPaths)
					}
					if !strings.Contains(strings.Join(task.CompletionCriteria, " | "), "领域") && !strings.Contains(strings.Join(task.CompletionCriteria, " | "), "标题") {
						t.Fatalf("domain copy completion criteria = %v, want domain wording requirements", task.CompletionCriteria)
					}
				}
			}
			if !foundDomainCopy {
				t.Fatalf("task bundle missing domain copy task: %+v", bundle.BuilderInput.TaskBundle)
			}

			androidBrandingTask := findTaskBundleItem(t, bundle.BuilderInput.TaskBundle, "task-create-android-branding")
			if len(androidBrandingTask.TargetPaths) != 1 || androidBrandingTask.TargetPaths[0] != "android/app/src/main/res/values/strings.xml" {
				t.Fatalf("android branding target_paths = %v, want strings.xml single-file target", androidBrandingTask.TargetPaths)
			}
			if androidBrandingTask.RouteHint != appruns.TaskRouteHintDeterministic {
				t.Fatalf("android branding route_hint = %q, want deterministic", androidBrandingTask.RouteHint)
			}
			androidBuildConfigTask := findTaskBundleItem(t, bundle.BuilderInput.TaskBundle, "task-create-android-build-config")
			if len(androidBuildConfigTask.TargetPaths) != 1 || androidBuildConfigTask.TargetPaths[0] != "android/app/build.gradle.kts" {
				t.Fatalf("android build config target_paths = %v, want build.gradle.kts single-file target", androidBuildConfigTask.TargetPaths)
			}
			if androidBuildConfigTask.RouteHint != appruns.TaskRouteHintDeterministic {
				t.Fatalf("android build config route_hint = %q, want deterministic", androidBuildConfigTask.RouteHint)
			}
		})
	}
}

func TestCompileJobsUIRequestGeneratesUniqueDefaultIDs(t *testing.T) {
	wantTimestamp := "20260403074000123"
	bundle, err := Compile(Request{
		RequirementText:   "做一个简单记账 app，不考虑上架，只考虑功能实现，需要首页概览、记一笔和账单列表。",
		RequirementSource: "jobs-ui",
		Now: func() time.Time {
			return time.Unix(0, 1775202000123456789).UTC()
		},
	})
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	if bundle.BuilderInput.JobID != "JOB_"+wantTimestamp {
		t.Fatalf("JobID = %q, want generated jobs-ui id", bundle.BuilderInput.JobID)
	}
	if bundle.PRD.ID != "PRD_"+wantTimestamp {
		t.Fatalf("PRD.ID = %q, want generated jobs-ui prd id", bundle.PRD.ID)
	}
	if !strings.HasPrefix(bundle.BuilderInput.PreparedPRDSubjectVersion, "PRD_"+wantTimestamp+"@0.1.0@sha256:") {
		t.Fatalf("PreparedPRDSubjectVersion = %q, want generated PRD id prefix", bundle.BuilderInput.PreparedPRDSubjectVersion)
	}
	var input map[string]any
	if err := json.Unmarshal(bundle.Files[builderInputFileName], &input); err != nil {
		t.Fatalf("Unmarshal(builder-input.json) error = %v", err)
	}
	if input["job_id"] != "JOB_"+wantTimestamp {
		t.Fatalf("builder-input job_id = %v, want generated jobs-ui id", input["job_id"])
	}
}

func TestCompileRejectsUnknownTemplateID(t *testing.T) {
	_, err := Compile(Request{RequirementText: "做一个简单记账 app。", TemplateID: "unknown-template"})
	if err == nil {
		t.Fatal("Compile() error = nil, want unknown template failure")
	}
	if !strings.Contains(err.Error(), "not found in registry") {
		t.Fatalf("Compile() error = %v, want registry not found", err)
	}
}

func legacyScreenRefForGenericSurfaceID(surfaceID string) string {
	switch surfaceID {
	case genericSurfaceOverviewID:
		return "screen-home"
	case genericSurfaceCollectionID:
		return "screen-list"
	case genericSurfaceMutationID:
		return "screen-editor"
	case genericSurfaceInspectionID:
		return "screen-detail"
	default:
		return ""
	}
}
