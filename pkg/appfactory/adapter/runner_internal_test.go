package adapter

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	appruns "github.com/sipeed/oneappfactory/pkg/appfactory/runs"
	appconfig "github.com/sipeed/oneappfactory/pkg/config"
)

type countingBuilderRuntimePatchGenerator struct {
	calls int
}

func (generator *countingBuilderRuntimePatchGenerator) GeneratePatch(ctx context.Context, request BuilderRuntimePatchRequest) (BuilderRuntimePatchResponse, error) {
	generator.calls++
	return BuilderRuntimePatchResponse{}, errors.New("unexpected builder runtime patch request")
}

func TestBuildSmokeReportIncludesDeviceAndSmokeSections(t *testing.T) {
	report := buildSmokeReport(
		[]appruns.CheckResult{
			{CheckID: "check-adb-device-ready", Stage: appruns.StageDevice, Outcome: "passed"},
			{CheckID: "check-launch-app-and-capture-logcat", Stage: appruns.StageSmoke, Outcome: "passed"},
		},
		[]appruns.CheckResult{{CheckID: "check-install-release-apk", Stage: appruns.StageDevice, Outcome: "failed"}},
		[]appruns.DeviceFailureCategoryStat{{
			Category:      "device_check_failed:apk_install_failed",
			FailureDomain: "device",
			Count:         1,
		}},
	)
	for _, want := range []string{"## 设备验证", "check-adb-device-ready", "check-install-release-apk", "## 冒烟检查", "check-launch-app-and-capture-logcat", "## 设备失败类别", "device_check_failed:apk_install_failed"} {
		if !strings.Contains(report, want) {
			t.Fatalf("buildSmokeReport() = %q, want substring %q", report, want)
		}
	}
}

func TestEnrichRunRecordWithSemanticChecksInjectsAcceptancePlanChecks(t *testing.T) {
	jobRoot := t.TempDir()
	workspacePath := filepath.Join(jobRoot, "workspace")
	prepareDir := filepath.Join(jobRoot, "prepare")
	if err := os.MkdirAll(workspacePath, 0o755); err != nil {
		t.Fatalf("MkdirAll(workspace) error = %v", err)
	}
	if err := os.MkdirAll(prepareDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(prepare) error = %v", err)
	}
	contextFilesJSON, err := json.Marshal(appruns.ContextFiles{SupportingFiles: []string{"acceptance-plan.json"}})
	if err != nil {
		t.Fatalf("Marshal(context files) error = %v", err)
	}
	builderInput := map[string]any{"context_files": json.RawMessage(contextFilesJSON)}
	builderInputJSON, err := json.Marshal(builderInput)
	if err != nil {
		t.Fatalf("Marshal(builder input) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(prepareDir, "builder-input.json"), builderInputJSON, 0o600); err != nil {
		t.Fatalf("WriteFile(builder-input.json) error = %v", err)
	}
	acceptancePlanJSON := []byte(`{"semantic_checks":[{"check_id":"ac-overview","label":"首页摘要完整","description":"首页必须保留摘要字段","stage":"cheap","commands":["grep -ER 'title|inbox_count' lib test >/dev/null 2>&1"],"required":true,"field_refs":["title","inbox_count"],"evidence_pattern":"title|inbox_count"}]}`)
	if err := os.WriteFile(filepath.Join(prepareDir, "acceptance-plan.json"), acceptancePlanJSON, 0o600); err != nil {
		t.Fatalf("WriteFile(acceptance-plan.json) error = %v", err)
	}

	runner := NewRunnerWithBackend(nil)
	run, err := runner.enrichRunRecordWithSemanticChecks(runRecord{
		WorkspacePath:    workspacePath,
		AcceptanceChecks: []appruns.AcceptanceCheck{{CheckID: "check-structure", Label: "结构检查", Stage: appruns.StageCheap, Required: true, Commands: []string{"flutter analyze"}}},
	})
	if err != nil {
		t.Fatalf("enrichRunRecordWithSemanticChecks() error = %v", err)
	}
	if len(run.SemanticChecks) != 1 {
		t.Fatalf("len(run.SemanticChecks) = %d, want 1", len(run.SemanticChecks))
	}
	if len(run.AcceptanceChecks) != 2 {
		t.Fatalf("len(run.AcceptanceChecks) = %d, want 2", len(run.AcceptanceChecks))
	}
	if run.AcceptanceChecks[1].CheckID != "ac-overview" {
		t.Fatalf("run.AcceptanceChecks[1].CheckID = %q, want ac-overview", run.AcceptanceChecks[1].CheckID)
	}
	if len(run.AcceptanceChecks[1].Commands) != 1 || !strings.Contains(run.AcceptanceChecks[1].Commands[0], "grep -ER") {
		t.Fatalf("run.AcceptanceChecks[1].Commands = %v, want semantic grep command", run.AcceptanceChecks[1].Commands)
	}
}

func TestDiagnoseExecutionStepFailureAllowsResumeForBuilderRuntimeModelRequestFailure(t *testing.T) {
	step := ExecutionStep{StepID: "thin-prepare", Stage: appruns.StageThinPrepare}
	diagnosis := diagnoseExecutionStepFailure(step, &builderRuntimeExecutionError{
		summary:            "builder runtime model request failed: context deadline exceeded",
		recoverySuggestion: "fix model connectivity or route configuration before rerun",
		signature:          "builder_runtime_model_request_failed",
		preserveWorkspace:  true,
		resumeAllowed:      true,
		policy: repairFailurePolicy{
			PreserveWorkspace:   true,
			ResumeAllowed:       true,
			RequiresHumanReview: true,
		},
	}, "/tmp/workspace", "/tmp/log", false)
	if diagnosis.NextAction != appruns.ControlActionResume {
		t.Fatalf("NextAction = %q, want resume", diagnosis.NextAction)
	}
	if !diagnosis.ResumeAllowed || !diagnosis.PreserveWorkspace {
		t.Fatalf("diagnosis = %+v, want resumable preserved workspace failure", diagnosis)
	}
}

func TestDiagnoseExecutionStepFailureClassifiesGradleDependencyDownloadFailure(t *testing.T) {
	workspacePath := filepath.Join(t.TempDir(), "workspace")
	if err := os.MkdirAll(workspacePath, 0o755); err != nil {
		t.Fatalf("MkdirAll(workspace) error = %v", err)
	}
	logPath := filepath.Join(t.TempDir(), "builder.log")
	content := strings.Join([]string{
		"A problem occurred configuring project ':path_provider_android'.",
		"> Could not resolve all artifacts for configuration ':path_provider_android:classpath'.",
		"   > Could not get resource 'https://dl.google.com/dl/android/maven2/com/android/tools/build/gradle/8.12.1/gradle-8.12.1.pom'.",
		"      > Could not GET 'https://dl.google.com/dl/android/maven2/com/android/tools/build/gradle/8.12.1/gradle-8.12.1.pom'.",
		"         > dl.google.com: Temporary failure in name resolution",
	}, "\n")
	if err := os.WriteFile(logPath, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile(logPath) error = %v", err)
	}

	diagnosis := diagnoseExecutionStepFailure(ExecutionStep{
		StepID: "check-flutter-build-apk",
		Stage:  appruns.StageMilestone,
		Check: &CheckExecutionPreview{
			CheckID:  "check-flutter-build-apk",
			Stage:    appruns.StageMilestone,
			Commands: []string{"flutter build apk --release --no-pub"},
		},
	}, errors.New("exit status 1"), workspacePath, logPath, false)

	if diagnosis.Signature != "environment_check_failed:gradle_dependency_download_failed" {
		t.Fatalf("diagnosis.Signature = %q, want gradle dependency signature", diagnosis.Signature)
	}
	if diagnosis.RecoverySuggestion != "restore network/DNS access to Gradle Maven repositories or prewarm the builder cache before rerun" {
		t.Fatalf("diagnosis.RecoverySuggestion = %q, want Gradle dependency guidance", diagnosis.RecoverySuggestion)
	}
	if diagnosis.ResumeAllowed || diagnosis.NextAction != appruns.ControlActionStop {
		t.Fatalf("diagnosis action = %+v, want non-resumable environment stop", diagnosis)
	}
}

func TestMergeRoundStateMetadataPreservesTaskStatuses(t *testing.T) {
	base := buildRoundState(
		[]appruns.RoundPhase{appruns.RoundPhaseInspect, appruns.RoundPhaseEdit, appruns.RoundPhaseFinalize},
		appruns.RoundPhaseFinalize,
		appruns.ControlActionResume,
		true,
		true,
	)
	existing := &appruns.RoundState{
		CurrentTaskID: "task-create-home-controller",
		TaskStatuses: map[string]appruns.BuilderRuntimeTaskStatus{
			"task-create-record-model":  appruns.BuilderRuntimeTaskStatusValidated,
			"task-create-summary-model": appruns.BuilderRuntimeTaskStatusValidated,
		},
	}
	merged := mergeRoundStateMetadata(base, existing, "task-create-form-controller")
	if merged.CurrentTaskID != "task-create-form-controller" {
		t.Fatalf("CurrentTaskID = %q, want task-create-form-controller", merged.CurrentTaskID)
	}
	if got := merged.TaskStatuses["task-create-record-model"]; got != appruns.BuilderRuntimeTaskStatusValidated {
		t.Fatalf("record-model status = %q, want validated", got)
	}
	if merged.NextAction != appruns.ControlActionResume || !merged.ResumeAllowed {
		t.Fatalf("merged state = %+v, want resumable failure state", merged)
	}
}

func TestHeartbeatCheckpointKeyPrefersCurrentTaskID(t *testing.T) {
	step := ExecutionStep{StepID: "task-create-record-model", Stage: appruns.StageThinPrepare}
	roundState := &appruns.RoundState{CurrentTaskID: "task-create-home-page"}
	if got := heartbeatCheckpointKey(step, roundState); got != "task-create-home-page" {
		t.Fatalf("heartbeatCheckpointKey() = %q, want task-create-home-page", got)
	}
}

func TestBuilderRuntimeTaskSummaryUsesTaskBundleMetadata(t *testing.T) {
	tasks := []appruns.TaskBundleItem{{TaskID: "task-create-home-page", Title: "创建首页页面"}}
	if got := builderRuntimeTaskSummary(tasks, "task-create-home-page"); got != "builder-runtime task task-create-home-page: 创建首页页面" {
		t.Fatalf("builderRuntimeTaskSummary() = %q", got)
	}
}

func TestBuildExecutionReportIncludesSemanticValidationSection(t *testing.T) {
	report := buildExecutionReport(
		RoundPlan{Summary: "plan"},
		runRecord{
			GoalSummary: "完成待办 app",
			SemanticChecks: []runtimeAcceptancePlanItem{{
				CheckID:     "ac-overview",
				Label:       "首页摘要完整",
				Description: "首页必须保留摘要字段",
				FieldRefs:   []string{"title", "inbox_count"},
			}},
		},
		[]appruns.CheckResult{{CheckID: "ac-overview", Stage: appruns.StageCheap, Outcome: "passed", Details: "semantic grep matched in workspace"}},
		nil,
		"",
	)
	for _, want := range []string{"## 语义验收", "通过：首页摘要完整", "fields: title, inbox_count", "semantic grep matched in workspace"} {
		if !strings.Contains(report, want) {
			t.Fatalf("buildExecutionReport() = %q, want substring %q", report, want)
		}
	}
}

func TestBuildValidationResultsAddsSemanticReviewEvidencePaths(t *testing.T) {
	results := buildValidationResults(
		[]appruns.CheckResult{{CheckID: "ac-overview", Label: "首页摘要完整", Stage: appruns.StageCheap, Outcome: "passed", Details: "semantic grep matched", EvidencePaths: []string{"jobs/job-1/run.log"}}},
		nil,
		[]runtimeAcceptancePlanItem{{CheckID: "ac-overview", Label: "首页摘要完整"}},
		[]string{"reports/build-report.md", "reports/smoke-test-report.md", "reports/device-screenshot.png"},
	)
	if len(results) != 1 {
		t.Fatalf("len(results) = %d, want 1", len(results))
	}
	for _, want := range []string{"jobs/job-1/run.log", "reports/build-report.md", "reports/smoke-test-report.md", "reports/device-screenshot.png"} {
		if !containsString(results[0].EvidencePaths, want) {
			t.Fatalf("results[0].EvidencePaths = %v, want include %q", results[0].EvidencePaths, want)
		}
	}
}

func TestBuildSemanticReviewHumanActionUsesReviewInputs(t *testing.T) {
	action := buildSemanticReviewHumanAction(
		[]runtimeAcceptancePlanItem{{CheckID: "ac-overview", Label: "首页摘要完整"}},
		[]string{"reports/build-report.md", "reports/device-logcat.txt", "reports/device-screenshot.png"},
	)
	if action == nil {
		t.Fatal("buildSemanticReviewHumanAction() = nil, want action")
	}
	if action.ActionID != "review-semantic-acceptance" {
		t.Fatalf("action.ActionID = %q, want review-semantic-acceptance", action.ActionID)
	}
	for _, want := range []string{"reports/build-report.md", "reports/device-logcat.txt", "reports/device-screenshot.png"} {
		if !containsString(action.RequiredInputs, want) {
			t.Fatalf("action.RequiredInputs = %v, want include %q", action.RequiredInputs, want)
		}
	}
}

func containsString(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}

func TestBuildDeviceFailureCategoryStatsAggregatesDeviceFailures(t *testing.T) {
	stats := buildDeviceFailureCategoryStats([]appruns.FailureSignature{
		{Signature: "device_check_failed:adb_device_unavailable", Count: 1, LastStage: appruns.StageDevice},
		{Signature: "environment_check_failed:release_apk_missing", Count: 1, LastStage: appruns.StageDevice},
		{Signature: "device_check_failed:adb_device_unavailable", Count: 2, LastStage: appruns.StageSmoke},
		{Signature: "runner_exit_nonzero", Count: 1, LastStage: appruns.StageBaseline},
	})
	if len(stats) != 2 {
		t.Fatalf("len(stats) = %d, want 2", len(stats))
	}
	if stats[0].Category != "device_check_failed:adb_device_unavailable" || stats[0].FailureDomain != "device" || stats[0].Count != 3 {
		t.Fatalf("stats[0] = %+v, want adb device unavailable x3", stats[0])
	}
	if stats[1].Category != "environment_check_failed:release_apk_missing" || stats[1].FailureDomain != "environment" || stats[1].Count != 1 {
		t.Fatalf("stats[1] = %+v, want release_apk_missing x1", stats[1])
	}
}

func TestBuildRuntimeArtifactOutputsIncludesReportsAndOptionalArtifacts(t *testing.T) {
	jobRoot := t.TempDir()
	workspacePath := filepath.Join(jobRoot, "workspace")
	reportsDir := filepath.Join(jobRoot, "reports")
	if err := os.MkdirAll(filepath.Join(workspacePath, "build", "app", "outputs", "flutter-apk"), 0o755); err != nil {
		t.Fatalf("MkdirAll(apk) error = %v", err)
	}
	if err := os.MkdirAll(reportsDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(reports) error = %v", err)
	}
	releaseAPKPath := filepath.Join(workspacePath, "build", "app", "outputs", "flutter-apk", "app-release.apk")
	changeSummaryPath := filepath.Join(reportsDir, "change-summary.md")
	buildReportPath := filepath.Join(reportsDir, "build-report.md")
	smokeReportPath := filepath.Join(reportsDir, "smoke-test-report.md")
	deviceLogcatPath := filepath.Join(reportsDir, "device-logcat.txt")
	deviceScreenshotPath := filepath.Join(reportsDir, "device-screenshot.png")
	for _, item := range []string{releaseAPKPath, changeSummaryPath, buildReportPath, smokeReportPath, deviceLogcatPath, deviceScreenshotPath} {
		if err := os.WriteFile(item, []byte("ok\n"), 0o600); err != nil {
			t.Fatalf("WriteFile(%s) error = %v", item, err)
		}
	}

	run := runRecord{
		RunID:         "run-1",
		JobID:         "job-1",
		WorkspacePath: workspacePath,
		LogPath:       "jobs/job-1/runs/run-1/run.log",
	}
	items, primaryOutputs := buildRuntimeArtifactOutputs(run, jobRoot, changeSummaryPath, buildReportPath, smokeReportPath)
	if len(items) != 7 {
		t.Fatalf("artifact items len = %d, want 7", len(items))
	}
	found := map[string]bool{}
	for _, item := range items {
		found[item.ArtifactID] = true
	}
	for _, want := range []string{"run-log", "change-summary", "build-report", "smoke-test-report", "release-apk", "device-logcat", "device-screenshot"} {
		if !found[want] {
			t.Fatalf("artifact %q missing from %+v", want, items)
		}
	}
	if len(primaryOutputs) != 2 {
		t.Fatalf("primary outputs len = %d, want 2", len(primaryOutputs))
	}
	if primaryOutputs[1].ArtifactID != "release-apk" {
		t.Fatalf("primary output[1] = %+v, want release-apk", primaryOutputs[1])
	}
}

func TestBuildValidationRepairTaskBundlePreservesOriginalTasks(t *testing.T) {
	bundle := buildValidationRepairTaskBundle([]appruns.TaskBundleItem{
		{TaskID: "task-generic-domain-models", TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, RouteHint: appruns.TaskRouteHintStrongModel},
		{TaskID: "task-generic-summary-remap", TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, RouteHint: appruns.TaskRouteHintStrongModel},
	}, appruns.TaskBundleItem{TaskID: "repair-check-flutter-analyze", TaskType: appruns.BuilderRuntimeTaskTypeAnalyzeRepair})
	if len(bundle) != 3 {
		t.Fatalf("bundle len = %d, want 3", len(bundle))
	}
	if bundle[0].TaskID != "repair-check-flutter-analyze" {
		t.Fatalf("bundle[0].TaskID = %q, want repair-check-flutter-analyze", bundle[0].TaskID)
	}
	if bundle[1].TaskID != "task-generic-domain-models" {
		t.Fatalf("bundle[1].TaskID = %q, want task-generic-domain-models", bundle[1].TaskID)
	}
	if bundle[2].TaskID != "task-generic-summary-remap" {
		t.Fatalf("bundle[2].TaskID = %q, want task-generic-summary-remap", bundle[2].TaskID)
	}
}

func TestExtractValidationFailurePathsDeduplicatesDartFiles(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "builder.log")
	content := strings.Join([]string{
		"lib/controllers/record_form_controller.dart:11:24: Error: The getter 'title' isn't defined for the type 'AppRecord'.",
		"lib/views/home_page.dart:20:10: Error: The getter 'totalCount' isn't defined for the type 'DashboardSummary'.",
		"lib/controllers/record_form_controller.dart:42:7: Error: Too many positional arguments.",
	}, "\n")
	if err := os.WriteFile(logPath, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile(logPath) error = %v", err)
	}
	paths := extractValidationFailurePaths(logPath)
	if len(paths) != 2 {
		t.Fatalf("len(paths) = %d, want 2; paths=%v", len(paths), paths)
	}
	if paths[0] != "lib/controllers/record_form_controller.dart" {
		t.Fatalf("paths[0] = %q, want lib/controllers/record_form_controller.dart", paths[0])
	}
	if paths[1] != "lib/views/home_page.dart" {
		t.Fatalf("paths[1] = %q, want lib/views/home_page.dart", paths[1])
	}
}

func TestExtractValidationFailurePathsIgnoresPackageImportPseudoPaths(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "builder.log")
	content := strings.Join([]string{
		"test/widget_test.dart:2:8: Error: Error when reading 'package:flutter_test/flutter_test.dart': No such file or directory",
		"import 'package:flutter_test/flutter_test.dart';",
	}, "\n")
	if err := os.WriteFile(logPath, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile(logPath) error = %v", err)
	}
	paths := extractValidationFailurePaths(logPath)
	if len(paths) != 1 {
		t.Fatalf("len(paths) = %d, want 1; paths=%v", len(paths), paths)
	}
	if paths[0] != "test/widget_test.dart" {
		t.Fatalf("paths[0] = %q, want test/widget_test.dart", paths[0])
	}
}

func TestExpandValidationFailurePathsWithLocalImportsDropsMissingPseudoPaths(t *testing.T) {
	workspacePath := t.TempDir()
	testDir := filepath.Join(workspacePath, "test")
	if err := os.MkdirAll(testDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(testDir) error = %v", err)
	}
	widgetPath := filepath.Join(testDir, "widget_test.dart")
	if err := os.WriteFile(widgetPath, []byte("void main() {}\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(widget_test.dart) error = %v", err)
	}

	paths := expandValidationFailurePathsWithLocalImports(workspacePath, []string{"test/flutter_test.dart", "test/widget_test.dart"})
	if len(paths) != 1 {
		t.Fatalf("len(paths) = %d, want 1; paths=%v", len(paths), paths)
	}
	if paths[0] != "test/widget_test.dart" {
		t.Fatalf("paths[0] = %q, want test/widget_test.dart", paths[0])
	}
}

func TestExtractValidationFailurePathsFromContentDeduplicatesDartFiles(t *testing.T) {
	paths := extractValidationFailurePathsFromContent(strings.Join([]string{
		"lib/views/home_page.dart:20:10: Error: The getter 'totalCount' isn't defined for the type 'DashboardSummary'.",
		"lib/views/home_page.dart:21:10: Error: The getter 'inboxCount' isn't defined for the type 'DashboardSummary'.",
		"lib/repositories/record_repository.dart:12:7: Error: Too many positional arguments.",
	}, "\n"))
	want := []string{"lib/repositories/record_repository.dart", "lib/views/home_page.dart"}
	if len(paths) != len(want) {
		t.Fatalf("len(paths) = %d, want %d; paths=%v", len(paths), len(want), paths)
	}
	for idx, item := range want {
		if paths[idx] != item {
			t.Fatalf("paths[%d] = %q, want %q", idx, paths[idx], item)
		}
	}
}

func TestExtractValidationFailureContextForAnalyzeSkipsEarlierPatchPaths(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "builder.log")
	content := strings.Join([]string{
		`{"operations":[{"write_file":"lib/models/entry.dart"},{"write_file":"lib/models/summary.dart"}]}`,
		"some unrelated tail",
		"Analyzing workspace...",
		"error • The getter 'entryCount' isn't defined for the type 'Summary' • lib/views/home_page.dart:107:38 • undefined_getter",
		"1 issue found. (ran in 1.2s)",
	}, "\n")
	if err := os.WriteFile(logPath, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile(logPath) error = %v", err)
	}
	context := extractValidationFailureContext(logPath, "check-flutter-analyze")
	paths := extractValidationFailurePathsFromContent(context)
	want := []string{"lib/views/home_page.dart"}
	if len(paths) != len(want) {
		t.Fatalf("len(paths) = %d, want %d; paths=%v; context=%q", len(paths), len(want), paths, context)
	}
	for idx, item := range want {
		if paths[idx] != item {
			t.Fatalf("paths[%d] = %q, want %q", idx, paths[idx], item)
		}
	}
}

func TestExtractLastFlutterAnalyzeFailureBlockReturnsLastAnalyzeSection(t *testing.T) {
	block := extractLastFlutterAnalyzeFailureBlock(strings.Join([]string{
		"=== builder-runtime model: foo ===",
		"Analyzing workspace...",
		"  error • old failure • lib/models/entry.dart:1:1 • old_code",
		"1 issue found. (ran in 0.5s)",
		"=== builder-runtime model: bar ===",
		"Analyzing workspace...",
		"  error • current failure • lib/views/home_page.dart:10:2 • undefined_getter",
		"  error • current test failure • test/widget_test.dart:8:14 • non_type_as_type_argument",
		"2 issues found. (ran in 1.5s)",
	}, "\n"))
	if strings.Contains(block, "lib/models/entry.dart") {
		t.Fatalf("block should not include earlier analyze section: %q", block)
	}
	for _, want := range []string{"Analyzing workspace...", "lib/views/home_page.dart", "test/widget_test.dart", "2 issues found."} {
		if !strings.Contains(block, want) {
			t.Fatalf("block missing %q: %q", want, block)
		}
	}
}

func TestRepairFlutterAnalyzeUnusedImportsRemovesImportWhenItIsTheOnlyIssue(t *testing.T) {
	workspacePath := t.TempDir()
	testPath := filepath.Join(workspacePath, "test", "widget_test.dart")
	if err := os.MkdirAll(filepath.Dir(testPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(testPath) error = %v", err)
	}
	original := strings.Join([]string{
		"import 'package:flutter_test/flutter_test.dart';",
		"import 'package:bookkeeping_lite/models/entry.dart';",
		"",
		"void main() {}",
	}, "\n") + "\n"
	if err := os.WriteFile(testPath, []byte(original), 0o600); err != nil {
		t.Fatalf("WriteFile(testPath) error = %v", err)
	}
	logPath := filepath.Join(t.TempDir(), "builder.log")
	log := strings.Join([]string{
		"Analyzing workspace...",
		"warning • Unused import: 'package:bookkeeping_lite/models/entry.dart' • test/widget_test.dart:2:8 • unused_import",
		"1 issue found. (ran in 1.3s)",
	}, "\n")
	if err := os.WriteFile(logPath, []byte(log), 0o600); err != nil {
		t.Fatalf("WriteFile(logPath) error = %v", err)
	}

	repaired, err := repairFlutterAnalyzeUnusedImports(workspacePath, logPath)
	if err != nil {
		t.Fatalf("repairFlutterAnalyzeUnusedImports() error = %v", err)
	}
	if !repaired {
		t.Fatal("repairFlutterAnalyzeUnusedImports() repaired = false, want true")
	}
	updated, err := os.ReadFile(testPath)
	if err != nil {
		t.Fatalf("ReadFile(testPath) error = %v", err)
	}
	if strings.Contains(string(updated), "package:bookkeeping_lite/models/entry.dart") {
		t.Fatalf("unused import still present after repair: %q", string(updated))
	}
	if !strings.Contains(string(updated), "import 'package:flutter_test/flutter_test.dart';") {
		t.Fatalf("expected remaining imports to stay intact: %q", string(updated))
	}
}

func TestRepairFlutterAnalyzeUnusedImportsSkipsMixedAnalyzeFailures(t *testing.T) {
	workspacePath := t.TempDir()
	testPath := filepath.Join(workspacePath, "test", "widget_test.dart")
	if err := os.MkdirAll(filepath.Dir(testPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(testPath) error = %v", err)
	}
	original := "import 'package:bookkeeping_lite/models/entry.dart';\nvoid main() {}\n"
	if err := os.WriteFile(testPath, []byte(original), 0o600); err != nil {
		t.Fatalf("WriteFile(testPath) error = %v", err)
	}
	logPath := filepath.Join(t.TempDir(), "builder.log")
	log := strings.Join([]string{
		"Analyzing workspace...",
		"error • Missing class • lib/main.dart:1:1 • undefined_class",
		"warning • Unused import: 'package:bookkeeping_lite/models/entry.dart' • test/widget_test.dart:1:8 • unused_import",
		"2 issues found. (ran in 1.3s)",
	}, "\n")
	if err := os.WriteFile(logPath, []byte(log), 0o600); err != nil {
		t.Fatalf("WriteFile(logPath) error = %v", err)
	}

	repaired, err := repairFlutterAnalyzeUnusedImports(workspacePath, logPath)
	if err != nil {
		t.Fatalf("repairFlutterAnalyzeUnusedImports() error = %v", err)
	}
	if repaired {
		t.Fatal("repairFlutterAnalyzeUnusedImports() repaired = true, want false")
	}
	updated, err := os.ReadFile(testPath)
	if err != nil {
		t.Fatalf("ReadFile(testPath) error = %v", err)
	}
	if string(updated) != original {
		t.Fatalf("file changed for mixed failure block: got %q want %q", string(updated), original)
	}
}

func TestRepairFlutterAnalyzeDeprecatedWithOpacityRewritesCurrentFile(t *testing.T) {
	workspacePath := t.TempDir()
	homePagePath := filepath.Join(workspacePath, "lib", "views", "home_page.dart")
	if err := os.MkdirAll(filepath.Dir(homePagePath), 0o755); err != nil {
		t.Fatalf("MkdirAll(homePagePath) error = %v", err)
	}
	original := strings.Join([]string{
		"import 'package:flutter/material.dart';",
		"",
		"class HomePage extends StatelessWidget {",
		"  const HomePage({super.key});",
		"",
		"  @override",
		"  Widget build(BuildContext context) {",
		"    return Container(color: Colors.white.withOpacity(0.14));",
		"  }",
		"}",
	}, "\n") + "\n"
	if err := os.WriteFile(homePagePath, []byte(original), 0o600); err != nil {
		t.Fatalf("WriteFile(homePagePath) error = %v", err)
	}
	logPath := filepath.Join(t.TempDir(), "builder.log")
	log := strings.Join([]string{
		"Analyzing workspace...",
		"info • 'withOpacity' is deprecated and shouldn't be used. Use .withValues() to avoid precision loss • lib/views/home_page.dart:8:41 • deprecated_member_use",
		"1 issue found. (ran in 1.2s)",
	}, "\n")
	if err := os.WriteFile(logPath, []byte(log), 0o600); err != nil {
		t.Fatalf("WriteFile(logPath) error = %v", err)
	}

	repaired, err := repairFlutterAnalyzeDeprecatedWithOpacity(workspacePath, logPath)
	if err != nil {
		t.Fatalf("repairFlutterAnalyzeDeprecatedWithOpacity() error = %v", err)
	}
	if !repaired {
		t.Fatal("repairFlutterAnalyzeDeprecatedWithOpacity() repaired = false, want true")
	}
	updated, err := os.ReadFile(homePagePath)
	if err != nil {
		t.Fatalf("ReadFile(homePagePath) error = %v", err)
	}
	if strings.Contains(string(updated), ".withOpacity(") {
		t.Fatalf("withOpacity still present after repair: %q", string(updated))
	}
	if !strings.Contains(string(updated), ".withValues(alpha: 0.14)") {
		t.Fatalf("withValues replacement missing after repair: %q", string(updated))
	}
}

func TestRepairFlutterAnalyzeDeprecatedWithOpacitySkipsMixedAnalyzeFailures(t *testing.T) {
	workspacePath := t.TempDir()
	homePagePath := filepath.Join(workspacePath, "lib", "views", "home_page.dart")
	if err := os.MkdirAll(filepath.Dir(homePagePath), 0o755); err != nil {
		t.Fatalf("MkdirAll(homePagePath) error = %v", err)
	}
	original := "final color = Colors.white.withOpacity(0.14);\n"
	if err := os.WriteFile(homePagePath, []byte(original), 0o600); err != nil {
		t.Fatalf("WriteFile(homePagePath) error = %v", err)
	}
	logPath := filepath.Join(t.TempDir(), "builder.log")
	log := strings.Join([]string{
		"Analyzing workspace...",
		"info • 'withOpacity' is deprecated and shouldn't be used. Use .withValues() to avoid precision loss • lib/views/home_page.dart:1:27 • deprecated_member_use",
		"error • Undefined name 'openLite0Copy' • lib/views/home_page.dart:2:16 • undefined_identifier",
		"2 issues found. (ran in 1.4s)",
	}, "\n")
	if err := os.WriteFile(logPath, []byte(log), 0o600); err != nil {
		t.Fatalf("WriteFile(logPath) error = %v", err)
	}

	repaired, err := repairFlutterAnalyzeDeprecatedWithOpacity(workspacePath, logPath)
	if err != nil {
		t.Fatalf("repairFlutterAnalyzeDeprecatedWithOpacity() error = %v", err)
	}
	if repaired {
		t.Fatal("repairFlutterAnalyzeDeprecatedWithOpacity() repaired = true, want false")
	}
	updated, err := os.ReadFile(homePagePath)
	if err != nil {
		t.Fatalf("ReadFile(homePagePath) error = %v", err)
	}
	if string(updated) != original {
		t.Fatalf("file changed for mixed failure block: got %q want %q", string(updated), original)
	}
}

func TestTryAutomaticValidationRepairRunsSequentialDeterministicRepairsBeforeBuilderRuntime(t *testing.T) {
	workspacePath := t.TempDir()
	homePagePath := filepath.Join(workspacePath, "lib", "views", "home_page.dart")
	if err := os.MkdirAll(filepath.Dir(homePagePath), 0o755); err != nil {
		t.Fatalf("MkdirAll(homePagePath) error = %v", err)
	}
	homePage := strings.Join([]string{
		"import 'package:flutter/material.dart';",
		"",
		"class HomePage extends StatelessWidget {",
		"  const HomePage({super.key});",
		"",
		"  @override",
		"  Widget build(BuildContext context) {",
		"    return Container(color: Colors.white.withOpacity(0.14));",
		"  }",
		"}",
	}, "\n") + "\n"
	if err := os.WriteFile(homePagePath, []byte(homePage), 0o600); err != nil {
		t.Fatalf("WriteFile(homePagePath) error = %v", err)
	}
	testPath := filepath.Join(workspacePath, "test", "widget_test.dart")
	if err := os.MkdirAll(filepath.Dir(testPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(testPath) error = %v", err)
	}
	testFile := strings.Join([]string{
		"import 'package:flutter_test/flutter_test.dart';",
		"import 'package:flutter_open_lite/models/record.dart';",
		"",
		"void main() {}",
	}, "\n") + "\n"
	if err := os.WriteFile(testPath, []byte(testFile), 0o600); err != nil {
		t.Fatalf("WriteFile(testPath) error = %v", err)
	}
	logPath := filepath.Join(t.TempDir(), "builder.log")
	command := exec.Command("/bin/sh", "-c", strings.Join([]string{
		"if grep -qF \"package:flutter_open_lite/models/record.dart\" test/widget_test.dart; then",
		"  cat <<'EOF'",
		"Analyzing workspace...",
		"warning • Unused import: 'package:flutter_open_lite/models/record.dart' • test/widget_test.dart:2:8 • unused_import",
		"info • 'withOpacity' is deprecated and shouldn't be used. Use .withValues() to avoid precision loss • lib/views/home_page.dart:8:41 • deprecated_member_use",
		"2 issues found. (ran in 1.4s)",
		"EOF",
		"  exit 1",
		"fi",
		"if grep -qF \".withOpacity(\" lib/views/home_page.dart; then",
		"  cat <<'EOF'",
		"Analyzing workspace...",
		"info • 'withOpacity' is deprecated and shouldn't be used. Use .withValues() to avoid precision loss • lib/views/home_page.dart:8:41 • deprecated_member_use",
		"1 issue found. (ran in 1.2s)",
		"EOF",
		"  exit 1",
		"fi",
		"exit 0",
	}, "\n"))
	command.Dir = workspacePath
	step := ExecutionStep{
		StepID:  "check-flutter-analyze",
		Stage:   appruns.StageCheap,
		Summary: "flutter analyze passes",
		Command: command,
		Check: &CheckExecutionPreview{
			CheckID:  "check-flutter-analyze",
			Label:    "Flutter Analyze",
			Stage:    appruns.StageCheap,
			Required: true,
		},
	}
	runner := NewRunnerWithBackend(nil)
	patchGenerator := &countingBuilderRuntimePatchGenerator{}
	runner.PatchGenerator = patchGenerator
	run := runRecord{
		WorkspacePath: workspacePath,
		LogPath:       logPath,
		TaskBundle: []appruns.TaskBundleItem{{
			TaskID:      "repair-check-flutter-analyze",
			Title:       "Repair Flutter Analyze",
			Category:    appruns.TaskCategoryValidation,
			TaskType:    appruns.BuilderRuntimeTaskTypeAnalyzeRepair,
			Objective:   "repair flutter analyze",
			TargetPaths: []string{"lib/views/home_page.dart", "test/widget_test.dart"},
			CompletionCriteria: []string{
				"check-flutter-analyze passes",
			},
		}},
		BuilderRuntime: &appruns.BuilderRuntimePlan{
			Enabled:      true,
			DefaultModel: appruns.BuilderRuntimeModelRef{Primary: "stub-model"},
			TaskRoutes: []appruns.BuilderRuntimeTaskRoute{{
				TaskType:    appruns.BuilderRuntimeTaskTypeAnalyzeRepair,
				RouteSource: "task_route",
				Model:       appruns.BuilderRuntimeModelRef{Primary: "stub-model"},
			}},
		},
	}
	if err := runner.runStep(step, workspacePath, logPath); err == nil {
		t.Fatal("runStep() error = nil, want initial analyze failure")
	}
	result, repaired, err := runner.tryAutomaticValidationRepair(context.Background(), nil, "run-1", run, appruns.RoundInput{Attempt: 1, TaskBundle: run.TaskBundle}, step)
	if err != nil {
		t.Fatalf("tryAutomaticValidationRepair() error = %v", err)
	}
	if !repaired {
		t.Fatal("tryAutomaticValidationRepair() repaired = false, want true")
	}
	if result != nil {
		t.Fatalf("tryAutomaticValidationRepair() result = %#v, want nil without builder runtime", result)
	}
	if patchGenerator.calls != 0 {
		t.Fatalf("builder runtime patch calls = %d, want 0", patchGenerator.calls)
	}
	updatedHomePage, err := os.ReadFile(homePagePath)
	if err != nil {
		t.Fatalf("ReadFile(homePagePath) error = %v", err)
	}
	if strings.Contains(string(updatedHomePage), ".withOpacity(") {
		t.Fatalf("home_page.dart still contains withOpacity after repair: %q", string(updatedHomePage))
	}
	updatedTest, err := os.ReadFile(testPath)
	if err != nil {
		t.Fatalf("ReadFile(testPath) error = %v", err)
	}
	if strings.Contains(string(updatedTest), "package:flutter_open_lite/models/record.dart") {
		t.Fatalf("widget_test.dart still contains unused import after repair: %q", string(updatedTest))
	}
}

func TestExtractValidationFailurePathsForCheckUsesLastAnalyzeSectionOnly(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "builder.log")
	content := strings.Join([]string{
		`{"operations":[{"write_file":"lib/models/entry.dart"},{"write_file":"lib/models/summary.dart"}]}`,
		"Analyzing workspace...",
		"  error • old failure • lib/models/entry.dart:1:1 • old_code",
		"1 issue found. (ran in 0.5s)",
		"=== builder-runtime model: latest ===",
		"Analyzing workspace...",
		"  error • current failure • lib/views/home_page.dart:10:2 • undefined_getter",
		"  error • current test failure • test/widget_test.dart:8:14 • non_type_as_type_argument",
		"2 issues found. (ran in 1.5s)",
	}, "\n")
	if err := os.WriteFile(logPath, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile(logPath) error = %v", err)
	}
	paths := extractValidationFailurePathsForCheck("", logPath, "check-flutter-analyze")
	want := []string{"lib/views/home_page.dart", "test/widget_test.dart"}
	if len(paths) != len(want) {
		t.Fatalf("len(paths) = %d, want %d; paths=%v", len(paths), len(want), paths)
	}
	for idx, item := range want {
		if paths[idx] != item {
			t.Fatalf("paths[%d] = %q, want %q", idx, paths[idx], item)
		}
	}
}

func TestExtractValidationFailurePathsForCheckIgnoresHistoricalAnalyzeResidueOutsideLastBlock(t *testing.T) {
	workspacePath := t.TempDir()
	logPath := filepath.Join(t.TempDir(), "builder.log")
	content := strings.Join([]string{
		"=== builder-runtime model: old ===",
		"Analyzing workspace...",
		"  error • old failure • lib/models/entry.dart:1:1 • old_code",
		"  error • old failure • lib/controllers/home_controller.dart:2:3 • undefined_identifier",
		"2 issues found. (ran in 0.5s)",
		"old absolute residue: file://" + filepath.ToSlash(filepath.Join(workspacePath, "lib", "models", "entry.dart")) + ":1:1",
		"old absolute residue: " + filepath.ToSlash(filepath.Join(workspacePath, "test", "widget_test.dart")) + ":8:14",
		"=== builder-runtime model: latest ===",
		"Analyzing workspace...",
		"  error • current failure • lib/views/home_page.dart:10:2 • undefined_getter",
		"1 issue found. (ran in 1.5s)",
	}, "\n")
	if err := os.WriteFile(logPath, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile(logPath) error = %v", err)
	}
	paths := extractValidationFailurePathsForCheck(workspacePath, logPath, "check-flutter-analyze")
	want := []string{"lib/views/home_page.dart"}
	if len(paths) != len(want) {
		t.Fatalf("len(paths) = %d, want %d; paths=%v", len(paths), len(want), paths)
	}
	for idx, item := range want {
		if paths[idx] != item {
			t.Fatalf("paths[%d] = %q, want %q", idx, paths[idx], item)
		}
	}
	for _, blocked := range []string{"lib/controllers/home_controller.dart", "lib/models/entry.dart", "test/widget_test.dart"} {
		if containsString(paths, blocked) {
			t.Fatalf("paths should not include historical residue %q: %v", blocked, paths)
		}
	}
}

func TestExtractValidationFailurePathsForCheckUsesLastFlutterTestBlockOnly(t *testing.T) {
	workspacePath := t.TempDir()
	logPath := filepath.Join(t.TempDir(), "builder.log")
	content := strings.Join([]string{
		`{"operations":[{"write_file":"lib/main.dart"},{"write_file":"lib/models/entry.dart"},{"write_file":"lib/views/home_page.dart"}]}`,
		"00:00 +0: loading /tmp/other/test/old_widget_test.dart",
		"00:01 +0: old test passes",
		"00:00 +0: loading " + filepath.Join(workspacePath, "test", "widget_test.dart"),
		"#2      main.<anonymous closure> (file://" + filepath.ToSlash(filepath.Join(workspacePath, "test", "widget_test.dart")) + ":12:5)",
		"#3      buildHome (file://" + filepath.ToSlash(filepath.Join(workspacePath, "lib", "views", "home_page.dart")) + ":18:7)",
		"00:10 +0 -1: bookkeeping flow saves an entry [E]",
		"Test failed. See exception logs above.",
	}, "\n")
	if err := os.WriteFile(logPath, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile(logPath) error = %v", err)
	}
	paths := extractValidationFailurePathsForCheck(workspacePath, logPath, "check-flutter-test")
	want := []string{"lib/views/home_page.dart", "test/widget_test.dart"}
	if len(paths) != len(want) {
		t.Fatalf("len(paths) = %d, want %d; paths=%v", len(paths), len(want), paths)
	}
	for idx, item := range want {
		if paths[idx] != item {
			t.Fatalf("paths[%d] = %q, want %q", idx, paths[idx], item)
		}
	}
}

func TestExtractValidationFailurePathsForCheckUsesLastFlutterTestProgressBlockWhenTestHangs(t *testing.T) {
	workspacePath := t.TempDir()
	logPath := filepath.Join(t.TempDir(), "builder.log")
	content := strings.Join([]string{
		`{"operations":[{"write_file":"lib/main.dart"},{"write_file":"lib/views/record_list_page.dart"}]}`,
		"00:00 +0: loading /tmp/other/test/old_widget_test.dart",
		"00:01 +0: old test passes",
		"00:00 +0: loading " + filepath.Join(workspacePath, "test", "widget_test.dart"),
		"00:05 +0: open lite flow supports create, read, update",
		"00:45 +0: open lite flow supports create, read, update",
		"01:15 +0: open lite flow supports create, read, update",
	}, "\n")
	if err := os.WriteFile(logPath, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile(logPath) error = %v", err)
	}
	paths := extractValidationFailurePathsForCheck(workspacePath, logPath, "check-flutter-test")
	want := []string{"test/widget_test.dart"}
	if len(paths) != len(want) {
		t.Fatalf("len(paths) = %d, want %d; paths=%v", len(paths), len(want), paths)
	}
	for idx, item := range want {
		if paths[idx] != item {
			t.Fatalf("paths[%d] = %q, want %q", idx, paths[idx], item)
		}
	}

	context := extractValidationFailureContext(logPath, "check-flutter-test")
	if !strings.Contains(context, "open lite flow supports create, read, update") {
		t.Fatalf("context = %q, want last flutter test progress block retained", context)
	}
	if strings.Contains(context, `{"operations"`) {
		t.Fatalf("context = %q, want prior builder patch JSON trimmed out", context)
	}
}

func TestExtractValidationFailureContextForFlutterTestTimeoutCompactsProgressNoise(t *testing.T) {
	workspacePath := t.TempDir()
	logPath := filepath.Join(t.TempDir(), "builder.log")
	content := strings.Join([]string{
		`{"operations":[{"write_file":"test/widget_test.dart"}]}`,
		"00:00 +0: loading " + filepath.Join(workspacePath, "test", "widget_test.dart"),
		"00:05 +0: open lite flow supports create, read, update",
		"09:59 +0: open lite flow supports create, read, update",
		"10:01 +0 -1: open lite flow supports create, read, update [E]",
		"TimeoutException after 0:10:00.000000: Test timed out after 10 minutes.",
		"dart:isolate  _RawReceivePort._handleMessage",
		"To run this test again: /opt/flutter/bin/cache/dart-sdk/bin/dart test " + filepath.Join(workspacePath, "test", "widget_test.dart"),
		"10:01 +0 -1: Some tests failed.",
	}, "\n")
	if err := os.WriteFile(logPath, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile(logPath) error = %v", err)
	}

	context := extractValidationFailureContext(logPath, "check-flutter-test")
	if !strings.Contains(context, "TimeoutException after 0:10:00.000000: Test timed out after 10 minutes.") {
		t.Fatalf("context = %q, want timeout line retained", context)
	}
	if !strings.Contains(context, "10:01 +0 -1: open lite flow supports create, read, update [E]") {
		t.Fatalf("context = %q, want last progress line retained", context)
	}
	if strings.Contains(context, "00:05 +0: open lite flow supports create, read, update") || strings.Contains(context, "09:59 +0: open lite flow supports create, read, update") {
		t.Fatalf("context = %q, want earlier progress spam trimmed out for timeout failures", context)
	}
	if strings.Contains(context, `{"operations"`) {
		t.Fatalf("context = %q, want prior builder patch JSON trimmed out", context)
	}
}

func TestExtractValidationFailureContextForFlutterTestUsesLastFailureBlock(t *testing.T) {
	workspacePath := t.TempDir()
	logPath := filepath.Join(t.TempDir(), "builder.log")
	content := strings.Join([]string{
		`{"operations":[{"write_file":"lib/main.dart"}]}`,
		"00:00 +0: loading " + filepath.Join(workspacePath, "test", "widget_test.dart"),
		"#2      main.<anonymous closure> (file://" + filepath.ToSlash(filepath.Join(workspacePath, "test", "widget_test.dart")) + ":12:5)",
		"00:10 +0 -1: bookkeeping flow saves an entry [E]",
		"Test failed. See exception logs above.",
	}, "\n")
	if err := os.WriteFile(logPath, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile(logPath) error = %v", err)
	}
	context := extractValidationFailureContext(logPath, "check-flutter-test")
	if !strings.Contains(context, "check=check-flutter-test") {
		t.Fatalf("context = %q, want check header", context)
	}
	if !strings.Contains(context, "Test failed. See exception logs above.") {
		t.Fatalf("context = %q, want last flutter test failure block", context)
	}
	if strings.Contains(context, `{"operations"`) {
		t.Fatalf("context = %q, want prior builder patch JSON trimmed out", context)
	}
}

func TestExpandValidationFailurePathsWithLocalImportsAddsAdjacentDartFiles(t *testing.T) {
	workspacePath := t.TempDir()
	viewDir := filepath.Join(workspacePath, "lib", "views")
	controllerDir := filepath.Join(workspacePath, "lib", "controllers")
	if err := os.MkdirAll(viewDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(viewDir) error = %v", err)
	}
	if err := os.MkdirAll(controllerDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(controllerDir) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(viewDir, "record_list_page.dart"), []byte(strings.Join([]string{
		"import '../controllers/record_list_controller.dart';",
		"import 'package:flutter/material.dart';",
		"import '../template/open_lite_copy.dart';",
	}, "\n")), 0o600); err != nil {
		t.Fatalf("WriteFile(record_list_page.dart) error = %v", err)
	}
	paths := expandValidationFailurePathsWithLocalImports(workspacePath, []string{"lib/views/record_list_page.dart"})
	want := []string{"lib/views/record_list_page.dart", "lib/controllers/record_list_controller.dart", "lib/template/open_lite_copy.dart"}
	if len(paths) != len(want) {
		t.Fatalf("len(paths) = %d, want %d; paths=%v", len(paths), len(want), paths)
	}
	for idx, item := range want {
		if paths[idx] != item {
			t.Fatalf("paths[%d] = %q, want %q", idx, paths[idx], item)
		}
	}
}

func TestExpandValidationFailurePathsWithPackageImportsAddsCurrentPackageFiles(t *testing.T) {
	workspacePath := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspacePath, "test"), 0o755); err != nil {
		t.Fatalf("MkdirAll(test) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(workspacePath, "pubspec.yaml"), []byte("name: bookkeeping_lite\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(pubspec.yaml) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(workspacePath, "test", "widget_test.dart"), []byte(strings.Join([]string{
		"import 'package:flutter_test/flutter_test.dart';",
		"import 'package:bookkeeping_lite/repositories/entry_repository.dart';",
	}, "\n")), 0o600); err != nil {
		t.Fatalf("WriteFile(widget_test.dart) error = %v", err)
	}
	paths := expandValidationFailurePathsWithLocalImports(workspacePath, []string{"test/widget_test.dart"})
	want := []string{"test/widget_test.dart", "lib/repositories/entry_repository.dart"}
	if len(paths) != len(want) {
		t.Fatalf("len(paths) = %d, want %d; paths=%v", len(paths), len(want), paths)
	}
	for idx, item := range want {
		if paths[idx] != item {
			t.Fatalf("paths[%d] = %q, want %q", idx, paths[idx], item)
		}
	}
}

func TestBuildValidationRepairTargetPathsKeepsDirectFailuresAndExpandedDeps(t *testing.T) {
	paths := buildValidationRepairTargetPaths(
		appruns.BuilderRuntimeTaskTypeAnalyzeRepair,
		[]string{"test/widget_test.dart"},
		[]string{"test/widget_test.dart", "lib/repositories/entry_repository.dart"},
		[]string{"lib/main.dart"},
	)
	want := []string{"test/widget_test.dart"}
	if len(paths) != len(want) {
		t.Fatalf("len(paths) = %d, want %d; paths=%v", len(paths), len(want), paths)
	}
	for idx, item := range want {
		if paths[idx] != item {
			t.Fatalf("paths[%d] = %q, want %q", idx, paths[idx], item)
		}
	}
}

func TestPrioritizeValidationFailurePathsPrependsFailures(t *testing.T) {
	paths := prioritizeValidationFailurePaths(
		[]string{"lib/views/home_page.dart", "lib/controllers/record_form_controller.dart"},
		[]string{"lib/models/record.dart", "lib/views/home_page.dart", "lib/repositories/record_repository.dart"},
	)
	want := []string{
		"lib/views/home_page.dart",
		"lib/controllers/record_form_controller.dart",
		"lib/models/record.dart",
		"lib/repositories/record_repository.dart",
	}
	if len(paths) != len(want) {
		t.Fatalf("len(paths) = %d, want %d; paths=%v", len(paths), len(want), paths)
	}
	for idx, item := range want {
		if paths[idx] != item {
			t.Fatalf("paths[%d] = %q, want %q", idx, paths[idx], item)
		}
	}
}

func TestBuildValidationRepairTargetPathsPrefersDirectFailures(t *testing.T) {
	paths := buildValidationRepairTargetPaths(
		appruns.BuilderRuntimeTaskTypeAnalyzeRepair,
		[]string{"lib/views/home_page.dart", "test/widget_test.dart"},
		[]string{"lib/views/home_page.dart", "lib/models/record.dart", "test/widget_test.dart"},
		[]string{"lib/models/record.dart", "lib/repositories/record_repository.dart"},
	)
	want := []string{"lib/views/home_page.dart", "test/widget_test.dart"}
	if len(paths) != len(want) {
		t.Fatalf("len(paths) = %d, want %d; paths=%v", len(paths), len(want), paths)
	}
	for idx, item := range want {
		if paths[idx] != item {
			t.Fatalf("paths[%d] = %q, want %q", idx, paths[idx], item)
		}
	}
}

func TestBuildValidationRepairTargetPathsFallsBackToExpandedFailures(t *testing.T) {
	paths := buildValidationRepairTargetPaths(
		appruns.BuilderRuntimeTaskTypeAnalyzeRepair,
		nil,
		[]string{"lib/views/home_page.dart", "lib/models/record.dart"},
		[]string{"lib/repositories/record_repository.dart"},
	)
	want := []string{"lib/views/home_page.dart", "lib/models/record.dart", "lib/repositories/record_repository.dart"}
	if len(paths) != len(want) {
		t.Fatalf("len(paths) = %d, want %d; paths=%v", len(paths), len(want), paths)
	}
	for idx, item := range want {
		if paths[idx] != item {
			t.Fatalf("paths[%d] = %q, want %q", idx, paths[idx], item)
		}
	}
}

func TestBuildValidationRepairTargetPathsUsesDirectFailuresOnlyForTestRepair(t *testing.T) {
	paths := buildValidationRepairTargetPaths(
		appruns.BuilderRuntimeTaskTypeTestRepair,
		[]string{"lib/controllers/entry_list_controller.dart", "test/widget_test.dart"},
		[]string{"lib/controllers/entry_list_controller.dart", "lib/models/entry.dart", "test/widget_test.dart"},
		[]string{"lib/models/entry.dart", "lib/repositories/entry_repository.dart"},
	)
	want := []string{"lib/controllers/entry_list_controller.dart", "test/widget_test.dart"}
	if len(paths) != len(want) {
		t.Fatalf("len(paths) = %d, want %d; paths=%v", len(paths), len(want), paths)
	}
	for idx, item := range want {
		if paths[idx] != item {
			t.Fatalf("paths[%d] = %q, want %q", idx, paths[idx], item)
		}
	}
}

func TestBuildValidationRepairTargetPathsKeepsExpandedDepsOutOfAnalyzeCoverageContract(t *testing.T) {
	paths := buildValidationRepairTargetPaths(
		appruns.BuilderRuntimeTaskTypeAnalyzeRepair,
		[]string{"lib/main.dart", "test/widget_test.dart"},
		[]string{"lib/main.dart", "lib/repositories/entry_repository.dart", "test/widget_test.dart"},
		[]string{"lib/repositories/entry_repository.dart", "lib/views/home_page.dart"},
	)
	want := []string{"lib/main.dart", "test/widget_test.dart"}
	if len(paths) != len(want) {
		t.Fatalf("len(paths) = %d, want %d; paths=%v", len(paths), len(want), paths)
	}
	for idx, item := range want {
		if paths[idx] != item {
			t.Fatalf("paths[%d] = %q, want %q", idx, paths[idx], item)
		}
	}
}

func TestSelectValidationRepairContextTasksKeepsMatchedTasksAndDependencies(t *testing.T) {
	tasks := selectValidationRepairContextTasks([]appruns.TaskBundleItem{
		{TaskID: "task-generic-domain-models", TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, TargetPaths: []string{"lib/models/record.dart", "lib/models/dashboard_summary.dart"}},
		{TaskID: "task-generic-screen-scaffold", TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, Dependencies: []string{"task-generic-domain-models"}, TargetPaths: []string{"lib/views/home_page.dart", "lib/views/record_list_page.dart"}},
		{TaskID: "task-generic-summary-remap", TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, Dependencies: []string{"task-generic-domain-models", "task-generic-screen-scaffold"}, TargetPaths: []string{"lib/controllers/home_controller.dart", "lib/views/home_page.dart"}},
		{TaskID: "task-generic-storage-wiring", TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, Dependencies: []string{"task-generic-domain-models"}, TargetPaths: []string{"lib/repositories/record_repository.dart"}},
		{TaskID: "task-generic-validation-closure", TaskType: appruns.BuilderRuntimeTaskTypeClosureRepair, Dependencies: []string{"task-generic-summary-remap"}, TargetPaths: []string{"lib/**"}},
	}, []string{"lib/views/home_page.dart"})
	want := []string{"task-generic-domain-models", "task-generic-screen-scaffold", "task-generic-summary-remap"}
	if len(tasks) != len(want) {
		t.Fatalf("len(tasks) = %d, want %d; tasks=%v", len(tasks), len(want), tasks)
	}
	for idx, item := range want {
		if tasks[idx].TaskID != item {
			t.Fatalf("tasks[%d].TaskID = %q, want %q", idx, tasks[idx].TaskID, item)
		}
	}
}

func TestSelectValidationRepairContextTasksFallsBackWhenNoFailurePaths(t *testing.T) {
	input := []appruns.TaskBundleItem{
		{TaskID: "task-a", TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, TargetPaths: []string{"lib/a.dart"}},
		{TaskID: "task-b", TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, TargetPaths: []string{"lib/b.dart"}},
	}
	tasks := selectValidationRepairContextTasks(input, nil)
	if len(tasks) != len(input) {
		t.Fatalf("len(tasks) = %d, want %d", len(tasks), len(input))
	}
	if tasks[0].TaskID != "task-a" || tasks[1].TaskID != "task-b" {
		t.Fatalf("tasks = %v, want original order", tasks)
	}
}

func TestMergeBuilderRuntimePlanFallbacksPreservesUpgradeConfig(t *testing.T) {
	primary := &appruns.BuilderRuntimePlan{
		Enabled:      true,
		DefaultModel: appruns.BuilderRuntimeModelRef{Primary: "local-default"},
		TaskRoutes: []appruns.BuilderRuntimeTaskRoute{{
			TaskID:      "repair-check-flutter-analyze",
			TaskType:    appruns.BuilderRuntimeTaskTypeAnalyzeRepair,
			RouteSource: "task_route",
			Model:       appruns.BuilderRuntimeModelRef{Primary: "local-default"},
		}},
	}
	fallback := &appruns.BuilderRuntimePlan{
		Enabled:      true,
		DefaultModel: appruns.BuilderRuntimeModelRef{Primary: "fallback-default"},
		UpgradeModel: appruns.BuilderRuntimeModelRef{Primary: "fallback-upgrade"},
		UpgradeThreshold: appruns.BuilderRuntimeUpgradeThreshold{
			UpgradeOnSemanticConflict: true,
			UpgradeOnValidationFail:   true,
		},
	}
	merged := mergeBuilderRuntimePlanFallbacks(primary, fallback)
	if merged == nil {
		t.Fatalf("mergeBuilderRuntimePlanFallbacks() = nil, want plan")
	}
	if merged.DefaultModel.Primary != "local-default" {
		t.Fatalf("DefaultModel.Primary = %q, want local-default", merged.DefaultModel.Primary)
	}
	if merged.UpgradeModel.Primary != "fallback-upgrade" {
		t.Fatalf("UpgradeModel.Primary = %q, want fallback-upgrade", merged.UpgradeModel.Primary)
	}
	if !merged.UpgradeThreshold.UpgradeOnSemanticConflict {
		t.Fatalf("UpgradeOnSemanticConflict = false, want true")
	}
	if !merged.UpgradeThreshold.UpgradeOnValidationFail {
		t.Fatalf("UpgradeOnValidationFail = false, want true")
	}
	if len(merged.TaskRoutes) != 1 || merged.TaskRoutes[0].TaskID != "repair-check-flutter-analyze" {
		t.Fatalf("TaskRoutes = %+v, want preserved primary task route", merged.TaskRoutes)
	}
}

func TestExpandBuilderRuntimePlanModelFallbacksUsesModelListChain(t *testing.T) {
	catalog := &appconfig.Config{ModelList: []*appconfig.ModelConfig{
		{ModelName: "gemma4-26b-local", Fallbacks: []string{"qwen2.5-coder-32b-local", "gpt-4o-mini"}},
		{ModelName: "qwen2.5-coder-32b-local", Fallbacks: []string{"gpt-4o-mini"}},
		{ModelName: "gpt-4o-mini"},
		{ModelName: "claude-sonnet-4"},
	}}
	plan := &appruns.BuilderRuntimePlan{
		DefaultModel: appruns.BuilderRuntimeModelRef{Primary: "gemma4-26b-local", Fallbacks: []string{"claude-sonnet-4"}},
		UpgradeModel: appruns.BuilderRuntimeModelRef{Primary: "gemma4-26b-local"},
		TaskRoutes: []appruns.BuilderRuntimeTaskRoute{{
			TaskID:      "task-create-record-model",
			TaskType:    appruns.BuilderRuntimeTaskTypeDualFileWiring,
			RouteSource: "task_route",
			Model:       appruns.BuilderRuntimeModelRef{Primary: "gemma4-26b-local"},
		}},
	}

	expandBuilderRuntimePlanModelFallbacks(plan, catalog)

	if got := append([]string{plan.DefaultModel.Primary}, plan.DefaultModel.Fallbacks...); strings.Join(got, ",") != "gemma4-26b-local,qwen2.5-coder-32b-local,gpt-4o-mini,claude-sonnet-4" {
		t.Fatalf("DefaultModel aliases = %v, want gemma -> qwen -> gpt -> claude", got)
	}
	if got := append([]string{plan.UpgradeModel.Primary}, plan.UpgradeModel.Fallbacks...); strings.Join(got, ",") != "gemma4-26b-local,qwen2.5-coder-32b-local,gpt-4o-mini" {
		t.Fatalf("UpgradeModel aliases = %v, want gemma -> qwen -> gpt", got)
	}
	if len(plan.TaskRoutes) != 1 {
		t.Fatalf("len(TaskRoutes) = %d, want 1", len(plan.TaskRoutes))
	}
	if got := append([]string{plan.TaskRoutes[0].Model.Primary}, plan.TaskRoutes[0].Model.Fallbacks...); strings.Join(got, ",") != "gemma4-26b-local,qwen2.5-coder-32b-local,gpt-4o-mini" {
		t.Fatalf("TaskRoute aliases = %v, want gemma -> qwen -> gpt", got)
	}
}

func TestNormalizeBuilderRuntimePatchSupportsNestedOperationShape(t *testing.T) {
	raw := map[string]any{
		"patch_id": "patch-1",
		"operations": []map[string]any{
			{
				"write_file": map[string]any{
					"path":    "lib/main.dart",
					"content": "void main() {}\n",
				},
			},
			{
				"delete_file": map[string]any{
					"path": "lib/old.dart",
				},
			},
		},
	}
	encoded, err := json.Marshal(raw)
	if err != nil {
		t.Fatalf("Marshal(raw) error = %v", err)
	}

	patch, normalized, driftCount, err := normalizeBuilderRuntimePatch(string(encoded), "round-1", nil)
	if err != nil {
		t.Fatalf("normalizeBuilderRuntimePatch() error = %v", err)
	}
	if !normalized {
		t.Fatalf("normalized = false, want true")
	}
	if driftCount != 2 {
		t.Fatalf("driftCount = %d, want 2", driftCount)
	}
	if len(patch.Operations) != 2 {
		t.Fatalf("len(patch.Operations) = %d, want 2", len(patch.Operations))
	}
	if patch.Operations[0].Type != "write_file" || patch.Operations[0].Path != "lib/main.dart" {
		t.Fatalf("patch.Operations[0] = %+v, want write_file lib/main.dart", patch.Operations[0])
	}
	if patch.Operations[1].Type != "delete_file" || patch.Operations[1].Path != "lib/old.dart" {
		t.Fatalf("patch.Operations[1] = %+v, want delete_file lib/old.dart", patch.Operations[1])
	}
}

func TestNormalizeBuilderRuntimePatchIgnoresNestedInnerTypeOverride(t *testing.T) {
	raw := map[string]any{
		"patch_id": "patch-2",
		"operations": []map[string]any{
			{
				"replace_block": map[string]any{
					"type":        "file",
					"path":        "lib/main.dart",
					"old_content": "old\n",
					"new_content": "new\n",
				},
			},
		},
	}
	encoded, err := json.Marshal(raw)
	if err != nil {
		t.Fatalf("Marshal(raw) error = %v", err)
	}

	patch, normalized, driftCount, err := normalizeBuilderRuntimePatch(string(encoded), "round-2", nil)
	if err != nil {
		t.Fatalf("normalizeBuilderRuntimePatch() error = %v", err)
	}
	if !normalized {
		t.Fatalf("normalized = false, want true")
	}
	if driftCount != 2 {
		t.Fatalf("driftCount = %d, want 2", driftCount)
	}
	if len(patch.Operations) != 1 {
		t.Fatalf("len(patch.Operations) = %d, want 1", len(patch.Operations))
	}
	if patch.Operations[0].Type != "replace_block" {
		t.Fatalf("patch.Operations[0].Type = %q, want replace_block", patch.Operations[0].Type)
	}
	if patch.Operations[0].Path != "lib/main.dart" {
		t.Fatalf("patch.Operations[0].Path = %q, want lib/main.dart", patch.Operations[0].Path)
	}
	if patch.Operations[0].OldContent != "old" {
		t.Fatalf("patch.Operations[0].OldContent = %q, want old content", patch.Operations[0].OldContent)
	}
	if patch.Operations[0].NewContent != "new\n" {
		t.Fatalf("patch.Operations[0].NewContent = %q, want new content", patch.Operations[0].NewContent)
	}
}

func TestNormalizeBuilderRuntimePatchRejectsReplaceBlockWithoutReplacement(t *testing.T) {
	raw := map[string]any{
		"patch_id": "patch-3",
		"operations": []map[string]any{
			{
				"replace_block": map[string]any{
					"path":        "lib/views/home_page.dart",
					"old_content": "class HomePage {}\n",
				},
			},
		},
	}
	encoded, err := json.Marshal(raw)
	if err != nil {
		t.Fatalf("Marshal(raw) error = %v", err)
	}

	_, normalized, driftCount, err := normalizeBuilderRuntimePatch(string(encoded), "round-3", nil)
	if err == nil {
		t.Fatal("normalizeBuilderRuntimePatch() error = nil, want missing replacement error")
	}
	if !strings.Contains(err.Error(), "replace_block requires new_content, content, or replacement") {
		t.Fatalf("normalizeBuilderRuntimePatch() error = %v, want missing replacement error", err)
	}
	if !normalized {
		t.Fatalf("normalized = false, want true")
	}
	if driftCount != 3 {
		t.Fatalf("driftCount = %d, want 3", driftCount)
	}
}

func TestShouldRetryBuilderRuntimeWithUpgradeSkipsWhenAlreadyUpgraded(t *testing.T) {
	run := runRecord{
		BuilderRuntime: &appruns.BuilderRuntimePlan{
			UpgradeModel: appruns.BuilderRuntimeModelRef{Primary: "qwen2.5-coder-32b-local"},
			UpgradeThreshold: appruns.BuilderRuntimeUpgradeThreshold{
				UpgradeOnPatchParseFail: true,
			},
		},
	}
	stats := &appruns.BuilderRuntimeExecutionStats{
		Attempts:       1,
		UpgradeApplied: true,
		SelectedModel:  "gpt-4o-mini",
	}

	if shouldRetryBuilderRuntimeWithUpgrade(run, stats, "parse_failure") {
		t.Fatal("shouldRetryBuilderRuntimeWithUpgrade() = true, want false when upgrade already applied")
	}
}

func TestNormalizeBuilderRuntimePatchRejectsConflictingNestedOperationType(t *testing.T) {
	raw := map[string]any{
		"patch_id": "patch-4",
		"operations": []map[string]any{
			{
				"replace_block": map[string]any{
					"type":        "delete",
					"path":        "lib/views/home_page.dart",
					"old_content": "class HomePage {}\n",
					"new_content": "class HomePageNew {}\n",
				},
			},
		},
	}
	encoded, err := json.Marshal(raw)
	if err != nil {
		t.Fatalf("Marshal(raw) error = %v", err)
	}

	_, normalized, driftCount, err := normalizeBuilderRuntimePatch(string(encoded), "round-4", nil)
	if err == nil {
		t.Fatal("normalizeBuilderRuntimePatch() error = nil, want conflicting type error")
	}
	if !strings.Contains(err.Error(), "builder runtime operation type conflict") {
		t.Fatalf("normalizeBuilderRuntimePatch() error = %v, want conflicting type error", err)
	}
	if !normalized {
		t.Fatalf("normalized = false, want true")
	}
	if driftCount != 2 {
		t.Fatalf("driftCount = %d, want 2", driftCount)
	}
}

func TestNormalizeBuilderRuntimePatchRepairsMissingObjectCloserBeforeArrayEnd(t *testing.T) {
	raw := "{\"patch_id\":\"patch-5\",\"operations\":[{\"write_file\":{\"path\":\"lib/main.dart\",\"content\":\"void main() {}\\n\"}},{\"delete_file\":{\"path\":\"lib/old.dart\"}] }"

	patch, normalized, driftCount, err := normalizeBuilderRuntimePatch(raw, "round-5", nil)
	if err != nil {
		t.Fatalf("normalizeBuilderRuntimePatch() error = %v, want repaired JSON", err)
	}
	if !normalized {
		t.Fatalf("normalized = false, want true")
	}
	if driftCount != 3 {
		t.Fatalf("driftCount = %d, want 3", driftCount)
	}
	if len(patch.Operations) != 2 {
		t.Fatalf("len(patch.Operations) = %d, want 2", len(patch.Operations))
	}
	if patch.Operations[1].Type != "delete_file" || patch.Operations[1].Path != "lib/old.dart" {
		t.Fatalf("patch.Operations[1] = %+v, want delete_file lib/old.dart", patch.Operations[1])
	}
}

func TestNormalizeBuilderRuntimePatchTrimsTrailingContentAfterJSONObject(t *testing.T) {
	raw := "{\"patch_id\":\"patch-6\",\"operations\":[{\"write_file\":{\"path\":\"lib/main.dart\",\"content\":\"void main() {}\\n\"}}]}\nextra trailing text"

	patch, normalized, driftCount, err := normalizeBuilderRuntimePatch(raw, "round-6", nil)
	if err != nil {
		t.Fatalf("normalizeBuilderRuntimePatch() error = %v, want trimmed JSON", err)
	}
	if !normalized {
		t.Fatalf("normalized = false, want true")
	}
	if driftCount != 2 {
		t.Fatalf("driftCount = %d, want 2", driftCount)
	}
	if len(patch.Operations) != 1 {
		t.Fatalf("len(patch.Operations) = %d, want 1", len(patch.Operations))
	}
	if patch.Operations[0].Type != "write_file" || patch.Operations[0].Path != "lib/main.dart" {
		t.Fatalf("patch.Operations[0] = %+v, want write_file lib/main.dart", patch.Operations[0])
	}
}

func TestNormalizeBuilderRuntimePatchRepairsSchemaRepairResponseWithTrailingArrayCloser(t *testing.T) {
	raw := strings.TrimSpace(`
{
  "patch_id": "patch-bind-mutation-surface-record-form",
  "operations": [
    {
      "write_file": {
        "path": "lib/views/record_form_page.dart",
        "new_content": "class RecordFormPage {}\n"
      }
    },
    {
      "replace_block": {
        "path": "lib/controllers/record_form_controller.dart",
        "old_content": "old status block",
        "new_content": "new status block"
      }
    },
    {
      "replace_block": {
        "path": "lib/controllers/record_form_controller.dart",
        "old_content": "old submit block",
        "new_content": "new submit block"
      }
    }
  ]
}
]`)

	patch, normalized, driftCount, err := normalizeBuilderRuntimePatch(raw, "round-form-repair", nil)
	if err != nil {
		t.Fatalf("normalizeBuilderRuntimePatch() error = %v, want repaired trailing array closer", err)
	}
	if !normalized {
		t.Fatalf("normalized = false, want true")
	}
	if driftCount < 2 {
		t.Fatalf("driftCount = %d, want at least 2", driftCount)
	}
	if len(patch.Operations) != 3 {
		t.Fatalf("len(patch.Operations) = %d, want 3", len(patch.Operations))
	}
	if patch.Operations[0].Type != "write_file" || patch.Operations[0].Path != "lib/views/record_form_page.dart" {
		t.Fatalf("patch.Operations[0] = %+v, want write_file lib/views/record_form_page.dart", patch.Operations[0])
	}
	if patch.Operations[1].Type != "replace_block" || patch.Operations[1].Path != "lib/controllers/record_form_controller.dart" {
		t.Fatalf("patch.Operations[1] = %+v, want replace_block lib/controllers/record_form_controller.dart", patch.Operations[1])
	}
	if patch.Operations[2].Type != "replace_block" || patch.Operations[2].Path != "lib/controllers/record_form_controller.dart" {
		t.Fatalf("patch.Operations[2] = %+v, want replace_block lib/controllers/record_form_controller.dart", patch.Operations[2])
	}
}

func TestNormalizeBuilderRuntimePatchRepairsLiveRecordFormSchemaRepairResponse(t *testing.T) {
	raw := strings.TrimSpace(`
{
	"patch_id": "patch-bind-mutation-surface-record-form",
	"operations": [
		{
			"write_file": {
				"path": "lib/views/record_form_page.dart",
				"new_content": "import 'package:flutter/material.dart';\n\nimport '../controllers/record_form_controller.dart';\nimport '../models/record.dart';\nimport '../template/open_lite_copy.dart';\n\nclass RecordFormPage extends StatefulWidget {\n  const RecordFormPage({\n    super.key,\n    required this.repository,\n    this.initialRecord,\n  });\n\n  final dynamic repository;\n  final AppRecord? initialRecord;\n\n  @override\n  State<RecordFormPage> createState() => _RecordFormPageState();\n}\n\nclass _RecordFormPageState extends State<RecordFormPage> {\n  final GlobalKey<FormState> _formKey = GlobalKey<FormState>();\n  late final RecordFormController _controller;\n\n  @override\n  void initState() {\n    super.initState();\n    _controller = Record\n        RecordFormController(\n      initialRecord: widget.initialRecord,\n      repository: widget.repository,\n    );\n  }\n\n  @override\n  void dispose() {\n    _controller.dispose();\n    super.dispose();\n  }\n\n  Future<void> _save() async {\n    if (!_formKey.currentState!.validate()) {\n      return;\n    }\n    try {\n      final savedRecord = await _controller.submit();\n      if (!mounted) {\n        return;\n      }\n      Navigator.of(context).pop(savedRecord);\n    } on FormatException catch (error) {\n      if (!mounted) {\n        return;\n      }\n      ScaffoldMessenger.of(context).showSnackBar(\n        SnackBar(content: Text(error.message)),\n      );\n    }\n  }\n\n  @override\n  Widget build(BuildContext context) {\n    return AnimatedBuilder(\n      animation: _controller,\n      builder: (context, _) {\n        return Scaffold(\n          appBar: AppBar(\n            title: Text(\n              _controller.isEditing ? openLiteCopy.editPageTitle : openLiteCopy.createPageTitle,\n            ),\n          ),\n          body: Form(\n            key: _formKey,\n            child: ListView(\n              padding: const EdgeInsets.all(20),\n              children: [\n                TextFormField(\n                  key: const Key('title-field'),\n                  controller: _controller.titleController,\n                  decoration: InputDecoration(\n                    labelText: openLiteCopy.titleFieldLabel,\n                  ),\n                  validator: (value) {\n                    if ((value ?? '').trim(). .isEmpty) {\n                      return openLiteCopy.titleFieldRequiredError;\n                    }\n                    return null;\n                  },\n                ),\n                const SizedBox(height: 16),\n                DropdownButtonFormField<String>(\n                  value: _controller.categoryController.text.isEmpty \n                      ? null \n                      : _controller.categoryController.text,\n                  items: _controller.categories\n                      .map(\n                        (category) => DropdownMenuItem<String>(\n                          value: category,\n                          child: Text(category),\n                        ),\n                      )\n                      .toList(),\n                  decoration: InputDecoration(\n                    labelText: openLiteCopy.categoryFieldLabel,\n                  ),\n                  onChanged: (value) {\n                    if (value != null) {\n                      _controller.setCategory(value);\n                    }\n                  },\n                ),\n                const SizedBox(height: 16),\n                SegmentedButton<RecordStatus>(\n                  segments: const [\n                    ButtonSegment<RecordStatus>(\n                      value: RecordStatus.inbox,\n                      label: Text('待整理'),\n                    ),\n                    ButtonSegment<RecordStatus>(\n                      value: RecordStatus.inProgress,\n                      label: Text('进行中'),\n                    ),\n                    ButtonSegment<RecordStatus>(\n                      value: RecordStatus.done,\n                      label: Text('已完成'),\n                    ),\n                  ],\n                  selected: <RecordStatus>{_controller.selectedStatus},\n                  onSelectionChanged: (selection) {\n                    _controller.setStatus(selection.first);\n                  },\n                ),\n                const SizedBox(height: 16),\n                TextFormField(\n                  key: const Key('note-field'),\n                  controller: _controller.noteController,\n                  decoration: InputDecoration(\n                    labelText: openLiteCopy.noteFieldLabel,\n                    hintText: openLiteCopy.noteFieldHint,\n                  ),\n                  minLines: 2,\n                  maxLines: 3,\n                ),\n                const SizedBox(height: 24),\n                FilledButton(\n                  onPressed: _save,\n                  child: Text(\n                    _controller.isEditing ? openLiteCopy.editSubmitLabel : openLiteCopy.createSubmitLabel,\n                  ),\n                ),\n              ],\n            ),\n          ),\n        );\n      },\n    );\n  }\n}\n"
			},
			{
				"replace_block": {
					"path": "lib/controllers/record_form_controller.dart",
					"old_content": "  Future<void> submit() async {\n    final title = titleController.text.trim();\n    if (title.isEmpty) {\n      throw const FormatException('请输入标题');\n    }\n    final category = categorylyController.text.trim();\n    if (category.isEmpty) {\n      throw const FormatException('请选择分类');\n    }\n\n    final note = noteController.text.trim();\n    final taskId = _initialRecord?.taskId ?? DateTime.now().microsecondsSinceEpoch.toString();\n\n    final record = AppRecord(\n      taskId: taskId,\n      title: title,\n      category: category,\n      status: _selectedStatus,\n      note: note.isEmpty ? null : note,\n    );\n\n    if (_initialRecord != null) {\n      await _repository.updateRecord(record);\n    } else {\n      await _repository.addRecord(record);\n    }\n  }",
					"new_content": "  Future<AppRecord> submit() async {\n    final title = titleController.text.trim();\n    if (title.isEmpty) {\n      throw const FormatException('请输入标题');\n    }\n    final category = categoryController.text.trim();\n    if (category.isEmpty) {\n      throw const FormatException('请选择分类');\n    }\n\n    final note = noteController.text.trim();\n    final taskId = _initialRecord?.taskId ?? DateTime.now().microsecondsSinceEpoch.toString();\n\n    final record = AppRecord(\n      taskId: taskId,\n      title: title,\n      category: category,\n      status: _selectedStatus,\n      note: note.isEmpty ? null : note,\n    );\n\n    if (_initialRecord != null) {\n      await _repository.updateRecord(record);\n    } else {\n      await _repository.addRecord(record);\n    }\n    return record;\n  }"
				}
			},
			{
				"replace_block": {
					"path": "lib/controllers/record_form_controller.dart",
					"old_content": "  void setStatus(RecordStatus status) {\n    _selectedStatus = status;\n    notifyListeners();\n  }",
					"new_content": "  void setStatus(RecordStatus status) {\n    _selectedStatus = status;\n    notifyListeners();\n  }\n\n  void setCategory(String category) {\n    categoryController.text = category;\n    notifyListeners();\n  }"
				}
			}
		]
	}
]`)

	patch, normalized, driftCount, err := normalizeBuilderRuntimePatch(raw, "round-live-form-repair", nil)
	if err != nil {
		t.Fatalf("normalizeBuilderRuntimePatch() error = %v, want repaired live schema repair response", err)
	}
	if !normalized {
		t.Fatalf("normalized = false, want true")
	}
	if driftCount < 2 {
		t.Fatalf("driftCount = %d, want at least 2", driftCount)
	}
	if len(patch.Operations) != 3 {
		t.Fatalf("len(patch.Operations) = %d, want 3", len(patch.Operations))
	}
	if patch.Operations[0].Path != "lib/views/record_form_page.dart" {
		t.Fatalf("patch.Operations[0].Path = %q, want lib/views/record_form_page.dart", patch.Operations[0].Path)
	}
}

func TestNormalizeBuilderRuntimePatchWrapsLiveOperationArrayRoot(t *testing.T) {
	raw := strings.TrimSpace(`
[
  {
    "operation": "write_file",
    "path": "lib/views/record_detail_page.dart",
    "new_content": "class RecordDetailPage {}\n"
  }
]`)

	patch, normalized, driftCount, err := normalizeBuilderRuntimePatch(raw, "round-live-detail-array", []string{"lib/views/record_detail_page.dart"})
	if err != nil {
		t.Fatalf("normalizeBuilderRuntimePatch() error = %v, want wrapped operation array", err)
	}
	if !normalized {
		t.Fatalf("normalized = false, want true")
	}
	if driftCount < 1 {
		t.Fatalf("driftCount = %d, want at least 1", driftCount)
	}
	if patch.PatchID != "round-live-detail-array-patch" {
		t.Fatalf("patch.PatchID = %q, want round-live-detail-array-patch", patch.PatchID)
	}
	if len(patch.Operations) != 1 {
		t.Fatalf("len(patch.Operations) = %d, want 1", len(patch.Operations))
	}
	if patch.Operations[0].Type != "write_file" || patch.Operations[0].Path != "lib/views/record_detail_page.dart" {
		t.Fatalf("patch.Operations[0] = %+v, want write_file lib/views/record_detail_page.dart", patch.Operations[0])
	}
	if patch.Operations[0].Content != "class RecordDetailPage {}\n" {
		t.Fatalf("patch.Operations[0].Content = %q, want wrapped new_content", patch.Operations[0].Content)
	}
}

func TestNormalizeBuilderRuntimePatchRejectsDeleteFileOnTargetPath(t *testing.T) {
	raw := map[string]any{
		"patch_id": "patch-7",
		"operations": []map[string]any{
			{
				"delete_file": map[string]any{
					"path": "lib/views/home_page.dart",
				},
			},
		},
	}
	encoded, err := json.Marshal(raw)
	if err != nil {
		t.Fatalf("Marshal(raw) error = %v", err)
	}

	_, normalized, driftCount, err := normalizeBuilderRuntimePatch(string(encoded), "round-7", []string{"lib/views/home_page.dart", "lib/main.dart"})
	if err == nil {
		t.Fatal("normalizeBuilderRuntimePatch() error = nil, want delete target path error")
	}
	if !strings.Contains(err.Error(), "delete_file cannot remove target path") {
		t.Fatalf("normalizeBuilderRuntimePatch() error = %v, want target delete error", err)
	}
	if !normalized {
		t.Fatalf("normalized = false, want true")
	}
	if driftCount != 1 {
		t.Fatalf("driftCount = %d, want 1", driftCount)
	}
}

func TestNormalizeBuilderRuntimePatchRepairsInvalidStringEscapeForDartInterpolation(t *testing.T) {
	raw := "{\"patch_id\":\"patch-8\",\"operations\":[{\"write_file\":{\"path\":\"lib/views/home_page.dart\",\"new_content\":\"Text('\\${summary.entryCount} 条记录')\\n\"}}]}"

	patch, normalized, driftCount, err := normalizeBuilderRuntimePatch(raw, "round-8", nil)
	if err != nil {
		t.Fatalf("normalizeBuilderRuntimePatch() error = %v, want repaired invalid escape", err)
	}
	if !normalized {
		t.Fatalf("normalized = false, want true")
	}
	if driftCount != 3 {
		t.Fatalf("driftCount = %d, want 3", driftCount)
	}
	if len(patch.Operations) != 1 {
		t.Fatalf("len(patch.Operations) = %d, want 1", len(patch.Operations))
	}
	if got := patch.Operations[0].Content; got != "Text('${summary.entryCount} 条记录')\n" {
		t.Fatalf("patch.Operations[0].Content = %q, want Dart interpolation without invalid escape", got)
	}
}

func TestNormalizeBuilderRuntimePatchRepairsEscapedQuotesOutsideStrings(t *testing.T) {
	raw := `{"patch_id":"repair-ui-colors-and-typos","operations":[{"operation":"write_file","path":"lib/views/home_page.dart","new_content":"class HomePage {}\n"},{\"operation":"write_file","path":"lib/views/record_detail_page.dart","new_content":"class RecordDetailPage {}\n"},{\"operation":"write_file","path":"lib/views/record_form_page.dart","new_content":"class RecordFormPage {}\n"}]}`

	patch, normalized, driftCount, err := normalizeBuilderRuntimePatch(raw, "round-escaped-ops", nil)
	if err != nil {
		t.Fatalf("normalizeBuilderRuntimePatch() error = %v, want repaired escaped operation objects", err)
	}
	if !normalized {
		t.Fatalf("normalized = false, want true")
	}
	if driftCount < 1 {
		t.Fatalf("driftCount = %d, want at least 1", driftCount)
	}
	if len(patch.Operations) != 3 {
		t.Fatalf("len(patch.Operations) = %d, want 3", len(patch.Operations))
	}
	for index, wantPath := range []string{"lib/views/home_page.dart", "lib/views/record_detail_page.dart", "lib/views/record_form_page.dart"} {
		if patch.Operations[index].Type != "write_file" {
			t.Fatalf("patch.Operations[%d].Type = %q, want write_file", index, patch.Operations[index].Type)
		}
		if patch.Operations[index].Path != wantPath {
			t.Fatalf("patch.Operations[%d].Path = %q, want %q", index, patch.Operations[index].Path, wantPath)
		}
	}
	if got := patch.Operations[1].Content; got != "class RecordDetailPage {}\n" {
		t.Fatalf("patch.Operations[1].Content = %q, want repaired record_detail content", got)
	}
	if got := patch.Operations[2].Content; got != "class RecordFormPage {}\n" {
		t.Fatalf("patch.Operations[2].Content = %q, want repaired record_form content", got)
	}
}

func TestMarkedFailureSignatureFromLog(t *testing.T) {
	logText := strings.Join([]string{
		"adb failed",
		"__oneappfactory_failure_signature__:device_check_failed:app_runtime_crash",
	}, "\n")
	if got := markedFailureSignatureFromLog(logText); got != "device_check_failed:app_runtime_crash" {
		t.Fatalf("markedFailureSignatureFromLog() = %q, want device_check_failed:app_runtime_crash", got)
	}
}

func TestDiagnoseExecutionStepFailureUsesMarkedDeviceFailureSignature(t *testing.T) {
	root := t.TempDir()
	workspacePath := filepath.Join(root, "workspace", "appfactory", "jobs", "job-1", "workspace")
	logPath := filepath.ToSlash(filepath.Join("jobs", "job-1", "runs", "run-1", "run.log"))
	resolvedLogPath := resolveRunFilePath(workspacePath, logPath)
	if err := os.MkdirAll(filepath.Dir(resolvedLogPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(log dir) error = %v", err)
	}
	if err := os.WriteFile(resolvedLogPath, []byte("adb output\n__oneappfactory_failure_signature__:device_check_failed:log_capture_failed\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(log) error = %v", err)
	}
	diagnosis := diagnoseExecutionStepFailure(ExecutionStep{
		Stage: appruns.StageSmoke,
		Check: &CheckExecutionPreview{
			CheckID:  "check-launch-app-and-capture-logcat",
			Label:    "launch app and capture logcat",
			Stage:    appruns.StageSmoke,
			Commands: []string{"adb logcat -d"},
		},
	}, errors.New("exit status 1"), workspacePath, logPath, false)
	if diagnosis.Signature != "device_check_failed:log_capture_failed" {
		t.Fatalf("diagnosis.Signature = %q, want device_check_failed:log_capture_failed", diagnosis.Signature)
	}
	if diagnosis.RecoverySuggestion != "fix adb logcat access and ensure runtime evidence can be captured before rerun" {
		t.Fatalf("diagnosis.RecoverySuggestion = %q, want log capture guidance", diagnosis.RecoverySuggestion)
	}
}
