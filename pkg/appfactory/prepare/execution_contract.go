package prepare

import (
	"bytes"
	"encoding/json"
	"strings"

	appruns "github.com/sipeed/picoclaw/pkg/appfactory/runs"
)

type ExecutionContract struct {
	ContractVersion       string                 `json:"contract_version"`
	DomainModel           ExecutionDomainModel   `json:"domain_model"`
	SurfaceRelationSchema SurfaceRelationSchema  `json:"surface_relation_schema,omitempty"`
	SurfaceContracts      []SurfaceContract      `json:"surface_contracts,omitempty"`
	KeyFlows              []KeyFlow              `json:"key_flows,omitempty"`
	AcceptanceMatrix      []AcceptanceMatrixItem `json:"acceptance_matrix,omitempty"`
	TaskProjection        []TaskProjectionItem   `json:"task_projection,omitempty"`
}

type executionContractJSON ExecutionContract

func (contract *ExecutionContract) UnmarshalJSON(data []byte) error {
	var aux executionContractJSON
	if err := strictUnmarshalJSON(data, &aux); err != nil {
		return err
	}
	*contract = ExecutionContract(aux)
	return nil
}

type ExecutionDomainModel struct {
	DomainName             string       `json:"domain_name"`
	Entities               []DataEntity `json:"entities,omitempty"`
	SummaryMetrics         []string     `json:"summary_metrics,omitempty"`
	CriticalFlowRefs       []string     `json:"critical_flow_refs,omitempty"`
	SemanticAcceptanceRefs []string     `json:"semantic_acceptance_refs,omitempty"`
}

type SurfaceRelationSchema struct {
	AnchorCollectionPath string                             `json:"anchor_collection_path"`
	AnchorIDPath         string                             `json:"anchor_id_path"`
	PrimaryRefFields     []string                           `json:"primary_ref_fields,omitempty"`
	AllowedReferences    []SurfaceRelationReferenceRule     `json:"allowed_references,omitempty"`
}

type SurfaceRelationReferenceRule struct {
	SourcePath  string `json:"source_path"`
	TargetPath  string `json:"target_path"`
	Cardinality string `json:"cardinality,omitempty"`
}

type SurfaceContract struct {
	ContractID         string                  `json:"contract_id"`
	SurfaceRef         string                  `json:"surface_ref"`
	TemplateBindingRef string                  `json:"template_binding_ref,omitempty"`
	Purpose            string                  `json:"purpose"`
	PrimaryFeatureRefs []string                `json:"primary_feature_refs,omitempty"`
	PrimaryEntityRefs  []string                `json:"primary_entity_refs,omitempty"`
	TargetPaths        []string                `json:"target_paths,omitempty"`
	RequiredActions    []SurfaceContractAction `json:"required_actions,omitempty"`
	RequiredStates     []SurfaceContractState  `json:"required_states,omitempty"`
	AcceptanceRefs     []string                `json:"acceptance_refs,omitempty"`
}

type surfaceContractJSON SurfaceContract

func (contract *SurfaceContract) UnmarshalJSON(data []byte) error {
	var aux surfaceContractJSON
	if err := strictUnmarshalJSON(data, &aux); err != nil {
		return err
	}
	*contract = SurfaceContract(aux)
	return nil
}

func strictUnmarshalJSON(data []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	return decoder.Decode(target)
}

type SurfaceContractAction struct {
	ActionID string `json:"action_id"`
	Label    string `json:"label"`
	Trigger  string `json:"trigger,omitempty"`
	Outcome  string `json:"outcome"`
}

type SurfaceContractState struct {
	StateID  string `json:"state_id"`
	Label    string `json:"label"`
	When     string `json:"when,omitempty"`
	Evidence string `json:"evidence"`
}

type KeyFlow struct {
	FlowID          string        `json:"flow_id"`
	Title           string        `json:"title"`
	EntrySurfaceRef string        `json:"entry_surface_ref,omitempty"`
	Steps           []KeyFlowStep `json:"steps,omitempty"`
	WriteEntityRefs []string      `json:"write_entity_refs,omitempty"`
	ReadEntityRefs  []string      `json:"read_entity_refs,omitempty"`
	SuccessSignals  []string      `json:"success_signals,omitempty"`
	AcceptanceRefs  []string      `json:"acceptance_refs,omitempty"`
}

type KeyFlowStep struct {
	StepID         string `json:"step_id"`
	SurfaceRef     string `json:"surface_ref,omitempty"`
	Actor          string `json:"actor,omitempty"`
	Action         string `json:"action"`
	ExpectedResult string `json:"expected_result"`
}

type AcceptanceMatrixItem struct {
	MatrixID        string   `json:"matrix_id"`
	AcceptanceRef   string   `json:"acceptance_ref"`
	Layer           string   `json:"layer"`
	Required        bool     `json:"required"`
	SourceRefs      []string `json:"source_refs,omitempty"`
	BindingRefs     []string `json:"slot_refs,omitempty"`
	FieldRefs       []string `json:"field_refs,omitempty"`
	CheckRefs       []string `json:"check_refs,omitempty"`
	EvidencePattern string   `json:"evidence_pattern,omitempty"`
}

type TaskProjectionItem struct {
	TaskID              string                         `json:"task_id"`
	Title               string                         `json:"title"`
	Wave                int                            `json:"wave"`
	Lane                string                         `json:"lane"`
	Category            string                         `json:"category,omitempty"`
	TaskType            appruns.BuilderRuntimeTaskType `json:"task_type"`
	RouteHint           appruns.TaskRouteHint          `json:"route_hint,omitempty"`
	RiskLevel           appruns.TaskRiskLevel          `json:"risk_level,omitempty"`
	SurfaceRefs         []string                       `json:"surface_refs,omitempty"`
	EntityRefs          []string                       `json:"entity_refs,omitempty"`
	RelationGroupRefs   []string                       `json:"relation_group_refs,omitempty"`
	SharedOwnershipRefs []string                       `json:"shared_ownership_refs,omitempty"`
	AcceptanceRefs      []string                       `json:"acceptance_refs,omitempty"`
	SemanticIntentRefs  []string                       `json:"semantic_intent_refs,omitempty"`
	DependsOn           []string                       `json:"depends_on,omitempty"`
	BindingRefs         []string                       `json:"slot_refs,omitempty"`
	TargetPaths         []string                       `json:"target_paths"`
	OwnedPaths          []string                       `json:"owned_paths,omitempty"`
	SuccessEvidence     []string                       `json:"success_evidence,omitempty"`
}

func buildExecutionContract(spec domainSpec, prd PRD, taskBundle []appruns.TaskBundleItem) *ExecutionContract {
	domainModel := buildDomainModel(spec)
	templateSlotMap := buildTemplateSlotMap(spec)
	taskAllocation := buildTaskAllocation(taskBundle, spec)
	acceptancePlan := buildAcceptancePlan(spec)

	contract := ExecutionContract{
		ContractVersion:       defaultSchemaVersion,
		DomainModel:           buildExecutionDomainModel(prd, domainModel),
		SurfaceRelationSchema: buildSurfaceRelationSchema(),
		SurfaceContracts:      buildSurfaceContracts(prd, templateSlotMap),
		KeyFlows:              buildKeyFlows(prd),
		AcceptanceMatrix:      buildAcceptanceMatrix(acceptancePlan),
		TaskProjection:        buildTaskProjection(taskBundle, taskAllocation),
	}
	return &contract
}

func buildExecutionDomainModel(prd PRD, domainModel DomainModel) ExecutionDomainModel {
	result := ExecutionDomainModel{
		DomainName:             firstNonEmpty(domainModel.DomainName, prd.Title),
		Entities:               append([]DataEntity(nil), domainModel.Entities...),
		SummaryMetrics:         append([]string(nil), domainModel.SummaryMetrics...),
		CriticalFlowRefs:       collectUserFlowRefs(prd.UserFlows),
		SemanticAcceptanceRefs: collectAcceptanceRefs(prd.AcceptanceCriteria),
	}
	return result
}

func buildSurfaceRelationSchema() SurfaceRelationSchema {
	return SurfaceRelationSchema{
		AnchorCollectionPath: "surface_list",
		AnchorIDPath:         "surface_list[].surface_id",
		PrimaryRefFields:     []string{"surface_ref", "surface_refs"},
		AllowedReferences: []SurfaceRelationReferenceRule{
			{
				SourcePath:  "feature_list[].related_surface_refs[]",
				TargetPath:  "surface_list[].surface_id",
				Cardinality: "many",
			},
			{
				SourcePath:  "user_flows[].steps[].surface_ref",
				TargetPath:  "surface_list[].surface_id",
				Cardinality: "one",
			},
			{
				SourcePath:  "execution_contract.surface_contracts[].surface_ref",
				TargetPath:  "surface_list[].surface_id",
				Cardinality: "one",
			},
			{
				SourcePath:  "execution_contract.key_flows[].entry_surface_ref",
				TargetPath:  "surface_list[].surface_id",
				Cardinality: "one",
			},
			{
				SourcePath:  "execution_contract.key_flows[].steps[].surface_ref",
				TargetPath:  "surface_list[].surface_id",
				Cardinality: "one",
			},
			{
				SourcePath:  "task_allocation.units[].surface_refs[]",
				TargetPath:  "surface_list[].surface_id",
				Cardinality: "many",
			},
			{
				SourcePath:  "execution_contract.task_projection[].surface_refs[]",
				TargetPath:  "surface_list[].surface_id",
				Cardinality: "many",
			},
		},
	}
}

type contractSurfaceSource struct {
	SurfaceRef         string
	Label              string
	Purpose            string
	PrimaryFeatureRefs []string
}

func buildSurfaceContracts(prd PRD, slotMap TemplateSlotMap) []SurfaceContract {
	sources := contractSurfaceSources(prd)
	contracts := make([]SurfaceContract, 0, len(sources))
	featureByID := featureMapByID(prd.FeatureList)
	criteriaByID := acceptanceCriterionMapByID(prd.AcceptanceCriteria)
	for _, surface := range sources {
		acceptanceRefs := acceptanceRefsForFeatures(surface.PrimaryFeatureRefs, prd.FeatureList)
		binding := selectSurfaceTemplateBinding(surface.SurfaceRef, acceptanceRefs, slotMap)
		actions := surfaceActionsForRef(surface.SurfaceRef, prd.UserFlows)
		states := surfaceStatesForAcceptanceRefs(acceptanceRefs, criteriaByID)
		contract := SurfaceContract{
			ContractID:         "contract-" + surface.SurfaceRef,
			SurfaceRef:         surface.SurfaceRef,
			Purpose:            surface.Purpose,
			PrimaryFeatureRefs: append([]string(nil), surface.PrimaryFeatureRefs...),
			PrimaryEntityRefs:  executionEntityRefsForSurface(surface, prd.DataEntities, featureByID, actions, states),
			AcceptanceRefs:     acceptanceRefs,
		}
		if bindingRef := publicTemplateBindingRef(binding); bindingRef != "" {
			contract.TemplateBindingRef = bindingRef
		}
		if len(binding.TargetPaths) > 0 {
			contract.TargetPaths = append([]string(nil), binding.TargetPaths...)
		}
		if len(actions) > 0 {
			contract.RequiredActions = actions
		}
		if len(states) > 0 {
			contract.RequiredStates = states
		}
		contracts = append(contracts, contract)
	}
	return contracts
}

func buildKeyFlows(prd PRD) []KeyFlow {
	flows := make([]KeyFlow, 0, len(prd.UserFlows))
	surfaceAcceptanceRefs := surfaceAcceptanceRefsBySurface(prd)
	for _, flow := range prd.UserFlows {
		steps := make([]KeyFlowStep, 0, len(flow.Steps))
		successSignals := make([]string, 0, len(flow.Steps))
		for _, step := range flow.Steps {
			surfaceRef := firstNonEmpty(step.SurfaceRef, step.ScreenRef)
			steps = append(steps, KeyFlowStep{
				StepID:         step.StepID,
				SurfaceRef:     surfaceRef,
				Actor:          step.Actor,
				Action:         firstNonEmpty(step.Title, step.ExpectedResult),
				ExpectedResult: firstNonEmpty(step.ExpectedResult, step.Title),
			})
			if strings.TrimSpace(step.ExpectedResult) != "" {
				successSignals = append(successSignals, step.ExpectedResult)
			}
		}
		readRefs, writeRefs := executionEntityRefsForFlow(flow, prd.DataEntities)
		acceptanceRefs := keyFlowAcceptanceRefs(flow, surfaceAcceptanceRefs)
		flows = append(flows, KeyFlow{
			FlowID:          flow.FlowID,
			Title:           flow.Title,
			EntrySurfaceRef: firstUserFlowSurfaceRef(flow),
			Steps:           steps,
			WriteEntityRefs: writeRefs,
			ReadEntityRefs:  readRefs,
			SuccessSignals:  uniqueStrings(successSignals),
			AcceptanceRefs:  acceptanceRefs,
		})
	}
	return flows
}

func buildAcceptanceMatrix(plan AcceptancePlan) []AcceptanceMatrixItem {
	items := make([]AcceptanceMatrixItem, 0, len(plan.StructureChecks)+len(plan.SemanticChecks)+len(plan.BehaviorChecks)+len(plan.DeliveryChecks))
	items = append(items, acceptanceMatrixItemsForLayer(plan.StructureChecks, "structure")...)
	items = append(items, acceptanceMatrixItemsForLayer(plan.SemanticChecks, "semantic")...)
	items = append(items, acceptanceMatrixItemsForLayer(plan.BehaviorChecks, "behavior")...)
	items = append(items, acceptanceMatrixItemsForLayer(plan.DeliveryChecks, "delivery")...)
	return items
}

func buildTaskProjection(taskBundle []appruns.TaskBundleItem, taskAllocation TaskAllocation) []TaskProjectionItem {
	unitsByID := make(map[string]TaskAllocationUnit, len(taskAllocation.Units))
	for _, unit := range taskAllocation.Units {
		unitsByID[unit.AllocationID] = unit
	}
	items := make([]TaskProjectionItem, 0, len(taskBundle))
	for _, task := range taskBundle {
		unit := unitsByID[task.TaskID]
		items = append(items, TaskProjectionItem{
			TaskID:              task.TaskID,
			Title:               task.Title,
			Wave:                unit.Wave,
			Lane:                unit.Lane,
			Category:            string(task.Category),
			TaskType:            task.TaskType,
			RouteHint:           task.RouteHint,
			RiskLevel:           task.RiskLevel,
			SurfaceRefs:         append([]string(nil), unit.SurfaceRefs...),
			EntityRefs:          append([]string(nil), unit.EntityRefs...),
			RelationGroupRefs:   append([]string(nil), unit.RelationGroupRefs...),
			SharedOwnershipRefs: append([]string(nil), unit.SharedOwnershipRefs...),
			AcceptanceRefs:      append([]string(nil), task.RelatedRequirements...),
			SemanticIntentRefs:  append([]string(nil), unit.SemanticIntentRefs...),
			DependsOn:           append([]string(nil), task.Dependencies...),
			BindingRefs:         append([]string(nil), unit.BindingRefs...),
			TargetPaths:         append([]string(nil), task.TargetPaths...),
			OwnedPaths:          append([]string(nil), unit.OwnedPaths...),
			SuccessEvidence:     append([]string(nil), unit.SuccessEvidence...),
		})
	}
	return items
}

func contractSurfaceSources(prd PRD) []contractSurfaceSource {
	if len(prd.SurfaceList) > 0 {
		sources := make([]contractSurfaceSource, 0, len(prd.SurfaceList))
		for _, surface := range prd.SurfaceList {
			sources = append(sources, contractSurfaceSource{
				SurfaceRef:         strings.TrimSpace(surface.SurfaceID),
				Label:              strings.TrimSpace(surface.Label),
				Purpose:            strings.TrimSpace(surface.Purpose),
				PrimaryFeatureRefs: append([]string(nil), surface.PrimaryFeatureRefs...),
			})
		}
		return sources
	}
	sources := make([]contractSurfaceSource, 0, len(prd.ScreenList))
	for _, screen := range prd.ScreenList {
		sources = append(sources, contractSurfaceSource{
			SurfaceRef:         strings.TrimSpace(screen.ScreenID),
			Label:              strings.TrimSpace(screen.Name),
			Purpose:            strings.TrimSpace(screen.Purpose),
			PrimaryFeatureRefs: append([]string(nil), screen.PrimaryFeatures...),
		})
	}
	return sources
}

func featureMapByID(features []Feature) map[string]Feature {
	result := make(map[string]Feature, len(features))
	for _, feature := range features {
		featureID := strings.TrimSpace(feature.FeatureID)
		if featureID == "" {
			continue
		}
		result[featureID] = feature
	}
	return result
}

func acceptanceCriterionMapByID(criteria []AcceptanceCriterion) map[string]AcceptanceCriterion {
	result := make(map[string]AcceptanceCriterion, len(criteria))
	for _, criterion := range criteria {
		criterionID := strings.TrimSpace(criterion.CriterionID)
		if criterionID == "" {
			continue
		}
		result[criterionID] = criterion
	}
	return result
}

func surfaceAcceptanceRefsBySurface(prd PRD) map[string][]string {
	surfaceList := normalizeInteractionSurfaces(prd.SurfaceList, prd.ScreenList)
	featureList := normalizeFeatureSurfaceRefs(prd.FeatureList, surfaceList)
	refs := map[string][]string{}
	for _, surface := range surfaceList {
		surfaceRef := strings.TrimSpace(surface.SurfaceID)
		if surfaceRef == "" {
			continue
		}
		refs[surfaceRef] = append(refs[surfaceRef], acceptanceRefsForFeatures(surface.PrimaryFeatureRefs, prd.FeatureList)...)
	}
	for _, feature := range featureList {
		for _, surfaceRef := range feature.RelatedSurfaceRefs {
			surfaceRef = strings.TrimSpace(surfaceRef)
			if surfaceRef == "" {
				continue
			}
			refs[surfaceRef] = append(refs[surfaceRef], feature.AcceptanceRefs...)
		}
	}
	for surfaceRef, acceptanceRefs := range refs {
		refs[surfaceRef] = uniqueStrings(acceptanceRefs)
	}
	return refs
}

func surfaceStatesForAcceptanceRefs(acceptanceRefs []string, criteriaByID map[string]AcceptanceCriterion) []SurfaceContractState {
	states := make([]SurfaceContractState, 0, len(acceptanceRefs))
	for _, acceptanceRef := range acceptanceRefs {
		acceptanceRef = strings.TrimSpace(acceptanceRef)
		if acceptanceRef == "" {
			continue
		}
		criterion, ok := criteriaByID[acceptanceRef]
		if !ok {
			states = append(states, SurfaceContractState{
				StateID:  stateIDForAcceptanceRef(acceptanceRef),
				Label:    acceptanceRef,
				Evidence: acceptanceRef,
			})
			continue
		}
		states = append(states, SurfaceContractState{
			StateID:  stateIDForAcceptanceRef(acceptanceRef),
			Label:    firstNonEmpty(strings.TrimSpace(criterion.Label), acceptanceRef),
			Evidence: firstNonEmpty(strings.TrimSpace(criterion.Description), strings.TrimSpace(criterion.Label), acceptanceRef),
		})
	}
	return states
}

func stateIDForAcceptanceRef(acceptanceRef string) string {
	trimmed := strings.TrimSpace(acceptanceRef)
	trimmed = strings.TrimPrefix(trimmed, "ac-")
	if trimmed == "" {
		return ""
	}
	if strings.HasPrefix(trimmed, "state-") {
		return trimmed
	}
	return "state-" + trimmed
}

func keyFlowAcceptanceRefs(flow UserFlow, surfaceAcceptanceRefs map[string][]string) []string {
	refs := make([]string, 0, len(flow.Steps)+1)
	if entrySurfaceRef := strings.TrimSpace(firstUserFlowSurfaceRef(flow)); entrySurfaceRef != "" {
		refs = append(refs, surfaceAcceptanceRefs[entrySurfaceRef]...)
	}
	for _, step := range flow.Steps {
		surfaceRef := strings.TrimSpace(firstNonEmpty(step.SurfaceRef, step.ScreenRef))
		if surfaceRef == "" {
			continue
		}
		refs = append(refs, surfaceAcceptanceRefs[surfaceRef]...)
	}
	return uniqueStrings(refs)
}

func selectSurfaceTemplateBinding(surfaceRef string, acceptanceRefs []string, slotMap TemplateSlotMap) TemplateSlot {
	_ = acceptanceRefs
	preferredBindingID := preferredTemplateBindingIDForSurface(surfaceRef)
	if preferredBindingID != "" {
		for _, slot := range slotMap.Slots {
			if !isSurfaceTemplateBinding(slot) || publicTemplateBindingRef(slot) == "" {
				continue
			}
			if publicTemplateBindingRef(slot) == preferredBindingID {
				return slot
			}
		}
	}
	return TemplateSlot{}
}

func preferredTemplateBindingIDForSurface(surfaceRef string) string {
	switch strings.TrimSpace(surfaceRef) {
	case genericSurfaceOverviewID, genericSurfaceCollectionID, genericSurfaceMutationID, genericSurfaceInspectionID:
		return strings.TrimSpace(surfaceRef)
	default:
		return ""
	}
}

func isSurfaceTemplateBinding(slot TemplateSlot) bool {
	switch strings.TrimSpace(publicTemplateBindingRef(slot)) {
	case genericSurfaceOverviewID, genericSurfaceCollectionID, genericSurfaceMutationID, genericSurfaceInspectionID:
		return true
	default:
		return false
	}
}

func publicTemplateBindingRef(slot TemplateSlot) string {
	return strings.TrimSpace(slot.BindingID)
}

func surfaceActionsForRef(surfaceRef string, flows []UserFlow) []SurfaceContractAction {
	actions := make([]SurfaceContractAction, 0)
	for _, flow := range flows {
		for _, step := range flow.Steps {
			if firstNonEmpty(step.SurfaceRef, step.ScreenRef) != surfaceRef {
				continue
			}
			actions = append(actions, SurfaceContractAction{
				ActionID: flow.FlowID + ":" + step.StepID,
				Label:    firstNonEmpty(step.Title, step.ExpectedResult),
				Trigger:  firstNonEmpty(step.Actor, step.Title),
				Outcome:  firstNonEmpty(step.ExpectedResult, step.Title),
			})
		}
	}
	return actions
}

func executionEntityRefsForSurface(surface contractSurfaceSource, entities []DataEntity, featureByID map[string]Feature, actions []SurfaceContractAction, states []SurfaceContractState) []string {
	primaryRefs, summaryRefs := executionEntityBuckets(entities)
	if len(primaryRefs) == 0 && len(summaryRefs) == 0 {
		return nil
	}
	textParts := []string{surface.SurfaceRef, surface.Label, surface.Purpose}
	for _, featureRef := range surface.PrimaryFeatureRefs {
		feature, ok := featureByID[strings.TrimSpace(featureRef)]
		if !ok {
			continue
		}
		textParts = append(textParts, feature.Title, feature.Summary)
	}
	for _, action := range actions {
		textParts = append(textParts, action.Label, action.Trigger, action.Outcome)
	}
	for _, state := range states {
		textParts = append(textParts, state.Label, state.When, state.Evidence)
	}
	text := strings.ToLower(strings.Join(textParts, " "))
	matchedPrimaryRefs, matchedSummaryRefs := executionEntityRefsMentionedInText(text, entities)
	summaryIntent := containsAnyExecutionKeyword(text, "首页", "概览", "摘要", "趋势", "看板", "overview", "summary", "trend", "dashboard", "board")
	primaryIntent := containsAnyExecutionKeyword(text, "列表", "详情", "查看", "历史", "浏览", "录入", "新增", "编辑", "删除", "筛选", "清单", "list", "detail", "inspect", "review", "history", "browse", "create", "add", "save", "edit", "delete", "filter", "form", "collection", "mutation")
	if summaryIntent && !primaryIntent && len(summaryRefs) > 0 {
		if len(matchedSummaryRefs) > 0 {
			return matchedSummaryRefs
		}
		return summaryRefs
	}
	if primaryIntent && !summaryIntent && len(primaryRefs) > 0 {
		if len(matchedPrimaryRefs) > 0 {
			return matchedPrimaryRefs
		}
		return primaryRefs
	}
	matchedRefs := uniqueStrings(append(append([]string(nil), matchedPrimaryRefs...), matchedSummaryRefs...))
	if len(matchedRefs) > 0 {
		return matchedRefs
	}
	if summaryIntent && primaryIntent {
		return uniqueStrings(append(append([]string(nil), primaryRefs...), summaryRefs...))
	}
	if summaryIntent && len(summaryRefs) > 0 {
		return summaryRefs
	}
	if len(primaryRefs) > 0 {
		return primaryRefs
	}
	return summaryRefs
}

func executionEntityBuckets(entities []DataEntity) ([]string, []string) {
	primary := make([]string, 0, len(entities))
	summary := make([]string, 0, len(entities))
	for _, entity := range entities {
		entityID := strings.TrimSpace(entity.EntityID)
		if entityID == "" {
			continue
		}
		lower := strings.ToLower(entity.EntityID + " " + entity.Name)
		if containsAnyExecutionKeyword(lower, "summary", "摘要", "汇总") {
			summary = append(summary, entityID)
			continue
		}
		if strings.EqualFold(strings.TrimSpace(entity.Source), "derived") {
			summary = append(summary, entityID)
			continue
		}
		primary = append(primary, entityID)
	}
	return uniqueStrings(primary), uniqueStrings(summary)
}

func executionEntityRefsMentionedInText(text string, entities []DataEntity) ([]string, []string) {
	primary := make([]string, 0)
	summary := make([]string, 0)
	for _, entity := range entities {
		entityID := strings.TrimSpace(entity.EntityID)
		if entityID == "" || !executionEntityMatchesText(text, entity) {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(entity.Source), "derived") || containsAnyExecutionKeyword(strings.ToLower(entity.EntityID+" "+entity.Name), "summary", "摘要", "汇总") {
			summary = append(summary, entityID)
			continue
		}
		primary = append(primary, entityID)
	}
	return uniqueStrings(primary), uniqueStrings(summary)
}

func executionEntityMatchesText(text string, entity DataEntity) bool {
	for _, candidate := range []string{entity.EntityID, entity.Name} {
		candidate = strings.ToLower(strings.TrimSpace(candidate))
		if candidate == "" {
			continue
		}
		if strings.Contains(text, candidate) {
			return true
		}
	}
	return false
}

func executionEntityRefsForFlow(flow UserFlow, entities []DataEntity) ([]string, []string) {
	primaryRefs, summaryRefs := executionEntityBuckets(entities)
	text := strings.ToLower(flow.Title)
	for _, step := range flow.Steps {
		text += " " + strings.ToLower(step.Title) + " " + strings.ToLower(step.ExpectedResult)
	}
	readRefs := make([]string, 0)
	writeRefs := make([]string, 0)
	if containsAnyExecutionKeyword(text, "新增", "保存", "编辑", "删除", "create", "add", "save", "edit", "delete") {
		writeRefs = append(writeRefs, primaryRefs...)
		writeRefs = append(writeRefs, summaryRefs...)
	}
	if containsAnyExecutionKeyword(text, "首页", "概览", "摘要", "趋势", "home", "overview", "summary", "trend") {
		readRefs = append(readRefs, summaryRefs...)
	}
	if containsAnyExecutionKeyword(text, "列表", "详情", "查看", "历史", "ledger", "list", "detail", "inspect", "review", "history") {
		readRefs = append(readRefs, primaryRefs...)
	}
	if len(readRefs) == 0 {
		readRefs = append(readRefs, primaryRefs...)
	}
	return uniqueStrings(readRefs), uniqueStrings(writeRefs)
}

func acceptanceMatrixItemsForLayer(checks []AcceptancePlanItem, layer string) []AcceptanceMatrixItem {
	items := make([]AcceptanceMatrixItem, 0, len(checks))
	for _, check := range checks {
		item := AcceptanceMatrixItem{
			MatrixID:      "matrix-" + check.CheckID,
			AcceptanceRef: check.CheckID,
			Layer:         layer,
			Required:      check.Required,
			BindingRefs:   append([]string(nil), check.BindingRefs...),
			FieldRefs:     append([]string(nil), check.FieldRefs...),
			CheckRefs:     []string{check.CheckID},
		}
		if strings.TrimSpace(check.SourceRef) != "" {
			item.SourceRefs = []string{check.SourceRef}
		}
		if strings.TrimSpace(check.EvidencePattern) != "" {
			item.EvidencePattern = check.EvidencePattern
		}
		items = append(items, item)
	}
	return items
}

func collectUserFlowRefs(flows []UserFlow) []string {
	result := make([]string, 0, len(flows))
	for _, flow := range flows {
		if strings.TrimSpace(flow.FlowID) != "" {
			result = append(result, flow.FlowID)
		}
	}
	return result
}

func collectAcceptanceRefs(criteria []AcceptanceCriterion) []string {
	result := make([]string, 0, len(criteria))
	for _, criterion := range criteria {
		if strings.TrimSpace(criterion.CriterionID) != "" {
			result = append(result, criterion.CriterionID)
		}
	}
	return result
}

func acceptanceRefsForFeatures(featureRefs []string, features []Feature) []string {
	refs := make([]string, 0)
	for _, featureRef := range featureRefs {
		for _, feature := range features {
			if feature.FeatureID != featureRef {
				continue
			}
			refs = append(refs, feature.AcceptanceRefs...)
		}
	}
	return uniqueStrings(refs)
}

func firstUserFlowSurfaceRef(flow UserFlow) string {
	for _, step := range flow.Steps {
		if ref := strings.TrimSpace(firstNonEmpty(step.SurfaceRef, step.ScreenRef)); ref != "" {
			return ref
		}
	}
	return ""
}

func containsAnyExecutionKeyword(text string, keywords ...string) bool {
	for _, keyword := range keywords {
		if strings.Contains(text, strings.ToLower(keyword)) {
			return true
		}
	}
	return false
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		result = append(result, trimmed)
	}
	return result
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			return trimmed
		}
	}
	return ""
}
