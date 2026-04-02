package xiaozhi

import (
	"context"
	"errors"
	"testing"

	"github.com/sipeed/picoclaw/pkg/bus"
	"github.com/sipeed/picoclaw/pkg/channels"
	"github.com/sipeed/picoclaw/pkg/config"
)

func TestXiaozhiChannelSendRequiresRunning(t *testing.T) {
	ch := &XiaozhiChannel{
		BaseChannel: channels.NewBaseChannel("xiaozhi", config.XiaozhiConfig{}, bus.NewMessageBus(), nil),
	}

	msgIDs, err := ch.Send(context.Background(), bus.OutboundMessage{Content: "hello"})
	if !errors.Is(err, channels.ErrNotRunning) {
		t.Fatalf("expected ErrNotRunning, got %v", err)
	}
	if msgIDs != nil {
		t.Fatalf("expected nil message IDs, got %#v", msgIDs)
	}
}

func TestXiaozhiChannelSendRejectsBusOutbound(t *testing.T) {
	ch := &XiaozhiChannel{
		BaseChannel: channels.NewBaseChannel("xiaozhi", config.XiaozhiConfig{}, bus.NewMessageBus(), nil),
	}
	ch.SetRunning(true)

	msgIDs, err := ch.Send(context.Background(), bus.OutboundMessage{Content: "hello"})
	if !errors.Is(err, channels.ErrSendFailed) {
		t.Fatalf("expected ErrSendFailed, got %v", err)
	}
	if msgIDs != nil {
		t.Fatalf("expected nil message IDs, got %#v", msgIDs)
	}
}

func TestXiaozhiChannelStartRequiresRuntimeAndWorkspace(t *testing.T) {
	ch := &XiaozhiChannel{
		BaseChannel: channels.NewBaseChannel("xiaozhi", config.XiaozhiConfig{}, bus.NewMessageBus(), nil),
	}

	if err := ch.Start(context.Background()); err == nil {
		t.Fatal("expected Start to fail when runtime/workspace are not configured")
	}

	ch.SetRuntime(&agentLoopRuntime{})
	if err := ch.Start(context.Background()); err == nil {
		t.Fatal("expected Start to fail when workspace stores are not configured")
	}

	workspace := t.TempDir()
	ch.SetWorkspace(workspace)
	if err := ch.Start(context.Background()); err != nil {
		t.Fatalf("expected Start to succeed after runtime/workspace injection, got %v", err)
	}
	if !ch.IsRunning() {
		t.Fatal("expected channel to be running after successful Start")
	}
}
