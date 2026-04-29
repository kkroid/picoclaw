package adapter

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
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
	if !containsDefaultEditPath(plan.Files, "lib/picoclaw_executor_probe.dart") {
		t.Fatalf("plan files = %+v, want fallback probe file preserved", plan.Files)
	}
}

func TestBuildDefaultEditPlanProjectsGenericBrandingBeforeBuilderRuntime(t *testing.T) {
	workspace := t.TempDir()
	copyPath := filepath.Join(workspace, "lib", "template", "open_lite_copy.dart")
	if err := os.MkdirAll(filepath.Dir(copyPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(copy) error = %v", err)
	}
	if err := os.WriteFile(copyPath, []byte("String get appTitle => 'Open Lite Seed';\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(copy) error = %v", err)
	}
	stringsPath := filepath.Join(workspace, "android", "app", "src", "main", "res", "values", "strings.xml")
	if err := os.MkdirAll(filepath.Dir(stringsPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(strings) error = %v", err)
	}
	if err := os.WriteFile(stringsPath, []byte("<resources><string name=\"app_name\">Open Lite Seed</string></resources>\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(strings) error = %v", err)
	}
	humanNotes, err := json.Marshal([]map[string]string{{
		"note_id": "note-weight-domain",
		"summary": "Builder 后续必须把中性 record 骨架收口为体重记录 app，而不是停在 open-lite 默认文案。",
	}})
	if err != nil {
		t.Fatalf("Marshal(humanNotes) error = %v", err)
	}
	run := runRecord{
		RunID:          "run-1",
		GoalSummary:    "将体重记录需求整理成基于 flutter-open-lite 的多页面 Android MVP 输入包。",
		HumanNotes:     humanNotes,
		WorkspacePath:  workspace,
		AllowedPaths:   []string{"lib/**", "test/**", "android/app/src/main/res/values/strings.xml"},
		ProtectedPaths: []string{"android/app/src/main/AndroidManifest.xml"},
		TaskBundle: []appruns.TaskBundleItem{{
			TaskID:      "task-generic-domain-copy",
			TargetPaths: []string{"lib/template/open_lite_copy.dart", "android/app/src/main/res/values/strings.xml"},
		}},
		BuilderRuntime: &appruns.BuilderRuntimePlan{
			Enabled:      true,
			DefaultModel: appruns.BuilderRuntimeModelRef{Primary: "qwen2.5-coder-14b-local"},
		},
	}

	plan, err := buildDefaultEditPlan(run)
	if err != nil {
		t.Fatalf("buildDefaultEditPlan() error = %v", err)
	}
	if !containsDefaultEditPath(plan.Files, "lib/template/open_lite_copy.dart") {
		t.Fatalf("plan files = %+v, want projected open_lite_copy.dart edit", plan.Files)
	}
	if !containsDefaultEditPath(plan.Files, "android/app/src/main/res/values/strings.xml") {
		t.Fatalf("plan files = %+v, want projected strings.xml edit", plan.Files)
	}
	var copyContent string
	var androidContent string
	for _, file := range plan.Files {
		switch file.Path {
		case "lib/template/open_lite_copy.dart":
			copyContent = file.Content
		case "android/app/src/main/res/values/strings.xml":
			androidContent = file.Content
		}
	}
	if strings.Contains(copyContent, "Open Lite Seed") || !strings.Contains(copyContent, "体重记录 App") {
		t.Fatalf("copy content = %q, want projected generic branding", copyContent)
	}
	if strings.Contains(androidContent, "Open Lite Seed") || !strings.Contains(androidContent, "体重记录 App") {
		t.Fatalf("android content = %q, want projected generic branding", androidContent)
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

func TestBuildDefaultEditPlanSkipsGenericBrandingForReferenceTemplateSeed(t *testing.T) {
	workspace := t.TempDir()
	copyPath := filepath.Join(workspace, "lib", "template", "open_lite_copy.dart")
	stringsPath := filepath.Join(workspace, "android", "app", "src", "main", "res", "values", "strings.xml")
	if err := os.MkdirAll(filepath.Dir(copyPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(copy) error = %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(stringsPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(strings) error = %v", err)
	}
	if err := os.WriteFile(copyPath, []byte("const title = 'Open Lite Seed';\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(copy) error = %v", err)
	}
	if err := os.WriteFile(stringsPath, []byte("<string name=\"app_name\">Open Lite Seed</string>\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(strings) error = %v", err)
	}
	run := runRecord{
		GoalSummary:   "将体重记录需求整理成 Android MVP 输入包",
		WorkspacePath: workspace,
		AllowedPaths:  []string{".", "lib/**", "android/**"},
		TaskBundle: []appruns.TaskBundleItem{
			{TaskID: "task-copy", TargetPaths: []string{"lib/template/open_lite_copy.dart"}},
			{TaskID: "task-android", TargetPaths: []string{"android/app/src/main/res/values/strings.xml"}},
		},
		TemplateReferenceFiles: map[string]string{"lib/template/open_lite_copy.dart": "/template/lib/template/open_lite_copy.dart"},
		BuilderRuntime: &appruns.BuilderRuntimePlan{
			Enabled:      true,
			DefaultModel: appruns.BuilderRuntimeModelRef{Primary: "qwen2.5-coder-14b-local"},
		},
	}

	plan, err := buildDefaultEditPlan(run)
	if err != nil {
		t.Fatalf("buildDefaultEditPlan() error = %v", err)
	}
	if containsDefaultEditPath(plan.Files, "lib/template/open_lite_copy.dart") {
		t.Fatalf("plan files = %+v, want no projected open_lite_copy.dart edit in reference-template mode", plan.Files)
	}
	if containsDefaultEditPath(plan.Files, "android/app/src/main/res/values/strings.xml") {
		t.Fatalf("plan files = %+v, want no projected strings.xml edit in reference-template mode", plan.Files)
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

func TestResolveBuilderRuntimePlanHonorsStrongRouteHint(t *testing.T) {
	cfg := appconfig.BuilderRuntimeConfig{
		Enabled:      true,
		DefaultModel: &appconfig.AgentModelConfig{Primary: "qwen2.5-coder-14b-local"},
		UpgradeModel: &appconfig.AgentModelConfig{Primary: "qwen2.5-coder-32b-local"},
	}

	plan := resolveBuilderRuntimePlan(cfg, []appruns.TaskBundleItem{{
		TaskID:    "task-generic-domain-models",
		TaskType:  appruns.BuilderRuntimeTaskTypeDualFileWiring,
		RouteHint: appruns.TaskRouteHintStrongModel,
	}})

	if len(plan.TaskRoutes) != 1 {
		t.Fatalf("task_routes len = %d, want 1", len(plan.TaskRoutes))
	}
	route := plan.TaskRoutes[0]
	if route.RouteSource != "route_hint" {
		t.Fatalf("route source = %q, want route_hint", route.RouteSource)
	}
	if route.Model.Primary != "qwen2.5-coder-32b-local" {
		t.Fatalf("route model = %q, want qwen2.5-coder-32b-local", route.Model.Primary)
	}
}

func TestStagedThinExecutorPrepareBuildsSequentialEditStepsForBuilderRuntime(t *testing.T) {
	executor := StagedThinExecutor{}
	root := t.TempDir()
	workspacePath := filepath.Join(root, "jobs", "job-1", "workspace")
	if err := os.MkdirAll(workspacePath, 0o755); err != nil {
		t.Fatalf("MkdirAll(workspace) error = %v", err)
	}
	run := runRecord{
		RunID:         "run-1",
		GoalSummary:   "build weight tracker app",
		WorkspacePath: workspacePath,
		AllowedPaths:  []string{"lib/**", "test/**", "android/**", "pubspec.yaml"},
		KnowledgePack: []appruns.ProfileSkill{{SkillID: "prd-to-task-bundle", UsageStage: "planning", Scope: "flutter-android-p0"}},
		TaskBundle: []appruns.TaskBundleItem{
			{TaskID: "task-create-record-model", Title: "创建记录模型", Category: appruns.TaskCategoryDomain, TaskType: appruns.BuilderRuntimeTaskTypeSingleFileEdit, TargetPaths: []string{"lib/models/record.dart"}},
			{TaskID: "task-create-summary-model", Title: "创建摘要模型", Category: appruns.TaskCategorySummary, TaskType: appruns.BuilderRuntimeTaskTypeSingleFileEdit, Dependencies: []string{"task-create-record-model"}, TargetPaths: []string{"lib/models/dashboard_summary.dart"}},
			{TaskID: "task-create-copy", Title: "创建领域文案", Category: appruns.TaskCategoryContent, TaskType: appruns.BuilderRuntimeTaskTypeSingleFileEdit, Dependencies: []string{"task-create-record-model"}, TargetPaths: []string{"lib/template/open_lite_copy.dart"}},
		},
		AcceptanceChecks: []appruns.AcceptanceCheck{{CheckID: "check-flutter-analyze", Label: "执行静态检查", Stage: appruns.StageCheap, Required: true, Commands: []string{"true"}}},
		BuilderRuntime: &appruns.BuilderRuntimePlan{
			Enabled:      true,
			DefaultModel: appruns.BuilderRuntimeModelRef{Primary: "qwen2.5-coder-32b-local"},
		},
	}
	plan, err := executor.Prepare(context.Background(), run)
	if err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}
	if len(plan.EditSteps) != 3 {
		t.Fatalf("len(plan.EditSteps) = %d, want 3", len(plan.EditSteps))
	}
	got := []string{plan.EditSteps[0].StepID, plan.EditSteps[1].StepID, plan.EditSteps[2].StepID}
	want := []string{"task-create-record-model", "task-create-summary-model", "task-create-copy"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("edit step ids = %v, want %v and preserve raw task order", got, want)
	}
	steps := plan.Steps()
	if len(steps) != 4 {
		t.Fatalf("len(plan.Steps()) = %d, want 4", len(steps))
	}
	if steps[0].StepID != "task-create-record-model" || steps[2].StepID != "task-create-copy" || steps[3].StepID != "check-flutter-analyze" {
		t.Fatalf("steps = %+v, want sequential edit steps before validation", steps)
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

func TestPlannedTaskBundleFromArtifact(t *testing.T) {
	jobRoot := t.TempDir()
	prepareDir := filepath.Join(jobRoot, "prepare")
	workspaceDir := filepath.Join(jobRoot, "workspace")
	os.MkdirAll(prepareDir, 0o755)
	os.MkdirAll(workspaceDir, 0o755)

	taJSON := `{
		"prd_digest": "sha256:test",
		"units": [
			{
				"allocation_id": "task-domain",
				"lane": "domain",
				"objective": "创建领域模型",
				"success_evidence": ["领域模型已创建"],
				"target_paths": ["lib/model.dart"]
			},
			{
				"allocation_id": "task-screen",
				"lane": "screen",
				"objective": "创建页面",
				"blocked_by": ["task-domain"],
				"success_evidence": ["页面已创建"],
				"target_paths": ["lib/page.dart"]
			}
		]
	}`
	os.WriteFile(filepath.Join(prepareDir, "task-allocation.json"), []byte(taJSON), 0o644)

	run := runRecord{
		WorkspacePath: workspaceDir,
		TaskBundle: []appruns.TaskBundleItem{
			{TaskID: "old-task", Category: appruns.TaskCategoryDomain},
		},
		KnowledgePack: []appruns.ProfileSkill{
			{SkillID: "artifact-sourced-plan"},
			{SkillID: "prd-to-task-bundle"},
		},
	}

	tasks := plannedTaskBundle(run)
	if len(tasks) != 2 {
		t.Fatalf("task count = %d, want 2", len(tasks))
	}
	// 排序后应该是 domain 先于 screen（依赖 + 类别）
	if tasks[0].TaskID != "task-domain" {
		t.Fatalf("first task = %q, want task-domain", tasks[0].TaskID)
	}
	if tasks[1].TaskID != "task-screen" {
		t.Fatalf("second task = %q, want task-screen", tasks[1].TaskID)
	}
}

func TestPlannedTaskBundleFallsBackWhenArtifactMissing(t *testing.T) {
	run := runRecord{
		WorkspacePath: "/nonexistent/workspace",
		TaskBundle: []appruns.TaskBundleItem{
			{TaskID: "fallback-task", Category: appruns.TaskCategoryDomain},
		},
		KnowledgePack: []appruns.ProfileSkill{
			{SkillID: "artifact-sourced-plan"},
		},
	}

	tasks := plannedTaskBundle(run)
	if len(tasks) != 1 || tasks[0].TaskID != "fallback-task" {
		t.Fatalf("expected fallback to run.TaskBundle, got %+v", tasks)
	}
}

func TestPlannedAcceptanceChecksFromArtifact(t *testing.T) {
	jobRoot := t.TempDir()
	prepareDir := filepath.Join(jobRoot, "prepare")
	workspaceDir := filepath.Join(jobRoot, "workspace")
	os.MkdirAll(prepareDir, 0o755)
	os.MkdirAll(workspaceDir, 0o755)

	apJSON := `{
		"structure_checks": [
			{
				"check_id": "check-analyze",
				"label": "flutter analyze",
				"stage": "cheap",
				"required": true,
				"commands": ["flutter analyze --no-fatal-infos"],
				"description": "静态分析通过"
			}
		],
		"delivery_checks": [
			{
				"check_id": "check-build",
				"label": "flutter build",
				"stage": "milestone",
				"required": true,
				"commands": ["flutter build apk --debug"],
				"description": "构建通过",
				"timeout_seconds": 300
			}
		]
	}`
	os.WriteFile(filepath.Join(prepareDir, "acceptance-plan.json"), []byte(apJSON), 0o644)

	run := runRecord{
		WorkspacePath: workspaceDir,
		AcceptanceChecks: []appruns.AcceptanceCheck{
			{CheckID: "old-check", Stage: appruns.StageMilestone},
		},
		KnowledgePack: []appruns.ProfileSkill{
			{SkillID: "artifact-sourced-plan"},
			{SkillID: "flutter-build-closure"},
		},
	}

	checks := plannedAcceptanceChecks(run)
	if len(checks) != 2 {
		t.Fatalf("check count = %d, want 2", len(checks))
	}
	// 排序后 cheap 在 milestone 之前
	if checks[0].CheckID != "check-analyze" {
		t.Fatalf("first check = %q, want check-analyze", checks[0].CheckID)
	}
	if checks[1].CheckID != "check-build" {
		t.Fatalf("second check = %q, want check-build", checks[1].CheckID)
	}
	if checks[1].TimeoutSeconds != 300 {
		t.Fatalf("timeout_seconds = %d, want 300", checks[1].TimeoutSeconds)
	}
}

func TestPlannedAcceptanceChecksFallsBackWhenArtifactMissing(t *testing.T) {
	run := runRecord{
		WorkspacePath: "/nonexistent/workspace",
		AcceptanceChecks: []appruns.AcceptanceCheck{
			{CheckID: "fallback-check", Stage: appruns.StageCheap},
		},
		KnowledgePack: []appruns.ProfileSkill{
			{SkillID: "artifact-sourced-plan"},
		},
	}

	checks := plannedAcceptanceChecks(run)
	if len(checks) != 1 || checks[0].CheckID != "fallback-check" {
		t.Fatalf("expected fallback to run.AcceptanceChecks, got %+v", checks)
	}
}
