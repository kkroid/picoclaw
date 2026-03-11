// PicoClaw - Ultra-lightweight personal AI agent
// License: MIT
//
// Copyright (c) 2026 PicoClaw contributors

// Package doubao implements TTS via Bytedance/Doubao HTTP API.
// 协议参考：https://openspeech.bytedance.com/api/v1/tts
// 返回 base64 编码的 16kHz 16-bit mono PCM（小端序 int16）。
package doubao

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/sipeed/picoclaw/pkg/tts"
)

const (
	defaultAPIURL  = "https://openspeech.bytedance.com/api/v1/tts"
	defaultCluster = "volcano_tts"
	defaultVoice   = "zh_female_wanwanxiaohe_moon_bigtts"
)

func init() {
	tts.Register("doubao", func(cfg map[string]any) (tts.Provider, error) {
		return newProvider(cfg)
	})
}

type provider struct {
	appid         string
	accessToken   string
	cluster       string
	defaultVoice  string
	apiURL        string
	authorization string // header prefix, default "Bearer;"
	client        *http.Client
}

func newProvider(cfg map[string]any) (*provider, error) {
	get := func(key string) string {
		v, _ := cfg[key].(string)
		return v
	}
	p := &provider{
		appid:         get("appid"),
		accessToken:   get("access_token"),
		cluster:       get("cluster"),
		defaultVoice:  get("voice"),
		apiURL:        get("api_url"),
		authorization: get("authorization"),
	}
	if p.accessToken == "" {
		return nil, fmt.Errorf("tts/doubao: access_token is required")
	}
	if p.cluster == "" {
		p.cluster = defaultCluster
	}
	if p.defaultVoice == "" {
		p.defaultVoice = defaultVoice
	}
	if p.apiURL == "" {
		p.apiURL = defaultAPIURL
	}
	if p.authorization == "" {
		p.authorization = "Bearer;"
	}
	p.client = &http.Client{Timeout: 30 * time.Second}
	return p, nil
}

func (p *provider) Name() string { return "doubao" }

// SynthesizeStream calls the HTTP TTS API and delivers the full PCM via onChunk once.
// 每次请求返回完整音频，无流式推送（HTTP API 不支持流式）。
func (p *provider) SynthesizeStream(ctx context.Context, text, voice string, onChunk func([]int16)) error {
	if voice == "" {
		voice = p.defaultVoice
	}

	reqBody := map[string]any{
		"app": map[string]any{
			"appid":   p.appid,
			"token":   p.accessToken,
			"cluster": p.cluster,
		},
		"user": map[string]any{
			"uid": "jarvis",
		},
		"audio": map[string]any{
			"voice_type": voice,
			"encoding":   "pcm",
			"rate":       16000,
			"bits":       16,
			"channel":    1,
		},
		"request": map[string]any{
			"reqid":         uuid.New().String(),
			"text":          text,
			"text_type":     "plain",
			"operation":     "query",
			"with_frontend": 1,
			"frontend_type": "unitTson",
		},
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("tts/doubao: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.apiURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("tts/doubao: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", p.authorization+p.accessToken)

	resp, err := p.client.Do(req)
	if err != nil {
		return fmt.Errorf("tts/doubao: http: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("tts/doubao: http %d: %s", resp.StatusCode, b)
	}

	var result struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    string `json:"data"` // base64 PCM
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("tts/doubao: decode response: %w", err)
	}
	if result.Code != 3000 {
		return fmt.Errorf("tts/doubao: api error %d: %s", result.Code, result.Message)
	}

	rawPCM, err := base64.StdEncoding.DecodeString(result.Data)
	if err != nil {
		return fmt.Errorf("tts/doubao: decode base64: %w", err)
	}

	// base64 解码后为小端序 int16 的原始字节
	if len(rawPCM)%2 != 0 {
		rawPCM = rawPCM[:len(rawPCM)-1]
	}
	samples := make([]int16, len(rawPCM)/2)
	for i := range samples {
		samples[i] = int16(binary.LittleEndian.Uint16(rawPCM[i*2:]))
	}

	onChunk(samples)
	return nil
}
