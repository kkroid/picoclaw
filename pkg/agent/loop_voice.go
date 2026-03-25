// [KKROID FORK] 语音流式 agent loop 入口。
// 将流式回调注入 context，复用主循环全部能力（压缩/重试/fallback/事件）。
package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/sipeed/picoclaw/pkg/logger"
	"github.com/sipeed/picoclaw/pkg/providers/streamctx"
)

// RunVoiceAgentLoop 是语音通道（如 xiaozhi）专用的流式入口。
// onToken 回调接收增量 delta 文本；内部通过 context 透传到 provider 层，
// 由 provider.Chat() 自动升级为 ChatStream()。
// memoryKey 不为空时，turn 结束后将 user/assistant 消息镜像写入 owner 级记忆。
func (al *AgentLoop) RunVoiceAgentLoop(
	ctx context.Context,
	sessionKey, memoryKey string,
	userText, channel, chatID string,
	onToken func(string),
) (string, error) {
	sessionKey = strings.TrimSpace(sessionKey)
	if sessionKey == "" {
		return "", fmt.Errorf("voice session key is required")
	}
	memoryKey = strings.TrimSpace(memoryKey)
	if memoryKey == sessionKey {
		memoryKey = ""
	}

	agent := al.registry.GetDefaultAgent()
	if agent == nil {
		return "", fmt.Errorf("no default agent available")
	}

	// 将流式回调注入 context，provider.Chat() 检测到后自动走 ChatStream
	if onToken != nil {
		ctx = streamctx.WithCallback(ctx, onToken)
	}

	opts := processOptions{
		SessionKey:    sessionKey,
		Channel:       channel,
		ChatID:        chatID,
		UserMessage:   userText,
		DefaultResponse: defaultResponse,
		EnableSummary: true,
		SendResponse:  false, // 语音路径自行管理输出
	}

	finalContent, err := al.runAgentLoop(ctx, agent, opts)
	if err != nil {
		return "", err
	}

	// 镜像到 owner 级长期记忆
	if memoryKey != "" && finalContent != "" {
		agent.Sessions.AddMessage(memoryKey, "user", userText)
		agent.Sessions.AddMessage(memoryKey, "assistant", finalContent)
		if saveErr := agent.Sessions.Save(memoryKey); saveErr != nil {
			logger.WarnCF("voice-stream", "Failed to save owner memory",
				map[string]any{"memory_key": memoryKey, "error": saveErr.Error()})
		}
		al.maybeSummarize(agent, memoryKey, al.newTurnEventScope(agent.ID, memoryKey))
	}

	return finalContent, nil
}
