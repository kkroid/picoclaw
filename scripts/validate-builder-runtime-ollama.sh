#!/usr/bin/env bash

set -euo pipefail

OLLAMA_BASE_URL="${OLLAMA_BASE_URL:-http://10.12.11.159:11434}"
MODEL_NAME="${MODEL_NAME:-gemma4:26b}"

echo "[info] validating Ollama connectivity via ${OLLAMA_BASE_URL}"

tags_response="$(curl -sS --max-time 10 "${OLLAMA_BASE_URL}/api/tags")"
echo "[info] /api/tags reachable"

if ! printf '%s' "$tags_response" | grep -F "\"name\":\"${MODEL_NAME}\"" >/dev/null 2>&1; then
	echo "[error] model ${MODEL_NAME} not found in /api/tags" >&2
	exit 1
fi

models_response="$(curl -sS --max-time 10 "${OLLAMA_BASE_URL}/v1/models")"
echo "[info] /v1/models reachable"

if ! printf '%s' "$models_response" | grep -F "\"id\":\"${MODEL_NAME}\"" >/dev/null 2>&1; then
	echo "[error] model ${MODEL_NAME} not found in /v1/models" >&2
	exit 1
fi

echo "[info] running structured JSON probe"
simple_response="$(curl -sS --max-time 120 "${OLLAMA_BASE_URL}/v1/chat/completions" \
	-H 'Content-Type: application/json' \
	-d @- <<EOF
{
  "model": "${MODEL_NAME}",
  "temperature": 0,
  "messages": [
    {
      "role": "system",
      "content": "You are a coding model. Reply with only compact JSON."
    },
    {
      "role": "user",
      "content": "Return JSON with keys task, verdict, patch. task must be narrow_dart_edit. verdict must be ok. patch must be an array with one object: {path:'lib/main.dart', intent:'replace_title_text'}."
    }
  ]
}
EOF
)"

if ! printf '%s' "$simple_response" | grep -F 'narrow_dart_edit' >/dev/null 2>&1; then
  echo "[error] structured JSON probe did not return expected task marker" >&2
	echo "$simple_response" >&2
	exit 1
fi

echo "[info] running workspace patch style probe"
patch_response="$(curl -sS --max-time 180 "${OLLAMA_BASE_URL}/v1/chat/completions" \
	-H 'Content-Type: application/json' \
	-d @- <<EOF
{
  "model": "${MODEL_NAME}",
  "temperature": 0,
  "messages": [
    {
      "role": "system",
      "content": "You are a builder-runtime patch generator. Output only compact JSON. No markdown."
    },
    {
      "role": "user",
      "content": "Generate a minimal WorkspacePatch-style JSON object for a narrow Flutter task. Task: update lib/main.dart so MaterialApp title becomes 'Budget Flow'. Allowed path: lib/main.dart only. Output format: {patch_id:string, operations:[{type:string,path:string,anchor:string,new_content:string}]}. Use type replace_block. Use anchor exactly \"title: 'Old Title',\"."
    }
  ]
}
EOF
)"

if ! printf '%s' "$patch_response" | grep -F 'update_app_title' >/dev/null 2>&1; then
	echo "[error] patch probe did not return expected patch_id" >&2
	echo "$patch_response" >&2
	exit 1
fi

if ! printf '%s' "$patch_response" | grep -F 'lib/main.dart' >/dev/null 2>&1; then
	echo "[error] patch probe did not stay on allowed path" >&2
	echo "$patch_response" >&2
	exit 1
fi

echo "[ok] Ollama connectivity and minimal builder-runtime probes passed for ${MODEL_NAME}"