package prepare

import (
	"strings"

	appruns "github.com/sipeed/picoclaw/pkg/appfactory/runs"
)

// ProjectBuildInput 从 6 个中间制品重建 builder-input。
// 这是 builder-input 降级为投影的核心函数。
func ProjectBuildInput(
	pc PlanningContext,
	dm DomainModel,
	tsm TemplateSlotMap,
	ta TaskAllocation,
	ap AcceptancePlan,
	rc RuntimeConfig,
) appruns.BuildInput {
	return appruns.BuildInput{
		SchemaVersion:                  appruns.CurrentBuildInputSchemaVersion,
		JobID:                          pc.JobID,
		PRDID:                          pc.PRDID,
		TemplateID:                     pc.TemplateID,
		PlanningPolicy:                 pc.PlanningPolicy,
		PreparedPRDSubjectVersion:      pc.PRDSubjectVersion,
		PreparedTemplateSubjectVersion: pc.TemplateSubjectVersion,
		ExecutorImage:                  rc.ExecutorImage,
		WorkspacePath:                  rc.WorkspacePath,
		ArtifactDir:                    rc.ArtifactDir,
		GoalSummary:                    rc.GoalSummary,
		TaskBundle:                     ProjectTaskBundle(ta),
		AcceptanceChecks:               ProjectAcceptanceChecks(ap),
		AllowedPaths:                   append([]string(nil), pc.ExecutionRouteSnapshot.AllowedPaths...),
		ProtectedPaths:                 append([]string(nil), pc.ExecutionRouteSnapshot.ProtectedPaths...),
		KnowledgePack:                  append([]appruns.ProfileSkill(nil), rc.KnowledgePack...),
		CommandProfile:                 rc.CommandProfile,
		ContextFiles:                   rc.ContextFiles,
		IterationBudget:                rc.IterationBudget,
		TokenBudget:                    rc.TokenBudget,
		HumanNotes:                     rc.HumanNotes,
	}
}

// ProjectTaskBundle 从 TaskAllocation 投影出 TaskBundleItem 列表。
func ProjectTaskBundle(ta TaskAllocation) []appruns.TaskBundleItem {
	if len(ta.Units) == 0 {
		return nil
	}
	items := make([]appruns.TaskBundleItem, 0, len(ta.Units))
	for _, unit := range ta.Units {
		transition := &appruns.TaskAllocationTransition{
			AllocationID:        unit.AllocationID,
			BindingRefs:         append([]string(nil), unit.BindingRefs...),
			SurfaceRefs:         append([]string(nil), unit.SurfaceRefs...),
			ScreenRefs:          append([]string(nil), unit.ScreenRefs...),
			EntityRefs:          append([]string(nil), unit.EntityRefs...),
			RelationGroupRefs:   append([]string(nil), unit.RelationGroupRefs...),
			SharedOwnershipRefs: append([]string(nil), unit.SharedOwnershipRefs...),
			SemanticIntentRefs:  append([]string(nil), unit.SemanticIntentRefs...),
			OwnedPaths:          append([]string(nil), unit.OwnedPaths...),
			BlockedBy:           append([]string(nil), unit.BlockedBy...),
			SuccessEvidence:     append([]string(nil), unit.SuccessEvidence...),
		}
		items = append(items, appruns.TaskBundleItem{
			TaskID:               unit.AllocationID,
			Title:                unit.Title,
			Category:             appruns.TaskCategory(unit.Lane),
			TaskType:             unit.TaskType,
			RiskLevel:            unit.RiskLevel,
			RouteHint:            unit.RouteHint,
			AllocationTransition: transition,
			Objective:            unit.Objective,
			Priority:             unit.Priority,
			RelatedRequirements:  append([]string(nil), unit.RelatedRequirements...),
			Dependencies:         append([]string(nil), unit.BlockedBy...),
			TargetPaths:          append([]string(nil), unit.TargetPaths...),
			OutputExpectations:   append([]string(nil), unit.OutputExpectations...),
			CompletionCriteria:   append([]string(nil), unit.SuccessEvidence...),
			RiskNotes:            append([]string(nil), unit.RiskNotes...),
		})
	}
	return items
}

// ProjectAcceptanceChecks 从 AcceptancePlan 投影出 AcceptanceCheck 列表。
func ProjectAcceptanceChecks(ap AcceptancePlan) []appruns.AcceptanceCheck {
	var checks []appruns.AcceptanceCheck
	for _, item := range ap.StructureChecks {
		checks = append(checks, projectAcceptanceCheckItem(item))
	}
	for _, item := range ap.DeliveryChecks {
		checks = append(checks, projectAcceptanceCheckItem(item))
	}
	return checks
}

func projectAcceptanceCheckItem(item AcceptancePlanItem) appruns.AcceptanceCheck {
	return appruns.AcceptanceCheck{
		CheckID:         strings.TrimSpace(item.CheckID),
		Label:           strings.TrimSpace(item.Label),
		Stage:           item.Stage,
		Required:        item.Required,
		Commands:        append([]string(nil), item.Commands...),
		SuccessCriteria: strings.TrimSpace(item.Description),
		TimeoutSeconds:  item.TimeoutSeconds,
	}
}
