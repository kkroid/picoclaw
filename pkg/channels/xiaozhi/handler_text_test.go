package xiaozhi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/sipeed/picoclaw/pkg/agent"
	"github.com/sipeed/picoclaw/pkg/asr"
	"github.com/sipeed/picoclaw/pkg/bus"
	"github.com/sipeed/picoclaw/pkg/config"
	"github.com/sipeed/picoclaw/pkg/providers"
	"github.com/sipeed/picoclaw/pkg/tts"
)

type textTurnStreamingProvider struct {
	response     string
	streamChunks []string
	lastMessages []providers.Message
}

type textTurnNonStreamingProvider struct {
	response     string
	lastMessages []providers.Message
}

func (m *textTurnNonStreamingProvider) Chat(
	ctx context.Context,
	messages []providers.Message,
	tools []providers.ToolDefinition,
	model string,
	options map[string]any,
) (*providers.LLMResponse, error) {
	m.lastMessages = append([]providers.Message(nil), messages...)
	return &providers.LLMResponse{Content: m.response}, nil
}

func (m *textTurnNonStreamingProvider) GetDefaultModel() string {
	return "xiaozhi-text-fallback-test"
}

func (m *textTurnStreamingProvider) Chat(
	ctx context.Context,
	messages []providers.Message,
	tools []providers.ToolDefinition,
	model string,
	options map[string]any,
) (*providers.LLMResponse, error) {
	return &providers.LLMResponse{Content: m.response}, nil
}

func (m *textTurnStreamingProvider) ChatStream(
	ctx context.Context,
	messages []providers.Message,
	tools []providers.ToolDefinition,
	model string,
	options map[string]any,
	onChunk func(accumulated string),
) (*providers.LLMResponse, error) {
	m.lastMessages = append([]providers.Message(nil), messages...)
	var accum string
	for _, chunk := range m.streamChunks {
		accum += chunk
		if onChunk != nil {
			onChunk(accum)
		}
	}
	return &providers.LLMResponse{Content: m.response}, nil
}

func (m *textTurnStreamingProvider) GetDefaultModel() string {
	return "xiaozhi-text-test"
}

type noopASRProvider struct{}

func (p *noopASRProvider) Name() string { return "noop-asr" }

func (p *noopASRProvider) AudioFormat() asr.AudioFormat {
	return asr.AudioFormat{Format: "pcm", Codec: "raw", SampleRate: 16000, Channels: 1}
}

func (p *noopASRProvider) Transcribe(ctx context.Context, frames [][]byte) (string, error) {
	return "", nil
}

type stubTTSProvider struct {
	texts []string
}

func (p *stubTTSProvider) Name() string { return "stub-tts" }

func (p *stubTTSProvider) AudioFormat() tts.AudioFormat {
	return tts.AudioFormat{Format: "ogg", Codec: "opus", SampleRate: 16000, Channels: 1}
}

func (p *stubTTSProvider) SynthesizeFrames(ctx context.Context, text, voice string, onFrame func([]byte)) error {
	p.texts = append(p.texts, text)
	if onFrame != nil {
		onFrame([]byte("frame:" + text))
	}
	return nil
}

type wsTextEnvelope struct {
	Type  string `json:"type"`
	Text  string `json:"text,omitempty"`
	State string `json:"state,omitempty"`
}

func newTestWebsocketPair(t *testing.T) (*websocket.Conn, *websocket.Conn, func()) {
	t.Helper()
	upgrader := websocket.Upgrader{}
	serverConnCh := make(chan *websocket.Conn, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade: %v", err)
			return
		}
		serverConnCh <- conn
	}))

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	clientConn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		server.Close()
		t.Fatalf("dial websocket: %v", err)
	}

	serverConn := <-serverConnCh
	cleanup := func() {
		clientConn.Close()
		serverConn.Close()
		server.Close()
	}
	return serverConn, clientConn, cleanup
}

func newTestAgentLoop(t *testing.T, workspace string, provider providers.LLMProvider) *agent.AgentLoop {
	t.Helper()
	cfg := &config.Config{
		Agents: config.AgentsConfig{
			Defaults: config.AgentDefaults{
				Workspace:         workspace,
				Model:             "test-model",
				MaxTokens:         4096,
				MaxToolIterations: 10,
			},
		},
	}
	return agent.NewAgentLoop(cfg, bus.NewMessageBus(), provider)
}

func readNextTextMessage(t *testing.T, conn *websocket.Conn) wsTextEnvelope {
	t.Helper()
	if err := conn.SetReadDeadline(time.Now().Add(3 * time.Second)); err != nil {
		t.Fatalf("SetReadDeadline: %v", err)
	}
	msgType, data, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("ReadMessage: %v", err)
	}
	if msgType != websocket.TextMessage {
		t.Fatalf("msgType = %d, want TextMessage", msgType)
	}
	var env wsTextEnvelope
	if err := json.Unmarshal(data, &env); err != nil {
		t.Fatalf("Unmarshal text message: %v, raw=%s", err, string(data))
	}
	return env
}

func collectConversationOutput(t *testing.T, conn *websocket.Conn) ([]wsTextEnvelope, int) {
	t.Helper()
	var messages []wsTextEnvelope
	binaryFrames := 0
	deadline := time.Now().Add(3 * time.Second)
	terminalSeenAt := time.Time{}
	for time.Now().Before(deadline) {
		if !terminalSeenAt.IsZero() && time.Since(terminalSeenAt) > 150*time.Millisecond {
			break
		}
		if err := conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond)); err != nil {
			t.Fatalf("SetReadDeadline: %v", err)
		}
		msgType, data, err := conn.ReadMessage()
		if err != nil {
			if ne, ok := err.(interface{ Timeout() bool }); ok && ne.Timeout() {
				continue
			}
			t.Fatalf("ReadMessage: %v", err)
		}
		if msgType == websocket.BinaryMessage {
			binaryFrames++
			continue
		}
		var env wsTextEnvelope
		if err := json.Unmarshal(data, &env); err != nil {
			t.Fatalf("Unmarshal: %v, raw=%s", err, string(data))
		}
		messages = append(messages, env)
		if (env.Type == "llm" && env.State == "stop") ||
			(env.Type == "tts" && (env.State == "stop" || env.State == "abort")) {
			if terminalSeenAt.IsZero() {
				terminalSeenAt = time.Now()
			}
		}
	}
	return messages, binaryFrames
}

func TestSessionHandleText_StartsTextConversationPipeline(t *testing.T) {
	workspace, err := os.MkdirTemp("", "xiaozhi-text-turn-*")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	defer os.RemoveAll(workspace)

	provider := &textTurnStreamingProvider{
		response:     "你好。",
		streamChunks: []string{"你好。"},
	}
	ttsProvider := &stubTTSProvider{}
	al := newTestAgentLoop(t, workspace, provider)
	serverConn, clientConn, cleanup := newTestWebsocketPair(t)
	defer cleanup()

	sess := newSession(
		serverConn,
		&noopASRProvider{},
		ttsProvider,
		al,
		newOwnerStore(workspace),
		newVoiceDeviceStore(workspace),
		nil,
		"",
		"per-owner-device",
		newDeviceRegistry(),
	)

	sess.handleText([]byte(`{"type":"hello","device_id":"desk-1","owner_id":"kkroid"}`))
	hello := readNextTextMessage(t, clientConn)
	if hello.Type != "hello" {
		t.Fatalf("hello type = %q, want hello", hello.Type)
	}

	sess.handleText([]byte(`{"type":"text","text":"你好","session_id":"turn-text-1"}`))
	textMessages, binaryFrames := collectConversationOutput(t, clientConn)

	var (
		gotLLMText    string
		gotLLMStop    bool
		gotTTSMessage bool
	)
	for _, msg := range textMessages {
		switch msg.Type {
		case "llm":
			if msg.State == "stop" {
				gotLLMStop = true
				continue
			}
			gotLLMText += msg.Text
		case "tts":
			gotTTSMessage = true
		}
	}

	if gotLLMText != "你好。" {
		t.Fatalf("gotLLMText = %q, want %q", gotLLMText, "你好。")
	}
	if !gotLLMStop {
		t.Fatal("expected llm stop message")
	}
	if gotTTSMessage {
		t.Fatal("text turn should not emit tts messages")
	}
	if binaryFrames != 0 {
		t.Fatalf("binaryFrames = %d, want 0", binaryFrames)
	}
	if len(ttsProvider.texts) != 0 {
		t.Fatalf("tts texts = %#v, want empty", ttsProvider.texts)
	}

	defaultAgent := al.GetRegistry().GetDefaultAgent()
	if defaultAgent == nil {
		t.Fatal("default agent is nil")
	}
	history := defaultAgent.Sessions.GetHistory("xiaozhi:owner:kkroid:device:desk-1")
	if len(history) != 2 {
		t.Fatalf("history len = %d, want 2", len(history))
	}
	if history[0].Role != "user" || history[0].Content != "你好" {
		t.Fatalf("unexpected user history: %+v", history[0])
	}
	if history[1].Role != "assistant" || history[1].Content != "你好。" {
		t.Fatalf("unexpected assistant history: %+v", history[1])
	}

	ownerHistory := defaultAgent.Sessions.GetHistory("xiaozhi:owner:kkroid")
	if len(ownerHistory) != 2 {
		t.Fatalf("owner history len = %d, want 2", len(ownerHistory))
	}
}

func TestSessionHandleText_UsesExplicitMemoryID(t *testing.T) {
	workspace, err := os.MkdirTemp("", "xiaozhi-text-memory-*")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	defer os.RemoveAll(workspace)

	provider := &textTurnStreamingProvider{
		response:     "已记住。",
		streamChunks: []string{"已记住。"},
	}
	al := newTestAgentLoop(t, workspace, provider)
	serverConn, clientConn, cleanup := newTestWebsocketPair(t)
	defer cleanup()

	sess := newSession(
		serverConn,
		&noopASRProvider{},
		&stubTTSProvider{},
		al,
		newOwnerStore(workspace),
		newVoiceDeviceStore(workspace),
		nil,
		"kkroid",
		"per-owner-device",
		newDeviceRegistry(),
	)

	sess.handleText([]byte(`{"type":"hello","device_id":"desk-2"}`))
	_ = readNextTextMessage(t, clientConn)
	sess.handleText([]byte(`{"type":"text","text":"帮我记住这个偏好","memory_id":"shared-text-memory"}`))
	_, _ = collectConversationOutput(t, clientConn)

	defaultAgent := al.GetRegistry().GetDefaultAgent()
	if defaultAgent == nil {
		t.Fatal("default agent is nil")
	}
	history := defaultAgent.Sessions.GetHistory("shared-text-memory")
	if len(history) != 2 {
		t.Fatalf("shared memory history len = %d, want 2", len(history))
	}
	if history[0].Content != "帮我记住这个偏好" || history[1].Content != "已记住。" {
		t.Fatalf("unexpected shared memory history: %+v", history)
	}
	ownerHistory := defaultAgent.Sessions.GetHistory("xiaozhi:owner:kkroid")
	if len(ownerHistory) != 2 {
		t.Fatalf("owner history len = %d, want 2", len(ownerHistory))
	}
}
