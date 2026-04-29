package runs

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
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
	At            time.Time      `json:"at"`
	Type          string         `json:"type"`
	RunID         string         `json:"run_id"`
	JobID         string         `json:"job_id"`
	Summary       string         `json:"summary,omitempty"`
	Stage         ExecutionStage `json:"stage,omitempty"`
	Status        Status         `json:"status,omitempty"`
	RoundID       string         `json:"round_id,omitempty"`
	Attempt       int            `json:"attempt,omitempty"`
	CheckpointKey string         `json:"checkpoint_key,omitempty"`
	CurrentPhase  RoundPhase     `json:"current_phase,omitempty"`
	PhaseTrace    []RoundPhase   `json:"phase_trace,omitempty"`
	TargetPaths   []string       `json:"target_paths,omitempty"`
	AffectedPaths []string       `json:"affected_paths,omitempty"`
	SnapshotPath  string         `json:"snapshot_path,omitempty"`
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
	templateDir := resolveTemplateSourceDir(input)
	templateReferences, err := resolveTemplateReferenceFiles(input)
	if err != nil {
		return RunRecord{}, err
	}
	workspaceEvents, err := prepareWorkspace(store.root, input, runID, jobRoot, workspacePath)
	if err != nil {
		return RunRecord{}, err
	}
	input.SchemaVersion = NormalizeBuildInputSchemaVersion(input.SchemaVersion)
	input.PlanningPolicy = NormalizePlanningPolicySnapshot(input.PlanningPolicy)
	input.WorkspacePath = workspacePath
	input.ArtifactDir = artifactDir
	input.ContextFiles = canonicalizeContextFiles(jobRoot, input.ContextFiles)
	if len(input.TaskBundle) > 0 {
		normalizedTasks := make([]TaskBundleItem, 0, len(input.TaskBundle))
		for _, task := range input.TaskBundle {
			normalizedTasks = append(normalizedTasks, NormalizeTaskBundleItem(task))
		}
		input.TaskBundle = normalizedTasks
	}
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
		RunID:               runID,
		JobID:               input.JobID,
		BuilderID:           builderID,
		WorkerID:            workerID,
		LeaseID:             leaseID,
		Status:              StatusRunning,
		ExecutorImage:       input.ExecutorImage,
		GoalSummary:         input.GoalSummary,
		PlanningPolicy:      input.PlanningPolicy,
		HumanNotes:          append(json.RawMessage(nil), input.HumanNotes...),
		TaskBundle:          append([]TaskBundleItem(nil), input.TaskBundle...),
		AcceptanceChecks:    append([]AcceptanceCheck(nil), input.AcceptanceChecks...),
		AllowedPaths:        append([]string(nil), input.AllowedPaths...),
		ProtectedPaths:      append([]string(nil), input.ProtectedPaths...),
		KnowledgePack:       append([]ProfileSkill(nil), input.KnowledgePack...),
		TemplateSourceDir:   filepath.ToSlash(templateDir),
		TemplateReferenceFiles: templateReferences,
		PreparedInputDigest: PreparedInputDigest(input, input.ContextSourceDir),
		InputPath:           filepath.ToSlash(filepath.Join("jobs", input.JobID, "prepare", "builder-input.json")),
		WorkspacePath:       filepath.ToSlash(workspacePath),
		ArtifactDir:         filepath.ToSlash(artifactDir),
		RunnerScriptPath:    relToRoot(store.root, runnerScriptPath),
		LaunchCommand:       launchCommand,
		LaunchArgs:          launchArgs,
		LogPath:             relToRoot(store.root, logPath),
		EventsPath:          filepath.ToSlash(filepath.Join("jobs", input.JobID, "runs", runID, "events.jsonl")),
		RoundState:          cloneRoundState(input.InitialRoundState),
		CreatedAt:           now,
		UpdatedAt:           now,
		StartedAt:           now,
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
	eventType := strings.TrimSpace(heartbeat.EventType)
	if eventType == "" {
		eventType = "run_heartbeat"
	}
	record.LastStage = heartbeat.Stage
	record.IterationCount = heartbeat.Iteration
	record.TotalTokens = heartbeat.TotalTokens
	record.FailureSignatures = append([]string(nil), heartbeat.FailureSignatures...)
	if heartbeat.RoundState != nil {
		record.RoundState = mergeHeartbeatRoundState(record.RoundState, heartbeat.RoundState)
	}
	roundChanged := eventType == "run_heartbeat" && hasHeartbeatRoundTransition(record, heartbeat)
	if heartbeat.RoundState != nil || strings.TrimSpace(heartbeat.RoundID) != "" || heartbeat.Attempt > 0 || len(heartbeat.TargetPaths) > 0 || strings.TrimSpace(heartbeat.CheckpointKey) != "" {
		record.CurrentRoundID = strings.TrimSpace(heartbeat.RoundID)
		record.CurrentRoundAttempt = heartbeat.Attempt
		record.CurrentCheckpointKey = strings.TrimSpace(heartbeat.CheckpointKey)
		record.CurrentRoundTargetPaths = trimAndDedupeEventPaths(heartbeat.TargetPaths)
	}
	record.UpdatedAt = now
	if err := writeJSON(store.runPath(record.JobID, record.RunID), record); err != nil {
		return RunRecord{}, err
	}
	if roundChanged {
		roundEvent := runEvent{
			At:            now,
			Type:          "run_round_started",
			RunID:         record.RunID,
			JobID:         record.JobID,
			Summary:       buildRoundStartedSummary(heartbeat.RoundID, heartbeat.Attempt),
			Stage:         heartbeat.Stage,
			Status:        record.Status,
			RoundID:       strings.TrimSpace(heartbeat.RoundID),
			Attempt:       heartbeat.Attempt,
			CheckpointKey: strings.TrimSpace(heartbeat.CheckpointKey),
			TargetPaths:   trimAndDedupeEventPaths(heartbeat.TargetPaths),
		}
		if heartbeat.RoundState != nil {
			roundEvent.CurrentPhase = heartbeat.RoundState.CurrentPhase
			roundEvent.PhaseTrace = append([]RoundPhase(nil), heartbeat.RoundState.PhaseTrace...)
		}
		if err := store.appendEvent(record, roundEvent); err != nil {
			return RunRecord{}, err
		}
	}
	event := runEvent{
		At:            now,
		Type:          eventType,
		RunID:         record.RunID,
		JobID:         record.JobID,
		Summary:       heartbeat.Summary,
		Stage:         heartbeat.Stage,
		Status:        record.Status,
		RoundID:       strings.TrimSpace(heartbeat.RoundID),
		Attempt:       heartbeat.Attempt,
		CheckpointKey: strings.TrimSpace(heartbeat.CheckpointKey),
	}
	if heartbeat.RoundState != nil {
		event.CurrentPhase = heartbeat.RoundState.CurrentPhase
		event.PhaseTrace = append([]RoundPhase(nil), heartbeat.RoundState.PhaseTrace...)
	}
	event.TargetPaths = trimAndDedupeEventPaths(heartbeat.TargetPaths)
	event.AffectedPaths = trimAndDedupeEventPaths(heartbeat.AffectedPaths)
	if err := store.appendEvent(record, event); err != nil {
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
	existingPatchRounds, err := store.existingPatchEventRoundIDs(record)
	if err != nil {
		return RunRecord{}, err
	}
	terminalEvents := buildRoundTerminalEvents(record, output, now, existingPatchRounds)
	for index, event := range terminalEvents {
		event.At = now.Add(-time.Duration(len(terminalEvents)-index) * time.Nanosecond)
		if err := store.appendEvent(record, event); err != nil {
			return RunRecord{}, err
		}
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
	if len(state.TaskStatuses) > 0 {
		cloned.TaskStatuses = maps.Clone(state.TaskStatuses)
	}
	return &cloned
}

func mergeHeartbeatRoundState(existing, next *RoundState) *RoundState {
	merged := cloneRoundState(next)
	if merged == nil {
		return cloneRoundState(existing)
	}
	if existing == nil {
		return merged
	}
	if strings.TrimSpace(merged.CurrentTaskID) == "" {
		merged.CurrentTaskID = strings.TrimSpace(existing.CurrentTaskID)
	}
	if len(existing.TaskStatuses) > 0 {
		if merged.TaskStatuses == nil {
			merged.TaskStatuses = map[string]BuilderRuntimeTaskStatus{}
		}
		for taskID, status := range existing.TaskStatuses {
			normalizedTaskID := strings.TrimSpace(taskID)
			if normalizedTaskID == "" {
				continue
			}
			if _, exists := merged.TaskStatuses[normalizedTaskID]; exists {
				continue
			}
			merged.TaskStatuses[normalizedTaskID] = status
		}
	}
	return merged
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

func trimAndDedupeEventPaths(items []string) []string {
	if len(items) == 0 {
		return nil
	}
	result := make([]string, 0, len(items))
	seen := map[string]struct{}{}
	for _, item := range items {
		normalized := strings.TrimSpace(item)
		if normalized == "" {
			continue
		}
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		result = append(result, normalized)
	}
	slices.Sort(result)
	return result
}

func hasHeartbeatRoundTransition(record RunRecord, heartbeat Heartbeat) bool {
	roundID := strings.TrimSpace(heartbeat.RoundID)
	if roundID == "" {
		return false
	}
	if roundID != strings.TrimSpace(record.CurrentRoundID) {
		return true
	}
	return heartbeat.Attempt > 0 && heartbeat.Attempt != record.CurrentRoundAttempt
}

func buildRoundStartedSummary(roundID string, attempt int) string {
	roundID = strings.TrimSpace(roundID)
	if roundID == "" {
		return "round started"
	}
	if attempt > 0 {
		return fmt.Sprintf("round %s started (attempt %d)", roundID, attempt)
	}
	return fmt.Sprintf("round %s started", roundID)
}

func buildRoundTerminalEvents(record RunRecord, output BuildOutput, completedAt time.Time, existingPatchRounds map[string]struct{}) []runEvent {
	if len(output.RoundOutputs) == 0 {
		return nil
	}
	roundInputsByID := map[string]RoundInput{}
	for _, roundInput := range output.RoundInputs {
		roundID := strings.TrimSpace(roundInput.RoundID)
		if roundID == "" {
			continue
		}
		roundInputsByID[roundID] = roundInput
	}
	events := make([]runEvent, 0, len(output.RoundOutputs))
	for _, roundOutput := range output.RoundOutputs {
		roundID := strings.TrimSpace(roundOutput.RoundID)
		if _, ok := existingPatchRounds[roundID]; ok {
			continue
		}
		roundInput := roundInputsByID[roundID]
		affectedPaths := collectRoundOutputModifiedPaths(roundOutput)
		if len(affectedPaths) == 0 {
			continue
		}
		event := runEvent{
			At:            completedAt,
			Type:          "run_patch_applied",
			RunID:         record.RunID,
			JobID:         record.JobID,
			Summary:       buildPatchAppliedSummary(roundID, affectedPaths),
			RoundID:       roundID,
			Attempt:       roundInput.Attempt,
			TargetPaths:   collectRoundInputTargetPaths(roundInput),
			AffectedPaths: affectedPaths,
		}
		if roundOutput.State != nil {
			event.CurrentPhase = roundOutput.State.CurrentPhase
			event.PhaseTrace = append([]RoundPhase(nil), roundOutput.State.PhaseTrace...)
		}
		events = append(events, event)
	}
	return events
}

func (store *FileStore) existingPatchEventRoundIDs(record RunRecord) (map[string]struct{}, error) {
	path := store.eventsPath(record.JobID, record.RunID)
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]struct{}{}, nil
		}
		return nil, fmt.Errorf("open job events: %w", err)
	}
	defer file.Close()
	seen := map[string]struct{}{}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var event runEvent
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			return nil, fmt.Errorf("decode job event: %w", err)
		}
		if strings.TrimSpace(event.Type) != "run_patch_applied" {
			continue
		}
		roundID := strings.TrimSpace(event.RoundID)
		if roundID == "" {
			continue
		}
		seen[roundID] = struct{}{}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan job events: %w", err)
	}
	return seen, nil
}

func buildPatchAppliedSummary(roundID string, affectedPaths []string) string {
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

func collectRoundInputTargetPaths(input RoundInput) []string {
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
	slices.Sort(paths)
	return paths
}

func collectRoundOutputModifiedPaths(output RoundOutput) []string {
	if output.WorkspacePatch == nil {
		return nil
	}
	return trimAndDedupeEventPaths(output.WorkspacePatch.ModifiedFiles)
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
	if !IsSupportedBuildInputSchemaVersion(input.SchemaVersion) {
		return fmt.Errorf("build input schema_version %q: %w", input.SchemaVersion, ErrInvalidRequest)
	}
	if err := validatePlanningPolicy(input.PlanningPolicy); err != nil {
		return err
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
	for _, task := range input.TaskBundle {
		if NormalizeTaskCategory(string(task.Category)) == "" {
			return fmt.Errorf("task bundle category %q: %w", task.Category, ErrInvalidRequest)
		}
		if task.TaskType != "" && task.EffectiveTaskType() == "" {
			return fmt.Errorf("task bundle task_type %q: %w", task.TaskType, ErrInvalidRequest)
		}
		if task.RiskLevel != "" && NormalizeTaskRiskLevel(string(task.RiskLevel)) == "" {
			return fmt.Errorf("task bundle risk_level %q: %w", task.RiskLevel, ErrInvalidRequest)
		}
		if task.RouteHint != "" && NormalizeTaskRouteHint(string(task.RouteHint)) == "" {
			return fmt.Errorf("task bundle route_hint %q: %w", task.RouteHint, ErrInvalidRequest)
		}
		if transition := normalizeTaskAllocationTransition(task.AllocationTransition, task); transition == nil || transition.AllocationID == "" {
			return fmt.Errorf("task bundle allocation_transition %q: %w", task.TaskID, ErrInvalidRequest)
		}
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

func validatePlanningPolicy(policy PlanningPolicySnapshot) error {
	normalized := NormalizePlanningPolicySnapshot(policy)
	if normalized.PolicyVersion == "" {
		return fmt.Errorf("planning policy version: %w", ErrInvalidRequest)
	}
	if len(normalized.Stages) == 0 {
		return fmt.Errorf("planning policy stages: %w", ErrInvalidRequest)
	}
	seen := make(map[PlanningStage]struct{}, len(normalized.Stages))
	for _, stage := range normalized.Stages {
		if stage.Stage == "" {
			return fmt.Errorf("planning policy stage: %w", ErrInvalidRequest)
		}
		if stage.Route == "" {
			return fmt.Errorf("planning policy route: %w", ErrInvalidRequest)
		}
		if _, ok := seen[stage.Stage]; ok {
			return fmt.Errorf("planning policy duplicate stage %q: %w", stage.Stage, ErrInvalidRequest)
		}
		seen[stage.Stage] = struct{}{}
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
			if isShellSnippet(trimmed) {
				continue
			}
			for _, name := range executableTokens(trimmed) {
				if len(profile.AllowedCommands) > 0 && !slices.Contains(profile.AllowedCommands, name) {
					return "", fmt.Errorf("command %q not allowed: %w", name, ErrInvalidRequest)
				}
				if slices.Contains(profile.DeniedCommands, name) {
					return "", fmt.Errorf("command %q denied: %w", name, ErrInvalidRequest)
				}
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
	templateDir := resolveTemplateSourceDir(input)
	if templateDir == "" {
		return nil
	}
	if _, err := os.Stat(templateDir); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("stat template dir: %w", err)
	}
	if shouldUseReferenceTemplateSeed(input) {
		return seedReferenceTemplateWorkspace(templateDir, workspacePath)
	}
	return copyDir(templateDir, workspacePath)
}

func shouldUseReferenceTemplateSeed(input BuildInput) bool {
	return strings.TrimSpace(input.TemplateID) == "flutter-open-lite"
}

func resolveTemplateReferenceFiles(input BuildInput) (map[string]string, error) {
	if !shouldUseReferenceTemplateSeed(input) {
		return nil, nil
	}
	templateDir := resolveTemplateSourceDir(input)
	if strings.TrimSpace(templateDir) == "" {
		return nil, nil
	}
	references := make(map[string]string)
	err := filepath.WalkDir(templateDir, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(templateDir, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if entry.IsDir() {
			if shouldSkipSeedWorkspaceDir(entry.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if shouldSkipSeedWorkspaceFile(entry.Name()) || !entry.Type().IsRegular() {
			return nil
		}
		if !shouldTrackReferenceTemplateFile(rel) {
			return nil
		}
		references[rel] = filepath.ToSlash(path)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk template reference files: %w", err)
	}
	if len(references) == 0 {
		return nil, nil
	}
	return references, nil
}

func seedReferenceTemplateWorkspace(templateDir, workspacePath string) error {
	if err := copyDirFiltered(templateDir, workspacePath, shouldCopyReferenceSeedPath); err != nil {
		return err
	}
	return writeFlutterOpenLiteSeedMain(filepath.Join(workspacePath, "lib", "main.dart"))
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
	return copyDirFiltered(src, dst, func(relPath string, entry os.DirEntry) bool {
		return true
	})
}

func copyDirFiltered(src, dst string, allow func(relPath string, entry os.DirEntry) bool) error {
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
		rel = filepath.ToSlash(rel)
		if entry.IsDir() {
			if shouldSkipSeedWorkspaceDir(entry.Name()) {
				return filepath.SkipDir
			}
			if !allow(rel, entry) {
				return filepath.SkipDir
			}
			return os.MkdirAll(filepath.Join(dst, filepath.FromSlash(rel)), 0o755)
		}
		if shouldSkipSeedWorkspaceFile(entry.Name()) || !entry.Type().IsRegular() {
			return nil
		}
		if !allow(rel, entry) {
			return nil
		}
		return fileutil.CopyFile(path, filepath.Join(dst, filepath.FromSlash(rel)), 0o644)
	})
}

func shouldCopyReferenceSeedPath(relPath string, entry os.DirEntry) bool {
	if entry.IsDir() {
		return !shouldSkipReferenceTemplateDir(relPath)
	}
	if relPath == "lib/main.dart" {
		return false
	}
	return !shouldSkipReferenceTemplateFile(relPath)
}

func shouldSkipReferenceTemplateDir(relPath string) bool {
	for _, prefix := range []string{
		"lib/models",
		"lib/controllers",
		"lib/views",
		"lib/repositories",
		"lib/template",
	} {
		if relPath == prefix || strings.HasPrefix(relPath, prefix+"/") {
			return true
		}
	}
	return false
}

func shouldSkipReferenceTemplateFile(relPath string) bool {
	if relPath == "test/widget_test.dart" {
		return true
	}
	return shouldSkipReferenceTemplateDir(filepath.ToSlash(filepath.Dir(relPath)))
}

func shouldTrackReferenceTemplateFile(relPath string) bool {
	if relPath == "lib/main.dart" {
		return true
	}
	return shouldSkipReferenceTemplateFile(relPath)
}

func writeFlutterOpenLiteSeedMain(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create seed main dir: %w", err)
	}
	const content = `import 'package:flutter/material.dart';
import 'package:hive_flutter/hive_flutter.dart';

Future<void> main() async {
  WidgetsFlutterBinding.ensureInitialized();
  await Hive.initFlutter();
  runApp(const AppFactorySeedApp());
}

class AppFactorySeedApp extends StatelessWidget {
  const AppFactorySeedApp({super.key});

  @override
  Widget build(BuildContext context) {
    return MaterialApp(
      title: 'AppFactory Seed',
      theme: ThemeData(
        colorScheme: ColorScheme.fromSeed(seedColor: const Color(0xFF1565C0)),
        useMaterial3: true,
      ),
      home: const AppFactorySeedHomePage(),
    );
  }
}

class AppFactorySeedHomePage extends StatelessWidget {
  const AppFactorySeedHomePage({super.key});

  @override
  Widget build(BuildContext context) {
    return const Scaffold(
      body: Center(
        child: Text('Seed workspace ready'),
      ),
    );
  }
}
`
	return fileutil.WriteFileAtomic(path, []byte(content), 0o644)
}

func isShellSnippet(command string) bool {
	return strings.Contains(command, "\n") || strings.Contains(command, "$((") || strings.Contains(command, "$(") || strings.Contains(command, "`")
}

func executableTokens(command string) []string {
	replacer := strings.NewReplacer("&&", "\n", "||", "\n", ";", "\n")
	segments := strings.Split(replacer.Replace(command), "\n")
	tokens := make([]string, 0, len(segments))
	for _, segment := range segments {
		name := firstExecutableToken(segment)
		if name == "" {
			continue
		}
		tokens = append(tokens, name)
	}
	return tokens
}

func firstExecutableToken(command string) string {
	fields := strings.Fields(strings.TrimSpace(command))
	if len(fields) == 0 {
		return ""
	}
	for len(fields) > 0 {
		token := fields[0]
		switch {
		case strings.HasPrefix(token, "#"):
			return ""
		case strings.HasPrefix(token, "-"):
			return ""
		case strings.HasSuffix(token, "()"):
			return ""
		case isShellAssignmentToken(token):
			fields = fields[1:]
			continue
		case isShellControlToken(token):
			fields = fields[1:]
			continue
		case isShellBuiltinToken(token):
			return ""
		default:
			return token
		}
	}
	return ""
}

func isShellAssignmentToken(token string) bool {
	if token == "" {
		return false
	}
	separator := strings.Index(token, "=")
	if separator <= 0 {
		return false
	}
	for index, char := range token[:separator] {
		if index == 0 {
			if !(char == '_' || (char >= 'A' && char <= 'Z') || (char >= 'a' && char <= 'z')) {
				return false
			}
			continue
		}
		if !(char == '_' || (char >= 'A' && char <= 'Z') || (char >= 'a' && char <= 'z') || (char >= '0' && char <= '9')) {
			return false
		}
	}
	return true
}

func isShellControlToken(token string) bool {
	switch token {
	case "{", "}", "(", ")", "if", "then", "else", "elif", "fi", "do", "done", "while", "for", "case", "esac", "in", "!":
		return true
	default:
		return false
	}
}

func isShellBuiltinToken(token string) bool {
	switch token {
	case "[", "test", "command", "echo", "printf", "export", "local", "readonly", "return", "exit", "true", "false", ":":
		return true
	default:
		return false
	}
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
