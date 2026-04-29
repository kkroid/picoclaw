package prepare

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestExecutionContractUnmarshalAcceptsSurfaceFirstSchema(t *testing.T) {
	var contract ExecutionContract
	err := json.Unmarshal([]byte(`{
		"contract_version": "0.1.0",
		"domain_model": {
			"domain_name": "待办事项"
		},
		"surface_relation_schema": {
			"anchor_collection_path": "surface_list",
			"anchor_id_path": "surface_list[].surface_id",
			"primary_ref_fields": ["surface_ref", "surface_refs"],
			"allowed_references": [
				{
					"source_path": "user_flows[].steps[].surface_ref",
					"target_path": "surface_list[].surface_id",
					"cardinality": "one"
				}
			]
		},
		"surface_contracts": [
			{
				"contract_id": "contract-overview",
				"surface_ref": "surface-overview",
				"template_binding_ref": "surface-overview",
				"purpose": "展示概览摘要",
				"required_states": [
					{
						"state_id": "state-overview",
						"label": "概览可见",
						"evidence": "显示摘要"
					}
				]
			}
		],
		"key_flows": [
			{
				"flow_id": "flow-open-overview",
				"title": "查看概览",
				"entry_surface_ref": "surface-overview",
				"acceptance_refs": ["ac-overview"],
				"steps": [
					{
						"step_id": "step-open-overview",
						"surface_ref": "surface-overview",
						"action": "打开概览",
						"expected_result": "显示摘要"
					}
				]
			}
		],
		"task_projection": [
			{
				"task_id": "task-bind-overview",
				"title": "绑定概览承载单元",
				"wave": 0,
				"lane": "screen",
				"task_type": "dual_file_wiring",
				"surface_refs": ["surface-overview"],
				"target_paths": ["lib/views/home_page.dart"]
			}
		]
	}`), &contract)
	if err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if len(contract.SurfaceContracts) != 1 || contract.SurfaceContracts[0].SurfaceRef != "surface-overview" {
		t.Fatalf("surface_contracts = %+v, want surface-overview contract", contract.SurfaceContracts)
	}
	if len(contract.SurfaceContracts[0].RequiredStates) != 1 || contract.SurfaceContracts[0].RequiredStates[0].StateID != "state-overview" {
		t.Fatalf("required_states = %+v, want overview required state kept", contract.SurfaceContracts[0].RequiredStates)
	}
	if len(contract.KeyFlows) != 1 || contract.KeyFlows[0].EntrySurfaceRef != "surface-overview" {
		t.Fatalf("key_flows = %+v, want entry_surface_ref kept", contract.KeyFlows)
	}
	if len(contract.KeyFlows[0].AcceptanceRefs) != 1 || contract.KeyFlows[0].AcceptanceRefs[0] != "ac-overview" {
		t.Fatalf("key_flows acceptance_refs = %+v, want ac-overview kept", contract.KeyFlows)
	}
	if contract.SurfaceRelationSchema.AnchorCollectionPath != "surface_list" {
		t.Fatalf("surface_relation_schema = %+v, want anchor_collection_path kept", contract.SurfaceRelationSchema)
	}
	if len(contract.SurfaceRelationSchema.AllowedReferences) != 1 || contract.SurfaceRelationSchema.AllowedReferences[0].SourcePath != "user_flows[].steps[].surface_ref" {
		t.Fatalf("allowed_references = %+v, want surface relation schema kept", contract.SurfaceRelationSchema.AllowedReferences)
	}
	if len(contract.TaskProjection) != 1 || len(contract.TaskProjection[0].SurfaceRefs) != 1 || contract.TaskProjection[0].SurfaceRefs[0] != "surface-overview" {
		t.Fatalf("task_projection = %+v, want surface-first projection", contract.TaskProjection)
	}
}

func TestExecutionContractUnmarshalRejectsLegacySurfaceRelationCompatibilityFields(t *testing.T) {
	var contract ExecutionContract
	err := json.Unmarshal([]byte(`{
		"contract_version": "0.1.0",
		"domain_model": {
			"domain_name": "待办事项"
		},
		"surface_relation_schema": {
			"anchor_collection_path": "surface_list",
			"anchor_id_path": "surface_list[].surface_id",
			"compatibility_fields": [
				{
					"field_path": "user_flows[].steps[].screen_ref",
					"canonical_path": "user_flows[].steps[].surface_ref",
					"policy": "read_only_input"
				}
			]
		}
	}`), &contract)
	if err == nil || !strings.Contains(err.Error(), `compatibility_fields`) {
		t.Fatalf("Unmarshal() error = %v, want unknown compatibility_fields", err)
	}
}

func TestExecutionContractUnmarshalRejectsLegacyPageContracts(t *testing.T) {
	var contract ExecutionContract
	err := json.Unmarshal([]byte(`{
		"contract_version": "0.1.0",
		"domain_model": {
			"domain_name": "待办事项"
		},
		"page_contracts": [
			{
				"contract_id": "contract-overview",
				"surface_ref": "surface-overview",
				"purpose": "展示概览摘要"
			}
		]
	}`), &contract)
	if err == nil || !strings.Contains(err.Error(), `page_contracts`) {
		t.Fatalf("Unmarshal() error = %v, want unknown page_contracts", err)
	}
}

func TestExecutionContractUnmarshalRejectsLegacySurfaceContractFields(t *testing.T) {
	var contract ExecutionContract
	err := json.Unmarshal([]byte(`{
		"contract_version": "0.1.0",
		"domain_model": {
			"domain_name": "待办事项"
		},
		"surface_contracts": [
			{
				"contract_id": "contract-overview",
				"screen_ref": "screen-home",
				"slot_ref": "home-summary",
				"purpose": "展示概览摘要"
			}
		]
	}`), &contract)
	if err == nil || !strings.Contains(err.Error(), `screen_ref`) {
		t.Fatalf("Unmarshal() error = %v, want unknown screen_ref", err)
	}
}

func TestExecutionContractUnmarshalRejectsLegacyKeyFlowFields(t *testing.T) {
	var contract ExecutionContract
	err := json.Unmarshal([]byte(`{
		"contract_version": "0.1.0",
		"domain_model": {
			"domain_name": "待办事项"
		},
		"key_flows": [
			{
				"flow_id": "flow-open-overview",
				"title": "查看概览",
				"entry_screen_ref": "screen-home",
				"steps": [
					{
						"step_id": "step-open-overview",
						"screen_ref": "screen-home",
						"action": "打开概览",
						"expected_result": "显示摘要"
					}
				]
			}
		]
	}`), &contract)
	if err == nil || !strings.Contains(err.Error(), `entry_screen_ref`) {
		t.Fatalf("Unmarshal() error = %v, want unknown entry_screen_ref", err)
	}
}

func TestExecutionContractUnmarshalRejectsLegacyTaskProjectionScreenRefs(t *testing.T) {
	var contract ExecutionContract
	err := json.Unmarshal([]byte(`{
		"contract_version": "0.1.0",
		"domain_model": {
			"domain_name": "待办事项"
		},
		"task_projection": [
			{
				"task_id": "task-bind-overview",
				"title": "绑定概览承载单元",
				"wave": 0,
				"lane": "screen",
				"task_type": "dual_file_wiring",
				"screen_refs": ["screen-home"],
				"target_paths": ["lib/views/home_page.dart"]
			}
		]
	}`), &contract)
	if err == nil || !strings.Contains(err.Error(), `screen_refs`) {
		t.Fatalf("Unmarshal() error = %v, want unknown screen_refs", err)
	}
}
