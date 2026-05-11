package config

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"

	"github.com/sipeed/oneappfactory/pkg/fileutil"
)

const SecurityConfigFile = ".security.yml"

func normalizeSecurityConfig(sec *SecurityConfig) *SecurityConfig {
	if sec == nil {
		sec = &SecurityConfig{}
	}
	if sec.ModelList == nil {
		sec.ModelList = map[string]ModelSecurityEntry{}
	}
	return sec
}

type SecurityConfig struct {
	ModelList map[string]ModelSecurityEntry `yaml:"model_list"`

	sensitiveCache *SensitiveDataCache
}

type ModelSecurityEntry struct {
	APIKeys []string `yaml:"api_keys,omitempty"`
}

func securityPath(configPath string) string {
	return filepath.Join(filepath.Dir(configPath), SecurityConfigFile)
}

func loadSecurityConfig(securityPath string) (*SecurityConfig, error) {
	data, err := os.ReadFile(securityPath)
	if err != nil {
		if os.IsNotExist(err) {
			return normalizeSecurityConfig(nil), nil
		}
		return nil, fmt.Errorf("failed to read security config: %w", err)
	}
	var sec SecurityConfig
	if err := yaml.Unmarshal(data, &sec); err != nil {
		return nil, fmt.Errorf("failed to parse security config: %w", err)
	}
	return normalizeSecurityConfig(&sec), nil
}

func saveSecurityConfig(securityPath string, sec *SecurityConfig) error {
	sec = normalizeSecurityConfig(sec)
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(sec); err != nil {
		return fmt.Errorf("failed to marshal security config: %w", err)
	}
	return fileutil.WriteFileAtomic(securityPath, buf.Bytes(), 0o600)
}

func mergeSecurityConfig(existing, newer *SecurityConfig) *SecurityConfig {
	if existing == nil {
		return normalizeSecurityConfig(newer)
	}
	if newer == nil {
		return normalizeSecurityConfig(existing)
	}
	result := normalizeSecurityConfig(nil)
	for key, value := range existing.ModelList {
		result.ModelList[key] = value
	}
	for key, value := range newer.ModelList {
		if len(value.APIKeys) > 0 {
			result.ModelList[key] = value
		}
	}
	return result
}

type SensitiveDataCache struct {
	once     sync.Once
	replacer *strings.Replacer
}

func (sec *SecurityConfig) SensitiveDataReplacer() *strings.Replacer {
	sec = normalizeSecurityConfig(sec)
	sec.initSensitiveCache()
	return sec.sensitiveCache.replacer
}

func (sec *SecurityConfig) initSensitiveCache() {
	if sec.sensitiveCache == nil {
		sec.sensitiveCache = &SensitiveDataCache{}
	}
	sec.sensitiveCache.once.Do(func() {
		values := sec.collectSensitiveValues()
		pairs := make([]string, 0, len(values)*2)
		for _, value := range values {
			pairs = append(pairs, value, "[FILTERED]")
		}
		if len(pairs) == 0 {
			pairs = []string{"\x00", "\x00"}
		}
		sec.sensitiveCache.replacer = strings.NewReplacer(pairs...)
	})
}

func (sec *SecurityConfig) collectSensitiveValues() []string {
	sec = normalizeSecurityConfig(sec)
	seen := map[string]struct{}{}
	values := make([]string, 0)
	for _, entry := range sec.ModelList {
		for _, key := range entry.APIKeys {
			key = strings.TrimSpace(key)
			if len(key) < 8 {
				continue
			}
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			values = append(values, key)
		}
	}
	sort.Strings(values)
	return values
}
