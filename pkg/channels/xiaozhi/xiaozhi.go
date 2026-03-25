// PicoClaw - Ultra-lightweight personal AI agent
// License: MIT
//
// Copyright (c) 2026 PicoClaw contributors

// xiaozhi 包实现 xiaozhi 语音 WebSocket 通道。
// 通过 ASR→LLM→TTS 三阶段流式流水线为 xiaozhi-esp32 等客户端提供实时语音对话。
package xiaozhi

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/gorilla/websocket"

	"github.com/sipeed/picoclaw/pkg/agent"
	"github.com/sipeed/picoclaw/pkg/asr"
	_ "github.com/sipeed/picoclaw/pkg/asr/doubao"
	_ "github.com/sipeed/picoclaw/pkg/asr/funasr"
	"github.com/sipeed/picoclaw/pkg/bus"
	"github.com/sipeed/picoclaw/pkg/channels"
	"github.com/sipeed/picoclaw/pkg/config"
	"github.com/sipeed/picoclaw/pkg/logger"
	"github.com/sipeed/picoclaw/pkg/memory"
	"github.com/sipeed/picoclaw/pkg/tts"
	_ "github.com/sipeed/picoclaw/pkg/tts/doubao"
	_ "github.com/sipeed/picoclaw/pkg/tts/fishspeech"
)

// XiaozhiChannel 实现 xiaozhi 语音 WebSocket 通道。
// 与其他文本 channel 不同，xiaozhi 直接调用 AgentLoop 进行流式推理，
// 不走 MessageBus 消息分发。
type XiaozhiChannel struct {
	*channels.BaseChannel
	config       config.XiaozhiConfig
	asr          asr.Provider
	tts          tts.Provider
	agentLoop    *agent.AgentLoop
	ownerStore   *ownerStore
	deviceStore  *voiceDeviceStore
	pendingStore *memory.VoicePendingStore
	registry     *deviceRegistry
	upgrader     websocket.Upgrader
}

// NewXiaozhiChannel 根据配置创建 xiaozhi 通道，初始化 ASR 和 TTS provider。
func NewXiaozhiChannel(cfg config.XiaozhiConfig, b *bus.MessageBus) (*XiaozhiChannel, error) {
	asrProvider := orStr(cfg.ASRProvider, "doubao")
	ttsProvider := orStr(cfg.TTSProvider, "doubao")

	logger.Infof("xiaozhi: ASR config: provider=%s ws_url=%s mode=%s", asrProvider, cfg.ASRWsURL, cfg.ASRMode)
	logger.Infof("xiaozhi: TTS config: provider=%s api_url=%s seed=%d", ttsProvider, cfg.TTSAPIURL, cfg.TTSSeed)

	asrProv, err := asr.New(asrProvider, map[string]any{
		"app_key":     orStr(cfg.ASRAppID, cfg.AppID),
		"access_key":  orStr(cfg.ASRToken, cfg.Token),
		"resource_id": orStr(cfg.ASRResourceID, "volc.bigasr.sauc.duration"),
		"ws_url":      cfg.ASRWsURL,
		"mode":        cfg.ASRMode,
	})
	if err != nil {
		return nil, fmt.Errorf("xiaozhi: init ASR provider %q: %w", asrProvider, err)
	}

	ttsProv, err := tts.New(ttsProvider, map[string]any{
		"api_url":      cfg.TTSAPIURL,
		"api_key":      cfg.TTSAPIKey,
		"reference_id": cfg.TTSReferenceID,
		"sample_rate":  cfg.TTSSampleRate,
		"seed":         cfg.TTSSeed,
		"appid":        orStr(cfg.TTSAppID, cfg.AppID),
		"access_token": orStr(cfg.TTSToken, cfg.Token),
		"cluster":      orStr(cfg.TTSCluster, "volcano_tts"),
		"voice":        cfg.TTSVoice,
	})
	if err != nil {
		return nil, fmt.Errorf("xiaozhi: init TTS provider %q: %w", ttsProvider, err)
	}

	return &XiaozhiChannel{
		BaseChannel: channels.NewBaseChannel("xiaozhi", cfg, b, nil),
		config:      cfg,
		asr:         asrProv,
		tts:         ttsProv,
		registry:    newDeviceRegistry(),
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true },
		},
	}, nil
}

// SetAgentLoop 注入 AgentLoop，在 channel 创建后、Start 之前调用。
// 参数类型为 any 以适配 Manager.InjectAgentLoop 的通用接口检测。
func (c *XiaozhiChannel) SetAgentLoop(v any) {
	al, _ := v.(*agent.AgentLoop)
	c.agentLoop = al
	if al == nil {
		c.ownerStore = nil
		c.deviceStore = nil
		c.pendingStore = nil
		return
	}
	defaultAgent := al.GetRegistry().GetDefaultAgent()
	if defaultAgent == nil {
		c.ownerStore = nil
		c.deviceStore = nil
		c.pendingStore = nil
		return
	}
	c.ownerStore = newOwnerStore(defaultAgent.Workspace)
	c.deviceStore = newVoiceDeviceStore(defaultAgent.Workspace)
	c.pendingStore = memory.NewVoicePendingStore(defaultAgent.Workspace)
}

// ---- Channel 接口实现 ----

func (c *XiaozhiChannel) Start(_ context.Context) error {
	if c.agentLoop == nil {
		return fmt.Errorf("xiaozhi: agentLoop not set, call SetAgentLoop before Start")
	}
	c.SetRunning(true)
	logger.InfoC("xiaozhi", "Xiaozhi voice channel started")
	return nil
}

func (c *XiaozhiChannel) Stop(_ context.Context) error {
	c.SetRunning(false)
	logger.InfoC("xiaozhi", "Xiaozhi voice channel stopped")
	return nil
}

func (c *XiaozhiChannel) Send(_ context.Context, _ bus.OutboundMessage) error {
	// xiaozhi 通道不接收 bus 出站消息，语音响应通过 TTS 直接推送到 WebSocket。
	return nil
}

// ---- WebhookHandler 接口实现 ----

func (c *XiaozhiChannel) WebhookPath() string { return "/xiaozhi/v1/" }

func (c *XiaozhiChannel) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/xiaozhi/v1")
	switch {
	case path == "/" || path == "":
		c.handleWebSocket(w, r)
	default:
		http.NotFound(w, r)
	}
}

func (c *XiaozhiChannel) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	if !c.IsRunning() {
		http.Error(w, "channel not running", http.StatusServiceUnavailable)
		return
	}
	conn, err := c.upgrader.Upgrade(w, r, nil)
	if err != nil {
		logger.Errorf("xiaozhi: ws upgrade: %v", err)
		return
	}
	s := newSession(
		conn,
		c.asr,
		c.tts,
		c.agentLoop,
		c.ownerStore,
		c.deviceStore,
		c.pendingStore,
		c.config.EffectiveDefaultOwnerID(),
		c.config.EffectiveSessionScope(),
		c.registry,
	)
	s.run()
}

func orStr(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
