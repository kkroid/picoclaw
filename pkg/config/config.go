package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"

	"github.com/sipeed/oneappfactory/pkg"
	"github.com/sipeed/oneappfactory/pkg/credential"
	"github.com/sipeed/oneappfactory/pkg/fileutil"
	"github.com/sipeed/oneappfactory/pkg/logger"
)

var rrCounter atomic.Uint64

const CurrentVersion = 1

type Config struct {
	Version    int              `json:"version"`
	Workspace  string           `json:"workspace"`
	ModelList  []*ModelConfig   `json:"model_list"`
	AppFactory AppFactoryConfig `json:"appfactory,omitempty"`
	BuildInfo  BuildInfo        `json:"build_info,omitempty"`

	security *SecurityConfig
}

func (c *Config) WithSecurity(sec *SecurityConfig) *Config {
	sec = normalizeSecurityConfig(sec)
	if err := applySecurityConfig(c, sec); err != nil {
		return nil
	}
	c.security = sec
	return c
}

func (c *Config) FilterSensitiveData(content string) string {
	if c.security == nil || content == "" || len(content) < 8 {
		return content
	}
	return c.security.SensitiveDataReplacer().Replace(content)
}

type BuildInfo struct {
	Version   string `json:"version"`
	GitCommit string `json:"git_commit"`
	BuildTime string `json:"build_time"`
	GoVersion string `json:"go_version"`
}

type AgentModelConfig struct {
	Primary   string   `json:"primary,omitempty"`
	Fallbacks []string `json:"fallbacks,omitempty"`
}

func (m *AgentModelConfig) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		m.Primary = s
		m.Fallbacks = nil
		return nil
	}
	type raw struct {
		Primary   string   `json:"primary"`
		Fallbacks []string `json:"fallbacks"`
	}
	var r raw
	if err := json.Unmarshal(data, &r); err != nil {
		return err
	}
	m.Primary = r.Primary
	m.Fallbacks = r.Fallbacks
	return nil
}

func (m AgentModelConfig) MarshalJSON() ([]byte, error) {
	if len(m.Fallbacks) == 0 && m.Primary != "" {
		return json.Marshal(m.Primary)
	}
	type raw struct {
		Primary   string   `json:"primary,omitempty"`
		Fallbacks []string `json:"fallbacks,omitempty"`
	}
	return json.Marshal(raw{Primary: m.Primary, Fallbacks: m.Fallbacks})
}

type AppFactoryConfig struct {
	PlanningEngine PlanningEngineConfig `json:"planning_engine,omitempty"`
	BuilderRuntime BuilderRuntimeConfig `json:"builder_runtime,omitempty"`
}

type PlanningEngineConfig struct {
	PlanningModel *AgentModelConfig `json:"planning_model,omitempty"`
	DecisionModel *AgentModelConfig `json:"decision_model,omitempty"`
}

type BuilderRuntimeConfig struct {
	Enabled          bool                                 `json:"enabled"`
	DefaultModel     *AgentModelConfig                    `json:"default_model,omitempty"`
	UpgradeModel     *AgentModelConfig                    `json:"upgrade_model,omitempty"`
	TaskRoutes       []BuilderRuntimeTaskRouteConfig      `json:"task_routes,omitempty"`
	UpgradeThreshold BuilderRuntimeUpgradeThresholdConfig `json:"upgrade_threshold,omitempty"`
}

type BuilderRuntimeTaskRouteConfig struct {
	TaskType string            `json:"task_type"`
	Model    *AgentModelConfig `json:"model,omitempty"`
}

type BuilderRuntimeUpgradeThresholdConfig struct {
	MaxAttemptsBeforeUpgrade    int     `json:"max_attempts_before_upgrade,omitempty"`
	MaxFilesBeforeUpgrade       int     `json:"max_files_before_upgrade,omitempty"`
	MaxSchemaDriftBeforeUpgrade int     `json:"max_schema_drift_before_upgrade,omitempty"`
	MaxUnrelatedOperationRate   float64 `json:"max_unrelated_operation_rate,omitempty"`
	UpgradeOnValidationFail     bool    `json:"upgrade_on_validation_fail,omitempty"`
	UpgradeOnPatchParseFail     bool    `json:"upgrade_on_patch_parse_fail,omitempty"`
	UpgradeOnScopeViolation     bool    `json:"upgrade_on_scope_violation,omitempty"`
	UpgradeOnSemanticConflict   bool    `json:"upgrade_on_semantic_conflict,omitempty"`
}

type ModelConfig struct {
	ModelName string `json:"model_name"`
	Model     string `json:"model"`

	APIBase   string   `json:"api_base,omitempty"`
	Proxy     string   `json:"proxy,omitempty"`
	Fallbacks []string `json:"fallbacks,omitempty"`

	AuthMethod  string `json:"auth_method,omitempty"`
	ConnectMode string `json:"connect_mode,omitempty"`
	Workspace   string `json:"workspace,omitempty"`

	RPM            int            `json:"rpm,omitempty"`
	MaxTokensField string         `json:"max_tokens_field,omitempty"`
	RequestTimeout int            `json:"request_timeout,omitempty"`
	ThinkingLevel  string         `json:"thinking_level,omitempty"`
	ExtraBody      map[string]any `json:"extra_body,omitempty"`

	secModelName string
	apiKeys      []string
	secDirty     bool
	isVirtual    bool
}

func (c *ModelConfig) UnmarshalJSON(data []byte) error {
	type alias ModelConfig
	var raw struct {
		*alias
		APIKey  string   `json:"api_key"`
		APIKeys []string `json:"api_keys"`
	}
	raw.alias = (*alias)(c)
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	c.apiKeys = MergeAPIKeys(raw.APIKey, raw.APIKeys)
	if len(c.apiKeys) > 0 {
		c.secDirty = true
	}
	return nil
}

func (c ModelConfig) MarshalJSON() ([]byte, error) {
	type raw struct {
		ModelName      string         `json:"model_name"`
		Model          string         `json:"model"`
		APIBase        string         `json:"api_base,omitempty"`
		Proxy          string         `json:"proxy,omitempty"`
		Fallbacks      []string       `json:"fallbacks,omitempty"`
		AuthMethod     string         `json:"auth_method,omitempty"`
		ConnectMode    string         `json:"connect_mode,omitempty"`
		Workspace      string         `json:"workspace,omitempty"`
		RPM            int            `json:"rpm,omitempty"`
		MaxTokensField string         `json:"max_tokens_field,omitempty"`
		RequestTimeout int            `json:"request_timeout,omitempty"`
		ThinkingLevel  string         `json:"thinking_level,omitempty"`
		ExtraBody      map[string]any `json:"extra_body,omitempty"`
	}
	return json.Marshal(raw{
		ModelName:      c.ModelName,
		Model:          c.Model,
		APIBase:        c.APIBase,
		Proxy:          c.Proxy,
		Fallbacks:      c.Fallbacks,
		AuthMethod:     c.AuthMethod,
		ConnectMode:    c.ConnectMode,
		Workspace:      c.Workspace,
		RPM:            c.RPM,
		MaxTokensField: c.MaxTokensField,
		RequestTimeout: c.RequestTimeout,
		ThinkingLevel:  c.ThinkingLevel,
		ExtraBody:      c.ExtraBody,
	})
}

func (c *ModelConfig) APIKey() string {
	if len(c.apiKeys) > 0 {
		return c.apiKeys[0]
	}
	return ""
}

func (c *ModelConfig) IsVirtual() bool {
	return c.isVirtual
}

func (c *ModelConfig) Validate() error {
	if strings.TrimSpace(c.ModelName) == "" {
		return fmt.Errorf("model_name is required")
	}
	if strings.TrimSpace(c.Model) == "" {
		return fmt.Errorf("model is required")
	}
	return nil
}

func (c *ModelConfig) SetAPIKey(value string) {
	if len(c.apiKeys) > 0 {
		c.apiKeys[0] = value
	} else {
		c.apiKeys = append(c.apiKeys, value)
	}
	c.secDirty = true
}

func LoadConfig(path string) (*Config, error) {
	logger.Debugf("loading config from %s", path)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			logger.WarnF("config file not found, using default config", map[string]any{"path": path})
			return DefaultConfig(), nil
		}
		return nil, err
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		return DefaultConfig(), nil
	}

	cfg := DefaultConfig()
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}
	if cfg.Version == 0 {
		cfg.Version = CurrentVersion
	}
	if cfg.Version != CurrentVersion {
		return nil, fmt.Errorf("unsupported config version: %d", cfg.Version)
	}

	sec, err := loadSecurityConfig(securityPath(path))
	if err != nil {
		return nil, fmt.Errorf("failed to load security config: %w", err)
	}
	if err := applySecurityConfig(cfg, sec); err != nil {
		return nil, fmt.Errorf("failed to apply security config: %w", err)
	}

	if passphrase := credential.PassphraseProvider(); passphrase != "" {
		for _, m := range cfg.ModelList {
			for _, k := range m.apiKeys {
				if k != "" && !strings.HasPrefix(k, "enc://") && !strings.HasPrefix(k, "file://") {
					fmt.Fprintf(os.Stderr, "oneappfactory: warning: model %q has a plaintext api_key; call SaveConfig to encrypt it\n", m.ModelName)
					break
				}
			}
		}
	}

	if err := resolveAPIKeys(cfg.ModelList, filepath.Dir(path)); err != nil {
		return nil, err
	}
	cfg.ModelList = expandMultiKeyModels(cfg.ModelList)
	if err := cfg.ValidateModelList(); err != nil {
		return nil, err
	}
	cfg.ensureWorkspace()
	return cfg, nil
}

func applySecurityConfig(cfg *Config, sec *SecurityConfig) error {
	sec = normalizeSecurityConfig(sec)
	names := toNameIndex(cfg.ModelList)
	for index, model := range cfg.ModelList {
		if len(model.apiKeys) > 0 {
			continue
		}
		if entry, exists := sec.ModelList[names[index]]; exists {
			model.apiKeys = append([]string(nil), entry.APIKeys...)
			model.secModelName = names[index]
			continue
		}
		if entry, exists := sec.ModelList[model.ModelName]; exists {
			model.apiKeys = append([]string(nil), entry.APIKeys...)
			model.secModelName = model.ModelName
		}
	}
	cfg.security = sec
	return nil
}

func toNameIndex(list []*ModelConfig) []string {
	nameList := make([]string, 0, len(list))
	countMap := make(map[string]int)
	for _, model := range list {
		name := model.ModelName
		index := countMap[name]
		nameList = append(nameList, fmt.Sprintf("%s:%d", name, index))
		countMap[name]++
	}
	return nameList
}

func encryptPlaintextAPIKeys(models map[string]ModelSecurityEntry, passphrase string) (map[string]ModelSecurityEntry, error) {
	sealed := make(map[string]ModelSecurityEntry, len(models))
	changed := false
	for name, model := range models {
		sealedEntry := ModelSecurityEntry{APIKeys: make([]string, len(model.APIKeys))}
		for index, key := range model.APIKeys {
			if key == "" || strings.HasPrefix(key, "enc://") || strings.HasPrefix(key, "file://") {
				sealedEntry.APIKeys[index] = key
				continue
			}
			encrypted, err := credential.Encrypt(passphrase, "", key)
			if err != nil {
				return nil, fmt.Errorf("cannot seal api_key for model %q: %w", name, err)
			}
			sealedEntry.APIKeys[index] = encrypted
			changed = true
		}
		sealed[name] = sealedEntry
	}
	if !changed {
		return nil, nil
	}
	return sealed, nil
}

func resolveAPIKeys(models []*ModelConfig, configDir string) error {
	resolver := credential.NewResolver(configDir)
	for modelIndex := range models {
		for keyIndex, key := range models[modelIndex].apiKeys {
			resolved, err := resolver.Resolve(key)
			if err != nil {
				return fmt.Errorf("model_list[%d] (%s): api_keys[%d]: %w", modelIndex, models[modelIndex].ModelName, keyIndex, err)
			}
			models[modelIndex].apiKeys[keyIndex] = resolved
		}
	}
	return nil
}

func SaveConfig(path string, cfg *Config) error {
	if cfg.security == nil {
		cfg.security = normalizeSecurityConfig(nil)
	}
	cfg.security = normalizeSecurityConfig(cfg.security)
	if cfg.Version == 0 {
		cfg.Version = CurrentVersion
	}
	cfg.ensureWorkspace()

	names := toNameIndex(cfg.ModelList)
	for index, model := range cfg.ModelList {
		if len(model.apiKeys) == 0 {
			continue
		}
		secName := model.secModelName
		if secName == "" {
			secName = names[index]
		}
		cfg.security.ModelList[secName] = ModelSecurityEntry{APIKeys: append([]string(nil), model.apiKeys...)}
		model.secModelName = secName
		model.secDirty = false
	}
	if passphrase := credential.PassphraseProvider(); passphrase != "" {
		sealed, err := encryptPlaintextAPIKeys(cfg.security.ModelList, passphrase)
		if err != nil {
			return err
		}
		if sealed != nil {
			cfg.security.ModelList = sealed
		}
	}
	if err := saveSecurityConfig(securityPath(path), cfg.security); err != nil {
		logger.ErrorCF("config", "cannot save .security.yml", map[string]any{"error": err})
		return err
	}

	nonVirtualModels := make([]*ModelConfig, 0, len(cfg.ModelList))
	for _, model := range cfg.ModelList {
		if !model.isVirtual {
			nonVirtualModels = append(nonVirtualModels, model)
		}
	}
	originalModelList := cfg.ModelList
	cfg.ModelList = nonVirtualModels
	data, err := json.MarshalIndent(cfg, "", "  ")
	cfg.ModelList = originalModelList
	if err != nil {
		return err
	}
	logger.Infof("saving config to %s", path)
	return fileutil.WriteFileAtomic(path, data, 0o600)
}

func (c *Config) ensureWorkspace() {
	if strings.TrimSpace(c.Workspace) == "" {
		c.Workspace = defaultWorkspacePath()
	}
}

func (c *Config) WorkspacePath() string {
	return expandHome(c.Workspace)
}

func expandHome(path string) string {
	if path == "" {
		return path
	}
	if path[0] == '~' {
		home, _ := os.UserHomeDir()
		if len(path) > 1 && path[1] == '/' {
			return home + path[1:]
		}
		return home
	}
	return path
}

func (c *Config) GetModelConfig(modelName string) (*ModelConfig, error) {
	matches := c.findMatches(modelName)
	if len(matches) == 0 {
		return nil, fmt.Errorf("model %q not found in model_list", modelName)
	}
	if len(matches) == 1 {
		return matches[0], nil
	}
	idx := (rrCounter.Add(1) - 1) % uint64(len(matches))
	return matches[idx], nil
}

func (c *Config) findMatches(modelName string) []*ModelConfig {
	var matches []*ModelConfig
	for _, model := range c.ModelList {
		if model.ModelName == modelName {
			matches = append(matches, model)
		}
	}
	return matches
}

func (c *Config) ValidateModelList() error {
	for index, model := range c.ModelList {
		if err := model.Validate(); err != nil {
			return fmt.Errorf("model_list[%d]: %w", index, err)
		}
	}
	return nil
}

func (c *Config) SecurityCopyFrom(cfg *Config) {
	if cfg == nil {
		return
	}
	c.security = cfg.security
	if c.security != nil {
		if err := applySecurityConfig(c, c.security); err != nil {
			logger.Errorf("failed to apply security config in SecurityCopyFrom: %v", err)
		}
	}
}

func (c *Config) ApplySecurity() error {
	return applySecurityConfig(c, c.security)
}

func MergeAPIKeys(apiKey string, apiKeys []string) []string {
	seen := make(map[string]struct{})
	all := make([]string, 0, len(apiKeys)+1)
	if key := strings.TrimSpace(apiKey); key != "" {
		seen[key] = struct{}{}
		all = append(all, key)
	}
	for _, key := range apiKeys {
		trimmed := strings.TrimSpace(key)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		all = append(all, trimmed)
	}
	return all
}

func expandMultiKeyModels(models []*ModelConfig) []*ModelConfig {
	expanded := make([]*ModelConfig, 0, len(models))
	for _, model := range models {
		keys := MergeAPIKeys("", model.apiKeys)
		if len(keys) <= 1 {
			model.apiKeys = keys
			expanded = append(expanded, model)
			continue
		}

		fallbackNames := make([]string, 0, len(keys)-1)
		for index := 1; index < len(keys); index++ {
			name := fmt.Sprintf("%s__key_%d", model.ModelName, index)
			entry := cloneModelConfig(model)
			entry.ModelName = name
			entry.apiKeys = []string{keys[index]}
			entry.Fallbacks = nil
			entry.isVirtual = true
			expanded = append(expanded, entry)
			fallbackNames = append(fallbackNames, name)
		}

		primary := cloneModelConfig(model)
		primary.apiKeys = []string{keys[0]}
		primary.Fallbacks = append(fallbackNames, model.Fallbacks...)
		primary.isVirtual = false
		expanded = append(expanded, primary)
	}
	return expanded
}

func cloneModelConfig(model *ModelConfig) *ModelConfig {
	clone := *model
	clone.Fallbacks = append([]string(nil), model.Fallbacks...)
	clone.apiKeys = append([]string(nil), model.apiKeys...)
	if model.ExtraBody != nil {
		clone.ExtraBody = make(map[string]any, len(model.ExtraBody))
		for key, value := range model.ExtraBody {
			clone.ExtraBody[key] = value
		}
	}
	return &clone
}

func defaultWorkspacePath() string {
	homePath := strings.TrimSpace(os.Getenv(EnvHome))
	if homePath == "" {
		home, _ := os.UserHomeDir()
		if home != "" {
			homePath = filepath.Join(home, pkg.DefaultOneAppFactoryHome)
		}
	}
	return filepath.Join(homePath, pkg.WorkspaceName)
}
