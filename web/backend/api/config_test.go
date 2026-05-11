package api

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/sipeed/oneappfactory/pkg/config"
)

func setupConfigTestEnv(t *testing.T) (string, func()) {
	t.Helper()

	tmp := t.TempDir()
	oldHome := os.Getenv("HOME")
	oldOneAppFactoryHome := os.Getenv("ONEAPPFACTORY_HOME")

	if err := os.Setenv("HOME", tmp); err != nil {
		t.Fatalf("set HOME: %v", err)
	}
	if err := os.Setenv("ONEAPPFACTORY_HOME", filepath.Join(tmp, ".appfactory")); err != nil {
		t.Fatalf("set ONEAPPFACTORY_HOME: %v", err)
	}

	cfg := config.DefaultConfig()
	cfg.ModelList = []*config.ModelConfig{{
		ModelName: "custom-default",
		Model:     "openai/gpt-4o",
	}}
	cfg.WithSecurity(&config.SecurityConfig{
		ModelList: map[string]config.ModelSecurityEntry{
			"custom-default": {APIKeys: []string{"sk-default"}},
		},
	})

	configPath := filepath.Join(tmp, "config.json")
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig error: %v", err)
	}

	cleanup := func() {
		_ = os.Setenv("HOME", oldHome)
		if oldOneAppFactoryHome == "" {
			_ = os.Unsetenv("ONEAPPFACTORY_HOME")
		} else {
			_ = os.Setenv("ONEAPPFACTORY_HOME", oldOneAppFactoryHome)
		}
	}
	return configPath, cleanup
}

func TestHandleUpdateConfig_StoresOneAppFactoryConfig(t *testing.T) {
	configPath, cleanup := setupConfigTestEnv(t)
	defer cleanup()

	h := NewHandler(configPath)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodPut, "/api/config", bytes.NewBufferString(`{
		"version": 1,
		"workspace": "~/.appfactory/workspace",
		"appfactory": {
			"builder_runtime": {
				"enabled": true,
				"default_model": "custom-default"
			}
		},
		"model_list": [
			{"model_name": "custom-default", "model": "openai/gpt-4o", "api_key": "sk-updated"}
		]
	}`))
	req.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	if cfg.Workspace != "~/.appfactory/workspace" {
		t.Fatalf("workspace = %q", cfg.Workspace)
	}
	if !cfg.AppFactory.BuilderRuntime.Enabled {
		t.Fatal("builder runtime should be enabled")
	}
	if cfg.ModelList[0].APIKey() != "sk-updated" {
		t.Fatalf("api key = %q, want updated key", cfg.ModelList[0].APIKey())
	}
}

func TestHandlePatchConfig_RejectsInvalidModel(t *testing.T) {
	configPath, cleanup := setupConfigTestEnv(t)
	defer cleanup()

	h := NewHandler(configPath)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodPatch, "/api/config", bytes.NewBufferString(`{
		"model_list": [{"model_name": "", "model": "openai/gpt-4o"}]
	}`))
	req.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte("model_name is required")) {
		t.Fatalf("expected model validation error, body=%s", rec.Body.String())
	}
}

func TestHandlePatchConfig_UpdatesWorkspace(t *testing.T) {
	configPath, cleanup := setupConfigTestEnv(t)
	defer cleanup()

	h := NewHandler(configPath)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodPatch, "/api/config", bytes.NewBufferString(`{"workspace":"/tmp/oneappfactory-workspace"}`))
	req.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	if cfg.Workspace != "/tmp/oneappfactory-workspace" {
		t.Fatalf("workspace = %q", cfg.Workspace)
	}
}
