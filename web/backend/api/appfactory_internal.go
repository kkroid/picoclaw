package api

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	appadapter "github.com/sipeed/picoclaw/pkg/appfactory/adapter"
	appbuilders "github.com/sipeed/picoclaw/pkg/appfactory/builders"
	appprepare "github.com/sipeed/picoclaw/pkg/appfactory/prepare"
	appruns "github.com/sipeed/picoclaw/pkg/appfactory/runs"
	"github.com/sipeed/picoclaw/pkg/config"
	"github.com/sipeed/picoclaw/pkg/envfile"
	"github.com/sipeed/picoclaw/pkg/fileutil"
)

type builderService interface {
	Register(ctx context.Context, req appbuilders.RegisterRequest) (appbuilders.BuilderNode, error)
	Heartbeat(ctx context.Context, req appbuilders.HeartbeatRequest) (appbuilders.BuilderNode, error)
	Drain(ctx context.Context, builderID, reason string) error
	Dispatch(ctx context.Context, req appbuilders.Requirement) (appbuilders.DispatchResult, error)
	BindRun(ctx context.Context, leaseID, runID string) error
	ReleaseByRun(ctx context.Context, runID, reason string) error
	ReleaseByBuilder(ctx context.Context, builderID, reason string) error
}

type runService interface {
	Create(ctx context.Context, builderID, workerID, leaseID string, input appruns.BuildInput) (appruns.RunRecord, error)
	Get(ctx context.Context, runID string) (appruns.RunRecord, error)
	Heartbeat(ctx context.Context, runID string, heartbeat appruns.Heartbeat) (appruns.RunRecord, error)
	Complete(ctx context.Context, runID string, output appruns.BuildOutput) (appruns.RunRecord, error)
	Fail(ctx context.Context, runID string, report appruns.FailureReport) (appruns.RunRecord, error)
	Cancel(ctx context.Context, runID, reason string) (appruns.RunRecord, error)
	IndexArtifacts(ctx context.Context, runID string, manifest appruns.ArtifactManifest) (appruns.RunRecord, error)
	IndexMetrics(ctx context.Context, runID string, metrics appruns.Metrics) (appruns.RunRecord, error)
}

type registerBuilderRequest struct {
	BuilderID       string                    `json:"builder_id"`
	DisplayName     string                    `json:"display_name"`
	Endpoint        string                    `json:"endpoint,omitempty"`
	CapabilityTags  []string                  `json:"capability_tags,omitempty"`
	ModelTags       []string                  `json:"model_tags,omitempty"`
	Priority        int                       `json:"priority,omitempty"`
	MaxParallelRuns int                       `json:"max_parallel_runs,omitempty"`
	WorkerProfile   appbuilders.WorkerProfile `json:"worker_profile"`
}

type heartbeatBuilderRequest struct {
	Status            appbuilders.Status `json:"status,omitempty"`
	CurrentRunID      string             `json:"current_run_id,omitempty"`
	LastFailureReason string             `json:"last_failure_reason,omitempty"`
	Load              float64            `json:"load,omitempty"`
	FreeDiskBytes     int64              `json:"free_disk_bytes,omitempty"`
}

type allocateWorkerRequest struct {
	JobID                   string   `json:"job_id"`
	RequiredCapabilityTags  []string `json:"required_capability_tags,omitempty"`
	PreferredCapabilityTags []string `json:"preferred_capability_tags,omitempty"`
	ModelTier               string   `json:"model_tier,omitempty"`
	BudgetClass             string   `json:"budget_class,omitempty"`
	WorkerProfileName       string   `json:"worker_profile_name,omitempty"`
}

type createBuildRunRequest struct {
	WorkerID     string             `json:"worker_id"`
	LeaseID      string             `json:"lease_id"`
	BuilderInput appruns.BuildInput `json:"builder_input"`
}

type runRequirementRequest struct {
	RequirementText   string   `json:"requirement_text"`
	RequirementSource string   `json:"requirement_source,omitempty"`
	Title             string   `json:"title,omitempty"`
	JobID             string   `json:"job_id,omitempty"`
	PRDID             string   `json:"prd_id,omitempty"`
	TemplateID        string   `json:"template_id,omitempty"`
	RealChecks        bool     `json:"real_checks,omitempty"`
	ExecutorImage     string   `json:"executor_image,omitempty"`
	BuilderID         string   `json:"builder_id"`
	DisplayName       string   `json:"display_name,omitempty"`
	BuilderImage      string   `json:"builder_image,omitempty"`
	CapabilityTags    []string `json:"capability_tags,omitempty"`
	ModelTags         []string `json:"model_tags,omitempty"`
	TimeoutSeconds    int      `json:"timeout_seconds,omitempty"`
}

type compilePRDRequest struct {
	RequirementText   string `json:"requirement_text"`
	RequirementSource string `json:"requirement_source,omitempty"`
	Title             string `json:"title,omitempty"`
	JobID             string `json:"job_id,omitempty"`
	PRDID             string `json:"prd_id,omitempty"`
	TemplateID        string `json:"template_id,omitempty"`
	RealChecks        bool   `json:"real_checks,omitempty"`
	ExecutorImage     string `json:"executor_image,omitempty"`
}

type matchTemplatesRequest struct {
	PRDID      string `json:"prd_id"`
	PRDVersion string `json:"prd_version,omitempty"`
	MaxResults int    `json:"max_results,omitempty"`
}

type submitTemplateApprovalRequest struct {
	JobID   string `json:"job_id"`
	PRDID   string `json:"prd_id"`
	Version string `json:"version,omitempty"`
	Summary string `json:"summary,omitempty"`
}

type submitPRDApprovalRequest struct {
	JobID         string   `json:"job_id"`
	Version       string   `json:"version,omitempty"`
	Summary       string   `json:"summary,omitempty"`
	EvidencePaths []string `json:"evidence_paths,omitempty"`
}

type createApprovalRequest struct {
	ApprovalType   string   `json:"approval_type"`
	JobID          string   `json:"job_id,omitempty"`
	PRDID          string   `json:"prd_id,omitempty"`
	TemplateID     string   `json:"template_id,omitempty"`
	SubjectVersion string   `json:"subject_version,omitempty"`
	Summary        string   `json:"summary,omitempty"`
	EvidencePaths  []string `json:"evidence_paths,omitempty"`
}

type approvalDecisionRequest struct {
	Decision        string   `json:"decision"`
	ReviewerID      string   `json:"reviewer_id"`
	Comment         string   `json:"comment,omitempty"`
	RequiredChanges []string `json:"required_changes,omitempty"`
}

type createJobRequest struct {
	PRDID       string          `json:"prd_id"`
	PRDVersion  string          `json:"prd_version,omitempty"`
	TemplateID  string          `json:"template_id"`
	GoalSummary string          `json:"goal_summary,omitempty"`
	HumanNotes  json.RawMessage `json:"human_notes,omitempty"`
}

type startJobRequest struct {
	Comment        string `json:"comment,omitempty"`
	TimeoutSeconds int    `json:"timeout_seconds,omitempty"`
}

type cancelJobRequest struct {
	Reason            string `json:"reason,omitempty"`
	PreserveWorkspace bool   `json:"preserve_workspace,omitempty"`
}

type resumeJobRequest struct {
	ResumeMode     string `json:"resume_mode,omitempty"`
	Confirm        bool   `json:"confirm,omitempty"`
	Note           string `json:"note,omitempty"`
	TimeoutSeconds int    `json:"timeout_seconds,omitempty"`
}

type unlockOrchestratorWatchRequest struct {
	Force bool `json:"force,omitempty"`
}

type startOrchestratorWatchRequest struct {
	IntervalSeconds int `json:"interval_seconds,omitempty"`
}

type publicJobRecord struct {
	SchemaVersion     string                    `json:"schema_version"`
	JobID             string                    `json:"job_id"`
	PRDID             string                    `json:"prd_id"`
	PRDVersion        string                    `json:"prd_version"`
	TemplateID        string                    `json:"template_id"`
	Status            string                    `json:"status"`
	Phase             string                    `json:"phase"`
	WorkspacePath     string                    `json:"workspace_path"`
	ArtifactDir       string                    `json:"artifact_dir"`
	BuilderInputPath  string                    `json:"builder_input_path,omitempty"`
	BuilderOutputPath string                    `json:"builder_output_path,omitempty"`
	Budgets           publicJobBudgets          `json:"budgets"`
	Runtime           *publicJobRuntime         `json:"runtime,omitempty"`
	Logs              *publicJobLogs            `json:"logs,omitempty"`
	Artifacts         []string                  `json:"artifacts,omitempty"`
	HumanApprovals    []string                  `json:"human_approvals"`
	FailureContext    *publicJobFailureContext  `json:"failure_context,omitempty"`
	ResumeContext     *publicJobResumeContext   `json:"resume_context,omitempty"`
	DeliveryContext   *publicJobDeliveryContext `json:"delivery_context,omitempty"`
	CreatedAt         string                    `json:"created_at"`
	UpdatedAt         string                    `json:"updated_at"`
	StartedAt         string                    `json:"started_at,omitempty"`
	FinishedAt        string                    `json:"finished_at,omitempty"`
}

type publicJobBudgets struct {
	IterationBudget   int `json:"iteration_budget"`
	TokenBudget       int `json:"token_budget"`
	ElapsedIterations int `json:"elapsed_iterations,omitempty"`
	ConsumedTokens    int `json:"consumed_tokens,omitempty"`
}

type publicJobRuntime struct {
	Builder       string `json:"builder,omitempty"`
	WorkerPool    string `json:"worker_pool,omitempty"`
	NetworkPolicy string `json:"network_policy,omitempty"`
}

type publicJobLogs struct {
	SummaryPath  string `json:"summary_path,omitempty"`
	EventLogPath string `json:"event_log_path,omitempty"`
}

type publicJobFailureContext struct {
	FailureSignature string `json:"failure_signature,omitempty"`
	LastErrorSummary string `json:"last_error_summary,omitempty"`
	Retryable        bool   `json:"retryable"`
}

type publicJobResumeContext struct {
	ResumeAllowed              bool   `json:"resume_allowed"`
	FailureDomain              string `json:"failure_domain,omitempty"`
	FailureCategory            string `json:"failure_category,omitempty"`
	RecommendedResumeMode      string `json:"recommended_resume_mode,omitempty"`
	RequiresHumanConfirmation  bool   `json:"requires_human_confirmation"`
	RequiresPreservedWorkspace bool   `json:"requires_preserved_workspace"`
	PreservedWorkspacePath     string `json:"preserved_workspace_path,omitempty"`
	NextAction                 string `json:"next_action,omitempty"`
}

type publicJobDeliveryContext struct {
	DeliveryRecordPath  string   `json:"delivery_record_path"`
	Status              string   `json:"status"`
	Summary             string   `json:"summary,omitempty"`
	NextAction          string   `json:"next_action,omitempty"`
	ReleaseChannel      string   `json:"release_channel,omitempty"`
	RolloutPercent      int      `json:"rollout_percent,omitempty"`
	ReviewerID          string   `json:"reviewer_id,omitempty"`
	EvidencePaths       []string `json:"evidence_paths,omitempty"`
	RequiredChanges     []string `json:"required_changes,omitempty"`
	SignedArtifactPaths []string `json:"signed_artifact_paths,omitempty"`
	RecordedAt          string   `json:"recorded_at"`
}

type publicJobEvent struct {
	At                         string `json:"at"`
	Type                       string `json:"type"`
	JobID                      string `json:"job_id"`
	RunID                      string `json:"run_id,omitempty"`
	Summary                    string `json:"summary,omitempty"`
	SnapshotPath               string `json:"snapshot_path,omitempty"`
	Stage                      string `json:"stage,omitempty"`
	Status                     string `json:"status,omitempty"`
	FailureDomain              string `json:"failure_domain,omitempty"`
	FailureCategory            string `json:"failure_category,omitempty"`
	RecommendedResumeMode      string `json:"recommended_resume_mode,omitempty"`
	RequiresHumanConfirmation  bool   `json:"requires_human_confirmation,omitempty"`
	RequiresPreservedWorkspace bool   `json:"requires_preserved_workspace,omitempty"`
	DeliveryStatus             string `json:"delivery_status,omitempty"`
	DeliveryRecordPath         string `json:"delivery_record_path,omitempty"`
	ReleaseChannel             string `json:"release_channel,omitempty"`
	RolloutPercent             int    `json:"rollout_percent,omitempty"`
}

type publicNotification struct {
	NotificationID             string   `json:"notification_id"`
	Type                       string   `json:"type"`
	JobID                      string   `json:"job_id,omitempty"`
	ApprovalID                 string   `json:"approval_id,omitempty"`
	Status                     string   `json:"status,omitempty"`
	Summary                    string   `json:"summary,omitempty"`
	SuggestedAction            string   `json:"suggested_action,omitempty"`
	Links                      []string `json:"links,omitempty"`
	CreatedAt                  string   `json:"created_at"`
	FailureDomain              string   `json:"failure_domain,omitempty"`
	FailureCategory            string   `json:"failure_category,omitempty"`
	RecommendedResumeMode      string   `json:"recommended_resume_mode,omitempty"`
	RequiresHumanConfirmation  bool     `json:"requires_human_confirmation,omitempty"`
	RequiresPreservedWorkspace bool     `json:"requires_preserved_workspace,omitempty"`
	DeliveryStatus             string   `json:"delivery_status,omitempty"`
	DeliveryRecordPath         string   `json:"delivery_record_path,omitempty"`
	ReleaseChannel             string   `json:"release_channel,omitempty"`
	RolloutPercent             int      `json:"rollout_percent,omitempty"`
	Acknowledged               bool     `json:"acknowledged,omitempty"`
	AcknowledgedAt             string   `json:"acknowledged_at,omitempty"`
}

type preparedBundleRecord struct {
	JobID        string
	PrepareDir   string
	PRD          appprepare.PRD
	BuilderInput appruns.BuildInput
}

type heartbeatRunRequest struct {
	Stage             appruns.ExecutionStage `json:"stage"`
	Iteration         int                    `json:"iteration"`
	FailureSignatures []string               `json:"failure_signatures,omitempty"`
	TotalTokens       int                    `json:"total_tokens,omitempty"`
	Summary           string                 `json:"summary,omitempty"`
}

type completeBuildRunRequest struct {
	BuilderOutput appruns.BuildOutput `json:"builder_output"`
}

type prepareReviewRequest struct {
	RunID string `json:"run_id"`
}

type recordDeliveryRequest struct {
	RunID               string   `json:"run_id"`
	ReviewerID          string   `json:"reviewer_id"`
	Status              string   `json:"status"`
	Summary             string   `json:"summary,omitempty"`
	NextAction          string   `json:"next_action,omitempty"`
	ReleaseChannel      string   `json:"release_channel,omitempty"`
	RolloutPercent      int      `json:"rollout_percent,omitempty"`
	EvidencePaths       []string `json:"evidence_paths,omitempty"`
	RequiredChanges     []string `json:"required_changes,omitempty"`
	SignedArtifactPaths []string `json:"signed_artifact_paths,omitempty"`
}

type failBuildRunRequest struct {
	Summary            string                 `json:"summary"`
	RecoverySuggestion string                 `json:"recovery_suggestion,omitempty"`
	FailureSignatures  []string               `json:"failure_signatures,omitempty"`
	RoundState         *appruns.RoundState    `json:"round_state,omitempty"`
	RepairContext      *appruns.RepairContext `json:"repair_context,omitempty"`
}

type cancelBuildRunRequest struct {
	Reason string `json:"reason"`
}

type preserveWorkerRequest struct {
	Reason string `json:"reason"`
}

type releaseWorkerRequest struct {
	Reason            string `json:"reason"`
	ArchiveArtifacts  bool   `json:"archive_artifacts,omitempty"`
	DestroyWorkspace  bool   `json:"destroy_workspace,omitempty"`
	PreserveWorkspace bool   `json:"preserve_workspace,omitempty"`
}

type indexArtifactsRequest struct {
	RunID    string                   `json:"run_id"`
	Manifest appruns.ArtifactManifest `json:"manifest"`
}

type indexMetricsRequest struct {
	RunID   string          `json:"run_id"`
	Metrics appruns.Metrics `json:"metrics"`
}

type internalErrorResponse struct {
	ErrorCode string `json:"error_code"`
	Message   string `json:"message"`
}

func (h *Handler) registerAppFactoryInternalRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/prds:compile", h.handleCompilePRD)
	mux.HandleFunc("GET /api/v1/prds/{prd_id}", h.handleGetPRD)
	mux.HandleFunc("POST /api/v1/prds/", h.handleSubmitPRDApproval)
	mux.HandleFunc("POST /api/v1/approvals", h.handleCreateApproval)
	mux.HandleFunc("GET /api/v1/approvals/{approval_id}", h.handleGetApproval)
	mux.HandleFunc("POST /api/v1/approvals/", h.handleDecideApproval)
	mux.HandleFunc("POST /api/v1/jobs", h.handleCreateJob)
	mux.HandleFunc("POST /api/v1/jobs/", h.handleStartJob)
	mux.HandleFunc("GET /api/v1/jobs", h.handleListJobs)
	mux.HandleFunc("GET /api/v1/jobs/{job_id}", h.handleGetJob)
	mux.HandleFunc("GET /api/v1/jobs/{job_id}/artifacts", h.handleGetJobArtifacts)
	mux.HandleFunc("GET /api/v1/jobs/{job_id}/events", h.handleGetJobEvents)
	mux.HandleFunc("GET /api/v1/notifications", h.handleGetNotifications)
	mux.HandleFunc("POST /api/v1/notifications:rebuild", h.handleRebuildNotifications)
	mux.HandleFunc("POST /api/v1/notifications/", h.handleAckNotification)
	mux.HandleFunc("POST /api/v1/templates:match", h.handleMatchTemplates)
	mux.HandleFunc("GET /api/v1/templates/{template_id}", h.handleGetTemplate)
	mux.HandleFunc("POST /api/v1/templates/", h.handleSubmitTemplateApproval)
	mux.HandleFunc("POST /internal/v1/builders:register", h.handleRegisterBuilder)
	mux.HandleFunc("POST /internal/v1/builders/{builder_id}/heartbeat", h.handleBuilderHeartbeat)
	mux.HandleFunc("POST /internal/v1/workers:allocate", h.handleAllocateWorker)
	mux.HandleFunc("POST /internal/v1/workers/{worker_id}/preserve", h.handlePreserveWorker)
	mux.HandleFunc("POST /internal/v1/workers/{worker_id}/release", h.handleReleaseWorker)
	mux.HandleFunc("POST /internal/v1/requirements:run", h.handleRunRequirement)
	mux.HandleFunc("POST /internal/v1/build-runs", h.handleCreateBuildRun)
	mux.HandleFunc("GET /internal/v1/build-runs/{run_id}", h.handleGetBuildRun)
	mux.HandleFunc("POST /internal/v1/build-runs/{run_id}/heartbeat", h.handleBuildRunHeartbeat)
	mux.HandleFunc("POST /internal/v1/build-runs/{run_id}/complete", h.handleBuildRunComplete)
	mux.HandleFunc("POST /internal/v1/build-runs/{run_id}/fail", h.handleBuildRunFail)
	mux.HandleFunc("POST /internal/v1/build-runs/{run_id}/cancel", h.handleBuildRunCancel)
	mux.HandleFunc("POST /internal/v1/reviews:prepare", h.handlePrepareReview)
	mux.HandleFunc("POST /internal/v1/deliveries:record", h.handleRecordDelivery)
	mux.HandleFunc("POST /internal/v1/artifacts:index", h.handleIndexArtifacts)
	mux.HandleFunc("POST /internal/v1/metrics:index", h.handleIndexMetrics)
	mux.HandleFunc("GET /internal/v1/orchestrator/status", h.handleGetOrchestratorStatus)
	mux.HandleFunc("POST /internal/v1/orchestrator:run", h.handleRunOrchestratorPass)
	mux.HandleFunc("POST /internal/v1/orchestrator:unlock-watch", h.handleUnlockOrchestratorWatch)
	mux.HandleFunc("POST /internal/v1/orchestrator/watch:start", h.handleStartOrchestratorWatch)
	mux.HandleFunc("POST /internal/v1/orchestrator/watch:stop", h.handleStopOrchestratorWatch)
}

func (h *Handler) handleMatchTemplates(w http.ResponseWriter, r *http.Request) {
	var req matchTemplatesRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeInternalError(w, http.StatusBadRequest, "VALIDATION_INVALID_JSON", err.Error())
		return
	}
	if strings.TrimSpace(req.PRDID) == "" {
		writeInternalError(w, http.StatusBadRequest, "VALIDATION_INVALID_REQUEST", "prd_id is required")
		return
	}
	prd, err := h.loadPreparedPRD(req.PRDID, req.PRDVersion)
	if err != nil {
		writeTemplateMatchError(w, err)
		return
	}
	items := appprepare.MatchTemplates(prd.TemplateConstraints, req.MaxResults)
	responseItems := make([]map[string]any, 0, len(items))
	for _, item := range items {
		responseItems = append(responseItems, map[string]any{
			"template_id":      item.Entry.TemplateID,
			"name":             item.Entry.Name,
			"score":            item.Score,
			"hard_gate_passed": item.HardGatePassed,
			"pinned_ref":       item.Entry.PinnedRef,
			"health_status":    item.Entry.HealthStatus,
			"missing_features": item.MissingFeatures,
			"reasons":          item.Reasons,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"prd_id":      prd.ID,
		"prd_version": prd.Version,
		"items":       responseItems,
	})
}

func (h *Handler) handleGetPRD(w http.ResponseWriter, r *http.Request) {
	prdID := strings.TrimSpace(r.PathValue("prd_id"))
	if prdID == "" {
		writeInternalError(w, http.StatusBadRequest, "VALIDATION_INVALID_REQUEST", "prd_id is required")
		return
	}
	prd, err := h.loadPreparedPRD(prdID, strings.TrimSpace(r.URL.Query().Get("version")))
	if err != nil {
		writePRDReadError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, prd)
}

func (h *Handler) handleCreateJob(w http.ResponseWriter, r *http.Request) {
	var req createJobRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeInternalError(w, http.StatusBadRequest, "VALIDATION_INVALID_JSON", err.Error())
		return
	}
	if strings.TrimSpace(req.PRDID) == "" {
		writeInternalError(w, http.StatusBadRequest, "VALIDATION_INVALID_REQUEST", "prd_id is required")
		return
	}
	if strings.TrimSpace(req.TemplateID) == "" {
		writeInternalError(w, http.StatusBadRequest, "VALIDATION_INVALID_REQUEST", "template_id is required")
		return
	}
	bundle, err := h.findPreparedBundleByPRD(req.PRDID, req.PRDVersion)
	if err != nil {
		writeJobError(w, err)
		return
	}
	if bundle.BuilderInput.TemplateID != "" && strings.TrimSpace(req.TemplateID) != bundle.BuilderInput.TemplateID {
		writeInternalError(w, http.StatusBadRequest, "VALIDATION_INVALID_REQUEST", fmt.Sprintf("prepared template mismatch: want %s got %s", bundle.BuilderInput.TemplateID, strings.TrimSpace(req.TemplateID)))
		return
	}
	if _, err := appprepare.GetTemplateRegistryEntry(strings.TrimSpace(req.TemplateID)); err != nil {
		writeInternalError(w, http.StatusNotFound, "TEMPLATE_NOT_FOUND", err.Error())
		return
	}
	record, err := h.createPublicJob(bundle)
	if err != nil {
		writeJobError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, record)
}

func (h *Handler) handleGetJob(w http.ResponseWriter, r *http.Request) {
	jobID := strings.TrimSpace(r.PathValue("job_id"))
	if jobID == "" {
		writeInternalError(w, http.StatusBadRequest, "VALIDATION_INVALID_REQUEST", "job_id is required")
		return
	}
	record, err := h.buildPublicJobRecord(jobID)
	if err != nil {
		writeJobError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, record)
}

func (h *Handler) handleListJobs(w http.ResponseWriter, r *http.Request) {
	items, err := h.loadPublicJobs(r.URL.Query().Get("status"))
	if err != nil {
		writeInternalError(w, http.StatusInternalServerError, "JOB_LIST_FAILED", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h *Handler) handleGetJobArtifacts(w http.ResponseWriter, r *http.Request) {
	jobID := strings.TrimSpace(r.PathValue("job_id"))
	if jobID == "" {
		writeInternalError(w, http.StatusBadRequest, "VALIDATION_INVALID_REQUEST", "job_id is required")
		return
	}
	manifest, err := h.loadJobArtifacts(jobID)
	if err != nil {
		writeJobError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, manifest)
}

func (h *Handler) handleGetJobEvents(w http.ResponseWriter, r *http.Request) {
	jobID := strings.TrimSpace(r.PathValue("job_id"))
	if jobID == "" {
		writeInternalError(w, http.StatusBadRequest, "VALIDATION_INVALID_REQUEST", "job_id is required")
		return
	}
	items, err := h.loadJobEvents(jobID)
	if err != nil {
		writeJobError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"job_id": jobID, "items": items})
}

func (h *Handler) handleStartJob(w http.ResponseWriter, r *http.Request) {
	if jobID, ok := parseJobResumePath(r.URL.Path); ok {
		var req resumeJobRequest
		if r.ContentLength > 0 {
			if err := decodeJSONBody(r, &req); err != nil {
				writeInternalError(w, http.StatusBadRequest, "VALIDATION_INVALID_JSON", err.Error())
				return
			}
		}
		record, err := h.resumePublicJob(r.Context(), jobID, req)
		if err != nil {
			writeJobResumeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, record)
		return
	}

	if jobID, ok := parseJobCancelPath(r.URL.Path); ok {
		var req cancelJobRequest
		if r.ContentLength > 0 {
			if err := decodeJSONBody(r, &req); err != nil {
				writeInternalError(w, http.StatusBadRequest, "VALIDATION_INVALID_JSON", err.Error())
				return
			}
		}
		record, err := h.cancelPublicJob(r.Context(), jobID, req)
		if err != nil {
			writeJobCancelError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, record)
		return
	}

	jobID, ok := parseJobStartPath(r.URL.Path)
	if !ok {
		writeInternalError(w, http.StatusNotFound, "JOB_NOT_FOUND", "job action route not found")
		return
	}
	var req startJobRequest
	if r.ContentLength > 0 {
		if err := decodeJSONBody(r, &req); err != nil {
			writeInternalError(w, http.StatusBadRequest, "VALIDATION_INVALID_JSON", err.Error())
			return
		}
	}
	record, err := h.startPublicJob(r.Context(), jobID, req)
	if err != nil {
		writeJobStartError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, record)
}

func (h *Handler) handleGetNotifications(w http.ResponseWriter, r *http.Request) {
	items, err := h.loadNotifications()
	if err != nil {
		writeNotificationError(w, err)
		return
	}
	items = filterPublicNotifications(items, r.URL.Query())
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h *Handler) handleAckNotification(w http.ResponseWriter, r *http.Request) {
	notificationID, ok := parseNotificationAckPath(r.URL.Path)
	if !ok {
		writeInternalError(w, http.StatusNotFound, "NOTIFICATION_NOT_FOUND", "notification ack route not found")
		return
	}
	item, err := h.acknowledgeNotification(notificationID)
	if err != nil {
		writeNotificationError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (h *Handler) handleRebuildNotifications(w http.ResponseWriter, r *http.Request) {
	items, err := h.loadNotifications()
	if err != nil {
		writeNotificationError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "rebuild": true})
}

func (h *Handler) handleGetOrchestratorStatus(w http.ResponseWriter, r *http.Request) {
	status, err := h.LoadPublicJobOrchestratorStatus()
	if err != nil {
		writeInternalError(w, http.StatusInternalServerError, "APPFACTORY_ORCHESTRATOR_STATUS_ERROR", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, status)
}

func (h *Handler) handleRunOrchestratorPass(w http.ResponseWriter, r *http.Request) {
	status, err := h.RunPublicJobOrchestratorPass("internal_api")
	if err != nil {
		writeInternalError(w, http.StatusInternalServerError, "APPFACTORY_ORCHESTRATOR_RUN_ERROR", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, status)
}

func (h *Handler) handleUnlockOrchestratorWatch(w http.ResponseWriter, r *http.Request) {
	var req unlockOrchestratorWatchRequest
	if r.ContentLength > 0 {
		if err := decodeJSONBody(r, &req); err != nil {
			writeInternalError(w, http.StatusBadRequest, "VALIDATION_INVALID_JSON", err.Error())
			return
		}
	}
	status, err := h.UnlockPublicJobOrchestratorWatchLock(req.Force)
	if err != nil {
		writeInternalError(w, http.StatusConflict, "APPFACTORY_ORCHESTRATOR_WATCH_LOCK_CONFLICT", err.Error())
		return
	}
	_ = h.auditPublicJobOrchestratorWatchAction(r, "orchestrator_watch_unlocked", req.Force, orchestratorWatchActionSummary("unlocked", status, req.Force), status)
	writeJSON(w, http.StatusOK, status)
}

func (h *Handler) handleStartOrchestratorWatch(w http.ResponseWriter, r *http.Request) {
	var req startOrchestratorWatchRequest
	if r.ContentLength > 0 {
		if err := decodeJSONBody(r, &req); err != nil {
			writeInternalError(w, http.StatusBadRequest, "VALIDATION_INVALID_JSON", err.Error())
			return
		}
	}
	interval := 15 * time.Second
	if req.IntervalSeconds > 0 {
		interval = time.Duration(req.IntervalSeconds) * time.Second
	}
	status, err := h.StartPublicJobOrchestratorWatch(interval)
	if err != nil {
		writeInternalError(w, http.StatusConflict, "APPFACTORY_ORCHESTRATOR_WATCH_CONFLICT", err.Error())
		return
	}
	_ = h.auditPublicJobOrchestratorWatchAction(r, "orchestrator_watch_started", false, orchestratorWatchActionSummary("started", status, false), status)
	writeJSON(w, http.StatusOK, status)
}

func (h *Handler) handleStopOrchestratorWatch(w http.ResponseWriter, r *http.Request) {
	if err := h.StopPublicJobOrchestratorWatch(); err != nil {
		writeInternalError(w, http.StatusInternalServerError, "APPFACTORY_ORCHESTRATOR_WATCH_STOP_ERROR", err.Error())
		return
	}
	status, err := h.LoadPublicJobOrchestratorStatus()
	if err != nil {
		writeInternalError(w, http.StatusInternalServerError, "APPFACTORY_ORCHESTRATOR_STATUS_ERROR", err.Error())
		return
	}
	_ = h.auditPublicJobOrchestratorWatchAction(r, "orchestrator_watch_stopped", false, orchestratorWatchActionSummary("stopped", status, false), status)
	writeJSON(w, http.StatusOK, status)
}

func (h *Handler) auditPublicJobOrchestratorWatchAction(r *http.Request, action string, force bool, summary string, status PublicJobOrchestratorStatus) error {
	createdAt := time.Now().UTC().Format(time.RFC3339)
	record := publicJobOrchestratorWatchAuditRecord{
		SchemaVersion:    "0.1.0",
		AuditID:          fmt.Sprintf("orchestrator-watch-audit-%s-%s", normalizeNotificationTimestamp(createdAt), strings.TrimSpace(action)),
		Action:           strings.TrimSpace(action),
		Actor:            strings.TrimSpace(r.RemoteAddr),
		RemoteAddr:       strings.TrimSpace(r.RemoteAddr),
		UserAgent:        strings.TrimSpace(r.UserAgent()),
		Force:            force,
		Summary:          strings.TrimSpace(summary),
		WatchRunnerState: strings.TrimSpace(status.WatchRunnerState),
		WatchRunnerMode:  strings.TrimSpace(status.WatchRunnerMode),
		WatchLockState:   strings.TrimSpace(status.WatchLockState),
		WatchLockOwnerID: strings.TrimSpace(status.WatchLockOwnerID),
		CreatedAt:        createdAt,
	}
	if err := h.appendPublicJobOrchestratorWatchAudit(record); err != nil {
		return err
	}
	_, _ = h.loadNotifications()
	return nil
}

func orchestratorWatchActionSummary(action string, status PublicJobOrchestratorStatus, force bool) string {
	switch strings.TrimSpace(action) {
	case "started":
		return fmt.Sprintf("orchestrator watch started with interval=%ds", status.WatchRunnerInterval)
	case "stopped":
		return "orchestrator watch stopped"
	case "unlocked":
		if force {
			return "orchestrator watch lock force unlocked"
		}
		return "orchestrator watch lock unlocked"
	default:
		return "orchestrator watch state changed"
	}
}

func (h *Handler) handleCreateApproval(w http.ResponseWriter, r *http.Request) {
	var req createApprovalRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeInternalError(w, http.StatusBadRequest, "VALIDATION_INVALID_JSON", err.Error())
		return
	}
	if strings.TrimSpace(req.ApprovalType) == "" {
		writeInternalError(w, http.StatusBadRequest, "VALIDATION_INVALID_REQUEST", "approval_type is required")
		return
	}
	record, err := h.createApproval(req)
	if err != nil {
		writeApprovalError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, record)
}

func (h *Handler) handleGetApproval(w http.ResponseWriter, r *http.Request) {
	approvalID := strings.TrimSpace(r.PathValue("approval_id"))
	if approvalID == "" {
		writeInternalError(w, http.StatusBadRequest, "VALIDATION_INVALID_REQUEST", "approval_id is required")
		return
	}
	record, err := h.loadApprovalRecord(approvalID)
	if err != nil {
		writeApprovalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, record)
}

func (h *Handler) handleDecideApproval(w http.ResponseWriter, r *http.Request) {
	approvalID, ok := parseApprovalDecisionPath(r.URL.Path)
	if !ok {
		writeInternalError(w, http.StatusNotFound, "APPROVAL_NOT_FOUND", "approval decision route not found")
		return
	}
	var req approvalDecisionRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeInternalError(w, http.StatusBadRequest, "VALIDATION_INVALID_JSON", err.Error())
		return
	}
	if strings.TrimSpace(req.Decision) == "" {
		writeInternalError(w, http.StatusBadRequest, "VALIDATION_INVALID_REQUEST", "decision is required")
		return
	}
	if strings.TrimSpace(req.ReviewerID) == "" {
		writeInternalError(w, http.StatusBadRequest, "VALIDATION_INVALID_REQUEST", "reviewer_id is required")
		return
	}
	record, err := h.decideApproval(approvalID, req)
	if err != nil {
		writeApprovalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, record)
}

func (h *Handler) handleSubmitPRDApproval(w http.ResponseWriter, r *http.Request) {
	prdID, ok := parsePRDApprovalPath(r.URL.Path)
	if !ok {
		writeInternalError(w, http.StatusNotFound, "PRD_NOT_FOUND", "prd approval route not found")
		return
	}
	if prdID == "" {
		writeInternalError(w, http.StatusBadRequest, "VALIDATION_INVALID_REQUEST", "prd_id is required")
		return
	}
	var req submitPRDApprovalRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeInternalError(w, http.StatusBadRequest, "VALIDATION_INVALID_JSON", err.Error())
		return
	}
	if strings.TrimSpace(req.JobID) == "" {
		writeInternalError(w, http.StatusBadRequest, "VALIDATION_INVALID_REQUEST", "job_id is required")
		return
	}
	record, err := h.submitPRDApproval(prdID, req)
	if err != nil {
		writePRDApprovalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, record)
}

func (h *Handler) handleGetTemplate(w http.ResponseWriter, r *http.Request) {
	templateID := strings.TrimSpace(r.PathValue("template_id"))
	if templateID == "" {
		writeInternalError(w, http.StatusBadRequest, "VALIDATION_INVALID_REQUEST", "template_id is required")
		return
	}
	entry, err := appprepare.GetTemplateRegistryEntry(templateID)
	if err != nil {
		writeInternalError(w, http.StatusNotFound, "TEMPLATE_NOT_FOUND", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"schema_version":  "0.1.0",
		"template_id":     entry.TemplateID,
		"name":            entry.Name,
		"description":     entry.Description,
		"repo_url":        "local://appfactory/templates/" + entry.TemplateID,
		"pinned_ref":      entry.PinnedRef,
		"license":         entry.License,
		"stack":           entry.Stack,
		"android_support": entry.AndroidSupport,
		"capabilities":    entry.Capabilities,
		"build_proof": map[string]any{
			"status":      "passed",
			"verified_at": "2026-03-28T00:00:00Z",
		},
		"test_proof": map[string]any{
			"status":      "unknown",
			"verified_at": "2026-03-28T00:00:00Z",
		},
		"health_status":   entry.HealthStatus,
		"risk_notes":      entry.RiskNotes,
		"selection_hints": entry.SelectionHints,
		"owner": map[string]any{
			"name": "appfactory-prepare",
		},
		"last_checked_at": "2026-03-28T00:00:00Z",
	})
}

func (h *Handler) handleSubmitTemplateApproval(w http.ResponseWriter, r *http.Request) {
	templateID, ok := parseTemplateApprovalPath(r.URL.Path)
	if !ok {
		writeInternalError(w, http.StatusNotFound, "TEMPLATE_NOT_FOUND", "template approval route not found")
		return
	}
	if templateID == "" {
		writeInternalError(w, http.StatusBadRequest, "VALIDATION_INVALID_REQUEST", "template_id is required")
		return
	}
	entry, err := appprepare.GetTemplateRegistryEntry(templateID)
	if err != nil {
		writeInternalError(w, http.StatusNotFound, "TEMPLATE_NOT_FOUND", err.Error())
		return
	}
	var req submitTemplateApprovalRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeInternalError(w, http.StatusBadRequest, "VALIDATION_INVALID_JSON", err.Error())
		return
	}
	if strings.TrimSpace(req.JobID) == "" {
		writeInternalError(w, http.StatusBadRequest, "VALIDATION_INVALID_REQUEST", "job_id is required")
		return
	}
	if strings.TrimSpace(req.PRDID) == "" {
		writeInternalError(w, http.StatusBadRequest, "VALIDATION_INVALID_REQUEST", "prd_id is required")
		return
	}
	record, err := h.submitTemplateApproval(entry, req)
	if err != nil {
		writeTemplateApprovalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, record)
}

func parseTemplateApprovalPath(path string) (string, bool) {
	const prefix = "/api/v1/templates/"
	const suffix = ":submit-approval"
	if !strings.HasPrefix(path, prefix) || !strings.HasSuffix(path, suffix) {
		return "", false
	}
	templateID := strings.TrimSuffix(strings.TrimPrefix(path, prefix), suffix)
	templateID = strings.TrimSpace(templateID)
	if templateID == "" || strings.Contains(templateID, "/") {
		return "", false
	}
	return templateID, true
}

func parsePRDApprovalPath(path string) (string, bool) {
	const prefix = "/api/v1/prds/"
	const suffix = ":submit-approval"
	if !strings.HasPrefix(path, prefix) || !strings.HasSuffix(path, suffix) {
		return "", false
	}
	prdID := strings.TrimSuffix(strings.TrimPrefix(path, prefix), suffix)
	prdID = strings.TrimSpace(prdID)
	if prdID == "" || strings.Contains(prdID, "/") {
		return "", false
	}
	return prdID, true
}

func parseApprovalDecisionPath(path string) (string, bool) {
	const prefix = "/api/v1/approvals/"
	const suffix = ":decision"
	if !strings.HasPrefix(path, prefix) || !strings.HasSuffix(path, suffix) {
		return "", false
	}
	approvalID := strings.TrimSuffix(strings.TrimPrefix(path, prefix), suffix)
	approvalID = strings.TrimSpace(approvalID)
	if approvalID == "" || strings.Contains(approvalID, "/") {
		return "", false
	}
	return approvalID, true
}

func parseJobStartPath(path string) (string, bool) {
	const prefix = "/api/v1/jobs/"
	const suffix = ":start"
	if !strings.HasPrefix(path, prefix) || !strings.HasSuffix(path, suffix) {
		return "", false
	}
	jobID := strings.TrimSuffix(strings.TrimPrefix(path, prefix), suffix)
	jobID = strings.TrimSpace(jobID)
	if jobID == "" || strings.Contains(jobID, "/") {
		return "", false
	}
	return jobID, true
}

func parseJobCancelPath(path string) (string, bool) {
	const prefix = "/api/v1/jobs/"
	const suffix = ":cancel"
	if !strings.HasPrefix(path, prefix) || !strings.HasSuffix(path, suffix) {
		return "", false
	}
	jobID := strings.TrimSuffix(strings.TrimPrefix(path, prefix), suffix)
	jobID = strings.TrimSpace(jobID)
	if jobID == "" || strings.Contains(jobID, "/") {
		return "", false
	}
	return jobID, true
}

func parseJobResumePath(path string) (string, bool) {
	const prefix = "/api/v1/jobs/"
	const suffix = ":resume"
	if !strings.HasPrefix(path, prefix) || !strings.HasSuffix(path, suffix) {
		return "", false
	}
	jobID := strings.TrimSuffix(strings.TrimPrefix(path, prefix), suffix)
	jobID = strings.TrimSpace(jobID)
	if jobID == "" || strings.Contains(jobID, "/") {
		return "", false
	}
	return jobID, true
}

func parseNotificationAckPath(path string) (string, bool) {
	const prefix = "/api/v1/notifications/"
	const suffix = ":ack"
	if !strings.HasPrefix(path, prefix) || !strings.HasSuffix(path, suffix) {
		return "", false
	}
	notificationID := strings.TrimSuffix(strings.TrimPrefix(path, prefix), suffix)
	notificationID = strings.TrimSpace(notificationID)
	if notificationID == "" || strings.Contains(notificationID, "/") {
		return "", false
	}
	return notificationID, true
}

func (h *Handler) handleCompilePRD(w http.ResponseWriter, r *http.Request) {
	var req compilePRDRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeInternalError(w, http.StatusBadRequest, "VALIDATION_INVALID_JSON", err.Error())
		return
	}
	bundle, bundleDir, err := h.compilePreparedBundle(r.Context(), compileRequestInput{
		RequirementText:   req.RequirementText,
		RequirementSource: req.RequirementSource,
		Title:             req.Title,
		JobID:             req.JobID,
		PRDID:             req.PRDID,
		TemplateID:        req.TemplateID,
		RealChecks:        req.RealChecks,
		ExecutorImage:     req.ExecutorImage,
	})
	if err != nil {
		writeCompilePRDError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"job_id":      bundle.BuilderInput.JobID,
		"prd_id":      bundle.BuilderInput.PRDID,
		"template_id": bundle.BuilderInput.TemplateID,
		"bundle_dir":  bundleDir,
		"files": map[string]any{
			"requirement_path":         filepath.Join(bundleDir, "requirement.md"),
			"prd_markdown_path":        filepath.Join(bundleDir, "PRD.md"),
			"prd_json_path":            filepath.Join(bundleDir, "PRD.json"),
			"prd_approval_path":        filepath.Join(bundleDir, appruns.PRDApprovalFileName),
			"template_approval_path":   filepath.Join(bundleDir, appruns.TemplateApprovalFileName),
			"template_fit_report_path": filepath.Join(bundleDir, "template-fit-report.md"),
			"implementation_plan_path": filepath.Join(bundleDir, "implementation-plan.md"),
			"builder_input_path":       filepath.Join(bundleDir, "builder-input.json"),
		},
	})
}

func (h *Handler) handleRunRequirement(w http.ResponseWriter, r *http.Request) {
	var req runRequirementRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeInternalError(w, http.StatusBadRequest, "VALIDATION_INVALID_JSON", err.Error())
		return
	}
	if strings.TrimSpace(req.RequirementText) == "" {
		writeInternalError(w, http.StatusBadRequest, "VALIDATION_INVALID_REQUEST", "requirement_text is required")
		return
	}
	if strings.TrimSpace(req.BuilderID) == "" {
		writeInternalError(w, http.StatusBadRequest, "VALIDATION_INVALID_REQUEST", "builder_id is required")
		return
	}
	bundle, bundleDir, err := h.compilePreparedBundle(r.Context(), compileRequestInput{
		RequirementText:   req.RequirementText,
		RequirementSource: req.RequirementSource,
		Title:             req.Title,
		JobID:             req.JobID,
		PRDID:             req.PRDID,
		TemplateID:        req.TemplateID,
		RealChecks:        req.RealChecks,
		ExecutorImage:     req.ExecutorImage,
	})
	if err != nil {
		writeCompilePRDError(w, err)
		return
	}
	bundle.BuilderInput.ExecutorImage = strings.TrimSpace(req.ExecutorImage)
	bundle.BuilderInput.ContextSourceDir = bundleDir
	result, err := h.executePreparedRequirementRun(r.Context(), bundle.BuilderInput, req)
	if err != nil {
		writeRunRequirementError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"job_id":      bundle.BuilderInput.JobID,
		"prd_id":      bundle.BuilderInput.PRDID,
		"template_id": bundle.BuilderInput.TemplateID,
		"bundle_dir":  bundleDir,
		"run_id":      result.RunID,
		"worker_id":   result.WorkerID,
		"status":      result.Status,
	})
}

func (h *Handler) handleRegisterBuilder(w http.ResponseWriter, r *http.Request) {
	var req registerBuilderRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeInternalError(w, http.StatusBadRequest, "VALIDATION_INVALID_JSON", err.Error())
		return
	}
	service, err := h.buildersControlPlane()
	if err != nil {
		writeInternalError(w, http.StatusInternalServerError, "BUILDERS_INIT_FAILED", err.Error())
		return
	}
	node, err := service.Register(r.Context(), appbuilders.RegisterRequest{
		BuilderID:       req.BuilderID,
		DisplayName:     req.DisplayName,
		Endpoint:        req.Endpoint,
		CapabilityTags:  req.CapabilityTags,
		ModelTags:       req.ModelTags,
		Priority:        req.Priority,
		MaxParallelRuns: req.MaxParallelRuns,
		WorkerProfile:   req.WorkerProfile,
	})
	if err != nil {
		writeBuilderDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, node)
}

func (h *Handler) handleBuilderHeartbeat(w http.ResponseWriter, r *http.Request) {
	var req heartbeatBuilderRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeInternalError(w, http.StatusBadRequest, "VALIDATION_INVALID_JSON", err.Error())
		return
	}
	service, err := h.buildersControlPlane()
	if err != nil {
		writeInternalError(w, http.StatusInternalServerError, "BUILDERS_INIT_FAILED", err.Error())
		return
	}
	node, err := service.Heartbeat(r.Context(), appbuilders.HeartbeatRequest{
		BuilderID:         r.PathValue("builder_id"),
		Status:            req.Status,
		CurrentRunID:      req.CurrentRunID,
		LastFailureReason: req.LastFailureReason,
		Load:              req.Load,
		FreeDiskBytes:     req.FreeDiskBytes,
	})
	if err != nil {
		writeBuilderDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, node)
}

func (h *Handler) handleAllocateWorker(w http.ResponseWriter, r *http.Request) {
	var req allocateWorkerRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeInternalError(w, http.StatusBadRequest, "VALIDATION_INVALID_JSON", err.Error())
		return
	}
	service, err := h.buildersControlPlane()
	if err != nil {
		writeInternalError(w, http.StatusInternalServerError, "BUILDERS_INIT_FAILED", err.Error())
		return
	}
	result, err := service.Dispatch(r.Context(), appbuilders.Requirement{
		JobID:                   req.JobID,
		RequiredCapabilityTags:  req.RequiredCapabilityTags,
		PreferredCapabilityTags: req.PreferredCapabilityTags,
		ModelTier:               req.ModelTier,
		BudgetClass:             req.BudgetClass,
		WorkerProfileName:       req.WorkerProfileName,
	})
	if err != nil {
		writeBuilderDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"worker_id":  result.Node.BuilderID,
		"builder_id": result.Node.BuilderID,
		"lease_id":   result.Lease.LeaseID,
		"status":     result.Node.Status,
		"decision":   result.Decision,
	})
}

func (h *Handler) handlePreserveWorker(w http.ResponseWriter, r *http.Request) {
	var req preserveWorkerRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeInternalError(w, http.StatusBadRequest, "VALIDATION_INVALID_JSON", err.Error())
		return
	}
	service, err := h.buildersControlPlane()
	if err != nil {
		writeInternalError(w, http.StatusInternalServerError, "BUILDERS_INIT_FAILED", err.Error())
		return
	}
	if err := service.Drain(r.Context(), r.PathValue("worker_id"), req.Reason); err != nil {
		writeBuilderDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) handleReleaseWorker(w http.ResponseWriter, r *http.Request) {
	var req releaseWorkerRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeInternalError(w, http.StatusBadRequest, "VALIDATION_INVALID_JSON", err.Error())
		return
	}
	service, err := h.buildersControlPlane()
	if err != nil {
		writeInternalError(w, http.StatusInternalServerError, "BUILDERS_INIT_FAILED", err.Error())
		return
	}
	reason := req.Reason
	if reason == "" {
		reason = "worker released"
	}
	if err := service.ReleaseByBuilder(r.Context(), r.PathValue("worker_id"), reason); err != nil {
		writeBuilderDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) handleCreateBuildRun(w http.ResponseWriter, r *http.Request) {
	var req createBuildRunRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeInternalError(w, http.StatusBadRequest, "VALIDATION_INVALID_JSON", err.Error())
		return
	}
	builderSvc, err := h.buildersControlPlane()
	if err != nil {
		writeInternalError(w, http.StatusInternalServerError, "BUILDERS_INIT_FAILED", err.Error())
		return
	}
	runsSvc, err := h.buildRunsControlPlane()
	if err != nil {
		writeInternalError(w, http.StatusInternalServerError, "RUNS_INIT_FAILED", err.Error())
		return
	}
	run, err := runsSvc.Create(r.Context(), req.WorkerID, req.WorkerID, req.LeaseID, req.BuilderInput)
	if err != nil {
		writeRunDomainError(w, err)
		return
	}
	if err := builderSvc.BindRun(r.Context(), req.LeaseID, run.RunID); err != nil {
		writeBuilderDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"run_id":             run.RunID,
		"worker_id":          run.WorkerID,
		"status":             run.Status,
		"input_path":         run.InputPath,
		"workspace_path":     run.WorkspacePath,
		"artifact_dir":       run.ArtifactDir,
		"runner_script_path": run.RunnerScriptPath,
		"launch_command":     run.LaunchCommand,
		"launch_args":        run.LaunchArgs,
		"log_path":           run.LogPath,
	})
}

func (h *Handler) handleGetBuildRun(w http.ResponseWriter, r *http.Request) {
	runsSvc, err := h.buildRunsControlPlane()
	if err != nil {
		writeInternalError(w, http.StatusInternalServerError, "RUNS_INIT_FAILED", err.Error())
		return
	}
	run, err := runsSvc.Get(r.Context(), r.PathValue("run_id"))
	if err != nil {
		writeRunDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, run)
}

func (h *Handler) handleBuildRunHeartbeat(w http.ResponseWriter, r *http.Request) {
	var req heartbeatRunRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeInternalError(w, http.StatusBadRequest, "VALIDATION_INVALID_JSON", err.Error())
		return
	}
	runsSvc, err := h.buildRunsControlPlane()
	if err != nil {
		writeInternalError(w, http.StatusInternalServerError, "RUNS_INIT_FAILED", err.Error())
		return
	}
	run, err := runsSvc.Heartbeat(r.Context(), r.PathValue("run_id"), appruns.Heartbeat{
		Stage:             req.Stage,
		Iteration:         req.Iteration,
		FailureSignatures: req.FailureSignatures,
		TotalTokens:       req.TotalTokens,
		Summary:           req.Summary,
	})
	if err != nil {
		writeRunDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ack": true, "status": run.Status})
}

func (h *Handler) handleBuildRunComplete(w http.ResponseWriter, r *http.Request) {
	var req completeBuildRunRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeInternalError(w, http.StatusBadRequest, "VALIDATION_INVALID_JSON", err.Error())
		return
	}
	runsSvc, err := h.buildRunsControlPlane()
	if err != nil {
		writeInternalError(w, http.StatusInternalServerError, "RUNS_INIT_FAILED", err.Error())
		return
	}
	run, err := runsSvc.Complete(r.Context(), r.PathValue("run_id"), req.BuilderOutput)
	if err != nil {
		writeRunDomainError(w, err)
		return
	}
	builderSvc, err := h.buildersControlPlane()
	if err != nil {
		writeInternalError(w, http.StatusInternalServerError, "BUILDERS_INIT_FAILED", err.Error())
		return
	}
	if err := builderSvc.ReleaseByRun(r.Context(), r.PathValue("run_id"), "run completed"); err != nil {
		writeBuilderDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ack": true, "status": run.Status})
}

func (h *Handler) handlePrepareReview(w http.ResponseWriter, r *http.Request) {
	var req prepareReviewRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeInternalError(w, http.StatusBadRequest, "VALIDATION_INVALID_JSON", err.Error())
		return
	}
	if strings.TrimSpace(req.RunID) == "" {
		writeInternalError(w, http.StatusBadRequest, "VALIDATION_INVALID_REQUEST", "run_id is required")
		return
	}
	runsSvc, err := h.buildRunsControlPlane()
	if err != nil {
		writeInternalError(w, http.StatusInternalServerError, "RUNS_INIT_FAILED", err.Error())
		return
	}
	run, err := runsSvc.Get(r.Context(), req.RunID)
	if err != nil {
		writeRunDomainError(w, err)
		return
	}
	prepared, err := prepareRunReviewArtifacts(r.Context(), runsSvc, run, deriveReviewPreparationOutcome(run))
	if err != nil {
		writeInternalError(w, http.StatusInternalServerError, "REVIEW_PREPARE_FAILED", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ack":                         true,
		"run_id":                      run.RunID,
		"job_id":                      run.JobID,
		"status":                      run.Status,
		"review_bundle_metadata_path": prepared.ReviewMetadataPath,
		"review_bundle_path":          prepared.ReviewBundlePath,
		"handoff_checklist_path":      prepared.HandoffChecklistPath,
		"artifact_manifest_path":      prepared.ArtifactManifestPath,
		"metrics_path":                prepared.MetricsPath,
	})
}

func (h *Handler) handleRecordDelivery(w http.ResponseWriter, r *http.Request) {
	var req recordDeliveryRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeInternalError(w, http.StatusBadRequest, "VALIDATION_INVALID_JSON", err.Error())
		return
	}
	req.RunID = strings.TrimSpace(req.RunID)
	req.ReviewerID = strings.TrimSpace(req.ReviewerID)
	req.Status = normalizeDeliveryStatus(req.Status)
	req.Summary = strings.TrimSpace(req.Summary)
	req.NextAction = strings.TrimSpace(req.NextAction)
	req.ReleaseChannel = normalizeDeliveryReleaseChannel(req.ReleaseChannel)
	req.EvidencePaths = normalizeStringList(req.EvidencePaths)
	req.RequiredChanges = normalizeStringList(req.RequiredChanges)
	req.SignedArtifactPaths = normalizeStringList(req.SignedArtifactPaths)
	if req.RunID == "" {
		writeInternalError(w, http.StatusBadRequest, "VALIDATION_INVALID_REQUEST", "run_id is required")
		return
	}
	if req.ReviewerID == "" {
		writeInternalError(w, http.StatusBadRequest, "VALIDATION_INVALID_REQUEST", "reviewer_id is required")
		return
	}
	if !isSupportedDeliveryStatus(req.Status) {
		writeInternalError(w, http.StatusBadRequest, "VALIDATION_INVALID_REQUEST", "status is not supported")
		return
	}
	if !isSupportedDeliveryReleaseChannel(req.ReleaseChannel) {
		writeInternalError(w, http.StatusBadRequest, "VALIDATION_INVALID_REQUEST", "release_channel is not supported")
		return
	}
	if req.RolloutPercent < 0 || req.RolloutPercent > 100 {
		writeInternalError(w, http.StatusBadRequest, "VALIDATION_INVALID_REQUEST", "rollout_percent must be between 0 and 100")
		return
	}
	if req.NextAction == "" {
		req.NextAction = defaultDeliveryNextAction(req.Status)
	}
	if req.Summary == "" {
		req.Summary = defaultDeliverySummary(req.Status, req.ReleaseChannel, req.RolloutPercent)
	}

	runsSvc, err := h.buildRunsControlPlane()
	if err != nil {
		writeInternalError(w, http.StatusInternalServerError, "RUNS_INIT_FAILED", err.Error())
		return
	}
	run, err := runsSvc.Get(r.Context(), req.RunID)
	if err != nil {
		writeRunDomainError(w, err)
		return
	}
	prepared, err := prepareRunReviewArtifacts(r.Context(), runsSvc, run, deriveReviewPreparationOutcome(run))
	if err != nil {
		writeInternalError(w, http.StatusInternalServerError, "DELIVERY_PREPARE_FAILED", err.Error())
		return
	}
	workspace, err := h.appFactoryWorkspacePath()
	if err != nil {
		writeInternalError(w, http.StatusInternalServerError, "WORKSPACE_INIT_FAILED", err.Error())
		return
	}
	delivery := deliveryRecord{
		SchemaVersion:       "0.1.0",
		JobID:               run.JobID,
		RunID:               run.RunID,
		Status:              req.Status,
		Summary:             req.Summary,
		NextAction:          req.NextAction,
		ReleaseChannel:      req.ReleaseChannel,
		RolloutPercent:      req.RolloutPercent,
		ReviewerID:          req.ReviewerID,
		EvidencePaths:       req.EvidencePaths,
		RequiredChanges:     req.RequiredChanges,
		SignedArtifactPaths: req.SignedArtifactPaths,
		RecordedAt:          time.Now().UTC().Format(time.RFC3339),
	}
	if err := writeDeliveryRecord(workspace, delivery); err != nil {
		writeInternalError(w, http.StatusInternalServerError, "DELIVERY_RECORD_WRITE_FAILED", err.Error())
		return
	}
	manifestPath := filepath.Join(workspace, "appfactory", filepath.FromSlash(prepared.ArtifactManifestPath))
	if err := upsertArtifactManifestItem(manifestPath, run.JobID, deliveryRecordArtifactItem()); err != nil {
		writeInternalError(w, http.StatusInternalServerError, "DELIVERY_MANIFEST_UPDATE_FAILED", err.Error())
		return
	}
	publicRecord, err := h.buildPublicJobRecord(run.JobID)
	if err != nil {
		writeInternalError(w, http.StatusInternalServerError, "JOB_REBUILD_FAILED", err.Error())
		return
	}
	if err := h.persistPublicJobRecord(publicRecord); err != nil {
		writeInternalError(w, http.StatusInternalServerError, "JOB_WRITE_FAILED", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ack":                  true,
		"job_id":               run.JobID,
		"run_id":               run.RunID,
		"status":               delivery.Status,
		"next_action":          delivery.NextAction,
		"delivery_record_path": deliveryRecordReportPath(),
	})
}

func (h *Handler) handleBuildRunFail(w http.ResponseWriter, r *http.Request) {
	var req failBuildRunRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeInternalError(w, http.StatusBadRequest, "VALIDATION_INVALID_JSON", err.Error())
		return
	}
	runsSvc, err := h.buildRunsControlPlane()
	if err != nil {
		writeInternalError(w, http.StatusInternalServerError, "RUNS_INIT_FAILED", err.Error())
		return
	}
	run, err := runsSvc.Fail(r.Context(), r.PathValue("run_id"), appruns.FailureReport{
		Summary:            req.Summary,
		RecoverySuggestion: req.RecoverySuggestion,
		FailureSignatures:  req.FailureSignatures,
		RoundState:         req.RoundState,
		RepairContext:      req.RepairContext,
	})
	if err != nil {
		writeRunDomainError(w, err)
		return
	}
	builderSvc, err := h.buildersControlPlane()
	if err != nil {
		writeInternalError(w, http.StatusInternalServerError, "BUILDERS_INIT_FAILED", err.Error())
		return
	}
	if err := builderSvc.ReleaseByRun(r.Context(), r.PathValue("run_id"), "run failed"); err != nil {
		writeBuilderDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ack": true, "status": run.Status})
}

func (h *Handler) handleBuildRunCancel(w http.ResponseWriter, r *http.Request) {
	var req cancelBuildRunRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeInternalError(w, http.StatusBadRequest, "VALIDATION_INVALID_JSON", err.Error())
		return
	}
	runsSvc, err := h.buildRunsControlPlane()
	if err != nil {
		writeInternalError(w, http.StatusInternalServerError, "RUNS_INIT_FAILED", err.Error())
		return
	}
	run, err := runsSvc.Cancel(r.Context(), r.PathValue("run_id"), req.Reason)
	if err != nil {
		writeRunDomainError(w, err)
		return
	}
	builderSvc, err := h.buildersControlPlane()
	if err != nil {
		writeInternalError(w, http.StatusInternalServerError, "BUILDERS_INIT_FAILED", err.Error())
		return
	}
	if err := builderSvc.ReleaseByRun(r.Context(), r.PathValue("run_id"), "run cancelled"); err != nil {
		writeBuilderDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ack": true, "status": run.Status})
}

func (h *Handler) handleIndexArtifacts(w http.ResponseWriter, r *http.Request) {
	var req indexArtifactsRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeInternalError(w, http.StatusBadRequest, "VALIDATION_INVALID_JSON", err.Error())
		return
	}
	runsSvc, err := h.buildRunsControlPlane()
	if err != nil {
		writeInternalError(w, http.StatusInternalServerError, "RUNS_INIT_FAILED", err.Error())
		return
	}
	run, err := runsSvc.IndexArtifacts(r.Context(), req.RunID, req.Manifest)
	if err != nil {
		writeRunDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ack": true, "artifact_manifest_path": run.ArtifactManifestPath})
}

func (h *Handler) handleIndexMetrics(w http.ResponseWriter, r *http.Request) {
	var req indexMetricsRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeInternalError(w, http.StatusBadRequest, "VALIDATION_INVALID_JSON", err.Error())
		return
	}
	runsSvc, err := h.buildRunsControlPlane()
	if err != nil {
		writeInternalError(w, http.StatusInternalServerError, "RUNS_INIT_FAILED", err.Error())
		return
	}
	run, err := runsSvc.IndexMetrics(r.Context(), req.RunID, req.Metrics)
	if err != nil {
		writeRunDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ack": true, "metrics_path": run.MetricsPath})
}

type runRequirementResult struct {
	RunID    string
	WorkerID string
	Status   string
}

type compileRequestInput struct {
	RequirementText   string
	RequirementSource string
	Title             string
	JobID             string
	PRDID             string
	TemplateID        string
	RealChecks        bool
	ExecutorImage     string
}

func (h *Handler) loadPreparedPRD(prdID, prdVersion string) (appprepare.PRD, error) {
	bundle, err := h.findPreparedBundleByPRD(prdID, prdVersion)
	if err != nil {
		return appprepare.PRD{}, err
	}
	return bundle.PRD, nil
}

func (h *Handler) findPreparedBundleByPRD(prdID, prdVersion string) (preparedBundleRecord, error) {
	workspace, err := h.appFactoryWorkspacePath()
	if err != nil {
		return preparedBundleRecord{}, fmt.Errorf("init workspace: %w", err)
	}
	pattern := filepath.Join(workspace, "appfactory", "jobs", "*", "prepare", "PRD.json")
	paths, err := filepath.Glob(pattern)
	if err != nil {
		return preparedBundleRecord{}, fmt.Errorf("glob prepared prds: %w", err)
	}
	for _, path := range paths {
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return preparedBundleRecord{}, fmt.Errorf("read prepared prd: %w", readErr)
		}
		var prd appprepare.PRD
		if unmarshalErr := json.Unmarshal(data, &prd); unmarshalErr != nil {
			return preparedBundleRecord{}, fmt.Errorf("decode prepared prd: %w", unmarshalErr)
		}
		if prd.ID != strings.TrimSpace(prdID) {
			continue
		}
		if strings.TrimSpace(prdVersion) != "" && prd.Version != strings.TrimSpace(prdVersion) {
			continue
		}
		prepareDir := filepath.Dir(path)
		builderInputPath := filepath.Join(prepareDir, "builder-input.json")
		builderInputData, readErr := os.ReadFile(builderInputPath)
		if readErr != nil {
			return preparedBundleRecord{}, fmt.Errorf("read prepared builder input: %w", readErr)
		}
		var builderInput appruns.BuildInput
		if unmarshalErr := json.Unmarshal(builderInputData, &builderInput); unmarshalErr != nil {
			return preparedBundleRecord{}, fmt.Errorf("decode prepared builder input: %w", unmarshalErr)
		}
		jobID := filepath.Base(filepath.Dir(prepareDir))
		if strings.TrimSpace(builderInput.JobID) != "" {
			jobID = strings.TrimSpace(builderInput.JobID)
		}
		return preparedBundleRecord{JobID: jobID, PrepareDir: prepareDir, PRD: prd, BuilderInput: builderInput}, nil
	}
	return preparedBundleRecord{}, fmt.Errorf("prd %q version %q not found", strings.TrimSpace(prdID), strings.TrimSpace(prdVersion))
}

func (h *Handler) loadPreparedBundleByJobID(jobID string) (preparedBundleRecord, error) {
	workspace, err := h.appFactoryWorkspacePath()
	if err != nil {
		return preparedBundleRecord{}, fmt.Errorf("init workspace: %w", err)
	}
	prepareDir := filepath.Join(workspace, "appfactory", "jobs", strings.TrimSpace(jobID), "prepare")
	prdPath := filepath.Join(prepareDir, "PRD.json")
	prdData, err := os.ReadFile(prdPath)
	if err != nil {
		if os.IsNotExist(err) {
			return preparedBundleRecord{}, fmt.Errorf("job %q not found", strings.TrimSpace(jobID))
		}
		return preparedBundleRecord{}, fmt.Errorf("read prepared prd: %w", err)
	}
	var prd appprepare.PRD
	if err := json.Unmarshal(prdData, &prd); err != nil {
		return preparedBundleRecord{}, fmt.Errorf("decode prepared prd: %w", err)
	}
	builderInputData, err := os.ReadFile(filepath.Join(prepareDir, "builder-input.json"))
	if err != nil {
		return preparedBundleRecord{}, fmt.Errorf("read prepared builder input: %w", err)
	}
	var builderInput appruns.BuildInput
	if err := json.Unmarshal(builderInputData, &builderInput); err != nil {
		return preparedBundleRecord{}, fmt.Errorf("decode prepared builder input: %w", err)
	}
	return preparedBundleRecord{JobID: strings.TrimSpace(jobID), PrepareDir: prepareDir, PRD: prd, BuilderInput: builderInput}, nil
}

func (h *Handler) createPublicJob(bundle preparedBundleRecord) (publicJobRecord, error) {
	record, err := h.buildPublicJobRecord(bundle.JobID)
	if err != nil {
		return publicJobRecord{}, err
	}
	if err := h.persistPublicJobRecord(record); err != nil {
		return publicJobRecord{}, err
	}
	return record, nil
}

func (h *Handler) loadPersistedPublicJobRecord(workspace, jobID string) (*publicJobRecord, error) {
	jobPath := filepath.Join(workspace, "appfactory", "jobs", jobID, "job.json")
	data, err := os.ReadFile(jobPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read job record: %w", err)
	}
	var record publicJobRecord
	if err := json.Unmarshal(data, &record); err != nil {
		return nil, fmt.Errorf("decode job record: %w", err)
	}
	return &record, nil
}

func (h *Handler) persistPublicJobRecord(record publicJobRecord) error {
	workspace, err := h.appFactoryWorkspacePath()
	if err != nil {
		return fmt.Errorf("init workspace: %w", err)
	}
	jobPath := filepath.Join(workspace, "appfactory", "jobs", record.JobID, "job.json")
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal job record: %w", err)
	}
	data = append(data, '\n')
	if err := fileutil.WriteFileAtomic(jobPath, data, 0o644); err != nil {
		return fmt.Errorf("write job record: %w", err)
	}
	return nil
}

func (h *Handler) buildPublicJobRecord(jobID string) (publicJobRecord, error) {
	bundle, err := h.loadPreparedBundleByJobID(jobID)
	if err != nil {
		return publicJobRecord{}, err
	}
	workspace, err := h.appFactoryWorkspacePath()
	if err != nil {
		return publicJobRecord{}, fmt.Errorf("init workspace: %w", err)
	}
	persistedRecord, err := h.loadPersistedPublicJobRecord(workspace, bundle.JobID)
	if err != nil {
		return publicJobRecord{}, err
	}
	latestRun, err := h.loadLatestRunForJob(workspace, bundle.JobID)
	if err != nil {
		return publicJobRecord{}, err
	}
	executionRecord, err := h.loadPublicJobExecutionRecord(workspace, bundle.JobID)
	if err != nil {
		return publicJobRecord{}, err
	}
	prdApprovalPath := filepath.Join(bundle.PrepareDir, appruns.PRDApprovalFileName)
	templateApprovalPath := filepath.Join(bundle.PrepareDir, appruns.TemplateApprovalFileName)
	prdApprovalStatus, _ := readApprovalStatus(prdApprovalPath)
	templateApprovalStatus, _ := readApprovalStatus(templateApprovalPath)
	status, phase := derivePublicJobStatus(prdApprovalStatus, templateApprovalStatus, latestRun)
	commandProfile := appruns.CommandProfile{}
	if len(bundle.BuilderInput.CommandProfile) > 0 {
		_ = json.Unmarshal(bundle.BuilderInput.CommandProfile, &commandProfile)
	}
	createdAt := bundle.PRD.CreatedAt
	if strings.TrimSpace(createdAt) == "" {
		createdAt = time.Now().UTC().Format(time.RFC3339)
	}
	updatedAt := createdAt
	startedAt := ""
	finishedAt := ""
	builderOutputPath := ""
	artifacts := []string{}
	logs := &publicJobLogs{
		SummaryPath: filepath.ToSlash(filepath.Join("jobs", bundle.JobID, "prepare", "template-fit-report.md")),
	}
	var failureContext *publicJobFailureContext
	var resumeContext *publicJobResumeContext
	var deliveryContext *publicJobDeliveryContext
	budgets := publicJobBudgets{
		IterationBudget: bundle.BuilderInput.IterationBudget,
		TokenBudget:     bundle.BuilderInput.TokenBudget,
	}
	if latestRun != nil {
		updatedAt = latestRun.UpdatedAt.UTC().Format(time.RFC3339)
		startedAt = latestRun.StartedAt.UTC().Format(time.RFC3339)
		if !latestRun.FinishedAt.IsZero() {
			finishedAt = latestRun.FinishedAt.UTC().Format(time.RFC3339)
		}
		builderOutputPath = latestRun.OutputPath
		if latestRun.ArtifactManifestPath != "" {
			artifacts = append(artifacts, latestRun.ArtifactManifestPath)
		}
		logs.EventLogPath = latestRun.EventsPath
		if latestRun.LogPath != "" {
			logs.SummaryPath = latestRun.LogPath
		}
		budgets.ElapsedIterations = latestRun.IterationCount
		budgets.ConsumedTokens = latestRun.TotalTokens
		if latestRun.Status == appruns.StatusFailed || latestRun.Status == appruns.StatusCancelled {
			failureContext = &publicJobFailureContext{
				LastErrorSummary: latestRun.FailureSummary,
				Retryable:        latestRun.Status == appruns.StatusFailed,
			}
			if len(latestRun.FailureSignatures) > 0 {
				failureContext.FailureSignature = latestRun.FailureSignatures[0]
			}
		}
		resumeContext, err = h.derivePublicJobResumeContext(workspace, bundle.JobID, *latestRun)
		if err != nil {
			return publicJobRecord{}, err
		}
	}
	if overrideStatus, overridePhase, ok := derivePublicJobExecutionStatusOverride(latestRun, executionRecord); ok {
		status = overrideStatus
		phase = overridePhase
		updatedAt = latestPublicJobTimestamp(updatedAt, executionRecord.RequestedAt, executionRecord.StartedAt, executionRecord.FinishedAt)
		startedAt = strings.TrimSpace(executionRecord.StartedAt)
		finishedAt = strings.TrimSpace(executionRecord.FinishedAt)
		failureContext = publicJobFailureContextFromExecutionRecord(executionRecord)
		resumeContext = nil
	}
	delivery, err := loadDeliveryRecord(workspace, bundle.JobID)
	if err != nil {
		return publicJobRecord{}, err
	}
	if delivery != nil {
		deliveryContext = publicJobDeliveryContextFromRecord(bundle.JobID, *delivery)
		artifacts = mergePublicArtifactPaths(artifacts, []string{deliveryRecordPublicPath(bundle.JobID)})
		if delivery.RecordedAt > updatedAt {
			updatedAt = delivery.RecordedAt
		}
	}
	if persistedRecord != nil {
		artifacts = mergePublicArtifactPaths(artifacts, persistedRecord.Artifacts)
		if deliveryContext == nil {
			deliveryContext = persistedRecord.DeliveryContext
		}
	}
	record := publicJobRecord{
		SchemaVersion:     "0.1.0",
		JobID:             bundle.JobID,
		PRDID:             bundle.PRD.ID,
		PRDVersion:        bundle.PRD.Version,
		TemplateID:        bundle.BuilderInput.TemplateID,
		Status:            status,
		Phase:             phase,
		WorkspacePath:     filepath.ToSlash(filepath.Join("jobs", bundle.JobID, "workspace")),
		ArtifactDir:       filepath.ToSlash(filepath.Join("jobs", bundle.JobID, "artifacts")),
		BuilderInputPath:  filepath.ToSlash(filepath.Join("jobs", bundle.JobID, "prepare", "builder-input.json")),
		BuilderOutputPath: builderOutputPath,
		Budgets:           budgets,
		Runtime: &publicJobRuntime{
			Builder:       "other",
			WorkerPool:    "local-file-store",
			NetworkPolicy: commandProfile.NetworkPolicy,
		},
		Logs:            logs,
		Artifacts:       artifacts,
		HumanApprovals:  []string{filepath.ToSlash(filepath.Join("jobs", bundle.JobID, "prepare", appruns.PRDApprovalFileName)), filepath.ToSlash(filepath.Join("jobs", bundle.JobID, "prepare", appruns.TemplateApprovalFileName))},
		FailureContext:  failureContext,
		ResumeContext:   resumeContext,
		DeliveryContext: deliveryContext,
		CreatedAt:       createdAt,
		UpdatedAt:       updatedAt,
		StartedAt:       startedAt,
		FinishedAt:      finishedAt,
	}
	if latestRun == nil && persistedRecord != nil && persistedRecord.Status == "cancelled" {
		record.Status = persistedRecord.Status
		record.Phase = persistedRecord.Phase
		record.UpdatedAt = persistedRecord.UpdatedAt
		record.FinishedAt = persistedRecord.FinishedAt
		record.FailureContext = persistedRecord.FailureContext
		record.ResumeContext = persistedRecord.ResumeContext
		record.DeliveryContext = persistedRecord.DeliveryContext
	}
	return record, nil
}

func (h *Handler) loadPublicJobs(statusFilter string) ([]publicJobRecord, error) {
	workspace, err := h.appFactoryWorkspacePath()
	if err != nil {
		return nil, fmt.Errorf("init workspace: %w", err)
	}
	jobPaths, err := filepath.Glob(filepath.Join(workspace, "appfactory", "jobs", "*", "prepare", "builder-input.json"))
	if err != nil {
		return nil, fmt.Errorf("glob job records: %w", err)
	}
	items := make([]publicJobRecord, 0, len(jobPaths))
	wantedStatus := strings.TrimSpace(statusFilter)
	for _, path := range jobPaths {
		jobID := filepath.Base(filepath.Dir(filepath.Dir(path)))
		record, err := h.buildPublicJobRecord(jobID)
		if err != nil {
			return nil, err
		}
		if wantedStatus != "" && record.Status != wantedStatus {
			continue
		}
		items = append(items, record)
	}
	sort.Slice(items, func(i, j int) bool {
		left := strings.TrimSpace(items[i].UpdatedAt)
		right := strings.TrimSpace(items[j].UpdatedAt)
		if left == right {
			return items[i].JobID < items[j].JobID
		}
		return left > right
	})
	return items, nil
}

func (h *Handler) derivePublicJobResumeContext(workspace, jobID string, latestRun appruns.RunRecord) (*publicJobResumeContext, error) {
	buildOutput, err := loadRunBuildOutput(workspace, latestRun)
	if err != nil {
		return nil, err
	}
	resumeContext, explicitResumeAllowed, err := decodePublicJobResumeContext(buildOutput)
	if err != nil {
		return nil, err
	}
	if resumeContext == nil {
		resumeContext, explicitResumeAllowed = synthesizePublicJobResumeContextFromRun(jobID, latestRun)
	}
	if latestRun.Status != appruns.StatusFailed {
		return nil, nil
	}
	if resumeContext == nil {
		resumeContext = &publicJobResumeContext{}
	}
	if !explicitResumeAllowed {
		resumeContext.ResumeAllowed = true
	}
	if strings.TrimSpace(resumeContext.FailureCategory) == "" {
		resumeContext.FailureCategory = derivePublicJobFailureCategory(latestRun)
	}
	if strings.TrimSpace(resumeContext.FailureDomain) == "" {
		resumeContext.FailureDomain = derivePublicJobFailureDomain(resumeContext.FailureCategory)
	}
	if strings.TrimSpace(resumeContext.PreservedWorkspacePath) == "" {
		resumeContext.PreservedWorkspacePath = filepath.ToSlash(filepath.Join("jobs", jobID, "workspace"))
	}
	if !resumeContext.RequiresPreservedWorkspace {
		resumeContext.RequiresPreservedWorkspace = publicJobResumePathUsesPreservedWorkspace(jobID, resumeContext.PreservedWorkspacePath)
	}
	if strings.TrimSpace(resumeContext.RecommendedResumeMode) == "" {
		resumeContext.RecommendedResumeMode = derivePublicJobRecommendedResumeMode(latestRun, jobID, resumeContext)
	}
	if strings.TrimSpace(resumeContext.NextAction) == "" {
		resumeContext.NextAction = publicJobResumeNextAction(resumeContext)
	}
	return resumeContext, nil
}

func synthesizePublicJobResumeContextFromRun(jobID string, run appruns.RunRecord) (*publicJobResumeContext, bool) {
	if run.RepairContext == nil && run.RoundState == nil {
		return nil, false
	}
	state := run.RoundState
	if state == nil && run.RepairContext != nil {
		state = run.RepairContext.State
	}
	resumeContext := &publicJobResumeContext{}
	explicitResumeAllowed := false
	if state != nil {
		resumeContext.ResumeAllowed = state.ResumeAllowed
		explicitResumeAllowed = true
	}
	if run.RepairContext != nil {
		resumeContext.RequiresHumanConfirmation = run.RepairContext.RequiresHumanReview
		if state == nil && run.RepairContext.PreserveWorkspace {
			resumeContext.RequiresPreservedWorkspace = false
		}
	}
	if signature := firstPublicFailureSignature(run.RepairContext, run.FailureSignatures); signature != "" {
		resumeContext.FailureCategory = signature
	}
	if mode := publicJobResumeModeFromRoundState(state); mode != "" {
		resumeContext.RecommendedResumeMode = mode
	}
	return resumeContext, explicitResumeAllowed
}

func firstPublicFailureSignature(context *appruns.RepairContext, runSignatures []string) string {
	if context != nil {
		for _, signature := range context.FailureSignatures {
			if strings.TrimSpace(signature) != "" {
				return strings.TrimSpace(signature)
			}
		}
	}
	for _, signature := range runSignatures {
		if strings.TrimSpace(signature) != "" {
			return strings.TrimSpace(signature)
		}
	}
	return ""
}

func publicJobResumeModeFromRoundState(state *appruns.RoundState) string {
	if state == nil || !state.ResumeAllowed {
		return ""
	}
	switch state.NextAction {
	case appruns.ControlActionResume:
		return "resume_from_failure"
	case appruns.ControlActionRetry:
		return "retry_failed_run"
	default:
		return ""
	}
}

func publicJobFailureHasSignature(run appruns.RunRecord, signature string) bool {
	target := strings.TrimSpace(signature)
	if target == "" {
		return false
	}
	for _, item := range run.FailureSignatures {
		if strings.TrimSpace(item) == target {
			return true
		}
	}
	return false
}

func loadRunBuildOutput(workspace string, run appruns.RunRecord) (*appruns.BuildOutput, error) {
	paths := []string{}
	if relPath := strings.TrimSpace(run.OutputPath); relPath != "" {
		paths = append(paths, filepath.Join(workspace, "appfactory", filepath.FromSlash(relPath)))
	}
	defaultPath := filepath.Join(workspace, "appfactory", "jobs", run.JobID, "runs", run.RunID, "builder-output.json")
	if len(paths) == 0 || paths[0] != defaultPath {
		paths = append(paths, defaultPath)
	}
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, fmt.Errorf("read build output: %w", err)
		}
		var output appruns.BuildOutput
		if err := json.Unmarshal(data, &output); err != nil {
			return nil, fmt.Errorf("decode build output: %w", err)
		}
		return &output, nil
	}
	return nil, nil
}

func decodePublicJobResumeContext(output *appruns.BuildOutput) (*publicJobResumeContext, bool, error) {
	if output == nil || strings.TrimSpace(string(output.ResumeContext)) == "" {
		return nil, false, nil
	}
	type rawResumeContext struct {
		ResumeAllowed              *bool  `json:"resume_allowed,omitempty"`
		FailureDomain              string `json:"failure_domain,omitempty"`
		FailureCategory            string `json:"failure_category,omitempty"`
		RecommendedResumeMode      string `json:"recommended_resume_mode,omitempty"`
		RequiresHumanConfirmation  *bool  `json:"requires_human_confirmation,omitempty"`
		RequiresPreservedWorkspace *bool  `json:"requires_preserved_workspace,omitempty"`
		PreservedWorkspacePath     string `json:"preserved_workspace_path,omitempty"`
		NextAction                 string `json:"next_action,omitempty"`
	}
	var raw rawResumeContext
	if err := json.Unmarshal(output.ResumeContext, &raw); err != nil {
		return nil, false, fmt.Errorf("decode resume context: %w", err)
	}
	resumeContext := &publicJobResumeContext{
		FailureDomain:          strings.TrimSpace(raw.FailureDomain),
		FailureCategory:        strings.TrimSpace(raw.FailureCategory),
		RecommendedResumeMode:  strings.TrimSpace(raw.RecommendedResumeMode),
		PreservedWorkspacePath: raw.PreservedWorkspacePath,
		NextAction:             raw.NextAction,
	}
	if raw.RequiresHumanConfirmation != nil {
		resumeContext.RequiresHumanConfirmation = *raw.RequiresHumanConfirmation
	}
	if raw.RequiresPreservedWorkspace != nil {
		resumeContext.RequiresPreservedWorkspace = *raw.RequiresPreservedWorkspace
	}
	if raw.ResumeAllowed != nil {
		resumeContext.ResumeAllowed = *raw.ResumeAllowed
		return resumeContext, true, nil
	}
	return resumeContext, false, nil
}

func derivePublicJobFailureCategory(run appruns.RunRecord) string {
	if publicJobFailureHasSignature(run, "orchestrator_dispatcher_lost") {
		return "execution_interrupted"
	}
	if publicJobFailureHasSignature(run, "runtime_crash") {
		return "builder_runtime_failure"
	}
	if len(run.FailureSignatures) > 0 {
		if signature := strings.TrimSpace(run.FailureSignatures[0]); signature != "" {
			return signature
		}
	}
	return "failed_run"
}

func derivePublicJobFailureDomain(category string) string {
	switch normalized := strings.TrimSpace(category); {
	case normalized == "":
		return ""
	case strings.HasPrefix(normalized, "skill_"):
		return "skill"
	case strings.HasPrefix(normalized, "legacy-builder_"):
		return "skill"
	case strings.HasPrefix(normalized, "profile_check_failed:"):
		return "profile"
	case strings.HasPrefix(normalized, "environment_check_failed:"):
		return "environment"
	case normalized == "workspace_patch_apply_failed":
		return "executor"
	case normalized == "execution_interrupted":
		return "environment"
	case normalized == "builder_runtime_failure":
		return "environment"
	case normalized == "runner_exit_nonzero":
		return "environment"
	case strings.HasPrefix(normalized, "check_failed:check-counter-demo-removed"):
		return "profile"
	case strings.HasPrefix(normalized, "check_failed:check-entry-form-wiring"):
		return "profile"
	case strings.HasPrefix(normalized, "check_failed:check-local-persistence-wiring"):
		return "profile"
	case strings.HasPrefix(normalized, "check_failed:check-flutter-"):
		return "environment"
	default:
		return "executor"
	}
}

func publicJobResumePathUsesPreservedWorkspace(jobID, relPath string) bool {
	cleanPath := strings.Trim(filepath.ToSlash(strings.TrimSpace(relPath)), "/")
	if cleanPath == "" {
		return false
	}
	prefix := filepath.ToSlash(filepath.Join("jobs", jobID, "snapshots", "preserved"))
	return cleanPath == prefix || strings.HasPrefix(cleanPath, prefix+"/")
}

func derivePublicJobRecommendedResumeMode(run appruns.RunRecord, jobID string, resumeContext *publicJobResumeContext) string {
	if publicJobFailureHasSignature(run, "orchestrator_dispatcher_lost") {
		return "resume_interrupted_job"
	}
	if resumeContext != nil && (resumeContext.RequiresPreservedWorkspace || publicJobResumePathUsesPreservedWorkspace(jobID, resumeContext.PreservedWorkspacePath)) {
		return "resume_from_failure"
	}
	return "retry_failed_run"
}

func publicJobResumeNextAction(resumeContext *publicJobResumeContext) string {
	if resumeContext == nil {
		return "inspect_failure_and_retry_later"
	}
	if resumeContext.RequiresHumanConfirmation {
		return "inspect_failure_and_confirm_resume"
	}
	if mode := strings.TrimSpace(resumeContext.RecommendedResumeMode); mode != "" {
		return mode
	}
	if nextAction := strings.TrimSpace(resumeContext.NextAction); nextAction != "" {
		return nextAction
	}
	if resumeContext.ResumeAllowed {
		return "resume_failed_job"
	}
	return "inspect_failure_and_retry_later"
}

func applyPublicJobResumeMetadataToEvent(event *publicJobEvent, resumeContext *publicJobResumeContext) {
	if event == nil || resumeContext == nil {
		return
	}
	event.FailureDomain = strings.TrimSpace(resumeContext.FailureDomain)
	event.FailureCategory = strings.TrimSpace(resumeContext.FailureCategory)
	event.RecommendedResumeMode = strings.TrimSpace(resumeContext.RecommendedResumeMode)
	event.RequiresHumanConfirmation = resumeContext.RequiresHumanConfirmation
	event.RequiresPreservedWorkspace = resumeContext.RequiresPreservedWorkspace
}

func applyPublicJobResumeMetadataToNotification(item *publicNotification, resumeContext *publicJobResumeContext) {
	if item == nil || resumeContext == nil {
		return
	}
	item.FailureDomain = strings.TrimSpace(resumeContext.FailureDomain)
	item.FailureCategory = strings.TrimSpace(resumeContext.FailureCategory)
	item.RecommendedResumeMode = strings.TrimSpace(resumeContext.RecommendedResumeMode)
	item.RequiresHumanConfirmation = resumeContext.RequiresHumanConfirmation
	item.RequiresPreservedWorkspace = resumeContext.RequiresPreservedWorkspace
}

func applyPublicJobDeliveryMetadataToEvent(event *publicJobEvent, delivery *publicJobDeliveryContext) {
	if event == nil || delivery == nil {
		return
	}
	event.DeliveryStatus = strings.TrimSpace(delivery.Status)
	event.DeliveryRecordPath = strings.TrimSpace(delivery.DeliveryRecordPath)
	event.ReleaseChannel = strings.TrimSpace(delivery.ReleaseChannel)
	event.RolloutPercent = delivery.RolloutPercent
}

func applyPublicJobDeliveryMetadataToNotification(item *publicNotification, delivery *publicJobDeliveryContext) {
	if item == nil || delivery == nil {
		return
	}
	item.DeliveryStatus = strings.TrimSpace(delivery.Status)
	item.DeliveryRecordPath = strings.TrimSpace(delivery.DeliveryRecordPath)
	item.ReleaseChannel = strings.TrimSpace(delivery.ReleaseChannel)
	item.RolloutPercent = delivery.RolloutPercent
}

func (h *Handler) loadLatestRunForJob(workspace, jobID string) (*appruns.RunRecord, error) {
	paths, err := filepath.Glob(filepath.Join(workspace, "appfactory", "jobs", jobID, "runs", "*", "run.json"))
	if err != nil {
		return nil, fmt.Errorf("glob job runs: %w", err)
	}
	if len(paths) == 0 {
		return nil, nil
	}
	var latest *appruns.RunRecord
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read run record: %w", err)
		}
		var record appruns.RunRecord
		if err := json.Unmarshal(data, &record); err != nil {
			return nil, fmt.Errorf("decode run record: %w", err)
		}
		if latest == nil || record.UpdatedAt.After(latest.UpdatedAt) {
			copied := record
			latest = &copied
		}
	}
	return latest, nil
}

func derivePublicJobStatus(prdApprovalStatus, templateApprovalStatus string, latestRun *appruns.RunRecord) (string, string) {
	if latestRun != nil {
		switch latestRun.Status {
		case appruns.StatusRunning:
			return "running_builder", "builder"
		case appruns.StatusCompleted:
			return "completed", "terminal"
		case appruns.StatusFailed:
			return "failed", "terminal"
		case appruns.StatusCancelled:
			return "cancelled", "terminal"
		}
	}
	if prdApprovalStatus == appruns.ApprovalStatusRejected {
		return "failed", "terminal"
	}
	if templateApprovalStatus == appruns.ApprovalStatusRejected {
		return "failed", "terminal"
	}
	if prdApprovalStatus != appruns.ApprovalStatusApproved {
		return "awaiting_prd_approval", "approval"
	}
	if templateApprovalStatus != appruns.ApprovalStatusApproved {
		return "awaiting_template_approval", "approval"
	}
	return "queued", "builder"
}

func derivePublicJobExecutionStatusOverride(latestRun *appruns.RunRecord, executionRecord *publicJobExecutionRecord) (string, string, bool) {
	if latestRun == nil || executionRecord == nil {
		return "", "", false
	}
	if strings.TrimSpace(latestRun.RunID) == "" || strings.TrimSpace(executionRecord.RunID) == "" {
		return "", "", false
	}
	if strings.TrimSpace(latestRun.RunID) != strings.TrimSpace(executionRecord.RunID) {
		return "", "", false
	}
	if isTerminalRunStatus(latestRun.Status) {
		return "", "", false
	}
	status := strings.TrimSpace(executionRecord.Status)
	if _, ok := publicJobExecutionTerminalStatuses[status]; !ok {
		return "", "", false
	}
	return status, "terminal", true
}

func isTerminalRunStatus(status appruns.Status) bool {
	switch status {
	case appruns.StatusCompleted, appruns.StatusFailed, appruns.StatusCancelled:
		return true
	default:
		return false
	}
}

func publicJobFailureContextFromExecutionRecord(executionRecord *publicJobExecutionRecord) *publicJobFailureContext {
	if executionRecord == nil {
		return nil
	}
	status := strings.TrimSpace(executionRecord.Status)
	if status != "failed" && status != "cancelled" {
		return nil
	}
	context := &publicJobFailureContext{
		LastErrorSummary: publicJobExecutionFailureSummary(executionRecord),
		Retryable:        status == "failed",
	}
	if signature := publicJobExecutionFailureSignature(executionRecord); signature != "" {
		context.FailureSignature = signature
	}
	return context
}

func publicJobExecutionFailureSummary(executionRecord *publicJobExecutionRecord) string {
	if executionRecord == nil {
		return ""
	}
	if summary := strings.TrimSpace(executionRecord.LastError); summary != "" {
		return summary
	}
	switch strings.TrimSpace(executionRecord.Status) {
	case "cancelled":
		return "orchestrator execution was cancelled before run status could be finalized"
	case "failed":
		return "orchestrator execution failed before run status could be finalized"
	default:
		return ""
	}
}

func publicJobExecutionFailureSignature(executionRecord *publicJobExecutionRecord) string {
	if executionRecord == nil {
		return ""
	}
	if len(executionRecord.Attempts) == 0 {
		switch strings.TrimSpace(executionRecord.Status) {
		case "failed":
			return "execution_failed"
		case "cancelled":
			return "execution_cancelled"
		default:
			return ""
		}
	}
	lastAttempt := executionRecord.Attempts[len(executionRecord.Attempts)-1]
	switch strings.TrimSpace(lastAttempt.ReleaseReason) {
	case "dispatcher_lost":
		return "orchestrator_dispatcher_lost"
	case "recovery_failed":
		return "execution_recovery_failed"
	case "failed":
		return "execution_failed"
	case "cancelled":
		return "execution_cancelled"
	default:
		return ""
	}
}

func latestPublicJobTimestamp(current string, candidates ...string) string {
	latestValue := strings.TrimSpace(current)
	latestTime, latestOK := parsePublicJobTimestamp(latestValue)
	for _, candidate := range candidates {
		trimmed := strings.TrimSpace(candidate)
		candidateTime, candidateOK := parsePublicJobTimestamp(trimmed)
		if !candidateOK {
			continue
		}
		if !latestOK || candidateTime.After(latestTime) {
			latestValue = trimmed
			latestTime = candidateTime
			latestOK = true
		}
	}
	return latestValue
}

func parsePublicJobTimestamp(value string) (time.Time, bool) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return time.Time{}, false
	}
	if parsed, err := time.Parse(time.RFC3339Nano, trimmed); err == nil {
		return parsed, true
	}
	if parsed, err := time.Parse(time.RFC3339, trimmed); err == nil {
		return parsed, true
	}
	return time.Time{}, false
}

func readApprovalStatus(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	var record appruns.ApprovalRecord
	if err := json.Unmarshal(data, &record); err != nil {
		return "", err
	}
	return strings.TrimSpace(record.Status), nil
}

func (h *Handler) loadJobArtifacts(jobID string) (appruns.ArtifactManifest, error) {
	record, err := h.buildPublicJobRecord(jobID)
	if err != nil {
		return appruns.ArtifactManifest{}, err
	}
	workspace, err := h.appFactoryWorkspacePath()
	if err != nil {
		return appruns.ArtifactManifest{}, fmt.Errorf("init workspace: %w", err)
	}
	latestRun, err := h.loadLatestRunForJob(workspace, jobID)
	if err != nil {
		return appruns.ArtifactManifest{}, err
	}
	if latestRun == nil {
		createdAt, parseErr := time.Parse(time.RFC3339, record.CreatedAt)
		if parseErr != nil {
			createdAt = time.Now().UTC()
		}
		return appruns.ArtifactManifest{SchemaVersion: "0.1.0", JobID: jobID, GeneratedAt: createdAt, Items: []appruns.ArtifactItem{}}, nil
	}
	candidates := []string{}
	if latestRun.ArtifactManifestPath != "" {
		candidates = append(candidates, filepath.Join(workspace, "appfactory", filepath.FromSlash(latestRun.ArtifactManifestPath)))
	}
	candidates = append(candidates, filepath.Join(workspace, "appfactory", "jobs", jobID, "runs", latestRun.RunID, "artifact-manifest.json"))
	for _, path := range candidates {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var manifest appruns.ArtifactManifest
		if err := json.Unmarshal(data, &manifest); err != nil {
			return appruns.ArtifactManifest{}, fmt.Errorf("decode artifact manifest: %w", err)
		}
		return manifest, nil
	}
	return appruns.ArtifactManifest{SchemaVersion: "0.1.0", JobID: jobID, GeneratedAt: time.Now().UTC(), Items: []appruns.ArtifactItem{}}, nil
}

func (h *Handler) loadJobEvents(jobID string) ([]publicJobEvent, error) {
	record, err := h.buildPublicJobRecord(jobID)
	if err != nil {
		return nil, err
	}
	items := []publicJobEvent{{At: record.CreatedAt, Type: "job_created", JobID: jobID, Status: record.Status, Summary: "job facade created"}}
	workspace, err := h.appFactoryWorkspacePath()
	if err != nil {
		return nil, fmt.Errorf("init workspace: %w", err)
	}
	for _, candidate := range []struct {
		path     string
		typeName string
		summary  string
	}{
		{path: filepath.Join(workspace, "appfactory", "jobs", jobID, "prepare", appruns.PRDApprovalFileName), typeName: "prd_approval_snapshot", summary: "prepared prd approval snapshot"},
		{path: filepath.Join(workspace, "appfactory", "jobs", jobID, "prepare", appruns.TemplateApprovalFileName), typeName: "template_approval_snapshot", summary: "prepared template approval snapshot"},
	} {
		data, err := os.ReadFile(candidate.path)
		if err != nil {
			continue
		}
		var approval appruns.ApprovalRecord
		if err := json.Unmarshal(data, &approval); err != nil {
			return nil, fmt.Errorf("decode approval record: %w", err)
		}
		items = append(items, publicJobEvent{At: approval.CreatedAt, Type: candidate.typeName, JobID: jobID, Status: approval.Status, Summary: candidate.summary})
	}
	items = append(items, syntheticPublicJobEvents(record)...)
	items = append(items, syntheticPublicJobDeliveryEvents(record)...)
	executionRecord, err := h.loadPublicJobExecutionRecord(workspace, jobID)
	if err != nil {
		return nil, err
	}
	items = append(items, syntheticPublicJobExecutionEvents(executionRecord)...)
	eventPaths, err := filepath.Glob(filepath.Join(workspace, "appfactory", "jobs", jobID, "runs", "*", "events.jsonl"))
	if err != nil {
		return nil, fmt.Errorf("glob job events: %w", err)
	}
	for _, path := range eventPaths {
		file, err := os.Open(path)
		if err != nil {
			return nil, fmt.Errorf("open job events: %w", err)
		}
		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" {
				continue
			}
			var event publicJobEvent
			if err := json.Unmarshal([]byte(line), &event); err != nil {
				_ = file.Close()
				return nil, fmt.Errorf("decode job event: %w", err)
			}
			items = append(items, event)
		}
		if err := scanner.Err(); err != nil {
			_ = file.Close()
			return nil, fmt.Errorf("scan job events: %w", err)
		}
		_ = file.Close()
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].At < items[j].At
	})
	return items, nil
}

func (h *Handler) startPublicJob(parent context.Context, jobID string, req startJobRequest) (publicJobRecord, error) {
	bundle, err := h.loadPreparedBundleByJobID(jobID)
	if err != nil {
		return publicJobRecord{}, err
	}
	prdApprovalStatus, err := readApprovalStatus(filepath.Join(bundle.PrepareDir, appruns.PRDApprovalFileName))
	if err != nil {
		return publicJobRecord{}, fmt.Errorf("read prd approval: %w", err)
	}
	templateApprovalStatus, err := readApprovalStatus(filepath.Join(bundle.PrepareDir, appruns.TemplateApprovalFileName))
	if err != nil {
		return publicJobRecord{}, fmt.Errorf("read template approval: %w", err)
	}
	if prdApprovalStatus != appruns.ApprovalStatusApproved || templateApprovalStatus != appruns.ApprovalStatusApproved {
		return publicJobRecord{}, fmt.Errorf("job %q approvals not ready: prd=%s template=%s", jobID, prdApprovalStatus, templateApprovalStatus)
	}
	workspace, err := h.appFactoryWorkspacePath()
	if err != nil {
		return publicJobRecord{}, fmt.Errorf("init workspace: %w", err)
	}
	if executionRecord, err := h.loadPublicJobExecutionRecord(workspace, jobID); err != nil {
		return publicJobRecord{}, err
	} else if executionRecord != nil && (executionRecord.Status == "queued" || executionRecord.Status == "running") {
		return publicJobRecord{}, fmt.Errorf("job %q already has active execution %q with status %s", jobID, executionRecord.RunID, executionRecord.Status)
	}
	latestRun, err := h.loadLatestRunForJob(workspace, jobID)
	if err != nil {
		return publicJobRecord{}, err
	}
	if latestRun != nil {
		return publicJobRecord{}, fmt.Errorf("job %q already has run %q with status %s", jobID, latestRun.RunID, latestRun.Status)
	}
	builderSvc, err := h.buildersControlPlane()
	if err != nil {
		return publicJobRecord{}, fmt.Errorf("init builders service: %w", err)
	}
	runsSvc, err := h.buildRunsControlPlane()
	if err != nil {
		return publicJobRecord{}, fmt.Errorf("init runs service: %w", err)
	}
	preparedInput := bundle.BuilderInput
	preparedInput.ContextSourceDir = bundle.PrepareDir
	dispatch, err := builderSvc.Dispatch(parent, appbuilders.Requirement{JobID: jobID})
	if err != nil {
		return publicJobRecord{}, err
	}
	run, err := runsSvc.Create(parent, dispatch.Node.BuilderID, dispatch.Node.BuilderID, dispatch.Lease.LeaseID, preparedInput)
	if err != nil {
		return publicJobRecord{}, err
	}
	if err := builderSvc.BindRun(parent, dispatch.Lease.LeaseID, run.RunID); err != nil {
		return publicJobRecord{}, err
	}
	record, err := h.buildPublicJobRecord(jobID)
	if err != nil {
		return publicJobRecord{}, err
	}
	if err := h.persistPublicJobRecord(record); err != nil {
		return publicJobRecord{}, err
	}
	if err := h.enqueuePublicJobExecution(jobID, run.RunID, "start", executionTimeoutFromSeconds(req.TimeoutSeconds)); err != nil {
		return publicJobRecord{}, err
	}
	return record, nil
}

func (h *Handler) cancelPublicJob(parent context.Context, jobID string, req cancelJobRequest) (publicJobRecord, error) {
	bundle, err := h.loadPreparedBundleByJobID(jobID)
	if err != nil {
		return publicJobRecord{}, err
	}
	workspace, err := h.appFactoryWorkspacePath()
	if err != nil {
		return publicJobRecord{}, fmt.Errorf("init workspace: %w", err)
	}
	reason := strings.TrimSpace(req.Reason)
	if reason == "" {
		reason = "job cancelled"
	}
	latestRun, err := h.loadLatestRunForJob(workspace, jobID)
	if err != nil {
		return publicJobRecord{}, err
	}
	if latestRun != nil {
		switch latestRun.Status {
		case appruns.StatusCompleted, appruns.StatusFailed, appruns.StatusCancelled:
			return publicJobRecord{}, fmt.Errorf("job %q already has terminal run %q with status %s", jobID, latestRun.RunID, latestRun.Status)
		}
		runsSvc, err := h.buildRunsControlPlane()
		if err != nil {
			return publicJobRecord{}, fmt.Errorf("init runs service: %w", err)
		}
		if _, err := runsSvc.Cancel(parent, latestRun.RunID, reason); err != nil {
			return publicJobRecord{}, err
		}
		builderSvc, err := h.buildersControlPlane()
		if err != nil {
			return publicJobRecord{}, fmt.Errorf("init builders service: %w", err)
		}
		if err := builderSvc.ReleaseByRun(parent, latestRun.RunID, "run cancelled"); err != nil {
			return publicJobRecord{}, err
		}
		if err := h.cancelPublicJobExecution(jobID, reason); err != nil {
			return publicJobRecord{}, err
		}
		record, err := h.buildPublicJobRecord(jobID)
		if err != nil {
			return publicJobRecord{}, err
		}
		if req.PreserveWorkspace {
			snapshotPath, err := h.preserveJobWorkspaceSnapshot(workspace, jobID, latestRun.WorkspacePath, "cancelled")
			if err != nil {
				return publicJobRecord{}, err
			}
			if snapshotPath != "" {
				record.Artifacts = mergePublicArtifactPaths(record.Artifacts, []string{snapshotPath})
			}
		}
		if err := h.persistPublicJobRecord(record); err != nil {
			return publicJobRecord{}, err
		}
		return record, nil
	}

	record, err := h.buildPublicJobRecord(bundle.JobID)
	if err != nil {
		return publicJobRecord{}, err
	}
	if record.Status == "completed" || record.Status == "failed" || record.Status == "cancelled" {
		return publicJobRecord{}, fmt.Errorf("job %q already terminal with status %s", jobID, record.Status)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	record.Status = "cancelled"
	record.Phase = "terminal"
	record.UpdatedAt = now
	record.FinishedAt = now
	record.FailureContext = &publicJobFailureContext{
		LastErrorSummary: reason,
		Retryable:        false,
	}
	if req.PreserveWorkspace {
		snapshotPath, err := h.preserveJobWorkspaceSnapshot(workspace, jobID, record.WorkspacePath, "cancelled")
		if err != nil {
			return publicJobRecord{}, err
		}
		if snapshotPath != "" {
			record.Artifacts = mergePublicArtifactPaths(record.Artifacts, []string{snapshotPath})
		}
	}
	if err := h.persistPublicJobRecord(record); err != nil {
		return publicJobRecord{}, err
	}
	if err := h.cancelPublicJobExecution(jobID, reason); err != nil {
		return publicJobRecord{}, err
	}
	return record, nil
}

func mergePublicArtifactPaths(base, extra []string) []string {
	if len(extra) == 0 {
		return append([]string(nil), base...)
	}
	merged := append([]string(nil), base...)
	seen := make(map[string]struct{}, len(base)+len(extra))
	for _, item := range merged {
		trimmed := strings.TrimSpace(item)
		if trimmed == "" {
			continue
		}
		seen[trimmed] = struct{}{}
	}
	for _, item := range extra {
		trimmed := strings.TrimSpace(item)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		merged = append(merged, trimmed)
		seen[trimmed] = struct{}{}
	}
	return merged
}

func (h *Handler) preserveJobWorkspaceSnapshot(workspace, jobID, sourcePath, reason string) (string, error) {
	sourceAbsPath, err := resolvePublicJobWorkspacePath(workspace, jobID, sourcePath)
	if err != nil {
		return "", err
	}
	if sourceAbsPath == "" {
		return "", nil
	}
	hasEntries, err := publicJobDirHasEntries(sourceAbsPath)
	if err != nil {
		return "", err
	}
	if !hasEntries {
		return "", nil
	}
	snapshotName := sanitizeSnapshotReason(reason) + "-" + time.Now().UTC().Format("20060102T150405Z")
	snapshotRelPath := filepath.ToSlash(filepath.Join("jobs", jobID, "snapshots", "preserved", snapshotName))
	snapshotAbsPath := filepath.Join(workspace, "appfactory", filepath.FromSlash(snapshotRelPath))
	if err := os.RemoveAll(snapshotAbsPath); err != nil {
		return "", fmt.Errorf("reset preserved snapshot dir: %w", err)
	}
	if err := publicJobCopyDir(sourceAbsPath, snapshotAbsPath); err != nil {
		return "", fmt.Errorf("preserve workspace snapshot: %w", err)
	}
	return snapshotRelPath, nil
}

func resolvePublicJobWorkspacePath(workspace, jobID, pathValue string) (string, error) {
	trimmed := strings.TrimSpace(pathValue)
	if trimmed == "" {
		return "", nil
	}
	jobRoot := filepath.Join(workspace, "appfactory", "jobs", jobID)
	var resolved string
	if filepath.IsAbs(trimmed) {
		resolved = filepath.Clean(trimmed)
	} else {
		resolved = filepath.Clean(filepath.Join(workspace, "appfactory", filepath.FromSlash(trimmed)))
	}
	rel, err := filepath.Rel(jobRoot, resolved)
	if err != nil {
		return "", fmt.Errorf("resolve workspace path: %w", err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("workspace path %q escapes job %q", pathValue, jobID)
	}
	if _, err := os.Stat(resolved); err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("stat workspace path: %w", err)
	}
	return resolved, nil
}

func publicJobDirHasEntries(path string) (bool, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("read workspace dir: %w", err)
	}
	return len(entries) > 0, nil
}

func sanitizeSnapshotReason(reason string) string {
	trimmed := strings.TrimSpace(strings.ToLower(reason))
	if trimmed == "" {
		return "snapshot"
	}
	var builder strings.Builder
	lastDash := false
	for _, r := range trimmed {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			builder.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			builder.WriteByte('-')
			lastDash = true
		}
	}
	result := strings.Trim(builder.String(), "-")
	if result == "" {
		return "snapshot"
	}
	return result
}

func publicJobCopyDir(src, dst string) error {
	return filepath.WalkDir(src, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return os.MkdirAll(dst, 0o755)
		}
		target := filepath.Join(dst, rel)
		if entry.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		return fileutil.CopyFile(path, target, 0o644)
	})
}

func (h *Handler) resumePublicJob(parent context.Context, jobID string, req resumeJobRequest) (publicJobRecord, error) {
	bundle, err := h.loadPreparedBundleByJobID(jobID)
	if err != nil {
		return publicJobRecord{}, err
	}
	mode := strings.TrimSpace(req.ResumeMode)
	if mode != "" && mode != "retry_failed_run" && mode != "resume_from_failure" && mode != "resume_interrupted_job" {
		return publicJobRecord{}, fmt.Errorf("unsupported resume_mode %q", mode)
	}
	prdApprovalStatus, err := readApprovalStatus(filepath.Join(bundle.PrepareDir, appruns.PRDApprovalFileName))
	if err != nil {
		return publicJobRecord{}, fmt.Errorf("read prd approval: %w", err)
	}
	templateApprovalStatus, err := readApprovalStatus(filepath.Join(bundle.PrepareDir, appruns.TemplateApprovalFileName))
	if err != nil {
		return publicJobRecord{}, fmt.Errorf("read template approval: %w", err)
	}
	if prdApprovalStatus != appruns.ApprovalStatusApproved || templateApprovalStatus != appruns.ApprovalStatusApproved {
		return publicJobRecord{}, fmt.Errorf("job %q approvals not ready: prd=%s template=%s", jobID, prdApprovalStatus, templateApprovalStatus)
	}
	workspace, err := h.appFactoryWorkspacePath()
	if err != nil {
		return publicJobRecord{}, fmt.Errorf("init workspace: %w", err)
	}
	if executionRecord, err := h.loadPublicJobExecutionRecord(workspace, jobID); err != nil {
		return publicJobRecord{}, err
	} else if executionRecord != nil && (executionRecord.Status == "queued" || executionRecord.Status == "running") {
		return publicJobRecord{}, fmt.Errorf("job %q already has active execution %q with status %s", jobID, executionRecord.RunID, executionRecord.Status)
	}
	latestRun, err := h.loadLatestRunForJob(workspace, jobID)
	if err != nil {
		return publicJobRecord{}, err
	}
	if latestRun == nil {
		return publicJobRecord{}, fmt.Errorf("job %q has no failed run to resume", jobID)
	}
	resumeContext, err := h.derivePublicJobResumeContext(workspace, jobID, *latestRun)
	if err != nil {
		return publicJobRecord{}, err
	}
	switch latestRun.Status {
	case appruns.StatusFailed:
		if resumeContext != nil && !resumeContext.ResumeAllowed {
			nextAction := publicJobResumeNextAction(resumeContext)
			return publicJobRecord{}, fmt.Errorf("job %q cannot be resumed yet; next_action=%s", jobID, nextAction)
		}
	case appruns.StatusCancelled:
		return publicJobRecord{}, fmt.Errorf("job %q is cancelled and cannot be resumed", jobID)
	case appruns.StatusCompleted:
		return publicJobRecord{}, fmt.Errorf("job %q already completed and cannot be resumed", jobID)
	case appruns.StatusRunning:
		return publicJobRecord{}, fmt.Errorf("job %q is still running with run %q", jobID, latestRun.RunID)
	default:
		return publicJobRecord{}, fmt.Errorf("job %q status %s cannot be resumed", jobID, latestRun.Status)
	}
	builderSvc, err := h.buildersControlPlane()
	if err != nil {
		return publicJobRecord{}, fmt.Errorf("init builders service: %w", err)
	}
	runsSvc, err := h.buildRunsControlPlane()
	if err != nil {
		return publicJobRecord{}, fmt.Errorf("init runs service: %w", err)
	}
	preparedInput := bundle.BuilderInput
	preparedInput.ContextSourceDir = bundle.PrepareDir
	if resumeContext != nil {
		if mode == "" {
			mode = strings.TrimSpace(resumeContext.RecommendedResumeMode)
		}
		if resumeContext.RequiresHumanConfirmation && !req.Confirm {
			return publicJobRecord{}, fmt.Errorf("job %q cannot be resumed yet; next_action=%s", jobID, publicJobResumeNextAction(resumeContext))
		}
		if resumeContext.RequiresPreservedWorkspace {
			if strings.TrimSpace(resumeContext.PreservedWorkspacePath) == "" {
				return publicJobRecord{}, fmt.Errorf("job %q cannot be resumed yet; next_action=inspect_preserved_workspace", jobID)
			}
			if mode == "" {
				mode = "resume_from_failure"
			}
			if mode != "resume_from_failure" {
				return publicJobRecord{}, fmt.Errorf("job %q cannot be resumed with mode %s; next_action=resume_from_failure", jobID, mode)
			}
		}
	}
	if resumeContext != nil && strings.TrimSpace(resumeContext.PreservedWorkspacePath) != "" {
		preparedInput.WorkspacePath = resumeContext.PreservedWorkspacePath
	}
	if mode != "" {
		preparedInput.GoalSummary = preparedInput.GoalSummary + " | resume_mode: " + mode
	}
	if note := strings.TrimSpace(req.Note); note != "" {
		preparedInput.GoalSummary = preparedInput.GoalSummary + " | resume_note: " + note
	}
	dispatch, err := builderSvc.Dispatch(parent, appbuilders.Requirement{JobID: jobID})
	if err != nil {
		return publicJobRecord{}, err
	}
	run, err := runsSvc.Create(parent, dispatch.Node.BuilderID, dispatch.Node.BuilderID, dispatch.Lease.LeaseID, preparedInput)
	if err != nil {
		return publicJobRecord{}, err
	}
	if err := builderSvc.BindRun(parent, dispatch.Lease.LeaseID, run.RunID); err != nil {
		return publicJobRecord{}, err
	}
	record, err := h.buildPublicJobRecord(jobID)
	if err != nil {
		return publicJobRecord{}, err
	}
	if err := h.persistPublicJobRecord(record); err != nil {
		return publicJobRecord{}, err
	}
	if err := h.enqueuePublicJobExecution(jobID, run.RunID, "resume", executionTimeoutFromSeconds(req.TimeoutSeconds)); err != nil {
		return publicJobRecord{}, err
	}
	return record, nil
}

func executionTimeoutFromSeconds(timeoutSeconds int) time.Duration {
	timeout := 45 * time.Minute
	if timeoutSeconds > 0 {
		timeout = time.Duration(timeoutSeconds) * time.Second
	}
	return timeout
}

func (h *Handler) loadNotifications() ([]publicNotification, error) {
	workspace, err := h.appFactoryWorkspacePath()
	if err != nil {
		return nil, fmt.Errorf("init workspace: %w", err)
	}
	previousItems, err := h.loadPersistedNotificationsSnapshot(workspace)
	if err != nil {
		return nil, err
	}
	items := make([]publicNotification, 0)
	approvals, err := h.loadAllApprovalRecords(workspace)
	if err != nil {
		return nil, err
	}
	for _, approval := range approvals {
		switch approval.Status {
		case appruns.ApprovalStatusPending:
			items = append(items, publicNotification{
				NotificationID:  "notification-" + approval.ApprovalID,
				Type:            approvalNotificationType(approval),
				JobID:           approval.JobID,
				ApprovalID:      approval.ApprovalID,
				Status:          approval.Status,
				Summary:         approval.Summary,
				SuggestedAction: "review_approval",
				Links:           approvalNotificationLinks(approval),
				CreatedAt:       approval.CreatedAt,
			})
		case appruns.ApprovalStatusChangesRequested, appruns.ApprovalStatusRejected:
			summary := approval.Summary
			if approval.Decision != nil && strings.TrimSpace(approval.Decision.Comment) != "" {
				summary = approval.Decision.Comment
			}
			items = append(items, publicNotification{
				NotificationID:  "notification-decision-" + approval.ApprovalID,
				Type:            "approval_decided",
				JobID:           approval.JobID,
				ApprovalID:      approval.ApprovalID,
				Status:          approval.Status,
				Summary:         summary,
				SuggestedAction: "update_subject_and_resubmit",
				Links:           approvalNotificationLinks(approval),
				CreatedAt:       approvalDecisionAt(approval),
			})
		}
	}
	jobPaths, err := filepath.Glob(filepath.Join(workspace, "appfactory", "jobs", "*", "prepare", "builder-input.json"))
	if err != nil {
		return nil, fmt.Errorf("glob job records: %w", err)
	}
	for _, path := range jobPaths {
		jobID := filepath.Base(filepath.Dir(filepath.Dir(path)))
		record, err := h.buildPublicJobRecord(jobID)
		if err != nil {
			return nil, err
		}
		executionRecord, err := h.loadPublicJobExecutionRecord(workspace, jobID)
		if err != nil {
			return nil, err
		}
		if record.Status == "failed" && record.FailureContext != nil {
			summary := record.FailureContext.LastErrorSummary
			if strings.TrimSpace(summary) == "" {
				summary = "builder run failed"
			}
			notification := publicNotification{
				NotificationID:  "notification-builder-failed-" + jobID,
				Type:            "builder_failed",
				JobID:           jobID,
				Status:          record.Status,
				Summary:         summary,
				SuggestedAction: publicJobFailureSuggestedAction(record),
				Links:           publicJobNotificationLinks(record),
				CreatedAt:       record.UpdatedAt,
			}
			applyPublicJobResumeMetadataToNotification(&notification, record.ResumeContext)
			items = append(items, notification)
		}
		items = append(items, publicDeliveryNotifications(record)...)
		items = append(items, publicExecutionNotifications(record, executionRecord)...)
		if record.Status == "cancelled" {
			snapshotLinks := preservedSnapshotArtifactPaths(record)
			if len(snapshotLinks) > 0 {
				items = append(items, publicNotification{
					NotificationID:  "notification-workspace-preserved-" + jobID,
					Type:            "workspace_preserved",
					JobID:           jobID,
					Status:          record.Status,
					Summary:         "job cancelled and workspace snapshot preserved",
					SuggestedAction: "inspect_preserved_workspace",
					Links:           publicJobNotificationLinks(record),
					CreatedAt:       record.UpdatedAt,
				})
			}
		}
	}
	audits, err := h.loadPublicJobOrchestratorWatchAudits(workspace)
	if err != nil {
		return nil, err
	}
	for _, audit := range audits {
		items = append(items, publicOrchestratorWatchNotifications(audit)...)
	}
	items = dedupePublicNotifications(items)
	items = mergePublicNotificationAcknowledgements(items, previousItems)
	sort.Slice(items, func(i, j int) bool {
		return items[i].CreatedAt > items[j].CreatedAt
	})
	if err := h.persistPublicNotificationsSnapshot(workspace, items); err != nil {
		return nil, err
	}
	return items, nil
}

func (h *Handler) loadPersistedNotificationsSnapshot(workspace string) ([]publicNotification, error) {
	path := filepath.Join(workspace, "appfactory", "notifications", "index.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read notifications snapshot: %w", err)
	}
	var payload struct {
		Items []publicNotification `json:"items"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, fmt.Errorf("decode notifications snapshot: %w", err)
	}
	return payload.Items, nil
}

func dedupePublicNotifications(items []publicNotification) []publicNotification {
	if len(items) == 0 {
		return nil
	}
	result := make([]publicNotification, 0, len(items))
	seen := make(map[string]struct{}, len(items))
	for _, item := range items {
		if strings.TrimSpace(item.NotificationID) == "" {
			continue
		}
		if _, ok := seen[item.NotificationID]; ok {
			continue
		}
		seen[item.NotificationID] = struct{}{}
		result = append(result, item)
	}
	return result
}

func mergePublicNotificationAcknowledgements(items, previous []publicNotification) []publicNotification {
	if len(items) == 0 {
		return nil
	}
	acknowledged := make(map[string]publicNotification, len(previous))
	for _, item := range previous {
		if !item.Acknowledged {
			continue
		}
		acknowledged[item.NotificationID] = item
	}
	merged := append([]publicNotification(nil), items...)
	for index := range merged {
		if previousItem, ok := acknowledged[merged[index].NotificationID]; ok {
			merged[index].Acknowledged = true
			merged[index].AcknowledgedAt = previousItem.AcknowledgedAt
		}
	}
	return merged
}

func filterPublicNotifications(items []publicNotification, values url.Values) []publicNotification {
	if len(items) == 0 {
		return nil
	}
	typeFilter := strings.TrimSpace(values.Get("type"))
	jobIDFilter := strings.TrimSpace(values.Get("job_id"))
	ackFilter := strings.TrimSpace(values.Get("acknowledged"))
	filtered := make([]publicNotification, 0, len(items))
	for _, item := range items {
		if typeFilter != "" && item.Type != typeFilter {
			continue
		}
		if jobIDFilter != "" && item.JobID != jobIDFilter {
			continue
		}
		if ackFilter == "true" && !item.Acknowledged {
			continue
		}
		if ackFilter == "false" && item.Acknowledged {
			continue
		}
		filtered = append(filtered, item)
	}
	return filtered
}

func (h *Handler) acknowledgeNotification(notificationID string) (publicNotification, error) {
	workspace, err := h.appFactoryWorkspacePath()
	if err != nil {
		return publicNotification{}, fmt.Errorf("init workspace: %w", err)
	}
	items, err := h.loadNotifications()
	if err != nil {
		return publicNotification{}, err
	}
	acknowledgedAt := time.Now().UTC().Format(time.RFC3339)
	found := false
	for index := range items {
		if items[index].NotificationID != notificationID {
			continue
		}
		items[index].Acknowledged = true
		items[index].AcknowledgedAt = acknowledgedAt
		found = true
		break
	}
	if !found {
		return publicNotification{}, fmt.Errorf("notification %q not found", notificationID)
	}
	if err := h.persistPublicNotificationsSnapshot(workspace, items); err != nil {
		return publicNotification{}, err
	}
	for _, item := range items {
		if item.NotificationID == notificationID {
			return item, nil
		}
	}
	return publicNotification{}, fmt.Errorf("notification %q not found", notificationID)
}

func (h *Handler) persistPublicNotificationsSnapshot(workspace string, items []publicNotification) error {
	path := filepath.Join(workspace, "appfactory", "notifications", "index.json")
	data, err := json.MarshalIndent(map[string]any{"items": items}, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal notifications snapshot: %w", err)
	}
	data = append(data, '\n')
	if err := fileutil.WriteFileAtomic(path, data, 0o644); err != nil {
		return fmt.Errorf("write notifications snapshot: %w", err)
	}
	return nil
}

func syntheticPublicJobEvents(record publicJobRecord) []publicJobEvent {
	items := make([]publicJobEvent, 0)
	for _, path := range preservedSnapshotArtifactPaths(record) {
		items = append(items, publicJobEvent{
			At:      record.UpdatedAt,
			Type:    "workspace_preserved",
			JobID:   record.JobID,
			Status:  record.Status,
			Summary: "workspace snapshot preserved at " + path,
		})
	}
	if record.ResumeContext != nil {
		if record.ResumeContext.ResumeAllowed {
			event := publicJobEvent{
				At:      record.UpdatedAt,
				Type:    "resume_ready",
				JobID:   record.JobID,
				Status:  record.Status,
				Summary: "failed job can be resumed",
			}
			applyPublicJobResumeMetadataToEvent(&event, record.ResumeContext)
			items = append(items, event)
		} else {
			summary := "resume blocked"
			if nextAction := strings.TrimSpace(record.ResumeContext.NextAction); nextAction != "" {
				summary = "resume blocked; next_action=" + nextAction
			}
			event := publicJobEvent{
				At:      record.UpdatedAt,
				Type:    "resume_blocked",
				JobID:   record.JobID,
				Status:  record.Status,
				Summary: summary,
			}
			applyPublicJobResumeMetadataToEvent(&event, record.ResumeContext)
			items = append(items, event)
		}
		if record.ResumeContext.RequiresHumanConfirmation {
			event := publicJobEvent{
				At:      record.UpdatedAt,
				Type:    "resume_confirmation_required",
				JobID:   record.JobID,
				Status:  record.Status,
				Summary: "resume requires human confirmation",
			}
			applyPublicJobResumeMetadataToEvent(&event, record.ResumeContext)
			items = append(items, event)
		}
		if record.ResumeContext.RequiresPreservedWorkspace {
			event := publicJobEvent{
				At:      record.UpdatedAt,
				Type:    "resume_requires_preserved_workspace",
				JobID:   record.JobID,
				Status:  record.Status,
				Summary: "resume requires preserved workspace",
			}
			applyPublicJobResumeMetadataToEvent(&event, record.ResumeContext)
			items = append(items, event)
		}
	}
	return items
}

func syntheticPublicJobDeliveryEvents(record publicJobRecord) []publicJobEvent {
	if record.DeliveryContext == nil {
		return nil
	}
	event := publicJobEvent{
		At:      record.DeliveryContext.RecordedAt,
		Type:    deliveryEventType(record.DeliveryContext.Status),
		JobID:   record.JobID,
		Status:  record.Status,
		Summary: strings.TrimSpace(record.DeliveryContext.Summary),
	}
	applyPublicJobDeliveryMetadataToEvent(&event, record.DeliveryContext)
	if strings.TrimSpace(event.Type) == "" {
		event.Type = "delivery_recorded"
	}
	return []publicJobEvent{event}
}

func publicJobFailureSuggestedAction(record publicJobRecord) string {
	if record.ResumeContext != nil {
		return publicJobResumeNextAction(record.ResumeContext)
	}
	if len(preservedSnapshotArtifactPaths(record)) > 0 {
		return "inspect_preserved_workspace"
	}
	return "inspect_failure_and_retry_later"
}

func publicJobNotificationLinks(record publicJobRecord) []string {
	links := []string{filepath.ToSlash(filepath.Join("jobs", record.JobID, "job.json"))}
	if strings.TrimSpace(record.BuilderOutputPath) != "" {
		links = append(links, record.BuilderOutputPath)
	}
	if record.DeliveryContext != nil && strings.TrimSpace(record.DeliveryContext.DeliveryRecordPath) != "" {
		links = append(links, record.DeliveryContext.DeliveryRecordPath)
	}
	return mergePublicArtifactPaths(links, preservedSnapshotArtifactPaths(record))
}

func publicDeliveryNotifications(record publicJobRecord) []publicNotification {
	if record.DeliveryContext == nil {
		return nil
	}
	notificationType, suggestedAction := deliveryNotificationDescriptor(record.DeliveryContext.Status)
	if notificationType == "" {
		return nil
	}
	item := publicNotification{
		NotificationID:  fmt.Sprintf("notification-%s-%s-%s", notificationType, record.JobID, normalizeNotificationTimestamp(record.DeliveryContext.RecordedAt)),
		Type:            notificationType,
		JobID:           record.JobID,
		Status:          record.Status,
		Summary:         strings.TrimSpace(record.DeliveryContext.Summary),
		SuggestedAction: suggestedAction,
		Links:           publicJobNotificationLinks(record),
		CreatedAt:       strings.TrimSpace(record.DeliveryContext.RecordedAt),
	}
	applyPublicJobDeliveryMetadataToNotification(&item, record.DeliveryContext)
	return []publicNotification{item}
}

func publicExecutionNotifications(record publicJobRecord, executionRecord *publicJobExecutionRecord) []publicNotification {
	if executionRecord == nil {
		return nil
	}
	events := syntheticPublicJobExecutionEvents(executionRecord)
	items := make([]publicNotification, 0, len(events))
	for _, event := range events {
		notificationType, suggestedAction := publicExecutionNotificationDescriptor(record, event)
		if notificationType == "" {
			continue
		}
		items = append(items, publicNotification{
			NotificationID:  fmt.Sprintf("notification-%s-%s-%s", notificationType, record.JobID, normalizeNotificationTimestamp(event.At)),
			Type:            notificationType,
			JobID:           record.JobID,
			Status:          strings.TrimSpace(event.Status),
			Summary:         strings.TrimSpace(event.Summary),
			SuggestedAction: suggestedAction,
			Links:           publicExecutionNotificationLinks(record),
			CreatedAt:       strings.TrimSpace(event.At),
		})
	}
	return items
}

func publicExecutionNotificationDescriptor(record publicJobRecord, event publicJobEvent) (string, string) {
	switch strings.TrimSpace(event.Type) {
	case "execution_dispatcher_lost":
		return "execution_interrupted", "resume_interrupted_job"
	case "execution_recovery_failed":
		return "execution_recovery_failed", "inspect_failure_and_retry_later"
	case "execution_handoff":
		return "execution_handoff", "inspect_execution_history"
	default:
		return "", ""
	}
}

func publicExecutionNotificationLinks(record publicJobRecord) []string {
	links := publicJobNotificationLinks(record)
	links = append(links, filepath.ToSlash(filepath.Join("orchestrator", "jobs", record.JobID+".json")))
	return mergePublicArtifactPaths(nil, links)
}

func publicOrchestratorWatchNotifications(record publicJobOrchestratorWatchAuditRecord) []publicNotification {
	if strings.TrimSpace(record.Action) == "" {
		return nil
	}
	return []publicNotification{{
		NotificationID:  "notification-" + record.AuditID,
		Type:            strings.TrimSpace(record.Action),
		Status:          strings.TrimSpace(record.WatchRunnerState),
		Summary:         strings.TrimSpace(record.Summary),
		SuggestedAction: orchestratorWatchSuggestedAction(record),
		Links: []string{
			filepath.ToSlash(filepath.Join("orchestrator", "status.json")),
			filepath.ToSlash(filepath.Join("orchestrator", "watch-events.jsonl")),
		},
		CreatedAt: strings.TrimSpace(record.CreatedAt),
	}}
}

func orchestratorWatchSuggestedAction(record publicJobOrchestratorWatchAuditRecord) string {
	switch strings.TrimSpace(record.Action) {
	case "orchestrator_watch_started":
		return "inspect_orchestrator_status"
	case "orchestrator_watch_stopped":
		return "restart_orchestrator_watch"
	case "orchestrator_watch_unlocked":
		return "inspect_orchestrator_lock_owner"
	default:
		return "inspect_orchestrator_status"
	}
}

func normalizeNotificationTimestamp(value string) string {
	trimmed := strings.TrimSpace(value)
	trimmed = strings.ReplaceAll(trimmed, ":", "-")
	trimmed = strings.ReplaceAll(trimmed, ".", "-")
	trimmed = strings.ReplaceAll(trimmed, "+", "-")
	return trimmed
}

func preservedSnapshotArtifactPaths(record publicJobRecord) []string {
	paths := make([]string, 0)
	for _, path := range record.Artifacts {
		trimmed := strings.TrimSpace(path)
		if trimmed == "" {
			continue
		}
		if strings.Contains(trimmed, "/snapshots/preserved/") {
			paths = append(paths, trimmed)
		}
	}
	if record.ResumeContext != nil {
		path := strings.TrimSpace(record.ResumeContext.PreservedWorkspacePath)
		if path != "" && strings.Contains(path, "/snapshots/preserved/") {
			paths = mergePublicArtifactPaths(paths, []string{path})
		}
	}
	return paths
}

func (h *Handler) loadAllApprovalRecords(workspace string) ([]appruns.ApprovalRecord, error) {
	records := make(map[string]appruns.ApprovalRecord)
	approvalPaths, err := filepath.Glob(filepath.Join(workspace, "appfactory", "approvals", "*.json"))
	if err != nil {
		return nil, fmt.Errorf("glob approval index: %w", err)
	}
	preparePaths, err := filepath.Glob(filepath.Join(workspace, "appfactory", "jobs", "*", "prepare", "*-approval.json"))
	if err != nil {
		return nil, fmt.Errorf("glob approval snapshots: %w", err)
	}
	for _, path := range append(approvalPaths, preparePaths...) {
		record, err := readApprovalRecordFromPath(path)
		if err != nil {
			return nil, fmt.Errorf("read approval record: %w", err)
		}
		if strings.TrimSpace(record.ApprovalID) == "" {
			continue
		}
		records[record.ApprovalID] = record
	}
	items := make([]appruns.ApprovalRecord, 0, len(records))
	for _, record := range records {
		items = append(items, record)
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].CreatedAt > items[j].CreatedAt
	})
	return items, nil
}

func approvalNotificationType(record appruns.ApprovalRecord) string {
	if record.Status != appruns.ApprovalStatusPending {
		return "approval_requested"
	}
	switch record.ApprovalType {
	case appruns.ApprovalTypePRD:
		return "prd_approval_requested"
	case appruns.ApprovalTypeTemplate:
		return "template_approval_requested"
	default:
		return "approval_requested"
	}
}

func approvalNotificationLinks(record appruns.ApprovalRecord) []string {
	links := []string{}
	if strings.TrimSpace(record.JobID) != "" {
		links = append(links, filepath.ToSlash(filepath.Join("jobs", record.JobID, "job.json")))
	}
	if strings.TrimSpace(record.ApprovalType) == appruns.ApprovalTypePRD && strings.TrimSpace(record.JobID) != "" {
		links = append(links, filepath.ToSlash(filepath.Join("jobs", record.JobID, "prepare", appruns.PRDApprovalFileName)))
	}
	if strings.TrimSpace(record.ApprovalType) == appruns.ApprovalTypeTemplate && strings.TrimSpace(record.JobID) != "" {
		links = append(links, filepath.ToSlash(filepath.Join("jobs", record.JobID, "prepare", appruns.TemplateApprovalFileName)))
	}
	return links
}

func approvalDecisionAt(record appruns.ApprovalRecord) string {
	if record.Decision != nil && strings.TrimSpace(record.Decision.DecidedAt) != "" {
		return record.Decision.DecidedAt
	}
	return record.CreatedAt
}

func (h *Handler) createApproval(req createApprovalRequest) (appruns.ApprovalRecord, error) {
	approvalType := strings.TrimSpace(req.ApprovalType)
	if !isSupportedApprovalType(approvalType) {
		return appruns.ApprovalRecord{}, fmt.Errorf("unsupported approval_type %q", approvalType)
	}
	now := time.Now().UTC()
	jobID := strings.TrimSpace(req.JobID)
	prdid := strings.TrimSpace(req.PRDID)
	subjectVersion := strings.TrimSpace(req.SubjectVersion)
	summary := strings.TrimSpace(req.Summary)
	evidencePaths := normalizeStringList(req.EvidencePaths)
	record := appruns.ApprovalRecord{
		SchemaVersion: "0.1.0",
		ApprovalType:  approvalType,
		JobID:         jobID,
		PRDID:         prdid,
		Status:        appruns.ApprovalStatusPending,
		RequestedBy:   appruns.ApprovalActor{ActorType: "system", ActorID: "appfactory-api"},
		CreatedAt:     now.Format(time.RFC3339),
	}
	switch approvalType {
	case appruns.ApprovalTypePRD:
		if jobID == "" {
			return appruns.ApprovalRecord{}, fmt.Errorf("job_id is required for prd approval")
		}
		bundle, err := h.loadPreparedBundleByJobID(jobID)
		if err != nil {
			return appruns.ApprovalRecord{}, err
		}
		if prdid != "" && prdid != bundle.PRD.ID {
			return appruns.ApprovalRecord{}, fmt.Errorf("prepared prd mismatch: want %s got %s", bundle.PRD.ID, prdid)
		}
		record.PRDID = bundle.PRD.ID
		if subjectVersion == "" {
			subjectVersion = strings.TrimSpace(bundle.PRD.Version)
		}
		if subjectVersion == "" {
			subjectVersion = "unknown"
		}
		if summary == "" {
			summary = fmt.Sprintf("PRD %s 等待人工审批。", bundle.PRD.ID)
		}
		if len(evidencePaths) == 0 {
			evidencePaths = []string{"PRD.md", "PRD.json", "requirement.md"}
		}
		record.ApprovalID = newApprovalID(approvalType, jobID, bundle.PRD.ID, now)
	case appruns.ApprovalTypeTemplate:
		if jobID == "" {
			return appruns.ApprovalRecord{}, fmt.Errorf("job_id is required for template approval")
		}
		bundle, err := h.loadPreparedBundleByJobID(jobID)
		if err != nil {
			return appruns.ApprovalRecord{}, err
		}
		templateID := strings.TrimSpace(req.TemplateID)
		if templateID != "" && templateID != bundle.BuilderInput.TemplateID {
			return appruns.ApprovalRecord{}, fmt.Errorf("prepared template mismatch: want %s got %s", bundle.BuilderInput.TemplateID, templateID)
		}
		entry, err := appprepare.GetTemplateRegistryEntry(bundle.BuilderInput.TemplateID)
		if err != nil {
			return appruns.ApprovalRecord{}, err
		}
		record.PRDID = bundle.PRD.ID
		if subjectVersion == "" {
			version := strings.TrimSpace(entry.PinnedRef)
			if version == "" {
				version = "unknown"
			}
			subjectVersion = "selected-template@" + bundle.BuilderInput.TemplateID + "@" + version
		}
		if summary == "" {
			summary = fmt.Sprintf("模板 %s 等待人工审批。", bundle.BuilderInput.TemplateID)
		}
		if len(evidencePaths) == 0 {
			evidencePaths = []string{"template-fit-report.md", "PRD.json"}
		}
		record.ApprovalID = newApprovalID(approvalType, jobID, bundle.BuilderInput.TemplateID, now)
	default:
		if subjectVersion == "" {
			return appruns.ApprovalRecord{}, fmt.Errorf("subject_version is required for %s approval", approvalType)
		}
		if summary == "" {
			summary = fmt.Sprintf("%s approval is pending.", approvalType)
		}
		record.ApprovalID = newApprovalID(approvalType, jobID, subjectVersion, now)
	}
	record.SubjectVersion = subjectVersion
	record.Summary = summary
	record.EvidencePaths = evidencePaths
	if err := h.writeApprovalRecord(record); err != nil {
		return appruns.ApprovalRecord{}, err
	}
	return record, nil
}

func (h *Handler) decideApproval(approvalID string, req approvalDecisionRequest) (appruns.ApprovalRecord, error) {
	record, err := h.loadApprovalRecord(approvalID)
	if err != nil {
		return appruns.ApprovalRecord{}, err
	}
	if record.Decision != nil || isTerminalApprovalStatus(record.Status) {
		return appruns.ApprovalRecord{}, fmt.Errorf("approval %q already decided", approvalID)
	}
	decision := strings.TrimSpace(req.Decision)
	if !isDecisionStatus(decision) {
		return appruns.ApprovalRecord{}, fmt.Errorf("unsupported decision %q", decision)
	}
	requiredChanges := normalizeStringList(req.RequiredChanges)
	if decision != appruns.ApprovalStatusChangesRequested {
		requiredChanges = nil
	}
	now := time.Now().UTC()
	record.Status = decision
	record.Decision = &appruns.ApprovalDecision{
		Decision:        decision,
		DecidedBy:       appruns.ApprovalActor{ActorType: "human", ActorID: strings.TrimSpace(req.ReviewerID)},
		DecidedAt:       now.Format(time.RFC3339),
		Comment:         strings.TrimSpace(req.Comment),
		RequiredChanges: requiredChanges,
	}
	if err := h.writeApprovalRecord(record); err != nil {
		return appruns.ApprovalRecord{}, err
	}
	return record, nil
}

func (h *Handler) loadApprovalRecord(approvalID string) (appruns.ApprovalRecord, error) {
	workspace, err := h.appFactoryWorkspacePath()
	if err != nil {
		return appruns.ApprovalRecord{}, fmt.Errorf("init workspace: %w", err)
	}
	approvalPath := filepath.Join(workspace, "appfactory", "approvals", strings.TrimSpace(approvalID)+".json")
	record, err := readApprovalRecordFromPath(approvalPath)
	if err == nil {
		return record, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return appruns.ApprovalRecord{}, fmt.Errorf("read approval record: %w", err)
	}
	paths, globErr := filepath.Glob(filepath.Join(workspace, "appfactory", "jobs", "*", "prepare", "*-approval.json"))
	if globErr != nil {
		return appruns.ApprovalRecord{}, fmt.Errorf("glob approval records: %w", globErr)
	}
	for _, candidate := range paths {
		record, readErr := readApprovalRecordFromPath(candidate)
		if readErr != nil {
			return appruns.ApprovalRecord{}, fmt.Errorf("read approval record: %w", readErr)
		}
		if record.ApprovalID == strings.TrimSpace(approvalID) {
			return record, nil
		}
	}
	return appruns.ApprovalRecord{}, fmt.Errorf("approval %q not found", strings.TrimSpace(approvalID))
}

func (h *Handler) writeApprovalRecord(record appruns.ApprovalRecord) error {
	workspace, err := h.appFactoryWorkspacePath()
	if err != nil {
		return fmt.Errorf("init workspace: %w", err)
	}
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal approval record: %w", err)
	}
	data = append(data, '\n')
	approvalDir := filepath.Join(workspace, "appfactory", "approvals")
	if err := os.MkdirAll(approvalDir, 0o755); err != nil {
		return fmt.Errorf("create approvals dir: %w", err)
	}
	approvalPath := filepath.Join(approvalDir, record.ApprovalID+".json")
	if err := fileutil.WriteFileAtomic(approvalPath, data, 0o644); err != nil {
		return fmt.Errorf("write approval record: %w", err)
	}
	if snapshotPath := approvalSnapshotPath(workspace, record); snapshotPath != "" {
		if err := fileutil.WriteFileAtomic(snapshotPath, data, 0o644); err != nil {
			return fmt.Errorf("write approval snapshot: %w", err)
		}
	}
	return nil
}

func readApprovalRecordFromPath(path string) (appruns.ApprovalRecord, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return appruns.ApprovalRecord{}, err
	}
	var record appruns.ApprovalRecord
	if err := json.Unmarshal(data, &record); err != nil {
		return appruns.ApprovalRecord{}, err
	}
	return record, nil
}

func approvalSnapshotPath(workspace string, record appruns.ApprovalRecord) string {
	if strings.TrimSpace(record.JobID) == "" {
		return ""
	}
	switch strings.TrimSpace(record.ApprovalType) {
	case appruns.ApprovalTypePRD:
		return filepath.Join(workspace, "appfactory", "jobs", record.JobID, "prepare", appruns.PRDApprovalFileName)
	case appruns.ApprovalTypeTemplate:
		return filepath.Join(workspace, "appfactory", "jobs", record.JobID, "prepare", appruns.TemplateApprovalFileName)
	default:
		return ""
	}
}

func normalizeStringList(items []string) []string {
	result := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		result = append(result, item)
	}
	return result
}

func isSupportedApprovalType(approvalType string) bool {
	switch approvalType {
	case appruns.ApprovalTypePRD, appruns.ApprovalTypeTemplate, "execution-plan", "build-result", "handoff", "escalation":
		return true
	default:
		return false
	}
}

func isDecisionStatus(status string) bool {
	switch status {
	case appruns.ApprovalStatusApproved, appruns.ApprovalStatusRejected, appruns.ApprovalStatusChangesRequested:
		return true
	default:
		return false
	}
}

func isTerminalApprovalStatus(status string) bool {
	switch strings.TrimSpace(status) {
	case appruns.ApprovalStatusApproved, appruns.ApprovalStatusRejected, appruns.ApprovalStatusChangesRequested, "expired", "cancelled":
		return true
	default:
		return false
	}
}

func newApprovalID(approvalType, jobID, subject string, now time.Time) string {
	parts := []string{"approval", sanitizeApprovalIDPart(approvalType)}
	if strings.TrimSpace(jobID) != "" {
		parts = append(parts, sanitizeApprovalIDPart(jobID))
	}
	if strings.TrimSpace(subject) != "" {
		parts = append(parts, sanitizeApprovalIDPart(subject))
	}
	parts = append(parts, strconv.FormatInt(now.UnixMilli(), 10))
	return strings.ToLower(strings.Join(parts, "-"))
}

func sanitizeApprovalIDPart(value string) string {
	replacer := strings.NewReplacer("/", "-", "\\", "-", " ", "-", "@", "-", ":", "-", ".", "-", "_", "-")
	cleaned := replacer.Replace(strings.TrimSpace(value))
	cleaned = strings.Trim(cleaned, "-")
	if cleaned == "" {
		return "item"
	}
	return cleaned
}

func (h *Handler) submitTemplateApproval(entry appprepare.TemplateRegistryEntry, req submitTemplateApprovalRequest) (appruns.ApprovalRecord, error) {
	workspace, err := h.appFactoryWorkspacePath()
	if err != nil {
		return appruns.ApprovalRecord{}, fmt.Errorf("init workspace: %w", err)
	}
	prepareDir := filepath.Join(workspace, "appfactory", "jobs", strings.TrimSpace(req.JobID), "prepare")
	prdPath := filepath.Join(prepareDir, "PRD.json")
	data, err := os.ReadFile(prdPath)
	if err != nil {
		return appruns.ApprovalRecord{}, fmt.Errorf("load prepared prd: %w", err)
	}
	var prd appprepare.PRD
	if err := json.Unmarshal(data, &prd); err != nil {
		return appruns.ApprovalRecord{}, fmt.Errorf("decode prepared prd: %w", err)
	}
	if prd.ID != strings.TrimSpace(req.PRDID) {
		return appruns.ApprovalRecord{}, fmt.Errorf("prepared prd mismatch: want %s got %s", strings.TrimSpace(req.PRDID), prd.ID)
	}
	version := strings.TrimSpace(req.Version)
	if version == "" {
		version = entry.PinnedRef
	}
	if version == "" {
		version = "unknown"
	}
	now := time.Now().UTC()
	summary := strings.TrimSpace(req.Summary)
	if summary == "" {
		summary = fmt.Sprintf("模板 %s 已被确认为当前 PRD 的批准模板。", entry.TemplateID)
	}
	record := appruns.ApprovalRecord{
		SchemaVersion:  "0.1.0",
		ApprovalID:     newApprovalID(appruns.ApprovalTypeTemplate, strings.TrimSpace(req.JobID), entry.TemplateID, now),
		ApprovalType:   appruns.ApprovalTypeTemplate,
		JobID:          strings.TrimSpace(req.JobID),
		PRDID:          prd.ID,
		SubjectVersion: "selected-template@" + entry.TemplateID + "@" + version,
		Status:         appruns.ApprovalStatusApproved,
		RequestedBy:    appruns.ApprovalActor{ActorType: "system", ActorID: "appfactory-api"},
		Decision: &appruns.ApprovalDecision{
			Decision:  appruns.ApprovalStatusApproved,
			DecidedBy: appruns.ApprovalActor{ActorType: "system", ActorID: "appfactory-api"},
			DecidedAt: now.Format(time.RFC3339),
			Comment:   summary,
		},
		Summary:       summary,
		EvidencePaths: []string{"template-fit-report.md", "PRD.json"},
		CreatedAt:     now.Format(time.RFC3339),
	}
	if err := h.writeApprovalRecord(record); err != nil {
		return appruns.ApprovalRecord{}, fmt.Errorf("persist template approval: %w", err)
	}
	return record, nil
}

func (h *Handler) submitPRDApproval(prdID string, req submitPRDApprovalRequest) (appruns.ApprovalRecord, error) {
	workspace, err := h.appFactoryWorkspacePath()
	if err != nil {
		return appruns.ApprovalRecord{}, fmt.Errorf("init workspace: %w", err)
	}
	prepareDir := filepath.Join(workspace, "appfactory", "jobs", strings.TrimSpace(req.JobID), "prepare")
	prdPath := filepath.Join(prepareDir, "PRD.json")
	data, err := os.ReadFile(prdPath)
	if err != nil {
		return appruns.ApprovalRecord{}, fmt.Errorf("load prepared prd: %w", err)
	}
	var prd appprepare.PRD
	if err := json.Unmarshal(data, &prd); err != nil {
		return appruns.ApprovalRecord{}, fmt.Errorf("decode prepared prd: %w", err)
	}
	if prd.ID != strings.TrimSpace(prdID) {
		return appruns.ApprovalRecord{}, fmt.Errorf("prepared prd mismatch: want %s got %s", strings.TrimSpace(prdID), prd.ID)
	}
	version := strings.TrimSpace(req.Version)
	if version == "" {
		version = prd.Version
	}
	if version == "" {
		version = "unknown"
	}
	now := time.Now().UTC()
	summary := strings.TrimSpace(req.Summary)
	if summary == "" {
		summary = fmt.Sprintf("PRD %s 已被确认为当前需求的批准版本。", prd.ID)
	}
	evidencePaths := make([]string, 0, len(req.EvidencePaths))
	for _, item := range req.EvidencePaths {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		evidencePaths = append(evidencePaths, item)
	}
	if len(evidencePaths) == 0 {
		evidencePaths = []string{"PRD.md", "PRD.json", "requirement.md"}
	}
	record := appruns.ApprovalRecord{
		SchemaVersion:  "0.1.0",
		ApprovalID:     newApprovalID(appruns.ApprovalTypePRD, strings.TrimSpace(req.JobID), prd.ID, now),
		ApprovalType:   appruns.ApprovalTypePRD,
		JobID:          strings.TrimSpace(req.JobID),
		PRDID:          prd.ID,
		SubjectVersion: version,
		Status:         appruns.ApprovalStatusApproved,
		RequestedBy:    appruns.ApprovalActor{ActorType: "system", ActorID: "appfactory-api"},
		Decision: &appruns.ApprovalDecision{
			Decision:  appruns.ApprovalStatusApproved,
			DecidedBy: appruns.ApprovalActor{ActorType: "system", ActorID: "appfactory-api"},
			DecidedAt: now.Format(time.RFC3339),
			Comment:   summary,
		},
		Summary:       summary,
		EvidencePaths: evidencePaths,
		CreatedAt:     now.Format(time.RFC3339),
	}
	if err := h.writeApprovalRecord(record); err != nil {
		return appruns.ApprovalRecord{}, fmt.Errorf("persist prd approval: %w", err)
	}
	return record, nil
}

func (h *Handler) compilePreparedBundle(_ context.Context, req compileRequestInput) (appprepare.Bundle, string, error) {
	if strings.TrimSpace(req.RequirementText) == "" {
		return appprepare.Bundle{}, "", fmt.Errorf("requirement_text is required")
	}
	bundle, err := appprepare.Compile(appprepare.Request{
		RequirementText:   req.RequirementText,
		RequirementSource: req.RequirementSource,
		TitleHint:         req.Title,
		JobID:             req.JobID,
		PRDID:             req.PRDID,
		TemplateID:        req.TemplateID,
		ExecutorImage:     req.ExecutorImage,
		RealBuild:         req.RealChecks || strings.TrimSpace(req.ExecutorImage) != "",
	})
	if err != nil {
		return appprepare.Bundle{}, "", err
	}
	workspace, err := h.appFactoryWorkspacePath()
	if err != nil {
		return appprepare.Bundle{}, "", fmt.Errorf("init workspace: %w", err)
	}
	bundleDir := filepath.Join(workspace, "appfactory", "jobs", bundle.BuilderInput.JobID, "prepare")
	if err := appprepare.WriteBundle(bundleDir, bundle); err != nil {
		return appprepare.Bundle{}, "", fmt.Errorf("write bundle: %w", err)
	}
	return bundle, bundleDir, nil
}

func (h *Handler) executePreparedRequirementRun(parent context.Context, input appruns.BuildInput, req runRequirementRequest) (runRequirementResult, error) {
	builderSvc, err := h.buildersControlPlane()
	if err != nil {
		return runRequirementResult{}, fmt.Errorf("init builders service: %w", err)
	}
	runsSvc, err := h.buildRunsControlPlane()
	if err != nil {
		return runRequirementResult{}, fmt.Errorf("init runs service: %w", err)
	}
	displayName := strings.TrimSpace(req.DisplayName)
	if displayName == "" {
		displayName = req.BuilderID
	}
	builderImage := strings.TrimSpace(req.BuilderImage)
	if builderImage == "" {
		builderImage = strings.TrimSpace(input.ExecutorImage)
	}
	if builderImage == "" {
		builderImage = "picoclaw/appfactory-builder:local"
	}
	if _, err := builderSvc.Register(parent, appbuilders.RegisterRequest{
		BuilderID:      req.BuilderID,
		DisplayName:    displayName,
		CapabilityTags: append([]string(nil), req.CapabilityTags...),
		ModelTags:      append([]string(nil), req.ModelTags...),
		WorkerProfile: appbuilders.WorkerProfile{
			Image: builderImage,
		},
	}); err != nil {
		return runRequirementResult{}, err
	}
	dispatch, err := builderSvc.Dispatch(parent, appbuilders.Requirement{JobID: input.JobID})
	if err != nil {
		return runRequirementResult{}, err
	}
	run, err := runsSvc.Create(parent, dispatch.Node.BuilderID, dispatch.Node.BuilderID, dispatch.Lease.LeaseID, input)
	if err != nil {
		return runRequirementResult{}, err
	}
	if err := builderSvc.BindRun(parent, dispatch.Lease.LeaseID, run.RunID); err != nil {
		return runRequirementResult{}, err
	}
	timeout := 45 * time.Minute
	if req.TimeoutSeconds > 0 {
		timeout = time.Duration(req.TimeoutSeconds) * time.Second
	}
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	completed, err := h.executePreparedRunLocally(ctx, runsSvc, builderSvc, run.RunID)
	if err != nil {
		return runRequirementResult{}, err
	}
	return runRequirementResult{RunID: completed.RunID, WorkerID: completed.WorkerID, Status: string(completed.Status)}, nil
}

func (h *Handler) appFactoryWorkspacePath() (string, error) {
	if err := h.ensureAppFactoryEnvLoaded(); err != nil {
		return "", err
	}
	cfg, err := config.LoadConfig(h.configPath)
	if err != nil {
		return "", fmt.Errorf("load config: %w", err)
	}
	workspace := cfg.WorkspacePath()
	if workspace == "" {
		return "", fmt.Errorf("workspace path is empty")
	}
	return workspace, nil
}

func (h *Handler) ensureAppFactoryEnvLoaded() error {
	h.appFactory.envOnce.Do(func() {
		h.appFactory.envErr = loadClosestAppFactoryEnv(h.configPath)
	})
	return h.appFactory.envErr
}

func loadClosestAppFactoryEnv(configPath string) error {
	searchRoots := make([]string, 0, 2)
	if strings.TrimSpace(configPath) != "" {
		searchRoots = append(searchRoots, filepath.Dir(configPath))
	} else if wd, err := os.Getwd(); err == nil && strings.TrimSpace(wd) != "" {
		searchRoots = append(searchRoots, wd)
	}
	for _, root := range searchRoots {
		if _, err := envfile.LoadClosest(root, false); err != nil {
			if errors.Is(err, envfile.ErrNotFound) {
				continue
			}
			return fmt.Errorf("load appfactory env from %s: %w", filepath.Clean(root), err)
		}
		return nil
	}
	return nil
}

func (h *Handler) buildersControlPlane() (builderService, error) {
	workspace, err := h.appFactoryWorkspacePath()
	if err != nil {
		return nil, err
	}

	h.appFactory.mu.Lock()
	defer h.appFactory.mu.Unlock()
	if h.appFactory.builders != nil && h.appFactory.workspace == workspace {
		return h.appFactory.builders, nil
	}
	registry, err := appbuilders.NewFileRegistryStore(filepath.Join(workspace, "appfactory", "builders", "nodes"))
	if err != nil {
		return nil, err
	}
	leases, err := appbuilders.NewFileLeaseStore(filepath.Join(workspace, "appfactory", "builders", "leases"))
	if err != nil {
		return nil, err
	}
	h.appFactory.builders = appbuilders.NewService(registry, leases, 10*time.Minute, 2*time.Minute)
	h.appFactory.workspace = workspace
	return h.appFactory.builders, nil
}

func (h *Handler) buildRunsControlPlane() (runService, error) {
	workspace, err := h.appFactoryWorkspacePath()
	if err != nil {
		return nil, err
	}

	h.appFactory.mu.Lock()
	defer h.appFactory.mu.Unlock()
	if h.appFactory.runs != nil && h.appFactory.workspace == workspace {
		return h.appFactory.runs, nil
	}
	store, err := appruns.NewFileStore(filepath.Join(workspace, "appfactory"))
	if err != nil {
		return nil, err
	}
	h.appFactory.runs = appruns.NewService(store)
	h.appFactory.workspace = workspace
	return h.appFactory.runs, nil
}

func decodeJSONBody(r *http.Request, target any) error {
	defer r.Body.Close()
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeBuilderDomainError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, appbuilders.ErrInvalidRequest):
		writeInternalError(w, http.StatusBadRequest, "VALIDATION_INVALID_REQUEST", err.Error())
	case errors.Is(err, appbuilders.ErrBuilderNotFound), errors.Is(err, appbuilders.ErrLeaseNotFound):
		writeInternalError(w, http.StatusNotFound, "BUILDERS_NOT_FOUND", err.Error())
	case errors.Is(err, appbuilders.ErrBuilderAlreadyLeased):
		writeInternalError(w, http.StatusConflict, "WORKER_ALREADY_ALLOCATED", err.Error())
	case errors.Is(err, appbuilders.ErrNoBuilderCandidate):
		writeInternalError(w, http.StatusConflict, "WORKER_NO_CANDIDATE", err.Error())
	default:
		writeInternalError(w, http.StatusInternalServerError, "BUILDERS_INTERNAL_ERROR", err.Error())
	}
}

func writeRunDomainError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, appruns.ErrInvalidRequest):
		writeInternalError(w, http.StatusBadRequest, "VALIDATION_INVALID_REQUEST", err.Error())
	case errors.Is(err, appruns.ErrRunNotFound):
		writeInternalError(w, http.StatusNotFound, "BUILD_RUN_NOT_FOUND", err.Error())
	case errors.Is(err, appruns.ErrTerminalRun):
		writeInternalError(w, http.StatusConflict, "BUILD_RUN_TERMINAL", err.Error())
	default:
		writeInternalError(w, http.StatusInternalServerError, "BUILD_RUN_INTERNAL_ERROR", err.Error())
	}
}

func writeInternalError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, internalErrorResponse{
		ErrorCode: code,
		Message:   message,
	})
}

func writeRunRequirementError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, appbuilders.ErrInvalidRequest), errors.Is(err, appruns.ErrInvalidRequest):
		writeInternalError(w, http.StatusBadRequest, "VALIDATION_INVALID_REQUEST", err.Error())
	case errors.Is(err, appbuilders.ErrBuilderNotFound), errors.Is(err, appbuilders.ErrLeaseNotFound), errors.Is(err, appruns.ErrRunNotFound):
		writeInternalError(w, http.StatusNotFound, "APPFACTORY_NOT_FOUND", err.Error())
	case errors.Is(err, appbuilders.ErrBuilderAlreadyLeased), errors.Is(err, appbuilders.ErrNoBuilderCandidate), errors.Is(err, appruns.ErrTerminalRun):
		writeInternalError(w, http.StatusConflict, "APPFACTORY_CONFLICT", err.Error())
	default:
		writeInternalError(w, http.StatusInternalServerError, "APPFACTORY_EXECUTION_FAILED", err.Error())
	}
}

func writeCompilePRDError(w http.ResponseWriter, err error) {
	message := err.Error()
	switch {
	case strings.Contains(message, "requirement_text is required"):
		writeInternalError(w, http.StatusBadRequest, "VALIDATION_INVALID_REQUEST", message)
	case strings.Contains(message, "not found in registry"), strings.Contains(message, "no template matches"):
		writeInternalError(w, http.StatusBadRequest, "PREPARE_COMPILE_FAILED", message)
	case strings.Contains(message, "init workspace"):
		writeInternalError(w, http.StatusInternalServerError, "APPFACTORY_INIT_FAILED", message)
	case strings.Contains(message, "write bundle"):
		writeInternalError(w, http.StatusInternalServerError, "PREPARE_WRITE_FAILED", message)
	default:
		writeInternalError(w, http.StatusBadRequest, "PREPARE_COMPILE_FAILED", message)
	}
}

func writeTemplateMatchError(w http.ResponseWriter, err error) {
	message := err.Error()
	switch {
	case strings.Contains(message, "prd ") && strings.Contains(message, "not found"):
		writeInternalError(w, http.StatusNotFound, "PRD_NOT_FOUND", message)
	case strings.Contains(message, "init workspace"):
		writeInternalError(w, http.StatusInternalServerError, "APPFACTORY_INIT_FAILED", message)
	default:
		writeInternalError(w, http.StatusInternalServerError, "TEMPLATE_MATCH_FAILED", message)
	}
}

func writePRDReadError(w http.ResponseWriter, err error) {
	message := err.Error()
	switch {
	case strings.Contains(message, "prd ") && strings.Contains(message, "not found"):
		writeInternalError(w, http.StatusNotFound, "PRD_NOT_FOUND", message)
	case strings.Contains(message, "init workspace"):
		writeInternalError(w, http.StatusInternalServerError, "APPFACTORY_INIT_FAILED", message)
	case strings.Contains(message, "read prepared prd"), strings.Contains(message, "decode prepared prd"), strings.Contains(message, "glob prepared prds"):
		writeInternalError(w, http.StatusInternalServerError, "PRD_READ_FAILED", message)
	default:
		writeInternalError(w, http.StatusInternalServerError, "PRD_READ_FAILED", message)
	}
}

func writeJobError(w http.ResponseWriter, err error) {
	message := err.Error()
	switch {
	case strings.Contains(message, "job ") && strings.Contains(message, "not found"):
		writeInternalError(w, http.StatusNotFound, "JOB_NOT_FOUND", message)
	case strings.Contains(message, "prd ") && strings.Contains(message, "not found"):
		writeInternalError(w, http.StatusNotFound, "PRD_NOT_FOUND", message)
	case strings.Contains(message, "init workspace"):
		writeInternalError(w, http.StatusInternalServerError, "APPFACTORY_INIT_FAILED", message)
	case strings.Contains(message, "write job record"), strings.Contains(message, "marshal job record"):
		writeInternalError(w, http.StatusInternalServerError, "JOB_WRITE_FAILED", message)
	case strings.Contains(message, "prepared template mismatch"):
		writeInternalError(w, http.StatusBadRequest, "VALIDATION_INVALID_REQUEST", message)
	default:
		writeInternalError(w, http.StatusInternalServerError, "JOB_READ_FAILED", message)
	}
}

func writeJobStartError(w http.ResponseWriter, err error) {
	message := err.Error()
	switch {
	case strings.Contains(message, "job ") && strings.Contains(message, "not found"):
		writeInternalError(w, http.StatusNotFound, "JOB_NOT_FOUND", message)
	case strings.Contains(message, "approvals not ready"), strings.Contains(message, "already has run"), strings.Contains(message, "active execution"):
		writeInternalError(w, http.StatusConflict, "JOB_START_CONFLICT", message)
	case errors.Is(err, appbuilders.ErrNoBuilderCandidate), errors.Is(err, appbuilders.ErrBuilderAlreadyLeased):
		writeInternalError(w, http.StatusConflict, "WORKER_NO_CANDIDATE", message)
	case errors.Is(err, appruns.ErrInvalidRequest), errors.Is(err, appbuilders.ErrInvalidRequest):
		writeInternalError(w, http.StatusBadRequest, "VALIDATION_INVALID_REQUEST", message)
	case strings.Contains(message, "init workspace"), strings.Contains(message, "init builders service"), strings.Contains(message, "init runs service"):
		writeInternalError(w, http.StatusInternalServerError, "APPFACTORY_INIT_FAILED", message)
	case strings.Contains(message, "write job record"), strings.Contains(message, "marshal job record"), strings.Contains(message, "read prd approval"), strings.Contains(message, "read template approval"):
		writeInternalError(w, http.StatusInternalServerError, "JOB_START_FAILED", message)
	default:
		writeInternalError(w, http.StatusInternalServerError, "JOB_START_FAILED", message)
	}
}

func writeJobCancelError(w http.ResponseWriter, err error) {
	message := err.Error()
	switch {
	case strings.Contains(message, "job ") && strings.Contains(message, "not found"):
		writeInternalError(w, http.StatusNotFound, "JOB_NOT_FOUND", message)
	case strings.Contains(message, "already terminal"), strings.Contains(message, "already has terminal run"):
		writeInternalError(w, http.StatusConflict, "JOB_CANCEL_CONFLICT", message)
	case errors.Is(err, appruns.ErrRunNotFound):
		writeInternalError(w, http.StatusNotFound, "BUILD_RUN_NOT_FOUND", message)
	case errors.Is(err, appruns.ErrTerminalRun), errors.Is(err, appbuilders.ErrLeaseNotFound):
		writeInternalError(w, http.StatusConflict, "JOB_CANCEL_CONFLICT", message)
	case errors.Is(err, appruns.ErrInvalidRequest), errors.Is(err, appbuilders.ErrInvalidRequest):
		writeInternalError(w, http.StatusBadRequest, "VALIDATION_INVALID_REQUEST", message)
	case strings.Contains(message, "init workspace"), strings.Contains(message, "init builders service"), strings.Contains(message, "init runs service"):
		writeInternalError(w, http.StatusInternalServerError, "APPFACTORY_INIT_FAILED", message)
	case strings.Contains(message, "write job record"), strings.Contains(message, "marshal job record"), strings.Contains(message, "read job record"), strings.Contains(message, "decode job record"):
		writeInternalError(w, http.StatusInternalServerError, "JOB_CANCEL_FAILED", message)
	default:
		writeInternalError(w, http.StatusInternalServerError, "JOB_CANCEL_FAILED", message)
	}
}

func writeJobResumeError(w http.ResponseWriter, err error) {
	message := err.Error()
	switch {
	case strings.Contains(message, "job ") && strings.Contains(message, "not found"):
		writeInternalError(w, http.StatusNotFound, "JOB_NOT_FOUND", message)
	case strings.Contains(message, "has no failed run to resume"), strings.Contains(message, "cannot be resumed"), strings.Contains(message, "is still running"), strings.Contains(message, "active execution"):
		writeInternalError(w, http.StatusConflict, "JOB_RESUME_CONFLICT", message)
	case strings.Contains(message, "approvals not ready"):
		writeInternalError(w, http.StatusConflict, "JOB_RESUME_CONFLICT", message)
	case errors.Is(err, appbuilders.ErrNoBuilderCandidate), errors.Is(err, appbuilders.ErrBuilderAlreadyLeased):
		writeInternalError(w, http.StatusConflict, "WORKER_NO_CANDIDATE", message)
	case errors.Is(err, appruns.ErrInvalidRequest), errors.Is(err, appbuilders.ErrInvalidRequest), strings.Contains(message, "unsupported resume_mode"):
		writeInternalError(w, http.StatusBadRequest, "VALIDATION_INVALID_REQUEST", message)
	case strings.Contains(message, "init workspace"), strings.Contains(message, "init builders service"), strings.Contains(message, "init runs service"):
		writeInternalError(w, http.StatusInternalServerError, "APPFACTORY_INIT_FAILED", message)
	case strings.Contains(message, "write job record"), strings.Contains(message, "marshal job record"), strings.Contains(message, "read prd approval"), strings.Contains(message, "read template approval"):
		writeInternalError(w, http.StatusInternalServerError, "JOB_RESUME_FAILED", message)
	default:
		writeInternalError(w, http.StatusInternalServerError, "JOB_RESUME_FAILED", message)
	}
}

func writeTemplateApprovalError(w http.ResponseWriter, err error) {
	message := err.Error()
	switch {
	case strings.Contains(message, "prepared prd mismatch"):
		writeInternalError(w, http.StatusBadRequest, "VALIDATION_INVALID_REQUEST", message)
	case strings.Contains(message, "load prepared prd"):
		writeInternalError(w, http.StatusNotFound, "PRD_NOT_FOUND", message)
	case strings.Contains(message, "init workspace"):
		writeInternalError(w, http.StatusInternalServerError, "APPFACTORY_INIT_FAILED", message)
	case strings.Contains(message, "persist template approval"), strings.Contains(message, "write approval record"), strings.Contains(message, "write approval snapshot"), strings.Contains(message, "marshal approval record"), strings.Contains(message, "decode prepared prd"):
		writeInternalError(w, http.StatusInternalServerError, "APPROVAL_WRITE_FAILED", message)
	default:
		writeInternalError(w, http.StatusInternalServerError, "APPROVAL_SUBMIT_FAILED", message)
	}
}

func writePRDApprovalError(w http.ResponseWriter, err error) {
	message := err.Error()
	switch {
	case strings.Contains(message, "prepared prd mismatch"):
		writeInternalError(w, http.StatusBadRequest, "VALIDATION_INVALID_REQUEST", message)
	case strings.Contains(message, "load prepared prd"):
		writeInternalError(w, http.StatusNotFound, "PRD_NOT_FOUND", message)
	case strings.Contains(message, "init workspace"):
		writeInternalError(w, http.StatusInternalServerError, "APPFACTORY_INIT_FAILED", message)
	case strings.Contains(message, "persist prd approval"), strings.Contains(message, "write approval record"), strings.Contains(message, "write approval snapshot"), strings.Contains(message, "marshal approval record"), strings.Contains(message, "decode prepared prd"):
		writeInternalError(w, http.StatusInternalServerError, "APPROVAL_WRITE_FAILED", message)
	default:
		writeInternalError(w, http.StatusInternalServerError, "APPROVAL_SUBMIT_FAILED", message)
	}
}

func writeApprovalError(w http.ResponseWriter, err error) {
	message := err.Error()
	switch {
	case strings.Contains(message, "already decided"):
		writeInternalError(w, http.StatusConflict, "APPROVAL_ALREADY_DECIDED", message)
	case strings.Contains(message, "not found"):
		writeInternalError(w, http.StatusNotFound, "APPROVAL_NOT_FOUND", message)
	case strings.Contains(message, "unsupported approval_type"), strings.Contains(message, "unsupported decision"), strings.Contains(message, "subject_version is required"), strings.Contains(message, "job_id is required"), strings.Contains(message, "prepared prd mismatch"), strings.Contains(message, "prepared template mismatch"):
		writeInternalError(w, http.StatusBadRequest, "VALIDATION_INVALID_REQUEST", message)
	case strings.Contains(message, "init workspace"):
		writeInternalError(w, http.StatusInternalServerError, "APPFACTORY_INIT_FAILED", message)
	case strings.Contains(message, "create approvals dir"), strings.Contains(message, "write approval record"), strings.Contains(message, "write approval snapshot"), strings.Contains(message, "marshal approval record"):
		writeInternalError(w, http.StatusInternalServerError, "APPROVAL_WRITE_FAILED", message)
	case strings.Contains(message, "read approval record"), strings.Contains(message, "glob approval records"):
		writeInternalError(w, http.StatusInternalServerError, "APPROVAL_READ_FAILED", message)
	default:
		writeInternalError(w, http.StatusInternalServerError, "APPROVAL_OPERATION_FAILED", message)
	}
}

func writeNotificationError(w http.ResponseWriter, err error) {
	message := err.Error()
	switch {
	case strings.Contains(message, "not found"):
		writeInternalError(w, http.StatusNotFound, "NOTIFICATION_NOT_FOUND", message)
	case strings.Contains(message, "init workspace"):
		writeInternalError(w, http.StatusInternalServerError, "APPFACTORY_INIT_FAILED", message)
	case strings.Contains(message, "glob approval index"), strings.Contains(message, "glob approval snapshots"), strings.Contains(message, "glob job records"), strings.Contains(message, "read approval record"):
		writeInternalError(w, http.StatusInternalServerError, "NOTIFICATION_READ_FAILED", message)
	default:
		writeInternalError(w, http.StatusInternalServerError, "NOTIFICATION_READ_FAILED", message)
	}
}

func (h *Handler) executePreparedRunLocally(ctx context.Context, runsSvc runService, builderSvc builderService, runID string) (appruns.RunRecord, error) {
	runner := h.newAppFactoryRunner(runsSvc)
	if err := runner.ExecuteRun(ctx, runID); err != nil {
		return appruns.RunRecord{}, err
	}
	run, err := runsSvc.Get(ctx, runID)
	if err != nil {
		return appruns.RunRecord{}, err
	}
	releaseReason := "run completed"
	if run.Status == appruns.StatusFailed {
		releaseReason = "run failed"
	}
	if run.Status == appruns.StatusCancelled {
		releaseReason = "run cancelled"
	}
	if err := builderSvc.ReleaseByRun(ctx, runID, releaseReason); err != nil {
		return appruns.RunRecord{}, err
	}
	return run, nil
}

func (h *Handler) newAppFactoryRunner(runsSvc runService) *appadapter.Runner {
	if h != nil && h.appFactory.runnerFactory != nil {
		return h.appFactory.runnerFactory(runsSvc)
	}
	return appadapter.NewRunnerWithBackend(runServiceRunnerBackend{runsSvc: runsSvc})
}

type runServiceRunnerBackend struct {
	runsSvc runService
}

func (backend runServiceRunnerBackend) GetRun(ctx context.Context, runID string) (appruns.RunRecord, error) {
	return backend.runsSvc.Get(ctx, runID)
}

func (backend runServiceRunnerBackend) Heartbeat(ctx context.Context, runID string, heartbeat appruns.Heartbeat) error {
	_, err := backend.runsSvc.Heartbeat(ctx, runID, heartbeat)
	return err
}

func (backend runServiceRunnerBackend) Complete(ctx context.Context, runID string, output appruns.BuildOutput) error {
	_, err := backend.runsSvc.Complete(ctx, runID, output)
	return err
}

func (backend runServiceRunnerBackend) Fail(ctx context.Context, runID string, report appruns.FailureReport) error {
	_, err := backend.runsSvc.Fail(ctx, runID, report)
	return err
}

func (backend runServiceRunnerBackend) IndexArtifacts(ctx context.Context, runID string, manifest appruns.ArtifactManifest) error {
	_, err := backend.runsSvc.IndexArtifacts(ctx, runID, manifest)
	return err
}

func (backend runServiceRunnerBackend) IndexMetrics(ctx context.Context, runID string, metrics appruns.Metrics) error {
	_, err := backend.runsSvc.IndexMetrics(ctx, runID, metrics)
	return err
}

func buildRunExecutionCommand(ctx context.Context, run appruns.RunRecord, argv []string) (*exec.Cmd, error) {
	if len(argv) == 0 || strings.TrimSpace(argv[0]) == "" {
		return nil, fmt.Errorf("run %s has no launch command", run.RunID)
	}
	if run.ExecutorImage != "" {
		return dockerRunCommand(ctx, run, argv), nil
	}
	env, err := localLaunchExecutionEnv(run)
	if err != nil {
		return nil, err
	}
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Dir = run.WorkspacePath
	cmd.Env = append(os.Environ(), env...)
	cmd.Env = append(cmd.Env, executionDeadlineEnv(ctx)...)
	return cmd, nil
}

func executeAcceptanceChecks(ctx context.Context, runsSvc runService, run appruns.RunRecord) ([]appruns.CheckResult, []appruns.CheckResult, error) {
	passed := make([]appruns.CheckResult, 0, len(run.AcceptanceChecks))
	failed := make([]appruns.CheckResult, 0)
	iteration := maxInt(run.IterationCount, 1)
	for _, check := range run.AcceptanceChecks {
		commands := compactAcceptanceCheckCommands(check.Commands)
		if len(commands) == 0 {
			continue
		}
		if _, err := runsSvc.Heartbeat(ctx, run.RunID, appruns.Heartbeat{
			Stage:       check.Stage,
			Iteration:   iteration,
			Summary:     "running acceptance check " + check.CheckID,
			TotalTokens: run.TotalTokens,
		}); err != nil {
			return passed, failed, err
		}
		cmd, err := buildRunExecutionCommand(ctx, run, []string{"/bin/sh", "-lc", strings.Join(commands, "\n")})
		if err != nil {
			return passed, failed, err
		}
		output, execErr := cmd.CombinedOutput()
		details := summarizeCheckOutput(output)
		if execErr != nil {
			result := appruns.CheckResult{
				CheckID: check.CheckID,
				Label:   check.Label,
				Stage:   check.Stage,
				Outcome: "failed",
				Details: details,
			}
			failed = append(failed, result)
			if check.Required && !check.AllowFailure {
				summary := "acceptance check failed: " + check.CheckID
				if details != "" {
					summary += ": " + details
				}
				return passed, failed, fmt.Errorf("%s", summary)
			}
			continue
		}
		passed = append(passed, appruns.CheckResult{
			CheckID: check.CheckID,
			Label:   check.Label,
			Stage:   check.Stage,
			Outcome: "passed",
			Details: details,
		})
	}
	return passed, failed, nil
}

func compactAcceptanceCheckCommands(commands []string) []string {
	result := make([]string, 0, len(commands))
	for _, command := range commands {
		trimmed := strings.TrimSpace(command)
		if trimmed == "" {
			continue
		}
		result = append(result, trimmed)
	}
	return result
}

func summarizeCheckOutput(output []byte) string {
	trimmed := strings.TrimSpace(string(output))
	if trimmed == "" {
		return ""
	}
	trimmed = strings.ReplaceAll(trimmed, "\r", "")
	if len(trimmed) > 500 {
		return strings.TrimSpace(trimmed[:500])
	}
	return trimmed
}

type reviewPreparationOutcome struct {
	Duration time.Duration
	ExecErr  error
}

type preparedRunReviewArtifacts struct {
	ChangeSummaryPath    string
	BuildReportPath      string
	SmokeTestReportPath  string
	ReviewMetadataPath   string
	ReviewBundlePath     string
	HandoffChecklistPath string
	ArtifactManifestPath string
	MetricsPath          string
}

type reviewBundleMetadata struct {
	SchemaVersion        string  `json:"schema_version"`
	JobID                string  `json:"job_id"`
	RunID                string  `json:"run_id"`
	Status               string  `json:"status"`
	BuilderID            string  `json:"builder_id"`
	WorkerID             string  `json:"worker_id"`
	DurationSeconds      float64 `json:"duration_seconds"`
	RiskLevel            string  `json:"risk_level"`
	Conclusion           string  `json:"conclusion"`
	BuilderLogPath       string  `json:"builder_log_path"`
	ChangeSummaryPath    string  `json:"change_summary_path"`
	BuildReportPath      string  `json:"build_report_path"`
	SmokeTestReportPath  string  `json:"smoke_test_report_path"`
	ReviewBundlePath     string  `json:"review_bundle_path"`
	HandoffChecklistPath string  `json:"handoff_checklist_path"`
	ArtifactManifestPath string  `json:"artifact_manifest_path"`
	MetricsPath          string  `json:"metrics_path"`
	FailureSummary       string  `json:"failure_summary,omitempty"`
	GeneratedAt          string  `json:"generated_at"`
}

type deliveryRecord struct {
	SchemaVersion       string   `json:"schema_version"`
	JobID               string   `json:"job_id"`
	RunID               string   `json:"run_id"`
	Status              string   `json:"status"`
	Summary             string   `json:"summary,omitempty"`
	NextAction          string   `json:"next_action,omitempty"`
	ReleaseChannel      string   `json:"release_channel,omitempty"`
	RolloutPercent      int      `json:"rollout_percent,omitempty"`
	ReviewerID          string   `json:"reviewer_id"`
	EvidencePaths       []string `json:"evidence_paths,omitempty"`
	RequiredChanges     []string `json:"required_changes,omitempty"`
	SignedArtifactPaths []string `json:"signed_artifact_paths,omitempty"`
	RecordedAt          string   `json:"recorded_at"`
}

func deriveReviewPreparationOutcome(run appruns.RunRecord) reviewPreparationOutcome {
	outcome := reviewPreparationOutcome{}
	if !run.StartedAt.IsZero() {
		finishedAt := run.UpdatedAt
		if !run.FinishedAt.IsZero() {
			finishedAt = run.FinishedAt
		}
		if finishedAt.After(run.StartedAt) {
			outcome.Duration = finishedAt.Sub(run.StartedAt)
		}
	}
	if run.Status == appruns.StatusFailed {
		message := strings.TrimSpace(run.FailureSummary)
		if message == "" {
			message = "run failed before review preparation"
		}
		outcome.ExecErr = errors.New(message)
	}
	return outcome
}

func prepareRunReviewArtifacts(ctx context.Context, runsSvc runService, run appruns.RunRecord, outcome reviewPreparationOutcome) (preparedRunReviewArtifacts, error) {
	jobRoot := filepath.Dir(run.WorkspacePath)
	reportsDir := filepath.Join(jobRoot, "reports")
	if err := os.MkdirAll(reportsDir, 0o755); err != nil {
		return preparedRunReviewArtifacts{}, fmt.Errorf("create reports dir: %w", err)
	}
	changeSummaryPath := filepath.Join(reportsDir, "change-summary.md")
	buildReportPath := filepath.Join(reportsDir, "build-report.md")
	smokeReportPath := filepath.Join(reportsDir, "smoke-test-report.md")
	reviewBundlePath := filepath.Join(reportsDir, "review-bundle.md")
	handoffChecklistPath := filepath.Join(reportsDir, "handoff-checklist.md")
	reviewMetadataPath := filepath.Join(reportsDir, "review-bundle.metadata.json")
	prepared := preparedRunReviewArtifacts{
		ChangeSummaryPath:    relFromJobRoot(jobRoot, changeSummaryPath),
		BuildReportPath:      relFromJobRoot(jobRoot, buildReportPath),
		SmokeTestReportPath:  relFromJobRoot(jobRoot, smokeReportPath),
		ReviewMetadataPath:   relFromJobRoot(jobRoot, reviewMetadataPath),
		ReviewBundlePath:     relFromJobRoot(jobRoot, reviewBundlePath),
		HandoffChecklistPath: relFromJobRoot(jobRoot, handoffChecklistPath),
		ArtifactManifestPath: artifactManifestPathForRun(run),
		MetricsPath:          metricsPathForRun(run),
	}
	if err := fileutil.WriteFileAtomic(changeSummaryPath, []byte("# 变更摘要\n\nP0 server-side run executed.\n"), 0o600); err != nil {
		return preparedRunReviewArtifacts{}, err
	}
	if err := fileutil.WriteFileAtomic(buildReportPath, []byte("# 构建报告\n\nServer-side runner finished launch script.\n"), 0o600); err != nil {
		return preparedRunReviewArtifacts{}, err
	}
	if err := fileutil.WriteFileAtomic(smokeReportPath, []byte("# 冒烟报告\n\nSmoke test not yet implemented in P0.\n"), 0o600); err != nil {
		return preparedRunReviewArtifacts{}, err
	}
	reviewMetadataContent, err := json.MarshalIndent(buildReviewBundleMetadata(run, outcome.Duration, outcome.ExecErr, prepared), "", "  ")
	if err != nil {
		return preparedRunReviewArtifacts{}, fmt.Errorf("marshal review bundle metadata: %w", err)
	}
	reviewMetadataContent = append(reviewMetadataContent, '\n')
	if err := fileutil.WriteFileAtomic(reviewMetadataPath, reviewMetadataContent, 0o600); err != nil {
		return preparedRunReviewArtifacts{}, err
	}
	reviewBundleContent := buildReviewBundleMarkdown(run, outcome.Duration, outcome.ExecErr, prepared.ChangeSummaryPath, prepared.BuildReportPath, prepared.SmokeTestReportPath, prepared.ArtifactManifestPath, prepared.MetricsPath, prepared.HandoffChecklistPath)
	if err := fileutil.WriteFileAtomic(reviewBundlePath, []byte(reviewBundleContent), 0o600); err != nil {
		return preparedRunReviewArtifacts{}, err
	}
	handoffChecklistContent := buildHandoffChecklistMarkdown(run, outcome.ExecErr, prepared.ReviewBundlePath, prepared.ChangeSummaryPath, prepared.BuildReportPath, prepared.SmokeTestReportPath, prepared.ArtifactManifestPath, prepared.MetricsPath)
	if err := fileutil.WriteFileAtomic(handoffChecklistPath, []byte(handoffChecklistContent), 0o600); err != nil {
		return preparedRunReviewArtifacts{}, err
	}
	artifactManifest := appruns.ArtifactManifest{
		SchemaVersion: "0.1.0",
		JobID:         run.JobID,
		GeneratedAt:   time.Now().UTC(),
		Items: []appruns.ArtifactItem{{
			ArtifactID:   "run-log",
			Path:         run.LogPath,
			ArtifactType: "log",
			Produced:     true,
			Required:     true,
			Label:        "builder run log",
			Description:  "执行器运行日志",
		}, {
			ArtifactID:   "change-summary",
			Path:         prepared.ChangeSummaryPath,
			ArtifactType: "report",
			Produced:     true,
			Required:     true,
			Label:        "change summary",
			Description:  "关键变更摘要",
		}, {
			ArtifactID:   "build-report",
			Path:         prepared.BuildReportPath,
			ArtifactType: "report",
			Produced:     true,
			Required:     true,
			Label:        "build report",
			Description:  "构建执行结果摘要",
		}, {
			ArtifactID:   "smoke-test-report",
			Path:         prepared.SmokeTestReportPath,
			ArtifactType: "report",
			Produced:     true,
			Required:     true,
			Label:        "smoke test report",
			Description:  "最小验证结论",
		}, {
			ArtifactID:   "review-bundle-metadata",
			Path:         prepared.ReviewMetadataPath,
			ArtifactType: "other",
			Produced:     true,
			Required:     true,
			Label:        "review bundle metadata",
			Description:  "review / handoff sidecar metadata",
		}, {
			ArtifactID:   "review-bundle",
			Path:         prepared.ReviewBundlePath,
			ArtifactType: "report",
			Produced:     true,
			Required:     true,
			Label:        "review bundle",
			Description:  "人工 review 汇总包",
		}, {
			ArtifactID:   "handoff-checklist",
			Path:         prepared.HandoffChecklistPath,
			ArtifactType: "checklist",
			Produced:     true,
			Required:     true,
			Label:        "handoff checklist",
			Description:  "人工交接动作清单",
		}},
	}
	if _, err := os.Stat(filepath.Join(reportsDir, "delivery-record.json")); err == nil {
		artifactManifest.Items = append(artifactManifest.Items, deliveryRecordArtifactItem())
	}
	metrics := appruns.Metrics{
		SchemaVersion:     "0.1.0",
		JobID:             run.JobID,
		TotalIterations:   maxInt(run.IterationCount, 1),
		TotalTokens:       run.TotalTokens,
		DurationSeconds:   outcome.Duration.Seconds(),
		CommandRuns:       len(run.LaunchArgs),
		FailureSignatures: []appruns.FailureSignature{},
	}
	if outcome.ExecErr != nil {
		metrics.FailureSignatures = []appruns.FailureSignature{{
			Signature:   "runner_exit_nonzero",
			Count:       1,
			LastStage:   "baseline",
			SampleError: outcome.ExecErr.Error(),
		}}
	}
	if _, err := runsSvc.IndexArtifacts(ctx, run.RunID, artifactManifest); err != nil {
		return preparedRunReviewArtifacts{}, err
	}
	if _, err := runsSvc.IndexMetrics(ctx, run.RunID, metrics); err != nil {
		return preparedRunReviewArtifacts{}, err
	}
	return prepared, nil
}

func buildReviewBundleMetadata(run appruns.RunRecord, duration time.Duration, execErr error, prepared preparedRunReviewArtifacts) reviewBundleMetadata {
	riskLevel := "medium"
	conclusion := "needs_manual_review"
	failureSummary := ""
	if execErr != nil {
		riskLevel = "high"
		conclusion = "failed"
		failureSummary = strings.TrimSpace(execErr.Error())
	}
	return reviewBundleMetadata{
		SchemaVersion:        "0.1.0",
		JobID:                run.JobID,
		RunID:                run.RunID,
		Status:               string(run.Status),
		BuilderID:            run.BuilderID,
		WorkerID:             run.WorkerID,
		DurationSeconds:      duration.Seconds(),
		RiskLevel:            riskLevel,
		Conclusion:           conclusion,
		BuilderLogPath:       run.LogPath,
		ChangeSummaryPath:    prepared.ChangeSummaryPath,
		BuildReportPath:      prepared.BuildReportPath,
		SmokeTestReportPath:  prepared.SmokeTestReportPath,
		ReviewBundlePath:     prepared.ReviewBundlePath,
		HandoffChecklistPath: prepared.HandoffChecklistPath,
		ArtifactManifestPath: prepared.ArtifactManifestPath,
		MetricsPath:          prepared.MetricsPath,
		FailureSummary:       failureSummary,
		GeneratedAt:          time.Now().UTC().Format(time.RFC3339),
	}
}

func relFromJobRoot(jobRoot, path string) string {
	rel, err := filepath.Rel(jobRoot, path)
	if err != nil {
		return filepath.ToSlash(path)
	}
	return filepath.ToSlash(rel)
}

func buildReviewBundleMarkdown(run appruns.RunRecord, duration time.Duration, execErr error, changeSummaryPath, buildReportPath, smokeReportPath, artifactManifestPath, metricsPath, handoffChecklistPath string) string {
	var builder strings.Builder
	builder.WriteString("# Review Bundle\n\n")
	builder.WriteString("## 交付概览\n\n")
	builder.WriteString("- Job ID: " + run.JobID + "\n")
	builder.WriteString("- Run ID: " + run.RunID + "\n")
	builder.WriteString("- Worker ID: " + run.WorkerID + "\n")
	builder.WriteString("- Builder ID: " + run.BuilderID + "\n")
	builder.WriteString("- Duration: " + fmt.Sprintf("%.1fs", duration.Seconds()) + "\n")
	builder.WriteString("- Builder Log: " + run.LogPath + "\n\n")

	builder.WriteString("## 风险摘要\n\n")
	if execErr != nil {
		builder.WriteString("- 等级：high\n")
		builder.WriteString("- 结论：runner 退出失败，当前交付包仅供排障，不应直接 handoff。\n")
		builder.WriteString("- 原因：" + execErr.Error() + "\n\n")
	} else {
		builder.WriteString("- 等级：medium\n")
		builder.WriteString("- 结论：server-side runner 已完成，但当前仍缺少真实 smoke / manual 验证，仍需人工 review。\n")
		builder.WriteString("- 关注点：确认关键报告、artifact 与日志之间没有相互矛盾。\n\n")
	}

	builder.WriteString("## 关键变更\n\n")
	builder.WriteString("- 已生成 change summary：" + changeSummaryPath + "\n")
	builder.WriteString("- 已生成 build report：" + buildReportPath + "\n")
	builder.WriteString("- 已生成 smoke report：" + smokeReportPath + "\n")
	builder.WriteString("- 已生成 handoff checklist：" + handoffChecklistPath + "\n\n")

	builder.WriteString("## 验证结论\n\n")
	if execErr != nil {
		builder.WriteString("- Baseline runner exit：failed\n")
		builder.WriteString("- Smoke checks：not verified\n")
		builder.WriteString("- Recommendation：先查看日志与 build report，再决定是否重跑。\n\n")
	} else {
		builder.WriteString("- Baseline runner exit：passed\n")
		builder.WriteString("- Smoke checks：pending manual follow-up\n")
		builder.WriteString("- Recommendation：进入人工 review / handoff，确认 artifact 与报告一致。\n\n")
	}

	builder.WriteString("## Artifact 深链\n\n")
	builder.WriteString("- Artifact Manifest: " + artifactManifestPath + "\n")
	builder.WriteString("- Metrics: " + metricsPath + "\n")
	builder.WriteString("- Change Summary: " + changeSummaryPath + "\n")
	builder.WriteString("- Build Report: " + buildReportPath + "\n")
	builder.WriteString("- Smoke Test Report: " + smokeReportPath + "\n")
	builder.WriteString("- Handoff Checklist: " + handoffChecklistPath + "\n\n")

	builder.WriteString("## 人工交接动作\n\n")
	builder.WriteString("- 逐项核对上述 artifact 是否存在且内容一致。\n")
	builder.WriteString("- 如果需要发布或测试，先完成 handoff checklist。\n")
	builder.WriteString("- 如果发现报告与实际产物不一致，回退到日志与运行记录排障。\n")
	return builder.String()
}

func buildHandoffChecklistMarkdown(run appruns.RunRecord, execErr error, reviewBundlePath, changeSummaryPath, buildReportPath, smokeReportPath, artifactManifestPath, metricsPath string) string {
	var builder strings.Builder
	builder.WriteString("# Handoff Checklist\n\n")
	builder.WriteString("## 上下文\n\n")
	builder.WriteString("- Job ID: " + run.JobID + "\n")
	builder.WriteString("- Run ID: " + run.RunID + "\n")
	builder.WriteString("- Review Bundle: " + reviewBundlePath + "\n\n")

	builder.WriteString("## 必做检查\n\n")
	builder.WriteString("- [ ] 阅读 " + reviewBundlePath + "\n")
	builder.WriteString("- [ ] 阅读 " + changeSummaryPath + "\n")
	builder.WriteString("- [ ] 阅读 " + buildReportPath + "\n")
	builder.WriteString("- [ ] 阅读 " + smokeReportPath + "\n")
	builder.WriteString("- [ ] 核对 " + artifactManifestPath + " 与 " + metricsPath + "\n")
	builder.WriteString("- [ ] 核对 builder log：" + run.LogPath + "\n\n")

	builder.WriteString("## 交接结论\n\n")
	if execErr != nil {
		builder.WriteString("- [ ] 当前 run 失败，确认是否需要重新触发执行\n")
		builder.WriteString("- [ ] 记录失败原因并明确下一位处理人\n\n")
	} else {
		builder.WriteString("- [ ] 确认可以进入人工验证 / QA / 发布前检查\n")
		builder.WriteString("- [ ] 记录剩余风险与人工 follow-up 动作\n\n")
	}

	builder.WriteString("## Artifact 深链\n\n")
	builder.WriteString("- Review Bundle: " + reviewBundlePath + "\n")
	builder.WriteString("- Artifact Manifest: " + artifactManifestPath + "\n")
	builder.WriteString("- Metrics: " + metricsPath + "\n")
	builder.WriteString("- Builder Log: " + run.LogPath + "\n")
	return builder.String()
}

func maxInt(value, fallback int) int {
	if value > 0 {
		return value
	}
	return fallback
}

func artifactManifestPathForRun(run appruns.RunRecord) string {
	if run.ArtifactManifestPath != "" {
		return run.ArtifactManifestPath
	}
	return filepath.ToSlash(filepath.Join("jobs", run.JobID, "runs", run.RunID, "artifact-manifest.json"))
}

func metricsPathForRun(run appruns.RunRecord) string {
	if run.MetricsPath != "" {
		return run.MetricsPath
	}
	return filepath.ToSlash(filepath.Join("jobs", run.JobID, "runs", run.RunID, "metrics.json"))
}

func normalizeDeliveryStatus(status string) string {
	return strings.TrimSpace(strings.ToLower(status))
}

func normalizeDeliveryReleaseChannel(channel string) string {
	return strings.TrimSpace(strings.ToLower(channel))
}

func isSupportedDeliveryStatus(status string) bool {
	switch normalizeDeliveryStatus(status) {
	case "changes_requested", "approved_for_signing", "signed", "staged", "released":
		return true
	default:
		return false
	}
}

func isSupportedDeliveryReleaseChannel(channel string) bool {
	switch normalizeDeliveryReleaseChannel(channel) {
	case "", "internal", "canary", "production":
		return true
	default:
		return false
	}
}

func defaultDeliveryNextAction(status string) string {
	switch normalizeDeliveryStatus(status) {
	case "changes_requested":
		return "update_subject_and_resubmit"
	case "approved_for_signing":
		return "prepare_signing"
	case "signed":
		return "stage_release"
	case "staged":
		return "verify_staged_release"
	case "released":
		return "monitor_release_feedback"
	default:
		return "inspect_delivery_record"
	}
}

func defaultDeliverySummary(status, releaseChannel string, rolloutPercent int) string {
	switch normalizeDeliveryStatus(status) {
	case "changes_requested":
		return "delivery review requested follow-up changes"
	case "approved_for_signing":
		return "delivery package approved for signing"
	case "signed":
		return "signed build artifacts recorded"
	case "staged":
		if normalizeDeliveryReleaseChannel(releaseChannel) != "" {
			return fmt.Sprintf("release staged on %s channel (%d%% rollout)", normalizeDeliveryReleaseChannel(releaseChannel), rolloutPercent)
		}
		return "release staged for limited rollout"
	case "released":
		if normalizeDeliveryReleaseChannel(releaseChannel) != "" {
			return fmt.Sprintf("release promoted on %s channel", normalizeDeliveryReleaseChannel(releaseChannel))
		}
		return "release promoted for delivery follow-up"
	default:
		return "delivery record updated"
	}
}

func deliveryEventType(status string) string {
	switch normalizeDeliveryStatus(status) {
	case "changes_requested":
		return "delivery_changes_requested"
	case "approved_for_signing":
		return "delivery_approved_for_signing"
	case "signed":
		return "delivery_signed"
	case "staged":
		return "delivery_staged"
	case "released":
		return "delivery_released"
	default:
		return ""
	}
}

func deliveryNotificationDescriptor(status string) (string, string) {
	switch normalizeDeliveryStatus(status) {
	case "changes_requested":
		return "delivery_changes_requested", "update_subject_and_resubmit"
	case "approved_for_signing":
		return "delivery_ready_for_signing", "prepare_signing"
	case "signed":
		return "delivery_signed", "stage_release"
	case "staged":
		return "delivery_staged", "verify_staged_release"
	case "released":
		return "delivery_released", "monitor_release_feedback"
	default:
		return "", ""
	}
}

func deliveryRecordReportPath() string {
	return filepath.ToSlash(filepath.Join("reports", "delivery-record.json"))
}

func deliveryRecordPublicPath(jobID string) string {
	return filepath.ToSlash(filepath.Join("jobs", jobID, "reports", "delivery-record.json"))
}

func deliveryRecordArtifactItem() appruns.ArtifactItem {
	return appruns.ArtifactItem{
		ArtifactID:   "delivery-record",
		Path:         deliveryRecordReportPath(),
		ArtifactType: "other",
		Produced:     true,
		Required:     false,
		Label:        "delivery record",
		Description:  "交付后签名/发布/反馈结构化记录",
	}
}

func publicJobDeliveryContextFromRecord(jobID string, record deliveryRecord) *publicJobDeliveryContext {
	return &publicJobDeliveryContext{
		DeliveryRecordPath:  deliveryRecordPublicPath(jobID),
		Status:              strings.TrimSpace(record.Status),
		Summary:             strings.TrimSpace(record.Summary),
		NextAction:          strings.TrimSpace(record.NextAction),
		ReleaseChannel:      strings.TrimSpace(record.ReleaseChannel),
		RolloutPercent:      record.RolloutPercent,
		ReviewerID:          strings.TrimSpace(record.ReviewerID),
		EvidencePaths:       append([]string(nil), record.EvidencePaths...),
		RequiredChanges:     append([]string(nil), record.RequiredChanges...),
		SignedArtifactPaths: append([]string(nil), record.SignedArtifactPaths...),
		RecordedAt:          strings.TrimSpace(record.RecordedAt),
	}
}

func loadDeliveryRecord(workspace, jobID string) (*deliveryRecord, error) {
	path := filepath.Join(workspace, "appfactory", "jobs", jobID, "reports", "delivery-record.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read delivery record: %w", err)
	}
	var record deliveryRecord
	if err := json.Unmarshal(data, &record); err != nil {
		return nil, fmt.Errorf("decode delivery record: %w", err)
	}
	return &record, nil
}

func writeDeliveryRecord(workspace string, record deliveryRecord) error {
	path := filepath.Join(workspace, "appfactory", "jobs", record.JobID, "reports", "delivery-record.json")
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal delivery record: %w", err)
	}
	data = append(data, '\n')
	if err := fileutil.WriteFileAtomic(path, data, 0o600); err != nil {
		return fmt.Errorf("write delivery record: %w", err)
	}
	return nil
}

func upsertArtifactManifestItem(path, jobID string, item appruns.ArtifactItem) error {
	manifest := appruns.ArtifactManifest{
		SchemaVersion: "0.1.0",
		JobID:         jobID,
		GeneratedAt:   time.Now().UTC(),
		Items:         []appruns.ArtifactItem{},
	}
	data, err := os.ReadFile(path)
	if err == nil {
		if err := json.Unmarshal(data, &manifest); err != nil {
			return fmt.Errorf("decode artifact manifest: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("read artifact manifest: %w", err)
	}
	replaced := false
	for index := range manifest.Items {
		if strings.TrimSpace(manifest.Items[index].ArtifactID) == strings.TrimSpace(item.ArtifactID) {
			manifest.Items[index] = item
			replaced = true
			break
		}
	}
	if !replaced {
		manifest.Items = append(manifest.Items, item)
	}
	manifest.GeneratedAt = time.Now().UTC()
	encoded, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal artifact manifest: %w", err)
	}
	encoded = append(encoded, '\n')
	if err := fileutil.WriteFileAtomic(path, encoded, 0o600); err != nil {
		return fmt.Errorf("write artifact manifest: %w", err)
	}
	return nil
}

func dockerLaunchRunCommand(ctx context.Context, run appruns.RunRecord) *exec.Cmd {
	return dockerRunCommand(ctx, run, append([]string{run.LaunchCommand}, run.LaunchArgs...))
}

func dockerRunCommand(ctx context.Context, run appruns.RunRecord, argv []string) *exec.Cmd {
	root := filepath.Clean(filepath.Join(run.WorkspacePath, "..", "..", ".."))
	useHostNetwork := strings.EqualFold(strings.TrimSpace(os.Getenv("APPFACTORY_BUILDER_DOCKER_NETWORK")), "host")
	args := []string{
		"run", "--rm",
		"--user", strconv.Itoa(os.Getuid()) + ":" + strconv.Itoa(os.Getgid()),
		"-v", root + ":" + root,
		"-w", run.WorkspacePath,
		"-e", "PICOCLAW_RUN_ID=" + run.RunID,
		"-e", "PICOCLAW_JOB_ID=" + run.JobID,
		"-e", "PICOCLAW_ARTIFACT_DIR=" + run.ArtifactDir,
	}
	for _, item := range executionDeadlineEnv(ctx) {
		args = append(args, "-e", item)
	}
	if useHostNetwork {
		args = append(args, "--network", "host")
	} else {
		args = append(args, "--add-host", "host.docker.internal:host-gateway")
	}
	pubCacheDir := resolveBuilderPubCache(root)
	_ = os.MkdirAll(pubCacheDir, 0o755)
	args = append(args, "-e", "PUB_CACHE="+pubCacheDir)
	args = append(args, dockerEnvArgs(
		"APPFACTORY_BUILDER_DOCKER_NETWORK",
		"APPFACTORY_BUILDER_RUNTIME",
		"HTTP_PROXY",
		"HTTPS_PROXY",
		"ALL_PROXY",
		"NO_PROXY",
	)...)
	args = append(args, run.ExecutorImage, "exec")
	args = append(args, argv...)
	return exec.CommandContext(ctx, "docker", args...)
}

func localLaunchExecutionEnv(run appruns.RunRecord) ([]string, error) {
	env := []string{
		"PICOCLAW_RUN_ID=" + run.RunID,
		"PICOCLAW_JOB_ID=" + run.JobID,
		"PICOCLAW_ARTIFACT_DIR=" + run.ArtifactDir,
	}
	root := filepath.Clean(filepath.Join(run.WorkspacePath, "..", "..", ".."))
	pubCacheDir := resolveBuilderPubCache(root)
	if err := os.MkdirAll(pubCacheDir, 0o755); err != nil {
		return nil, fmt.Errorf("create builder pub cache: %w", err)
	}
	env = append(env, "PUB_CACHE="+pubCacheDir)
	return env, nil
}

func executionDeadlineEnv(ctx context.Context) []string {
	deadline, ok := ctx.Deadline()
	if !ok {
		return nil
	}
	return []string{"PICOCLAW_EXECUTION_DEADLINE_UNIX=" + strconv.FormatInt(deadline.UTC().Unix(), 10)}
}

func dockerEnvArgs(names ...string) []string {
	args := []string{}
	for _, name := range names {
		value, ok := os.LookupEnv(name)
		if !ok || strings.TrimSpace(value) == "" {
			continue
		}
		args = append(args, "-e", name+"="+value)
	}
	return args
}

func resolveBuilderPubCache(root string) string {
	return filepath.Join(root, ".runtime", "pub-cache")
}
