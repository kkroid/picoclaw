package adapter_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	adapter "github.com/sipeed/picoclaw/pkg/appfactory/adapter"
	appruns "github.com/sipeed/picoclaw/pkg/appfactory/runs"
	"github.com/sipeed/picoclaw/pkg/config"
	api "github.com/sipeed/picoclaw/web/backend/api"
)

func TestRunnerExecuteRunCompletesAndReleasesBuilder(t *testing.T) {
	t.Setenv("APPFACTORY_BUILDER_RUNTIME", "")
	t.Setenv("APPFACTORY_BUILDER_DOCKER_NETWORK", "")
	t.Setenv("HTTP_PROXY", "")
	t.Setenv("HTTPS_PROXY", "")
	t.Setenv("ALL_PROXY", "")
	t.Setenv("NO_PROXY", "")

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

	postJSON(t, server.Client(), server.URL+"/internal/v1/builders:register", map[string]any{
		"builder_id":        "builder-a",
		"display_name":      "Builder A",
		"capability_tags":   []string{"flutter"},
		"max_parallel_runs": 1,
		"worker_profile":    map[string]any{"image": "builder:latest"},
	})
	allocate := postJSON(t, server.Client(), server.URL+"/internal/v1/workers:allocate", map[string]any{"job_id": "job-1"})
	leaseID, _ := allocate["lease_id"].(string)
	create := postJSON(t, server.Client(), server.URL+"/internal/v1/build-runs", map[string]any{
		"worker_id":     "builder-a",
		"lease_id":      leaseID,
		"builder_input": sampleBuildRunInput(),
	})
	runID, _ := create["run_id"].(string)

	runner := adapter.NewRunner(server.URL)
	if err := runner.ExecuteRun(context.Background(), runID); err != nil {
		t.Fatalf("ExecuteRun() error = %v", err)
	}
	run := getJSON(t, server.Client(), server.URL+"/internal/v1/build-runs/"+runID)
	if run["status"] != "completed" {
		t.Fatalf("run status = %v, want completed (failure_summary=%v)", run["status"], run["failure_summary"])
	}
	if run["last_stage"] != "baseline" {
		t.Fatalf("last_stage = %v, want baseline", run["last_stage"])
	}
	builderOutput := readBuilderOutput(t, run)
	roundOutputs, ok := builderOutput["round_outputs"].([]any)
	if !ok || len(roundOutputs) != 1 {
		t.Fatalf("round_outputs = %T %#v, want one round output", builderOutput["round_outputs"], builderOutput["round_outputs"])
	}
	firstRound, ok := roundOutputs[0].(map[string]any)
	if !ok {
		t.Fatalf("round_output[0] = %T, want object", roundOutputs[0])
	}
	workspacePatch, ok := firstRound["workspace_patch"].(map[string]any)
	if !ok {
		t.Fatalf("workspace_patch = %T, want object", firstRound["workspace_patch"])
	}
	if workspacePatch["status"] != "applied" {
		t.Fatalf("workspace_patch.status = %v, want applied", workspacePatch["status"])
	}
	allocateAgain := postJSON(t, server.Client(), server.URL+"/internal/v1/workers:allocate", map[string]any{"job_id": "job-2"})
	if allocateAgain["worker_id"] != "builder-a" {
		t.Fatalf("worker_id after run complete = %v, want builder-a", allocateAgain["worker_id"])
	}
}

func TestRunnerExecuteRunUsesInjectedThinExecutor(t *testing.T) {
	t.Setenv("APPFACTORY_BUILDER_RUNTIME", "")
	t.Setenv("APPFACTORY_BUILDER_DOCKER_NETWORK", "")
	t.Setenv("HTTP_PROXY", "")
	t.Setenv("HTTPS_PROXY", "")
	t.Setenv("ALL_PROXY", "")
	t.Setenv("NO_PROXY", "")

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

	postJSON(t, server.Client(), server.URL+"/internal/v1/builders:register", map[string]any{
		"builder_id":        "builder-a",
		"display_name":      "Builder A",
		"capability_tags":   []string{"flutter"},
		"max_parallel_runs": 1,
		"worker_profile":    map[string]any{"image": "builder:latest"},
	})
	allocate := postJSON(t, server.Client(), server.URL+"/internal/v1/workers:allocate", map[string]any{"job_id": "job-1"})
	leaseID, _ := allocate["lease_id"].(string)
	create := postJSON(t, server.Client(), server.URL+"/internal/v1/build-runs", map[string]any{
		"worker_id":     "builder-a",
		"lease_id":      leaseID,
		"builder_input": sampleBuildRunInput(),
	})
	runID, _ := create["run_id"].(string)

	fake := &stubThinExecutor{workspace: t.TempDir()}
	runner := adapter.NewRunner(server.URL)
	runner.Executor = fake
	if err := runner.ExecuteRun(context.Background(), runID); err != nil {
		t.Fatalf("ExecuteRun() error = %v", err)
	}
	if fake.called != 1 {
		t.Fatalf("executor called = %d, want 1", fake.called)
	}
	if fake.lastRunID != runID {
		t.Fatalf("executor run id = %q, want %q", fake.lastRunID, runID)
	}
	run := getJSON(t, server.Client(), server.URL+"/internal/v1/build-runs/"+runID)
	if run["status"] != "completed" {
		t.Fatalf("run status = %v, want completed", run["status"])
	}
	if run["last_stage"] != "thin-prepare" {
		t.Fatalf("last_stage = %v, want thin-prepare", run["last_stage"])
	}
	if run["failure_summary"] != nil && run["failure_summary"] != "" {
		t.Fatalf("failure_summary = %v, want empty", run["failure_summary"])
	}
	if len(fake.lastEnv) == 0 {
		t.Fatalf("executor command env should be set")
	}
	if fake.lastTaskCount != 1 {
		t.Fatalf("executor task count = %d, want 1", fake.lastTaskCount)
	}
	if fake.lastCheckCount != 1 {
		t.Fatalf("executor check count = %d, want 1", fake.lastCheckCount)
	}
	if fake.lastGoalSummary != "build android app" {
		t.Fatalf("executor goal summary = %q, want build android app", fake.lastGoalSummary)
	}
	if len(fake.lastKnowledgePack) != 4 || fake.lastKnowledgePack[0].SkillID != "prd-to-task-bundle" {
		t.Fatalf("executor knowledge pack = %+v, want Flutter skill pack", fake.lastKnowledgePack)
	}
	if fake.lastDir != fake.workspace {
		t.Fatalf("executor command dir = %q, want %q", fake.lastDir, fake.workspace)
	}
}

func TestRunnerExecuteRunCapturesWorkspacePatchFromEditStep(t *testing.T) {
	t.Setenv("APPFACTORY_BUILDER_RUNTIME", "")
	t.Setenv("APPFACTORY_BUILDER_DOCKER_NETWORK", "")
	t.Setenv("HTTP_PROXY", "")
	t.Setenv("HTTPS_PROXY", "")
	t.Setenv("ALL_PROXY", "")
	t.Setenv("NO_PROXY", "")

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

	postJSON(t, server.Client(), server.URL+"/internal/v1/builders:register", map[string]any{
		"builder_id":        "builder-a",
		"display_name":      "Builder A",
		"capability_tags":   []string{"flutter"},
		"max_parallel_runs": 1,
		"worker_profile":    map[string]any{"image": "builder:latest"},
	})
	allocate := postJSON(t, server.Client(), server.URL+"/internal/v1/workers:allocate", map[string]any{"job_id": "job-1"})
	leaseID, _ := allocate["lease_id"].(string)
	create := postJSON(t, server.Client(), server.URL+"/internal/v1/build-runs", map[string]any{
		"worker_id":     "builder-a",
		"lease_id":      leaseID,
		"builder_input": sampleBuildRunInput(),
	})
	runID, _ := create["run_id"].(string)

	runner := adapter.NewRunner(server.URL)
	runner.Executor = &patchWritingExecutor{}
	if err := runner.ExecuteRun(context.Background(), runID); err != nil {
		t.Fatalf("ExecuteRun() error = %v", err)
	}

	run := getJSON(t, server.Client(), server.URL+"/internal/v1/build-runs/"+runID)
	builderOutput := readBuilderOutput(t, run)
	modifiedFiles, ok := builderOutput["modified_files"].([]any)
	if !ok || len(modifiedFiles) != 1 {
		t.Fatalf("modified_files = %T %#v, want one file change", builderOutput["modified_files"], builderOutput["modified_files"])
	}
	firstFile, ok := modifiedFiles[0].(map[string]any)
	if !ok || firstFile["path"] != "lib/generated.dart" || firstFile["change_type"] != "added" {
		t.Fatalf("modified_file = %#v, want lib/generated.dart added", modifiedFiles[0])
	}
	roundOutputs, ok := builderOutput["round_outputs"].([]any)
	if !ok || len(roundOutputs) != 1 {
		t.Fatalf("round_outputs = %T %#v, want one round output", builderOutput["round_outputs"], builderOutput["round_outputs"])
	}
	firstRound, ok := roundOutputs[0].(map[string]any)
	if !ok {
		t.Fatalf("round_output[0] = %T, want object", roundOutputs[0])
	}
	workspacePatch, ok := firstRound["workspace_patch"].(map[string]any)
	if !ok {
		t.Fatalf("workspace_patch = %T, want object", firstRound["workspace_patch"])
	}
	if workspacePatch["status"] != "applied" {
		t.Fatalf("workspace_patch.status = %v, want applied", workspacePatch["status"])
	}
	operations, ok := workspacePatch["operations"].([]any)
	if !ok || len(operations) != 1 {
		t.Fatalf("workspace_patch.operations = %v, want one operation", workspacePatch["operations"])
	}
	generatedPath := filepath.Join(filepath.FromSlash(run["workspace_path"].(string)), "lib", "generated.dart")
	data, err := os.ReadFile(generatedPath)
	if err != nil {
		t.Fatalf("ReadFile(generated.dart) error = %v", err)
	}
	if string(data) != "const generated = true;\n" {
		t.Fatalf("generated.dart = %q, want applied generated content", string(data))
	}
}

func TestRunnerExecuteRunClassifiesWorkspacePatchApplyFailure(t *testing.T) {
	t.Setenv("APPFACTORY_BUILDER_RUNTIME", "")
	t.Setenv("APPFACTORY_BUILDER_DOCKER_NETWORK", "")
	t.Setenv("HTTP_PROXY", "")
	t.Setenv("HTTPS_PROXY", "")
	t.Setenv("ALL_PROXY", "")
	t.Setenv("NO_PROXY", "")

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

	postJSON(t, server.Client(), server.URL+"/internal/v1/builders:register", map[string]any{
		"builder_id":        "builder-a",
		"display_name":      "Builder A",
		"capability_tags":   []string{"flutter"},
		"max_parallel_runs": 1,
		"worker_profile":    map[string]any{"image": "builder:latest"},
	})
	allocate := postJSON(t, server.Client(), server.URL+"/internal/v1/workers:allocate", map[string]any{"job_id": "job-1"})
	leaseID, _ := allocate["lease_id"].(string)
	create := postJSON(t, server.Client(), server.URL+"/internal/v1/build-runs", map[string]any{
		"worker_id": "builder-a",
		"lease_id":  leaseID,
		"builder_input": map[string]any{
			"schema_version": "0.1.0",
			"job_id":         "job-1",
			"prd_id":         "prd-1",
			"template_id":    "template-1",
			"workspace_path": "/workspace/job-1",
			"artifact_dir":   "/artifacts/job-1",
			"goal_summary":   "build android app",
			"task_bundle": []map[string]any{{
				"task_id":             "task-1",
				"title":               "Touch android config",
				"category":            "runtime",
				"objective":           "attempt protected path write",
				"target_paths":        []string{"android/app/build.gradle.kts"},
				"completion_criteria": []string{"protected path write blocked"},
			}},
			"acceptance_checks": []map[string]any{{
				"check_id": "check-1",
				"label":    "build",
				"stage":    "baseline",
				"required": true,
				"commands": []string{"echo build-start"},
			}},
			"allowed_paths":   []string{"android/**"},
			"protected_paths": []string{"android/app/build.gradle.kts"},
			"command_profile": map[string]any{
				"profile_name":     "manual-p0",
				"allowed_stages":   []string{"baseline", "cheap"},
				"allowed_commands": []string{"echo", "sh"},
				"network_policy":   "disabled",
			},
			"context_files": map[string]any{
				"prd_markdown_path":        "PRD.md",
				"prd_json_path":            "PRD.json",
				"template_fit_report_path": "template-fit-report.md",
				"implementation_plan_path": "implementation-plan.md",
			},
			"iteration_budget": 3,
			"token_budget":     2000,
		},
	})
	runID, _ := create["run_id"].(string)

	runner := adapter.NewRunner(server.URL)
	runner.Executor = patchProtectedPathExecutor{}
	if err := runner.ExecuteRun(context.Background(), runID); err != nil {
		t.Fatalf("ExecuteRun() error = %v", err)
	}

	run := getJSON(t, server.Client(), server.URL+"/internal/v1/build-runs/"+runID)
	if run["status"] != "failed" {
		t.Fatalf("run status = %v, want failed", run["status"])
	}
	failureSummary, _ := run["failure_summary"].(string)
	if !strings.Contains(failureSummary, "workspace patch apply failed") || !strings.Contains(failureSummary, "android/app/build.gradle.kts") {
		t.Fatalf("failure_summary = %q, want workspace patch apply failure mentioning android/app/build.gradle.kts", failureSummary)
	}
	failureSignatures, ok := run["failure_signatures"].([]any)
	if !ok || len(failureSignatures) != 1 || failureSignatures[0] != "workspace_patch_apply_failed" {
		t.Fatalf("failure_signatures = %#v, want [workspace_patch_apply_failed]", run["failure_signatures"])
	}
	roundState, ok := run["round_state"].(map[string]any)
	if !ok {
		t.Fatalf("round_state = %T, want object", run["round_state"])
	}
	if roundState["next_action"] != "stop" {
		t.Fatalf("round_state.next_action = %v, want stop", roundState["next_action"])
	}
	if value, exists := roundState["preserve_workspace"]; exists && value != false {
		t.Fatalf("round_state.preserve_workspace = %v, want false or omitted", value)
	}
	if value, exists := roundState["resume_allowed"]; exists && value != false {
		t.Fatalf("round_state.resume_allowed = %v, want false or omitted", value)
	}
	repairContext, ok := run["repair_context"].(map[string]any)
	if !ok {
		t.Fatalf("repair_context = %T, want object", run["repair_context"])
	}
	if repairContext["recommended_action"] != "inspect executor-generated changes and allowed/protected path constraints before rerun" {
		t.Fatalf("repair_context.recommended_action = %v, want executor patch guidance", repairContext["recommended_action"])
	}
	if value, exists := repairContext["preserve_workspace"]; exists && value != false {
		t.Fatalf("repair_context.preserve_workspace = %v, want false or omitted", value)
	}
	if repairContext["requires_human_review"] != true {
		t.Fatalf("repair_context.requires_human_review = %v, want true", repairContext["requires_human_review"])
	}
	workspacePath, _ := run["workspace_path"].(string)
	if _, err := os.Stat(filepath.Join(filepath.FromSlash(workspacePath), "android", "app", "build.gradle.kts")); !os.IsNotExist(err) {
		t.Fatalf("android/app/build.gradle.kts should not remain in workspace after failed patch apply, stat err=%v", err)
	}
	logPath, _ := run["log_path"].(string)
	resolvedLogPath := filepath.Join(filepath.Clean(filepath.Join(workspacePath, "..", "..", "..")), filepath.FromSlash(logPath))
	logData, err := os.ReadFile(resolvedLogPath)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", resolvedLogPath, err)
	}
	if !strings.Contains(string(logData), "protected-path") {
		t.Fatalf("resolved log missing protected-path marker: %q", string(logData))
	}
}

func TestRunnerExecuteRunCreatesFlutterLandingFilesOnDefaultExecutor(t *testing.T) {
	t.Setenv("APPFACTORY_BUILDER_RUNTIME", "")
	t.Setenv("APPFACTORY_BUILDER_DOCKER_NETWORK", "")
	t.Setenv("HTTP_PROXY", "")
	t.Setenv("HTTPS_PROXY", "")
	t.Setenv("ALL_PROXY", "")
	t.Setenv("NO_PROXY", "")

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

	postJSON(t, server.Client(), server.URL+"/internal/v1/builders:register", map[string]any{
		"builder_id":        "builder-a",
		"display_name":      "Builder A",
		"capability_tags":   []string{"flutter"},
		"max_parallel_runs": 1,
		"worker_profile":    map[string]any{"image": "builder:latest"},
	})
	allocate := postJSON(t, server.Client(), server.URL+"/internal/v1/workers:allocate", map[string]any{"job_id": "job-1"})
	leaseID, _ := allocate["lease_id"].(string)
	create := postJSON(t, server.Client(), server.URL+"/internal/v1/build-runs", map[string]any{
		"worker_id":     "builder-a",
		"lease_id":      leaseID,
		"builder_input": sampleFlutterLandingBuildRunInput(),
	})
	runID, _ := create["run_id"].(string)

	runner := adapter.NewRunner(server.URL)
	if err := runner.ExecuteRun(context.Background(), runID); err != nil {
		t.Fatalf("ExecuteRun() error = %v", err)
	}

	run := getJSON(t, server.Client(), server.URL+"/internal/v1/build-runs/"+runID)
	if run["status"] != "completed" {
		t.Fatalf("run status = %v, want completed (failure_summary=%v)", run["status"], run["failure_summary"])
	}
	builderOutput := readBuilderOutput(t, run)
	modifiedFiles, ok := builderOutput["modified_files"].([]any)
	if !ok || len(modifiedFiles) < 4 {
		t.Fatalf("modified_files = %T %#v, want multiple flutter landing files", builderOutput["modified_files"], builderOutput["modified_files"])
	}
	modifiedPaths := make(map[string]bool, len(modifiedFiles))
	for _, item := range modifiedFiles {
		change, ok := item.(map[string]any)
		if !ok {
			t.Fatalf("modified_file = %T, want object", item)
		}
		path, _ := change["path"].(string)
		modifiedPaths[path] = true
	}
	for _, wanted := range []string{"lib/main.dart", "lib/views/entry_form_page.dart", "lib/repositories/entry_repository.dart"} {
		if !modifiedPaths[wanted] {
			t.Fatalf("modified_paths missing %s: %#v", wanted, modifiedPaths)
		}
	}
	roundOutputs, ok := builderOutput["round_outputs"].([]any)
	if !ok || len(roundOutputs) != 1 {
		t.Fatalf("round_outputs = %T %#v, want one round output", builderOutput["round_outputs"], builderOutput["round_outputs"])
	}
	firstRound, ok := roundOutputs[0].(map[string]any)
	if !ok {
		t.Fatalf("round_output[0] = %T, want object", roundOutputs[0])
	}
	workspacePatch, ok := firstRound["workspace_patch"].(map[string]any)
	if !ok || workspacePatch["status"] != "applied" {
		t.Fatalf("workspace_patch = %#v, want applied", firstRound["workspace_patch"])
	}
	workspacePath, _ := run["workspace_path"].(string)
	mainDart, err := os.ReadFile(filepath.Join(filepath.FromSlash(workspacePath), "lib", "main.dart"))
	if err != nil {
		t.Fatalf("ReadFile(lib/main.dart) error = %v", err)
	}
	if !strings.Contains(string(mainDart), "BookkeepingApp") {
		t.Fatalf("lib/main.dart = %q, want bookkeeping app shell", string(mainDart))
	}
	for _, forbidden := range []string{"Flutter Demo Home Page", "You have pushed the button this many times", "_counter", "_incrementCounter", "MyHomePage"} {
		if strings.Contains(string(mainDart), forbidden) {
			t.Fatalf("lib/main.dart should not contain demo marker %q\n%s", forbidden, string(mainDart))
		}
	}
	entryForm, err := os.ReadFile(filepath.Join(filepath.FromSlash(workspacePath), "lib", "views", "entry_form_page.dart"))
	if err != nil {
		t.Fatalf("ReadFile(lib/views/entry_form_page.dart) error = %v", err)
	}
	if !strings.Contains(string(entryForm), "TextFormField") || !strings.Contains(string(entryForm), "showDatePicker") {
		t.Fatalf("entry_form_page.dart = %q, want real form wiring", string(entryForm))
	}
	repository, err := os.ReadFile(filepath.Join(filepath.FromSlash(workspacePath), "lib", "repositories", "entry_repository.dart"))
	if err != nil {
		t.Fatalf("ReadFile(lib/repositories/entry_repository.dart) error = %v", err)
	}
	if !strings.Contains(string(repository), "SharedPreferences") {
		t.Fatalf("entry_repository.dart = %q, want persistence wiring", string(repository))
	}
	pubspec, err := os.ReadFile(filepath.Join(filepath.FromSlash(workspacePath), "pubspec.yaml"))
	if err != nil {
		t.Fatalf("ReadFile(pubspec.yaml) error = %v", err)
	}
	if !strings.Contains(string(pubspec), "shared_preferences: ^2.2.3") {
		t.Fatalf("pubspec.yaml = %q, want shared_preferences dependency", string(pubspec))
	}
	widgetTest, err := os.ReadFile(filepath.Join(filepath.FromSlash(workspacePath), "test", "widget_test.dart"))
	if err != nil {
		t.Fatalf("ReadFile(test/widget_test.dart) error = %v", err)
	}
	if !strings.Contains(string(widgetTest), "bookkeeping shell renders") {
		t.Fatalf("widget_test.dart = %q, want bookkeeping smoke test", string(widgetTest))
	}
	for _, forbidden := range []string{"Counter increments smoke test", "MyHomePage", "_incrementCounter"} {
		if strings.Contains(string(widgetTest), forbidden) {
			t.Fatalf("widget_test.dart should not contain demo marker %q\n%s", forbidden, string(widgetTest))
		}
	}
	if _, err := os.Stat(filepath.Join(filepath.FromSlash(workspacePath), "lib", "picoclaw_executor_probe.dart")); !os.IsNotExist(err) {
		t.Fatalf("probe file should not be created on flutter landing path, stat err=%v", err)
	}
}

func TestRunnerExecuteRunSkipsNoopClassificationAfterAppliedChanges(t *testing.T) {
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

	postJSON(t, server.Client(), server.URL+"/internal/v1/builders:register", map[string]any{
		"builder_id":        "builder-a",
		"display_name":      "Builder A",
		"capability_tags":   []string{"flutter"},
		"max_parallel_runs": 1,
		"worker_profile":    map[string]any{"image": "builder:latest"},
	})
	allocate := postJSON(t, server.Client(), server.URL+"/internal/v1/workers:allocate", map[string]any{"job_id": "job-1"})
	leaseID, _ := allocate["lease_id"].(string)
	create := postJSON(t, server.Client(), server.URL+"/internal/v1/build-runs", map[string]any{
		"worker_id":     "builder-a",
		"lease_id":      leaseID,
		"builder_input": sampleBuildRunInput(),
	})
	runID, _ := create["run_id"].(string)

	runner := adapter.NewRunner(server.URL)
	runner.Executor = failingLogExecutor{script: "echo 'This plan has no pending changes to build'; echo '✅ Applied changes, 11 files updated'; exit 1"}
	if err := runner.ExecuteRun(context.Background(), runID); err != nil {
		t.Fatalf("ExecuteRun() error = %v", err)
	}

	run := getJSON(t, server.Client(), server.URL+"/internal/v1/build-runs/"+runID)
	if run["status"] != "failed" {
		t.Fatalf("run status = %v, want failed", run["status"])
	}
	if run["failure_summary"] != "exit status 1" {
		t.Fatalf("failure_summary = %v, want exit status 1", run["failure_summary"])
	}
	failureSignatures, ok := run["failure_signatures"].([]any)
	if !ok || len(failureSignatures) != 1 || failureSignatures[0] != "runner_exit_nonzero" {
		t.Fatalf("failure_signatures = %#v, want [runner_exit_nonzero]", run["failure_signatures"])
	}
	repairContext, ok := run["repair_context"].(map[string]any)
	if !ok {
		t.Fatalf("repair_context = %T, want object", run["repair_context"])
	}
	if repairContext["requires_human_review"] != true {
		t.Fatalf("repair_context.requires_human_review = %v, want true", repairContext["requires_human_review"])
	}
}

func TestRunnerExecuteRunTreatsLegacyBuilderLogAsGenericFailureOnDefaultRuntime(t *testing.T) {
	t.Setenv("APPFACTORY_BUILDER_RUNTIME", "")
	t.Setenv("APPFACTORY_BUILDER_DOCKER_NETWORK", "")
	t.Setenv("HTTP_PROXY", "")
	t.Setenv("HTTPS_PROXY", "")
	t.Setenv("ALL_PROXY", "")
	t.Setenv("NO_PROXY", "")

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

	postJSON(t, server.Client(), server.URL+"/internal/v1/builders:register", map[string]any{
		"builder_id":        "builder-a",
		"display_name":      "Builder A",
		"capability_tags":   []string{"flutter"},
		"max_parallel_runs": 1,
		"worker_profile":    map[string]any{"image": "builder:latest"},
	})
	allocate := postJSON(t, server.Client(), server.URL+"/internal/v1/workers:allocate", map[string]any{"job_id": "job-1"})
	leaseID, _ := allocate["lease_id"].(string)
	create := postJSON(t, server.Client(), server.URL+"/internal/v1/build-runs", map[string]any{
		"worker_id":     "builder-a",
		"lease_id":      leaseID,
		"builder_input": sampleBuildRunInput(),
	})
	runID, _ := create["run_id"].(string)

	runner := adapter.NewRunner(server.URL)
	runner.Executor = failingLogExecutor{script: "echo 'This plan has no pending changes to build'; exit 1"}
	if err := runner.ExecuteRun(context.Background(), runID); err != nil {
		t.Fatalf("ExecuteRun() error = %v", err)
	}

	run := getJSON(t, server.Client(), server.URL+"/internal/v1/build-runs/"+runID)
	if run["status"] != "failed" {
		t.Fatalf("run status = %v, want failed", run["status"])
	}
	if run["failure_summary"] != "exit status 1" {
		t.Fatalf("failure_summary = %v, want exit status 1", run["failure_summary"])
	}
	failureSignatures, ok := run["failure_signatures"].([]any)
	if !ok || len(failureSignatures) != 1 || failureSignatures[0] != "runner_exit_nonzero" {
		t.Fatalf("failure_signatures = %#v, want [runner_exit_nonzero]", run["failure_signatures"])
	}
	roundState, ok := run["round_state"].(map[string]any)
	if !ok {
		t.Fatalf("round_state = %T, want object", run["round_state"])
	}
	if roundState["next_action"] != "stop" {
		t.Fatalf("round_state.next_action = %v, want stop", roundState["next_action"])
	}
	if value, exists := roundState["resume_allowed"]; exists && value != false {
		t.Fatalf("round_state.resume_allowed = %v, want false or omitted", value)
	}
	repairContext, ok := run["repair_context"].(map[string]any)
	if !ok {
		t.Fatalf("repair_context = %T, want object", run["repair_context"])
	}
	if repairContext["requires_human_review"] != true {
		t.Fatalf("repair_context.requires_human_review = %v, want true", repairContext["requires_human_review"])
	}
	if repairContext["recommended_action"] != "inspect runner log and rerun after fixing prepare stage or environment" {
		t.Fatalf("repair_context.recommended_action = %v, want generic executor guidance", repairContext["recommended_action"])
	}
}

func TestRunnerExecuteRunClassifiesProfileStructuralCheckFailure(t *testing.T) {
	t.Setenv("APPFACTORY_BUILDER_RUNTIME", "")
	t.Setenv("APPFACTORY_BUILDER_DOCKER_NETWORK", "")
	t.Setenv("HTTP_PROXY", "")
	t.Setenv("HTTPS_PROXY", "")
	t.Setenv("ALL_PROXY", "")
	t.Setenv("NO_PROXY", "")

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

	postJSON(t, server.Client(), server.URL+"/internal/v1/builders:register", map[string]any{
		"builder_id":        "builder-a",
		"display_name":      "Builder A",
		"capability_tags":   []string{"flutter"},
		"max_parallel_runs": 1,
		"worker_profile":    map[string]any{"image": "builder:latest"},
	})
	allocate := postJSON(t, server.Client(), server.URL+"/internal/v1/workers:allocate", map[string]any{"job_id": "job-1"})
	leaseID, _ := allocate["lease_id"].(string)
	create := postJSON(t, server.Client(), server.URL+"/internal/v1/build-runs", map[string]any{
		"worker_id":     "builder-a",
		"lease_id":      leaseID,
		"builder_input": sampleBuildRunInputWithAcceptanceCheck("check-counter-demo-removed", "counter demo removed", "cheap", []string{"grep demo"}),
	})
	runID, _ := create["run_id"].(string)

	runner := adapter.NewRunner(server.URL)
	runner.Executor = failingValidationExecutor{
		checkID:  "check-counter-demo-removed",
		label:    "counter demo removed",
		stage:    appruns.StageCheap,
		commands: []string{"grep demo"},
		script:   "echo flutter demo markers still exist >&2; exit 1",
	}
	if err := runner.ExecuteRun(context.Background(), runID); err != nil {
		t.Fatalf("ExecuteRun() error = %v", err)
	}

	run := getJSON(t, server.Client(), server.URL+"/internal/v1/build-runs/"+runID)
	if run["status"] != "failed" {
		t.Fatalf("run status = %v, want failed", run["status"])
	}
	if run["failure_summary"] != "profile structural check check-counter-demo-removed failed: exit status 1" {
		t.Fatalf("failure_summary = %v, want profile structural check summary", run["failure_summary"])
	}
	failureSignatures, ok := run["failure_signatures"].([]any)
	if !ok || len(failureSignatures) != 1 || failureSignatures[0] != "profile_check_failed:check-counter-demo-removed" {
		t.Fatalf("failure_signatures = %#v, want [profile_check_failed:check-counter-demo-removed]", run["failure_signatures"])
	}
	roundState, ok := run["round_state"].(map[string]any)
	if !ok {
		t.Fatalf("round_state = %T, want object", run["round_state"])
	}
	if roundState["next_action"] != "resume" {
		t.Fatalf("round_state.next_action = %v, want resume", roundState["next_action"])
	}
	if roundState["resume_allowed"] != true {
		t.Fatalf("round_state.resume_allowed = %v, want true", roundState["resume_allowed"])
	}
	repairContext, ok := run["repair_context"].(map[string]any)
	if !ok {
		t.Fatalf("repair_context = %T, want object", run["repair_context"])
	}
	if repairContext["recommended_action"] != "complete the missing Flutter profile implementation before rerun" {
		t.Fatalf("repair_context.recommended_action = %v, want profile guidance", repairContext["recommended_action"])
	}
	if value, exists := repairContext["requires_human_review"]; exists && value != false {
		t.Fatalf("repair_context.requires_human_review = %v, want false or omitted", value)
	}
}

func TestRunnerExecuteRunClassifiesEnvironmentClosureCheckFailure(t *testing.T) {
	t.Setenv("APPFACTORY_BUILDER_RUNTIME", "")
	t.Setenv("APPFACTORY_BUILDER_DOCKER_NETWORK", "")
	t.Setenv("HTTP_PROXY", "")
	t.Setenv("HTTPS_PROXY", "")
	t.Setenv("ALL_PROXY", "")
	t.Setenv("NO_PROXY", "")

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

	postJSON(t, server.Client(), server.URL+"/internal/v1/builders:register", map[string]any{
		"builder_id":        "builder-a",
		"display_name":      "Builder A",
		"capability_tags":   []string{"flutter"},
		"max_parallel_runs": 1,
		"worker_profile":    map[string]any{"image": "builder:latest"},
	})
	allocate := postJSON(t, server.Client(), server.URL+"/internal/v1/workers:allocate", map[string]any{"job_id": "job-1"})
	leaseID, _ := allocate["lease_id"].(string)
	create := postJSON(t, server.Client(), server.URL+"/internal/v1/build-runs", map[string]any{
		"worker_id":     "builder-a",
		"lease_id":      leaseID,
		"builder_input": sampleBuildRunInputWithAcceptanceCheck("check-flutter-analyze", "flutter analyze", "cheap", []string{"flutter analyze"}),
	})
	runID, _ := create["run_id"].(string)

	runner := adapter.NewRunner(server.URL)
	runner.Executor = failingValidationExecutor{
		checkID:  "check-flutter-analyze",
		label:    "flutter analyze",
		stage:    appruns.StageCheap,
		commands: []string{"flutter analyze"},
		script:   "echo flutter analyze failed because sdk is unavailable >&2; exit 1",
	}
	if err := runner.ExecuteRun(context.Background(), runID); err != nil {
		t.Fatalf("ExecuteRun() error = %v", err)
	}

	run := getJSON(t, server.Client(), server.URL+"/internal/v1/build-runs/"+runID)
	if run["status"] != "failed" {
		t.Fatalf("run status = %v, want failed", run["status"])
	}
	if run["failure_summary"] != "environment closure check check-flutter-analyze failed: exit status 1" {
		t.Fatalf("failure_summary = %v, want environment closure check summary", run["failure_summary"])
	}
	failureSignatures, ok := run["failure_signatures"].([]any)
	if !ok || len(failureSignatures) != 1 || failureSignatures[0] != "environment_check_failed:check-flutter-analyze" {
		t.Fatalf("failure_signatures = %#v, want [environment_check_failed:check-flutter-analyze]", run["failure_signatures"])
	}
	roundState, ok := run["round_state"].(map[string]any)
	if !ok {
		t.Fatalf("round_state = %T, want object", run["round_state"])
	}
	if roundState["next_action"] != "stop" {
		t.Fatalf("round_state.next_action = %v, want stop", roundState["next_action"])
	}
	if value, exists := roundState["resume_allowed"]; exists && value != false {
		t.Fatalf("round_state.resume_allowed = %v, want false or omitted", value)
	}
	repairContext, ok := run["repair_context"].(map[string]any)
	if !ok {
		t.Fatalf("repair_context = %T, want object", run["repair_context"])
	}
	if repairContext["recommended_action"] != "fix the builder environment or command dependencies before rerun" {
		t.Fatalf("repair_context.recommended_action = %v, want environment guidance", repairContext["recommended_action"])
	}
	if repairContext["requires_human_review"] != true {
		t.Fatalf("repair_context.requires_human_review = %v, want true", repairContext["requires_human_review"])
	}
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

func readBuilderOutput(t *testing.T, run map[string]any) map[string]any {
	t.Helper()
	outputPath, _ := run["output_path"].(string)
	if strings.TrimSpace(outputPath) == "" {
		t.Fatalf("output_path is empty in run record: %#v", run)
	}
	workspacePath, _ := run["workspace_path"].(string)
	if strings.TrimSpace(workspacePath) == "" {
		t.Fatalf("workspace_path is empty in run record: %#v", run)
	}
	appFactoryRoot := filepath.Clean(filepath.Join(filepath.FromSlash(workspacePath), "..", "..", ".."))
	data, err := os.ReadFile(filepath.Join(appFactoryRoot, filepath.FromSlash(outputPath)))
	if err != nil {
		t.Fatalf("ReadFile(builder-output) error = %v", err)
	}
	var output map[string]any
	if err := json.Unmarshal(data, &output); err != nil {
		t.Fatalf("Unmarshal(builder-output) error = %v", err)
	}
	return output
}

func sampleBuildRunInput() map[string]any {
	return map[string]any{
		"schema_version": "0.1.0",
		"job_id":         "job-1",
		"prd_id":         "prd-1",
		"template_id":    "template-1",
		"workspace_path": "/workspace/job-1",
		"artifact_dir":   "/artifacts/job-1",
		"goal_summary":   "build android app",
		"task_bundle": []map[string]any{{
			"task_id":             "task-1",
			"title":               "Implement UI",
			"category":            "ui",
			"objective":           "Create home screen",
			"target_paths":        []string{"lib/main.dart"},
			"completion_criteria": []string{"screen renders"},
		}},
		"acceptance_checks": []map[string]any{{
			"check_id": "check-1",
			"label":    "build",
			"stage":    "baseline",
			"required": true,
			"commands": []string{"echo build-start"},
		}},
		"allowed_paths":   []string{"lib/**"},
		"protected_paths": []string{"android/app/build.gradle.kts"},
		"knowledge_pack": []map[string]any{
			{"skill_id": "prd-to-task-bundle", "role": "把 PRD 压成 Flutter profile 可执行任务包", "usage_stage": "planning", "scope": "flutter-android-p0"},
			{"skill_id": "flutter-mvc-template", "role": "固定 Flutter MVC 目录、依赖与页面落点", "usage_stage": "layout", "scope": "flutter-android-p0"},
			{"skill_id": "builder-direct-edit", "role": "约束 Builder 在 Flutter 工作区中直接改码", "usage_stage": "edit", "scope": "flutter-android-p0"},
			{"skill_id": "flutter-build-closure", "role": "在 Flutter 工作区接近完成时做低风险收口", "usage_stage": "closure", "scope": "flutter-android-p0"},
		},
		"command_profile": map[string]any{
			"profile_name":     "manual-p0",
			"allowed_stages":   []string{"baseline", "cheap"},
			"allowed_commands": []string{"echo", "flutter"},
			"network_policy":   "disabled",
		},
		"context_files": map[string]any{
			"prd_markdown_path":        "PRD.md",
			"prd_json_path":            "PRD.json",
			"template_fit_report_path": "template-fit-report.md",
			"implementation_plan_path": "implementation-plan.md",
		},
		"iteration_budget": 3,
		"token_budget":     2000,
	}
}

func sampleBuildRunInputWithAcceptanceCheck(checkID, label, stage string, commands []string) map[string]any {
	input := sampleBuildRunInput()
	input["acceptance_checks"] = []map[string]any{{
		"check_id": checkID,
		"label":    label,
		"stage":    stage,
		"required": true,
		"commands": commands,
	}}
	commandProfile, _ := input["command_profile"].(map[string]any)
	allowedCommands := []string{"echo", "flutter"}
	for _, command := range commands {
		trimmedCommand := strings.TrimSpace(command)
		if trimmedCommand == "" {
			continue
		}
		allowedCommands = append(allowedCommands, strings.Fields(trimmedCommand)[0])
	}
	commandProfile["allowed_commands"] = uniqueStrings(allowedCommands)
	input["command_profile"] = commandProfile
	return input
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		result = append(result, trimmed)
	}
	return result
}

func sampleFlutterLandingBuildRunInput() map[string]any {
	return map[string]any{
		"schema_version": "0.1.0",
		"job_id":         "job-1",
		"prd_id":         "prd-1",
		"template_id":    "flutter-finance-lite",
		"workspace_path": "/workspace/job-1",
		"artifact_dir":   "/artifacts/job-1",
		"goal_summary":   "deliver bookkeeping shell",
		"task_bundle": []map[string]any{
			{
				"task_id":             "screen-main",
				"title":               "Replace main entry",
				"category":            "screen",
				"objective":           "replace demo with bookkeeping navigation shell",
				"target_paths":        []string{"lib/main.dart", "lib/views/home_page.dart"},
				"completion_criteria": []string{"main shell exists"},
			},
			{
				"task_id":             "flow-entry",
				"title":               "Wire entry flow",
				"category":            "flow",
				"objective":           "add entry form and local persistence",
				"target_paths":        []string{"lib/views/entry_form_page.dart", "lib/repositories/entry_repository.dart"},
				"completion_criteria": []string{"entry form and repository exist"},
			},
		},
		"acceptance_checks": []map[string]any{
			{
				"check_id": "check-entry-form-wiring",
				"label":    "entry form wiring",
				"stage":    "cheap",
				"required": true,
				"commands": []string{"grep -E 'TextEditingController|TextFormField|DropdownButtonFormField|showDatePicker' lib/views/entry_form_page.dart lib/controllers/entry_form_controller.dart"},
			},
			{
				"check_id": "check-local-persistence-wiring",
				"label":    "local persistence wiring",
				"stage":    "cheap",
				"required": true,
				"commands": []string{"grep -E 'shared_preferences|SharedPreferences|sqflite|hive|isar' pubspec.yaml lib/repositories/entry_repository.dart lib/main.dart"},
			},
		},
		"allowed_paths":   []string{"lib/**", "test/**", "pubspec.yaml"},
		"protected_paths": []string{"android/**", "ios/**"},
		"knowledge_pack": []map[string]any{
			{"skill_id": "prd-to-task-bundle", "role": "把 PRD 压成 Flutter profile 可执行任务包", "usage_stage": "planning", "scope": "flutter-android-p0"},
			{"skill_id": "flutter-mvc-template", "role": "固定 Flutter MVC 目录、依赖与页面落点", "usage_stage": "layout", "scope": "flutter-android-p0"},
			{"skill_id": "builder-direct-edit", "role": "约束 Builder 在 Flutter 工作区中直接改码", "usage_stage": "edit", "scope": "flutter-android-p0"},
			{"skill_id": "flutter-build-closure", "role": "在 Flutter 工作区接近完成时做低风险收口", "usage_stage": "closure", "scope": "flutter-android-p0"},
		},
		"command_profile": map[string]any{
			"profile_name":     "flutter-builder-p0",
			"allowed_stages":   []string{"cheap"},
			"allowed_commands": []string{"grep"},
			"network_policy":   "disabled",
		},
		"context_files": map[string]any{
			"prd_markdown_path":        "PRD.md",
			"prd_json_path":            "PRD.json",
			"template_fit_report_path": "template-fit-report.md",
			"implementation_plan_path": "implementation-plan.md",
		},
		"iteration_budget": 3,
		"token_budget":     2000,
	}
}

type stubThinExecutor struct {
	called            int
	lastRunID         string
	lastDir           string
	lastEnv           []string
	lastGoalSummary   string
	lastKnowledgePack []appruns.ProfileSkill
	lastTaskCount     int
	lastCheckCount    int
	workspace         string
}

func (executor *stubThinExecutor) Prepare(ctx context.Context, run adapter.RunRecord) (adapter.RoundPlan, error) {
	executor.called++
	executor.lastRunID = run.RunID
	executor.lastGoalSummary = run.GoalSummary
	executor.lastKnowledgePack = append([]appruns.ProfileSkill(nil), run.KnowledgePack...)
	executor.lastTaskCount = len(run.TaskBundle)
	executor.lastCheckCount = len(run.AcceptanceChecks)
	cmd := exec.CommandContext(ctx, "/bin/sh", "-lc", "true")
	cmd.Dir = executor.workspace
	cmd.Env = append(os.Environ(), "PICOCLAW_STUB_EXECUTOR=1")
	executor.lastDir = cmd.Dir
	executor.lastEnv = append([]string(nil), cmd.Env...)
	return adapter.RoundPlan{
		Summary:    "stub executor prepared command",
		RoundInput: adapter.BuildRoundInputForTest(run),
		EditStep: adapter.ExecutionStep{
			StepID:  "thin-prepare",
			Stage:   appruns.StageThinPrepare,
			Summary: "stub executor prepared command",
			Command: cmd,
		},
	}, nil
}

type patchWritingExecutor struct{}

func (patchWritingExecutor) Prepare(ctx context.Context, run adapter.RunRecord) (adapter.RoundPlan, error) {
	cmd := exec.CommandContext(ctx, "/bin/sh", "-lc", "mkdir -p lib && printf 'const generated = true;\\n' > lib/generated.dart")
	cmd.Dir = run.WorkspacePath
	cmd.Env = append(os.Environ(), "PICOCLAW_STUB_EXECUTOR=patch-write")
	return adapter.RoundPlan{
		Summary:    "patch writing executor",
		RoundInput: adapter.BuildRoundInputForTest(run),
		EditStep: adapter.ExecutionStep{
			StepID:  "thin-prepare",
			Stage:   appruns.StageThinPrepare,
			Summary: "write generated file",
			Command: cmd,
		},
	}, nil
}

type patchProtectedPathExecutor struct{}

func (patchProtectedPathExecutor) Prepare(ctx context.Context, run adapter.RunRecord) (adapter.RoundPlan, error) {
	cmd := exec.CommandContext(ctx, "/bin/sh", "-lc", "mkdir -p android/app && printf 'protected-path\n' > android/app/build.gradle.kts && echo protected-path-attempted")
	cmd.Dir = run.WorkspacePath
	cmd.Env = append(os.Environ(), "PICOCLAW_STUB_EXECUTOR=patch-protected-path")
	return adapter.RoundPlan{
		Summary:    "patch protected path executor",
		RoundInput: adapter.BuildRoundInputForTest(run),
		EditStep: adapter.ExecutionStep{
			StepID:  "thin-prepare",
			Stage:   appruns.StageThinPrepare,
			Summary: "write protected path",
			Command: cmd,
		},
	}, nil
}

type failingLogExecutor struct {
	script string
}

func (executor failingLogExecutor) Prepare(ctx context.Context, run adapter.RunRecord) (adapter.RoundPlan, error) {
	cmd := exec.CommandContext(ctx, "/bin/sh", "-lc", executor.script)
	cmd.Dir = run.WorkspacePath
	cmd.Env = append(os.Environ(), "PICOCLAW_STUB_EXECUTOR=failure-log")
	return adapter.RoundPlan{
		Summary:    "failing log executor",
		RoundInput: adapter.BuildRoundInputForTest(run),
		EditStep: adapter.ExecutionStep{
			StepID:  "thin-prepare",
			Stage:   appruns.StageThinPrepare,
			Summary: "emit classified failure log",
			Command: cmd,
		},
	}, nil
}

type failingValidationExecutor struct {
	checkID  string
	label    string
	stage    appruns.ExecutionStage
	commands []string
	script   string
}

func (executor failingValidationExecutor) Prepare(ctx context.Context, run adapter.RunRecord) (adapter.RoundPlan, error) {
	editCmd := exec.CommandContext(ctx, "/bin/sh", "-lc", "true")
	editCmd.Dir = run.WorkspacePath
	editCmd.Env = append(os.Environ(), "PICOCLAW_STUB_EXECUTOR=validation-edit")
	validationCmd := exec.CommandContext(ctx, "/bin/sh", "-lc", executor.script)
	validationCmd.Dir = run.WorkspacePath
	validationCmd.Env = append(os.Environ(), "PICOCLAW_STUB_EXECUTOR=validation-fail")
	return adapter.RoundPlan{
		Summary:    "failing validation executor",
		RoundInput: adapter.BuildRoundInputForTest(run),
		EditStep: adapter.ExecutionStep{
			StepID:  "thin-prepare",
			Stage:   appruns.StageThinPrepare,
			Summary: "prepare workspace",
			Command: editCmd,
		},
		ValidationSteps: []adapter.ExecutionStep{{
			StepID:  executor.checkID,
			Stage:   executor.stage,
			Summary: executor.label,
			Command: validationCmd,
			Check: &adapter.CheckExecutionPreview{
				CheckID:      executor.checkID,
				Label:        executor.label,
				Stage:        executor.stage,
				Required:     true,
				AllowFailure: false,
				Commands:     append([]string(nil), executor.commands...),
			},
		}},
	}, nil
}
