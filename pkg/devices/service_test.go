package devices

import (
	"context"
	"testing"
	"time"

	"github.com/sipeed/picoclaw/pkg/bus"
	"github.com/sipeed/picoclaw/pkg/devices/events"
	"github.com/sipeed/picoclaw/pkg/memory"
	"github.com/sipeed/picoclaw/pkg/state"
)

func TestServiceSendNotification_PublishesOutbound(t *testing.T) {
	stateMgr := state.NewManager(t.TempDir())
	if err := stateMgr.SetLastChannel("telegram:chat-1"); err != nil {
		t.Fatalf("SetLastChannel: %v", err)
	}

	svc := NewService(Config{Enabled: true}, stateMgr)
	msgBus := bus.NewMessageBus()
	svc.SetBus(msgBus)

	ev := &events.DeviceEvent{
		Action:  events.ActionAdd,
		Kind:    events.KindUSB,
		Vendor:  "Sipeed",
		Product: "Speaker",
	}
	svc.sendNotification(ev)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	select {
	case outbound := <-msgBus.OutboundChan():
		if outbound.Channel != "telegram" || outbound.ChatID != "chat-1" {
			t.Fatalf("outbound = %+v", outbound)
		}
		if outbound.Content == "" {
			t.Fatal("expected outbound content")
		}
	case <-ctx.Done():
		t.Fatal("expected outbound message")
	}
}

func TestServiceSendNotification_SkipsWithoutLastChannel(t *testing.T) {
	stateMgr := state.NewManager(t.TempDir())
	svc := NewService(Config{Enabled: true}, stateMgr)
	msgBus := bus.NewMessageBus()
	svc.SetBus(msgBus)

	ev := &events.DeviceEvent{
		Action:  events.ActionRemove,
		Kind:    events.KindUSB,
		Vendor:  "Sipeed",
		Product: "Camera",
	}
	svc.sendNotification(ev)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	select {
	case outbound := <-msgBus.OutboundChan():
		t.Fatalf("unexpected outbound message: %+v", outbound)
	case <-ctx.Done():
		// 符合预期：没有 last channel 时不应有消息
	}
}

func TestServiceSendNotification_QueuesVoicePendingToLinkedOwner(t *testing.T) {
	workspace := t.TempDir()
	stateMgr := state.NewManager(workspace)
	if err := stateMgr.SetLastChannel("telegram:chat-1"); err != nil {
		t.Fatalf("SetLastChannel: %v", err)
	}

	msgBus := bus.NewMessageBus()
	pw := memory.NewIdentityLinkedVoicePendingWriter(workspace, "fallback-owner", map[string][]string{
		"kkroid": {"telegram:chat-1"},
	})
	msgBus.OnOutbound(func(msg bus.OutboundMessage) {
		if pw != nil {
			_ = pw.Enqueue("", "", msg.Content, msg.Channel, msg.ChatID)
		}
	})

	svc := NewService(Config{Enabled: true}, stateMgr)
	svc.SetBus(msgBus)

	ev := &events.DeviceEvent{
		Action:  events.ActionAdd,
		Kind:    events.KindUSB,
		Vendor:  "Sipeed",
		Product: "Speaker",
	}
	svc.sendNotification(ev)

	queue, err := memory.NewVoicePendingStore(workspace).Load("kkroid")
	if err != nil {
		t.Fatalf("Load queue: %v", err)
	}
	if len(queue.Items) != 1 {
		t.Fatalf("queue items len = %d, want 1", len(queue.Items))
	}
	if queue.Items[0].Content == "" {
		t.Fatal("expected queued device notification content")
	}
}

func TestServiceSendNotification_QueuesVoicePendingEvenIfOutboundFails(t *testing.T) {
	workspace := t.TempDir()
	stateMgr := state.NewManager(workspace)
	if err := stateMgr.SetLastChannel("telegram:chat-1"); err != nil {
		t.Fatalf("SetLastChannel: %v", err)
	}

	msgBus := bus.NewMessageBus()
	pw := memory.NewIdentityLinkedVoicePendingWriter(workspace, "fallback-owner", map[string][]string{
		"kkroid": {"telegram:chat-1"},
	})
	msgBus.OnOutbound(func(msg bus.OutboundMessage) {
		if pw != nil {
			_ = pw.Enqueue("", "", msg.Content, msg.Channel, msg.ChatID)
		}
	})
	// 关闭 bus 后钩子仍应在 publish 前执行
	msgBus.Close()

	svc := NewService(Config{Enabled: true}, stateMgr)
	svc.SetBus(msgBus)

	ev := &events.DeviceEvent{
		Action:  events.ActionAdd,
		Kind:    events.KindUSB,
		Vendor:  "Sipeed",
		Product: "Speaker",
	}
	svc.sendNotification(ev)

	queue, err := memory.NewVoicePendingStore(workspace).Load("kkroid")
	if err != nil {
		t.Fatalf("Load queue: %v", err)
	}
	if len(queue.Items) != 1 {
		t.Fatalf("queue items len = %d, want 1", len(queue.Items))
	}
	if queue.Items[0].Content == "" {
		t.Fatal("expected queued device notification content")
	}
}
