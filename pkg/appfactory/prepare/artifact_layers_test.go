package prepare

import (
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	appruns "github.com/sipeed/oneappfactory/pkg/appfactory/runs"
)

func TestBuildPlanningContextLayerCapturesPolicyAndRoutes(t *testing.T) {
	spec, prd := newLayerTestFixture()
	spec.TaskBundle[1].RouteHint = appruns.TaskRouteHintStrongModel
	spec.TaskBundle[1].RiskLevel = appruns.TaskRiskLevelHigh
	spec.TaskBundle[1].AllocationTransition = &appruns.TaskAllocationTransition{
		AllocationID:       "alloc-domain-copy",
		SemanticIntentRefs: []string{"ac-overview"},
		OwnedPaths:         []string{"lib/template/open_lite_copy.dart"},
		BlockedBy:          []string{"task-generic-domain-models"},
		SuccessEvidence:    []string{"领域文案已覆盖默认 seed 文案"},
	}

	context := buildPlanningContext(spec, prd, currentPlanningPolicySnapshot())
	if context.PlanningPolicyVersion != "phase1-boundary-v1" {
		t.Fatalf("PlanningPolicyVersion = %q, want phase1-boundary-v1", context.PlanningPolicyVersion)
	}
	if context.PlanningModelSnapshot.DomainModeling != string(appruns.PlanningStageRoutePlanningModel) {
		t.Fatalf("DomainModeling route = %q, want planning_model", context.PlanningModelSnapshot.DomainModeling)
	}
	if context.PlanningModelSnapshot.BuildInputProjection != string(appruns.PlanningStageRouteDeterministic) {
		t.Fatalf("BuildInputProjection route = %q, want deterministic", context.PlanningModelSnapshot.BuildInputProjection)
	}
	if got := context.ExecutionRouteSnapshot.RouteHintCounts[string(appruns.TaskRouteHintDefaultModel)]; got != 0 {
		t.Fatalf("default_model count = %d, want 0", got)
	}
	if got := context.ExecutionRouteSnapshot.RouteHintCounts[string(appruns.TaskRouteHintStrongModel)]; got != 1 {
		t.Fatalf("strong_model count = %d, want 1", got)
	}
	if got := context.ExecutionRouteSnapshot.RouteHintCounts[string(appruns.TaskRouteHintDeterministic)]; got != len(context.ExecutionRouteSnapshot.TaskRoutes)-1 {
		t.Fatalf("deterministic count = %d, want task route count minus strong_model", got)
	}
	if context.ExecutionRouteSnapshot.TaskRoutes[1].AllocationID != "alloc-domain-copy" {
		t.Fatalf("TaskRoutes[1].AllocationID = %q, want alloc-domain-copy", context.ExecutionRouteSnapshot.TaskRoutes[1].AllocationID)
	}
	if context.ExecutionRouteSnapshot.TaskRoutes[1].RiskLevel != appruns.TaskRiskLevelHigh {
		t.Fatalf("TaskRoutes[1].RiskLevel = %q, want high", context.ExecutionRouteSnapshot.TaskRoutes[1].RiskLevel)
	}
	if context.ExecutionRouteSnapshot.CommandProfileRef != spec.CommandProfile.ProfileName {
		t.Fatalf("CommandProfileRef = %q, want %q", context.ExecutionRouteSnapshot.CommandProfileRef, spec.CommandProfile.ProfileName)
	}
	if context.ExecutionRouteSnapshot.UpgradePolicy.MaxFilesBeforeUpgrade != 2 {
		t.Fatalf("UpgradePolicy.MaxFilesBeforeUpgrade = %d, want 2", context.ExecutionRouteSnapshot.UpgradePolicy.MaxFilesBeforeUpgrade)
	}
	if len(context.ExecutionRouteSnapshot.UpgradePolicy.SupportedRetryUpgradeSignals) != 6 {
		t.Fatalf("SupportedRetryUpgradeSignals = %v, want 6 signals", context.ExecutionRouteSnapshot.UpgradePolicy.SupportedRetryUpgradeSignals)
	}
	if !context.ExecutionRouteSnapshot.StartUpgradeCheck.WouldUpgradeFromStart {
		t.Fatalf("StartUpgradeCheck = %+v, want would_upgrade_from_start=true", context.ExecutionRouteSnapshot.StartUpgradeCheck)
	}
	if !strings.Contains(strings.Join(context.ExecutionRouteSnapshot.StartUpgradeCheck.TriggeredReasons, ","), "max_files_before_upgrade") {
		t.Fatalf("TriggeredReasons = %v, want include max_files_before_upgrade", context.ExecutionRouteSnapshot.StartUpgradeCheck.TriggeredReasons)
	}
}

func TestBuildDomainModelLayerCapturesDerivedMetricsAndSemanticRules(t *testing.T) {
	spec, _ := newLayerTestFixture()

	model := buildDomainModel(spec)
	if model.DomainName != spec.Title {
		t.Fatalf("DomainName = %q, want %q", model.DomainName, spec.Title)
	}
	if len(model.Entities) != 2 {
		t.Fatalf("Entities len = %d, want 2", len(model.Entities))
	}
	if len(model.SummaryMetrics) != 1 || model.SummaryMetrics[0] != "待办概览摘要" {
		t.Fatalf("SummaryMetrics = %v, want [待办概览摘要]", model.SummaryMetrics)
	}
	if len(model.CriticalFlows) != len(spec.UserFlows) {
		t.Fatalf("CriticalFlows len = %d, want %d", len(model.CriticalFlows), len(spec.UserFlows))
	}
	if model.DomainCopy.ProblemStatement != spec.ProblemStatement {
		t.Fatalf("DomainCopy.ProblemStatement = %q, want %q", model.DomainCopy.ProblemStatement, spec.ProblemStatement)
	}
	if len(model.SemanticAcceptanceRules) == 0 || model.SemanticAcceptanceRules[0].RuleID != "ac-overview" {
		t.Fatalf("SemanticAcceptanceRules = %+v, want first rule ac-overview", model.SemanticAcceptanceRules)
	}
	if len(model.SemanticAcceptanceRules[0].FieldRefs) == 0 {
		t.Fatalf("SemanticAcceptanceRules[0].FieldRefs = %v, want non-empty summary refs", model.SemanticAcceptanceRules[0].FieldRefs)
	}
	if !strings.Contains(model.SemanticAcceptanceRules[0].EvidencePattern, "title") && !strings.Contains(model.SemanticAcceptanceRules[0].EvidencePattern, "inbox_count") {
		t.Fatalf("SemanticAcceptanceRules[0].EvidencePattern = %q, want include semantic field refs", model.SemanticAcceptanceRules[0].EvidencePattern)
	}
	if !strings.Contains(model.SemanticAcceptanceRules[0].EvidencePattern, "inboxCount") {
		t.Fatalf("SemanticAcceptanceRules[0].EvidencePattern = %q, want include camelCase semantic aliases", model.SemanticAcceptanceRules[0].EvidencePattern)
	}
}

func TestSemanticEvidenceAliasesIncludeCamelCaseAndTotalBase(t *testing.T) {
	got := semanticEvidenceAliases("income_total")
	want := []string{"income_total", "incomeTotal", "income"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("semanticEvidenceAliases() = %v, want %v", got, want)
	}
}

func TestBuildSemanticEvidenceCommandSkipsMissingAndroidStringsPath(t *testing.T) {
	got := buildSemanticEvidenceCommand("income_total|balance")
	if !strings.Contains(got, "[ -f android/app/src/main/res/values/strings.xml ]") {
		t.Fatalf("buildSemanticEvidenceCommand() = %q, want conditional strings.xml guard", got)
	}
	if !strings.Contains(got, "[ -d lib ]") || !strings.Contains(got, "[ -d test ]") {
		t.Fatalf("buildSemanticEvidenceCommand() = %q, want lib/test guards", got)
	}
	if !strings.Contains(got, "grep -ER 'income_total|balance' $paths") {
		t.Fatalf("buildSemanticEvidenceCommand() = %q, want grep over guarded paths", got)
	}
}

func TestBuildTemplateSlotMapLayerKeepsOpenLiteProjection(t *testing.T) {
	spec, _ := newLayerTestFixture()

	slotMap := buildTemplateSlotMap(spec)
	if want := TemplateCompileSourceVersion("flutter-open-lite", "v0.2.0"); slotMap.TemplateSubjectVersion != want {
		t.Fatalf("TemplateSubjectVersion = %q, want %q", slotMap.TemplateSubjectVersion, want)
	}
	if len(slotMap.Slots) != 9 {
		t.Fatalf("Slots len = %d, want 9", len(slotMap.Slots))
	}
	if !strings.HasPrefix(slotMap.Slots[0].SlotID, "open-lite-") || slotMap.Slots[0].BindingID != genericSurfaceOverviewID || slotMap.Slots[0].OverridePolicy != "replace" {
		t.Fatalf("first slot = %+v, want template-private open-lite overview binding", slotMap.Slots[0])
	}
	var foundAppEntry bool
	var foundDomainCopy bool
	for _, slot := range slotMap.Slots {
		if slot.BindingID == publicBindingAppEntry {
			foundAppEntry = true
			if slot.SlotKind != "app_entry" || len(slot.TargetPaths) != 1 || slot.TargetPaths[0] != "lib/main.dart" {
				t.Fatalf("app-entry slot = %+v, want app_entry -> lib/main.dart", slot)
			}
			if slot.OverridePolicy != "replace" || !slot.EmitEligible {
				t.Fatalf("app-entry slot routing = %+v, want replace + emit eligible", slot)
			}
			continue
		}
		if slot.BindingID != publicBindingDomainCopy {
			continue
		}
		foundDomainCopy = true
		if !strings.HasPrefix(slot.SlotID, "open-lite-") {
			t.Fatalf("domain copy slot_id = %q, want open-lite private prefix", slot.SlotID)
		}
		if slot.BindingID != publicBindingDomainCopy {
			t.Fatalf("domain-copy binding = %q, want %q", slot.BindingID, publicBindingDomainCopy)
		}
		if slot.OverridePolicy != "synchronize" {
			t.Fatalf("domain-copy override = %q, want synchronize", slot.OverridePolicy)
		}
		if len(slot.AcceptanceImpacts) != 3 {
			t.Fatalf("domain-copy acceptance impacts = %v, want 3 items", slot.AcceptanceImpacts)
		}
	}
	if !foundAppEntry {
		t.Fatal("missing app-entry slot")
	}
	if !foundDomainCopy {
		t.Fatal("missing domain-copy slot")
	}
}

func TestBuildTemplateSlotMapLayerKeepsFinanceLiteProjection(t *testing.T) {
	spec := compileBookkeepingSpec(Request{
		JobID:      "job-finance-slot-fixture",
		PRDID:      "prd-finance-slot-fixture",
		TemplateID: "flutter-finance-lite",
		Now:        func() time.Time { return time.Date(2026, 4, 10, 8, 0, 0, 0, time.UTC) },
	}, "做一个简单记账 app，需要首页概览、记一笔和账单列表。")
	spec.TemplatePinnedRef = "v0.1.0"

	slotMap := buildTemplateSlotMap(spec)
	if want := TemplateCompileSourceVersion("flutter-finance-lite", "v0.1.0"); slotMap.TemplateSubjectVersion != want {
		t.Fatalf("TemplateSubjectVersion = %q, want %q", slotMap.TemplateSubjectVersion, want)
	}
	if len(slotMap.Slots) != 8 {
		t.Fatalf("Slots len = %d, want 8", len(slotMap.Slots))
	}
	if !strings.HasPrefix(slotMap.Slots[1].SlotID, "finance-lite-") || slotMap.Slots[1].BindingID != genericSurfaceCollectionID || slotMap.Slots[1].TargetPaths[0] != "lib/views/entry_list_page.dart" {
		t.Fatalf("list slot = %+v, want finance-lite private collection projection", slotMap.Slots[1])
	}
	if !strings.HasPrefix(slotMap.Slots[4].SlotID, "finance-lite-") || slotMap.Slots[4].BindingID != publicBindingDomainCopy || slotMap.Slots[4].OverridePolicy != "synchronize" {
		t.Fatalf("domain-copy slot = %+v, want finance-lite private synchronize copy slot", slotMap.Slots[4])
	}
	if !strings.Contains(strings.Join(slotMap.Slots[4].AcceptanceImpacts, ","), "mrp-copy") {
		t.Fatalf("domain-copy acceptance impacts = %v, want include mrp-copy", slotMap.Slots[4].AcceptanceImpacts)
	}
	if slotMap.Slots[3].BindingID != publicBindingAppEntry || slotMap.Slots[3].SlotKind != "app_entry" || slotMap.Slots[3].TargetPaths[0] != "lib/main.dart" {
		t.Fatalf("app-entry slot = %+v, want finance-lite app_entry main.dart slot", slotMap.Slots[3])
	}
	for _, slot := range slotMap.Slots {
		if slot.BindingID == genericSurfaceInspectionID {
			t.Fatalf("unexpected inspection binding in finance-lite projection: %+v", slot)
		}
	}
}

func TestBuildTemplateSlotMapLayerUsesTemplatePrivateSlotIDs(t *testing.T) {
	openLiteSpec, _ := newLayerTestFixture()
	testCases := []struct {
		name          string
		spec          domainSpec
		privatePrefix string
	}{
		{name: "open-lite", spec: openLiteSpec, privatePrefix: "open-lite-"},
		{name: "finance-lite", spec: func() domainSpec {
			spec := compileBookkeepingSpec(Request{JobID: "job-finance-slot-private", PRDID: "prd-finance-slot-private", TemplateID: "flutter-finance-lite", Now: func() time.Time { return time.Date(2026, 4, 10, 8, 0, 0, 0, time.UTC) }}, "做一个简单记账 app，需要首页概览、记一笔和账单列表。")
			spec.TemplatePinnedRef = "v0.1.0"
			return spec
		}(), privatePrefix: "finance-lite-"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			slotMap := buildTemplateSlotMap(tc.spec)
			for _, slot := range slotMap.Slots {
				if !strings.HasPrefix(slot.SlotID, tc.privatePrefix) {
					t.Fatalf("slot %q = %q, want private prefix %q", slot.BindingID, slot.SlotID, tc.privatePrefix)
				}
				if slot.SlotID == slot.BindingID {
					t.Fatalf("slot %q uses public binding as slot_id, want template-private id", slot.BindingID)
				}
			}
		})
	}
}

func TestBuildTemplateSlotMapLayerRequiresBindingIDForEverySlot(t *testing.T) {
	spec, _ := newLayerTestFixture()

	slotMap := buildTemplateSlotMap(spec)
	for _, slot := range slotMap.Slots {
		if strings.TrimSpace(slot.BindingID) == "" {
			t.Fatalf("slot %q binding_id empty, want public binding anchor", slot.SlotID)
		}
	}
}

func TestBuildSurfaceContractsSkipsTemplatePrivateSlotProjectionWithoutBindingID(t *testing.T) {
	contracts := buildSurfaceContracts(PRD{
		FeatureList: []Feature{{
			FeatureID:      "feature-custom-overview",
			AcceptanceRefs: []string{"ac-overview"},
		}},
		SurfaceList: []InteractionSurface{{
			SurfaceID:          "surface-custom-overview",
			Purpose:            "展示自定义概览",
			PrimaryFeatureRefs: []string{"feature-custom-overview"},
		}},
	}, TemplateSlotMap{Slots: []TemplateSlot{{
		SlotID:            "template-private-overview",
		SlotKind:          "summary",
		TargetPaths:       []string{"lib/views/private_overview.dart"},
		AcceptanceImpacts: []string{"ac-overview"},
	}}})

	if len(contracts) != 1 {
		t.Fatalf("contracts len = %d, want 1", len(contracts))
	}
	if contracts[0].TemplateBindingRef != "" {
		t.Fatalf("TemplateBindingRef = %q, want omitted for template-private slot", contracts[0].TemplateBindingRef)
	}
	if len(contracts[0].TargetPaths) != 0 {
		t.Fatalf("TargetPaths = %v, want omitted for template-private slot", contracts[0].TargetPaths)
	}
}

func TestBuildSurfaceContractsProjectsRequiredStatesAndSemanticEntities(t *testing.T) {
	contracts := buildSurfaceContracts(PRD{
		FeatureList: []Feature{{
			FeatureID:      "feature-custom-overview",
			Title:          "库存概览",
			Summary:        "展示低库存摘要与预警",
			AcceptanceRefs: []string{"ac-overview"},
		}},
		SurfaceList: []InteractionSurface{{
			SurfaceID:          "surface-custom-overview",
			Label:              "库存概览",
			Purpose:            "展示低库存概览与预警摘要",
			PrimaryFeatureRefs: []string{"feature-custom-overview"},
		}},
		DataEntities: []DataEntity{
			{EntityID: "entity-sheet", Name: "库存单"},
			{EntityID: "entity-low-stock-summary", Name: "低库存摘要", Source: "derived"},
		},
		AcceptanceCriteria: []AcceptanceCriterion{{
			CriterionID: "ac-overview",
			Label:       "库存概览可见",
			Category:    "functional",
			Required:    true,
			Description: "概览承载单元展示低库存摘要与预警",
		}},
	}, TemplateSlotMap{})

	if len(contracts) != 1 {
		t.Fatalf("contracts len = %d, want 1", len(contracts))
	}
	if !reflect.DeepEqual(contracts[0].PrimaryEntityRefs, []string{"entity-low-stock-summary"}) {
		t.Fatalf("PrimaryEntityRefs = %v, want [entity-low-stock-summary]", contracts[0].PrimaryEntityRefs)
	}
	if len(contracts[0].RequiredStates) != 1 {
		t.Fatalf("RequiredStates len = %d, want 1", len(contracts[0].RequiredStates))
	}
	if contracts[0].RequiredStates[0].StateID != "state-overview" {
		t.Fatalf("RequiredStates[0].StateID = %q, want state-overview", contracts[0].RequiredStates[0].StateID)
	}
	if contracts[0].RequiredStates[0].Evidence != "概览承载单元展示低库存摘要与预警" {
		t.Fatalf("RequiredStates[0].Evidence = %q, want overview evidence", contracts[0].RequiredStates[0].Evidence)
	}
}

func TestBuildKeyFlowsProjectsAcceptanceRefsFromSurfaceBindings(t *testing.T) {
	flows := buildKeyFlows(PRD{
		FeatureList: []Feature{{
			FeatureID:          "feature-custom-overview",
			AcceptanceRefs:     []string{"ac-overview"},
			RelatedSurfaceRefs: []string{"surface-custom-overview"},
		}},
		SurfaceList: []InteractionSurface{{
			SurfaceID:          "surface-custom-overview",
			PrimaryFeatureRefs: []string{"feature-custom-overview"},
		}},
		UserFlows: []UserFlow{{
			FlowID: "flow-open-overview",
			Title:  "查看库存概览",
			Steps: []FlowStep{{
				StepID:         "step-open-overview",
				SurfaceRef:     "surface-custom-overview",
				Title:          "打开概览",
				ExpectedResult: "显示低库存摘要",
			}},
		}},
	})

	if len(flows) != 1 {
		t.Fatalf("flows len = %d, want 1", len(flows))
	}
	if !reflect.DeepEqual(flows[0].AcceptanceRefs, []string{"ac-overview"}) {
		t.Fatalf("AcceptanceRefs = %v, want [ac-overview]", flows[0].AcceptanceRefs)
	}
}

func TestSelectSurfaceTemplateBindingDoesNotGuessCustomSurfaceFromTemplateAcceptanceImpacts(t *testing.T) {
	binding := selectSurfaceTemplateBinding("surface-custom-overview", []string{"ac-overview"}, TemplateSlotMap{Slots: []TemplateSlot{
		{
			SlotID:            "template-private-overview",
			SlotKind:          "summary",
			TargetPaths:       []string{"lib/views/private_overview.dart"},
			AcceptanceImpacts: []string{"ac-overview"},
		},
		{
			BindingID:         genericSurfaceOverviewID,
			SlotID:            "template-public-overview",
			SlotKind:          "summary",
			TargetPaths:       []string{"lib/views/public_overview.dart"},
			AcceptanceImpacts: []string{"ac-overview"},
		},
	}})

	if binding.BindingID != "" {
		t.Fatalf("binding = %+v, want custom surface binding omitted without explicit public binding_id match", binding)
	}
	if len(binding.TargetPaths) != 0 {
		t.Fatalf("binding target_paths = %v, want omitted without explicit public binding_id match", binding.TargetPaths)
	}
}

func TestSelectSurfaceTemplateBindingUsesBindingIDEvenWhenSlotKindPrivate(t *testing.T) {
	binding := selectSurfaceTemplateBinding(genericSurfaceOverviewID, []string{"ac-overview"}, TemplateSlotMap{Slots: []TemplateSlot{
		{
			BindingID:         genericSurfaceOverviewID,
			SlotID:            "template-private-overview",
			SlotKind:          "template-private-dashboard",
			TargetPaths:       []string{"lib/views/public_overview.dart"},
			AcceptanceImpacts: []string{"ac-overview"},
		},
		{
			BindingID:         publicBindingDomainCopy,
			SlotID:            "template-domain-copy",
			SlotKind:          "copy",
			TargetPaths:       []string{"lib/template/open_lite_copy.dart"},
			AcceptanceImpacts: []string{"ac-overview"},
		},
	}})

	if binding.BindingID != genericSurfaceOverviewID {
		t.Fatalf("binding = %+v, want binding_id %q even when slot_kind is private", binding, genericSurfaceOverviewID)
	}
	if len(binding.TargetPaths) != 1 || binding.TargetPaths[0] != "lib/views/public_overview.dart" {
		t.Fatalf("binding target_paths = %v, want overview target even when slot_kind is private", binding.TargetPaths)
	}
}

func TestFlutterOpenLiteSlotRegistryFixtureMatchesProjection(t *testing.T) {
	spec, _ := newLayerTestFixture()
	registryData, err := os.ReadFile(appFactoryRepoPath("docs", "design", "template-registry", "flutter-open-lite.json"))
	if err != nil {
		t.Fatalf("ReadFile(flutter-open-lite registry) error = %v", err)
	}
	var entry struct {
		TemplateID string          `json:"template_id"`
		SlotMap    TemplateSlotMap `json:"slot_map"`
	}
	if err := json.Unmarshal(registryData, &entry); err != nil {
		t.Fatalf("Unmarshal(flutter-open-lite registry) error = %v", err)
	}
	if entry.TemplateID != "flutter-open-lite" {
		t.Fatalf("registry template_id = %q, want flutter-open-lite", entry.TemplateID)
	}
	projected := buildTemplateSlotMap(spec)
	if entry.SlotMap.TemplateSubjectVersion != projected.TemplateSubjectVersion {
		t.Fatalf("registry template_subject_version = %q, want %q", entry.SlotMap.TemplateSubjectVersion, projected.TemplateSubjectVersion)
	}
	if len(entry.SlotMap.Slots) != len(projected.Slots) {
		t.Fatalf("registry slots len = %d, want %d", len(entry.SlotMap.Slots), len(projected.Slots))
	}
	for index, slot := range projected.Slots {
		if entry.SlotMap.Slots[index].SlotID != slot.SlotID {
			t.Fatalf("registry slot[%d] = %q, want %q", index, entry.SlotMap.Slots[index].SlotID, slot.SlotID)
		}
		if entry.SlotMap.Slots[index].BindingID != slot.BindingID {
			t.Fatalf("registry slot[%d] binding = %q, want %q", index, entry.SlotMap.Slots[index].BindingID, slot.BindingID)
		}
		if entry.SlotMap.Slots[index].OverridePolicy != slot.OverridePolicy {
			t.Fatalf("registry slot[%d] override = %q, want %q", index, entry.SlotMap.Slots[index].OverridePolicy, slot.OverridePolicy)
		}
		if len(entry.SlotMap.Slots[index].AcceptanceImpacts) == 0 {
			t.Fatalf("registry slot[%d] acceptance_impacts empty, want governance baseline", index)
		}
	}
}

func TestFlutterFinanceLiteSlotRegistryFixtureMatchesProjection(t *testing.T) {
	spec := compileBookkeepingSpec(Request{
		JobID:      "job-finance-registry-fixture",
		PRDID:      "prd-finance-registry-fixture",
		TemplateID: "flutter-finance-lite",
		Now:        func() time.Time { return time.Date(2026, 4, 10, 8, 30, 0, 0, time.UTC) },
	}, "做一个简单记账 app，需要首页概览、记一笔和账单列表。")
	spec.TemplatePinnedRef = "v0.1.0"
	registryData, err := os.ReadFile(appFactoryRepoPath("docs", "design", "template-registry", "flutter-finance-lite.json"))
	if err != nil {
		t.Fatalf("ReadFile(flutter-finance-lite registry) error = %v", err)
	}
	var entry struct {
		TemplateID string          `json:"template_id"`
		SlotMap    TemplateSlotMap `json:"slot_map"`
	}
	if err := json.Unmarshal(registryData, &entry); err != nil {
		t.Fatalf("Unmarshal(flutter-finance-lite registry) error = %v", err)
	}
	if entry.TemplateID != "flutter-finance-lite" {
		t.Fatalf("registry template_id = %q, want flutter-finance-lite", entry.TemplateID)
	}
	projected := buildTemplateSlotMap(spec)
	if entry.SlotMap.TemplateSubjectVersion != projected.TemplateSubjectVersion {
		t.Fatalf("registry template_subject_version = %q, want %q", entry.SlotMap.TemplateSubjectVersion, projected.TemplateSubjectVersion)
	}
	if len(entry.SlotMap.Slots) != len(projected.Slots) {
		t.Fatalf("registry slots len = %d, want %d", len(entry.SlotMap.Slots), len(projected.Slots))
	}
	for index, slot := range projected.Slots {
		if entry.SlotMap.Slots[index].SlotID != slot.SlotID {
			t.Fatalf("registry slot[%d] = %q, want %q", index, entry.SlotMap.Slots[index].SlotID, slot.SlotID)
		}
		if entry.SlotMap.Slots[index].BindingID != slot.BindingID {
			t.Fatalf("registry slot[%d] binding = %q, want %q", index, entry.SlotMap.Slots[index].BindingID, slot.BindingID)
		}
		if entry.SlotMap.Slots[index].OverridePolicy != slot.OverridePolicy {
			t.Fatalf("registry slot[%d] override = %q, want %q", index, entry.SlotMap.Slots[index].OverridePolicy, slot.OverridePolicy)
		}
		if len(entry.SlotMap.Slots[index].AcceptanceImpacts) == 0 {
			t.Fatalf("registry slot[%d] acceptance_impacts empty, want governance baseline", index)
		}
	}
}

func TestBuildBundleLayerProjectsArtifactsAndNormalizesTasks(t *testing.T) {
	spec, prd := newLayerTestFixture()
	now := time.Date(2026, 4, 6, 9, 30, 0, 0, time.UTC)

	bundle, err := buildBundle(spec, prd, now)
	if err != nil {
		t.Fatalf("buildBundle() error = %v", err)
	}
	if bundle.BuilderInput.WorkspacePath != "/workspace/job-layer-fixture" {
		t.Fatalf("WorkspacePath = %q, want /workspace/job-layer-fixture", bundle.BuilderInput.WorkspacePath)
	}
	if bundle.BuilderInput.ArtifactDir != "/artifacts/job-layer-fixture" {
		t.Fatalf("ArtifactDir = %q, want /artifacts/job-layer-fixture", bundle.BuilderInput.ArtifactDir)
	}
	if bundle.BuilderInput.TaskBundle[0].AllocationTransition == nil {
		t.Fatal("TaskBundle[0].AllocationTransition = nil, want normalized transition")
	}
	if bundle.BuilderInput.TaskBundle[0].AllocationTransition.AllocationID != "task-create-record-model" {
		t.Fatalf("TaskBundle[0].allocation_id = %q, want task-create-record-model", bundle.BuilderInput.TaskBundle[0].AllocationTransition.AllocationID)
	}
	for _, name := range []string{planningContextFileName, domainModelFileName, templateSlotMapFileName, taskAllocationFileName, acceptancePlanFileName, prdApprovalFileName, templateApprovalFileName} {
		if _, ok := bundle.Files[name]; !ok {
			t.Fatalf("missing generated file %s", name)
		}
	}
	if !strings.Contains(string(bundle.Files[fitReportFileName]), "flutter-open-lite") {
		t.Fatalf("template fit report missing template id: %s", string(bundle.Files[fitReportFileName]))
	}
	if !strings.Contains(string(bundle.Files[requirementFileName]), "需求摘录") {
		t.Fatalf("requirement file missing highlights section: %s", string(bundle.Files[requirementFileName]))
	}
}

func TestBuildTaskProjectionLayerPreservesComplexCoordinationRefs(t *testing.T) {
	tasks := []appruns.TaskBundleItem{{
		TaskID:              "task-project-tag-bridge",
		Title:               "打通任务与标签关联",
		Category:            appruns.TaskCategoryFlow,
		TaskType:            appruns.BuilderRuntimeTaskTypeDualFileWiring,
		Objective:           "接通任务、标签和关联写入链路",
		RelatedRequirements: []string{"ac-task-tag-binding"},
		Dependencies:        []string{"task-project-models"},
		TargetPaths:         []string{"lib/controllers/task_tag_controller.dart", "lib/repositories/task_repository.dart"},
		CompletionCriteria:  []string{"任务与标签关系可保存"},
	}}
	allocation := TaskAllocation{Units: []TaskAllocationUnit{{
		AllocationID:        "task-project-tag-bridge",
		Wave:                2,
		Lane:                "flow",
		SurfaceRefs:         []string{"surface-project-board", "surface-task-detail"},
		EntityRefs:          []string{"entity-project", "entity-task", "entity-tag"},
		RelationGroupRefs:   []string{"relation-project-task", "relation-task-tag"},
		SharedOwnershipRefs: []string{"shared-task-repository"},
		BindingRefs:         []string{publicBindingFlow, publicBindingStorage},
		SemanticIntentRefs:  []string{"ac-task-tag-binding"},
		OwnedPaths:          []string{"lib/controllers/task_tag_controller.dart"},
		SuccessEvidence:     []string{"任务与标签关系已接线"},
	}}}

	projection := buildTaskProjection(tasks, allocation)
	if len(projection) != 1 {
		t.Fatalf("projection len = %d, want 1", len(projection))
	}
	item := projection[0]
	if !reflect.DeepEqual(item.SurfaceRefs, []string{"surface-project-board", "surface-task-detail"}) {
		t.Fatalf("surface_refs = %v, want project/detail refs", item.SurfaceRefs)
	}
	if !reflect.DeepEqual(item.EntityRefs, []string{"entity-project", "entity-task", "entity-tag"}) {
		t.Fatalf("entity_refs = %v, want project/task/tag refs", item.EntityRefs)
	}
	if !reflect.DeepEqual(item.RelationGroupRefs, []string{"relation-project-task", "relation-task-tag"}) {
		t.Fatalf("relation_group_refs = %v, want relation refs", item.RelationGroupRefs)
	}
	if !reflect.DeepEqual(item.SharedOwnershipRefs, []string{"shared-task-repository"}) {
		t.Fatalf("shared_ownership_refs = %v, want shared repository ref", item.SharedOwnershipRefs)
	}
	projectionJSON, err := json.Marshal(item)
	if err != nil {
		t.Fatalf("Marshal(task projection) error = %v", err)
	}
	if strings.Contains(string(projectionJSON), `"screen_refs"`) {
		t.Fatalf("task projection JSON = %s, want screen_refs omitted", string(projectionJSON))
	}
}

func TestNormalizePreparedTaskBundleItemProjectsExplicitBindingRefsFromTaskOwnership(t *testing.T) {
	task := normalizePreparedTaskBundleItem(appruns.TaskBundleItem{
		TaskID:      "task-bind-copy-and-surface",
		Category:    appruns.TaskCategoryScreen,
		TargetPaths: []string{"lib/main.dart", "lib/views/home_page.dart", "lib/template/open_lite_copy.dart"},
		AllocationTransition: &appruns.TaskAllocationTransition{
			SurfaceRefs: []string{genericSurfaceOverviewID},
			BindingRefs: []string{publicBindingDomainCopy},
		},
	})
	if task.AllocationTransition == nil {
		t.Fatal("AllocationTransition = nil, want normalized transition")
	}
	joined := strings.Join(task.AllocationTransition.BindingRefs, ",")
	for _, expected := range []string{publicBindingAppEntry, genericSurfaceOverviewID, publicBindingDomainCopy} {
		if !strings.Contains(joined, expected) {
			t.Fatalf("binding_refs = %v, want include %s", task.AllocationTransition.BindingRefs, expected)
		}
	}
}

func TestBuildTaskIntentBindingRefsUsesDeclaredTransitionBindingRefsOnly(t *testing.T) {
	spec := domainSpec{
		SurfaceList: []InteractionSurface{
			{SurfaceID: "surface-project-board", Label: "项目看板", LegacyScreenRef: "screen-project-board"},
			{SurfaceID: "surface-task-detail", Label: "任务详情", LegacyScreenRef: "screen-task-detail"},
		},
		TaskBundle: []appruns.TaskBundleItem{{
			TaskID:   "task-project-tag-bridge",
			Category: appruns.TaskCategoryFlow,
			TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring,
			AllocationTransition: &appruns.TaskAllocationTransition{
				AllocationID:       "task-project-tag-bridge",
				ScreenRefs:         []string{"screen-project-board", "screen-task-detail"},
				BindingRefs:        []string{"binding-project-board", "binding-task-detail"},
				SemanticIntentRefs: []string{"ac-task-tag-binding"},
			},
		}},
	}

	refs := buildTaskIntentBindingRefs(spec)
	taskRefs := refs["ac-task-tag-binding"]
	if len(taskRefs) != 1 {
		t.Fatalf("task intent refs = %v, want 1 binding group", taskRefs)
	}
	if !reflect.DeepEqual(taskIntentBindingRefs(taskRefs), []string{"binding-project-board", "binding-task-detail"}) {
		t.Fatalf("binding refs = %v, want canonical surface binding refs", taskIntentBindingRefs(taskRefs))
	}
}

func TestBuildTaskIntentBindingRefsDoesNotBackfillFromLegacyScreenRefs(t *testing.T) {
	spec := domainSpec{
		SurfaceList: []InteractionSurface{{SurfaceID: "surface-project-board", Label: "项目看板", LegacyScreenRef: "screen-project-board"}},
		TaskBundle: []appruns.TaskBundleItem{{
			TaskID:   "task-project-tag-bridge",
			Category: appruns.TaskCategoryFlow,
			TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring,
			AllocationTransition: &appruns.TaskAllocationTransition{
				AllocationID:       "task-project-tag-bridge",
				ScreenRefs:         []string{"screen-project-board"},
				SemanticIntentRefs: []string{"ac-task-tag-binding"},
			},
		}},
	}

	refs := buildTaskIntentBindingRefs(spec)
	if len(refs["ac-task-tag-binding"]) != 0 {
		t.Fatalf("task intent refs = %v, want no legacy screen-derived binding backfill without explicit binding_refs", refs["ac-task-tag-binding"])
	}
}

func TestManualReviewBindingRefsUsesDeclaredTaskBindingsWithoutPointIDHeuristics(t *testing.T) {
	taskRefs := []taskIntentBindingRef{{lane: string(appruns.TaskCategoryStorage), bindingRefs: []string{publicBindingStorage}}}

	if got := manualReviewBindingRefs("mrp-copy", taskRefs); !reflect.DeepEqual(got, []string{publicBindingStorage}) {
		t.Fatalf("manual review binding refs = %v, want declared task bindings without point_id remap", got)
	}
}

func TestApprovalLayerSubjectVersionChangesWithEvidence(t *testing.T) {
	spec, prd := newLayerTestFixture()
	now := time.Date(2026, 4, 6, 10, 0, 0, 0, time.UTC)

	prdApprovalA := buildPRDApprovalRecord(spec, prd, "# 待办事项 App\n\n版本 A", "原始需求 A", now)
	prdApprovalB := buildPRDApprovalRecord(spec, prd, "# 待办事项 App\n\n版本 B", "原始需求 A", now)
	if prdApprovalA.SubjectVersion == prdApprovalB.SubjectVersion {
		t.Fatalf("PRD approval subject version should change when evidence changes: %q", prdApprovalA.SubjectVersion)
	}
	if prdApprovalA.Decision == nil || prdApprovalA.Decision.DecidedAt != now.Format(time.RFC3339) {
		t.Fatalf("PRD approval decision = %+v, want decided_at %s", prdApprovalA.Decision, now.Format(time.RFC3339))
	}

	templateApprovalA := buildTemplateApprovalRecord(spec, prd, "fit-report-a", now)
	templateApprovalB := buildTemplateApprovalRecord(spec, prd, "fit-report-b", now)
	if templateApprovalA.SubjectVersion == templateApprovalB.SubjectVersion {
		t.Fatalf("template approval subject version should change when fit report changes: %q", templateApprovalA.SubjectVersion)
	}
	if templateApprovalA.ApprovalType != appruns.ApprovalTypeTemplate {
		t.Fatalf("template approval type = %q, want template", templateApprovalA.ApprovalType)
	}
}

func newLayerTestFixture() (domainSpec, PRD) {
	spec := compileGenericSpec(Request{
		JobID:      "job-layer-fixture",
		PRDID:      "prd-layer-fixture",
		TemplateID: "flutter-open-lite",
		Now:        func() time.Time { return time.Date(2026, 4, 6, 8, 0, 0, 0, time.UTC) },
		TitleHint:  "待办事项 App",
		RealBuild:  false,
	}, "做一个待办事项 app，需要首页摘要、新建待办、详情页和本地可用。")
	spec.TemplatePinnedRef = "v0.2.0"
	spec.TemplateName = "Flutter Open Lite"
	spec.TemplateHealthStatus = "healthy"
	prd := PRD{
		SchemaVersion:       defaultSchemaVersion,
		ID:                  spec.PRDID,
		Version:             defaultSchemaVersion,
		Status:              "draft",
		Title:               spec.Title,
		Summary:             spec.Summary,
		ProblemStatement:    spec.ProblemStatement,
		TargetUsers:         append([]UserProfile(nil), spec.TargetUsers...),
		CoreScenarios:       append([]Scenario(nil), spec.CoreScenarios...),
		Goals:               append([]string(nil), spec.Goals...),
		NonGoals:            append([]string(nil), spec.NonGoals...),
		FeatureList:         append([]Feature(nil), spec.FeatureList...),
		ScreenList:          append([]Screen(nil), spec.ScreenList...),
		UserFlows:           append([]UserFlow(nil), spec.UserFlows...),
		DataEntities:        append([]DataEntity(nil), spec.DataEntities...),
		TemplateConstraints: spec.TemplateConstraints,
		AcceptanceCriteria:  append([]AcceptanceCriterion(nil), spec.AcceptanceCriteria...),
		ManualReviewPoints:  append([]ManualReviewPoint(nil), spec.ManualReviewPoints...),
		KnownUnknowns:       append([]KnownUnknown(nil), spec.KnownUnknowns...),
		SourceRefs:          []string{sourceSummary("inline:test")},
		CreatedAt:           time.Date(2026, 4, 6, 8, 0, 0, 0, time.UTC).Format(time.RFC3339),
		UpdatedAt:           time.Date(2026, 4, 6, 8, 0, 0, 0, time.UTC).Format(time.RFC3339),
	}
	return spec, prd
}
