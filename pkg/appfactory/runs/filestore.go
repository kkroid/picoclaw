package runs

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/sipeed/picoclaw/pkg/fileutil"
)

const (
	snapshotBucketRetainCount = 5
)

type FileStore struct {
	root string
	mu   sync.Mutex
}

type runIndex struct {
	JobID string `json:"job_id"`
	RunID string `json:"run_id"`
}

type runEvent struct {
	At           time.Time      `json:"at"`
	Type         string         `json:"type"`
	RunID        string         `json:"run_id"`
	JobID        string         `json:"job_id"`
	Summary      string         `json:"summary,omitempty"`
	Stage        ExecutionStage `json:"stage,omitempty"`
	Status       Status         `json:"status,omitempty"`
	SnapshotPath string         `json:"snapshot_path,omitempty"`
}

func NewFileStore(root string) (*FileStore, error) {
	if err := os.MkdirAll(filepath.Join(root, "jobs"), 0o755); err != nil {
		return nil, fmt.Errorf("create runs root: %w", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "run-index"), 0o755); err != nil {
		return nil, fmt.Errorf("create runs index root: %w", err)
	}
	return &FileStore{root: root}, nil
}

func (store *FileStore) CreateRun(_ context.Context, builderID, workerID, leaseID string, input BuildInput) (RunRecord, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	if err := validateBuildInput(input); err != nil {
		return RunRecord{}, err
	}
	if builderID == "" || workerID == "" || leaseID == "" {
		return RunRecord{}, fmt.Errorf("create run: %w", ErrInvalidRequest)
	}
	now := time.Now().UTC()
	runID := uuid.NewString()
	jobRoot := filepath.Join(store.root, "jobs", input.JobID)
	workspacePath := filepath.Join(jobRoot, "workspace")
	artifactDir := filepath.Join(jobRoot, "artifacts")
	if err := ensureJobLayout(jobRoot); err != nil {
		return RunRecord{}, err
	}
	workspaceEvents, err := prepareWorkspace(store.root, input, runID, jobRoot, workspacePath)
	if err != nil {
		return RunRecord{}, err
	}
	input.WorkspacePath = workspacePath
	input.ArtifactDir = artifactDir
	input.ContextFiles = canonicalizeContextFiles(jobRoot, input.ContextFiles)
	runnerScriptPath := filepath.Join(jobRoot, "runs", runID, "runner.sh")
	logPath := filepath.Join(jobRoot, "logs", "builder-run-"+runID+".log")
	launchCommand := ""
	launchArgs := []string(nil)
	if runnerScript, err := buildRunnerScript(input, workspacePath, artifactDir, logPath); err != nil {
		return RunRecord{}, err
	} else if runnerScript != "" {
		if err := fileutil.WriteFileAtomic(runnerScriptPath, []byte(runnerScript), 0o700); err != nil {
			return RunRecord{}, err
		}
		launchCommand = "/bin/sh"
		launchArgs = []string{runnerScriptPath}
	}
	record := RunRecord{
		RunID:            runID,
		JobID:            input.JobID,
		BuilderID:        builderID,
		WorkerID:         workerID,
		LeaseID:          leaseID,
		Status:           StatusRunning,
		ExecutorImage:    input.ExecutorImage,
		GoalSummary:      input.GoalSummary,
		TaskBundle:       append([]TaskBundleItem(nil), input.TaskBundle...),
		AcceptanceChecks: append([]AcceptanceCheck(nil), input.AcceptanceChecks...),
		AllowedPaths:     append([]string(nil), input.AllowedPaths...),
		ProtectedPaths:   append([]string(nil), input.ProtectedPaths...),
		KnowledgePack:    append([]ProfileSkill(nil), input.KnowledgePack...),
		InputPath:        filepath.ToSlash(filepath.Join("jobs", input.JobID, "prepare", "builder-input.json")),
		WorkspacePath:    filepath.ToSlash(workspacePath),
		ArtifactDir:      filepath.ToSlash(artifactDir),
		RunnerScriptPath: relToRoot(store.root, runnerScriptPath),
		LaunchCommand:    launchCommand,
		LaunchArgs:       launchArgs,
		LogPath:          relToRoot(store.root, logPath),
		EventsPath:       filepath.ToSlash(filepath.Join("jobs", input.JobID, "runs", runID, "events.jsonl")),
		CreatedAt:        now,
		UpdatedAt:        now,
		StartedAt:        now,
	}
	if err := writeJSON(store.inputPath(input.JobID), input); err != nil {
		return RunRecord{}, err
	}
	if err := writeJSON(store.runPath(input.JobID, runID), record); err != nil {
		return RunRecord{}, err
	}
	if err := writeJSON(store.indexPath(runID), runIndex{JobID: input.JobID, RunID: runID}); err != nil {
		return RunRecord{}, err
	}
	if err := store.appendEvent(record, runEvent{At: now, Type: "run_created", RunID: runID, JobID: input.JobID, Status: StatusRunning}); err != nil {
		return RunRecord{}, err
	}
	for _, event := range workspaceEvents {
		if err := store.appendEvent(record, event); err != nil {
			return RunRecord{}, err
		}
	}
	return record, nil
}

func (store *FileStore) GetRun(_ context.Context, runID string) (RunRecord, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	return store.getRunLocked(runID)
}

func (store *FileStore) UpdateHeartbeat(_ context.Context, runID string, heartbeat Heartbeat) (RunRecord, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	record, err := store.getRunLocked(runID)
	if err != nil {
		return RunRecord{}, err
	}
	if isTerminal(record.Status) {
		return RunRecord{}, ErrTerminalRun
	}
	now := time.Now().UTC()
	record.LastStage = heartbeat.Stage
	record.IterationCount = heartbeat.Iteration
	record.TotalTokens = heartbeat.TotalTokens
	record.FailureSignatures = append([]string(nil), heartbeat.FailureSignatures...)
	record.UpdatedAt = now
	if err := writeJSON(store.runPath(record.JobID, record.RunID), record); err != nil {
		return RunRecord{}, err
	}
	if err := store.appendEvent(record, runEvent{At: now, Type: "run_heartbeat", RunID: record.RunID, JobID: record.JobID, Summary: heartbeat.Summary, Stage: heartbeat.Stage, Status: record.Status}); err != nil {
		return RunRecord{}, err
	}
	return record, nil
}

func (store *FileStore) CompleteRun(_ context.Context, runID string, output BuildOutput) (RunRecord, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	record, err := store.getRunLocked(runID)
	if err != nil {
		return RunRecord{}, err
	}
	if isTerminal(record.Status) {
		return RunRecord{}, ErrTerminalRun
	}
	if output.JobID == "" || output.FinalSummary == "" || output.SchemaVersion == "" {
		return RunRecord{}, fmt.Errorf("complete run: %w", ErrInvalidRequest)
	}
	now := time.Now().UTC()
	record.Status = StatusCompleted
	record.OutputPath = filepath.ToSlash(filepath.Join("jobs", record.JobID, "runs", record.RunID, "builder-output.json"))
	record.ArtifactManifestPath = output.Artifacts.ManifestPath
	record.MetricsPath = output.Metrics.MetricsPath
	record.FinishedAt = now
	record.UpdatedAt = now
	record.DistinctFailureCount = output.Metrics.DistinctFailureSignatures
	if err := writeJSON(store.outputPath(record.JobID, record.RunID), output); err != nil {
		return RunRecord{}, err
	}
	if err := store.appendEvent(record, runEvent{At: now, Type: "run_completed", RunID: record.RunID, JobID: record.JobID, Summary: output.FinalSummary, Status: StatusCompleted}); err != nil {
		return RunRecord{}, err
	}
	if err := writeJSON(store.runPath(record.JobID, record.RunID), record); err != nil {
		return RunRecord{}, err
	}
	return record, nil
}

func (store *FileStore) FailRun(_ context.Context, runID string, report FailureReport) (RunRecord, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	record, err := store.getRunLocked(runID)
	if err != nil {
		return RunRecord{}, err
	}
	if isTerminal(record.Status) {
		return RunRecord{}, ErrTerminalRun
	}
	now := time.Now().UTC()
	record.Status = StatusFailed
	record.FailureSummary = report.Summary
	record.RecoverySuggestion = report.RecoverySuggestion
	record.FailureSignatures = append([]string(nil), report.FailureSignatures...)
	record.RoundState = cloneRoundState(report.RoundState)
	record.RepairContext = cloneRepairContext(report.RepairContext)
	record.DistinctFailureCount = len(record.FailureSignatures)
	record.FinishedAt = now
	record.UpdatedAt = now
	if err := store.appendEvent(record, runEvent{At: now, Type: "run_failed", RunID: record.RunID, JobID: record.JobID, Summary: report.Summary, Status: StatusFailed}); err != nil {
		return RunRecord{}, err
	}
	if err := writeJSON(store.runPath(record.JobID, record.RunID), record); err != nil {
		return RunRecord{}, err
	}
	return record, nil
}

func cloneRoundState(state *RoundState) *RoundState {
	if state == nil {
		return nil
	}
	cloned := *state
	cloned.PhaseTrace = append([]RoundPhase(nil), state.PhaseTrace...)
	return &cloned
}

func cloneRepairContext(context *RepairContext) *RepairContext {
	if context == nil {
		return nil
	}
	cloned := *context
	cloned.State = cloneRoundState(context.State)
	if context.Budget != nil {
		budget := *context.Budget
		cloned.Budget = &budget
	}
	cloned.FailedChecks = append([]string(nil), context.FailedChecks...)
	cloned.FailureSignatures = append([]string(nil), context.FailureSignatures...)
	return &cloned
}

func (store *FileStore) CancelRun(_ context.Context, runID, reason string) (RunRecord, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	record, err := store.getRunLocked(runID)
	if err != nil {
		return RunRecord{}, err
	}
	if isTerminal(record.Status) {
		return RunRecord{}, ErrTerminalRun
	}
	now := time.Now().UTC()
	record.Status = StatusCancelled
	record.FailureSummary = reason
	record.FinishedAt = now
	record.UpdatedAt = now
	if err := store.appendEvent(record, runEvent{At: now, Type: "run_cancelled", RunID: record.RunID, JobID: record.JobID, Summary: reason, Status: StatusCancelled}); err != nil {
		return RunRecord{}, err
	}
	if err := writeJSON(store.runPath(record.JobID, record.RunID), record); err != nil {
		return RunRecord{}, err
	}
	return record, nil
}

func (store *FileStore) IndexArtifacts(_ context.Context, runID string, manifest ArtifactManifest) (RunRecord, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	record, err := store.getRunLocked(runID)
	if err != nil {
		return RunRecord{}, err
	}
	if manifest.JobID == "" || len(manifest.Items) == 0 || manifest.SchemaVersion == "" {
		return RunRecord{}, fmt.Errorf("index artifacts: %w", ErrInvalidRequest)
	}
	record.ArtifactManifestPath = filepath.ToSlash(filepath.Join("jobs", record.JobID, "runs", record.RunID, "artifact-manifest.json"))
	record.UpdatedAt = time.Now().UTC()
	if err := writeJSON(store.artifactPath(record.JobID, record.RunID), manifest); err != nil {
		return RunRecord{}, err
	}
	if err := writeJSON(store.runPath(record.JobID, record.RunID), record); err != nil {
		return RunRecord{}, err
	}
	return record, nil
}

func (store *FileStore) IndexMetrics(_ context.Context, runID string, metrics Metrics) (RunRecord, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	record, err := store.getRunLocked(runID)
	if err != nil {
		return RunRecord{}, err
	}
	if metrics.JobID == "" || metrics.SchemaVersion == "" {
		return RunRecord{}, fmt.Errorf("index metrics: %w", ErrInvalidRequest)
	}
	record.MetricsPath = filepath.ToSlash(filepath.Join("jobs", record.JobID, "runs", record.RunID, "metrics.json"))
	record.TotalTokens = metrics.TotalTokens
	record.IterationCount = metrics.TotalIterations
	record.DistinctFailureCount = len(metrics.FailureSignatures)
	record.UpdatedAt = time.Now().UTC()
	if err := writeJSON(store.metricsPath(record.JobID, record.RunID), metrics); err != nil {
		return RunRecord{}, err
	}
	if err := writeJSON(store.runPath(record.JobID, record.RunID), record); err != nil {
		return RunRecord{}, err
	}
	return record, nil
}

func (store *FileStore) Close() error { return nil }

func (store *FileStore) getRunLocked(runID string) (RunRecord, error) {
	var idx runIndex
	if err := readJSON(store.indexPath(runID), &idx); err != nil {
		if os.IsNotExist(err) {
			return RunRecord{}, ErrRunNotFound
		}
		return RunRecord{}, err
	}
	var record RunRecord
	if err := readJSON(store.runPath(idx.JobID, runID), &record); err != nil {
		if os.IsNotExist(err) {
			return RunRecord{}, ErrRunNotFound
		}
		return RunRecord{}, err
	}
	return record, nil
}

func (store *FileStore) inputPath(jobID string) string {
	return filepath.Join(store.root, "jobs", jobID, "prepare", "builder-input.json")
}

func (store *FileStore) runPath(jobID, runID string) string {
	return filepath.Join(store.root, "jobs", jobID, "runs", runID, "run.json")
}

func (store *FileStore) outputPath(jobID, runID string) string {
	return filepath.Join(store.root, "jobs", jobID, "runs", runID, "builder-output.json")
}

func (store *FileStore) artifactPath(jobID, runID string) string {
	return filepath.Join(store.root, "jobs", jobID, "runs", runID, "artifact-manifest.json")
}

func (store *FileStore) metricsPath(jobID, runID string) string {
	return filepath.Join(store.root, "jobs", jobID, "runs", runID, "metrics.json")
}

func (store *FileStore) eventsPath(jobID, runID string) string {
	return filepath.Join(store.root, "jobs", jobID, "runs", runID, "events.jsonl")
}

func (store *FileStore) indexPath(runID string) string {
	return filepath.Join(store.root, "run-index", runID+".json")
}

func (store *FileStore) appendEvent(record RunRecord, event runEvent) error {
	data, err := json.Marshal(event)
	if err != nil {
		return err
	}
	path := store.eventsPath(record.JobID, record.RunID)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()
	if _, err := file.Write(append(data, '\n')); err != nil {
		return err
	}
	return file.Sync()
}

func writeJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal json: %w", err)
	}
	return fileutil.WriteFileAtomic(path, data, 0o600)
}

func readJSON(path string, target any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, target)
}

func validateBuildInput(input BuildInput) error {
	if input.SchemaVersion == "" || input.JobID == "" || input.PRDID == "" || input.TemplateID == "" {
		return fmt.Errorf("build input ids: %w", ErrInvalidRequest)
	}
	if input.WorkspacePath == "" || input.ArtifactDir == "" || input.GoalSummary == "" {
		return fmt.Errorf("build input paths: %w", ErrInvalidRequest)
	}
	if len(input.TaskBundle) == 0 || len(input.AcceptanceChecks) == 0 || len(input.AllowedPaths) == 0 {
		return fmt.Errorf("build input lists: %w", ErrInvalidRequest)
	}
	if len(input.CommandProfile) == 0 || len(input.ContextFiles) == 0 || input.IterationBudget <= 0 || input.TokenBudget <= 0 {
		return fmt.Errorf("build input control: %w", ErrInvalidRequest)
	}
	var profile CommandProfile
	if err := json.Unmarshal(input.CommandProfile, &profile); err != nil {
		return fmt.Errorf("command profile: %w", ErrInvalidRequest)
	}
	if profile.ProfileName == "" || profile.NetworkPolicy == "" || len(profile.AllowedStages) == 0 {
		return fmt.Errorf("command profile required fields: %w", ErrInvalidRequest)
	}
	var contextFiles ContextFiles
	if err := json.Unmarshal(input.ContextFiles, &contextFiles); err != nil {
		return fmt.Errorf("context files: %w", ErrInvalidRequest)
	}
	if contextFiles.PRDMarkdownPath == "" || contextFiles.PRDJSONPath == "" || contextFiles.TemplateFitReportPath == "" || contextFiles.ImplementationPlanPath == "" {
		return fmt.Errorf("context files required fields: %w", ErrInvalidRequest)
	}
	return nil
}

func isTerminal(status Status) bool {
	return status == StatusCompleted || status == StatusFailed || status == StatusCancelled
}

func ensureJobLayout(jobRoot string) error {
	dirs := []string{
		filepath.Join(jobRoot, "prepare"),
		filepath.Join(jobRoot, "runs"),
		filepath.Join(jobRoot, "workspace"),
		filepath.Join(jobRoot, "reports"),
		filepath.Join(jobRoot, "logs"),
		filepath.Join(jobRoot, "artifacts", "apk"),
		filepath.Join(jobRoot, "artifacts", "screenshots"),
		filepath.Join(jobRoot, "artifacts", "diagnostics"),
		filepath.Join(jobRoot, "snapshots", "preserved"),
		filepath.Join(jobRoot, "snapshots", "archived"),
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create job layout: %w", err)
		}
	}
	return nil
}

func canonicalizeContextFiles(jobRoot string, raw json.RawMessage) json.RawMessage {
	var files ContextFiles
	if err := json.Unmarshal(raw, &files); err != nil {
		return raw
	}
	base := filepath.Join(jobRoot, "input", "context")
	base = filepath.Join(jobRoot, "prepare")
	files.PRDMarkdownPath = filepath.ToSlash(filepath.Join(base, filepath.Base(files.PRDMarkdownPath)))
	files.PRDJSONPath = filepath.ToSlash(filepath.Join(base, filepath.Base(files.PRDJSONPath)))
	files.TemplateFitReportPath = filepath.ToSlash(filepath.Join(base, filepath.Base(files.TemplateFitReportPath)))
	files.ImplementationPlanPath = filepath.ToSlash(filepath.Join(base, filepath.Base(files.ImplementationPlanPath)))
	if files.ManualConstraintsPath != "" {
		files.ManualConstraintsPath = filepath.ToSlash(filepath.Join(base, filepath.Base(files.ManualConstraintsPath)))
	}
	if len(files.SupportingFiles) > 0 {
		result := make([]string, 0, len(files.SupportingFiles))
		for _, item := range files.SupportingFiles {
			result = append(result, filepath.ToSlash(filepath.Join(base, filepath.Base(item))))
		}
		files.SupportingFiles = result
	}
	updated, err := json.Marshal(files)
	if err != nil {
		return raw
	}
	return updated
}

func buildRunnerScript(input BuildInput, workspacePath, artifactDir, logPath string) (string, error) {
	var profile CommandProfile
	if err := json.Unmarshal(input.CommandProfile, &profile); err != nil {
		return "", fmt.Errorf("decode command profile: %w", err)
	}
	var contextFiles ContextFiles
	if err := json.Unmarshal(input.ContextFiles, &contextFiles); err != nil {
		return "", fmt.Errorf("decode context files: %w", err)
	}
	for _, check := range input.AcceptanceChecks {
		for _, command := range check.Commands {
			trimmed := strings.TrimSpace(command)
			if trimmed == "" {
				continue
			}
			name := firstToken(trimmed)
			if len(profile.AllowedCommands) > 0 && !slices.Contains(profile.AllowedCommands, name) {
				return "", fmt.Errorf("command %q not allowed: %w", name, ErrInvalidRequest)
			}
			if slices.Contains(profile.DeniedCommands, name) {
				return "", fmt.Errorf("command %q denied: %w", name, ErrInvalidRequest)
			}
		}
	}
	var builder strings.Builder
	builder.WriteString("#!/bin/sh\n")
	builder.WriteString("set -eu\n")
	builder.WriteString("mkdir -p ")
	builder.WriteString(shellQuote(workspacePath))
	builder.WriteString(" ")
	builder.WriteString(shellQuote(artifactDir))
	builder.WriteString(" ")
	builder.WriteString(shellQuote(filepath.Dir(logPath)))
	builder.WriteString("\n")
	builder.WriteString("cd ")
	builder.WriteString(shellQuote(workspacePath))
	builder.WriteString("\n")
	builder.WriteString("exec >")
	builder.WriteString(shellQuote(logPath))
	builder.WriteString(" 2>&1\n")
	builder.WriteString(buildDefaultExecutorPhaseScript())
	builder.WriteString("mkdir -p .runtime/gradle-user-home\n")
	builder.WriteString("export GRADLE_USER_HOME=\"$PWD/.runtime/gradle-user-home\"\n")
	builder.WriteString("rm -rf android/.gradle\n")
	return builder.String(), nil
}

func buildDefaultExecutorPhaseScript() string {
	var builder strings.Builder
	builder.WriteString("runtime=\"${APPFACTORY_BUILDER_RUNTIME:-executor}\"\n")
	builder.WriteString("if [ \"$runtime\" != \"executor\" ]; then\n")
	builder.WriteString("  echo \"unsupported builder runtime: $runtime\" >&2\n")
	builder.WriteString("  exit 1\n")
	builder.WriteString("fi\n")
	builder.WriteString("echo '== skill executor phase =='\n")
	builder.WriteString("echo 'default skill + executor runtime is not implemented yet; replace runner generation before executing appfactory runs' >&2\n")
	builder.WriteString("exit 1\n")
	return builder.String()
}

func prepareWorkspace(root string, input BuildInput, runID, jobRoot, workspacePath string) ([]runEvent, error) {
	if err := copyContextFiles(input, jobRoot); err != nil {
		return nil, err
	}
	restored, events, err := restoreRequestedWorkspace(root, input.JobID, runID, input.WorkspacePath, jobRoot, workspacePath)
	if err != nil {
		return nil, err
	}
	if restored {
		return events, nil
	}
	return nil, seedWorkspace(input, jobRoot, workspacePath)
}

func restoreRequestedWorkspace(root, jobID, runID, requestedPath, jobRoot, workspacePath string) (bool, []runEvent, error) {
	resolvedPath, explicit, err := resolveRequestedWorkspacePath(root, jobID, requestedPath, jobRoot)
	if err != nil {
		return false, nil, err
	}
	if !explicit {
		return false, nil, nil
	}
	info, err := os.Stat(resolvedPath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil, fmt.Errorf("requested workspace restore source %q not found: %w", requestedPath, ErrInvalidRequest)
		}
		return false, nil, fmt.Errorf("stat requested workspace restore source: %w", err)
	}
	if !info.IsDir() {
		return false, nil, fmt.Errorf("requested workspace restore source %q is not a directory: %w", requestedPath, ErrInvalidRequest)
	}
	if filepath.Clean(resolvedPath) == filepath.Clean(workspacePath) {
		hasEntries, err := dirHasEntries(workspacePath)
		if err != nil {
			return false, nil, err
		}
		return hasEntries, nil, nil
	}
	events := make([]runEvent, 0, 2)
	if hasEntries, err := dirHasEntries(workspacePath); err != nil {
		return false, nil, err
	} else if hasEntries {
		archivedSnapshotPath, err := createWorkspaceSnapshot(root, jobID, workspacePath, "archived", "pre-restore")
		if err != nil {
			return false, nil, err
		}
		if err := pruneWorkspaceSnapshots(jobRoot, "archived", snapshotBucketRetainCount); err != nil {
			return false, nil, err
		}
		events = append(events, runEvent{
			At:           time.Now().UTC(),
			Type:         "workspace_archived",
			RunID:        runID,
			JobID:        jobID,
			Status:       StatusRunning,
			Summary:      "archived canonical workspace before restore",
			SnapshotPath: archivedSnapshotPath,
		})
	}
	if err := os.RemoveAll(workspacePath); err != nil {
		return false, nil, fmt.Errorf("reset workspace before restore: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(workspacePath), 0o755); err != nil {
		return false, nil, fmt.Errorf("prepare workspace parent: %w", err)
	}
	if err := copyDir(resolvedPath, workspacePath); err != nil {
		return false, nil, fmt.Errorf("restore workspace snapshot: %w", err)
	}
	restoredSnapshotPath := filepath.ToSlash(strings.TrimSpace(requestedPath))
	if filepath.IsAbs(strings.TrimSpace(requestedPath)) {
		if rel, err := filepath.Rel(root, resolvedPath); err == nil {
			restoredSnapshotPath = filepath.ToSlash(rel)
		}
	}
	events = append(events, runEvent{
		At:           time.Now().UTC(),
		Type:         "workspace_restored",
		RunID:        runID,
		JobID:        jobID,
		Status:       StatusRunning,
		Summary:      "restored workspace from snapshot",
		SnapshotPath: restoredSnapshotPath,
	})
	return true, events, nil
}

func createWorkspaceSnapshot(root, jobID, sourcePath, bucket, reason string) (string, error) {
	snapshotName := sanitizeSnapshotReason(reason) + "-" + time.Now().UTC().Format("20060102T150405Z")
	snapshotRelPath := filepath.ToSlash(filepath.Join("jobs", jobID, "snapshots", bucket, snapshotName))
	snapshotAbsPath := filepath.Join(root, filepath.FromSlash(snapshotRelPath))
	if err := os.RemoveAll(snapshotAbsPath); err != nil {
		return "", fmt.Errorf("reset %s snapshot dir: %w", bucket, err)
	}
	if err := copyDir(sourcePath, snapshotAbsPath); err != nil {
		return "", fmt.Errorf("copy %s workspace snapshot: %w", bucket, err)
	}
	return snapshotRelPath, nil
}

func pruneWorkspaceSnapshots(jobRoot, bucket string, keep int) error {
	if keep <= 0 {
		return nil
	}
	bucketPath := filepath.Join(jobRoot, "snapshots", bucket)
	entries, err := os.ReadDir(bucketPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read %s snapshot bucket: %w", bucket, err)
	}
	if len(entries) <= keep {
		return nil
	}
	slices.SortFunc(entries, func(a, b os.DirEntry) int {
		return strings.Compare(a.Name(), b.Name())
	})
	for _, entry := range entries[:len(entries)-keep] {
		if err := os.RemoveAll(filepath.Join(bucketPath, entry.Name())); err != nil {
			return fmt.Errorf("prune %s snapshot %q: %w", bucket, entry.Name(), err)
		}
	}
	return nil
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

func resolveRequestedWorkspacePath(root, jobID, requestedPath, jobRoot string) (string, bool, error) {
	trimmed := strings.TrimSpace(requestedPath)
	if trimmed == "" {
		return "", false, nil
	}
	if filepath.IsAbs(trimmed) {
		resolvedPath := filepath.Clean(trimmed)
		if !pathWithinBase(resolvedPath, jobRoot) {
			return "", false, nil
		}
		return resolvedPath, true, nil
	}
	resolvedPath := filepath.Clean(filepath.Join(root, filepath.FromSlash(trimmed)))
	if !pathWithinBase(resolvedPath, jobRoot) {
		return "", false, fmt.Errorf("requested workspace path %q escapes job %q: %w", requestedPath, jobID, ErrInvalidRequest)
	}
	return resolvedPath, true, nil
}

func pathWithinBase(path, base string) bool {
	rel, err := filepath.Rel(base, path)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}

func dirHasEntries(path string) (bool, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("read workspace directory: %w", err)
	}
	return len(entries) > 0, nil
}

func seedWorkspace(input BuildInput, jobRoot, workspacePath string) error {
	templateDir := strings.TrimSpace(input.TemplateSourceDir)
	if templateDir == "" {
		templateDir = defaultTemplateSourceDir(input.TemplateID)
	}
	if templateDir == "" {
		return nil
	}
	if _, err := os.Stat(templateDir); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("stat template dir: %w", err)
	}
	return copyDir(templateDir, workspacePath)
}

func copyContextFiles(input BuildInput, jobRoot string) error {
	sourceDir := strings.TrimSpace(input.ContextSourceDir)
	if sourceDir == "" {
		return nil
	}
	targetRoot := filepath.Join(jobRoot, "prepare")
	var files ContextFiles
	if err := json.Unmarshal(input.ContextFiles, &files); err != nil {
		return fmt.Errorf("decode context files: %w", err)
	}
	targets := []string{files.PRDMarkdownPath, files.PRDJSONPath, files.TemplateFitReportPath, files.ImplementationPlanPath, files.ManualConstraintsPath}
	targets = append(targets, files.SupportingFiles...)
	for _, target := range targets {
		if strings.TrimSpace(target) == "" {
			continue
		}
		targetPath := filepath.Join(targetRoot, filepath.Base(target))
		sourcePath := filepath.Join(sourceDir, filepath.Base(targetPath))
		if _, err := os.Stat(sourcePath); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return fmt.Errorf("stat context source file: %w", err)
		}
		if err := fileutil.CopyFile(sourcePath, targetPath, 0o644); err != nil {
			return fmt.Errorf("copy context file %s: %w", filepath.Base(targetPath), err)
		}
	}
	return nil
}

func copyDir(src, dst string) error {
	return filepath.WalkDir(src, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		if entry.IsDir() {
			if entry.Name() == ".dart_tool" || entry.Name() == ".idea" || entry.Name() == "build" {
				return filepath.SkipDir
			}
			return os.MkdirAll(filepath.Join(dst, rel), 0o755)
		}
		if strings.HasSuffix(entry.Name(), ".iml") {
			return nil
		}
		return fileutil.CopyFile(path, filepath.Join(dst, rel), 0o644)
	})
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

func firstToken(command string) string {
	parts := strings.Fields(command)
	if len(parts) == 0 {
		return ""
	}
	return parts[0]
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

func relToRoot(root, path string) string {
	if path == "" {
		return ""
	}
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return filepath.ToSlash(path)
	}
	return filepath.ToSlash(rel)
}
