package adapter

import (
	"strings"
	"testing"

	appruns "github.com/sipeed/oneappfactory/pkg/appfactory/runs"
)

func TestDraftGenericOpenLitePolicyRegistryValidates(t *testing.T) {
	registry := DraftGenericOpenLitePolicyRegistry()
	if err := registry.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestDraftGenericOpenLitePolicyRegistryCoversCoreBindings(t *testing.T) {
	registry := DraftGenericOpenLitePolicyRegistry()
	covered := make(map[string]struct{})
	for _, entry := range registry.Entries {
		for _, ref := range entry.Match.BindingRefs {
			covered[ref] = struct{}{}
		}
	}
	for _, required := range []string{"surface-overview", "surface-collection", "surface-mutation", "surface-inspection", "app-entry", "domain-copy", "android-branding", "widget-test"} {
		if _, ok := covered[required]; !ok {
			t.Fatalf("registry missing binding %q in %#v", required, covered)
		}
	}
}

func TestDraftGenericOpenLitePolicyRegistryExcludesModelRequestFailureRepair(t *testing.T) {
	registry := DraftGenericOpenLitePolicyRegistry()
	for _, entry := range registry.Entries {
		for _, class := range entry.Match.FailureClasses {
			if class == RuntimePolicyFailureClassModelRequestFailure {
				t.Fatalf("entry %q should not route model_request_failure as code repair", entry.PolicyID)
			}
		}
	}
}

func TestRuntimePolicyRegistryValidateRejectsPrimaryModeMismatch(t *testing.T) {
	registry := RuntimePolicyRegistry{
		Version: "test-v1",
		Entries: []RuntimePolicyEntry{{
			PolicyID: "bad-emit-policy",
			Match: RuntimePolicyMatch{
				BindingRefs:    []string{"surface-overview"},
				FailureClasses: []RuntimePolicyFailureClass{RuntimePolicyFailureClassAnalyze},
				TaskTypes:      []appruns.BuilderRuntimeTaskType{appruns.BuilderRuntimeTaskTypeDualFileWiring},
			},
			Decision: RuntimePolicyDecision{
				OverridePolicy:         RuntimeOverridePolicyEmit,
				PrimaryGenerationMode:  RuntimePolicyGenerationModeModelGenerate,
				AllowedGenerationModes: []RuntimePolicyGenerationMode{RuntimePolicyGenerationModeModelGenerate},
			},
		}},
	}
	err := registry.Validate()
	if err == nil || !strings.Contains(err.Error(), "override_policy emit") {
		t.Fatalf("Validate() error = %v, want override_policy emit mismatch", err)
	}
}
