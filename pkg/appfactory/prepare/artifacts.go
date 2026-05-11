package prepare

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	appruns "github.com/sipeed/oneappfactory/pkg/appfactory/runs"
)

func buildRuntimeConfig(spec domainSpec) RuntimeConfig {
	commandProfileJSON, _ := json.Marshal(spec.CommandProfile)
	contextFilesJSON, _ := json.Marshal(spec.ContextFiles)
	humanNotesJSON, _ := json.Marshal(spec.HumanNotes)
	return RuntimeConfig{
		SchemaVersion:   defaultSchemaVersion,
		ExecutorImage:   spec.ExecutorImage,
		WorkspacePath:   filepath.ToSlash(filepath.Join("/workspace", spec.JobID)),
		ArtifactDir:     filepath.ToSlash(filepath.Join("/artifacts", spec.JobID)),
		GoalSummary:     spec.GoalSummary,
		KnowledgePack:   append([]appruns.ProfileSkill(nil), spec.KnowledgePack...),
		CommandProfile:  commandProfileJSON,
		ContextFiles:    contextFilesJSON,
		IterationBudget: 3,
		TokenBudget:     4000,
		HumanNotes:      humanNotesJSON,
	}
}

func buildPlanningContext(spec domainSpec, prd PRD, policy appruns.PlanningPolicySnapshot) PlanningContext {
	planningPolicy := appruns.NormalizePlanningPolicySnapshot(policy)
	return PlanningContext{
		SchemaVersion:          defaultSchemaVersion,
		JobID:                  spec.JobID,
		PRDID:                  spec.PRDID,
		TemplateID:             spec.TemplateID,
		PRDSubjectVersion:      PRDCompileSourceVersion(prd),
		TemplateSubjectVersion: TemplateCompileSourceVersion(spec.TemplateID, spec.TemplatePinnedRef),
		PlanningPolicyVersion:  planningPolicy.PolicyVersion,
		PlanningModelSnapshot:  buildPlanningModelSnapshot(planningPolicy),
		ExecutionRouteSnapshot: buildExecutionRouteSnapshot(spec),
		PlanningPolicy:         planningPolicy,
		HumanNotes:             append([]map[string]string(nil), spec.HumanNotes...),
		ManualConstraints:      append([]string(nil), spec.ManualConstraints...),
		RequirementHighlights:  append([]string(nil), spec.RequirementHighlights...),
		SupportingAssumptions:  append([]string(nil), spec.SupportingAssumptions...),
	}
}

func buildPlanningModelSnapshot(policy appruns.PlanningPolicySnapshot) PlanningModelSnapshot {
	snapshot := PlanningModelSnapshot{}
	for _, stage := range policy.Stages {
		route := strings.TrimSpace(string(stage.Route))
		switch stage.Stage {
		case appruns.PlanningStageRequirementStructuring:
			snapshot.RequirementStructuring = route
		case appruns.PlanningStageDomainModeling:
			snapshot.DomainModeling = route
		case appruns.PlanningStageTaskAllocation:
			snapshot.TaskAllocation = route
		case appruns.PlanningStageAcceptancePlanning:
			snapshot.AcceptancePlanning = route
		case appruns.PlanningStageBuildInputProjection:
			snapshot.BuildInputProjection = route
		}
	}
	return snapshot
}

func buildExecutionRouteSnapshot(spec domainSpec) ExecutionRouteSnapshot {
	routeCounts := map[string]int{}
	routes := make([]ExecutionTaskRoute, 0, len(spec.TaskBundle))
	defaultRouteHint := appruns.TaskRouteHintDefaultModel
	upgradePolicy := currentBuilderRuntimeUpgradePolicy()
	for _, task := range spec.TaskBundle {
		normalized := appruns.NormalizeTaskBundleItem(task)
		routeHint := normalized.RouteHint
		if routeHint == "" {
			routeHint = defaultRouteHint
		}
		routeCounts[string(routeHint)]++
		allocationID := strings.TrimSpace(normalized.TaskID)
		if normalized.AllocationTransition != nil && strings.TrimSpace(normalized.AllocationTransition.AllocationID) != "" {
			allocationID = strings.TrimSpace(normalized.AllocationTransition.AllocationID)
		}
		routes = append(routes, ExecutionTaskRoute{
			AllocationID: allocationID,
			Category:     normalized.Category,
			TaskType:     normalized.EffectiveTaskType(),
			RouteHint:    routeHint,
			RiskLevel:    normalized.RiskLevel,
			TargetPaths:  append([]string(nil), normalized.TargetPaths...),
		})
	}
	return ExecutionRouteSnapshot{
		DefaultRouteHint:  defaultRouteHint,
		RouteHintCounts:   routeCounts,
		TaskRoutes:        routes,
		UpgradePolicy:     upgradePolicy,
		StartUpgradeCheck: buildStartUpgradeCheck(spec.TaskBundle, upgradePolicy),
		AllowedPaths:      append([]string(nil), spec.PreferredAllowedPaths...),
		ProtectedPaths:    append([]string(nil), spec.PreferredProtectedPaths...),
		CommandProfileRef: strings.TrimSpace(spec.CommandProfile.ProfileName),
	}
}

func currentBuilderRuntimeUpgradePolicy() ExecutionUpgradePolicy {
	return ExecutionUpgradePolicy{
		Source:                       "config/oneappfactory.example.json:appfactory.builder_runtime.upgrade_threshold",
		MaxAttemptsBeforeUpgrade:     2,
		MaxFilesBeforeUpgrade:        2,
		MaxSchemaDriftBeforeUpgrade:  1,
		MaxUnrelatedOperationRate:    0.4,
		UpgradeOnValidationFail:      true,
		UpgradeOnPatchParseFail:      true,
		UpgradeOnScopeViolation:      true,
		UpgradeOnSemanticConflict:    true,
		SupportedRetryUpgradeSignals: []string{"parse_failure", "schema_drift", "unrelated_edits", "scope_violation", "validation_failure", "semantic_conflict"},
	}
}

func buildStartUpgradeCheck(tasks []appruns.TaskBundleItem, policy ExecutionUpgradePolicy) StartUpgradeCheck {
	check := StartUpgradeCheck{
		CurrentAttempt:          1,
		ConcreteTargetFileCount: len(concretePlanningTargetPaths(tasks)),
	}
	if policy.MaxAttemptsBeforeUpgrade > 0 && check.CurrentAttempt > policy.MaxAttemptsBeforeUpgrade {
		check.TriggeredReasons = append(check.TriggeredReasons, "max_attempts_before_upgrade")
	}
	if policy.MaxFilesBeforeUpgrade > 0 && check.ConcreteTargetFileCount > policy.MaxFilesBeforeUpgrade {
		check.TriggeredReasons = append(check.TriggeredReasons, "max_files_before_upgrade")
	}
	check.TriggeredReasons = trimStringSlice(check.TriggeredReasons)
	check.WouldUpgradeFromStart = len(check.TriggeredReasons) > 0
	return check
}

func concretePlanningTargetPaths(tasks []appruns.TaskBundleItem) []string {
	seen := map[string]struct{}{}
	paths := make([]string, 0)
	for _, task := range tasks {
		for _, path := range task.TargetPaths {
			trimmed := filepath.ToSlash(strings.TrimSpace(path))
			if trimmed == "" || strings.ContainsAny(trimmed, "*?[]") {
				continue
			}
			if _, ok := seen[trimmed]; ok {
				continue
			}
			seen[trimmed] = struct{}{}
			paths = append(paths, trimmed)
		}
	}
	return paths
}

func buildDomainModel(spec domainSpec) DomainModel {
	summaryMetrics := make([]string, 0, len(spec.DataEntities))
	for _, entity := range spec.DataEntities {
		if strings.EqualFold(strings.TrimSpace(entity.Source), "derived") {
			summaryMetrics = append(summaryMetrics, strings.TrimSpace(entity.Name))
		}
	}
	criticalFlows := make([]string, 0, len(spec.UserFlows))
	for _, flow := range spec.UserFlows {
		if title := strings.TrimSpace(flow.Title); title != "" {
			criticalFlows = append(criticalFlows, title)
		}
	}
	semanticRules := buildSemanticAcceptanceRules(spec)
	return DomainModel{
		SchemaVersion:       defaultSchemaVersion,
		JobID:               spec.JobID,
		PRDID:               spec.PRDID,
		TemplateID:          spec.TemplateID,
		DomainName:          strings.TrimSpace(spec.Title),
		ComplexityLevel:     strings.TrimSpace(spec.ComplexityLevel),
		CapabilityFlags:     append([]string(nil), spec.CapabilityFlags...),
		Entities:            append([]DataEntity(nil), spec.DataEntities...),
		SummaryMetrics:      summaryMetrics,
		BehaviorRules:       append([]DomainBehaviorRule(nil), spec.BehaviorRules...),
		PersistenceContract: clonePersistenceContract(spec.PersistenceContract),
		DomainCopy: DomainCopy{
			Title:            strings.TrimSpace(spec.Title),
			Summary:          strings.TrimSpace(spec.Summary),
			ProblemStatement: strings.TrimSpace(spec.ProblemStatement),
		},
		CriticalFlows:           trimStringSlice(criticalFlows),
		SemanticAcceptanceRules: semanticRules,
		ProtocolContract:        cloneProtocolContract(spec.ProtocolContract),
		RealtimeContract:        cloneRealtimeContract(spec.RealtimeContract),
		RuntimeContract:         cloneRuntimeDependencyContract(spec.RuntimeContract),
	}
}

func clonePersistenceContract(contract *PersistenceContract) *PersistenceContract {
	if contract == nil {
		return nil
	}
	return &PersistenceContract{
		Mode:           strings.TrimSpace(contract.Mode),
		RepositoryPath: strings.TrimSpace(contract.RepositoryPath),
		EntityRefs:     append([]string(nil), contract.EntityRefs...),
		CapabilityRefs: append([]string(nil), contract.CapabilityRefs...),
		Required:       contract.Required,
	}
}

func cloneProtocolContract(contract *ProtocolContract) *ProtocolContract {
	if contract == nil {
		return nil
	}
	clone := &ProtocolContract{
		BasePath: strings.TrimSpace(contract.BasePath),
		ProjectContext: ProjectContextSpec{
			Required:   contract.ProjectContext.Required,
			QueryKey:   strings.TrimSpace(contract.ProjectContext.QueryKey),
			HeaderKey:  strings.TrimSpace(contract.ProjectContext.HeaderKey),
			EntityRefs: append([]string(nil), contract.ProjectContext.EntityRefs...),
		},
		Auth: AuthContract{
			Mode:      strings.TrimSpace(contract.Auth.Mode),
			HeaderKey: strings.TrimSpace(contract.Auth.HeaderKey),
			Required:  contract.Auth.Required,
		},
		Endpoints: make([]EndpointContract, 0, len(contract.Endpoints)),
	}
	for _, endpoint := range contract.Endpoints {
		clone.Endpoints = append(clone.Endpoints, EndpointContract{
			EndpointID:     strings.TrimSpace(endpoint.EndpointID),
			Method:         strings.TrimSpace(endpoint.Method),
			Path:           strings.TrimSpace(endpoint.Path),
			Purpose:        strings.TrimSpace(endpoint.Purpose),
			EntityRefs:     append([]string(nil), endpoint.EntityRefs...),
			CapabilityRefs: append([]string(nil), endpoint.CapabilityRefs...),
			RequestBody:    strings.TrimSpace(endpoint.RequestBody),
			ResponseBody:   strings.TrimSpace(endpoint.ResponseBody),
			Required:       endpoint.Required,
			Excluded:       endpoint.Excluded,
		})
	}
	return clone
}

func cloneRealtimeContract(contract *RealtimeContract) *RealtimeContract {
	if contract == nil {
		return nil
	}
	clone := &RealtimeContract{
		Transport:       strings.TrimSpace(contract.Transport),
		URLPattern:      strings.TrimSpace(contract.URLPattern),
		Subscribe:       cloneRealtimeClientMessage(contract.Subscribe),
		Unsubscribe:     cloneRealtimeClientMessage(contract.Unsubscribe),
		ReconnectPolicy: strings.TrimSpace(contract.ReconnectPolicy),
		ReplayPolicy:    strings.TrimSpace(contract.ReplayPolicy),
		ServerEvents:    make([]RealtimeServerEvent, 0, len(contract.ServerEvents)),
	}
	for _, event := range contract.ServerEvents {
		clone.ServerEvents = append(clone.ServerEvents, RealtimeServerEvent{
			EventType:      strings.TrimSpace(event.EventType),
			Purpose:        strings.TrimSpace(event.Purpose),
			CapabilityRefs: append([]string(nil), event.CapabilityRefs...),
			EntityRefs:     append([]string(nil), event.EntityRefs...),
		})
	}
	return clone
}

func cloneRealtimeClientMessage(message RealtimeClientMessage) RealtimeClientMessage {
	return RealtimeClientMessage{Type: strings.TrimSpace(message.Type), Required: append([]string(nil), message.Required...)}
}

func cloneRuntimeDependencyContract(contract *RuntimeDependencyContract) *RuntimeDependencyContract {
	if contract == nil {
		return nil
	}
	clone := &RuntimeDependencyContract{
		PackageName:     strings.TrimSpace(contract.PackageName),
		AppEntry:        strings.TrimSpace(contract.AppEntry),
		RouteStrategy:   strings.TrimSpace(contract.RouteStrategy),
		StateManagement: strings.TrimSpace(contract.StateManagement),
		SettingsStore:   strings.TrimSpace(contract.SettingsStore),
		FakeTestHarness: append([]string(nil), contract.FakeTestHarness...),
		Dependencies:    make([]DependencyContract, 0, len(contract.Dependencies)),
		PlatformConfig:  make([]PlatformConfig, 0, len(contract.PlatformConfig)),
	}
	for _, dependency := range contract.Dependencies {
		clone.Dependencies = append(clone.Dependencies, DependencyContract{Name: strings.TrimSpace(dependency.Name), Version: strings.TrimSpace(dependency.Version), Purpose: strings.TrimSpace(dependency.Purpose)})
	}
	for _, platform := range contract.PlatformConfig {
		clone.PlatformConfig = append(clone.PlatformConfig, PlatformConfig{Platform: strings.TrimSpace(platform.Platform), Paths: append([]string(nil), platform.Paths...), Purpose: strings.TrimSpace(platform.Purpose)})
	}
	return clone
}

func buildTemplateSlotMap(spec domainSpec) TemplateSlotMap {
	templateSubjectVersion := TemplateCompileSourceVersion(spec.TemplateID, spec.TemplatePinnedRef)
	slots := templateSlotRegistrySlots(spec)
	return TemplateSlotMap{
		SchemaVersion:          defaultSchemaVersion,
		TemplateID:             spec.TemplateID,
		TemplateSubjectVersion: templateSubjectVersion,
		Slots:                  slots,
	}
}

func templateSlotRegistrySlots(spec domainSpec) []TemplateSlot {
	if strings.TrimSpace(spec.Kind) == "protocol-client" {
		return flutterProtocolClientTemplateSlots()
	}
	templateID := strings.TrimSpace(spec.TemplateID)
	switch templateID {
	case "flutter-finance-lite":
		return flutterFinanceLiteTemplateSlots()
	case "", "flutter-open-lite":
		return flutterOpenLiteTemplateSlots()
	default:
		return flutterOpenLiteTemplateSlots()
	}
}

func flutterProtocolClientTemplateSlots() []TemplateSlot {
	return []TemplateSlot{
		{BindingID: "protocol-models", SlotID: templatePrivateSlotID("protocol-lite", "models"), SlotKind: "models", TargetPaths: []string{"lib/models/protocol_models.dart"}, OverridePolicy: "replace", RequiredInputs: []string{"domain-model.json"}, AcceptanceImpacts: []string{"ac-protocol-models"}, EmitEligible: true},
		{BindingID: "protocol-services", SlotID: templatePrivateSlotID("protocol-lite", "services"), SlotKind: "services", TargetPaths: []string{"lib/services/api_client.dart", "lib/services/ws_client.dart", "lib/services/settings_store.dart"}, OverridePolicy: "replace", RequiredInputs: []string{"domain-model.json", "task-allocation.json"}, AcceptanceImpacts: []string{"ac-rest-client", "ac-websocket-client", "ac-settings"}, EmitEligible: true},
		{BindingID: "protocol-state", SlotID: templatePrivateSlotID("protocol-lite", "state"), SlotKind: "state", TargetPaths: []string{"lib/providers/connection_provider.dart", "lib/providers/project_provider.dart", "lib/providers/conversation_provider.dart", "lib/providers/file_provider.dart"}, OverridePolicy: "replace", RequiredInputs: []string{"domain-model.json", "task-allocation.json"}, AcceptanceImpacts: []string{"ac-state-management"}, EmitEligible: true},
		{BindingID: "protocol-app-entry", SlotID: templatePrivateSlotID("protocol-lite", "app-entry"), SlotKind: "app_entry", TargetPaths: []string{"lib/main.dart", "lib/app.dart", "pubspec.yaml", "android/app/build.gradle.kts", "android/app/src/main/AndroidManifest.xml", "android/app/src/main/kotlin/com/appfactory/onepilot/MainActivity.kt", "android/app/src/main/res/values/strings.xml"}, OverridePolicy: "replace", RequiredInputs: []string{"planning-context.json", "domain-model.json"}, AcceptanceImpacts: []string{"ac-entry", "ac-platform-network"}, EmitEligible: true},
		{BindingID: "protocol-surfaces", SlotID: templatePrivateSlotID("protocol-lite", "surfaces"), SlotKind: "screens", TargetPaths: []string{"lib/screens/home.dart", "lib/screens/conversations/list_page.dart", "lib/screens/conversations/detail_page.dart", "lib/screens/files/browser_page.dart", "lib/screens/files/file_preview_page.dart", "lib/screens/settings/connection_page.dart", "lib/widgets/project_drawer.dart", "lib/widgets/chat_bubble.dart", "lib/widgets/thinking_block.dart", "lib/widgets/connection_indicator.dart", "lib/widgets/file_tree_tile.dart"}, OverridePolicy: "replace", RequiredInputs: []string{"domain-model.json", "task-allocation.json"}, AcceptanceImpacts: []string{"ac-navigation", "ac-conversations", "ac-files"}, EmitEligible: true},
		{BindingID: "protocol-tests", SlotID: templatePrivateSlotID("protocol-lite", "tests"), SlotKind: "test", TargetPaths: []string{"test/widget_test.dart"}, OverridePolicy: "replace", RequiredInputs: []string{"acceptance-plan.json", "domain-model.json"}, AcceptanceImpacts: []string{"ac-fake-protocol"}, EmitEligible: true},
	}
}

func templatePrivateSlotID(templateKey, slotKey string) string {
	return templateKey + "-" + slotKey + "-slot"
}

func flutterFinanceLiteTemplateSlots() []TemplateSlot {
	return []TemplateSlot{
		{
			BindingID:         genericSurfaceOverviewID,
			SlotID:            templatePrivateSlotID("finance-lite", "overview"),
			SlotKind:          "summary",
			TargetPaths:       []string{"lib/views/home_page.dart", "lib/controllers/home_controller.dart", "lib/models/summary.dart"},
			OverridePolicy:    "replace",
			RequiredInputs:    []string{"domain-model.json", "planning-context.json"},
			AcceptanceImpacts: []string{"ac-home", "ac-navigation"},
		},
		{
			BindingID:         genericSurfaceCollectionID,
			SlotID:            templatePrivateSlotID("finance-lite", "collection"),
			SlotKind:          "list",
			TargetPaths:       []string{"lib/views/entry_list_page.dart", "lib/controllers/entry_list_controller.dart"},
			OverridePolicy:    "replace",
			RequiredInputs:    []string{"domain-model.json", "task-allocation.json"},
			AcceptanceImpacts: []string{"ac-ledger"},
		},
		{
			BindingID:         genericSurfaceMutationID,
			SlotID:            templatePrivateSlotID("finance-lite", "mutation"),
			SlotKind:          "form",
			TargetPaths:       []string{"lib/views/entry_form_page.dart", "lib/controllers/entry_form_controller.dart"},
			OverridePolicy:    "replace",
			RequiredInputs:    []string{"domain-model.json", "task-allocation.json"},
			AcceptanceImpacts: []string{"ac-entry"},
		},
		{
			BindingID:         publicBindingAppEntry,
			SlotID:            templatePrivateSlotID("finance-lite", "app-entry"),
			SlotKind:          "app_entry",
			TargetPaths:       []string{"lib/main.dart"},
			OverridePolicy:    "replace",
			RequiredInputs:    []string{"planning-context.json", "domain-model.json"},
			AcceptanceImpacts: []string{"ac-home", "ac-navigation"},
			EmitEligible:      true,
		},
		{
			BindingID:         publicBindingDomainCopy,
			SlotID:            templatePrivateSlotID("finance-lite", "domain-copy"),
			SlotKind:          "copy",
			TargetPaths:       []string{"lib/views/home_page.dart", "lib/views/entry_form_page.dart", "lib/views/entry_list_page.dart"},
			OverridePolicy:    "synchronize",
			RequiredInputs:    []string{"planning-context.json", "domain-model.json"},
			AcceptanceImpacts: []string{"mrp-copy"},
			EmitEligible:      true,
		},
		{
			BindingID:         publicBindingBranding,
			SlotID:            templatePrivateSlotID("finance-lite", "branding"),
			SlotKind:          "branding",
			TargetPaths:       []string{"android/app/src/main/res/values/strings.xml", "android/app/build.gradle.kts"},
			OverridePolicy:    "synchronize",
			RequiredInputs:    []string{"planning-context.json", "domain-model.json"},
			AcceptanceImpacts: []string{"check-counter-demo-removed"},
			EmitEligible:      true,
		},
		{
			BindingID:         publicBindingStorage,
			SlotID:            templatePrivateSlotID("finance-lite", "storage"),
			SlotKind:          "storage",
			TargetPaths:       []string{"lib/repositories/entry_repository.dart", "pubspec.yaml"},
			OverridePolicy:    "extend",
			RequiredInputs:    []string{"domain-model.json", "task-allocation.json"},
			AcceptanceImpacts: []string{"ac-persistence"},
			EmitEligible:      true,
		},
		{
			BindingID:         publicBindingWidgetTest,
			SlotID:            templatePrivateSlotID("finance-lite", "widget-test"),
			SlotKind:          "test",
			TargetPaths:       []string{"test/widget_test.dart"},
			OverridePolicy:    "synchronize",
			RequiredInputs:    []string{"acceptance-plan.json", "task-allocation.json"},
			AcceptanceImpacts: []string{"ac-entry", "ac-ledger", "ac-navigation", "check-counter-demo-removed"},
			EmitEligible:      true,
		},
	}
}

func flutterOpenLiteTemplateSlots() []TemplateSlot {
	return []TemplateSlot{
		{
			BindingID:         genericSurfaceOverviewID,
			SlotID:            templatePrivateSlotID("open-lite", "overview"),
			SlotKind:          "summary",
			TargetPaths:       []string{"lib/views/home_page.dart", "lib/controllers/home_controller.dart", "lib/models/dashboard_summary.dart"},
			OverridePolicy:    "replace",
			RequiredInputs:    []string{"domain-model.json", "planning-context.json"},
			AcceptanceImpacts: []string{"ac-overview", "ac-navigation"},
			EmitEligible:      true,
		},
		{
			BindingID:         genericSurfaceCollectionID,
			SlotID:            templatePrivateSlotID("open-lite", "collection"),
			SlotKind:          "list",
			TargetPaths:       []string{"lib/views/record_list_page.dart", "lib/controllers/record_list_controller.dart"},
			OverridePolicy:    "replace",
			RequiredInputs:    []string{"domain-model.json", "task-allocation.json"},
			AcceptanceImpacts: []string{"ac-list", "ac-detail"},
			EmitEligible:      true,
		},
		{
			BindingID:         genericSurfaceMutationID,
			SlotID:            templatePrivateSlotID("open-lite", "mutation"),
			SlotKind:          "form",
			TargetPaths:       []string{"lib/views/record_form_page.dart", "lib/controllers/record_form_controller.dart"},
			OverridePolicy:    "replace",
			RequiredInputs:    []string{"domain-model.json", "task-allocation.json"},
			AcceptanceImpacts: []string{"ac-form"},
			EmitEligible:      true,
		},
		{
			BindingID:         genericSurfaceInspectionID,
			SlotID:            templatePrivateSlotID("open-lite", "inspection"),
			SlotKind:          "detail",
			TargetPaths:       []string{"lib/views/record_detail_page.dart"},
			OverridePolicy:    "replace",
			RequiredInputs:    []string{"domain-model.json", "task-allocation.json"},
			AcceptanceImpacts: []string{"ac-detail", "ac-delete"},
			EmitEligible:      true,
		},
		{
			BindingID:         publicBindingAppEntry,
			SlotID:            templatePrivateSlotID("open-lite", "app-entry"),
			SlotKind:          "app_entry",
			TargetPaths:       []string{"lib/main.dart"},
			OverridePolicy:    "replace",
			RequiredInputs:    []string{"planning-context.json", "domain-model.json"},
			AcceptanceImpacts: []string{"ac-navigation"},
			EmitEligible:      true,
		},
		{
			BindingID:         publicBindingDomainCopy,
			SlotID:            templatePrivateSlotID("open-lite", "domain-copy"),
			SlotKind:          "copy",
			TargetPaths:       []string{"lib/template/open_lite_copy.dart", "lib/views/home_page.dart", "lib/views/record_form_page.dart", "lib/views/record_detail_page.dart", "lib/views/record_list_page.dart"},
			OverridePolicy:    "synchronize",
			RequiredInputs:    []string{"planning-context.json", "domain-model.json"},
			AcceptanceImpacts: []string{"mrp-domain-wording", "check-profile-open-lite-domain-branding", "check-profile-open-lite-domain-language"},
			EmitEligible:      true,
		},
		{
			BindingID:         publicBindingBranding,
			SlotID:            templatePrivateSlotID("open-lite", "branding"),
			SlotKind:          "branding",
			TargetPaths:       []string{"android/app/src/main/res/values/strings.xml", "android/app/build.gradle.kts"},
			OverridePolicy:    "synchronize",
			RequiredInputs:    []string{"planning-context.json", "domain-model.json"},
			AcceptanceImpacts: []string{"check-profile-open-lite-domain-branding"},
			EmitEligible:      true,
		},
		{
			BindingID:         publicBindingStorage,
			SlotID:            templatePrivateSlotID("open-lite", "storage"),
			SlotKind:          "storage",
			TargetPaths:       []string{"lib/repositories/record_repository.dart", "pubspec.yaml"},
			OverridePolicy:    "extend",
			RequiredInputs:    []string{"domain-model.json", "task-allocation.json"},
			AcceptanceImpacts: []string{"ac-persistence", "check-open-lite-local-persistence-wiring"},
			EmitEligible:      true,
		},
		{
			BindingID:         publicBindingWidgetTest,
			SlotID:            templatePrivateSlotID("open-lite", "widget-test"),
			SlotKind:          "test",
			TargetPaths:       []string{"test/widget_test.dart"},
			OverridePolicy:    "synchronize",
			RequiredInputs:    []string{"acceptance-plan.json", "task-allocation.json"},
			AcceptanceImpacts: []string{"check-open-lite-counter-demo-removed", "check-open-lite-record-flow-wiring", "check-profile-open-lite-domain-language"},
			EmitEligible:      true,
		},
	}
}

func buildTaskAllocation(tasks []appruns.TaskBundleItem, spec domainSpec) TaskAllocation {
	units := make([]TaskAllocationUnit, 0, len(tasks))
	waves := computeAllocationWaves(tasks)
	surfaceList := normalizeInteractionSurfaces(spec.SurfaceList, spec.ScreenList)
	surfaceByScreen := surfaceIDByLegacyScreenRef(surfaceList)
	knownSurfaceIDs := surfaceIDSet(surfaceList)
	for _, task := range tasks {
		transition := task.AllocationTransition
		if transition == nil {
			transition = &appruns.TaskAllocationTransition{}
		}
		allocationID := strings.TrimSpace(transition.AllocationID)
		if allocationID == "" {
			allocationID = strings.TrimSpace(task.TaskID)
		}
		surfaceRefs := append([]string(nil), transition.SurfaceRefs...)
		surfaceRefs = append(surfaceRefs, deriveSurfaceRefsFromLegacyScreens(transition.ScreenRefs, surfaceByScreen)...)
		surfaceRefs = normalizeSurfaceRefsForSchema(surfaceRefs, knownSurfaceIDs)
		behaviorRefs := deriveAllocationBehaviorRefs(spec, surfaceRefs, transition.EntityRefs, task.RelatedRequirements, task.TargetPaths)
		units = append(units, TaskAllocationUnit{
			AllocationID:        allocationID,
			Title:               strings.TrimSpace(task.Title),
			Wave:                waves[allocationID],
			Lane:                string(task.Category),
			Priority:            strings.TrimSpace(task.Priority),
			BindingRefs:         deriveAllocationBindingRefs(task),
			SurfaceRefs:         surfaceRefs,
			EntityRefs:          append([]string(nil), transition.EntityRefs...),
			RelationGroupRefs:   append([]string(nil), transition.RelationGroupRefs...),
			SharedOwnershipRefs: append([]string(nil), transition.SharedOwnershipRefs...),
			CapabilityFlags:     deriveAllocationCapabilityFlags(spec, surfaceRefs, task.TargetPaths, behaviorRefs),
			BehaviorRefs:        behaviorRefs,
			TaskType:            task.EffectiveTaskType(),
			Objective:           strings.TrimSpace(task.Objective),
			SemanticIntentRefs:  append([]string(nil), transition.SemanticIntentRefs...),
			RelatedRequirements: append([]string(nil), task.RelatedRequirements...),
			TargetPaths:         append([]string(nil), task.TargetPaths...),
			OwnedPaths:          append([]string(nil), transition.OwnedPaths...),
			BlockedBy:           append([]string(nil), transition.BlockedBy...),
			SuccessEvidence:     append([]string(nil), transition.SuccessEvidence...),
			OutputExpectations:  append([]string(nil), task.OutputExpectations...),
			RiskNotes:           append([]string(nil), task.RiskNotes...),
			RiskLevel:           task.RiskLevel,
			RouteHint:           task.RouteHint,
		})
	}
	return TaskAllocation{
		SchemaVersion: defaultSchemaVersion,
		JobID:         spec.JobID,
		PRDID:         spec.PRDID,
		TemplateID:    spec.TemplateID,
		Units:         units,
	}
}

func deriveAllocationBehaviorRefs(spec domainSpec, surfaceRefs, entityRefs, acceptanceRefs, targetPaths []string) []string {
	refs := make([]string, 0, len(spec.BehaviorRules))
	for _, rule := range spec.BehaviorRules {
		if behaviorRuleMatches(rule, surfaceRefs, entityRefs, acceptanceRefs, targetPaths) {
			refs = append(refs, rule.RuleID)
		}
	}
	return uniqueStrings(refs)
}

func behaviorRuleMatches(rule DomainBehaviorRule, surfaceRefs, entityRefs, acceptanceRefs, targetPaths []string) bool {
	if intersectsStrings(rule.SurfaceRefs, surfaceRefs) || intersectsStrings(rule.AcceptanceRefs, acceptanceRefs) {
		return true
	}
	if intersectsStrings(rule.EntityRefs, entityRefs) && targetPathsContain(targetPaths, "/models/") {
		return true
	}
	if strings.EqualFold(strings.TrimSpace(rule.Kind), "persist") {
		return targetPathsContain(targetPaths, "/repositories/")
	}
	return false
}

func targetPathsContain(targetPaths []string, fragment string) bool {
	for _, path := range targetPaths {
		if strings.Contains(filepath.ToSlash(strings.TrimSpace(path)), fragment) {
			return true
		}
	}
	return false
}

func deriveAllocationCapabilityFlags(spec domainSpec, surfaceRefs, targetPaths, behaviorRefs []string) []string {
	capabilities := make([]string, 0, len(spec.CapabilityFlags))
	for _, behaviorRef := range behaviorRefs {
		if rule, ok := behaviorRuleByID(spec.BehaviorRules, behaviorRef); ok {
			capabilities = append(capabilities, rule.CapabilityRefs...)
		}
	}
	for _, surfaceRef := range surfaceRefs {
		switch surfaceRef {
		case genericSurfaceOverviewID:
			capabilities = append(capabilities, "summary-card")
		case genericSurfaceCollectionID:
			capabilities = append(capabilities, "list")
		case genericSurfaceMutationID:
			capabilities = append(capabilities, "form")
			if strings.TrimSpace(spec.Kind) == "generic" {
				capabilities = append(capabilities, "create-record", "edit-record")
			}
		case genericSurfaceInspectionID:
			capabilities = append(capabilities, "detail")
		}
	}
	for _, path := range targetPaths {
		normalizedPath := filepath.ToSlash(strings.TrimSpace(path))
		switch {
		case normalizedPath == "lib/main.dart":
			capabilities = append(capabilities, "navigation")
		case strings.Contains(normalizedPath, "/repositories/"):
			capabilities = append(capabilities, "local-storage")
		case strings.Contains(normalizedPath, "/models/"):
			capabilities = append(capabilities, "domain-model")
		case normalizedPath == "test/widget_test.dart":
			capabilities = append(capabilities, "acceptance-test")
		}
	}
	return uniqueStrings(capabilities)
}

func behaviorRuleByID(rules []DomainBehaviorRule, ruleID string) (DomainBehaviorRule, bool) {
	trimmedID := strings.TrimSpace(ruleID)
	for _, rule := range rules {
		if strings.TrimSpace(rule.RuleID) == trimmedID {
			return rule, true
		}
	}
	return DomainBehaviorRule{}, false
}

func intersectsStrings(left, right []string) bool {
	if len(left) == 0 || len(right) == 0 {
		return false
	}
	seen := make(map[string]struct{}, len(left))
	for _, value := range left {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			seen[trimmed] = struct{}{}
		}
	}
	for _, value := range right {
		if _, ok := seen[strings.TrimSpace(value)]; ok {
			return true
		}
	}
	return false
}

func buildAcceptancePlan(spec domainSpec) AcceptancePlan {
	acceptanceBindingRefs := buildAcceptanceBindingRefs(spec)
	semanticRules := buildSemanticAcceptanceRules(spec)
	semanticChecks := make([]AcceptancePlanItem, 0, len(semanticRules)+len(spec.ManualReviewPoints))
	for _, rule := range semanticRules {
		behaviorRefs := deriveAcceptanceBehaviorRefs(spec, strings.TrimSpace(rule.RuleID), nil, rule.FieldRefs)
		commands := []string(nil)
		if strings.TrimSpace(rule.EvidencePattern) != "" {
			commands = []string{buildSemanticEvidenceCommand(rule.EvidencePattern)}
		}
		semanticChecks = append(semanticChecks, AcceptancePlanItem{
			CheckID:         strings.TrimSpace(rule.RuleID),
			Label:           strings.TrimSpace(rule.Label),
			Description:     strings.TrimSpace(rule.Description),
			Stage:           appruns.StageCheap,
			Commands:        commands,
			Required:        true,
			SourceType:      "semantic_acceptance_rule",
			SourceRef:       strings.TrimSpace(rule.RuleID),
			BindingRefs:     cloneStringSlice(acceptanceBindingRefs[strings.TrimSpace(rule.RuleID)]),
			CapabilityRefs:  capabilityRefsForBehaviorRefs(spec.BehaviorRules, behaviorRefs),
			BehaviorRefs:    behaviorRefs,
			FieldRefs:       append([]string(nil), rule.FieldRefs...),
			EvidencePattern: strings.TrimSpace(rule.EvidencePattern),
		})
	}
	for _, point := range spec.ManualReviewPoints {
		behaviorRefs := deriveAcceptanceBehaviorRefs(spec, strings.TrimSpace(point.PointID), nil, nil)
		semanticChecks = append(semanticChecks, AcceptancePlanItem{
			CheckID:        strings.TrimSpace(point.PointID),
			Label:          strings.TrimSpace(point.Summary),
			Description:    strings.TrimSpace(point.Reason),
			Required:       true,
			SourceType:     "manual_review_point",
			SourceRef:      strings.TrimSpace(point.PointID),
			BindingRefs:    cloneStringSlice(acceptanceBindingRefs[strings.TrimSpace(point.PointID)]),
			CapabilityRefs: capabilityRefsForBehaviorRefs(spec.BehaviorRules, behaviorRefs),
			BehaviorRefs:   behaviorRefs,
		})
	}
	behaviorChecks := make([]AcceptancePlanItem, 0, len(spec.UserFlows))
	for _, flow := range spec.UserFlows {
		behaviorRefs := deriveAcceptanceBehaviorRefs(spec, strings.TrimSpace(flow.FlowID), flowSurfaceRefs(flow), nil)
		behaviorChecks = append(behaviorChecks, AcceptancePlanItem{
			CheckID:        strings.TrimSpace(flow.FlowID),
			Label:          strings.TrimSpace(flow.Title),
			Description:    summarizeFlow(flow),
			Required:       true,
			SourceType:     "user_flow",
			SourceRef:      strings.TrimSpace(flow.FlowID),
			BindingRefs:    cloneStringSlice(acceptanceBindingRefs[strings.TrimSpace(flow.FlowID)]),
			CapabilityRefs: capabilityRefsForBehaviorRefs(spec.BehaviorRules, behaviorRefs),
			BehaviorRefs:   behaviorRefs,
		})
	}
	structureChecks := make([]AcceptancePlanItem, 0, len(spec.AcceptanceChecks))
	deliveryChecks := make([]AcceptancePlanItem, 0, len(spec.AcceptanceChecks))
	for _, check := range spec.AcceptanceChecks {
		checkID := strings.TrimSpace(check.CheckID)
		behaviorRefs := deriveAcceptanceBehaviorRefs(spec, checkID, nil, nil)
		item := AcceptancePlanItem{
			CheckID:        checkID,
			Label:          strings.TrimSpace(check.Label),
			Description:    strings.TrimSpace(check.SuccessCriteria),
			Stage:          check.Stage,
			Commands:       append([]string(nil), check.Commands...),
			Required:       check.Required,
			SourceType:     "acceptance_check",
			SourceRef:      checkID,
			BindingRefs:    cloneStringSlice(acceptanceBindingRefs[checkID]),
			CapabilityRefs: capabilityRefsForBehaviorRefs(spec.BehaviorRules, behaviorRefs),
			BehaviorRefs:   behaviorRefs,
			TimeoutSeconds: check.TimeoutSeconds,
		}
		if isDeliveryAcceptanceCheck(check) {
			deliveryChecks = append(deliveryChecks, item)
			continue
		}
		structureChecks = append(structureChecks, item)
	}
	return AcceptancePlan{
		SchemaVersion:   defaultSchemaVersion,
		JobID:           spec.JobID,
		PRDID:           spec.PRDID,
		TemplateID:      spec.TemplateID,
		StructureChecks: structureChecks,
		SemanticChecks:  semanticChecks,
		BehaviorChecks:  behaviorChecks,
		DeliveryChecks:  deliveryChecks,
	}
}

func deriveAcceptanceBehaviorRefs(spec domainSpec, acceptanceRef string, surfaceRefs, fieldRefs []string) []string {
	refs := make([]string, 0, len(spec.BehaviorRules))
	for _, rule := range spec.BehaviorRules {
		if strings.TrimSpace(acceptanceRef) != "" && containsTrimmedString(rule.AcceptanceRefs, acceptanceRef) {
			refs = append(refs, rule.RuleID)
			continue
		}
		if intersectsStrings(rule.SurfaceRefs, surfaceRefs) {
			refs = append(refs, rule.RuleID)
		}
	}
	return uniqueStrings(refs)
}

func capabilityRefsForBehaviorRefs(rules []DomainBehaviorRule, behaviorRefs []string) []string {
	capabilities := make([]string, 0, len(behaviorRefs))
	for _, behaviorRef := range behaviorRefs {
		if rule, ok := behaviorRuleByID(rules, behaviorRef); ok {
			capabilities = append(capabilities, rule.CapabilityRefs...)
		}
	}
	return uniqueStrings(capabilities)
}

func flowSurfaceRefs(flow UserFlow) []string {
	refs := make([]string, 0, len(flow.Steps))
	for _, step := range flow.Steps {
		refs = append(refs, step.SurfaceRef)
	}
	return uniqueStrings(refs)
}

func containsTrimmedString(values []string, want string) bool {
	trimmedWant := strings.TrimSpace(want)
	for _, value := range values {
		if strings.TrimSpace(value) == trimmedWant {
			return true
		}
	}
	return false
}

func buildSemanticEvidenceCommand(pattern string) string {
	trimmed := strings.TrimSpace(pattern)
	if trimmed == "" {
		return ""
	}
	return fmt.Sprintf("paths=''; [ -d lib ] && paths=\"$paths lib\"; [ -d test ] && paths=\"$paths test\"; [ -f android/app/src/main/res/values/strings.xml ] && paths=\"$paths android/app/src/main/res/values/strings.xml\"; [ -n \"$paths\" ] && grep -ER '%s' $paths >/dev/null 2>&1", trimmed)
}

func buildSemanticAcceptanceRules(spec domainSpec) []SemanticAcceptanceRule {
	primaryFieldRefs, summaryFieldRefs := semanticAcceptanceRuleFieldSets(spec)
	rules := make([]SemanticAcceptanceRule, 0, len(spec.AcceptanceCriteria))
	for _, criterion := range spec.AcceptanceCriteria {
		fieldRefs := deriveSemanticRuleFieldRefs(criterion, primaryFieldRefs, summaryFieldRefs)
		rules = append(rules, SemanticAcceptanceRule{
			RuleID:          strings.TrimSpace(criterion.CriterionID),
			Label:           strings.TrimSpace(criterion.Label),
			Description:     strings.TrimSpace(criterion.Description),
			FieldRefs:       fieldRefs,
			EvidencePattern: buildSemanticEvidencePattern(spec.Title, fieldRefs),
		})
	}
	return rules
}

func semanticAcceptanceRuleFieldSets(spec domainSpec) ([]string, []string) {
	primary := make([]string, 0)
	summary := make([]string, 0)
	for _, entity := range spec.DataEntities {
		target := &primary
		if strings.EqualFold(strings.TrimSpace(entity.Source), "derived") {
			target = &summary
		}
		for _, field := range entity.Fields {
			name := strings.TrimSpace(field.Name)
			if name == "" {
				continue
			}
			*target = append(*target, name)
		}
	}
	return trimStringSlice(primary), trimStringSlice(summary)
}

func deriveSemanticRuleFieldRefs(criterion AcceptanceCriterion, primaryFieldRefs, summaryFieldRefs []string) []string {
	text := strings.ToLower(strings.Join([]string{criterion.CriterionID, criterion.Label, criterion.Description}, " "))
	switch {
	case strings.Contains(text, "overview"), strings.Contains(text, "summary"), strings.Contains(text, "首页"), strings.Contains(text, "摘要"), strings.Contains(text, "概览"):
		return append([]string(nil), summaryFieldRefs...)
	case strings.Contains(text, "navigation"), strings.Contains(text, "导航"):
		return nil
	default:
		return append([]string(nil), primaryFieldRefs...)
	}
}

func buildSemanticEvidencePattern(domainName string, fieldRefs []string) string {
	tokens := make([]string, 0, len(fieldRefs)+1)
	if trimmed := strings.TrimSpace(domainName); trimmed != "" {
		tokens = append(tokens, regexp.QuoteMeta(trimmed))
	}
	for _, fieldRef := range fieldRefs {
		for _, alias := range semanticEvidenceAliases(fieldRef) {
			tokens = append(tokens, regexp.QuoteMeta(alias))
		}
	}
	return strings.Join(trimStringSlice(tokens), "|")
}

type taskIntentBindingRef struct {
	lane        string
	bindingRefs []string
}

func buildAcceptanceBindingRefs(spec domainSpec) map[string][]string {
	surfaceList := normalizeInteractionSurfaces(spec.SurfaceList, spec.ScreenList)
	surfaceBindings := buildSurfaceBindingRefs(spec, surfaceList)
	refs := buildFeatureAcceptanceBindingRefs(spec.FeatureList, surfaceList, surfaceBindings)
	for flowID, bindingRefs := range buildFlowAcceptanceBindingRefs(spec.UserFlows, surfaceList, surfaceBindings) {
		refs[flowID] = bindingRefs
	}
	taskIntentBindings := buildTaskIntentBindingRefs(spec)
	for _, point := range spec.ManualReviewPoints {
		pointID := strings.TrimSpace(point.PointID)
		if pointID == "" {
			continue
		}
		refs[pointID] = manualReviewBindingRefs(pointID, taskIntentBindings[pointID])
	}
	for _, check := range spec.AcceptanceChecks {
		checkID := strings.TrimSpace(check.CheckID)
		if checkID == "" {
			continue
		}
		refs[checkID] = taskIntentBindingRefs(taskIntentBindings[checkID])
	}
	for ref, bindingRefs := range refs {
		refs[ref] = uniqueStrings(bindingRefs)
	}
	return refs
}

func normalizePreparedTaskBundle(taskBundle []appruns.TaskBundleItem) []appruns.TaskBundleItem {
	if len(taskBundle) == 0 {
		return nil
	}
	normalized := make([]appruns.TaskBundleItem, 0, len(taskBundle))
	for _, task := range taskBundle {
		normalized = append(normalized, normalizePreparedTaskBundleItem(task))
	}
	return normalized
}

func normalizePreparedTaskBundleItem(task appruns.TaskBundleItem) appruns.TaskBundleItem {
	normalized := appruns.NormalizeTaskBundleItem(task)
	if normalized.AllocationTransition == nil {
		return normalized
	}
	normalized.AllocationTransition.BindingRefs = declaredTransitionBindingRefs(normalized)
	return normalized
}

func declaredTransitionBindingRefs(task appruns.TaskBundleItem) []string {
	if task.AllocationTransition == nil {
		return nil
	}
	refs := cloneStringSlice(task.AllocationTransition.BindingRefs)
	preferredSurfaceRefs := trimStringSlice(task.AllocationTransition.SurfaceRefs)
	for _, path := range task.TargetPaths {
		refs = append(refs, bindingRefsForTargetPath(path, preferredSurfaceRefs)...)
	}
	if len(refs) == 0 {
		refs = append(refs, preferredSurfaceRefs...)
	}
	return uniqueStrings(refs)
}

func buildSurfaceBindingRefs(spec domainSpec, surfaceList []InteractionSurface) map[string][]string {
	features := normalizeFeatureSurfaceRefs(spec.FeatureList, surfaceList)
	acceptanceRefsBySurface := make(map[string][]string, len(surfaceList))
	for _, feature := range features {
		for _, surfaceRef := range feature.RelatedSurfaceRefs {
			surfaceRef = strings.TrimSpace(surfaceRef)
			if surfaceRef == "" {
				continue
			}
			acceptanceRefsBySurface[surfaceRef] = append(acceptanceRefsBySurface[surfaceRef], feature.AcceptanceRefs...)
		}
	}
	slotMap := buildTemplateSlotMap(spec)
	refs := make(map[string][]string, len(surfaceList))
	for _, surface := range surfaceList {
		surfaceID := strings.TrimSpace(surface.SurfaceID)
		if surfaceID == "" {
			continue
		}
		bindingRef := normalizeBindingRef(publicTemplateBindingRef(selectSurfaceTemplateBinding(surfaceID, uniqueStrings(acceptanceRefsBySurface[surfaceID]), slotMap)))
		refs[surfaceID] = uniqueStrings([]string{bindingRef})
	}
	return refs
}

func buildFeatureAcceptanceBindingRefs(features []Feature, surfaceList []InteractionSurface, surfaceBindings map[string][]string) map[string][]string {
	normalizedFeatures := normalizeFeatureSurfaceRefs(features, surfaceList)
	refs := map[string][]string{}
	for _, feature := range normalizedFeatures {
		bindingRefs := collectBindingRefsForSurfaceRefs(feature.RelatedSurfaceRefs, surfaceBindings)
		for _, acceptanceRef := range feature.AcceptanceRefs {
			acceptanceRef = strings.TrimSpace(acceptanceRef)
			if acceptanceRef == "" {
				continue
			}
			refs[acceptanceRef] = append(refs[acceptanceRef], bindingRefs...)
		}
	}
	for acceptanceRef, bindingRefs := range refs {
		refs[acceptanceRef] = uniqueStrings(bindingRefs)
	}
	return refs
}

func buildFlowAcceptanceBindingRefs(flows []UserFlow, surfaceList []InteractionSurface, surfaceBindings map[string][]string) map[string][]string {
	normalizedFlows := normalizeUserFlowsForSurfaces(flows, surfaceList)
	refs := map[string][]string{}
	for _, flow := range normalizedFlows {
		flowID := strings.TrimSpace(flow.FlowID)
		if flowID == "" {
			continue
		}
		bindingRefs := make([]string, 0, len(flow.Steps))
		for _, step := range flow.Steps {
			surfaceRef := strings.TrimSpace(step.SurfaceRef)
			if surfaceRef == "" {
				continue
			}
			bindingRefs = append(bindingRefs, surfaceBindings[surfaceRef]...)
		}
		refs[flowID] = uniqueStrings(bindingRefs)
	}
	return refs
}

func buildTaskIntentBindingRefs(spec domainSpec) map[string][]taskIntentBindingRef {
	taskBundle := normalizePreparedTaskBundle(spec.TaskBundle)
	refs := map[string][]taskIntentBindingRef{}
	for _, task := range taskBundle {
		if task.AllocationTransition == nil {
			continue
		}
		bindingRefs := uniqueStrings(task.AllocationTransition.BindingRefs)
		if len(bindingRefs) == 0 {
			continue
		}
		for _, semanticIntentRef := range task.AllocationTransition.SemanticIntentRefs {
			semanticIntentRef = strings.TrimSpace(semanticIntentRef)
			if semanticIntentRef == "" {
				continue
			}
			refs[semanticIntentRef] = append(refs[semanticIntentRef], taskIntentBindingRef{lane: strings.TrimSpace(string(task.Category)), bindingRefs: cloneStringSlice(bindingRefs)})
		}
	}
	return refs
}

func collectBindingRefsForSurfaceRefs(surfaceRefs []string, surfaceBindings map[string][]string) []string {
	refs := make([]string, 0, len(surfaceRefs))
	for _, surfaceRef := range surfaceRefs {
		surfaceRef = strings.TrimSpace(surfaceRef)
		if surfaceRef == "" {
			continue
		}
		refs = append(refs, surfaceBindings[surfaceRef]...)
	}
	return uniqueStrings(refs)
}

func taskIntentBindingRefs(taskRefs []taskIntentBindingRef) []string {
	refs := make([]string, 0, len(taskRefs))
	for _, taskRef := range taskRefs {
		refs = append(refs, taskRef.bindingRefs...)
	}
	return uniqueStrings(refs)
}

func manualReviewBindingRefs(pointID string, taskRefs []taskIntentBindingRef) []string {
	_ = pointID
	return taskIntentBindingRefs(taskRefs)
}

func semanticEvidenceAliases(fieldRef string) []string {
	trimmed := strings.TrimSpace(fieldRef)
	if trimmed == "" {
		return nil
	}
	aliases := []string{trimmed}
	camel := snakeToLowerCamel(trimmed)
	if camel != "" && camel != trimmed {
		aliases = append(aliases, camel)
	}
	if strings.HasSuffix(trimmed, "_total") {
		base := strings.TrimSuffix(trimmed, "_total")
		if base != "" {
			aliases = append(aliases, base)
		}
	}
	return trimStringSlice(aliases)
}

func snakeToLowerCamel(value string) string {
	parts := strings.Split(strings.TrimSpace(value), "_")
	if len(parts) == 0 {
		return ""
	}
	for index, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		lower := strings.ToLower(part)
		if index == 0 {
			parts[index] = lower
			continue
		}
		parts[index] = strings.ToUpper(lower[:1]) + lower[1:]
	}
	return strings.Join(parts, "")
}

func normalizeBindingRef(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	return value
}

func cloneStringSlice(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	return append([]string(nil), values...)
}

func summarizeFlow(flow UserFlow) string {
	parts := make([]string, 0, len(flow.Steps))
	for _, step := range flow.Steps {
		if title := strings.TrimSpace(step.Title); title != "" {
			parts = append(parts, title)
		}
	}
	return strings.Join(parts, " -> ")
}

func isDeliveryAcceptanceCheck(check appruns.AcceptanceCheck) bool {
	parts := append([]string{check.CheckID, check.Label, check.SuccessCriteria}, check.Commands...)
	text := strings.ToLower(strings.Join(parts, " "))
	return strings.Contains(text, "apk") || strings.Contains(text, "device") || strings.Contains(text, "review") || strings.Contains(text, "install")
}

func computeAllocationWaves(tasks []appruns.TaskBundleItem) map[string]int {
	waves := make(map[string]int, len(tasks))
	dependencies := make(map[string][]string, len(tasks))
	for _, task := range tasks {
		allocationID := strings.TrimSpace(task.TaskID)
		if task.AllocationTransition != nil && strings.TrimSpace(task.AllocationTransition.AllocationID) != "" {
			allocationID = strings.TrimSpace(task.AllocationTransition.AllocationID)
		}
		dependencies[allocationID] = append([]string(nil), task.Dependencies...)
	}
	for _, task := range tasks {
		allocationID := strings.TrimSpace(task.TaskID)
		if task.AllocationTransition != nil && strings.TrimSpace(task.AllocationTransition.AllocationID) != "" {
			allocationID = strings.TrimSpace(task.AllocationTransition.AllocationID)
		}
		waves[allocationID] = resolveAllocationWave(allocationID, dependencies, waves, map[string]bool{})
	}
	return waves
}

func resolveAllocationWave(allocationID string, dependencies map[string][]string, waves map[string]int, visiting map[string]bool) int {
	if wave, ok := waves[allocationID]; ok && wave > 0 {
		return wave
	}
	if visiting[allocationID] {
		return 0
	}
	visiting[allocationID] = true
	maxWave := 0
	for _, dep := range dependencies[allocationID] {
		dep = strings.TrimSpace(dep)
		if dep == "" {
			continue
		}
		depWave := resolveAllocationWave(dep, dependencies, waves, visiting) + 1
		if depWave > maxWave {
			maxWave = depWave
		}
	}
	delete(visiting, allocationID)
	waves[allocationID] = maxWave
	return maxWave
}

func deriveAllocationBindingRefs(task appruns.TaskBundleItem) []string {
	refs := make([]string, 0, len(task.TargetPaths))
	preferredSurfaceRefs := []string(nil)
	explicitBindingRefs := []string(nil)
	if task.AllocationTransition != nil {
		preferredSurfaceRefs = trimStringSlice(task.AllocationTransition.SurfaceRefs)
		explicitBindingRefs = trimStringSlice(task.AllocationTransition.BindingRefs)
	}
	appendRef := func(ref string) {
		if trimmed := strings.TrimSpace(ref); trimmed != "" {
			refs = append(refs, trimmed)
		}
	}
	for _, path := range task.TargetPaths {
		for _, ref := range bindingRefsForTargetPath(path, preferredSurfaceRefs) {
			appendRef(ref)
		}
	}
	for _, ref := range explicitBindingRefs {
		appendRef(ref)
	}
	return uniqueStrings(refs)
}

func bindingRefsForTargetPath(path string, preferredSurfaceRefs []string) []string {
	normalized := filepath.ToSlash(strings.TrimSpace(path))
	switch {
	case normalized == "lib/main.dart":
		return []string{publicBindingAppEntry}
	case strings.HasPrefix(normalized, "lib/template/") && strings.HasSuffix(normalized, ".dart"):
		return []string{publicBindingDomainCopy}
	case normalized == "android/app/src/main/res/values/strings.xml", normalized == "android/app/build.gradle.kts":
		return []string{publicBindingBranding}
	case strings.Contains(normalized, "lib/models/"):
		return []string{publicBindingDomainModel}
	case strings.Contains(normalized, "lib/repositories/"):
		return []string{publicBindingStorage}
	case strings.Contains(normalized, "lib/views/"):
		if len(preferredSurfaceRefs) > 0 {
			return append([]string(nil), preferredSurfaceRefs...)
		}
		return []string{publicBindingSurface}
	case strings.Contains(normalized, "lib/controllers/"):
		return []string{publicBindingFlow}
	case strings.Contains(normalized, "test/widget_test.dart"):
		return []string{publicBindingWidgetTest}
	case normalized == "pubspec.yaml":
		return []string{publicBindingManifest}
	default:
		return nil
	}
}
func trimStringSlice(items []string) []string {
	if len(items) == 0 {
		return nil
	}
	result := make([]string, 0, len(items))
	for _, item := range items {
		if trimmed := strings.TrimSpace(item); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func currentPlanningPolicySnapshot() appruns.PlanningPolicySnapshot {
	return appruns.PlanningPolicySnapshot{
		PolicyVersion: "phase1-boundary-v1",
		Stages: []appruns.PlanningStagePolicy{
			{Stage: appruns.PlanningStageRequirementStructuring, Route: appruns.PlanningStageRoutePlanningModel, Rationale: "需求整理允许强模型参与，但输出必须收敛到受控 PRD 结构。"},
			{Stage: appruns.PlanningStageDomainModeling, Route: appruns.PlanningStageRoutePlanningModel, Rationale: "领域对象抽取属于高不确定性语义建模，默认走 planning_model。"},
			{Stage: appruns.PlanningStageTaskAllocation, Route: appruns.PlanningStageRouteDecisionModel, Rationale: "任务拆解、依赖裁决和 route hint 分配默认走 decision_model。"},
			{Stage: appruns.PlanningStageAcceptancePlanning, Route: appruns.PlanningStageRouteDecisionModel, Rationale: "验收边界与冲突消解属于裁决层，不交给自由执行层。"},
			{Stage: appruns.PlanningStageBuildInputProjection, Route: appruns.PlanningStageRouteDeterministic, Rationale: "builder-input 只是运行时投影包，默认禁止自由生成模型参与。"},
		},
	}
}
