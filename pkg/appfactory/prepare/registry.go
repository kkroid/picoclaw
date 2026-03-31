package prepare

import (
	"fmt"
	"sort"
	"strings"
)

type TemplateRegistryEntry struct {
	TemplateID     string
	Name           string
	Description    string
	PinnedRef      string
	License        string
	Stack          string
	AndroidSupport bool
	Capabilities   []string
	HealthStatus   string
	RiskNotes      []string
	SelectionHints []string
}

type templateSelection struct {
	Entry   TemplateRegistryEntry
	Reasons []string
	Gaps    []string
}

type TemplateMatch struct {
	Entry           TemplateRegistryEntry
	Score           int
	HardGatePassed  bool
	Reasons         []string
	MissingFeatures []string
}

func GetTemplateRegistryEntry(templateID string) (TemplateRegistryEntry, error) {
	entry, ok := findTemplateByID(defaultTemplateRegistry(), strings.TrimSpace(templateID))
	if !ok {
		return TemplateRegistryEntry{}, fmt.Errorf("template %q not found in registry", strings.TrimSpace(templateID))
	}
	return entry, nil
}

func ListTemplateRegistryEntries() []TemplateRegistryEntry {
	entries := defaultTemplateRegistry()
	result := make([]TemplateRegistryEntry, 0, len(entries))
	for _, entry := range entries {
		result = append(result, entry)
	}
	return result
}

func MatchTemplates(constraints TemplateConstraints, maxResults int) []TemplateMatch {
	entries := defaultTemplateRegistry()
	results := make([]TemplateMatch, 0, len(entries))
	for _, entry := range entries {
		score, ok := scoreTemplateEntry(entry, constraints)
		if !ok {
			continue
		}
		selection := buildTemplateSelection(entry, constraints, false)
		_, missing := splitCapabilities(entry.Capabilities, constraints.RequiredCapabilities)
		results = append(results, TemplateMatch{
			Entry:           entry,
			Score:           score,
			HardGatePassed:  true,
			Reasons:         append(append([]string(nil), selection.Reasons...), selection.Gaps...),
			MissingFeatures: missing,
		})
	}
	sort.Slice(results, func(i, j int) bool {
		if results[i].Score == results[j].Score {
			return results[i].Entry.TemplateID < results[j].Entry.TemplateID
		}
		return results[i].Score > results[j].Score
	})
	if maxResults <= 0 || maxResults >= len(results) {
		return results
	}
	return append([]TemplateMatch(nil), results[:maxResults]...)
}

func finalizeTemplateSelection(spec domainSpec) (domainSpec, error) {
	selection, err := resolveTemplateSelection(spec.TemplateConstraints, spec.TemplateID)
	if err != nil {
		return domainSpec{}, err
	}
	spec.TemplateID = selection.Entry.TemplateID
	spec.TemplateName = selection.Entry.Name
	spec.TemplatePinnedRef = selection.Entry.PinnedRef
	spec.TemplateHealthStatus = selection.Entry.HealthStatus
	spec.TemplateFitReasons = append(selection.Reasons, spec.TemplateFitReasons...)
	spec.TemplateFitGaps = append(spec.TemplateFitGaps, selection.Gaps...)
	return spec, nil
}

func resolveTemplateSelection(constraints TemplateConstraints, explicitTemplateID string) (templateSelection, error) {
	entries := defaultTemplateRegistry()
	explicitTemplateID = strings.TrimSpace(explicitTemplateID)
	if explicitTemplateID != "" {
		entry, ok := findTemplateByID(entries, explicitTemplateID)
		if !ok {
			return templateSelection{}, fmt.Errorf("template %q not found in registry", explicitTemplateID)
		}
		return buildTemplateSelection(entry, constraints, true), nil
	}
	bestScore := -1 << 30
	bestEntry := TemplateRegistryEntry{}
	found := false
	for _, entry := range entries {
		score, ok := scoreTemplateEntry(entry, constraints)
		if !ok {
			continue
		}
		if !found || score > bestScore || (score == bestScore && entry.TemplateID < bestEntry.TemplateID) {
			bestScore = score
			bestEntry = entry
			found = true
		}
	}
	if !found {
		return templateSelection{}, fmt.Errorf("no template matches stack=%q android_required=%t preferred_template_ids=%v", constraints.Stack, constraints.AndroidRequired, constraints.PreferredTemplateIDs)
	}
	return buildTemplateSelection(bestEntry, constraints, false), nil
}

func scoreTemplateEntry(entry TemplateRegistryEntry, constraints TemplateConstraints) (int, bool) {
	if constraints.Stack != "" && entry.Stack != constraints.Stack {
		return 0, false
	}
	if constraints.AndroidRequired && !entry.AndroidSupport {
		return 0, false
	}
	for _, excluded := range constraints.ExcludedLicenses {
		if strings.EqualFold(strings.TrimSpace(excluded), entry.License) {
			return 0, false
		}
	}
	if entry.HealthStatus == "blocked" {
		return 0, false
	}
	score := 0
	if index := indexOf(constraints.PreferredTemplateIDs, entry.TemplateID); index >= 0 {
		score += 100 - index*10
	}
	matched, missing := splitCapabilities(entry.Capabilities, constraints.RequiredCapabilities)
	score += len(matched) * 4
	score -= len(missing) * 2
	switch entry.HealthStatus {
	case "healthy":
		score += 10
	case "degraded":
		score += 2
	case "deprecated":
		score -= 8
	}
	if entry.AndroidSupport {
		score += 3
	}
	return score, true
}

func buildTemplateSelection(entry TemplateRegistryEntry, constraints TemplateConstraints, explicit bool) templateSelection {
	matched, missing := splitCapabilities(entry.Capabilities, constraints.RequiredCapabilities)
	reasons := []string{fmt.Sprintf("命中模板注册表条目：%s（%s）", entry.TemplateID, entry.Name)}
	if explicit {
		reasons = append(reasons, fmt.Sprintf("显式指定 template_id=%s，已在模板注册表中确认可用。", entry.TemplateID))
	}
	if constraints.Stack != "" {
		reasons = append(reasons, fmt.Sprintf("模板技术栈与约束一致：%s。", entry.Stack))
	}
	if constraints.AndroidRequired && entry.AndroidSupport {
		reasons = append(reasons, "模板声明支持 Android 交付。")
	}
	if len(matched) > 0 {
		reasons = append(reasons, fmt.Sprintf("模板已覆盖关键能力：%s。", strings.Join(matched, "、")))
	}
	if len(entry.SelectionHints) > 0 {
		reasons = append(reasons, entry.SelectionHints[0])
	}
	gaps := make([]string, 0, len(missing)+len(entry.RiskNotes)+1)
	gaps = append(gaps, entry.RiskNotes...)
	if len(missing) > 0 {
		gaps = append(gaps, fmt.Sprintf("模板当前未直接覆盖这些能力，实施阶段需补齐：%s。", strings.Join(missing, "、")))
	}
	if entry.HealthStatus == "degraded" || entry.HealthStatus == "deprecated" {
		gaps = append(gaps, fmt.Sprintf("模板健康状态为 %s，落地时要额外关注 seed 代码质量与依赖漂移。", entry.HealthStatus))
	}
	return templateSelection{Entry: entry, Reasons: reasons, Gaps: gaps}
}

func splitCapabilities(available, required []string) (matched []string, missing []string) {
	availableSet := make(map[string]struct{}, len(available))
	for _, item := range available {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		availableSet[item] = struct{}{}
	}
	for _, item := range required {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if _, ok := availableSet[item]; ok {
			matched = append(matched, item)
			continue
		}
		missing = append(missing, item)
	}
	sort.Strings(matched)
	sort.Strings(missing)
	return matched, missing
}

func indexOf(items []string, target string) int {
	for index, item := range items {
		if strings.TrimSpace(item) == target {
			return index
		}
	}
	return -1
}

func findTemplateByID(entries []TemplateRegistryEntry, templateID string) (TemplateRegistryEntry, bool) {
	for _, entry := range entries {
		if entry.TemplateID == templateID {
			return entry, true
		}
	}
	return TemplateRegistryEntry{}, false
}

func defaultTemplateRegistry() []TemplateRegistryEntry {
	return []TemplateRegistryEntry{
		{
			TemplateID:     "flutter-finance-lite",
			Name:           "Flutter Finance Lite",
			Description:    "适合作为离线记账和列表/表单类 Android MVP 的起始模板。",
			PinnedRef:      "v0.1.0",
			License:        "MIT",
			Stack:          "flutter",
			AndroidSupport: true,
			Capabilities:   []string{"bottom-navigation", "form", "list", "local-storage", "summary-card"},
			HealthStatus:   "healthy",
			RiskNotes:      []string{"模板只提供 seed 级页面结构，不能把默认页面直接当作交付结果。"},
			SelectionHints: []string{"适合需要首页概览、录入表单、列表与本地存储的离线 Android MVP。"},
		},
		{
			TemplateID:     "flutter-template-demo",
			Name:           "Flutter Template Demo",
			Description:    "适合作为单屏或轻量状态型 Flutter MVP 的通用模板。",
			PinnedRef:      "v0.1.0",
			License:        "MIT",
			Stack:          "flutter",
			AndroidSupport: true,
			Capabilities:   []string{"local-state", "scaffold", "single-screen"},
			HealthStatus:   "healthy",
			RiskNotes:      []string{"如果需求扩展到多页面复杂流程，需要在实现阶段尽快升级导航与状态管理骨架。"},
			SelectionHints: []string{"适合还处在需求压缩阶段、只需要先跑通单屏主流程的 Android MVP。"},
		},
	}
}
