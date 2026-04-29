package prepare

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	appruns "github.com/sipeed/picoclaw/pkg/appfactory/runs"
)

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
	SurfaceList             []InteractionSurface
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

type genericDomainSignals struct {
	AppTitle                 string
	DomainLabel              string
	SummaryLabel             string
	OverviewTitle            string
	OverviewSummary          string
	CreateTitle              string
	CreateSummary            string
	UpdateTitle              string
	UpdateSummary            string
	DeleteTitle              string
	DeleteSummary            string
	InspectTitle             string
	InspectSummary           string
	FilterTitle              string
	FilterSummary            string
	HasFilter                bool
	OverviewFeatureTitle     string
	OverviewFeatureSummary   string
	CollectionFeatureTitle   string
	CollectionFeatureSummary string
	MutationFeatureTitle     string
	MutationFeatureSummary   string
	DeleteFeatureTitle       string
	DeleteFeatureSummary     string
	CreateFieldsExpected     string
	UpdateFieldsExpected     string
	DetailFieldsExpected     string
	FilterExpected           string
	Summary                  string
	ProblemStatement         string
	Goals                    []string
	Highlights               []string
	Entity                   DataEntity
	SummaryEntity            DataEntity
	OverviewAcceptance       string
	ListAcceptance           string
	FormAcceptance           string
	DeleteAcceptance         string
	NavigationAcceptance     string
	ManualReviewSummary      string
	ManualReviewReason       string
	SummaryReviewSummary     string
	SummaryReviewReason      string
	GoalSummary              string
	TemplateFitDomainGap     string
	ImplementationPhases     []string
	ManualConstraints        []string
	HumanNotes               []map[string]string
	TargetUserSummary        string
	TargetUserPainPoints     []string
	ValidatorPainPoints      []string
	DomainCheckPattern       string
	DomainCheckSummary       string
}

type genericRequirementOverlay struct {
	ResolvedTitle         string
	RequirementHighlights []string
}

type genericTopologyPlan struct {
	Overview   bool
	Collection bool
	Mutation   bool
	Inspection bool
	Delete     bool
	Filter     bool
}

const (
	genericSurfaceOverviewID    = "surface-overview"
	genericSurfaceCollectionID  = "surface-collection"
	genericSurfaceMutationID    = "surface-mutation"
	genericSurfaceInspectionID  = "surface-inspection"
	taskBindOverviewSurfaceID   = "task-bind-overview-surface"
	taskBindCollectionSurfaceID = "task-bind-collection-surface"
	taskBindMutationSurfaceID   = "task-bind-mutation-surface"
	taskBindInspectionSurfaceID = "task-bind-inspection-surface"
	taskBindAppEntryID          = "task-bind-app-entry"
)

func analyzeRequirement(request Request, requirementText string) domainSpec {
	lower := strings.ToLower(requirementText)
	if isBookkeepingRequirement(lower, requirementText) {
		return compileBookkeepingSpec(request, requirementText)
	}
	if isInventorySheetLineItemRequirement(lower, requirementText) {
		return compileInventorySheetLineItemSpec(request, requirementText)
	}
	if isRelationRichRequirement(lower, requirementText) {
		return compileRelationRichSpec(request, requirementText)
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

func isRelationRichRequirement(lower, raw string) bool {
	return isProjectTaskTagRequirement(lower, raw)
}

func isProjectTaskTagRequirement(lower, raw string) bool {
	hasTask := strings.Contains(lower, "task") || strings.Contains(raw, "任务")
	hasProject := strings.Contains(lower, "project") || strings.Contains(raw, "项目")
	hasTag := strings.Contains(lower, "tag") || strings.Contains(raw, "标签")
	return hasTask && hasProject && hasTag
}

func isInventorySheetLineItemRequirement(lower, raw string) bool {
	hasInventory := strings.Contains(lower, "inventory") || strings.Contains(raw, "库存")
	hasSheet := strings.Contains(lower, "sheet") || strings.Contains(raw, "库存单") || strings.Contains(raw, "盘点单")
	hasLineItem := strings.Contains(lower, "line item") || strings.Contains(lower, "line-item") || strings.Contains(raw, "明细")
	return hasInventory && hasSheet && hasLineItem
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
		"模板能力覆盖集合浏览、记账动作、统计卡片和底部导航，适合记账 MVP。",
		"长期技术基线已经冻结为 Flutter，当前模板与目标栈一致。",
		"P0 只验证 Builder 链路和成本，不要求真实上架流程，轻量 finance 模板风险最低。",
	}
	gaps := []string{
		"当前仍未把模拟器安装、启动和截图验证接进默认链路。",
		"分类图标、视觉品牌和账单导入等非核心增强点暂不进入本轮。",
	}
	flutterProfile := appruns.NewFlutterAndroidProfile()
	acceptanceChecks := []appruns.AcceptanceCheck{
		{CheckID: "check-context-ready", Label: "准备上下文文件", Stage: "baseline", Required: true, Commands: []string{"echo context-ready"}, SuccessCriteria: "上下文装载命令返回 0。", TimeoutSeconds: 30},
		{CheckID: "check-bookkeeping-scope", Label: "确认记账 MVP 范围", Stage: "baseline", Required: true, Commands: []string{"echo bookkeeping-scope-overview-mutation-collection"}, SuccessCriteria: "最小范围被明确限定在概览承载、记账动作和账单集合浏览。", TimeoutSeconds: 30},
		{CheckID: "check-plan-ready", Label: "确认 Builder 输入包可执行", Stage: "cheap", Required: true, Commands: []string{"echo builder-input-ready"}, SuccessCriteria: "Builder 输入包与最小计划文件已生成。", TimeoutSeconds: 30},
	}
	acceptanceChecks = append(acceptanceChecks, defaultFlutterFallbackChecks()...)
	goalSummary := "生成一个围绕收支概览、记账动作和账单集合浏览三类核心承载的记账 App 最小输入包，并驱动当前 P0 Builder 默认执行器写入交接探针，证明 inspect/edit/validate 闭环可用。"
	implementationPhases := []string{"阶段 1：冻结记账 App 最小范围，确认账单实体、收支汇总与概览/动作/集合三类承载关系。", "阶段 2：把范围拆成 task bundle，生成 Builder 可执行的 acceptance checks。", "阶段 3：通过当前 P0 adapter 写入交接探针并跑通最小链路，为后续接入真实 Flutter Builder runtime 留接口。"}
	manualConstraints := []string{"当前阶段不接入登录、远程同步、上架流程。", "日志和构建报告保留英文输出，文档与计划说明使用中文。", "Builder 只允许执行白名单命令，避免在 P0 阶段扩散风险。", "默认无 real build 路径只验证 seed 工作区和交接探针，不再把仓库内硬编码业务 Dart 生成当作完成标准。"}
	humanNotes := []map[string]string{{"note_id": "note-scope", "summary": "当前实验只验证需求整理、交接探针和 Builder 链路，不追求真实 APK。", "scope": "engineering"}, {"note_id": "note-template", "summary": "优先匹配 " + selectedTemplateLabel + "，避免引入复杂模板依赖。", "scope": "product"}}
	commandProfile := appruns.CommandProfile{ProfileName: "prepare-p0", AllowedStages: []string{"baseline", "cheap"}, AllowedCommands: []string{"echo", "grep"}, DeniedCommands: []string{"rm", "sudo"}, MaxSingleCommandSeconds: 60, MaxParallelCommands: 1, NetworkPolicy: "disabled", WritableRoots: []string{"lib", "assets", "."}, EnvAllowlist: []string{"PATH", "HOME"}}
	if realBuild {
		gaps = []string{
			"flutter-finance-lite 只是静态 seed，当前还缺少真实的记账动作闭环、账单集合交互和本地持久化实现。",
			"当前 seed 仍是 Flutter 默认 counter demo，必须由执行器主动替换，不能把可编译 demo 当成完成结果。",
			"当前已切到真实 Flutter analyze/test/build 链路，但模拟器安装、启动和截图验证仍未接入默认 flow。",
			"分类图标、视觉品牌和账单导入等非核心增强点暂不进入本轮。",
		}
		acceptanceChecks = flutterProfile.AcceptanceChecks()
		goalSummary = "在 " + selectedTemplateLabel + " 模板基础上完成一个可用的离线记账 MVP，必须彻底替换默认 counter demo，至少实现收支概览、记账动作、账单集合浏览和本地持久化，并通过 analyze、test 和 debug APK 构建验证。"
		implementationPhases = []string{"阶段 1：先冻结领域模型和本地存储边界，再补齐概览承载、记账动作和账单集合浏览三块核心承载。", "阶段 2：完成录入保存、集合回显、概览刷新和应用重启后的本地数据恢复，并保持修改范围收敛在 Flutter 应用层。", "阶段 3：通过 analyze、test、功能接线检查和 debug APK 构建完成低风险收口，不把静态占位承载当作完成。"}
		manualConstraints = []string{"当前阶段不接入登录、远程同步、上架流程。", "日志和构建报告保留英文输出，文档与计划说明使用中文。", "真实验证默认依赖 Builder 容器镜像，不再使用 echo 级 smoke check。", "不得把现有静态 seed 页面当作交付结果，也不得通过删除或弱化验收线索来规避实现。", "如果结果仍保留 Flutter 默认 counter demo 文案、计数器状态或测试，整次 run 必须判失败。"}
		humanNotes = []map[string]string{{"note_id": "note-real-build", "summary": "该输入包用于交付可运行的记账 MVP，不只是验证 Flutter 工具链可用。", "scope": "engineering"}, {"note_id": "note-template", "summary": selectedTemplateLabel + " 只是起始 seed，必须继续补齐概览承载、记账动作、账单集合浏览和本地持久化。", "scope": "product"}, {"note_id": "note-no-counter-demo", "summary": "默认 Flutter counter demo 必须被彻底替换，任何残留都视为当前执行器未完成任务。", "scope": "engineering"}}
		commandProfile = flutterProfile.CommandProfile
	}
	return domainSpec{
		Kind:                    "bookkeeping",
		Slug:                    "bookkeeping-lite",
		Title:                   title,
		TemplateID:              templateID,
		ExecutorImage:           executorImage,
		RequirementText:         requirementText,
		PRDID:                   prdID,
		JobID:                   jobID,
		SourceSummary:           sourceSummary(request.RequirementSource),
		Summary:                 "为个人用户提供一个只覆盖最小核心流程的 Android 记账应用，围绕收支概览、记账动作和账单集合浏览验证需求整理、PRD 生成、Builder 输入包生成和最小执行链路。",
		ProblemStatement:        "当前实验阶段需要一条从自然语言需求到 Builder 输入包的最小闭环，用来验证 PicoClaw 能否把简单记账需求整理成结构化规格，并围绕中性交互承载驱动 Builder 执行最小任务链。",
		TargetUsers:             []UserProfile{{UserID: "user-office-worker", Label: "个人记账用户", Summary: "希望快速记录日常收支，不想维护复杂预算系统。", PainPoints: []string{"记录动作过重", "很难快速回看最近消费", "实验阶段不需要注册登录"}}, {UserID: "user-validator", Label: "内部验证人员", Summary: "需要一眼看出概览、记账动作和账单集合浏览承载是否已被清晰定义。", PainPoints: []string{"输入层散乱", "Builder 输入包经常缺上下文"}}},
		CoreScenarios:           []Scenario{{ScenarioID: "scenario-quick-add", Title: "快速新增一笔收支", Summary: "用户能在数秒内录入金额、分类、日期和备注。", PrimaryUserRefs: []string{"user-office-worker"}}, {ScenarioID: "scenario-ledger", Title: "查看最近账单", Summary: "用户可以按时间顺序浏览最近账单并确认金额与分类。", PrimaryUserRefs: []string{"user-office-worker"}}, {ScenarioID: "scenario-summary", Title: "查看本月概览", Summary: "用户能在概览承载单元看到收入、支出和结余概览。", PrimaryUserRefs: []string{"user-office-worker", "user-validator"}}},
		Goals:                   []string{"输出完整 PRD.md 和 PRD.json，覆盖收支概览、记账动作、账单集合浏览三个核心范围。", "生成可直接被当前 P0 Builder 链路消费的 builder-input.json。", "将任务拆成最小 task bundle，便于后续继续收敛到稳定的 thin executor 主线。"},
		NonGoals:                []string{"不做账号体系、云同步、多人协作。", "不做应用上架、签名、隐私政策页面。", "不做复杂预算、报税、OCR 导入。"},
		FeatureList:             buildBookkeepingFeatureList(),
		SurfaceList:             buildBookkeepingSurfaceList(),
		UserFlows:               buildBookkeepingUserFlows(),
		DataEntities:            []DataEntity{{EntityID: "entity-entry", Name: "账单记录", Source: "local_storage", Fields: []DataField{{Name: "entry_id", Type: "string", Required: true, Description: "账单唯一标识"}, {Name: "entry_type", Type: "enum[income,expense]", Required: true, Description: "收入或支出"}, {Name: "amount", Type: "decimal", Required: true, Description: "金额"}, {Name: "category", Type: "string", Required: true, Description: "分类"}, {Name: "occurred_on", Type: "date", Required: true, Description: "发生日期"}, {Name: "note", Type: "string", Required: false, Description: "备注"}}}, {EntityID: "entity-summary", Name: "月度汇总", Source: "derived", Fields: []DataField{{Name: "month", Type: "string", Required: true, Description: "月份标识"}, {Name: "income_total", Type: "decimal", Required: true, Description: "本月收入总额"}, {Name: "expense_total", Type: "decimal", Required: true, Description: "本月支出总额"}, {Name: "balance", Type: "decimal", Required: true, Description: "结余"}}}},
		TemplateConstraints:     TemplateConstraints{Stack: "flutter", AndroidRequired: true, RequiredCapabilities: []string{"list", "form", "local-storage", "bottom-navigation", "summary-card"}, ExcludedLicenses: []string{"GPL-3.0"}, PreferredTemplateIDs: preferredTemplateIDs},
		AcceptanceCriteria:      []AcceptanceCriterion{{CriterionID: "ac-home", Label: "概览展示本月收支", Category: "functional", Required: true, Description: "概览承载单元必须展示收入、支出、结余和最近账单入口。"}, {CriterionID: "ac-entry", Label: "记账动作链路完整", Category: "functional", Required: true, Description: "用户可以在记账动作承载单元新增一笔账单，并回看概览与账单集合。"}, {CriterionID: "ac-ledger", Label: "账单集合可浏览", Category: "functional", Required: true, Description: "账单集合浏览承载单元按时间倒序显示账单，并展示金额和分类。"}, {CriterionID: "ac-persistence", Label: "本地持久化可恢复", Category: "smoke", Required: true, Description: "应用重启后仍能读取本地账单数据，并恢复概览与账单集合。"}, {CriterionID: "ac-navigation", Label: "核心承载入口可导航", Category: "ui", Required: true, Description: "概览承载、记账动作和账单集合浏览之间的入口明确且可达。"}},
		ManualReviewPoints:      []ManualReviewPoint{{PointID: "mrp-copy", Summary: "检查概览承载与记账动作中文文案是否一致", Reason: "P0 阶段主要靠模板改造，概览与动作文案容易残留模板默认值。", Owner: "product"}, {PointID: "mrp-data", Summary: "确认本地持久化方案不引入额外后端依赖", Reason: "当前实验阶段明确不接入云端服务。", Owner: "engineering"}},
		KnownUnknowns:           []KnownUnknown{{Question: "是否需要在下一阶段加入按分类过滤或图表统计", Impact: "medium", Owner: "product"}, {Question: "后续设备验证与恢复闭环是否沿用当前 acceptance check 结构", Impact: "medium", Owner: "engineering"}},
		TaskBundle:              buildBookkeepingTaskBundle(),
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
		ContextFiles:            appruns.ContextFiles{PRDMarkdownPath: prdMarkdownFileName, PRDJSONPath: prdJSONFileName, TemplateFitReportPath: fitReportFileName, ImplementationPlanPath: planFileName, ManualConstraintsPath: constraintsFileName, SupportingFiles: []string{requirementFileName, prdApprovalFileName, templateApprovalFileName, planningContextFileName, domainModelFileName, templateSlotMapFileName, taskAllocationFileName, acceptancePlanFileName}},
	}
}

func compileRelationRichSpec(request Request, requirementText string) domainSpec {
	title := strings.TrimSpace(request.TitleHint)
	if title == "" {
		title = "项目任务协同 App"
	}
	now := time.Now().UTC()
	if request.Now != nil {
		now = request.Now()
	}
	jobID := fallbackGeneratedID(request.JobID, "job-project-task-tag-open-lite", request.RequirementSource, now)
	prdID := fallbackGeneratedID(request.PRDID, "prd-project-task-tag-open-lite", request.RequirementSource, now)
	selectedTemplateLabel := fallbackID(request.TemplateID, "flutter-open-lite")
	templateID := strings.TrimSpace(request.TemplateID)
	preferredTemplateIDs := []string{"flutter-open-lite"}
	if templateID != "" {
		preferredTemplateIDs = []string{templateID}
	}
	realBuild := request.RealBuild || strings.TrimSpace(request.ExecutorImage) != ""
	executorImage := strings.TrimSpace(request.ExecutorImage)
	if realBuild && executorImage == "" {
		executorImage = "picoclaw/appfactory-builder:local"
	}
	flutterProfile := appruns.NewFlutterAndroidProfile()
	acceptanceChecks := []appruns.AcceptanceCheck{
		{CheckID: "check-context-ready", Label: "准备上下文文件", Stage: "baseline", Required: true, Commands: []string{"echo context-ready"}, SuccessCriteria: "上下文装载命令返回 0。", TimeoutSeconds: 30},
		{CheckID: "check-relation-rich-boundary", Label: "确认 relation-rich 规划边界", Stage: "baseline", Required: true, Commands: []string{"echo relation-rich-project-task-tag"}, SuccessCriteria: "当前样例已冻结为项目、任务、标签与关联边界，不再退回单实体 generic 假设。", TimeoutSeconds: 30},
		{CheckID: "check-plan-ready", Label: "确认 Builder 输入包可执行", Stage: "cheap", Required: true, Commands: []string{"echo builder-input-ready"}, SuccessCriteria: "Builder 输入包与复杂领域计划文件已生成。", TimeoutSeconds: 30},
	}
	acceptanceChecks = append(acceptanceChecks, defaultFlutterFallbackChecks()...)
	commandProfile := appruns.CommandProfile{ProfileName: "prepare-p0", AllowedStages: []string{"baseline", "cheap"}, AllowedCommands: []string{"echo", "grep"}, DeniedCommands: []string{"rm", "sudo"}, MaxSingleCommandSeconds: 60, MaxParallelCommands: 1, NetworkPolicy: "disabled", WritableRoots: []string{"lib", "assets", "."}, EnvAllowlist: []string{"PATH", "HOME"}}
	if realBuild {
		acceptanceChecks = flutterProfile.AcceptanceChecks()
		commandProfile = flutterProfile.CommandProfile
	}
	requirementHighlights := extractRequirementHighlights(requirementText, []string{
		"需求包含项目、任务、标签三类主实体，以及任务与标签的多对多关系。",
		"概览承载单元需要展示按项目聚合的任务进度，集合浏览承载单元需要支持按项目和标签过滤。",
		"当前阶段目标是冻结 executable PRD 和 prepare baseline，不承诺直接进入 real build 主链。",
	})
	featureList := buildRelationRichFeatureList()
	surfaceList := buildRelationRichSurfaceList()
	userFlows := buildRelationRichUserFlows()
	return domainSpec{
		Kind:             "relation-rich",
		Slug:             "project-task-tag-open-lite",
		Title:            title,
		TemplateID:       templateID,
		ExecutorImage:    executorImage,
		RequirementText:  requirementText,
		PRDID:            prdID,
		JobID:            jobID,
		SourceSummary:    sourceSummary(request.RequirementSource),
		Summary:          "为小团队提供离线优先的项目、任务与标签协同看板，重点验证多实体与多对多关系在 planning-engine 中的冻结和投影能力。",
		ProblemStatement: "当前 planning-engine 已能稳定覆盖单实体 generic 与 bookkeeping 路径，但还缺少一条能真实表达项目、任务、标签和关联边界的 relation-rich baseline，用来验证 task_projection 与 task-allocation 是否足够承载复杂协同语义。",
		TargetUsers: []UserProfile{
			{UserID: "user-project-owner", Label: "项目负责人", Summary: "需要快速查看项目下任务进度、标签分布和待处理事项。", PainPoints: []string{"项目进度与任务细节割裂", "标签筛选容易和实际任务状态脱钩"}},
			{UserID: "user-validator", Label: "内部验证人员", Summary: "需要确认 planning-engine 是否已经能冻结多实体、多对多和共享归属面的执行语义。", PainPoints: []string{"当前样例大多还是单实体闭环", "复杂关系缺少稳定的 prepare baseline"}},
		},
		CoreScenarios: []Scenario{
			{ScenarioID: "scenario-project-board", Title: "查看项目任务看板", Summary: "用户能在概览承载单元看到项目维度的任务分布、完成度和重点标签。", PrimaryUserRefs: []string{"user-project-owner", "user-validator"}},
			{ScenarioID: "scenario-create-task", Title: "创建并打标签", Summary: "用户可以在实体变更承载单元创建任务、选择项目并绑定多个标签。", PrimaryUserRefs: []string{"user-project-owner"}},
			{ScenarioID: "scenario-filter-task", Title: "按项目和标签筛选任务", Summary: "用户能在集合浏览承载单元按项目、标签和状态交叉过滤任务。", PrimaryUserRefs: []string{"user-project-owner", "user-validator"}},
		},
		Goals: []string{
			"产出一份 relation-rich executable PRD，显式冻结项目、任务、标签、任务标签关联和项目摘要。",
			"生成一份可回归的 prepare baseline，用来校验 surface_refs / entity_refs / relation_group_refs / shared_ownership_refs 的真实投影。",
			"验证 planning-engine 可以在不预设固定页面骨架的前提下表达复杂关系边界。",
		},
		NonGoals: []string{
			"不做账号体系、评论、消息提醒、多人实时协作。",
			"不把当前样例直接推进到 real build 或设备验证主链。",
			"不引入额外远端 API、自建后端或复杂权限系统。",
		},
		FeatureList: featureList,
		SurfaceList: surfaceList,
		UserFlows:   userFlows,
		DataEntities: []DataEntity{
			{EntityID: "entity-project", Name: "项目", Source: "local_storage", Fields: []DataField{{Name: "project_id", Type: "string", Required: true, Description: "项目唯一标识"}, {Name: "title", Type: "string", Required: true, Description: "项目名称"}, {Name: "status", Type: "enum[active,paused,done]", Required: true, Description: "项目状态"}, {Name: "color", Type: "string", Required: false, Description: "项目标识颜色"}}},
			{EntityID: "entity-task", Name: "任务", Source: "local_storage", Fields: []DataField{{Name: "task_id", Type: "string", Required: true, Description: "任务唯一标识"}, {Name: "project_id", Type: "string", Required: true, Description: "所属项目标识"}, {Name: "title", Type: "string", Required: true, Description: "任务标题"}, {Name: "status", Type: "enum[todo,doing,done]", Required: true, Description: "任务状态"}, {Name: "due_on", Type: "date", Required: false, Description: "截止日期"}, {Name: "note", Type: "string", Required: false, Description: "备注"}}},
			{EntityID: "entity-tag", Name: "标签", Source: "local_storage", Fields: []DataField{{Name: "tag_id", Type: "string", Required: true, Description: "标签唯一标识"}, {Name: "name", Type: "string", Required: true, Description: "标签名称"}, {Name: "color", Type: "string", Required: false, Description: "标签颜色"}}},
			{EntityID: "entity-task-tag-link", Name: "任务标签关联", Source: "local_storage", Fields: []DataField{{Name: "link_id", Type: "string", Required: true, Description: "关联记录标识"}, {Name: "task_id", Type: "string", Required: true, Description: "任务标识"}, {Name: "tag_id", Type: "string", Required: true, Description: "标签标识"}}},
			{EntityID: "entity-project-summary", Name: "项目看板摘要", Source: "derived", Fields: []DataField{{Name: "project_id", Type: "string", Required: true, Description: "项目标识"}, {Name: "open_task_count", Type: "integer", Required: true, Description: "待处理任务数"}, {Name: "done_task_count", Type: "integer", Required: true, Description: "已完成任务数"}, {Name: "tagged_task_count", Type: "integer", Required: true, Description: "已绑定标签任务数"}}},
		},
		TemplateConstraints: TemplateConstraints{Stack: "flutter", AndroidRequired: true, RequiredCapabilities: []string{"navigation", "summary-card", "list", "form", "detail", "local-storage", "theme"}, PreferredTemplateIDs: preferredTemplateIDs},
		AcceptanceCriteria: []AcceptanceCriterion{
			{CriterionID: "ac-overview", Label: "项目看板摘要完整", Category: "functional", Required: true, Description: "概览承载单元必须展示项目级任务分布、完成度和重点标签摘要。"},
			{CriterionID: "ac-list", Label: "任务列表可浏览", Category: "functional", Required: true, Description: "集合浏览承载单元能展示项目、状态和标签摘要。"},
			{CriterionID: "ac-form", Label: "任务编辑链路完整", Category: "functional", Required: true, Description: "实体变更承载单元可以创建任务并维护项目归属与基本字段。"},
			{CriterionID: "ac-detail", Label: "任务详情可达", Category: "functional", Required: true, Description: "结果检查承载单元至少展示所属项目、标签集合和主要字段。"},
			{CriterionID: "ac-filter", Label: "项目与标签筛选可用", Category: "functional", Required: true, Description: "集合浏览承载单元支持按项目和标签组合筛选任务。"},
			{CriterionID: "ac-tag-binding", Label: "任务标签绑定一致", Category: "functional", Required: true, Description: "任务标签绑定或解绑后，集合浏览、结果检查和概览摘要保持一致。"},
			{CriterionID: "ac-persistence", Label: "本地持久化可恢复", Category: "smoke", Required: true, Description: "项目、任务、标签和任务标签关联在应用重启后仍能恢复。"},
			{CriterionID: "ac-navigation", Label: "核心承载入口可导航", Category: "ui", Required: true, Description: "概览、集合浏览、实体变更和结果检查承载单元之间入口明确且可达。"},
		},
		ManualReviewPoints: []ManualReviewPoint{
			{PointID: "mrp-domain-wording", Summary: "检查项目、任务和标签文案是否保持一致", Reason: "relation-rich 样例会跨概览、集合浏览、实体变更和结果检查承载单元重复投影领域词，容易残留 generic 文案。", Owner: "product"},
			{PointID: "mrp-shared-ownership", Summary: "确认共享 repository 与筛选状态归属没有被拆散", Reason: "复杂领域的主要风险不是页面骨架，而是共享归属面被多个任务各自写坏。", Owner: "engineering"},
		},
		KnownUnknowns: []KnownUnknown{
			{Question: "后续是否需要支持子任务、成员分配或评论流", Impact: "medium", Owner: "product"},
			{Question: "open-lite 之外是否需要专门的 relation-rich 模板来承接多实体 controller / repository 拆分", Impact: "medium", Owner: "engineering"},
		},
		TaskBundle:       buildRelationRichTaskBundle(),
		AcceptanceChecks: acceptanceChecks,
		GoalSummary:      fmt.Sprintf("将项目、任务、标签的 relation-rich 需求整理成基于 %s 的复杂领域 planning baseline，冻结多实体、多对多关系和共享归属面，验证 prepare compiler 能产出稳定的 execution_contract 与中间产物。", selectedTemplateLabel),
		TemplateFitReasons: []string{
			"flutter-open-lite 仍是当前唯一能稳定承接概览、集合浏览、实体变更和结果检查承载映射的多入口 seed。",
			"当前阶段目标是验证 planning-engine 对复杂关系的表达能力，而不是直接进入真实 Flutter 交付。",
			"relation-rich 样例可以先复用 open-lite 的承载映射入口，再把复杂性收口到 domain model、task projection 和 repository 边界。",
		},
		TemplateFitGaps: []string{
			"当前模板并没有原生的项目/任务/标签多实体 controller 和 repository 分层，真实交付时仍需要更具体的模板或进一步模板扩展。",
			"当前样例只用于 prepare baseline，不建议直接进入 real build 主链。",
			"任务与标签的多对多关系、项目聚合查询和共享筛选状态是当前最主要的结构风险。",
		},
		ImplementationPhases: []string{
			"阶段 1：冻结项目、任务、标签、任务标签关联和项目摘要五类实体，并明确 relation group。",
			"阶段 2：冻结 repository、看板查询、筛选状态和标签选择这几类 shared ownership。",
			"阶段 3：把概览、集合浏览、实体变更、结果检查承载单元与测试入口投影到 prepare baseline。",
		},
		ManualConstraints: []string{
			"当前样例只验证 planning-engine 和 prepare compiler，不直接承诺真实 APK 构建。",
			"不得把项目、任务、标签压扁回单一 record 字段集合来规避复杂关系。",
			"当前模板仍然是参考骨架，复杂 repository 和 controller 拆分以后续模板演进为准。",
			"日志和构建报告保留英文输出，文档与计划说明使用中文。",
		},
		HumanNotes: []map[string]string{
			{"note_id": "note-relation-rich-baseline", "summary": "该样例用于冻结 relation-rich task projection，不直接作为 real build 交付目标。", "scope": "engineering"},
			{"note_id": "note-template-gap", "summary": "open-lite 当前只是复杂领域的 planning seed，不代表模板已经适配多实体协同。", "scope": "product"},
		},
		RequirementHighlights:   requirementHighlights,
		SupportingAssumptions:   []string{"默认离线单机运行。", "项目、任务和标签数据都由本地仓储统一持有。", "当前复杂领域样例只验证 planning 层冻结，不验证实时协作或远端同步。"},
		PreferredAllowedPaths:   preferredAllowedPaths(realBuild),
		PreferredProtectedPaths: preferredProtectedPaths(realBuild),
		KnowledgePack:           append([]appruns.ProfileSkill(nil), flutterProfile.KnowledgePack...),
		CommandProfile:          commandProfile,
		ContextFiles:            appruns.ContextFiles{PRDMarkdownPath: prdMarkdownFileName, PRDJSONPath: prdJSONFileName, TemplateFitReportPath: fitReportFileName, ImplementationPlanPath: planFileName, ManualConstraintsPath: constraintsFileName, SupportingFiles: []string{requirementFileName, prdApprovalFileName, templateApprovalFileName, planningContextFileName, domainModelFileName, templateSlotMapFileName, taskAllocationFileName, acceptancePlanFileName}},
	}
}

func compileInventorySheetLineItemSpec(request Request, requirementText string) domainSpec {
	title := strings.TrimSpace(request.TitleHint)
	if title == "" {
		title = "库存单协同 App"
	}
	now := time.Now().UTC()
	if request.Now != nil {
		now = request.Now()
	}
	jobID := fallbackGeneratedID(request.JobID, "job-inventory-sheet-line-item-open-lite", request.RequirementSource, now)
	prdID := fallbackGeneratedID(request.PRDID, "prd-inventory-sheet-line-item-open-lite", request.RequirementSource, now)
	selectedTemplateLabel := fallbackID(request.TemplateID, "flutter-open-lite")
	templateID := strings.TrimSpace(request.TemplateID)
	preferredTemplateIDs := []string{"flutter-open-lite"}
	if templateID != "" {
		preferredTemplateIDs = []string{templateID}
	}
	realBuild := request.RealBuild || strings.TrimSpace(request.ExecutorImage) != ""
	executorImage := strings.TrimSpace(request.ExecutorImage)
	if realBuild && executorImage == "" {
		executorImage = "picoclaw/appfactory-builder:local"
	}
	flutterProfile := appruns.NewFlutterAndroidProfile()
	acceptanceChecks := []appruns.AcceptanceCheck{
		{CheckID: "check-context-ready", Label: "准备上下文文件", Stage: "baseline", Required: true, Commands: []string{"echo context-ready"}, SuccessCriteria: "上下文装载命令返回 0。", TimeoutSeconds: 30},
		{CheckID: "check-relation-rich-boundary", Label: "确认 inventory line-item 规划边界", Stage: "baseline", Required: true, Commands: []string{"echo relation-rich-inventory-sheet-line-item"}, SuccessCriteria: "当前样例已冻结为库存单、明细项、SKU 与仓库边界，不再退回单实体 generic 假设。", TimeoutSeconds: 30},
		{CheckID: "check-plan-ready", Label: "确认 Builder 输入包可执行", Stage: "cheap", Required: true, Commands: []string{"echo builder-input-ready"}, SuccessCriteria: "Builder 输入包与复杂领域计划文件已生成。", TimeoutSeconds: 30},
	}
	acceptanceChecks = append(acceptanceChecks, defaultFlutterFallbackChecks()...)
	commandProfile := appruns.CommandProfile{ProfileName: "prepare-p0", AllowedStages: []string{"baseline", "cheap"}, AllowedCommands: []string{"echo", "grep"}, DeniedCommands: []string{"rm", "sudo"}, MaxSingleCommandSeconds: 60, MaxParallelCommands: 1, NetworkPolicy: "disabled", WritableRoots: []string{"lib", "assets", "."}, EnvAllowlist: []string{"PATH", "HOME"}}
	if realBuild {
		acceptanceChecks = flutterProfile.AcceptanceChecks()
		commandProfile = flutterProfile.CommandProfile
	}
	requirementHighlights := extractRequirementHighlights(requirementText, []string{
		"需求包含库存单、明细项、SKU 和仓库四类主实体，库存单和明细项是一对多关系。",
		"概览承载单元需要展示低库存预警和盘点单概览，集合浏览承载单元需要支持按仓库和低库存状态过滤。",
		"当前阶段目标是冻结 executable PRD 和 prepare baseline，不承诺直接进入 real build 主链。",
	})
	featureList := buildInventoryFeatureList()
	surfaceList := buildInventorySurfaceList()
	userFlows := buildInventoryUserFlows()
	return domainSpec{
		Kind:             "relation-rich",
		Slug:             "inventory-sheet-line-item-open-lite",
		Title:            title,
		TemplateID:       templateID,
		ExecutorImage:    executorImage,
		RequirementText:  requirementText,
		PRDID:            prdID,
		JobID:            jobID,
		SourceSummary:    sourceSummary(request.RequirementSource),
		Summary:          "为仓储或门店负责人提供离线优先的库存单、明细项与 SKU 协同看板，重点验证一对多关系、低库存摘要与共享归属面在 planning-engine 中的冻结和投影能力。",
		ProblemStatement: "当前 planning-engine 已验证项目/任务/标签复杂域，但还缺少一条库存单、明细项、SKU 与仓库协同的 relation-rich baseline，用来验证 relation_group_refs 与 shared_ownership_refs 是否具备跨领域稳定性。",
		TargetUsers: []UserProfile{
			{UserID: "user-warehouse-owner", Label: "仓储负责人", Summary: "需要快速查看低库存预警、盘点单状态和明细差异。", PainPoints: []string{"低库存预警和盘点明细经常割裂", "不同仓库的筛选结果容易失真"}},
			{UserID: "user-validator", Label: "内部验证人员", Summary: "需要确认 planning-engine 是否已经能在第二个复杂域中稳定冻结关系边界和共享归属面。", PainPoints: []string{"当前 relation-rich 只有项目/任务/标签一条样例", "复杂领域约束还没证明具有跨领域稳定性"}},
		},
		CoreScenarios: []Scenario{
			{ScenarioID: "scenario-inventory-board", Title: "查看库存看板", Summary: "用户能在概览承载单元看到低库存预警、待处理库存单和仓库维度摘要。", PrimaryUserRefs: []string{"user-warehouse-owner", "user-validator"}},
			{ScenarioID: "scenario-create-sheet", Title: "创建库存单并维护明细项", Summary: "用户可以在实体变更承载单元选择仓库、录入多个明细项并保存。", PrimaryUserRefs: []string{"user-warehouse-owner"}},
			{ScenarioID: "scenario-filter-items", Title: "按仓库和低库存状态筛选", Summary: "用户能在集合浏览承载单元按仓库、低库存状态和盘点状态组合筛选库存单与明细项。", PrimaryUserRefs: []string{"user-warehouse-owner", "user-validator"}},
		},
		Goals: []string{
			"产出第二份 relation-rich executable PRD，显式冻结库存单、明细项、SKU、仓库和低库存摘要。",
			"生成第二份可回归的 prepare baseline，用来校验 surface_refs / entity_refs / relation_group_refs / shared_ownership_refs 的跨领域稳定性。",
			"验证 planning-engine 可以在不预设固定页面骨架的前提下表达库存单与明细项的一对多关系和共享查询边界。",
		},
		NonGoals: []string{
			"不做扫码枪、打印机、外部 ERP、审批流或多人协作。",
			"不把当前样例直接推进到 real build 或设备验证主链。",
			"不引入远端 API、自建后端或复杂权限系统。",
		},
		FeatureList: featureList,
		SurfaceList: surfaceList,
		UserFlows:   userFlows,
		DataEntities: []DataEntity{
			{EntityID: "entity-inventory-sheet", Name: "库存单", Source: "local_storage", Fields: []DataField{{Name: "sheet_id", Type: "string", Required: true, Description: "库存单唯一标识"}, {Name: "warehouse_id", Type: "string", Required: true, Description: "所属仓库标识"}, {Name: "status", Type: "enum[draft,checking,closed]", Required: true, Description: "库存单状态"}, {Name: "counted_on", Type: "date", Required: true, Description: "盘点日期"}, {Name: "note", Type: "string", Required: false, Description: "盘点备注"}}},
			{EntityID: "entity-line-item", Name: "明细项", Source: "local_storage", Fields: []DataField{{Name: "line_item_id", Type: "string", Required: true, Description: "明细项唯一标识"}, {Name: "sheet_id", Type: "string", Required: true, Description: "所属库存单标识"}, {Name: "sku_id", Type: "string", Required: true, Description: "SKU 标识"}, {Name: "expected_qty", Type: "integer", Required: true, Description: "系统库存数"}, {Name: "counted_qty", Type: "integer", Required: true, Description: "实盘数量"}, {Name: "variance_qty", Type: "integer", Required: true, Description: "差异数量"}}},
			{EntityID: "entity-sku", Name: "SKU", Source: "local_storage", Fields: []DataField{{Name: "sku_id", Type: "string", Required: true, Description: "SKU 唯一标识"}, {Name: "name", Type: "string", Required: true, Description: "SKU 名称"}, {Name: "category", Type: "string", Required: true, Description: "SKU 分类"}, {Name: "reorder_threshold", Type: "integer", Required: true, Description: "补货阈值"}}},
			{EntityID: "entity-warehouse", Name: "仓库", Source: "local_storage", Fields: []DataField{{Name: "warehouse_id", Type: "string", Required: true, Description: "仓库唯一标识"}, {Name: "name", Type: "string", Required: true, Description: "仓库名称"}, {Name: "location", Type: "string", Required: false, Description: "仓库位置"}}},
			{EntityID: "entity-low-stock-summary", Name: "低库存摘要", Source: "derived", Fields: []DataField{{Name: "warehouse_id", Type: "string", Required: true, Description: "仓库标识"}, {Name: "open_sheet_count", Type: "integer", Required: true, Description: "待处理库存单数量"}, {Name: "low_stock_sku_count", Type: "integer", Required: true, Description: "低库存 SKU 数量"}, {Name: "variance_line_item_count", Type: "integer", Required: true, Description: "存在差异的明细项数量"}}},
		},
		TemplateConstraints: TemplateConstraints{Stack: "flutter", AndroidRequired: true, RequiredCapabilities: []string{"navigation", "summary-card", "list", "form", "detail", "local-storage", "theme"}, PreferredTemplateIDs: preferredTemplateIDs},
		AcceptanceCriteria: []AcceptanceCriterion{
			{CriterionID: "ac-overview", Label: "库存看板摘要完整", Category: "functional", Required: true, Description: "概览承载单元必须展示低库存预警、待处理库存单和仓库维度摘要。"},
			{CriterionID: "ac-list", Label: "库存列表可浏览", Category: "functional", Required: true, Description: "集合浏览承载单元能展示仓库、盘点状态和明细项摘要。"},
			{CriterionID: "ac-form", Label: "库存单编辑链路完整", Category: "functional", Required: true, Description: "实体变更承载单元可以创建库存单并维护所属仓库与基础字段。"},
			{CriterionID: "ac-detail", Label: "库存单详情可达", Category: "functional", Required: true, Description: "结果检查承载单元至少展示所属仓库、明细项集合和主要差异字段。"},
			{CriterionID: "ac-filter", Label: "仓库与低库存筛选可用", Category: "functional", Required: true, Description: "集合浏览承载单元支持按仓库和低库存状态组合筛选库存单与明细项。"},
			{CriterionID: "ac-line-item-binding", Label: "明细项维护一致", Category: "functional", Required: true, Description: "库存单明细项维护后，集合浏览、结果检查和概览摘要保持一致。"},
			{CriterionID: "ac-persistence", Label: "本地持久化可恢复", Category: "smoke", Required: true, Description: "库存单、明细项、SKU 和仓库数据在应用重启后仍能恢复。"},
			{CriterionID: "ac-navigation", Label: "核心承载入口可导航", Category: "ui", Required: true, Description: "概览、集合浏览、实体变更和结果检查承载单元之间入口明确且可达。"},
		},
		ManualReviewPoints: []ManualReviewPoint{
			{PointID: "mrp-domain-wording", Summary: "检查库存单、明细项和仓库文案是否保持一致", Reason: "第二条 relation-rich 样例会跨概览、集合浏览、实体变更和结果检查承载单元重复投影领域词，容易残留 generic 文案。", Owner: "product"},
			{PointID: "mrp-shared-ownership", Summary: "确认共享 repository、低库存查询和明细编辑归属没有被拆散", Reason: "复杂领域的主要风险不是页面骨架，而是共享归属面被多个任务各自写坏。", Owner: "engineering"},
		},
		KnownUnknowns: []KnownUnknown{
			{Question: "后续是否需要支持扫码录入、批量导入或供应商维度联动", Impact: "medium", Owner: "product"},
			{Question: "open-lite 之外是否需要专门的 relation-rich 模板来承接一对多明细编辑与库存摘要查询", Impact: "medium", Owner: "engineering"},
		},
		TaskBundle:       buildInventorySheetLineItemTaskBundle(),
		AcceptanceChecks: acceptanceChecks,
		GoalSummary:      fmt.Sprintf("将库存单、明细项和 SKU 的 relation-rich 需求整理成基于 %s 的第二条复杂领域 planning baseline，冻结一对多关系、低库存摘要和共享归属面，验证 prepare compiler 的 refs 投影具备跨领域稳定性。", selectedTemplateLabel),
		TemplateFitReasons: []string{
			"flutter-open-lite 仍是当前唯一能稳定承接概览、集合浏览、实体变更和结果检查承载映射的多入口 seed。",
			"当前阶段目标是验证第二个复杂领域的 planning 表达能力，而不是直接进入真实 Flutter 交付。",
			"库存单与明细项样例可以先复用 open-lite 的承载映射入口，再把复杂性收口到 domain model、task projection 和 repository 边界。",
		},
		TemplateFitGaps: []string{
			"当前模板并没有原生的一对多明细编辑器、低库存看板查询和仓库筛选状态管理，真实交付时仍需要更具体的模板或进一步模板扩展。",
			"当前样例只用于 prepare baseline，不建议直接进入 real build 主链。",
			"库存单与明细项的一对多关系、低库存聚合查询和共享筛选状态是当前最主要的结构风险。",
		},
		ImplementationPhases: []string{
			"阶段 1：冻结库存单、明细项、SKU、仓库和低库存摘要五类实体，并明确 relation group。",
			"阶段 2：冻结 shared repository、低库存看板查询、筛选状态和明细编辑这几类 shared ownership。",
			"阶段 3：把概览、集合浏览、实体变更、结果检查承载单元与测试入口投影到第二条 prepare baseline。",
		},
		ManualConstraints: []string{
			"当前样例只验证 planning-engine 和 prepare compiler，不直接承诺真实 APK 构建。",
			"不得把库存单和明细项压扁回单一 record 字段集合来规避一对多关系。",
			"当前模板仍然是参考骨架，复杂明细编辑和仓储查询以模板后续演进为准。",
			"日志和构建报告保留英文输出，文档与计划说明使用中文。",
		},
		HumanNotes: []map[string]string{
			{"note_id": "note-second-relation-rich-baseline", "summary": "该样例用于验证 relation_group_refs 与 shared_ownership_refs 是否具备跨领域稳定性，不直接作为 real build 交付目标。", "scope": "engineering"},
			{"note_id": "note-template-gap", "summary": "open-lite 当前只是库存单复杂域的 planning seed，不代表模板已经适配明细项编辑。", "scope": "product"},
		},
		RequirementHighlights:   requirementHighlights,
		SupportingAssumptions:   []string{"默认离线单机运行。", "库存单、明细项、SKU 和仓库数据都由本地仓储统一持有。", "当前复杂领域样例只验证 planning 层冻结，不验证扫码枪、打印机或远端 ERP 同步。"},
		PreferredAllowedPaths:   preferredAllowedPaths(realBuild),
		PreferredProtectedPaths: preferredProtectedPaths(realBuild),
		KnowledgePack:           append([]appruns.ProfileSkill(nil), flutterProfile.KnowledgePack...),
		CommandProfile:          commandProfile,
		ContextFiles:            appruns.ContextFiles{PRDMarkdownPath: prdMarkdownFileName, PRDJSONPath: prdJSONFileName, TemplateFitReportPath: fitReportFileName, ImplementationPlanPath: planFileName, ManualConstraintsPath: constraintsFileName, SupportingFiles: []string{requirementFileName, prdApprovalFileName, templateApprovalFileName, planningContextFileName, domainModelFileName, templateSlotMapFileName, taskAllocationFileName, acceptancePlanFileName}},
	}
}

func relationRichTransition(allocationID string, semanticIntentRefs, surfaceRefs, entityRefs, relationGroupRefs, sharedOwnershipRefs, ownedPaths, blockedBy, successEvidence []string) *appruns.TaskAllocationTransition {
	return &appruns.TaskAllocationTransition{
		AllocationID:        allocationID,
		SemanticIntentRefs:  semanticIntentRefs,
		SurfaceRefs:         surfaceRefs,
		EntityRefs:          entityRefs,
		RelationGroupRefs:   relationGroupRefs,
		SharedOwnershipRefs: sharedOwnershipRefs,
		OwnedPaths:          ownedPaths,
		BlockedBy:           blockedBy,
		SuccessEvidence:     successEvidence,
	}
}

func genericSurfaceTransitionWithBindings(allocationID string, semanticIntentRefs, bindingRefs, surfaceRefs, entityRefs, ownedPaths, blockedBy, successEvidence []string) *appruns.TaskAllocationTransition {
	return &appruns.TaskAllocationTransition{
		AllocationID:       allocationID,
		SemanticIntentRefs: semanticIntentRefs,
		BindingRefs:        bindingRefs,
		SurfaceRefs:        surfaceRefs,
		EntityRefs:         entityRefs,
		OwnedPaths:         ownedPaths,
		BlockedBy:          blockedBy,
		SuccessEvidence:    successEvidence,
	}
}

type genericSurfaceRefSet struct {
	AllSurfaces                  []string
	OverviewSurfaces             []string
	CollectionSurfaces           []string
	CollectionInspectionSurfaces []string
	MutationSurfaces             []string
	InspectionSurfaces           []string
	PrimaryEntityRefs            []string
	SummaryEntityRefs            []string
}

func newGenericSurfaceRefSet(topology genericTopologyPlan, primaryEntityID, summaryEntityID string) genericSurfaceRefSet {
	summaryRefs := compactRefs(primaryEntityID, summaryEntityID)
	if !topology.Overview {
		summaryRefs = compactRefs(primaryEntityID)
	}
	return genericSurfaceRefSet{
		AllSurfaces: compactRefs(
			genericConditionalRef(topology.Overview, genericSurfaceOverviewID),
			genericConditionalRef(topology.Collection, genericSurfaceCollectionID),
			genericConditionalRef(topology.Mutation, genericSurfaceMutationID),
			genericConditionalRef(topology.Inspection, genericSurfaceInspectionID),
		),
		OverviewSurfaces:             compactRefs(genericConditionalRef(topology.Overview, genericSurfaceOverviewID)),
		CollectionSurfaces:           compactRefs(genericConditionalRef(topology.Collection, genericSurfaceCollectionID)),
		CollectionInspectionSurfaces: compactRefs(genericConditionalRef(topology.Collection, genericSurfaceCollectionID), genericConditionalRef(topology.Inspection, genericSurfaceInspectionID)),
		MutationSurfaces:             compactRefs(genericConditionalRef(topology.Mutation, genericSurfaceMutationID)),
		InspectionSurfaces:           compactRefs(genericConditionalRef(topology.Inspection, genericSurfaceInspectionID)),
		PrimaryEntityRefs:            compactRefs(primaryEntityID),
		SummaryEntityRefs:            summaryRefs,
	}
}

func genericConditionalRef(enabled bool, ref string) string {
	if !enabled {
		return ""
	}
	return ref
}

func compactRefs(values ...string) []string {
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
	if len(result) == 0 {
		return nil
	}
	return result
}

func genericSurfaceTransition(allocationID string, semanticIntentRefs, surfaceRefs, entityRefs, ownedPaths, blockedBy, successEvidence []string) *appruns.TaskAllocationTransition {
	return genericSurfaceTransitionWithBindings(allocationID, semanticIntentRefs, nil, surfaceRefs, entityRefs, ownedPaths, blockedBy, successEvidence)
}

func buildGenericSurfaceList(featureList []Feature) []InteractionSurface {
	surfaces := make([]InteractionSurface, 0, 4)
	if featureListIncludesSurfaceRef(featureList, genericSurfaceOverviewID) {
		overviewFeatures := compactRefs("feature-home-overview")
		if len(overviewFeatures) == 0 || !featureListHasID(featureList, "feature-home-overview") {
			overviewFeatures = featureListPrimaryRefsForSurface(featureList, genericSurfaceOverviewID)
		}
		surfaces = append(surfaces, InteractionSurface{
			SurfaceID:          genericSurfaceOverviewID,
			Label:              "概览承载单元",
			Purpose:            "承载领域摘要观察、导航入口和最近结果回看。",
			PrimaryFeatureRefs: overviewFeatures,
		})
	}
	if featureListIncludesSurfaceRef(featureList, genericSurfaceCollectionID) {
		collectionFeatures := compactRefs("feature-record-list")
		if len(collectionFeatures) == 0 || !featureListHasID(featureList, "feature-record-list") {
			collectionFeatures = featureListPrimaryRefsForSurface(featureList, genericSurfaceCollectionID)
		}
		surfaces = append(surfaces, InteractionSurface{
			SurfaceID:          genericSurfaceCollectionID,
			Label:              "集合浏览承载单元",
			Purpose:            "承载领域记录集合浏览、筛选和结果定位。",
			PrimaryFeatureRefs: collectionFeatures,
		})
	}
	if featureListIncludesSurfaceRef(featureList, genericSurfaceMutationID) {
		mutationFeatures := compactRefs("feature-record-form")
		if len(mutationFeatures) == 0 || !featureListHasID(featureList, "feature-record-form") {
			mutationFeatures = featureListPrimaryRefsForSurface(featureList, genericSurfaceMutationID)
		}
		surfaces = append(surfaces, InteractionSurface{
			SurfaceID:          genericSurfaceMutationID,
			Label:              "实体变更承载单元",
			Purpose:            "承载实体创建、编辑、校验和保存动作。",
			PrimaryFeatureRefs: mutationFeatures,
		})
	}
	if featureListIncludesSurfaceRef(featureList, genericSurfaceInspectionID) {
		inspectionFeatures := compactRefs(
			genericConditionalRef(featureListHasID(featureList, "feature-record-list"), "feature-record-list"),
			genericConditionalRef(featureListHasID(featureList, "feature-record-delete"), "feature-record-delete"),
		)
		if len(inspectionFeatures) == 0 {
			inspectionFeatures = featureListPrimaryRefsForSurface(featureList, genericSurfaceInspectionID)
		}
		surfaces = append(surfaces, InteractionSurface{
			SurfaceID:          genericSurfaceInspectionID,
			Label:              "结果检查承载单元",
			Purpose:            "承载单条记录检查、删除确认和结果回流。",
			PrimaryFeatureRefs: inspectionFeatures,
		})
	}
	return surfaces
}

func featureListIncludesSurfaceRef(featureList []Feature, surfaceRef string) bool {
	for _, feature := range featureList {
		for _, ref := range feature.RelatedSurfaceRefs {
			if strings.TrimSpace(ref) == surfaceRef {
				return true
			}
		}
	}
	return false
}

func featureListPrimaryRefsForSurface(featureList []Feature, surfaceRef string) []string {
	refs := make([]string, 0, len(featureList))
	for _, feature := range featureList {
		for _, ref := range feature.RelatedSurfaceRefs {
			if strings.TrimSpace(ref) != surfaceRef {
				continue
			}
			refs = append(refs, strings.TrimSpace(feature.FeatureID))
			break
		}
	}
	return compactRefs(refs...)
}

func buildRelationRichSurfaceList() []InteractionSurface {
	return []InteractionSurface{
		{
			SurfaceID:          genericSurfaceOverviewID,
			Label:              "概览承载单元",
			Purpose:            "承载项目级摘要观察、重点标签回看和主导航入口。",
			PrimaryFeatureRefs: []string{"feature-project-board"},
		},
		{
			SurfaceID:          genericSurfaceCollectionID,
			Label:              "集合浏览承载单元",
			Purpose:            "承载任务集合浏览、项目与标签筛选以及结果定位。",
			PrimaryFeatureRefs: []string{"feature-task-list"},
		},
		{
			SurfaceID:          genericSurfaceMutationID,
			Label:              "实体变更承载单元",
			Purpose:            "承载任务创建、项目归属变更、标签绑定和保存动作。",
			PrimaryFeatureRefs: []string{"feature-task-editor", "feature-tag-binding"},
		},
		{
			SurfaceID:          genericSurfaceInspectionID,
			Label:              "结果检查承载单元",
			Purpose:            "承载任务结果检查、标签回看和详情回流。",
			PrimaryFeatureRefs: []string{"feature-task-list", "feature-tag-binding"},
		},
	}
}

func buildInventorySurfaceList() []InteractionSurface {
	return []InteractionSurface{
		{
			SurfaceID:          genericSurfaceOverviewID,
			Label:              "概览承载单元",
			Purpose:            "承载低库存摘要观察、待处理库存单回看和主导航入口。",
			PrimaryFeatureRefs: []string{"feature-inventory-board"},
		},
		{
			SurfaceID:          genericSurfaceCollectionID,
			Label:              "集合浏览承载单元",
			Purpose:            "承载库存单与明细项集合浏览、筛选和结果定位。",
			PrimaryFeatureRefs: []string{"feature-sheet-list"},
		},
		{
			SurfaceID:          genericSurfaceMutationID,
			Label:              "实体变更承载单元",
			Purpose:            "承载库存单创建、仓库归属变更、明细项维护和保存动作。",
			PrimaryFeatureRefs: []string{"feature-sheet-editor", "feature-line-item-binding"},
		},
		{
			SurfaceID:          genericSurfaceInspectionID,
			Label:              "结果检查承载单元",
			Purpose:            "承载库存单结果检查、差异回看和详情回流。",
			PrimaryFeatureRefs: []string{"feature-sheet-list", "feature-line-item-binding"},
		},
	}
}

func buildBookkeepingFeatureList() []Feature {
	return []Feature{
		{FeatureID: "feature-home-summary", Title: "收支概览", Summary: "在概览承载单元展示本月收入、支出、结余和最近三笔账单。", Priority: "p0", Required: true, RelatedScenarios: []string{"scenario-summary"}, RelatedSurfaceRefs: []string{genericSurfaceOverviewID}, AcceptanceRefs: []string{"ac-home", "ac-navigation"}},
		{FeatureID: "feature-add-entry", Title: "记账动作", Summary: "支持录入金额、类型、分类、日期和备注，并提交新增账单。", Priority: "p0", Required: true, RelatedScenarios: []string{"scenario-quick-add"}, RelatedSurfaceRefs: []string{genericSurfaceMutationID}, AcceptanceRefs: []string{"ac-entry"}},
		{FeatureID: "feature-ledger", Title: "账单集合浏览", Summary: "按时间倒序展示账单集合，并支持核对金额和分类。", Priority: "p0", Required: true, RelatedScenarios: []string{"scenario-ledger"}, RelatedSurfaceRefs: []string{genericSurfaceCollectionID}, AcceptanceRefs: []string{"ac-ledger"}},
		{FeatureID: "feature-local-data", Title: "本地数据持久化", Summary: "账单数据本地持久化，应用重启后仍能恢复概览与账单集合。", Priority: "p1", Required: true, RelatedScenarios: []string{"scenario-quick-add", "scenario-ledger", "scenario-summary"}, RelatedSurfaceRefs: []string{genericSurfaceOverviewID, genericSurfaceCollectionID, genericSurfaceMutationID}, AcceptanceRefs: []string{"ac-persistence"}},
	}
}

func buildBookkeepingSurfaceList() []InteractionSurface {
	return []InteractionSurface{
		{
			SurfaceID:          genericSurfaceOverviewID,
			Label:              "收支概览承载单元",
			Purpose:            "承载本月收支摘要、最近账单回看和主导航入口。",
			PrimaryFeatureRefs: []string{"feature-home-summary", "feature-ledger"},
		},
		{
			SurfaceID:          genericSurfaceMutationID,
			Label:              "记账动作承载单元",
			Purpose:            "承载金额、分类、日期和备注录入，以及保存动作。",
			PrimaryFeatureRefs: []string{"feature-add-entry"},
		},
		{
			SurfaceID:          genericSurfaceCollectionID,
			Label:              "账单集合浏览承载单元",
			Purpose:            "承载账单集合浏览、结果核对和回到概览的入口。",
			PrimaryFeatureRefs: []string{"feature-ledger", "feature-local-data"},
		},
	}
}

func buildBookkeepingUserFlows() []UserFlow {
	return []UserFlow{
		{
			FlowID: "flow-add-expense",
			Title:  "新增支出",
			Steps: []FlowStep{
				{StepID: "step-open-entry-action", Title: "从概览承载单元进入记账动作入口", SurfaceRef: genericSurfaceOverviewID, Actor: "个人记账用户", ExpectedResult: "能看到金额和分类输入控件"},
				{StepID: "step-fill-entry", Title: "填写金额、分类、日期和备注", SurfaceRef: genericSurfaceMutationID, Actor: "个人记账用户", ExpectedResult: "录入字段校验通过"},
				{StepID: "step-save-entry", Title: "保存账单并回看概览与账单集合", SurfaceRef: genericSurfaceMutationID, Actor: "个人记账用户", ExpectedResult: "概览承载单元和账单集合浏览承载单元都能看到新增账单"},
			},
		},
		{
			FlowID: "flow-review-ledger",
			Title:  "查看账单集合",
			Steps: []FlowStep{
				{StepID: "step-open-ledger", Title: "从概览承载单元进入账单集合浏览", SurfaceRef: genericSurfaceOverviewID, Actor: "个人记账用户", ExpectedResult: "能看到最近账单集合"},
				{StepID: "step-confirm-entry", Title: "核对账单金额和分类", SurfaceRef: genericSurfaceCollectionID, Actor: "个人记账用户", ExpectedResult: "关键信息易于识别"},
			},
		},
	}
}

func buildRelationRichFeatureList() []Feature {
	return []Feature{
		{FeatureID: "feature-project-board", Title: "项目任务看板", Summary: "概览承载单元展示按项目聚合的任务分布、完成度和重点标签。", Priority: "p0", Required: true, RelatedScenarios: []string{"scenario-project-board"}, RelatedSurfaceRefs: []string{genericSurfaceOverviewID}, AcceptanceRefs: []string{"ac-overview", "ac-navigation"}},
		{FeatureID: "feature-task-list", Title: "任务列表与筛选", Summary: "集合浏览与结果检查承载单元支持按项目、标签和状态交叉筛选任务。", Priority: "p0", Required: true, RelatedScenarios: []string{"scenario-filter-task"}, RelatedSurfaceRefs: []string{genericSurfaceCollectionID, genericSurfaceInspectionID}, AcceptanceRefs: []string{"ac-list", "ac-detail", "ac-filter"}},
		{FeatureID: "feature-task-editor", Title: "任务编辑与项目归属", Summary: "实体变更承载单元支持创建任务、选择项目并维护任务状态与备注。", Priority: "p0", Required: true, RelatedScenarios: []string{"scenario-create-task"}, RelatedSurfaceRefs: []string{genericSurfaceMutationID}, AcceptanceRefs: []string{"ac-form"}},
		{FeatureID: "feature-tag-binding", Title: "任务标签绑定", Summary: "任务支持绑定多个标签，并在集合浏览与结果检查承载单元保持一致显示。", Priority: "p0", Required: true, RelatedScenarios: []string{"scenario-create-task", "scenario-filter-task"}, RelatedSurfaceRefs: []string{genericSurfaceMutationID, genericSurfaceCollectionID, genericSurfaceInspectionID}, AcceptanceRefs: []string{"ac-tag-binding"}},
		{FeatureID: "feature-local-storage", Title: "本地持久化", Summary: "项目、任务、标签和任务标签关联数据在应用重启后仍可恢复。", Priority: "p1", Required: true, RelatedScenarios: []string{"scenario-project-board", "scenario-create-task", "scenario-filter-task"}, RelatedSurfaceRefs: []string{genericSurfaceOverviewID, genericSurfaceCollectionID, genericSurfaceMutationID, genericSurfaceInspectionID}, AcceptanceRefs: []string{"ac-persistence"}},
	}
}

func buildRelationRichUserFlows() []UserFlow {
	return []UserFlow{
		{FlowID: "flow-create-task-with-tags", Title: "创建带标签的任务", Steps: []FlowStep{{StepID: "step-open-editor", Title: "从概览承载单元进入实体变更入口", SurfaceRef: genericSurfaceOverviewID, Actor: "项目负责人", ExpectedResult: "可选择项目并输入任务字段"}, {StepID: "step-bind-tags", Title: "选择多个标签并保存", SurfaceRef: genericSurfaceMutationID, Actor: "项目负责人", ExpectedResult: "任务与标签关联写入成功"}, {StepID: "step-review-detail", Title: "进入结果检查承载单元确认项目与标签信息", SurfaceRef: genericSurfaceInspectionID, Actor: "项目负责人", ExpectedResult: "结果检查承载单元展示所属项目和标签集合"}}},
		{FlowID: "flow-filter-by-project-tag", Title: "按项目和标签筛选任务", Steps: []FlowStep{{StepID: "step-open-list", Title: "从概览承载单元进入集合浏览承载单元", SurfaceRef: genericSurfaceOverviewID, Actor: "项目负责人", ExpectedResult: "能看到完整任务列表"}, {StepID: "step-apply-filter", Title: "按项目和标签组合筛选", SurfaceRef: genericSurfaceCollectionID, Actor: "项目负责人", ExpectedResult: "筛选结果与项目和标签关联保持一致"}}},
		{FlowID: "flow-review-project-progress", Title: "查看项目进度概览", Steps: []FlowStep{{StepID: "step-open-board", Title: "进入项目概览承载单元", SurfaceRef: genericSurfaceOverviewID, Actor: "项目负责人", ExpectedResult: "能看到项目级摘要和任务状态分布"}, {StepID: "step-open-detail", Title: "进入结果检查承载单元核对任务结果", SurfaceRef: genericSurfaceInspectionID, Actor: "项目负责人", ExpectedResult: "结果检查承载单元与概览摘要数据保持一致"}}},
	}
}

func buildInventoryFeatureList() []Feature {
	return []Feature{
		{FeatureID: "feature-inventory-board", Title: "库存看板", Summary: "概览承载单元展示低库存预警、待处理库存单和仓库维度摘要。", Priority: "p0", Required: true, RelatedScenarios: []string{"scenario-inventory-board"}, RelatedSurfaceRefs: []string{genericSurfaceOverviewID}, AcceptanceRefs: []string{"ac-overview", "ac-navigation"}},
		{FeatureID: "feature-sheet-list", Title: "库存单列表与筛选", Summary: "集合浏览与结果检查承载单元支持按仓库、低库存状态和盘点状态交叉筛选库存单与明细项。", Priority: "p0", Required: true, RelatedScenarios: []string{"scenario-filter-items"}, RelatedSurfaceRefs: []string{genericSurfaceCollectionID, genericSurfaceInspectionID}, AcceptanceRefs: []string{"ac-list", "ac-detail", "ac-filter"}},
		{FeatureID: "feature-sheet-editor", Title: "库存单编辑与仓库归属", Summary: "实体变更承载单元支持创建库存单、选择仓库并维护多个明细项。", Priority: "p0", Required: true, RelatedScenarios: []string{"scenario-create-sheet"}, RelatedSurfaceRefs: []string{genericSurfaceMutationID}, AcceptanceRefs: []string{"ac-form"}},
		{FeatureID: "feature-line-item-binding", Title: "明细项维护", Summary: "库存单支持维护多个明细项，并在集合浏览与结果检查承载单元保持一致显示。", Priority: "p0", Required: true, RelatedScenarios: []string{"scenario-create-sheet", "scenario-filter-items"}, RelatedSurfaceRefs: []string{genericSurfaceMutationID, genericSurfaceCollectionID, genericSurfaceInspectionID}, AcceptanceRefs: []string{"ac-line-item-binding"}},
		{FeatureID: "feature-local-storage", Title: "本地持久化", Summary: "库存单、明细项、SKU 和仓库数据在应用重启后仍可恢复。", Priority: "p1", Required: true, RelatedScenarios: []string{"scenario-inventory-board", "scenario-create-sheet", "scenario-filter-items"}, RelatedSurfaceRefs: []string{genericSurfaceOverviewID, genericSurfaceCollectionID, genericSurfaceMutationID, genericSurfaceInspectionID}, AcceptanceRefs: []string{"ac-persistence"}},
	}
}

func buildInventoryUserFlows() []UserFlow {
	return []UserFlow{
		{FlowID: "flow-create-sheet-with-items", Title: "创建带明细项的库存单", Steps: []FlowStep{{StepID: "step-open-editor", Title: "从概览承载单元进入实体变更入口", SurfaceRef: genericSurfaceOverviewID, Actor: "仓储负责人", ExpectedResult: "可选择仓库并输入库存单基础字段"}, {StepID: "step-edit-line-items", Title: "维护多个明细项并保存", SurfaceRef: genericSurfaceMutationID, Actor: "仓储负责人", ExpectedResult: "库存单与明细项写入成功"}, {StepID: "step-review-detail", Title: "进入结果检查承载单元确认仓库与明细差异", SurfaceRef: genericSurfaceInspectionID, Actor: "仓储负责人", ExpectedResult: "结果检查承载单元展示所属仓库和明细项集合"}}},
		{FlowID: "flow-filter-low-stock-items", Title: "按仓库和低库存状态筛选", Steps: []FlowStep{{StepID: "step-open-list", Title: "从概览承载单元进入集合浏览承载单元", SurfaceRef: genericSurfaceOverviewID, Actor: "仓储负责人", ExpectedResult: "能看到完整库存单和明细项列表"}, {StepID: "step-apply-filter", Title: "按仓库和低库存状态组合筛选", SurfaceRef: genericSurfaceCollectionID, Actor: "仓储负责人", ExpectedResult: "筛选结果与库存单和明细项关系保持一致"}}},
		{FlowID: "flow-review-low-stock-summary", Title: "查看低库存预警概览", Steps: []FlowStep{{StepID: "step-open-board", Title: "进入库存概览承载单元", SurfaceRef: genericSurfaceOverviewID, Actor: "仓储负责人", ExpectedResult: "能看到低库存预警和待处理库存单摘要"}, {StepID: "step-open-detail", Title: "进入结果检查承载单元核对库存差异", SurfaceRef: genericSurfaceInspectionID, Actor: "仓储负责人", ExpectedResult: "结果检查承载单元与概览摘要数据保持一致"}}},
	}
}

func featureListHasID(featureList []Feature, featureID string) bool {
	for _, feature := range featureList {
		if strings.TrimSpace(feature.FeatureID) == featureID {
			return true
		}
	}
	return false
}

func buildBookkeepingTaskBundle() []appruns.TaskBundleItem {
	surfaceRefs := newGenericSurfaceRefSet(genericTopologyPlan{Overview: true, Collection: true, Mutation: true}, "entity-entry", "entity-summary")

	return []appruns.TaskBundleItem{
		{TaskID: "task-domain-models", Title: "冻结账单实体与收支汇总模型", Category: "domain", TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, Objective: "先明确记账 App 的账单实体、派生汇总和目录落点，避免后续承载单元与存储边界反复漂移。", Priority: "p0", RelatedRequirements: []string{"feature-local-data", "feature-home-summary"}, TargetPaths: []string{"lib/models/entry.dart", "lib/models/summary.dart"}, OutputExpectations: []string{"账单实体字段稳定", "月度汇总可从账单实体推导"}, CompletionCriteria: []string{"账单记录包含金额、分类、日期和备注", "汇总模型可以表达收入、支出和结余"}, RiskNotes: []string{"如果实体字段频繁变化，后续承载单元和测试都会跟着返工"}, AllocationTransition: genericSurfaceTransition("task-domain-models", nil, surfaceRefs.AllSurfaces, surfaceRefs.SummaryEntityRefs, nil, nil, nil)},
		{TaskID: "task-storage-wiring", Title: "接通本地持久化与账单仓储边界", Category: "storage", TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, Objective: "把本地持久化方案和 repository 边界固定下来，确保新增账单后可恢复概览与账单集合。", Priority: "p0", RelatedRequirements: []string{"feature-local-data"}, Dependencies: []string{"task-domain-models"}, TargetPaths: []string{"lib/repositories/entry_repository.dart", "pubspec.yaml"}, OutputExpectations: []string{"明确的本地持久化实现", "账单读写入口稳定"}, CompletionCriteria: []string{"代码或依赖中出现明确的本地持久化实现", "应用重启后仍能读取账单集合并恢复概览"}, RiskNotes: []string{"不要同时引入两个同类本地存储方案"}, AllocationTransition: genericSurfaceTransition("task-storage-wiring", []string{"ac-persistence", "mrp-data"}, surfaceRefs.AllSurfaces, surfaceRefs.SummaryEntityRefs, nil, nil, nil)},
		{TaskID: "task-bind-core-surfaces", Title: "绑定概览、记账动作与集合浏览承载单元", Category: "screen", TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, Objective: "先把概览承载、实体变更承载、集合浏览承载和主导航入口搭出来，不把默认 seed 骨架当作完成结果。", Priority: "p0", RelatedRequirements: []string{"feature-home-summary", "feature-add-entry", "feature-ledger"}, Dependencies: []string{"task-domain-models"}, TargetPaths: []string{"lib/main.dart", "lib/views/home_page.dart", "lib/views/entry_form_page.dart", "lib/views/entry_list_page.dart"}, OutputExpectations: []string{"概览承载单元骨架", "记账动作承载单元骨架", "账单集合浏览承载单元骨架"}, CompletionCriteria: []string{"概览、记账动作、账单集合浏览之间入口明确可达", "默认 counter demo 页面与文案已被移除"}, RiskNotes: []string{"承载单元骨架完成不等于记账主流程闭环已接通"}, AllocationTransition: genericSurfaceTransitionWithBindings("task-bind-core-surfaces", []string{"mrp-copy"}, []string{publicBindingDomainCopy}, surfaceRefs.AllSurfaces, surfaceRefs.SummaryEntityRefs, nil, nil, nil)},
		{TaskID: "task-flow-wiring", Title: "接通记账动作、概览刷新与账单集合回显主流程", Category: "flow", TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, Objective: "把账单录入保存、概览刷新和账单集合回显串成一个完整功能闭环。", Priority: "p0", RelatedRequirements: []string{"feature-add-entry", "feature-home-summary", "feature-ledger", "feature-local-data"}, Dependencies: []string{"task-storage-wiring", "task-bind-core-surfaces"}, TargetPaths: []string{"lib/controllers/home_controller.dart", "lib/controllers/entry_form_controller.dart", "lib/controllers/entry_list_controller.dart", "lib/repositories/entry_repository.dart", "test/widget_test.dart"}, OutputExpectations: []string{"保存动作可触发持久化", "概览和账单集合能看到新增账单"}, CompletionCriteria: []string{"记账动作字段齐全并可保存", "新增账单后概览与账单集合都能反映最新数据"}, RiskNotes: []string{"不要为了接线方便重做整套承载结构或状态管理"}, AllocationTransition: genericSurfaceTransition("task-flow-wiring", []string{"check-counter-demo-removed"}, surfaceRefs.AllSurfaces, surfaceRefs.SummaryEntityRefs, nil, nil, []string{"记账动作、概览刷新与账单集合回显主流程已接通"})},
		{TaskID: "task-validation-closure", Title: "完成 analyze、test 与 APK 构建收口", Category: "validation", TaskType: appruns.BuilderRuntimeTaskTypeClosureRepair, Objective: "在功能闭环接通后，通过低风险修复把工作区收敛到 analyze、test 和 debug APK 全通过。", Priority: "p0", RelatedRequirements: []string{"ac-entry", "ac-ledger", "ac-persistence", "ac-navigation"}, Dependencies: []string{"task-flow-wiring"}, TargetPaths: []string{"lib/main.dart", "lib/views/**", "lib/controllers/**", "lib/repositories/**", "test/**", "pubspec.yaml"}, OutputExpectations: []string{"静态检查通过", "测试通过", "可构建 Debug APK"}, CompletionCriteria: []string{"flutter analyze 无错误", "flutter test 全通过", "flutter build apk --debug 成功"}, RiskNotes: []string{"只做低风险收口，不把单点失败扩展为大面积自由重构"}, AllocationTransition: genericSurfaceTransition("task-validation-closure", nil, surfaceRefs.AllSurfaces, surfaceRefs.SummaryEntityRefs, nil, nil, nil)},
	}

}

func buildRelationRichTaskBundle() []appruns.TaskBundleItem {
	relationRefs := []string{"relation-project-task", "relation-task-tag"}
	entityRefs := []string{"entity-project", "entity-task", "entity-tag", "entity-task-tag-link"}
	boardEntityRefs := []string{"entity-project-summary", "entity-project", "entity-task", "entity-tag", "entity-task-tag-link"}
	overviewSurfaces := []string{genericSurfaceOverviewID}
	collectionSurfaces := []string{genericSurfaceCollectionID}
	mutationSurfaces := []string{genericSurfaceMutationID}
	inspectionSurfaces := []string{genericSurfaceInspectionID}
	collectionInspectionSurfaces := []string{genericSurfaceCollectionID, genericSurfaceInspectionID}
	allSurfaces := []string{genericSurfaceOverviewID, genericSurfaceCollectionID, genericSurfaceMutationID, genericSurfaceInspectionID}
	repoShared := []string{"shared-record-repository"}
	boardShared := []string{"shared-record-repository", "shared-project-board-query"}
	filterShared := []string{"shared-record-repository", "shared-filter-state"}
	tagSelectionShared := []string{"shared-record-repository", "shared-tag-selection"}
	wiringShared := []string{"shared-record-repository", "shared-project-board-query", "shared-filter-state", "shared-tag-selection"}

	return []appruns.TaskBundleItem{
		{TaskID: "task-create-relation-models", Title: "创建项目、任务、标签与关联模型", Category: appruns.TaskCategoryDomain, TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, RouteHint: appruns.TaskRouteHintDeterministic, RiskLevel: appruns.TaskRiskLevelHigh, Objective: "先冻结项目、任务、标签和任务标签关联四类实体，避免后续把多对多关系重新压扁成单实体。", Priority: "p0", RelatedRequirements: []string{"ac-list", "ac-form", "ac-detail", "ac-tag-binding"}, TargetPaths: []string{"lib/models/project.dart", "lib/models/task.dart", "lib/models/tag.dart", "lib/models/task_tag_link.dart"}, OutputExpectations: []string{"多实体模型字段稳定", "任务与标签的关联边界明确"}, CompletionCriteria: []string{"项目、任务、标签和关联模型字段已冻结", "任务与标签关系不再退回单字段拼接"}, RiskNotes: []string{"如果多实体模型边界摇摆，后续 controller、页面和测试都会跟着返工"}, AllocationTransition: relationRichTransition("task-create-relation-models", []string{"ac-list", "ac-form", "ac-detail", "ac-tag-binding"}, allSurfaces, entityRefs, relationRefs, []string{"shared-domain-schema"}, []string{"lib/models/project.dart", "lib/models/task.dart", "lib/models/tag.dart", "lib/models/task_tag_link.dart"}, nil, []string{"relation-rich 领域模型已创建", "项目与标签关联边界已冻结"})},
		{TaskID: "task-create-summary-model", Title: "创建项目看板摘要模型", Category: appruns.TaskCategorySummary, TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, RouteHint: appruns.TaskRouteHintDeterministic, RiskLevel: appruns.TaskRiskLevelHigh, Objective: "基于项目和任务关系创建首页摘要模型，让看板直接承接项目级任务分布。", Priority: "p0", RelatedRequirements: []string{"ac-overview"}, Dependencies: []string{"task-create-relation-models"}, TargetPaths: []string{"lib/models/dashboard_summary.dart"}, OutputExpectations: []string{"项目看板摘要字段稳定", "摘要字段可由任务与标签关系推导"}, CompletionCriteria: []string{"项目看板摘要字段已明确", "首页摘要不再退回单记录计数"}, RiskNotes: []string{"摘要模型必须与项目和任务关系保持同一语义口径"}, AllocationTransition: relationRichTransition("task-create-summary-model", []string{"ac-overview"}, overviewSurfaces, boardEntityRefs, relationRefs, []string{"shared-project-board-query"}, []string{"lib/models/dashboard_summary.dart"}, []string{"task-create-relation-models"}, []string{"项目看板摘要模型已创建", "首页可消费项目级聚合字段"})},
		{TaskID: "task-create-repository", Title: "创建共享任务仓储边界", Category: appruns.TaskCategoryStorage, TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, RouteHint: appruns.TaskRouteHintDeterministic, Objective: "把项目、任务、标签与关联读写收口到统一仓储，避免页面层各自拼接关系。", Priority: "p0", RelatedRequirements: []string{"ac-persistence", "ac-tag-binding"}, Dependencies: []string{"task-create-relation-models"}, TargetPaths: []string{"lib/repositories/record_repository.dart"}, OutputExpectations: []string{"共享仓储边界稳定", "项目/任务/标签读写入口统一"}, CompletionCriteria: []string{"共享仓储已能承接多实体读写", "任务与标签关系可通过仓储持久化"}, RiskNotes: []string{"不要把看板查询、筛选状态和标签选择分散到多个临时存储入口"}, AllocationTransition: relationRichTransition("task-create-repository", []string{"ac-persistence", "ac-tag-binding"}, allSurfaces, boardEntityRefs, relationRefs, repoShared, []string{"lib/repositories/record_repository.dart"}, []string{"task-create-relation-models"}, []string{"共享任务仓储已创建", "多实体读写入口已统一"})},
		{TaskID: "task-create-home-controller", Title: "创建项目看板控制器", Category: appruns.TaskCategorySummary, TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, RouteHint: appruns.TaskRouteHintDeterministic, Objective: "围绕共享仓储和摘要模型创建首页控制器，承接项目级任务进度和重点标签读取。", Priority: "p0", RelatedRequirements: []string{"ac-overview", "ac-navigation"}, Dependencies: []string{"task-create-repository", "task-create-summary-model"}, TargetPaths: []string{"lib/controllers/home_controller.dart"}, OutputExpectations: []string{"首页控制器可读取项目看板摘要", "项目和标签的聚合查询入口稳定"}, CompletionCriteria: []string{"首页控制器可输出项目级摘要和重点标签", "首页不再只围绕单记录摘要"}, RiskNotes: []string{"项目看板查询必须和共享仓储使用同一套关系边界"}, AllocationTransition: relationRichTransition("task-create-home-controller", []string{"ac-overview", "ac-navigation"}, overviewSurfaces, boardEntityRefs, relationRefs, boardShared, []string{"lib/controllers/home_controller.dart"}, []string{"task-create-repository", "task-create-summary-model"}, []string{"项目看板控制器已创建", "首页可消费共享看板查询"})},
		{TaskID: "task-create-form-controller", Title: "创建任务编辑与标签绑定控制器", Category: appruns.TaskCategoryFlow, TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, RouteHint: appruns.TaskRouteHintDeterministic, Objective: "围绕共享仓储创建任务编辑控制器，负责项目选择、标签绑定和保存动作。", Priority: "p0", RelatedRequirements: []string{"ac-form", "ac-tag-binding"}, Dependencies: []string{"task-create-repository", "task-create-relation-models"}, TargetPaths: []string{"lib/controllers/record_form_controller.dart"}, OutputExpectations: []string{"编辑控制器承接项目与标签选择", "保存动作可写入共享仓储"}, CompletionCriteria: []string{"表单状态与多实体字段保持一致", "任务与标签绑定动作可保存"}, RiskNotes: []string{"不要把标签选择状态散落到页面私有字段，必须与共享仓储保持协调"}, AllocationTransition: relationRichTransition("task-create-form-controller", []string{"ac-form", "ac-tag-binding"}, mutationSurfaces, entityRefs, relationRefs, tagSelectionShared, []string{"lib/controllers/record_form_controller.dart"}, []string{"task-create-repository", "task-create-relation-models"}, []string{"任务编辑控制器已创建", "标签绑定动作已接入共享仓储"})},
		{TaskID: "task-create-list-controller", Title: "创建任务列表与筛选控制器", Category: appruns.TaskCategoryFlow, TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, RouteHint: appruns.TaskRouteHintDeterministic, Objective: "围绕共享仓储创建任务列表控制器，负责项目/标签筛选和详情跳转入口。", Priority: "p0", RelatedRequirements: []string{"ac-list", "ac-filter", "ac-detail"}, Dependencies: []string{"task-create-repository", "task-create-relation-models"}, TargetPaths: []string{"lib/controllers/record_list_controller.dart"}, OutputExpectations: []string{"列表控制器提供项目与标签筛选", "详情跳转仍和筛选结果保持一致"}, CompletionCriteria: []string{"任务列表可按项目和标签筛选", "列表与详情共享同一任务集合语义"}, RiskNotes: []string{"筛选状态必须视为共享归属面，不能在页面间重复推导"}, AllocationTransition: relationRichTransition("task-create-list-controller", []string{"ac-list", "ac-filter", "ac-detail"}, collectionInspectionSurfaces, entityRefs, relationRefs, filterShared, []string{"lib/controllers/record_list_controller.dart"}, []string{"task-create-repository", "task-create-relation-models"}, []string{"任务列表控制器已创建", "项目与标签筛选状态已稳定"})},
		{TaskID: "task-create-copy", Title: "创建 relation-rich 领域文案投影", Category: appruns.TaskCategoryContent, TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, RouteHint: appruns.TaskRouteHintDeterministic, RiskLevel: appruns.TaskRiskLevelHigh, Objective: "基于参考模板创建 open_lite_copy.dart，让项目、任务和标签文案在概览、集合浏览、实体变更和结果检查承载单元中保持一致。", Priority: "p0", RelatedRequirements: []string{"mrp-domain-wording"}, Dependencies: []string{"task-create-relation-models"}, TargetPaths: []string{"lib/template/open_lite_copy.dart"}, OutputExpectations: []string{"领域文案投影稳定", "多承载单元文案不再残留 generic record 语义"}, CompletionCriteria: []string{"项目、任务和标签文案已覆盖默认 seed", "多承载单元文案口径保持一致"}, RiskNotes: []string{"relation-rich 样例最容易在 copy 层残留 generic 词汇"}, AllocationTransition: relationRichTransition("task-create-copy", []string{"mrp-domain-wording"}, allSurfaces, []string{"entity-project", "entity-task", "entity-tag"}, relationRefs, nil, []string{"lib/template/open_lite_copy.dart"}, []string{"task-create-relation-models"}, []string{"relation-rich 领域文案已创建", "默认 generic 文案已被替换"})},
		{TaskID: "task-create-android-branding", Title: "创建 Android 启动器文案", Category: appruns.TaskCategoryContent, TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, RouteHint: appruns.TaskRouteHintDeterministic, Objective: "在 strings.xml 中创建 relation-rich 启动器品牌文案，确保不再残留默认 seed branding。", Priority: "p0", RelatedRequirements: []string{"mrp-domain-wording"}, Dependencies: []string{"task-create-copy"}, TargetPaths: []string{"android/app/src/main/res/values/strings.xml"}, OutputExpectations: []string{"启动器名称与领域文案一致", "默认 seed branding 不再残留"}, CompletionCriteria: []string{"Android 启动器名称已切换到 relation-rich 领域文案", "默认 branding 已移除"}, RiskNotes: []string{"启动器 branding 不应和页面 copy 口径分离"}, AllocationTransition: relationRichTransition("task-create-android-branding", []string{"mrp-domain-wording"}, allSurfaces, []string{"entity-project", "entity-task", "entity-tag"}, nil, nil, []string{"android/app/src/main/res/values/strings.xml"}, []string{"task-create-copy"}, []string{"Android relation-rich branding 已创建"})},
		{TaskID: "task-create-android-build-config", Title: "创建 Android 构建文案入口", Category: appruns.TaskCategoryContent, TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, RouteHint: appruns.TaskRouteHintDeterministic, Objective: "在 build.gradle.kts 中保留 Android branding 的单点覆盖入口，避免 relation-rich 品牌改动分散。", Priority: "p0", RelatedRequirements: []string{"mrp-domain-wording"}, Dependencies: []string{"task-create-android-branding"}, TargetPaths: []string{"android/app/build.gradle.kts"}, OutputExpectations: []string{"Android branding 构建入口稳定"}, CompletionCriteria: []string{"build.gradle.kts 保留单点 branding 覆盖入口"}, RiskNotes: []string{"不要让 relation-rich branding 改动扩散到未治理路径"}, AllocationTransition: relationRichTransition("task-create-android-build-config", []string{"mrp-domain-wording"}, allSurfaces, nil, nil, nil, []string{"android/app/build.gradle.kts"}, []string{"task-create-android-branding"}, []string{"Android branding 构建入口已创建"})},
		{TaskID: taskBindOverviewSurfaceID, Title: "绑定项目概览承载单元", Category: appruns.TaskCategoryScreen, TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, RouteHint: appruns.TaskRouteHintDeterministic, Objective: "根据首页控制器和领域文案绑定项目概览承载单元，展示项目级摘要和重点标签。", Priority: "p0", RelatedRequirements: []string{"ac-overview", "mrp-domain-wording"}, Dependencies: []string{"task-create-home-controller", "task-create-copy"}, TargetPaths: []string{"lib/views/home_page.dart"}, OutputExpectations: []string{"项目概览承载单元可渲染项目摘要", "承载单元文案与领域语义一致"}, CompletionCriteria: []string{"概览承载单元能展示项目级任务分布和重点标签", "承载单元标题与领域语义一致"}, RiskNotes: []string{"概览承载单元必须和共享查询边界保持一致，不能自行拼聚合数据"}, AllocationTransition: relationRichTransition(taskBindOverviewSurfaceID, []string{"ac-overview", "mrp-domain-wording"}, overviewSurfaces, boardEntityRefs, relationRefs, boardShared, []string{"lib/views/home_page.dart"}, []string{"task-create-home-controller", "task-create-copy"}, []string{"项目概览承载单元已绑定", "概览摘要和标签入口已接入"})},
		{TaskID: taskBindCollectionSurfaceID, Title: "绑定集合浏览承载单元", Category: appruns.TaskCategoryScreen, TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, RouteHint: appruns.TaskRouteHintDeterministic, Objective: "根据列表控制器和领域文案绑定集合浏览承载单元，提供项目/标签筛选与结果定位入口。", Priority: "p0", RelatedRequirements: []string{"ac-list", "ac-filter", "ac-detail"}, Dependencies: []string{"task-create-list-controller", "task-create-copy"}, TargetPaths: []string{"lib/views/record_list_page.dart"}, OutputExpectations: []string{"集合浏览承载单元可展示筛选结果", "结果定位入口稳定"}, CompletionCriteria: []string{"集合浏览承载单元能按项目和标签筛选任务", "筛选结果和结果检查入口保持一致"}, RiskNotes: []string{"集合浏览承载单元不能自己维护一套与 controller 不一致的筛选状态"}, AllocationTransition: relationRichTransition(taskBindCollectionSurfaceID, []string{"ac-list", "ac-filter", "ac-detail"}, collectionSurfaces, entityRefs, relationRefs, filterShared, []string{"lib/views/record_list_page.dart"}, []string{"task-create-list-controller", "task-create-copy"}, []string{"集合浏览承载单元已绑定", "项目与标签筛选入口已接入"})},
		{TaskID: taskBindMutationSurfaceID, Title: "绑定实体变更承载单元", Category: appruns.TaskCategoryScreen, TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, RouteHint: appruns.TaskRouteHintDeterministic, Objective: "根据编辑控制器和领域文案绑定实体变更承载单元，承接项目选择和多标签绑定。", Priority: "p0", RelatedRequirements: []string{"ac-form", "ac-tag-binding", "mrp-domain-wording"}, Dependencies: []string{"task-create-form-controller", "task-create-copy"}, TargetPaths: []string{"lib/views/record_form_page.dart"}, OutputExpectations: []string{"实体变更承载单元承接项目和标签输入", "保存动作已接线到控制器"}, CompletionCriteria: []string{"实体变更承载单元可选择所属项目并绑定多个标签", "保存动作已连接到共享仓储"}, RiskNotes: []string{"实体变更承载单元不能绕开共享标签选择状态单独保存"}, AllocationTransition: relationRichTransition(taskBindMutationSurfaceID, []string{"ac-form", "ac-tag-binding", "mrp-domain-wording"}, mutationSurfaces, entityRefs, relationRefs, tagSelectionShared, []string{"lib/views/record_form_page.dart"}, []string{"task-create-form-controller", "task-create-copy"}, []string{"实体变更承载单元已绑定", "项目与标签输入链路已接入"})},
		{TaskID: taskBindInspectionSurfaceID, Title: "绑定结果检查承载单元", Category: appruns.TaskCategoryScreen, TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, RouteHint: appruns.TaskRouteHintDeterministic, Objective: "根据列表控制器和领域文案绑定结果检查承载单元，展示所属项目、标签集合和主要字段。", Priority: "p0", RelatedRequirements: []string{"ac-detail", "ac-tag-binding", "mrp-domain-wording"}, Dependencies: []string{"task-create-list-controller", "task-create-copy"}, TargetPaths: []string{"lib/views/record_detail_page.dart"}, OutputExpectations: []string{"结果检查承载单元展示项目和标签关联", "结果检查与集合结果语义一致"}, CompletionCriteria: []string{"结果检查承载单元展示所属项目与标签集合", "结果检查字段与集合筛选结果保持一致"}, RiskNotes: []string{"结果检查承载单元不应重新拼接一套和集合浏览不同的标签关系"}, AllocationTransition: relationRichTransition(taskBindInspectionSurfaceID, []string{"ac-detail", "ac-tag-binding", "mrp-domain-wording"}, inspectionSurfaces, entityRefs, relationRefs, filterShared, []string{"lib/views/record_detail_page.dart"}, []string{"task-create-list-controller", "task-create-copy"}, []string{"结果检查承载单元已绑定", "项目与标签结果检查已接入"})},
		{TaskID: taskBindAppEntryID, Title: "接线 relation-rich 应用入口与承载单元路由", Category: appruns.TaskCategoryScreen, TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, RouteHint: appruns.TaskRouteHintDeterministic, Objective: "把概览、集合浏览、实体变更、结果检查承载单元、共享仓储与文案投影接线到应用入口，形成稳定的交互骨架。", Priority: "p0", RelatedRequirements: []string{"ac-navigation", "ac-overview", "ac-list", "ac-form", "ac-detail"}, Dependencies: []string{taskBindOverviewSurfaceID, taskBindCollectionSurfaceID, taskBindMutationSurfaceID, taskBindInspectionSurfaceID, "task-create-home-controller", "task-create-list-controller", "task-create-form-controller", "task-create-repository"}, TargetPaths: []string{"lib/main.dart"}, OutputExpectations: []string{"应用入口完成接线", "四类核心承载单元可达"}, CompletionCriteria: []string{"概览、集合浏览、实体变更、结果检查承载单元之间导航可达", "共享仓储与承载单元入口已统一接线"}, RiskNotes: []string{"应用入口不应再把复杂领域退回固定页面导航假设"}, AllocationTransition: relationRichTransition(taskBindAppEntryID, []string{"ac-navigation", "ac-overview", "ac-list", "ac-form", "ac-detail"}, allSurfaces, boardEntityRefs, relationRefs, wiringShared, []string{"lib/main.dart"}, []string{taskBindOverviewSurfaceID, taskBindCollectionSurfaceID, taskBindMutationSurfaceID, taskBindInspectionSurfaceID, "task-create-home-controller", "task-create-list-controller", "task-create-form-controller", "task-create-repository"}, []string{"relation-rich 应用入口已接线", "复杂领域交互骨架已可达"})},
		{TaskID: "task-create-test", Title: "创建 relation-rich 最小 Widget 测试", Category: appruns.TaskCategoryFlow, TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, RouteHint: appruns.TaskRouteHintDeterministic, Objective: "为项目概览、任务筛选和标签绑定主流程创建最小 widget 测试，确保后续扩展有稳定入口。", Priority: "p0", RelatedRequirements: []string{"ac-navigation", "ac-filter", "ac-tag-binding"}, Dependencies: []string{taskBindAppEntryID}, TargetPaths: []string{"test/widget_test.dart"}, OutputExpectations: []string{"测试入口覆盖复杂领域主流程", "断言与领域文案和关系边界同步"}, CompletionCriteria: []string{"Widget 测试能覆盖项目概览和任务筛选主流程", "任务与标签绑定断言与当前领域语义一致"}, RiskNotes: []string{"测试不能退回 generic 单记录 smoke test"}, AllocationTransition: relationRichTransition("task-create-test", []string{"ac-navigation", "ac-filter", "ac-tag-binding"}, allSurfaces, boardEntityRefs, relationRefs, wiringShared, []string{"test/widget_test.dart"}, []string{taskBindAppEntryID}, []string{"relation-rich Widget 测试已创建", "主流程断言与复杂领域语义同步"})},
	}
}

func buildInventorySheetLineItemTaskBundle() []appruns.TaskBundleItem {
	relationRefs := []string{"relation-sheet-line-item", "relation-line-item-sku"}
	entityRefs := []string{"entity-inventory-sheet", "entity-line-item", "entity-sku", "entity-warehouse"}
	boardEntityRefs := []string{"entity-low-stock-summary", "entity-inventory-sheet", "entity-line-item", "entity-sku", "entity-warehouse"}
	overviewSurfaces := []string{genericSurfaceOverviewID}
	collectionSurfaces := []string{genericSurfaceCollectionID}
	mutationSurfaces := []string{genericSurfaceMutationID}
	inspectionSurfaces := []string{genericSurfaceInspectionID}
	collectionInspectionSurfaces := []string{genericSurfaceCollectionID, genericSurfaceInspectionID}
	allSurfaces := []string{genericSurfaceOverviewID, genericSurfaceCollectionID, genericSurfaceMutationID, genericSurfaceInspectionID}
	repoShared := []string{"shared-inventory-repository"}
	boardShared := []string{"shared-inventory-repository", "shared-low-stock-board-query"}
	filterShared := []string{"shared-inventory-repository", "shared-filter-state"}
	lineItemShared := []string{"shared-inventory-repository", "shared-line-item-editor"}
	wiringShared := []string{"shared-inventory-repository", "shared-low-stock-board-query", "shared-filter-state", "shared-line-item-editor"}

	return []appruns.TaskBundleItem{
		{TaskID: "task-create-relation-models", Title: "创建库存单、明细项、SKU 与仓库模型", Category: appruns.TaskCategoryDomain, TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, RouteHint: appruns.TaskRouteHintDeterministic, RiskLevel: appruns.TaskRiskLevelHigh, Objective: "先冻结库存单、明细项、SKU 和仓库四类实体，避免后续把一对多关系重新压扁成单实体。", Priority: "p0", RelatedRequirements: []string{"ac-list", "ac-form", "ac-detail", "ac-line-item-binding"}, TargetPaths: []string{"lib/models/inventory_sheet.dart", "lib/models/line_item.dart", "lib/models/sku.dart", "lib/models/warehouse.dart"}, OutputExpectations: []string{"多实体模型字段稳定", "库存单与明细项的关联边界明确"}, CompletionCriteria: []string{"库存单、明细项、SKU 和仓库模型字段已冻结", "库存单与明细项关系不再退回单字段拼接"}, RiskNotes: []string{"如果多实体模型边界摇摆，后续 controller、页面和测试都会跟着返工"}, AllocationTransition: relationRichTransition("task-create-relation-models", []string{"ac-list", "ac-form", "ac-detail", "ac-line-item-binding"}, allSurfaces, entityRefs, relationRefs, []string{"shared-domain-schema"}, []string{"lib/models/inventory_sheet.dart", "lib/models/line_item.dart", "lib/models/sku.dart", "lib/models/warehouse.dart"}, nil, []string{"库存 relation-rich 领域模型已创建", "库存单与明细项关联边界已冻结"})},
		{TaskID: "task-create-summary-model", Title: "创建低库存摘要模型", Category: appruns.TaskCategorySummary, TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, RouteHint: appruns.TaskRouteHintDeterministic, RiskLevel: appruns.TaskRiskLevelHigh, Objective: "基于库存单和明细项关系创建首页低库存摘要模型，让看板直接承接仓库级库存风险分布。", Priority: "p0", RelatedRequirements: []string{"ac-overview"}, Dependencies: []string{"task-create-relation-models"}, TargetPaths: []string{"lib/models/dashboard_summary.dart"}, OutputExpectations: []string{"低库存摘要字段稳定", "摘要字段可由库存单与明细项关系推导"}, CompletionCriteria: []string{"低库存摘要字段已明确", "首页摘要不再退回单记录计数"}, RiskNotes: []string{"摘要模型必须与库存单和明细项关系保持同一语义口径"}, AllocationTransition: relationRichTransition("task-create-summary-model", []string{"ac-overview"}, overviewSurfaces, boardEntityRefs, relationRefs, []string{"shared-low-stock-board-query"}, []string{"lib/models/dashboard_summary.dart"}, []string{"task-create-relation-models"}, []string{"低库存摘要模型已创建", "首页可消费仓库级聚合字段"})},
		{TaskID: "task-create-repository", Title: "创建共享库存仓储边界", Category: appruns.TaskCategoryStorage, TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, RouteHint: appruns.TaskRouteHintDeterministic, Objective: "把库存单、明细项、SKU 与仓库读写收口到统一仓储，避免页面层各自拼接关系。", Priority: "p0", RelatedRequirements: []string{"ac-persistence", "ac-line-item-binding"}, Dependencies: []string{"task-create-relation-models"}, TargetPaths: []string{"lib/repositories/record_repository.dart"}, OutputExpectations: []string{"共享仓储边界稳定", "库存单/明细项/SKU/仓库读写入口统一"}, CompletionCriteria: []string{"共享仓储已能承接多实体读写", "库存单与明细项关系可通过仓储持久化"}, RiskNotes: []string{"不要把低库存查询、筛选状态和明细编辑分散到多个临时存储入口"}, AllocationTransition: relationRichTransition("task-create-repository", []string{"ac-persistence", "ac-line-item-binding"}, allSurfaces, boardEntityRefs, relationRefs, repoShared, []string{"lib/repositories/record_repository.dart"}, []string{"task-create-relation-models"}, []string{"共享库存仓储已创建", "多实体读写入口已统一"})},
		{TaskID: "task-create-home-controller", Title: "创建库存看板控制器", Category: appruns.TaskCategorySummary, TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, RouteHint: appruns.TaskRouteHintDeterministic, Objective: "围绕共享仓储和低库存摘要模型创建首页控制器，承接仓库级库存风险和待处理库存单读取。", Priority: "p0", RelatedRequirements: []string{"ac-overview", "ac-navigation"}, Dependencies: []string{"task-create-repository", "task-create-summary-model"}, TargetPaths: []string{"lib/controllers/home_controller.dart"}, OutputExpectations: []string{"首页控制器可读取低库存摘要", "仓库和库存单的聚合查询入口稳定"}, CompletionCriteria: []string{"首页控制器可输出仓库级低库存摘要和待处理库存单", "首页不再只围绕单记录摘要"}, RiskNotes: []string{"低库存看板查询必须和共享仓储使用同一套关系边界"}, AllocationTransition: relationRichTransition("task-create-home-controller", []string{"ac-overview", "ac-navigation"}, overviewSurfaces, boardEntityRefs, relationRefs, boardShared, []string{"lib/controllers/home_controller.dart"}, []string{"task-create-repository", "task-create-summary-model"}, []string{"库存看板控制器已创建", "首页可消费共享低库存查询"})},
		{TaskID: "task-create-form-controller", Title: "创建库存单编辑与明细维护控制器", Category: appruns.TaskCategoryFlow, TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, RouteHint: appruns.TaskRouteHintDeterministic, Objective: "围绕共享仓储创建库存单编辑控制器，负责仓库选择、明细项维护和保存动作。", Priority: "p0", RelatedRequirements: []string{"ac-form", "ac-line-item-binding"}, Dependencies: []string{"task-create-repository", "task-create-relation-models"}, TargetPaths: []string{"lib/controllers/record_form_controller.dart"}, OutputExpectations: []string{"编辑控制器承接仓库选择与明细项维护", "保存动作可写入共享仓储"}, CompletionCriteria: []string{"表单状态与多实体字段保持一致", "库存单与明细项维护动作可保存"}, RiskNotes: []string{"不要把明细项编辑状态散落到页面私有字段，必须与共享仓储保持协调"}, AllocationTransition: relationRichTransition("task-create-form-controller", []string{"ac-form", "ac-line-item-binding"}, mutationSurfaces, entityRefs, relationRefs, lineItemShared, []string{"lib/controllers/record_form_controller.dart"}, []string{"task-create-repository", "task-create-relation-models"}, []string{"库存单编辑控制器已创建", "明细项维护动作已接入共享仓储"})},
		{TaskID: "task-create-list-controller", Title: "创建库存列表与筛选控制器", Category: appruns.TaskCategoryFlow, TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, RouteHint: appruns.TaskRouteHintDeterministic, Objective: "围绕共享仓储创建库存列表控制器，负责按仓库和低库存状态筛选库存单/明细项，并提供详情跳转入口。", Priority: "p0", RelatedRequirements: []string{"ac-list", "ac-filter", "ac-detail"}, Dependencies: []string{"task-create-repository", "task-create-relation-models"}, TargetPaths: []string{"lib/controllers/record_list_controller.dart"}, OutputExpectations: []string{"列表控制器提供仓库和低库存筛选", "详情跳转仍和筛选结果保持一致"}, CompletionCriteria: []string{"库存列表可按仓库和低库存状态筛选", "列表与详情共享同一库存单集合语义"}, RiskNotes: []string{"筛选状态必须视为共享归属面，不能在页面间重复推导"}, AllocationTransition: relationRichTransition("task-create-list-controller", []string{"ac-list", "ac-filter", "ac-detail"}, collectionInspectionSurfaces, entityRefs, relationRefs, filterShared, []string{"lib/controllers/record_list_controller.dart"}, []string{"task-create-repository", "task-create-relation-models"}, []string{"库存列表控制器已创建", "仓库与低库存筛选状态已稳定"})},
		{TaskID: "task-create-copy", Title: "创建库存 relation-rich 领域文案投影", Category: appruns.TaskCategoryContent, TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, RouteHint: appruns.TaskRouteHintDeterministic, RiskLevel: appruns.TaskRiskLevelHigh, Objective: "基于参考模板创建 open_lite_copy.dart，让库存单、明细项、SKU 和仓库文案在概览、集合浏览、实体变更和结果检查承载单元中保持一致。", Priority: "p0", RelatedRequirements: []string{"mrp-domain-wording"}, Dependencies: []string{"task-create-relation-models"}, TargetPaths: []string{"lib/template/open_lite_copy.dart"}, OutputExpectations: []string{"领域文案投影稳定", "多承载单元文案不再残留 generic record 语义"}, CompletionCriteria: []string{"库存单、明细项和仓库文案已覆盖默认 seed", "多承载单元文案口径保持一致"}, RiskNotes: []string{"第二条 relation-rich 样例最容易在 copy 层残留 generic 词汇"}, AllocationTransition: relationRichTransition("task-create-copy", []string{"mrp-domain-wording"}, allSurfaces, []string{"entity-inventory-sheet", "entity-line-item", "entity-sku", "entity-warehouse"}, relationRefs, nil, []string{"lib/template/open_lite_copy.dart"}, []string{"task-create-relation-models"}, []string{"库存 relation-rich 领域文案已创建", "默认 generic 文案已被替换"})},
		{TaskID: "task-create-android-branding", Title: "创建库存 Android 启动器文案", Category: appruns.TaskCategoryContent, TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, RouteHint: appruns.TaskRouteHintDeterministic, Objective: "在 strings.xml 中创建库存 relation-rich 启动器品牌文案，确保不再残留默认 seed branding。", Priority: "p0", RelatedRequirements: []string{"mrp-domain-wording"}, Dependencies: []string{"task-create-copy"}, TargetPaths: []string{"android/app/src/main/res/values/strings.xml"}, OutputExpectations: []string{"启动器名称与领域文案一致", "默认 seed branding 不再残留"}, CompletionCriteria: []string{"Android 启动器名称已切换到库存 relation-rich 领域文案", "默认 branding 已移除"}, RiskNotes: []string{"启动器 branding 不应和页面 copy 口径分离"}, AllocationTransition: relationRichTransition("task-create-android-branding", []string{"mrp-domain-wording"}, allSurfaces, []string{"entity-inventory-sheet", "entity-line-item", "entity-sku"}, nil, nil, []string{"android/app/src/main/res/values/strings.xml"}, []string{"task-create-copy"}, []string{"Android 库存 relation-rich branding 已创建"})},
		{TaskID: "task-create-android-build-config", Title: "创建库存 Android 构建文案入口", Category: appruns.TaskCategoryContent, TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, RouteHint: appruns.TaskRouteHintDeterministic, Objective: "在 build.gradle.kts 中保留 Android branding 的单点覆盖入口，避免库存 relation-rich 品牌改动分散。", Priority: "p0", RelatedRequirements: []string{"mrp-domain-wording"}, Dependencies: []string{"task-create-android-branding"}, TargetPaths: []string{"android/app/build.gradle.kts"}, OutputExpectations: []string{"Android branding 构建入口稳定"}, CompletionCriteria: []string{"build.gradle.kts 保留单点 branding 覆盖入口"}, RiskNotes: []string{"不要让库存 relation-rich branding 改动扩散到未治理路径"}, AllocationTransition: relationRichTransition("task-create-android-build-config", []string{"mrp-domain-wording"}, allSurfaces, nil, nil, nil, []string{"android/app/build.gradle.kts"}, []string{"task-create-android-branding"}, []string{"Android branding 构建入口已创建"})},
		{TaskID: taskBindOverviewSurfaceID, Title: "绑定库存概览承载单元", Category: appruns.TaskCategoryScreen, TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, RouteHint: appruns.TaskRouteHintDeterministic, Objective: "根据首页控制器和领域文案绑定库存概览承载单元，展示低库存摘要和待处理库存单。", Priority: "p0", RelatedRequirements: []string{"ac-overview", "mrp-domain-wording"}, Dependencies: []string{"task-create-home-controller", "task-create-copy"}, TargetPaths: []string{"lib/views/home_page.dart"}, OutputExpectations: []string{"库存概览承载单元可渲染低库存摘要", "承载单元文案与领域语义一致"}, CompletionCriteria: []string{"概览承载单元能展示低库存预警和待处理库存单", "承载单元标题与领域语义一致"}, RiskNotes: []string{"概览承载单元必须和共享查询边界保持一致，不能自行拼聚合数据"}, AllocationTransition: relationRichTransition(taskBindOverviewSurfaceID, []string{"ac-overview", "mrp-domain-wording"}, overviewSurfaces, boardEntityRefs, relationRefs, boardShared, []string{"lib/views/home_page.dart"}, []string{"task-create-home-controller", "task-create-copy"}, []string{"库存概览承载单元已绑定", "概览摘要和预警入口已接入"})},
		{TaskID: taskBindCollectionSurfaceID, Title: "绑定集合浏览承载单元", Category: appruns.TaskCategoryScreen, TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, RouteHint: appruns.TaskRouteHintDeterministic, Objective: "根据列表控制器和领域文案绑定集合浏览承载单元，提供仓库/低库存筛选与结果定位入口。", Priority: "p0", RelatedRequirements: []string{"ac-list", "ac-filter", "ac-detail"}, Dependencies: []string{"task-create-list-controller", "task-create-copy"}, TargetPaths: []string{"lib/views/record_list_page.dart"}, OutputExpectations: []string{"集合浏览承载单元可展示筛选结果", "结果定位入口稳定"}, CompletionCriteria: []string{"集合浏览承载单元能按仓库和低库存状态筛选库存单与明细项", "筛选结果和结果检查入口保持一致"}, RiskNotes: []string{"集合浏览承载单元不能自己维护一套与 controller 不一致的筛选状态"}, AllocationTransition: relationRichTransition(taskBindCollectionSurfaceID, []string{"ac-list", "ac-filter", "ac-detail"}, collectionSurfaces, entityRefs, relationRefs, filterShared, []string{"lib/views/record_list_page.dart"}, []string{"task-create-list-controller", "task-create-copy"}, []string{"集合浏览承载单元已绑定", "仓库与低库存筛选入口已接入"})},
		{TaskID: taskBindMutationSurfaceID, Title: "绑定实体变更承载单元", Category: appruns.TaskCategoryScreen, TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, RouteHint: appruns.TaskRouteHintDeterministic, Objective: "根据编辑控制器和领域文案绑定实体变更承载单元，承接仓库选择和多明细项维护。", Priority: "p0", RelatedRequirements: []string{"ac-form", "ac-line-item-binding", "mrp-domain-wording"}, Dependencies: []string{"task-create-form-controller", "task-create-copy"}, TargetPaths: []string{"lib/views/record_form_page.dart"}, OutputExpectations: []string{"实体变更承载单元承接仓库和明细项输入", "保存动作已接线到控制器"}, CompletionCriteria: []string{"实体变更承载单元可选择所属仓库并维护多个明细项", "保存动作已连接到共享仓储"}, RiskNotes: []string{"实体变更承载单元不能绕开共享明细编辑状态单独保存"}, AllocationTransition: relationRichTransition(taskBindMutationSurfaceID, []string{"ac-form", "ac-line-item-binding", "mrp-domain-wording"}, mutationSurfaces, entityRefs, relationRefs, lineItemShared, []string{"lib/views/record_form_page.dart"}, []string{"task-create-form-controller", "task-create-copy"}, []string{"实体变更承载单元已绑定", "仓库与明细项输入链路已接入"})},
		{TaskID: taskBindInspectionSurfaceID, Title: "绑定结果检查承载单元", Category: appruns.TaskCategoryScreen, TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, RouteHint: appruns.TaskRouteHintDeterministic, Objective: "根据列表控制器和领域文案绑定结果检查承载单元，展示所属仓库、明细项集合和主要库存差异。", Priority: "p0", RelatedRequirements: []string{"ac-detail", "ac-line-item-binding", "mrp-domain-wording"}, Dependencies: []string{"task-create-list-controller", "task-create-copy"}, TargetPaths: []string{"lib/views/record_detail_page.dart"}, OutputExpectations: []string{"结果检查承载单元展示仓库和明细项关联", "结果检查与集合结果语义一致"}, CompletionCriteria: []string{"结果检查承载单元展示所属仓库与明细项集合", "结果检查字段与集合筛选结果保持一致"}, RiskNotes: []string{"结果检查承载单元不应重新拼接一套和集合浏览不同的明细项关系"}, AllocationTransition: relationRichTransition(taskBindInspectionSurfaceID, []string{"ac-detail", "ac-line-item-binding", "mrp-domain-wording"}, inspectionSurfaces, entityRefs, relationRefs, filterShared, []string{"lib/views/record_detail_page.dart"}, []string{"task-create-list-controller", "task-create-copy"}, []string{"结果检查承载单元已绑定", "仓库与明细项结果检查已接入"})},
		{TaskID: taskBindAppEntryID, Title: "接线库存 relation-rich 应用入口与承载单元路由", Category: appruns.TaskCategoryScreen, TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, RouteHint: appruns.TaskRouteHintDeterministic, Objective: "把概览、集合浏览、实体变更、结果检查承载单元、共享仓储与文案投影接线到应用入口，形成稳定的交互骨架。", Priority: "p0", RelatedRequirements: []string{"ac-navigation", "ac-overview", "ac-list", "ac-form", "ac-detail"}, Dependencies: []string{taskBindOverviewSurfaceID, taskBindCollectionSurfaceID, taskBindMutationSurfaceID, taskBindInspectionSurfaceID, "task-create-home-controller", "task-create-list-controller", "task-create-form-controller", "task-create-repository"}, TargetPaths: []string{"lib/main.dart"}, OutputExpectations: []string{"应用入口完成接线", "四类核心承载单元可达"}, CompletionCriteria: []string{"概览、集合浏览、实体变更、结果检查承载单元之间导航可达", "共享仓储与承载单元入口已统一接线"}, RiskNotes: []string{"应用入口不应再把复杂领域退回固定页面导航假设"}, AllocationTransition: relationRichTransition(taskBindAppEntryID, []string{"ac-navigation", "ac-overview", "ac-list", "ac-form", "ac-detail"}, allSurfaces, boardEntityRefs, relationRefs, wiringShared, []string{"lib/main.dart"}, []string{taskBindOverviewSurfaceID, taskBindCollectionSurfaceID, taskBindMutationSurfaceID, taskBindInspectionSurfaceID, "task-create-home-controller", "task-create-list-controller", "task-create-form-controller", "task-create-repository"}, []string{"库存 relation-rich 应用入口已接线", "复杂领域交互骨架已可达"})},
		{TaskID: "task-create-test", Title: "创建库存 relation-rich 最小 Widget 测试", Category: appruns.TaskCategoryFlow, TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, RouteHint: appruns.TaskRouteHintDeterministic, Objective: "为低库存概览、列表筛选和明细维护主流程创建最小 widget 测试，确保后续扩展有稳定入口。", Priority: "p0", RelatedRequirements: []string{"ac-navigation", "ac-filter", "ac-line-item-binding"}, Dependencies: []string{taskBindAppEntryID}, TargetPaths: []string{"test/widget_test.dart"}, OutputExpectations: []string{"测试入口覆盖复杂领域主流程", "断言与领域文案和关系边界同步"}, CompletionCriteria: []string{"Widget 测试能覆盖低库存概览和列表筛选主流程", "库存单与明细项断言与当前领域语义一致"}, RiskNotes: []string{"测试不能退回 generic 单记录 smoke test"}, AllocationTransition: relationRichTransition("task-create-test", []string{"ac-navigation", "ac-filter", "ac-line-item-binding"}, allSurfaces, boardEntityRefs, relationRefs, wiringShared, []string{"test/widget_test.dart"}, []string{taskBindAppEntryID}, []string{"库存 relation-rich Widget 测试已创建", "主流程断言与复杂领域语义同步"})},
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

func extractRequirementHighlights(requirementText string, fallback []string) []string {
	lines := strings.Split(requirementText, "\n")
	highlights := make([]string, 0, len(fallback))
	inHighlights := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "## ") {
			inHighlights = strings.Contains(trimmed, "需求摘录")
			continue
		}
		if inHighlights && strings.HasPrefix(trimmed, "- ") {
			highlights = append(highlights, strings.TrimSpace(strings.TrimPrefix(trimmed, "- ")))
		}
	}
	if len(highlights) > 0 {
		return highlights
	}
	if len(fallback) > 0 {
		return append([]string(nil), fallback...)
	}
	return []string{strings.TrimSpace(requirementText)}
}

func detectGenericDomainSignals(requirementText string) genericDomainSignals {
	raw := strings.TrimSpace(requirementText)
	lower := strings.ToLower(raw)
	if catalog, err := loadGenericDomainProfileCatalog(); err == nil {
		for _, profile := range catalog.Profiles {
			if matchesGenericDomainProfile(profile, raw, lower) {
				return profile.Signals
			}
		}
	}
	return defaultGenericDomainSignals()
}

func extractGenericRequirementOverlay(request Request, requirementText string, signals genericDomainSignals) genericRequirementOverlay {
	title := strings.TrimSpace(request.TitleHint)
	if title == "" {
		title = signals.AppTitle
	}
	return genericRequirementOverlay{
		ResolvedTitle:         title,
		RequirementHighlights: extractRequirementHighlights(requirementText, signals.Highlights),
	}
}

func buildGenericTaskBundle(signals genericDomainSignals) []appruns.TaskBundleItem {
	return buildGenericTaskBundleForTopology(signals, defaultGenericTopologyPlan(signals))
}

func defaultGenericTopologyPlan(signals genericDomainSignals) genericTopologyPlan {
	return normalizeGenericTopologyPlan(genericTopologyPlan{
		Overview:   genericSignalsDeclareOverview(signals),
		Collection: genericSignalsDeclareCollection(signals),
		Mutation:   genericSignalsDeclareMutation(signals),
		Inspection: genericSignalsDeclareInspection(signals),
		Delete:     genericSignalsDeclareDelete(signals),
		Filter:     genericSignalsDeclareFilter(signals),
	})
}

func (plan genericTopologyPlan) isDefault(signals genericDomainSignals) bool {
	return plan == defaultGenericTopologyPlan(signals)
}

func detectGenericTopology(requirementText string, signals genericDomainSignals) genericTopologyPlan {
	plan := defaultGenericTopologyPlan(signals)
	raw := strings.TrimSpace(requirementText)
	lower := strings.ToLower(raw)

	disableOverview := containsGenericTopologyCue(raw, lower, "无首页", "无首页摘要", "不需要首页", "不需要首页摘要", "无需首页", "无需首页摘要", "不要首页", "无概览", "不需要概览", "无需概览", "不需要摘要", "无需摘要", "直接进入列表", "直接展示列表")
	disableCollection := containsGenericTopologyCue(raw, lower, "无列表", "不需要列表", "无需列表", "不要列表", "无集合浏览", "不需要集合浏览")
	disableMutation := containsGenericTopologyCue(raw, lower, "无录入", "不需要录入", "无需录入", "不要录入", "不需要新建", "无需新建", "只读", "read-only")
	disableInspection := containsGenericTopologyCue(raw, lower, "无详情", "无详情页", "不需要详情", "不需要详情页", "无需详情", "无需详情页", "不要详情", "不做详情", "列表内编辑", "direct edit without detail")
	disableDelete := containsGenericTopologyCue(raw, lower, "不需要删除", "无需删除", "不要删除", "不做删除", "禁止删除", "只允许新增")
	disableFilter := containsGenericTopologyCue(raw, lower, "不需要筛选", "无需筛选", "不要筛选", "不做筛选", "无筛选")
	enableOverview := containsGenericTopologyCue(raw, lower, "首页", "概览", "摘要", "看板", "home", "overview", "summary", "dashboard", "趋势")
	enableCollection := containsGenericTopologyCue(raw, lower, "列表", "清单", "浏览", "历史", "list", "browse", "history", "feed")
	enableMutation := containsGenericTopologyCue(raw, lower, "新建", "新增", "创建", "录入", "填写", "编辑", "表单", "抽屉", "底部抽屉", "弹窗", "sheet", "drawer", "form", "create", "add", "edit", "打卡")
	enableInspection := containsGenericTopologyCue(raw, lower, "详情", "详情页", "明细", "detail", "inspect", "详情入口")
	enableDelete := containsGenericTopologyCue(raw, lower, "删除", "移除", "清除", "delete", "remove")
	enableFilter := signals.HasFilter && containsGenericTopologyCue(raw, lower, "筛选", "过滤", "按状态", "按分类", "按优先级", "filter", "status")

	if disableOverview {
		plan.Overview = false
	} else if enableOverview {
		plan.Overview = true
	}
	if disableCollection {
		plan.Collection = false
	} else if enableCollection {
		plan.Collection = true
	}
	if disableMutation {
		plan.Mutation = false
	} else if enableMutation {
		plan.Mutation = true
	}
	if disableInspection {
		plan.Inspection = false
		plan.Delete = false
	} else if enableInspection {
		plan.Inspection = true
	}
	if disableDelete || disableInspection {
		plan.Delete = false
	} else if enableDelete {
		plan.Delete = true
	}
	if disableFilter {
		plan.Filter = false
	} else if enableFilter {
		plan.Filter = true
	}

	return normalizeGenericTopologyPlan(plan)
}

func containsGenericTopologyCue(raw, lower string, keywords ...string) bool {
	for _, keyword := range keywords {
		trimmed := strings.TrimSpace(keyword)
		if trimmed == "" {
			continue
		}
		if strings.Contains(raw, trimmed) || strings.Contains(lower, strings.ToLower(trimmed)) {
			return true
		}
	}
	return false
}

func normalizeGenericTopologyPlan(plan genericTopologyPlan) genericTopologyPlan {
	if plan.Delete {
		plan.Inspection = true
	}
	if plan.Inspection {
		plan.Collection = true
	}
	if !plan.Overview && !plan.Collection && !plan.Mutation {
		plan.Collection = true
	}
	return plan
}

func genericSignalsDeclareOverview(signals genericDomainSignals) bool {
	return strings.TrimSpace(signals.OverviewFeatureTitle) != "" && strings.TrimSpace(signals.OverviewAcceptance) != "" && genericEntityHasFields(signals.SummaryEntity)
}

func genericSignalsDeclareCollection(signals genericDomainSignals) bool {
	return strings.TrimSpace(signals.CollectionFeatureTitle) != "" && strings.TrimSpace(signals.ListAcceptance) != "" && genericEntityHasFields(signals.Entity)
}

func genericSignalsDeclareMutation(signals genericDomainSignals) bool {
	return strings.TrimSpace(signals.MutationFeatureTitle) != "" && strings.TrimSpace(signals.FormAcceptance) != ""
}

func genericSignalsDeclareInspection(signals genericDomainSignals) bool {
	return strings.TrimSpace(signals.InspectTitle) != "" && strings.TrimSpace(signals.DetailFieldsExpected) != ""
}

func genericSignalsDeclareDelete(signals genericDomainSignals) bool {
	return genericSignalsDeclareInspection(signals) && strings.TrimSpace(signals.DeleteFeatureTitle) != "" && strings.TrimSpace(signals.DeleteAcceptance) != ""
}

func genericSignalsDeclareFilter(signals genericDomainSignals) bool {
	return signals.HasFilter && strings.TrimSpace(signals.FilterTitle) != "" && strings.TrimSpace(signals.FilterExpected) != ""
}

func genericEntityHasFields(entity DataEntity) bool {
	return strings.TrimSpace(entity.EntityID) != "" && len(entity.Fields) > 0
}

func buildGenericTaskBundleForTopology(signals genericDomainSignals, topology genericTopologyPlan) []appruns.TaskBundleItem {
	summaryEntityID := ""
	if topology.Overview {
		summaryEntityID = signals.SummaryEntity.EntityID
	}
	surfaceRefs := newGenericSurfaceRefSet(topology, signals.Entity.EntityID, summaryEntityID)
	listControllerTitle := "创建集合浏览控制器"
	if topology.Filter {
		listControllerTitle = "创建集合浏览与筛选控制器"
	}
	listControllerObjective := buildGenericListControllerObjective(topology)
	listControllerEvidence := []string{"集合浏览入口可浏览领域记录"}
	collectionSurfaceCriteria := []string{"集合浏览承载单元可浏览领域记录"}
	collectionSurfaceEvidence := []string{"集合浏览承载单元已绑定", "集合结果可浏览领域记录"}
	if topology.Inspection {
		listControllerEvidence = append(listControllerEvidence, "结果检查入口与集合数据保持一致")
		collectionSurfaceCriteria = append(collectionSurfaceCriteria, "结果检查入口与集合数据保持一致")
		collectionSurfaceEvidence = append(collectionSurfaceEvidence, "结果检查入口与集合数据保持一致")
	}
	if topology.Filter {
		listControllerEvidence = append(listControllerEvidence, "集合浏览入口可按领域状态筛选记录")
		collectionSurfaceCriteria = append(collectionSurfaceCriteria, "集合结果可按领域状态筛选")
		collectionSurfaceEvidence = append(collectionSurfaceEvidence, "集合结果可按领域状态筛选")
	}
	tasks := make([]appruns.TaskBundleItem, 0, 15)
	tasks = append(tasks, appruns.TaskBundleItem{TaskID: "task-create-record-model", Title: "创建领域记录模型", Category: appruns.TaskCategoryDomain, TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, RouteHint: appruns.TaskRouteHintStrongModel, RiskLevel: appruns.TaskRiskLevelHigh, Objective: "根据 domain model JSON 创建最终的领域记录实体，替代默认 generic record 语义。", Priority: "p0", TargetPaths: []string{"lib/models/record.dart"}, CompletionCriteria: []string{"领域记录字段已明确", "导出的模型类型可被控制器和页面直接消费"}, AllocationTransition: genericSurfaceTransition("task-create-record-model", compactRefs(genericConditionalRef(topology.Collection, "ac-list"), genericConditionalRef(topology.Mutation, "ac-form"), genericConditionalRef(topology.Inspection, "ac-detail"), "mrp-entity-fit", "check-profile-open-lite-domain-language"), surfaceRefs.AllSurfaces, surfaceRefs.PrimaryEntityRefs, []string{"lib/models/record.dart"}, nil, []string{"领域记录模型已创建", "不再保留默认 generic record 字段"})})
	if topology.Overview {
		tasks = append(tasks, appruns.TaskBundleItem{TaskID: "task-create-summary-model", Title: "创建首页摘要模型", Category: appruns.TaskCategorySummary, TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, RouteHint: appruns.TaskRouteHintStrongModel, RiskLevel: appruns.TaskRiskLevelHigh, Objective: "根据领域记录模型创建首页摘要模型，让首页指标直接承接领域语义。", Priority: "p0", Dependencies: []string{"task-create-record-model"}, TargetPaths: []string{"lib/models/dashboard_summary.dart"}, CompletionCriteria: []string{"摘要字段已明确", "摘要模型可由领域记录推导"}, AllocationTransition: genericSurfaceTransition("task-create-summary-model", []string{"ac-overview", "mrp-summary-fit", "check-profile-open-lite-domain-language"}, surfaceRefs.OverviewSurfaces, surfaceRefs.SummaryEntityRefs, []string{"lib/models/dashboard_summary.dart"}, []string{"task-create-record-model"}, []string{"首页摘要模型已创建", "摘要字段与领域关键指标保持一致"})})
	}
	tasks = append(tasks, appruns.TaskBundleItem{TaskID: "task-create-repository", Title: "创建本地仓储实现", Category: appruns.TaskCategoryStorage, TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, Objective: "固定本地持久化边界，为后续控制器和页面提供稳定的记录读写入口。", Priority: "p0", Dependencies: []string{"task-create-record-model"}, TargetPaths: []string{"lib/repositories/record_repository.dart"}, CompletionCriteria: []string{"本地持久化实现明确", "记录读写入口稳定"}, AllocationTransition: genericSurfaceTransition("task-create-repository", []string{"ac-persistence", "check-open-lite-local-persistence-wiring"}, surfaceRefs.AllSurfaces, surfaceRefs.SummaryEntityRefs, []string{"lib/repositories/record_repository.dart"}, []string{"task-create-record-model"}, []string{"仓储实现已创建", "领域记录读写入口稳定"})})
	if topology.Overview {
		tasks = append(tasks, appruns.TaskBundleItem{TaskID: "task-create-home-controller", Title: "创建概览承载控制器", Category: appruns.TaskCategorySummary, TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, Objective: "围绕领域记录与摘要模型创建概览承载控制器，承接概览入口与最近记录读取。", Priority: "p0", Dependencies: []string{"task-create-repository", "task-create-summary-model"}, TargetPaths: []string{"lib/controllers/home_controller.dart"}, CompletionCriteria: []string{"概览承载控制器可读取摘要与最近记录", "概览摘要字段与领域语义一致"}, AllocationTransition: genericSurfaceTransition("task-create-home-controller", []string{"ac-overview", "check-profile-open-lite-domain-language"}, surfaceRefs.OverviewSurfaces, surfaceRefs.SummaryEntityRefs, []string{"lib/controllers/home_controller.dart"}, []string{"task-create-repository", "task-create-summary-model"}, []string{"概览承载控制器已创建", "概览入口可消费领域摘要模型"})})
	}
	if topology.Mutation {
		tasks = append(tasks, appruns.TaskBundleItem{TaskID: "task-create-form-controller", Title: "创建实体变更控制器", Category: appruns.TaskCategoryFlow, TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, Objective: "围绕领域记录创建实体变更控制器，负责输入状态、校验和保存动作。", Priority: "p0", Dependencies: []string{"task-create-repository", "task-create-record-model"}, TargetPaths: []string{"lib/controllers/record_form_controller.dart"}, CompletionCriteria: []string{"实体变更状态与领域字段一致", "保存动作可写入仓储"}, AllocationTransition: genericSurfaceTransition("task-create-form-controller", compactRefs("ac-form", "check-open-lite-record-flow-wiring", "check-profile-open-lite-domain-language"), surfaceRefs.MutationSurfaces, surfaceRefs.PrimaryEntityRefs, []string{"lib/controllers/record_form_controller.dart"}, []string{"task-create-repository", "task-create-record-model"}, []string{"实体变更控制器已创建", "新增与编辑动作围绕领域字段收口"})})
	}
	if topology.Collection || topology.Inspection {
		tasks = append(tasks, appruns.TaskBundleItem{TaskID: "task-create-list-controller", Title: listControllerTitle, Category: appruns.TaskCategoryFlow, TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, Objective: listControllerObjective, Priority: "p0", Dependencies: []string{"task-create-repository", "task-create-record-model"}, TargetPaths: []string{"lib/controllers/record_list_controller.dart"}, CompletionCriteria: append([]string{"集合浏览控制器可提供领域记录集合"}, listControllerEvidence...), AllocationTransition: genericSurfaceTransition("task-create-list-controller", compactRefs("ac-list", genericConditionalRef(topology.Inspection, "ac-detail"), "check-profile-open-lite-domain-language"), surfaceRefs.CollectionInspectionSurfaces, surfaceRefs.PrimaryEntityRefs, []string{"lib/controllers/record_list_controller.dart"}, []string{"task-create-repository", "task-create-record-model"}, append([]string{"集合浏览控制器已创建"}, listControllerEvidence...))})
	}
	tasks = append(tasks,
		appruns.TaskBundleItem{TaskID: "task-create-copy", Title: "创建领域文案投影", Category: appruns.TaskCategoryContent, TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, RouteHint: appruns.TaskRouteHintStrongModel, RiskLevel: appruns.TaskRiskLevelHigh, Objective: "基于参考模板创建 open_lite_copy.dart，使页面标题、标签和空态文案直接映射到领域语义。", Priority: "p0", Dependencies: []string{"task-create-record-model"}, TargetPaths: []string{"lib/template/open_lite_copy.dart"}, CompletionCriteria: []string{"页面标题和表单标签体现领域命名", "默认 seed 文案不再残留"}, AllocationTransition: genericSurfaceTransition("task-create-copy", []string{"mrp-domain-wording", "check-profile-open-lite-domain-branding", "check-profile-open-lite-domain-language"}, surfaceRefs.AllSurfaces, surfaceRefs.SummaryEntityRefs, []string{"lib/template/open_lite_copy.dart"}, []string{"task-create-record-model"}, []string{"领域文案投影文件已创建", "默认 seed 文案已被领域文案替换"})},
		appruns.TaskBundleItem{TaskID: "task-create-android-branding", Title: "创建 Android 启动器文案", Category: appruns.TaskCategoryContent, TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, RouteHint: appruns.TaskRouteHintDeterministic, Objective: "在 strings.xml 中创建 Android 启动器名称与品牌文案，确保不再残留默认 seed branding。", Priority: "p0", Dependencies: []string{"task-create-copy"}, TargetPaths: []string{"android/app/src/main/res/values/strings.xml"}, CompletionCriteria: []string{"Android 启动器名称已切换到领域文案", "默认 seed branding 不再残留"}, AllocationTransition: genericSurfaceTransition("task-create-android-branding", []string{"mrp-domain-wording", "check-profile-open-lite-domain-branding"}, surfaceRefs.AllSurfaces, surfaceRefs.PrimaryEntityRefs, []string{"android/app/src/main/res/values/strings.xml"}, []string{"task-create-copy"}, []string{"Android 启动器名称已创建", "领域品牌文案已同步到 strings.xml"})},
		appruns.TaskBundleItem{TaskID: "task-create-android-build-config", Title: "创建 Android 构建文案入口", Category: appruns.TaskCategoryContent, TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, RouteHint: appruns.TaskRouteHintDeterministic, Objective: "在 build.gradle.kts 中保留 Android branding 的单点覆盖入口，避免品牌改动分散到其他文件。", Priority: "p0", Dependencies: []string{"task-create-android-branding"}, TargetPaths: []string{"android/app/build.gradle.kts"}, CompletionCriteria: []string{"Android branding 单点覆盖入口存在", "applicationId 或 launcher branding 的改动路径明确"}, AllocationTransition: genericSurfaceTransition("task-create-android-build-config", []string{"check-profile-open-lite-domain-branding"}, surfaceRefs.AllSurfaces, surfaceRefs.PrimaryEntityRefs, []string{"android/app/build.gradle.kts"}, []string{"task-create-android-branding"}, []string{"Android branding 构建入口已创建", "build.gradle.kts 保留单点覆盖能力"})},
	)
	if topology.Overview {
		tasks = append(tasks, appruns.TaskBundleItem{TaskID: taskBindOverviewSurfaceID, Title: "绑定概览承载单元", Category: appruns.TaskCategoryScreen, TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, Objective: "根据概览承载控制器和领域文案绑定概览承载单元，承接领域摘要与最近记录入口。", Priority: "p0", Dependencies: []string{"task-create-home-controller", "task-create-copy"}, TargetPaths: []string{"lib/views/home_page.dart"}, CompletionCriteria: []string{"概览承载单元可渲染领域摘要", "承载单元标题与领域语义一致"}, AllocationTransition: genericSurfaceTransition(taskBindOverviewSurfaceID, []string{"ac-overview", "mrp-domain-wording"}, surfaceRefs.OverviewSurfaces, surfaceRefs.SummaryEntityRefs, []string{"lib/views/home_page.dart"}, []string{"task-create-home-controller", "task-create-copy"}, []string{"概览承载单元已绑定", "领域摘要与最近记录入口已接入"})})
	}
	if topology.Collection {
		tasks = append(tasks, appruns.TaskBundleItem{TaskID: taskBindCollectionSurfaceID, Title: "绑定集合浏览承载单元", Category: appruns.TaskCategoryScreen, TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, Objective: buildGenericCollectionSurfaceObjective(signals, topology), Priority: "p0", Dependencies: compactRefs("task-create-list-controller", "task-create-copy"), TargetPaths: []string{"lib/views/record_list_page.dart"}, CompletionCriteria: collectionSurfaceCriteria, AllocationTransition: genericSurfaceTransition(taskBindCollectionSurfaceID, compactRefs("ac-list", genericConditionalRef(topology.Inspection, "ac-detail")), surfaceRefs.CollectionSurfaces, surfaceRefs.PrimaryEntityRefs, []string{"lib/views/record_list_page.dart"}, compactRefs("task-create-list-controller", "task-create-copy"), collectionSurfaceEvidence)})
	}
	if topology.Mutation {
		tasks = append(tasks, appruns.TaskBundleItem{TaskID: taskBindMutationSurfaceID, Title: "绑定实体变更承载单元", Category: appruns.TaskCategoryScreen, TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, Objective: "根据实体变更控制器和领域文案绑定实体变更承载单元，承接领域字段输入。", Priority: "p0", Dependencies: []string{"task-create-form-controller", "task-create-copy"}, TargetPaths: []string{"lib/views/record_form_page.dart"}, CompletionCriteria: []string{"实体变更承载单元字段与领域记录一致", "保存动作已接线到控制器"}, AllocationTransition: genericSurfaceTransition(taskBindMutationSurfaceID, []string{"ac-form", "mrp-domain-wording"}, surfaceRefs.MutationSurfaces, surfaceRefs.PrimaryEntityRefs, []string{"lib/views/record_form_page.dart"}, []string{"task-create-form-controller", "task-create-copy"}, []string{"实体变更承载单元已绑定", "字段输入链路已接线到控制器"})})
	}
	if topology.Inspection {
		inspectionCriteria := []string{"结果检查承载单元可展示单条领域记录"}
		switch {
		case topology.Mutation && topology.Delete:
			inspectionCriteria = append(inspectionCriteria, "编辑和删除入口已预留")
		case topology.Delete:
			inspectionCriteria = append(inspectionCriteria, "删除入口已预留")
		case topology.Mutation:
			inspectionCriteria = append(inspectionCriteria, "编辑入口已预留")
		}
		tasks = append(tasks, appruns.TaskBundleItem{TaskID: taskBindInspectionSurfaceID, Title: "绑定结果检查承载单元", Category: appruns.TaskCategoryScreen, TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, Objective: "根据领域记录模型和文案绑定结果检查承载单元，承接单条记录展示、编辑和删除入口。", Priority: "p0", Dependencies: []string{"task-create-record-model", "task-create-copy"}, TargetPaths: []string{"lib/views/record_detail_page.dart"}, CompletionCriteria: inspectionCriteria, AllocationTransition: genericSurfaceTransition(taskBindInspectionSurfaceID, compactRefs("ac-detail", genericConditionalRef(topology.Delete, "ac-delete"), "mrp-domain-wording"), surfaceRefs.InspectionSurfaces, surfaceRefs.PrimaryEntityRefs, []string{"lib/views/record_detail_page.dart"}, []string{"task-create-record-model", "task-create-copy"}, []string{"结果检查承载单元已绑定", "结果检查可围绕单条领域记录展示"})})
	}
	appEntryDependencies := compactRefs(
		genericConditionalRef(topology.Overview, taskBindOverviewSurfaceID),
		genericConditionalRef(topology.Collection, taskBindCollectionSurfaceID),
		genericConditionalRef(topology.Mutation, taskBindMutationSurfaceID),
		genericConditionalRef(topology.Inspection, taskBindInspectionSurfaceID),
		genericConditionalRef(topology.Overview, "task-create-home-controller"),
		genericConditionalRef(topology.Collection || topology.Inspection, "task-create-list-controller"),
		genericConditionalRef(topology.Mutation, "task-create-form-controller"),
		"task-create-repository",
	)
	appEntryCompletionCriteria := []string{"应用入口完成接线"}
	appEntrySuccessEvidence := []string{"应用入口已接线"}
	navigationSummary := strings.Join(buildGenericNavigationSurfaceLabels(topology), "、")
	if navigationSummary != "" {
		appEntryCompletionCriteria = append(appEntryCompletionCriteria, navigationSummary+"承载单元导航可达")
		if topology.isDefault(signals) {
			appEntrySuccessEvidence = append(appEntrySuccessEvidence, "核心承载单元导航骨架可用")
		} else {
			appEntrySuccessEvidence = append(appEntrySuccessEvidence, navigationSummary+"承载单元导航骨架可用")
		}
	} else {
		appEntryCompletionCriteria = append(appEntryCompletionCriteria, "核心交互入口已可达")
		appEntrySuccessEvidence = append(appEntrySuccessEvidence, "核心交互入口已可用")
	}
	tasks = append(tasks, appruns.TaskBundleItem{TaskID: taskBindAppEntryID, Title: "接线应用入口与承载单元路由", Category: appruns.TaskCategoryScreen, TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, Objective: "把已创建的控制器、承载单元和仓储接线到应用入口，形成稳定的中性交互骨架。", Priority: "p0", Dependencies: appEntryDependencies, TargetPaths: []string{"lib/main.dart"}, CompletionCriteria: appEntryCompletionCriteria, AllocationTransition: genericSurfaceTransition(taskBindAppEntryID, []string{"ac-navigation", "check-generic-open-scope"}, surfaceRefs.AllSurfaces, surfaceRefs.SummaryEntityRefs, []string{"lib/main.dart"}, appEntryDependencies, appEntrySuccessEvidence)})
	tasks = append(tasks, appruns.TaskBundleItem{TaskID: "task-create-test", Title: "创建最小 Widget 测试", Category: appruns.TaskCategoryFlow, TaskType: appruns.BuilderRuntimeTaskTypeDualFileWiring, Objective: "为当前领域承载单元和主流程创建最小 widget 测试，确保后续 analyze/test 收口有稳定入口。", Priority: "p0", Dependencies: []string{taskBindAppEntryID}, TargetPaths: []string{"test/widget_test.dart"}, CompletionCriteria: []string{"test/widget_test.dart 已同步当前领域文案", "测试入口可覆盖主承载单元或主流程"}, AllocationTransition: genericSurfaceTransition("task-create-test", compactRefs("ac-navigation", genericConditionalRef(topology.Mutation, "ac-form"), genericConditionalRef(topology.Inspection, "ac-detail"), "check-open-lite-counter-demo-removed", genericConditionalRef(topology.Mutation, "check-open-lite-record-flow-wiring"), "check-profile-open-lite-domain-language"), surfaceRefs.AllSurfaces, surfaceRefs.SummaryEntityRefs, []string{"test/widget_test.dart"}, []string{taskBindAppEntryID}, []string{"最小 Widget 测试已创建", "测试断言与当前领域文案和主流程保持同步"})})
	return tasks
}

func buildGenericNonDefaultCoreScenarios(signals genericDomainSignals, topology genericTopologyPlan) []Scenario {
	scenarios := make([]Scenario, 0, 6)
	if topology.Overview {
		scenarios = append(scenarios, Scenario{ScenarioID: "scenario-overview", Title: signals.OverviewTitle, Summary: signals.OverviewSummary, PrimaryUserRefs: []string{"user-builder", "user-validator"}})
	}
	if topology.Mutation {
		scenarios = append(scenarios, Scenario{ScenarioID: "scenario-create-record", Title: signals.CreateTitle, Summary: signals.CreateSummary, PrimaryUserRefs: []string{"user-builder"}})
	}
	if topology.Mutation && topology.Inspection {
		scenarios = append(scenarios, Scenario{ScenarioID: "scenario-update-record", Title: signals.UpdateTitle, Summary: signals.UpdateSummary, PrimaryUserRefs: []string{"user-builder"}})
	}
	if topology.Delete {
		scenarios = append(scenarios, Scenario{ScenarioID: "scenario-delete-record", Title: signals.DeleteTitle, Summary: signals.DeleteSummary, PrimaryUserRefs: []string{"user-builder"}})
	}
	if topology.Inspection {
		scenarios = append(scenarios, Scenario{ScenarioID: "scenario-inspect-detail", Title: signals.InspectTitle, Summary: signals.InspectSummary, PrimaryUserRefs: []string{"user-builder"}})
	}
	if topology.Filter {
		scenarios = append(scenarios, Scenario{ScenarioID: "scenario-filter-records", Title: signals.FilterTitle, Summary: signals.FilterSummary, PrimaryUserRefs: []string{"user-builder", "user-validator"}})
	}
	if len(scenarios) == 0 && topology.Collection {
		scenarios = append(scenarios, Scenario{ScenarioID: "scenario-browse-records", Title: signals.CollectionFeatureTitle, Summary: signals.CollectionFeatureSummary, PrimaryUserRefs: []string{"user-builder", "user-validator"}})
	}
	return scenarios
}

func buildGenericNonDefaultFeatureList(signals genericDomainSignals, topology genericTopologyPlan) []Feature {
	features := make([]Feature, 0, 5)
	if topology.Overview {
		features = append(features, Feature{FeatureID: "feature-home-overview", Title: signals.OverviewFeatureTitle, Summary: signals.OverviewFeatureSummary, Priority: "p0", Required: true, RelatedScenarios: []string{"scenario-overview"}, RelatedSurfaceRefs: []string{genericSurfaceOverviewID}, AcceptanceRefs: []string{"ac-overview", "ac-navigation"}})
	}
	if topology.Collection {
		features = append(features, Feature{FeatureID: "feature-record-list", Title: signals.CollectionFeatureTitle, Summary: signals.CollectionFeatureSummary, Priority: "p0", Required: true, RelatedScenarios: compactRefs(genericConditionalRef(topology.Overview, "scenario-overview"), genericConditionalRef(topology.Inspection, "scenario-inspect-detail"), genericConditionalRef(topology.Filter, "scenario-filter-records")), RelatedSurfaceRefs: compactRefs(genericSurfaceCollectionID, genericConditionalRef(topology.Inspection, genericSurfaceInspectionID)), AcceptanceRefs: compactRefs("ac-list", genericConditionalRef(topology.Inspection, "ac-detail"))})
	}
	if topology.Mutation {
		features = append(features, Feature{FeatureID: "feature-record-form", Title: signals.MutationFeatureTitle, Summary: signals.MutationFeatureSummary, Priority: "p0", Required: true, RelatedScenarios: compactRefs("scenario-create-record", genericConditionalRef(topology.Inspection, "scenario-update-record")), RelatedSurfaceRefs: []string{genericSurfaceMutationID}, AcceptanceRefs: []string{"ac-form"}})
	}
	if topology.Delete {
		features = append(features, Feature{FeatureID: "feature-record-delete", Title: signals.DeleteFeatureTitle, Summary: signals.DeleteFeatureSummary, Priority: "p0", Required: true, RelatedScenarios: []string{"scenario-delete-record"}, RelatedSurfaceRefs: compactRefs(genericSurfaceInspectionID, genericSurfaceCollectionID), AcceptanceRefs: []string{"ac-delete"}})
	}
	storageSurfaceRefs := compactRefs(genericConditionalRef(topology.Overview, genericSurfaceOverviewID), genericConditionalRef(topology.Collection, genericSurfaceCollectionID), genericConditionalRef(topology.Inspection, genericSurfaceInspectionID))
	if len(storageSurfaceRefs) == 0 {
		storageSurfaceRefs = compactRefs(genericConditionalRef(topology.Mutation, genericSurfaceMutationID))
	}
	features = append(features, Feature{FeatureID: "feature-local-storage", Title: "本地持久化", Summary: signals.DomainLabel + " 数据本地持久化，应用重启后仍能恢复。", Priority: "p1", Required: true, RelatedScenarios: compactRefs(genericConditionalRef(topology.Mutation, "scenario-create-record"), genericConditionalRef(topology.Overview, "scenario-overview"), genericConditionalRef(topology.Mutation && topology.Inspection, "scenario-update-record"), genericConditionalRef(topology.Delete, "scenario-delete-record")), RelatedSurfaceRefs: storageSurfaceRefs, AcceptanceRefs: []string{"ac-persistence"}})
	return features
}

func buildGenericNonDefaultUserFlows(signals genericDomainSignals, topology genericTopologyPlan) []UserFlow {
	flows := make([]UserFlow, 0, 5)
	actor := signals.DomainLabel + " 应用使用者"
	entrySurfaceRef := genericCreateEntrySurfaceRef(topology)
	returnSurfaceRef := genericCreateReturnSurfaceRef(topology)
	if topology.Mutation {
		createSteps := []FlowStep{{StepID: "step-open-form", Title: genericCreateEntryTitle(entrySurfaceRef), SurfaceRef: entrySurfaceRef, Actor: actor, ExpectedResult: signals.CreateFieldsExpected}, {StepID: "step-fill-form", Title: "填写表单并保存", SurfaceRef: genericSurfaceMutationID, Actor: actor, ExpectedResult: "表单校验通过并保存成功"}}
		if returnSurfaceRef != "" {
			returnStepID := "step-return-primary"
			if topology.isDefault(signals) && returnSurfaceRef == genericSurfaceOverviewID {
				returnStepID = "step-return-overview"
			}
			createSteps = append(createSteps, FlowStep{StepID: returnStepID, Title: genericCreateReturnTitle(returnSurfaceRef), SurfaceRef: returnSurfaceRef, Actor: actor, ExpectedResult: genericCreateReturnExpected(returnSurfaceRef)})
		}
		flows = append(flows, UserFlow{FlowID: "flow-create-record", Title: signals.CreateTitle, Steps: createSteps})
	}
	if topology.Mutation && topology.Inspection {
		updateInspectionTitle := "从集合浏览入口进入结果检查入口"
		updateInspectionSurfaceRef := genericSurfaceCollectionID
		if topology.isDefault(signals) {
			updateInspectionTitle = "从概览或集合浏览入口进入结果检查入口"
			updateInspectionSurfaceRef = genericSurfaceInspectionID
		}
		flows = append(flows, UserFlow{FlowID: "flow-update-record", Title: signals.UpdateTitle, Steps: []FlowStep{{StepID: "step-open-inspection", Title: updateInspectionTitle, SurfaceRef: updateInspectionSurfaceRef, Actor: actor, ExpectedResult: "能看到单条记录详情与编辑入口"}, {StepID: "step-open-mutation", Title: "进入实体变更入口并修改字段", SurfaceRef: genericSurfaceMutationID, Actor: actor, ExpectedResult: signals.UpdateFieldsExpected}, {StepID: "step-refresh-inspection", Title: "保存后返回结果检查入口", SurfaceRef: genericSurfaceInspectionID, Actor: actor, ExpectedResult: "详情页显示最新内容"}}})
	}
	if topology.Delete {
		deleteResult := "集合浏览入口不再显示该记录"
		if topology.Overview {
			deleteResult = "概览或集合浏览入口不再显示该记录"
		}
		flows = append(flows, UserFlow{FlowID: "flow-delete-record", Title: signals.DeleteTitle, Steps: []FlowStep{{StepID: "step-open-delete", Title: "在结果检查入口触发删除动作", SurfaceRef: genericSurfaceInspectionID, Actor: actor, ExpectedResult: "看到删除确认提示"}, {StepID: "step-confirm-delete", Title: "确认删除并返回上一层", SurfaceRef: genericSurfaceInspectionID, Actor: actor, ExpectedResult: deleteResult}}})
	}
	if topology.Inspection {
		inspectSteps := make([]FlowStep, 0, 2)
		if topology.Overview {
			inspectSteps = append(inspectSteps, FlowStep{StepID: "step-open-collection", Title: "从概览入口进入集合浏览入口", SurfaceRef: genericSurfaceOverviewID, Actor: actor, ExpectedResult: "能看到完整记录列表"})
		}
		inspectSteps = append(inspectSteps, FlowStep{StepID: "step-open-inspection", Title: "点击一条记录进入结果检查入口", SurfaceRef: genericSurfaceCollectionID, Actor: actor, ExpectedResult: signals.DetailFieldsExpected})
		flows = append(flows, UserFlow{FlowID: "flow-inspect-record", Title: signals.InspectTitle, Steps: inspectSteps})
	}
	if topology.Filter && topology.Collection {
		filterSteps := make([]FlowStep, 0, 2)
		if topology.Overview {
			filterSteps = append(filterSteps, FlowStep{StepID: "step-open-collection", Title: "从概览入口进入集合浏览入口", SurfaceRef: genericSurfaceOverviewID, Actor: actor, ExpectedResult: "能看到筛选入口和完整记录列表"})
		}
		filterSteps = append(filterSteps, FlowStep{StepID: "step-switch-filter", Title: "切换筛选视图", SurfaceRef: genericSurfaceCollectionID, Actor: actor, ExpectedResult: signals.FilterExpected})
		flows = append(flows, UserFlow{FlowID: "flow-filter-record", Title: signals.FilterTitle, Steps: filterSteps})
	}
	return flows
}

func buildGenericNonDefaultDataEntities(signals genericDomainSignals, topology genericTopologyPlan) []DataEntity {
	entities := []DataEntity{signals.Entity}
	if topology.Overview {
		entities = append(entities, signals.SummaryEntity)
	}
	return entities
}

func buildGenericNonDefaultAcceptanceCriteria(signals genericDomainSignals, topology genericTopologyPlan) []AcceptanceCriterion {
	criteria := make([]AcceptanceCriterion, 0, 7)
	if topology.Overview {
		criteria = append(criteria, AcceptanceCriterion{CriterionID: "ac-overview", Label: signals.SummaryLabel + "完整", Category: "functional", Required: true, Description: signals.OverviewAcceptance})
	}
	if topology.Collection {
		criteria = append(criteria, AcceptanceCriterion{CriterionID: "ac-list", Label: signals.CollectionFeatureTitle + "可浏览", Category: "functional", Required: true, Description: signals.ListAcceptance})
	}
	if topology.Mutation {
		criteria = append(criteria, AcceptanceCriterion{CriterionID: "ac-form", Label: signals.MutationFeatureTitle + "链路完整", Category: "functional", Required: true, Description: signals.FormAcceptance})
	}
	if topology.Inspection {
		detailDescription := "从集合浏览入口至少存在一条进入结果检查入口的路径。"
		if topology.Overview {
			detailDescription = "从概览或集合浏览入口至少存在一条进入结果检查入口的路径。"
		}
		criteria = append(criteria, AcceptanceCriterion{CriterionID: "ac-detail", Label: "结果检查入口可达", Category: "functional", Required: true, Description: detailDescription})
	}
	if topology.Delete {
		criteria = append(criteria, AcceptanceCriterion{CriterionID: "ac-delete", Label: "删除链路完整", Category: "functional", Required: true, Description: signals.DeleteAcceptance})
	}
	criteria = append(criteria,
		AcceptanceCriterion{CriterionID: "ac-persistence", Label: "本地持久化可恢复", Category: "smoke", Required: true, Description: "应用重启后仍能读取本地记录数据。"},
		AcceptanceCriterion{CriterionID: "ac-navigation", Label: "核心承载单元可导航", Category: "ui", Required: true, Description: buildGenericNonDefaultNavigationAcceptance(signals, topology)},
	)
	return criteria
}

func buildGenericNonDefaultManualReviewPoints(signals genericDomainSignals, topology genericTopologyPlan) []ManualReviewPoint {
	points := []ManualReviewPoint{{PointID: "mrp-domain-wording", Summary: signals.ManualReviewSummary, Reason: signals.ManualReviewReason, Owner: "product"}, {PointID: "mrp-entity-fit", Summary: "确认领域实体能映射到 open-lite 中性交互承载骨架", Reason: buildGenericEntityFitReason(signals, topology), Owner: "engineering"}}
	if topology.Overview {
		points = append(points, ManualReviewPoint{PointID: "mrp-summary-fit", Summary: signals.SummaryReviewSummary, Reason: signals.SummaryReviewReason, Owner: "engineering"})
	}
	return points
}

func buildGenericCoreScenarios(signals genericDomainSignals, topology genericTopologyPlan) []Scenario {
	return buildGenericNonDefaultCoreScenarios(signals, topology)
}

func buildGenericFeatureList(signals genericDomainSignals, topology genericTopologyPlan) []Feature {
	return buildGenericNonDefaultFeatureList(signals, topology)
}

func buildGenericUserFlows(signals genericDomainSignals, topology genericTopologyPlan) []UserFlow {
	return buildGenericNonDefaultUserFlows(signals, topology)
}

func buildGenericDataEntities(signals genericDomainSignals, topology genericTopologyPlan) []DataEntity {
	return buildGenericNonDefaultDataEntities(signals, topology)
}

func buildGenericAcceptanceCriteria(signals genericDomainSignals, topology genericTopologyPlan) []AcceptanceCriterion {
	return buildGenericNonDefaultAcceptanceCriteria(signals, topology)
}

func buildGenericManualReviewPoints(signals genericDomainSignals, topology genericTopologyPlan) []ManualReviewPoint {
	return buildGenericNonDefaultManualReviewPoints(signals, topology)
}

func buildGenericAcceptanceChecks(signals genericDomainSignals, topology genericTopologyPlan, realBuild bool) []appruns.AcceptanceCheck {
	if realBuild {
		return defaultFlutterOpenLiteRealBuildChecks(signals, topology)
	}
	checks := []appruns.AcceptanceCheck{{CheckID: "check-context-ready", Label: "准备上下文文件", Stage: "baseline", Required: true, Commands: []string{"echo context-ready"}, SuccessCriteria: "上下文装载命令返回 0。", TimeoutSeconds: 30}, {CheckID: "check-generic-open-scope", Label: "确认通用模板能力边界", Stage: "baseline", Required: true, Commands: []string{buildGenericScopeCheckCommand(topology)}, SuccessCriteria: buildGenericScopeSuccessCriteria(signals, topology), TimeoutSeconds: 30}, {CheckID: "check-plan-ready", Label: "确认 Builder 输入包可执行", Stage: "cheap", Required: true, Commands: []string{"echo builder-input-ready"}, SuccessCriteria: "Builder 输入包与最小计划文件已生成。", TimeoutSeconds: 30}}
	return append(checks, defaultFlutterFallbackChecks()...)
}

func buildGenericGoalSummary(selectedTemplateLabel string, signals genericDomainSignals, topology genericTopologyPlan) string {
	if topology.isDefault(signals) {
		return fmt.Sprintf(signals.GoalSummary, selectedTemplateLabel)
	}
	return buildGenericNonDefaultGoalSummary(selectedTemplateLabel, signals, topology)
}

func buildGenericNonDefaultGoalSummary(selectedTemplateLabel string, signals genericDomainSignals, topology genericTopologyPlan) string {
	capabilities := append(buildGenericCapabilityLabels(topology), "本地持久化")
	return fmt.Sprintf("将%s需求整理成基于 %s 的中性交互承载 Android MVP 输入包，围绕%s形成当前最小闭环。", signals.DomainLabel, selectedTemplateLabel, joinChineseItems(capabilities))
}

func buildGenericRequiredCapabilities(signals genericDomainSignals, topology genericTopologyPlan) []string {
	if topology.isDefault(signals) {
		capabilities := []string{"navigation"}
		if topology.Overview {
			capabilities = append(capabilities, "summary-card")
		}
		if topology.Collection {
			capabilities = append(capabilities, "list")
		}
		if topology.Mutation {
			capabilities = append(capabilities, "form")
		}
		if topology.Inspection {
			capabilities = append(capabilities, "detail")
		}
		return append(capabilities, "local-storage", "theme")
	}
	capabilities := []string{"navigation", "local-storage", "theme"}
	if topology.Overview {
		capabilities = append(capabilities, "summary-card")
	}
	if topology.Collection {
		capabilities = append(capabilities, "list")
	}
	if topology.Mutation {
		capabilities = append(capabilities, "form")
	}
	if topology.Inspection {
		capabilities = append(capabilities, "detail")
	}
	return capabilities
}

func buildGenericTemplateFitReasons(signals genericDomainSignals, topology genericTopologyPlan) []string {
	if topology.isDefault(signals) {
		return []string{
			"flutter-open-lite 已覆盖概览、集合浏览、实体变更、结果检查和本地持久化所需的最小能力集合。",
			"当前长期技术基线已冻结为 Flutter，且 generic 模板明确不依赖自建服务器。",
			"该模板已进入 builder 镜像内的 analyze、test 与 debug APK build structural checks，可直接作为 generic real-check 的执行起点。",
		}
	}
	return buildGenericNonDefaultTemplateFitReasons(topology)
}

func buildGenericNonDefaultTemplateFitReasons(topology genericTopologyPlan) []string {
	labels := buildGenericNavigationSurfaceLabels(topology)
	capabilitySummary := "本地持久化"
	if len(labels) > 0 {
		capabilitySummary = joinChineseItems(labels) + "与本地持久化"
	}
	return []string{
		"flutter-open-lite 已覆盖当前需求声明的" + capabilitySummary + "所需的最小能力集合。",
		"当前长期技术基线已冻结为 Flutter，且 generic 模板明确不依赖自建服务器。",
		"该模板已进入 builder 镜像内的 analyze、test 与 debug APK build structural checks，可直接作为 generic real-check 的执行起点。",
	}
}

func buildGenericImplementationPhases(signals genericDomainSignals, topology genericTopologyPlan) []string {
	if topology.isDefault(signals) {
		return append([]string(nil), signals.ImplementationPhases...)
	}
	return buildGenericNonDefaultImplementationPhases(signals, topology)
}

func buildGenericNonDefaultImplementationPhases(signals genericDomainSignals, topology genericTopologyPlan) []string {
	phaseOneTargets := []string{signals.Entity.Name}
	if topology.Overview {
		phaseOneTargets = append(phaseOneTargets, signals.SummaryEntity.Name)
	}
	phases := []string{fmt.Sprintf("阶段 1：冻结%s和本地仓储边界。", joinChineseItems(phaseOneTargets))}
	navigationLabels := buildGenericNavigationSurfaceLabels(topology)
	if len(navigationLabels) > 0 {
		phases = append(phases, fmt.Sprintf("阶段 2：搭建%s承载单元并接通最小导航。", joinChineseItems(navigationLabels)))
	}
	closureTargets := make([]string, 0, 5)
	if topology.Mutation {
		closureTargets = append(closureTargets, "新增录入")
	}
	if topology.Collection {
		closureTargets = append(closureTargets, "集合浏览")
	}
	if topology.Inspection {
		closureTargets = append(closureTargets, "结果检查")
	}
	if topology.Delete {
		closureTargets = append(closureTargets, "删除收口")
	}
	if topology.Filter {
		closureTargets = append(closureTargets, "状态筛选")
	}
	if len(closureTargets) == 0 {
		closureTargets = append(closureTargets, "最小可用流程")
	}
	phases = append(phases, fmt.Sprintf("阶段 3：补齐%s主流程，为后续真实 validate 链路预留稳定输入包。", joinChineseItems(closureTargets)))
	return phases
}

func buildGenericManualConstraints(signals genericDomainSignals, topology genericTopologyPlan) []string {
	constraints := append([]string(nil), signals.ManualConstraints...)
	if topology.isDefault(signals) {
		return constraints
	}
	return append(constraints, "当前需求未声明的承载单元不得被 compile 自动补齐。")
}

func buildGenericHumanNotes(signals genericDomainSignals, topology genericTopologyPlan) []map[string]string {
	notes := make([]map[string]string, 0, len(signals.HumanNotes)+1)
	for _, note := range signals.HumanNotes {
		if !genericHumanNoteMatchesTopology(note, topology) {
			continue
		}
		copied := make(map[string]string, len(note))
		for key, value := range note {
			copied[key] = value
		}
		notes = append(notes, copied)
	}
	if topology.isDefault(signals) {
		return notes
	}
	return append(notes, map[string]string{
		"note_id": "note-generic-topology-scope",
		"scope":   "engineering",
		"summary": buildGenericScopedHumanNote(topology),
	})
}

func genericHumanNoteMatchesTopology(note map[string]string, topology genericTopologyPlan) bool {
	summary := strings.TrimSpace(note["summary"])
	lower := strings.ToLower(summary)
	if !topology.Overview && containsGenericTopologyCue(summary, lower, "首页", "概览", "摘要", "home", "overview", "summary", "dashboard") {
		return false
	}
	if !topology.Collection && containsGenericTopologyCue(summary, lower, "列表", "清单", "集合浏览", "list", "collection", "history") {
		return false
	}
	if !topology.Mutation && containsGenericTopologyCue(summary, lower, "录入", "新建", "编辑", "表单", "抽屉", "drawer", "form", "create", "add", "edit") {
		return false
	}
	if !topology.Inspection && containsGenericTopologyCue(summary, lower, "详情", "结果检查", "detail", "inspect") {
		return false
	}
	if !topology.Delete && containsGenericTopologyCue(summary, lower, "删除", "delete", "remove") {
		return false
	}
	if !topology.Filter && containsGenericTopologyCue(summary, lower, "筛选", "过滤", "filter") {
		return false
	}
	return true
}

func buildGenericScopedHumanNote(topology genericTopologyPlan) string {
	labels := buildGenericNavigationSurfaceLabels(topology)
	if len(labels) == 0 {
		return "当前需求只围绕最小可用承载单元和本地持久化展开，未声明承载单元和流程不要自动补齐。"
	}
	return "当前需求只围绕" + joinChineseItems(labels) + "和本地持久化展开，未声明承载单元和流程不要自动补齐。"
}

func buildGenericNonDefaultNavigationAcceptance(signals genericDomainSignals, topology genericTopologyPlan) string {
	if topology.isDefault(signals) {
		return signals.NavigationAcceptance
	}
	labels := buildGenericNavigationSurfaceLabels(topology)
	if len(labels) == 0 {
		return signals.NavigationAcceptance
	}
	return "当前需求声明的" + joinChineseItems(labels) + "承载单元之间路径明确且可达。"
}

func buildGenericNavigationSurfaceLabels(topology genericTopologyPlan) []string {
	return compactRefs(
		genericConditionalRef(topology.Overview, "概览"),
		genericConditionalRef(topology.Collection, "集合浏览"),
		genericConditionalRef(topology.Mutation, "实体变更"),
		genericConditionalRef(topology.Inspection, "结果检查"),
	)
}

func buildGenericCapabilityLabels(topology genericTopologyPlan) []string {
	labels := buildGenericNavigationSurfaceLabels(topology)
	if topology.Filter {
		labels = append(labels, "状态筛选")
	}
	return compactRefs(labels...)
}

func buildGenericScopeCheckCommand(topology genericTopologyPlan) string {
	components := compactRefs(
		"generic-open-lite-scope",
		genericConditionalRef(topology.Overview, "overview"),
		genericConditionalRef(topology.Collection, "collection"),
		genericConditionalRef(topology.Mutation, "mutation"),
		genericConditionalRef(topology.Inspection, "inspection"),
	)
	return "echo " + strings.Join(components, "-")
}

func buildGenericScopeSuccessCriteria(signals genericDomainSignals, topology genericTopologyPlan) string {
	if topology.isDefault(signals) {
		return "能力边界已收敛到概览、集合浏览、实体变更、结果检查和本地持久化。"
	}
	labels := buildGenericNavigationSurfaceLabels(topology)
	if len(labels) == 0 {
		return "能力边界已收敛到最小可用承载单元和本地持久化，不再默认补齐未声明承载单元。"
	}
	return "能力边界已收敛到" + joinChineseItems(labels) + "和本地持久化，不再默认补齐未声明承载单元。"
}

func buildGenericListControllerObjective(topology genericTopologyPlan) string {
	parts := []string{"集合读取", "排序"}
	if topology.Filter {
		parts = append(parts, "筛选")
	}
	if topology.Inspection {
		parts = append(parts, "结果检查跳转")
	}
	return "围绕领域记录提供" + joinChineseItems(parts) + "所需的控制器接口。"
}

func buildGenericCollectionSurfaceObjective(signals genericDomainSignals, topology genericTopologyPlan) string {
	if topology.isDefault(signals) {
		return "根据集合浏览控制器和领域文案绑定集合浏览承载单元，提供记录浏览与结果定位入口。"
	}
	parts := []string{"记录浏览"}
	if topology.Filter {
		parts = append(parts, "筛选")
	}
	if topology.Inspection {
		parts = append(parts, "结果定位")
	}
	return "根据集合浏览控制器和领域文案绑定集合浏览承载单元，提供" + joinChineseItems(parts) + "入口。"
}

func buildGenericEntityFitReason(signals genericDomainSignals, topology genericTopologyPlan) string {
	if topology.isDefault(signals) {
		return "compile 后必须在复用概览、集合浏览、实体变更、结果检查承载单元的同时保留领域字段。"
	}
	labels := buildGenericNavigationSurfaceLabels(topology)
	if len(labels) == 0 {
		return "compile 后必须在当前最小中性交互承载骨架内保留领域字段。"
	}
	return "compile 后必须在复用" + joinChineseItems(labels) + "承载单元的同时保留领域字段。"
}

func joinChineseItems(items []string) string {
	filtered := compactRefs(items...)
	switch len(filtered) {
	case 0:
		return ""
	case 1:
		return filtered[0]
	case 2:
		return filtered[0] + "和" + filtered[1]
	default:
		return strings.Join(filtered[:len(filtered)-1], "、") + "和" + filtered[len(filtered)-1]
	}
}

func genericCreateEntrySurfaceRef(topology genericTopologyPlan) string {
	switch {
	case topology.Overview:
		return genericSurfaceOverviewID
	case topology.Collection:
		return genericSurfaceCollectionID
	case topology.Mutation:
		return genericSurfaceMutationID
	case topology.Inspection:
		return genericSurfaceInspectionID
	default:
		return genericSurfaceMutationID
	}
}

func genericCreateReturnSurfaceRef(topology genericTopologyPlan) string {
	switch {
	case topology.Overview:
		return genericSurfaceOverviewID
	case topology.Collection:
		return genericSurfaceCollectionID
	case topology.Inspection:
		return genericSurfaceInspectionID
	case topology.Mutation:
		return genericSurfaceMutationID
	default:
		return genericSurfaceMutationID
	}
}

func genericCreateEntryTitle(surfaceRef string) string {
	switch surfaceRef {
	case genericSurfaceOverviewID:
		return "从概览入口进入新建动作"
	case genericSurfaceCollectionID:
		return "从集合浏览入口进入新建动作"
	case genericSurfaceMutationID:
		return "进入实体变更入口开始录入"
	default:
		return "进入录入入口开始填写"
	}
}

func genericCreateReturnTitle(surfaceRef string) string {
	switch surfaceRef {
	case genericSurfaceOverviewID:
		return "返回概览入口查看最新摘要"
	case genericSurfaceCollectionID:
		return "返回集合浏览入口查看最新结果"
	case genericSurfaceInspectionID:
		return "返回结果检查入口查看最新结果"
	case genericSurfaceMutationID:
		return "保存后留在当前录入入口"
	default:
		return "保存后查看最新结果"
	}
}

func genericCreateReturnExpected(surfaceRef string) string {
	switch surfaceRef {
	case genericSurfaceOverviewID:
		return "首页摘要和最近记录已刷新"
	case genericSurfaceCollectionID:
		return "集合浏览入口已展示最新记录"
	case genericSurfaceInspectionID:
		return "结果检查入口已显示最新内容"
	case genericSurfaceMutationID:
		return "当前录入入口已展示保存成功"
	default:
		return "最新结果已可见"
	}
}

func compileGenericSpec(request Request, requirementText string) domainSpec {
	signals := detectGenericDomainSignals(requirementText)
	overlay := extractGenericRequirementOverlay(request, requirementText, signals)
	topology := detectGenericTopology(requirementText, signals)
	now := time.Now().UTC()
	if request.Now != nil {
		now = request.Now()
	}
	jobID := fallbackGeneratedID(request.JobID, "job-generic-open-lite", request.RequirementSource, now)
	prdID := fallbackGeneratedID(request.PRDID, "prd-generic-open-lite", request.RequirementSource, now)
	selectedTemplateLabel := fallbackID(request.TemplateID, "flutter-open-lite")
	templateID := strings.TrimSpace(request.TemplateID)
	preferredTemplateIDs := []string{"flutter-open-lite"}
	if templateID != "" {
		preferredTemplateIDs = []string{templateID}
	}
	realBuild := request.RealBuild || strings.TrimSpace(request.ExecutorImage) != ""
	executorImage := strings.TrimSpace(request.ExecutorImage)
	if realBuild && executorImage == "" {
		executorImage = "picoclaw/appfactory-builder:local"
	}
	flutterProfile := appruns.NewFlutterAndroidProfile()
	acceptanceChecks := buildGenericAcceptanceChecks(signals, topology, realBuild)
	commandProfile := appruns.CommandProfile{ProfileName: "prepare-p0", AllowedStages: []string{"baseline", "cheap"}, AllowedCommands: []string{"echo", "grep"}, DeniedCommands: []string{"rm", "sudo"}, MaxSingleCommandSeconds: 60, MaxParallelCommands: 1, NetworkPolicy: "disabled", WritableRoots: []string{"lib", "assets", "."}, EnvAllowlist: []string{"PATH", "HOME"}}
	if realBuild {
		commandProfile = flutterProfile.CommandProfile
	}
	coreScenarios := buildGenericCoreScenarios(signals, topology)
	featureList := buildGenericFeatureList(signals, topology)
	userFlows := buildGenericUserFlows(signals, topology)
	surfaceList := buildGenericSurfaceList(featureList)
	acceptanceCriteria := buildGenericAcceptanceCriteria(signals, topology)
	manualReviewPoints := buildGenericManualReviewPoints(signals, topology)
	return domainSpec{
		Kind:                    "generic",
		Slug:                    "generic-open-lite",
		Title:                   overlay.ResolvedTitle,
		TemplateID:              templateID,
		ExecutorImage:           executorImage,
		RequirementText:         requirementText,
		PRDID:                   prdID,
		JobID:                   jobID,
		SourceSummary:           sourceSummary(request.RequirementSource),
		Summary:                 signals.Summary,
		ProblemStatement:        signals.ProblemStatement,
		TargetUsers:             []UserProfile{{UserID: "user-builder", Label: signals.DomainLabel + " 应用使用者", Summary: signals.TargetUserSummary, PainPoints: append([]string(nil), signals.TargetUserPainPoints...)}, {UserID: "user-validator", Label: "内部验证人员", Summary: "需要快速确认 PRD 与 builder-input 是否同时保留领域语义和中性交互承载约束。", PainPoints: append([]string(nil), signals.ValidatorPainPoints...)}},
		CoreScenarios:           coreScenarios,
		Goals:                   append([]string(nil), signals.Goals...),
		NonGoals:                []string{"不引入登录、支付、广告、地图、推送、实时通信。", "不把远端 API 或自建服务器当作模板基础依赖。", "不处理上架、签名或复杂商业化配置。"},
		FeatureList:             featureList,
		SurfaceList:             surfaceList,
		UserFlows:               userFlows,
		DataEntities:            buildGenericDataEntities(signals, topology),
		TemplateConstraints:     TemplateConstraints{Stack: "flutter", AndroidRequired: true, RequiredCapabilities: buildGenericRequiredCapabilities(signals, topology), PreferredTemplateIDs: preferredTemplateIDs},
		AcceptanceCriteria:      acceptanceCriteria,
		ManualReviewPoints:      manualReviewPoints,
		KnownUnknowns:           []KnownUnknown{{Question: "后续是否需要在当前状态筛选基线之上补批量操作、归档、撤销或软删除", Impact: "medium", Owner: "product"}, {Question: "当需求出现复杂多实体关系时，是继续扩展 record 模型，还是拆出新模板", Impact: "medium", Owner: "engineering"}},
		TaskBundle:              buildGenericTaskBundleForTopology(signals, topology),
		AcceptanceChecks:        acceptanceChecks,
		GoalSummary:             buildGenericGoalSummary(selectedTemplateLabel, signals, topology),
		TemplateFitReasons:      buildGenericTemplateFitReasons(signals, topology),
		TemplateFitGaps:         []string{signals.TemplateFitDomainGap, "如果需求超出单一核心任务闭环，例如账号体系、远端协作或复杂后台流程，仍需要切换到更具体的模板或补新的模板能力。"},
		ImplementationPhases:    buildGenericImplementationPhases(signals, topology),
		ManualConstraints:       buildGenericManualConstraints(signals, topology),
		HumanNotes:              buildGenericHumanNotes(signals, topology),
		RequirementHighlights:   overlay.RequirementHighlights,
		SupportingAssumptions:   []string{"默认离线单机运行。", "模板服务单一核心任务闭环。", "本地持久化是当前默认数据源。"},
		PreferredAllowedPaths:   preferredAllowedPaths(realBuild),
		PreferredProtectedPaths: preferredProtectedPaths(realBuild),
		KnowledgePack:           append([]appruns.ProfileSkill(nil), flutterProfile.KnowledgePack...),
		CommandProfile:          commandProfile,
		ContextFiles:            appruns.ContextFiles{PRDMarkdownPath: prdMarkdownFileName, PRDJSONPath: prdJSONFileName, TemplateFitReportPath: fitReportFileName, ImplementationPlanPath: planFileName, ManualConstraintsPath: constraintsFileName, SupportingFiles: []string{requirementFileName, prdApprovalFileName, templateApprovalFileName, planningContextFileName, domainModelFileName, templateSlotMapFileName, taskAllocationFileName, acceptancePlanFileName}},
	}
}

func defaultFlutterFallbackChecks() []appruns.AcceptanceCheck {
	return []appruns.AcceptanceCheck{{CheckID: "check-structural-template-files-ready", Label: "确认 Flutter seed 工作区存在", Stage: "baseline", Required: true, Commands: []string{"grep -q . pubspec.yaml lib/main.dart test/widget_test.dart"}, SuccessCriteria: "默认 seed 工作区关键文件已就位。", TimeoutSeconds: 30}, {CheckID: "check-legacy-thin-fallback-probe", Label: "确认默认 thin fallback 写入探针", Stage: "cheap", Required: true, Commands: []string{"grep -E \"Generated by PicoClaw thin executor.|'goalSummary':\" lib/picoclaw_executor_probe.dart >/dev/null 2>&1"}, SuccessCriteria: "默认 thin executor fallback 已在允许目录中写入探针文件，用于证明 legacy fallback 链可用。", TimeoutSeconds: 30}, {CheckID: "check-legacy-thin-fallback-metadata", Label: "确认 fallback 探针包含执行元数据", Stage: "cheap", Required: true, Commands: []string{"grep -E \"'taskCount': [0-9]+,|'acceptanceCheckCount': [0-9]+,\" lib/picoclaw_executor_probe.dart >/dev/null 2>&1"}, SuccessCriteria: "fallback 探针里已带上 task 与 acceptance check 元数据，供定位 legacy fallback 输入边界，不代表真实 builder-runtime 产出。", TimeoutSeconds: 30}}
}

func buildOpenLiteRequiredWorkspaceFiles(topology genericTopologyPlan) []string {
	files := compactRefs(
		"lib/main.dart",
		"lib/models/record.dart",
		genericConditionalRef(topology.Overview, "lib/models/dashboard_summary.dart"),
		genericConditionalRef(topology.Overview, "lib/views/home_page.dart"),
		genericConditionalRef(topology.Overview, "lib/controllers/home_controller.dart"),
		genericConditionalRef(topology.Mutation, "lib/views/record_form_page.dart"),
		genericConditionalRef(topology.Mutation, "lib/controllers/record_form_controller.dart"),
		genericConditionalRef(topology.Collection || topology.Inspection, "lib/views/record_list_page.dart"),
		genericConditionalRef(topology.Collection || topology.Inspection, "lib/controllers/record_list_controller.dart"),
		genericConditionalRef(topology.Inspection, "lib/views/record_detail_page.dart"),
		"lib/repositories/record_repository.dart",
	)
	return files
}

func buildOpenLiteCounterDemoRemovedCommand(topology genericTopologyPlan) string {
	requiredFiles := buildOpenLiteRequiredWorkspaceFiles(topology)
	return fmt.Sprintf("grep -q . %s && ! grep -E 'Flutter Demo Home Page|You have pushed the button this many times|Counter increments smoke test|_counter|_incrementCounter|MyHomePage' lib/main.dart test/widget_test.dart >/dev/null 2>&1", strings.Join(requiredFiles, " "))
}

func buildOpenLiteRecordFlowWiringCommand(topology genericTopologyPlan) string {
	parts := make([]string, 0, 6)
	if topology.Mutation {
		parts = append(parts, "grep -E 'TextEditingController|TextFormField|DropdownButtonFormField|showDatePicker|RecordStatus|categoryController' lib/views/record_form_page.dart lib/controllers/record_form_controller.dart >/dev/null 2>&1")
	}
	switch {
	case topology.Overview:
		parts = append(parts, "grep -E 'HomePage' lib/main.dart >/dev/null 2>&1")
	case topology.Collection:
		parts = append(parts, "grep -E 'RecordListPage' lib/main.dart >/dev/null 2>&1")
	case topology.Mutation:
		parts = append(parts, "grep -E 'RecordFormPage' lib/main.dart >/dev/null 2>&1")
	}
	if topology.Collection {
		parts = append(parts, "grep -E 'RecordListPage' lib/views/record_list_page.dart >/dev/null 2>&1")
	}
	if topology.Inspection {
		parts = append(parts, "grep -E 'RecordDetailPage' lib/main.dart lib/views/record_detail_page.dart lib/views/record_list_page.dart >/dev/null 2>&1")
	}
	if topology.Delete {
		parts = append(parts, "grep -E 'deleteRecord' lib/main.dart lib/views/record_detail_page.dart lib/views/record_list_page.dart lib/controllers/home_controller.dart >/dev/null 2>&1")
	}
	return strings.Join(parts, " && ")
}

func defaultFlutterOpenLiteRealBuildChecks(signals genericDomainSignals, topology genericTopologyPlan) []appruns.AcceptanceCheck {
	flutterProfile := appruns.NewFlutterAndroidProfile()
	checks := make([]appruns.AcceptanceCheck, 0, len(flutterProfile.CommandChecks)+3)
	if len(flutterProfile.CommandChecks) > 0 {
		checks = append(checks, flutterProfile.CommandChecks[0])
	}
	checks = append(checks,
		appruns.AcceptanceCheck{CheckID: "check-open-lite-counter-demo-removed", Label: "阻断 open-lite 默认 counter 模板", Stage: appruns.StageCheap, Required: true, Commands: []string{buildOpenLiteCounterDemoRemovedCommand(topology)}, SuccessCriteria: "工作区中不再保留 Flutter 默认 counter demo 文案、状态字段、页面类名或 smoke test。", TimeoutSeconds: 30},
		appruns.AcceptanceCheck{CheckID: "check-open-lite-record-flow-wiring", Label: "确认 open-lite CRUD 页面与控制器已接线", Stage: appruns.StageCheap, Required: true, Commands: []string{buildOpenLiteRecordFlowWiringCommand(topology)}, SuccessCriteria: "代码中存在真实的新建、编辑、删除和详情接线，而不是只有静态按钮或单屏占位。", TimeoutSeconds: 30},
		appruns.AcceptanceCheck{CheckID: "check-open-lite-local-persistence-wiring", Label: "确认 open-lite 本地持久化已接线", Stage: appruns.StageCheap, Required: true, Commands: []string{"grep -E 'hive_flutter|Hive' pubspec.yaml lib/repositories/record_repository.dart lib/main.dart >/dev/null 2>&1 && ! grep -E 'shared_preferences|SharedPreferences' pubspec.yaml lib/repositories/record_repository.dart lib/main.dart >/dev/null 2>&1"}, SuccessCriteria: "代码或依赖中出现明确的 Hive 本地持久化实现，而不是只展示静态记录。", TimeoutSeconds: 30},
	)
	if strings.TrimSpace(signals.DomainCheckPattern) != "" {
		checks = append(checks,
			appruns.AcceptanceCheck{CheckID: "check-profile-open-lite-domain-branding", Label: "阻断 open-lite 默认 seed branding", Stage: appruns.StageCheap, Required: true, Commands: []string{"! grep -ER 'Open Lite Seed|open_lite_seed' lib android/app/src/main/AndroidManifest.xml android/app/src/main/res/values/strings.xml >/dev/null 2>&1"}, SuccessCriteria: "非空领域需求下，默认 seed branding 已从 Flutter 标题、首页标题和 Android 启动器名称中移除。", TimeoutSeconds: 30},
			appruns.AcceptanceCheck{CheckID: "check-profile-open-lite-domain-language", Label: "确认领域字段与文案已进入 open-lite 工作区", Stage: appruns.StageCheap, Required: true, Commands: []string{fmt.Sprintf("grep -ER '%s' lib test android/app/src/main/res/values/strings.xml >/dev/null 2>&1", signals.DomainCheckPattern)}, SuccessCriteria: signals.DomainCheckSummary, TimeoutSeconds: 30},
		)
	}
	if len(flutterProfile.CommandChecks) > 1 {
		checks = append(checks, flutterProfile.CommandChecks[1:]...)
	}
	return checks
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
