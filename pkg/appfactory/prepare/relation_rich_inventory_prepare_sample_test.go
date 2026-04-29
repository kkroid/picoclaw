package prepare

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestInventorySheetLineItemPrepareSampleMatchesExampleBundle(t *testing.T) {
	t.Parallel()

	bundle := compileInventorySheetLineItemPrepareSampleBundle(t)
	sampleDir := filepath.Join("..", "..", "..", "examples", "appfactory", "relation-rich", "inventory-sheet-line-item", "prepare-sample")
	if os.Getenv(updateRelationRichPrepareSamplesEnv) == "1" {
		if err := os.RemoveAll(sampleDir); err != nil {
			t.Fatalf("RemoveAll(%s) error = %v", sampleDir, err)
		}
		if err := WriteBundle(sampleDir, bundle); err != nil {
			t.Fatalf("WriteBundle(%s) error = %v", sampleDir, err)
		}
	}
	assertPrepareSampleMatchesBundle(t, sampleDir, bundle)
}

func compileInventorySheetLineItemPrepareSampleBundle(t *testing.T) Bundle {
	t.Helper()
	requestPath := filepath.Join("..", "..", "..", "examples", "appfactory", "relation-rich", "inventory-sheet-line-item", "compile-request.sample.json")
	requestData, err := os.ReadFile(requestPath)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", requestPath, err)
	}
	var requestSample genericCompileRequestSample
	if err := json.Unmarshal(requestData, &requestSample); err != nil {
		t.Fatalf("Unmarshal(%s) error = %v", requestPath, err)
	}
	bundle, err := Compile(Request{
		RequirementText:   requestSample.RequirementText,
		RequirementSource: requestSample.RequirementSource,
		TitleHint:         requestSample.Title,
		JobID:             requestSample.JobID,
		PRDID:             requestSample.PRDID,
		RealBuild:         requestSample.RealChecks,
		ExecutorImage:     requestSample.ExecutorImage,
		Now: func() time.Time {
			return time.Date(2026, 4, 10, 18, 0, 0, 0, time.UTC)
		},
	})
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	return bundle
}
