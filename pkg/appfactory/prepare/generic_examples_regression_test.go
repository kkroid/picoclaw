package prepare

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	appruns "github.com/sipeed/oneappfactory/pkg/appfactory/runs"
)

type genericExampleExpectation struct {
	Name                    string
	DomainName              string
	PrimaryEntityID         string
	PrimaryFieldNames       []string
	SummaryEntityID         string
	SummaryFieldNames       []string
	RequirementHighlights   []string
	HumanNoteCount          int
	OverviewPattern         string
	ListFieldRefs           []string
	BehaviorCheckCount      int
	ListControllerTitle     string
	ListControllerObjective string
	ListControllerEvidence  string
}

func TestCompileGenericExamplesExposePlanningArtifacts(t *testing.T) {
	t.Parallel()

	cases := []genericExampleExpectation{
		{
			Name:                    "weight-tracker",
			DomainName:              "体重记录 App",
			PrimaryEntityID:         "entity-weight-record",
			PrimaryFieldNames:       []string{"record_id", "weight", "recorded_at", "note"},
			SummaryEntityID:         "entity-weight-summary",
			SummaryFieldNames:       []string{"latest_weight", "record_count", "trend"},
			RequirementHighlights:   []string{"目标是离线使用的个人健康记录工具。", "需要首页摘要区展示最近趋势和记录数量。", "需要表单页录入体重、日期和备注。", "需要列表页和详情页查看历史记录。"},
			HumanNoteCount:          2,
			OverviewPattern:         "体重记录 App|latest_weight|latestWeight|record_count|recordCount|trend",
			ListFieldRefs:           []string{"record_id", "weight", "recorded_at", "note"},
			BehaviorCheckCount:      4,
			ListControllerTitle:     "创建集合浏览控制器",
			ListControllerObjective: "围绕领域记录提供集合读取、排序和结果检查跳转所需的控制器接口。",
			ListControllerEvidence:  "结果检查入口与集合数据保持一致",
		},
		{
			Name:                    "todo-lite",
			DomainName:              "待办事项 App",
			PrimaryEntityID:         "entity-todo-item",
			PrimaryFieldNames:       []string{"task_id", "title", "category", "status", "note"},
			SummaryEntityID:         "entity-todo-summary",
			SummaryFieldNames:       []string{"inbox_count", "in_progress_count", "done_count"},
			RequirementHighlights:   []string{"首页需要看到待整理、进行中、已完成数量。", "表单页需要录入标题、分类、状态和备注。", "列表页需要快速浏览全部待办。", "详情页需要查看单条任务的完整说明。"},
			HumanNoteCount:          1,
			OverviewPattern:         "待办事项 App|inbox_count|inboxCount|in_progress_count|inProgressCount|done_count|doneCount",
			ListFieldRefs:           []string{"task_id", "title", "category", "status", "note"},
			BehaviorCheckCount:      5,
			ListControllerTitle:     "创建集合浏览与筛选控制器",
			ListControllerObjective: "围绕领域记录提供集合读取、排序、筛选和结果检查跳转所需的控制器接口。",
			ListControllerEvidence:  "集合浏览入口可按领域状态筛选记录",
		},
		{
			Name:                    "habit-checkin",
			DomainName:              "习惯打卡 App",
			PrimaryEntityID:         "entity-habit-record",
			PrimaryFieldNames:       []string{"habit_record_id", "title", "category", "status", "checked_at", "note"},
			SummaryEntityID:         "entity-habit-summary",
			SummaryFieldNames:       []string{"total_count", "pending_count", "done_count"},
			RequirementHighlights:   []string{"首页摘要区需要能展示总记录数和完成状态分布。", "表单页需要录入习惯标题、分类、状态和备注。", "列表页需要按时间查看最近打卡记录。", "详情页需要查看单条打卡记录的完整信息。"},
			HumanNoteCount:          1,
			OverviewPattern:         "习惯打卡 App|total_count|totalCount|pending_count|pendingCount|done_count|doneCount",
			ListFieldRefs:           []string{"habit_record_id", "title", "category", "status", "checked_at", "note"},
			BehaviorCheckCount:      5,
			ListControllerTitle:     "创建集合浏览与筛选控制器",
			ListControllerObjective: "围绕领域记录提供集合读取、排序、筛选和结果检查跳转所需的控制器接口。",
			ListControllerEvidence:  "集合浏览入口可按领域状态筛选记录",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.Name, func(t *testing.T) {
			t.Parallel()

			bundle := compileGenericExampleBundle(t, tc.Name)

			var planningContext PlanningContext
			mustDecodeJSON(t, bundle.Files[planningContextFileName], &planningContext)
			if planningContext.TemplateID != "flutter-open-lite" {
				t.Fatalf("PlanningContext.TemplateID = %q, want flutter-open-lite", planningContext.TemplateID)
			}
			if len(planningContext.RequirementHighlights) != len(tc.RequirementHighlights) {
				t.Fatalf("RequirementHighlights len = %d, want %d", len(planningContext.RequirementHighlights), len(tc.RequirementHighlights))
			}
			if !reflect.DeepEqual(planningContext.RequirementHighlights, tc.RequirementHighlights) {
				t.Fatalf("RequirementHighlights = %v, want %v", planningContext.RequirementHighlights, tc.RequirementHighlights)
			}
			if len(planningContext.HumanNotes) != tc.HumanNoteCount {
				t.Fatalf("HumanNotes len = %d, want %d", len(planningContext.HumanNotes), tc.HumanNoteCount)
			}
			if got := planningContext.ExecutionRouteSnapshot.RouteHintCounts[string(appruns.TaskRouteHintDefaultModel)]; got != 0 {
				t.Fatalf("default_model route count = %d, want 0", got)
			}
			if got := planningContext.ExecutionRouteSnapshot.RouteHintCounts[string(appruns.TaskRouteHintStrongModel)]; got != 0 {
				t.Fatalf("strong_model route count = %d, want 0", got)
			}
			if got := planningContext.ExecutionRouteSnapshot.RouteHintCounts[string(appruns.TaskRouteHintDeterministic)]; got != len(planningContext.ExecutionRouteSnapshot.TaskRoutes) {
				t.Fatalf("deterministic route count = %d, want task route count", got)
			}
			if planningContext.ExecutionRouteSnapshot.StartUpgradeCheck.ConcreteTargetFileCount != len(planningContext.ExecutionRouteSnapshot.TaskRoutes) {
				t.Fatalf("ConcreteTargetFileCount = %d, want task route count %d", planningContext.ExecutionRouteSnapshot.StartUpgradeCheck.ConcreteTargetFileCount, len(planningContext.ExecutionRouteSnapshot.TaskRoutes))
			}

			var domainModel DomainModel
			mustDecodeJSON(t, bundle.Files[domainModelFileName], &domainModel)
			if domainModel.DomainName != tc.DomainName {
				t.Fatalf("DomainName = %q, want %q", domainModel.DomainName, tc.DomainName)
			}
			if len(domainModel.Entities) != 2 {
				t.Fatalf("Entities len = %d, want 2", len(domainModel.Entities))
			}
			if domainModel.Entities[0].EntityID != tc.PrimaryEntityID {
				t.Fatalf("primary entity id = %q, want %q", domainModel.Entities[0].EntityID, tc.PrimaryEntityID)
			}
			if !reflect.DeepEqual(fieldNames(domainModel.Entities[0].Fields), tc.PrimaryFieldNames) {
				t.Fatalf("primary fields = %v, want %v", fieldNames(domainModel.Entities[0].Fields), tc.PrimaryFieldNames)
			}
			if domainModel.Entities[1].EntityID != tc.SummaryEntityID {
				t.Fatalf("summary entity id = %q, want %q", domainModel.Entities[1].EntityID, tc.SummaryEntityID)
			}
			if !reflect.DeepEqual(fieldNames(domainModel.Entities[1].Fields), tc.SummaryFieldNames) {
				t.Fatalf("summary fields = %v, want %v", fieldNames(domainModel.Entities[1].Fields), tc.SummaryFieldNames)
			}
			if len(domainModel.SemanticAcceptanceRules) != 7 {
				t.Fatalf("SemanticAcceptanceRules len = %d, want 7", len(domainModel.SemanticAcceptanceRules))
			}
			if domainModel.SemanticAcceptanceRules[0].EvidencePattern != tc.OverviewPattern {
				t.Fatalf("overview evidence pattern = %q, want %q", domainModel.SemanticAcceptanceRules[0].EvidencePattern, tc.OverviewPattern)
			}
			if bundle.PRD.ExecutionContract == nil {
				t.Fatal("PRD.ExecutionContract = nil, want execution contract")
			}
			if bundle.PRD.ExecutionContract.ContractVersion != defaultSchemaVersion {
				t.Fatalf("ExecutionContract.ContractVersion = %q, want %q", bundle.PRD.ExecutionContract.ContractVersion, defaultSchemaVersion)
			}
			if bundle.PRD.ExecutionContract.DomainModel.DomainName != tc.DomainName {
				t.Fatalf("ExecutionContract.DomainModel.DomainName = %q, want %q", bundle.PRD.ExecutionContract.DomainModel.DomainName, tc.DomainName)
			}
			assertSurfaceRelationSchema(t, bundle.PRD.ExecutionContract)
			if !reflect.DeepEqual(fieldNames(bundle.PRD.ExecutionContract.DomainModel.Entities[0].Fields), tc.PrimaryFieldNames) {
				t.Fatalf("execution contract primary fields = %v, want %v", fieldNames(bundle.PRD.ExecutionContract.DomainModel.Entities[0].Fields), tc.PrimaryFieldNames)
			}
			if len(bundle.PRD.ExecutionContract.SurfaceContracts) != len(bundle.PRD.SurfaceList) {
				t.Fatalf("ExecutionContract.SurfaceContracts len = %d, want %d", len(bundle.PRD.ExecutionContract.SurfaceContracts), len(bundle.PRD.SurfaceList))
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
						t.Fatalf("user flow %q step %q surface_ref is empty, want surface-first flow projection", flow.FlowID, step.StepID)
					}
					if step.ScreenRef != "" {
						t.Fatalf("user flow %q step %q screen_ref = %q, want compatibility screen_ref omitted", flow.FlowID, step.StepID, step.ScreenRef)
					}
				}
			}
			for _, want := range []string{genericSurfaceOverviewID, genericSurfaceCollectionID, genericSurfaceMutationID, genericSurfaceInspectionID} {
				found := false
				for _, contract := range bundle.PRD.ExecutionContract.SurfaceContracts {
					if contract.SurfaceRef != want {
						continue
					}
					found = true
					if contract.TemplateBindingRef != want {
						t.Fatalf("surface %q template_binding_ref = %q, want %q", contract.SurfaceRef, contract.TemplateBindingRef, want)
					}
					if len(contract.RequiredStates) == 0 {
						t.Fatalf("surface %q required_states = %v, want non-empty action/state contract", contract.SurfaceRef, contract.RequiredStates)
					}
					break
				}
				if !found {
					t.Fatalf("missing surface contract for %q: %+v", want, bundle.PRD.ExecutionContract.SurfaceContracts)
				}
			}
			if len(bundle.PRD.ExecutionContract.KeyFlows) != len(bundle.PRD.UserFlows) {
				t.Fatalf("ExecutionContract.KeyFlows len = %d, want %d", len(bundle.PRD.ExecutionContract.KeyFlows), len(bundle.PRD.UserFlows))
			}
			contractJSON, err := json.Marshal(bundle.PRD.ExecutionContract)
			if err != nil {
				t.Fatalf("Marshal(ExecutionContract) error = %v", err)
			}
			if bytes.Contains(contractJSON, []byte(`"entry_screen_ref"`)) {
				t.Fatalf("execution contract JSON = %s, want entry_screen_ref omitted", string(contractJSON))
			}
			if bytes.Contains(contractJSON, []byte(`"screen_ref"`)) {
				t.Fatalf("execution contract JSON = %s, want screen_ref omitted", string(contractJSON))
			}
			if bytes.Contains(contractJSON, []byte(`"screen_refs"`)) {
				t.Fatalf("execution contract JSON = %s, want screen_refs omitted", string(contractJSON))
			}
			for _, flow := range bundle.PRD.ExecutionContract.KeyFlows {
				if flow.EntrySurfaceRef == "" {
					t.Fatalf("key flow %q entry_surface_ref is empty, want surface-first execution contract", flow.FlowID)
				}
				if len(flow.AcceptanceRefs) == 0 {
					t.Fatalf("key flow %q acceptance_refs = %v, want linked execution acceptance refs", flow.FlowID, flow.AcceptanceRefs)
				}
				for _, step := range flow.Steps {
					if step.SurfaceRef == "" {
						t.Fatalf("key flow %q step %q surface_ref is empty, want surface-first projection", flow.FlowID, step.StepID)
					}
				}
			}
			if len(bundle.PRD.ExecutionContract.TaskProjection) != len(bundle.BuilderInput.TaskBundle) {
				t.Fatalf("ExecutionContract.TaskProjection len = %d, want %d", len(bundle.PRD.ExecutionContract.TaskProjection), len(bundle.BuilderInput.TaskBundle))
			}

			var taskAllocation TaskAllocation
			mustDecodeJSON(t, bundle.Files[taskAllocationFileName], &taskAllocation)
			if len(taskAllocation.Units) != len(bundle.BuilderInput.TaskBundle) {
				t.Fatalf("TaskAllocation.Units len = %d, want %d", len(taskAllocation.Units), len(bundle.BuilderInput.TaskBundle))
			}
			listControllerUnit := findAllocationUnit(t, taskAllocation.Units, "task-create-list-controller")
			if listControllerUnit.Lane != "flow" {
				t.Fatalf("list controller unit = %+v, want lane flow", listControllerUnit)
			}
			if !reflect.DeepEqual(listControllerUnit.SurfaceRefs, []string{genericSurfaceCollectionID, genericSurfaceInspectionID}) {
				t.Fatalf("list controller surface_refs = %v, want [%s %s]", listControllerUnit.SurfaceRefs, genericSurfaceCollectionID, genericSurfaceInspectionID)
			}
			if len(listControllerUnit.ScreenRefs) != 0 {
				t.Fatalf("list controller screen_refs = %v, want omitted compatibility refs", listControllerUnit.ScreenRefs)
			}
			if !reflect.DeepEqual(listControllerUnit.EntityRefs, []string{tc.PrimaryEntityID}) {
				t.Fatalf("list controller entity_refs = %v, want [%s]", listControllerUnit.EntityRefs, tc.PrimaryEntityID)
			}
			if listControllerUnit.Objective != tc.ListControllerObjective {
				t.Fatalf("list controller objective = %q, want %q", listControllerUnit.Objective, tc.ListControllerObjective)
			}
			if !containsString(listControllerUnit.SuccessEvidence, tc.ListControllerEvidence) {
				t.Fatalf("list controller success evidence = %v, want contain %q", listControllerUnit.SuccessEvidence, tc.ListControllerEvidence)
			}
			if !containsString(listControllerUnit.BindingRefs, publicBindingFlow) {
				t.Fatalf("list controller binding refs = %v, want include %s", listControllerUnit.BindingRefs, publicBindingFlow)
			}
			listControllerTask := findTaskBundleItem(t, bundle.BuilderInput.TaskBundle, "task-create-list-controller")
			if listControllerTask.Title != tc.ListControllerTitle {
				t.Fatalf("list controller task title = %q, want %q", listControllerTask.Title, tc.ListControllerTitle)
			}
			if listControllerTask.AllocationTransition == nil {
				t.Fatal("list controller allocation_transition = nil, want simple-domain refs")
			}
			if !reflect.DeepEqual(listControllerTask.AllocationTransition.SurfaceRefs, []string{genericSurfaceCollectionID, genericSurfaceInspectionID}) {
				t.Fatalf("task bundle surface_refs = %v, want [%s %s]", listControllerTask.AllocationTransition.SurfaceRefs, genericSurfaceCollectionID, genericSurfaceInspectionID)
			}
			if len(listControllerTask.AllocationTransition.ScreenRefs) != 0 {
				t.Fatalf("task bundle screen_refs = %v, want omitted compatibility refs", listControllerTask.AllocationTransition.ScreenRefs)
			}
			if !reflect.DeepEqual(listControllerTask.AllocationTransition.EntityRefs, []string{tc.PrimaryEntityID}) {
				t.Fatalf("task bundle entity_refs = %v, want [%s]", listControllerTask.AllocationTransition.EntityRefs, tc.PrimaryEntityID)
			}
			projection := mustFindTaskProjectionItem(t, bundle.PRD.ExecutionContract.TaskProjection, "task-create-list-controller")
			if !reflect.DeepEqual(projection.SurfaceRefs, []string{genericSurfaceCollectionID, genericSurfaceInspectionID}) {
				t.Fatalf("task projection surface_refs = %v, want [%s %s]", projection.SurfaceRefs, genericSurfaceCollectionID, genericSurfaceInspectionID)
			}
			if !reflect.DeepEqual(projection.EntityRefs, []string{tc.PrimaryEntityID}) {
				t.Fatalf("task projection entity_refs = %v, want [%s]", projection.EntityRefs, tc.PrimaryEntityID)
			}

			var acceptancePlan AcceptancePlan
			mustDecodeJSON(t, bundle.Files[acceptancePlanFileName], &acceptancePlan)
			if len(acceptancePlan.StructureChecks) != 6 {
				t.Fatalf("StructureChecks len = %d, want 6", len(acceptancePlan.StructureChecks))
			}
			if len(acceptancePlan.SemanticChecks) != 10 {
				t.Fatalf("SemanticChecks len = %d, want 10", len(acceptancePlan.SemanticChecks))
			}
			if len(acceptancePlan.BehaviorChecks) != tc.BehaviorCheckCount {
				t.Fatalf("BehaviorChecks len = %d, want %d", len(acceptancePlan.BehaviorChecks), tc.BehaviorCheckCount)
			}
			if len(acceptancePlan.DeliveryChecks) != 0 {
				t.Fatalf("DeliveryChecks len = %d, want 0", len(acceptancePlan.DeliveryChecks))
			}
			overviewCheck := mustFindAcceptancePlanItem(t, acceptancePlan.SemanticChecks, "ac-overview")
			if overviewCheck.EvidencePattern != tc.OverviewPattern {
				t.Fatalf("overview evidence pattern = %q, want %q", overviewCheck.EvidencePattern, tc.OverviewPattern)
			}
			if !reflect.DeepEqual(overviewCheck.BindingRefs, []string{genericSurfaceOverviewID}) {
				t.Fatalf("overview binding refs = %v, want [%s]", overviewCheck.BindingRefs, genericSurfaceOverviewID)
			}
			listCheck := mustFindAcceptancePlanItem(t, acceptancePlan.SemanticChecks, "ac-list")
			if !reflect.DeepEqual(listCheck.FieldRefs, tc.ListFieldRefs) {
				t.Fatalf("list field refs = %v, want %v", listCheck.FieldRefs, tc.ListFieldRefs)
			}
			manualReview := mustFindAcceptancePlanItem(t, acceptancePlan.SemanticChecks, "mrp-domain-wording")
			manualReviewJoined := strings.Join(manualReview.BindingRefs, ",")
			for _, expected := range []string{publicBindingDomainCopy, publicBindingBranding, genericSurfaceOverviewID, genericSurfaceMutationID, genericSurfaceInspectionID} {
				if !strings.Contains(manualReviewJoined, expected) {
					t.Fatalf("manual review binding refs = %v, want include %s from declared task bindings", manualReview.BindingRefs, expected)
				}
			}
			if manualReviewJoined == publicBindingDomainCopy {
				t.Fatalf("manual review binding refs = %v, want declared task bindings instead of point_id keyword narrowing", manualReview.BindingRefs)
			}
			wantMatrixCount := len(acceptancePlan.StructureChecks) + len(acceptancePlan.SemanticChecks) + len(acceptancePlan.BehaviorChecks) + len(acceptancePlan.DeliveryChecks)
			if len(bundle.PRD.ExecutionContract.AcceptanceMatrix) != wantMatrixCount {
				t.Fatalf("ExecutionContract.AcceptanceMatrix len = %d, want %d", len(bundle.PRD.ExecutionContract.AcceptanceMatrix), wantMatrixCount)
			}
		})
	}
}

func TestCompileGenericExamplesBuilderInputGolden(t *testing.T) {
	t.Parallel()

	for _, name := range []string{"weight-tracker", "todo-lite", "habit-checkin"} {
		name := name
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			bundle := compileGenericExampleBundle(t, name)
			got, err := normalizeBuildInputForGolden(bundle.BuilderInput)
			if err != nil {
				t.Fatalf("normalizeBuildInputForGolden() error = %v", err)
			}
			goldenPath := filepath.Join("testdata", "generic_examples", name+".builder-input.golden.json")
			if os.Getenv("ONEAPPFACTORY_UPDATE_GENERIC_GOLDENS") == "1" {
				if err := os.WriteFile(goldenPath, got, 0o644); err != nil {
					t.Fatalf("WriteFile(%s) error = %v", goldenPath, err)
				}
			}
			want, err := os.ReadFile(goldenPath)
			if err != nil {
				t.Fatalf("ReadFile(%s) error = %v", goldenPath, err)
			}
			want = normalizeJSONForGolden(want)
			var gotDecoded any
			if err := json.Unmarshal(got, &gotDecoded); err != nil {
				t.Fatalf("Unmarshal(got) error = %v", err)
			}
			var wantDecoded any
			if err := json.Unmarshal(want, &wantDecoded); err != nil {
				t.Fatalf("Unmarshal(want) error = %v", err)
			}
			if !reflect.DeepEqual(gotDecoded, wantDecoded) {
				t.Fatalf("builder input golden mismatch\nwant:\n%s\n\ngot:\n%s", want, got)
			}
		})
	}
}

func compileGenericExampleBundle(t *testing.T, name string) Bundle {
	t.Helper()
	path := filepath.Join("..", "..", "..", "examples", "appfactory", "generic", name, "requirement.md")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", path, err)
	}
	bundle, err := Compile(Request{
		RequirementText:   string(data),
		RequirementSource: path,
	})
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	return bundle
}

func normalizeBuildInputForGolden(input appruns.BuildInput) ([]byte, error) {
	normalized := input
	normalized.PreparedPRDSubjectVersion = "<normalized>"
	normalized.PreparedTemplateSubjectVersion = "<normalized>"
	normalized.ContextSourceDir = ""
	normalized.TemplateSourceDir = ""
	normalized.WorkspacePath = "<normalized>"
	normalized.ArtifactDir = "<normalized>"
	normalized.CommandProfile = normalizeRawJSONForGolden(normalized.CommandProfile)
	normalized.ContextFiles = normalizeRawJSONForGolden(normalized.ContextFiles)
	normalized.HumanNotes = normalizeRawJSONForGolden(normalized.HumanNotes)
	normalized.PlanningPolicy = appruns.NormalizePlanningPolicySnapshot(normalized.PlanningPolicy)
	data, err := json.MarshalIndent(normalized, "", "  ")
	if err != nil {
		return nil, err
	}
	return data, nil
}

func normalizeRawJSONForGolden(raw json.RawMessage) json.RawMessage {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return nil
	}
	return normalizeJSONForGolden(trimmed)
}

func normalizeJSONForGolden(raw []byte) []byte {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return nil
	}
	var decoded any
	if err := json.Unmarshal(trimmed, &decoded); err != nil {
		copyBytes := make([]byte, len(trimmed))
		copy(copyBytes, trimmed)
		return copyBytes
	}
	normalized, err := json.MarshalIndent(decoded, "", "  ")
	if err != nil {
		copyBytes := make([]byte, len(trimmed))
		copy(copyBytes, trimmed)
		return copyBytes
	}
	return normalized
}

func mustDecodeJSON(t *testing.T, data []byte, target any) {
	t.Helper()
	if err := json.Unmarshal(data, target); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
}

func fieldNames(fields []DataField) []string {
	names := make([]string, 0, len(fields))
	for _, field := range fields {
		names = append(names, field.Name)
	}
	return names
}

func findAllocationUnit(t *testing.T, units []TaskAllocationUnit, allocationID string) TaskAllocationUnit {
	t.Helper()
	for _, unit := range units {
		if unit.AllocationID == allocationID {
			return unit
		}
	}
	t.Fatalf("allocation unit %q not found", allocationID)
	return TaskAllocationUnit{}
}

func findTaskBundleItem(t *testing.T, items []appruns.TaskBundleItem, taskID string) appruns.TaskBundleItem {
	t.Helper()
	for _, item := range items {
		if item.TaskID == taskID {
			return item
		}
	}
	t.Fatalf("task bundle item %q not found", taskID)
	return appruns.TaskBundleItem{}
}

func mustFindAcceptancePlanItem(t *testing.T, items []AcceptancePlanItem, checkID string) AcceptancePlanItem {
	t.Helper()
	for _, item := range items {
		if item.CheckID == checkID {
			return item
		}
	}
	t.Fatalf("acceptance plan item %q not found", checkID)
	return AcceptancePlanItem{}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
