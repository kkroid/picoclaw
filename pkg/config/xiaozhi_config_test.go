package config

import (
	"encoding/json"
	"testing"
)

func TestXiaozhiConfig_DefaultOwnerAndSessionScope(t *testing.T) {
	jsonData := `{
		"channels": {
			"xiaozhi": {
				"enabled": true,
				"default_owner_id": "kkroid",
				"session_scope": "per-owner",
				"asr_provider": "doubao",
				"tts_provider": "doubao"
			}
		}
	}`

	cfg := DefaultConfig()
	if err := json.Unmarshal([]byte(jsonData), cfg); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if !cfg.Channels.Xiaozhi.Enabled {
		t.Fatal("Channels.Xiaozhi.Enabled = false, want true")
	}
	if cfg.Channels.Xiaozhi.EffectiveDefaultOwnerID() != "kkroid" {
		t.Fatalf("EffectiveDefaultOwnerID = %q, want %q", cfg.Channels.Xiaozhi.EffectiveDefaultOwnerID(), "kkroid")
	}
	if cfg.Channels.Xiaozhi.EffectiveSessionScope() != XiaozhiSessionScopePerOwner {
		t.Fatalf(
			"EffectiveSessionScope = %q, want %q",
			cfg.Channels.Xiaozhi.EffectiveSessionScope(),
			XiaozhiSessionScopePerOwner,
		)
	}
	if cfg.Channels.Xiaozhi.ASRProvider != "doubao" || cfg.Channels.Xiaozhi.TTSProvider != "doubao" {
		t.Fatalf("Xiaozhi providers = %+v", cfg.Channels.Xiaozhi)
	}
}

func TestXiaozhiConfig_LegacyOwnerAlias(t *testing.T) {
	jsonData := `{
		"channels": {
			"xiaozhi": {
				"enabled": true,
				"owner_id": "legacy-owner"
			}
		}
	}`

	cfg := DefaultConfig()
	if err := json.Unmarshal([]byte(jsonData), cfg); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if cfg.Channels.Xiaozhi.EffectiveDefaultOwnerID() != "legacy-owner" {
		t.Fatalf(
			"EffectiveDefaultOwnerID = %q, want %q",
			cfg.Channels.Xiaozhi.EffectiveDefaultOwnerID(),
			"legacy-owner",
		)
	}
	if cfg.Channels.Xiaozhi.EffectiveSessionScope() != XiaozhiSessionScopePerOwnerDevice {
		t.Fatalf(
			"EffectiveSessionScope = %q, want %q",
			cfg.Channels.Xiaozhi.EffectiveSessionScope(),
			XiaozhiSessionScopePerOwnerDevice,
		)
	}
}
