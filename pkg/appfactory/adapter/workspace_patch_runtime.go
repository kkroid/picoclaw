package adapter

import (
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	appruns "github.com/sipeed/oneappfactory/pkg/appfactory/runs"
	"github.com/sipeed/oneappfactory/pkg/fileutil"
)

var ignoredWorkspaceRuntimeDirs = map[string]struct{}{
	".appfactory-context": {},
	".dart_tool":          {},
	".git":                {},
	".gradle":             {},
	".idea":               {},
	".runtime":            {},
	".vscode":             {},
	"build":               {},
	"dist":                {},
	"node_modules":        {},
	"out":                 {},
}

type workspaceSnapshot map[string]string

func captureWorkspaceSnapshot(workspaceRoot string) (workspaceSnapshot, error) {
	result := workspaceSnapshot{}
	err := filepath.WalkDir(workspaceRoot, func(current string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if current == workspaceRoot {
			return nil
		}
		relPath, err := filepath.Rel(workspaceRoot, current)
		if err != nil {
			return fmt.Errorf("resolve workspace path %s: %w", current, err)
		}
		relPath = filepath.ToSlash(relPath)
		if shouldSkipWorkspacePath(relPath) {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		if !entry.Type().IsRegular() {
			return nil
		}
		data, err := os.ReadFile(current)
		if err != nil {
			return fmt.Errorf("read workspace file %s: %w", relPath, err)
		}
		result[relPath] = string(data)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func captureWorkspaceSnapshotForPaths(workspaceRoot string, relPaths []string) (workspaceSnapshot, error) {
	result := workspaceSnapshot{}
	seen := make(map[string]struct{}, len(relPaths))
	for _, relPath := range relPaths {
		relPath = filepath.ToSlash(filepath.Clean(strings.TrimSpace(relPath)))
		if relPath == "" || relPath == "." || shouldSkipWorkspacePath(relPath) {
			continue
		}
		if _, ok := seen[relPath]; ok {
			continue
		}
		seen[relPath] = struct{}{}
		absPath := filepath.Join(workspaceRoot, filepath.FromSlash(relPath))
		data, err := os.ReadFile(absPath)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, fmt.Errorf("read workspace file %s: %w", relPath, err)
		}
		result[relPath] = string(data)
	}
	return result, nil
}

func shouldSkipWorkspacePath(relPath string) bool {
	relPath = filepath.ToSlash(strings.TrimSpace(relPath))
	if relPath == "" || relPath == "." {
		return false
	}
	segments := strings.Split(filepath.ToSlash(relPath), "/")
	for _, segment := range segments {
		if _, ok := ignoredWorkspaceRuntimeDirs[segment]; ok {
			return true
		}
	}
	base := path.Base(relPath)
	if strings.HasPrefix(base, ".appfactory-runner-") {
		return true
	}
	return false
}

func buildCapturedWorkspacePatch(roundID string, before, after workspaceSnapshot, allowedPaths, protectedPaths []string) (*appruns.WorkspacePatch, error) {
	patch := &appruns.WorkspacePatch{
		PatchID: roundID + "-patch",
		Status:  "not_reported",
	}
	allPaths := snapshotUnionPaths(before, after)
	for _, relPath := range allPaths {
		if shouldSkipWorkspacePath(relPath) {
			continue
		}
		beforeContent, hadBefore := before[relPath]
		afterContent, hadAfter := after[relPath]
		if hadBefore == hadAfter && beforeContent == afterContent {
			continue
		}
		if !matchesWorkspacePatterns(relPath, allowedPaths) {
			if matchesWorkspacePatterns(relPath, protectedPaths) {
				return nil, fmt.Errorf("workspace edit touched protected path %s", relPath)
			}
			return nil, fmt.Errorf("workspace edit touched path outside allowed roots: %s", relPath)
		}
		patch.ModifiedFiles = append(patch.ModifiedFiles, relPath)
		switch {
		case !hadBefore && hadAfter:
			patch.Operations = append(patch.Operations, appruns.WorkspacePatchOperation{
				Type:    "write_file",
				Path:    relPath,
				Content: afterContent,
			})
		case hadBefore && !hadAfter:
			patch.Operations = append(patch.Operations, appruns.WorkspacePatchOperation{
				Type: "delete_file",
				Path: relPath,
			})
		case hadBefore && hadAfter:
			if beforeContent == "" {
				patch.Operations = append(patch.Operations, appruns.WorkspacePatchOperation{
					Type:    "write_file",
					Path:    relPath,
					Content: afterContent,
				})
				continue
			}
			patch.Operations = append(patch.Operations, appruns.WorkspacePatchOperation{
				Type:       "replace_block",
				Path:       relPath,
				OldContent: beforeContent,
				NewContent: afterContent,
			})
		}
	}
	if len(patch.Operations) == 0 {
		return patch, nil
	}
	patch.Status = "captured"
	return patch, nil
}

func restoreWorkspaceSnapshot(workspaceRoot string, baseline, current workspaceSnapshot) error {
	allPaths := snapshotUnionPaths(baseline, current)
	for _, relPath := range allPaths {
		if shouldSkipWorkspacePath(relPath) {
			continue
		}
		baselineContent, hasBaseline := baseline[relPath]
		currentContent, hasCurrent := current[relPath]
		if hasBaseline == hasCurrent && baselineContent == currentContent {
			continue
		}
		absPath := filepath.Join(workspaceRoot, filepath.FromSlash(relPath))
		if !hasBaseline {
			if err := os.Remove(absPath); err != nil && !os.IsNotExist(err) {
				return fmt.Errorf("restore workspace delete %s: %w", relPath, err)
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
			return fmt.Errorf("restore workspace mkdir %s: %w", relPath, err)
		}
		if err := fileutil.WriteFileAtomic(absPath, []byte(baselineContent), 0o600); err != nil {
			return fmt.Errorf("restore workspace write %s: %w", relPath, err)
		}
	}
	return nil
}

func restoreWorkspaceSnapshotForPaths(workspaceRoot string, baseline workspaceSnapshot, relPaths []string) error {
	current, err := captureWorkspaceSnapshotForPaths(workspaceRoot, relPaths)
	if err != nil {
		return err
	}
	return restoreWorkspaceSnapshot(workspaceRoot, baseline, current)
}

func snapshotUnionPaths(before, after workspaceSnapshot) []string {
	paths := make([]string, 0, len(before)+len(after))
	seen := make(map[string]struct{}, len(before)+len(after))
	for relPath := range before {
		if _, ok := seen[relPath]; ok {
			continue
		}
		seen[relPath] = struct{}{}
		paths = append(paths, relPath)
	}
	for relPath := range after {
		if _, ok := seen[relPath]; ok {
			continue
		}
		seen[relPath] = struct{}{}
		paths = append(paths, relPath)
	}
	sort.Strings(paths)
	return paths
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
