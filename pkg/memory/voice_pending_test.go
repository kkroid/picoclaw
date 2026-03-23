package memory

import (
	"strings"
	"testing"
	"time"
)

func TestVoicePendingStore_EnqueuePrepareAndConfirm(t *testing.T) {
	workspace := t.TempDir()
	store := NewVoicePendingStore(workspace)
	now := time.Date(2026, 3, 18, 8, 30, 0, 0, time.UTC)
	store.now = func() time.Time { return now }

	if err := store.Enqueue("KKRoid", VoicePendingItem{Title: "晨报", Content: "今天上海多云，记得带伞"}); err != nil {
		t.Fatalf("Enqueue 1: %v", err)
	}
	if err := store.Enqueue("KKRoid", VoicePendingItem{Content: "下午三点提醒你开会"}); err != nil {
		t.Fatalf("Enqueue 2: %v", err)
	}

	queue, err := store.Load("KKRoid")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if queue.OwnerID != "kkroid" {
		t.Fatalf("OwnerID = %q, want kkroid", queue.OwnerID)
	}
	if len(queue.Items) != 2 {
		t.Fatalf("Items len = %d, want 2", len(queue.Items))
	}

	prepared, err := store.PrepareFirstDailySummary("KKRoid", 3, 20)
	if err != nil {
		t.Fatalf("PrepareFirstDailySummary: %v", err)
	}
	if len(prepared.Items) != 2 {
		t.Fatalf("prepared items len = %d, want 2", len(prepared.Items))
	}
	summary := prepared.Summary
	if !strings.Contains(summary, "2 条待播更新") {
		t.Fatalf("summary = %q, want pending count", summary)
	}
	if !strings.Contains(summary, "晨报") {
		t.Fatalf("summary = %q, want title", summary)
	}

	queue, err = store.Load("KKRoid")
	if err != nil {
		t.Fatalf("Load after prepare: %v", err)
	}
	if len(queue.Items) != 2 {
		t.Fatalf("remaining items len after prepare = %d, want 2", len(queue.Items))
	}

	itemIDs := []string{prepared.Items[0].ID, prepared.Items[1].ID}
	if err := store.ConfirmFirstDailySummary("KKRoid", itemIDs); err != nil {
		t.Fatalf("ConfirmFirstDailySummary: %v", err)
	}

	queue, err = store.Load("KKRoid")
	if err != nil {
		t.Fatalf("Load after confirm: %v", err)
	}
	if len(queue.Items) != 0 {
		t.Fatalf("remaining items len = %d, want 0", len(queue.Items))
	}
	if !queue.LastAnnouncedAt.Equal(now) {
		t.Fatalf("LastAnnouncedAt = %v, want %v", queue.LastAnnouncedAt, now)
	}

	prepared, err = store.PrepareFirstDailySummary("KKRoid", 3, 20)
	if err != nil {
		t.Fatalf("PrepareFirstDailySummary second call: %v", err)
	}
	if prepared.Summary != "" {
		t.Fatalf("summary on same day = %q, want empty", prepared.Summary)
	}
	if len(prepared.Items) != 0 {
		t.Fatalf("prepared items len on same day = %d, want 0", len(prepared.Items))
	}
}

func TestVoicePendingStore_ConfirmLeavesRemainder(t *testing.T) {
	workspace := t.TempDir()
	store := NewVoicePendingStore(workspace)
	now := time.Date(2026, 3, 18, 9, 0, 0, 0, time.UTC)
	store.now = func() time.Time { return now }

	for idx := 1; idx <= 4; idx++ {
		if err := store.Enqueue("owner-a", VoicePendingItem{Content: strings.Repeat("消息", idx)}); err != nil {
			t.Fatalf("Enqueue %d: %v", idx, err)
		}
	}

	prepared, err := store.PrepareFirstDailySummary("owner-a", 2, 8)
	if err != nil {
		t.Fatalf("PrepareFirstDailySummary: %v", err)
	}
	if len(prepared.Items) != 2 {
		t.Fatalf("prepared items len = %d, want 2", len(prepared.Items))
	}
	if !strings.Contains(prepared.Summary, "其余 2 条我先帮你记着") {
		t.Fatalf("summary = %q, want remaining hint", prepared.Summary)
	}

	itemIDs := []string{prepared.Items[0].ID, prepared.Items[1].ID}
	if err := store.ConfirmFirstDailySummary("owner-a", itemIDs); err != nil {
		t.Fatalf("ConfirmFirstDailySummary: %v", err)
	}

	queue, err := store.Load("owner-a")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(queue.Items) != 2 {
		t.Fatalf("remaining items len = %d, want 2", len(queue.Items))
	}
}

func TestDefaultOwnerVoicePendingWriter_UsesResolvedOwner(t *testing.T) {
	workspace := t.TempDir()
	writer := NewResolvedVoicePendingWriter(workspace, "fallback-owner", func(channel, chatID string) string {
		if channel == "telegram" && chatID == "chat-1" {
			return "Alice"
		}
		return ""
	})

	if err := writer.Enqueue("渠道推送", "", "完整结果已发送", "telegram", "chat-1"); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	store := NewVoicePendingStore(workspace)
	resolvedQueue, err := store.Load("alice")
	if err != nil {
		t.Fatalf("Load resolved queue: %v", err)
	}
	if len(resolvedQueue.Items) != 1 {
		t.Fatalf("resolved queue len = %d, want 1", len(resolvedQueue.Items))
	}

	fallbackQueue, err := store.Load("fallback-owner")
	if err != nil {
		t.Fatalf("Load fallback queue: %v", err)
	}
	if len(fallbackQueue.Items) != 0 {
		t.Fatalf("fallback queue len = %d, want 0", len(fallbackQueue.Items))
	}
}

func TestDefaultOwnerVoicePendingWriter_DoesNotFallbackForUnresolvedExternalPeer(t *testing.T) {
	workspace := t.TempDir()
	writer := NewResolvedVoicePendingWriter(workspace, "fallback-owner", func(channel, chatID string) string {
		return ""
	})

	if err := writer.Enqueue("渠道推送", "", "完整结果已发送", "telegram", "chat-1"); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	store := NewVoicePendingStore(workspace)
	queue, err := store.Load("fallback-owner")
	if err != nil {
		t.Fatalf("Load queue: %v", err)
	}
	if len(queue.Items) != 0 {
		t.Fatalf("fallback queue len = %d, want 0", len(queue.Items))
	}
}

func TestDefaultOwnerVoicePendingWriter_FallsBackToDefaultOwner(t *testing.T) {
	workspace := t.TempDir()
	writer := NewResolvedVoicePendingWriter(workspace, "fallback-owner", func(channel, chatID string) string {
		return ""
	})

	if err := writer.Enqueue("设备事件", "", "USB camera connected", "", ""); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	store := NewVoicePendingStore(workspace)
	queue, err := store.Load("fallback-owner")
	if err != nil {
		t.Fatalf("Load queue: %v", err)
	}
	if len(queue.Items) != 1 {
		t.Fatalf("queue len = %d, want 1", len(queue.Items))
	}
}
