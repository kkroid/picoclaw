package runs

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestFileStoreRunLifecycle(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "appfactory")
	store, err := NewFileStore(root)
	if err != nil {
		t.Fatalf("NewFileStore() error = %v", err)
	}
	service := NewService(store)
	run, err := service.Create(ctx, "builder-a", "builder-a", "lease-1", sampleBuildInput())
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if run.PreparedInputDigest == "" {
		t.Fatal("PreparedInputDigest should not be empty")
	}
	if run.Status != StatusRunning {
		t.Fatalf("status = %s, want %s", run.Status, StatusRunning)
	}
	if len(run.KnowledgePack) != 4 || run.KnowledgePack[0].SkillID != "prd-to-task-bundle" {
		t.Fatalf("KnowledgePack = %+v, want Flutter skill pack persisted on run", run.KnowledgePack)
	}
	if _, err := service.Heartbeat(ctx, run.RunID, Heartbeat{Stage: "baseline", Iteration: 1, TotalTokens: 128}); err != nil {
		t.Fatalf("Heartbeat() error = %v", err)
	}
	if _, err := service.IndexArtifacts(ctx, run.RunID, sampleArtifactManifest()); err != nil {
		t.Fatalf("IndexArtifacts() error = %v", err)
	}
	if _, err := service.IndexMetrics(ctx, run.RunID, sampleMetrics()); err != nil {
		t.Fatalf("IndexMetrics() error = %v", err)
	}
	completed, err := service.Complete(ctx, run.RunID, sampleBuildOutput())
	if err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
	if completed.Status != StatusCompleted {
		t.Fatalf("status = %s, want %s", completed.Status, StatusCompleted)
	}
	loaded, err := service.Get(ctx, run.RunID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if loaded.OutputPath == "" || loaded.ArtifactManifestPath == "" || loaded.MetricsPath == "" {
		t.Fatalf("expected output/artifact/metrics paths to be stored, got %+v", loaded)
	}
	metricsData, err := os.ReadFile(filepath.Join(root, "jobs", loaded.JobID, "runs", loaded.RunID, "metrics.json"))
	if err != nil {
		t.Fatalf("ReadFile(metrics) error = %v", err)
	}
	var metrics Metrics
	if err := json.Unmarshal(metricsData, &metrics); err != nil {
		t.Fatalf("Unmarshal(metrics) error = %v", err)
	}
	if len(metrics.DeviceFailureCategories) != 1 || metrics.DeviceFailureCategories[0].Category != "device_check_failed:adb_device_unavailable" {
		t.Fatalf("DeviceFailureCategories = %+v, want adb device unavailable stat", metrics.DeviceFailureCategories)
	}
	outputData, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(loaded.OutputPath)))
	if err != nil {
		t.Fatalf("ReadFile(output) error = %v", err)
	}
	var output BuildOutput
	if err := json.Unmarshal(outputData, &output); err != nil {
		t.Fatalf("Unmarshal(output) error = %v", err)
	}
	if len(output.RoundInputs) != 1 || output.RoundInputs[0].RoundID != "round-1" {
		t.Fatalf("RoundInputs = %+v, want round-1", output.RoundInputs)
	}
	if len(output.RoundOutputs) != 1 || output.RoundOutputs[0].Status == "" {
		t.Fatalf("RoundOutputs = %+v, want one populated round output", output.RoundOutputs)
	}
	if loaded.RunnerScriptPath == "" || loaded.LaunchCommand != "/bin/sh" || len(loaded.LaunchArgs) != 1 {
		t.Fatalf("expected launch spec to be prepared, got %+v", loaded)
	}
	if _, err := service.Cancel(ctx, run.RunID, "should fail"); err != ErrTerminalRun {
		t.Fatalf("Cancel() err = %v, want %v", err, ErrTerminalRun)
	}
}

func TestBuildInputDigestIgnoresRuntimePathAndJSONFormatting(t *testing.T) {
	left := sampleBuildInput()
	right := sampleBuildInput()
	right.ContextSourceDir = "/tmp/prepare"
	right.TemplateSourceDir = "/tmp/template"
	right.WorkspacePath = "/other/workspace/job-1"
	right.ArtifactDir = "/other/artifacts/job-1"
	right.CommandProfile = json.RawMessage("{\n  \"network_policy\": \"disabled\",\n  \"allowed_commands\": [\"flutter\", \"echo\"],\n  \"allowed_stages\": [\"baseline\", \"cheap\"],\n  \"profile_name\": \"manual-p0\"\n}")
	right.ContextFiles = json.RawMessage("{\n  \"implementation_plan_path\": \"implementation-plan.md\",\n  \"manual_constraints_path\": \"manual-constraints.md\",\n  \"prd_json_path\": \"PRD.json\",\n  \"prd_markdown_path\": \"PRD.md\",\n  \"supporting_files\": [\"requirement.md\"],\n  \"template_fit_report_path\": \"template-fit-report.md\"\n}")

	if BuildInputDigest(left) != BuildInputDigest(right) {
		t.Fatalf("digest should ignore runtime-only paths and JSON formatting: left=%q right=%q", BuildInputDigest(left), BuildInputDigest(right))
	}

	right.IterationBudget++
	if BuildInputDigest(left) == BuildInputDigest(right) {
		t.Fatal("digest should change when semantic builder-input fields change")
	}
}

func TestPreparedInputDigestIncludesContextFileContentsButIgnoresApprovalSnapshots(t *testing.T) {
	prepareDir := t.TempDir()
	input := sampleBuildInput()
	input.ContextSourceDir = prepareDir
	files := map[string]string{
		"PRD.md":                 "prd markdown\n",
		"PRD.json":               "{\"id\":\"prd-1\"}\n",
		"template-fit-report.md": "fit\n",
		"implementation-plan.md": "plan-v1\n",
		"manual-constraints.md":  "constraints\n",
		"requirement.md":         "requirement\n",
		PRDApprovalFileName:      "approved-a\n",
		TemplateApprovalFileName: "approved-b\n",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(prepareDir, name), []byte(content), 0o644); err != nil {
			t.Fatalf("WriteFile(%s) error = %v", name, err)
		}
	}
	base := PreparedInputDigest(input, prepareDir)
	if base == "" {
		t.Fatal("PreparedInputDigest should not be empty")
	}
	if err := os.WriteFile(filepath.Join(prepareDir, "implementation-plan.md"), []byte("plan-v2\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(implementation-plan.md) error = %v", err)
	}
	changed := PreparedInputDigest(input, prepareDir)
	if changed == base {
		t.Fatal("PreparedInputDigest should change when implementation plan changes")
	}
	if err := os.WriteFile(filepath.Join(prepareDir, "implementation-plan.md"), []byte("plan-v1\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(implementation-plan.md reset) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(prepareDir, PRDApprovalFileName), []byte("approved-c\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(prd-approval.json) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(prepareDir, TemplateApprovalFileName), []byte("approved-d\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(template-approval.json) error = %v", err)
	}
	approvalOnly := PreparedInputDigest(input, prepareDir)
	if approvalOnly != base {
		t.Fatalf("PreparedInputDigest should ignore approval snapshots: base=%q approvalOnly=%q", base, approvalOnly)
	}
}

func TestPreparedInputDigestIgnoresTemplateFitReportContents(t *testing.T) {
	prepareDir := t.TempDir()
	input := sampleBuildInput()
	input.ContextSourceDir = prepareDir
	files := map[string]string{
		"PRD.md":                 "prd markdown\n",
		"PRD.json":               "{\"id\":\"prd-1\"}\n",
		"template-fit-report.md": "fit-v1\n",
		"implementation-plan.md": "plan-v1\n",
		"manual-constraints.md":  "constraints\n",
		"requirement.md":         "requirement\n",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(prepareDir, name), []byte(content), 0o644); err != nil {
			t.Fatalf("WriteFile(%s) error = %v", name, err)
		}
	}
	before := PreparedInputDigest(input, prepareDir)
	if before == "" {
		t.Fatal("PreparedInputDigest should not be empty")
	}
	if err := os.WriteFile(filepath.Join(prepareDir, "template-fit-report.md"), []byte("fit-v2\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(template-fit-report.md) error = %v", err)
	}
	after := PreparedInputDigest(input, prepareDir)
	if after != before {
		t.Fatalf("PreparedInputDigest should ignore template fit report drift: before=%q after=%q", before, after)
	}
}

func TestPreparedInputDigestIgnoresPRDMarkdownContents(t *testing.T) {
	prepareDir := t.TempDir()
	input := sampleBuildInput()
	input.ContextSourceDir = prepareDir
	files := map[string]string{
		"PRD.md":                 "prd markdown v1\n",
		"PRD.json":               "{\"id\":\"prd-1\"}\n",
		"template-fit-report.md": "fit\n",
		"implementation-plan.md": "plan-v1\n",
		"manual-constraints.md":  "constraints\n",
		"requirement.md":         "requirement\n",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(prepareDir, name), []byte(content), 0o644); err != nil {
			t.Fatalf("WriteFile(%s) error = %v", name, err)
		}
	}
	before := PreparedInputDigest(input, prepareDir)
	if before == "" {
		t.Fatal("PreparedInputDigest should not be empty")
	}
	if err := os.WriteFile(filepath.Join(prepareDir, "PRD.md"), []byte("prd markdown v2\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(PRD.md) error = %v", err)
	}
	after := PreparedInputDigest(input, prepareDir)
	if after != before {
		t.Fatalf("PreparedInputDigest should ignore PRD markdown drift: before=%q after=%q", before, after)
	}
}

func TestPreparedInputDigestIgnoresRequirementContents(t *testing.T) {
	prepareDir := t.TempDir()
	input := sampleBuildInput()
	input.ContextSourceDir = prepareDir
	files := map[string]string{
		"PRD.md":                 "prd markdown\n",
		"PRD.json":               "{\"id\":\"prd-1\"}\n",
		"template-fit-report.md": "fit\n",
		"implementation-plan.md": "plan-v1\n",
		"manual-constraints.md":  "constraints\n",
		"requirement.md":         "requirement-v1\n",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(prepareDir, name), []byte(content), 0o644); err != nil {
			t.Fatalf("WriteFile(%s) error = %v", name, err)
		}
	}
	before := PreparedInputDigest(input, prepareDir)
	if before == "" {
		t.Fatal("PreparedInputDigest should not be empty")
	}
	if err := os.WriteFile(filepath.Join(prepareDir, "requirement.md"), []byte("requirement-v2\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(requirement.md) error = %v", err)
	}
	after := PreparedInputDigest(input, prepareDir)
	if after != before {
		t.Fatalf("PreparedInputDigest should ignore requirement drift: before=%q after=%q", before, after)
	}
}

func TestPreparedInputDigestTracksAbsoluteContextPaths(t *testing.T) {
	prepareDir := t.TempDir()
	input := sampleBuildInput()
	contextFiles, err := json.Marshal(map[string]any{
		"prd_markdown_path":        filepath.Join(prepareDir, "PRD.md"),
		"prd_json_path":            filepath.Join(prepareDir, "PRD.json"),
		"template_fit_report_path": filepath.Join(prepareDir, "template-fit-report.md"),
		"implementation_plan_path": filepath.Join(prepareDir, "implementation-plan.md"),
	})
	if err != nil {
		t.Fatalf("Marshal(context files) error = %v", err)
	}
	input.ContextFiles = contextFiles
	files := map[string]string{
		"PRD.md":                 "prd markdown\n",
		"PRD.json":               "{\"id\":\"prd-1\"}\n",
		"template-fit-report.md": "fit\n",
		"implementation-plan.md": "plan-v1\n",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(prepareDir, name), []byte(content), 0o644); err != nil {
			t.Fatalf("WriteFile(%s) error = %v", name, err)
		}
	}
	before := PreparedInputDigest(input, prepareDir)
	if err := os.WriteFile(filepath.Join(prepareDir, "implementation-plan.md"), []byte("plan-v2\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(implementation-plan.md) error = %v", err)
	}
	after := PreparedInputDigest(input, prepareDir)
	if after == before {
		t.Fatalf("PreparedInputDigest should change for absolute implementation-plan path: before=%q after=%q", before, after)
	}
}

func TestPreparedInputDigestIncludesTemplateSourceContents(t *testing.T) {
	prepareDir := t.TempDir()
	templateDir := filepath.Join(t.TempDir(), "template")
	if err := os.MkdirAll(filepath.Join(templateDir, "lib"), 0o755); err != nil {
		t.Fatalf("MkdirAll(templateDir) error = %v", err)
	}
	files := map[string]string{
		"PRD.md":                 "prd markdown\n",
		"PRD.json":               "{\"id\":\"prd-1\"}\n",
		"template-fit-report.md": "fit\n",
		"implementation-plan.md": "plan-v1\n",
		"manual-constraints.md":  "constraints\n",
		"requirement.md":         "requirement\n",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(prepareDir, name), []byte(content), 0o644); err != nil {
			t.Fatalf("WriteFile(%s) error = %v", name, err)
		}
	}
	if err := os.WriteFile(filepath.Join(templateDir, "pubspec.yaml"), []byte("name: seeded_template\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(pubspec.yaml) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(templateDir, "lib", "main.dart"), []byte("void main() => print('v1');\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(main.dart) error = %v", err)
	}
	input := sampleBuildInput()
	input.ContextSourceDir = prepareDir
	input.TemplateSourceDir = templateDir
	before := PreparedInputDigest(input, prepareDir)
	if before == "" {
		t.Fatal("PreparedInputDigest should not be empty when template source dir is present")
	}
	if err := os.WriteFile(filepath.Join(templateDir, "lib", "main.dart"), []byte("void main() => print('v2');\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(main.dart) error = %v", err)
	}
	after := PreparedInputDigest(input, prepareDir)
	if after == before {
		t.Fatalf("PreparedInputDigest should change when template source changes: before=%q after=%q", before, after)
	}
}

func TestFileStoreFailRun(t *testing.T) {
	ctx := context.Background()
	store, err := NewFileStore(filepath.Join(t.TempDir(), "appfactory"))
	if err != nil {
		t.Fatalf("NewFileStore() error = %v", err)
	}
	service := NewService(store)
	run, err := service.Create(ctx, "builder-a", "builder-a", "lease-1", sampleBuildInput())
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	failed, err := service.Fail(ctx, run.RunID, FailureReport{
		Summary:            "model unavailable",
		RecoverySuggestion: "retry later",
		FailureSignatures:  []string{"rate_limit"},
		RoundState: &RoundState{
			CurrentPhase:      RoundPhaseFinalize,
			PhaseTrace:        []RoundPhase{RoundPhaseInspect, RoundPhaseEdit, RoundPhaseValidate, RoundPhaseRepair, RoundPhaseFinalize},
			NextAction:        ControlActionStop,
			PreserveWorkspace: true,
		},
		RepairContext: &RepairContext{
			State: &RoundState{
				CurrentPhase:      RoundPhaseFinalize,
				PhaseTrace:        []RoundPhase{RoundPhaseInspect, RoundPhaseEdit, RoundPhaseValidate, RoundPhaseRepair, RoundPhaseFinalize},
				NextAction:        ControlActionStop,
				PreserveWorkspace: true,
			},
			Budget: &RepairBudget{
				Owner:             RepairBudgetOwnerPlatform,
				MaxRounds:         1,
				UsedRounds:        1,
				RemainingRounds:   0,
				TerminationReason: "platform repair budget exhausted for current run",
			},
			Reason:              "model unavailable",
			FailureSignatures:   []string{"rate_limit"},
			RecommendedAction:   "retry later",
			PreserveWorkspace:   true,
			RequiresHumanReview: true,
		},
	})
	if err != nil {
		t.Fatalf("Fail() error = %v", err)
	}
	if failed.Status != StatusFailed {
		t.Fatalf("status = %s, want %s", failed.Status, StatusFailed)
	}
	if failed.DistinctFailureCount != 1 {
		t.Fatalf("DistinctFailureCount = %d, want 1", failed.DistinctFailureCount)
	}
	if failed.RoundState == nil || failed.RoundState.NextAction != ControlActionStop {
		t.Fatalf("RoundState = %+v, want stop action", failed.RoundState)
	}
	if failed.RepairContext == nil || failed.RepairContext.Budget == nil || failed.RepairContext.Budget.Owner != RepairBudgetOwnerPlatform {
		t.Fatalf("RepairContext = %+v, want platform-owned repair budget", failed.RepairContext)
	}
}

func TestFileStoreSeedsContextAndTemplateIntoWorkspace(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	store, err := NewFileStore(filepath.Join(root, "appfactory"))
	if err != nil {
		t.Fatalf("NewFileStore() error = %v", err)
	}
	service := NewService(store)

	contextDir := filepath.Join(root, "bundle")
	if err := os.MkdirAll(contextDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	for name, content := range map[string]string{
		"PRD.md":                 "# PRD\n",
		"PRD.json":               "{}\n",
		"template-fit-report.md": "fit\n",
		"implementation-plan.md": "plan\n",
		"manual-constraints.md":  "constraints\n",
		"requirement.md":         "requirement\n",
	} {
		if err := os.WriteFile(filepath.Join(contextDir, name), []byte(content), 0o644); err != nil {
			t.Fatalf("WriteFile(%s) error = %v", name, err)
		}
	}
	templateDir := filepath.Join(root, "template")
	if err := os.MkdirAll(filepath.Join(templateDir, "lib"), 0o755); err != nil {
		t.Fatalf("MkdirAll(template) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(templateDir, "pubspec.yaml"), []byte("name: seeded_template\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(pubspec) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(templateDir, "lib", "main.dart"), []byte("void main() {}\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(main.dart) error = %v", err)
	}

	input := sampleBuildInput()
	input.ContextSourceDir = contextDir
	input.TemplateSourceDir = templateDir
	run, err := service.Create(ctx, "builder-a", "builder-a", "lease-1", input)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	jobRoot := filepath.Dir(filepath.FromSlash(run.WorkspacePath))
	for _, path := range []string{
		filepath.Join(jobRoot, "prepare", "PRD.md"),
		filepath.Join(jobRoot, "prepare", "requirement.md"),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected context file %s to exist: %v", path, err)
		}
	}
	for _, path := range []string{
		filepath.Join(filepath.FromSlash(run.WorkspacePath), "pubspec.yaml"),
		filepath.Join(filepath.FromSlash(run.WorkspacePath), "lib", "main.dart"),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected workspace file %s to exist: %v", path, err)
		}
	}
}

func TestFileStoreRestoresRequestedWorkspaceIntoCanonicalWorkspace(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	store, err := NewFileStore(filepath.Join(root, "appfactory"))
	if err != nil {
		t.Fatalf("NewFileStore() error = %v", err)
	}
	service := NewService(store)

	preservedDir := filepath.Join(root, "appfactory", "jobs", "job-1", "snapshots", "preserved", "resume-1")
	if err := os.MkdirAll(filepath.Join(preservedDir, "lib"), 0o755); err != nil {
		t.Fatalf("MkdirAll(preservedDir) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(preservedDir, "pubspec.yaml"), []byte("name: restored_workspace\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(pubspec.yaml) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(preservedDir, "lib", "main.dart"), []byte("void main() => print('restored');\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(main.dart) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(preservedDir, "resume-marker.txt"), []byte("restored-marker\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(resume-marker.txt) error = %v", err)
	}
	canonicalWorkspace := filepath.Join(root, "appfactory", "jobs", "job-1", "workspace")
	if err := os.MkdirAll(filepath.Join(canonicalWorkspace, "lib"), 0o755); err != nil {
		t.Fatalf("MkdirAll(canonicalWorkspace) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(canonicalWorkspace, "resume-marker.txt"), []byte("old-canonical\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(old resume-marker.txt) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(canonicalWorkspace, "lib", "stale.dart"), []byte("void stale() {}\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(stale.dart) error = %v", err)
	}

	templateDir := filepath.Join(root, "template")
	if err := os.MkdirAll(filepath.Join(templateDir, "lib"), 0o755); err != nil {
		t.Fatalf("MkdirAll(templateDir) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(templateDir, "pubspec.yaml"), []byte("name: template_seed\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(pubspec.yaml) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(templateDir, "lib", "main.dart"), []byte("void main() => print('template');\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(main.dart) error = %v", err)
	}

	input := sampleBuildInput()
	input.TemplateSourceDir = templateDir
	input.WorkspacePath = "jobs/job-1/snapshots/preserved/resume-1"
	run, err := service.Create(ctx, "builder-a", "builder-a", "lease-1", input)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	workspacePath := filepath.FromSlash(run.WorkspacePath)
	markerData, err := os.ReadFile(filepath.Join(workspacePath, "resume-marker.txt"))
	if err != nil {
		t.Fatalf("ReadFile(resume-marker.txt) error = %v", err)
	}
	if strings.TrimSpace(string(markerData)) != "restored-marker" {
		t.Fatalf("resume-marker.txt = %q, want restored-marker", string(markerData))
	}
	mainData, err := os.ReadFile(filepath.Join(workspacePath, "lib", "main.dart"))
	if err != nil {
		t.Fatalf("ReadFile(main.dart) error = %v", err)
	}
	if strings.Contains(string(mainData), "template") {
		t.Fatalf("expected restored workspace to win over template seeding, got %q", string(mainData))
	}
	archivedBuckets, err := filepath.Glob(filepath.Join(root, "appfactory", "jobs", "job-1", "snapshots", "archived", "pre-restore-*"))
	if err != nil {
		t.Fatalf("Glob(archived snapshots) error = %v", err)
	}
	if len(archivedBuckets) != 1 {
		t.Fatalf("archived snapshots = %v, want 1 pre-restore snapshot", archivedBuckets)
	}
	staleData, err := os.ReadFile(filepath.Join(archivedBuckets[0], "lib", "stale.dart"))
	if err != nil {
		t.Fatalf("ReadFile(stale.dart) error = %v", err)
	}
	if !strings.Contains(string(staleData), "stale") {
		t.Fatalf("archived stale.dart = %q, want stale content", string(staleData))
	}
	eventsData, err := os.ReadFile(filepath.Join(root, "appfactory", filepath.FromSlash(run.EventsPath)))
	if err != nil {
		t.Fatalf("ReadFile(events.jsonl) error = %v", err)
	}
	eventsText := string(eventsData)
	if !strings.Contains(eventsText, "workspace_archived") || !strings.Contains(eventsText, "workspace_restored") {
		t.Fatalf("events.jsonl = %q, want workspace_archived and workspace_restored", eventsText)
	}
}

func TestFileStoreRunnerScriptDefaultsToSkillExecutorPhase(t *testing.T) {
	ctx := context.Background()
	store, err := NewFileStore(filepath.Join(t.TempDir(), "appfactory"))
	if err != nil {
		t.Fatalf("NewFileStore() error = %v", err)
	}
	service := NewService(store)
	run, err := service.Create(ctx, "builder-a", "builder-a", "lease-1", sampleBuildInput())
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	scriptPath := filepath.Clean(filepath.Join(filepath.FromSlash(run.WorkspacePath), "..", "runs", run.RunID, "runner.sh"))
	data, err := os.ReadFile(scriptPath)
	if err != nil {
		t.Fatalf("ReadFile(runner.sh) error = %v", err)
	}
	script := string(data)
	for _, snippet := range []string{
		"runtime=\"${APPFACTORY_BUILDER_RUNTIME:-executor}\"",
		"if [ \"$runtime\" != \"executor\" ]; then",
		"unsupported builder runtime: $runtime",
		"== skill executor phase ==",
		"default skill + executor runtime is not implemented yet; replace runner generation before executing appfactory runs",
		"mkdir -p .runtime/gradle-user-home",
		"export GRADLE_USER_HOME=\"$PWD/.runtime/gradle-user-home\"",
		"rm -rf android/.gradle",
	} {
		if !strings.Contains(script, snippet) {
			t.Fatalf("runner.sh missing snippet %q\n%s", snippet, script)
		}
	}
	for _, forbidden := range []string{
		"configure_legacy_builder_for_runner()",
		"legacy-builder new --name",
		"legacy-builder tell --file",
		"legacy-builder apply --skip-commit --no-exec",
		"run_legacy_builder_edit_round()",
	} {
		if strings.Contains(script, forbidden) {
			t.Fatalf("runner.sh should not contain legacy builder execution logic %q\n%s", forbidden, script)
		}
	}
}

func sampleBuildInput() BuildInput {
	commandProfile, _ := json.Marshal(map[string]any{"mode": "safe"})
	commandProfile, _ = json.Marshal(map[string]any{
		"profile_name":     "manual-p0",
		"allowed_stages":   []string{"baseline", "cheap"},
		"allowed_commands": []string{"flutter", "echo"},
		"network_policy":   "disabled",
	})
	contextFiles, _ := json.Marshal(map[string]any{
		"prd_markdown_path":        "PRD.md",
		"prd_json_path":            "PRD.json",
		"template_fit_report_path": "template-fit-report.md",
		"implementation_plan_path": "implementation-plan.md",
		"manual_constraints_path":  "manual-constraints.md",
		"supporting_files":         []string{"requirement.md"},
	})
	return BuildInput{
		SchemaVersion: "0.1.0",
		JobID:         "job-1",
		PRDID:         "prd-1",
		TemplateID:    "template-1",
		WorkspacePath: "/workspace/job-1",
		ArtifactDir:   "/artifacts/job-1",
		GoalSummary:   "build android app",
		TaskBundle: []TaskBundleItem{{
			TaskID:             "task-1",
			Title:              "Implement UI",
			Category:           "ui",
			Objective:          "Create home screen",
			TargetPaths:        []string{"lib/main.dart"},
			CompletionCriteria: []string{"screen renders"},
		}},
		AcceptanceChecks: []AcceptanceCheck{{CheckID: "check-1", Label: "build", Stage: "baseline", Required: true, Commands: []string{"echo build-start"}}},
		AllowedPaths:     []string{"lib/**"},
		ProtectedPaths:   []string{"android/app/build.gradle.kts"},
		KnowledgePack: []ProfileSkill{
			{SkillID: "prd-to-task-bundle", UsageStage: "planning", Scope: "flutter-android-p0"},
			{SkillID: "flutter-mvc-template", UsageStage: "layout", Scope: "flutter-android-p0"},
			{SkillID: "builder-direct-edit", UsageStage: "edit", Scope: "flutter-android-p0"},
			{SkillID: "flutter-build-closure", UsageStage: "closure", Scope: "flutter-android-p0"},
		},
		CommandProfile:  commandProfile,
		ContextFiles:    contextFiles,
		IterationBudget: 3,
		TokenBudget:     2000,
	}
}

func sampleArtifactManifest() ArtifactManifest {
	return ArtifactManifest{
		SchemaVersion: "0.1.0",
		JobID:         "job-1",
		GeneratedAt:   mustNow(),
		Items: []ArtifactItem{{
			ArtifactID:   "apk-1",
			Path:         "artifacts/apk/app-debug.apk",
			ArtifactType: "apk",
			Produced:     true,
		}},
	}
}

func sampleMetrics() Metrics {
	return Metrics{
		SchemaVersion:   "0.1.0",
		JobID:           "job-1",
		TotalIterations: 2,
		TotalTokens:     1024,
		DurationSeconds: 35,
		CommandRuns:     4,
		DeviceFailureCategories: []DeviceFailureCategoryStat{{
			Category:      "device_check_failed:adb_device_unavailable",
			FailureDomain: "device",
			Count:         1,
		}},
		FailureSignatures: []FailureSignature{{
			Signature: "device_check_failed:adb_device_unavailable",
			Count:     1,
			LastStage: "device",
		}},
	}
}

func sampleBuildOutput() BuildOutput {
	return BuildOutput{
		SchemaVersion: "0.1.0",
		JobID:         "job-1",
		Status:        "success",
		FinalSummary:  "build completed",
		ModifiedFiles: []FileChange{{Path: "lib/main.dart", ChangeType: "modified"}},
		ChecksPassed:  []CheckResult{{CheckID: "check-1", Label: "build", Stage: "baseline", Outcome: "passed"}},
		ChecksFailed:  []CheckResult{},
		NextHumanActions: []HumanAction{{
			ActionID: "review-1",
			Summary:  "review apk",
			Reason:   "manual validation",
		}},
		Artifacts: ArtifactSummary{
			ManifestPath:   "jobs/job-1/runs/current/artifact-manifest.json",
			PrimaryOutputs: []ArtifactPointer{{ArtifactID: "apk-1", Path: "artifacts/apk/app-debug.apk", ArtifactType: "apk"}},
		},
		Metrics: MetricsSummary{
			MetricsPath:               "jobs/job-1/runs/current/metrics.json",
			TotalIterations:           2,
			TotalTokens:               1024,
			DistinctFailureSignatures: 1,
		},
		ReportPaths: ReportPaths{
			ChangeSummaryPath:   "reports/change-summary.md",
			BuildReportPath:     "reports/build-report.md",
			SmokeTestReportPath: "reports/smoke-report.md",
		},
		RoundInputs: []RoundInput{{
			RoundID:      "round-1",
			Attempt:      1,
			GoalSummary:  "build completed",
			TaskBundle:   []TaskBundleItem{{TaskID: "task-1", Title: "render home", Category: TaskCategoryScreen, TargetPaths: []string{"lib/main.dart"}, CompletionCriteria: []string{"screen renders"}}},
			AllowedPaths: []string{"lib/**"},
		}},
		RoundOutputs: []RoundOutput{{
			RoundID: "round-1",
			Status:  "completed",
			Summary: "round completed",
			State: &RoundState{
				CurrentPhase: RoundPhaseFinalize,
				PhaseTrace:   []RoundPhase{RoundPhaseInspect, RoundPhaseEdit, RoundPhaseValidate, RoundPhaseFinalize},
				NextAction:   ControlActionStop,
			},
			WorkspacePatch: &WorkspacePatch{
				PatchID:       "round-1-patch",
				Status:        "applied",
				ModifiedFiles: []string{"lib/main.dart"},
				Operations:    []WorkspacePatchOperation{{Type: "replace_block", Path: "lib/main.dart"}},
			},
			ValidationResults: []ValidationResult{{CheckID: "check-1", Stage: StageBaseline, Outcome: "passed", Summary: "command chain completed"}},
		}},
		ValidationResults: []ValidationResult{{CheckID: "check-1", Stage: StageBaseline, Outcome: "passed", Summary: "command chain completed"}},
	}
}

func mustNow() (value time.Time) {
	return time.Now().UTC()
}
