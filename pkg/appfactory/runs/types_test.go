package runs

import (
	"path/filepath"
	"testing"
)

func TestNormalizeTaskBundleItemDropsCompatibilityScreenRefsWhenSurfaceRefsExist(t *testing.T) {
	task := NormalizeTaskBundleItem(TaskBundleItem{
		TaskID: "task-bind-overview",
		AllocationTransition: &TaskAllocationTransition{
			SurfaceRefs: []string{"surface-overview"},
			ScreenRefs:  []string{"screen-home"},
		},
	})
	if task.AllocationTransition == nil {
		t.Fatal("AllocationTransition = nil, want normalized transition")
	}
	if len(task.AllocationTransition.SurfaceRefs) != 1 || task.AllocationTransition.SurfaceRefs[0] != "surface-overview" {
		t.Fatalf("SurfaceRefs = %v, want [surface-overview]", task.AllocationTransition.SurfaceRefs)
	}
	if len(task.AllocationTransition.ScreenRefs) != 0 {
		t.Fatalf("ScreenRefs = %v, want compatibility screen refs dropped when surface refs exist", task.AllocationTransition.ScreenRefs)
	}
}

func TestNormalizeTaskBundleItemTrimsAndDedupesBindingRefs(t *testing.T) {
	task := NormalizeTaskBundleItem(TaskBundleItem{
		TaskID: "task-bind-copy",
		AllocationTransition: &TaskAllocationTransition{
			BindingRefs: []string{"  domain-copy  ", "domain-copy", ""},
		},
	})
	if task.AllocationTransition == nil {
		t.Fatal("AllocationTransition = nil, want normalized transition")
	}
	if len(task.AllocationTransition.BindingRefs) != 1 || task.AllocationTransition.BindingRefs[0] != "domain-copy" {
		t.Fatalf("BindingRefs = %v, want [domain-copy]", task.AllocationTransition.BindingRefs)
	}
}

func TestDefaultTemplateSourceDirUsesEnvRoot(t *testing.T) {
	t.Setenv("APPFACTORY_TEMPLATE_ROOT", "/opt/appfactory/templates")

	got := defaultTemplateSourceDir("flutter-open-lite")
	want := filepath.Join("/opt/appfactory/templates", "flutter-open-lite")
	if got != want {
		t.Fatalf("defaultTemplateSourceDir() = %q, want %q", got, want)
	}
}

func TestDefaultTemplateSourceDirFallsBackToRepoTemplates(t *testing.T) {
	t.Setenv("APPFACTORY_TEMPLATE_ROOT", "")

	got := defaultTemplateSourceDir("flutter-open-lite")
	if got == "" {
		t.Fatal("defaultTemplateSourceDir() returned empty path")
	}
	wantSuffix := filepath.Join("examples", "appfactory", "templates", "flutter-open-lite")
	if filepath.Clean(got) != got {
		t.Fatalf("defaultTemplateSourceDir() should return a clean path, got %q", got)
	}
	if len(got) < len(wantSuffix) || got[len(got)-len(wantSuffix):] != wantSuffix {
		t.Fatalf("defaultTemplateSourceDir() = %q, want suffix %q", got, wantSuffix)
	}
}
