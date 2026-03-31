package runs

import (
	"encoding/json"
	"time"
)

type Status string

const (
	StatusRunning   Status = "running"
	StatusCompleted Status = "completed"
	StatusFailed    Status = "failed"
	StatusCancelled Status = "cancelled"
)

type TaskCategory string

const (
	TaskCategoryDomain     TaskCategory = "domain"
	TaskCategoryStorage    TaskCategory = "storage"
	TaskCategoryScreen     TaskCategory = "screen"
	TaskCategoryFlow       TaskCategory = "flow"
	TaskCategoryValidation TaskCategory = "validation"
)

type ExecutionStage string

const (
	StageThinPrepare ExecutionStage = "thin-prepare"
	StageBaseline    ExecutionStage = "baseline"
	StageCheap       ExecutionStage = "cheap"
	StageHeavy       ExecutionStage = "heavy"
	StageMilestone   ExecutionStage = "milestone"
	StageDevice      ExecutionStage = "device"
	StageSmoke       ExecutionStage = "smoke"
	StageManual      ExecutionStage = "manual"
	StageOther       ExecutionStage = "other"
)

const (
	PRDApprovalFileName            = "prd-approval.json"
	TemplateApprovalFileName       = "template-approval.json"
	ApprovalStatusPending          = "pending"
	ApprovalTypePRD                = "prd"
	ApprovalTypeTemplate           = "template"
	ApprovalStatusApproved         = "approved"
	ApprovalStatusRejected         = "rejected"
	ApprovalStatusChangesRequested = "changes_requested"
)

type BuildInput struct {
	SchemaVersion     string            `json:"schema_version"`
	JobID             string            `json:"job_id"`
	PRDID             string            `json:"prd_id"`
	TemplateID        string            `json:"template_id"`
	ExecutorImage     string            `json:"executor_image,omitempty"`
	ContextSourceDir  string            `json:"context_source_dir,omitempty"`
	TemplateSourceDir string            `json:"template_source_dir,omitempty"`
	WorkspacePath     string            `json:"workspace_path"`
	ArtifactDir       string            `json:"artifact_dir"`
	GoalSummary       string            `json:"goal_summary"`
	TaskBundle        []TaskBundleItem  `json:"task_bundle"`
	AcceptanceChecks  []AcceptanceCheck `json:"acceptance_checks"`
	AllowedPaths      []string          `json:"allowed_paths"`
	ProtectedPaths    []string          `json:"protected_paths"`
	KnowledgePack     []ProfileSkill    `json:"knowledge_pack,omitempty"`
	CommandProfile    json.RawMessage   `json:"command_profile"`
	ContextFiles      json.RawMessage   `json:"context_files"`
	IterationBudget   int               `json:"iteration_budget"`
	TokenBudget       int               `json:"token_budget"`
	HumanNotes        json.RawMessage   `json:"human_notes,omitempty"`
}

type CommandProfile struct {
	ProfileName             string   `json:"profile_name"`
	AllowedStages           []string `json:"allowed_stages"`
	AllowedCommands         []string `json:"allowed_commands,omitempty"`
	DeniedCommands          []string `json:"denied_commands,omitempty"`
	MaxSingleCommandSeconds int      `json:"max_single_command_seconds,omitempty"`
	MaxParallelCommands     int      `json:"max_parallel_commands,omitempty"`
	NetworkPolicy           string   `json:"network_policy"`
	WritableRoots           []string `json:"writable_roots,omitempty"`
	EnvAllowlist            []string `json:"env_allowlist,omitempty"`
}

type ContextFiles struct {
	PRDMarkdownPath        string   `json:"prd_markdown_path"`
	PRDJSONPath            string   `json:"prd_json_path"`
	TemplateFitReportPath  string   `json:"template_fit_report_path"`
	ImplementationPlanPath string   `json:"implementation_plan_path"`
	ManualConstraintsPath  string   `json:"manual_constraints_path,omitempty"`
	SupportingFiles        []string `json:"supporting_files,omitempty"`
}

type ApprovalRecord struct {
	SchemaVersion  string            `json:"schema_version"`
	ApprovalID     string            `json:"approval_id"`
	ApprovalType   string            `json:"approval_type"`
	JobID          string            `json:"job_id,omitempty"`
	PRDID          string            `json:"prd_id,omitempty"`
	SubjectVersion string            `json:"subject_version"`
	Status         string            `json:"status"`
	RequestedBy    ApprovalActor     `json:"requested_by"`
	Decision       *ApprovalDecision `json:"decision,omitempty"`
	Summary        string            `json:"summary,omitempty"`
	EvidencePaths  []string          `json:"evidence_paths,omitempty"`
	CreatedAt      string            `json:"created_at"`
}

type ApprovalActor struct {
	ActorType string `json:"actor_type"`
	ActorID   string `json:"actor_id"`
}

type ApprovalDecision struct {
	Decision        string        `json:"decision"`
	DecidedBy       ApprovalActor `json:"decided_by"`
	DecidedAt       string        `json:"decided_at"`
	Comment         string        `json:"comment,omitempty"`
	RequiredChanges []string      `json:"required_changes,omitempty"`
}

type TaskBundleItem struct {
	TaskID              string       `json:"task_id"`
	Title               string       `json:"title"`
	Category            TaskCategory `json:"category"`
	Objective           string       `json:"objective"`
	Priority            string       `json:"priority,omitempty"`
	RelatedRequirements []string     `json:"related_requirements,omitempty"`
	Dependencies        []string     `json:"dependencies,omitempty"`
	TargetPaths         []string     `json:"target_paths"`
	OutputExpectations  []string     `json:"output_expectations,omitempty"`
	CompletionCriteria  []string     `json:"completion_criteria"`
	RiskNotes           []string     `json:"risk_notes,omitempty"`
}

type AcceptanceCheck struct {
	CheckID         string         `json:"check_id"`
	Label           string         `json:"label"`
	Stage           ExecutionStage `json:"stage"`
	Required        bool           `json:"required"`
	Commands        []string       `json:"commands,omitempty"`
	SuccessCriteria string         `json:"success_criteria,omitempty"`
	TimeoutSeconds  int            `json:"timeout_seconds,omitempty"`
	AllowFailure    bool           `json:"allow_failure,omitempty"`
}

type RoundPhase string

const (
	RoundPhaseInspect  RoundPhase = "inspect"
	RoundPhaseEdit     RoundPhase = "edit"
	RoundPhaseValidate RoundPhase = "validate"
	RoundPhaseRepair   RoundPhase = "repair"
	RoundPhaseFinalize RoundPhase = "finalize"
)

type ControlAction string

const (
	ControlActionNone     ControlAction = "none"
	ControlActionStop     ControlAction = "stop"
	ControlActionRetry    ControlAction = "retry"
	ControlActionResume   ControlAction = "resume"
	ControlActionPreserve ControlAction = "preserve"
)

const RepairBudgetOwnerPlatform = "platform"

type RoundInput struct {
	RoundID          string            `json:"round_id"`
	Attempt          int               `json:"attempt"`
	GoalSummary      string            `json:"goal_summary,omitempty"`
	TaskBundle       []TaskBundleItem  `json:"task_bundle,omitempty"`
	AllowedPaths     []string          `json:"allowed_paths,omitempty"`
	KnowledgePack     []ProfileSkill    `json:"knowledge_pack,omitempty"`
	AcceptanceChecks []AcceptanceCheck `json:"acceptance_checks,omitempty"`
}

type RoundOutput struct {
	RoundID           string             `json:"round_id"`
	Status            string             `json:"status"`
	Summary           string             `json:"summary,omitempty"`
	State             *RoundState        `json:"state,omitempty"`
	WorkspacePatch    *WorkspacePatch    `json:"workspace_patch,omitempty"`
	ValidationResults []ValidationResult `json:"validation_results,omitempty"`
	FailureSignatures []string           `json:"failure_signatures,omitempty"`
	RepairContext     *RepairContext     `json:"repair_context,omitempty"`
}

type RoundState struct {
	CurrentPhase      RoundPhase    `json:"current_phase"`
	PhaseTrace        []RoundPhase  `json:"phase_trace,omitempty"`
	NextAction        ControlAction `json:"next_action,omitempty"`
	PreserveWorkspace bool          `json:"preserve_workspace,omitempty"`
	ResumeAllowed     bool          `json:"resume_allowed,omitempty"`
}

type WorkspacePatch struct {
	PatchID       string                    `json:"patch_id"`
	Status        string                    `json:"status"`
	ModifiedFiles []string                  `json:"modified_files,omitempty"`
	Operations    []WorkspacePatchOperation `json:"operations,omitempty"`
}

type WorkspacePatchOperation struct {
	Type       string `json:"type"`
	Path       string `json:"path"`
	Anchor     string `json:"anchor,omitempty"`
	Content    string `json:"content,omitempty"`
	OldContent string `json:"old_content,omitempty"`
	NewContent string `json:"new_content,omitempty"`
}

type WorkspacePatchApplyResult struct {
	PatchID       string       `json:"patch_id"`
	Status        string       `json:"status"`
	ModifiedFiles []FileChange `json:"modified_files,omitempty"`
	AppliedOps    int          `json:"applied_ops,omitempty"`
	FailureReason string       `json:"failure_reason,omitempty"`
}

type ValidationResult struct {
	CheckID       string         `json:"check_id"`
	Label         string         `json:"label,omitempty"`
	Stage         ExecutionStage `json:"stage,omitempty"`
	Outcome       string         `json:"outcome"`
	Summary       string         `json:"summary,omitempty"`
	EvidencePaths []string       `json:"evidence_paths,omitempty"`
	Blocking      bool           `json:"blocking,omitempty"`
}

type RepairContext struct {
	State               *RoundState   `json:"state,omitempty"`
	Budget              *RepairBudget `json:"budget,omitempty"`
	Reason              string        `json:"reason,omitempty"`
	FailedChecks        []string      `json:"failed_checks,omitempty"`
	FailureSignatures   []string      `json:"failure_signatures,omitempty"`
	RecommendedAction   string        `json:"recommended_action,omitempty"`
	PreserveWorkspace   bool          `json:"preserve_workspace,omitempty"`
	RequiresHumanReview bool          `json:"requires_human_review,omitempty"`
}

type RepairBudget struct {
	Owner             string `json:"owner"`
	MaxRounds         int    `json:"max_rounds,omitempty"`
	UsedRounds        int    `json:"used_rounds,omitempty"`
	RemainingRounds   int    `json:"remaining_rounds,omitempty"`
	TerminationReason string `json:"termination_reason,omitempty"`
}

type StackProfile struct {
	ProfileID         string            `json:"profile_id"`
	Stack             string            `json:"stack"`
	WorkspaceDetector WorkspaceDetector `json:"workspace_detector"`
	StructuralChecks  []AcceptanceCheck `json:"structural_checks,omitempty"`
	CommandChecks     []AcceptanceCheck `json:"command_checks,omitempty"`
	CommandProfile    CommandProfile    `json:"command_profile"`
	AllowedPaths      []string          `json:"allowed_paths,omitempty"`
	ProtectedPaths    []string          `json:"protected_paths,omitempty"`
	KnowledgePack     []ProfileSkill    `json:"knowledge_pack,omitempty"`
	Metadata          map[string]string `json:"metadata,omitempty"`
}
type ProfileSkill struct {
	SkillID    string `json:"skill_id"`
	Role       string `json:"role"`
	UsageStage string `json:"usage_stage,omitempty"`
	Scope      string `json:"scope,omitempty"`
	Notes      string `json:"notes,omitempty"`
}

type WorkspaceDetector struct {
	RequiredFiles   []string `json:"required_files,omitempty"`
	RequiredDirs    []string `json:"required_dirs,omitempty"`
	AndroidMarkers  []string `json:"android_markers,omitempty"`
	ForbiddenMarker []string `json:"forbidden_markers,omitempty"`
}

type BuildOutput struct {
	SchemaVersion     string             `json:"schema_version"`
	JobID             string             `json:"job_id"`
	Status            string             `json:"status"`
	ExitReason        string             `json:"exit_reason,omitempty"`
	WorkerID          string             `json:"worker_id,omitempty"`
	StartedAt         *time.Time         `json:"started_at,omitempty"`
	FinishedAt        *time.Time         `json:"finished_at,omitempty"`
	FinalSummary      string             `json:"final_summary"`
	ModifiedFiles     []FileChange       `json:"modified_files"`
	ChecksPassed      []CheckResult      `json:"checks_passed"`
	ChecksFailed      []CheckResult      `json:"checks_failed"`
	Blockers          []Blocker          `json:"blockers,omitempty"`
	NextHumanActions  []HumanAction      `json:"next_human_actions"`
	Artifacts         ArtifactSummary    `json:"artifacts"`
	Metrics           MetricsSummary     `json:"metrics"`
	ReportPaths       ReportPaths        `json:"report_paths"`
	RoundInputs       []RoundInput       `json:"round_inputs,omitempty"`
	RoundOutputs      []RoundOutput      `json:"round_outputs,omitempty"`
	ValidationResults []ValidationResult `json:"validation_results,omitempty"`
	RepairContext     *RepairContext     `json:"repair_context,omitempty"`
	ResumeContext     json.RawMessage    `json:"resume_context,omitempty"`
}

type FileChange struct {
	Path         string   `json:"path"`
	ChangeType   string   `json:"change_type"`
	Summary      string   `json:"summary,omitempty"`
	RelatedTasks []string `json:"related_tasks,omitempty"`
}

type CheckResult struct {
	CheckID       string         `json:"check_id"`
	Label         string         `json:"label"`
	Stage         ExecutionStage `json:"stage"`
	Outcome       string         `json:"outcome"`
	Details       string         `json:"details,omitempty"`
	EvidencePaths []string       `json:"evidence_paths,omitempty"`
}

type Blocker struct {
	BlockerID      string `json:"blocker_id"`
	Severity       string `json:"severity"`
	Summary        string `json:"summary"`
	ResolutionHint string `json:"resolution_hint,omitempty"`
}

type HumanAction struct {
	ActionID       string   `json:"action_id"`
	Summary        string   `json:"summary"`
	Reason         string   `json:"reason"`
	Owner          string   `json:"owner,omitempty"`
	RequiredInputs []string `json:"required_inputs,omitempty"`
}

type ArtifactSummary struct {
	ManifestPath   string            `json:"manifest_path"`
	PrimaryOutputs []ArtifactPointer `json:"primary_outputs"`
}

type ArtifactPointer struct {
	ArtifactID   string `json:"artifact_id"`
	Path         string `json:"path"`
	ArtifactType string `json:"artifact_type"`
	Label        string `json:"label,omitempty"`
}

type MetricsSummary struct {
	MetricsPath               string `json:"metrics_path"`
	TotalIterations           int    `json:"total_iterations"`
	TotalTokens               int    `json:"total_tokens"`
	DistinctFailureSignatures int    `json:"distinct_failure_signatures"`
	NoProgressHits            int    `json:"no_progress_hits,omitempty"`
}

type ReportPaths struct {
	ChangeSummaryPath   string `json:"change_summary_path"`
	BuildReportPath     string `json:"build_report_path"`
	SmokeTestReportPath string `json:"smoke_test_report_path"`
}

type ArtifactManifest struct {
	SchemaVersion string         `json:"schema_version"`
	JobID         string         `json:"job_id"`
	GeneratedAt   time.Time      `json:"generated_at"`
	Items         []ArtifactItem `json:"items"`
}

type ArtifactItem struct {
	ArtifactID   string `json:"artifact_id"`
	Path         string `json:"path"`
	ArtifactType string `json:"artifact_type"`
	Produced     bool   `json:"produced"`
	Required     bool   `json:"required,omitempty"`
	Label        string `json:"label,omitempty"`
	Description  string `json:"description,omitempty"`
	SizeBytes    int64  `json:"size_bytes,omitempty"`
	SHA256       string `json:"sha256,omitempty"`
	ContentType  string `json:"content_type,omitempty"`
}

type Metrics struct {
	SchemaVersion           string             `json:"schema_version"`
	JobID                   string             `json:"job_id"`
	TotalIterations         int                `json:"total_iterations"`
	TotalTokens             int                `json:"total_tokens"`
	PromptTokens            int                `json:"prompt_tokens,omitempty"`
	CompletionTokens        int                `json:"completion_tokens,omitempty"`
	DurationSeconds         float64            `json:"duration_seconds"`
	CommandRuns             int                `json:"command_runs"`
	SuccessfulChecks        int                `json:"successful_checks,omitempty"`
	FailedChecks            int                `json:"failed_checks,omitempty"`
	NoProgressHits          int                `json:"no_progress_hits,omitempty"`
	ModelRequestRetries     int                `json:"model_request_retries,omitempty"`
	ModelRequestFailures    int                `json:"model_request_failures,omitempty"`
	ModelUnavailableSeconds float64            `json:"model_unavailable_seconds,omitempty"`
	FailureSignatures       []FailureSignature `json:"failure_signatures"`
}

type FailureSignature struct {
	Signature   string         `json:"signature"`
	Count       int            `json:"count"`
	LastStage   ExecutionStage `json:"last_stage"`
	LastSeenAt  *time.Time     `json:"last_seen_at,omitempty"`
	SampleError string         `json:"sample_error,omitempty"`
}

type RunRecord struct {
	RunID                string            `json:"run_id"`
	JobID                string            `json:"job_id"`
	BuilderID            string            `json:"builder_id"`
	WorkerID             string            `json:"worker_id"`
	LeaseID              string            `json:"lease_id"`
	Status               Status            `json:"status"`
	ExecutorImage        string            `json:"executor_image,omitempty"`
	GoalSummary          string            `json:"goal_summary,omitempty"`
	TaskBundle           []TaskBundleItem  `json:"task_bundle,omitempty"`
	AcceptanceChecks     []AcceptanceCheck `json:"acceptance_checks,omitempty"`
	AllowedPaths         []string          `json:"allowed_paths,omitempty"`
	ProtectedPaths       []string          `json:"protected_paths,omitempty"`
	KnowledgePack        []ProfileSkill    `json:"knowledge_pack,omitempty"`
	InputPath            string            `json:"input_path"`
	WorkspacePath        string            `json:"workspace_path"`
	ArtifactDir          string            `json:"artifact_dir"`
	RunnerScriptPath     string            `json:"runner_script_path,omitempty"`
	LaunchCommand        string            `json:"launch_command,omitempty"`
	LaunchArgs           []string          `json:"launch_args,omitempty"`
	LogPath              string            `json:"log_path,omitempty"`
	OutputPath           string            `json:"output_path,omitempty"`
	ArtifactManifestPath string            `json:"artifact_manifest_path,omitempty"`
	MetricsPath          string            `json:"metrics_path,omitempty"`
	EventsPath           string            `json:"events_path"`
	CreatedAt            time.Time         `json:"created_at"`
	UpdatedAt            time.Time         `json:"updated_at"`
	StartedAt            time.Time         `json:"started_at"`
	FinishedAt           time.Time         `json:"finished_at,omitempty"`
	LastStage            ExecutionStage    `json:"last_stage,omitempty"`
	IterationCount       int               `json:"iteration_count,omitempty"`
	TotalTokens          int               `json:"total_tokens,omitempty"`
	FailureSummary       string            `json:"failure_summary,omitempty"`
	FailureSignatures    []string          `json:"failure_signatures,omitempty"`
	RecoverySuggestion   string            `json:"recovery_suggestion,omitempty"`
	RoundState           *RoundState       `json:"round_state,omitempty"`
	RepairContext        *RepairContext    `json:"repair_context,omitempty"`
	DistinctFailureCount int               `json:"distinct_failure_count,omitempty"`
}

type Heartbeat struct {
	Stage             ExecutionStage `json:"stage"`
	Iteration         int            `json:"iteration"`
	FailureSignatures []string       `json:"failure_signatures,omitempty"`
	TotalTokens       int            `json:"total_tokens,omitempty"`
	Summary           string         `json:"summary,omitempty"`
}

type FailureReport struct {
	Summary            string         `json:"summary"`
	RecoverySuggestion string         `json:"recovery_suggestion,omitempty"`
	FailureSignatures  []string       `json:"failure_signatures,omitempty"`
	RoundState         *RoundState    `json:"round_state,omitempty"`
	RepairContext      *RepairContext `json:"repair_context,omitempty"`
}
