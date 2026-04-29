package prepare

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
	"time"
)

const updateGenericPrepareSamplesEnv = "PICOCLAW_UPDATE_PREPARE_SAMPLES"

type genericPrepareSampleExpectation struct {
	Name     string
	FixedNow time.Time
}

type genericCompileRequestSample struct {
	RequirementText   string `json:"requirement_text"`
	RequirementSource string `json:"requirement_source"`
	Title             string `json:"title"`
	JobID             string `json:"job_id"`
	PRDID             string `json:"prd_id"`
	RealChecks        bool   `json:"real_checks"`
	ExecutorImage     string `json:"executor_image"`
}

func TestGenericPrepareSamplesMatchExampleBundles(t *testing.T) {
	t.Parallel()

	cases := []genericPrepareSampleExpectation{
		{
			Name:     "weight-tracker",
			FixedNow: time.Date(2026, 4, 10, 12, 30, 0, 0, time.UTC),
		},
		{
			Name:     "todo-lite-no-home-no-detail",
			FixedNow: time.Date(2026, 4, 11, 9, 0, 0, 0, time.UTC),
		},
		{
			Name:     "weight-tracker-detail-without-delete",
			FixedNow: time.Date(2026, 4, 11, 9, 15, 0, 0, time.UTC),
		},
		{
			Name:     "habit-checkin-drawer-entry",
			FixedNow: time.Date(2026, 4, 11, 9, 30, 0, 0, time.UTC),
		},
	}
	updateSamples := os.Getenv(updateGenericPrepareSamplesEnv) == "1"

	for _, tc := range cases {
		tc := tc
		t.Run(tc.Name, func(t *testing.T) {
			t.Parallel()

			bundle := compileGenericPrepareSampleBundle(t, tc.Name, tc.FixedNow)
			sampleDir := filepath.Join("..", "..", "..", "examples", "appfactory", "generic", tc.Name, "prepare-sample")
			if updateSamples {
				if err := os.RemoveAll(sampleDir); err != nil {
					t.Fatalf("RemoveAll(%s) error = %v", sampleDir, err)
				}
				if err := WriteBundle(sampleDir, bundle); err != nil {
					t.Fatalf("WriteBundle(%s) error = %v", sampleDir, err)
				}
			}
			assertPrepareSampleMatchesBundle(t, sampleDir, bundle)
		})
	}
}

func assertPrepareSampleMatchesBundle(t *testing.T, sampleDir string, bundle Bundle) {
	t.Helper()

	wantFiles := []string{
		requirementFileName,
		prdMarkdownFileName,
		prdJSONFileName,
		prdApprovalFileName,
		templateApprovalFileName,
		fitReportFileName,
		planFileName,
		constraintsFileName,
		planningContextFileName,
		domainModelFileName,
		templateSlotMapFileName,
		taskAllocationFileName,
		acceptancePlanFileName,
		runtimeConfigFileName,
		builderInputFileName,
	}
	sort.Strings(wantFiles)

	entries, err := os.ReadDir(sampleDir)
	if err != nil {
		t.Fatalf("ReadDir(%s) error = %v", sampleDir, err)
	}
	gotNames := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		gotNames = append(gotNames, entry.Name())
	}
	sort.Strings(gotNames)
	if !reflect.DeepEqual(gotNames, wantFiles) {
		t.Fatalf("sample file list = %v, want %v", gotNames, wantFiles)
	}
	for _, name := range wantFiles {
		want, err := os.ReadFile(filepath.Join(sampleDir, name))
		if err != nil {
			t.Fatalf("ReadFile(%s) error = %v", name, err)
		}
		got := bundle.Files[name]
		if filepath.Ext(name) == ".json" {
			want = normalizeJSONForGolden(want)
			got = normalizeJSONForGolden(got)
		}
		if string(got) != string(want) {
			t.Fatalf("sample bundle mismatch for %s\nwant:\n%s\n\ngot:\n%s", name, string(want), string(got))
		}
	}
}

func compileGenericPrepareSampleBundle(t *testing.T, name string, fixedNow time.Time) Bundle {
	t.Helper()

	requestPath := filepath.Join("..", "..", "..", "examples", "appfactory", "generic", name, "compile-request.sample.json")
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
			return fixedNow
		},
	})
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	return bundle
}
