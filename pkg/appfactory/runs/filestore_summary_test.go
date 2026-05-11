package runs

import "testing"

func TestBuildPatchAppliedSummaryIncludesPaths(t *testing.T) {
	paths := []string{"lib/views/home_page.dart"}
	if got := buildPatchAppliedSummary("round-1", paths); got != "round round-1 applied patch to lib/views/home_page.dart" {
		t.Fatalf("buildPatchAppliedSummary() = %q", got)
	}
}

func TestBuildPatchAppliedSummaryPreviewsMultiplePaths(t *testing.T) {
	paths := []string{"lib/a.dart", "lib/b.dart", "lib/c.dart", "lib/d.dart"}
	want := "round round-1 applied patch to 4 files: lib/a.dart, lib/b.dart, lib/c.dart, +1 more"
	if got := buildPatchAppliedSummary("round-1", paths); got != want {
		t.Fatalf("buildPatchAppliedSummary() = %q, want %q", got, want)
	}
}
