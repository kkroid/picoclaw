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
	"sort"
	"strconv"
	"strings"
	"time"

	appruns "github.com/sipeed/picoclaw/pkg/appfactory/runs"
	appconfig "github.com/sipeed/picoclaw/pkg/config"
	"github.com/sipeed/picoclaw/pkg/fileutil"
)

type Runner struct {
	BaseURL              string
	HTTPClient           *http.Client
	Now                  func() time.Time
	Executor             ThinExecutor
	Backend              RunnerBackend
	BuilderRuntimeConfig appconfig.BuilderRuntimeConfig
	ModelCatalog         *appconfig.Config
	PatchGenerator       BuilderRuntimePatchGenerator
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
	return NewRunnerWithBackendAndConfig(backend, appconfig.BuilderRuntimeConfig{})
}

func NewRunnerWithBackendAndConfig(backend RunnerBackend, runtimeConfig appconfig.BuilderRuntimeConfig) *Runner {
	return &Runner{
		Now:                  func() time.Time { return time.Now().UTC() },
		Executor:             NewThinExecutor(),
		Backend:              backend,
		BuilderRuntimeConfig: runtimeConfig,
		ModelCatalog:         nil,
		PatchGenerator:       nil,
	}
}

func NewRunnerWithBackendAndAppConfig(backend RunnerBackend, cfg *appconfig.Config) *Runner {
	runtimeConfig := appconfig.BuilderRuntimeConfig{}
	if cfg != nil {
		runtimeConfig = cfg.AppFactory.BuilderRuntime
	}
	runner := NewRunnerWithBackendAndConfig(backend, runtimeConfig)
	runner.ModelCatalog = cfg
	return runner
}

func (runner *Runner) ExecuteRun(ctx context.Context, runID string) error {
	backend := runner.backend()
	rawRun, err := backend.GetRun(ctx, runID)
	if err != nil {
		return err
	}
	run := runner.prepareRunRecord(rawRun)
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
	builderRuntimeStats := (*appruns.BuilderRuntimeExecutionStats)(nil)
	failureSummary := ""
	recoverySuggestion := ""
	failureRoundState := (*appruns.RoundState)(nil)
	failurePolicy := repairFailurePolicy{}
	for index, step := range steps {
		if err := backend.Heartbeat(ctx, runID, appruns.Heartbeat{
			Stage:         step.Stage,
			Iteration:     index + 1,
			RoundID:       roundInput.RoundID,
			Attempt:       maxInt(roundInput.Attempt, 1),
			CheckpointKey: heartbeatCheckpointKey(step),
			RoundState:    heartbeatRoundStateForStep(step),
			TargetPaths:   collectHeartbeatTargetPaths(roundInput),
			Summary:       step.Summary,
			TotalTokens:   0,
		}); err != nil {
			return err
		}
		if step.Command == nil {
			continue
		}
		execErr := error(nil)
		workspacePatchApplyFailed := false
		if step.Check == nil {
			roundState := heartbeatRoundStateForStep(step)
			if runner.shouldUseBuilderRuntime(run) {
				editResult, editErr := runner.executeBuilderRuntimeEdit(ctx, backend, runID, step, run, roundInput)
				if editResult != nil && editResult.Stats != nil {
					builderRuntimeStats = editResult.Stats
					roundOutput.BuilderRuntime = editResult.Stats
				}
				if editErr != nil {
					execErr = editErr
				} else if editResult != nil {
					roundOutput.WorkspacePatch = editResult.Patch
					modifiedFiles = append(modifiedFiles, editResult.ApplyResult.ModifiedFiles...)
					if err := runner.reportAppliedPatch(ctx, backend, runID, step, roundInput, roundState, editResult.ApplyResult.ModifiedFiles); err != nil {
						return err
					}
				}
			} else {
				commandRuns++
				beforeSnapshot, snapshotErr := captureWorkspaceSnapshot(run.WorkspacePath)
				if snapshotErr != nil {
					execErr = snapshotErr
				} else {
					if err := runner.reportPatchGenerationStarted(ctx, backend, runID, step, roundInput, roundState); err != nil {
						return err
					}
					execErr = runner.runStep(step, run.WorkspacePath, run.LogPath)
					afterSnapshot, afterErr := captureWorkspaceSnapshot(run.WorkspacePath)
					if afterErr != nil {
						execErr = afterErr
					} else {
						capturedPatch, patchErr := buildCapturedWorkspacePatch(roundInput.RoundID, beforeSnapshot, afterSnapshot, run.AllowedPaths, run.ProtectedPaths)
						if restoreErr := restoreWorkspaceSnapshot(run.WorkspacePath, beforeSnapshot, afterSnapshot); restoreErr != nil {
							execErr = restoreErr
						} else if execErr == nil && patchErr != nil {
							if err := runner.reportPatchGenerationFailed(ctx, backend, runID, step, roundInput, roundState, collectHeartbeatTargetPaths(roundInput), patchErr); err != nil {
								return err
							}
							execErr = patchErr
						} else if execErr == nil {
							if err := runner.reportGeneratedPatch(ctx, backend, runID, step, roundInput, roundState, capturedPatch); err != nil {
								return err
							}
							applyResult, applyErr := appruns.ApplyWorkspacePatch(run.WorkspacePath, run.AllowedPaths, run.ProtectedPaths, *capturedPatch)
							if applyErr != nil {
								workspacePatchApplyFailed = true
								if err := runner.reportPatchApplyFailed(ctx, backend, runID, step, roundInput, roundState, workspacePatchPaths(capturedPatch), applyErr); err != nil {
									return err
								}
								execErr = applyErr
							} else {
								roundOutput.WorkspacePatch = capturedPatch
								if applyResult.Status != "" {
									roundOutput.WorkspacePatch.Status = applyResult.Status
								}
								modifiedFiles = append(modifiedFiles, applyResult.ModifiedFiles...)
								if err := runner.reportAppliedPatch(ctx, backend, runID, step, roundInput, roundState, applyResult.ModifiedFiles); err != nil {
									return err
								}
							}
						}
					}
				}
			}
		} else {
			commandRuns++
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
		if step.Check.Required && !step.Check.AllowFailure {
			if repairResult, repaired, repairErr := runner.tryAutomaticValidationRepair(ctx, backend, runID, run, roundInput, step); repaired {
				if repairResult != nil {
					modifiedFiles = append(modifiedFiles, repairResult.ApplyResult.ModifiedFiles...)
					if repairResult.Patch != nil {
						roundOutput.WorkspacePatch = repairResult.Patch
					}
					if err := runner.reportAppliedPatch(ctx, backend, runID, step, roundInput, buildRoundState([]appruns.RoundPhase{appruns.RoundPhaseInspect, appruns.RoundPhaseEdit, appruns.RoundPhaseValidate, appruns.RoundPhaseRepair}, appruns.RoundPhaseRepair, appruns.ControlActionNone, false, false), repairResult.ApplyResult.ModifiedFiles); err != nil {
						return err
					}
					if repairResult.Stats != nil {
						builderRuntimeStats = mergeBuilderRuntimeStats(builderRuntimeStats, repairResult.Stats)
						roundOutput.BuilderRuntime = builderRuntimeStats
					}
				}
				if repairErr == nil {
					commandRuns++
					execErr = runner.rerunStep(step, run.WorkspacePath, run.LogPath)
					if execErr == nil {
						result.Outcome = "passed"
						result.Details = "command chain completed after builder-runtime repair"
						passedChecks = append(passedChecks, result)
						continue
					}
				} else {
					execErr = repairErr
				}
			}
			result.Outcome = "failed"
			result.Details = execErr.Error()
			failedChecks = append(failedChecks, result)
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
		result.Outcome = "failed"
		result.Details = execErr.Error()
		failedChecks = append(failedChecks, result)
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
	deviceFailureCategories := buildDeviceFailureCategoryStats(failureSignatures)
	if err := fileutil.WriteFileAtomic(changeSummaryPath, []byte(buildChangeSummary(plan, run, modifiedFiles, passedChecks, failedChecks)), 0o600); err != nil {
		return err
	}
	if err := fileutil.WriteFileAtomic(buildReportPath, []byte(buildExecutionReport(plan, run, passedChecks, failedChecks, failureSummary)), 0o600); err != nil {
		return err
	}
	if err := fileutil.WriteFileAtomic(smokeReportPath, []byte(buildSmokeReport(passedChecks, failedChecks, deviceFailureCategories)), 0o600); err != nil {
		return err
	}

	artifactItems, primaryOutputs := buildRuntimeArtifactOutputs(run, jobRoot, changeSummaryPath, buildReportPath, smokeReportPath)
	artifactManifest := appruns.ArtifactManifest{
		SchemaVersion: "0.1.0",
		JobID:         run.JobID,
		GeneratedAt:   runner.Now(),
		Items:         artifactItems,
	}
	metrics := appruns.Metrics{
		SchemaVersion:           "0.1.0",
		JobID:                   run.JobID,
		TotalIterations:         maxInt(len(steps), 1),
		TotalTokens:             run.TotalTokens,
		BuilderRuntime:          builderRuntimeStats,
		DurationSeconds:         duration.Seconds(),
		CommandRuns:             commandRuns,
		SuccessfulChecks:        len(passedChecks),
		FailedChecks:            len(failedChecks),
		DeviceFailureCategories: deviceFailureCategories,
		FailureSignatures:       failureSignatures,
	}
	if builderRuntimeStats != nil {
		metrics.TotalTokens += builderRuntimeStats.TotalTokens
		metrics.PromptTokens = builderRuntimeStats.PromptTokens
		metrics.CompletionTokens = builderRuntimeStats.CompletionTokens
		metrics.ModelRequestRetries = maxInt(builderRuntimeStats.Attempts-1, 0)
		metrics.ModelRequestFailures = builderRuntimeStats.ParseFailureCount + builderRuntimeStats.ScopeViolationCount
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
			ManifestPath:   run.ArtifactManifestPathOrDefault(),
			PrimaryOutputs: primaryOutputs,
		},
		Metrics: appruns.MetricsSummary{
			MetricsPath:               run.MetricsPathOrDefault(),
			TotalIterations:           maxInt(len(steps), 1),
			TotalTokens:               metrics.TotalTokens,
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

func (runner *Runner) tryAutomaticValidationRepair(ctx context.Context, backend RunnerBackend, runID string, run runRecord, roundInput appruns.RoundInput, step ExecutionStep) (*builderRuntimeExecutionResult, bool, error) {
	if step.Check == nil || !runner.shouldUseBuilderRuntime(run) {
		return nil, false, nil
	}
	repairTaskType, ok := validationRepairTaskType(*step.Check)
	if !ok {
		return nil, false, nil
	}
	targetPaths := concreteTaskTargetPaths(run.TaskBundle)
	if len(targetPaths) == 0 {
		targetPaths = concreteAllowedPaths(run.AllowedPaths)
	}
	repairTask := appruns.TaskBundleItem{
		TaskID:      "repair-" + strings.TrimSpace(step.Check.CheckID),
		Title:       "Repair " + strings.TrimSpace(step.Check.Label),
		Category:    appruns.TaskCategoryValidation,
		TaskType:    repairTaskType,
		Objective:   fmt.Sprintf("repair the workspace so %s passes", strings.TrimSpace(step.Check.CheckID)),
		TargetPaths: targetPaths,
		CompletionCriteria: []string{
			fmt.Sprintf("%s passes", strings.TrimSpace(step.Check.CheckID)),
		},
	}
	repairRun := run
	repairRun.TaskBundle = []appruns.TaskBundleItem{repairTask}
	if hasBuilderRuntimeConfig(runner.BuilderRuntimeConfig) {
		plan := resolveBuilderRuntimePlan(runner.BuilderRuntimeConfig, repairRun.TaskBundle)
		repairRun.BuilderRuntime = &plan
	}
	repairRoundInput := roundInput
	repairRoundInput.Attempt = maxInt(roundInput.Attempt+1, 2)
	repairRoundInput.TaskBundle = repairRun.TaskBundle
	repairRoundInput.AcceptanceChecks = []appruns.AcceptanceCheck{{
		CheckID:         step.Check.CheckID,
		Label:           step.Check.Label,
		Stage:           step.Check.Stage,
		Required:        step.Check.Required,
		Commands:        append([]string(nil), step.Check.Commands...),
		AllowFailure:    step.Check.AllowFailure,
		SuccessCriteria: strings.TrimSpace(step.Summary),
	}}
	repairRoundInput.BuilderRuntime = repairRun.BuilderRuntime
	if backend != nil {
		if err := backend.Heartbeat(ctx, runID, appruns.Heartbeat{
			Stage:         appruns.StageOther,
			Iteration:     maxInt(repairRoundInput.Attempt, 2),
			RoundID:       repairRoundInput.RoundID,
			Attempt:       maxInt(repairRoundInput.Attempt, 2),
			CheckpointKey: strings.TrimSpace(step.Check.CheckID),
			RoundState:    buildRoundState([]appruns.RoundPhase{appruns.RoundPhaseInspect, appruns.RoundPhaseEdit, appruns.RoundPhaseValidate, appruns.RoundPhaseRepair}, appruns.RoundPhaseRepair, appruns.ControlActionNone, false, false),
			TargetPaths:   collectHeartbeatTargetPaths(repairRoundInput),
			Summary:       fmt.Sprintf("builder-runtime auto repair: %s | task_type=%s", strings.TrimSpace(step.Check.CheckID), repairTaskType),
			TotalTokens:   0,
		}); err != nil {
			return nil, true, err
		}
	}
	result, err := runner.executeBuilderRuntimeEdit(ctx, backend, runID, step, repairRun, repairRoundInput)
	return result, true, err
}

func validationRepairTaskType(check CheckExecutionPreview) (appruns.BuilderRuntimeTaskType, bool) {
	switch strings.TrimSpace(check.CheckID) {
	case "check-flutter-analyze":
		return appruns.BuilderRuntimeTaskTypeAnalyzeRepair, true
	case "check-flutter-test":
		return appruns.BuilderRuntimeTaskTypeTestRepair, true
	case "check-flutter-build-apk":
		return appruns.BuilderRuntimeTaskTypeClosureRepair, true
	default:
		return "", false
	}
}

func heartbeatRoundStateForStep(step ExecutionStep) *appruns.RoundState {
	if step.Check != nil {
		return buildRoundState(
			[]appruns.RoundPhase{appruns.RoundPhaseInspect, appruns.RoundPhaseEdit, appruns.RoundPhaseValidate},
			appruns.RoundPhaseValidate,
			appruns.ControlActionNone,
			false,
			false,
		)
	}
	return buildRoundState(
		[]appruns.RoundPhase{appruns.RoundPhaseInspect, appruns.RoundPhaseEdit},
		appruns.RoundPhaseEdit,
		appruns.ControlActionNone,
		false,
		false,
	)
}

func heartbeatCheckpointKey(step ExecutionStep) string {
	if step.Check != nil {
		return strings.TrimSpace(step.Check.CheckID)
	}
	return strings.TrimSpace(step.StepID)
}

func collectHeartbeatTargetPaths(input appruns.RoundInput) []string {
	paths := make([]string, 0)
	seen := map[string]struct{}{}
	appendPath := func(value string) {
		normalized := strings.TrimSpace(value)
		if normalized == "" {
			return
		}
		if _, ok := seen[normalized]; ok {
			return
		}
		seen[normalized] = struct{}{}
		paths = append(paths, normalized)
	}
	for _, task := range input.TaskBundle {
		for _, targetPath := range task.TargetPaths {
			appendPath(targetPath)
		}
	}
	for _, allowedPath := range input.AllowedPaths {
		appendPath(allowedPath)
	}
	sort.Strings(paths)
	return paths
}

func mergeBuilderRuntimeStats(base, extra *appruns.BuilderRuntimeExecutionStats) *appruns.BuilderRuntimeExecutionStats {
	if base == nil {
		return extra
	}
	if extra == nil {
		return base
	}
	merged := *base
	if extra.Mode != "" {
		merged.Mode = extra.Mode
	}
	if extra.TaskType != "" {
		merged.TaskType = extra.TaskType
	}
	if extra.RouteSource != "" {
		merged.RouteSource = extra.RouteSource
	}
	if extra.SelectedModel != "" {
		merged.SelectedModel = extra.SelectedModel
	}
	merged.ModelSequence = append(append([]string(nil), merged.ModelSequence...), extra.ModelSequence...)
	merged.UpgradeApplied = merged.UpgradeApplied || extra.UpgradeApplied
	merged.Attempts += extra.Attempts
	merged.PromptTokens += extra.PromptTokens
	merged.CompletionTokens += extra.CompletionTokens
	merged.TotalTokens += extra.TotalTokens
	merged.OperationCount += extra.OperationCount
	merged.TargetedOperationCount += extra.TargetedOperationCount
	merged.UnrelatedOperationCount += extra.UnrelatedOperationCount
	if merged.OperationCount > 0 {
		merged.UnrelatedOperationRate = float64(merged.UnrelatedOperationCount) / float64(merged.OperationCount)
	}
	merged.SchemaNormalized = merged.SchemaNormalized || extra.SchemaNormalized
	merged.SchemaDriftCount += extra.SchemaDriftCount
	merged.ParseFailureCount += extra.ParseFailureCount
	merged.ScopeViolationCount += extra.ScopeViolationCount
	if extra.FailureReason != "" {
		merged.FailureReason = extra.FailureReason
	}
	return &merged
}

func (runner *Runner) runStep(step ExecutionStep, workspacePath, logPath string) error {
	closeLog, err := attachLogOutput(step.Command, resolveRunFilePath(workspacePath, logPath))
	if err != nil {
		return err
	}
	defer closeLog()
	return step.Command.Run()
}

func (runner *Runner) rerunStep(step ExecutionStep, workspacePath, logPath string) error {
	cmd, err := cloneExecCommand(step.Command)
	if err != nil {
		return err
	}
	retryStep := step
	retryStep.Command = cmd
	return runner.runStep(retryStep, workspacePath, logPath)
}

func cloneExecCommand(cmd *exec.Cmd) (*exec.Cmd, error) {
	if cmd == nil {
		return nil, fmt.Errorf("cannot clone nil exec command")
	}
	if strings.TrimSpace(cmd.Path) == "" {
		return nil, fmt.Errorf("cannot clone exec command with empty path")
	}
	args := append([]string(nil), cmd.Args...)
	if len(args) == 0 {
		args = []string{cmd.Path}
	}
	cloned := exec.Command(args[0], args[1:]...)
	cloned.Dir = cmd.Dir
	cloned.Env = append([]string(nil), cmd.Env...)
	return cloned, nil
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

func (runner *Runner) reportAppliedPatch(
	ctx context.Context,
	backend RunnerBackend,
	runID string,
	step ExecutionStep,
	roundInput appruns.RoundInput,
	roundState *appruns.RoundState,
	modifiedFiles []appruns.FileChange,
) error {
	affectedPaths := modifiedFilePaths(modifiedFiles)
	if len(affectedPaths) == 0 {
		return nil
	}
	return backend.Heartbeat(ctx, runID, appruns.Heartbeat{
		Stage:         step.Stage,
		Iteration:     maxInt(roundInput.Attempt, 1),
		EventType:     "run_patch_applied",
		RoundID:       roundInput.RoundID,
		Attempt:       maxInt(roundInput.Attempt, 1),
		CheckpointKey: heartbeatCheckpointKey(step),
		RoundState:    roundState,
		TargetPaths:   collectHeartbeatTargetPaths(roundInput),
		AffectedPaths: affectedPaths,
		Summary:       buildAppliedPatchSummary(roundInput.RoundID, affectedPaths),
		TotalTokens:   0,
	})
}

func (runner *Runner) reportPatchGenerationStarted(
	ctx context.Context,
	backend RunnerBackend,
	runID string,
	step ExecutionStep,
	roundInput appruns.RoundInput,
	roundState *appruns.RoundState,
) error {
	targetPaths := collectHeartbeatTargetPaths(roundInput)
	return backend.Heartbeat(ctx, runID, appruns.Heartbeat{
		Stage:         step.Stage,
		Iteration:     maxInt(roundInput.Attempt, 1),
		EventType:     "run_patch_generation_started",
		RoundID:       roundInput.RoundID,
		Attempt:       maxInt(roundInput.Attempt, 1),
		CheckpointKey: heartbeatCheckpointKey(step),
		RoundState:    roundState,
		TargetPaths:   targetPaths,
		Summary:       buildPatchGenerationStartedSummary(roundInput.RoundID, targetPaths),
		TotalTokens:   0,
	})
}

func (runner *Runner) reportPatchGenerationFailed(
	ctx context.Context,
	backend RunnerBackend,
	runID string,
	step ExecutionStep,
	roundInput appruns.RoundInput,
	roundState *appruns.RoundState,
	targetPaths []string,
	reason error,
) error {
	return backend.Heartbeat(ctx, runID, appruns.Heartbeat{
		Stage:         step.Stage,
		Iteration:     maxInt(roundInput.Attempt, 1),
		EventType:     "run_patch_generation_failed",
		RoundID:       roundInput.RoundID,
		Attempt:       maxInt(roundInput.Attempt, 1),
		CheckpointKey: heartbeatCheckpointKey(step),
		RoundState:    roundState,
		TargetPaths:   targetPaths,
		Summary:       buildPatchGenerationFailedSummary(roundInput.RoundID, targetPaths, reason),
		TotalTokens:   0,
	})
}

func (runner *Runner) reportPatchApplyFailed(
	ctx context.Context,
	backend RunnerBackend,
	runID string,
	step ExecutionStep,
	roundInput appruns.RoundInput,
	roundState *appruns.RoundState,
	targetPaths []string,
	reason error,
) error {
	return backend.Heartbeat(ctx, runID, appruns.Heartbeat{
		Stage:         step.Stage,
		Iteration:     maxInt(roundInput.Attempt, 1),
		EventType:     "run_patch_apply_failed",
		RoundID:       roundInput.RoundID,
		Attempt:       maxInt(roundInput.Attempt, 1),
		CheckpointKey: heartbeatCheckpointKey(step),
		RoundState:    roundState,
		TargetPaths:   targetPaths,
		Summary:       buildPatchApplyFailedSummary(roundInput.RoundID, targetPaths, reason),
		TotalTokens:   0,
	})
}

func (runner *Runner) reportGeneratedPatch(
	ctx context.Context,
	backend RunnerBackend,
	runID string,
	step ExecutionStep,
	roundInput appruns.RoundInput,
	roundState *appruns.RoundState,
	patch *appruns.WorkspacePatch,
) error {
	patchPaths := workspacePatchPaths(patch)
	if len(patchPaths) == 0 {
		return nil
	}
	return backend.Heartbeat(ctx, runID, appruns.Heartbeat{
		Stage:         step.Stage,
		Iteration:     maxInt(roundInput.Attempt, 1),
		EventType:     "run_patch_generated",
		RoundID:       roundInput.RoundID,
		Attempt:       maxInt(roundInput.Attempt, 1),
		CheckpointKey: heartbeatCheckpointKey(step),
		RoundState:    roundState,
		TargetPaths:   patchPaths,
		Summary:       buildGeneratedPatchSummary(roundInput.RoundID, patchPaths),
		TotalTokens:   0,
	})
}

func buildPatchGenerationStartedSummary(roundID string, targetPaths []string) string {
	roundID = strings.TrimSpace(roundID)
	count := len(targetPaths)
	if roundID == "" {
		if count <= 0 {
			return "started patch generation"
		}
		if count == 1 {
			return "started patch generation for 1 file"
		}
		return fmt.Sprintf("started patch generation for %d files", count)
	}
	if count <= 0 {
		return fmt.Sprintf("round %s started patch generation", roundID)
	}
	if count == 1 {
		return fmt.Sprintf("round %s started patch generation for 1 file", roundID)
	}
	return fmt.Sprintf("round %s started patch generation for %d files", roundID, count)
}

func buildPatchGenerationFailedSummary(roundID string, targetPaths []string, reason error) string {
	base := buildPatchGenerationStartedSummary(roundID, targetPaths)
	reasonText := strings.TrimSpace(errorText(reason))
	if reasonText == "" {
		return strings.Replace(base, "started", "failed", 1)
	}
	return fmt.Sprintf("%s: %s", strings.Replace(base, "started", "failed", 1), reasonText)
}

func buildPatchApplyFailedSummary(roundID string, targetPaths []string, reason error) string {
	roundID = strings.TrimSpace(roundID)
	count := len(targetPaths)
	base := "patch apply failed"
	if roundID != "" {
		if count == 1 {
			base = fmt.Sprintf("round %s patch apply failed for 1 file", roundID)
		} else if count > 1 {
			base = fmt.Sprintf("round %s patch apply failed for %d files", roundID, count)
		} else {
			base = fmt.Sprintf("round %s patch apply failed", roundID)
		}
	} else if count == 1 {
		base = "patch apply failed for 1 file"
	} else if count > 1 {
		base = fmt.Sprintf("patch apply failed for %d files", count)
	}
	reasonText := strings.TrimSpace(errorText(reason))
	if reasonText == "" {
		return base
	}
	return fmt.Sprintf("%s: %s", base, reasonText)
}

func buildAppliedPatchSummary(roundID string, affectedPaths []string) string {
	roundID = strings.TrimSpace(roundID)
	count := len(affectedPaths)
	if roundID == "" {
		if count == 1 {
			return "applied patch to 1 file"
		}
		return fmt.Sprintf("applied patch to %d files", count)
	}
	if count == 1 {
		return fmt.Sprintf("round %s applied patch to 1 file", roundID)
	}
	return fmt.Sprintf("round %s applied patch to %d files", roundID, count)
}

func buildGeneratedPatchSummary(roundID string, patchPaths []string) string {
	roundID = strings.TrimSpace(roundID)
	count := len(patchPaths)
	if roundID == "" {
		if count == 1 {
			return "generated patch for 1 file"
		}
		return fmt.Sprintf("generated patch for %d files", count)
	}
	if count == 1 {
		return fmt.Sprintf("round %s generated patch for 1 file", roundID)
	}
	return fmt.Sprintf("round %s generated patch for %d files", roundID, count)
}

func workspacePatchPaths(patch *appruns.WorkspacePatch) []string {
	if patch == nil {
		return nil
	}
	paths := make([]string, 0, len(patch.ModifiedFiles)+len(patch.Operations))
	seen := map[string]struct{}{}
	appendPath := func(value string) {
		normalized := strings.TrimSpace(value)
		if normalized == "" {
			return
		}
		if _, ok := seen[normalized]; ok {
			return
		}
		seen[normalized] = struct{}{}
		paths = append(paths, normalized)
	}
	for _, value := range patch.ModifiedFiles {
		appendPath(value)
	}
	for _, operation := range patch.Operations {
		appendPath(operation.Path)
	}
	sort.Strings(paths)
	return paths
}

func errorText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
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
	RunID                string                      `json:"run_id"`
	JobID                string                      `json:"job_id"`
	WorkerID             string                      `json:"worker_id"`
	ExecutorImage        string                      `json:"executor_image,omitempty"`
	GoalSummary          string                      `json:"goal_summary,omitempty"`
	HumanNotes           json.RawMessage             `json:"human_notes,omitempty"`
	TaskBundle           []appruns.TaskBundleItem    `json:"task_bundle,omitempty"`
	AcceptanceChecks     []appruns.AcceptanceCheck   `json:"acceptance_checks,omitempty"`
	AllowedPaths         []string                    `json:"allowed_paths,omitempty"`
	ProtectedPaths       []string                    `json:"protected_paths,omitempty"`
	KnowledgePack        []appruns.ProfileSkill      `json:"knowledge_pack,omitempty"`
	WorkspacePath        string                      `json:"workspace_path"`
	ArtifactDir          string                      `json:"artifact_dir"`
	LaunchCommand        string                      `json:"launch_command"`
	LaunchArgs           []string                    `json:"launch_args"`
	LogPath              string                      `json:"log_path"`
	ArtifactManifestPath string                      `json:"artifact_manifest_path,omitempty"`
	MetricsPath          string                      `json:"metrics_path,omitempty"`
	IterationCount       int                         `json:"iteration_count,omitempty"`
	TotalTokens          int                         `json:"total_tokens,omitempty"`
	BuilderRuntime       *appruns.BuilderRuntimePlan `json:"builder_runtime,omitempty"`
}

type RunRecord = runRecord

func newRunRecord(run appruns.RunRecord) runRecord {
	return runRecord{
		RunID:                run.RunID,
		JobID:                run.JobID,
		WorkerID:             run.WorkerID,
		ExecutorImage:        run.ExecutorImage,
		GoalSummary:          run.GoalSummary,
		HumanNotes:           append(json.RawMessage(nil), run.HumanNotes...),
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

func (runner *Runner) prepareRunRecord(rawRun appruns.RunRecord) runRecord {
	run := newRunRecord(rawRun)
	if len(run.TaskBundle) > 0 {
		normalized := make([]appruns.TaskBundleItem, 0, len(run.TaskBundle))
		for _, task := range run.TaskBundle {
			normalized = append(normalized, appruns.NormalizeTaskBundleItem(task))
		}
		run.TaskBundle = normalized
	}
	if hasBuilderRuntimeConfig(runner.BuilderRuntimeConfig) {
		plan := resolveBuilderRuntimePlan(runner.BuilderRuntimeConfig, run.TaskBundle)
		run.BuilderRuntime = &plan
	}
	return run
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

func hasBuilderRuntimeConfig(cfg appconfig.BuilderRuntimeConfig) bool {
	return cfg.Enabled || cfg.DefaultModel != nil || cfg.UpgradeModel != nil || len(cfg.TaskRoutes) > 0 || cfg.UpgradeThreshold.MaxAttemptsBeforeUpgrade > 0 || cfg.UpgradeThreshold.MaxFilesBeforeUpgrade > 0 || cfg.UpgradeThreshold.UpgradeOnValidationFail || cfg.UpgradeThreshold.UpgradeOnPatchParseFail || cfg.UpgradeThreshold.UpgradeOnScopeViolation
}

func resolveBuilderRuntimePlan(cfg appconfig.BuilderRuntimeConfig, tasks []appruns.TaskBundleItem) appruns.BuilderRuntimePlan {
	plan := appruns.BuilderRuntimePlan{
		Enabled:      cfg.Enabled,
		DefaultModel: convertAgentModelConfig(cfg.DefaultModel),
		UpgradeModel: convertAgentModelConfig(cfg.UpgradeModel),
		UpgradeThreshold: appruns.BuilderRuntimeUpgradeThreshold{
			MaxAttemptsBeforeUpgrade: cfg.UpgradeThreshold.MaxAttemptsBeforeUpgrade,
			MaxFilesBeforeUpgrade:    cfg.UpgradeThreshold.MaxFilesBeforeUpgrade,
			UpgradeOnValidationFail:  cfg.UpgradeThreshold.UpgradeOnValidationFail,
			UpgradeOnPatchParseFail:  cfg.UpgradeThreshold.UpgradeOnPatchParseFail,
			UpgradeOnScopeViolation:  cfg.UpgradeThreshold.UpgradeOnScopeViolation,
		},
	}
	routesByType := map[appruns.BuilderRuntimeTaskType]appruns.BuilderRuntimeModelRef{}
	for _, route := range cfg.TaskRoutes {
		taskType := appruns.NormalizeBuilderRuntimeTaskType(route.TaskType)
		if taskType == "" {
			continue
		}
		routesByType[taskType] = convertAgentModelConfig(route.Model)
	}
	plan.TaskRoutes = make([]appruns.BuilderRuntimeTaskRoute, 0, len(tasks))
	for _, task := range tasks {
		resolvedTaskType := task.EffectiveTaskType()
		decision := appruns.BuilderRuntimeTaskRoute{
			TaskID:   task.TaskID,
			TaskType: resolvedTaskType,
		}
		if !cfg.Enabled {
			decision.RouteSource = "disabled"
			plan.TaskRoutes = append(plan.TaskRoutes, decision)
			continue
		}
		if model, ok := routesByType[resolvedTaskType]; ok && (model.Primary != "" || len(model.Fallbacks) > 0) {
			decision.RouteSource = "task_route"
			decision.Model = model
		} else if plan.DefaultModel.Primary != "" || len(plan.DefaultModel.Fallbacks) > 0 {
			decision.RouteSource = "default_model"
			decision.Model = plan.DefaultModel
		} else {
			decision.RouteSource = "unconfigured"
		}
		plan.TaskRoutes = append(plan.TaskRoutes, decision)
	}
	return plan
}

func convertAgentModelConfig(model *appconfig.AgentModelConfig) appruns.BuilderRuntimeModelRef {
	if model == nil {
		return appruns.BuilderRuntimeModelRef{}
	}
	return appruns.BuilderRuntimeModelRef{
		Primary:   strings.TrimSpace(model.Primary),
		Fallbacks: append([]string(nil), model.Fallbacks...),
	}
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
	if isDeviceVerificationCheck(checkID, check.Stage, check.Commands) {
		return deviceFailureSignatureForCheck(checkID)
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

func isDeviceVerificationCheck(checkID string, stage appruns.ExecutionStage, commands []string) bool {
	trimmedID := strings.TrimSpace(checkID)
	if strings.HasPrefix(trimmedID, "check-adb-") || strings.HasPrefix(trimmedID, "check-install-") {
		return true
	}
	if strings.Contains(trimmedID, "logcat") || strings.Contains(trimmedID, "screenshot") || strings.Contains(trimmedID, "launch-app") {
		return true
	}
	if stage == appruns.StageDevice {
		return true
	}
	if stage == appruns.StageSmoke {
		for _, command := range commands {
			trimmedCommand := strings.TrimSpace(command)
			if strings.HasPrefix(trimmedCommand, "adb ") || trimmedCommand == "adb" {
				return true
			}
		}
	}
	return false
}

func deviceFailureSignatureForCheck(checkID string) string {
	switch strings.TrimSpace(checkID) {
	case "check-adb-device-ready":
		return "device_check_failed:adb_device_unavailable"
	case "check-install-debug-apk":
		return "device_check_failed:apk_install_failed"
	case "check-launch-app-and-capture-logcat":
		return "device_check_failed:app_launch_failed"
	default:
		return "device_check_failed:verification_step_failed"
	}
}

const failureSignatureMarker = "__picoclaw_failure_signature__:"

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
	if runtimeErr, ok := builderRuntimeDiagnosis(execErr); ok {
		return executionFailureDiagnosis{
			Summary:            runtimeErr.summary,
			RecoverySuggestion: runtimeErr.recoverySuggestion,
			Signature:          runtimeErr.signature,
			NextAction:         appruns.ControlActionStop,
			Policy:             runtimeErr.policy,
			PreserveWorkspace:  runtimeErr.preserveWorkspace,
			ResumeAllowed:      runtimeErr.resumeAllowed,
		}
	}
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
		if isDeviceVerificationCheck(checkID, step.Check.Stage, step.Check.Commands) {
			logText := strings.ToLower(strings.TrimSpace(readExecutionLogForFailureDiagnosis(workspacePath, logPath)))
			diagnosis.Summary = fmt.Sprintf("device verification check %s failed: %s", checkID, execErr.Error())
			diagnosis.Signature = failureSignatureForCheck(*step.Check)
			if signature := markedFailureSignatureFromLog(logText); signature != "" {
				diagnosis.Signature = signature
			}
			diagnosis.RecoverySuggestion = deviceVerificationRecoverySuggestion(diagnosis.Signature, checkID)
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
				TerminationReason:   "device verification requires manual inspection before rerun",
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

func deviceVerificationRecoverySuggestion(signature, checkID string) string {
	switch strings.TrimSpace(signature) {
	case "environment_check_failed:adb_binary_unavailable":
		return "install adb in the builder environment or expose Android platform-tools before rerun"
	case "environment_check_failed:debug_apk_missing":
		return "confirm the debug apk was built before device installation starts"
	case "environment_check_failed:android_app_id_missing":
		return "set APPFACTORY_ANDROID_APP_ID or fix applicationId detection before rerun"
	case "device_check_failed:adb_device_unavailable":
		return "connect the target device or fix adb connectivity before rerun"
	case "device_check_failed:apk_install_failed":
		return "fix adb install prerequisites or clear conflicting packages before rerun"
	case "device_check_failed:app_launch_failed":
		return "inspect launcher intent resolution and installed package state before rerun"
	case "device_check_failed:app_runtime_crash":
		return "inspect logcat and app startup behavior before rerun"
	case "device_check_failed:log_capture_failed":
		return "fix adb logcat access and ensure runtime evidence can be captured before rerun"
	default:
		switch strings.TrimSpace(checkID) {
		case "check-adb-device-ready":
			return "connect the target device or fix adb connectivity before rerun"
		case "check-install-debug-apk":
			return "fix adb install prerequisites or clear conflicting packages before rerun"
		default:
			return "inspect device connectivity and runtime evidence before rerun"
		}
	}
}

func markedFailureSignatureFromLog(logText string) string {
	trimmed := strings.TrimSpace(logText)
	if trimmed == "" {
		return ""
	}
	index := strings.LastIndex(trimmed, failureSignatureMarker)
	if index < 0 {
		return ""
	}
	marked := trimmed[index+len(failureSignatureMarker):]
	if newline := strings.Index(marked, "\n"); newline >= 0 {
		marked = marked[:newline]
	}
	return strings.TrimSpace(marked)
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
	taskBundle := make([]appruns.TaskBundleItem, 0, len(plannedRun.TaskBundle))
	for _, task := range plannedRun.TaskBundle {
		taskBundle = append(taskBundle, appruns.NormalizeTaskBundleItem(task))
	}
	return appruns.RoundInput{
		RoundID:          "round-1",
		Attempt:          maxInt(plannedRun.IterationCount, 1),
		GoalSummary:      plannedRun.GoalSummary,
		TaskBundle:       taskBundle,
		AllowedPaths:     append([]string(nil), plannedRun.AllowedPaths...),
		KnowledgePack:    append([]appruns.ProfileSkill(nil), plannedRun.KnowledgePack...),
		AcceptanceChecks: append([]appruns.AcceptanceCheck(nil), plannedRun.AcceptanceChecks...),
		BuilderRuntime:   plannedRun.BuilderRuntime,
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

func buildSmokeReport(passedChecks, failedChecks []appruns.CheckResult, deviceFailureCategories []appruns.DeviceFailureCategoryStat) string {
	var builder strings.Builder
	builder.WriteString("# 冒烟报告\n\n")
	foundDevice := appendSmokeCheckSection(&builder, "设备验证", appruns.StageDevice, passedChecks, failedChecks)
	foundSmoke := appendSmokeCheckSection(&builder, "冒烟检查", appruns.StageSmoke, passedChecks, failedChecks)
	if len(deviceFailureCategories) > 0 {
		appendDeviceFailureCategorySection(&builder, deviceFailureCategories)
	}
	if !foundDevice && !foundSmoke {
		builder.WriteString("当前运行未包含 device / smoke 阶段检查。\n")
	}
	return builder.String()
}

func buildDeviceFailureCategoryStats(failureSignatures []appruns.FailureSignature) []appruns.DeviceFailureCategoryStat {
	if len(failureSignatures) == 0 {
		return nil
	}
	stats := make([]appruns.DeviceFailureCategoryStat, 0)
	indexByCategory := make(map[string]int)
	for _, signature := range failureSignatures {
		category := strings.TrimSpace(signature.Signature)
		domain := deviceFailureDomainForMetrics(category)
		if domain == "" {
			continue
		}
		count := signature.Count
		if count <= 0 {
			count = 1
		}
		if idx, ok := indexByCategory[category]; ok {
			stats[idx].Count += count
			continue
		}
		indexByCategory[category] = len(stats)
		stats = append(stats, appruns.DeviceFailureCategoryStat{
			Category:      category,
			FailureDomain: domain,
			Count:         count,
		})
	}
	return stats
}

func deviceFailureDomainForMetrics(category string) string {
	normalized := strings.TrimSpace(category)
	switch {
	case strings.HasPrefix(normalized, "device_check_failed:"):
		return "device"
	case normalized == "environment_check_failed:adb_binary_unavailable":
		return "environment"
	case normalized == "environment_check_failed:debug_apk_missing":
		return "environment"
	case normalized == "environment_check_failed:android_app_id_missing":
		return "environment"
	default:
		return ""
	}
}

func appendDeviceFailureCategorySection(builder *strings.Builder, deviceFailureCategories []appruns.DeviceFailureCategoryStat) {
	builder.WriteString("## 设备失败类别\n\n")
	for _, item := range deviceFailureCategories {
		builder.WriteString("- ")
		builder.WriteString(item.FailureDomain)
		builder.WriteString("：")
		builder.WriteString(item.Category)
		builder.WriteString(" (")
		builder.WriteString(strconv.Itoa(item.Count))
		builder.WriteString(" 次)\n")
	}
	builder.WriteString("\n")
}

func appendSmokeCheckSection(builder *strings.Builder, title string, stage appruns.ExecutionStage, passedChecks, failedChecks []appruns.CheckResult) bool {
	found := false
	for _, check := range passedChecks {
		if check.Stage != stage {
			continue
		}
		if !found {
			builder.WriteString("## ")
			builder.WriteString(title)
			builder.WriteString("\n\n")
		}
		found = true
		builder.WriteString("- 通过：")
		builder.WriteString(check.CheckID)
		builder.WriteString("\n")
	}
	for _, check := range failedChecks {
		if check.Stage != stage {
			continue
		}
		if !found {
			builder.WriteString("## ")
			builder.WriteString(title)
			builder.WriteString("\n\n")
		}
		found = true
		builder.WriteString("- 失败：")
		builder.WriteString(check.CheckID)
		builder.WriteString("\n")
	}
	if found {
		builder.WriteString("\n")
	}
	return found
}

func buildRuntimeArtifactOutputs(run runRecord, jobRoot, changeSummaryPath, buildReportPath, smokeReportPath string) ([]appruns.ArtifactItem, []appruns.ArtifactPointer) {
	items := []appruns.ArtifactItem{
		{
			ArtifactID:   "run-log",
			Path:         run.LogPath,
			ArtifactType: "log",
			Produced:     true,
			Required:     true,
			Label:        "builder run log",
		},
		{
			ArtifactID:   "change-summary",
			Path:         relFromJobRoot(jobRoot, changeSummaryPath),
			ArtifactType: "report",
			Produced:     true,
			Label:        "change summary",
		},
		{
			ArtifactID:   "build-report",
			Path:         relFromJobRoot(jobRoot, buildReportPath),
			ArtifactType: "report",
			Produced:     true,
			Label:        "build report",
		},
		{
			ArtifactID:   "smoke-test-report",
			Path:         relFromJobRoot(jobRoot, smokeReportPath),
			ArtifactType: "report",
			Produced:     true,
			Label:        "smoke test report",
		},
	}
	primaryOutputs := []appruns.ArtifactPointer{{
		ArtifactID:   "run-log",
		Path:         run.LogPath,
		ArtifactType: "log",
		Label:        "builder run log",
	}}
	optionalArtifacts := []struct {
		artifactID   string
		path         string
		artifactType string
		label        string
		primary      bool
	}{
		{
			artifactID:   "debug-apk",
			path:         filepath.Join(run.WorkspacePath, "build", "app", "outputs", "flutter-apk", "app-debug.apk"),
			artifactType: "apk",
			label:        "debug apk",
			primary:      true,
		},
		{
			artifactID:   "device-logcat",
			path:         filepath.Join(jobRoot, "reports", "device-logcat.txt"),
			artifactType: "log",
			label:        "device logcat",
		},
		{
			artifactID:   "device-screenshot",
			path:         filepath.Join(jobRoot, "reports", "device-screenshot.png"),
			artifactType: "image",
			label:        "device screenshot",
		},
	}
	for _, artifact := range optionalArtifacts {
		if _, err := os.Stat(artifact.path); err != nil {
			continue
		}
		relPath := relFromJobRoot(jobRoot, artifact.path)
		items = append(items, appruns.ArtifactItem{
			ArtifactID:   artifact.artifactID,
			Path:         relPath,
			ArtifactType: artifact.artifactType,
			Produced:     true,
			Label:        artifact.label,
		})
		if artifact.primary {
			primaryOutputs = append(primaryOutputs, appruns.ArtifactPointer{
				ArtifactID:   artifact.artifactID,
				Path:         relPath,
				ArtifactType: artifact.artifactType,
				Label:        artifact.label,
			})
		}
	}
	return items, primaryOutputs
}

func dockerPrepareCommand(ctx context.Context, run runRecord) (*exec.Cmd, error) {
	return dockerCommand(ctx, run, append([]string{run.LaunchCommand}, run.LaunchArgs...), nil)
}

func dockerCommand(ctx context.Context, run runRecord, argv []string, extraEnv []string) (*exec.Cmd, error) {
	root := appFactoryRootFromWorkspace(run.WorkspacePath)
	if err := ensureDockerWritablePaths(ctx, root, run); err != nil {
		return nil, err
	}
	gradleUserHome := resolveBuilderGradleUserHome(root)
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
	if err := ensureBuilderGradleUserHome(gradleUserHome); err != nil {
		return nil, fmt.Errorf("create builder gradle user home: %w", err)
	}
	args = append(args, "-e", "PUB_CACHE="+pubCacheDir)
	args = append(args, "-e", "GRADLE_USER_HOME="+gradleUserHome)
	args = append(args, dockerEnvArgs(
		"APPFACTORY_BUILDER_DOCKER_NETWORK",
		"APPFACTORY_BUILDER_RUNTIME",
		"APPFACTORY_DEVICE_VERIFICATION_ENABLED",
		"APPFACTORY_DEVICE_SERIAL",
		"APPFACTORY_ANDROID_APP_ID",
		"APPFACTORY_DEVICE_CAPTURE_SCREENSHOT",
		"ADB_SERVER_SOCKET",
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
	paths = append(paths, filepath.Clean(resolveBuilderGradleUserHome(root)))

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

func resolveBuilderGradleUserHome(root string) string {
	return filepath.Join(root, ".runtime", "gradle-user-home")
}

func ensureBuilderGradleUserHome(path string) error {
	for _, child := range []string{"", filepath.Join("wrapper", "dists")} {
		target := path
		if child != "" {
			target = filepath.Join(path, child)
		}
		if err := os.MkdirAll(target, 0o755); err != nil {
			return err
		}
	}
	return nil
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}
