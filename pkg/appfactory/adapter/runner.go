package adapter

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	appruns "github.com/sipeed/picoclaw/pkg/appfactory/runs"
	"github.com/sipeed/picoclaw/pkg/fileutil"
)

type Runner struct {
	BaseURL    string
	HTTPClient *http.Client
	Now        func() time.Time
	Executor   ThinExecutor
	Backend    RunnerBackend
}

type RunnerBackend interface {
	GetRun(ctx context.Context, runID string) (appruns.RunRecord, error)
	Heartbeat(ctx context.Context, runID string, heartbeat appruns.Heartbeat) error
	Complete(ctx context.Context, runID string, output appruns.BuildOutput) error
	Fail(ctx context.Context, runID string, report appruns.FailureReport) error
	IndexArtifacts(ctx context.Context, runID string, manifest appruns.ArtifactManifest) error
	IndexMetrics(ctx context.Context, runID string, metrics appruns.Metrics) error
}

func NewRunner(baseURL string) *Runner {
	return &Runner{
		BaseURL:    strings.TrimRight(baseURL, "/"),
		HTTPClient: &http.Client{Timeout: 30 * time.Second},
		Now:        func() time.Time { return time.Now().UTC() },
		Executor:   NewThinExecutor(),
	}
}

func NewRunnerWithBackend(backend RunnerBackend) *Runner {
	return &Runner{
		Now:      func() time.Time { return time.Now().UTC() },
		Executor: NewThinExecutor(),
		Backend:  backend,
	}
}

func (runner *Runner) ExecuteRun(ctx context.Context, runID string) error {
	backend := runner.backend()
	rawRun, err := backend.GetRun(ctx, runID)
	if err != nil {
		return err
	}
	run := newRunRecord(rawRun)
	executor := runner.Executor
	if executor == nil {
		executor = NewThinExecutor()
	}
	plan, err := executor.Prepare(ctx, run)
	if err != nil {
		return err
	}
	steps := plan.Steps()
	if len(steps) == 0 {
		return fmt.Errorf("run %s prepared empty round plan", runID)
	}
	if err := validateApprovalSnapshots(run); err != nil {
		repairContext := buildRepairContext(
			err.Error(),
			"regenerate the prepare bundle or restore approved approval snapshots before rerun",
			nil,
			nil,
			buildRoundState([]appruns.RoundPhase{appruns.RoundPhaseInspect, appruns.RoundPhaseFinalize}, appruns.RoundPhaseFinalize, appruns.ControlActionStop, false, false),
			repairFailurePolicy{
				PreserveWorkspace:   false,
				ResumeAllowed:       false,
				RequiresHumanReview: true,
				MaxRounds:           1,
				UsedRounds:          1,
				RemainingRounds:     0,
				TerminationReason:   "platform repair budget exhausted for current run",
			},
		)
		return backend.Fail(ctx, runID, appruns.FailureReport{
			Summary:            err.Error(),
			RecoverySuggestion: "regenerate the prepare bundle or restore approved approval snapshots before rerun",
			FailureSignatures:  []string{"approval_gate_failed"},
			RoundState:         repairContext.State,
			RepairContext:      repairContext,
		})
	}
	startedAt := runner.Now()
	roundInput := plan.RoundInput
	if strings.TrimSpace(roundInput.RoundID) == "" {
		roundInput = buildRoundInput(run)
	}
	roundOutput := appruns.RoundOutput{
		RoundID: roundInput.RoundID,
		Status:  "running",
		State: buildRoundState(
			[]appruns.RoundPhase{appruns.RoundPhaseInspect, appruns.RoundPhaseEdit},
			appruns.RoundPhaseEdit,
			appruns.ControlActionNone,
			false,
			false,
		),
	}
	passedChecks := make([]appruns.CheckResult, 0, len(plan.AcceptanceChecks))
	failedChecks := make([]appruns.CheckResult, 0)
	failureSignatures := make([]appruns.FailureSignature, 0)
	commandRuns := 0
	modifiedFiles := make([]appruns.FileChange, 0)
	failureSummary := ""
	recoverySuggestion := ""
	failureRoundState := (*appruns.RoundState)(nil)
	failurePolicy := repairFailurePolicy{}
	for index, step := range steps {
		if err := backend.Heartbeat(ctx, runID, appruns.Heartbeat{
			Stage:       step.Stage,
			Iteration:   index + 1,
			Summary:     step.Summary,
			TotalTokens: 0,
		}); err != nil {
			return err
		}
		if step.Command == nil {
			continue
		}
		commandRuns++
		execErr := error(nil)
		workspacePatchApplyFailed := false
		if step.Check == nil {
			beforeSnapshot, snapshotErr := captureWorkspaceSnapshot(run.WorkspacePath)
			if snapshotErr != nil {
				execErr = snapshotErr
			} else {
				execErr = runner.runStep(step, run.WorkspacePath, run.LogPath)
				afterSnapshot, afterErr := captureWorkspaceSnapshot(run.WorkspacePath)
				if afterErr != nil {
					execErr = afterErr
				} else {
					capturedPatch, patchErr := buildCapturedWorkspacePatch(roundInput.RoundID, beforeSnapshot, afterSnapshot, run.AllowedPaths, run.ProtectedPaths)
					if restoreErr := restoreWorkspaceSnapshot(run.WorkspacePath, beforeSnapshot, afterSnapshot); restoreErr != nil {
						execErr = restoreErr
					} else if execErr == nil && patchErr != nil {
						execErr = patchErr
					} else if execErr == nil {
						applyResult, applyErr := appruns.ApplyWorkspacePatch(run.WorkspacePath, run.AllowedPaths, run.ProtectedPaths, *capturedPatch)
						if applyErr != nil {
							workspacePatchApplyFailed = true
							execErr = applyErr
						} else {
							roundOutput.WorkspacePatch = capturedPatch
							if applyResult.Status != "" {
								roundOutput.WorkspacePatch.Status = applyResult.Status
							}
							modifiedFiles = append(modifiedFiles, applyResult.ModifiedFiles...)
						}
					}
				}
			}
		} else {
			execErr = runner.runStep(step, run.WorkspacePath, run.LogPath)
		}
		if step.Check == nil {
			if execErr != nil {
				diagnosis := diagnoseExecutionStepFailure(step, execErr, run.WorkspacePath, run.LogPath, workspacePatchApplyFailed)
				failureSummary = diagnosis.Summary
				recoverySuggestion = diagnosis.RecoverySuggestion
				failureRoundState = buildRoundState(
					[]appruns.RoundPhase{appruns.RoundPhaseInspect, appruns.RoundPhaseEdit, appruns.RoundPhaseRepair, appruns.RoundPhaseFinalize},
					appruns.RoundPhaseFinalize,
					diagnosis.NextAction,
					diagnosis.PreserveWorkspace,
					diagnosis.ResumeAllowed,
				)
				failurePolicy = diagnosis.Policy
				failureSignatures = append(failureSignatures, appruns.FailureSignature{
					Signature:   diagnosis.Signature,
					Count:       1,
					LastStage:   step.Stage,
					SampleError: failureSummary,
				})
				break
			}
			continue
		}
		result := appruns.CheckResult{
			CheckID:       step.Check.CheckID,
			Label:         step.Check.Label,
			Stage:         step.Check.Stage,
			EvidencePaths: []string{run.LogPath},
		}
		if execErr == nil {
			result.Outcome = "passed"
			result.Details = "command chain completed"
			passedChecks = append(passedChecks, result)
			continue
		}
		result.Outcome = "failed"
		result.Details = execErr.Error()
		failedChecks = append(failedChecks, result)
		if step.Check.Required && !step.Check.AllowFailure {
			diagnosis := diagnoseExecutionStepFailure(step, execErr, run.WorkspacePath, run.LogPath, false)
			failureSummary = diagnosis.Summary
			recoverySuggestion = diagnosis.RecoverySuggestion
			failureRoundState = buildRoundState(
				[]appruns.RoundPhase{appruns.RoundPhaseInspect, appruns.RoundPhaseEdit, appruns.RoundPhaseValidate, appruns.RoundPhaseRepair, appruns.RoundPhaseFinalize},
				appruns.RoundPhaseFinalize,
				diagnosis.NextAction,
				diagnosis.PreserveWorkspace,
				diagnosis.ResumeAllowed,
			)
			failurePolicy = diagnosis.Policy
			failureSignatures = append(failureSignatures, appruns.FailureSignature{
				Signature:   diagnosis.Signature,
				Count:       1,
				LastStage:   step.Stage,
				SampleError: failureSummary,
			})
			break
		}
		failureRoundState = buildRoundState([]appruns.RoundPhase{appruns.RoundPhaseInspect, appruns.RoundPhaseEdit, appruns.RoundPhaseValidate, appruns.RoundPhaseRepair, appruns.RoundPhaseFinalize}, appruns.RoundPhaseFinalize, appruns.ControlActionStop, true, false)
		failureSignatures = append(failureSignatures, appruns.FailureSignature{
			Signature:   failureSignatureForStep(step),
			Count:       1,
			LastStage:   step.Stage,
			SampleError: execErr.Error(),
		})
	}
	duration := runner.Now().Sub(startedAt)

	jobRoot := filepath.Dir(run.WorkspacePath)
	reportsDir := filepath.Join(jobRoot, "reports")
	if err := os.MkdirAll(reportsDir, 0o755); err != nil {
		return fmt.Errorf("create reports dir: %w", err)
	}
	changeSummaryPath := filepath.Join(reportsDir, "change-summary.md")
	buildReportPath := filepath.Join(reportsDir, "build-report.md")
	smokeReportPath := filepath.Join(reportsDir, "smoke-test-report.md")
	if err := fileutil.WriteFileAtomic(changeSummaryPath, []byte(buildChangeSummary(plan, run, modifiedFiles, passedChecks, failedChecks)), 0o600); err != nil {
		return err
	}
	if err := fileutil.WriteFileAtomic(buildReportPath, []byte(buildExecutionReport(plan, run, passedChecks, failedChecks, failureSummary)), 0o600); err != nil {
		return err
	}
	if err := fileutil.WriteFileAtomic(smokeReportPath, []byte(buildSmokeReport(passedChecks, failedChecks)), 0o600); err != nil {
		return err
	}

	artifactManifest := appruns.ArtifactManifest{
		SchemaVersion: "0.1.0",
		JobID:         run.JobID,
		GeneratedAt:   runner.Now(),
		Items: []appruns.ArtifactItem{{
			ArtifactID:   "run-log",
			Path:         run.LogPath,
			ArtifactType: "log",
			Produced:     true,
			Required:     true,
			Label:        "builder run log",
		}},
	}
	metrics := appruns.Metrics{
		SchemaVersion:     "0.1.0",
		JobID:             run.JobID,
		TotalIterations:   maxInt(len(steps), 1),
		TotalTokens:       run.TotalTokens,
		DurationSeconds:   duration.Seconds(),
		CommandRuns:       commandRuns,
		SuccessfulChecks:  len(passedChecks),
		FailedChecks:      len(failedChecks),
		FailureSignatures: failureSignatures,
	}
	if err := backend.IndexArtifacts(ctx, run.RunID, artifactManifest); err != nil {
		return err
	}
	if err := backend.IndexMetrics(ctx, run.RunID, metrics); err != nil {
		return err
	}
	validationResults := buildValidationResults(passedChecks, failedChecks)
	roundOutput.ValidationResults = validationResults
	roundOutput.FailureSignatures = failureSignatureNames(failureSignatures)
	repairContext := buildRepairContext(failureSummary, recoverySuggestion, failedChecks, failureSignatures, failureRoundState, failurePolicy)
	roundOutput.RepairContext = repairContext
	if failureSummary != "" {
		roundOutput.Status = "failed"
		roundOutput.Summary = failureSummary
		roundOutput.State = failureRoundState
		return backend.Fail(ctx, runID, appruns.FailureReport{
			Summary:            failureSummary,
			RecoverySuggestion: recoverySuggestion,
			FailureSignatures:  failureSignatureNames(failureSignatures),
			RoundState:         failureRoundState,
			RepairContext:      repairContext,
		})
	}
	finishedAt := runner.Now()
	finalSummary := fmt.Sprintf("thin executor completed: %d/%d checks passed", len(passedChecks), len(passedChecks)+len(failedChecks))
	if len(failedChecks) > 0 {
		finalSummary = fmt.Sprintf("%s, %d allowed failures", finalSummary, len(failedChecks))
	}
	successRoundState := buildRoundState([]appruns.RoundPhase{appruns.RoundPhaseInspect, appruns.RoundPhaseEdit, appruns.RoundPhaseValidate, appruns.RoundPhaseFinalize}, appruns.RoundPhaseFinalize, appruns.ControlActionStop, false, false)
	roundOutput.Status = "completed"
	roundOutput.Summary = finalSummary
	roundOutput.State = successRoundState
	if roundOutput.WorkspacePatch == nil {
		roundOutput.WorkspacePatch = buildWorkspacePatch(modifiedFiles)
	}
	return backend.Complete(ctx, runID, appruns.BuildOutput{
		SchemaVersion: "0.1.0",
		JobID:         run.JobID,
		Status:        "success",
		ExitReason:    "completed",
		WorkerID:      run.WorkerID,
		StartedAt:     &startedAt,
		FinishedAt:    &finishedAt,
		FinalSummary:  finalSummary,
		ModifiedFiles: modifiedFiles,
		ChecksPassed:  passedChecks,
		ChecksFailed:  failedChecks,
		NextHumanActions: []appruns.HumanAction{{
			ActionID: "review-run-result",
			Summary:  "检查运行日志和产物",
			Reason:   "P0 阶段仍需人工确认实际输出",
			Owner:    "engineering",
		}},
		Artifacts: appruns.ArtifactSummary{
			ManifestPath: run.ArtifactManifestPathOrDefault(),
			PrimaryOutputs: []appruns.ArtifactPointer{{
				ArtifactID:   "run-log",
				Path:         run.LogPath,
				ArtifactType: "log",
				Label:        "builder run log",
			}},
		},
		Metrics: appruns.MetricsSummary{
			MetricsPath:               run.MetricsPathOrDefault(),
			TotalIterations:           maxInt(len(steps), 1),
			TotalTokens:               run.TotalTokens,
			DistinctFailureSignatures: len(failureSignatures),
		},
		ReportPaths: appruns.ReportPaths{
			ChangeSummaryPath:   relFromJobRoot(jobRoot, changeSummaryPath),
			BuildReportPath:     relFromJobRoot(jobRoot, buildReportPath),
			SmokeTestReportPath: relFromJobRoot(jobRoot, smokeReportPath),
		},
		RoundInputs:       []appruns.RoundInput{roundInput},
		RoundOutputs:      []appruns.RoundOutput{roundOutput},
		ValidationResults: validationResults,
		RepairContext:     repairContext,
	})
}

func (runner *Runner) backend() RunnerBackend {
	if runner.Backend != nil {
		return runner.Backend
	}
	return &httpRunnerBackend{runner: runner}
}

func (runner *Runner) runStep(step ExecutionStep, workspacePath, logPath string) error {
	closeLog, err := attachLogOutput(step.Command, resolveRunFilePath(workspacePath, logPath))
	if err != nil {
		return err
	}
	defer closeLog()
	return step.Command.Run()
}

func validateApprovalSnapshots(run runRecord) error {
	contextFiles, err := loadRunContextFiles(run)
	if err != nil {
		return err
	}
	for _, supportingFile := range contextFiles.SupportingFiles {
		fileName := filepath.Base(supportingFile)
		var approvalType string
		switch fileName {
		case appruns.PRDApprovalFileName:
			approvalType = appruns.ApprovalTypePRD
		case appruns.TemplateApprovalFileName:
			approvalType = appruns.ApprovalTypeTemplate
		default:
			continue
		}
		if err := validateApprovalSnapshotFile(supportingFile, approvalType); err != nil {
			return err
		}
	}
	return nil
}

func loadRunContextFiles(run runRecord) (appruns.ContextFiles, error) {
	inputPath := filepath.Join(filepath.Dir(run.WorkspacePath), "prepare", "builder-input.json")
	data, err := os.ReadFile(inputPath)
	if err != nil {
		return appruns.ContextFiles{}, fmt.Errorf("load builder input for approval gate: %w", err)
	}
	var input appruns.BuildInput
	if err := json.Unmarshal(data, &input); err != nil {
		return appruns.ContextFiles{}, fmt.Errorf("decode builder input for approval gate: %w", err)
	}
	var contextFiles appruns.ContextFiles
	if err := json.Unmarshal(input.ContextFiles, &contextFiles); err != nil {
		return appruns.ContextFiles{}, fmt.Errorf("decode context files for approval gate: %w", err)
	}
	return contextFiles, nil
}

func validateApprovalSnapshotFile(path, approvalType string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("approval gate missing %s: %w", filepath.Base(path), err)
	}
	var record appruns.ApprovalRecord
	if err := json.Unmarshal(data, &record); err != nil {
		return fmt.Errorf("approval gate decode %s: %w", filepath.Base(path), err)
	}
	if record.ApprovalType != approvalType {
		return fmt.Errorf("approval gate rejected %s: approval_type=%s, want %s", filepath.Base(path), record.ApprovalType, approvalType)
	}
	if record.Status != appruns.ApprovalStatusApproved {
		return fmt.Errorf("approval gate rejected %s: status=%s", filepath.Base(path), record.Status)
	}
	return nil
}

type httpRunnerBackend struct {
	runner *Runner
}

func (backend *httpRunnerBackend) GetRun(ctx context.Context, runID string) (appruns.RunRecord, error) {
	var run appruns.RunRecord
	if err := backend.runner.doJSON(ctx, http.MethodGet, "/internal/v1/build-runs/"+runID, nil, &run); err != nil {
		return appruns.RunRecord{}, err
	}
	return run, nil
}

func (backend *httpRunnerBackend) Heartbeat(ctx context.Context, runID string, heartbeat appruns.Heartbeat) error {
	return backend.runner.doJSON(ctx, http.MethodPost, "/internal/v1/build-runs/"+runID+"/heartbeat", heartbeat, nil)
}

func (backend *httpRunnerBackend) Complete(ctx context.Context, runID string, output appruns.BuildOutput) error {
	return backend.runner.doJSON(ctx, http.MethodPost, "/internal/v1/build-runs/"+runID+"/complete", map[string]any{"builder_output": output}, nil)
}

func (backend *httpRunnerBackend) Fail(ctx context.Context, runID string, report appruns.FailureReport) error {
	return backend.runner.doJSON(ctx, http.MethodPost, "/internal/v1/build-runs/"+runID+"/fail", map[string]any{
		"summary":             report.Summary,
		"recovery_suggestion": report.RecoverySuggestion,
		"failure_signatures":  report.FailureSignatures,
		"round_state":         report.RoundState,
		"repair_context":      report.RepairContext,
	}, nil)
}

func (backend *httpRunnerBackend) IndexArtifacts(ctx context.Context, runID string, manifest appruns.ArtifactManifest) error {
	return backend.runner.doJSON(ctx, http.MethodPost, "/internal/v1/artifacts:index", map[string]any{
		"run_id":   runID,
		"manifest": manifest,
	}, nil)
}

func (backend *httpRunnerBackend) IndexMetrics(ctx context.Context, runID string, metrics appruns.Metrics) error {
	return backend.runner.doJSON(ctx, http.MethodPost, "/internal/v1/metrics:index", map[string]any{
		"run_id":  runID,
		"metrics": metrics,
	}, nil)
}

func (runner *Runner) doJSON(ctx context.Context, method, path string, payload any, out any) error {
	var body []byte
	var err error
	if payload != nil {
		body, err = json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("marshal request: %w", err)
		}
	}
	var reader *bytes.Reader
	if body == nil {
		reader = bytes.NewReader(nil)
	} else {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, runner.BaseURL+path, reader)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := runner.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		var failure map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&failure)
		return fmt.Errorf("request %s %s failed: status=%d body=%v", method, path, resp.StatusCode, failure)
	}
	if out != nil {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			return fmt.Errorf("decode response: %w", err)
		}
	}
	return nil
}

type runRecord struct {
	RunID                string                    `json:"run_id"`
	JobID                string                    `json:"job_id"`
	WorkerID             string                    `json:"worker_id"`
	ExecutorImage        string                    `json:"executor_image,omitempty"`
	GoalSummary          string                    `json:"goal_summary,omitempty"`
	TaskBundle           []appruns.TaskBundleItem  `json:"task_bundle,omitempty"`
	AcceptanceChecks     []appruns.AcceptanceCheck `json:"acceptance_checks,omitempty"`
	AllowedPaths         []string                  `json:"allowed_paths,omitempty"`
	ProtectedPaths       []string                  `json:"protected_paths,omitempty"`
	KnowledgePack        []appruns.ProfileSkill    `json:"knowledge_pack,omitempty"`
	WorkspacePath        string                    `json:"workspace_path"`
	ArtifactDir          string                    `json:"artifact_dir"`
	LaunchCommand        string                    `json:"launch_command"`
	LaunchArgs           []string                  `json:"launch_args"`
	LogPath              string                    `json:"log_path"`
	ArtifactManifestPath string                    `json:"artifact_manifest_path,omitempty"`
	MetricsPath          string                    `json:"metrics_path,omitempty"`
	IterationCount       int                       `json:"iteration_count,omitempty"`
	TotalTokens          int                       `json:"total_tokens,omitempty"`
}

type RunRecord = runRecord

func newRunRecord(run appruns.RunRecord) runRecord {
	return runRecord{
		RunID:                run.RunID,
		JobID:                run.JobID,
		WorkerID:             run.WorkerID,
		ExecutorImage:        run.ExecutorImage,
		GoalSummary:          run.GoalSummary,
		TaskBundle:           append([]appruns.TaskBundleItem(nil), run.TaskBundle...),
		AcceptanceChecks:     append([]appruns.AcceptanceCheck(nil), run.AcceptanceChecks...),
		AllowedPaths:         append([]string(nil), run.AllowedPaths...),
		ProtectedPaths:       append([]string(nil), run.ProtectedPaths...),
		KnowledgePack:        append([]appruns.ProfileSkill(nil), run.KnowledgePack...),
		WorkspacePath:        run.WorkspacePath,
		ArtifactDir:          run.ArtifactDir,
		LaunchCommand:        run.LaunchCommand,
		LaunchArgs:           append([]string(nil), run.LaunchArgs...),
		LogPath:              run.LogPath,
		ArtifactManifestPath: run.ArtifactManifestPath,
		MetricsPath:          run.MetricsPath,
		IterationCount:       run.IterationCount,
		TotalTokens:          run.TotalTokens,
	}
}

func BuildRoundInputForTest(run RunRecord) appruns.RoundInput {
	return buildRoundInput(run)
}

func (run runRecord) ArtifactManifestPathOrDefault() string {
	if run.ArtifactManifestPath != "" {
		return run.ArtifactManifestPath
	}
	return filepath.ToSlash(filepath.Join("jobs", run.JobID, "runs", run.RunID, "artifact-manifest.json"))
}

func (run runRecord) MetricsPathOrDefault() string {
	if run.MetricsPath != "" {
		return run.MetricsPath
	}
	return filepath.ToSlash(filepath.Join("jobs", run.JobID, "runs", run.RunID, "metrics.json"))
}

func relFromJobRoot(jobRoot, path string) string {
	rel, err := filepath.Rel(jobRoot, path)
	if err != nil {
		return filepath.ToSlash(path)
	}
	return filepath.ToSlash(rel)
}

func maxInt(value, fallback int) int {
	if value > 0 {
		return value
	}
	return fallback
}

func attachLogOutput(cmd *exec.Cmd, logPath string) (func(), error) {
	if cmd == nil {
		return func() {}, nil
	}
	if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
		return nil, fmt.Errorf("create log dir: %w", err)
	}
	file, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open log file: %w", err)
	}
	if cmd.Stdout == nil {
		cmd.Stdout = file
	}
	if cmd.Stderr == nil {
		cmd.Stderr = file
	}
	return func() { _ = file.Close() }, nil
}

func failureSignatureForStep(step ExecutionStep) string {
	if step.Check != nil && strings.TrimSpace(step.Check.CheckID) != "" {
		return failureSignatureForCheck(*step.Check)
	}
	return "runner_exit_nonzero"
}

func failureSignatureForCheck(check CheckExecutionPreview) string {
	checkID := strings.TrimSpace(check.CheckID)
	if checkID == "" {
		return "runner_exit_nonzero"
	}
	if isProfileStructuralCheck(checkID) {
		return "profile_check_failed:" + checkID
	}
	if isEnvironmentClosureCheck(checkID, check.Commands) {
		return "environment_check_failed:" + checkID
	}
	return "check_failed:" + checkID
}

func isProfileStructuralCheck(checkID string) bool {
	switch strings.TrimSpace(checkID) {
	case "check-counter-demo-removed", "check-entry-form-wiring", "check-local-persistence-wiring":
		return true
	default:
		return strings.HasPrefix(strings.TrimSpace(checkID), "check-structural-") || strings.HasPrefix(strings.TrimSpace(checkID), "check-profile-")
	}
}

func isEnvironmentClosureCheck(checkID string, commands []string) bool {
	trimmedID := strings.TrimSpace(checkID)
	if strings.HasPrefix(trimmedID, "check-flutter-") || strings.HasPrefix(trimmedID, "check-environment-") {
		return true
	}
	for _, command := range commands {
		trimmedCommand := strings.TrimSpace(command)
		if strings.HasPrefix(trimmedCommand, "flutter ") || trimmedCommand == "flutter" {
			return true
		}
	}
	return false
}

type repairFailurePolicy struct {
	PreserveWorkspace   bool
	ResumeAllowed       bool
	RequiresHumanReview bool
	MaxRounds           int
	UsedRounds          int
	RemainingRounds     int
	TerminationReason   string
}

type executionFailureDiagnosis struct {
	Summary            string
	RecoverySuggestion string
	Signature          string
	NextAction         appruns.ControlAction
	Policy             repairFailurePolicy
	PreserveWorkspace  bool
	ResumeAllowed      bool
}

func diagnoseExecutionStepFailure(step ExecutionStep, execErr error, workspacePath, logPath string, workspacePatchFailed bool) executionFailureDiagnosis {
	diagnosis := executionFailureDiagnosis{
		Summary:            execErr.Error(),
		RecoverySuggestion: "inspect runner log and rerun after fixing prepare stage or environment",
		Signature:          failureSignatureForStep(step),
		NextAction:         appruns.ControlActionStop,
		Policy: repairFailurePolicy{
			PreserveWorkspace:   true,
			ResumeAllowed:       false,
			RequiresHumanReview: true,
			MaxRounds:           1,
			UsedRounds:          1,
			RemainingRounds:     0,
			TerminationReason:   "platform repair budget exhausted for current run",
		},
		PreserveWorkspace: true,
		ResumeAllowed:     false,
	}
	if workspacePatchFailed {
		diagnosis.Summary = fmt.Sprintf("workspace patch apply failed: %s", execErr.Error())
		diagnosis.RecoverySuggestion = "inspect executor-generated changes and allowed/protected path constraints before rerun"
		diagnosis.Signature = "workspace_patch_apply_failed"
		diagnosis.PreserveWorkspace = false
		diagnosis.ResumeAllowed = false
		diagnosis.Policy.PreserveWorkspace = false
		diagnosis.Policy.ResumeAllowed = false
		return diagnosis
	}
	if step.Check != nil {
		checkID := strings.TrimSpace(step.Check.CheckID)
		if isProfileStructuralCheck(checkID) {
			diagnosis.Summary = fmt.Sprintf("profile structural check %s failed: %s", checkID, execErr.Error())
			diagnosis.RecoverySuggestion = "complete the missing Flutter profile implementation before rerun"
			diagnosis.Signature = failureSignatureForCheck(*step.Check)
			diagnosis.NextAction = appruns.ControlActionResume
			diagnosis.PreserveWorkspace = true
			diagnosis.ResumeAllowed = true
			diagnosis.Policy = repairFailurePolicy{
				PreserveWorkspace:   true,
				ResumeAllowed:       true,
				RequiresHumanReview: false,
				MaxRounds:           2,
				UsedRounds:          1,
				RemainingRounds:     1,
			}
			return diagnosis
		}
		if isEnvironmentClosureCheck(checkID, step.Check.Commands) {
			diagnosis.Summary = fmt.Sprintf("environment closure check %s failed: %s", checkID, execErr.Error())
			diagnosis.RecoverySuggestion = "fix the builder environment or command dependencies before rerun"
			diagnosis.Signature = failureSignatureForCheck(*step.Check)
			diagnosis.NextAction = appruns.ControlActionStop
			diagnosis.PreserveWorkspace = true
			diagnosis.ResumeAllowed = false
			diagnosis.Policy = repairFailurePolicy{
				PreserveWorkspace:   true,
				ResumeAllowed:       false,
				RequiresHumanReview: true,
				MaxRounds:           1,
				UsedRounds:          1,
				RemainingRounds:     0,
				TerminationReason:   "platform repair budget exhausted for current run",
			}
			return diagnosis
		}
	}
	logText := strings.ToLower(strings.TrimSpace(readExecutionLogForFailureDiagnosis(workspacePath, logPath)))
	if logText == "" {
		return diagnosis
	}
	return diagnosis
}

func readExecutionLogForFailureDiagnosis(workspacePath, logPath string) string {
	resolvedPath := resolveRunFilePath(workspacePath, logPath)
	if strings.TrimSpace(resolvedPath) == "" {
		return ""
	}
	data, err := os.ReadFile(resolvedPath)
	if err != nil {
		return ""
	}
	const maxHeadBytes = 64 * 1024
	const maxTailBytes = 64 * 1024
	if len(data) > maxHeadBytes+maxTailBytes {
		trimmed := make([]byte, 0, maxHeadBytes+maxTailBytes+8)
		trimmed = append(trimmed, data[:maxHeadBytes]...)
		trimmed = append(trimmed, []byte("\n...\n")...)
		trimmed = append(trimmed, data[len(data)-maxTailBytes:]...)
		data = trimmed
	}
	return string(data)
}

func resolveRunFilePath(workspacePath, path string) string {
	trimmedPath := strings.TrimSpace(path)
	if trimmedPath == "" {
		return ""
	}
	if filepath.IsAbs(trimmedPath) {
		return filepath.Clean(trimmedPath)
	}
	return filepath.Join(appFactoryRootFromWorkspace(workspacePath), filepath.FromSlash(trimmedPath))
}

func failureSignatureNames(signatures []appruns.FailureSignature) []string {
	result := make([]string, 0, len(signatures))
	for _, signature := range signatures {
		if strings.TrimSpace(signature.Signature) == "" {
			continue
		}
		result = append(result, signature.Signature)
	}
	return result
}

func buildRoundInput(run runRecord) appruns.RoundInput {
	plannedRun := plannedExecutionRun(run)
	return appruns.RoundInput{
		RoundID:          "round-1",
		Attempt:          maxInt(plannedRun.IterationCount, 1),
		GoalSummary:      plannedRun.GoalSummary,
		TaskBundle:       append([]appruns.TaskBundleItem(nil), plannedRun.TaskBundle...),
		AllowedPaths:     append([]string(nil), plannedRun.AllowedPaths...),
		KnowledgePack:    append([]appruns.ProfileSkill(nil), plannedRun.KnowledgePack...),
		AcceptanceChecks: append([]appruns.AcceptanceCheck(nil), plannedRun.AcceptanceChecks...),
	}
}

func buildValidationResults(passedChecks, failedChecks []appruns.CheckResult) []appruns.ValidationResult {
	results := make([]appruns.ValidationResult, 0, len(passedChecks)+len(failedChecks))
	for _, check := range passedChecks {
		results = append(results, appruns.ValidationResult{
			CheckID:       check.CheckID,
			Label:         check.Label,
			Stage:         check.Stage,
			Outcome:       check.Outcome,
			Summary:       check.Details,
			EvidencePaths: append([]string(nil), check.EvidencePaths...),
		})
	}
	for _, check := range failedChecks {
		results = append(results, appruns.ValidationResult{
			CheckID:       check.CheckID,
			Label:         check.Label,
			Stage:         check.Stage,
			Outcome:       check.Outcome,
			Summary:       check.Details,
			EvidencePaths: append([]string(nil), check.EvidencePaths...),
			Blocking:      true,
		})
	}
	return results
}

func buildRepairContext(failureSummary, recoverySuggestion string, failedChecks []appruns.CheckResult, failureSignatures []appruns.FailureSignature, roundState *appruns.RoundState, policy repairFailurePolicy) *appruns.RepairContext {
	if strings.TrimSpace(failureSummary) == "" && len(failedChecks) == 0 && len(failureSignatures) == 0 && roundState == nil {
		return nil
	}
	if !policy.PreserveWorkspace && !policy.ResumeAllowed && !policy.RequiresHumanReview && policy.MaxRounds == 0 && policy.UsedRounds == 0 && policy.RemainingRounds == 0 && strings.TrimSpace(policy.TerminationReason) == "" {
		policy = repairFailurePolicy{
			PreserveWorkspace:   true,
			ResumeAllowed:       false,
			RequiresHumanReview: true,
			MaxRounds:           1,
			UsedRounds:          1,
			RemainingRounds:     0,
			TerminationReason:   "platform repair budget exhausted for current run",
		}
	}
	result := &appruns.RepairContext{
		State:               roundState,
		Budget:              buildRepairBudget(failureSummary, policy),
		Reason:              failureSummary,
		FailureSignatures:   failureSignatureNames(failureSignatures),
		RecommendedAction:   recoverySuggestion,
		PreserveWorkspace:   policy.PreserveWorkspace,
		RequiresHumanReview: policy.RequiresHumanReview,
	}
	for _, check := range failedChecks {
		if strings.TrimSpace(check.CheckID) == "" {
			continue
		}
		result.FailedChecks = append(result.FailedChecks, check.CheckID)
	}
	return result
}

func buildRepairBudget(failureSummary string, policy repairFailurePolicy) *appruns.RepairBudget {
	if strings.TrimSpace(failureSummary) == "" {
		return nil
	}
	if policy.MaxRounds <= 0 {
		policy.MaxRounds = 1
	}
	if policy.UsedRounds <= 0 {
		policy.UsedRounds = 1
	}
	if policy.RemainingRounds < 0 {
		policy.RemainingRounds = 0
	}
	terminationReason := strings.TrimSpace(policy.TerminationReason)
	if terminationReason == "" && policy.RemainingRounds == 0 {
		terminationReason = "platform repair budget exhausted for current run"
	}
	return &appruns.RepairBudget{
		Owner:             appruns.RepairBudgetOwnerPlatform,
		MaxRounds:         policy.MaxRounds,
		UsedRounds:        policy.UsedRounds,
		RemainingRounds:   policy.RemainingRounds,
		TerminationReason: terminationReason,
	}
}

func buildRoundState(trace []appruns.RoundPhase, current appruns.RoundPhase, next appruns.ControlAction, preserveWorkspace, resumeAllowed bool) *appruns.RoundState {
	return &appruns.RoundState{
		CurrentPhase:      current,
		PhaseTrace:        append([]appruns.RoundPhase(nil), trace...),
		NextAction:        next,
		PreserveWorkspace: preserveWorkspace,
		ResumeAllowed:     resumeAllowed,
	}
}

func buildWorkspacePatch(modifiedFiles []appruns.FileChange) *appruns.WorkspacePatch {
	if len(modifiedFiles) == 0 {
		return &appruns.WorkspacePatch{PatchID: "round-1-patch", Status: "not_reported"}
	}
	result := &appruns.WorkspacePatch{PatchID: "round-1-patch", Status: "applied"}
	for _, file := range modifiedFiles {
		if strings.TrimSpace(file.Path) == "" {
			continue
		}
		result.ModifiedFiles = append(result.ModifiedFiles, file.Path)
		result.Operations = append(result.Operations, appruns.WorkspacePatchOperation{
			Type: mapFileChangeToPatchOp(file.ChangeType),
			Path: file.Path,
		})
	}
	if len(result.ModifiedFiles) == 0 {
		result.Status = "not_reported"
		result.Operations = nil
	}
	return result
}

func mapFileChangeToPatchOp(changeType string) string {
	switch strings.ToLower(strings.TrimSpace(changeType)) {
	case "added", "created":
		return "write_file"
	case "deleted", "removed":
		return "delete_file"
	case "replaced":
		return "replace_block"
	case "modified", "updated", "patched":
		return "replace_block"
	default:
		return "write_file"
	}
}

func buildChangeSummary(plan RoundPlan, run runRecord, modifiedFiles []appruns.FileChange, passedChecks, failedChecks []appruns.CheckResult) string {
	var builder strings.Builder
	builder.WriteString("# 变更摘要\n\n")
	builder.WriteString("- 目标：")
	builder.WriteString(run.GoalSummary)
	builder.WriteString("\n")
	builder.WriteString("- 任务数：")
	builder.WriteString(strconv.Itoa(len(plan.TaskBundle)))
	builder.WriteString("\n")
	builder.WriteString("- 变更文件：")
	builder.WriteString(strconv.Itoa(len(modifiedFiles)))
	builder.WriteString("\n")
	builder.WriteString("- 通过检查：")
	builder.WriteString(strconv.Itoa(len(passedChecks)))
	builder.WriteString("\n")
	builder.WriteString("- 失败检查：")
	builder.WriteString(strconv.Itoa(len(failedChecks)))
	builder.WriteString("\n")
	for _, file := range modifiedFiles {
		if strings.TrimSpace(file.Path) == "" {
			continue
		}
		builder.WriteString("- 文件：")
		builder.WriteString(file.Path)
		if strings.TrimSpace(file.ChangeType) != "" {
			builder.WriteString(" (")
			builder.WriteString(file.ChangeType)
			builder.WriteString(")")
		}
		builder.WriteString("\n")
	}
	return builder.String()
}

func buildExecutionReport(plan RoundPlan, run runRecord, passedChecks, failedChecks []appruns.CheckResult, failureSummary string) string {
	var builder strings.Builder
	steps := plan.Steps()
	builder.WriteString("# 构建报告\n\n")
	builder.WriteString("## 执行概览\n")
	builder.WriteString("- 目标：")
	builder.WriteString(run.GoalSummary)
	builder.WriteString("\n")
	builder.WriteString("- 执行步骤：")
	builder.WriteString(strconv.Itoa(len(steps)))
	builder.WriteString("\n")
	builder.WriteString("- 通过检查：")
	builder.WriteString(strconv.Itoa(len(passedChecks)))
	builder.WriteString("\n")
	builder.WriteString("- 失败检查：")
	builder.WriteString(strconv.Itoa(len(failedChecks)))
	builder.WriteString("\n\n")
	builder.WriteString("## 步骤\n")
	for _, step := range steps {
		builder.WriteString("- [")
		builder.WriteString(string(step.Stage))
		builder.WriteString("] ")
		builder.WriteString(step.StepID)
		builder.WriteString("：")
		builder.WriteString(step.Summary)
		builder.WriteString("\n")
	}
	if failureSummary != "" {
		builder.WriteString("\n## 失败摘要\n")
		builder.WriteString(failureSummary)
		builder.WriteString("\n")
	}
	return builder.String()
}

func buildSmokeReport(passedChecks, failedChecks []appruns.CheckResult) string {
	var builder strings.Builder
	builder.WriteString("# 冒烟报告\n\n")
	foundSmoke := false
	for _, check := range passedChecks {
		if check.Stage != appruns.StageSmoke {
			continue
		}
		foundSmoke = true
		builder.WriteString("- 通过：")
		builder.WriteString(check.CheckID)
		builder.WriteString("\n")
	}
	for _, check := range failedChecks {
		if check.Stage != appruns.StageSmoke {
			continue
		}
		foundSmoke = true
		builder.WriteString("- 失败：")
		builder.WriteString(check.CheckID)
		builder.WriteString("\n")
	}
	if !foundSmoke {
		builder.WriteString("当前运行未包含 smoke 阶段检查。\n")
	}
	return builder.String()
}

func dockerPrepareCommand(ctx context.Context, run runRecord) (*exec.Cmd, error) {
	return dockerCommand(ctx, run, append([]string{run.LaunchCommand}, run.LaunchArgs...), nil)
}

func dockerCommand(ctx context.Context, run runRecord, argv []string, extraEnv []string) (*exec.Cmd, error) {
	root := appFactoryRootFromWorkspace(run.WorkspacePath)
	if err := ensureDockerWritablePaths(ctx, root, run); err != nil {
		return nil, err
	}
	useHostNetwork := strings.EqualFold(strings.TrimSpace(os.Getenv("APPFACTORY_BUILDER_DOCKER_NETWORK")), "host")
	args := []string{
		"run", "--rm",
		"--user", strconv.Itoa(os.Getuid()) + ":" + strconv.Itoa(os.Getgid()),
		"-v", root + ":" + root,
		"-w", run.WorkspacePath,
	}
	for _, env := range append(baseExecutionEnv(run), extraEnv...) {
		args = append(args, "-e", env)
	}
	if useHostNetwork {
		args = append(args, "--network", "host")
	} else {
		args = append(args, "--add-host", "host.docker.internal:host-gateway")
	}
	pubCacheDir := resolveBuilderPubCache(root)
	if err := os.MkdirAll(pubCacheDir, 0o755); err != nil {
		return nil, fmt.Errorf("create builder pub cache: %w", err)
	}
	args = append(args, "-e", "PUB_CACHE="+pubCacheDir)
	args = append(args, dockerEnvArgs(
		"APPFACTORY_BUILDER_DOCKER_NETWORK",
		"APPFACTORY_BUILDER_RUNTIME",
		"HTTP_PROXY",
		"HTTPS_PROXY",
		"ALL_PROXY",
		"NO_PROXY",
	)...)
	args = append(args,
		run.ExecutorImage,
		"exec",
		argv[0],
	)
	args = append(args, argv[1:]...)
	return exec.CommandContext(ctx, "docker", args...), nil
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

func appFactoryRootFromWorkspace(workspacePath string) string {
	return filepath.Clean(filepath.Join(workspacePath, "..", "..", ".."))
}

func ensureDockerWritablePaths(ctx context.Context, root string, run runRecord) error {
	paths := []string{
		filepath.Clean(filepath.Join(run.WorkspacePath, "..")),
	}
	paths = append(paths, filepath.Clean(resolveBuilderPubCache(root)))

	args := []string{
		"run", "--rm",
		"--user", "0:0",
		"-v", root + ":" + root,
	}
	for _, mount := range extraOwnershipMounts(paths, root) {
		args = append(args, "-v", mount+":"+mount)
	}
	args = append(args,
		run.ExecutorImage,
		"exec",
		"/bin/sh",
		"-lc",
		buildChownCommand(paths),
	)
	cmd := exec.CommandContext(ctx, "docker", args...)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("prepare docker workspace ownership: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func extraOwnershipMounts(paths []string, root string) []string {
	seen := map[string]struct{}{}
	result := make([]string, 0)
	for _, path := range paths {
		cleaned := filepath.Clean(path)
		if cleaned == root {
			continue
		}
		if strings.HasPrefix(cleaned, root+string(os.PathSeparator)) {
			continue
		}
		if _, ok := seen[cleaned]; ok {
			continue
		}
		seen[cleaned] = struct{}{}
		result = append(result, cleaned)
	}
	return result
}

func buildChownCommand(paths []string) string {
	commands := make([]string, 0, len(paths))
	uidgid := strconv.Itoa(os.Getuid()) + ":" + strconv.Itoa(os.Getgid())
	for _, path := range paths {
		quoted := shellQuote(path)
		commands = append(commands, "mkdir -p "+quoted+" && chown -R "+uidgid+" "+quoted)
	}
	return strings.Join(commands, " && ")
}

func resolveBuilderPubCache(root string) string {
	return filepath.Join(root, ".runtime", "pub-cache")
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}
