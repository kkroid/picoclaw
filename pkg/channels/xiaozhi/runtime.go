package xiaozhi

import (
	"context"
	"strings"

	"github.com/sipeed/picoclaw/pkg/agent"
	"github.com/sipeed/picoclaw/pkg/providers"
)

type Runtime interface {
	RunVoiceTurn(
		ctx context.Context,
		sessionKey, memoryKey, userText, channel, chatID string,
		onToken func(string),
	) (string, error)
	WorkspacePath() string
	GetHistory(sessionKey string) []providers.Message
	GetSummary(sessionKey string) string
}

type agentLoopRuntime struct {
	loop *agent.AgentLoop
}

func NewAgentLoopRuntime(loop *agent.AgentLoop) Runtime {
	if loop == nil {
		return nil
	}
	return &agentLoopRuntime{loop: loop}
}

func (r *agentLoopRuntime) defaultAgent() *agent.AgentInstance {
	if r == nil || r.loop == nil {
		return nil
	}
	return r.loop.GetRegistry().GetDefaultAgent()
}

func (r *agentLoopRuntime) RunVoiceTurn(
	ctx context.Context,
	sessionKey, memoryKey, userText, channel, chatID string,
	onToken func(string),
) (string, error) {
	if r == nil || r.loop == nil {
		return "", nil
	}
	return r.loop.RunVoiceAgentLoop(ctx, sessionKey, memoryKey, userText, channel, chatID, onToken)
}

func (r *agentLoopRuntime) WorkspacePath() string {
	agent := r.defaultAgent()
	if agent == nil {
		return ""
	}
	return strings.TrimSpace(agent.Workspace)
}

func (r *agentLoopRuntime) GetHistory(sessionKey string) []providers.Message {
	agent := r.defaultAgent()
	if agent == nil || agent.Sessions == nil {
		return nil
	}
	return agent.Sessions.GetHistory(sessionKey)
}

func (r *agentLoopRuntime) GetSummary(sessionKey string) string {
	agent := r.defaultAgent()
	if agent == nil || agent.Sessions == nil {
		return ""
	}
	return agent.Sessions.GetSummary(sessionKey)
}
