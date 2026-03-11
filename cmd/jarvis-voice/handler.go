// PicoClaw - Ultra-lightweight personal AI agent
// License: MIT
//
// Copyright (c) 2026 PicoClaw contributors

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"gopkg.in/hraban/opus.v2"

	"github.com/sipeed/picoclaw/pkg/agent"
	"github.com/sipeed/picoclaw/pkg/asr"
	"github.com/sipeed/picoclaw/pkg/tts"
)

// ---- xiaozhi 协议消息类型 ----

type helloMsg struct {
	Type        string       `json:"type"`
	Version     int          `json:"version"`
	Transport   string       `json:"transport"`
	DeviceID    string       `json:"device_id,omitempty"`
	MacAddr     string       `json:"mac_addr,omitempty"`
	AudioParams *audioParams `json:"audio_params,omitempty"`
}

type audioParams struct {
	Format        string `json:"format"`
	SampleRate    int    `json:"sample_rate"`
	Channels      int    `json:"channels"`
	FrameDuration int    `json:"frame_duration"`
}

type listenMsg struct {
	Type      string `json:"type"`
	State     string `json:"state"` // "start" | "end" | "detect"
	Mode      string `json:"mode,omitempty"`
	SessionID string `json:"session_id,omitempty"` // turn_id: 客户端在 VAD start 时生成，贯穿整轮
	MemoryID  string `json:"memory_id,omitempty"`  // memory_id: 客户端指定的 LLM 记忆 key，控制多轮对话上下文
}

type abortMsg struct {
	Type   string `json:"type"`
	Reason string `json:"reason,omitempty"`
}

// ---- 服务端发出的消息 ----

func jsonMsg(typ string, extra map[string]any) []byte {
	m := map[string]any{"type": typ}
	for k, v := range extra {
		m[k] = v
	}
	b, _ := json.Marshal(m)
	return b
}

func helloReply(sessID string) []byte {
	return jsonMsg("hello", map[string]any{
		"version":    3,
		"transport":  "websocket",
		"session_id": sessID,
		"audio_params": map[string]any{
			"format":         "opus",
			"sample_rate":    16000,
			"channels":       1,
			"frame_duration": 60,
		},
	})
}

// ---- session ----

const (
	opusSampleRate  = 16000
	opusChannels    = 1
	frameSamples    = 960 // 60ms @ 16kHz
	opusMaxFrameLen = 4000
)

// deviceRegistry 维护 device_id → 活跃 session 的映射。
// 同一 device_id 的新连接到来时，旧连接被立即关闭（last-write-wins）。
type deviceRegistry struct {
	mu     sync.Mutex
	active map[string]*session
}

func newDeviceRegistry() *deviceRegistry {
	return &deviceRegistry{active: make(map[string]*session)}
}

// register 注册新 session，并关闭同设备的旧连接。
func (r *deviceRegistry) register(deviceID string, s *session) {
	r.mu.Lock()
	old, exists := r.active[deviceID]
	r.active[deviceID] = s
	r.mu.Unlock()
	if exists && old != s {
		log.Printf("jarvis-voice: device %s reconnected, evicting old session", deviceID)
		old.connCancel()
		old.conn.Close()
	}
}

// unregister 在 session 退出时清理（仅当仍是活跃 session 时）。
func (r *deviceRegistry) unregister(deviceID string, s *session) {
	r.mu.Lock()
	if r.active[deviceID] == s {
		delete(r.active, deviceID)
	}
	r.mu.Unlock()
}

// sentenceTerminators: 断句硬结束符（中英文标点）
const sentenceTerminators = "。！？\n.!?"

// asrChunk 是实时 ASR 音频帧，isLast=true 表示结束帧。
type asrChunk struct {
	pcm    []int16
	isLast bool
}

// audioItem 是 TTS 合成完毕的一个句子及其 PCM 数据。
type audioItem struct {
	text string
	pcm  []int16
}

type session struct {
	conn      *websocket.Conn
	writeMu   sync.Mutex
	asr       asr.Provider
	tts       tts.Provider
	agentLoop *agent.AgentLoop
	registry  *deviceRegistry
	// connCancel 由 deviceRegistry.register 在设备重连时调用，强制关闭本连接。
	connCancel context.CancelFunc
	// llmSessOverride：通过 JARVIS_LLM_SESSION 强制指定的 LLM 记忆 session，优先级最高
	llmSessOverride string
	// deviceID：从 hello 消息提取的设备标识（MAC 或 device_id）
	deviceID string
	// connID：WebSocket 连接级 ID，服务端在 hello 时生成并回传客户端，客户端不关心。
	// 用于标识一条长连接，设备重连时更新。
	connID string
	// turnID：问题级 ID，由客户端在 VAD start（listen.start）时生成并下发。
	// 贯穿整个问题生命周期：VAD start → ASR → LLM → TTS stop。
	turnID string
	// memoryID：LLM 多轮记忆 key，由客户端在 listen.start 时指定。
	// 同一 memoryID 的问题共享对话历史；客户端可按场景/用户自由切换。
	// 未携带时退化为 deviceID，deviceID 也无时退化为 connID。
	memoryID string

	opusDec *opus.Decoder
	opusEnc *opus.Encoder

	audioBuf []int16

	// asrSess 和 asrFeedCh 仅在使用实时 ASR（RealtimeProvider）时非 nil。
	// listen start 时建连，handleAudio 推送帧，listen end 时发结束帧。
	asrSess   asr.StreamingSession
	asrFeedCh chan asrChunk

	// 当前语音交互的总取消函数
	cancelMu sync.Mutex
	cancel   context.CancelFunc
}

func newSession(conn *websocket.Conn, asrProv asr.Provider, ttsProv tts.Provider, al *agent.AgentLoop, llmSessOverride string, reg *deviceRegistry) *session {
	dec, err := opus.NewDecoder(opusSampleRate, opusChannels)
	if err != nil {
		log.Printf("jarvis-voice: opus decoder init: %v", err)
	}
	enc, err := opus.NewEncoder(opusSampleRate, opusChannels, opus.AppVoIP)
	if err != nil {
		log.Printf("jarvis-voice: opus encoder init: %v", err)
	}
	_, cancel := context.WithCancel(context.Background())
	return &session{
		conn:            conn,
		asr:             asrProv,
		tts:             ttsProv,
		agentLoop:       al,
		registry:        reg,
		connCancel:      cancel,
		llmSessOverride: llmSessOverride,
		opusDec:         dec,
		opusEnc:         enc,
	}
}

func (s *session) run() {
	defer s.connCancel()
	defer s.conn.Close()
	defer func() {
		if s.deviceID != "" {
			s.registry.unregister(s.deviceID, s)
		}
	}()
	for {
		msgType, data, err := s.conn.ReadMessage()
		if err != nil {
			log.Printf("jarvis-voice: session %s (%s) read error: %v", s.connID, s.deviceID, err)
			return
		}
		switch msgType {
		case websocket.TextMessage:
			s.handleText(data)
		case websocket.BinaryMessage:
			s.handleAudio(data)
		}
	}
}

func (s *session) handleText(data []byte) {
	var base struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(data, &base); err != nil {
		return
	}

	switch base.Type {
	case "hello":
		var msg helloMsg
		if err := json.Unmarshal(data, &msg); err == nil {
			if msg.DeviceID != "" {
				s.deviceID = msg.DeviceID
			} else if msg.MacAddr != "" {
				s.deviceID = msg.MacAddr
			}
		}
		// hello：服务端生成连接级 connID 并回传客户端（客户端不关心，仅用于日志关联）
		if s.connID == "" {
			if s.llmSessOverride != "" {
				s.connID = s.llmSessOverride
			} else {
				s.connID = uuid.New().String()
			}
			log.Printf("jarvis-voice: conn_id=%s device=%s", s.connID, s.deviceID)
		}
		// 注册设备：驱逐同设备的旧连接（重连场景）
		if s.deviceID != "" {
			s.registry.register(s.deviceID, s)
		}
		s.writeText(helloReply(s.connID))

	case "listen":
		var msg listenMsg
		if err := json.Unmarshal(data, &msg); err != nil {
			return
		}
		switch msg.State {
		case "start":
			s.audioBuf = s.audioBuf[:0]
			// turn_id
			if msg.SessionID != "" {
				s.turnID = msg.SessionID
				log.Printf("jarvis-voice: turn_id=%s", s.turnID)
			} else {
				// 客户端未携带（兼容旧版本）：服务端按 connID+时间戳兜底
				now := time.Now()
				ts := now.Format("20060102150405") + fmt.Sprintf("%03d", now.UnixMilli()%1000)
				s.turnID = s.connID + "_" + ts
				log.Printf("jarvis-voice: turn_id=%s (server-generated)", s.turnID)
			}
			// memory_id
			if msg.MemoryID != "" {
				s.memoryID = msg.MemoryID
			} else if s.deviceID != "" {
				s.memoryID = s.deviceID
			} else {
				s.memoryID = s.connID
			}
			// 取消旧流水线，关闭旧实时 ASR 会话
			s.cancelMu.Lock()
			if s.cancel != nil {
				s.cancel()
				s.cancel = nil
			}
			s.cancelMu.Unlock()
			if s.asrSess != nil {
				s.asrSess.Close()
				s.asrSess = nil
				s.asrFeedCh = nil
			}
			// 实时 ASR 模式：listen start 时就建连，音频帧实时推送
			if rp, ok := s.asr.(asr.RealtimeProvider); ok {
				pipeCtx, pipeCancel := context.WithCancel(context.Background())
				s.cancelMu.Lock()
				s.cancel = pipeCancel
				s.cancelMu.Unlock()
				asrCtx, asrCancel := context.WithTimeout(pipeCtx, 30*time.Second)
				sess, err := rp.OpenSession(asrCtx, func(text string, final bool) {
					state := "recognizing"
					if final {
						state = "stop"
					}
					s.writeText(jsonMsg("stt", map[string]any{"text": text, "state": state}))
				})
				if err != nil {
					asrCancel()
					pipeCancel()
					s.cancelMu.Lock()
					s.cancel = nil
					s.cancelMu.Unlock()
					log.Printf("jarvis-voice: open asr session: %v", err)
				} else {
					feedCh := make(chan asrChunk, 64)
					s.asrSess = sess
					s.asrFeedCh = feedCh
					turnID, memoryID := s.turnID, s.memoryID
					go func() {
						defer asrCancel()
						s.processStreamSpeech(pipeCtx, sess, feedCh, time.Now(), turnID, memoryID)
					}()
				}
			}
		case "end", "stop":
			if s.asrSess != nil {
				// 实时模式：发送结束帧，processStreamSpeech 等待最终 ASR 结果
				select {
				case s.asrFeedCh <- asrChunk{isLast: true}:
				default:
					log.Printf("jarvis-voice: asr feed channel full on finalize")
				}
				s.asrSess = nil
				s.asrFeedCh = nil
			} else {
				// 批量模式：VAD 结束后一次性 ASR
				s.cancelMu.Lock()
				if s.cancel != nil {
					s.cancel()
					s.cancel = nil
				}
				s.cancelMu.Unlock()
				buf := make([]int16, len(s.audioBuf))
				copy(buf, s.audioBuf)
				go s.processSpeech(buf)
			}
		}

	case "abort":
		// 关闭实时 ASR 会话（如果有）
		if s.asrSess != nil {
			s.asrSess.Close()
			s.asrSess = nil
			s.asrFeedCh = nil
		}
		s.cancelMu.Lock()
		if s.cancel != nil {
			s.cancel()
			s.cancel = nil
		}
		s.cancelMu.Unlock()
		// 通知客户端立即清空播放缓冲（触发 miniaudio stop+restart）
		s.writeText(jsonMsg("tts", map[string]any{"state": "abort"}))
	}
}

func (s *session) handleAudio(data []byte) {
	if s.opusDec == nil {
		return
	}
	pcm := make([]int16, frameSamples*4) // 预留 4 倍空间
	n, err := s.opusDec.Decode(data, pcm)
	if err != nil {
		return
	}
	pcm = pcm[:n]
	if s.asrFeedCh != nil {
		// 实时模式：直接推送给 ASR，不缓冲
		select {
		case s.asrFeedCh <- asrChunk{pcm: pcm}:
		default:
			log.Printf("jarvis-voice: asr feed channel full, dropping frame")
		}
	} else {
		// 批量模式：累积到 audioBuf，等 VAD end 后一次性 ASR
		s.audioBuf = append(s.audioBuf, pcm...)
	}
}

// processSpeech: ASR → LLM → TTS → Opus 三阶段并发流水线。
//
// 阶段 A (goroutine): LLM 流式推理，按断句符切句，写入 sentCh。
// 阶段 B (goroutine): 从 sentCh 读句子，调用 TTS 合成，写入 readyCh。
//
//	B 与 A 并发：LLM 生下一句时，TTS 已在合成当前句。
//
// 阶段 C (main):      从 readyCh 按序读 PCM，编码 Opus 推给客户端。
//
//	C 发音频时，B 已在合成下一句，消除句间空档。
func (s *session) processSpeech(pcm []int16) {
	reqStart := time.Now()
	log.Printf("jarvis-voice: processSpeech: pcm=%d samples (%.0fms)", len(pcm), float64(len(pcm))/16.0)

	ctx, cancel := context.WithCancel(context.Background())
	s.cancelMu.Lock()
	s.cancel = cancel
	s.cancelMu.Unlock()
	defer cancel()

	// 1. ASR（独立 20s 超时，不影响后续 LLM/TTS 管道）
	asrCtx, asrCancel := context.WithTimeout(ctx, 20*time.Second)
	defer asrCancel()

	var finalText string
	var asrErr error
	if sp, ok := s.asr.(asr.StreamingProvider); ok {
		// 流式 ASR：每次文本变化都发给客户端（recognizing），最终结果发 stop
		asrErr = sp.TranscribeStream(asrCtx, pcm, func(text string, final bool) {
			state := "recognizing"
			if final {
				state = "stop"
				finalText = text
			}
			s.writeText(jsonMsg("stt", map[string]any{"text": text, "state": state}))
		})
	} else {
		finalText, asrErr = s.asr.Transcribe(asrCtx, pcm)
		if asrErr == nil && strings.TrimSpace(finalText) != "" {
			s.writeText(jsonMsg("stt", map[string]any{"text": finalText, "state": "stop"}))
		}
	}

	if asrErr != nil {
		if ctx.Err() == nil {
			log.Printf("jarvis-voice: asr: %v", asrErr)
		}
		// 通知客户端复位 FSM（PROCESSING → IDLE）
		s.writeText(jsonMsg("tts", map[string]any{"state": "stop"}))
		return
	}
	if strings.TrimSpace(finalText) == "" {
		// 静默或未识别：通知客户端复位 FSM（PROCESSING → IDLE）
		s.writeText(jsonMsg("tts", map[string]any{"state": "stop"}))
		return
	}
	log.Printf("jarvis-voice: asr=%q latency=%dms", finalText, time.Since(reqStart).Milliseconds())

	turnID := s.turnID
	if turnID == "" {
		turnID = s.connID
	}
	memoryID := s.memoryID
	if memoryID == "" {
		memoryID = s.connID
	}
	s.runPipeline(ctx, finalText, reqStart, turnID, memoryID)
}

// runPipeline 执行 LLM → TTS → Opus 三阶段并发流水线。
// 由 processSpeech（批量 ASR）和 processStreamSpeech（实时 ASR）共同调用。
func (s *session) runPipeline(ctx context.Context, text string, reqStart time.Time, turnID, memoryID string) {
	sentCh := make(chan string, 8)     // 阶段 A → B
	readyCh := make(chan audioItem, 2) // 阶段 B → C（预合成最多 2 句）

	// 阶段 A: LLM 流式 → sentCh
	go func() {
		defer close(sentCh)
		if err := s.streamLLM(ctx, turnID, memoryID, text, reqStart, func(sentence string) {
			select {
			case sentCh <- sentence:
			case <-ctx.Done():
			}
		}); err != nil {
			// ctx.Err() != nil 表示主动 abort/重请求；err == Canceled 是 watchdog 取消（已自行记录）
			if ctx.Err() == nil && err != context.Canceled && err != context.DeadlineExceeded {
				log.Printf("jarvis-voice: llm: %v", err)
			}
		}
	}()

	// 阶段 B: sentCh → TTS 合成 → readyCh
	go func() {
		defer close(readyCh)
		for {
			select {
			case sentence, ok := <-sentCh:
				if !ok {
					return
				}
				var pcmBuf []int16
				if err := s.tts.SynthesizeStream(ctx, sentence, "", func(chunk []int16) {
					pcmBuf = append(pcmBuf, chunk...)
				}); err != nil {
					if ctx.Err() == nil {
						log.Printf("jarvis-voice: tts skip sentence %q: %v", sentence, err)
					}
					// TTS 单句失败时跳过该句继续，不中止整条流水线
					continue
				}
				select {
				case readyCh <- audioItem{text: sentence, pcm: pcmBuf}:
				case <-ctx.Done():
					return
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	// 阶段 C: readyCh → Opus 编码 → WebSocket（保序）
	firstItem := true
	for item := range readyCh {
		if firstItem {
			// tts.start 触发客户端 FSM: PROCESSING → SPEAKING
			s.writeText(jsonMsg("tts", map[string]any{"state": "start"}))
			firstItem = false
		}
		s.writeText(jsonMsg("tts", map[string]any{"state": "sentence_start", "text": item.text}))
		if err := s.sendOpus(item.pcm); err != nil {
			log.Printf("jarvis-voice: send opus: %v", err)
		}
		s.writeText(jsonMsg("tts", map[string]any{"state": "sentence_end"}))
	}

	s.writeText(jsonMsg("tts", map[string]any{"state": "stop"}))
}

// processStreamSpeech 处理实时 ASR 流：并发发送音频帧 + 等待最终识别结果，然后运行 LLM/TTS 流水线。
func (s *session) processStreamSpeech(ctx context.Context, sess asr.StreamingSession, feedCh <-chan asrChunk, reqStart time.Time, turnID, memoryID string) {
	defer sess.Close()

	// 并发发送音频帧给 ASR 服务
	sendDone := make(chan struct{})
	go func() {
		defer close(sendDone)
		for {
			select {
			case chunk, ok := <-feedCh:
				if !ok {
					return
				}
				if err := sess.SendAudio(chunk.pcm, chunk.isLast); err != nil {
					if ctx.Err() == nil {
						log.Printf("jarvis-voice: asr send: %v", err)
					}
					return
				}
				if chunk.isLast {
					return
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	// 等待 ASR 最终识别结果
	text, err := sess.Wait(ctx)
	<-sendDone

	if err != nil {
		if ctx.Err() == nil && !errors.Is(err, asr.ErrSessionClosed) {
			log.Printf("jarvis-voice: asr: %v", err)
			s.writeText(jsonMsg("tts", map[string]any{"state": "stop"}))
		}
		return
	}
	if strings.TrimSpace(text) == "" {
		s.writeText(jsonMsg("tts", map[string]any{"state": "stop"}))
		return
	}
	log.Printf("jarvis-voice: asr=%q latency=%dms", text, time.Since(reqStart).Milliseconds())

	s.runPipeline(ctx, text, reqStart, turnID, memoryID)
}

// streamLLM 直接调用 AgentLoop 流式推理，按句触发 onSentence 回调。
// 同时支持 thinking 模式（<think>...</think>）和普通模式：
//   - thinking 块内容不送 TTS，仅记录耗时日志
//   - thinking 块之外的内容按断句符触发 TTS
//
// reqStart 用于记录 LLM 首 token 延迟（含 ASR 时间，为端到端指标）。
func (s *session) streamLLM(ctx context.Context, turnID, memoryID, userText string, reqStart time.Time, onSentence func(string)) error {
	llmStart := time.Now()
	log.Printf("jarvis-voice: llm request: turn=%s memory=%s text=%q", turnID, memoryID, userText)
	const startTag = "<think>"
	const endTag = "</think>"

	// 首 token 超时 30s；之后每个 token 间隔超时 8s（每收到 token 自动重置）
	const firstTokenTimeout = 30 * time.Second
	const interTokenTimeout = 8 * time.Second
	llmCtx, llmCancel := context.WithCancel(ctx)
	defer llmCancel()
	tokenActivity := make(chan struct{}, 1)
	go func() {
		firstToken := true
		t := time.NewTimer(firstTokenTimeout)
		defer t.Stop()
		for {
			select {
			case <-llmCtx.Done():
				return
			case <-tokenActivity:
				if firstToken {
					firstToken = false
				}
				if !t.Stop() {
					select {
					case <-t.C:
					default:
					}
				}
				t.Reset(interTokenTimeout)
			case <-t.C:
				if firstToken {
					log.Printf("jarvis-voice: llm timeout: no first token after %s", firstTokenTimeout)
				} else {
					log.Printf("jarvis-voice: llm timeout: no token for %s", interTokenTimeout)
				}
				llmCancel()
				return
			}
		}
	}()

	var normalBuf strings.Builder
	// thinkTail 保存 thinking 块末尾最多 len(endTag)-1 个字节，
	// 用于跨 token 边界检测 </think> 而无需缓存完整 thinking 内容。
	thinkTail := make([]byte, 0, len(endTag)-1)
	inThinking := false
	var thinkStart time.Time
	firstContent := reqStart // 非零时尚未记录首个实际内容 token 延迟

	flushNormal := func() {
		text := normalBuf.String()
		if idx := lastSentenceBreak(text); idx > 0 {
			sentence := text[:idx]
			remaining := text[idx:]
			// 先更新缓冲，再发送（onSentence 可能阻塞）
			normalBuf.Reset()
			normalBuf.WriteString(remaining)
			if trimmed := strings.TrimSpace(sentence); trimmed != "" {
				log.Printf("jarvis-voice: llm sentence: %q", trimmed)
				s.writeText(jsonMsg("llm", map[string]any{"text": trimmed, "emotion": "neutral"}))
				onSentence(trimmed)
			}
		}
	}

	onToken := func(token string) {
		// token 到达，重置 watchdog 定时器
		select {
		case tokenActivity <- struct{}{}:
		default:
		}
		// token 可能跨多个 tag 边界，用循环处理完
		for token != "" {
			if inThinking {
				// 用滑动窗口检测 </think>，不存储完整 thinking 内容
				scan := string(thinkTail) + token
				if idx := strings.Index(scan, endTag); idx >= 0 {
					log.Printf("jarvis-voice: thinking=%dms", time.Since(thinkStart).Milliseconds())
					after := scan[idx+len(endTag):]
					thinkTail = thinkTail[:0]
					inThinking = false
					token = after // 继续处理 </think> 之后的内容
				} else {
					// 更新尾部窗口（保留最多 len(endTag)-1 字节）
					if len(scan) >= len(endTag)-1 {
						thinkTail = []byte(scan[len(scan)-(len(endTag)-1):])
					} else {
						thinkTail = []byte(scan)
					}
					token = ""
				}
			} else {
				normalBuf.WriteString(token)
				token = ""
				combined := normalBuf.String()
				if idx := strings.Index(combined, startTag); idx >= 0 {
					before := combined[:idx]
					after := combined[idx+len(startTag):]
					normalBuf.Reset()
					normalBuf.WriteString(before)
					// before 非空时记录首内容延迟（thinking 前已有实际输出）
					if !firstContent.IsZero() && before != "" {
						log.Printf("jarvis-voice: llm_first_token latency=%dms", time.Since(firstContent).Milliseconds())
						firstContent = time.Time{}
					}
					flushNormal()
					inThinking = true
					thinkStart = time.Now()
					thinkTail = thinkTail[:0]
					log.Printf("jarvis-voice: llm thinking...")
					token = after
				} else {
					// 普通内容：记录首 token 延迟并断句
					if !firstContent.IsZero() && strings.TrimSpace(normalBuf.String()) != "" {
						log.Printf("jarvis-voice: llm_first_token latency=%dms", time.Since(firstContent).Milliseconds())
						firstContent = time.Time{}
					}
					flushNormal()
				}
			}
		}
	}

	if err := s.agentLoop.RunStreamAgentLoop(llmCtx, memoryID, userText, "voice", turnID, onToken); err != nil {
		return err
	}

	// 处理最后剩余片段（thinking 块未正常关闭时跳过，避免把 thinking 内容送 TTS）
	if !inThinking && normalBuf.Len() > 0 {
		if trimmed := strings.TrimSpace(normalBuf.String()); trimmed != "" {
			log.Printf("jarvis-voice: llm sentence: %q", trimmed)
			s.writeText(jsonMsg("llm", map[string]any{"text": trimmed, "emotion": "neutral"}))
			onSentence(trimmed)
		}
	}

	log.Printf("jarvis-voice: llm done: turn=%s latency=%dms", turnID, time.Since(llmStart).Milliseconds())
	return nil
}

// lastSentenceBreak 返回字符串中最后一个句子边界之后的字节偏移，
// 即 sentenceTerminators rune 之后第一个字节的位置。
// 找不到时返回 0。
//
// 规则：
//   - 中文结束符（。！？）和换行符（\n）直接算句末。
//   - 英文 !? 后跟空格时才切（末尾不切，防止流式缓冲末尾误判）。
//   - 英文 . 仅当同时满足以下条件才切：
//     1. 前一字符不是数字（排除版本号/小数点 1.25.7）
//     2. 后一字符不是数字
//     3. 后跟 "空格 + 大写字母"（典型新句子开头），而非末尾等待
func lastSentenceBreak(s string) int {
	idx := -1
	bytes := []byte(s)
	bytePos := 0
	for _, r := range s {
		rLen := utf8.RuneLen(r)
		if strings.ContainsRune("。！？\n", r) {
			// 中文结束符 / 换行：直接切
			idx = bytePos + rLen
		} else if strings.ContainsRune("!?", r) {
			// 英文 !?：后跟空格时才切（末尾不切）
			nextPos := bytePos + rLen
			if nextPos < len(bytes) && (bytes[nextPos] == ' ' || bytes[nextPos] == '\t') {
				idx = nextPos
			}
		} else if r == '.' {
			// 英文句号：区分句末与版本号/小数点
			// 切分条件：前一字符非数字 AND 后跟空格（不是数字开头的版本号）
			nextPos := bytePos + rLen
			prevIsDigit := bytePos > 0 && bytes[bytePos-1] >= '0' && bytes[bytePos-1] <= '9'
			if !prevIsDigit && nextPos < len(bytes) && !(bytes[nextPos] >= '0' && bytes[nextPos] <= '9') {
				if bytes[nextPos] == ' ' || bytes[nextPos] == '\t' {
					idx = nextPos
				}
			}
			// 末尾不切：等待下一个 token 确认是版本号还是真正句末
		}
		bytePos += rLen
	}
	if idx < 0 {
		return 0
	}
	return idx
}

// sendOpus 将 PCM int16 切成 960 样本帧并逐帧编码为 Opus 推给客户端。
func (s *session) sendOpus(pcm []int16) error {
	if s.opusEnc == nil {
		return fmt.Errorf("opus encoder not initialized")
	}
	out := make([]byte, opusMaxFrameLen)
	for len(pcm) > 0 {
		frame := pcm
		if len(frame) > frameSamples {
			frame = pcm[:frameSamples]
		}
		// Opus 要求帧长为合法值；若最后一帧不足 960，补零
		if len(frame) < frameSamples {
			padded := make([]int16, frameSamples)
			copy(padded, frame)
			frame = padded
		}
		n, err := s.opusEnc.Encode(frame, out)
		if err != nil {
			return fmt.Errorf("encode: %w", err)
		}
		if err := s.writeBinary(out[:n]); err != nil {
			return err
		}
		if len(pcm) <= frameSamples {
			break
		}
		pcm = pcm[frameSamples:]
	}
	return nil
}

func (s *session) writeText(data []byte) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	return s.conn.WriteMessage(websocket.TextMessage, data)
}

func (s *session) writeBinary(data []byte) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	return s.conn.WriteMessage(websocket.BinaryMessage, data)
}
