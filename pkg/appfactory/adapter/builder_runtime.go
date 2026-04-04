package adapter

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	appruns "github.com/sipeed/picoclaw/pkg/appfactory/runs"
	appconfig "github.com/sipeed/picoclaw/pkg/config"
	"github.com/sipeed/picoclaw/pkg/providers"
)

type BuilderRuntimePatchGenerator interface {
	GeneratePatch(ctx context.Context, request BuilderRuntimePatchRequest) (BuilderRuntimePatchResponse, error)
}

type BuilderRuntimePatchRequest struct {
	ModelAliases []string
	Run          runRecord
	RoundInput   appruns.RoundInput
	Route        appruns.BuilderRuntimeTaskRoute
}

type BuilderRuntimePatchResponse struct {
	ModelAlias       string
	Content          string
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
}

type defaultBuilderRuntimePatchGenerator struct {
	modelCatalog *appconfig.Config
}

type builderRuntimeExecutionResult struct {
	Patch       *appruns.WorkspacePatch
	ApplyResult appruns.WorkspacePatchApplyResult
	Stats       *appruns.BuilderRuntimeExecutionStats
}

type builderRuntimeExecutionError struct {
	summary            string
	recoverySuggestion string
	signature          string
	policy             repairFailurePolicy
	preserveWorkspace  bool
	resumeAllowed      bool
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
	prompt, err := buildBuilderRuntimePrompt(request.Run, request.RoundInput, request.Route)
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
	return route.Model.Primary != ""
}

func (runner *Runner) executeBuilderRuntimeEdit(ctx context.Context, backend RunnerBackend, runID string, step ExecutionStep, run runRecord, roundInput appruns.RoundInput) (*builderRuntimeExecutionResult, error) {
	route, upgradedAtStart := selectBuilderRuntimeRoute(run)
	stats := &appruns.BuilderRuntimeExecutionStats{
		Mode:        "model",
		TaskType:    route.TaskType,
		RouteSource: route.RouteSource,
	}
	result := &builderRuntimeExecutionResult{Stats: stats}
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
	request := BuilderRuntimePatchRequest{Run: run, RoundInput: roundInput, Route: route, ModelAliases: modelAliasesFromRef(route.Model)}
	roundState := heartbeatRoundStateForStep(step)
	if err := runner.reportPatchGenerationStarted(ctx, backend, runID, step, roundInput, roundState); err != nil {
		return result, err
	}
	response, err := runner.builderRuntimePatchGenerator().GeneratePatch(ctx, request)
	stats.Attempts++
	if err != nil {
		stats.FailureReason = err.Error()
		if reportErr := runner.reportPatchGenerationFailed(ctx, backend, runID, step, roundInput, roundState, collectHeartbeatTargetPaths(roundInput), err); reportErr != nil {
			return result, reportErr
		}
		return result, &builderRuntimeExecutionError{
			summary:            fmt.Sprintf("builder runtime model request failed: %v", err),
			recoverySuggestion: "fix model connectivity or route configuration before rerun",
			signature:          "builder_runtime_model_request_failed",
			preserveWorkspace:  true,
			resumeAllowed:      false,
			policy:             repairFailurePolicy{PreserveWorkspace: true, ResumeAllowed: false, RequiresHumanReview: true, MaxRounds: 1, UsedRounds: 1, RemainingRounds: 0, TerminationReason: "builder runtime model request failed"},
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
	patch, normalized, driftCount, parseErr := normalizeBuilderRuntimePatch(response.Content, roundInput.RoundID, concreteTaskTargetPaths(run.TaskBundle))
	stats.SchemaNormalized = normalized
	stats.SchemaDriftCount += driftCount
	if parseErr != nil {
		stats.ParseFailureCount++
		stats.FailureReason = parseErr.Error()
		if reportErr := runner.reportPatchGenerationFailed(ctx, backend, runID, step, roundInput, roundState, collectHeartbeatTargetPaths(roundInput), parseErr); reportErr != nil {
			return result, reportErr
		}
		if shouldRetryBuilderRuntimeWithUpgrade(run, stats, "parse_failure") {
			return runner.retryBuilderRuntimeWithUpgrade(ctx, backend, runID, step, run, roundInput, stats, response.ModelAlias, parseErr)
		}
		return result, &builderRuntimeExecutionError{
			summary:            fmt.Sprintf("builder runtime patch parse failed: %v", parseErr),
			recoverySuggestion: "tighten patch schema normalization or upgrade the model route before rerun",
			signature:          "builder_runtime_patch_parse_failed",
			preserveWorkspace:  true,
			resumeAllowed:      false,
			policy:             repairFailurePolicy{PreserveWorkspace: true, ResumeAllowed: false, RequiresHumanReview: false, MaxRounds: 2, UsedRounds: 1, RemainingRounds: 1},
			stats:              stats,
			wrapped:            parseErr,
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
		if shouldRetryBuilderRuntimeWithUpgrade(run, stats, "scope_violation") {
			return runner.retryBuilderRuntimeWithUpgrade(ctx, backend, runID, step, run, roundInput, stats, response.ModelAlias, applyErr)
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
	stats.OperationCount = len(patch.Operations)
	stats.TargetedOperationCount, stats.UnrelatedOperationCount = classifyBuilderRuntimeOperations(patch.Operations, concreteTaskTargetPaths(run.TaskBundle))
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

func (runner *Runner) retryBuilderRuntimeWithUpgrade(ctx context.Context, backend RunnerBackend, runID string, step ExecutionStep, run runRecord, roundInput appruns.RoundInput, stats *appruns.BuilderRuntimeExecutionStats, previousModel string, previousErr error) (*builderRuntimeExecutionResult, error) {
	result := &builderRuntimeExecutionResult{Stats: stats}
	if run.BuilderRuntime == nil || run.BuilderRuntime.UpgradeModel.Primary == "" {
		return result, previousErr
	}
	if previousModel == run.BuilderRuntime.UpgradeModel.Primary {
		return result, previousErr
	}
	stats.UpgradeApplied = true
	upgradeRoute := appruns.BuilderRuntimeTaskRoute{
		TaskID:      dominantBuilderRuntimeTask(run.TaskBundle).TaskID,
		TaskType:    dominantBuilderRuntimeTask(run.TaskBundle).EffectiveTaskType(),
		RouteSource: "upgrade_model",
		Model:       run.BuilderRuntime.UpgradeModel,
	}
	request := BuilderRuntimePatchRequest{Run: run, RoundInput: roundInput, Route: upgradeRoute, ModelAliases: modelAliasesFromRef(upgradeRoute.Model)}
	roundState := heartbeatRoundStateForStep(step)
	if err := runner.reportPatchGenerationStarted(ctx, backend, runID, step, roundInput, roundState); err != nil {
		return result, err
	}
	response, err := runner.builderRuntimePatchGenerator().GeneratePatch(ctx, request)
	stats.Attempts++
	if err != nil {
		stats.FailureReason = err.Error()
		if reportErr := runner.reportPatchGenerationFailed(ctx, backend, runID, step, roundInput, roundState, collectHeartbeatTargetPaths(roundInput), err); reportErr != nil {
			return result, reportErr
		}
		return result, &builderRuntimeExecutionError{
			summary:            fmt.Sprintf("builder runtime upgrade model request failed: %v", err),
			recoverySuggestion: "fix upgrade model connectivity or route configuration before rerun",
			signature:          "builder_runtime_model_request_failed",
			preserveWorkspace:  true,
			resumeAllowed:      false,
			policy:             repairFailurePolicy{PreserveWorkspace: true, ResumeAllowed: false, RequiresHumanReview: true, MaxRounds: 1, UsedRounds: 1, RemainingRounds: 0, TerminationReason: "builder runtime upgrade model request failed"},
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
	patch, normalized, driftCount, parseErr := normalizeBuilderRuntimePatch(response.Content, roundInput.RoundID, concreteTaskTargetPaths(run.TaskBundle))
	stats.SchemaNormalized = stats.SchemaNormalized || normalized
	stats.SchemaDriftCount += driftCount
	if parseErr != nil {
		stats.ParseFailureCount++
		stats.FailureReason = parseErr.Error()
		if reportErr := runner.reportPatchGenerationFailed(ctx, backend, runID, step, roundInput, roundState, collectHeartbeatTargetPaths(roundInput), parseErr); reportErr != nil {
			return result, reportErr
		}
		return result, &builderRuntimeExecutionError{
			summary:            fmt.Sprintf("builder runtime patch parse failed after upgrade: %v", parseErr),
			recoverySuggestion: "tighten patch schema normalization or refine the builder-runtime prompt before rerun",
			signature:          "builder_runtime_patch_parse_failed",
			preserveWorkspace:  true,
			resumeAllowed:      false,
			policy:             repairFailurePolicy{PreserveWorkspace: true, ResumeAllowed: false, RequiresHumanReview: false, MaxRounds: 2, UsedRounds: 2, RemainingRounds: 0, TerminationReason: "builder runtime upgrade parse failed"},
			stats:              stats,
			wrapped:            parseErr,
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
	stats.OperationCount = len(patch.Operations)
	stats.TargetedOperationCount, stats.UnrelatedOperationCount = classifyBuilderRuntimeOperations(patch.Operations, concreteTaskTargetPaths(run.TaskBundle))
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

func buildBuilderRuntimePrompt(run runRecord, roundInput appruns.RoundInput, route appruns.BuilderRuntimeTaskRoute) (string, error) {
	files, err := buildBuilderRuntimeFileContext(run)
	if err != nil {
		return "", err
	}
	tasksJSON, _ := json.Marshal(roundInput.TaskBundle)
	checksJSON, _ := json.Marshal(roundInput.AcceptanceChecks)
	allowedJSON, _ := json.Marshal(roundInput.AllowedPaths)
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
	builder.WriteString("For UI entry files in task target_paths such as lib/main.dart and lib/views/*.dart, prefer write_file with the complete final file content instead of replace_block. Do not emit a partial replace_block for those files.\n")
	builder.WriteString("After applying your patch, every Dart import and referenced widget/controller/repository class must still resolve.\n")
	builder.WriteString("Do not introduce Flutter or Dart APIs that are already deprecated in current stable toolchains. For color alpha updates, prefer withValues() and avoid withOpacity().\n")
	builder.WriteString("Treat flutter analyze as a hard acceptance gate: return code that is free of deprecated_member_use and comparable analyzer findings, not just syntax errors.\n")
	builder.WriteString("Goal summary: ")
	builder.WriteString(strings.TrimSpace(run.GoalSummary))
	builder.WriteString("\n")
	if notes := strings.TrimSpace(string(run.HumanNotes)); notes != "" {
		builder.WriteString("Human notes JSON: ")
		builder.WriteString(notes)
		builder.WriteString("\n")
	}
	builder.WriteString("Attempt: ")
	builder.WriteString(fmt.Sprintf("%d\n", roundInput.Attempt))
	builder.WriteString("Primary task route: ")
	builder.WriteString(string(route.TaskType))
	builder.WriteString(" via ")
	builder.WriteString(route.RouteSource)
	builder.WriteString("\n")
	builder.WriteString("Task bundle JSON: ")
	builder.Write(tasksJSON)
	builder.WriteString("\n")
	builder.WriteString("Acceptance checks JSON: ")
	builder.Write(checksJSON)
	builder.WriteString("\n")
	builder.WriteString("Allowed paths JSON: ")
	builder.Write(allowedJSON)
	builder.WriteString("\n")
	builder.WriteString("Current file context:\n")
	builder.WriteString(files)
	return builder.String(), nil
}

func buildBuilderRuntimeFileContext(run runRecord) (string, error) {
	paths := concreteTaskTargetPaths(run.TaskBundle)
	if len(paths) == 0 {
		paths = concreteAllowedPaths(run.AllowedPaths)
	}
	if len(paths) == 0 {
		return "- no concrete files available\n", nil
	}
	var builder strings.Builder
	for _, relPath := range paths {
		builder.WriteString("FILE: ")
		builder.WriteString(relPath)
		builder.WriteString("\n")
		absPath := filepath.Join(run.WorkspacePath, filepath.FromSlash(relPath))
		content, err := os.ReadFile(absPath)
		if err != nil {
			if os.IsNotExist(err) {
				builder.WriteString("<missing>\n")
				continue
			}
			return "", fmt.Errorf("read builder runtime context file %s: %w", relPath, err)
		}
		builder.Write(content)
		if len(content) == 0 || content[len(content)-1] != '\n' {
			builder.WriteString("\n")
		}
	}
	return builder.String(), nil
}

func normalizeBuilderRuntimePatch(rawContent, roundID string, targetPaths []string) (appruns.WorkspacePatch, bool, int, error) {
	normalizedContent, normalizedFence := sanitizeBuilderRuntimeJSON(rawContent)
	var payload map[string]any
	if err := json.Unmarshal([]byte(normalizedContent), &payload); err != nil {
		return appruns.WorkspacePatch{}, normalizedFence, btoi(normalizedFence), err
	}
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
	driftCount := btoi(normalizedFence)
	usedNormalization := normalizedFence
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

func normalizeBuilderRuntimeTargetPath(path string) string {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return ""
	}
	normalized := filepath.ToSlash(filepath.Clean(filepath.FromSlash(trimmed)))
	return strings.TrimPrefix(normalized, "./")
}

func normalizeBuilderRuntimeOperationShape(opMap map[string]any) (map[string]any, bool) {
	if len(opMap) != 1 {
		return opMap, false
	}
	for key, value := range opMap {
		normalizedType := normalizeBuilderRuntimeOperationType(key)
		if normalizedType == "" {
			return opMap, false
		}
		nested, ok := value.(map[string]any)
		if !ok {
			return opMap, false
		}
		flattened := make(map[string]any, len(nested)+1)
		if nestedType, _ := firstStringWithKey(nested, "type"); nestedType != "" {
			normalizedNestedType := normalizeBuilderRuntimeOperationType(nestedType)
			if normalizedNestedType != "" && normalizedNestedType != normalizedType {
				flattened["_conflicting_type"] = normalizedNestedType
			}
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
	repairedEscapes, repairedEscapesOK := repairBuilderRuntimeInvalidStringEscapes(trimmed)
	if repairedEscapesOK {
		trimmed = repairedEscapes
		normalized = true
	}
	repaired, repairedOK := repairBuilderRuntimeJSONObject(trimmed)
	if repairedOK {
		return repaired, true
	}
	return trimmed, normalized
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
	start := strings.IndexByte(trimmed, '{')
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
	dominant := dominantBuilderRuntimeTask(run.TaskBundle)
	selected := appruns.BuilderRuntimeTaskRoute{TaskID: dominant.TaskID, TaskType: dominant.EffectiveTaskType(), RouteSource: "unconfigured"}
	if run.BuilderRuntime == nil {
		return selected, false
	}
	for _, route := range run.BuilderRuntime.TaskRoutes {
		if route.TaskID == dominant.TaskID {
			selected = route
			break
		}
	}
	if shouldUpgradeBuilderRuntimeFromStart(run, selected) && run.BuilderRuntime.UpgradeModel.Primary != "" {
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
	if run.BuilderRuntime == nil || run.BuilderRuntime.UpgradeModel.Primary == "" {
		return false
	}
	if stats == nil || stats.UpgradeApplied || stats.SelectedModel == run.BuilderRuntime.UpgradeModel.Primary || stats.Attempts >= 2 {
		return false
	}
	switch reason {
	case "parse_failure":
		return run.BuilderRuntime.UpgradeThreshold.UpgradeOnPatchParseFail
	case "scope_violation":
		return run.BuilderRuntime.UpgradeThreshold.UpgradeOnScopeViolation
	default:
		return false
	}
}

func dominantBuilderRuntimeTask(tasks []appruns.TaskBundleItem) appruns.TaskBundleItem {
	if len(tasks) == 0 {
		return appruns.TaskBundleItem{}
	}
	selected := appruns.NormalizeTaskBundleItem(tasks[0])
	selectedScore := builderRuntimeTaskPriority(selected.EffectiveTaskType())
	for _, candidate := range tasks[1:] {
		normalized := appruns.NormalizeTaskBundleItem(candidate)
		score := builderRuntimeTaskPriority(normalized.EffectiveTaskType())
		if score > selectedScore {
			selected = normalized
			selectedScore = score
		}
	}
	return selected
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
