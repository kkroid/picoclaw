package adapter

import (
	"errors"
	"testing"
)

func TestPatchSummariesIncludeFilePaths(t *testing.T) {
	path := "lib/views/home_page.dart"

	if got := buildPatchGenerationStartedSummary("round-1", []string{path}); got != "round round-1 started patch generation for lib/views/home_page.dart" {
		t.Fatalf("buildPatchGenerationStartedSummary() = %q", got)
	}
	if got := buildGeneratedPatchSummary("round-1", []string{path}); got != "round round-1 generated patch for lib/views/home_page.dart" {
		t.Fatalf("buildGeneratedPatchSummary() = %q", got)
	}
	if got := buildAppliedPatchSummary("round-1", []string{path}); got != "round round-1 applied patch to lib/views/home_page.dart" {
		t.Fatalf("buildAppliedPatchSummary() = %q", got)
	}
	if got := buildPatchGenerationFailedSummary("round-1", []string{path}, errors.New("timeout")); got != "round round-1 patch generation failed for lib/views/home_page.dart: timeout" {
		t.Fatalf("buildPatchGenerationFailedSummary() = %q", got)
	}
	if got := buildPatchApplyFailedSummary("round-1", []string{path}, errors.New("permission denied")); got != "round round-1 patch apply failed for lib/views/home_page.dart: permission denied" {
		t.Fatalf("buildPatchApplyFailedSummary() = %q", got)
	}
}

func TestPatchSummariesPreviewMultiplePaths(t *testing.T) {
	paths := []string{"lib/a.dart", "lib/b.dart", "lib/c.dart", "lib/d.dart", "lib/a.dart"}
	want := "round round-1 started patch generation for 4 files: lib/a.dart, lib/b.dart, lib/c.dart, +1 more"
	if got := buildPatchGenerationStartedSummary("round-1", paths); got != want {
		t.Fatalf("buildPatchGenerationStartedSummary() = %q, want %q", got, want)
	}
}
