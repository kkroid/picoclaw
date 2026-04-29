package adapter

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"maps"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
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

var aapt2PermissionDeniedPattern = regexp.MustCompile(`Cannot run program "([^"]+/aapt2)": error=13, Permission denied`)
var aapt2DaemonStartupFailurePattern = regexp.MustCompile(`(?i)AAPT2 .*Daemon #\d+: Daemon startup failed`)
var aapt2ResourceLinkFailurePattern = regexp.MustCompile(`(?i)(:app:processDebugResources|LinkApplicationAndroidResourcesTask)`)
var dartValidationFailurePathPattern = regexp.MustCompile(`(?m)(?:^|[\s"'(])((?:lib|test)/[A-Za-z0-9_./-]+\.dart)(?:[:\s"')]|$)`)
var dartValidationFailureWorkspacePathPattern = regexp.MustCompile(`(?m)(?:file://)?[A-Za-z0-9_./:-]*/((?:lib|test)/[A-Za-z0-9_./-]+\.dart)(?:[:\s"')]|$)`)
var dartValidationIssueLinePattern = regexp.MustCompile(`(?m)^\s*(?:warning|error)\s+•\s+.*$`)
var dartUnusedImportFailurePattern = regexp.MustCompile(`(?m)^\s*warning • Unused import: ['"]([^'"]+)['"] • ((?:lib|test)/[A-Za-z0-9_./-]+\.dart):\d+:\d+ • unused_import$`)
var dartDeprecatedWithOpacityFailurePattern = regexp.MustCompile(`(?m)^\s*info • 'withOpacity' is deprecated and shouldn't be used\. Use \.withValues\(\) to avoid precision loss • ((?:lib|test)/[A-Za-z0-9_./-]+\.dart):\d+:\d+ • deprecated_member_use$`)
var dartLocalImportPattern = regexp.MustCompile(`(?m)^\s*(?:import|export|part)\s+['"]([^'"]+)['"]`)
var pubspecPackageNamePattern = regexp.MustCompile(`(?m)^name:\s*['"]?([A-Za-z0-9_]+)['"]?\s*$`)
var flutterTestProgressLinePattern = regexp.MustCompile(`^\d{2}:\d{2}\s+\+\d+(?:\s+-\d+)?:`)

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
	run, err = runner.enrichRunRecordWithSemanticChecks(run)
	if err != nil {
		return err
	}
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
		task := executionTask(nil)
		taskMeta := taskMetadata{}
		heartbeatState := heartbeatRoundStateForStep(step)
		heartbeatSummary := step.Summary
		heartbeatTargetPaths := collectHeartbeatTargetPaths(roundInput)
		if step.Command != nil {
			task = runner.buildExecutionTask(run, step, roundInput)
			taskMeta = task.Metadata()
			heartbeatState, heartbeatSummary, heartbeatTargetPaths = buildTaskHeartbeat(run, step, roundInput, taskMeta)
		}
		if err := backend.Heartbeat(ctx, runID, appruns.Heartbeat{
			Stage:         step.Stage,
			Iteration:     index + 1,
			RoundID:       roundInput.RoundID,
			Attempt:       maxInt(roundInput.Attempt, 1),
			CheckpointKey: heartbeatCheckpointKey(step, heartbeatState),
			RoundState:    heartbeatState,
			TargetPaths:   heartbeatTargetPaths,
			Summary:       heartbeatSummary,
			TotalTokens:   0,
		}); err != nil {
			return err
		}
		if step.Command == nil {
			continue
		}
		taskResult := task.Execute(ctx, taskEnvironment{
			Runner:     runner,
			Backend:    backend,
			RunID:      runID,
			Run:        run,
			RoundInput: roundInput,
		})
		commandRuns += taskResult.CommandRuns
		if taskResult.BuilderRuntime != nil {
			if taskMeta.Kind == taskKindValidationCheck {
				builderRuntimeStats = mergeBuilderRuntimeStats(builderRuntimeStats, taskResult.BuilderRuntime)
			} else {
				builderRuntimeStats = taskResult.BuilderRuntime
			}
			roundOutput.BuilderRuntime = builderRuntimeStats
		}
		if taskResult.WorkspacePatch != nil {
			roundOutput.WorkspacePatch = taskResult.WorkspacePatch
			if taskResult.ApplyResult.Status != "" {
				roundOutput.WorkspacePatch.Status = taskResult.ApplyResult.Status
			}
		}
		if len(taskResult.ApplyResult.ModifiedFiles) > 0 {
			modifiedFiles = append(modifiedFiles, taskResult.ApplyResult.ModifiedFiles...)
			appliedTargetPaths := append([]string(nil), taskResult.AppliedTargetPaths...)
			if len(appliedTargetPaths) == 0 {
				appliedTargetPaths = append([]string(nil), heartbeatTargetPaths...)
			}
			if taskResult.AppliedRoundState != nil {
				appliedRoundInput := roundInput
				if taskResult.AppliedRoundInput != nil {
					appliedRoundInput = *taskResult.AppliedRoundInput
				}
				if err := runner.reportAppliedPatch(ctx, backend, runID, step, appliedRoundInput, taskResult.AppliedRoundState, appliedTargetPaths, taskResult.ApplyResult.ModifiedFiles); err != nil {
					return err
				}
			}
		}
		if taskResult.UpdatedRunRoundState != nil {
			run.RoundState = cloneRoundStateRef(taskResult.UpdatedRunRoundState)
		}
		execErr := taskResult.Err
		if step.Check == nil {
			if execErr != nil {
				diagnosis := diagnoseExecutionStepFailure(step, execErr, run.WorkspacePath, run.LogPath, taskResult.WorkspacePatchApplyFailed)
				failureSummary = diagnosis.Summary
				recoverySuggestion = diagnosis.RecoverySuggestion
				existingFailureState := run.RoundState
				if diagnosis.RoundState != nil {
					existingFailureState = diagnosis.RoundState
				}
				failureRoundState = mergeRoundStateMetadata(buildRoundState(
					[]appruns.RoundPhase{appruns.RoundPhaseInspect, appruns.RoundPhaseEdit, appruns.RoundPhaseRepair, appruns.RoundPhaseFinalize},
					appruns.RoundPhaseFinalize,
					diagnosis.NextAction,
					diagnosis.PreserveWorkspace,
					diagnosis.ResumeAllowed,
				), existingFailureState, failureRoundStateTaskID(run, step))
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
			result.Details = strings.TrimSpace(taskResult.CheckDetails)
			if result.Details == "" {
				result.Details = "command chain completed"
			}
			passedChecks = append(passedChecks, result)
			continue
		}
		if step.Check.Required && !step.Check.AllowFailure {
			result.Outcome = "failed"
			result.Details = execErr.Error()
			failedChecks = append(failedChecks, result)
			diagnosis := diagnoseExecutionStepFailure(step, execErr, run.WorkspacePath, run.LogPath, false)
			failureSummary = diagnosis.Summary
			recoverySuggestion = diagnosis.RecoverySuggestion
			failureRoundState = mergeRoundStateMetadata(buildRoundState(
				[]appruns.RoundPhase{appruns.RoundPhaseInspect, appruns.RoundPhaseEdit, appruns.RoundPhaseValidate, appruns.RoundPhaseRepair, appruns.RoundPhaseFinalize},
				appruns.RoundPhaseFinalize,
				diagnosis.NextAction,
				diagnosis.PreserveWorkspace,
				diagnosis.ResumeAllowed,
			), run.RoundState, failureRoundStateTaskID(run, step))
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
		failureRoundState = mergeRoundStateMetadata(buildRoundState([]appruns.RoundPhase{appruns.RoundPhaseInspect, appruns.RoundPhaseEdit, appruns.RoundPhaseValidate, appruns.RoundPhaseRepair, appruns.RoundPhaseFinalize}, appruns.RoundPhaseFinalize, appruns.ControlActionStop, true, false), run.RoundState, failureRoundStateTaskID(run, step))
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
	semanticReviewInputs := buildSemanticReviewInputs(jobRoot, buildReportPath, smokeReportPath, run.SemanticChecks)
	validationResults := buildValidationResults(passedChecks, failedChecks, run.SemanticChecks, semanticReviewInputs)
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
	nextHumanActions := []appruns.HumanAction{{
		ActionID: "review-run-result",
		Summary:  "检查运行日志和产物",
		Reason:   "P0 阶段仍需人工确认实际输出",
		Owner:    "engineering",
	}}
	if action := buildSemanticReviewHumanAction(run.SemanticChecks, semanticReviewInputs); action != nil {
		nextHumanActions = append(nextHumanActions, *action)
	}
	return backend.Complete(ctx, runID, appruns.BuildOutput{
		SchemaVersion:    "0.1.0",
		JobID:            run.JobID,
		Status:           "success",
		ExitReason:       "completed",
		WorkerID:         run.WorkerID,
		StartedAt:        &startedAt,
		FinishedAt:       &finishedAt,
		FinalSummary:     finalSummary,
		ModifiedFiles:    modifiedFiles,
		ChecksPassed:     passedChecks,
		ChecksFailed:     failedChecks,
		NextHumanActions: nextHumanActions,
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
	if shouldSkipAutomaticValidationRepair(run.LogPath, *step.Check) {
		return nil, false, nil
	}
	repairTaskType, ok := validationRepairTaskType(*step.Check)
	if !ok {
		return nil, false, nil
	}
	const maxValidationRepairRounds = 4
	baseTasks := plannedExecutionRun(run).TaskBundle
	planSeed := run
	var combined *builderRuntimeExecutionResult
	for repairRound := 0; repairRound < maxValidationRepairRounds; repairRound++ {
		if repaired, repairErr := repairDeterministicValidationFailure(run.WorkspacePath, run.LogPath, *step.Check); repairErr != nil {
			return combined, true, repairErr
		} else if repaired {
			retryErr := runner.rerunStep(step, run.WorkspacePath, run.LogPath)
			if retryErr == nil {
				return combined, true, nil
			}
			continue
		}
		failureContext := extractValidationFailureContext(run.LogPath, step.Check.CheckID)
		directFailurePaths := extractValidationFailurePathsForCheck(run.WorkspacePath, run.LogPath, step.Check.CheckID)
		failurePaths := expandValidationFailurePathsWithLocalImports(run.WorkspacePath, directFailurePaths)
		scopedTasks := selectValidationRepairContextTasks(baseTasks, failurePaths)
		targetPaths := concreteTaskTargetPaths(scopedTasks)
		if len(targetPaths) == 0 {
			targetPaths = concreteTaskTargetPaths(run.TaskBundle)
		}
		if len(targetPaths) == 0 {
			targetPaths = concreteAllowedPaths(run.AllowedPaths)
		}
		repairTargetPaths := buildValidationRepairTargetPaths(repairTaskType, directFailurePaths, failurePaths, targetPaths)
		if len(repairTargetPaths) == 0 {
			repairTargetPaths = targetPaths
		}
		repairTask := appruns.TaskBundleItem{
			TaskID:      "repair-" + strings.TrimSpace(step.Check.CheckID),
			Title:       "Repair " + strings.TrimSpace(step.Check.Label),
			Category:    appruns.TaskCategoryValidation,
			TaskType:    repairTaskType,
			Objective:   fmt.Sprintf("repair the workspace so %s passes", strings.TrimSpace(step.Check.CheckID)),
			TargetPaths: repairTargetPaths,
			CompletionCriteria: []string{
				fmt.Sprintf("%s passes", strings.TrimSpace(step.Check.CheckID)),
			},
		}
		repairRun := run
		repairRun.TaskBundle = buildValidationRepairTaskBundle(scopedTasks, repairTask)
		if hasBuilderRuntimeConfig(runner.BuilderRuntimeConfig) {
			plan := resolveBuilderRuntimePlan(runner.BuilderRuntimeConfig, repairRun.TaskBundle)
			repairRun.BuilderRuntime = mergeBuilderRuntimePlanFallbacks(&plan, planSeed.BuilderRuntime)
			expandBuilderRuntimePlanModelFallbacks(repairRun.BuilderRuntime, runner.ModelCatalog)
		}
		repairRoundInput := roundInput
		repairRoundInput.Attempt = maxInt(roundInput.Attempt+1+repairRound, 2)
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
				return combined, true, err
			}
		}
		result, err := runner.executeBuilderRuntimeEditWithFailureContext(ctx, backend, runID, step, repairRun, repairRoundInput, failureContext)
		combined = mergeBuilderRuntimeResults(combined, result)
		if err != nil {
			return combined, true, err
		}
		retryErr := runner.rerunStep(step, run.WorkspacePath, run.LogPath)
		if retryErr == nil {
			return combined, true, nil
		}
		planSeed = repairRun
		if shouldUpgradeValidationRepair(repairRun, *step.Check) && !builderRuntimeStatsAlreadyUpgraded(combined) {
			upgradeRun := forceBuilderRuntimeUpgradeForTask(repairRun, repairTask)
			upgradeRoundInput := repairRoundInput
			upgradeRoundInput.Attempt = maxInt(repairRoundInput.Attempt+1, 3)
			upgradeRoundInput.TaskBundle = upgradeRun.TaskBundle
			upgradeRoundInput.BuilderRuntime = upgradeRun.BuilderRuntime
			upgradeResult, upgradeErr := runner.executeBuilderRuntimeEditWithFailureContext(ctx, backend, runID, step, upgradeRun, upgradeRoundInput, failureContext)
			if upgradeResult != nil && upgradeResult.Stats != nil {
				upgradeResult.Stats.UpgradeApplied = true
				if strings.TrimSpace(upgradeResult.Stats.RouteSource) == "" || upgradeResult.Stats.RouteSource == "task_route" {
					upgradeResult.Stats.RouteSource = "upgrade_threshold"
				}
			}
			combined = mergeBuilderRuntimeResults(combined, upgradeResult)
			if upgradeErr != nil {
				return combined, true, upgradeErr
			}
			if retryErr = runner.rerunStep(step, run.WorkspacePath, run.LogPath); retryErr == nil {
				return combined, true, nil
			}
			planSeed = upgradeRun
		}
		if repairRound == maxValidationRepairRounds-1 {
			return combined, true, retryErr
		}
	}
	return combined, true, nil
}

func repairDeterministicValidationFailure(workspacePath, logPath string, check CheckExecutionPreview) (bool, error) {
	if strings.TrimSpace(check.CheckID) != "check-flutter-analyze" {
		return false, nil
	}
	if repaired, err := repairFlutterAnalyzeUnusedImports(workspacePath, logPath); repaired || err != nil {
		return repaired, err
	}
	return repairFlutterAnalyzeDeprecatedWithOpacity(workspacePath, logPath)
}

func validationRepairTaskType(check CheckExecutionPreview) (appruns.BuilderRuntimeTaskType, bool) {
	if isSemanticValidationCheck(check) {
		return appruns.BuilderRuntimeTaskTypeDualFileWiring, true
	}
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

func preferredValidationRepairTask(tasks []appruns.TaskBundleItem, taskType appruns.BuilderRuntimeTaskType) appruns.TaskBundleItem {
	for _, task := range tasks {
		normalized := appruns.NormalizeTaskBundleItem(task)
		if normalized.EffectiveTaskType() == taskType {
			return normalized
		}
	}
	return appruns.TaskBundleItem{}
}

func isSemanticValidationCheck(check CheckExecutionPreview) bool {
	checkID := strings.ToLower(strings.TrimSpace(check.CheckID))
	label := strings.ToLower(strings.TrimSpace(check.Label))
	semanticTokens := []string{
		"semantic",
		"branding",
		"domain-language",
		"domain-wording",
		"domain-branding",
		"copy",
		"summary",
		"wording",
		"语义",
		"文案",
		"品牌",
	}
	for _, token := range semanticTokens {
		if strings.Contains(checkID, token) || strings.Contains(label, token) {
			return true
		}
	}
	return false
}

func shouldUpgradeValidationRepair(run runRecord, check CheckExecutionPreview) bool {
	if run.BuilderRuntime == nil || strings.TrimSpace(run.BuilderRuntime.UpgradeModel.Primary) == "" {
		return false
	}
	if isSemanticValidationCheck(check) {
		return run.BuilderRuntime.UpgradeThreshold.UpgradeOnSemanticConflict
	}
	return run.BuilderRuntime.UpgradeThreshold.UpgradeOnValidationFail
}

func builderRuntimeStatsAlreadyUpgraded(result *builderRuntimeExecutionResult) bool {
	if result == nil || result.Stats == nil {
		return false
	}
	return result.Stats.UpgradeApplied
}

func forceBuilderRuntimeUpgradeForTask(run runRecord, task appruns.TaskBundleItem) runRecord {
	if run.BuilderRuntime == nil {
		return run
	}
	plan := *run.BuilderRuntime
	routes := make([]appruns.BuilderRuntimeTaskRoute, 0, len(plan.TaskRoutes)+1)
	for _, route := range plan.TaskRoutes {
		if route.TaskID == task.TaskID {
			continue
		}
		routes = append(routes, route)
	}
	plan.TaskRoutes = append([]appruns.BuilderRuntimeTaskRoute{{
		TaskID:      task.TaskID,
		TaskType:    task.EffectiveTaskType(),
		RouteSource: "upgrade_threshold",
		Model:       plan.UpgradeModel,
	}}, routes...)
	run.BuilderRuntime = &plan
	return run
}

func mergeBuilderRuntimeResults(base, extra *builderRuntimeExecutionResult) *builderRuntimeExecutionResult {
	if base == nil {
		return extra
	}
	if extra == nil {
		return base
	}
	merged := *base
	merged.AppliedRoundInput = cloneRoundInputRef(base.AppliedRoundInput)
	merged.Stats = mergeBuilderRuntimeStats(base.Stats, extra.Stats)
	if extra.Patch != nil {
		merged.Patch = extra.Patch
	}
	if extra.ApplyResult.PatchID != "" || extra.ApplyResult.Status != "" || len(extra.ApplyResult.ModifiedFiles) > 0 {
		merged.ApplyResult = extra.ApplyResult
		merged.AppliedRoundInput = cloneRoundInputRef(extra.AppliedRoundInput)
	}
	return &merged
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

func heartbeatCheckpointKey(step ExecutionStep, roundState *appruns.RoundState) string {
	if roundState != nil {
		if currentTaskID := strings.TrimSpace(roundState.CurrentTaskID); currentTaskID != "" {
			return currentTaskID
		}
	}
	if step.Check != nil {
		return strings.TrimSpace(step.Check.CheckID)
	}
	return strings.TrimSpace(step.StepID)
}

func builderRuntimeTaskSummary(tasks []appruns.TaskBundleItem, taskID string) string {
	normalizedTaskID := strings.TrimSpace(taskID)
	if normalizedTaskID == "" {
		return ""
	}
	for _, candidate := range tasks {
		normalized := appruns.NormalizeTaskBundleItem(candidate)
		if strings.TrimSpace(normalized.TaskID) != normalizedTaskID {
			continue
		}
		summary := strings.TrimSpace(normalized.Title)
		if summary == "" {
			summary = strings.TrimSpace(normalized.Objective)
		}
		if summary == "" {
			summary = normalizedTaskID
		}
		return fmt.Sprintf("builder-runtime task %s: %s", normalizedTaskID, summary)
	}
	return normalizedTaskID
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
	if len(extra.ModelSequence) > 0 {
		merged.ModelSequence = append(append([]string(nil), merged.ModelSequence...), extra.ModelSequence...)
	}
	merged.UpgradeApplied = merged.UpgradeApplied || extra.UpgradeApplied
	merged.Attempts += extra.Attempts
	merged.PromptTokens += extra.PromptTokens
	merged.CompletionTokens += extra.CompletionTokens
	merged.TotalTokens += extra.TotalTokens
	merged.OperationCount += extra.OperationCount
	merged.TargetedOperationCount += extra.TargetedOperationCount
	merged.UnrelatedOperationCount += extra.UnrelatedOperationCount
	if extra.UnrelatedOperationRate > 0 {
		merged.UnrelatedOperationRate = extra.UnrelatedOperationRate
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

func buildValidationRepairTaskBundle(tasks []appruns.TaskBundleItem, repairTask appruns.TaskBundleItem) []appruns.TaskBundleItem {
	bundle := make([]appruns.TaskBundleItem, 0, len(tasks)+1)
	bundle = append(bundle, appruns.NormalizeTaskBundleItem(repairTask))
	for _, task := range tasks {
		normalized := appruns.NormalizeTaskBundleItem(task)
		if strings.TrimSpace(normalized.TaskID) == strings.TrimSpace(repairTask.TaskID) {
			continue
		}
		bundle = append(bundle, normalized)
	}
	return bundle
}

func extractValidationFailureContext(logPath, checkID string) string {
	content, err := os.ReadFile(logPath)
	if err != nil || len(content) == 0 {
		return ""
	}
	trimmedCheckID := strings.TrimSpace(checkID)
	switch trimmedCheckID {
	case "check-flutter-analyze":
		if block := extractLastFlutterAnalyzeFailureBlock(string(content)); block != "" {
			return fmt.Sprintf("check=%s\n%s", trimmedCheckID, block)
		}
	case "check-flutter-test":
		if block := extractLastFlutterTestFailureBlock(string(content)); block != "" {
			return fmt.Sprintf("check=%s\n%s", trimmedCheckID, block)
		}
	}
	const maxBytes = 12000
	if len(content) > maxBytes {
		content = content[len(content)-maxBytes:]
	}
	trimmed := strings.TrimSpace(string(content))
	if trimmed == "" {
		return ""
	}
	if trimmedCheckID == "" {
		return trimmed
	}
	return fmt.Sprintf("check=%s\n%s", trimmedCheckID, trimmed)
}

func extractLastFlutterAnalyzeFailureBlock(content string) string {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	content = strings.ReplaceAll(content, "\r", "\n")
	lines := strings.Split(content, "\n")
	end := -1
	for idx := len(lines) - 1; idx >= 0; idx-- {
		if strings.Contains(lines[idx], "issue found.") || strings.Contains(lines[idx], "issues found.") {
			end = idx
			break
		}
	}
	if end < 0 {
		return ""
	}
	start := -1
	for idx := end; idx >= 0; idx-- {
		if strings.Contains(lines[idx], "Analyzing workspace") {
			start = idx
			break
		}
	}
	if start < 0 || start > end {
		return ""
	}
	return strings.TrimSpace(strings.Join(lines[start:end+1], "\n"))
}

func extractLastFlutterTestFailureBlock(content string) string {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	content = strings.ReplaceAll(content, "\r", "\n")
	lines := strings.Split(content, "\n")
	start := -1
	for idx := len(lines) - 1; idx >= 0; idx-- {
		line := lines[idx]
		if strings.Contains(line, "loading ") && strings.Contains(line, "_test.dart") {
			start = idx
			break
		}
		if strings.Contains(line, "flutter test") {
			start = idx
			break
		}
	}
	end := -1
	for idx := len(lines) - 1; idx >= 0; idx-- {
		line := lines[idx]
		if strings.Contains(line, "Test failed. See exception logs above.") || strings.Contains(line, "Some tests failed.") || strings.Contains(line, "TimeoutException after") || strings.Contains(line, "Test timed out after") || strings.Contains(line, "[E]") {
			end = idx
			break
		}
	}
	if end < 0 {
		lastProgress := -1
		for idx := len(lines) - 1; idx >= 0; idx-- {
			if flutterTestProgressLinePattern.MatchString(strings.TrimSpace(lines[idx])) {
				lastProgress = idx
				break
			}
		}
		if start >= 0 && lastProgress >= start {
			return strings.TrimSpace(strings.Join(lines[start:lastProgress+1], "\n"))
		}
		return ""
	}
	if start < 0 || start > end {
		start = maxInt(0, end-80)
	}
	blockLines := lines[start : end+1]
	if compact := compactFlutterTestTimeoutBlock(blockLines); compact != "" {
		return compact
	}
	return strings.TrimSpace(strings.Join(blockLines, "\n"))
}

func compactFlutterTestTimeoutBlock(lines []string) string {
	if len(lines) == 0 {
		return ""
	}
	timeoutIndex := -1
	for idx := len(lines) - 1; idx >= 0; idx-- {
		line := strings.TrimSpace(lines[idx])
		if strings.Contains(line, "TimeoutException after") || strings.Contains(line, "Test timed out after") {
			timeoutIndex = idx
			break
		}
	}
	if timeoutIndex < 0 {
		return ""
	}
	result := make([]string, 0, 8)
	appendUniqueTrimmed := func(line string) {
		line = strings.TrimSpace(line)
		if line == "" {
			return
		}
		if len(result) > 0 && result[len(result)-1] == line {
			return
		}
		result = append(result, line)
	}
	loadingIndex := -1
	for idx, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.Contains(trimmed, "loading ") && strings.Contains(trimmed, "_test.dart") {
			loadingIndex = idx
		}
	}
	if loadingIndex >= 0 {
		appendUniqueTrimmed(lines[loadingIndex])
	}
	for idx := timeoutIndex - 1; idx >= 0; idx-- {
		if flutterTestProgressLinePattern.MatchString(strings.TrimSpace(lines[idx])) {
			appendUniqueTrimmed(lines[idx])
			break
		}
	}
	for idx := timeoutIndex; idx < len(lines); idx++ {
		appendUniqueTrimmed(lines[idx])
	}
	if len(result) == 0 {
		return ""
	}
	return strings.Join(result, "\n")
}

func repairFlutterAnalyzeUnusedImports(workspacePath, logPath string) (bool, error) {
	if strings.TrimSpace(workspacePath) == "" || strings.TrimSpace(logPath) == "" {
		return false, nil
	}
	content, err := os.ReadFile(logPath)
	if err != nil || len(content) == 0 {
		return false, nil
	}
	block := extractLastFlutterAnalyzeFailureBlock(string(content))
	if strings.TrimSpace(block) == "" {
		return false, nil
	}
	matches := dartUnusedImportFailurePattern.FindAllStringSubmatch(block, -1)
	if len(matches) == 0 {
		return false, nil
	}
	if issueLines := dartValidationIssueLinePattern.FindAllString(block, -1); len(issueLines) != len(matches) {
		return false, nil
	}
	type repairPlan struct {
		content []byte
		mode    os.FileMode
	}
	plans := make(map[string]repairPlan, len(matches))
	for _, match := range matches {
		if len(match) < 3 {
			return false, nil
		}
		importPath := strings.TrimSpace(match[1])
		relPath := filepath.ToSlash(strings.TrimSpace(match[2]))
		if importPath == "" || relPath == "" {
			return false, nil
		}
		plan, ok := plans[relPath]
		if !ok {
			absPath := filepath.Join(workspacePath, filepath.FromSlash(relPath))
			fileContent, readErr := os.ReadFile(absPath)
			if readErr != nil {
				return false, nil
			}
			info, statErr := os.Stat(absPath)
			if statErr != nil {
				return false, nil
			}
			plan = repairPlan{content: fileContent, mode: info.Mode().Perm()}
		}
		updated, removed := removeDartImportLine(plan.content, importPath)
		if !removed {
			return false, nil
		}
		plan.content = updated
		plans[relPath] = plan
	}
	for relPath, plan := range plans {
		absPath := filepath.Join(workspacePath, filepath.FromSlash(relPath))
		if writeErr := fileutil.WriteFileAtomic(absPath, plan.content, plan.mode); writeErr != nil {
			return false, fmt.Errorf("repair unused import in %s: %w", relPath, writeErr)
		}
	}
	return len(plans) > 0, nil
}

func repairFlutterAnalyzeDeprecatedWithOpacity(workspacePath, logPath string) (bool, error) {
	if strings.TrimSpace(workspacePath) == "" || strings.TrimSpace(logPath) == "" {
		return false, nil
	}
	content, err := os.ReadFile(logPath)
	if err != nil || len(content) == 0 {
		return false, nil
	}
	block := extractLastFlutterAnalyzeFailureBlock(string(content))
	if strings.TrimSpace(block) == "" {
		return false, nil
	}
	matches := dartDeprecatedWithOpacityFailurePattern.FindAllStringSubmatch(block, -1)
	if len(matches) == 0 {
		return false, nil
	}
	issueCount := 0
	for _, line := range strings.Split(strings.TrimSpace(block), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "warning •") || strings.HasPrefix(trimmed, "error •") || strings.HasPrefix(trimmed, "info •") {
			issueCount++
		}
	}
	if issueCount != len(matches) {
		return false, nil
	}
	type repairPlan struct {
		content []byte
		mode    os.FileMode
	}
	plans := make(map[string]repairPlan, len(matches))
	for _, match := range matches {
		if len(match) < 2 {
			return false, nil
		}
		relPath := filepath.ToSlash(strings.TrimSpace(match[1]))
		if relPath == "" {
			return false, nil
		}
		plan, ok := plans[relPath]
		if !ok {
			absPath := filepath.Join(workspacePath, filepath.FromSlash(relPath))
			fileContent, readErr := os.ReadFile(absPath)
			if readErr != nil {
				return false, nil
			}
			info, statErr := os.Stat(absPath)
			if statErr != nil {
				return false, nil
			}
			plan = repairPlan{content: fileContent, mode: info.Mode().Perm()}
		}
		updated := bytes.ReplaceAll(plan.content, []byte(".withOpacity("), []byte(".withValues(alpha: "))
		if bytes.Equal(updated, plan.content) {
			return false, nil
		}
		plan.content = updated
		plans[relPath] = plan
	}
	for relPath, plan := range plans {
		absPath := filepath.Join(workspacePath, filepath.FromSlash(relPath))
		if writeErr := fileutil.WriteFileAtomic(absPath, plan.content, plan.mode); writeErr != nil {
			return false, fmt.Errorf("repair deprecated withOpacity in %s: %w", relPath, writeErr)
		}
	}
	return len(plans) > 0, nil
}

func removeDartImportLine(content []byte, importPath string) ([]byte, bool) {
	normalized := strings.ReplaceAll(string(content), "\r\n", "\n")
	normalized = strings.ReplaceAll(normalized, "\r", "\n")
	lines := strings.Split(normalized, "\n")
	pattern := regexp.MustCompile(`^\s*import\s+['"]` + regexp.QuoteMeta(strings.TrimSpace(importPath)) + `['"]\s*;\s*$`)
	filtered := make([]string, 0, len(lines))
	removed := false
	for _, line := range lines {
		if pattern.MatchString(line) {
			removed = true
			continue
		}
		filtered = append(filtered, line)
	}
	if !removed {
		return content, false
	}
	updated := strings.Join(filtered, "\n")
	if strings.HasSuffix(normalized, "\n") && !strings.HasSuffix(updated, "\n") {
		updated += "\n"
	}
	return []byte(updated), true
}

func extractValidationFailurePaths(logPath string) []string {
	content, err := os.ReadFile(logPath)
	if err != nil || len(content) == 0 {
		return nil
	}
	return extractValidationFailurePathsFromContent(string(content))
}

func extractValidationFailurePathsForCheck(workspacePath, logPath, checkID string) []string {
	content, err := os.ReadFile(logPath)
	if err != nil || len(content) == 0 {
		return nil
	}
	trimmedCheckID := strings.TrimSpace(checkID)
	switch trimmedCheckID {
	case "check-flutter-analyze":
		if block := extractLastFlutterAnalyzeFailureBlock(string(content)); block != "" {
			return extractValidationFailurePathsFromContentForWorkspace(block, workspacePath)
		}
	case "check-flutter-test":
		if block := extractLastFlutterTestFailureBlock(string(content)); block != "" {
			return extractValidationFailurePathsFromContentForWorkspace(block, workspacePath)
		}
	}
	return extractValidationFailurePathsFromContentForWorkspace(string(content), workspacePath)
}

func extractValidationFailurePathsFromContent(content string) []string {
	return extractValidationFailurePathsFromContentForWorkspace(content, "")
}

func extractValidationFailurePathsFromContentForWorkspace(content, workspacePath string) []string {
	seen := map[string]struct{}{}
	paths := make([]string, 0)
	appendMatches := func(matches [][]string) {
		for _, match := range matches {
			if len(match) < 2 {
				continue
			}
			path := filepath.ToSlash(strings.TrimSpace(match[1]))
			if path == "" {
				continue
			}
			if _, ok := seen[path]; ok {
				continue
			}
			seen[path] = struct{}{}
			paths = append(paths, path)
		}
	}
	appendMatches(dartValidationFailurePathPattern.FindAllStringSubmatch(content, -1))
	appendMatches(dartValidationFailureWorkspacePathPattern.FindAllStringSubmatch(content, -1))
	if strings.TrimSpace(workspacePath) != "" {
		normalizedWorkspace := filepath.ToSlash(strings.TrimSpace(workspacePath))
		for _, candidate := range strings.FieldsFunc(content, func(r rune) bool {
			switch r {
			case '\n', '\r', '\t', ' ', '"', '\'', '(', ')':
				return true
			default:
				return false
			}
		}) {
			normalized := filepath.ToSlash(strings.TrimSpace(candidate))
			normalized = strings.TrimPrefix(normalized, "file://")
			if !strings.Contains(normalized, normalizedWorkspace+"/") {
				continue
			}
			index := strings.Index(normalized, normalizedWorkspace+"/")
			if index < 0 {
				continue
			}
			rel := strings.TrimPrefix(normalized[index+len(normalizedWorkspace)+1:], "./")
			rel = strings.TrimLeft(rel, "/")
			rel = strings.TrimRight(rel, ":,.;")
			if !strings.HasPrefix(rel, "lib/") && !strings.HasPrefix(rel, "test/") {
				continue
			}
			if !strings.HasSuffix(rel, ".dart") {
				continue
			}
			if _, ok := seen[rel]; ok {
				continue
			}
			seen[rel] = struct{}{}
			paths = append(paths, rel)
		}
	}
	if len(paths) == 0 {
		return nil
	}
	sort.Strings(paths)
	return paths
}

func expandValidationFailurePathsWithLocalImports(workspacePath string, failurePaths []string) []string {
	if strings.TrimSpace(workspacePath) == "" || len(failurePaths) == 0 {
		return failurePaths
	}
	packageName := workspaceDartPackageName(workspacePath)
	seen := map[string]struct{}{}
	expanded := make([]string, 0, len(failurePaths))
	appendPath := func(path string) {
		normalized := filepath.ToSlash(strings.TrimSpace(path))
		if normalized == "" {
			return
		}
		if strings.HasPrefix(normalized, "../") || strings.HasPrefix(normalized, "/") {
			return
		}
		if _, ok := seen[normalized]; ok {
			return
		}
		seen[normalized] = struct{}{}
		expanded = append(expanded, normalized)
	}
	for _, path := range failurePaths {
		normalized := filepath.ToSlash(strings.TrimSpace(path))
		if normalized == "" {
			continue
		}
		absPath := filepath.Join(workspacePath, filepath.FromSlash(normalized))
		if _, err := os.Stat(absPath); err != nil {
			continue
		}
		appendPath(normalized)
	}
	for _, path := range expanded {
		absPath := filepath.Join(workspacePath, filepath.FromSlash(path))
		content, err := os.ReadFile(absPath)
		if err != nil || len(content) == 0 {
			continue
		}
		baseDir := filepath.Dir(filepath.ToSlash(strings.TrimSpace(path)))
		matches := dartLocalImportPattern.FindAllStringSubmatch(string(content), -1)
		for _, match := range matches {
			if len(match) < 2 {
				continue
			}
			ref := strings.TrimSpace(match[1])
			if ref == "" {
				continue
			}
			var resolved string
			switch {
			case strings.HasPrefix(ref, "."):
				resolved = filepath.ToSlash(filepath.Clean(filepath.Join(baseDir, ref)))
			case packageName != "" && strings.HasPrefix(ref, "package:"+packageName+"/"):
				resolved = filepath.ToSlash(filepath.Join("lib", strings.TrimPrefix(ref, "package:"+packageName+"/")))
			default:
				continue
			}
			if !strings.HasPrefix(resolved, "lib/") && !strings.HasPrefix(resolved, "test/") {
				continue
			}
			appendPath(resolved)
		}
	}
	return expanded
}

func workspaceDartPackageName(workspacePath string) string {
	if strings.TrimSpace(workspacePath) == "" {
		return ""
	}
	content, err := os.ReadFile(filepath.Join(workspacePath, "pubspec.yaml"))
	if err != nil || len(content) == 0 {
		return ""
	}
	match := pubspecPackageNamePattern.FindStringSubmatch(string(content))
	if len(match) < 2 {
		return ""
	}
	return strings.TrimSpace(match[1])
}

func prioritizeValidationFailurePaths(failurePaths, targetPaths []string) []string {
	seen := map[string]struct{}{}
	prioritized := make([]string, 0, len(failurePaths)+len(targetPaths))
	appendPath := func(path string) {
		normalized := filepath.ToSlash(strings.TrimSpace(path))
		if normalized == "" {
			return
		}
		if _, ok := seen[normalized]; ok {
			return
		}
		seen[normalized] = struct{}{}
		prioritized = append(prioritized, normalized)
	}
	for _, path := range failurePaths {
		appendPath(path)
	}
	for _, path := range targetPaths {
		appendPath(path)
	}
	return prioritized
}

func buildValidationRepairTargetPaths(taskType appruns.BuilderRuntimeTaskType, directFailurePaths, expandedFailurePaths, targetPaths []string) []string {
	if len(directFailurePaths) > 0 {
		if taskType == appruns.BuilderRuntimeTaskTypeAnalyzeRepair || taskType == appruns.BuilderRuntimeTaskTypeTestRepair {
			return prioritizeValidationFailurePaths(directFailurePaths, nil)
		}
		return prioritizeValidationFailurePaths(directFailurePaths, expandedFailurePaths)
	}
	if len(expandedFailurePaths) > 0 {
		return prioritizeValidationFailurePaths(expandedFailurePaths, targetPaths)
	}
	return prioritizeValidationFailurePaths(nil, targetPaths)
}

func selectValidationRepairContextTasks(tasks []appruns.TaskBundleItem, failurePaths []string) []appruns.TaskBundleItem {
	if len(tasks) == 0 {
		return nil
	}
	failureSet := map[string]struct{}{}
	for _, path := range failurePaths {
		normalized := filepath.ToSlash(strings.TrimSpace(path))
		if normalized == "" {
			continue
		}
		failureSet[normalized] = struct{}{}
	}
	if len(failureSet) == 0 {
		return append([]appruns.TaskBundleItem(nil), tasks...)
	}
	matched := map[string]struct{}{}
	for _, task := range tasks {
		normalized := appruns.NormalizeTaskBundleItem(task)
		for _, targetPath := range normalized.TargetPaths {
			if _, ok := failureSet[filepath.ToSlash(strings.TrimSpace(targetPath))]; ok {
				matched[strings.TrimSpace(normalized.TaskID)] = struct{}{}
				break
			}
		}
	}
	if len(matched) == 0 {
		return append([]appruns.TaskBundleItem(nil), tasks...)
	}
	for changed := true; changed; {
		changed = false
		for _, task := range tasks {
			normalized := appruns.NormalizeTaskBundleItem(task)
			if _, ok := matched[strings.TrimSpace(normalized.TaskID)]; !ok {
				continue
			}
			for _, dependency := range normalized.Dependencies {
				dependency = strings.TrimSpace(dependency)
				if dependency == "" {
					continue
				}
				if _, ok := matched[dependency]; ok {
					continue
				}
				matched[dependency] = struct{}{}
				changed = true
			}
		}
	}
	selected := make([]appruns.TaskBundleItem, 0, len(matched))
	for _, task := range tasks {
		normalized := appruns.NormalizeTaskBundleItem(task)
		if _, ok := matched[strings.TrimSpace(normalized.TaskID)]; ok {
			selected = append(selected, normalized)
		}
	}
	if len(selected) == 0 {
		return append([]appruns.TaskBundleItem(nil), tasks...)
	}
	return selected
}

func (runner *Runner) runStep(step ExecutionStep, workspacePath, logPath string) error {
	resolvedLogPath := resolveRunFilePath(workspacePath, logPath)
	closeLog, err := attachLogOutput(step.Command, resolvedLogPath)
	if err != nil {
		return err
	}
	defer closeLog()
	if err := step.Command.Run(); err != nil {
		repaired, repairErr := repairAAPT2PermissionFailure(step, workspacePath, resolvedLogPath)
		if repairErr != nil {
			return repairErr
		}
		if !repaired {
			return err
		}
		closeLog()
		closeLog = func() {}
		return runner.rerunStep(step, workspacePath, logPath)
	}
	return nil
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

func shouldSkipAutomaticValidationRepair(logPath string, check CheckExecutionPreview) bool {
	if strings.TrimSpace(check.CheckID) != "check-flutter-build-apk" {
		return false
	}
	content, err := os.ReadFile(logPath)
	if err != nil || len(content) == 0 {
		return false
	}
	return isAAPT2EnvironmentFailureLog(content)
}

func isAAPT2EnvironmentFailureLog(content []byte) bool {
	if len(content) == 0 {
		return false
	}
	if aapt2PermissionDeniedPattern.Match(content) {
		return true
	}
	return aapt2DaemonStartupFailurePattern.Match(content) && aapt2ResourceLinkFailurePattern.Match(content)
}

func repairAAPT2PermissionFailure(step ExecutionStep, workspacePath, logPath string) (bool, error) {
	if step.Check == nil || strings.TrimSpace(step.Check.CheckID) != "check-flutter-build-apk" {
		return false, nil
	}
	content, err := os.ReadFile(logPath)
	if err != nil || len(content) == 0 {
		return false, nil
	}
	if !isAAPT2EnvironmentFailureLog(content) {
		return false, nil
	}
	gradleUserHome := resolveBuilderGradleUserHome(appFactoryRootFromWorkspace(workspacePath))
	if strings.TrimSpace(gradleUserHome) == "" {
		return false, nil
	}
	repaired := false
	chmodErr := filepath.WalkDir(gradleUserHome, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || entry.Name() != "aapt2" || !strings.Contains(path, string(filepath.Separator)+"transformed"+string(filepath.Separator)) {
			return nil
		}
		info, statErr := entry.Info()
		if statErr != nil {
			return statErr
		}
		if info.Mode()&0o111 != 0 {
			return nil
		}
		if err := os.Chmod(path, info.Mode()|0o755); err != nil {
			return err
		}
		repaired = true
		return nil
	})
	if chmodErr != nil {
		return false, fmt.Errorf("repair gradle aapt2 permissions: %w", chmodErr)
	}
	return repaired, nil
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

func loadRuntimeAcceptancePlan(run runRecord) (runtimeAcceptancePlan, error) {
	contextFiles, err := loadRunContextFiles(run)
	if err != nil {
		return runtimeAcceptancePlan{}, err
	}
	prepareRoot := filepath.Join(filepath.Dir(run.WorkspacePath), "prepare")
	for _, supportingFile := range contextFiles.SupportingFiles {
		if filepath.Base(supportingFile) != "acceptance-plan.json" {
			continue
		}
		planPath := resolvePrepareContextFilePath(prepareRoot, supportingFile)
		data, readErr := os.ReadFile(planPath)
		if readErr != nil {
			return runtimeAcceptancePlan{}, fmt.Errorf("load acceptance plan for runtime semantic checks: %w", readErr)
		}
		var plan runtimeAcceptancePlan
		if err := json.Unmarshal(data, &plan); err != nil {
			return runtimeAcceptancePlan{}, fmt.Errorf("decode acceptance plan for runtime semantic checks: %w", err)
		}
		return plan, nil
	}
	return runtimeAcceptancePlan{}, nil
}

func resolvePrepareContextFilePath(prepareRoot, item string) string {
	trimmed := filepath.FromSlash(strings.TrimSpace(item))
	if filepath.IsAbs(trimmed) {
		return trimmed
	}
	return filepath.Join(prepareRoot, trimmed)
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
	targetPaths []string,
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
		CheckpointKey: heartbeatCheckpointKey(step, roundState),
		RoundState:    roundState,
		TargetPaths:   targetPaths,
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
	targetPaths []string,
) error {
	return backend.Heartbeat(ctx, runID, appruns.Heartbeat{
		Stage:         step.Stage,
		Iteration:     maxInt(roundInput.Attempt, 1),
		EventType:     "run_patch_generation_started",
		RoundID:       roundInput.RoundID,
		Attempt:       maxInt(roundInput.Attempt, 1),
		CheckpointKey: heartbeatCheckpointKey(step, roundState),
		RoundState:    roundState,
		TargetPaths:   targetPaths,
		Summary:       buildPatchGenerationStartedSummary(roundInput.RoundID, targetPaths),
		TotalTokens:   0,
	})
}

func (runner *Runner) reportPatchGenerationWaiting(
	ctx context.Context,
	backend RunnerBackend,
	runID string,
	step ExecutionStep,
	roundInput appruns.RoundInput,
	roundState *appruns.RoundState,
	targetPaths []string,
	modelAlias string,
	elapsed time.Duration,
) error {
	return backend.Heartbeat(ctx, runID, appruns.Heartbeat{
		Stage:         step.Stage,
		Iteration:     maxInt(roundInput.Attempt, 1),
		EventType:     "run_patch_generation_waiting",
		RoundID:       roundInput.RoundID,
		Attempt:       maxInt(roundInput.Attempt, 1),
		CheckpointKey: heartbeatCheckpointKey(step, roundState),
		RoundState:    roundState,
		TargetPaths:   targetPaths,
		Summary:       buildPatchGenerationWaitingSummary(roundInput.RoundID, targetPaths, modelAlias, elapsed),
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
		CheckpointKey: heartbeatCheckpointKey(step, roundState),
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
		CheckpointKey: heartbeatCheckpointKey(step, roundState),
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
		CheckpointKey: heartbeatCheckpointKey(step, roundState),
		RoundState:    roundState,
		TargetPaths:   patchPaths,
		Summary:       buildGeneratedPatchSummary(roundInput.RoundID, patchPaths),
		TotalTokens:   0,
	})
}

func buildPatchGenerationStartedSummary(roundID string, targetPaths []string) string {
	roundID = strings.TrimSpace(roundID)
	pathSummary := buildPatchPathSummary(targetPaths)
	if roundID == "" {
		if pathSummary == "" {
			return "started patch generation"
		}
		return fmt.Sprintf("started patch generation for %s", pathSummary)
	}
	if pathSummary == "" {
		return fmt.Sprintf("round %s started patch generation", roundID)
	}
	return fmt.Sprintf("round %s started patch generation for %s", roundID, pathSummary)
}

func buildPatchGenerationWaitingSummary(roundID string, targetPaths []string, modelAlias string, elapsed time.Duration) string {
	roundID = strings.TrimSpace(roundID)
	pathSummary := buildPatchPathSummary(targetPaths)
	modelAlias = strings.TrimSpace(modelAlias)
	base := "still waiting for patch generation"
	if roundID != "" {
		base = fmt.Sprintf("round %s still waiting for patch generation", roundID)
	}
	if pathSummary != "" {
		base = fmt.Sprintf("%s for %s", base, pathSummary)
	}
	if modelAlias != "" {
		base = fmt.Sprintf("%s via %s", base, modelAlias)
	}
	if elapsed <= 0 {
		return base
	}
	if elapsed < time.Second {
		elapsed = elapsed.Round(10 * time.Millisecond)
	} else {
		elapsed = elapsed.Round(time.Second)
	}
	return fmt.Sprintf("%s after %s", base, elapsed)
}

func buildPatchGenerationFailedSummary(roundID string, targetPaths []string, reason error) string {
	roundID = strings.TrimSpace(roundID)
	pathSummary := buildPatchPathSummary(targetPaths)
	base := "patch generation failed"
	if roundID != "" {
		base = fmt.Sprintf("round %s patch generation failed", roundID)
	}
	if pathSummary != "" {
		base = fmt.Sprintf("%s for %s", base, pathSummary)
	}
	reasonText := strings.TrimSpace(errorText(reason))
	if reasonText == "" {
		return base
	}
	return fmt.Sprintf("%s: %s", base, reasonText)
}

func buildPatchApplyFailedSummary(roundID string, targetPaths []string, reason error) string {
	roundID = strings.TrimSpace(roundID)
	base := "patch apply failed"
	if roundID != "" {
		base = fmt.Sprintf("round %s patch apply failed", roundID)
	}
	if pathSummary := buildPatchPathSummary(targetPaths); pathSummary != "" {
		base = fmt.Sprintf("%s for %s", base, pathSummary)
	}
	reasonText := strings.TrimSpace(errorText(reason))
	if reasonText == "" {
		return base
	}
	return fmt.Sprintf("%s: %s", base, reasonText)
}

func buildAppliedPatchSummary(roundID string, affectedPaths []string) string {
	roundID = strings.TrimSpace(roundID)
	pathSummary := buildPatchPathSummary(affectedPaths)
	if roundID == "" {
		if pathSummary == "" {
			return "applied patch"
		}
		return fmt.Sprintf("applied patch to %s", pathSummary)
	}
	if pathSummary == "" {
		return fmt.Sprintf("round %s applied patch", roundID)
	}
	return fmt.Sprintf("round %s applied patch to %s", roundID, pathSummary)
}

func buildGeneratedPatchSummary(roundID string, patchPaths []string) string {
	roundID = strings.TrimSpace(roundID)
	pathSummary := buildPatchPathSummary(patchPaths)
	if roundID == "" {
		if pathSummary == "" {
			return "generated patch"
		}
		return fmt.Sprintf("generated patch for %s", pathSummary)
	}
	if pathSummary == "" {
		return fmt.Sprintf("round %s generated patch", roundID)
	}
	return fmt.Sprintf("round %s generated patch for %s", roundID, pathSummary)
}

func buildPatchPathSummary(paths []string) string {
	normalized := normalizePatchSummaryPaths(paths)
	if len(normalized) == 0 {
		return ""
	}
	if len(normalized) == 1 {
		return normalized[0]
	}
	const previewLimit = 3
	preview := normalized
	if len(preview) > previewLimit {
		preview = preview[:previewLimit]
		return fmt.Sprintf("%d files: %s, +%d more", len(normalized), strings.Join(preview, ", "), len(normalized)-len(preview))
	}
	return fmt.Sprintf("%d files: %s", len(normalized), strings.Join(preview, ", "))
}

func normalizePatchSummaryPaths(paths []string) []string {
	normalized := make([]string, 0, len(paths))
	seen := make(map[string]struct{}, len(paths))
	for _, path := range paths {
		trimmed := filepath.ToSlash(strings.TrimSpace(path))
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		normalized = append(normalized, trimmed)
	}
	return normalized
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
	RunID                  string                         `json:"run_id"`
	JobID                  string                         `json:"job_id"`
	WorkerID               string                         `json:"worker_id"`
	ExecutorImage          string                         `json:"executor_image,omitempty"`
	GoalSummary            string                         `json:"goal_summary,omitempty"`
	PlanningPolicy         appruns.PlanningPolicySnapshot `json:"planning_policy,omitempty"`
	HumanNotes             json.RawMessage                `json:"human_notes,omitempty"`
	TaskBundle             []appruns.TaskBundleItem       `json:"task_bundle,omitempty"`
	AcceptanceChecks       []appruns.AcceptanceCheck      `json:"acceptance_checks,omitempty"`
	AllowedPaths           []string                       `json:"allowed_paths,omitempty"`
	ProtectedPaths         []string                       `json:"protected_paths,omitempty"`
	KnowledgePack          []appruns.ProfileSkill         `json:"knowledge_pack,omitempty"`
	TemplateSourceDir      string                         `json:"template_source_dir,omitempty"`
	TemplateReferenceFiles map[string]string              `json:"template_reference_files,omitempty"`
	WorkspacePath          string                         `json:"workspace_path"`
	ArtifactDir            string                         `json:"artifact_dir"`
	LaunchCommand          string                         `json:"launch_command"`
	LaunchArgs             []string                       `json:"launch_args"`
	LogPath                string                         `json:"log_path"`
	ArtifactManifestPath   string                         `json:"artifact_manifest_path,omitempty"`
	MetricsPath            string                         `json:"metrics_path,omitempty"`
	IterationCount         int                            `json:"iteration_count,omitempty"`
	TotalTokens            int                            `json:"total_tokens,omitempty"`
	RoundState             *appruns.RoundState            `json:"round_state,omitempty"`
	BuilderRuntime         *appruns.BuilderRuntimePlan    `json:"builder_runtime,omitempty"`
	SemanticChecks         []runtimeAcceptancePlanItem    `json:"-"`
}

type runtimeAcceptancePlan struct {
	SemanticChecks []runtimeAcceptancePlanItem `json:"semantic_checks,omitempty"`
}

type runtimeAcceptancePlanItem struct {
	CheckID         string                 `json:"check_id"`
	Label           string                 `json:"label"`
	Description     string                 `json:"description,omitempty"`
	Stage           appruns.ExecutionStage `json:"stage,omitempty"`
	Commands        []string               `json:"commands,omitempty"`
	Required        bool                   `json:"required,omitempty"`
	SourceType      string                 `json:"source_type,omitempty"`
	SourceRef       string                 `json:"source_ref,omitempty"`
	BindingRefs     []string               `json:"slot_refs,omitempty"`
	FieldRefs       []string               `json:"field_refs,omitempty"`
	EvidencePattern string                 `json:"evidence_pattern,omitempty"`
}

type RunRecord = runRecord

func newRunRecord(run appruns.RunRecord) runRecord {
	return runRecord{
		RunID:                  run.RunID,
		JobID:                  run.JobID,
		WorkerID:               run.WorkerID,
		ExecutorImage:          run.ExecutorImage,
		GoalSummary:            run.GoalSummary,
		PlanningPolicy:         appruns.NormalizePlanningPolicySnapshot(run.PlanningPolicy),
		HumanNotes:             append(json.RawMessage(nil), run.HumanNotes...),
		TaskBundle:             append([]appruns.TaskBundleItem(nil), run.TaskBundle...),
		AcceptanceChecks:       append([]appruns.AcceptanceCheck(nil), run.AcceptanceChecks...),
		AllowedPaths:           append([]string(nil), run.AllowedPaths...),
		ProtectedPaths:         append([]string(nil), run.ProtectedPaths...),
		KnowledgePack:          append([]appruns.ProfileSkill(nil), run.KnowledgePack...),
		TemplateSourceDir:      run.TemplateSourceDir,
		TemplateReferenceFiles: maps.Clone(run.TemplateReferenceFiles),
		WorkspacePath:          run.WorkspacePath,
		ArtifactDir:            run.ArtifactDir,
		LaunchCommand:          run.LaunchCommand,
		LaunchArgs:             append([]string(nil), run.LaunchArgs...),
		LogPath:                run.LogPath,
		ArtifactManifestPath:   run.ArtifactManifestPath,
		MetricsPath:            run.MetricsPath,
		IterationCount:         run.IterationCount,
		TotalTokens:            run.TotalTokens,
		RoundState:             cloneRoundStateRef(run.RoundState),
	}
}

func cloneRoundStateRef(state *appruns.RoundState) *appruns.RoundState {
	if state == nil {
		return nil
	}
	cloned := *state
	cloned.PhaseTrace = append([]appruns.RoundPhase(nil), state.PhaseTrace...)
	if len(state.TaskStatuses) > 0 {
		cloned.TaskStatuses = maps.Clone(state.TaskStatuses)
	}
	return &cloned
}

func cloneRoundInputRef(input *appruns.RoundInput) *appruns.RoundInput {
	if input == nil {
		return nil
	}
	cloned := *input
	cloned.TaskBundle = append([]appruns.TaskBundleItem(nil), input.TaskBundle...)
	cloned.AllowedPaths = append([]string(nil), input.AllowedPaths...)
	cloned.KnowledgePack = append([]appruns.ProfileSkill(nil), input.KnowledgePack...)
	cloned.AcceptanceChecks = append([]appruns.AcceptanceCheck(nil), input.AcceptanceChecks...)
	if input.BuilderRuntime != nil {
		builderRuntime := *input.BuilderRuntime
		cloned.BuilderRuntime = &builderRuntime
	}
	return &cloned
}

func cloneRoundInputValue(input appruns.RoundInput) *appruns.RoundInput {
	return cloneRoundInputRef(&input)
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
		expandBuilderRuntimePlanModelFallbacks(run.BuilderRuntime, runner.ModelCatalog)
	}
	return run
}

func (runner *Runner) enrichRunRecordWithSemanticChecks(run runRecord) (runRecord, error) {
	plan, err := loadRuntimeAcceptancePlan(run)
	if err != nil {
		return run, err
	}
	if len(plan.SemanticChecks) == 0 {
		return run, nil
	}
	run.SemanticChecks = append([]runtimeAcceptancePlanItem(nil), plan.SemanticChecks...)
	existing := make(map[string]struct{}, len(run.AcceptanceChecks))
	for _, check := range run.AcceptanceChecks {
		existing[strings.TrimSpace(check.CheckID)] = struct{}{}
	}
	for _, check := range plan.SemanticChecks {
		checkID := strings.TrimSpace(check.CheckID)
		if checkID == "" || len(check.Commands) == 0 {
			continue
		}
		if _, exists := existing[checkID]; exists {
			continue
		}
		run.AcceptanceChecks = append(run.AcceptanceChecks, appruns.AcceptanceCheck{
			CheckID:         checkID,
			Label:           firstNonEmpty(check.Label, checkID),
			Stage:           firstNonEmptyStage(check.Stage, appruns.StageCheap),
			Required:        check.Required,
			Commands:        append([]string(nil), check.Commands...),
			SuccessCriteria: strings.TrimSpace(check.Description),
			TimeoutSeconds:  30,
		})
		existing[checkID] = struct{}{}
	}
	return run, nil
}

func BuildRoundInputForTest(run RunRecord) appruns.RoundInput {
	return buildRoundInput(run)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func firstNonEmptyStage(values ...appruns.ExecutionStage) appruns.ExecutionStage {
	for _, value := range values {
		if strings.TrimSpace(string(value)) != "" {
			return value
		}
	}
	return appruns.StageCheap
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
	return cfg.Enabled || cfg.DefaultModel != nil || cfg.UpgradeModel != nil || len(cfg.TaskRoutes) > 0 || cfg.UpgradeThreshold.MaxAttemptsBeforeUpgrade > 0 || cfg.UpgradeThreshold.MaxFilesBeforeUpgrade > 0 || cfg.UpgradeThreshold.MaxSchemaDriftBeforeUpgrade > 0 || cfg.UpgradeThreshold.MaxUnrelatedOperationRate > 0 || cfg.UpgradeThreshold.UpgradeOnValidationFail || cfg.UpgradeThreshold.UpgradeOnPatchParseFail || cfg.UpgradeThreshold.UpgradeOnScopeViolation || cfg.UpgradeThreshold.UpgradeOnSemanticConflict
}

func resolveBuilderRuntimePlan(cfg appconfig.BuilderRuntimeConfig, tasks []appruns.TaskBundleItem) appruns.BuilderRuntimePlan {
	plan := appruns.BuilderRuntimePlan{
		Enabled:      cfg.Enabled,
		DefaultModel: convertAgentModelConfig(cfg.DefaultModel),
		UpgradeModel: convertAgentModelConfig(cfg.UpgradeModel),
		UpgradeThreshold: appruns.BuilderRuntimeUpgradeThreshold{
			MaxAttemptsBeforeUpgrade:    cfg.UpgradeThreshold.MaxAttemptsBeforeUpgrade,
			MaxFilesBeforeUpgrade:       cfg.UpgradeThreshold.MaxFilesBeforeUpgrade,
			MaxSchemaDriftBeforeUpgrade: cfg.UpgradeThreshold.MaxSchemaDriftBeforeUpgrade,
			MaxUnrelatedOperationRate:   cfg.UpgradeThreshold.MaxUnrelatedOperationRate,
			UpgradeOnValidationFail:     cfg.UpgradeThreshold.UpgradeOnValidationFail,
			UpgradeOnPatchParseFail:     cfg.UpgradeThreshold.UpgradeOnPatchParseFail,
			UpgradeOnScopeViolation:     cfg.UpgradeThreshold.UpgradeOnScopeViolation,
			UpgradeOnSemanticConflict:   cfg.UpgradeThreshold.UpgradeOnSemanticConflict,
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
		switch normalizedHint := appruns.NormalizeTaskRouteHint(string(task.RouteHint)); normalizedHint {
		case appruns.TaskRouteHintDeterministic:
			decision.RouteSource = "route_hint"
			plan.TaskRoutes = append(plan.TaskRoutes, decision)
			continue
		case appruns.TaskRouteHintUpgradeModel, appruns.TaskRouteHintStrongModel:
			if plan.UpgradeModel.Primary != "" || len(plan.UpgradeModel.Fallbacks) > 0 {
				decision.RouteSource = "route_hint"
				decision.Model = plan.UpgradeModel
				plan.TaskRoutes = append(plan.TaskRoutes, decision)
				continue
			}
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

func mergeBuilderRuntimePlanFallbacks(primary, fallback *appruns.BuilderRuntimePlan) *appruns.BuilderRuntimePlan {
	if primary == nil {
		return fallback
	}
	merged := *primary
	if fallback == nil {
		return &merged
	}
	if merged.DefaultModel.Primary == "" && len(merged.DefaultModel.Fallbacks) == 0 {
		merged.DefaultModel = fallback.DefaultModel
	}
	if merged.UpgradeModel.Primary == "" && len(merged.UpgradeModel.Fallbacks) == 0 {
		merged.UpgradeModel = fallback.UpgradeModel
	}
	if merged.UpgradeThreshold.MaxAttemptsBeforeUpgrade == 0 {
		merged.UpgradeThreshold.MaxAttemptsBeforeUpgrade = fallback.UpgradeThreshold.MaxAttemptsBeforeUpgrade
	}
	if merged.UpgradeThreshold.MaxFilesBeforeUpgrade == 0 {
		merged.UpgradeThreshold.MaxFilesBeforeUpgrade = fallback.UpgradeThreshold.MaxFilesBeforeUpgrade
	}
	if merged.UpgradeThreshold.MaxSchemaDriftBeforeUpgrade == 0 {
		merged.UpgradeThreshold.MaxSchemaDriftBeforeUpgrade = fallback.UpgradeThreshold.MaxSchemaDriftBeforeUpgrade
	}
	if merged.UpgradeThreshold.MaxUnrelatedOperationRate == 0 {
		merged.UpgradeThreshold.MaxUnrelatedOperationRate = fallback.UpgradeThreshold.MaxUnrelatedOperationRate
	}
	merged.UpgradeThreshold.UpgradeOnValidationFail = merged.UpgradeThreshold.UpgradeOnValidationFail || fallback.UpgradeThreshold.UpgradeOnValidationFail
	merged.UpgradeThreshold.UpgradeOnPatchParseFail = merged.UpgradeThreshold.UpgradeOnPatchParseFail || fallback.UpgradeThreshold.UpgradeOnPatchParseFail
	merged.UpgradeThreshold.UpgradeOnScopeViolation = merged.UpgradeThreshold.UpgradeOnScopeViolation || fallback.UpgradeThreshold.UpgradeOnScopeViolation
	merged.UpgradeThreshold.UpgradeOnSemanticConflict = merged.UpgradeThreshold.UpgradeOnSemanticConflict || fallback.UpgradeThreshold.UpgradeOnSemanticConflict
	if len(merged.TaskRoutes) == 0 && len(fallback.TaskRoutes) > 0 {
		merged.TaskRoutes = append([]appruns.BuilderRuntimeTaskRoute(nil), fallback.TaskRoutes...)
	}
	merged.Enabled = merged.Enabled || fallback.Enabled
	return &merged
}

func expandBuilderRuntimePlanModelFallbacks(plan *appruns.BuilderRuntimePlan, catalog *appconfig.Config) {
	if plan == nil || catalog == nil {
		return
	}
	plan.DefaultModel = expandBuilderRuntimeModelRefFallbacks(plan.DefaultModel, catalog)
	plan.UpgradeModel = expandBuilderRuntimeModelRefFallbacks(plan.UpgradeModel, catalog)
	for index := range plan.TaskRoutes {
		plan.TaskRoutes[index].Model = expandBuilderRuntimeModelRefFallbacks(plan.TaskRoutes[index].Model, catalog)
	}
}

func expandBuilderRuntimeModelRefFallbacks(ref appruns.BuilderRuntimeModelRef, catalog *appconfig.Config) appruns.BuilderRuntimeModelRef {
	aliases := expandBuilderRuntimeModelAliases(ref, catalog)
	if len(aliases) == 0 {
		return appruns.BuilderRuntimeModelRef{}
	}
	return appruns.BuilderRuntimeModelRef{
		Primary:   aliases[0],
		Fallbacks: append([]string(nil), aliases[1:]...),
	}
}

func expandBuilderRuntimeModelAliases(ref appruns.BuilderRuntimeModelRef, catalog *appconfig.Config) []string {
	seen := map[string]struct{}{}
	aliases := make([]string, 0, 1+len(ref.Fallbacks))
	var appendAlias func(string)
	appendAlias = func(value string) {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			return
		}
		if _, ok := seen[trimmed]; ok {
			return
		}
		seen[trimmed] = struct{}{}
		aliases = append(aliases, trimmed)
		modelCfg, err := catalog.GetModelConfig(trimmed)
		if err != nil || modelCfg == nil {
			return
		}
		for _, fallback := range modelCfg.Fallbacks {
			appendAlias(fallback)
		}
	}
	for _, alias := range modelAliasesFromRef(ref) {
		appendAlias(alias)
	}
	return aliases
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
	RoundState         *appruns.RoundState
}

func diagnoseExecutionStepFailure(step ExecutionStep, execErr error, workspacePath, logPath string, workspacePatchFailed bool) executionFailureDiagnosis {
	if runtimeErr, ok := builderRuntimeDiagnosis(execErr); ok {
		nextAction := appruns.ControlActionStop
		if runtimeErr.resumeAllowed {
			nextAction = appruns.ControlActionResume
		}
		return executionFailureDiagnosis{
			Summary:            runtimeErr.summary,
			RecoverySuggestion: runtimeErr.recoverySuggestion,
			Signature:          runtimeErr.signature,
			NextAction:         nextAction,
			Policy:             runtimeErr.policy,
			PreserveWorkspace:  runtimeErr.preserveWorkspace,
			ResumeAllowed:      runtimeErr.resumeAllowed,
			RoundState:         cloneRoundStateRef(runtimeErr.state),
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
	if taskErr, ok := taskDiagnosis(execErr); ok {
		return taskErr.diagnosis()
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
		PlanningPolicy:   appruns.NormalizePlanningPolicySnapshot(plannedRun.PlanningPolicy),
		TaskBundle:       taskBundle,
		AllowedPaths:     append([]string(nil), plannedRun.AllowedPaths...),
		KnowledgePack:    append([]appruns.ProfileSkill(nil), plannedRun.KnowledgePack...),
		AcceptanceChecks: append([]appruns.AcceptanceCheck(nil), plannedRun.AcceptanceChecks...),
		BuilderRuntime:   plannedRun.BuilderRuntime,
	}
}

func buildValidationResults(passedChecks, failedChecks []appruns.CheckResult, semanticChecks []runtimeAcceptancePlanItem, semanticReviewInputs []string) []appruns.ValidationResult {
	semanticCheckIDs := make(map[string]struct{}, len(semanticChecks))
	for _, check := range semanticChecks {
		checkID := strings.TrimSpace(check.CheckID)
		if checkID == "" {
			continue
		}
		semanticCheckIDs[checkID] = struct{}{}
	}
	results := make([]appruns.ValidationResult, 0, len(passedChecks)+len(failedChecks))
	for _, check := range passedChecks {
		evidencePaths := append([]string(nil), check.EvidencePaths...)
		if _, ok := semanticCheckIDs[strings.TrimSpace(check.CheckID)]; ok {
			evidencePaths = append(evidencePaths, semanticReviewInputs...)
			evidencePaths = trimStringSlice(evidencePaths)
		}
		results = append(results, appruns.ValidationResult{
			CheckID:       check.CheckID,
			Label:         check.Label,
			Stage:         check.Stage,
			Outcome:       check.Outcome,
			Summary:       check.Details,
			EvidencePaths: evidencePaths,
		})
	}
	for _, check := range failedChecks {
		evidencePaths := append([]string(nil), check.EvidencePaths...)
		if _, ok := semanticCheckIDs[strings.TrimSpace(check.CheckID)]; ok {
			evidencePaths = append(evidencePaths, semanticReviewInputs...)
			evidencePaths = trimStringSlice(evidencePaths)
		}
		results = append(results, appruns.ValidationResult{
			CheckID:       check.CheckID,
			Label:         check.Label,
			Stage:         check.Stage,
			Outcome:       check.Outcome,
			Summary:       check.Details,
			EvidencePaths: evidencePaths,
			Blocking:      true,
		})
	}
	return results
}

func buildSemanticReviewInputs(jobRoot, buildReportPath, smokeReportPath string, semanticChecks []runtimeAcceptancePlanItem) []string {
	if len(semanticChecks) == 0 {
		return nil
	}
	inputs := []string{relFromJobRoot(jobRoot, buildReportPath)}
	if strings.TrimSpace(smokeReportPath) != "" {
		inputs = append(inputs, relFromJobRoot(jobRoot, smokeReportPath))
	}
	for _, candidate := range []string{
		filepath.Join(jobRoot, "reports", "device-logcat.txt"),
		filepath.Join(jobRoot, "reports", "device-screenshot.png"),
	} {
		if _, err := os.Stat(candidate); err == nil {
			inputs = append(inputs, relFromJobRoot(jobRoot, candidate))
		}
	}
	return trimStringSlice(inputs)
}

func buildSemanticReviewHumanAction(semanticChecks []runtimeAcceptancePlanItem, semanticReviewInputs []string) *appruns.HumanAction {
	if len(semanticChecks) == 0 {
		return nil
	}
	return &appruns.HumanAction{
		ActionID:       "review-semantic-acceptance",
		Summary:        "复核语义验收与设备证据",
		Reason:         "语义规则需要结合 build report 与设备证据确认目标 app 已真正落地。",
		Owner:          "product",
		RequiredInputs: append([]string(nil), semanticReviewInputs...),
	}
}

func trimStringSlice(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if _, exists := seen[trimmed]; exists {
			continue
		}
		seen[trimmed] = struct{}{}
		result = append(result, trimmed)
	}
	if len(result) == 0 {
		return nil
	}
	return result
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

func mergeRoundStateMetadata(base, existing *appruns.RoundState, currentTaskID string) *appruns.RoundState {
	if base == nil {
		return cloneRoundStateRef(existing)
	}
	merged := cloneRoundStateRef(base)
	if merged == nil {
		return cloneRoundStateRef(existing)
	}
	if existing != nil && len(existing.TaskStatuses) > 0 {
		merged.TaskStatuses = maps.Clone(existing.TaskStatuses)
	}
	if trimmedTaskID := strings.TrimSpace(currentTaskID); trimmedTaskID != "" {
		merged.CurrentTaskID = trimmedTaskID
	} else if existing != nil && strings.TrimSpace(existing.CurrentTaskID) != "" {
		merged.CurrentTaskID = strings.TrimSpace(existing.CurrentTaskID)
	}
	return merged
}

func failureRoundStateTaskID(run runRecord, step ExecutionStep) string {
	if step.Check != nil || run.BuilderRuntime == nil || !run.BuilderRuntime.Enabled {
		return ""
	}
	route, _ := selectBuilderRuntimeRoute(run)
	return strings.TrimSpace(route.TaskID)
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
	appendSemanticValidationSection(&builder, run.SemanticChecks, passedChecks, failedChecks)
	if failureSummary != "" {
		builder.WriteString("\n## 失败摘要\n")
		builder.WriteString(failureSummary)
		builder.WriteString("\n")
	}
	return builder.String()
}

func appendSemanticValidationSection(builder *strings.Builder, semanticChecks []runtimeAcceptancePlanItem, passedChecks, failedChecks []appruns.CheckResult) {
	if len(semanticChecks) == 0 {
		return
	}
	passed := make(map[string]appruns.CheckResult, len(passedChecks))
	failed := make(map[string]appruns.CheckResult, len(failedChecks))
	for _, check := range passedChecks {
		passed[strings.TrimSpace(check.CheckID)] = check
	}
	for _, check := range failedChecks {
		failed[strings.TrimSpace(check.CheckID)] = check
	}
	builder.WriteString("\n## 语义验收\n")
	for _, check := range semanticChecks {
		checkID := strings.TrimSpace(check.CheckID)
		status := "未执行"
		details := strings.TrimSpace(check.Description)
		if result, ok := passed[checkID]; ok {
			status = "通过"
			if strings.TrimSpace(result.Details) != "" {
				details = strings.TrimSpace(result.Details)
			}
		} else if result, ok := failed[checkID]; ok {
			status = "失败"
			if strings.TrimSpace(result.Details) != "" {
				details = strings.TrimSpace(result.Details)
			}
		}
		builder.WriteString("- ")
		builder.WriteString(status)
		builder.WriteString("：")
		builder.WriteString(firstNonEmpty(check.Label, checkID))
		if len(check.FieldRefs) > 0 {
			builder.WriteString(" (fields: ")
			builder.WriteString(strings.Join(check.FieldRefs, ", "))
			builder.WriteString(")")
		}
		builder.WriteString("\n")
		if details != "" {
			builder.WriteString("  ")
			builder.WriteString(details)
			builder.WriteString("\n")
		}
	}
	builder.WriteString("\n")
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
	builderHome := resolveBuilderHome(root)
	builderTempDir := resolveBuilderTempDir(root)
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
	if err := ensureBuilderHome(builderHome); err != nil {
		return nil, fmt.Errorf("create builder home: %w", err)
	}
	if err := ensureBuilderTempDir(builderTempDir); err != nil {
		return nil, fmt.Errorf("create builder temp dir: %w", err)
	}
	args = append(args, "-e", "PUB_CACHE="+pubCacheDir)
	args = append(args, "-e", "GRADLE_USER_HOME="+gradleUserHome)
	args = append(args, "-e", "HOME="+builderHome)
	args = append(args, "-e", "TMPDIR="+builderTempDir)
	args = append(args, "-e", "XDG_CACHE_HOME="+resolveBuilderXDGCacheHome(root))
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
	paths = append(paths, filepath.Clean(resolveBuilderHome(root)))
	paths = append(paths, filepath.Clean(resolveBuilderTempDir(root)))
	paths = append(paths, filepath.Clean(resolveBuilderXDGCacheHome(root)))

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

func resolveBuilderHome(root string) string {
	return filepath.Join(root, ".runtime", "home")
}

func resolveBuilderTempDir(root string) string {
	return filepath.Join(root, ".runtime", "tmp")
}

func resolveBuilderXDGCacheHome(root string) string {
	return filepath.Join(resolveBuilderHome(root), ".cache")
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

func ensureBuilderHome(path string) error {
	for _, child := range []string{"", ".cache", filepath.Join(".kotlin", "daemon")} {
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

func ensureBuilderTempDir(path string) error {
	return os.MkdirAll(path, 0o755)
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}
