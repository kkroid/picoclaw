package runs

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
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

type BuilderRuntimeTaskType string

const (
	BuilderRuntimeTaskTypeSingleFileEdit BuilderRuntimeTaskType = "single_file_edit"
	BuilderRuntimeTaskTypeDualFileWiring BuilderRuntimeTaskType = "dual_file_wiring"
	BuilderRuntimeTaskTypeAnalyzeRepair  BuilderRuntimeTaskType = "analyze_repair"
	BuilderRuntimeTaskTypeTestRepair     BuilderRuntimeTaskType = "test_repair"
	BuilderRuntimeTaskTypeClosureRepair  BuilderRuntimeTaskType = "closure_repair"

	legacyBuilderRuntimeTaskTypeHighRiskRepair BuilderRuntimeTaskType = "high_risk_repair"
)

type BuilderRuntimeModelRef struct {
	Primary   string   `json:"primary,omitempty"`
	Fallbacks []string `json:"fallbacks,omitempty"`
}

type BuilderRuntimeUpgradeThreshold struct {
	MaxAttemptsBeforeUpgrade int  `json:"max_attempts_before_upgrade,omitempty"`
	MaxFilesBeforeUpgrade    int  `json:"max_files_before_upgrade,omitempty"`
	UpgradeOnValidationFail  bool `json:"upgrade_on_validation_fail,omitempty"`
	UpgradeOnPatchParseFail  bool `json:"upgrade_on_patch_parse_fail,omitempty"`
	UpgradeOnScopeViolation  bool `json:"upgrade_on_scope_violation,omitempty"`
}

type BuilderRuntimeTaskRoute struct {
	TaskID      string                 `json:"task_id,omitempty"`
	TaskType    BuilderRuntimeTaskType `json:"task_type,omitempty"`
	RouteSource string                 `json:"route_source,omitempty"`
	Model       BuilderRuntimeModelRef `json:"model,omitempty"`
}

type BuilderRuntimePlan struct {
	Enabled          bool                           `json:"enabled"`
	DefaultModel     BuilderRuntimeModelRef         `json:"default_model,omitempty"`
	UpgradeModel     BuilderRuntimeModelRef         `json:"upgrade_model,omitempty"`
	UpgradeThreshold BuilderRuntimeUpgradeThreshold `json:"upgrade_threshold,omitempty"`
	TaskRoutes       []BuilderRuntimeTaskRoute      `json:"task_routes,omitempty"`
}

func NormalizeBuilderRuntimeTaskType(value string) BuilderRuntimeTaskType {
	switch BuilderRuntimeTaskType(strings.ToLower(strings.TrimSpace(value))) {
	case BuilderRuntimeTaskTypeSingleFileEdit:
		return BuilderRuntimeTaskTypeSingleFileEdit
	case BuilderRuntimeTaskTypeDualFileWiring:
		return BuilderRuntimeTaskTypeDualFileWiring
	case BuilderRuntimeTaskTypeAnalyzeRepair:
		return BuilderRuntimeTaskTypeAnalyzeRepair
	case BuilderRuntimeTaskTypeTestRepair:
		return BuilderRuntimeTaskTypeTestRepair
	case BuilderRuntimeTaskTypeClosureRepair, legacyBuilderRuntimeTaskTypeHighRiskRepair:
		return BuilderRuntimeTaskTypeClosureRepair
	default:
		return ""
	}
}

func NormalizeTaskBundleItem(task TaskBundleItem) TaskBundleItem {
	task.TaskType = task.EffectiveTaskType()
	return task
}

func (task TaskBundleItem) EffectiveTaskType() BuilderRuntimeTaskType {
	if taskType := NormalizeBuilderRuntimeTaskType(string(task.TaskType)); taskType != "" {
		return taskType
	}
	return inferBuilderRuntimeTaskType(task)
}

func inferBuilderRuntimeTaskType(task TaskBundleItem) BuilderRuntimeTaskType {
	text := strings.ToLower(strings.Join([]string{
		task.TaskID,
		task.Title,
		task.Objective,
		strings.Join(task.CompletionCriteria, " "),
		strings.Join(task.TargetPaths, " "),
	}, " "))

	if strings.Contains(text, "closure") || strings.Contains(text, "build apk") || (strings.Contains(text, "analyze") && strings.Contains(text, "test")) {
		return BuilderRuntimeTaskTypeClosureRepair
	}
	if strings.Contains(text, "flutter test") || strings.Contains(text, " test") || strings.Contains(text, "test/") || strings.Contains(text, "_test") {
		return BuilderRuntimeTaskTypeTestRepair
	}
	if strings.Contains(text, "flutter analyze") || strings.Contains(text, "analyze") {
		return BuilderRuntimeTaskTypeAnalyzeRepair
	}
	if task.Category == TaskCategoryValidation {
		return BuilderRuntimeTaskTypeClosureRepair
	}
	if concreteTargetPathCount(task.TargetPaths) <= 1 {
		return BuilderRuntimeTaskTypeSingleFileEdit
	}
	return BuilderRuntimeTaskTypeDualFileWiring
}

func concreteTargetPathCount(paths []string) int {
	count := 0
	for _, item := range paths {
		trimmed := strings.TrimSpace(item)
		if trimmed == "" || strings.ContainsAny(trimmed, "*?[]") {
			continue
		}
		count++
	}
	return count
}

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
	SchemaVersion                  string            `json:"schema_version"`
	JobID                          string            `json:"job_id"`
	PRDID                          string            `json:"prd_id"`
	TemplateID                     string            `json:"template_id"`
	PreparedPRDSubjectVersion      string            `json:"prepared_prd_subject_version,omitempty"`
	PreparedTemplateSubjectVersion string            `json:"prepared_template_subject_version,omitempty"`
	ExecutorImage                  string            `json:"executor_image,omitempty"`
	ContextSourceDir               string            `json:"context_source_dir,omitempty"`
	TemplateSourceDir              string            `json:"template_source_dir,omitempty"`
	WorkspacePath                  string            `json:"workspace_path"`
	ArtifactDir                    string            `json:"artifact_dir"`
	GoalSummary                    string            `json:"goal_summary"`
	TaskBundle                     []TaskBundleItem  `json:"task_bundle"`
	AcceptanceChecks               []AcceptanceCheck `json:"acceptance_checks"`
	AllowedPaths                   []string          `json:"allowed_paths"`
	ProtectedPaths                 []string          `json:"protected_paths"`
	KnowledgePack                  []ProfileSkill    `json:"knowledge_pack,omitempty"`
	CommandProfile                 json.RawMessage   `json:"command_profile"`
	ContextFiles                   json.RawMessage   `json:"context_files"`
	IterationBudget                int               `json:"iteration_budget"`
	TokenBudget                    int               `json:"token_budget"`
	HumanNotes                     json.RawMessage   `json:"human_notes,omitempty"`
}

func BuildInputDigest(input BuildInput) string {
	normalized := input
	normalized.PreparedPRDSubjectVersion = ""
	normalized.PreparedTemplateSubjectVersion = ""
	normalized.ContextSourceDir = ""
	normalized.TemplateSourceDir = ""
	normalized.WorkspacePath = ""
	normalized.ArtifactDir = ""
	normalized.CommandProfile = normalizeDigestRawJSON(normalized.CommandProfile)
	normalized.ContextFiles = normalizeDigestRawJSON(normalized.ContextFiles)
	normalized.HumanNotes = normalizeDigestRawJSON(normalized.HumanNotes)
	data, err := json.Marshal(normalized)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:16])
}

func PreparedInputDigest(input BuildInput, contextSourceDir string) string {
	builderDigest := BuildInputDigest(input)
	contextDigests := digestContextFiles(input, contextSourceDir)
	templateSourceDigest := digestTemplateSource(input)
	if builderDigest == "" && len(contextDigests) == 0 && templateSourceDigest == "" {
		return ""
	}
	if len(contextDigests) == 0 && templateSourceDigest == "" {
		return builderDigest
	}
	payload := struct {
		BuilderInputDigest string            `json:"builder_input_digest,omitempty"`
		ContextDigests     map[string]string `json:"context_digests,omitempty"`
		TemplateDigest     string            `json:"template_digest,omitempty"`
	}{
		BuilderInputDigest: builderDigest,
		ContextDigests:     contextDigests,
		TemplateDigest:     templateSourceDigest,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return builderDigest
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:16])
}

func digestTemplateSource(input BuildInput) string {
	templateDir := resolveTemplateSourceDir(input)
	if strings.TrimSpace(templateDir) == "" {
		return ""
	}
	info, err := os.Stat(templateDir)
	if err != nil {
		if os.IsNotExist(err) {
			return "missing"
		}
		return "error"
	}
	if !info.IsDir() {
		return "invalid"
	}
	files := map[string]string{}
	err = filepath.WalkDir(templateDir, func(current string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if current == templateDir {
			return nil
		}
		relPath, err := filepath.Rel(templateDir, current)
		if err != nil {
			return err
		}
		relPath = filepath.ToSlash(relPath)
		if entry.IsDir() {
			if shouldSkipSeedWorkspaceDir(entry.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if shouldSkipSeedWorkspaceFile(entry.Name()) || !entry.Type().IsRegular() {
			return nil
		}
		data, err := os.ReadFile(current)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(data)
		files[relPath] = hex.EncodeToString(sum[:16])
		return nil
	})
	if err != nil {
		return "error"
	}
	data, err := json.Marshal(files)
	if err != nil {
		return "error"
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:16])
}

func resolveTemplateSourceDir(input BuildInput) string {
	templateDir := strings.TrimSpace(input.TemplateSourceDir)
	if templateDir != "" {
		return templateDir
	}
	return defaultTemplateSourceDir(input.TemplateID)
}

func defaultTemplateSourceDir(templateID string) string {
	if strings.TrimSpace(templateID) == "" {
		return ""
	}
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		return ""
	}
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(currentFile), "..", "..", ".."))
	return filepath.Join(repoRoot, "examples", "appfactory", "templates", templateID)
}

func shouldSkipSeedWorkspaceDir(name string) bool {
	switch name {
	case ".dart_tool", ".idea", "build":
		return true
	default:
		return false
	}
}

func shouldSkipSeedWorkspaceFile(name string) bool {
	return strings.HasSuffix(name, ".iml")
}

func digestContextFiles(input BuildInput, contextSourceDir string) map[string]string {
	if strings.TrimSpace(contextSourceDir) == "" {
		return nil
	}
	paths := preparedContextDigestPaths(input)
	if len(paths) == 0 {
		return nil
	}
	result := make(map[string]string, len(paths))
	for _, name := range paths {
		resolvedName := filepath.FromSlash(name)
		resolved := filepath.Clean(filepath.Join(contextSourceDir, resolvedName))
		if filepath.IsAbs(resolvedName) {
			resolved = filepath.Clean(resolvedName)
		}
		data, err := os.ReadFile(resolved)
		if err != nil {
			result[name] = "missing"
			continue
		}
		sum := sha256.Sum256(data)
		result[name] = hex.EncodeToString(sum[:16])
	}
	return result
}

func preparedContextDigestPaths(input BuildInput) []string {
	var files ContextFiles
	if trimmed := bytes.TrimSpace(input.ContextFiles); len(trimmed) > 0 {
		if err := json.Unmarshal(trimmed, &files); err != nil {
			return nil
		}
	}
	seen := make(map[string]struct{})
	paths := make([]string, 0, 6+len(files.SupportingFiles))
	appendPath := func(name string) {
		name = strings.TrimSpace(name)
		if name == "" {
			return
		}
		base := filepath.Base(filepath.FromSlash(name))
		if base == PRDApprovalFileName || base == TemplateApprovalFileName || base == "builder-input.json" || base == "requirement.md" {
			return
		}
		name = filepath.ToSlash(name)
		if _, ok := seen[name]; ok {
			return
		}
		seen[name] = struct{}{}
		paths = append(paths, name)
	}
	appendPath(files.PRDJSONPath)
	appendPath(files.ImplementationPlanPath)
	appendPath(files.ManualConstraintsPath)
	for _, item := range files.SupportingFiles {
		appendPath(item)
	}
	sort.Strings(paths)
	return paths
}

func normalizeDigestRawJSON(raw json.RawMessage) json.RawMessage {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return nil
	}
	var decoded any
	if err := json.Unmarshal(trimmed, &decoded); err != nil {
		return trimmed
	}
	normalized, err := json.Marshal(decoded)
	if err != nil {
		return trimmed
	}
	return normalized
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
	TaskID              string                 `json:"task_id"`
	Title               string                 `json:"title"`
	Category            TaskCategory           `json:"category"`
	TaskType            BuilderRuntimeTaskType `json:"task_type,omitempty"`
	Objective           string                 `json:"objective"`
	Priority            string                 `json:"priority,omitempty"`
	RelatedRequirements []string               `json:"related_requirements,omitempty"`
	Dependencies        []string               `json:"dependencies,omitempty"`
	TargetPaths         []string               `json:"target_paths"`
	OutputExpectations  []string               `json:"output_expectations,omitempty"`
	CompletionCriteria  []string               `json:"completion_criteria"`
	RiskNotes           []string               `json:"risk_notes,omitempty"`
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
	RoundID          string              `json:"round_id"`
	Attempt          int                 `json:"attempt"`
	GoalSummary      string              `json:"goal_summary,omitempty"`
	TaskBundle       []TaskBundleItem    `json:"task_bundle,omitempty"`
	AllowedPaths     []string            `json:"allowed_paths,omitempty"`
	KnowledgePack    []ProfileSkill      `json:"knowledge_pack,omitempty"`
	AcceptanceChecks []AcceptanceCheck   `json:"acceptance_checks,omitempty"`
	BuilderRuntime   *BuilderRuntimePlan `json:"builder_runtime,omitempty"`
}

type RoundOutput struct {
	RoundID           string                        `json:"round_id"`
	Status            string                        `json:"status"`
	Summary           string                        `json:"summary,omitempty"`
	State             *RoundState                   `json:"state,omitempty"`
	BuilderRuntime    *BuilderRuntimeExecutionStats `json:"builder_runtime_execution,omitempty"`
	WorkspacePatch    *WorkspacePatch               `json:"workspace_patch,omitempty"`
	ValidationResults []ValidationResult            `json:"validation_results,omitempty"`
	FailureSignatures []string                      `json:"failure_signatures,omitempty"`
	RepairContext     *RepairContext                `json:"repair_context,omitempty"`
}

type BuilderRuntimeExecutionStats struct {
	Mode                    string                 `json:"mode,omitempty"`
	TaskType                BuilderRuntimeTaskType `json:"task_type,omitempty"`
	RouteSource             string                 `json:"route_source,omitempty"`
	SelectedModel           string                 `json:"selected_model,omitempty"`
	ModelSequence           []string               `json:"model_sequence,omitempty"`
	UpgradeApplied          bool                   `json:"upgrade_applied,omitempty"`
	Attempts                int                    `json:"attempts,omitempty"`
	PromptTokens            int                    `json:"prompt_tokens,omitempty"`
	CompletionTokens        int                    `json:"completion_tokens,omitempty"`
	TotalTokens             int                    `json:"total_tokens,omitempty"`
	OperationCount          int                    `json:"operation_count,omitempty"`
	TargetedOperationCount  int                    `json:"targeted_operation_count,omitempty"`
	UnrelatedOperationCount int                    `json:"unrelated_operation_count,omitempty"`
	UnrelatedOperationRate  float64                `json:"unrelated_operation_rate,omitempty"`
	SchemaNormalized        bool                   `json:"schema_normalized,omitempty"`
	SchemaDriftCount        int                    `json:"schema_drift_count,omitempty"`
	ParseFailureCount       int                    `json:"parse_failure_count,omitempty"`
	ScopeViolationCount     int                    `json:"scope_violation_count,omitempty"`
	FailureReason           string                 `json:"failure_reason,omitempty"`
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
	Start      string `json:"start,omitempty"`
	End        string `json:"end,omitempty"`
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
	SchemaVersion           string                        `json:"schema_version"`
	JobID                   string                        `json:"job_id"`
	TotalIterations         int                           `json:"total_iterations"`
	TotalTokens             int                           `json:"total_tokens"`
	PromptTokens            int                           `json:"prompt_tokens,omitempty"`
	CompletionTokens        int                           `json:"completion_tokens,omitempty"`
	DurationSeconds         float64                       `json:"duration_seconds"`
	CommandRuns             int                           `json:"command_runs"`
	BuilderRuntime          *BuilderRuntimeExecutionStats `json:"builder_runtime,omitempty"`
	SuccessfulChecks        int                           `json:"successful_checks,omitempty"`
	FailedChecks            int                           `json:"failed_checks,omitempty"`
	NoProgressHits          int                           `json:"no_progress_hits,omitempty"`
	ModelRequestRetries     int                           `json:"model_request_retries,omitempty"`
	ModelRequestFailures    int                           `json:"model_request_failures,omitempty"`
	ModelUnavailableSeconds float64                       `json:"model_unavailable_seconds,omitempty"`
	DeviceFailureCategories []DeviceFailureCategoryStat   `json:"device_failure_categories,omitempty"`
	FailureSignatures       []FailureSignature            `json:"failure_signatures"`
}

type DeviceFailureCategoryStat struct {
	Category      string `json:"category"`
	FailureDomain string `json:"failure_domain"`
	Count         int    `json:"count"`
}

type FailureSignature struct {
	Signature   string         `json:"signature"`
	Count       int            `json:"count"`
	LastStage   ExecutionStage `json:"last_stage"`
	LastSeenAt  *time.Time     `json:"last_seen_at,omitempty"`
	SampleError string         `json:"sample_error,omitempty"`
}

type RunRecord struct {
	RunID                   string            `json:"run_id"`
	JobID                   string            `json:"job_id"`
	BuilderID               string            `json:"builder_id"`
	WorkerID                string            `json:"worker_id"`
	LeaseID                 string            `json:"lease_id"`
	Status                  Status            `json:"status"`
	ExecutorImage           string            `json:"executor_image,omitempty"`
	GoalSummary             string            `json:"goal_summary,omitempty"`
	HumanNotes              json.RawMessage   `json:"human_notes,omitempty"`
	TaskBundle              []TaskBundleItem  `json:"task_bundle,omitempty"`
	AcceptanceChecks        []AcceptanceCheck `json:"acceptance_checks,omitempty"`
	AllowedPaths            []string          `json:"allowed_paths,omitempty"`
	ProtectedPaths          []string          `json:"protected_paths,omitempty"`
	KnowledgePack           []ProfileSkill    `json:"knowledge_pack,omitempty"`
	PreparedInputDigest     string            `json:"prepared_input_digest,omitempty"`
	InputPath               string            `json:"input_path"`
	WorkspacePath           string            `json:"workspace_path"`
	ArtifactDir             string            `json:"artifact_dir"`
	RunnerScriptPath        string            `json:"runner_script_path,omitempty"`
	LaunchCommand           string            `json:"launch_command,omitempty"`
	LaunchArgs              []string          `json:"launch_args,omitempty"`
	LogPath                 string            `json:"log_path,omitempty"`
	OutputPath              string            `json:"output_path,omitempty"`
	ArtifactManifestPath    string            `json:"artifact_manifest_path,omitempty"`
	MetricsPath             string            `json:"metrics_path,omitempty"`
	EventsPath              string            `json:"events_path"`
	CreatedAt               time.Time         `json:"created_at"`
	UpdatedAt               time.Time         `json:"updated_at"`
	StartedAt               time.Time         `json:"started_at"`
	FinishedAt              time.Time         `json:"finished_at,omitempty"`
	LastStage               ExecutionStage    `json:"last_stage,omitempty"`
	IterationCount          int               `json:"iteration_count,omitempty"`
	TotalTokens             int               `json:"total_tokens,omitempty"`
	FailureSummary          string            `json:"failure_summary,omitempty"`
	FailureSignatures       []string          `json:"failure_signatures,omitempty"`
	RecoverySuggestion      string            `json:"recovery_suggestion,omitempty"`
	CurrentRoundID          string            `json:"current_round_id,omitempty"`
	CurrentRoundAttempt     int               `json:"current_round_attempt,omitempty"`
	CurrentCheckpointKey    string            `json:"current_checkpoint_key,omitempty"`
	CurrentRoundTargetPaths []string          `json:"current_round_target_paths,omitempty"`
	RoundState              *RoundState       `json:"round_state,omitempty"`
	RepairContext           *RepairContext    `json:"repair_context,omitempty"`
	DistinctFailureCount    int               `json:"distinct_failure_count,omitempty"`
}

type Heartbeat struct {
	Stage             ExecutionStage `json:"stage"`
	Iteration         int            `json:"iteration"`
	EventType         string         `json:"event_type,omitempty"`
	RoundID           string         `json:"round_id,omitempty"`
	Attempt           int            `json:"attempt,omitempty"`
	CheckpointKey     string         `json:"checkpoint_key,omitempty"`
	RoundState        *RoundState    `json:"round_state,omitempty"`
	TargetPaths       []string       `json:"target_paths,omitempty"`
	AffectedPaths     []string       `json:"affected_paths,omitempty"`
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
