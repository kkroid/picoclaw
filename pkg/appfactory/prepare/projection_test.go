package prepare

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	appruns "github.com/sipeed/oneappfactory/pkg/appfactory/runs"
)

func TestProjectBuildInputMatchesDirectCompile(t *testing.T) {
	t.Parallel()

	samples := []struct {
		name      string
		sampleDir string
		buildFunc func(t *testing.T) Bundle
	}{
		{
			name:      "relation-rich/project-task-tag",
			sampleDir: filepath.Join("..", "..", "..", "examples", "appfactory", "relation-rich", "project-task-tag", "prepare-sample"),
			buildFunc: compileRelationRichPrepareSampleBundle,
		},
	}

	for _, sample := range samples {
		sample := sample
		t.Run(sample.name, func(t *testing.T) {
			t.Parallel()
			bundle := sample.buildFunc(t)

			pc := loadArtifact[PlanningContext](t, sample.sampleDir, planningContextFileName)
			dm := loadArtifact[DomainModel](t, sample.sampleDir, domainModelFileName)
			tsm := loadArtifact[TemplateSlotMap](t, sample.sampleDir, templateSlotMapFileName)
			ta := loadArtifact[TaskAllocation](t, sample.sampleDir, taskAllocationFileName)
			ap := loadArtifact[AcceptancePlan](t, sample.sampleDir, acceptancePlanFileName)
			rc := loadArtifact[RuntimeConfig](t, sample.sampleDir, runtimeConfigFileName)

			projected := ProjectBuildInput(pc, dm, tsm, ta, ap, rc)
			direct := bundle.BuilderInput
			scrubNonFunctionalFields(&projected)
			scrubNonFunctionalFields(&direct)

			projectedJSON := mustMarshalJSON(t, projected)
			directJSON := mustMarshalJSON(t, direct)

			projectedNorm := normalizeJSONForGolden(projectedJSON)
			directNorm := normalizeJSONForGolden(directJSON)

			if string(projectedNorm) != string(directNorm) {
				os.WriteFile("/tmp/projected.json", projectedNorm, 0o644)
				os.WriteFile("/tmp/direct.json", directNorm, 0o644)
				t.Fatalf("projected builder-input differs from direct compile; wrote /tmp/projected.json and /tmp/direct.json")
			}
		})
	}
}

func TestProjectBuildInputMatchesGenericSample(t *testing.T) {
	t.Parallel()

	bundle := compileGenericPrepareSampleBundle(t, "weight-tracker", time.Date(2026, 4, 10, 16, 0, 0, 0, time.UTC))
	sampleDir := filepath.Join("..", "..", "..", "examples", "appfactory", "generic", "weight-tracker", "prepare-sample")

	pc := loadArtifact[PlanningContext](t, sampleDir, planningContextFileName)
	dm := loadArtifact[DomainModel](t, sampleDir, domainModelFileName)
	tsm := loadArtifact[TemplateSlotMap](t, sampleDir, templateSlotMapFileName)
	ta := loadArtifact[TaskAllocation](t, sampleDir, taskAllocationFileName)
	ap := loadArtifact[AcceptancePlan](t, sampleDir, acceptancePlanFileName)
	rc := loadArtifact[RuntimeConfig](t, sampleDir, runtimeConfigFileName)

	projected := ProjectBuildInput(pc, dm, tsm, ta, ap, rc)
	direct := bundle.BuilderInput
	scrubNonFunctionalFields(&projected)
	scrubNonFunctionalFields(&direct)

	projectedJSON := mustMarshalJSON(t, projected)
	directJSON := mustMarshalJSON(t, direct)

	projectedNorm := normalizeJSONForGolden(projectedJSON)
	directNorm := normalizeJSONForGolden(directJSON)

	if string(projectedNorm) != string(directNorm) {
		os.WriteFile("/tmp/projected_generic.json", projectedNorm, 0o644)
		os.WriteFile("/tmp/direct_generic.json", directNorm, 0o644)
		t.Fatalf("projected builder-input differs; wrote /tmp/projected_generic.json and /tmp/direct_generic.json")
	}
}

func loadArtifact[T any](t *testing.T, dir, filename string) T {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, filename))
	if err != nil {
		t.Fatalf("ReadFile(%s/%s) error = %v", dir, filename, err)
	}
	var result T
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("Unmarshal(%s) error = %v", filename, err)
	}
	return result
}

func mustMarshalJSON(t *testing.T, v any) []byte {
	t.Helper()
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		t.Fatalf("MarshalIndent error = %v", err)
	}
	return data
}

// scrubNonFunctionalFields 清除投影不保证 byte-exact 的字段：
// - completion_criteria：运行时被 compactBuilderRuntimeTask 丢弃
// - prepared_prd_subject_version：因 task-allocation 内容变化导致 hash 不同
func scrubNonFunctionalFields(bi *appruns.BuildInput) {
	bi.PreparedPRDSubjectVersion = ""
	for i := range bi.TaskBundle {
		bi.TaskBundle[i].CompletionCriteria = nil
	}
}
