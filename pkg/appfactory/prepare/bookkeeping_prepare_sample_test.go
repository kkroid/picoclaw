package prepare

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

const updateBookkeepingPrepareSamplesEnv = "PICOCLAW_UPDATE_BOOKKEEPING_PREPARE_SAMPLES"
const updateBookkeepingGoldensEnv = "PICOCLAW_UPDATE_BOOKKEEPING_GOLDENS"

func TestCompileBookkeepingBuilderInputGolden(t *testing.T) {
	t.Parallel()

	bundle := compileBookkeepingPrepareSampleBundle(t)
	got, err := normalizeBuildInputForGolden(bundle.BuilderInput)
	if err != nil {
		t.Fatalf("normalizeBuildInputForGolden() error = %v", err)
	}
	goldenPath := filepath.Join("testdata", "bookkeeping_examples", "bookkeeping.builder-input.golden.json")
	if os.Getenv(updateBookkeepingGoldensEnv) == "1" {
		if err := os.MkdirAll(filepath.Dir(goldenPath), 0o755); err != nil {
			t.Fatalf("MkdirAll(%s) error = %v", filepath.Dir(goldenPath), err)
		}
		if err := os.WriteFile(goldenPath, got, 0o644); err != nil {
			t.Fatalf("WriteFile(%s) error = %v", goldenPath, err)
		}
	}
	want, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", goldenPath, err)
	}
	want = normalizeJSONForGolden(want)
	var gotDecoded any
	if err := json.Unmarshal(got, &gotDecoded); err != nil {
		t.Fatalf("Unmarshal(got) error = %v", err)
	}
	var wantDecoded any
	if err := json.Unmarshal(want, &wantDecoded); err != nil {
		t.Fatalf("Unmarshal(want) error = %v", err)
	}
	if !reflect.DeepEqual(gotDecoded, wantDecoded) {
		t.Fatalf("bookkeeping builder input golden mismatch\nwant:\n%s\n\ngot:\n%s", want, got)
	}
}

func TestBookkeepingPrepareSampleMatchesExampleBundle(t *testing.T) {
	t.Parallel()

	bundle := compileBookkeepingPrepareSampleBundle(t)
	sampleDir := filepath.Join("..", "..", "..", "examples", "appfactory", "bookkeeping", "prepare-sample")
	if os.Getenv(updateBookkeepingPrepareSamplesEnv) == "1" {
		if err := os.RemoveAll(sampleDir); err != nil {
			t.Fatalf("RemoveAll(%s) error = %v", sampleDir, err)
		}
		if err := WriteBundle(sampleDir, bundle); err != nil {
			t.Fatalf("WriteBundle(%s) error = %v", sampleDir, err)
		}
	}
	assertPrepareSampleMatchesBundle(t, sampleDir, bundle)
}

func compileBookkeepingPrepareSampleBundle(t *testing.T) Bundle {
	t.Helper()

	requestPath := filepath.Join("..", "..", "..", "examples", "appfactory", "bookkeeping", "compile-request.sample.json")
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
			return time.Date(2026, 4, 11, 11, 0, 0, 0, time.UTC)
		},
	})
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	return bundle
}