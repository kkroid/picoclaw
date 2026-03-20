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
	"github.com/sipeed/picoclaw/pkg/tools"
)

type streamingMockProvider struct {
	response      string
	streamChunks  []string
	lastMessages  []providers.Message
	chatResponse  string
	streamInvoked int
}

func (m *streamingMockProvider) Chat(
	ctx context.Context,
	messages []providers.Message,
	tools []providers.ToolDefinition,
	model string,
	options map[string]any,
) (*providers.LLMResponse, error) {
	content := m.chatResponse
	if content == "" {
		content = m.response
	}
	return &providers.LLMResponse{Content: content}, nil
}

func (m *streamingMockProvider) ChatStream(
	ctx context.Context,
	messages []providers.Message,
	tools []providers.ToolDefinition,
	model string,
	options map[string]any,
	onChunk func(accumulated string),
) (*providers.LLMResponse, error) {
	m.streamInvoked++
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

func (m *streamingMockProvider) GetDefaultModel() string {
	return "stream-mock-model"
}

type streamingSequenceStep struct {
	response *providers.LLMResponse
	chunks   []string
}

type streamingSequenceProvider struct {
	steps        []streamingSequenceStep
	calls        int
	lastMessages [][]providers.Message
}

type nonStreamingMockProvider struct {
	response     string
	lastMessages []providers.Message
	chatCalls    int
}

func (m *nonStreamingMockProvider) Chat(
	ctx context.Context,
	messages []providers.Message,
	tools []providers.ToolDefinition,
	model string,
	options map[string]any,
) (*providers.LLMResponse, error) {
	m.chatCalls++
	m.lastMessages = append([]providers.Message(nil), messages...)
	return &providers.LLMResponse{Content: m.response}, nil
}

func (m *nonStreamingMockProvider) GetDefaultModel() string {
	return "non-stream-mock-model"
}

type noTokenStreamingProvider struct {
	response     string
	lastMessages []providers.Message
	streamCalls  int
}

func (m *noTokenStreamingProvider) Chat(
	ctx context.Context,
	messages []providers.Message,
	tools []providers.ToolDefinition,
	model string,
	options map[string]any,
) (*providers.LLMResponse, error) {
	return &providers.LLMResponse{Content: m.response}, nil
}

func (m *noTokenStreamingProvider) ChatStream(
	ctx context.Context,
	messages []providers.Message,
	tools []providers.ToolDefinition,
	model string,
	options map[string]any,
	onChunk func(accumulated string),
) (*providers.LLMResponse, error) {
	m.streamCalls++
	m.lastMessages = append([]providers.Message(nil), messages...)
	return &providers.LLMResponse{Content: m.response}, nil
}

func (m *noTokenStreamingProvider) GetDefaultModel() string {
	return "no-token-stream-model"
}

func (m *streamingSequenceProvider) Chat(
	ctx context.Context,
	messages []providers.Message,
	tools []providers.ToolDefinition,
	model string,
	options map[string]any,
) (*providers.LLMResponse, error) {
	return &providers.LLMResponse{}, nil
}

func (m *streamingSequenceProvider) ChatStream(
	ctx context.Context,
	messages []providers.Message,
	tools []providers.ToolDefinition,
	model string,
	options map[string]any,
	onChunk func(accumulated string),
) (*providers.LLMResponse, error) {
	step := m.steps[m.calls]
	m.calls++
	m.lastMessages = append(m.lastMessages, append([]providers.Message(nil), messages...))
	var accum string
	for _, chunk := range step.chunks {
		accum += chunk
		if onChunk != nil {
			onChunk(accum)
		}
	}
	return step.response, nil
}

func (m *streamingSequenceProvider) GetDefaultModel() string {
	return "stream-sequence-model"
}

type staticTool struct {
	name   string
	result *tools.ToolResult
}

func (t *staticTool) Name() string { return t.name }

func (t *staticTool) Description() string { return "static test tool" }

func (t *staticTool) Parameters() map[string]any {
	return map[string]any{"type": "object"}
}

func (t *staticTool) Execute(ctx context.Context, args map[string]any) *tools.ToolResult {
	return t.result
}

func newStreamingAgentLoop(t *testing.T) (*AgentLoop, *streamingMockProvider, func()) {
	t.Helper()
	tmpDir, err := os.MkdirTemp("", "agent-stream-test-*")
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

	provider := &streamingMockProvider{}
	al := NewAgentLoop(cfg, bus.NewMessageBus(), provider)
	return al, provider, func() { os.RemoveAll(tmpDir) }
}

func TestRunStreamAgentLoopWithKeys_SeparatesOwnerMemory(t *testing.T) {
	al, provider, cleanup := newStreamingAgentLoop(t)
	defer cleanup()

	provider.response = "owner aware reply"
	provider.streamChunks = []string{"owner ", "aware ", "reply"}

	defaultAgent := al.registry.GetDefaultAgent()
	if defaultAgent == nil {
		t.Fatal("No default agent found")
	}

	sessionKey := "xiaozhi:owner:kkroid:device:desk"
	memoryKey := "xiaozhi:owner:kkroid"
	defaultAgent.Sessions.SetSummary(sessionKey, "This device was asking about deployment details.")
	defaultAgent.Sessions.SetSummary(memoryKey, "Owner prefers concise technical answers.")
	defaultAgent.Sessions.AddMessage(memoryKey, "user", "previous owner question")
	defaultAgent.Sessions.AddMessage(memoryKey, "assistant", "previous owner answer")

	var streamed strings.Builder
	err := al.RunStreamAgentLoopWithKeys(
		context.Background(),
		StreamConversationKeys{SessionKey: sessionKey, MemoryKey: memoryKey},
		"hello from device",
		"xiaozhi",
		"turn-1",
		func(token string) { streamed.WriteString(token) },
	)
	if err != nil {
		t.Fatalf("RunStreamAgentLoopWithKeys: %v", err)
	}

	if streamed.String() != "owner aware reply" {
		t.Fatalf("streamed tokens = %q, want %q", streamed.String(), "owner aware reply")
	}
	if provider.streamInvoked != 1 {
		t.Fatalf("streamInvoked = %d, want 1", provider.streamInvoked)
	}
	if len(provider.lastMessages) == 0 {
		t.Fatal("lastMessages is empty")
	}
	if !strings.Contains(provider.lastMessages[0].Content, "VOICE_SESSION_SUMMARY") {
		t.Fatalf("system prompt missing session summary marker: %q", provider.lastMessages[0].Content)
	}
	if !strings.Contains(provider.lastMessages[0].Content, "OWNER_MEMORY_SUMMARY") {
		t.Fatalf("system prompt missing owner summary marker: %q", provider.lastMessages[0].Content)
	}
	if !strings.Contains(provider.lastMessages[0].Content, "Owner prefers concise technical answers.") {
		t.Fatalf("system prompt missing owner summary content: %q", provider.lastMessages[0].Content)
	}

	sessionHistory := defaultAgent.Sessions.GetHistory(sessionKey)
	if len(sessionHistory) != 2 {
		t.Fatalf("session history len = %d, want 2", len(sessionHistory))
	}
	if sessionHistory[0].Role != "user" || sessionHistory[0].Content != "hello from device" {
		t.Fatalf("unexpected session user message: %+v", sessionHistory[0])
	}
	if sessionHistory[1].Role != "assistant" || sessionHistory[1].Content != "owner aware reply" {
		t.Fatalf("unexpected session assistant message: %+v", sessionHistory[1])
	}

	ownerHistory := defaultAgent.Sessions.GetHistory(memoryKey)
	if len(ownerHistory) != 4 {
		t.Fatalf("owner history len = %d, want 4", len(ownerHistory))
	}
	if ownerHistory[2].Role != "user" || ownerHistory[2].Content != "hello from device" {
		t.Fatalf("unexpected owner user message: %+v", ownerHistory[2])
	}
	if ownerHistory[3].Role != "assistant" || ownerHistory[3].Content != "owner aware reply" {
		t.Fatalf("unexpected owner assistant message: %+v", ownerHistory[3])
	}
}

func TestRunStreamAgentLoopWithKeys_DeduplicatesSameKey(t *testing.T) {
	al, provider, cleanup := newStreamingAgentLoop(t)
	defer cleanup()

	provider.response = "same key reply"
	provider.streamChunks = []string{"same ", "key ", "reply"}

	defaultAgent := al.registry.GetDefaultAgent()
	if defaultAgent == nil {
		t.Fatal("No default agent found")
	}

	key := "xiaozhi:owner:kkroid"
	err := al.RunStreamAgentLoopWithKeys(
		context.Background(),
		StreamConversationKeys{SessionKey: key, MemoryKey: key},
		"hello",
		"xiaozhi",
		"turn-2",
		nil,
	)
	if err != nil {
		t.Fatalf("RunStreamAgentLoopWithKeys: %v", err)
	}

	history := defaultAgent.Sessions.GetHistory(key)
	if len(history) != 2 {
		t.Fatalf("history len = %d, want 2", len(history))
	}
	if history[0].Content != "hello" || history[1].Content != "same key reply" {
		t.Fatalf("unexpected history: %+v", history)
	}
}

func TestRunStreamAgentLoopWithKeys_StreamsFollowUpWithoutExternalChannelFallback(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "agent-stream-route-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

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

	provider := &streamingSequenceProvider{
		steps: []streamingSequenceStep{
			{
				response: &providers.LLMResponse{ToolCalls: []providers.ToolCall{{
					ID:   "call-1",
					Name: "web_fetch",
					Arguments: map[string]any{
						"url": "https://example.com",
					},
				}}},
			},
			{
				response: &providers.LLMResponse{Content: "这是站内摘要。"},
				chunks:   []string{"这是", "站内摘要。"},
			},
		},
	}

	msgBus := bus.NewMessageBus()
	al := NewAgentLoop(cfg, msgBus, provider)
	defaultAgent := al.registry.GetDefaultAgent()
	if defaultAgent == nil {
		t.Fatal("No default agent found")
	}
	defaultAgent.Tools.Register(&staticTool{
		name:   "web_fetch",
		result: tools.UserResult("完整长结果"),
	})
	if err := al.state.SetLastChannel("telegram:tg-chat"); err != nil {
		t.Fatalf("SetLastChannel: %v", err)
	}

	var streamed strings.Builder
	err = al.RunStreamAgentLoopWithKeys(
		context.Background(),
		StreamConversationKeys{
			SessionKey: "xiaozhi:owner:kkroid:device:desk",
			MemoryKey:  "xiaozhi:owner:kkroid",
		},
		"帮我总结这个链接",
		"xiaozhi",
		"turn-3",
		func(token string) { streamed.WriteString(token) },
	)
	if err != nil {
		t.Fatalf("RunStreamAgentLoopWithKeys: %v", err)
	}

	if streamed.String() != "这是站内摘要。" {
		t.Fatalf("streamed = %q, want %q", streamed.String(), "这是站内摘要。")
	}
	if provider.calls != 2 {
		t.Fatalf("provider calls = %d, want 2", provider.calls)
	}
	if len(provider.lastMessages) != 2 {
		t.Fatalf("lastMessages len = %d, want 2", len(provider.lastMessages))
	}
	lastCallMessages := provider.lastMessages[1]
	if len(lastCallMessages) == 0 {
		t.Fatal("second call messages should not be empty")
	}
	if lastCallMessages[len(lastCallMessages)-1].Role != "tool" {
		t.Fatalf("expected trailing tool message, got %+v", lastCallMessages[len(lastCallMessages)-1])
	}
	if lastCallMessages[len(lastCallMessages)-1].Content != "完整长结果" {
		t.Fatalf("tool message = %q, want 完整长结果", lastCallMessages[len(lastCallMessages)-1].Content)
	}

	outCtx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	select {
	case outbound := <-msgBus.OutboundChan():
		t.Fatalf("unexpected outbound message: %+v", outbound)
	case <-outCtx.Done():
		// expected: no outbound message
	}
}

func TestRunStreamAgentLoopWithKeys_FallsBackToNonStreamingProvider(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "agent-stream-fallback-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

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

	provider := &nonStreamingMockProvider{response: "非流式回复也应该下发。"}
	al := NewAgentLoop(cfg, bus.NewMessageBus(), provider)

	var streamed strings.Builder
	err = al.RunStreamAgentLoopWithKeys(
		context.Background(),
		StreamConversationKeys{SessionKey: "xiaozhi:owner:kkroid:device:desk", MemoryKey: "xiaozhi:owner:kkroid"},
		"你好",
		"xiaozhi",
		"turn-fallback-1",
		func(token string) { streamed.WriteString(token) },
	)
	if err != nil {
		t.Fatalf("RunStreamAgentLoopWithKeys: %v", err)
	}
	if provider.chatCalls != 1 {
		t.Fatalf("chatCalls = %d, want 1", provider.chatCalls)
	}
	if streamed.String() != "非流式回复也应该下发。" {
		t.Fatalf("streamed = %q, want %q", streamed.String(), "非流式回复也应该下发。")
	}

	defaultAgent := al.registry.GetDefaultAgent()
	if defaultAgent == nil {
		t.Fatal("No default agent found")
	}
	history := defaultAgent.Sessions.GetHistory("xiaozhi:owner:kkroid:device:desk")
	if len(history) != 2 {
		t.Fatalf("history len = %d, want 2", len(history))
	}
	if history[1].Content != "非流式回复也应该下发。" {
		t.Fatalf("assistant content = %q, want expected response", history[1].Content)
	}
}

func TestRunStreamAgentLoopWithKeys_UsesFinalContentWhenStreamDoesNotEmitTokens(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "agent-stream-no-token-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

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

	provider := &noTokenStreamingProvider{response: "没有 token 回调也要补发文本。"}
	al := NewAgentLoop(cfg, bus.NewMessageBus(), provider)

	var streamed strings.Builder
	err = al.RunStreamAgentLoopWithKeys(
		context.Background(),
		StreamConversationKeys{SessionKey: "xiaozhi:owner:kkroid:device:desk"},
		"你好",
		"xiaozhi",
		"turn-no-token-1",
		func(token string) { streamed.WriteString(token) },
	)
	if err != nil {
		t.Fatalf("RunStreamAgentLoopWithKeys: %v", err)
	}
	if provider.streamCalls != 1 {
		t.Fatalf("streamCalls = %d, want 1", provider.streamCalls)
	}
	if streamed.String() != "没有 token 回调也要补发文本。" {
		t.Fatalf("streamed = %q, want %q", streamed.String(), "没有 token 回调也要补发文本。")
	}
}
