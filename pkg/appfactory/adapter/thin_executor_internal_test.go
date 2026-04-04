package adapter

import (
	"strings"
	"testing"

	appruns "github.com/sipeed/picoclaw/pkg/appfactory/runs"
	appconfig "github.com/sipeed/picoclaw/pkg/config"
)

func TestBuildRoundInputIncludesKnowledgePack(t *testing.T) {
	run := runRecord{
		GoalSummary: "build android app",
		TaskBundle: []appruns.TaskBundleItem{
			{TaskID: "task-validation", Category: appruns.TaskCategoryValidation, Dependencies: []string{"task-flow"}},
			{TaskID: "task-domain", Category: appruns.TaskCategoryDomain},
			{TaskID: "task-flow", Category: appruns.TaskCategoryFlow, Dependencies: []string{"task-domain"}},
		},
		AcceptanceChecks: []appruns.AcceptanceCheck{
			{CheckID: "check-build", Stage: appruns.StageMilestone},
			{CheckID: "check-pub-get", Stage: appruns.StageBaseline},
			{CheckID: "check-analyze", Stage: appruns.StageCheap},
		},
		KnowledgePack: []appruns.ProfileSkill{
			{SkillID: "prd-to-task-bundle", UsageStage: "planning", Scope: "flutter-android-p0"},
			{SkillID: "flutter-mvc-template", UsageStage: "layout", Scope: "flutter-android-p0"},
			{SkillID: "flutter-build-closure", UsageStage: "closure", Scope: "flutter-android-p0"},
		},
	}

	roundInput := buildRoundInput(run)
	if len(roundInput.KnowledgePack) != 3 {
		t.Fatalf("KnowledgePack len = %d, want 3", len(roundInput.KnowledgePack))
	}
	if roundInput.KnowledgePack[0].SkillID != "prd-to-task-bundle" {
		t.Fatalf("first skill = %q, want prd-to-task-bundle", roundInput.KnowledgePack[0].SkillID)
	}
	if len(roundInput.TaskBundle) != 3 || roundInput.TaskBundle[0].TaskID != "task-domain" || roundInput.TaskBundle[1].TaskID != "task-flow" || roundInput.TaskBundle[2].TaskID != "task-validation" {
		t.Fatalf("TaskBundle = %+v, want domain -> flow -> validation order", roundInput.TaskBundle)
	}
	if len(roundInput.AcceptanceChecks) != 3 || roundInput.AcceptanceChecks[0].CheckID != "check-pub-get" || roundInput.AcceptanceChecks[2].CheckID != "check-build" {
		t.Fatalf("AcceptanceChecks = %+v, want baseline -> cheap -> milestone order", roundInput.AcceptanceChecks)
	}
}

func TestBuildDefaultEditPlanUsesProbeForFlutterWorkspace(t *testing.T) {
	run := runRecord{
		RunID:          "run-1",
		GoalSummary:    "build android app",
		AllowedPaths:   []string{"lib/**", "test/**", "pubspec.yaml"},
		ProtectedPaths: []string{"android/**", "ios/**"},
		KnowledgePack: []appruns.ProfileSkill{
			{SkillID: "flutter-mvc-template", UsageStage: "layout", Scope: "flutter-android-p0"},
			{SkillID: "builder-direct-edit", UsageStage: "edit", Scope: "flutter-android-p0"},
		},
	}

	plan, err := buildDefaultEditPlan(run)
	if err != nil {
		t.Fatalf("buildDefaultEditPlan() error = %v", err)
	}
	if !strings.Contains(plan.Summary, "output=lib/picoclaw_executor_probe.dart") {
		t.Fatalf("summary = %q, want default probe output path", plan.Summary)
	}
	if len(plan.Files) != 1 || plan.Files[0].Path != "lib/picoclaw_executor_probe.dart" {
		t.Fatalf("plan files = %+v, want only probe file", plan.Files)
	}
	if strings.Contains(plan.Files[0].Content, "BookkeepingApp") {
		t.Fatalf("probe content should not include hardcoded Flutter business shell: %q", plan.Files[0].Content)
	}
}

func TestBuildDefaultEditPlanUsesBuilderRuntimeSummaryWhenEnabled(t *testing.T) {
	run := runRecord{
		RunID:          "run-1",
		GoalSummary:    "build android app",
		AllowedPaths:   []string{"lib/**", "test/**", "pubspec.yaml"},
		ProtectedPaths: []string{"android/**", "ios/**"},
		TaskBundle: []appruns.TaskBundleItem{{
			TaskID:      "task-validation",
			TaskType:    appruns.BuilderRuntimeTaskTypeClosureRepair,
			TargetPaths: []string{"lib/main.dart", "test/widget_test.dart"},
		}},
		BuilderRuntime: &appruns.BuilderRuntimePlan{
			Enabled:      true,
			UpgradeModel: appruns.BuilderRuntimeModelRef{Primary: "qwen2.5-coder-32b-local"},
			TaskRoutes: []appruns.BuilderRuntimeTaskRoute{{
				TaskID:      "task-validation",
				TaskType:    appruns.BuilderRuntimeTaskTypeClosureRepair,
				RouteSource: "task_route",
				Model:       appruns.BuilderRuntimeModelRef{Primary: "qwen2.5-coder-32b-local"},
			}},
		},
	}

	plan, err := buildDefaultEditPlan(run)
	if err != nil {
		t.Fatalf("buildDefaultEditPlan() error = %v", err)
	}
	if strings.Contains(plan.Summary, "output=lib/picoclaw_executor_probe.dart") {
		t.Fatalf("summary = %q, want builder runtime summary without probe output", plan.Summary)
	}
	if !strings.Contains(plan.Summary, "output=builder-runtime-workspace-patch") {
		t.Fatalf("summary = %q, want builder runtime workspace patch output", plan.Summary)
	}
	if !strings.Contains(plan.Summary, "route=task_route") {
		t.Fatalf("summary = %q, want builder runtime route source", plan.Summary)
	}
	if len(plan.Files) != 1 || plan.Files[0].Path != "lib/picoclaw_executor_probe.dart" {
		t.Fatalf("plan files = %+v, want fallback probe file preserved", plan.Files)
	}
}

func TestBuildExecutionSummaryIncludesOrderedSkillFlow(t *testing.T) {
	run := runRecord{
		GoalSummary:      "build android app",
		TaskBundle:       []appruns.TaskBundleItem{{TaskID: "task-domain"}, {TaskID: "task-flow"}},
		AcceptanceChecks: []appruns.AcceptanceCheck{{CheckID: "check-1"}},
		KnowledgePack: []appruns.ProfileSkill{
			{SkillID: "builder-direct-edit", UsageStage: "edit"},
			{SkillID: "flutter-build-closure", UsageStage: "closure"},
			{SkillID: "prd-to-task-bundle", UsageStage: "planning"},
			{SkillID: "flutter-mvc-template", UsageStage: "layout"},
		},
	}

	summary := buildExecutionSummary(run)
	want := "skill_flow=planning:prd-to-task-bundle -> layout:flutter-mvc-template -> edit:builder-direct-edit -> closure:flutter-build-closure"
	if !strings.Contains(summary, want) {
		t.Fatalf("summary = %q, want %q", summary, want)
	}
	if !strings.Contains(summary, "focus_tasks=task-domain,task-flow") {
		t.Fatalf("summary = %q, want ordered focus tasks", summary)
	}
}

func TestPlannedTaskBundleOrdersByDependencyAndCategory(t *testing.T) {
	run := runRecord{
		TaskBundle: []appruns.TaskBundleItem{
			{TaskID: "task-validation", Category: appruns.TaskCategoryValidation, Dependencies: []string{"task-flow"}},
			{TaskID: "task-screen", Category: appruns.TaskCategoryScreen, Dependencies: []string{"task-domain"}},
			{TaskID: "task-domain", Category: appruns.TaskCategoryDomain},
			{TaskID: "task-storage", Category: appruns.TaskCategoryStorage, Dependencies: []string{"task-domain"}},
			{TaskID: "task-flow", Category: appruns.TaskCategoryFlow, Dependencies: []string{"task-storage", "task-screen"}},
		},
		KnowledgePack: []appruns.ProfileSkill{{SkillID: "prd-to-task-bundle", UsageStage: "planning"}},
	}

	ordered := plannedTaskBundle(run)
	got := []string{ordered[0].TaskID, ordered[1].TaskID, ordered[2].TaskID, ordered[3].TaskID, ordered[4].TaskID}
	want := []string{"task-domain", "task-storage", "task-screen", "task-flow", "task-validation"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("task order = %v, want %v", got, want)
	}
}

func TestPlannedAcceptanceChecksOrdersClosureStages(t *testing.T) {
	run := runRecord{
		AcceptanceChecks: []appruns.AcceptanceCheck{
			{CheckID: "check-build", Stage: appruns.StageMilestone},
			{CheckID: "check-analyze", Stage: appruns.StageCheap},
			{CheckID: "check-pub-get", Stage: appruns.StageBaseline},
		},
		KnowledgePack: []appruns.ProfileSkill{{SkillID: "flutter-build-closure", UsageStage: "closure"}},
	}

	ordered := plannedAcceptanceChecks(run)
	got := []string{ordered[0].CheckID, ordered[1].CheckID, ordered[2].CheckID}
	want := []string{"check-pub-get", "check-analyze", "check-build"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("check order = %v, want %v", got, want)
	}
}

func TestBuildRoundInputNormalizesTaskType(t *testing.T) {
	run := runRecord{
		GoalSummary: "build android app",
		TaskBundle: []appruns.TaskBundleItem{{
			TaskID:             "task-main",
			Category:           appruns.TaskCategoryScreen,
			TargetPaths:        []string{"lib/main.dart"},
			CompletionCriteria: []string{"screen renders"},
		}},
	}

	roundInput := buildRoundInput(run)
	if len(roundInput.TaskBundle) != 1 {
		t.Fatalf("TaskBundle len = %d, want 1", len(roundInput.TaskBundle))
	}
	if roundInput.TaskBundle[0].TaskType != appruns.BuilderRuntimeTaskTypeSingleFileEdit {
		t.Fatalf("task_type = %q, want %q", roundInput.TaskBundle[0].TaskType, appruns.BuilderRuntimeTaskTypeSingleFileEdit)
	}
}

func TestResolveBuilderRuntimePlanUsesNormalizedTaskTypeRoutes(t *testing.T) {
	cfg := appconfig.BuilderRuntimeConfig{
		Enabled:      true,
		DefaultModel: &appconfig.AgentModelConfig{Primary: "qwen2.5-coder-14b-local"},
		TaskRoutes: []appconfig.BuilderRuntimeTaskRouteConfig{{
			TaskType: "high_risk_repair",
			Model:    &appconfig.AgentModelConfig{Primary: "qwen2.5-coder-32b-local"},
		}},
	}

	plan := resolveBuilderRuntimePlan(cfg, []appruns.TaskBundleItem{{
		TaskID:             "task-validation-closure",
		Category:           appruns.TaskCategoryValidation,
		Objective:          "run flutter analyze and flutter test before apk closure",
		TargetPaths:        []string{"lib/main.dart", "test/widget_test.dart"},
		CompletionCriteria: []string{"flutter analyze passes", "flutter test passes"},
	}})

	if !plan.Enabled {
		t.Fatal("builder_runtime plan should be enabled")
	}
	if len(plan.TaskRoutes) != 1 {
		t.Fatalf("task_routes len = %d, want 1", len(plan.TaskRoutes))
	}
	route := plan.TaskRoutes[0]
	if route.TaskType != appruns.BuilderRuntimeTaskTypeClosureRepair {
		t.Fatalf("route task_type = %q, want %q", route.TaskType, appruns.BuilderRuntimeTaskTypeClosureRepair)
	}
	if route.RouteSource != "task_route" {
		t.Fatalf("route source = %q, want task_route", route.RouteSource)
	}
	if route.Model.Primary != "qwen2.5-coder-32b-local" {
		t.Fatalf("route model = %q, want qwen2.5-coder-32b-local", route.Model.Primary)
	}
}

func containsDefaultEditPath(files []defaultEditFile, target string) bool {
	for _, file := range files {
		if file.Path == target {
			return true
		}
	}
	return false
}
