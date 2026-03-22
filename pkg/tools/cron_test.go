package tools

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sipeed/picoclaw/pkg/bus"
	"github.com/sipeed/picoclaw/pkg/config"
	"github.com/sipeed/picoclaw/pkg/cron"
	"github.com/sipeed/picoclaw/pkg/memory"
)

func newTestCronToolWithConfig(t *testing.T, cfg *config.Config) *CronTool {
	t.Helper()
	storePath := filepath.Join(t.TempDir(), "cron.json")
	cronService := cron.NewCronService(storePath, nil)
	msgBus := bus.NewMessageBus()
	tool, err := NewCronTool(cronService, nil, msgBus, t.TempDir(), true, 0, cfg)
	if err != nil {
		t.Fatalf("NewCronTool() error: %v", err)
	}
	return tool
}

func newTestCronTool(t *testing.T) *CronTool {
	t.Helper()
	return newTestCronToolWithConfig(t, config.DefaultConfig())
}

// TestCronTool_CommandBlockedFromRemoteChannel verifies command scheduling is restricted to internal channels
func TestCronTool_CommandBlockedFromRemoteChannel(t *testing.T) {
	tool := newTestCronTool(t)
	ctx := WithToolContext(context.Background(), "telegram", "chat-1")
	result := tool.Execute(ctx, map[string]any{
		"action":          "add",
		"message":         "check disk",
		"command":         "df -h",
		"command_confirm": true,
		"at_seconds":      float64(60),
	})

	if !result.IsError {
		t.Fatal("expected command scheduling to be blocked from remote channel")
	}
	if !strings.Contains(result.ForLLM, "restricted to internal channels") {
		t.Errorf("expected 'restricted to internal channels', got: %s", result.ForLLM)
	}
}

func TestCronTool_CommandDoesNotRequireConfirmByDefault(t *testing.T) {
	tool := newTestCronTool(t)
	ctx := WithToolContext(context.Background(), "cli", "direct")
	result := tool.Execute(ctx, map[string]any{
		"action":     "add",
		"message":    "check disk",
		"command":    "df -h",
		"at_seconds": float64(60),
	})

	if result.IsError {
		t.Fatalf("expected command scheduling without confirm to succeed by default, got: %s", result.ForLLM)
	}
	if !strings.Contains(result.ForLLM, "Cron job added") {
		t.Errorf("expected 'Cron job added', got: %s", result.ForLLM)
	}
}

func TestCronTool_CommandRequiresConfirmWhenAllowCommandDisabled(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Tools.Cron.AllowCommand = false

	tool := newTestCronToolWithConfig(t, cfg)
	ctx := WithToolContext(context.Background(), "cli", "direct")
	result := tool.Execute(ctx, map[string]any{
		"action":     "add",
		"message":    "check disk",
		"command":    "df -h",
		"at_seconds": float64(60),
	})

	if !result.IsError {
		t.Fatal("expected command scheduling to require confirm when allow_command is disabled")
	}
	if !strings.Contains(result.ForLLM, "command_confirm=true") {
		t.Errorf("expected command_confirm requirement message, got: %s", result.ForLLM)
	}
}

func TestCronTool_CommandAllowedWithConfirmWhenAllowCommandDisabled(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Tools.Cron.AllowCommand = false

	tool := newTestCronToolWithConfig(t, cfg)
	ctx := WithToolContext(context.Background(), "cli", "direct")
	result := tool.Execute(ctx, map[string]any{
		"action":          "add",
		"message":         "check disk",
		"command":         "df -h",
		"command_confirm": true,
		"at_seconds":      float64(60),
	})

	if result.IsError {
		t.Fatalf(
			"expected command scheduling with confirm to succeed when allow_command is disabled, got: %s",
			result.ForLLM,
		)
	}
	if !strings.Contains(result.ForLLM, "Cron job added") {
		t.Errorf("expected 'Cron job added', got: %s", result.ForLLM)
	}
}

func TestCronTool_CommandBlockedWhenExecDisabled(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Tools.Exec.Enabled = false

	tool := newTestCronToolWithConfig(t, cfg)
	ctx := WithToolContext(context.Background(), "cli", "direct")
	result := tool.Execute(ctx, map[string]any{
		"action":          "add",
		"message":         "check disk",
		"command":         "df -h",
		"command_confirm": true,
		"at_seconds":      float64(60),
	})

	if !result.IsError {
		t.Fatal("expected command scheduling to be blocked when exec is disabled")
	}
	if !strings.Contains(result.ForLLM, "command execution is disabled") {
		t.Errorf("expected exec disabled message, got: %s", result.ForLLM)
	}
}

// TestCronTool_CommandAllowedFromInternalChannel verifies command scheduling works from internal channels
func TestCronTool_CommandAllowedFromInternalChannel(t *testing.T) {
	tool := newTestCronTool(t)
	ctx := WithToolContext(context.Background(), "cli", "direct")
	result := tool.Execute(ctx, map[string]any{
		"action":          "add",
		"message":         "check disk",
		"command":         "df -h",
		"command_confirm": true,
		"at_seconds":      float64(60),
	})

	if result.IsError {
		t.Fatalf("expected command scheduling to succeed from internal channel, got: %s", result.ForLLM)
	}
	if !strings.Contains(result.ForLLM, "Cron job added") {
		t.Errorf("expected 'Cron job added', got: %s", result.ForLLM)
	}
}

// TestCronTool_AddJobRequiresSessionContext verifies fail-closed when channel/chatID missing
func TestCronTool_AddJobRequiresSessionContext(t *testing.T) {
	tool := newTestCronTool(t)
	result := tool.Execute(context.Background(), map[string]any{
		"action":     "add",
		"message":    "reminder",
		"at_seconds": float64(60),
	})

	if !result.IsError {
		t.Fatal("expected error when session context is missing")
	}
	if !strings.Contains(result.ForLLM, "no session context") {
		t.Errorf("expected 'no session context' message, got: %s", result.ForLLM)
	}
}

// TestCronTool_NonCommandJobAllowedFromRemoteChannel verifies regular reminders work from any channel
func TestCronTool_NonCommandJobAllowedFromRemoteChannel(t *testing.T) {
	tool := newTestCronTool(t)
	ctx := WithToolContext(context.Background(), "telegram", "chat-1")
	result := tool.Execute(ctx, map[string]any{
		"action":     "add",
		"message":    "time to stretch",
		"at_seconds": float64(600),
	})

	if result.IsError {
		t.Fatalf("expected non-command reminder to succeed from remote channel, got: %s", result.ForLLM)
	}
}

func TestCronTool_ExecuteJob_SkipsVoicePendingWithoutLinkedOwner(t *testing.T) {
	workspace := t.TempDir()
	storePath := filepath.Join(workspace, "cron.json")
	cronService := cron.NewCronService(storePath, nil)
	msgBus := bus.NewMessageBus()
	cfg := config.DefaultConfig()
	cfg.Channels.Xiaozhi.DefaultOwnerID = "fallback-owner"

	// 不注册 bus hook → 不会有 voice pending 入队

	tool, err := NewCronTool(cronService, nil, msgBus, workspace, true, 0, cfg)
	if err != nil {
		t.Fatalf("NewCronTool() error: %v", err)
	}

	job := &cron.CronJob{
		ID:      "job-1",
		Name:    "喝水提醒",
		Enabled: true,
		Payload: cron.CronPayload{
			Message: "现在该喝水了",
			Deliver: true,
			Channel: "telegram",
			To:      "chat-1",
		},
	}

	if got := tool.ExecuteJob(context.Background(), job); got != "ok" {
		t.Fatalf("ExecuteJob() = %q, want ok", got)
	}

	store := memory.NewVoicePendingStore(workspace)
	queue, err := store.Load("fallback-owner")
	if err != nil {
		t.Fatalf("Load queue: %v", err)
	}
	if len(queue.Items) != 0 {
		t.Fatalf("queue items len = %d, want 0", len(queue.Items))
	}
}

func TestCronTool_NonCommandJobDefaultsDeliverToFalse(t *testing.T) {
	tool := newTestCronTool(t)
	ctx := WithToolContext(context.Background(), "telegram", "chat-1")
	result := tool.Execute(ctx, map[string]any{
		"action":     "add",
		"message":    "send me a poem",
		"at_seconds": float64(600),
	})

	if result.IsError {
		t.Fatalf("expected non-command reminder to succeed, got: %s", result.ForLLM)
	}

	jobs := tool.cronService.ListJobs(false)
	if len(jobs) != 1 {
		t.Fatalf("expected 1 job, got %d", len(jobs))
	}
	if jobs[0].Payload.Deliver {
		t.Fatal("expected deliver=false by default for non-command jobs")
	}
}

func TestCronTool_ExecuteJobPublishesErrorWhenExecDisabled(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Tools.Exec.Enabled = false

	tool := newTestCronToolWithConfig(t, cfg)
	job := &cron.CronJob{}
	job.Payload.Channel = "cli"
	job.Payload.To = "direct"
	job.Payload.Command = "df -h"

	if got := tool.ExecuteJob(context.Background(), job); got != "ok" {
		t.Fatalf("ExecuteJob() = %q, want ok", got)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	var msg bus.OutboundMessage
	select {
	case msg = <-tool.msgBus.OutboundChan():
		// got message
	case <-ctx.Done():
		t.Fatal("timeout waiting for outbound message")
	}
	if !strings.Contains(msg.Content, "command execution is disabled") {
		t.Fatalf("expected exec disabled message, got: %s", msg.Content)
	}
}

func TestCronTool_ExecuteJob_QueuesVoicePendingToLinkedOwner(t *testing.T) {
	workspace := t.TempDir()
	storePath := filepath.Join(workspace, "cron.json")
	cronService := cron.NewCronService(storePath, nil)
	msgBus := bus.NewMessageBus()
	cfg := config.DefaultConfig()
	cfg.Channels.Xiaozhi.DefaultOwnerID = "fallback-owner"
	cfg.Session.IdentityLinks = map[string][]string{
		"kkroid": {"telegram:chat-1"},
	}

	// 通过 bus 出站钩子注册 pendingWriter（与 gateway 中的方式一致）
	pw := memory.NewIdentityLinkedVoicePendingWriter(workspace, "fallback-owner", cfg.Session.IdentityLinks)
	if pw != nil {
		msgBus.OnOutbound(func(msg bus.OutboundMessage) {
			_ = pw.Enqueue("", "", msg.Content, msg.Channel, msg.ChatID)
		})
	}

	tool, err := NewCronTool(cronService, nil, msgBus, workspace, true, 0, cfg)
	if err != nil {
		t.Fatalf("NewCronTool() error: %v", err)
	}

	job := &cron.CronJob{
		ID:      "job-2",
		Name:    "晨报",
		Enabled: true,
		Payload: cron.CronPayload{
			Message: "今天有三条重点更新",
			Deliver: true,
			Channel: "telegram",
			To:      "chat-1",
		},
	}

	if got := tool.ExecuteJob(context.Background(), job); got != "ok" {
		t.Fatalf("ExecuteJob() = %q, want ok", got)
	}

	store := memory.NewVoicePendingStore(workspace)
	queue, err := store.Load("kkroid")
	if err != nil {
		t.Fatalf("Load queue: %v", err)
	}
	if len(queue.Items) != 1 {
		t.Fatalf("queue items len = %d, want 1", len(queue.Items))
	}
	if queue.Items[0].Content != "今天有三条重点更新" {
		t.Fatalf("queue content = %q, want 今天有三条重点更新", queue.Items[0].Content)
	}

	fallbackQueue, err := store.Load("fallback-owner")
	if err != nil {
		t.Fatalf("Load fallback queue: %v", err)
	}
	if len(fallbackQueue.Items) != 0 {
		t.Fatalf("fallback queue items len = %d, want 0", len(fallbackQueue.Items))
	}
}
