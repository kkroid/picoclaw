package adapter

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"

	appruns "github.com/sipeed/picoclaw/pkg/appfactory/runs"
	appconfig "github.com/sipeed/picoclaw/pkg/config"
	"github.com/sipeed/picoclaw/pkg/providers"
	"gopkg.in/yaml.v3"
)

type BuilderRuntimePatchGenerator interface {
	GeneratePatch(ctx context.Context, request BuilderRuntimePatchRequest) (BuilderRuntimePatchResponse, error)
}

type BuilderRuntimePatchRequest struct {
	ModelAliases   []string
	Run            runRecord
	RoundInput     appruns.RoundInput
	Route          appruns.BuilderRuntimeTaskRoute
	RepairOnly     bool
	PreviousErr    string
	PreviousBody   string
	FailureContext string
}

type BuilderRuntimePatchResponse struct {
	ModelAlias       string
	Content          string
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
}

var builderRuntimePatchRequestTimeout = 5 * time.Minute
var builderRuntimePatchProgressHeartbeatInterval = 30 * time.Second
var builderRuntimeTransientModelRequestRetryDelay = 30 * time.Second

const (
	builderRuntimeOverviewSurfaceRef   = "surface-overview"
	builderRuntimeCollectionSurfaceRef = "surface-collection"
	builderRuntimeMutationSurfaceRef   = "surface-mutation"
	builderRuntimeInspectionSurfaceRef = "surface-inspection"
)

type defaultBuilderRuntimePatchGenerator struct {
	modelCatalog *appconfig.Config
}

type builderRuntimeExecutionResult struct {
	Patch       *appruns.WorkspacePatch
	ApplyResult appruns.WorkspacePatchApplyResult
	AppliedRoundInput *appruns.RoundInput
	Stats       *appruns.BuilderRuntimeExecutionStats
}

type builderRuntimeExecutionError struct {
	summary            string
	recoverySuggestion string
	signature          string
	policy             repairFailurePolicy
	preserveWorkspace  bool
	resumeAllowed      bool
	state              *appruns.RoundState
	stats              *appruns.BuilderRuntimeExecutionStats
	wrapped            error
}

func (err *builderRuntimeExecutionError) Error() string {
	if err == nil {
		return ""
	}
	if err.wrapped != nil {
		return err.wrapped.Error()
	}
	return err.summary
}

func (err *builderRuntimeExecutionError) Unwrap() error {
	if err == nil {
		return nil
	}
	return err.wrapped
}

func (generator defaultBuilderRuntimePatchGenerator) GeneratePatch(ctx context.Context, request BuilderRuntimePatchRequest) (BuilderRuntimePatchResponse, error) {
	if generator.modelCatalog == nil {
		return BuilderRuntimePatchResponse{}, fmt.Errorf("builder runtime model catalog is not configured")
	}
	prompt, err := buildBuilderRuntimePrompt(request)
	if err != nil {
		return BuilderRuntimePatchResponse{}, err
	}
	messages := []providers.Message{{
		Role:    "system",
		Content: "You are PicoClaw builder-runtime patch generator. Output only compact JSON. No markdown. Stay inside the allowed path set. Before sending, verify the content is valid JSON with all arrays and objects closed.",
	}, {
		Role:    "user",
		Content: prompt,
	}}
	var attemptErrs []string
	for _, alias := range request.ModelAliases {
		trimmedAlias := strings.TrimSpace(alias)
		if trimmedAlias == "" {
			continue
		}
		modelCfg, err := generator.modelCatalog.GetModelConfig(trimmedAlias)
		if err != nil {
			attemptErrs = append(attemptErrs, fmt.Sprintf("%s: %v", trimmedAlias, err))
			continue
		}
		provider, modelID, err := providers.CreateProviderFromConfig(modelCfg)
		if err != nil {
			attemptErrs = append(attemptErrs, fmt.Sprintf("%s: %v", trimmedAlias, err))
			continue
		}
		response, err := provider.Chat(ctx, messages, nil, modelID, map[string]any{"temperature": 0})
		if err != nil {
			attemptErrs = append(attemptErrs, fmt.Sprintf("%s: %v", trimmedAlias, err))
			continue
		}
		result := BuilderRuntimePatchResponse{ModelAlias: trimmedAlias, Content: response.Content}
		if response.Usage != nil {
			result.PromptTokens = response.Usage.PromptTokens
			result.CompletionTokens = response.Usage.CompletionTokens
			result.TotalTokens = response.Usage.TotalTokens
		}
		return result, nil
	}
	if len(attemptErrs) == 0 {
		return BuilderRuntimePatchResponse{}, fmt.Errorf("builder runtime has no model aliases to try")
	}
	return BuilderRuntimePatchResponse{}, fmt.Errorf("builder runtime model request failed: %s", strings.Join(attemptErrs, "; "))
}

func (runner *Runner) builderRuntimePatchGenerator() BuilderRuntimePatchGenerator {
	if runner.PatchGenerator != nil {
		return runner.PatchGenerator
	}
	if runner.ModelCatalog == nil {
		return nil
	}
	return defaultBuilderRuntimePatchGenerator{modelCatalog: runner.ModelCatalog}
}

func (runner *Runner) shouldUseBuilderRuntime(run runRecord) bool {
	if run.BuilderRuntime == nil || !run.BuilderRuntime.Enabled {
		return false
	}
	if runner.builderRuntimePatchGenerator() == nil {
		return false
	}
	route, _ := selectBuilderRuntimeRoute(run)
	return route.Model.Primary != "" || route.RouteSource == "route_hint"
}

func (runner *Runner) executeBuilderRuntimeEditWithFailureContext(ctx context.Context, backend RunnerBackend, runID string, step ExecutionStep, run runRecord, roundInput appruns.RoundInput, failureContext string) (*builderRuntimeExecutionResult, error) {
	route, upgradedAtStart := selectBuilderRuntimeRoute(run)
	stats := &appruns.BuilderRuntimeExecutionStats{
		Mode:        "model",
		TaskType:    route.TaskType,
		RouteSource: route.RouteSource,
	}
	result := &builderRuntimeExecutionResult{AppliedRoundInput: cloneRoundInputValue(roundInput), Stats: stats}

	// M3.4: route_hint == deterministic 时尝试确定性 Emitter 路径，跳过 LLM。
	if route.RouteSource == "route_hint" && route.Model.Primary == "" {
		emitResult, emitErr := tryDeterministicEmit(run, roundInput, roundInput.RoundID+"-deterministic-emit")
		if emitErr != nil {
			stats.FailureReason = emitErr.Error()
			return result, &builderRuntimeExecutionError{
				summary:            fmt.Sprintf("deterministic emit failed: %v", emitErr),
				recoverySuggestion: "inspect prepare/domain-model.json and emitter configuration",
				signature:          "deterministic_emit_failed",
				preserveWorkspace:  false,
				resumeAllowed:      false,
				policy:             repairFailurePolicy{PreserveWorkspace: false, ResumeAllowed: false, RequiresHumanReview: true, MaxRounds: 1, UsedRounds: 1, RemainingRounds: 0, TerminationReason: "deterministic emit failed"},
				stats:              stats,
				wrapped:            emitErr,
			}
		}
		if emitResult.Handled {
			stats.Mode = "deterministic_emit"
			result.Patch = emitResult.Patch
			result.ApplyResult = emitResult.ApplyResult
			return result, nil
		}
		// emitter 不适用，fallthrough 到常规 model 路径
	}

	if route.Model.Primary == "" {
		stats.FailureReason = "builder runtime route has no model"
		return result, &builderRuntimeExecutionError{
			summary:            "builder runtime route has no model",
			recoverySuggestion: "configure default_model, upgrade_model, or task_routes before enabling builder runtime execution",
			signature:          "builder_runtime_unconfigured",
			preserveWorkspace:  true,
			resumeAllowed:      false,
			policy:             repairFailurePolicy{PreserveWorkspace: true, ResumeAllowed: false, RequiresHumanReview: true, MaxRounds: 1, UsedRounds: 1, RemainingRounds: 0, TerminationReason: "builder runtime route is unconfigured"},
			stats:              stats,
		}
	}
	stats.UpgradeApplied = upgradedAtStart
	prePatch, preApplyResult, preApplyErr := runner.applyBuilderRuntimeDeterministicEdits(run, roundInput)
	if preApplyErr != nil {
		stats.FailureReason = preApplyErr.Error()
		return result, &builderRuntimeExecutionError{
			summary:            fmt.Sprintf("builder runtime deterministic edit failed: %v", preApplyErr),
			recoverySuggestion: "inspect generic branding pre-projection inputs and workspace permissions before rerun",
			signature:          "workspace_patch_apply_failed",
			preserveWorkspace:  false,
			resumeAllowed:      false,
			policy:             repairFailurePolicy{PreserveWorkspace: false, ResumeAllowed: false, RequiresHumanReview: true, MaxRounds: 1, UsedRounds: 1, RemainingRounds: 0, TerminationReason: "builder runtime deterministic edit failed"},
			stats:              stats,
			wrapped:            preApplyErr,
		}
	}
	if prePatch != nil {
		result.Patch = prePatch
		result.ApplyResult = preApplyResult
	}
	request := BuilderRuntimePatchRequest{Run: run, RoundInput: roundInput, Route: route, ModelAliases: modelAliasesFromRef(route.Model), FailureContext: failureContext}
	return runner.executeBuilderRuntimePatchRequest(ctx, backend, runID, step, roundInput, result, request)
}

func (runner *Runner) executeBuilderRuntimeEdit(ctx context.Context, backend RunnerBackend, runID string, step ExecutionStep, run runRecord, roundInput appruns.RoundInput) (*builderRuntimeExecutionResult, error) {
	return runner.executeBuilderRuntimeEditWithFailureContext(ctx, backend, runID, step, run, roundInput, "")
}

func (runner *Runner) generateBuilderRuntimePatch(ctx context.Context, request BuilderRuntimePatchRequest) (BuilderRuntimePatchResponse, error) {
	return runner.generateBuilderRuntimePatchWithProgress(ctx, request, nil)
}

type builderRuntimePatchProgressReporter func(alias string, elapsed time.Duration) error

type builderRuntimePatchAttemptResult struct {
	response BuilderRuntimePatchResponse
	err      error
}

func (runner *Runner) generateBuilderRuntimePatchWithProgress(ctx context.Context, request BuilderRuntimePatchRequest, reportProgress builderRuntimePatchProgressReporter) (BuilderRuntimePatchResponse, error) {
	generator := runner.builderRuntimePatchGenerator()
	if generator == nil {
		return BuilderRuntimePatchResponse{}, fmt.Errorf("builder runtime patch generator is not configured")
	}
	aliases := modelAliasesFromRef(appruns.BuilderRuntimeModelRef{
		Primary:   firstBuilderRuntimeAlias(request.ModelAliases),
		Fallbacks: append([]string(nil), remainingBuilderRuntimeAliases(request.ModelAliases)...),
	})
	if len(aliases) == 0 {
		aliases = append([]string(nil), request.ModelAliases...)
	}
	if len(aliases) == 0 {
		return runBuilderRuntimePatchAttempt(ctx, generator, request, firstBuilderRuntimeAlias(request.ModelAliases), reportProgress)
	}
	attemptErrs := make([]string, 0, len(aliases))
	var firstEmptyResponse *BuilderRuntimePatchResponse
	allAttemptsEmpty := true
	for _, alias := range aliases {
		if err := ctx.Err(); err != nil {
			return BuilderRuntimePatchResponse{}, err
		}
		response, err := runBuilderRuntimePatchAttempt(ctx, generator, builderRuntimeRequestForAlias(request, alias), alias, reportProgress)
		if err == nil {
			if strings.TrimSpace(response.ModelAlias) == "" {
				response.ModelAlias = alias
			}
			if strings.TrimSpace(response.Content) == "" {
				if firstEmptyResponse == nil {
					captured := response
					firstEmptyResponse = &captured
				}
				attemptErrs = append(attemptErrs, fmt.Sprintf("%s: builder runtime model returned empty patch content", alias))
				continue
			}
			return response, nil
		}
		allAttemptsEmpty = false
		attemptErrs = append(attemptErrs, fmt.Sprintf("%s: %v", alias, err))
	}
	if firstEmptyResponse != nil && allAttemptsEmpty {
		return *firstEmptyResponse, nil
	}
	if len(attemptErrs) == 0 {
		return BuilderRuntimePatchResponse{}, fmt.Errorf("builder runtime has no model aliases to try")
	}
	return BuilderRuntimePatchResponse{}, fmt.Errorf("builder runtime model request failed: %s", strings.Join(attemptErrs, "; "))
}

func effectiveBuilderRuntimePatchProgressHeartbeatInterval() time.Duration {
	interval := builderRuntimePatchProgressHeartbeatInterval
	if interval <= 0 {
		return 0
	}
	if builderRuntimePatchRequestTimeout > 0 && interval >= builderRuntimePatchRequestTimeout {
		interval = builderRuntimePatchRequestTimeout / 2
		if interval < time.Millisecond {
			interval = time.Millisecond
		}
	}
	return interval
}

func runBuilderRuntimePatchAttempt(
	ctx context.Context,
	generator BuilderRuntimePatchGenerator,
	request BuilderRuntimePatchRequest,
	alias string,
	reportProgress builderRuntimePatchProgressReporter,
) (BuilderRuntimePatchResponse, error) {
	requestCtx := ctx
	cancelTimeout := func() {}
	if builderRuntimePatchRequestTimeout > 0 {
		requestCtx, cancelTimeout = context.WithTimeout(ctx, builderRuntimePatchRequestTimeout)
	}
	defer cancelTimeout()
	interval := effectiveBuilderRuntimePatchProgressHeartbeatInterval()
	if reportProgress == nil || interval <= 0 {
		response, err := generator.GeneratePatch(requestCtx, request)
		return normalizeBuilderRuntimePatchAttemptResult(requestCtx, response, err)
	}
	progressCtx, cancelProgress := context.WithCancelCause(requestCtx)
	defer cancelProgress(nil)
	resultCh := make(chan builderRuntimePatchAttemptResult, 1)
	startedAt := time.Now()
	go func() {
		response, err := generator.GeneratePatch(progressCtx, request)
		resultCh <- builderRuntimePatchAttemptResult{response: response, err: err}
	}()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case result := <-resultCh:
			return normalizeBuilderRuntimePatchAttemptResult(progressCtx, result.response, result.err)
		case <-ticker.C:
			if err := reportProgress(strings.TrimSpace(alias), time.Since(startedAt)); err != nil {
				cancelProgress(err)
				<-resultCh
				return BuilderRuntimePatchResponse{}, err
			}
		}
	}
}

func normalizeBuilderRuntimePatchAttemptResult(ctx context.Context, response BuilderRuntimePatchResponse, err error) (BuilderRuntimePatchResponse, error) {
	if cause := context.Cause(ctx); cause != nil {
		if errors.Is(cause, context.DeadlineExceeded) {
			return BuilderRuntimePatchResponse{}, fmt.Errorf("builder runtime patch request timed out after %s: %w", builderRuntimePatchRequestTimeout, context.DeadlineExceeded)
		}
		return BuilderRuntimePatchResponse{}, cause
	}
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) || errors.Is(err, context.DeadlineExceeded) {
			return BuilderRuntimePatchResponse{}, fmt.Errorf("builder runtime patch request timed out after %s: %w", builderRuntimePatchRequestTimeout, context.DeadlineExceeded)
		}
		return BuilderRuntimePatchResponse{}, err
	}
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return BuilderRuntimePatchResponse{}, fmt.Errorf("builder runtime patch request timed out after %s: %w", builderRuntimePatchRequestTimeout, context.DeadlineExceeded)
	}
	return response, nil
}

func builderRuntimeRequestForAlias(request BuilderRuntimePatchRequest, alias string) BuilderRuntimePatchRequest {
	cloned := request
	trimmed := strings.TrimSpace(alias)
	if trimmed == "" {
		cloned.ModelAliases = nil
		return cloned
	}
	cloned.ModelAliases = []string{trimmed}
	return cloned
}

func firstBuilderRuntimeAlias(aliases []string) string {
	for _, alias := range aliases {
		if trimmed := strings.TrimSpace(alias); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func remainingBuilderRuntimeAliases(aliases []string) []string {
	remaining := make([]string, 0, maxInt(0, len(aliases)-1))
	seenFirst := false
	for _, alias := range aliases {
		trimmed := strings.TrimSpace(alias)
		if trimmed == "" {
			continue
		}
		if !seenFirst {
			seenFirst = true
			continue
		}
		remaining = append(remaining, trimmed)
	}
	return remaining
}

func (runner *Runner) executeBuilderRuntimePatchRequest(ctx context.Context, backend RunnerBackend, runID string, step ExecutionStep, roundInput appruns.RoundInput, result *builderRuntimeExecutionResult, request BuilderRuntimePatchRequest) (*builderRuntimeExecutionResult, error) {
	route := request.Route
	stats := result.Stats
	run := request.Run
	if stats == nil {
		stats = &appruns.BuilderRuntimeExecutionStats{}
		result.Stats = stats
	}
	routeTargetPaths := builderRuntimeRouteTargetPaths(run.TaskBundle, route.TaskID)
	roundState := builderRuntimeRoundState(run.RoundState, step, route.TaskID, "")
	if err := runner.reportPatchGenerationStarted(ctx, backend, runID, step, roundInput, roundState, routeTargetPaths); err != nil {
		return result, err
	}
	var (
		response     BuilderRuntimePatchResponse
		patch        appruns.WorkspacePatch
		normalized   bool
		driftCount   int
		parseErr     error
		parseRetries int
		err          error
	)
	for {
		var attemptsUsed int
		response, attemptsUsed, err = runner.generateBuilderRuntimePatchWithProgressAndTransientRetry(ctx, request, func(alias string, elapsed time.Duration) error {
			return runner.reportPatchGenerationWaiting(ctx, backend, runID, step, roundInput, roundState, routeTargetPaths, alias, elapsed)
		})
		stats.Attempts += attemptsUsed
		if err != nil {
			stats.FailureReason = err.Error()
			if reportErr := runner.reportPatchGenerationFailed(ctx, backend, runID, step, roundInput, roundState, routeTargetPaths, err); reportErr != nil {
				return result, reportErr
			}
			return result, &builderRuntimeExecutionError{
				summary:            fmt.Sprintf("builder runtime model request failed: %v", err),
				recoverySuggestion: "fix model connectivity or route configuration before rerun",
				signature:          "builder_runtime_model_request_failed",
				preserveWorkspace:  true,
				resumeAllowed:      true,
				policy:             repairFailurePolicy{PreserveWorkspace: true, ResumeAllowed: true, RequiresHumanReview: true, MaxRounds: 1, UsedRounds: 1, RemainingRounds: 0, TerminationReason: "builder runtime model request failed"},
				stats:              stats,
				wrapped:            err,
			}
		}
		stats.SelectedModel = response.ModelAlias
		stats.ModelSequence = append(stats.ModelSequence, response.ModelAlias)
		stats.PromptTokens += response.PromptTokens
		stats.CompletionTokens += response.CompletionTokens
		stats.TotalTokens += response.TotalTokens
		appendBuilderRuntimeLog(run.LogPath, response.ModelAlias, response.Content)
		patch, normalized, driftCount, parseErr = normalizeBuilderRuntimePatchWithWorkspace(run.WorkspacePath, response.Content, roundInput.RoundID, routeTargetPaths)
		normalizeBuilderRuntimePatchForWorkspaceWithTaskBundleAndTargetPaths(run.WorkspacePath, roundInput.TaskBundle, routeTargetPaths, &patch)
		stats.SchemaNormalized = normalized
		stats.SchemaDriftCount += driftCount
		if parseErr == nil {
			break
		}
		stats.ParseFailureCount++
		stats.FailureReason = parseErr.Error()
		if reportErr := runner.reportPatchGenerationFailed(ctx, backend, runID, step, roundInput, roundState, routeTargetPaths, parseErr); reportErr != nil {
			return result, reportErr
		}
		parseRetries++
		if shouldRetryBuilderRuntimeTransientPatchParseFailure(parseRetries, response.Content, parseErr) {
			if waitErr := waitBuilderRuntimeTransientModelRequestRetry(ctx); waitErr != nil {
				return result, waitErr
			}
			if err := runner.reportPatchGenerationStarted(ctx, backend, runID, step, roundInput, roundState, routeTargetPaths); err != nil {
				return result, err
			}
			continue
		}
		if shouldRetryBuilderRuntimeWithUpgrade(run, stats, "parse_failure") {
			return runner.retryBuilderRuntimeWithUpgrade(ctx, backend, runID, step, run, roundInput, stats, response.ModelAlias, response.Content, parseErr)
		}
		return runner.retryBuilderRuntimeSchemaRepair(ctx, backend, runID, step, run, roundInput, route, stats, response.ModelAlias, response.Content, parseErr)
	}
	targetPaths := routeTargetPaths
	scopePaths := builderRuntimeAllowedPaths(run, roundInput, route)
	if shouldEnforceBuilderRuntimeTargetScope(route.TaskType) {
		if scopeErr := validateBuilderRuntimeTargetScope(patch.Operations, scopePaths); scopeErr != nil {
			stats.ScopeViolationCount++
			stats.FailureReason = scopeErr.Error()
			if reportErr := runner.reportPatchGenerationFailed(ctx, backend, runID, step, roundInput, roundState, scopePaths, scopeErr); reportErr != nil {
				return result, reportErr
			}
			if shouldRetryBuilderRuntimeWithUpgrade(run, stats, "scope_violation") {
				return runner.retryBuilderRuntimeWithUpgrade(ctx, backend, runID, step, run, roundInput, stats, response.ModelAlias, response.Content, scopeErr)
			}
			return result, &builderRuntimeExecutionError{
				summary:            fmt.Sprintf("builder runtime patch exceeded repair target scope: %v", scopeErr),
				recoverySuggestion: "keep analyze/test repair edits inside the current repair allowed_paths and cover every directly failing target file",
				signature:          "builder_runtime_scope_violation",
				preserveWorkspace:  true,
				resumeAllowed:      false,
				policy:             repairFailurePolicy{PreserveWorkspace: true, ResumeAllowed: false, RequiresHumanReview: false, MaxRounds: 1, UsedRounds: stats.Attempts, RemainingRounds: 0, TerminationReason: "builder runtime repair patch exceeded target scope"},
				stats:              stats,
				wrapped:            scopeErr,
			}
		}
	}
	if semanticErr := validateBuilderRuntimeSemanticConsistency(run, patch.Operations, request.FailureContext); semanticErr != nil {
		stats.FailureReason = semanticErr.Error()
		if reportErr := runner.reportPatchGenerationFailed(ctx, backend, runID, step, roundInput, roundState, targetPaths, semanticErr); reportErr != nil {
			return result, reportErr
		}
		if shouldRetryBuilderRuntimeSemanticConflictRepair(route.TaskType, stats, response.ModelAlias, response.Content, semanticErr) {
			return runner.retryBuilderRuntimeSemanticConflictRepair(ctx, backend, runID, step, run, roundInput, route, stats, response.ModelAlias, response.Content, &patch, semanticErr, request.FailureContext)
		}
		if shouldRetryBuilderRuntimeWithUpgrade(run, stats, "semantic_conflict") {
			return runner.retryBuilderRuntimeWithUpgrade(ctx, backend, runID, step, run, roundInput, stats, response.ModelAlias, response.Content, semanticErr)
		}
		return result, &builderRuntimeExecutionError{
			summary:            fmt.Sprintf("builder runtime patch reintroduced schema-conflicting symbols: %v", semanticErr),
			recoverySuggestion: "align builder edits with the current model files and keep exported Dart model types plus removed generic fields consistent across dependents",
			signature:          "builder_runtime_semantic_conflict",
			preserveWorkspace:  true,
			resumeAllowed:      false,
			policy:             repairFailurePolicy{PreserveWorkspace: true, ResumeAllowed: false, RequiresHumanReview: false, MaxRounds: 1, UsedRounds: stats.Attempts, RemainingRounds: 0, TerminationReason: "builder runtime patch reintroduced schema-conflicting symbols"},
			stats:              stats,
			wrapped:            semanticErr,
		}
	}
	if coverageErr := validateBuilderRuntimeDirectFailureCoverage(route.TaskType, patch.Operations, targetPaths); coverageErr != nil {
		stats.FailureReason = coverageErr.Error()
		if reportErr := runner.reportPatchGenerationFailed(ctx, backend, runID, step, roundInput, roundState, targetPaths, coverageErr); reportErr != nil {
			return result, reportErr
		}
		if shouldRetryBuilderRuntimeDirectFailureCoverageRepair(route.TaskType, stats, response.ModelAlias, response.Content) {
			return runner.retryBuilderRuntimeDirectFailureCoverageRepair(ctx, backend, runID, step, run, roundInput, route, stats, response.ModelAlias, response.Content, &patch, coverageErr, request.FailureContext)
		}
		if shouldRetryBuilderRuntimeWithUpgrade(run, stats, "semantic_conflict") {
			return runner.retryBuilderRuntimeWithUpgrade(ctx, backend, runID, step, run, roundInput, stats, response.ModelAlias, response.Content, coverageErr)
		}
		return result, &builderRuntimeExecutionError{
			summary:            fmt.Sprintf("builder runtime patch left directly failing repair targets untouched: %v", coverageErr),
			recoverySuggestion: "for analyze/test repair, touch every directly failing target file in the current repair slice before finishing the patch",
			signature:          "builder_runtime_semantic_conflict",
			preserveWorkspace:  true,
			resumeAllowed:      false,
			policy:             repairFailurePolicy{PreserveWorkspace: true, ResumeAllowed: false, RequiresHumanReview: false, MaxRounds: 1, UsedRounds: stats.Attempts, RemainingRounds: 0, TerminationReason: "builder runtime patch left directly failing repair targets untouched"},
			stats:              stats,
			wrapped:            coverageErr,
		}
	}
	if taskCoverageErr := validateBuilderRuntimeTaskTargetCoverage(route.TaskType, mergeBuilderRuntimePatchOperations(result.Patch, patch.Operations), targetPaths); taskCoverageErr != nil {
		stats.FailureReason = taskCoverageErr.Error()
		if reportErr := runner.reportPatchGenerationFailed(ctx, backend, runID, step, roundInput, roundState, targetPaths, taskCoverageErr); reportErr != nil {
			return result, reportErr
		}
		if shouldRetryBuilderRuntimeTaskTargetCoverageRepair(route.TaskType, stats, response.ModelAlias, response.Content) {
			return runner.retryBuilderRuntimeTaskTargetCoverageRepair(ctx, backend, runID, step, run, roundInput, route, stats, response.ModelAlias, response.Content, result.Patch, &patch, taskCoverageErr)
		}
		if shouldRetryBuilderRuntimeWithUpgrade(run, stats, "validation_failure") {
			return runner.retryBuilderRuntimeWithUpgrade(ctx, backend, runID, step, run, roundInput, stats, response.ModelAlias, response.Content, taskCoverageErr)
		}
		return result, &builderRuntimeExecutionError{
			summary:            fmt.Sprintf("builder runtime patch left current task target paths untouched: %v", taskCoverageErr),
			recoverySuggestion: "for non-repair builder tasks, touch every concrete task target path in the current patch before moving on",
			signature:          "builder_runtime_task_output_invalid",
			preserveWorkspace:  true,
			resumeAllowed:      false,
			policy:             repairFailurePolicy{PreserveWorkspace: true, ResumeAllowed: false, RequiresHumanReview: false, MaxRounds: 1, UsedRounds: stats.Attempts, RemainingRounds: 0, TerminationReason: "builder runtime patch left current task target paths untouched"},
			stats:              stats,
			wrapped:            taskCoverageErr,
		}
	}
	updateBuilderRuntimeOperationStats(stats, patch.Operations, targetPaths)
	if shouldRetryBuilderRuntimeWithUpgrade(run, stats, "schema_drift") {
		return runner.retryBuilderRuntimeWithUpgrade(ctx, backend, runID, step, run, roundInput, stats, response.ModelAlias, response.Content, fmt.Errorf("builder runtime schema drift count %d reached upgrade threshold", stats.SchemaDriftCount))
	}
	if shouldRetryBuilderRuntimeWithUpgrade(run, stats, "unrelated_edits") {
		return runner.retryBuilderRuntimeWithUpgrade(ctx, backend, runID, step, run, roundInput, stats, response.ModelAlias, response.Content, fmt.Errorf("builder runtime unrelated operation rate %.2f reached upgrade threshold", stats.UnrelatedOperationRate))
	}
	if err := runner.reportGeneratedPatch(ctx, backend, runID, step, roundInput, roundState, &patch); err != nil {
		return result, err
	}
	preApplySnapshot, snapshotErr := captureBuilderRuntimeWorkspacePatchSnapshot(run.WorkspacePath, &patch)
	if snapshotErr != nil {
		return result, fmt.Errorf("capture builder runtime workspace before patch apply: %w", snapshotErr)
	}
	applyResult, applyErr := appruns.ApplyWorkspacePatch(run.WorkspacePath, run.AllowedPaths, run.ProtectedPaths, patch)
	if applyErr != nil {
		if strings.Contains(applyErr.Error(), "outside allowed roots") || strings.Contains(applyErr.Error(), "protected") {
			stats.ScopeViolationCount++
		}
		stats.FailureReason = applyErr.Error()
		if reportErr := runner.reportPatchApplyFailed(ctx, backend, runID, step, roundInput, roundState, workspacePatchPaths(&patch), applyErr); reportErr != nil {
			return result, reportErr
		}
		if shouldRetryBuilderRuntimePatchApplyRepair(applyErr) && stats.Attempts < 2 {
			return runner.retryBuilderRuntimePatchApplyRepair(ctx, backend, runID, step, run, roundInput, route, stats, response.ModelAlias, response.Content, applyErr)
		}
		if shouldRetryBuilderRuntimeWithUpgrade(run, stats, "scope_violation") {
			return runner.retryBuilderRuntimeWithUpgrade(ctx, backend, runID, step, run, roundInput, stats, response.ModelAlias, response.Content, applyErr)
		}
		return result, &builderRuntimeExecutionError{
			summary:            fmt.Sprintf("builder runtime workspace patch apply failed: %v", applyErr),
			recoverySuggestion: "inspect generated patch, allowed paths, and target paths before rerun",
			signature:          "workspace_patch_apply_failed",
			preserveWorkspace:  false,
			resumeAllowed:      false,
			policy:             repairFailurePolicy{PreserveWorkspace: false, ResumeAllowed: false, RequiresHumanReview: true, MaxRounds: 1, UsedRounds: 1, RemainingRounds: 0, TerminationReason: "workspace patch apply failed"},
			stats:              stats,
			wrapped:            applyErr,
		}
	}
	if applyResult.Status != "" {
		patch.Status = applyResult.Status
	}
	if taskValidationErr := validateBuilderRuntimeTaskOutputs(run, route.TaskID); taskValidationErr != nil {
		if isBuilderRuntimeRepairableTaskOutputValidationFailure(taskValidationErr) {
			if restoreErr := restoreBuilderRuntimeWorkspacePatchSnapshot(run.WorkspacePath, preApplySnapshot, &patch); restoreErr != nil {
				return result, fmt.Errorf("restore builder runtime workspace after task output validation failure: %w", restoreErr)
			}
		}
		stats.FailureReason = taskValidationErr.Error()
		createdState := builderRuntimeRoundState(run.RoundState, step, route.TaskID, appruns.BuilderRuntimeTaskStatusCreated)
		if reportErr := runner.reportPatchApplyFailed(ctx, backend, runID, step, roundInput, createdState, workspacePatchPaths(&patch), taskValidationErr); reportErr != nil {
			return result, reportErr
		}
		if shouldRetryBuilderRuntimeWithUpgrade(run, stats, "validation_failure") {
			return runner.retryBuilderRuntimeWithUpgrade(ctx, backend, runID, step, run, roundInput, stats, response.ModelAlias, response.Content, taskValidationErr)
		}
		if shouldRetryBuilderRuntimeTaskOutputValidationRepair(route.TaskType, stats, response.ModelAlias, response.Content, taskValidationErr) {
			return runner.retryBuilderRuntimeTaskOutputValidationRepair(ctx, backend, runID, step, run, roundInput, route, stats, response.ModelAlias, response.Content, &patch, taskValidationErr, request.FailureContext)
		}
		return result, &builderRuntimeExecutionError{
			summary:            fmt.Sprintf("builder runtime task output validation failed: %v", taskValidationErr),
			recoverySuggestion: "keep the current task focused on one file and ensure every local Dart import/export/part resolves before moving to the next task",
			signature:          "builder_runtime_task_output_invalid",
			preserveWorkspace:  true,
			resumeAllowed:      false,
			state:              createdState,
			policy:             repairFailurePolicy{PreserveWorkspace: true, ResumeAllowed: false, RequiresHumanReview: false, MaxRounds: 1, UsedRounds: stats.Attempts, RemainingRounds: 0, TerminationReason: "builder runtime task output validation failed"},
			stats:              stats,
			wrapped:            taskValidationErr,
		}
	}
	patch.ModifiedFiles = modifiedFilePaths(applyResult.ModifiedFiles)
	result.Patch = mergeWorkspacePatches(result.Patch, &patch)
	result.ApplyResult = mergeWorkspacePatchApplyResults(result.ApplyResult, applyResult)
	return result, nil
}

func (runner *Runner) applyBuilderRuntimeDeterministicEdits(run runRecord, roundInput appruns.RoundInput) (*appruns.WorkspacePatch, appruns.WorkspacePatchApplyResult, error) {
	patch, err := buildGenericBrandingWorkspacePatch(run, roundInput.RoundID+"-generic-branding")
	if err != nil || patch == nil || len(patch.Operations) == 0 {
		return patch, appruns.WorkspacePatchApplyResult{}, err
	}
	applyResult, applyErr := appruns.ApplyWorkspacePatch(run.WorkspacePath, run.AllowedPaths, run.ProtectedPaths, *patch)
	if applyErr != nil {
		return patch, appruns.WorkspacePatchApplyResult{}, applyErr
	}
	if applyResult.Status != "" {
		patch.Status = applyResult.Status
	}
	patch.ModifiedFiles = modifiedFilePaths(applyResult.ModifiedFiles)
	return patch, applyResult, nil
}

func mergeWorkspacePatches(base, extra *appruns.WorkspacePatch) *appruns.WorkspacePatch {
	if base == nil {
		return extra
	}
	if extra == nil {
		return base
	}
	merged := *base
	if strings.TrimSpace(extra.PatchID) != "" {
		merged.PatchID = extra.PatchID
	}
	if strings.TrimSpace(extra.Status) != "" {
		merged.Status = extra.Status
	}
	merged.Operations = append(append([]appruns.WorkspacePatchOperation(nil), base.Operations...), extra.Operations...)
	merged.ModifiedFiles = append(append([]string(nil), base.ModifiedFiles...), extra.ModifiedFiles...)
	return &merged
}

func captureBuilderRuntimeWorkspacePatchSnapshot(workspacePath string, patch *appruns.WorkspacePatch) (workspaceSnapshot, error) {
	if patch == nil {
		return workspaceSnapshot{}, nil
	}
	return captureWorkspaceSnapshotForPaths(workspacePath, workspacePatchPaths(patch))
}

func restoreBuilderRuntimeWorkspacePatchSnapshot(workspacePath string, baseline workspaceSnapshot, patch *appruns.WorkspacePatch) error {
	if patch == nil {
		return nil
	}
	return restoreWorkspaceSnapshotForPaths(workspacePath, baseline, workspacePatchPaths(patch))
}

func isBuilderRuntimeDartSyntaxValidationFailure(err error) bool {
	return err != nil && strings.Contains(err.Error(), "failed Dart syntax validation")
}

func isBuilderRuntimeRepairableTaskOutputValidationFailure(err error) bool {
	if err == nil {
		return false
	}
	if isBuilderRuntimeDartSyntaxValidationFailure(err) {
		return true
	}
	return strings.Contains(err.Error(), "unresolved local import/export/part") ||
		strings.Contains(err.Error(), "counter-demo shell MyHomePage") ||
		strings.Contains(err.Error(), "nested MaterialApp") ||
		strings.Contains(err.Error(), "passes unsupported named parameters") ||
		strings.Contains(err.Error(), "omits required named parameters")
}

func mergeWorkspacePatchApplyResults(base, extra appruns.WorkspacePatchApplyResult) appruns.WorkspacePatchApplyResult {
	if base.PatchID == "" && base.Status == "" && len(base.ModifiedFiles) == 0 {
		return extra
	}
	merged := base
	if strings.TrimSpace(extra.PatchID) != "" {
		merged.PatchID = extra.PatchID
	}
	if strings.TrimSpace(extra.Status) != "" {
		merged.Status = extra.Status
	}
	merged.ModifiedFiles = append(append([]appruns.FileChange(nil), base.ModifiedFiles...), extra.ModifiedFiles...)
	return merged
}

func (runner *Runner) retryBuilderRuntimeWithUpgrade(ctx context.Context, backend RunnerBackend, runID string, step ExecutionStep, run runRecord, roundInput appruns.RoundInput, stats *appruns.BuilderRuntimeExecutionStats, previousModel string, previousBody string, previousErr error) (*builderRuntimeExecutionResult, error) {
	result := &builderRuntimeExecutionResult{AppliedRoundInput: cloneRoundInputValue(roundInput), Stats: stats}
	upgradeModel, ok := nextBuilderRuntimeUpgradeModel(run.BuilderRuntime, previousModel)
	if !ok {
		return result, previousErr
	}
	stats.UpgradeApplied = true
	selectedTask := preferredBuilderRuntimeTask(run.TaskBundle, run.RoundState)
	upgradeRoute := appruns.BuilderRuntimeTaskRoute{
		TaskID:      selectedTask.TaskID,
		TaskType:    selectedTask.EffectiveTaskType(),
		RouteSource: "upgrade_model",
		Model:       upgradeModel,
	}
	request := BuilderRuntimePatchRequest{
		Run:            run,
		RoundInput:     roundInput,
		Route:          upgradeRoute,
		ModelAliases:   modelAliasesFromRef(upgradeRoute.Model),
		PreviousErr:    strings.TrimSpace(previousErr.Error()),
		PreviousBody:   previousBody,
		FailureContext: strings.TrimSpace(previousErr.Error()),
	}
	routeTargetPaths := builderRuntimeRouteTargetPaths(run.TaskBundle, upgradeRoute.TaskID)
	roundState := builderRuntimeRoundState(run.RoundState, step, upgradeRoute.TaskID, "")
	if err := runner.reportPatchGenerationStarted(ctx, backend, runID, step, roundInput, roundState, routeTargetPaths); err != nil {
		return result, err
	}
	response, attemptsUsed, err := runner.generateBuilderRuntimePatchWithTransientRetry(ctx, request)
	stats.Attempts += attemptsUsed
	if err != nil {
		stats.FailureReason = err.Error()
		if reportErr := runner.reportPatchGenerationFailed(ctx, backend, runID, step, roundInput, roundState, routeTargetPaths, err); reportErr != nil {
			return result, reportErr
		}
		return result, &builderRuntimeExecutionError{
			summary:            fmt.Sprintf("builder runtime upgrade model request failed: %v", err),
			recoverySuggestion: "fix upgrade model connectivity or route configuration before rerun",
			signature:          "builder_runtime_model_request_failed",
			preserveWorkspace:  true,
			resumeAllowed:      true,
			policy:             repairFailurePolicy{PreserveWorkspace: true, ResumeAllowed: true, RequiresHumanReview: true, MaxRounds: 1, UsedRounds: 1, RemainingRounds: 0, TerminationReason: "builder runtime upgrade model request failed"},
			stats:              stats,
			wrapped:            err,
		}
	}
	stats.SelectedModel = response.ModelAlias
	stats.RouteSource = "upgrade_model"
	stats.ModelSequence = append(stats.ModelSequence, response.ModelAlias)
	stats.PromptTokens += response.PromptTokens
	stats.CompletionTokens += response.CompletionTokens
	stats.TotalTokens += response.TotalTokens
	appendBuilderRuntimeLog(run.LogPath, response.ModelAlias, response.Content)
	patch, normalized, driftCount, parseErr := normalizeBuilderRuntimePatchWithWorkspace(run.WorkspacePath, response.Content, roundInput.RoundID, routeTargetPaths)
	normalizeBuilderRuntimePatchForWorkspaceWithTaskBundleAndTargetPaths(run.WorkspacePath, roundInput.TaskBundle, routeTargetPaths, &patch)
	stats.SchemaNormalized = stats.SchemaNormalized || normalized
	stats.SchemaDriftCount += driftCount
	if parseErr != nil {
		stats.ParseFailureCount++
		stats.FailureReason = parseErr.Error()
		if reportErr := runner.reportPatchGenerationFailed(ctx, backend, runID, step, roundInput, roundState, routeTargetPaths, parseErr); reportErr != nil {
			return result, reportErr
		}
		return runner.retryBuilderRuntimeSchemaRepair(ctx, backend, runID, step, run, roundInput, upgradeRoute, stats, response.ModelAlias, response.Content, parseErr)
	}
	targetPaths := routeTargetPaths
	scopePaths := builderRuntimeAllowedPaths(run, roundInput, upgradeRoute)
	if shouldEnforceBuilderRuntimeTargetScope(upgradeRoute.TaskType) {
		if scopeErr := validateBuilderRuntimeTargetScope(patch.Operations, scopePaths); scopeErr != nil {
			stats.ScopeViolationCount++
			stats.FailureReason = scopeErr.Error()
			if reportErr := runner.reportPatchGenerationFailed(ctx, backend, runID, step, roundInput, roundState, scopePaths, scopeErr); reportErr != nil {
				return result, reportErr
			}
			return result, &builderRuntimeExecutionError{
				summary:            fmt.Sprintf("builder runtime upgrade patch exceeded repair target scope: %v", scopeErr),
				recoverySuggestion: "keep analyze/test repair edits inside the current repair allowed_paths and cover every directly failing target file",
				signature:          "builder_runtime_scope_violation",
				preserveWorkspace:  true,
				resumeAllowed:      false,
				policy:             repairFailurePolicy{PreserveWorkspace: true, ResumeAllowed: false, RequiresHumanReview: false, MaxRounds: 2, UsedRounds: 2, RemainingRounds: 0, TerminationReason: "builder runtime upgrade patch exceeded target scope"},
				stats:              stats,
				wrapped:            scopeErr,
			}
		}
	}
	if semanticErr := validateBuilderRuntimeSemanticConsistency(run, patch.Operations, ""); semanticErr != nil {
		stats.FailureReason = semanticErr.Error()
		if reportErr := runner.reportPatchGenerationFailed(ctx, backend, runID, step, roundInput, roundState, targetPaths, semanticErr); reportErr != nil {
			return result, reportErr
		}
		return result, &builderRuntimeExecutionError{
			summary:            fmt.Sprintf("builder runtime upgrade patch reintroduced schema-conflicting symbols: %v", semanticErr),
			recoverySuggestion: "align builder edits with the current model files and keep exported Dart model types plus removed generic fields consistent across dependents",
			signature:          "builder_runtime_semantic_conflict",
			preserveWorkspace:  true,
			resumeAllowed:      false,
			policy:             repairFailurePolicy{PreserveWorkspace: true, ResumeAllowed: false, RequiresHumanReview: false, MaxRounds: 2, UsedRounds: 2, RemainingRounds: 0, TerminationReason: "builder runtime upgrade patch reintroduced schema-conflicting symbols"},
			stats:              stats,
			wrapped:            semanticErr,
		}
	}
	if taskCoverageErr := validateBuilderRuntimeTaskTargetCoverage(upgradeRoute.TaskType, patch.Operations, targetPaths); taskCoverageErr != nil {
		stats.FailureReason = taskCoverageErr.Error()
		if reportErr := runner.reportPatchGenerationFailed(ctx, backend, runID, step, roundInput, roundState, targetPaths, taskCoverageErr); reportErr != nil {
			return result, reportErr
		}
		return result, &builderRuntimeExecutionError{
			summary:            fmt.Sprintf("builder runtime upgrade patch left current task target paths untouched: %v", taskCoverageErr),
			recoverySuggestion: "for non-repair builder tasks, touch every concrete task target path in the current patch before moving on",
			signature:          "builder_runtime_task_output_invalid",
			preserveWorkspace:  true,
			resumeAllowed:      false,
			policy:             repairFailurePolicy{PreserveWorkspace: true, ResumeAllowed: false, RequiresHumanReview: false, MaxRounds: 2, UsedRounds: 2, RemainingRounds: 0, TerminationReason: "builder runtime upgrade patch left current task target paths untouched"},
			stats:              stats,
			wrapped:            taskCoverageErr,
		}
	}
	if err := runner.reportGeneratedPatch(ctx, backend, runID, step, roundInput, roundState, &patch); err != nil {
		return result, err
	}
	preApplySnapshot, snapshotErr := captureBuilderRuntimeWorkspacePatchSnapshot(run.WorkspacePath, &patch)
	if snapshotErr != nil {
		return result, fmt.Errorf("capture builder runtime workspace before upgrade patch apply: %w", snapshotErr)
	}
	applyResult, applyErr := appruns.ApplyWorkspacePatch(run.WorkspacePath, run.AllowedPaths, run.ProtectedPaths, patch)
	if applyErr != nil {
		if strings.Contains(applyErr.Error(), "outside allowed roots") || strings.Contains(applyErr.Error(), "protected") {
			stats.ScopeViolationCount++
		}
		stats.FailureReason = applyErr.Error()
		if reportErr := runner.reportPatchApplyFailed(ctx, backend, runID, step, roundInput, roundState, workspacePatchPaths(&patch), applyErr); reportErr != nil {
			return result, reportErr
		}
		return result, &builderRuntimeExecutionError{
			summary:            fmt.Sprintf("builder runtime workspace patch apply failed after upgrade: %v", applyErr),
			recoverySuggestion: "inspect generated patch, allowed paths, and task routing before rerun",
			signature:          "workspace_patch_apply_failed",
			preserveWorkspace:  false,
			resumeAllowed:      false,
			policy:             repairFailurePolicy{PreserveWorkspace: false, ResumeAllowed: false, RequiresHumanReview: true, MaxRounds: 2, UsedRounds: 2, RemainingRounds: 0, TerminationReason: "builder runtime upgrade patch apply failed"},
			stats:              stats,
			wrapped:            applyErr,
		}
	}
	updateBuilderRuntimeOperationStats(stats, patch.Operations, targetPaths)
	if applyResult.Status != "" {
		patch.Status = applyResult.Status
	}
	if taskValidationErr := validateBuilderRuntimeTaskOutputs(run, upgradeRoute.TaskID); taskValidationErr != nil {
		if isBuilderRuntimeRepairableTaskOutputValidationFailure(taskValidationErr) {
			if restoreErr := restoreBuilderRuntimeWorkspacePatchSnapshot(run.WorkspacePath, preApplySnapshot, &patch); restoreErr != nil {
				return result, fmt.Errorf("restore builder runtime workspace after upgrade validation failure: %w", restoreErr)
			}
		}
		stats.FailureReason = taskValidationErr.Error()
		createdState := builderRuntimeRoundState(run.RoundState, step, upgradeRoute.TaskID, appruns.BuilderRuntimeTaskStatusCreated)
		if reportErr := runner.reportPatchApplyFailed(ctx, backend, runID, step, roundInput, createdState, workspacePatchPaths(&patch), taskValidationErr); reportErr != nil {
			return result, reportErr
		}
		if shouldRetryBuilderRuntimeTaskOutputValidationRepair(upgradeRoute.TaskType, stats, response.ModelAlias, response.Content, taskValidationErr) {
			return runner.retryBuilderRuntimeTaskOutputValidationRepair(ctx, backend, runID, step, run, roundInput, upgradeRoute, stats, response.ModelAlias, response.Content, &patch, taskValidationErr, request.FailureContext)
		}
		return result, &builderRuntimeExecutionError{
			summary:            fmt.Sprintf("builder runtime task output validation failed after upgrade: %v", taskValidationErr),
			recoverySuggestion: "keep the current task focused on one file and ensure every local Dart import/export/part resolves before moving to the next task",
			signature:          "builder_runtime_task_output_invalid",
			preserveWorkspace:  true,
			resumeAllowed:      false,
			policy:             repairFailurePolicy{PreserveWorkspace: true, ResumeAllowed: false, RequiresHumanReview: false, MaxRounds: 2, UsedRounds: 2, RemainingRounds: 0, TerminationReason: "builder runtime task output validation failed after upgrade"},
			stats:              stats,
			wrapped:            taskValidationErr,
		}
	}
	patch.ModifiedFiles = modifiedFilePaths(applyResult.ModifiedFiles)
	result.Patch = &patch
	result.ApplyResult = applyResult
	return result, nil
}

func shouldRetryBuilderRuntimeTaskTargetCoverageRepair(taskType appruns.BuilderRuntimeTaskType, stats *appruns.BuilderRuntimeExecutionStats, modelAlias, invalidContent string) bool {
	if stats == nil || stats.Attempts >= 2 {
		return false
	}
	switch taskType {
	case appruns.BuilderRuntimeTaskTypeAnalyzeRepair, appruns.BuilderRuntimeTaskTypeTestRepair, appruns.BuilderRuntimeTaskTypeClosureRepair:
		return false
	}
	return strings.TrimSpace(modelAlias) != "" && strings.TrimSpace(invalidContent) != ""
}

func shouldRetryBuilderRuntimeSemanticConflictRepair(taskType appruns.BuilderRuntimeTaskType, stats *appruns.BuilderRuntimeExecutionStats, modelAlias, invalidContent string, semanticErr error) bool {
	if stats == nil || stats.Attempts >= 2 {
		return false
	}
	if !isBuilderRuntimeRepairableSemanticConflict(semanticErr) {
		return false
	}
	switch taskType {
	case appruns.BuilderRuntimeTaskTypeAnalyzeRepair, appruns.BuilderRuntimeTaskTypeTestRepair, appruns.BuilderRuntimeTaskTypeClosureRepair:
		return isBuilderRuntimeRepairableFailureSymbolSemanticConflict(semanticErr) && strings.TrimSpace(modelAlias) != "" && strings.TrimSpace(invalidContent) != ""
	default:
		return strings.TrimSpace(modelAlias) != "" && strings.TrimSpace(invalidContent) != ""
	}
}

func shouldRetryBuilderRuntimeSemanticConflictAfterValidationRepair(stats *appruns.BuilderRuntimeExecutionStats, modelAlias, invalidContent string, semanticErr error) bool {
	const maxTaskOutputValidationRepairAttempts = 3
	const maxSemanticRepairAttemptsAfterValidation = maxTaskOutputValidationRepairAttempts + 1
	if stats == nil || stats.Attempts >= maxSemanticRepairAttemptsAfterValidation {
		return false
	}
	if !isBuilderRuntimeRepairableSemanticConflict(semanticErr) {
		return false
	}
	return strings.TrimSpace(modelAlias) != "" && strings.TrimSpace(invalidContent) != ""
}

func isBuilderRuntimeRepairableSemanticConflict(semanticErr error) bool {
	if semanticErr == nil {
		return false
	}
	message := semanticErr.Error()
	return strings.Contains(message, "reintroduced symbols absent from current models") ||
		strings.Contains(message, "invalid bookkeeping field/model references") ||
		strings.Contains(message, "patch introduced package imports not declared in pubspec.yaml") ||
		strings.Contains(message, "patch removed required helper symbols referenced by current validation failures") ||
		strings.Contains(message, "patch still references unresolved helper symbols from current validation failures")
}

func isBuilderRuntimeRepairableFailureSymbolSemanticConflict(semanticErr error) bool {
	if semanticErr == nil {
		return false
	}
	message := semanticErr.Error()
	return strings.Contains(message, "patch removed required helper symbols referenced by current validation failures") ||
		strings.Contains(message, "patch still references unresolved helper symbols from current validation failures")
}

func shouldRetryBuilderRuntimeTaskOutputValidationRepair(taskType appruns.BuilderRuntimeTaskType, stats *appruns.BuilderRuntimeExecutionStats, modelAlias, invalidContent string, validationErr error) bool {
	const maxTaskOutputValidationRepairAttempts = 3
	if stats == nil || stats.Attempts >= maxTaskOutputValidationRepairAttempts {
		return false
	}
	if !isBuilderRuntimeRepairableTaskOutputValidationFailure(validationErr) {
		return false
	}
	if taskType == appruns.BuilderRuntimeTaskTypeSingleFileEdit && strings.Contains(validationErr.Error(), "unresolved local import/export/part") {
		return false
	}
	return strings.TrimSpace(modelAlias) != "" && strings.TrimSpace(invalidContent) != ""
}

func (runner *Runner) retryBuilderRuntimeTaskOutputValidationRepair(ctx context.Context, backend RunnerBackend, runID string, step ExecutionStep, run runRecord, roundInput appruns.RoundInput, route appruns.BuilderRuntimeTaskRoute, stats *appruns.BuilderRuntimeExecutionStats, modelAlias, invalidContent string, invalidPatch *appruns.WorkspacePatch, validationErr error, failureContext string) (*builderRuntimeExecutionResult, error) {
	const maxTaskOutputValidationRepairAttempts = 3
	result := &builderRuntimeExecutionResult{AppliedRoundInput: cloneRoundInputValue(roundInput), Stats: stats}
	originalTargetPaths := builderRuntimeRouteTargetPaths(run.TaskBundle, route.TaskID)
	currentPatch := invalidPatch
	currentValidationErr := validationErr
	_ = invalidContent
	previousBody := ""
	for stats.Attempts < maxTaskOutputValidationRepairAttempts {
		repairTargetPaths := builderRuntimeTaskOutputValidationPaths(currentValidationErr)
		if len(repairTargetPaths) == 0 {
			repairTargetPaths = originalTargetPaths
		}
		repairRun := cloneRunRecordWithRouteTargetPaths(run, route.TaskID, repairTargetPaths)
		repairRoundInput := cloneRoundInputWithTaskTargetPaths(roundInput, route.TaskID, repairTargetPaths)
		request := BuilderRuntimePatchRequest{
			Run:            repairRun,
			RoundInput:     repairRoundInput,
			Route:          route,
			ModelAliases:   builderRuntimeRepairModelAliases(route.Model, modelAlias),
			RepairOnly:     true,
			PreviousErr:    currentValidationErr.Error(),
			PreviousBody:   previousBody,
			FailureContext: strings.TrimSpace(strings.Join([]string{failureContext, currentValidationErr.Error()}, "\n")),
		}
		routeTargetPaths := repairTargetPaths
		roundState := heartbeatRoundStateForStep(step)
		if err := runner.reportPatchGenerationStarted(ctx, backend, runID, step, roundInput, roundState, routeTargetPaths); err != nil {
			return result, err
		}
		response, attemptsUsed, err := runner.generateBuilderRuntimePatchWithTransientRetry(ctx, request)
		stats.Attempts += attemptsUsed
		if err != nil {
			stats.FailureReason = err.Error()
			if reportErr := runner.reportPatchGenerationFailed(ctx, backend, runID, step, roundInput, roundState, routeTargetPaths, err); reportErr != nil {
				return result, reportErr
			}
			return result, &builderRuntimeExecutionError{
				summary:            fmt.Sprintf("builder runtime task output validation repair request failed: %v", err),
				recoverySuggestion: "fix builder-runtime model connectivity or prompt configuration before rerun",
				signature:          "builder_runtime_model_request_failed",
				preserveWorkspace:  true,
				resumeAllowed:      false,
				policy:             repairFailurePolicy{PreserveWorkspace: true, ResumeAllowed: false, RequiresHumanReview: true, MaxRounds: maxTaskOutputValidationRepairAttempts, UsedRounds: stats.Attempts, RemainingRounds: maxInt(0, maxTaskOutputValidationRepairAttempts-stats.Attempts), TerminationReason: "builder runtime task output validation repair request failed"},
				stats:              stats,
				wrapped:            err,
			}
		}
		stats.SelectedModel = response.ModelAlias
		stats.ModelSequence = append(stats.ModelSequence, response.ModelAlias)
		stats.PromptTokens += response.PromptTokens
		stats.CompletionTokens += response.CompletionTokens
		stats.TotalTokens += response.TotalTokens
		appendBuilderRuntimeLog(run.LogPath, response.ModelAlias, response.Content)
		patch, normalized, driftCount, repairParseErr := normalizeBuilderRuntimePatchWithWorkspace(run.WorkspacePath, response.Content, roundInput.RoundID, routeTargetPaths)
		normalizeBuilderRuntimePatchForWorkspaceWithTaskBundleAndTargetPaths(run.WorkspacePath, roundInput.TaskBundle, routeTargetPaths, &patch)
		stats.SchemaNormalized = stats.SchemaNormalized || normalized
		stats.SchemaDriftCount += driftCount
		if repairParseErr != nil {
			stats.ParseFailureCount++
			stats.FailureReason = repairParseErr.Error()
			if reportErr := runner.reportPatchGenerationFailed(ctx, backend, runID, step, roundInput, roundState, routeTargetPaths, repairParseErr); reportErr != nil {
				return result, reportErr
			}
			return result, &builderRuntimeExecutionError{
				summary:            fmt.Sprintf("builder runtime patch parse failed after task output validation repair: %v", repairParseErr),
				recoverySuggestion: "keep the previous task intent but return valid compact JSON that fixes the reported Dart validation issue",
				signature:          "builder_runtime_patch_parse_failed",
				preserveWorkspace:  true,
				resumeAllowed:      false,
				policy:             repairFailurePolicy{PreserveWorkspace: true, ResumeAllowed: false, RequiresHumanReview: false, MaxRounds: maxTaskOutputValidationRepairAttempts, UsedRounds: stats.Attempts, RemainingRounds: maxInt(0, maxTaskOutputValidationRepairAttempts-stats.Attempts), TerminationReason: "builder runtime task output validation repair parse failed"},
				stats:              stats,
				wrapped:            repairParseErr,
			}
		}
		scopePaths := builderRuntimeAllowedPaths(repairRun, repairRoundInput, route)
		if shouldEnforceBuilderRuntimeTargetScope(route.TaskType) {
			if scopeErr := validateBuilderRuntimeTargetScope(patch.Operations, scopePaths); scopeErr != nil {
				stats.ScopeViolationCount++
				stats.FailureReason = scopeErr.Error()
				if reportErr := runner.reportPatchGenerationFailed(ctx, backend, runID, step, roundInput, roundState, scopePaths, scopeErr); reportErr != nil {
					return result, reportErr
				}
				return result, &builderRuntimeExecutionError{
					summary:            fmt.Sprintf("builder runtime workspace patch exceeded repair target scope after task output validation repair: %v", scopeErr),
					recoverySuggestion: "keep validation repairs inside the current task target paths and rewrite only the files named by the Dart validation failure",
					signature:          "builder_runtime_scope_violation",
					preserveWorkspace:  true,
					resumeAllowed:      false,
					policy:             repairFailurePolicy{PreserveWorkspace: true, ResumeAllowed: false, RequiresHumanReview: false, MaxRounds: maxTaskOutputValidationRepairAttempts, UsedRounds: stats.Attempts, RemainingRounds: maxInt(0, maxTaskOutputValidationRepairAttempts-stats.Attempts), TerminationReason: "builder runtime task output validation repair exceeded target scope"},
					stats:              stats,
					wrapped:            scopeErr,
				}
			}
		}
		if semanticErr := validateBuilderRuntimeSemanticConsistency(run, patch.Operations, failureContext); semanticErr != nil {
			stats.FailureReason = semanticErr.Error()
			if reportErr := runner.reportPatchGenerationFailed(ctx, backend, runID, step, roundInput, roundState, routeTargetPaths, semanticErr); reportErr != nil {
				return result, reportErr
			}
			if shouldRetryBuilderRuntimeSemanticConflictAfterValidationRepair(stats, response.ModelAlias, response.Content, semanticErr) {
				return runner.retryBuilderRuntimeSemanticConflictRepair(ctx, backend, runID, step, run, roundInput, route, stats, response.ModelAlias, response.Content, currentPatch, semanticErr, failureContext)
			}
			return result, &builderRuntimeExecutionError{
				summary:            fmt.Sprintf("builder runtime workspace patch reintroduced schema-conflicting symbols after task output validation repair: %v", semanticErr),
				recoverySuggestion: "align validation repairs with current models and avoid reintroducing removed schema tokens while fixing Dart syntax/import issues",
				signature:          "builder_runtime_semantic_conflict",
				preserveWorkspace:  true,
				resumeAllowed:      false,
				policy:             repairFailurePolicy{PreserveWorkspace: true, ResumeAllowed: false, RequiresHumanReview: false, MaxRounds: maxTaskOutputValidationRepairAttempts, UsedRounds: stats.Attempts, RemainingRounds: maxInt(0, maxTaskOutputValidationRepairAttempts-stats.Attempts), TerminationReason: "builder runtime task output validation repair reintroduced schema-conflicting symbols"},
				stats:              stats,
				wrapped:            semanticErr,
			}
		}
		combinedPatch := mergeWorkspacePatchesReplacingPaths(currentPatch, &patch)
		if combinedPatch == nil {
			combinedPatch = &patch
		}
		if semanticErr := validateBuilderRuntimeSemanticConsistency(run, combinedPatch.Operations, failureContext); semanticErr != nil {
			stats.FailureReason = semanticErr.Error()
			if reportErr := runner.reportPatchGenerationFailed(ctx, backend, runID, step, roundInput, roundState, originalTargetPaths, semanticErr); reportErr != nil {
				return result, reportErr
			}
			if shouldRetryBuilderRuntimeSemanticConflictAfterValidationRepair(stats, response.ModelAlias, response.Content, semanticErr) {
				return runner.retryBuilderRuntimeSemanticConflictRepair(ctx, backend, runID, step, run, roundInput, route, stats, response.ModelAlias, response.Content, currentPatch, semanticErr, failureContext)
			}
			return result, &builderRuntimeExecutionError{
				summary:            fmt.Sprintf("builder runtime workspace patch still reintroduced schema-conflicting symbols after task output validation repair: %v", semanticErr),
				recoverySuggestion: "rewrite the files named by the validation failure without renaming current exported models or restoring removed schema fields",
				signature:          "builder_runtime_semantic_conflict",
				preserveWorkspace:  true,
				resumeAllowed:      false,
				policy:             repairFailurePolicy{PreserveWorkspace: true, ResumeAllowed: false, RequiresHumanReview: false, MaxRounds: maxTaskOutputValidationRepairAttempts, UsedRounds: stats.Attempts, RemainingRounds: maxInt(0, maxTaskOutputValidationRepairAttempts-stats.Attempts), TerminationReason: "builder runtime task output validation repair still reintroduced schema-conflicting symbols"},
				stats:              stats,
				wrapped:            semanticErr,
			}
		}
		if taskCoverageErr := validateBuilderRuntimeTaskTargetCoverage(route.TaskType, combinedPatch.Operations, originalTargetPaths); taskCoverageErr != nil {
			stats.FailureReason = taskCoverageErr.Error()
			if reportErr := runner.reportPatchGenerationFailed(ctx, backend, runID, step, roundInput, roundState, originalTargetPaths, taskCoverageErr); reportErr != nil {
				return result, reportErr
			}
			return runner.retryBuilderRuntimeTaskTargetCoverageRepair(ctx, backend, runID, step, run, roundInput, route, stats, response.ModelAlias, response.Content, nil, combinedPatch, taskCoverageErr)
		}
		if err := runner.reportGeneratedPatch(ctx, backend, runID, step, roundInput, roundState, combinedPatch); err != nil {
			return result, err
		}
		preApplySnapshot, snapshotErr := captureBuilderRuntimeWorkspacePatchSnapshot(run.WorkspacePath, combinedPatch)
		if snapshotErr != nil {
			return result, fmt.Errorf("capture builder runtime workspace before validation repair apply: %w", snapshotErr)
		}
		applyResult, repairApplyErr := appruns.ApplyWorkspacePatch(run.WorkspacePath, run.AllowedPaths, run.ProtectedPaths, *combinedPatch)
		if repairApplyErr != nil {
			if strings.Contains(repairApplyErr.Error(), "outside allowed roots") || strings.Contains(repairApplyErr.Error(), "protected") {
				stats.ScopeViolationCount++
			}
			stats.FailureReason = repairApplyErr.Error()
			if reportErr := runner.reportPatchApplyFailed(ctx, backend, runID, step, roundInput, roundState, workspacePatchPaths(combinedPatch), repairApplyErr); reportErr != nil {
				return result, reportErr
			}
			return result, &builderRuntimeExecutionError{
				summary:            fmt.Sprintf("builder runtime workspace patch apply failed after task output validation repair: %v", repairApplyErr),
				recoverySuggestion: "prefer full-file write_file operations for the files named by the Dart validation failure before rerun",
				signature:          "workspace_patch_apply_failed",
				preserveWorkspace:  false,
				resumeAllowed:      false,
				policy:             repairFailurePolicy{PreserveWorkspace: false, ResumeAllowed: false, RequiresHumanReview: true, MaxRounds: maxTaskOutputValidationRepairAttempts, UsedRounds: stats.Attempts, RemainingRounds: maxInt(0, maxTaskOutputValidationRepairAttempts-stats.Attempts), TerminationReason: "builder runtime task output validation repair apply failed"},
				stats:              stats,
				wrapped:            repairApplyErr,
			}
		}
		updateBuilderRuntimeOperationStats(stats, combinedPatch.Operations, originalTargetPaths)
		if applyResult.Status != "" {
			combinedPatch.Status = applyResult.Status
		}
		if taskValidationErr := validateBuilderRuntimeTaskOutputs(run, route.TaskID); taskValidationErr != nil {
			if isBuilderRuntimeRepairableTaskOutputValidationFailure(taskValidationErr) {
				if restoreErr := restoreBuilderRuntimeWorkspacePatchSnapshot(run.WorkspacePath, preApplySnapshot, combinedPatch); restoreErr != nil {
					return result, fmt.Errorf("restore builder runtime workspace after validation repair failure: %w", restoreErr)
				}
			}
			stats.FailureReason = taskValidationErr.Error()
			createdState := builderRuntimeRoundState(run.RoundState, step, route.TaskID, appruns.BuilderRuntimeTaskStatusCreated)
			if reportErr := runner.reportPatchApplyFailed(ctx, backend, runID, step, roundInput, createdState, workspacePatchPaths(combinedPatch), taskValidationErr); reportErr != nil {
				return result, reportErr
			}
			currentPatch = combinedPatch
			currentValidationErr = taskValidationErr
			if stats.Attempts < maxTaskOutputValidationRepairAttempts {
				continue
			}
			return result, &builderRuntimeExecutionError{
				summary:            fmt.Sprintf("builder runtime task output validation failed after validation repair: %v", taskValidationErr),
				recoverySuggestion: "keep the current task focused and make the named Dart files parse cleanly before moving on",
				signature:          "builder_runtime_task_output_invalid",
				preserveWorkspace:  true,
				resumeAllowed:      false,
				state:              createdState,
				policy:             repairFailurePolicy{PreserveWorkspace: true, ResumeAllowed: false, RequiresHumanReview: false, MaxRounds: maxTaskOutputValidationRepairAttempts, UsedRounds: stats.Attempts, RemainingRounds: maxInt(0, maxTaskOutputValidationRepairAttempts-stats.Attempts), TerminationReason: "builder runtime task output validation failed after validation repair"},
				stats:              stats,
				wrapped:            taskValidationErr,
			}
		}
		combinedPatch.ModifiedFiles = modifiedFilePaths(applyResult.ModifiedFiles)
		result.Patch = combinedPatch
		result.ApplyResult = applyResult
		return result, nil
	}
	return result, &builderRuntimeExecutionError{
		summary:            fmt.Sprintf("builder runtime task output validation failed after validation repair: %v", currentValidationErr),
		recoverySuggestion: "keep the current task focused and make the named Dart files parse cleanly before moving on",
		signature:          "builder_runtime_task_output_invalid",
		preserveWorkspace:  true,
		resumeAllowed:      false,
		policy:             repairFailurePolicy{PreserveWorkspace: true, ResumeAllowed: false, RequiresHumanReview: false, MaxRounds: maxTaskOutputValidationRepairAttempts, UsedRounds: stats.Attempts, RemainingRounds: maxInt(0, maxTaskOutputValidationRepairAttempts-stats.Attempts), TerminationReason: "builder runtime task output validation repair exhausted"},
		stats:              stats,
		wrapped:            currentValidationErr,
	}
}

func (runner *Runner) retryBuilderRuntimeSemanticConflictRepair(ctx context.Context, backend RunnerBackend, runID string, step ExecutionStep, run runRecord, roundInput appruns.RoundInput, route appruns.BuilderRuntimeTaskRoute, stats *appruns.BuilderRuntimeExecutionStats, modelAlias, invalidContent string, invalidPatch *appruns.WorkspacePatch, semanticErr error, failureContext string) (*builderRuntimeExecutionResult, error) {
	result := &builderRuntimeExecutionResult{AppliedRoundInput: cloneRoundInputValue(roundInput), Stats: stats}
	originalTargetPaths := builderRuntimeRouteTargetPaths(run.TaskBundle, route.TaskID)
	repairTargetPaths := builderRuntimeSemanticConflictPaths(semanticErr)
	if len(repairTargetPaths) == 0 {
		repairTargetPaths = originalTargetPaths
	}
	repairRun := cloneRunRecordWithRouteTargetPaths(run, route.TaskID, repairTargetPaths)
	repairRoundInput := cloneRoundInputWithTaskTargetPaths(roundInput, route.TaskID, repairTargetPaths)
	request := BuilderRuntimePatchRequest{
		Run:            repairRun,
		RoundInput:     repairRoundInput,
		Route:          route,
		ModelAliases:   builderRuntimeRepairModelAliases(route.Model, modelAlias),
		RepairOnly:     true,
		PreviousErr:    semanticErr.Error(),
		PreviousBody:   invalidContent,
		FailureContext: strings.TrimSpace(strings.Join([]string{failureContext, semanticErr.Error()}, "\n")),
	}
	routeTargetPaths := repairTargetPaths
	roundState := heartbeatRoundStateForStep(step)
	if err := runner.reportPatchGenerationStarted(ctx, backend, runID, step, roundInput, roundState, routeTargetPaths); err != nil {
		return result, err
	}
	response, attemptsUsed, err := runner.generateBuilderRuntimePatchWithTransientRetry(ctx, request)
	stats.Attempts += attemptsUsed
	if err != nil {
		stats.FailureReason = err.Error()
		if reportErr := runner.reportPatchGenerationFailed(ctx, backend, runID, step, roundInput, roundState, routeTargetPaths, err); reportErr != nil {
			return result, reportErr
		}
		return result, &builderRuntimeExecutionError{
			summary:            fmt.Sprintf("builder runtime semantic repair request failed: %v", err),
			recoverySuggestion: "fix builder-runtime model connectivity or prompt configuration before rerun",
			signature:          "builder_runtime_model_request_failed",
			preserveWorkspace:  true,
			resumeAllowed:      false,
			policy:             repairFailurePolicy{PreserveWorkspace: true, ResumeAllowed: false, RequiresHumanReview: true, MaxRounds: 2, UsedRounds: stats.Attempts, RemainingRounds: maxInt(0, 2-stats.Attempts), TerminationReason: "builder runtime semantic repair request failed"},
			stats:              stats,
			wrapped:            err,
		}
	}
	stats.SelectedModel = response.ModelAlias
	stats.ModelSequence = append(stats.ModelSequence, response.ModelAlias)
	stats.PromptTokens += response.PromptTokens
	stats.CompletionTokens += response.CompletionTokens
	stats.TotalTokens += response.TotalTokens
	appendBuilderRuntimeLog(run.LogPath, response.ModelAlias, response.Content)
	patch, normalized, driftCount, repairParseErr := normalizeBuilderRuntimePatchWithWorkspace(run.WorkspacePath, response.Content, roundInput.RoundID, routeTargetPaths)
	normalizeBuilderRuntimePatchForWorkspaceWithTaskBundleAndTargetPaths(run.WorkspacePath, roundInput.TaskBundle, routeTargetPaths, &patch)
	stats.SchemaNormalized = stats.SchemaNormalized || normalized
	stats.SchemaDriftCount += driftCount
	if repairParseErr != nil {
		stats.ParseFailureCount++
		stats.FailureReason = repairParseErr.Error()
		if reportErr := runner.reportPatchGenerationFailed(ctx, backend, runID, step, roundInput, roundState, routeTargetPaths, repairParseErr); reportErr != nil {
			return result, reportErr
		}
		return result, &builderRuntimeExecutionError{
			summary:            fmt.Sprintf("builder runtime patch parse failed after semantic repair: %v", repairParseErr),
			recoverySuggestion: "keep the original task intent but return valid compact JSON that fixes the reported semantic conflict",
			signature:          "builder_runtime_patch_parse_failed",
			preserveWorkspace:  true,
			resumeAllowed:      false,
			policy:             repairFailurePolicy{PreserveWorkspace: true, ResumeAllowed: false, RequiresHumanReview: false, MaxRounds: 2, UsedRounds: stats.Attempts, RemainingRounds: maxInt(0, 2-stats.Attempts), TerminationReason: "builder runtime semantic repair parse failed"},
			stats:              stats,
			wrapped:            repairParseErr,
		}
	}
	scopePaths := builderRuntimeAllowedPaths(repairRun, repairRoundInput, route)
	if shouldEnforceBuilderRuntimeTargetScope(route.TaskType) {
		if scopeErr := validateBuilderRuntimeTargetScope(patch.Operations, scopePaths); scopeErr != nil {
			stats.ScopeViolationCount++
			stats.FailureReason = scopeErr.Error()
			if reportErr := runner.reportPatchGenerationFailed(ctx, backend, runID, step, roundInput, roundState, scopePaths, scopeErr); reportErr != nil {
				return result, reportErr
			}
			return result, &builderRuntimeExecutionError{
				summary:            fmt.Sprintf("builder runtime workspace patch exceeded repair target scope after semantic repair: %v", scopeErr),
				recoverySuggestion: "keep semantic repairs inside the current task allowed_paths and only rewrite the files implicated by the semantic conflict",
				signature:          "builder_runtime_scope_violation",
				preserveWorkspace:  true,
				resumeAllowed:      false,
				policy:             repairFailurePolicy{PreserveWorkspace: true, ResumeAllowed: false, RequiresHumanReview: false, MaxRounds: 2, UsedRounds: stats.Attempts, RemainingRounds: maxInt(0, 2-stats.Attempts), TerminationReason: "builder runtime semantic repair exceeded target scope"},
				stats:              stats,
				wrapped:            scopeErr,
			}
		}
	}
	combinedPatch := mergeWorkspacePatchesReplacingPaths(invalidPatch, &patch)
	if combinedPatch == nil {
		combinedPatch = &patch
	}
	if semanticErr := validateBuilderRuntimeSemanticConsistency(run, combinedPatch.Operations, failureContext); semanticErr != nil {
		stats.FailureReason = semanticErr.Error()
		if reportErr := runner.reportPatchGenerationFailed(ctx, backend, runID, step, roundInput, roundState, originalTargetPaths, semanticErr); reportErr != nil {
			return result, reportErr
		}
		return result, &builderRuntimeExecutionError{
			summary:            fmt.Sprintf("builder runtime workspace patch still reintroduced schema-conflicting symbols after semantic repair: %v", semanticErr),
			recoverySuggestion: "rewrite the files named in the semantic conflict and keep dependent bookkeeping fields/imports aligned with current models",
			signature:          "builder_runtime_semantic_conflict",
			preserveWorkspace:  true,
			resumeAllowed:      false,
			policy:             repairFailurePolicy{PreserveWorkspace: true, ResumeAllowed: false, RequiresHumanReview: false, MaxRounds: 2, UsedRounds: stats.Attempts, RemainingRounds: maxInt(0, 2-stats.Attempts), TerminationReason: "builder runtime semantic repair still reintroduced schema-conflicting symbols"},
			stats:              stats,
			wrapped:            semanticErr,
		}
	}
	if coverageErr := validateBuilderRuntimeDirectFailureCoverage(route.TaskType, combinedPatch.Operations, originalTargetPaths); coverageErr != nil {
		stats.FailureReason = coverageErr.Error()
		if reportErr := runner.reportPatchGenerationFailed(ctx, backend, runID, step, roundInput, roundState, originalTargetPaths, coverageErr); reportErr != nil {
			return result, reportErr
		}
		return result, &builderRuntimeExecutionError{
			summary:            fmt.Sprintf("builder runtime patch left directly failing repair targets untouched after semantic repair: %v", coverageErr),
			recoverySuggestion: "rewrite every directly failing file in the current repair slice before rerun",
			signature:          "builder_runtime_semantic_conflict",
			preserveWorkspace:  true,
			resumeAllowed:      false,
			policy:             repairFailurePolicy{PreserveWorkspace: true, ResumeAllowed: false, RequiresHumanReview: false, MaxRounds: 2, UsedRounds: stats.Attempts, RemainingRounds: maxInt(0, 2-stats.Attempts), TerminationReason: "builder runtime semantic repair left directly failing files untouched"},
			stats:              stats,
			wrapped:            coverageErr,
		}
	}
	if taskCoverageErr := validateBuilderRuntimeTaskTargetCoverage(route.TaskType, combinedPatch.Operations, originalTargetPaths); taskCoverageErr != nil {
		stats.FailureReason = taskCoverageErr.Error()
		if reportErr := runner.reportPatchGenerationFailed(ctx, backend, runID, step, roundInput, roundState, originalTargetPaths, taskCoverageErr); reportErr != nil {
			return result, reportErr
		}
		return runner.retryBuilderRuntimeTaskTargetCoverageRepair(ctx, backend, runID, step, run, roundInput, route, stats, response.ModelAlias, response.Content, nil, combinedPatch, taskCoverageErr)
		return result, &builderRuntimeExecutionError{
			summary:            fmt.Sprintf("builder runtime patch left current task target paths untouched after semantic repair: %v", taskCoverageErr),
			recoverySuggestion: "after fixing semantic conflicts, touch every concrete task target path in the current patch before moving on",
			signature:          "builder_runtime_task_output_invalid",
			preserveWorkspace:  true,
			resumeAllowed:      false,
			policy:             repairFailurePolicy{PreserveWorkspace: true, ResumeAllowed: false, RequiresHumanReview: false, MaxRounds: 2, UsedRounds: stats.Attempts, RemainingRounds: maxInt(0, 2-stats.Attempts), TerminationReason: "builder runtime semantic repair left current task target paths untouched"},
			stats:              stats,
			wrapped:            taskCoverageErr,
		}
	}
	if err := runner.reportGeneratedPatch(ctx, backend, runID, step, roundInput, roundState, combinedPatch); err != nil {
		return result, err
	}
	applyResult, repairApplyErr := appruns.ApplyWorkspacePatch(run.WorkspacePath, run.AllowedPaths, run.ProtectedPaths, *combinedPatch)
	if repairApplyErr != nil {
		if strings.Contains(repairApplyErr.Error(), "outside allowed roots") || strings.Contains(repairApplyErr.Error(), "protected") {
			stats.ScopeViolationCount++
		}
		stats.FailureReason = repairApplyErr.Error()
		if reportErr := runner.reportPatchApplyFailed(ctx, backend, runID, step, roundInput, roundState, workspacePatchPaths(combinedPatch), repairApplyErr); reportErr != nil {
			return result, reportErr
		}
		return result, &builderRuntimeExecutionError{
			summary:            fmt.Sprintf("builder runtime workspace patch apply failed after semantic repair: %v", repairApplyErr),
			recoverySuggestion: "prefer full-file write_file operations for the files named by the semantic conflict before rerun",
			signature:          "workspace_patch_apply_failed",
			preserveWorkspace:  false,
			resumeAllowed:      false,
			policy:             repairFailurePolicy{PreserveWorkspace: false, ResumeAllowed: false, RequiresHumanReview: true, MaxRounds: 2, UsedRounds: stats.Attempts, RemainingRounds: maxInt(0, 2-stats.Attempts), TerminationReason: "builder runtime semantic repair apply failed"},
			stats:              stats,
			wrapped:            repairApplyErr,
		}
	}
	updateBuilderRuntimeOperationStats(stats, combinedPatch.Operations, originalTargetPaths)
	if applyResult.Status != "" {
		combinedPatch.Status = applyResult.Status
	}
	combinedPatch.ModifiedFiles = modifiedFilePaths(applyResult.ModifiedFiles)
	result.Patch = combinedPatch
	result.ApplyResult = applyResult
	return result, nil
}

func builderRuntimeSemanticConflictPaths(err error) []string {
	if err == nil {
		return nil
	}
	matches := regexp.MustCompile(`([A-Za-z0-9_./-]+\.dart)\s*->`).FindAllStringSubmatch(err.Error(), -1)
	if len(matches) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	paths := make([]string, 0, len(matches))
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
	sort.Strings(paths)
	return paths
}

func builderRuntimeTaskOutputValidationPaths(err error) []string {
	if err == nil {
		return nil
	}
	matches := regexp.MustCompile(`task (?:file|target file)\s+([A-Za-z0-9_./-]+\.dart)\b`).FindAllStringSubmatch(err.Error(), -1)
	if len(matches) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	paths := make([]string, 0, len(matches))
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
	sort.Strings(paths)
	return paths
}

func mergeWorkspacePatchesReplacingPaths(base, extra *appruns.WorkspacePatch) *appruns.WorkspacePatch {
	if base == nil {
		return extra
	}
	if extra == nil {
		return base
	}
	replacedPaths := map[string]struct{}{}
	for _, operation := range extra.Operations {
		path := filepath.ToSlash(strings.TrimSpace(operation.Path))
		if path == "" {
			continue
		}
		replacedPaths[path] = struct{}{}
	}
	merged := *base
	merged.Operations = make([]appruns.WorkspacePatchOperation, 0, len(base.Operations)+len(extra.Operations))
	for _, operation := range base.Operations {
		path := filepath.ToSlash(strings.TrimSpace(operation.Path))
		if _, replaced := replacedPaths[path]; replaced {
			continue
		}
		merged.Operations = append(merged.Operations, operation)
	}
	merged.Operations = append(merged.Operations, extra.Operations...)
	if strings.TrimSpace(extra.PatchID) != "" {
		merged.PatchID = extra.PatchID
	}
	if strings.TrimSpace(extra.Status) != "" {
		merged.Status = extra.Status
	}
	merged.ModifiedFiles = append(append([]string(nil), base.ModifiedFiles...), extra.ModifiedFiles...)
	return &merged
}

func (runner *Runner) retryBuilderRuntimeTaskTargetCoverageRepair(ctx context.Context, backend RunnerBackend, runID string, step ExecutionStep, run runRecord, roundInput appruns.RoundInput, route appruns.BuilderRuntimeTaskRoute, stats *appruns.BuilderRuntimeExecutionStats, modelAlias, invalidContent string, existingPatch *appruns.WorkspacePatch, invalidPatch *appruns.WorkspacePatch, coverageErr error) (*builderRuntimeExecutionResult, error) {
	const maxTaskTargetCoverageRepairAttempts = 3
	result := &builderRuntimeExecutionResult{AppliedRoundInput: cloneRoundInputValue(roundInput), Stats: stats}
	repairAttempts := 0
	if strings.TrimSpace(modelAlias) == "" || strings.TrimSpace(invalidContent) == "" {
		return result, &builderRuntimeExecutionError{
			summary:            fmt.Sprintf("builder runtime patch left current task target paths untouched: %v", coverageErr),
			recoverySuggestion: "rerun the builder task with a patch that covers every concrete target path",
			signature:          "builder_runtime_task_output_invalid",
			preserveWorkspace:  true,
			resumeAllowed:      false,
			policy:             repairFailurePolicy{PreserveWorkspace: true, ResumeAllowed: false, RequiresHumanReview: false, MaxRounds: maxTaskTargetCoverageRepairAttempts, UsedRounds: repairAttempts, RemainingRounds: maxTaskTargetCoverageRepairAttempts, TerminationReason: "builder runtime task target coverage repair unavailable"},
			stats:              stats,
			wrapped:            coverageErr,
		}
	}
	originalTargetPaths := builderRuntimeRouteTargetPaths(run.TaskBundle, route.TaskID)
	currentPatch := invalidPatch
	previousBody := invalidContent
	currentCoverageErr := coverageErr
	for repairAttempts < maxTaskTargetCoverageRepairAttempts {
		combinedCurrent := mergeWorkspacePatches(existingPatch, currentPatch)
		repairTargetPaths := builderRuntimeDirectFailureCoverageMissingTargets(nil, originalTargetPaths)
		if combinedCurrent != nil {
			repairTargetPaths = builderRuntimeDirectFailureCoverageMissingTargets(combinedCurrent.Operations, originalTargetPaths)
		}
		if len(repairTargetPaths) == 0 {
			repairTargetPaths = originalTargetPaths
		}
		repairRun := cloneRunRecordWithRouteTargetPaths(run, route.TaskID, repairTargetPaths)
		repairRoundInput := cloneRoundInputWithTaskTargetPaths(roundInput, route.TaskID, repairTargetPaths)
		request := BuilderRuntimePatchRequest{
			Run:            repairRun,
			RoundInput:     repairRoundInput,
			Route:          route,
			ModelAliases:   builderRuntimeRepairModelAliases(route.Model, modelAlias),
			RepairOnly:     true,
			PreviousErr:    currentCoverageErr.Error(),
			PreviousBody:   previousBody,
			FailureContext: currentCoverageErr.Error(),
		}
		routeTargetPaths := repairTargetPaths
		roundState := heartbeatRoundStateForStep(step)
		if err := runner.reportPatchGenerationStarted(ctx, backend, runID, step, roundInput, roundState, routeTargetPaths); err != nil {
			return result, err
		}
		response, attemptsUsed, err := runner.generateBuilderRuntimePatchWithTransientRetry(ctx, request)
		stats.Attempts += attemptsUsed
		repairAttempts++
		if err != nil {
			stats.FailureReason = err.Error()
			if reportErr := runner.reportPatchGenerationFailed(ctx, backend, runID, step, roundInput, roundState, routeTargetPaths, err); reportErr != nil {
				return result, reportErr
			}
			return result, &builderRuntimeExecutionError{
				summary:            fmt.Sprintf("builder runtime task target coverage repair request failed: %v", err),
				recoverySuggestion: "fix builder-runtime model connectivity or prompt configuration before rerun",
				signature:          "builder_runtime_model_request_failed",
				preserveWorkspace:  true,
				resumeAllowed:      false,
				policy:             repairFailurePolicy{PreserveWorkspace: true, ResumeAllowed: false, RequiresHumanReview: true, MaxRounds: maxTaskTargetCoverageRepairAttempts, UsedRounds: repairAttempts, RemainingRounds: maxInt(0, maxTaskTargetCoverageRepairAttempts-repairAttempts), TerminationReason: "builder runtime task target coverage repair request failed"},
				stats:              stats,
				wrapped:            err,
			}
		}
		stats.SelectedModel = response.ModelAlias
		stats.ModelSequence = append(stats.ModelSequence, response.ModelAlias)
		stats.PromptTokens += response.PromptTokens
		stats.CompletionTokens += response.CompletionTokens
		stats.TotalTokens += response.TotalTokens
		appendBuilderRuntimeLog(run.LogPath, response.ModelAlias, response.Content)
		patch, normalized, driftCount, repairParseErr := normalizeBuilderRuntimePatchWithWorkspace(run.WorkspacePath, response.Content, roundInput.RoundID, routeTargetPaths)
		normalizeBuilderRuntimePatchForWorkspaceWithTaskBundleAndTargetPaths(run.WorkspacePath, roundInput.TaskBundle, routeTargetPaths, &patch)
		stats.SchemaNormalized = stats.SchemaNormalized || normalized
		stats.SchemaDriftCount += driftCount
		if repairParseErr != nil {
			stats.ParseFailureCount++
			stats.FailureReason = repairParseErr.Error()
			if reportErr := runner.reportPatchGenerationFailed(ctx, backend, runID, step, roundInput, roundState, routeTargetPaths, repairParseErr); reportErr != nil {
				return result, reportErr
			}
			return result, &builderRuntimeExecutionError{
				summary:            fmt.Sprintf("builder runtime patch parse failed after task target coverage repair: %v", repairParseErr),
				recoverySuggestion: "keep the previous patch intent but return valid compact JSON that covers every remaining task target file",
				signature:          "builder_runtime_patch_parse_failed",
				preserveWorkspace:  true,
				resumeAllowed:      false,
				policy:             repairFailurePolicy{PreserveWorkspace: true, ResumeAllowed: false, RequiresHumanReview: false, MaxRounds: maxTaskTargetCoverageRepairAttempts, UsedRounds: repairAttempts, RemainingRounds: maxInt(0, maxTaskTargetCoverageRepairAttempts-repairAttempts), TerminationReason: "builder runtime task target coverage repair parse failed"},
				stats:              stats,
				wrapped:            repairParseErr,
			}
		}
		scopePaths := builderRuntimeAllowedPaths(run, roundInput, route)
		if shouldEnforceBuilderRuntimeTargetScope(route.TaskType) {
			if scopeErr := validateBuilderRuntimeTargetScope(patch.Operations, scopePaths); scopeErr != nil {
				stats.ScopeViolationCount++
				stats.FailureReason = scopeErr.Error()
				if reportErr := runner.reportPatchGenerationFailed(ctx, backend, runID, step, roundInput, roundState, scopePaths, scopeErr); reportErr != nil {
					return result, reportErr
				}
				return result, &builderRuntimeExecutionError{
					summary:            fmt.Sprintf("builder runtime workspace patch exceeded task target scope after coverage repair: %v", scopeErr),
					recoverySuggestion: "keep builder task edits inside the current task allowed_paths while covering every missing target file",
					signature:          "builder_runtime_scope_violation",
					preserveWorkspace:  true,
					resumeAllowed:      false,
					policy:             repairFailurePolicy{PreserveWorkspace: true, ResumeAllowed: false, RequiresHumanReview: false, MaxRounds: maxTaskTargetCoverageRepairAttempts, UsedRounds: repairAttempts, RemainingRounds: maxInt(0, maxTaskTargetCoverageRepairAttempts-repairAttempts), TerminationReason: "builder runtime task target coverage repair exceeded target scope"},
					stats:              stats,
					wrapped:            scopeErr,
				}
			}
		}
		if semanticErr := validateBuilderRuntimeSemanticConsistency(run, patch.Operations, ""); semanticErr != nil {
			stats.FailureReason = semanticErr.Error()
			if reportErr := runner.reportPatchGenerationFailed(ctx, backend, runID, step, roundInput, roundState, routeTargetPaths, semanticErr); reportErr != nil {
				return result, reportErr
			}
			return result, &builderRuntimeExecutionError{
				summary:            fmt.Sprintf("builder runtime workspace patch reintroduced schema-conflicting symbols after task target coverage repair: %v", semanticErr),
				recoverySuggestion: "align builder edits with current models and task contracts before continuing",
				signature:          "builder_runtime_semantic_conflict",
				preserveWorkspace:  true,
				resumeAllowed:      false,
				policy:             repairFailurePolicy{PreserveWorkspace: true, ResumeAllowed: false, RequiresHumanReview: false, MaxRounds: maxTaskTargetCoverageRepairAttempts, UsedRounds: repairAttempts, RemainingRounds: maxInt(0, maxTaskTargetCoverageRepairAttempts-repairAttempts), TerminationReason: "builder runtime task target coverage repair reintroduced schema-conflicting symbols"},
				stats:              stats,
				wrapped:            semanticErr,
			}
		}
		combinedPatch := mergeWorkspacePatches(combinedCurrent, &patch)
		if combinedPatch == nil {
			combinedPatch = &patch
		}
		if taskCoverageErr := validateBuilderRuntimeTaskTargetCoverage(route.TaskType, combinedPatch.Operations, originalTargetPaths); taskCoverageErr != nil {
			stats.FailureReason = taskCoverageErr.Error()
			if reportErr := runner.reportPatchGenerationFailed(ctx, backend, runID, step, roundInput, roundState, originalTargetPaths, taskCoverageErr); reportErr != nil {
				return result, reportErr
			}
			currentPatch = combinedPatch
			previousBody = response.Content
			currentCoverageErr = taskCoverageErr
			if repairAttempts < maxTaskTargetCoverageRepairAttempts {
				continue
			}
			return result, &builderRuntimeExecutionError{
				summary:            fmt.Sprintf("builder runtime patch still left current task target paths untouched after coverage repair: %v", taskCoverageErr),
				recoverySuggestion: "rewrite every remaining task target file in the current slice before rerun",
				signature:          "builder_runtime_task_output_invalid",
				preserveWorkspace:  true,
				resumeAllowed:      false,
				policy:             repairFailurePolicy{PreserveWorkspace: true, ResumeAllowed: false, RequiresHumanReview: false, MaxRounds: maxTaskTargetCoverageRepairAttempts, UsedRounds: repairAttempts, RemainingRounds: maxInt(0, maxTaskTargetCoverageRepairAttempts-repairAttempts), TerminationReason: "builder runtime task target coverage repair still left required files untouched"},
				stats:              stats,
				wrapped:            taskCoverageErr,
			}
		}
		if err := runner.reportGeneratedPatch(ctx, backend, runID, step, roundInput, roundState, combinedPatch); err != nil {
			return result, err
		}
		applyResult, repairApplyErr := appruns.ApplyWorkspacePatch(run.WorkspacePath, run.AllowedPaths, run.ProtectedPaths, *combinedPatch)
		if repairApplyErr != nil {
			if strings.Contains(repairApplyErr.Error(), "outside allowed roots") || strings.Contains(repairApplyErr.Error(), "protected") {
				stats.ScopeViolationCount++
			}
			stats.FailureReason = repairApplyErr.Error()
			if reportErr := runner.reportPatchApplyFailed(ctx, backend, runID, step, roundInput, roundState, workspacePatchPaths(combinedPatch), repairApplyErr); reportErr != nil {
				return result, reportErr
			}
			return result, &builderRuntimeExecutionError{
				summary:            fmt.Sprintf("builder runtime workspace patch apply failed after task target coverage repair: %v", repairApplyErr),
				recoverySuggestion: "inspect generated patch, allowed paths, and task routing before rerun",
				signature:          "workspace_patch_apply_failed",
				preserveWorkspace:  false,
				resumeAllowed:      false,
				policy:             repairFailurePolicy{PreserveWorkspace: false, ResumeAllowed: false, RequiresHumanReview: true, MaxRounds: maxTaskTargetCoverageRepairAttempts, UsedRounds: repairAttempts, RemainingRounds: maxInt(0, maxTaskTargetCoverageRepairAttempts-repairAttempts), TerminationReason: "builder runtime task target coverage repair apply failed"},
				stats:              stats,
				wrapped:            repairApplyErr,
			}
		}
		if applyResult.Status != "" {
			combinedPatch.Status = applyResult.Status
		}
		if taskValidationErr := validateBuilderRuntimeTaskOutputs(run, route.TaskID); taskValidationErr != nil {
			stats.FailureReason = taskValidationErr.Error()
			createdState := builderRuntimeRoundState(run.RoundState, step, route.TaskID, appruns.BuilderRuntimeTaskStatusCreated)
			if reportErr := runner.reportPatchApplyFailed(ctx, backend, runID, step, roundInput, createdState, workspacePatchPaths(combinedPatch), taskValidationErr); reportErr != nil {
				return result, reportErr
			}
			if shouldRetryBuilderRuntimeTaskOutputValidationRepair(route.TaskType, stats, response.ModelAlias, response.Content, taskValidationErr) {
				return runner.retryBuilderRuntimeTaskOutputValidationRepair(ctx, backend, runID, step, run, roundInput, route, stats, response.ModelAlias, response.Content, combinedPatch, taskValidationErr, currentCoverageErr.Error())
			}
			return result, &builderRuntimeExecutionError{
				summary:            fmt.Sprintf("builder runtime task output validation failed after task target coverage repair: %v", taskValidationErr),
				recoverySuggestion: "keep the current task focused and ensure every local Dart import/export/part resolves before moving on",
				signature:          "builder_runtime_task_output_invalid",
				preserveWorkspace:  true,
				resumeAllowed:      false,
				state:              createdState,
				policy:             repairFailurePolicy{PreserveWorkspace: true, ResumeAllowed: false, RequiresHumanReview: false, MaxRounds: maxTaskTargetCoverageRepairAttempts, UsedRounds: repairAttempts, RemainingRounds: maxInt(0, maxTaskTargetCoverageRepairAttempts-repairAttempts), TerminationReason: "builder runtime task output validation failed after task target coverage repair"},
				stats:              stats,
				wrapped:            taskValidationErr,
			}
		}
		combinedPatch.ModifiedFiles = modifiedFilePaths(applyResult.ModifiedFiles)
		result.Patch = combinedPatch
		result.ApplyResult = applyResult
		return result, nil
	}
	return result, &builderRuntimeExecutionError{
		summary:            fmt.Sprintf("builder runtime patch still left current task target paths untouched after coverage repair: %v", currentCoverageErr),
		recoverySuggestion: "rewrite every remaining task target file in the current slice before rerun",
		signature:          "builder_runtime_task_output_invalid",
		preserveWorkspace:  true,
		resumeAllowed:      false,
		policy:             repairFailurePolicy{PreserveWorkspace: true, ResumeAllowed: false, RequiresHumanReview: false, MaxRounds: maxTaskTargetCoverageRepairAttempts, UsedRounds: repairAttempts, RemainingRounds: maxInt(0, maxTaskTargetCoverageRepairAttempts-repairAttempts), TerminationReason: "builder runtime task target coverage repair exhausted"},
		stats:              stats,
		wrapped:            currentCoverageErr,
	}
}

func (runner *Runner) retryBuilderRuntimeDirectFailureCoverageRepair(ctx context.Context, backend RunnerBackend, runID string, step ExecutionStep, run runRecord, roundInput appruns.RoundInput, route appruns.BuilderRuntimeTaskRoute, stats *appruns.BuilderRuntimeExecutionStats, modelAlias, invalidContent string, invalidPatch *appruns.WorkspacePatch, coverageErr error, failureContext string) (*builderRuntimeExecutionResult, error) {
	const maxDirectFailureCoverageRepairAttempts = 4
	result := &builderRuntimeExecutionResult{AppliedRoundInput: cloneRoundInputValue(roundInput), Stats: stats}
	if strings.TrimSpace(modelAlias) == "" || strings.TrimSpace(invalidContent) == "" {
		return result, &builderRuntimeExecutionError{
			summary:            fmt.Sprintf("builder runtime patch left directly failing repair targets untouched: %v", coverageErr),
			recoverySuggestion: "rerun analyze/test repair with a patch that covers every directly failing target file in the current slice",
			signature:          "builder_runtime_semantic_conflict",
			preserveWorkspace:  true,
			resumeAllowed:      false,
			policy:             repairFailurePolicy{PreserveWorkspace: true, ResumeAllowed: false, RequiresHumanReview: false, MaxRounds: maxDirectFailureCoverageRepairAttempts, UsedRounds: stats.Attempts, RemainingRounds: maxInt(0, maxDirectFailureCoverageRepairAttempts-stats.Attempts), TerminationReason: "builder runtime direct failure coverage repair unavailable"},
			stats:              stats,
			wrapped:            coverageErr,
		}
	}
	originalTargetPaths := builderRuntimeRouteTargetPaths(run.TaskBundle, route.TaskID)
	currentPatch := invalidPatch
	previousBody := invalidContent
	currentCoverageErr := coverageErr
	for stats.Attempts < maxDirectFailureCoverageRepairAttempts {
		repairTargetPaths := builderRuntimeDirectFailureCoverageMissingTargets(nil, originalTargetPaths)
		if currentPatch != nil {
			repairTargetPaths = builderRuntimeDirectFailureCoverageMissingTargets(currentPatch.Operations, originalTargetPaths)
		}
		if len(repairTargetPaths) == 0 {
			repairTargetPaths = originalTargetPaths
		}
		repairRun := cloneRunRecordWithRouteTargetPaths(run, route.TaskID, repairTargetPaths)
		repairRoundInput := cloneRoundInputWithTaskTargetPaths(roundInput, route.TaskID, repairTargetPaths)
		request := BuilderRuntimePatchRequest{
			Run:            repairRun,
			RoundInput:     repairRoundInput,
			Route:          route,
			ModelAliases:   builderRuntimeRepairModelAliases(route.Model, modelAlias),
			RepairOnly:     true,
			PreviousErr:    currentCoverageErr.Error(),
			PreviousBody:   previousBody,
			FailureContext: strings.TrimSpace(strings.Join([]string{failureContext, currentCoverageErr.Error()}, "\n")),
		}
		routeTargetPaths := repairTargetPaths
		roundState := heartbeatRoundStateForStep(step)
		if err := runner.reportPatchGenerationStarted(ctx, backend, runID, step, roundInput, roundState, routeTargetPaths); err != nil {
			return result, err
		}
		response, attemptsUsed, err := runner.generateBuilderRuntimePatchWithTransientRetry(ctx, request)
		stats.Attempts += attemptsUsed
		if err != nil {
			stats.FailureReason = err.Error()
			if reportErr := runner.reportPatchGenerationFailed(ctx, backend, runID, step, roundInput, roundState, routeTargetPaths, err); reportErr != nil {
				return result, reportErr
			}
			return result, &builderRuntimeExecutionError{
				summary:            fmt.Sprintf("builder runtime direct failure coverage repair request failed: %v", err),
				recoverySuggestion: "fix builder-runtime model connectivity or prompt configuration before rerun",
				signature:          "builder_runtime_model_request_failed",
				preserveWorkspace:  true,
				resumeAllowed:      false,
				policy:             repairFailurePolicy{PreserveWorkspace: true, ResumeAllowed: false, RequiresHumanReview: true, MaxRounds: maxDirectFailureCoverageRepairAttempts, UsedRounds: stats.Attempts, RemainingRounds: maxInt(0, maxDirectFailureCoverageRepairAttempts-stats.Attempts), TerminationReason: "builder runtime direct failure coverage repair request failed"},
				stats:              stats,
				wrapped:            err,
			}
		}
		stats.SelectedModel = response.ModelAlias
		stats.ModelSequence = append(stats.ModelSequence, response.ModelAlias)
		stats.PromptTokens += response.PromptTokens
		stats.CompletionTokens += response.CompletionTokens
		stats.TotalTokens += response.TotalTokens
		appendBuilderRuntimeLog(run.LogPath, response.ModelAlias, response.Content)
		patch, normalized, driftCount, repairParseErr := normalizeBuilderRuntimePatchWithWorkspace(run.WorkspacePath, response.Content, roundInput.RoundID, routeTargetPaths)
		normalizeBuilderRuntimePatchForWorkspaceWithTaskBundleAndTargetPaths(run.WorkspacePath, roundInput.TaskBundle, routeTargetPaths, &patch)
		stats.SchemaNormalized = stats.SchemaNormalized || normalized
		stats.SchemaDriftCount += driftCount
		if repairParseErr != nil {
			stats.ParseFailureCount++
			stats.FailureReason = repairParseErr.Error()
			if reportErr := runner.reportPatchGenerationFailed(ctx, backend, runID, step, roundInput, roundState, routeTargetPaths, repairParseErr); reportErr != nil {
				return result, reportErr
			}
			return result, &builderRuntimeExecutionError{
				summary:            fmt.Sprintf("builder runtime patch parse failed after direct failure coverage repair: %v", repairParseErr),
				recoverySuggestion: "keep the previous patch intent but return valid compact JSON that covers every directly failing target file",
				signature:          "builder_runtime_patch_parse_failed",
				preserveWorkspace:  true,
				resumeAllowed:      false,
				policy:             repairFailurePolicy{PreserveWorkspace: true, ResumeAllowed: false, RequiresHumanReview: false, MaxRounds: maxDirectFailureCoverageRepairAttempts, UsedRounds: stats.Attempts, RemainingRounds: maxInt(0, maxDirectFailureCoverageRepairAttempts-stats.Attempts), TerminationReason: "builder runtime direct failure coverage repair parse failed"},
				stats:              stats,
				wrapped:            repairParseErr,
			}
		}
		targetPaths := routeTargetPaths
		scopePaths := builderRuntimeAllowedPaths(run, roundInput, route)
		if shouldEnforceBuilderRuntimeTargetScope(route.TaskType) {
			if scopeErr := validateBuilderRuntimeTargetScope(patch.Operations, scopePaths); scopeErr != nil {
				stats.ScopeViolationCount++
				stats.FailureReason = scopeErr.Error()
				if reportErr := runner.reportPatchGenerationFailed(ctx, backend, runID, step, roundInput, roundState, scopePaths, scopeErr); reportErr != nil {
					return result, reportErr
				}
				return result, &builderRuntimeExecutionError{
					summary:            fmt.Sprintf("builder runtime workspace patch exceeded repair target scope after direct failure coverage repair: %v", scopeErr),
					recoverySuggestion: "keep analyze/test repair edits inside the current repair allowed_paths and cover every directly failing target file",
					signature:          "builder_runtime_scope_violation",
					preserveWorkspace:  true,
					resumeAllowed:      false,
					policy:             repairFailurePolicy{PreserveWorkspace: true, ResumeAllowed: false, RequiresHumanReview: false, MaxRounds: maxDirectFailureCoverageRepairAttempts, UsedRounds: stats.Attempts, RemainingRounds: maxInt(0, maxDirectFailureCoverageRepairAttempts-stats.Attempts), TerminationReason: "builder runtime direct failure coverage repair exceeded target scope"},
					stats:              stats,
					wrapped:            scopeErr,
				}
			}
		}
		if semanticErr := validateBuilderRuntimeSemanticConsistency(run, patch.Operations, failureContext); semanticErr != nil {
			stats.FailureReason = semanticErr.Error()
			if reportErr := runner.reportPatchGenerationFailed(ctx, backend, runID, step, roundInput, roundState, targetPaths, semanticErr); reportErr != nil {
				return result, reportErr
			}
			return result, &builderRuntimeExecutionError{
				summary:            fmt.Sprintf("builder runtime workspace patch reintroduced schema-conflicting symbols after direct failure coverage repair: %v", semanticErr),
				recoverySuggestion: "align builder edits with current models and repair dependent files without reintroducing removed schema tokens",
				signature:          "builder_runtime_semantic_conflict",
				preserveWorkspace:  true,
				resumeAllowed:      false,
				policy:             repairFailurePolicy{PreserveWorkspace: true, ResumeAllowed: false, RequiresHumanReview: false, MaxRounds: maxDirectFailureCoverageRepairAttempts, UsedRounds: stats.Attempts, RemainingRounds: maxInt(0, maxDirectFailureCoverageRepairAttempts-stats.Attempts), TerminationReason: "builder runtime direct failure coverage repair reintroduced schema-conflicting symbols"},
				stats:              stats,
				wrapped:            semanticErr,
			}
		}
		combinedPatch := mergeWorkspacePatches(currentPatch, &patch)
		if combinedPatch == nil {
			combinedPatch = &patch
		}
		if semanticErr := validateBuilderRuntimeSemanticConsistency(run, combinedPatch.Operations, failureContext); semanticErr != nil {
			stats.FailureReason = semanticErr.Error()
			if reportErr := runner.reportPatchGenerationFailed(ctx, backend, runID, step, roundInput, roundState, originalTargetPaths, semanticErr); reportErr != nil {
				return result, reportErr
			}
			return result, &builderRuntimeExecutionError{
				summary:            fmt.Sprintf("builder runtime workspace patch reintroduced schema-conflicting symbols after direct failure coverage repair: %v", semanticErr),
				recoverySuggestion: "preserve or restore currently defined helper symbols referenced by the active analyze/test failure before continuing other edits",
				signature:          "builder_runtime_semantic_conflict",
				preserveWorkspace:  true,
				resumeAllowed:      false,
				policy:             repairFailurePolicy{PreserveWorkspace: true, ResumeAllowed: false, RequiresHumanReview: false, MaxRounds: maxDirectFailureCoverageRepairAttempts, UsedRounds: stats.Attempts, RemainingRounds: maxInt(0, maxDirectFailureCoverageRepairAttempts-stats.Attempts), TerminationReason: "builder runtime direct failure coverage repair removed required helper symbols"},
				stats:              stats,
				wrapped:            semanticErr,
			}
		}
		if repairCoverageErr := validateBuilderRuntimeDirectFailureCoverage(route.TaskType, combinedPatch.Operations, originalTargetPaths); repairCoverageErr != nil {
			stats.FailureReason = repairCoverageErr.Error()
			if reportErr := runner.reportPatchGenerationFailed(ctx, backend, runID, step, roundInput, roundState, originalTargetPaths, repairCoverageErr); reportErr != nil {
				return result, reportErr
			}
			currentPatch = combinedPatch
			previousBody = response.Content
			currentCoverageErr = repairCoverageErr
			if stats.Attempts < maxDirectFailureCoverageRepairAttempts {
				continue
			}
			return result, &builderRuntimeExecutionError{
				summary:            fmt.Sprintf("builder runtime patch still left directly failing repair targets untouched after coverage repair: %v", repairCoverageErr),
				recoverySuggestion: "rewrite every directly failing file in the current repair slice with explicit operations before rerun",
				signature:          "builder_runtime_semantic_conflict",
				preserveWorkspace:  true,
				resumeAllowed:      false,
				policy:             repairFailurePolicy{PreserveWorkspace: true, ResumeAllowed: false, RequiresHumanReview: false, MaxRounds: maxDirectFailureCoverageRepairAttempts, UsedRounds: stats.Attempts, RemainingRounds: maxInt(0, maxDirectFailureCoverageRepairAttempts-stats.Attempts), TerminationReason: "builder runtime direct failure coverage repair still left required files untouched"},
				stats:              stats,
				wrapped:            repairCoverageErr,
			}
		}
		if err := runner.reportGeneratedPatch(ctx, backend, runID, step, roundInput, roundState, combinedPatch); err != nil {
			return result, err
		}
		applyResult, repairApplyErr := appruns.ApplyWorkspacePatch(run.WorkspacePath, run.AllowedPaths, run.ProtectedPaths, *combinedPatch)
		if repairApplyErr != nil {
			if strings.Contains(repairApplyErr.Error(), "outside allowed roots") || strings.Contains(repairApplyErr.Error(), "protected") {
				stats.ScopeViolationCount++
			}
			stats.FailureReason = repairApplyErr.Error()
			if reportErr := runner.reportPatchApplyFailed(ctx, backend, runID, step, roundInput, roundState, workspacePatchPaths(combinedPatch), repairApplyErr); reportErr != nil {
				return result, reportErr
			}
			if shouldRetryBuilderRuntimePatchApplyRepair(repairApplyErr) {
				invalidBody := previousBody
				if encodedPatch, marshalErr := json.Marshal(combinedPatch); marshalErr == nil {
					invalidBody = string(encodedPatch)
				}
				return runner.retryBuilderRuntimePatchApplyRepair(ctx, backend, runID, step, run, roundInput, route, stats, response.ModelAlias, invalidBody, repairApplyErr)
			}
			return result, &builderRuntimeExecutionError{
				summary:            fmt.Sprintf("builder runtime workspace patch apply failed after direct failure coverage repair: %v", repairApplyErr),
				recoverySuggestion: "inspect the repaired patch and prefer full-file write_file operations for directly failing views and tests before rerun",
				signature:          "workspace_patch_apply_failed",
				preserveWorkspace:  false,
				resumeAllowed:      false,
				policy:             repairFailurePolicy{PreserveWorkspace: false, ResumeAllowed: false, RequiresHumanReview: true, MaxRounds: maxDirectFailureCoverageRepairAttempts, UsedRounds: stats.Attempts, RemainingRounds: maxInt(0, maxDirectFailureCoverageRepairAttempts-stats.Attempts), TerminationReason: "builder runtime direct failure coverage repair apply failed"},
				stats:              stats,
				wrapped:            repairApplyErr,
			}
		}
		updateBuilderRuntimeOperationStats(stats, combinedPatch.Operations, originalTargetPaths)
		if applyResult.Status != "" {
			combinedPatch.Status = applyResult.Status
		}
		combinedPatch.ModifiedFiles = modifiedFilePaths(applyResult.ModifiedFiles)
		result.Patch = combinedPatch
		result.ApplyResult = applyResult
		return result, nil
	}
	return result, &builderRuntimeExecutionError{
		summary:            fmt.Sprintf("builder runtime patch still left directly failing repair targets untouched after coverage repair: %v", currentCoverageErr),
		recoverySuggestion: "rewrite every directly failing file in the current repair slice with explicit operations before rerun",
		signature:          "builder_runtime_semantic_conflict",
		preserveWorkspace:  true,
		resumeAllowed:      false,
		policy:             repairFailurePolicy{PreserveWorkspace: true, ResumeAllowed: false, RequiresHumanReview: false, MaxRounds: maxDirectFailureCoverageRepairAttempts, UsedRounds: stats.Attempts, RemainingRounds: maxInt(0, maxDirectFailureCoverageRepairAttempts-stats.Attempts), TerminationReason: "builder runtime direct failure coverage repair exhausted"},
		stats:              stats,
		wrapped:            currentCoverageErr,
	}
}

func nextBuilderRuntimeUpgradeModel(plan *appruns.BuilderRuntimePlan, previousModel string) (appruns.BuilderRuntimeModelRef, bool) {
	if plan == nil {
		return appruns.BuilderRuntimeModelRef{}, false
	}
	aliases := modelAliasesFromRef(plan.UpgradeModel)
	if len(aliases) == 0 {
		return appruns.BuilderRuntimeModelRef{}, false
	}
	start := 0
	trimmedPrevious := strings.TrimSpace(previousModel)
	if trimmedPrevious != "" {
		index := slices.Index(aliases, trimmedPrevious)
		if index >= 0 {
			start = index + 1
		}
	}
	if start >= len(aliases) {
		return appruns.BuilderRuntimeModelRef{}, false
	}
	return appruns.BuilderRuntimeModelRef{
		Primary:   aliases[start],
		Fallbacks: append([]string(nil), aliases[start+1:]...),
	}, true
}

func updateBuilderRuntimeOperationStats(stats *appruns.BuilderRuntimeExecutionStats, operations []appruns.WorkspacePatchOperation, targetPaths []string) {
	if stats == nil {
		return
	}
	stats.OperationCount = len(operations)
	stats.TargetedOperationCount, stats.UnrelatedOperationCount = classifyBuilderRuntimeOperations(operations, targetPaths)
	if stats.OperationCount == 0 {
		return
	}
	rate := float64(stats.UnrelatedOperationCount) / float64(stats.OperationCount)
	if rate > stats.UnrelatedOperationRate {
		stats.UnrelatedOperationRate = rate
	}
}

func buildBuilderRuntimePrompt(request BuilderRuntimePatchRequest) (string, error) {
	files, err := buildBuilderRuntimeFileContext(request.Run, request.Route.TaskID, request.Route.TaskType)
	if err != nil {
		return "", err
	}
	domainModelContext, err := loadBuilderRuntimeDomainModelContext(request.Run.WorkspacePath)
	if err != nil {
		return "", err
	}
	dominantTask := appruns.NormalizeTaskBundleItem(resolveBuilderRuntimePromptTask(request.RoundInput.TaskBundle, request.Route.TaskID))
	checksJSON, _ := json.Marshal(request.RoundInput.AcceptanceChecks)
	allowedJSON, _ := json.Marshal(builderRuntimeAllowedPaths(request.Run, request.RoundInput, request.Route))
	currentTaskTargetPaths := concreteTaskTargetPaths([]appruns.TaskBundleItem{dominantTask})
	if shouldUseCompactBuilderRuntimeSummaryModelCreatePrompt(request, currentTaskTargetPaths) {
		return buildCompactBuilderRuntimeSummaryModelCreatePrompt(request, files, domainModelContext, dominantTask, currentTaskTargetPaths, checksJSON, allowedJSON)
	}
	if shouldUseCompactBuilderRuntimeModelCreatePrompt(request, currentTaskTargetPaths) {
		return buildCompactBuilderRuntimeModelCreatePrompt(request, files, domainModelContext, dominantTask, currentTaskTargetPaths, checksJSON, allowedJSON), nil
	}
	if shouldUseCompactBuilderRuntimeOverviewBindPrompt(request, dominantTask, currentTaskTargetPaths) {
		return buildCompactBuilderRuntimeOverviewBindPrompt(request, files, domainModelContext, dominantTask, currentTaskTargetPaths, checksJSON, allowedJSON)
	}
	exportedModelContext, err := loadBuilderRuntimeExportedModelContext(request.Run.WorkspacePath)
	if err != nil {
		return "", err
	}
	modelAPISnapshot, err := loadBuilderRuntimeModelAPISnapshot(request.Run.WorkspacePath)
	if err != nil {
		return "", err
	}
	forbiddenSchemaTokens, err := builderRuntimeForbiddenSchemaTokens(request.Run.WorkspacePath)
	if err != nil {
		return "", err
	}
	removableLegacyExports, err := builderRuntimeRemovableLegacyExportNames(request.Run.WorkspacePath)
	if err != nil {
		return "", err
	}
	focusTasks := focusBuilderRuntimeTasks(request.RoundInput.TaskBundle, request.Route.TaskID, request.Route.TaskType)
	focusTasksJSON, _ := json.Marshal(focusTasks)
	dominantTaskJSON, _ := json.Marshal(dominantTask)
	failureContext := strings.TrimSpace(request.FailureContext)
	directFailurePaths := extractValidationFailurePathsFromContent(failureContext)
	failureScopePaths := mergeBuilderRuntimeContextPaths(currentTaskTargetPaths, directFailurePaths)
	dartSyntaxRepair := request.RepairOnly && isBuilderRuntimeDartSyntaxValidationFailure(errors.New(strings.TrimSpace(strings.Join([]string{request.PreviousErr, failureContext}, "\n"))))
	weightRecordDomain := builderRuntimePromptLooksLikeWeightRecordDomain(request.Run.WorkspacePath)
	bookkeepingDomain := builderRuntimePromptLooksLikeBookkeepingDomain(request.Run.WorkspacePath)
	relationRichModelProfile := builderRuntimeRelationRichModelProfileForWorkspace(request.Run.WorkspacePath)
	relationRichWorkspace := builderRuntimeWorkspaceHasSemanticFile(request.Run.WorkspacePath, "lib/models/project.dart") &&
		builderRuntimeWorkspaceHasSemanticFile(request.Run.WorkspacePath, "lib/models/task.dart") &&
		builderRuntimeWorkspaceHasSemanticFile(request.Run.WorkspacePath, "lib/models/tag.dart") &&
		builderRuntimeWorkspaceHasSemanticFile(request.Run.WorkspacePath, "lib/models/task_tag_link.dart") &&
		builderRuntimeWorkspaceHasSemanticFile(request.Run.WorkspacePath, "lib/models/dashboard_summary.dart")
	useReferenceTemplateForbiddenTokenFallback := len(request.Run.TemplateReferenceFiles) > 0 && !builderRuntimeTargetsModelFiles(currentTaskTargetPaths)
	if useReferenceTemplateForbiddenTokenFallback {
		forbiddenSchemaTokens = nil
	}
	var builder strings.Builder
	builder.WriteString("Return compact JSON only. No markdown.\n")
	builder.WriteString("Top-level keys: patch_id, operations.\n")
	builder.WriteString("Allowed operation types: write_file, replace_block, delete_file.\n")
	builder.WriteString("Do not include a nested type field inside an operation body that conflicts with the outer operation key. If deleting a file, emit delete_file as the outer operation type instead of embedding type=delete inside replace_block.\n")
	builder.WriteString("For replace_block, new_content is mandatory. Also provide path and either anchor or old_content; you may additionally provide start/end if anchor is unavailable.\n")
	builder.WriteString("Do not use line-number fields such as start_line or end_line.\n")
	builder.WriteString("Before sending, verify the final response is valid JSON and every array/object is closed.\n")
	builder.WriteString("Do not add commentary before or after the JSON object.\n")
	builder.WriteString("If you output two operations, ensure the operations array contains exactly two objects and ends with ].\n")
	builder.WriteString("Never touch paths outside allowed_paths.\n")
	builder.WriteString("Do not delete, empty, or orphan files listed in task target_paths. If a target file needs a full rewrite, use write_file with complete replacement content.\n")
	builder.WriteString("For any path listed in task target_paths, delete_file is forbidden. Rewrite the file with write_file or edit it with replace_block instead.\n")
	builder.WriteString("Never use replace_block with an anchor or old_content that may match multiple regions in the same file. If uniqueness is uncertain, rewrite the full target file with write_file instead.\n")
	builder.WriteString("For UI entry files in task target_paths such as lib/main.dart and lib/views/*.dart, prefer write_file with the complete final file content instead of replace_block. Do not emit a partial replace_block for those files.\n")
	builder.WriteString("For repository, controller, and model files in task target_paths such as lib/repositories/*.dart, lib/controllers/*.dart, and lib/models/*.dart, prefer write_file with the complete final file content over replace_block unless the matched old_content is guaranteed unique inside that file.\n")
	if len(request.Run.TemplateReferenceFiles) > 0 {
		builder.WriteString("Reference-template mode is active. Sections labeled REFERENCE TEMPLATE are examples from the template source tree, not existing workspace files to patch in place. Use them as structural/style references only.\n")
		builder.WriteString("Sections labeled REFERENCE TEMPLATE OMITTED mean a template counterpart exists, but its full body is intentionally excluded in closure/repair or dependency context to keep the prompt focused on current workspace APIs.\n")
		builder.WriteString("If the current task target file appears as REFERENCE TEMPLATE or TARGET FILE TO CREATE, create the workspace file from scratch with write_file using the domain model and current dependency files. Do not copy generic schema fields verbatim from the reference template.\n")
		builder.WriteString("When reference-template mode is active, prefer a single write_file operation for the current task target file unless the target file already exists in the workspace and you are doing a same-file repair.\n")
	}
	if len(currentTaskTargetPaths) == 1 {
		builder.WriteString("Current task should converge on one primary workspace file: ")
		builder.WriteString(currentTaskTargetPaths[0])
		builder.WriteString(". Prefer exactly one write_file operation for that file unless a same-file replace_block is clearly safer during repair.\n")
	}
	if len(currentTaskTargetPaths) == 1 && request.Route.TaskType != appruns.BuilderRuntimeTaskTypeAnalyzeRepair && request.Route.TaskType != appruns.BuilderRuntimeTaskTypeTestRepair && request.Route.TaskType != appruns.BuilderRuntimeTaskTypeClosureRepair {
		builder.WriteString("Current non-repair task target_paths contains exactly one concrete file. Emit operations only for ")
		builder.WriteString(currentTaskTargetPaths[0])
		builder.WriteString(" in this task; treat every other file shown in context as read-only reference material.\n")
	}
	if builderRuntimeTargetsRelationRichModelSlice(currentTaskTargetPaths) {
		builder.WriteString(builderRuntimeRelationRichModelPromptGuidance(builderRuntimeRelationRichModelProfileForContext(request.Run.WorkspacePath, currentTaskTargetPaths)))
	}
	if builderRuntimeTargetsOnlyPath(currentTaskTargetPaths, "lib/main.dart") {
		builder.WriteString("If current task target_paths contains only lib/main.dart, treat every other file shown in context as read-only reference material. Emit operations only for lib/main.dart in this task; do not rewrite dependent view, controller, repository, model, template, Android, or test files here.\n")
	}
	builder.WriteString("If you change shared model fields, constructor parameters, enum values, widget constructor signatures, controller public APIs, or summary metrics, update every dependent controller, repository, widget, lib/main.dart callsite, and test in the current file context within the same patch. Do not leave stale references to removed fields, old constructor arguments, or superseded summary properties.\n")
	if builderRuntimePathsContainDartPathWithin(currentTaskTargetPaths, "lib/template/") {
		appendBuilderRuntimeOpenLiteTemplateCopyPromptGuidance(&builder, forbiddenSchemaTokens)
	}
	if builderRuntimePathsContainPrefix(currentTaskTargetPaths, "lib/repositories/") {
		repositoryContent := ""
		for _, targetPath := range currentTaskTargetPaths {
			if !strings.HasPrefix(targetPath, "lib/repositories/") {
				continue
			}
			content, err := builderRuntimeSemanticFileContent(request.Run.WorkspacePath, nil, targetPath)
			if err != nil || strings.TrimSpace(content) == "" {
				continue
			}
			repositoryContent = content
			break
		}
		repositoryModelContent := builderRuntimeRepositoryPrimaryModelContent(request.Run.WorkspacePath, repositoryContent)
		if strings.TrimSpace(repositoryModelContent) != "" {
			builder.WriteString("For current repository-scope target files under lib/repositories/*.dart, keep persistence, serialization, sorting, and query helpers aligned to the exact field names exported by the current primary record model in context. Do not assume generic seed fields such as id or updatedAt unless the current model still defines them.\n")
			builder.WriteString("If the current primary record model already exposes toMap()/fromMap() but does not declare Hive TypeAdapter/annotation code, keep the current repository file on the template-style Hive map persistence path: use Box<dynamic>, persist List<Map<String, dynamic>> payloads via _box.get/_box.put, and reconstruct records with RecordType.fromMap(Map<String, dynamic>.from(item as Map)). Do not introduce Hive.registerAdapter(...) or Box<RecordType> unless the same patch also defines the required adapter types in the current primary model.\n")
			if slices.Contains(forbiddenSchemaTokens, ".updatedAt") {
				builder.WriteString("Current primary record model does not define updatedAt. Do not read, write, sort, serialize, or map by updatedAt inside the current repository file; use the actual time field from the current model if one exists, otherwise omit updatedAt-based behavior.\n")
			}
		}
	}
	if builderRuntimePathsContainPrefix(currentTaskTargetPaths, "lib/repositories/") && relationRichModelProfile != "" {
		builder.WriteString(builderRuntimeRelationRichRecordRepositoryPromptGuidance(relationRichModelProfile))
	}
	if builderRuntimeTaskTouchesGenericOverviewSlice(dominantTask, request.RoundInput.TaskBundle, currentTaskTargetPaths) {
		builder.WriteString("For current overview-surface target files, keep summary cards, total-count copy, recent-record metadata, and trailing record text aligned to the exact fields exported by the current dashboard_summary.dart and current primary record model in context. Do not assume generic seed fields such as totalCount or updatedAt unless the current models still define them.\n")
		if slices.Contains(forbiddenSchemaTokens, "totalCount") {
			builder.WriteString("Current summary model does not define totalCount. In current overview-surface target files, derive any overall record count from controller.records.length or other current summary fields; do not reference summary.totalCount.\n")
		}
		if slices.Contains(forbiddenSchemaTokens, ".updatedAt") {
			builder.WriteString("Current primary record model does not define updatedAt. In current overview-surface target files, use the actual time field from the current primary record model if one exists; otherwise omit updatedAt-based date text instead of inventing a generic timestamp.\n")
		}
	}
	if builderRuntimeTaskTouchesGenericCollectionInspectionSlice(dominantTask, request.RoundInput.TaskBundle, currentTaskTargetPaths) {
		builder.WriteString("For current collection/inspection-surface target files, keep trailing date text and detail date tiles aligned to the actual time field exported by the current primary record model. Do not assume updatedAt unless the current model still defines it.\n")
		if slices.Contains(forbiddenSchemaTokens, ".updatedAt") {
			builder.WriteString("Current primary record model does not define updatedAt. In current collection/inspection-surface target files, use the actual time field from the current primary record model if one exists; otherwise omit updatedAt-based date text and date tiles.\n")
		}
	}
	if builderRuntimeTargetsPath(currentTaskTargetPaths, "android/app/build.gradle.kts") {
		builder.WriteString("For android/app/build.gradle.kts, keep the Kotlin DSL valid and preserve flutter { source = \"../..\" } exactly. Do not emit malformed Flutter source path literals such as source = \"..\".. .\n")
		builder.WriteString("Keep namespace and applicationId aligned to the existing defaultOpenLiteApplicationId constant. Do not invent undefined identifiers such as defaultOpenLiteApplication in android/app/build.gradle.kts.\n")
	}
	if containsTargetPath(request.RoundInput.TaskBundle, "lib/models/record.dart") && containsTargetPath(request.RoundInput.TaskBundle, "lib/models/dashboard_summary.dart") {
		appendBuilderRuntimeOpenLiteSchemaRemapGuidance(&builder)
	}
	if dominantTask.AllocationTransition != nil {
		ownedPathsJSON, _ := json.Marshal(dominantTask.AllocationTransition.OwnedPaths)
		semanticRefsJSON, _ := json.Marshal(dominantTask.AllocationTransition.SemanticIntentRefs)
		surfaceRefsJSON, _ := json.Marshal(dominantTask.AllocationTransition.SurfaceRefs)
		entityRefsJSON, _ := json.Marshal(dominantTask.AllocationTransition.EntityRefs)
		successEvidenceJSON, _ := json.Marshal(dominantTask.AllocationTransition.SuccessEvidence)
		if len(dominantTask.AllocationTransition.SurfaceRefs) > 0 {
			builder.WriteString("Treat allocation_transition.surface_refs as the primary interaction surfaces for this patch. Keep navigation, widget states, copy, and tests aligned with those surfaces instead of drifting back to unrelated default seed pages.\n")
			builder.WriteString("Current allocation surface_refs JSON: ")
			builder.Write(surfaceRefsJSON)
			builder.WriteString("\n")
		}
		if len(dominantTask.AllocationTransition.EntityRefs) > 0 {
			builder.WriteString("Treat allocation_transition.entity_refs as the primary domain entities for this patch. Keep model, repository, controller, widget, copy, and test changes aligned with those entities instead of reintroducing unrelated generic schema concepts.\n")
			builder.WriteString("Current allocation entity_refs JSON: ")
			builder.Write(entityRefsJSON)
			builder.WriteString("\n")
		}
		if len(dominantTask.AllocationTransition.SemanticIntentRefs) > 0 {
			builder.WriteString("Current allocation semantic_intent_refs JSON: ")
			builder.Write(semanticRefsJSON)
			builder.WriteString("\n")
		}
		if len(dominantTask.AllocationTransition.SuccessEvidence) > 0 {
			builder.WriteString("Current allocation success_evidence JSON: ")
			builder.Write(successEvidenceJSON)
			builder.WriteString("\n")
		}
		if len(dominantTask.AllocationTransition.OwnedPaths) > 0 {
			builder.WriteString("Current allocation owned_paths JSON: ")
			builder.Write(ownedPathsJSON)
			builder.WriteString("\n")
		}
	}
	hasExplicitSurfaceTopology := builderRuntimeTaskBundleHasAnySurfaceRef(
		request.RoundInput.TaskBundle,
		builderRuntimeOverviewSurfaceRef,
		builderRuntimeCollectionSurfaceRef,
		builderRuntimeMutationSurfaceRef,
		builderRuntimeInspectionSurfaceRef,
	)
	hasOverviewSurface := builderRuntimeTaskBundleHasSurfaceRef(request.RoundInput.TaskBundle, builderRuntimeOverviewSurfaceRef) || (!hasExplicitSurfaceTopology && (containsTargetPath(request.RoundInput.TaskBundle, "lib/views/home_page.dart") || containsTargetPath(request.RoundInput.TaskBundle, "lib/controllers/home_controller.dart")))
	hasCollectionSurface := builderRuntimeTaskBundleHasSurfaceRef(request.RoundInput.TaskBundle, builderRuntimeCollectionSurfaceRef) || (!hasExplicitSurfaceTopology && (containsTargetPath(request.RoundInput.TaskBundle, "lib/views/record_list_page.dart") || containsTargetPath(request.RoundInput.TaskBundle, "lib/controllers/record_list_controller.dart")))
	hasDetailSurface := builderRuntimeTaskBundleHasSurfaceRef(request.RoundInput.TaskBundle, builderRuntimeInspectionSurfaceRef) || (!hasExplicitSurfaceTopology && builderRuntimeTaskBundleHasFallbackInspectionSurface(request.RoundInput.TaskBundle))
	if !hasExplicitSurfaceTopology {
		likelyOverviewViewPaths, likelyOverviewControllerPaths := builderRuntimeLikelyOverviewSurfaceTargetPaths(request.RoundInput.TaskBundle)
		likelyCollectionViewPaths, likelyCollectionControllerPaths := builderRuntimeLikelyCollectionSurfaceTargetPaths(request.RoundInput.TaskBundle)
		hasOverviewSurface = hasOverviewSurface || len(likelyOverviewViewPaths) > 0 || len(likelyOverviewControllerPaths) > 0
		hasCollectionSurface = hasCollectionSurface || len(likelyCollectionViewPaths) > 0 || len(likelyCollectionControllerPaths) > 0
	}
	hasFilterFlow := builderRuntimeTaskBundleHasSemanticIntentRef(request.RoundInput.TaskBundle, "ac-filter")
	hasDeleteFlow := builderRuntimeTaskBundleHasSemanticIntentRef(request.RoundInput.TaskBundle, "ac-delete")
	hasRepositoryScope := builderRuntimePathsContainPrefix(currentTaskTargetPaths, "lib/repositories/")
	hasControllerScope := builderRuntimePathsContainPrefix(currentTaskTargetPaths, "lib/controllers/")
	hasViewScope := builderRuntimePathsContainPrefix(currentTaskTargetPaths, "lib/views/")
	currentTaskTargetsMain := builderRuntimeTargetsPath(currentTaskTargetPaths, "lib/main.dart")
	currentTaskTargetsWidgetTest := builderRuntimeTargetsPath(currentTaskTargetPaths, "test/widget_test.dart")
	currentTaskTouchesCollectionSurface := currentTaskTargetsWidgetTest || builderRuntimeTaskHasSurfaceRef(dominantTask, builderRuntimeCollectionSurfaceRef) || (!hasExplicitSurfaceTopology && (builderRuntimeTargetsPath(currentTaskTargetPaths, "lib/views/record_list_page.dart") || builderRuntimeTargetsPath(currentTaskTargetPaths, "lib/controllers/record_list_controller.dart") || builderRuntimeTargetPathsContainLikelySurface(currentTaskTargetPaths, builderRuntimeLikelyCollectionSurfaceViewPath, builderRuntimeLikelyCollectionControllerPath)))
	if (hasRepositoryScope || hasControllerScope || hasViewScope || currentTaskTargetsMain || currentTaskTargetsWidgetTest) && !hasDeleteFlow {
		builder.WriteString("Current topology does not include delete behavior. For current repository, controller, view, app wiring, and widget test files, do not introduce deleteRecord, repository delete helpers, delete buttons, swipe-to-delete affordances, confirm-delete dialogs, or delete assertions.\n")
	}
	if currentTaskTargetsMain || currentTaskTargetsWidgetTest {
		if currentTaskTargetsMain && !currentTaskTargetsWidgetTest {
			builder.WriteString("The current patch scope does not include test/widget_test.dart. Keep widget-test compatibility in mind, but do not emit widget-test edits in this patch; let the dedicated test task or later repair update test/widget_test.dart.\n")
		}
		if !hasOverviewSurface {
			builder.WriteString("Current topology does not include an overview/home flow. Do not add summary cards, overview-first navigation, HomePage/HomeController references, or widget-test assertions that expect a home summary screen.\n")
		}
		if !hasDetailSurface {
			builder.WriteString("Current topology does not include a standalone detail flow. Do not create or reference standalone detail page routes, detail-only navigation, or widget-test steps that depend on opening a separate detail page.\n")
		}
		if !hasFilterFlow {
			builder.WriteString("Current topology does not include filtering. Do not add filter state, filter chips/dropdowns, setFilter calls, filter counts, or widget-test assertions for filter behavior.\n")
		}
		if !hasOverviewSurface && hasCollectionSurface {
			builder.WriteString("Because the current topology boots directly into the collection surface, widget tests should assert collection-root behavior instead of overview-only copy such as home summary headers or recent-record sections.\n")
		}
	}
	if currentTaskTargetsMain {
		builder.WriteString("For lib/main.dart, wire the actual current surfaces into the app entry and remove every seed placeholder. Do not leave AppFactorySeedHomePage or 'Seed workspace ready' in the final file.\n")
		builder.WriteString("Keep exactly one MaterialApp in lib/main.dart. Do not return a second MaterialApp from helper widgets, page wrappers, or state widgets inside the app entry flow.\n")
		builder.WriteString("If current workspace already defines view widgets under lib/views/*.dart, import and wire those existing widgets. Do not declare replacement placeholder widgets such as local HomePage or MyHomePage inside lib/main.dart.\n")
		builder.WriteString("For lib/main.dart, derive page constructor arguments and callback signatures from the current workspace files in context. Do not invent named parameters or callbacks that the referenced page/controller/repository files do not export.\n")
		if !hasOverviewSurface && hasCollectionSurface {
			builder.WriteString("Because the current topology has no overview/home surface but does have a collection surface, use the collection page as the app home/root entry instead of inventing a summary-first home screen.\n")
			if !hasDetailSurface {
				builder.WriteString("If lib/main.dart still passes the current collection detail callback into the collection page while standalone detail is pruned, use an async callback that returns Future<void> or direct remaining-surface navigation. Do not leave a synchronous placeholder block that returns null.\n")
				builder.WriteString("Do not leave an async no-op placeholder callback that only contains comments or empty logic for the current collection detail callback. Route the remaining mutation/navigation flow explicitly.\n")
			}
		}
	}
	if currentTaskTouchesCollectionSurface && !hasFilterFlow {
		builder.WriteString("For current collection-surface files and widget tests, keep the list as an unfiltered collection when filtering is absent from the current topology. Do not emit selectedFilter/setFilter, filter keys, or filter-specific empty/count copy.\n")
	}
	hasWidgetTestTarget := containsTargetPath(request.RoundInput.TaskBundle, "test/widget_test.dart")
	if hasWidgetTestTarget {
		builder.WriteString("Any rewrite of test/widget_test.dart must remain valid Dart test code. Keep imports referenced, keep a valid top-level structure, and wrap testWidgets calls inside void main() { ... } instead of emitting a bare testWidgets call.\n")
		builder.WriteString("For test/widget_test.dart, interact only with widgets, text, keys, routes, and helper APIs that already exist in the current workspace. Do not invent Key(...) values, fake helper extensions, placeholder routes, or repository/test helpers just to make the scenario compile.\n")
		builder.WriteString("Before calling tester.enterText, confirm the finder resolves to an actual text input in the current view tree such as TextField, TextFormField, or EditableText. Do not call enterText on DropdownButtonFormField, labels, chips, buttons, or other non-text-input widgets; use tap plus visible menu text for dropdown selection instead.\n")
		builder.WriteString("When repairing widget tests after Bad state: No element, missing finder, or gesture lookup failures, align the test steps with the real rendered controls from the current view files. Prefer visible text or existing keys from the workspace over guessed keys.\n")
		builder.WriteString("For test/widget_test.dart, keep imports and helper declarations minimal: remove unused imports and unreferenced private helpers, and do not add wrapper extensions around tester.pumpAndSettle(). Call await tester.pumpAndSettle() directly unless an existing referenced helper already exists in the workspace.\n")
	}
	if request.RepairOnly {
		builder.WriteString("Schema repair mode: preserve the same implementation intent as the previous invalid response. Repair only JSON shape and required fields; do not introduce new product behavior.\n")
		builder.WriteString("If the previous response body was empty or truncated, emit a fresh complete compact JSON patch from current workspace context instead of repeating the empty response.\n")
		builder.WriteString("If a replace_block was missing new_content, either supply new_content for that exact replacement or rewrite the file with write_file and complete final content. Never emit replace_block without new_content.\n")
		builder.WriteString("If the previous patch failed because replace_block matched zero or multiple regions, do not emit another ambiguous replace_block. Rewrite the full target file with write_file, or use old_content that uniquely matches exactly one existing block.\n")
		if currentTaskTargetsMain {
			builder.WriteString("For lib/main.dart repair, preserve a single app shell: initialize the shared repository in main() when needed, pass it into the root app widget if required, and keep exactly one MaterialApp in the file.\n")
			builder.WriteString("For lib/main.dart repair, do not declare local fallback pages like MyHomePage or a replacement local HomePage when imported surfaces already exist under lib/views/*.dart.\n")
			builder.WriteString("For lib/main.dart repair, derive every route/page constructor from the current workspace files in context. Do not invent named parameters such as recordRepository, and do not omit required parameters such as homeController when wiring the current pages.\n")
		}
		if dartSyntaxRepair {
			builder.WriteString("Dart syntax repair mode is active. Rewrite each directly failing Dart file from clean workspace context with complete write_file content instead of trying to preserve corrupted tokens from the rejected patch.\n")
			builder.WriteString("Outside string literals and comments, use ASCII punctuation only. Never emit fullwidth punctuation, duplicated dots, stray spaces around member access, broken generic declarations, or line breaks inside identifiers or type names.\n")
			builder.WriteString("Before sending, self-check that every identifier is contiguous, every member access uses a single '.', callback names stay exact (for example onPressed/onTap/onChanged), and generics remain balanced forms such as State<RecordFormPage>.\n")
		}
		if trimmedErr := strings.TrimSpace(request.PreviousErr); trimmedErr != "" {
			builder.WriteString("Previous parser error: ")
			builder.WriteString(trimmedErr)
			builder.WriteString("\n")
		}
		if trimmedBody := strings.TrimSpace(request.PreviousBody); trimmedBody != "" && !dartSyntaxRepair {
			builder.WriteString("Previous invalid response:\n")
			builder.WriteString(trimmedBody)
			builder.WriteString("\n")
		}
	}
	builder.WriteString("After applying your patch, every Dart import and referenced widget/controller/repository class must still resolve.\n")
	if builderRuntimeFlutterWithValuesEnabled() {
		builder.WriteString("Do not introduce Flutter or Dart APIs that are already deprecated in current stable toolchains. Toolchain capability note: withValues() is enabled for this run, but only use it when the target file already compiles with that API and do not rewrite unrelated code just to introduce it.\n")
	} else {
		builder.WriteString("Do not introduce Flutter or Dart APIs that are version-sensitive across stable toolchains. For color alpha updates, keep withOpacity() unless the current workspace already compiles with withValues(); do not introduce new withValues() calls when compatibility is uncertain.\n")
	}
	builder.WriteString("Keep Flutter/Dart framework identifiers and syntax tokens byte-for-byte exact. Do not translate, localize, or mix non-ASCII characters into identifiers such as Widget, BuildContext, ChangeNotifier, CrossAxisAlignment, Future, const, or enum/value names. Restrict non-ASCII text to user-facing string literals and comments.\n")
	builder.WriteString("Treat flutter analyze as a hard acceptance gate: return code that is free of deprecated_member_use and comparable analyzer findings, not just syntax errors.\n")
	builder.WriteString("Goal summary: ")
	builder.WriteString(strings.TrimSpace(request.Run.GoalSummary))
	builder.WriteString("\n")
	if strings.TrimSpace(domainModelContext) != "" {
		builder.WriteString("Prepare domain model JSON:\n")
		builder.WriteString(domainModelContext)
		builder.WriteString("\n")
		builder.WriteString("Treat prepare domain model as the source of truth for final entity and summary fields. If a field or workflow concept is not present there, do not recreate it in controllers, copy, repositories, widgets, or tests.\n")
		builder.WriteString("When translating prepare domain model fields into Dart source, keep public Dart identifiers idiomatic: type names stay UpperCamelCase, fields/getters/parameters stay lowerCamelCase, and only serialized storage keys may remain snake_case.\n")
		builder.WriteString("Do not rename exported types in lib/models/*.dart unless every dependent controller, repository, widget, lib/main.dart callsite, and test in the current file context is updated in the same patch.\n")
	}
	if strings.TrimSpace(exportedModelContext) != "" {
		builder.WriteString("Current exported Dart model types:\n")
		builder.WriteString(exportedModelContext)
		builder.WriteString("\n")
		builder.WriteString("Keep these exported model type names exactly as they appear above unless the same patch also updates every dependent import, constructor call, repository, widget, lib/main.dart callsite, and test.\n")
		builder.WriteString("Do not delete currently exported enums, classes, or mixins from lib/models/*.dart during domain remaps. If dependents still reference a legacy export such as RecordStatus, keep a compatibility definition until those dependents are updated in the same patch.\n")
		if len(removableLegacyExports) > 0 {
			builder.WriteString("Legacy exported helper types that may be removed when prepare domain model no longer needs them: ")
			builder.WriteString(strings.Join(removableLegacyExports, ", "))
			builder.WriteString(". Remove them only when the same patch also updates every dependent file to the final domain schema.\n")
		}
	}
	if strings.TrimSpace(modelAPISnapshot) != "" {
		builder.WriteString("Current Dart model API snapshot:\n")
		builder.WriteString(modelAPISnapshot)
		builder.WriteString("\n")
		builder.WriteString("Use this snapshot as the compatibility baseline for model rewrites. Do not invent removed fields or enum cases that are absent from the current snapshot.\n")
	}
	if len(forbiddenSchemaTokens) > 0 {
		builder.WriteString("Current forbidden schema tokens:\n")
		builder.WriteString(strings.Join(forbiddenSchemaTokens, ", "))
		builder.WriteString("\n")
		builder.WriteString("These tokens are invalid for the current domain model and must not appear in rewritten models, controllers, repositories, widgets, copy, or tests unless the live file context still defines them.\n")
	} else if len(request.Run.TemplateReferenceFiles) > 0 {
		builder.WriteString("Current forbidden schema tokens: <none yet from workspace files>\n")
		builder.WriteString("In early reference-template tasks, the workspace may not contain model files yet. In that case, rely on the prepare domain model JSON as the field authority and do not recreate generic schema concepts that are absent from it.\n")
	}
	if notes := strings.TrimSpace(string(request.Run.HumanNotes)); notes != "" {
		builder.WriteString("Human notes JSON: ")
		builder.WriteString(notes)
		builder.WriteString("\n")
	}
	if failureContext != "" {
		builder.WriteString("Validation failure context:\n")
		builder.WriteString(failureContext)
		builder.WriteString("\n")
		if preservedSymbols, err := builderRuntimeCurrentFailureSymbolDefinitions(request.Run.WorkspacePath, request.Run.TaskBundle, builderRuntimeUndefinedTypeLikeSymbolsFromFailureContext(failureContext)); err != nil {
			return "", err
		} else if len(preservedSymbols) > 0 {
			builder.WriteString("The current workspace already defines these failure-referenced Dart helper symbols and this patch must preserve or restore them: ")
			builder.WriteString(strings.Join(preservedSymbols, ", "))
			builder.WriteString(". Do not rewrite a file in a way that removes one of these helpers unless the same patch recreates it in the final workspace.\n")
		}
		if len(directFailurePaths) > 0 {
			directFailurePathsJSON, _ := json.Marshal(directFailurePaths)
			builder.WriteString("Current directly failing files JSON: ")
			builder.Write(directFailurePathsJSON)
			builder.WriteString("\n")
			builder.WriteString("Treat Current directly failing files JSON as the first files to fix. If any listed file still has analyzer or test failures, do not stop after editing only shared model files.\n")
			builder.WriteString("Current directly failing files JSON is a hard coverage contract for analyze/test repair. Your patch will be rejected unless it includes at least one concrete operation for every listed file path.\n")
			builder.WriteString("When a directly failing file is a view, lib/main.dart, or test/widget_test.dart, prefer a full-file write_file for that file instead of leaving it untouched while only changing shared dependencies.\n")
		}
		if missingPackages := builderRuntimeMissingPackageDependenciesFromFailureContext(failureContext); len(missingPackages) > 0 {
			builder.WriteString("Current validation failures show undeclared package imports or dependencies: ")
			builder.WriteString(strings.Join(missingPackages, ", "))
			builder.WriteString(". Do not introduce new package imports or APIs from packages absent from current pubspec.yaml. Rewrite the current files to use only Flutter/Dart SDK APIs and the local workspace types already in context. Only edit pubspec.yaml to add a dependency when pubspec.yaml is explicitly included in both Allowed paths JSON and current validation-repair target_paths JSON; otherwise remove the stray package import and related API usage instead of working around it.\n")
		}
		if strings.Contains(failureContext, "debugPrint") {
			builder.WriteString("When a current Dart file still calls Flutter framework helpers such as debugPrint(...), keep an import that actually exposes every referenced helper. Do not leave a flutter/foundation.dart show-list import that keeps ChangeNotifier but omits debugPrint while debugPrint(...) remains in the file.\n")
		}
		if strings.Contains(failureContext, "unused_import") || strings.Contains(failureContext, "unused_element") {
			builder.WriteString("Current validation failure is closure-only analyzer noise such as unused_import/unused_element. Prefer the smallest cleanup that removes unused imports, duplicate placeholder helpers, and other unreferenced private declarations. Do not rewrite unrelated architecture, navigation, or data flow when the failure context is only closure noise.\n")
			builder.WriteString("When the only remaining analyzer findings in current views/tests are unused imports, delete the now-unused import lines directly and keep the existing view/test structure unchanged.\n")
		}
		if trimmedPreviousErr := strings.TrimSpace(request.PreviousErr); trimmedPreviousErr != "" {
			builder.WriteString("Previous failed patch error: ")
			builder.WriteString(trimmedPreviousErr)
			builder.WriteString("\n")
		}
		if trimmedPreviousBody := strings.TrimSpace(request.PreviousBody); trimmedPreviousBody != "" && !dartSyntaxRepair {
			builder.WriteString("Previous rejected patch:\n")
			builder.WriteString(trimmedPreviousBody)
			builder.WriteString("\n")
		}
		if strings.Contains(failureContext, "patch renamed exported Dart model types") {
			builder.WriteString("The previous attempt failed because it renamed exported Dart model types. In this retry, preserve the current exported names from lib/models/*.dart exactly and repair dependent files around them instead of inventing new type names.\n")
			builder.WriteString("If the rejected patch dropped an existing exported enum or helper type, restore that export in place before remapping dependent files.\n")
			builder.WriteString("Do not add extra exported helper classes, enums, or mixins beyond the current snapshot unless the same patch also updates every dependent file to use them.\n")
			if len(removableLegacyExports) > 0 {
				builder.WriteString("Legacy helper exports absent from prepare domain model may be removed, but do not rename them into new exported types.\n")
			}
		}
		if strings.Contains(failureContext, "replace_block expects exactly one match") {
			builder.WriteString("The previous attempt failed because replace_block anchor matching was ambiguous. In this retry, do not reuse a short anchor like a repeated method signature. Rewrite the whole file with write_file, or use old_content that includes the exact surrounding class-specific block and matches exactly once.\n")
		}
		if forbiddenTokens := builderRuntimeForbiddenTokensFromFailureContext(failureContext); len(forbiddenTokens) > 0 {
			builder.WriteString("The listed forbidden schema tokens must not appear anywhere in this retry patch: ")
			builder.WriteString(strings.Join(forbiddenTokens, ", "))
			builder.WriteString(". Rewrite around the current domain schema instead of reintroducing those tokens.\n")
		}
		hasViewScope := builderRuntimePathsContainPrefix(failureScopePaths, "lib/views/")
		hasControllerScope := builderRuntimePathsContainPrefix(failureScopePaths, "lib/controllers/")
		hasRepositoryScope := builderRuntimePathsContainPrefix(failureScopePaths, "lib/repositories/")
		hasTemplateScope := builderRuntimePathsContainPrefix(failureScopePaths, "lib/template/")
		hasMainScope := builderRuntimeTargetsPath(failureScopePaths, "lib/main.dart")
		hasWidgetTestScope := builderRuntimeTargetsPath(failureScopePaths, "test/widget_test.dart")
		if relationRichWorkspace && (hasControllerScope || hasViewScope || hasRepositoryScope || hasMainScope || hasWidgetTestScope) {
			builder.WriteString("For current relation-rich controller/view/app/test repairs, preserve the current constructor-injected workspace architecture. Import models from the existing local lib/models/*.dart files and keep repository/controller/view APIs aligned to the current workspace instead of introducing new global-state packages or ad hoc helper layers.\n")
			builder.WriteString("Do not introduce package:provider/provider.dart, package:uuid/uuid.dart, MultiProvider, ChangeNotifierProvider, Provider.of/context.read, Uuid(), seedTokens, or renamed repository helpers such as loadTasksTagLinks. Use the existing repository contract and current local model/helper names from context.\n")
			builder.WriteString("If analyzer failures name undefined methods, getters, functions, or names on controller, repository, view, app, or test files, either restore that API on the owning file or update every current caller in validation-repair target_paths to the final API in the same patch. Do not leave one-sided callsite drift such as a page that still calls a missing controller method.\n")
		}
		if weightRecordDomain {
			if hasControllerScope && builderRuntimeTaskTouchesCurrentSurfaceSlice(dominantTask, request.RoundInput.TaskBundle, currentTaskTargetPaths, builderRuntimeMutationSurfaceRef) {
				builder.WriteString("For current mutation-surface controller files in the weight-record domain, rebuild the controller around weight, recordedAt, and note editing only. Do not recreate category pickers, categories lists, categoryController, setCategory, titleController, or status workflows.\n")
			}
			if hasControllerScope && builderRuntimeTaskTouchesCurrentSurfaceSlice(dominantTask, request.RoundInput.TaskBundle, currentTaskTargetPaths, builderRuntimeCollectionSurfaceRef) {
				builder.WriteString("For current collection-surface controller files when the domain model has no status field, expose the record list without RecordListFilter or any status-based filtering buckets.\n")
			}
			if (hasViewScope || hasControllerScope) && builderRuntimeTaskTouchesCurrentSurfaceSlice(dominantTask, request.RoundInput.TaskBundle, currentTaskTargetPaths, builderRuntimeMutationSurfaceRef) {
				builder.WriteString("For current mutation-surface files in the weight-record domain, keep only the fields required by AppRecord: weight, recordedAt, and optional note. The controller and page should manage a numeric weight input, a selectedDate/recordedAt picker, and note text, then construct AppRecord(recordId, weight, recordedAt, note).\n")
			}
			if hasViewScope && builderRuntimeTaskTouchesCurrentSurfaceSlice(dominantTask, request.RoundInput.TaskBundle, currentTaskTargetPaths, builderRuntimeOverviewSurfaceRef, builderRuntimeCollectionSurfaceRef, builderRuntimeInspectionSurfaceRef) {
				builder.WriteString("For current overview/collection/inspection surface files in the weight-record domain, render weight, recordedAt, note, latestWeight, recordCount, and trend. Do not keep generic task labels or workflow buckets such as title, category, status, updatedAt, totalCount, inboxCount, inProgressCount, or doneCount.\n")
			}
			if hasRepositoryScope || hasMainScope {
				builder.WriteString("For current repository and app wiring files, use recordId as the record identifier and recordedAt as the time field. Remove id/updatedAt assumptions from persistence, routing, and page launch code.\n")
			}
			if hasTemplateScope {
				appendBuilderRuntimeOpenLiteWeightRecordTemplateCopyFailureGuidance(&builder)
			}
		}
		if bookkeepingDomain {
			if hasRepositoryScope {
				builder.WriteString("For current bookkeeping repository files, store the entries collection with a Hive box/value shape that matches the actual persisted payload. Do not declare Box<Map<String, dynamic>> if the entries key stores a whole List<Map<String, dynamic>>. Keep loadEntries/saveEntries generic types aligned so _box.get/_box.put do not pass a list where Hive expects a single map. For the entries default value, use a flat empty list like <Map<String, dynamic>>[]; never emit nested defaults like <List<Map<String, dynamic>>>[] or any List<List<Map<String, dynamic>>> shape.\n")
			}
			if hasViewScope && builderRuntimeTaskTouchesCurrentSurfaceSlice(dominantTask, request.RoundInput.TaskBundle, currentTaskTargetPaths, builderRuntimeOverviewSurfaceRef, builderRuntimeCollectionSurfaceRef) {
				builder.WriteString("For current bookkeeping overview/collection surface files, BookkeepingEntry uses occurredOn and nullable note. Do not reference entry.date. When rendering note text, handle null safely and fall back to category. Do not reference summary.entryCount unless Summary currently defines it; if the UI needs a count, derive it from the current entries list or add the summary field only if the same patch updates the summary model and all dependents consistently.\n")
			}
		}
		if hasWidgetTestScope && bookkeepingDomain {
			builder.WriteString("For test/widget_test.dart in the bookkeeping domain, import the concrete entry model types you use, and do not use BookkeepingEntry as a type argument unless the imported symbol is actually a Dart type in the current workspace. Keep the test aligned with the final app/repository API instead of inventing missing helper types.\n")
			if undefinedSymbols := builderRuntimeUndefinedTypeLikeSymbolsFromFailureContext(failureContext); len(undefinedSymbols) > 0 {
				builder.WriteString("If test/widget_test.dart currently fails because a Dart helper symbol is undefined, first restore or preserve the existing helper definitions for: ")
				builder.WriteString(strings.Join(undefinedSymbols, ", "))
				builder.WriteString(". Do not replace the whole test or repository file with a simpler scaffold that drops those helpers.\n")
				builder.WriteString("If the test still instantiates one of those helpers, the same patch must define that concrete helper class in the final workspace instead of leaving the reference unresolved.\n")
			}
		}
		builder.WriteString("Prioritize fixing the concrete files named in validation failure context before revisiting already-remapped model files. If analyzer failures name controllers, repositories, views, or tests, update those dependents in the same patch until the final schema is consistent.\n")
	}
	builder.WriteString("Attempt: ")
	builder.WriteString(fmt.Sprintf("%d\n", request.RoundInput.Attempt))
	builder.WriteString("Primary task route: ")
	builder.WriteString(string(request.Route.TaskType))
	builder.WriteString(" via ")
	builder.WriteString(request.Route.RouteSource)
	builder.WriteString("\n")
	switch appruns.NormalizeTaskRouteHint(string(dominantTask.RouteHint)) {
	case appruns.TaskRouteHintStrongModel, appruns.TaskRouteHintUpgradeModel:
		builder.WriteString("Current allocation route_hint requires domain-specific implementation. Replace generic seed fields with the domain fields implied by task target_paths, semantic refs, human notes, and acceptance checks; do not stop at branding-only edits.\n")
	}
	if len(focusTasks) > 1 {
		builder.WriteString("Current focus tasks JSON: ")
		builder.Write(focusTasksJSON)
		builder.WriteString("\n")
		builder.WriteString("Treat current focus tasks as a coordinated implementation slice. If acceptance checks depend on domain-field remaps, branding, summary wiring, and flow wiring together, do not stop after satisfying only one focus task while leaving later focus tasks on default seed fields.\n")
	}
	builder.WriteString("Current allocation task JSON: ")
	builder.Write(dominantTaskJSON)
	builder.WriteString("\n")
	if (request.Route.TaskType == appruns.BuilderRuntimeTaskTypeAnalyzeRepair || request.Route.TaskType == appruns.BuilderRuntimeTaskTypeTestRepair) && len(currentTaskTargetPaths) > 0 {
		currentTaskTargetPathsJSON, _ := json.Marshal(currentTaskTargetPaths)
		builder.WriteString("Current validation-repair target_paths JSON: ")
		builder.Write(currentTaskTargetPathsJSON)
		builder.WriteString("\n")
		builder.WriteString("Do not stop after changing only shared model files if direct analyzer or test failures still name other files in current validation-repair target_paths. Update the named repository, controller, view, lib/main.dart, and test files in the same patch until the final API is consistent across every directly failing target path.\n")
		builder.WriteString("For analyze/test repair, treat the directly failing subset of current validation-repair target_paths as mandatory outputs for this patch, not optional follow-up work.\n")
	}
	builder.WriteString("Current round acceptance checks JSON: ")
	builder.Write(checksJSON)
	builder.WriteString("\n")
	builder.WriteString("Allowed paths JSON: ")
	builder.Write(allowedJSON)
	builder.WriteString("\n")
	builder.WriteString("Current file context:\n")
	builder.WriteString(files)
	return builder.String(), nil
}

func shouldUseCompactBuilderRuntimeModelCreatePrompt(request BuilderRuntimePatchRequest, currentTaskTargetPaths []string) bool {
	if request.RepairOnly {
		return false
	}
	switch request.Route.TaskType {
	case appruns.BuilderRuntimeTaskTypeAnalyzeRepair, appruns.BuilderRuntimeTaskTypeTestRepair, appruns.BuilderRuntimeTaskTypeClosureRepair:
		return false
	}
	if len(currentTaskTargetPaths) != 1 || !builderRuntimeTargetsModelFiles(currentTaskTargetPaths) {
		return false
	}
	targetPath := filepath.ToSlash(strings.TrimSpace(currentTaskTargetPaths[0]))
	if targetPath == "" || strings.TrimSpace(request.Run.WorkspacePath) == "" {
		return false
	}
	return builderRuntimeTargetFileMissing(request.Run.WorkspacePath, targetPath)
}

func shouldUseCompactBuilderRuntimeSummaryModelCreatePrompt(request BuilderRuntimePatchRequest, currentTaskTargetPaths []string) bool {
	if !shouldUseCompactBuilderRuntimeModelCreatePrompt(request, currentTaskTargetPaths) {
		return false
	}
	if !builderRuntimeTargetsOnlyPath(currentTaskTargetPaths, "lib/models/dashboard_summary.dart") {
		return false
	}
	return containsTargetPath(request.RoundInput.TaskBundle, "lib/models/record.dart")
}

func shouldUseCompactBuilderRuntimeOverviewBindPrompt(request BuilderRuntimePatchRequest, dominantTask appruns.TaskBundleItem, currentTaskTargetPaths []string) bool {
	if request.RepairOnly {
		return false
	}
	switch request.Route.TaskType {
	case appruns.BuilderRuntimeTaskTypeAnalyzeRepair, appruns.BuilderRuntimeTaskTypeTestRepair, appruns.BuilderRuntimeTaskTypeClosureRepair:
		return false
	}
	if len(currentTaskTargetPaths) != 1 {
		return false
	}
	normalizedTargetPath := filepath.ToSlash(strings.TrimSpace(currentTaskTargetPaths[0]))
	if !strings.HasPrefix(normalizedTargetPath, "lib/views/") || !strings.HasSuffix(normalizedTargetPath, ".dart") {
		return false
	}
	if !builderRuntimeTaskHasSurfaceRef(dominantTask, builderRuntimeOverviewSurfaceRef) && normalizedTargetPath != "lib/views/home_page.dart" {
		return false
	}
	if strings.TrimSpace(request.Run.WorkspacePath) == "" {
		return false
	}
	if !builderRuntimeTaskTouchesGenericOverviewSlice(dominantTask, request.RoundInput.TaskBundle, currentTaskTargetPaths) {
		return false
	}
	return builderRuntimeTargetFileMissing(request.Run.WorkspacePath, normalizedTargetPath)
}

func builderRuntimeTargetFileMissing(workspacePath, targetPath string) bool {
	workspacePath = strings.TrimSpace(workspacePath)
	targetPath = filepath.ToSlash(strings.TrimSpace(targetPath))
	if workspacePath == "" || targetPath == "" {
		return false
	}
	_, err := os.Stat(filepath.Join(workspacePath, filepath.FromSlash(targetPath)))
	return os.IsNotExist(err)
}

func buildCompactBuilderRuntimeModelCreatePrompt(request BuilderRuntimePatchRequest, files, domainModelContext string, dominantTask appruns.TaskBundleItem, currentTaskTargetPaths []string, checksJSON, allowedJSON []byte) string {
	targetPath := filepath.ToSlash(strings.TrimSpace(currentTaskTargetPaths[0]))
	taskSummary := map[string]any{
		"task_id":      strings.TrimSpace(dominantTask.TaskID),
		"title":        strings.TrimSpace(dominantTask.Title),
		"task_type":    string(dominantTask.EffectiveTaskType()),
		"target_paths": currentTaskTargetPaths,
	}
	if routeHint := strings.TrimSpace(string(appruns.NormalizeTaskRouteHint(string(dominantTask.RouteHint)))); routeHint != "" {
		taskSummary["route_hint"] = routeHint
	}
	if dominantTask.AllocationTransition != nil {
		if len(dominantTask.AllocationTransition.EntityRefs) > 0 {
			taskSummary["entity_refs"] = append([]string(nil), dominantTask.AllocationTransition.EntityRefs...)
		}
		if len(dominantTask.AllocationTransition.OwnedPaths) > 0 {
			taskSummary["owned_paths"] = append([]string(nil), dominantTask.AllocationTransition.OwnedPaths...)
		}
		if len(dominantTask.AllocationTransition.SuccessEvidence) > 0 {
			taskSummary["success_evidence"] = append([]string(nil), dominantTask.AllocationTransition.SuccessEvidence...)
		}
	}
	taskJSON, _ := json.Marshal(taskSummary)
	relationRichModelProfile := builderRuntimeRelationRichModelProfileForContext(request.Run.WorkspacePath, currentTaskTargetPaths)
	var builder strings.Builder
	builder.WriteString("Return compact JSON only. No markdown.\n")
	builder.WriteString("Top-level keys: patch_id, operations.\n")
	builder.WriteString("Allowed operation types: write_file, replace_block, delete_file.\n")
	builder.WriteString("Emit exactly one write_file operation for ")
	builder.WriteString(targetPath)
	builder.WriteString(". Do not touch any other path.\n")
	builder.WriteString("This is a single-file model-create task. Do not emit controller, repository, view, app-entry, Android, or test edits in this patch.\n")
	builder.WriteString("The target model file is being created from scratch. Use current file context and any reference template for structure only, but follow the prepare domain model for final fields and names.\n")
	builder.WriteString("For lib/models/*.dart targets, emit plain Dart model code with final fields, constructors, copyWith, and serialization helpers that match the current workspace contract. Do not keep generic seed fields absent from the final domain model.\n")
	builder.WriteString("Keep public Dart identifiers idiomatic: type names stay UpperCamelCase, fields/getters/parameters stay lowerCamelCase, and only serialized storage keys may remain snake_case when needed.\n")
	if strings.TrimSpace(request.Run.TemplateReferenceFiles[targetPath]) != "" {
		builder.WriteString("Reference-template mode is active. The reference template is structural guidance only; do not copy generic seed schema fields verbatim.\n")
	}
	if relationRichModelProfile != "" {
		builder.WriteString(builderRuntimeRelationRichModelPromptGuidance(relationRichModelProfile))
	}
	if containsTargetPath(request.RoundInput.TaskBundle, "lib/models/record.dart") && containsTargetPath(request.RoundInput.TaskBundle, "lib/models/dashboard_summary.dart") {
		appendBuilderRuntimeOpenLiteSchemaRemapGuidance(&builder)
	}
	builder.WriteString("Goal summary: ")
	builder.WriteString(strings.TrimSpace(request.Run.GoalSummary))
	builder.WriteString("\n")
	if notes := strings.TrimSpace(string(request.Run.HumanNotes)); notes != "" {
		builder.WriteString("Human notes JSON: ")
		builder.WriteString(notes)
		builder.WriteString("\n")
	}
	if strings.TrimSpace(domainModelContext) != "" {
		builder.WriteString("Prepare domain model JSON:\n")
		builder.WriteString(domainModelContext)
		builder.WriteString("\n")
		builder.WriteString("Treat prepare domain model as the source of truth for final entity and summary fields. If a field or workflow concept is not present there, do not recreate it in the new model.\n")
	}
	switch appruns.NormalizeTaskRouteHint(string(dominantTask.RouteHint)) {
	case appruns.TaskRouteHintStrongModel, appruns.TaskRouteHintUpgradeModel:
		builder.WriteString("Current allocation route_hint requires a domain-specific model implementation. Replace generic seed fields with the domain fields implied by the current task and domain model.\n")
	}
	builder.WriteString("Current task summary JSON: ")
	builder.Write(taskJSON)
	builder.WriteString("\n")
	builder.WriteString("Current round acceptance checks JSON: ")
	builder.Write(checksJSON)
	builder.WriteString("\n")
	builder.WriteString("Allowed paths JSON: ")
	builder.Write(allowedJSON)
	builder.WriteString("\n")
	builder.WriteString("Current file context:\n")
	builder.WriteString(files)
	return builder.String()
}

func buildCompactBuilderRuntimeSummaryModelCreatePrompt(request BuilderRuntimePatchRequest, files, domainModelContext string, dominantTask appruns.TaskBundleItem, currentTaskTargetPaths []string, checksJSON, allowedJSON []byte) (string, error) {
	targetPath := filepath.ToSlash(strings.TrimSpace(currentTaskTargetPaths[0]))
	forbiddenSchemaTokens, err := builderRuntimeForbiddenSchemaTokens(request.Run.WorkspacePath)
	if err != nil {
		return "", err
	}
	taskSummary := map[string]any{
		"task_id":      strings.TrimSpace(dominantTask.TaskID),
		"title":        strings.TrimSpace(dominantTask.Title),
		"task_type":    string(dominantTask.EffectiveTaskType()),
		"target_paths": currentTaskTargetPaths,
	}
	if routeHint := strings.TrimSpace(string(appruns.NormalizeTaskRouteHint(string(dominantTask.RouteHint)))); routeHint != "" {
		taskSummary["route_hint"] = routeHint
	}
	if dominantTask.AllocationTransition != nil {
		if len(dominantTask.AllocationTransition.EntityRefs) > 0 {
			taskSummary["entity_refs"] = append([]string(nil), dominantTask.AllocationTransition.EntityRefs...)
		}
		if len(dominantTask.AllocationTransition.OwnedPaths) > 0 {
			taskSummary["owned_paths"] = append([]string(nil), dominantTask.AllocationTransition.OwnedPaths...)
		}
		if len(dominantTask.AllocationTransition.SuccessEvidence) > 0 {
			taskSummary["success_evidence"] = append([]string(nil), dominantTask.AllocationTransition.SuccessEvidence...)
		}
	}
	taskJSON, _ := json.Marshal(taskSummary)
	relationRichModelProfile := builderRuntimeRelationRichModelProfileForContext(request.Run.WorkspacePath, currentTaskTargetPaths)
	var builder strings.Builder
	builder.WriteString("Return compact JSON only. No markdown.\n")
	builder.WriteString("Top-level keys: patch_id, operations.\n")
	builder.WriteString("Allowed operation types: write_file, replace_block, delete_file.\n")
	builder.WriteString("Emit exactly one write_file operation for ")
	builder.WriteString(targetPath)
	builder.WriteString(". Do not touch any other path.\n")
	builder.WriteString("This is a single-file summary-model create task. Rebuild the final DashboardSummary model from scratch around the current domain summary fields and current record model context already present in workspace files.\n")
	builder.WriteString("Do not emit controller, repository, view, app-entry, Android, or test edits in this patch.\n")
	builder.WriteString("Keep the exported model type named DashboardSummary. Emit plain Dart model code with final fields, constructors, copyWith, and serialization helpers only.\n")
	builder.WriteString("Use aggregate scalar summary fields only. Do not embed record lists, controller state, repository calls, widgets, or navigation logic in dashboard_summary.dart.\n")
	builder.WriteString("Do not introduce generic seed summary fields such as totalCount, inboxCount, inProgressCount, doneCount, status, or updatedAt unless the current domain model and file context explicitly require them.\n")
	if strings.TrimSpace(request.Run.TemplateReferenceFiles[targetPath]) != "" {
		builder.WriteString("Reference-template mode is active. The reference template is structural guidance only; keep current workspace naming and domain fields from context instead of copying generic seed summary schema verbatim.\n")
	}
	if relationRichModelProfile != "" {
		builder.WriteString(builderRuntimeRelationRichModelPromptGuidance(relationRichModelProfile))
	}
	if slices.Contains(forbiddenSchemaTokens, "totalCount") {
		builder.WriteString("Current summary model does not define totalCount. Do not add totalCount or count-bucket seed metrics unless the current domain model explicitly restores them.\n")
	}
	if slices.Contains(forbiddenSchemaTokens, "RecordStatus") || slices.Contains(forbiddenSchemaTokens, ".status") {
		builder.WriteString("Current domain model does not define a status workflow for DashboardSummary. Do not add status buckets or RecordStatus-derived summary fields.\n")
	}
	builder.WriteString("Goal summary: ")
	builder.WriteString(strings.TrimSpace(request.Run.GoalSummary))
	builder.WriteString("\n")
	if notes := strings.TrimSpace(string(request.Run.HumanNotes)); notes != "" {
		builder.WriteString("Human notes JSON: ")
		builder.WriteString(notes)
		builder.WriteString("\n")
	}
	if strings.TrimSpace(domainModelContext) != "" {
		builder.WriteString("Prepare domain model JSON:\n")
		builder.WriteString(domainModelContext)
		builder.WriteString("\n")
		builder.WriteString("Treat prepare domain model as the source of truth for final summary fields. If a field or workflow concept is not present there, do not recreate it in the new summary model.\n")
	}
	if len(forbiddenSchemaTokens) > 0 {
		builder.WriteString("Current forbidden schema tokens:\n")
		builder.WriteString(strings.Join(forbiddenSchemaTokens, ", "))
		builder.WriteString("\n")
		builder.WriteString("These tokens are invalid for the current domain model and must not appear in the new summary model unless the current file context still defines them.\n")
	}
	switch appruns.NormalizeTaskRouteHint(string(dominantTask.RouteHint)) {
	case appruns.TaskRouteHintStrongModel, appruns.TaskRouteHintUpgradeModel:
		builder.WriteString("Current allocation route_hint requires a domain-specific summary model. Replace generic seed metrics with the domain fields implied by the current task and domain model.\n")
	}
	builder.WriteString("Current task summary JSON: ")
	builder.Write(taskJSON)
	builder.WriteString("\n")
	builder.WriteString("Current round acceptance checks JSON: ")
	builder.Write(checksJSON)
	builder.WriteString("\n")
	builder.WriteString("Allowed paths JSON: ")
	builder.Write(allowedJSON)
	builder.WriteString("\n")
	builder.WriteString("Current file context:\n")
	builder.WriteString(files)
	return builder.String(), nil
}

func buildCompactBuilderRuntimeOverviewBindPrompt(request BuilderRuntimePatchRequest, files, domainModelContext string, dominantTask appruns.TaskBundleItem, currentTaskTargetPaths []string, checksJSON, allowedJSON []byte) (string, error) {
	targetPath := filepath.ToSlash(strings.TrimSpace(currentTaskTargetPaths[0]))
	forbiddenSchemaTokens, err := builderRuntimeForbiddenSchemaTokens(request.Run.WorkspacePath)
	if err != nil {
		return "", err
	}
	taskSummary := map[string]any{
		"task_id":      strings.TrimSpace(dominantTask.TaskID),
		"title":        strings.TrimSpace(dominantTask.Title),
		"task_type":    string(dominantTask.EffectiveTaskType()),
		"target_paths": currentTaskTargetPaths,
	}
	if routeHint := strings.TrimSpace(string(appruns.NormalizeTaskRouteHint(string(dominantTask.RouteHint)))); routeHint != "" {
		taskSummary["route_hint"] = routeHint
	}
	if dominantTask.AllocationTransition != nil {
		if len(dominantTask.AllocationTransition.SurfaceRefs) > 0 {
			taskSummary["surface_refs"] = append([]string(nil), dominantTask.AllocationTransition.SurfaceRefs...)
		}
		if len(dominantTask.AllocationTransition.EntityRefs) > 0 {
			taskSummary["entity_refs"] = append([]string(nil), dominantTask.AllocationTransition.EntityRefs...)
		}
		if len(dominantTask.AllocationTransition.OwnedPaths) > 0 {
			taskSummary["owned_paths"] = append([]string(nil), dominantTask.AllocationTransition.OwnedPaths...)
		}
		if len(dominantTask.AllocationTransition.SuccessEvidence) > 0 {
			taskSummary["success_evidence"] = append([]string(nil), dominantTask.AllocationTransition.SuccessEvidence...)
		}
	}
	taskJSON, _ := json.Marshal(taskSummary)
	var builder strings.Builder
	builder.WriteString("Return compact JSON only. No markdown.\n")
	builder.WriteString("Top-level keys: patch_id, operations.\n")
	builder.WriteString("Allowed operation types: write_file, replace_block, delete_file.\n")
	builder.WriteString("Emit exactly one write_file operation for ")
	builder.WriteString(targetPath)
	builder.WriteString(". Do not touch any other path.\n")
	builder.WriteString("This is a single-file overview-surface binding task. Rebuild the final home page from scratch around the current HomeController, copy helper, and current record/dashboard summary models already present in workspace context.\n")
	builder.WriteString("Do not emit controller, repository, model, app-entry, Android, or test edits in this patch.\n")
	builder.WriteString("Prefer a complete final file with imports, widget tree, and helper methods ready to compile. Do not leave TODOs, placeholders, or seed copy.\n")
	if strings.TrimSpace(request.Run.TemplateReferenceFiles[targetPath]) != "" {
		builder.WriteString("Reference-template mode is active. The reference template is structural guidance only; keep current workspace APIs, controller calls, and domain fields from context instead of copying generic seed schema verbatim.\n")
	}
	builder.WriteString("For current overview-surface target files, keep summary cards, total-count copy, recent-record metadata, and trailing record text aligned to the exact fields exported by the current dashboard_summary.dart and current primary record model in context.\n")
	builder.WriteString("Consume the existing HomeController public API from current file context instead of inventing new repository reads, filter state, or summary fields inside the page.\n")
	if slices.Contains(forbiddenSchemaTokens, "totalCount") {
		builder.WriteString("Current summary model does not define totalCount. In current overview-surface target files, derive any overall record count from controller.records.length or other current summary fields; do not reference summary.totalCount.\n")
	}
	if slices.Contains(forbiddenSchemaTokens, ".updatedAt") {
		builder.WriteString("Current primary record model does not define updatedAt. In current overview-surface target files, use the actual time field from the current primary record model if one exists; otherwise omit updatedAt-based date text instead of inventing a generic timestamp.\n")
	}
	builder.WriteString("Goal summary: ")
	builder.WriteString(strings.TrimSpace(request.Run.GoalSummary))
	builder.WriteString("\n")
	if notes := strings.TrimSpace(string(request.Run.HumanNotes)); notes != "" {
		builder.WriteString("Human notes JSON: ")
		builder.WriteString(notes)
		builder.WriteString("\n")
	}
	if strings.TrimSpace(domainModelContext) != "" {
		builder.WriteString("Prepare domain model JSON:\n")
		builder.WriteString(domainModelContext)
		builder.WriteString("\n")
		builder.WriteString("Treat prepare domain model as the source of truth for final summary and record wording. If a field or workflow concept is not present there, do not recreate it in the home page.\n")
	}
	if len(forbiddenSchemaTokens) > 0 {
		builder.WriteString("Current forbidden schema tokens:\n")
		builder.WriteString(strings.Join(forbiddenSchemaTokens, ", "))
		builder.WriteString("\n")
		builder.WriteString("These tokens are invalid for the current domain model and must not appear in the rewritten home page unless the current file context still defines them.\n")
	}
	switch appruns.NormalizeTaskRouteHint(string(dominantTask.RouteHint)) {
	case appruns.TaskRouteHintStrongModel, appruns.TaskRouteHintUpgradeModel:
		builder.WriteString("Current allocation route_hint requires a domain-specific overview surface. Replace generic seed copy and metrics with the domain fields implied by the current task and domain model.\n")
	}
	builder.WriteString("Current task summary JSON: ")
	builder.Write(taskJSON)
	builder.WriteString("\n")
	builder.WriteString("Current round acceptance checks JSON: ")
	builder.Write(checksJSON)
	builder.WriteString("\n")
	builder.WriteString("Allowed paths JSON: ")
	builder.Write(allowedJSON)
	builder.WriteString("\n")
	builder.WriteString("Current file context:\n")
	builder.WriteString(files)
	return builder.String(), nil
}

func resolveBuilderRuntimePromptTask(tasks []appruns.TaskBundleItem, routeTaskID string) appruns.TaskBundleItem {
	routeTaskID = strings.TrimSpace(routeTaskID)
	if routeTaskID != "" {
		for _, task := range tasks {
			normalized := appruns.NormalizeTaskBundleItem(task)
			if strings.TrimSpace(normalized.TaskID) == routeTaskID {
				if normalized.EffectiveTaskType() == appruns.BuilderRuntimeTaskTypeClosureRepair {
					break
				}
				return normalized
			}
		}
	}
	return dominantBuilderRuntimeTask(tasks)
}

func builderRuntimeTargetsModelFiles(paths []string) bool {
	for _, path := range paths {
		normalized := strings.TrimSpace(filepath.ToSlash(path))
		if strings.HasPrefix(normalized, "lib/models/") {
			return true
		}
	}
	return false
}

func focusBuilderRuntimeTasks(tasks []appruns.TaskBundleItem, routeTaskID string, routeTaskType appruns.BuilderRuntimeTaskType) []appruns.TaskBundleItem {
	if len(tasks) == 0 {
		return nil
	}
	if strings.TrimSpace(routeTaskID) != "" && routeTaskType != appruns.BuilderRuntimeTaskTypeClosureRepair {
		dominant := appruns.NormalizeTaskBundleItem(resolveBuilderRuntimePromptTask(tasks, routeTaskID))
		if strings.TrimSpace(dominant.TaskID) != "" {
			return []appruns.TaskBundleItem{compactBuilderRuntimeTask(dominant)}
		}
	}
	focus := make([]appruns.TaskBundleItem, 0, len(tasks))
	for _, task := range tasks {
		normalized := appruns.NormalizeTaskBundleItem(task)
		if strings.TrimSpace(normalized.TaskID) == "" {
			continue
		}
		if normalized.EffectiveTaskType() == appruns.BuilderRuntimeTaskTypeClosureRepair {
			continue
		}
		if normalized.Category == appruns.TaskCategoryValidation && normalized.EffectiveTaskType() != appruns.BuilderRuntimeTaskTypeAnalyzeRepair && normalized.EffectiveTaskType() != appruns.BuilderRuntimeTaskTypeTestRepair {
			continue
		}
		focus = append(focus, compactBuilderRuntimeTask(normalized))
	}
	if len(focus) == 0 {
		return []appruns.TaskBundleItem{compactBuilderRuntimeTask(appruns.NormalizeTaskBundleItem(dominantBuilderRuntimeTask(tasks)))}
	}
	return focus
}

func compactBuilderRuntimeTask(task appruns.TaskBundleItem) appruns.TaskBundleItem {
	compact := task
	compact.RelatedRequirements = nil
	compact.OutputExpectations = nil
	compact.CompletionCriteria = nil
	compact.RiskNotes = nil
	return compact
}

func containsTargetPath(tasks []appruns.TaskBundleItem, want string) bool {
	want = filepath.ToSlash(strings.TrimSpace(want))
	if want == "" {
		return false
	}
	for _, task := range tasks {
		for _, targetPath := range task.TargetPaths {
			if filepath.ToSlash(strings.TrimSpace(targetPath)) == want {
				return true
			}
		}
	}
	return false
}

func builderRuntimeTargetsPath(paths []string, want string) bool {
	want = filepath.ToSlash(strings.TrimSpace(want))
	if want == "" {
		return false
	}
	for _, path := range paths {
		if filepath.ToSlash(strings.TrimSpace(path)) == want {
			return true
		}
	}
	return false
}

func builderRuntimeTargetsOnlyPath(paths []string, want string) bool {
	want = filepath.ToSlash(strings.TrimSpace(want))
	if want == "" {
		return false
	}
	hasWant := false
	for _, path := range paths {
		normalized := filepath.ToSlash(strings.TrimSpace(path))
		if normalized == "" || strings.Contains(normalized, "*") {
			continue
		}
		if normalized != want {
			return false
		}
		hasWant = true
	}
	return hasWant
}

func builderRuntimeTaskHasSurfaceRef(task appruns.TaskBundleItem, want string) bool {
	want = strings.TrimSpace(want)
	if want == "" || task.AllocationTransition == nil {
		return false
	}
	for _, surfaceRef := range task.AllocationTransition.SurfaceRefs {
		if strings.TrimSpace(surfaceRef) == want {
			return true
		}
	}
	return false
}

func builderRuntimeTaskHasAnySurfaceRef(task appruns.TaskBundleItem, wants ...string) bool {
	for _, want := range wants {
		if builderRuntimeTaskHasSurfaceRef(task, want) {
			return true
		}
	}
	return false
}

func builderRuntimeTaskBundleHasSurfaceRef(tasks []appruns.TaskBundleItem, want string) bool {
	for _, task := range tasks {
		if builderRuntimeTaskHasSurfaceRef(task, want) {
			return true
		}
	}
	return false
}

func builderRuntimeTaskBundleHasAnySurfaceRef(tasks []appruns.TaskBundleItem, wants ...string) bool {
	for _, task := range tasks {
		if builderRuntimeTaskHasAnySurfaceRef(task, wants...) {
			return true
		}
	}
	return false
}

func builderRuntimeTaskHasSemanticIntentRef(task appruns.TaskBundleItem, want string) bool {
	want = strings.TrimSpace(want)
	if want == "" || task.AllocationTransition == nil {
		return false
	}
	for _, ref := range task.AllocationTransition.SemanticIntentRefs {
		if strings.TrimSpace(ref) == want {
			return true
		}
	}
	return false
}

func builderRuntimeTaskBundleHasSemanticIntentRef(tasks []appruns.TaskBundleItem, want string) bool {
	for _, task := range tasks {
		if builderRuntimeTaskHasSemanticIntentRef(task, want) {
			return true
		}
	}
	return false
}

func builderRuntimePathsContainPrefix(paths []string, prefix string) bool {
	prefix = filepath.ToSlash(strings.TrimSpace(prefix))
	if prefix == "" {
		return false
	}
	for _, path := range paths {
		normalized := filepath.ToSlash(strings.TrimSpace(path))
		if strings.HasPrefix(normalized, prefix) {
			return true
		}
	}
	return false
}

func builderRuntimePathsContainDartPathWithin(paths []string, prefix string) bool {
	for _, path := range paths {
		if builderRuntimeIsDartPathWithin(path, prefix) {
			return true
		}
	}
	return false
}

func builderRuntimePromptLooksLikeWeightRecordDomain(workspacePath string) bool {
	recordModelContent := builderRuntimeOpenLitePrimaryRecordModelContent(workspacePath)
	if strings.TrimSpace(recordModelContent) == "" {
		return false
	}
	fields := builderRuntimeDeclaredFieldNames(recordModelContent)
	_, hasWeight := fields["weight"]
	_, hasRecordedAt := fields["recordedAt"]
	return hasWeight && hasRecordedAt
}

func builderRuntimeLooksLikeBookkeepingDomain(workspacePath string, operations []appruns.WorkspacePatchOperation) bool {
	entryModelContent, err := builderRuntimeSemanticFileContent(workspacePath, operations, "lib/models/entry.dart")
	if err != nil || strings.TrimSpace(entryModelContent) == "" {
		return false
	}
	fields := builderRuntimeDeclaredFieldNames(entryModelContent)
	_, hasOccurredOn := fields["occurredOn"]
	return hasOccurredOn
}

func builderRuntimePromptLooksLikeBookkeepingDomain(workspacePath string) bool {
	return builderRuntimeLooksLikeBookkeepingDomain(workspacePath, nil)
}

func builderRuntimeWorkspaceHasSemanticFile(workspacePath, targetPath string) bool {
	content, err := builderRuntimeSemanticFileContent(workspacePath, nil, targetPath)
	return err == nil && strings.TrimSpace(content) != ""
}

func builderRuntimePathsOverlap(left, right []string) bool {
	if len(left) == 0 || len(right) == 0 {
		return false
	}
	leftSet := make(map[string]struct{}, len(left))
	for _, path := range left {
		trimmed := filepath.ToSlash(strings.TrimSpace(path))
		if trimmed == "" {
			continue
		}
		leftSet[trimmed] = struct{}{}
	}
	for _, path := range right {
		trimmed := filepath.ToSlash(strings.TrimSpace(path))
		if trimmed == "" {
			continue
		}
		if _, ok := leftSet[trimmed]; ok {
			return true
		}
	}
	return false
}

func builderRuntimeTaskTouchesCurrentSurfaceSlice(task appruns.TaskBundleItem, taskBundle []appruns.TaskBundleItem, currentTaskTargetPaths []string, wants ...string) bool {
	if builderRuntimeTaskHasAnySurfaceRef(task, wants...) {
		return true
	}
	if len(currentTaskTargetPaths) == 0 {
		return false
	}
	for _, candidate := range taskBundle {
		normalized := appruns.NormalizeTaskBundleItem(candidate)
		if !builderRuntimeTaskHasAnySurfaceRef(normalized, wants...) {
			continue
		}
		if builderRuntimePathsOverlap(currentTaskTargetPaths, concreteTaskTargetPaths([]appruns.TaskBundleItem{normalized})) {
			return true
		}
	}
	return false
}

func builderRuntimeTaskTouchesGenericOverviewSlice(task appruns.TaskBundleItem, taskBundle []appruns.TaskBundleItem, currentTaskTargetPaths []string) bool {
	if !containsTargetPath(taskBundle, "lib/models/record.dart") || !containsTargetPath(taskBundle, "lib/models/dashboard_summary.dart") {
		return false
	}
	return builderRuntimeTaskTouchesCurrentSurfaceSlice(task, taskBundle, currentTaskTargetPaths, builderRuntimeOverviewSurfaceRef)
}

func builderRuntimeTaskTouchesGenericCollectionInspectionSlice(task appruns.TaskBundleItem, taskBundle []appruns.TaskBundleItem, currentTaskTargetPaths []string) bool {
	if !containsTargetPath(taskBundle, "lib/models/record.dart") {
		return false
	}
	return builderRuntimeTaskTouchesCurrentSurfaceSlice(task, taskBundle, currentTaskTargetPaths, builderRuntimeCollectionSurfaceRef, builderRuntimeInspectionSurfaceRef)
}

func (runner *Runner) retryBuilderRuntimeSchemaRepair(ctx context.Context, backend RunnerBackend, runID string, step ExecutionStep, run runRecord, roundInput appruns.RoundInput, route appruns.BuilderRuntimeTaskRoute, stats *appruns.BuilderRuntimeExecutionStats, modelAlias, invalidContent string, parseErr error) (*builderRuntimeExecutionResult, error) {
	result := &builderRuntimeExecutionResult{AppliedRoundInput: cloneRoundInputValue(roundInput), Stats: stats}
	if strings.TrimSpace(modelAlias) == "" {
		return result, &builderRuntimeExecutionError{
			summary:            fmt.Sprintf("builder runtime patch parse failed: %v", parseErr),
			recoverySuggestion: "tighten patch schema normalization or refine the builder-runtime prompt before rerun",
			signature:          "builder_runtime_patch_parse_failed",
			preserveWorkspace:  true,
			resumeAllowed:      false,
			policy:             repairFailurePolicy{PreserveWorkspace: true, ResumeAllowed: false, RequiresHumanReview: false, MaxRounds: 2, UsedRounds: stats.Attempts, RemainingRounds: maxInt(0, 2-stats.Attempts)},
			stats:              stats,
			wrapped:            parseErr,
		}
	}
	request := BuilderRuntimePatchRequest{
		Run:          run,
		RoundInput:   roundInput,
		Route:        route,
		ModelAliases: builderRuntimeRepairModelAliases(route.Model, modelAlias),
		RepairOnly:   true,
		PreviousErr:  parseErr.Error(),
		PreviousBody: invalidContent,
	}
	routeTargetPaths := builderRuntimeRouteTargetPaths(run.TaskBundle, route.TaskID)
	roundState := heartbeatRoundStateForStep(step)
	if err := runner.reportPatchGenerationStarted(ctx, backend, runID, step, roundInput, roundState, routeTargetPaths); err != nil {
		return result, err
	}
	var (
		response       BuilderRuntimePatchResponse
		patch          appruns.WorkspacePatch
		normalized     bool
		driftCount     int
		repairParseErr error
		parseRetries   int
		err            error
	)
	for {
		var attemptsUsed int
		response, attemptsUsed, err = runner.generateBuilderRuntimePatchWithTransientRetry(ctx, request)
		stats.Attempts += attemptsUsed
		if err != nil {
			stats.FailureReason = err.Error()
			if reportErr := runner.reportPatchGenerationFailed(ctx, backend, runID, step, roundInput, roundState, routeTargetPaths, err); reportErr != nil {
				return result, reportErr
			}
			return result, &builderRuntimeExecutionError{
				summary:            fmt.Sprintf("builder runtime schema repair request failed: %v", err),
				recoverySuggestion: "fix builder-runtime model connectivity or prompt configuration before rerun",
				signature:          "builder_runtime_model_request_failed",
				preserveWorkspace:  true,
				resumeAllowed:      false,
				policy:             repairFailurePolicy{PreserveWorkspace: true, ResumeAllowed: false, RequiresHumanReview: true, MaxRounds: 2, UsedRounds: stats.Attempts, RemainingRounds: maxInt(0, 2-stats.Attempts), TerminationReason: "builder runtime schema repair request failed"},
				stats:              stats,
				wrapped:            err,
			}
		}
		stats.SelectedModel = response.ModelAlias
		stats.ModelSequence = append(stats.ModelSequence, response.ModelAlias)
		stats.PromptTokens += response.PromptTokens
		stats.CompletionTokens += response.CompletionTokens
		stats.TotalTokens += response.TotalTokens
		appendBuilderRuntimeLog(run.LogPath, response.ModelAlias, response.Content)
		patch, normalized, driftCount, repairParseErr = normalizeBuilderRuntimePatchWithWorkspace(run.WorkspacePath, response.Content, roundInput.RoundID, routeTargetPaths)
		normalizeBuilderRuntimePatchForWorkspaceWithTaskBundleAndTargetPaths(run.WorkspacePath, roundInput.TaskBundle, routeTargetPaths, &patch)
		stats.SchemaNormalized = stats.SchemaNormalized || normalized
		stats.SchemaDriftCount += driftCount
		if repairParseErr == nil {
			break
		}
		stats.ParseFailureCount++
		stats.FailureReason = repairParseErr.Error()
		if reportErr := runner.reportPatchGenerationFailed(ctx, backend, runID, step, roundInput, roundState, routeTargetPaths, repairParseErr); reportErr != nil {
			return result, reportErr
		}
		parseRetries++
		if shouldRetryBuilderRuntimeTransientPatchParseFailure(parseRetries, response.Content, repairParseErr) {
			if waitErr := waitBuilderRuntimeTransientModelRequestRetry(ctx); waitErr != nil {
				return result, waitErr
			}
			if err := runner.reportPatchGenerationStarted(ctx, backend, runID, step, roundInput, roundState, routeTargetPaths); err != nil {
				return result, err
			}
			continue
		}
		return result, &builderRuntimeExecutionError{
			summary:            fmt.Sprintf("builder runtime patch parse failed after schema repair: %v", repairParseErr),
			recoverySuggestion: "tighten patch schema normalization or refine the builder-runtime schema-repair prompt before rerun",
			signature:          "builder_runtime_patch_parse_failed",
			preserveWorkspace:  true,
			resumeAllowed:      false,
			policy:             repairFailurePolicy{PreserveWorkspace: true, ResumeAllowed: false, RequiresHumanReview: false, MaxRounds: 2, UsedRounds: stats.Attempts, RemainingRounds: maxInt(0, 2-stats.Attempts), TerminationReason: "builder runtime schema repair parse failed"},
			stats:              stats,
			wrapped:            repairParseErr,
		}
	}
	targetPaths := routeTargetPaths
	if shouldEnforceBuilderRuntimeTargetScope(route.TaskType) {
		if scopeErr := validateBuilderRuntimeTargetScope(patch.Operations, targetPaths); scopeErr != nil {
			stats.ScopeViolationCount++
			stats.FailureReason = scopeErr.Error()
			if reportErr := runner.reportPatchGenerationFailed(ctx, backend, runID, step, roundInput, roundState, targetPaths, scopeErr); reportErr != nil {
				return result, reportErr
			}
			return result, &builderRuntimeExecutionError{
				summary:            fmt.Sprintf("builder runtime workspace patch exceeded repair target scope after schema repair: %v", scopeErr),
				recoverySuggestion: "keep analyze/test repair edits inside the scoped task target_paths for the current failure slice",
				signature:          "builder_runtime_scope_violation",
				preserveWorkspace:  true,
				resumeAllowed:      false,
				policy:             repairFailurePolicy{PreserveWorkspace: true, ResumeAllowed: false, RequiresHumanReview: false, MaxRounds: 2, UsedRounds: stats.Attempts, RemainingRounds: maxInt(0, 2-stats.Attempts), TerminationReason: "builder runtime schema repair patch exceeded target scope"},
				stats:              stats,
				wrapped:            scopeErr,
			}
		}
	}
	if semanticErr := validateBuilderRuntimeSemanticConsistency(run, patch.Operations, request.FailureContext); semanticErr != nil {
		stats.FailureReason = semanticErr.Error()
		if reportErr := runner.reportPatchGenerationFailed(ctx, backend, runID, step, roundInput, roundState, targetPaths, semanticErr); reportErr != nil {
			return result, reportErr
		}
		return result, &builderRuntimeExecutionError{
			summary:            fmt.Sprintf("builder runtime workspace patch reintroduced schema-conflicting symbols after schema repair: %v", semanticErr),
			recoverySuggestion: "align builder edits with the current model files and keep exported Dart model types plus removed generic fields consistent across dependents",
			signature:          "builder_runtime_semantic_conflict",
			preserveWorkspace:  true,
			resumeAllowed:      false,
			policy:             repairFailurePolicy{PreserveWorkspace: true, ResumeAllowed: false, RequiresHumanReview: false, MaxRounds: 2, UsedRounds: stats.Attempts, RemainingRounds: maxInt(0, 2-stats.Attempts), TerminationReason: "builder runtime schema repair patch reintroduced schema-conflicting symbols"},
			stats:              stats,
			wrapped:            semanticErr,
		}
	}
	if err := runner.reportGeneratedPatch(ctx, backend, runID, step, roundInput, roundState, &patch); err != nil {
		return result, err
	}
	applyResult, applyErr := appruns.ApplyWorkspacePatch(run.WorkspacePath, run.AllowedPaths, run.ProtectedPaths, patch)
	if applyErr != nil {
		if strings.Contains(applyErr.Error(), "outside allowed roots") || strings.Contains(applyErr.Error(), "protected") {
			stats.ScopeViolationCount++
		}
		stats.FailureReason = applyErr.Error()
		if reportErr := runner.reportPatchApplyFailed(ctx, backend, runID, step, roundInput, roundState, workspacePatchPaths(&patch), applyErr); reportErr != nil {
			return result, reportErr
		}
		return result, &builderRuntimeExecutionError{
			summary:            fmt.Sprintf("builder runtime workspace patch apply failed after schema repair: %v", applyErr),
			recoverySuggestion: "inspect the repaired patch, allowed paths, and target paths before rerun",
			signature:          "workspace_patch_apply_failed",
			preserveWorkspace:  false,
			resumeAllowed:      false,
			policy:             repairFailurePolicy{PreserveWorkspace: false, ResumeAllowed: false, RequiresHumanReview: true, MaxRounds: 2, UsedRounds: stats.Attempts, RemainingRounds: maxInt(0, 2-stats.Attempts), TerminationReason: "builder runtime schema repair patch apply failed"},
			stats:              stats,
			wrapped:            applyErr,
		}
	}
	stats.OperationCount = len(patch.Operations)
	stats.TargetedOperationCount, stats.UnrelatedOperationCount = classifyBuilderRuntimeOperations(patch.Operations, targetPaths)
	if stats.OperationCount > 0 {
		stats.UnrelatedOperationRate = float64(stats.UnrelatedOperationCount) / float64(stats.OperationCount)
	}
	if applyResult.Status != "" {
		patch.Status = applyResult.Status
	}
	patch.ModifiedFiles = modifiedFilePaths(applyResult.ModifiedFiles)
	result.Patch = &patch
	result.ApplyResult = applyResult
	return result, nil
}

func (runner *Runner) retryBuilderRuntimePatchApplyRepair(ctx context.Context, backend RunnerBackend, runID string, step ExecutionStep, run runRecord, roundInput appruns.RoundInput, route appruns.BuilderRuntimeTaskRoute, stats *appruns.BuilderRuntimeExecutionStats, modelAlias, invalidContent string, applyErr error) (*builderRuntimeExecutionResult, error) {
	result := &builderRuntimeExecutionResult{AppliedRoundInput: cloneRoundInputValue(roundInput), Stats: stats}
	if strings.TrimSpace(modelAlias) == "" || strings.TrimSpace(invalidContent) == "" {
		return result, &builderRuntimeExecutionError{
			summary:            fmt.Sprintf("builder runtime workspace patch apply failed: %v", applyErr),
			recoverySuggestion: "inspect generated patch, allowed paths, and target paths before rerun",
			signature:          "workspace_patch_apply_failed",
			preserveWorkspace:  false,
			resumeAllowed:      false,
			policy:             repairFailurePolicy{PreserveWorkspace: false, ResumeAllowed: false, RequiresHumanReview: true, MaxRounds: 2, UsedRounds: stats.Attempts, RemainingRounds: maxInt(0, 2-stats.Attempts), TerminationReason: "builder runtime patch apply repair unavailable"},
			stats:              stats,
			wrapped:            applyErr,
		}
	}
	request := BuilderRuntimePatchRequest{
		Run:            run,
		RoundInput:     roundInput,
		Route:          route,
		ModelAliases:   builderRuntimeRepairModelAliases(route.Model, modelAlias),
		RepairOnly:     true,
		PreviousErr:    applyErr.Error(),
		PreviousBody:   invalidContent,
		FailureContext: applyErr.Error(),
	}
	routeTargetPaths := builderRuntimeRouteTargetPaths(run.TaskBundle, route.TaskID)
	roundState := heartbeatRoundStateForStep(step)
	if err := runner.reportPatchGenerationStarted(ctx, backend, runID, step, roundInput, roundState, routeTargetPaths); err != nil {
		return result, err
	}
	response, attemptsUsed, err := runner.generateBuilderRuntimePatchWithTransientRetry(ctx, request)
	stats.Attempts += attemptsUsed
	if err != nil {
		stats.FailureReason = err.Error()
		if reportErr := runner.reportPatchGenerationFailed(ctx, backend, runID, step, roundInput, roundState, routeTargetPaths, err); reportErr != nil {
			return result, reportErr
		}
		return result, &builderRuntimeExecutionError{
			summary:            fmt.Sprintf("builder runtime patch-apply repair request failed: %v", err),
			recoverySuggestion: "fix builder-runtime model connectivity or prompt configuration before rerun",
			signature:          "builder_runtime_model_request_failed",
			preserveWorkspace:  true,
			resumeAllowed:      false,
			policy:             repairFailurePolicy{PreserveWorkspace: true, ResumeAllowed: false, RequiresHumanReview: true, MaxRounds: 2, UsedRounds: stats.Attempts, RemainingRounds: maxInt(0, 2-stats.Attempts), TerminationReason: "builder runtime patch-apply repair request failed"},
			stats:              stats,
			wrapped:            err,
		}
	}
	stats.SelectedModel = response.ModelAlias
	stats.ModelSequence = append(stats.ModelSequence, response.ModelAlias)
	stats.PromptTokens += response.PromptTokens
	stats.CompletionTokens += response.CompletionTokens
	stats.TotalTokens += response.TotalTokens
	appendBuilderRuntimeLog(run.LogPath, response.ModelAlias, response.Content)
	patch, normalized, driftCount, repairParseErr := normalizeBuilderRuntimePatchWithWorkspace(run.WorkspacePath, response.Content, roundInput.RoundID, routeTargetPaths)
	normalizeBuilderRuntimePatchForWorkspaceWithTaskBundle(run.WorkspacePath, roundInput.TaskBundle, &patch)
	stats.SchemaNormalized = stats.SchemaNormalized || normalized
	stats.SchemaDriftCount += driftCount
	if repairParseErr != nil {
		stats.ParseFailureCount++
		stats.FailureReason = repairParseErr.Error()
		if reportErr := runner.reportPatchGenerationFailed(ctx, backend, runID, step, roundInput, roundState, routeTargetPaths, repairParseErr); reportErr != nil {
			return result, reportErr
		}
		return result, &builderRuntimeExecutionError{
			summary:            fmt.Sprintf("builder runtime patch parse failed after patch-apply repair: %v", repairParseErr),
			recoverySuggestion: "tighten replace_block uniqueness guidance or force full-file write_file on the target file before rerun",
			signature:          "builder_runtime_patch_parse_failed",
			preserveWorkspace:  true,
			resumeAllowed:      false,
			policy:             repairFailurePolicy{PreserveWorkspace: true, ResumeAllowed: false, RequiresHumanReview: false, MaxRounds: 2, UsedRounds: stats.Attempts, RemainingRounds: maxInt(0, 2-stats.Attempts), TerminationReason: "builder runtime patch-apply repair parse failed"},
			stats:              stats,
			wrapped:            repairParseErr,
		}
	}
	targetPaths := routeTargetPaths
	if shouldEnforceBuilderRuntimeTargetScope(route.TaskType) {
		if scopeErr := validateBuilderRuntimeTargetScope(patch.Operations, targetPaths); scopeErr != nil {
			stats.ScopeViolationCount++
			stats.FailureReason = scopeErr.Error()
			if reportErr := runner.reportPatchGenerationFailed(ctx, backend, runID, step, roundInput, roundState, targetPaths, scopeErr); reportErr != nil {
				return result, reportErr
			}
			return result, &builderRuntimeExecutionError{
				summary:            fmt.Sprintf("builder runtime workspace patch exceeded repair target scope after patch-apply repair: %v", scopeErr),
				recoverySuggestion: "keep repair edits inside the scoped task target_paths for the current task",
				signature:          "builder_runtime_scope_violation",
				preserveWorkspace:  true,
				resumeAllowed:      false,
				policy:             repairFailurePolicy{PreserveWorkspace: true, ResumeAllowed: false, RequiresHumanReview: false, MaxRounds: 2, UsedRounds: stats.Attempts, RemainingRounds: maxInt(0, 2-stats.Attempts), TerminationReason: "builder runtime patch-apply repair exceeded target scope"},
				stats:              stats,
				wrapped:            scopeErr,
			}
		}
	}
	if semanticErr := validateBuilderRuntimeSemanticConsistency(run, patch.Operations, ""); semanticErr != nil {
		stats.FailureReason = semanticErr.Error()
		if reportErr := runner.reportPatchGenerationFailed(ctx, backend, runID, step, roundInput, roundState, targetPaths, semanticErr); reportErr != nil {
			return result, reportErr
		}
		return result, &builderRuntimeExecutionError{
			summary:            fmt.Sprintf("builder runtime workspace patch reintroduced schema-conflicting symbols after patch-apply repair: %v", semanticErr),
			recoverySuggestion: "align builder edits with current models and remove stale schema references before rerun",
			signature:          "builder_runtime_semantic_conflict",
			preserveWorkspace:  true,
			resumeAllowed:      false,
			policy:             repairFailurePolicy{PreserveWorkspace: true, ResumeAllowed: false, RequiresHumanReview: false, MaxRounds: 2, UsedRounds: stats.Attempts, RemainingRounds: maxInt(0, 2-stats.Attempts), TerminationReason: "builder runtime patch-apply repair reintroduced schema-conflicting symbols"},
			stats:              stats,
			wrapped:            semanticErr,
		}
	}
	if err := runner.reportGeneratedPatch(ctx, backend, runID, step, roundInput, roundState, &patch); err != nil {
		return result, err
	}
	applyResult, repairApplyErr := appruns.ApplyWorkspacePatch(run.WorkspacePath, run.AllowedPaths, run.ProtectedPaths, patch)
	if repairApplyErr != nil {
		if strings.Contains(repairApplyErr.Error(), "outside allowed roots") || strings.Contains(repairApplyErr.Error(), "protected") {
			stats.ScopeViolationCount++
		}
		stats.FailureReason = repairApplyErr.Error()
		if reportErr := runner.reportPatchApplyFailed(ctx, backend, runID, step, roundInput, roundState, workspacePatchPaths(&patch), repairApplyErr); reportErr != nil {
			return result, reportErr
		}
		return result, &builderRuntimeExecutionError{
			summary:            fmt.Sprintf("builder runtime workspace patch apply failed after patch-apply repair: %v", repairApplyErr),
			recoverySuggestion: "inspect the repaired patch and force a full-file write_file for the conflicting file before rerun",
			signature:          "workspace_patch_apply_failed",
			preserveWorkspace:  false,
			resumeAllowed:      false,
			policy:             repairFailurePolicy{PreserveWorkspace: false, ResumeAllowed: false, RequiresHumanReview: true, MaxRounds: 2, UsedRounds: stats.Attempts, RemainingRounds: maxInt(0, 2-stats.Attempts), TerminationReason: "builder runtime patch-apply repair apply failed"},
			stats:              stats,
			wrapped:            repairApplyErr,
		}
	}
	stats.OperationCount = len(patch.Operations)
	stats.TargetedOperationCount, stats.UnrelatedOperationCount = classifyBuilderRuntimeOperations(patch.Operations, targetPaths)
	if stats.OperationCount > 0 {
		stats.UnrelatedOperationRate = float64(stats.UnrelatedOperationCount) / float64(stats.OperationCount)
	}
	if applyResult.Status != "" {
		patch.Status = applyResult.Status
	}
	patch.ModifiedFiles = modifiedFilePaths(applyResult.ModifiedFiles)
	result.Patch = &patch
	result.ApplyResult = applyResult
	return result, nil
}

func shouldRetryBuilderRuntimePatchApplyRepair(applyErr error) bool {
	if applyErr == nil {
		return false
	}
	return strings.Contains(applyErr.Error(), "replace_block expects exactly one match")
}

func buildBuilderRuntimeFileContext(run runRecord, routeTaskID string, routeTaskType appruns.BuilderRuntimeTaskType) (string, error) {
	paths := builderRuntimeFileContextPaths(run.TaskBundle, routeTaskID, routeTaskType)
	if len(paths) == 0 {
		paths = concreteAllowedPaths(run.AllowedPaths)
	}
	if len(paths) == 0 {
		return "- no concrete files available\n", nil
	}
	var builder strings.Builder
	routeTaskID = strings.TrimSpace(routeTaskID)
	targetPathSet := map[string]struct{}{}
	if routeTaskID != "" {
		for _, task := range run.TaskBundle {
			normalized := appruns.NormalizeTaskBundleItem(task)
			if strings.TrimSpace(normalized.TaskID) != routeTaskID {
				continue
			}
			for _, path := range concreteTaskTargetPaths([]appruns.TaskBundleItem{normalized}) {
				targetPathSet[path] = struct{}{}
			}
			break
		}
	}
	for _, relPath := range paths {
		absPath := filepath.Join(run.WorkspacePath, filepath.FromSlash(relPath))
		content, err := os.ReadFile(absPath)
		if err != nil {
			if os.IsNotExist(err) {
				if referencePath := strings.TrimSpace(run.TemplateReferenceFiles[relPath]); referencePath != "" {
					if shouldInlineBuilderRuntimeReferenceTemplate(routeTaskType, targetPathSet, relPath) {
						referenceContent, readErr := os.ReadFile(filepath.FromSlash(referencePath))
						if readErr != nil {
							return "", fmt.Errorf("read builder runtime reference template %s: %w", relPath, readErr)
						}
						builder.WriteString("REFERENCE TEMPLATE: ")
						builder.WriteString(relPath)
						builder.WriteString("\n")
						builder.Write(referenceContent)
						if len(referenceContent) == 0 || referenceContent[len(referenceContent)-1] != '\n' {
							builder.WriteString("\n")
						}
						continue
					}
					builder.WriteString("REFERENCE TEMPLATE OMITTED: ")
					builder.WriteString(relPath)
					builder.WriteString("\n")
					builder.WriteString("template_source=")
					builder.WriteString(referencePath)
					builder.WriteString("\n")
					continue
				}
				if _, ok := targetPathSet[relPath]; ok {
					builder.WriteString("TARGET FILE TO CREATE: ")
					builder.WriteString(relPath)
					builder.WriteString("\n<create from domain model and current dependencies>\n")
					continue
				}
				builder.WriteString("MISSING CONTEXT FILE: ")
				builder.WriteString(relPath)
				builder.WriteString("\n")
				continue
			}
			return "", fmt.Errorf("read builder runtime context file %s: %w", relPath, err)
		}
		builder.WriteString("EXISTING FILE: ")
		builder.WriteString(relPath)
		builder.WriteString("\n")
		builder.Write(content)
		if len(content) == 0 || content[len(content)-1] != '\n' {
			builder.WriteString("\n")
		}
	}
	return builder.String(), nil
}

func shouldInlineBuilderRuntimeReferenceTemplate(routeTaskType appruns.BuilderRuntimeTaskType, targetPathSet map[string]struct{}, relPath string) bool {
	if routeTaskType == appruns.BuilderRuntimeTaskTypeClosureRepair || routeTaskType == appruns.BuilderRuntimeTaskTypeAnalyzeRepair || routeTaskType == appruns.BuilderRuntimeTaskTypeTestRepair {
		return false
	}
	_, ok := targetPathSet[filepath.ToSlash(strings.TrimSpace(relPath))]
	return ok
}

func builderRuntimeAllowedPaths(run runRecord, roundInput appruns.RoundInput, route appruns.BuilderRuntimeTaskRoute) []string {
	routeTargetPaths := builderRuntimeRouteTargetPaths(run.TaskBundle, route.TaskID)
	switch route.TaskType {
	case appruns.BuilderRuntimeTaskTypeAnalyzeRepair, appruns.BuilderRuntimeTaskTypeTestRepair:
		if len(routeTargetPaths) > 0 {
			return mergeBuilderRuntimeContextPaths(routeTargetPaths, builderRuntimeModelContextPaths(run.TaskBundle))
		}
	case appruns.BuilderRuntimeTaskTypeClosureRepair:
		return roundInput.AllowedPaths
	default:
		if len(routeTargetPaths) > 0 {
			return routeTargetPaths
		}
	}
	return roundInput.AllowedPaths
}

func builderRuntimeRouteTargetPaths(tasks []appruns.TaskBundleItem, routeTaskID string) []string {
	routeTask := appruns.NormalizeTaskBundleItem(resolveBuilderRuntimePromptTask(tasks, routeTaskID))
	if strings.TrimSpace(routeTask.TaskID) == "" {
		return nil
	}
	return concreteTaskTargetPaths([]appruns.TaskBundleItem{routeTask})
}

func builderRuntimeFileContextPaths(tasks []appruns.TaskBundleItem, routeTaskID string, routeTaskType appruns.BuilderRuntimeTaskType) []string {
	if trimmedTaskID := strings.TrimSpace(routeTaskID); trimmedTaskID != "" {
		primary := builderRuntimeTaskContextPaths(tasks, trimmedTaskID)
		switch routeTaskType {
		case appruns.BuilderRuntimeTaskTypeAnalyzeRepair, appruns.BuilderRuntimeTaskTypeTestRepair:
			if len(primary) > 0 {
				return mergeBuilderRuntimeContextPaths(primary, builderRuntimeModelContextPaths(tasks))
			}
		default:
			if len(primary) > 0 {
				if builderRuntimePathsContainDartPathWithin(primary, "lib/template/") {
					return mergeBuilderRuntimeContextPaths(primary, builderRuntimeModelContextPaths(tasks))
				}
				return primary
			}
		}
	}
	return concreteTaskTargetPaths(tasks)
}

func builderRuntimeTaskContextPaths(tasks []appruns.TaskBundleItem, routeTaskID string) []string {
	routeTaskID = strings.TrimSpace(routeTaskID)
	if routeTaskID == "" {
		return nil
	}
	taskMap := make(map[string]appruns.TaskBundleItem, len(tasks))
	for _, task := range tasks {
		normalized := appruns.NormalizeTaskBundleItem(task)
		if taskID := strings.TrimSpace(normalized.TaskID); taskID != "" {
			taskMap[taskID] = normalized
		}
	}
	seenTasks := map[string]struct{}{}
	paths := make([]string, 0)
	var visit func(taskID string)
	visit = func(taskID string) {
		taskID = strings.TrimSpace(taskID)
		if taskID == "" {
			return
		}
		if _, ok := seenTasks[taskID]; ok {
			return
		}
		seenTasks[taskID] = struct{}{}
		task, ok := taskMap[taskID]
		if !ok {
			return
		}
		for _, dep := range task.Dependencies {
			visit(dep)
		}
		paths = mergeBuilderRuntimeContextPaths(paths, concreteTaskTargetPaths([]appruns.TaskBundleItem{task}))
	}
	visit(routeTaskID)
	return paths
}

func builderRuntimeModelContextPaths(tasks []appruns.TaskBundleItem) []string {
	paths := concreteTaskTargetPaths(tasks)
	if len(paths) == 0 {
		return nil
	}
	modelPaths := make([]string, 0)
	for _, path := range paths {
		if strings.HasPrefix(path, "lib/models/") {
			modelPaths = append(modelPaths, path)
		}
	}
	return modelPaths
}

func mergeBuilderRuntimeContextPaths(primary, extras []string) []string {
	seen := map[string]struct{}{}
	merged := make([]string, 0, len(primary)+len(extras))
	appendPath := func(path string) {
		normalized := filepath.ToSlash(strings.TrimSpace(path))
		if normalized == "" {
			return
		}
		if _, ok := seen[normalized]; ok {
			return
		}
		seen[normalized] = struct{}{}
		merged = append(merged, normalized)
	}
	for _, path := range primary {
		appendPath(path)
	}
	for _, path := range extras {
		appendPath(path)
	}
	return merged
}

func shouldEnforceBuilderRuntimeTargetScope(taskType appruns.BuilderRuntimeTaskType) bool {
	switch taskType {
	case "", appruns.BuilderRuntimeTaskTypeClosureRepair:
		return false
	}
	return true
}

func validateBuilderRuntimeTargetScope(operations []appruns.WorkspacePatchOperation, targetPaths []string) error {
	if len(operations) == 0 || len(targetPaths) == 0 {
		return nil
	}
	allowed := make(map[string]struct{}, len(targetPaths))
	for _, path := range targetPaths {
		normalized := filepath.ToSlash(strings.TrimSpace(path))
		if normalized == "" {
			continue
		}
		allowed[normalized] = struct{}{}
	}
	if len(allowed) == 0 {
		return nil
	}
	outside := make([]string, 0)
	seen := map[string]struct{}{}
	for _, operation := range operations {
		normalized := filepath.ToSlash(strings.TrimSpace(operation.Path))
		if normalized == "" {
			continue
		}
		if _, ok := allowed[normalized]; ok {
			continue
		}
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		outside = append(outside, normalized)
	}
	if len(outside) == 0 {
		return nil
	}
	sort.Strings(outside)
	return fmt.Errorf("operations touched paths outside current repair target_paths: %s", strings.Join(outside, ", "))
}

func validateBuilderRuntimeSemanticConsistency(run runRecord, operations []appruns.WorkspacePatchOperation, failureContext string) error {
	if err := validateBuilderRuntimeFailureSymbolPreservation(run, operations, failureContext); err != nil {
		return err
	}
	if err := validateBuilderRuntimeFailureSymbolResolution(run, operations, failureContext); err != nil {
		return err
	}
	if err := validateBuilderRuntimeDeclaredPackageImports(run.WorkspacePath, operations); err != nil {
		return err
	}
	if builderRuntimeLooksLikeBookkeepingDomain(run.WorkspacePath, operations) {
		if err := validateBuilderRuntimeBookkeepingRepositoryShape(operations); err != nil {
			return err
		}
		if err := validateBuilderRuntimeBookkeepingFieldConsistency(run.WorkspacePath, operations); err != nil {
			return err
		}
	}
	if err := validateBuilderRuntimeModelExportStability(run.WorkspacePath, operations); err != nil {
		return err
	}
	forbidden, err := builderRuntimeForbiddenSchemaTokens(run.WorkspacePath)
	if err != nil || len(forbidden) == 0 {
		return err
	}
	violations := make([]string, 0)
	for _, operation := range operations {
		content := operation.Content
		if operation.Type == "replace_block" {
			content = operation.NewContent
		}
		if strings.TrimSpace(content) == "" {
			continue
		}
		matched := make([]string, 0)
		for _, token := range forbidden {
			if strings.Contains(content, token) {
				matched = append(matched, token)
			}
		}
		if len(matched) == 0 {
			continue
		}
		sort.Strings(matched)
		violations = append(violations, fmt.Sprintf("%s -> %s", operation.Path, strings.Join(matched, ", ")))
	}
	if len(violations) == 0 {
		return nil
	}
	sort.Strings(violations)
	return fmt.Errorf("patch content reintroduced symbols absent from current models: %s", strings.Join(violations, "; "))
}

func validateBuilderRuntimeFailureSymbolPreservation(run runRecord, operations []appruns.WorkspacePatchOperation, failureContext string) error {
	undefinedSymbols := builderRuntimeUndefinedTypeLikeSymbolsFromFailureContext(failureContext)
	if len(undefinedSymbols) == 0 {
		return nil
	}
	patchedDefinitions := builderRuntimePatchedTypeLikeDefinitions(operations, undefinedSymbols)
	violations := make([]string, 0)
	for _, symbol := range undefinedSymbols {
		definingPath, ok := builderRuntimeCurrentFailureSymbolDefinitionPath(run.WorkspacePath, run.TaskBundle, symbol)
		if !ok {
			continue
		}
		if !builderRuntimeOperationRewritesWholeFile(operations, definingPath) {
			continue
		}
		if _, preserved := patchedDefinitions[symbol]; preserved {
			continue
		}
		usedOutsideDefiningPath, err := builderRuntimeWorkspaceUsesFailureSymbolOutsidePath(run.WorkspacePath, definingPath, symbol)
		isTopLevelShim, shimErr := builderRuntimeCurrentFailureSymbolIsTopLevelLocalShim(run.WorkspacePath, definingPath, symbol)
		if err == nil && shimErr == nil && !usedOutsideDefiningPath && isTopLevelShim {
			continue
		}
		violations = append(violations, fmt.Sprintf("%s -> %s", symbol, definingPath))
	}
	if len(violations) == 0 {
		return nil
	}
	sort.Strings(violations)
	return fmt.Errorf("patch removed required helper symbols referenced by current validation failures: %s", strings.Join(violations, "; "))
}

var errBuilderRuntimeFailureSymbolUsageFound = errors.New("builder runtime failure symbol usage found outside defining path")

func builderRuntimeWorkspaceUsesFailureSymbolOutsidePath(workspacePath, definingPath, symbol string) (bool, error) {
	trimmedWorkspacePath := strings.TrimSpace(workspacePath)
	normalizedDefiningPath := normalizeBuilderRuntimeTargetPath(definingPath)
	trimmedSymbol := strings.TrimSpace(symbol)
	if trimmedWorkspacePath == "" || normalizedDefiningPath == "" || trimmedSymbol == "" {
		return false, nil
	}
	for _, relativeRoot := range []string{"lib", "test"} {
		rootPath := filepath.Join(trimmedWorkspacePath, filepath.FromSlash(relativeRoot))
		rootInfo, err := os.Stat(rootPath)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return false, err
		}
		if !rootInfo.IsDir() {
			continue
		}
		err = filepath.Walk(rootPath, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() || filepath.Ext(path) != ".dart" {
				return nil
			}
			relPath, err := filepath.Rel(trimmedWorkspacePath, path)
			if err != nil {
				return err
			}
			normalizedPath := filepath.ToSlash(relPath)
			if normalizedPath == normalizedDefiningPath {
				return nil
			}
			content, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			if builderRuntimeContentContainsDartIdentifier(string(content), trimmedSymbol) {
				return errBuilderRuntimeFailureSymbolUsageFound
			}
			return nil
		})
		if errors.Is(err, errBuilderRuntimeFailureSymbolUsageFound) {
			return true, nil
		}
		if err != nil {
			return false, err
		}
	}
	return false, nil
}

func builderRuntimeCurrentFailureSymbolIsTopLevelLocalShim(workspacePath, definingPath, symbol string) (bool, error) {
	trimmedWorkspacePath := strings.TrimSpace(workspacePath)
	normalizedDefiningPath := normalizeBuilderRuntimeTargetPath(definingPath)
	trimmedSymbol := strings.TrimSpace(symbol)
	if trimmedWorkspacePath == "" || normalizedDefiningPath == "" || trimmedSymbol == "" {
		return false, nil
	}
	content, err := os.ReadFile(filepath.Join(trimmedWorkspacePath, filepath.FromSlash(normalizedDefiningPath)))
	if err != nil {
		return false, err
	}
	return builderRuntimeContentDefinesTopLevelFailureSymbol(string(content), trimmedSymbol), nil
}

func builderRuntimeContentDefinesTopLevelFailureSymbol(content, symbol string) bool {
	trimmedSymbol := strings.TrimSpace(symbol)
	if trimmedSymbol == "" || strings.TrimSpace(content) == "" {
		return false
	}
	pattern := regexp.MustCompile(`(?m)^(?:@override\s+)?(?:abstract\s+|base\s+|final\s+|sealed\s+|static\s+|external\s+)?(?:[A-Za-z_][A-Za-z0-9_<>,?\[\] ]*\s+)?(?:get\s+|set\s+)?` + regexp.QuoteMeta(trimmedSymbol) + `\s*(?:\(|=>|\{|;|=)`)
	return pattern.MatchString(content)
}

func validateBuilderRuntimeFailureSymbolResolution(run runRecord, operations []appruns.WorkspacePatchOperation, failureContext string) error {
	undefinedSymbols := builderRuntimeUndefinedTypeLikeSymbolsFromFailureContext(failureContext)
	if len(undefinedSymbols) == 0 {
		return nil
	}
	ownerPaths := builderRuntimeUndefinedFailureSymbolOwnerPaths(run.WorkspacePath, failureContext)
	patchedDefinitions := builderRuntimePatchedTypeLikeDefinitions(operations, undefinedSymbols)
	patchedUsages := builderRuntimePatchedTypeLikeUsages(operations, undefinedSymbols)
	violations := make([]string, 0)
	for _, symbol := range undefinedSymbols {
		if _, used := patchedUsages[symbol]; !used {
			continue
		}
		if _, defined := patchedDefinitions[symbol]; defined {
			continue
		}
		if _, exists := builderRuntimeCurrentFailureSymbolDefinitionPath(run.WorkspacePath, run.TaskBundle, symbol); exists {
			continue
		}
		if ownerPath, ok := ownerPaths[symbol]; ok {
			violations = append(violations, fmt.Sprintf("%s -> %s", ownerPath, symbol))
			continue
		}
		violations = append(violations, symbol)
	}
	if len(violations) == 0 {
		return nil
	}
	sort.Strings(violations)
	return fmt.Errorf("patch still references unresolved helper symbols from current validation failures: %s", strings.Join(violations, ", "))
}

func validateBuilderRuntimeDeclaredPackageImports(workspacePath string, operations []appruns.WorkspacePatchOperation) error {
	declared, err := builderRuntimeDeclaredPubspecDependencies(workspacePath)
	if err != nil {
		return err
	}
	if len(declared) == 0 {
		return nil
	}
	packageName := workspaceDartPackageName(workspacePath)
	violations := make([]string, 0)
	for _, operation := range operations {
		path := filepath.ToSlash(strings.TrimSpace(operation.Path))
		if !strings.HasSuffix(path, ".dart") {
			continue
		}
		content := operation.Content
		if operation.Type == "replace_block" {
			content = operation.NewContent
		}
		if strings.TrimSpace(content) == "" {
			continue
		}
		imports := builderRuntimeImportedPackageNames(content)
		if len(imports) == 0 {
			continue
		}
		missing := make([]string, 0)
		for _, name := range imports {
			if name == packageName {
				continue
			}
			if _, ok := declared[name]; ok {
				continue
			}
			missing = append(missing, name)
		}
		if len(missing) == 0 {
			continue
		}
		sort.Strings(missing)
		violations = append(violations, fmt.Sprintf("%s -> %s", path, strings.Join(missing, ", ")))
	}
	if len(violations) == 0 {
		return nil
	}
	sort.Strings(violations)
	return fmt.Errorf("patch introduced package imports not declared in pubspec.yaml: %s", strings.Join(violations, "; "))
}

func builderRuntimeDeclaredPubspecDependencies(workspacePath string) (map[string]struct{}, error) {
	if strings.TrimSpace(workspacePath) == "" {
		return nil, nil
	}
	content, err := os.ReadFile(filepath.Join(workspacePath, "pubspec.yaml"))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("read pubspec dependencies for semantic consistency: %w", err)
	}
	var payload struct {
		Dependencies    map[string]any `yaml:"dependencies"`
		DevDependencies map[string]any `yaml:"dev_dependencies"`
	}
	if err := yaml.Unmarshal(content, &payload); err != nil {
		return nil, fmt.Errorf("parse pubspec dependencies for semantic consistency: %w", err)
	}
	declared := make(map[string]struct{}, len(payload.Dependencies)+len(payload.DevDependencies))
	for name := range payload.Dependencies {
		trimmed := strings.TrimSpace(name)
		if trimmed == "" {
			continue
		}
		declared[trimmed] = struct{}{}
	}
	for name := range payload.DevDependencies {
		trimmed := strings.TrimSpace(name)
		if trimmed == "" {
			continue
		}
		declared[trimmed] = struct{}{}
	}
	return declared, nil
}

func builderRuntimeImportedPackageNames(content string) []string {
	matches := builderRuntimeDartPackageImportPattern.FindAllStringSubmatch(content, -1)
	if len(matches) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	packages := make([]string, 0, len(matches))
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		name := strings.TrimSpace(match[1])
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		packages = append(packages, name)
	}
	if len(packages) == 0 {
		return nil
	}
	sort.Strings(packages)
	return packages
}

func builderRuntimeCurrentFailureSymbolDefinitions(workspacePath string, taskBundle []appruns.TaskBundleItem, symbols []string) ([]string, error) {
	if strings.TrimSpace(workspacePath) == "" || len(symbols) == 0 {
		return nil, nil
	}
	definitions := make([]string, 0, len(symbols))
	for _, symbol := range symbols {
		path, ok := builderRuntimeCurrentFailureSymbolDefinitionPath(workspacePath, taskBundle, symbol)
		if !ok {
			continue
		}
		definitions = append(definitions, fmt.Sprintf("%s -> %s", symbol, path))
	}
	sort.Strings(definitions)
	return definitions, nil
}

func builderRuntimeCurrentFailureSymbolDefinitionPath(workspacePath string, taskBundle []appruns.TaskBundleItem, symbol string) (string, bool) {
	if strings.TrimSpace(workspacePath) == "" || strings.TrimSpace(symbol) == "" {
		return "", false
	}
	for _, targetPath := range concreteTaskTargetPaths(taskBundle) {
		normalizedPath := filepath.ToSlash(strings.TrimSpace(targetPath))
		if normalizedPath == "" || strings.Contains(normalizedPath, "**") {
			continue
		}
		content, err := os.ReadFile(filepath.Join(workspacePath, filepath.FromSlash(normalizedPath)))
		if err != nil {
			continue
		}
		if builderRuntimeContentDefinesFailureSymbol(string(content), symbol) {
			return normalizedPath, true
		}
	}
	return "", false
}

var errBuilderRuntimeNamedTypeDeclarationFound = errors.New("builder runtime named type declaration found")

func builderRuntimeUndefinedFailureSymbolOwnerPaths(workspacePath, failureContext string) map[string]string {
	trimmedWorkspacePath := strings.TrimSpace(workspacePath)
	trimmedFailureContext := strings.TrimSpace(failureContext)
	if trimmedWorkspacePath == "" || trimmedFailureContext == "" {
		return nil
	}
	matches := builderRuntimeUndefinedMemberOwnerPattern.FindAllStringSubmatch(trimmedFailureContext, -1)
	if len(matches) == 0 {
		return nil
	}
	ownerPaths := make(map[string]string, len(matches))
	for _, match := range matches {
		if len(match) < 3 {
			continue
		}
		symbol := strings.TrimSpace(match[1])
		ownerType := strings.TrimSpace(match[2])
		if symbol == "" || ownerType == "" {
			continue
		}
		if _, ok := ownerPaths[symbol]; ok {
			continue
		}
		path, ok := builderRuntimeWorkspaceNamedTypeDeclarationPath(trimmedWorkspacePath, ownerType)
		if !ok {
			continue
		}
		ownerPaths[symbol] = path
	}
	if len(ownerPaths) == 0 {
		return nil
	}
	return ownerPaths
}

func builderRuntimeWorkspaceNamedTypeDeclarationPath(workspacePath, typeName string) (string, bool) {
	trimmedWorkspacePath := strings.TrimSpace(workspacePath)
	trimmedTypeName := strings.TrimSpace(typeName)
	if trimmedWorkspacePath == "" || trimmedTypeName == "" {
		return "", false
	}
	for _, relativeRoot := range []string{"lib", "test"} {
		rootPath := filepath.Join(trimmedWorkspacePath, filepath.FromSlash(relativeRoot))
		rootInfo, err := os.Stat(rootPath)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return "", false
		}
		if !rootInfo.IsDir() {
			continue
		}
		foundPath := ""
		err = filepath.Walk(rootPath, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() || filepath.Ext(path) != ".dart" {
				return nil
			}
			content, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			if builderRuntimeLastNamedDartTypeBlock(string(content), "class", trimmedTypeName) == "" && builderRuntimeLastNamedDartTypeBlock(string(content), "enum", trimmedTypeName) == "" {
				return nil
			}
			relPath, err := filepath.Rel(trimmedWorkspacePath, path)
			if err != nil {
				return err
			}
			foundPath = filepath.ToSlash(relPath)
			return errBuilderRuntimeNamedTypeDeclarationFound
		})
		if errors.Is(err, errBuilderRuntimeNamedTypeDeclarationFound) {
			return foundPath, true
		}
		if err != nil {
			return "", false
		}
	}
	return "", false
}

func builderRuntimeUndefinedTypeLikeSymbolsFromFailureContext(failureContext string) []string {
	seen := map[string]struct{}{}
	result := make([]string, 0)
	for _, pattern := range builderRuntimeUndefinedTypeLikeSymbolPatterns {
		matches := pattern.FindAllStringSubmatch(failureContext, -1)
		for _, match := range matches {
			if len(match) < 2 {
				continue
			}
			symbol := strings.TrimSpace(match[1])
			if symbol == "" {
				continue
			}
			if _, ok := seen[symbol]; ok {
				continue
			}
			seen[symbol] = struct{}{}
			result = append(result, symbol)
		}
	}
	if len(result) == 0 {
		return nil
	}
	sort.Strings(result)
	return result
}

func builderRuntimePatchedTypeLikeDefinitions(operations []appruns.WorkspacePatchOperation, symbols []string) map[string]struct{} {
	if len(symbols) == 0 {
		return nil
	}
	defined := make(map[string]struct{}, len(symbols))
	for _, operation := range operations {
		content := operation.Content
		if operation.Type == "replace_block" {
			content = operation.NewContent
		}
		if strings.TrimSpace(content) == "" {
			continue
		}
		for _, symbol := range symbols {
			if builderRuntimeContentDefinesFailureSymbol(content, symbol) {
				defined[symbol] = struct{}{}
			}
		}
	}
	return defined
}

func builderRuntimePatchedTypeLikeUsages(operations []appruns.WorkspacePatchOperation, symbols []string) map[string]struct{} {
	if len(symbols) == 0 {
		return nil
	}
	used := make(map[string]struct{}, len(symbols))
	for _, operation := range operations {
		content := operation.Content
		if operation.Type == "replace_block" {
			content = operation.NewContent
		}
		if strings.TrimSpace(content) == "" {
			continue
		}
		for _, symbol := range symbols {
			if builderRuntimeContentContainsDartIdentifier(content, symbol) {
				used[symbol] = struct{}{}
			}
		}
	}
	return used
}

func builderRuntimeMissingPackageDependenciesFromFailureContext(failureContext string) []string {
	matches := builderRuntimeMissingPackageImportPattern.FindAllStringSubmatch(failureContext, -1)
	if len(matches) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	packages := make([]string, 0, len(matches))
	for _, match := range matches {
		if len(match) < 3 {
			continue
		}
		pkg := strings.TrimSpace(match[1])
		if pkg == "" {
			pkg = strings.TrimSpace(match[2])
		}
		if pkg == "" {
			continue
		}
		if _, ok := seen[pkg]; ok {
			continue
		}
		seen[pkg] = struct{}{}
		packages = append(packages, pkg)
	}
	if len(packages) == 0 {
		return nil
	}
	sort.Strings(packages)
	return packages
}

func builderRuntimeContentDefinesFailureSymbol(content, symbol string) bool {
	trimmedSymbol := strings.TrimSpace(symbol)
	if trimmedSymbol == "" || strings.TrimSpace(content) == "" {
		return false
	}
	if builderRuntimeContentDefinesTypeLikeSymbol(content, trimmedSymbol) || builderRuntimeContentDeclaresField(content, trimmedSymbol) {
		return true
	}
	pattern := regexp.MustCompile(`(?m)^\s*(?:@override\s+)?(?:static\s+)?(?:external\s+)?(?:[A-Za-z_][A-Za-z0-9_<>,?\[\] ]*\s+)?(?:get\s+|set\s+)?` + regexp.QuoteMeta(trimmedSymbol) + `\s*(?:\(|=>|\{|;|=)`)
	return pattern.MatchString(content)
}

func validateBuilderRuntimeBookkeepingRepositoryShape(operations []appruns.WorkspacePatchOperation) error {
	violations := make([]string, 0)
	for _, operation := range operations {
		path := filepath.ToSlash(strings.TrimSpace(operation.Path))
		if !builderRuntimeIsDartPathWithin(path, "lib/repositories/") {
			continue
		}
		content := operation.Content
		if operation.Type == "replace_block" {
			content = operation.NewContent
		}
		if strings.TrimSpace(content) == "" {
			continue
		}
		if strings.Contains(content, "<List<Map<String, dynamic>>>[]") || strings.Contains(content, "List<List<Map<String, dynamic>>>") {
			violations = append(violations, path+" -> nested list default/value shape for bookkeeping entries")
		}
	}
	if len(violations) == 0 {
		return nil
	}
	sort.Strings(violations)
	return fmt.Errorf("patch introduced invalid bookkeeping repository value shapes: %s", strings.Join(violations, "; "))
}

func validateBuilderRuntimeBookkeepingFieldConsistency(workspacePath string, operations []appruns.WorkspacePatchOperation) error {
	entryModelContent, err := builderRuntimeSemanticFileContent(workspacePath, operations, "lib/models/entry.dart")
	if err != nil {
		return err
	}
	summaryModelContent, err := builderRuntimeSemanticFileContent(workspacePath, operations, "lib/models/summary.dart")
	if err != nil {
		return err
	}
	hasInMemoryEntryRepository, err := builderRuntimeDirectoryContainsTypeLikeSymbol(workspacePath, operations, "lib/repositories/", "InMemoryEntryRepository")
	if err != nil {
		return err
	}
	widgetTestContent, err := builderRuntimeSemanticFileContent(workspacePath, operations, "test/widget_test.dart")
	if err != nil {
		return err
	}
	entryFields := builderRuntimeDeclaredFieldNames(entryModelContent)
	summaryFields := builderRuntimeDeclaredFieldNames(summaryModelContent)
	entryHasOccurredOn := strings.Contains(entryModelContent, "occurredOn")
	entryHasDate := builderRuntimeContentDeclaresField(entryModelContent, "date")
	summaryHasEntryCount := builderRuntimeContentDeclaresField(summaryModelContent, "entryCount")
	hasInMemoryEntryRepository = hasInMemoryEntryRepository || builderRuntimeContentDefinesTypeLikeSymbol(widgetTestContent, "InMemoryEntryRepository")
	violations := make([]string, 0)
	for _, operation := range operations {
		content := operation.Content
		if operation.Type == "replace_block" {
			content = operation.NewContent
		}
		if strings.TrimSpace(content) == "" {
			continue
		}
		path := filepath.ToSlash(strings.TrimSpace(operation.Path))
		if builderRuntimeUsesBookkeepingSnapshotGuards(path) {
			for _, field := range builderRuntimeReferencedMemberNames(content, "entry") {
				if len(entryFields) == 0 {
					break
				}
				if _, ok := entryFields[field]; ok {
					continue
				}
				violations = append(violations, fmt.Sprintf("%s -> references entry.%s even though BookkeepingEntry does not define it in the current model snapshot", path, field))
			}
			for _, field := range builderRuntimeReferencedMemberNames(content, "summary") {
				if len(summaryFields) == 0 {
					break
				}
				if _, ok := summaryFields[field]; ok {
					continue
				}
				violations = append(violations, fmt.Sprintf("%s -> references summary.%s even though Summary does not define it in the current model snapshot", path, field))
			}
		}
		if entryHasOccurredOn && !entryHasDate && strings.Contains(content, "entry.date") {
			violations = append(violations, path+" -> references entry.date even though BookkeepingEntry only exposes occurredOn")
		}
		if !summaryHasEntryCount && strings.Contains(content, "summary.entryCount") {
			violations = append(violations, path+" -> references summary.entryCount even though Summary does not define it")
		}
		if path == "test/widget_test.dart" && strings.Contains(content, "BookkeepingEntry") && !builderRuntimeContentImportsEntryModel(content) {
			violations = append(violations, path+" -> uses BookkeepingEntry without importing the entry model")
		}
		if path == "test/widget_test.dart" && strings.Contains(content, "InMemoryEntryRepository(") && !hasInMemoryEntryRepository {
			violations = append(violations, path+" -> instantiates InMemoryEntryRepository even though the helper is not defined in the current workspace or patch")
		}
	}
	if len(violations) == 0 {
		return nil
	}
	sort.Strings(violations)
	return fmt.Errorf("patch introduced invalid bookkeeping field/model references: %s", strings.Join(violations, "; "))
}

func builderRuntimeUsesBookkeepingSnapshotGuards(path string) bool {
	normalized := filepath.ToSlash(strings.TrimSpace(path))
	if normalized == "test/widget_test.dart" {
		return true
	}
	return strings.HasPrefix(normalized, "lib/views/")
}

func builderRuntimeDeclaredFieldNames(content string) map[string]struct{} {
	if strings.TrimSpace(content) == "" {
		return nil
	}
	matches := builderRuntimeDeclaredFieldPattern.FindAllStringSubmatch(content, -1)
	if len(matches) == 0 {
		return nil
	}
	fields := make(map[string]struct{}, len(matches))
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		field := strings.TrimSpace(match[1])
		if field == "" {
			continue
		}
		fields[field] = struct{}{}
	}
	return fields
}

func builderRuntimeReferencedMemberNames(content, receiver string) []string {
	trimmedReceiver := strings.TrimSpace(receiver)
	if trimmedReceiver == "" || strings.TrimSpace(content) == "" {
		return nil
	}
	pattern := regexp.MustCompile(`\b` + regexp.QuoteMeta(trimmedReceiver) + `\.([A-Za-z_][A-Za-z0-9_]*)\b`)
	matches := pattern.FindAllStringSubmatch(content, -1)
	if len(matches) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	fields := make([]string, 0, len(matches))
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		field := strings.TrimSpace(match[1])
		if field == "" {
			continue
		}
		if field == "dart" {
			continue
		}
		if _, ok := seen[field]; ok {
			continue
		}
		seen[field] = struct{}{}
		fields = append(fields, field)
	}
	sort.Strings(fields)
	return fields
}

func builderRuntimeSemanticFileContent(workspacePath string, operations []appruns.WorkspacePatchOperation, targetPath string) (string, error) {
	normalizedTarget := filepath.ToSlash(strings.TrimSpace(targetPath))
	if normalizedTarget == "" {
		return "", nil
	}
	for i := len(operations) - 1; i >= 0; i-- {
		operation := operations[i]
		if filepath.ToSlash(strings.TrimSpace(operation.Path)) != normalizedTarget {
			continue
		}
		if operation.Type == "write_file" {
			return operation.Content, nil
		}
		if operation.Type == "replace_block" {
			return operation.NewContent, nil
		}
	}
	if strings.TrimSpace(workspacePath) == "" {
		return "", nil
	}
	content, err := os.ReadFile(filepath.Join(workspacePath, filepath.FromSlash(normalizedTarget)))
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	return string(content), nil
}

func builderRuntimeIsDartPathWithin(path, prefix string) bool {
	normalizedPath := filepath.ToSlash(strings.TrimSpace(path))
	normalizedPrefix := filepath.ToSlash(strings.TrimSpace(prefix))
	if normalizedPath == "" || normalizedPrefix == "" {
		return false
	}
	if !strings.HasSuffix(normalizedPrefix, "/") {
		normalizedPrefix += "/"
	}
	return strings.HasPrefix(normalizedPath, normalizedPrefix) && strings.HasSuffix(normalizedPath, ".dart")
}

func builderRuntimeDirectoryContainsTypeLikeSymbol(workspacePath string, operations []appruns.WorkspacePatchOperation, prefix, symbol string) (bool, error) {
	normalizedPrefix := filepath.ToSlash(strings.TrimSpace(prefix))
	if normalizedPrefix == "" || strings.TrimSpace(symbol) == "" {
		return false, nil
	}
	if !strings.HasSuffix(normalizedPrefix, "/") {
		normalizedPrefix += "/"
	}
	seen := map[string]struct{}{}
	for i := len(operations) - 1; i >= 0; i-- {
		operation := operations[i]
		normalizedPath := filepath.ToSlash(strings.TrimSpace(operation.Path))
		if !builderRuntimeIsDartPathWithin(normalizedPath, normalizedPrefix) {
			continue
		}
		if _, ok := seen[normalizedPath]; ok {
			continue
		}
		seen[normalizedPath] = struct{}{}
		content := operation.Content
		switch operation.Type {
		case "replace_block":
			content = operation.NewContent
		case "delete_file":
			content = ""
		}
		if builderRuntimeContentDefinesTypeLikeSymbol(content, symbol) {
			return true, nil
		}
	}
	if strings.TrimSpace(workspacePath) == "" {
		return false, nil
	}
	dirPath := filepath.Join(workspacePath, filepath.FromSlash(normalizedPrefix))
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".dart") {
			continue
		}
		normalizedPath := normalizedPrefix + entry.Name()
		if _, ok := seen[normalizedPath]; ok {
			continue
		}
		content, err := os.ReadFile(filepath.Join(dirPath, entry.Name()))
		if err != nil {
			return false, err
		}
		if builderRuntimeContentDefinesTypeLikeSymbol(string(content), symbol) {
			return true, nil
		}
	}
	return false, nil
}

func builderRuntimeContentDeclaresField(content, field string) bool {
	trimmedField := strings.TrimSpace(field)
	if trimmedField == "" || strings.TrimSpace(content) == "" {
		return false
	}
	pattern := regexp.MustCompile(`(?m)^\s*(?:final|late\s+final|late|var|const)\s+[A-Za-z0-9_<>,? ]+\s+` + regexp.QuoteMeta(trimmedField) + `\s*(?:=|;)`)
	return pattern.MatchString(content)
}

func builderRuntimeContentImportsEntryModel(content string) bool {
	if strings.TrimSpace(content) == "" {
		return false
	}
	return strings.Contains(content, "models/entry.dart")
}

func builderRuntimeContentDefinesTypeLikeSymbol(content, symbol string) bool {
	trimmedSymbol := strings.TrimSpace(symbol)
	if trimmedSymbol == "" || strings.TrimSpace(content) == "" {
		return false
	}
	pattern := regexp.MustCompile(`(?m)^\s*(?:abstract\s+|base\s+|final\s+|sealed\s+)?(?:class|enum|mixin)\s+` + regexp.QuoteMeta(trimmedSymbol) + `\b|^\s*typedef\s+` + regexp.QuoteMeta(trimmedSymbol) + `\b`)
	return pattern.MatchString(content)
}

func builderRuntimeOperationRewritesWholeFile(operations []appruns.WorkspacePatchOperation, targetPath string) bool {
	normalizedTarget := filepath.ToSlash(strings.TrimSpace(targetPath))
	if normalizedTarget == "" {
		return false
	}
	for _, operation := range operations {
		if filepath.ToSlash(strings.TrimSpace(operation.Path)) != normalizedTarget {
			continue
		}
		if operation.Type == "write_file" || operation.Type == "delete_file" {
			return true
		}
	}
	return false
}

func validateBuilderRuntimeModelExportStability(workspacePath string, operations []appruns.WorkspacePatchOperation) error {
	if strings.TrimSpace(workspacePath) == "" {
		return nil
	}
	removableExports, err := builderRuntimeRemovableLegacyExports(workspacePath)
	if err != nil {
		return err
	}
	violations := make([]string, 0)
	for _, operation := range operations {
		path := filepath.ToSlash(strings.TrimSpace(operation.Path))
		if !strings.HasPrefix(path, "lib/models/") || !strings.HasSuffix(path, ".dart") {
			continue
		}
		content := operation.Content
		if operation.Type == "replace_block" {
			content = operation.NewContent
		}
		if strings.TrimSpace(content) == "" {
			continue
		}
		currentContent, err := os.ReadFile(filepath.Join(workspacePath, filepath.FromSlash(path)))
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return fmt.Errorf("read model file for export stability: %w", err)
		}
		currentExports := builderRuntimeExportedDartTypes(string(currentContent))
		if len(currentExports) == 0 {
			continue
		}
		updatedExports := builderRuntimeExportedDartTypes(content)
		missingRequired, unexpectedNew := builderRuntimeExportStabilityDiff(currentExports, updatedExports, removableExports)
		if len(missingRequired) == 0 && len(unexpectedNew) == 0 {
			continue
		}
		parts := make([]string, 0, 2)
		if len(missingRequired) > 0 {
			parts = append(parts, fmt.Sprintf("missing required exports %s", strings.Join(missingRequired, ", ")))
		}
		if len(unexpectedNew) > 0 {
			parts = append(parts, fmt.Sprintf("unexpected new exports %s", strings.Join(unexpectedNew, ", ")))
		}
		violations = append(violations, fmt.Sprintf("%s -> %s => %s (%s)", path, strings.Join(currentExports, ", "), strings.Join(updatedExports, ", "), strings.Join(parts, "; ")))
	}
	if len(violations) == 0 {
		return nil
	}
	sort.Strings(violations)
	return fmt.Errorf("patch renamed exported Dart model types without coordinated dependent updates: %s", strings.Join(violations, "; "))
}

func builderRuntimeExportStabilityDiff(currentExports, updatedExports []string, removableExports map[string]struct{}) ([]string, []string) {
	required := make([]string, 0, len(currentExports))
	currentSet := make(map[string]struct{}, len(currentExports))
	updatedSet := make(map[string]struct{}, len(updatedExports))
	for _, exportName := range currentExports {
		currentSet[exportName] = struct{}{}
		if _, removable := removableExports[exportName]; removable {
			continue
		}
		required = append(required, exportName)
	}
	for _, exportName := range updatedExports {
		updatedSet[exportName] = struct{}{}
	}
	missingRequired := make([]string, 0)
	for _, exportName := range required {
		if _, ok := updatedSet[exportName]; !ok {
			missingRequired = append(missingRequired, exportName)
		}
	}
	unexpectedNew := make([]string, 0)
	for _, exportName := range updatedExports {
		if _, ok := currentSet[exportName]; ok {
			continue
		}
		unexpectedNew = append(unexpectedNew, exportName)
	}
	sort.Strings(missingRequired)
	sort.Strings(unexpectedNew)
	return missingRequired, unexpectedNew
}

func builderRuntimeExportedDartTypes(content string) []string {
	matches := builderRuntimeExportedDartTypePattern.FindAllStringSubmatch(content, -1)
	if len(matches) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	result := make([]string, 0, len(matches))
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		name := strings.TrimSpace(match[1])
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		result = append(result, name)
	}
	sort.Strings(result)
	return result
}

func builderRuntimeForbiddenSchemaTokens(workspacePath string) ([]string, error) {
	if strings.TrimSpace(workspacePath) == "" {
		return nil, nil
	}
	if forbidden, err := builderRuntimeForbiddenTokensFromDomainModel(workspacePath); err != nil {
		return nil, err
	} else if len(forbidden) > 0 {
		return forbidden, nil
	}
	recordContent := []byte(nil)
	if recordModelPath := builderRuntimeOpenLitePrimaryRecordModelPath(workspacePath); strings.TrimSpace(recordModelPath) != "" {
		content, err := os.ReadFile(filepath.Join(workspacePath, filepath.FromSlash(recordModelPath)))
		if err != nil {
			if !os.IsNotExist(err) {
				return nil, fmt.Errorf("read primary record model for semantic consistency: %w", err)
			}
		} else {
			recordContent = content
		}
	}
	summaryContent := []byte(nil)
	summaryModelPath := filepath.Join(workspacePath, "lib", "models", "dashboard_summary.dart")
	if content, readErr := os.ReadFile(summaryModelPath); readErr == nil {
		summaryContent = content
	} else if !os.IsNotExist(readErr) {
		return nil, fmt.Errorf("read dashboard summary model for semantic consistency: %w", readErr)
	}
	forbidden := make([]string, 0)
	addTokens := func(enabled bool, tokens ...string) {
		if !enabled {
			return
		}
		forbidden = append(forbidden, tokens...)
	}
	recordText := string(recordContent)
	summaryText := string(summaryContent)
	fieldSemantics := builderRuntimeOpenLiteCollectionFieldSemantics(workspacePath)
	addTokens(strings.TrimSpace(fieldSemantics.statusField) == "", "RecordStatus", ".status", "selectedStatus", "setStatus(", "statusLabel(", "inboxFilterLabel", "inProgressFilterLabel", "doneFilterLabel")
	addTokens(!strings.Contains(recordText, "title"), "titleController", "titleFieldLabel", "titleFieldRequiredError")
	addTokens(!strings.Contains(recordText, "category"), "categoryController", "categoryFieldLabel", "setCategory(", "categories")
	addTokens(!strings.Contains(recordText, "updatedAt"), ".updatedAt")
	addTokens(!strings.Contains(summaryText, "inboxCount"), "inboxCount")
	addTokens(!strings.Contains(summaryText, "inProgressCount"), "inProgressCount")
	addTokens(!strings.Contains(summaryText, "doneCount"), "doneCount")
	addTokens(!strings.Contains(summaryText, "totalCount"), "totalCount")
	if len(forbidden) == 0 {
		return nil, nil
	}
	seen := map[string]struct{}{}
	deduped := make([]string, 0, len(forbidden))
	for _, token := range forbidden {
		if _, ok := seen[token]; ok {
			continue
		}
		seen[token] = struct{}{}
		deduped = append(deduped, token)
	}
	sort.Strings(deduped)
	return deduped, nil
}

func builderRuntimeForbiddenTokensFromDomainModel(workspacePath string) ([]string, error) {
	fieldSet, err := builderRuntimeDomainModelFieldSet(workspacePath)
	if err != nil {
		return nil, err
	}
	if len(fieldSet) == 0 {
		return nil, nil
	}
	hasField := func(name string) bool {
		_, ok := fieldSet[strings.ToLower(strings.TrimSpace(name))]
		return ok
	}
	forbidden := make([]string, 0)
	addTokens := func(enabled bool, tokens ...string) {
		if !enabled {
			return
		}
		forbidden = append(forbidden, tokens...)
	}
	addTokens(!hasField("status"), "RecordStatus", ".status", "selectedStatus", "setStatus(", "statusLabel(", "inboxFilterLabel", "inProgressFilterLabel", "doneFilterLabel", "RecordListFilter")
	addTokens(!hasField("title"), "titleController", "titleFieldLabel", "titleFieldRequiredError")
	addTokens(!hasField("category"), "categoryController", "categoryFieldLabel", "detailCategoryLabel", "setCategory(", "categories")
	addTokens(!hasField("updated_at") && !hasField("updatedAt"), ".updatedAt")
	addTokens(!hasField("total_count") && !hasField("totalCount"), "totalCount")
	addTokens(!hasField("inbox_count") && !hasField("inboxCount"), "inboxCount")
	addTokens(!hasField("in_progress_count") && !hasField("inProgressCount"), "inProgressCount")
	addTokens(!hasField("done_count") && !hasField("doneCount"), "doneCount")
	seen := map[string]struct{}{}
	deduped := make([]string, 0, len(forbidden))
	for _, token := range forbidden {
		if _, ok := seen[token]; ok {
			continue
		}
		seen[token] = struct{}{}
		deduped = append(deduped, token)
	}
	sort.Strings(deduped)
	return deduped, nil
}

func builderRuntimeDomainModelFieldSet(workspacePath string) (map[string]struct{}, error) {
	jobRoot := filepath.Dir(workspacePath)
	domainModelPath := filepath.Join(jobRoot, "prepare", "domain-model.json")
	content, err := os.ReadFile(domainModelPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read prepare domain model for semantic consistency: %w", err)
	}
	var payload struct {
		Entities []struct {
			Fields []struct {
				Name string `json:"name"`
			} `json:"fields"`
		} `json:"entities"`
	}
	if err := json.Unmarshal(content, &payload); err != nil {
		return nil, fmt.Errorf("parse prepare domain model for semantic consistency: %w", err)
	}
	fieldSet := map[string]struct{}{}
	for _, entity := range payload.Entities {
		for _, field := range entity.Fields {
			name := strings.TrimSpace(strings.ToLower(field.Name))
			if name == "" {
				continue
			}
			fieldSet[name] = struct{}{}
		}
	}
	return fieldSet, nil
}

func builderRuntimeRemovableLegacyExports(workspacePath string) (map[string]struct{}, error) {
	fieldSet, err := builderRuntimeDomainModelFieldSet(workspacePath)
	if err != nil {
		return nil, err
	}
	removable := map[string]struct{}{}
	hasField := func(name string) bool {
		_, ok := fieldSet[strings.ToLower(strings.TrimSpace(name))]
		return ok
	}
	if len(fieldSet) == 0 {
		return removable, nil
	}
	if !hasField("status") {
		removable["RecordStatus"] = struct{}{}
		removable["RecordStatusHelper"] = struct{}{}
		removable["RecordListFilter"] = struct{}{}
	}
	return removable, nil
}

func builderRuntimeRemovableLegacyExportNames(workspacePath string) ([]string, error) {
	removable, err := builderRuntimeRemovableLegacyExports(workspacePath)
	if err != nil {
		return nil, err
	}
	if len(removable) == 0 {
		return nil, nil
	}
	names := make([]string, 0, len(removable))
	for name := range removable {
		names = append(names, name)
	}
	sort.Strings(names)
	return names, nil
}

var builderRuntimeExportedDartTypePattern = regexp.MustCompile(`(?m)^\s*(?:abstract\s+|base\s+|final\s+|sealed\s+)?(?:class|enum|mixin)\s+([A-Z][A-Za-z0-9_]*)\b`)
var builderRuntimeExportedDartClassPattern = regexp.MustCompile(`(?m)^\s*(?:abstract\s+|base\s+|final\s+|sealed\s+)?class\s+([A-Z][A-Za-z0-9_]*)\b`)
var builderRuntimeDartPackageImportPattern = regexp.MustCompile(`(?m)^\s*import\s+['"]package:([^/'"]+)/[^'"]+['"];`)
var builderRuntimeDartFieldPattern = regexp.MustCompile(`(?m)^\s*(?:final|late\s+final)\s+[A-Za-z_][A-Za-z0-9_<>,? ]*\s+([a-z][A-Za-z0-9_]*)\s*;`)
var builderRuntimeDartTypedFieldPattern = regexp.MustCompile(`(?m)^\s*(?:final|late\s+final)\s+([A-Za-z_][A-Za-z0-9_<>,? ]*)\s+([a-z][A-Za-z0-9_]*)\s*;`)
var builderRuntimeDeclaredFieldPattern = regexp.MustCompile(`(?m)^\s*(?:final|late\s+final|late|var|const)\s+[A-Za-z0-9_<>,? ]+\s+([a-zA-Z_][A-Za-z0-9_]*)\s*(?:=|;)`)
var builderRuntimeDartEnumValuePattern = regexp.MustCompile(`(?m)^\s*([a-z][A-Za-z0-9_]*)\s*,\s*$`)
var builderRuntimeUndefinedTypeLikeSymbolPatterns = []*regexp.Regexp{
	regexp.MustCompile(`'([A-Za-z_][A-Za-z0-9_]*)' isn't defined`),
	regexp.MustCompile(`Undefined name '([A-Za-z_][A-Za-z0-9_]*)'`),
	regexp.MustCompile(`The (?:method|getter|setter|function) '([A-Za-z_][A-Za-z0-9_]*)' isn't defined`),
	regexp.MustCompile(`The name '([A-Za-z_][A-Za-z0-9_]*)' isn't a class`),
}
var builderRuntimeUndefinedMemberOwnerPattern = regexp.MustCompile(`The (?:method|getter|setter|function) '([A-Za-z_][A-Za-z0-9_]*)' isn't defined for the type '([A-Za-z_][A-Za-z0-9_]*)'`)
var builderRuntimeMissingPackageImportPattern = regexp.MustCompile(`The imported package '([^']+)' isn't a dependency|Target of URI doesn't exist: 'package:([^/'"]+)/[^']*'`)
var builderRuntimeUpdatedAtCascadeSortPattern = regexp.MustCompile(`\n[ \t]*\.\.sort\(\(left, right\) => right\.updatedAt\.compareTo\(left\.updatedAt\)\);`)
var builderRuntimeUpdatedAtStatementSortPattern = regexp.MustCompile(`(?m)^\s*[A-Za-z_][A-Za-z0-9_]*\.sort\(\(left, right\) => right\.updatedAt\.compareTo\(left\.updatedAt\)\);\n?`)
var builderRuntimeHomePageUpdatedAtTextPattern = regexp.MustCompile(`Text\(\s*'\$\{record\.updatedAt\.month\.toString\(\)\.padLeft\(2, '0'\)\}-\$\{record\.updatedAt\.day\.toString\(\)\.padLeft\(2, '0'\)\}'\s*,\s*style:\s*const TextStyle\(fontWeight:\s*FontWeight\.w600\),\s*\),`)
var builderRuntimeHomePageUpdatedAtToLocalTextPattern = regexp.MustCompile(`Text\(\s*'\$\{record\.updatedAt\?\.toLocal\(\)\.toString\(\)\.split\(" "\)\[0\]\}'\s*,\s*style:\s*const TextStyle\(fontWeight:\s*FontWeight\.w600\),\s*\),`)
var builderRuntimeRecordListPageUpdatedAtTextPattern = regexp.MustCompile(`Text\(\s*'\$\{record\.updatedAt\.year\}-\$\{record\.updatedAt\.month\.toString\(\)\.padLeft\(2, '0'\)\}-\$\{record\.updatedAt\.day\.toString\(\)\.padLeft\(2, '0'\)\}'\s*,\s*style:\s*const TextStyle\(fontWeight:\s*FontWeight\.w600\),\s*\),`)
var builderRuntimeMainNullReturningDetailCallbackPattern = regexp.MustCompile(`(?s)onOpen[A-Za-z0-9_]*Detail\s*:\s*\([^\)]*\)\s*(?:async\s*)?\{\s*(?:(?://[^\n]*\n)|/\*.*?\*/|\s)*\}`)
var builderRuntimeMainNullReturningDetailCallbackNamePattern = regexp.MustCompile(`(?s)(onOpen[A-Za-z0-9_]*Detail)\s*:\s*\([^\)]*\)\s*(?:async\s*)?\{\s*(?:(?://[^\n]*\n)|/\*.*?\*/|\s)*\}`)
var builderRuntimeWidgetTestPumpWidgetRepositoryPattern = regexp.MustCompile(`(?s)pumpWidget\(\s*(?:const\s+)?([A-Z][A-Za-z0-9_]*)\(\s*repository\s*:`)
var builderRuntimeRunAppWidgetPattern = regexp.MustCompile(`runApp\(\s*(?:const\s+)?([A-Z][A-Za-z0-9_]*)\s*\(`)
var builderRuntimeAndroidBuildGradleFlutterSourcePattern = regexp.MustCompile(`(?m)^(\s*source\s*=\s*)"\.\."\.\.(\s*)$`)
var builderRuntimeAndroidBuildGradleNamespaceIdentifierPattern = regexp.MustCompile(`(?m)^(\s*namespace\s*=\s*)defaultOpenLiteApplication(\s*)$`)
var builderRuntimeRecordRepositoryBoxNamePattern = regexp.MustCompile(`(?m)^\s*static\s+const\s+String\s+_boxName\s*=\s*['"]([^'"]+)['"];`)
var builderRuntimeRecordRepositoryRecordsKeyPattern = regexp.MustCompile(`(?m)^\s*static\s+const\s+String\s+_recordsKey\s*=\s*['"]([^'"]+)['"];`)
var builderRuntimeRepositoryPrimaryTypePattern = regexp.MustCompile(`Future<List<([A-Z][A-Za-z0-9_]*)>>\s+load[A-Za-z0-9_]*\(`)
var builderRuntimeRepositoryModelImportPattern = regexp.MustCompile(`(?m)^import\s+'\.\./models/([^']+\.dart)';`)
var builderRuntimeRepositoryDeleteDeclarationPattern = regexp.MustCompile(`(?m)^\s*Future<void>\s+deleteRecord\([^\n]*\);\n?`)
var builderRuntimeRepositoryDeleteImplementationPattern = regexp.MustCompile(`(?s)\n\s*@override\s*\n\s*Future<void>\s+deleteRecord\([^\)]*\)\s+async\s*\{.*?\n\s*\}\n`)
var builderRuntimeRepositoryDeleteAsyncPattern = regexp.MustCompile(`(?s)\n\s*Future<void>\s+deleteRecord\([^\)]*\)\s+async\s*\{.*?\n\s*\}\n`)
var builderRuntimeRecordModelMalformedStatusSwitchCasePattern = regexp.MustCompile(`(?m)^(\s*'[^']+'):\s*(RecordStatus\.[A-Za-z0-9_]+,)`)
var builderRuntimeRecordModelMissingNoteDefaultPattern = regexp.MustCompile(`(?m)^(\s*this\.note),\s*$`)
var builderRuntimeNamedDartTypeDeclarationPattern = regexp.MustCompile(`(?m)^\s*(?:abstract\s+|base\s+|final\s+|sealed\s+)?(class|enum)\s+([A-Z][A-Za-z0-9_]*)\b[^\{]*\{`)
var builderRuntimeCollectionCreateEntryCallbackPattern = regexp.MustCompile(`\bonCreate[A-Z][A-Za-z0-9_]*\b`)
var builderRuntimeConstructedDetailTypePattern = regexp.MustCompile(`\b([A-Z][A-Za-z0-9_]*Detail[A-Za-z0-9_]*)\s*\(`)

func normalizeBuilderRuntimePatchForWorkspace(workspacePath string, patch *appruns.WorkspacePatch) {
	normalizeBuilderRuntimePatchForWorkspaceWithTaskBundle(workspacePath, nil, patch)
}

func normalizeBuilderRuntimePatchForWorkspaceWithTaskBundle(workspacePath string, taskBundle []appruns.TaskBundleItem, patch *appruns.WorkspacePatch) {
	normalizeBuilderRuntimePatchForWorkspaceWithTaskBundleAndTargetPaths(workspacePath, taskBundle, concreteTaskTargetPaths(taskBundle), patch)
}

func normalizeBuilderRuntimePatchForWorkspaceWithTaskBundleAndTargetPaths(workspacePath string, taskBundle []appruns.TaskBundleItem, targetPaths []string, patch *appruns.WorkspacePatch) {
	if patch == nil || strings.TrimSpace(workspacePath) == "" {
		return
	}
	allowDeleteFlow := true
	allowFilterFlow := true
	needsCollectionCreateEntry := builderRuntimeNeedsCollectionCreateEntry(taskBundle, workspacePath)
	if len(taskBundle) > 0 {
		allowDeleteFlow = builderRuntimeTaskBundleHasSemanticIntentRef(taskBundle, "ac-delete")
		allowFilterFlow = builderRuntimeTaskBundleHasSemanticIntentRef(taskBundle, "ac-filter")
	}
	for index := range patch.Operations {
		operation := &patch.Operations[index]
		normalizedPath := filepath.ToSlash(strings.TrimSpace(operation.Path))
		if strings.HasSuffix(normalizedPath, ".dart") {
			if operation.Type == "replace_block" {
				operation.NewContent = normalizeBuilderRuntimeDeprecatedDartContent(operation.NewContent)
			} else {
				operation.Content = normalizeBuilderRuntimeDeprecatedDartContent(operation.Content)
			}
		}
		// fallback-only: 以下规范化仅在 LLM 模型路径触发，emit 路径已在上游直接返回
		switch {
		case builderRuntimeIsRelationRichModelPath(normalizedPath):
			if operation.Type == "replace_block" {
				operation.NewContent = normalizeBuilderRuntimeRelationRichModelContent(workspacePath, targetPaths, normalizedPath, operation.NewContent)
				continue
			}
			operation.Content = normalizeBuilderRuntimeRelationRichModelContent(workspacePath, targetPaths, normalizedPath, operation.Content)
		case normalizedPath == "lib/models/record.dart":
			if operation.Type == "replace_block" {
				operation.NewContent = normalizeBuilderRuntimeRecordModelContent(operation.NewContent)
				continue
			}
			operation.Content = normalizeBuilderRuntimeRecordModelContent(operation.Content)
		case builderRuntimeShouldNormalizeOverviewControllerPath(normalizedPath):
			if operation.Type == "replace_block" {
				operation.NewContent = normalizeBuilderRuntimeHomeControllerContent(workspacePath, operation.NewContent)
				continue
			}
			operation.Content = normalizeBuilderRuntimeHomeControllerContent(workspacePath, operation.Content)
		case builderRuntimeLikelyOverviewSurfaceViewPath(normalizedPath):
			if operation.Type == "replace_block" {
				operation.NewContent = normalizeBuilderRuntimeHomePageContent(workspacePath, normalizedPath, operation.NewContent)
				continue
			}
			operation.Content = normalizeBuilderRuntimeHomePageContent(workspacePath, normalizedPath, operation.Content)
		case builderRuntimeShouldNormalizeMutationControllerPath(workspacePath, normalizedPath):
			if operation.Type == "replace_block" {
				operation.NewContent = normalizeBuilderRuntimeRecordFormControllerContent(workspacePath, operation.NewContent)
				continue
			}
			operation.Content = normalizeBuilderRuntimeRecordFormControllerContent(workspacePath, operation.Content)
		case builderRuntimeShouldNormalizeCollectionControllerPath(workspacePath, normalizedPath):
			if operation.Type == "replace_block" {
				operation.NewContent = normalizeBuilderRuntimeRecordListControllerContent(workspacePath, allowFilterFlow, needsCollectionCreateEntry, operation.NewContent)
				continue
			}
			operation.Content = normalizeBuilderRuntimeRecordListControllerContent(workspacePath, allowFilterFlow, needsCollectionCreateEntry, operation.Content)
		case builderRuntimeIsDartPathWithin(normalizedPath, "lib/repositories/"):
			if operation.Type == "replace_block" {
				operation.NewContent = normalizeBuilderRuntimeRecordRepositoryContent(workspacePath, operation.NewContent, allowDeleteFlow)
				continue
			}
			operation.Content = normalizeBuilderRuntimeRecordRepositoryContent(workspacePath, operation.Content, allowDeleteFlow)
		case builderRuntimeIsDartPathWithin(normalizedPath, "lib/template/"):
			if operation.Type == "replace_block" {
				operation.NewContent = normalizeBuilderRuntimeTemplateCopyContent(workspacePath, operation.NewContent)
				continue
			}
			operation.Content = normalizeBuilderRuntimeTemplateCopyContent(workspacePath, operation.Content)
		case normalizedPath == "lib/main.dart":
			if operation.Type == "replace_block" {
				operation.NewContent = normalizeBuilderRuntimeMainContent(workspacePath, needsCollectionCreateEntry, operation.NewContent)
				continue
			}
			operation.Content = normalizeBuilderRuntimeMainContent(workspacePath, needsCollectionCreateEntry, operation.Content)
		case strings.HasPrefix(normalizedPath, "lib/views/") && strings.HasSuffix(normalizedPath, ".dart"):
			if operation.Type == "replace_block" {
				operation.NewContent = normalizeBuilderRuntimeViewContent(workspacePath, allowFilterFlow, needsCollectionCreateEntry, operation.NewContent)
				continue
			}
			operation.Content = normalizeBuilderRuntimeViewContent(workspacePath, allowFilterFlow, needsCollectionCreateEntry, operation.Content)
		case normalizedPath == "android/app/build.gradle.kts":
			if operation.Type == "replace_block" {
				operation.NewContent = normalizeBuilderRuntimeAndroidBuildGradleContent(operation.NewContent)
				continue
			}
			operation.Content = normalizeBuilderRuntimeAndroidBuildGradleContent(operation.Content)
		case normalizedPath == "test/widget_test.dart":
			if operation.Type == "replace_block" {
				operation.NewContent = normalizeBuilderRuntimeWidgetTestContent(workspacePath, needsCollectionCreateEntry, operation.NewContent)
				continue
			}
			operation.Content = normalizeBuilderRuntimeWidgetTestContent(workspacePath, needsCollectionCreateEntry, operation.Content)
		}
	}
	if !allowFilterFlow {
		patch.Operations = pruneUnexpectedNoFilterCollectionControllerOperations(workspacePath, taskBundle, patch.Operations)
	}
	patch.Operations = pruneUnexpectedDependentOperationsFromSingleTargetNonRepairPatch(workspacePath, taskBundle, patch.Operations, targetPaths)
	patch.Operations = pruneUnexpectedDependentOperationsFromMainPatch(patch.Operations, targetPaths)
}

func builderRuntimeNeedsCollectionCreateEntry(taskBundle []appruns.TaskBundleItem, workspacePath string) bool {
	if len(taskBundle) > 0 {
		hasExplicitSurfaceTopology := builderRuntimeTaskBundleHasAnySurfaceRef(
			taskBundle,
			builderRuntimeOverviewSurfaceRef,
			builderRuntimeCollectionSurfaceRef,
			builderRuntimeMutationSurfaceRef,
		)
		if hasExplicitSurfaceTopology {
			hasOverviewSurface := builderRuntimeTaskBundleHasSurfaceRef(taskBundle, builderRuntimeOverviewSurfaceRef)
			hasCollectionSurface := builderRuntimeTaskBundleHasSurfaceRef(taskBundle, builderRuntimeCollectionSurfaceRef)
			hasMutationSurface := builderRuntimeTaskBundleHasSurfaceRef(taskBundle, builderRuntimeMutationSurfaceRef)
			if hasCollectionSurface && hasMutationSurface && !hasOverviewSurface {
				return true
			}
			return false
		}
		hasOverviewSurface := containsTargetPath(taskBundle, "lib/views/home_page.dart") || containsTargetPath(taskBundle, "lib/controllers/home_controller.dart")
		hasCollectionSurface := containsTargetPath(taskBundle, "lib/views/record_list_page.dart") || containsTargetPath(taskBundle, "lib/controllers/record_list_controller.dart")
		hasMutationSurface := containsTargetPath(taskBundle, "lib/views/record_form_page.dart") || containsTargetPath(taskBundle, "lib/controllers/record_form_controller.dart")
		if hasCollectionSurface && hasMutationSurface && !hasOverviewSurface {
			return true
		}
		if hasOverviewSurface {
			return false
		}
		likelyOverviewViewPaths, likelyOverviewControllerPaths := builderRuntimeLikelyOverviewSurfaceTargetPaths(taskBundle)
		likelyCollectionViewPaths, likelyCollectionControllerPaths := builderRuntimeLikelyCollectionSurfaceTargetPaths(taskBundle)
		likelyMutationViewPaths, likelyMutationControllerPaths := builderRuntimeLikelyMutationSurfaceTargetPaths(taskBundle)
		hasOverviewSurface = hasOverviewSurface || len(likelyOverviewViewPaths) > 0 || len(likelyOverviewControllerPaths) > 0
		hasCollectionSurface = hasCollectionSurface || len(likelyCollectionViewPaths) > 0 || len(likelyCollectionControllerPaths) > 0
		hasMutationSurface = hasMutationSurface || len(likelyMutationViewPaths) > 0 || len(likelyMutationControllerPaths) > 0
		if hasCollectionSurface && hasMutationSurface && !hasOverviewSurface {
			if builderRuntimeOpenLiteWorkspaceHasOverviewSurface(workspacePath) {
				return false
			}
			return true
		}
		if hasOverviewSurface || builderRuntimeOpenLiteWorkspaceHasOverviewSurface(workspacePath) {
			return false
		}
	}
	return builderRuntimeOpenLiteNeedsCollectionCreateEntryFromWorkspace(workspacePath)
}

func pruneUnexpectedNoFilterCollectionControllerOperations(workspacePath string, taskBundle []appruns.TaskBundleItem, operations []appruns.WorkspacePatchOperation) []appruns.WorkspacePatchOperation {
	if len(operations) == 0 {
		return operations
	}
	collectionViewPaths, collectionControllerPaths := builderRuntimeCollectionSurfaceTargetPaths(taskBundle)
	if len(collectionViewPaths) == 0 || len(collectionControllerPaths) == 0 {
		likelyViewPaths, likelyControllerPaths := builderRuntimeLikelyCollectionSurfaceTargetPaths(taskBundle)
		if len(collectionViewPaths) == 0 {
			collectionViewPaths = likelyViewPaths
		}
		if len(collectionControllerPaths) == 0 {
			collectionControllerPaths = likelyControllerPaths
		}
	}
	if len(collectionViewPaths) == 0 {
		collectionViewPaths = []string{"lib/views/record_list_page.dart"}
	}
	if len(collectionControllerPaths) == 0 {
		collectionControllerPaths = []string{"lib/controllers/record_list_controller.dart"}
	}
	hasExistingCollectionController := false
	for _, controllerPath := range collectionControllerPaths {
		if builderRuntimeWorkspaceHasSemanticFile(workspacePath, controllerPath) {
			hasExistingCollectionController = true
			break
		}
	}
	if !hasExistingCollectionController {
		return operations
	}
	hasListPageOperation := false
	for _, operation := range operations {
		if slices.Contains(collectionViewPaths, filepath.ToSlash(strings.TrimSpace(operation.Path))) {
			hasListPageOperation = true
			break
		}
	}
	if !hasListPageOperation {
		return operations
	}
	filtered := make([]appruns.WorkspacePatchOperation, 0, len(operations))
	removed := false
	for _, operation := range operations {
		if slices.Contains(collectionControllerPaths, filepath.ToSlash(strings.TrimSpace(operation.Path))) {
			removed = true
			continue
		}
		filtered = append(filtered, operation)
	}
	if !removed {
		return operations
	}
	return filtered
}

func builderRuntimeCollectionSurfaceTargetPaths(taskBundle []appruns.TaskBundleItem) ([]string, []string) {
	return builderRuntimeSurfaceTargetPaths(taskBundle, builderRuntimeCollectionSurfaceRef)
}

func builderRuntimeLikelyCollectionSurfaceTargetPaths(taskBundle []appruns.TaskBundleItem) ([]string, []string) {
	return builderRuntimeLikelySurfaceTargetPaths(taskBundle, builderRuntimeLikelyCollectionSurfaceViewPath, builderRuntimeLikelyCollectionControllerPath)
}

func builderRuntimeLikelyMutationSurfaceTargetPaths(taskBundle []appruns.TaskBundleItem) ([]string, []string) {
	return builderRuntimeLikelySurfaceTargetPaths(taskBundle, builderRuntimeLikelyMutationSurfaceViewPath, builderRuntimeLikelyMutationControllerPath)
}

func builderRuntimeLikelyOverviewSurfaceTargetPaths(taskBundle []appruns.TaskBundleItem) ([]string, []string) {
	return builderRuntimeLikelySurfaceTargetPaths(taskBundle, builderRuntimeLikelyOverviewSurfaceViewPath, builderRuntimeLikelyOverviewControllerPath)
}

func builderRuntimeInspectionSurfaceTargetPaths(taskBundle []appruns.TaskBundleItem) ([]string, []string) {
	return builderRuntimeSurfaceTargetPaths(taskBundle, builderRuntimeInspectionSurfaceRef)
}

func builderRuntimeShouldNormalizeCollectionControllerPath(workspacePath, path string) bool {
	normalizedPath := filepath.ToSlash(strings.TrimSpace(path))
	if normalizedPath == "" {
		return false
	}
	if normalizedPath == "lib/controllers/record_list_controller.dart" {
		return true
	}
	if builderRuntimeLikelyCollectionControllerPath(normalizedPath) {
		return true
	}
	candidate := builderRuntimePrimaryCollectionControllerCandidate(workspacePath)
	return filepath.ToSlash(strings.TrimSpace(candidate.path)) == normalizedPath
}

func builderRuntimeShouldNormalizeMutationControllerPath(workspacePath, path string) bool {
	normalizedPath := filepath.ToSlash(strings.TrimSpace(path))
	if normalizedPath == "" {
		return false
	}
	if normalizedPath == "lib/controllers/record_form_controller.dart" {
		return true
	}
	if hintedPath := builderRuntimePrimaryMutationControllerPathHint(workspacePath); hintedPath != "" && hintedPath == normalizedPath {
		return true
	}
	return builderRuntimeLikelyMutationControllerPath(normalizedPath)
}

func builderRuntimeShouldNormalizeOverviewControllerPath(path string) bool {
	normalizedPath := filepath.ToSlash(strings.TrimSpace(path))
	if normalizedPath == "" {
		return false
	}
	if normalizedPath == "lib/controllers/home_controller.dart" {
		return true
	}
	return builderRuntimeLikelyOverviewControllerPath(normalizedPath)
}

func builderRuntimePrimaryMutationControllerPathHint(workspacePath string) string {
	surface := builderRuntimePrimaryMutationSurfaceCandidate(workspacePath)
	if path := filepath.ToSlash(strings.TrimSpace(surface.controller.path)); path != "" {
		return path
	}
	if surface.view.path == "" {
		return ""
	}
	return filepath.ToSlash(strings.TrimSpace(builderRuntimeControllerPathHintForViewPath(surface.view.path)))
}

func builderRuntimeLikelyMutationControllerPath(path string) bool {
	normalizedPath := filepath.ToSlash(strings.TrimSpace(path))
	if !strings.HasPrefix(normalizedPath, "lib/controllers/") || !strings.HasSuffix(normalizedPath, ".dart") {
		return false
	}
	baseName := strings.TrimSuffix(filepath.Base(normalizedPath), ".dart")
	if baseName == "" {
		return false
	}
	tokens := strings.Split(strings.ToLower(baseName), "_")
	hasMutationToken := false
	hasControllerToken := false
	for _, token := range tokens {
		switch token {
		case "form", "edit", "mutation":
			hasMutationToken = true
		case "controller":
			hasControllerToken = true
		case "home", "overview", "list", "collection":
			return false
		}
	}
	return hasMutationToken && hasControllerToken
}

func builderRuntimeLikelyCollectionControllerPath(path string) bool {
	normalizedPath := filepath.ToSlash(strings.TrimSpace(path))
	if !strings.HasPrefix(normalizedPath, "lib/controllers/") || !strings.HasSuffix(normalizedPath, ".dart") {
		return false
	}
	baseName := strings.TrimSuffix(filepath.Base(normalizedPath), ".dart")
	if baseName == "" {
		return false
	}
	tokens := strings.Split(strings.ToLower(baseName), "_")
	hasCollectionToken := false
	hasControllerToken := false
	for _, token := range tokens {
		switch token {
		case "list", "collection":
			hasCollectionToken = true
		case "controller":
			hasControllerToken = true
		case "home", "overview", "form", "edit", "mutation", "detail", "inspection":
			return false
		}
	}
	return hasCollectionToken && hasControllerToken
}

func builderRuntimeLikelyOverviewControllerPath(path string) bool {
	normalizedPath := filepath.ToSlash(strings.TrimSpace(path))
	if !strings.HasPrefix(normalizedPath, "lib/controllers/") || !strings.HasSuffix(normalizedPath, ".dart") {
		return false
	}
	baseName := strings.TrimSuffix(filepath.Base(normalizedPath), ".dart")
	if baseName == "" {
		return false
	}
	tokens := strings.Split(strings.ToLower(baseName), "_")
	hasHomeToken := false
	hasOverviewToken := false
	hasControllerToken := false
	for _, token := range tokens {
		switch token {
		case "home":
			hasHomeToken = true
		case "overview":
			hasOverviewToken = true
		case "controller":
			hasControllerToken = true
		case "form", "edit", "mutation", "list", "collection", "detail", "inspection":
			return false
		}
	}
	return hasControllerToken && (hasHomeToken || hasOverviewToken)
}

func builderRuntimeMutationSurfaceTargetPaths(taskBundle []appruns.TaskBundleItem) ([]string, []string) {
	return builderRuntimeSurfaceTargetPaths(taskBundle, builderRuntimeMutationSurfaceRef)
}

func builderRuntimeSurfaceTargetPaths(taskBundle []appruns.TaskBundleItem, surfaceRef string) ([]string, []string) {
	if len(taskBundle) == 0 {
		return nil, nil
	}
	viewSet := make(map[string]struct{})
	controllerSet := make(map[string]struct{})
	hasExplicitSurface := false
	for _, task := range taskBundle {
		if !builderRuntimeTaskHasSurfaceRef(task, surfaceRef) {
			continue
		}
		hasExplicitSurface = true
		for _, targetPath := range task.TargetPaths {
			normalized := filepath.ToSlash(strings.TrimSpace(targetPath))
			if strings.HasPrefix(normalized, "lib/views/") && strings.HasSuffix(normalized, ".dart") {
				viewSet[normalized] = struct{}{}
				continue
			}
			if strings.HasPrefix(normalized, "lib/controllers/") && strings.HasSuffix(normalized, ".dart") {
				controllerSet[normalized] = struct{}{}
			}
		}
	}
	if !hasExplicitSurface {
		return nil, nil
	}
	views := make([]string, 0, len(viewSet))
	for path := range viewSet {
		views = append(views, path)
	}
	slices.Sort(views)
	controllers := make([]string, 0, len(controllerSet))
	for path := range controllerSet {
		controllers = append(controllers, path)
	}
	slices.Sort(controllers)
	return views, controllers
}

func builderRuntimeLikelySurfaceTargetPaths(taskBundle []appruns.TaskBundleItem, matchesView func(string) bool, matchesController func(string) bool) ([]string, []string) {
	if len(taskBundle) == 0 {
		return nil, nil
	}
	viewSet := make(map[string]struct{})
	controllerSet := make(map[string]struct{})
	for _, task := range taskBundle {
		for _, targetPath := range task.TargetPaths {
			normalized := filepath.ToSlash(strings.TrimSpace(targetPath))
			if normalized == "" {
				continue
			}
			if matchesView != nil && matchesView(normalized) {
				viewSet[normalized] = struct{}{}
				continue
			}
			if matchesController != nil && matchesController(normalized) {
				controllerSet[normalized] = struct{}{}
			}
		}
	}
	views := make([]string, 0, len(viewSet))
	for path := range viewSet {
		views = append(views, path)
	}
	slices.Sort(views)
	controllers := make([]string, 0, len(controllerSet))
	for path := range controllerSet {
		controllers = append(controllers, path)
	}
	slices.Sort(controllers)
	return views, controllers
}

func builderRuntimeTargetPathsContainLikelySurface(targetPaths []string, matchesView func(string) bool, matchesController func(string) bool) bool {
	for _, targetPath := range targetPaths {
		normalized := filepath.ToSlash(strings.TrimSpace(targetPath))
		if normalized == "" {
			continue
		}
		if matchesView != nil && matchesView(normalized) {
			return true
		}
		if matchesController != nil && matchesController(normalized) {
			return true
		}
	}
	return false
}

func pruneUnexpectedDependentOperationsFromMainPatch(operations []appruns.WorkspacePatchOperation, targetPaths []string) []appruns.WorkspacePatchOperation {
	if len(operations) == 0 || !builderRuntimeTargetsOnlyPath(targetPaths, "lib/main.dart") {
		return operations
	}
	hasMainOperation := false
	hasPrunableDependentOperation := false
	hasOtherOperation := false
	for _, operation := range operations {
		normalized := filepath.ToSlash(strings.TrimSpace(operation.Path))
		switch {
		case normalized == "lib/main.dart":
			hasMainOperation = true
		case builderRuntimeIsPrunableMainPatchDependentPath(normalized):
			hasPrunableDependentOperation = true
		default:
			hasOtherOperation = true
		}
	}
	if !hasMainOperation || !hasPrunableDependentOperation || hasOtherOperation {
		return operations
	}
	filtered := make([]appruns.WorkspacePatchOperation, 0, len(operations))
	for _, operation := range operations {
		normalized := filepath.ToSlash(strings.TrimSpace(operation.Path))
		if normalized != "lib/main.dart" && builderRuntimeIsPrunableMainPatchDependentPath(normalized) {
			continue
		}
		filtered = append(filtered, operation)
	}
	return filtered
}

func pruneUnexpectedDependentOperationsFromSingleTargetNonRepairPatch(workspacePath string, taskBundle []appruns.TaskBundleItem, operations []appruns.WorkspacePatchOperation, targetPaths []string) []appruns.WorkspacePatchOperation {
	if len(operations) == 0 || len(targetPaths) != 1 {
		return operations
	}
	switch builderRuntimeTaskTypeForTargetPaths(taskBundle, targetPaths) {
	case appruns.BuilderRuntimeTaskTypeAnalyzeRepair, appruns.BuilderRuntimeTaskTypeTestRepair, appruns.BuilderRuntimeTaskTypeClosureRepair:
		return operations
	}
	targetPath := filepath.ToSlash(strings.TrimSpace(targetPaths[0]))
	if targetPath == "" {
		return operations
	}
	prunableDependents := builderRuntimeSingleTargetPrunableDependentPaths(workspacePath, taskBundle, targetPath)
	if len(prunableDependents) == 0 {
		return operations
	}
	hasTargetOperation := false
	hasPrunableDependentOperation := false
	for _, operation := range operations {
		normalizedPath := filepath.ToSlash(strings.TrimSpace(operation.Path))
		if normalizedPath == targetPath {
			hasTargetOperation = true
			continue
		}
		if _, ok := prunableDependents[normalizedPath]; ok {
			hasPrunableDependentOperation = true
		}
	}
	if !hasTargetOperation || !hasPrunableDependentOperation {
		return operations
	}
	filtered := make([]appruns.WorkspacePatchOperation, 0, len(operations))
	for _, operation := range operations {
		normalizedPath := filepath.ToSlash(strings.TrimSpace(operation.Path))
		if normalizedPath == targetPath {
			filtered = append(filtered, operation)
			continue
		}
		if _, ok := prunableDependents[normalizedPath]; ok {
			continue
		}
		filtered = append(filtered, operation)
	}
	if len(filtered) == 0 {
		return operations
	}
	return filtered
}

func builderRuntimeOverviewSurfaceTargetPaths(taskBundle []appruns.TaskBundleItem) ([]string, []string) {
	return builderRuntimeSurfaceTargetPaths(taskBundle, builderRuntimeOverviewSurfaceRef)
}

func builderRuntimeSingleTargetPrunableDependentPaths(workspacePath string, taskBundle []appruns.TaskBundleItem, targetPath string) map[string]struct{} {
	normalizedTargetPath := filepath.ToSlash(strings.TrimSpace(targetPath))
	if normalizedTargetPath == "" {
		return nil
	}
	surfacePrunable := builderRuntimeSingleTargetSurfacePrunableDependentPaths(taskBundle, normalizedTargetPath)
	legacyPrunable := builderRuntimeSingleTargetLegacyPrunableDependentPaths(workspacePath, normalizedTargetPath)
	if len(surfacePrunable) == 0 {
		return legacyPrunable
	}
	if len(legacyPrunable) == 0 {
		return surfacePrunable
	}
	merged := make(map[string]struct{}, len(surfacePrunable)+len(legacyPrunable))
	for dependentPath := range surfacePrunable {
		merged[dependentPath] = struct{}{}
	}
	for dependentPath := range legacyPrunable {
		merged[dependentPath] = struct{}{}
	}
	return merged
}

func builderRuntimeSingleTargetSurfacePrunableDependentPaths(taskBundle []appruns.TaskBundleItem, targetPath string) map[string]struct{} {
	overviewViewPaths, overviewControllerPaths := builderRuntimeOverviewSurfaceTargetPaths(taskBundle)
	if len(overviewViewPaths) == 0 || len(overviewControllerPaths) == 0 {
		likelyOverviewViewPaths, likelyOverviewControllerPaths := builderRuntimeLikelyOverviewSurfaceTargetPaths(taskBundle)
		if len(overviewViewPaths) == 0 {
			overviewViewPaths = likelyOverviewViewPaths
		}
		if len(overviewControllerPaths) == 0 {
			overviewControllerPaths = likelyOverviewControllerPaths
		}
	}
	if slices.Contains(overviewViewPaths, targetPath) {
		prunable := make(map[string]struct{}, len(overviewControllerPaths)+1)
		for _, controllerPath := range overviewControllerPaths {
			prunable[controllerPath] = struct{}{}
		}
		prunable["lib/template/open_lite_copy.dart"] = struct{}{}
		return prunable
	}
	collectionViewPaths, collectionControllerPaths := builderRuntimeCollectionSurfaceTargetPaths(taskBundle)
	if len(collectionViewPaths) == 0 || len(collectionControllerPaths) == 0 {
		likelyCollectionViewPaths, likelyCollectionControllerPaths := builderRuntimeLikelyCollectionSurfaceTargetPaths(taskBundle)
		if len(collectionViewPaths) == 0 {
			collectionViewPaths = likelyCollectionViewPaths
		}
		if len(collectionControllerPaths) == 0 {
			collectionControllerPaths = likelyCollectionControllerPaths
		}
	}
	if slices.Contains(collectionViewPaths, targetPath) {
		prunable := make(map[string]struct{}, len(collectionControllerPaths)+2)
		for _, controllerPath := range collectionControllerPaths {
			prunable[controllerPath] = struct{}{}
		}
		prunable["lib/template/open_lite_copy.dart"] = struct{}{}
		prunable["lib/template/open_lite_pop.dart"] = struct{}{}
		return prunable
	}
	mutationViewPaths, mutationControllerPaths := builderRuntimeMutationSurfaceTargetPaths(taskBundle)
	if len(mutationViewPaths) == 0 || len(mutationControllerPaths) == 0 {
		likelyMutationViewPaths, likelyMutationControllerPaths := builderRuntimeLikelyMutationSurfaceTargetPaths(taskBundle)
		if len(mutationViewPaths) == 0 {
			mutationViewPaths = likelyMutationViewPaths
		}
		if len(mutationControllerPaths) == 0 {
			mutationControllerPaths = likelyMutationControllerPaths
		}
	}
	if slices.Contains(mutationViewPaths, targetPath) {
		prunable := make(map[string]struct{}, len(mutationControllerPaths))
		for _, controllerPath := range mutationControllerPaths {
			prunable[controllerPath] = struct{}{}
		}
		return prunable
	}
	inspectionViewPaths, _ := builderRuntimeInspectionSurfaceTargetPaths(taskBundle)
	if slices.Contains(inspectionViewPaths, targetPath) {
		return map[string]struct{}{
			"lib/template/open_lite_copy.dart": {},
		}
	}
	return nil
}

func builderRuntimeSingleTargetLegacyPrunableDependentPaths(workspacePath, targetPath string) map[string]struct{} {
	normalizedTargetPath := filepath.ToSlash(strings.TrimSpace(targetPath))
	if prunable := builderRuntimeGenericRepositoryPrunableDependentPaths(normalizedTargetPath); len(prunable) > 0 {
		return prunable
	}
	switch normalizedTargetPath {
	case "lib/views/record_list_page.dart":
		return map[string]struct{}{
			"lib/controllers/record_list_controller.dart": {},
			"lib/template/open_lite_copy.dart":            {},
			"lib/template/open_lite_pop.dart":             {},
		}
	case "lib/views/home_page.dart":
		return map[string]struct{}{
			"lib/controllers/home_controller.dart": {},
			"lib/template/open_lite_copy.dart":     {},
		}
	case "lib/views/record_form_page.dart":
		return map[string]struct{}{
			"lib/controllers/record_form_controller.dart": {},
		}
	default:
		if prunable := builderRuntimeSingleTargetLegacyCollectionViewPrunableDependentPaths(workspacePath, normalizedTargetPath); len(prunable) > 0 {
			return prunable
		}
		if prunable := builderRuntimeSingleTargetLegacyOverviewViewPrunableDependentPaths(normalizedTargetPath); len(prunable) > 0 {
			return prunable
		}
		if prunable := builderRuntimeSingleTargetLegacyDetailViewPrunableDependentPaths(normalizedTargetPath); len(prunable) > 0 {
			return prunable
		}
		return builderRuntimeSingleTargetLegacyMutationViewPrunableDependentPaths(normalizedTargetPath)
	}
}

func builderRuntimeSingleTargetLegacyCollectionViewPrunableDependentPaths(workspacePath, targetPath string) map[string]struct{} {
	if !builderRuntimeLikelyCollectionSurfaceViewPath(targetPath) {
		return nil
	}
	prunable := map[string]struct{}{
		"lib/template/open_lite_copy.dart": {},
		"lib/template/open_lite_pop.dart":  {},
	}
	controllerHint := builderRuntimeControllerPathHintForViewPath(targetPath)
	controllerPath := ""
	if candidate := builderRuntimePrimaryCollectionControllerCandidate(workspacePath, controllerHint, targetPath); candidate.path != "" {
		controllerPath = filepath.ToSlash(strings.TrimSpace(candidate.path))
	}
	if controllerPath == "" {
		normalizedHint := filepath.ToSlash(strings.TrimSpace(controllerHint))
		if normalizedHint != "" && builderRuntimeWorkspaceHasSemanticFile(workspacePath, normalizedHint) {
			controllerPath = normalizedHint
		}
	}
	if controllerPath != "" {
		prunable[controllerPath] = struct{}{}
	}
	return prunable
}

func builderRuntimeLikelyCollectionSurfaceViewPath(path string) bool {
	normalizedPath := filepath.ToSlash(strings.TrimSpace(path))
	if !strings.HasPrefix(normalizedPath, "lib/views/") || !strings.HasSuffix(normalizedPath, ".dart") {
		return false
	}
	tokens := builderRuntimeViewPathTokens(normalizedPath)
	if len(tokens) == 0 {
		return false
	}
	hasCollectionToken := false
	hasPageToken := false
	for _, token := range tokens {
		switch token {
		case "list", "collection":
			hasCollectionToken = true
		case "page", "view":
			hasPageToken = true
		case "detail", "form", "home", "overview":
			return false
		}
	}
	return hasCollectionToken && hasPageToken
}

func builderRuntimeSingleTargetLegacyOverviewViewPrunableDependentPaths(targetPath string) map[string]struct{} {
	if !builderRuntimeLikelyOverviewSurfaceViewPath(targetPath) {
		return nil
	}
	prunable := map[string]struct{}{
		"lib/template/open_lite_copy.dart": {},
	}
	if controllerPath := filepath.ToSlash(strings.TrimSpace(builderRuntimeControllerPathHintForViewPath(targetPath))); controllerPath != "" {
		prunable[controllerPath] = struct{}{}
	}
	return prunable
}

func builderRuntimeLikelyOverviewSurfaceViewPath(path string) bool {
	normalizedPath := filepath.ToSlash(strings.TrimSpace(path))
	if !strings.HasPrefix(normalizedPath, "lib/views/") || !strings.HasSuffix(normalizedPath, ".dart") {
		return false
	}
	tokens := builderRuntimeViewPathTokens(normalizedPath)
	if len(tokens) == 0 {
		return false
	}
	hasOverviewToken := false
	hasPageToken := false
	for _, token := range tokens {
		switch token {
		case "home", "overview":
			hasOverviewToken = true
		case "page", "view":
			hasPageToken = true
		case "detail", "form", "list", "collection":
			return false
		}
	}
	return hasOverviewToken && hasPageToken
}

func builderRuntimeSingleTargetLegacyDetailViewPrunableDependentPaths(targetPath string) map[string]struct{} {
	if !builderRuntimeLikelyDetailSurfaceViewPath(targetPath) {
		return nil
	}
	return map[string]struct{}{
		"lib/template/open_lite_copy.dart": {},
	}
}

func builderRuntimeLikelyDetailSurfaceViewPath(path string) bool {
	normalizedPath := filepath.ToSlash(strings.TrimSpace(path))
	if !strings.HasPrefix(normalizedPath, "lib/views/") || !strings.HasSuffix(normalizedPath, ".dart") {
		return false
	}
	tokens := builderRuntimeViewPathTokens(normalizedPath)
	if len(tokens) == 0 {
		return false
	}
	hasDetailToken := false
	hasPageToken := false
	for _, token := range tokens {
		switch token {
		case "detail", "inspection":
			hasDetailToken = true
		case "page", "view":
			hasPageToken = true
		case "form", "edit", "mutation", "home", "overview", "list", "collection":
			return false
		}
	}
	return hasDetailToken && hasPageToken
}

func builderRuntimeSingleTargetLegacyMutationViewPrunableDependentPaths(targetPath string) map[string]struct{} {
	if !builderRuntimeLikelyMutationSurfaceViewPath(targetPath) {
		return nil
	}
	if controllerPath := filepath.ToSlash(strings.TrimSpace(builderRuntimeControllerPathHintForViewPath(targetPath))); controllerPath != "" {
		return map[string]struct{}{controllerPath: {}}
	}
	return nil
}

func builderRuntimeLikelyMutationSurfaceViewPath(path string) bool {
	normalizedPath := filepath.ToSlash(strings.TrimSpace(path))
	if !strings.HasPrefix(normalizedPath, "lib/views/") || !strings.HasSuffix(normalizedPath, ".dart") {
		return false
	}
	tokens := builderRuntimeViewPathTokens(normalizedPath)
	if len(tokens) == 0 {
		return false
	}
	hasMutationToken := false
	hasPageToken := false
	for _, token := range tokens {
		switch token {
		case "form", "edit", "mutation":
			hasMutationToken = true
		case "page", "view":
			hasPageToken = true
		case "detail", "home", "overview", "list", "collection":
			return false
		}
	}
	return hasMutationToken && hasPageToken
}

func builderRuntimeViewPathTokens(path string) []string {
	baseName := strings.TrimSuffix(filepath.Base(filepath.ToSlash(strings.TrimSpace(path))), ".dart")
	if baseName == "" {
		return nil
	}
	return strings.Split(strings.ToLower(baseName), "_")
}

func builderRuntimeControllerPathHintForViewPath(path string) string {
	normalizedPath := filepath.ToSlash(strings.TrimSpace(path))
	if !strings.HasPrefix(normalizedPath, "lib/views/") {
		return ""
	}
	controllerPath := strings.Replace(normalizedPath, "lib/views/", "lib/controllers/", 1)
	switch {
	case strings.HasSuffix(controllerPath, "_page.dart"):
		return strings.TrimSuffix(controllerPath, "_page.dart") + "_controller.dart"
	case strings.HasSuffix(controllerPath, "_view.dart"):
		return strings.TrimSuffix(controllerPath, "_view.dart") + "_controller.dart"
	default:
		return controllerPath
	}
}

func builderRuntimeGenericRepositoryPrunableDependentPaths(targetPath string) map[string]struct{} {
	if !strings.HasPrefix(targetPath, "lib/repositories/") || !strings.HasSuffix(targetPath, "_repository.dart") {
		return nil
	}
	baseName := strings.TrimSuffix(filepath.Base(targetPath), ".dart")
	if baseName == "" || strings.HasPrefix(baseName, "hive_") || strings.HasPrefix(baseName, "in_memory_") {
		return nil
	}
	return map[string]struct{}{
		filepath.ToSlash(filepath.Join("lib", "repositories", "hive_"+baseName+".dart")):      {},
		filepath.ToSlash(filepath.Join("lib", "repositories", "in_memory_"+baseName+".dart")): {},
	}
}

func builderRuntimeTaskTypeForTargetPaths(taskBundle []appruns.TaskBundleItem, targetPaths []string) appruns.BuilderRuntimeTaskType {
	if len(taskBundle) == 0 || len(targetPaths) == 0 {
		return ""
	}
	normalizedTargetPaths := normalizeBuilderRuntimeTargetPathList(targetPaths)
	for _, task := range taskBundle {
		normalized := appruns.NormalizeTaskBundleItem(task)
		if slices.Equal(normalizeBuilderRuntimeTargetPathList(concreteTaskTargetPaths([]appruns.TaskBundleItem{normalized})), normalizedTargetPaths) {
			return normalized.TaskType
		}
	}
	return ""
}

func normalizeBuilderRuntimeTargetPathList(paths []string) []string {
	if len(paths) == 0 {
		return nil
	}
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
	sort.Strings(normalized)
	return normalized
}

func builderRuntimeIsPrunableMainPatchDependentPath(path string) bool {
	path = filepath.ToSlash(strings.TrimSpace(path))
	if path == "test/widget_test.dart" {
		return true
	}
	return strings.HasPrefix(path, "lib/views/") && strings.HasSuffix(path, ".dart")
}

func normalizeBuilderRuntimeDeprecatedDartContent(content string) string {
	if strings.TrimSpace(content) == "" || builderRuntimeFlutterWithValuesEnabled() || !strings.Contains(content, ".withValues(") {
		return content
	}
	const needle = ".withValues("
	var builder strings.Builder
	searchStart := 0
	for searchStart < len(content) {
		matchIndex := strings.Index(content[searchStart:], needle)
		if matchIndex < 0 {
			builder.WriteString(content[searchStart:])
			break
		}
		matchIndex += searchStart
		builder.WriteString(content[searchStart:matchIndex])
		openParen := matchIndex + len(needle) - 1
		closeParen := builderRuntimeMatchingParenIndex(content, openParen)
		if closeParen < 0 {
			builder.WriteString(content[matchIndex:])
			break
		}
		args := content[openParen+1 : closeParen]
		if alphaExpr, ok := builderRuntimeWithValuesAlphaOnlyArgument(args); ok {
			builder.WriteString(".withOpacity(")
			builder.WriteString(alphaExpr)
			builder.WriteString(")")
		} else {
			builder.WriteString(content[matchIndex : closeParen+1])
		}
		searchStart = closeParen + 1
	}
	return builder.String()
}

func builderRuntimeFlutterWithValuesEnabled() bool {
	value, ok := os.LookupEnv("APPFACTORY_FLUTTER_ENABLE_WITH_VALUES")
	if !ok {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func builderRuntimeWithValuesAlphaOnlyArgument(args string) (string, bool) {
	trimmedArgs := strings.TrimSpace(args)
	if trimmedArgs == "" {
		return "", false
	}
	firstArg := strings.TrimSpace(builderRuntimeFirstTopLevelArgument(trimmedArgs))
	if firstArg == "" || firstArg != trimmedArgs {
		return "", false
	}
	parts := strings.SplitN(firstArg, ":", 2)
	if len(parts) != 2 || strings.TrimSpace(parts[0]) != "alpha" {
		return "", false
	}
	alphaExpr := strings.TrimSpace(parts[1])
	if alphaExpr == "" {
		return "", false
	}
	return alphaExpr, true
}

func normalizeBuilderRuntimeMainContent(workspacePath string, needsCollectionCreateEntry bool, content string) string {
	return normalizeBuilderRuntimeOpenLiteMainContent(workspacePath, needsCollectionCreateEntry, content)
}

type builderRuntimeRelationRichModelProfile string

const (
	builderRuntimeRelationRichProjectTaskTagProfile     builderRuntimeRelationRichModelProfile = "project-task-tag"
	builderRuntimeRelationRichInventorySheetLineProfile builderRuntimeRelationRichModelProfile = "inventory-sheet-line-item"
)

func builderRuntimeIsRelationRichModelPath(path string) bool {
	normalizedPath := filepath.ToSlash(strings.TrimSpace(path))
	for _, profile := range []builderRuntimeRelationRichModelProfile{
		builderRuntimeRelationRichProjectTaskTagProfile,
		builderRuntimeRelationRichInventorySheetLineProfile,
	} {
		if builderRuntimeRelationRichModelPathAllowedForProfile(normalizedPath, profile) {
			return true
		}
	}
	return false
}

func builderRuntimeTargetsRelationRichModelSlice(paths []string) bool {
	if len(paths) == 0 {
		return false
	}
	for _, path := range paths {
		if !builderRuntimeIsRelationRichModelPath(path) {
			return false
		}
	}
	return true
}

func builderRuntimeRelationRichDomainWorkspace(workspacePath string) bool {
	return builderRuntimeRelationRichModelProfileForWorkspace(workspacePath) != ""
}

func normalizeBuilderRuntimeRelationRichModelContent(workspacePath string, targetPaths []string, path, content string) string {
	profile := builderRuntimeRelationRichModelProfileForContext(workspacePath, targetPaths)
	if !builderRuntimeShouldCanonicalizeRelationRichModel(profile, path, content) {
		return content
	}
	if canonical := builderRuntimeCanonicalRelationRichModel(profile, path); strings.TrimSpace(canonical) != "" {
		return canonical
	}
	return content
}

func builderRuntimeShouldCanonicalizeRelationRichModel(profile builderRuntimeRelationRichModelProfile, path, content string) bool {
	normalizedPath := filepath.ToSlash(strings.TrimSpace(path))
	if profile == "" || !builderRuntimeIsRelationRichModelPath(normalizedPath) || strings.TrimSpace(content) == "" {
		return false
	}
	for _, marker := range []string{
		"package:hive/hive.dart",
		"@HiveType",
		"@HiveField",
		"HiveObject",
		"part '",
		"TypeAdapter",
	} {
		if strings.Contains(content, marker) {
			return true
		}
	}
	for _, marker := range builderRuntimeRelationRichModelRequiredMarkers(profile, normalizedPath) {
		if !strings.Contains(content, marker) {
			return true
		}
	}
	return false
}

func builderRuntimeRelationRichModelRequiredMarkers(profile builderRuntimeRelationRichModelProfile, path string) []string {
	switch profile {
	case builderRuntimeRelationRichProjectTaskTagProfile:
		switch filepath.ToSlash(strings.TrimSpace(path)) {
		case "lib/models/project.dart":
			return []string{
				"class Project {",
				"factory Project.fromJson(Map<String, dynamic> json)",
				"Map<String, dynamic> toJson()",
				"enum ProjectStatus {",
			}
		case "lib/models/task.dart":
			return []string{
				"class Task {",
				"factory Task.fromJson(Map<String, dynamic> json)",
				"Map<String, dynamic> toJson()",
				"enum TaskStatus {",
			}
		case "lib/models/tag.dart":
			return []string{
				"class Tag {",
				"factory Tag.fromJson(Map<String, dynamic> json)",
				"Map<String, dynamic> toJson()",
			}
		case "lib/models/task_tag_link.dart":
			return []string{
				"class TaskTagLink {",
				"factory TaskTagLink.fromJson(Map<String, dynamic> json)",
				"Map<String, dynamic> toJson()",
			}
		case "lib/models/dashboard_summary.dart":
			return []string{
				"class DashboardSummary {",
				"factory DashboardSummary.fromJson(Map<String, dynamic> json)",
				"Map<String, dynamic> toJson()",
				"final int doneTaskCount;",
				"final int taggedTaskCount;",
				"'done_task_count': doneTaskCount",
			}
		}
	case builderRuntimeRelationRichInventorySheetLineProfile:
		switch filepath.ToSlash(strings.TrimSpace(path)) {
		case "lib/models/inventory_sheet.dart":
			return []string{
				"class InventorySheet {",
				"factory InventorySheet.fromJson(Map<String, dynamic> json)",
				"Map<String, dynamic> toJson()",
				"enum InventorySheetStatus {",
			}
		case "lib/models/line_item.dart":
			return []string{
				"class LineItem {",
				"factory LineItem.fromJson(Map<String, dynamic> json)",
				"Map<String, dynamic> toJson()",
				"'variance_qty': varianceQty",
			}
		case "lib/models/sku.dart":
			return []string{
				"class Sku {",
				"factory Sku.fromJson(Map<String, dynamic> json)",
				"Map<String, dynamic> toJson()",
				"'reorder_threshold': reorderThreshold",
			}
		case "lib/models/warehouse.dart":
			return []string{
				"class Warehouse {",
				"factory Warehouse.fromJson(Map<String, dynamic> json)",
				"Map<String, dynamic> toJson()",
			}
		case "lib/models/dashboard_summary.dart":
			return []string{
				"class DashboardSummary {",
				"factory DashboardSummary.fromJson(Map<String, dynamic> json)",
				"Map<String, dynamic> toJson()",
				"final int lowStockSkuCount;",
				"final int varianceLineItemCount;",
				"'low_stock_sku_count': lowStockSkuCount",
			}
		}
	}
	return nil
}

func builderRuntimeCanonicalRelationRichModel(profile builderRuntimeRelationRichModelProfile, path string) string {
	switch profile {
	case builderRuntimeRelationRichProjectTaskTagProfile:
		switch filepath.ToSlash(strings.TrimSpace(path)) {
		case "lib/models/project.dart":
			return strings.Join([]string{
				"class Project {",
				"  final String projectId;",
				"  final String title;",
				"  final ProjectStatus status;",
				"  final String? color;",
				"",
				"  Project({",
				"    required this.projectId,",
				"    required this.title,",
				"    required this.status,",
				"    this.color,",
				"  });",
				"",
				"  factory Project.fromJson(Map<String, dynamic> json) {",
				"    return Project(",
				"      projectId: json['project_id'] as String,",
				"      title: json['title'] as String,",
				"      status: ProjectStatus.values.firstWhere(",
				"        (status) => status.name == json['status'],",
				"        orElse: () => ProjectStatus.active,",
				"      ),",
				"      color: json['color'] as String?,",
				"    );",
				"  }",
				"",
				"  Map<String, dynamic> toJson() {",
				"    return {",
				"      'project_id': projectId,",
				"      'title': title,",
				"      'status': status.name,",
				"      'color': color,",
				"    };",
				"  }",
				"}",
				"",
				"enum ProjectStatus {",
				"  active,",
				"  paused,",
				"  done",
				"}",
			}, "\n") + "\n"
		case "lib/models/task.dart":
			return strings.Join([]string{
				"class Task {",
				"  final String taskId;",
				"  final String projectId;",
				"  final String title;",
				"  final TaskStatus status;",
				"  final DateTime? dueOn;",
				"  final String? note;",
				"",
				"  Task({",
				"    required this.taskId,",
				"    required this.projectId,",
				"    required this.title,",
				"    required this.status,",
				"    this.dueOn,",
				"    this.note,",
				"  });",
				"",
				"  factory Task.fromJson(Map<String, dynamic> json) {",
				"    return Task(",
				"      taskId: json['task_id'] as String,",
				"      projectId: json['project_id'] as String,",
				"      title: json['title'] as String,",
				"      status: TaskStatus.values.firstWhere(",
				"        (status) => status.name == json['status'],",
				"        orElse: () => TaskStatus.todo,",
				"      ),",
				"      dueOn: json['due_on'] != null ? DateTime.parse(json['due_on'] as String) : null,",
				"      note: json['note'] as String?,",
				"    );",
				"  }",
				"",
				"  Map<String, dynamic> toJson() {",
				"    return {",
				"      'task_id': taskId,",
				"      'project_id': projectId,",
				"      'title': title,",
				"      'status': status.name,",
				"      'due_on': dueOn?.toIso8601String(),",
				"      'note': note,",
				"    };",
				"  }",
				"}",
				"",
				"enum TaskStatus {",
				"  todo,",
				"  doing,",
				"  done",
				"}",
			}, "\n") + "\n"
		case "lib/models/tag.dart":
			return strings.Join([]string{
				"class Tag {",
				"  final String tagId;",
				"  final String name;",
				"  final String? color;",
				"",
				"  Tag({",
				"    required this.tagId,",
				"    required this.name,",
				"    this.color,",
				"  });",
				"",
				"  factory Tag.fromJson(Map<String, dynamic> json) {",
				"    return Tag(",
				"      tagId: json['tag_id'] as String,",
				"      name: json['name'] as String,",
				"      color: json['color'] as String?,",
				"    );",
				"  }",
				"",
				"  Map<String, dynamic> toJson() {",
				"    return {",
				"      'tag_id': tagId,",
				"      'name': name,",
				"      'color': color,",
				"    };",
				"  }",
				"}",
			}, "\n") + "\n"
		case "lib/models/task_tag_link.dart":
			return strings.Join([]string{
				"class TaskTagLink {",
				"  final String linkId;",
				"  final String taskId;",
				"  final String tagId;",
				"",
				"  TaskTagLink({",
				"    required this.linkId,",
				"    required this.taskId,",
				"    required this.tagId,",
				"  });",
				"",
				"  factory TaskTagLink.fromJson(Map<String, dynamic> json) {",
				"    return TaskTagLink(",
				"      linkId: json['link_id'] as String,",
				"      taskId: json['task_id'] as String,",
				"      tagId: json['tag_id'] as String,",
				"    );",
				"  }",
				"",
				"  Map<String, dynamic> toJson() {",
				"    return {",
				"      'link_id': linkId,",
				"      'task_id': taskId,",
				"      'tag_id': tagId,",
				"    };",
				"  }",
				"}",
			}, "\n") + "\n"
		case "lib/models/dashboard_summary.dart":
			return strings.Join([]string{
				"class DashboardSummary {",
				"  final String projectId;",
				"  final int openTaskCount;",
				"  final int doneTaskCount;",
				"  final int taggedTaskCount;",
				"",
				"  const DashboardSummary({",
				"    required this.projectId,",
				"    required this.openTaskCount,",
				"    required this.doneTaskCount,",
				"    required this.taggedTaskCount,",
				"  });",
				"",
				"  factory DashboardSummary.fromJson(Map<String, dynamic> json) {",
				"    return DashboardSummary(",
				"      projectId: json['project_id'] as String,",
				"      openTaskCount: json['open_task_count'] as int,",
				"      doneTaskCount: json['done_task_count'] as int,",
				"      taggedTaskCount: json['tagged_task_count'] as int,",
				"    );",
				"  }",
				"",
				"  Map<String, dynamic> toJson() {",
				"    return {",
				"      'project_id': projectId,",
				"      'open_task_count': openTaskCount,",
				"      'done_task_count': doneTaskCount,",
				"      'tagged_task_count': taggedTaskCount,",
				"    };",
				"  }",
				"}",
			}, "\n") + "\n"
		}
	case builderRuntimeRelationRichInventorySheetLineProfile:
		switch filepath.ToSlash(strings.TrimSpace(path)) {
		case "lib/models/inventory_sheet.dart":
			return strings.Join([]string{
				"class InventorySheet {",
				"  final String sheetId;",
				"  final String warehouseId;",
				"  final InventorySheetStatus status;",
				"  final DateTime countedOn;",
				"  final String? note;",
				"",
				"  InventorySheet({",
				"    required this.sheetId,",
				"    required this.warehouseId,",
				"    required this.status,",
				"    required this.countedOn,",
				"    this.note,",
				"  });",
				"",
				"  factory InventorySheet.fromJson(Map<String, dynamic> json) {",
				"    return InventorySheet(",
				"      sheetId: json['sheet_id'] as String,",
				"      warehouseId: json['warehouse_id'] as String,",
				"      status: InventorySheetStatus.values.firstWhere(",
				"        (status) => status.name == json['status'],",
				"        orElse: () => InventorySheetStatus.draft,",
				"      ),",
				"      countedOn: DateTime.parse(json['counted_on'] as String),",
				"      note: json['note'] as String?,",
				"    );",
				"  }",
				"",
				"  Map<String, dynamic> toJson() {",
				"    return {",
				"      'sheet_id': sheetId,",
				"      'warehouse_id': warehouseId,",
				"      'status': status.name,",
				"      'counted_on': countedOn.toIso8601String(),",
				"      'note': note,",
				"    };",
				"  }",
				"}",
				"",
				"enum InventorySheetStatus {",
				"  draft,",
				"  checking,",
				"  closed",
				"}",
			}, "\n") + "\n"
		case "lib/models/line_item.dart":
			return strings.Join([]string{
				"class LineItem {",
				"  final String lineItemId;",
				"  final String sheetId;",
				"  final String skuId;",
				"  final int expectedQty;",
				"  final int countedQty;",
				"  final int varianceQty;",
				"",
				"  LineItem({",
				"    required this.lineItemId,",
				"    required this.sheetId,",
				"    required this.skuId,",
				"    required this.expectedQty,",
				"    required this.countedQty,",
				"    required this.varianceQty,",
				"  });",
				"",
				"  factory LineItem.fromJson(Map<String, dynamic> json) {",
				"    return LineItem(",
				"      lineItemId: json['line_item_id'] as String,",
				"      sheetId: json['sheet_id'] as String,",
				"      skuId: json['sku_id'] as String,",
				"      expectedQty: json['expected_qty'] as int,",
				"      countedQty: json['counted_qty'] as int,",
				"      varianceQty: json['variance_qty'] as int,",
				"    );",
				"  }",
				"",
				"  Map<String, dynamic> toJson() {",
				"    return {",
				"      'line_item_id': lineItemId,",
				"      'sheet_id': sheetId,",
				"      'sku_id': skuId,",
				"      'expected_qty': expectedQty,",
				"      'counted_qty': countedQty,",
				"      'variance_qty': varianceQty,",
				"    };",
				"  }",
				"}",
			}, "\n") + "\n"
		case "lib/models/sku.dart":
			return strings.Join([]string{
				"class Sku {",
				"  final String skuId;",
				"  final String name;",
				"  final String category;",
				"  final int reorderThreshold;",
				"",
				"  Sku({",
				"    required this.skuId,",
				"    required this.name,",
				"    required this.category,",
				"    required this.reorderThreshold,",
				"  });",
				"",
				"  factory Sku.fromJson(Map<String, dynamic> json) {",
				"    return Sku(",
				"      skuId: json['sku_id'] as String,",
				"      name: json['name'] as String,",
				"      category: json['category'] as String,",
				"      reorderThreshold: json['reorder_threshold'] as int,",
				"    );",
				"  }",
				"",
				"  Map<String, dynamic> toJson() {",
				"    return {",
				"      'sku_id': skuId,",
				"      'name': name,",
				"      'category': category,",
				"      'reorder_threshold': reorderThreshold,",
				"    };",
				"  }",
				"}",
			}, "\n") + "\n"
		case "lib/models/warehouse.dart":
			return strings.Join([]string{
				"class Warehouse {",
				"  final String warehouseId;",
				"  final String name;",
				"  final String? location;",
				"",
				"  Warehouse({",
				"    required this.warehouseId,",
				"    required this.name,",
				"    this.location,",
				"  });",
				"",
				"  factory Warehouse.fromJson(Map<String, dynamic> json) {",
				"    return Warehouse(",
				"      warehouseId: json['warehouse_id'] as String,",
				"      name: json['name'] as String,",
				"      location: json['location'] as String?,",
				"    );",
				"  }",
				"",
				"  Map<String, dynamic> toJson() {",
				"    return {",
				"      'warehouse_id': warehouseId,",
				"      'name': name,",
				"      'location': location,",
				"    };",
				"  }",
				"}",
			}, "\n") + "\n"
		case "lib/models/dashboard_summary.dart":
			return strings.Join([]string{
				"class DashboardSummary {",
				"  final String warehouseId;",
				"  final int openSheetCount;",
				"  final int lowStockSkuCount;",
				"  final int varianceLineItemCount;",
				"",
				"  const DashboardSummary({",
				"    required this.warehouseId,",
				"    required this.openSheetCount,",
				"    required this.lowStockSkuCount,",
				"    required this.varianceLineItemCount,",
				"  });",
				"",
				"  factory DashboardSummary.fromJson(Map<String, dynamic> json) {",
				"    return DashboardSummary(",
				"      warehouseId: json['warehouse_id'] as String,",
				"      openSheetCount: json['open_sheet_count'] as int,",
				"      lowStockSkuCount: json['low_stock_sku_count'] as int,",
				"      varianceLineItemCount: json['variance_line_item_count'] as int,",
				"    );",
				"  }",
				"",
				"  Map<String, dynamic> toJson() {",
				"    return {",
				"      'warehouse_id': warehouseId,",
				"      'open_sheet_count': openSheetCount,",
				"      'low_stock_sku_count': lowStockSkuCount,",
				"      'variance_line_item_count': varianceLineItemCount,",
				"    };",
				"  }",
				"}",
			}, "\n") + "\n"
		}
	}
	return ""
}

func builderRuntimeRelationRichModelPromptGuidance(profile builderRuntimeRelationRichModelProfile) string {
	switch profile {
	case builderRuntimeRelationRichProjectTaskTagProfile:
		return "For current relation-rich domain and summary model targets under lib/models/project.dart, lib/models/task.dart, lib/models/tag.dart, lib/models/task_tag_link.dart, and lib/models/dashboard_summary.dart, emit plain Dart data classes with enums plus fromJson/toJson helpers only. Do not import package:hive/hive.dart, do not add part '*.g.dart', and do not introduce HiveObject, @HiveType, @HiveField, or generated adapter dependencies in these files.\n"
	case builderRuntimeRelationRichInventorySheetLineProfile:
		return "For current relation-rich domain and summary model targets under lib/models/inventory_sheet.dart, lib/models/line_item.dart, lib/models/sku.dart, lib/models/warehouse.dart, and lib/models/dashboard_summary.dart, emit plain Dart data classes with enums plus fromJson/toJson helpers only. Keep inventory-sheet, line-item, SKU, warehouse, and low-stock-summary fields aligned to the current execution contract. Do not import package:hive/hive.dart, do not add part '*.g.dart', and do not introduce HiveObject, @HiveType, @HiveField, or generated adapter dependencies in these files.\n"
	default:
		return "For current relation-rich model targets, emit plain Dart data classes with enums plus fromJson/toJson helpers only. Do not import package:hive/hive.dart, do not add part '*.g.dart', and do not introduce HiveObject, @HiveType, @HiveField, or generated adapter dependencies in these files.\n"
	}
}

func builderRuntimeRelationRichModelProfileForContext(workspacePath string, targetPaths []string) builderRuntimeRelationRichModelProfile {
	if profile := builderRuntimeRelationRichModelProfileForPaths(targetPaths); profile != "" {
		return profile
	}
	return builderRuntimeRelationRichModelProfileForWorkspace(workspacePath)
}

func builderRuntimeRelationRichModelProfileForPaths(paths []string) builderRuntimeRelationRichModelProfile {
	if len(paths) == 0 {
		return ""
	}
	projectMatch := builderRuntimeRelationRichModelPathsBelongToProfile(paths, builderRuntimeRelationRichProjectTaskTagProfile)
	inventoryMatch := builderRuntimeRelationRichModelPathsBelongToProfile(paths, builderRuntimeRelationRichInventorySheetLineProfile)
	switch {
	case projectMatch && !inventoryMatch:
		return builderRuntimeRelationRichProjectTaskTagProfile
	case inventoryMatch && !projectMatch:
		return builderRuntimeRelationRichInventorySheetLineProfile
	default:
		return ""
	}
}

func builderRuntimeRelationRichModelProfileForWorkspace(workspacePath string) builderRuntimeRelationRichModelProfile {
	if strings.TrimSpace(workspacePath) == "" {
		return ""
	}
	for _, profile := range []builderRuntimeRelationRichModelProfile{
		builderRuntimeRelationRichProjectTaskTagProfile,
		builderRuntimeRelationRichInventorySheetLineProfile,
	} {
		matched := true
		for _, path := range builderRuntimeRelationRichModelCorePaths(profile) {
			if !builderRuntimeWorkspaceHasSemanticFile(workspacePath, path) {
				matched = false
				break
			}
		}
		if matched {
			return profile
		}
	}
	return ""
}

func builderRuntimeRelationRichModelPathsBelongToProfile(paths []string, profile builderRuntimeRelationRichModelProfile) bool {
	if len(paths) == 0 {
		return false
	}
	for _, path := range paths {
		if !builderRuntimeRelationRichModelPathAllowedForProfile(filepath.ToSlash(strings.TrimSpace(path)), profile) {
			return false
		}
	}
	return true
}

func builderRuntimeRelationRichModelPathAllowedForProfile(path string, profile builderRuntimeRelationRichModelProfile) bool {
	for _, candidate := range builderRuntimeRelationRichModelAllowedPaths(profile) {
		if path == candidate {
			return true
		}
	}
	return false
}

func builderRuntimeRelationRichModelAllowedPaths(profile builderRuntimeRelationRichModelProfile) []string {
	paths := append([]string(nil), builderRuntimeRelationRichModelCorePaths(profile)...)
	if profile == "" {
		return paths
	}
	return append(paths, "lib/models/dashboard_summary.dart")
}

func builderRuntimeRelationRichModelCorePaths(profile builderRuntimeRelationRichModelProfile) []string {
	switch profile {
	case builderRuntimeRelationRichProjectTaskTagProfile:
		return []string{
			"lib/models/project.dart",
			"lib/models/task.dart",
			"lib/models/tag.dart",
			"lib/models/task_tag_link.dart",
		}
	case builderRuntimeRelationRichInventorySheetLineProfile:
		return []string{
			"lib/models/inventory_sheet.dart",
			"lib/models/line_item.dart",
			"lib/models/sku.dart",
			"lib/models/warehouse.dart",
		}
	default:
		return nil
	}
}

func normalizeBuilderRuntimeRecordModelContent(content string) string {
	if strings.TrimSpace(content) == "" {
		return content
	}
	updated := builderRuntimeRecordModelMalformedStatusSwitchCasePattern.ReplaceAllString(content, "$1 => $2")
	updated = normalizeBuilderRuntimeRecordModelRedundantNullCoalescingTernaries(updated)
	if strings.Contains(updated, "final String note;") && !strings.Contains(updated, "final String? note;") {
		updated = builderRuntimeRecordModelMissingNoteDefaultPattern.ReplaceAllString(updated, "${1} = '',")
	}
	if !builderRuntimeRecordModelNeedsCollapse(updated) {
		return updated
	}
	if canonical := builderRuntimeCanonicalLatestRecordModel(updated); strings.TrimSpace(canonical) != "" {
		return canonical
	}
	return updated
}

func normalizeBuilderRuntimeRecordModelRedundantNullCoalescingTernaries(content string) string {
	if strings.TrimSpace(content) == "" || !strings.Contains(content, "??") || !strings.Contains(content, "?") || !strings.Contains(content, ":") {
		return content
	}
	lines := strings.Split(content, "\n")
	for index, line := range lines {
		lines[index] = normalizeBuilderRuntimeRecordModelRedundantNullCoalescingTernaryLine(line)
	}
	return strings.Join(lines, "\n")
}

func normalizeBuilderRuntimeRecordModelRedundantNullCoalescingTernaryLine(line string) string {
	trimmed := strings.TrimSpace(line)
	if !strings.HasSuffix(trimmed, ",") || !strings.Contains(trimmed, "??") || !strings.Contains(trimmed, ":") || strings.Count(trimmed, "?") < 2 {
		return line
	}
	body := strings.TrimSuffix(trimmed, ",")
	labelSeparator := strings.Index(body, ":")
	if labelSeparator < 0 {
		return line
	}
	fieldName := strings.TrimSpace(body[:labelSeparator])
	if fieldName == "" {
		return line
	}
	expr := strings.TrimSpace(body[labelSeparator+1:])
	coalesceIndex := strings.Index(expr, "??")
	if coalesceIndex < 0 {
		return line
	}
	fallback := strings.TrimSpace(expr[coalesceIndex+2:])
	questionIndex, colonIndex := builderRuntimeTopLevelTernaryIndexes(fallback)
	if questionIndex < 0 || colonIndex < 0 {
		return line
	}
	whenTrue := strings.TrimSpace(fallback[questionIndex+1 : colonIndex])
	whenFalse := strings.TrimSpace(fallback[colonIndex+1:])
	if whenTrue == "" || whenFalse == "" {
		return line
	}
	if normalizeBuilderRuntimeRecordModelWhitespace(whenTrue) != normalizeBuilderRuntimeRecordModelWhitespace(whenFalse) {
		return line
	}
	leading := line[:len(line)-len(strings.TrimLeft(line, " \t"))]
	return leading + fieldName + ": " + strings.TrimSpace(expr[:coalesceIndex+2]+" "+whenTrue) + ","
}

func builderRuntimeTopLevelTernaryIndexes(content string) (int, int) {
	depthParen := 0
	depthBracket := 0
	depthBrace := 0
	inSingleQuoted := false
	inDoubleQuoted := false
	escaped := false
	questionIndex := -1
	for index := 0; index < len(content); index++ {
		ch := content[index]
		if inSingleQuoted || inDoubleQuoted {
			if escaped {
				escaped = false
				continue
			}
			if ch == '\\' {
				escaped = true
				continue
			}
			if inSingleQuoted && ch == '\'' {
				inSingleQuoted = false
				continue
			}
			if inDoubleQuoted && ch == '"' {
				inDoubleQuoted = false
			}
			continue
		}
		switch ch {
		case '\'':
			inSingleQuoted = true
		case '"':
			inDoubleQuoted = true
		case '(':
			depthParen++
		case ')':
			if depthParen > 0 {
				depthParen--
			}
		case '[':
			depthBracket++
		case ']':
			if depthBracket > 0 {
				depthBracket--
			}
		case '{':
			depthBrace++
		case '}':
			if depthBrace > 0 {
				depthBrace--
			}
		case '?':
			if depthParen != 0 || depthBracket != 0 || depthBrace != 0 {
				continue
			}
			if index+1 < len(content) && content[index+1] == '?' {
				continue
			}
			if index > 0 && content[index-1] == '?' {
				continue
			}
			questionIndex = index
		case ':':
			if questionIndex >= 0 && depthParen == 0 && depthBracket == 0 && depthBrace == 0 {
				return questionIndex, index
			}
		}
	}
	return -1, -1
}

func normalizeBuilderRuntimeRecordModelWhitespace(content string) string {
	return strings.Join(strings.Fields(content), " ")
}

// fallback-only: LLM 模型路径兜底规范化，emit 路径不经过此函数
func normalizeBuilderRuntimeHomeControllerContent(workspacePath, content string) string {
	return normalizeBuilderRuntimeOpenLiteHomeControllerContent(workspacePath, content)
}

// fallback-only: LLM 模型路径兜底规范化，emit 路径不经过此函数
func normalizeBuilderRuntimeRecordFormControllerContent(workspacePath, content string) string {
	return normalizeBuilderRuntimeOpenLiteFormControllerContent(workspacePath, content)
}

// fallback-only: LLM 模型路径兜底规范化，emit 路径不经过此函数
func normalizeBuilderRuntimeRecordListControllerContent(workspacePath string, allowFilterFlow bool, needsCollectionCreateEntry bool, content string) string {
	return normalizeBuilderRuntimeOpenLiteListControllerContent(workspacePath, allowFilterFlow, needsCollectionCreateEntry, content)
}

// fallback-only: LLM 模型路径兜底规范化，emit 路径不经过此函数
func normalizeBuilderRuntimeWidgetTestContent(workspacePath string, needsCollectionCreateEntry bool, content string) string {
	return normalizeBuilderRuntimeOpenLiteWidgetTestContent(workspacePath, needsCollectionCreateEntry, content)
}

func normalizeBuilderRuntimeRecordRepositoryContent(workspacePath, content string, allowDeleteFlow bool) string {
	if strings.TrimSpace(content) == "" {
		return content
	}
	relationRichModelProfile := builderRuntimeRelationRichModelProfileForWorkspace(workspacePath)
	if builderRuntimeShouldCanonicalizeRelationRichRecordRepository(relationRichModelProfile, content) {
		if canonical := builderRuntimeCanonicalRelationRichRecordRepository(relationRichModelProfile); strings.TrimSpace(canonical) != "" {
			return canonical
		}
	}
	recordModelContent := builderRuntimeRepositoryPrimaryModelContent(workspacePath, content)
	if strings.TrimSpace(recordModelContent) == "" {
		return content
	}
	updated := content
	if builderRuntimeShouldRewriteRecordRepositoryToMapPersistence(recordModelContent, updated) {
		if canonical := builderRuntimeCanonicalMapBackedRecordRepository(recordModelContent, updated, allowDeleteFlow); strings.TrimSpace(canonical) != "" {
			updated = canonical
		}
	}
	if !allowDeleteFlow {
		updated = builderRuntimeStripDeleteFlowFromRecordRepository(updated)
	}
	if !strings.Contains(updated, ".updatedAt") {
		return updated
	}
	if strings.Contains(recordModelContent, "updatedAt") {
		return updated
	}
	if replacementField := builderRuntimePreferredRecordTimeField(recordModelContent); replacementField != "" {
		return strings.ReplaceAll(updated, ".updatedAt", "."+replacementField)
	}
	updated = builderRuntimeUpdatedAtCascadeSortPattern.ReplaceAllString(updated, ";")
	updated = builderRuntimeUpdatedAtStatementSortPattern.ReplaceAllString(updated, "")
	if !allowDeleteFlow {
		updated = builderRuntimeStripDeleteFlowFromRecordRepository(updated)
	}
	return updated
}

func builderRuntimePreferredRecordTimeField(recordModelContent string) string {
	fields := builderRuntimeDeclaredFieldNames(recordModelContent)
	if len(fields) == 0 {
		return ""
	}
	fieldTypes := builderRuntimeDeclaredFieldTypes(recordModelContent)
	shouldSkipField := func(field string) bool {
		fieldType := strings.TrimSpace(fieldTypes[field])
		return strings.Contains(fieldType, "?")
	}
	for _, candidate := range []string{"recordedAt", "checkedAt", "occurredOn", "createdAt", "date"} {
		if _, ok := fields[candidate]; ok && !shouldSkipField(candidate) {
			return candidate
		}
	}
	fallback := make([]string, 0)
	for field := range fields {
		if strings.HasSuffix(field, "At") || strings.HasSuffix(field, "On") || strings.HasSuffix(field, "Date") {
			if shouldSkipField(field) {
				continue
			}
			fallback = append(fallback, field)
		}
	}
	if len(fallback) == 0 {
		return ""
	}
	sort.Strings(fallback)
	return fallback[0]
}

func builderRuntimeRepositoryPrimaryModelContent(workspacePath, repositoryContent string) string {
	if strings.TrimSpace(workspacePath) == "" {
		return ""
	}
	primaryType := ""
	if match := builderRuntimeRepositoryPrimaryTypePattern.FindStringSubmatch(repositoryContent); len(match) >= 2 {
		primaryType = strings.TrimSpace(match[1])
	}
	imports := builderRuntimeRepositoryModelImportPattern.FindAllStringSubmatch(repositoryContent, -1)
	for _, match := range imports {
		if len(match) < 2 {
			continue
		}
		modelPath := filepath.ToSlash(filepath.Join("lib", "models", strings.TrimSpace(match[1])))
		modelContent, err := builderRuntimeSemanticFileContent(workspacePath, nil, modelPath)
		if err != nil || strings.TrimSpace(modelContent) == "" {
			continue
		}
		if primaryType == "" || builderRuntimePrimaryClassName(modelContent) == primaryType {
			return modelContent
		}
	}
	if primaryModelContent := builderRuntimeOpenLitePrimaryRecordModelContent(workspacePath); strings.TrimSpace(primaryModelContent) != "" {
		return primaryModelContent
	}
	if recordContent, err := builderRuntimeSemanticFileContent(workspacePath, nil, "lib/models/record.dart"); err == nil && strings.TrimSpace(recordContent) != "" {
		return recordContent
	}
	return ""
}

func builderRuntimeDeclaredFieldTypes(content string) map[string]string {
	matches := builderRuntimeDartTypedFieldPattern.FindAllStringSubmatch(content, -1)
	if len(matches) == 0 {
		return nil
	}
	fields := make(map[string]string, len(matches))
	for _, match := range matches {
		if len(match) < 3 {
			continue
		}
		fieldType := strings.TrimSpace(match[1])
		fieldName := strings.TrimSpace(match[2])
		if fieldName == "" || fieldType == "" {
			continue
		}
		fields[fieldName] = fieldType
	}
	if len(fields) == 0 {
		return nil
	}
	return fields
}

func builderRuntimePrimaryClassName(content string) string {
	match := builderRuntimeExportedDartClassPattern.FindStringSubmatch(content)
	if len(match) < 2 {
		return ""
	}
	return strings.TrimSpace(match[1])
}

func builderRuntimeRecordModelNeedsCollapse(content string) bool {
	if strings.TrimSpace(content) == "" {
		return false
	}
	primaryType := builderRuntimePrimaryClassName(content)
	if primaryType == "" {
		return false
	}
	if strings.Count(content, "class "+primaryType) > 1 {
		return true
	}
	if builderRuntimeRecordModelMalformedStatusSwitchCasePattern.MatchString(content) {
		return true
	}
	for _, marker := range []string{"Final clean version", "Correcting to", "Re-writing the class", "Wait, the above"} {
		if strings.Contains(content, marker) {
			return true
		}
	}
	return false
}

func builderRuntimeCanonicalLatestRecordModel(content string) string {
	primaryType := builderRuntimePrimaryClassName(content)
	if primaryType == "" {
		return ""
	}
	primaryBlock := builderRuntimeLastNamedDartTypeBlock(content, "class", primaryType)
	if strings.TrimSpace(primaryBlock) == "" {
		return ""
	}
	imports := builderRuntimeUniqueDartImports(content)
	identifierMatches := regexp.MustCompile(`\b([A-Z][A-Za-z0-9_]*)\b`).FindAllStringSubmatch(primaryBlock, -1)
	seenEnums := map[string]struct{}{}
	blocks := make([]string, 0, len(identifierMatches)+1)
	for _, match := range identifierMatches {
		if len(match) < 2 {
			continue
		}
		name := strings.TrimSpace(match[1])
		if name == "" || name == primaryType {
			continue
		}
		if _, ok := seenEnums[name]; ok {
			continue
		}
		enumBlock := builderRuntimeLastNamedDartTypeBlock(content, "enum", name)
		if strings.TrimSpace(enumBlock) == "" {
			continue
		}
		seenEnums[name] = struct{}{}
		blocks = append(blocks, enumBlock)
	}
	blocks = append(blocks, primaryBlock)
	sections := make([]string, 0, len(imports)+len(blocks))
	if len(imports) > 0 {
		sections = append(sections, strings.Join(imports, "\n"))
	}
	sections = append(sections, blocks...)
	return strings.Join(sections, "\n\n") + "\n"
}

func builderRuntimeLastNamedDartTypeBlock(content, kind, name string) string {
	trimmedKind := strings.TrimSpace(kind)
	trimmedName := strings.TrimSpace(name)
	if strings.TrimSpace(content) == "" || trimmedKind == "" || trimmedName == "" {
		return ""
	}
	matches := builderRuntimeNamedDartTypeDeclarationPattern.FindAllStringSubmatchIndex(content, -1)
	last := ""
	for _, match := range matches {
		if len(match) < 6 {
			continue
		}
		matchedKind := strings.TrimSpace(content[match[2]:match[3]])
		matchedName := strings.TrimSpace(content[match[4]:match[5]])
		if matchedKind != trimmedKind || matchedName != trimmedName {
			continue
		}
		openBrace := strings.Index(content[match[0]:match[1]], "{")
		if openBrace < 0 {
			continue
		}
		openBrace += match[0]
		closeBrace := builderRuntimeMatchingBraceIndex(content, openBrace)
		if closeBrace < 0 {
			continue
		}
		last = strings.TrimSpace(content[match[0] : closeBrace+1])
	}
	return last
}

func builderRuntimeUniqueDartImports(content string) []string {
	if strings.TrimSpace(content) == "" {
		return nil
	}
	lines := strings.Split(content, "\n")
	seen := map[string]struct{}{}
	imports := make([]string, 0)
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "import ") {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		imports = append(imports, trimmed)
	}
	return imports
}

func builderRuntimeHasDartImportOutsideHeader(content string) bool {
	if strings.TrimSpace(content) == "" {
		return false
	}
	sawNonImportHeaderContent := false
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "//") {
			continue
		}
		if strings.HasPrefix(trimmed, "import ") || strings.HasPrefix(trimmed, "library ") || strings.HasPrefix(trimmed, "export ") || strings.HasPrefix(trimmed, "part ") {
			if sawNonImportHeaderContent {
				return true
			}
			continue
		}
		sawNonImportHeaderContent = true
	}
	return false
}

func builderRuntimePreferredRecordIDField(recordModelContent string) string {
	fields := builderRuntimeDeclaredFieldNames(recordModelContent)
	if len(fields) == 0 {
		return ""
	}
	for _, candidate := range []string{"taskId", "recordId", "id", "entryId", "itemId"} {
		if _, ok := fields[candidate]; ok {
			return candidate
		}
	}
	fallback := make([]string, 0)
	for field := range fields {
		lower := strings.ToLower(field)
		if strings.HasSuffix(lower, "id") {
			fallback = append(fallback, field)
		}
	}
	if len(fallback) == 0 {
		return ""
	}
	sort.Strings(fallback)
	return fallback[0]
}

func builderRuntimeShouldCanonicalizeRelationRichRecordRepository(profile builderRuntimeRelationRichModelProfile, content string) bool {
	if profile == "" || strings.TrimSpace(content) == "" {
		return false
	}
	if builderRuntimeHasDartImportOutsideHeader(content) {
		return true
	}
	switch profile {
	case builderRuntimeRelationRichProjectTaskTagProfile:
		for _, marker := range []string{
			"@toOverride",
			"Dashboard<dynamic>",
			"loadDashboardSummary(",
			"loadTasksTagLinks(",
			"_safe<",
			"tagged_task_count",
		} {
			if strings.Contains(content, marker) {
				return true
			}
		}
		for _, requiredMarker := range []string{"Future<List<Project>> loadProjects();", "Future<List<TaskTagLink>> loadTaskTagLinks();", "Future<List<DashboardSummary>> loadSummaries();", "class HiveRecordRepository extends RecordRepository", "class InMemoryRecordRepository extends RecordRepository"} {
			if !strings.Contains(content, requiredMarker) {
				return true
			}
		}
	case builderRuntimeRelationRichInventorySheetLineProfile:
		for _, marker := range []string{
			"List<async>",
			"class DashboardSummary {",
			"omitted for brevity",
			"Simplified logic for demo",
			"_safe<",
			"low_stock_suku_count",
		} {
			if strings.Contains(content, marker) {
				return true
			}
		}
		for _, requiredMarker := range []string{"import '../models/dashboard_summary.dart';", "Future<List<InventorySheet>> loadInventorySheets();", "Future<List<LineItem>> loadLineItems();", "Future<List<Sku>> loadSkus();", "Future<List<Warehouse>> loadWarehouses();", "Future<List<DashboardSummary>> loadSummaries();", "Future<DashboardSummary?> loadDashboardSummary(String warehouseId) async {", "class HiveRecordRepository extends RecordRepository", "class InMemoryRecordRepository extends RecordRepository"} {
			if !strings.Contains(content, requiredMarker) {
				return true
			}
		}
	}
	return false
}

func builderRuntimeCanonicalRelationRichRecordRepository(profile builderRuntimeRelationRichModelProfile) string {
	switch profile {
	case builderRuntimeRelationRichProjectTaskTagProfile:
		return strings.Join([]string{
			"import 'package:hive_flutter/hive_flutter.dart';",
			"",
			"import '../models/dashboard_summary.dart';",
			"import '../models/project.dart';",
			"import '../models/tag.dart';",
			"import '../models/task.dart';",
			"import '../models/task_tag_link.dart';",
			"",
			"abstract class RecordRepository {",
			"  Future<void> init();",
			"",
			"  Future<List<Project>> loadProjects();",
			"  Future<void> saveProjects(List<Project> projects);",
			"  Future<void> addProject(Project project) async {",
			"    final nextProjects = await loadProjects();",
			"    nextProjects.add(project);",
			"    await saveProjects(nextProjects);",
			"  }",
			"",
			"  Future<List<Task>> loadTasks();",
			"  Future<void> saveTasks(List<Task> tasks);",
			"  Future<void> addTask(Task task) async {",
			"    final nextTasks = await loadTasks();",
			"    nextTasks.add(task);",
			"    await saveTasks(nextTasks);",
			"  }",
			"",
			"  Future<List<Tag>> loadTags();",
			"  Future<void> saveTags(List<Tag> tags);",
			"  Future<void> addTag(Tag tag) async {",
			"    final nextTags = await loadTags();",
			"    nextTags.add(tag);",
			"    await saveTags(nextTags);",
			"  }",
			"",
			"  Future<List<TaskTagLink>> loadTaskTagLinks();",
			"  Future<void> saveTaskTagLinks(List<TaskTagLink> links);",
			"  Future<void> addTaskTagLink(TaskTagLink link) async {",
			"    final nextLinks = await loadTaskTagLinks();",
			"    nextLinks.add(link);",
			"    await saveTaskTagLinks(nextLinks);",
			"  }",
			"",
			"  Future<List<DashboardSummary>> loadSummaries();",
			"  Future<void> saveSummaries(List<DashboardSummary> summaries);",
			"}",
			"",
			"class HiveRecordRepository extends RecordRepository {",
			"  static const String _boxName = 'jobs_records_box';",
			"  static const String _projectsKey = 'projects';",
			"  static const String _tasksKey = 'tasks';",
			"  static const String _tagsKey = 'tags';",
			"  static const String _linksKey = 'task_tag_links';",
			"  static const String _summariesKey = 'summaries';",
			"",
			"  Box<dynamic>? _box;",
			"",
			"  @override",
			"  Future<void> init() async {",
			"    await Hive.initFlutter();",
			"    _box = await Hive.openBox<dynamic>(_boxName);",
			"  }",
			"",
			"  Box<dynamic> get _safeBox {",
			"    final box = _box;",
			"    if (box == null) {",
			"      throw StateError('repository not initialized');",
			"    }",
			"    return box;",
			"  }",
			"",
			"  @override",
			"  Future<List<Project>> loadProjects() async {",
			"    final raw = _safeBox.get(_projectsKey, defaultValue: <dynamic>[]) as List<dynamic>;",
			"    return raw",
			"        .map((item) => Project.fromJson(Map<String, dynamic>.from(item as Map)))",
			"        .toList(growable: false);",
			"  }",
			"",
			"  @override",
			"  Future<void> saveProjects(List<Project> projects) async {",
			"    await _safeBox.put(_projectsKey, projects.map((project) => project.toJson()).toList());",
			"  }",
			"",
			"  @override",
			"  Future<List<Task>> loadTasks() async {",
			"    final raw = _safeBox.get(_tasksKey, defaultValue: <dynamic>[]) as List<dynamic>;",
			"    return raw",
			"        .map((item) => Task.fromJson(Map<String, dynamic>.from(item as Map)))",
			"        .toList(growable: false);",
			"  }",
			"",
			"  @override",
			"  Future<void> saveTasks(List<Task> tasks) async {",
			"    await _safeBox.put(_tasksKey, tasks.map((task) => task.toJson()).toList());",
			"  }",
			"",
			"  @override",
			"  Future<List<Tag>> loadTags() async {",
			"    final raw = _safeBox.get(_tagsKey, defaultValue: <dynamic>[]) as List<dynamic>;",
			"    return raw",
			"        .map((item) => Tag.fromJson(Map<String, dynamic>.from(item as Map)))",
			"        .toList(growable: false);",
			"  }",
			"",
			"  @override",
			"  Future<void> saveTags(List<Tag> tags) async {",
			"    await _safeBox.put(_tagsKey, tags.map((tag) => tag.toJson()).toList());",
			"  }",
			"",
			"  @override",
			"  Future<List<TaskTagLink>> loadTaskTagLinks() async {",
			"    final raw = _safeBox.get(_linksKey, defaultValue: <dynamic>[]) as List<dynamic>;",
			"    return raw",
			"        .map((item) => TaskTagLink.fromJson(Map<String, dynamic>.from(item as Map)))",
			"        .toList(growable: false);",
			"  }",
			"",
			"  @override",
			"  Future<void> saveTaskTagLinks(List<TaskTagLink> links) async {",
			"    await _safeBox.put(_linksKey, links.map((link) => link.toJson()).toList());",
			"  }",
			"",
			"  @override",
			"  Future<List<DashboardSummary>> loadSummaries() async {",
			"    final raw = _safeBox.get(_summariesKey, defaultValue: <dynamic>[]) as List<dynamic>;",
			"    return raw",
			"        .map((item) => DashboardSummary.fromJson(Map<String, dynamic>.from(item as Map)))",
			"        .toList(growable: false);",
			"  }",
			"",
			"  @override",
			"  Future<void> saveSummaries(List<DashboardSummary> summaries) async {",
			"    await _safeBox.put(_summariesKey, summaries.map((summary) => summary.toJson()).toList());",
			"  }",
			"}",
			"",
			"class InMemoryRecordRepository extends RecordRepository {",
			"  InMemoryRecordRepository({",
			"    List<Project>? projects,",
			"    List<Task>? tasks,",
			"    List<Tag>? tags,",
			"    List<TaskTagLink>? links,",
			"    List<DashboardSummary>? summaries,",
			"  })  : _projects = projects ?? <Project>[],",
			"        _tasks = tasks ?? <Task>[],",
			"        _tags = tags ?? <Tag>[],",
			"        _links = links ?? <TaskTagLink>[],",
			"        _summaries = summaries ?? <DashboardSummary>[];",
			"",
			"  final List<Project> _projects;",
			"  final List<Task> _tasks;",
			"  final List<Tag> _tags;",
			"  final List<TaskTagLink> _links;",
			"  final List<DashboardSummary> _summaries;",
			"",
			"  @override",
			"  Future<void> init() async {}",
			"",
			"  @override",
			"  Future<List<Project>> loadProjects() async => List.unmodifiable(_projects);",
			"",
			"  @override",
			"  Future<void> saveProjects(List<Project> projects) async {",
			"    _projects",
			"      ..clear()",
			"      ..addAll(projects);",
			"  }",
			"",
			"  @override",
			"  Future<List<Task>> loadTasks() async => List.unmodifiable(_tasks);",
			"",
			"  @override",
			"  Future<void> saveTasks(List<Task> tasks) async {",
			"    _tasks",
			"      ..clear()",
			"      ..addAll(tasks);",
			"  }",
			"",
			"  @override",
			"  Future<List<Tag>> loadTags() async => List.unmodifiable(_tags);",
			"",
			"  @override",
			"  Future<void> saveTags(List<Tag> tags) async {",
			"    _tags",
			"      ..clear()",
			"      ..addAll(tags);",
			"  }",
			"",
			"  @override",
			"  Future<List<TaskTagLink>> loadTaskTagLinks() async => List.unmodifiable(_links);",
			"",
			"  @override",
			"  Future<void> saveTaskTagLinks(List<TaskTagLink> links) async {",
			"    _links",
			"      ..clear()",
			"      ..addAll(links);",
			"  }",
			"",
			"  @override",
			"  Future<List<DashboardSummary>> loadSummaries() async => List.unmodifiable(_summaries);",
			"",
			"  @override",
			"  Future<void> saveSummaries(List<DashboardSummary> summaries) async {",
			"    _summaries",
			"      ..clear()",
			"      ..addAll(summaries);",
			"  }",
			"}",
		}, "\n") + "\n"
	case builderRuntimeRelationRichInventorySheetLineProfile:
		return strings.Join([]string{
			"import 'package:hive_flutter/hive_flutter.dart';",
			"",
			"import '../models/dashboard_summary.dart';",
			"import '../models/inventory_sheet.dart';",
			"import '../models/line_item.dart';",
			"import '../models/sku.dart';",
			"import '../models/warehouse.dart';",
			"",
			"abstract class RecordRepository {",
			"  Future<void> init();",
			"",
			"  Future<List<InventorySheet>> loadInventorySheets();",
			"  Future<void> saveInventorySheets(List<InventorySheet> sheets);",
			"  Future<void> addInventorySheet(InventorySheet sheet) async {",
			"    final nextSheets = List<InventorySheet>.of(await loadInventorySheets());",
			"    nextSheets.add(sheet);",
			"    await saveInventorySheets(nextSheets);",
			"  }",
			"  Future<void> updateInventorySheet(InventorySheet sheet) async {",
			"    final nextSheets = List<InventorySheet>.of(await loadInventorySheets());",
			"    final index = nextSheets.indexWhere((candidate) => candidate.sheetId == sheet.sheetId);",
			"    if (index >= 0) {",
			"      nextSheets[index] = sheet;",
			"    } else {",
			"      nextSheets.add(sheet);",
			"    }",
			"    await saveInventorySheets(nextSheets);",
			"  }",
			"",
			"  Future<List<LineItem>> loadLineItems();",
			"  Future<void> saveLineItems(List<LineItem> items);",
			"  Future<void> addLineItem(LineItem item) async {",
			"    final nextItems = List<LineItem>.of(await loadLineItems());",
			"    nextItems.add(item);",
			"    await saveLineItems(nextItems);",
			"  }",
			"  Future<void> updateLineItem(LineItem item) async {",
			"    final nextItems = List<LineItem>.of(await loadLineItems());",
			"    final index = nextItems.indexWhere((candidate) => candidate.lineItemId == item.lineItemId);",
			"    if (index >= 0) {",
			"      nextItems[index] = item;",
			"    } else {",
			"      nextItems.add(item);",
			"    }",
			"    await saveLineItems(nextItems);",
			"  }",
			"",
			"  Future<List<Sku>> loadSkus();",
			"  Future<void> saveSkus(List<Sku> skus);",
			"  Future<void> addSku(Sku sku) async {",
			"    final nextSkus = List<Sku>.of(await loadSkus());",
			"    nextSkus.add(sku);",
			"    await saveSkus(nextSkus);",
			"  }",
			"",
			"  Future<List<Warehouse>> loadWarehouses();",
			"  Future<void> saveWarehouses(List<Warehouse> warehouses);",
			"  Future<void> addWarehouse(Warehouse warehouse) async {",
			"    final nextWarehouses = List<Warehouse>.of(await loadWarehouses());",
			"    nextWarehouses.add(warehouse);",
			"    await saveWarehouses(nextWarehouses);",
			"  }",
			"",
			"  Future<List<DashboardSummary>> loadSummaries();",
			"  Future<DashboardSummary?> loadDashboardSummary(String warehouseId) async {",
			"    final summaries = await loadSummaries();",
			"    for (final summary in summaries) {",
			"      if (summary.warehouseId == warehouseId) {",
			"        return summary;",
			"      }",
			"    }",
			"    return null;",
			"  }",
			"}",
			"",
			"DashboardSummary _buildDashboardSummary(",
			"  Warehouse warehouse,",
			"  List<InventorySheet> sheets,",
			"  List<LineItem> items,",
			"  List<Sku> skus,",
			") {",
			"  final warehouseSheets = sheets",
			"      .where((sheet) => sheet.warehouseId == warehouse.warehouseId)",
			"      .toList(growable: false);",
			"  final sheetIds = warehouseSheets.map((sheet) => sheet.sheetId).toSet();",
			"  final warehouseItems = items",
			"      .where((item) => sheetIds.contains(item.sheetId))",
			"      .toList(growable: false);",
			"  final skuById = <String, Sku>{for (final sku in skus) sku.skuId: sku};",
			"  final lowStockSkuIds = <String>{};",
			"  for (final item in warehouseItems) {",
			"    final sku = skuById[item.skuId];",
			"    if (sku != null && item.countedQty <= sku.reorderThreshold) {",
			"      lowStockSkuIds.add(item.skuId);",
			"    }",
			"  }",
			"  return DashboardSummary(",
			"    warehouseId: warehouse.warehouseId,",
			"    openSheetCount: warehouseSheets",
			"        .where((sheet) => sheet.status != InventorySheetStatus.closed)",
			"        .length,",
			"    lowStockSkuCount: lowStockSkuIds.length,",
			"    varianceLineItemCount:",
			"        warehouseItems.where((item) => item.varianceQty != 0).length,",
			"  );",
			"}",
			"",
			"class HiveRecordRepository extends RecordRepository {",
			"  static const String _boxName = 'inventory_box';",
			"  static const String _sheetsKey = 'inventory_sheets';",
			"  static const String _lineItemsKey = 'line_items';",
			"  static const String _skusKey = 'skus';",
			"  static const String _warehousesKey = 'warehouses';",
			"",
			"  Box<dynamic>? _box;",
			"",
			"  @override",
			"  Future<void> init() async {",
			"    await Hive.initFlutter();",
			"    _box = await Hive.openBox<dynamic>(_boxName);",
			"  }",
			"",
			"  Box<dynamic> get _safeBox {",
			"    final box = _box;",
			"    if (box == null) {",
			"      throw StateError('repository not initialized');",
			"    }",
			"    return box;",
			"  }",
			"",
			"  @override",
			"  Future<List<InventorySheet>> loadInventorySheets() async {",
			"    final raw = _safeBox.get(_sheetsKey, defaultValue: <dynamic>[]) as List<dynamic>;",
			"    return raw",
			"        .map((item) => InventorySheet.fromJson(Map<String, dynamic>.from(item as Map)))",
			"        .toList(growable: false);",
			"  }",
			"",
			"  @override",
			"  Future<void> saveInventorySheets(List<InventorySheet> sheets) async {",
			"    await _safeBox.put(_sheetsKey, sheets.map((sheet) => sheet.toJson()).toList());",
			"  }",
			"",
			"  @override",
			"  Future<List<LineItem>> loadLineItems() async {",
			"    final raw = _safeBox.get(_lineItemsKey, defaultValue: <dynamic>[]) as List<dynamic>;",
			"    return raw",
			"        .map((item) => LineItem.fromJson(Map<String, dynamic>.from(item as Map)))",
			"        .toList(growable: false);",
			"  }",
			"",
			"  @override",
			"  Future<void> saveLineItems(List<LineItem> items) async {",
			"    await _safeBox.put(_lineItemsKey, items.map((item) => item.toJson()).toList());",
			"  }",
			"",
			"  @override",
			"  Future<List<Sku>> loadSkus() async {",
			"    final raw = _safeBox.get(_skusKey, defaultValue: <dynamic>[]) as List<dynamic>;",
			"    return raw",
			"        .map((item) => Sku.fromJson(Map<String, dynamic>.from(item as Map)))",
			"        .toList(growable: false);",
			"  }",
			"",
			"  @override",
			"  Future<void> saveSkus(List<Sku> skus) async {",
			"    await _safeBox.put(_skusKey, skus.map((sku) => sku.toJson()).toList());",
			"  }",
			"",
			"  @override",
			"  Future<List<Warehouse>> loadWarehouses() async {",
			"    final raw = _safeBox.get(_warehousesKey, defaultValue: <dynamic>[]) as List<dynamic>;",
			"    return raw",
			"        .map((item) => Warehouse.fromJson(Map<String, dynamic>.from(item as Map)))",
			"        .toList(growable: false);",
			"  }",
			"",
			"  @override",
			"  Future<void> saveWarehouses(List<Warehouse> warehouses) async {",
			"    await _safeBox.put(_warehousesKey, warehouses.map((warehouse) => warehouse.toJson()).toList());",
			"  }",
			"",
			"  @override",
			"  Future<List<DashboardSummary>> loadSummaries() async {",
			"    final warehouses = await loadWarehouses();",
			"    final sheets = await loadInventorySheets();",
			"    final items = await loadLineItems();",
			"    final skus = await loadSkus();",
			"    return warehouses",
			"        .map((warehouse) => _buildDashboardSummary(warehouse, sheets, items, skus))",
			"        .toList(growable: false);",
			"  }",
			"}",
			"",
			"class InMemoryRecordRepository extends RecordRepository {",
			"  InMemoryRecordRepository({",
			"    List<InventorySheet>? inventorySheets,",
			"    List<LineItem>? lineItems,",
			"    List<Sku>? skus,",
			"    List<Warehouse>? warehouses,",
			"  })  : _inventorySheets = inventorySheets ?? <InventorySheet>[],",
			"        _lineItems = lineItems ?? <LineItem>[],",
			"        _skus = skus ?? <Sku>[],",
			"        _warehouses = warehouses ?? <Warehouse>[];",
			"",
			"  final List<InventorySheet> _inventorySheets;",
			"  final List<LineItem> _lineItems;",
			"  final List<Sku> _skus;",
			"  final List<Warehouse> _warehouses;",
			"",
			"  @override",
			"  Future<void> init() async {}",
			"",
			"  @override",
			"  Future<List<InventorySheet>> loadInventorySheets() async => List.unmodifiable(_inventorySheets);",
			"",
			"  @override",
			"  Future<void> saveInventorySheets(List<InventorySheet> sheets) async {",
			"    _inventorySheets",
			"      ..clear()",
			"      ..addAll(sheets);",
			"  }",
			"",
			"  @override",
			"  Future<List<LineItem>> loadLineItems() async => List.unmodifiable(_lineItems);",
			"",
			"  @override",
			"  Future<void> saveLineItems(List<LineItem> items) async {",
			"    _lineItems",
			"      ..clear()",
			"      ..addAll(items);",
			"  }",
			"",
			"  @override",
			"  Future<List<Sku>> loadSkus() async => List.unmodifiable(_skus);",
			"",
			"  @override",
			"  Future<void> saveSkus(List<Sku> skus) async {",
			"    _skus",
			"      ..clear()",
			"      ..addAll(skus);",
			"  }",
			"",
			"  @override",
			"  Future<List<Warehouse>> loadWarehouses() async => List.unmodifiable(_warehouses);",
			"",
			"  @override",
			"  Future<void> saveWarehouses(List<Warehouse> warehouses) async {",
			"    _warehouses",
			"      ..clear()",
			"      ..addAll(warehouses);",
			"  }",
			"",
			"  @override",
			"  Future<List<DashboardSummary>> loadSummaries() async {",
			"    return _warehouses",
			"        .map((warehouse) => _buildDashboardSummary(warehouse, _inventorySheets, _lineItems, _skus))",
			"        .toList(growable: false);",
			"  }",
			"}",
		}, "\n") + "\n"
	default:
		return ""
	}
}

func builderRuntimeRelationRichRecordRepositoryPromptGuidance(profile builderRuntimeRelationRichModelProfile) string {
	switch profile {
	case builderRuntimeRelationRichProjectTaskTagProfile:
		return strings.Join([]string{
			"For current repository-scope target files under lib/repositories/*.dart in the current relation-rich task graph, preserve the existing abstract repository contract unless the same patch also updates every dependent controller, view, and lib/main.dart callsite. Do not rename or replace existing methods such as loadProjects/saveProjects/addProject, loadTasks/saveTasks/addTask, loadTags/saveTags/addTag, loadTaskTagLinks/saveTaskTagLinks/addTaskTagLink, loadSummaries, or saveSummaries with ad hoc helpers.",
			"For current repository-scope syntax or semantic repairs under lib/repositories/*.dart, do not introduce convenience APIs such as clearAll, loadDashboardSummary, or alternate summary-loading contracts unless those methods already exist in the current workspace file context. Repair the existing file shape instead of redesigning the repository surface.",
			"When computing dashboard summary values inside current repository-scope target files, keep names aligned to the current DashboardSummary fields: openTaskCount, doneTaskCount, taggedTaskCount. Do not introduce legacy helper tokens such as openCount, doneCount, or taggedCount in the repaired repository file.",
			"Keep every import for current repository-scope target files in the file header only. Do not paste a second import block below an existing class or method body during repair.",
			"When repairing current repository-scope target files, reuse the current _safeBox.get(...)/_safeBox.put(...) storage path. Do not invent helper calls such as _safe<T>(...) unless that helper already exists in the current file context.",
		}, "\n") + "\n"
	case builderRuntimeRelationRichInventorySheetLineProfile:
		return strings.Join([]string{
			"For current repository-scope target files under lib/repositories/*.dart in the current inventory relation-rich task graph, preserve the existing abstract repository contract unless the same patch also updates every dependent controller, view, and lib/main.dart callsite. Do not rename or replace existing methods such as loadInventorySheets/saveInventorySheets/addInventorySheet/updateInventorySheet, loadLineItems/saveLineItems/addLineItem/updateLineItem, loadSkus/saveSkus/addSku, loadWarehouses/saveWarehouses/addWarehouse, loadSummaries, or loadDashboardSummary with ad hoc helpers.",
			"Do not redefine DashboardSummary, InventorySheet, LineItem, Sku, or Warehouse inside current repository-scope target files. Import the current model files from lib/models instead of pasting placeholder duplicates into the repository file.",
			"When computing low-stock summaries inside current repository-scope target files, keep names aligned to the current DashboardSummary fields: openSheetCount, lowStockSkuCount, varianceLineItemCount. Do not introduce misspelled keys such as lowStockSukuCount or placeholder collection types such as List<async>.",
			"Keep every import for current repository-scope target files in the file header only. Do not paste a second import block below an existing class or method body during repair.",
			"When repairing current repository-scope target files, reuse the current _safeBox.get(...)/_safeBox.put(...) storage path. Do not invent helper calls such as _safe<T>(...) or leave placeholder comments such as 'omitted for brevity' in the repository file.",
		}, "\n") + "\n"
	default:
		return ""
	}
}

func builderRuntimeShouldRewriteRecordRepositoryToMapPersistence(recordModelContent, content string) bool {
	if strings.TrimSpace(recordModelContent) == "" || strings.TrimSpace(content) == "" {
		return false
	}
	if !strings.Contains(recordModelContent, "toMap(") || !strings.Contains(recordModelContent, "fromMap(") {
		return false
	}
	if strings.Contains(content, "registerAdapter(") {
		return true
	}
	recordType := builderRuntimePrimaryClassName(recordModelContent)
	if recordType == "" {
		return false
	}
	if strings.Contains(content, "class RecordRepository {") && !strings.Contains(content, "abstract class RecordRepository") {
		if strings.Contains(content, "Future<List<"+recordType+">> loadRecords(") && strings.Contains(content, "Future<void> saveRecords(") {
			return true
		}
	}
	if strings.Contains(content, "class HiveRecordRepository") || strings.Contains(content, "class InMemoryRecordRepository") {
		return true
	}
	if strings.Contains(content, "Box<"+recordType+")") {
		return true
	}
	if strings.Contains(content, "Box<"+recordType+">") || strings.Contains(content, "openBox<"+recordType+">") {
		return true
	}
	return strings.Contains(content, "_safeBox.values.toList()")
}

func builderRuntimeCanonicalMapBackedRecordRepository(recordModelContent, currentContent string, allowDeleteFlow bool) string {
	recordType := builderRuntimePrimaryClassName(recordModelContent)
	if recordType == "" {
		return ""
	}
	idField := builderRuntimePreferredRecordIDField(recordModelContent)
	if idField == "" {
		return ""
	}
	boxName := builderRuntimeRecordRepositoryStorageConstant(currentContent, builderRuntimeRecordRepositoryBoxNamePattern, "flutter_open_lite")
	recordsKey := builderRuntimeRecordRepositoryStorageConstant(currentContent, builderRuntimeRecordRepositoryRecordsKeyPattern, "records")
	sortStatement := builderRuntimeRecordRepositorySortStatement(recordModelContent, "records")
	inMemorySortStatement := builderRuntimeRecordRepositorySortStatement(recordModelContent, "records")
	var builder strings.Builder
	builder.WriteString("import 'package:hive_flutter/hive_flutter.dart';\n\n")
	builder.WriteString("import '../models/record.dart';\n\n")
	builder.WriteString("abstract class RecordRepository {\n")
	builder.WriteString("  Future<void> init();\n")
	builder.WriteString("  Future<List<")
	builder.WriteString(recordType)
	builder.WriteString(">> loadRecords();\n")
	builder.WriteString("  Future<void> saveRecords(List<")
	builder.WriteString(recordType)
	builder.WriteString("> records);\n\n")
	builder.WriteString("  Future<void> addRecord(")
	builder.WriteString(recordType)
	builder.WriteString(" record) async {\n")
	builder.WriteString("    final records = await loadRecords();\n")
	builder.WriteString("    records.add(record);\n")
	builder.WriteString("    await saveRecords(records);\n")
	builder.WriteString("  }\n\n")
	builder.WriteString("  Future<void> updateRecord(")
	builder.WriteString(recordType)
	builder.WriteString(" record) async {\n")
	builder.WriteString("    final records = await loadRecords();\n")
	builder.WriteString("    final index = records.indexWhere((item) => item.")
	builder.WriteString(idField)
	builder.WriteString(" == record.")
	builder.WriteString(idField)
	builder.WriteString(");\n")
	builder.WriteString("    if (index < 0) {\n")
	builder.WriteString("      throw StateError('record not found');\n")
	builder.WriteString("    }\n")
	builder.WriteString("    records[index] = record;\n")
	builder.WriteString("    await saveRecords(records);\n")
	builder.WriteString("  }\n\n")
	if allowDeleteFlow {
		builder.WriteString("  Future<void> deleteRecord(String ")
		builder.WriteString(idField)
		builder.WriteString(") async {\n")
		builder.WriteString("    final records = await loadRecords();\n")
		builder.WriteString("    records.removeWhere((item) => item.")
		builder.WriteString(idField)
		builder.WriteString(" == ")
		builder.WriteString(idField)
		builder.WriteString(");\n")
		builder.WriteString("    await saveRecords(records);\n")
		builder.WriteString("  }\n")
	}
	builder.WriteString("}\n\n")
	builder.WriteString("class HiveRecordRepository extends RecordRepository {\n")
	builder.WriteString("  static const String _boxName = '")
	builder.WriteString(boxName)
	builder.WriteString("';\n")
	builder.WriteString("  static const String _recordsKey = '")
	builder.WriteString(recordsKey)
	builder.WriteString("';\n\n")
	builder.WriteString("  Box<dynamic>? _box;\n\n")
	builder.WriteString("  @override\n")
	builder.WriteString("  Future<void> init() async {\n")
	builder.WriteString("    await Hive.initFlutter();\n")
	builder.WriteString("    _box = await Hive.openBox<dynamic>(_boxName);\n")
	builder.WriteString("  }\n\n")
	builder.WriteString("  Box<dynamic> get _safeBox {\n")
	builder.WriteString("    final box = _box;\n")
	builder.WriteString("    if (box == null) {\n")
	builder.WriteString("      throw StateError('repository not initialized');\n")
	builder.WriteString("    }\n")
	builder.WriteString("    return box;\n")
	builder.WriteString("  }\n\n")
	builder.WriteString("  @override\n")
	builder.WriteString("  Future<List<")
	builder.WriteString(recordType)
	builder.WriteString(">> loadRecords() async {\n")
	builder.WriteString("    final rawRecords = _safeBox.get(_recordsKey, defaultValue: <dynamic>[]) as List<dynamic>;\n")
	builder.WriteString("    final records = rawRecords\n")
	builder.WriteString("        .map((item) => ")
	builder.WriteString(recordType)
	builder.WriteString(".fromMap(Map<String, dynamic>.from(item as Map)))\n")
	builder.WriteString("        .toList();\n")
	if sortStatement != "" {
		builder.WriteString("    ")
		builder.WriteString(sortStatement)
		builder.WriteString("\n")
	}
	builder.WriteString("    return records;\n")
	builder.WriteString("  }\n\n")
	builder.WriteString("  @override\n")
	builder.WriteString("  Future<void> saveRecords(List<")
	builder.WriteString(recordType)
	builder.WriteString("> records) async {\n")
	builder.WriteString("    await _safeBox.put(\n")
	builder.WriteString("      _recordsKey,\n")
	builder.WriteString("      records.map((record) => record.toMap()).toList(),\n")
	builder.WriteString("    );\n")
	builder.WriteString("  }\n")
	builder.WriteString("}\n\n")
	builder.WriteString("class InMemoryRecordRepository extends RecordRepository {\n")
	builder.WriteString("  InMemoryRecordRepository({List<")
	builder.WriteString(recordType)
	builder.WriteString(">? seedRecords})\n")
	builder.WriteString("      : _records = List<")
	builder.WriteString(recordType)
	builder.WriteString(">.from(seedRecords ?? const []);\n\n")
	builder.WriteString("  final List<")
	builder.WriteString(recordType)
	builder.WriteString("> _records;\n\n")
	builder.WriteString("  @override\n")
	builder.WriteString("  Future<void> init() async {}\n\n")
	builder.WriteString("  @override\n")
	builder.WriteString("  Future<List<")
	builder.WriteString(recordType)
	builder.WriteString(">> loadRecords() async {\n")
	builder.WriteString("    final records = List<")
	builder.WriteString(recordType)
	builder.WriteString(">.from(_records);\n")
	if inMemorySortStatement != "" {
		builder.WriteString("    ")
		builder.WriteString(inMemorySortStatement)
		builder.WriteString("\n")
	}
	builder.WriteString("    return records;\n")
	builder.WriteString("  }\n\n")
	builder.WriteString("  @override\n")
	builder.WriteString("  Future<void> saveRecords(List<")
	builder.WriteString(recordType)
	builder.WriteString("> records) async {\n")
	builder.WriteString("    _records\n")
	builder.WriteString("      ..clear()\n")
	builder.WriteString("      ..addAll(records);\n")
	builder.WriteString("  }\n")
	builder.WriteString("}\n")
	return builder.String()
}

func builderRuntimeStripDeleteFlowFromRecordRepository(content string) string {
	if strings.TrimSpace(content) == "" || !strings.Contains(content, "deleteRecord") {
		return content
	}
	updated := builderRuntimeRepositoryDeleteImplementationPattern.ReplaceAllString(content, "\n")
	updated = builderRuntimeRepositoryDeleteAsyncPattern.ReplaceAllString(updated, "\n")
	updated = builderRuntimeRepositoryDeleteDeclarationPattern.ReplaceAllString(updated, "")
	for strings.Contains(updated, "\n\n\n") {
		updated = strings.ReplaceAll(updated, "\n\n\n", "\n\n")
	}
	return updated
}

func builderRuntimeRecordRepositoryStorageConstant(content string, pattern *regexp.Regexp, fallback string) string {
	if pattern == nil {
		return fallback
	}
	match := pattern.FindStringSubmatch(content)
	if len(match) < 2 || strings.TrimSpace(match[1]) == "" {
		return fallback
	}
	return strings.TrimSpace(match[1])
}

func builderRuntimeRecordRepositorySortStatement(recordModelContent, collectionName string) string {
	trimmedCollection := strings.TrimSpace(collectionName)
	if trimmedCollection == "" {
		trimmedCollection = "records"
	}
	if timeField := builderRuntimePreferredRecordTimeField(recordModelContent); timeField != "" {
		return fmt.Sprintf("%s.sort((left, right) => right.%s.compareTo(left.%s));", trimmedCollection, timeField, timeField)
	}
	fields := builderRuntimeDeclaredFieldNames(recordModelContent)
	for _, candidate := range []string{"title", "name"} {
		if _, ok := fields[candidate]; ok {
			return fmt.Sprintf("%s.sort((left, right) => left.%s.compareTo(right.%s));", trimmedCollection, candidate, candidate)
		}
	}
	return ""
}

// fallback-only: LLM 模型路径兜底规范化，emit 路径不经过此函数
func normalizeBuilderRuntimeViewContent(workspacePath string, allowFilterFlow bool, needsCollectionCreateEntry bool, content string) string {
	updated := normalizeBuilderRuntimeOpenLiteCopyHelperReferences(content)
	updated = normalizeBuilderRuntimeOpenLiteDropdownButtonInitialValue(updated)
	updated = normalizeBuilderRuntimeRecordFormPageContent(workspacePath, updated)
	updated = normalizeBuilderRuntimeHomePageContent(workspacePath, "", updated)
	updated = normalizeBuilderRuntimeRecordListPageContent(workspacePath, allowFilterFlow, needsCollectionCreateEntry, updated)
	return normalizeBuilderRuntimeRecordDetailPageContent(workspacePath, updated)
}

// fallback-only: LLM 模型路径兜底规范化，emit 路径不经过此函数
func normalizeBuilderRuntimeRecordFormPageContent(workspacePath, content string) string {
	return normalizeBuilderRuntimeOpenLiteFormPageContent(workspacePath, content)
}

// fallback-only: LLM 模型路径兜底规范化，emit 路径不经过此函数
func normalizeBuilderRuntimeHomePageContent(workspacePath, path, content string) string {
	if strings.TrimSpace(content) == "" {
		return content
	}
	updated := normalizeBuilderRuntimeOpenLiteHomePageContent(workspacePath, path, content)
	if builderRuntimeOpenLiteShouldDropTotalCount(workspacePath) {
		updated = strings.ReplaceAll(updated, "summary.totalCount", builderRuntimePreferredHomePageCountExpression())
	}
	if !strings.Contains(updated, ".updatedAt") {
		return updated
	}
	return normalizeBuilderRuntimeRecordTimeContent(workspacePath, updated, func(content string) string {
		updated := builderRuntimeHomePageUpdatedAtTextPattern.ReplaceAllString(content, "const SizedBox.shrink(),")
		updated = builderRuntimeHomePageUpdatedAtToLocalTextPattern.ReplaceAllString(updated, "const SizedBox.shrink(),")
		return updated
	})
}

// fallback-only: LLM 模型路径兜底规范化，emit 路径不经过此函数
func normalizeBuilderRuntimeRecordListPageContent(workspacePath string, allowFilterFlow bool, needsCollectionCreateEntry bool, content string) string {
	if !builderRuntimeOpenLiteLooksLikeCollectionPageContent(workspacePath, content) {
		return content
	}
	updated := normalizeBuilderRuntimeOpenLiteListPageContent(workspacePath, allowFilterFlow, needsCollectionCreateEntry, content)
	if strings.TrimSpace(updated) == "" || !strings.Contains(updated, ".updatedAt") {
		return updated
	}
	return normalizeBuilderRuntimeRecordTimeContent(workspacePath, updated, func(content string) string {
		return builderRuntimeRecordListPageUpdatedAtTextPattern.ReplaceAllString(content, "const SizedBox.shrink(),")
	})
}

// fallback-only: LLM 模型路径兜底规范化，emit 路径不经过此函数
func normalizeBuilderRuntimeRecordDetailPageContent(workspacePath, content string) string {
	if strings.TrimSpace(content) == "" {
		return content
	}
	updated := normalizeBuilderRuntimeOpenLiteRecordDetailPageContent(workspacePath, content)
	if !strings.Contains(updated, ".updatedAt") {
		return updated
	}
	return normalizeBuilderRuntimeRecordTimeContent(workspacePath, updated, func(content string) string {
		return builderRuntimeRecordDetailPageUpdatedAtTilePattern.ReplaceAllString(content, "const SizedBox.shrink(),")
	})
}

func normalizeBuilderRuntimeAndroidBuildGradleContent(content string) string {
	if strings.TrimSpace(content) == "" {
		return content
	}
	updated := builderRuntimeAndroidBuildGradleFlutterSourcePattern.ReplaceAllString(content, `${1}"../.."$2`)
	updated = builderRuntimeAndroidBuildGradleNamespaceIdentifierPattern.ReplaceAllString(updated, `${1}defaultOpenLiteApplicationId$2`)
	return updated
}

func normalizeBuilderRuntimeRecordTimeContent(workspacePath, content string, fallback func(string) string) string {
	if strings.TrimSpace(content) == "" || !strings.Contains(content, ".updatedAt") {
		return content
	}
	recordModelContent := builderRuntimeOpenLitePrimaryRecordModelContent(workspacePath)
	if strings.TrimSpace(recordModelContent) == "" {
		return content
	}
	if strings.Contains(recordModelContent, "updatedAt") {
		return content
	}
	if replacementField := builderRuntimePreferredRecordTimeField(recordModelContent); replacementField != "" {
		return strings.ReplaceAll(content, ".updatedAt", "."+replacementField)
	}
	if fallback == nil {
		return content
	}
	return fallback(content)
}

func builderRuntimePreferredHomePageCountExpression() string {
	return "controller.records.length"
}

func normalizeBuilderRuntimePatch(rawContent, roundID string, targetPaths []string) (appruns.WorkspacePatch, bool, int, error) {
	return normalizeBuilderRuntimePatchWithWorkspace("", rawContent, roundID, targetPaths)
}

func normalizeBuilderRuntimePatchWithWorkspace(workspacePath, rawContent, roundID string, targetPaths []string) (appruns.WorkspacePatch, bool, int, error) {
	normalizedContent, normalizedFence := sanitizeBuilderRuntimeJSON(rawContent)
	payload, normalizedRootArray, err := normalizeBuilderRuntimePayload(normalizedContent, roundID)
	if err != nil {
		if recoveredPatch, recovered := recoverBuilderRuntimeSingleTargetWriteFilePatchWithWorkspace(workspacePath, rawContent, roundID, targetPaths); recovered {
			driftCount := btoi(normalizedFence) + btoi(normalizedRootArray) + 1
			return recoveredPatch, true, driftCount, nil
		}
		usedNormalization := normalizedFence || normalizedRootArray
		return appruns.WorkspacePatch{}, usedNormalization, btoi(normalizedFence) + btoi(normalizedRootArray), err
	}
	usedNormalization := normalizedFence || normalizedRootArray
	targetPathSet := make(map[string]struct{}, len(targetPaths))
	for _, targetPath := range targetPaths {
		normalizedTargetPath := normalizeBuilderRuntimeTargetPath(targetPath)
		if normalizedTargetPath == "" {
			continue
		}
		targetPathSet[normalizedTargetPath] = struct{}{}
	}
	patchID := firstString(payload, "patch_id", "case_id", "id")
	if strings.TrimSpace(patchID) == "" {
		patchID = strings.TrimSpace(roundID) + "-patch"
	}
	opsValue, ok := payload["operations"]
	if !ok {
		return appruns.WorkspacePatch{}, true, 1, fmt.Errorf("operations missing from builder runtime response")
	}
	opsSlice, ok := opsValue.([]any)
	if !ok || len(opsSlice) == 0 {
		return appruns.WorkspacePatch{}, true, 1, fmt.Errorf("operations must be a non-empty array")
	}
	patch := appruns.WorkspacePatch{PatchID: patchID, Status: "generated", Operations: make([]appruns.WorkspacePatchOperation, 0, len(opsSlice))}
	driftCount := btoi(normalizedFence) + btoi(normalizedRootArray)
	for _, item := range opsSlice {
		opMap, ok := item.(map[string]any)
		if !ok {
			return appruns.WorkspacePatch{}, true, driftCount + 1, fmt.Errorf("operation is not an object")
		}
		flattenedOpMap, normalizedOperation := normalizeBuilderRuntimeOperationShape(opMap)
		if normalizedOperation {
			driftCount++
			usedNormalization = true
		}
		opMap = flattenedOpMap
		if conflictingType, _ := firstStringWithKey(opMap, "_conflicting_type"); conflictingType != "" {
			return appruns.WorkspacePatch{}, true, driftCount + 1, fmt.Errorf("builder runtime operation type conflict: outer=%s inner=%s", firstString(opMap, "type"), conflictingType)
		}
		opType, typeKey := firstStringWithKey(opMap, "type", "action", "op", "kind", "operation", "operation_type")
		path, pathKey := firstStringWithKey(opMap, "path", "file_path", "target_path", "file")
		normalizedType := normalizeBuilderRuntimeOperationType(opType)
		if normalizedType == "" {
			return appruns.WorkspacePatch{}, true, driftCount + 1, fmt.Errorf("unsupported builder runtime operation %q", opType)
		}
		if strings.TrimSpace(path) == "" {
			return appruns.WorkspacePatch{}, true, driftCount + 1, fmt.Errorf("builder runtime operation path is empty")
		}
		op := appruns.WorkspacePatchOperation{Type: normalizedType, Path: strings.TrimSpace(path)}
		if typeKey != "type" || pathKey != "path" {
			driftCount++
			usedNormalization = true
		}
		switch normalizedType {
		case "write_file":
			content, contentKey := firstStringWithKey(opMap, "content", "new_content", "replacement", "file_content")
			if contentKey != "content" && contentKey != "" {
				driftCount++
				usedNormalization = true
			}
			op.Content = content
		case "replace_block":
			anchor, anchorKey := firstStringWithKey(opMap, "anchor", "old_content", "match")
			start, startKey := firstStringWithKey(opMap, "start", "start_marker", "begin")
			end, endKey := firstStringWithKey(opMap, "end", "end_marker", "finish")
			replacement, replacementKey := firstStringWithKey(opMap, "new_content", "content", "replacement")
			if anchorKey != "anchor" && anchorKey != "" {
				driftCount++
				usedNormalization = true
			}
			if (startKey != "start" && startKey != "") || (endKey != "end" && endKey != "") {
				driftCount++
				usedNormalization = true
			}
			if replacementKey != "new_content" && replacementKey != "" {
				driftCount++
				usedNormalization = true
			}
			op.Anchor = strings.TrimSpace(anchor)
			if anchorKey == "old_content" {
				op.OldContent = strings.TrimSpace(anchor)
			}
			op.Start = strings.TrimSpace(start)
			op.End = strings.TrimSpace(end)
			op.NewContent = replacement
			if op.Anchor == "" && op.OldContent == "" && (op.Start == "" || op.End == "") {
				return appruns.WorkspacePatch{}, true, driftCount + 1, fmt.Errorf("replace_block requires anchor, old_content, or start/end for %s", op.Path)
			}
			if replacementKey == "" {
				return appruns.WorkspacePatch{}, true, driftCount + 1, fmt.Errorf("replace_block requires new_content, content, or replacement for %s", op.Path)
			}
		case "delete_file":
			if _, blocked := targetPathSet[normalizeBuilderRuntimeTargetPath(op.Path)]; blocked {
				return appruns.WorkspacePatch{}, usedNormalization, driftCount, fmt.Errorf("delete_file cannot remove target path %s", op.Path)
			}
		}
		patch.Operations = append(patch.Operations, op)
	}
	if len(targetPaths) > 0 {
		sort.Strings(targetPaths)
	}
	return patch, usedNormalization, driftCount, nil
}

func normalizeBuilderRuntimePayload(rawContent, roundID string) (map[string]any, bool, error) {
	var payload map[string]any
	if err := json.Unmarshal([]byte(rawContent), &payload); err == nil {
		return payload, false, nil
	}
	var payloadArray []any
	if err := json.Unmarshal([]byte(rawContent), &payloadArray); err != nil {
		return nil, false, err
	}
	normalizedPayload, ok := normalizeBuilderRuntimePayloadArray(payloadArray, roundID)
	if !ok {
		return nil, false, fmt.Errorf("builder runtime root array must contain a patch object or operation objects")
	}
	return normalizedPayload, true, nil
}

func normalizeBuilderRuntimePayloadArray(payloadArray []any, roundID string) (map[string]any, bool) {
	if len(payloadArray) == 0 {
		return nil, false
	}
	if len(payloadArray) == 1 {
		if payload, ok := payloadArray[0].(map[string]any); ok {
			if _, exists := payload["operations"]; exists {
				return payload, true
			}
		}
	}
	for _, item := range payloadArray {
		opMap, ok := item.(map[string]any)
		if !ok {
			return nil, false
		}
		if firstString(opMap, "type", "action", "op", "kind", "operation", "operation_type") == "" {
			return nil, false
		}
	}
	patchID := strings.TrimSpace(roundID)
	if patchID == "" {
		patchID = "builder-runtime"
	}
	return map[string]any{
		"patch_id":   patchID + "-patch",
		"operations": payloadArray,
	}, true
}

func normalizeBuilderRuntimeTargetPath(path string) string {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return ""
	}
	normalized := filepath.ToSlash(filepath.Clean(filepath.FromSlash(trimmed)))
	return strings.TrimPrefix(normalized, "./")
}

func normalizeBuilderRuntimeOperationShape(opMap map[string]any) (map[string]any, bool) {
	operationKeys := make([]string, 0, 1)
	for key := range opMap {
		normalizedType := normalizeBuilderRuntimeOperationType(key)
		if normalizedType != "" {
			operationKeys = append(operationKeys, key)
		}
	}
	if len(operationKeys) != 1 {
		return opMap, false
	}
	for _, key := range operationKeys {
		value := opMap[key]
		normalizedType := normalizeBuilderRuntimeOperationType(key)
		nested, ok := value.(map[string]any)
		if ok {
			flattened := make(map[string]any, len(opMap)+len(nested))
			if nestedType, _ := firstStringWithKey(nested, "type"); nestedType != "" {
				normalizedNestedType := normalizeBuilderRuntimeOperationType(nestedType)
				if normalizedNestedType != "" && normalizedNestedType != normalizedType {
					flattened["_conflicting_type"] = normalizedNestedType
				}
			}
			for outerKey, outerValue := range opMap {
				if outerKey == key {
					continue
				}
				flattened[outerKey] = outerValue
			}
			for nestedKey, nestedValue := range nested {
				if nestedKey == "type" {
					continue
				}
				flattened[nestedKey] = nestedValue
			}
			flattened["type"] = normalizedType
			return flattened, true
		}
		flattened := make(map[string]any, len(opMap)+2)
		for outerKey, outerValue := range opMap {
			if outerKey == key {
				continue
			}
			flattened[outerKey] = outerValue
		}
		switch normalizedType {
		case "write_file", "replace_block", "delete_file":
			flattened["path"] = value
		default:
			return opMap, false
		}
		flattened["type"] = normalizedType
		return flattened, true
	}
	return opMap, false
}

func sanitizeBuilderRuntimeJSON(raw string) (string, bool) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return trimmed, false
	}
	normalized := false
	if strings.HasPrefix(trimmed, "```") {
		trimmed = strings.TrimPrefix(trimmed, "```")
		if index := strings.IndexByte(trimmed, '\n'); index >= 0 {
			trimmed = trimmed[index+1:]
		}
		trimmed = strings.TrimSpace(strings.TrimSuffix(trimmed, "```"))
		normalized = true
	}
	repairedStructureQuotes, repairedStructureQuotesOK := repairBuilderRuntimeEscapedQuotesOutsideStrings(trimmed)
	if repairedStructureQuotesOK {
		trimmed = repairedStructureQuotes
		normalized = true
	}
	repairedEscapes, repairedEscapesOK := repairBuilderRuntimeInvalidStringEscapes(trimmed)
	if repairedEscapesOK {
		trimmed = repairedEscapes
		normalized = true
	}
	strippedComments, strippedCommentsOK := stripBuilderRuntimeJSONComments(trimmed)
	if strippedCommentsOK {
		trimmed = strippedComments
		normalized = true
	}
	repaired, repairedOK := repairBuilderRuntimeJSONObject(trimmed)
	if repairedOK {
		return repaired, true
	}
	return trimmed, normalized
}

func repairBuilderRuntimeEscapedQuotesOutsideStrings(raw string) (string, bool) {
	if raw == "" || !strings.Contains(raw, `\"`) {
		return raw, false
	}
	var builder strings.Builder
	builder.Grow(len(raw))
	inString := false
	escaped := false
	changed := false

	for i := 0; i < len(raw); i++ {
		ch := raw[i]
		if inString {
			builder.WriteByte(ch)
			if escaped {
				escaped = false
				continue
			}
			if ch == '\\' {
				escaped = true
				continue
			}
			if ch == '"' {
				inString = false
			}
			continue
		}
		if ch == '\\' && i+1 < len(raw) && raw[i+1] == '"' {
			changed = true
			continue
		}
		builder.WriteByte(ch)
		if ch == '"' {
			inString = true
		}
	}

	if !changed {
		return raw, false
	}
	return builder.String(), true
}

func stripBuilderRuntimeJSONComments(raw string) (string, bool) {
	if raw == "" || !strings.Contains(raw, "/") {
		return raw, false
	}
	var builder strings.Builder
	builder.Grow(len(raw))
	inString := false
	escaped := false
	inLineComment := false
	inBlockComment := false
	changed := false

	for i := 0; i < len(raw); i++ {
		ch := raw[i]
		if inLineComment {
			changed = true
			switch ch {
			case '\n', '\r':
				builder.WriteByte(ch)
				inLineComment = false
			}
			continue
		}
		if inBlockComment {
			changed = true
			if ch == '*' && i+1 < len(raw) && raw[i+1] == '/' {
				inBlockComment = false
				i++
				continue
			}
			switch ch {
			case '\n', '\r', '\t', ' ':
				builder.WriteByte(ch)
			}
			continue
		}
		if inString {
			builder.WriteByte(ch)
			if escaped {
				escaped = false
				continue
			}
			if ch == '\\' {
				escaped = true
				continue
			}
			if ch == '"' {
				inString = false
			}
			continue
		}
		if ch == '"' {
			inString = true
			builder.WriteByte(ch)
			continue
		}
		if ch == '/' && i+1 < len(raw) {
			switch raw[i+1] {
			case '/':
				inLineComment = true
				changed = true
				i++
				continue
			case '*':
				inBlockComment = true
				changed = true
				i++
				continue
			}
		}
		builder.WriteByte(ch)
	}

	if !changed {
		return raw, false
	}
	return builder.String(), true
}

var builderRuntimeLikelyJSONObjectStartPattern = regexp.MustCompile(`\{\s*"(?:patch_id|case_id|operations)"\s*:`)

var (
	builderRuntimeMainLocalProjectImportPattern = regexp.MustCompile(`(?m)^\s*import\s+['"](?:(?:\./|\.\./)?|package:[^/'"]+/)(?:views|controllers|repositories|models|template)/[^'"]+\.dart['"]\s*;`)
	builderRuntimeMainSurfaceConstructorPattern = regexp.MustCompile(`home\s*:\s*(?:const\s+)?([A-Z][A-Za-z0-9_]*)\s*\(`)
)

type builderRuntimeFencedCodeBlock struct {
	language string
	content  string
}

func recoverBuilderRuntimeSingleTargetWriteFilePatch(rawContent, roundID string, targetPaths []string) (appruns.WorkspacePatch, bool) {
	return recoverBuilderRuntimeSingleTargetWriteFilePatchWithWorkspace("", rawContent, roundID, targetPaths)
}

func recoverBuilderRuntimeSingleTargetWriteFilePatchWithWorkspace(workspacePath, rawContent, roundID string, targetPaths []string) (appruns.WorkspacePatch, bool) {
	if len(targetPaths) != 1 {
		return appruns.WorkspacePatch{}, false
	}
	targetPath := normalizeBuilderRuntimeTargetPath(targetPaths[0])
	if !strings.HasSuffix(targetPath, ".dart") {
		return appruns.WorkspacePatch{}, false
	}
	content, ok := extractBuilderRuntimeSingleTargetDartContentWithWorkspace(workspacePath, rawContent, targetPath)
	if !ok {
		return appruns.WorkspacePatch{}, false
	}
	patchID := strings.TrimSpace(roundID)
	if patchID == "" {
		patchID = "builder-runtime"
	}
	return appruns.WorkspacePatch{
		PatchID: patchID + "-patch",
		Status:  "generated",
		Operations: []appruns.WorkspacePatchOperation{{
			Type:    "write_file",
			Path:    targetPath,
			Content: content,
		}},
	}, true
}

func extractBuilderRuntimeSingleTargetDartContent(rawContent, targetPath string) (string, bool) {
	return extractBuilderRuntimeSingleTargetDartContentWithWorkspace("", rawContent, targetPath)
}

func extractBuilderRuntimeSingleTargetDartContentWithWorkspace(workspacePath, rawContent, targetPath string) (string, bool) {
	if !strings.HasSuffix(targetPath, ".dart") {
		return "", false
	}
	bestScore := 0
	bestContent := ""
	for _, block := range extractBuilderRuntimeFencedCodeBlocks(rawContent) {
		candidate := strings.TrimSpace(block.content)
		score := builderRuntimeSingleTargetDartCandidateScoreWithWorkspace(workspacePath, targetPath, block.language, candidate)
		if score <= 0 {
			continue
		}
		if score > bestScore || (score == bestScore && len(candidate) >= len(bestContent)) {
			bestScore = score
			bestContent = candidate
		}
	}
	if bestScore == 0 || strings.TrimSpace(bestContent) == "" {
		return "", false
	}
	if !strings.HasSuffix(bestContent, "\n") {
		bestContent += "\n"
	}
	return bestContent, true
}

func builderRuntimeSingleTargetDartCandidateScore(targetPath, language, content string) int {
	return builderRuntimeSingleTargetDartCandidateScoreWithWorkspace("", targetPath, language, content)
}

func builderRuntimeSingleTargetDartCandidateScoreWithWorkspace(workspacePath, targetPath, language, content string) int {
	trimmedContent := strings.TrimSpace(content)
	if !builderRuntimeRecoveredSingleTargetDartLooksValid(targetPath, trimmedContent) {
		return 0
	}
	score := 1
	trimmedLanguage := strings.ToLower(strings.TrimSpace(language))
	if strings.HasPrefix(trimmedLanguage, "dart") {
		score += 10
	}
	if strings.Contains(trimmedContent, "void main(") {
		score += 20
	}
	if strings.Contains(trimmedContent, "runApp(") {
		score += 10
	}
	if strings.Contains(trimmedContent, "import 'package:flutter/") {
		score += 8
	}
	if strings.Contains(trimmedContent, "MaterialApp(") || strings.Contains(trimmedContent, "CupertinoApp(") {
		score += 8
	}
	if strings.Contains(trimmedContent, "class ") {
		score += 4
	}
	if targetPath == "lib/main.dart" {
		score += builderRuntimeSingleTargetMainCandidateScoreWithWorkspace(workspacePath, trimmedContent)
	}
	return score
}

func builderRuntimeSingleTargetMainCandidateScore(content string) int {
	return builderRuntimeSingleTargetMainCandidateScoreWithWorkspace("", content)
}

func builderRuntimeSingleTargetMainCandidateScoreWithWorkspace(workspacePath, content string) int {
	score := 0
	if builderRuntimeMainLocalProjectImportPattern.MatchString(content) {
		score += 4
	}
	if builderRuntimeMainCandidateHasSurfaceConstructor(content) {
		score += 4
	}
	score += builderRuntimeSingleTargetMainRegistryScore(workspacePath, content)
	if strings.Contains(content, "flutter_application_1/main.dart") {
		score -= 20
	}
	return score
}

func builderRuntimeSingleTargetMainRegistryScore(workspacePath, content string) int {
	if strings.TrimSpace(workspacePath) == "" || strings.TrimSpace(content) == "" {
		return 0
	}
	score := 0
	for _, entry := range []builderRuntimeOpenLiteSurfaceRegistryEntry{
		builderRuntimeOpenLiteOverviewSurfaceRegistry(workspacePath).view,
		builderRuntimeOpenLiteCollectionSurfaceRegistry(workspacePath).view,
	} {
		resolvedPath := strings.TrimPrefix(strings.TrimSpace(entry.resolvedPath), "lib/")
		if resolvedPath != "" && strings.Contains(content, "import '"+resolvedPath+"';") {
			score += 3
		}
		resolvedClass := strings.TrimSpace(entry.resolvedClassName)
		if resolvedClass != "" && strings.Contains(content, resolvedClass+"(") {
			score += 3
		}
	}
	return score
}

func builderRuntimeMainCandidateHasSurfaceConstructor(content string) bool {
	matches := builderRuntimeMainSurfaceConstructorPattern.FindAllStringSubmatch(content, -1)
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		if !builderRuntimeIsPlaceholderMainSurfaceConstructor(match[1]) {
			return true
		}
	}
	return false
}

func builderRuntimeIsPlaceholderMainSurfaceConstructor(name string) bool {
	switch strings.TrimSpace(name) {
	case "Center", "Column", "Container", "CupertinoApp", "Icon", "MaterialApp", "Placeholder", "Row", "SafeArea", "Scaffold", "SizedBox", "Text":
		return true
	default:
		return false
	}
}

func builderRuntimeRecoveredSingleTargetDartLooksValid(targetPath, content string) bool {
	trimmedContent := strings.TrimSpace(content)
	if trimmedContent == "" {
		return false
	}
	if strings.Contains(trimmedContent, `"operations"`) || strings.HasPrefix(trimmedContent, "{") {
		return false
	}
	if targetPath == "lib/main.dart" {
		return strings.Contains(trimmedContent, "void main(") && strings.Contains(trimmedContent, "runApp(")
	}
	return strings.Contains(trimmedContent, "class ") || strings.Contains(trimmedContent, "enum ") || strings.Contains(trimmedContent, "typedef ") || strings.Contains(trimmedContent, "extension ")
}

func extractBuilderRuntimeFencedCodeBlocks(raw string) []builderRuntimeFencedCodeBlock {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || !strings.Contains(trimmed, "```") {
		return nil
	}
	lines := strings.Split(trimmed, "\n")
	blocks := make([]builderRuntimeFencedCodeBlock, 0, 2)
	inFence := false
	language := ""
	contentLines := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmedLine := strings.TrimSpace(line)
		if !inFence {
			if strings.HasPrefix(trimmedLine, "```") {
				inFence = true
				language = strings.TrimSpace(strings.TrimPrefix(trimmedLine, "```"))
				contentLines = contentLines[:0]
			}
			continue
		}
		if strings.HasPrefix(trimmedLine, "```") {
			blocks = append(blocks, builderRuntimeFencedCodeBlock{
				language: language,
				content:  strings.Join(contentLines, "\n"),
			})
			inFence = false
			language = ""
			contentLines = contentLines[:0]
			continue
		}
		contentLines = append(contentLines, line)
	}
	return blocks
}

func builderRuntimeLikelyJSONObjectStart(raw string) int {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return -1
	}
	if strings.HasPrefix(trimmed, "{") {
		return 0
	}
	location := builderRuntimeLikelyJSONObjectStartPattern.FindStringIndex(trimmed)
	if len(location) != 2 {
		return -1
	}
	return location[0]
}

func repairBuilderRuntimeInvalidStringEscapes(raw string) (string, bool) {
	if raw == "" {
		return raw, false
	}
	var builder strings.Builder
	builder.Grow(len(raw))
	inString := false
	changed := false

	for i := 0; i < len(raw); i++ {
		ch := raw[i]
		if !inString {
			builder.WriteByte(ch)
			if ch == '"' {
				inString = true
			}
			continue
		}
		if ch == '\\' {
			if i+1 >= len(raw) {
				builder.WriteByte(ch)
				continue
			}
			next := raw[i+1]
			if isValidBuilderRuntimeJSONEscape(next) {
				builder.WriteByte(ch)
				builder.WriteByte(next)
				i++
				continue
			}
			changed = true
			continue
		}
		if ch == '\n' {
			builder.WriteString(`\n`)
			changed = true
			continue
		}
		if ch == '\r' {
			builder.WriteString(`\r`)
			changed = true
			continue
		}
		if ch == '\t' {
			builder.WriteString(`\t`)
			changed = true
			continue
		}
		if ch < 0x20 {
			builder.WriteString(fmt.Sprintf("\\u%04x", ch))
			changed = true
			continue
		}
		builder.WriteByte(ch)
		if ch == '"' {
			inString = false
		}
	}

	if !changed {
		return raw, false
	}
	return builder.String(), true
}

func isValidBuilderRuntimeJSONEscape(ch byte) bool {
	switch ch {
	case '"', '\\', '/', 'b', 'f', 'n', 'r', 't', 'u':
		return true
	default:
		return false
	}
}

func repairBuilderRuntimeJSONObject(raw string) (string, bool) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return trimmed, false
	}
	start := builderRuntimeLikelyJSONObjectStart(trimmed)
	if start < 0 {
		return trimmed, false
	}
	changed := start > 0
	trimmed = trimmed[start:]

	stack := make([]byte, 0, len(trimmed))
	var builder strings.Builder
	builder.Grow(len(trimmed) + 8)
	inString := false
	escaped := false
	completedRoot := false

	for i := 0; i < len(trimmed); i++ {
		ch := trimmed[i]
		if completedRoot {
			if ch == ' ' || ch == '\n' || ch == '\r' || ch == '\t' {
				continue
			}
			changed = true
			continue
		}
		if inString {
			builder.WriteByte(ch)
			if escaped {
				escaped = false
				continue
			}
			if ch == '\\' {
				escaped = true
				continue
			}
			if ch == '"' {
				inString = false
			}
			continue
		}
		switch ch {
		case '"':
			inString = true
			builder.WriteByte(ch)
		case '{':
			stack = append(stack, '}')
			builder.WriteByte(ch)
		case '[':
			stack = append(stack, ']')
			builder.WriteByte(ch)
		case '}', ']':
			for len(stack) > 0 && stack[len(stack)-1] != ch {
				builder.WriteByte(stack[len(stack)-1])
				stack = stack[:len(stack)-1]
				changed = true
			}
			if len(stack) == 0 {
				changed = true
				continue
			}
			builder.WriteByte(ch)
			stack = stack[:len(stack)-1]
			if len(stack) == 0 {
				completedRoot = true
			}
		case ',':
			nextSignificant := nextSignificantBuilderRuntimeJSONChar(trimmed, i+1)
			if nextSignificant == '}' || nextSignificant == ']' || nextSignificant == 0 {
				changed = true
				continue
			}
			for len(stack) > 0 && stack[len(stack)-1] == '}' && nextSignificant != '"' && nextSignificant != '}' {
				builder.WriteByte(stack[len(stack)-1])
				stack = stack[:len(stack)-1]
				changed = true
			}
			builder.WriteByte(ch)
		default:
			builder.WriteByte(ch)
		}
	}

	if inString {
		return trimmed, changed
	}
	for len(stack) > 0 {
		builder.WriteByte(stack[len(stack)-1])
		stack = stack[:len(stack)-1]
		changed = true
	}
	if !changed {
		return trimmed, false
	}
	return strings.TrimSpace(builder.String()), true
}

func nextSignificantBuilderRuntimeJSONChar(raw string, start int) byte {
	for i := start; i < len(raw); i++ {
		switch raw[i] {
		case ' ', '\n', '\r', '\t':
			continue
		default:
			return raw[i]
		}
	}
	return 0
}

func normalizeBuilderRuntimeOperationType(value string) string {
	switch strings.TrimSpace(strings.ToLower(value)) {
	case "write_file", "write", "create_file", "replace_file", "update_file":
		return "write_file"
	case "replace_block", "replace", "edit_block", "update_block":
		return "replace_block"
	case "delete_file", "delete", "remove_file":
		return "delete_file"
	default:
		return ""
	}
}

func selectBuilderRuntimeRoute(run runRecord) (appruns.BuilderRuntimeTaskRoute, bool) {
	selectedTask := preferredBuilderRuntimeTask(run.TaskBundle, run.RoundState)
	selected := appruns.BuilderRuntimeTaskRoute{TaskID: selectedTask.TaskID, TaskType: selectedTask.EffectiveTaskType(), RouteSource: "unconfigured"}
	if run.BuilderRuntime == nil {
		return selected, false
	}
	for _, route := range run.BuilderRuntime.TaskRoutes {
		if strings.TrimSpace(route.TaskID) != "" && route.TaskID == selectedTask.TaskID {
			selected = route
			if selected.TaskID == "" {
				selected.TaskID = selectedTask.TaskID
			}
			if selected.TaskType == "" {
				selected.TaskType = selectedTask.EffectiveTaskType()
			}
			break
		}
		if strings.TrimSpace(route.TaskID) == "" && route.TaskType == selectedTask.EffectiveTaskType() {
			selected = route
			selected.TaskID = selectedTask.TaskID
			if selected.TaskType == "" {
				selected.TaskType = selectedTask.EffectiveTaskType()
			}
			break
		}
	}
	if selected.Model.Primary == "" && run.BuilderRuntime.DefaultModel.Primary != "" && selected.RouteSource != "route_hint" {
		selected.Model = run.BuilderRuntime.DefaultModel
		if selected.RouteSource == "unconfigured" {
			selected.RouteSource = "default_model"
		}
	}
	if shouldUpgradeBuilderRuntimeFromStart(run, selected) && run.BuilderRuntime.UpgradeModel.Primary != "" && selected.RouteSource != "route_hint" {
		selected.Model = run.BuilderRuntime.UpgradeModel
		selected.RouteSource = "upgrade_threshold"
		return selected, true
	}
	return selected, false
}

func shouldUpgradeBuilderRuntimeFromStart(run runRecord, route appruns.BuilderRuntimeTaskRoute) bool {
	if run.BuilderRuntime == nil {
		return false
	}
	threshold := run.BuilderRuntime.UpgradeThreshold
	if threshold.MaxAttemptsBeforeUpgrade > 0 && maxInt(run.IterationCount, 1) > threshold.MaxAttemptsBeforeUpgrade {
		return true
	}
	if threshold.MaxFilesBeforeUpgrade > 0 && len(concreteTaskTargetPaths(run.TaskBundle)) > threshold.MaxFilesBeforeUpgrade {
		return true
	}
	return route.Model.Primary == "" && run.BuilderRuntime.UpgradeModel.Primary != ""
}

func shouldRetryBuilderRuntimeWithUpgrade(run runRecord, stats *appruns.BuilderRuntimeExecutionStats, reason string) bool {
	if run.BuilderRuntime == nil {
		return false
	}
	if stats == nil || stats.Attempts >= 2 {
		return false
	}
	upgradeModel, ok := nextBuilderRuntimeUpgradeModel(run.BuilderRuntime, stats.SelectedModel)
	if !ok {
		return false
	}
	if stats.UpgradeApplied && reason != "semantic_conflict" {
		return false
	}
	switch reason {
	case "parse_failure":
		return run.BuilderRuntime.UpgradeThreshold.UpgradeOnPatchParseFail
	case "schema_drift":
		return run.BuilderRuntime.UpgradeThreshold.MaxSchemaDriftBeforeUpgrade > 0 && stats.SchemaDriftCount >= run.BuilderRuntime.UpgradeThreshold.MaxSchemaDriftBeforeUpgrade
	case "unrelated_edits":
		return run.BuilderRuntime.UpgradeThreshold.MaxUnrelatedOperationRate > 0 && stats.UnrelatedOperationRate >= run.BuilderRuntime.UpgradeThreshold.MaxUnrelatedOperationRate
	case "scope_violation":
		return run.BuilderRuntime.UpgradeThreshold.UpgradeOnScopeViolation
	case "validation_failure":
		return run.BuilderRuntime.UpgradeThreshold.UpgradeOnValidationFail
	case "semantic_conflict":
		if run.BuilderRuntime.UpgradeThreshold.UpgradeOnSemanticConflict {
			return true
		}
		if upgradeModel.Primary != "" && stats.SelectedModel == run.BuilderRuntime.UpgradeModel.Primary {
			return true
		}
		return stats.TaskType == appruns.BuilderRuntimeTaskTypeAnalyzeRepair || stats.TaskType == appruns.BuilderRuntimeTaskTypeTestRepair
	default:
		return false
	}
}

func (runner *Runner) generateBuilderRuntimePatchWithTransientRetry(ctx context.Context, request BuilderRuntimePatchRequest) (BuilderRuntimePatchResponse, int, error) {
	return runner.generateBuilderRuntimePatchWithRetryFunc(ctx, request, func(ctx context.Context, request BuilderRuntimePatchRequest) (BuilderRuntimePatchResponse, error) {
		return runner.generateBuilderRuntimePatch(ctx, request)
	})
}

func (runner *Runner) generateBuilderRuntimePatchWithProgressAndTransientRetry(ctx context.Context, request BuilderRuntimePatchRequest, onWaiting func(alias string, elapsed time.Duration) error) (BuilderRuntimePatchResponse, int, error) {
	return runner.generateBuilderRuntimePatchWithRetryFunc(ctx, request, func(ctx context.Context, request BuilderRuntimePatchRequest) (BuilderRuntimePatchResponse, error) {
		return runner.generateBuilderRuntimePatchWithProgress(ctx, request, onWaiting)
	})
}

func (runner *Runner) generateBuilderRuntimePatchWithRetryFunc(ctx context.Context, request BuilderRuntimePatchRequest, generate func(context.Context, BuilderRuntimePatchRequest) (BuilderRuntimePatchResponse, error)) (BuilderRuntimePatchResponse, int, error) {
	requestAttempts := 0
	for {
		response, err := generate(ctx, request)
		requestAttempts++
		if err == nil {
			return response, requestAttempts, nil
		}
		if !shouldRetryBuilderRuntimeTransientModelRequestFailure(requestAttempts, err) {
			return BuilderRuntimePatchResponse{}, requestAttempts, err
		}
		if waitErr := waitBuilderRuntimeTransientModelRequestRetry(ctx); waitErr != nil {
			return BuilderRuntimePatchResponse{}, requestAttempts, waitErr
		}
	}
}

func shouldRetryBuilderRuntimeTransientModelRequestFailure(requestAttempts int, modelErr error) bool {
	if requestAttempts >= 2 || modelErr == nil {
		return false
	}
	classified := providers.ClassifyError(modelErr, "builder-runtime", "")
	if classified == nil {
		return false
	}
	switch classified.Reason {
	case providers.FailoverRateLimit, providers.FailoverTimeout:
		return true
	default:
		return false
	}
}

func shouldRetryBuilderRuntimeTransientPatchParseFailure(parseAttempts int, responseContent string, parseErr error) bool {
	if parseAttempts >= 2 || parseErr == nil {
		return false
	}
	if strings.TrimSpace(responseContent) == "" {
		return false
	}
	message := strings.ToLower(strings.TrimSpace(parseErr.Error()))
	return strings.Contains(message, "unexpected end of json input") || strings.Contains(message, "unexpected eof")
}

func waitBuilderRuntimeTransientModelRequestRetry(ctx context.Context) error {
	delay := builderRuntimeTransientModelRequestRetryDelay
	if delay <= 0 {
		return nil
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func shouldRetryBuilderRuntimeDirectFailureCoverageRepair(taskType appruns.BuilderRuntimeTaskType, stats *appruns.BuilderRuntimeExecutionStats, modelAlias, invalidContent string) bool {
	if stats == nil || stats.Attempts >= 2 {
		return false
	}
	switch taskType {
	case appruns.BuilderRuntimeTaskTypeAnalyzeRepair, appruns.BuilderRuntimeTaskTypeTestRepair:
	default:
		return false
	}
	return strings.TrimSpace(modelAlias) != "" && strings.TrimSpace(invalidContent) != ""
}

func loadBuilderRuntimeDomainModelContext(workspacePath string) (string, error) {
	if strings.TrimSpace(workspacePath) == "" {
		return "", nil
	}
	jobRoot := filepath.Dir(workspacePath)
	domainModelPath := filepath.Join(jobRoot, "prepare", "domain-model.json")
	content, err := os.ReadFile(domainModelPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("read prepare domain model for builder runtime prompt: %w", err)
	}
	trimmed := strings.TrimSpace(string(content))
	if trimmed == "" {
		return "", nil
	}
	const maxBytes = 12000
	if len(trimmed) > maxBytes {
		trimmed = trimmed[:maxBytes]
	}
	return trimmed, nil
}

func loadBuilderRuntimeExportedModelContext(workspacePath string) (string, error) {
	if strings.TrimSpace(workspacePath) == "" {
		return "", nil
	}
	modelDir := filepath.Join(workspacePath, "lib", "models")
	entries, err := os.ReadDir(modelDir)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	lines := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".dart" {
			continue
		}
		path := filepath.Join(modelDir, entry.Name())
		content, readErr := os.ReadFile(path)
		if readErr != nil {
			return "", readErr
		}
		exports := builderRuntimeExportedDartTypes(string(content))
		if len(exports) == 0 {
			continue
		}
		lines = append(lines, fmt.Sprintf("lib/models/%s: %s", entry.Name(), strings.Join(exports, ", ")))
	}
	if len(lines) == 0 {
		return "", nil
	}
	sort.Strings(lines)
	return strings.Join(lines, "\n"), nil
}

func loadBuilderRuntimeModelAPISnapshot(workspacePath string) (string, error) {
	if strings.TrimSpace(workspacePath) == "" {
		return "", nil
	}
	modelDir := filepath.Join(workspacePath, "lib", "models")
	entries, err := os.ReadDir(modelDir)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	lines := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".dart" {
			continue
		}
		path := filepath.Join(modelDir, entry.Name())
		content, readErr := os.ReadFile(path)
		if readErr != nil {
			return "", readErr
		}
		parts := make([]string, 0, 3)
		if exports := builderRuntimeExportedDartTypes(string(content)); len(exports) > 0 {
			parts = append(parts, fmt.Sprintf("exports=%s", strings.Join(exports, ", ")))
		}
		if fields := builderRuntimeDartFieldNames(string(content)); len(fields) > 0 {
			parts = append(parts, fmt.Sprintf("fields=%s", strings.Join(fields, ", ")))
		}
		if enumValues := builderRuntimeDartEnumValues(string(content)); len(enumValues) > 0 {
			parts = append(parts, fmt.Sprintf("enum_values=%s", strings.Join(enumValues, ", ")))
		}
		if len(parts) == 0 {
			continue
		}
		lines = append(lines, fmt.Sprintf("lib/models/%s: %s", entry.Name(), strings.Join(parts, " | ")))
	}
	if len(lines) == 0 {
		return "", nil
	}
	sort.Strings(lines)
	return strings.Join(lines, "\n"), nil
}

func builderRuntimeForbiddenTokensFromFailureContext(failureContext string) []string {
	failureContext = strings.TrimSpace(failureContext)
	if failureContext == "" || !strings.Contains(failureContext, "patch content reintroduced symbols absent from current models:") {
		return nil
	}
	clauses := strings.Split(failureContext, ";")
	seen := map[string]struct{}{}
	tokens := make([]string, 0)
	for _, clause := range clauses {
		arrow := strings.Index(clause, "->")
		if arrow < 0 {
			continue
		}
		for _, token := range strings.Split(clause[arrow+2:], ",") {
			trimmed := strings.TrimSpace(token)
			if trimmed == "" {
				continue
			}
			if _, ok := seen[trimmed]; ok {
				continue
			}
			seen[trimmed] = struct{}{}
			tokens = append(tokens, trimmed)
		}
	}
	sort.Strings(tokens)
	return tokens
}

func builderRuntimeDartFieldNames(content string) []string {
	matches := builderRuntimeDartFieldPattern.FindAllStringSubmatch(content, -1)
	if len(matches) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	fields := make([]string, 0, len(matches))
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		name := strings.TrimSpace(match[1])
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		fields = append(fields, name)
	}
	sort.Strings(fields)
	return fields
}

func builderRuntimeDartEnumValues(content string) []string {
	matches := builderRuntimeDartEnumValuePattern.FindAllStringSubmatch(content, -1)
	if len(matches) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	values := make([]string, 0, len(matches))
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		name := strings.TrimSpace(match[1])
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		values = append(values, name)
	}
	sort.Strings(values)
	return values
}

func dominantBuilderRuntimeTask(tasks []appruns.TaskBundleItem) appruns.TaskBundleItem {
	if len(tasks) == 0 {
		return appruns.TaskBundleItem{}
	}
	selected := appruns.NormalizeTaskBundleItem(tasks[0])
	selectedScore := builderRuntimeTaskSelectionScore(selected)
	for _, candidate := range tasks[1:] {
		normalized := appruns.NormalizeTaskBundleItem(candidate)
		score := builderRuntimeTaskSelectionScore(normalized)
		if score > selectedScore {
			selected = normalized
			selectedScore = score
		}
	}
	return selected
}

func preferredBuilderRuntimeTask(tasks []appruns.TaskBundleItem, roundState *appruns.RoundState) appruns.TaskBundleItem {
	statusByTask := builderRuntimeTaskStatuses(roundState)
	for _, candidate := range tasks {
		normalized := appruns.NormalizeTaskBundleItem(candidate)
		taskID := strings.TrimSpace(normalized.TaskID)
		if taskID == "" {
			continue
		}
		if builderRuntimeTaskIsValidated(statusByTask[taskID]) {
			continue
		}
		if !builderRuntimeDependenciesValidated(normalized.Dependencies, statusByTask) {
			continue
		}
		return normalized
	}
	for _, candidate := range tasks {
		normalized := appruns.NormalizeTaskBundleItem(candidate)
		if !builderRuntimeTaskIsValidated(statusByTask[strings.TrimSpace(normalized.TaskID)]) {
			return normalized
		}
	}
	return dominantBuilderRuntimeTask(tasks)
}

func builderRuntimeTaskStatuses(roundState *appruns.RoundState) map[string]appruns.BuilderRuntimeTaskStatus {
	if roundState == nil || len(roundState.TaskStatuses) == 0 {
		return nil
	}
	statuses := make(map[string]appruns.BuilderRuntimeTaskStatus, len(roundState.TaskStatuses))
	for taskID, status := range roundState.TaskStatuses {
		normalizedTaskID := strings.TrimSpace(taskID)
		if normalizedTaskID == "" {
			continue
		}
		if normalizedStatus := appruns.NormalizeBuilderRuntimeTaskStatus(string(status)); normalizedStatus != "" {
			statuses[normalizedTaskID] = normalizedStatus
		}
	}
	if len(statuses) == 0 {
		return nil
	}
	return statuses
}

func builderRuntimeDependenciesValidated(dependencies []string, statusByTask map[string]appruns.BuilderRuntimeTaskStatus) bool {
	for _, dependency := range dependencies {
		dependency = strings.TrimSpace(dependency)
		if dependency == "" {
			continue
		}
		if !builderRuntimeTaskIsValidated(statusByTask[dependency]) {
			return false
		}
	}
	return true
}

func builderRuntimeTaskIsValidated(status appruns.BuilderRuntimeTaskStatus) bool {
	return appruns.NormalizeBuilderRuntimeTaskStatus(string(status)) == appruns.BuilderRuntimeTaskStatusValidated
}

func builderRuntimeRoundState(existing *appruns.RoundState, step ExecutionStep, taskID string, status appruns.BuilderRuntimeTaskStatus) *appruns.RoundState {
	state := cloneRoundStateRef(existing)
	if state == nil {
		state = &appruns.RoundState{}
	}
	template := heartbeatRoundStateForStep(step)
	if template != nil {
		state.CurrentPhase = template.CurrentPhase
		state.PhaseTrace = append([]appruns.RoundPhase(nil), template.PhaseTrace...)
		state.NextAction = template.NextAction
		state.PreserveWorkspace = template.PreserveWorkspace
		state.ResumeAllowed = template.ResumeAllowed
	}
	taskID = strings.TrimSpace(taskID)
	if taskID != "" {
		state.CurrentTaskID = taskID
	}
	if normalizedStatus := appruns.NormalizeBuilderRuntimeTaskStatus(string(status)); normalizedStatus != "" && taskID != "" {
		if state.TaskStatuses == nil {
			state.TaskStatuses = map[string]appruns.BuilderRuntimeTaskStatus{}
		}
		state.TaskStatuses[taskID] = normalizedStatus
	}
	return state
}

func validateBuilderRuntimeTaskOutputs(run runRecord, taskID string) error {
	for _, relPath := range builderRuntimeTaskContextPaths(run.TaskBundle, taskID) {
		if !builderRuntimeTaskOwnsPath(run.TaskBundle, taskID, relPath) {
			continue
		}
		if !strings.HasSuffix(strings.ToLower(relPath), ".dart") {
			continue
		}
		absPath := filepath.Join(run.WorkspacePath, filepath.FromSlash(relPath))
		content, err := os.ReadFile(absPath)
		if err != nil {
			if os.IsNotExist(err) {
				return fmt.Errorf("task target file missing after patch apply: %s", relPath)
			}
			return fmt.Errorf("read task target file %s: %w", relPath, err)
		}
		if syntaxErr := validateBuilderRuntimeDartSyntax(run, relPath, absPath, content); syntaxErr != nil {
			return fmt.Errorf("task file %s failed Dart syntax validation: %w", relPath, syntaxErr)
		}
		baseDir := filepath.Dir(filepath.ToSlash(relPath))
		matches := dartLocalImportPattern.FindAllStringSubmatch(string(content), -1)
		for _, match := range matches {
			if len(match) < 2 {
				continue
			}
			ref := strings.TrimSpace(match[1])
			if ref == "" || !strings.HasPrefix(ref, ".") {
				continue
			}
			resolved := filepath.ToSlash(filepath.Clean(filepath.Join(baseDir, ref)))
			if strings.HasPrefix(resolved, "../") || strings.HasPrefix(resolved, "/") {
				return fmt.Errorf("task file %s references import outside workspace roots: %s", relPath, ref)
			}
			resolvedAbsPath := filepath.Join(run.WorkspacePath, filepath.FromSlash(resolved))
			if _, err := os.Stat(resolvedAbsPath); err != nil {
				if os.IsNotExist(err) {
					return fmt.Errorf("task file %s has unresolved local import/export/part %s -> %s", relPath, ref, resolved)
				}
				return fmt.Errorf("stat local import for %s: %w", relPath, err)
			}
		}
		if semanticErr := validateBuilderRuntimeTopologyScopedTaskOutput(run, relPath, content); semanticErr != nil {
			return semanticErr
		}
	}
	return nil
}

func validateBuilderRuntimeTopologyScopedTaskOutput(run runRecord, relPath string, content []byte) error {
	normalizedPath := filepath.ToSlash(strings.TrimSpace(relPath))
	body := string(content)
	if normalizedPath == "" || strings.TrimSpace(body) == "" {
		return nil
	}
	hasExplicitSurfaceTopology := builderRuntimeTaskBundleHasAnySurfaceRef(
		run.TaskBundle,
		builderRuntimeOverviewSurfaceRef,
		builderRuntimeCollectionSurfaceRef,
		builderRuntimeMutationSurfaceRef,
		builderRuntimeInspectionSurfaceRef,
	)
	hasOverviewSurface := builderRuntimeTaskBundleHasSurfaceRef(run.TaskBundle, builderRuntimeOverviewSurfaceRef) || (!hasExplicitSurfaceTopology && (containsTargetPath(run.TaskBundle, "lib/views/home_page.dart") || containsTargetPath(run.TaskBundle, "lib/controllers/home_controller.dart")))
	hasCollectionSurface := builderRuntimeTaskBundleHasSurfaceRef(run.TaskBundle, builderRuntimeCollectionSurfaceRef) || (!hasExplicitSurfaceTopology && (containsTargetPath(run.TaskBundle, "lib/views/record_list_page.dart") || containsTargetPath(run.TaskBundle, "lib/controllers/record_list_controller.dart")))
	hasMutationSurface := builderRuntimeTaskBundleHasSurfaceRef(run.TaskBundle, builderRuntimeMutationSurfaceRef) || (!hasExplicitSurfaceTopology && (containsTargetPath(run.TaskBundle, "lib/views/record_form_page.dart") || containsTargetPath(run.TaskBundle, "lib/controllers/record_form_controller.dart")))
	hasDetailSurface := builderRuntimeTaskBundleHasSurfaceRef(run.TaskBundle, builderRuntimeInspectionSurfaceRef) || (!hasExplicitSurfaceTopology && builderRuntimeTaskBundleHasFallbackInspectionSurface(run.TaskBundle))
	hasFilterFlow := builderRuntimeTaskBundleHasSemanticIntentRef(run.TaskBundle, "ac-filter")
	hasDeleteFlow := builderRuntimeTaskBundleHasSemanticIntentRef(run.TaskBundle, "ac-delete")
	overviewViewPaths, overviewControllerPaths := builderRuntimeOverviewSurfaceTargetPaths(run.TaskBundle)
	collectionViewPaths, collectionControllerPaths := builderRuntimeCollectionSurfaceTargetPaths(run.TaskBundle)
	mutationViewPaths, mutationControllerPaths := builderRuntimeMutationSurfaceTargetPaths(run.TaskBundle)
	if !hasExplicitSurfaceTopology {
		if len(overviewViewPaths) == 0 && len(overviewControllerPaths) == 0 {
			overviewViewPaths, overviewControllerPaths = builderRuntimeLikelyOverviewSurfaceTargetPaths(run.TaskBundle)
		}
		if len(collectionViewPaths) == 0 && len(collectionControllerPaths) == 0 {
			collectionViewPaths, collectionControllerPaths = builderRuntimeLikelyCollectionSurfaceTargetPaths(run.TaskBundle)
		}
		if len(mutationViewPaths) == 0 && len(mutationControllerPaths) == 0 {
			mutationViewPaths, mutationControllerPaths = builderRuntimeLikelyMutationSurfaceTargetPaths(run.TaskBundle)
		}
		hasOverviewSurface = hasOverviewSurface || len(overviewViewPaths) > 0 || len(overviewControllerPaths) > 0
		hasCollectionSurface = hasCollectionSurface || len(collectionViewPaths) > 0 || len(collectionControllerPaths) > 0
		hasMutationSurface = hasMutationSurface || len(mutationViewPaths) > 0 || len(mutationControllerPaths) > 0
	}
	if len(overviewViewPaths) == 0 {
		overviewViewPaths = []string{"lib/views/home_page.dart"}
	}
	if len(collectionViewPaths) == 0 {
		collectionViewPaths = []string{"lib/views/record_list_page.dart"}
	}
	if len(collectionControllerPaths) == 0 {
		collectionControllerPaths = []string{"lib/controllers/record_list_controller.dart"}
	}
	if len(mutationControllerPaths) == 0 {
		mutationControllerPaths = []string{"lib/controllers/record_form_controller.dart"}
	}
	isCollectionViewPath := slices.Contains(collectionViewPaths, normalizedPath)
	isCollectionControllerPath := slices.Contains(collectionControllerPaths, normalizedPath)
	isMutationControllerPath := slices.Contains(mutationControllerPaths, normalizedPath)
	switch {
	case normalizedPath == "lib/main.dart":
		if strings.Contains(body, "AppFactorySeedHomePage") || strings.Contains(body, "Seed workspace ready") {
			return fmt.Errorf("task file %s still uses open-lite seed placeholder app entry", relPath)
		}
		if strings.Contains(body, "MyHomePage") {
			return fmt.Errorf("task file %s still retains Flutter default counter-demo shell MyHomePage in app entry", relPath)
		}
		if strings.Count(body, "MaterialApp(") > 1 {
			return fmt.Errorf("task file %s still contains nested MaterialApp roots in app entry", relPath)
		}
		switch {
		case hasOverviewSurface && !builderRuntimeMainConstructsSurfaceView(run.WorkspacePath, body, overviewViewPaths, "HomePage"):
			return fmt.Errorf("task file %s did not wire the current overview surface into app entry", relPath)
		case !hasOverviewSurface && hasCollectionSurface && !builderRuntimeMainConstructsSurfaceView(run.WorkspacePath, body, collectionViewPaths, "RecordListPage"):
			return fmt.Errorf("task file %s did not wire the current collection surface into app entry", relPath)
		}
		if builderRuntimeMainConstructsConcreteRepositoryInline(run.WorkspacePath, body) {
			return fmt.Errorf("task file %s still constructs a concrete repository inline instead of initializing one shared repository in main() and injecting it through the app widget", relPath)
		}
		if builderRuntimeMainOmitsRequiredRepositoryParameter(body) {
			return fmt.Errorf("task file %s declares a required repository app-widget parameter but runApp does not pass it", relPath)
		}
		if err := builderRuntimeMainViewConstructorSignatureError(run.WorkspacePath, body); err != nil {
			return err
		}
		if !hasDetailSurface && hasCollectionSurface && builderRuntimeTaskOutputHasNullReturningCollectionCallback(body) {
			callbackName := builderRuntimeNullReturningCollectionCallbackName(body)
			if callbackName == "" {
				callbackName = "the collection detail callback"
			}
			return fmt.Errorf("task file %s left %s as a placeholder callback even though current topology does not include a detail surface", relPath, callbackName)
		}
	case normalizedPath == "test/widget_test.dart":
		if builderRuntimeWidgetTestPumpsRemovedRepositoryParameter(run.WorkspacePath, body) {
			return fmt.Errorf("task file %s pumps an app widget with repository even though lib/main.dart does not declare that named parameter", relPath)
		}
		if builderRuntimeWidgetTestOmitsRequiredRepositoryParameter(run.WorkspacePath, body) {
			return fmt.Errorf("task file %s pumps an app widget without repository even though lib/main.dart requires that named parameter", relPath)
		}
		if !hasOverviewSurface && builderRuntimeWidgetTestLooksLikeOverviewFirstFlow(body) {
			return fmt.Errorf("task file %s still asserts overview-only copy even though current topology does not include an overview/home flow", relPath)
		}
	case isMutationControllerPath:
		if !hasOverviewSurface && hasMutationSurface && strings.Contains(body, "HomeController") {
			return fmt.Errorf("task file %s still depends on HomeController even though current topology does not include an overview/home controller", relPath)
		}
	case isCollectionControllerPath || isCollectionViewPath:
		if !hasFilterFlow && builderRuntimeTaskOutputLooksLikeUnexpectedListFilter(body) {
			return fmt.Errorf("task file %s introduced list filtering even though current topology does not include ac-filter", relPath)
		}
		if isCollectionViewPath && !hasOverviewSurface && hasCollectionSurface && hasMutationSurface && !builderRuntimeTaskOutputWiresCollectionCreateEntry(body) {
			return fmt.Errorf("task file %s did not expose a create entry on the collection root even though current topology has mutation without overview/home", relPath)
		}
	}
	if !hasDetailSurface && builderRuntimeTaskOutputPathCanReferenceRecordDetail(normalizedPath) {
		if detailTypes := builderRuntimeTaskOutputReferencedDetailTypes(body); len(detailTypes) > 0 {
			return fmt.Errorf("task file %s referenced standalone detail page types %s even though current topology does not include a detail surface", relPath, strings.Join(detailTypes, ", "))
		}
	}
	if !hasDeleteFlow && builderRuntimeTaskOutputPathCanReferenceDeleteFlow(normalizedPath) && strings.Contains(body, "deleteRecord") {
		return fmt.Errorf("task file %s referenced deleteRecord even though current topology does not include delete behavior", relPath)
	}
	return nil
}

func builderRuntimeTaskOutputHasNullReturningCollectionCallback(content string) bool {
	if strings.TrimSpace(content) == "" {
		return false
	}
	return builderRuntimeMainNullReturningDetailCallbackPattern.FindStringIndex(content) != nil
}

func builderRuntimeNullReturningCollectionCallbackName(content string) string {
	if strings.TrimSpace(content) == "" {
		return ""
	}
	match := builderRuntimeMainNullReturningDetailCallbackNamePattern.FindStringSubmatch(content)
	if len(match) < 2 {
		return ""
	}
	return strings.TrimSpace(match[1])
}

func builderRuntimeWidgetTestLooksLikeOverviewFirstFlow(content string) bool {
	for _, marker := range []string{"homeSummaryTitle", "recentRecordsTitle", "今日摘要", "最近待办事项"} {
		if strings.Contains(content, marker) {
			return true
		}
	}
	return false
}

func builderRuntimeWidgetTestPumpsRemovedRepositoryParameter(workspacePath, widgetTestContent string) bool {
	if strings.TrimSpace(workspacePath) == "" || strings.TrimSpace(widgetTestContent) == "" {
		return false
	}
	mainContent, err := builderRuntimeSemanticFileContent(workspacePath, nil, "lib/main.dart")
	if err != nil || strings.TrimSpace(mainContent) == "" {
		return false
	}
	className := builderRuntimeRunAppWidgetClassName(mainContent)
	if className == "" {
		return false
	}
	args, ok := builderRuntimeCallConstructsClassArgs(widgetTestContent, "pumpWidget", className)
	if !ok || !strings.Contains(args, "repository:") {
		return false
	}
	return !builderRuntimeDartConstructorAcceptsNamedParameter(mainContent, className, "repository")
}

func builderRuntimeWidgetTestOmitsRequiredRepositoryParameter(workspacePath, widgetTestContent string) bool {
	if strings.TrimSpace(workspacePath) == "" || strings.TrimSpace(widgetTestContent) == "" {
		return false
	}
	mainContent, err := builderRuntimeSemanticFileContent(workspacePath, nil, "lib/main.dart")
	if err != nil || strings.TrimSpace(mainContent) == "" {
		return false
	}
	className := builderRuntimeRunAppWidgetClassName(mainContent)
	if className == "" || !builderRuntimeDartConstructorAcceptsNamedParameter(mainContent, className, "repository") {
		return false
	}
	args, ok := builderRuntimeCallConstructsClassArgs(widgetTestContent, "pumpWidget", className)
	if !ok {
		return false
	}
	return !strings.Contains(args, "repository:")
}

func builderRuntimeMainConstructsConcreteRepositoryInline(workspacePath, mainContent string) bool {
	if strings.TrimSpace(workspacePath) == "" || strings.TrimSpace(mainContent) == "" {
		return false
	}
	concreteType := builderRuntimeOpenLiteConcreteRecordRepositoryTypeName(workspacePath)
	if concreteType == "" {
		concreteType = "HiveRecordRepository"
	}
	pattern := regexp.MustCompile(`\b(?:repository|recordRepository)\s*:\s*` + regexp.QuoteMeta(concreteType) + `\s*\(\s*\)`)
	return pattern.FindStringIndex(mainContent) != nil
}

func builderRuntimeMainOmitsRequiredRepositoryParameter(mainContent string) bool {
	className := builderRuntimeRunAppWidgetClassName(mainContent)
	if className == "" || !builderRuntimeDartConstructorAcceptsNamedParameter(mainContent, className, "repository") {
		return false
	}
	args, ok := builderRuntimeCallConstructsClassArgs(mainContent, "runApp", className)
	if !ok {
		return false
	}
	return !strings.Contains(args, "repository:")
}

func builderRuntimeMainViewConstructorSignatureError(workspacePath, mainContent string) error {
	if strings.TrimSpace(workspacePath) == "" || strings.TrimSpace(mainContent) == "" {
		return nil
	}
	candidates, err := builderRuntimeMainViewConstructorCandidates(workspacePath, mainContent)
	if err != nil {
		return err
	}
	for _, candidate := range candidates {
		viewContent, err := builderRuntimeSemanticFileContent(workspacePath, nil, candidate.path)
		if err != nil || strings.TrimSpace(viewContent) == "" {
			continue
		}
		accepted, required := builderRuntimeDartConstructorNamedParameters(viewContent, candidate.className)
		if len(accepted) == 0 && len(required) == 0 {
			continue
		}
		for _, args := range builderRuntimeConstructedClassArgs(mainContent, candidate.className) {
			provided := builderRuntimeTopLevelNamedArgumentSet(args)
			invalid := make([]string, 0, len(provided))
			for name := range provided {
				if _, ok := accepted[name]; !ok {
					invalid = append(invalid, name)
				}
			}
			if len(invalid) > 0 {
				sort.Strings(invalid)
				return fmt.Errorf("task file lib/main.dart passes unsupported named parameters %s to %s; derive app-entry wiring from %s", strings.Join(invalid, ", "), candidate.className, candidate.path)
			}
			missing := make([]string, 0, len(required))
			for name := range required {
				if _, ok := provided[name]; !ok {
					missing = append(missing, name)
				}
			}
			if len(missing) > 0 {
				sort.Strings(missing)
				return fmt.Errorf("task file lib/main.dart omits required named parameters %s when instantiating %s from %s", strings.Join(missing, ", "), candidate.className, candidate.path)
			}
		}
	}
	return nil
}

type builderRuntimeViewConstructorCandidate struct {
	path      string
	className string
}

type builderRuntimeRepositoryCandidate struct {
	path           string
	repositoryType string
	concreteType   string
	inMemoryType   string
}

type builderRuntimeMutationViewCandidate struct {
	path            string
	className       string
	controllerParam string
	controllerType  string
	repositoryParam string
	initialParam    string
}

type builderRuntimeMutationControllerCandidate struct {
	path            string
	className       string
	repositoryParam string
}

type builderRuntimeOverviewControllerCandidate struct {
	path            string
	className       string
	repositoryParam string
}

type builderRuntimeOverviewViewCandidate struct {
	path                string
	className           string
	controllerParam     string
	controllerType      string
	createCallbackName  string
	viewAllCallbackName string
}

type builderRuntimeOverviewSurfaceCandidate struct {
	view       builderRuntimeOverviewViewCandidate
	controller builderRuntimeOverviewControllerCandidate
	repository builderRuntimeRepositoryCandidate
}

type builderRuntimeDetailSurfaceCandidate struct {
	view       builderRuntimeDetailViewCandidate
	mutation   builderRuntimeMutationSurfaceCandidate
	controller builderRuntimeCollectionControllerCandidate
}

type builderRuntimeMutationSurfaceCandidate struct {
	view       builderRuntimeMutationViewCandidate
	controller builderRuntimeMutationControllerCandidate
	repository builderRuntimeRepositoryCandidate
}

type builderRuntimeCollectionControllerCandidate struct {
	path            string
	className       string
	repositoryParam string
	supportsInit    bool
	supportsRefresh bool
	supportsUpdate  bool
}

type builderRuntimeCollectionViewCandidate struct {
	path               string
	className          string
	controllerParam    string
	controllerType     string
	detailCallbackName string
	detailCallbackType string
	createCallbackName string
}

type builderRuntimeCollectionSurfaceCandidate struct {
	controller builderRuntimeCollectionControllerCandidate
	view       builderRuntimeCollectionViewCandidate
	repository builderRuntimeRepositoryCandidate
}

type builderRuntimeDetailViewCandidate struct {
	path               string
	className          string
	recordParam        string
	editCallbackName   string
	deleteCallbackName string
	hasOnEdit          bool
	hasOnDelete        bool
	requiredParams     []string
}

func builderRuntimeIdentifierPascalCase(identifier string) string {
	trimmedIdentifier := strings.TrimSpace(identifier)
	if trimmedIdentifier == "" {
		return ""
	}
	snake := builderRuntimeOpenLiteIdentifierSnakeCase(trimmedIdentifier)
	if snake == "" {
		return ""
	}
	parts := strings.Split(snake, "_")
	var builder strings.Builder
	for _, part := range parts {
		if part == "" {
			continue
		}
		builder.WriteString(strings.ToUpper(part[:1]))
		builder.WriteString(part[1:])
	}
	return builder.String()
}

func builderRuntimeRepositoryCandidates(workspacePath string) ([]builderRuntimeRepositoryCandidate, error) {
	if strings.TrimSpace(workspacePath) == "" {
		return nil, nil
	}
	repositoryDir := filepath.Join(workspacePath, "lib", "repositories")
	entries, err := os.ReadDir(repositoryDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	candidates := make([]builderRuntimeRepositoryCandidate, 0)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".dart") {
			continue
		}
		relPath := filepath.ToSlash(filepath.Join("lib", "repositories", entry.Name()))
		content, readErr := os.ReadFile(filepath.Join(repositoryDir, entry.Name()))
		if readErr != nil {
			return nil, readErr
		}
		matches := builderRuntimeExportedDartTypePattern.FindAllStringSubmatch(string(content), -1)
		repositoryType := ""
		for _, match := range matches {
			if len(match) < 2 {
				continue
			}
			candidateType := strings.TrimSpace(match[1])
			if strings.HasSuffix(candidateType, "Repository") {
				repositoryType = candidateType
				break
			}
		}
		if repositoryType == "" {
			continue
		}
		implementationPattern := regexp.MustCompile(`(?m)^\s*class\s+([A-Z][A-Za-z0-9_]*)\s+(?:extends|implements)\s+` + regexp.QuoteMeta(repositoryType) + `\b`)
		implementationMatches := implementationPattern.FindAllStringSubmatch(string(content), -1)
		candidate := builderRuntimeRepositoryCandidate{path: relPath, repositoryType: repositoryType}
		for _, match := range implementationMatches {
			if len(match) < 2 {
				continue
			}
			typeName := strings.TrimSpace(match[1])
			if typeName == "" {
				continue
			}
			if strings.HasPrefix(typeName, "InMemory") {
				if candidate.inMemoryType == "" {
					candidate.inMemoryType = typeName
				}
				continue
			}
			if candidate.concreteType == "" {
				candidate.concreteType = typeName
			}
		}
		if candidate.concreteType == "" {
			candidate.concreteType = candidate.inMemoryType
		}
		candidates = append(candidates, candidate)
	}
	sort.Slice(candidates, func(left, right int) bool {
		if candidates[left].path == candidates[right].path {
			return candidates[left].repositoryType < candidates[right].repositoryType
		}
		return candidates[left].path < candidates[right].path
	})
	return candidates, nil
}

func builderRuntimePrimaryRepositoryCandidate(workspacePath string, hints ...string) builderRuntimeRepositoryCandidate {
	candidates, err := builderRuntimeRepositoryCandidates(workspacePath)
	if err != nil || len(candidates) == 0 {
		return builderRuntimeRepositoryCandidate{}
	}
	for _, candidate := range candidates {
		if builderRuntimeRepositoryCandidateMatchesHints(candidate, hints...) {
			return candidate
		}
	}
	for _, candidate := range candidates {
		if candidate.path == "lib/repositories/record_repository.dart" || candidate.repositoryType == "RecordRepository" {
			return candidate
		}
	}
	if len(candidates) == 1 {
		return candidates[0]
	}
	return candidates[0]
}

func builderRuntimeRepositoryCandidateMatchesHints(candidate builderRuntimeRepositoryCandidate, hints ...string) bool {
	matchTokens := []string{
		strings.TrimSpace(candidate.path),
		strings.TrimPrefix(strings.TrimSpace(candidate.path), "lib/"),
		strings.TrimSpace(candidate.repositoryType),
		strings.TrimSpace(candidate.concreteType),
		strings.TrimSpace(candidate.inMemoryType),
	}
	for _, hint := range hints {
		trimmedHint := strings.TrimSpace(hint)
		if trimmedHint == "" {
			continue
		}
		for _, token := range matchTokens {
			if token != "" && strings.Contains(trimmedHint, token) {
				return true
			}
		}
	}
	return false
}

func builderRuntimeOverviewViewCandidates(workspacePath string) ([]builderRuntimeOverviewViewCandidate, error) {
	if strings.TrimSpace(workspacePath) == "" {
		return nil, nil
	}
	viewDir := filepath.Join(workspacePath, "lib", "views")
	entries, err := os.ReadDir(viewDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	seen := make(map[string]struct{})
	candidates := make([]builderRuntimeOverviewViewCandidate, 0)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".dart") {
			continue
		}
		relPath := filepath.ToSlash(filepath.Join("lib", "views", entry.Name()))
		contentBytes, readErr := os.ReadFile(filepath.Join(viewDir, entry.Name()))
		if readErr != nil {
			return nil, readErr
		}
		content := string(contentBytes)
		matches := builderRuntimeNamedDartTypeDeclarationPattern.FindAllStringSubmatch(content, -1)
		for _, match := range matches {
			if len(match) < 3 || strings.TrimSpace(match[1]) != "class" {
				continue
			}
			className := strings.TrimSpace(match[2])
			if className == "" {
				continue
			}
			accepted, _ := builderRuntimeDartConstructorNamedParameters(content, className)
			controllerParam := builderRuntimeOverviewViewControllerParam(accepted)
			createCallbackName := builderRuntimeOverviewViewCreateCallbackName(accepted)
			viewAllCallbackName := builderRuntimeOverviewViewViewAllCallbackName(accepted)
			lowerPath := strings.ToLower(relPath)
			lowerClass := strings.ToLower(className)
			if relPath != "lib/views/home_page.dart" && className != "HomePage" && !strings.Contains(lowerPath, "home") && !strings.Contains(lowerPath, "overview") && !strings.Contains(lowerClass, "home") && !strings.Contains(lowerClass, "overview") && viewAllCallbackName == "" {
				continue
			}
			if controllerParam == "" {
				continue
			}
			key := relPath + "::" + className
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			candidates = append(candidates, builderRuntimeOverviewViewCandidate{
				path:                relPath,
				className:           className,
				controllerParam:     controllerParam,
				controllerType:      builderRuntimeDartFieldTypeByName(content, controllerParam),
				createCallbackName:  createCallbackName,
				viewAllCallbackName: viewAllCallbackName,
			})
		}
	}
	sort.Slice(candidates, func(left, right int) bool {
		leftRank := builderRuntimeOverviewViewCandidateRank(candidates[left])
		rightRank := builderRuntimeOverviewViewCandidateRank(candidates[right])
		if leftRank != rightRank {
			return leftRank > rightRank
		}
		if candidates[left].path == candidates[right].path {
			return candidates[left].className < candidates[right].className
		}
		return candidates[left].path < candidates[right].path
	})
	return candidates, nil
}

func builderRuntimePrimaryOverviewViewCandidate(workspacePath string, hints ...string) builderRuntimeOverviewViewCandidate {
	candidates, err := builderRuntimeOverviewViewCandidates(workspacePath)
	if err != nil || len(candidates) == 0 {
		return builderRuntimeOverviewViewCandidate{}
	}
	for _, candidate := range candidates {
		if builderRuntimeOverviewViewCandidateMatchesHints(candidate, hints...) {
			return candidate
		}
	}
	return candidates[0]
}

func builderRuntimeOverviewControllerCandidates(workspacePath string) ([]builderRuntimeOverviewControllerCandidate, error) {
	if strings.TrimSpace(workspacePath) == "" {
		return nil, nil
	}
	controllerDir := filepath.Join(workspacePath, "lib", "controllers")
	entries, err := os.ReadDir(controllerDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	seen := make(map[string]struct{})
	candidates := make([]builderRuntimeOverviewControllerCandidate, 0)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".dart") {
			continue
		}
		relPath := filepath.ToSlash(filepath.Join("lib", "controllers", entry.Name()))
		contentBytes, readErr := os.ReadFile(filepath.Join(controllerDir, entry.Name()))
		if readErr != nil {
			return nil, readErr
		}
		content := string(contentBytes)
		matches := builderRuntimeNamedDartTypeDeclarationPattern.FindAllStringSubmatch(content, -1)
		for _, match := range matches {
			if len(match) < 3 || strings.TrimSpace(match[1]) != "class" {
				continue
			}
			className := strings.TrimSpace(match[2])
			if className == "" {
				continue
			}
			accepted, _ := builderRuntimeDartConstructorNamedParameters(content, className)
			repositoryParam := builderRuntimeOverviewControllerRepositoryParam(accepted)
			if repositoryParam == "" && relPath == "lib/controllers/home_controller.dart" && className == "HomeController" {
				repositoryParam = "repository"
			}
			if repositoryParam == "" {
				continue
			}
			lowerPath := strings.ToLower(relPath)
			lowerClass := strings.ToLower(className)
			if relPath != "lib/controllers/home_controller.dart" && className != "HomeController" && !strings.Contains(lowerPath, "home") && !strings.Contains(lowerPath, "overview") && !strings.Contains(lowerClass, "home") && !strings.Contains(lowerClass, "overview") && !builderRuntimeOverviewControllerHasOverviewViewSignal(workspacePath, relPath, className) {
				continue
			}
			key := relPath + "::" + className
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			candidates = append(candidates, builderRuntimeOverviewControllerCandidate{
				path:            relPath,
				className:       className,
				repositoryParam: repositoryParam,
			})
		}
	}
	sort.Slice(candidates, func(left, right int) bool {
		leftRank := builderRuntimeOverviewControllerCandidateRank(candidates[left])
		rightRank := builderRuntimeOverviewControllerCandidateRank(candidates[right])
		if leftRank != rightRank {
			return leftRank > rightRank
		}
		if candidates[left].path == candidates[right].path {
			return candidates[left].className < candidates[right].className
		}
		return candidates[left].path < candidates[right].path
	})
	return candidates, nil
}

func builderRuntimeOverviewControllerHasOverviewViewSignal(workspacePath, controllerPath, controllerClass string) bool {
	trimmedWorkspace := strings.TrimSpace(workspacePath)
	trimmedControllerPath := filepath.ToSlash(strings.TrimSpace(controllerPath))
	trimmedControllerClass := strings.TrimSpace(controllerClass)
	if trimmedWorkspace == "" || trimmedControllerPath == "" || trimmedControllerClass == "" {
		return false
	}
	if !strings.HasPrefix(trimmedControllerPath, "lib/controllers/") || !strings.HasSuffix(trimmedControllerPath, "_controller.dart") {
		return false
	}
	baseName := strings.TrimSuffix(filepath.Base(trimmedControllerPath), "_controller.dart")
	if baseName == "" {
		return false
	}
	for _, suffix := range []string{"_page.dart", "_view.dart"} {
		viewRelPath := filepath.ToSlash(filepath.Join("lib", "views", baseName+suffix))
		viewContent, err := os.ReadFile(filepath.Join(trimmedWorkspace, filepath.FromSlash(viewRelPath)))
		if err != nil {
			continue
		}
		content := string(viewContent)
		for _, match := range builderRuntimeNamedDartTypeDeclarationPattern.FindAllStringSubmatch(content, -1) {
			if len(match) < 3 || strings.TrimSpace(match[1]) != "class" {
				continue
			}
			className := strings.TrimSpace(match[2])
			if className == "" {
				continue
			}
			accepted, _ := builderRuntimeDartConstructorNamedParameters(content, className)
			controllerParam := builderRuntimeOverviewViewControllerParam(accepted)
			if controllerParam == "" || builderRuntimeOverviewViewViewAllCallbackName(accepted) == "" {
				continue
			}
			if builderRuntimeDartFieldTypeByName(content, controllerParam) == trimmedControllerClass {
				return true
			}
		}
	}
	return false
}

func builderRuntimePrimaryOverviewControllerCandidate(workspacePath string, hints ...string) builderRuntimeOverviewControllerCandidate {
	candidates, err := builderRuntimeOverviewControllerCandidates(workspacePath)
	if err != nil || len(candidates) == 0 {
		return builderRuntimeOverviewControllerCandidate{}
	}
	for _, hint := range hints {
		trimmedHint := strings.TrimSpace(hint)
		if trimmedHint == "" {
			continue
		}
		for _, candidate := range candidates {
			if builderRuntimeOverviewControllerCandidateMatchesHints(candidate, trimmedHint) {
				return candidate
			}
		}
	}
	return candidates[0]
}

func builderRuntimePrimaryOverviewSurfaceCandidate(workspacePath string, hints ...string) builderRuntimeOverviewSurfaceCandidate {
	view := builderRuntimePrimaryOverviewViewCandidate(workspacePath, hints...)
	controllerHints := []string{
		view.path,
		view.className,
		view.controllerParam,
		view.controllerType,
		view.createCallbackName,
		view.viewAllCallbackName,
		builderRuntimeControllerPathHintForViewPath(view.path),
	}
	controllerHints = append(controllerHints, hints...)
	controller := builderRuntimePrimaryOverviewControllerCandidate(workspacePath, controllerHints...)
	repositoryHints := append([]string{}, hints...)
	repositoryHints = append(repositoryHints,
		view.path,
		view.className,
		view.controllerParam,
		view.controllerType,
		controller.path,
		controller.className,
		controller.repositoryParam,
	)
	repository := builderRuntimePrimaryRepositoryCandidate(workspacePath, repositoryHints...)
	return builderRuntimeOverviewSurfaceCandidate{
		view:       view,
		controller: controller,
		repository: repository,
	}
}

func builderRuntimeMutationViewCandidates(workspacePath string) ([]builderRuntimeMutationViewCandidate, error) {
	if strings.TrimSpace(workspacePath) == "" {
		return nil, nil
	}
	viewDir := filepath.Join(workspacePath, "lib", "views")
	entries, err := os.ReadDir(viewDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	seen := make(map[string]struct{})
	candidates := make([]builderRuntimeMutationViewCandidate, 0)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".dart") {
			continue
		}
		relPath := filepath.ToSlash(filepath.Join("lib", "views", entry.Name()))
		content, readErr := os.ReadFile(filepath.Join(viewDir, entry.Name()))
		if readErr != nil {
			return nil, readErr
		}
		matches := builderRuntimeNamedDartTypeDeclarationPattern.FindAllStringSubmatch(string(content), -1)
		for _, match := range matches {
			if len(match) < 3 || strings.TrimSpace(match[1]) != "class" {
				continue
			}
			className := strings.TrimSpace(match[2])
			if className == "" {
				continue
			}
			accepted, _ := builderRuntimeDartConstructorNamedParameters(string(content), className)
			controllerParam := builderRuntimeMutationViewControllerParam(accepted)
			repositoryParam := builderRuntimeMutationViewRepositoryParam(accepted)
			initialParam := builderRuntimeMutationViewInitialParam(accepted)
			controllerType := builderRuntimeDartFieldTypeByName(string(content), controllerParam)
			if repositoryParam == "" && relPath == "lib/views/record_form_page.dart" && className == "RecordFormPage" {
				repositoryParam = "recordRepository"
			}
			if initialParam == "" && relPath == "lib/views/record_form_page.dart" && className == "RecordFormPage" {
				initialParam = "initialRecord"
			}
			if controllerParam == "" && repositoryParam == "" {
				continue
			}
			key := relPath + "::" + className
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			candidates = append(candidates, builderRuntimeMutationViewCandidate{
				path:            relPath,
				className:       className,
				controllerParam: controllerParam,
				controllerType:  controllerType,
				repositoryParam: repositoryParam,
				initialParam:    initialParam,
			})
		}
	}
	sort.Slice(candidates, func(left, right int) bool {
		leftRank := builderRuntimeMutationViewCandidateRank(candidates[left])
		rightRank := builderRuntimeMutationViewCandidateRank(candidates[right])
		if leftRank != rightRank {
			return leftRank > rightRank
		}
		if candidates[left].path == candidates[right].path {
			return candidates[left].className < candidates[right].className
		}
		return candidates[left].path < candidates[right].path
	})
	return candidates, nil
}

func builderRuntimePrimaryMutationViewCandidate(workspacePath string, hints ...string) builderRuntimeMutationViewCandidate {
	candidates, err := builderRuntimeMutationViewCandidates(workspacePath)
	if err != nil || len(candidates) == 0 {
		return builderRuntimeMutationViewCandidate{}
	}
	for _, hint := range hints {
		trimmedHint := strings.TrimSpace(hint)
		if trimmedHint == "" {
			continue
		}
		for _, candidate := range candidates {
			if builderRuntimeMutationViewCandidateMatchesHints(candidate, trimmedHint) {
				return candidate
			}
		}
	}
	return candidates[0]
}

func builderRuntimeMutationControllerCandidates(workspacePath string) ([]builderRuntimeMutationControllerCandidate, error) {
	if strings.TrimSpace(workspacePath) == "" {
		return nil, nil
	}
	controllerDir := filepath.Join(workspacePath, "lib", "controllers")
	entries, err := os.ReadDir(controllerDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	seen := make(map[string]struct{})
	candidates := make([]builderRuntimeMutationControllerCandidate, 0)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".dart") {
			continue
		}
		relPath := filepath.ToSlash(filepath.Join("lib", "controllers", entry.Name()))
		contentBytes, readErr := os.ReadFile(filepath.Join(controllerDir, entry.Name()))
		if readErr != nil {
			return nil, readErr
		}
		content := string(contentBytes)
		matches := builderRuntimeNamedDartTypeDeclarationPattern.FindAllStringSubmatch(content, -1)
		for _, match := range matches {
			if len(match) < 3 || strings.TrimSpace(match[1]) != "class" {
				continue
			}
			className := strings.TrimSpace(match[2])
			if className == "" {
				continue
			}
			accepted, _ := builderRuntimeDartConstructorNamedParameters(content, className)
			repositoryParam := builderRuntimeMutationViewRepositoryParam(accepted)
			if repositoryParam == "" && relPath == "lib/controllers/record_form_controller.dart" && className == "RecordFormController" {
				repositoryParam = "repository"
			}
			if repositoryParam == "" {
				continue
			}
			lowerPath := strings.ToLower(relPath)
			lowerClass := strings.ToLower(className)
			if relPath != "lib/controllers/record_form_controller.dart" && !strings.Contains(lowerPath, "form") && !strings.Contains(lowerPath, "edit") && !strings.Contains(lowerPath, "mutation") && !strings.Contains(lowerClass, "form") && !strings.Contains(lowerClass, "edit") && !strings.Contains(lowerClass, "mutation") {
				continue
			}
			key := relPath + "::" + className
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			candidates = append(candidates, builderRuntimeMutationControllerCandidate{
				path:            relPath,
				className:       className,
				repositoryParam: repositoryParam,
			})
		}
	}
	sort.Slice(candidates, func(left, right int) bool {
		leftRank := builderRuntimeMutationControllerCandidateRank(candidates[left])
		rightRank := builderRuntimeMutationControllerCandidateRank(candidates[right])
		if leftRank != rightRank {
			return leftRank > rightRank
		}
		if candidates[left].path == candidates[right].path {
			return candidates[left].className < candidates[right].className
		}
		return candidates[left].path < candidates[right].path
	})
	return candidates, nil
}

func builderRuntimePrimaryMutationControllerCandidate(workspacePath string, hints ...string) builderRuntimeMutationControllerCandidate {
	candidates, err := builderRuntimeMutationControllerCandidates(workspacePath)
	if err != nil || len(candidates) == 0 {
		return builderRuntimeMutationControllerCandidate{}
	}
	for _, hint := range hints {
		trimmedHint := strings.TrimSpace(hint)
		if trimmedHint == "" {
			continue
		}
		for _, candidate := range candidates {
			if builderRuntimeMutationControllerCandidateMatchesHints(candidate, trimmedHint) {
				return candidate
			}
		}
	}
	return candidates[0]
}

func builderRuntimePrimaryMutationSurfaceCandidate(workspacePath string, hints ...string) builderRuntimeMutationSurfaceCandidate {
	view := builderRuntimePrimaryMutationViewCandidate(workspacePath, hints...)
	controllerHints := []string{
		view.path,
		view.className,
		view.controllerParam,
		view.controllerType,
		view.repositoryParam,
		view.initialParam,
		builderRuntimeControllerPathHintForViewPath(view.path),
	}
	controllerHints = append(controllerHints, hints...)
	controller := builderRuntimePrimaryMutationControllerCandidate(workspacePath, controllerHints...)
	repositoryHints := append([]string{}, hints...)
	repositoryHints = append(repositoryHints,
		view.path,
		view.className,
		view.controllerParam,
		view.controllerType,
		view.repositoryParam,
		view.initialParam,
		controller.path,
		controller.className,
		controller.repositoryParam,
	)
	repository := builderRuntimePrimaryRepositoryCandidate(workspacePath, repositoryHints...)
	return builderRuntimeMutationSurfaceCandidate{
		view:       view,
		controller: controller,
		repository: repository,
	}
}

func builderRuntimeMutationViewCandidateRank(candidate builderRuntimeMutationViewCandidate) int {
	rank := 0
	lowerPath := strings.ToLower(candidate.path)
	lowerClass := strings.ToLower(candidate.className)
	if candidate.path == "lib/views/record_form_page.dart" || candidate.className == "RecordFormPage" {
		rank += 100
	}
	if strings.Contains(lowerPath, "form") || strings.Contains(lowerClass, "form") {
		rank += 40
	}
	if strings.Contains(lowerPath, "edit") || strings.Contains(lowerClass, "edit") {
		rank += 30
	}
	if candidate.initialParam != "" {
		rank += 20
	}
	if candidate.controllerParam != "" {
		rank += 15
	}
	if candidate.repositoryParam != "" {
		rank += 10
	}
	return rank
}

func builderRuntimeMutationViewCandidateMatchesHints(candidate builderRuntimeMutationViewCandidate, hints ...string) bool {
	matchTokens := []string{
		strings.TrimSpace(candidate.path),
		strings.TrimPrefix(strings.TrimSpace(candidate.path), "lib/"),
		strings.TrimSpace(candidate.className),
		strings.TrimSpace(candidate.controllerParam),
		strings.TrimSpace(candidate.controllerType),
		strings.TrimSpace(candidate.repositoryParam),
		strings.TrimSpace(candidate.initialParam),
	}
	for _, hint := range hints {
		trimmedHint := strings.TrimSpace(hint)
		if trimmedHint == "" {
			continue
		}
		for _, token := range matchTokens {
			if token != "" && strings.Contains(trimmedHint, token) {
				return true
			}
		}
	}
	return false
}

func builderRuntimeMutationControllerCandidateRank(candidate builderRuntimeMutationControllerCandidate) int {
	rank := 0
	lowerPath := strings.ToLower(candidate.path)
	lowerClass := strings.ToLower(candidate.className)
	if candidate.path == "lib/controllers/record_form_controller.dart" || candidate.className == "RecordFormController" {
		rank += 100
	}
	if strings.Contains(lowerPath, "form") || strings.Contains(lowerClass, "form") {
		rank += 40
	}
	if strings.Contains(lowerPath, "edit") || strings.Contains(lowerClass, "edit") {
		rank += 30
	}
	if candidate.repositoryParam != "" {
		rank += 10
	}
	return rank
}

func builderRuntimeMutationControllerCandidateMatchesHints(candidate builderRuntimeMutationControllerCandidate, hints ...string) bool {
	matchTokens := []string{
		strings.TrimSpace(candidate.path),
		strings.TrimPrefix(strings.TrimSpace(candidate.path), "lib/"),
		strings.TrimSpace(candidate.className),
		strings.TrimSpace(candidate.repositoryParam),
	}
	for _, hint := range hints {
		trimmedHint := strings.TrimSpace(hint)
		if trimmedHint == "" {
			continue
		}
		for _, token := range matchTokens {
			if token != "" && strings.Contains(trimmedHint, token) {
				return true
			}
		}
	}
	return false
}

func builderRuntimeMutationViewRepositoryParam(accepted map[string]struct{}) string {
	for _, name := range []string{"recordRepository", "repository"} {
		if _, ok := accepted[name]; ok {
			return name
		}
	}
	for name := range accepted {
		lower := strings.ToLower(strings.TrimSpace(name))
		if strings.Contains(lower, "repository") {
			return name
		}
	}
	return ""
}

func builderRuntimeMutationViewControllerParam(accepted map[string]struct{}) string {
	if _, ok := accepted["controller"]; ok {
		return "controller"
	}
	for name := range accepted {
		if strings.Contains(strings.ToLower(strings.TrimSpace(name)), "controller") {
			return name
		}
	}
	return ""
}

func builderRuntimeMutationViewInitialParam(accepted map[string]struct{}) string {
	if _, ok := accepted["initialRecord"]; ok {
		return "initialRecord"
	}
	for name := range accepted {
		if strings.HasPrefix(strings.TrimSpace(name), "initial") {
			return name
		}
	}
	return ""
}

func builderRuntimeOverviewViewControllerParam(accepted map[string]struct{}) string {
	if _, ok := accepted["controller"]; ok {
		return "controller"
	}
	for name := range accepted {
		if strings.Contains(strings.ToLower(strings.TrimSpace(name)), "controller") {
			return name
		}
	}
	return ""
}

func builderRuntimeOverviewViewCreateCallbackName(accepted map[string]struct{}) string {
	for _, name := range []string{"onCreateRecord", "onCreateTask"} {
		if _, ok := accepted[name]; ok {
			return name
		}
	}
	for name := range accepted {
		if strings.HasPrefix(strings.TrimSpace(name), "onCreate") {
			return name
		}
	}
	return ""
}

func builderRuntimeOverviewViewViewAllCallbackName(accepted map[string]struct{}) string {
	for _, name := range []string{"onViewAllRecords", "onViewAllTasks"} {
		if _, ok := accepted[name]; ok {
			return name
		}
	}
	for name := range accepted {
		trimmedName := strings.TrimSpace(name)
		if strings.HasPrefix(trimmedName, "onViewAll") || strings.HasPrefix(trimmedName, "onBrowse") {
			return name
		}
	}
	return ""
}

func builderRuntimeOverviewControllerRepositoryParam(accepted map[string]struct{}) string {
	for _, name := range []string{"repository", "recordRepository", "taskRepository", "dashboardRepository"} {
		if _, ok := accepted[name]; ok {
			return name
		}
	}
	for name := range accepted {
		if strings.Contains(strings.ToLower(strings.TrimSpace(name)), "repository") {
			return name
		}
	}
	return ""
}

func builderRuntimeDartFieldTypeByName(content, fieldName string) string {
	trimmedFieldName := strings.TrimSpace(fieldName)
	if strings.TrimSpace(content) == "" || trimmedFieldName == "" {
		return ""
	}
	pattern := regexp.MustCompile(`(?m)^\s*(?:final|late\s+final)\s+(.+?)\s+` + regexp.QuoteMeta(trimmedFieldName) + `\s*;`)
	if match := pattern.FindStringSubmatch(content); len(match) >= 2 {
		return strings.TrimSpace(match[1])
	}
	inlinePattern := regexp.MustCompile(`(?:^|[;{]\s*)(?:final|late\s+final)\s+(.+?)\s+` + regexp.QuoteMeta(trimmedFieldName) + `\s*;`)
	if match := inlinePattern.FindStringSubmatch(content); len(match) >= 2 {
		return strings.TrimSpace(match[1])
	}
	return ""
}

func builderRuntimeCollectionControllerCandidates(workspacePath string) ([]builderRuntimeCollectionControllerCandidate, error) {
	if strings.TrimSpace(workspacePath) == "" {
		return nil, nil
	}
	controllerDir := filepath.Join(workspacePath, "lib", "controllers")
	entries, err := os.ReadDir(controllerDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	seen := make(map[string]struct{})
	candidates := make([]builderRuntimeCollectionControllerCandidate, 0)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".dart") {
			continue
		}
		relPath := filepath.ToSlash(filepath.Join("lib", "controllers", entry.Name()))
		contentBytes, readErr := os.ReadFile(filepath.Join(controllerDir, entry.Name()))
		if readErr != nil {
			return nil, readErr
		}
		content := string(contentBytes)
		matches := builderRuntimeNamedDartTypeDeclarationPattern.FindAllStringSubmatch(content, -1)
		for _, match := range matches {
			if len(match) < 3 || strings.TrimSpace(match[1]) != "class" {
				continue
			}
			className := strings.TrimSpace(match[2])
			if className == "" {
				continue
			}
			accepted, _ := builderRuntimeDartConstructorNamedParameters(content, className)
			repositoryParam := builderRuntimeMutationViewRepositoryParam(accepted)
			if repositoryParam == "" && relPath == "lib/controllers/record_list_controller.dart" && className == "RecordListController" {
				repositoryParam = "repository"
			}
			if repositoryParam == "" {
				continue
			}
			supportsInit := strings.Contains(content, "init()")
			supportsRefresh := strings.Contains(content, "refresh()")
			supportsUpdate := strings.Contains(content, "updateRecord(")
			lowerPath := strings.ToLower(relPath)
			lowerClass := strings.ToLower(className)
			if relPath != "lib/controllers/record_list_controller.dart" && !strings.Contains(lowerPath, "list") && !strings.Contains(lowerPath, "collection") && !strings.Contains(lowerClass, "list") && !strings.Contains(lowerClass, "collection") && !supportsInit && !supportsRefresh && !supportsUpdate {
				continue
			}
			key := relPath + "::" + className
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			candidates = append(candidates, builderRuntimeCollectionControllerCandidate{
				path:            relPath,
				className:       className,
				repositoryParam: repositoryParam,
				supportsInit:    supportsInit,
				supportsRefresh: supportsRefresh,
				supportsUpdate:  supportsUpdate,
			})
		}
	}
	sort.Slice(candidates, func(left, right int) bool {
		leftRank := builderRuntimeCollectionControllerCandidateRank(candidates[left])
		rightRank := builderRuntimeCollectionControllerCandidateRank(candidates[right])
		if leftRank != rightRank {
			return leftRank > rightRank
		}
		if candidates[left].path == candidates[right].path {
			return candidates[left].className < candidates[right].className
		}
		return candidates[left].path < candidates[right].path
	})
	return candidates, nil
}

func builderRuntimePrimaryCollectionControllerCandidate(workspacePath string, hints ...string) builderRuntimeCollectionControllerCandidate {
	candidates, err := builderRuntimeCollectionControllerCandidates(workspacePath)
	if err != nil || len(candidates) == 0 {
		return builderRuntimeCollectionControllerCandidate{}
	}
	for _, hint := range hints {
		trimmedHint := strings.TrimSpace(hint)
		if trimmedHint == "" {
			continue
		}
		for _, candidate := range candidates {
			if builderRuntimeCollectionControllerCandidateMatchesHints(candidate, trimmedHint) {
				return candidate
			}
		}
	}
	return candidates[0]
}

func builderRuntimeCollectionControllerCandidateRank(candidate builderRuntimeCollectionControllerCandidate) int {
	rank := 0
	lowerPath := strings.ToLower(candidate.path)
	lowerClass := strings.ToLower(candidate.className)
	if candidate.path == "lib/controllers/record_list_controller.dart" || candidate.className == "RecordListController" {
		rank += 100
	}
	if strings.Contains(lowerPath, "list") || strings.Contains(lowerClass, "list") {
		rank += 40
	}
	if strings.Contains(lowerPath, "collection") || strings.Contains(lowerClass, "collection") {
		rank += 35
	}
	if candidate.supportsRefresh {
		rank += 20
	}
	if candidate.supportsUpdate {
		rank += 10
	}
	return rank
}

func builderRuntimeCollectionControllerCandidateMatchesHints(candidate builderRuntimeCollectionControllerCandidate, hints ...string) bool {
	matchTokens := []string{
		strings.TrimSpace(candidate.path),
		strings.TrimPrefix(strings.TrimSpace(candidate.path), "lib/"),
		strings.TrimSpace(candidate.className),
		strings.TrimSpace(candidate.repositoryParam),
	}
	for _, hint := range hints {
		trimmedHint := strings.TrimSpace(hint)
		if trimmedHint == "" {
			continue
		}
		for _, token := range matchTokens {
			if token != "" && strings.Contains(trimmedHint, token) {
				return true
			}
		}
	}
	return false
}

func builderRuntimeCollectionViewCandidates(workspacePath string) ([]builderRuntimeCollectionViewCandidate, error) {
	if strings.TrimSpace(workspacePath) == "" {
		return nil, nil
	}
	viewDir := filepath.Join(workspacePath, "lib", "views")
	entries, err := os.ReadDir(viewDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	seen := make(map[string]struct{})
	candidates := make([]builderRuntimeCollectionViewCandidate, 0)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".dart") {
			continue
		}
		relPath := filepath.ToSlash(filepath.Join("lib", "views", entry.Name()))
		contentBytes, readErr := os.ReadFile(filepath.Join(viewDir, entry.Name()))
		if readErr != nil {
			return nil, readErr
		}
		content := string(contentBytes)
		matches := builderRuntimeNamedDartTypeDeclarationPattern.FindAllStringSubmatch(content, -1)
		for _, match := range matches {
			if len(match) < 3 || strings.TrimSpace(match[1]) != "class" {
				continue
			}
			className := strings.TrimSpace(match[2])
			if className == "" {
				continue
			}
			accepted, _ := builderRuntimeDartConstructorNamedParameters(content, className)
			controllerParam := builderRuntimeCollectionViewControllerParam(accepted)
			detailCallbackName := builderRuntimeCollectionViewDetailCallbackName(accepted)
			if controllerParam == "" && relPath == "lib/views/record_list_page.dart" && className == "RecordListPage" {
				controllerParam = "controller"
			}
			if detailCallbackName == "" && relPath == "lib/views/record_list_page.dart" && className == "RecordListPage" {
				detailCallbackName = "onOpenRecordDetail"
			}
			if controllerParam == "" || detailCallbackName == "" {
				continue
			}
			key := relPath + "::" + className
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			candidates = append(candidates, builderRuntimeCollectionViewCandidate{
				path:               relPath,
				className:          className,
				controllerParam:    controllerParam,
				controllerType:     builderRuntimeDartFieldTypeByName(content, controllerParam),
				detailCallbackName: detailCallbackName,
				detailCallbackType: builderRuntimeDartFieldTypeByName(content, detailCallbackName),
				createCallbackName: builderRuntimeCollectionViewCreateCallbackName(accepted),
			})
		}
	}
	sort.Slice(candidates, func(left, right int) bool {
		leftRank := builderRuntimeCollectionViewCandidateRank(candidates[left])
		rightRank := builderRuntimeCollectionViewCandidateRank(candidates[right])
		if leftRank != rightRank {
			return leftRank > rightRank
		}
		if candidates[left].path == candidates[right].path {
			return candidates[left].className < candidates[right].className
		}
		return candidates[left].path < candidates[right].path
	})
	return candidates, nil
}

func builderRuntimePrimaryCollectionViewCandidate(workspacePath string, hints ...string) builderRuntimeCollectionViewCandidate {
	candidates, err := builderRuntimeCollectionViewCandidates(workspacePath)
	if err != nil || len(candidates) == 0 {
		return builderRuntimeCollectionViewCandidate{}
	}
	for _, candidate := range candidates {
		if builderRuntimeCollectionViewCandidateMatchesHints(candidate, hints...) {
			return candidate
		}
	}
	return candidates[0]
}

func builderRuntimePrimaryCollectionSurfaceCandidate(workspacePath string, hints ...string) builderRuntimeCollectionSurfaceCandidate {
	view := builderRuntimePrimaryCollectionViewCandidate(workspacePath, hints...)
	controllerHints := []string{view.path, view.className, view.controllerParam, view.controllerType}
	controllerHints = append(controllerHints, hints...)
	controller := builderRuntimePrimaryCollectionControllerCandidate(workspacePath, controllerHints...)
	repositoryHints := append([]string{}, hints...)
	repositoryHints = append(repositoryHints,
		view.path,
		view.className,
		view.controllerParam,
		view.controllerType,
		controller.path,
		controller.className,
		controller.repositoryParam,
	)
	repository := builderRuntimePrimaryRepositoryCandidate(workspacePath, repositoryHints...)
	return builderRuntimeCollectionSurfaceCandidate{
		controller: controller,
		view:       view,
		repository: repository,
	}
}

func builderRuntimeCollectionViewCandidateRank(candidate builderRuntimeCollectionViewCandidate) int {
	rank := 0
	lowerPath := strings.ToLower(candidate.path)
	lowerClass := strings.ToLower(candidate.className)
	if candidate.path == "lib/views/record_list_page.dart" || candidate.className == "RecordListPage" {
		rank += 100
	}
	if strings.Contains(lowerPath, "list") || strings.Contains(lowerClass, "list") {
		rank += 40
	}
	if strings.Contains(lowerPath, "collection") || strings.Contains(lowerClass, "collection") {
		rank += 35
	}
	if candidate.detailCallbackName != "" {
		rank += 15
	}
	if candidate.createCallbackName != "" {
		rank += 10
	}
	return rank
}

func builderRuntimeCollectionViewCandidateMatchesHints(candidate builderRuntimeCollectionViewCandidate, hints ...string) bool {
	matchTokens := []string{
		strings.TrimSpace(candidate.path),
		strings.TrimPrefix(strings.TrimSpace(candidate.path), "lib/"),
		strings.TrimSpace(candidate.className),
		strings.TrimSpace(candidate.controllerParam),
		strings.TrimSpace(candidate.controllerType),
		strings.TrimSpace(candidate.detailCallbackName),
		strings.TrimSpace(candidate.detailCallbackType),
		strings.TrimSpace(candidate.createCallbackName),
	}
	for _, hint := range hints {
		trimmedHint := strings.TrimSpace(hint)
		if trimmedHint == "" {
			continue
		}
		for _, token := range matchTokens {
			if token != "" && strings.Contains(trimmedHint, token) {
				return true
			}
		}
	}
	return false
}

func builderRuntimeOverviewViewCandidateRank(candidate builderRuntimeOverviewViewCandidate) int {
	rank := 0
	lowerPath := strings.ToLower(candidate.path)
	lowerClass := strings.ToLower(candidate.className)
	if candidate.path == "lib/views/home_page.dart" || candidate.className == "HomePage" {
		rank += 100
	}
	if strings.Contains(lowerPath, "overview") || strings.Contains(lowerClass, "overview") {
		rank += 45
	}
	if strings.Contains(lowerPath, "home") || strings.Contains(lowerClass, "home") {
		rank += 40
	}
	if candidate.controllerParam != "" {
		rank += 15
	}
	if candidate.createCallbackName != "" {
		rank += 10
	}
	if candidate.viewAllCallbackName != "" {
		rank += 10
	}
	return rank
}

func builderRuntimeOverviewViewCandidateMatchesHints(candidate builderRuntimeOverviewViewCandidate, hints ...string) bool {
	matchTokens := []string{
		strings.TrimSpace(candidate.path),
		strings.TrimPrefix(strings.TrimSpace(candidate.path), "lib/"),
		strings.TrimSpace(candidate.className),
		strings.TrimSpace(candidate.controllerParam),
		strings.TrimSpace(candidate.controllerType),
		strings.TrimSpace(candidate.createCallbackName),
		strings.TrimSpace(candidate.viewAllCallbackName),
	}
	for _, hint := range hints {
		trimmedHint := strings.TrimSpace(hint)
		if trimmedHint == "" {
			continue
		}
		for _, token := range matchTokens {
			if token != "" && strings.Contains(trimmedHint, token) {
				return true
			}
		}
	}
	return false
}

func builderRuntimeOverviewControllerCandidateRank(candidate builderRuntimeOverviewControllerCandidate) int {
	rank := 0
	lowerPath := strings.ToLower(candidate.path)
	lowerClass := strings.ToLower(candidate.className)
	if candidate.path == "lib/controllers/home_controller.dart" || candidate.className == "HomeController" {
		rank += 100
	}
	if strings.Contains(lowerPath, "overview") || strings.Contains(lowerClass, "overview") {
		rank += 45
	}
	if strings.Contains(lowerPath, "home") || strings.Contains(lowerClass, "home") {
		rank += 40
	}
	if candidate.repositoryParam != "" {
		rank += 15
	}
	return rank
}

func builderRuntimeOverviewControllerCandidateMatchesHints(candidate builderRuntimeOverviewControllerCandidate, hints ...string) bool {
	matchTokens := []string{
		strings.TrimSpace(candidate.path),
		strings.TrimPrefix(strings.TrimSpace(candidate.path), "lib/"),
		strings.TrimSpace(candidate.className),
		strings.TrimSpace(candidate.repositoryParam),
	}
	for _, hint := range hints {
		trimmedHint := strings.TrimSpace(hint)
		if trimmedHint == "" {
			continue
		}
		for _, token := range matchTokens {
			if token != "" && strings.Contains(trimmedHint, token) {
				return true
			}
		}
	}
	return false
}

func builderRuntimeCollectionViewControllerParam(accepted map[string]struct{}) string {
	if _, ok := accepted["controller"]; ok {
		return "controller"
	}
	for name := range accepted {
		if strings.Contains(strings.ToLower(strings.TrimSpace(name)), "controller") {
			return name
		}
	}
	return ""
}

func builderRuntimeCollectionViewDetailCallbackName(accepted map[string]struct{}) string {
	for _, name := range []string{"onOpenRecordDetail", "onOpenTaskDetail"} {
		if _, ok := accepted[name]; ok {
			return name
		}
	}
	for name := range accepted {
		if strings.HasPrefix(strings.TrimSpace(name), "onOpen") {
			return name
		}
	}
	return ""
}

func builderRuntimeCollectionViewCreateCallbackName(accepted map[string]struct{}) string {
	for _, name := range []string{"onCreateRecord", "onCreateTask"} {
		if _, ok := accepted[name]; ok {
			return name
		}
	}
	for name := range accepted {
		if strings.HasPrefix(strings.TrimSpace(name), "onCreate") {
			return name
		}
	}
	return ""
}

func builderRuntimeModelImportPathForType(workspacePath, typeName string) string {
	trimmedTypeName := strings.TrimSpace(typeName)
	if strings.TrimSpace(workspacePath) == "" || trimmedTypeName == "" {
		return ""
	}
	modelDir := filepath.Join(workspacePath, "lib", "models")
	entries, err := os.ReadDir(modelDir)
	if err != nil {
		return ""
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".dart") {
			continue
		}
		content, readErr := os.ReadFile(filepath.Join(modelDir, entry.Name()))
		if readErr != nil {
			continue
		}
		for _, match := range builderRuntimeExportedDartTypePattern.FindAllStringSubmatch(string(content), -1) {
			if len(match) < 2 || strings.TrimSpace(match[1]) != trimmedTypeName {
				continue
			}
			return filepath.ToSlash(filepath.Join("lib", "models", entry.Name()))
		}
	}
	return ""
}

func builderRuntimeLikelyDetailViewPath(path string) bool {
	normalized := filepath.ToSlash(strings.TrimSpace(path))
	if !strings.HasPrefix(normalized, "lib/views/") || !strings.HasSuffix(normalized, ".dart") {
		return false
	}
	base := normalized[strings.LastIndex(normalized, "/")+1:]
	lowerBase := strings.ToLower(base)
	return strings.Contains(lowerBase, "detail") || strings.Contains(lowerBase, "inspection")
}

func builderRuntimeTaskBundleHasFallbackInspectionSurface(taskBundle []appruns.TaskBundleItem) bool {
	for _, task := range taskBundle {
		for _, targetPath := range task.TargetPaths {
			if builderRuntimeLikelyDetailViewPath(targetPath) {
				return true
			}
		}
	}
	return false
}

func builderRuntimeDetailViewCandidates(workspacePath string) ([]builderRuntimeDetailViewCandidate, error) {
	if strings.TrimSpace(workspacePath) == "" {
		return nil, nil
	}
	viewDir := filepath.Join(workspacePath, "lib", "views")
	entries, err := os.ReadDir(viewDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	seen := make(map[string]struct{})
	candidates := make([]builderRuntimeDetailViewCandidate, 0)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".dart") {
			continue
		}
		relPath := filepath.ToSlash(filepath.Join("lib", "views", entry.Name()))
		contentBytes, readErr := os.ReadFile(filepath.Join(viewDir, entry.Name()))
		if readErr != nil {
			return nil, readErr
		}
		content := string(contentBytes)
		matches := builderRuntimeNamedDartTypeDeclarationPattern.FindAllStringSubmatch(content, -1)
		for _, match := range matches {
			if len(match) < 3 || strings.TrimSpace(match[1]) != "class" {
				continue
			}
			className := strings.TrimSpace(match[2])
			if className == "" {
				continue
			}
			accepted, _ := builderRuntimeDartConstructorNamedParameters(content, className)
			_, required := builderRuntimeDartConstructorNamedParameters(content, className)
			editCallbackName := builderRuntimeDetailViewEditCallbackName(accepted)
			deleteCallbackName := builderRuntimeDetailViewDeleteCallbackName(accepted)
			hasOnEdit := editCallbackName != ""
			hasOnDelete := deleteCallbackName != ""
			lowerClassName := strings.ToLower(className)
			if !builderRuntimeLikelyDetailViewPath(relPath) && !strings.Contains(lowerClassName, "detail") && !strings.Contains(lowerClassName, "inspection") && !hasOnEdit && !hasOnDelete {
				continue
			}
			key := relPath + "::" + className
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			candidates = append(candidates, builderRuntimeDetailViewCandidate{
				path:               relPath,
				className:          className,
				recordParam:        builderRuntimeDetailViewRecordParam(required),
				editCallbackName:   editCallbackName,
				deleteCallbackName: deleteCallbackName,
				hasOnEdit:          hasOnEdit,
				hasOnDelete:        hasOnDelete,
				requiredParams:     builderRuntimeSortedAcceptedNames(required),
			})
		}
	}
	sort.Slice(candidates, func(left, right int) bool {
		leftRank := builderRuntimeDetailViewCandidateRank(candidates[left])
		rightRank := builderRuntimeDetailViewCandidateRank(candidates[right])
		if leftRank != rightRank {
			return leftRank > rightRank
		}
		if candidates[left].path == candidates[right].path {
			return candidates[left].className < candidates[right].className
		}
		return candidates[left].path < candidates[right].path
	})
	return candidates, nil
}

func builderRuntimePrimaryDetailViewCandidate(workspacePath string, hints ...string) builderRuntimeDetailViewCandidate {
	candidates, err := builderRuntimeDetailViewCandidates(workspacePath)
	if err != nil || len(candidates) == 0 {
		return builderRuntimeDetailViewCandidate{}
	}
	for _, candidate := range candidates {
		if builderRuntimeDetailViewCandidateMatchesHints(candidate, hints...) {
			return candidate
		}
	}
	if primaryRecordType := builderRuntimeOpenLitePrimaryRecordTypeName(workspacePath); primaryRecordType != "" {
		for _, candidate := range candidates {
			if builderRuntimeDetailViewCandidateMatchesPrimaryRecordType(candidate, primaryRecordType) {
				return candidate
			}
		}
	}
	if specificCandidate, ok := builderRuntimeUniqueSpecificDetailViewCandidate(candidates); ok {
		return specificCandidate
	}
	return candidates[0]
}

func builderRuntimeDetailViewCandidateMatchesPrimaryRecordType(candidate builderRuntimeDetailViewCandidate, primaryRecordType string) bool {
	trimmedType := strings.TrimSpace(primaryRecordType)
	if trimmedType == "" {
		return false
	}
	matchTokens := []string{
		strings.ToLower(strings.TrimSpace(candidate.path)),
		strings.ToLower(strings.TrimSpace(candidate.className)),
		strings.ToLower(strings.TrimSpace(candidate.recordParam)),
	}
	primaryLower := strings.ToLower(trimmedType)
	primarySnake := strings.ToLower(strings.TrimSpace(builderRuntimeOpenLiteIdentifierSnakeCase(trimmedType)))
	for _, want := range []string{primaryLower, primarySnake} {
		if want == "" {
			continue
		}
		for _, token := range matchTokens {
			if token != "" && strings.Contains(token, want) {
				return true
			}
		}
	}
	return false
}

func builderRuntimeUniqueSpecificDetailViewCandidate(candidates []builderRuntimeDetailViewCandidate) (builderRuntimeDetailViewCandidate, bool) {
	matchCount := 0
	matched := builderRuntimeDetailViewCandidate{}
	for _, candidate := range candidates {
		if !builderRuntimeDetailViewCandidateHasSpecificRecordParam(candidate) {
			continue
		}
		matched = candidate
		matchCount++
		if matchCount > 1 {
			return builderRuntimeDetailViewCandidate{}, false
		}
	}
	if matchCount == 1 {
		return matched, true
	}
	return builderRuntimeDetailViewCandidate{}, false
}

func builderRuntimeDetailViewCandidateHasSpecificRecordParam(candidate builderRuntimeDetailViewCandidate) bool {
	recordParam := strings.ToLower(strings.TrimSpace(candidate.recordParam))
	if recordParam == "" {
		return false
	}
	return recordParam != "record"
}

func builderRuntimeDetailSurfaceMutationHints(view builderRuntimeDetailViewCandidate) []string {
	hints := []string{
		view.path,
		view.className,
		view.recordParam,
		view.editCallbackName,
		view.deleteCallbackName,
	}
	trimmedRecordParam := strings.TrimSpace(view.recordParam)
	if trimmedRecordParam == "" {
		return hints
	}
	recordParamPascal := builderRuntimeIdentifierPascalCase(trimmedRecordParam)
	if recordParamPascal != "" {
		hints = append(hints,
			recordParamPascal+"FormPage",
			"initial"+recordParamPascal,
		)
	}
	recordParamSnake := builderRuntimeOpenLiteIdentifierSnakeCase(trimmedRecordParam)
	if recordParamSnake != "" {
		hints = append(hints,
			"lib/views/"+recordParamSnake+"_form_page.dart",
			"views/"+recordParamSnake+"_form_page.dart",
			recordParamSnake+"_form_page.dart",
		)
	}
	return hints
}

func builderRuntimeDetailSurfaceCollectionControllerHints(view builderRuntimeDetailViewCandidate, mutation builderRuntimeMutationSurfaceCandidate) []string {
	hints := []string{
		view.path,
		view.className,
		view.recordParam,
		view.editCallbackName,
		mutation.view.path,
		mutation.view.className,
		mutation.view.repositoryParam,
		mutation.view.initialParam,
	}
	trimmedRecordParam := strings.TrimSpace(view.recordParam)
	if trimmedRecordParam == "" {
		return hints
	}
	recordParamPascal := builderRuntimeIdentifierPascalCase(trimmedRecordParam)
	if recordParamPascal != "" {
		hints = append(hints, recordParamPascal+"CollectionController")
	}
	recordParamSnake := builderRuntimeOpenLiteIdentifierSnakeCase(trimmedRecordParam)
	if recordParamSnake != "" {
		hints = append(hints,
			"lib/controllers/"+recordParamSnake+"_collection_controller.dart",
			"controllers/"+recordParamSnake+"_collection_controller.dart",
			recordParamSnake+"_collection_controller.dart",
		)
	}
	return hints
}

func builderRuntimePrimaryDetailSurfaceCandidate(workspacePath string, hints ...string) builderRuntimeDetailSurfaceCandidate {
	view := builderRuntimePrimaryDetailViewCandidate(workspacePath, hints...)
	mutationHints := builderRuntimeDetailSurfaceMutationHints(view)
	mutationHints = append(mutationHints, hints...)
	mutation := builderRuntimePrimaryMutationSurfaceCandidate(workspacePath, mutationHints...)
	controllerHints := builderRuntimeDetailSurfaceCollectionControllerHints(view, mutation)
	controllerHints = append(controllerHints, hints...)
	controller := builderRuntimePrimaryCollectionControllerCandidate(workspacePath, controllerHints...)
	return builderRuntimeDetailSurfaceCandidate{
		view:       view,
		mutation:   mutation,
		controller: controller,
	}
}

func builderRuntimeDetailViewCandidateRank(candidate builderRuntimeDetailViewCandidate) int {
	rank := 0
	lowerPath := strings.ToLower(candidate.path)
	lowerClass := strings.ToLower(candidate.className)
	if candidate.path == "lib/views/record_detail_page.dart" || candidate.className == "RecordDetailPage" {
		rank += 100
	}
	if strings.Contains(lowerPath, "detail") || strings.Contains(lowerClass, "detail") || strings.Contains(lowerPath, "inspection") || strings.Contains(lowerClass, "inspection") {
		rank += 40
	}
	if candidate.hasOnEdit {
		rank += 20
	}
	if candidate.hasOnDelete {
		rank += 10
	}
	return rank
}

func builderRuntimeDetailViewCandidateMatchesHints(candidate builderRuntimeDetailViewCandidate, hints ...string) bool {
	matchTokens := []string{
		strings.TrimSpace(candidate.path),
		strings.TrimPrefix(strings.TrimSpace(candidate.path), "lib/"),
		strings.TrimSpace(candidate.className),
		strings.TrimSpace(candidate.recordParam),
		strings.TrimSpace(candidate.editCallbackName),
		strings.TrimSpace(candidate.deleteCallbackName),
	}
	for _, hint := range hints {
		trimmedHint := strings.TrimSpace(hint)
		if trimmedHint == "" {
			continue
		}
		for _, token := range matchTokens {
			if token != "" && strings.Contains(trimmedHint, token) {
				return true
			}
		}
	}
	return false
}

func builderRuntimeDetailViewEditCallbackName(accepted map[string]struct{}) string {
	for _, preferred := range []string{"onEdit", "onUpdate", "onModify"} {
		if _, ok := accepted[preferred]; ok {
			return preferred
		}
	}
	for _, prefix := range []string{"onEdit", "onUpdate", "onModify"} {
		for name := range accepted {
			if strings.HasPrefix(strings.TrimSpace(name), prefix) {
				return name
			}
		}
	}
	return ""
}

func builderRuntimeDetailViewDeleteCallbackName(accepted map[string]struct{}) string {
	if _, ok := accepted["onDelete"]; ok {
		return "onDelete"
	}
	for name := range accepted {
		if strings.HasPrefix(strings.TrimSpace(name), "onDelete") {
			return name
		}
	}
	return ""
}

func builderRuntimeDetailViewRecordParam(required map[string]struct{}) string {
	names := builderRuntimeSortedAcceptedNames(required)
	if len(names) == 0 {
		return ""
	}
	filtered := make([]string, 0, len(names))
	for _, name := range names {
		trimmed := strings.TrimSpace(name)
		if trimmed == "" || trimmed == "key" || strings.HasPrefix(trimmed, "on") {
			continue
		}
		filtered = append(filtered, trimmed)
	}
	if len(filtered) == 0 {
		return ""
	}
	for _, preferred := range []string{"record", "task", "sheet", "entry", "item"} {
		for _, name := range filtered {
			if strings.EqualFold(name, preferred) {
				return name
			}
		}
	}
	if len(filtered) == 1 {
		return filtered[0]
	}
	for _, name := range filtered {
		lower := strings.ToLower(name)
		if !strings.HasSuffix(lower, "s") && !strings.HasSuffix(lower, "ids") {
			return name
		}
	}
	return filtered[0]
}

func builderRuntimeSortedAcceptedNames(accepted map[string]struct{}) []string {
	if len(accepted) == 0 {
		return nil
	}
	names := make([]string, 0, len(accepted))
	for name := range accepted {
		names = append(names, strings.TrimSpace(name))
	}
	sort.Strings(names)
	return names
}

func builderRuntimeDetailViewSupportsMainCallbackWiring(candidate builderRuntimeDetailViewCandidate, resolvedRequiredParams map[string]builderRuntimeOpenLiteResolvedArgument) bool {
	if candidate.path == "" || candidate.className == "" || candidate.recordParam == "" {
		return false
	}
	allowed := map[string]struct{}{candidate.recordParam: {}}
	if candidate.editCallbackName != "" {
		allowed[candidate.editCallbackName] = struct{}{}
	}
	if candidate.deleteCallbackName != "" {
		allowed[candidate.deleteCallbackName] = struct{}{}
	}
	for _, name := range candidate.requiredParams {
		if _, ok := allowed[name]; ok {
			continue
		}
		if _, ok := resolvedRequiredParams[name]; ok {
			continue
		}
		return false
	}
	if candidate.deleteCallbackName != "" {
		return false
	}
	return true
}

func builderRuntimeMainViewConstructorCandidates(workspacePath, mainContent string) ([]builderRuntimeViewConstructorCandidate, error) {
	if strings.TrimSpace(workspacePath) == "" || strings.TrimSpace(mainContent) == "" {
		return nil, nil
	}
	viewDir := filepath.Join(workspacePath, "lib", "views")
	entries, err := os.ReadDir(viewDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	seen := make(map[string]struct{})
	candidates := make([]builderRuntimeViewConstructorCandidate, 0)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".dart") {
			continue
		}
		relPath := filepath.ToSlash(filepath.Join("lib", "views", entry.Name()))
		content, err := os.ReadFile(filepath.Join(viewDir, entry.Name()))
		if err != nil {
			return nil, err
		}
		matches := builderRuntimeNamedDartTypeDeclarationPattern.FindAllStringSubmatch(string(content), -1)
		for _, match := range matches {
			if len(match) < 3 || strings.TrimSpace(match[1]) != "class" {
				continue
			}
			className := strings.TrimSpace(match[2])
			if className == "" || len(builderRuntimeConstructedClassArgs(mainContent, className)) == 0 {
				continue
			}
			key := relPath + "::" + className
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			candidates = append(candidates, builderRuntimeViewConstructorCandidate{path: relPath, className: className})
		}
	}
	sort.Slice(candidates, func(left, right int) bool {
		if candidates[left].path == candidates[right].path {
			return candidates[left].className < candidates[right].className
		}
		return candidates[left].path < candidates[right].path
	})
	return candidates, nil
}

func builderRuntimeMainConstructsSurfaceView(workspacePath, mainContent string, viewPaths []string, fallbackClassNames ...string) bool {
	trimmedContent := strings.TrimSpace(mainContent)
	if trimmedContent == "" {
		return false
	}
	pathSet := make(map[string]struct{}, len(viewPaths))
	for _, viewPath := range viewPaths {
		normalizedPath := filepath.ToSlash(strings.TrimSpace(viewPath))
		if normalizedPath == "" {
			continue
		}
		pathSet[normalizedPath] = struct{}{}
	}
	if len(pathSet) > 0 {
		candidates, err := builderRuntimeMainViewConstructorCandidates(workspacePath, trimmedContent)
		if err == nil {
			for _, candidate := range candidates {
				if _, ok := pathSet[candidate.path]; ok {
					return true
				}
			}
		}
	}
	for _, className := range fallbackClassNames {
		trimmedClassName := strings.TrimSpace(className)
		if trimmedClassName != "" && strings.Contains(trimmedContent, trimmedClassName) {
			return true
		}
	}
	return false
}

func builderRuntimeRunAppWidgetClassName(content string) string {
	if strings.TrimSpace(content) == "" {
		return ""
	}
	match := builderRuntimeRunAppWidgetPattern.FindStringSubmatch(content)
	if len(match) < 2 {
		return ""
	}
	return strings.TrimSpace(match[1])
}

func builderRuntimeCallConstructsClassArgs(content, outerCall, className string) (string, bool) {
	trimmedOuterCall := strings.TrimSpace(outerCall)
	trimmedClassName := strings.TrimSpace(className)
	if strings.TrimSpace(content) == "" || trimmedOuterCall == "" || trimmedClassName == "" {
		return "", false
	}
	needle := trimmedOuterCall + "("
	searchStart := 0
	for searchStart < len(content) {
		index := strings.Index(content[searchStart:], needle)
		if index < 0 {
			return "", false
		}
		index += searchStart
		openOuter := index + len(trimmedOuterCall)
		closeOuter := builderRuntimeMatchingParenIndex(content, openOuter)
		if closeOuter < 0 {
			return "", false
		}
		outerArgs := content[openOuter+1 : closeOuter]
		innerIndex := strings.Index(outerArgs, trimmedClassName+"(")
		if innerIndex >= 0 {
			openInner := innerIndex + len(trimmedClassName)
			closeInner := builderRuntimeMatchingParenIndex(outerArgs, openInner)
			if closeInner < 0 {
				return "", false
			}
			return outerArgs[openInner+1 : closeInner], true
		}
		searchStart = closeOuter + 1
	}
	return "", false
}

func builderRuntimeDartConstructorAcceptsNamedParameter(content, className, parameter string) bool {
	trimmedParameter := strings.TrimSpace(parameter)
	if trimmedParameter == "" || strings.TrimSpace(content) == "" {
		return false
	}
	accepted, _ := builderRuntimeDartConstructorNamedParameters(content, className)
	_, ok := accepted[trimmedParameter]
	return ok
}

func builderRuntimeDartConstructorNamedParameters(content, className string) (map[string]struct{}, map[string]struct{}) {
	accepted := map[string]struct{}{}
	required := map[string]struct{}{}
	trimmedClassName := strings.TrimSpace(className)
	if trimmedClassName == "" || strings.TrimSpace(content) == "" {
		return accepted, required
	}
	pattern := regexp.MustCompile(`(?s)(?:const\s+)?` + regexp.QuoteMeta(trimmedClassName) + `\s*\(\s*\{([^\)]*)\}\s*\)`)
	for _, match := range pattern.FindAllStringSubmatch(content, -1) {
		if len(match) < 2 {
			continue
		}
		for _, segment := range builderRuntimeSplitTopLevelArguments(match[1]) {
			name, isRequired := builderRuntimeDartNamedParameter(segment)
			if name == "" {
				continue
			}
			accepted[name] = struct{}{}
			if isRequired {
				required[name] = struct{}{}
			}
		}
	}
	return accepted, required
}

func builderRuntimeDartNamedParameter(segment string) (string, bool) {
	trimmed := strings.TrimSpace(segment)
	if trimmed == "" {
		return "", false
	}
	isRequired := strings.HasPrefix(trimmed, "required ")
	trimmed = strings.TrimSpace(strings.TrimPrefix(trimmed, "required "))
	if defaultIndex := builderRuntimeTopLevelNamedArgumentColonIndex(trimmed); defaultIndex > 0 {
		trimmed = strings.TrimSpace(trimmed[:defaultIndex])
	}
	if assignIndex := strings.Index(trimmed, "="); assignIndex >= 0 {
		trimmed = strings.TrimSpace(trimmed[:assignIndex])
	}
	fieldFormalPattern := regexp.MustCompile(`(?:this|super)\.([A-Za-z_][A-Za-z0-9_]*)`)
	if match := fieldFormalPattern.FindStringSubmatch(trimmed); len(match) == 2 {
		return match[1], isRequired
	}
	trailingIdentifierPattern := regexp.MustCompile(`([A-Za-z_][A-Za-z0-9_]*)\s*$`)
	if match := trailingIdentifierPattern.FindStringSubmatch(trimmed); len(match) == 2 {
		return match[1], isRequired
	}
	return "", false
}

func builderRuntimeConstructedClassArgs(content, className string) []string {
	trimmedClassName := strings.TrimSpace(className)
	if trimmedClassName == "" || strings.TrimSpace(content) == "" {
		return nil
	}
	needle := trimmedClassName + "("
	searchStart := 0
	argsList := make([]string, 0, 1)
	for searchStart < len(content) {
		index := strings.Index(content[searchStart:], needle)
		if index < 0 {
			break
		}
		index += searchStart
		if index > 0 {
			if prev := content[index-1]; (prev >= 'a' && prev <= 'z') || (prev >= 'A' && prev <= 'Z') || (prev >= '0' && prev <= '9') || prev == '_' {
				searchStart = index + len(trimmedClassName)
				continue
			}
		}
		openParen := index + len(trimmedClassName)
		closeParen := builderRuntimeMatchingParenIndex(content, openParen)
		if closeParen < 0 {
			break
		}
		argsList = append(argsList, content[openParen+1:closeParen])
		searchStart = closeParen + 1
	}
	return argsList
}

func builderRuntimeTopLevelNamedArgumentSet(args string) map[string]struct{} {
	result := map[string]struct{}{}
	for _, segment := range builderRuntimeSplitTopLevelArguments(args) {
		trimmed := strings.TrimSpace(segment)
		if trimmed == "" {
			continue
		}
		colonIndex := builderRuntimeTopLevelNamedArgumentColonIndex(trimmed)
		if colonIndex <= 0 {
			continue
		}
		name := strings.TrimSpace(trimmed[:colonIndex])
		if matched, _ := regexp.MatchString(`^[A-Za-z_][A-Za-z0-9_]*$`, name); !matched {
			continue
		}
		result[name] = struct{}{}
	}
	return result
}

func builderRuntimeSplitTopLevelArguments(content string) []string {
	if strings.TrimSpace(content) == "" {
		return nil
	}
	segments := make([]string, 0, 4)
	start := 0
	parenDepth := 0
	braceDepth := 0
	bracketDepth := 0
	var quote byte
	escaped := false
	for index := 0; index < len(content); index++ {
		char := content[index]
		if quote != 0 {
			if escaped {
				escaped = false
				continue
			}
			if char == '\\' {
				escaped = true
				continue
			}
			if char == quote {
				quote = 0
			}
			continue
		}
		switch char {
		case '\'', '"':
			quote = char
		case '(':
			parenDepth++
		case ')':
			if parenDepth > 0 {
				parenDepth--
			}
		case '{':
			braceDepth++
		case '}':
			if braceDepth > 0 {
				braceDepth--
			}
		case '[':
			bracketDepth++
		case ']':
			if bracketDepth > 0 {
				bracketDepth--
			}
		case ',':
			if parenDepth == 0 && braceDepth == 0 && bracketDepth == 0 {
				segments = append(segments, content[start:index])
				start = index + 1
			}
		}
	}
	segments = append(segments, content[start:])
	return segments
}

func builderRuntimeTopLevelNamedArgumentColonIndex(content string) int {
	parenDepth := 0
	braceDepth := 0
	bracketDepth := 0
	var quote byte
	escaped := false
	for index := 0; index < len(content); index++ {
		char := content[index]
		if quote != 0 {
			if escaped {
				escaped = false
				continue
			}
			if char == '\\' {
				escaped = true
				continue
			}
			if char == quote {
				quote = 0
			}
			continue
		}
		switch char {
		case '\'', '"':
			quote = char
		case '(':
			parenDepth++
		case ')':
			if parenDepth > 0 {
				parenDepth--
			}
		case '{':
			braceDepth++
		case '}':
			if braceDepth > 0 {
				braceDepth--
			}
		case '[':
			bracketDepth++
		case ']':
			if bracketDepth > 0 {
				bracketDepth--
			}
		case ':':
			if parenDepth == 0 && braceDepth == 0 && bracketDepth == 0 {
				return index
			}
		}
	}
	return -1
}

func builderRuntimeTaskOutputLooksLikeUnexpectedListFilter(content string) bool {
	for _, marker := range []string{"setFilter(", "selectedFilter", "RecordListFilter", "record-filter-"} {
		if strings.Contains(content, marker) {
			return true
		}
	}
	return false
}

func builderRuntimeTaskOutputWiresCollectionCreateEntry(content string) bool {
	return builderRuntimeCollectionCreateEntryCallbackPattern.MatchString(content) && (strings.Contains(content, "FloatingActionButton") || strings.Contains(content, "createPrimaryActionLabel"))
}

func builderRuntimeTaskOutputReferencedDetailTypes(content string) []string {
	if strings.TrimSpace(content) == "" {
		return nil
	}
	seen := make(map[string]struct{})
	types := make([]string, 0)
	for _, match := range builderRuntimeConstructedDetailTypePattern.FindAllStringSubmatch(content, -1) {
		if len(match) < 2 {
			continue
		}
		typeName := strings.TrimSpace(match[1])
		if typeName == "" {
			continue
		}
		if _, ok := seen[typeName]; ok {
			continue
		}
		seen[typeName] = struct{}{}
		types = append(types, typeName)
	}
	slices.Sort(types)
	return types
}

func builderRuntimeTaskOutputPathCanReferenceRecordDetail(path string) bool {
	normalized := filepath.ToSlash(strings.TrimSpace(path))
	if normalized == "lib/main.dart" || normalized == "test/widget_test.dart" {
		return true
	}
	return strings.HasPrefix(normalized, "lib/views/") && strings.HasSuffix(normalized, ".dart")
}

func builderRuntimeTaskOutputPathCanReferenceDeleteFlow(path string) bool {
	normalized := filepath.ToSlash(strings.TrimSpace(path))
	if normalized == "lib/main.dart" || normalized == "test/widget_test.dart" {
		return true
	}
	return strings.HasPrefix(normalized, "lib/views/") || strings.HasPrefix(normalized, "lib/controllers/") || strings.HasPrefix(normalized, "lib/repositories/")
}

func validateBuilderRuntimeDirectFailureCoverage(taskType appruns.BuilderRuntimeTaskType, operations []appruns.WorkspacePatchOperation, targetPaths []string) error {
	switch taskType {
	case appruns.BuilderRuntimeTaskTypeAnalyzeRepair, appruns.BuilderRuntimeTaskTypeTestRepair:
	default:
		return nil
	}
	required := make([]string, 0, len(targetPaths))
	seenRequired := map[string]struct{}{}
	for _, path := range targetPaths {
		normalized := filepath.ToSlash(strings.TrimSpace(path))
		if normalized == "" || strings.Contains(normalized, "*") {
			continue
		}
		if _, ok := seenRequired[normalized]; ok {
			continue
		}
		seenRequired[normalized] = struct{}{}
		required = append(required, normalized)
	}
	if len(required) == 0 {
		return nil
	}
	touched := map[string]struct{}{}
	for _, operation := range operations {
		normalized := filepath.ToSlash(strings.TrimSpace(operation.Path))
		if normalized == "" {
			continue
		}
		touched[normalized] = struct{}{}
	}
	missing := builderRuntimeDirectFailureCoverageMissingTargets(operations, required)
	if len(missing) == 0 {
		return nil
	}
	return fmt.Errorf("patch did not touch every directly failing repair target: %s", strings.Join(missing, ", "))
}

func validateBuilderRuntimeTaskTargetCoverage(taskType appruns.BuilderRuntimeTaskType, operations []appruns.WorkspacePatchOperation, targetPaths []string) error {
	switch taskType {
	case appruns.BuilderRuntimeTaskTypeAnalyzeRepair, appruns.BuilderRuntimeTaskTypeTestRepair, appruns.BuilderRuntimeTaskTypeClosureRepair:
		return nil
	}
	missing := builderRuntimeDirectFailureCoverageMissingTargets(operations, targetPaths)
	if len(missing) == 0 {
		return nil
	}
	return fmt.Errorf("patch did not touch every current task target path: %s", strings.Join(missing, ", "))
}

func mergeBuilderRuntimePatchOperations(existing *appruns.WorkspacePatch, operations []appruns.WorkspacePatchOperation) []appruns.WorkspacePatchOperation {
	merged := make([]appruns.WorkspacePatchOperation, 0, len(operations))
	if existing != nil && len(existing.Operations) > 0 {
		merged = append(merged, existing.Operations...)
	}
	merged = append(merged, operations...)
	return merged
}

func builderRuntimeDirectFailureCoverageMissingTargets(operations []appruns.WorkspacePatchOperation, targetPaths []string) []string {
	required := make([]string, 0, len(targetPaths))
	seenRequired := map[string]struct{}{}
	for _, path := range targetPaths {
		normalized := filepath.ToSlash(strings.TrimSpace(path))
		if normalized == "" || strings.Contains(normalized, "*") {
			continue
		}
		if _, ok := seenRequired[normalized]; ok {
			continue
		}
		seenRequired[normalized] = struct{}{}
		required = append(required, normalized)
	}
	touched := map[string]struct{}{}
	for _, operation := range operations {
		normalized := filepath.ToSlash(strings.TrimSpace(operation.Path))
		if normalized == "" {
			continue
		}
		touched[normalized] = struct{}{}
	}
	missing := make([]string, 0)
	for _, path := range required {
		if _, ok := touched[path]; ok {
			continue
		}
		missing = append(missing, path)
	}
	sort.Strings(missing)
	return missing
}

func cloneRunRecordWithRouteTargetPaths(run runRecord, routeTaskID string, targetPaths []string) runRecord {
	cloned := run
	cloned.TaskBundle = cloneTaskBundleWithRouteTargetPaths(run.TaskBundle, routeTaskID, targetPaths)
	return cloned
}

func cloneRoundInputWithTaskTargetPaths(input appruns.RoundInput, routeTaskID string, targetPaths []string) appruns.RoundInput {
	cloned := input
	cloned.TaskBundle = cloneTaskBundleWithRouteTargetPaths(input.TaskBundle, routeTaskID, targetPaths)
	return cloned
}

func cloneTaskBundleWithRouteTargetPaths(tasks []appruns.TaskBundleItem, routeTaskID string, targetPaths []string) []appruns.TaskBundleItem {
	cloned := append([]appruns.TaskBundleItem(nil), tasks...)
	for index, task := range cloned {
		normalized := appruns.NormalizeTaskBundleItem(task)
		if strings.TrimSpace(normalized.TaskID) != strings.TrimSpace(routeTaskID) {
			continue
		}
		normalized.TargetPaths = append([]string(nil), targetPaths...)
		cloned[index] = normalized
	}
	return cloned
}

var errBuilderRuntimeDartFormatterUnavailable = errors.New("dart formatter unavailable")

var builderRuntimeDartFormatterCommandBuilder = buildBuilderRuntimeDartFormatterCommand

func validateBuilderRuntimeDartSyntax(run runRecord, relPath, absPath string, content []byte) error {
	formatterErr := validateBuilderRuntimeDartSyntaxWithFormatter(run, relPath, absPath)
	if formatterErr == nil {
		return nil
	}
	if heuristicErr := validateBuilderRuntimeDartSyntaxHeuristically(string(content)); heuristicErr != nil {
		if errors.Is(formatterErr, errBuilderRuntimeDartFormatterUnavailable) {
			return heuristicErr
		}
		return fmt.Errorf("%v; heuristic fallback also failed: %w", formatterErr, heuristicErr)
	}
	if errors.Is(formatterErr, errBuilderRuntimeDartFormatterUnavailable) {
		return nil
	}
	return formatterErr
}

func validateBuilderRuntimeDartSyntaxWithFormatter(run runRecord, relPath, absPath string) error {
	command, err := builderRuntimeDartFormatterCommandBuilder(context.Background(), run, relPath, absPath)
	if err != nil {
		return err
	}
	if command.Path == "" {
		return errBuilderRuntimeDartFormatterUnavailable
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	originalDir := command.Dir
	originalEnv := append([]string(nil), command.Env...)
	originalArgs := append([]string(nil), command.Args...)
	command = exec.CommandContext(ctx, command.Path, originalArgs[1:]...)
	command.Dir = originalDir
	command.Env = originalEnv
	output, err := command.CombinedOutput()
	if err == nil {
		return nil
	}
	var execErr *exec.Error
	if errors.As(err, &execErr) {
		return errBuilderRuntimeDartFormatterUnavailable
	}
	var pathErr *os.PathError
	if errors.As(err, &pathErr) && strings.Contains(strings.ToLower(pathErr.Err.Error()), "not found") {
		return errBuilderRuntimeDartFormatterUnavailable
	}
	trimmed := strings.TrimSpace(string(output))
	if trimmed == "" {
		trimmed = err.Error()
	}
	return fmt.Errorf("dart format rejected file: %s", trimmed)
}

func buildBuilderRuntimeDartFormatterCommand(ctx context.Context, run runRecord, relPath, absPath string) (*exec.Cmd, error) {
	if strings.TrimSpace(run.ExecutorImage) != "" {
		return dockerCommand(ctx, run, []string{"dart", "format", "--output=none", relPath}, nil)
	}
	dartPath, err := exec.LookPath("dart")
	if err != nil {
		return nil, errBuilderRuntimeDartFormatterUnavailable
	}
	return exec.CommandContext(ctx, dartPath, "format", "--output=none", absPath), nil
}

func validateBuilderRuntimeDartSyntaxHeuristically(content string) error {
	type delimiter struct {
		char rune
		line int
		col  int
	}
	stack := make([]delimiter, 0)
	line := 1
	col := 0
	inLineComment := false
	inBlockComment := false
	stringDelimiter := rune(0)
	escaped := false
	stringStartLine := 0
	stringStartCol := 0
	for _, char := range content {
		if char == '\n' {
			line++
			col = 0
			if inLineComment {
				inLineComment = false
			}
			if stringDelimiter != 0 && !escaped {
				return fmt.Errorf("unterminated string literal starting at line %d column %d", stringStartLine, stringStartCol)
			}
			escaped = false
			continue
		}
		col++
		if inLineComment {
			continue
		}
		if inBlockComment {
			if char == '/' && col > 1 {
				prev := rune(contentRuneAt(content, line, col-1))
				if prev == '*' {
					inBlockComment = false
				}
			}
			continue
		}
		if stringDelimiter != 0 {
			if escaped {
				escaped = false
				continue
			}
			if char == '\\' {
				escaped = true
				continue
			}
			if char == stringDelimiter {
				stringDelimiter = 0
			}
			continue
		}
		if char == '/' {
			prev := rune(0)
			if col > 1 {
				prev = rune(contentRuneAt(content, line, col-1))
			}
			next := rune(0)
			if nextRune, ok := nextNonConsumedRune(content, line, col); ok {
				next = nextRune
			}
			if next == '/' && prev != '/' {
				inLineComment = true
				continue
			}
			if next == '*' && prev != '*' {
				inBlockComment = true
				continue
			}
		}
		switch char {
		case '\'', '"':
			stringDelimiter = char
			stringStartLine = line
			stringStartCol = col
		case '(', '{', '[':
			stack = append(stack, delimiter{char: char, line: line, col: col})
		case ')', '}', ']':
			if len(stack) == 0 {
				return fmt.Errorf("unexpected closing %q at line %d column %d", char, line, col)
			}
			top := stack[len(stack)-1]
			if !builderRuntimeMatchingDelimiter(top.char, char) {
				return fmt.Errorf("mismatched closing %q at line %d column %d for opener %q at line %d column %d", char, line, col, top.char, top.line, top.col)
			}
			stack = stack[:len(stack)-1]
		}
	}
	if stringDelimiter != 0 {
		return fmt.Errorf("unterminated string literal starting at line %d column %d", stringStartLine, stringStartCol)
	}
	if inBlockComment {
		return fmt.Errorf("unterminated block comment")
	}
	if len(stack) > 0 {
		top := stack[len(stack)-1]
		return fmt.Errorf("unclosed delimiter %q opened at line %d column %d", top.char, top.line, top.col)
	}
	return nil
}

func builderRuntimeMatchingDelimiter(open, close rune) bool {
	return (open == '(' && close == ')') || (open == '{' && close == '}') || (open == '[' && close == ']')
}

func contentRuneAt(content string, wantLine, wantCol int) byte {
	line := 1
	col := 0
	for index := 0; index < len(content); index++ {
		if content[index] == '\n' {
			line++
			col = 0
			continue
		}
		col++
		if line == wantLine && col == wantCol {
			return content[index]
		}
	}
	return 0
}

func nextNonConsumedRune(content string, line, col int) (rune, bool) {
	currentLine := 1
	currentCol := 0
	for index := 0; index < len(content); index++ {
		if content[index] == '\n' {
			currentLine++
			currentCol = 0
			continue
		}
		currentCol++
		if currentLine == line && currentCol == col {
			if index+1 >= len(content) {
				return 0, false
			}
			return rune(content[index+1]), true
		}
	}
	return 0, false
}

func builderRuntimeTaskOwnsPath(tasks []appruns.TaskBundleItem, taskID, relPath string) bool {
	taskID = strings.TrimSpace(taskID)
	relPath = filepath.ToSlash(strings.TrimSpace(relPath))
	if taskID == "" || relPath == "" {
		return false
	}
	for _, task := range tasks {
		normalized := appruns.NormalizeTaskBundleItem(task)
		if strings.TrimSpace(normalized.TaskID) != taskID {
			continue
		}
		for _, targetPath := range concreteTaskTargetPaths([]appruns.TaskBundleItem{normalized}) {
			if filepath.ToSlash(strings.TrimSpace(targetPath)) == relPath {
				return true
			}
		}
		return false
	}
	return false
}

func builderRuntimeTaskSelectionScore(task appruns.TaskBundleItem) int {
	score := builderRuntimeTaskPriority(task.EffectiveTaskType())
	switch appruns.NormalizeTaskRouteHint(string(task.RouteHint)) {
	case appruns.TaskRouteHintStrongModel, appruns.TaskRouteHintUpgradeModel:
		score += 100
	case appruns.TaskRouteHintDeterministic:
		score += 50
	}
	switch task.RiskLevel {
	case appruns.TaskRiskLevelHigh:
		score += 20
	case appruns.TaskRiskLevelMedium:
		score += 10
	}
	return score
}

func builderRuntimeTaskPriority(taskType appruns.BuilderRuntimeTaskType) int {
	switch taskType {
	case appruns.BuilderRuntimeTaskTypeClosureRepair:
		return 5
	case appruns.BuilderRuntimeTaskTypeTestRepair:
		return 4
	case appruns.BuilderRuntimeTaskTypeAnalyzeRepair:
		return 3
	case appruns.BuilderRuntimeTaskTypeDualFileWiring:
		return 2
	case appruns.BuilderRuntimeTaskTypeSingleFileEdit:
		return 1
	default:
		return 0
	}
}

func concreteTaskTargetPaths(tasks []appruns.TaskBundleItem) []string {
	seen := map[string]struct{}{}
	paths := make([]string, 0)
	for _, task := range tasks {
		for _, path := range task.TargetPaths {
			trimmed := filepath.ToSlash(strings.TrimSpace(path))
			if trimmed == "" || strings.ContainsAny(trimmed, "*?[]") {
				continue
			}
			if _, ok := seen[trimmed]; ok {
				continue
			}
			seen[trimmed] = struct{}{}
			paths = append(paths, trimmed)
		}
	}
	sort.Strings(paths)
	return paths
}

func concreteAllowedPaths(paths []string) []string {
	seen := map[string]struct{}{}
	result := make([]string, 0)
	for _, item := range paths {
		trimmed := filepath.ToSlash(strings.TrimSpace(item))
		if trimmed == "" || strings.ContainsAny(trimmed, "*?[]") {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		result = append(result, trimmed)
	}
	sort.Strings(result)
	return result
}

func classifyBuilderRuntimeOperations(operations []appruns.WorkspacePatchOperation, targetPaths []string) (int, int) {
	if len(operations) == 0 {
		return 0, 0
	}
	if len(targetPaths) == 0 {
		return len(operations), 0
	}
	targetSet := map[string]struct{}{}
	for _, path := range targetPaths {
		targetSet[path] = struct{}{}
	}
	targeted := 0
	unrelated := 0
	for _, operation := range operations {
		if _, ok := targetSet[filepath.ToSlash(strings.TrimSpace(operation.Path))]; ok {
			targeted++
			continue
		}
		unrelated++
	}
	return targeted, unrelated
}

func modelAliasesFromRef(ref appruns.BuilderRuntimeModelRef) []string {
	seen := map[string]struct{}{}
	aliases := make([]string, 0, 1+len(ref.Fallbacks))
	for _, item := range append([]string{ref.Primary}, ref.Fallbacks...) {
		trimmed := strings.TrimSpace(item)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		aliases = append(aliases, trimmed)
	}
	return aliases
}

func builderRuntimeRepairModelAliases(ref appruns.BuilderRuntimeModelRef, preferred string) []string {
	aliases := make([]string, 0, 1+len(ref.Fallbacks))
	seen := map[string]struct{}{}
	appendAlias := func(value string) {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			return
		}
		if _, ok := seen[trimmed]; ok {
			return
		}
		seen[trimmed] = struct{}{}
		aliases = append(aliases, trimmed)
	}
	appendAlias(preferred)
	for _, item := range modelAliasesFromRef(ref) {
		appendAlias(item)
	}
	return aliases
}

func firstString(values map[string]any, keys ...string) string {
	value, _ := firstStringWithKey(values, keys...)
	return value
}

func firstStringWithKey(values map[string]any, keys ...string) (string, string) {
	for _, key := range keys {
		raw, ok := values[key]
		if !ok {
			continue
		}
		if text, ok := raw.(string); ok {
			return text, key
		}
	}
	return "", ""
}

func appendBuilderRuntimeLog(logPath, modelAlias, content string) {
	if strings.TrimSpace(logPath) == "" {
		return
	}
	if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
		return
	}
	file, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return
	}
	defer file.Close()
	_, _ = file.WriteString("\n=== builder-runtime model: " + strings.TrimSpace(modelAlias) + " ===\n")
	_, _ = file.WriteString(content)
	if !strings.HasSuffix(content, "\n") {
		_, _ = file.WriteString("\n")
	}
}

func modifiedFilePaths(changes []appruns.FileChange) []string {
	paths := make([]string, 0, len(changes))
	for _, change := range changes {
		if strings.TrimSpace(change.Path) == "" {
			continue
		}
		paths = append(paths, change.Path)
	}
	return paths
}

func btoi(value bool) int {
	if value {
		return 1
	}
	return 0
}

func builderRuntimeDiagnosis(err error) (*builderRuntimeExecutionError, bool) {
	var runtimeErr *builderRuntimeExecutionError
	if errors.As(err, &runtimeErr) {
		return runtimeErr, true
	}
	return nil, false
}
