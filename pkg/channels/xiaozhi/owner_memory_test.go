package xiaozhi

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sipeed/picoclaw/pkg/providers"
)

func TestBuildOwnerVoiceSnapshot_RecentTurns(t *testing.T) {
	now := time.Date(2026, 3, 18, 9, 30, 0, 0, time.UTC)
	history := []providers.Message{
		{Role: "system", Content: "ignore system"},
		{Role: "user", Content: "u1"},
		{Role: "assistant", Content: "a1"},
		{Role: "tool", Content: "ignore tool"},
		{Role: "user", Content: "u2"},
		{Role: "assistant", Content: "a2"},
		{Role: "user", Content: "u3"},
		{Role: "assistant", Content: "a3"},
		{Role: "user", Content: "u4"},
	}

	snapshot := buildOwnerVoiceSnapshot(
		"KKRoid",
		"xiaozhi:owner:kkroid",
		"xiaozhi:owner:kkroid:device:desk",
		"xiaozhi",
		"Desk Speaker",
		"turn-1",
		"owner summary",
		history,
		now,
	)

	if snapshot.OwnerID != "kkroid" {
		t.Fatalf("OwnerID = %q, want kkroid", snapshot.OwnerID)
	}
	if snapshot.DeviceID != "desk-speaker" {
		t.Fatalf("DeviceID = %q, want desk-speaker", snapshot.DeviceID)
	}
	if len(snapshot.RecentTurns) != 6 {
		t.Fatalf("RecentTurns len = %d, want 6", len(snapshot.RecentTurns))
	}
	if snapshot.RecentTurns[0].Content != "a1" {
		t.Fatalf("first recent turn = %+v", snapshot.RecentTurns[0])
	}
	if snapshot.RecentTurns[5].Content != "u4" {
		t.Fatalf("last recent turn = %+v", snapshot.RecentTurns[5])
	}
	if !snapshot.LastUpdatedAt.Equal(now) {
		t.Fatalf("LastUpdatedAt = %v, want %v", snapshot.LastUpdatedAt, now)
	}
}

func TestOwnerVoiceStore_WriteSnapshot(t *testing.T) {
	workspace := t.TempDir()
	store := newOwnerStore(workspace)
	now := time.Date(2026, 3, 18, 10, 0, 0, 0, time.UTC)
	store.now = func() time.Time { return now }

	snapshot := ownerVoiceSnapshot{
		OwnerID:     "KKRoid",
		MemoryKey:   "xiaozhi:owner:kkroid",
		SessionKey:  "xiaozhi:owner:kkroid:device:desk",
		Channel:     "xiaozhi",
		DeviceID:    "Desk Speaker",
		LastTurnID:  "turn-42",
		Summary:     "owner summary",
		RecentTurns: []ownerVoiceTurn{{Role: "user", Content: "hello"}},
	}

	if err := store.WriteSnapshot(snapshot); err != nil {
		t.Fatalf("WriteSnapshot: %v", err)
	}

	path := filepath.Join(workspace, "memory", "owners", "kkroid", ownerVoiceContextFilename)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	var got ownerVoiceSnapshot
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if got.OwnerID != "kkroid" {
		t.Fatalf("OwnerID = %q, want kkroid", got.OwnerID)
	}
	if got.DeviceID != "desk-speaker" {
		t.Fatalf("DeviceID = %q, want desk-speaker", got.DeviceID)
	}
	if got.LastTurnID != "turn-42" {
		t.Fatalf("LastTurnID = %q, want turn-42", got.LastTurnID)
	}
	if !got.LastUpdatedAt.Equal(now) {
		t.Fatalf("LastUpdatedAt = %v, want %v", got.LastUpdatedAt, now)
	}
	if len(got.RecentTurns) != 1 || got.RecentTurns[0].Content != "hello" {
		t.Fatalf("RecentTurns = %+v", got.RecentTurns)
	}

	for _, filename := range []string{ownerProfileFilename, ownerSubscriptionsFilename, ownerRemindersFilename, ownerWatchlistFilename} {
		if _, err := os.Stat(filepath.Join(workspace, "memory", "owners", "kkroid", filename)); err != nil {
			t.Fatalf("expected %s to exist: %v", filename, err)
		}
	}
}

func TestOwnerStore_BindDeviceWritesProfile(t *testing.T) {
	workspace := t.TempDir()
	store := newOwnerStore(workspace)
	now := time.Date(2026, 3, 18, 11, 0, 0, 0, time.UTC)
	store.now = func() time.Time { return now }

	if err := store.BindDevice("KKRoid", "Desk Speaker", ownerBindingSourceExplicit); err != nil {
		t.Fatalf("BindDevice: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(workspace, "memory", "owners", "kkroid", ownerProfileFilename))
	if err != nil {
		t.Fatalf("Read profile: %v", err)
	}

	var profile ownerProfile
	if err := json.Unmarshal(data, &profile); err != nil {
		t.Fatalf("Unmarshal profile: %v", err)
	}
	if len(profile.BoundDevices) != 1 {
		t.Fatalf("BoundDevices len = %d, want 1", len(profile.BoundDevices))
	}
	if profile.BoundDevices[0].DeviceID != "desk-speaker" {
		t.Fatalf("DeviceID = %q, want desk-speaker", profile.BoundDevices[0].DeviceID)
	}
	if profile.BoundDevices[0].BindingSource != ownerBindingSourceExplicit {
		t.Fatalf("BindingSource = %q, want %q", profile.BoundDevices[0].BindingSource, ownerBindingSourceExplicit)
	}
}
