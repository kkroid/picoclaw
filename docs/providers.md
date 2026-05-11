# Providers

OneAppFactory uses provider entries only for AppFactory planning and builder runtime model calls. It does not expose a general chat model management product surface.

Provider entries live in `model_list`:

```json
{
  "model_name": "qwen2.5-coder-14b-local",
  "model": "ollama/qwen2.5-coder:14b",
  "api_base": "http://127.0.0.1:11434/v1"
}
```

`model_name` is the local alias referenced by AppFactory config. `model` is the provider/model identifier sent to the provider adapter.

## Common Patterns

### Ollama / Local OpenAI-Compatible Endpoint

```json
{
  "model_name": "local-coder",
  "model": "ollama/qwen2.5-coder:14b",
  "api_base": "http://127.0.0.1:11434/v1"
}
```

### OpenAI-Compatible API

```json
{
  "model_name": "openai-compatible-coder",
  "model": "openai/gpt-4.1-mini",
  "api_base": "https://api.openai.com/v1",
  "api_key": "YOUR_API_KEY"
}
```

### OpenRouter

Use the full OpenRouter model id in `model`:

```json
{
  "model_name": "openrouter-free",
  "model": "openrouter/free",
  "api_base": "https://openrouter.ai/api/v1",
  "api_key": "YOUR_OPENROUTER_KEY"
}
```

### Anthropic Messages

```json
{
  "model_name": "claude-coder",
  "model": "anthropic-messages/claude-sonnet-4-5",
  "api_key": "YOUR_ANTHROPIC_KEY"
}
```

### Azure OpenAI

```json
{
  "model_name": "azure-coder",
  "model": "azure/gpt-4.1-mini",
  "api_base": "https://YOUR_RESOURCE.openai.azure.com/openai/deployments/YOUR_DEPLOYMENT",
  "api_key": "YOUR_AZURE_KEY"
}
```

### Bedrock

Bedrock uses AWS credentials from the environment or the default AWS credential chain.

```json
{
  "model_name": "bedrock-coder",
  "model": "bedrock/us.anthropic.claude-3-7-sonnet-20250219-v1:0"
}
```

## Fallbacks

A model entry can declare fallback aliases:

```json
{
  "model_name": "primary-coder",
  "model": "ollama/qwen2.5-coder:14b",
  "api_base": "http://127.0.0.1:11434/v1",
  "fallbacks": ["larger-coder"]
}
```

Builder runtime routes can also set `default_model.fallbacks`. Prefer explicit aliases so logs and regression summaries show which model failed and which model took over.

## Troubleshooting

If a run reports `model ... not found in model_list`, verify that every alias referenced by `planning_engine`, `builder_runtime.default_model`, `builder_runtime.upgrade_model`, and `builder_runtime.task_routes` exists in `model_list`.

For OpenRouter, do not use shorthand model ids such as `free`; use `openrouter/free` or a concrete provider model id.
