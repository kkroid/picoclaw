package config

// DefaultConfig returns the default OneAppFactory configuration.
func DefaultConfig() *Config {
	return &Config{
		Version:   CurrentVersion,
		Workspace: defaultWorkspacePath(),
		AppFactory: AppFactoryConfig{
			BuilderRuntime: BuilderRuntimeConfig{
				Enabled: false,
				UpgradeThreshold: BuilderRuntimeUpgradeThresholdConfig{
					MaxAttemptsBeforeUpgrade: 2,
					MaxFilesBeforeUpgrade:    2,
					UpgradeOnValidationFail:  true,
					UpgradeOnPatchParseFail:  true,
					UpgradeOnScopeViolation:  true,
				},
			},
		},
		ModelList: defaultModelList(),
		BuildInfo: BuildInfo{
			Version:   Version,
			GitCommit: GitCommit,
			BuildTime: BuildTime,
			GoVersion: GoVersion,
		},
		security: normalizeSecurityConfig(nil),
	}
}

func defaultModelList() []*ModelConfig {
	return []*ModelConfig{
		{
			ModelName: "glm-4.7",
			Model:     "zhipu/glm-4.7",
			APIBase:   "https://open.bigmodel.cn/api/paas/v4",
		},
		{
			ModelName: "gpt-5.4",
			Model:     "openai/gpt-5.4",
			APIBase:   "https://api.openai.com/v1",
		},
		{
			ModelName: "claude-sonnet-4.6",
			Model:     "anthropic/claude-sonnet-4.6",
			APIBase:   "https://api.anthropic.com/v1",
		},
		{
			ModelName: "deepseek-chat",
			Model:     "deepseek/deepseek-chat",
			APIBase:   "https://api.deepseek.com/v1",
		},
		{
			ModelName: "gemini-2.0-flash",
			Model:     "gemini/gemini-2.0-flash-exp",
			APIBase:   "https://generativelanguage.googleapis.com/v1beta",
		},
		{
			ModelName: "qwen-plus",
			Model:     "qwen/qwen-plus",
			APIBase:   "https://dashscope.aliyuncs.com/compatible-mode/v1",
		},
		{
			ModelName: "openrouter-auto",
			Model:     "openrouter/auto",
			APIBase:   "https://openrouter.ai/api/v1",
		},
		{
			ModelName: "openrouter-gpt-5.4",
			Model:     "openrouter/openai/gpt-5.4",
			APIBase:   "https://openrouter.ai/api/v1",
		},
		{
			ModelName: "ark-code-latest",
			Model:     "volcengine/ark-code-latest",
			APIBase:   "https://ark.cn-beijing.volces.com/api/v3",
		},
		{
			ModelName:  "gemini-flash",
			Model:      "antigravity/gemini-3-flash",
			AuthMethod: "oauth",
		},
		{
			ModelName:  "copilot-gpt-5.4",
			Model:      "github-copilot/gpt-5.4",
			APIBase:    "http://localhost:4321",
			AuthMethod: "oauth",
		},
		{
			ModelName: "llama3",
			Model:     "ollama/llama3",
			APIBase:   "http://localhost:11434/v1",
		},
		{
			ModelName: "MiniMax-M2.5",
			Model:     "minimax/MiniMax-M2.5",
			APIBase:   "https://api.minimaxi.com/v1",
			ExtraBody: map[string]any{"reasoning_split": true},
		},
		{
			ModelName: "local-model",
			Model:     "vllm/custom-model",
			APIBase:   "http://localhost:8000/v1",
		},
	}
}
