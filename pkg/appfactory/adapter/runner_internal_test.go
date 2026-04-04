package adapter

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	appruns "github.com/sipeed/picoclaw/pkg/appfactory/runs"
)

func TestBuildSmokeReportIncludesDeviceAndSmokeSections(t *testing.T) {
	report := buildSmokeReport(
		[]appruns.CheckResult{
			{CheckID: "check-adb-device-ready", Stage: appruns.StageDevice, Outcome: "passed"},
			{CheckID: "check-launch-app-and-capture-logcat", Stage: appruns.StageSmoke, Outcome: "passed"},
		},
		[]appruns.CheckResult{{CheckID: "check-install-debug-apk", Stage: appruns.StageDevice, Outcome: "failed"}},
		[]appruns.DeviceFailureCategoryStat{{
			Category:      "device_check_failed:apk_install_failed",
			FailureDomain: "device",
			Count:         1,
		}},
	)
	for _, want := range []string{"## 设备验证", "check-adb-device-ready", "check-install-debug-apk", "## 冒烟检查", "check-launch-app-and-capture-logcat", "## 设备失败类别", "device_check_failed:apk_install_failed"} {
		if !strings.Contains(report, want) {
			t.Fatalf("buildSmokeReport() = %q, want substring %q", report, want)
		}
	}
}

func TestBuildDeviceFailureCategoryStatsAggregatesDeviceFailures(t *testing.T) {
	stats := buildDeviceFailureCategoryStats([]appruns.FailureSignature{
		{Signature: "device_check_failed:adb_device_unavailable", Count: 1, LastStage: appruns.StageDevice},
		{Signature: "environment_check_failed:debug_apk_missing", Count: 1, LastStage: appruns.StageDevice},
		{Signature: "device_check_failed:adb_device_unavailable", Count: 2, LastStage: appruns.StageSmoke},
		{Signature: "runner_exit_nonzero", Count: 1, LastStage: appruns.StageBaseline},
	})
	if len(stats) != 2 {
		t.Fatalf("len(stats) = %d, want 2", len(stats))
	}
	if stats[0].Category != "device_check_failed:adb_device_unavailable" || stats[0].FailureDomain != "device" || stats[0].Count != 3 {
		t.Fatalf("stats[0] = %+v, want adb device unavailable x3", stats[0])
	}
	if stats[1].Category != "environment_check_failed:debug_apk_missing" || stats[1].FailureDomain != "environment" || stats[1].Count != 1 {
		t.Fatalf("stats[1] = %+v, want debug_apk_missing x1", stats[1])
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
	debugAPKPath := filepath.Join(workspacePath, "build", "app", "outputs", "flutter-apk", "app-debug.apk")
	changeSummaryPath := filepath.Join(reportsDir, "change-summary.md")
	buildReportPath := filepath.Join(reportsDir, "build-report.md")
	smokeReportPath := filepath.Join(reportsDir, "smoke-test-report.md")
	deviceLogcatPath := filepath.Join(reportsDir, "device-logcat.txt")
	deviceScreenshotPath := filepath.Join(reportsDir, "device-screenshot.png")
	for _, item := range []string{debugAPKPath, changeSummaryPath, buildReportPath, smokeReportPath, deviceLogcatPath, deviceScreenshotPath} {
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
	for _, want := range []string{"run-log", "change-summary", "build-report", "smoke-test-report", "debug-apk", "device-logcat", "device-screenshot"} {
		if !found[want] {
			t.Fatalf("artifact %q missing from %+v", want, items)
		}
	}
	if len(primaryOutputs) != 2 {
		t.Fatalf("primary outputs len = %d, want 2", len(primaryOutputs))
	}
	if primaryOutputs[1].ArtifactID != "debug-apk" {
		t.Fatalf("primary output[1] = %+v, want debug-apk", primaryOutputs[1])
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

func TestMarkedFailureSignatureFromLog(t *testing.T) {
	logText := strings.Join([]string{
		"adb failed",
		"__picoclaw_failure_signature__:device_check_failed:app_runtime_crash",
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
	if err := os.WriteFile(resolvedLogPath, []byte("adb output\n__picoclaw_failure_signature__:device_check_failed:log_capture_failed\n"), 0o600); err != nil {
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
