package adapter

import (
	"context"
	"errors"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	appruns "github.com/sipeed/picoclaw/pkg/appfactory/runs"
)

func TestBuildTaskMetadataForBuilderRuntimeStep(t *testing.T) {
	previousTimeout := builderRuntimePatchRequestTimeout
	previousHeartbeatInterval := builderRuntimePatchProgressHeartbeatInterval
	builderRuntimePatchRequestTimeout = 2 * time.Minute
	builderRuntimePatchProgressHeartbeatInterval = 15 * time.Second
	defer func() {
		builderRuntimePatchRequestTimeout = previousTimeout
		builderRuntimePatchProgressHeartbeatInterval = previousHeartbeatInterval
	}()

	run := runRecord{
		TaskBundle: []appruns.TaskBundleItem{{
			TaskID:      "task-create-record-model",
			Title:       "创建领域记录模型",
			TaskType:    appruns.BuilderRuntimeTaskTypeSingleFileEdit,
			TargetPaths: []string{"lib/models/record.dart"},
		}},
		BuilderRuntime: &appruns.BuilderRuntimePlan{
			Enabled: true,
			TaskRoutes: []appruns.BuilderRuntimeTaskRoute{{
				TaskID:      "task-create-record-model",
				TaskType:    appruns.BuilderRuntimeTaskTypeSingleFileEdit,
				RouteSource: "task_route",
				Model:       appruns.BuilderRuntimeModelRef{Primary: "gemma4-26b-local"},
			}},
		},
	}
	step := ExecutionStep{
		StepID:  "task-create-record-model",
		Stage:   appruns.StageThinPrepare,
		Summary: "fallback summary",
	}
	roundInput := appruns.RoundInput{
		RoundID:    "round-1",
		Attempt:    1,
		TaskBundle: run.TaskBundle,
	}

	metadata := buildTaskMetadata(run, step, roundInput, true)
	if metadata.Kind != taskKindBuilderRuntimeEdit {
		t.Fatalf("metadata.Kind = %q, want %q", metadata.Kind, taskKindBuilderRuntimeEdit)
	}
	if metadata.ID != "task-create-record-model" {
		t.Fatalf("metadata.ID = %q, want task-create-record-model", metadata.ID)
	}
	if metadata.Summary != "builder-runtime task task-create-record-model: 创建领域记录模型" {
		t.Fatalf("metadata.Summary = %q, want builder-runtime task summary", metadata.Summary)
	}
	if metadata.Timeout.Limit != 2*time.Minute {
		t.Fatalf("metadata.Timeout.Limit = %s, want 2m", metadata.Timeout.Limit)
	}
	if metadata.Timeout.ProgressHeartbeatInterval != 15*time.Second {
		t.Fatalf("metadata.Timeout.ProgressHeartbeatInterval = %s, want 15s", metadata.Timeout.ProgressHeartbeatInterval)
	}
	if metadata.Timeout.Meaning != "builder_runtime_patch_request" {
		t.Fatalf("metadata.Timeout.Meaning = %q, want builder_runtime_patch_request", metadata.Timeout.Meaning)
	}
	if len(metadata.TargetPaths) != 1 || metadata.TargetPaths[0] != "lib/models/record.dart" {
		t.Fatalf("metadata.TargetPaths = %#v, want [lib/models/record.dart]", metadata.TargetPaths)
	}
	if metadata.Failure.Signature != "runner_exit_nonzero" {
		t.Fatalf("metadata.Failure.Signature = %q, want runner_exit_nonzero", metadata.Failure.Signature)
	}
}

func TestTaskBaseMetadataClonesTargetPaths(t *testing.T) {
	base := newTaskBase(taskMetadata{
		ID:          "task-a",
		Summary:     "task-a",
		Kind:        taskKindCapturedWorkspace,
		TargetPaths: []string{"lib/main.dart"},
	})

	metadata := base.Metadata()
	metadata.TargetPaths[0] = "lib/other.dart"

	if got := base.Metadata().TargetPaths[0]; got != "lib/main.dart" {
		t.Fatalf("base.Metadata().TargetPaths[0] = %q, want lib/main.dart", got)
	}
}

func TestDiagnoseExecutionStepFailureUsesTaskErrorSemantics(t *testing.T) {
	step := ExecutionStep{StepID: "check-flutter-test", Stage: appruns.StageCheap}
	state := buildRoundState([]appruns.RoundPhase{appruns.RoundPhaseInspect, appruns.RoundPhaseValidate}, appruns.RoundPhaseValidate, appruns.ControlActionResume, true, true)
	err := &taskError{
		Kind:               taskErrorKindTimeout,
		Task:               taskMetadata{ID: "check-flutter-test", Stage: appruns.StageCheap, Kind: taskKindValidationCheck},
		Summary:            "check-flutter-test timed out after 2m",
		RecoverySuggestion: "inspect timeout budget before rerun",
		Signature:          "check_timeout",
		PreserveWorkspace:  true,
		ResumeAllowed:      true,
		Policy: repairFailurePolicy{
			PreserveWorkspace:   true,
			ResumeAllowed:       true,
			RequiresHumanReview: false,
			MaxRounds:           2,
			UsedRounds:          1,
			RemainingRounds:     1,
		},
		State:   state,
		Wrapped: errors.New("deadline exceeded"),
	}

	diagnosis := diagnoseExecutionStepFailure(step, err, t.TempDir(), "", false)
	if diagnosis.Summary != "check-flutter-test timed out after 2m" {
		t.Fatalf("diagnosis.Summary = %q, want timeout summary", diagnosis.Summary)
	}
	if diagnosis.RecoverySuggestion != "inspect timeout budget before rerun" {
		t.Fatalf("diagnosis.RecoverySuggestion = %q, want timeout guidance", diagnosis.RecoverySuggestion)
	}
	if diagnosis.Signature != "check_timeout" {
		t.Fatalf("diagnosis.Signature = %q, want check_timeout", diagnosis.Signature)
	}
	if diagnosis.NextAction != appruns.ControlActionResume {
		t.Fatalf("diagnosis.NextAction = %q, want resume", diagnosis.NextAction)
	}
	if !diagnosis.ResumeAllowed {
		t.Fatal("diagnosis.ResumeAllowed = false, want true")
	}
	if diagnosis.RoundState == state {
		t.Fatal("diagnosis.RoundState should be cloned, got same pointer")
	}
	if diagnosis.RoundState == nil || diagnosis.RoundState.CurrentPhase != appruns.RoundPhaseValidate {
		t.Fatalf("diagnosis.RoundState = %#v, want validate phase clone", diagnosis.RoundState)
	}
}

func TestDiagnoseExecutionStepFailurePrefersWorkspacePatchApplyFailureForTaskErrors(t *testing.T) {
	step := ExecutionStep{StepID: "thin-prepare", Stage: appruns.StageThinPrepare}
	err := newTaskError(taskMetadata{
		ID:    "thin-prepare",
		Stage: appruns.StageThinPrepare,
		Kind:  taskKindCapturedWorkspace,
		Failure: taskFailureSpec{
			Signature:         "runner_exit_nonzero",
			RecoverySuggestion: "inspect captured workspace edit before rerun",
		},
	}, taskErrorKindExecution, errors.New("android/app/build.gradle.kts is protected"))

	diagnosis := diagnoseExecutionStepFailure(step, err, t.TempDir(), "", true)
	if diagnosis.Signature != "workspace_patch_apply_failed" {
		t.Fatalf("diagnosis.Signature = %q, want workspace_patch_apply_failed", diagnosis.Signature)
	}
	if diagnosis.PreserveWorkspace {
		t.Fatal("diagnosis.PreserveWorkspace = true, want false")
	}
	if diagnosis.ResumeAllowed {
		t.Fatal("diagnosis.ResumeAllowed = true, want false")
	}
	if diagnosis.RecoverySuggestion != "inspect executor-generated changes and allowed/protected path constraints before rerun" {
		t.Fatalf("diagnosis.RecoverySuggestion = %q, want workspace patch guidance", diagnosis.RecoverySuggestion)
	}
}

func TestValidationCheckTaskExecutePreservesRawCheckFailure(t *testing.T) {
	workspacePath := t.TempDir()
	run := runRecord{WorkspacePath: workspacePath, LogPath: "run.log"}
	step := ExecutionStep{
		StepID:  "check-flutter-analyze",
		Stage:   appruns.StageCheap,
		Summary: "run flutter analyze",
		Command: exec.Command("/bin/sh", "-lc", "exit 1"),
		Check: &CheckExecutionPreview{
			CheckID:  "check-flutter-analyze",
			Label:    "flutter analyze",
			Stage:    appruns.StageCheap,
			Required: true,
			Commands: []string{"flutter analyze"},
		},
	}
	task := newValidationCheckTask(run, step, appruns.RoundInput{RoundID: "round-1", Attempt: 1})

	result := task.Execute(context.Background(), taskEnvironment{
		Runner:     NewRunnerWithBackend(nil),
		RunID:      "run-1",
		Run:        run,
		RoundInput: appruns.RoundInput{RoundID: "round-1", Attempt: 1},
	})
	if result.Err == nil {
		t.Fatal("result.Err = nil, want raw check failure")
	}
	var taskErr *taskError
	if errors.As(result.Err, &taskErr) {
		t.Fatalf("result.Err = %#v, want raw command error instead of taskError", taskErr)
	}
	if result.CommandRuns != 1 {
		t.Fatalf("result.CommandRuns = %d, want 1", result.CommandRuns)
	}
	if result.CheckDetails != "command chain completed" {
		t.Fatalf("result.CheckDetails = %q, want default success detail placeholder", result.CheckDetails)
	}
}

func TestExecuteCapturedWorkspaceEditClonesReusableCommand(t *testing.T) {
	workspacePath := t.TempDir()
	runner := NewRunnerWithBackend(nil)
	step := ExecutionStep{
		StepID:  "task-probe",
		Stage:   appruns.StageThinPrepare,
		Summary: "write probe file",
		Command: exec.Command("/bin/sh", "-lc", "mkdir -p lib && cat <<'EOF' > lib/probe.dart\nconst probe = 'ok';\nEOF"),
	}
	run := runRecord{
		WorkspacePath: workspacePath,
		AllowedPaths:  []string{"lib/**"},
		LogPath:       filepath.Join(t.TempDir(), "run.log"),
	}
	roundInput := appruns.RoundInput{RoundID: "round-1", Attempt: 1, AllowedPaths: []string{"lib/**"}}
	metadata := buildTaskMetadata(run, step, roundInput, false)

	backend := noopRunnerBackend{}
	first := runner.executeCapturedWorkspaceEdit(context.Background(), backend, "run-1", step, run, roundInput, metadata)
	if first.Err != nil {
		t.Fatalf("first executeCapturedWorkspaceEdit() error = %v", first.Err)
	}
	second := runner.executeCapturedWorkspaceEdit(context.Background(), backend, "run-1", step, run, roundInput, metadata)
	if second.Err != nil {
		t.Fatalf("second executeCapturedWorkspaceEdit() error = %v, want cloned command instead of exec reuse failure", second.Err)
	}
}