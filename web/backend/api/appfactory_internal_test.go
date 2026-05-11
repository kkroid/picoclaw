package api

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
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	appadapter "github.com/sipeed/oneappfactory/pkg/appfactory/adapter"
	appbuilders "github.com/sipeed/oneappfactory/pkg/appfactory/builders"
	appprepare "github.com/sipeed/oneappfactory/pkg/appfactory/prepare"
	appruns "github.com/sipeed/oneappfactory/pkg/appfactory/runs"
	"github.com/sipeed/oneappfactory/pkg/config"
)

func TestInternalBuildersRegisterHeartbeatAndAllocateRelease(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	cfg := config.DefaultConfig()
	cfg.Workspace = filepath.Join(filepath.Dir(configPath), "workspace")
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	h := NewHandler(configPath)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	postJSON(t, mux, "/internal/v1/builders:register", map[string]any{
		"builder_id":        "builder-a",
		"display_name":      "Builder A",
		"capability_tags":   []string{"flutter", "android"},
		"max_parallel_runs": 1,
		"worker_profile": map[string]any{
			"image": "builder:latest",
		},
	}, http.StatusOK)

	hbResp := postJSON(t, mux, "/internal/v1/builders/builder-a/heartbeat", map[string]any{
		"status": "idle",
	}, http.StatusOK)
	if hbResp["builder_id"] != "builder-a" {
		t.Fatalf("heartbeat builder_id = %v, want builder-a", hbResp["builder_id"])
	}

	allocateResp := postJSON(t, mux, "/internal/v1/workers:allocate", map[string]any{
		"job_id":                   "job-1",
		"required_capability_tags": []string{"flutter"},
	}, http.StatusOK)
	if allocateResp["worker_id"] != "builder-a" {
		t.Fatalf("worker_id = %v, want builder-a", allocateResp["worker_id"])
	}

	postJSON(t, mux, "/internal/v1/workers/builder-a/release", map[string]any{
		"reason": "done",
	}, http.StatusOK)

	allocateResp2 := postJSON(t, mux, "/internal/v1/workers:allocate", map[string]any{
		"job_id":                   "job-2",
		"required_capability_tags": []string{"flutter"},
	}, http.StatusOK)
	if allocateResp2["worker_id"] != "builder-a" {
		t.Fatalf("worker_id after release = %v, want builder-a", allocateResp2["worker_id"])
	}
}

func TestNewAppFactoryRunnerLoadsBuilderRuntimeConfig(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	cfg := config.DefaultConfig()
	cfg.Workspace = filepath.Join(filepath.Dir(configPath), "workspace")
	cfg.AppFactory.BuilderRuntime = config.BuilderRuntimeConfig{
		Enabled:      true,
		DefaultModel: &config.AgentModelConfig{Primary: "qwen2.5-coder-14b-local"},
		UpgradeModel: &config.AgentModelConfig{Primary: "qwen2.5-coder-32b-local"},
		TaskRoutes: []config.BuilderRuntimeTaskRouteConfig{{
			TaskType: "closure_repair",
			Model:    &config.AgentModelConfig{Primary: "qwen2.5-coder-32b-local"},
		}},
	}
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	h := NewHandler(configPath)
	var runsSvc runService
	runner := h.newAppFactoryRunner(runsSvc)
	if runner.BuilderRuntimeConfig.Enabled != true {
		t.Fatal("runner should load enabled builder_runtime config")
	}
	if runner.BuilderRuntimeConfig.DefaultModel == nil || runner.BuilderRuntimeConfig.DefaultModel.Primary != "qwen2.5-coder-14b-local" {
		t.Fatalf("default model = %#v, want qwen2.5-coder-14b-local", runner.BuilderRuntimeConfig.DefaultModel)
	}
	if runner.BuilderRuntimeConfig.UpgradeModel == nil || runner.BuilderRuntimeConfig.UpgradeModel.Primary != "qwen2.5-coder-32b-local" {
		t.Fatalf("upgrade model = %#v, want qwen2.5-coder-32b-local", runner.BuilderRuntimeConfig.UpgradeModel)
	}
	if len(runner.BuilderRuntimeConfig.TaskRoutes) != 1 || runner.BuilderRuntimeConfig.TaskRoutes[0].TaskType != "closure_repair" {
		t.Fatalf("task routes = %#v, want closure_repair route", runner.BuilderRuntimeConfig.TaskRoutes)
	}
}

func TestRunServiceRunnerBackendResolvesExecutionPaths(t *testing.T) {
	workspaceRoot := filepath.Join(t.TempDir(), "workspace")
	backend := runServiceRunnerBackend{
		runsSvc: stubRunService{
			run: appruns.RunRecord{
				RunID:         "run-1",
				JobID:         "job-1",
				WorkspacePath: filepath.ToSlash(filepath.Join("jobs", "job-1", "workspace")),
				ArtifactDir:   filepath.ToSlash(filepath.Join("jobs", "job-1", "artifacts")),
				LogPath:       filepath.ToSlash(filepath.Join("jobs", "job-1", "logs", "builder.log")),
			},
		},
		workspaceRoot: workspaceRoot,
	}

	run, err := backend.GetRun(context.Background(), "run-1")
	if err != nil {
		t.Fatalf("GetRun() error = %v", err)
	}
	if run.WorkspacePath != filepath.Join(workspaceRoot, "appfactory", "jobs", "job-1", "workspace") {
		t.Fatalf("WorkspacePath = %q", run.WorkspacePath)
	}
	if run.ArtifactDir != filepath.Join(workspaceRoot, "appfactory", "jobs", "job-1", "artifacts") {
		t.Fatalf("ArtifactDir = %q", run.ArtifactDir)
	}
	if run.LogPath != filepath.Join(workspaceRoot, "appfactory", "jobs", "job-1", "logs", "builder.log") {
		t.Fatalf("LogPath = %q", run.LogPath)
	}
	if run.RunID != "run-1" {
		t.Fatalf("RunID = %q, want run-1", run.RunID)
	}
	if run.JobID != "job-1" {
		t.Fatalf("JobID = %q, want job-1", run.JobID)
	}
	absolute := normalizeRunRecordForExecution(appruns.RunRecord{WorkspacePath: "/tmp/workspace"}, workspaceRoot)
	if absolute.WorkspacePath != "/tmp/workspace" {
		t.Fatalf("absolute workspace path should be preserved, got %q", absolute.WorkspacePath)
	}
	blank := normalizeRunRecordForExecution(appruns.RunRecord{WorkspacePath: "jobs/job-2/workspace"}, "")
	if blank.WorkspacePath != "jobs/job-2/workspace" {
		t.Fatalf("blank workspace root should preserve original path, got %q", blank.WorkspacePath)
	}
}

type stubRunService struct {
	run appruns.RunRecord
}

func (stub stubRunService) Create(context.Context, string, string, string, appruns.BuildInput) (appruns.RunRecord, error) {
	return appruns.RunRecord{}, fmt.Errorf("not implemented")
}

func (stub stubRunService) Get(context.Context, string) (appruns.RunRecord, error) {
	return stub.run, nil
}

func (stub stubRunService) Heartbeat(context.Context, string, appruns.Heartbeat) (appruns.RunRecord, error) {
	return appruns.RunRecord{}, fmt.Errorf("not implemented")
}

func (stub stubRunService) Complete(context.Context, string, appruns.BuildOutput) (appruns.RunRecord, error) {
	return appruns.RunRecord{}, fmt.Errorf("not implemented")
}

func (stub stubRunService) Fail(context.Context, string, appruns.FailureReport) (appruns.RunRecord, error) {
	return appruns.RunRecord{}, fmt.Errorf("not implemented")
}

func (stub stubRunService) Cancel(context.Context, string, string) (appruns.RunRecord, error) {
	return appruns.RunRecord{}, fmt.Errorf("not implemented")
}

func (stub stubRunService) IndexArtifacts(context.Context, string, appruns.ArtifactManifest) (appruns.RunRecord, error) {
	return appruns.RunRecord{}, fmt.Errorf("not implemented")
}

func (stub stubRunService) IndexMetrics(context.Context, string, appruns.Metrics) (appruns.RunRecord, error) {
	return appruns.RunRecord{}, fmt.Errorf("not implemented")
}

func TestInternalPreserveWorkerDrainsBuilder(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	cfg := config.DefaultConfig()
	cfg.Workspace = filepath.Join(filepath.Dir(configPath), "workspace")
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	h := NewHandler(configPath)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	postJSON(t, mux, "/internal/v1/builders:register", map[string]any{
		"builder_id":   "builder-a",
		"display_name": "Builder A",
		"worker_profile": map[string]any{
			"image": "builder:latest",
		},
	}, http.StatusOK)

	postJSON(t, mux, "/internal/v1/workers/builder-a/preserve", map[string]any{
		"reason": "debug preserve",
	}, http.StatusOK)

	hbResp := postJSON(t, mux, "/internal/v1/builders/builder-a/heartbeat", map[string]any{
		"status": "idle",
	}, http.StatusOK)
	if hbResp["status"] != "draining" {
		t.Fatalf("status after preserve = %v, want draining", hbResp["status"])
	}

	req := httptest.NewRequest(http.MethodPost, "/internal/v1/workers:allocate", bytes.NewBufferString(`{"job_id":"job-1"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusConflict {
		t.Fatalf("allocate status = %d, want %d, body=%s", rec.Code, http.StatusConflict, rec.Body.String())
	}
}

func TestInternalAllocateWorkerPrefersHigherPriorityAndPreferredCapabilities(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	cfg := config.DefaultConfig()
	cfg.Workspace = filepath.Join(filepath.Dir(configPath), "workspace")
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	h := NewHandler(configPath)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	postJSON(t, mux, "/internal/v1/builders:register", map[string]any{
		"builder_id":      "builder-a",
		"display_name":    "Builder A",
		"capability_tags": []string{"flutter", "android"},
		"priority":        10,
		"worker_profile": map[string]any{
			"image": "builder-a:latest",
		},
	}, http.StatusOK)
	postJSON(t, mux, "/internal/v1/builders:register", map[string]any{
		"builder_id":      "builder-b",
		"display_name":    "Builder B",
		"capability_tags": []string{"flutter", "android", "dart"},
		"priority":        50,
		"worker_profile": map[string]any{
			"image": "builder-b:latest",
		},
	}, http.StatusOK)

	allocateResp := postJSON(t, mux, "/internal/v1/workers:allocate", map[string]any{
		"job_id":                    "job-priority",
		"required_capability_tags":  []string{"flutter"},
		"preferred_capability_tags": []string{"dart"},
	}, http.StatusOK)
	if allocateResp["worker_id"] != "builder-b" {
		t.Fatalf("worker_id = %v, want builder-b", allocateResp["worker_id"])
	}
	decision, ok := allocateResp["decision"].(map[string]any)
	if !ok {
		t.Fatalf("decision = %T, want map[string]any", allocateResp["decision"])
	}
	if decision["selected_builder_id"] != "builder-b" {
		t.Fatalf("selected_builder_id = %v, want builder-b", decision["selected_builder_id"])
	}
	candidates, ok := decision["candidate_builder_ids"].([]any)
	if !ok || len(candidates) != 2 || candidates[0] != "builder-b" {
		t.Fatalf("candidate_builder_ids = %v, want [builder-b builder-a]", decision["candidate_builder_ids"])
	}
	if decision["fallback_count"] != float64(1) {
		t.Fatalf("fallback_count = %v, want 1", decision["fallback_count"])
	}
	if !strings.Contains(fmt.Sprint(decision["decision_reason"]), "preferred_capability_matches=1") {
		t.Fatalf("decision_reason = %v, want preferred capability summary", decision["decision_reason"])
	}
}

func TestInternalAllocateWorkerRejectsLowBudgetFlagshipOnlyPool(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	cfg := config.DefaultConfig()
	cfg.Workspace = filepath.Join(filepath.Dir(configPath), "workspace")
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	h := NewHandler(configPath)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	postJSON(t, mux, "/internal/v1/builders:register", map[string]any{
		"builder_id":      "builder-flagship",
		"display_name":    "Builder Flagship",
		"capability_tags": []string{"flutter"},
		"model_tags":      []string{"flagship"},
		"priority":        100,
		"worker_profile": map[string]any{
			"image": "flagship:latest",
		},
	}, http.StatusOK)

	req := httptest.NewRequest(http.MethodPost, "/internal/v1/workers:allocate", bytes.NewBufferString(`{"job_id":"job-low-budget","required_capability_tags":["flutter"],"budget_class":"low"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusConflict {
		t.Fatalf("allocate status = %d, want %d, body=%s", rec.Code, http.StatusConflict, rec.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("Unmarshal(error response) error = %v", err)
	}
	if body["error_code"] != "WORKER_NO_CANDIDATE" {
		t.Fatalf("error_code = %v, want WORKER_NO_CANDIDATE", body["error_code"])
	}
}

func TestInternalBuildRunsLifecycle(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	cfg := config.DefaultConfig()
	cfg.Workspace = filepath.Join(filepath.Dir(configPath), "workspace")
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	h := NewHandler(configPath)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	postJSON(t, mux, "/internal/v1/builders:register", map[string]any{
		"builder_id":        "builder-a",
		"display_name":      "Builder A",
		"capability_tags":   []string{"flutter"},
		"max_parallel_runs": 1,
		"worker_profile": map[string]any{
			"image": "builder:latest",
		},
	}, http.StatusOK)

	allocateResp := postJSON(t, mux, "/internal/v1/workers:allocate", map[string]any{
		"job_id": "job-1",
	}, http.StatusOK)
	leaseID, _ := allocateResp["lease_id"].(string)

	createResp := postJSON(t, mux, "/internal/v1/build-runs", map[string]any{
		"worker_id":     "builder-a",
		"lease_id":      leaseID,
		"builder_input": sampleBuildRunInput(),
	}, http.StatusOK)
	runID, _ := createResp["run_id"].(string)
	if runID == "" {
		t.Fatal("run_id should not be empty")
	}
	if createResp["launch_command"] != "/bin/sh" {
		t.Fatalf("launch_command = %v, want /bin/sh", createResp["launch_command"])
	}
	if createResp["runner_script_path"] == "" {
		t.Fatal("runner_script_path should not be empty")
	}

	postJSON(t, mux, "/internal/v1/build-runs/"+runID+"/heartbeat", map[string]any{
		"stage":        "baseline",
		"iteration":    1,
		"total_tokens": 128,
	}, http.StatusOK)

	postJSON(t, mux, "/internal/v1/artifacts:index", map[string]any{
		"run_id":   runID,
		"manifest": sampleArtifactManifestPayload(),
	}, http.StatusOK)

	postJSON(t, mux, "/internal/v1/metrics:index", map[string]any{
		"run_id":  runID,
		"metrics": sampleMetricsPayload(),
	}, http.StatusOK)

	postJSON(t, mux, "/internal/v1/build-runs/"+runID+"/complete", map[string]any{
		"builder_output": sampleBuildRunOutput(),
	}, http.StatusOK)

	getReq := httptest.NewRequest(http.MethodGet, "/internal/v1/build-runs/"+runID, nil)
	getRec := httptest.NewRecorder()
	mux.ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("GET run status = %d, want %d, body=%s", getRec.Code, http.StatusOK, getRec.Body.String())
	}
	var run map[string]any
	if err := json.Unmarshal(getRec.Body.Bytes(), &run); err != nil {
		t.Fatalf("Unmarshal run error = %v", err)
	}
	if run["status"] != "completed" {
		t.Fatalf("run status = %v, want completed", run["status"])
	}
}

func TestInternalRunRequirementCompletesRunAndWritesPrepareBundle(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	cfg := config.DefaultConfig()
	workspace := filepath.Join(filepath.Dir(configPath), "workspace")
	cfg.Workspace = workspace
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	h := NewHandler(configPath)
	t.Cleanup(h.waitForAsyncJobs)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	server := httptest.NewServer(mux)
	defer server.Close()

	resp := postJSONURL(t, server.Client(), server.URL+"/internal/v1/requirements:run", map[string]any{
		"requirement_text": "做一个简单记账 app，不考虑上架，只考虑功能实现，需要首页概览、记一笔和账单列表。",
		"builder_id":       "builder-a",
		"display_name":     "Builder A",
		"builder_image":    "builder:latest",
	}, http.StatusOK)
	if resp["worker_id"] != "builder-a" {
		t.Fatalf("worker_id = %v, want builder-a", resp["worker_id"])
	}
	if resp["status"] != "completed" {
		t.Fatalf("status = %v, want completed", resp["status"])
	}
	bundleDir, _ := resp["bundle_dir"].(string)
	if bundleDir == "" {
		t.Fatal("bundle_dir should not be empty")
	}
	for _, name := range []string{"requirement.md", "PRD.md", "PRD.json", "prd-approval.json", "template-approval.json", "template-fit-report.md", "implementation-plan.md", "manual-constraints.md", "builder-input.json"} {
		if _, err := os.Stat(filepath.Join(bundleDir, name)); err != nil {
			t.Fatalf("expected %s to exist in bundle dir: %v", name, err)
		}
	}
	runID, _ := resp["run_id"].(string)
	if runID == "" {
		t.Fatal("run_id should not be empty")
	}
	run := getJSONURL(t, server.Client(), server.URL+"/internal/v1/build-runs/"+runID, http.StatusOK)
	if run["status"] != "completed" {
		t.Fatalf("run status = %v, want completed", run["status"])
	}
	if _, err := os.Stat(filepath.Join(workspace, "appfactory", "jobs")); err != nil {
		t.Fatalf("expected appfactory jobs root to exist: %v", err)
	}
}

func TestCompilePRDWritesBundleWithoutRun(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	cfg := config.DefaultConfig()
	workspace := filepath.Join(filepath.Dir(configPath), "workspace")
	cfg.Workspace = workspace
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	h := NewHandler(configPath)
	t.Cleanup(h.waitForAsyncJobs)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	server := httptest.NewServer(mux)
	defer server.Close()

	resp := postJSONURL(t, server.Client(), server.URL+"/api/v1/prds:compile", map[string]any{
		"requirement_text": "做一个简单记账 app，不考虑上架，只考虑功能实现，需要首页概览、记一笔和账单列表。",
	}, http.StatusOK)
	if resp["job_id"] != "job-bookkeeping-lite" {
		t.Fatalf("job_id = %v, want job-bookkeeping-lite", resp["job_id"])
	}
	if resp["prd_id"] != "prd-bookkeeping-lite" {
		t.Fatalf("prd_id = %v, want prd-bookkeeping-lite", resp["prd_id"])
	}
	if resp["template_id"] != "flutter-finance-lite" {
		t.Fatalf("template_id = %v, want flutter-finance-lite", resp["template_id"])
	}
	bundleDir, _ := resp["bundle_dir"].(string)
	if bundleDir == "" {
		t.Fatal("bundle_dir should not be empty")
	}
	for _, name := range []string{"requirement.md", "PRD.md", "PRD.json", "prd-approval.json", "template-approval.json", "template-fit-report.md", "implementation-plan.md", "manual-constraints.md", "builder-input.json"} {
		if _, err := os.Stat(filepath.Join(bundleDir, name)); err != nil {
			t.Fatalf("expected %s to exist in bundle dir: %v", name, err)
		}
	}
	files, ok := resp["files"].(map[string]any)
	if !ok {
		t.Fatalf("files type = %T, want map[string]any", resp["files"])
	}
	prdJSONPath, _ := files["prd_json_path"].(string)
	if prdJSONPath == "" || !strings.HasSuffix(prdJSONPath, filepath.Join("prepare", "PRD.json")) {
		t.Fatalf("prd_json_path = %q, want prepare/PRD.json", prdJSONPath)
	}
	if _, err := os.Stat(prdJSONPath); err != nil {
		t.Fatalf("expected prd_json_path to exist: %v", err)
	}
	jobsRoot := filepath.Join(workspace, "appfactory", "jobs", "job-bookkeeping-lite")
	runsRoot := filepath.Join(jobsRoot, "runs")
	if _, err := os.Stat(runsRoot); !os.IsNotExist(err) {
		t.Fatalf("expected no runs created for compile-only endpoint, stat err=%v", err)
	}
}

func TestCompilePRDGeneratesUniqueDefaultIDsForJobsUI(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	cfg := config.DefaultConfig()
	workspace := filepath.Join(filepath.Dir(configPath), "workspace")
	cfg.Workspace = workspace
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	h := NewHandler(configPath)
	t.Cleanup(h.waitForAsyncJobs)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	server := httptest.NewServer(mux)
	defer server.Close()

	resp := postJSONURL(t, server.Client(), server.URL+"/api/v1/prds:compile", map[string]any{
		"requirement_text":   "做一个简单记账 app，不考虑上架，只考虑功能实现，需要首页概览、记一笔和账单列表。",
		"requirement_source": "jobs-ui",
	}, http.StatusOK)
	jobID, _ := resp["job_id"].(string)
	prdID, _ := resp["prd_id"].(string)
	if !strings.HasPrefix(jobID, "JOB_") {
		t.Fatalf("job_id = %q, want generated jobs-ui job prefix", jobID)
	}
	if !strings.HasPrefix(prdID, "PRD_") {
		t.Fatalf("prd_id = %q, want generated jobs-ui prd prefix", prdID)
	}
	if strings.TrimPrefix(jobID, "JOB_") != strings.TrimPrefix(prdID, "PRD_") {
		t.Fatalf("job_id = %q and prd_id = %q, want shared timestamp suffix", jobID, prdID)
	}
	bundleDir, _ := resp["bundle_dir"].(string)
	if !strings.Contains(bundleDir, filepath.Join("jobs", jobID, "prepare")) {
		t.Fatalf("bundle_dir = %q, want generated job-specific prepare dir", bundleDir)
	}
	if _, err := os.Stat(filepath.Join(workspace, "appfactory", "jobs", jobID, "prepare", "builder-input.json")); err != nil {
		t.Fatalf("expected generated job prepare bundle to exist: %v", err)
	}
}

func TestCompilePRDAllowsGenericStructuredBundleForJobsUIRealChecks(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	cfg := config.DefaultConfig()
	workspace := filepath.Join(filepath.Dir(configPath), "workspace")
	cfg.Workspace = workspace
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	h := NewHandler(configPath)
	t.Cleanup(h.waitForAsyncJobs)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	server := httptest.NewServer(mux)
	defer server.Close()

	resp := postJSONURL(t, server.Client(), server.URL+"/api/v1/prds:compile", map[string]any{
		"requirement_text":   "做一个待办事项 app，需要首页摘要、新建待办、任务列表和详情页。",
		"requirement_source": "jobs-ui",
		"real_checks":        true,
	}, http.StatusOK)
	if resp["template_id"] != "flutter-open-lite" {
		t.Fatalf("template_id = %v, want flutter-open-lite", resp["template_id"])
	}
	jobID, _ := resp["job_id"].(string)
	if !strings.HasPrefix(jobID, "JOB_") {
		t.Fatalf("job_id = %q, want generated jobs-ui id", jobID)
	}
	bundleDir, _ := resp["bundle_dir"].(string)
	if !strings.Contains(bundleDir, filepath.Join("jobs", jobID, "prepare")) {
		t.Fatalf("bundle_dir = %q, want generated job-specific prepare dir", bundleDir)
	}
	builderInputPath := filepath.Join(workspace, "appfactory", "jobs", jobID, "prepare", "builder-input.json")
	data, err := os.ReadFile(builderInputPath)
	if err != nil {
		t.Fatalf("ReadFile(builder-input.json) error = %v", err)
	}
	var builderInput map[string]any
	if err := json.Unmarshal(data, &builderInput); err != nil {
		t.Fatalf("Unmarshal(builder-input.json) error = %v", err)
	}
	if builderInput["template_id"] != "flutter-open-lite" {
		t.Fatalf("builder-input template_id = %v, want flutter-open-lite", builderInput["template_id"])
	}
	checks, ok := builderInput["acceptance_checks"].([]any)
	if !ok || len(checks) != 9 {
		t.Fatalf("acceptance_checks = %v, want 9 checks for generic real-check Flutter bundle", builderInput["acceptance_checks"])
	}
	for index, checkID := range []string{
		"check-flutter-pub-get",
		"check-open-lite-counter-demo-removed",
		"check-open-lite-record-flow-wiring",
		"check-open-lite-local-persistence-wiring",
		"check-profile-open-lite-domain-branding",
		"check-profile-open-lite-domain-language",
		"check-flutter-analyze",
		"check-flutter-test",
		"check-flutter-build-apk",
	} {
		check, ok := checks[index].(map[string]any)
		if !ok || check["check_id"] != checkID {
			t.Fatalf("acceptance_checks[%d] = %v, want check_id=%q", index, checks[index], checkID)
		}
	}
}

func TestCompilePRDRejectsUnknownTemplate(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	cfg := config.DefaultConfig()
	cfg.Workspace = filepath.Join(filepath.Dir(configPath), "workspace")
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	h := NewHandler(configPath)
	t.Cleanup(h.waitForAsyncJobs)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	server := httptest.NewServer(mux)
	defer server.Close()

	req := httptest.NewRequest(http.MethodPost, server.URL+"/api/v1/prds:compile", bytes.NewBufferString(`{"requirement_text":"做一个简单记账 app。","template_id":"unknown-template"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
	var resp internalErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if resp.ErrorCode != "PREPARE_COMPILE_FAILED" {
		t.Fatalf("error_code = %q, want PREPARE_COMPILE_FAILED", resp.ErrorCode)
	}
	if !strings.Contains(resp.Message, "模板选择失败") || !strings.Contains(resp.Message, "template_id=unknown-template") || !strings.Contains(resp.Message, "省略 template_id 让系统自动匹配") {
		t.Fatalf("message = %q, want user-facing template selection guidance", resp.Message)
	}
	if strings.Contains(strings.ToLower(resp.Message), "bookkeeping") {
		t.Fatalf("message = %q, should not depend on bookkeeping wording", resp.Message)
	}
}

func TestGetPRDReturnsPreparedPRD(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	cfg := config.DefaultConfig()
	workspace := filepath.Join(filepath.Dir(configPath), "workspace")
	cfg.Workspace = workspace
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	h := NewHandler(configPath)
	t.Cleanup(h.waitForAsyncJobs)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	server := httptest.NewServer(mux)
	defer server.Close()

	postJSONURL(t, server.Client(), server.URL+"/api/v1/prds:compile", map[string]any{
		"requirement_text": "做一个简单记账 app，不考虑上架，只考虑功能实现，需要首页概览、记一笔和账单列表。",
	}, http.StatusOK)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/prds/prd-bookkeeping-lite?version=0.1.0", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var prd appprepare.PRD
	if err := json.Unmarshal(rec.Body.Bytes(), &prd); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if prd.ID != "prd-bookkeeping-lite" {
		t.Fatalf("id = %q, want prd-bookkeeping-lite", prd.ID)
	}
	if prd.Version != "0.1.0" {
		t.Fatalf("version = %q, want 0.1.0", prd.Version)
	}
	if len(prd.TemplateConstraints.PreferredTemplateIDs) == 0 || prd.TemplateConstraints.PreferredTemplateIDs[0] != "flutter-finance-lite" {
		t.Fatalf("template_constraints.preferred_template_ids = %v, want first=flutter-finance-lite", prd.TemplateConstraints.PreferredTemplateIDs)
	}
	if len(prd.FeatureList) == 0 {
		t.Fatal("feature_list should not be empty")
	}
}

func TestGetPRDReturnsNotFoundForUnknownID(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	cfg := config.DefaultConfig()
	cfg.Workspace = filepath.Join(filepath.Dir(configPath), "workspace")
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	h := NewHandler(configPath)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/prds/missing-prd", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusNotFound, rec.Body.String())
	}
	var resp internalErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if resp.ErrorCode != "PRD_NOT_FOUND" {
		t.Fatalf("error_code = %q, want PRD_NOT_FOUND", resp.ErrorCode)
	}
}

func TestCreateAndGetPublicJob(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	cfg := config.DefaultConfig()
	workspace := filepath.Join(filepath.Dir(configPath), "workspace")
	cfg.Workspace = workspace
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	h := NewHandler(configPath)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	server := httptest.NewServer(mux)
	defer server.Close()

	postJSONURL(t, server.Client(), server.URL+"/api/v1/prds:compile", map[string]any{
		"requirement_text": "做一个简单记账 app，不考虑上架，只考虑功能实现，需要首页概览、记一笔和账单列表。",
		"title":            "轻量记账 App",
		"job_id":           "job-public-bookkeeping",
		"prd_id":           "prd-public-bookkeeping",
	}, http.StatusOK)

	created := postJSONURL(t, server.Client(), server.URL+"/api/v1/jobs", map[string]any{
		"prd_id":      "prd-public-bookkeeping",
		"prd_version": "0.1.0",
		"template_id": "flutter-finance-lite",
	}, http.StatusOK)
	if created["job_id"] != "job-public-bookkeeping" {
		t.Fatalf("job_id = %v, want job-public-bookkeeping", created["job_id"])
	}
	if created["status"] != "queued" {
		t.Fatalf("status = %v, want queued", created["status"])
	}
	if created["title"] != "轻量记账 App" {
		t.Fatalf("title = %v, want 轻量记账 App", created["title"])
	}
	if created["phase"] != "builder" {
		t.Fatalf("phase = %v, want builder", created["phase"])
	}
	got := getJSONURL(t, server.Client(), server.URL+"/api/v1/jobs/job-public-bookkeeping", http.StatusOK)
	if got["prd_id"] != "prd-public-bookkeeping" {
		t.Fatalf("prd_id = %v, want prd-public-bookkeeping", got["prd_id"])
	}
	if got["template_id"] != "flutter-finance-lite" {
		t.Fatalf("template_id = %v, want flutter-finance-lite", got["template_id"])
	}
	if got["title"] != "轻量记账 App" {
		t.Fatalf("title = %v, want 轻量记账 App", got["title"])
	}
	approvals, ok := got["human_approvals"].([]any)
	if !ok || len(approvals) != 2 {
		t.Fatalf("human_approvals = %v, want 2 approval paths", got["human_approvals"])
	}
	if _, err := os.Stat(filepath.Join(workspace, "appfactory", "jobs", "job-public-bookkeeping", "job.json")); err != nil {
		t.Fatalf("expected job.json to exist: %v", err)
	}
}

func TestGetPublicJobReflectsUpdatedPreparedBundleInputs(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	cfg := config.DefaultConfig()
	workspace := filepath.Join(filepath.Dir(configPath), "workspace")
	cfg.Workspace = workspace
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	h := NewHandler(configPath)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	server := httptest.NewServer(mux)
	defer server.Close()

	postJSONURL(t, server.Client(), server.URL+"/api/v1/prds:compile", map[string]any{
		"requirement_text": "做一个简单记账 app，不考虑上架，只考虑功能实现，需要首页概览、记一笔和账单列表。",
		"job_id":           "job-public-input-refresh",
		"prd_id":           "prd-public-input-refresh",
	}, http.StatusOK)
	postJSONURL(t, server.Client(), server.URL+"/api/v1/jobs", map[string]any{
		"prd_id":      "prd-public-input-refresh",
		"template_id": "flutter-finance-lite",
	}, http.StatusOK)

	prepareDir := filepath.Join(workspace, "appfactory", "jobs", "job-public-input-refresh", "prepare")
	prdPath := filepath.Join(prepareDir, "PRD.json")
	prdData, err := os.ReadFile(prdPath)
	if err != nil {
		t.Fatalf("ReadFile(PRD.json) error = %v", err)
	}
	var prd map[string]any
	if err := json.Unmarshal(prdData, &prd); err != nil {
		t.Fatalf("Unmarshal(PRD.json) error = %v", err)
	}
	prd["version"] = "0.2.0"
	updatedPRD, err := json.MarshalIndent(prd, "", "  ")
	if err != nil {
		t.Fatalf("Marshal(PRD.json) error = %v", err)
	}
	if err := os.WriteFile(prdPath, append(updatedPRD, '\n'), 0o644); err != nil {
		t.Fatalf("WriteFile(PRD.json) error = %v", err)
	}

	builderInputPath := filepath.Join(prepareDir, "builder-input.json")
	builderInputData, err := os.ReadFile(builderInputPath)
	if err != nil {
		t.Fatalf("ReadFile(builder-input.json) error = %v", err)
	}
	var builderInput map[string]any
	if err := json.Unmarshal(builderInputData, &builderInput); err != nil {
		t.Fatalf("Unmarshal(builder-input.json) error = %v", err)
	}
	builderInput["template_id"] = "flutter-open-lite"
	updatedBuilderInput, err := json.MarshalIndent(builderInput, "", "  ")
	if err != nil {
		t.Fatalf("Marshal(builder-input.json) error = %v", err)
	}
	if err := os.WriteFile(builderInputPath, append(updatedBuilderInput, '\n'), 0o644); err != nil {
		t.Fatalf("WriteFile(builder-input.json) error = %v", err)
	}

	loaded := getJSONURL(t, server.Client(), server.URL+"/api/v1/jobs/job-public-input-refresh", http.StatusOK)
	if loaded["prd_version"] != "0.2.0" {
		t.Fatalf("prd_version = %v, want 0.2.0 from updated prepared PRD", loaded["prd_version"])
	}
	if loaded["template_id"] != "flutter-open-lite" {
		t.Fatalf("template_id = %v, want flutter-open-lite from updated builder-input", loaded["template_id"])
	}
	if loaded["builder_input_path"] != "jobs/job-public-input-refresh/prepare/builder-input.json" {
		t.Fatalf("builder_input_path = %v, want prepare builder-input path", loaded["builder_input_path"])
	}
}

func TestCreatePublicJobAppliesGoalSummaryAndHumanNotesOverridesToPreparedBuilderInput(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	cfg := config.DefaultConfig()
	workspace := filepath.Join(filepath.Dir(configPath), "workspace")
	cfg.Workspace = workspace
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	h := NewHandler(configPath)
	t.Cleanup(h.waitForAsyncJobs)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	server := httptest.NewServer(mux)
	defer server.Close()

	postJSONURL(t, server.Client(), server.URL+"/api/v1/prds:compile", map[string]any{
		"requirement_text": "做一个简单记账 app，不考虑上架，只考虑功能实现，需要首页概览、记一笔和账单列表。",
		"job_id":           "job-public-create-overrides",
		"prd_id":           "prd-public-create-overrides",
	}, http.StatusOK)
	created := postJSONURL(t, server.Client(), server.URL+"/api/v1/jobs", map[string]any{
		"prd_id":       "prd-public-create-overrides",
		"template_id":  "flutter-finance-lite",
		"goal_summary": "build android app with live repair canary setup",
		"human_notes": []map[string]any{{
			"note_id": "note-canary",
			"summary": "prefer stable public jobs repair canary",
			"scope":   "engineering",
		}},
	}, http.StatusOK)
	if created["job_id"] != "job-public-create-overrides" {
		t.Fatalf("job_id = %v, want job-public-create-overrides", created["job_id"])
	}
	builderInput := requireJSONObjectFile(t, filepath.Join(workspace, "appfactory", "jobs", "job-public-create-overrides", "prepare", "builder-input.json"))
	if builderInput["goal_summary"] != "build android app with live repair canary setup" {
		t.Fatalf("goal_summary = %v, want override applied", builderInput["goal_summary"])
	}
	humanNotes, ok := builderInput["human_notes"].([]any)
	if !ok || len(humanNotes) != 1 {
		t.Fatalf("human_notes = %#v, want one note", builderInput["human_notes"])
	}
	firstNote, ok := humanNotes[0].(map[string]any)
	if !ok {
		t.Fatalf("human_notes[0] = %T, want object", humanNotes[0])
	}
	if firstNote["note_id"] != "note-canary" {
		t.Fatalf("note_id = %v, want note-canary", firstNote["note_id"])
	}
	if firstNote["summary"] != "prefer stable public jobs repair canary" {
		t.Fatalf("summary = %v, want override summary", firstNote["summary"])
	}
}

func TestStartPublicJobUsesCurrentApprovalSnapshotsAfterJobCreation(t *testing.T) {
	configPath := filepath.Join(newRetriableTempDir(t), "config.json")
	cfg := config.DefaultConfig()
	workspace := filepath.Join(filepath.Dir(configPath), "workspace")
	cfg.Workspace = workspace
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	h := NewHandler(configPath)
	t.Cleanup(h.waitForAsyncJobs)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	server := httptest.NewServer(mux)
	defer server.Close()

	postJSONURL(t, server.Client(), server.URL+"/api/v1/prds:compile", map[string]any{
		"requirement_text": "做一个简单记账 app，不考虑上架，只考虑功能实现，需要首页概览、记一笔和账单列表。",
		"job_id":           "job-public-current-approval-snapshots",
		"prd_id":           "prd-public-current-approval-snapshots",
	}, http.StatusOK)
	postJSONURL(t, server.Client(), server.URL+"/api/v1/jobs", map[string]any{
		"prd_id":      "prd-public-current-approval-snapshots",
		"template_id": "flutter-finance-lite",
	}, http.StatusOK)
	postJSONURL(t, server.Client(), server.URL+"/internal/v1/builders:register", map[string]any{
		"builder_id":        "builder-a",
		"display_name":      "Builder A",
		"capability_tags":   []string{"flutter"},
		"max_parallel_runs": 1,
		"worker_profile": map[string]any{
			"image": "builder:latest",
		},
	}, http.StatusOK)

	prepareDir := filepath.Join(workspace, "appfactory", "jobs", "job-public-current-approval-snapshots", "prepare")
	prdApprovalPath := filepath.Join(prepareDir, appruns.PRDApprovalFileName)
	prdApprovalData, err := os.ReadFile(prdApprovalPath)
	if err != nil {
		t.Fatalf("ReadFile(prd-approval.json) error = %v", err)
	}
	var prdApproval appruns.ApprovalRecord
	if err := json.Unmarshal(prdApprovalData, &prdApproval); err != nil {
		t.Fatalf("Unmarshal(prd-approval.json) error = %v", err)
	}
	originalSubjectVersion := prdApproval.SubjectVersion
	prdApproval.Status = appruns.ApprovalStatusPending
	prdApproval.SubjectVersion = "prd-public-current-approval-snapshots@0.2.0"
	updatedPRDApproval, err := json.MarshalIndent(prdApproval, "", "  ")
	if err != nil {
		t.Fatalf("Marshal(prd-approval.json) error = %v", err)
	}
	if err := os.WriteFile(prdApprovalPath, append(updatedPRDApproval, '\n'), 0o644); err != nil {
		t.Fatalf("WriteFile(prd-approval.json) error = %v", err)
	}

	req, err := http.NewRequest(http.MethodPost, server.URL+"/api/v1/jobs/job-public-current-approval-snapshots:start", bytes.NewReader([]byte(`{}`)))
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := server.Client().Do(req)
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusConflict)
	}
	var body internalErrorResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	if body.ErrorCode != "JOB_START_CONFLICT" {
		t.Fatalf("error_code = %q, want JOB_START_CONFLICT", body.ErrorCode)
	}
	if !strings.Contains(body.Message, "approvals not ready") || !strings.Contains(body.Message, "prd=pending") {
		t.Fatalf("message = %q, want current approval snapshot conflict", body.Message)
	}

	prdApproval.Status = appruns.ApprovalStatusApproved
	prdApproval.SubjectVersion = originalSubjectVersion
	updatedPRDApproval, err = json.MarshalIndent(prdApproval, "", "  ")
	if err != nil {
		t.Fatalf("Marshal(restored prd-approval.json) error = %v", err)
	}
	if err := os.WriteFile(prdApprovalPath, append(updatedPRDApproval, '\n'), 0o644); err != nil {
		t.Fatalf("WriteFile(restored prd-approval.json) error = %v", err)
	}

	started := postJSONURL(t, server.Client(), server.URL+"/api/v1/jobs/job-public-current-approval-snapshots:start", map[string]any{}, http.StatusOK)
	if started["status"] != "running_builder" {
		t.Fatalf("status = %v, want running_builder after restored approval snapshot", started["status"])
	}
}

func TestStartPublicJobRejectsWhenApprovedPRDVersionNoLongerMatchesPreparedBundle(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	cfg := config.DefaultConfig()
	workspace := filepath.Join(filepath.Dir(configPath), "workspace")
	cfg.Workspace = workspace
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	h := NewHandler(configPath)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	server := httptest.NewServer(mux)
	defer server.Close()

	postJSONURL(t, server.Client(), server.URL+"/api/v1/prds:compile", map[string]any{
		"requirement_text": "做一个简单记账 app，不考虑上架，只考虑功能实现，需要首页概览、记一笔和账单列表。",
		"job_id":           "job-public-prd-version-drift",
		"prd_id":           "prd-public-prd-version-drift",
	}, http.StatusOK)
	postJSONURL(t, server.Client(), server.URL+"/api/v1/jobs", map[string]any{
		"prd_id":      "prd-public-prd-version-drift",
		"template_id": "flutter-finance-lite",
	}, http.StatusOK)

	prepareDir := filepath.Join(workspace, "appfactory", "jobs", "job-public-prd-version-drift", "prepare")
	prdPath := filepath.Join(prepareDir, "PRD.json")
	prdData, err := os.ReadFile(prdPath)
	if err != nil {
		t.Fatalf("ReadFile(PRD.json) error = %v", err)
	}
	var prd map[string]any
	if err := json.Unmarshal(prdData, &prd); err != nil {
		t.Fatalf("Unmarshal(PRD.json) error = %v", err)
	}
	prd["version"] = "0.2.0"
	updatedPRD, err := json.MarshalIndent(prd, "", "  ")
	if err != nil {
		t.Fatalf("Marshal(PRD.json) error = %v", err)
	}
	if err := os.WriteFile(prdPath, append(updatedPRD, '\n'), 0o644); err != nil {
		t.Fatalf("WriteFile(PRD.json) error = %v", err)
	}

	req, err := http.NewRequest(http.MethodPost, server.URL+"/api/v1/jobs/job-public-prd-version-drift:start", bytes.NewReader([]byte(`{}`)))
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := server.Client().Do(req)
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusConflict)
	}
	var body internalErrorResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	if body.ErrorCode != "JOB_START_CONFLICT" {
		t.Fatalf("error_code = %q, want JOB_START_CONFLICT", body.ErrorCode)
	}
	if !strings.Contains(body.Message, "approval snapshot conflict") || !strings.Contains(body.Message, "prd approval subject_version mismatch") || !strings.Contains(body.Message, "want prd-public-prd-version-drift@0.2.0@sha256:") || !strings.Contains(body.Message, "got prd-public-prd-version-drift@0.1.0@sha256:") {
		t.Fatalf("message = %q, want prd subject_version drift conflict", body.Message)
	}
	loaded := getJSONURL(t, server.Client(), server.URL+"/api/v1/jobs/job-public-prd-version-drift", http.StatusOK)
	if loaded["prd_version"] != "0.2.0" {
		t.Fatalf("prd_version = %v, want 0.2.0 after prepared bundle drift", loaded["prd_version"])
	}
	if loaded["status"] != "awaiting_prd_approval" {
		t.Fatalf("status = %v, want awaiting_prd_approval after approval drift", loaded["status"])
	}
	if loaded["phase"] != "approval" {
		t.Fatalf("phase = %v, want approval after approval drift", loaded["phase"])
	}
	statusContext, ok := loaded["status_context"].(map[string]any)
	if !ok {
		t.Fatalf("status_context = %T, want map[string]any", loaded["status_context"])
	}
	if statusContext["reason_code"] != "prd_approval_drift" {
		t.Fatalf("reason_code = %v, want prd_approval_drift", statusContext["reason_code"])
	}
	if statusContext["suggested_action"] != "submit_prd_approval" {
		t.Fatalf("suggested_action = %v, want submit_prd_approval", statusContext["suggested_action"])
	}
}

func TestSubmitPRDApprovalRequiresPrepareRecompileAfterPRDVersionDrift(t *testing.T) {
	configPath := filepath.Join(newRetriableTempDir(t), "config.json")
	cfg := config.DefaultConfig()
	workspace := filepath.Join(filepath.Dir(configPath), "workspace")
	cfg.Workspace = workspace
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	h := NewHandler(configPath)
	t.Cleanup(h.waitForAsyncJobs)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	server := httptest.NewServer(mux)
	defer server.Close()

	postJSONURL(t, server.Client(), server.URL+"/api/v1/prds:compile", map[string]any{
		"requirement_text": "做一个简单记账 app，不考虑上架，只考虑功能实现，需要首页概览、记一笔和账单列表。",
		"job_id":           "job-public-prd-reapproval",
		"prd_id":           "prd-public-prd-reapproval",
	}, http.StatusOK)
	postJSONURL(t, server.Client(), server.URL+"/api/v1/jobs", map[string]any{
		"prd_id":      "prd-public-prd-reapproval",
		"template_id": "flutter-finance-lite",
	}, http.StatusOK)
	postJSONURL(t, server.Client(), server.URL+"/internal/v1/builders:register", map[string]any{
		"builder_id":        "builder-a",
		"display_name":      "Builder A",
		"capability_tags":   []string{"flutter"},
		"max_parallel_runs": 1,
		"worker_profile": map[string]any{
			"image": "builder:latest",
		},
	}, http.StatusOK)

	prepareDir := filepath.Join(workspace, "appfactory", "jobs", "job-public-prd-reapproval", "prepare")
	prdPath := filepath.Join(prepareDir, "PRD.json")
	prdData, err := os.ReadFile(prdPath)
	if err != nil {
		t.Fatalf("ReadFile(PRD.json) error = %v", err)
	}
	var prd map[string]any
	if err := json.Unmarshal(prdData, &prd); err != nil {
		t.Fatalf("Unmarshal(PRD.json) error = %v", err)
	}
	prd["version"] = "0.2.0"
	updatedPRD, err := json.MarshalIndent(prd, "", "  ")
	if err != nil {
		t.Fatalf("Marshal(PRD.json) error = %v", err)
	}
	if err := os.WriteFile(prdPath, append(updatedPRD, '\n'), 0o644); err != nil {
		t.Fatalf("WriteFile(PRD.json) error = %v", err)
	}

	req, err := http.NewRequest(http.MethodPost, server.URL+"/api/v1/jobs/job-public-prd-reapproval:start", bytes.NewReader([]byte(`{}`)))
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := server.Client().Do(req)
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusConflict)
	}

	reapproval := postJSONURL(t, server.Client(), server.URL+"/api/v1/prds/prd-public-prd-reapproval:submit-approval", map[string]any{
		"job_id":  "job-public-prd-reapproval",
		"summary": "确认当前 PRD 0.2.0 可继续执行。",
	}, http.StatusOK)
	subjectVersion, _ := reapproval["subject_version"].(string)
	if !strings.HasPrefix(subjectVersion, "prd-public-prd-reapproval@0.2.0@sha256:") {
		t.Fatalf("subject_version = %v, want PRD content-bound subject version after reapproval", reapproval["subject_version"])
	}

	loaded := getJSONURL(t, server.Client(), server.URL+"/api/v1/jobs/job-public-prd-reapproval", http.StatusOK)
	if loaded["status"] != "awaiting_prepare_recompile" {
		t.Fatalf("status = %v, want awaiting_prepare_recompile after PRD reapproval", loaded["status"])
	}
	if loaded["phase"] != "prepare" {
		t.Fatalf("phase = %v, want prepare after PRD reapproval", loaded["phase"])
	}
	statusContext, ok := loaded["status_context"].(map[string]any)
	if !ok {
		t.Fatalf("status_context = %T, want map[string]any", loaded["status_context"])
	}
	if statusContext["reason_code"] != "prepared_prd_source_stale" {
		t.Fatalf("reason_code = %v, want prepared_prd_source_stale", statusContext["reason_code"])
	}
	if statusContext["suggested_action"] != "compile_prepare_bundle" {
		t.Fatalf("suggested_action = %v, want compile_prepare_bundle", statusContext["suggested_action"])
	}
	if _, ok := loaded["resume_context"]; ok {
		t.Fatalf("resume_context should be omitted while prepare recompile is required, got %v", loaded["resume_context"])
	}

	req, err = http.NewRequest(http.MethodPost, server.URL+"/api/v1/jobs/job-public-prd-reapproval:start", bytes.NewReader([]byte(`{}`)))
	if err != nil {
		t.Fatalf("NewRequest(recompile required start) error = %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err = server.Client().Do(req)
	if err != nil {
		t.Fatalf("Do(recompile required start) error = %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusConflict)
	}
	var body internalErrorResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("Decode(recompile required start) error = %v", err)
	}
	if body.ErrorCode != "JOB_START_CONFLICT" {
		t.Fatalf("error_code = %q, want JOB_START_CONFLICT", body.ErrorCode)
	}
	if !strings.Contains(body.Message, "requires prepare recompile") {
		t.Fatalf("message = %q, want prepare recompile conflict", body.Message)
	}

	recompiled := postJSONURL(t, server.Client(), server.URL+"/api/v1/jobs/job-public-prd-reapproval:compile-prepare", map[string]any{}, http.StatusOK)
	if recompiled["status"] != "queued" {
		t.Fatalf("status = %v, want queued after compile prepare", recompiled["status"])
	}
	if recompiled["phase"] != "builder" {
		t.Fatalf("phase = %v, want builder after compile prepare", recompiled["phase"])
	}
	if rawStatusContext, ok := recompiled["status_context"]; ok && rawStatusContext != nil {
		statusContext, ok = rawStatusContext.(map[string]any)
		if !ok {
			t.Fatalf("status_context after compile = %T, want map[string]any", rawStatusContext)
		}
		if statusContext["reason_code"] != "prepared_input_stale" {
			t.Fatalf("reason_code after compile = %v, want prepared_input_stale", statusContext["reason_code"])
		}
		if statusContext["suggested_action"] != "start" {
			t.Fatalf("suggested_action after compile = %v, want start", statusContext["suggested_action"])
		}
	}

	builderInputPath := filepath.Join(prepareDir, "builder-input.json")
	builderInputData, err := os.ReadFile(builderInputPath)
	if err != nil {
		t.Fatalf("ReadFile(builder-input.json) error = %v", err)
	}
	var builderInput map[string]any
	if err := json.Unmarshal(builderInputData, &builderInput); err != nil {
		t.Fatalf("Unmarshal(builder-input.json) error = %v", err)
	}
	preparedPRDSubjectVersion, _ := builderInput["prepared_prd_subject_version"].(string)
	if !strings.HasPrefix(preparedPRDSubjectVersion, "prd-public-prd-reapproval@0.2.0@sha256:") {
		t.Fatalf("prepared_prd_subject_version = %q, want refreshed 0.2.0 source", preparedPRDSubjectVersion)
	}
}

func TestPRDMarkdownDriftRequiresPRDReapprovalInsteadOfPreparedStale(t *testing.T) {
	configPath := filepath.Join(newRetriableTempDir(t), "config.json")
	cfg := config.DefaultConfig()
	workspace := filepath.Join(filepath.Dir(configPath), "workspace")
	cfg.Workspace = workspace
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	h := NewHandler(configPath)
	t.Cleanup(h.waitForAsyncJobs)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	server := httptest.NewServer(mux)
	defer server.Close()

	postJSONURL(t, server.Client(), server.URL+"/api/v1/prds:compile", map[string]any{
		"requirement_text": "做一个简单记账 app，不考虑上架，只考虑功能实现，需要首页概览、记一笔和账单列表。",
		"job_id":           "job-public-prd-markdown-reapproval",
		"prd_id":           "prd-public-prd-markdown-reapproval",
	}, http.StatusOK)
	postJSONURL(t, server.Client(), server.URL+"/api/v1/jobs", map[string]any{
		"prd_id":      "prd-public-prd-markdown-reapproval",
		"template_id": "flutter-finance-lite",
	}, http.StatusOK)
	postJSONURL(t, server.Client(), server.URL+"/internal/v1/builders:register", map[string]any{
		"builder_id":        "builder-a",
		"display_name":      "Builder A",
		"capability_tags":   []string{"flutter"},
		"max_parallel_runs": 1,
		"worker_profile": map[string]any{
			"image": "builder:latest",
		},
	}, http.StatusOK)
	allocated := postJSONURL(t, server.Client(), server.URL+"/internal/v1/workers:allocate", map[string]any{
		"job_id": "job-public-prd-markdown-reapproval",
	}, http.StatusOK)
	leaseID, _ := allocated["lease_id"].(string)
	createdRun := postJSONURL(t, server.Client(), server.URL+"/internal/v1/build-runs", map[string]any{
		"worker_id":     "builder-a",
		"lease_id":      leaseID,
		"builder_input": sampleBuildRunInputFor("job-public-prd-markdown-reapproval", "prd-public-prd-markdown-reapproval", "flutter-finance-lite"),
	}, http.StatusOK)
	runID, _ := createdRun["run_id"].(string)
	postJSONURL(t, server.Client(), server.URL+"/internal/v1/build-runs/"+runID+"/fail", map[string]any{
		"summary":             "builder runtime crashed",
		"recovery_suggestion": "retry later",
		"failure_signatures":  []string{"runtime_crash"},
	}, http.StatusOK)

	prdMarkdownPath := filepath.Join(workspace, "appfactory", "jobs", "job-public-prd-markdown-reapproval", "prepare", "PRD.md")
	prdMarkdownData, err := os.ReadFile(prdMarkdownPath)
	if err != nil {
		t.Fatalf("ReadFile(PRD.md) error = %v", err)
	}
	if err := os.WriteFile(prdMarkdownPath, append(prdMarkdownData, []byte("\n- 新增人工说明：首页概览必须强调本月结余口径。\n")...), 0o644); err != nil {
		t.Fatalf("WriteFile(PRD.md) error = %v", err)
	}

	loaded := getJSONURL(t, server.Client(), server.URL+"/api/v1/jobs/job-public-prd-markdown-reapproval", http.StatusOK)
	if loaded["status"] != "awaiting_prd_approval" {
		t.Fatalf("status = %v, want awaiting_prd_approval after PRD markdown drift", loaded["status"])
	}
	if loaded["phase"] != "approval" {
		t.Fatalf("phase = %v, want approval after PRD markdown drift", loaded["phase"])
	}
	if _, ok := loaded["resume_context"]; ok {
		t.Fatalf("resume_context should be omitted while PRD reapproval is required, got %v", loaded["resume_context"])
	}
	statusContext, ok := loaded["status_context"].(map[string]any)
	if !ok {
		t.Fatalf("status_context = %T, want map[string]any", loaded["status_context"])
	}
	if statusContext["reason_code"] != "prd_approval_drift" {
		t.Fatalf("reason_code = %v, want prd_approval_drift", statusContext["reason_code"])
	}
	if statusContext["suggested_action"] != "submit_prd_approval" {
		t.Fatalf("suggested_action = %v, want submit_prd_approval", statusContext["suggested_action"])
	}

	reapproval := postJSONURL(t, server.Client(), server.URL+"/api/v1/prds/prd-public-prd-markdown-reapproval:submit-approval", map[string]any{
		"job_id":  "job-public-prd-markdown-reapproval",
		"summary": "确认更新后的 PRD 说明仍然沿用当前 failed run 的结构化输入。",
	}, http.StatusOK)
	if subjectVersion, _ := reapproval["subject_version"].(string); !strings.HasPrefix(subjectVersion, "prd-public-prd-markdown-reapproval@0.1.0@sha256:") || strings.Count(subjectVersion, "@sha256:") != 2 {
		t.Fatalf("subject_version = %v, want PRD approval version bound to markdown drift", reapproval["subject_version"])
	}

	resumed := postJSONURL(t, server.Client(), server.URL+"/api/v1/jobs/job-public-prd-markdown-reapproval:resume", map[string]any{
		"resume_mode": "retry_failed_run",
	}, http.StatusOK)
	if resumed["status"] != "queued" && resumed["status"] != "running_builder" {
		t.Fatalf("status = %v, want queued or running_builder after PRD markdown reapproval resume", resumed["status"])
	}
	if resumed["phase"] != "builder" {
		t.Fatalf("phase = %v, want builder after PRD markdown reapproval resume", resumed["phase"])
	}
	execution := readPublicJobExecutionRecord(t, workspace, "job-public-prd-markdown-reapproval")
	if execution.Action != "resume" {
		t.Fatalf("execution action = %q, want resume after PRD markdown reapproval", execution.Action)
	}
}

func TestRequirementDriftRequiresPRDReapprovalInsteadOfPreparedStale(t *testing.T) {
	configPath := filepath.Join(newRetriableTempDir(t), "config.json")
	cfg := config.DefaultConfig()
	workspace := filepath.Join(filepath.Dir(configPath), "workspace")
	cfg.Workspace = workspace
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	h := NewHandler(configPath)
	t.Cleanup(h.waitForAsyncJobs)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	server := httptest.NewServer(mux)
	defer server.Close()

	postJSONURL(t, server.Client(), server.URL+"/api/v1/prds:compile", map[string]any{
		"requirement_text": "做一个简单记账 app，不考虑上架，只考虑功能实现，需要首页概览、记一笔和账单列表。",
		"job_id":           "job-public-requirement-reapproval",
		"prd_id":           "prd-public-requirement-reapproval",
	}, http.StatusOK)
	postJSONURL(t, server.Client(), server.URL+"/api/v1/jobs", map[string]any{
		"prd_id":      "prd-public-requirement-reapproval",
		"template_id": "flutter-finance-lite",
	}, http.StatusOK)
	postJSONURL(t, server.Client(), server.URL+"/internal/v1/builders:register", map[string]any{
		"builder_id":        "builder-a",
		"display_name":      "Builder A",
		"capability_tags":   []string{"flutter"},
		"max_parallel_runs": 1,
		"worker_profile": map[string]any{
			"image": "builder:latest",
		},
	}, http.StatusOK)
	allocated := postJSONURL(t, server.Client(), server.URL+"/internal/v1/workers:allocate", map[string]any{
		"job_id": "job-public-requirement-reapproval",
	}, http.StatusOK)
	leaseID, _ := allocated["lease_id"].(string)
	createdRun := postJSONURL(t, server.Client(), server.URL+"/internal/v1/build-runs", map[string]any{
		"worker_id":     "builder-a",
		"lease_id":      leaseID,
		"builder_input": sampleBuildRunInputFor("job-public-requirement-reapproval", "prd-public-requirement-reapproval", "flutter-finance-lite"),
	}, http.StatusOK)
	runID, _ := createdRun["run_id"].(string)
	postJSONURL(t, server.Client(), server.URL+"/internal/v1/build-runs/"+runID+"/fail", map[string]any{
		"summary":             "builder runtime crashed",
		"recovery_suggestion": "retry later",
		"failure_signatures":  []string{"runtime_crash"},
	}, http.StatusOK)

	requirementPath := filepath.Join(workspace, "appfactory", "jobs", "job-public-requirement-reapproval", "prepare", "requirement.md")
	requirementData, err := os.ReadFile(requirementPath)
	if err != nil {
		t.Fatalf("ReadFile(requirement.md) error = %v", err)
	}
	if err := os.WriteFile(requirementPath, append(requirementData, []byte("\n- 新增原始需求备注：首页必须保留一块本月结余解释说明。\n")...), 0o644); err != nil {
		t.Fatalf("WriteFile(requirement.md) error = %v", err)
	}

	loaded := getJSONURL(t, server.Client(), server.URL+"/api/v1/jobs/job-public-requirement-reapproval", http.StatusOK)
	if loaded["status"] != "awaiting_prd_approval" {
		t.Fatalf("status = %v, want awaiting_prd_approval after requirement drift", loaded["status"])
	}
	if loaded["phase"] != "approval" {
		t.Fatalf("phase = %v, want approval after requirement drift", loaded["phase"])
	}
	if _, ok := loaded["resume_context"]; ok {
		t.Fatalf("resume_context should be omitted while PRD reapproval is required, got %v", loaded["resume_context"])
	}
	statusContext, ok := loaded["status_context"].(map[string]any)
	if !ok {
		t.Fatalf("status_context = %T, want map[string]any", loaded["status_context"])
	}
	if statusContext["reason_code"] != "prd_approval_drift" {
		t.Fatalf("reason_code = %v, want prd_approval_drift", statusContext["reason_code"])
	}
	if statusContext["suggested_action"] != "submit_prd_approval" {
		t.Fatalf("suggested_action = %v, want submit_prd_approval", statusContext["suggested_action"])
	}

	reapproval := postJSONURL(t, server.Client(), server.URL+"/api/v1/prds/prd-public-requirement-reapproval:submit-approval", map[string]any{
		"job_id":  "job-public-requirement-reapproval",
		"summary": "确认更新后的原始需求说明仍然沿用当前 failed run 的结构化 PRD。",
	}, http.StatusOK)
	if subjectVersion, _ := reapproval["subject_version"].(string); !strings.HasPrefix(subjectVersion, "prd-public-requirement-reapproval@0.1.0@sha256:") || strings.Count(subjectVersion, "@sha256:") != 2 {
		t.Fatalf("subject_version = %v, want PRD approval version bound to requirement drift", reapproval["subject_version"])
	}

	resumed := postJSONURL(t, server.Client(), server.URL+"/api/v1/jobs/job-public-requirement-reapproval:resume", map[string]any{
		"resume_mode": "retry_failed_run",
	}, http.StatusOK)
	if resumed["status"] != "queued" && resumed["status"] != "running_builder" {
		t.Fatalf("status = %v, want queued or running_builder after requirement reapproval resume", resumed["status"])
	}
	if resumed["phase"] != "builder" {
		t.Fatalf("phase = %v, want builder after requirement reapproval resume", resumed["phase"])
	}
	execution := readPublicJobExecutionRecord(t, workspace, "job-public-requirement-reapproval")
	if execution.Action != "resume" {
		t.Fatalf("execution action = %q, want resume after requirement reapproval", execution.Action)
	}
}

func TestGetPublicJobReturnsQueuedAfterRecompileInvalidatesFailedRun(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	cfg := config.DefaultConfig()
	workspace := filepath.Join(filepath.Dir(configPath), "workspace")
	cfg.Workspace = workspace
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	h := NewHandler(configPath)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	server := httptest.NewServer(mux)
	defer server.Close()

	postJSONURL(t, server.Client(), server.URL+"/api/v1/prds:compile", map[string]any{
		"requirement_text": "做一个简单记账 app，不考虑上架，只考虑功能实现，需要首页概览、记一笔和账单列表。",
		"job_id":           "job-public-recompile-invalidates-failed-run",
		"prd_id":           "prd-public-recompile-invalidates-failed-run",
	}, http.StatusOK)
	postJSONURL(t, server.Client(), server.URL+"/api/v1/jobs", map[string]any{
		"prd_id":      "prd-public-recompile-invalidates-failed-run",
		"template_id": "flutter-finance-lite",
	}, http.StatusOK)
	postJSONURL(t, server.Client(), server.URL+"/internal/v1/builders:register", map[string]any{
		"builder_id":        "builder-a",
		"display_name":      "Builder A",
		"capability_tags":   []string{"flutter"},
		"max_parallel_runs": 1,
		"worker_profile": map[string]any{
			"image": "builder:latest",
		},
	}, http.StatusOK)
	allocated := postJSONURL(t, server.Client(), server.URL+"/internal/v1/workers:allocate", map[string]any{
		"job_id": "job-public-recompile-invalidates-failed-run",
	}, http.StatusOK)
	leaseID, _ := allocated["lease_id"].(string)
	createdRun := postJSONURL(t, server.Client(), server.URL+"/internal/v1/build-runs", map[string]any{
		"worker_id":     "builder-a",
		"lease_id":      leaseID,
		"builder_input": sampleBuildRunInputFor("job-public-recompile-invalidates-failed-run", "prd-public-recompile-invalidates-failed-run", "flutter-finance-lite"),
	}, http.StatusOK)
	runID, _ := createdRun["run_id"].(string)
	postJSONURL(t, server.Client(), server.URL+"/internal/v1/build-runs/"+runID+"/fail", map[string]any{
		"summary":             "builder runtime crashed",
		"recovery_suggestion": "retry later",
		"failure_signatures":  []string{"runtime_crash"},
	}, http.StatusOK)

	postJSONURL(t, server.Client(), server.URL+"/api/v1/prds:compile", map[string]any{
		"requirement_text": "做一个简单记账 app，重新整理需求后保留首页概览、记一笔和账单列表。",
		"job_id":           "job-public-recompile-invalidates-failed-run",
		"prd_id":           "prd-public-recompile-invalidates-failed-run",
	}, http.StatusOK)

	loaded := getJSONURL(t, server.Client(), server.URL+"/api/v1/jobs/job-public-recompile-invalidates-failed-run", http.StatusOK)
	if loaded["status"] != "queued" {
		t.Fatalf("status = %v, want queued after recompile invalidates failed run", loaded["status"])
	}
	if loaded["phase"] != "builder" {
		t.Fatalf("phase = %v, want builder after recompile invalidates failed run", loaded["phase"])
	}
	if _, exists := loaded["resume_context"]; exists {
		t.Fatalf("resume_context should be omitted after recompile invalidates failed run, got %v", loaded["resume_context"])
	}
}

func TestStartPublicJobAllowsRestartAfterRecompileInvalidatesFailedRun(t *testing.T) {
	configPath := filepath.Join(newRetriableTempDir(t), "config.json")
	cfg := config.DefaultConfig()
	workspace := filepath.Join(filepath.Dir(configPath), "workspace")
	cfg.Workspace = workspace
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	h := NewHandler(configPath)
	t.Cleanup(h.waitForAsyncJobs)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	server := httptest.NewServer(mux)
	defer server.Close()

	postJSONURL(t, server.Client(), server.URL+"/api/v1/prds:compile", map[string]any{
		"requirement_text": "做一个简单记账 app，不考虑上架，只考虑功能实现，需要首页概览、记一笔和账单列表。",
		"job_id":           "job-public-recompile-restart",
		"prd_id":           "prd-public-recompile-restart",
	}, http.StatusOK)
	postJSONURL(t, server.Client(), server.URL+"/api/v1/jobs", map[string]any{
		"prd_id":      "prd-public-recompile-restart",
		"template_id": "flutter-finance-lite",
	}, http.StatusOK)
	postJSONURL(t, server.Client(), server.URL+"/internal/v1/builders:register", map[string]any{
		"builder_id":        "builder-a",
		"display_name":      "Builder A",
		"capability_tags":   []string{"flutter"},
		"max_parallel_runs": 1,
		"worker_profile": map[string]any{
			"image": "builder:latest",
		},
	}, http.StatusOK)
	allocated := postJSONURL(t, server.Client(), server.URL+"/internal/v1/workers:allocate", map[string]any{
		"job_id": "job-public-recompile-restart",
	}, http.StatusOK)
	leaseID, _ := allocated["lease_id"].(string)
	createdRun := postJSONURL(t, server.Client(), server.URL+"/internal/v1/build-runs", map[string]any{
		"worker_id":     "builder-a",
		"lease_id":      leaseID,
		"builder_input": sampleBuildRunInputFor("job-public-recompile-restart", "prd-public-recompile-restart", "flutter-finance-lite"),
	}, http.StatusOK)
	runID, _ := createdRun["run_id"].(string)
	postJSONURL(t, server.Client(), server.URL+"/internal/v1/build-runs/"+runID+"/fail", map[string]any{
		"summary":             "builder runtime crashed",
		"recovery_suggestion": "retry later",
		"failure_signatures":  []string{"runtime_crash"},
	}, http.StatusOK)

	postJSONURL(t, server.Client(), server.URL+"/api/v1/prds:compile", map[string]any{
		"requirement_text": "做一个简单记账 app，重新编译后作为新一轮执行输入。",
		"job_id":           "job-public-recompile-restart",
		"prd_id":           "prd-public-recompile-restart",
	}, http.StatusOK)

	started := postJSONURL(t, server.Client(), server.URL+"/api/v1/jobs/job-public-recompile-restart:start", map[string]any{}, http.StatusOK)
	if started["status"] != "queued" && started["status"] != "running_builder" {
		t.Fatalf("status = %v, want queued or running_builder after restart from recompiled bundle", started["status"])
	}
	if started["phase"] != "builder" {
		t.Fatalf("phase = %v, want builder after restart from recompiled bundle", started["phase"])
	}
	execution := readPublicJobExecutionRecord(t, workspace, "job-public-recompile-restart")
	if execution.Action != "start" {
		t.Fatalf("execution action = %q, want start after recompiled bundle restart", execution.Action)
	}
}

func TestGetPublicJobReturnsQueuedWhenPreparedBuilderInputChangesAfterFailure(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	cfg := config.DefaultConfig()
	workspace := filepath.Join(filepath.Dir(configPath), "workspace")
	cfg.Workspace = workspace
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	h := NewHandler(configPath)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	server := httptest.NewServer(mux)
	defer server.Close()

	postJSONURL(t, server.Client(), server.URL+"/api/v1/prds:compile", map[string]any{
		"requirement_text": "做一个简单记账 app，不考虑上架，只考虑功能实现，需要首页概览、记一笔和账单列表。",
		"job_id":           "job-public-builder-input-stale-view",
		"prd_id":           "prd-public-builder-input-stale-view",
	}, http.StatusOK)
	postJSONURL(t, server.Client(), server.URL+"/api/v1/jobs", map[string]any{
		"prd_id":      "prd-public-builder-input-stale-view",
		"template_id": "flutter-finance-lite",
	}, http.StatusOK)
	postJSONURL(t, server.Client(), server.URL+"/internal/v1/builders:register", map[string]any{
		"builder_id":        "builder-a",
		"display_name":      "Builder A",
		"capability_tags":   []string{"flutter"},
		"max_parallel_runs": 1,
		"worker_profile": map[string]any{
			"image": "builder:latest",
		},
	}, http.StatusOK)
	allocated := postJSONURL(t, server.Client(), server.URL+"/internal/v1/workers:allocate", map[string]any{
		"job_id": "job-public-builder-input-stale-view",
	}, http.StatusOK)
	leaseID, _ := allocated["lease_id"].(string)
	createdRun := postJSONURL(t, server.Client(), server.URL+"/internal/v1/build-runs", map[string]any{
		"worker_id":     "builder-a",
		"lease_id":      leaseID,
		"builder_input": sampleBuildRunInputFor("job-public-builder-input-stale-view", "prd-public-builder-input-stale-view", "flutter-finance-lite"),
	}, http.StatusOK)
	runID, _ := createdRun["run_id"].(string)
	postJSONURL(t, server.Client(), server.URL+"/internal/v1/build-runs/"+runID+"/fail", map[string]any{
		"summary":             "builder runtime crashed",
		"recovery_suggestion": "retry later",
		"failure_signatures":  []string{"runtime_crash"},
	}, http.StatusOK)

	prepareDir := filepath.Join(workspace, "appfactory", "jobs", "job-public-builder-input-stale-view", "prepare")
	builderInputPath := filepath.Join(prepareDir, "builder-input.json")
	builderInputData, err := os.ReadFile(builderInputPath)
	if err != nil {
		t.Fatalf("ReadFile(builder-input.json) error = %v", err)
	}
	var builderInput map[string]any
	if err := json.Unmarshal(builderInputData, &builderInput); err != nil {
		t.Fatalf("Unmarshal(builder-input.json) error = %v", err)
	}
	builderInput["iteration_budget"] = 5
	updatedBuilderInput, err := json.MarshalIndent(builderInput, "", "  ")
	if err != nil {
		t.Fatalf("Marshal(builder-input.json) error = %v", err)
	}
	if err := os.WriteFile(builderInputPath, append(updatedBuilderInput, '\n'), 0o644); err != nil {
		t.Fatalf("WriteFile(builder-input.json) error = %v", err)
	}

	loaded := getJSONURL(t, server.Client(), server.URL+"/api/v1/jobs/job-public-builder-input-stale-view", http.StatusOK)
	if loaded["status"] != "queued" {
		t.Fatalf("status = %v, want queued after prepared builder-input drift", loaded["status"])
	}
	if loaded["phase"] != "builder" {
		t.Fatalf("phase = %v, want builder after prepared builder-input drift", loaded["phase"])
	}
	if _, exists := loaded["resume_context"]; exists {
		t.Fatalf("resume_context should be omitted after prepared builder-input drift, got %v", loaded["resume_context"])
	}
	statusContext, ok := loaded["status_context"].(map[string]any)
	if !ok {
		t.Fatalf("status_context = %T, want map[string]any", loaded["status_context"])
	}
	if statusContext["reason_code"] != "prepared_input_stale" {
		t.Fatalf("reason_code = %v, want prepared_input_stale", statusContext["reason_code"])
	}
	if statusContext["suggested_action"] != "start" {
		t.Fatalf("suggested_action = %v, want start", statusContext["suggested_action"])
	}
	if budgets, ok := loaded["budgets"].(map[string]any); !ok || budgets["iteration_budget"] != float64(5) {
		t.Fatalf("budgets = %v, want iteration_budget=5 from current builder-input", loaded["budgets"])
	}
}

func TestStartPublicJobAllowsRestartWhenPreparedBuilderInputChangesAfterFailure(t *testing.T) {
	configPath := filepath.Join(newRetriableTempDir(t), "config.json")
	cfg := config.DefaultConfig()
	workspace := filepath.Join(filepath.Dir(configPath), "workspace")
	cfg.Workspace = workspace
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	h := NewHandler(configPath)
	t.Cleanup(h.waitForAsyncJobs)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	server := httptest.NewServer(mux)
	defer server.Close()

	postJSONURL(t, server.Client(), server.URL+"/api/v1/prds:compile", map[string]any{
		"requirement_text": "做一个简单记账 app，不考虑上架，只考虑功能实现，需要首页概览、记一笔和账单列表。",
		"job_id":           "job-public-builder-input-stale-start",
		"prd_id":           "prd-public-builder-input-stale-start",
	}, http.StatusOK)
	postJSONURL(t, server.Client(), server.URL+"/api/v1/jobs", map[string]any{
		"prd_id":      "prd-public-builder-input-stale-start",
		"template_id": "flutter-finance-lite",
	}, http.StatusOK)
	postJSONURL(t, server.Client(), server.URL+"/internal/v1/builders:register", map[string]any{
		"builder_id":        "builder-a",
		"display_name":      "Builder A",
		"capability_tags":   []string{"flutter"},
		"max_parallel_runs": 1,
		"worker_profile": map[string]any{
			"image": "builder:latest",
		},
	}, http.StatusOK)
	allocated := postJSONURL(t, server.Client(), server.URL+"/internal/v1/workers:allocate", map[string]any{
		"job_id": "job-public-builder-input-stale-start",
	}, http.StatusOK)
	leaseID, _ := allocated["lease_id"].(string)
	createdRun := postJSONURL(t, server.Client(), server.URL+"/internal/v1/build-runs", map[string]any{
		"worker_id":     "builder-a",
		"lease_id":      leaseID,
		"builder_input": sampleBuildRunInputFor("job-public-builder-input-stale-start", "prd-public-builder-input-stale-start", "flutter-finance-lite"),
	}, http.StatusOK)
	runID, _ := createdRun["run_id"].(string)
	postJSONURL(t, server.Client(), server.URL+"/internal/v1/build-runs/"+runID+"/fail", map[string]any{
		"summary":             "builder runtime crashed",
		"recovery_suggestion": "retry later",
		"failure_signatures":  []string{"runtime_crash"},
	}, http.StatusOK)

	builderInputPath := filepath.Join(workspace, "appfactory", "jobs", "job-public-builder-input-stale-start", "prepare", "builder-input.json")
	builderInputData, err := os.ReadFile(builderInputPath)
	if err != nil {
		t.Fatalf("ReadFile(builder-input.json) error = %v", err)
	}
	var builderInput map[string]any
	if err := json.Unmarshal(builderInputData, &builderInput); err != nil {
		t.Fatalf("Unmarshal(builder-input.json) error = %v", err)
	}
	builderInput["goal_summary"] = "build android app with refreshed prepared input"
	updatedBuilderInput, err := json.MarshalIndent(builderInput, "", "  ")
	if err != nil {
		t.Fatalf("Marshal(builder-input.json) error = %v", err)
	}
	if err := os.WriteFile(builderInputPath, append(updatedBuilderInput, '\n'), 0o644); err != nil {
		t.Fatalf("WriteFile(builder-input.json) error = %v", err)
	}

	started := postJSONURL(t, server.Client(), server.URL+"/api/v1/jobs/job-public-builder-input-stale-start:start", map[string]any{}, http.StatusOK)
	if started["status"] != "queued" && started["status"] != "running_builder" {
		t.Fatalf("status = %v, want queued or running_builder after prepared builder-input drift", started["status"])
	}
	if started["phase"] != "builder" {
		t.Fatalf("phase = %v, want builder after prepared builder-input drift", started["phase"])
	}
	execution := readPublicJobExecutionRecord(t, workspace, "job-public-builder-input-stale-start")
	if execution.Action != "start" {
		t.Fatalf("execution action = %q, want start after prepared builder-input drift", execution.Action)
	}
}

func TestGetPublicJobReturnsQueuedWhenPreparedPlanChangesAfterFailure(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	cfg := config.DefaultConfig()
	workspace := filepath.Join(filepath.Dir(configPath), "workspace")
	cfg.Workspace = workspace
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	h := NewHandler(configPath)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	server := httptest.NewServer(mux)
	defer server.Close()

	postJSONURL(t, server.Client(), server.URL+"/api/v1/prds:compile", map[string]any{
		"requirement_text": "做一个简单记账 app，不考虑上架，只考虑功能实现，需要首页概览、记一笔和账单列表。",
		"job_id":           "job-public-plan-stale-view",
		"prd_id":           "prd-public-plan-stale-view",
	}, http.StatusOK)
	postJSONURL(t, server.Client(), server.URL+"/api/v1/jobs", map[string]any{
		"prd_id":      "prd-public-plan-stale-view",
		"template_id": "flutter-finance-lite",
	}, http.StatusOK)
	postJSONURL(t, server.Client(), server.URL+"/internal/v1/builders:register", map[string]any{
		"builder_id":        "builder-a",
		"display_name":      "Builder A",
		"capability_tags":   []string{"flutter"},
		"max_parallel_runs": 1,
		"worker_profile": map[string]any{
			"image": "builder:latest",
		},
	}, http.StatusOK)
	allocated := postJSONURL(t, server.Client(), server.URL+"/internal/v1/workers:allocate", map[string]any{
		"job_id": "job-public-plan-stale-view",
	}, http.StatusOK)
	leaseID, _ := allocated["lease_id"].(string)
	createdRun := postJSONURL(t, server.Client(), server.URL+"/internal/v1/build-runs", map[string]any{
		"worker_id":     "builder-a",
		"lease_id":      leaseID,
		"builder_input": sampleBuildRunInputFor("job-public-plan-stale-view", "prd-public-plan-stale-view", "flutter-finance-lite"),
	}, http.StatusOK)
	runID, _ := createdRun["run_id"].(string)
	postJSONURL(t, server.Client(), server.URL+"/internal/v1/build-runs/"+runID+"/fail", map[string]any{
		"summary":             "builder runtime crashed",
		"recovery_suggestion": "retry later",
		"failure_signatures":  []string{"runtime_crash"},
	}, http.StatusOK)
	bundleBefore, err := h.loadPreparedBundleByJobID("job-public-plan-stale-view")
	if err != nil {
		t.Fatalf("loadPreparedBundleByJobID() before plan drift error = %v", err)
	}
	if !strings.Contains(string(bundleBefore.BuilderInput.ContextFiles), "implementation_plan_path") {
		t.Fatal("context_files should include implementation_plan_path")
	}
	runsSvc, err := h.buildRunsControlPlane()
	if err != nil {
		t.Fatalf("buildRunsControlPlane() error = %v", err)
	}
	runBefore, err := runsSvc.Get(context.Background(), runID)
	if err != nil {
		t.Fatalf("runsSvc.Get() error = %v", err)
	}
	beforeDigest := appruns.PreparedInputDigest(bundleBefore.BuilderInput, bundleBefore.PrepareDir)
	if beforeDigest != strings.TrimSpace(runBefore.PreparedInputDigest) {
		t.Fatalf("prepared digest before plan drift = %q, run digest = %q, want equal", beforeDigest, runBefore.PreparedInputDigest)
	}

	planPath := filepath.Join(workspace, "appfactory", "jobs", "job-public-plan-stale-view", "prepare", "implementation-plan.md")
	planData, err := os.ReadFile(planPath)
	if err != nil {
		t.Fatalf("ReadFile(implementation-plan.md) error = %v", err)
	}
	updatedPlanData := append(append([]byte(nil), planData...), []byte("\n- 新增人工修订约束：首页概览必须先展示本月结余。\n")...)
	if err := os.WriteFile(planPath, updatedPlanData, 0o644); err != nil {
		t.Fatalf("WriteFile(implementation-plan.md) error = %v", err)
	}
	planDataAfterWrite, err := os.ReadFile(planPath)
	if err != nil {
		t.Fatalf("ReadFile(implementation-plan.md after write) error = %v", err)
	}
	if bytes.Equal(planData, planDataAfterWrite) {
		t.Fatal("implementation-plan.md content should change after write")
	}
	bundleAfter, err := h.loadPreparedBundleByJobID("job-public-plan-stale-view")
	if err != nil {
		t.Fatalf("loadPreparedBundleByJobID() after plan drift error = %v", err)
	}
	afterDigest := appruns.PreparedInputDigest(bundleAfter.BuilderInput, bundleAfter.PrepareDir)
	if afterDigest == beforeDigest {
		t.Fatalf("prepared digest should change after implementation-plan drift: before=%q after=%q", beforeDigest, afterDigest)
	}

	loaded := getJSONURL(t, server.Client(), server.URL+"/api/v1/jobs/job-public-plan-stale-view", http.StatusOK)
	if loaded["status"] != "queued" {
		t.Fatalf("status = %v, want queued after prepared plan drift", loaded["status"])
	}
	if loaded["phase"] != "builder" {
		t.Fatalf("phase = %v, want builder after prepared plan drift", loaded["phase"])
	}
	if _, exists := loaded["resume_context"]; exists {
		t.Fatalf("resume_context should be omitted after prepared plan drift, got %v", loaded["resume_context"])
	}
	statusContext, ok := loaded["status_context"].(map[string]any)
	if !ok {
		t.Fatalf("status_context = %T, want map[string]any", loaded["status_context"])
	}
	if statusContext["reason_code"] != "prepared_input_stale" {
		t.Fatalf("reason_code = %v, want prepared_input_stale", statusContext["reason_code"])
	}
	if statusContext["suggested_action"] != "start" {
		t.Fatalf("suggested_action = %v, want start", statusContext["suggested_action"])
	}
	if budgets, ok := loaded["budgets"].(map[string]any); !ok || budgets["iteration_budget"] != float64(3) {
		t.Fatalf("budgets = %v, want current builder budgets to remain intact", loaded["budgets"])
	}
}

func TestResumeFailedPublicJobRejectsWhenPreparedPlanChangesAfterFailure(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	cfg := config.DefaultConfig()
	workspace := filepath.Join(filepath.Dir(configPath), "workspace")
	cfg.Workspace = workspace
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	h := NewHandler(configPath)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	server := httptest.NewServer(mux)
	defer server.Close()

	postJSONURL(t, server.Client(), server.URL+"/api/v1/prds:compile", map[string]any{
		"requirement_text": "做一个简单记账 app，不考虑上架，只考虑功能实现，需要首页概览、记一笔和账单列表。",
		"job_id":           "job-public-resume-plan-conflict",
		"prd_id":           "prd-public-resume-plan-conflict",
	}, http.StatusOK)
	postJSONURL(t, server.Client(), server.URL+"/api/v1/jobs", map[string]any{
		"prd_id":      "prd-public-resume-plan-conflict",
		"template_id": "flutter-finance-lite",
	}, http.StatusOK)
	postJSONURL(t, server.Client(), server.URL+"/internal/v1/builders:register", map[string]any{
		"builder_id":        "builder-a",
		"display_name":      "Builder A",
		"capability_tags":   []string{"flutter"},
		"max_parallel_runs": 1,
		"worker_profile": map[string]any{
			"image": "builder:latest",
		},
	}, http.StatusOK)
	allocated := postJSONURL(t, server.Client(), server.URL+"/internal/v1/workers:allocate", map[string]any{
		"job_id": "job-public-resume-plan-conflict",
	}, http.StatusOK)
	leaseID, _ := allocated["lease_id"].(string)
	createdRun := postJSONURL(t, server.Client(), server.URL+"/internal/v1/build-runs", map[string]any{
		"worker_id":     "builder-a",
		"lease_id":      leaseID,
		"builder_input": sampleBuildRunInputFor("job-public-resume-plan-conflict", "prd-public-resume-plan-conflict", "flutter-finance-lite"),
	}, http.StatusOK)
	runID, _ := createdRun["run_id"].(string)
	postJSONURL(t, server.Client(), server.URL+"/internal/v1/build-runs/"+runID+"/fail", map[string]any{
		"summary":             "builder runtime crashed",
		"recovery_suggestion": "retry later",
		"failure_signatures":  []string{"runtime_crash"},
	}, http.StatusOK)

	planPath := filepath.Join(workspace, "appfactory", "jobs", "job-public-resume-plan-conflict", "prepare", "implementation-plan.md")
	planData, err := os.ReadFile(planPath)
	if err != nil {
		t.Fatalf("ReadFile(implementation-plan.md) error = %v", err)
	}
	if err := os.WriteFile(planPath, append(planData, []byte("\n- 新增人工修订约束：账单列表必须先显示未分类条目。\n")...), 0o644); err != nil {
		t.Fatalf("WriteFile(implementation-plan.md) error = %v", err)
	}

	req, err := http.NewRequest(http.MethodPost, server.URL+"/api/v1/jobs/job-public-resume-plan-conflict:resume", bytes.NewReader([]byte(`{"resume_mode":"retry_failed_run"}`)))
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := server.Client().Do(req)
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusConflict)
	}
	var body internalErrorResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	if body.ErrorCode != "JOB_RESUME_CONFLICT" {
		t.Fatalf("error_code = %q, want JOB_RESUME_CONFLICT", body.ErrorCode)
	}
	if !strings.Contains(body.Message, "use start instead of resume") {
		t.Fatalf("message = %q, want stale prepared run conflict after plan drift", body.Message)
	}
}

func TestTemplateFitReportDriftRequiresTemplateReapprovalInsteadOfPreparedStale(t *testing.T) {
	configPath := filepath.Join(newRetriableTempDir(t), "config.json")
	cfg := config.DefaultConfig()
	workspace := filepath.Join(filepath.Dir(configPath), "workspace")
	cfg.Workspace = workspace
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	h := NewHandler(configPath)
	t.Cleanup(h.waitForAsyncJobs)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	server := httptest.NewServer(mux)
	defer server.Close()

	postJSONURL(t, server.Client(), server.URL+"/api/v1/prds:compile", map[string]any{
		"requirement_text": "做一个简单记账 app，不考虑上架，只考虑功能实现，需要首页概览、记一笔和账单列表。",
		"job_id":           "job-public-template-fit-reapproval",
		"prd_id":           "prd-public-template-fit-reapproval",
	}, http.StatusOK)
	postJSONURL(t, server.Client(), server.URL+"/api/v1/jobs", map[string]any{
		"prd_id":      "prd-public-template-fit-reapproval",
		"template_id": "flutter-finance-lite",
	}, http.StatusOK)
	postJSONURL(t, server.Client(), server.URL+"/internal/v1/builders:register", map[string]any{
		"builder_id":        "builder-a",
		"display_name":      "Builder A",
		"capability_tags":   []string{"flutter"},
		"max_parallel_runs": 1,
		"worker_profile": map[string]any{
			"image": "builder:latest",
		},
	}, http.StatusOK)
	allocated := postJSONURL(t, server.Client(), server.URL+"/internal/v1/workers:allocate", map[string]any{
		"job_id": "job-public-template-fit-reapproval",
	}, http.StatusOK)
	leaseID, _ := allocated["lease_id"].(string)
	createdRun := postJSONURL(t, server.Client(), server.URL+"/internal/v1/build-runs", map[string]any{
		"worker_id":     "builder-a",
		"lease_id":      leaseID,
		"builder_input": sampleBuildRunInputFor("job-public-template-fit-reapproval", "prd-public-template-fit-reapproval", "flutter-finance-lite"),
	}, http.StatusOK)
	runID, _ := createdRun["run_id"].(string)
	postJSONURL(t, server.Client(), server.URL+"/internal/v1/build-runs/"+runID+"/fail", map[string]any{
		"summary":             "builder runtime crashed",
		"recovery_suggestion": "retry later",
		"failure_signatures":  []string{"runtime_crash"},
	}, http.StatusOK)

	fitReportPath := filepath.Join(workspace, "appfactory", "jobs", "job-public-template-fit-reapproval", "prepare", "template-fit-report.md")
	fitReportData, err := os.ReadFile(fitReportPath)
	if err != nil {
		t.Fatalf("ReadFile(template-fit-report.md) error = %v", err)
	}
	if err := os.WriteFile(fitReportPath, append(fitReportData, []byte("\n- 新增人工备注：当前模板需要补看离线账单归档场景。\n")...), 0o644); err != nil {
		t.Fatalf("WriteFile(template-fit-report.md) error = %v", err)
	}

	loaded := getJSONURL(t, server.Client(), server.URL+"/api/v1/jobs/job-public-template-fit-reapproval", http.StatusOK)
	if loaded["status"] != "awaiting_template_approval" {
		t.Fatalf("status = %v, want awaiting_template_approval after fit report drift", loaded["status"])
	}
	if loaded["phase"] != "approval" {
		t.Fatalf("phase = %v, want approval after fit report drift", loaded["phase"])
	}
	if _, ok := loaded["resume_context"]; ok {
		t.Fatalf("resume_context should be omitted while template reapproval is required, got %v", loaded["resume_context"])
	}
	statusContext, ok := loaded["status_context"].(map[string]any)
	if !ok {
		t.Fatalf("status_context = %T, want map[string]any", loaded["status_context"])
	}
	if statusContext["reason_code"] != "template_approval_drift" {
		t.Fatalf("reason_code = %v, want template_approval_drift", statusContext["reason_code"])
	}
	if statusContext["suggested_action"] != "submit_template_approval" {
		t.Fatalf("suggested_action = %v, want submit_template_approval", statusContext["suggested_action"])
	}

	reapproval := postJSONURL(t, server.Client(), server.URL+"/api/v1/templates/flutter-finance-lite:submit-approval", map[string]any{
		"job_id":  "job-public-template-fit-reapproval",
		"prd_id":  "prd-public-template-fit-reapproval",
		"summary": "确认更新后的模板匹配结果仍然可以沿用当前 failed run 继续恢复。",
	}, http.StatusOK)
	if subjectVersion, _ := reapproval["subject_version"].(string); !strings.HasPrefix(subjectVersion, "selected-template@flutter-finance-lite@v0.1.0@sha256:") {
		t.Fatalf("subject_version = %v, want content-bound template approval version after fit report reapproval", reapproval["subject_version"])
	}

	resumed := postJSONURL(t, server.Client(), server.URL+"/api/v1/jobs/job-public-template-fit-reapproval:resume", map[string]any{
		"resume_mode": "retry_failed_run",
	}, http.StatusOK)
	if resumed["status"] != "queued" && resumed["status"] != "running_builder" {
		t.Fatalf("status = %v, want queued or running_builder after fit report reapproval resume", resumed["status"])
	}
	if resumed["phase"] != "builder" {
		t.Fatalf("phase = %v, want builder after fit report reapproval resume", resumed["phase"])
	}
	execution := readPublicJobExecutionRecord(t, workspace, "job-public-template-fit-reapproval")
	if execution.Action != "resume" {
		t.Fatalf("execution action = %q, want resume after fit report reapproval", execution.Action)
	}
}

func TestGetPublicJobReturnsQueuedWhenTemplateSeedChangesAfterFailure(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	cfg := config.DefaultConfig()
	workspace := filepath.Join(filepath.Dir(configPath), "workspace")
	cfg.Workspace = workspace
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	h := NewHandler(configPath)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	server := httptest.NewServer(mux)
	defer server.Close()

	postJSONURL(t, server.Client(), server.URL+"/api/v1/prds:compile", map[string]any{
		"requirement_text": "做一个简单记账 app，不考虑上架，只考虑功能实现，需要首页概览、记一笔和账单列表。",
		"job_id":           "job-public-template-seed-stale-view",
		"prd_id":           "prd-public-template-seed-stale-view",
	}, http.StatusOK)
	postJSONURL(t, server.Client(), server.URL+"/api/v1/jobs", map[string]any{
		"prd_id":      "prd-public-template-seed-stale-view",
		"template_id": "flutter-finance-lite",
	}, http.StatusOK)
	postJSONURL(t, server.Client(), server.URL+"/internal/v1/builders:register", map[string]any{
		"builder_id":        "builder-a",
		"display_name":      "Builder A",
		"capability_tags":   []string{"flutter"},
		"max_parallel_runs": 1,
		"worker_profile": map[string]any{
			"image": "builder:latest",
		},
	}, http.StatusOK)

	templateDir := filepath.Join(t.TempDir(), "template-seed")
	if err := os.MkdirAll(filepath.Join(templateDir, "lib"), 0o755); err != nil {
		t.Fatalf("MkdirAll(templateDir) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(templateDir, "pubspec.yaml"), []byte("name: template_seed\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(pubspec.yaml) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(templateDir, "lib", "main.dart"), []byte("void main() => print('seed-v1');\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(main.dart) error = %v", err)
	}
	builderInputPath := filepath.Join(workspace, "appfactory", "jobs", "job-public-template-seed-stale-view", "prepare", "builder-input.json")
	builderInputData, err := os.ReadFile(builderInputPath)
	if err != nil {
		t.Fatalf("ReadFile(builder-input.json) error = %v", err)
	}
	var builderInput map[string]any
	if err := json.Unmarshal(builderInputData, &builderInput); err != nil {
		t.Fatalf("Unmarshal(builder-input.json) error = %v", err)
	}
	builderInput["template_source_dir"] = templateDir
	updatedBuilderInputData, err := json.MarshalIndent(builderInput, "", "  ")
	if err != nil {
		t.Fatalf("Marshal(builder-input.json) error = %v", err)
	}
	updatedBuilderInputData = append(updatedBuilderInputData, '\n')
	if err := os.WriteFile(builderInputPath, updatedBuilderInputData, 0o644); err != nil {
		t.Fatalf("WriteFile(builder-input.json) error = %v", err)
	}

	allocated := postJSONURL(t, server.Client(), server.URL+"/internal/v1/workers:allocate", map[string]any{
		"job_id": "job-public-template-seed-stale-view",
	}, http.StatusOK)
	leaseID, _ := allocated["lease_id"].(string)
	buildInput := sampleBuildRunInputFor("job-public-template-seed-stale-view", "prd-public-template-seed-stale-view", "flutter-finance-lite")
	buildInput["template_source_dir"] = templateDir
	createdRun := postJSONURL(t, server.Client(), server.URL+"/internal/v1/build-runs", map[string]any{
		"worker_id":     "builder-a",
		"lease_id":      leaseID,
		"builder_input": buildInput,
	}, http.StatusOK)
	runID, _ := createdRun["run_id"].(string)
	postJSONURL(t, server.Client(), server.URL+"/internal/v1/build-runs/"+runID+"/fail", map[string]any{
		"summary":             "builder runtime crashed",
		"recovery_suggestion": "retry later",
		"failure_signatures":  []string{"runtime_crash"},
	}, http.StatusOK)

	bundleBefore, err := h.loadPreparedBundleByJobID("job-public-template-seed-stale-view")
	if err != nil {
		t.Fatalf("loadPreparedBundleByJobID() before template drift error = %v", err)
	}
	runsSvc, err := h.buildRunsControlPlane()
	if err != nil {
		t.Fatalf("buildRunsControlPlane() error = %v", err)
	}
	runBefore, err := runsSvc.Get(context.Background(), runID)
	if err != nil {
		t.Fatalf("runsSvc.Get() error = %v", err)
	}
	beforeDigest := appruns.PreparedInputDigest(bundleBefore.BuilderInput, bundleBefore.PrepareDir)
	if beforeDigest != strings.TrimSpace(runBefore.PreparedInputDigest) {
		t.Fatalf("prepared digest before template drift = %q, run digest = %q, want equal", beforeDigest, runBefore.PreparedInputDigest)
	}
	if err := os.WriteFile(filepath.Join(templateDir, "lib", "main.dart"), []byte("void main() => print('seed-v2');\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(main.dart) error = %v", err)
	}
	bundleAfter, err := h.loadPreparedBundleByJobID("job-public-template-seed-stale-view")
	if err != nil {
		t.Fatalf("loadPreparedBundleByJobID() after template drift error = %v", err)
	}
	afterDigest := appruns.PreparedInputDigest(bundleAfter.BuilderInput, bundleAfter.PrepareDir)
	if afterDigest == beforeDigest {
		t.Fatalf("prepared digest should change after template seed drift: before=%q after=%q", beforeDigest, afterDigest)
	}

	loaded := getJSONURL(t, server.Client(), server.URL+"/api/v1/jobs/job-public-template-seed-stale-view", http.StatusOK)
	if loaded["status"] != "queued" {
		t.Fatalf("status = %v, want queued after template seed drift", loaded["status"])
	}
	if loaded["phase"] != "builder" {
		t.Fatalf("phase = %v, want builder after template seed drift", loaded["phase"])
	}
	if _, exists := loaded["resume_context"]; exists {
		t.Fatalf("resume_context should be omitted after template seed drift, got %v", loaded["resume_context"])
	}
	statusContext, ok := loaded["status_context"].(map[string]any)
	if !ok {
		t.Fatalf("status_context = %T, want map[string]any", loaded["status_context"])
	}
	if statusContext["reason_code"] != "prepared_input_stale" {
		t.Fatalf("reason_code = %v, want prepared_input_stale", statusContext["reason_code"])
	}
	if statusContext["suggested_action"] != "start" {
		t.Fatalf("suggested_action = %v, want start", statusContext["suggested_action"])
	}
}

func TestResumeFailedPublicJobRejectsWhenTemplateSeedChangesAfterFailure(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	cfg := config.DefaultConfig()
	workspace := filepath.Join(filepath.Dir(configPath), "workspace")
	cfg.Workspace = workspace
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	h := NewHandler(configPath)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	server := httptest.NewServer(mux)
	defer server.Close()

	postJSONURL(t, server.Client(), server.URL+"/api/v1/prds:compile", map[string]any{
		"requirement_text": "做一个简单记账 app，不考虑上架，只考虑功能实现，需要首页概览、记一笔和账单列表。",
		"job_id":           "job-public-resume-template-seed-conflict",
		"prd_id":           "prd-public-resume-template-seed-conflict",
	}, http.StatusOK)
	postJSONURL(t, server.Client(), server.URL+"/api/v1/jobs", map[string]any{
		"prd_id":      "prd-public-resume-template-seed-conflict",
		"template_id": "flutter-finance-lite",
	}, http.StatusOK)
	postJSONURL(t, server.Client(), server.URL+"/internal/v1/builders:register", map[string]any{
		"builder_id":        "builder-a",
		"display_name":      "Builder A",
		"capability_tags":   []string{"flutter"},
		"max_parallel_runs": 1,
		"worker_profile": map[string]any{
			"image": "builder:latest",
		},
	}, http.StatusOK)

	templateDir := filepath.Join(t.TempDir(), "template-seed")
	if err := os.MkdirAll(filepath.Join(templateDir, "lib"), 0o755); err != nil {
		t.Fatalf("MkdirAll(templateDir) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(templateDir, "pubspec.yaml"), []byte("name: template_seed\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(pubspec.yaml) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(templateDir, "lib", "main.dart"), []byte("void main() => print('seed-v1');\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(main.dart) error = %v", err)
	}
	builderInputPath := filepath.Join(workspace, "appfactory", "jobs", "job-public-resume-template-seed-conflict", "prepare", "builder-input.json")
	builderInputData, err := os.ReadFile(builderInputPath)
	if err != nil {
		t.Fatalf("ReadFile(builder-input.json) error = %v", err)
	}
	var builderInput map[string]any
	if err := json.Unmarshal(builderInputData, &builderInput); err != nil {
		t.Fatalf("Unmarshal(builder-input.json) error = %v", err)
	}
	builderInput["template_source_dir"] = templateDir
	updatedBuilderInputData, err := json.MarshalIndent(builderInput, "", "  ")
	if err != nil {
		t.Fatalf("Marshal(builder-input.json) error = %v", err)
	}
	updatedBuilderInputData = append(updatedBuilderInputData, '\n')
	if err := os.WriteFile(builderInputPath, updatedBuilderInputData, 0o644); err != nil {
		t.Fatalf("WriteFile(builder-input.json) error = %v", err)
	}

	allocated := postJSONURL(t, server.Client(), server.URL+"/internal/v1/workers:allocate", map[string]any{
		"job_id": "job-public-resume-template-seed-conflict",
	}, http.StatusOK)
	leaseID, _ := allocated["lease_id"].(string)
	buildInput := sampleBuildRunInputFor("job-public-resume-template-seed-conflict", "prd-public-resume-template-seed-conflict", "flutter-finance-lite")
	buildInput["template_source_dir"] = templateDir
	createdRun := postJSONURL(t, server.Client(), server.URL+"/internal/v1/build-runs", map[string]any{
		"worker_id":     "builder-a",
		"lease_id":      leaseID,
		"builder_input": buildInput,
	}, http.StatusOK)
	runID, _ := createdRun["run_id"].(string)
	postJSONURL(t, server.Client(), server.URL+"/internal/v1/build-runs/"+runID+"/fail", map[string]any{
		"summary":             "builder runtime crashed",
		"recovery_suggestion": "retry later",
		"failure_signatures":  []string{"runtime_crash"},
	}, http.StatusOK)
	if err := os.WriteFile(filepath.Join(templateDir, "lib", "main.dart"), []byte("void main() => print('seed-v2');\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(main.dart) error = %v", err)
	}

	req, err := http.NewRequest(http.MethodPost, server.URL+"/api/v1/jobs/job-public-resume-template-seed-conflict:resume", bytes.NewReader([]byte(`{"resume_mode":"retry_failed_run"}`)))
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := server.Client().Do(req)
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusConflict)
	}
	var body internalErrorResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	if body.ErrorCode != "JOB_RESUME_CONFLICT" {
		t.Fatalf("error_code = %q, want JOB_RESUME_CONFLICT", body.ErrorCode)
	}
	if !strings.Contains(body.Message, "use start instead of resume") {
		t.Fatalf("message = %q, want stale prepared run conflict after template seed drift", body.Message)
	}
}

func TestStartPublicJobAllowsRestartWhenTemplateSeedChangesAfterFailure(t *testing.T) {
	configPath := filepath.Join(newRetriableTempDir(t), "config.json")
	cfg := config.DefaultConfig()
	workspace := filepath.Join(filepath.Dir(configPath), "workspace")
	cfg.Workspace = workspace
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	h := NewHandler(configPath)
	t.Cleanup(h.waitForAsyncJobs)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	server := httptest.NewServer(mux)
	defer server.Close()

	postJSONURL(t, server.Client(), server.URL+"/api/v1/prds:compile", map[string]any{
		"requirement_text": "做一个简单记账 app，不考虑上架，只考虑功能实现，需要首页概览、记一笔和账单列表。",
		"job_id":           "job-public-template-seed-stale-start",
		"prd_id":           "prd-public-template-seed-stale-start",
	}, http.StatusOK)
	postJSONURL(t, server.Client(), server.URL+"/api/v1/jobs", map[string]any{
		"prd_id":      "prd-public-template-seed-stale-start",
		"template_id": "flutter-finance-lite",
	}, http.StatusOK)
	postJSONURL(t, server.Client(), server.URL+"/internal/v1/builders:register", map[string]any{
		"builder_id":        "builder-a",
		"display_name":      "Builder A",
		"capability_tags":   []string{"flutter"},
		"max_parallel_runs": 1,
		"worker_profile": map[string]any{
			"image": "builder:latest",
		},
	}, http.StatusOK)

	templateDir := filepath.Join(t.TempDir(), "template-seed")
	if err := os.MkdirAll(filepath.Join(templateDir, "lib"), 0o755); err != nil {
		t.Fatalf("MkdirAll(templateDir) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(templateDir, "pubspec.yaml"), []byte("name: template_seed\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(pubspec.yaml) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(templateDir, "lib", "main.dart"), []byte("void main() => print('seed-v1');\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(main.dart) error = %v", err)
	}
	builderInputPath := filepath.Join(workspace, "appfactory", "jobs", "job-public-template-seed-stale-start", "prepare", "builder-input.json")
	builderInputData, err := os.ReadFile(builderInputPath)
	if err != nil {
		t.Fatalf("ReadFile(builder-input.json) error = %v", err)
	}
	var builderInput map[string]any
	if err := json.Unmarshal(builderInputData, &builderInput); err != nil {
		t.Fatalf("Unmarshal(builder-input.json) error = %v", err)
	}
	builderInput["template_source_dir"] = templateDir
	updatedBuilderInputData, err := json.MarshalIndent(builderInput, "", "  ")
	if err != nil {
		t.Fatalf("Marshal(builder-input.json) error = %v", err)
	}
	updatedBuilderInputData = append(updatedBuilderInputData, '\n')
	if err := os.WriteFile(builderInputPath, updatedBuilderInputData, 0o644); err != nil {
		t.Fatalf("WriteFile(builder-input.json) error = %v", err)
	}

	allocated := postJSONURL(t, server.Client(), server.URL+"/internal/v1/workers:allocate", map[string]any{
		"job_id": "job-public-template-seed-stale-start",
	}, http.StatusOK)
	leaseID, _ := allocated["lease_id"].(string)
	buildInput := sampleBuildRunInputFor("job-public-template-seed-stale-start", "prd-public-template-seed-stale-start", "flutter-finance-lite")
	buildInput["template_source_dir"] = templateDir
	createdRun := postJSONURL(t, server.Client(), server.URL+"/internal/v1/build-runs", map[string]any{
		"worker_id":     "builder-a",
		"lease_id":      leaseID,
		"builder_input": buildInput,
	}, http.StatusOK)
	runID, _ := createdRun["run_id"].(string)
	postJSONURL(t, server.Client(), server.URL+"/internal/v1/build-runs/"+runID+"/fail", map[string]any{
		"summary":             "builder runtime crashed",
		"recovery_suggestion": "retry later",
		"failure_signatures":  []string{"runtime_crash"},
	}, http.StatusOK)
	if err := os.WriteFile(filepath.Join(templateDir, "lib", "main.dart"), []byte("void main() => print('seed-v2');\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(main.dart) error = %v", err)
	}

	started := postJSONURL(t, server.Client(), server.URL+"/api/v1/jobs/job-public-template-seed-stale-start:start", map[string]any{}, http.StatusOK)
	if started["status"] != "queued" && started["status"] != "running_builder" {
		t.Fatalf("status = %v, want queued or running_builder after template seed drift", started["status"])
	}
	if started["phase"] != "builder" {
		t.Fatalf("phase = %v, want builder after template seed drift", started["phase"])
	}
	execution := readPublicJobExecutionRecord(t, workspace, "job-public-template-seed-stale-start")
	if execution.Action != "start" {
		t.Fatalf("execution action = %q, want start after template seed drift", execution.Action)
	}
}

func TestGetPublicJobArtifactsReturnsIndexedManifest(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	cfg := config.DefaultConfig()
	workspace := filepath.Join(filepath.Dir(configPath), "workspace")
	cfg.Workspace = workspace
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	h := NewHandler(configPath)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	server := httptest.NewServer(mux)
	defer server.Close()

	postJSONURL(t, server.Client(), server.URL+"/api/v1/prds:compile", map[string]any{
		"requirement_text": "做一个简单记账 app，不考虑上架，只考虑功能实现，需要首页概览、记一笔和账单列表。",
		"job_id":           "job-public-artifacts",
		"prd_id":           "prd-public-artifacts",
	}, http.StatusOK)
	postJSONURL(t, server.Client(), server.URL+"/api/v1/jobs", map[string]any{
		"prd_id":      "prd-public-artifacts",
		"prd_version": "0.1.0",
		"template_id": "flutter-finance-lite",
	}, http.StatusOK)

	postJSONURL(t, server.Client(), server.URL+"/internal/v1/builders:register", map[string]any{
		"builder_id":        "builder-a",
		"display_name":      "Builder A",
		"capability_tags":   []string{"flutter"},
		"max_parallel_runs": 1,
		"worker_profile": map[string]any{
			"image": "builder:latest",
		},
	}, http.StatusOK)
	allocated := postJSONURL(t, server.Client(), server.URL+"/internal/v1/workers:allocate", map[string]any{
		"job_id": "job-public-artifacts",
	}, http.StatusOK)
	leaseID, _ := allocated["lease_id"].(string)
	createdRun := postJSONURL(t, server.Client(), server.URL+"/internal/v1/build-runs", map[string]any{
		"worker_id":     "builder-a",
		"lease_id":      leaseID,
		"builder_input": sampleBuildRunInputFor("job-public-artifacts", "prd-public-artifacts", "flutter-finance-lite"),
	}, http.StatusOK)
	runID, _ := createdRun["run_id"].(string)
	postJSONURL(t, server.Client(), server.URL+"/internal/v1/artifacts:index", map[string]any{
		"run_id":   runID,
		"manifest": sampleArtifactManifestPayloadFor("job-public-artifacts"),
	}, http.StatusOK)
	postJSONURL(t, server.Client(), server.URL+"/internal/v1/build-runs/"+runID+"/complete", map[string]any{
		"builder_output": sampleBuildRunOutputFor("job-public-artifacts"),
	}, http.StatusOK)

	manifest := getJSONURL(t, server.Client(), server.URL+"/api/v1/jobs/job-public-artifacts/artifacts", http.StatusOK)
	if manifest["job_id"] != "job-public-artifacts" {
		t.Fatalf("job_id = %v, want job-public-artifacts", manifest["job_id"])
	}
	items, ok := manifest["items"].([]any)
	if !ok || len(items) != 1 {
		t.Fatalf("items = %v, want 1 artifact", manifest["items"])
	}
	first, ok := items[0].(map[string]any)
	if !ok {
		t.Fatalf("first artifact type = %T, want map[string]any", items[0])
	}
	if first["artifact_id"] != "apk-1" {
		t.Fatalf("artifact_id = %v, want apk-1", first["artifact_id"])
	}
}

func TestGetPublicJobEventsReturnsAggregatedTimeline(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	cfg := config.DefaultConfig()
	workspace := filepath.Join(filepath.Dir(configPath), "workspace")
	cfg.Workspace = workspace
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	h := NewHandler(configPath)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	server := httptest.NewServer(mux)
	defer server.Close()

	postJSONURL(t, server.Client(), server.URL+"/api/v1/prds:compile", map[string]any{
		"requirement_text": "做一个简单记账 app，不考虑上架，只考虑功能实现，需要首页概览、记一笔和账单列表。",
		"job_id":           "job-public-events",
		"prd_id":           "prd-public-events",
	}, http.StatusOK)
	postJSONURL(t, server.Client(), server.URL+"/api/v1/jobs", map[string]any{
		"prd_id":      "prd-public-events",
		"prd_version": "0.1.0",
		"template_id": "flutter-finance-lite",
	}, http.StatusOK)
	postJSONURL(t, server.Client(), server.URL+"/internal/v1/builders:register", map[string]any{
		"builder_id":   "builder-a",
		"display_name": "Builder A",
		"worker_profile": map[string]any{
			"image": "builder:latest",
		},
	}, http.StatusOK)
	allocated := postJSONURL(t, server.Client(), server.URL+"/internal/v1/workers:allocate", map[string]any{
		"job_id": "job-public-events",
	}, http.StatusOK)
	leaseID, _ := allocated["lease_id"].(string)
	createdRun := postJSONURL(t, server.Client(), server.URL+"/internal/v1/build-runs", map[string]any{
		"worker_id":     "builder-a",
		"lease_id":      leaseID,
		"builder_input": sampleBuildRunInputFor("job-public-events", "prd-public-events", "flutter-finance-lite"),
	}, http.StatusOK)
	runID, _ := createdRun["run_id"].(string)
	postJSONURL(t, server.Client(), server.URL+"/internal/v1/build-runs/"+runID+"/heartbeat", map[string]any{
		"stage":          "baseline",
		"iteration":      1,
		"round_id":       "round-1",
		"attempt":        1,
		"checkpoint_key": "check-flutter-pub-get",
		"round_state": map[string]any{
			"current_phase": "validate",
			"phase_trace":   []string{"inspect", "edit", "validate"},
		},
		"target_paths": []string{"lib/main.dart", "pubspec.yaml"},
		"total_tokens": 128,
		"summary":      "baseline passed",
	}, http.StatusOK)
	postJSONURL(t, server.Client(), server.URL+"/internal/v1/build-runs/"+runID+"/heartbeat", map[string]any{
		"event_type":     "run_patch_generation_started",
		"stage":          "baseline",
		"iteration":      1,
		"round_id":       "round-1",
		"attempt":        1,
		"checkpoint_key": "thin-prepare",
		"round_state": map[string]any{
			"current_phase": "edit",
			"phase_trace":   []string{"inspect", "edit"},
		},
		"target_paths": []string{"lib/main.dart"},
		"summary":      "round round-1 started patch generation for 1 file",
	}, http.StatusOK)
	postJSONURL(t, server.Client(), server.URL+"/internal/v1/build-runs/"+runID+"/heartbeat", map[string]any{
		"event_type":     "run_patch_generated",
		"stage":          "baseline",
		"iteration":      1,
		"round_id":       "round-1",
		"attempt":        1,
		"checkpoint_key": "thin-prepare",
		"round_state": map[string]any{
			"current_phase": "edit",
			"phase_trace":   []string{"inspect", "edit"},
		},
		"target_paths": []string{"lib/main.dart"},
		"summary":      "round round-1 generated patch for 1 file",
	}, http.StatusOK)
	postJSONURL(t, server.Client(), server.URL+"/internal/v1/build-runs/"+runID+"/heartbeat", map[string]any{
		"event_type":     "run_patch_generation_failed",
		"stage":          "baseline",
		"iteration":      1,
		"round_id":       "round-1",
		"attempt":        1,
		"checkpoint_key": "thin-prepare",
		"round_state": map[string]any{
			"current_phase": "edit",
			"phase_trace":   []string{"inspect", "edit"},
		},
		"target_paths": []string{"lib/main.dart"},
		"summary":      "round round-1 failed patch generation for 1 file: invalid patch schema",
	}, http.StatusOK)
	postJSONURL(t, server.Client(), server.URL+"/internal/v1/build-runs/"+runID+"/heartbeat", map[string]any{
		"event_type":     "run_patch_applied",
		"stage":          "baseline",
		"iteration":      1,
		"round_id":       "round-1",
		"attempt":        1,
		"checkpoint_key": "check-flutter-pub-get",
		"round_state": map[string]any{
			"current_phase": "edit",
			"phase_trace":   []string{"inspect", "edit"},
		},
		"target_paths":   []string{"lib/main.dart", "pubspec.yaml"},
		"affected_paths": []string{"lib/main.dart"},
		"summary":        "round round-1 applied patch to 1 file",
	}, http.StatusOK)
	postJSONURL(t, server.Client(), server.URL+"/internal/v1/build-runs/"+runID+"/heartbeat", map[string]any{
		"event_type":     "run_patch_apply_failed",
		"stage":          "baseline",
		"iteration":      1,
		"round_id":       "round-1",
		"attempt":        1,
		"checkpoint_key": "thin-prepare",
		"round_state": map[string]any{
			"current_phase": "edit",
			"phase_trace":   []string{"inspect", "edit"},
		},
		"target_paths": []string{"lib/main.dart"},
		"summary":      "round round-1 patch apply failed for 1 file: protected path violation",
	}, http.StatusOK)

	events := getJSONURL(t, server.Client(), server.URL+"/api/v1/jobs/job-public-events/events", http.StatusOK)
	items, ok := events["items"].([]any)
	if !ok || len(items) < 7 {
		t.Fatalf("items = %v, want at least 7 timeline items", events["items"])
	}
	foundCreated := false
	foundHeartbeat := false
	foundRoundStarted := false
	foundPatchGenerationStarted := false
	foundPatchGenerated := false
	foundPatchGenerationFailed := false
	foundPatchApplied := false
	foundPatchApplyFailed := false
	for _, item := range items {
		event, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if event["type"] == "job_created" {
			foundCreated = true
		}
		if event["type"] == "run_heartbeat" {
			foundHeartbeat = true
			if event["round_id"] != "round-1" {
				t.Fatalf("run_heartbeat round_id = %v, want round-1", event["round_id"])
			}
			if event["attempt"] != float64(1) {
				t.Fatalf("run_heartbeat attempt = %v, want 1", event["attempt"])
			}
			if event["checkpoint_key"] != "check-flutter-pub-get" {
				t.Fatalf("run_heartbeat checkpoint_key = %v, want check-flutter-pub-get", event["checkpoint_key"])
			}
			if event["current_phase"] != "validate" {
				t.Fatalf("run_heartbeat current_phase = %v, want validate", event["current_phase"])
			}
			phaseTrace, ok := event["phase_trace"].([]any)
			if !ok || len(phaseTrace) != 3 {
				t.Fatalf("run_heartbeat phase_trace = %v, want 3 phases", event["phase_trace"])
			}
			targetPaths, ok := event["target_paths"].([]any)
			if !ok || len(targetPaths) == 0 {
				t.Fatalf("run_heartbeat target_paths = %v, want non-empty array", event["target_paths"])
			}
		}
		if event["type"] == "run_round_started" {
			foundRoundStarted = true
			if event["round_id"] != "round-1" {
				t.Fatalf("run_round_started round_id = %v, want round-1", event["round_id"])
			}
			if event["attempt"] != float64(1) {
				t.Fatalf("run_round_started attempt = %v, want 1", event["attempt"])
			}
		}
		if event["type"] == "run_patch_generation_started" {
			foundPatchGenerationStarted = true
			if event["checkpoint_key"] != "thin-prepare" {
				t.Fatalf("run_patch_generation_started checkpoint_key = %v, want thin-prepare", event["checkpoint_key"])
			}
		}
		if event["type"] == "run_patch_generated" {
			foundPatchGenerated = true
			if event["checkpoint_key"] != "thin-prepare" {
				t.Fatalf("run_patch_generated checkpoint_key = %v, want thin-prepare", event["checkpoint_key"])
			}
			targetPaths, ok := event["target_paths"].([]any)
			if !ok || len(targetPaths) != 1 {
				t.Fatalf("run_patch_generated target_paths = %v, want 1 path", event["target_paths"])
			}
			fileFacts, ok := event["file_facts"].([]any)
			if !ok || len(fileFacts) != 1 {
				t.Fatalf("run_patch_generated file_facts = %v, want 1 fact", event["file_facts"])
			}
		}
		if event["type"] == "run_patch_generation_failed" {
			foundPatchGenerationFailed = true
			if event["checkpoint_key"] != "thin-prepare" {
				t.Fatalf("run_patch_generation_failed checkpoint_key = %v, want thin-prepare", event["checkpoint_key"])
			}
		}
		if event["type"] == "run_patch_applied" {
			foundPatchApplied = true
			if event["checkpoint_key"] != "check-flutter-pub-get" {
				t.Fatalf("run_patch_applied checkpoint_key = %v, want check-flutter-pub-get", event["checkpoint_key"])
			}
			affectedPaths, ok := event["affected_paths"].([]any)
			if !ok || len(affectedPaths) != 1 {
				t.Fatalf("run_patch_applied affected_paths = %v, want 1 path", event["affected_paths"])
			}
			fileFacts, ok := event["file_facts"].([]any)
			if !ok || len(fileFacts) != 1 {
				t.Fatalf("run_patch_applied file_facts = %v, want 1 fact", event["file_facts"])
			}
		}
		if event["type"] == "run_patch_apply_failed" {
			foundPatchApplyFailed = true
			if event["checkpoint_key"] != "thin-prepare" {
				t.Fatalf("run_patch_apply_failed checkpoint_key = %v, want thin-prepare", event["checkpoint_key"])
			}
		}
	}
	if !foundCreated || !foundHeartbeat || !foundRoundStarted || !foundPatchGenerationStarted || !foundPatchGenerated || !foundPatchGenerationFailed || !foundPatchApplied || !foundPatchApplyFailed {
		t.Fatalf("events missing expected items: created=%v heartbeat=%v round_started=%v patch_generation_started=%v patch_generated=%v patch_generation_failed=%v patch_applied=%v patch_apply_failed=%v items=%v", foundCreated, foundHeartbeat, foundRoundStarted, foundPatchGenerationStarted, foundPatchGenerated, foundPatchGenerationFailed, foundPatchApplied, foundPatchApplyFailed, items)
	}
}

func TestGetPublicJobExposesCurrentRoundWhileRunIsActive(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	cfg := config.DefaultConfig()
	workspace := filepath.Join(filepath.Dir(configPath), "workspace")
	cfg.Workspace = workspace
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	h := NewHandler(configPath)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	server := httptest.NewServer(mux)
	defer server.Close()

	postJSONURL(t, server.Client(), server.URL+"/api/v1/prds:compile", map[string]any{
		"requirement_text": "做一个简单记账 app，不考虑上架，只考虑功能实现，需要首页概览、记一笔和账单列表。",
		"job_id":           "job-public-current-round",
		"prd_id":           "prd-public-current-round",
	}, http.StatusOK)
	postJSONURL(t, server.Client(), server.URL+"/api/v1/jobs", map[string]any{
		"prd_id":      "prd-public-current-round",
		"prd_version": "0.1.0",
		"template_id": "flutter-finance-lite",
	}, http.StatusOK)
	postJSONURL(t, server.Client(), server.URL+"/internal/v1/builders:register", map[string]any{
		"builder_id":   "builder-a",
		"display_name": "Builder A",
		"worker_profile": map[string]any{
			"image": "builder:latest",
		},
	}, http.StatusOK)
	allocated := postJSONURL(t, server.Client(), server.URL+"/internal/v1/workers:allocate", map[string]any{
		"job_id": "job-public-current-round",
	}, http.StatusOK)
	leaseID, _ := allocated["lease_id"].(string)
	createdRun := postJSONURL(t, server.Client(), server.URL+"/internal/v1/build-runs", map[string]any{
		"worker_id":     "builder-a",
		"lease_id":      leaseID,
		"builder_input": sampleBuildRunInputFor("job-public-current-round", "prd-public-current-round", "flutter-finance-lite"),
	}, http.StatusOK)
	runID, _ := createdRun["run_id"].(string)
	postJSONURL(t, server.Client(), server.URL+"/internal/v1/build-runs/"+runID+"/heartbeat", map[string]any{
		"stage":          "baseline",
		"iteration":      1,
		"round_id":       "round-1",
		"attempt":        1,
		"checkpoint_key": "check-flutter-pub-get",
		"round_state": map[string]any{
			"current_phase": "validate",
			"phase_trace":   []string{"inspect", "edit", "validate"},
		},
		"target_paths": []string{"lib/main.dart", "pubspec.yaml"},
		"total_tokens": 128,
		"summary":      "baseline passed",
	}, http.StatusOK)

	loaded := getJSONURL(t, server.Client(), server.URL+"/api/v1/jobs/job-public-current-round", http.StatusOK)
	currentRound, ok := loaded["current_round"].(map[string]any)
	if !ok {
		t.Fatalf("current_round = %T, want object", loaded["current_round"])
	}
	if currentRound["round_id"] != "round-1" {
		t.Fatalf("current_round.round_id = %v, want round-1", currentRound["round_id"])
	}
	if currentRound["attempt"] != float64(1) {
		t.Fatalf("current_round.attempt = %v, want 1", currentRound["attempt"])
	}
	if currentRound["checkpoint_key"] != "check-flutter-pub-get" {
		t.Fatalf("current_round.checkpoint_key = %v, want check-flutter-pub-get", currentRound["checkpoint_key"])
	}
	if currentRound["current_phase"] != "validate" {
		t.Fatalf("current_round.current_phase = %v, want validate", currentRound["current_phase"])
	}
	phaseTrace, ok := currentRound["phase_trace"].([]any)
	if !ok || len(phaseTrace) != 3 {
		t.Fatalf("current_round.phase_trace = %v, want 3 phases", currentRound["phase_trace"])
	}
	targetPaths, ok := currentRound["target_paths"].([]any)
	if !ok || len(targetPaths) == 0 {
		t.Fatalf("current_round.target_paths = %v, want non-empty array", currentRound["target_paths"])
	}
}

func TestStartPublicJobRunsToCompletion(t *testing.T) {
	configPath := filepath.Join(newRetriableTempDir(t), "config.json")
	cfg := config.DefaultConfig()
	workspace := filepath.Join(filepath.Dir(configPath), "workspace")
	cfg.Workspace = workspace
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	h := NewHandler(configPath)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	server := httptest.NewServer(mux)
	defer server.Close()

	postJSONURL(t, server.Client(), server.URL+"/api/v1/prds:compile", map[string]any{
		"requirement_text": "做一个简单记账 app，不考虑上架，只考虑功能实现，需要首页概览、记一笔和账单列表。",
		"job_id":           "job-public-start",
		"prd_id":           "prd-public-start",
	}, http.StatusOK)
	postJSONURL(t, server.Client(), server.URL+"/api/v1/jobs", map[string]any{
		"prd_id":      "prd-public-start",
		"template_id": "flutter-finance-lite",
	}, http.StatusOK)
	postJSONURL(t, server.Client(), server.URL+"/internal/v1/builders:register", map[string]any{
		"builder_id":        "builder-a",
		"display_name":      "Builder A",
		"capability_tags":   []string{"flutter"},
		"max_parallel_runs": 1,
		"worker_profile": map[string]any{
			"image": "builder:latest",
		},
	}, http.StatusOK)

	started := postJSONURL(t, server.Client(), server.URL+"/api/v1/jobs/job-public-start:start", map[string]any{}, http.StatusOK)
	if started["status"] != "running_builder" {
		t.Fatalf("status = %v, want running_builder", started["status"])
	}
	if started["phase"] != "builder" {
		t.Fatalf("phase = %v, want builder", started["phase"])
	}
	if value, exists := started["builder_output_path"]; exists && value != "" {
		t.Fatalf("builder_output_path = %v, want omitted or empty before async completion", value)
	}
	execution := readPublicJobExecutionRecord(t, workspace, "job-public-start")
	if execution.Action != "start" {
		t.Fatalf("execution action = %q, want start", execution.Action)
	}
	if execution.RunID == "" {
		t.Fatal("execution run_id should not be empty")
	}
	if execution.Status != "queued" && execution.Status != "running" {
		t.Fatalf("execution status = %q, want queued or running", execution.Status)
	}
	completed := waitForJobStatus(t, server.Client(), server.URL+"/api/v1/jobs/job-public-start", "completed", 5*time.Second)
	if completed["builder_output_path"] == "" {
		t.Fatal("builder_output_path should not be empty after async completion")
	}
	execution = waitForPublicJobExecutionStatus(t, workspace, "job-public-start", "completed", 5*time.Second)
	if execution.LastError != "" {
		t.Fatalf("execution last_error = %q, want empty", execution.LastError)
	}

	events := getJSONURL(t, server.Client(), server.URL+"/api/v1/jobs/job-public-start/events", http.StatusOK)
	items, ok := events["items"].([]any)
	if !ok || len(items) == 0 {
		t.Fatalf("items = %v, want non-empty", events["items"])
	}
	foundExecutionEnqueued := false
	foundExecutionFinished := false
	foundRunCreated := false
	foundRunCompleted := false
	foundRunHeartbeat := false
	foundRunRoundStarted := false
	foundRunPatchGenerationStarted := false
	foundRunPatchGenerated := false
	foundRunPatchGenerationFailed := false
	foundRunPatchApplied := false
	foundRunPatchApplyFailed := false
	runPatchGenerationStartedCount := 0
	runPatchGeneratedCount := 0
	runPatchAppliedCount := 0
	for _, item := range items {
		event, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if event["type"] == "execution_enqueued" {
			foundExecutionEnqueued = true
		}
		if event["type"] == "execution_finished" {
			foundExecutionFinished = true
		}
		if event["type"] == "run_created" {
			foundRunCreated = true
			targetPaths, ok := event["target_paths"].([]any)
			if !ok || len(targetPaths) == 0 {
				t.Fatalf("run_created target_paths = %v, want non-empty array", event["target_paths"])
			}
		}
		if event["type"] == "run_heartbeat" {
			foundRunHeartbeat = true
			if strings.TrimSpace(fmt.Sprint(event["round_id"])) == "" {
				t.Fatalf("run_heartbeat round_id = %v, want non-empty string", event["round_id"])
			}
			if event["attempt"] != float64(1) {
				t.Fatalf("run_heartbeat attempt = %v, want 1", event["attempt"])
			}
			if strings.TrimSpace(fmt.Sprint(event["current_phase"])) == "" {
				t.Fatalf("run_heartbeat current_phase = %v, want non-empty string", event["current_phase"])
			}
			targetPaths, ok := event["target_paths"].([]any)
			if !ok || len(targetPaths) == 0 {
				t.Fatalf("run_heartbeat target_paths = %v, want non-empty array", event["target_paths"])
			}
		}
		if event["type"] == "run_round_started" {
			foundRunRoundStarted = true
			if strings.TrimSpace(fmt.Sprint(event["round_id"])) == "" {
				t.Fatalf("run_round_started round_id = %v, want non-empty string", event["round_id"])
			}
			if event["attempt"] != float64(1) {
				t.Fatalf("run_round_started attempt = %v, want 1", event["attempt"])
			}
		}
		if event["type"] == "run_patch_generation_started" {
			foundRunPatchGenerationStarted = true
			runPatchGenerationStartedCount++
			targetPaths, ok := event["target_paths"].([]any)
			if !ok || len(targetPaths) == 0 {
				t.Fatalf("run_patch_generation_started target_paths = %v, want non-empty array", event["target_paths"])
			}
		}
		if event["type"] == "run_patch_generated" {
			foundRunPatchGenerated = true
			runPatchGeneratedCount++
			targetPaths, ok := event["target_paths"].([]any)
			if !ok || len(targetPaths) == 0 {
				t.Fatalf("run_patch_generated target_paths = %v, want non-empty array", event["target_paths"])
			}
		}
		if event["type"] == "run_patch_generation_failed" {
			foundRunPatchGenerationFailed = true
		}
		if event["type"] == "run_patch_applied" {
			foundRunPatchApplied = true
			runPatchAppliedCount++
			affectedPaths, ok := event["affected_paths"].([]any)
			if !ok || len(affectedPaths) == 0 {
				t.Fatalf("run_patch_applied affected_paths = %v, want non-empty array", event["affected_paths"])
			}
			targetPaths, ok := event["target_paths"].([]any)
			if !ok || len(targetPaths) == 0 {
				t.Fatalf("run_patch_applied target_paths = %v, want non-empty array", event["target_paths"])
			}
		}
		if event["type"] == "run_patch_apply_failed" {
			foundRunPatchApplyFailed = true
		}
		if event["type"] == "run_completed" {
			foundRunCompleted = true
			affectedPaths, ok := event["affected_paths"].([]any)
			if !ok || len(affectedPaths) == 0 {
				t.Fatalf("run_completed affected_paths = %v, want non-empty array", event["affected_paths"])
			}
			roundSummaries, ok := event["round_summaries"].([]any)
			if !ok || len(roundSummaries) == 0 {
				t.Fatalf("run_completed round_summaries = %v, want non-empty array", event["round_summaries"])
			}
			firstRound, ok := roundSummaries[0].(map[string]any)
			if !ok {
				t.Fatalf("run_completed round_summaries[0] = %T, want object", roundSummaries[0])
			}
			if firstRound["attempt"] != float64(1) {
				t.Fatalf("run_completed round_summaries[0].attempt = %v, want 1", firstRound["attempt"])
			}
			targetPaths, ok := firstRound["target_paths"].([]any)
			if !ok || len(targetPaths) == 0 {
				t.Fatalf("run_completed round_summaries[0].target_paths = %v, want non-empty array", firstRound["target_paths"])
			}
		}
	}
	if !foundExecutionEnqueued || !foundExecutionFinished || !foundRunCreated || !foundRunHeartbeat || !foundRunRoundStarted || !foundRunPatchGenerationStarted || !foundRunPatchGenerated || !foundRunPatchApplied || !foundRunCompleted {
		t.Fatalf("missing events: execution_enqueued=%v execution_finished=%v run_created=%v run_heartbeat=%v run_round_started=%v run_patch_generation_started=%v run_patch_generated=%v run_patch_applied=%v run_completed=%v items=%v", foundExecutionEnqueued, foundExecutionFinished, foundRunCreated, foundRunHeartbeat, foundRunRoundStarted, foundRunPatchGenerationStarted, foundRunPatchGenerated, foundRunPatchApplied, foundRunCompleted, items)
	}
	if foundRunPatchGenerationFailed {
		t.Fatalf("run_patch_generation_failed should not appear in successful run, items=%v", items)
	}
	if foundRunPatchApplyFailed {
		t.Fatalf("run_patch_apply_failed should not appear in successful run, items=%v", items)
	}
	if runPatchGenerationStartedCount != 1 {
		t.Fatalf("run_patch_generation_started count = %d, want 1", runPatchGenerationStartedCount)
	}
	if runPatchGeneratedCount != 1 {
		t.Fatalf("run_patch_generated count = %d, want 1", runPatchGeneratedCount)
	}
	if runPatchAppliedCount != 1 {
		t.Fatalf("run_patch_applied count = %d, want 1 to avoid duplicate terminal replay", runPatchAppliedCount)
	}
}

func TestStartPublicJobDefaultExecutorWritesFlutterFallbackProbe(t *testing.T) {
	configPath := filepath.Join(newRetriableTempDir(t), "config.json")
	cfg := config.DefaultConfig()
	workspace := filepath.Join(filepath.Dir(configPath), "workspace")
	cfg.Workspace = workspace
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	h := NewHandler(configPath)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	server := httptest.NewServer(mux)
	defer server.Close()

	postJSONURL(t, server.Client(), server.URL+"/api/v1/prds:compile", map[string]any{
		"requirement_text": "做一个简单记账 app，不考虑上架，只考虑功能实现，需要首页概览、记一笔和账单列表。",
		"job_id":           "job-public-default-landing",
		"prd_id":           "prd-public-default-landing",
	}, http.StatusOK)
	postJSONURL(t, server.Client(), server.URL+"/api/v1/jobs", map[string]any{
		"prd_id":      "prd-public-default-landing",
		"template_id": "flutter-finance-lite",
	}, http.StatusOK)
	postJSONURL(t, server.Client(), server.URL+"/internal/v1/builders:register", map[string]any{
		"builder_id":        "builder-a",
		"display_name":      "Builder A",
		"capability_tags":   []string{"flutter"},
		"max_parallel_runs": 1,
		"worker_profile": map[string]any{
			"image": "builder:latest",
		},
	}, http.StatusOK)

	started := postJSONURL(t, server.Client(), server.URL+"/api/v1/jobs/job-public-default-landing:start", map[string]any{}, http.StatusOK)
	if started["status"] != "running_builder" {
		t.Fatalf("status = %v, want running_builder", started["status"])
	}
	completed := waitForJobStatus(t, server.Client(), server.URL+"/api/v1/jobs/job-public-default-landing", "completed", 5*time.Second)
	builderOutputPath, _ := completed["builder_output_path"].(string)
	if builderOutputPath == "" {
		t.Fatal("builder_output_path should not be empty after async completion")
	}
	builderOutput := requireJSONObjectFile(t, filepath.Join(workspace, "appfactory", filepath.FromSlash(builderOutputPath)))
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
		pathValue, _ := change["path"].(string)
		modifiedPaths[pathValue] = true
	}
	jobWorkspace := filepath.Join(workspace, "appfactory", "jobs", "job-public-default-landing", "workspace")
	if !modifiedPaths["lib/oneappfactory_executor_probe.dart"] {
		t.Fatalf("modified_files = %v, want probe path", modifiedPaths)
	}
	probe := string(mustReadFileBytes(t, filepath.Join(jobWorkspace, "lib", "oneappfactory_executor_probe.dart")))
	if !strings.Contains(probe, "Generated by OneAppFactory thin executor.") || !strings.Contains(probe, "'goalSummary': ") {
		t.Fatalf("probe = %q, want fallback metadata", probe)
	}
	if strings.Contains(probe, "BookkeepingApp") {
		t.Fatalf("probe should not embed hardcoded Flutter business implementation\n%s", probe)
	}
	if _, err := os.Stat(filepath.Join(jobWorkspace, "lib", "main.dart")); err != nil {
		t.Fatalf("expected seed file lib/main.dart to remain available: %v", err)
	}
	if _, err := os.Stat(filepath.Join(jobWorkspace, "lib", "models", "entry.dart")); err != nil {
		t.Fatalf("expected seeded bookkeeping model to remain available: %v", err)
	}
	if modifiedPaths["lib/models/entry.dart"] || modifiedPaths["lib/views/home_page.dart"] || modifiedPaths["lib/repositories/entry_repository.dart"] {
		t.Fatalf("thin executor fallback should only add the handoff probe, modified_paths=%v", modifiedPaths)
	}
	if _, err := os.Stat(filepath.Join(jobWorkspace, "pubspec.yaml")); err != nil {
		t.Fatalf("expected seed file pubspec.yaml to remain available: %v", err)
	}
	validationResults, ok := builderOutput["validation_results"].([]any)
	if !ok || len(validationResults) == 0 {
		t.Fatalf("validation_results = %T %#v, want non-empty results", builderOutput["validation_results"], builderOutput["validation_results"])
	}
	seenChecks := map[string]bool{}
	for _, item := range validationResults {
		result, ok := item.(map[string]any)
		if !ok {
			t.Fatalf("validation_result = %T, want object", item)
		}
		checkID, _ := result["check_id"].(string)
		seenChecks[checkID] = true
	}
	for _, checkID := range []string{"check-structural-template-files-ready", "check-oneappfactory-thin-fallback-probe", "check-oneappfactory-thin-fallback-metadata"} {
		if !seenChecks[checkID] {
			t.Fatalf("validation_results missing %s: %v", checkID, seenChecks)
		}
	}
}

func TestStartPublicJobWritesStructuredRoundOutputForFlutterProfile(t *testing.T) {
	configPath := filepath.Join(newRetriableTempDir(t), "config.json")
	cfg := config.DefaultConfig()
	workspace := filepath.Join(filepath.Dir(configPath), "workspace")
	cfg.Workspace = workspace
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	h := NewHandler(configPath)
	h.appFactory.runnerFactory = func(runsSvc runService) *appadapter.Runner {
		runner := appadapter.NewRunnerWithBackend(runServiceRunnerBackend{runsSvc: runsSvc})
		runner.Executor = structuredRoundTestExecutor{}
		return runner
	}
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	server := httptest.NewServer(mux)
	defer server.Close()

	postJSONURL(t, server.Client(), server.URL+"/api/v1/prds:compile", map[string]any{
		"requirement_text": "做一个简单记账 app，不考虑上架，只考虑功能实现，需要首页概览、记一笔和账单列表。",
		"job_id":           "job-public-structured-round",
		"prd_id":           "prd-public-structured-round",
	}, http.StatusOK)
	postJSONURL(t, server.Client(), server.URL+"/api/v1/jobs", map[string]any{
		"prd_id":      "prd-public-structured-round",
		"template_id": "flutter-finance-lite",
	}, http.StatusOK)
	postJSONURL(t, server.Client(), server.URL+"/internal/v1/builders:register", map[string]any{
		"builder_id":        "builder-a",
		"display_name":      "Builder A",
		"capability_tags":   []string{"flutter"},
		"max_parallel_runs": 1,
		"worker_profile": map[string]any{
			"image": "builder:latest",
		},
	}, http.StatusOK)

	started := postJSONURL(t, server.Client(), server.URL+"/api/v1/jobs/job-public-structured-round:start", map[string]any{}, http.StatusOK)
	if started["status"] != "running_builder" {
		t.Fatalf("status = %v, want running_builder", started["status"])
	}
	completed := waitForJobStatus(t, server.Client(), server.URL+"/api/v1/jobs/job-public-structured-round", "completed", 5*time.Second)
	builderOutputPath, _ := completed["builder_output_path"].(string)
	if builderOutputPath == "" {
		t.Fatal("builder_output_path should not be empty after async completion")
	}
	builderOutput := requireJSONObjectFile(t, filepath.Join(workspace, "appfactory", filepath.FromSlash(builderOutputPath)))

	roundInputs, ok := builderOutput["round_inputs"].([]any)
	if !ok || len(roundInputs) != 1 {
		t.Fatalf("round_inputs = %T %#v, want one round input", builderOutput["round_inputs"], builderOutput["round_inputs"])
	}
	roundOutputs, ok := builderOutput["round_outputs"].([]any)
	if !ok || len(roundOutputs) != 1 {
		t.Fatalf("round_outputs = %T %#v, want one round output", builderOutput["round_outputs"], builderOutput["round_outputs"])
	}
	firstRound, ok := roundOutputs[0].(map[string]any)
	if !ok {
		t.Fatalf("round_output[0] = %T, want object", roundOutputs[0])
	}
	if firstRound["status"] != "completed" {
		t.Fatalf("round_output.status = %v, want completed", firstRound["status"])
	}
	state, ok := firstRound["state"].(map[string]any)
	if !ok {
		t.Fatalf("round_output.state = %T, want object", firstRound["state"])
	}
	if state["current_phase"] != "finalize" || state["next_action"] != "stop" {
		t.Fatalf("round state = %#v, want finalize/stop", state)
	}
	workspacePatch, ok := firstRound["workspace_patch"].(map[string]any)
	if !ok {
		t.Fatalf("workspace_patch = %T, want object", firstRound["workspace_patch"])
	}
	status, _ := workspacePatch["status"].(string)
	if status != "not_reported" && status != "applied" {
		t.Fatalf("workspace_patch.status = %v, want not_reported or applied", workspacePatch["status"])
	}

	modifiedFiles, ok := builderOutput["modified_files"].([]any)
	if !ok {
		t.Fatalf("modified_files = %T %#v, want file change array", builderOutput["modified_files"], builderOutput["modified_files"])
	}
	for _, item := range modifiedFiles {
		change, ok := item.(map[string]any)
		if !ok {
			t.Fatalf("modified_file = %T, want object", item)
		}
		pathValue, _ := change["path"].(string)
		if strings.HasPrefix(pathValue, "android/") {
			t.Fatalf("modified_file.path = %q, want Flutter profile protected paths untouched", pathValue)
		}
	}

	validationResults, ok := builderOutput["validation_results"].([]any)
	if !ok || len(validationResults) == 0 {
		t.Fatalf("validation_results = %T %#v, want non-empty results", builderOutput["validation_results"], builderOutput["validation_results"])
	}
	seenChecks := map[string]bool{}
	for _, item := range validationResults {
		result, ok := item.(map[string]any)
		if !ok {
			t.Fatalf("validation_result = %T, want object", item)
		}
		checkID, _ := result["check_id"].(string)
		seenChecks[checkID] = true
	}
	for _, checkID := range []string{
		"check-counter-demo-removed",
		"check-entry-form-wiring",
		"check-local-persistence-wiring",
		"check-flutter-analyze",
		"check-flutter-test",
		"check-flutter-build-apk",
	} {
		if !seenChecks[checkID] {
			t.Fatalf("validation_results missing %s: %v", checkID, seenChecks)
		}
	}

	reportPaths, ok := builderOutput["report_paths"].(map[string]any)
	if !ok {
		t.Fatalf("report_paths = %T, want object", builderOutput["report_paths"])
	}
	changeSummaryPath, _ := reportPaths["change_summary_path"].(string)
	if changeSummaryPath == "" {
		t.Fatal("change_summary_path should not be empty")
	}
	changeSummary := string(mustReadFileBytes(t, filepath.Join(workspace, "appfactory", "jobs", "job-public-structured-round", filepath.FromSlash(changeSummaryPath))))
	for _, forbidden := range []string{"<LegacyBuilderFinish/>", "Made Plan", "Commands", "Uses"} {
		if strings.Contains(changeSummary, forbidden) {
			t.Fatalf("change summary should not contain legacy plan-only marker %q\n%s", forbidden, changeSummary)
		}
	}
}

func TestStartPublicJobGenericOpenLiteProducesWorkspaceForDeviceVerify(t *testing.T) {
	configPath := filepath.Join(newRetriableTempDir(t), "config.json")
	cfg := config.DefaultConfig()
	workspace := filepath.Join(filepath.Dir(configPath), "workspace")
	cfg.Workspace = workspace
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	h := NewHandler(configPath)
	h.appFactory.runnerFactory = func(runsSvc runService) *appadapter.Runner {
		runner := appadapter.NewRunnerWithBackend(runServiceRunnerBackend{runsSvc: runsSvc})
		runner.Executor = structuredGenericOpenLiteRoundTestExecutor{}
		return runner
	}
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	server := httptest.NewServer(mux)
	defer server.Close()

	postJSONURL(t, server.Client(), server.URL+"/api/v1/prds:compile", map[string]any{
		"requirement_text": "做一个待办事项 app，需要首页摘要、新建待办、任务列表和详情页，优先保证本地可用。",
		"job_id":           "job-public-generic-open-lite-device",
		"prd_id":           "prd-public-generic-open-lite-device",
		"real_checks":      true,
		"executor_image":   "oneappfactory/builder:local",
	}, http.StatusOK)
	postJSONURL(t, server.Client(), server.URL+"/api/v1/jobs", map[string]any{
		"prd_id":      "prd-public-generic-open-lite-device",
		"template_id": "flutter-open-lite",
	}, http.StatusOK)
	postJSONURL(t, server.Client(), server.URL+"/internal/v1/builders:register", map[string]any{
		"builder_id":        "builder-a",
		"display_name":      "Builder A",
		"capability_tags":   []string{"flutter"},
		"max_parallel_runs": 1,
		"worker_profile": map[string]any{
			"image": "builder:latest",
		},
	}, http.StatusOK)

	started := postJSONURL(t, server.Client(), server.URL+"/api/v1/jobs/job-public-generic-open-lite-device:start", map[string]any{}, http.StatusOK)
	if started["status"] != "running_builder" {
		t.Fatalf("status = %v, want running_builder", started["status"])
	}
	completed := waitForJobStatus(t, server.Client(), server.URL+"/api/v1/jobs/job-public-generic-open-lite-device", "completed", 5*time.Second)
	builderOutputPath, _ := completed["builder_output_path"].(string)
	if builderOutputPath == "" {
		t.Fatal("builder_output_path should not be empty after async completion")
	}
	builderOutput := requireJSONObjectFile(t, filepath.Join(workspace, "appfactory", filepath.FromSlash(builderOutputPath)))

	jobWorkspace := filepath.Join(workspace, "appfactory", "jobs", "job-public-generic-open-lite-device", "workspace")
	for _, rel := range []string{
		"pubspec.yaml",
		"lib/main.dart",
		"lib/models/record.dart",
		"lib/models/dashboard_summary.dart",
		"lib/repositories/record_repository.dart",
		"lib/views/home_page.dart",
		"lib/views/record_form_page.dart",
		"lib/views/record_list_page.dart",
		"lib/views/record_detail_page.dart",
		"test/widget_test.dart",
	} {
		if _, err := os.Stat(filepath.Join(jobWorkspace, filepath.FromSlash(rel))); err != nil {
			t.Fatalf("expected open-lite workspace file %s: %v", rel, err)
		}
	}

	validationResults, ok := builderOutput["validation_results"].([]any)
	if !ok || len(validationResults) == 0 {
		t.Fatalf("validation_results = %T %#v, want non-empty results", builderOutput["validation_results"], builderOutput["validation_results"])
	}
	seenChecks := map[string]bool{}
	for _, item := range validationResults {
		result, ok := item.(map[string]any)
		if !ok {
			t.Fatalf("validation_result = %T, want object", item)
		}
		checkID, _ := result["check_id"].(string)
		seenChecks[checkID] = true
	}
	for _, checkID := range []string{
		"check-open-lite-counter-demo-removed",
		"check-open-lite-record-flow-wiring",
		"check-open-lite-local-persistence-wiring",
		"check-flutter-analyze",
		"check-flutter-test",
		"check-flutter-build-apk",
	} {
		if !seenChecks[checkID] {
			t.Fatalf("validation_results missing %s: %v", checkID, seenChecks)
		}
	}
	if strings.Contains(string(mustReadFileBytes(t, filepath.Join(jobWorkspace, "lib", "main.dart"))), "BookkeepingApp") {
		t.Fatalf("generic open-lite workspace should not fall back to bookkeeping wording")
	}
	if completed["template_id"] != "flutter-open-lite" {
		t.Fatalf("template_id = %v, want flutter-open-lite", completed["template_id"])
	}
	if builderInput := requireJSONObjectFile(t, filepath.Join(workspace, "appfactory", "jobs", "job-public-generic-open-lite-device", "prepare", "builder-input.json")); builderInput["template_id"] != "flutter-open-lite" {
		t.Fatalf("builder-input template_id = %v, want flutter-open-lite", builderInput["template_id"])
	}
	if completed["status"] != "completed" {
		t.Fatalf("status = %v, want completed", completed["status"])
	}
	t.Logf("public_job_generic_open_lite_device_job_id=%s", "job-public-generic-open-lite-device")
}

func TestStartPublicJobAutoRepairsFlutterAnalyzeFailureThroughPublicJobsAPI(t *testing.T) {
	configPath := filepath.Join(newRetriableTempDir(t), "config.json")
	cfg := config.DefaultConfig()
	workspace := filepath.Join(filepath.Dir(configPath), "workspace")
	cfg.Workspace = workspace
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	h := NewHandler(configPath)
	t.Cleanup(h.waitForAsyncJobs)
	h.appFactory.runnerFactory = func(runsSvc runService) *appadapter.Runner {
		runner := appadapter.NewRunnerWithBackend(runServiceRunnerBackend{runsSvc: runsSvc})
		runner.BuilderRuntimeConfig = config.BuilderRuntimeConfig{
			Enabled:      true,
			DefaultModel: &config.AgentModelConfig{Primary: "qwen2.5-coder-14b-local"},
			TaskRoutes: []config.BuilderRuntimeTaskRouteConfig{{
				TaskType: "analyze_repair",
				Model:    &config.AgentModelConfig{Primary: "qwen2.5-coder-14b-local"},
			}},
		}
		runner.PatchGenerator = &publicJobAutoRepairCoveragePatchGenerator{}
		runner.Executor = publicJobAutoRepairExecutor{
			checkID:         "check-flutter-analyze",
			label:           "flutter analyze",
			stage:           appruns.StageCheap,
			commands:        []string{"flutter analyze"},
			script:          "grep -q 'Budget Flow' lib/main.dart || { echo 'Analyzing workspace...' >&2; echo '  error • Budget Flow missing • lib/main.dart:1:1 • oneappfactory_test' >&2; echo '1 issue found.' >&2; exit 1; }",
			skipEditCommand: true,
		}
		return runner
	}
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	server := httptest.NewServer(mux)
	defer server.Close()

	postJSONURL(t, server.Client(), server.URL+"/api/v1/prds:compile", map[string]any{
		"requirement_text": "做一个简单记账 app，不考虑上架，只考虑功能实现，需要首页概览、记一笔和账单列表。",
		"job_id":           "job-public-auto-repair",
		"prd_id":           "prd-public-auto-repair",
	}, http.StatusOK)
	postJSONURL(t, server.Client(), server.URL+"/api/v1/jobs", map[string]any{
		"prd_id":      "prd-public-auto-repair",
		"template_id": "flutter-finance-lite",
	}, http.StatusOK)
	postJSONURL(t, server.Client(), server.URL+"/internal/v1/builders:register", map[string]any{
		"builder_id":        "builder-a",
		"display_name":      "Builder A",
		"capability_tags":   []string{"flutter"},
		"max_parallel_runs": 1,
		"worker_profile": map[string]any{
			"image": "builder:latest",
		},
	}, http.StatusOK)

	started := postJSONURL(t, server.Client(), server.URL+"/api/v1/jobs/job-public-auto-repair:start", map[string]any{}, http.StatusOK)
	if started["status"] != "running_builder" {
		t.Fatalf("status = %v, want running_builder", started["status"])
	}
	completed := waitForJobStatus(t, server.Client(), server.URL+"/api/v1/jobs/job-public-auto-repair", "completed", 5*time.Second)
	builderOutputPath, _ := completed["builder_output_path"].(string)
	if builderOutputPath == "" {
		t.Fatal("builder_output_path should not be empty after async completion")
	}
	builderOutput := requireJSONObjectFile(t, filepath.Join(workspace, "appfactory", filepath.FromSlash(builderOutputPath)))

	roundOutputs, ok := builderOutput["round_outputs"].([]any)
	if !ok || len(roundOutputs) != 1 {
		t.Fatalf("round_outputs = %T %#v, want one round output", builderOutput["round_outputs"], builderOutput["round_outputs"])
	}
	firstRound, ok := roundOutputs[0].(map[string]any)
	if !ok {
		t.Fatalf("round_output[0] = %T, want object", roundOutputs[0])
	}
	builderRuntime, ok := firstRound["builder_runtime_execution"].(map[string]any)
	if !ok {
		t.Fatalf("builder_runtime_execution = %T, want object", firstRound["builder_runtime_execution"])
	}
	if builderRuntime["task_type"] != "analyze_repair" {
		t.Fatalf("task_type = %v, want analyze_repair", builderRuntime["task_type"])
	}
	if attempts, ok := builderRuntime["attempts"].(float64); !ok || attempts < 2 {
		t.Fatalf("attempts = %v, want at least 2", builderRuntime["attempts"])
	}
	validationResults, ok := firstRound["validation_results"].([]any)
	if !ok || len(validationResults) != 1 {
		t.Fatalf("validation_results = %T %#v, want one validation result", firstRound["validation_results"], firstRound["validation_results"])
	}
	validationResult, ok := validationResults[0].(map[string]any)
	if !ok {
		t.Fatalf("validation_result = %T, want object", validationResults[0])
	}
	if validationResult["check_id"] != "check-flutter-analyze" {
		t.Fatalf("check_id = %v, want check-flutter-analyze", validationResult["check_id"])
	}
	checksPassed, ok := builderOutput["checks_passed"].([]any)
	if !ok || len(checksPassed) != 1 {
		t.Fatalf("checks_passed = %#v, want one passed check", builderOutput["checks_passed"])
	}
	workspacePatch, ok := firstRound["workspace_patch"].(map[string]any)
	if !ok {
		t.Fatalf("workspace_patch = %T, want object", firstRound["workspace_patch"])
	}
	if workspacePatch["status"] != "applied" {
		t.Fatalf("workspace_patch.status = %v, want applied", workspacePatch["status"])
	}
	events := getJSONURL(t, server.Client(), server.URL+"/api/v1/jobs/job-public-auto-repair/events", http.StatusOK)
	items, ok := events["items"].([]any)
	if !ok || len(items) == 0 {
		t.Fatalf("events.items = %T %#v, want non-empty array", events["items"], events["items"])
	}
	seenAutoRepairHeartbeat := false
	foundRunCreatedTargets := false
	foundRunCompletedAffected := false
	for _, item := range items {
		event, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if event["type"] == "run_created" {
			targetPaths, ok := event["target_paths"].([]any)
			if ok {
				for _, path := range targetPaths {
					if fmt.Sprint(path) == "lib/main.dart" {
						foundRunCreatedTargets = true
						break
					}
				}
			}
		}
		if event["type"] == "run_completed" {
			affectedPaths, ok := event["affected_paths"].([]any)
			if ok {
				for _, path := range affectedPaths {
					if fmt.Sprint(path) == "lib/main.dart" {
						foundRunCompletedAffected = true
						break
					}
				}
			}
			fileFacts, ok := event["file_facts"].([]any)
			if !ok || len(fileFacts) == 0 {
				t.Fatalf("run_completed file_facts = %v, want non-empty array", event["file_facts"])
			}
			roundSummaries, ok := event["round_summaries"].([]any)
			if ok && len(roundSummaries) > 0 {
				firstRound, ok := roundSummaries[0].(map[string]any)
				if !ok {
					t.Fatalf("round_summaries[0] = %T, want object", roundSummaries[0])
				}
				if firstRound["attempt"] != float64(1) {
					t.Fatalf("round_summaries[0].attempt = %v, want 1", firstRound["attempt"])
				}
				targetPaths, ok := firstRound["target_paths"].([]any)
				if !ok || len(targetPaths) == 0 {
					t.Fatalf("round_summaries[0].target_paths = %v, want non-empty array", firstRound["target_paths"])
				}
				modifiedPaths, ok := firstRound["modified_paths"].([]any)
				if !ok || len(modifiedPaths) == 0 {
					t.Fatalf("round_summaries[0].modified_paths = %v, want non-empty array", firstRound["modified_paths"])
				}
				fileFacts, ok := firstRound["file_facts"].([]any)
				if !ok || len(fileFacts) == 0 {
					t.Fatalf("round_summaries[0].file_facts = %v, want non-empty array", firstRound["file_facts"])
				}
				summary := fmt.Sprint(firstRound["summary"])
				if strings.TrimSpace(summary) == "" {
					t.Fatalf("round_summaries[0].summary = %v, want non-empty string", firstRound["summary"])
				}
			}
		}
		summary := fmt.Sprint(event["summary"])
		if strings.Contains(summary, "builder-runtime auto repair: check-flutter-analyze | task_type=analyze_repair") {
			seenAutoRepairHeartbeat = true
			if attempt, ok := event["attempt"].(float64); !ok || attempt < 2 {
				t.Fatalf("auto repair heartbeat attempt = %v, want at least 2", event["attempt"])
			}
			if event["current_phase"] != "repair" {
				t.Fatalf("auto repair heartbeat current_phase = %v, want repair", event["current_phase"])
			}
			targetPaths, ok := event["target_paths"].([]any)
			if !ok || len(targetPaths) == 0 {
				t.Fatalf("auto repair heartbeat target_paths = %v, want non-empty array", event["target_paths"])
			}
		}
	}
	if !seenAutoRepairHeartbeat {
		t.Fatalf("events = %#v, want auto repair heartbeat", items)
	}
	mainDartPath := filepath.Join(workspace, "appfactory", "jobs", "job-public-auto-repair", "workspace", "lib", "main.dart")
	content, err := os.ReadFile(mainDartPath)
	if err != nil {
		t.Fatalf("ReadFile(lib/main.dart) error = %v", err)
	}
	if !strings.Contains(string(content), "Budget Flow") {
		t.Fatalf("lib/main.dart = %q, want repaired content", string(content))
	}
	if !foundRunCreatedTargets || !foundRunCompletedAffected {
		t.Fatalf("events should expose file facts: run_created_targets=%v run_completed_affected=%v items=%v", foundRunCreatedTargets, foundRunCompletedAffected, items)
	}
	t.Logf("public_jobs_auto_repair_task_type=%v", builderRuntime["task_type"])
	t.Logf("public_jobs_auto_repair_attempts=%v", builderRuntime["attempts"])
	t.Logf("public_jobs_auto_repair_recovered_check=%s", "check-flutter-analyze")
	t.Logf("public_jobs_auto_repair_job_id=%s", "job-public-auto-repair")
	t.Logf("public_jobs_auto_repair_heartbeat=%t", seenAutoRepairHeartbeat)
}

func TestStartPublicJobRejectsWhenApprovalsNotReady(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	cfg := config.DefaultConfig()
	cfg.Workspace = filepath.Join(filepath.Dir(configPath), "workspace")
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	h := NewHandler(configPath)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	server := httptest.NewServer(mux)
	defer server.Close()

	postJSONURL(t, server.Client(), server.URL+"/api/v1/prds:compile", map[string]any{
		"requirement_text": "做一个简单记账 app，不考虑上架，只考虑功能实现，需要首页概览、记一笔和账单列表。",
		"job_id":           "job-start-blocked",
		"prd_id":           "prd-start-blocked",
	}, http.StatusOK)
	postJSONURL(t, server.Client(), server.URL+"/api/v1/jobs", map[string]any{
		"prd_id":      "prd-start-blocked",
		"template_id": "flutter-finance-lite",
	}, http.StatusOK)
	postJSONURL(t, server.Client(), server.URL+"/api/v1/approvals", map[string]any{
		"approval_type": "prd",
		"job_id":        "job-start-blocked",
	}, http.StatusCreated)
	postJSONURL(t, server.Client(), server.URL+"/internal/v1/builders:register", map[string]any{
		"builder_id":   "builder-a",
		"display_name": "Builder A",
		"worker_profile": map[string]any{
			"image": "builder:latest",
		},
	}, http.StatusOK)

	req, err := http.NewRequest(http.MethodPost, server.URL+"/api/v1/jobs/job-start-blocked:start", bytes.NewReader([]byte(`{}`)))
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := server.Client().Do(req)
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusConflict)
	}
	var body internalErrorResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	if body.ErrorCode != "JOB_START_CONFLICT" {
		t.Fatalf("error_code = %q, want JOB_START_CONFLICT", body.ErrorCode)
	}
}

func TestCancelQueuedPublicJobMarksJobCancelled(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	cfg := config.DefaultConfig()
	workspace := filepath.Join(filepath.Dir(configPath), "workspace")
	cfg.Workspace = workspace
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	h := NewHandler(configPath)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	server := httptest.NewServer(mux)
	defer server.Close()

	postJSONURL(t, server.Client(), server.URL+"/api/v1/prds:compile", map[string]any{
		"requirement_text": "做一个简单记账 app，不考虑上架，只考虑功能实现，需要首页概览、记一笔和账单列表。",
		"job_id":           "job-public-cancel-queued",
		"prd_id":           "prd-public-cancel-queued",
	}, http.StatusOK)
	postJSONURL(t, server.Client(), server.URL+"/api/v1/jobs", map[string]any{
		"prd_id":      "prd-public-cancel-queued",
		"template_id": "flutter-finance-lite",
	}, http.StatusOK)
	workspaceDir := filepath.Join(workspace, "appfactory", "jobs", "job-public-cancel-queued", "workspace")
	if err := os.MkdirAll(workspaceDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(workspaceDir) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(workspaceDir, "queued-marker.txt"), []byte("queued-cancel-preserved\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(queued-marker.txt) error = %v", err)
	}

	cancelled := postJSONURL(t, server.Client(), server.URL+"/api/v1/jobs/job-public-cancel-queued:cancel", map[string]any{
		"reason":             "manual stop before execution",
		"preserve_workspace": true,
	}, http.StatusOK)
	if cancelled["status"] != "cancelled" {
		t.Fatalf("status = %v, want cancelled", cancelled["status"])
	}
	if cancelled["phase"] != "terminal" {
		t.Fatalf("phase = %v, want terminal", cancelled["phase"])
	}
	failureContext, ok := cancelled["failure_context"].(map[string]any)
	if !ok {
		t.Fatalf("failure_context = %T, want map[string]any", cancelled["failure_context"])
	}
	if failureContext["last_error_summary"] != "manual stop before execution" {
		t.Fatalf("last_error_summary = %v, want cancel reason", failureContext["last_error_summary"])
	}
	artifacts, ok := cancelled["artifacts"].([]any)
	if !ok || len(artifacts) == 0 {
		t.Fatalf("artifacts = %v, want preserved snapshot path", cancelled["artifacts"])
	}
	snapshotPath, _ := artifacts[len(artifacts)-1].(string)
	if !strings.Contains(snapshotPath, "jobs/job-public-cancel-queued/snapshots/preserved/") {
		t.Fatalf("snapshot path = %q, want preserved snapshot path", snapshotPath)
	}
	markerData, err := os.ReadFile(filepath.Join(workspace, "appfactory", filepath.FromSlash(snapshotPath), "queued-marker.txt"))
	if err != nil {
		t.Fatalf("ReadFile(queued snapshot marker) error = %v", err)
	}
	if strings.TrimSpace(string(markerData)) != "queued-cancel-preserved" {
		t.Fatalf("queued snapshot marker = %q, want queued-cancel-preserved", string(markerData))
	}

	loaded := getJSONURL(t, server.Client(), server.URL+"/api/v1/jobs/job-public-cancel-queued", http.StatusOK)
	if loaded["status"] != "cancelled" {
		t.Fatalf("loaded status = %v, want cancelled", loaded["status"])
	}
	loadedArtifacts, ok := loaded["artifacts"].([]any)
	if !ok || len(loadedArtifacts) == 0 {
		t.Fatalf("loaded artifacts = %v, want preserved snapshot path", loaded["artifacts"])
	}
	if _, ok := loaded["resume_context"]; ok {
		t.Fatalf("resume_context should be omitted for cancelled job, got %v", loaded["resume_context"])
	}
	if _, err := os.Stat(filepath.Join(workspace, "appfactory", "jobs", "job-public-cancel-queued", "job.json")); err != nil {
		t.Fatalf("expected job.json to exist: %v", err)
	}
}

func TestCancelRunningPublicJobCancelsLatestRun(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	cfg := config.DefaultConfig()
	workspace := filepath.Join(filepath.Dir(configPath), "workspace")
	cfg.Workspace = workspace
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	h := NewHandler(configPath)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	server := httptest.NewServer(mux)
	defer server.Close()

	postJSONURL(t, server.Client(), server.URL+"/api/v1/prds:compile", map[string]any{
		"requirement_text": "做一个简单记账 app，不考虑上架，只考虑功能实现，需要首页概览、记一笔和账单列表。",
		"job_id":           "job-public-cancel-running",
		"prd_id":           "prd-public-cancel-running",
	}, http.StatusOK)
	postJSONURL(t, server.Client(), server.URL+"/api/v1/jobs", map[string]any{
		"prd_id":      "prd-public-cancel-running",
		"template_id": "flutter-finance-lite",
	}, http.StatusOK)
	postJSONURL(t, server.Client(), server.URL+"/internal/v1/builders:register", map[string]any{
		"builder_id":        "builder-a",
		"display_name":      "Builder A",
		"capability_tags":   []string{"flutter"},
		"max_parallel_runs": 1,
		"worker_profile": map[string]any{
			"image": "builder:latest",
		},
	}, http.StatusOK)
	allocated := postJSONURL(t, server.Client(), server.URL+"/internal/v1/workers:allocate", map[string]any{
		"job_id": "job-public-cancel-running",
	}, http.StatusOK)
	leaseID, _ := allocated["lease_id"].(string)
	createdRun := postJSONURL(t, server.Client(), server.URL+"/internal/v1/build-runs", map[string]any{
		"worker_id":     "builder-a",
		"lease_id":      leaseID,
		"builder_input": sampleBuildRunInputFor("job-public-cancel-running", "prd-public-cancel-running", "flutter-finance-lite"),
	}, http.StatusOK)
	runID, _ := createdRun["run_id"].(string)
	runWorkspaceDir := filepath.Join(workspace, "appfactory", "jobs", "job-public-cancel-running", "workspace")
	if err := os.WriteFile(filepath.Join(runWorkspaceDir, "running-marker.txt"), []byte("running-cancel-preserved\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(running-marker.txt) error = %v", err)
	}

	cancelled := postJSONURL(t, server.Client(), server.URL+"/api/v1/jobs/job-public-cancel-running:cancel", map[string]any{
		"reason":             "manual stop while builder running",
		"preserve_workspace": true,
	}, http.StatusOK)
	if cancelled["status"] != "cancelled" {
		t.Fatalf("status = %v, want cancelled", cancelled["status"])
	}
	if cancelled["phase"] != "terminal" {
		t.Fatalf("phase = %v, want terminal", cancelled["phase"])
	}
	run := getJSONURL(t, server.Client(), server.URL+"/internal/v1/build-runs/"+runID, http.StatusOK)
	if run["status"] != "cancelled" {
		t.Fatalf("run status = %v, want cancelled", run["status"])
	}
	artifacts, ok := cancelled["artifacts"].([]any)
	if !ok || len(artifacts) == 0 {
		t.Fatalf("artifacts = %v, want preserved snapshot path", cancelled["artifacts"])
	}
	snapshotPath, _ := artifacts[len(artifacts)-1].(string)
	if !strings.Contains(snapshotPath, "jobs/job-public-cancel-running/snapshots/preserved/") {
		t.Fatalf("snapshot path = %q, want preserved snapshot path", snapshotPath)
	}
	markerData, err := os.ReadFile(filepath.Join(workspace, "appfactory", filepath.FromSlash(snapshotPath), "running-marker.txt"))
	if err != nil {
		t.Fatalf("ReadFile(running snapshot marker) error = %v", err)
	}
	if strings.TrimSpace(string(markerData)) != "running-cancel-preserved" {
		t.Fatalf("running snapshot marker = %q, want running-cancel-preserved", string(markerData))
	}
	loaded := getJSONURL(t, server.Client(), server.URL+"/api/v1/jobs/job-public-cancel-running", http.StatusOK)
	if loaded["status"] != "cancelled" {
		t.Fatalf("loaded status = %v, want cancelled", loaded["status"])
	}
	loadedArtifacts, ok := loaded["artifacts"].([]any)
	if !ok || len(loadedArtifacts) == 0 {
		t.Fatalf("loaded artifacts = %v, want preserved snapshot path", loaded["artifacts"])
	}
}

func TestCancelQueuedExecutionMarksExecutionCancelledAndDoesNotReplay(t *testing.T) {
	configPath := filepath.Join(newRetriableTempDir(t), "config.json")
	cfg := config.DefaultConfig()
	workspace := filepath.Join(filepath.Dir(configPath), "workspace")
	cfg.Workspace = workspace
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	h := NewHandler(configPath)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	server := httptest.NewServer(mux)
	defer server.Close()

	postJSONURL(t, server.Client(), server.URL+"/api/v1/prds:compile", map[string]any{
		"requirement_text": "做一个简单记账 app，不考虑上架，只考虑功能实现，需要首页概览、记一笔和账单列表。",
		"job_id":           "job-cancel-queued-execution",
		"prd_id":           "prd-cancel-queued-execution",
	}, http.StatusOK)
	postJSONURL(t, server.Client(), server.URL+"/api/v1/jobs", map[string]any{
		"prd_id":      "prd-cancel-queued-execution",
		"template_id": "flutter-finance-lite",
	}, http.StatusOK)
	postJSONURL(t, server.Client(), server.URL+"/internal/v1/builders:register", map[string]any{
		"builder_id":        "builder-a",
		"display_name":      "Builder A",
		"capability_tags":   []string{"flutter"},
		"max_parallel_runs": 1,
		"worker_profile": map[string]any{
			"image": "builder:latest",
		},
	}, http.StatusOK)

	builderSvc, err := h.buildersControlPlane()
	if err != nil {
		t.Fatalf("buildersControlPlane() error = %v", err)
	}
	runsSvc, err := h.buildRunsControlPlane()
	if err != nil {
		t.Fatalf("buildRunsControlPlane() error = %v", err)
	}
	bundle, err := h.loadPreparedBundleByJobID("job-cancel-queued-execution")
	if err != nil {
		t.Fatalf("loadPreparedBundleByJobID() error = %v", err)
	}
	preparedInput := bundle.BuilderInput
	preparedInput.ContextSourceDir = bundle.PrepareDir
	dispatch, err := builderSvc.Dispatch(context.Background(), appbuilders.Requirement{JobID: "job-cancel-queued-execution"})
	if err != nil {
		t.Fatalf("Dispatch() error = %v", err)
	}
	run, err := runsSvc.Create(context.Background(), dispatch.Node.BuilderID, dispatch.Node.BuilderID, dispatch.Lease.LeaseID, preparedInput)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if err := builderSvc.BindRun(context.Background(), dispatch.Lease.LeaseID, run.RunID); err != nil {
		t.Fatalf("BindRun() error = %v", err)
	}
	if err := h.persistPublicJobExecutionRecord(publicJobExecutionRecord{
		SchemaVersion:  "0.1.0",
		JobID:          "job-cancel-queued-execution",
		RunID:          run.RunID,
		Action:         "start",
		Status:         "queued",
		TimeoutSeconds: 300,
		RequestedAt:    time.Now().UTC().Format(time.RFC3339),
	}); err != nil {
		t.Fatalf("persistPublicJobExecutionRecord() error = %v", err)
	}

	cancelled := postJSONURL(t, server.Client(), server.URL+"/api/v1/jobs/job-cancel-queued-execution:cancel", map[string]any{
		"reason": "cancel queued execution before dispatch",
	}, http.StatusOK)
	if cancelled["status"] != "cancelled" {
		t.Fatalf("status = %v, want cancelled", cancelled["status"])
	}
	execution := readPublicJobExecutionRecord(t, workspace, "job-cancel-queued-execution")
	if execution.Status != "cancelled" {
		t.Fatalf("execution status = %q, want cancelled", execution.Status)
	}
	if execution.FinishedAt == "" {
		t.Fatal("execution finished_at should not be empty after cancel")
	}
	if !strings.Contains(execution.LastError, "cancel queued execution") {
		t.Fatalf("execution last_error = %q, want cancel reason", execution.LastError)
	}
	restarted := NewHandler(configPath)
	t.Cleanup(restarted.waitForAsyncJobs)
	restartedMux := http.NewServeMux()
	restarted.RegisterRoutes(restartedMux)
	restarted.waitForAsyncJobs()
	reloadedExecution := readPublicJobExecutionRecord(t, workspace, "job-cancel-queued-execution")
	if reloadedExecution.Status != "cancelled" {
		t.Fatalf("reloaded execution status = %q, want cancelled", reloadedExecution.Status)
	}
	recoveredRun, err := runsSvc.Get(context.Background(), run.RunID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if recoveredRun.Status != appruns.StatusCancelled {
		t.Fatalf("run status = %s, want cancelled", recoveredRun.Status)
	}
}

func TestCancelRunningExecutionMarksExecutionCancelledAndDoesNotReplay(t *testing.T) {
	configPath := filepath.Join(newRetriableTempDir(t), "config.json")
	cfg := config.DefaultConfig()
	workspace := filepath.Join(filepath.Dir(configPath), "workspace")
	cfg.Workspace = workspace
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	h := NewHandler(configPath)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	server := httptest.NewServer(mux)
	defer server.Close()

	postJSONURL(t, server.Client(), server.URL+"/api/v1/prds:compile", map[string]any{
		"requirement_text": "做一个简单记账 app，不考虑上架，只考虑功能实现，需要首页概览、记一笔和账单列表。",
		"job_id":           "job-cancel-running-execution",
		"prd_id":           "prd-cancel-running-execution",
	}, http.StatusOK)
	postJSONURL(t, server.Client(), server.URL+"/api/v1/jobs", map[string]any{
		"prd_id":      "prd-cancel-running-execution",
		"template_id": "flutter-finance-lite",
	}, http.StatusOK)
	postJSONURL(t, server.Client(), server.URL+"/internal/v1/builders:register", map[string]any{
		"builder_id":        "builder-a",
		"display_name":      "Builder A",
		"capability_tags":   []string{"flutter"},
		"max_parallel_runs": 1,
		"worker_profile": map[string]any{
			"image": "builder:latest",
		},
	}, http.StatusOK)

	builderSvc, err := h.buildersControlPlane()
	if err != nil {
		t.Fatalf("buildersControlPlane() error = %v", err)
	}
	runsSvc, err := h.buildRunsControlPlane()
	if err != nil {
		t.Fatalf("buildRunsControlPlane() error = %v", err)
	}
	bundle, err := h.loadPreparedBundleByJobID("job-cancel-running-execution")
	if err != nil {
		t.Fatalf("loadPreparedBundleByJobID() error = %v", err)
	}
	preparedInput := bundle.BuilderInput
	preparedInput.ContextSourceDir = bundle.PrepareDir
	dispatch, err := builderSvc.Dispatch(context.Background(), appbuilders.Requirement{JobID: "job-cancel-running-execution"})
	if err != nil {
		t.Fatalf("Dispatch() error = %v", err)
	}
	run, err := runsSvc.Create(context.Background(), dispatch.Node.BuilderID, dispatch.Node.BuilderID, dispatch.Lease.LeaseID, preparedInput)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if err := builderSvc.BindRun(context.Background(), dispatch.Lease.LeaseID, run.RunID); err != nil {
		t.Fatalf("BindRun() error = %v", err)
	}
	if err := h.persistPublicJobExecutionRecord(publicJobExecutionRecord{
		SchemaVersion:   "0.1.0",
		JobID:           "job-cancel-running-execution",
		RunID:           run.RunID,
		Action:          "start",
		Status:          "running",
		AttemptCount:    1,
		LeaseOwnerID:    h.orchestratorInstanceID,
		LeaseAcquiredAt: time.Now().UTC().Add(-20 * time.Second).Format(time.RFC3339),
		LeaseExpiresAt:  time.Now().UTC().Add(90 * time.Second).Format(time.RFC3339),
		TimeoutSeconds:  300,
		RequestedAt:     time.Now().UTC().Add(-30 * time.Second).Format(time.RFC3339),
		StartedAt:       time.Now().UTC().Add(-25 * time.Second).Format(time.RFC3339),
		Attempts: []publicJobExecutionAttempt{{
			Attempt:       1,
			OwnerID:       h.orchestratorInstanceID,
			ClaimedAt:     time.Now().UTC().Add(-25 * time.Second).Format(time.RFC3339),
			LastRenewedAt: time.Now().UTC().Add(-10 * time.Second).Format(time.RFC3339),
		}},
	}); err != nil {
		t.Fatalf("persistPublicJobExecutionRecord() error = %v", err)
	}

	cancelled := postJSONURL(t, server.Client(), server.URL+"/api/v1/jobs/job-cancel-running-execution:cancel", map[string]any{
		"reason": "cancel running execution before recovery replay",
	}, http.StatusOK)
	if cancelled["status"] != "cancelled" {
		t.Fatalf("status = %v, want cancelled", cancelled["status"])
	}
	execution := readPublicJobExecutionRecord(t, workspace, "job-cancel-running-execution")
	if execution.Status != "cancelled" {
		t.Fatalf("execution status = %q, want cancelled", execution.Status)
	}
	if execution.LeaseOwnerID != "" {
		t.Fatalf("lease_owner_id = %q, want cleared after cancel", execution.LeaseOwnerID)
	}
	if len(execution.Attempts) != 1 || execution.Attempts[0].ReleaseReason != "cancelled" {
		t.Fatalf("attempt audit = %+v, want single cancelled attempt", execution.Attempts)
	}

	restarted := NewHandler(configPath)
	restarted.EnableStartupOrchestrator()
	t.Cleanup(restarted.waitForAsyncJobs)
	restartedMux := http.NewServeMux()
	restarted.RegisterRoutes(restartedMux)
	restarted.waitForAsyncJobs()

	reloadedExecution := readPublicJobExecutionRecord(t, workspace, "job-cancel-running-execution")
	if reloadedExecution.Status != "cancelled" {
		t.Fatalf("reloaded execution status = %q, want cancelled", reloadedExecution.Status)
	}
	if reloadedExecution.AttemptCount != 1 {
		t.Fatalf("attempt_count = %d, want 1 after restart", reloadedExecution.AttemptCount)
	}
	recoveredRun, err := runsSvc.Get(context.Background(), run.RunID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if recoveredRun.Status != appruns.StatusCancelled {
		t.Fatalf("run status = %s, want cancelled", recoveredRun.Status)
	}
}

func TestCancelRunningPublicJobInterruptsBlockingBuilderRuntimePatch(t *testing.T) {
	configPath := filepath.Join(newRetriableTempDir(t), "config.json")
	cfg := config.DefaultConfig()
	workspace := filepath.Join(filepath.Dir(configPath), "workspace")
	cfg.Workspace = workspace
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	generator := &blockingPublicJobPatchGenerator{
		started:   make(chan struct{}),
		cancelled: make(chan struct{}),
	}

	h := NewHandler(configPath)
	t.Cleanup(h.waitForAsyncJobs)
	h.appFactory.runnerFactory = func(runsSvc runService) *appadapter.Runner {
		runner := appadapter.NewRunnerWithBackend(runServiceRunnerBackend{runsSvc: runsSvc})
		runner.BuilderRuntimeConfig = config.BuilderRuntimeConfig{
			Enabled:      true,
			DefaultModel: &config.AgentModelConfig{Primary: "qwen2.5-coder-14b-local"},
			TaskRoutes: []config.BuilderRuntimeTaskRouteConfig{{
				TaskType: "analyze_repair",
				Model:    &config.AgentModelConfig{Primary: "qwen2.5-coder-14b-local"},
			}},
		}
		runner.PatchGenerator = generator
		runner.Executor = publicJobAutoRepairExecutor{
			checkID:         "check-flutter-analyze",
			label:           "flutter analyze",
			stage:           appruns.StageCheap,
			commands:        []string{"flutter analyze"},
			script:          "grep -q 'Budget Flow' lib/main.dart || { echo 'Analyzing workspace...' >&2; echo '  error • Budget Flow missing • lib/main.dart:1:1 • oneappfactory_test' >&2; echo '1 issue found.' >&2; exit 1; }",
			skipEditCommand: true,
		}
		return runner
	}
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	server := httptest.NewServer(mux)
	defer server.Close()

	postJSONURL(t, server.Client(), server.URL+"/api/v1/prds:compile", map[string]any{
		"requirement_text": "做一个简单记账 app，不考虑上架，只考虑功能实现，需要首页概览、记一笔和账单列表。",
		"job_id":           "job-public-cancel-blocking-builder-runtime",
		"prd_id":           "prd-public-cancel-blocking-builder-runtime",
	}, http.StatusOK)
	postJSONURL(t, server.Client(), server.URL+"/api/v1/jobs", map[string]any{
		"prd_id":      "prd-public-cancel-blocking-builder-runtime",
		"template_id": "flutter-finance-lite",
	}, http.StatusOK)
	postJSONURL(t, server.Client(), server.URL+"/internal/v1/builders:register", map[string]any{
		"builder_id":        "builder-a",
		"display_name":      "Builder A",
		"capability_tags":   []string{"flutter"},
		"max_parallel_runs": 1,
		"worker_profile": map[string]any{
			"image": "builder:latest",
		},
	}, http.StatusOK)

	started := postJSONURL(t, server.Client(), server.URL+"/api/v1/jobs/job-public-cancel-blocking-builder-runtime:start", map[string]any{}, http.StatusOK)
	if started["status"] != "running_builder" {
		t.Fatalf("status = %v, want running_builder", started["status"])
	}

	select {
	case <-generator.started:
	case <-time.After(8 * time.Second):
		t.Fatal("blocking builder-runtime patch generator did not start before timeout")
	}

	cancelled := postJSONURL(t, server.Client(), server.URL+"/api/v1/jobs/job-public-cancel-blocking-builder-runtime:cancel", map[string]any{
		"reason": "cancel blocking builder runtime patch",
	}, http.StatusOK)
	if cancelled["status"] != "cancelled" {
		t.Fatalf("status = %v, want cancelled", cancelled["status"])
	}

	select {
	case <-generator.cancelled:
	case <-time.After(2 * time.Second):
		t.Fatal("blocking builder-runtime patch generator did not observe ctx cancellation")
	}

	finalJob := waitForJobStatus(t, server.Client(), server.URL+"/api/v1/jobs/job-public-cancel-blocking-builder-runtime", "cancelled", 2*time.Second)
	if finalJob["phase"] != "terminal" {
		t.Fatalf("phase = %v, want terminal", finalJob["phase"])
	}
	execution := waitForPublicJobExecutionStatus(t, workspace, "job-public-cancel-blocking-builder-runtime", "cancelled", 2*time.Second)
	if execution.LeaseOwnerID != "" {
		t.Fatalf("lease_owner_id = %q, want cleared after cancel", execution.LeaseOwnerID)
	}

	runsSvc, err := h.buildRunsControlPlane()
	if err != nil {
		t.Fatalf("buildRunsControlPlane() error = %v", err)
	}
	runRecord, err := runsSvc.Get(context.Background(), execution.RunID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if runRecord.Status != appruns.StatusCancelled {
		t.Fatalf("run status = %s, want cancelled", runRecord.Status)
	}
}

func TestResumeFailedPublicJobRunsToCompletion(t *testing.T) {
	configPath := filepath.Join(newRetriableTempDir(t), "config.json")
	cfg := config.DefaultConfig()
	workspace := filepath.Join(filepath.Dir(configPath), "workspace")
	cfg.Workspace = workspace
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	h := NewHandler(configPath)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	server := httptest.NewServer(mux)
	defer server.Close()

	postJSONURL(t, server.Client(), server.URL+"/api/v1/prds:compile", map[string]any{
		"requirement_text": "做一个简单记账 app，不考虑上架，只考虑功能实现，需要首页概览、记一笔和账单列表。",
		"job_id":           "job-public-resume-failed",
		"prd_id":           "prd-public-resume-failed",
	}, http.StatusOK)
	postJSONURL(t, server.Client(), server.URL+"/api/v1/jobs", map[string]any{
		"prd_id":      "prd-public-resume-failed",
		"template_id": "flutter-finance-lite",
	}, http.StatusOK)
	postJSONURL(t, server.Client(), server.URL+"/internal/v1/builders:register", map[string]any{
		"builder_id":        "builder-a",
		"display_name":      "Builder A",
		"capability_tags":   []string{"flutter"},
		"max_parallel_runs": 1,
		"worker_profile": map[string]any{
			"image": "builder:latest",
		},
	}, http.StatusOK)
	allocated := postJSONURL(t, server.Client(), server.URL+"/internal/v1/workers:allocate", map[string]any{
		"job_id": "job-public-resume-failed",
	}, http.StatusOK)
	leaseID, _ := allocated["lease_id"].(string)
	createdRun := postJSONURL(t, server.Client(), server.URL+"/internal/v1/build-runs", map[string]any{
		"worker_id":     "builder-a",
		"lease_id":      leaseID,
		"builder_input": sampleBuildRunInputFor("job-public-resume-failed", "prd-public-resume-failed", "flutter-finance-lite"),
	}, http.StatusOK)
	runID, _ := createdRun["run_id"].(string)
	postJSONURL(t, server.Client(), server.URL+"/internal/v1/build-runs/"+runID+"/fail", map[string]any{
		"summary":             "builder runtime crashed",
		"recovery_suggestion": "retry later",
		"failure_signatures":  []string{"runtime_crash"},
	}, http.StatusOK)

	resumed := postJSONURL(t, server.Client(), server.URL+"/api/v1/jobs/job-public-resume-failed:resume", map[string]any{
		"resume_mode": "retry_failed_run",
		"note":        "manual resume after fixing environment",
	}, http.StatusOK)
	if resumed["status"] != "running_builder" {
		t.Fatalf("status = %v, want running_builder", resumed["status"])
	}
	if resumed["phase"] != "builder" {
		t.Fatalf("phase = %v, want builder", resumed["phase"])
	}
	if value, exists := resumed["builder_output_path"]; exists && value != "" {
		t.Fatalf("builder_output_path = %v, want omitted or empty before async completion", value)
	}
	execution := readPublicJobExecutionRecord(t, workspace, "job-public-resume-failed")
	if execution.Action != "resume" {
		t.Fatalf("execution action = %q, want resume", execution.Action)
	}
	completed := waitForJobStatus(t, server.Client(), server.URL+"/api/v1/jobs/job-public-resume-failed", "completed", 5*time.Second)
	if completed["builder_output_path"] == "" {
		t.Fatal("builder_output_path should not be empty after async resume completion")
	}
	execution = waitForPublicJobExecutionStatus(t, workspace, "job-public-resume-failed", "completed", 5*time.Second)
	if execution.LastError != "" {
		t.Fatalf("execution last_error = %q, want empty", execution.LastError)
	}
	events := getJSONURL(t, server.Client(), server.URL+"/api/v1/jobs/job-public-resume-failed/events", http.StatusOK)
	items, ok := events["items"].([]any)
	if !ok || len(items) == 0 {
		t.Fatalf("items = %v, want non-empty", events["items"])
	}
	runCreatedCount := 0
	for _, item := range items {
		event, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if event["type"] == "run_created" {
			runCreatedCount++
		}
	}
	if runCreatedCount < 2 {
		t.Fatalf("run_created count = %d, want at least 2 after resume", runCreatedCount)
	}
}

func TestGetFailedPublicJobIncludesResumeContext(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	cfg := config.DefaultConfig()
	workspace := filepath.Join(filepath.Dir(configPath), "workspace")
	cfg.Workspace = workspace
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	h := NewHandler(configPath)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	server := httptest.NewServer(mux)
	defer server.Close()

	postJSONURL(t, server.Client(), server.URL+"/api/v1/prds:compile", map[string]any{
		"requirement_text": "做一个简单记账 app，不考虑上架，只考虑功能实现，需要首页概览、记一笔和账单列表。",
		"job_id":           "job-public-resume-context-failed",
		"prd_id":           "prd-public-resume-context-failed",
	}, http.StatusOK)
	postJSONURL(t, server.Client(), server.URL+"/api/v1/jobs", map[string]any{
		"prd_id":      "prd-public-resume-context-failed",
		"template_id": "flutter-finance-lite",
	}, http.StatusOK)
	postJSONURL(t, server.Client(), server.URL+"/internal/v1/builders:register", map[string]any{
		"builder_id":        "builder-a",
		"display_name":      "Builder A",
		"capability_tags":   []string{"flutter"},
		"max_parallel_runs": 1,
		"worker_profile": map[string]any{
			"image": "builder:latest",
		},
	}, http.StatusOK)
	allocated := postJSONURL(t, server.Client(), server.URL+"/internal/v1/workers:allocate", map[string]any{
		"job_id": "job-public-resume-context-failed",
	}, http.StatusOK)
	leaseID, _ := allocated["lease_id"].(string)
	createdRun := postJSONURL(t, server.Client(), server.URL+"/internal/v1/build-runs", map[string]any{
		"worker_id":     "builder-a",
		"lease_id":      leaseID,
		"builder_input": sampleBuildRunInputFor("job-public-resume-context-failed", "prd-public-resume-context-failed", "flutter-finance-lite"),
	}, http.StatusOK)
	runID, _ := createdRun["run_id"].(string)
	postJSONURL(t, server.Client(), server.URL+"/internal/v1/build-runs/"+runID+"/fail", map[string]any{
		"summary":             "builder runtime crashed",
		"recovery_suggestion": "retry later",
		"failure_signatures":  []string{"runtime_crash"},
	}, http.StatusOK)

	outputPath := filepath.Join(workspace, "appfactory", "jobs", "job-public-resume-context-failed", "runs", runID, "builder-output.json")
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	output := sampleBuildRunOutputFor("job-public-resume-context-failed")
	output["status"] = "failed"
	output["final_summary"] = "build failed but workspace preserved"
	output["resume_context"] = map[string]any{
		"resume_allowed":               false,
		"failure_category":             "builder_runtime_failure",
		"recommended_resume_mode":      "resume_from_failure",
		"requires_human_confirmation":  true,
		"requires_preserved_workspace": true,
		"preserved_workspace_path":     "jobs/job-public-resume-context-failed/workspace-preserved",
		"next_action":                  "inspect_failure_then_retry",
	}
	data, err := json.Marshal(output)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	if err := os.WriteFile(outputPath, append(data, '\n'), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	loaded := getJSONURL(t, server.Client(), server.URL+"/api/v1/jobs/job-public-resume-context-failed", http.StatusOK)
	resumeContext, ok := loaded["resume_context"].(map[string]any)
	if !ok {
		t.Fatalf("resume_context = %T, want map[string]any", loaded["resume_context"])
	}
	if resumeContext["resume_allowed"] != false {
		t.Fatalf("resume_allowed = %v, want false", resumeContext["resume_allowed"])
	}
	if resumeContext["preserved_workspace_path"] != "jobs/job-public-resume-context-failed/workspace-preserved" {
		t.Fatalf("preserved_workspace_path = %v, want preserved path from builder output", resumeContext["preserved_workspace_path"])
	}
	if resumeContext["suggested_action"] != "inspect_failure_then_retry" {
		t.Fatalf("suggested_action = %v, want inspect_failure_then_retry", resumeContext["suggested_action"])
	}
	if resumeContext["failure_category"] != "builder_runtime_failure" {
		t.Fatalf("failure_category = %v, want builder_runtime_failure", resumeContext["failure_category"])
	}
	if resumeContext["failure_domain"] != "environment" {
		t.Fatalf("failure_domain = %v, want environment", resumeContext["failure_domain"])
	}
	failureContext, ok := loaded["failure_context"].(map[string]any)
	if !ok {
		t.Fatalf("failure_context = %T, want map[string]any", loaded["failure_context"])
	}
	if failureContext["failure_category"] != "builder_runtime_failure" {
		t.Fatalf("failure_context.failure_category = %v, want builder_runtime_failure", failureContext["failure_category"])
	}
	if failureContext["failure_domain"] != "environment" {
		t.Fatalf("failure_context.failure_domain = %v, want environment", failureContext["failure_domain"])
	}
	if resumeContext["recommended_resume_mode"] != "resume_from_failure" {
		t.Fatalf("recommended_resume_mode = %v, want resume_from_failure", resumeContext["recommended_resume_mode"])
	}
	if resumeContext["requires_human_confirmation"] != true {
		t.Fatalf("requires_human_confirmation = %v, want true", resumeContext["requires_human_confirmation"])
	}
	if resumeContext["requires_preserved_workspace"] != true {
		t.Fatalf("requires_preserved_workspace = %v, want true", resumeContext["requires_preserved_workspace"])
	}
}

func TestGetFailedPublicJobSynthesizesResumeContextWhenOutputMissing(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	cfg := config.DefaultConfig()
	workspace := filepath.Join(filepath.Dir(configPath), "workspace")
	cfg.Workspace = workspace
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	h := NewHandler(configPath)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	server := httptest.NewServer(mux)
	defer server.Close()

	postJSONURL(t, server.Client(), server.URL+"/api/v1/prds:compile", map[string]any{
		"requirement_text": "做一个简单记账 app，不考虑上架，只考虑功能实现，需要首页概览、记一笔和账单列表。",
		"job_id":           "job-public-resume-context-synth",
		"prd_id":           "prd-public-resume-context-synth",
	}, http.StatusOK)
	postJSONURL(t, server.Client(), server.URL+"/api/v1/jobs", map[string]any{
		"prd_id":      "prd-public-resume-context-synth",
		"template_id": "flutter-finance-lite",
	}, http.StatusOK)
	postJSONURL(t, server.Client(), server.URL+"/internal/v1/builders:register", map[string]any{
		"builder_id":        "builder-a",
		"display_name":      "Builder A",
		"capability_tags":   []string{"flutter"},
		"max_parallel_runs": 1,
		"worker_profile": map[string]any{
			"image": "builder:latest",
		},
	}, http.StatusOK)
	allocated := postJSONURL(t, server.Client(), server.URL+"/internal/v1/workers:allocate", map[string]any{
		"job_id": "job-public-resume-context-synth",
	}, http.StatusOK)
	leaseID, _ := allocated["lease_id"].(string)
	createdRun := postJSONURL(t, server.Client(), server.URL+"/internal/v1/build-runs", map[string]any{
		"worker_id":     "builder-a",
		"lease_id":      leaseID,
		"builder_input": sampleBuildRunInputFor("job-public-resume-context-synth", "prd-public-resume-context-synth", "flutter-finance-lite"),
	}, http.StatusOK)
	runID, _ := createdRun["run_id"].(string)
	postJSONURL(t, server.Client(), server.URL+"/internal/v1/build-runs/"+runID+"/fail", map[string]any{
		"summary":             "builder runtime crashed",
		"recovery_suggestion": "retry later",
		"failure_signatures":  []string{"runtime_crash"},
	}, http.StatusOK)

	loaded := getJSONURL(t, server.Client(), server.URL+"/api/v1/jobs/job-public-resume-context-synth", http.StatusOK)
	resumeContext, ok := loaded["resume_context"].(map[string]any)
	if !ok {
		t.Fatalf("resume_context = %T, want map[string]any", loaded["resume_context"])
	}
	if resumeContext["resume_allowed"] != true {
		t.Fatalf("resume_allowed = %v, want true", resumeContext["resume_allowed"])
	}
	if resumeContext["preserved_workspace_path"] != "jobs/job-public-resume-context-synth/workspace" {
		t.Fatalf("preserved_workspace_path = %v, want synthesized workspace path", resumeContext["preserved_workspace_path"])
	}
	if resumeContext["suggested_action"] != "retry_failed_run" {
		t.Fatalf("suggested_action = %v, want retry_failed_run", resumeContext["suggested_action"])
	}
	if resumeContext["failure_category"] != "builder_runtime_failure" {
		t.Fatalf("failure_category = %v, want builder_runtime_failure", resumeContext["failure_category"])
	}
	if resumeContext["failure_domain"] != "environment" {
		t.Fatalf("failure_domain = %v, want environment", resumeContext["failure_domain"])
	}
	failureContext, ok := loaded["failure_context"].(map[string]any)
	if !ok {
		t.Fatalf("failure_context = %T, want map[string]any", loaded["failure_context"])
	}
	if failureContext["failure_category"] != "builder_runtime_failure" {
		t.Fatalf("failure_context.failure_category = %v, want builder_runtime_failure", failureContext["failure_category"])
	}
	if failureContext["failure_domain"] != "environment" {
		t.Fatalf("failure_context.failure_domain = %v, want environment", failureContext["failure_domain"])
	}
	if resumeContext["recommended_resume_mode"] != "retry_failed_run" {
		t.Fatalf("recommended_resume_mode = %v, want retry_failed_run", resumeContext["recommended_resume_mode"])
	}
	if resumeContext["requires_human_confirmation"] != false {
		t.Fatalf("requires_human_confirmation = %v, want false", resumeContext["requires_human_confirmation"])
	}
	if resumeContext["requires_preserved_workspace"] != false {
		t.Fatalf("requires_preserved_workspace = %v, want false", resumeContext["requires_preserved_workspace"])
	}
}

func TestGetFailedPublicJobSynthesizesResumeContextFromRepairContextWhenOutputMissing(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	cfg := config.DefaultConfig()
	workspace := filepath.Join(filepath.Dir(configPath), "workspace")
	cfg.Workspace = workspace
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	h := NewHandler(configPath)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	server := httptest.NewServer(mux)
	defer server.Close()

	postJSONURL(t, server.Client(), server.URL+"/api/v1/prds:compile", map[string]any{
		"requirement_text": "做一个简单记账 app，不考虑上架，只考虑功能实现，需要首页概览、记一笔和账单列表。",
		"job_id":           "job-public-repair-context-synth",
		"prd_id":           "prd-public-repair-context-synth",
	}, http.StatusOK)
	postJSONURL(t, server.Client(), server.URL+"/api/v1/jobs", map[string]any{
		"prd_id":      "prd-public-repair-context-synth",
		"template_id": "flutter-finance-lite",
	}, http.StatusOK)
	postJSONURL(t, server.Client(), server.URL+"/internal/v1/builders:register", map[string]any{
		"builder_id":        "builder-a",
		"display_name":      "Builder A",
		"capability_tags":   []string{"flutter"},
		"max_parallel_runs": 1,
		"worker_profile": map[string]any{
			"image": "builder:latest",
		},
	}, http.StatusOK)
	allocated := postJSONURL(t, server.Client(), server.URL+"/internal/v1/workers:allocate", map[string]any{
		"job_id": "job-public-repair-context-synth",
	}, http.StatusOK)
	leaseID, _ := allocated["lease_id"].(string)
	createdRun := postJSONURL(t, server.Client(), server.URL+"/internal/v1/build-runs", map[string]any{
		"worker_id":     "builder-a",
		"lease_id":      leaseID,
		"builder_input": sampleBuildRunInputFor("job-public-repair-context-synth", "prd-public-repair-context-synth", "flutter-finance-lite"),
	}, http.StatusOK)
	runID, _ := createdRun["run_id"].(string)
	postJSONURL(t, server.Client(), server.URL+"/internal/v1/build-runs/"+runID+"/fail", map[string]any{
		"summary":             "skill direct-edit 轮次没有产出任何可物化的 pending changes",
		"recovery_suggestion": "start the next repair round from the platform runner; do not continue inside the current legacy-builder conversation",
		"failure_signatures":  []string{"skill_direct_edit_noop"},
		"round_state": map[string]any{
			"current_phase":      "finalize",
			"phase_trace":        []string{"inspect", "edit", "repair", "finalize"},
			"next_action":        "retry",
			"preserve_workspace": false,
			"resume_allowed":     true,
		},
		"repair_context": map[string]any{
			"reason":                "skill direct-edit 轮次没有产出任何可物化的 pending changes",
			"failure_signatures":    []string{"skill_direct_edit_noop"},
			"recommended_action":    "start the next repair round from the platform runner; do not continue inside the current legacy-builder conversation",
			"preserve_workspace":    false,
			"requires_human_review": false,
		},
	}, http.StatusOK)

	loaded := getJSONURL(t, server.Client(), server.URL+"/api/v1/jobs/job-public-repair-context-synth", http.StatusOK)
	resumeContext, ok := loaded["resume_context"].(map[string]any)
	if !ok {
		t.Fatalf("resume_context = %T, want map[string]any", loaded["resume_context"])
	}
	if resumeContext["resume_allowed"] != true {
		t.Fatalf("resume_allowed = %v, want true", resumeContext["resume_allowed"])
	}
	if resumeContext["failure_category"] != "skill_direct_edit_noop" {
		t.Fatalf("failure_category = %v, want skill_direct_edit_noop", resumeContext["failure_category"])
	}
	if resumeContext["failure_domain"] != "skill" {
		t.Fatalf("failure_domain = %v, want skill", resumeContext["failure_domain"])
	}
	if resumeContext["recommended_resume_mode"] != "retry_failed_run" {
		t.Fatalf("recommended_resume_mode = %v, want retry_failed_run", resumeContext["recommended_resume_mode"])
	}
	if resumeContext["suggested_action"] != "retry_failed_run" {
		t.Fatalf("suggested_action = %v, want retry_failed_run", resumeContext["suggested_action"])
	}
	if resumeContext["requires_human_confirmation"] != false {
		t.Fatalf("requires_human_confirmation = %v, want false", resumeContext["requires_human_confirmation"])
	}
	if resumeContext["requires_preserved_workspace"] != false {
		t.Fatalf("requires_preserved_workspace = %v, want false", resumeContext["requires_preserved_workspace"])
	}
}

func TestGetFailedPublicJobSynthesizesPatchApplyFailureResumeContextWhenOutputMissing(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	cfg := config.DefaultConfig()
	workspace := filepath.Join(filepath.Dir(configPath), "workspace")
	cfg.Workspace = workspace
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	h := NewHandler(configPath)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	server := httptest.NewServer(mux)
	defer server.Close()

	postJSONURL(t, server.Client(), server.URL+"/api/v1/prds:compile", map[string]any{
		"requirement_text": "做一个简单记账 app，不考虑上架，只考虑功能实现，需要首页概览、记一笔和账单列表。",
		"job_id":           "job-public-patch-apply-failure-synth",
		"prd_id":           "prd-public-patch-apply-failure-synth",
	}, http.StatusOK)
	postJSONURL(t, server.Client(), server.URL+"/api/v1/jobs", map[string]any{
		"prd_id":      "prd-public-patch-apply-failure-synth",
		"template_id": "flutter-finance-lite",
	}, http.StatusOK)
	postJSONURL(t, server.Client(), server.URL+"/internal/v1/builders:register", map[string]any{
		"builder_id":        "builder-a",
		"display_name":      "Builder A",
		"capability_tags":   []string{"flutter"},
		"max_parallel_runs": 1,
		"worker_profile": map[string]any{
			"image": "builder:latest",
		},
	}, http.StatusOK)
	allocated := postJSONURL(t, server.Client(), server.URL+"/internal/v1/workers:allocate", map[string]any{
		"job_id": "job-public-patch-apply-failure-synth",
	}, http.StatusOK)
	leaseID, _ := allocated["lease_id"].(string)
	createdRun := postJSONURL(t, server.Client(), server.URL+"/internal/v1/build-runs", map[string]any{
		"worker_id":     "builder-a",
		"lease_id":      leaseID,
		"builder_input": sampleBuildRunInputFor("job-public-patch-apply-failure-synth", "prd-public-patch-apply-failure-synth", "flutter-finance-lite"),
	}, http.StatusOK)
	runID, _ := createdRun["run_id"].(string)
	postJSONURL(t, server.Client(), server.URL+"/internal/v1/build-runs/"+runID+"/fail", map[string]any{
		"summary":             "workspace patch apply failed: path android/app/build.gradle.kts is protected",
		"recovery_suggestion": "inspect executor-generated changes and allowed/protected path constraints before rerun",
		"failure_signatures":  []string{"workspace_patch_apply_failed"},
		"round_state": map[string]any{
			"current_phase":      "finalize",
			"phase_trace":        []string{"inspect", "edit", "repair", "finalize"},
			"next_action":        "stop",
			"preserve_workspace": false,
			"resume_allowed":     false,
		},
		"repair_context": map[string]any{
			"reason":                "workspace patch apply failed: path android/app/build.gradle.kts is protected",
			"failure_signatures":    []string{"workspace_patch_apply_failed"},
			"recommended_action":    "inspect executor-generated changes and allowed/protected path constraints before rerun",
			"preserve_workspace":    false,
			"requires_human_review": true,
		},
	}, http.StatusOK)

	loaded := getJSONURL(t, server.Client(), server.URL+"/api/v1/jobs/job-public-patch-apply-failure-synth", http.StatusOK)
	resumeContext, ok := loaded["resume_context"].(map[string]any)
	if !ok {
		t.Fatalf("resume_context = %T, want map[string]any", loaded["resume_context"])
	}
	if resumeContext["resume_allowed"] != false {
		t.Fatalf("resume_allowed = %v, want false", resumeContext["resume_allowed"])
	}
	if resumeContext["failure_category"] != "workspace_patch_apply_failed" {
		t.Fatalf("failure_category = %v, want workspace_patch_apply_failed", resumeContext["failure_category"])
	}
	if resumeContext["failure_domain"] != "executor" {
		t.Fatalf("failure_domain = %v, want executor", resumeContext["failure_domain"])
	}
	if resumeContext["recommended_resume_mode"] != "retry_failed_run" {
		t.Fatalf("recommended_resume_mode = %v, want retry_failed_run", resumeContext["recommended_resume_mode"])
	}
	if resumeContext["suggested_action"] != "inspect_failure_and_confirm_resume" {
		t.Fatalf("suggested_action = %v, want inspect_failure_and_confirm_resume", resumeContext["suggested_action"])
	}
	if resumeContext["requires_human_confirmation"] != true {
		t.Fatalf("requires_human_confirmation = %v, want true", resumeContext["requires_human_confirmation"])
	}
	if resumeContext["requires_preserved_workspace"] != false {
		t.Fatalf("requires_preserved_workspace = %v, want false", resumeContext["requires_preserved_workspace"])
	}
	if resumeContext["preserved_workspace_path"] != "jobs/job-public-patch-apply-failure-synth/workspace" {
		t.Fatalf("preserved_workspace_path = %v, want synthesized workspace path", resumeContext["preserved_workspace_path"])
	}
}

func TestGetFailedPublicJobSynthesizesInterruptedResumeContextWhenDispatcherLost(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	cfg := config.DefaultConfig()
	workspace := filepath.Join(filepath.Dir(configPath), "workspace")
	cfg.Workspace = workspace
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	h := NewHandler(configPath)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	server := httptest.NewServer(mux)
	defer server.Close()

	postJSONURL(t, server.Client(), server.URL+"/api/v1/prds:compile", map[string]any{
		"requirement_text": "做一个简单记账 app，不考虑上架，只考虑功能实现，需要首页概览、记一笔和账单列表。",
		"job_id":           "job-public-resume-context-dispatcher-lost",
		"prd_id":           "prd-public-resume-context-dispatcher-lost",
	}, http.StatusOK)
	postJSONURL(t, server.Client(), server.URL+"/api/v1/jobs", map[string]any{
		"prd_id":      "prd-public-resume-context-dispatcher-lost",
		"template_id": "flutter-finance-lite",
	}, http.StatusOK)
	postJSONURL(t, server.Client(), server.URL+"/internal/v1/builders:register", map[string]any{
		"builder_id":        "builder-a",
		"display_name":      "Builder A",
		"capability_tags":   []string{"flutter"},
		"max_parallel_runs": 1,
		"worker_profile": map[string]any{
			"image": "builder:latest",
		},
	}, http.StatusOK)
	allocated := postJSONURL(t, server.Client(), server.URL+"/internal/v1/workers:allocate", map[string]any{
		"job_id": "job-public-resume-context-dispatcher-lost",
	}, http.StatusOK)
	leaseID, _ := allocated["lease_id"].(string)
	createdRun := postJSONURL(t, server.Client(), server.URL+"/internal/v1/build-runs", map[string]any{
		"worker_id":     "builder-a",
		"lease_id":      leaseID,
		"builder_input": sampleBuildRunInputFor("job-public-resume-context-dispatcher-lost", "prd-public-resume-context-dispatcher-lost", "flutter-finance-lite"),
	}, http.StatusOK)
	runID, _ := createdRun["run_id"].(string)
	postJSONURL(t, server.Client(), server.URL+"/internal/v1/build-runs/"+runID+"/fail", map[string]any{
		"summary":             "orchestrator dispatcher stopped unexpectedly; resume required",
		"recovery_suggestion": "retry resume after the API orchestrator is running again",
		"failure_signatures":  []string{"orchestrator_dispatcher_lost"},
	}, http.StatusOK)

	loaded := getJSONURL(t, server.Client(), server.URL+"/api/v1/jobs/job-public-resume-context-dispatcher-lost", http.StatusOK)
	resumeContext, ok := loaded["resume_context"].(map[string]any)
	if !ok {
		t.Fatalf("resume_context = %T, want map[string]any", loaded["resume_context"])
	}
	if resumeContext["resume_allowed"] != true {
		t.Fatalf("resume_allowed = %v, want true", resumeContext["resume_allowed"])
	}
	if resumeContext["suggested_action"] != "resume_interrupted_job" {
		t.Fatalf("suggested_action = %v, want resume_interrupted_job", resumeContext["suggested_action"])
	}
	if resumeContext["preserved_workspace_path"] != "jobs/job-public-resume-context-dispatcher-lost/workspace" {
		t.Fatalf("preserved_workspace_path = %v, want synthesized workspace path", resumeContext["preserved_workspace_path"])
	}
	if resumeContext["failure_category"] != "execution_interrupted" {
		t.Fatalf("failure_category = %v, want execution_interrupted", resumeContext["failure_category"])
	}
	if resumeContext["failure_domain"] != "environment" {
		t.Fatalf("failure_domain = %v, want environment", resumeContext["failure_domain"])
	}
	failureContext, ok := loaded["failure_context"].(map[string]any)
	if !ok {
		t.Fatalf("failure_context = %T, want map[string]any", loaded["failure_context"])
	}
	if failureContext["failure_category"] != "execution_interrupted" {
		t.Fatalf("failure_context.failure_category = %v, want execution_interrupted", failureContext["failure_category"])
	}
	if failureContext["failure_domain"] != "environment" {
		t.Fatalf("failure_context.failure_domain = %v, want environment", failureContext["failure_domain"])
	}
	if resumeContext["recommended_resume_mode"] != "resume_interrupted_job" {
		t.Fatalf("recommended_resume_mode = %v, want resume_interrupted_job", resumeContext["recommended_resume_mode"])
	}
}

func TestGetFailedPublicJobSynthesizesProfileFailureDomainForStructuralCheck(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	cfg := config.DefaultConfig()
	workspace := filepath.Join(filepath.Dir(configPath), "workspace")
	cfg.Workspace = workspace
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	h := NewHandler(configPath)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	server := httptest.NewServer(mux)
	defer server.Close()

	postJSONURL(t, server.Client(), server.URL+"/api/v1/prds:compile", map[string]any{
		"requirement_text": "做一个简单记账 app，不考虑上架，只考虑功能实现，需要首页概览、记一笔和账单列表。",
		"job_id":           "job-public-profile-failure-synth",
		"prd_id":           "prd-public-profile-failure-synth",
	}, http.StatusOK)
	postJSONURL(t, server.Client(), server.URL+"/api/v1/jobs", map[string]any{
		"prd_id":      "prd-public-profile-failure-synth",
		"template_id": "flutter-finance-lite",
	}, http.StatusOK)
	postJSONURL(t, server.Client(), server.URL+"/internal/v1/builders:register", map[string]any{
		"builder_id":        "builder-a",
		"display_name":      "Builder A",
		"capability_tags":   []string{"flutter"},
		"max_parallel_runs": 1,
		"worker_profile": map[string]any{
			"image": "builder:latest",
		},
	}, http.StatusOK)
	allocated := postJSONURL(t, server.Client(), server.URL+"/internal/v1/workers:allocate", map[string]any{
		"job_id": "job-public-profile-failure-synth",
	}, http.StatusOK)
	leaseID, _ := allocated["lease_id"].(string)
	createdRun := postJSONURL(t, server.Client(), server.URL+"/internal/v1/build-runs", map[string]any{
		"worker_id":     "builder-a",
		"lease_id":      leaseID,
		"builder_input": sampleBuildRunInputFor("job-public-profile-failure-synth", "prd-public-profile-failure-synth", "flutter-finance-lite"),
	}, http.StatusOK)
	runID, _ := createdRun["run_id"].(string)
	postJSONURL(t, server.Client(), server.URL+"/internal/v1/build-runs/"+runID+"/fail", map[string]any{
		"summary":             "acceptance check check-counter-demo-removed failed: flutter demo markers still exist",
		"recovery_suggestion": "replace remaining demo markers and rerun the structural checks",
		"failure_signatures":  []string{"profile_check_failed:check-counter-demo-removed"},
	}, http.StatusOK)

	loaded := getJSONURL(t, server.Client(), server.URL+"/api/v1/jobs/job-public-profile-failure-synth", http.StatusOK)
	resumeContext, ok := loaded["resume_context"].(map[string]any)
	if !ok {
		t.Fatalf("resume_context = %T, want map[string]any", loaded["resume_context"])
	}
	if resumeContext["failure_category"] != "profile_check_failed:check-counter-demo-removed" {
		t.Fatalf("failure_category = %v, want profile_check_failed:check-counter-demo-removed", resumeContext["failure_category"])
	}
	if resumeContext["failure_domain"] != "profile" {
		t.Fatalf("failure_domain = %v, want profile", resumeContext["failure_domain"])
	}
}

func TestDerivePublicJobFailureDomainSupportsSkillSignatures(t *testing.T) {
	testCases := []struct {
		name     string
		category string
		want     string
	}{
		{name: "native skill signature", category: "skill_direct_edit_noop", want: "skill"},
		{name: "legacy legacy-builder signature", category: "legacy-builder_direct_edit_noop", want: "skill"},
		{name: "device verification signature", category: "device_check_failed:adb_device_unavailable", want: "device"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if got := derivePublicJobFailureDomain(tc.category); got != tc.want {
				t.Fatalf("derivePublicJobFailureDomain(%q) = %q, want %q", tc.category, got, tc.want)
			}
		})
	}
}

func TestPublicDeviceFailureSuggestedAction(t *testing.T) {
	testCases := []struct {
		name     string
		domain   string
		category string
		want     string
	}{
		{name: "device domain", domain: "device", category: "device_check_failed:adb_device_unavailable", want: "inspect_device_evidence"},
		{name: "device environment category", domain: "environment", category: "environment_check_failed:release_apk_missing", want: "inspect_device_evidence"},
		{name: "other environment category", domain: "environment", category: "runner_exit_nonzero", want: ""},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if got := publicDeviceFailureSuggestedAction(tc.domain, tc.category); got != tc.want {
				t.Fatalf("publicDeviceFailureSuggestedAction(%q, %q) = %q, want %q", tc.domain, tc.category, got, tc.want)
			}
		})
	}
}

func TestPublicExecutionNotificationDescriptorUsesDeviceSuggestedAction(t *testing.T) {
	record := publicJobRecord{
		ResumeContext: &publicJobResumeContext{
			FailureDomain:         "device",
			FailureCategory:       "device_check_failed:app_runtime_crash",
			RecommendedResumeMode: "resume_from_failure",
		},
	}

	notificationType, suggestedAction := publicExecutionNotificationDescriptor(record, publicJobEvent{Type: "execution_failed"})
	if notificationType != "execution_failed" {
		t.Fatalf("notificationType = %q, want execution_failed", notificationType)
	}
	if suggestedAction != "inspect_device_evidence" {
		t.Fatalf("suggestedAction = %q, want inspect_device_evidence", suggestedAction)
	}
}

func TestListPublicJobsSupportsStatusFilter(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	cfg := config.DefaultConfig()
	workspace := filepath.Join(filepath.Dir(configPath), "workspace")
	cfg.Workspace = workspace
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	h := NewHandler(configPath)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	server := httptest.NewServer(mux)
	defer server.Close()

	postJSONURL(t, server.Client(), server.URL+"/api/v1/prds:compile", map[string]any{
		"requirement_text": "做一个简单记账 app，不考虑上架，只考虑功能实现，需要首页概览、记一笔和账单列表。",
		"title":            "运行中记账 App",
		"job_id":           "job-list-running",
		"prd_id":           "prd-list-running",
	}, http.StatusOK)
	postJSONURL(t, server.Client(), server.URL+"/api/v1/jobs", map[string]any{
		"prd_id":      "prd-list-running",
		"template_id": "flutter-finance-lite",
	}, http.StatusOK)

	postJSONURL(t, server.Client(), server.URL+"/api/v1/prds:compile", map[string]any{
		"requirement_text": "做一个简单记账 app，不考虑上架，只考虑功能实现，需要首页概览、记一笔和账单列表。",
		"title":            "已取消记账 App",
		"job_id":           "job-list-cancelled",
		"prd_id":           "prd-list-cancelled",
	}, http.StatusOK)
	postJSONURL(t, server.Client(), server.URL+"/api/v1/jobs", map[string]any{
		"prd_id":      "prd-list-cancelled",
		"template_id": "flutter-finance-lite",
	}, http.StatusOK)
	postJSONURL(t, server.Client(), server.URL+"/api/v1/jobs/job-list-cancelled:cancel", map[string]any{
		"reason": "cancel for list filter test",
	}, http.StatusOK)

	all := getJSONURL(t, server.Client(), server.URL+"/api/v1/jobs", http.StatusOK)
	allItems, ok := all["items"].([]any)
	if !ok || len(allItems) < 2 {
		t.Fatalf("items = %v, want at least 2", all["items"])
	}

	cancelledOnly := getJSONURL(t, server.Client(), server.URL+"/api/v1/jobs?status=cancelled", http.StatusOK)
	cancelledItems, ok := cancelledOnly["items"].([]any)
	if !ok || len(cancelledItems) != 1 {
		t.Fatalf("cancelled items = %v, want exactly 1", cancelledOnly["items"])
	}
	record, ok := cancelledItems[0].(map[string]any)
	if !ok {
		t.Fatalf("cancelled item = %T, want map[string]any", cancelledItems[0])
	}
	if record["job_id"] != "job-list-cancelled" {
		t.Fatalf("job_id = %v, want job-list-cancelled", record["job_id"])
	}
	if record["title"] != "已取消记账 App" {
		t.Fatalf("title = %v, want 已取消记账 App", record["title"])
	}
}

func TestPublicJobsTolerateLegacyPreparedExecutionContract(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	cfg := config.DefaultConfig()
	workspace := filepath.Join(filepath.Dir(configPath), "workspace")
	cfg.Workspace = workspace
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	h := NewHandler(configPath)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	server := httptest.NewServer(mux)
	defer server.Close()

	postJSONURL(t, server.Client(), server.URL+"/api/v1/prds:compile", map[string]any{
		"requirement_text": "做一个简单待办 app，不要首页概览，也不要详情页，只要列表和编辑入口。",
		"title":            "Legacy Execution Contract Job",
		"job_id":           "job-public-legacy-contract",
		"prd_id":           "prd-public-legacy-contract",
	}, http.StatusOK)
	postJSONURL(t, server.Client(), server.URL+"/api/v1/jobs", map[string]any{
		"prd_id":      "prd-public-legacy-contract",
		"template_id": "flutter-open-lite",
	}, http.StatusOK)

	prdPath := filepath.Join(workspace, "appfactory", "jobs", "job-public-legacy-contract", "prepare", "PRD.json")
	prdData, err := os.ReadFile(prdPath)
	if err != nil {
		t.Fatalf("ReadFile(PRD.json) error = %v", err)
	}
	legacyPRDData := bytes.Replace(prdData, []byte(`"surface_contracts"`), []byte(`"page_contracts"`), 1)
	if bytes.Equal(legacyPRDData, prdData) {
		t.Fatal("expected prepared PRD to contain surface_contracts before legacy mutation")
	}
	if err := os.WriteFile(prdPath, legacyPRDData, 0o644); err != nil {
		t.Fatalf("WriteFile(PRD.json) error = %v", err)
	}

	listed := getJSONURL(t, server.Client(), server.URL+"/api/v1/jobs", http.StatusOK)
	items, ok := listed["items"].([]any)
	if !ok || len(items) == 0 {
		t.Fatalf("items = %v, want non-empty job list", listed["items"])
	}
	found := false
	for _, item := range items {
		record, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if record["job_id"] == "job-public-legacy-contract" {
			found = true
			if record["title"] != "Legacy Execution Contract Job" {
				t.Fatalf("title = %v, want Legacy Execution Contract Job", record["title"])
			}
		}
	}
	if !found {
		t.Fatalf("legacy job not found in list: %v", items)
	}

	loaded := getJSONURL(t, server.Client(), server.URL+"/api/v1/jobs/job-public-legacy-contract", http.StatusOK)
	if loaded["job_id"] != "job-public-legacy-contract" {
		t.Fatalf("job_id = %v, want job-public-legacy-contract", loaded["job_id"])
	}
	if loaded["prd_id"] != "prd-public-legacy-contract" {
		t.Fatalf("prd_id = %v, want prd-public-legacy-contract", loaded["prd_id"])
	}
}

func TestStartPublicJobReturnsConflictWhenExecutionAlreadyActive(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	cfg := config.DefaultConfig()
	workspace := filepath.Join(filepath.Dir(configPath), "workspace")
	cfg.Workspace = workspace
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	h := NewHandler(configPath)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	server := httptest.NewServer(mux)
	defer server.Close()

	postJSONURL(t, server.Client(), server.URL+"/api/v1/prds:compile", map[string]any{
		"requirement_text": "做一个简单记账 app，不考虑上架，只考虑功能实现，需要首页概览、记一笔和账单列表。",
		"job_id":           "job-start-active-execution",
		"prd_id":           "prd-start-active-execution",
	}, http.StatusOK)
	postJSONURL(t, server.Client(), server.URL+"/api/v1/jobs", map[string]any{
		"prd_id":      "prd-start-active-execution",
		"template_id": "flutter-finance-lite",
	}, http.StatusOK)
	if err := h.persistPublicJobExecutionRecord(publicJobExecutionRecord{
		SchemaVersion:  "0.1.0",
		JobID:          "job-start-active-execution",
		RunID:          "run-active-start",
		Action:         "start",
		Status:         "queued",
		TimeoutSeconds: 300,
		RequestedAt:    time.Now().UTC().Format(time.RFC3339),
	}); err != nil {
		t.Fatalf("persistPublicJobExecutionRecord() error = %v", err)
	}
	postJSONURL(t, server.Client(), server.URL+"/internal/v1/builders:register", map[string]any{
		"builder_id":   "builder-a",
		"display_name": "Builder A",
		"worker_profile": map[string]any{
			"image": "builder:latest",
		},
	}, http.StatusOK)

	req, err := http.NewRequest(http.MethodPost, server.URL+"/api/v1/jobs/job-start-active-execution:start", bytes.NewReader([]byte(`{}`)))
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := server.Client().Do(req)
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusConflict)
	}
	var body internalErrorResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	if body.ErrorCode != "JOB_START_CONFLICT" {
		t.Fatalf("error_code = %q, want JOB_START_CONFLICT", body.ErrorCode)
	}
}

func TestResumeFailedPublicJobReturnsConflictWhenResumeDisallowed(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	cfg := config.DefaultConfig()
	workspace := filepath.Join(filepath.Dir(configPath), "workspace")
	cfg.Workspace = workspace
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	h := NewHandler(configPath)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	server := httptest.NewServer(mux)
	defer server.Close()

	postJSONURL(t, server.Client(), server.URL+"/api/v1/prds:compile", map[string]any{
		"requirement_text": "做一个简单记账 app，不考虑上架，只考虑功能实现，需要首页概览、记一笔和账单列表。",
		"job_id":           "job-public-resume-disallowed",
		"prd_id":           "prd-public-resume-disallowed",
	}, http.StatusOK)
	postJSONURL(t, server.Client(), server.URL+"/api/v1/jobs", map[string]any{
		"prd_id":      "prd-public-resume-disallowed",
		"template_id": "flutter-finance-lite",
	}, http.StatusOK)
	postJSONURL(t, server.Client(), server.URL+"/internal/v1/builders:register", map[string]any{
		"builder_id":        "builder-a",
		"display_name":      "Builder A",
		"capability_tags":   []string{"flutter"},
		"max_parallel_runs": 1,
		"worker_profile": map[string]any{
			"image": "builder:latest",
		},
	}, http.StatusOK)
	allocated := postJSONURL(t, server.Client(), server.URL+"/internal/v1/workers:allocate", map[string]any{
		"job_id": "job-public-resume-disallowed",
	}, http.StatusOK)
	leaseID, _ := allocated["lease_id"].(string)
	createdRun := postJSONURL(t, server.Client(), server.URL+"/internal/v1/build-runs", map[string]any{
		"worker_id":     "builder-a",
		"lease_id":      leaseID,
		"builder_input": sampleBuildRunInputFor("job-public-resume-disallowed", "prd-public-resume-disallowed", "flutter-finance-lite"),
	}, http.StatusOK)
	runID, _ := createdRun["run_id"].(string)
	postJSONURL(t, server.Client(), server.URL+"/internal/v1/build-runs/"+runID+"/fail", map[string]any{
		"summary":             "builder runtime crashed",
		"recovery_suggestion": "retry later",
		"failure_signatures":  []string{"runtime_crash"},
	}, http.StatusOK)

	outputPath := filepath.Join(workspace, "appfactory", "jobs", "job-public-resume-disallowed", "runs", runID, "builder-output.json")
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	output := sampleBuildRunOutputFor("job-public-resume-disallowed")
	output["status"] = "failed"
	output["final_summary"] = "build failed and needs manual inspection"
	output["resume_context"] = map[string]any{
		"resume_allowed": false,
		"next_action":    "inspect_failure_then_retry",
	}
	data, err := json.Marshal(output)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	if err := os.WriteFile(outputPath, append(data, '\n'), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	req, err := http.NewRequest(http.MethodPost, server.URL+"/api/v1/jobs/job-public-resume-disallowed:resume", bytes.NewReader([]byte(`{"resume_mode":"retry_failed_run"}`)))
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := server.Client().Do(req)
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusConflict)
	}
	var body internalErrorResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	if body.ErrorCode != "JOB_RESUME_CONFLICT" {
		t.Fatalf("error_code = %q, want JOB_RESUME_CONFLICT", body.ErrorCode)
	}
}

func TestResumeFailedPublicJobReturnsConflictWhenExecutionAlreadyActive(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	cfg := config.DefaultConfig()
	workspace := filepath.Join(filepath.Dir(configPath), "workspace")
	cfg.Workspace = workspace
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	h := NewHandler(configPath)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	server := httptest.NewServer(mux)
	defer server.Close()

	postJSONURL(t, server.Client(), server.URL+"/api/v1/prds:compile", map[string]any{
		"requirement_text": "做一个简单记账 app，不考虑上架，只考虑功能实现，需要首页概览、记一笔和账单列表。",
		"job_id":           "job-public-resume-active-execution",
		"prd_id":           "prd-public-resume-active-execution",
	}, http.StatusOK)
	postJSONURL(t, server.Client(), server.URL+"/api/v1/jobs", map[string]any{
		"prd_id":      "prd-public-resume-active-execution",
		"template_id": "flutter-finance-lite",
	}, http.StatusOK)
	if err := h.persistPublicJobExecutionRecord(publicJobExecutionRecord{
		SchemaVersion:   "0.1.0",
		JobID:           "job-public-resume-active-execution",
		RunID:           "run-resume-active-execution",
		Action:          "resume",
		Status:          "running",
		TimeoutSeconds:  300,
		RequestedAt:     time.Now().UTC().Add(-30 * time.Second).Format(time.RFC3339),
		StartedAt:       time.Now().UTC().Add(-25 * time.Second).Format(time.RFC3339),
		LeaseOwnerID:    h.orchestratorInstanceID,
		LeaseAcquiredAt: time.Now().UTC().Add(-25 * time.Second).Format(time.RFC3339),
		LeaseExpiresAt:  time.Now().UTC().Add(90 * time.Second).Format(time.RFC3339),
	}); err != nil {
		t.Fatalf("persistPublicJobExecutionRecord() error = %v", err)
	}

	req, err := http.NewRequest(http.MethodPost, server.URL+"/api/v1/jobs/job-public-resume-active-execution:resume", bytes.NewReader([]byte(`{"resume_mode":"retry_failed_run"}`)))
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := server.Client().Do(req)
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusConflict)
	}
	var body internalErrorResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	if body.ErrorCode != "JOB_RESUME_CONFLICT" {
		t.Fatalf("error_code = %q, want JOB_RESUME_CONFLICT", body.ErrorCode)
	}
}

func TestResumeFailedPublicJobRejectsWhenApprovedTemplateVersionNoLongerMatchesPreparedBundle(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	cfg := config.DefaultConfig()
	workspace := filepath.Join(filepath.Dir(configPath), "workspace")
	cfg.Workspace = workspace
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	h := NewHandler(configPath)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	server := httptest.NewServer(mux)
	defer server.Close()

	postJSONURL(t, server.Client(), server.URL+"/api/v1/prds:compile", map[string]any{
		"requirement_text": "做一个简单记账 app，不考虑上架，只考虑功能实现，需要首页概览、记一笔和账单列表。",
		"job_id":           "job-public-resume-template-version-drift",
		"prd_id":           "prd-public-resume-template-version-drift",
	}, http.StatusOK)
	postJSONURL(t, server.Client(), server.URL+"/api/v1/jobs", map[string]any{
		"prd_id":      "prd-public-resume-template-version-drift",
		"template_id": "flutter-finance-lite",
	}, http.StatusOK)
	postJSONURL(t, server.Client(), server.URL+"/internal/v1/builders:register", map[string]any{
		"builder_id":        "builder-a",
		"display_name":      "Builder A",
		"capability_tags":   []string{"flutter"},
		"max_parallel_runs": 1,
		"worker_profile": map[string]any{
			"image": "builder:latest",
		},
	}, http.StatusOK)
	allocated := postJSONURL(t, server.Client(), server.URL+"/internal/v1/workers:allocate", map[string]any{
		"job_id": "job-public-resume-template-version-drift",
	}, http.StatusOK)
	leaseID, _ := allocated["lease_id"].(string)
	createdRun := postJSONURL(t, server.Client(), server.URL+"/internal/v1/build-runs", map[string]any{
		"worker_id":     "builder-a",
		"lease_id":      leaseID,
		"builder_input": sampleBuildRunInputFor("job-public-resume-template-version-drift", "prd-public-resume-template-version-drift", "flutter-finance-lite"),
	}, http.StatusOK)
	runID, _ := createdRun["run_id"].(string)
	postJSONURL(t, server.Client(), server.URL+"/internal/v1/build-runs/"+runID+"/fail", map[string]any{
		"summary":             "builder runtime crashed",
		"recovery_suggestion": "retry later",
		"failure_signatures":  []string{"runtime_crash"},
	}, http.StatusOK)

	outputPath := filepath.Join(workspace, "appfactory", "jobs", "job-public-resume-template-version-drift", "runs", runID, "builder-output.json")
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	output := sampleBuildRunOutputFor("job-public-resume-template-version-drift")
	output["status"] = "failed"
	output["resume_context"] = map[string]any{
		"resume_allowed":          true,
		"recommended_resume_mode": "retry_failed_run",
		"next_action":             "retry_failed_run",
	}
	data, err := json.Marshal(output)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	if err := os.WriteFile(outputPath, append(data, '\n'), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	builderInputPath := filepath.Join(workspace, "appfactory", "jobs", "job-public-resume-template-version-drift", "prepare", "builder-input.json")
	builderInputData, err := os.ReadFile(builderInputPath)
	if err != nil {
		t.Fatalf("ReadFile(builder-input.json) error = %v", err)
	}
	var builderInput map[string]any
	if err := json.Unmarshal(builderInputData, &builderInput); err != nil {
		t.Fatalf("Unmarshal(builder-input.json) error = %v", err)
	}
	builderInput["template_id"] = "flutter-open-lite"
	updatedBuilderInput, err := json.MarshalIndent(builderInput, "", "  ")
	if err != nil {
		t.Fatalf("Marshal(builder-input.json) error = %v", err)
	}
	if err := os.WriteFile(builderInputPath, append(updatedBuilderInput, '\n'), 0o644); err != nil {
		t.Fatalf("WriteFile(builder-input.json) error = %v", err)
	}

	req, err := http.NewRequest(http.MethodPost, server.URL+"/api/v1/jobs/job-public-resume-template-version-drift:resume", bytes.NewReader([]byte(`{"resume_mode":"retry_failed_run"}`)))
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := server.Client().Do(req)
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusConflict)
	}
	var body internalErrorResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	if body.ErrorCode != "JOB_RESUME_CONFLICT" {
		t.Fatalf("error_code = %q, want JOB_RESUME_CONFLICT", body.ErrorCode)
	}
	if !strings.Contains(body.Message, "approval snapshot conflict") || !strings.Contains(body.Message, "template approval subject_version mismatch") || !strings.Contains(body.Message, "selected-template@flutter-open-lite@v0.1.0@sha256:") || !strings.Contains(body.Message, "selected-template@flutter-finance-lite@v0.1.0@sha256:") {
		t.Fatalf("message = %q, want template subject_version drift conflict", body.Message)
	}
	loaded := getJSONURL(t, server.Client(), server.URL+"/api/v1/jobs/job-public-resume-template-version-drift", http.StatusOK)
	if loaded["template_id"] != "flutter-open-lite" {
		t.Fatalf("template_id = %v, want flutter-open-lite after prepared bundle drift", loaded["template_id"])
	}
	if loaded["status"] != "awaiting_template_approval" {
		t.Fatalf("status = %v, want awaiting_template_approval after template approval drift", loaded["status"])
	}
	if loaded["phase"] != "approval" {
		t.Fatalf("phase = %v, want approval after template approval drift", loaded["phase"])
	}
	statusContext, ok := loaded["status_context"].(map[string]any)
	if !ok {
		t.Fatalf("status_context = %T, want map[string]any", loaded["status_context"])
	}
	if statusContext["reason_code"] != "template_approval_drift" {
		t.Fatalf("reason_code = %v, want template_approval_drift", statusContext["reason_code"])
	}
	if statusContext["suggested_action"] != "submit_template_approval" {
		t.Fatalf("suggested_action = %v, want submit_template_approval", statusContext["suggested_action"])
	}
	if _, exists := loaded["resume_context"]; exists {
		t.Fatalf("resume_context should be omitted after template approval drift, got %v", loaded["resume_context"])
	}
}

func TestSubmitTemplateApprovalPromotesFreshStartAfterTemplateSelectionChange(t *testing.T) {
	configPath := filepath.Join(newRetriableTempDir(t), "config.json")
	cfg := config.DefaultConfig()
	workspace := filepath.Join(filepath.Dir(configPath), "workspace")
	cfg.Workspace = workspace
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	h := NewHandler(configPath)
	t.Cleanup(h.waitForAsyncJobs)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	server := httptest.NewServer(mux)
	defer server.Close()

	postJSONURL(t, server.Client(), server.URL+"/api/v1/prds:compile", map[string]any{
		"requirement_text": "做一个简单记账 app，不考虑上架，只考虑功能实现，需要首页概览、记一笔和账单列表。",
		"job_id":           "job-public-template-reapproval",
		"prd_id":           "prd-public-template-reapproval",
	}, http.StatusOK)
	postJSONURL(t, server.Client(), server.URL+"/api/v1/jobs", map[string]any{
		"prd_id":      "prd-public-template-reapproval",
		"template_id": "flutter-finance-lite",
	}, http.StatusOK)
	postJSONURL(t, server.Client(), server.URL+"/internal/v1/builders:register", map[string]any{
		"builder_id":        "builder-a",
		"display_name":      "Builder A",
		"capability_tags":   []string{"flutter"},
		"max_parallel_runs": 1,
		"worker_profile": map[string]any{
			"image": "builder:latest",
		},
	}, http.StatusOK)
	allocated := postJSONURL(t, server.Client(), server.URL+"/internal/v1/workers:allocate", map[string]any{
		"job_id": "job-public-template-reapproval",
	}, http.StatusOK)
	leaseID, _ := allocated["lease_id"].(string)
	createdRun := postJSONURL(t, server.Client(), server.URL+"/internal/v1/build-runs", map[string]any{
		"worker_id":     "builder-a",
		"lease_id":      leaseID,
		"builder_input": sampleBuildRunInputFor("job-public-template-reapproval", "prd-public-template-reapproval", "flutter-finance-lite"),
	}, http.StatusOK)
	runID, _ := createdRun["run_id"].(string)
	postJSONURL(t, server.Client(), server.URL+"/internal/v1/build-runs/"+runID+"/fail", map[string]any{
		"summary":             "builder runtime crashed",
		"recovery_suggestion": "retry later",
		"failure_signatures":  []string{"runtime_crash"},
	}, http.StatusOK)

	outputPath := filepath.Join(workspace, "appfactory", "jobs", "job-public-template-reapproval", "runs", runID, "builder-output.json")
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	output := sampleBuildRunOutputFor("job-public-template-reapproval")
	output["status"] = "failed"
	output["resume_context"] = map[string]any{
		"resume_allowed":          true,
		"recommended_resume_mode": "retry_failed_run",
		"next_action":             "retry_failed_run",
	}
	data, err := json.Marshal(output)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	if err := os.WriteFile(outputPath, append(data, '\n'), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	builderInputPath := filepath.Join(workspace, "appfactory", "jobs", "job-public-template-reapproval", "prepare", "builder-input.json")
	builderInputData, err := os.ReadFile(builderInputPath)
	if err != nil {
		t.Fatalf("ReadFile(builder-input.json) error = %v", err)
	}
	var builderInput map[string]any
	if err := json.Unmarshal(builderInputData, &builderInput); err != nil {
		t.Fatalf("Unmarshal(builder-input.json) error = %v", err)
	}
	builderInput["template_id"] = "flutter-open-lite"
	updatedBuilderInput, err := json.MarshalIndent(builderInput, "", "  ")
	if err != nil {
		t.Fatalf("Marshal(builder-input.json) error = %v", err)
	}
	if err := os.WriteFile(builderInputPath, append(updatedBuilderInput, '\n'), 0o644); err != nil {
		t.Fatalf("WriteFile(builder-input.json) error = %v", err)
	}

	req, err := http.NewRequest(http.MethodPost, server.URL+"/api/v1/jobs/job-public-template-reapproval:resume", bytes.NewReader([]byte(`{"resume_mode":"retry_failed_run"}`)))
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := server.Client().Do(req)
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusConflict)
	}

	reapproval := postJSONURL(t, server.Client(), server.URL+"/api/v1/templates/flutter-open-lite:submit-approval", map[string]any{
		"job_id":  "job-public-template-reapproval",
		"prd_id":  "prd-public-template-reapproval",
		"summary": "确认切换到 flutter-open-lite 后可以继续恢复。",
	}, http.StatusOK)
	if subjectVersion, _ := reapproval["subject_version"].(string); !strings.HasPrefix(subjectVersion, "selected-template@flutter-open-lite@v0.1.0@sha256:") {
		t.Fatalf("subject_version = %v, want content-bound template approval version after template reapproval", reapproval["subject_version"])
	}

	loaded := getJSONURL(t, server.Client(), server.URL+"/api/v1/jobs/job-public-template-reapproval", http.StatusOK)
	if loaded["status"] != "queued" {
		t.Fatalf("status = %v, want queued after template reapproval promotes fresh start semantics", loaded["status"])
	}
	if loaded["phase"] != "builder" {
		t.Fatalf("phase = %v, want builder after template reapproval promotes fresh start semantics", loaded["phase"])
	}
	if _, ok := loaded["resume_context"]; ok {
		t.Fatalf("resume_context should be omitted after template selection changes current builder-input, got %v", loaded["resume_context"])
	}
	statusContext, ok := loaded["status_context"].(map[string]any)
	if !ok {
		t.Fatalf("status_context = %T, want map[string]any", loaded["status_context"])
	}
	if statusContext["reason_code"] != "prepared_input_stale" {
		t.Fatalf("reason_code = %v, want prepared_input_stale", statusContext["reason_code"])
	}
	if statusContext["suggested_action"] != "start" {
		t.Fatalf("suggested_action = %v, want start", statusContext["suggested_action"])
	}

	started := postJSONURL(t, server.Client(), server.URL+"/api/v1/jobs/job-public-template-reapproval:start", map[string]any{}, http.StatusOK)
	if started["status"] != "queued" && started["status"] != "running_builder" {
		t.Fatalf("status = %v, want queued or running_builder after template reapproval fresh start", started["status"])
	}
	if started["phase"] != "builder" {
		t.Fatalf("phase = %v, want builder after template reapproval fresh start", started["phase"])
	}
}

func TestSubmitTemplateApprovalRequiresPrepareRecompileAfterTemplateSourceVersionDrift(t *testing.T) {
	configPath := filepath.Join(newRetriableTempDir(t), "config.json")
	cfg := config.DefaultConfig()
	workspace := filepath.Join(filepath.Dir(configPath), "workspace")
	cfg.Workspace = workspace
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	h := NewHandler(configPath)
	t.Cleanup(h.waitForAsyncJobs)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	server := httptest.NewServer(mux)
	defer server.Close()

	postJSONURL(t, server.Client(), server.URL+"/api/v1/prds:compile", map[string]any{
		"requirement_text": "做一个简单记账 app，不考虑上架，只考虑功能实现，需要首页概览、记一笔和账单列表。",
		"job_id":           "job-public-template-source-reapproval",
		"prd_id":           "prd-public-template-source-reapproval",
	}, http.StatusOK)
	postJSONURL(t, server.Client(), server.URL+"/api/v1/jobs", map[string]any{
		"prd_id":      "prd-public-template-source-reapproval",
		"template_id": "flutter-finance-lite",
	}, http.StatusOK)
	postJSONURL(t, server.Client(), server.URL+"/internal/v1/builders:register", map[string]any{
		"builder_id":        "builder-a",
		"display_name":      "Builder A",
		"capability_tags":   []string{"flutter"},
		"max_parallel_runs": 1,
		"worker_profile": map[string]any{
			"image": "builder:latest",
		},
	}, http.StatusOK)
	allocated := postJSONURL(t, server.Client(), server.URL+"/internal/v1/workers:allocate", map[string]any{
		"job_id": "job-public-template-source-reapproval",
	}, http.StatusOK)
	leaseID, _ := allocated["lease_id"].(string)
	createdRun := postJSONURL(t, server.Client(), server.URL+"/internal/v1/build-runs", map[string]any{
		"worker_id":     "builder-a",
		"lease_id":      leaseID,
		"builder_input": sampleBuildRunInputFor("job-public-template-source-reapproval", "prd-public-template-source-reapproval", "flutter-finance-lite"),
	}, http.StatusOK)
	runID, _ := createdRun["run_id"].(string)
	postJSONURL(t, server.Client(), server.URL+"/internal/v1/build-runs/"+runID+"/fail", map[string]any{
		"summary":             "builder runtime crashed",
		"recovery_suggestion": "retry later",
		"failure_signatures":  []string{"runtime_crash"},
	}, http.StatusOK)

	outputPath := filepath.Join(workspace, "appfactory", "jobs", "job-public-template-source-reapproval", "runs", runID, "builder-output.json")
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	output := sampleBuildRunOutputFor("job-public-template-source-reapproval")
	output["status"] = "failed"
	output["resume_context"] = map[string]any{
		"resume_allowed":          true,
		"recommended_resume_mode": "retry_failed_run",
		"next_action":             "retry_failed_run",
	}
	data, err := json.Marshal(output)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	if err := os.WriteFile(outputPath, append(data, '\n'), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	prepareDir := filepath.Join(workspace, "appfactory", "jobs", "job-public-template-source-reapproval", "prepare")
	templateApprovalPath := filepath.Join(prepareDir, appruns.TemplateApprovalFileName)
	templateApprovalData, err := os.ReadFile(templateApprovalPath)
	if err != nil {
		t.Fatalf("ReadFile(template-approval.json) error = %v", err)
	}
	var templateApproval map[string]any
	if err := json.Unmarshal(templateApprovalData, &templateApproval); err != nil {
		t.Fatalf("Unmarshal(template-approval.json) error = %v", err)
	}
	templateApproval["subject_version"] = "selected-template@flutter-finance-lite@v0.0.9"
	updatedTemplateApproval, err := json.MarshalIndent(templateApproval, "", "  ")
	if err != nil {
		t.Fatalf("Marshal(template-approval.json) error = %v", err)
	}
	if err := os.WriteFile(templateApprovalPath, append(updatedTemplateApproval, '\n'), 0o644); err != nil {
		t.Fatalf("WriteFile(template-approval.json) error = %v", err)
	}

	builderInputPath := filepath.Join(prepareDir, "builder-input.json")
	builderInputData, err := os.ReadFile(builderInputPath)
	if err != nil {
		t.Fatalf("ReadFile(builder-input.json) error = %v", err)
	}
	var builderInput map[string]any
	if err := json.Unmarshal(builderInputData, &builderInput); err != nil {
		t.Fatalf("Unmarshal(builder-input.json) error = %v", err)
	}
	builderInput["prepared_template_subject_version"] = "selected-template@flutter-finance-lite@v0.0.9"
	updatedBuilderInput, err := json.MarshalIndent(builderInput, "", "  ")
	if err != nil {
		t.Fatalf("Marshal(builder-input.json) error = %v", err)
	}
	if err := os.WriteFile(builderInputPath, append(updatedBuilderInput, '\n'), 0o644); err != nil {
		t.Fatalf("WriteFile(builder-input.json) error = %v", err)
	}

	loaded := getJSONURL(t, server.Client(), server.URL+"/api/v1/jobs/job-public-template-source-reapproval", http.StatusOK)
	if loaded["status"] != "awaiting_template_approval" {
		t.Fatalf("status = %v, want awaiting_template_approval after template approval drift", loaded["status"])
	}
	if loaded["phase"] != "approval" {
		t.Fatalf("phase = %v, want approval after template approval drift", loaded["phase"])
	}
	statusContext, ok := loaded["status_context"].(map[string]any)
	if !ok {
		t.Fatalf("status_context = %T, want map[string]any", loaded["status_context"])
	}
	if statusContext["reason_code"] != "template_approval_drift" {
		t.Fatalf("reason_code = %v, want template_approval_drift before reapproval", statusContext["reason_code"])
	}

	reapproval := postJSONURL(t, server.Client(), server.URL+"/api/v1/templates/flutter-finance-lite:submit-approval", map[string]any{
		"job_id":  "job-public-template-source-reapproval",
		"prd_id":  "prd-public-template-source-reapproval",
		"summary": "确认当前模板冻结版本升级后需要重新编译 prepare 产物。",
	}, http.StatusOK)
	if subjectVersion, _ := reapproval["subject_version"].(string); !strings.HasPrefix(subjectVersion, "selected-template@flutter-finance-lite@v0.1.0@sha256:") {
		t.Fatalf("subject_version = %v, want content-bound template approval version after template reapproval", reapproval["subject_version"])
	}

	loaded = getJSONURL(t, server.Client(), server.URL+"/api/v1/jobs/job-public-template-source-reapproval", http.StatusOK)
	if loaded["status"] != "awaiting_prepare_recompile" {
		t.Fatalf("status = %v, want awaiting_prepare_recompile after template reapproval", loaded["status"])
	}
	if loaded["phase"] != "prepare" {
		t.Fatalf("phase = %v, want prepare after template reapproval", loaded["phase"])
	}
	statusContext, ok = loaded["status_context"].(map[string]any)
	if !ok {
		t.Fatalf("status_context = %T, want map[string]any", loaded["status_context"])
	}
	if statusContext["reason_code"] != "prepared_template_source_stale" {
		t.Fatalf("reason_code = %v, want prepared_template_source_stale", statusContext["reason_code"])
	}
	if statusContext["suggested_action"] != "compile_prepare_bundle" {
		t.Fatalf("suggested_action = %v, want compile_prepare_bundle", statusContext["suggested_action"])
	}
	if _, ok := loaded["resume_context"]; ok {
		t.Fatalf("resume_context should be omitted while template prepare recompile is required, got %v", loaded["resume_context"])
	}

	startReq, err := http.NewRequest(http.MethodPost, server.URL+"/api/v1/jobs/job-public-template-source-reapproval:start", bytes.NewReader([]byte(`{}`)))
	if err != nil {
		t.Fatalf("NewRequest(start) error = %v", err)
	}
	startReq.Header.Set("Content-Type", "application/json")
	startResp, err := server.Client().Do(startReq)
	if err != nil {
		t.Fatalf("Do(start) error = %v", err)
	}
	defer startResp.Body.Close()
	if startResp.StatusCode != http.StatusConflict {
		t.Fatalf("status = %d, want %d", startResp.StatusCode, http.StatusConflict)
	}
	var startBody internalErrorResponse
	if err := json.NewDecoder(startResp.Body).Decode(&startBody); err != nil {
		t.Fatalf("Decode(start) error = %v", err)
	}
	if startBody.ErrorCode != "JOB_START_CONFLICT" {
		t.Fatalf("error_code = %q, want JOB_START_CONFLICT", startBody.ErrorCode)
	}
	if !strings.Contains(startBody.Message, "requires prepare recompile") || !strings.Contains(startBody.Message, "prepared_template_subject_version mismatch") {
		t.Fatalf("message = %q, want template prepare recompile conflict", startBody.Message)
	}

	resumeReq, err := http.NewRequest(http.MethodPost, server.URL+"/api/v1/jobs/job-public-template-source-reapproval:resume", bytes.NewReader([]byte(`{"resume_mode":"retry_failed_run"}`)))
	if err != nil {
		t.Fatalf("NewRequest(resume) error = %v", err)
	}
	resumeReq.Header.Set("Content-Type", "application/json")
	resumeResp, err := server.Client().Do(resumeReq)
	if err != nil {
		t.Fatalf("Do(resume) error = %v", err)
	}
	defer resumeResp.Body.Close()
	if resumeResp.StatusCode != http.StatusConflict {
		t.Fatalf("status = %d, want %d", resumeResp.StatusCode, http.StatusConflict)
	}
	var resumeBody internalErrorResponse
	if err := json.NewDecoder(resumeResp.Body).Decode(&resumeBody); err != nil {
		t.Fatalf("Decode(resume) error = %v", err)
	}
	if resumeBody.ErrorCode != "JOB_RESUME_CONFLICT" {
		t.Fatalf("error_code = %q, want JOB_RESUME_CONFLICT", resumeBody.ErrorCode)
	}
	if !strings.Contains(resumeBody.Message, "requires prepare recompile") || !strings.Contains(resumeBody.Message, "prepared_template_subject_version mismatch") {
		t.Fatalf("message = %q, want template prepare recompile conflict", resumeBody.Message)
	}
}

func TestResumeFailedPublicJobRejectsWhenPreparedBundleRecompiledAfterFailure(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	cfg := config.DefaultConfig()
	workspace := filepath.Join(filepath.Dir(configPath), "workspace")
	cfg.Workspace = workspace
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	h := NewHandler(configPath)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	server := httptest.NewServer(mux)
	defer server.Close()

	postJSONURL(t, server.Client(), server.URL+"/api/v1/prds:compile", map[string]any{
		"requirement_text": "做一个简单记账 app，不考虑上架，只考虑功能实现，需要首页概览、记一笔和账单列表。",
		"job_id":           "job-public-resume-recompile-conflict",
		"prd_id":           "prd-public-resume-recompile-conflict",
	}, http.StatusOK)
	postJSONURL(t, server.Client(), server.URL+"/api/v1/jobs", map[string]any{
		"prd_id":      "prd-public-resume-recompile-conflict",
		"template_id": "flutter-finance-lite",
	}, http.StatusOK)
	postJSONURL(t, server.Client(), server.URL+"/internal/v1/builders:register", map[string]any{
		"builder_id":        "builder-a",
		"display_name":      "Builder A",
		"capability_tags":   []string{"flutter"},
		"max_parallel_runs": 1,
		"worker_profile": map[string]any{
			"image": "builder:latest",
		},
	}, http.StatusOK)
	allocated := postJSONURL(t, server.Client(), server.URL+"/internal/v1/workers:allocate", map[string]any{
		"job_id": "job-public-resume-recompile-conflict",
	}, http.StatusOK)
	leaseID, _ := allocated["lease_id"].(string)
	createdRun := postJSONURL(t, server.Client(), server.URL+"/internal/v1/build-runs", map[string]any{
		"worker_id":     "builder-a",
		"lease_id":      leaseID,
		"builder_input": sampleBuildRunInputFor("job-public-resume-recompile-conflict", "prd-public-resume-recompile-conflict", "flutter-finance-lite"),
	}, http.StatusOK)
	runID, _ := createdRun["run_id"].(string)
	postJSONURL(t, server.Client(), server.URL+"/internal/v1/build-runs/"+runID+"/fail", map[string]any{
		"summary":             "builder runtime crashed",
		"recovery_suggestion": "retry later",
		"failure_signatures":  []string{"runtime_crash"},
	}, http.StatusOK)

	postJSONURL(t, server.Client(), server.URL+"/api/v1/prds:compile", map[string]any{
		"requirement_text": "做一个简单记账 app，重新编译后进入新的执行轮次。",
		"job_id":           "job-public-resume-recompile-conflict",
		"prd_id":           "prd-public-resume-recompile-conflict",
	}, http.StatusOK)

	req, err := http.NewRequest(http.MethodPost, server.URL+"/api/v1/jobs/job-public-resume-recompile-conflict:resume", bytes.NewReader([]byte(`{"resume_mode":"retry_failed_run"}`)))
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := server.Client().Do(req)
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusConflict)
	}
	var body internalErrorResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	if body.ErrorCode != "JOB_RESUME_CONFLICT" {
		t.Fatalf("error_code = %q, want JOB_RESUME_CONFLICT", body.ErrorCode)
	}
	if !strings.Contains(body.Message, "use start instead of resume") {
		t.Fatalf("message = %q, want stale prepared run conflict", body.Message)
	}
}

func TestResumeFailedPublicJobRejectsWhenPreparedBuilderInputChangesAfterFailure(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	cfg := config.DefaultConfig()
	workspace := filepath.Join(filepath.Dir(configPath), "workspace")
	cfg.Workspace = workspace
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	h := NewHandler(configPath)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	server := httptest.NewServer(mux)
	defer server.Close()

	postJSONURL(t, server.Client(), server.URL+"/api/v1/prds:compile", map[string]any{
		"requirement_text": "做一个简单记账 app，不考虑上架，只考虑功能实现，需要首页概览、记一笔和账单列表。",
		"job_id":           "job-public-resume-builder-input-conflict",
		"prd_id":           "prd-public-resume-builder-input-conflict",
	}, http.StatusOK)
	postJSONURL(t, server.Client(), server.URL+"/api/v1/jobs", map[string]any{
		"prd_id":      "prd-public-resume-builder-input-conflict",
		"template_id": "flutter-finance-lite",
	}, http.StatusOK)
	postJSONURL(t, server.Client(), server.URL+"/internal/v1/builders:register", map[string]any{
		"builder_id":        "builder-a",
		"display_name":      "Builder A",
		"capability_tags":   []string{"flutter"},
		"max_parallel_runs": 1,
		"worker_profile": map[string]any{
			"image": "builder:latest",
		},
	}, http.StatusOK)
	allocated := postJSONURL(t, server.Client(), server.URL+"/internal/v1/workers:allocate", map[string]any{
		"job_id": "job-public-resume-builder-input-conflict",
	}, http.StatusOK)
	leaseID, _ := allocated["lease_id"].(string)
	createdRun := postJSONURL(t, server.Client(), server.URL+"/internal/v1/build-runs", map[string]any{
		"worker_id":     "builder-a",
		"lease_id":      leaseID,
		"builder_input": sampleBuildRunInputFor("job-public-resume-builder-input-conflict", "prd-public-resume-builder-input-conflict", "flutter-finance-lite"),
	}, http.StatusOK)
	runID, _ := createdRun["run_id"].(string)
	postJSONURL(t, server.Client(), server.URL+"/internal/v1/build-runs/"+runID+"/fail", map[string]any{
		"summary":             "builder runtime crashed",
		"recovery_suggestion": "retry later",
		"failure_signatures":  []string{"runtime_crash"},
	}, http.StatusOK)

	builderInputPath := filepath.Join(workspace, "appfactory", "jobs", "job-public-resume-builder-input-conflict", "prepare", "builder-input.json")
	builderInputData, err := os.ReadFile(builderInputPath)
	if err != nil {
		t.Fatalf("ReadFile(builder-input.json) error = %v", err)
	}
	var builderInput map[string]any
	if err := json.Unmarshal(builderInputData, &builderInput); err != nil {
		t.Fatalf("Unmarshal(builder-input.json) error = %v", err)
	}
	builderInput["token_budget"] = 4096
	updatedBuilderInput, err := json.MarshalIndent(builderInput, "", "  ")
	if err != nil {
		t.Fatalf("Marshal(builder-input.json) error = %v", err)
	}
	if err := os.WriteFile(builderInputPath, append(updatedBuilderInput, '\n'), 0o644); err != nil {
		t.Fatalf("WriteFile(builder-input.json) error = %v", err)
	}

	req, err := http.NewRequest(http.MethodPost, server.URL+"/api/v1/jobs/job-public-resume-builder-input-conflict:resume", bytes.NewReader([]byte(`{"resume_mode":"retry_failed_run"}`)))
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := server.Client().Do(req)
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusConflict)
	}
	var body internalErrorResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	if body.ErrorCode != "JOB_RESUME_CONFLICT" {
		t.Fatalf("error_code = %q, want JOB_RESUME_CONFLICT", body.ErrorCode)
	}
	if !strings.Contains(body.Message, "use start instead of resume") {
		t.Fatalf("message = %q, want stale prepared run conflict after builder-input drift", body.Message)
	}
}

func TestResumeFailedPublicJobRestoresPreservedWorkspace(t *testing.T) {
	configPath := filepath.Join(newRetriableTempDir(t), "config.json")
	cfg := config.DefaultConfig()
	workspace := filepath.Join(filepath.Dir(configPath), "workspace")
	cfg.Workspace = workspace
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	h := NewHandler(configPath)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	server := httptest.NewServer(mux)
	defer server.Close()

	postJSONURL(t, server.Client(), server.URL+"/api/v1/prds:compile", map[string]any{
		"requirement_text": "做一个简单记账 app，不考虑上架，只考虑功能实现，需要首页概览、记一笔和账单列表。",
		"job_id":           "job-public-resume-preserved",
		"prd_id":           "prd-public-resume-preserved",
	}, http.StatusOK)
	postJSONURL(t, server.Client(), server.URL+"/api/v1/jobs", map[string]any{
		"prd_id":      "prd-public-resume-preserved",
		"template_id": "flutter-finance-lite",
	}, http.StatusOK)
	postJSONURL(t, server.Client(), server.URL+"/internal/v1/builders:register", map[string]any{
		"builder_id":        "builder-a",
		"display_name":      "Builder A",
		"capability_tags":   []string{"flutter"},
		"max_parallel_runs": 1,
		"worker_profile": map[string]any{
			"image": "builder:latest",
		},
	}, http.StatusOK)
	allocated := postJSONURL(t, server.Client(), server.URL+"/internal/v1/workers:allocate", map[string]any{
		"job_id": "job-public-resume-preserved",
	}, http.StatusOK)
	leaseID, _ := allocated["lease_id"].(string)
	createdRun := postJSONURL(t, server.Client(), server.URL+"/internal/v1/build-runs", map[string]any{
		"worker_id":     "builder-a",
		"lease_id":      leaseID,
		"builder_input": sampleBuildRunInputFor("job-public-resume-preserved", "prd-public-resume-preserved", "flutter-finance-lite"),
	}, http.StatusOK)
	runID, _ := createdRun["run_id"].(string)
	postJSONURL(t, server.Client(), server.URL+"/internal/v1/build-runs/"+runID+"/fail", map[string]any{
		"summary":             "builder runtime crashed",
		"recovery_suggestion": "retry later",
		"failure_signatures":  []string{"runtime_crash"},
	}, http.StatusOK)

	preservedDir := filepath.Join(workspace, "appfactory", "jobs", "job-public-resume-preserved", "snapshots", "preserved", "resume-1")
	if err := os.MkdirAll(preservedDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(preservedDir) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(preservedDir, "resume-marker.txt"), []byte("resume-from-preserved\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(resume-marker.txt) error = %v", err)
	}

	outputPath := filepath.Join(workspace, "appfactory", "jobs", "job-public-resume-preserved", "runs", runID, "builder-output.json")
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	output := sampleBuildRunOutputFor("job-public-resume-preserved")
	output["status"] = "failed"
	output["resume_context"] = map[string]any{
		"resume_allowed":               true,
		"failure_category":             "builder_runtime_failure",
		"recommended_resume_mode":      "resume_from_failure",
		"requires_preserved_workspace": true,
		"preserved_workspace_path":     "jobs/job-public-resume-preserved/snapshots/preserved/resume-1",
		"next_action":                  "resume_from_failure",
	}
	data, err := json.Marshal(output)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	if err := os.WriteFile(outputPath, append(data, '\n'), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	req, err := http.NewRequest(http.MethodPost, server.URL+"/api/v1/jobs/job-public-resume-preserved:resume", bytes.NewReader([]byte(`{"resume_mode":"retry_failed_run"}`)))
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := server.Client().Do(req)
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusConflict)
	}

	resumed := postJSONURL(t, server.Client(), server.URL+"/api/v1/jobs/job-public-resume-preserved:resume", map[string]any{
		"resume_mode": "resume_from_failure",
	}, http.StatusOK)
	if resumed["status"] != "running_builder" {
		t.Fatalf("status = %v, want running_builder", resumed["status"])
	}
	completed := waitForJobStatus(t, server.Client(), server.URL+"/api/v1/jobs/job-public-resume-preserved", "completed", 5*time.Second)
	if completed["builder_output_path"] == "" {
		t.Fatal("builder_output_path should not be empty after restored resume completion")
	}
	markerData, err := os.ReadFile(filepath.Join(workspace, "appfactory", "jobs", "job-public-resume-preserved", "workspace", "resume-marker.txt"))
	if err != nil {
		t.Fatalf("ReadFile(resume-marker.txt) error = %v", err)
	}
	if strings.TrimSpace(string(markerData)) != "resume-from-preserved" {
		t.Fatalf("resume-marker.txt = %q, want resume-from-preserved", string(markerData))
	}
	events := getJSONURL(t, server.Client(), server.URL+"/api/v1/jobs/job-public-resume-preserved/events", http.StatusOK)
	items, ok := events["items"].([]any)
	if !ok || len(items) == 0 {
		t.Fatalf("items = %v, want non-empty", events["items"])
	}
	foundWorkspaceArchived := false
	foundWorkspaceRestored := false
	for _, item := range items {
		event, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if event["type"] == "workspace_archived" {
			foundWorkspaceArchived = true
			if snapshotPath, _ := event["snapshot_path"].(string); !strings.Contains(snapshotPath, "/snapshots/archived/") {
				t.Fatalf("workspace_archived snapshot_path = %v, want archived snapshot path", event["snapshot_path"])
			}
		}
		if event["type"] == "workspace_restored" {
			foundWorkspaceRestored = true
			if event["snapshot_path"] != "jobs/job-public-resume-preserved/snapshots/preserved/resume-1" {
				t.Fatalf("workspace_restored snapshot_path = %v, want preserved snapshot path", event["snapshot_path"])
			}
		}
	}
	if !foundWorkspaceArchived || !foundWorkspaceRestored {
		t.Fatalf("missing workspace lifecycle events: archived=%v restored=%v items=%v", foundWorkspaceArchived, foundWorkspaceRestored, items)
	}
}

func TestResumeFailedPublicJobFallsBackToJobWorkspaceWhenPreservedWorkspacePathMissing(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	cfg := config.DefaultConfig()
	workspace := filepath.Join(filepath.Dir(configPath), "workspace")
	cfg.Workspace = workspace
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	h := NewHandler(configPath)
	t.Cleanup(h.waitForAsyncJobs)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	server := httptest.NewServer(mux)
	defer server.Close()

	postJSONURL(t, server.Client(), server.URL+"/api/v1/prds:compile", map[string]any{
		"requirement_text": "做一个简单记账 app，不考虑上架，只考虑功能实现，需要首页概览、记一笔和账单列表。",
		"job_id":           "job-public-resume-preserved-missing-path",
		"prd_id":           "prd-public-resume-preserved-missing-path",
	}, http.StatusOK)
	postJSONURL(t, server.Client(), server.URL+"/api/v1/jobs", map[string]any{
		"prd_id":      "prd-public-resume-preserved-missing-path",
		"template_id": "flutter-finance-lite",
	}, http.StatusOK)
	postJSONURL(t, server.Client(), server.URL+"/internal/v1/builders:register", map[string]any{
		"builder_id":        "builder-a",
		"display_name":      "Builder A",
		"capability_tags":   []string{"flutter"},
		"max_parallel_runs": 1,
		"worker_profile": map[string]any{
			"image": "builder:latest",
		},
	}, http.StatusOK)
	allocated := postJSONURL(t, server.Client(), server.URL+"/internal/v1/workers:allocate", map[string]any{
		"job_id": "job-public-resume-preserved-missing-path",
	}, http.StatusOK)
	leaseID, _ := allocated["lease_id"].(string)
	createdRun := postJSONURL(t, server.Client(), server.URL+"/internal/v1/build-runs", map[string]any{
		"worker_id":     "builder-a",
		"lease_id":      leaseID,
		"builder_input": sampleBuildRunInputFor("job-public-resume-preserved-missing-path", "prd-public-resume-preserved-missing-path", "flutter-finance-lite"),
	}, http.StatusOK)
	runID, _ := createdRun["run_id"].(string)
	postJSONURL(t, server.Client(), server.URL+"/internal/v1/build-runs/"+runID+"/fail", map[string]any{
		"summary":             "builder runtime crashed",
		"recovery_suggestion": "retry later",
		"failure_signatures":  []string{"runtime_crash"},
	}, http.StatusOK)

	if err := os.WriteFile(filepath.Join(workspace, "appfactory", "jobs", "job-public-resume-preserved-missing-path", "workspace", "resume-marker.txt"), []byte("resume-fallback-workspace\n"), 0o644); err != nil {
		if err := os.MkdirAll(filepath.Join(workspace, "appfactory", "jobs", "job-public-resume-preserved-missing-path", "workspace"), 0o755); err != nil {
			t.Fatalf("MkdirAll(workspace) error = %v", err)
		}
		if err := os.WriteFile(filepath.Join(workspace, "appfactory", "jobs", "job-public-resume-preserved-missing-path", "workspace", "resume-marker.txt"), []byte("resume-fallback-workspace\n"), 0o644); err != nil {
			t.Fatalf("WriteFile(resume-marker.txt) error = %v", err)
		}
	}

	outputPath := filepath.Join(workspace, "appfactory", "jobs", "job-public-resume-preserved-missing-path", "runs", runID, "builder-output.json")
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	output := sampleBuildRunOutputFor("job-public-resume-preserved-missing-path")
	output["status"] = "failed"
	output["resume_context"] = map[string]any{
		"resume_allowed":               true,
		"failure_category":             "builder_runtime_failure",
		"requires_preserved_workspace": true,
		"preserved_workspace_path":     "",
		"next_action":                  "inspect_preserved_workspace",
	}
	data, err := json.Marshal(output)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	if err := os.WriteFile(outputPath, append(data, '\n'), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	loaded := getJSONURL(t, server.Client(), server.URL+"/api/v1/jobs/job-public-resume-preserved-missing-path", http.StatusOK)
	resumeContext, ok := loaded["resume_context"].(map[string]any)
	if !ok {
		t.Fatalf("resume_context = %T, want map[string]any", loaded["resume_context"])
	}
	if resumeContext["preserved_workspace_path"] != "jobs/job-public-resume-preserved-missing-path/workspace" {
		t.Fatalf("preserved_workspace_path = %v, want synthesized workspace path", resumeContext["preserved_workspace_path"])
	}
	if resumeContext["recommended_resume_mode"] != "resume_from_failure" {
		t.Fatalf("recommended_resume_mode = %v, want resume_from_failure", resumeContext["recommended_resume_mode"])
	}

	resumed := postJSONURL(t, server.Client(), server.URL+"/api/v1/jobs/job-public-resume-preserved-missing-path:resume", map[string]any{
		"resume_mode": "resume_from_failure",
	}, http.StatusOK)
	if resumed["status"] != "running_builder" {
		t.Fatalf("status = %v, want running_builder", resumed["status"])
	}
	completed := waitForJobStatus(t, server.Client(), server.URL+"/api/v1/jobs/job-public-resume-preserved-missing-path", "completed", 5*time.Second)
	if completed["builder_output_path"] == "" {
		t.Fatal("builder_output_path should not be empty after fallback workspace resume completion")
	}
	markerData, err := os.ReadFile(filepath.Join(workspace, "appfactory", "jobs", "job-public-resume-preserved-missing-path", "workspace", "resume-marker.txt"))
	if err != nil {
		t.Fatalf("ReadFile(resume-marker.txt) error = %v", err)
	}
	if strings.TrimSpace(string(markerData)) != "resume-fallback-workspace" {
		t.Fatalf("resume-marker.txt = %q, want resume-fallback-workspace", string(markerData))
	}
}

func TestResumeFailedPublicJobDefaultsToResumeFromFailureWhenPreservedWorkspaceRequired(t *testing.T) {
	configPath := filepath.Join(newRetriableTempDir(t), "config.json")
	cfg := config.DefaultConfig()
	workspace := filepath.Join(filepath.Dir(configPath), "workspace")
	cfg.Workspace = workspace
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	h := NewHandler(configPath)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	server := httptest.NewServer(mux)
	defer server.Close()

	postJSONURL(t, server.Client(), server.URL+"/api/v1/prds:compile", map[string]any{
		"requirement_text": "做一个简单记账 app，不考虑上架，只考虑功能实现，需要首页概览、记一笔和账单列表。",
		"job_id":           "job-public-resume-preserved-default-mode",
		"prd_id":           "prd-public-resume-preserved-default-mode",
	}, http.StatusOK)
	postJSONURL(t, server.Client(), server.URL+"/api/v1/jobs", map[string]any{
		"prd_id":      "prd-public-resume-preserved-default-mode",
		"template_id": "flutter-finance-lite",
	}, http.StatusOK)
	postJSONURL(t, server.Client(), server.URL+"/internal/v1/builders:register", map[string]any{
		"builder_id":        "builder-a",
		"display_name":      "Builder A",
		"capability_tags":   []string{"flutter"},
		"max_parallel_runs": 1,
		"worker_profile": map[string]any{
			"image": "builder:latest",
		},
	}, http.StatusOK)
	allocated := postJSONURL(t, server.Client(), server.URL+"/internal/v1/workers:allocate", map[string]any{
		"job_id": "job-public-resume-preserved-default-mode",
	}, http.StatusOK)
	leaseID, _ := allocated["lease_id"].(string)
	createdRun := postJSONURL(t, server.Client(), server.URL+"/internal/v1/build-runs", map[string]any{
		"worker_id":     "builder-a",
		"lease_id":      leaseID,
		"builder_input": sampleBuildRunInputFor("job-public-resume-preserved-default-mode", "prd-public-resume-preserved-default-mode", "flutter-finance-lite"),
	}, http.StatusOK)
	runID, _ := createdRun["run_id"].(string)
	postJSONURL(t, server.Client(), server.URL+"/internal/v1/build-runs/"+runID+"/fail", map[string]any{
		"summary":             "builder runtime crashed",
		"recovery_suggestion": "retry later",
		"failure_signatures":  []string{"runtime_crash"},
	}, http.StatusOK)

	preservedDir := filepath.Join(workspace, "appfactory", "jobs", "job-public-resume-preserved-default-mode", "snapshots", "preserved", "resume-1")
	if err := os.MkdirAll(preservedDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(preservedDir) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(preservedDir, "resume-marker.txt"), []byte("resume-default-mode\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(resume-marker.txt) error = %v", err)
	}

	outputPath := filepath.Join(workspace, "appfactory", "jobs", "job-public-resume-preserved-default-mode", "runs", runID, "builder-output.json")
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	output := sampleBuildRunOutputFor("job-public-resume-preserved-default-mode")
	output["status"] = "failed"
	output["resume_context"] = map[string]any{
		"resume_allowed":               true,
		"failure_category":             "builder_runtime_failure",
		"requires_preserved_workspace": true,
		"preserved_workspace_path":     "jobs/job-public-resume-preserved-default-mode/snapshots/preserved/resume-1",
		"next_action":                  "resume_from_failure",
	}
	data, err := json.Marshal(output)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	if err := os.WriteFile(outputPath, append(data, '\n'), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	resumed := postJSONURL(t, server.Client(), server.URL+"/api/v1/jobs/job-public-resume-preserved-default-mode:resume", map[string]any{}, http.StatusOK)
	if resumed["status"] != "running_builder" {
		t.Fatalf("status = %v, want running_builder", resumed["status"])
	}
	completed := waitForJobStatus(t, server.Client(), server.URL+"/api/v1/jobs/job-public-resume-preserved-default-mode", "completed", 5*time.Second)
	if completed["builder_output_path"] == "" {
		t.Fatal("builder_output_path should not be empty after default resume_from_failure completion")
	}
	markerData, err := os.ReadFile(filepath.Join(workspace, "appfactory", "jobs", "job-public-resume-preserved-default-mode", "workspace", "resume-marker.txt"))
	if err != nil {
		t.Fatalf("ReadFile(resume-marker.txt) error = %v", err)
	}
	if strings.TrimSpace(string(markerData)) != "resume-default-mode" {
		t.Fatalf("resume-marker.txt = %q, want resume-default-mode", string(markerData))
	}
}

func TestResumeFailedPublicJobRequiresHumanConfirmation(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	cfg := config.DefaultConfig()
	workspace := filepath.Join(filepath.Dir(configPath), "workspace")
	cfg.Workspace = workspace
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	h := NewHandler(configPath)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	server := httptest.NewServer(mux)
	defer server.Close()

	postJSONURL(t, server.Client(), server.URL+"/api/v1/prds:compile", map[string]any{
		"requirement_text": "做一个简单记账 app，不考虑上架，只考虑功能实现，需要首页概览、记一笔和账单列表。",
		"job_id":           "job-public-resume-confirm",
		"prd_id":           "prd-public-resume-confirm",
	}, http.StatusOK)
	postJSONURL(t, server.Client(), server.URL+"/api/v1/jobs", map[string]any{
		"prd_id":      "prd-public-resume-confirm",
		"template_id": "flutter-finance-lite",
	}, http.StatusOK)
	postJSONURL(t, server.Client(), server.URL+"/internal/v1/builders:register", map[string]any{
		"builder_id":        "builder-a",
		"display_name":      "Builder A",
		"capability_tags":   []string{"flutter"},
		"max_parallel_runs": 1,
		"worker_profile": map[string]any{
			"image": "builder:latest",
		},
	}, http.StatusOK)
	allocated := postJSONURL(t, server.Client(), server.URL+"/internal/v1/workers:allocate", map[string]any{
		"job_id": "job-public-resume-confirm",
	}, http.StatusOK)
	leaseID, _ := allocated["lease_id"].(string)
	createdRun := postJSONURL(t, server.Client(), server.URL+"/internal/v1/build-runs", map[string]any{
		"worker_id":     "builder-a",
		"lease_id":      leaseID,
		"builder_input": sampleBuildRunInputFor("job-public-resume-confirm", "prd-public-resume-confirm", "flutter-finance-lite"),
	}, http.StatusOK)
	runID, _ := createdRun["run_id"].(string)
	postJSONURL(t, server.Client(), server.URL+"/internal/v1/build-runs/"+runID+"/fail", map[string]any{
		"summary":             "builder runtime crashed",
		"recovery_suggestion": "inspect builder log",
		"failure_signatures":  []string{"runtime_crash"},
	}, http.StatusOK)

	outputPath := filepath.Join(workspace, "appfactory", "jobs", "job-public-resume-confirm", "runs", runID, "builder-output.json")
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	output := sampleBuildRunOutputFor("job-public-resume-confirm")
	output["status"] = "failed"
	output["resume_context"] = map[string]any{
		"resume_allowed":              true,
		"failure_category":            "builder_runtime_failure",
		"recommended_resume_mode":     "retry_failed_run",
		"requires_human_confirmation": true,
		"next_action":                 "inspect_failure_and_confirm_resume",
	}
	data, err := json.Marshal(output)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	if err := os.WriteFile(outputPath, append(data, '\n'), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	req, err := http.NewRequest(http.MethodPost, server.URL+"/api/v1/jobs/job-public-resume-confirm:resume", bytes.NewReader([]byte(`{"resume_mode":"retry_failed_run"}`)))
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := server.Client().Do(req)
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusConflict)
	}
	var body internalErrorResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	if body.ErrorCode != "JOB_RESUME_CONFLICT" {
		t.Fatalf("error_code = %q, want JOB_RESUME_CONFLICT", body.ErrorCode)
	}

	resumed := postJSONURL(t, server.Client(), server.URL+"/api/v1/jobs/job-public-resume-confirm:resume", map[string]any{
		"resume_mode": "retry_failed_run",
		"confirm":     true,
	}, http.StatusOK)
	if resumed["status"] != "running_builder" {
		t.Fatalf("status = %v, want running_builder", resumed["status"])
	}
}

func TestResumeInterruptedPublicJobRunsToCompletion(t *testing.T) {
	configPath := filepath.Join(newRetriableTempDir(t), "config.json")
	cfg := config.DefaultConfig()
	workspace := filepath.Join(filepath.Dir(configPath), "workspace")
	cfg.Workspace = workspace
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	h := NewHandler(configPath)
	t.Cleanup(h.waitForAsyncJobs)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	server := httptest.NewServer(mux)
	defer server.Close()

	postJSONURL(t, server.Client(), server.URL+"/api/v1/prds:compile", map[string]any{
		"requirement_text": "做一个简单记账 app，不考虑上架，只考虑功能实现，需要首页概览、记一笔和账单列表。",
		"job_id":           "job-public-resume-interrupted",
		"prd_id":           "prd-public-resume-interrupted",
	}, http.StatusOK)
	postJSONURL(t, server.Client(), server.URL+"/api/v1/jobs", map[string]any{
		"prd_id":      "prd-public-resume-interrupted",
		"template_id": "flutter-finance-lite",
	}, http.StatusOK)
	postJSONURL(t, server.Client(), server.URL+"/internal/v1/builders:register", map[string]any{
		"builder_id":        "builder-a",
		"display_name":      "Builder A",
		"capability_tags":   []string{"flutter"},
		"max_parallel_runs": 1,
		"worker_profile": map[string]any{
			"image": "builder:latest",
		},
	}, http.StatusOK)
	allocated := postJSONURL(t, server.Client(), server.URL+"/internal/v1/workers:allocate", map[string]any{
		"job_id": "job-public-resume-interrupted",
	}, http.StatusOK)
	leaseID, _ := allocated["lease_id"].(string)
	createdRun := postJSONURL(t, server.Client(), server.URL+"/internal/v1/build-runs", map[string]any{
		"worker_id":     "builder-a",
		"lease_id":      leaseID,
		"builder_input": sampleBuildRunInputFor("job-public-resume-interrupted", "prd-public-resume-interrupted", "flutter-finance-lite"),
	}, http.StatusOK)
	runID, _ := createdRun["run_id"].(string)
	postJSONURL(t, server.Client(), server.URL+"/internal/v1/build-runs/"+runID+"/fail", map[string]any{
		"summary":             "orchestrator dispatcher stopped unexpectedly; resume required",
		"recovery_suggestion": "retry resume after the API orchestrator is running again",
		"failure_signatures":  []string{"orchestrator_dispatcher_lost"},
	}, http.StatusOK)

	resumed := postJSONURL(t, server.Client(), server.URL+"/api/v1/jobs/job-public-resume-interrupted:resume", map[string]any{
		"resume_mode": "resume_interrupted_job",
	}, http.StatusOK)
	if resumed["status"] != "running_builder" {
		t.Fatalf("status = %v, want running_builder", resumed["status"])
	}
	completed := waitForJobStatus(t, server.Client(), server.URL+"/api/v1/jobs/job-public-resume-interrupted", "completed", 5*time.Second)
	if completed["builder_output_path"] == "" {
		t.Fatal("builder_output_path should not be empty after interrupted resume completion")
	}
}

func TestResumeCancelledPublicJobReturnsConflict(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	cfg := config.DefaultConfig()
	cfg.Workspace = filepath.Join(filepath.Dir(configPath), "workspace")
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	h := NewHandler(configPath)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	server := httptest.NewServer(mux)
	defer server.Close()

	postJSONURL(t, server.Client(), server.URL+"/api/v1/prds:compile", map[string]any{
		"requirement_text": "做一个简单记账 app，不考虑上架，只考虑功能实现，需要首页概览、记一笔和账单列表。",
		"job_id":           "job-public-resume-cancelled",
		"prd_id":           "prd-public-resume-cancelled",
	}, http.StatusOK)
	postJSONURL(t, server.Client(), server.URL+"/api/v1/jobs", map[string]any{
		"prd_id":      "prd-public-resume-cancelled",
		"template_id": "flutter-finance-lite",
	}, http.StatusOK)
	postJSONURL(t, server.Client(), server.URL+"/api/v1/jobs/job-public-resume-cancelled:cancel", map[string]any{
		"reason": "manual stop before execution",
	}, http.StatusOK)

	req, err := http.NewRequest(http.MethodPost, server.URL+"/api/v1/jobs/job-public-resume-cancelled:resume", bytes.NewReader([]byte(`{"resume_mode":"retry_failed_run"}`)))
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := server.Client().Do(req)
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusConflict)
	}
	var body internalErrorResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	if body.ErrorCode != "JOB_RESUME_CONFLICT" {
		t.Fatalf("error_code = %q, want JOB_RESUME_CONFLICT", body.ErrorCode)
	}
}

func TestGetNotificationsReturnsPendingApprovalsAndFailedJobs(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	cfg := config.DefaultConfig()
	workspace := filepath.Join(filepath.Dir(configPath), "workspace")
	cfg.Workspace = workspace
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	h := NewHandler(configPath)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	server := httptest.NewServer(mux)
	defer server.Close()

	postJSONURL(t, server.Client(), server.URL+"/api/v1/prds:compile", map[string]any{
		"requirement_text": "做一个简单记账 app，不考虑上架，只考虑功能实现，需要首页概览、记一笔和账单列表。",
		"job_id":           "job-notify-approval",
		"prd_id":           "prd-notify-approval",
	}, http.StatusOK)
	postJSONURL(t, server.Client(), server.URL+"/api/v1/approvals", map[string]any{
		"approval_type": "prd",
		"job_id":        "job-notify-approval",
	}, http.StatusCreated)

	postJSONURL(t, server.Client(), server.URL+"/api/v1/prds:compile", map[string]any{
		"requirement_text": "做一个简单记账 app，不考虑上架，只考虑功能实现，需要首页概览、记一笔和账单列表。",
		"job_id":           "job-notify-failed",
		"prd_id":           "prd-notify-failed",
	}, http.StatusOK)
	postJSONURL(t, server.Client(), server.URL+"/api/v1/jobs", map[string]any{
		"prd_id":      "prd-notify-failed",
		"template_id": "flutter-finance-lite",
	}, http.StatusOK)
	postJSONURL(t, server.Client(), server.URL+"/internal/v1/builders:register", map[string]any{
		"builder_id":        "builder-a",
		"display_name":      "Builder A",
		"capability_tags":   []string{"flutter"},
		"max_parallel_runs": 1,
		"worker_profile": map[string]any{
			"image": "builder:latest",
		},
	}, http.StatusOK)
	allocated := postJSONURL(t, server.Client(), server.URL+"/internal/v1/workers:allocate", map[string]any{
		"job_id": "job-notify-failed",
	}, http.StatusOK)
	leaseID, _ := allocated["lease_id"].(string)
	createdRun := postJSONURL(t, server.Client(), server.URL+"/internal/v1/build-runs", map[string]any{
		"worker_id":     "builder-a",
		"lease_id":      leaseID,
		"builder_input": sampleBuildRunInputFor("job-notify-failed", "prd-notify-failed", "flutter-finance-lite"),
	}, http.StatusOK)
	runID, _ := createdRun["run_id"].(string)
	postJSONURL(t, server.Client(), server.URL+"/internal/v1/build-runs/"+runID+"/fail", map[string]any{
		"summary":             "builder runtime crashed",
		"recovery_suggestion": "inspect builder log",
		"failure_signatures":  []string{"runtime_crash"},
	}, http.StatusOK)
	preservedDir := filepath.Join(workspace, "appfactory", "jobs", "job-notify-failed", "snapshots", "preserved", "resume-1")
	if err := os.MkdirAll(preservedDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(preservedDir) error = %v", err)
	}
	outputPath := filepath.Join(workspace, "appfactory", "jobs", "job-notify-failed", "runs", runID, "builder-output.json")
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(outputPath) error = %v", err)
	}
	output := sampleBuildRunOutputFor("job-notify-failed")
	output["status"] = "failed"
	output["resume_context"] = map[string]any{
		"resume_allowed":               true,
		"failure_category":             "builder_runtime_failure",
		"recommended_resume_mode":      "resume_from_failure",
		"requires_preserved_workspace": true,
		"preserved_workspace_path":     "jobs/job-notify-failed/snapshots/preserved/resume-1",
		"next_action":                  "resume_from_failure",
	}
	outputData, err := json.Marshal(output)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	if err := os.WriteFile(outputPath, append(outputData, '\n'), 0o644); err != nil {
		t.Fatalf("WriteFile(builder-output.json) error = %v", err)
	}
	if err := h.persistPublicJobExecutionRecord(publicJobExecutionRecord{
		SchemaVersion: "0.1.0",
		JobID:         "job-notify-failed",
		RunID:         runID,
		Action:        "resume",
		Status:        "failed",
		AttemptCount:  2,
		RequestedAt:   time.Now().UTC().Add(-4 * time.Minute).Format(time.RFC3339),
		StartedAt:     time.Now().UTC().Add(-4 * time.Minute).Format(time.RFC3339),
		FinishedAt:    time.Now().UTC().Add(-2 * time.Minute).Format(time.RFC3339),
		LastError:     "orchestrator dispatcher stopped unexpectedly; resume required",
		Attempts: []publicJobExecutionAttempt{
			{
				Attempt:       1,
				OwnerID:       "orchestrator-foreign",
				ClaimedAt:     time.Now().UTC().Add(-5 * time.Minute).Format(time.RFC3339),
				LastRenewedAt: time.Now().UTC().Add(-4 * time.Minute).Format(time.RFC3339),
				ReleasedAt:    time.Now().UTC().Add(-3 * time.Minute).Format(time.RFC3339),
				ReleaseReason: "handoff_after_lease_expiry",
			},
			{
				Attempt:       2,
				OwnerID:       h.orchestratorInstanceID,
				ClaimedAt:     time.Now().UTC().Add(-3 * time.Minute).Format(time.RFC3339),
				LastRenewedAt: time.Now().UTC().Add(-2 * time.Minute).Format(time.RFC3339),
				ReleasedAt:    time.Now().UTC().Add(-2 * time.Minute).Format(time.RFC3339),
				ReleaseReason: "dispatcher_lost",
			},
		},
	}); err != nil {
		t.Fatalf("persistPublicJobExecutionRecord() error = %v", err)
	}

	postJSONURL(t, server.Client(), server.URL+"/api/v1/prds:compile", map[string]any{
		"requirement_text": "做一个简单记账 app，不考虑上架，只考虑功能实现，需要首页概览、记一笔和账单列表。",
		"job_id":           "job-notify-recovery-failed",
		"prd_id":           "prd-notify-recovery-failed",
	}, http.StatusOK)
	postJSONURL(t, server.Client(), server.URL+"/api/v1/jobs", map[string]any{
		"prd_id":      "prd-notify-recovery-failed",
		"template_id": "flutter-finance-lite",
	}, http.StatusOK)
	if err := h.persistPublicJobExecutionRecord(publicJobExecutionRecord{
		SchemaVersion: "0.1.0",
		JobID:         "job-notify-recovery-failed",
		RunID:         "run-notify-recovery-failed",
		Action:        "resume",
		Status:        "failed",
		AttemptCount:  1,
		RequestedAt:   time.Now().UTC().Add(-90 * time.Second).Format(time.RFC3339),
		FinishedAt:    time.Now().UTC().Add(-60 * time.Second).Format(time.RFC3339),
		LastError:     "recovery failed while reconciling persisted run",
		Attempts: []publicJobExecutionAttempt{{
			Attempt:       1,
			OwnerID:       h.orchestratorInstanceID,
			ClaimedAt:     time.Now().UTC().Add(-80 * time.Second).Format(time.RFC3339),
			LastRenewedAt: time.Now().UTC().Add(-70 * time.Second).Format(time.RFC3339),
			ReleasedAt:    time.Now().UTC().Add(-60 * time.Second).Format(time.RFC3339),
			ReleaseReason: "recovery_failed",
		}},
	}); err != nil {
		t.Fatalf("persistPublicJobExecutionRecord() error = %v", err)
	}

	postJSONURL(t, server.Client(), server.URL+"/api/v1/prds:compile", map[string]any{
		"requirement_text": "做一个简单记账 app，不考虑上架，只考虑功能实现，需要首页概览、记一笔和账单列表。",
		"job_id":           "job-notify-execution-failed",
		"prd_id":           "prd-notify-execution-failed",
	}, http.StatusOK)
	postJSONURL(t, server.Client(), server.URL+"/api/v1/jobs", map[string]any{
		"prd_id":      "prd-notify-execution-failed",
		"template_id": "flutter-finance-lite",
	}, http.StatusOK)
	if err := h.persistPublicJobExecutionRecord(publicJobExecutionRecord{
		SchemaVersion: "0.1.0",
		JobID:         "job-notify-execution-failed",
		RunID:         "run-notify-execution-failed",
		Action:        "start",
		Status:        "failed",
		AttemptCount:  1,
		RequestedAt:   time.Now().UTC().Add(-75 * time.Second).Format(time.RFC3339),
		FinishedAt:    time.Now().UTC().Add(-70 * time.Second).Format(time.RFC3339),
		LastError:     "executor crashed before run status could be finalized",
		Attempts: []publicJobExecutionAttempt{{
			Attempt:       1,
			OwnerID:       h.orchestratorInstanceID,
			ClaimedAt:     time.Now().UTC().Add(-74 * time.Second).Format(time.RFC3339),
			LastRenewedAt: time.Now().UTC().Add(-72 * time.Second).Format(time.RFC3339),
			ReleasedAt:    time.Now().UTC().Add(-70 * time.Second).Format(time.RFC3339),
			ReleaseReason: "failed",
		}},
	}); err != nil {
		t.Fatalf("persistPublicJobExecutionRecord() error = %v", err)
	}

	postJSONURL(t, server.Client(), server.URL+"/api/v1/prds:compile", map[string]any{
		"requirement_text": "做一个简单记账 app，不考虑上架，只考虑功能实现，需要首页概览、记一笔和账单列表。",
		"job_id":           "job-notify-execution-cancelled",
		"prd_id":           "prd-notify-execution-cancelled",
	}, http.StatusOK)
	postJSONURL(t, server.Client(), server.URL+"/api/v1/jobs", map[string]any{
		"prd_id":      "prd-notify-execution-cancelled",
		"template_id": "flutter-finance-lite",
	}, http.StatusOK)
	if err := h.persistPublicJobExecutionRecord(publicJobExecutionRecord{
		SchemaVersion: "0.1.0",
		JobID:         "job-notify-execution-cancelled",
		RunID:         "run-notify-execution-cancelled",
		Action:        "resume",
		Status:        "cancelled",
		AttemptCount:  1,
		RequestedAt:   time.Now().UTC().Add(-68 * time.Second).Format(time.RFC3339),
		FinishedAt:    time.Now().UTC().Add(-64 * time.Second).Format(time.RFC3339),
		LastError:     "operator cancelled orchestrator execution before dispatch stabilized",
		Attempts: []publicJobExecutionAttempt{{
			Attempt:       1,
			OwnerID:       h.orchestratorInstanceID,
			ClaimedAt:     time.Now().UTC().Add(-67 * time.Second).Format(time.RFC3339),
			LastRenewedAt: time.Now().UTC().Add(-65 * time.Second).Format(time.RFC3339),
			ReleasedAt:    time.Now().UTC().Add(-64 * time.Second).Format(time.RFC3339),
			ReleaseReason: "cancelled",
		}},
	}); err != nil {
		t.Fatalf("persistPublicJobExecutionRecord() error = %v", err)
	}

	postJSONURL(t, server.Client(), server.URL+"/api/v1/prds:compile", map[string]any{
		"requirement_text": "做一个简单记账 app，不考虑上架，只考虑功能实现，需要首页概览、记一笔和账单列表。",
		"job_id":           "job-notify-cancelled",
		"prd_id":           "prd-notify-cancelled",
	}, http.StatusOK)
	postJSONURL(t, server.Client(), server.URL+"/api/v1/jobs", map[string]any{
		"prd_id":      "prd-notify-cancelled",
		"template_id": "flutter-finance-lite",
	}, http.StatusOK)
	cancelWorkspaceDir := filepath.Join(workspace, "appfactory", "jobs", "job-notify-cancelled", "workspace")
	if err := os.MkdirAll(cancelWorkspaceDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(cancelWorkspaceDir) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(cancelWorkspaceDir, "cancel-marker.txt"), []byte("cancelled-preserved\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(cancel-marker.txt) error = %v", err)
	}
	postJSONURL(t, server.Client(), server.URL+"/api/v1/jobs/job-notify-cancelled:cancel", map[string]any{
		"reason":             "manual stop before execution",
		"preserve_workspace": true,
	}, http.StatusOK)

	notifications := getJSONURL(t, server.Client(), server.URL+"/api/v1/notifications", http.StatusOK)
	items, ok := notifications["items"].([]any)
	if !ok || len(items) < 8 {
		t.Fatalf("items = %v, want at least 8 notifications", notifications["items"])
	}
	foundApprovalRequested := false
	foundBuilderFailed := false
	foundExecutionFailed := false
	foundExecutionCancelled := false
	foundExecutionInterrupted := false
	foundExecutionHandoff := false
	foundExecutionRecoveryFailed := false
	foundWorkspacePreserved := false
	for _, item := range items {
		notification, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if notification["type"] == "prd_approval_requested" {
			foundApprovalRequested = true
		}
		if notification["type"] == "builder_failed" {
			foundBuilderFailed = true
			if notification["suggested_action"] != "resume_from_failure" {
				t.Fatalf("builder_failed suggested_action = %v, want resume_from_failure", notification["suggested_action"])
			}
			if notification["recommended_resume_mode"] != "resume_from_failure" {
				t.Fatalf("builder_failed recommended_resume_mode = %v, want resume_from_failure", notification["recommended_resume_mode"])
			}
			if notification["failure_category"] != "builder_runtime_failure" {
				t.Fatalf("builder_failed failure_category = %v, want builder_runtime_failure", notification["failure_category"])
			}
			if notification["requires_preserved_workspace"] != true {
				t.Fatalf("builder_failed requires_preserved_workspace = %v, want true", notification["requires_preserved_workspace"])
			}
			links, ok := notification["links"].([]any)
			if !ok || len(links) < 2 {
				t.Fatalf("builder_failed links = %v, want job + preserved snapshot links", notification["links"])
			}
		}
		if notification["type"] == "execution_interrupted" {
			foundExecutionInterrupted = true
			if notification["suggested_action"] != "resume_interrupted_job" {
				t.Fatalf("execution_interrupted suggested_action = %v, want resume_interrupted_job", notification["suggested_action"])
			}
		}
		if notification["type"] == "execution_failed" {
			foundExecutionFailed = true
			if notification["suggested_action"] != "inspect_failure_and_retry_later" {
				t.Fatalf("execution_failed suggested_action = %v, want inspect_failure_and_retry_later", notification["suggested_action"])
			}
		}
		if notification["type"] == "execution_cancelled" {
			foundExecutionCancelled = true
			if notification["suggested_action"] != "inspect_execution_history" {
				t.Fatalf("execution_cancelled suggested_action = %v, want inspect_execution_history", notification["suggested_action"])
			}
		}
		if notification["type"] == "execution_handoff" {
			foundExecutionHandoff = true
			if notification["suggested_action"] != "inspect_execution_history" {
				t.Fatalf("execution_handoff suggested_action = %v, want inspect_execution_history", notification["suggested_action"])
			}
		}
		if notification["type"] == "execution_recovery_failed" {
			foundExecutionRecoveryFailed = true
			if notification["suggested_action"] != "inspect_failure_and_retry_later" {
				t.Fatalf("execution_recovery_failed suggested_action = %v, want inspect_failure_and_retry_later", notification["suggested_action"])
			}
		}
		if notification["type"] == "workspace_preserved" {
			foundWorkspacePreserved = true
			if notification["suggested_action"] != "inspect_preserved_workspace" {
				t.Fatalf("workspace_preserved suggested_action = %v, want inspect_preserved_workspace", notification["suggested_action"])
			}
		}
	}
	if !foundApprovalRequested || !foundBuilderFailed || !foundExecutionFailed || !foundExecutionCancelled || !foundExecutionInterrupted || !foundExecutionHandoff || !foundExecutionRecoveryFailed || !foundWorkspacePreserved {
		t.Fatalf("missing notification types: approval=%v builder_failed=%v execution_failed=%v execution_cancelled=%v execution_interrupted=%v execution_handoff=%v execution_recovery_failed=%v workspace_preserved=%v items=%v", foundApprovalRequested, foundBuilderFailed, foundExecutionFailed, foundExecutionCancelled, foundExecutionInterrupted, foundExecutionHandoff, foundExecutionRecoveryFailed, foundWorkspacePreserved, items)
	}
	notificationIndexPath := filepath.Join(workspace, "appfactory", "notifications", "index.json")
	indexData, err := os.ReadFile(notificationIndexPath)
	if err != nil {
		t.Fatalf("ReadFile(notification index) error = %v", err)
	}
	var persisted map[string]any
	if err := json.Unmarshal(indexData, &persisted); err != nil {
		t.Fatalf("Unmarshal(notification index) error = %v", err)
	}
	requireExactObjectKeys(t, persisted, []string{"generated_at", "items"})
	if generatedAt, _ := persisted["generated_at"].(string); strings.TrimSpace(generatedAt) == "" {
		t.Fatalf("generated_at = %v, want non-empty", persisted["generated_at"])
	}
	persistedItems, ok := persisted["items"].([]any)
	if !ok || len(persistedItems) < 8 {
		t.Fatalf("persisted items = %v, want at least 8 notifications", persisted["items"])
	}
	for _, item := range persistedItems {
		notification, ok := item.(map[string]any)
		if !ok {
			continue
		}
		requireObjectContainsKeys(t, notification, []string{"notification_id", "type", "created_at"})
	}
}

func TestNotificationsSupportAckFilterAndRebuild(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	cfg := config.DefaultConfig()
	workspace := filepath.Join(filepath.Dir(configPath), "workspace")
	cfg.Workspace = workspace
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	h := NewHandler(configPath)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	server := httptest.NewServer(mux)
	defer server.Close()

	postJSONURL(t, server.Client(), server.URL+"/api/v1/prds:compile", map[string]any{
		"requirement_text": "做一个简单记账 app，不考虑上架，只考虑功能实现，需要首页概览、记一笔和账单列表。",
		"job_id":           "job-notify-ack",
		"prd_id":           "prd-notify-ack",
	}, http.StatusOK)
	postJSONURL(t, server.Client(), server.URL+"/api/v1/approvals", map[string]any{
		"approval_type": "prd",
		"job_id":        "job-notify-ack",
	}, http.StatusCreated)

	notifications := getJSONURL(t, server.Client(), server.URL+"/api/v1/notifications", http.StatusOK)
	items, ok := notifications["items"].([]any)
	if !ok || len(items) == 0 {
		t.Fatalf("items = %v, want non-empty", notifications["items"])
	}
	notificationID := ""
	for _, item := range items {
		notification, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if notification["type"] == "prd_approval_requested" {
			id, _ := notification["notification_id"].(string)
			notificationID = id
			break
		}
	}
	if notificationID == "" {
		t.Fatalf("missing prd_approval_requested notification in %v", items)
	}

	acked := postJSONURL(t, server.Client(), server.URL+"/api/v1/notifications/"+notificationID+":ack", map[string]any{}, http.StatusOK)
	if acked["acknowledged"] != true {
		t.Fatalf("acknowledged = %v, want true", acked["acknowledged"])
	}
	if acked["acknowledged_at"] == "" {
		t.Fatalf("acknowledged_at = %v, want non-empty", acked["acknowledged_at"])
	}

	unackedOnly := getJSONURL(t, server.Client(), server.URL+"/api/v1/notifications?acknowledged=false", http.StatusOK)
	unackedItems, ok := unackedOnly["items"].([]any)
	if !ok {
		t.Fatalf("unacked items = %v, want array", unackedOnly["items"])
	}
	for _, item := range unackedItems {
		notification, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if notification["notification_id"] == notificationID {
			t.Fatalf("acknowledged notification should be filtered from unacked list: %v", notification)
		}
	}

	ackedOnly := getJSONURL(t, server.Client(), server.URL+"/api/v1/notifications?acknowledged=true&type=prd_approval_requested&job_id=job-notify-ack", http.StatusOK)
	ackedItems, ok := ackedOnly["items"].([]any)
	if !ok || len(ackedItems) != 1 {
		t.Fatalf("acked items = %v, want exactly one acknowledged approval notification", ackedOnly["items"])
	}
	ackedNotification, ok := ackedItems[0].(map[string]any)
	if !ok {
		t.Fatalf("acked item = %T, want map[string]any", ackedItems[0])
	}
	if ackedNotification["notification_id"] != notificationID {
		t.Fatalf("notification_id = %v, want %s", ackedNotification["notification_id"], notificationID)
	}

	rebuilt := postJSONURL(t, server.Client(), server.URL+"/api/v1/notifications:rebuild", map[string]any{}, http.StatusOK)
	if rebuilt["rebuild"] != true {
		t.Fatalf("rebuild = %v, want true", rebuilt["rebuild"])
	}
	if generatedAt, _ := rebuilt["generated_at"].(string); strings.TrimSpace(generatedAt) == "" {
		t.Fatalf("generated_at = %v, want non-empty", rebuilt["generated_at"])
	}
	rebuiltItems, ok := rebuilt["items"].([]any)
	if !ok || len(rebuiltItems) == 0 {
		t.Fatalf("rebuilt items = %v, want non-empty", rebuilt["items"])
	}
	foundAckAfterRebuild := false
	for _, item := range rebuiltItems {
		notification, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if notification["notification_id"] == notificationID {
			foundAckAfterRebuild = true
			if notification["acknowledged"] != true {
				t.Fatalf("acknowledged after rebuild = %v, want true", notification["acknowledged"])
			}
		}
	}
	if !foundAckAfterRebuild {
		t.Fatalf("acknowledged notification missing after rebuild: %v", rebuiltItems)
	}
}

func TestGetOrchestratorStatusReturnsSnapshot(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	cfg := config.DefaultConfig()
	workspace := filepath.Join(filepath.Dir(configPath), "workspace")
	cfg.Workspace = workspace
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	h := NewHandler(configPath)
	if err := h.persistPublicJobExecutionRecord(publicJobExecutionRecord{
		SchemaVersion:   "0.1.0",
		JobID:           "job-status-skip",
		RunID:           "run-status-skip",
		Action:          "start",
		Status:          "queued",
		LeaseOwnerID:    "orchestrator-foreign",
		LeaseAcquiredAt: time.Now().UTC().Add(-10 * time.Second).Format(time.RFC3339),
		LeaseExpiresAt:  time.Now().UTC().Add(90 * time.Second).Format(time.RFC3339),
		RequestedAt:     time.Now().UTC().Add(-10 * time.Second).Format(time.RFC3339),
	}); err != nil {
		t.Fatalf("persistPublicJobExecutionRecord() error = %v", err)
	}

	mux := http.NewServeMux()
	h.EnableStartupOrchestrator()
	h.RegisterRoutes(mux)
	h.waitForAsyncJobs()
	server := httptest.NewServer(mux)
	defer server.Close()

	status := getJSONURL(t, server.Client(), server.URL+"/internal/v1/orchestrator/status", http.StatusOK)
	if status["state"] != "idle" {
		t.Fatalf("state = %v, want idle", status["state"])
	}
	if status["last_trigger"] != "handler_startup" {
		t.Fatalf("last_trigger = %v, want handler_startup", status["last_trigger"])
	}
	if status["foreign_live_lease_skips"] != float64(1) {
		t.Fatalf("foreign_live_lease_skips = %v, want 1", status["foreign_live_lease_skips"])
	}
}

func TestRunOrchestratorPassEndpointReturnsUpdatedStatus(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	cfg := config.DefaultConfig()
	workspace := filepath.Join(filepath.Dir(configPath), "workspace")
	cfg.Workspace = workspace
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	h := NewHandler(configPath)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	h.waitForAsyncJobs()
	server := httptest.NewServer(mux)
	defer server.Close()

	if err := h.persistPublicJobExecutionRecord(publicJobExecutionRecord{
		SchemaVersion:   "0.1.0",
		JobID:           "job-run-endpoint-skip",
		RunID:           "run-run-endpoint-skip",
		Action:          "resume",
		Status:          "queued",
		LeaseOwnerID:    "orchestrator-foreign",
		LeaseAcquiredAt: time.Now().UTC().Add(-10 * time.Second).Format(time.RFC3339),
		LeaseExpiresAt:  time.Now().UTC().Add(90 * time.Second).Format(time.RFC3339),
		RequestedAt:     time.Now().UTC().Add(-10 * time.Second).Format(time.RFC3339),
	}); err != nil {
		t.Fatalf("persistPublicJobExecutionRecord() error = %v", err)
	}

	status := postJSONURL(t, server.Client(), server.URL+"/internal/v1/orchestrator:run", map[string]any{}, http.StatusOK)
	if status["state"] != "idle" {
		t.Fatalf("state = %v, want idle", status["state"])
	}
	if status["last_trigger"] != "internal_api" {
		t.Fatalf("last_trigger = %v, want internal_api", status["last_trigger"])
	}
	if status["foreign_live_lease_skips"] != float64(1) {
		t.Fatalf("foreign_live_lease_skips = %v, want 1", status["foreign_live_lease_skips"])
	}
}

func TestRunPublicJobOrchestratorPassAccumulatesForeignLiveLeaseSkipsAcrossPasses(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	cfg := config.DefaultConfig()
	workspace := filepath.Join(filepath.Dir(configPath), "workspace")
	cfg.Workspace = workspace
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	h := NewHandler(configPath)
	if err := h.persistPublicJobExecutionRecord(publicJobExecutionRecord{
		SchemaVersion:   "0.1.0",
		JobID:           "job-pass-accumulate-skip",
		RunID:           "run-pass-accumulate-skip",
		Action:          "resume",
		Status:          "queued",
		LeaseOwnerID:    "orchestrator-foreign",
		LeaseAcquiredAt: time.Now().UTC().Add(-10 * time.Second).Format(time.RFC3339),
		LeaseExpiresAt:  time.Now().UTC().Add(90 * time.Second).Format(time.RFC3339),
		RequestedAt:     time.Now().UTC().Add(-10 * time.Second).Format(time.RFC3339),
	}); err != nil {
		t.Fatalf("persistPublicJobExecutionRecord() error = %v", err)
	}

	firstStatus, err := h.RunPublicJobOrchestratorPass("test_accumulate_skip_first")
	if err != nil {
		t.Fatalf("RunPublicJobOrchestratorPass(first) error = %v", err)
	}
	if firstStatus.ForeignLiveLeaseSkips != 1 {
		t.Fatalf("foreign_live_lease_skips = %d, want 1 after first pass", firstStatus.ForeignLiveLeaseSkips)
	}
	if firstStatus.LastTrigger != "test_accumulate_skip_first" {
		t.Fatalf("last_trigger = %q, want test_accumulate_skip_first", firstStatus.LastTrigger)
	}

	secondStatus, err := h.RunPublicJobOrchestratorPass("test_accumulate_skip_second")
	if err != nil {
		t.Fatalf("RunPublicJobOrchestratorPass(second) error = %v", err)
	}
	if secondStatus.ForeignLiveLeaseSkips != 2 {
		t.Fatalf("foreign_live_lease_skips = %d, want 2 after second pass", secondStatus.ForeignLiveLeaseSkips)
	}
	if secondStatus.LastTrigger != "test_accumulate_skip_second" {
		t.Fatalf("last_trigger = %q, want test_accumulate_skip_second", secondStatus.LastTrigger)
	}

	loaded, err := h.LoadPublicJobOrchestratorStatus()
	if err != nil {
		t.Fatalf("LoadPublicJobOrchestratorStatus() error = %v", err)
	}
	if loaded.ForeignLiveLeaseSkips != 2 {
		t.Fatalf("persisted foreign_live_lease_skips = %d, want 2", loaded.ForeignLiveLeaseSkips)
	}
	if loaded.LastTrigger != "test_accumulate_skip_second" {
		t.Fatalf("persisted last_trigger = %q, want test_accumulate_skip_second", loaded.LastTrigger)
	}
}

func TestRunPublicJobOrchestratorPassRetainsQueuedAndRunningRecoveryCountersAcrossPasses(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	cfg := config.DefaultConfig()
	workspace := filepath.Join(filepath.Dir(configPath), "workspace")
	cfg.Workspace = workspace
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	h := NewHandler(configPath)
	t.Cleanup(h.waitForAsyncJobs)
	requestedAt := time.Now().UTC().Add(-2 * time.Minute).Format(time.RFC3339)
	if err := h.persistPublicJobExecutionRecord(publicJobExecutionRecord{
		SchemaVersion:  "0.1.0",
		JobID:          "job-pass-queued-recovery",
		RunID:          "run-pass-queued-recovery",
		Action:         "start",
		Status:         "queued",
		TimeoutSeconds: 300,
		RequestedAt:    requestedAt,
	}); err != nil {
		t.Fatalf("persistPublicJobExecutionRecord(queued) error = %v", err)
	}
	if err := h.persistPublicJobExecutionRecord(publicJobExecutionRecord{
		SchemaVersion:  "0.1.0",
		JobID:          "job-pass-running-recovery",
		RunID:          "run-pass-running-recovery",
		Action:         "resume",
		Status:         "running",
		TimeoutSeconds: 300,
		RequestedAt:    requestedAt,
		StartedAt:      time.Now().UTC().Add(-90 * time.Second).Format(time.RFC3339),
	}); err != nil {
		t.Fatalf("persistPublicJobExecutionRecord(running) error = %v", err)
	}

	firstStatus, err := h.RunPublicJobOrchestratorPass("test_recovery_counters_first")
	if err != nil {
		t.Fatalf("RunPublicJobOrchestratorPass(first) error = %v", err)
	}
	h.waitForAsyncJobs()
	if firstStatus.QueuedRecoveries != 1 {
		t.Fatalf("queued_recoveries = %d, want 1 after first pass", firstStatus.QueuedRecoveries)
	}
	if firstStatus.RunningRecoveries != 1 {
		t.Fatalf("running_recoveries = %d, want 1 after first pass", firstStatus.RunningRecoveries)
	}

	queuedExecution := readPublicJobExecutionRecord(t, workspace, "job-pass-queued-recovery")
	if queuedExecution.Status != "failed" {
		t.Fatalf("queued execution status = %q, want failed after first recovery attempt", queuedExecution.Status)
	}
	runningExecution := readPublicJobExecutionRecord(t, workspace, "job-pass-running-recovery")
	if runningExecution.Status != "failed" {
		t.Fatalf("running execution status = %q, want failed after first recovery attempt", runningExecution.Status)
	}

	secondStatus, err := h.RunPublicJobOrchestratorPass("test_recovery_counters_second")
	if err != nil {
		t.Fatalf("RunPublicJobOrchestratorPass(second) error = %v", err)
	}
	h.waitForAsyncJobs()
	if secondStatus.QueuedRecoveries != firstStatus.QueuedRecoveries {
		t.Fatalf("queued_recoveries = %d, want retained %d after second pass", secondStatus.QueuedRecoveries, firstStatus.QueuedRecoveries)
	}
	if secondStatus.RunningRecoveries != firstStatus.RunningRecoveries {
		t.Fatalf("running_recoveries = %d, want retained %d after second pass", secondStatus.RunningRecoveries, firstStatus.RunningRecoveries)
	}

	loaded, err := h.LoadPublicJobOrchestratorStatus()
	if err != nil {
		t.Fatalf("LoadPublicJobOrchestratorStatus() error = %v", err)
	}
	if loaded.QueuedRecoveries != 1 {
		t.Fatalf("persisted queued_recoveries = %d, want 1", loaded.QueuedRecoveries)
	}
	if loaded.RunningRecoveries != 1 {
		t.Fatalf("persisted running_recoveries = %d, want 1", loaded.RunningRecoveries)
	}
	if loaded.LastTrigger != "test_recovery_counters_second" {
		t.Fatalf("persisted last_trigger = %q, want test_recovery_counters_second", loaded.LastTrigger)
	}
}

func TestGetOrchestratorStatusIncludesWatchLockState(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	cfg := config.DefaultConfig()
	workspace := filepath.Join(filepath.Dir(configPath), "workspace")
	cfg.Workspace = workspace
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	h := NewHandler(configPath)
	if err := h.persistPublicJobOrchestratorWatchLock(publicJobOrchestratorWatchLock{
		SchemaVersion: "0.1.0",
		OwnerID:       "orchestrator-foreign",
		Mode:          "cli_watch",
		AcquiredAt:    time.Now().UTC().Add(-20 * time.Second).Format(time.RFC3339),
		UpdatedAt:     time.Now().UTC().Add(-5 * time.Second).Format(time.RFC3339),
		ExpiresAt:     time.Now().UTC().Add(30 * time.Second).Format(time.RFC3339),
	}); err != nil {
		t.Fatalf("persistPublicJobOrchestratorWatchLock() error = %v", err)
	}

	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	h.waitForAsyncJobs()
	server := httptest.NewServer(mux)
	defer server.Close()

	status := getJSONURL(t, server.Client(), server.URL+"/internal/v1/orchestrator/status", http.StatusOK)
	if status["watch_lock_state"] != "held_by_other" {
		t.Fatalf("watch_lock_state = %v, want held_by_other", status["watch_lock_state"])
	}
	if status["watch_lock_owner_id"] != "orchestrator-foreign" {
		t.Fatalf("watch_lock_owner_id = %v, want orchestrator-foreign", status["watch_lock_owner_id"])
	}
	if status["watch_lock_mode"] != "cli_watch" {
		t.Fatalf("watch_lock_mode = %v, want cli_watch", status["watch_lock_mode"])
	}
	if strings.TrimSpace(fmt.Sprint(status["watch_lock_updated_at"])) == "" {
		t.Fatalf("watch_lock_updated_at = %v, want non-empty value", status["watch_lock_updated_at"])
	}
}

func TestGetOrchestratorStatusIncludesPersistedCLIWatchRunnerRole(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	cfg := config.DefaultConfig()
	workspace := filepath.Join(filepath.Dir(configPath), "workspace")
	cfg.Workspace = workspace
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	h := NewHandler(configPath)
	if err := h.persistPublicJobOrchestratorStatus(PublicJobOrchestratorStatus{
		SchemaVersion:        "0.1.0",
		InstanceID:           h.orchestratorInstanceID,
		State:                "idle",
		WatchRunnerState:     "running",
		WatchRunnerMode:      "cli_watch",
		WatchRunnerOwnerID:   "orchestrator-foreign",
		WatchRunnerInterval:  15,
		WatchRunnerStartedAt: time.Now().UTC().Add(-20 * time.Second).Format(time.RFC3339),
		UpdatedAt:            time.Now().UTC().Format(time.RFC3339),
	}); err != nil {
		t.Fatalf("persistPublicJobOrchestratorStatus() error = %v", err)
	}
	if err := h.persistPublicJobOrchestratorWatchLock(publicJobOrchestratorWatchLock{
		SchemaVersion: "0.1.0",
		OwnerID:       "orchestrator-foreign",
		Mode:          "cli_watch",
		AcquiredAt:    time.Now().UTC().Add(-20 * time.Second).Format(time.RFC3339),
		UpdatedAt:     time.Now().UTC().Add(-5 * time.Second).Format(time.RFC3339),
		ExpiresAt:     time.Now().UTC().Add(30 * time.Second).Format(time.RFC3339),
	}); err != nil {
		t.Fatalf("persistPublicJobOrchestratorWatchLock() error = %v", err)
	}

	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	h.waitForAsyncJobs()
	server := httptest.NewServer(mux)
	defer server.Close()

	status := getJSONURL(t, server.Client(), server.URL+"/internal/v1/orchestrator/status", http.StatusOK)
	if status["watch_runner_state"] != "running" {
		t.Fatalf("watch_runner_state = %v, want running", status["watch_runner_state"])
	}
	if status["watch_runner_mode"] != "cli_watch" {
		t.Fatalf("watch_runner_mode = %v, want cli_watch", status["watch_runner_mode"])
	}
	if status["watch_runner_owner_id"] != "orchestrator-foreign" {
		t.Fatalf("watch_runner_owner_id = %v, want orchestrator-foreign", status["watch_runner_owner_id"])
	}
	if status["watch_lock_mode"] != "cli_watch" {
		t.Fatalf("watch_lock_mode = %v, want cli_watch", status["watch_lock_mode"])
	}
}

func TestUnlockOrchestratorWatchEndpointRejectsForeignActiveLockWithoutForce(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	cfg := config.DefaultConfig()
	workspace := filepath.Join(filepath.Dir(configPath), "workspace")
	cfg.Workspace = workspace
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	h := NewHandler(configPath)
	if err := h.persistPublicJobOrchestratorWatchLock(publicJobOrchestratorWatchLock{
		SchemaVersion: "0.1.0",
		OwnerID:       "orchestrator-foreign",
		Mode:          "cli_watch",
		AcquiredAt:    time.Now().UTC().Add(-20 * time.Second).Format(time.RFC3339),
		UpdatedAt:     time.Now().UTC().Add(-5 * time.Second).Format(time.RFC3339),
		ExpiresAt:     time.Now().UTC().Add(30 * time.Second).Format(time.RFC3339),
	}); err != nil {
		t.Fatalf("persistPublicJobOrchestratorWatchLock() error = %v", err)
	}

	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	server := httptest.NewServer(mux)
	defer server.Close()

	payload, err := json.Marshal(map[string]any{})
	if err != nil {
		t.Fatalf("Marshal(payload) error = %v", err)
	}
	req, err := http.NewRequest(http.MethodPost, server.URL+"/internal/v1/orchestrator:unlock-watch", bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := server.Client().Do(req)
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusConflict)
	}
	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("Decode(error response) error = %v", err)
	}
	if body["error_code"] != "APPFACTORY_ORCHESTRATOR_WATCH_LOCK_CONFLICT" {
		t.Fatalf("error_code = %v, want APPFACTORY_ORCHESTRATOR_WATCH_LOCK_CONFLICT", body["error_code"])
	}
	audits := requireJSONLinesObjectsFile(t, filepath.Join(workspace, "appfactory", "orchestrator", "watch-events.jsonl"))
	if len(audits) != 1 {
		t.Fatalf("audits = %v, want exactly 1 rejected unlock audit", audits)
	}
	audit := audits[0]
	if audit["action"] != "orchestrator_watch_unlock_rejected" {
		t.Fatalf("audit action = %v, want orchestrator_watch_unlock_rejected", audit["action"])
	}
	if audit["watch_lock_state"] != "held_by_other" {
		t.Fatalf("watch_lock_state = %v, want held_by_other", audit["watch_lock_state"])
	}
	if audit["watch_lock_owner_id"] != "orchestrator-foreign" {
		t.Fatalf("watch_lock_owner_id = %v, want orchestrator-foreign", audit["watch_lock_owner_id"])
	}
	if !strings.Contains(fmt.Sprint(audit["summary"]), "rerun with force") {
		t.Fatalf("summary = %v, want force hint", audit["summary"])
	}
}

func TestUnlockOrchestratorWatchEndpointForceClearsForeignActiveLock(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	cfg := config.DefaultConfig()
	workspace := filepath.Join(filepath.Dir(configPath), "workspace")
	cfg.Workspace = workspace
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	h := NewHandler(configPath)
	if err := h.persistPublicJobOrchestratorWatchLock(publicJobOrchestratorWatchLock{
		SchemaVersion: "0.1.0",
		OwnerID:       "orchestrator-foreign",
		Mode:          "cli_watch",
		AcquiredAt:    time.Now().UTC().Add(-20 * time.Second).Format(time.RFC3339),
		UpdatedAt:     time.Now().UTC().Add(-5 * time.Second).Format(time.RFC3339),
		ExpiresAt:     time.Now().UTC().Add(30 * time.Second).Format(time.RFC3339),
	}); err != nil {
		t.Fatalf("persistPublicJobOrchestratorWatchLock() error = %v", err)
	}

	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	server := httptest.NewServer(mux)
	defer server.Close()

	status := postJSONURL(t, server.Client(), server.URL+"/internal/v1/orchestrator:unlock-watch", map[string]any{"force": true}, http.StatusOK)
	if status["watch_lock_state"] != "unlocked" {
		t.Fatalf("watch_lock_state = %v, want unlocked", status["watch_lock_state"])
	}
	lockPath := filepath.Join(workspace, "appfactory", "orchestrator", "watch-lock.json")
	if _, err := os.Stat(lockPath); !os.IsNotExist(err) {
		t.Fatalf("watch lock should be removed, stat err=%v", err)
	}
}

func TestStartOrchestratorWatchEndpointStartsBackgroundWatcher(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	cfg := config.DefaultConfig()
	workspace := filepath.Join(filepath.Dir(configPath), "workspace")
	cfg.Workspace = workspace
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	h := NewHandler(configPath)
	t.Cleanup(func() {
		_ = h.StopPublicJobOrchestratorWatch()
	})
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	h.waitForAsyncJobs()
	server := httptest.NewServer(mux)
	defer server.Close()

	status := postJSONURL(t, server.Client(), server.URL+"/internal/v1/orchestrator/watch:start", map[string]any{"interval_seconds": 1}, http.StatusOK)
	if status["watch_runner_state"] != "running" {
		t.Fatalf("watch_runner_state = %v, want running", status["watch_runner_state"])
	}
	if status["watch_runner_interval_seconds"] != float64(1) {
		t.Fatalf("watch_runner_interval_seconds = %v, want 1", status["watch_runner_interval_seconds"])
	}
	if status["watch_lock_state"] != "held_by_self" {
		t.Fatalf("watch_lock_state = %v, want held_by_self", status["watch_lock_state"])
	}
	if status["watch_lock_mode"] != "internal_api_watch" {
		t.Fatalf("watch_lock_mode = %v, want internal_api_watch", status["watch_lock_mode"])
	}
	loaded := getJSONURL(t, server.Client(), server.URL+"/internal/v1/orchestrator/status", http.StatusOK)
	if loaded["watch_runner_state"] != "running" {
		t.Fatalf("loaded watch_runner_state = %v, want running", loaded["watch_runner_state"])
	}
	statusPath := filepath.Join(workspace, "appfactory", "orchestrator", "status.json")
	persistedStatus := requireJSONObjectFile(t, statusPath)
	requireObjectContainsKeys(t, persistedStatus, []string{
		"schema_version",
		"instance_id",
		"state",
		"watch_runner_state",
		"watch_runner_mode",
		"watch_runner_owner_id",
		"watch_runner_interval_seconds",
		"watch_runner_started_at",
		"watch_lock_state",
		"updated_at",
	})
}

func TestStartOrchestratorWatchEndpointRejectsDuplicateStart(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	cfg := config.DefaultConfig()
	workspace := filepath.Join(filepath.Dir(configPath), "workspace")
	cfg.Workspace = workspace
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	h := NewHandler(configPath)
	t.Cleanup(func() {
		_ = h.StopPublicJobOrchestratorWatch()
	})
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	h.waitForAsyncJobs()
	server := httptest.NewServer(mux)
	defer server.Close()

	postJSONURL(t, server.Client(), server.URL+"/internal/v1/orchestrator/watch:start", map[string]any{"interval_seconds": 1}, http.StatusOK)
	payload, err := json.Marshal(map[string]any{"interval_seconds": 1})
	if err != nil {
		t.Fatalf("Marshal(payload) error = %v", err)
	}
	req, err := http.NewRequest(http.MethodPost, server.URL+"/internal/v1/orchestrator/watch:start", bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := server.Client().Do(req)
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusConflict)
	}
	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("Decode(error response) error = %v", err)
	}
	if body["error_code"] != "APPFACTORY_ORCHESTRATOR_WATCH_CONFLICT" {
		t.Fatalf("error_code = %v, want APPFACTORY_ORCHESTRATOR_WATCH_CONFLICT", body["error_code"])
	}
	audits := requireJSONLinesObjectsFile(t, filepath.Join(workspace, "appfactory", "orchestrator", "watch-events.jsonl"))
	if len(audits) < 2 {
		t.Fatalf("audits = %v, want started + rejected start audits", audits)
	}
	lastAudit := audits[len(audits)-1]
	if lastAudit["action"] != "orchestrator_watch_start_rejected" {
		t.Fatalf("last audit action = %v, want orchestrator_watch_start_rejected", lastAudit["action"])
	}
	if lastAudit["watch_runner_state"] != "running" {
		t.Fatalf("watch_runner_state = %v, want running", lastAudit["watch_runner_state"])
	}
	if lastAudit["watch_lock_state"] != "held_by_self" {
		t.Fatalf("watch_lock_state = %v, want held_by_self", lastAudit["watch_lock_state"])
	}
	if !strings.Contains(fmt.Sprint(lastAudit["summary"]), "already running") {
		t.Fatalf("summary = %v, want already running", lastAudit["summary"])
	}
}

func TestRunOrchestratorPassEndpointReturnsWhileWatchRuns(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	cfg := config.DefaultConfig()
	workspace := filepath.Join(filepath.Dir(configPath), "workspace")
	cfg.Workspace = workspace
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	h := NewHandler(configPath)
	t.Cleanup(func() {
		_ = h.StopPublicJobOrchestratorWatch()
	})
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	h.waitForAsyncJobs()
	server := httptest.NewServer(mux)
	defer server.Close()

	postJSONURL(t, server.Client(), server.URL+"/internal/v1/orchestrator/watch:start", map[string]any{"interval_seconds": 1}, http.StatusOK)

	resultCh := make(chan map[string]any, 1)
	errCh := make(chan error, 1)
	go func() {
		defer close(resultCh)
		defer close(errCh)
		payload, err := json.Marshal(map[string]any{})
		if err != nil {
			errCh <- err
			return
		}
		req, err := http.NewRequest(http.MethodPost, server.URL+"/internal/v1/orchestrator:run", bytes.NewReader(payload))
		if err != nil {
			errCh <- err
			return
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := server.Client().Do(req)
		if err != nil {
			errCh <- err
			return
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			errCh <- fmt.Errorf("status = %d, want %d", resp.StatusCode, http.StatusOK)
			return
		}
		var body map[string]any
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			errCh <- err
			return
		}
		resultCh <- body
	}()

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("run orchestrator request error = %v", err)
		}
	case result := <-resultCh:
		if result["state"] != "idle" {
			t.Fatalf("state = %v, want idle", result["state"])
		}
	case <-time.After(2 * time.Second):
		t.Fatal("expected orchestrator run endpoint to return while watch is active")
	}
}

func TestStopOrchestratorWatchEndpointStopsBackgroundWatcher(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	cfg := config.DefaultConfig()
	workspace := filepath.Join(filepath.Dir(configPath), "workspace")
	cfg.Workspace = workspace
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	h := NewHandler(configPath)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	h.waitForAsyncJobs()
	server := httptest.NewServer(mux)
	defer server.Close()

	postJSONURL(t, server.Client(), server.URL+"/internal/v1/orchestrator/watch:start", map[string]any{"interval_seconds": 1}, http.StatusOK)
	status := postJSONURL(t, server.Client(), server.URL+"/internal/v1/orchestrator/watch:stop", map[string]any{}, http.StatusOK)
	if status["watch_runner_state"] != "not_running" {
		t.Fatalf("watch_runner_state = %v, want not_running", status["watch_runner_state"])
	}
	if status["watch_lock_state"] != "unlocked" {
		t.Fatalf("watch_lock_state = %v, want unlocked", status["watch_lock_state"])
	}
	if strings.TrimSpace(fmt.Sprint(status["watch_runner_stopped_at"])) == "" {
		t.Fatalf("watch_runner_stopped_at = %v, want non-empty value", status["watch_runner_stopped_at"])
	}
	lockPath := filepath.Join(workspace, "appfactory", "orchestrator", "watch-lock.json")
	if _, err := os.Stat(lockPath); !os.IsNotExist(err) {
		t.Fatalf("watch lock should be removed, stat err=%v", err)
	}
}

func TestHandlerShutdownStopsBackgroundOrchestratorWatch(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	cfg := config.DefaultConfig()
	workspace := filepath.Join(filepath.Dir(configPath), "workspace")
	cfg.Workspace = workspace
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	h := NewHandler(configPath)
	status, err := h.StartPublicJobOrchestratorWatch(1 * time.Second)
	if err != nil {
		t.Fatalf("StartPublicJobOrchestratorWatch() error = %v", err)
	}
	if status.WatchRunnerState != "running" {
		t.Fatalf("watch_runner_state = %s, want running", status.WatchRunnerState)
	}

	shutdownDone := make(chan struct{})
	go func() {
		defer close(shutdownDone)
		h.Shutdown()
	}()
	select {
	case <-shutdownDone:
	case <-time.After(3 * time.Second):
		t.Fatal("Shutdown() timed out while stopping orchestrator watch")
	}
	loaded, err := h.LoadPublicJobOrchestratorStatus()
	if err != nil {
		t.Fatalf("LoadPublicJobOrchestratorStatus() error = %v", err)
	}
	if loaded.WatchRunnerState != "not_running" {
		t.Fatalf("watch_runner_state = %s, want not_running", loaded.WatchRunnerState)
	}
	if loaded.WatchLockState != "unlocked" {
		t.Fatalf("watch_lock_state = %s, want unlocked", loaded.WatchLockState)
	}
}

func TestGetNotificationsIncludesOrchestratorWatchAuditEvents(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	cfg := config.DefaultConfig()
	workspace := filepath.Join(filepath.Dir(configPath), "workspace")
	cfg.Workspace = workspace
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	h := NewHandler(configPath)
	t.Cleanup(func() {
		_ = h.StopPublicJobOrchestratorWatch()
	})
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	h.waitForAsyncJobs()
	server := httptest.NewServer(mux)
	defer server.Close()

	postJSONURL(t, server.Client(), server.URL+"/internal/v1/orchestrator/watch:start", map[string]any{"interval_seconds": 1}, http.StatusOK)
	postJSONURL(t, server.Client(), server.URL+"/internal/v1/orchestrator/watch:stop", map[string]any{}, http.StatusOK)
	if err := h.persistPublicJobOrchestratorWatchLock(publicJobOrchestratorWatchLock{
		SchemaVersion: "0.1.0",
		OwnerID:       "orchestrator-foreign",
		Mode:          "cli_watch",
		AcquiredAt:    time.Now().UTC().Add(-20 * time.Second).Format(time.RFC3339),
		UpdatedAt:     time.Now().UTC().Add(-5 * time.Second).Format(time.RFC3339),
		ExpiresAt:     time.Now().UTC().Add(30 * time.Second).Format(time.RFC3339),
	}); err != nil {
		t.Fatalf("persistPublicJobOrchestratorWatchLock() error = %v", err)
	}
	postJSONURL(t, server.Client(), server.URL+"/internal/v1/orchestrator:unlock-watch", map[string]any{"force": true}, http.StatusOK)

	notifications := getJSONURL(t, server.Client(), server.URL+"/api/v1/notifications", http.StatusOK)
	items, ok := notifications["items"].([]any)
	if !ok {
		t.Fatalf("items = %T, want []any", notifications["items"])
	}
	seen := map[string]bool{}
	for _, item := range items {
		entry, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if entry["type"] == "orchestrator_watch_started" || entry["type"] == "orchestrator_watch_stopped" || entry["type"] == "orchestrator_watch_unlocked" {
			seen[fmt.Sprint(entry["type"])] = true
		}
	}
	if !seen["orchestrator_watch_started"] || !seen["orchestrator_watch_stopped"] || !seen["orchestrator_watch_unlocked"] {
		t.Fatalf("seen watcher notifications = %v, want started/stopped/unlocked", seen)
	}
	auditPath := filepath.Join(workspace, "appfactory", "orchestrator", "watch-events.jsonl")
	auditData, err := os.ReadFile(auditPath)
	if err != nil {
		t.Fatalf("ReadFile(watch-events.jsonl) error = %v", err)
	}
	if !strings.Contains(string(auditData), "orchestrator_watch_started") || !strings.Contains(string(auditData), "orchestrator_watch_stopped") || !strings.Contains(string(auditData), "orchestrator_watch_unlocked") {
		t.Fatalf("audit log = %s, want watcher audit actions", string(auditData))
	}
	audits := requireJSONLinesObjectsFile(t, auditPath)
	if len(audits) < 3 {
		t.Fatalf("audits = %v, want at least 3 watcher audit records", audits)
	}
	for _, audit := range audits {
		requireObjectContainsKeys(t, audit, []string{
			"schema_version",
			"audit_id",
			"action",
			"actor",
			"remote_addr",
			"user_agent",
			"summary",
			"watch_runner_state",
			"watch_lock_state",
			"created_at",
		})
	}
}

func TestGetPublicJobEventsIncludesResumeAndSnapshotSignals(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	cfg := config.DefaultConfig()
	workspace := filepath.Join(filepath.Dir(configPath), "workspace")
	cfg.Workspace = workspace
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	h := NewHandler(configPath)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	server := httptest.NewServer(mux)
	defer server.Close()

	postJSONURL(t, server.Client(), server.URL+"/api/v1/prds:compile", map[string]any{
		"requirement_text": "做一个简单记账 app，不考虑上架，只考虑功能实现，需要首页概览、记一笔和账单列表。",
		"job_id":           "job-events-signals",
		"prd_id":           "prd-events-signals",
	}, http.StatusOK)
	postJSONURL(t, server.Client(), server.URL+"/api/v1/jobs", map[string]any{
		"prd_id":      "prd-events-signals",
		"template_id": "flutter-finance-lite",
	}, http.StatusOK)
	postJSONURL(t, server.Client(), server.URL+"/internal/v1/builders:register", map[string]any{
		"builder_id":        "builder-a",
		"display_name":      "Builder A",
		"capability_tags":   []string{"flutter"},
		"max_parallel_runs": 1,
		"worker_profile": map[string]any{
			"image": "builder:latest",
		},
	}, http.StatusOK)
	allocated := postJSONURL(t, server.Client(), server.URL+"/internal/v1/workers:allocate", map[string]any{
		"job_id": "job-events-signals",
	}, http.StatusOK)
	leaseID, _ := allocated["lease_id"].(string)
	createdRun := postJSONURL(t, server.Client(), server.URL+"/internal/v1/build-runs", map[string]any{
		"worker_id":     "builder-a",
		"lease_id":      leaseID,
		"builder_input": sampleBuildRunInputFor("job-events-signals", "prd-events-signals", "flutter-finance-lite"),
	}, http.StatusOK)
	runID, _ := createdRun["run_id"].(string)
	postJSONURL(t, server.Client(), server.URL+"/internal/v1/build-runs/"+runID+"/fail", map[string]any{
		"summary":             "builder runtime crashed",
		"recovery_suggestion": "inspect builder log",
		"failure_signatures":  []string{"runtime_crash"},
	}, http.StatusOK)
	preservedDir := filepath.Join(workspace, "appfactory", "jobs", "job-events-signals", "snapshots", "preserved", "resume-1")
	if err := os.MkdirAll(preservedDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(preservedDir) error = %v", err)
	}
	outputPath := filepath.Join(workspace, "appfactory", "jobs", "job-events-signals", "runs", runID, "builder-output.json")
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(outputPath) error = %v", err)
	}
	output := sampleBuildRunOutputFor("job-events-signals")
	output["status"] = "failed"
	output["resume_context"] = map[string]any{
		"resume_allowed":               true,
		"failure_category":             "builder_runtime_failure",
		"recommended_resume_mode":      "resume_from_failure",
		"requires_preserved_workspace": true,
		"preserved_workspace_path":     "jobs/job-events-signals/snapshots/preserved/resume-1",
		"next_action":                  "resume_from_failure",
	}
	outputData, err := json.Marshal(output)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	if err := os.WriteFile(outputPath, append(outputData, '\n'), 0o644); err != nil {
		t.Fatalf("WriteFile(builder-output.json) error = %v", err)
	}

	events := getJSONURL(t, server.Client(), server.URL+"/api/v1/jobs/job-events-signals/events", http.StatusOK)
	items, ok := events["items"].([]any)
	if !ok || len(items) == 0 {
		t.Fatalf("items = %v, want non-empty", events["items"])
	}
	foundResumeReady := false
	foundResumeRequiresPreservedWorkspace := false
	foundWorkspacePreserved := false
	for _, item := range items {
		event, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if event["type"] == "resume_ready" {
			foundResumeReady = true
			if event["recommended_resume_mode"] != "resume_from_failure" {
				t.Fatalf("resume_ready recommended_resume_mode = %v, want resume_from_failure", event["recommended_resume_mode"])
			}
			if event["requires_preserved_workspace"] != true {
				t.Fatalf("resume_ready requires_preserved_workspace = %v, want true", event["requires_preserved_workspace"])
			}
		}
		if event["type"] == "resume_requires_preserved_workspace" {
			foundResumeRequiresPreservedWorkspace = true
		}
		if event["type"] == "workspace_preserved" {
			foundWorkspacePreserved = true
		}
	}
	if !foundResumeReady || !foundResumeRequiresPreservedWorkspace || !foundWorkspacePreserved {
		t.Fatalf("missing synthetic events: resume_ready=%v resume_requires_preserved_workspace=%v workspace_preserved=%v items=%v", foundResumeReady, foundResumeRequiresPreservedWorkspace, foundWorkspacePreserved, items)
	}
}

func TestStartPublicJobArtifactsIncludeRunLog(t *testing.T) {
	configPath := filepath.Join(newRetriableTempDir(t), "config.json")
	cfg := config.DefaultConfig()
	workspace := filepath.Join(filepath.Dir(configPath), "workspace")
	cfg.Workspace = workspace
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	h := NewHandler(configPath)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	server := httptest.NewServer(mux)
	defer server.Close()

	postJSONURL(t, server.Client(), server.URL+"/api/v1/prds:compile", map[string]any{
		"requirement_text": "做一个简单记账 app，不考虑上架，只考虑功能实现，需要首页概览、记一笔和账单列表。",
		"job_id":           "job-start-review-bundle",
		"prd_id":           "prd-start-review-bundle",
	}, http.StatusOK)
	postJSONURL(t, server.Client(), server.URL+"/api/v1/jobs", map[string]any{
		"prd_id":      "prd-start-review-bundle",
		"template_id": "flutter-finance-lite",
	}, http.StatusOK)
	postJSONURL(t, server.Client(), server.URL+"/internal/v1/builders:register", map[string]any{
		"builder_id":        "builder-a",
		"display_name":      "Builder A",
		"capability_tags":   []string{"flutter"},
		"max_parallel_runs": 1,
		"worker_profile": map[string]any{
			"image": "builder:latest",
		},
	}, http.StatusOK)

	started := postJSONURL(t, server.Client(), server.URL+"/api/v1/jobs/job-start-review-bundle:start", map[string]any{}, http.StatusOK)
	if started["status"] != "running_builder" {
		t.Fatalf("status = %v, want running_builder", started["status"])
	}
	completed := waitForJobStatus(t, server.Client(), server.URL+"/api/v1/jobs/job-start-review-bundle", "completed", 5*time.Second)
	if completed["builder_output_path"] == "" {
		t.Fatal("builder_output_path should not be empty after review bundle generation")
	}

	manifest := getJSONURL(t, server.Client(), server.URL+"/api/v1/jobs/job-start-review-bundle/artifacts", http.StatusOK)
	items, ok := manifest["items"].([]any)
	if !ok || len(items) == 0 {
		t.Fatalf("items = %v, want non-empty artifacts", manifest["items"])
	}
	foundRunLog := false
	for _, item := range items {
		artifact, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if artifact["artifact_id"] == "run-log" {
			foundRunLog = true
		}
	}
	if !foundRunLog {
		t.Fatalf("missing run-log artifact: items=%v", items)
	}
}

func TestPrepareReviewEndpointRebuildsReviewArtifactsForCompletedRun(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	cfg := config.DefaultConfig()
	workspace := filepath.Join(filepath.Dir(configPath), "workspace")
	cfg.Workspace = workspace
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	h := NewHandler(configPath)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	server := httptest.NewServer(mux)
	defer server.Close()

	postJSONURL(t, server.Client(), server.URL+"/api/v1/prds:compile", map[string]any{
		"requirement_text": "做一个简单记账 app，不考虑上架，只考虑功能实现，需要首页概览、记一笔和账单列表。",
		"job_id":           "job-review-prepare",
		"prd_id":           "prd-review-prepare",
	}, http.StatusOK)
	postJSONURL(t, server.Client(), server.URL+"/api/v1/jobs", map[string]any{
		"prd_id":      "prd-review-prepare",
		"template_id": "flutter-finance-lite",
	}, http.StatusOK)
	postJSONURL(t, server.Client(), server.URL+"/internal/v1/builders:register", map[string]any{
		"builder_id":        "builder-a",
		"display_name":      "Builder A",
		"capability_tags":   []string{"flutter"},
		"max_parallel_runs": 1,
		"worker_profile": map[string]any{
			"image": "builder:latest",
		},
	}, http.StatusOK)
	allocated := postJSONURL(t, server.Client(), server.URL+"/internal/v1/workers:allocate", map[string]any{
		"job_id": "job-review-prepare",
	}, http.StatusOK)
	leaseID, _ := allocated["lease_id"].(string)
	createdRun := postJSONURL(t, server.Client(), server.URL+"/internal/v1/build-runs", map[string]any{
		"worker_id":     "builder-a",
		"lease_id":      leaseID,
		"builder_input": sampleBuildRunInputFor("job-review-prepare", "prd-review-prepare", "flutter-finance-lite"),
	}, http.StatusOK)
	runID, _ := createdRun["run_id"].(string)
	finishedAt := time.Now().UTC()
	startedAt := finishedAt.Add(-45 * time.Second)
	postJSONURL(t, server.Client(), server.URL+"/internal/v1/build-runs/"+runID+"/complete", map[string]any{
		"builder_output": map[string]any{
			"schema_version": "0.1.0",
			"job_id":         "job-review-prepare",
			"status":         "success",
			"exit_reason":    "completed",
			"worker_id":      "builder-a",
			"started_at":     startedAt.Format(time.RFC3339),
			"finished_at":    finishedAt.Format(time.RFC3339),
			"final_summary":  "builder completed without auto review packaging",
			"modified_files": []any{},
			"checks_passed": []map[string]any{{
				"check_id": "manual-complete",
				"label":    "manual complete",
				"stage":    "baseline",
				"outcome":  "passed",
			}},
			"checks_failed":      []any{},
			"next_human_actions": []any{},
			"artifacts": map[string]any{
				"manifest_path":   "",
				"primary_outputs": []any{},
			},
			"metrics": map[string]any{
				"metrics_path":                "",
				"total_iterations":            1,
				"total_tokens":                0,
				"distinct_failure_signatures": 0,
			},
			"report_paths": map[string]any{
				"change_summary_path":    "",
				"build_report_path":      "",
				"smoke_test_report_path": "",
			},
		},
	}, http.StatusOK)

	prepared := postJSONURL(t, server.Client(), server.URL+"/internal/v1/reviews:prepare", map[string]any{
		"run_id": runID,
	}, http.StatusOK)
	if prepared["ack"] != true {
		t.Fatalf("ack = %v, want true", prepared["ack"])
	}
	if prepared["review_bundle_metadata_path"] != "reports/review-bundle.metadata.json" {
		t.Fatalf("review_bundle_metadata_path = %v, want reports/review-bundle.metadata.json", prepared["review_bundle_metadata_path"])
	}
	if prepared["review_bundle_path"] != "reports/review-bundle.md" {
		t.Fatalf("review_bundle_path = %v, want reports/review-bundle.md", prepared["review_bundle_path"])
	}
	if prepared["handoff_checklist_path"] != "reports/handoff-checklist.md" {
		t.Fatalf("handoff_checklist_path = %v, want reports/handoff-checklist.md", prepared["handoff_checklist_path"])
	}

	manifest := getJSONURL(t, server.Client(), server.URL+"/api/v1/jobs/job-review-prepare/artifacts", http.StatusOK)
	items, ok := manifest["items"].([]any)
	if !ok || len(items) < 7 {
		t.Fatalf("items = %v, want at least 7 artifacts after review prepare", manifest["items"])
	}
	foundReviewBundleMetadata := false
	for _, item := range items {
		artifact, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if artifact["artifact_id"] == "review-bundle-metadata" {
			foundReviewBundleMetadata = true
			if artifact["path"] != "reports/review-bundle.metadata.json" {
				t.Fatalf("review-bundle-metadata path = %v, want reports/review-bundle.metadata.json", artifact["path"])
			}
		}
	}
	if !foundReviewBundleMetadata {
		t.Fatalf("review-bundle-metadata artifact missing from manifest: %v", items)
	}
	reviewMetadata := requireJSONObjectFile(t, filepath.Join(workspace, "appfactory", "jobs", "job-review-prepare", "reports", "review-bundle.metadata.json"))
	requireExactObjectKeys(t, reviewMetadata, []string{
		"schema_version",
		"job_id",
		"run_id",
		"status",
		"builder_id",
		"worker_id",
		"duration_seconds",
		"risk_level",
		"conclusion",
		"builder_log_path",
		"change_summary_path",
		"build_report_path",
		"smoke_test_report_path",
		"review_bundle_path",
		"handoff_checklist_path",
		"artifact_manifest_path",
		"metrics_path",
		"generated_at",
	})
	if reviewMetadata["review_bundle_path"] != "reports/review-bundle.md" {
		t.Fatalf("review_bundle_path in metadata = %v, want reports/review-bundle.md", reviewMetadata["review_bundle_path"])
	}
	reviewBundleData, err := os.ReadFile(filepath.Join(workspace, "appfactory", "jobs", "job-review-prepare", "reports", "review-bundle.md"))
	if err != nil {
		t.Fatalf("ReadFile(review-bundle.md) error = %v", err)
	}
	if !strings.Contains(string(reviewBundleData), "## Artifact 深链") {
		t.Fatalf("review-bundle.md should contain artifact links, got %s", string(reviewBundleData))
	}
}

func TestRecordDeliveryEndpointPersistsDeliveryRecordAndProjectsSignals(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	cfg := config.DefaultConfig()
	workspace := filepath.Join(filepath.Dir(configPath), "workspace")
	cfg.Workspace = workspace
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	h := NewHandler(configPath)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	server := httptest.NewServer(mux)
	defer server.Close()

	postJSONURL(t, server.Client(), server.URL+"/api/v1/prds:compile", map[string]any{
		"requirement_text": "做一个简单记账 app，不考虑上架，只考虑功能实现，需要首页概览、记一笔和账单列表。",
		"job_id":           "job-delivery-record",
		"prd_id":           "prd-delivery-record",
	}, http.StatusOK)
	postJSONURL(t, server.Client(), server.URL+"/api/v1/jobs", map[string]any{
		"prd_id":      "prd-delivery-record",
		"template_id": "flutter-finance-lite",
	}, http.StatusOK)
	postJSONURL(t, server.Client(), server.URL+"/internal/v1/builders:register", map[string]any{
		"builder_id":        "builder-a",
		"display_name":      "Builder A",
		"capability_tags":   []string{"flutter"},
		"max_parallel_runs": 1,
		"worker_profile": map[string]any{
			"image": "builder:latest",
		},
	}, http.StatusOK)
	allocated := postJSONURL(t, server.Client(), server.URL+"/internal/v1/workers:allocate", map[string]any{
		"job_id": "job-delivery-record",
	}, http.StatusOK)
	leaseID, _ := allocated["lease_id"].(string)
	createdRun := postJSONURL(t, server.Client(), server.URL+"/internal/v1/build-runs", map[string]any{
		"worker_id":     "builder-a",
		"lease_id":      leaseID,
		"builder_input": sampleBuildRunInputFor("job-delivery-record", "prd-delivery-record", "flutter-finance-lite"),
	}, http.StatusOK)
	runID, _ := createdRun["run_id"].(string)
	finishedAt := time.Now().UTC()
	startedAt := finishedAt.Add(-30 * time.Second)
	postJSONURL(t, server.Client(), server.URL+"/internal/v1/build-runs/"+runID+"/complete", map[string]any{
		"builder_output": map[string]any{
			"schema_version": "0.1.0",
			"job_id":         "job-delivery-record",
			"status":         "success",
			"exit_reason":    "completed",
			"worker_id":      "builder-a",
			"started_at":     startedAt.Format(time.RFC3339),
			"finished_at":    finishedAt.Format(time.RFC3339),
			"final_summary":  "builder completed for delivery recording",
			"modified_files": []any{},
			"checks_passed": []map[string]any{{
				"check_id": "manual-complete",
				"label":    "manual complete",
				"stage":    "baseline",
				"outcome":  "passed",
			}},
			"checks_failed":      []any{},
			"next_human_actions": []any{},
			"artifacts": map[string]any{
				"manifest_path":   "",
				"primary_outputs": []any{},
			},
			"metrics": map[string]any{
				"metrics_path":                "",
				"total_iterations":            1,
				"total_tokens":                0,
				"distinct_failure_signatures": 0,
			},
			"report_paths": map[string]any{
				"change_summary_path":    "",
				"build_report_path":      "",
				"smoke_test_report_path": "",
			},
		},
	}, http.StatusOK)

	recorded := postJSONURL(t, server.Client(), server.URL+"/internal/v1/deliveries:record", map[string]any{
		"run_id":                             runID,
		"reviewer_id":                        "qa-owner",
		"status":                             "staged",
		"release_channel":                    "canary",
		"rollout_percent":                    20,
		"summary":                            "canary rollout approved",
		"evidence_paths":                     []string{"reports/review-bundle.md", "reports/handoff-checklist.md"},
		"signed_artifact_paths":              []string{"artifacts/app-signed.apk"},
		"device_verification_status":         "pending",
		"device_verification_summary":        "waiting for device validation",
		"device_verification_evidence_paths": []string{"reports/smoke-test-report.md"},
	}, http.StatusOK)
	if recorded["ack"] != true {
		t.Fatalf("ack = %v, want true", recorded["ack"])
	}
	if recorded["delivery_record_path"] != "reports/delivery-record.json" {
		t.Fatalf("delivery_record_path = %v, want reports/delivery-record.json", recorded["delivery_record_path"])
	}

	delivery := requireJSONObjectFile(t, filepath.Join(workspace, "appfactory", "jobs", "job-delivery-record", "reports", "delivery-record.json"))
	requireExactObjectKeys(t, delivery, []string{
		"schema_version",
		"job_id",
		"run_id",
		"status",
		"summary",
		"next_action",
		"release_channel",
		"rollout_percent",
		"reviewer_id",
		"evidence_paths",
		"signed_artifact_paths",
		"device_verification",
		"recorded_at",
	})
	if delivery["status"] != "staged" {
		t.Fatalf("delivery status = %v, want staged", delivery["status"])
	}
	if delivery["release_channel"] != "canary" {
		t.Fatalf("delivery release_channel = %v, want canary", delivery["release_channel"])
	}
	deviceVerification, ok := delivery["device_verification"].(map[string]any)
	if !ok {
		t.Fatalf("delivery.device_verification = %T, want map[string]any", delivery["device_verification"])
	}
	if deviceVerification["status"] != "pending" {
		t.Fatalf("device_verification.status = %v, want pending", deviceVerification["status"])
	}

	manifest := getJSONURL(t, server.Client(), server.URL+"/api/v1/jobs/job-delivery-record/artifacts", http.StatusOK)
	items, ok := manifest["items"].([]any)
	if !ok || len(items) < 9 {
		t.Fatalf("items = %v, want at least 9 artifacts", manifest["items"])
	}
	foundDeliveryRecord := false
	foundDeviceVerification := false
	for _, item := range items {
		artifact, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if artifact["artifact_id"] == "delivery-record" {
			foundDeliveryRecord = true
			if artifact["path"] != "reports/delivery-record.json" {
				t.Fatalf("delivery-record path = %v, want reports/delivery-record.json", artifact["path"])
			}
		}
		if artifact["artifact_id"] == "device-verification" {
			foundDeviceVerification = true
			if artifact["path"] != "reports/device-verification.json" {
				t.Fatalf("device-verification path = %v, want reports/device-verification.json", artifact["path"])
			}
		}
	}
	if !foundDeliveryRecord {
		t.Fatalf("delivery-record artifact missing from manifest: %v", items)
	}
	if !foundDeviceVerification {
		t.Fatalf("device-verification artifact missing from manifest: %v", items)
	}

	job := getJSONURL(t, server.Client(), server.URL+"/api/v1/jobs/job-delivery-record", http.StatusOK)
	deliveryContext, ok := job["delivery_context"].(map[string]any)
	if !ok {
		t.Fatalf("delivery_context = %v, want object", job["delivery_context"])
	}
	if deliveryContext["status"] != "staged" {
		t.Fatalf("delivery_context.status = %v, want staged", deliveryContext["status"])
	}
	if deliveryContext["delivery_record_path"] != "jobs/job-delivery-record/reports/delivery-record.json" {
		t.Fatalf("delivery_context.delivery_record_path = %v, want jobs/job-delivery-record/reports/delivery-record.json", deliveryContext["delivery_record_path"])
	}
	if deliveryContext["suggested_action"] != "verify_staged_release" {
		t.Fatalf("delivery_context.suggested_action = %v, want verify_staged_release", deliveryContext["suggested_action"])
	}
	deviceVerificationContext, ok := deliveryContext["device_verification"].(map[string]any)
	if !ok {
		t.Fatalf("delivery_context.device_verification = %T, want map[string]any", deliveryContext["device_verification"])
	}
	if deviceVerificationContext["status"] != "pending" {
		t.Fatalf("delivery_context.device_verification.status = %v, want pending", deviceVerificationContext["status"])
	}

	events := getJSONURL(t, server.Client(), server.URL+"/api/v1/jobs/job-delivery-record/events", http.StatusOK)
	eventItems, ok := events["items"].([]any)
	if !ok {
		t.Fatalf("event items = %v, want array", events["items"])
	}
	foundDeliveryEvent := false
	for _, raw := range eventItems {
		event, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if event["type"] == "delivery_staged" {
			foundDeliveryEvent = true
			if event["delivery_status"] != "staged" {
				t.Fatalf("delivery event status = %v, want staged", event["delivery_status"])
			}
			if event["release_channel"] != "canary" {
				t.Fatalf("delivery event release_channel = %v, want canary", event["release_channel"])
			}
		}
	}
	if !foundDeliveryEvent {
		t.Fatalf("delivery_staged event missing: %v", eventItems)
	}
	foundDeviceVerificationEvent := false
	for _, raw := range eventItems {
		event, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if event["type"] == "device_verification_updated" {
			foundDeviceVerificationEvent = true
			if event["device_verification_status"] != "pending" {
				t.Fatalf("device verification event status = %v, want pending", event["device_verification_status"])
			}
		}
	}
	if !foundDeviceVerificationEvent {
		t.Fatalf("device_verification_updated event missing: %v", eventItems)
	}

	notifications := getJSONURL(t, server.Client(), server.URL+"/api/v1/notifications?job_id=job-delivery-record", http.StatusOK)
	notificationItems, ok := notifications["items"].([]any)
	if !ok {
		t.Fatalf("notification items = %v, want array", notifications["items"])
	}
	foundDeliveryNotification := false
	for _, raw := range notificationItems {
		notification, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if notification["type"] == "delivery_staged" {
			foundDeliveryNotification = true
			if notification["suggested_action"] != "verify_staged_release" {
				t.Fatalf("delivery_staged suggested_action = %v, want verify_staged_release", notification["suggested_action"])
			}
			if notification["delivery_record_path"] != "jobs/job-delivery-record/reports/delivery-record.json" {
				t.Fatalf("delivery notification path = %v, want jobs/job-delivery-record/reports/delivery-record.json", notification["delivery_record_path"])
			}
		}
	}
	if !foundDeliveryNotification {
		t.Fatalf("delivery_staged notification missing: %v", notificationItems)
	}
	foundDeviceVerificationNotification := false
	for _, raw := range notificationItems {
		notification, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if notification["type"] == "device_verification_updated" {
			foundDeviceVerificationNotification = true
			if notification["device_verification_status"] != "pending" {
				t.Fatalf("device verification notification status = %v, want pending", notification["device_verification_status"])
			}
		}
	}
	if !foundDeviceVerificationNotification {
		t.Fatalf("device_verification_updated notification missing: %v", notificationItems)
	}
}

func TestRecordDeliveryFollowUpPersistsAndProjectsSignals(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	cfg := config.DefaultConfig()
	workspace := filepath.Join(filepath.Dir(configPath), "workspace")
	cfg.Workspace = workspace
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	h := NewHandler(configPath)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	server := httptest.NewServer(mux)
	defer server.Close()

	postJSONURL(t, server.Client(), server.URL+"/api/v1/prds:compile", map[string]any{
		"requirement_text": "做一个简单记账 app，不考虑上架，只考虑功能实现，需要首页概览、记一笔和账单列表。",
		"job_id":           "job-delivery-follow-up",
		"prd_id":           "prd-delivery-follow-up",
	}, http.StatusOK)
	postJSONURL(t, server.Client(), server.URL+"/api/v1/jobs", map[string]any{
		"prd_id":      "prd-delivery-follow-up",
		"template_id": "flutter-finance-lite",
	}, http.StatusOK)
	postJSONURL(t, server.Client(), server.URL+"/internal/v1/builders:register", map[string]any{
		"builder_id":        "builder-a",
		"display_name":      "Builder A",
		"capability_tags":   []string{"flutter"},
		"max_parallel_runs": 1,
		"worker_profile": map[string]any{
			"image": "builder:latest",
		},
	}, http.StatusOK)
	allocated := postJSONURL(t, server.Client(), server.URL+"/internal/v1/workers:allocate", map[string]any{
		"job_id": "job-delivery-follow-up",
	}, http.StatusOK)
	leaseID, _ := allocated["lease_id"].(string)
	createdRun := postJSONURL(t, server.Client(), server.URL+"/internal/v1/build-runs", map[string]any{
		"worker_id":     "builder-a",
		"lease_id":      leaseID,
		"builder_input": sampleBuildRunInputFor("job-delivery-follow-up", "prd-delivery-follow-up", "flutter-finance-lite"),
	}, http.StatusOK)
	runID, _ := createdRun["run_id"].(string)
	finishedAt := time.Now().UTC()
	startedAt := finishedAt.Add(-30 * time.Second)
	postJSONURL(t, server.Client(), server.URL+"/internal/v1/build-runs/"+runID+"/complete", map[string]any{
		"builder_output": map[string]any{
			"schema_version":     "0.1.0",
			"job_id":             "job-delivery-follow-up",
			"status":             "success",
			"exit_reason":        "completed",
			"worker_id":          "builder-a",
			"started_at":         startedAt.Format(time.RFC3339),
			"finished_at":        finishedAt.Format(time.RFC3339),
			"final_summary":      "builder completed for delivery follow-up",
			"modified_files":     []any{},
			"checks_passed":      []any{},
			"checks_failed":      []any{},
			"next_human_actions": []any{},
			"artifacts":          map[string]any{"manifest_path": "", "primary_outputs": []any{}},
			"metrics":            map[string]any{"metrics_path": "", "total_iterations": 1, "total_tokens": 0, "distinct_failure_signatures": 0},
			"report_paths":       map[string]any{"change_summary_path": "", "build_report_path": "", "smoke_test_report_path": ""},
		},
	}, http.StatusOK)

	postJSONURL(t, server.Client(), server.URL+"/internal/v1/deliveries:record", map[string]any{
		"run_id":          runID,
		"reviewer_id":     "release-owner",
		"status":          "released",
		"release_channel": "production",
		"summary":         "production release approved",
		"evidence_paths":  []string{"reports/review-bundle.md"},
	}, http.StatusOK)

	followUp := postJSONURL(t, server.Client(), server.URL+"/internal/v1/deliveries:follow-up", map[string]any{
		"run_id":         runID,
		"owner_id":       "release-duty",
		"status":         "monitoring",
		"summary":        "monitoring production feedback",
		"evidence_paths": []string{"reports/release-feedback.md"},
	}, http.StatusOK)
	if followUp["follow_up_record_path"] != "reports/release-follow-up.json" {
		t.Fatalf("follow_up_record_path = %v, want reports/release-follow-up.json", followUp["follow_up_record_path"])
	}

	delivery := requireJSONObjectFile(t, filepath.Join(workspace, "appfactory", "jobs", "job-delivery-follow-up", "reports", "delivery-record.json"))
	releaseFollowUp, ok := delivery["release_follow_up"].(map[string]any)
	if !ok {
		t.Fatalf("delivery.release_follow_up = %T, want map[string]any", delivery["release_follow_up"])
	}
	if releaseFollowUp["status"] != "monitoring" {
		t.Fatalf("release_follow_up.status = %v, want monitoring", releaseFollowUp["status"])
	}

	job := getJSONURL(t, server.Client(), server.URL+"/api/v1/jobs/job-delivery-follow-up", http.StatusOK)
	deliveryContext, ok := job["delivery_context"].(map[string]any)
	if !ok {
		t.Fatalf("delivery_context = %v, want object", job["delivery_context"])
	}
	followUpContext, ok := deliveryContext["release_follow_up"].(map[string]any)
	if !ok {
		t.Fatalf("delivery_context.release_follow_up = %T, want map[string]any", deliveryContext["release_follow_up"])
	}
	if followUpContext["status"] != "monitoring" {
		t.Fatalf("delivery_context.release_follow_up.status = %v, want monitoring", followUpContext["status"])
	}

	events := getJSONURL(t, server.Client(), server.URL+"/api/v1/jobs/job-delivery-follow-up/events", http.StatusOK)
	eventItems, _ := events["items"].([]any)
	foundFollowUpEvent := false
	for _, raw := range eventItems {
		event, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if event["type"] == "delivery_follow_up_updated" {
			foundFollowUpEvent = true
			if event["release_follow_up_status"] != "monitoring" {
				t.Fatalf("release_follow_up event status = %v, want monitoring", event["release_follow_up_status"])
			}
		}
	}
	if !foundFollowUpEvent {
		t.Fatalf("delivery_follow_up_updated event missing: %v", eventItems)
	}

	notifications := getJSONURL(t, server.Client(), server.URL+"/api/v1/notifications?job_id=job-delivery-follow-up", http.StatusOK)
	notificationItems, _ := notifications["items"].([]any)
	foundFollowUpNotification := false
	for _, raw := range notificationItems {
		notification, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if notification["type"] == "delivery_follow_up_updated" {
			foundFollowUpNotification = true
			if notification["release_follow_up_status"] != "monitoring" {
				t.Fatalf("release_follow_up notification status = %v, want monitoring", notification["release_follow_up_status"])
			}
		}
	}
	if !foundFollowUpNotification {
		t.Fatalf("delivery_follow_up_updated notification missing: %v", notificationItems)
	}
}

func TestQueuedPublicJobExecutionRecoversAfterHandlerRestart(t *testing.T) {
	configPath := filepath.Join(newRetriableTempDir(t), "config.json")
	cfg := config.DefaultConfig()
	workspace := filepath.Join(filepath.Dir(configPath), "workspace")
	cfg.Workspace = workspace
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	h := NewHandler(configPath)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	server := httptest.NewServer(mux)
	defer server.Close()

	postJSONURL(t, server.Client(), server.URL+"/api/v1/prds:compile", map[string]any{
		"requirement_text": "做一个简单记账 app，不考虑上架，只考虑功能实现，需要首页概览、记一笔和账单列表。",
		"job_id":           "job-orchestrator-recover-queued",
		"prd_id":           "prd-orchestrator-recover-queued",
	}, http.StatusOK)
	postJSONURL(t, server.Client(), server.URL+"/api/v1/jobs", map[string]any{
		"prd_id":      "prd-orchestrator-recover-queued",
		"template_id": "flutter-finance-lite",
	}, http.StatusOK)
	postJSONURL(t, server.Client(), server.URL+"/internal/v1/builders:register", map[string]any{
		"builder_id":        "builder-a",
		"display_name":      "Builder A",
		"capability_tags":   []string{"flutter"},
		"max_parallel_runs": 1,
		"worker_profile": map[string]any{
			"image": "builder:latest",
		},
	}, http.StatusOK)

	builderSvc, err := h.buildersControlPlane()
	if err != nil {
		t.Fatalf("buildersControlPlane() error = %v", err)
	}
	runsSvc, err := h.buildRunsControlPlane()
	if err != nil {
		t.Fatalf("buildRunsControlPlane() error = %v", err)
	}
	bundle, err := h.loadPreparedBundleByJobID("job-orchestrator-recover-queued")
	if err != nil {
		t.Fatalf("loadPreparedBundleByJobID() error = %v", err)
	}
	preparedInput := bundle.BuilderInput
	preparedInput.ContextSourceDir = bundle.PrepareDir
	dispatch, err := builderSvc.Dispatch(context.Background(), appbuilders.Requirement{JobID: "job-orchestrator-recover-queued"})
	if err != nil {
		t.Fatalf("Dispatch() error = %v", err)
	}
	run, err := runsSvc.Create(context.Background(), dispatch.Node.BuilderID, dispatch.Node.BuilderID, dispatch.Lease.LeaseID, preparedInput)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if err := builderSvc.BindRun(context.Background(), dispatch.Lease.LeaseID, run.RunID); err != nil {
		t.Fatalf("BindRun() error = %v", err)
	}
	if err := h.persistPublicJobExecutionRecord(publicJobExecutionRecord{
		SchemaVersion:  "0.1.0",
		JobID:          "job-orchestrator-recover-queued",
		RunID:          run.RunID,
		Action:         "start",
		Status:         "queued",
		TimeoutSeconds: 300,
		RequestedAt:    time.Now().UTC().Format(time.RFC3339),
	}); err != nil {
		t.Fatalf("persistPublicJobExecutionRecord() error = %v", err)
	}

	restarted := NewHandler(configPath)
	restarted.EnableStartupOrchestrator()
	t.Cleanup(restarted.waitForAsyncJobs)
	restartedMux := http.NewServeMux()
	restarted.RegisterRoutes(restartedMux)
	restartedServer := httptest.NewServer(restartedMux)
	defer restartedServer.Close()

	completed := waitForJobStatus(t, restartedServer.Client(), restartedServer.URL+"/api/v1/jobs/job-orchestrator-recover-queued", "completed", 5*time.Second)
	if completed["builder_output_path"] == "" {
		t.Fatal("builder_output_path should not be empty after recovered queued execution")
	}
	execution := waitForPublicJobExecutionStatus(t, workspace, "job-orchestrator-recover-queued", "completed", 5*time.Second)
	if execution.RunID != run.RunID {
		t.Fatalf("execution run_id = %q, want %q", execution.RunID, run.RunID)
	}
	if execution.StartedAt == "" || execution.FinishedAt == "" {
		t.Fatalf("execution timestamps = %+v, want started_at and finished_at", execution)
	}
	if execution.AttemptCount != 1 {
		t.Fatalf("attempt_count = %d, want 1", execution.AttemptCount)
	}
	if len(execution.Attempts) != 1 {
		t.Fatalf("attempts len = %d, want 1", len(execution.Attempts))
	}
	if execution.Attempts[0].Attempt != 1 || execution.Attempts[0].ReleaseReason != "completed" {
		t.Fatalf("attempt audit = %+v, want attempt=1 release_reason=completed", execution.Attempts[0])
	}
	if execution.LeaseOwnerID != "" {
		t.Fatalf("lease_owner_id = %q, want cleared after completion", execution.LeaseOwnerID)
	}
}

func TestClaimPublicJobExecutionLeaseInitializesAttemptCount(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	cfg := config.DefaultConfig()
	workspace := filepath.Join(filepath.Dir(configPath), "workspace")
	cfg.Workspace = workspace
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	h := NewHandler(configPath)
	if err := h.persistPublicJobExecutionRecord(publicJobExecutionRecord{
		SchemaVersion:  "0.1.0",
		JobID:          "job-claim-attempt-init",
		RunID:          "run-claim-attempt-init",
		Action:         "start",
		Status:         "queued",
		TimeoutSeconds: 300,
		RequestedAt:    time.Now().UTC().Format(time.RFC3339),
	}); err != nil {
		t.Fatalf("persistPublicJobExecutionRecord() error = %v", err)
	}

	execution, err := h.claimPublicJobExecutionLease(workspace, "job-claim-attempt-init")
	if err != nil {
		t.Fatalf("claimPublicJobExecutionLease() error = %v", err)
	}
	if execution == nil {
		t.Fatal("claimPublicJobExecutionLease() = nil, want execution record")
	}
	if execution.AttemptCount != 1 {
		t.Fatalf("attempt_count = %d, want 1", execution.AttemptCount)
	}
	if len(execution.Attempts) != 1 {
		t.Fatalf("attempts len = %d, want 1", len(execution.Attempts))
	}
	if execution.Attempts[0].OwnerID != h.orchestratorInstanceID {
		t.Fatalf("attempt owner_id = %q, want %q", execution.Attempts[0].OwnerID, h.orchestratorInstanceID)
	}
	if execution.Attempts[0].ClaimedAt == "" || execution.Attempts[0].LastRenewedAt == "" {
		t.Fatalf("attempt audit = %+v, want claimed_at and last_renewed_at", execution.Attempts[0])
	}
	if execution.LeaseOwnerID != h.orchestratorInstanceID {
		t.Fatalf("lease_owner_id = %q, want %q", execution.LeaseOwnerID, h.orchestratorInstanceID)
	}
	if execution.LeaseAcquiredAt == "" || execution.LeaseExpiresAt == "" {
		t.Fatalf("lease timestamps = %+v, want acquired_at and expires_at", execution)
	}
}

func TestClaimPublicJobExecutionLeaseKeepsAttemptCountForSameLiveOwner(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	cfg := config.DefaultConfig()
	workspace := filepath.Join(filepath.Dir(configPath), "workspace")
	cfg.Workspace = workspace
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	h := NewHandler(configPath)
	acquiredAt := time.Now().UTC().Add(-15 * time.Second).Format(time.RFC3339)
	if err := h.persistPublicJobExecutionRecord(publicJobExecutionRecord{
		SchemaVersion:   "0.1.0",
		JobID:           "job-claim-attempt-stable",
		RunID:           "run-claim-attempt-stable",
		Action:          "start",
		Status:          "queued",
		AttemptCount:    2,
		LeaseOwnerID:    h.orchestratorInstanceID,
		LeaseAcquiredAt: acquiredAt,
		LeaseExpiresAt:  time.Now().UTC().Add(90 * time.Second).Format(time.RFC3339),
		TimeoutSeconds:  300,
		RequestedAt:     time.Now().UTC().Add(-15 * time.Second).Format(time.RFC3339),
	}); err != nil {
		t.Fatalf("persistPublicJobExecutionRecord() error = %v", err)
	}

	execution, err := h.claimPublicJobExecutionLease(workspace, "job-claim-attempt-stable")
	if err != nil {
		t.Fatalf("claimPublicJobExecutionLease() error = %v", err)
	}
	if execution == nil {
		t.Fatal("claimPublicJobExecutionLease() = nil, want execution record")
	}
	if execution.AttemptCount != 2 {
		t.Fatalf("attempt_count = %d, want 2", execution.AttemptCount)
	}
	if len(execution.Attempts) != 1 {
		t.Fatalf("attempts len = %d, want 1 for same live owner", len(execution.Attempts))
	}
	if execution.LeaseAcquiredAt != acquiredAt {
		t.Fatalf("lease_acquired_at = %q, want %q", execution.LeaseAcquiredAt, acquiredAt)
	}
}

func TestClaimPublicJobExecutionLeaseRecordsHandoffAfterExpiry(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	cfg := config.DefaultConfig()
	workspace := filepath.Join(filepath.Dir(configPath), "workspace")
	cfg.Workspace = workspace
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	h := NewHandler(configPath)
	claimedAt := time.Now().UTC().Add(-5 * time.Minute).Format(time.RFC3339)
	if err := h.persistPublicJobExecutionRecord(publicJobExecutionRecord{
		SchemaVersion:   "0.1.0",
		JobID:           "job-claim-handoff",
		RunID:           "run-claim-handoff",
		Action:          "resume",
		Status:          "queued",
		AttemptCount:    1,
		LeaseOwnerID:    "orchestrator-foreign",
		LeaseAcquiredAt: claimedAt,
		LeaseExpiresAt:  time.Now().UTC().Add(-3 * time.Minute).Format(time.RFC3339),
		TimeoutSeconds:  300,
		RequestedAt:     time.Now().UTC().Add(-5 * time.Minute).Format(time.RFC3339),
		Attempts: []publicJobExecutionAttempt{{
			Attempt:       1,
			OwnerID:       "orchestrator-foreign",
			ClaimedAt:     claimedAt,
			LastRenewedAt: time.Now().UTC().Add(-4 * time.Minute).Format(time.RFC3339),
		}},
	}); err != nil {
		t.Fatalf("persistPublicJobExecutionRecord() error = %v", err)
	}

	execution, err := h.claimPublicJobExecutionLease(workspace, "job-claim-handoff")
	if err != nil {
		t.Fatalf("claimPublicJobExecutionLease() error = %v", err)
	}
	if execution == nil {
		t.Fatal("claimPublicJobExecutionLease() = nil, want execution record")
	}
	if execution.AttemptCount != 2 {
		t.Fatalf("attempt_count = %d, want 2", execution.AttemptCount)
	}
	if len(execution.Attempts) != 2 {
		t.Fatalf("attempts len = %d, want 2", len(execution.Attempts))
	}
	if execution.Attempts[0].ReleaseReason != "handoff_after_lease_expiry" || execution.Attempts[0].ReleasedAt == "" {
		t.Fatalf("first attempt audit = %+v, want handoff_after_lease_expiry with released_at", execution.Attempts[0])
	}
	if execution.Attempts[1].Attempt != 2 || execution.Attempts[1].OwnerID != h.orchestratorInstanceID {
		t.Fatalf("second attempt audit = %+v, want attempt=2 owner=%q", execution.Attempts[1], h.orchestratorInstanceID)
	}
}

func TestQueuedPublicJobExecutionRecoverySkipsForeignLiveLease(t *testing.T) {
	configPath := filepath.Join(newRetriableTempDir(t), "config.json")
	cfg := config.DefaultConfig()
	workspace := filepath.Join(filepath.Dir(configPath), "workspace")
	cfg.Workspace = workspace
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	h := NewHandler(configPath)
	t.Cleanup(h.waitForAsyncJobs)
	if err := h.persistPublicJobExecutionRecord(publicJobExecutionRecord{
		SchemaVersion:   "0.1.0",
		JobID:           "job-foreign-live-lease",
		RunID:           "run-foreign-live-lease",
		Action:          "start",
		Status:          "queued",
		LeaseOwnerID:    "orchestrator-foreign",
		LeaseAcquiredAt: time.Now().UTC().Add(-10 * time.Second).Format(time.RFC3339),
		LeaseExpiresAt:  time.Now().UTC().Add(90 * time.Second).Format(time.RFC3339),
		TimeoutSeconds:  300,
		RequestedAt:     time.Now().UTC().Add(-10 * time.Second).Format(time.RFC3339),
	}); err != nil {
		t.Fatalf("persistPublicJobExecutionRecord() error = %v", err)
	}

	restarted := NewHandler(configPath)
	t.Cleanup(restarted.waitForAsyncJobs)
	restartedMux := http.NewServeMux()
	restarted.RegisterRoutes(restartedMux)
	restarted.waitForAsyncJobs()

	execution := readPublicJobExecutionRecord(t, workspace, "job-foreign-live-lease")
	if execution.Status != "queued" {
		t.Fatalf("execution status = %q, want queued", execution.Status)
	}
	if execution.LeaseOwnerID != "orchestrator-foreign" {
		t.Fatalf("lease_owner_id = %q, want orchestrator-foreign", execution.LeaseOwnerID)
	}
	if execution.AttemptCount != 0 {
		t.Fatalf("attempt_count = %d, want 0 when foreign live lease blocks recovery", execution.AttemptCount)
	}
	if len(execution.Attempts) != 0 {
		t.Fatalf("attempts len = %d, want 0 when foreign live lease blocks recovery", len(execution.Attempts))
	}
	if execution.StartedAt != "" {
		t.Fatalf("started_at = %q, want empty when foreign live lease blocks recovery", execution.StartedAt)
	}
}

func TestQueuedPublicJobExecutionRecoveryReclaimsExpiredLease(t *testing.T) {
	configPath := filepath.Join(newRetriableTempDir(t), "config.json")
	cfg := config.DefaultConfig()
	workspace := filepath.Join(filepath.Dir(configPath), "workspace")
	cfg.Workspace = workspace
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	h := NewHandler(configPath)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	server := httptest.NewServer(mux)
	defer server.Close()

	postJSONURL(t, server.Client(), server.URL+"/api/v1/prds:compile", map[string]any{
		"requirement_text": "做一个简单记账 app，不考虑上架，只考虑功能实现，需要首页概览、记一笔和账单列表。",
		"job_id":           "job-expired-lease-recover",
		"prd_id":           "prd-expired-lease-recover",
	}, http.StatusOK)
	postJSONURL(t, server.Client(), server.URL+"/api/v1/jobs", map[string]any{
		"prd_id":      "prd-expired-lease-recover",
		"template_id": "flutter-finance-lite",
	}, http.StatusOK)
	postJSONURL(t, server.Client(), server.URL+"/internal/v1/builders:register", map[string]any{
		"builder_id":        "builder-a",
		"display_name":      "Builder A",
		"capability_tags":   []string{"flutter"},
		"max_parallel_runs": 1,
		"worker_profile": map[string]any{
			"image": "builder:latest",
		},
	}, http.StatusOK)

	builderSvc, err := h.buildersControlPlane()
	if err != nil {
		t.Fatalf("buildersControlPlane() error = %v", err)
	}
	runsSvc, err := h.buildRunsControlPlane()
	if err != nil {
		t.Fatalf("buildRunsControlPlane() error = %v", err)
	}
	bundle, err := h.loadPreparedBundleByJobID("job-expired-lease-recover")
	if err != nil {
		t.Fatalf("loadPreparedBundleByJobID() error = %v", err)
	}
	preparedInput := bundle.BuilderInput
	preparedInput.ContextSourceDir = bundle.PrepareDir
	dispatch, err := builderSvc.Dispatch(context.Background(), appbuilders.Requirement{JobID: "job-expired-lease-recover"})
	if err != nil {
		t.Fatalf("Dispatch() error = %v", err)
	}
	run, err := runsSvc.Create(context.Background(), dispatch.Node.BuilderID, dispatch.Node.BuilderID, dispatch.Lease.LeaseID, preparedInput)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if err := builderSvc.BindRun(context.Background(), dispatch.Lease.LeaseID, run.RunID); err != nil {
		t.Fatalf("BindRun() error = %v", err)
	}
	if err := h.persistPublicJobExecutionRecord(publicJobExecutionRecord{
		SchemaVersion:   "0.1.0",
		JobID:           "job-expired-lease-recover",
		RunID:           run.RunID,
		Action:          "start",
		Status:          "queued",
		AttemptCount:    1,
		LeaseOwnerID:    "orchestrator-foreign",
		LeaseAcquiredAt: time.Now().UTC().Add(-5 * time.Minute).Format(time.RFC3339),
		LeaseExpiresAt:  time.Now().UTC().Add(-3 * time.Minute).Format(time.RFC3339),
		TimeoutSeconds:  300,
		RequestedAt:     time.Now().UTC().Add(-5 * time.Minute).Format(time.RFC3339),
		Attempts: []publicJobExecutionAttempt{{
			Attempt:       1,
			OwnerID:       "orchestrator-foreign",
			ClaimedAt:     time.Now().UTC().Add(-5 * time.Minute).Format(time.RFC3339),
			LastRenewedAt: time.Now().UTC().Add(-4 * time.Minute).Format(time.RFC3339),
		}},
	}); err != nil {
		t.Fatalf("persistPublicJobExecutionRecord() error = %v", err)
	}

	restarted := NewHandler(configPath)
	restarted.EnableStartupOrchestrator()
	t.Cleanup(restarted.waitForAsyncJobs)
	restartedMux := http.NewServeMux()
	restarted.RegisterRoutes(restartedMux)
	restartedServer := httptest.NewServer(restartedMux)
	defer restartedServer.Close()

	completed := waitForJobStatus(t, restartedServer.Client(), restartedServer.URL+"/api/v1/jobs/job-expired-lease-recover", "completed", 5*time.Second)
	if completed["builder_output_path"] == "" {
		t.Fatal("builder_output_path should not be empty after expired lease recovery")
	}
	execution := waitForPublicJobExecutionStatus(t, workspace, "job-expired-lease-recover", "completed", 5*time.Second)
	if execution.AttemptCount != 2 {
		t.Fatalf("attempt_count = %d, want 2 after expired lease handoff and reclaim", execution.AttemptCount)
	}
	if len(execution.Attempts) != 2 {
		t.Fatalf("attempts len = %d, want 2 after expired lease recovery", len(execution.Attempts))
	}
	if execution.Attempts[0].ReleaseReason != "handoff_after_lease_expiry" {
		t.Fatalf("first attempt audit = %+v, want handoff_after_lease_expiry", execution.Attempts[0])
	}
	if execution.Attempts[1].ReleaseReason != "completed" {
		t.Fatalf("second attempt audit = %+v, want completed release reason", execution.Attempts[1])
	}
	if execution.LeaseOwnerID != "" {
		t.Fatalf("lease_owner_id = %q, want cleared after completion", execution.LeaseOwnerID)
	}
	if execution.StartedAt == "" {
		t.Fatal("started_at should not be empty after expired lease recovery")
	}

	events := getJSONURL(t, restartedServer.Client(), restartedServer.URL+"/api/v1/jobs/job-expired-lease-recover/events", http.StatusOK)
	items, ok := events["items"].([]any)
	if !ok || len(items) == 0 {
		t.Fatalf("items = %v, want non-empty", events["items"])
	}
	foundAttemptClaimed := false
	foundAttemptReleased := false
	foundExecutionHandoff := false
	for _, item := range items {
		event, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if event["type"] == "execution_attempt_claimed" {
			foundAttemptClaimed = true
		}
		if event["type"] == "execution_attempt_released" {
			foundAttemptReleased = true
		}
		if event["type"] == "execution_handoff" {
			foundExecutionHandoff = true
		}
	}
	if !foundAttemptClaimed || !foundAttemptReleased || !foundExecutionHandoff {
		t.Fatalf("missing execution audit events: claimed=%v released=%v handoff=%v items=%v", foundAttemptClaimed, foundAttemptReleased, foundExecutionHandoff, items)
	}
}

func TestRunningPublicJobExecutionRecoverySkipsForeignLiveLeaseThenReclaimsExpiredLease(t *testing.T) {
	configPath := filepath.Join(newRetriableTempDir(t), "config.json")
	cfg := config.DefaultConfig()
	workspace := filepath.Join(filepath.Dir(configPath), "workspace")
	cfg.Workspace = workspace
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	h := NewHandler(configPath)
	t.Cleanup(h.waitForAsyncJobs)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	server := httptest.NewServer(mux)
	defer server.Close()

	postJSONURL(t, server.Client(), server.URL+"/api/v1/prds:compile", map[string]any{
		"requirement_text": "做一个简单记账 app，不考虑上架，只考虑功能实现，需要首页概览、记一笔和账单列表。",
		"job_id":           "job-running-lease-handoff",
		"prd_id":           "prd-running-lease-handoff",
	}, http.StatusOK)
	postJSONURL(t, server.Client(), server.URL+"/api/v1/jobs", map[string]any{
		"prd_id":      "prd-running-lease-handoff",
		"template_id": "flutter-finance-lite",
	}, http.StatusOK)
	postJSONURL(t, server.Client(), server.URL+"/internal/v1/builders:register", map[string]any{
		"builder_id":        "builder-a",
		"display_name":      "Builder A",
		"capability_tags":   []string{"flutter"},
		"max_parallel_runs": 1,
		"worker_profile": map[string]any{
			"image": "builder:latest",
		},
	}, http.StatusOK)

	builderSvc, err := h.buildersControlPlane()
	if err != nil {
		t.Fatalf("buildersControlPlane() error = %v", err)
	}
	runsSvc, err := h.buildRunsControlPlane()
	if err != nil {
		t.Fatalf("buildRunsControlPlane() error = %v", err)
	}
	bundle, err := h.loadPreparedBundleByJobID("job-running-lease-handoff")
	if err != nil {
		t.Fatalf("loadPreparedBundleByJobID() error = %v", err)
	}
	preparedInput := bundle.BuilderInput
	preparedInput.ContextSourceDir = bundle.PrepareDir
	dispatch, err := builderSvc.Dispatch(context.Background(), appbuilders.Requirement{JobID: "job-running-lease-handoff"})
	if err != nil {
		t.Fatalf("Dispatch() error = %v", err)
	}
	run, err := runsSvc.Create(context.Background(), dispatch.Node.BuilderID, dispatch.Node.BuilderID, dispatch.Lease.LeaseID, preparedInput)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if err := builderSvc.BindRun(context.Background(), dispatch.Lease.LeaseID, run.RunID); err != nil {
		t.Fatalf("BindRun() error = %v", err)
	}

	claimedAt := time.Now().UTC().Add(-5 * time.Minute).Format(time.RFC3339)
	lastRenewedAt := time.Now().UTC().Add(-4 * time.Minute).Format(time.RFC3339)
	if err := h.persistPublicJobExecutionRecord(publicJobExecutionRecord{
		SchemaVersion:   "0.1.0",
		JobID:           "job-running-lease-handoff",
		RunID:           run.RunID,
		Action:          "resume",
		Status:          "running",
		AttemptCount:    1,
		LeaseOwnerID:    "orchestrator-foreign",
		LeaseAcquiredAt: claimedAt,
		LeaseExpiresAt:  time.Now().UTC().Add(90 * time.Second).Format(time.RFC3339),
		TimeoutSeconds:  300,
		RequestedAt:     time.Now().UTC().Add(-5 * time.Minute).Format(time.RFC3339),
		StartedAt:       time.Now().UTC().Add(-4 * time.Minute).Format(time.RFC3339),
		Attempts: []publicJobExecutionAttempt{{
			Attempt:       1,
			OwnerID:       "orchestrator-foreign",
			ClaimedAt:     claimedAt,
			LastRenewedAt: lastRenewedAt,
		}},
	}); err != nil {
		t.Fatalf("persistPublicJobExecutionRecord() error = %v", err)
	}

	restarted := NewHandler(configPath)
	t.Cleanup(restarted.waitForAsyncJobs)
	restartedMux := http.NewServeMux()
	restarted.RegisterRoutes(restartedMux)
	restartedServer := httptest.NewServer(restartedMux)
	defer restartedServer.Close()

	firstStatus, err := restarted.RunPublicJobOrchestratorPass("test_running_foreign_lease_skip")
	if err != nil {
		t.Fatalf("RunPublicJobOrchestratorPass(skip) error = %v", err)
	}
	restarted.waitForAsyncJobs()
	if firstStatus.ForeignLiveLeaseSkips != 1 {
		t.Fatalf("foreign_live_lease_skips = %d, want 1", firstStatus.ForeignLiveLeaseSkips)
	}
	execution := readPublicJobExecutionRecord(t, workspace, "job-running-lease-handoff")
	if execution.Status != "running" {
		t.Fatalf("execution status = %q, want running after foreign live lease skip", execution.Status)
	}
	if execution.LeaseOwnerID != "orchestrator-foreign" {
		t.Fatalf("lease_owner_id = %q, want orchestrator-foreign after skip", execution.LeaseOwnerID)
	}
	if execution.AttemptCount != 1 || len(execution.Attempts) != 1 {
		t.Fatalf("attempt audit = %+v, want single foreign attempt after skip", execution.Attempts)
	}
	if execution.Attempts[0].ReleasedAt != "" {
		t.Fatalf("released_at = %q, want empty before lease expiry handoff", execution.Attempts[0].ReleasedAt)
	}

	execution.LeaseExpiresAt = time.Now().UTC().Add(-2 * time.Minute).Format(time.RFC3339)
	if err := restarted.persistPublicJobExecutionRecord(execution); err != nil {
		t.Fatalf("persistPublicJobExecutionRecord(expired) error = %v", err)
	}

	secondStatus, err := restarted.RunPublicJobOrchestratorPass("test_running_expired_lease_reclaim")
	if err != nil {
		t.Fatalf("RunPublicJobOrchestratorPass(reclaim) error = %v", err)
	}
	restarted.waitForAsyncJobs()
	if secondStatus.ForeignLiveLeaseSkips != firstStatus.ForeignLiveLeaseSkips {
		t.Fatalf("foreign_live_lease_skips = %d, want unchanged %d after lease expiry reclaim", secondStatus.ForeignLiveLeaseSkips, firstStatus.ForeignLiveLeaseSkips)
	}

	failed := waitForJobStatus(t, restartedServer.Client(), restartedServer.URL+"/api/v1/jobs/job-running-lease-handoff", "failed", 5*time.Second)
	failureContext, ok := failed["failure_context"].(map[string]any)
	if !ok {
		t.Fatalf("failure_context = %T, want map[string]any", failed["failure_context"])
	}
	if !strings.Contains(strings.TrimSpace(toString(failureContext["last_error_summary"])), "dispatcher") {
		t.Fatalf("last_error_summary = %v, want dispatcher-lost summary", failureContext["last_error_summary"])
	}
	if failureContext["failure_category"] != "execution_interrupted" {
		t.Fatalf("failure_category = %v, want execution_interrupted", failureContext["failure_category"])
	}
	statusContext, ok := failed["status_context"].(map[string]any)
	if !ok {
		t.Fatalf("status_context = %T, want map[string]any", failed["status_context"])
	}
	if statusContext["reason_code"] != "execution_interrupted" {
		t.Fatalf("reason_code = %v, want execution_interrupted", statusContext["reason_code"])
	}
	if statusContext["suggested_action"] != "resume_interrupted_job" {
		t.Fatalf("suggested_action = %v, want resume_interrupted_job", statusContext["suggested_action"])
	}
	execution = waitForPublicJobExecutionStatus(t, workspace, "job-running-lease-handoff", "failed", 5*time.Second)
	if execution.AttemptCount != 2 {
		t.Fatalf("attempt_count = %d, want 2 after expiry handoff", execution.AttemptCount)
	}
	if len(execution.Attempts) != 2 {
		t.Fatalf("attempts len = %d, want 2 after expiry handoff", len(execution.Attempts))
	}
	if execution.Attempts[0].ReleaseReason != "handoff_after_lease_expiry" || execution.Attempts[0].ReleasedAt == "" {
		t.Fatalf("first attempt audit = %+v, want handoff_after_lease_expiry with released_at", execution.Attempts[0])
	}
	if execution.Attempts[1].OwnerID != restarted.orchestratorInstanceID {
		t.Fatalf("second attempt owner_id = %q, want %q", execution.Attempts[1].OwnerID, restarted.orchestratorInstanceID)
	}
	if execution.Attempts[1].ReleaseReason != "dispatcher_lost" {
		t.Fatalf("second attempt audit = %+v, want dispatcher_lost", execution.Attempts[1])
	}
	if execution.LeaseOwnerID != "" {
		t.Fatalf("lease_owner_id = %q, want cleared after dispatcher-lost recovery", execution.LeaseOwnerID)
	}
	if !strings.Contains(execution.LastError, "dispatcher") {
		t.Fatalf("execution last_error = %q, want dispatcher-lost message", execution.LastError)
	}

	recoveredRun, err := runsSvc.Get(context.Background(), run.RunID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if recoveredRun.Status != appruns.StatusFailed {
		t.Fatalf("run status = %s, want failed after handoff recovery", recoveredRun.Status)
	}

	events := getJSONURL(t, restartedServer.Client(), restartedServer.URL+"/api/v1/jobs/job-running-lease-handoff/events", http.StatusOK)
	items, ok := events["items"].([]any)
	if !ok || len(items) == 0 {
		t.Fatalf("items = %v, want non-empty", events["items"])
	}
	foundExecutionHandoff := false
	foundInterruptedEvent := false
	for _, item := range items {
		event, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if event["type"] == "execution_handoff" {
			foundExecutionHandoff = true
		}
		if event["type"] == "execution_interrupted" {
			foundInterruptedEvent = true
		}
	}
	if !foundExecutionHandoff || !foundInterruptedEvent {
		t.Fatalf("missing running recovery events: handoff=%v execution_interrupted=%v items=%v", foundExecutionHandoff, foundInterruptedEvent, items)
	}

	notifications := getJSONURL(t, restartedServer.Client(), restartedServer.URL+"/api/v1/notifications", http.StatusOK)
	notificationItems, ok := notifications["items"].([]any)
	if !ok || len(notificationItems) == 0 {
		t.Fatalf("notification items = %v, want non-empty", notifications["items"])
	}
	foundExecutionInterrupted := false
	foundExecutionHandoffNotification := false
	for _, item := range notificationItems {
		notification, ok := item.(map[string]any)
		if !ok || notification["job_id"] != "job-running-lease-handoff" {
			continue
		}
		if notification["type"] == "execution_interrupted" {
			foundExecutionInterrupted = true
		}
		if notification["type"] == "execution_handoff" {
			foundExecutionHandoffNotification = true
			if notification["suggested_action"] != "inspect_execution_history" {
				t.Fatalf("execution_handoff suggested_action = %v, want inspect_execution_history", notification["suggested_action"])
			}
		}
	}
	if !foundExecutionInterrupted || !foundExecutionHandoffNotification {
		t.Fatalf("missing running recovery notifications: interrupted=%v handoff=%v items=%v", foundExecutionInterrupted, foundExecutionHandoffNotification, notificationItems)
	}
}

func TestPublicJobExecutionLeaseHeartbeatRenewsExpiry(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	cfg := config.DefaultConfig()
	workspace := filepath.Join(filepath.Dir(configPath), "workspace")
	cfg.Workspace = workspace
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	previousWindow := publicJobExecutionLeaseWindow
	previousRenewInterval := publicJobExecutionLeaseRenewInterval
	publicJobExecutionLeaseWindow = 2 * time.Second
	publicJobExecutionLeaseRenewInterval = 1100 * time.Millisecond
	t.Cleanup(func() {
		publicJobExecutionLeaseWindow = previousWindow
		publicJobExecutionLeaseRenewInterval = previousRenewInterval
	})

	h := NewHandler(configPath)
	initialExpiry := time.Now().UTC().Add(1 * time.Second).Truncate(time.Second)
	record := publicJobExecutionRecord{
		SchemaVersion:   "0.1.0",
		JobID:           "job-lease-heartbeat",
		RunID:           "run-lease-heartbeat",
		Action:          "start",
		Status:          "running",
		AttemptCount:    1,
		LeaseOwnerID:    h.orchestratorInstanceID,
		LeaseAcquiredAt: time.Now().UTC().Format(time.RFC3339),
		LeaseExpiresAt:  initialExpiry.Format(time.RFC3339Nano),
		TimeoutSeconds:  300,
		RequestedAt:     time.Now().UTC().Format(time.RFC3339),
		StartedAt:       time.Now().UTC().Format(time.RFC3339),
	}
	if err := h.persistPublicJobExecutionRecord(record); err != nil {
		t.Fatalf("persistPublicJobExecutionRecord() error = %v", err)
	}

	var mu sync.Mutex
	stopHeartbeat := h.startPublicJobExecutionLeaseHeartbeat(context.Background(), workspace, &record, &mu)
	time.Sleep(1300 * time.Millisecond)
	stopHeartbeat()

	updated := readPublicJobExecutionRecord(t, workspace, "job-lease-heartbeat")
	if updated.AttemptCount != 1 {
		t.Fatalf("attempt_count = %d, want 1 after heartbeat renewals", updated.AttemptCount)
	}
	if len(updated.Attempts) != 1 {
		t.Fatalf("attempts len = %d, want 1 after heartbeat renewals", len(updated.Attempts))
	}
	if updated.Attempts[0].LastRenewedAt == "" {
		t.Fatalf("attempt audit = %+v, want last_renewed_at to be refreshed", updated.Attempts[0])
	}
	renewedExpiry, err := time.Parse(time.RFC3339Nano, updated.LeaseExpiresAt)
	if err != nil {
		t.Fatalf("Parse(lease_expires_at) error = %v", err)
	}
	if !renewedExpiry.After(initialExpiry) {
		t.Fatalf("lease_expires_at = %s, want after initial expiry %s", renewedExpiry.Format(time.RFC3339Nano), initialExpiry.Format(time.RFC3339Nano))
	}
}

func TestPublicJobExecutionLeaseHeartbeatDoesNotOverwriteCancelledExecution(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	cfg := config.DefaultConfig()
	workspace := filepath.Join(filepath.Dir(configPath), "workspace")
	cfg.Workspace = workspace
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	previousWindow := publicJobExecutionLeaseWindow
	previousRenewInterval := publicJobExecutionLeaseRenewInterval
	publicJobExecutionLeaseWindow = 2 * time.Second
	publicJobExecutionLeaseRenewInterval = 300 * time.Millisecond
	t.Cleanup(func() {
		publicJobExecutionLeaseWindow = previousWindow
		publicJobExecutionLeaseRenewInterval = previousRenewInterval
	})

	h := NewHandler(configPath)
	claimedAt := time.Now().UTC().Add(-10 * time.Second).Format(time.RFC3339)
	record := publicJobExecutionRecord{
		SchemaVersion:   "0.1.0",
		JobID:           "job-lease-heartbeat-cancelled",
		RunID:           "run-lease-heartbeat-cancelled",
		Action:          "start",
		Status:          "running",
		AttemptCount:    1,
		LeaseOwnerID:    h.orchestratorInstanceID,
		LeaseAcquiredAt: claimedAt,
		LeaseExpiresAt:  time.Now().UTC().Add(1 * time.Second).Format(time.RFC3339Nano),
		TimeoutSeconds:  300,
		RequestedAt:     time.Now().UTC().Add(-15 * time.Second).Format(time.RFC3339),
		StartedAt:       time.Now().UTC().Add(-10 * time.Second).Format(time.RFC3339),
		Attempts: []publicJobExecutionAttempt{{
			Attempt:       1,
			OwnerID:       h.orchestratorInstanceID,
			ClaimedAt:     claimedAt,
			LastRenewedAt: time.Now().UTC().Add(-1 * time.Second).Format(time.RFC3339),
		}},
	}
	if err := h.persistPublicJobExecutionRecord(record); err != nil {
		t.Fatalf("persistPublicJobExecutionRecord() error = %v", err)
	}

	var mu sync.Mutex
	stopHeartbeat := h.startPublicJobExecutionLeaseHeartbeat(context.Background(), workspace, &record, &mu)
	t.Cleanup(stopHeartbeat)
	time.Sleep(350 * time.Millisecond)

	if err := h.cancelPublicJobExecution("job-lease-heartbeat-cancelled", "cancelled while heartbeat still alive"); err != nil {
		t.Fatalf("cancelPublicJobExecution() error = %v", err)
	}
	time.Sleep(700 * time.Millisecond)
	stopHeartbeat()

	updated := readPublicJobExecutionRecord(t, workspace, "job-lease-heartbeat-cancelled")
	if updated.Status != "cancelled" {
		t.Fatalf("execution status = %q, want cancelled", updated.Status)
	}
	if updated.LeaseOwnerID != "" {
		t.Fatalf("lease_owner_id = %q, want cleared after cancel", updated.LeaseOwnerID)
	}
	if len(updated.Attempts) != 1 {
		t.Fatalf("attempts len = %d, want 1", len(updated.Attempts))
	}
	if updated.Attempts[0].ReleaseReason != "cancelled" || updated.Attempts[0].ReleasedAt == "" {
		t.Fatalf("attempt audit = %+v, want cancelled release reason with released_at", updated.Attempts[0])
	}
	if !strings.Contains(updated.LastError, "heartbeat still alive") {
		t.Fatalf("last_error = %q, want cancel reason retained", updated.LastError)
	}
	if updated.FinishedAt == "" {
		t.Fatal("finished_at should not be empty after cancel")
	}
}

func TestSyntheticPublicJobExecutionEventsEmitSpecializedReleaseTypes(t *testing.T) {
	record := &publicJobExecutionRecord{
		JobID:       "job-event-types",
		RunID:       "run-event-types",
		Action:      "resume",
		Status:      "failed",
		RequestedAt: time.Now().UTC().Add(-10 * time.Minute).Format(time.RFC3339),
		Attempts: []publicJobExecutionAttempt{
			{Attempt: 1, OwnerID: "orchestrator-a", ClaimedAt: time.Now().UTC().Add(-9 * time.Minute).Format(time.RFC3339), ReleasedAt: time.Now().UTC().Add(-8 * time.Minute).Format(time.RFC3339), ReleaseReason: "cancelled"},
			{Attempt: 2, OwnerID: "orchestrator-b", ClaimedAt: time.Now().UTC().Add(-7 * time.Minute).Format(time.RFC3339), ReleasedAt: time.Now().UTC().Add(-6 * time.Minute).Format(time.RFC3339), ReleaseReason: "dispatcher_lost"},
			{Attempt: 3, OwnerID: "orchestrator-c", ClaimedAt: time.Now().UTC().Add(-5 * time.Minute).Format(time.RFC3339), ReleasedAt: time.Now().UTC().Add(-4 * time.Minute).Format(time.RFC3339), ReleaseReason: "recovery_failed"},
			{Attempt: 4, OwnerID: "orchestrator-d", ClaimedAt: time.Now().UTC().Add(-3 * time.Minute).Format(time.RFC3339), ReleasedAt: time.Now().UTC().Add(-2 * time.Minute).Format(time.RFC3339), ReleaseReason: "failed"},
			{Attempt: 5, OwnerID: "orchestrator-e", ClaimedAt: time.Now().UTC().Add(-90 * time.Second).Format(time.RFC3339), ReleasedAt: time.Now().UTC().Add(-60 * time.Second).Format(time.RFC3339), ReleaseReason: "completed"},
		},
	}
	events := syntheticPublicJobExecutionEvents(record)
	foundTypes := map[string]bool{}
	for _, event := range events {
		foundTypes[event.Type] = true
	}
	for _, eventType := range []string{"execution_cancelled", "execution_interrupted", "execution_recovery_failed", "execution_failed", "execution_completed"} {
		if !foundTypes[eventType] {
			t.Fatalf("missing event type %q in events=%+v", eventType, events)
		}
	}
}

func TestGetPublicJobPrefersCancelledExecutionRecordWhenRunIsStaleRunning(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	cfg := config.DefaultConfig()
	workspace := filepath.Join(filepath.Dir(configPath), "workspace")
	cfg.Workspace = workspace
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	h := NewHandler(configPath)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	server := httptest.NewServer(mux)
	defer server.Close()

	postJSONURL(t, server.Client(), server.URL+"/api/v1/prds:compile", map[string]any{
		"requirement_text": "做一个简单记账 app，不考虑上架，只考虑功能实现，需要首页概览、记一笔和账单列表。",
		"job_id":           "job-stale-running-execution-cancelled",
		"prd_id":           "prd-stale-running-execution-cancelled",
	}, http.StatusOK)
	postJSONURL(t, server.Client(), server.URL+"/api/v1/jobs", map[string]any{
		"prd_id":      "prd-stale-running-execution-cancelled",
		"template_id": "flutter-finance-lite",
	}, http.StatusOK)
	postJSONURL(t, server.Client(), server.URL+"/internal/v1/builders:register", map[string]any{
		"builder_id":        "builder-a",
		"display_name":      "Builder A",
		"capability_tags":   []string{"flutter"},
		"max_parallel_runs": 1,
		"worker_profile": map[string]any{
			"image": "builder:latest",
		},
	}, http.StatusOK)

	builderSvc, err := h.buildersControlPlane()
	if err != nil {
		t.Fatalf("buildersControlPlane() error = %v", err)
	}
	runsSvc, err := h.buildRunsControlPlane()
	if err != nil {
		t.Fatalf("buildRunsControlPlane() error = %v", err)
	}
	bundle, err := h.loadPreparedBundleByJobID("job-stale-running-execution-cancelled")
	if err != nil {
		t.Fatalf("loadPreparedBundleByJobID() error = %v", err)
	}
	preparedInput := bundle.BuilderInput
	preparedInput.ContextSourceDir = bundle.PrepareDir
	dispatch, err := builderSvc.Dispatch(context.Background(), appbuilders.Requirement{JobID: "job-stale-running-execution-cancelled"})
	if err != nil {
		t.Fatalf("Dispatch() error = %v", err)
	}
	run, err := runsSvc.Create(context.Background(), dispatch.Node.BuilderID, dispatch.Node.BuilderID, dispatch.Lease.LeaseID, preparedInput)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if err := builderSvc.BindRun(context.Background(), dispatch.Lease.LeaseID, run.RunID); err != nil {
		t.Fatalf("BindRun() error = %v", err)
	}

	requestedAt := time.Now().UTC().Add(-3 * time.Minute).Format(time.RFC3339)
	startedAt := time.Now().UTC().Add(-2 * time.Minute).Format(time.RFC3339)
	finishedAt := time.Now().UTC().Add(-90 * time.Second).Format(time.RFC3339)
	lastError := "operator cancelled orchestrator execution while stale running run was being reconciled"
	if err := h.persistPublicJobExecutionRecord(publicJobExecutionRecord{
		SchemaVersion:  "0.1.0",
		JobID:          "job-stale-running-execution-cancelled",
		RunID:          run.RunID,
		Action:         "resume",
		Status:         "cancelled",
		AttemptCount:   1,
		TimeoutSeconds: 300,
		RequestedAt:    requestedAt,
		StartedAt:      startedAt,
		FinishedAt:     finishedAt,
		LastError:      lastError,
		Attempts: []publicJobExecutionAttempt{{
			Attempt:       1,
			OwnerID:       h.orchestratorInstanceID,
			ClaimedAt:     startedAt,
			LastRenewedAt: finishedAt,
			ReleasedAt:    finishedAt,
			ReleaseReason: "cancelled",
		}},
	}); err != nil {
		t.Fatalf("persistPublicJobExecutionRecord() error = %v", err)
	}

	loaded := getJSONURL(t, server.Client(), server.URL+"/api/v1/jobs/job-stale-running-execution-cancelled", http.StatusOK)
	if loaded["status"] != "cancelled" {
		t.Fatalf("status = %v, want cancelled", loaded["status"])
	}
	if loaded["phase"] != "terminal" {
		t.Fatalf("phase = %v, want terminal", loaded["phase"])
	}
	if loaded["started_at"] != startedAt {
		t.Fatalf("started_at = %v, want %s", loaded["started_at"], startedAt)
	}
	if loaded["finished_at"] != finishedAt {
		t.Fatalf("finished_at = %v, want %s", loaded["finished_at"], finishedAt)
	}
	failureContext, ok := loaded["failure_context"].(map[string]any)
	if !ok {
		t.Fatalf("failure_context = %T, want map[string]any", loaded["failure_context"])
	}
	if failureContext["last_error_summary"] != lastError {
		t.Fatalf("last_error_summary = %v, want %s", failureContext["last_error_summary"], lastError)
	}
	if failureContext["failure_signature"] != "execution_cancelled" {
		t.Fatalf("failure_signature = %v, want execution_cancelled", failureContext["failure_signature"])
	}
	if failureContext["failure_category"] != "execution_cancelled" {
		t.Fatalf("failure_category = %v, want execution_cancelled", failureContext["failure_category"])
	}
	if failureContext["failure_domain"] != "executor" {
		t.Fatalf("failure_domain = %v, want executor", failureContext["failure_domain"])
	}
	if failureContext["retryable"] != false {
		t.Fatalf("retryable = %v, want false", failureContext["retryable"])
	}
	statusContext, ok := loaded["status_context"].(map[string]any)
	if !ok {
		t.Fatalf("status_context = %T, want map[string]any", loaded["status_context"])
	}
	if statusContext["reason_code"] != "execution_cancelled" {
		t.Fatalf("reason_code = %v, want execution_cancelled", statusContext["reason_code"])
	}
	if statusContext["suggested_action"] != "inspect_execution_history" {
		t.Fatalf("suggested_action = %v, want inspect_execution_history", statusContext["suggested_action"])
	}
	if _, exists := loaded["resume_context"]; exists {
		t.Fatalf("resume_context should be omitted when cancelled execution overrides stale running run, got %v", loaded["resume_context"])
	}
	if builderOutputPath, _ := loaded["builder_output_path"].(string); builderOutputPath != "" {
		t.Fatalf("builder_output_path = %q, want empty for stale running run", builderOutputPath)
	}

	persistedRun, err := runsSvc.Get(context.Background(), run.RunID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if persistedRun.Status != appruns.StatusRunning {
		t.Fatalf("run status = %s, want running to verify stale facade override", persistedRun.Status)
	}
}

func TestCancelPublicJobExecutionFinalizesAttemptAudit(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	cfg := config.DefaultConfig()
	workspace := filepath.Join(filepath.Dir(configPath), "workspace")
	cfg.Workspace = workspace
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	h := NewHandler(configPath)
	claimedAt := time.Now().UTC().Add(-30 * time.Second).Format(time.RFC3339)
	if err := h.persistPublicJobExecutionRecord(publicJobExecutionRecord{
		SchemaVersion:   "0.1.0",
		JobID:           "job-cancel-attempt-audit",
		RunID:           "run-cancel-attempt-audit",
		Action:          "start",
		Status:          "running",
		AttemptCount:    1,
		LeaseOwnerID:    h.orchestratorInstanceID,
		LeaseAcquiredAt: claimedAt,
		LeaseExpiresAt:  time.Now().UTC().Add(90 * time.Second).Format(time.RFC3339),
		TimeoutSeconds:  300,
		RequestedAt:     time.Now().UTC().Add(-30 * time.Second).Format(time.RFC3339),
		StartedAt:       time.Now().UTC().Add(-25 * time.Second).Format(time.RFC3339),
		Attempts: []publicJobExecutionAttempt{{
			Attempt:       1,
			OwnerID:       h.orchestratorInstanceID,
			ClaimedAt:     claimedAt,
			LastRenewedAt: time.Now().UTC().Add(-5 * time.Second).Format(time.RFC3339),
		}},
	}); err != nil {
		t.Fatalf("persistPublicJobExecutionRecord() error = %v", err)
	}

	if err := h.cancelPublicJobExecution("job-cancel-attempt-audit", "operator cancelled execution"); err != nil {
		t.Fatalf("cancelPublicJobExecution() error = %v", err)
	}

	execution := readPublicJobExecutionRecord(t, workspace, "job-cancel-attempt-audit")
	if execution.Status != "cancelled" {
		t.Fatalf("execution status = %q, want cancelled", execution.Status)
	}
	if len(execution.Attempts) != 1 {
		t.Fatalf("attempts len = %d, want 1", len(execution.Attempts))
	}
	if execution.Attempts[0].ReleaseReason != "cancelled" || execution.Attempts[0].ReleasedAt == "" {
		t.Fatalf("attempt audit = %+v, want cancelled release reason with released_at", execution.Attempts[0])
	}
}

func TestPersistPublicJobExecutionRecordRejectsSameRunTransitionBackToQueued(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	cfg := config.DefaultConfig()
	workspace := filepath.Join(filepath.Dir(configPath), "workspace")
	cfg.Workspace = workspace
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	h := NewHandler(configPath)
	requestedAt := time.Now().UTC().Add(-2 * time.Minute).Format(time.RFC3339)
	startedAt := time.Now().UTC().Add(-90 * time.Second).Format(time.RFC3339)
	if err := h.persistPublicJobExecutionRecord(publicJobExecutionRecord{
		SchemaVersion:  "0.1.0",
		JobID:          "job-invalid-same-run-transition",
		RunID:          "run-invalid-same-run-transition",
		Action:         "start",
		Status:         "running",
		TimeoutSeconds: 300,
		RequestedAt:    requestedAt,
		StartedAt:      startedAt,
	}); err != nil {
		t.Fatalf("persistPublicJobExecutionRecord(initial) error = %v", err)
	}

	err := h.persistPublicJobExecutionRecord(publicJobExecutionRecord{
		SchemaVersion:  "0.1.0",
		JobID:          "job-invalid-same-run-transition",
		RunID:          "run-invalid-same-run-transition",
		Action:         "start",
		Status:         "queued",
		TimeoutSeconds: 300,
		RequestedAt:    requestedAt,
	})
	if err == nil {
		t.Fatal("persistPublicJobExecutionRecord() error = nil, want illegal transition error")
	}
	if !strings.Contains(err.Error(), "running -> queued") {
		t.Fatalf("error = %v, want running -> queued transition message", err)
	}
}

func TestPersistPublicJobExecutionRecordRejectsReplacingNonTerminalRun(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	cfg := config.DefaultConfig()
	workspace := filepath.Join(filepath.Dir(configPath), "workspace")
	cfg.Workspace = workspace
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	h := NewHandler(configPath)
	if err := h.persistPublicJobExecutionRecord(publicJobExecutionRecord{
		SchemaVersion:  "0.1.0",
		JobID:          "job-invalid-run-replacement",
		RunID:          "run-active-old",
		Action:         "start",
		Status:         "running",
		TimeoutSeconds: 300,
		RequestedAt:    time.Now().UTC().Add(-2 * time.Minute).Format(time.RFC3339),
		StartedAt:      time.Now().UTC().Add(-90 * time.Second).Format(time.RFC3339),
	}); err != nil {
		t.Fatalf("persistPublicJobExecutionRecord(initial) error = %v", err)
	}

	err := h.persistPublicJobExecutionRecord(publicJobExecutionRecord{
		SchemaVersion:  "0.1.0",
		JobID:          "job-invalid-run-replacement",
		RunID:          "run-active-new",
		Action:         "resume",
		Status:         "queued",
		TimeoutSeconds: 300,
		RequestedAt:    time.Now().UTC().Format(time.RFC3339),
	})
	if err == nil {
		t.Fatal("persistPublicJobExecutionRecord() error = nil, want non-terminal replacement error")
	}
	if !strings.Contains(err.Error(), "cannot replace non-terminal orchestrator execution") {
		t.Fatalf("error = %v, want non-terminal replacement message", err)
	}
}

func TestPersistPublicJobExecutionRecordRejectsStaleCompletedOverwriteAfterCancel(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	cfg := config.DefaultConfig()
	workspace := filepath.Join(filepath.Dir(configPath), "workspace")
	cfg.Workspace = workspace
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	h := NewHandler(configPath)
	requestedAt := time.Now().UTC().Add(-2 * time.Minute).Format(time.RFC3339)
	startedAt := time.Now().UTC().Add(-90 * time.Second).Format(time.RFC3339)
	staleRecord := publicJobExecutionRecord{
		SchemaVersion:  "0.1.0",
		JobID:          "job-stale-completed-after-cancel",
		RunID:          "run-stale-completed-after-cancel",
		Action:         "start",
		Status:         "running",
		TimeoutSeconds: 300,
		RequestedAt:    requestedAt,
		StartedAt:      startedAt,
	}
	if err := h.persistPublicJobExecutionRecord(staleRecord); err != nil {
		t.Fatalf("persistPublicJobExecutionRecord(initial) error = %v", err)
	}
	if err := h.cancelPublicJobExecution("job-stale-completed-after-cancel", "cancel before stale complete persists"); err != nil {
		t.Fatalf("cancelPublicJobExecution() error = %v", err)
	}

	staleRecord.Status = "completed"
	staleRecord.FinishedAt = time.Now().UTC().Format(time.RFC3339)
	err := h.persistPublicJobExecutionRecord(staleRecord)
	if err == nil {
		t.Fatal("persistPublicJobExecutionRecord() error = nil, want cancelled -> completed transition rejection")
	}
	if !strings.Contains(err.Error(), "cancelled -> completed") {
		t.Fatalf("error = %v, want cancelled -> completed transition message", err)
	}

	updated := readPublicJobExecutionRecord(t, workspace, "job-stale-completed-after-cancel")
	if updated.Status != "cancelled" {
		t.Fatalf("execution status = %q, want cancelled after rejected stale overwrite", updated.Status)
	}
}

func TestPersistPublicJobExecutionRecordAllowsNewQueuedRunAfterTerminalExecution(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	cfg := config.DefaultConfig()
	workspace := filepath.Join(filepath.Dir(configPath), "workspace")
	cfg.Workspace = workspace
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	h := NewHandler(configPath)
	if err := h.persistPublicJobExecutionRecord(publicJobExecutionRecord{
		SchemaVersion:  "0.1.0",
		JobID:          "job-terminal-to-queued",
		RunID:          "run-terminal-old",
		Action:         "start",
		Status:         "failed",
		TimeoutSeconds: 300,
		RequestedAt:    time.Now().UTC().Add(-4 * time.Minute).Format(time.RFC3339),
		StartedAt:      time.Now().UTC().Add(-3 * time.Minute).Format(time.RFC3339),
		FinishedAt:     time.Now().UTC().Add(-2 * time.Minute).Format(time.RFC3339),
		LastError:      "builder failed",
	}); err != nil {
		t.Fatalf("persistPublicJobExecutionRecord(initial) error = %v", err)
	}

	if err := h.persistPublicJobExecutionRecord(publicJobExecutionRecord{
		SchemaVersion:  "0.1.0",
		JobID:          "job-terminal-to-queued",
		RunID:          "run-terminal-new",
		Action:         "resume",
		Status:         "queued",
		TimeoutSeconds: 300,
		RequestedAt:    time.Now().UTC().Format(time.RFC3339),
	}); err != nil {
		t.Fatalf("persistPublicJobExecutionRecord(requeue) error = %v", err)
	}

	execution := readPublicJobExecutionRecord(t, workspace, "job-terminal-to-queued")
	if execution.RunID != "run-terminal-new" {
		t.Fatalf("run_id = %q, want run-terminal-new", execution.RunID)
	}
	if execution.Status != "queued" {
		t.Fatalf("status = %q, want queued", execution.Status)
	}
	if execution.FinishedAt != "" {
		t.Fatalf("finished_at = %q, want empty for new queued execution", execution.FinishedAt)
	}
}

func TestRunningPublicJobExecutionRecoversAsFailedAfterHandlerRestart(t *testing.T) {
	configPath := filepath.Join(newRetriableTempDir(t), "config.json")
	cfg := config.DefaultConfig()
	workspace := filepath.Join(filepath.Dir(configPath), "workspace")
	cfg.Workspace = workspace
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	h := NewHandler(configPath)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	server := httptest.NewServer(mux)
	defer server.Close()

	postJSONURL(t, server.Client(), server.URL+"/api/v1/prds:compile", map[string]any{
		"requirement_text": "做一个简单记账 app，不考虑上架，只考虑功能实现，需要首页概览、记一笔和账单列表。",
		"job_id":           "job-orchestrator-recover-running",
		"prd_id":           "prd-orchestrator-recover-running",
	}, http.StatusOK)
	postJSONURL(t, server.Client(), server.URL+"/api/v1/jobs", map[string]any{
		"prd_id":      "prd-orchestrator-recover-running",
		"template_id": "flutter-finance-lite",
	}, http.StatusOK)
	postJSONURL(t, server.Client(), server.URL+"/internal/v1/builders:register", map[string]any{
		"builder_id":        "builder-a",
		"display_name":      "Builder A",
		"capability_tags":   []string{"flutter"},
		"max_parallel_runs": 1,
		"worker_profile": map[string]any{
			"image": "builder:latest",
		},
	}, http.StatusOK)

	builderSvc, err := h.buildersControlPlane()
	if err != nil {
		t.Fatalf("buildersControlPlane() error = %v", err)
	}
	runsSvc, err := h.buildRunsControlPlane()
	if err != nil {
		t.Fatalf("buildRunsControlPlane() error = %v", err)
	}
	bundle, err := h.loadPreparedBundleByJobID("job-orchestrator-recover-running")
	if err != nil {
		t.Fatalf("loadPreparedBundleByJobID() error = %v", err)
	}
	preparedInput := bundle.BuilderInput
	preparedInput.ContextSourceDir = bundle.PrepareDir
	dispatch, err := builderSvc.Dispatch(context.Background(), appbuilders.Requirement{JobID: "job-orchestrator-recover-running"})
	if err != nil {
		t.Fatalf("Dispatch() error = %v", err)
	}
	run, err := runsSvc.Create(context.Background(), dispatch.Node.BuilderID, dispatch.Node.BuilderID, dispatch.Lease.LeaseID, preparedInput)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if err := builderSvc.BindRun(context.Background(), dispatch.Lease.LeaseID, run.RunID); err != nil {
		t.Fatalf("BindRun() error = %v", err)
	}
	if err := h.persistPublicJobExecutionRecord(publicJobExecutionRecord{
		SchemaVersion:  "0.1.0",
		JobID:          "job-orchestrator-recover-running",
		RunID:          run.RunID,
		Action:         "start",
		Status:         "running",
		TimeoutSeconds: 300,
		RequestedAt:    time.Now().Add(-2 * time.Minute).UTC().Format(time.RFC3339),
		StartedAt:      time.Now().Add(-90 * time.Second).UTC().Format(time.RFC3339),
	}); err != nil {
		t.Fatalf("persistPublicJobExecutionRecord() error = %v", err)
	}

	restarted := NewHandler(configPath)
	restarted.EnableStartupOrchestrator()
	t.Cleanup(restarted.waitForAsyncJobs)
	restartedMux := http.NewServeMux()
	restarted.RegisterRoutes(restartedMux)
	restartedServer := httptest.NewServer(restartedMux)
	defer restartedServer.Close()

	failed := waitForJobStatus(t, restartedServer.Client(), restartedServer.URL+"/api/v1/jobs/job-orchestrator-recover-running", "failed", 5*time.Second)
	failureContext, ok := failed["failure_context"].(map[string]any)
	if !ok {
		t.Fatalf("failure_context = %T, want map[string]any", failed["failure_context"])
	}
	if !strings.Contains(strings.TrimSpace(toString(failureContext["last_error_summary"])), "dispatcher") {
		t.Fatalf("last_error_summary = %v, want dispatcher-lost summary", failureContext["last_error_summary"])
	}
	if failureContext["failure_category"] != "execution_interrupted" {
		t.Fatalf("failure_category = %v, want execution_interrupted", failureContext["failure_category"])
	}
	statusContext, ok := failed["status_context"].(map[string]any)
	if !ok {
		t.Fatalf("status_context = %T, want map[string]any", failed["status_context"])
	}
	if statusContext["reason_code"] != "execution_interrupted" {
		t.Fatalf("reason_code = %v, want execution_interrupted", statusContext["reason_code"])
	}
	if statusContext["suggested_action"] != "resume_interrupted_job" {
		t.Fatalf("suggested_action = %v, want resume_interrupted_job", statusContext["suggested_action"])
	}
	execution := waitForPublicJobExecutionStatus(t, workspace, "job-orchestrator-recover-running", "failed", 5*time.Second)
	if !strings.Contains(execution.LastError, "dispatcher") {
		t.Fatalf("execution last_error = %q, want dispatcher-lost message", execution.LastError)
	}
	recoveredRun, err := runsSvc.Get(context.Background(), run.RunID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if recoveredRun.Status != appruns.StatusFailed {
		t.Fatalf("run status = %s, want failed", recoveredRun.Status)
	}
}

func TestGetPublicJobPrefersTerminalExecutionRecordWhenRunIsStaleRunning(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	cfg := config.DefaultConfig()
	workspace := filepath.Join(filepath.Dir(configPath), "workspace")
	cfg.Workspace = workspace
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	h := NewHandler(configPath)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	server := httptest.NewServer(mux)
	defer server.Close()

	postJSONURL(t, server.Client(), server.URL+"/api/v1/prds:compile", map[string]any{
		"requirement_text": "做一个简单记账 app，不考虑上架，只考虑功能实现，需要首页概览、记一笔和账单列表。",
		"job_id":           "job-stale-running-execution",
		"prd_id":           "prd-stale-running-execution",
	}, http.StatusOK)
	postJSONURL(t, server.Client(), server.URL+"/api/v1/jobs", map[string]any{
		"prd_id":      "prd-stale-running-execution",
		"template_id": "flutter-finance-lite",
	}, http.StatusOK)
	postJSONURL(t, server.Client(), server.URL+"/internal/v1/builders:register", map[string]any{
		"builder_id":        "builder-a",
		"display_name":      "Builder A",
		"capability_tags":   []string{"flutter"},
		"max_parallel_runs": 1,
		"worker_profile": map[string]any{
			"image": "builder:latest",
		},
	}, http.StatusOK)

	builderSvc, err := h.buildersControlPlane()
	if err != nil {
		t.Fatalf("buildersControlPlane() error = %v", err)
	}
	runsSvc, err := h.buildRunsControlPlane()
	if err != nil {
		t.Fatalf("buildRunsControlPlane() error = %v", err)
	}
	bundle, err := h.loadPreparedBundleByJobID("job-stale-running-execution")
	if err != nil {
		t.Fatalf("loadPreparedBundleByJobID() error = %v", err)
	}
	preparedInput := bundle.BuilderInput
	preparedInput.ContextSourceDir = bundle.PrepareDir
	dispatch, err := builderSvc.Dispatch(context.Background(), appbuilders.Requirement{JobID: "job-stale-running-execution"})
	if err != nil {
		t.Fatalf("Dispatch() error = %v", err)
	}
	run, err := runsSvc.Create(context.Background(), dispatch.Node.BuilderID, dispatch.Node.BuilderID, dispatch.Lease.LeaseID, preparedInput)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if err := builderSvc.BindRun(context.Background(), dispatch.Lease.LeaseID, run.RunID); err != nil {
		t.Fatalf("BindRun() error = %v", err)
	}

	requestedAt := time.Now().UTC().Add(-3 * time.Minute).Format(time.RFC3339)
	startedAt := time.Now().UTC().Add(-2 * time.Minute).Format(time.RFC3339)
	finishedAt := time.Now().UTC().Add(-90 * time.Second).Format(time.RFC3339)
	lastError := "prepare docker workspace ownership: exit status 125: Unable to find image 'builder:latest' locally"
	if err := h.persistPublicJobExecutionRecord(publicJobExecutionRecord{
		SchemaVersion:  "0.1.0",
		JobID:          "job-stale-running-execution",
		RunID:          run.RunID,
		Action:         "start",
		Status:         "failed",
		AttemptCount:   1,
		TimeoutSeconds: 300,
		RequestedAt:    requestedAt,
		StartedAt:      startedAt,
		FinishedAt:     finishedAt,
		LastError:      lastError,
		Attempts: []publicJobExecutionAttempt{{
			Attempt:       1,
			OwnerID:       h.orchestratorInstanceID,
			ClaimedAt:     startedAt,
			LastRenewedAt: finishedAt,
			ReleasedAt:    finishedAt,
			ReleaseReason: "failed",
		}},
	}); err != nil {
		t.Fatalf("persistPublicJobExecutionRecord() error = %v", err)
	}

	loaded := getJSONURL(t, server.Client(), server.URL+"/api/v1/jobs/job-stale-running-execution", http.StatusOK)
	if loaded["status"] != "failed" {
		t.Fatalf("status = %v, want failed", loaded["status"])
	}
	if loaded["phase"] != "terminal" {
		t.Fatalf("phase = %v, want terminal", loaded["phase"])
	}
	if loaded["started_at"] != startedAt {
		t.Fatalf("started_at = %v, want %s", loaded["started_at"], startedAt)
	}
	if loaded["finished_at"] != finishedAt {
		t.Fatalf("finished_at = %v, want %s", loaded["finished_at"], finishedAt)
	}
	failureContext, ok := loaded["failure_context"].(map[string]any)
	if !ok {
		t.Fatalf("failure_context = %T, want map[string]any", loaded["failure_context"])
	}
	if failureContext["last_error_summary"] != lastError {
		t.Fatalf("last_error_summary = %v, want %s", failureContext["last_error_summary"], lastError)
	}
	if failureContext["failure_signature"] != "execution_failed" {
		t.Fatalf("failure_signature = %v, want execution_failed", failureContext["failure_signature"])
	}
	if failureContext["failure_category"] != "execution_failed" {
		t.Fatalf("failure_category = %v, want execution_failed", failureContext["failure_category"])
	}
	if failureContext["failure_domain"] != "executor" {
		t.Fatalf("failure_domain = %v, want executor", failureContext["failure_domain"])
	}
	if failureContext["retryable"] != true {
		t.Fatalf("retryable = %v, want true", failureContext["retryable"])
	}
	statusContext, ok := loaded["status_context"].(map[string]any)
	if !ok {
		t.Fatalf("status_context = %T, want map[string]any", loaded["status_context"])
	}
	if statusContext["reason_code"] != "execution_failed" {
		t.Fatalf("reason_code = %v, want execution_failed", statusContext["reason_code"])
	}
	if statusContext["suggested_action"] != "inspect_failure_and_retry_later" {
		t.Fatalf("suggested_action = %v, want inspect_failure_and_retry_later", statusContext["suggested_action"])
	}
	if _, exists := loaded["resume_context"]; exists {
		t.Fatalf("resume_context should be omitted when execution record overrides stale running run, got %v", loaded["resume_context"])
	}
	if builderOutputPath, _ := loaded["builder_output_path"].(string); builderOutputPath != "" {
		t.Fatalf("builder_output_path = %q, want empty for stale running run", builderOutputPath)
	}

	persistedRun, err := runsSvc.Get(context.Background(), run.RunID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if persistedRun.Status != appruns.StatusRunning {
		t.Fatalf("run status = %s, want running to verify stale facade override", persistedRun.Status)
	}
}

func TestGetTemplateReturnsRegistryEntry(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	cfg := config.DefaultConfig()
	cfg.Workspace = filepath.Join(filepath.Dir(configPath), "workspace")
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	h := NewHandler(configPath)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/templates/flutter-finance-lite", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if resp["template_id"] != "flutter-finance-lite" {
		t.Fatalf("template_id = %v, want flutter-finance-lite", resp["template_id"])
	}
	if resp["stack"] != "flutter" {
		t.Fatalf("stack = %v, want flutter", resp["stack"])
	}
	if resp["health_status"] != "healthy" {
		t.Fatalf("health_status = %v, want healthy", resp["health_status"])
	}
	capabilities, ok := resp["capabilities"].([]any)
	if !ok || len(capabilities) == 0 {
		t.Fatalf("capabilities = %v, want non-empty array", resp["capabilities"])
	}
	if resp["repo_url"] != "local://appfactory/templates/flutter-finance-lite" {
		t.Fatalf("repo_url = %v, want local template repo url", resp["repo_url"])
	}
}

func TestGetTemplateReturnsNotFoundForUnknownID(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	cfg := config.DefaultConfig()
	cfg.Workspace = filepath.Join(filepath.Dir(configPath), "workspace")
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	h := NewHandler(configPath)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/templates/unknown-template", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusNotFound, rec.Body.String())
	}
	var resp internalErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if resp.ErrorCode != "TEMPLATE_NOT_FOUND" {
		t.Fatalf("error_code = %q, want TEMPLATE_NOT_FOUND", resp.ErrorCode)
	}
}

func TestMatchTemplatesReturnsRankedCandidates(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	cfg := config.DefaultConfig()
	workspace := filepath.Join(filepath.Dir(configPath), "workspace")
	cfg.Workspace = workspace
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	h := NewHandler(configPath)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	server := httptest.NewServer(mux)
	defer server.Close()

	postJSONURL(t, server.Client(), server.URL+"/api/v1/prds:compile", map[string]any{
		"requirement_text": "做一个简单记账 app，不考虑上架，只考虑功能实现，需要首页概览、记一笔和账单列表。",
	}, http.StatusOK)

	resp := postJSONURL(t, server.Client(), server.URL+"/api/v1/templates:match", map[string]any{
		"prd_id":      "prd-bookkeeping-lite",
		"prd_version": "0.1.0",
		"max_results": 2,
	}, http.StatusOK)
	items, ok := resp["items"].([]any)
	if !ok || len(items) == 0 {
		t.Fatalf("items = %v, want non-empty", resp["items"])
	}
	first, ok := items[0].(map[string]any)
	if !ok {
		t.Fatalf("first item type = %T, want map[string]any", items[0])
	}
	if first["template_id"] != "flutter-finance-lite" {
		t.Fatalf("first template_id = %v, want flutter-finance-lite", first["template_id"])
	}
	if first["hard_gate_passed"] != true {
		t.Fatalf("hard_gate_passed = %v, want true", first["hard_gate_passed"])
	}
	reasons, ok := first["reasons"].([]any)
	if !ok || len(reasons) == 0 {
		t.Fatalf("reasons = %v, want non-empty", first["reasons"])
	}
	if len(items) > 1 {
		second, ok := items[1].(map[string]any)
		if !ok {
			t.Fatalf("second item type = %T, want map[string]any", items[1])
		}
		if first["score"].(float64) < second["score"].(float64) {
			t.Fatalf("scores not sorted desc: first=%v second=%v", first["score"], second["score"])
		}
	}
}

func TestMatchTemplatesReturnsNotFoundForUnknownPRD(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	cfg := config.DefaultConfig()
	cfg.Workspace = filepath.Join(filepath.Dir(configPath), "workspace")
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	h := NewHandler(configPath)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/templates:match", bytes.NewBufferString(`{"prd_id":"missing-prd"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusNotFound, rec.Body.String())
	}
	var resp internalErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if resp.ErrorCode != "PRD_NOT_FOUND" {
		t.Fatalf("error_code = %q, want PRD_NOT_FOUND", resp.ErrorCode)
	}
}

func TestSubmitTemplateApprovalUpdatesPrepareBundle(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	cfg := config.DefaultConfig()
	workspace := filepath.Join(filepath.Dir(configPath), "workspace")
	cfg.Workspace = workspace
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	h := NewHandler(configPath)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	server := httptest.NewServer(mux)
	defer server.Close()

	compileResp := postJSONURL(t, server.Client(), server.URL+"/api/v1/prds:compile", map[string]any{
		"requirement_text": "做一个简单记账 app，不考虑上架，只考虑功能实现，需要首页概览、记一笔和账单列表。",
	}, http.StatusOK)
	bundleDir, _ := compileResp["bundle_dir"].(string)
	resp := postJSONURL(t, server.Client(), server.URL+"/api/v1/templates/flutter-finance-lite:submit-approval", map[string]any{
		"job_id":  "job-bookkeeping-lite",
		"prd_id":  "prd-bookkeeping-lite",
		"summary": "人工确认当前模板适合记账 MVP。",
	}, http.StatusOK)
	if resp["approval_type"] != "template" {
		t.Fatalf("approval_type = %v, want template", resp["approval_type"])
	}
	if resp["status"] != "approved" {
		t.Fatalf("status = %v, want approved", resp["status"])
	}
	if subjectVersion, _ := resp["subject_version"].(string); !strings.HasPrefix(subjectVersion, "selected-template@flutter-finance-lite@v0.1.0@sha256:") {
		t.Fatalf("subject_version = %v, want content-bound template approval version", resp["subject_version"])
	}
	approvalPath := filepath.Join(bundleDir, appruns.TemplateApprovalFileName)
	data, err := os.ReadFile(approvalPath)
	if err != nil {
		t.Fatalf("ReadFile(template-approval.json) error = %v", err)
	}
	var record appruns.ApprovalRecord
	if err := json.Unmarshal(data, &record); err != nil {
		t.Fatalf("Unmarshal(template-approval.json) error = %v", err)
	}
	if record.Summary != "人工确认当前模板适合记账 MVP。" {
		t.Fatalf("summary = %q, want updated summary", record.Summary)
	}
	if record.RequestedBy.ActorID != "appfactory-api" {
		t.Fatalf("requested_by.actor_id = %q, want appfactory-api", record.RequestedBy.ActorID)
	}
}

func TestSubmitTemplateApprovalRejectsMismatchedPRD(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	cfg := config.DefaultConfig()
	cfg.Workspace = filepath.Join(filepath.Dir(configPath), "workspace")
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	h := NewHandler(configPath)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	server := httptest.NewServer(mux)
	defer server.Close()

	postJSONURL(t, server.Client(), server.URL+"/api/v1/prds:compile", map[string]any{
		"requirement_text": "做一个简单记账 app，不考虑上架，只考虑功能实现，需要首页概览、记一笔和账单列表。",
	}, http.StatusOK)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/templates/flutter-finance-lite:submit-approval", bytes.NewBufferString(`{"job_id":"job-bookkeeping-lite","prd_id":"wrong-prd"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
	var resp internalErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if resp.ErrorCode != "VALIDATION_INVALID_REQUEST" {
		t.Fatalf("error_code = %q, want VALIDATION_INVALID_REQUEST", resp.ErrorCode)
	}
}

func TestSubmitPRDApprovalUpdatesPrepareBundle(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	cfg := config.DefaultConfig()
	workspace := filepath.Join(filepath.Dir(configPath), "workspace")
	cfg.Workspace = workspace
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	h := NewHandler(configPath)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	server := httptest.NewServer(mux)
	defer server.Close()

	compileResp := postJSONURL(t, server.Client(), server.URL+"/api/v1/prds:compile", map[string]any{
		"requirement_text": "做一个简单记账 app，不考虑上架，只考虑功能实现，需要首页概览、记一笔和账单列表。",
	}, http.StatusOK)
	bundleDir, _ := compileResp["bundle_dir"].(string)
	resp := postJSONURL(t, server.Client(), server.URL+"/api/v1/prds/prd-bookkeeping-lite:submit-approval", map[string]any{
		"job_id":         "job-bookkeeping-lite",
		"summary":        "人工确认当前 PRD 已可进入模板选择阶段。",
		"evidence_paths": []string{"PRD.md", "PRD.json", "template-fit-report.md"},
	}, http.StatusOK)
	if resp["approval_type"] != "prd" {
		t.Fatalf("approval_type = %v, want prd", resp["approval_type"])
	}
	if resp["status"] != "approved" {
		t.Fatalf("status = %v, want approved", resp["status"])
	}
	if subjectVersion, _ := resp["subject_version"].(string); !strings.HasPrefix(subjectVersion, "prd-bookkeeping-lite@0.1.0@sha256:") {
		t.Fatalf("subject_version = %v, want PRD content-bound subject version", resp["subject_version"])
	}
	approvalPath := filepath.Join(bundleDir, appruns.PRDApprovalFileName)
	data, err := os.ReadFile(approvalPath)
	if err != nil {
		t.Fatalf("ReadFile(prd-approval.json) error = %v", err)
	}
	var record appruns.ApprovalRecord
	if err := json.Unmarshal(data, &record); err != nil {
		t.Fatalf("Unmarshal(prd-approval.json) error = %v", err)
	}
	if record.Summary != "人工确认当前 PRD 已可进入模板选择阶段。" {
		t.Fatalf("summary = %q, want updated summary", record.Summary)
	}
	if len(record.EvidencePaths) != 3 || record.EvidencePaths[2] != "template-fit-report.md" {
		t.Fatalf("evidence_paths = %v, want custom evidence paths", record.EvidencePaths)
	}
	if record.RequestedBy.ActorID != "appfactory-api" {
		t.Fatalf("requested_by.actor_id = %q, want appfactory-api", record.RequestedBy.ActorID)
	}
}

func TestSubmitPRDApprovalRejectsMismatchedPRD(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	cfg := config.DefaultConfig()
	cfg.Workspace = filepath.Join(filepath.Dir(configPath), "workspace")
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	h := NewHandler(configPath)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	server := httptest.NewServer(mux)
	defer server.Close()

	postJSONURL(t, server.Client(), server.URL+"/api/v1/prds:compile", map[string]any{
		"requirement_text": "做一个简单记账 app，不考虑上架，只考虑功能实现，需要首页概览、记一笔和账单列表。",
	}, http.StatusOK)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/prds/wrong-prd:submit-approval", bytes.NewBufferString(`{"job_id":"job-bookkeeping-lite"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
	var resp internalErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if resp.ErrorCode != "VALIDATION_INVALID_REQUEST" {
		t.Fatalf("error_code = %q, want VALIDATION_INVALID_REQUEST", resp.ErrorCode)
	}
}

func TestCreateGetAndDecidePRDApproval(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	cfg := config.DefaultConfig()
	workspace := filepath.Join(filepath.Dir(configPath), "workspace")
	cfg.Workspace = workspace
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	h := NewHandler(configPath)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	server := httptest.NewServer(mux)
	defer server.Close()

	postJSONURL(t, server.Client(), server.URL+"/api/v1/prds:compile", map[string]any{
		"requirement_text": "做一个简单记账 app，不考虑上架，只考虑功能实现，需要首页概览、记一笔和账单列表。",
	}, http.StatusOK)

	created := postJSONURL(t, server.Client(), server.URL+"/api/v1/approvals", map[string]any{
		"approval_type":  "prd",
		"job_id":         "job-bookkeeping-lite",
		"summary":        "请人工确认 PRD 是否可以进入下一阶段。",
		"evidence_paths": []string{"PRD.md", "PRD.json"},
	}, http.StatusCreated)
	approvalID, _ := created["approval_id"].(string)
	if approvalID == "" {
		t.Fatalf("approval_id = %q, want non-empty", approvalID)
	}
	if created["status"] != appruns.ApprovalStatusPending {
		t.Fatalf("status = %v, want pending", created["status"])
	}

	loaded := getJSONURL(t, server.Client(), server.URL+"/api/v1/approvals/"+approvalID, http.StatusOK)
	if loaded["summary"] != "请人工确认 PRD 是否可以进入下一阶段。" {
		t.Fatalf("summary = %v, want pending summary", loaded["summary"])
	}

	decided := postJSONURL(t, server.Client(), server.URL+"/api/v1/approvals/"+approvalID+":decision", map[string]any{
		"decision":         appruns.ApprovalStatusChangesRequested,
		"reviewer_id":      "user-001",
		"comment":          "请补充数据同步与离线策略。",
		"required_changes": []string{"add-sync-strategy", "add-offline-mode"},
	}, http.StatusOK)
	if decided["status"] != appruns.ApprovalStatusChangesRequested {
		t.Fatalf("status = %v, want changes_requested", decided["status"])
	}
	decision, ok := decided["decision"].(map[string]any)
	if !ok {
		t.Fatalf("decision = %T, want map[string]any", decided["decision"])
	}
	if decision["decision"] != appruns.ApprovalStatusChangesRequested {
		t.Fatalf("decision.decision = %v, want changes_requested", decision["decision"])
	}
	requiredChanges, ok := decision["required_changes"].([]any)
	if !ok || len(requiredChanges) != 2 {
		t.Fatalf("required_changes = %v, want 2 entries", decision["required_changes"])
	}

	approvalPath := filepath.Join(workspace, "appfactory", "approvals", approvalID+".json")
	data, err := os.ReadFile(approvalPath)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", approvalPath, err)
	}
	var record appruns.ApprovalRecord
	if err := json.Unmarshal(data, &record); err != nil {
		t.Fatalf("Unmarshal(approval) error = %v", err)
	}
	if record.Status != appruns.ApprovalStatusChangesRequested {
		t.Fatalf("record.status = %q, want changes_requested", record.Status)
	}
	if record.Decision == nil || len(record.Decision.RequiredChanges) != 2 {
		t.Fatalf("record.decision = %+v, want required changes", record.Decision)
	}

	snapshotPath := filepath.Join(workspace, "appfactory", "jobs", "job-bookkeeping-lite", "prepare", appruns.PRDApprovalFileName)
	snapshotData, err := os.ReadFile(snapshotPath)
	if err != nil {
		t.Fatalf("ReadFile(prd-approval.json) error = %v", err)
	}
	var snapshot appruns.ApprovalRecord
	if err := json.Unmarshal(snapshotData, &snapshot); err != nil {
		t.Fatalf("Unmarshal(prd-approval.json) error = %v", err)
	}
	if snapshot.ApprovalID != approvalID {
		t.Fatalf("snapshot.approval_id = %q, want %q", snapshot.ApprovalID, approvalID)
	}
	if snapshot.Status != appruns.ApprovalStatusChangesRequested {
		t.Fatalf("snapshot.status = %q, want changes_requested", snapshot.Status)
	}
}

func TestRejectedPRDApprovalMarksPublicJobFailed(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	cfg := config.DefaultConfig()
	cfg.Workspace = filepath.Join(filepath.Dir(configPath), "workspace")
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	h := NewHandler(configPath)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	server := httptest.NewServer(mux)
	defer server.Close()

	postJSONURL(t, server.Client(), server.URL+"/api/v1/prds:compile", map[string]any{
		"requirement_text": "做一个简单记账 app，不考虑上架，只考虑功能实现，需要首页概览、记一笔和账单列表。",
	}, http.StatusOK)

	postJSONURL(t, server.Client(), server.URL+"/api/v1/jobs", map[string]any{
		"prd_id":      "prd-bookkeeping-lite",
		"template_id": "flutter-finance-lite",
	}, http.StatusOK)

	created := postJSONURL(t, server.Client(), server.URL+"/api/v1/approvals", map[string]any{
		"approval_type": "prd",
		"job_id":        "job-bookkeeping-lite",
	}, http.StatusCreated)
	approvalID, _ := created["approval_id"].(string)

	postJSONURL(t, server.Client(), server.URL+"/api/v1/approvals/"+approvalID+":decision", map[string]any{
		"decision":    appruns.ApprovalStatusRejected,
		"reviewer_id": "user-002",
		"comment":     "当前需求描述不完整，退回补充。",
	}, http.StatusOK)

	job := getJSONURL(t, server.Client(), server.URL+"/api/v1/jobs/job-bookkeeping-lite", http.StatusOK)
	if job["status"] != "failed" {
		t.Fatalf("job.status = %v, want failed", job["status"])
	}
	if job["phase"] != "terminal" {
		t.Fatalf("job.phase = %v, want terminal", job["phase"])
	}
}

func TestSubmittedApprovalCanBeFetchedAndCannotBeDecidedTwice(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	cfg := config.DefaultConfig()
	cfg.Workspace = filepath.Join(filepath.Dir(configPath), "workspace")
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	h := NewHandler(configPath)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	server := httptest.NewServer(mux)
	defer server.Close()

	postJSONURL(t, server.Client(), server.URL+"/api/v1/prds:compile", map[string]any{
		"requirement_text": "做一个简单记账 app，不考虑上架，只考虑功能实现，需要首页概览、记一笔和账单列表。",
	}, http.StatusOK)

	approved := postJSONURL(t, server.Client(), server.URL+"/api/v1/templates/flutter-finance-lite:submit-approval", map[string]any{
		"job_id":  "job-bookkeeping-lite",
		"prd_id":  "prd-bookkeeping-lite",
		"summary": "人工确认当前模板适合记账 MVP。",
	}, http.StatusOK)
	approvalID, _ := approved["approval_id"].(string)
	loaded := getJSONURL(t, server.Client(), server.URL+"/api/v1/approvals/"+approvalID, http.StatusOK)
	if loaded["status"] != appruns.ApprovalStatusApproved {
		t.Fatalf("status = %v, want approved", loaded["status"])
	}

	reqBody, err := json.Marshal(map[string]any{
		"decision":    appruns.ApprovalStatusRejected,
		"reviewer_id": "user-003",
	})
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	req, err := http.NewRequest(http.MethodPost, server.URL+"/api/v1/approvals/"+approvalID+":decision", bytes.NewReader(reqBody))
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := server.Client().Do(req)
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusConflict)
	}
	var body internalErrorResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	if body.ErrorCode != "APPROVAL_ALREADY_DECIDED" {
		t.Fatalf("error_code = %q, want APPROVAL_ALREADY_DECIDED", body.ErrorCode)
	}
}

func sampleBuildRunInput() map[string]any {
	return map[string]any{
		"schema_version": "0.1.0",
		"job_id":         "job-1",
		"prd_id":         "prd-1",
		"template_id":    "template-1",
		"planning_policy": map[string]any{
			"policy_version": "phase1-boundary-v1",
			"stages": []map[string]any{
				{"stage": "requirement_structuring", "route": "planning_model"},
				{"stage": "domain_modeling", "route": "planning_model"},
				{"stage": "task_allocation", "route": "decision_model"},
				{"stage": "acceptance_planning", "route": "decision_model"},
				{"stage": "build_input_projection", "route": "deterministic"},
			},
		},
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

func sampleBuildRunInputFor(jobID, prdID, templateID string) map[string]any {
	input := sampleBuildRunInput()
	input["job_id"] = jobID
	input["prd_id"] = prdID
	input["template_id"] = templateID
	input["workspace_path"] = "/workspace/" + jobID
	input["artifact_dir"] = "/artifacts/" + jobID
	return input
}

func sampleArtifactManifestPayload() map[string]any {
	return map[string]any{
		"schema_version": "0.1.0",
		"job_id":         "job-1",
		"generated_at":   "2026-03-26T10:00:00Z",
		"items": []map[string]any{{
			"artifact_id":   "apk-1",
			"path":          "artifacts/apk/app-debug.apk",
			"artifact_type": "apk",
			"produced":      true,
		}},
	}
}

func sampleArtifactManifestPayloadFor(jobID string) map[string]any {
	manifest := sampleArtifactManifestPayload()
	manifest["job_id"] = jobID
	return manifest
}

func sampleMetricsPayload() map[string]any {
	return map[string]any{
		"schema_version":   "0.1.0",
		"job_id":           "job-1",
		"total_iterations": 2,
		"total_tokens":     1024,
		"duration_seconds": 35,
		"command_runs":     4,
		"device_failure_categories": []map[string]any{{
			"category":       "device_check_failed:adb_device_unavailable",
			"failure_domain": "device",
			"count":          1,
		}},
		"failure_signatures": []map[string]any{{
			"signature":  "device_check_failed:adb_device_unavailable",
			"count":      1,
			"last_stage": "device",
		}},
	}
}

func sampleBuildRunOutput() map[string]any {
	return map[string]any{
		"schema_version": "0.1.0",
		"job_id":         "job-1",
		"status":         "success",
		"final_summary":  "build completed",
		"modified_files": []map[string]any{{
			"path":        "lib/main.dart",
			"change_type": "modified",
		}},
		"checks_passed": []map[string]any{{
			"check_id": "check-1",
			"label":    "build",
			"stage":    "baseline",
			"outcome":  "passed",
		}},
		"checks_failed": []map[string]any{},
		"next_human_actions": []map[string]any{{
			"action_id": "review-1",
			"summary":   "review apk",
			"reason":    "manual validation",
		}},
		"artifacts": map[string]any{
			"manifest_path": "jobs/job-1/runs/current/artifact-manifest.json",
			"primary_outputs": []map[string]any{{
				"artifact_id":   "apk-1",
				"path":          "artifacts/apk/app-debug.apk",
				"artifact_type": "apk",
			}},
		},
		"metrics": map[string]any{
			"metrics_path":                "jobs/job-1/runs/current/metrics.json",
			"total_iterations":            2,
			"total_tokens":                1024,
			"distinct_failure_signatures": 1,
		},
		"report_paths": map[string]any{
			"change_summary_path":    "reports/change-summary.md",
			"build_report_path":      "reports/build-report.md",
			"smoke_test_report_path": "reports/smoke-report.md",
		},
	}
}

func sampleBuildRunOutputFor(jobID string) map[string]any {
	output := sampleBuildRunOutput()
	output["job_id"] = jobID
	output["artifacts"] = map[string]any{
		"manifest_path": "jobs/" + jobID + "/runs/current/artifact-manifest.json",
		"primary_outputs": []map[string]any{{
			"artifact_id":   "apk-1",
			"path":          "artifacts/apk/app-debug.apk",
			"artifact_type": "apk",
		}},
	}
	output["metrics"] = map[string]any{
		"metrics_path":                "jobs/" + jobID + "/runs/current/metrics.json",
		"total_iterations":            2,
		"total_tokens":                1024,
		"distinct_failure_signatures": 1,
	}
	return output
}

func sampleRunRecordForDockerLaunch() appruns.RunRecord {
	return appruns.RunRecord{
		RunID:         "run-1",
		JobID:         "job-1",
		ExecutorImage: "oneappfactory/builder:local",
		WorkspacePath: "/tmp/appfactory/jobs/job-1/workspace",
		ArtifactDir:   "/tmp/appfactory/jobs/job-1/artifacts",
		LaunchCommand: "/bin/sh",
		LaunchArgs:    []string{"/tmp/appfactory/jobs/job-1/runs/run-1/runner.sh"},
	}
}

func postJSON(t *testing.T, mux *http.ServeMux, path string, body map[string]any, wantStatus int) map[string]any {
	t.Helper()
	data, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != wantStatus {
		t.Fatalf("POST %s status = %d, want %d, body=%s", path, rec.Code, wantStatus, rec.Body.String())
	}
	if rec.Body.Len() == 0 {
		return map[string]any{}
	}
	var response map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("Unmarshal() error = %v, body=%s", err, rec.Body.String())
	}
	return response
}

func postJSONURL(t *testing.T, client *http.Client, url string, body map[string]any, wantStatus int) map[string]any {
	t.Helper()
	data, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != wantStatus {
		var bodyBytes bytes.Buffer
		_, _ = bodyBytes.ReadFrom(resp.Body)
		t.Fatalf("POST %s status = %d, want %d, body=%s", url, resp.StatusCode, wantStatus, bodyBytes.String())
	}
	if resp.ContentLength == 0 {
		return map[string]any{}
	}
	var response map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	return response
}

func getJSONURL(t *testing.T, client *http.Client, url string, wantStatus int) map[string]any {
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
	if resp.StatusCode != wantStatus {
		var bodyBytes bytes.Buffer
		_, _ = bodyBytes.ReadFrom(resp.Body)
		t.Fatalf("GET %s status = %d, want %d, body=%s", url, resp.StatusCode, wantStatus, bodyBytes.String())
	}
	var response map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	return response
}

func waitForJobStatus(t *testing.T, client *http.Client, url, wantStatus string, timeout time.Duration) map[string]any {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var last map[string]any
	for time.Now().Before(deadline) {
		last = getJSONURL(t, client, url, http.StatusOK)
		if last["status"] == wantStatus {
			return last
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("job status did not reach %q before timeout, last=%v", wantStatus, last)
	return nil
}

func toString(value any) string {
	text, _ := value.(string)
	return text
}

func readPublicJobExecutionRecord(t *testing.T, workspace, jobID string) publicJobExecutionRecord {
	t.Helper()
	path := publicJobExecutionPath(workspace, jobID)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", path, err)
	}
	var record publicJobExecutionRecord
	if err := json.Unmarshal(data, &record); err != nil {
		t.Fatalf("Unmarshal(%s) error = %v", path, err)
	}
	return record
}

func waitForPublicJobExecutionStatus(t *testing.T, workspace, jobID, wantStatus string, timeout time.Duration) publicJobExecutionRecord {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var last publicJobExecutionRecord
	for time.Now().Before(deadline) {
		last = readPublicJobExecutionRecord(t, workspace, jobID)
		if last.Status == wantStatus {
			return last
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("execution status did not reach %q before timeout, last=%+v", wantStatus, last)
	return publicJobExecutionRecord{}
}

func newRetriableTempDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "oneappfactory-api-test-")
	if err != nil {
		t.Fatalf("MkdirTemp() error = %v", err)
	}
	if recordPath := strings.TrimSpace(os.Getenv("ONEAPPFACTORY_TEST_TEMPDIR_RECORD_FILE")); recordPath != "" {
		if err := os.WriteFile(recordPath, []byte(dir), 0o600); err != nil {
			t.Fatalf("WriteFile(%s) error = %v", recordPath, err)
		}
	}
	if os.Getenv("ONEAPPFACTORY_KEEP_TEST_TEMPDIR") == "1" {
		t.Logf("preserving temp dir for debugging: %s", dir)
		return dir
	}
	t.Cleanup(func() {
		deadline := time.Now().Add(2 * time.Second)
		for {
			err := os.RemoveAll(dir)
			if err == nil {
				return
			}
			if time.Now().After(deadline) {
				t.Fatalf("RemoveAll(%s) error = %v", dir, err)
			}
			time.Sleep(20 * time.Millisecond)
		}
	})
	return dir
}

func requireJSONObjectFile(t *testing.T, path string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", path, err)
	}
	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("Unmarshal(%s) error = %v", path, err)
	}
	return payload
}

func mustReadFileBytes(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", path, err)
	}
	return data
}

type publicJobAutoRepairExecutor struct {
	checkID         string
	label           string
	stage           appruns.ExecutionStage
	commands        []string
	script          string
	skipEditCommand bool
}

type publicJobAutoRepairPatchGenerator struct {
	responses map[string][]appadapter.BuilderRuntimePatchResponse
	models    []string
}

type publicJobAutoRepairCoveragePatchGenerator struct {
	attempts int
	models   []string
}

type blockingPublicJobPatchGenerator struct {
	started    chan struct{}
	cancelled  chan struct{}
	startOnce  sync.Once
	cancelOnce sync.Once
}

func (generator *blockingPublicJobPatchGenerator) GeneratePatch(ctx context.Context, request appadapter.BuilderRuntimePatchRequest) (appadapter.BuilderRuntimePatchResponse, error) {
	generator.startOnce.Do(func() {
		close(generator.started)
	})
	<-ctx.Done()
	generator.cancelOnce.Do(func() {
		close(generator.cancelled)
	})
	return appadapter.BuilderRuntimePatchResponse{}, ctx.Err()
}

func (generator *publicJobAutoRepairPatchGenerator) GeneratePatch(ctx context.Context, request appadapter.BuilderRuntimePatchRequest) (appadapter.BuilderRuntimePatchResponse, error) {
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
	return appadapter.BuilderRuntimePatchResponse{}, fmt.Errorf("no stub response for aliases %v", request.ModelAliases)
}

func (generator *publicJobAutoRepairCoveragePatchGenerator) GeneratePatch(ctx context.Context, request appadapter.BuilderRuntimePatchRequest) (appadapter.BuilderRuntimePatchResponse, error) {
	if len(request.ModelAliases) == 0 {
		return appadapter.BuilderRuntimePatchResponse{}, fmt.Errorf("no model aliases in request")
	}
	alias := request.ModelAliases[0]
	generator.models = append(generator.models, alias)
	generator.attempts++
	marker := "Old Title"
	if generator.attempts >= 2 {
		marker = "Budget Flow"
	}
	paths := publicJobAutoRepairTargetPaths(request)
	operations := make([]map[string]string, 0, len(paths))
	for _, relPath := range paths {
		contentBytes, err := os.ReadFile(filepath.Join(request.Run.WorkspacePath, filepath.FromSlash(relPath)))
		if err != nil {
			contentBytes = []byte("// OneAppFactory test placeholder\n")
		}
		content := string(contentBytes)
		if relPath == "lib/main.dart" {
			content = strings.TrimRight(content, "\n") + "\n// " + marker + "\n"
		}
		operations = append(operations, map[string]string{
			"type":    "write_file",
			"path":    relPath,
			"content": content,
		})
	}
	body, err := json.Marshal(map[string]any{
		"patch_id":   fmt.Sprintf("auto-repair-%d", generator.attempts),
		"operations": operations,
	})
	if err != nil {
		return appadapter.BuilderRuntimePatchResponse{}, err
	}
	return appadapter.BuilderRuntimePatchResponse{ModelAlias: alias, Content: string(body)}, nil
}

func publicJobAutoRepairTargetPaths(request appadapter.BuilderRuntimePatchRequest) []string {
	seen := map[string]struct{}{}
	paths := make([]string, 0)
	appendPath := func(path string) {
		normalized := filepath.ToSlash(strings.TrimSpace(path))
		if normalized == "" || strings.Contains(normalized, "*") {
			return
		}
		if _, ok := seen[normalized]; ok {
			return
		}
		seen[normalized] = struct{}{}
		paths = append(paths, normalized)
	}
	if len(request.RoundInput.TaskBundle) > 0 {
		for _, path := range request.RoundInput.TaskBundle[0].TargetPaths {
			appendPath(path)
		}
	}
	if len(paths) == 0 {
		for _, task := range request.Run.TaskBundle {
			for _, path := range task.TargetPaths {
				appendPath(path)
			}
		}
	}
	if len(paths) == 0 {
		appendPath("lib/main.dart")
	}
	sort.Strings(paths)
	return paths
}

func (executor publicJobAutoRepairExecutor) Prepare(ctx context.Context, run appadapter.RunRecord) (appadapter.RoundPlan, error) {
	var editCmd *exec.Cmd
	if !executor.skipEditCommand {
		editCmd = exec.CommandContext(ctx, "/bin/sh", "-lc", "true")
		editCmd.Dir = run.WorkspacePath
		editCmd.Env = append(os.Environ(), "ONEAPPFACTORY_STUB_EXECUTOR=public-job-auto-repair-edit")
	}
	validationCmd := exec.CommandContext(ctx, "/bin/sh", "-lc", executor.script)
	validationCmd.Dir = run.WorkspacePath
	validationCmd.Env = append(os.Environ(), "ONEAPPFACTORY_STUB_EXECUTOR=public-job-auto-repair-validate")
	return appadapter.RoundPlan{
		Summary:    "public job auto repair executor",
		RoundInput: appadapter.BuildRoundInputForTest(run),
		EditStep: appadapter.ExecutionStep{
			StepID:  "thin-prepare",
			Stage:   appruns.StageThinPrepare,
			Summary: "prepare workspace",
			Command: editCmd,
		},
		ValidationSteps: []appadapter.ExecutionStep{{
			StepID:  executor.checkID,
			Stage:   executor.stage,
			Summary: executor.label,
			Command: validationCmd,
			Check: &appadapter.CheckExecutionPreview{
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

type structuredRoundTestExecutor struct{}

func (structuredRoundTestExecutor) Prepare(ctx context.Context, run appadapter.RunRecord) (appadapter.RoundPlan, error) {
	roundInput := appadapter.BuildRoundInputForTest(run)
	editScript := "mkdir -p lib\ncat > lib/main.dart <<'EOF'\nimport 'package:flutter/material.dart';\n\nvoid main() {\n  runApp(const BookkeepingApp());\n}\n\nclass BookkeepingApp extends StatelessWidget {\n  const BookkeepingApp({super.key});\n\n  @override\n  Widget build(BuildContext context) {\n    return const MaterialApp(\n      home: Scaffold(\n        body: Center(child: Text('Bookkeeping home ready')),\n      ),\n    );\n  }\n}\nEOF\n"
	editCmd := exec.CommandContext(ctx, "sh", "-c", editScript)
	editCmd.Dir = run.WorkspacePath

	checks := []struct {
		id    string
		label string
		stage appruns.ExecutionStage
	}{
		{id: "check-counter-demo-removed", label: "Counter demo removed", stage: appruns.StageBaseline},
		{id: "check-entry-form-wiring", label: "Entry form wiring", stage: appruns.StageBaseline},
		{id: "check-local-persistence-wiring", label: "Local persistence wiring", stage: appruns.StageCheap},
		{id: "check-flutter-analyze", label: "Flutter analyze", stage: appruns.StageHeavy},
		{id: "check-flutter-test", label: "Flutter test", stage: appruns.StageHeavy},
		{id: "check-flutter-build-apk", label: "Flutter build apk", stage: appruns.StageHeavy},
	}
	validationSteps := make([]appadapter.ExecutionStep, 0, len(checks))
	acceptanceChecks := make([]appadapter.CheckExecutionPreview, 0, len(checks))
	for _, check := range checks {
		cmd := exec.CommandContext(ctx, "sh", "-c", "echo "+check.id)
		cmd.Dir = run.WorkspacePath
		preview := appadapter.CheckExecutionPreview{
			CheckID:  check.id,
			Label:    check.label,
			Stage:    check.stage,
			Required: true,
			Commands: []string{"echo " + check.id},
		}
		validationSteps = append(validationSteps, appadapter.ExecutionStep{
			StepID:  check.id,
			Stage:   check.stage,
			Summary: check.label,
			Command: cmd,
			Check:   &preview,
		})
		acceptanceChecks = append(acceptanceChecks, preview)
	}
	return appadapter.RoundPlan{
		Summary:    "structured public job round",
		RoundInput: roundInput,
		EditStep: appadapter.ExecutionStep{
			StepID:  "edit-workspace",
			Stage:   appruns.StageThinPrepare,
			Summary: "write bookkeeping app entry",
			Command: editCmd,
		},
		ValidationSteps:  validationSteps,
		AcceptanceChecks: acceptanceChecks,
	}, nil
}

type structuredGenericOpenLiteRoundTestExecutor struct{}

func (structuredGenericOpenLiteRoundTestExecutor) Prepare(ctx context.Context, run appadapter.RunRecord) (appadapter.RoundPlan, error) {
	roundInput := appadapter.BuildRoundInputForTest(run)
	editCmd := exec.CommandContext(ctx, "sh", "-c", materializeOpenLiteReferenceFilesScript(run))
	editCmd.Dir = run.WorkspacePath
	checks := []struct {
		id    string
		label string
		stage appruns.ExecutionStage
	}{
		{id: "check-open-lite-counter-demo-removed", label: "Open lite counter demo removed", stage: appruns.StageCheap},
		{id: "check-open-lite-record-flow-wiring", label: "Open lite CRUD wiring", stage: appruns.StageCheap},
		{id: "check-open-lite-local-persistence-wiring", label: "Open lite local persistence wiring", stage: appruns.StageCheap},
		{id: "check-flutter-analyze", label: "Flutter analyze", stage: appruns.StageHeavy},
		{id: "check-flutter-test", label: "Flutter test", stage: appruns.StageHeavy},
		{id: "check-flutter-build-apk", label: "Flutter build apk", stage: appruns.StageHeavy},
	}
	validationSteps := make([]appadapter.ExecutionStep, 0, len(checks))
	acceptanceChecks := make([]appadapter.CheckExecutionPreview, 0, len(checks))
	for _, check := range checks {
		cmd := exec.CommandContext(ctx, "sh", "-c", "echo "+check.id)
		cmd.Dir = run.WorkspacePath
		preview := appadapter.CheckExecutionPreview{
			CheckID:  check.id,
			Label:    check.label,
			Stage:    check.stage,
			Required: true,
			Commands: []string{"echo " + check.id},
		}
		validationSteps = append(validationSteps, appadapter.ExecutionStep{
			StepID:  check.id,
			Stage:   check.stage,
			Summary: check.label,
			Command: cmd,
			Check:   &preview,
		})
		acceptanceChecks = append(acceptanceChecks, preview)
	}
	return appadapter.RoundPlan{
		Summary:    "structured generic open-lite public job round",
		RoundInput: roundInput,
		EditStep: appadapter.ExecutionStep{
			StepID:  "inspect-open-lite-workspace",
			Stage:   appruns.StageThinPrepare,
			Summary: "inspect open-lite workspace seed",
			Command: editCmd,
		},
		ValidationSteps:  validationSteps,
		AcceptanceChecks: acceptanceChecks,
	}, nil
}

func materializeOpenLiteReferenceFilesScript(run appadapter.RunRecord) string {
	referenceFiles := []string{
		"lib/models/record.dart",
		"lib/models/dashboard_summary.dart",
		"lib/repositories/record_repository.dart",
		"lib/controllers/home_controller.dart",
		"lib/controllers/record_form_controller.dart",
		"lib/controllers/record_list_controller.dart",
		"lib/template/open_lite_copy.dart",
		"lib/views/home_page.dart",
		"lib/views/record_detail_page.dart",
		"lib/views/record_form_page.dart",
		"lib/views/record_list_page.dart",
		"test/widget_test.dart",
	}
	var script strings.Builder
	script.WriteString("set -eu\n")
	script.WriteString("test -f lib/main.dart\n")
	for _, relPath := range referenceFiles {
		sourcePath := strings.TrimSpace(run.TemplateReferenceFiles[relPath])
		if sourcePath == "" {
			sourcePath = filepath.Join(run.TemplateSourceDir, filepath.FromSlash(relPath))
		}
		script.WriteString("mkdir -p ")
		script.WriteString(testShellQuote(filepath.Dir(filepath.FromSlash(relPath))))
		script.WriteByte('\n')
		script.WriteString("cp ")
		script.WriteString(testShellQuote(sourcePath))
		script.WriteByte(' ')
		script.WriteString(testShellQuote(filepath.FromSlash(relPath)))
		script.WriteByte('\n')
	}
	return script.String()
}

func testShellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

func requireJSONLinesObjectsFile(t *testing.T, path string) []map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", path, err)
	}
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" {
		return nil
	}
	lines := strings.Split(trimmed, "\n")
	items := make([]map[string]any, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var item map[string]any
		if err := json.Unmarshal([]byte(line), &item); err != nil {
			t.Fatalf("Unmarshal(JSONL %s) error = %v, line=%s", path, err, line)
		}
		items = append(items, item)
	}
	return items
}

func requireObjectContainsKeys(t *testing.T, object map[string]any, keys []string) {
	t.Helper()
	for _, key := range keys {
		if _, ok := object[key]; !ok {
			t.Fatalf("object missing key %q: %v", key, object)
		}
	}
}

func requireExactObjectKeys(t *testing.T, object map[string]any, want []string) {
	t.Helper()
	got := make([]string, 0, len(object))
	for key := range object {
		got = append(got, key)
	}
	sort.Strings(got)
	wantKeys := append([]string(nil), want...)
	sort.Strings(wantKeys)
	if strings.Join(got, ",") != strings.Join(wantKeys, ",") {
		t.Fatalf("object keys = %v, want %v; object=%v", got, wantKeys, object)
	}
}
