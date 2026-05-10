package prepare

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
)

type genericDomainProfileCatalog struct {
	SchemaVersion string                 `json:"SchemaVersion"`
	Profiles      []genericDomainProfile `json:"Profiles"`
}

type genericDomainProfile struct {
	ProfileID   string               `json:"ProfileID"`
	MatchTokens []string             `json:"MatchTokens"`
	Signals     genericDomainSignals `json:"Signals"`
}

var (
	//go:embed generic_domain_profiles.json
	genericDomainProfilesFixture []byte

	genericDomainProfilesOnce    sync.Once
	genericDomainProfilesCatalog genericDomainProfileCatalog
	genericDomainProfilesErr     error
)

func loadGenericDomainProfileCatalog() (genericDomainProfileCatalog, error) {
	genericDomainProfilesOnce.Do(func() {
		if err := json.Unmarshal(genericDomainProfilesFixture, &genericDomainProfilesCatalog); err != nil {
			genericDomainProfilesErr = fmt.Errorf("unmarshal generic domain profiles: %w", err)
			return
		}
		genericDomainProfilesErr = validateGenericDomainProfileCatalog(genericDomainProfilesCatalog)
	})
	if genericDomainProfilesErr != nil {
		return genericDomainProfileCatalog{}, genericDomainProfilesErr
	}
	return genericDomainProfilesCatalog, nil
}

func validateGenericDomainProfileCatalog(catalog genericDomainProfileCatalog) error {
	if strings.TrimSpace(catalog.SchemaVersion) == "" {
		return fmt.Errorf("generic domain profiles schema version is required")
	}
	if len(catalog.Profiles) == 0 {
		return fmt.Errorf("generic domain profiles must declare at least one profile")
	}
	seenIDs := make(map[string]struct{}, len(catalog.Profiles))
	for _, profile := range catalog.Profiles {
		profileID := strings.TrimSpace(profile.ProfileID)
		if profileID == "" {
			return fmt.Errorf("generic domain profile id is required")
		}
		if _, exists := seenIDs[profileID]; exists {
			return fmt.Errorf("duplicate generic domain profile id %q", profileID)
		}
		seenIDs[profileID] = struct{}{}
		if len(profile.MatchTokens) == 0 {
			return fmt.Errorf("generic domain profile %q must declare match tokens", profileID)
		}
		if strings.TrimSpace(profile.Signals.AppTitle) == "" {
			return fmt.Errorf("generic domain profile %q missing app title", profileID)
		}
		if strings.TrimSpace(profile.Signals.DomainCheckPattern) == "" {
			return fmt.Errorf("generic domain profile %q missing domain check pattern", profileID)
		}
		if err := validateGenericDomainProfileEntity(profileID, "entity", profile.Signals.Entity); err != nil {
			return err
		}
		if err := validateGenericDomainProfileEntity(profileID, "summary entity", profile.Signals.SummaryEntity); err != nil {
			return err
		}
	}
	return nil
}

func validateGenericDomainProfileEntity(profileID, label string, entity DataEntity) error {
	if strings.TrimSpace(entity.EntityID) == "" {
		return fmt.Errorf("generic domain profile %q missing %s id", profileID, label)
	}
	if len(entity.Fields) == 0 {
		return fmt.Errorf("generic domain profile %q %s must declare fields", profileID, label)
	}
	for _, field := range entity.Fields {
		if strings.TrimSpace(field.Name) == "" {
			return fmt.Errorf("generic domain profile %q %s has field without name", profileID, label)
		}
		if strings.TrimSpace(field.Type) == "" {
			return fmt.Errorf("generic domain profile %q %s field %q missing type", profileID, label, field.Name)
		}
	}
	return nil
}

func matchesGenericDomainProfile(profile genericDomainProfile, raw, lower string) bool {
	for _, token := range profile.MatchTokens {
		trimmed := strings.TrimSpace(token)
		if trimmed == "" {
			continue
		}
		if strings.Contains(raw, trimmed) || strings.Contains(lower, strings.ToLower(trimmed)) {
			return true
		}
	}
	return false
}

func defaultGenericDomainSignals() genericDomainSignals {
	return genericDomainSignals{
		AppTitle:                 "通用工具 App",
		DomainLabel:              "通用工具",
		SummaryLabel:             "概览摘要",
		OverviewTitle:            "查看概览摘要",
		OverviewSummary:          "用户打开应用后可以看到摘要卡和最近记录。",
		CreateTitle:              "创建一条新记录",
		CreateSummary:            "用户可以通过实体变更承载单元录入一条记录，并立即回到概览或集合浏览入口看到结果。",
		UpdateTitle:              "编辑已有记录",
		UpdateSummary:            "用户可以从结果检查入口进入实体变更承载单元，更新标题、分类、状态和备注。",
		DeleteTitle:              "删除已有记录",
		DeleteSummary:            "用户可以在确认后删除单条记录，并让概览与集合结果同步刷新。",
		InspectTitle:             "查看记录详情",
		InspectSummary:           "用户可以从集合浏览或概览入口进入结果检查入口查看单条记录。",
		FilterTitle:              "按状态筛选记录",
		FilterSummary:            "用户可以在集合浏览承载单元按待整理、进行中、已完成切换视图，快速收敛到目标记录。",
		HasFilter:                true,
		OverviewFeatureTitle:     "概览承载单元",
		OverviewFeatureSummary:   "承接摘要指标、最近记录与主入口动作，作为单一核心任务的概览入口。",
		CollectionFeatureTitle:   "集合浏览承载单元",
		CollectionFeatureSummary: "承接记录集合浏览、状态筛选与结果检查入口。",
		MutationFeatureTitle:     "实体变更承载单元",
		MutationFeatureSummary:   "承接新增、编辑、校验和保存动作。",
		DeleteFeatureTitle:       "记录删除收口",
		DeleteFeatureSummary:     "承接单条记录删除确认，并让概览摘要与集合结果同步收敛。",
		CreateFieldsExpected:     "可以看到标题、分类、状态和备注字段",
		UpdateFieldsExpected:     "标题、分类、状态与备注可被更新",
		DetailFieldsExpected:     "能看到单条记录的分类、状态、备注和日期",
		FilterExpected:           "列表立即收敛到对应状态的记录",
		Summary:                  "把自然语言需求整理成基于通用中性交互承载 Flutter 模板的 Android MVP 规格，并输出 Builder 可执行输入包。",
		ProblemStatement:         "generic compile 不能继续退化为单屏 smoke fallback，需要稳定产出概览、集合浏览、实体变更、结果检查和本地持久化都可映射的通用结构。",
		Goals:                    []string{"输出覆盖概览、集合浏览、实体变更、结果检查和本地存储的 PRD 与 builder-input。", "让体重记录、待办、习惯打卡这类 generic domain 都能映射到同一模板槽位。", "把 generic 需求压成可被 Builder 稳定消费的中性交互承载任务包，而不是单屏 smoke 占位。"},
		Highlights:               []string{"概览承载单元需要提供摘要卡和最近记录。", "实体变更承载单元需要录入标题、分类、状态和备注。", "集合浏览承载单元需要浏览记录并进入结果检查入口。", "结果检查入口需要支持编辑和删除收口。"},
		Entity:                   DataEntity{EntityID: "entity-record", Name: "通用记录", Source: "local_storage", Fields: []DataField{{Name: "record_id", Type: "string", Required: true, Description: "记录唯一标识"}, {Name: "title", Type: "string", Required: true, Description: "记录标题"}, {Name: "category", Type: "string", Required: true, Description: "记录分类"}, {Name: "status", Type: "enum[inbox,inProgress,done]", Required: true, Description: "记录状态"}, {Name: "updated_at", Type: "date", Required: true, Description: "更新时间"}, {Name: "note", Type: "string", Required: false, Description: "补充备注"}}},
		SummaryEntity:            DataEntity{EntityID: "entity-dashboard-summary", Name: "概览摘要", Source: "derived", Fields: []DataField{{Name: "total_count", Type: "int", Required: true, Description: "记录总数"}, {Name: "inbox_count", Type: "int", Required: true, Description: "待整理记录数"}, {Name: "in_progress_count", Type: "int", Required: true, Description: "进行中记录数"}, {Name: "done_count", Type: "int", Required: true, Description: "已完成记录数"}}},
		OverviewAcceptance:       "概览承载单元必须展示摘要卡和最近记录。",
		ListAcceptance:           "集合浏览承载单元必须展示完整记录、支持按状态筛选，并允许进入结果检查入口。",
		FormAcceptance:           "用户可以通过实体变更承载单元创建或编辑一条记录并立即看到结果。",
		DeleteAcceptance:         "用户确认删除后，概览摘要和集合结果内容必须同步收敛。",
		NavigationAcceptance:     "概览、集合浏览、实体变更和结果检查入口之间路径明确且可达。",
		ManualReviewSummary:      "检查模板默认文案是否仍然携带垂直业务语义",
		ManualReviewReason:       "该模板必须保持中性，不应残留记账等强业务词。",
		SummaryReviewSummary:     "确认新增领域字段仍遵守交互承载映射规则",
		SummaryReviewReason:      "需求拆解后二开应优先复用概览、集合浏览、实体变更、结果检查四类承载单元。",
		GoalSummary:              "将通用需求整理成基于 %s 的中性交互承载 Android MVP 输入包，围绕概览、集合浏览、实体变更、结果检查、本地持久化和状态筛选能力形成最小 CRUD 闭环。",
		TemplateFitDomainGap:     "真实业务字段仍需在后续任务包里覆盖默认记录模型与默认文案，不能把 seed 内容直接视为交付结果。",
		ImplementationPhases:     []string{"阶段 1：冻结通用记录实体、概览摘要模型和本地仓储边界。", "阶段 2：搭建概览、集合浏览、实体变更、结果检查四类承载单元并接通最小导航。", "阶段 3：补齐创建、编辑、删除、状态筛选和结果检查主流程，为后续真实 validate 链路预留稳定输入包。"},
		ManualConstraints:        []string{"当前 generic 模板默认本地优先，不接入登录、支付、广告、推送、地图、实时通信。", "模板必须保持中性，不保留记账或其他强垂直业务语义。", "当前阶段仍以稳定 Builder 输入包和多页面 seed 为主，不把远端 API 作为默认前提。", "需求拆解后二开应优先复用概览、集合浏览、实体变更、结果检查四类承载单元，而不是新增第二套平行骨架。"},
		HumanNotes:               []map[string]string{{"note_id": "note-generic-template", "summary": "该输入包用于 generic domain 的多页面工具类 seed，不再允许退回单屏 smoke-only 模板。", "scope": "engineering"}, {"note_id": "note-field-remap", "summary": "后续具体业务应优先映射记录模型和摘要卡，而不是新增第二套页面骨架。", "scope": "product"}, {"note_id": "note-crud-baseline", "summary": "当前 generic 模板默认能力已经扩展到最小 CRUD 闭环，后续优先在此基础上增量改造。", "scope": "engineering"}},
		TargetUserSummary:        "需要一个围绕单一核心任务运行的轻量移动工具。",
		TargetUserPainPoints:     []string{"业务术语不稳定", "需求经常先是自然语言，再被压成结构化页面与实体"},
		ValidatorPainPoints:      []string{"单屏 smoke 模板无法承接真实 generic 需求", "模板边界和页面槽位容易漂移"},
		DomainCheckPattern:       "record|dashboard_summary|detail|filter",
		DomainCheckSummary:       "生成代码或配置中必须出现通用记录、摘要或结果检查相关字段。",
	}
}
