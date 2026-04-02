package xiaozhi

import (
	"testing"
	"time"
)

func TestResolveVoiceOwnerID_PrefersBoundOwner(t *testing.T) {
	if got := resolveVoiceOwnerID("Alice", "fallback-owner"); got != "alice" {
		t.Fatalf("resolveVoiceOwnerID = %q, want alice", got)
	}
}

func TestResolveVoiceOwnerID_FallsBackToDefaultOwner(t *testing.T) {
	if got := resolveVoiceOwnerID("", "fallback-owner"); got != "fallback-owner" {
		t.Fatalf("resolveVoiceOwnerID = %q, want fallback-owner", got)
	}
}

func TestBuildVoiceSessionContext_PerOwnerDevice(t *testing.T) {
	ctx := buildVoiceSessionContext("KKRoid", "per-owner-device", "Mac Book", "conn-1", "turn-1", "")
	if ctx.OwnerID != "kkroid" {
		t.Fatalf("OwnerID = %q, want %q", ctx.OwnerID, "kkroid")
	}
	if ctx.SessionKey != "xiaozhi:owner:kkroid:device:mac-book" {
		t.Fatalf("SessionKey = %q", ctx.SessionKey)
	}
	if ctx.OwnerMemoryKey != "xiaozhi:owner:kkroid" {
		t.Fatalf("OwnerMemoryKey = %q", ctx.OwnerMemoryKey)
	}
}

func TestBuildVoiceSessionContext_PerOwner(t *testing.T) {
	ctx := buildVoiceSessionContext("kkroid", "per-owner", "speaker-1", "conn-1", "turn-1", "")
	if ctx.SessionKey != "xiaozhi:owner:kkroid" {
		t.Fatalf("SessionKey = %q", ctx.SessionKey)
	}
}

func TestBuildVoiceSessionContext_ExplicitMemoryOverride(t *testing.T) {
	ctx := buildVoiceSessionContext("kkroid", "per-owner-device", "speaker-1", "conn-1", "turn-1", "custom-session")
	if ctx.SessionKey != "custom-session" {
		t.Fatalf("SessionKey = %q, want custom-session", ctx.SessionKey)
	}
	if ctx.OwnerMemoryKey != "xiaozhi:owner:kkroid" {
		t.Fatalf("OwnerMemoryKey = %q", ctx.OwnerMemoryKey)
	}
}

func TestBuildVoiceSessionContext_FallbackToDevice(t *testing.T) {
	ctx := buildVoiceSessionContext("", "per-owner-device", "speaker-1", "conn-1", "turn-1", "")
	if ctx.SessionKey != "xiaozhi:device:speaker-1" {
		t.Fatalf("SessionKey = %q", ctx.SessionKey)
	}
}

func TestBuildVoiceSessionContext_FallbackToConnection(t *testing.T) {
	ctx := buildVoiceSessionContext("", "per-owner-device", "", "conn-1", "turn-1", "")
	if ctx.SessionKey != "xiaozhi:conn:conn-1" {
		t.Fatalf("SessionKey = %q", ctx.SessionKey)
	}
}

func TestBuildVoiceSessionContext_NormalizesDeviceIDForSessionKey(t *testing.T) {
	ctx := buildVoiceSessionContext("kkroid", "per-owner-device", "AA:BB:CC:DD:EE:FF", "conn-1", "turn-1", "")
	if ctx.SessionKey != "xiaozhi:owner:kkroid:device:aa-bb-cc-dd-ee-ff" {
		t.Fatalf("SessionKey = %q", ctx.SessionKey)
	}
}

func TestPrepareTurn_CancelsPreviousTurnAndWaitsPipeline(t *testing.T) {
	s := &session{
		defaultOwnerID: "kkroid",
		sessionScope:   "per-owner-device",
		deviceID:       "desk-1",
		connID:         "conn-1",
	}

	cancelled := make(chan struct{})
	pipelineDone := make(chan struct{})
	s.cancel = func() {
		select {
		case <-cancelled:
		default:
			close(cancelled)
		}
	}
	s.pipelineWg.Add(1)
	go func() {
		defer s.pipelineWg.Done()
		<-cancelled
		close(pipelineDone)
	}()

	realtimeSess := &stubStreamingSession{}
	s.asrSess = realtimeSess
	s.asrFeedCh = make(chan asrChunk, 1)

	done := make(chan struct{})
	go func() {
		s.prepareTurn("turn-2", "", "voice")
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("prepareTurn did not finish after cancelling previous pipeline")
	}

	select {
	case <-pipelineDone:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("expected previous pipeline to finish before prepareTurn returned")
	}

	if !realtimeSess.closed {
		t.Fatal("expected previous realtime ASR session to be closed")
	}
	if s.cancel != nil {
		t.Fatal("expected cancel func to be cleared after prepareTurn")
	}
	if s.asrSess != nil || s.asrFeedCh != nil {
		t.Fatal("expected previous ASR session state to be cleared")
	}
	if s.turnID != "turn-2" {
		t.Fatalf("turnID = %q, want %q", s.turnID, "turn-2")
	}
}
