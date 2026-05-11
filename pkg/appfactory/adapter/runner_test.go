package adapter_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	adapter "github.com/sipeed/oneappfactory/pkg/appfactory/adapter"
	appruns "github.com/sipeed/oneappfactory/pkg/appfactory/runs"
	"github.com/sipeed/oneappfactory/pkg/config"
	api "github.com/sipeed/oneappfactory/web/backend/api"
)

func TestRunnerExecuteRunCompletesAndReleasesBuilder(t *testing.T) {
	t.Setenv("ONEAPPFACTORY_BUILDER_RUNTIME", "")
	t.Setenv("ONEAPPFACTORY_BUILDER_DOCKER_NETWORK", "")
	t.Setenv("HTTP_PROXY", "")
	t.Setenv("HTTPS_PROXY", "")
	t.Setenv("ALL_PROXY", "")
	t.Setenv("NO_PROXY", "")

	configPath := filepath.Join(t.TempDir(), "config.json")
	cfg := config.DefaultConfig()
	cfg.Workspace = filepath.Join(filepath.Dir(configPath), "workspace")
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
	builderInput := sampleBuildRunInput()
	taskBundle := builderInput["task_bundle"].([]map[string]any)
	taskBundle[0]["title"] = "Closure repair"
	taskBundle[0]["task_type"] = string(appruns.BuilderRuntimeTaskTypeClosureRepair)
	create := postJSON(t, server.Client(), server.URL+"/internal/v1/build-runs", map[string]any{
		"worker_id":     "builder-a",
		"lease_id":      leaseID,
		"builder_input": builderInput,
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

func TestExecutionEnvUsesWorkspaceGradleUserHome(t *testing.T) {
	run := adapter.RunRecord{
		RunID:         "run-1",
		JobID:         "job-1",
		ExecutorImage: "oneappfactory/builder:local",
		LaunchCommand: "/bin/sh",
		LaunchArgs:    []string{"-lc", "true"},
		WorkspacePath: filepath.Join(t.TempDir(), "workspace", "appfactory", "jobs", "job-1", "workspace"),
	}
	if err := os.MkdirAll(run.WorkspacePath, 0o755); err != nil {
		t.Fatalf("MkdirAll(workspace) error = %v", err)
	}

	env, err := adapter.LocalExecutionEnvForTest(run)
	if err != nil {
		t.Fatalf("LocalExecutionEnvForTest() error = %v", err)
	}
	builderHome := filepath.Join(filepath.Dir(filepath.Dir(filepath.Dir(run.WorkspacePath))), ".runtime", "home")
	builderTempDir := filepath.Join(filepath.Dir(filepath.Dir(filepath.Dir(run.WorkspacePath))), ".runtime", "tmp")
	gradleUserHome := filepath.Join(filepath.Dir(filepath.Dir(filepath.Dir(run.WorkspacePath))), ".runtime", "gradle-user-home")
	if !containsString(env, "GRADLE_USER_HOME="+gradleUserHome) {
		t.Fatalf("env missing GRADLE_USER_HOME, env=%v", env)
	}
	if !containsString(env, "HOME="+builderHome) {
		t.Fatalf("env missing HOME, env=%v", env)
	}
	if !containsString(env, "TMPDIR="+builderTempDir) {
		t.Fatalf("env missing TMPDIR, env=%v", env)
	}
	if !containsString(env, "XDG_CACHE_HOME="+filepath.Join(builderHome, ".cache")) {
		t.Fatalf("env missing XDG_CACHE_HOME, env=%v", env)
	}
	if _, err := os.Stat(filepath.Join(gradleUserHome, "wrapper", "dists")); err != nil {
		t.Fatalf("expected gradle wrapper dists dir: %v", err)
	}
	if _, err := os.Stat(filepath.Join(builderHome, ".kotlin", "daemon")); err != nil {
		t.Fatalf("expected builder home kotlin daemon dir: %v", err)
	}
	if _, err := os.Stat(builderTempDir); err != nil {
		t.Fatalf("expected builder temp dir: %v", err)
	}

	cmd, err := adapter.DockerPrepareCommandForTest(context.Background(), run)
	if err != nil {
		t.Fatalf("DockerPrepareCommandForTest() error = %v", err)
	}
	args := cmd.Args
	if !containsString(args, "GRADLE_USER_HOME="+gradleUserHome) {
		t.Fatalf("docker args missing GRADLE_USER_HOME, args=%v", args)
	}
	if !containsString(args, "HOME="+builderHome) {
		t.Fatalf("docker args missing HOME, args=%v", args)
	}
	if !containsString(args, "TMPDIR="+builderTempDir) {
		t.Fatalf("docker args missing TMPDIR, args=%v", args)
	}
	if !containsString(args, "XDG_CACHE_HOME="+filepath.Join(builderHome, ".cache")) {
		t.Fatalf("docker args missing XDG_CACHE_HOME, args=%v", args)
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

func TestRunnerExecuteRunUpgradesBuilderRuntimeModelOnParseFailure(t *testing.T) {
	t.Setenv("ONEAPPFACTORY_BUILDER_RUNTIME", "")
	t.Setenv("ONEAPPFACTORY_BUILDER_DOCKER_NETWORK", "")
	t.Setenv("HTTP_PROXY", "")
	t.Setenv("HTTPS_PROXY", "")
	t.Setenv("ALL_PROXY", "")
	t.Setenv("NO_PROXY", "")

	configPath := filepath.Join(t.TempDir(), "config.json")
	cfg := config.DefaultConfig()
	cfg.Workspace = filepath.Join(filepath.Dir(configPath), "workspace")
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
	runner.BuilderRuntimeConfig = config.BuilderRuntimeConfig{
		Enabled:      true,
		DefaultModel: &config.AgentModelConfig{Primary: "qwen2.5-coder-14b-local"},
		UpgradeModel: &config.AgentModelConfig{Primary: "qwen2.5-coder-32b-local"},
		UpgradeThreshold: config.BuilderRuntimeUpgradeThresholdConfig{
			UpgradeOnPatchParseFail: true,
		},
	}
	patchGenerator := &stubBuilderRuntimePatchGenerator{responses: map[string][]adapter.BuilderRuntimePatchResponse{
		"qwen2.5-coder-14b-local": {{Content: `{"patch_id":"round-1-patch","operations":[{"type":"write_file"}]}`}},
		"qwen2.5-coder-32b-local": {{Content: `{"patch_id":"round-1-patch","operations":[{"type":"write_file","path":"lib/main.dart","content":"const title = 'Budget Flow';\n"}]}`}},
	}}
	runner.PatchGenerator = patchGenerator

	if err := runner.ExecuteRun(context.Background(), runID); err != nil {
		t.Fatalf("ExecuteRun() error = %v", err)
	}
	if got := strings.Join(patchGenerator.models, ","); got != "qwen2.5-coder-14b-local,qwen2.5-coder-32b-local" {
		t.Fatalf("models = %q, want 14b then 32b", got)
	}
	run := getJSON(t, server.Client(), server.URL+"/internal/v1/build-runs/"+runID)
	if run["status"] != "completed" {
		t.Fatalf("run status = %v, want completed", run["status"])
	}
	builderOutput := readBuilderOutput(t, run)
	roundOutputs, _ := builderOutput["round_outputs"].([]any)
	firstRound, _ := roundOutputs[0].(map[string]any)
	builderRuntime, ok := firstRound["builder_runtime_execution"].(map[string]any)
	if !ok {
		t.Fatalf("builder_runtime_execution = %T, want object", firstRound["builder_runtime_execution"])
	}
	if builderRuntime["selected_model"] != "qwen2.5-coder-32b-local" {
		t.Fatalf("selected_model = %v, want 32b", builderRuntime["selected_model"])
	}
	if builderRuntime["upgrade_applied"] != true {
		t.Fatalf("upgrade_applied = %v, want true", builderRuntime["upgrade_applied"])
	}
	if builderRuntime["parse_failure_count"] != float64(1) {
		t.Fatalf("parse_failure_count = %v, want 1", builderRuntime["parse_failure_count"])
	}
	workspacePatch, _ := firstRound["workspace_patch"].(map[string]any)
	if workspacePatch["status"] != "applied" {
		t.Fatalf("workspace_patch.status = %v, want applied", workspacePatch["status"])
	}
}

func TestRunnerExecuteRunRepairsBuilderRuntimeSchemaAfterUpgradeParseFailure(t *testing.T) {
	t.Setenv("ONEAPPFACTORY_BUILDER_RUNTIME", "")
	t.Setenv("ONEAPPFACTORY_BUILDER_DOCKER_NETWORK", "")
	t.Setenv("HTTP_PROXY", "")
	t.Setenv("HTTPS_PROXY", "")
	t.Setenv("ALL_PROXY", "")
	t.Setenv("NO_PROXY", "")

	configPath := filepath.Join(t.TempDir(), "config.json")
	cfg := config.DefaultConfig()
	cfg.Workspace = filepath.Join(filepath.Dir(configPath), "workspace")
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
	runner.BuilderRuntimeConfig = config.BuilderRuntimeConfig{
		Enabled:      true,
		UpgradeModel: &config.AgentModelConfig{Primary: "qwen2.5-coder-32b-local"},
	}
	runner.PatchGenerator = &stubBuilderRuntimePatchGenerator{responses: map[string][]adapter.BuilderRuntimePatchResponse{
		"qwen2.5-coder-32b-local": {
			{Content: `{"patch_id":"round-1-patch","operations":[{"replace_block":{"path":"lib/main.dart","old_content":"const title = 'Old Title';\n"}}]}`},
			{Content: `{"patch_id":"round-1-patch-repaired","operations":[{"write_file":{"path":"lib/main.dart","content":"const title = 'Budget Flow';\n"}}]}`},
		},
	}}

	if err := runner.ExecuteRun(context.Background(), runID); err != nil {
		t.Fatalf("ExecuteRun() error = %v", err)
	}
	run := getJSON(t, server.Client(), server.URL+"/internal/v1/build-runs/"+runID)
	if run["status"] != "completed" {
		t.Fatalf("run status = %v, want completed", run["status"])
	}
	builderOutput := readBuilderOutput(t, run)
	roundOutputs, _ := builderOutput["round_outputs"].([]any)
	firstRound, _ := roundOutputs[0].(map[string]any)
	builderRuntime, ok := firstRound["builder_runtime_execution"].(map[string]any)
	if !ok {
		t.Fatalf("builder_runtime_execution = %T, want object", firstRound["builder_runtime_execution"])
	}
	if builderRuntime["selected_model"] != "qwen2.5-coder-32b-local" {
		t.Fatalf("selected_model = %v, want qwen2.5-coder-32b-local", builderRuntime["selected_model"])
	}
	if builderRuntime["upgrade_applied"] != true {
		t.Fatalf("upgrade_applied = %v, want true", builderRuntime["upgrade_applied"])
	}
	if builderRuntime["attempts"] != float64(2) {
		t.Fatalf("attempts = %v, want 2", builderRuntime["attempts"])
	}
	if builderRuntime["parse_failure_count"] != float64(1) {
		t.Fatalf("parse_failure_count = %v, want 1", builderRuntime["parse_failure_count"])
	}
	workspacePatch, _ := firstRound["workspace_patch"].(map[string]any)
	if workspacePatch["status"] != "applied" {
		t.Fatalf("workspace_patch.status = %v, want applied", workspacePatch["status"])
	}
	mainDartPath := filepath.Join(filepath.Dir(configPath), "workspace", "appfactory", "jobs", "job-1", "workspace", "lib", "main.dart")
	content, err := os.ReadFile(mainDartPath)
	if err != nil {
		t.Fatalf("ReadFile(lib/main.dart) error = %v", err)
	}
	if string(content) != "const title = 'Budget Flow';\n" {
		t.Fatalf("lib/main.dart = %q, want repaired content", string(content))
	}
}

func TestRunnerExecuteRunStopsOnTaskCreateLocalValidationFailure(t *testing.T) {
	t.Setenv("ONEAPPFACTORY_BUILDER_RUNTIME", "")
	t.Setenv("ONEAPPFACTORY_BUILDER_DOCKER_NETWORK", "")
	t.Setenv("HTTP_PROXY", "")
	t.Setenv("HTTPS_PROXY", "")
	t.Setenv("ALL_PROXY", "")
	t.Setenv("NO_PROXY", "")

	configPath := filepath.Join(t.TempDir(), "config.json")
	cfg := config.DefaultConfig()
	cfg.Workspace = filepath.Join(filepath.Dir(configPath), "workspace")
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
	builderInput := sampleBuildRunInputWithAcceptanceCheck("check-flutter-analyze", "flutter analyze", "cheap", []string{"echo analyze"})
	builderInput["task_bundle"] = []map[string]any{
		{
			"task_id":             "task-create-home-controller",
			"title":               "Create controller",
			"category":            "summary",
			"objective":           "Create home controller",
			"task_type":           string(appruns.BuilderRuntimeTaskTypeSingleFileEdit),
			"target_paths":        []string{"lib/controllers/home_controller.dart"},
			"completion_criteria": []string{"controller file exists"},
		},
		{
			"task_id":             "task-create-home-page",
			"title":               "Create page",
			"category":            "screen",
			"objective":           "Create home page",
			"task_type":           string(appruns.BuilderRuntimeTaskTypeSingleFileEdit),
			"dependencies":        []string{"task-create-home-controller"},
			"target_paths":        []string{"lib/views/home_page.dart"},
			"completion_criteria": []string{"home page file exists"},
		},
	}
	create := postJSON(t, server.Client(), server.URL+"/internal/v1/build-runs", map[string]any{
		"worker_id":     "builder-a",
		"lease_id":      leaseID,
		"builder_input": builderInput,
	})
	runID, _ := create["run_id"].(string)

	runner := adapter.NewRunner(server.URL)
	runner.BuilderRuntimeConfig = config.BuilderRuntimeConfig{
		Enabled:      true,
		DefaultModel: &config.AgentModelConfig{Primary: "qwen2.5-coder-32b-local"},
	}
	patchGenerator := &stubBuilderRuntimePatchGenerator{responses: map[string][]adapter.BuilderRuntimePatchResponse{
		"qwen2.5-coder-32b-local": {
			{Content: `{"patch_id":"task-create-home-controller","operations":[{"type":"write_file","path":"lib/controllers/home_controller.dart","content":"import '../repositories/record_repository.dart';\nclass HomeController {}\n"}]}`},
			{Content: `{"patch_id":"task-create-home-page","operations":[{"type":"write_file","path":"lib/views/home_page.dart","content":"class HomePage {}\n"}]}`},
		},
	}}
	runner.PatchGenerator = patchGenerator

	if err := runner.ExecuteRun(context.Background(), runID); err != nil {
		t.Fatalf("ExecuteRun() error = %v", err)
	}

	run := getJSON(t, server.Client(), server.URL+"/internal/v1/build-runs/"+runID)
	if run["status"] != "failed" {
		t.Fatalf("run status = %v, want failed", run["status"])
	}
	failureSummary, _ := run["failure_summary"].(string)
	if !strings.Contains(failureSummary, "builder runtime task output validation failed") || !strings.Contains(failureSummary, "unresolved local import/export/part") {
		t.Fatalf("failure_summary = %q, want local validation failure detail", failureSummary)
	}
	failureSignatures, ok := run["failure_signatures"].([]any)
	if !ok || len(failureSignatures) != 1 || failureSignatures[0] != "builder_runtime_task_output_invalid" {
		t.Fatalf("failure_signatures = %#v, want [builder_runtime_task_output_invalid]", run["failure_signatures"])
	}
	if len(patchGenerator.requests) != 1 {
		t.Fatalf("patch requests = %d, want 1 and stop before next task", len(patchGenerator.requests))
	}
	if patchGenerator.requests[0].Route.TaskID != "task-create-home-controller" {
		t.Fatalf("first request task = %q, want task-create-home-controller", patchGenerator.requests[0].Route.TaskID)
	}
	roundState, ok := run["round_state"].(map[string]any)
	if !ok {
		t.Fatalf("round_state = %T, want object", run["round_state"])
	}
	if roundState["current_task_id"] != "task-create-home-controller" {
		t.Fatalf("round_state.current_task_id = %v, want task-create-home-controller", roundState["current_task_id"])
	}
	taskStatuses, ok := roundState["task_statuses"].(map[string]any)
	if !ok {
		t.Fatalf("round_state.task_statuses = %T, want object", roundState["task_statuses"])
	}
	if taskStatuses["task-create-home-controller"] != string(appruns.BuilderRuntimeTaskStatusCreated) {
		t.Fatalf("task-create-home-controller status = %v, want created", taskStatuses["task-create-home-controller"])
	}
	if _, exists := taskStatuses["task-create-home-page"]; exists {
		t.Fatalf("task-create-home-page status = %v, want omitted because execution stopped before next task", taskStatuses["task-create-home-page"])
	}
	workspacePath, _ := run["workspace_path"].(string)
	homePagePath := filepath.Join(filepath.FromSlash(workspacePath), "lib", "views", "home_page.dart")
	if _, err := os.Stat(homePagePath); !os.IsNotExist(err) {
		t.Fatalf("home page path stat error = %v, want next task output absent", err)
	}
}

func TestRunnerExecuteRunUpgradesBuilderRuntimeModelOnSchemaDriftThreshold(t *testing.T) {
	t.Setenv("ONEAPPFACTORY_BUILDER_RUNTIME", "")
	t.Setenv("ONEAPPFACTORY_BUILDER_DOCKER_NETWORK", "")
	t.Setenv("HTTP_PROXY", "")
	t.Setenv("HTTPS_PROXY", "")
	t.Setenv("ALL_PROXY", "")
	t.Setenv("NO_PROXY", "")

	configPath := filepath.Join(t.TempDir(), "config.json")
	cfg := config.DefaultConfig()
	cfg.Workspace = filepath.Join(filepath.Dir(configPath), "workspace")
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
	runner.BuilderRuntimeConfig = config.BuilderRuntimeConfig{
		Enabled:      true,
		DefaultModel: &config.AgentModelConfig{Primary: "qwen2.5-coder-14b-local"},
		UpgradeModel: &config.AgentModelConfig{Primary: "qwen2.5-coder-32b-local"},
		UpgradeThreshold: config.BuilderRuntimeUpgradeThresholdConfig{
			MaxSchemaDriftBeforeUpgrade: 1,
		},
	}
	patchGenerator := &stubBuilderRuntimePatchGenerator{responses: map[string][]adapter.BuilderRuntimePatchResponse{
		"qwen2.5-coder-14b-local": {{Content: `{"patch_id":"round-1-patch","operations":[{"write_file":{"path":"lib/main.dart","content":"const title = 'Budget Flow';\n"}}]}`}},
		"qwen2.5-coder-32b-local": {{Content: `{"patch_id":"round-1-patch","operations":[{"type":"write_file","path":"lib/main.dart","content":"const title = 'Budget Flow';\n"}]}`}},
	}}
	runner.PatchGenerator = patchGenerator

	if err := runner.ExecuteRun(context.Background(), runID); err != nil {
		t.Fatalf("ExecuteRun() error = %v", err)
	}
	if got := strings.Join(patchGenerator.models, ","); got != "qwen2.5-coder-14b-local,qwen2.5-coder-32b-local" {
		t.Fatalf("models = %q, want 14b then 32b", got)
	}
	run := getJSON(t, server.Client(), server.URL+"/internal/v1/build-runs/"+runID)
	builderOutput := readBuilderOutput(t, run)
	roundOutputs, _ := builderOutput["round_outputs"].([]any)
	firstRound, _ := roundOutputs[0].(map[string]any)
	builderRuntime, ok := firstRound["builder_runtime_execution"].(map[string]any)
	if !ok {
		t.Fatalf("builder_runtime_execution = %T, want object", firstRound["builder_runtime_execution"])
	}
	if builderRuntime["selected_model"] != "qwen2.5-coder-32b-local" {
		t.Fatalf("selected_model = %v, want 32b", builderRuntime["selected_model"])
	}
	if builderRuntime["upgrade_applied"] != true {
		t.Fatalf("upgrade_applied = %v, want true", builderRuntime["upgrade_applied"])
	}
	if builderRuntime["schema_drift_count"] == nil || builderRuntime["schema_drift_count"].(float64) < 1 {
		t.Fatalf("schema_drift_count = %v, want >= 1", builderRuntime["schema_drift_count"])
	}
}

func TestRunnerExecuteRunUpgradesBuilderRuntimeModelOnUnrelatedOperationRate(t *testing.T) {
	t.Setenv("ONEAPPFACTORY_BUILDER_RUNTIME", "")
	t.Setenv("ONEAPPFACTORY_BUILDER_DOCKER_NETWORK", "")
	t.Setenv("HTTP_PROXY", "")
	t.Setenv("HTTPS_PROXY", "")
	t.Setenv("ALL_PROXY", "")
	t.Setenv("NO_PROXY", "")

	configPath := filepath.Join(t.TempDir(), "config.json")
	cfg := config.DefaultConfig()
	cfg.Workspace = filepath.Join(filepath.Dir(configPath), "workspace")
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
	runner.BuilderRuntimeConfig = config.BuilderRuntimeConfig{
		Enabled:      true,
		DefaultModel: &config.AgentModelConfig{Primary: "qwen2.5-coder-14b-local"},
		UpgradeModel: &config.AgentModelConfig{Primary: "qwen2.5-coder-32b-local"},
		UpgradeThreshold: config.BuilderRuntimeUpgradeThresholdConfig{
			MaxUnrelatedOperationRate: 0.4,
			UpgradeOnScopeViolation:   true,
		},
	}
	patchGenerator := &stubBuilderRuntimePatchGenerator{responses: map[string][]adapter.BuilderRuntimePatchResponse{
		"qwen2.5-coder-14b-local": {{Content: `{"patch_id":"round-1-patch","operations":[{"type":"write_file","path":"lib/main.dart","content":"const title = 'Budget Flow';\n"},{"type":"write_file","path":"lib/extra.dart","content":"const note = 'noise';\n"}]}`}},
		"qwen2.5-coder-32b-local": {{Content: `{"patch_id":"round-1-patch","operations":[{"type":"write_file","path":"lib/main.dart","content":"const title = 'Budget Flow';\n"}]}`}},
	}}
	runner.PatchGenerator = patchGenerator

	if err := runner.ExecuteRun(context.Background(), runID); err != nil {
		t.Fatalf("ExecuteRun() error = %v", err)
	}
	if got := strings.Join(patchGenerator.models, ","); got != "qwen2.5-coder-14b-local,qwen2.5-coder-32b-local" {
		t.Fatalf("models = %q, want 14b then 32b", got)
	}
	run := getJSON(t, server.Client(), server.URL+"/internal/v1/build-runs/"+runID)
	builderOutput := readBuilderOutput(t, run)
	roundOutputs, _ := builderOutput["round_outputs"].([]any)
	firstRound, _ := roundOutputs[0].(map[string]any)
	builderRuntime, ok := firstRound["builder_runtime_execution"].(map[string]any)
	if !ok {
		t.Fatalf("builder_runtime_execution = %T, want object", firstRound["builder_runtime_execution"])
	}
	if builderRuntime["selected_model"] != "qwen2.5-coder-32b-local" {
		t.Fatalf("selected_model = %v, want 32b", builderRuntime["selected_model"])
	}
	if builderRuntime["upgrade_applied"] != true {
		t.Fatalf("upgrade_applied = %v, want true", builderRuntime["upgrade_applied"])
	}
	if builderRuntime["scope_violation_count"] != float64(1) {
		t.Fatalf("scope_violation_count = %v, want 1", builderRuntime["scope_violation_count"])
	}
}

func TestRunnerExecuteRunNormalizesBuilderRuntimeSchemaAndTracksUnrelatedEdits(t *testing.T) {
	t.Setenv("ONEAPPFACTORY_BUILDER_RUNTIME", "")
	t.Setenv("ONEAPPFACTORY_BUILDER_DOCKER_NETWORK", "")
	t.Setenv("HTTP_PROXY", "")
	t.Setenv("HTTPS_PROXY", "")
	t.Setenv("ALL_PROXY", "")
	t.Setenv("NO_PROXY", "")

	configPath := filepath.Join(t.TempDir(), "config.json")
	cfg := config.DefaultConfig()
	cfg.Workspace = filepath.Join(filepath.Dir(configPath), "workspace")
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
	runner.BuilderRuntimeConfig = config.BuilderRuntimeConfig{
		Enabled:      true,
		DefaultModel: &config.AgentModelConfig{Primary: "qwen2.5-coder-14b-local"},
	}
	runner.PatchGenerator = &stubBuilderRuntimePatchGenerator{responses: map[string][]adapter.BuilderRuntimePatchResponse{
		"qwen2.5-coder-14b-local": {{Content: "```json\n{\"patch_id\":\"round-1-patch\",\"operations\":[{\"action\":\"write\",\"file_path\":\"lib/main.dart\",\"file_content\":\"const title = 'Budget Flow';\\n\"}]}\n```"}},
	}}

	if err := runner.ExecuteRun(context.Background(), runID); err != nil {
		t.Fatalf("ExecuteRun() error = %v", err)
	}
	run := getJSON(t, server.Client(), server.URL+"/internal/v1/build-runs/"+runID)
	builderOutput := readBuilderOutput(t, run)
	roundOutputs, _ := builderOutput["round_outputs"].([]any)
	firstRound, _ := roundOutputs[0].(map[string]any)
	builderRuntime, ok := firstRound["builder_runtime_execution"].(map[string]any)
	if !ok {
		t.Fatalf("builder_runtime_execution = %T, want object", firstRound["builder_runtime_execution"])
	}
	if builderRuntime["schema_normalized"] != true {
		t.Fatalf("schema_normalized = %v, want true", builderRuntime["schema_normalized"])
	}
	if builderRuntime["schema_drift_count"] == nil || builderRuntime["schema_drift_count"].(float64) < 1 {
		t.Fatalf("schema_drift_count = %v, want >= 1", builderRuntime["schema_drift_count"])
	}
	if builderRuntime["targeted_operation_count"] != float64(1) {
		t.Fatalf("targeted_operation_count = %v, want 1", builderRuntime["targeted_operation_count"])
	}
	if value, ok := builderRuntime["unrelated_operation_count"]; ok {
		t.Fatalf("unrelated_operation_count = %v, want omitted zero value", value)
	}
	if value, ok := builderRuntime["unrelated_operation_rate"]; ok {
		t.Fatalf("unrelated_operation_rate = %v, want omitted zero value", value)
	}
}

func TestRunnerExecuteRunClassifiesWorkspacePatchApplyFailure(t *testing.T) {
	t.Setenv("ONEAPPFACTORY_BUILDER_RUNTIME", "")
	t.Setenv("ONEAPPFACTORY_BUILDER_DOCKER_NETWORK", "")
	t.Setenv("HTTP_PROXY", "")
	t.Setenv("HTTPS_PROXY", "")
	t.Setenv("ALL_PROXY", "")
	t.Setenv("NO_PROXY", "")

	configPath := filepath.Join(t.TempDir(), "config.json")
	cfg := config.DefaultConfig()
	cfg.Workspace = filepath.Join(filepath.Dir(configPath), "workspace")
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
			"schema_version":  "0.1.0",
			"job_id":          "job-1",
			"prd_id":          "prd-1",
			"template_id":     "template-1",
			"planning_policy": samplePlanningPolicyMap(),
			"workspace_path":  "/workspace/job-1",
			"artifact_dir":    "/artifacts/job-1",
			"goal_summary":    "build android app",
			"task_bundle": []map[string]any{{
				"task_id":             "task-1",
				"title":               "Touch android config",
				"category":            "validation",
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
	eventsPath, _ := run["events_path"].(string)
	if strings.TrimSpace(eventsPath) == "" {
		t.Fatal("events_path should not be empty")
	}
	eventsData, err := os.ReadFile(filepath.Join(filepath.Clean(filepath.Join(workspacePath, "..", "..", "..")), filepath.FromSlash(eventsPath)))
	if err != nil {
		t.Fatalf("ReadFile(events_path) error = %v", err)
	}
	eventsText := string(eventsData)
	if !strings.Contains(eventsText, "run_patch_generation_started") {
		t.Fatalf("events missing run_patch_generation_started: %s", eventsText)
	}
	if !strings.Contains(eventsText, "run_patch_generated") {
		t.Fatalf("events missing run_patch_generated: %s", eventsText)
	}
	if !strings.Contains(eventsText, "run_patch_apply_failed") {
		t.Fatalf("events missing run_patch_apply_failed: %s", eventsText)
	}
}

func TestRunnerExecuteRunWritesFlutterHandoffProbeOnDefaultExecutor(t *testing.T) {
	t.Setenv("ONEAPPFACTORY_BUILDER_RUNTIME", "")
	t.Setenv("ONEAPPFACTORY_BUILDER_DOCKER_NETWORK", "")
	t.Setenv("HTTP_PROXY", "")
	t.Setenv("HTTPS_PROXY", "")
	t.Setenv("ALL_PROXY", "")
	t.Setenv("NO_PROXY", "")

	configPath := filepath.Join(t.TempDir(), "config.json")
	cfg := config.DefaultConfig()
	cfg.Workspace = filepath.Join(filepath.Dir(configPath), "workspace")
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
		"builder_input": sampleFlutterFallbackBuildRunInput(),
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
	if !ok {
		t.Fatalf("modified_files = %T, want array", builderOutput["modified_files"])
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
	if status, _ := workspacePatch["status"].(string); status != "applied" && status != "not_reported" {
		t.Fatalf("workspace_patch status = %#v, want applied or not_reported", workspacePatch["status"])
	}
	workspacePath, _ := run["workspace_path"].(string)
	if !modifiedPaths["lib/oneappfactory_executor_probe.dart"] {
		t.Fatalf("modified_files = %v, want probe path", modifiedPaths)
	}
	probe, err := os.ReadFile(filepath.Join(filepath.FromSlash(workspacePath), "lib", "oneappfactory_executor_probe.dart"))
	if err != nil {
		t.Fatalf("ReadFile(lib/oneappfactory_executor_probe.dart) error = %v", err)
	}
	if !strings.Contains(string(probe), "Generated by OneAppFactory thin executor.") || !strings.Contains(string(probe), "'goalSummary': 'deliver bookkeeping shell'") {
		t.Fatalf("probe = %q, want handoff metadata", string(probe))
	}
	if strings.Contains(string(probe), "BookkeepingApp") {
		t.Fatalf("probe should not embed hardcoded business implementation\n%s", string(probe))
	}
	if _, err := os.Stat(filepath.Join(filepath.FromSlash(workspacePath), "lib", "main.dart")); err != nil {
		t.Fatalf("expected seed file lib/main.dart to remain available: %v", err)
	}
	if _, err := os.Stat(filepath.Join(filepath.FromSlash(workspacePath), "lib", "models", "entry.dart")); err != nil {
		t.Fatalf("expected seeded bookkeeping model to remain available: %v", err)
	}
	if modifiedPaths["lib/models/entry.dart"] || modifiedPaths["lib/views/home_page.dart"] || modifiedPaths["lib/repositories/entry_repository.dart"] {
		t.Fatalf("thin executor fallback should only add the handoff probe, modified_paths=%v", modifiedPaths)
	}
	if _, err := os.Stat(filepath.Join(filepath.FromSlash(workspacePath), "pubspec.yaml")); err != nil {
		t.Fatalf("expected seed file pubspec.yaml to remain available: %v", err)
	}
}

func TestRunnerExecuteRunSkipsNoopClassificationAfterAppliedChanges(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	cfg := config.DefaultConfig()
	cfg.Workspace = filepath.Join(filepath.Dir(configPath), "workspace")
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
	t.Setenv("ONEAPPFACTORY_BUILDER_RUNTIME", "")
	t.Setenv("ONEAPPFACTORY_BUILDER_DOCKER_NETWORK", "")
	t.Setenv("HTTP_PROXY", "")
	t.Setenv("HTTPS_PROXY", "")
	t.Setenv("ALL_PROXY", "")
	t.Setenv("NO_PROXY", "")

	configPath := filepath.Join(t.TempDir(), "config.json")
	cfg := config.DefaultConfig()
	cfg.Workspace = filepath.Join(filepath.Dir(configPath), "workspace")
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
	t.Setenv("ONEAPPFACTORY_BUILDER_RUNTIME", "")
	t.Setenv("ONEAPPFACTORY_BUILDER_DOCKER_NETWORK", "")
	t.Setenv("HTTP_PROXY", "")
	t.Setenv("HTTPS_PROXY", "")
	t.Setenv("ALL_PROXY", "")
	t.Setenv("NO_PROXY", "")

	configPath := filepath.Join(t.TempDir(), "config.json")
	cfg := config.DefaultConfig()
	cfg.Workspace = filepath.Join(filepath.Dir(configPath), "workspace")
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
	t.Setenv("ONEAPPFACTORY_BUILDER_RUNTIME", "")
	t.Setenv("ONEAPPFACTORY_BUILDER_DOCKER_NETWORK", "")
	t.Setenv("HTTP_PROXY", "")
	t.Setenv("HTTPS_PROXY", "")
	t.Setenv("ALL_PROXY", "")
	t.Setenv("NO_PROXY", "")

	configPath := filepath.Join(t.TempDir(), "config.json")
	cfg := config.DefaultConfig()
	cfg.Workspace = filepath.Join(filepath.Dir(configPath), "workspace")
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
	builderInput := sampleBuildRunInputWithAcceptanceCheck("check-flutter-analyze", "flutter analyze", "cheap", []string{"flutter analyze"})
	builderInput["allowed_paths"] = []string{"lib/**", "test/**"}
	taskBundle, _ := builderInput["task_bundle"].([]map[string]any)
	taskBundle[0]["target_paths"] = []string{"lib/main.dart", "test/widget_test.dart"}
	builderInput["task_bundle"] = taskBundle
	create := postJSON(t, server.Client(), server.URL+"/internal/v1/build-runs", map[string]any{
		"worker_id":     "builder-a",
		"lease_id":      leaseID,
		"builder_input": builderInput,
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

func TestRunnerExecuteRunAutoRepairsFlutterAnalyzeFailure(t *testing.T) {
	t.Setenv("ONEAPPFACTORY_BUILDER_RUNTIME", "")
	t.Setenv("ONEAPPFACTORY_BUILDER_DOCKER_NETWORK", "")
	t.Setenv("HTTP_PROXY", "")
	t.Setenv("HTTPS_PROXY", "")
	t.Setenv("ALL_PROXY", "")
	t.Setenv("NO_PROXY", "")

	configPath := filepath.Join(t.TempDir(), "config.json")
	cfg := config.DefaultConfig()
	cfg.Workspace = filepath.Join(filepath.Dir(configPath), "workspace")
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
	runner.BuilderRuntimeConfig = config.BuilderRuntimeConfig{
		Enabled:      true,
		DefaultModel: &config.AgentModelConfig{Primary: "qwen2.5-coder-14b-local"},
		TaskRoutes: []config.BuilderRuntimeTaskRouteConfig{{
			TaskType: "analyze_repair",
			Model:    &config.AgentModelConfig{Primary: "qwen2.5-coder-14b-local"},
		}},
	}
	runner.PatchGenerator = &stubBuilderRuntimePatchGenerator{responses: map[string][]adapter.BuilderRuntimePatchResponse{
		"qwen2.5-coder-14b-local": {
			{Content: `{"patch_id":"initial-edit","operations":[{"type":"write_file","path":"lib/main.dart","content":"const title = 'Old Title';\n"}]}`},
			{Content: `{"patch_id":"analyze-repair","operations":[{"type":"write_file","path":"lib/main.dart","content":"const title = 'Budget Flow';\n"}]}`},
		},
	}}
	runner.Executor = failingValidationExecutor{
		checkID:  "check-flutter-analyze",
		label:    "flutter analyze",
		stage:    appruns.StageCheap,
		commands: []string{"flutter analyze"},
		script:   "grep -q 'Budget Flow' lib/main.dart || { echo flutter analyze failed before repair >&2; exit 1; }",
	}

	if err := runner.ExecuteRun(context.Background(), runID); err != nil {
		t.Fatalf("ExecuteRun() error = %v", err)
	}

	run := getJSON(t, server.Client(), server.URL+"/internal/v1/build-runs/"+runID)
	if run["status"] != "completed" {
		t.Fatalf("run status = %v, want completed", run["status"])
	}
	if summary, _ := run["failure_summary"].(string); strings.TrimSpace(summary) != "" {
		t.Fatalf("failure_summary = %q, want empty", summary)
	}
	builderOutput := readBuilderOutput(t, run)
	checksPassed, ok := builderOutput["checks_passed"].([]any)
	if !ok || len(checksPassed) != 1 {
		t.Fatalf("checks_passed = %#v, want one passed check", builderOutput["checks_passed"])
	}
	roundOutputs, _ := builderOutput["round_outputs"].([]any)
	firstRound, _ := roundOutputs[0].(map[string]any)
	builderRuntime, ok := firstRound["builder_runtime_execution"].(map[string]any)
	if !ok {
		t.Fatalf("builder_runtime_execution = %T, want object", firstRound["builder_runtime_execution"])
	}
	if builderRuntime["task_type"] != "analyze_repair" {
		t.Fatalf("task_type = %v, want analyze_repair", builderRuntime["task_type"])
	}
	if builderRuntime["attempts"] != float64(2) {
		t.Fatalf("attempts = %v, want 2", builderRuntime["attempts"])
	}
	t.Logf("auto_repair_task_type=%v", builderRuntime["task_type"])
	t.Logf("auto_repair_attempts=%v", builderRuntime["attempts"])
	t.Logf("auto_repair_recovered_check=%s", "check-flutter-analyze")
	workspacePatch, _ := firstRound["workspace_patch"].(map[string]any)
	if workspacePatch["status"] != "applied" {
		t.Fatalf("workspace_patch.status = %v, want applied", workspacePatch["status"])
	}
	mainDartPath := filepath.Join(filepath.Dir(configPath), "workspace", "appfactory", "jobs", "job-1", "workspace", "lib", "main.dart")
	content, err := os.ReadFile(mainDartPath)
	if err != nil {
		t.Fatalf("ReadFile(lib/main.dart) error = %v", err)
	}
	if !strings.Contains(string(content), "Budget Flow") {
		t.Fatalf("lib/main.dart = %q, want repaired content", string(content))
	}
	metricsData, err := os.ReadFile(filepath.Join(filepath.Dir(configPath), "workspace", "appfactory", "jobs", "job-1", "runs", runID, "metrics.json"))
	if err != nil {
		t.Fatalf("ReadFile(metrics) error = %v", err)
	}
	var metrics map[string]any
	if err := json.Unmarshal(metricsData, &metrics); err != nil {
		t.Fatalf("Unmarshal(metrics) error = %v", err)
	}
	if metrics["model_request_retries"] != float64(1) {
		t.Fatalf("model_request_retries = %v, want 1", metrics["model_request_retries"])
	}
	events := readRunEvents(t, run)
	var lastAppliedEvent map[string]any
	for _, event := range events {
		if event["type"] == "run_patch_applied" {
			lastAppliedEvent = event
		}
	}
	if lastAppliedEvent == nil {
		t.Fatalf("events missing run_patch_applied: %#v", events)
	}
	if lastAppliedEvent["attempt"] != float64(2) {
		t.Fatalf("last run_patch_applied attempt = %v, want 2", lastAppliedEvent["attempt"])
	}
}

func TestRunnerExecuteRunUpgradesValidationRepairAfterAnalyzeStillFails(t *testing.T) {
	t.Setenv("ONEAPPFACTORY_BUILDER_RUNTIME", "")
	t.Setenv("ONEAPPFACTORY_BUILDER_DOCKER_NETWORK", "")
	t.Setenv("HTTP_PROXY", "")
	t.Setenv("HTTPS_PROXY", "")
	t.Setenv("ALL_PROXY", "")
	t.Setenv("NO_PROXY", "")

	configPath := filepath.Join(t.TempDir(), "config.json")
	cfg := config.DefaultConfig()
	cfg.Workspace = filepath.Join(filepath.Dir(configPath), "workspace")
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
	runner.BuilderRuntimeConfig = config.BuilderRuntimeConfig{
		Enabled:      true,
		DefaultModel: &config.AgentModelConfig{Primary: "qwen2.5-coder-14b-local"},
		UpgradeModel: &config.AgentModelConfig{Primary: "qwen2.5-coder-32b-local"},
		UpgradeThreshold: config.BuilderRuntimeUpgradeThresholdConfig{
			UpgradeOnValidationFail: true,
		},
		TaskRoutes: []config.BuilderRuntimeTaskRouteConfig{{
			TaskType: "analyze_repair",
			Model:    &config.AgentModelConfig{Primary: "qwen2.5-coder-14b-local"},
		}},
	}
	runner.PatchGenerator = &stubBuilderRuntimePatchGenerator{responses: map[string][]adapter.BuilderRuntimePatchResponse{
		"qwen2.5-coder-14b-local": {
			{Content: `{"patch_id":"initial-edit","operations":[{"type":"write_file","path":"lib/main.dart","content":"const title = 'Old Title';\n"}]}`},
			{Content: `{"patch_id":"analyze-repair-local","operations":[{"type":"write_file","path":"lib/main.dart","content":"const title = 'Almost Fixed';\n"}]}`},
		},
		"qwen2.5-coder-32b-local": {
			{Content: `{"patch_id":"analyze-repair-upgrade","operations":[{"type":"write_file","path":"lib/main.dart","content":"const title = 'Budget Flow';\n"}]}`},
		},
	}}
	runner.Executor = failingValidationExecutor{
		checkID:  "check-flutter-analyze",
		label:    "flutter analyze",
		stage:    appruns.StageCheap,
		commands: []string{"flutter analyze"},
		script:   "grep -q 'Budget Flow' lib/main.dart || { echo flutter analyze still failing >&2; exit 1; }",
	}

	if err := runner.ExecuteRun(context.Background(), runID); err != nil {
		t.Fatalf("ExecuteRun() error = %v", err)
	}

	run := getJSON(t, server.Client(), server.URL+"/internal/v1/build-runs/"+runID)
	if run["status"] != "completed" {
		t.Fatalf("run status = %v, want completed", run["status"])
	}
	builderOutput := readBuilderOutput(t, run)
	roundOutputs, _ := builderOutput["round_outputs"].([]any)
	firstRound, _ := roundOutputs[0].(map[string]any)
	builderRuntime, ok := firstRound["builder_runtime_execution"].(map[string]any)
	if !ok {
		t.Fatalf("builder_runtime_execution = %T, want object", firstRound["builder_runtime_execution"])
	}
	if builderRuntime["selected_model"] != "qwen2.5-coder-32b-local" {
		t.Fatalf("selected_model = %v, want qwen2.5-coder-32b-local", builderRuntime["selected_model"])
	}
	if builderRuntime["upgrade_applied"] != true {
		t.Fatalf("upgrade_applied = %v, want true", builderRuntime["upgrade_applied"])
	}
	if builderRuntime["task_type"] != "analyze_repair" {
		t.Fatalf("task_type = %v, want analyze_repair", builderRuntime["task_type"])
	}
	if builderRuntime["attempts"] != float64(3) {
		t.Fatalf("attempts = %v, want 3", builderRuntime["attempts"])
	}
}

func TestRunnerExecuteRunRepeatsValidationRepairWithLatestAnalyzeFailureContext(t *testing.T) {
	t.Setenv("ONEAPPFACTORY_BUILDER_RUNTIME", "")
	t.Setenv("ONEAPPFACTORY_BUILDER_DOCKER_NETWORK", "")
	t.Setenv("HTTP_PROXY", "")
	t.Setenv("HTTPS_PROXY", "")
	t.Setenv("ALL_PROXY", "")
	t.Setenv("NO_PROXY", "")

	configPath := filepath.Join(t.TempDir(), "config.json")
	cfg := config.DefaultConfig()
	cfg.Workspace = filepath.Join(filepath.Dir(configPath), "workspace")
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

	generator := &stubBuilderRuntimePatchGenerator{responses: map[string][]adapter.BuilderRuntimePatchResponse{
		"qwen2.5-coder-14b-local": {
			{Content: `{"patch_id":"initial-edit","operations":[{"type":"write_file","path":"lib/main.dart","content":"const title = 'Old Title';\n"}]}`},
			{Content: `{"patch_id":"analyze-repair-1","operations":[{"type":"write_file","path":"lib/main.dart","content":"const title = 'First Fix';\n"}]}`},
			{Content: `{"patch_id":"analyze-repair-2","operations":[{"type":"write_file","path":"lib/main.dart","content":"const title = 'Second Fix';\n"}]}`},
			{Content: `{"patch_id":"analyze-repair-3","operations":[{"type":"write_file","path":"lib/main.dart","content":"const title = 'Third Fix';\n"}]}`},
		},
	}}

	runner := adapter.NewRunner(server.URL)
	runner.BuilderRuntimeConfig = config.BuilderRuntimeConfig{
		Enabled:      true,
		DefaultModel: &config.AgentModelConfig{Primary: "qwen2.5-coder-14b-local"},
		TaskRoutes: []config.BuilderRuntimeTaskRouteConfig{{
			TaskType: "analyze_repair",
			Model:    &config.AgentModelConfig{Primary: "qwen2.5-coder-14b-local"},
		}},
	}
	runner.PatchGenerator = generator
	runner.Executor = failingValidationExecutor{
		checkID:  "check-flutter-analyze",
		label:    "flutter analyze",
		stage:    appruns.StageCheap,
		commands: []string{"flutter analyze"},
		script: strings.Join([]string{
			"if grep -q \"Third Fix\" lib/main.dart; then",
			"  exit 0",
			"fi",
			"if grep -q \"Second Fix\" lib/main.dart; then",
			"  echo 'Analyzing workspace...'",
			"  echo \"warning • Unused import: 'package:bookkeeping_lite/models/entry.dart' • test/widget_test.dart:6:8 • unused_import\"",
			"  echo '1 issue found. (ran in 1.7s)'",
			"  exit 1",
			"fi",
			"if grep -q \"First Fix\" lib/main.dart; then",
			"  echo 'Analyzing workspace...'",
			"  echo \"  error • The function 'InMemoryEntryRepository' isn't defined • test/widget_test.dart:11:34 • undefined_function\"",
			"  echo '1 issue found. (ran in 1.7s)'",
			"  exit 1",
			"fi",
			"echo 'Analyzing workspace...'",
			"echo \"  error • The getter 'entryCount' isn't defined for the type 'Summary' • lib/views/home_page.dart:107:38 • undefined_getter\"",
			"echo \"  error • The function 'InMemoryEntryRepository' isn't defined • test/widget_test.dart:11:34 • undefined_function\"",
			"echo '2 issues found. (ran in 1.7s)'",
			"exit 1",
		}, "\n"),
	}

	if err := runner.ExecuteRun(context.Background(), runID); err != nil {
		t.Fatalf("ExecuteRun() error = %v", err)
	}

	run := getJSON(t, server.Client(), server.URL+"/internal/v1/build-runs/"+runID)
	if run["status"] != "completed" {
		t.Fatalf("run status = %v, want completed", run["status"])
	}
	builderOutput := readBuilderOutput(t, run)
	roundOutputs, _ := builderOutput["round_outputs"].([]any)
	firstRound, _ := roundOutputs[0].(map[string]any)
	builderRuntime, ok := firstRound["builder_runtime_execution"].(map[string]any)
	if !ok {
		t.Fatalf("builder_runtime_execution = %T, want object", firstRound["builder_runtime_execution"])
	}
	if builderRuntime["attempts"] != float64(4) {
		t.Fatalf("attempts = %v, want 4", builderRuntime["attempts"])
	}
	if len(generator.requests) != 4 {
		t.Fatalf("len(generator.requests) = %d, want 4", len(generator.requests))
	}
	if generator.requests[3].RoundInput.Attempt != 4 {
		t.Fatalf("third repair attempt = %d, want 4", generator.requests[3].RoundInput.Attempt)
	}
}

func TestRunnerExecuteRunAllowsFourthValidationRepairRoundWithLatestAnalyzeFailureContext(t *testing.T) {
	t.Setenv("ONEAPPFACTORY_BUILDER_RUNTIME", "")
	t.Setenv("ONEAPPFACTORY_BUILDER_DOCKER_NETWORK", "")
	t.Setenv("HTTP_PROXY", "")
	t.Setenv("HTTPS_PROXY", "")
	t.Setenv("ALL_PROXY", "")
	t.Setenv("NO_PROXY", "")

	configPath := filepath.Join(t.TempDir(), "config.json")
	cfg := config.DefaultConfig()
	cfg.Workspace = filepath.Join(filepath.Dir(configPath), "workspace")
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

	generator := &stubBuilderRuntimePatchGenerator{responses: map[string][]adapter.BuilderRuntimePatchResponse{
		"qwen2.5-coder-14b-local": {
			{Content: `{"patch_id":"initial-edit","operations":[{"type":"write_file","path":"lib/main.dart","content":"const title = 'Old Title';\n"}]}`},
			{Content: `{"patch_id":"analyze-repair-1","operations":[{"type":"write_file","path":"lib/main.dart","content":"const title = 'First Fix';\n"}]}`},
			{Content: `{"patch_id":"analyze-repair-2","operations":[{"type":"write_file","path":"lib/main.dart","content":"const title = 'Second Fix';\n"}]}`},
			{Content: `{"patch_id":"analyze-repair-3","operations":[{"type":"write_file","path":"lib/main.dart","content":"const title = 'Third Fix';\n"}]}`},
			{Content: `{"patch_id":"analyze-repair-4","operations":[{"type":"write_file","path":"lib/main.dart","content":"const title = 'Fourth Fix';\n"}]}`},
		},
	}}

	runner := adapter.NewRunner(server.URL)
	runner.BuilderRuntimeConfig = config.BuilderRuntimeConfig{
		Enabled:      true,
		DefaultModel: &config.AgentModelConfig{Primary: "qwen2.5-coder-14b-local"},
		TaskRoutes: []config.BuilderRuntimeTaskRouteConfig{{
			TaskType: "analyze_repair",
			Model:    &config.AgentModelConfig{Primary: "qwen2.5-coder-14b-local"},
		}},
	}
	runner.PatchGenerator = generator
	runner.Executor = failingValidationExecutor{
		checkID:  "check-flutter-analyze",
		label:    "flutter analyze",
		stage:    appruns.StageCheap,
		commands: []string{"flutter analyze"},
		script: strings.Join([]string{
			"if grep -q \"Fourth Fix\" lib/main.dart; then",
			"  exit 0",
			"fi",
			"if grep -q \"Third Fix\" lib/main.dart; then",
			"  echo 'Analyzing workspace...'",
			"  echo \"  error • Undefined name 'openLite0Copy' • lib/views/home_page.dart:180:16 • undefined_identifier\"",
			"  echo '1 issue found. (ran in 1.4s)'",
			"  exit 1",
			"fi",
			"if grep -q \"Second Fix\" lib/main.dart; then",
			"  echo 'Analyzing workspace...'",
			"  echo \"warning • Unused import: 'package:bookkeeping_lite/models/entry.dart' • test/widget_test.dart:6:8 • unused_import\"",
			"  echo '1 issue found. (ran in 1.7s)'",
			"  exit 1",
			"fi",
			"if grep -q \"First Fix\" lib/main.dart; then",
			"  echo 'Analyzing workspace...'",
			"  echo \"  error • The function 'InMemoryEntryRepository' isn't defined • test/widget_test.dart:11:34 • undefined_function\"",
			"  echo '1 issue found. (ran in 1.7s)'",
			"  exit 1",
			"fi",
			"echo 'Analyzing workspace...'",
			"echo \"  error • The getter 'entryCount' isn't defined for the type 'Summary' • lib/views/home_page.dart:107:38 • undefined_getter\"",
			"echo \"  error • The function 'InMemoryEntryRepository' isn't defined • test/widget_test.dart:11:34 • undefined_function\"",
			"echo '2 issues found. (ran in 1.7s)'",
			"exit 1",
		}, "\n"),
	}

	if err := runner.ExecuteRun(context.Background(), runID); err != nil {
		t.Fatalf("ExecuteRun() error = %v", err)
	}

	run := getJSON(t, server.Client(), server.URL+"/internal/v1/build-runs/"+runID)
	if run["status"] != "completed" {
		t.Fatalf("run status = %v, want completed", run["status"])
	}
	builderOutput := readBuilderOutput(t, run)
	roundOutputs, _ := builderOutput["round_outputs"].([]any)
	firstRound, _ := roundOutputs[0].(map[string]any)
	builderRuntime, ok := firstRound["builder_runtime_execution"].(map[string]any)
	if !ok {
		t.Fatalf("builder_runtime_execution = %T, want object", firstRound["builder_runtime_execution"])
	}
	if builderRuntime["attempts"] != float64(5) {
		t.Fatalf("attempts = %v, want 5", builderRuntime["attempts"])
	}
	if len(generator.requests) != 5 {
		t.Fatalf("len(generator.requests) = %d, want 5", len(generator.requests))
	}
	if generator.requests[4].RoundInput.Attempt != 5 {
		t.Fatalf("fourth repair attempt = %d, want 5", generator.requests[4].RoundInput.Attempt)
	}
	if strings.TrimSpace(generator.requests[4].FailureContext) == "" {
		t.Fatalf("fourth repair FailureContext = %q, want non-empty context", generator.requests[4].FailureContext)
	}
}

func TestRunnerExecuteRunAnalyzeRepairCarriesFailureSliceIntoCoverageRepair(t *testing.T) {
	t.Setenv("ONEAPPFACTORY_BUILDER_RUNTIME", "")
	t.Setenv("ONEAPPFACTORY_BUILDER_DOCKER_NETWORK", "")
	t.Setenv("HTTP_PROXY", "")
	t.Setenv("HTTPS_PROXY", "")
	t.Setenv("ALL_PROXY", "")
	t.Setenv("NO_PROXY", "")

	configPath := filepath.Join(t.TempDir(), "config.json")
	cfg := config.DefaultConfig()
	cfg.Workspace = filepath.Join(filepath.Dir(configPath), "workspace")
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
	builderInput := sampleBuildRunInputWithAcceptanceCheck("check-flutter-analyze", "flutter analyze", "cheap", []string{"flutter analyze"})
	builderInput["allowed_paths"] = []string{"lib/**", "test/**"}
	builderInput["task_bundle"] = []map[string]any{
		{
			"task_id":             "task-bootstrap",
			"title":               "bootstrap repair fixture",
			"category":            "screen",
			"task_type":           "dual_file_wiring",
			"objective":           "seed analyze repair fixture files",
			"target_paths":        []string{"lib/views/home_page.dart", "lib/controllers/home_controller.dart", "lib/models/entry.dart"},
			"completion_criteria": []string{"repair fixture files exist"},
		},
		{
			"task_id":             "task-domain-entry",
			"title":               "entry domain model",
			"category":            "domain",
			"task_type":           "dual_file_wiring",
			"objective":           "define the entry domain model",
			"target_paths":        []string{"lib/models/entry.dart"},
			"completion_criteria": []string{"entry model exists"},
		},
		{
			"task_id":             "task-home-controller",
			"title":               "home controller",
			"category":            "flow",
			"task_type":           "dual_file_wiring",
			"objective":           "wire the home controller",
			"dependencies":        []string{"task-domain-entry"},
			"target_paths":        []string{"lib/controllers/home_controller.dart"},
			"completion_criteria": []string{"home controller exists"},
		},
		{
			"task_id":             "task-home-page",
			"title":               "home page",
			"category":            "screen",
			"task_type":           "dual_file_wiring",
			"objective":           "wire the home page",
			"dependencies":        []string{"task-home-controller"},
			"target_paths":        []string{"lib/views/home_page.dart"},
			"completion_criteria": []string{"home page exists"},
		},
	}
	create := postJSON(t, server.Client(), server.URL+"/internal/v1/build-runs", map[string]any{
		"worker_id":     "builder-a",
		"lease_id":      leaseID,
		"builder_input": builderInput,
	})
	runID, _ := create["run_id"].(string)

	generator := &stubBuilderRuntimePatchGenerator{responses: map[string][]adapter.BuilderRuntimePatchResponse{
		"qwen2.5-coder-14b-local": {
			{Content: strings.Join([]string{
				`{"patch_id":"initial-edit","operations":[`,
				`{"type":"write_file","path":"lib/models/entry.dart","content":"class Entry {}\n"},`,
				`{"type":"write_file","path":"lib/controllers/home_controller.dart","content":"import '../models/entry.dart';\nclass HomeController {\n  final Entry? latestEntry = null;\n}\n"},`,
				`{"type":"write_file","path":"lib/views/home_page.dart","content":"import '../controllers/home_controller.dart';\nfinal controller = HomeController();\nString buildHeadline() => controller.displayTitle;\n"}`,
				`]}`,
			}, "")},
			{Content: `{"patch_id":"analyze-repair-controller-only","operations":[{"type":"write_file","path":"lib/controllers/home_controller.dart","content":"import '../models/entry.dart';\nclass HomeController {\n  final Entry? latestEntry = null;\n  String get displayTitle => 'Budget Flow';\n}\n"}]}`},
			{Content: `{"patch_id":"analyze-repair-home-page","operations":[{"type":"write_file","path":"lib/views/home_page.dart","content":"import '../controllers/home_controller.dart';\nfinal controller = HomeController();\nString buildHeadline() => 'Budget Flow';\n"}]}`},
			{Content: `{"patch_id":"analyze-repair-model","operations":[{"type":"write_file","path":"lib/models/entry.dart","content":"class Entry {\n  const Entry();\n}\n"}]}`},
		},
	}}

	runner := adapter.NewRunner(server.URL)
	runner.BuilderRuntimeConfig = config.BuilderRuntimeConfig{
		Enabled:      true,
		DefaultModel: &config.AgentModelConfig{Primary: "qwen2.5-coder-14b-local"},
		TaskRoutes: []config.BuilderRuntimeTaskRouteConfig{{
			TaskType: "analyze_repair",
			Model:    &config.AgentModelConfig{Primary: "qwen2.5-coder-14b-local"},
		}},
	}
	runner.PatchGenerator = generator
	runner.Executor = failingValidationExecutor{
		checkID:  "check-flutter-analyze",
		label:    "flutter analyze",
		stage:    appruns.StageCheap,
		commands: []string{"flutter analyze"},
		script: strings.Join([]string{
			"if grep -q 'Budget Flow' lib/views/home_page.dart; then",
			"  exit 0",
			"fi",
			"echo '{\"patch_id\":\"noise\",\"operations\":[{\"type\":\"write_file\",\"path\":\"lib/noise.dart\",\"content\":\"noise\"}]}'",
			"echo 'Analyzing workspace...'",
			"echo \"error • current failure • lib/views/home_page.dart:3:36 • undefined_getter\"",
			"echo '1 issue found. (ran in 1.2s)'",
			"exit 1",
		}, "\n"),
	}

	if err := runner.ExecuteRun(context.Background(), runID); err != nil {
		t.Fatalf("ExecuteRun() error = %v", err)
	}

	run := getJSON(t, server.Client(), server.URL+"/internal/v1/build-runs/"+runID)
	if run["status"] != "completed" {
		t.Fatalf("run status = %v, want completed (failure_summary=%v, requests=%s)", run["status"], run["failure_summary"], summarizePatchRequests(generator.requests))
	}
	if len(generator.requests) != 4 {
		t.Fatalf("len(generator.requests) = %d, want 4", len(generator.requests))
	}
	repairRequest := generator.requests[1]
	if repairRequest.Route.TaskType != appruns.BuilderRuntimeTaskTypeAnalyzeRepair {
		t.Fatalf("repair task type = %v, want analyze_repair", repairRequest.Route.TaskType)
	}
	if repairRequest.RoundInput.Attempt != 2 {
		t.Fatalf("repair attempt = %d, want 2", repairRequest.RoundInput.Attempt)
	}
	if !strings.Contains(repairRequest.FailureContext, "check=check-flutter-analyze") {
		t.Fatalf("failure context should keep the current analyze check id: %q", repairRequest.FailureContext)
	}
	repairTask := mustFindTaskBundleItem(t, repairRequest.Run.TaskBundle, "repair-check-flutter-analyze")
	assertExactTargetPaths(t, repairTask.TargetPaths, []string{"lib/controllers/home_controller.dart", "lib/models/entry.dart", "lib/views/home_page.dart"})
	if !hasTaskBundleItem(repairRequest.Run.TaskBundle, "task-domain-entry") {
		t.Fatalf("repair context should keep the dependent domain task")
	}
	if !hasTaskBundleItem(repairRequest.Run.TaskBundle, "task-home-controller") {
		t.Fatalf("repair context should keep the imported controller task")
	}
	coverageRepairRequest := generator.requests[2]
	if coverageRepairRequest.RepairOnly != true {
		t.Fatalf("coverage repair should be repair-only")
	}
	if !strings.Contains(coverageRepairRequest.PreviousErr, "lib/models/entry.dart") || !strings.Contains(coverageRepairRequest.PreviousErr, "lib/views/home_page.dart") {
		t.Fatalf("coverage repair should name the remaining missing targets: %q", coverageRepairRequest.PreviousErr)
	}
	coverageRepairTask := mustFindTaskBundleItem(t, coverageRepairRequest.Run.TaskBundle, "repair-check-flutter-analyze")
	assertExactTargetPaths(t, coverageRepairTask.TargetPaths, []string{"lib/models/entry.dart", "lib/views/home_page.dart"})
	finalCoverageRequest := generator.requests[3]
	if finalCoverageRequest.RepairOnly != true {
		t.Fatalf("final coverage repair should be repair-only")
	}
	if !strings.Contains(finalCoverageRequest.PreviousErr, "lib/models/entry.dart") {
		t.Fatalf("final coverage repair should narrow to the last missing model target: %q", finalCoverageRequest.PreviousErr)
	}
	finalCoverageTask := mustFindTaskBundleItem(t, finalCoverageRequest.Run.TaskBundle, "repair-check-flutter-analyze")
	assertExactTargetPaths(t, finalCoverageTask.TargetPaths, []string{"lib/models/entry.dart"})
	builderOutput := readBuilderOutput(t, run)
	roundOutputs, _ := builderOutput["round_outputs"].([]any)
	firstRound, _ := roundOutputs[0].(map[string]any)
	builderRuntime, ok := firstRound["builder_runtime_execution"].(map[string]any)
	if !ok {
		t.Fatalf("builder_runtime_execution = %T, want object", firstRound["builder_runtime_execution"])
	}
	if builderRuntime["attempts"] != float64(4) {
		t.Fatalf("attempts = %v, want 4", builderRuntime["attempts"])
	}
}

func TestRunnerExecuteRunFlutterTestRepairUsesLastFailureBlockAndCoverageRetry(t *testing.T) {
	t.Setenv("ONEAPPFACTORY_BUILDER_RUNTIME", "")
	t.Setenv("ONEAPPFACTORY_BUILDER_DOCKER_NETWORK", "")
	t.Setenv("HTTP_PROXY", "")
	t.Setenv("HTTPS_PROXY", "")
	t.Setenv("ALL_PROXY", "")
	t.Setenv("NO_PROXY", "")

	configPath := filepath.Join(t.TempDir(), "config.json")
	cfg := config.DefaultConfig()
	cfg.Workspace = filepath.Join(filepath.Dir(configPath), "workspace")
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
	builderInput := sampleBuildRunInputWithAcceptanceCheck("check-flutter-test", "flutter test", "cheap", []string{"flutter test"})
	builderInput["allowed_paths"] = []string{"lib/**", "test/**"}
	builderInput["task_bundle"] = []map[string]any{
		{
			"task_id":             "task-bootstrap",
			"title":               "bootstrap test repair fixture",
			"category":            "screen",
			"task_type":           "dual_file_wiring",
			"objective":           "seed flutter test repair fixture files",
			"target_paths":        []string{"test/widget_test.dart", "lib/controllers/home_controller.dart", "lib/models/entry.dart"},
			"completion_criteria": []string{"test repair fixture files exist"},
		},
		{
			"task_id":             "task-domain-entry",
			"title":               "entry domain model",
			"category":            "domain",
			"task_type":           "dual_file_wiring",
			"objective":           "define the entry domain model",
			"target_paths":        []string{"lib/models/entry.dart"},
			"completion_criteria": []string{"entry model exists"},
		},
		{
			"task_id":             "task-home-controller",
			"title":               "home controller",
			"category":            "flow",
			"task_type":           "dual_file_wiring",
			"objective":           "wire the home controller",
			"dependencies":        []string{"task-domain-entry"},
			"target_paths":        []string{"lib/controllers/home_controller.dart"},
			"completion_criteria": []string{"home controller exists"},
		},
		{
			"task_id":             "task-widget-test",
			"title":               "widget smoke test",
			"category":            "validation",
			"task_type":           "dual_file_wiring",
			"objective":           "wire the widget smoke test",
			"dependencies":        []string{"task-home-controller"},
			"target_paths":        []string{"test/widget_test.dart"},
			"completion_criteria": []string{"widget test exists"},
		},
	}
	create := postJSON(t, server.Client(), server.URL+"/internal/v1/build-runs", map[string]any{
		"worker_id":     "builder-a",
		"lease_id":      leaseID,
		"builder_input": builderInput,
	})
	runID, _ := create["run_id"].(string)

	generator := &stubBuilderRuntimePatchGenerator{responses: map[string][]adapter.BuilderRuntimePatchResponse{
		"qwen2.5-coder-14b-local": {
			{Content: strings.Join([]string{
				`{"patch_id":"initial-edit","operations":[`,
				`{"type":"write_file","path":"lib/models/entry.dart","content":"class Entry {}\n"},`,
				`{"type":"write_file","path":"lib/controllers/home_controller.dart","content":"import '../models/entry.dart';\nclass HomeController {\n  final Entry? latestEntry = null;\n}\n"},`,
				`{"type":"write_file","path":"test/widget_test.dart","content":"import '../lib/controllers/home_controller.dart';\nfinal controller = HomeController();\nString smokeTest() => controller.displayTitle;\n"}`,
				`]}`,
			}, "")},
			{Content: `{"patch_id":"test-repair-controller-only","operations":[{"type":"write_file","path":"lib/controllers/home_controller.dart","content":"import '../models/entry.dart';\nclass HomeController {\n  final Entry? latestEntry = null;\n  String get displayTitle => 'repair done';\n}\n"}]}`},
			{Content: `{"patch_id":"test-repair-widget-test","operations":[{"type":"write_file","path":"test/widget_test.dart","content":"import '../lib/controllers/home_controller.dart';\nfinal controller = HomeController();\nString smokeTest() => 'repair done';\n"}]}`},
			{Content: `{"patch_id":"test-repair-model","operations":[{"type":"write_file","path":"lib/models/entry.dart","content":"class Entry {\n  const Entry();\n}\n"}]}`},
		},
	}}

	runner := adapter.NewRunner(server.URL)
	runner.BuilderRuntimeConfig = config.BuilderRuntimeConfig{
		Enabled:      true,
		DefaultModel: &config.AgentModelConfig{Primary: "qwen2.5-coder-14b-local"},
		TaskRoutes: []config.BuilderRuntimeTaskRouteConfig{{
			TaskType: "test_repair",
			Model:    &config.AgentModelConfig{Primary: "qwen2.5-coder-14b-local"},
		}},
	}
	runner.PatchGenerator = generator
	runner.Executor = failingValidationExecutor{
		checkID:  "check-flutter-test",
		label:    "flutter test",
		stage:    appruns.StageCheap,
		commands: []string{"flutter test"},
		script: strings.Join([]string{
			"if grep -q 'repair done' test/widget_test.dart; then",
			"  exit 0",
			"fi",
			"workspace=$(pwd)",
			"echo '{\"patch_id\":\"noise\",\"operations\":[{\"type\":\"write_file\",\"path\":\"test/noise_test.dart\",\"content\":\"noise\"}]}'",
			"echo '00:00 +0: loading /tmp/other/test/old_widget_test.dart'",
			"echo '00:01 +0: old test passes'",
			"echo \"00:02 +0: loading $workspace/test/widget_test.dart\"",
			"echo \"#2      main.<anonymous closure> (file://$workspace/test/widget_test.dart:3:23)\"",
			"echo '00:03 +0 -1: Home screen smoke test [E]'",
			"echo 'Some tests failed.'",
			"exit 1",
		}, "\n"),
	}

	if err := runner.ExecuteRun(context.Background(), runID); err != nil {
		t.Fatalf("ExecuteRun() error = %v", err)
	}

	run := getJSON(t, server.Client(), server.URL+"/internal/v1/build-runs/"+runID)
	if run["status"] != "completed" {
		t.Fatalf("run status = %v, want completed (failure_summary=%v, requests=%s)", run["status"], run["failure_summary"], summarizePatchRequests(generator.requests))
	}
	if len(generator.requests) != 4 {
		t.Fatalf("len(generator.requests) = %d, want 4", len(generator.requests))
	}
	repairRequest := generator.requests[1]
	if repairRequest.Route.TaskType != appruns.BuilderRuntimeTaskTypeTestRepair {
		t.Fatalf("repair task type = %v, want test_repair", repairRequest.Route.TaskType)
	}
	if repairRequest.RoundInput.Attempt != 2 {
		t.Fatalf("repair attempt = %d, want 2", repairRequest.RoundInput.Attempt)
	}
	if !strings.Contains(repairRequest.FailureContext, "check=check-flutter-test") {
		t.Fatalf("failure context should keep the current flutter test check id: %q", repairRequest.FailureContext)
	}
	repairTask := mustFindTaskBundleItem(t, repairRequest.Run.TaskBundle, "repair-check-flutter-test")
	assertExactTargetPaths(t, repairTask.TargetPaths, []string{"lib/controllers/home_controller.dart", "lib/models/entry.dart", "test/widget_test.dart"})
	if !hasTaskBundleItem(repairRequest.Run.TaskBundle, "task-domain-entry") {
		t.Fatalf("repair context should keep the dependent domain task")
	}
	if !hasTaskBundleItem(repairRequest.Run.TaskBundle, "task-home-controller") {
		t.Fatalf("repair context should keep the imported controller task")
	}
	coverageRepairRequest := generator.requests[2]
	if coverageRepairRequest.RepairOnly != true {
		t.Fatalf("coverage repair should be repair-only")
	}
	if !strings.Contains(coverageRepairRequest.PreviousErr, "lib/models/entry.dart") || !strings.Contains(coverageRepairRequest.PreviousErr, "test/widget_test.dart") {
		t.Fatalf("coverage repair should name the remaining missing targets: %q", coverageRepairRequest.PreviousErr)
	}
	coverageRepairTask := mustFindTaskBundleItem(t, coverageRepairRequest.Run.TaskBundle, "repair-check-flutter-test")
	assertExactTargetPaths(t, coverageRepairTask.TargetPaths, []string{"lib/models/entry.dart", "test/widget_test.dart"})
	finalCoverageRequest := generator.requests[3]
	if finalCoverageRequest.RepairOnly != true {
		t.Fatalf("final coverage repair should be repair-only")
	}
	if !strings.Contains(finalCoverageRequest.PreviousErr, "lib/models/entry.dart") {
		t.Fatalf("final coverage repair should narrow to the last missing model target: %q", finalCoverageRequest.PreviousErr)
	}
	finalCoverageTask := mustFindTaskBundleItem(t, finalCoverageRequest.Run.TaskBundle, "repair-check-flutter-test")
	assertExactTargetPaths(t, finalCoverageTask.TargetPaths, []string{"lib/models/entry.dart"})
	builderOutput := readBuilderOutput(t, run)
	roundOutputs, _ := builderOutput["round_outputs"].([]any)
	firstRound, _ := roundOutputs[0].(map[string]any)
	builderRuntime, ok := firstRound["builder_runtime_execution"].(map[string]any)
	if !ok {
		t.Fatalf("builder_runtime_execution = %T, want object", firstRound["builder_runtime_execution"])
	}
	if builderRuntime["attempts"] != float64(4) {
		t.Fatalf("attempts = %v, want 4", builderRuntime["attempts"])
	}
}

func TestRunnerExecuteRunUpgradesSemanticConflictRepair(t *testing.T) {
	t.Setenv("ONEAPPFACTORY_BUILDER_RUNTIME", "")
	t.Setenv("ONEAPPFACTORY_BUILDER_DOCKER_NETWORK", "")
	t.Setenv("HTTP_PROXY", "")
	t.Setenv("HTTPS_PROXY", "")
	t.Setenv("ALL_PROXY", "")
	t.Setenv("NO_PROXY", "")

	configPath := filepath.Join(t.TempDir(), "config.json")
	cfg := config.DefaultConfig()
	cfg.Workspace = filepath.Join(filepath.Dir(configPath), "workspace")
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
		"builder_input": sampleBuildRunInputWithAcceptanceCheck("check-profile-open-lite-domain-branding", "domain branding", "cheap", []string{"grep Weight Tracker lib/main.dart"}),
	})
	runID, _ := create["run_id"].(string)

	runner := adapter.NewRunner(server.URL)
	runner.BuilderRuntimeConfig = config.BuilderRuntimeConfig{
		Enabled:      true,
		DefaultModel: &config.AgentModelConfig{Primary: "qwen2.5-coder-14b-local"},
		UpgradeModel: &config.AgentModelConfig{Primary: "qwen2.5-coder-32b-local"},
		UpgradeThreshold: config.BuilderRuntimeUpgradeThresholdConfig{
			UpgradeOnSemanticConflict: true,
		},
		TaskRoutes: []config.BuilderRuntimeTaskRouteConfig{{
			TaskType: "dual_file_wiring",
			Model:    &config.AgentModelConfig{Primary: "qwen2.5-coder-14b-local"},
		}},
	}
	runner.PatchGenerator = &stubBuilderRuntimePatchGenerator{responses: map[string][]adapter.BuilderRuntimePatchResponse{
		"qwen2.5-coder-14b-local": {
			{Content: `{"patch_id":"initial-edit","operations":[{"type":"write_file","path":"lib/main.dart","content":"const title = 'Open Lite Seed';\n"}]}`},
			{Content: `{"patch_id":"semantic-repair-local","operations":[{"type":"write_file","path":"lib/main.dart","content":"const title = 'Generic Tracker';\n"}]}`},
		},
		"qwen2.5-coder-32b-local": {
			{Content: `{"patch_id":"semantic-repair-upgrade","operations":[{"type":"write_file","path":"lib/main.dart","content":"const title = 'Weight Tracker';\n"}]}`},
		},
	}}
	runner.Executor = failingValidationExecutor{
		checkID:  "check-profile-open-lite-domain-branding",
		label:    "domain branding",
		stage:    appruns.StageCheap,
		commands: []string{"grep Weight Tracker lib/main.dart"},
		script:   "grep -q 'Weight Tracker' lib/main.dart || { echo semantic conflict remains >&2; exit 1; }",
	}

	if err := runner.ExecuteRun(context.Background(), runID); err != nil {
		t.Fatalf("ExecuteRun() error = %v", err)
	}

	run := getJSON(t, server.Client(), server.URL+"/internal/v1/build-runs/"+runID)
	if run["status"] != "completed" {
		t.Fatalf("run status = %v, want completed", run["status"])
	}
	builderOutput := readBuilderOutput(t, run)
	roundOutputs, _ := builderOutput["round_outputs"].([]any)
	firstRound, _ := roundOutputs[0].(map[string]any)
	builderRuntime, ok := firstRound["builder_runtime_execution"].(map[string]any)
	if !ok {
		t.Fatalf("builder_runtime_execution = %T, want object", firstRound["builder_runtime_execution"])
	}
	if builderRuntime["selected_model"] != "qwen2.5-coder-32b-local" {
		t.Fatalf("selected_model = %v, want qwen2.5-coder-32b-local", builderRuntime["selected_model"])
	}
	if builderRuntime["upgrade_applied"] != true {
		t.Fatalf("upgrade_applied = %v, want true", builderRuntime["upgrade_applied"])
	}
	if builderRuntime["task_type"] != "dual_file_wiring" {
		t.Fatalf("task_type = %v, want dual_file_wiring", builderRuntime["task_type"])
	}
	if builderRuntime["attempts"] != float64(3) {
		t.Fatalf("attempts = %v, want 3", builderRuntime["attempts"])
	}
}

func TestRunnerExecuteRunPassesGenericSemanticChecksFromCompileBundle(t *testing.T) {
	t.Setenv("ONEAPPFACTORY_BUILDER_RUNTIME", "")
	t.Setenv("ONEAPPFACTORY_BUILDER_DOCKER_NETWORK", "")
	t.Setenv("HTTP_PROXY", "")
	t.Setenv("HTTPS_PROXY", "")
	t.Setenv("ALL_PROXY", "")
	t.Setenv("NO_PROXY", "")

	configPath := filepath.Join(t.TempDir(), "config.json")
	cfg := config.DefaultConfig()
	cfg.Workspace = filepath.Join(filepath.Dir(configPath), "workspace")
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	h := api.NewHandler(configPath)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	server := httptest.NewServer(mux)
	defer server.Close()

	requirementPath := filepath.Join("..", "..", "..", "examples", "appfactory", "generic", "weight-tracker", "requirement.md")
	requirementText, err := os.ReadFile(requirementPath)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", requirementPath, err)
	}
	compileResp := postJSON(t, server.Client(), server.URL+"/api/v1/prds:compile", map[string]any{
		"requirement_text": string(requirementText),
	})
	bundleDir, _ := compileResp["bundle_dir"].(string)
	if strings.TrimSpace(bundleDir) == "" {
		t.Fatal("bundle_dir should not be empty")
	}
	builderInputData, err := os.ReadFile(filepath.Join(bundleDir, "builder-input.json"))
	if err != nil {
		t.Fatalf("ReadFile(builder-input.json) error = %v", err)
	}
	var builderInput map[string]any
	if err := json.Unmarshal(builderInputData, &builderInput); err != nil {
		t.Fatalf("Unmarshal(builder-input.json) error = %v", err)
	}

	postJSON(t, server.Client(), server.URL+"/internal/v1/builders:register", map[string]any{
		"builder_id":        "builder-a",
		"display_name":      "Builder A",
		"capability_tags":   []string{"flutter"},
		"max_parallel_runs": 1,
		"worker_profile":    map[string]any{"image": "builder:latest"},
	})
	allocate := postJSON(t, server.Client(), server.URL+"/internal/v1/workers:allocate", map[string]any{"job_id": "job-generic-open-lite"})
	leaseID, _ := allocate["lease_id"].(string)
	create := postJSON(t, server.Client(), server.URL+"/internal/v1/build-runs", map[string]any{
		"worker_id":     "builder-a",
		"lease_id":      leaseID,
		"builder_input": builderInput,
	})
	runID, _ := create["run_id"].(string)
	if strings.TrimSpace(runID) == "" {
		t.Fatal("run_id should not be empty")
	}
	runBefore := getJSON(t, server.Client(), server.URL+"/internal/v1/build-runs/"+runID)
	workspacePath, _ := runBefore["workspace_path"].(string)
	if strings.TrimSpace(workspacePath) == "" {
		t.Fatalf("workspace_path = %v, want non-empty", runBefore["workspace_path"])
	}
	copyCompileBundleToRunPrepareDir(t, bundleDir, workspacePath)
	seedGenericSemanticWorkspace(t, workspacePath)

	runner := adapter.NewRunner(server.URL)
	runner.Executor = semanticSuccessExecutor{}
	if err := runner.ExecuteRun(context.Background(), runID); err != nil {
		t.Fatalf("ExecuteRun() error = %v", err)
	}

	run := getJSON(t, server.Client(), server.URL+"/internal/v1/build-runs/"+runID)
	if run["status"] != "completed" {
		t.Fatalf("run status = %v, want completed", run["status"])
	}
	builderOutput := readBuilderOutput(t, run)
	checksPassed, ok := builderOutput["checks_passed"].([]any)
	if !ok {
		t.Fatalf("checks_passed = %T, want []any", builderOutput["checks_passed"])
	}
	for _, checkID := range []string{"ac-overview", "ac-list", "ac-form", "ac-persistence"} {
		result := findValidationResult(t, checksPassed, checkID)
		if result["outcome"] != "passed" {
			t.Fatalf("check result %s outcome = %v, want passed", checkID, result["outcome"])
		}
	}
	nextHumanActions, ok := builderOutput["next_human_actions"].([]any)
	if !ok {
		t.Fatalf("next_human_actions = %T, want []any", builderOutput["next_human_actions"])
	}
	semanticReview := findHumanAction(t, nextHumanActions, "review-semantic-acceptance")
	requiredInputs, ok := semanticReview["required_inputs"].([]any)
	if !ok || len(requiredInputs) == 0 {
		t.Fatalf("required_inputs = %v, want semantic review inputs", semanticReview["required_inputs"])
	}
	requiredInputStrings := anySliceToStrings(requiredInputs)
	for _, want := range []string{"reports/build-report.md", "reports/smoke-test-report.md"} {
		if !containsString(requiredInputStrings, want) {
			t.Fatalf("required_inputs = %v, want include %q", requiredInputStrings, want)
		}
	}

	jobRoot := filepath.Dir(filepath.FromSlash(workspacePath))
	buildReportPath := filepath.Join(jobRoot, "reports", "build-report.md")
	buildReport, err := os.ReadFile(buildReportPath)
	if err != nil {
		t.Fatalf("ReadFile(build-report.md) error = %v", err)
	}
	for _, want := range []string{"## 语义验收", "通过：体重摘要完整", "通过：历史记录集合承载单元可浏览"} {
		if !strings.Contains(string(buildReport), want) {
			t.Fatalf("build report missing %q: %s", want, string(buildReport))
		}
	}
}

func TestRunnerExecuteRunClassifiesDeviceVerificationCheckFailure(t *testing.T) {
	t.Setenv("ONEAPPFACTORY_BUILDER_RUNTIME", "")
	t.Setenv("ONEAPPFACTORY_BUILDER_DOCKER_NETWORK", "")
	t.Setenv("HTTP_PROXY", "")
	t.Setenv("HTTPS_PROXY", "")
	t.Setenv("ALL_PROXY", "")
	t.Setenv("NO_PROXY", "")

	configPath := filepath.Join(t.TempDir(), "config.json")
	cfg := config.DefaultConfig()
	cfg.Workspace = filepath.Join(filepath.Dir(configPath), "workspace")
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
		"builder_input": sampleBuildRunInputWithAcceptanceCheck("check-adb-device-ready", "adb device ready", "device", []string{"adb wait-for-device"}),
	})
	runID, _ := create["run_id"].(string)

	runner := adapter.NewRunner(server.URL)
	runner.Executor = failingValidationExecutor{
		checkID:  "check-adb-device-ready",
		label:    "adb device ready",
		stage:    appruns.StageDevice,
		commands: []string{"adb wait-for-device"},
		script:   "echo adb device missing >&2; exit 1",
	}
	if err := runner.ExecuteRun(context.Background(), runID); err != nil {
		t.Fatalf("ExecuteRun() error = %v", err)
	}

	run := getJSON(t, server.Client(), server.URL+"/internal/v1/build-runs/"+runID)
	if run["status"] != "failed" {
		t.Fatalf("run status = %v, want failed", run["status"])
	}
	if run["failure_summary"] != "device verification check check-adb-device-ready failed: exit status 1" {
		t.Fatalf("failure_summary = %v, want device verification summary", run["failure_summary"])
	}
	failureSignatures, ok := run["failure_signatures"].([]any)
	if !ok || len(failureSignatures) != 1 || failureSignatures[0] != "device_check_failed:adb_device_unavailable" {
		t.Fatalf("failure_signatures = %#v, want [device_check_failed:adb_device_unavailable]", run["failure_signatures"])
	}
	repairContext, ok := run["repair_context"].(map[string]any)
	if !ok {
		t.Fatalf("repair_context = %T, want object", run["repair_context"])
	}
	if repairContext["recommended_action"] != "connect the target device or fix adb connectivity before rerun" {
		t.Fatalf("repair_context.recommended_action = %v, want device guidance", repairContext["recommended_action"])
	}
	if repairContext["requires_human_review"] != true {
		t.Fatalf("repair_context.requires_human_review = %v, want true", repairContext["requires_human_review"])
	}
	if roundState, ok := run["round_state"].(map[string]any); !ok || roundState["next_action"] != "stop" {
		t.Fatalf("round_state = %#v, want next_action=stop", run["round_state"])
	}
	metricsData, err := os.ReadFile(filepath.Join(filepath.Dir(configPath), "workspace", "appfactory", "jobs", "job-1", "runs", runID, "metrics.json"))
	if err != nil {
		t.Fatalf("ReadFile(metrics) error = %v", err)
	}
	var metrics map[string]any
	if err := json.Unmarshal(metricsData, &metrics); err != nil {
		t.Fatalf("Unmarshal(metrics) error = %v", err)
	}
	deviceFailureCategories, ok := metrics["device_failure_categories"].([]any)
	if !ok || len(deviceFailureCategories) != 1 {
		t.Fatalf("device_failure_categories = %#v, want one device stat", metrics["device_failure_categories"])
	}
	deviceFailureCategory, ok := deviceFailureCategories[0].(map[string]any)
	if !ok {
		t.Fatalf("device_failure_categories[0] = %T, want object", deviceFailureCategories[0])
	}
	if deviceFailureCategory["category"] != "device_check_failed:adb_device_unavailable" {
		t.Fatalf("device_failure_categories[0].category = %v, want device_check_failed:adb_device_unavailable", deviceFailureCategory["category"])
	}
	if deviceFailureCategory["failure_domain"] != "device" {
		t.Fatalf("device_failure_categories[0].failure_domain = %v, want device", deviceFailureCategory["failure_domain"])
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

func readRunEvents(t *testing.T, run map[string]any) []map[string]any {
	t.Helper()
	workspacePath, _ := run["workspace_path"].(string)
	eventsPath, _ := run["events_path"].(string)
	if strings.TrimSpace(workspacePath) == "" || strings.TrimSpace(eventsPath) == "" {
		t.Fatalf("workspace_path=%q events_path=%q, want non-empty values", workspacePath, eventsPath)
	}
	appFactoryRoot := filepath.Clean(filepath.Join(filepath.FromSlash(workspacePath), "..", "..", ".."))
	eventsData, err := os.ReadFile(filepath.Join(appFactoryRoot, filepath.FromSlash(eventsPath)))
	if err != nil {
		t.Fatalf("ReadFile(events) error = %v", err)
	}
	events := make([]map[string]any, 0)
	for _, line := range bytes.Split(eventsData, []byte{'\n'}) {
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}
		var event map[string]any
		if err := json.Unmarshal(line, &event); err != nil {
			t.Fatalf("Unmarshal(event) error = %v, line=%q", err, string(line))
		}
		events = append(events, event)
	}
	return events
}

func findValidationResult(t *testing.T, results []any, checkID string) map[string]any {
	t.Helper()
	for _, item := range results {
		result, ok := item.(map[string]any)
		if ok && result["check_id"] == checkID {
			return result
		}
	}
	t.Fatalf("validation result %q not found", checkID)
	return nil
}

func findHumanAction(t *testing.T, actions []any, actionID string) map[string]any {
	t.Helper()
	for _, item := range actions {
		action, ok := item.(map[string]any)
		if ok && action["action_id"] == actionID {
			return action
		}
	}
	t.Fatalf("human action %q not found", actionID)
	return nil
}

func anySliceToStrings(items []any) []string {
	values := make([]string, 0, len(items))
	for _, item := range items {
		if value, ok := item.(string); ok {
			values = append(values, value)
		}
	}
	return values
}

func sampleBuildRunInput() map[string]any {
	return map[string]any{
		"schema_version":  "0.1.0",
		"job_id":          "job-1",
		"prd_id":          "prd-1",
		"template_id":     "template-1",
		"planning_policy": samplePlanningPolicyMap(),
		"workspace_path":  "/workspace/job-1",
		"artifact_dir":    "/artifacts/job-1",
		"goal_summary":    "build android app",
		"task_bundle": []map[string]any{{
			"task_id":             "task-1",
			"title":               "Implement UI",
			"category":            "screen",
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

func samplePlanningPolicyMap() map[string]any {
	return map[string]any{
		"policy_version": "phase1-boundary-v1",
		"stages": []map[string]any{
			{"stage": "requirement_structuring", "route": "planning_model"},
			{"stage": "domain_modeling", "route": "planning_model"},
			{"stage": "task_allocation", "route": "decision_model"},
			{"stage": "acceptance_planning", "route": "decision_model"},
			{"stage": "build_input_projection", "route": "deterministic"},
		},
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

func sampleFlutterFallbackBuildRunInput() map[string]any {
	return map[string]any{
		"schema_version":  "0.1.0",
		"job_id":          "job-1",
		"prd_id":          "prd-1",
		"template_id":     "flutter-finance-lite",
		"planning_policy": samplePlanningPolicyMap(),
		"workspace_path":  "/workspace/job-1",
		"artifact_dir":    "/artifacts/job-1",
		"goal_summary":    "deliver bookkeeping shell",
		"task_bundle": []map[string]any{
			{
				"task_id":             "screen-main",
				"title":               "Write fallback probe",
				"category":            "screen",
				"objective":           "write a Flutter workspace fallback probe for OneAppFactory thin executor verification",
				"target_paths":        []string{"lib/main.dart", "lib/views/home_page.dart"},
				"completion_criteria": []string{"fallback probe exists"},
			},
			{
				"task_id":             "flow-entry",
				"title":               "Preserve template boundary",
				"category":            "flow",
				"objective":           "preserve seed workspace for OneAppFactory thin fallback verification",
				"target_paths":        []string{"lib/views/entry_form_page.dart", "lib/repositories/entry_repository.dart"},
				"completion_criteria": []string{"seed workspace remains intact"},
			},
		},
		"acceptance_checks": []map[string]any{
			{
				"check_id": "check-structural-template-files-ready",
				"label":    "template files ready",
				"stage":    "cheap",
				"required": true,
				"commands": []string{"grep -q . pubspec.yaml lib/main.dart test/widget_test.dart"},
			},
			{
				"check_id": "check-oneappfactory-thin-fallback-probe",
				"label":    "OneAppFactory thin fallback probe",
				"stage":    "cheap",
				"required": true,
				"commands": []string{"grep -E \"Generated by OneAppFactory thin executor.|'goalSummary':\" lib/oneappfactory_executor_probe.dart >/dev/null 2>&1"},
			},
			{
				"check_id": "check-oneappfactory-thin-fallback-metadata",
				"label":    "OneAppFactory thin fallback metadata",
				"stage":    "cheap",
				"required": true,
				"commands": []string{"grep -E \"'taskCount': [0-9]+,|'acceptanceCheckCount': [0-9]+,\" lib/oneappfactory_executor_probe.dart >/dev/null 2>&1"},
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

type semanticSuccessExecutor struct{}

func (semanticSuccessExecutor) Prepare(ctx context.Context, run adapter.RunRecord) (adapter.RoundPlan, error) {
	cmd := exec.CommandContext(ctx, "/bin/sh", "-lc", "mkdir -p lib && cat <<'EOF' > lib/oneappfactory_executor_probe.dart\n// Generated by OneAppFactory thin executor.\nconst probe = {\n  'goalSummary': 'semantic regression',\n  'taskCount': 7,\n  'acceptanceCheckCount': 16,\n};\nEOF")
	cmd.Dir = run.WorkspacePath
	cmd.Env = append(os.Environ(), "ONEAPPFACTORY_STUB_EXECUTOR=semantic-success")
	validationSteps := make([]adapter.ExecutionStep, 0, len(run.AcceptanceChecks))
	acceptanceChecks := make([]adapter.CheckExecutionPreview, 0, len(run.AcceptanceChecks))
	for _, check := range run.AcceptanceChecks {
		preview := adapter.CheckExecutionPreview{
			CheckID:      check.CheckID,
			Label:        check.Label,
			Stage:        check.Stage,
			Required:     check.Required,
			AllowFailure: check.AllowFailure,
			Commands:     append([]string(nil), check.Commands...),
		}
		acceptanceChecks = append(acceptanceChecks, preview)
		if len(check.Commands) == 0 {
			continue
		}
		validationCmd := exec.CommandContext(ctx, "/bin/sh", "-lc", strings.Join(check.Commands, "\n"))
		validationCmd.Dir = run.WorkspacePath
		validationCmd.Env = append(os.Environ(),
			"ONEAPPFACTORY_STUB_EXECUTOR=semantic-success",
			"ONEAPPFACTORY_EXECUTION_STEP="+check.CheckID,
			"ONEAPPFACTORY_EXECUTION_STAGE="+string(check.Stage),
			"ONEAPPFACTORY_ACCEPTANCE_CHECK_ID="+check.CheckID,
		)
		validationSteps = append(validationSteps, adapter.ExecutionStep{
			StepID:  check.CheckID,
			Stage:   check.Stage,
			Summary: check.Label,
			Command: validationCmd,
			Check:   &preview,
		})
	}
	return adapter.RoundPlan{
		Summary:          "semantic success executor",
		RoundInput:       adapter.BuildRoundInputForTest(run),
		ValidationSteps:  validationSteps,
		AcceptanceChecks: acceptanceChecks,
		EditStep: adapter.ExecutionStep{
			StepID:  "thin-prepare",
			Stage:   appruns.StageThinPrepare,
			Summary: "write semantic regression probe",
			Command: cmd,
		},
	}, nil
}

func seedGenericSemanticWorkspace(t *testing.T, workspacePath string) {
	t.Helper()
	files := map[string]string{
		"pubspec.yaml":          "name: weight_tracker\n",
		"lib/main.dart":         "const appTitle = '体重记录 App';\nconst summaryKeys = ['latest_weight', 'record_count', 'trend'];\nconst recordKeys = ['record_id', 'weight', 'recorded_at', 'note'];\n",
		"test/widget_test.dart": "void main() {}\n",
		"android/app/src/main/res/values/strings.xml": "<resources><string name=\"app_name\">体重记录 App</string></resources>\n",
	}
	for relativePath, content := range files {
		path := filepath.Join(workspacePath, filepath.FromSlash(relativePath))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("MkdirAll(%s) error = %v", path, err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatalf("WriteFile(%s) error = %v", path, err)
		}
	}
}

func copyCompileBundleToRunPrepareDir(t *testing.T, bundleDir, workspacePath string) {
	t.Helper()
	prepareDir := filepath.Join(filepath.Dir(filepath.FromSlash(workspacePath)), "prepare")
	entries, err := os.ReadDir(bundleDir)
	if err != nil {
		t.Fatalf("ReadDir(%s) error = %v", bundleDir, err)
	}
	if err := os.MkdirAll(prepareDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%s) error = %v", prepareDir, err)
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		sourcePath := filepath.Join(bundleDir, entry.Name())
		targetPath := filepath.Join(prepareDir, entry.Name())
		data, err := os.ReadFile(sourcePath)
		if err != nil {
			t.Fatalf("ReadFile(%s) error = %v", sourcePath, err)
		}
		if err := os.WriteFile(targetPath, data, 0o600); err != nil {
			t.Fatalf("WriteFile(%s) error = %v", targetPath, err)
		}
	}
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
	cmd.Env = append(os.Environ(), "ONEAPPFACTORY_STUB_EXECUTOR=1")
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
	cmd.Env = append(os.Environ(), "ONEAPPFACTORY_STUB_EXECUTOR=patch-write")
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
	cmd.Env = append(os.Environ(), "ONEAPPFACTORY_STUB_EXECUTOR=patch-protected-path")
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
	cmd.Env = append(os.Environ(), "ONEAPPFACTORY_STUB_EXECUTOR=failure-log")
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

type stubBuilderRuntimePatchGenerator struct {
	responses map[string][]adapter.BuilderRuntimePatchResponse
	models    []string
	requests  []adapter.BuilderRuntimePatchRequest
}

func (generator *stubBuilderRuntimePatchGenerator) GeneratePatch(ctx context.Context, request adapter.BuilderRuntimePatchRequest) (adapter.BuilderRuntimePatchResponse, error) {
	generator.requests = append(generator.requests, request)
	for _, alias := range request.ModelAliases {
		generator.models = append(generator.models, alias)
		responses := generator.responses[alias]
		if len(responses) == 0 {
			continue
		}
		response := responses[0]
		generator.responses[alias] = responses[1:]
		response.ModelAlias = alias
		return response, nil
	}
	return adapter.BuilderRuntimePatchResponse{}, fmt.Errorf("no stub response for aliases %v", request.ModelAliases)
}

func (executor failingValidationExecutor) Prepare(ctx context.Context, run adapter.RunRecord) (adapter.RoundPlan, error) {
	editCmd := exec.CommandContext(ctx, "/bin/sh", "-lc", "true")
	editCmd.Dir = run.WorkspacePath
	editCmd.Env = append(os.Environ(), "ONEAPPFACTORY_STUB_EXECUTOR=validation-edit")
	validationCmd := exec.CommandContext(ctx, "/bin/sh", "-lc", executor.script)
	validationCmd.Dir = run.WorkspacePath
	validationCmd.Env = append(os.Environ(), "ONEAPPFACTORY_STUB_EXECUTOR=validation-fail")
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

func mustFindTaskBundleItem(t *testing.T, tasks []appruns.TaskBundleItem, taskID string) appruns.TaskBundleItem {
	t.Helper()
	for _, task := range tasks {
		if strings.TrimSpace(task.TaskID) == strings.TrimSpace(taskID) {
			return task
		}
	}
	t.Fatalf("task %q not found in task bundle %#v", taskID, tasks)
	return appruns.TaskBundleItem{}
}

func hasTaskBundleItem(tasks []appruns.TaskBundleItem, taskID string) bool {
	for _, task := range tasks {
		if strings.TrimSpace(task.TaskID) == strings.TrimSpace(taskID) {
			return true
		}
	}
	return false
}

func assertExactTargetPaths(t *testing.T, actual, expected []string) {
	t.Helper()
	if len(actual) != len(expected) {
		t.Fatalf("target paths = %v, want %v", actual, expected)
	}
	for index := range expected {
		if actual[index] != expected[index] {
			t.Fatalf("target paths = %v, want %v", actual, expected)
		}
	}
}

func summarizePatchRequests(requests []adapter.BuilderRuntimePatchRequest) string {
	parts := make([]string, 0, len(requests))
	for index, request := range requests {
		routeTask := mustFindTaskBundleItemByFallback(request.Run.TaskBundle, request.Route.TaskID)
		parts = append(parts, fmt.Sprintf("%d:%s:%v:repair=%t:prev=%q", index+1, request.Route.TaskType, routeTask.TargetPaths, request.RepairOnly, request.PreviousErr))
	}
	return strings.Join(parts, " | ")
}

func mustFindTaskBundleItemByFallback(tasks []appruns.TaskBundleItem, taskID string) appruns.TaskBundleItem {
	for _, task := range tasks {
		if strings.TrimSpace(task.TaskID) == strings.TrimSpace(taskID) {
			return task
		}
	}
	return appruns.TaskBundleItem{TaskID: taskID}
}
