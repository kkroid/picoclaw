package adapter

import (
	"fmt"
	"strings"

	appruns "github.com/sipeed/oneappfactory/pkg/appfactory/runs"
)

type RuntimeOverridePolicy string

const (
	RuntimeOverridePolicyEmit     RuntimeOverridePolicy = "emit"
	RuntimeOverridePolicyGenerate RuntimeOverridePolicy = "generate"
	RuntimeOverridePolicyManual   RuntimeOverridePolicy = "manual"
)

type RuntimePolicyGenerationMode string

const (
	RuntimePolicyGenerationModeDeterministicEmit     RuntimePolicyGenerationMode = "deterministic_emit"
	RuntimePolicyGenerationModeModelGenerate         RuntimePolicyGenerationMode = "model_generate"
	RuntimePolicyGenerationModeTemplatePrivateRepair RuntimePolicyGenerationMode = "template_private_repair"
	RuntimePolicyGenerationModeValidationRepair      RuntimePolicyGenerationMode = "validation_repair"
	RuntimePolicyGenerationModeManualReview          RuntimePolicyGenerationMode = "manual_review"
)

type RuntimePolicyFallbackMode string

const (
	RuntimePolicyFallbackModeNone                  RuntimePolicyFallbackMode = "none"
	RuntimePolicyFallbackModeUpgradeModel          RuntimePolicyFallbackMode = "upgrade_model"
	RuntimePolicyFallbackModeTemplatePrivateRepair RuntimePolicyFallbackMode = "template_private_repair"
	RuntimePolicyFallbackModeValidationRepair      RuntimePolicyFallbackMode = "validation_repair"
	RuntimePolicyFallbackModeManualReview          RuntimePolicyFallbackMode = "manual_review"
)

type RuntimePolicyFailureClass string

const (
	RuntimePolicyFailureClassSchemaDrift         RuntimePolicyFailureClass = "schema_drift"
	RuntimePolicyFailureClassPatchParse          RuntimePolicyFailureClass = "patch_parse"
	RuntimePolicyFailureClassAnalyze             RuntimePolicyFailureClass = "analyze"
	RuntimePolicyFailureClassTest                RuntimePolicyFailureClass = "test"
	RuntimePolicyFailureClassClosure             RuntimePolicyFailureClass = "closure"
	RuntimePolicyFailureClassScopeViolation      RuntimePolicyFailureClass = "scope_violation"
	RuntimePolicyFailureClassSemanticConflict    RuntimePolicyFailureClass = "semantic_conflict"
	RuntimePolicyFailureClassModelRequestFailure RuntimePolicyFailureClass = "model_request_failure"
)

type RuntimePolicyPathClass string

const (
	RuntimePolicyPathClassAppEntry        RuntimePolicyPathClass = "app_entry"
	RuntimePolicyPathClassView            RuntimePolicyPathClass = "view"
	RuntimePolicyPathClassController      RuntimePolicyPathClass = "controller"
	RuntimePolicyPathClassModel           RuntimePolicyPathClass = "model"
	RuntimePolicyPathClassRepository      RuntimePolicyPathClass = "repository"
	RuntimePolicyPathClassTemplateCopy    RuntimePolicyPathClass = "template_copy"
	RuntimePolicyPathClassWidgetTest      RuntimePolicyPathClass = "widget_test"
	RuntimePolicyPathClassAndroidBranding RuntimePolicyPathClass = "android_branding"
	RuntimePolicyPathClassManifest        RuntimePolicyPathClass = "manifest"
)

type RuntimePolicyRegistry struct {
	Version string               `json:"version"`
	Entries []RuntimePolicyEntry `json:"entries"`
}

type RuntimePolicyEntry struct {
	PolicyID   string                  `json:"policy_id"`
	Scope      RuntimePolicyScope      `json:"scope,omitempty"`
	Match      RuntimePolicyMatch      `json:"match"`
	Decision   RuntimePolicyDecision   `json:"decision"`
	GuardRails RuntimePolicyGuardRails `json:"guard_rails,omitempty"`
}

type RuntimePolicyScope struct {
	TemplateFamily string `json:"template_family,omitempty"`
	DomainVariant  string `json:"domain_variant,omitempty"`
}

type RuntimePolicyMatch struct {
	BindingRefs    []string                         `json:"binding_refs,omitempty"`
	SurfaceRefs    []string                         `json:"surface_refs,omitempty"`
	PathClasses    []RuntimePolicyPathClass         `json:"path_classes,omitempty"`
	FailureClasses []RuntimePolicyFailureClass      `json:"failure_classes,omitempty"`
	TaskTypes      []appruns.BuilderRuntimeTaskType `json:"task_types,omitempty"`
}

type RuntimePolicyDecision struct {
	OverridePolicy         RuntimeOverridePolicy         `json:"override_policy"`
	PrimaryGenerationMode  RuntimePolicyGenerationMode   `json:"primary_generation_mode"`
	AllowedGenerationModes []RuntimePolicyGenerationMode `json:"allowed_generation_modes,omitempty"`
	FallbackMode           RuntimePolicyFallbackMode     `json:"fallback_mode,omitempty"`
	UpgradeTriggers        RuntimePolicyUpgradeTriggers  `json:"upgrade_triggers,omitempty"`
}

type RuntimePolicyUpgradeTriggers struct {
	FailureClasses             []RuntimePolicyFailureClass `json:"failure_classes,omitempty"`
	MaxAttemptsBeforeUpgrade   int                         `json:"max_attempts_before_upgrade,omitempty"`
	MaxFilesBeforeUpgrade      int                         `json:"max_files_before_upgrade,omitempty"`
	UpgradeOnScopeViolation    bool                        `json:"upgrade_on_scope_violation,omitempty"`
	UpgradeOnSemanticConflict  bool                        `json:"upgrade_on_semantic_conflict,omitempty"`
	UpgradeOnPatchParseFailure bool                        `json:"upgrade_on_patch_parse_failure,omitempty"`
}

type RuntimePolicyGuardRails struct {
	RequiresOwnedPaths                 bool `json:"requires_owned_paths,omitempty"`
	RequiresAllowedPaths               bool `json:"requires_allowed_paths,omitempty"`
	PreserveReferencedHelpers          bool `json:"preserve_referenced_helpers,omitempty"`
	ForbidLegacyScreenRefs             bool `json:"forbid_legacy_screen_refs,omitempty"`
	ForbidTemplatePrivatePathPromotion bool `json:"forbid_template_private_path_promotion,omitempty"`
	AllowPathClassExpansion            bool `json:"allow_path_class_expansion,omitempty"`
}

func (registry RuntimePolicyRegistry) Validate() error {
	if strings.TrimSpace(registry.Version) == "" {
		return fmt.Errorf("version is required")
	}
	if len(registry.Entries) == 0 {
		return fmt.Errorf("entries is required")
	}
	seen := make(map[string]struct{}, len(registry.Entries))
	for index, entry := range registry.Entries {
		policyID := strings.TrimSpace(entry.PolicyID)
		if policyID == "" {
			return fmt.Errorf("entries[%d].policy_id is required", index)
		}
		if _, exists := seen[policyID]; exists {
			return fmt.Errorf("entries[%d].policy_id %q is duplicated", index, policyID)
		}
		seen[policyID] = struct{}{}
		if err := entry.Validate(); err != nil {
			return fmt.Errorf("entries[%d]: %w", index, err)
		}
	}
	return nil
}

func (entry RuntimePolicyEntry) Validate() error {
	if !entry.Match.hasSelector() {
		return fmt.Errorf("at least one match selector is required")
	}
	for _, ref := range entry.Match.BindingRefs {
		if strings.TrimSpace(ref) == "" {
			return fmt.Errorf("binding_refs contains empty value")
		}
	}
	for _, ref := range entry.Match.SurfaceRefs {
		if strings.TrimSpace(ref) == "" {
			return fmt.Errorf("surface_refs contains empty value")
		}
	}
	for _, class := range entry.Match.PathClasses {
		if !class.valid() {
			return fmt.Errorf("path_classes contains unsupported value %q", class)
		}
	}
	for _, class := range entry.Match.FailureClasses {
		if !class.valid() {
			return fmt.Errorf("failure_classes contains unsupported value %q", class)
		}
	}
	for _, taskType := range entry.Match.TaskTypes {
		if appruns.NormalizeBuilderRuntimeTaskType(string(taskType)) == "" {
			return fmt.Errorf("task_types contains unsupported value %q", taskType)
		}
	}
	if err := entry.Decision.Validate(); err != nil {
		return err
	}
	return nil
}

func (match RuntimePolicyMatch) hasSelector() bool {
	return len(match.BindingRefs) > 0 || len(match.SurfaceRefs) > 0 || len(match.PathClasses) > 0 || len(match.FailureClasses) > 0 || len(match.TaskTypes) > 0
}

func (decision RuntimePolicyDecision) Validate() error {
	if !decision.OverridePolicy.valid() {
		return fmt.Errorf("override_policy %q is unsupported", decision.OverridePolicy)
	}
	if !decision.PrimaryGenerationMode.valid() {
		return fmt.Errorf("primary_generation_mode %q is unsupported", decision.PrimaryGenerationMode)
	}
	allowedModes := decision.AllowedGenerationModes
	if len(allowedModes) == 0 {
		allowedModes = []RuntimePolicyGenerationMode{decision.PrimaryGenerationMode}
	}
	for _, mode := range allowedModes {
		if !mode.valid() {
			return fmt.Errorf("allowed_generation_modes contains unsupported value %q", mode)
		}
	}
	if !containsGenerationMode(allowedModes, decision.PrimaryGenerationMode) {
		return fmt.Errorf("primary_generation_mode %q must be included in allowed_generation_modes", decision.PrimaryGenerationMode)
	}
	switch decision.OverridePolicy {
	case RuntimeOverridePolicyEmit:
		if decision.PrimaryGenerationMode != RuntimePolicyGenerationModeDeterministicEmit {
			return fmt.Errorf("override_policy emit must use primary_generation_mode deterministic_emit")
		}
	case RuntimeOverridePolicyGenerate:
		if decision.PrimaryGenerationMode != RuntimePolicyGenerationModeModelGenerate {
			return fmt.Errorf("override_policy generate must use primary_generation_mode model_generate")
		}
	case RuntimeOverridePolicyManual:
		if decision.PrimaryGenerationMode != RuntimePolicyGenerationModeManualReview {
			return fmt.Errorf("override_policy manual must use primary_generation_mode manual_review")
		}
	}
	if strings.TrimSpace(string(decision.FallbackMode)) != "" && !decision.FallbackMode.valid() {
		return fmt.Errorf("fallback_mode %q is unsupported", decision.FallbackMode)
	}
	if err := decision.UpgradeTriggers.Validate(); err != nil {
		return err
	}
	return nil
}

func (triggers RuntimePolicyUpgradeTriggers) Validate() error {
	for _, class := range triggers.FailureClasses {
		if !class.valid() {
			return fmt.Errorf("upgrade_triggers.failure_classes contains unsupported value %q", class)
		}
	}
	if triggers.MaxAttemptsBeforeUpgrade < 0 {
		return fmt.Errorf("upgrade_triggers.max_attempts_before_upgrade must be >= 0")
	}
	if triggers.MaxFilesBeforeUpgrade < 0 {
		return fmt.Errorf("upgrade_triggers.max_files_before_upgrade must be >= 0")
	}
	return nil
}

func (policy RuntimeOverridePolicy) valid() bool {
	switch policy {
	case RuntimeOverridePolicyEmit, RuntimeOverridePolicyGenerate, RuntimeOverridePolicyManual:
		return true
	default:
		return false
	}
}

func (mode RuntimePolicyGenerationMode) valid() bool {
	switch mode {
	case RuntimePolicyGenerationModeDeterministicEmit,
		RuntimePolicyGenerationModeModelGenerate,
		RuntimePolicyGenerationModeTemplatePrivateRepair,
		RuntimePolicyGenerationModeValidationRepair,
		RuntimePolicyGenerationModeManualReview:
		return true
	default:
		return false
	}
}

func (mode RuntimePolicyFallbackMode) valid() bool {
	switch mode {
	case RuntimePolicyFallbackModeNone,
		RuntimePolicyFallbackModeUpgradeModel,
		RuntimePolicyFallbackModeTemplatePrivateRepair,
		RuntimePolicyFallbackModeValidationRepair,
		RuntimePolicyFallbackModeManualReview:
		return true
	default:
		return false
	}
}

func (class RuntimePolicyFailureClass) valid() bool {
	switch class {
	case RuntimePolicyFailureClassSchemaDrift,
		RuntimePolicyFailureClassPatchParse,
		RuntimePolicyFailureClassAnalyze,
		RuntimePolicyFailureClassTest,
		RuntimePolicyFailureClassClosure,
		RuntimePolicyFailureClassScopeViolation,
		RuntimePolicyFailureClassSemanticConflict,
		RuntimePolicyFailureClassModelRequestFailure:
		return true
	default:
		return false
	}
}

func (class RuntimePolicyPathClass) valid() bool {
	switch class {
	case RuntimePolicyPathClassAppEntry,
		RuntimePolicyPathClassView,
		RuntimePolicyPathClassController,
		RuntimePolicyPathClassModel,
		RuntimePolicyPathClassRepository,
		RuntimePolicyPathClassTemplateCopy,
		RuntimePolicyPathClassWidgetTest,
		RuntimePolicyPathClassAndroidBranding,
		RuntimePolicyPathClassManifest:
		return true
	default:
		return false
	}
}

func containsGenerationMode(values []RuntimePolicyGenerationMode, target RuntimePolicyGenerationMode) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func DraftGenericOpenLitePolicyRegistry() RuntimePolicyRegistry {
	guardRails := RuntimePolicyGuardRails{
		RequiresOwnedPaths:                 true,
		RequiresAllowedPaths:               true,
		PreserveReferencedHelpers:          true,
		ForbidLegacyScreenRefs:             true,
		ForbidTemplatePrivatePathPromotion: true,
	}
	sharedRepairFailures := []RuntimePolicyFailureClass{
		RuntimePolicyFailureClassAnalyze,
		RuntimePolicyFailureClassTest,
		RuntimePolicyFailureClassClosure,
		RuntimePolicyFailureClassScopeViolation,
		RuntimePolicyFailureClassSemanticConflict,
	}
	sharedUpgradeFailures := []RuntimePolicyFailureClass{
		RuntimePolicyFailureClassSchemaDrift,
		RuntimePolicyFailureClassPatchParse,
		RuntimePolicyFailureClassScopeViolation,
		RuntimePolicyFailureClassSemanticConflict,
	}
	sharedEmitDecision := RuntimePolicyDecision{
		OverridePolicy:         RuntimeOverridePolicyEmit,
		PrimaryGenerationMode:  RuntimePolicyGenerationModeDeterministicEmit,
		AllowedGenerationModes: []RuntimePolicyGenerationMode{RuntimePolicyGenerationModeDeterministicEmit, RuntimePolicyGenerationModeValidationRepair},
		FallbackMode:           RuntimePolicyFallbackModeValidationRepair,
		UpgradeTriggers: RuntimePolicyUpgradeTriggers{
			FailureClasses:             sharedUpgradeFailures,
			MaxAttemptsBeforeUpgrade:   2,
			MaxFilesBeforeUpgrade:      2,
			UpgradeOnScopeViolation:    true,
			UpgradeOnSemanticConflict:  true,
			UpgradeOnPatchParseFailure: true,
		},
	}
	scope := RuntimePolicyScope{TemplateFamily: "flutter-open-lite", DomainVariant: "generic"}
	return RuntimePolicyRegistry{
		Version: "draft-generic-open-lite-v1",
		Entries: []RuntimePolicyEntry{
			{
				PolicyID: "generic-overview-surface",
				Scope:    scope,
				Match: RuntimePolicyMatch{
					BindingRefs:    []string{"surface-overview"},
					SurfaceRefs:    []string{"surface-overview"},
					PathClasses:    []RuntimePolicyPathClass{RuntimePolicyPathClassView, RuntimePolicyPathClassController, RuntimePolicyPathClassModel},
					FailureClasses: sharedRepairFailures,
					TaskTypes:      []appruns.BuilderRuntimeTaskType{appruns.BuilderRuntimeTaskTypeDualFileWiring, appruns.BuilderRuntimeTaskTypeAnalyzeRepair, appruns.BuilderRuntimeTaskTypeTestRepair},
				},
				Decision:   sharedEmitDecision,
				GuardRails: guardRails,
			},
			{
				PolicyID: "generic-collection-surface",
				Scope:    scope,
				Match: RuntimePolicyMatch{
					BindingRefs:    []string{"surface-collection"},
					SurfaceRefs:    []string{"surface-collection"},
					PathClasses:    []RuntimePolicyPathClass{RuntimePolicyPathClassView, RuntimePolicyPathClassController},
					FailureClasses: sharedRepairFailures,
					TaskTypes:      []appruns.BuilderRuntimeTaskType{appruns.BuilderRuntimeTaskTypeDualFileWiring, appruns.BuilderRuntimeTaskTypeAnalyzeRepair, appruns.BuilderRuntimeTaskTypeTestRepair},
				},
				Decision:   sharedEmitDecision,
				GuardRails: guardRails,
			},
			{
				PolicyID: "generic-mutation-surface",
				Scope:    scope,
				Match: RuntimePolicyMatch{
					BindingRefs:    []string{"surface-mutation"},
					SurfaceRefs:    []string{"surface-mutation"},
					PathClasses:    []RuntimePolicyPathClass{RuntimePolicyPathClassView, RuntimePolicyPathClassController},
					FailureClasses: sharedRepairFailures,
					TaskTypes:      []appruns.BuilderRuntimeTaskType{appruns.BuilderRuntimeTaskTypeDualFileWiring, appruns.BuilderRuntimeTaskTypeAnalyzeRepair, appruns.BuilderRuntimeTaskTypeTestRepair},
				},
				Decision:   sharedEmitDecision,
				GuardRails: guardRails,
			},
			{
				PolicyID: "generic-inspection-surface",
				Scope:    scope,
				Match: RuntimePolicyMatch{
					BindingRefs:    []string{"surface-inspection"},
					SurfaceRefs:    []string{"surface-inspection"},
					PathClasses:    []RuntimePolicyPathClass{RuntimePolicyPathClassView},
					FailureClasses: sharedRepairFailures,
					TaskTypes:      []appruns.BuilderRuntimeTaskType{appruns.BuilderRuntimeTaskTypeDualFileWiring, appruns.BuilderRuntimeTaskTypeAnalyzeRepair, appruns.BuilderRuntimeTaskTypeTestRepair},
				},
				Decision:   sharedEmitDecision,
				GuardRails: guardRails,
			},
			{
				PolicyID: "generic-app-entry",
				Scope:    scope,
				Match: RuntimePolicyMatch{
					BindingRefs:    []string{"app-entry"},
					PathClasses:    []RuntimePolicyPathClass{RuntimePolicyPathClassAppEntry},
					FailureClasses: sharedRepairFailures,
					TaskTypes:      []appruns.BuilderRuntimeTaskType{appruns.BuilderRuntimeTaskTypeDualFileWiring, appruns.BuilderRuntimeTaskTypeAnalyzeRepair, appruns.BuilderRuntimeTaskTypeTestRepair},
				},
				Decision: RuntimePolicyDecision{
					OverridePolicy:         RuntimeOverridePolicyEmit,
					PrimaryGenerationMode:  RuntimePolicyGenerationModeDeterministicEmit,
					AllowedGenerationModes: []RuntimePolicyGenerationMode{RuntimePolicyGenerationModeDeterministicEmit, RuntimePolicyGenerationModeValidationRepair, RuntimePolicyGenerationModeModelGenerate},
					FallbackMode:           RuntimePolicyFallbackModeUpgradeModel,
					UpgradeTriggers: RuntimePolicyUpgradeTriggers{
						FailureClasses:             sharedUpgradeFailures,
						MaxAttemptsBeforeUpgrade:   2,
						MaxFilesBeforeUpgrade:      2,
						UpgradeOnScopeViolation:    true,
						UpgradeOnSemanticConflict:  true,
						UpgradeOnPatchParseFailure: true,
					},
				},
				GuardRails: guardRails,
			},
			{
				PolicyID: "generic-domain-copy",
				Scope:    scope,
				Match: RuntimePolicyMatch{
					BindingRefs:    []string{"domain-copy"},
					PathClasses:    []RuntimePolicyPathClass{RuntimePolicyPathClassTemplateCopy, RuntimePolicyPathClassView},
					FailureClasses: []RuntimePolicyFailureClass{RuntimePolicyFailureClassAnalyze, RuntimePolicyFailureClassTest, RuntimePolicyFailureClassSemanticConflict, RuntimePolicyFailureClassScopeViolation},
					TaskTypes:      []appruns.BuilderRuntimeTaskType{appruns.BuilderRuntimeTaskTypeSingleFileEdit, appruns.BuilderRuntimeTaskTypeDualFileWiring, appruns.BuilderRuntimeTaskTypeAnalyzeRepair, appruns.BuilderRuntimeTaskTypeTestRepair},
				},
				Decision: RuntimePolicyDecision{
					OverridePolicy:         RuntimeOverridePolicyEmit,
					PrimaryGenerationMode:  RuntimePolicyGenerationModeDeterministicEmit,
					AllowedGenerationModes: []RuntimePolicyGenerationMode{RuntimePolicyGenerationModeDeterministicEmit, RuntimePolicyGenerationModeTemplatePrivateRepair, RuntimePolicyGenerationModeValidationRepair},
					FallbackMode:           RuntimePolicyFallbackModeTemplatePrivateRepair,
					UpgradeTriggers: RuntimePolicyUpgradeTriggers{
						FailureClasses:             sharedUpgradeFailures,
						MaxAttemptsBeforeUpgrade:   2,
						MaxFilesBeforeUpgrade:      2,
						UpgradeOnScopeViolation:    true,
						UpgradeOnSemanticConflict:  true,
						UpgradeOnPatchParseFailure: true,
					},
				},
				GuardRails: guardRails,
			},
			{
				PolicyID: "generic-android-branding",
				Scope:    scope,
				Match: RuntimePolicyMatch{
					BindingRefs:    []string{"android-branding"},
					PathClasses:    []RuntimePolicyPathClass{RuntimePolicyPathClassAndroidBranding, RuntimePolicyPathClassManifest},
					FailureClasses: []RuntimePolicyFailureClass{RuntimePolicyFailureClassAnalyze, RuntimePolicyFailureClassTest, RuntimePolicyFailureClassSemanticConflict},
					TaskTypes:      []appruns.BuilderRuntimeTaskType{appruns.BuilderRuntimeTaskTypeSingleFileEdit, appruns.BuilderRuntimeTaskTypeDualFileWiring, appruns.BuilderRuntimeTaskTypeAnalyzeRepair},
				},
				Decision:   sharedEmitDecision,
				GuardRails: guardRails,
			},
			{
				PolicyID: "generic-storage-boundary",
				Scope:    scope,
				Match: RuntimePolicyMatch{
					BindingRefs:    []string{"storage-boundary"},
					PathClasses:    []RuntimePolicyPathClass{RuntimePolicyPathClassRepository, RuntimePolicyPathClassManifest},
					FailureClasses: []RuntimePolicyFailureClass{RuntimePolicyFailureClassAnalyze, RuntimePolicyFailureClassTest, RuntimePolicyFailureClassClosure, RuntimePolicyFailureClassSemanticConflict},
					TaskTypes:      []appruns.BuilderRuntimeTaskType{appruns.BuilderRuntimeTaskTypeSingleFileEdit, appruns.BuilderRuntimeTaskTypeDualFileWiring, appruns.BuilderRuntimeTaskTypeAnalyzeRepair, appruns.BuilderRuntimeTaskTypeClosureRepair},
				},
				Decision: RuntimePolicyDecision{
					OverridePolicy:         RuntimeOverridePolicyGenerate,
					PrimaryGenerationMode:  RuntimePolicyGenerationModeModelGenerate,
					AllowedGenerationModes: []RuntimePolicyGenerationMode{RuntimePolicyGenerationModeModelGenerate, RuntimePolicyGenerationModeTemplatePrivateRepair, RuntimePolicyGenerationModeValidationRepair},
					FallbackMode:           RuntimePolicyFallbackModeTemplatePrivateRepair,
					UpgradeTriggers: RuntimePolicyUpgradeTriggers{
						FailureClasses:             sharedUpgradeFailures,
						MaxAttemptsBeforeUpgrade:   2,
						MaxFilesBeforeUpgrade:      2,
						UpgradeOnScopeViolation:    true,
						UpgradeOnSemanticConflict:  true,
						UpgradeOnPatchParseFailure: true,
					},
				},
				GuardRails: guardRails,
			},
			{
				PolicyID: "generic-widget-test",
				Scope:    scope,
				Match: RuntimePolicyMatch{
					BindingRefs:    []string{"widget-test"},
					PathClasses:    []RuntimePolicyPathClass{RuntimePolicyPathClassWidgetTest},
					FailureClasses: []RuntimePolicyFailureClass{RuntimePolicyFailureClassAnalyze, RuntimePolicyFailureClassTest, RuntimePolicyFailureClassClosure, RuntimePolicyFailureClassSemanticConflict},
					TaskTypes:      []appruns.BuilderRuntimeTaskType{appruns.BuilderRuntimeTaskTypeDualFileWiring, appruns.BuilderRuntimeTaskTypeTestRepair, appruns.BuilderRuntimeTaskTypeAnalyzeRepair},
				},
				Decision: RuntimePolicyDecision{
					OverridePolicy:         RuntimeOverridePolicyEmit,
					PrimaryGenerationMode:  RuntimePolicyGenerationModeDeterministicEmit,
					AllowedGenerationModes: []RuntimePolicyGenerationMode{RuntimePolicyGenerationModeDeterministicEmit, RuntimePolicyGenerationModeValidationRepair},
					FallbackMode:           RuntimePolicyFallbackModeValidationRepair,
					UpgradeTriggers: RuntimePolicyUpgradeTriggers{
						FailureClasses:             sharedUpgradeFailures,
						MaxAttemptsBeforeUpgrade:   2,
						MaxFilesBeforeUpgrade:      2,
						UpgradeOnScopeViolation:    true,
						UpgradeOnSemanticConflict:  true,
						UpgradeOnPatchParseFailure: true,
					},
				},
				GuardRails: guardRails,
			},
		},
	}
}
