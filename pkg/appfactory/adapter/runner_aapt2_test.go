package adapter

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	appruns "github.com/sipeed/oneappfactory/pkg/appfactory/runs"
)

func TestRepairAAPT2PermissionFailureRepairsExtractedBinary(t *testing.T) {
	workspacePath := filepath.Join(t.TempDir(), "workspace", "appfactory", "jobs", "job-1", "workspace")
	if err := os.MkdirAll(workspacePath, 0o755); err != nil {
		t.Fatalf("MkdirAll(workspace) error = %v", err)
	}
	gradleUserHome := resolveBuilderGradleUserHome(appFactoryRootFromWorkspace(workspacePath))
	binaryPath := filepath.Join(gradleUserHome, "caches", "8.10.2", "transforms", "demo", "transformed", "aapt2-8.7.0-12006047-linux", "aapt2")
	if err := os.MkdirAll(filepath.Dir(binaryPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(binary dir) error = %v", err)
	}
	if err := os.WriteFile(binaryPath, []byte("binary"), 0o644); err != nil {
		t.Fatalf("WriteFile(aapt2) error = %v", err)
	}
	logPath := filepath.Join(appFactoryRootFromWorkspace(workspacePath), "logs", "builder.log")
	logData := []byte("Caused by: java.io.IOException: Cannot run program \"" + binaryPath + "\": error=13, Permission denied\n")
	if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(log dir) error = %v", err)
	}
	if err := os.WriteFile(logPath, logData, 0o600); err != nil {
		t.Fatalf("WriteFile(log) error = %v", err)
	}

	repaired, err := repairAAPT2PermissionFailure(ExecutionStep{
		Check: &CheckExecutionPreview{CheckID: "check-flutter-build-apk", Stage: appruns.StageMilestone},
	}, workspacePath, logPath)
	if err != nil {
		t.Fatalf("repairAAPT2PermissionFailure() error = %v", err)
	}
	if !repaired {
		t.Fatal("repairAAPT2PermissionFailure() repaired = false, want true")
	}
	info, err := os.Stat(binaryPath)
	if err != nil {
		t.Fatalf("Stat(aapt2) error = %v", err)
	}
	if info.Mode()&0o111 == 0 {
		t.Fatalf("aapt2 mode = %v, want executable", info.Mode())
	}
}

func TestRunnerRunStepRetriesAfterRepairingAAPT2Permission(t *testing.T) {
	workspacePath := filepath.Join(t.TempDir(), "workspace", "appfactory", "jobs", "job-2", "workspace")
	if err := os.MkdirAll(workspacePath, 0o755); err != nil {
		t.Fatalf("MkdirAll(workspace) error = %v", err)
	}
	gradleUserHome := resolveBuilderGradleUserHome(appFactoryRootFromWorkspace(workspacePath))
	binaryPath := filepath.Join(gradleUserHome, "caches", "8.10.2", "transforms", "demo", "transformed", "aapt2-8.7.0-12006047-linux", "aapt2")
	if err := os.MkdirAll(filepath.Dir(binaryPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(binary dir) error = %v", err)
	}
	if err := os.WriteFile(binaryPath, []byte("binary"), 0o644); err != nil {
		t.Fatalf("WriteFile(aapt2) error = %v", err)
	}
	logPath := filepath.Join(appFactoryRootFromWorkspace(workspacePath), "logs", "builder.log")
	script := "if [ ! -x \"" + binaryPath + "\" ]; then echo 'Caused by: java.io.IOException: Cannot run program \"" + binaryPath + "\": error=13, Permission denied' >&2; exit 1; fi; echo apk-build-ok"
	cmd := exec.CommandContext(context.Background(), "/bin/sh", "-lc", script)
	cmd.Dir = workspacePath
	step := ExecutionStep{
		StepID:  "check-flutter-build-apk",
		Stage:   appruns.StageMilestone,
		Summary: "构建 Release APK",
		Command: cmd,
		Check: &CheckExecutionPreview{
			CheckID:  "check-flutter-build-apk",
			Label:    "构建 Release APK",
			Stage:    appruns.StageMilestone,
			Required: true,
		},
	}

	runner := &Runner{}
	if err := runner.runStep(step, workspacePath, logPath); err != nil {
		t.Fatalf("runStep() error = %v", err)
	}
	info, err := os.Stat(binaryPath)
	if err != nil {
		t.Fatalf("Stat(aapt2) error = %v", err)
	}
	if info.Mode()&0o111 == 0 {
		t.Fatalf("aapt2 mode = %v, want executable", info.Mode())
	}
	logData, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("ReadFile(log) error = %v", err)
	}
	if string(logData) == "" || !strings.Contains(string(logData), "apk-build-ok") {
		t.Fatalf("log missing retry success marker, log=%q", string(logData))
	}
}

func TestRepairAAPT2PermissionFailureRepairsDaemonStartupFailure(t *testing.T) {
	workspacePath := filepath.Join(t.TempDir(), "workspace", "appfactory", "jobs", "job-3", "workspace")
	if err := os.MkdirAll(workspacePath, 0o755); err != nil {
		t.Fatalf("MkdirAll(workspace) error = %v", err)
	}
	gradleUserHome := resolveBuilderGradleUserHome(appFactoryRootFromWorkspace(workspacePath))
	binaryPath := filepath.Join(gradleUserHome, "caches", "8.10.2", "transforms", "demo", "transformed", "aapt2-8.7.0-12006047-linux", "aapt2")
	if err := os.MkdirAll(filepath.Dir(binaryPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(binary dir) error = %v", err)
	}
	if err := os.WriteFile(binaryPath, []byte("binary"), 0o644); err != nil {
		t.Fatalf("WriteFile(aapt2) error = %v", err)
	}
	logPath := filepath.Join(appFactoryRootFromWorkspace(workspacePath), "logs", "builder.log")
	logData := []byte(strings.Join([]string{
		"Execution failed for task ':app:processDebugResources'.",
		"> A failure occurred while executing com.android.build.gradle.internal.res.LinkApplicationAndroidResourcesTask$TaskAction",
		"   > AAPT2 aapt2-8.7.0-12006047-linux Daemon #0: Daemon startup failed",
	}, "\n"))
	if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(log dir) error = %v", err)
	}
	if err := os.WriteFile(logPath, logData, 0o600); err != nil {
		t.Fatalf("WriteFile(log) error = %v", err)
	}

	repaired, err := repairAAPT2PermissionFailure(ExecutionStep{
		Check: &CheckExecutionPreview{CheckID: "check-flutter-build-apk", Stage: appruns.StageMilestone},
	}, workspacePath, logPath)
	if err != nil {
		t.Fatalf("repairAAPT2PermissionFailure() error = %v", err)
	}
	if !repaired {
		t.Fatal("repairAAPT2PermissionFailure() repaired = false, want true")
	}
	info, err := os.Stat(binaryPath)
	if err != nil {
		t.Fatalf("Stat(aapt2) error = %v", err)
	}
	if info.Mode()&0o111 == 0 {
		t.Fatalf("aapt2 mode = %v, want executable", info.Mode())
	}
}

func TestShouldSkipAutomaticValidationRepairForAAPT2EnvironmentFailure(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "builder.log")
	content := strings.Join([]string{
		"Execution failed for task ':app:processDebugResources'.",
		"> A failure occurred while executing com.android.build.gradle.internal.res.LinkApplicationAndroidResourcesTask$TaskAction",
		"   > AAPT2 aapt2-8.7.0-12006047-linux Daemon #0: Daemon startup failed",
	}, "\n")
	if err := os.WriteFile(logPath, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile(logPath) error = %v", err)
	}
	if !shouldSkipAutomaticValidationRepair(logPath, CheckExecutionPreview{CheckID: "check-flutter-build-apk"}) {
		t.Fatal("shouldSkipAutomaticValidationRepair() = false, want true")
	}
	if shouldSkipAutomaticValidationRepair(logPath, CheckExecutionPreview{CheckID: "check-flutter-analyze"}) {
		t.Fatal("shouldSkipAutomaticValidationRepair() = true for analyze, want false")
	}
}

func TestShouldSkipAutomaticValidationRepairForGradleDependencyDownloadFailure(t *testing.T) {
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
	if !shouldSkipAutomaticValidationRepair(logPath, CheckExecutionPreview{CheckID: "check-flutter-build-apk"}) {
		t.Fatal("shouldSkipAutomaticValidationRepair() = false, want true")
	}
	if shouldSkipAutomaticValidationRepair(logPath, CheckExecutionPreview{CheckID: "check-flutter-test"}) {
		t.Fatal("shouldSkipAutomaticValidationRepair() = true for test, want false")
	}
}
