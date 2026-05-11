package emitter

import (
	"strings"
	"testing"

	appprepare "github.com/sipeed/oneappfactory/pkg/appfactory/prepare"
)

func TestEmitRelationModelsProjectTaskTag(t *testing.T) {
	result, ok := EmitRelationModels(appprepare.DomainModel{
		Entities: []appprepare.DataEntity{
			{EntityID: "entity-project", Name: "Project"},
			{EntityID: "entity-task", Name: "Task"},
			{EntityID: "entity-tag", Name: "Tag"},
		},
	})
	if !ok {
		t.Fatal("EmitRelationModels returned false for project-task-tag domain model")
	}
	if len(result.Files) != 5 {
		t.Fatalf("len(Files) = %d, want 5", len(result.Files))
	}
	for path, markers := range map[string][]string{
		"lib/models/project.dart":           {"enum ProjectStatus {", "factory Project.fromJson(Map<String, dynamic> json)"},
		"lib/models/task.dart":              {"enum TaskStatus {", "'due_on': dueOn?.toIso8601String()"},
		"lib/models/tag.dart":               {"class Tag {", "Map<String, dynamic> toJson()"},
		"lib/models/task_tag_link.dart":     {"class TaskTagLink {", "factory TaskTagLink.fromJson(Map<String, dynamic> json)"},
		"lib/models/dashboard_summary.dart": {"final int doneTaskCount;", "'tagged_task_count': taggedTaskCount"},
	} {
		content := result.Files[path]
		if strings.TrimSpace(content) == "" {
			t.Fatalf("missing emitted content for %s", path)
		}
		for _, marker := range markers {
			if !strings.Contains(content, marker) {
				t.Fatalf("content for %s missing %q:\n%s", path, marker, content)
			}
		}
	}
}

func TestEmitRelationModelsInventory(t *testing.T) {
	result, ok := EmitRelationModels(appprepare.DomainModel{
		Entities: []appprepare.DataEntity{
			{EntityID: "entity-inventory-sheet", Name: "InventorySheet"},
			{EntityID: "entity-line-item", Name: "LineItem"},
			{EntityID: "entity-sku", Name: "Sku"},
		},
	})
	if !ok {
		t.Fatal("EmitRelationModels returned false for inventory domain model")
	}
	if len(result.Files) != 5 {
		t.Fatalf("len(Files) = %d, want 5", len(result.Files))
	}
	for path, markers := range map[string][]string{
		"lib/models/inventory_sheet.dart":   {"enum InventorySheetStatus {", "'counted_on': countedOn.toIso8601String()"},
		"lib/models/line_item.dart":         {"class LineItem {", "'variance_qty': varianceQty"},
		"lib/models/sku.dart":               {"class Sku {", "'reorder_threshold': reorderThreshold"},
		"lib/models/warehouse.dart":         {"class Warehouse {", "'location': location"},
		"lib/models/dashboard_summary.dart": {"final int lowStockSkuCount;", "'variance_line_item_count': varianceLineItemCount"},
	} {
		content := result.Files[path]
		if strings.TrimSpace(content) == "" {
			t.Fatalf("missing emitted content for %s", path)
		}
		for _, marker := range markers {
			if !strings.Contains(content, marker) {
				t.Fatalf("content for %s missing %q:\n%s", path, marker, content)
			}
		}
	}
}

func TestEmitRelationModelsRejectsGenericDomainModel(t *testing.T) {
	if _, ok := EmitRelationModels(appprepare.DomainModel{Entities: []appprepare.DataEntity{{EntityID: "entity-weight-record", Name: "WeightRecord"}}}); ok {
		t.Fatal("EmitRelationModels returned true for generic domain model")
	}
}
