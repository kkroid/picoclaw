// OneAppFactory - Android app factory
// License: MIT
//
// Copyright (c) 2026 OneAppFactory contributors

package providers

import (
	"fmt"

	"github.com/sipeed/oneappfactory/pkg/config"
)

// CreateProvider creates a provider from the first configured model.
func CreateProvider(cfg *config.Config) (LLMProvider, string, error) {
	if cfg == nil || len(cfg.ModelList) == 0 {
		return nil, "", fmt.Errorf("no providers configured. Please add entries to model_list in your config")
	}
	modelCfg := cfg.ModelList[0]

	// Inject global workspace if not set in model config
	if modelCfg.Workspace == "" {
		modelCfg.Workspace = cfg.WorkspacePath()
	}

	// Use factory to create provider
	provider, modelID, err := CreateProviderFromConfig(modelCfg)
	if err != nil {
		return nil, "", fmt.Errorf("failed to create provider for model %q: %w", modelCfg.ModelName, err)
	}

	return provider, modelID, nil
}
