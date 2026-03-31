package runs

import (
	"os"
	"path/filepath"
	"testing"
)

func TestApplyWorkspacePatchWriteReplaceDelete(t *testing.T) {
	workspaceRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspaceRoot, "lib"), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	mainPath := filepath.Join(workspaceRoot, "lib", "main.dart")
	if err := os.WriteFile(mainPath, []byte("void main() {\n  print('old');\n}\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(main.dart) error = %v", err)
	}
	tempPath := filepath.Join(workspaceRoot, "lib", "temp.txt")
	if err := os.WriteFile(tempPath, []byte("delete me\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(temp.txt) error = %v", err)
	}

	result, err := ApplyWorkspacePatch(workspaceRoot, []string{"lib/**"}, []string{"lib/protected.dart"}, WorkspacePatch{
		PatchID: "patch-1",
		Operations: []WorkspacePatchOperation{
			{Type: "write_file", Path: "lib/generated.dart", Content: "const generated = true;\n"},
			{Type: "replace_block", Path: "lib/main.dart", OldContent: "print('old');", NewContent: "print('new');"},
			{Type: "delete_file", Path: "lib/temp.txt"},
		},
	})
	if err != nil {
		t.Fatalf("ApplyWorkspacePatch() error = %v", err)
	}
	if result.Status != "applied" {
		t.Fatalf("Status = %q, want applied", result.Status)
	}
	if result.AppliedOps != 3 {
		t.Fatalf("AppliedOps = %d, want 3", result.AppliedOps)
	}
	if len(result.ModifiedFiles) != 3 {
		t.Fatalf("ModifiedFiles len = %d, want 3", len(result.ModifiedFiles))
	}
	mainData, err := os.ReadFile(mainPath)
	if err != nil {
		t.Fatalf("ReadFile(main.dart) error = %v", err)
	}
	if string(mainData) != "void main() {\n  print('new');\n}\n" {
		t.Fatalf("main.dart = %q, want replaced content", string(mainData))
	}
	generatedData, err := os.ReadFile(filepath.Join(workspaceRoot, "lib", "generated.dart"))
	if err != nil {
		t.Fatalf("ReadFile(generated.dart) error = %v", err)
	}
	if string(generatedData) != "const generated = true;\n" {
		t.Fatalf("generated.dart = %q, want generated content", string(generatedData))
	}
	if _, err := os.Stat(tempPath); !os.IsNotExist(err) {
		t.Fatalf("temp.txt still exists or stat failed: %v", err)
	}
}

func TestApplyWorkspacePatchRejectsProtectedAndOutOfScopePaths(t *testing.T) {
	workspaceRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspaceRoot, "lib"), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(workspaceRoot, "lib", "protected.dart"), []byte("const v = 1;\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(protected.dart) error = %v", err)
	}

	_, err := ApplyWorkspacePatch(workspaceRoot, []string{"lib/**"}, []string{"lib/protected.dart"}, WorkspacePatch{
		PatchID:    "patch-protected",
		Operations: []WorkspacePatchOperation{{Type: "write_file", Path: "lib/protected.dart", Content: "const v = 2;\n"}},
	})
	if err == nil || err.Error() != "path lib/protected.dart is protected" {
		t.Fatalf("protected err = %v, want protected path error", err)
	}

	_, err = ApplyWorkspacePatch(workspaceRoot, []string{"lib/**"}, nil, WorkspacePatch{
		PatchID:    "patch-scope",
		Operations: []WorkspacePatchOperation{{Type: "write_file", Path: "android/app/build.gradle.kts", Content: "ignored\n"}},
	})
	if err == nil || err.Error() != "path android/app/build.gradle.kts is outside allowed roots" {
		t.Fatalf("allowed err = %v, want outside allowed roots", err)
	}
}

func TestApplyWorkspacePatchRequiresExactAnchorMatch(t *testing.T) {
	workspaceRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspaceRoot, "lib"), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	filePath := filepath.Join(workspaceRoot, "lib", "main.dart")
	if err := os.WriteFile(filePath, []byte("anchor\nanchor\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(main.dart) error = %v", err)
	}

	_, err := ApplyWorkspacePatch(workspaceRoot, []string{"lib/**"}, nil, WorkspacePatch{
		PatchID:    "patch-anchor",
		Operations: []WorkspacePatchOperation{{Type: "replace_block", Path: "lib/main.dart", Anchor: "anchor", NewContent: "updated"}},
	})
	if err == nil || err.Error() != "replace_block expects exactly one match in lib/main.dart, got 2" {
		t.Fatalf("anchor err = %v, want exact-match failure", err)
	}
}
