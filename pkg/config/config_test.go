package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/sipeed/oneappfactory/pkg/credential"
)

func mustSetupSSHKey(t *testing.T) {
	t.Helper()
	keyPath := filepath.Join(t.TempDir(), "oneappfactory_ed25519.key")
	if err := credential.GenerateSSHKey(keyPath); err != nil {
		t.Fatalf("mustSetupSSHKey: %v", err)
	}
	t.Setenv("ONEAPPFACTORY_SSH_KEY_PATH", keyPath)
}

func TestAgentModelConfig_UnmarshalString(t *testing.T) {
	var model AgentModelConfig
	if err := json.Unmarshal([]byte(`"gpt-4"`), &model); err != nil {
		t.Fatalf("unmarshal string: %v", err)
	}
	if model.Primary != "gpt-4" || model.Fallbacks != nil {
		t.Fatalf("model = %#v, want primary only", model)
	}
}

func TestAgentModelConfig_UnmarshalObject(t *testing.T) {
	var model AgentModelConfig
	data := `{"primary":"claude-opus","fallbacks":["gpt-4o-mini","haiku"]}`
	if err := json.Unmarshal([]byte(data), &model); err != nil {
		t.Fatalf("unmarshal object: %v", err)
	}
	if model.Primary != "claude-opus" || len(model.Fallbacks) != 2 {
		t.Fatalf("model = %#v", model)
	}
}

func TestConfig_AppFactoryBuilderRuntimeParse(t *testing.T) {
	jsonData := `{
		"appfactory": {
			"builder_runtime": {
				"enabled": true,
				"default_model": {"primary": "qwen2.5-coder-14b-local", "fallbacks": ["gpt-5.4"]},
				"upgrade_model": "qwen2.5-coder-32b-local",
				"task_routes": [
					{"task_type": "single_file_edit", "model": "qwen2.5-coder-14b-local"},
					{"task_type": "high_risk_repair", "model": {"primary": "qwen2.5-coder-32b-local", "fallbacks": ["gpt-5.4"]}}
				],
				"upgrade_threshold": {
					"max_attempts_before_upgrade": 2,
					"max_files_before_upgrade": 3,
					"upgrade_on_validation_fail": true,
					"upgrade_on_patch_parse_fail": true,
					"upgrade_on_scope_violation": true
				}
			}
		}
	}`

	cfg := DefaultConfig()
	if err := json.Unmarshal([]byte(jsonData), cfg); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !cfg.AppFactory.BuilderRuntime.Enabled {
		t.Fatal("BuilderRuntime should be enabled")
	}
	if cfg.AppFactory.BuilderRuntime.DefaultModel == nil || cfg.AppFactory.BuilderRuntime.DefaultModel.Primary != "qwen2.5-coder-14b-local" {
		t.Fatalf("DefaultModel = %+v", cfg.AppFactory.BuilderRuntime.DefaultModel)
	}
	if cfg.AppFactory.BuilderRuntime.UpgradeModel == nil || cfg.AppFactory.BuilderRuntime.UpgradeModel.Primary != "qwen2.5-coder-32b-local" {
		t.Fatalf("UpgradeModel = %+v", cfg.AppFactory.BuilderRuntime.UpgradeModel)
	}
	if len(cfg.AppFactory.BuilderRuntime.TaskRoutes) != 2 {
		t.Fatalf("TaskRoutes len = %d, want 2", len(cfg.AppFactory.BuilderRuntime.TaskRoutes))
	}
	if cfg.AppFactory.BuilderRuntime.UpgradeThreshold.MaxFilesBeforeUpgrade != 3 {
		t.Fatalf("MaxFilesBeforeUpgrade = %d, want 3", cfg.AppFactory.BuilderRuntime.UpgradeThreshold.MaxFilesBeforeUpgrade)
	}
}

func TestDefaultConfig_WorkspacePathDefault(t *testing.T) {
	t.Setenv("ONEAPPFACTORY_HOME", "")
	fakeHome := "/tmp/home"
	if runtime.GOOS == "windows" {
		fakeHome = `C:\tmp\home`
		t.Setenv("USERPROFILE", fakeHome)
	} else {
		t.Setenv("HOME", fakeHome)
	}

	cfg := DefaultConfig()
	want := filepath.Join(fakeHome, ".appfactory", "workspace")
	if cfg.Workspace != want {
		t.Fatalf("workspace = %q, want %q", cfg.Workspace, want)
	}
}

func TestDefaultConfig_WorkspacePathWithHomeEnv(t *testing.T) {
	t.Setenv("ONEAPPFACTORY_HOME", "/custom/oneappfactory/home")
	cfg := DefaultConfig()
	want := filepath.Join("/custom/oneappfactory/home", "workspace")
	if cfg.Workspace != want {
		t.Fatalf("workspace = %q, want %q", cfg.Workspace, want)
	}
}

func TestSaveConfig_FilePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("file permission bits are not enforced on Windows")
	}
	path := filepath.Join(t.TempDir(), "config.json")
	if err := SaveConfig(path, DefaultConfig()); err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat failed: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("config file has permission %04o, want 0600", perm)
	}
}

func TestModelConfigDirectAPIKeyRoundTripMovesToSecurity(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	data := `{"version":1,"workspace":"./workspace","model_list":[{"model_name":"test","model":"openai/gpt-4","api_key":"sk-plaintext"}]}`
	if err := os.WriteFile(cfgPath, []byte(data), 0o600); err != nil {
		t.Fatalf("setup: %v", err)
	}

	cfg, err := LoadConfig(cfgPath)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.ModelList[0].APIKey() != "sk-plaintext" {
		t.Fatalf("api_key = %q, want plaintext", cfg.ModelList[0].APIKey())
	}
	if err := SaveConfig(cfgPath, cfg); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}
	raw, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if strings.Contains(string(raw), "sk-plaintext") || strings.Contains(string(raw), "api_key") {
		t.Fatalf("saved config should not include API keys: %s", string(raw))
	}
	secRaw, err := os.ReadFile(filepath.Join(dir, SecurityConfigFile))
	if err != nil {
		t.Fatalf("ReadFile security: %v", err)
	}
	if !strings.Contains(string(secRaw), "sk-plaintext") {
		t.Fatalf("security sidecar should contain API key, got: %s", string(secRaw))
	}
}

func TestSaveConfig_EncryptsPlaintextAPIKey(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	t.Setenv("ONEAPPFACTORY_KEY_PASSPHRASE", "test-passphrase")
	mustSetupSSHKey(t)

	cfg := DefaultConfig()
	cfg.ModelList = []*ModelConfig{{ModelName: "test", Model: "openai/gpt-4", apiKeys: []string{"sk-plaintext"}}}
	if err := SaveConfig(cfgPath, cfg); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}
	raw, _ := os.ReadFile(filepath.Join(dir, SecurityConfigFile))
	if !strings.Contains(string(raw), "enc://") || strings.Contains(string(raw), "sk-plaintext") {
		t.Fatalf("security sidecar should contain encrypted key only, got:\n%s", raw)
	}

	loaded, err := LoadConfig(cfgPath)
	if err != nil {
		t.Fatalf("LoadConfig after SaveConfig: %v", err)
	}
	if loaded.ModelList[0].APIKey() != "sk-plaintext" {
		t.Fatalf("loaded api_key = %q, want plaintext", loaded.ModelList[0].APIKey())
	}
}

func TestModelConfig_ExtraBodyRoundTrip(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	cfg := &Config{
		Version:   CurrentVersion,
		Workspace: "./workspace",
		ModelList: []*ModelConfig{{
			ModelName: "test-model",
			Model:     "openai/test",
			apiKeys:   []string{"sk-test"},
			ExtraBody: map[string]any{"custom_field": "value", "num_field": 42},
		}},
	}
	if err := SaveConfig(cfgPath, cfg); err != nil {
		t.Fatalf("SaveConfig error: %v", err)
	}
	loaded, err := LoadConfig(cfgPath)
	if err != nil {
		t.Fatalf("LoadConfig error: %v", err)
	}
	if got := loaded.ModelList[0].ExtraBody["custom_field"]; got != "value" {
		t.Fatalf("ExtraBody[custom_field] = %v, want value", got)
	}
	if got := loaded.ModelList[0].ExtraBody["num_field"]; got != float64(42) {
		t.Fatalf("ExtraBody[num_field] = %v, want 42", got)
	}
}

func TestDefaultConfig_MinimaxExtraBody(t *testing.T) {
	cfg := DefaultConfig()
	var minimaxCfg *ModelConfig
	for _, model := range cfg.ModelList {
		if model.Model == "minimax/MiniMax-M2.5" {
			minimaxCfg = model
			break
		}
	}
	if minimaxCfg == nil || minimaxCfg.ExtraBody == nil || minimaxCfg.ExtraBody["reasoning_split"] != true {
		t.Fatalf("Minimax ExtraBody = %#v", minimaxCfg)
	}
}

func TestFilterSensitiveData(t *testing.T) {
	cfg := &Config{}
	if got := cfg.FilterSensitiveData("hello sk-key123 world"); got != "hello sk-key123 world" {
		t.Fatalf("nil security got %q", got)
	}
	cfg.security = &SecurityConfig{ModelList: map[string]ModelSecurityEntry{"test": {APIKeys: []string{"sk-long-key-12345"}}}}
	content := "Your API key is sk-long-key-12345"
	if got := cfg.FilterSensitiveData(content); got != "Your API key is [FILTERED]" {
		t.Fatalf("filtering got %q", got)
	}
}
