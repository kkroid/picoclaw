package prepare

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/sipeed/oneappfactory/pkg/fileutil"
)

func WriteBundle(outputDir string, bundle Bundle) error {
	if strings.TrimSpace(outputDir) == "" {
		return fmt.Errorf("output dir is required")
	}
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return fmt.Errorf("create output dir: %w", err)
	}
	for _, name := range bundle.FileNames() {
		path := filepath.Join(outputDir, name)
		if err := fileutil.WriteFileAtomic(path, bundle.Files[name], 0o644); err != nil {
			return fmt.Errorf("write %s: %w", name, err)
		}
	}
	return nil
}

func (bundle Bundle) FileNames() []string {
	result := make([]string, 0, len(bundle.Files))
	for name := range bundle.Files {
		result = append(result, name)
	}
	sort.Strings(result)
	return result
}
