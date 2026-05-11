package api

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	appruns "github.com/sipeed/oneappfactory/pkg/appfactory/runs"
	"github.com/sipeed/oneappfactory/pkg/fileutil"
)

type publicJobExecutionRecord struct {
	SchemaVersion   string                      `json:"schema_version"`
	JobID           string                      `json:"job_id"`
	RunID           string                      `json:"run_id"`
	Action          string                      `json:"action"`
	Status          string                      `json:"status"`
	AttemptCount    int                         `json:"attempt_count,omitempty"`
	Attempts        []publicJobExecutionAttempt `json:"attempts,omitempty"`
	LeaseOwnerID    string                      `json:"lease_owner_id,omitempty"`
	LeaseAcquiredAt string                      `json:"lease_acquired_at,omitempty"`
	LeaseExpiresAt  string                      `json:"lease_expires_at,omitempty"`
	TimeoutSeconds  int                         `json:"timeout_seconds,omitempty"`
	RequestedAt     string                      `json:"requested_at"`
	StartedAt       string                      `json:"started_at,omitempty"`
	FinishedAt      string                      `json:"finished_at,omitempty"`
	LastError       string                      `json:"last_error,omitempty"`
}

type publicJobExecutionAttempt struct {
	Attempt       int    `json:"attempt"`
	OwnerID       string `json:"owner_id"`
	ClaimedAt     string `json:"claimed_at"`
	LastRenewedAt string `json:"last_renewed_at,omitempty"`
	ReleasedAt    string `json:"released_at,omitempty"`
	ReleaseReason string `json:"release_reason,omitempty"`
}

type PublicJobOrchestratorStatus struct {
	SchemaVersion         string `json:"schema_version"`
	InstanceID            string `json:"instance_id"`
	State                 string `json:"state"`
	WatchRunnerState      string `json:"watch_runner_state,omitempty"`
	WatchRunnerMode       string `json:"watch_runner_mode,omitempty"`
	WatchRunnerOwnerID    string `json:"watch_runner_owner_id,omitempty"`
	WatchRunnerInterval   int    `json:"watch_runner_interval_seconds,omitempty"`
	WatchRunnerStartedAt  string `json:"watch_runner_started_at,omitempty"`
	WatchRunnerStoppedAt  string `json:"watch_runner_stopped_at,omitempty"`
	WatchRunnerLastError  string `json:"watch_runner_last_error,omitempty"`
	WatchLockState        string `json:"watch_lock_state,omitempty"`
	WatchLockOwnerID      string `json:"watch_lock_owner_id,omitempty"`
	WatchLockMode         string `json:"watch_lock_mode,omitempty"`
	WatchLockAcquiredAt   string `json:"watch_lock_acquired_at,omitempty"`
	WatchLockUpdatedAt    string `json:"watch_lock_updated_at,omitempty"`
	WatchLockExpiresAt    string `json:"watch_lock_expires_at,omitempty"`
	LastTrigger           string `json:"last_trigger,omitempty"`
	LastPassStartedAt     string `json:"last_pass_started_at,omitempty"`
	LastPassFinishedAt    string `json:"last_pass_finished_at,omitempty"`
	QueuedRecoveries      int    `json:"queued_recoveries,omitempty"`
	RunningRecoveries     int    `json:"running_recoveries,omitempty"`
	ForeignLiveLeaseSkips int    `json:"foreign_live_lease_skips,omitempty"`
	LastError             string `json:"last_error,omitempty"`
	UpdatedAt             string `json:"updated_at"`
}

type publicJobOrchestratorWatchLock struct {
	SchemaVersion string `json:"schema_version"`
	OwnerID       string `json:"owner_id"`
	Mode          string `json:"mode,omitempty"`
	AcquiredAt    string `json:"acquired_at"`
	UpdatedAt     string `json:"updated_at"`
	ExpiresAt     string `json:"expires_at"`
}

type publicJobOrchestratorWatchAuditRecord struct {
	SchemaVersion    string `json:"schema_version"`
	AuditID          string `json:"audit_id"`
	Action           string `json:"action"`
	Actor            string `json:"actor,omitempty"`
	RemoteAddr       string `json:"remote_addr,omitempty"`
	UserAgent        string `json:"user_agent,omitempty"`
	Force            bool   `json:"force,omitempty"`
	Summary          string `json:"summary,omitempty"`
	WatchRunnerState string `json:"watch_runner_state,omitempty"`
	WatchRunnerMode  string `json:"watch_runner_mode,omitempty"`
	WatchLockState   string `json:"watch_lock_state,omitempty"`
	WatchLockOwnerID string `json:"watch_lock_owner_id,omitempty"`
	CreatedAt        string `json:"created_at"`
}

type PublicJobOrchestratorWatchRunResult struct {
	Status          PublicJobOrchestratorStatus
	CompletedPasses int
}

var publicJobExecutionTerminalStatuses = map[string]struct{}{
	"completed": {},
	"failed":    {},
	"cancelled": {},
}

var publicJobExecutionAllowedStatuses = map[string]struct{}{
	"queued":    {},
	"running":   {},
	"completed": {},
	"failed":    {},
	"cancelled": {},
}

var publicJobExecutionSameRunTransitions = map[string]map[string]struct{}{
	"queued": {
		"queued":    {},
		"running":   {},
		"completed": {},
		"failed":    {},
		"cancelled": {},
	},
	"running": {
		"running":   {},
		"completed": {},
		"failed":    {},
		"cancelled": {},
	},
	"completed": {
		"completed": {},
	},
	"failed": {
		"failed": {},
	},
	"cancelled": {
		"cancelled": {},
	},
}

var (
	publicJobExecutionLeaseWindow        = 2 * time.Minute
	publicJobExecutionLeaseRenewInterval = publicJobExecutionLeaseWindow / 3
	publicJobOrchestratorWatchLockWindow = 1 * time.Minute
)

func (h *Handler) enqueuePublicJobExecution(jobID, runID, action string, timeout time.Duration) error {
	now := time.Now().UTC().Format(time.RFC3339)
	record := publicJobExecutionRecord{
		SchemaVersion:  "0.1.0",
		JobID:          jobID,
		RunID:          runID,
		Action:         strings.TrimSpace(action),
		Status:         "queued",
		TimeoutSeconds: int(timeout / time.Second),
		RequestedAt:    now,
	}
	if err := h.persistPublicJobExecutionRecord(record); err != nil {
		return err
	}
	h.launchQueuedPublicJobExecution(jobID)
	return nil
}

func (h *Handler) recoverPublicJobExecutions() {
	_, _ = h.RunPublicJobOrchestratorPass("legacy_recover")
}

func (h *Handler) RunPublicJobOrchestratorPass(trigger string) (PublicJobOrchestratorStatus, error) {
	h.orchestratorPassMu.Lock()
	defer h.orchestratorPassMu.Unlock()

	status := h.currentPublicJobOrchestratorStatusSnapshot()
	status.LastTrigger = strings.TrimSpace(trigger)
	status.State = "running"
	status.LastPassStartedAt = time.Now().UTC().Format(time.RFC3339)
	status.UpdatedAt = status.LastPassStartedAt
	_ = h.persistPublicJobOrchestratorStatus(status)

	workspace, err := h.appFactoryWorkspacePath()
	if err != nil {
		status.State = "error"
		status.LastError = err.Error()
		status.LastPassFinishedAt = time.Now().UTC().Format(time.RFC3339)
		status.UpdatedAt = status.LastPassFinishedAt
		return status, err
	}
	records, err := h.listPublicJobExecutionRecords(workspace)
	if err != nil {
		status.State = "error"
		status.LastError = err.Error()
		status.LastPassFinishedAt = time.Now().UTC().Format(time.RFC3339)
		status.UpdatedAt = status.LastPassFinishedAt
		_ = h.persistPublicJobOrchestratorStatus(status)
		return status, err
	}
	for _, record := range records {
		if publicJobExecutionLeaseOwnedByAnotherLiveDispatcher(record, h.orchestratorInstanceID) {
			status.ForeignLiveLeaseSkips++
			continue
		}
		switch record.Status {
		case "queued":
			status.QueuedRecoveries++
			h.launchQueuedPublicJobExecution(record.JobID)
		case "running":
			status.RunningRecoveries++
			h.recoverRunningPublicJobExecution(workspace, record)
		}
	}
	status.State = "idle"
	status.LastPassFinishedAt = time.Now().UTC().Format(time.RFC3339)
	status.UpdatedAt = status.LastPassFinishedAt
	if err := h.persistPublicJobOrchestratorStatus(status); err != nil {
		return status, err
	}
	return status, nil
}

func (h *Handler) StartPublicJobOrchestratorWatch(interval time.Duration) (PublicJobOrchestratorStatus, error) {
	if interval <= 0 {
		return PublicJobOrchestratorStatus{}, fmt.Errorf("orchestrator watch interval must be positive")
	}
	h.orchestratorWatchMu.Lock()
	if h.orchestratorWatchRun {
		h.orchestratorWatchMu.Unlock()
		status, _ := h.LoadPublicJobOrchestratorStatus()
		return status, fmt.Errorf("orchestrator watch already running")
	}
	if err := h.AcquirePublicJobOrchestratorWatchLock("internal_api_watch"); err != nil {
		h.orchestratorWatchMu.Unlock()
		return PublicJobOrchestratorStatus{}, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	now := time.Now().UTC().Format(time.RFC3339)
	h.orchestratorWatchRun = true
	h.orchestratorWatchStop = false
	h.orchestratorWatchMode = "internal_api_watch"
	h.orchestratorWatchEvery = interval
	h.orchestratorWatchStart = now
	h.orchestratorWatchEnd = ""
	h.orchestratorWatchError = ""
	h.orchestratorWatchDone = make(chan struct{})
	h.orchestratorWatchCancel = cancel
	h.orchestratorWatchMu.Unlock()
	if err := h.persistCurrentPublicJobOrchestratorStatus(); err != nil {
		_ = h.ReleasePublicJobOrchestratorWatchLock()
		h.orchestratorWatchMu.Lock()
		h.orchestratorWatchRun = false
		h.orchestratorWatchStop = false
		h.orchestratorWatchMode = ""
		h.orchestratorWatchEvery = 0
		h.orchestratorWatchStart = ""
		h.orchestratorWatchEnd = ""
		h.orchestratorWatchError = ""
		h.orchestratorWatchDone = nil
		h.orchestratorWatchCancel = nil
		h.orchestratorWatchMu.Unlock()
		return PublicJobOrchestratorStatus{}, err
	}
	h.orchestratorWatchMu.Lock()
	done := h.orchestratorWatchDone
	h.orchestratorWatchMu.Unlock()
	h.asyncJobs.Add(1)
	go h.runPublicJobOrchestratorWatch(ctx, done, interval)
	return h.LoadPublicJobOrchestratorStatus()
}

func (h *Handler) StopPublicJobOrchestratorWatch() error {
	h.orchestratorWatchMu.Lock()
	if !h.orchestratorWatchRun {
		h.orchestratorWatchMu.Unlock()
		return nil
	}
	h.orchestratorWatchStop = true
	cancel := h.orchestratorWatchCancel
	done := h.orchestratorWatchDone
	h.orchestratorWatchMu.Unlock()
	_ = h.persistCurrentPublicJobOrchestratorStatus()
	if cancel != nil {
		cancel()
	}
	if done != nil {
		<-done
	}
	return nil
}

func (h *Handler) runPublicJobOrchestratorWatch(ctx context.Context, done chan struct{}, interval time.Duration) {
	defer h.asyncJobs.Done()
	defer close(done)
	defer func() {
		h.orchestratorWatchMu.Lock()
		h.orchestratorWatchRun = false
		h.orchestratorWatchStop = false
		h.orchestratorWatchMode = ""
		h.orchestratorWatchEvery = interval
		h.orchestratorWatchEnd = time.Now().UTC().Format(time.RFC3339)
		h.orchestratorWatchDone = nil
		h.orchestratorWatchCancel = nil
		h.orchestratorWatchMu.Unlock()
		_ = h.persistCurrentPublicJobOrchestratorStatus()
	}()
	result, err := h.RunPublicJobOrchestratorWatch(ctx, "internal_api_watch", interval, 0)
	if err != nil {
		h.setPublicJobOrchestratorWatchError(err.Error())
	}
	if result.Status.WatchRunnerLastError != "" {
		h.setPublicJobOrchestratorWatchError(result.Status.WatchRunnerLastError)
	}
}

func (h *Handler) RunPublicJobOrchestratorWatch(ctx context.Context, mode string, interval time.Duration, maxPasses int) (PublicJobOrchestratorWatchRunResult, error) {
	mode = strings.TrimSpace(mode)
	if mode == "" {
		return PublicJobOrchestratorWatchRunResult{}, fmt.Errorf("orchestrator watch mode is required")
	}
	if interval <= 0 {
		return PublicJobOrchestratorWatchRunResult{}, fmt.Errorf("orchestrator watch interval must be positive")
	}
	if err := h.AcquirePublicJobOrchestratorWatchLock(mode); err != nil {
		return PublicJobOrchestratorWatchRunResult{}, err
	}
	startedAt := time.Now().UTC().Format(time.RFC3339)
	if err := h.persistPublicJobOrchestratorWatchRunnerSnapshot("running", mode, interval, startedAt, "", ""); err != nil {
		_ = h.ReleasePublicJobOrchestratorWatchLock()
		return PublicJobOrchestratorWatchRunResult{}, err
	}
	result := PublicJobOrchestratorWatchRunResult{}
	lastError := ""
	defer func() {
		stoppedAt := time.Now().UTC().Format(time.RFC3339)
		if releaseErr := h.ReleasePublicJobOrchestratorWatchLock(); releaseErr != nil && lastError == "" {
			lastError = releaseErr.Error()
		}
		_ = h.persistPublicJobOrchestratorWatchRunnerSnapshot("not_running", mode, interval, startedAt, stoppedAt, lastError)
	}()
	runPass := func() error {
		status, err := h.RunPublicJobOrchestratorPass(mode)
		result.Status = status
		if err != nil {
			lastError = err.Error()
			result.Status.WatchRunnerLastError = lastError
			_ = h.persistPublicJobOrchestratorWatchRunnerSnapshot("running", mode, interval, startedAt, "", lastError)
			return err
		}
		if err := h.RenewPublicJobOrchestratorWatchLock(mode); err != nil {
			lastError = err.Error()
			result.Status.WatchRunnerLastError = lastError
			_ = h.persistPublicJobOrchestratorWatchRunnerSnapshot("running", mode, interval, startedAt, "", lastError)
			return err
		}
		lastError = ""
		result.CompletedPasses++
		if loaded, loadErr := h.LoadPublicJobOrchestratorStatus(); loadErr == nil {
			result.Status = loaded
		}
		_ = h.persistPublicJobOrchestratorWatchRunnerSnapshot("running", mode, interval, startedAt, "", "")
		return nil
	}
	if err := runPass(); err != nil {
		return result, err
	}
	if maxPasses > 0 && result.CompletedPasses >= maxPasses {
		return result, nil
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return result, nil
		case <-ticker.C:
			if err := runPass(); err != nil {
				return result, err
			}
			if maxPasses > 0 && result.CompletedPasses >= maxPasses {
				return result, nil
			}
		}
	}
}

func (h *Handler) LoadPublicJobOrchestratorStatus() (PublicJobOrchestratorStatus, error) {
	workspace, err := h.appFactoryWorkspacePath()
	if err != nil {
		return h.defaultPublicJobOrchestratorStatus(), err
	}
	watchLock, lockErr := h.loadPublicJobOrchestratorWatchLock(workspace)
	if lockErr != nil {
		return h.defaultPublicJobOrchestratorStatus(), lockErr
	}
	status, err := h.loadPublicJobOrchestratorStatus(workspace)
	if err != nil {
		return h.defaultPublicJobOrchestratorStatus(), err
	}
	if status == nil {
		defaultStatus := h.defaultPublicJobOrchestratorStatus()
		hydratePublicJobOrchestratorStatusWatchRunner(&defaultStatus, h)
		hydratePublicJobOrchestratorStatusWatchLock(&defaultStatus, watchLock, h.orchestratorInstanceID)
		return defaultStatus, nil
	}
	hydratePublicJobOrchestratorStatusWatchRunner(status, h)
	hydratePublicJobOrchestratorStatusWatchLock(status, watchLock, h.orchestratorInstanceID)
	return *status, nil
}

func (h *Handler) defaultPublicJobOrchestratorStatus() PublicJobOrchestratorStatus {
	now := time.Now().UTC().Format(time.RFC3339)
	return PublicJobOrchestratorStatus{
		SchemaVersion:    "0.1.0",
		InstanceID:       h.orchestratorInstanceID,
		State:            "not_started",
		WatchRunnerState: "not_running",
		WatchLockState:   "unlocked",
		UpdatedAt:        now,
	}
}

func (h *Handler) persistedPublicJobOrchestratorStatusSnapshot() PublicJobOrchestratorStatus {
	status := h.defaultPublicJobOrchestratorStatus()
	workspace, err := h.appFactoryWorkspacePath()
	if err != nil {
		return status
	}
	loaded, loadErr := h.loadPublicJobOrchestratorStatus(workspace)
	if loadErr == nil && loaded != nil {
		status = *loaded
	}
	return status
}

func (h *Handler) currentPublicJobOrchestratorStatusSnapshot() PublicJobOrchestratorStatus {
	status := h.persistedPublicJobOrchestratorStatusSnapshot()
	hydratePublicJobOrchestratorStatusWatchRunner(&status, h)
	return status
}

func (h *Handler) setPublicJobOrchestratorWatchError(message string) {
	h.orchestratorWatchMu.Lock()
	defer h.orchestratorWatchMu.Unlock()
	h.orchestratorWatchError = strings.TrimSpace(message)
}

func (h *Handler) persistCurrentPublicJobOrchestratorStatus() error {
	status := h.currentPublicJobOrchestratorStatusSnapshot()
	return h.persistPublicJobOrchestratorStatus(status)
}

func (h *Handler) persistPublicJobOrchestratorWatchRunnerSnapshot(state, mode string, interval time.Duration, startedAt, stoppedAt, lastError string) error {
	status := h.persistedPublicJobOrchestratorStatusSnapshot()
	status.WatchRunnerState = strings.TrimSpace(state)
	status.WatchRunnerMode = strings.TrimSpace(mode)
	status.WatchRunnerOwnerID = strings.TrimSpace(h.orchestratorInstanceID)
	if status.WatchRunnerState == "not_running" {
		status.WatchRunnerOwnerID = ""
	}
	if interval > 0 {
		status.WatchRunnerInterval = int(interval / time.Second)
	}
	if strings.TrimSpace(startedAt) != "" {
		status.WatchRunnerStartedAt = strings.TrimSpace(startedAt)
	}
	if strings.TrimSpace(stoppedAt) != "" {
		status.WatchRunnerStoppedAt = strings.TrimSpace(stoppedAt)
	}
	status.WatchRunnerLastError = strings.TrimSpace(lastError)
	status.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	return h.writePublicJobOrchestratorStatus(status)
}

func (h *Handler) AcquirePublicJobOrchestratorWatchLock(mode string) error {
	workspace, err := h.appFactoryWorkspacePath()
	if err != nil {
		return err
	}
	lock, err := h.loadPublicJobOrchestratorWatchLock(workspace)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	if publicJobOrchestratorWatchLockActive(lock) && strings.TrimSpace(lock.OwnerID) != strings.TrimSpace(h.orchestratorInstanceID) {
		return fmt.Errorf("orchestrator watch already held by %s until %s", lock.OwnerID, lock.ExpiresAt)
	}
	updated := publicJobOrchestratorWatchLock{
		SchemaVersion: "0.1.0",
		OwnerID:       strings.TrimSpace(h.orchestratorInstanceID),
		Mode:          strings.TrimSpace(mode),
		AcquiredAt:    now.Format(time.RFC3339),
		UpdatedAt:     now.Format(time.RFC3339),
		ExpiresAt:     now.Add(publicJobOrchestratorWatchLockWindow).Format(time.RFC3339),
	}
	if lock != nil && strings.TrimSpace(lock.OwnerID) == strings.TrimSpace(h.orchestratorInstanceID) && strings.TrimSpace(lock.AcquiredAt) != "" {
		updated.AcquiredAt = lock.AcquiredAt
	}
	return h.persistPublicJobOrchestratorWatchLock(updated)
}

func (h *Handler) RenewPublicJobOrchestratorWatchLock(mode string) error {
	workspace, err := h.appFactoryWorkspacePath()
	if err != nil {
		return err
	}
	lock, err := h.loadPublicJobOrchestratorWatchLock(workspace)
	if err != nil {
		return err
	}
	if lock == nil || strings.TrimSpace(lock.OwnerID) == "" {
		return h.AcquirePublicJobOrchestratorWatchLock(mode)
	}
	if strings.TrimSpace(lock.OwnerID) != strings.TrimSpace(h.orchestratorInstanceID) {
		return fmt.Errorf("cannot renew orchestrator watch held by %s", lock.OwnerID)
	}
	now := time.Now().UTC()
	lock.Mode = strings.TrimSpace(mode)
	lock.UpdatedAt = now.Format(time.RFC3339)
	lock.ExpiresAt = now.Add(publicJobOrchestratorWatchLockWindow).Format(time.RFC3339)
	return h.persistPublicJobOrchestratorWatchLock(*lock)
}

func (h *Handler) ReleasePublicJobOrchestratorWatchLock() error {
	workspace, err := h.appFactoryWorkspacePath()
	if err != nil {
		return err
	}
	lock, err := h.loadPublicJobOrchestratorWatchLock(workspace)
	if err != nil {
		return err
	}
	if lock == nil || strings.TrimSpace(lock.OwnerID) == "" {
		return nil
	}
	if strings.TrimSpace(lock.OwnerID) != strings.TrimSpace(h.orchestratorInstanceID) {
		return nil
	}
	path := publicJobOrchestratorWatchLockPath(workspace)
	return removePublicJobOrchestratorWatchLock(path)
}

func (h *Handler) UnlockPublicJobOrchestratorWatchLock(force bool) (PublicJobOrchestratorStatus, error) {
	workspace, err := h.appFactoryWorkspacePath()
	if err != nil {
		return h.defaultPublicJobOrchestratorStatus(), err
	}
	lock, err := h.loadPublicJobOrchestratorWatchLock(workspace)
	if err != nil {
		return h.defaultPublicJobOrchestratorStatus(), err
	}
	if lock == nil || strings.TrimSpace(lock.OwnerID) == "" {
		return h.LoadPublicJobOrchestratorStatus()
	}
	if publicJobOrchestratorWatchLockActive(lock) && strings.TrimSpace(lock.OwnerID) != strings.TrimSpace(h.orchestratorInstanceID) && !force {
		return PublicJobOrchestratorStatus{}, fmt.Errorf("orchestrator watch held by %s until %s; rerun with force to unlock", lock.OwnerID, lock.ExpiresAt)
	}
	if err := removePublicJobOrchestratorWatchLock(publicJobOrchestratorWatchLockPath(workspace)); err != nil {
		return h.defaultPublicJobOrchestratorStatus(), err
	}
	return h.LoadPublicJobOrchestratorStatus()
}

func (h *Handler) launchQueuedPublicJobExecution(jobID string) {
	h.orchestratorMu.Lock()
	if h.orchestratorActive[jobID] {
		h.orchestratorMu.Unlock()
		return
	}
	h.orchestratorActive[jobID] = true
	h.orchestratorMu.Unlock()

	h.asyncJobs.Add(1)
	go func() {
		defer h.asyncJobs.Done()
		defer func() {
			h.orchestratorMu.Lock()
			delete(h.orchestratorActive, jobID)
			h.orchestratorMu.Unlock()
		}()
		h.processQueuedPublicJobExecution(jobID)
	}()
}

func (h *Handler) processQueuedPublicJobExecution(jobID string) {
	workspace, err := h.appFactoryWorkspacePath()
	if err != nil {
		return
	}
	runsSvc, err := h.buildRunsControlPlane()
	if err != nil {
		return
	}
	record, err := h.claimPublicJobExecutionLease(workspace, jobID)
	if err != nil || record == nil {
		return
	}
	if record.Status == "completed" || record.Status == "failed" || record.Status == "cancelled" {
		return
	}
	latestRun, err := runsSvc.Get(context.Background(), record.RunID)
	if err == nil && isTerminalOrchestratorRunStatus(latestRun.Status) {
		record.Status = orchestratorExecutionStatusForRun(latestRun.Status)
		record.FinishedAt = finishedAtForRun(latestRun)
		record.LastError = strings.TrimSpace(latestRun.FailureSummary)
		_ = h.persistPublicJobExecutionRecord(*record)
		_, _ = h.loadNotifications()
		jobRecord, buildErr := h.buildPublicJobRecord(jobID)
		if buildErr == nil {
			_ = h.persistPublicJobRecord(jobRecord)
		}
		return
	}
	record.Status = "running"
	record.StartedAt = time.Now().UTC().Format(time.RFC3339)
	record.LastError = ""
	assignPublicJobExecutionLease(record, h.orchestratorInstanceID)
	if err := h.persistPublicJobExecutionRecord(*record); err != nil {
		return
	}

	terminalStatus := "failed"
	lastError := ""
	builderSvc, err := h.buildersControlPlane()
	if err == nil {
		timeout := executionTimeoutFromSeconds(record.TimeoutSeconds)
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		unregisterExecutionCancel := h.registerPublicJobExecutionCancel(jobID, cancel)
		defer func() {
			unregisterExecutionCancel()
			cancel()
		}()
		var recordMu sync.Mutex
		stopLeaseHeartbeat := h.startPublicJobExecutionLeaseHeartbeat(ctx, workspace, record, &recordMu)
		completedRun, executeErr := h.executePreparedRunLocally(ctx, runsSvc, builderSvc, record.RunID)
		stopLeaseHeartbeat()
		if executeErr != nil {
			lastError = executeErr.Error()
			latestRun, getErr := runsSvc.Get(context.Background(), record.RunID)
			if getErr == nil && isTerminalOrchestratorRunStatus(latestRun.Status) {
				terminalStatus = orchestratorExecutionStatusForRun(latestRun.Status)
				if strings.TrimSpace(latestRun.FailureSummary) != "" {
					lastError = strings.TrimSpace(latestRun.FailureSummary)
				}
			}
		} else {
			terminalStatus = orchestratorExecutionStatusForRun(completedRun.Status)
			if completedRun.Status == appruns.StatusFailed {
				lastError = strings.TrimSpace(completedRun.FailureSummary)
			}
		}
	}
	if err != nil && lastError == "" {
		lastError = err.Error()
	}
	record.Status = terminalStatus
	record.FinishedAt = time.Now().UTC().Format(time.RFC3339)
	record.LastError = lastError
	finalizePublicJobExecutionAttempt(record, terminalReasonForExecutionStatus(terminalStatus, lastError), time.Now().UTC())
	clearPublicJobExecutionLease(record)
	_ = h.persistPublicJobExecutionRecord(*record)
	_, _ = h.loadNotifications()
	jobRecord, buildErr := h.buildPublicJobRecord(jobID)
	if buildErr == nil {
		_ = h.persistPublicJobRecord(jobRecord)
	}
}

func (h *Handler) registerPublicJobExecutionCancel(jobID string, cancel context.CancelFunc) func() {
	if h == nil || strings.TrimSpace(jobID) == "" || cancel == nil {
		return func() {}
	}
	h.executionCancelMu.Lock()
	h.executionCancels[jobID] = cancel
	h.executionCancelMu.Unlock()
	var once sync.Once
	return func() {
		once.Do(func() {
			h.executionCancelMu.Lock()
			delete(h.executionCancels, jobID)
			h.executionCancelMu.Unlock()
		})
	}
}

func (h *Handler) cancelActivePublicJobExecution(jobID string) {
	if h == nil || strings.TrimSpace(jobID) == "" {
		return
	}
	h.executionCancelMu.Lock()
	cancel := h.executionCancels[jobID]
	h.executionCancelMu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func (h *Handler) startPublicJobExecutionLeaseHeartbeat(parent context.Context, workspace string, record *publicJobExecutionRecord, mu *sync.Mutex) func() {
	if record == nil || mu == nil || publicJobExecutionLeaseRenewInterval <= 0 {
		return func() {}
	}
	ctx, cancel := context.WithCancel(parent)
	done := make(chan struct{})
	ticker := time.NewTicker(publicJobExecutionLeaseRenewInterval)
	go func() {
		defer close(done)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				mu.Lock()
				if strings.TrimSpace(record.Status) != "running" || strings.TrimSpace(record.LeaseOwnerID) != strings.TrimSpace(h.orchestratorInstanceID) {
					mu.Unlock()
					continue
				}
				renewPublicJobExecutionLease(record, h.orchestratorInstanceID)
				snapshot := *record
				mu.Unlock()
				_ = h.persistPublicJobExecutionRecord(snapshot)
			}
		}
	}()
	var once sync.Once
	return func() {
		once.Do(func() {
			cancel()
			<-done
		})
	}
}

func (h *Handler) recoverRunningPublicJobExecution(workspace string, record publicJobExecutionRecord) {
	h.orchestratorMu.Lock()
	if h.orchestratorActive[record.JobID] {
		h.orchestratorMu.Unlock()
		return
	}
	h.orchestratorActive[record.JobID] = true
	h.orchestratorMu.Unlock()

	h.asyncJobs.Add(1)
	go func() {
		defer h.asyncJobs.Done()
		defer func() {
			h.orchestratorMu.Lock()
			delete(h.orchestratorActive, record.JobID)
			h.orchestratorMu.Unlock()
		}()
		reloadedRecord, claimErr := h.claimPublicJobExecutionLease(workspace, record.JobID)
		if claimErr != nil || reloadedRecord == nil {
			return
		}
		record = *reloadedRecord

		runsSvc, err := h.buildRunsControlPlane()
		if err != nil {
			return
		}
		runRecord, err := runsSvc.Get(context.Background(), record.RunID)
		if err != nil {
			record.Status = "failed"
			record.FinishedAt = time.Now().UTC().Format(time.RFC3339)
			record.LastError = err.Error()
			finalizePublicJobExecutionAttempt(&record, "recovery_failed", time.Now().UTC())
			clearPublicJobExecutionLease(&record)
			_ = h.persistPublicJobExecutionRecord(record)
			return
		}
		switch runRecord.Status {
		case appruns.StatusCompleted, appruns.StatusFailed, appruns.StatusCancelled:
			record.Status = orchestratorExecutionStatusForRun(runRecord.Status)
			record.FinishedAt = runRecord.FinishedAt.UTC().Format(time.RFC3339)
			record.LastError = strings.TrimSpace(runRecord.FailureSummary)
			finalizePublicJobExecutionAttempt(&record, terminalReasonForExecutionStatus(record.Status, record.LastError), time.Now().UTC())
			clearPublicJobExecutionLease(&record)
			_ = h.persistPublicJobExecutionRecord(record)
			_, _ = h.loadNotifications()
			jobRecord, buildErr := h.buildPublicJobRecord(record.JobID)
			if buildErr == nil {
				_ = h.persistPublicJobRecord(jobRecord)
			}
			return
		case appruns.StatusRunning:
			builderSvc, builderErr := h.buildersControlPlane()
			if builderErr == nil {
				_, err = runsSvc.Fail(context.Background(), record.RunID, appruns.FailureReport{
					Summary:            "orchestrator dispatcher stopped unexpectedly; resume required",
					RecoverySuggestion: "retry resume after the API orchestrator is running again",
					FailureSignatures:  []string{"orchestrator_dispatcher_lost"},
				})
				if err == nil {
					_ = builderSvc.ReleaseByRun(context.Background(), record.RunID, "orchestrator dispatcher lost")
				}
			}
			record.Status = "failed"
			record.FinishedAt = time.Now().UTC().Format(time.RFC3339)
			if err != nil {
				record.LastError = err.Error()
			} else {
				record.LastError = "orchestrator dispatcher stopped unexpectedly; resume required"
			}
			finalizePublicJobExecutionAttempt(&record, "dispatcher_lost", time.Now().UTC())
			clearPublicJobExecutionLease(&record)
			_ = h.persistPublicJobExecutionRecord(record)
			_, _ = h.loadNotifications()
			jobRecord, buildErr := h.buildPublicJobRecord(record.JobID)
			if buildErr == nil {
				_ = h.persistPublicJobRecord(jobRecord)
			}
			return
		default:
			return
		}
	}()
}

func (h *Handler) loadPublicJobExecutionRecord(workspace, jobID string) (*publicJobExecutionRecord, error) {
	data, err := os.ReadFile(publicJobExecutionPath(workspace, jobID))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read orchestrator record: %w", err)
	}
	var record publicJobExecutionRecord
	if err := json.Unmarshal(data, &record); err != nil {
		return nil, fmt.Errorf("decode orchestrator record: %w", err)
	}
	return &record, nil
}

func (h *Handler) persistPublicJobExecutionRecord(record publicJobExecutionRecord) error {
	workspace, err := h.appFactoryWorkspacePath()
	if err != nil {
		return fmt.Errorf("init workspace: %w", err)
	}
	previous, err := h.loadPublicJobExecutionRecord(workspace, record.JobID)
	if err != nil {
		return err
	}
	if err := validatePublicJobExecutionRecord(previous, record); err != nil {
		return err
	}
	path := publicJobExecutionPath(workspace, record.JobID)
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal orchestrator record: %w", err)
	}
	data = append(data, '\n')
	if err := fileutil.WriteFileAtomic(path, data, 0o644); err != nil {
		return fmt.Errorf("write orchestrator record: %w", err)
	}
	return nil
}

func (h *Handler) loadPublicJobOrchestratorStatus(workspace string) (*PublicJobOrchestratorStatus, error) {
	data, err := os.ReadFile(publicJobOrchestratorStatusPath(workspace))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read orchestrator status: %w", err)
	}
	var status PublicJobOrchestratorStatus
	if err := json.Unmarshal(data, &status); err != nil {
		return nil, fmt.Errorf("decode orchestrator status: %w", err)
	}
	return &status, nil
}

func (h *Handler) persistPublicJobOrchestratorStatus(status PublicJobOrchestratorStatus) error {
	hydratePublicJobOrchestratorStatusWatchRunner(&status, h)
	workspace, err := h.appFactoryWorkspacePath()
	if err == nil {
		if watchLock, lockErr := h.loadPublicJobOrchestratorWatchLock(workspace); lockErr == nil {
			hydratePublicJobOrchestratorStatusWatchLock(&status, watchLock, h.orchestratorInstanceID)
		}
	}
	return h.writePublicJobOrchestratorStatus(status)
}

func (h *Handler) writePublicJobOrchestratorStatus(status PublicJobOrchestratorStatus) error {
	workspace, err := h.appFactoryWorkspacePath()
	if err != nil {
		return fmt.Errorf("init workspace: %w", err)
	}
	path := publicJobOrchestratorStatusPath(workspace)
	data, err := json.MarshalIndent(status, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal orchestrator status: %w", err)
	}
	data = append(data, '\n')
	if err := fileutil.WriteFileAtomic(path, data, 0o644); err != nil {
		return fmt.Errorf("write orchestrator status: %w", err)
	}
	return nil
}

func (h *Handler) loadPublicJobOrchestratorWatchLock(workspace string) (*publicJobOrchestratorWatchLock, error) {
	data, err := os.ReadFile(publicJobOrchestratorWatchLockPath(workspace))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read orchestrator watch lock: %w", err)
	}
	var lock publicJobOrchestratorWatchLock
	if err := json.Unmarshal(data, &lock); err != nil {
		return nil, fmt.Errorf("decode orchestrator watch lock: %w", err)
	}
	return &lock, nil
}

func (h *Handler) persistPublicJobOrchestratorWatchLock(lock publicJobOrchestratorWatchLock) error {
	workspace, err := h.appFactoryWorkspacePath()
	if err != nil {
		return fmt.Errorf("init workspace: %w", err)
	}
	path := publicJobOrchestratorWatchLockPath(workspace)
	data, err := json.MarshalIndent(lock, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal orchestrator watch lock: %w", err)
	}
	data = append(data, '\n')
	if err := fileutil.WriteFileAtomic(path, data, 0o644); err != nil {
		return fmt.Errorf("write orchestrator watch lock: %w", err)
	}
	return nil
}

func (h *Handler) loadPublicJobOrchestratorWatchAudits(workspace string) ([]publicJobOrchestratorWatchAuditRecord, error) {
	data, err := os.ReadFile(publicJobOrchestratorWatchAuditPath(workspace))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read orchestrator watch audits: %w", err)
	}
	lines := strings.Split(string(data), "\n")
	items := make([]publicJobOrchestratorWatchAuditRecord, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var record publicJobOrchestratorWatchAuditRecord
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			return nil, fmt.Errorf("decode orchestrator watch audit: %w", err)
		}
		items = append(items, record)
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].CreatedAt < items[j].CreatedAt
	})
	return items, nil
}

func (h *Handler) appendPublicJobOrchestratorWatchAudit(record publicJobOrchestratorWatchAuditRecord) error {
	workspace, err := h.appFactoryWorkspacePath()
	if err != nil {
		return fmt.Errorf("init workspace: %w", err)
	}
	path := publicJobOrchestratorWatchAuditPath(workspace)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("mkdir orchestrator watch audit dir: %w", err)
	}
	data, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("marshal orchestrator watch audit: %w", err)
	}
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open orchestrator watch audit: %w", err)
	}
	defer file.Close()
	if _, err := file.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("append orchestrator watch audit: %w", err)
	}
	return nil
}

func (h *Handler) claimPublicJobExecutionLease(workspace, jobID string) (*publicJobExecutionRecord, error) {
	record, err := h.loadPublicJobExecutionRecord(workspace, jobID)
	if err != nil || record == nil {
		return record, err
	}
	if record.Status == "completed" || record.Status == "failed" || record.Status == "cancelled" {
		return record, nil
	}
	if publicJobExecutionLeaseOwnedByAnotherLiveDispatcher(*record, h.orchestratorInstanceID) {
		return nil, nil
	}
	assignPublicJobExecutionLease(record, h.orchestratorInstanceID)
	if err := h.persistPublicJobExecutionRecord(*record); err != nil {
		return nil, err
	}
	return record, nil
}

func (h *Handler) listPublicJobExecutionRecords(workspace string) ([]publicJobExecutionRecord, error) {
	paths, err := filepath.Glob(filepath.Join(workspace, "appfactory", "orchestrator", "jobs", "*.json"))
	if err != nil {
		return nil, fmt.Errorf("glob orchestrator records: %w", err)
	}
	items := make([]publicJobExecutionRecord, 0, len(paths))
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read orchestrator record: %w", err)
		}
		var record publicJobExecutionRecord
		if err := json.Unmarshal(data, &record); err != nil {
			return nil, fmt.Errorf("decode orchestrator record: %w", err)
		}
		items = append(items, record)
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].RequestedAt < items[j].RequestedAt
	})
	return items, nil
}

func publicJobExecutionPath(workspace, jobID string) string {
	return filepath.Join(workspace, "appfactory", "orchestrator", "jobs", strings.TrimSpace(jobID)+".json")
}

func publicJobOrchestratorStatusPath(workspace string) string {
	return filepath.Join(workspace, "appfactory", "orchestrator", "status.json")
}

func publicJobOrchestratorWatchLockPath(workspace string) string {
	return filepath.Join(workspace, "appfactory", "orchestrator", "watch-lock.json")
}

func publicJobOrchestratorWatchAuditPath(workspace string) string {
	return filepath.Join(workspace, "appfactory", "orchestrator", "watch-events.jsonl")
}

func validatePublicJobExecutionRecord(previous *publicJobExecutionRecord, next publicJobExecutionRecord) error {
	if strings.TrimSpace(next.SchemaVersion) == "" {
		return fmt.Errorf("orchestrator execution schema_version is required")
	}
	if strings.TrimSpace(next.JobID) == "" {
		return fmt.Errorf("orchestrator execution job_id is required")
	}
	if strings.TrimSpace(next.RunID) == "" {
		return fmt.Errorf("orchestrator execution run_id is required")
	}
	if strings.TrimSpace(next.Action) == "" {
		return fmt.Errorf("orchestrator execution action is required")
	}
	if _, ok := publicJobExecutionAllowedStatuses[strings.TrimSpace(next.Status)]; !ok {
		return fmt.Errorf("orchestrator execution status %q is invalid", next.Status)
	}
	if strings.TrimSpace(next.RequestedAt) == "" {
		return fmt.Errorf("orchestrator execution requested_at is required")
	}
	if strings.TrimSpace(next.Status) == "running" && strings.TrimSpace(next.StartedAt) == "" {
		return fmt.Errorf("running orchestrator execution requires started_at")
	}
	if publicJobExecutionStatusIsTerminal(next.Status) && strings.TrimSpace(next.FinishedAt) == "" {
		return fmt.Errorf("terminal orchestrator execution requires finished_at")
	}
	if strings.TrimSpace(next.Status) == "queued" && strings.TrimSpace(next.FinishedAt) != "" {
		return fmt.Errorf("queued orchestrator execution cannot have finished_at")
	}
	if strings.TrimSpace(next.Status) == "queued" && strings.TrimSpace(next.StartedAt) != "" {
		return fmt.Errorf("queued orchestrator execution cannot have started_at")
	}
	if previous == nil {
		return nil
	}
	if strings.TrimSpace(previous.JobID) != "" && strings.TrimSpace(previous.JobID) != strings.TrimSpace(next.JobID) {
		return fmt.Errorf("orchestrator execution job_id mismatch: %q -> %q", previous.JobID, next.JobID)
	}
	previousRunID := strings.TrimSpace(previous.RunID)
	nextRunID := strings.TrimSpace(next.RunID)
	if previousRunID == nextRunID {
		return validatePublicJobExecutionSameRunTransition(*previous, next)
	}
	if !publicJobExecutionStatusIsTerminal(previous.Status) {
		return fmt.Errorf("cannot replace non-terminal orchestrator execution %q for job %q", previous.Status, next.JobID)
	}
	if strings.TrimSpace(next.Status) != "queued" {
		return fmt.Errorf("new orchestrator execution for job %q must start from queued, got %q", next.JobID, next.Status)
	}
	return nil
}

func validatePublicJobExecutionSameRunTransition(previous, next publicJobExecutionRecord) error {
	currentStatus := strings.TrimSpace(previous.Status)
	nextStatus := strings.TrimSpace(next.Status)
	allowedNextStatuses, ok := publicJobExecutionSameRunTransitions[currentStatus]
	if !ok {
		return fmt.Errorf("orchestrator execution status %q is invalid", previous.Status)
	}
	if _, ok := allowedNextStatuses[nextStatus]; !ok {
		return fmt.Errorf("illegal orchestrator execution transition for job %q run %q: %s -> %s", next.JobID, next.RunID, currentStatus, nextStatus)
	}
	return nil
}

func publicJobExecutionStatusIsTerminal(status string) bool {
	_, ok := publicJobExecutionTerminalStatuses[strings.TrimSpace(status)]
	return ok
}

func removePublicJobOrchestratorWatchLock(path string) error {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove orchestrator watch lock: %w", err)
	}
	return nil
}

func publicJobOrchestratorWatchLockActive(lock *publicJobOrchestratorWatchLock) bool {
	if lock == nil || strings.TrimSpace(lock.ExpiresAt) == "" {
		return false
	}
	expiresAt, err := parsePublicJobExecutionTimestamp(lock.ExpiresAt)
	if err != nil {
		return false
	}
	return expiresAt.After(time.Now().UTC())
}

func hydratePublicJobOrchestratorStatusWatchRunner(status *PublicJobOrchestratorStatus, h *Handler) {
	if status == nil || h == nil {
		return
	}
	h.orchestratorWatchMu.Lock()
	defer h.orchestratorWatchMu.Unlock()
	if h.orchestratorWatchRun {
		status.WatchRunnerState = "running"
		if h.orchestratorWatchStop {
			status.WatchRunnerState = "stopping"
		}
		status.WatchRunnerMode = strings.TrimSpace(h.orchestratorWatchMode)
		status.WatchRunnerOwnerID = strings.TrimSpace(h.orchestratorInstanceID)
		if h.orchestratorWatchEvery > 0 {
			status.WatchRunnerInterval = int(h.orchestratorWatchEvery / time.Second)
		}
		status.WatchRunnerStartedAt = strings.TrimSpace(h.orchestratorWatchStart)
		status.WatchRunnerStoppedAt = strings.TrimSpace(h.orchestratorWatchEnd)
		status.WatchRunnerLastError = strings.TrimSpace(h.orchestratorWatchError)
		return
	}
	if strings.TrimSpace(status.WatchRunnerState) == "" {
		status.WatchRunnerState = "not_running"
	}
	if h.orchestratorWatchEvery > 0 && status.WatchRunnerInterval == 0 {
		status.WatchRunnerInterval = int(h.orchestratorWatchEvery / time.Second)
	}
	if strings.TrimSpace(status.WatchRunnerStartedAt) == "" {
		status.WatchRunnerStartedAt = strings.TrimSpace(h.orchestratorWatchStart)
	}
	if strings.TrimSpace(status.WatchRunnerStoppedAt) == "" {
		status.WatchRunnerStoppedAt = strings.TrimSpace(h.orchestratorWatchEnd)
	}
	if strings.TrimSpace(status.WatchRunnerLastError) == "" {
		status.WatchRunnerLastError = strings.TrimSpace(h.orchestratorWatchError)
	}
}

func hydratePublicJobOrchestratorStatusWatchLock(status *PublicJobOrchestratorStatus, lock *publicJobOrchestratorWatchLock, instanceID string) {
	if status == nil {
		return
	}
	status.WatchLockState = "unlocked"
	status.WatchLockOwnerID = ""
	status.WatchLockAcquiredAt = ""
	status.WatchLockExpiresAt = ""
	if lock == nil || strings.TrimSpace(lock.OwnerID) == "" {
		return
	}
	status.WatchLockOwnerID = strings.TrimSpace(lock.OwnerID)
	status.WatchLockMode = strings.TrimSpace(lock.Mode)
	status.WatchLockAcquiredAt = strings.TrimSpace(lock.AcquiredAt)
	status.WatchLockUpdatedAt = strings.TrimSpace(lock.UpdatedAt)
	status.WatchLockExpiresAt = strings.TrimSpace(lock.ExpiresAt)
	if !publicJobOrchestratorWatchLockActive(lock) {
		status.WatchLockState = "stale"
		return
	}
	if strings.TrimSpace(lock.OwnerID) == strings.TrimSpace(instanceID) {
		status.WatchLockState = "held_by_self"
		return
	}
	status.WatchLockState = "held_by_other"
}

func orchestratorExecutionStatusForRun(status appruns.Status) string {
	switch status {
	case appruns.StatusCompleted:
		return "completed"
	case appruns.StatusCancelled:
		return "cancelled"
	case appruns.StatusFailed:
		return "failed"
	default:
		return string(status)
	}
}

func publicJobExecutionLeaseOwnedByAnotherLiveDispatcher(record publicJobExecutionRecord, instanceID string) bool {
	if !publicJobExecutionLeaseActive(record) {
		return false
	}
	ownerID := strings.TrimSpace(record.LeaseOwnerID)
	if ownerID == "" {
		return false
	}
	return ownerID != strings.TrimSpace(instanceID)
}

func publicJobExecutionLeaseActive(record publicJobExecutionRecord) bool {
	expiresAt := strings.TrimSpace(record.LeaseExpiresAt)
	if expiresAt == "" {
		return false
	}
	parsed, err := parsePublicJobExecutionTimestamp(expiresAt)
	if err != nil {
		return false
	}
	return parsed.After(time.Now().UTC())
}

func assignPublicJobExecutionLease(record *publicJobExecutionRecord, instanceID string) {
	now := time.Now().UTC()
	ownerID := strings.TrimSpace(instanceID)
	if ownerID == "" {
		return
	}
	attemptCount := record.AttemptCount
	if attemptCount < 0 {
		attemptCount = 0
	}
	if strings.TrimSpace(record.LeaseOwnerID) != ownerID || !publicJobExecutionLeaseActive(*record) {
		if activeAttempt := currentPublicJobExecutionAttempt(record); activeAttempt != nil && strings.TrimSpace(activeAttempt.ReleasedAt) == "" {
			releaseReason := "lease_reacquired"
			if strings.TrimSpace(activeAttempt.OwnerID) != ownerID {
				releaseReason = "handoff_after_lease_expiry"
			}
			finalizePublicJobExecutionAttempt(record, releaseReason, now)
		}
		record.AttemptCount = attemptCount + 1
		record.LeaseAcquiredAt = now.Format(time.RFC3339)
		appendPublicJobExecutionAttempt(record, record.AttemptCount, ownerID, now)
	} else if strings.TrimSpace(record.LeaseAcquiredAt) == "" {
		if attemptCount == 0 {
			record.AttemptCount = 1
		}
		record.LeaseAcquiredAt = now.Format(time.RFC3339)
		ensureCurrentPublicJobExecutionAttempt(record, record.AttemptCount, ownerID, now)
	} else if attemptCount == 0 {
		record.AttemptCount = 1
		ensureCurrentPublicJobExecutionAttempt(record, record.AttemptCount, ownerID, now)
	}
	record.LeaseOwnerID = ownerID
	record.LeaseExpiresAt = now.Add(publicJobExecutionLeaseWindow).Format(time.RFC3339)
	ensureCurrentPublicJobExecutionAttempt(record, record.AttemptCount, ownerID, publicJobExecutionClaimedAt(record, now))
}

func renewPublicJobExecutionLease(record *publicJobExecutionRecord, instanceID string) {
	ownerID := strings.TrimSpace(instanceID)
	if record == nil || ownerID == "" || strings.TrimSpace(record.LeaseOwnerID) != ownerID {
		return
	}
	now := time.Now().UTC()
	if strings.TrimSpace(record.LeaseAcquiredAt) == "" {
		record.LeaseAcquiredAt = now.Format(time.RFC3339)
	}
	record.LeaseExpiresAt = now.Add(publicJobExecutionLeaseWindow).Format(time.RFC3339)
	ensureCurrentPublicJobExecutionAttempt(record, record.AttemptCount, ownerID, publicJobExecutionClaimedAt(record, now))
	markPublicJobExecutionAttemptRenewed(record, ownerID, now)
}

func clearPublicJobExecutionLease(record *publicJobExecutionRecord) {
	record.LeaseOwnerID = ""
	record.LeaseAcquiredAt = ""
	record.LeaseExpiresAt = ""
}

func isTerminalOrchestratorRunStatus(status appruns.Status) bool {
	switch status {
	case appruns.StatusCompleted, appruns.StatusFailed, appruns.StatusCancelled:
		return true
	default:
		return false
	}
}

func finishedAtForRun(record appruns.RunRecord) string {
	if record.FinishedAt.IsZero() {
		return time.Now().UTC().Format(time.RFC3339)
	}
	return record.FinishedAt.UTC().Format(time.RFC3339)
}

func (h *Handler) cancelPublicJobExecution(jobID, reason string) error {
	workspace, err := h.appFactoryWorkspacePath()
	if err != nil {
		return fmt.Errorf("init workspace: %w", err)
	}
	record, err := h.loadPublicJobExecutionRecord(workspace, jobID)
	if err != nil {
		return err
	}
	if record == nil || record.Status == "completed" || record.Status == "failed" || record.Status == "cancelled" {
		return nil
	}
	record.Status = "cancelled"
	record.FinishedAt = time.Now().UTC().Format(time.RFC3339)
	record.LastError = strings.TrimSpace(reason)
	finalizePublicJobExecutionAttempt(record, "cancelled", time.Now().UTC())
	clearPublicJobExecutionLease(record)
	if err := h.persistPublicJobExecutionRecord(*record); err != nil {
		return err
	}
	h.cancelActivePublicJobExecution(jobID)
	return nil
}

func currentPublicJobExecutionAttempt(record *publicJobExecutionRecord) *publicJobExecutionAttempt {
	if record == nil || len(record.Attempts) == 0 {
		return nil
	}
	return &record.Attempts[len(record.Attempts)-1]
}

func appendPublicJobExecutionAttempt(record *publicJobExecutionRecord, attempt int, ownerID string, claimedAt time.Time) {
	if record == nil || attempt <= 0 || strings.TrimSpace(ownerID) == "" {
		return
	}
	record.Attempts = append(record.Attempts, publicJobExecutionAttempt{
		Attempt:       attempt,
		OwnerID:       strings.TrimSpace(ownerID),
		ClaimedAt:     claimedAt.Format(time.RFC3339),
		LastRenewedAt: claimedAt.Format(time.RFC3339),
	})
}

func ensureCurrentPublicJobExecutionAttempt(record *publicJobExecutionRecord, attempt int, ownerID string, claimedAt time.Time) {
	if record == nil || attempt <= 0 || strings.TrimSpace(ownerID) == "" {
		return
	}
	current := currentPublicJobExecutionAttempt(record)
	if current == nil || current.Attempt != attempt || strings.TrimSpace(current.OwnerID) != strings.TrimSpace(ownerID) {
		appendPublicJobExecutionAttempt(record, attempt, ownerID, claimedAt)
		return
	}
	if strings.TrimSpace(current.ClaimedAt) == "" {
		current.ClaimedAt = claimedAt.Format(time.RFC3339)
	}
	if strings.TrimSpace(current.LastRenewedAt) == "" {
		current.LastRenewedAt = claimedAt.Format(time.RFC3339)
	}
}

func publicJobExecutionClaimedAt(record *publicJobExecutionRecord, fallback time.Time) time.Time {
	if record == nil {
		return fallback
	}
	claimedAt, err := parsePublicJobExecutionTimestamp(strings.TrimSpace(record.LeaseAcquiredAt))
	if err == nil {
		return claimedAt.UTC()
	}
	return fallback
}

func markPublicJobExecutionAttemptRenewed(record *publicJobExecutionRecord, ownerID string, renewedAt time.Time) {
	current := currentPublicJobExecutionAttempt(record)
	if current == nil || strings.TrimSpace(current.OwnerID) != strings.TrimSpace(ownerID) || strings.TrimSpace(current.ReleasedAt) != "" {
		return
	}
	current.LastRenewedAt = renewedAt.Format(time.RFC3339)
}

func finalizePublicJobExecutionAttempt(record *publicJobExecutionRecord, reason string, releasedAt time.Time) {
	current := currentPublicJobExecutionAttempt(record)
	if current == nil || strings.TrimSpace(current.ReleasedAt) != "" {
		return
	}
	current.ReleasedAt = releasedAt.Format(time.RFC3339)
	current.ReleaseReason = strings.TrimSpace(reason)
	if strings.TrimSpace(current.LastRenewedAt) == "" {
		current.LastRenewedAt = current.ClaimedAt
	}
}

func terminalReasonForExecutionStatus(status, lastError string) string {
	switch strings.TrimSpace(status) {
	case "completed":
		return "completed"
	case "cancelled":
		return "cancelled"
	case "failed":
		if strings.Contains(strings.TrimSpace(lastError), "orchestrator dispatcher stopped unexpectedly") {
			return "dispatcher_lost"
		}
		return "failed"
	default:
		return strings.TrimSpace(status)
	}
}

func parsePublicJobExecutionTimestamp(value string) (time.Time, error) {
	parsed, err := time.Parse(time.RFC3339, value)
	if err == nil {
		return parsed, nil
	}
	return time.Parse(time.RFC3339Nano, value)
}

func syntheticPublicJobExecutionEvents(record *publicJobExecutionRecord) []publicJobEvent {
	if record == nil {
		return nil
	}
	items := []publicJobEvent{{
		At:      record.RequestedAt,
		Type:    "execution_enqueued",
		JobID:   record.JobID,
		RunID:   record.RunID,
		Status:  record.Status,
		Summary: "orchestrator accepted " + record.Action + " request",
	}}
	items = append(items, syntheticPublicJobExecutionAttemptEvents(record)...)
	if strings.TrimSpace(record.StartedAt) != "" {
		items = append(items, publicJobEvent{
			At:      record.StartedAt,
			Type:    "execution_started",
			JobID:   record.JobID,
			RunID:   record.RunID,
			Stage:   "orchestrator",
			Status:  "running",
			Summary: "orchestrator started " + record.Action + " dispatch",
		})
	}
	if strings.TrimSpace(record.FinishedAt) != "" {
		summary := "orchestrator finished " + record.Action + " dispatch"
		if strings.TrimSpace(record.LastError) != "" {
			summary = record.LastError
		}
		items = append(items, publicJobEvent{
			At:      record.FinishedAt,
			Type:    "execution_finished",
			JobID:   record.JobID,
			RunID:   record.RunID,
			Stage:   "orchestrator",
			Status:  record.Status,
			Summary: summary,
		})
	}
	return items
}

func syntheticPublicJobExecutionAttemptEvents(record *publicJobExecutionRecord) []publicJobEvent {
	if record == nil || len(record.Attempts) == 0 {
		return nil
	}
	items := make([]publicJobEvent, 0, len(record.Attempts)*3)
	for index, attempt := range record.Attempts {
		ownerID := strings.TrimSpace(attempt.OwnerID)
		if strings.TrimSpace(attempt.ClaimedAt) != "" {
			items = append(items, publicJobEvent{
				At:      attempt.ClaimedAt,
				Type:    "execution_attempt_claimed",
				JobID:   record.JobID,
				RunID:   record.RunID,
				Stage:   "orchestrator",
				Status:  "claimed",
				Summary: fmt.Sprintf("orchestrator attempt %d claimed by %s", attempt.Attempt, ownerID),
			})
		}
		if strings.TrimSpace(attempt.ReleasedAt) != "" {
			releaseReason := strings.TrimSpace(attempt.ReleaseReason)
			summary := fmt.Sprintf("orchestrator attempt %d released by %s", attempt.Attempt, ownerID)
			if releaseReason != "" {
				summary = fmt.Sprintf("orchestrator attempt %d released by %s (%s)", attempt.Attempt, ownerID, releaseReason)
			}
			items = append(items, publicJobEvent{
				At:      attempt.ReleasedAt,
				Type:    "execution_attempt_released",
				JobID:   record.JobID,
				RunID:   record.RunID,
				Stage:   "orchestrator",
				Status:  releaseReason,
				Summary: summary,
			})
			if eventType, eventSummary := executionAttemptReleaseEventDescriptor(attempt, record); eventType != "" {
				items = append(items, publicJobEvent{
					At:      attempt.ReleasedAt,
					Type:    eventType,
					JobID:   record.JobID,
					RunID:   record.RunID,
					Stage:   "orchestrator",
					Status:  releaseReason,
					Summary: eventSummary,
				})
			}
			if releaseReason == "handoff_after_lease_expiry" && index+1 < len(record.Attempts) {
				nextOwnerID := strings.TrimSpace(record.Attempts[index+1].OwnerID)
				if nextOwnerID != "" {
					items = append(items, publicJobEvent{
						At:      attempt.ReleasedAt,
						Type:    "execution_handoff",
						JobID:   record.JobID,
						RunID:   record.RunID,
						Stage:   "orchestrator",
						Status:  releaseReason,
						Summary: fmt.Sprintf("orchestrator handed execution from %s to %s after lease expiry", ownerID, nextOwnerID),
					})
				}
			}
		}
	}
	return items
}

func executionAttemptReleaseEventDescriptor(attempt publicJobExecutionAttempt, record *publicJobExecutionRecord) (string, string) {
	ownerID := strings.TrimSpace(attempt.OwnerID)
	switch strings.TrimSpace(attempt.ReleaseReason) {
	case "completed":
		return "execution_completed", fmt.Sprintf("orchestrator attempt %d completed under %s", attempt.Attempt, ownerID)
	case "failed":
		return "execution_failed", fmt.Sprintf("orchestrator attempt %d failed under %s", attempt.Attempt, ownerID)
	case "cancelled":
		return "execution_cancelled", fmt.Sprintf("orchestrator attempt %d cancelled under %s", attempt.Attempt, ownerID)
	case "dispatcher_lost":
		return "execution_interrupted", fmt.Sprintf("orchestrator attempt %d lost dispatcher ownership under %s", attempt.Attempt, ownerID)
	case "recovery_failed":
		return "execution_recovery_failed", fmt.Sprintf("orchestrator attempt %d failed during recovery under %s", attempt.Attempt, ownerID)
	case "lease_reacquired":
		return "execution_lease_reacquired", fmt.Sprintf("orchestrator attempt %d reacquired lease under %s", attempt.Attempt, ownerID)
	}
	return "", ""
}
