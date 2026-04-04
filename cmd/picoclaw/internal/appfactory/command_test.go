package appfactory

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	appprepare "github.com/sipeed/picoclaw/pkg/appfactory/prepare"
	appruns "github.com/sipeed/picoclaw/pkg/appfactory/runs"
	"github.com/sipeed/picoclaw/pkg/config"
	api "github.com/sipeed/picoclaw/web/backend/api"
)

func TestPrepareCommandWritesArtifacts(t *testing.T) {
	outputDir := filepath.Join(t.TempDir(), "bookkeeping")
	cmd := NewAppFactoryCommand()
	stdout := &bytes.Buffer{}
	cmd.SetOut(stdout)
	cmd.SetErr(stdout)
	cmd.SetArgs([]string{
		"prepare",
		"--requirement", "做一个简单记账 app，不考虑上架，只考虑功能实现，需要首页概览、记一笔和账单列表。",
		"--output-dir", outputDir,
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v, output=%s", err, stdout.String())
	}
	for _, name := range []string{"requirement.md", "PRD.md", "PRD.json", "prd-approval.json", "template-approval.json", "template-fit-report.md", "implementation-plan.md", "manual-constraints.md", "builder-input.json"} {
		if _, err := os.Stat(filepath.Join(outputDir, name)); err != nil {
			t.Fatalf("expected %s to exist: %v", name, err)
		}
	}
	data, err := os.ReadFile(filepath.Join(outputDir, "builder-input.json"))
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	var input map[string]any
	if err := json.Unmarshal(data, &input); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if input["template_id"] != "flutter-finance-lite" {
		t.Fatalf("template_id = %v, want flutter-finance-lite", input["template_id"])
	}
	if input["prd_id"] != "prd-bookkeeping-lite" {
		t.Fatalf("prd_id = %v, want prd-bookkeeping-lite", input["prd_id"])
	}
}

func TestLoadClosestAppFactoryEnvFromConfigPath(t *testing.T) {
	rootDir := t.TempDir()
	childDir := filepath.Join(rootDir, "nested", "workspace")
	configPath := filepath.Join(childDir, "config.json")
	if err := os.MkdirAll(childDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(rootDir, ".env"), []byte("PICOLAW_APPFACTORY_TEST=enabled\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(.env) error = %v", err)
	}
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}
	oldValue, hadValue := os.LookupEnv("PICOLAW_APPFACTORY_TEST")
	t.Cleanup(func() {
		if err := os.Chdir(oldWD); err != nil {
			t.Fatalf("Chdir(restore) error = %v", err)
		}
		if hadValue {
			if err := os.Setenv("PICOLAW_APPFACTORY_TEST", oldValue); err != nil {
				t.Fatalf("Setenv(restore) error = %v", err)
			}
		} else {
			if err := os.Unsetenv("PICOLAW_APPFACTORY_TEST"); err != nil {
				t.Fatalf("Unsetenv(restore) error = %v", err)
			}
		}
	})
	if err := os.Unsetenv("PICOLAW_APPFACTORY_TEST"); err != nil {
		t.Fatalf("Unsetenv() error = %v", err)
	}
	if err := os.Chdir(childDir); err != nil {
		t.Fatalf("Chdir() error = %v", err)
	}
	if err := loadClosestAppFactoryEnv(configPath); err != nil {
		t.Fatalf("loadClosestAppFactoryEnv() error = %v", err)
	}
	if got := os.Getenv("PICOLAW_APPFACTORY_TEST"); got != "enabled" {
		t.Fatalf("PICOLAW_APPFACTORY_TEST = %q, want enabled", got)
	}
}

func TestPrepareCommandWritesRealArtifacts(t *testing.T) {
	outputDir := filepath.Join(t.TempDir(), "bookkeeping-real")
	cmd := NewAppFactoryCommand()
	stdout := &bytes.Buffer{}
	cmd.SetOut(stdout)
	cmd.SetErr(stdout)
	cmd.SetArgs([]string{
		"prepare",
		"--requirement", "做一个简单记账 app，不考虑上架，只考虑功能实现，需要首页概览、记一笔和账单列表。",
		"--output-dir", outputDir,
		"--real-checks",
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v, output=%s", err, stdout.String())
	}
	data, err := os.ReadFile(filepath.Join(outputDir, "builder-input.json"))
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	var input map[string]any
	if err := json.Unmarshal(data, &input); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if input["executor_image"] != "picoclaw/appfactory-builder:local" {
		t.Fatalf("executor_image = %v, want picoclaw/appfactory-builder:local", input["executor_image"])
	}
	checks, ok := input["acceptance_checks"].([]any)
	if !ok || len(checks) != 7 {
		t.Fatalf("acceptance_checks = %v, want 7 real checks", input["acceptance_checks"])
	}
	checkIDs := make([]string, 0, len(checks))
	for _, item := range checks {
		check, ok := item.(map[string]any)
		if !ok {
			t.Fatalf("acceptance_check item type = %T, want map[string]any", item)
		}
		checkID, _ := check["check_id"].(string)
		checkIDs = append(checkIDs, checkID)
	}
	wantCheckIDs := []string{
		"check-flutter-pub-get",
		"check-counter-demo-removed",
		"check-entry-form-wiring",
		"check-local-persistence-wiring",
		"check-flutter-analyze",
		"check-flutter-test",
		"check-flutter-build-apk",
	}
	for index, want := range wantCheckIDs {
		if checkIDs[index] != want {
			t.Fatalf("acceptance_check[%d] = %q, want %q", index, checkIDs[index], want)
		}
	}
}

func TestRunOnceCommandCompletesRun(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	cfg := config.DefaultConfig()
	cfg.Agents.Defaults.Workspace = filepath.Join(filepath.Dir(configPath), "workspace")
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}
	h := api.NewHandler(configPath)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	server := httptest.NewServer(mux)
	defer server.Close()

	bundleDir := filepath.Join(t.TempDir(), "bundle")
	bundle, err := appprepare.Compile(appprepare.Request{
		RequirementText: "做一个简单记账 app，不考虑上架，只考虑功能实现，需要首页概览、记一笔和账单列表。",
	})
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	if err := appprepare.WriteBundle(bundleDir, bundle); err != nil {
		t.Fatalf("WriteBundle() error = %v", err)
	}
	inputPath := filepath.Join(bundleDir, "builder-input.json")

	cmd := NewAppFactoryCommand()
	stdout := &bytes.Buffer{}
	cmd.SetOut(stdout)
	cmd.SetErr(stdout)
	cmd.SetArgs([]string{
		"run-executor-once",
		"--api-base", server.URL,
		"--input", inputPath,
		"--builder-id", "builder-a",
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v, output=%s", err, stdout.String())
	}
	runID := parseRunID(stdout.String())
	if runID == "" {
		t.Fatalf("expected run_id in output, got %q", stdout.String())
	}
	run := getJSON(t, server.Client(), server.URL+"/internal/v1/build-runs/"+runID)
	if run["status"] != "completed" {
		t.Fatalf("run status = %v, want completed", run["status"])
	}
	allocateAgain := postJSON(t, server.Client(), server.URL+"/internal/v1/workers:allocate", map[string]any{"job_id": "job-2"})
	if allocateAgain["worker_id"] != "builder-a" {
		t.Fatalf("worker_id after run-executor-once = %v, want builder-a", allocateAgain["worker_id"])
	}
}

func TestRunRequirementCommandCompletesRunAndWritesBundle(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	cfg := config.DefaultConfig()
	cfg.Agents.Defaults.Workspace = filepath.Join(filepath.Dir(configPath), "workspace")
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}
	h := api.NewHandler(configPath)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	server := httptest.NewServer(mux)
	defer server.Close()

	outputDir := filepath.Join(t.TempDir(), "bundle")
	cmd := NewAppFactoryCommand()
	stdout := &bytes.Buffer{}
	cmd.SetOut(stdout)
	cmd.SetErr(stdout)
	cmd.SetArgs([]string{
		"run-requirement",
		"--api-base", server.URL,
		"--requirement", "做一个简单记账 app，不考虑上架，只考虑功能实现，需要首页概览、记一笔和账单列表。",
		"--output-dir", outputDir,
		"--builder-id", "builder-a",
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v, output=%s", err, stdout.String())
	}
	for _, name := range []string{"requirement.md", "PRD.md", "PRD.json", "prd-approval.json", "template-approval.json", "template-fit-report.md", "implementation-plan.md", "manual-constraints.md", "builder-input.json"} {
		if _, err := os.Stat(filepath.Join(outputDir, name)); err != nil {
			t.Fatalf("expected %s to exist: %v", name, err)
		}
	}
	runID := parseKeyValue(stdout.String(), "run_id")
	if runID == "" {
		t.Fatalf("expected run_id in output, got %q", stdout.String())
	}
	if bundleDir := parseKeyValue(stdout.String(), "bundle_dir"); bundleDir != outputDir {
		t.Fatalf("bundle_dir = %q, want %q", bundleDir, outputDir)
	}
	run := getJSON(t, server.Client(), server.URL+"/internal/v1/build-runs/"+runID)
	if run["status"] != "completed" {
		t.Fatalf("run status = %v, want completed", run["status"])
	}
}

func TestOrchestratorRunCommandCompletesSinglePassWithoutWork(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	cfg := config.DefaultConfig()
	workspace := filepath.Join(filepath.Dir(configPath), "workspace")
	cfg.Agents.Defaults.Workspace = workspace
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	cmd := NewAppFactoryCommand()
	stdout := &bytes.Buffer{}
	cmd.SetOut(stdout)
	cmd.SetErr(stdout)
	cmd.SetArgs([]string{"orchestrator", "run", "--config", configPath})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v, output=%s", err, stdout.String())
	}
	if !strings.Contains(stdout.String(), "mode=once") {
		t.Fatalf("output = %q, want mode=once", stdout.String())
	}
	if !strings.Contains(stdout.String(), "state=idle") {
		t.Fatalf("output = %q, want state=idle", stdout.String())
	}
	if !strings.Contains(stdout.String(), configPath) {
		t.Fatalf("output = %q, want config path %q", stdout.String(), configPath)
	}
	statusPath := filepath.Join(workspace, "appfactory", "orchestrator", "status.json")
	if _, err := os.Stat(statusPath); err != nil {
		t.Fatalf("expected status snapshot %s to exist: %v", statusPath, err)
	}
}

func TestOrchestratorRunCommandWatchStopsAfterMaxPasses(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	cfg := config.DefaultConfig()
	workspace := filepath.Join(filepath.Dir(configPath), "workspace")
	cfg.Agents.Defaults.Workspace = workspace
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	cmd := NewAppFactoryCommand()
	stdout := &bytes.Buffer{}
	cmd.SetOut(stdout)
	cmd.SetErr(stdout)
	cmd.SetArgs([]string{"orchestrator", "run", "--config", configPath, "--watch", "--interval", "10ms", "--max-passes", "2"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v, output=%s", err, stdout.String())
	}
	if !strings.Contains(stdout.String(), "mode=watch") {
		t.Fatalf("output = %q, want mode=watch", stdout.String())
	}
	if !strings.Contains(stdout.String(), "watch_runner_mode=cli_watch") {
		t.Fatalf("output = %q, want watch_runner_mode=cli_watch", stdout.String())
	}
	if !strings.Contains(stdout.String(), "completed_passes=2") {
		t.Fatalf("output = %q, want completed_passes=2", stdout.String())
	}
	lockPath := filepath.Join(workspace, "appfactory", "orchestrator", "watch-lock.json")
	if _, err := os.Stat(lockPath); !os.IsNotExist(err) {
		t.Fatalf("watch lock should be released after bounded watch run, stat err=%v", err)
	}
	statusPath := filepath.Join(workspace, "appfactory", "orchestrator", "status.json")
	statusData, err := os.ReadFile(statusPath)
	if err != nil {
		t.Fatalf("ReadFile(status.json) error = %v", err)
	}
	var status map[string]any
	if err := json.Unmarshal(statusData, &status); err != nil {
		t.Fatalf("Unmarshal(status.json) error = %v", err)
	}
	if status["last_trigger"] != "cli_watch" {
		t.Fatalf("last_trigger = %v, want cli_watch", status["last_trigger"])
	}
	if status["watch_runner_state"] != "not_running" {
		t.Fatalf("watch_runner_state = %v, want not_running after watch exit", status["watch_runner_state"])
	}
	if status["watch_runner_mode"] != "cli_watch" {
		t.Fatalf("watch_runner_mode = %v, want cli_watch", status["watch_runner_mode"])
	}
}

func TestOrchestratorRunCommandWatchRejectsForeignActiveLock(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	cfg := config.DefaultConfig()
	workspace := filepath.Join(filepath.Dir(configPath), "workspace")
	cfg.Agents.Defaults.Workspace = workspace
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}
	lockPath := filepath.Join(workspace, "appfactory", "orchestrator", "watch-lock.json")
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(lockPath dir) error = %v", err)
	}
	lockPayload := map[string]any{
		"schema_version": "0.1.0",
		"owner_id":       "orchestrator-foreign",
		"mode":           "cli_watch",
		"acquired_at":    time.Now().UTC().Add(-10 * time.Second).Format(time.RFC3339),
		"updated_at":     time.Now().UTC().Format(time.RFC3339),
		"expires_at":     time.Now().UTC().Add(30 * time.Second).Format(time.RFC3339),
	}
	lockData, err := json.Marshal(lockPayload)
	if err != nil {
		t.Fatalf("Marshal(lockPayload) error = %v", err)
	}
	if err := os.WriteFile(lockPath, append(lockData, '\n'), 0o644); err != nil {
		t.Fatalf("WriteFile(watch-lock.json) error = %v", err)
	}

	cmd := NewAppFactoryCommand()
	stdout := &bytes.Buffer{}
	cmd.SetOut(stdout)
	cmd.SetErr(stdout)
	cmd.SetArgs([]string{"orchestrator", "run", "--config", configPath, "--watch", "--interval", "10ms", "--max-passes", "1"})
	err = cmd.Execute()
	if err == nil {
		t.Fatalf("Execute() error = nil, want active watch lock conflict")
	}
	if !strings.Contains(err.Error(), "already held by orchestrator-foreign") {
		t.Fatalf("error = %v, want foreign watch lock conflict", err)
	}
}

func TestOrchestratorStatusCommandPrintsWatchLockFields(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	cfg := config.DefaultConfig()
	workspace := filepath.Join(filepath.Dir(configPath), "workspace")
	cfg.Agents.Defaults.Workspace = workspace
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}
	statusPath := filepath.Join(workspace, "appfactory", "orchestrator", "status.json")
	if err := os.MkdirAll(filepath.Dir(statusPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(statusPath dir) error = %v", err)
	}
	statusPayload := map[string]any{
		"schema_version":           "0.1.0",
		"instance_id":              "orchestrator-status-test",
		"state":                    "idle",
		"last_trigger":             "cli_watch",
		"watch_runner_state":       "running",
		"watch_runner_mode":        "cli_watch",
		"watch_runner_owner_id":    "orchestrator-foreign",
		"last_pass_started_at":     time.Now().UTC().Add(-2 * time.Second).Format(time.RFC3339),
		"last_pass_finished_at":    time.Now().UTC().Add(-1 * time.Second).Format(time.RFC3339),
		"queued_recoveries":        1,
		"running_recoveries":       2,
		"foreign_live_lease_skips": 3,
		"updated_at":               time.Now().UTC().Format(time.RFC3339),
	}
	statusData, err := json.Marshal(statusPayload)
	if err != nil {
		t.Fatalf("Marshal(statusPayload) error = %v", err)
	}
	if err := os.WriteFile(statusPath, append(statusData, '\n'), 0o644); err != nil {
		t.Fatalf("WriteFile(status.json) error = %v", err)
	}
	lockPath := filepath.Join(workspace, "appfactory", "orchestrator", "watch-lock.json")
	lockPayload := map[string]any{
		"schema_version": "0.1.0",
		"owner_id":       "orchestrator-foreign",
		"mode":           "cli_watch",
		"acquired_at":    time.Now().UTC().Add(-20 * time.Second).Format(time.RFC3339),
		"updated_at":     time.Now().UTC().Add(-5 * time.Second).Format(time.RFC3339),
		"expires_at":     time.Now().UTC().Add(30 * time.Second).Format(time.RFC3339),
	}
	lockData, err := json.Marshal(lockPayload)
	if err != nil {
		t.Fatalf("Marshal(lockPayload) error = %v", err)
	}
	if err := os.WriteFile(lockPath, append(lockData, '\n'), 0o644); err != nil {
		t.Fatalf("WriteFile(watch-lock.json) error = %v", err)
	}

	cmd := NewAppFactoryCommand()
	stdout := &bytes.Buffer{}
	cmd.SetOut(stdout)
	cmd.SetErr(stdout)
	cmd.SetArgs([]string{"orchestrator", "status", "--config", configPath})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v, output=%s", err, stdout.String())
	}
	if !strings.Contains(stdout.String(), "watch_lock_state=held_by_other") {
		t.Fatalf("output = %q, want watch_lock_state=held_by_other", stdout.String())
	}
	if !strings.Contains(stdout.String(), "watch_runner_state=running") {
		t.Fatalf("output = %q, want watch_runner_state=running", stdout.String())
	}
	if !strings.Contains(stdout.String(), "watch_runner_mode=cli_watch") {
		t.Fatalf("output = %q, want watch_runner_mode=cli_watch", stdout.String())
	}
	if !strings.Contains(stdout.String(), "watch_lock_mode=cli_watch") {
		t.Fatalf("output = %q, want watch_lock_mode=cli_watch", stdout.String())
	}
	if !strings.Contains(stdout.String(), "foreign_live_lease_skips=3") {
		t.Fatalf("output = %q, want foreign_live_lease_skips=3", stdout.String())
	}
}

func TestOrchestratorUnlockWatchCommandRequiresForceForForeignActiveLock(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	cfg := config.DefaultConfig()
	workspace := filepath.Join(filepath.Dir(configPath), "workspace")
	cfg.Agents.Defaults.Workspace = workspace
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}
	lockPath := filepath.Join(workspace, "appfactory", "orchestrator", "watch-lock.json")
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(lockPath dir) error = %v", err)
	}
	lockPayload := map[string]any{
		"schema_version": "0.1.0",
		"owner_id":       "orchestrator-foreign",
		"mode":           "cli_watch",
		"acquired_at":    time.Now().UTC().Add(-10 * time.Second).Format(time.RFC3339),
		"updated_at":     time.Now().UTC().Format(time.RFC3339),
		"expires_at":     time.Now().UTC().Add(30 * time.Second).Format(time.RFC3339),
	}
	lockData, err := json.Marshal(lockPayload)
	if err != nil {
		t.Fatalf("Marshal(lockPayload) error = %v", err)
	}
	if err := os.WriteFile(lockPath, append(lockData, '\n'), 0o644); err != nil {
		t.Fatalf("WriteFile(watch-lock.json) error = %v", err)
	}

	cmd := NewAppFactoryCommand()
	stdout := &bytes.Buffer{}
	cmd.SetOut(stdout)
	cmd.SetErr(stdout)
	cmd.SetArgs([]string{"orchestrator", "unlock-watch", "--config", configPath})
	err = cmd.Execute()
	if err == nil {
		t.Fatalf("Execute() error = nil, want foreign watch lock conflict")
	}
	if !strings.Contains(err.Error(), "rerun with force to unlock") {
		t.Fatalf("error = %v, want force hint", err)
	}
}

func TestOrchestratorUnlockWatchCommandForceClearsForeignActiveLock(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	cfg := config.DefaultConfig()
	workspace := filepath.Join(filepath.Dir(configPath), "workspace")
	cfg.Agents.Defaults.Workspace = workspace
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}
	lockPath := filepath.Join(workspace, "appfactory", "orchestrator", "watch-lock.json")
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(lockPath dir) error = %v", err)
	}
	lockPayload := map[string]any{
		"schema_version": "0.1.0",
		"owner_id":       "orchestrator-foreign",
		"mode":           "cli_watch",
		"acquired_at":    time.Now().UTC().Add(-10 * time.Second).Format(time.RFC3339),
		"updated_at":     time.Now().UTC().Format(time.RFC3339),
		"expires_at":     time.Now().UTC().Add(30 * time.Second).Format(time.RFC3339),
	}
	lockData, err := json.Marshal(lockPayload)
	if err != nil {
		t.Fatalf("Marshal(lockPayload) error = %v", err)
	}
	if err := os.WriteFile(lockPath, append(lockData, '\n'), 0o644); err != nil {
		t.Fatalf("WriteFile(watch-lock.json) error = %v", err)
	}

	cmd := NewAppFactoryCommand()
	stdout := &bytes.Buffer{}
	cmd.SetOut(stdout)
	cmd.SetErr(stdout)
	cmd.SetArgs([]string{"orchestrator", "unlock-watch", "--config", configPath, "--force"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v, output=%s", err, stdout.String())
	}
	if !strings.Contains(stdout.String(), "watch_lock_state=unlocked") {
		t.Fatalf("output = %q, want watch_lock_state=unlocked", stdout.String())
	}
	if _, err := os.Stat(lockPath); !os.IsNotExist(err) {
		t.Fatalf("watch lock should be removed, stat err=%v", err)
	}
}

func TestPrepareThenRunOnceWritesStagedBuilderOutput(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	cfg := config.DefaultConfig()
	cfg.Agents.Defaults.Workspace = filepath.Join(filepath.Dir(configPath), "workspace")
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}
	h := api.NewHandler(configPath)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	server := httptest.NewServer(mux)
	defer server.Close()

	bundleDir := filepath.Join(t.TempDir(), "bundle")
	prepareCmd := NewAppFactoryCommand()
	prepareStdout := &bytes.Buffer{}
	prepareCmd.SetOut(prepareStdout)
	prepareCmd.SetErr(prepareStdout)
	prepareCmd.SetArgs([]string{
		"prepare",
		"--requirement", "做一个简单记账 app，不考虑上架，只考虑功能实现，需要首页概览、记一笔和账单列表。",
		"--output-dir", bundleDir,
	})
	if err := prepareCmd.Execute(); err != nil {
		t.Fatalf("prepare Execute() error = %v, output=%s", err, prepareStdout.String())
	}

	inputPath := filepath.Join(bundleDir, "builder-input.json")
	inputData, err := os.ReadFile(inputPath)
	if err != nil {
		t.Fatalf("ReadFile(builder-input.json) error = %v", err)
	}
	var input appruns.BuildInput
	if err := json.Unmarshal(inputData, &input); err != nil {
		t.Fatalf("Unmarshal(builder-input.json) error = %v", err)
	}
	for _, name := range []string{"prd-approval.json", "template-approval.json"} {
		if _, err := os.Stat(filepath.Join(bundleDir, name)); err != nil {
			t.Fatalf("expected %s to exist: %v", name, err)
		}
	}

	runCmd := NewAppFactoryCommand()
	runStdout := &bytes.Buffer{}
	runCmd.SetOut(runStdout)
	runCmd.SetErr(runStdout)
	runCmd.SetArgs([]string{
		"run-executor-once",
		"--api-base", server.URL,
		"--input", inputPath,
		"--builder-id", "builder-a",
	})
	if err := runCmd.Execute(); err != nil {
		t.Fatalf("run-executor-once Execute() error = %v, output=%s", err, runStdout.String())
	}
	runID := parseRunID(runStdout.String())
	if runID == "" {
		t.Fatalf("expected run_id in output, got %q", runStdout.String())
	}

	run := getJSON(t, server.Client(), server.URL+"/internal/v1/build-runs/"+runID)
	if run["status"] != "completed" {
		t.Fatalf("run status = %v, want completed", run["status"])
	}
	if run["last_stage"] != "cheap" {
		t.Fatalf("last_stage = %v, want cheap", run["last_stage"])
	}

	outputRel, _ := run["output_path"].(string)
	if outputRel == "" {
		t.Fatalf("output_path = %v, want non-empty", run["output_path"])
	}
	outputPath := filepath.Join(cfg.Agents.Defaults.Workspace, "appfactory", filepath.FromSlash(outputRel))
	outputData, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("ReadFile(builder-output.json) error = %v", err)
	}
	var output appruns.BuildOutput
	if err := json.Unmarshal(outputData, &output); err != nil {
		t.Fatalf("Unmarshal(builder-output.json) error = %v", err)
	}
	if output.Status != "success" {
		t.Fatalf("output.Status = %q, want success", output.Status)
	}
	if len(output.ChecksPassed) != 6 {
		t.Fatalf("len(output.ChecksPassed) = %d, want 6", len(output.ChecksPassed))
	}
	if len(output.ChecksFailed) != 0 {
		t.Fatalf("len(output.ChecksFailed) = %d, want 0", len(output.ChecksFailed))
	}
	passedCheckIDs := make(map[string]bool, len(output.ChecksPassed))
	for _, check := range output.ChecksPassed {
		passedCheckIDs[check.CheckID] = true
	}
	for _, want := range []string{
		"check-context-ready",
		"check-bookkeeping-scope",
		"check-structural-template-files-ready",
		"check-legacy-thin-fallback-probe",
		"check-legacy-thin-fallback-metadata",
	} {
		if !passedCheckIDs[want] {
			t.Fatalf("output.ChecksPassed missing %q: %#v", want, output.ChecksPassed)
		}
	}
	if passedCheckIDs["check-counter-demo-removed"] || passedCheckIDs["check-entry-form-wiring"] || passedCheckIDs["check-local-persistence-wiring"] {
		t.Fatalf("output.ChecksPassed should not contain legacy default-executor business checks: %#v", output.ChecksPassed)
	}
	if output.Metrics.TotalIterations != 7 {
		t.Fatalf("output.Metrics.TotalIterations = %d, want 7", output.Metrics.TotalIterations)
	}

	jobRoot := filepath.Join(cfg.Agents.Defaults.Workspace, "appfactory", "jobs", input.JobID)
	for _, rel := range []string{
		output.ReportPaths.ChangeSummaryPath,
		output.ReportPaths.BuildReportPath,
		output.ReportPaths.SmokeTestReportPath,
	} {
		if rel == "" {
			t.Fatal("expected report path to be non-empty")
		}
		if _, err := os.Stat(filepath.Join(jobRoot, filepath.FromSlash(rel))); err != nil {
			t.Fatalf("expected report %s to exist: %v", rel, err)
		}
	}
	for _, rel := range []string{output.Artifacts.ManifestPath, output.Metrics.MetricsPath} {
		if rel == "" {
			t.Fatal("expected manifest/metrics path to be non-empty")
		}
		if _, err := os.Stat(filepath.Join(cfg.Agents.Defaults.Workspace, "appfactory", filepath.FromSlash(rel))); err != nil {
			t.Fatalf("expected artifact file %s to exist: %v", rel, err)
		}
	}
	logRel, _ := run["log_path"].(string)
	if logRel == "" {
		t.Fatalf("log_path = %v, want non-empty", run["log_path"])
	}
	if _, err := os.Stat(filepath.Join(cfg.Agents.Defaults.Workspace, "appfactory", filepath.FromSlash(logRel))); err != nil {
		t.Fatalf("expected log file %s to exist: %v", logRel, err)
	}
	allocateAgain := postJSON(t, server.Client(), server.URL+"/internal/v1/workers:allocate", map[string]any{"job_id": "job-2"})
	if allocateAgain["worker_id"] != "builder-a" {
		t.Fatalf("worker_id after staged run = %v, want builder-a", allocateAgain["worker_id"])
	}
}

func TestRunOnceFailsWhenApprovalSnapshotRejected(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	cfg := config.DefaultConfig()
	cfg.Agents.Defaults.Workspace = filepath.Join(filepath.Dir(configPath), "workspace")
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}
	h := api.NewHandler(configPath)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	server := httptest.NewServer(mux)
	defer server.Close()

	bundleDir := filepath.Join(t.TempDir(), "bundle")
	prepareCmd := NewAppFactoryCommand()
	prepareStdout := &bytes.Buffer{}
	prepareCmd.SetOut(prepareStdout)
	prepareCmd.SetErr(prepareStdout)
	prepareCmd.SetArgs([]string{
		"prepare",
		"--requirement", "做一个简单记账 app，不考虑上架，只考虑功能实现，需要首页概览、记一笔和账单列表。",
		"--output-dir", bundleDir,
	})
	if err := prepareCmd.Execute(); err != nil {
		t.Fatalf("prepare Execute() error = %v, output=%s", err, prepareStdout.String())
	}

	approvalPath := filepath.Join(bundleDir, appruns.PRDApprovalFileName)
	approvalData, err := os.ReadFile(approvalPath)
	if err != nil {
		t.Fatalf("ReadFile(prd-approval.json) error = %v", err)
	}
	var approval appruns.ApprovalRecord
	if err := json.Unmarshal(approvalData, &approval); err != nil {
		t.Fatalf("Unmarshal(prd-approval.json) error = %v", err)
	}
	approval.Status = "rejected"
	if approval.Decision != nil {
		approval.Decision.Decision = "rejected"
		approval.Decision.Comment = "test injected rejection"
	}
	updatedApproval, err := json.MarshalIndent(approval, "", "  ")
	if err != nil {
		t.Fatalf("MarshalIndent(prd approval) error = %v", err)
	}
	if err := os.WriteFile(approvalPath, updatedApproval, 0o600); err != nil {
		t.Fatalf("WriteFile(prd-approval.json) error = %v", err)
	}

	inputPath := filepath.Join(bundleDir, "builder-input.json")
	runCmd := NewAppFactoryCommand()
	runStdout := &bytes.Buffer{}
	runCmd.SetOut(runStdout)
	runCmd.SetErr(runStdout)
	runCmd.SetArgs([]string{
		"run-executor-once",
		"--api-base", server.URL,
		"--input", inputPath,
		"--builder-id", "builder-a",
	})
	err = runCmd.Execute()
	if err == nil {
		t.Fatalf("run-executor-once Execute() error = nil, want approval gate failure")
	}
	if !strings.Contains(err.Error(), "status=failed") || !strings.Contains(err.Error(), appruns.PRDApprovalFileName) {
		t.Fatalf("run-executor-once error = %v, want failed status with approval detail", err)
	}
	runID := parseKeyValue(strings.ReplaceAll(err.Error(), " ", "\n"), "run_id")
	run := getJSON(t, server.Client(), server.URL+"/internal/v1/build-runs/"+runID)
	if run["status"] != "failed" {
		t.Fatalf("run status = %v, want failed", run["status"])
	}
	failureSummary, _ := run["failure_summary"].(string)
	if !strings.Contains(failureSummary, appruns.PRDApprovalFileName) || !strings.Contains(failureSummary, "status=rejected") {
		t.Fatalf("failure_summary = %q, want approval gate rejection", failureSummary)
	}
	if run["last_stage"] != nil && run["last_stage"] != "" {
		t.Fatalf("last_stage = %v, want empty before execution", run["last_stage"])
	}
	allocateAgain := postJSON(t, server.Client(), server.URL+"/internal/v1/workers:allocate", map[string]any{"job_id": "job-2"})
	if allocateAgain["worker_id"] != "builder-a" {
		t.Fatalf("worker_id after rejected approval = %v, want builder-a", allocateAgain["worker_id"])
	}
}

func parseRunID(output string) string {
	return parseKeyValue(output, "run_id")
}

func parseKeyValue(output, key string) string {
	prefix := []byte(key + "=")
	for _, line := range bytes.Split([]byte(output), []byte("\n")) {
		if bytes.HasPrefix(line, prefix) {
			return string(bytes.TrimPrefix(line, prefix))
		}
	}
	return ""
}

func postJSON(t *testing.T, client *http.Client, url string, payload map[string]any) map[string]any {
	t.Helper()
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		var failure map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&failure)
		t.Fatalf("POST %s status = %d body=%v", url, resp.StatusCode, failure)
	}
	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	return result
}

func getJSON(t *testing.T, client *http.Client, url string) map[string]any {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		var failure map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&failure)
		t.Fatalf("GET %s status = %d body=%v", url, resp.StatusCode, failure)
	}
	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	return result
}
