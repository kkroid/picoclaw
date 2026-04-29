package adapter

import (
	"context"
	"errors"
	"strings"
	"time"

	appruns "github.com/sipeed/picoclaw/pkg/appfactory/runs"
)

type taskKind string

const (
	taskKindBuilderRuntimeEdit  taskKind = "builder_runtime_edit"
	taskKindCapturedWorkspace   taskKind = "captured_workspace_edit"
	taskKindValidationCheck     taskKind = "validation_check"
)

type taskTimeoutSpec struct {
	Limit                     time.Duration
	ProgressHeartbeatInterval time.Duration
	Meaning                   string
}

type taskFailureSpec struct {
	Signature          string
	RecoverySuggestion string
	PreserveWorkspace  bool
	ResumeAllowed      bool
	RequiresHumanReview bool
	Policy             repairFailurePolicy
}

type taskMetadata struct {
	ID          string
	Stage       appruns.ExecutionStage
	Summary     string
	Kind        taskKind
	CheckID     string
	TargetPaths []string
	Timeout     taskTimeoutSpec
	Failure     taskFailureSpec
}

type taskEnvironment struct {
	Runner         *Runner
	Backend        RunnerBackend
	RunID          string
	Run            runRecord
	RoundInput     appruns.RoundInput
	FailureContext string
}

type taskResult struct {
	WorkspacePatch            *appruns.WorkspacePatch
	ApplyResult               appruns.WorkspacePatchApplyResult
	BuilderRuntime            *appruns.BuilderRuntimeExecutionStats
	WorkspacePatchApplyFailed bool
	AppliedRoundInput         *appruns.RoundInput
	AppliedRoundState         *appruns.RoundState
	UpdatedRunRoundState      *appruns.RoundState
	AppliedTargetPaths        []string
	CheckDetails              string
	CommandRuns               int
	Err                       error
}

type executionTask interface {
	Metadata() taskMetadata
	Execute(ctx context.Context, env taskEnvironment) taskResult
}

type taskBase struct {
	metadata taskMetadata
}

func newTaskBase(metadata taskMetadata) taskBase {
	return taskBase{metadata: cloneTaskMetadata(metadata)}
}

func (base taskBase) Metadata() taskMetadata {
	return cloneTaskMetadata(base.metadata)
}

type builderRuntimeEditTask struct {
	taskBase
	step ExecutionStep
}

func newBuilderRuntimeEditTask(run runRecord, step ExecutionStep, roundInput appruns.RoundInput) builderRuntimeEditTask {
	return builderRuntimeEditTask{
		taskBase: newTaskBase(buildTaskMetadata(run, step, roundInput, true)),
		step:     step,
	}
}

func (task builderRuntimeEditTask) Execute(ctx context.Context, env taskEnvironment) taskResult {
	metadata := task.Metadata()
	result, err := env.Runner.executeBuilderRuntimeEditWithFailureContext(ctx, env.Backend, env.RunID, task.step, env.Run, env.RoundInput, env.FailureContext)
	outcome := taskResult{Err: err}
	if result != nil {
		outcome.WorkspacePatch = result.Patch
		outcome.ApplyResult = result.ApplyResult
		outcome.BuilderRuntime = result.Stats
		if len(result.ApplyResult.ModifiedFiles) > 0 {
			outcome.AppliedRoundInput = cloneRoundInputRef(result.AppliedRoundInput)
			if outcome.AppliedRoundInput == nil {
				outcome.AppliedRoundInput = cloneRoundInputValue(env.RoundInput)
			}
			appliedState := buildTaskAppliedRoundState(env.Run, task.step, metadata)
			outcome.AppliedRoundState = appliedState
			outcome.UpdatedRunRoundState = cloneRoundStateRef(appliedState)
			outcome.AppliedTargetPaths = append([]string(nil), metadata.TargetPaths...)
		}
	}
	return outcome
}

type capturedWorkspaceEditTask struct {
	taskBase
	step ExecutionStep
}

func newCapturedWorkspaceEditTask(run runRecord, step ExecutionStep, roundInput appruns.RoundInput) capturedWorkspaceEditTask {
	return capturedWorkspaceEditTask{
		taskBase: newTaskBase(buildTaskMetadata(run, step, roundInput, false)),
		step:     step,
	}
}

func (task capturedWorkspaceEditTask) Execute(ctx context.Context, env taskEnvironment) taskResult {
	metadata := task.Metadata()
	result := env.Runner.executeCapturedWorkspaceEdit(ctx, env.Backend, env.RunID, task.step, env.Run, env.RoundInput, metadata)
	result.CommandRuns = 1
	if len(result.ApplyResult.ModifiedFiles) > 0 {
		result.AppliedRoundInput = cloneRoundInputValue(env.RoundInput)
		result.AppliedRoundState = buildTaskAppliedRoundState(env.Run, task.step, metadata)
		result.AppliedTargetPaths = append([]string(nil), metadata.TargetPaths...)
	}
	return result
}

type validationCheckTask struct {
	taskBase
	step ExecutionStep
}

func newValidationCheckTask(run runRecord, step ExecutionStep, roundInput appruns.RoundInput) validationCheckTask {
	return validationCheckTask{
		taskBase: newTaskBase(buildTaskMetadata(run, step, roundInput, false)),
		step:     step,
	}
}

func (task validationCheckTask) Execute(ctx context.Context, env taskEnvironment) taskResult {
	outcome := taskResult{
		CheckDetails: "command chain completed",
		CommandRuns:  1,
	}
	err := env.Runner.runStep(task.step, env.Run.WorkspacePath, env.Run.LogPath)
	if err == nil {
		return outcome
	}
	if task.step.Check != nil && task.step.Check.Required && !task.step.Check.AllowFailure {
		repairResult, repaired, repairErr := env.Runner.tryAutomaticValidationRepair(ctx, env.Backend, env.RunID, env.Run, env.RoundInput, task.step)
		if repairResult != nil {
			outcome.WorkspacePatch = repairResult.Patch
			outcome.ApplyResult = repairResult.ApplyResult
			outcome.BuilderRuntime = repairResult.Stats
			if len(repairResult.ApplyResult.ModifiedFiles) > 0 {
				outcome.AppliedRoundInput = cloneRoundInputRef(repairResult.AppliedRoundInput)
				if outcome.AppliedRoundInput == nil {
					outcome.AppliedRoundInput = cloneRoundInputValue(env.RoundInput)
				}
				repairState := buildValidationRepairAppliedRoundState(env.Run, task.step)
				outcome.AppliedRoundState = repairState
				outcome.UpdatedRunRoundState = cloneRoundStateRef(repairState)
				outcome.AppliedTargetPaths = append([]string(nil), collectHeartbeatTargetPaths(env.RoundInput)...)
			}
		}
		if repaired {
			if repairErr == nil {
				outcome.CommandRuns++
				err = env.Runner.rerunStep(task.step, env.Run.WorkspacePath, env.Run.LogPath)
				if err == nil {
					if repairResult != nil && repairResult.Stats != nil {
						outcome.CheckDetails = "command chain completed after builder-runtime repair"
					} else {
						outcome.CheckDetails = "command chain completed after validation repair"
					}
					return outcome
				}
			} else {
				err = repairErr
			}
		}
	}
	if errors.Is(err, context.DeadlineExceeded) {
		meta := task.Metadata()
		outcome.Err = newTaskError(meta, taskErrorKindTimeout, err)
		return outcome
	}
	outcome.Err = err
	return outcome
}

func (runner *Runner) buildExecutionTask(run runRecord, step ExecutionStep, roundInput appruns.RoundInput) executionTask {
	if step.Check != nil {
		return newValidationCheckTask(run, step, roundInput)
	}
	if runner.shouldUseBuilderRuntime(run) {
		return newBuilderRuntimeEditTask(run, step, roundInput)
	}
	return newCapturedWorkspaceEditTask(run, step, roundInput)
}

func buildTaskMetadata(run runRecord, step ExecutionStep, roundInput appruns.RoundInput, builderRuntime bool) taskMetadata {
	metadata := taskMetadata{
		ID:          strings.TrimSpace(step.StepID),
		Stage:       step.Stage,
		Summary:     strings.TrimSpace(step.Summary),
		Kind:        taskKindCapturedWorkspace,
		TargetPaths: append([]string(nil), collectHeartbeatTargetPaths(roundInput)...),
	}
	if step.Check != nil {
		metadata.Kind = taskKindValidationCheck
		metadata.CheckID = strings.TrimSpace(step.Check.CheckID)
		metadata.Failure = defaultTaskFailureSpec(step, metadata.Kind)
		return metadata
	}
	if builderRuntime {
		metadata.Kind = taskKindBuilderRuntimeEdit
		metadata.Timeout = taskTimeoutSpec{
			Limit:                     builderRuntimePatchRequestTimeout,
			ProgressHeartbeatInterval: effectiveBuilderRuntimePatchProgressHeartbeatInterval(),
			Meaning:                   "builder_runtime_patch_request",
		}
		if route, _ := selectBuilderRuntimeRoute(run); strings.TrimSpace(route.TaskID) != "" {
			metadata.ID = strings.TrimSpace(route.TaskID)
			if summary := strings.TrimSpace(builderRuntimeTaskSummary(run.TaskBundle, route.TaskID)); summary != "" {
				metadata.Summary = summary
			}
			if scopedTargetPaths := builderRuntimeRouteTargetPaths(run.TaskBundle, route.TaskID); len(scopedTargetPaths) > 0 {
				metadata.TargetPaths = append([]string(nil), scopedTargetPaths...)
			}
		}
	}
	metadata.Failure = defaultTaskFailureSpec(step, metadata.Kind)
	return metadata
}

func defaultTaskFailureSpec(step ExecutionStep, kind taskKind) taskFailureSpec {
	policy := repairFailurePolicy{
		PreserveWorkspace:   true,
		ResumeAllowed:       false,
		RequiresHumanReview: true,
		MaxRounds:           1,
		UsedRounds:          1,
		RemainingRounds:     0,
	}
	signature := failureSignatureForStep(step)
	recoverySuggestion := "inspect runner log and rerun after fixing workspace, command chain, or environment"
	if step.Check != nil {
		signature = failureSignatureForCheck(*step.Check)
		recoverySuggestion = "inspect validation output and rerun after fixing workspace or environment"
	}
	if kind == taskKindBuilderRuntimeEdit {
		recoverySuggestion = "inspect builder-runtime route, timeout, generated patch, and workspace state before rerun"
	}
	return taskFailureSpec{
		Signature:           signature,
		RecoverySuggestion:  recoverySuggestion,
		PreserveWorkspace:   policy.PreserveWorkspace,
		ResumeAllowed:       policy.ResumeAllowed,
		RequiresHumanReview: policy.RequiresHumanReview,
		Policy:              policy,
	}
}

func cloneTaskMetadata(metadata taskMetadata) taskMetadata {
	cloned := metadata
	cloned.TargetPaths = append([]string(nil), metadata.TargetPaths...)
	return cloned
}

func taskExecutionID(step ExecutionStep, metadata taskMetadata) string {
	if taskID := strings.TrimSpace(metadata.ID); taskID != "" {
		return taskID
	}
	return strings.TrimSpace(step.StepID)
}

func buildTaskHeartbeat(run runRecord, step ExecutionStep, roundInput appruns.RoundInput, metadata taskMetadata) (*appruns.RoundState, string, []string) {
	roundState := heartbeatRoundStateForStep(step)
	if metadata.Kind == taskKindBuilderRuntimeEdit {
		roundState = builderRuntimeRoundState(run.RoundState, step, taskExecutionID(step, metadata), "")
	}
	summary := strings.TrimSpace(metadata.Summary)
	if summary == "" {
		summary = step.Summary
	}
	targetPaths := append([]string(nil), metadata.TargetPaths...)
	if len(targetPaths) == 0 {
		targetPaths = append([]string(nil), collectHeartbeatTargetPaths(roundInput)...)
	}
	return roundState, summary, targetPaths
}

func buildTaskAppliedRoundState(run runRecord, step ExecutionStep, metadata taskMetadata) *appruns.RoundState {
	if metadata.Kind == taskKindBuilderRuntimeEdit {
		return builderRuntimeRoundState(run.RoundState, step, taskExecutionID(step, metadata), appruns.BuilderRuntimeTaskStatusValidated)
	}
	return heartbeatRoundStateForStep(step)
}

func buildValidationRepairAppliedRoundState(run runRecord, step ExecutionStep) *appruns.RoundState {
	repairState := buildRoundState(
		[]appruns.RoundPhase{appruns.RoundPhaseInspect, appruns.RoundPhaseEdit, appruns.RoundPhaseValidate, appruns.RoundPhaseRepair},
		appruns.RoundPhaseRepair,
		appruns.ControlActionNone,
		false,
		false,
	)
	if step.Check == nil {
		return repairState
	}
	if repairTaskType, ok := validationRepairTaskType(*step.Check); ok {
		repairTask := preferredValidationRepairTask(run.TaskBundle, repairTaskType)
		if strings.TrimSpace(repairTask.TaskID) != "" {
			return builderRuntimeRoundState(run.RoundState, step, repairTask.TaskID, appruns.BuilderRuntimeTaskStatusValidated)
		}
	}
	return repairState
}

func taskConsumesCommandRunBudget(metadata taskMetadata) bool {
	return metadata.Kind != taskKindBuilderRuntimeEdit
}

type taskErrorKind string

const (
	taskErrorKindCommandFailed taskErrorKind = "command_failed"
	taskErrorKindTimeout       taskErrorKind = "timeout"
	taskErrorKindExecution     taskErrorKind = "execution_failed"
)

type taskError struct {
	Kind               taskErrorKind
	Task               taskMetadata
	Summary            string
	RecoverySuggestion string
	Signature          string
	PreserveWorkspace  bool
	ResumeAllowed      bool
	Policy             repairFailurePolicy
	State              *appruns.RoundState
	Wrapped            error
}

func newTaskError(metadata taskMetadata, kind taskErrorKind, wrapped error) *taskError {
	summary := ""
	if wrapped != nil {
		summary = wrapped.Error()
	}
	if kind == taskErrorKindTimeout && metadata.Timeout.Limit > 0 {
		summary = strings.TrimSpace(metadata.Summary)
		if summary == "" {
			summary = strings.TrimSpace(metadata.ID)
		}
		if summary == "" {
			summary = "task"
		}
		summary = summary + " timed out after " + metadata.Timeout.Limit.String()
		if wrapped != nil {
			summary += ": " + wrapped.Error()
		}
	}
	return &taskError{
		Kind:               kind,
		Task:               cloneTaskMetadata(metadata),
		Summary:            summary,
		RecoverySuggestion: metadata.Failure.RecoverySuggestion,
		Signature:          metadata.Failure.Signature,
		PreserveWorkspace:  metadata.Failure.PreserveWorkspace,
		ResumeAllowed:      metadata.Failure.ResumeAllowed,
		Policy:             metadata.Failure.Policy,
		Wrapped:            wrapped,
	}
}

func (err *taskError) Error() string {
	if err == nil {
		return ""
	}
	if err.Wrapped != nil {
		return err.Wrapped.Error()
	}
	return err.Summary
}

func (err *taskError) Unwrap() error {
	if err == nil {
		return nil
	}
	return err.Wrapped
}

func (err *taskError) diagnosis() executionFailureDiagnosis {
	if err == nil {
		return executionFailureDiagnosis{}
	}
	nextAction := appruns.ControlActionStop
	if err.ResumeAllowed {
		nextAction = appruns.ControlActionResume
	}
	return executionFailureDiagnosis{
		Summary:            err.Summary,
		RecoverySuggestion: err.RecoverySuggestion,
		Signature:          err.Signature,
		NextAction:         nextAction,
		Policy:             err.Policy,
		PreserveWorkspace:  err.PreserveWorkspace,
		ResumeAllowed:      err.ResumeAllowed,
		RoundState:         cloneRoundStateRef(err.State),
	}
}

func taskDiagnosis(err error) (*taskError, bool) {
	var diagnosed *taskError
	if errors.As(err, &diagnosed) {
		return diagnosed, true
	}
	return nil, false
}

func (runner *Runner) executeCapturedWorkspaceEdit(
	ctx context.Context,
	backend RunnerBackend,
	runID string,
	step ExecutionStep,
	run runRecord,
	roundInput appruns.RoundInput,
	metadata taskMetadata,
) taskResult {
	executionStep := step
	if step.Command != nil {
		clonedCmd, cloneErr := cloneExecCommand(step.Command)
		if cloneErr != nil {
			return taskResult{Err: newTaskError(metadata, taskErrorKindExecution, cloneErr)}
		}
		executionStep.Command = clonedCmd
	}
	roundState := heartbeatRoundStateForStep(step)
	beforeSnapshot, snapshotErr := captureWorkspaceSnapshot(run.WorkspacePath)
	if snapshotErr != nil {
		return taskResult{Err: newTaskError(metadata, taskErrorKindExecution, snapshotErr)}
	}
	if err := runner.reportPatchGenerationStarted(ctx, backend, runID, step, roundInput, roundState, metadata.TargetPaths); err != nil {
		return taskResult{Err: err}
	}
	execErr := runner.runStep(executionStep, run.WorkspacePath, run.LogPath)
	afterSnapshot, afterErr := captureWorkspaceSnapshot(run.WorkspacePath)
	if afterErr != nil {
		return taskResult{Err: newTaskError(metadata, taskErrorKindExecution, afterErr)}
	}
	capturedPatch, patchErr := buildCapturedWorkspacePatch(roundInput.RoundID, beforeSnapshot, afterSnapshot, run.AllowedPaths, run.ProtectedPaths)
	if restoreErr := restoreWorkspaceSnapshot(run.WorkspacePath, beforeSnapshot, afterSnapshot); restoreErr != nil {
		return taskResult{Err: newTaskError(metadata, taskErrorKindExecution, restoreErr)}
	}
	if execErr == nil && patchErr != nil {
		if err := runner.reportPatchGenerationFailed(ctx, backend, runID, step, roundInput, roundState, metadata.TargetPaths, patchErr); err != nil {
			return taskResult{Err: err}
		}
		return taskResult{Err: newTaskError(metadata, taskErrorKindExecution, patchErr)}
	}
	if execErr != nil {
		if errors.Is(execErr, context.DeadlineExceeded) {
			return taskResult{Err: newTaskError(metadata, taskErrorKindTimeout, execErr)}
		}
		return taskResult{Err: execErr}
	}
	if err := runner.reportGeneratedPatch(ctx, backend, runID, step, roundInput, roundState, capturedPatch); err != nil {
		return taskResult{Err: err}
	}
	applyResult, applyErr := appruns.ApplyWorkspacePatch(run.WorkspacePath, run.AllowedPaths, run.ProtectedPaths, *capturedPatch)
	if applyErr != nil {
		if err := runner.reportPatchApplyFailed(ctx, backend, runID, step, roundInput, roundState, workspacePatchPaths(capturedPatch), applyErr); err != nil {
			return taskResult{Err: err}
		}
		return taskResult{WorkspacePatchApplyFailed: true, Err: newTaskError(metadata, taskErrorKindExecution, applyErr)}
	}
	if applyResult.Status != "" {
		capturedPatch.Status = applyResult.Status
	}
	return taskResult{WorkspacePatch: capturedPatch, ApplyResult: applyResult}
}