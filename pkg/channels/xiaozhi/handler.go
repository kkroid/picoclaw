package xiaozhi

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"

	"github.com/sipeed/picoclaw/pkg/asr"
	"github.com/sipeed/picoclaw/pkg/logger"
	"github.com/sipeed/picoclaw/pkg/memory"
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
// frame 内容格式与协商的 audioFmt 一致，例如 PCM 字节流或 Ogg Opus 字节流。
type asrChunk struct {
	frame  []byte
	isLast bool
}

type turnOutputMode string

const (
	turnOutputModeTextOnly     turnOutputMode = "text-only"
	turnOutputModeTextAndVoice turnOutputMode = "text-and-voice"
)

type session struct {
	conn     *websocket.Conn
	writeMu  sync.Mutex
	asr      asr.Provider
	tts      tts.Provider
	runtime  Runtime
	stores   sessionStores
	registry *deviceRegistry
	// defaultOwnerID / sessionScope 只描述语音通道内的身份与 session 解析策略。
	// 它们不再覆盖 connID，也不直接等价于底层会话主键。
	defaultOwnerID string
	sessionScope   string
	// connCtx/connCancel：session 级生命周期上下文。
	// deviceRegistry.register 在设备重连时调用 connCancel 驱逐旧连接，
	// 同时级联取消所有 pipeline 子 context。
	connCtx    context.Context
	connCancel context.CancelFunc
	// ownerID 是当前连接解析出的租户内 owner，不等价于 device 或 conn。
	ownerID string
	// boundOwnerID 表示设备显式绑定或从设备状态文件恢复出的 owner。
	// 仅当存在显式 owner 绑定时非空；default_owner_id 不会写入该字段。
	boundOwnerID string
	// deviceID：从 hello 消息提取的设备标识（MAC 或 device_id）
	deviceID string
	// connID：WebSocket 连接级 ID，服务端在 hello 时生成并回传客户端，客户端不关心。
	connID string
	// turnID：问题级 ID，由客户端在 VAD start（listen.start）时生成并下发。
	turnID string
	// requestedMemoryID 是客户端显式传入的 memory_id 覆盖值。
	requestedMemoryID string
	// sessionKey 是当前语音短期上下文使用的 key，会传给 AgentLoop。
	sessionKey string
	// ownerMemoryKey 预留给后续 owner 级长期记忆归档。
	ownerMemoryKey string

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

type sessionStores struct {
	owner   *ownerStore
	device  *voiceDeviceStore
	pending *memory.VoicePendingStore
}

func newSession(
	conn *websocket.Conn,
	asrProv asr.Provider,
	ttsProv tts.Provider,
	runtime Runtime,
	stores sessionStores,
	defaultOwnerID, sessionScope string,
	reg *deviceRegistry,
) *session {
	ctx, cancel := context.WithCancel(context.Background())
	return &session{
		conn:           conn,
		asr:            asrProv,
		tts:            ttsProv,
		runtime:        runtime,
		stores:         stores,
		registry:       reg,
		defaultOwnerID: defaultOwnerID,
		sessionScope:   sessionScope,
		connCtx:        ctx,
		connCancel:     cancel,
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
		explicitOwnerID := ""
		if err := json.Unmarshal(data, &msg); err == nil {
			s.deviceID = msg.DeviceID
			explicitOwnerID = normalizeVoiceKeySegment(msg.OwnerID)
		}
		if s.connID == "" {
			s.connID = uuid.New().String()
			logger.Infof("xiaozhi: conn_id=%s device=%s", s.connID, s.deviceID)
		}
		if s.deviceID != "" {
			s.registry.register(s.deviceID, s)
		}
		boundOwnerID := explicitOwnerID
		if boundOwnerID != "" {
			if err := s.bindDeviceOwner(boundOwnerID); err != nil {
				logger.WarnCF("xiaozhi", "Bind device owner failed", map[string]any{
					"device_id": s.deviceID,
					"owner_id":  boundOwnerID,
					"error":     err.Error(),
				})
			}
		} else {
			boundOwnerID = s.loadBoundOwnerID()
		}
		s.boundOwnerID = boundOwnerID
		if err := s.touchBoundDevice(); err != nil {
			logger.WarnCF("xiaozhi", "Touch voice device failed", map[string]any{
				"device_id": s.deviceID,
				"error":     err.Error(),
			})
		}
		s.audioFmt = s.asr.AudioFormat()
		if err := validateNegotiatedUpstreamAudioFormat(s.audioFmt); err != nil {
			fields := s.negotiatedAudioFormatFields()
			fields["error"] = err.Error()
			s.rejectSession(
				"Rejecting session due to unsupported negotiated upstream audio format",
				websocket.CloseUnsupportedData,
				"unsupported upstream format",
				fields,
			)
			return
		}
		s.logNegotiatedAudioFormat()
		effectiveOwnerID := resolveVoiceOwnerID(s.boundOwnerID, s.defaultOwnerID)
		s.writeText(helloReply(s.connID, effectiveOwnerID, s.audioFmt, s.tts.AudioFormat()))

	case "listen":
		var msg listenMsg
		if err := json.Unmarshal(data, &msg); err != nil {
			return
		}
		switch msg.State {
		case "start":
			s.prepareTurn(msg.SessionID, msg.MemoryID, "voice")
			outputMode := resolveTurnOutputMode("voice")

			// 实时 ASR 模式：listen start 时就建连，音频帧实时推送
			if rp, ok := s.asr.(asr.RealtimeProvider); ok {
				pipeCtx, pipeCancel := context.WithCancel(s.connCtx)
				s.setTurnCancel(pipeCancel)
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
					s.setTurnCancel(nil)
					logger.Errorf("xiaozhi: open asr session: %v", err)
				} else {
					feedCh := make(chan asrChunk, 64)
					s.asrMu.Lock()
					s.asrSess = sess
					s.asrFeedCh = feedCh
					s.asrMu.Unlock()
					turnID, sessionKey := s.turnID, s.sessionKey
					s.pipelineWg.Add(1)
					go func() {
						defer s.pipelineWg.Done()
						defer asrCancel()
						s.processStreamSpeech(pipeCtx, sess, feedCh, time.Now(), turnID, sessionKey, outputMode)
						s.asrMu.Lock()
						s.asrSess = nil
						s.asrFeedCh = nil
						s.asrMu.Unlock()
					}()
				}
			}
		case "end", "stop":
			outputMode := resolveTurnOutputMode("voice")
			feedCh := s.detachRealtimeASRFeed()
			if feedCh != nil {
				select {
				case feedCh <- asrChunk{isLast: true}:
				default:
					logger.Errorf("xiaozhi: asr feed channel full on finalize")
				}
			} else {
				s.cancelCurrentTurn()
				s.pipelineWg.Wait()
				buf := make([][]byte, len(s.audioBuf))
				copy(buf, s.audioBuf)
				turnID, sessionKey := s.turnID, s.sessionKey
				s.pipelineWg.Add(1)
				go func() {
					defer s.pipelineWg.Done()
					s.processSpeech(buf, turnID, sessionKey, outputMode)
				}()
			}
		}

	case "text":
		var msg textMsg
		if err := json.Unmarshal(data, &msg); err != nil {
			return
		}
		trimmedText := strings.TrimSpace(msg.Text)
		if trimmedText == "" {
			return
		}
		s.prepareTurn(msg.SessionID, msg.MemoryID, "text")
		outputMode := resolveTurnOutputMode("text")
		turnID, sessionKey := s.turnID, s.sessionKey
		s.pipelineWg.Add(1)
		go func() {
			defer s.pipelineWg.Done()
			s.processTextTurn(trimmedText, turnID, sessionKey, outputMode)
		}()

	case "abort":
		s.closeRealtimeASRSession()
		s.cancelCurrentTurn()
		s.writeText(newTts("abort", ""))
	}
}

func (s *session) handleAudio(data []byte) {
	if len(data) == 0 {
		logger.Infof("xiaozhi: empty audio frame, dropping")
		return
	}
	if err := s.validateAudioFrame(data); err != nil {
		fields := s.negotiatedAudioFormatFields()
		fields["frame_size"] = len(data)
		fields["error"] = err.Error()
		s.rejectSession(
			"Rejecting session due to invalid upstream audio frame",
			websocket.CloseUnsupportedData,
			"invalid upstream audio frame",
			fields,
		)
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

func (s *session) rejectSession(message string, closeCode int, closeReason string, fields map[string]any) {
	logger.ErrorCF("xiaozhi", message, fields)
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	_ = s.conn.WriteControl(
		websocket.CloseMessage,
		websocket.FormatCloseMessage(closeCode, closeReason),
		time.Now().Add(2*time.Second),
	)
	if s.connCancel != nil {
		s.connCancel()
	}
	_ = s.conn.Close()
}

func (s *session) setTurnCancel(cancel context.CancelFunc) {
	s.cancelMu.Lock()
	s.cancel = cancel
	s.cancelMu.Unlock()
}

func (s *session) cancelCurrentTurn() {
	s.cancelMu.Lock()
	if s.cancel != nil {
		s.cancel()
		s.cancel = nil
	}
	s.cancelMu.Unlock()
}

func (s *session) detachRealtimeASRFeed() chan asrChunk {
	s.asrMu.Lock()
	defer s.asrMu.Unlock()
	feedCh := s.asrFeedCh
	if feedCh != nil {
		s.asrSess = nil
		s.asrFeedCh = nil
	}
	return feedCh
}

func (s *session) closeRealtimeASRSession() {
	s.asrMu.Lock()
	defer s.asrMu.Unlock()
	if s.asrSess != nil {
		s.asrSess.Close()
	}
	s.asrSess = nil
	s.asrFeedCh = nil
}
