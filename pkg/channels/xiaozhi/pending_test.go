package xiaozhi

import (
	"strings"
	"testing"

	"github.com/sipeed/picoclaw/pkg/memory"
)

func TestSessionPrepareAndConfirmOwnerPendingAnnouncement(t *testing.T) {
	workspace := t.TempDir()
	store := memory.NewVoicePendingStore(workspace)
	if err := store.Enqueue("KKRoid", memory.VoicePendingItem{Content: "下午三点提醒你开会"}); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	s := &session{
		ownerID: "kkroid",
		stores: sessionStores{
			pending: store,
		},
	}

	batch := s.prepareOwnerPendingAnnouncement()
	if !strings.Contains(batch.summary, "下午三点提醒你开会") {
		t.Fatalf("summary = %q, want pending content", batch.summary)
	}

	batchAgain := s.prepareOwnerPendingAnnouncement()
	if batchAgain.summary == "" {
		t.Fatal("expected pending summary to remain until confirmation")
	}

	if err := s.confirmOwnerPendingAnnouncement(batch); err != nil {
		t.Fatalf("confirmOwnerPendingAnnouncement: %v", err)
	}

	batchAfterConfirm := s.prepareOwnerPendingAnnouncement()
	if batchAfterConfirm.summary != "" {
		t.Fatalf("summary after confirm = %q, want empty", batchAfterConfirm.summary)
	}
}
