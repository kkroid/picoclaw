// PicoClaw - Ultra-lightweight personal AI agent
// License: MIT
//
// Copyright (c) 2026 PicoClaw contributors

package xiaozhi

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"

	"github.com/sipeed/picoclaw/pkg/agent"
	"github.com/sipeed/picoclaw/pkg/asr"
	"github.com/sipeed/picoclaw/pkg/logger"
	"github.com/sipeed/picoclaw/pkg/tts"
)

// ---- session ----

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
		logger.Infof("xiaozhi: device %s reconnected, evicting old session", deviceID)
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

// asrChunk 是实时 ASR 音频帧，isLast=true 表示结束帧。
// frame 内容格式与协商的 audioFmt 一致（常见为 PCM 字节）。
type asrChunk struct {
	frame  []byte
	isLast bool
}

// audioItem 是 TTS 正在合成的一句话，frameCh 流式传递音频帧。
// 阶段 B 合成时边回调边写入 frameCh，close(frameCh) 表示本句合成完毕。
type audioItem struct {
	text    string
	frameCh chan []byte // 流式音频帧，close 表示本句结束
}

type session struct {
	conn      *websocket.Conn
	writeMu   sync.Mutex
	asr       asr.Provider
	tts       tts.Provider
	agentLoop *agent.AgentLoop
	registry  *deviceRegistry
	// connCtx/connCancel：session 级生命周期上下文。
	// deviceRegistry.register 在设备重连时调用 connCancel 驱逐旧连接，
	// 同时级联取消所有 pipeline 子 context。
	connCtx    context.Context
	connCancel context.CancelFunc
	// llmSessOverride：通过 owner_id 设置，优先级最高。
	// 同时作为 connID 基础和强制 memoryID，确保所有渠道共享同一记忆上下文（跨渠道记忆统一）。
	llmSessOverride string
	// deviceID：从 hello 消息提取的设备标识（MAC 或 device_id）
	deviceID string
	// connID：WebSocket 连接级 ID，服务端在 hello 时生成并回传客户端，客户端不关心。
	connID string
	// turnID：问题级 ID，由客户端在 VAD start（listen.start）时生成并下发。
	turnID string
	// memoryID：LLM 多轮记忆 key，由客户端在 listen.start 时指定。
	memoryID string

	// audioFmt 是在 hello 阶段由 ASR provider 声明并写入 helloReply 的格式。
	audioFmt asr.AudioFormat
	audioBuf [][]byte

	// asrMu 保护 asrSess 和 asrFeedCh 的并发读写（主 goroutine 与 pipeline goroutine 之间）。
	asrMu     sync.Mutex
	asrSess   asr.StreamingSession
	asrFeedCh chan asrChunk

	// pipelineWg 跟踪当前 pipeline goroutine，新 listen.start 时等旧 pipeline 退出。
	pipelineWg sync.WaitGroup

	// cancelMu 保护当前 pipeline 的取消函数。
	cancelMu sync.Mutex
	cancel   context.CancelFunc
}

func newSession(conn *websocket.Conn, asrProv asr.Provider, ttsProv tts.Provider, al *agent.AgentLoop, llmSessOverride string, reg *deviceRegistry) *session {
	ctx, cancel := context.WithCancel(context.Background())
	return &session{
		conn:            conn,
		asr:             asrProv,
		tts:             ttsProv,
		agentLoop:       al,
		registry:        reg,
		connCtx:         ctx,
		connCancel:      cancel,
		llmSessOverride: llmSessOverride,
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
			logger.Infof("xiaozhi: session %s (%s) read error: %v", s.connID, s.deviceID, err)
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
			s.deviceID = msg.DeviceID
		}
		if s.connID == "" {
			if s.llmSessOverride != "" {
				s.connID = s.llmSessOverride
			} else {
				s.connID = uuid.New().String()
			}
			logger.Infof("xiaozhi: conn_id=%s device=%s", s.connID, s.deviceID)
		}
		if s.deviceID != "" {
			s.registry.register(s.deviceID, s)
		}
		s.audioFmt = s.asr.AudioFormat()
		s.writeText(helloReply(s.connID, s.audioFmt, s.tts.AudioFormat()))

	case "listen":
		var msg listenMsg
		if err := json.Unmarshal(data, &msg); err != nil {
			return
		}
		switch msg.State {
		case "start":
			s.audioBuf = s.audioBuf[:0]
			if msg.SessionID != "" {
				s.turnID = msg.SessionID
			} else {
				s.turnID = uuid.New().String()
			}
			logger.Infof("xiaozhi: turn_id=%s", s.turnID)
			if s.llmSessOverride != "" {
				s.memoryID = s.llmSessOverride
			} else if msg.MemoryID != "" {
				s.memoryID = msg.MemoryID
			} else if s.deviceID != "" {
				s.memoryID = s.deviceID
			} else {
				s.memoryID = s.connID
			}
			// 取消旧 pipeline 并等待其退出，确保新旧 pipeline 不交错
			s.cancelMu.Lock()
			if s.cancel != nil {
				s.cancel()
				s.cancel = nil
			}
			s.cancelMu.Unlock()
			s.pipelineWg.Wait()

			// 清理旧 ASR session
			s.asrMu.Lock()
			if s.asrSess != nil {
				s.asrSess.Close()
				s.asrSess = nil
				s.asrFeedCh = nil
			}
			s.asrMu.Unlock()

			// 实时 ASR 模式：listen start 时就建连，音频帧实时推送
			if rp, ok := s.asr.(asr.RealtimeProvider); ok {
				pipeCtx, pipeCancel := context.WithCancel(s.connCtx)
				s.cancelMu.Lock()
				s.cancel = pipeCancel
				s.cancelMu.Unlock()
				asrCtx, asrCancel := context.WithTimeout(pipeCtx, 30*time.Second)
				sess, err := rp.OpenSession(asrCtx, func(text string, final bool) {
					state := "recognizing"
					if final {
						state = "stop"
					}
					s.writeText(newStt(text, state))
				})
				if err != nil {
					asrCancel()
					pipeCancel()
					s.cancelMu.Lock()
					s.cancel = nil
					s.cancelMu.Unlock()
					logger.Errorf("xiaozhi: open asr session: %v", err)
				} else {
					feedCh := make(chan asrChunk, 64)
					s.asrMu.Lock()
					s.asrSess = sess
					s.asrFeedCh = feedCh
					s.asrMu.Unlock()
					turnID, memoryID := s.turnID, s.memoryID
					s.pipelineWg.Add(1)
					go func() {
						defer s.pipelineWg.Done()
						defer asrCancel()
						s.processStreamSpeech(pipeCtx, sess, feedCh, time.Now(), turnID, memoryID)
						s.asrMu.Lock()
						s.asrSess = nil
						s.asrFeedCh = nil
						s.asrMu.Unlock()
					}()
				}
			}
		case "end", "stop":
			s.asrMu.Lock()
			feedCh := s.asrFeedCh
			if feedCh != nil {
				s.asrSess = nil
				s.asrFeedCh = nil
			}
			s.asrMu.Unlock()
			if feedCh != nil {
				select {
				case feedCh <- asrChunk{isLast: true}:
				default:
					logger.Errorf("xiaozhi: asr feed channel full on finalize")
				}
			} else {
				s.cancelMu.Lock()
				if s.cancel != nil {
					s.cancel()
					s.cancel = nil
				}
				s.cancelMu.Unlock()
				s.pipelineWg.Wait()
				buf := make([][]byte, len(s.audioBuf))
				copy(buf, s.audioBuf)
				turnID, memoryID := s.turnID, s.memoryID
				s.pipelineWg.Add(1)
				go func() {
					defer s.pipelineWg.Done()
					s.processSpeech(buf, turnID, memoryID)
				}()
			}
		}

	case "abort":
		s.asrMu.Lock()
		if s.asrSess != nil {
			s.asrSess.Close()
			s.asrSess = nil
			s.asrFeedCh = nil
		}
		s.asrMu.Unlock()
		s.cancelMu.Lock()
		if s.cancel != nil {
			s.cancel()
			s.cancel = nil
		}
		s.cancelMu.Unlock()
		s.writeText(newTts("abort", ""))
	}
}

// validateAudioFrame 轻量校验帧格式是否与协商结果一致。
func (s *session) validateAudioFrame(data []byte) bool {
	if len(data) == 0 {
		logger.Infof("xiaozhi: empty audio frame, dropping")
		return false
	}
	switch s.audioFmt.Codec {
	case "pcm":
		if len(data)%2 != 0 {
			logger.Infof("xiaozhi: pcm frame size=%d not 16-bit aligned, dropping", len(data))
			return false
		}
	}
	return true
}

func (s *session) handleAudio(data []byte) {
	if !s.validateAudioFrame(data) {
		return
	}
	frame := make([]byte, len(data))
	copy(frame, data)
	s.asrMu.Lock()
	feedCh := s.asrFeedCh
	s.asrMu.Unlock()
	if feedCh != nil {
		select {
		case feedCh <- asrChunk{frame: frame}:
		default:
			logger.Infof("xiaozhi: asr feed channel full, dropping frame")
		}
	} else {
		s.audioBuf = append(s.audioBuf, frame)
	}
}

// processSpeech: ASR → LLM → TTS 三阶段并发流水线。
// turnID 和 memoryID 由调用方快照传入，避免与主 goroutine 的写入竞争。
func (s *session) processSpeech(frames [][]byte, turnID, memoryID string) {
	reqStart := time.Now()
	logger.Infof("xiaozhi: processSpeech: frames=%d", len(frames))

	ctx, cancel := context.WithCancel(s.connCtx)
	s.cancelMu.Lock()
	s.cancel = cancel
	s.cancelMu.Unlock()
	defer cancel()

	asrCtx, asrCancel := context.WithTimeout(ctx, 20*time.Second)
	defer asrCancel()

	var finalText string
	var asrErr error
	if sp, ok := s.asr.(asr.StreamingProvider); ok {
		asrErr = sp.TranscribeStream(asrCtx, frames, func(text string, final bool) {
			state := "recognizing"
			if final {
				state = "stop"
				finalText = text
			}
			s.writeText(newStt(text, state))
		})
	} else {
		finalText, asrErr = s.asr.Transcribe(asrCtx, frames)
		if asrErr == nil && strings.TrimSpace(finalText) != "" {
			s.writeText(newStt(finalText, "stop"))
		}
	}

	if asrErr != nil {
		if ctx.Err() == nil {
			logger.Errorf("xiaozhi: asr: %v", asrErr)
		}
		s.writeText(newTts("stop", ""))
		return
	}
	if strings.TrimSpace(finalText) == "" {
		s.writeText(newTts("stop", ""))
		return
	}
	logger.Infof("xiaozhi: asr=%q latency=%dms", finalText, time.Since(reqStart).Milliseconds())

	if turnID == "" {
		turnID = s.connID
	}
	if memoryID == "" {
		memoryID = s.connID
	}
	s.runPipeline(ctx, finalText, reqStart, turnID, memoryID)
}

// runPipeline 执行 LLM → TTS 三阶段并发流水线。
func (s *session) runPipeline(ctx context.Context, text string, reqStart time.Time, turnID, memoryID string) {
	sentCh := make(chan string, 8)
	readyCh := make(chan audioItem, 4)

	// 阶段 A: LLM 流式 → sentCh
	go func() {
		defer close(sentCh)
		if err := s.streamLLM(ctx, turnID, memoryID, text, reqStart, func(sentence string) {
			select {
			case sentCh <- sentence:
			case <-ctx.Done():
			}
		}); err != nil {
			if ctx.Err() == nil && err != context.Canceled && err != context.DeadlineExceeded {
				logger.Errorf("xiaozhi: llm: %v", err)
			}
		}
	}()

	// 阶段 B: sentCh → TTS 并发合成 → readyCh（流式）
	go func() {
		defer close(readyCh)
		for {
			select {
			case sentence, ok := <-sentCh:
				if !ok {
					return
				}
				frameCh := make(chan []byte, 32)
				select {
				case readyCh <- audioItem{text: sentence, frameCh: frameCh}:
				case <-ctx.Done():
					close(frameCh)
					return
				}
				go func(sentence string, frameCh chan []byte) {
					defer close(frameCh)
					synthErr := s.tts.SynthesizeFrames(ctx, sentence, "", func(frame []byte) {
						select {
						case frameCh <- frame:
						case <-ctx.Done():
						}
					})
					if synthErr != nil && ctx.Err() == nil {
						logger.Errorf("xiaozhi: tts sentence %q: %v", sentence, synthErr)
					}
				}(sentence, frameCh)
			case <-ctx.Done():
				return
			}
		}
	}()

	// 阶段 C: readyCh → 音频帧 → WebSocket（保序）
	firstItem := true
	for item := range readyCh {
		if firstItem {
			s.writeText(newTts("start", ""))
			firstItem = false
		}
		s.writeText(newTts("sentence_start", item.text))
		for frame := range item.frameCh {
			if err := s.writeBinary(frame); err != nil {
				logger.Errorf("xiaozhi: send audio frame: %v", err)
			}
		}
		s.writeText(newTts("sentence_end", ""))
	}

	s.writeText(newTts("stop", ""))
}

// processStreamSpeech 处理实时 ASR 流：并发发送音频帧 + 等待最终识别结果，然后运行 LLM/TTS 流水线。
func (s *session) processStreamSpeech(ctx context.Context, sess asr.StreamingSession, feedCh <-chan asrChunk, reqStart time.Time, turnID, memoryID string) {
	defer sess.Close()

	sendDone := make(chan struct{})
	go func() {
		defer close(sendDone)
		for {
			select {
			case chunk, ok := <-feedCh:
				if !ok {
					return
				}
				if err := sess.SendAudio(chunk.frame, chunk.isLast); err != nil {
					if ctx.Err() == nil {
						logger.Errorf("xiaozhi: asr send: %v", err)
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

	text, err := sess.Wait(ctx)
	<-sendDone

	if err != nil {
		if ctx.Err() == nil && !errors.Is(err, asr.ErrSessionClosed) {
			logger.Errorf("xiaozhi: asr: %v", err)
			s.writeText(newTts("stop", ""))
		}
		return
	}
	if strings.TrimSpace(text) == "" {
		s.writeText(newTts("stop", ""))
		return
	}
	logger.Infof("xiaozhi: asr=%q latency=%dms", text, time.Since(reqStart).Milliseconds())

	s.runPipeline(ctx, text, reqStart, turnID, memoryID)
}

// streamLLM 直接调用 AgentLoop 流式推理，按句触发 onSentence 回调。
func (s *session) streamLLM(ctx context.Context, turnID, memoryID, userText string, reqStart time.Time, onSentence func(string)) error {
	llmStart := time.Now()
	logger.Infof("xiaozhi: llm request: turn=%s memory=%s text=%q", turnID, memoryID, userText)
	const startTag = "<think>"
	const endTag = "</think>"

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
					logger.Errorf("xiaozhi: llm timeout: no first token after %s", firstTokenTimeout)
				} else {
					logger.Errorf("xiaozhi: llm timeout: no token for %s", interTokenTimeout)
				}
				llmCancel()
				return
			}
		}
	}()

	var normalBuf strings.Builder
	thinkTail := make([]byte, 0, len(endTag)-1)
	inThinking := false
	var thinkStart time.Time
	firstContent := reqStart

	flushNormal := func() {
		text := normalBuf.String()
		if idx := lastSentenceBreak(text); idx > 0 {
			sentence := text[:idx]
			remaining := text[idx:]
			normalBuf.Reset()
			normalBuf.WriteString(remaining)
			if trimmed := strings.TrimSpace(sentence); trimmed != "" {
				logger.Infof("xiaozhi: llm sentence: %q", trimmed)
				s.writeText(newLlmText(trimmed))
				onSentence(trimmed)
			}
		}
	}

	onToken := func(token string) {
		select {
		case tokenActivity <- struct{}{}:
		default:
		}
		for token != "" {
			if inThinking {
				scan := string(thinkTail) + token
				if idx := strings.Index(scan, endTag); idx >= 0 {
					durationMs := time.Since(thinkStart).Milliseconds()
					logger.Infof("xiaozhi: thinking=%dms", durationMs)
					s.writeText(newLlmThinkingEnd(durationMs))
					after := scan[idx+len(endTag):]
					thinkTail = thinkTail[:0]
					inThinking = false
					token = after
				} else {
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
					if !firstContent.IsZero() && before != "" {
						logger.Infof("xiaozhi: llm_first_token latency=%dms", time.Since(firstContent).Milliseconds())
						firstContent = time.Time{}
					}
					flushNormal()
					inThinking = true
					thinkStart = time.Now()
					thinkTail = thinkTail[:0]
					logger.Infof("xiaozhi: llm thinking...")
					s.writeText(newLlmThinkingStart())
					token = after
				} else {
					if !firstContent.IsZero() && strings.TrimSpace(normalBuf.String()) != "" {
						logger.Infof("xiaozhi: llm_first_token latency=%dms", time.Since(firstContent).Milliseconds())
						firstContent = time.Time{}
					}
					flushNormal()
				}
			}
		}
	}

	if err := s.agentLoop.RunStreamAgentLoop(llmCtx, memoryID, userText, "xiaozhi", turnID, onToken); err != nil {
		return err
	}

	if !inThinking && normalBuf.Len() > 0 {
		if trimmed := strings.TrimSpace(normalBuf.String()); trimmed != "" {
			logger.Infof("xiaozhi: llm sentence: %q", trimmed)
			s.writeText(newLlmText(trimmed))
			onSentence(trimmed)
		}
	}

	logger.Infof("xiaozhi: llm done: turn=%s latency=%dms", turnID, time.Since(llmStart).Milliseconds())
	return nil
}

// lastSentenceBreak 返回字符串中最后一个句子边界之后的字节偏移。
func lastSentenceBreak(s string) int {
	idx := -1
	bytes := []byte(s)
	bytePos := 0
	for _, r := range s {
		rLen := utf8.RuneLen(r)
		if strings.ContainsRune("。！？\n", r) {
			idx = bytePos + rLen
		} else if strings.ContainsRune("!?", r) {
			nextPos := bytePos + rLen
			if nextPos < len(bytes) && (bytes[nextPos] == ' ' || bytes[nextPos] == '\t') {
				idx = nextPos
			}
		} else if r == '.' {
			nextPos := bytePos + rLen
			prevIsDigit := bytePos > 0 && bytes[bytePos-1] >= '0' && bytes[bytePos-1] <= '9'
			if !prevIsDigit && nextPos < len(bytes) && !(bytes[nextPos] >= '0' && bytes[nextPos] <= '9') {
				if bytes[nextPos] == ' ' || bytes[nextPos] == '\t' {
					idx = nextPos
				}
			}
		}
		bytePos += rLen
	}
	if idx < 0 {
		return 0
	}
	return idx
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
