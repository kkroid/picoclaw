package agent

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/sipeed/picoclaw/pkg/bus"
	"github.com/sipeed/picoclaw/pkg/config"
	"github.com/sipeed/picoclaw/pkg/providers"
	"github.com/sipeed/picoclaw/pkg/providers/streamctx"
	"github.com/sipeed/picoclaw/pkg/tools"
)

// streamAwareMockProvider 模拟 openai_compat 的行为：
// Chat() 检测 ctx 中的流式回调，有则走 ChatStream 路径。
type streamAwareMockProvider struct {
	response     string
	streamChunks []string
	lastMessages []providers.Message
	streamCalls  int
	chatCalls    int
}

// nonStreamingMockProvider 不实现 StreamingProvider，仅走 Chat()。
type nonStreamingMockProvider struct {
	response     string
	lastMessages []providers.Message
	chatCalls    int
}

func (m *nonStreamingMockProvider) Chat(
	_ context.Context,
	messages []providers.Message,
	_ []providers.ToolDefinition,
	_ string,
	_ map[string]any,
) (*providers.LLMResponse, error) {
	m.chatCalls++
	m.lastMessages = append([]providers.Message(nil), messages...)
	return &providers.LLMResponse{Content: m.response}, nil
}

func (m *nonStreamingMockProvider) GetDefaultModel() string {
	return "non-stream-mock-model"
}

type streamingSequenceStep struct {
	response *providers.LLMResponse
	chunks   []string
}

type staticTool struct {
	name   string
	result *tools.ToolResult
}

func (t *staticTool) Name() string        { return t.name }
func (t *staticTool) Description() string { return "static test tool" }
func (t *staticTool) Parameters() map[string]any {
	return map[string]any{"type": "object"}
}
func (t *staticTool) Execute(_ context.Context, _ map[string]any) *tools.ToolResult {
	return t.result
}

func (m *streamAwareMockProvider) Chat(
	ctx context.Context,
	messages []providers.Message,
	toolDefs []providers.ToolDefinition,
	model string,
	options map[string]any,
) (*providers.LLMResponse, error) {
	// 模拟 provider.go 的 [KKROID FORK] 逻辑
	if onChunk, ok := streamctx.GetCallback(ctx); ok {
		return m.ChatStream(ctx, messages, toolDefs, model, options, onChunk)
	}
	m.chatCalls++
	m.lastMessages = append([]providers.Message(nil), messages...)
	return &providers.LLMResponse{Content: m.response}, nil
}

func (m *streamAwareMockProvider) ChatStream(
	_ context.Context,
	messages []providers.Message,
	_ []providers.ToolDefinition,
	_ string,
	_ map[string]any,
	onChunk func(string),
) (*providers.LLMResponse, error) {
	m.streamCalls++
	m.lastMessages = append([]providers.Message(nil), messages...)
	for _, chunk := range m.streamChunks {
		if onChunk != nil {
			onChunk(chunk)
		}
	}
	return &providers.LLMResponse{Content: m.response}, nil
}

func (m *streamAwareMockProvider) GetDefaultModel() string {
	return "stream-aware-mock"
}

// streamAwareSequenceProvider 支持多轮工具调用的流式 mock。
type streamAwareSequenceProvider struct {
	steps        []streamingSequenceStep
	calls        int
	lastMessages [][]providers.Message
}

func (m *streamAwareSequenceProvider) Chat(
	ctx context.Context,
	messages []providers.Message,
	toolDefs []providers.ToolDefinition,
	model string,
	options map[string]any,
) (*providers.LLMResponse, error) {
	if onChunk, ok := streamctx.GetCallback(ctx); ok {
		return m.chatStream(ctx, messages, toolDefs, model, options, onChunk)
	}
	step := m.steps[m.calls]
	m.calls++
	m.lastMessages = append(m.lastMessages, append([]providers.Message(nil), messages...))
	return step.response, nil
}

func (m *streamAwareSequenceProvider) chatStream(
	_ context.Context,
	messages []providers.Message,
	_ []providers.ToolDefinition,
	_ string,
	_ map[string]any,
	onChunk func(string),
) (*providers.LLMResponse, error) {
	step := m.steps[m.calls]
	m.calls++
	m.lastMessages = append(m.lastMessages, append([]providers.Message(nil), messages...))
	for _, chunk := range step.chunks {
		if onChunk != nil {
			onChunk(chunk)
		}
	}
	return step.response, nil
}

func (m *streamAwareSequenceProvider) GetDefaultModel() string {
	return "stream-aware-sequence"
}

func newVoiceAgentLoop(t *testing.T, provider providers.LLMProvider) (*AgentLoop, func()) {
	t.Helper()
	tmpDir, err := os.MkdirTemp("", "agent-voice-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	cfg := &config.Config{
		Agents: config.AgentsConfig{
			Defaults: config.AgentDefaults{
				Workspace:         tmpDir,
				Model:             "test-model",
				MaxTokens:         4096,
				MaxToolIterations: 10,
			},
		},
	}

	al := NewAgentLoop(cfg, bus.NewMessageBus(), provider)
	return al, func() { os.RemoveAll(tmpDir) }
}

func TestRunVoiceAgentLoop_StreamTokens(t *testing.T) {
	provider := &streamAwareMockProvider{
		response:     "hello world",
		streamChunks: []string{"hello ", "world"},
	}
	al, cleanup := newVoiceAgentLoop(t, provider)
	defer cleanup()

	var streamed strings.Builder
	content, err := al.RunVoiceAgentLoop(
		context.Background(),
		"xiaozhi:device:desk", "",
		"你好", "xiaozhi", "turn-1",
		func(token string) { streamed.WriteString(token) },
	)
	if err != nil {
		t.Fatalf("RunVoiceAgentLoop: %v", err)
	}
	if content != "hello world" {
		t.Fatalf("content = %q, want %q", content, "hello world")
	}
	if streamed.String() != "hello world" {
		t.Fatalf("streamed = %q, want %q", streamed.String(), "hello world")
	}
	if provider.streamCalls != 1 {
		t.Fatalf("streamCalls = %d, want 1", provider.streamCalls)
	}
}

func TestRunVoiceAgentLoop_MirrorsToMemoryKey(t *testing.T) {
	provider := &streamAwareMockProvider{
		response:     "owner aware reply",
		streamChunks: []string{"owner ", "aware ", "reply"},
	}
	al, cleanup := newVoiceAgentLoop(t, provider)
	defer cleanup()

	agent := al.registry.GetDefaultAgent()
	sessionKey := "xiaozhi:owner:kkroid:device:desk"
	memoryKey := "xiaozhi:owner:kkroid"

	// 提前在 memoryKey 中写入历史
	agent.Sessions.AddMessage(memoryKey, "user", "previous question")
	agent.Sessions.AddMessage(memoryKey, "assistant", "previous answer")

	_, err := al.RunVoiceAgentLoop(
		context.Background(),
		sessionKey, memoryKey,
		"hello from device", "xiaozhi", "turn-1",
		nil,
	)
	if err != nil {
		t.Fatalf("RunVoiceAgentLoop: %v", err)
	}

	// 验证 session 历史
	sessionHistory := agent.Sessions.GetHistory(sessionKey)
	if len(sessionHistory) != 2 {
		t.Fatalf("session history len = %d, want 2", len(sessionHistory))
	}

	// 验证 owner memory 镜像
	ownerHistory := agent.Sessions.GetHistory(memoryKey)
	if len(ownerHistory) != 4 {
		t.Fatalf("owner history len = %d, want 4 (2 previous + 2 mirrored)", len(ownerHistory))
	}
	if ownerHistory[2].Role != "user" || ownerHistory[2].Content != "hello from device" {
		t.Fatalf("owner user message = %+v", ownerHistory[2])
	}
	if ownerHistory[3].Role != "assistant" || ownerHistory[3].Content != "owner aware reply" {
		t.Fatalf("owner assistant message = %+v", ownerHistory[3])
	}
}

func TestRunVoiceAgentLoop_DeduplicatesSameKey(t *testing.T) {
	provider := &streamAwareMockProvider{
		response:     "same key reply",
		streamChunks: []string{"same ", "key ", "reply"},
	}
	al, cleanup := newVoiceAgentLoop(t, provider)
	defer cleanup()

	agent := al.registry.GetDefaultAgent()
	key := "xiaozhi:owner:kkroid"

	// SessionKey == MemoryKey 时不应重复写入
	_, err := al.RunVoiceAgentLoop(
		context.Background(),
		key, key,
		"hello", "xiaozhi", "turn-2",
		nil,
	)
	if err != nil {
		t.Fatalf("RunVoiceAgentLoop: %v", err)
	}

	history := agent.Sessions.GetHistory(key)
	if len(history) != 2 {
		t.Fatalf("history len = %d, want 2", len(history))
	}
}

func TestRunVoiceAgentLoop_ToolCallFollowUp(t *testing.T) {
	provider := &streamAwareSequenceProvider{
		steps: []streamingSequenceStep{
			{
				response: &providers.LLMResponse{ToolCalls: []providers.ToolCall{{
					ID:        "call-1",
					Name:      "web_fetch",
					Arguments: map[string]any{"url": "https://example.com"},
				}}},
			},
			{
				response: &providers.LLMResponse{Content: "这是站内摘要。"},
				chunks:   []string{"这是", "站内摘要。"},
			},
		},
	}

	al, cleanup := newVoiceAgentLoop(t, provider)
	defer cleanup()

	agent := al.registry.GetDefaultAgent()
	agent.Tools.Register(&staticTool{
		name:   "web_fetch",
		result: tools.UserResult("完整长结果"),
	})

	var streamed strings.Builder
	content, err := al.RunVoiceAgentLoop(
		context.Background(),
		"xiaozhi:owner:kkroid:device:desk", "xiaozhi:owner:kkroid",
		"帮我总结这个链接", "xiaozhi", "turn-3",
		func(token string) { streamed.WriteString(token) },
	)
	if err != nil {
		t.Fatalf("RunVoiceAgentLoop: %v", err)
	}
	if content != "这是站内摘要。" {
		t.Fatalf("content = %q, want %q", content, "这是站内摘要。")
	}
	if streamed.String() != "这是站内摘要。" {
		t.Fatalf("streamed = %q, want %q", streamed.String(), "这是站内摘要。")
	}
	if provider.calls != 2 {
		t.Fatalf("provider calls = %d, want 2", provider.calls)
	}

	// 不应通过 bus 发送 outbound（语音路径 SendResponse=false）
	outCtx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	select {
	case outbound := <-al.bus.OutboundChan():
		t.Fatalf("unexpected outbound message: %+v", outbound)
	case <-outCtx.Done():
		// 预期：无 outbound
	}
}

func TestRunVoiceAgentLoop_NonStreamingProviderFallback(t *testing.T) {
	provider := &nonStreamingMockProvider{response: "非流式回复也应该下发。"}
	al, cleanup := newVoiceAgentLoop(t, provider)
	defer cleanup()

	// 非流式 provider 没有 ChatStream，onToken 通过主循环 runTurn 的 response.Content 生效
	// （不会被调用，因为 Chat() 里没有 streamctx 检测）
	content, err := al.RunVoiceAgentLoop(
		context.Background(),
		"xiaozhi:owner:kkroid:device:desk", "",
		"你好", "xiaozhi", "turn-fallback",
		nil,
	)
	if err != nil {
		t.Fatalf("RunVoiceAgentLoop: %v", err)
	}
	if content != "非流式回复也应该下发。" {
		t.Fatalf("content = %q, want expected response", content)
	}
}
