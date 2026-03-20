package xiaozhi

import (
	"context"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/sipeed/picoclaw/pkg/asr"
	"github.com/sipeed/picoclaw/pkg/tts"
)

type unsupportedUpstreamASRProvider struct{}

func (p *unsupportedUpstreamASRProvider) Name() string { return "unsupported-asr" }

func (p *unsupportedUpstreamASRProvider) AudioFormat() asr.AudioFormat {
	return asr.AudioFormat{Format: "wav", Codec: "raw", SampleRate: 16000, Channels: 1}
}

func (p *unsupportedUpstreamASRProvider) Transcribe(_ context.Context, _ [][]byte) (string, error) {
	return "", nil
}

type stubTestTTSProvider struct{}

func (p *stubTestTTSProvider) Name() string { return "stub-tts" }

func (p *stubTestTTSProvider) AudioFormat() tts.AudioFormat {
	return tts.AudioFormat{Format: "ogg", Codec: "opus", SampleRate: 16000, Channels: 1}
}

func (p *stubTestTTSProvider) SynthesizeFrames(_ context.Context, _ string, _ string, _ func([]byte)) error {
	return nil
}

func TestValidateAudioFrame_RejectsOddLengthFrameWhenNegotiatedPCM(t *testing.T) {
	s := &session{
		audioFmt: asr.AudioFormat{Format: "pcm", Codec: "raw", SampleRate: 16000, Channels: 1},
	}

	if err := s.validateAudioFrame([]byte{1, 2, 3}); err == nil {
		t.Fatal("expected odd-length frame to be rejected under negotiated pcm")
	}
}

func TestValidateNegotiatedUpstreamAudioFormat_RejectsUnsupportedContainer(t *testing.T) {
	err := validateNegotiatedUpstreamAudioFormat(
		asr.AudioFormat{Format: "wav", Codec: "raw", SampleRate: 16000, Channels: 1},
	)
	if err == nil {
		t.Fatal("expected unsupported upstream format to be rejected")
	}
}

func TestValidateNegotiatedUpstreamAudioFormat_AcceptsOggOpus(t *testing.T) {
	err := validateNegotiatedUpstreamAudioFormat(
		asr.AudioFormat{Format: "ogg", Codec: "opus", SampleRate: 16000, Channels: 1},
	)
	if err != nil {
		t.Fatalf("expected ogg/opus upstream format to be accepted: %v", err)
	}
}

func TestValidateAudioFrame_AcceptsEvenLengthFrameWhenNegotiatedPCM(t *testing.T) {
	s := &session{
		audioFmt: asr.AudioFormat{Format: "pcm", Codec: "raw", SampleRate: 16000, Channels: 1},
	}

	if err := s.validateAudioFrame([]byte{1, 2, 3, 4}); err != nil {
		t.Fatalf("expected even-length frame to be accepted under negotiated pcm: %v", err)
	}
}

func TestSessionHandleHello_RejectsUnsupportedUpstreamFormat(t *testing.T) {
	serverConn, clientConn, cleanup := newTestWebsocketPair(t)
	defer cleanup()

	sess := newSession(
		serverConn,
		&unsupportedUpstreamASRProvider{},
		&stubTestTTSProvider{},
		nil,
		nil,
		nil,
		nil,
		"",
		"per-owner-device",
		newDeviceRegistry(),
	)
	sess.handleText([]byte(`{"type":"hello","device_id":"desk-1"}`))

	if err := clientConn.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("SetReadDeadline: %v", err)
	}
	_, _, err := clientConn.ReadMessage()
	closeErr, ok := err.(*websocket.CloseError)
	if !ok {
		t.Fatalf("expected websocket close error, got %v", err)
	}
	if closeErr.Code != websocket.CloseUnsupportedData {
		t.Fatalf("close code = %d, want %d", closeErr.Code, websocket.CloseUnsupportedData)
	}
}

func TestSessionHandleAudio_RejectsInvalidPCMFrame(t *testing.T) {
	serverConn, clientConn, cleanup := newTestWebsocketPair(t)
	defer cleanup()

	sess := newSession(
		serverConn,
		&noopASRProvider{},
		&stubTestTTSProvider{},
		nil,
		nil,
		nil,
		nil,
		"",
		"per-owner-device",
		newDeviceRegistry(),
	)
	sess.audioFmt = asr.AudioFormat{Format: "pcm", Codec: "raw", SampleRate: 16000, Channels: 1}
	sess.handleAudio([]byte{1, 2, 3})

	if err := clientConn.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("SetReadDeadline: %v", err)
	}
	_, _, err := clientConn.ReadMessage()
	closeErr, ok := err.(*websocket.CloseError)
	if !ok {
		t.Fatalf("expected websocket close error, got %v", err)
	}
	if closeErr.Code != websocket.CloseUnsupportedData {
		t.Fatalf("close code = %d, want %d", closeErr.Code, websocket.CloseUnsupportedData)
	}
}

func TestValidateAudioFrame_AcceptsOggOpusPayload(t *testing.T) {
	s := &session{
		audioFmt: asr.AudioFormat{Format: "ogg", Codec: "opus", SampleRate: 16000, Channels: 1},
	}

	if err := s.validateAudioFrame([]byte("OggS...")); err != nil {
		t.Fatalf("expected ogg/opus payload to pass through validation: %v", err)
	}
}
