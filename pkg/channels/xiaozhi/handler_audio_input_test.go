package xiaozhi

import (
	"context"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/sipeed/picoclaw/pkg/asr"
	"github.com/sipeed/picoclaw/pkg/providers"
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

type stubStreamingSession struct {
	closed bool
}

func (s *stubStreamingSession) SendAudio(_ []byte, _ bool) error { return nil }

func (s *stubStreamingSession) Wait(ctx context.Context) (string, error) {
	<-ctx.Done()
	return "", ctx.Err()
}

func (s *stubStreamingSession) Close() {
	s.closed = true
}

type realtimeTestASRProvider struct{}

func (p *realtimeTestASRProvider) Name() string { return "realtime-test-asr" }

func (p *realtimeTestASRProvider) AudioFormat() asr.AudioFormat {
	return asr.AudioFormat{Format: "pcm", Codec: "raw", SampleRate: 16000, Channels: 1}
}

func (p *realtimeTestASRProvider) Transcribe(_ context.Context, _ [][]byte) (string, error) {
	return "", nil
}

func (p *realtimeTestASRProvider) OpenSession(_ context.Context, callback asr.ResultCallback) (asr.StreamingSession, error) {
	return &realtimeTestStreamingSession{callback: callback, finalText: "实时识别结果"}, nil
}

type realtimeTestStreamingSession struct {
	callback  asr.ResultCallback
	finalText string
	finalCh   chan string
	closed    bool
}

func (s *realtimeTestStreamingSession) SendAudio(_ []byte, isLast bool) error {
	if s.closed {
		return asr.ErrSessionClosed
	}
	if s.callback != nil && !isLast {
		s.callback("实时识别中", false)
	}
	if isLast {
		if s.callback != nil {
			s.callback(s.finalText, true)
		}
		if s.finalCh == nil {
			s.finalCh = make(chan string, 1)
		}
		s.finalCh <- s.finalText
	}
	return nil
}

func (s *realtimeTestStreamingSession) Wait(ctx context.Context) (string, error) {
	if s.finalCh == nil {
		s.finalCh = make(chan string, 1)
	}
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case text := <-s.finalCh:
		return text, nil
	}
}

func (s *realtimeTestStreamingSession) Close() {
	s.closed = true
}

type batchStreamingTestASRProvider struct{}

func (p *batchStreamingTestASRProvider) Name() string { return "batch-streaming-test-asr" }

func (p *batchStreamingTestASRProvider) AudioFormat() asr.AudioFormat {
	return asr.AudioFormat{Format: "pcm", Codec: "raw", SampleRate: 16000, Channels: 1}
}

func (p *batchStreamingTestASRProvider) Transcribe(_ context.Context, _ [][]byte) (string, error) {
	return "", nil
}

func (p *batchStreamingTestASRProvider) TranscribeStream(
	_ context.Context,
	_ [][]byte,
	callback asr.ResultCallback,
) error {
	if callback != nil {
		callback("批量识别中", false)
		callback("批量识别结果", true)
	}
	return nil
}

type audioTurnStreamingProvider struct {
	response string
}

func (p *audioTurnStreamingProvider) Chat(
	_ context.Context,
	_ []providers.Message,
	_ []providers.ToolDefinition,
	_ string,
	_ map[string]any,
) (*providers.LLMResponse, error) {
	return &providers.LLMResponse{Content: p.response}, nil
}

func (p *audioTurnStreamingProvider) ChatStream(
	_ context.Context,
	_ []providers.Message,
	_ []providers.ToolDefinition,
	_ string,
	_ map[string]any,
	onChunk func(string),
) (*providers.LLMResponse, error) {
	if onChunk != nil {
		onChunk(p.response)
	}
	return &providers.LLMResponse{Content: p.response}, nil
}

func (p *audioTurnStreamingProvider) GetDefaultModel() string { return "xiaozhi-audio-test" }

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
		sessionStores{},
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
		sessionStores{},
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

func TestSessionHandleAbort_CancelsCurrentTurnAndClosesRealtimeASR(t *testing.T) {
	serverConn, clientConn, cleanup := newTestWebsocketPair(t)
	defer cleanup()

	realtimeSess := &stubStreamingSession{}
	cancelled := false
	sess := newSession(
		serverConn,
		&noopASRProvider{},
		&stubTestTTSProvider{},
		nil,
		sessionStores{},
		"",
		"per-owner-device",
		newDeviceRegistry(),
	)
	sess.asrSess = realtimeSess
	sess.asrFeedCh = make(chan asrChunk, 1)
	sess.cancel = func() { cancelled = true }

	sess.handleText([]byte(`{"type":"abort","reason":"user"}`))

	msg := readNextTextMessage(t, clientConn)
	if msg.Type != "tts" || msg.State != "abort" {
		t.Fatalf("abort message = %#v, want type=tts state=abort", msg)
	}
	if !cancelled {
		t.Fatal("expected current turn cancel func to be called")
	}
	if !realtimeSess.closed {
		t.Fatal("expected realtime ASR session to be closed")
	}
	if sess.asrSess != nil {
		t.Fatal("expected session asrSess to be cleared after abort")
	}
	if sess.asrFeedCh != nil {
		t.Fatal("expected session asrFeedCh to be cleared after abort")
	}
	if sess.cancel != nil {
		t.Fatal("expected session cancel func to be cleared after abort")
	}
}

func TestSessionHandleVoice_UsesRealtimeASRAndRunsConversationPipeline(t *testing.T) {
	workspace := t.TempDir()
	provider := &audioTurnStreamingProvider{response: "收到实时语音"}
	al := newTestAgentLoop(t, workspace, provider)
	serverConn, clientConn, cleanup := newTestWebsocketPair(t)
	defer cleanup()

	sess := newSession(
		serverConn,
		&realtimeTestASRProvider{},
		&stubTTSProvider{},
		NewAgentLoopRuntime(al),
		sessionStores{owner: newOwnerStore(workspace), device: newVoiceDeviceStore(workspace)},
		"kkroid",
		"per-owner-device",
		newDeviceRegistry(),
	)

	sess.handleText([]byte(`{"type":"hello","device_id":"desk-rt-1","owner_id":"kkroid"}`))
	hello := readNextTextMessage(t, clientConn)
	if hello.Type != "hello" {
		t.Fatalf("hello type = %q, want hello", hello.Type)
	}

	sess.handleText([]byte(`{"type":"listen","state":"start","session_id":"turn-rt-1"}`))
	sess.handleAudio([]byte{1, 2, 3, 4})
	sess.handleText([]byte(`{"type":"listen","state":"end","session_id":"turn-rt-1"}`))

	messages, binaryFrames := collectConversationOutput(t, clientConn)
	var gotRecognizing bool
	var gotSttStop bool
	var gotLLMText string
	var gotTTSStart bool
	for _, msg := range messages {
		switch msg.Type {
		case "stt":
			if msg.State == "recognizing" && msg.Text == "实时识别中" {
				gotRecognizing = true
			}
			if msg.State == "stop" && msg.Text == "实时识别结果" {
				gotSttStop = true
			}
		case "llm":
			if msg.State == "" {
				gotLLMText += msg.Text
			}
		case "tts":
			if msg.State == "start" {
				gotTTSStart = true
			}
		}
	}
	if !gotRecognizing || !gotSttStop {
		t.Fatalf("expected realtime stt recognizing/stop, got recognizing=%v stop=%v messages=%#v", gotRecognizing, gotSttStop, messages)
	}
	if gotLLMText != "收到实时语音" {
		t.Fatalf("gotLLMText = %q, want %q", gotLLMText, "收到实时语音")
	}
	if !gotTTSStart {
		t.Fatal("expected realtime ASR result to drive downstream TTS")
	}
	if binaryFrames == 0 {
		t.Fatal("expected downstream TTS audio frames after realtime ASR")
	}
}

func TestSessionHandleVoice_FallsBackToBatchStreamingASRAndRunsConversationPipeline(t *testing.T) {
	workspace := t.TempDir()
	provider := &audioTurnStreamingProvider{response: "收到批量语音"}
	al := newTestAgentLoop(t, workspace, provider)
	serverConn, clientConn, cleanup := newTestWebsocketPair(t)
	defer cleanup()

	sess := newSession(
		serverConn,
		&batchStreamingTestASRProvider{},
		&stubTTSProvider{},
		NewAgentLoopRuntime(al),
		sessionStores{owner: newOwnerStore(workspace), device: newVoiceDeviceStore(workspace)},
		"kkroid",
		"per-owner-device",
		newDeviceRegistry(),
	)

	sess.handleText([]byte(`{"type":"hello","device_id":"desk-batch-1","owner_id":"kkroid"}`))
	hello := readNextTextMessage(t, clientConn)
	if hello.Type != "hello" {
		t.Fatalf("hello type = %q, want hello", hello.Type)
	}

	sess.handleText([]byte(`{"type":"listen","state":"start","session_id":"turn-batch-1"}`))
	sess.handleAudio([]byte{1, 2, 3, 4})
	sess.handleText([]byte(`{"type":"listen","state":"end","session_id":"turn-batch-1"}`))

	messages, binaryFrames := collectConversationOutput(t, clientConn)
	var gotRecognizing bool
	var gotSttStop bool
	var gotLLMText string
	var gotTTSStart bool
	for _, msg := range messages {
		switch msg.Type {
		case "stt":
			if msg.State == "recognizing" && msg.Text == "批量识别中" {
				gotRecognizing = true
			}
			if msg.State == "stop" && msg.Text == "批量识别结果" {
				gotSttStop = true
			}
		case "llm":
			if msg.State == "" {
				gotLLMText += msg.Text
			}
		case "tts":
			if msg.State == "start" {
				gotTTSStart = true
			}
		}
	}
	if !gotRecognizing || !gotSttStop {
		t.Fatalf("expected batch streaming stt recognizing/stop, got recognizing=%v stop=%v messages=%#v", gotRecognizing, gotSttStop, messages)
	}
	if gotLLMText != "收到批量语音" {
		t.Fatalf("gotLLMText = %q, want %q", gotLLMText, "收到批量语音")
	}
	if !gotTTSStart {
		t.Fatal("expected batch ASR result to drive downstream TTS")
	}
	if binaryFrames == 0 {
		t.Fatal("expected downstream TTS audio frames after batch streaming ASR")
	}
}
