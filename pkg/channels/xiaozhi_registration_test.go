package channels_test

import (
	"testing"

	"github.com/sipeed/picoclaw/pkg/bus"
	"github.com/sipeed/picoclaw/pkg/channels"
	_ "github.com/sipeed/picoclaw/pkg/channels/xiaozhi"
	"github.com/sipeed/picoclaw/pkg/config"
)

func TestXiaozhiFactoryRegistered(t *testing.T) {
	t.Helper()

	cfg := config.DefaultConfig()
	cfg.Channels.Xiaozhi.Enabled = true
	cfg.Channels.Xiaozhi.Token = "dummy-token"
	cfg.Channels.Xiaozhi.DefaultOwnerID = "test-owner"

	mgr, err := channels.NewManager(cfg, bus.NewMessageBus(), nil)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	if _, ok := mgr.GetChannel("xiaozhi"); !ok {
		t.Fatal("xiaozhi channel not initialized")
	}
}
