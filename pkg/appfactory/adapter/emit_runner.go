package adapter

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/sipeed/picoclaw/pkg/appfactory/emitter"
	appprepare "github.com/sipeed/picoclaw/pkg/appfactory/prepare"
	appruns "github.com/sipeed/picoclaw/pkg/appfactory/runs"
)

// emitRunnerResult 是 tryDeterministicEmit 的返回结果。
type emitRunnerResult struct {
	Patch       *appruns.WorkspacePatch
	ApplyResult appruns.WorkspacePatchApplyResult
	Handled     bool
}

// tryDeterministicEmit 尝试用确定性 Emitter 生成 workspace patch。
// 当选中的 task route_hint 为 deterministic 时调用。
// 返回 Handled=true 表示成功完成（调用方应跳过 LLM），
// 返回 Handled=false 表示 emitter 不适用或失败（调用方应 fallback 到 LLM）。
func tryDeterministicEmit(run runRecord, roundInput appruns.RoundInput, patchID string) (emitRunnerResult, error) {
	selectedTask := preferredBuilderRuntimeTask(run.TaskBundle, run.RoundState)
	if appruns.NormalizeTaskRouteHint(string(selectedTask.RouteHint)) != appruns.TaskRouteHintDeterministic {
		return emitRunnerResult{}, nil
	}

	dm, err := loadPrepareDomainModel(run.WorkspacePath)
	if err != nil {
		return emitRunnerResult{}, fmt.Errorf("load domain model for deterministic emit: %w", err)
	}

	targetSet := normalizeTargetPaths(selectedTask.TargetPaths)
	slotMap, err := loadPrepareTemplateSlotMap(run.WorkspacePath)
	if err != nil {
		return emitRunnerResult{}, fmt.Errorf("load template slot map for deterministic emit: %w", err)
	}
	matchedSlots := resolveDeterministicTemplateSlots(selectedTask, slotMap, targetSet)
	relationRichProfile := deterministicRelationRichModelProfile(dm)

	var planningContext appprepare.PlanningContext
	if deterministicSlotsRequirePlanningContext(matchedSlots) {
		planningContext, err = loadPreparePlanningContext(run.WorkspacePath)
		if err != nil {
			return emitRunnerResult{}, fmt.Errorf("load planning context for deterministic emit: %w", err)
		}
	}
	var ops []appruns.WorkspacePatchOperation
	ops = append(ops, deterministicRelationModelOperations(dm, selectedTask.TargetPaths)...)
	ops = append(ops, deterministicTemplateSlotOperations(matchedSlots, deterministicTemplateSlotContext{
		run:                 run,
		dm:                  dm,
		planningContext:     planningContext,
		relationRichProfile: relationRichProfile,
		targetSet:           targetSet,
	})...)

	if len(ops) == 0 {
		return emitRunnerResult{}, nil
	}

	patch := &appruns.WorkspacePatch{
		PatchID:    patchID,
		Status:     "emitted",
		Operations: ops,
	}

	applyResult, applyErr := appruns.ApplyWorkspacePatch(run.WorkspacePath, run.AllowedPaths, run.ProtectedPaths, *patch)
	if applyErr != nil {
		return emitRunnerResult{}, fmt.Errorf("apply deterministic emit patch: %w", applyErr)
	}
	if applyResult.Status != "" {
		patch.Status = applyResult.Status
	}
	patch.ModifiedFiles = modifiedFilePaths(applyResult.ModifiedFiles)

	return emitRunnerResult{
		Patch:       patch,
		ApplyResult: applyResult,
		Handled:     true,
	}, nil
}

// loadPrepareDomainModel 从 job 根目录的 prepare/domain-model.json 加载 DomainModel。
func loadPrepareDomainModel(workspacePath string) (appprepare.DomainModel, error) {
	jobRoot := filepath.Dir(workspacePath)
	dmPath := filepath.Join(jobRoot, "prepare", "domain-model.json")
	content, err := os.ReadFile(dmPath)
	if err != nil {
		return appprepare.DomainModel{}, fmt.Errorf("read %s: %w", dmPath, err)
	}
	var dm appprepare.DomainModel
	if err := json.Unmarshal(content, &dm); err != nil {
		return appprepare.DomainModel{}, fmt.Errorf("parse domain-model.json: %w", err)
	}
	return dm, nil
}

func loadPreparePlanningContext(workspacePath string) (appprepare.PlanningContext, error) {
	jobRoot := filepath.Dir(workspacePath)
	planningContextPath := filepath.Join(jobRoot, "prepare", "planning-context.json")
	content, err := os.ReadFile(planningContextPath)
	if err != nil {
		return appprepare.PlanningContext{}, fmt.Errorf("read %s: %w", planningContextPath, err)
	}
	var planningContext appprepare.PlanningContext
	if err := json.Unmarshal(content, &planningContext); err != nil {
		return appprepare.PlanningContext{}, fmt.Errorf("parse planning-context.json: %w", err)
	}
	return planningContext, nil
}

func loadPrepareTemplateSlotMap(workspacePath string) (appprepare.TemplateSlotMap, error) {
	jobRoot := filepath.Dir(workspacePath)
	templateSlotMapPath := filepath.Join(jobRoot, "prepare", "template-slot-map.json")
	content, err := os.ReadFile(templateSlotMapPath)
	if err != nil {
		return appprepare.TemplateSlotMap{}, fmt.Errorf("read %s: %w", templateSlotMapPath, err)
	}
	var slotMap appprepare.TemplateSlotMap
	if err := json.Unmarshal(content, &slotMap); err != nil {
		return appprepare.TemplateSlotMap{}, fmt.Errorf("parse template-slot-map.json: %w", err)
	}
	return slotMap, nil
}

type deterministicTemplateSlotContext struct {
	run                 runRecord
	dm                  appprepare.DomainModel
	planningContext     appprepare.PlanningContext
	relationRichProfile builderRuntimeRelationRichModelProfile
	targetSet           map[string]struct{}
}

func deterministicRelationModelOperations(dm appprepare.DomainModel, targetPaths []string) []appruns.WorkspacePatchOperation {
	result, emitted := emitter.EmitRelationModels(dm)
	if !emitted {
		return nil
	}
	ops := make([]appruns.WorkspacePatchOperation, 0, len(targetPaths))
	for _, targetPath := range targetPaths {
		normalizedTargetPath := filepath.ToSlash(strings.TrimSpace(targetPath))
		if content, ok := result.Files[normalizedTargetPath]; ok {
			ops = append(ops, appruns.WorkspacePatchOperation{
				Type:    "write_file",
				Path:    normalizedTargetPath,
				Content: content,
			})
		}
	}
	return ops
}

func resolveDeterministicTemplateSlots(task appruns.TaskBundleItem, slotMap appprepare.TemplateSlotMap, targetSet map[string]struct{}) []appprepare.TemplateSlot {
	if len(slotMap.Slots) == 0 {
		return nil
	}
	bindingRefs := deterministicTaskBindingRefSet(task)
	matched := make([]appprepare.TemplateSlot, 0, len(slotMap.Slots))
	seen := make(map[string]struct{}, len(slotMap.Slots))
	appendSlot := func(slot appprepare.TemplateSlot) {
		key := strings.TrimSpace(slot.SlotID)
		if key == "" {
			key = strings.TrimSpace(slot.BindingID) + ":" + strings.TrimSpace(slot.SlotKind)
		}
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		matched = append(matched, slot)
	}
	if len(bindingRefs) > 0 {
		for _, slot := range slotMap.Slots {
			if _, ok := bindingRefs[strings.TrimSpace(slot.BindingID)]; ok {
				appendSlot(slot)
			}
		}
	}
	if len(matched) == 0 {
		for _, slot := range slotMap.Slots {
			if deterministicTemplateSlotMatchesTargets(slot, targetSet) {
				appendSlot(slot)
			}
		}
	}
	return matched
}

func deterministicTaskBindingRefSet(task appruns.TaskBundleItem) map[string]struct{} {
	if task.AllocationTransition == nil || len(task.AllocationTransition.BindingRefs) == 0 {
		return nil
	}
	refs := make(map[string]struct{}, len(task.AllocationTransition.BindingRefs))
	for _, bindingRef := range task.AllocationTransition.BindingRefs {
		trimmed := strings.TrimSpace(bindingRef)
		if trimmed == "" {
			continue
		}
		refs[trimmed] = struct{}{}
	}
	if len(refs) == 0 {
		return nil
	}
	return refs
}

func deterministicTemplateSlotMatchesTargets(slot appprepare.TemplateSlot, targetSet map[string]struct{}) bool {
	if len(targetSet) == 0 || len(slot.TargetPaths) == 0 {
		return false
	}
	for _, targetPath := range slot.TargetPaths {
		normalizedTargetPath := filepath.ToSlash(strings.TrimSpace(targetPath))
		if _, ok := targetSet[normalizedTargetPath]; ok {
			return true
		}
	}
	return false
}

func deterministicSlotsRequirePlanningContext(slots []appprepare.TemplateSlot) bool {
	for _, slot := range slots {
		if normalizeDeterministicSlotKind(slot.SlotKind) == "app_entry" {
			return true
		}
	}
	return false
}

func deterministicTemplateSlotOperations(slots []appprepare.TemplateSlot, ctx deterministicTemplateSlotContext) []appruns.WorkspacePatchOperation {
	if len(slots) == 0 {
		return nil
	}
	ops := make([]appruns.WorkspacePatchOperation, 0, len(slots))
	seenPaths := make(map[string]struct{}, len(slots))
	appendIfTargeted := func(path, content string) {
		normalizedPath := filepath.ToSlash(strings.TrimSpace(path))
		if normalizedPath == "" || strings.TrimSpace(content) == "" {
			return
		}
		if _, ok := ctx.targetSet[normalizedPath]; !ok {
			return
		}
		if _, ok := seenPaths[normalizedPath]; ok {
			return
		}
		seenPaths[normalizedPath] = struct{}{}
		ops = append(ops, appruns.WorkspacePatchOperation{
			Type:    "write_file",
			Path:    normalizedPath,
			Content: content,
		})
	}
	for _, slot := range slots {
		if !deterministicTemplateSlotCanEmit(slot) {
			continue
		}
		switch normalizeDeterministicSlotKind(slot.SlotKind) {
		case "summary":
			if result, emitted := emitter.EmitOverview(ctx.dm); emitted {
				appendIfTargeted(result.HomePagePath, result.HomePageContent)
				appendIfTargeted(result.HomeControllerPath, result.HomeControllerContent)
			}
		case "list":
			if result, emitted := emitter.EmitCollection(ctx.dm); emitted {
				appendIfTargeted(result.ListPagePath, result.ListPageContent)
				appendIfTargeted(result.ListControllerPath, result.ListControllerContent)
			}
		case "form":
			if result, emitted := emitter.EmitMutation(ctx.dm); emitted {
				appendIfTargeted(result.FormPagePath, result.FormPageContent)
				appendIfTargeted(result.FormControllerPath, result.FormControllerContent)
			}
		case "detail":
			if result, emitted := emitter.EmitInspection(ctx.dm); emitted {
				appendIfTargeted(result.DetailPagePath, result.DetailPageContent)
			}
		case "app_entry":
			if result, emitted := emitter.EmitAppEntry(ctx.dm, ctx.planningContext); emitted {
				appendIfTargeted(result.FilePath, result.Content)
			}
		case "copy":
			if result, emitted := emitter.EmitCopy(ctx.dm); emitted {
				appendIfTargeted(result.FilePath, result.Content)
			}
		case "branding":
			if result, emitted := emitter.EmitBrand(ctx.dm); emitted {
				appendIfTargeted(result.StringsXMLPath, result.StringsXML)
			}
			if result, emitted := emitter.EmitAndroidBuildConfig(ctx.dm); emitted {
				appendIfTargeted(result.FilePath, result.Content)
			}
		case "storage":
			if content := builderRuntimeCanonicalRelationRichRecordRepository(ctx.relationRichProfile); strings.TrimSpace(content) != "" {
				appendIfTargeted("lib/repositories/record_repository.dart", content)
			}
		case "test":
			cfg := buildTestEmitConfig(ctx.run)
			if result, emitted := emitter.EmitTest(ctx.dm, cfg); emitted {
				appendIfTargeted(result.FilePath, result.Content)
			}
		}
	}
	return ops
}

func deterministicTemplateSlotCanEmit(slot appprepare.TemplateSlot) bool {
	if !slot.EmitEligible {
		return false
	}
	slotKind := normalizeDeterministicSlotKind(slot.SlotKind)
	overridePolicy := strings.ToLower(strings.TrimSpace(slot.OverridePolicy))
	switch slotKind {
	case "summary", "list", "form", "detail", "app_entry":
		return overridePolicy == "replace" || overridePolicy == "synchronize"
	case "copy", "branding", "test":
		return overridePolicy == "synchronize" || overridePolicy == "replace"
	case "storage":
		return overridePolicy == "extend" || overridePolicy == "replace" || overridePolicy == "synchronize"
	default:
		return false
	}
}

func normalizeDeterministicSlotKind(slotKind string) string {
	normalized := strings.ToLower(strings.TrimSpace(slotKind))
	return strings.ReplaceAll(normalized, "-", "_")
}

// normalizeTargetPaths 将 TargetPaths 转为 set（统一正斜杠并去除空白）。
func normalizeTargetPaths(paths []string) map[string]struct{} {
	set := make(map[string]struct{}, len(paths))
	for _, p := range paths {
		normalized := filepath.ToSlash(strings.TrimSpace(p))
		if normalized != "" {
			set[normalized] = struct{}{}
		}
	}
	return set
}

func deterministicRelationRichModelProfile(dm appprepare.DomainModel) builderRuntimeRelationRichModelProfile {
	entityIDs := make(map[string]struct{}, len(dm.Entities))
	for _, entity := range dm.Entities {
		entityIDs[strings.TrimSpace(entity.EntityID)] = struct{}{}
	}
	if _, ok := entityIDs["entity-project"]; ok {
		if _, ok := entityIDs["entity-task"]; ok {
			if _, ok := entityIDs["entity-tag"]; ok {
				return builderRuntimeRelationRichProjectTaskTagProfile
			}
		}
	}
	if _, ok := entityIDs["entity-inventory-sheet"]; ok {
		if _, ok := entityIDs["entity-line-item"]; ok {
			if _, ok := entityIDs["entity-sku"]; ok {
				return builderRuntimeRelationRichInventorySheetLineProfile
			}
		}
	}
	return ""
}

// buildTestEmitConfig 从 runRecord 构建 TestEmitConfig，使用默认值。
func buildTestEmitConfig(run runRecord) emitter.TestEmitConfig {
	// 默认值在 EmitTest 内部处理，此处只提供零值即可。
	// 未来可从 workspace 的 pubspec.yaml / main.dart 提取动态值。
	_ = run
	return emitter.TestEmitConfig{}
}
