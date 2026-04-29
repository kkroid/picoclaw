package prepare

import "strings"

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
	surfaceList := normalizeInteractionSurfaces(prd.SurfaceList, prd.ScreenList)
	if len(surfaceList) > 0 {
		builder.WriteString("\n## 交互承载单元\n\n")
		for _, surface := range surfaceList {
			builder.WriteString("- ")
			builder.WriteString(firstNonEmpty(surface.Label, surface.SurfaceID))
			builder.WriteString("：")
			builder.WriteString(surface.Purpose)
			builder.WriteString("\n")
		}
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
