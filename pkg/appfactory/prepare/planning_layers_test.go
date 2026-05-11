package prepare

import (
	"reflect"
	"strings"
	"testing"

	appruns "github.com/sipeed/oneappfactory/pkg/appfactory/runs"
)

func TestAnalyzeRequirementLayerRoutesBookkeeping(t *testing.T) {
	spec := analyzeRequirement(Request{}, "做一个简单记账 app，需要首页概览、记一笔和账单列表。")
	if spec.Kind != "bookkeeping" {
		t.Fatalf("Kind = %q, want bookkeeping", spec.Kind)
	}
	if len(spec.TaskBundle) != 5 {
		t.Fatalf("TaskBundle len = %d, want 5", len(spec.TaskBundle))
	}
	if spec.TaskBundle[0].Category != appruns.TaskCategoryDomain {
		t.Fatalf("first category = %q, want domain", spec.TaskBundle[0].Category)
	}
}

func TestCompileBookkeepingSpecLayerUsesSurfaceFirstProjection(t *testing.T) {
	spec := compileBookkeepingSpec(Request{}, "做一个简单记账 app，需要首页概览、记一笔和账单列表。")
	if len(spec.SurfaceList) != 3 {
		t.Fatalf("SurfaceList len = %d, want 3", len(spec.SurfaceList))
	}
	for index, surfaceID := range []string{genericSurfaceOverviewID, genericSurfaceMutationID, genericSurfaceCollectionID} {
		if spec.SurfaceList[index].SurfaceID != surfaceID {
			t.Fatalf("SurfaceList[%d].SurfaceID = %q, want %q", index, spec.SurfaceList[index].SurfaceID, surfaceID)
		}
	}
	for _, feature := range spec.FeatureList {
		if len(feature.RelatedSurfaceRefs) == 0 {
			t.Fatalf("feature %q related_surface_refs = %v, want surface-first refs", feature.FeatureID, feature.RelatedSurfaceRefs)
		}
		if len(feature.RelatedScreens) != 0 {
			t.Fatalf("feature %q related_screens = %v, want compatibility screens omitted", feature.FeatureID, feature.RelatedScreens)
		}
	}
	for _, flow := range spec.UserFlows {
		for _, step := range flow.Steps {
			if step.SurfaceRef == "" {
				t.Fatalf("user flow %q step %q surface_ref is empty, want surface-first projection", flow.FlowID, step.StepID)
			}
			if step.ScreenRef != "" {
				t.Fatalf("user flow %q step %q screen_ref = %q, want compatibility screen_ref omitted", flow.FlowID, step.StepID, step.ScreenRef)
			}
		}
	}
	taskIDs := make([]string, 0, len(spec.TaskBundle))
	for _, task := range spec.TaskBundle {
		taskIDs = append(taskIDs, task.TaskID)
	}
	if containsString(taskIDs, "task-screen-scaffold") {
		t.Fatalf("task ids = %v, want legacy task-screen-scaffold removed", taskIDs)
	}
	if !containsString(taskIDs, "task-bind-core-surfaces") {
		t.Fatalf("task ids = %v, want task-bind-core-surfaces kept", taskIDs)
	}
}

func TestCompileRelationRichSpecLayerUsesSurfaceFirstProjection(t *testing.T) {
	spec := compileRelationRichSpec(Request{}, "做一个项目任务协同 app，需要项目看板、任务列表、任务编辑和标签绑定，支持按项目和标签筛选任务。")
	if len(spec.SurfaceList) != 4 {
		t.Fatalf("SurfaceList len = %d, want 4", len(spec.SurfaceList))
	}
	if len(spec.TaskBundle) != 15 {
		t.Fatalf("TaskBundle len = %d, want 15", len(spec.TaskBundle))
	}
	assertUniqueTaskIDs(t, spec.TaskBundle)
	for _, feature := range spec.FeatureList {
		if len(feature.RelatedSurfaceRefs) == 0 {
			t.Fatalf("feature %q related_surface_refs = %v, want surface-first refs", feature.FeatureID, feature.RelatedSurfaceRefs)
		}
		if len(feature.RelatedScreens) != 0 {
			t.Fatalf("feature %q related_screens = %v, want compatibility screens omitted", feature.FeatureID, feature.RelatedScreens)
		}
	}
	for _, flow := range spec.UserFlows {
		for _, step := range flow.Steps {
			if step.SurfaceRef == "" {
				t.Fatalf("user flow %q step %q surface_ref is empty, want surface-first projection", flow.FlowID, step.StepID)
			}
			if step.ScreenRef != "" {
				t.Fatalf("user flow %q step %q screen_ref = %q, want compatibility screen_ref omitted", flow.FlowID, step.StepID, step.ScreenRef)
			}
		}
	}
	listControllerTask := findTaskBundleItem(t, spec.TaskBundle, "task-create-list-controller")
	if listControllerTask.AllocationTransition == nil {
		t.Fatal("list controller allocation_transition = nil, want surface-first relation-rich transition")
	}
	if len(listControllerTask.AllocationTransition.SurfaceRefs) == 0 {
		t.Fatalf("list controller surface_refs = %v, want surface-first refs", listControllerTask.AllocationTransition.SurfaceRefs)
	}
	if len(listControllerTask.AllocationTransition.ScreenRefs) != 0 {
		t.Fatalf("list controller screen_refs = %v, want compatibility refs omitted", listControllerTask.AllocationTransition.ScreenRefs)
	}
	if containsString(collectTaskIDs(spec.TaskBundle), "task-screen-scaffold") {
		t.Fatalf("task ids = %v, want legacy task-screen-scaffold removed", collectTaskIDs(spec.TaskBundle))
	}
}

func TestCompileInventorySheetLineItemSpecLayerUsesSurfaceFirstProjection(t *testing.T) {
	spec := compileInventorySheetLineItemSpec(Request{}, "做一个库存单协同 app，需要库存看板、库存单列表、库存单编辑和库存单详情，支持在库存单里维护多个明细项，并按仓库和低库存状态筛选。")
	if len(spec.SurfaceList) != 4 {
		t.Fatalf("SurfaceList len = %d, want 4", len(spec.SurfaceList))
	}
	if len(spec.TaskBundle) != 15 {
		t.Fatalf("TaskBundle len = %d, want 15", len(spec.TaskBundle))
	}
	assertUniqueTaskIDs(t, spec.TaskBundle)
	for _, feature := range spec.FeatureList {
		if len(feature.RelatedSurfaceRefs) == 0 {
			t.Fatalf("feature %q related_surface_refs = %v, want surface-first refs", feature.FeatureID, feature.RelatedSurfaceRefs)
		}
		if len(feature.RelatedScreens) != 0 {
			t.Fatalf("feature %q related_screens = %v, want compatibility screens omitted", feature.FeatureID, feature.RelatedScreens)
		}
	}
	for _, flow := range spec.UserFlows {
		for _, step := range flow.Steps {
			if step.SurfaceRef == "" {
				t.Fatalf("user flow %q step %q surface_ref is empty, want surface-first projection", flow.FlowID, step.StepID)
			}
			if step.ScreenRef != "" {
				t.Fatalf("user flow %q step %q screen_ref = %q, want compatibility screen_ref omitted", flow.FlowID, step.StepID, step.ScreenRef)
			}
		}
	}
	listControllerTask := findTaskBundleItem(t, spec.TaskBundle, "task-create-list-controller")
	if listControllerTask.AllocationTransition == nil {
		t.Fatal("list controller allocation_transition = nil, want surface-first inventory relation-rich transition")
	}
	if len(listControllerTask.AllocationTransition.SurfaceRefs) == 0 {
		t.Fatalf("list controller surface_refs = %v, want surface-first refs", listControllerTask.AllocationTransition.SurfaceRefs)
	}
	if len(listControllerTask.AllocationTransition.ScreenRefs) != 0 {
		t.Fatalf("list controller screen_refs = %v, want compatibility refs omitted", listControllerTask.AllocationTransition.ScreenRefs)
	}
}

func collectTaskIDs(tasks []appruns.TaskBundleItem) []string {
	ids := make([]string, 0, len(tasks))
	for _, task := range tasks {
		ids = append(ids, task.TaskID)
	}
	return ids
}

func assertUniqueTaskIDs(t *testing.T, tasks []appruns.TaskBundleItem) {
	t.Helper()
	seen := make(map[string]struct{}, len(tasks))
	for _, task := range tasks {
		if _, exists := seen[task.TaskID]; exists {
			t.Fatalf("task ids = %v, found duplicate %q", collectTaskIDs(tasks), task.TaskID)
		}
		seen[task.TaskID] = struct{}{}
	}
}

func TestCompileGenericSpecLayerKeepsTodoDomainSignals(t *testing.T) {
	spec := compileGenericSpec(Request{}, "做一个待办事项 app，需要首页摘要、新建待办、任务列表和详情页，优先保证本地可用。")
	if spec.Kind != "generic" {
		t.Fatalf("Kind = %q, want generic", spec.Kind)
	}
	if spec.Title != "待办事项 App" {
		t.Fatalf("Title = %q, want 待办事项 App", spec.Title)
	}
	if len(spec.TaskBundle) != 14 {
		t.Fatalf("TaskBundle len = %d, want 14", len(spec.TaskBundle))
	}
	if spec.TaskBundle[6].Category != appruns.TaskCategoryContent {
		t.Fatalf("domain copy category = %q, want content", spec.TaskBundle[6].Category)
	}
	if spec.TaskBundle[1].Category != appruns.TaskCategorySummary {
		t.Fatalf("summary task category = %q, want summary", spec.TaskBundle[1].Category)
	}
	if len(spec.RequirementHighlights) == 0 || !strings.Contains(strings.Join(spec.RequirementHighlights, " | "), "待整理") {
		t.Fatalf("RequirementHighlights = %v, want todo-specific highlights", spec.RequirementHighlights)
	}
}

func TestDefaultGenericTopologyPlanLayerFollowsDeclaredSignals(t *testing.T) {
	signals := genericDomainSignals{
		DomainLabel:            "轻量记录",
		CollectionFeatureTitle: "记录列表",
		ListAcceptance:         "用户可以浏览本地记录。",
		MutationFeatureTitle:   "记录录入",
		FormAcceptance:         "用户可以录入并保存记录。",
		Entity: DataEntity{
			EntityID: "entity-lite-record",
			Name:     "轻量记录",
			Fields: []DataField{{
				Name:     "record_id",
				Type:     "string",
				Required: true,
			}},
		},
	}

	plan := defaultGenericTopologyPlan(signals)
	if plan != (genericTopologyPlan{Collection: true, Mutation: true}) {
		t.Fatalf("defaultGenericTopologyPlan() = %+v, want collection+mutation only", plan)
	}

	tasks := buildGenericTaskBundle(signals)
	taskIDs := make([]string, 0, len(tasks))
	for _, task := range tasks {
		taskIDs = append(taskIDs, task.TaskID)
	}
	if containsString(taskIDs, taskBindOverviewSurfaceID) || containsString(taskIDs, taskBindInspectionSurfaceID) {
		t.Fatalf("task ids = %v, want overview/detail tasks pruned", taskIDs)
	}
	if !containsString(taskIDs, taskBindCollectionSurfaceID) || !containsString(taskIDs, taskBindMutationSurfaceID) {
		t.Fatalf("task ids = %v, want collection/mutation tasks kept", taskIDs)
	}
}

func TestCompileGenericSpecLayerPrunesExplicitNoHomeNoDetailTopology(t *testing.T) {
	spec := compileGenericSpec(Request{}, "做一个待办事项 app，无首页、无详情页，直接进入列表，通过抽屉录入新待办，不需要删除，也不需要筛选，优先保证本地可用。")
	if len(spec.SurfaceList) != 2 {
		t.Fatalf("SurfaceList len = %d, want 2", len(spec.SurfaceList))
	}
	surfaceIDs := make([]string, 0, len(spec.SurfaceList))
	for _, surface := range spec.SurfaceList {
		surfaceIDs = append(surfaceIDs, surface.SurfaceID)
	}
	if !containsString(surfaceIDs, genericSurfaceCollectionID) || !containsString(surfaceIDs, genericSurfaceMutationID) {
		t.Fatalf("surface ids = %v, want [%s %s]", surfaceIDs, genericSurfaceCollectionID, genericSurfaceMutationID)
	}
	if containsString(surfaceIDs, genericSurfaceOverviewID) || containsString(surfaceIDs, genericSurfaceInspectionID) {
		t.Fatalf("surface ids = %v, want overview/detail pruned", surfaceIDs)
	}
	if len(spec.DataEntities) != 1 {
		t.Fatalf("DataEntities len = %d, want 1", len(spec.DataEntities))
	}
	featureIDs := make([]string, 0, len(spec.FeatureList))
	for _, feature := range spec.FeatureList {
		featureIDs = append(featureIDs, feature.FeatureID)
	}
	if containsString(featureIDs, "feature-home-overview") || containsString(featureIDs, "feature-record-delete") {
		t.Fatalf("feature ids = %v, want overview/delete pruned", featureIDs)
	}
	if !containsString(featureIDs, "feature-record-list") || !containsString(featureIDs, "feature-record-form") {
		t.Fatalf("feature ids = %v, want list/form kept", featureIDs)
	}
	if len(spec.TaskBundle) != 10 {
		t.Fatalf("TaskBundle len = %d, want 10", len(spec.TaskBundle))
	}
	if len(spec.UserFlows) != 1 || spec.UserFlows[0].FlowID != "flow-create-record" {
		t.Fatalf("UserFlows = %+v, want only create flow", spec.UserFlows)
	}
	if len(spec.AcceptanceCriteria) != 4 {
		t.Fatalf("AcceptanceCriteria len = %d, want 4", len(spec.AcceptanceCriteria))
	}
	if len(spec.AcceptanceChecks) < 2 || len(spec.AcceptanceChecks[1].Commands) != 1 || spec.AcceptanceChecks[1].Commands[0] != "echo generic-open-lite-scope-collection-mutation" {
		t.Fatalf("AcceptanceChecks[1] = %+v, want collection-mutation scope check", spec.AcceptanceChecks[1])
	}
	if containsString(spec.TemplateConstraints.RequiredCapabilities, "summary-card") || containsString(spec.TemplateConstraints.RequiredCapabilities, "detail") {
		t.Fatalf("RequiredCapabilities = %v, want summary/detail pruned", spec.TemplateConstraints.RequiredCapabilities)
	}
	if !containsString(spec.TemplateConstraints.RequiredCapabilities, "list") || !containsString(spec.TemplateConstraints.RequiredCapabilities, "form") {
		t.Fatalf("RequiredCapabilities = %v, want list/form kept", spec.TemplateConstraints.RequiredCapabilities)
	}
	if len(spec.HumanNotes) != 1 {
		t.Fatalf("HumanNotes len = %d, want 1", len(spec.HumanNotes))
	}
	humanNotes := collectHumanNoteSummaries(spec.HumanNotes)
	if strings.Contains(humanNotes, "首页摘要") || strings.Contains(humanNotes, "状态分布") || strings.Contains(humanNotes, "详情") {
		t.Fatalf("HumanNotes = %q, want overview/detail hints pruned", humanNotes)
	}
	if !strings.Contains(humanNotes, "集合浏览和实体变更") {
		t.Fatalf("HumanNotes = %q, want collection/mutation scope note kept", humanNotes)
	}
}

func TestCompileGenericSpecLayerKeepsInspectionWithoutDelete(t *testing.T) {
	spec := compileGenericSpec(Request{}, "做一个体重记录 app，无首页，只读查看历史列表和详情页，不需要删除，优先保证本地可用。")
	if len(spec.SurfaceList) != 2 {
		t.Fatalf("SurfaceList len = %d, want 2", len(spec.SurfaceList))
	}
	surfaceIDs := make([]string, 0, len(spec.SurfaceList))
	for _, surface := range spec.SurfaceList {
		surfaceIDs = append(surfaceIDs, surface.SurfaceID)
	}
	if !containsString(surfaceIDs, genericSurfaceCollectionID) || !containsString(surfaceIDs, genericSurfaceInspectionID) {
		t.Fatalf("surface ids = %v, want [%s %s]", surfaceIDs, genericSurfaceCollectionID, genericSurfaceInspectionID)
	}
	if containsString(surfaceIDs, genericSurfaceOverviewID) || containsString(surfaceIDs, genericSurfaceMutationID) {
		t.Fatalf("surface ids = %v, want overview/mutation pruned", surfaceIDs)
	}
	featureIDs := make([]string, 0, len(spec.FeatureList))
	for _, feature := range spec.FeatureList {
		featureIDs = append(featureIDs, feature.FeatureID)
	}
	if containsString(featureIDs, "feature-record-delete") {
		t.Fatalf("feature ids = %v, want delete pruned", featureIDs)
	}
	if !containsString(featureIDs, "feature-record-list") {
		t.Fatalf("feature ids = %v, want list feature kept", featureIDs)
	}
	if len(spec.DataEntities) != 1 {
		t.Fatalf("DataEntities len = %d, want 1", len(spec.DataEntities))
	}
	if len(spec.TaskBundle) != 10 {
		t.Fatalf("TaskBundle len = %d, want 10", len(spec.TaskBundle))
	}
	if len(spec.UserFlows) != 1 || spec.UserFlows[0].FlowID != "flow-inspect-record" {
		t.Fatalf("UserFlows = %+v, want only inspect flow", spec.UserFlows)
	}
	if len(spec.AcceptanceChecks) < 2 || len(spec.AcceptanceChecks[1].Commands) != 1 || spec.AcceptanceChecks[1].Commands[0] != "echo generic-open-lite-scope-collection-inspection" {
		t.Fatalf("AcceptanceChecks[1] = %+v, want collection-inspection scope check", spec.AcceptanceChecks[1])
	}
	if containsString(spec.TemplateConstraints.RequiredCapabilities, "form") || containsString(spec.TemplateConstraints.RequiredCapabilities, "summary-card") {
		t.Fatalf("RequiredCapabilities = %v, want mutation/summary pruned", spec.TemplateConstraints.RequiredCapabilities)
	}
	if !containsString(spec.TemplateConstraints.RequiredCapabilities, "list") || !containsString(spec.TemplateConstraints.RequiredCapabilities, "detail") {
		t.Fatalf("RequiredCapabilities = %v, want list/detail kept", spec.TemplateConstraints.RequiredCapabilities)
	}
	if len(spec.HumanNotes) != 2 {
		t.Fatalf("HumanNotes len = %d, want 2", len(spec.HumanNotes))
	}
	humanNotes := collectHumanNoteSummaries(spec.HumanNotes)
	if strings.Contains(humanNotes, "首页摘要") || strings.Contains(humanNotes, "趋势") || strings.Contains(humanNotes, "删除") {
		t.Fatalf("HumanNotes = %q, want overview/delete hints pruned", humanNotes)
	}
	if !strings.Contains(humanNotes, "集合浏览和结果检查") {
		t.Fatalf("HumanNotes = %q, want collection/inspection scope note kept", humanNotes)
	}
}

func TestCompileGenericSpecLayerLetsRequirementOverlayOverrideProfileFallbacks(t *testing.T) {
	requirement := "# 需求\n\n## 需求摘录\n- 首页要突出今日待办\n- 列表需要按优先级查看\n"
	spec := compileGenericSpec(Request{TitleHint: "我的待办清单"}, requirement)
	if spec.Title != "我的待办清单" {
		t.Fatalf("Title = %q, want 我的待办清单", spec.Title)
	}
	if len(spec.RequirementHighlights) != 2 {
		t.Fatalf("RequirementHighlights len = %d, want 2", len(spec.RequirementHighlights))
	}
	if spec.RequirementHighlights[0] != "首页要突出今日待办" {
		t.Fatalf("RequirementHighlights[0] = %q, want 首页要突出今日待办", spec.RequirementHighlights[0])
	}
	if spec.RequirementHighlights[1] != "列表需要按优先级查看" {
		t.Fatalf("RequirementHighlights[1] = %q, want 列表需要按优先级查看", spec.RequirementHighlights[1])
	}
}

func TestDetectGenericDomainSignalsLayerKeepsWeightSemantics(t *testing.T) {
	signals := detectGenericDomainSignals("做一个体重记录 app，需要首页趋势、历史列表、详情页和本地记录。")
	if signals.AppTitle != "体重记录 App" {
		t.Fatalf("AppTitle = %q, want 体重记录 App", signals.AppTitle)
	}
	if signals.SummaryEntity.EntityID != "entity-weight-summary" {
		t.Fatalf("SummaryEntity.EntityID = %q, want entity-weight-summary", signals.SummaryEntity.EntityID)
	}
	if !strings.Contains(signals.DomainCheckPattern, "trend") {
		t.Fatalf("DomainCheckPattern = %q, want include trend", signals.DomainCheckPattern)
	}
	if len(signals.ImplementationPhases) != 3 {
		t.Fatalf("ImplementationPhases len = %d, want 3", len(signals.ImplementationPhases))
	}
}

func collectHumanNoteSummaries(notes []map[string]string) string {
	summaries := make([]string, 0, len(notes))
	for _, note := range notes {
		summary := strings.TrimSpace(note["summary"])
		if summary == "" {
			continue
		}
		summaries = append(summaries, summary)
	}
	return strings.Join(summaries, " | ")
}

func TestDefaultFlutterOpenLiteRealBuildChecksLayerIncludesDomainGuard(t *testing.T) {
	checks := defaultFlutterOpenLiteRealBuildChecks(genericDomainSignals{
		DomainCheckPattern: "体重|weight|trend",
		DomainCheckSummary: "生成代码中必须出现体重领域词。",
	}, genericTopologyPlan{Overview: true, Collection: true, Mutation: true, Inspection: true, Delete: true, Filter: true})
	if len(checks) < 5 {
		t.Fatalf("checks len = %d, want at least 5", len(checks))
	}
	var foundBranding bool
	var foundDomainLanguage bool
	for _, check := range checks {
		switch check.CheckID {
		case "check-profile-open-lite-domain-branding":
			foundBranding = true
		case "check-profile-open-lite-domain-language":
			foundDomainLanguage = true
			if len(check.Commands) != 1 || !strings.Contains(check.Commands[0], "体重|weight|trend") {
				t.Fatalf("domain language commands = %v, want include domain pattern", check.Commands)
			}
		}
	}
	if !foundBranding {
		t.Fatal("missing check-profile-open-lite-domain-branding")
	}
	if !foundDomainLanguage {
		t.Fatal("missing check-profile-open-lite-domain-language")
	}
}

func TestDefaultFlutterOpenLiteRealBuildChecksLayerPrunesNoHomeNoDetailTopology(t *testing.T) {
	checks := defaultFlutterOpenLiteRealBuildChecks(genericDomainSignals{}, genericTopologyPlan{Collection: true, Mutation: true})
	var counterCommand string
	var flowCommand string
	for _, check := range checks {
		switch check.CheckID {
		case "check-open-lite-counter-demo-removed":
			counterCommand = strings.Join(check.Commands, " ")
		case "check-open-lite-record-flow-wiring":
			flowCommand = strings.Join(check.Commands, " ")
		}
	}
	if counterCommand == "" || flowCommand == "" {
		t.Fatalf("checks = %+v, want counter-demo and flow-wiring checks", checks)
	}
	for _, forbidden := range []string{"lib/views/home_page.dart", "lib/controllers/home_controller.dart", "lib/views/record_detail_page.dart", "lib/models/dashboard_summary.dart"} {
		if strings.Contains(counterCommand, forbidden) {
			t.Fatalf("counter demo command = %q, want %s pruned", counterCommand, forbidden)
		}
	}
	if !strings.Contains(flowCommand, "lib/main.dart") || !strings.Contains(flowCommand, "RecordListPage") {
		t.Fatalf("flow command = %q, want RecordListPage wired through main", flowCommand)
	}
	for _, forbidden := range []string{"RecordDetailPage", "deleteRecord", "lib/views/record_detail_page.dart", "lib/controllers/home_controller.dart"} {
		if strings.Contains(flowCommand, forbidden) {
			t.Fatalf("flow command = %q, want %s pruned for no-home/no-detail topology", flowCommand, forbidden)
		}
	}
	if !strings.Contains(flowCommand, "lib/views/record_form_page.dart") || !strings.Contains(flowCommand, "lib/controllers/record_form_controller.dart") {
		t.Fatalf("flow command = %q, want form wiring kept", flowCommand)
	}
}

func TestDefaultFlutterOpenLiteRealBuildChecksLayerKeepsInspectionWithoutDelete(t *testing.T) {
	checks := defaultFlutterOpenLiteRealBuildChecks(genericDomainSignals{}, genericTopologyPlan{Collection: true, Inspection: true})
	var flowCommand string
	for _, check := range checks {
		if check.CheckID == "check-open-lite-record-flow-wiring" {
			flowCommand = strings.Join(check.Commands, " ")
			break
		}
	}
	if flowCommand == "" {
		t.Fatalf("checks = %+v, want flow-wiring check", checks)
	}
	if !strings.Contains(flowCommand, "RecordDetailPage") {
		t.Fatalf("flow command = %q, want detail wiring kept", flowCommand)
	}
	for _, forbidden := range []string{"deleteRecord", "lib/views/record_form_page.dart", "lib/controllers/record_form_controller.dart"} {
		if strings.Contains(flowCommand, forbidden) {
			t.Fatalf("flow command = %q, want %s pruned when mutation/delete are absent", flowCommand, forbidden)
		}
	}
}

func TestBuildTaskAllocationLayerUsesGenericSurfaceFallbackForUndeclaredViewTargets(t *testing.T) {
	tasks := []appruns.TaskBundleItem{
		{
			TaskID:             "task-domain",
			Title:              "沉淀领域模型",
			Category:           appruns.TaskCategoryDomain,
			Objective:          "生成记录模型",
			TargetPaths:        []string{"lib/models/record.dart"},
			CompletionCriteria: []string{"记录实体字段可用"},
			AllocationTransition: &appruns.TaskAllocationTransition{
				AllocationID:       "alloc-domain",
				SemanticIntentRefs: []string{"req-domain"},
				OwnedPaths:         []string{"lib/models/record.dart"},
				SuccessEvidence:    []string{"领域对象字段已冻结"},
			},
		},
		{
			TaskID:             "task-home",
			Title:              "接线首页",
			Category:           appruns.TaskCategoryScreen,
			Objective:          "展示首页摘要",
			Dependencies:       []string{"alloc-domain"},
			TargetPaths:        []string{"lib/views/home_page.dart", "lib/controllers/home_controller.dart"},
			RouteHint:          appruns.TaskRouteHintStrongModel,
			RiskLevel:          appruns.TaskRiskLevelMedium,
			CompletionCriteria: []string{"首页摘要已接入"},
			AllocationTransition: &appruns.TaskAllocationTransition{
				AllocationID:    "alloc-home",
				OwnedPaths:      []string{"lib/views/home_page.dart", "lib/controllers/home_controller.dart"},
				BlockedBy:       []string{"alloc-domain"},
				SuccessEvidence: []string{"首页显示领域摘要"},
			},
		},
	}

	allocation := buildTaskAllocation(tasks, domainSpec{JobID: "job-layer", PRDID: "prd-layer", TemplateID: "flutter-open-lite"})
	if len(allocation.Units) != 2 {
		t.Fatalf("Units len = %d, want 2", len(allocation.Units))
	}
	if allocation.Units[0].AllocationID != "alloc-domain" || allocation.Units[0].Wave != 0 {
		t.Fatalf("first unit = %+v, want alloc-domain wave 0", allocation.Units[0])
	}
	if allocation.Units[1].AllocationID != "alloc-home" || allocation.Units[1].Wave != 1 {
		t.Fatalf("second unit = %+v, want alloc-home wave 1", allocation.Units[1])
	}
	joined := strings.Join(allocation.Units[1].BindingRefs, ",")
	if !strings.Contains(joined, publicBindingSurface) {
		t.Fatalf("home unit binding refs = %v, want include %s for undeclared view target", allocation.Units[1].BindingRefs, publicBindingSurface)
	}
	if strings.Contains(joined, genericSurfaceOverviewID) {
		t.Fatalf("home unit binding refs = %v, want no implicit %s without explicit surface_refs", allocation.Units[1].BindingRefs, genericSurfaceOverviewID)
	}
	if allocation.Units[1].RouteHint != appruns.TaskRouteHintStrongModel {
		t.Fatalf("home unit route_hint = %q, want strong_model", allocation.Units[1].RouteHint)
	}
}

func TestBuildTaskAllocationLayerPrefersDeclaredSurfaceRefsForViewTargets(t *testing.T) {
	tasks := []appruns.TaskBundleItem{{
		TaskID:             "task-home",
		Title:              "接线首页",
		Category:           appruns.TaskCategoryScreen,
		Objective:          "展示首页摘要",
		TargetPaths:        []string{"lib/views/home_page.dart", "lib/controllers/home_controller.dart"},
		CompletionCriteria: []string{"首页摘要已接入"},
		AllocationTransition: &appruns.TaskAllocationTransition{
			AllocationID: "alloc-home",
			SurfaceRefs:  []string{genericSurfaceOverviewID},
		},
	}}

	allocation := buildTaskAllocation(tasks, domainSpec{JobID: "job-layer", PRDID: "prd-layer", TemplateID: "flutter-open-lite"})
	if len(allocation.Units) != 1 {
		t.Fatalf("Units len = %d, want 1", len(allocation.Units))
	}
	joined := strings.Join(allocation.Units[0].BindingRefs, ",")
	if !strings.Contains(joined, genericSurfaceOverviewID) {
		t.Fatalf("declared view binding refs = %v, want include %s", allocation.Units[0].BindingRefs, genericSurfaceOverviewID)
	}
	if strings.Contains(joined, publicBindingSurface) {
		t.Fatalf("declared view binding refs = %v, want no fallback %s when surface_refs exist", allocation.Units[0].BindingRefs, publicBindingSurface)
	}
}

func TestBuildTaskAllocationLayerUsesDeclaredBindingRefsWithoutSemanticIntentFallback(t *testing.T) {
	tasks := []appruns.TaskBundleItem{{
		TaskID:      "task-copy",
		Title:       "同步领域文案",
		Category:    appruns.TaskCategoryContent,
		Objective:   "创建 copy 投影",
		TargetPaths: []string{"lib/template/open_lite_copy.dart"},
		AllocationTransition: &appruns.TaskAllocationTransition{
			AllocationID:       "alloc-copy",
			SemanticIntentRefs: []string{"mrp-copy"},
			BindingRefs:        []string{publicBindingDomainCopy},
		},
	}}

	allocation := buildTaskAllocation(tasks, domainSpec{JobID: "job-layer", PRDID: "prd-layer", TemplateID: "flutter-open-lite"})
	if len(allocation.Units) != 1 {
		t.Fatalf("Units len = %d, want 1", len(allocation.Units))
	}
	if !reflect.DeepEqual(allocation.Units[0].BindingRefs, []string{publicBindingDomainCopy}) {
		t.Fatalf("declared binding refs = %v, want [%s]", allocation.Units[0].BindingRefs, publicBindingDomainCopy)
	}
}

func TestBindingRefsForTargetPathUsesTemplateCopyDirectory(t *testing.T) {
	refs := bindingRefsForTargetPath("lib/template/domain_copy.dart", nil)
	if !reflect.DeepEqual(refs, []string{publicBindingDomainCopy}) {
		t.Fatalf("bindingRefsForTargetPath(domain_copy.dart) = %v, want [%s]", refs, publicBindingDomainCopy)
	}
}

func TestBuildAcceptancePlanLayer(t *testing.T) {
	spec := domainSpec{
		JobID:      "job-layer",
		PRDID:      "prd-layer",
		TemplateID: "flutter-open-lite",
		Title:      "待办事项 App",
		DataEntities: []DataEntity{
			{EntityID: "entity-todo-item", Name: "待办事项", Source: "local_storage", Fields: []DataField{{Name: "task_id", Type: "string", Required: true}, {Name: "title", Type: "string", Required: true}, {Name: "status", Type: "string", Required: true}}},
			{EntityID: "entity-todo-summary", Name: "首页待办摘要", Source: "derived", Fields: []DataField{{Name: "inbox_count", Type: "int", Required: true}, {Name: "done_count", Type: "int", Required: true}}},
		},
		AcceptanceCriteria: []AcceptanceCriterion{
			{CriterionID: "ac-home", Label: "首页摘要", Required: true, Description: "首页需要显示摘要卡片"},
		},
		UserFlows: []UserFlow{
			{FlowID: "flow-create", Title: "创建记录", Steps: []FlowStep{{Title: "填写表单"}, {Title: "保存记录"}}},
		},
		AcceptanceChecks: []appruns.AcceptanceCheck{
			{CheckID: "check-structure", Label: "结构检查", Stage: appruns.StageCheap, Required: true, Commands: []string{"flutter analyze"}, SuccessCriteria: "结构通过"},
			{CheckID: "check-device-apk", Label: "设备安装", Stage: appruns.StageDevice, Required: true, Commands: []string{"adb install app.apk"}, SuccessCriteria: "apk 可安装到设备"},
		},
	}

	plan := buildAcceptancePlan(spec)
	if len(plan.SemanticChecks) != 1 {
		t.Fatalf("SemanticChecks len = %d, want 1", len(plan.SemanticChecks))
	}
	if plan.SemanticChecks[0].SourceType != "semantic_acceptance_rule" {
		t.Fatalf("SemanticChecks[0].SourceType = %q, want semantic_acceptance_rule", plan.SemanticChecks[0].SourceType)
	}
	if plan.SemanticChecks[0].Stage != appruns.StageCheap {
		t.Fatalf("SemanticChecks[0].Stage = %q, want cheap", plan.SemanticChecks[0].Stage)
	}
	if len(plan.SemanticChecks[0].Commands) != 1 || !strings.Contains(plan.SemanticChecks[0].Commands[0], "grep -ER") {
		t.Fatalf("SemanticChecks[0].Commands = %v, want grep-based semantic evidence check", plan.SemanticChecks[0].Commands)
	}
	if len(plan.SemanticChecks[0].FieldRefs) == 0 {
		t.Fatalf("SemanticChecks[0].FieldRefs = %v, want non-empty semantic field refs", plan.SemanticChecks[0].FieldRefs)
	}
	if len(plan.BehaviorChecks) != 1 {
		t.Fatalf("BehaviorChecks len = %d, want 1", len(plan.BehaviorChecks))
	}
	if len(plan.StructureChecks) != 1 {
		t.Fatalf("StructureChecks len = %d, want 1", len(plan.StructureChecks))
	}
	if len(plan.DeliveryChecks) != 1 {
		t.Fatalf("DeliveryChecks len = %d, want 1", len(plan.DeliveryChecks))
	}
	if plan.BehaviorChecks[0].Description != "填写表单 -> 保存记录" {
		t.Fatalf("BehaviorChecks[0].Description = %q, want flow summary", plan.BehaviorChecks[0].Description)
	}
	if plan.DeliveryChecks[0].CheckID != "check-device-apk" {
		t.Fatalf("DeliveryChecks[0].CheckID = %q, want check-device-apk", plan.DeliveryChecks[0].CheckID)
	}
}

func TestRenderImplementationPlanLayer(t *testing.T) {
	spec := domainSpec{
		JobID:                "job-render",
		PRDID:                "prd-render",
		ImplementationPhases: []string{"阶段一：建模", "阶段二：接线"},
		TaskBundle: []appruns.TaskBundleItem{
			{
				Title:              "首页摘要",
				Category:           appruns.TaskCategoryScreen,
				Objective:          "接入首页摘要卡片",
				TargetPaths:        []string{"lib/views/home_page.dart"},
				CompletionCriteria: []string{"首页展示摘要"},
				RiskNotes:          []string{"注意同步文案"},
			},
		},
		AcceptanceChecks: []appruns.AcceptanceCheck{{Label: "结构检查", Stage: appruns.StageCheap, Commands: []string{"flutter analyze"}}},
	}

	plan := renderImplementationPlan(spec)
	for _, expected := range []string{"# 实施计划", "阶段一：建模", "### 首页摘要", "- 风险提示：注意同步文案", "结构检查：stage=cheap"} {
		if !strings.Contains(plan, expected) {
			t.Fatalf("rendered implementation plan missing %q:\n%s", expected, plan)
		}
	}
}
