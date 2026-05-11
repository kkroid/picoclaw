package prepare

import (
	"strings"
	"testing"

	appruns "github.com/sipeed/oneappfactory/pkg/appfactory/runs"
)

func TestBuildTaskAllocationLayerMapsDomainCopyAndBrandingSlots(t *testing.T) {
	spec, _ := newLayerTestFixture()
	allocation := buildTaskAllocation(spec.TaskBundle, spec)

	var bindingRefs []string
	var semanticRefs []string
	found := false
	for _, unit := range allocation.Units {
		if unit.Lane != string(appruns.TaskCategoryContent) {
			continue
		}
		found = true
		bindingRefs = append(bindingRefs, unit.BindingRefs...)
		semanticRefs = append(semanticRefs, unit.SemanticIntentRefs...)
	}
	if !found {
		t.Fatal("missing content allocation unit")
	}
	joined := strings.Join(bindingRefs, ",")
	for _, expected := range []string{publicBindingDomainCopy, publicBindingBranding} {
		if !strings.Contains(joined, expected) {
			t.Fatalf("content binding refs = %v, want include %s", bindingRefs, expected)
		}
	}
	intentJoined := strings.Join(semanticRefs, ",")
	for _, expected := range []string{"mrp-domain-wording", "check-profile-open-lite-domain-branding", "check-profile-open-lite-domain-language"} {
		if !strings.Contains(intentJoined, expected) {
			t.Fatalf("content semantic intents = %v, want include %s", semanticRefs, expected)
		}
	}
}

func TestCompileGenericSpecLayerCarriesWidgetTestSyncConstraint(t *testing.T) {
	spec := compileGenericSpec(Request{}, "做一个待办事项 app，需要首页摘要、新建待办、任务列表和详情页，优先保证本地可用。")

	var testTask appruns.TaskBundleItem
	found := false
	for _, task := range spec.TaskBundle {
		if task.TaskID != "task-create-test" {
			continue
		}
		testTask = task
		found = true
		break
	}
	if !found {
		t.Fatal("missing task-create-test task")
	}
	joined := strings.Join(testTask.CompletionCriteria, " | ")
	if !strings.Contains(joined, "test/widget_test.dart") {
		t.Fatalf("test completion criteria = %v, want widget_test sync constraint", testTask.CompletionCriteria)
	}
}

func TestBuildAcceptancePlanLayerProjectsCoordinationBindingRefs(t *testing.T) {
	spec := compileGenericSpec(Request{ExecutorImage: "oneappfactory/builder:local"}, "做一个待办事项 app，需要首页摘要、新建待办、任务列表和详情页，优先保证本地可用。")
	plan := buildAcceptancePlan(spec)

	branding := findAcceptancePlanItem(plan.StructureChecks, "check-profile-open-lite-domain-branding")
	if branding == nil {
		t.Fatal("missing branding structure check")
	}
	for _, expected := range []string{publicBindingDomainCopy, publicBindingBranding} {
		if !strings.Contains(strings.Join(branding.BindingRefs, ","), expected) {
			t.Fatalf("branding binding refs = %v, want include %s", branding.BindingRefs, expected)
		}
	}

	domainLanguage := findAcceptancePlanItem(plan.StructureChecks, "check-profile-open-lite-domain-language")
	if domainLanguage == nil {
		t.Fatal("missing domain language structure check")
	}
	for _, expected := range []string{publicBindingDomainCopy, publicBindingWidgetTest} {
		if !strings.Contains(strings.Join(domainLanguage.BindingRefs, ","), expected) {
			t.Fatalf("domain language binding refs = %v, want include %s", domainLanguage.BindingRefs, expected)
		}
	}

	manualReview := findAcceptancePlanItem(plan.SemanticChecks, "mrp-domain-wording")
	if manualReview == nil {
		t.Fatal("missing domain wording manual review projection")
	}
	if manualReview.SourceType != "manual_review_point" {
		t.Fatalf("manual review source_type = %q, want manual_review_point", manualReview.SourceType)
	}
	manualReviewJoined := strings.Join(manualReview.BindingRefs, ",")
	for _, expected := range []string{publicBindingDomainCopy, publicBindingBranding, genericSurfaceOverviewID, genericSurfaceMutationID, genericSurfaceInspectionID} {
		if !strings.Contains(manualReviewJoined, expected) {
			t.Fatalf("manual review binding refs = %v, want include %s from declared task bindings", manualReview.BindingRefs, expected)
		}
	}
	if manualReviewJoined == publicBindingDomainCopy {
		t.Fatalf("manual review binding refs = %v, want declared task bindings instead of point_id keyword narrowing", manualReview.BindingRefs)
	}
}

func TestBuildAcceptancePlanLayerUsesPlanningSemanticsInsteadOfTemplateAcceptanceImpacts(t *testing.T) {
	spec := compileGenericSpec(Request{TemplateID: "flutter-finance-lite", ExecutorImage: "oneappfactory/builder:local"}, "做一个待办事项 app，需要首页摘要、新建待办、任务列表和详情页，优先保证本地可用。")
	plan := buildAcceptancePlan(spec)

	overview := findAcceptancePlanItem(plan.SemanticChecks, "ac-overview")
	if overview == nil {
		t.Fatal("missing overview semantic check")
	}
	if strings.Join(overview.BindingRefs, ",") != genericSurfaceOverviewID {
		t.Fatalf("overview binding refs = %v, want [%s] even under mismatched template slot impacts", overview.BindingRefs, genericSurfaceOverviewID)
	}

	branding := findAcceptancePlanItem(plan.StructureChecks, "check-profile-open-lite-domain-branding")
	if branding == nil {
		t.Fatal("missing branding structure check")
	}
	for _, expected := range []string{publicBindingDomainCopy, publicBindingBranding} {
		if !strings.Contains(strings.Join(branding.BindingRefs, ","), expected) {
			t.Fatalf("branding binding refs = %v, want include %s under planning-driven projection", branding.BindingRefs, expected)
		}
	}

	persistence := findAcceptancePlanItem(plan.StructureChecks, "check-open-lite-local-persistence-wiring")
	if persistence == nil {
		t.Fatal("missing local persistence structure check")
	}
	if !strings.Contains(strings.Join(persistence.BindingRefs, ","), publicBindingStorage) {
		t.Fatalf("persistence binding refs = %v, want include %s under planning-driven projection", persistence.BindingRefs, publicBindingStorage)
	}

	manualReview := findAcceptancePlanItem(plan.SemanticChecks, "mrp-domain-wording")
	if manualReview == nil {
		t.Fatal("missing domain wording manual review")
	}
	manualReviewJoined := strings.Join(manualReview.BindingRefs, ",")
	for _, expected := range []string{publicBindingDomainCopy, publicBindingBranding, genericSurfaceOverviewID, genericSurfaceMutationID, genericSurfaceInspectionID} {
		if !strings.Contains(manualReviewJoined, expected) {
			t.Fatalf("manual review binding refs = %v, want include %s under planning-driven projection", manualReview.BindingRefs, expected)
		}
	}
	if manualReviewJoined == publicBindingDomainCopy {
		t.Fatalf("manual review binding refs = %v, want declared task bindings instead of point_id keyword narrowing", manualReview.BindingRefs)
	}

	counterDemo := findAcceptancePlanItem(plan.StructureChecks, "check-open-lite-counter-demo-removed")
	if counterDemo == nil {
		t.Fatal("missing counter demo structure check")
	}
	if !strings.Contains(strings.Join(counterDemo.BindingRefs, ","), publicBindingWidgetTest) {
		t.Fatalf("counter demo binding refs = %v, want include %s under planning-driven projection", counterDemo.BindingRefs, publicBindingWidgetTest)
	}
}

func TestTemplateCompileSourceVersionLayerBindsOpenLiteSlotRegistryDigest(t *testing.T) {
	version := TemplateCompileSourceVersion("flutter-open-lite", "v0.2.0")
	if !strings.HasPrefix(version, "selected-template@flutter-open-lite@v0.2.0@sha256:") {
		t.Fatalf("TemplateCompileSourceVersion = %q, want open-lite slot registry digest", version)
	}
	if digest := templateSlotRegistryDigest("flutter-open-lite"); digest == "" || !strings.HasSuffix(version, digest) {
		t.Fatalf("TemplateCompileSourceVersion = %q, want suffix %q", version, digest)
	}
	if finance := TemplateCompileSourceVersion("flutter-finance-lite", "v0.1.0"); !strings.HasPrefix(finance, "selected-template@flutter-finance-lite@v0.1.0@sha256:") {
		t.Fatalf("finance TemplateCompileSourceVersion = %q, want finance slot registry digest", finance)
	}
	if digest := templateSlotRegistryDigest("flutter-finance-lite"); digest == "" || !strings.HasSuffix(TemplateCompileSourceVersion("flutter-finance-lite", "v0.1.0"), digest) {
		t.Fatalf("finance TemplateCompileSourceVersion = %q, want suffix %q", TemplateCompileSourceVersion("flutter-finance-lite", "v0.1.0"), digest)
	}
}

func TestPlanningArtifactsLayerKeepsMultiFileCoordinationWithoutRuntimePrompt(t *testing.T) {
	spec := compileGenericSpec(Request{ExecutorImage: "oneappfactory/builder:local"}, "做一个待办事项 app，需要首页摘要、新建待办、任务列表和详情页，优先保证本地可用。")
	allocation := buildTaskAllocation(spec.TaskBundle, spec)
	plan := buildAcceptancePlan(spec)

	bindingOwners := map[string]string{}
	for _, unit := range allocation.Units {
		for _, bindingRef := range unit.BindingRefs {
			bindingOwners[bindingRef] = unit.Lane
		}
	}
	bindingChecks := map[string][]string{}
	for _, item := range append(append([]AcceptancePlanItem{}, plan.SemanticChecks...), plan.StructureChecks...) {
		for _, bindingRef := range item.BindingRefs {
			bindingChecks[bindingRef] = append(bindingChecks[bindingRef], item.CheckID)
		}
	}

	if bindingOwners[publicBindingDomainCopy] != string(appruns.TaskCategoryContent) {
		t.Fatalf("%s owner lane = %q, want content", publicBindingDomainCopy, bindingOwners[publicBindingDomainCopy])
	}
	for _, expected := range []string{"mrp-domain-wording", "check-profile-open-lite-domain-branding", "check-profile-open-lite-domain-language"} {
		if !strings.Contains(strings.Join(bindingChecks[publicBindingDomainCopy], ","), expected) {
			t.Fatalf("%s acceptance refs = %v, want include %s", publicBindingDomainCopy, bindingChecks[publicBindingDomainCopy], expected)
		}
	}

	if bindingOwners[publicBindingBranding] != string(appruns.TaskCategoryContent) {
		t.Fatalf("%s owner lane = %q, want content", publicBindingBranding, bindingOwners[publicBindingBranding])
	}
	if !strings.Contains(strings.Join(bindingChecks[publicBindingBranding], ","), "check-profile-open-lite-domain-branding") {
		t.Fatalf("%s acceptance refs = %v, want branding guard", publicBindingBranding, bindingChecks[publicBindingBranding])
	}

	if bindingOwners[publicBindingWidgetTest] != string(appruns.TaskCategoryFlow) {
		t.Fatalf("%s owner lane = %q, want flow", publicBindingWidgetTest, bindingOwners[publicBindingWidgetTest])
	}
	for _, expected := range []string{"check-open-lite-record-flow-wiring", "check-profile-open-lite-domain-language"} {
		if !strings.Contains(strings.Join(bindingChecks[publicBindingWidgetTest], ","), expected) {
			t.Fatalf("%s acceptance refs = %v, want include %s", publicBindingWidgetTest, bindingChecks[publicBindingWidgetTest], expected)
		}
	}
	if !strings.Contains(strings.Join(bindingChecks[publicBindingWidgetTest], ","), "check-open-lite-counter-demo-removed") {
		t.Fatalf("%s acceptance refs = %v, want include counter demo guard", publicBindingWidgetTest, bindingChecks[publicBindingWidgetTest])
	}
}

func findAcceptancePlanItem(items []AcceptancePlanItem, checkID string) *AcceptancePlanItem {
	for index := range items {
		if items[index].CheckID == checkID {
			return &items[index]
		}
	}
	return nil
}
