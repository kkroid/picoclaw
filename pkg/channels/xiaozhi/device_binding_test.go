package xiaozhi

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestVoiceDeviceStore_BindOwnerAndResolve(t *testing.T) {
	workspace := t.TempDir()
	store := newVoiceDeviceStore(workspace)
	now := time.Date(2026, 3, 18, 12, 0, 0, 0, time.UTC)
	store.now = func() time.Time { return now }

	if err := store.BindOwner("Desk Speaker", "KKRoid", "conn-1"); err != nil {
		t.Fatalf("BindOwner: %v", err)
	}

	ownerID, err := store.ResolveOwner("Desk Speaker")
	if err != nil {
		t.Fatalf("ResolveOwner: %v", err)
	}
	if ownerID != "kkroid" {
		t.Fatalf("ResolveOwner = %q, want kkroid", ownerID)
	}

	data, err := os.ReadFile(filepath.Join(workspace, "state", "devices", "desk-speaker.json"))
	if err != nil {
		t.Fatalf("Read binding file: %v", err)
	}

	var binding voiceDeviceBinding
	if err := json.Unmarshal(data, &binding); err != nil {
		t.Fatalf("Unmarshal binding: %v", err)
	}
	if binding.OwnerID != "kkroid" {
		t.Fatalf("binding.OwnerID = %q, want kkroid", binding.OwnerID)
	}
	if binding.BindingSource != ownerBindingSourceExplicit {
		t.Fatalf("binding.BindingSource = %q, want %q", binding.BindingSource, ownerBindingSourceExplicit)
	}
}

func TestVoiceDeviceStore_BindOwner_NormalizesMACStyleDeviceIDForFilePath(t *testing.T) {
	workspace := t.TempDir()
	store := newVoiceDeviceStore(workspace)

	if err := store.BindOwner("AA:BB:CC:DD:EE:FF", "KKRoid", "conn-1"); err != nil {
		t.Fatalf("BindOwner: %v", err)
	}

	if _, err := os.Stat(filepath.Join(workspace, "state", "devices", "aa-bb-cc-dd-ee-ff.json")); err != nil {
		t.Fatalf("expected normalized MAC binding file: %v", err)
	}
}
