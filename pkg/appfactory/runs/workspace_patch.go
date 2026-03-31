package runs

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/sipeed/picoclaw/pkg/fileutil"
)

type workspacePatchState struct {
	relPath  string
	absPath  string
	original string
	content  string
	existed  bool
	deleted  bool
	loaded   bool
	modified bool
}

func ApplyWorkspacePatch(workspaceRoot string, allowedPaths, protectedPaths []string, patch WorkspacePatch) (WorkspacePatchApplyResult, error) {
	result := WorkspacePatchApplyResult{
		PatchID: patch.PatchID,
		Status:  "not_reported",
	}
	if strings.TrimSpace(workspaceRoot) == "" {
		result.FailureReason = "workspace root is empty"
		return result, fmt.Errorf("%s", result.FailureReason)
	}
	if len(patch.Operations) == 0 {
		return result, nil
	}
	states := make(map[string]*workspacePatchState, len(patch.Operations))
	for _, op := range patch.Operations {
		state, err := loadWorkspacePatchState(workspaceRoot, allowedPaths, protectedPaths, op, states)
		if err != nil {
			result.FailureReason = err.Error()
			return result, err
		}
		if err := applyWorkspacePatchOperation(state, op); err != nil {
			result.FailureReason = err.Error()
			return result, err
		}
	}
	for _, state := range states {
		if !state.modified {
			continue
		}
		if state.deleted {
			if err := os.Remove(state.absPath); err != nil {
				result.FailureReason = fmt.Sprintf("delete %s: %v", state.relPath, err)
				return result, fmt.Errorf("%s", result.FailureReason)
			}
			result.ModifiedFiles = append(result.ModifiedFiles, FileChange{Path: state.relPath, ChangeType: "deleted"})
			result.AppliedOps++
			continue
		}
		if err := os.MkdirAll(filepath.Dir(state.absPath), 0o755); err != nil {
			result.FailureReason = fmt.Sprintf("create parent dir for %s: %v", state.relPath, err)
			return result, fmt.Errorf("%s", result.FailureReason)
		}
		if err := fileutil.WriteFileAtomic(state.absPath, []byte(state.content), 0o600); err != nil {
			result.FailureReason = fmt.Sprintf("write %s: %v", state.relPath, err)
			return result, fmt.Errorf("%s", result.FailureReason)
		}
		changeType := "modified"
		if !state.existed {
			changeType = "added"
		}
		result.ModifiedFiles = append(result.ModifiedFiles, FileChange{Path: state.relPath, ChangeType: changeType})
		result.AppliedOps++
	}
	if result.AppliedOps == 0 {
		return result, nil
	}
	result.Status = "applied"
	return result, nil
}

func loadWorkspacePatchState(workspaceRoot string, allowedPaths, protectedPaths []string, op WorkspacePatchOperation, states map[string]*workspacePatchState) (*workspacePatchState, error) {
	relPath, absPath, err := resolveWorkspacePatchPath(workspaceRoot, op.Path)
	if err != nil {
		return nil, err
	}
	if !matchesWorkspacePatterns(relPath, allowedPaths) {
		return nil, fmt.Errorf("path %s is outside allowed roots", relPath)
	}
	if matchesWorkspacePatterns(relPath, protectedPaths) {
		return nil, fmt.Errorf("path %s is protected", relPath)
	}
	if state, ok := states[relPath]; ok {
		return state, nil
	}
	state := &workspacePatchState{relPath: relPath, absPath: absPath}
	data, err := os.ReadFile(absPath)
	if err == nil {
		state.original = string(data)
		state.content = string(data)
		state.existed = true
		state.loaded = true
	} else if os.IsNotExist(err) {
		state.loaded = true
	} else {
		return nil, fmt.Errorf("read %s: %w", relPath, err)
	}
	states[relPath] = state
	return state, nil
}

func applyWorkspacePatchOperation(state *workspacePatchState, op WorkspacePatchOperation) error {
	switch strings.TrimSpace(op.Type) {
	case "write_file":
		content := op.Content
		if content == "" {
			content = op.NewContent
		}
		state.content = content
		state.deleted = false
		state.modified = true
		return nil
	case "replace_block":
		if !state.existed && state.content == "" {
			return fmt.Errorf("replace_block requires existing file: %s", state.relPath)
		}
		needle := op.OldContent
		if needle == "" {
			needle = op.Anchor
		}
		if strings.TrimSpace(needle) == "" {
			return fmt.Errorf("replace_block requires old_content or anchor for %s", state.relPath)
		}
		replacement := op.NewContent
		if replacement == "" {
			replacement = op.Content
		}
		count := strings.Count(state.content, needle)
		if count != 1 {
			return fmt.Errorf("replace_block expects exactly one match in %s, got %d", state.relPath, count)
		}
		state.content = strings.Replace(state.content, needle, replacement, 1)
		state.deleted = false
		state.modified = true
		return nil
	case "delete_file":
		if !state.existed {
			return fmt.Errorf("delete_file requires existing file: %s", state.relPath)
		}
		state.deleted = true
		state.modified = true
		return nil
	default:
		return fmt.Errorf("unsupported workspace patch operation %q for %s", op.Type, state.relPath)
	}
}

func resolveWorkspacePatchPath(workspaceRoot, target string) (string, string, error) {
	trimmed := filepath.ToSlash(filepath.Clean(strings.TrimSpace(target)))
	if trimmed == "." || trimmed == "" {
		return "", "", fmt.Errorf("workspace patch path is empty")
	}
	if strings.HasPrefix(trimmed, "../") || trimmed == ".." || path.IsAbs(trimmed) {
		return "", "", fmt.Errorf("workspace patch path %q escapes workspace root", target)
	}
	absPath := filepath.Join(workspaceRoot, filepath.FromSlash(trimmed))
	return trimmed, absPath, nil
}

func matchesWorkspacePatterns(relPath string, patterns []string) bool {
	if len(patterns) == 0 {
		return false
	}
	for _, pattern := range patterns {
		if workspacePatternMatches(relPath, pattern) {
			return true
		}
	}
	return false
}

func workspacePatternMatches(relPath, pattern string) bool {
	relPath = filepath.ToSlash(filepath.Clean(relPath))
	pattern = filepath.ToSlash(filepath.Clean(strings.TrimSpace(pattern)))
	if pattern == "" {
		return false
	}
	if pattern == relPath {
		return true
	}
	if strings.HasSuffix(pattern, "/**") {
		prefix := strings.TrimSuffix(pattern, "/**")
		return relPath == prefix || strings.HasPrefix(relPath, prefix+"/")
	}
	ok, err := path.Match(pattern, relPath)
	return err == nil && ok
}
