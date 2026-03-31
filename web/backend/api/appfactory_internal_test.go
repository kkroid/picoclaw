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

	appadapter "github.com/sipeed/picoclaw/pkg/appfactory/adapter"
	appbuilders "github.com/sipeed/picoclaw/pkg/appfactory/builders"
	appprepare "github.com/sipeed/picoclaw/pkg/appfactory/prepare"
	appruns "github.com/sipeed/picoclaw/pkg/appfactory/runs"
	"github.com/sipeed/picoclaw/pkg/config"
)

func TestInternalBuildersRegisterHeartbeatAndAllocateRelease(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	cfg := config.DefaultConfig()
	cfg.Agents.Defaults.Workspace = filepath.Join(filepath.Dir(configPath), "workspace")
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

func TestInternalPreserveWorkerDrainsBuilder(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	cfg := config.DefaultConfig()
	cfg.Agents.Defaults.Workspace = filepath.Join(filepath.Dir(configPath), "workspace")
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
	cfg.Agents.Defaults.Workspace = filepath.Join(filepath.Dir(configPath), "workspace")
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
	cfg.Agents.Defaults.Workspace = filepath.Join(filepath.Dir(configPath), "workspace")
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
	cfg.Agents.Defaults.Workspace = filepath.Join(filepath.Dir(configPath), "workspace")
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
	cfg.Agents.Defaults.Workspace = workspace
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
	cfg.Agents.Defaults.Workspace = workspace
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

func TestCompilePRDRejectsUnknownTemplate(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	cfg := config.DefaultConfig()
	cfg.Agents.Defaults.Workspace = filepath.Join(filepath.Dir(configPath), "workspace")
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
	if !strings.Contains(resp.Message, "not found in registry") {
		t.Fatalf("message = %q, want registry error", resp.Message)
	}
}

func TestGetPRDReturnsPreparedPRD(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	cfg := config.DefaultConfig()
	workspace := filepath.Join(filepath.Dir(configPath), "workspace")
	cfg.Agents.Defaults.Workspace = workspace
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
	cfg.Agents.Defaults.Workspace = filepath.Join(filepath.Dir(configPath), "workspace")
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
	cfg.Agents.Defaults.Workspace = workspace
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
	approvals, ok := got["human_approvals"].([]any)
	if !ok || len(approvals) != 2 {
		t.Fatalf("human_approvals = %v, want 2 approval paths", got["human_approvals"])
	}
	if _, err := os.Stat(filepath.Join(workspace, "appfactory", "jobs", "job-public-bookkeeping", "job.json")); err != nil {
		t.Fatalf("expected job.json to exist: %v", err)
	}
}

func TestGetPublicJobArtifactsReturnsIndexedManifest(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	cfg := config.DefaultConfig()
	workspace := filepath.Join(filepath.Dir(configPath), "workspace")
	cfg.Agents.Defaults.Workspace = workspace
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
	cfg.Agents.Defaults.Workspace = workspace
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
		"stage":        "baseline",
		"iteration":    1,
		"total_tokens": 128,
		"summary":      "baseline passed",
	}, http.StatusOK)

	events := getJSONURL(t, server.Client(), server.URL+"/api/v1/jobs/job-public-events/events", http.StatusOK)
	items, ok := events["items"].([]any)
	if !ok || len(items) < 4 {
		t.Fatalf("items = %v, want at least 4 timeline items", events["items"])
	}
	foundCreated := false
	foundHeartbeat := false
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
		}
	}
	if !foundCreated || !foundHeartbeat {
		t.Fatalf("events missing expected items: created=%v heartbeat=%v items=%v", foundCreated, foundHeartbeat, items)
	}
}

func TestStartPublicJobRunsToCompletion(t *testing.T) {
	configPath := filepath.Join(newRetriableTempDir(t), "config.json")
	cfg := config.DefaultConfig()
	workspace := filepath.Join(filepath.Dir(configPath), "workspace")
	cfg.Agents.Defaults.Workspace = workspace
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
		}
		if event["type"] == "run_completed" {
			foundRunCompleted = true
		}
	}
	if !foundExecutionEnqueued || !foundExecutionFinished || !foundRunCreated || !foundRunCompleted {
		t.Fatalf("missing events: execution_enqueued=%v execution_finished=%v run_created=%v run_completed=%v items=%v", foundExecutionEnqueued, foundExecutionFinished, foundRunCreated, foundRunCompleted, items)
	}
}

func TestStartPublicJobDefaultExecutorCreatesFlutterLandingFiles(t *testing.T) {
	configPath := filepath.Join(newRetriableTempDir(t), "config.json")
	cfg := config.DefaultConfig()
	workspace := filepath.Join(filepath.Dir(configPath), "workspace")
	cfg.Agents.Defaults.Workspace = workspace
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
	if !ok || len(modifiedFiles) < 4 {
		t.Fatalf("modified_files = %T %#v, want multiple flutter landing files", builderOutput["modified_files"], builderOutput["modified_files"])
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
	for _, wanted := range []string{"lib/main.dart", "lib/views/entry_form_page.dart", "lib/repositories/entry_repository.dart"} {
		if !modifiedPaths[wanted] {
			t.Fatalf("modified_paths missing %s: %#v", wanted, modifiedPaths)
		}
	}
	jobWorkspace := filepath.Join(workspace, "appfactory", "jobs", "job-public-default-landing", "workspace")
	mainDart := string(mustReadFileBytes(t, filepath.Join(jobWorkspace, "lib", "main.dart")))
	if !strings.Contains(mainDart, "BookkeepingApp") {
		t.Fatalf("lib/main.dart = %q, want bookkeeping shell", mainDart)
	}
	for _, forbidden := range []string{"Flutter Demo Home Page", "You have pushed the button this many times", "_counter", "_incrementCounter", "MyHomePage"} {
		if strings.Contains(mainDart, forbidden) {
			t.Fatalf("lib/main.dart should not contain demo marker %q\n%s", forbidden, mainDart)
		}
	}
	entryForm := string(mustReadFileBytes(t, filepath.Join(jobWorkspace, "lib", "views", "entry_form_page.dart")))
	if !strings.Contains(entryForm, "TextFormField") || !strings.Contains(entryForm, "showDatePicker") {
		t.Fatalf("entry_form_page.dart = %q, want real form wiring", entryForm)
	}
	repository := string(mustReadFileBytes(t, filepath.Join(jobWorkspace, "lib", "repositories", "entry_repository.dart")))
	if !strings.Contains(repository, "SharedPreferences") {
		t.Fatalf("entry_repository.dart = %q, want local persistence wiring", repository)
	}
	pubspec := string(mustReadFileBytes(t, filepath.Join(jobWorkspace, "pubspec.yaml")))
	if !strings.Contains(pubspec, "shared_preferences: ^2.2.3") {
		t.Fatalf("pubspec.yaml = %q, want shared_preferences dependency", pubspec)
	}
	if strings.Contains(pubspec, "A new Flutter project.") {
		t.Fatalf("pubspec.yaml should not keep default template description\n%s", pubspec)
	}
	widgetTest := string(mustReadFileBytes(t, filepath.Join(jobWorkspace, "test", "widget_test.dart")))
	if !strings.Contains(widgetTest, "bookkeeping shell renders") {
		t.Fatalf("widget_test.dart = %q, want bookkeeping smoke test", widgetTest)
	}
	for _, forbidden := range []string{"Counter increments smoke test", "MyHomePage", "_incrementCounter"} {
		if strings.Contains(widgetTest, forbidden) {
			t.Fatalf("widget_test.dart should not contain demo marker %q\n%s", forbidden, widgetTest)
		}
	}
	if _, err := os.Stat(filepath.Join(jobWorkspace, "lib", "picoclaw_executor_probe.dart")); !os.IsNotExist(err) {
		t.Fatalf("probe file should not be created on flutter landing path, stat err=%v", err)
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
	for _, checkID := range []string{"check-counter-demo-removed", "check-entry-form-wiring", "check-local-persistence-wiring"} {
		if !seenChecks[checkID] {
			t.Fatalf("validation_results missing %s: %v", checkID, seenChecks)
		}
	}
}

func TestStartPublicJobWritesStructuredRoundOutputForFlutterProfile(t *testing.T) {
	configPath := filepath.Join(newRetriableTempDir(t), "config.json")
	cfg := config.DefaultConfig()
	workspace := filepath.Join(filepath.Dir(configPath), "workspace")
	cfg.Agents.Defaults.Workspace = workspace
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

func TestStartPublicJobRejectsWhenApprovalsNotReady(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	cfg := config.DefaultConfig()
	cfg.Agents.Defaults.Workspace = filepath.Join(filepath.Dir(configPath), "workspace")
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
	cfg.Agents.Defaults.Workspace = workspace
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
	cfg.Agents.Defaults.Workspace = workspace
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
	cfg.Agents.Defaults.Workspace = workspace
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

func TestResumeFailedPublicJobRunsToCompletion(t *testing.T) {
	configPath := filepath.Join(newRetriableTempDir(t), "config.json")
	cfg := config.DefaultConfig()
	workspace := filepath.Join(filepath.Dir(configPath), "workspace")
	cfg.Agents.Defaults.Workspace = workspace
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
	cfg.Agents.Defaults.Workspace = workspace
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
	if resumeContext["next_action"] != "inspect_failure_then_retry" {
		t.Fatalf("next_action = %v, want inspect_failure_then_retry", resumeContext["next_action"])
	}
	if resumeContext["failure_category"] != "builder_runtime_failure" {
		t.Fatalf("failure_category = %v, want builder_runtime_failure", resumeContext["failure_category"])
	}
	if resumeContext["failure_domain"] != "environment" {
		t.Fatalf("failure_domain = %v, want environment", resumeContext["failure_domain"])
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
	cfg.Agents.Defaults.Workspace = workspace
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
	if resumeContext["next_action"] != "retry_failed_run" {
		t.Fatalf("next_action = %v, want retry_failed_run", resumeContext["next_action"])
	}
	if resumeContext["failure_category"] != "builder_runtime_failure" {
		t.Fatalf("failure_category = %v, want builder_runtime_failure", resumeContext["failure_category"])
	}
	if resumeContext["failure_domain"] != "environment" {
		t.Fatalf("failure_domain = %v, want environment", resumeContext["failure_domain"])
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
	cfg.Agents.Defaults.Workspace = workspace
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
	if resumeContext["next_action"] != "retry_failed_run" {
		t.Fatalf("next_action = %v, want retry_failed_run", resumeContext["next_action"])
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
	cfg.Agents.Defaults.Workspace = workspace
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
	if resumeContext["next_action"] != "inspect_failure_and_confirm_resume" {
		t.Fatalf("next_action = %v, want inspect_failure_and_confirm_resume", resumeContext["next_action"])
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
	cfg.Agents.Defaults.Workspace = workspace
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
	if resumeContext["next_action"] != "resume_interrupted_job" {
		t.Fatalf("next_action = %v, want resume_interrupted_job", resumeContext["next_action"])
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
	if resumeContext["recommended_resume_mode"] != "resume_interrupted_job" {
		t.Fatalf("recommended_resume_mode = %v, want resume_interrupted_job", resumeContext["recommended_resume_mode"])
	}
}

func TestGetFailedPublicJobSynthesizesProfileFailureDomainForStructuralCheck(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	cfg := config.DefaultConfig()
	workspace := filepath.Join(filepath.Dir(configPath), "workspace")
	cfg.Agents.Defaults.Workspace = workspace
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
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if got := derivePublicJobFailureDomain(tc.category); got != tc.want {
				t.Fatalf("derivePublicJobFailureDomain(%q) = %q, want %q", tc.category, got, tc.want)
			}
		})
	}
}

func TestListPublicJobsSupportsStatusFilter(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	cfg := config.DefaultConfig()
	workspace := filepath.Join(filepath.Dir(configPath), "workspace")
	cfg.Agents.Defaults.Workspace = workspace
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
		"job_id":           "job-list-running",
		"prd_id":           "prd-list-running",
	}, http.StatusOK)
	postJSONURL(t, server.Client(), server.URL+"/api/v1/jobs", map[string]any{
		"prd_id":      "prd-list-running",
		"template_id": "flutter-finance-lite",
	}, http.StatusOK)

	postJSONURL(t, server.Client(), server.URL+"/api/v1/prds:compile", map[string]any{
		"requirement_text": "做一个简单记账 app，不考虑上架，只考虑功能实现，需要首页概览、记一笔和账单列表。",
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
}

func TestStartPublicJobReturnsConflictWhenExecutionAlreadyActive(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	cfg := config.DefaultConfig()
	workspace := filepath.Join(filepath.Dir(configPath), "workspace")
	cfg.Agents.Defaults.Workspace = workspace
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
	cfg.Agents.Defaults.Workspace = workspace
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

func TestResumeFailedPublicJobRestoresPreservedWorkspace(t *testing.T) {
	configPath := filepath.Join(newRetriableTempDir(t), "config.json")
	cfg := config.DefaultConfig()
	workspace := filepath.Join(filepath.Dir(configPath), "workspace")
	cfg.Agents.Defaults.Workspace = workspace
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

func TestResumeFailedPublicJobRequiresHumanConfirmation(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	cfg := config.DefaultConfig()
	workspace := filepath.Join(filepath.Dir(configPath), "workspace")
	cfg.Agents.Defaults.Workspace = workspace
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
	cfg.Agents.Defaults.Workspace = workspace
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
	cfg.Agents.Defaults.Workspace = filepath.Join(filepath.Dir(configPath), "workspace")
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
	cfg.Agents.Defaults.Workspace = workspace
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
	if !ok || len(items) < 6 {
		t.Fatalf("items = %v, want at least 6 notifications", notifications["items"])
	}
	foundApprovalRequested := false
	foundBuilderFailed := false
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
	if !foundApprovalRequested || !foundBuilderFailed || !foundExecutionInterrupted || !foundExecutionHandoff || !foundExecutionRecoveryFailed || !foundWorkspacePreserved {
		t.Fatalf("missing notification types: approval=%v builder_failed=%v execution_interrupted=%v execution_handoff=%v execution_recovery_failed=%v workspace_preserved=%v items=%v", foundApprovalRequested, foundBuilderFailed, foundExecutionInterrupted, foundExecutionHandoff, foundExecutionRecoveryFailed, foundWorkspacePreserved, items)
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
	requireExactObjectKeys(t, persisted, []string{"items"})
	persistedItems, ok := persisted["items"].([]any)
	if !ok || len(persistedItems) < 6 {
		t.Fatalf("persisted items = %v, want at least 6 notifications", persisted["items"])
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
	cfg.Agents.Defaults.Workspace = workspace
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
	cfg.Agents.Defaults.Workspace = workspace
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
	cfg.Agents.Defaults.Workspace = workspace
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

func TestGetOrchestratorStatusIncludesWatchLockState(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	cfg := config.DefaultConfig()
	workspace := filepath.Join(filepath.Dir(configPath), "workspace")
	cfg.Agents.Defaults.Workspace = workspace
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
	cfg.Agents.Defaults.Workspace = workspace
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
	cfg.Agents.Defaults.Workspace = workspace
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
}

func TestUnlockOrchestratorWatchEndpointForceClearsForeignActiveLock(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	cfg := config.DefaultConfig()
	workspace := filepath.Join(filepath.Dir(configPath), "workspace")
	cfg.Agents.Defaults.Workspace = workspace
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
	cfg.Agents.Defaults.Workspace = workspace
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
	cfg.Agents.Defaults.Workspace = workspace
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
}

func TestRunOrchestratorPassEndpointReturnsWhileWatchRuns(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	cfg := config.DefaultConfig()
	workspace := filepath.Join(filepath.Dir(configPath), "workspace")
	cfg.Agents.Defaults.Workspace = workspace
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
	cfg.Agents.Defaults.Workspace = workspace
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
	cfg.Agents.Defaults.Workspace = workspace
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
	cfg.Agents.Defaults.Workspace = workspace
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
	cfg.Agents.Defaults.Workspace = workspace
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
	cfg.Agents.Defaults.Workspace = workspace
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
	cfg.Agents.Defaults.Workspace = workspace
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
	cfg.Agents.Defaults.Workspace = workspace
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
		"run_id":                runID,
		"reviewer_id":           "qa-owner",
		"status":                "staged",
		"release_channel":       "canary",
		"rollout_percent":       20,
		"summary":               "canary rollout approved",
		"evidence_paths":        []string{"reports/review-bundle.md", "reports/handoff-checklist.md"},
		"signed_artifact_paths": []string{"artifacts/app-signed.apk"},
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
		"recorded_at",
	})
	if delivery["status"] != "staged" {
		t.Fatalf("delivery status = %v, want staged", delivery["status"])
	}
	if delivery["release_channel"] != "canary" {
		t.Fatalf("delivery release_channel = %v, want canary", delivery["release_channel"])
	}

	manifest := getJSONURL(t, server.Client(), server.URL+"/api/v1/jobs/job-delivery-record/artifacts", http.StatusOK)
	items, ok := manifest["items"].([]any)
	if !ok || len(items) < 8 {
		t.Fatalf("items = %v, want at least 8 artifacts", manifest["items"])
	}
	foundDeliveryRecord := false
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
	}
	if !foundDeliveryRecord {
		t.Fatalf("delivery-record artifact missing from manifest: %v", items)
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
}

func TestQueuedPublicJobExecutionRecoversAfterHandlerRestart(t *testing.T) {
	configPath := filepath.Join(newRetriableTempDir(t), "config.json")
	cfg := config.DefaultConfig()
	workspace := filepath.Join(filepath.Dir(configPath), "workspace")
	cfg.Agents.Defaults.Workspace = workspace
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
	cfg.Agents.Defaults.Workspace = workspace
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
	cfg.Agents.Defaults.Workspace = workspace
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
	cfg.Agents.Defaults.Workspace = workspace
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
	cfg.Agents.Defaults.Workspace = workspace
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
	cfg.Agents.Defaults.Workspace = workspace
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

func TestPublicJobExecutionLeaseHeartbeatRenewsExpiry(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	cfg := config.DefaultConfig()
	workspace := filepath.Join(filepath.Dir(configPath), "workspace")
	cfg.Agents.Defaults.Workspace = workspace
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
	for _, eventType := range []string{"execution_cancelled", "execution_dispatcher_lost", "execution_recovery_failed", "execution_failed", "execution_completed"} {
		if !foundTypes[eventType] {
			t.Fatalf("missing event type %q in events=%+v", eventType, events)
		}
	}
}

func TestCancelPublicJobExecutionFinalizesAttemptAudit(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	cfg := config.DefaultConfig()
	workspace := filepath.Join(filepath.Dir(configPath), "workspace")
	cfg.Agents.Defaults.Workspace = workspace
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
	cfg.Agents.Defaults.Workspace = workspace
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
	cfg.Agents.Defaults.Workspace = workspace
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

func TestPersistPublicJobExecutionRecordAllowsNewQueuedRunAfterTerminalExecution(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	cfg := config.DefaultConfig()
	workspace := filepath.Join(filepath.Dir(configPath), "workspace")
	cfg.Agents.Defaults.Workspace = workspace
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
	cfg.Agents.Defaults.Workspace = workspace
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
	cfg.Agents.Defaults.Workspace = workspace
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
	if failureContext["retryable"] != true {
		t.Fatalf("retryable = %v, want true", failureContext["retryable"])
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
	cfg.Agents.Defaults.Workspace = filepath.Join(filepath.Dir(configPath), "workspace")
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
	cfg.Agents.Defaults.Workspace = filepath.Join(filepath.Dir(configPath), "workspace")
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
	cfg.Agents.Defaults.Workspace = workspace
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
	cfg.Agents.Defaults.Workspace = filepath.Join(filepath.Dir(configPath), "workspace")
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
	cfg.Agents.Defaults.Workspace = workspace
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
	if resp["subject_version"] != "selected-template@flutter-finance-lite@v0.1.0" {
		t.Fatalf("subject_version = %v, want selected-template@flutter-finance-lite@v0.1.0", resp["subject_version"])
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
	cfg.Agents.Defaults.Workspace = filepath.Join(filepath.Dir(configPath), "workspace")
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
	cfg.Agents.Defaults.Workspace = workspace
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
	if resp["subject_version"] != "0.1.0" {
		t.Fatalf("subject_version = %v, want 0.1.0", resp["subject_version"])
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
	cfg.Agents.Defaults.Workspace = filepath.Join(filepath.Dir(configPath), "workspace")
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
	cfg.Agents.Defaults.Workspace = workspace
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
	cfg.Agents.Defaults.Workspace = filepath.Join(filepath.Dir(configPath), "workspace")
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
	cfg.Agents.Defaults.Workspace = filepath.Join(filepath.Dir(configPath), "workspace")
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
		"failure_signatures": []map[string]any{{
			"signature":  "lint_warning",
			"count":      1,
			"last_stage": "baseline",
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
		ExecutorImage: "picoclaw/appfactory-builder:local",
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
	dir, err := os.MkdirTemp("", "picoclaw-api-test-")
	if err != nil {
		t.Fatalf("MkdirTemp() error = %v", err)
	}
	if recordPath := strings.TrimSpace(os.Getenv("PICOCLAW_TEST_TEMPDIR_RECORD_FILE")); recordPath != "" {
		if err := os.WriteFile(recordPath, []byte(dir), 0o600); err != nil {
			t.Fatalf("WriteFile(%s) error = %v", recordPath, err)
		}
	}
	if os.Getenv("PICOCLAW_KEEP_TEST_TEMPDIR") == "1" {
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
