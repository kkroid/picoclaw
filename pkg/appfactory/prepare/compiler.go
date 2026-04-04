package prepare

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	appruns "github.com/sipeed/picoclaw/pkg/appfactory/runs"
	"github.com/sipeed/picoclaw/pkg/fileutil"
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
	builderInputFileName     = "builder-input.json"
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
	ScreenList          []Screen              `json:"screen_list"`
	UserFlows           []UserFlow            `json:"user_flows"`
	DataEntities        []DataEntity          `json:"data_entities"`
	TemplateConstraints TemplateConstraints   `json:"template_constraints"`
	AcceptanceCriteria  []AcceptanceCriterion `json:"acceptance_criteria"`
	ManualReviewPoints  []ManualReviewPoint   `json:"manual_review_points"`
	KnownUnknowns       []KnownUnknown        `json:"known_unknowns"`
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
	FeatureID        string   `json:"feature_id"`
	Title            string   `json:"title"`
	Summary          string   `json:"summary"`
	Priority         string   `json:"priority"`
	Required         bool     `json:"required"`
	RelatedScenarios []string `json:"related_scenarios,omitempty"`
	RelatedScreens   []string `json:"related_screens,omitempty"`
	AcceptanceRefs   []string `json:"acceptance_refs,omitempty"`
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

type domainSpec struct {
	Kind                    string
	Slug                    string
	Title                   string
	TemplateID              string
	TemplateName            string
	TemplatePinnedRef       string
	TemplateHealthStatus    string
	ExecutorImage           string
	Summary                 string
	ProblemStatement        string
	TargetUsers             []UserProfile
	CoreScenarios           []Scenario
	Goals                   []string
	NonGoals                []string
	FeatureList             []Feature
	ScreenList              []Screen
	UserFlows               []UserFlow
	DataEntities            []DataEntity
	TemplateConstraints     TemplateConstraints
	AcceptanceCriteria      []AcceptanceCriterion
	ManualReviewPoints      []ManualReviewPoint
	KnownUnknowns           []KnownUnknown
	TaskBundle              []appruns.TaskBundleItem
	AcceptanceChecks        []appruns.AcceptanceCheck
	GoalSummary             string
	TemplateFitReasons      []string
	TemplateFitGaps         []string
	ImplementationPhases    []string
	ManualConstraints       []string
	HumanNotes              []map[string]string
	RequirementHighlights   []string
	SupportingAssumptions   []string
	PreferredAllowedPaths   []string
	PreferredProtectedPaths []string
	KnowledgePack           []appruns.ProfileSkill
	CommandProfile          appruns.CommandProfile
	ContextFiles            appruns.ContextFiles
	SourceSummary           string
	RequirementText         string
	PRDID                   string
	JobID                   string
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
		FeatureList:         spec.FeatureList,
		ScreenList:          spec.ScreenList,
		UserFlows:           spec.UserFlows,
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
	spec.FeatureList = append([]Feature(nil), prd.FeatureList...)
	spec.ScreenList = append([]Screen(nil), prd.ScreenList...)
	spec.UserFlows = append([]UserFlow(nil), prd.UserFlows...)
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
	return normalized
}

func buildBundle(spec domainSpec, prd PRD, now time.Time) (Bundle, error) {
	prdJSON, err := marshalJSON(prd)
	if err != nil {
		return Bundle{}, err
	}
	prdMarkdown := renderPRDMarkdown(prd)
	fitReport := renderTemplateFitReport(spec)
	commandProfileJSON, err := json.Marshal(spec.CommandProfile)
	if err != nil {
		return Bundle{}, fmt.Errorf("marshal command profile: %w", err)
	}
	contextFilesJSON, err := json.Marshal(spec.ContextFiles)
	if err != nil {
		return Bundle{}, fmt.Errorf("marshal context files: %w", err)
	}
	humanNotesJSON, err := json.Marshal(spec.HumanNotes)
	if err != nil {
		return Bundle{}, fmt.Errorf("marshal human notes: %w", err)
	}
	builderInput := appruns.BuildInput{
		SchemaVersion:                  defaultSchemaVersion,
		JobID:                          spec.JobID,
		PRDID:                          spec.PRDID,
		TemplateID:                     spec.TemplateID,
		PreparedPRDSubjectVersion:      PRDCompileSourceVersion(prd),
		PreparedTemplateSubjectVersion: TemplateCompileSourceVersion(spec.TemplateID, spec.TemplatePinnedRef),
		ExecutorImage:                  spec.ExecutorImage,
		WorkspacePath:                  filepath.ToSlash(filepath.Join("/workspace", spec.JobID)),
		ArtifactDir:                    filepath.ToSlash(filepath.Join("/artifacts", spec.JobID)),
		GoalSummary:                    spec.GoalSummary,
		TaskBundle:                     spec.TaskBundle,
		AcceptanceChecks:               spec.AcceptanceChecks,
		AllowedPaths:                   append([]string(nil), spec.PreferredAllowedPaths...),
		ProtectedPaths:                 append([]string(nil), spec.PreferredProtectedPaths...),
		KnowledgePack:                  append([]appruns.ProfileSkill(nil), spec.KnowledgePack...),
		CommandProfile:                 commandProfileJSON,
		ContextFiles:                   contextFilesJSON,
		IterationBudget:                3,
		TokenBudget:                    4000,
		HumanNotes:                     humanNotesJSON,
	}
	builderInputJSON, err := marshalJSON(builderInput)
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
		builderInputFileName:     builderInputJSON,
	}
	return Bundle{PRD: prd, BuilderInput: builderInput, Files: files}, nil
}

func buildPRDApprovalRecord(spec domainSpec, prd PRD, prdMarkdown, requirement string, now time.Time) appruns.ApprovalRecord {
	return appruns.ApprovalRecord{
		SchemaVersion:  defaultSchemaVersion,
		ApprovalID:     "approval-prd-" + spec.PRDID,
		ApprovalType:   appruns.ApprovalTypePRD,
		JobID:          spec.JobID,
		PRDID:          spec.PRDID,
		SubjectVersion: PRDApprovalSubjectVersion(prd, []byte(prdMarkdown), []byte(requirement)),
		Status:         appruns.ApprovalStatusApproved,
		RequestedBy:    approvalSystemActor(),
		Decision: &appruns.ApprovalDecision{
			Decision:  appruns.ApprovalStatusApproved,
			DecidedBy: approvalSystemActor(),
			DecidedAt: now.Format(time.RFC3339),
			Comment:   "系统为当前最小编排链路生成已批准 PRD 快照，用于后续执行接线。",
		},
		Summary:       "当前需求已整理为可执行 PRD 快照。",
		EvidencePaths: []string{prdMarkdownFileName, prdJSONFileName, requirementFileName},
		CreatedAt:     now.Format(time.RFC3339),
	}
}

func PRDCompileSourceVersion(prd PRD) string {
	prdID := strings.TrimSpace(prd.ID)
	if prdID == "" {
		prdID = "unknown-prd"
	}
	version := strings.TrimSpace(prd.Version)
	if version == "" {
		version = "unknown"
	}
	data, err := json.Marshal(prd)
	if err != nil {
		return prdID + "@" + version + "@invalid"
	}
	sum := sha256.Sum256(data)
	return prdID + "@" + version + "@sha256:" + hex.EncodeToString(sum[:8])
}

func PRDApprovalSubjectVersion(prd PRD, prdMarkdown, requirement []byte) string {
	base := PRDCompileSourceVersion(prd)
	payload, err := json.Marshal(struct {
		PRDMarkdown string `json:"prd_markdown"`
		Requirement string `json:"requirement"`
	}{
		PRDMarkdown: string(prdMarkdown),
		Requirement: string(requirement),
	})
	if err != nil {
		payload = append(append(append([]byte(nil), prdMarkdown...), '\n'), requirement...)
	}
	sum := sha256.Sum256(payload)
	return base + "@sha256:" + hex.EncodeToString(sum[:8])
}

func buildTemplateApprovalRecord(spec domainSpec, prd PRD, fitReport string, now time.Time) appruns.ApprovalRecord {
	subjectVersion := TemplateApprovalSubjectVersion(spec.TemplateID, spec.TemplatePinnedRef, []byte(fitReport))
	return appruns.ApprovalRecord{
		SchemaVersion:  defaultSchemaVersion,
		ApprovalID:     "approval-template-" + spec.TemplateID,
		ApprovalType:   appruns.ApprovalTypeTemplate,
		JobID:          spec.JobID,
		PRDID:          spec.PRDID,
		SubjectVersion: subjectVersion,
		Status:         appruns.ApprovalStatusApproved,
		RequestedBy:    approvalSystemActor(),
		Decision: &appruns.ApprovalDecision{
			Decision:  appruns.ApprovalStatusApproved,
			DecidedBy: approvalSystemActor(),
			DecidedAt: now.Format(time.RFC3339),
			Comment:   "系统为当前最小编排链路生成已批准模板快照，用于 builder-input 接线。",
		},
		Summary:       "当前模板选择已冻结到本次执行快照。",
		EvidencePaths: []string{fitReportFileName, prdJSONFileName},
		CreatedAt:     now.Format(time.RFC3339),
	}
}

func TemplateCompileSourceVersion(templateID, pinnedRef string) string {
	trimmedTemplateID := strings.TrimSpace(templateID)
	if trimmedTemplateID == "" {
		trimmedTemplateID = "unknown-template"
	}
	trimmedPinnedRef := strings.TrimSpace(pinnedRef)
	if trimmedPinnedRef == "" {
		trimmedPinnedRef = "unknown"
	}
	return "selected-template@" + trimmedTemplateID + "@" + trimmedPinnedRef
}

func TemplateApprovalSubjectVersion(templateID, pinnedRef string, fitReport []byte) string {
	base := TemplateCompileSourceVersion(templateID, pinnedRef)
	sum := sha256.Sum256(fitReport)
	return base + "@sha256:" + hex.EncodeToString(sum[:8])
}

func approvalSystemActor() appruns.ApprovalActor {
	return appruns.ApprovalActor{ActorType: "system", ActorID: "appfactory-prepare"}
}

func WriteBundle(outputDir string, bundle Bundle) error {
	if strings.TrimSpace(outputDir) == "" {
		return fmt.Errorf("output dir is required")
	}
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return fmt.Errorf("create output dir: %w", err)
	}
	for _, name := range bundle.FileNames() {
		path := filepath.Join(outputDir, name)
		if err := fileutil.WriteFileAtomic(path, bundle.Files[name], 0o644); err != nil {
			return fmt.Errorf("write %s: %w", name, err)
		}
	}
	return nil
}

func (bundle Bundle) FileNames() []string {
	result := make([]string, 0, len(bundle.Files))
	for name := range bundle.Files {
		result = append(result, name)
	}
	sort.Strings(result)
	return result
}

func analyzeRequirement(request Request, requirementText string) domainSpec {
	lower := strings.ToLower(requirementText)
	if isBookkeepingRequirement(lower, requirementText) {
		return compileBookkeepingSpec(request, requirementText)
	}
	return compileGenericSpec(request, requirementText)
}

func isBookkeepingRequirement(lower, raw string) bool {
	keywords := []string{"记账", "账单", "收支", "expense", "bookkeeping", "budget", "finance"}
	for _, keyword := range keywords {
		if strings.Contains(lower, strings.ToLower(keyword)) || strings.Contains(raw, keyword) {
			return true
		}
	}
	return false
}

func compileBookkeepingSpec(request Request, requirementText string) domainSpec {
	title := strings.TrimSpace(request.TitleHint)
	if title == "" {
		title = "轻量记账 App"
	}
	now := time.Now().UTC()
	if request.Now != nil {
		now = request.Now()
	}
	jobID := fallbackGeneratedID(request.JobID, "job-bookkeeping-lite", request.RequirementSource, now)
	prdID := fallbackGeneratedID(request.PRDID, "prd-bookkeeping-lite", request.RequirementSource, now)
	selectedTemplateLabel := fallbackID(request.TemplateID, "flutter-finance-lite")
	templateID := strings.TrimSpace(request.TemplateID)
	preferredTemplateIDs := []string{"flutter-finance-lite"}
	if templateID != "" {
		preferredTemplateIDs = []string{templateID}
	}
	realBuild := request.RealBuild || strings.TrimSpace(request.ExecutorImage) != ""
	executorImage := strings.TrimSpace(request.ExecutorImage)
	if realBuild && executorImage == "" {
		executorImage = "picoclaw/appfactory-builder:local"
	}
	reasons := []string{
		"模板能力覆盖列表、录入表单、统计卡片和底部导航，适合记账 MVP。",
		"长期技术基线已经冻结为 Flutter，当前模板与目标栈一致。",
		"P0 只验证 Builder 链路和成本，不要求真实上架流程，轻量 finance 模板风险最低。",
	}
	gaps := []string{
		"当前仍未把模拟器安装、启动和截图验证接进默认链路。",
		"分类图标、视觉品牌和账单导入等非核心增强点暂不进入本轮。",
	}
	flutterProfile := appruns.NewFlutterAndroidProfile()
	acceptanceChecks := []appruns.AcceptanceCheck{{CheckID: "check-context-ready", Label: "准备上下文文件", Stage: "baseline", Required: true, Commands: []string{"echo context-ready"}, SuccessCriteria: "上下文装载命令返回 0。", TimeoutSeconds: 30}, {CheckID: "check-bookkeeping-scope", Label: "确认记账 MVP 范围", Stage: "baseline", Required: true, Commands: []string{"echo bookkeeping-scope-home-entry-ledger"}, SuccessCriteria: "最小范围被明确限定在首页、录入页和列表页。", TimeoutSeconds: 30}, {CheckID: "check-plan-ready", Label: "确认 Builder 输入包可执行", Stage: "cheap", Required: true, Commands: []string{"echo builder-input-ready"}, SuccessCriteria: "Builder 输入包与最小计划文件已生成。", TimeoutSeconds: 30}}
	acceptanceChecks = append(acceptanceChecks, defaultFlutterFallbackChecks()...)
	goalSummary := "生成一个围绕首页概览、记账录入、账单列表三块核心功能的记账 App 最小输入包，并驱动当前 P0 Builder 默认执行器写入交接探针，证明 inspect/edit/validate 闭环可用。"
	implementationPhases := []string{"阶段 1：冻结记账 App 最小范围，确认首页、录入页、列表页和数据模型。", "阶段 2：把范围拆成 task bundle，生成 Builder 可执行的 acceptance checks。", "阶段 3：通过当前 P0 adapter 写入交接探针并跑通最小链路，为后续接入真实 Flutter Builder runtime 留接口。"}
	manualConstraints := []string{"当前阶段不接入登录、远程同步、上架流程。", "日志和构建报告保留英文输出，文档与计划说明使用中文。", "Builder 只允许执行白名单命令，避免在 P0 阶段扩散风险。", "默认无 real build 路径只验证 seed 工作区和交接探针，不再把仓库内硬编码业务 Dart 生成当作完成标准。"}
	humanNotes := []map[string]string{{"note_id": "note-scope", "summary": "当前实验只验证需求整理、交接探针和 Builder 链路，不追求真实 APK。", "scope": "engineering"}, {"note_id": "note-template", "summary": "优先匹配 " + selectedTemplateLabel + "，避免引入复杂模板依赖。", "scope": "product"}}
	commandProfile := appruns.CommandProfile{ProfileName: "prepare-p0", AllowedStages: []string{"baseline", "cheap"}, AllowedCommands: []string{"echo", "grep"}, DeniedCommands: []string{"rm", "sudo"}, MaxSingleCommandSeconds: 60, MaxParallelCommands: 1, NetworkPolicy: "disabled", WritableRoots: []string{"lib", "assets", "."}, EnvAllowlist: []string{"PATH", "HOME"}}
	if realBuild {
		gaps = []string{
			"flutter-finance-lite 只是静态 seed，当前还缺少真实的记一笔流程、账单列表交互和本地持久化实现。",
			"当前 seed 仍是 Flutter 默认 counter demo，必须由执行器主动替换，不能把可编译 demo 当成完成结果。",
			"当前已切到真实 Flutter analyze/test/build 链路，但模拟器安装、启动和截图验证仍未接入默认 flow。",
			"分类图标、视觉品牌和账单导入等非核心增强点暂不进入本轮。",
		}
		acceptanceChecks = flutterProfile.AcceptanceChecks()
		goalSummary = "在 " + selectedTemplateLabel + " 模板基础上完成一个可用的离线记账 MVP，必须彻底替换默认 counter demo，至少实现首页概览、记一笔录入、账单列表和本地持久化，并通过 analyze、test 和 debug APK 构建验证。"
		implementationPhases = []string{"阶段 1：先冻结领域模型和本地存储边界，再补齐首页、记一笔、账单列表三块核心页面。", "阶段 2：完成录入保存、列表回显、首页概览刷新和应用重启后的本地数据恢复，并保持修改范围收敛在 Flutter 应用层。", "阶段 3：通过 analyze、test、功能接线检查和 debug APK 构建完成低风险收口，不把静态占位页面视为完成。"}
		manualConstraints = []string{"当前阶段不接入登录、远程同步、上架流程。", "日志和构建报告保留英文输出，文档与计划说明使用中文。", "真实验证默认依赖 Builder 容器镜像，不再使用 echo 级 smoke check。", "不得把现有静态 seed 页面当作交付结果，也不得通过删除或弱化验收线索来规避实现。", "如果结果仍保留 Flutter 默认 counter demo 文案、计数器状态或测试，整次 run 必须判失败。"}
		humanNotes = []map[string]string{{"note_id": "note-real-build", "summary": "该输入包用于交付可运行的记账 MVP，不只是验证 Flutter 工具链可用。", "scope": "engineering"}, {"note_id": "note-template", "summary": selectedTemplateLabel + " 只是起始 seed，必须继续补齐录入、列表和本地持久化。", "scope": "product"}, {"note_id": "note-no-counter-demo", "summary": "默认 Flutter counter demo 必须被彻底替换，任何残留都视为当前执行器未完成任务。", "scope": "engineering"}}
		commandProfile = flutterProfile.CommandProfile
	}
	return domainSpec{
		Kind:             "bookkeeping",
		Slug:             "bookkeeping-lite",
		Title:            title,
		TemplateID:       templateID,
		ExecutorImage:    executorImage,
		RequirementText:  requirementText,
		PRDID:            prdID,
		JobID:            jobID,
		SourceSummary:    sourceSummary(request.RequirementSource),
		Summary:          "为个人用户提供一个只覆盖最小核心流程的 Android 记账应用，优先验证需求整理、PRD 生成、Builder 输入包生成和最小执行链路。",
		ProblemStatement: "当前实验阶段需要一条从自然语言需求到 Builder 输入包的最小闭环，用来验证 PicoClaw 能否把简单记账需求整理成结构化规格，并驱动 Builder 执行最小任务链。",
		TargetUsers: []UserProfile{
			{UserID: "user-office-worker", Label: "个人记账用户", Summary: "希望快速记录日常收支，不想维护复杂预算系统。", PainPoints: []string{"记录动作过重", "很难快速回看最近消费", "实验阶段不需要注册登录"}},
			{UserID: "user-validator", Label: "内部验证人员", Summary: "需要一眼看出首页、录入页、列表页是否已被清晰定义。", PainPoints: []string{"输入层散乱", "Builder 输入包经常缺上下文"}},
		},
		CoreScenarios: []Scenario{
			{ScenarioID: "scenario-quick-add", Title: "快速新增一笔收支", Summary: "用户能在数秒内录入金额、分类、日期和备注。", PrimaryUserRefs: []string{"user-office-worker"}},
			{ScenarioID: "scenario-ledger", Title: "查看最近账单", Summary: "用户可以按时间顺序浏览最近账单并确认金额与分类。", PrimaryUserRefs: []string{"user-office-worker"}},
			{ScenarioID: "scenario-summary", Title: "查看本月概览", Summary: "用户能在首页看到收入、支出和结余概览。", PrimaryUserRefs: []string{"user-office-worker", "user-validator"}},
		},
		Goals: []string{
			"输出完整 PRD.md 和 PRD.json，覆盖首页、记账录入、账单列表三个核心范围。",
			"生成可直接被当前 P0 Builder 链路消费的 builder-input.json。",
			"将任务拆成最小 task bundle，便于后续继续收敛到稳定的 thin executor 主线。",
		},
		NonGoals: []string{
			"不做账号体系、云同步、多人协作。",
			"不做应用上架、签名、隐私政策页面。",
			"不做复杂预算、报税、OCR 导入。",
		},
		FeatureList: []Feature{
			{FeatureID: "feature-home-summary", Title: "首页收支概览", Summary: "展示本月收入、支出、结余和最近三笔账单。", Priority: "p0", Required: true, RelatedScenarios: []string{"scenario-summary"}, RelatedScreens: []string{"screen-home"}, AcceptanceRefs: []string{"ac-home", "ac-navigation"}},
			{FeatureID: "feature-add-entry", Title: "记一笔", Summary: "支持录入金额、类型、分类、日期和备注。", Priority: "p0", Required: true, RelatedScenarios: []string{"scenario-quick-add"}, RelatedScreens: []string{"screen-entry-form"}, AcceptanceRefs: []string{"ac-entry"}},
			{FeatureID: "feature-ledger", Title: "账单列表", Summary: "按时间倒序展示账单，并显示分类和金额。", Priority: "p0", Required: true, RelatedScenarios: []string{"scenario-ledger"}, RelatedScreens: []string{"screen-ledger"}, AcceptanceRefs: []string{"ac-ledger"}},
			{FeatureID: "feature-local-data", Title: "本地数据持久化", Summary: "账单数据本地持久化，关闭应用后仍能恢复。", Priority: "p1", Required: true, RelatedScenarios: []string{"scenario-quick-add", "scenario-ledger"}, RelatedScreens: []string{"screen-entry-form", "screen-ledger"}, AcceptanceRefs: []string{"ac-persistence"}},
		},
		ScreenList: []Screen{
			{ScreenID: "screen-home", Name: "首页", Purpose: "显示本月概览和最近账单入口。", PrimaryFeatures: []string{"feature-home-summary", "feature-ledger"}},
			{ScreenID: "screen-entry-form", Name: "记账录入页", Purpose: "完成一笔收支的新增录入。", PrimaryFeatures: []string{"feature-add-entry"}},
			{ScreenID: "screen-ledger", Name: "账单列表页", Purpose: "浏览、筛选和核对近期账单。", PrimaryFeatures: []string{"feature-ledger", "feature-local-data"}},
		},
		UserFlows: []UserFlow{
			{FlowID: "flow-add-expense", Title: "新增支出", Steps: []FlowStep{{StepID: "step-open-form", Title: "从首页进入记账录入页", ScreenRef: "screen-home", Actor: "个人记账用户", ExpectedResult: "能看到金额和分类输入控件"}, {StepID: "step-fill-entry", Title: "填写金额、分类、日期和备注", ScreenRef: "screen-entry-form", Actor: "个人记账用户", ExpectedResult: "表单校验通过"}, {StepID: "step-save-entry", Title: "保存账单并返回首页", ScreenRef: "screen-entry-form", Actor: "个人记账用户", ExpectedResult: "首页和列表页能看到新增账单"}}},
			{FlowID: "flow-review-ledger", Title: "查看账单列表", Steps: []FlowStep{{StepID: "step-open-ledger", Title: "从首页切到账单列表页", ScreenRef: "screen-home", Actor: "个人记账用户", ExpectedResult: "页面展示最近账单"}, {StepID: "step-confirm-entry", Title: "核对账单金额和分类", ScreenRef: "screen-ledger", Actor: "个人记账用户", ExpectedResult: "关键信息易于识别"}}},
		},
		DataEntities: []DataEntity{
			{EntityID: "entity-entry", Name: "账单记录", Source: "local_storage", Fields: []DataField{{Name: "entry_id", Type: "string", Required: true, Description: "账单唯一标识"}, {Name: "entry_type", Type: "enum[income,expense]", Required: true, Description: "收入或支出"}, {Name: "amount", Type: "decimal", Required: true, Description: "金额"}, {Name: "category", Type: "string", Required: true, Description: "分类"}, {Name: "occurred_on", Type: "date", Required: true, Description: "发生日期"}, {Name: "note", Type: "string", Required: false, Description: "备注"}}},
			{EntityID: "entity-summary", Name: "月度汇总", Source: "derived", Fields: []DataField{{Name: "month", Type: "string", Required: true, Description: "月份标识"}, {Name: "income_total", Type: "decimal", Required: true, Description: "本月收入总额"}, {Name: "expense_total", Type: "decimal", Required: true, Description: "本月支出总额"}, {Name: "balance", Type: "decimal", Required: true, Description: "结余"}}},
		},
		TemplateConstraints:     TemplateConstraints{Stack: "flutter", AndroidRequired: true, RequiredCapabilities: []string{"list", "form", "local-storage", "bottom-navigation", "summary-card"}, ExcludedLicenses: []string{"GPL-3.0"}, PreferredTemplateIDs: preferredTemplateIDs},
		AcceptanceCriteria:      []AcceptanceCriterion{{CriterionID: "ac-home", Label: "首页展示本月概览", Category: "functional", Required: true, Description: "首页必须展示收入、支出、结余和最近账单入口。"}, {CriterionID: "ac-entry", Label: "记账录入链路完整", Category: "functional", Required: true, Description: "用户可以新增一笔账单并返回首页。"}, {CriterionID: "ac-ledger", Label: "账单列表可浏览", Category: "functional", Required: true, Description: "账单列表按时间倒序显示，并展示金额和分类。"}, {CriterionID: "ac-persistence", Label: "本地持久化可恢复", Category: "smoke", Required: true, Description: "应用重启后仍能读取本地账单数据。"}, {CriterionID: "ac-navigation", Label: "核心页面可导航", Category: "ui", Required: true, Description: "首页、录入页、列表页之间的入口明确且可达。"}},
		ManualReviewPoints:      []ManualReviewPoint{{PointID: "mrp-copy", Summary: "检查首页与录入页中文文案是否一致", Reason: "P0 阶段主要靠模板改造，文案容易残留模板默认值。", Owner: "product"}, {PointID: "mrp-data", Summary: "确认本地持久化方案不引入额外后端依赖", Reason: "当前实验阶段明确不接入云端服务。", Owner: "engineering"}},
		KnownUnknowns:           []KnownUnknown{{Question: "是否需要在下一阶段加入按分类过滤或图表统计", Impact: "medium", Owner: "product"}, {Question: "后续设备验证与恢复闭环是否沿用当前 acceptance check 结构", Impact: "medium", Owner: "engineering"}},
		TaskBundle:              []appruns.TaskBundleItem{{TaskID: "task-domain-models", Title: "冻结账单与汇总领域模型", Category: "domain", TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, Objective: "先明确记账 App 的实体字段、派生汇总和目录落点，避免后续页面和存储反复漂移。", Priority: "p0", RelatedRequirements: []string{"feature-local-data", "feature-home-summary"}, TargetPaths: []string{"lib/models/entry.dart", "lib/models/summary.dart"}, OutputExpectations: []string{"账单实体字段稳定", "月度汇总可从账单实体推导"}, CompletionCriteria: []string{"账单记录包含金额、分类、日期和备注", "汇总模型可以表达收入、支出和结余"}, RiskNotes: []string{"如果实体字段频繁变化，后续页面和测试都会跟着返工"}}, {TaskID: "task-storage-wiring", Title: "接通本地持久化与仓储边界", Category: "storage", TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, Objective: "把本地持久化方案和 repository 边界固定下来，确保新增账单后可恢复。", Priority: "p0", RelatedRequirements: []string{"feature-local-data"}, Dependencies: []string{"task-domain-models"}, TargetPaths: []string{"lib/repositories/entry_repository.dart", "pubspec.yaml"}, OutputExpectations: []string{"明确的本地持久化实现", "账单读写入口稳定"}, CompletionCriteria: []string{"代码或依赖中出现明确的本地持久化实现", "应用重启后仍能读取账单列表"}, RiskNotes: []string{"不要同时引入两个同类本地存储方案"}}, {TaskID: "task-screen-scaffold", Title: "搭建首页、录入页和列表页骨架", Category: "screen", TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, Objective: "先把三块核心页面、导航入口和主要视觉区域搭出来，不把默认 seed 页面当作完成结果。", Priority: "p0", RelatedRequirements: []string{"feature-home-summary", "feature-add-entry", "feature-ledger"}, Dependencies: []string{"task-domain-models"}, TargetPaths: []string{"lib/main.dart", "lib/views/home_page.dart", "lib/views/entry_form_page.dart", "lib/views/entry_list_page.dart"}, OutputExpectations: []string{"首页概览区", "记一笔表单骨架", "账单列表骨架"}, CompletionCriteria: []string{"首页、录入页、列表页之间入口明确可达", "默认 counter demo 页面与文案已被移除"}, RiskNotes: []string{"页面骨架完成不等于交互链路完成"}}, {TaskID: "task-flow-wiring", Title: "接通记一笔与首页刷新主流程", Category: "flow", TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, Objective: "把录入保存、首页概览刷新和账单列表回显串成一个完整功能闭环。", Priority: "p0", RelatedRequirements: []string{"feature-add-entry", "feature-home-summary", "feature-ledger", "feature-local-data"}, Dependencies: []string{"task-storage-wiring", "task-screen-scaffold"}, TargetPaths: []string{"lib/controllers/home_controller.dart", "lib/controllers/entry_form_controller.dart", "lib/controllers/entry_list_controller.dart", "lib/repositories/entry_repository.dart", "test/widget_test.dart"}, OutputExpectations: []string{"保存动作可触发持久化", "首页和列表页能看到新增账单"}, CompletionCriteria: []string{"录入表单字段齐全并可保存", "新增账单后首页和列表页都能反映最新数据"}, RiskNotes: []string{"不要为了接线方便重做整套页面结构或状态管理"}}, {TaskID: "task-validation-closure", Title: "完成 analyze、test 与 APK 构建收口", Category: "validation", TaskType: appruns.BuilderRuntimeTaskTypeClosureRepair, Objective: "在功能闭环接通后，通过低风险修复把工作区收敛到 analyze、test 和 debug APK 全通过。", Priority: "p0", RelatedRequirements: []string{"ac-entry", "ac-ledger", "ac-persistence", "ac-navigation"}, Dependencies: []string{"task-flow-wiring"}, TargetPaths: []string{"lib/main.dart", "lib/views/**", "lib/controllers/**", "lib/repositories/**", "test/**", "pubspec.yaml"}, OutputExpectations: []string{"静态检查通过", "测试通过", "可构建 Debug APK"}, CompletionCriteria: []string{"flutter analyze 无错误", "flutter test 全通过", "flutter build apk --debug 成功"}, RiskNotes: []string{"只做低风险收口，不把单点失败扩展为大面积自由重构"}}},
		AcceptanceChecks:        acceptanceChecks,
		GoalSummary:             goalSummary,
		TemplateFitReasons:      reasons,
		TemplateFitGaps:         gaps,
		ImplementationPhases:    implementationPhases,
		ManualConstraints:       manualConstraints,
		HumanNotes:              humanNotes,
		RequirementHighlights:   []string{"需求入口是自然语言，不再要求人工手写 builder-input。", "当前例子是简单记账 App，只考虑功能实现，不考虑上架。", "验证重点是 PRD、计划和 Builder 输入包能否稳定生成。"},
		SupportingAssumptions:   []string{"默认离线单机使用。", "数据仅做本地存储。", "视觉样式复用模板默认风格，后续再单独收敛。"},
		PreferredAllowedPaths:   preferredAllowedPaths(realBuild),
		PreferredProtectedPaths: preferredProtectedPaths(realBuild),
		KnowledgePack:           append([]appruns.ProfileSkill(nil), flutterProfile.KnowledgePack...),
		CommandProfile:          commandProfile,
		ContextFiles:            appruns.ContextFiles{PRDMarkdownPath: prdMarkdownFileName, PRDJSONPath: prdJSONFileName, TemplateFitReportPath: fitReportFileName, ImplementationPlanPath: planFileName, ManualConstraintsPath: constraintsFileName, SupportingFiles: []string{requirementFileName, prdApprovalFileName, templateApprovalFileName}},
	}
}

func preferredAllowedPaths(realBuild bool) []string {
	if realBuild {
		return append([]string(nil), appruns.NewFlutterAndroidProfile().AllowedPaths...)
	}
	return []string{"lib/**", "assets/**", "pubspec.yaml", "test/**"}
}

func preferredProtectedPaths(realBuild bool) []string {
	if realBuild {
		return append([]string(nil), appruns.NewFlutterAndroidProfile().ProtectedPaths...)
	}
	return []string{"android/**", "ios/**", "linux/**", "macos/**", "windows/**"}
}

func compileGenericSpec(request Request, requirementText string) domainSpec {
	title := strings.TrimSpace(request.TitleHint)
	if title == "" {
		title = "Android MVP App"
	}
	now := time.Now().UTC()
	if request.Now != nil {
		now = request.Now()
	}
	jobID := fallbackGeneratedID(request.JobID, "job-generic-mvp", request.RequirementSource, now)
	prdID := fallbackGeneratedID(request.PRDID, "prd-generic-mvp", request.RequirementSource, now)
	selectedTemplateLabel := fallbackID(request.TemplateID, "flutter-template-demo")
	templateID := strings.TrimSpace(request.TemplateID)
	preferredTemplateIDs := []string{"flutter-template-demo"}
	if templateID != "" {
		preferredTemplateIDs = []string{templateID}
	}
	return domainSpec{
		Kind:                    "generic",
		Slug:                    "generic-mvp",
		Title:                   title,
		TemplateID:              templateID,
		RequirementText:         requirementText,
		PRDID:                   prdID,
		JobID:                   jobID,
		SourceSummary:           sourceSummary(request.RequirementSource),
		Summary:                 "把自然语言需求整理成最小 Android MVP 规格，并输出 Builder 可执行输入包。",
		ProblemStatement:        "当前实验阶段需要先验证需求编译和 Builder 输入包生成，而不是让用户直接编写内部协议对象。",
		TargetUsers:             []UserProfile{{UserID: "user-primary", Label: "目标用户", Summary: "等待进一步细化的 MVP 使用者。", PainPoints: []string{"原始需求不结构化", "Builder 缺少稳定上下文"}}},
		CoreScenarios:           []Scenario{{ScenarioID: "scenario-primary", Title: "完成主流程", Summary: "用户可以进入主流程并完成一次核心动作。", PrimaryUserRefs: []string{"user-primary"}}},
		Goals:                   []string{"输出 PRD 和 builder-input。", "把需求压缩为 Builder 可执行任务。"},
		NonGoals:                []string{"不处理上架和复杂外部集成。"},
		FeatureList:             []Feature{{FeatureID: "feature-primary", Title: "主流程页面", Summary: "定义主页面和一次核心交互。", Priority: "p0", Required: true, RelatedScenarios: []string{"scenario-primary"}, RelatedScreens: []string{"screen-main"}, AcceptanceRefs: []string{"ac-primary"}}},
		ScreenList:              []Screen{{ScreenID: "screen-main", Name: "主页面", Purpose: "承载 MVP 的单一主流程。", PrimaryFeatures: []string{"feature-primary"}}},
		UserFlows:               []UserFlow{{FlowID: "flow-primary", Title: "主流程", Steps: []FlowStep{{StepID: "step-enter-main", Title: "进入主页面", ScreenRef: "screen-main", Actor: "目标用户", ExpectedResult: "可以看到核心入口"}, {StepID: "step-complete-primary", Title: "完成核心操作", ScreenRef: "screen-main", Actor: "目标用户", ExpectedResult: "主流程闭环完成"}}}},
		DataEntities:            []DataEntity{{EntityID: "entity-primary", Name: "核心记录", Source: "local_storage", Fields: []DataField{{Name: "record_id", Type: "string", Required: true, Description: "主记录标识"}, {Name: "title", Type: "string", Required: true, Description: "记录标题"}}}},
		TemplateConstraints:     TemplateConstraints{Stack: "flutter", AndroidRequired: true, RequiredCapabilities: []string{"single-screen", "local-state"}, PreferredTemplateIDs: preferredTemplateIDs},
		AcceptanceCriteria:      []AcceptanceCriterion{{CriterionID: "ac-primary", Label: "主流程可达", Category: "functional", Required: true, Description: "应用可进入主页面并完成一次主操作。"}},
		ManualReviewPoints:      []ManualReviewPoint{{PointID: "mrp-primary", Summary: "确认需求是否还需要拆更多页面", Reason: "当前只保留单屏 MVP。", Owner: "product"}},
		KnownUnknowns:           []KnownUnknown{{Question: "是否需要额外页面或外部集成", Impact: "medium", Owner: "product"}},
		TaskBundle:              []appruns.TaskBundleItem{{TaskID: "task-generic-domain", Title: "冻结 MVP 核心记录模型", Category: "domain", TaskType: appruns.BuilderRuntimeTaskTypeSingleFileEdit, Objective: "先把最小数据对象和命名边界固定下来，避免主流程描述漂移。", Priority: "p0", TargetPaths: []string{"lib/models/**"}, CompletionCriteria: []string{"核心记录字段定义完整", "数据对象命名与主流程一致"}}, {TaskID: "task-generic-screen", Title: "搭建 MVP 主页面骨架", Category: "screen", TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, Objective: "让 Builder 看到明确的主页面入口和一次核心交互。", Priority: "p0", Dependencies: []string{"task-generic-domain"}, TargetPaths: []string{"lib/**"}, CompletionCriteria: []string{"主页面定义完整", "核心操作入口可见"}}, {TaskID: "task-generic-validation", Title: "完成最小输入包校验", Category: "validation", TaskType: appruns.BuilderRuntimeTaskTypeClosureRepair, Objective: "确认 builder 输入包和上下文文件可被后续 Builder 消费。", Priority: "p0", Dependencies: []string{"task-generic-screen"}, TargetPaths: []string{"implementation-plan.md", "builder-input.json"}, CompletionCriteria: []string{"builder-input.json 已生成", "主流程可被描述为可执行任务"}}},
		AcceptanceChecks:        []appruns.AcceptanceCheck{{CheckID: "check-primary", Label: "确认输入包生成", Stage: "baseline", Required: true, Commands: []string{"echo generic-input-ready"}, SuccessCriteria: "Builder 输入包命令返回 0。", TimeoutSeconds: 30}},
		GoalSummary:             "将通用需求整理成基于 " + selectedTemplateLabel + " 的最小 Android MVP 输入包。",
		TemplateFitReasons:      []string{"默认 Flutter 模板足以承载单屏 MVP。"},
		TemplateFitGaps:         []string{"具体页面细节仍需下一轮细化。"},
		ImplementationPhases:    []string{"阶段 1：抽取需求主流程。", "阶段 2：生成 PRD 与 Builder 输入包。"},
		ManualConstraints:       []string{"当前只验证最小编排链路。"},
		HumanNotes:              []map[string]string{{"note_id": "note-primary", "summary": "后续根据真实领域需求替换 generic 模板。", "scope": "engineering"}},
		RequirementHighlights:   []string{requirementText},
		SupportingAssumptions:   []string{"默认离线单机运行。"},
		PreferredAllowedPaths:   []string{"lib/**", "pubspec.yaml"},
		PreferredProtectedPaths: []string{"android/**", "ios/**"},
		CommandProfile:          appruns.CommandProfile{ProfileName: "prepare-p0", AllowedStages: []string{"baseline"}, AllowedCommands: []string{"echo"}, DeniedCommands: []string{"rm", "sudo"}, MaxSingleCommandSeconds: 60, MaxParallelCommands: 1, NetworkPolicy: "disabled", WritableRoots: []string{"lib", "."}, EnvAllowlist: []string{"PATH", "HOME"}},
		ContextFiles:            appruns.ContextFiles{PRDMarkdownPath: prdMarkdownFileName, PRDJSONPath: prdJSONFileName, TemplateFitReportPath: fitReportFileName, ImplementationPlanPath: planFileName, ManualConstraintsPath: constraintsFileName, SupportingFiles: []string{requirementFileName, prdApprovalFileName, templateApprovalFileName}},
	}
}

func renderRequirement(spec domainSpec) string {
	var builder strings.Builder
	builder.WriteString("# 原始需求\n\n")
	builder.WriteString(spec.RequirementText)
	builder.WriteString("\n\n## 需求摘录\n\n")
	for _, item := range spec.RequirementHighlights {
		builder.WriteString("- ")
		builder.WriteString(item)
		builder.WriteString("\n")
	}
	builder.WriteString("\n## 当前假设\n\n")
	for _, item := range spec.SupportingAssumptions {
		builder.WriteString("- ")
		builder.WriteString(item)
		builder.WriteString("\n")
	}
	return builder.String()
}

func renderPRDMarkdown(prd PRD) string {
	var builder strings.Builder
	builder.WriteString("# ")
	builder.WriteString(prd.Title)
	builder.WriteString("\n\n")
	builder.WriteString("- PRD ID: ")
	builder.WriteString(prd.ID)
	builder.WriteString("\n")
	builder.WriteString("- Version: ")
	builder.WriteString(prd.Version)
	builder.WriteString("\n")
	builder.WriteString("- Status: ")
	builder.WriteString(prd.Status)
	builder.WriteString("\n\n## 摘要\n\n")
	builder.WriteString(prd.Summary)
	builder.WriteString("\n\n## 问题陈述\n\n")
	builder.WriteString(prd.ProblemStatement)
	builder.WriteString("\n\n## 目标用户\n\n")
	for _, user := range prd.TargetUsers {
		builder.WriteString("- ")
		builder.WriteString(user.Label)
		builder.WriteString("：")
		builder.WriteString(user.Summary)
		if len(user.PainPoints) > 0 {
			builder.WriteString(" 关键痛点：")
			builder.WriteString(strings.Join(user.PainPoints, "；"))
		}
		builder.WriteString("\n")
	}
	builder.WriteString("\n## 核心场景\n\n")
	for _, scenario := range prd.CoreScenarios {
		builder.WriteString("- ")
		builder.WriteString(scenario.Title)
		builder.WriteString("：")
		builder.WriteString(scenario.Summary)
		builder.WriteString("\n")
	}
	builder.WriteString("\n## 目标\n\n")
	for _, item := range prd.Goals {
		builder.WriteString("- ")
		builder.WriteString(item)
		builder.WriteString("\n")
	}
	builder.WriteString("\n## 非目标\n\n")
	for _, item := range prd.NonGoals {
		builder.WriteString("- ")
		builder.WriteString(item)
		builder.WriteString("\n")
	}
	builder.WriteString("\n## 功能列表\n\n")
	for _, feature := range prd.FeatureList {
		builder.WriteString("- ")
		builder.WriteString(feature.Title)
		builder.WriteString("（")
		builder.WriteString(strings.ToUpper(feature.Priority))
		builder.WriteString("）：")
		builder.WriteString(feature.Summary)
		builder.WriteString("\n")
	}
	builder.WriteString("\n## 页面列表\n\n")
	for _, screen := range prd.ScreenList {
		builder.WriteString("- ")
		builder.WriteString(screen.Name)
		builder.WriteString("：")
		builder.WriteString(screen.Purpose)
		builder.WriteString("\n")
	}
	builder.WriteString("\n## 用户流程\n\n")
	for _, flow := range prd.UserFlows {
		builder.WriteString("### ")
		builder.WriteString(flow.Title)
		builder.WriteString("\n\n")
		for _, step := range flow.Steps {
			builder.WriteString("- ")
			builder.WriteString(step.Title)
			if step.ExpectedResult != "" {
				builder.WriteString("，期望结果：")
				builder.WriteString(step.ExpectedResult)
			}
			builder.WriteString("\n")
		}
		builder.WriteString("\n")
	}
	builder.WriteString("## 数据实体\n\n")
	for _, entity := range prd.DataEntities {
		builder.WriteString("### ")
		builder.WriteString(entity.Name)
		builder.WriteString("\n\n")
		for _, field := range entity.Fields {
			builder.WriteString("- ")
			builder.WriteString(field.Name)
			builder.WriteString("：")
			builder.WriteString(field.Type)
			if field.Description != "" {
				builder.WriteString("，")
				builder.WriteString(field.Description)
			}
			builder.WriteString("\n")
		}
		builder.WriteString("\n")
	}
	builder.WriteString("## 模板约束\n\n")
	builder.WriteString("- 技术栈：")
	builder.WriteString(prd.TemplateConstraints.Stack)
	builder.WriteString("\n- Android 必须：")
	if prd.TemplateConstraints.AndroidRequired {
		builder.WriteString("是")
	} else {
		builder.WriteString("否")
	}
	builder.WriteString("\n- 必需能力：")
	builder.WriteString(strings.Join(prd.TemplateConstraints.RequiredCapabilities, "、"))
	builder.WriteString("\n\n## 验收标准\n\n")
	for _, criterion := range prd.AcceptanceCriteria {
		builder.WriteString("- ")
		builder.WriteString(criterion.Label)
		builder.WriteString("：")
		builder.WriteString(criterion.Description)
		builder.WriteString("\n")
	}
	builder.WriteString("\n## 人工审核点\n\n")
	for _, point := range prd.ManualReviewPoints {
		builder.WriteString("- ")
		builder.WriteString(point.Summary)
		builder.WriteString("：")
		builder.WriteString(point.Reason)
		builder.WriteString("\n")
	}
	builder.WriteString("\n## 已知未知项\n\n")
	for _, item := range prd.KnownUnknowns {
		builder.WriteString("- ")
		builder.WriteString(item.Question)
		builder.WriteString("（影响：")
		builder.WriteString(item.Impact)
		builder.WriteString("）\n")
	}
	return builder.String()
}

func renderTemplateFitReport(spec domainSpec) string {
	var builder strings.Builder
	builder.WriteString("# 模板适配报告\n\n")
	builder.WriteString("- PRD ID: ")
	builder.WriteString(spec.PRDID)
	builder.WriteString("\n- Template ID: ")
	builder.WriteString(spec.TemplateID)
	if strings.TrimSpace(spec.TemplateName) != "" {
		builder.WriteString("\n- Template Name: ")
		builder.WriteString(spec.TemplateName)
	}
	if strings.TrimSpace(spec.TemplatePinnedRef) != "" {
		builder.WriteString("\n- Pinned Ref: ")
		builder.WriteString(spec.TemplatePinnedRef)
	}
	if strings.TrimSpace(spec.TemplateHealthStatus) != "" {
		builder.WriteString("\n- Health Status: ")
		builder.WriteString(spec.TemplateHealthStatus)
	}
	builder.WriteString("\n- Domain: ")
	builder.WriteString(spec.Kind)
	builder.WriteString("\n\n## 选择理由\n\n")
	for _, item := range spec.TemplateFitReasons {
		builder.WriteString("- ")
		builder.WriteString(item)
		builder.WriteString("\n")
	}
	builder.WriteString("\n## 当前差距\n\n")
	for _, item := range spec.TemplateFitGaps {
		builder.WriteString("- ")
		builder.WriteString(item)
		builder.WriteString("\n")
	}
	builder.WriteString("\n## 结论\n\n")
	builder.WriteString("当前模板可以作为 Builder 执行起点，但仍需在实现阶段补齐缺失功能，不能把 seed 模板直接视为完成结果。\n")
	return builder.String()
}

func renderImplementationPlan(spec domainSpec) string {
	var builder strings.Builder
	builder.WriteString("# 实施计划\n\n")
	builder.WriteString("- Job ID: ")
	builder.WriteString(spec.JobID)
	builder.WriteString("\n- PRD ID: ")
	builder.WriteString(spec.PRDID)
	builder.WriteString("\n\n## 阶段拆分\n\n")
	for _, item := range spec.ImplementationPhases {
		builder.WriteString("- ")
		builder.WriteString(item)
		builder.WriteString("\n")
	}
	builder.WriteString("\n## 任务包\n\n")
	for _, task := range spec.TaskBundle {
		builder.WriteString("### ")
		builder.WriteString(task.Title)
		builder.WriteString("\n\n")
		builder.WriteString("- 目标：")
		builder.WriteString(task.Objective)
		builder.WriteString("\n- 分类：")
		builder.WriteString(string(task.Category))
		if len(task.RelatedRequirements) > 0 {
			builder.WriteString("\n- 关联需求：")
			builder.WriteString(strings.Join(task.RelatedRequirements, ", "))
		}
		if len(task.Dependencies) > 0 {
			builder.WriteString("\n- 依赖任务：")
			builder.WriteString(strings.Join(task.Dependencies, ", "))
		}
		builder.WriteString("\n- 目标路径：")
		builder.WriteString(strings.Join(task.TargetPaths, ", "))
		if len(task.OutputExpectations) > 0 {
			builder.WriteString("\n- 预期输出：")
			builder.WriteString(strings.Join(task.OutputExpectations, "；"))
		}
		builder.WriteString("\n- 完成标准：")
		builder.WriteString(strings.Join(task.CompletionCriteria, "；"))
		if len(task.RiskNotes) > 0 {
			builder.WriteString("\n- 风险提示：")
			builder.WriteString(strings.Join(task.RiskNotes, "；"))
		}
		builder.WriteString("\n\n")
	}
	builder.WriteString("## 验收检查\n\n")
	for _, check := range spec.AcceptanceChecks {
		builder.WriteString("- ")
		builder.WriteString(check.Label)
		builder.WriteString("：stage=")
		builder.WriteString(string(check.Stage))
		builder.WriteString("，commands=")
		builder.WriteString(strings.Join(check.Commands, " && "))
		builder.WriteString("\n")
	}
	return builder.String()
}

func renderManualConstraints(spec domainSpec) string {
	var builder strings.Builder
	builder.WriteString("# 人工约束\n\n")
	for _, item := range spec.ManualConstraints {
		builder.WriteString("- ")
		builder.WriteString(item)
		builder.WriteString("\n")
	}
	builder.WriteString("\n## 人工备注\n\n")
	for _, item := range spec.HumanNotes {
		builder.WriteString("- ")
		builder.WriteString(item["summary"])
		builder.WriteString("\n")
	}
	return builder.String()
}

func defaultFlutterFallbackChecks() []appruns.AcceptanceCheck {
	return []appruns.AcceptanceCheck{
		{
			CheckID:         "check-structural-template-files-ready",
			Label:           "确认 Flutter seed 工作区存在",
			Stage:           "baseline",
			Required:        true,
			Commands:        []string{"grep -q . pubspec.yaml lib/main.dart test/widget_test.dart"},
			SuccessCriteria: "默认 seed 工作区关键文件已就位。",
			TimeoutSeconds:  30,
		},
		{
			CheckID:         "check-legacy-thin-fallback-probe",
			Label:           "确认默认 thin fallback 写入探针",
			Stage:           "cheap",
			Required:        true,
			Commands:        []string{"grep -E \"Generated by PicoClaw thin executor.|'goalSummary':\" lib/picoclaw_executor_probe.dart >/dev/null 2>&1"},
			SuccessCriteria: "默认 thin executor fallback 已在允许目录中写入探针文件，用于证明 legacy fallback 链可用。",
			TimeoutSeconds:  30,
		},
		{
			CheckID:         "check-legacy-thin-fallback-metadata",
			Label:           "确认 fallback 探针包含执行元数据",
			Stage:           "cheap",
			Required:        true,
			Commands:        []string{"grep -E \"'taskCount': [0-9]+,|'acceptanceCheckCount': [0-9]+,\" lib/picoclaw_executor_probe.dart >/dev/null 2>&1"},
			SuccessCriteria: "fallback 探针里已带上 task 与 acceptance check 元数据，供定位 legacy fallback 输入边界，不代表真实 builder-runtime 产出。",
			TimeoutSeconds:  30,
		},
	}
}

func fallbackID(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value != "" {
		return value
	}
	return fallback
}

func fallbackGeneratedID(value, fallback, source string, now time.Time) string {
	value = strings.TrimSpace(value)
	if value != "" {
		return value
	}
	if strings.TrimSpace(source) == "jobs-ui" {
		return fmt.Sprintf("%s_%s", jobsUIObjectLabel(fallback), jobsUITimestamp(now))
	}
	return fallback
}

func jobsUITimestamp(now time.Time) string {
	utc := now.UTC()
	return fmt.Sprintf("%s%03d", utc.Format("20060102150405"), utc.Nanosecond()/1_000_000)
}

func jobsUIObjectLabel(fallback string) string {
	fallback = strings.TrimSpace(fallback)
	switch {
	case strings.HasPrefix(fallback, "job-"):
		return "JOB"
	case strings.HasPrefix(fallback, "prd-"):
		return "PRD"
	default:
		return "TASK"
	}
}

func sourceSummary(source string) string {
	source = strings.TrimSpace(source)
	if source == "" {
		return "inline:requirement"
	}
	return source
}

func marshalJSON(value any) ([]byte, error) {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal json: %w", err)
	}
	return append(data, '\n'), nil
}
