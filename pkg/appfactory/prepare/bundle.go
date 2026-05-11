package prepare

import (
	"time"

	appruns "github.com/sipeed/oneappfactory/pkg/appfactory/runs"
)

func buildBundle(spec domainSpec, prd PRD, now time.Time) (Bundle, error) {
	taskBundle := normalizePreparedTaskBundle(spec.TaskBundle)
	if prd.ExecutionContract == nil {
		prd.ExecutionContract = buildExecutionContract(spec, prd, taskBundle)
	}
	prdJSON, err := marshalJSON(prd)
	if err != nil {
		return Bundle{}, err
	}
	prdMarkdown := renderPRDMarkdown(prd)
	fitReport := renderTemplateFitReport(spec)
	runtimeConfig := buildRuntimeConfig(spec)
	builderInput := appruns.BuildInput{
		SchemaVersion:                  appruns.CurrentBuildInputSchemaVersion,
		JobID:                          spec.JobID,
		PRDID:                          spec.PRDID,
		TemplateID:                     spec.TemplateID,
		PlanningPolicy:                 currentPlanningPolicySnapshot(),
		PreparedPRDSubjectVersion:      PRDCompileSourceVersion(prd),
		PreparedTemplateSubjectVersion: TemplateCompileSourceVersion(spec.TemplateID, spec.TemplatePinnedRef),
		ExecutorImage:                  runtimeConfig.ExecutorImage,
		WorkspacePath:                  runtimeConfig.WorkspacePath,
		ArtifactDir:                    runtimeConfig.ArtifactDir,
		GoalSummary:                    runtimeConfig.GoalSummary,
		TaskBundle:                     taskBundle,
		AcceptanceChecks:               spec.AcceptanceChecks,
		AllowedPaths:                   append([]string(nil), spec.PreferredAllowedPaths...),
		ProtectedPaths:                 append([]string(nil), spec.PreferredProtectedPaths...),
		KnowledgePack:                  append([]appruns.ProfileSkill(nil), runtimeConfig.KnowledgePack...),
		CommandProfile:                 runtimeConfig.CommandProfile,
		ContextFiles:                   runtimeConfig.ContextFiles,
		IterationBudget:                runtimeConfig.IterationBudget,
		TokenBudget:                    runtimeConfig.TokenBudget,
		HumanNotes:                     runtimeConfig.HumanNotes,
	}
	builderInputJSON, err := marshalJSON(builderInput)
	if err != nil {
		return Bundle{}, err
	}
	planningContextJSON, err := marshalJSON(buildPlanningContext(spec, prd, builderInput.PlanningPolicy))
	if err != nil {
		return Bundle{}, err
	}
	domainModelJSON, err := marshalJSON(buildDomainModel(spec))
	if err != nil {
		return Bundle{}, err
	}
	templateSlotMapJSON, err := marshalJSON(buildTemplateSlotMap(spec))
	if err != nil {
		return Bundle{}, err
	}
	taskAllocationJSON, err := marshalJSON(buildTaskAllocation(taskBundle, spec))
	if err != nil {
		return Bundle{}, err
	}
	acceptancePlanJSON, err := marshalJSON(buildAcceptancePlan(spec))
	if err != nil {
		return Bundle{}, err
	}
	runtimeConfigJSON, err := marshalJSON(runtimeConfig)
	if err != nil {
		return Bundle{}, err
	}
	requirement := renderRequirement(spec)
	prdApprovalJSON, err := marshalJSON(buildPRDApprovalRecord(spec, prd, prdMarkdown, requirement, now))
	if err != nil {
		return Bundle{}, err
	}
	templateApprovalJSON, err := marshalJSON(buildTemplateApprovalRecord(spec, prd, fitReport, now))
	if err != nil {
		return Bundle{}, err
	}
	files := map[string][]byte{
		requirementFileName:      []byte(requirement),
		prdMarkdownFileName:      []byte(prdMarkdown),
		prdJSONFileName:          prdJSON,
		prdApprovalFileName:      prdApprovalJSON,
		templateApprovalFileName: templateApprovalJSON,
		fitReportFileName:        []byte(fitReport),
		planFileName:             []byte(renderImplementationPlan(spec)),
		constraintsFileName:      []byte(renderManualConstraints(spec)),
		planningContextFileName:  planningContextJSON,
		domainModelFileName:      domainModelJSON,
		templateSlotMapFileName:  templateSlotMapJSON,
		taskAllocationFileName:   taskAllocationJSON,
		acceptancePlanFileName:   acceptancePlanJSON,
		runtimeConfigFileName:    runtimeConfigJSON,
		builderInputFileName:     builderInputJSON,
	}
	return Bundle{PRD: prd, BuilderInput: builderInput, Files: files}, nil
}
