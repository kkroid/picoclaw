package prepare

import (
	"reflect"
	"testing"
)

func assertSurfaceRelationSchema(t *testing.T, contract *ExecutionContract) {
	t.Helper()
	if contract == nil {
		t.Fatal("execution contract = nil, want surface relation schema")
	}
	want := SurfaceRelationSchema{
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
	if !reflect.DeepEqual(contract.SurfaceRelationSchema, want) {
		t.Fatalf("surface_relation_schema = %+v, want %+v", contract.SurfaceRelationSchema, want)
	}
}