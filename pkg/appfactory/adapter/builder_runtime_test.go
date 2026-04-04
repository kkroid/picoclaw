package adapter

import (
	"encoding/json"
	"strings"
	"testing"

	appruns "github.com/sipeed/picoclaw/pkg/appfactory/runs"
)

func TestBuildBuilderRuntimePromptAvoidsDeprecatedFlutterAPIs(t *testing.T) {
	run := runRecord{
		GoalSummary:   "repair flutter ui",
		WorkspacePath: t.TempDir(),
		TaskBundle: []appruns.TaskBundleItem{{
			TaskID:      "task-home",
			TaskType:    appruns.BuilderRuntimeTaskTypeSingleFileEdit,
			TargetPaths: []string{"lib/views/home_page.dart"},
		}},
	}
	roundInput := appruns.RoundInput{
		TaskBundle: run.TaskBundle,
		AllowedPaths: []string{
			"lib/**",
		},
	}
	route := appruns.BuilderRuntimeTaskRoute{
		TaskID:      "task-home",
		TaskType:    appruns.BuilderRuntimeTaskTypeSingleFileEdit,
		RouteSource: "default_model",
	}

	prompt, err := buildBuilderRuntimePrompt(run, roundInput, route)
	if err != nil {
		t.Fatalf("buildBuilderRuntimePrompt() error = %v", err)
	}
	if !strings.Contains(prompt, "prefer withValues() and avoid withOpacity()") {
		t.Fatalf("prompt missing deprecated Flutter API guidance: %q", prompt)
	}
	if !strings.Contains(prompt, "free of deprecated_member_use") {
		t.Fatalf("prompt missing flutter analyze hard-gate guidance: %q", prompt)
	}
}

func TestBuildBuilderRuntimePromptIncludesHumanNotes(t *testing.T) {
	run := runRecord{
		GoalSummary:   "repair flutter ui",
		HumanNotes:    json.RawMessage(`[{"note_id":"note-canary","summary":"prefer stable public jobs repair canary","scope":"engineering"}]`),
		WorkspacePath: t.TempDir(),
		TaskBundle: []appruns.TaskBundleItem{{
			TaskID:      "task-home",
			TaskType:    appruns.BuilderRuntimeTaskTypeAnalyzeRepair,
			TargetPaths: []string{"lib/main.dart"},
		}},
	}
	roundInput := appruns.RoundInput{
		TaskBundle: run.TaskBundle,
		AllowedPaths: []string{
			"lib/**",
		},
	}
	route := appruns.BuilderRuntimeTaskRoute{
		TaskID:      "task-home",
		TaskType:    appruns.BuilderRuntimeTaskTypeAnalyzeRepair,
		RouteSource: "default_model",
	}

	prompt, err := buildBuilderRuntimePrompt(run, roundInput, route)
	if err != nil {
		t.Fatalf("buildBuilderRuntimePrompt() error = %v", err)
	}
	if !strings.Contains(prompt, "Human notes JSON: [{\"note_id\":\"note-canary\"") {
		t.Fatalf("prompt missing human notes payload: %q", prompt)
	}
}