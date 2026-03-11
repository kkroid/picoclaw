// PicoClaw - Ultra-lightweight personal AI agent
// License: MIT
//
// Copyright (c) 2026 PicoClaw contributors

// jarvis-voice: xiaozhi WebSocket gateway — Opus↔ASR→LLM→TTS→Opus pipeline.
// 通过 xiaozhi-esp32 协议与客户端通信，内部调用 ASR/LLM/TTS 三方服务。
//
// 配置通过环境变量注入（见 Config 结构体）:
// JARVIS_LISTEN      监听地址，默认 :8765
// JARVIS_LLM_SESSION 可选固定 session_id（多实例场景），默认空（每连接新建）
// PICOCLAW_CONFIG    picoclaw config.json 路径，默认 ~/.picoclaw/config.json
// PICOCLAW_HOME      picoclaw home 目录，默认 ~/.picoclaw
//
// 共享凭证（ASR 和 TTS 使用同一套火山引擎账号时，只需填这两项）：
// JARVIS_APPID  火山引擎 AppID（ASR/TTS 共用兜底）
// JARVIS_TOKEN  火山引擎 Access Token（ASR/TTS 共用兜底）
//
// ASR 单独覆盖（可选）：
// JARVIS_ASR_PROVIDER    ASR 提供商，默认 doubao
// JARVIS_ASR_APPID        覆盖 JARVIS_APPID
// JARVIS_ASR_TOKEN        覆盖 JARVIS_TOKEN
// JARVIS_ASR_CLUSTER      默认 bigmodel_transcribe
// JARVIS_ASR_RESOURCE_ID  默认 volc.bigasr.sauc.duration
//
// TTS 单独覆盖（可选）：
// JARVIS_TTS_PROVIDER TTS 提供商，默认 doubao
// JARVIS_TTS_APPID    覆盖 JARVIS_APPID
// JARVIS_TTS_TOKEN    覆盖 JARVIS_TOKEN
// JARVIS_TTS_CLUSTER  默认 volcano_tts
// JARVIS_TTS_VOICE    音色
package main

import (
	"bufio"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/caarlos0/env/v11"
	"github.com/gorilla/websocket"

	"github.com/sipeed/picoclaw/pkg/agent"
	"github.com/sipeed/picoclaw/pkg/asr"
	_ "github.com/sipeed/picoclaw/pkg/asr/doubao"
	"github.com/sipeed/picoclaw/pkg/bus"
	picoconfig "github.com/sipeed/picoclaw/pkg/config"
	"github.com/sipeed/picoclaw/pkg/providers"
	"github.com/sipeed/picoclaw/pkg/tts"
	_ "github.com/sipeed/picoclaw/pkg/tts/doubao"
)

type config struct {
	Listen     string `env:"JARVIS_LISTEN"       envDefault:":8765"`
	LLMSession string `env:"JARVIS_LLM_SESSION"`

	// 共享凭证：ASR 和 TTS 可复用同一套火山引擎账号
	AppID string `env:"JARVIS_APPID"`
	Token string `env:"JARVIS_TOKEN"`

	ASRProvider   string `env:"JARVIS_ASR_PROVIDER"     envDefault:"doubao"`
	ASRAppID      string `env:"JARVIS_ASR_APPID"` // 覆盖 JARVIS_APPID
	ASRToken      string `env:"JARVIS_ASR_TOKEN"` // 覆盖 JARVIS_TOKEN
	ASRCluster    string `env:"JARVIS_ASR_CLUSTER"      envDefault:"bigmodel_transcribe"`
	ASRResourceID string `env:"JARVIS_ASR_RESOURCE_ID"`

	TTSProvider string `env:"JARVIS_TTS_PROVIDER" envDefault:"doubao"`
	TTSAppID    string `env:"JARVIS_TTS_APPID"` // 覆盖 JARVIS_APPID
	TTSToken    string `env:"JARVIS_TTS_TOKEN"` // 覆盖 JARVIS_TOKEN
	TTSCluster  string `env:"JARVIS_TTS_CLUSTER"  envDefault:"volcano_tts"`
	TTSVoice    string `env:"JARVIS_TTS_VOICE"`
}

func main() {
	loadDotEnv(".env") // 自动加载当前目录的 .env，已有环境变量优先

	var cfg config
	if err := env.Parse(&cfg); err != nil {
		log.Fatalf("jarvis-voice: parse config: %v", err)
	}

	// 加载 picoclaw config 并初始化 AgentLoop (LLM)
	pcCfgPath := picoclawConfigPath()
	pcCfg, err := picoconfig.LoadConfig(pcCfgPath)
	if err != nil {
		log.Fatalf("jarvis-voice: load picoclaw config from %s: %v", pcCfgPath, err)
	}
	llmProvider, modelID, err := providers.CreateProvider(pcCfg)
	if err != nil {
		log.Fatalf("jarvis-voice: create LLM provider: %v", err)
	}
	if modelID != "" {
		pcCfg.Agents.Defaults.ModelName = modelID
	}
	msgBus := bus.NewMessageBus()
	agentLoop := agent.NewAgentLoop(pcCfg, msgBus, llmProvider)

	asrAppID := orStr(cfg.ASRAppID, cfg.AppID)
	asrToken := orStr(cfg.ASRToken, cfg.Token)
	asrResourceID := cfg.ASRResourceID
	if asrResourceID == "" {
		asrResourceID = "volc.bigasr.sauc.duration" // provider default
	}
	log.Printf("jarvis-voice: ASR config: provider=%s appid=%s cluster=%s resource_id=%s",
		cfg.ASRProvider, asrAppID, cfg.ASRCluster, asrResourceID)

	asrProvider, err := asr.New(cfg.ASRProvider, map[string]any{
		"appid":        asrAppID,
		"access_token": asrToken,
		"cluster":      cfg.ASRCluster,
		"resource_id":  cfg.ASRResourceID,
	})
	if err != nil {
		log.Fatalf("jarvis-voice: init ASR: %v", err)
	}

	ttsProvider, err := tts.New(cfg.TTSProvider, map[string]any{
		"appid":        orStr(cfg.TTSAppID, cfg.AppID),
		"access_token": orStr(cfg.TTSToken, cfg.Token),
		"cluster":      cfg.TTSCluster,
		"voice":        cfg.TTSVoice,
	})
	if err != nil {
		log.Fatalf("jarvis-voice: init TTS: %v", err)
	}

	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true },
	}

	reg := newDeviceRegistry()

	http.HandleFunc("/xiaozhi/v1/", func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Printf("jarvis-voice: ws upgrade: %v", err)
			return
		}
		s := newSession(conn, asrProvider, ttsProvider, agentLoop, cfg.LLMSession, reg)
		s.run()
	})

	log.Printf("jarvis-voice: listening on %s", cfg.Listen)
	if err := http.ListenAndServe(cfg.Listen, nil); err != nil {
		log.Fatalf("jarvis-voice: %v", err)
	}
}

// picoclawConfigPath 返回 picoclaw config.json 路径。
// 优先级: $PICOCLAW_CONFIG > $PICOCLAW_HOME/config.json > ~/.picoclaw/config.json
func picoclawConfigPath() string {
	if p := os.Getenv("PICOCLAW_CONFIG"); p != "" {
		return p
	}
	if h := os.Getenv("PICOCLAW_HOME"); h != "" {
		return filepath.Join(h, "config.json")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".picoclaw", "config.json")
}

// orStr 返回 a（若非空），否则返回 b。
func orStr(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// loadDotEnv 从指定路径加载 .env 文件，覆盖同名环境变量。
// 格式：KEY=VALUE，忽略空行和 # 注释行。
// 若需在 shell 级别覆盖，直接在命令行前置：JARVIS_APPID=xxx ./jarvis-voice
func loadDotEnv(path string) {
	f, err := os.Open(path)
	if err != nil {
		return // 文件不存在时静默跳过
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		idx := strings.IndexByte(line, '=')
		if idx < 0 {
			continue
		}
		key := strings.TrimSpace(line[:idx])
		val := strings.TrimSpace(line[idx+1:])
		if key == "" {
			continue
		}
		os.Setenv(key, val) // 始终覆盖，.env 文件是配置源
	}
}
