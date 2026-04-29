package prepare

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	appruns "github.com/sipeed/picoclaw/pkg/appfactory/runs"
)

const (
	defaultSchemaVersion     = "0.1.0"
	requirementFileName      = "requirement.md"
	prdMarkdownFileName      = "PRD.md"
	prdJSONFileName          = "PRD.json"
	prdApprovalFileName      = appruns.PRDApprovalFileName
	templateApprovalFileName = appruns.TemplateApprovalFileName
	fitReportFileName        = "template-fit-report.md"
	planFileName             = "implementation-plan.md"
	constraintsFileName      = "manual-constraints.md"
	planningContextFileName  = "planning-context.json"
	domainModelFileName      = "domain-model.json"
	templateSlotMapFileName  = "template-slot-map.json"
	taskAllocationFileName   = "task-allocation.json"
	acceptancePlanFileName   = "acceptance-plan.json"
	runtimeConfigFileName   = "runtime-config.json"
	builderInputFileName     = "builder-input.json"
	publicBindingAppEntry    = "app-entry"
	publicBindingDomainModel = "domain-model"
	publicBindingDomainCopy  = "domain-copy"
	publicBindingBranding    = "android-branding"
	publicBindingStorage     = "storage-boundary"
	publicBindingFlow        = "flow-controller"
	publicBindingWidgetTest  = "widget-test"
	publicBindingManifest    = "dependency-manifest"
	publicBindingSurface     = "interaction-surface"
)

type Request struct {
	RequirementText   string
	RequirementSource string
	TitleHint         string
	JobID             string
	PRDID             string
	TemplateID        string
	ExecutorImage     string
	RealBuild         bool
	Now               func() time.Time
}

type RecompileRequest struct {
	RequirementText   string
	RequirementSource string
	PRD               PRD
	JobID             string
	TemplateID        string
	ExecutorImage     string
	Now               func() time.Time
}

type Bundle struct {
	PRD          PRD
	BuilderInput appruns.BuildInput
	Files        map[string][]byte
}

type PlanningContext struct {
	SchemaVersion          string                         `json:"schema_version"`
	JobID                  string                         `json:"job_id"`
	PRDID                  string                         `json:"prd_id"`
	TemplateID             string                         `json:"template_id"`
	PRDSubjectVersion      string                         `json:"prd_subject_version"`
	TemplateSubjectVersion string                         `json:"template_subject_version"`
	PlanningPolicyVersion  string                         `json:"planning_policy_version,omitempty"`
	PlanningModelSnapshot  PlanningModelSnapshot          `json:"planning_model_snapshot,omitempty"`
	ExecutionRouteSnapshot ExecutionRouteSnapshot         `json:"execution_route_snapshot,omitempty"`
	PlanningPolicy         appruns.PlanningPolicySnapshot `json:"planning_policy"`
	HumanNotes             []map[string]string            `json:"human_notes,omitempty"`
	ManualConstraints      []string                       `json:"manual_constraints,omitempty"`
	RequirementHighlights  []string                       `json:"requirement_highlights,omitempty"`
	SupportingAssumptions  []string                       `json:"supporting_assumptions,omitempty"`
}

type PlanningModelSnapshot struct {
	RequirementStructuring string `json:"requirement_structuring,omitempty"`
	DomainModeling         string `json:"domain_modeling,omitempty"`
	TaskAllocation         string `json:"task_allocation,omitempty"`
	AcceptancePlanning     string `json:"acceptance_planning,omitempty"`
	BuildInputProjection   string `json:"build_input_projection,omitempty"`
}

type ExecutionRouteSnapshot struct {
	DefaultRouteHint  appruns.TaskRouteHint  `json:"default_route_hint,omitempty"`
	RouteHintCounts   map[string]int         `json:"route_hint_counts,omitempty"`
	TaskRoutes        []ExecutionTaskRoute   `json:"task_routes,omitempty"`
	UpgradePolicy     ExecutionUpgradePolicy `json:"upgrade_policy,omitempty"`
	StartUpgradeCheck StartUpgradeCheck      `json:"start_upgrade_check,omitempty"`
	AllowedPaths      []string               `json:"allowed_paths,omitempty"`
	ProtectedPaths    []string               `json:"protected_paths,omitempty"`
	CommandProfileRef string                 `json:"command_profile_ref,omitempty"`
}

type ExecutionUpgradePolicy struct {
	Source                       string   `json:"source,omitempty"`
	MaxAttemptsBeforeUpgrade     int      `json:"max_attempts_before_upgrade,omitempty"`
	MaxFilesBeforeUpgrade        int      `json:"max_files_before_upgrade,omitempty"`
	MaxSchemaDriftBeforeUpgrade  int      `json:"max_schema_drift_before_upgrade,omitempty"`
	MaxUnrelatedOperationRate    float64  `json:"max_unrelated_operation_rate,omitempty"`
	UpgradeOnValidationFail      bool     `json:"upgrade_on_validation_fail,omitempty"`
	UpgradeOnPatchParseFail      bool     `json:"upgrade_on_patch_parse_fail,omitempty"`
	UpgradeOnScopeViolation      bool     `json:"upgrade_on_scope_violation,omitempty"`
	UpgradeOnSemanticConflict    bool     `json:"upgrade_on_semantic_conflict,omitempty"`
	SupportedRetryUpgradeSignals []string `json:"supported_retry_upgrade_signals,omitempty"`
}

type StartUpgradeCheck struct {
	CurrentAttempt          int      `json:"current_attempt,omitempty"`
	ConcreteTargetFileCount int      `json:"concrete_target_file_count,omitempty"`
	WouldUpgradeFromStart   bool     `json:"would_upgrade_from_start,omitempty"`
	TriggeredReasons        []string `json:"triggered_reasons,omitempty"`
}

type ExecutionTaskRoute struct {
	AllocationID string                         `json:"allocation_id"`
	Category     appruns.TaskCategory           `json:"category,omitempty"`
	TaskType     appruns.BuilderRuntimeTaskType `json:"task_type,omitempty"`
	RouteHint    appruns.TaskRouteHint          `json:"route_hint,omitempty"`
	RiskLevel    appruns.TaskRiskLevel          `json:"risk_level,omitempty"`
	TargetPaths  []string                       `json:"target_paths,omitempty"`
}

type DomainModel struct {
	SchemaVersion           string                   `json:"schema_version"`
	JobID                   string                   `json:"job_id"`
	PRDID                   string                   `json:"prd_id"`
	TemplateID              string                   `json:"template_id"`
	DomainName              string                   `json:"domain_name"`
	Entities                []DataEntity             `json:"entities,omitempty"`
	SummaryMetrics          []string                 `json:"summary_metrics,omitempty"`
	DomainCopy              DomainCopy               `json:"domain_copy"`
	CriticalFlows           []string                 `json:"critical_flows,omitempty"`
	SemanticAcceptanceRules []SemanticAcceptanceRule `json:"semantic_acceptance_rules,omitempty"`
}

type TemplateSlotMap struct {
	SchemaVersion          string         `json:"schema_version"`
	TemplateID             string         `json:"template_id"`
	TemplateSubjectVersion string         `json:"template_subject_version,omitempty"`
	Slots                  []TemplateSlot `json:"slots,omitempty"`
}

type TemplateSlot struct {
	BindingID         string   `json:"binding_id"`
	SlotID            string   `json:"slot_id"`
	SlotKind          string   `json:"slot_kind"`
	TargetPaths       []string `json:"target_paths,omitempty"`
	OverridePolicy    string   `json:"override_policy"`
	RequiredInputs    []string `json:"required_inputs,omitempty"`
	AcceptanceImpacts []string `json:"acceptance_impacts,omitempty"`
	EmitEligible      bool     `json:"emit_eligible"`
}

type DomainCopy struct {
	Title            string `json:"title,omitempty"`
	Summary          string `json:"summary,omitempty"`
	ProblemStatement string `json:"problem_statement,omitempty"`
}

type SemanticAcceptanceRule struct {
	RuleID          string   `json:"rule_id"`
	Label           string   `json:"label"`
	Description     string   `json:"description"`
	FieldRefs       []string `json:"field_refs,omitempty"`
	EvidencePattern string   `json:"evidence_pattern,omitempty"`
}

type TaskAllocation struct {
	SchemaVersion string               `json:"schema_version"`
	JobID         string               `json:"job_id"`
	PRDID         string               `json:"prd_id"`
	TemplateID    string               `json:"template_id"`
	Units         []TaskAllocationUnit `json:"units,omitempty"`
}

type TaskAllocationUnit struct {
	AllocationID        string                         `json:"allocation_id"`
	Title               string                         `json:"title,omitempty"`
	Wave                int                            `json:"wave"`
	Lane                string                         `json:"lane,omitempty"`
	Priority            string                         `json:"priority,omitempty"`
	BindingRefs         []string                       `json:"slot_refs,omitempty"`
	SurfaceRefs         []string                       `json:"surface_refs,omitempty"`
	ScreenRefs          []string                       `json:"screen_refs,omitempty"`
	EntityRefs          []string                       `json:"entity_refs,omitempty"`
	RelationGroupRefs   []string                       `json:"relation_group_refs,omitempty"`
	SharedOwnershipRefs []string                       `json:"shared_ownership_refs,omitempty"`
	TaskType            appruns.BuilderRuntimeTaskType `json:"task_type,omitempty"`
	Objective           string                         `json:"objective,omitempty"`
	SemanticIntentRefs  []string                       `json:"semantic_intent_refs,omitempty"`
	RelatedRequirements []string                       `json:"related_requirements,omitempty"`
	TargetPaths         []string                       `json:"target_paths,omitempty"`
	OwnedPaths          []string                       `json:"owned_paths,omitempty"`
	BlockedBy           []string                       `json:"blocked_by,omitempty"`
	SuccessEvidence     []string                       `json:"success_evidence,omitempty"`
	OutputExpectations  []string                       `json:"output_expectations,omitempty"`
	RiskNotes           []string                       `json:"risk_notes,omitempty"`
	RiskLevel           appruns.TaskRiskLevel          `json:"risk_level,omitempty"`
	RouteHint           appruns.TaskRouteHint          `json:"route_hint,omitempty"`
}

type AcceptancePlan struct {
	SchemaVersion   string               `json:"schema_version"`
	JobID           string               `json:"job_id"`
	PRDID           string               `json:"prd_id"`
	TemplateID      string               `json:"template_id"`
	StructureChecks []AcceptancePlanItem `json:"structure_checks,omitempty"`
	SemanticChecks  []AcceptancePlanItem `json:"semantic_checks,omitempty"`
	BehaviorChecks  []AcceptancePlanItem `json:"behavior_checks,omitempty"`
	DeliveryChecks  []AcceptancePlanItem `json:"delivery_checks,omitempty"`
}

type AcceptancePlanItem struct {
	CheckID         string                 `json:"check_id"`
	Label           string                 `json:"label"`
	Description     string                 `json:"description,omitempty"`
	Stage           appruns.ExecutionStage `json:"stage,omitempty"`
	Commands        []string               `json:"commands,omitempty"`
	Required        bool                   `json:"required,omitempty"`
	SourceType      string                 `json:"source_type,omitempty"`
	SourceRef       string                 `json:"source_ref,omitempty"`
	BindingRefs     []string               `json:"slot_refs,omitempty"`
	FieldRefs       []string               `json:"field_refs,omitempty"`
	EvidencePattern string                 `json:"evidence_pattern,omitempty"`
	TimeoutSeconds  int                    `json:"timeout_seconds,omitempty"`
}

type RuntimeConfig struct {
	SchemaVersion   string                   `json:"schema_version"`
	ExecutorImage   string                   `json:"executor_image,omitempty"`
	WorkspacePath   string                   `json:"workspace_path"`
	ArtifactDir     string                   `json:"artifact_dir"`
	GoalSummary     string                   `json:"goal_summary"`
	KnowledgePack   []appruns.ProfileSkill   `json:"knowledge_pack,omitempty"`
	CommandProfile  json.RawMessage          `json:"command_profile"`
	ContextFiles    json.RawMessage          `json:"context_files"`
	IterationBudget int                      `json:"iteration_budget"`
	TokenBudget     int                      `json:"token_budget"`
	HumanNotes      json.RawMessage          `json:"human_notes,omitempty"`
}

type PRD struct {
	SchemaVersion       string                `json:"schema_version"`
	ID                  string                `json:"id"`
	Version             string                `json:"version"`
	Status              string                `json:"status"`
	Title               string                `json:"title"`
	Summary             string                `json:"summary"`
	ProblemStatement    string                `json:"problem_statement"`
	TargetUsers         []UserProfile         `json:"target_users"`
	CoreScenarios       []Scenario            `json:"core_scenarios"`
	Goals               []string              `json:"goals"`
	NonGoals            []string              `json:"non_goals"`
	FeatureList         []Feature             `json:"feature_list"`
	SurfaceList         []InteractionSurface  `json:"surface_list,omitempty"`
	ScreenList          []Screen              `json:"screen_list,omitempty"`
	UserFlows           []UserFlow            `json:"user_flows"`
	DataEntities        []DataEntity          `json:"data_entities"`
	TemplateConstraints TemplateConstraints   `json:"template_constraints"`
	AcceptanceCriteria  []AcceptanceCriterion `json:"acceptance_criteria"`
	ManualReviewPoints  []ManualReviewPoint   `json:"manual_review_points"`
	KnownUnknowns       []KnownUnknown        `json:"known_unknowns"`
	ExecutionContract   *ExecutionContract    `json:"execution_contract,omitempty"`
	SourceRefs          []string              `json:"source_refs,omitempty"`
	CreatedAt           string                `json:"created_at,omitempty"`
	UpdatedAt           string                `json:"updated_at,omitempty"`
}

type UserProfile struct {
	UserID     string   `json:"user_id"`
	Label      string   `json:"label"`
	Summary    string   `json:"summary"`
	PainPoints []string `json:"pain_points,omitempty"`
}

type Scenario struct {
	ScenarioID      string   `json:"scenario_id"`
	Title           string   `json:"title"`
	Summary         string   `json:"summary"`
	PrimaryUserRefs []string `json:"primary_user_refs,omitempty"`
}

type Feature struct {
	FeatureID          string   `json:"feature_id"`
	Title              string   `json:"title"`
	Summary            string   `json:"summary"`
	Priority           string   `json:"priority"`
	Required           bool     `json:"required"`
	RelatedScenarios   []string `json:"related_scenarios,omitempty"`
	RelatedSurfaceRefs []string `json:"related_surface_refs,omitempty"`
	RelatedScreens     []string `json:"related_screens,omitempty"`
	AcceptanceRefs     []string `json:"acceptance_refs,omitempty"`
}

type InteractionSurface struct {
	SurfaceID          string   `json:"surface_id"`
	Label              string   `json:"label"`
	Purpose            string   `json:"purpose"`
	PrimaryFeatureRefs []string `json:"primary_feature_refs,omitempty"`
	LegacyScreenRef    string   `json:"legacy_screen_ref,omitempty"`
}

type Screen struct {
	ScreenID        string   `json:"screen_id"`
	Name            string   `json:"name"`
	Purpose         string   `json:"purpose"`
	PrimaryFeatures []string `json:"primary_features,omitempty"`
}

type UserFlow struct {
	FlowID string     `json:"flow_id"`
	Title  string     `json:"title"`
	Steps  []FlowStep `json:"steps"`
}

type FlowStep struct {
	StepID         string `json:"step_id"`
	Title          string `json:"title"`
	SurfaceRef     string `json:"surface_ref,omitempty"`
	ScreenRef      string `json:"screen_ref,omitempty"`
	Actor          string `json:"actor,omitempty"`
	ExpectedResult string `json:"expected_result,omitempty"`
}

type DataEntity struct {
	EntityID string      `json:"entity_id"`
	Name     string      `json:"name"`
	Fields   []DataField `json:"fields"`
	Source   string      `json:"source,omitempty"`
}

type DataField struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Required    bool   `json:"required"`
	Description string `json:"description,omitempty"`
}

type TemplateConstraints struct {
	Stack                string   `json:"stack"`
	AndroidRequired      bool     `json:"android_required"`
	RequiredCapabilities []string `json:"required_capabilities"`
	ExcludedLicenses     []string `json:"excluded_licenses,omitempty"`
	PreferredTemplateIDs []string `json:"preferred_template_ids,omitempty"`
}

type AcceptanceCriterion struct {
	CriterionID string `json:"criterion_id"`
	Label       string `json:"label"`
	Category    string `json:"category"`
	Required    bool   `json:"required"`
	Description string `json:"description,omitempty"`
}

type ManualReviewPoint struct {
	PointID string `json:"point_id"`
	Summary string `json:"summary"`
	Reason  string `json:"reason"`
	Owner   string `json:"owner,omitempty"`
}

type KnownUnknown struct {
	Question string `json:"question"`
	Impact   string `json:"impact"`
	Owner    string `json:"owner,omitempty"`
}

func Compile(request Request) (Bundle, error) {
	requirementText := strings.TrimSpace(request.RequirementText)
	if requirementText == "" {
		return Bundle{}, fmt.Errorf("requirement text is required")
	}
	now := time.Now().UTC()
	if request.Now != nil {
		now = request.Now().UTC()
	}
	spec := analyzeRequirement(request, requirementText)
	spec, err := finalizeTemplateSelection(spec)
	if err != nil {
		return Bundle{}, err
	}
	surfaceList := normalizeInteractionSurfaces(spec.SurfaceList, spec.ScreenList)
	featureList := normalizeFeatureSurfaceRefs(spec.FeatureList, surfaceList)
	userFlows := normalizeUserFlowsForSurfaces(spec.UserFlows, surfaceList)
	spec.SurfaceList = surfaceList
	spec.ScreenList = nil
	spec.FeatureList = featureList
	spec.UserFlows = userFlows
	prd := PRD{
		SchemaVersion:       defaultSchemaVersion,
		ID:                  spec.PRDID,
		Version:             "0.1.0",
		Status:              "draft",
		Title:               spec.Title,
		Summary:             spec.Summary,
		ProblemStatement:    spec.ProblemStatement,
		TargetUsers:         spec.TargetUsers,
		CoreScenarios:       spec.CoreScenarios,
		Goals:               spec.Goals,
		NonGoals:            spec.NonGoals,
		FeatureList:         featureList,
		SurfaceList:         publicInteractionSurfaceList(surfaceList),
		UserFlows:           userFlows,
		DataEntities:        spec.DataEntities,
		TemplateConstraints: spec.TemplateConstraints,
		AcceptanceCriteria:  spec.AcceptanceCriteria,
		ManualReviewPoints:  spec.ManualReviewPoints,
		KnownUnknowns:       spec.KnownUnknowns,
		SourceRefs:          []string{spec.SourceSummary},
		CreatedAt:           now.Format(time.RFC3339),
		UpdatedAt:           now.Format(time.RFC3339),
	}
	return buildBundle(spec, prd, now)
}

func Recompile(request RecompileRequest) (Bundle, error) {
	requirementText := strings.TrimSpace(request.RequirementText)
	if requirementText == "" {
		return Bundle{}, fmt.Errorf("requirement text is required")
	}
	jobID := strings.TrimSpace(request.JobID)
	if jobID == "" {
		return Bundle{}, fmt.Errorf("job_id is required")
	}
	if strings.TrimSpace(request.PRD.ID) == "" {
		return Bundle{}, fmt.Errorf("prepared prd id is required")
	}
	now := time.Now().UTC()
	if request.Now != nil {
		now = request.Now().UTC()
	}
	compileRequest := Request{
		RequirementText:   requirementText,
		RequirementSource: request.RequirementSource,
		TitleHint:         request.PRD.Title,
		JobID:             jobID,
		PRDID:             request.PRD.ID,
		TemplateID:        request.TemplateID,
		ExecutorImage:     request.ExecutorImage,
		Now:               request.Now,
	}
	spec := analyzeRequirement(compileRequest, requirementText)
	spec, err := finalizeTemplateSelection(spec)
	if err != nil {
		return Bundle{}, err
	}
	spec = applyPreparedPRDToDomainSpec(spec, request.PRD)
	prd := normalizePreparedPRD(request.PRD, spec, now)
	return buildBundle(spec, prd, now)
}

func applyPreparedPRDToDomainSpec(spec domainSpec, prd PRD) domainSpec {
	spec.PRDID = prd.ID
	spec.Title = prd.Title
	spec.Summary = prd.Summary
	spec.ProblemStatement = prd.ProblemStatement
	spec.TargetUsers = append([]UserProfile(nil), prd.TargetUsers...)
	spec.CoreScenarios = append([]Scenario(nil), prd.CoreScenarios...)
	spec.Goals = append([]string(nil), prd.Goals...)
	spec.NonGoals = append([]string(nil), prd.NonGoals...)
	surfaceList := normalizeInteractionSurfaces(prd.SurfaceList, prd.ScreenList)
	spec.FeatureList = normalizeFeatureSurfaceRefs(prd.FeatureList, surfaceList)
	spec.SurfaceList = surfaceList
	spec.ScreenList = nil
	spec.UserFlows = normalizeUserFlowsForSurfaces(prd.UserFlows, surfaceList)
	spec.DataEntities = append([]DataEntity(nil), prd.DataEntities...)
	spec.TemplateConstraints = prd.TemplateConstraints
	spec.AcceptanceCriteria = append([]AcceptanceCriterion(nil), prd.AcceptanceCriteria...)
	spec.ManualReviewPoints = append([]ManualReviewPoint(nil), prd.ManualReviewPoints...)
	spec.KnownUnknowns = append([]KnownUnknown(nil), prd.KnownUnknowns...)
	if len(prd.SourceRefs) > 0 && strings.TrimSpace(prd.SourceRefs[0]) != "" {
		spec.SourceSummary = strings.TrimSpace(prd.SourceRefs[0])
	}
	return spec
}

func normalizePreparedPRD(prd PRD, spec domainSpec, now time.Time) PRD {
	normalized := prd
	if strings.TrimSpace(normalized.SchemaVersion) == "" {
		normalized.SchemaVersion = defaultSchemaVersion
	}
	normalized.ID = strings.TrimSpace(normalized.ID)
	if normalized.ID == "" {
		normalized.ID = strings.TrimSpace(spec.PRDID)
	}
	if strings.TrimSpace(normalized.Status) == "" {
		normalized.Status = "draft"
	}
	if strings.TrimSpace(normalized.CreatedAt) == "" {
		normalized.CreatedAt = now.Format(time.RFC3339)
	}
	if strings.TrimSpace(normalized.UpdatedAt) == "" {
		normalized.UpdatedAt = now.Format(time.RFC3339)
	}
	surfaceList := normalizeInteractionSurfaces(normalized.SurfaceList, normalized.ScreenList)
	normalized.SurfaceList = publicInteractionSurfaceList(surfaceList)
	normalized.ScreenList = nil
	normalized.FeatureList = normalizeFeatureSurfaceRefs(normalized.FeatureList, surfaceList)
	normalized.UserFlows = normalizeUserFlowsForSurfaces(normalized.UserFlows, surfaceList)
	return normalized
}
