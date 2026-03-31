package adapter

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCaptureWorkspaceSnapshotSkipsAppFactoryRuntimeArtifacts(t *testing.T) {
	workspaceRoot := t.TempDir()
	libDir := filepath.Join(workspaceRoot, "lib")
	if err := os.MkdirAll(libDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(lib) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(libDir, "main.dart"), []byte("void main() {}\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(main.dart) error = %v", err)
	}
	contextDir := filepath.Join(workspaceRoot, ".appfactory-context")
	if err := os.MkdirAll(contextDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(.appfactory-context) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(contextDir, "PRD.json"), []byte("{}\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(PRD.json) error = %v", err)
	}
	runtimeDir := filepath.Join(workspaceRoot, ".runtime", "gradle-user-home")
	if err := os.MkdirAll(runtimeDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(.runtime) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(runtimeDir, "cache.txt"), []byte("cache\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(cache.txt) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(workspaceRoot, ".appfactory-runner-prompt.md"), []byte("prompt\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(.appfactory-runner-prompt.md) error = %v", err)
	}

	snapshot, err := captureWorkspaceSnapshot(workspaceRoot)
	if err != nil {
		t.Fatalf("captureWorkspaceSnapshot() error = %v", err)
	}
	if len(snapshot) != 1 {
		t.Fatalf("snapshot entries = %#v, want only lib/main.dart", snapshot)
	}
	if snapshot["lib/main.dart"] != "void main() {}\n" {
		t.Fatalf("snapshot lib/main.dart = %q, want source file", snapshot["lib/main.dart"])
	}
	if _, exists := snapshot[".appfactory-context/PRD.json"]; exists {
		t.Fatalf("snapshot should ignore .appfactory-context/PRD.json: %#v", snapshot)
	}
	if _, exists := snapshot[".appfactory-runner-prompt.md"]; exists {
		t.Fatalf("snapshot should ignore .appfactory-runner-prompt.md: %#v", snapshot)
	}
	if _, exists := snapshot[".runtime/gradle-user-home/cache.txt"]; exists {
		t.Fatalf("snapshot should ignore .runtime/gradle-user-home/cache.txt: %#v", snapshot)
	}
}

func TestBuildCapturedWorkspacePatchIgnoresAppFactoryRuntimeArtifacts(t *testing.T) {
	before := workspaceSnapshot{
		"lib/main.dart": "void main() {}\n",
	}
	after := workspaceSnapshot{
		"lib/main.dart":                       "void main() { print('ok'); }\n",
		".appfactory-context/PRD.json":        "{}\n",
		".appfactory-runner-prompt.md":        "prompt\n",
		".runtime/gradle-user-home/cache.txt": "cache\n",
	}

	patch, err := buildCapturedWorkspacePatch("round-1", before, after, []string{"lib/**", "test/**", "pubspec.yaml"}, nil)
	if err != nil {
		t.Fatalf("buildCapturedWorkspacePatch() error = %v", err)
	}
	if patch.Status != "captured" {
		t.Fatalf("patch.Status = %q, want captured", patch.Status)
	}
	if len(patch.ModifiedFiles) != 1 || patch.ModifiedFiles[0] != "lib/main.dart" {
		t.Fatalf("patch.ModifiedFiles = %#v, want only lib/main.dart", patch.ModifiedFiles)
	}
	if len(patch.Operations) != 1 {
		t.Fatalf("patch.Operations = %#v, want one operation", patch.Operations)
	}
	if patch.Operations[0].Path != "lib/main.dart" {
		t.Fatalf("patch.Operations[0].Path = %q, want lib/main.dart", patch.Operations[0].Path)
	}
}
