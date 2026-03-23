// PicoClaw - Ultra-lightweight personal AI agent
// License: MIT
//
// Copyright (c) 2026 PicoClaw contributors

package xiaozhi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"

	"github.com/sipeed/picoclaw/pkg/agent"
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
	turnOutputModeTextOnly                    turnOutputMode = "text-only"
	turnOutputModeTextAndVoice                turnOutputMode = "text-and-voice"
	voiceReplyMaxRunesForSpeech                              = 96
	voiceReplyMaxSentencesForSpeech                          = 3
	voiceReplyMaxSingleSentenceRunesForSpeech                = 48
	voiceReplySummaryMaxRunesForSpeech                       = 72
	voiceReplySummaryMaxSentencesForSpeech                   = 2
	voiceReplyStructuredMaxTitlesForSpeech                   = 5
)

type llmStreamResult struct {
	finalContent string
	sentences    []string
}

type voiceReplySpeechPlan struct {
	mode       turnOutputMode
	items      []string
	condensed  bool
	skipReason string
}

type pendingAnnouncementBatch struct {
	summary string
	items   []memory.VoicePendingItem
}

func resolveTurnOutputMode(inputType string) turnOutputMode {
	if strings.EqualFold(strings.TrimSpace(inputType), "text") {
		return turnOutputModeTextOnly
	}
	return turnOutputModeTextAndVoice
}

func decideVoiceReplyOutputMode(finalContent string, sentences []string) turnOutputMode {
	return buildVoiceReplySpeechPlan(finalContent, sentences).mode
}

func buildVoiceReplySpeechPlan(finalContent string, sentences []string) voiceReplySpeechPlan {
	finalContent = strings.TrimSpace(finalContent)
	if finalContent == "" {
		return voiceReplySpeechPlan{mode: turnOutputModeTextOnly, skipReason: "empty"}
	}
	normalized := normalizeVoiceReplySentences(finalContent, sentences)
	if len(normalized) == 0 {
		return voiceReplySpeechPlan{mode: turnOutputModeTextOnly, skipReason: "empty"}
	}
	if isStructuredVoiceReply(finalContent) {
		if condensed := buildStructuredVoiceReplyItems(normalized); len(condensed) > 0 {
			return voiceReplySpeechPlan{mode: turnOutputModeTextAndVoice, items: condensed, condensed: true}
		}
		return voiceReplySpeechPlan{mode: turnOutputModeTextOnly, skipReason: "structured"}
	}
	if voiceReplyFitsSpeechBudget(
		normalized,
		voiceReplyMaxRunesForSpeech,
		voiceReplyMaxSentencesForSpeech,
		voiceReplyMaxSingleSentenceRunesForSpeech,
	) {
		return voiceReplySpeechPlan{mode: turnOutputModeTextAndVoice, items: normalized}
	}
	condensed := buildCondensedVoiceReplyItems(normalized)
	if len(condensed) == 0 {
		return voiceReplySpeechPlan{mode: turnOutputModeTextOnly, skipReason: "too-long"}
	}
	return voiceReplySpeechPlan{mode: turnOutputModeTextAndVoice, items: condensed, condensed: true}
}

func normalizeVoiceReplySentences(finalContent string, sentences []string) []string {
	items := make([]string, 0, len(sentences)+1)
	for _, sentence := range sentences {
		sentence = strings.TrimSpace(sentence)
		if sentence == "" {
			continue
		}
		items = append(items, sentence)
	}
	if len(items) == 0 && finalContent != "" {
		items = append(items, finalContent)
	}
	return items
}

func voiceReplyFitsSpeechBudget(sentences []string, maxRunes, maxSentences, maxSingleSentenceRunes int) bool {
	if len(sentences) == 0 {
		return false
	}
	if len(sentences) > maxSentences {
		return false
	}
	totalRunes := 0
	for _, sentence := range sentences {
		runes := utf8.RuneCountInString(sentence)
		if runes > maxSingleSentenceRunes {
			return false
		}
		totalRunes += runes
		if totalRunes > maxRunes {
			return false
		}
	}
	return true
}

func buildCondensedVoiceReplyItems(sentences []string) []string {
	items := make([]string, 0, voiceReplySummaryMaxSentencesForSpeech)
	totalRunes := 0
	for _, sentence := range sentences {
		if len(items) >= voiceReplySummaryMaxSentencesForSpeech {
			break
		}
		sentence = shortenVoiceSentenceForSpeech(sentence, voiceReplyMaxSingleSentenceRunesForSpeech)
		if sentence == "" {
			continue
		}
		runes := utf8.RuneCountInString(sentence)
		if len(items) > 0 && totalRunes+runes > voiceReplySummaryMaxRunesForSpeech {
			break
		}
		if len(items) == 0 && runes > voiceReplySummaryMaxRunesForSpeech {
			sentence = shortenVoiceSentenceForSpeech(sentence, voiceReplySummaryMaxRunesForSpeech)
			runes = utf8.RuneCountInString(sentence)
		}
		if sentence == "" {
			continue
		}
		items = append(items, sentence)
		totalRunes += runes
		if totalRunes >= voiceReplySummaryMaxRunesForSpeech {
			break
		}
	}
	if len(items) == 0 && len(sentences) > 0 {
		if shortened := shortenVoiceSentenceForSpeech(
			sentences[0],
			voiceReplySummaryMaxRunesForSpeech,
		); shortened != "" {
			items = append(items, shortened)
		}
	}
	return items
}

func buildStructuredVoiceReplyItems(sentences []string) []string {
	intro := ""
	titles := make([]string, 0, voiceReplyStructuredMaxTitlesForSpeech)
	for _, sentence := range sentences {
		raw := strings.TrimSpace(sentence)
		if raw == "" {
			continue
		}
		if strings.Contains(raw, "```") || strings.Contains(raw, "|") {
			return nil
		}
		if strings.Contains(raw, "http://") || strings.Contains(raw, "https://") {
			return nil
		}
		if embeddedIntro, embeddedList, ok := splitEmbeddedVoiceEnumeration(raw); ok {
			if intro == "" {
				intro = sanitizeVoiceSpeechText(embeddedIntro)
			}
			if title := extractVoiceEnumerationTitle(embeddedList); title != "" {
				titles = append(titles, title)
				if len(titles) >= voiceReplyStructuredMaxTitlesForSpeech {
					break
				}
			}
			continue
		}
		if trimmed, ok := trimVoiceListPrefix(raw); ok {
			title := extractVoiceEnumerationTitle(trimmed)
			if title != "" {
				titles = append(titles, title)
				if len(titles) >= voiceReplyStructuredMaxTitlesForSpeech {
					break
				}
			}
			continue
		}
		if intro == "" {
			intro = sanitizeVoiceSpeechText(raw)
		}
	}
	if len(titles) == 0 {
		return nil
	}
	firstCount := 3
	if len(titles) < firstCount {
		firstCount = len(titles)
	}
	firstBatch := joinVoiceSpeechTitles(titles[:firstCount])
	items := make([]string, 0, voiceReplySummaryMaxSentencesForSpeech)
	intro = trimStructuredIntroForSpeech(intro)
	if intro != "" {
		connector := "，比如"
		if strings.HasSuffix(intro, "包括") || strings.HasSuffix(intro, "例如") || strings.HasSuffix(intro, "比如") {
			connector = ""
		}
		items = append(
			items,
			shortenVoiceSentenceForSpeech(intro+connector+firstBatch+"。", voiceReplyMaxSingleSentenceRunesForSpeech),
		)
	} else {
		items = append(
			items,
			shortenVoiceSentenceForSpeech("主要包括"+firstBatch+"。", voiceReplyMaxSingleSentenceRunesForSpeech),
		)
	}
	if len(titles) > firstCount {
		items = append(
			items,
			shortenVoiceSentenceForSpeech(
				"也能"+joinVoiceSpeechTitles(titles[firstCount:])+"。",
				voiceReplyMaxSingleSentenceRunesForSpeech,
			),
		)
	}
	return compactVoiceReplyItems(items)
}

func splitEmbeddedVoiceEnumeration(text string) (string, string, bool) {
	text = strings.TrimSpace(text)
	for idx, r := range text {
		if r != '：' && r != ':' {
			continue
		}
		rest := strings.TrimSpace(text[idx+utf8.RuneLen(r):])
		if trimmed, ok := trimVoiceListPrefix(rest); ok {
			return strings.TrimSpace(text[:idx]), trimmed, true
		}
	}
	return "", "", false
}

func compactVoiceReplyItems(items []string) []string {
	compacted := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		compacted = append(compacted, item)
	}
	return compacted
}

func trimVoiceListPrefix(text string) (string, bool) {
	text = strings.TrimSpace(text)
	for _, prefix := range []string{"- ", "* ", "• "} {
		if strings.HasPrefix(text, prefix) {
			return strings.TrimSpace(text[len(prefix):]), true
		}
	}
	idx := 0
	for idx < len(text) && text[idx] >= '0' && text[idx] <= '9' {
		idx++
	}
	if idx == 0 || idx >= len(text) {
		return text, false
	}
	rest := text[idx:]
	switch {
	case strings.HasPrefix(rest, "."),
		strings.HasPrefix(rest, "、"),
		strings.HasPrefix(rest, ")"),
		strings.HasPrefix(rest, "）"):
		_, size := utf8.DecodeRuneInString(rest)
		idx += size
		for idx < len(text) && (text[idx] == ' ' || text[idx] == '\t') {
			idx++
		}
		return strings.TrimSpace(text[idx:]), true
	default:
		return text, false
	}
}

func extractVoiceEnumerationTitle(text string) string {
	text = sanitizeVoiceSpeechText(text)
	if idx := strings.IndexAny(text, "：:"); idx > 0 {
		text = text[:idx]
	}
	return trimVoiceSentenceTerminalPunctuation(text)
}

func sanitizeVoiceSpeechText(text string) string {
	replacer := strings.NewReplacer(
		"**", "",
		"__", "",
		"`", "",
		"#", "",
		"*", "",
	)
	text = replacer.Replace(strings.TrimSpace(text))
	return strings.Join(strings.Fields(text), " ")
}

func trimStructuredIntroForSpeech(text string) string {
	text = trimVoiceSentenceTerminalPunctuation(sanitizeVoiceSpeechText(text))
	text = strings.TrimRight(text, "，,、；;：: ")
	return strings.TrimSpace(text)
}

func trimVoiceSentenceTerminalPunctuation(text string) string {
	return strings.TrimSpace(strings.TrimRight(strings.TrimSpace(text), "。！？.!?：:，,；;、"))
}

func joinVoiceSpeechTitles(titles []string) string {
	cleaned := make([]string, 0, len(titles))
	for _, title := range titles {
		title = trimVoiceSentenceTerminalPunctuation(title)
		if title == "" {
			continue
		}
		cleaned = append(cleaned, title)
	}
	if len(cleaned) == 0 {
		return ""
	}
	if len(cleaned) == 1 {
		return cleaned[0]
	}
	if len(cleaned) == 2 {
		return cleaned[0] + "和" + cleaned[1]
	}
	return strings.Join(cleaned[:len(cleaned)-1], "、") + "和" + cleaned[len(cleaned)-1]
}

func shortenVoiceSentenceForSpeech(sentence string, maxRunes int) string {
	sentence = strings.TrimSpace(sentence)
	if sentence == "" || maxRunes <= 0 {
		return ""
	}
	runes := []rune(sentence)
	if len(runes) <= maxRunes {
		return sentence
	}
	cut := -1
	searchStart := maxRunes / 2
	if searchStart < 1 {
		searchStart = 1
	}
	for i := maxRunes - 1; i >= searchStart; i-- {
		if isVoiceSpeechPauseRune(runes[i]) {
			cut = i + 1
			break
		}
	}
	if cut <= 0 {
		if maxRunes <= 3 {
			return strings.TrimSpace(string(runes[:maxRunes]))
		}
		cut = maxRunes - 3
	}
	shortened := strings.TrimSpace(string(runes[:cut]))
	shortened = strings.TrimRight(shortened, "，,、；;：: ")
	if shortened == "" {
		shortened = strings.TrimSpace(string(runes[:cut]))
	}
	if shortened == "" {
		return ""
	}
	if endsWithTerminalPunctuation(shortened) {
		return shortened
	}
	return shortened + "..."
}

func isVoiceSpeechPauseRune(r rune) bool {
	return strings.ContainsRune("，,、；;：:。！？!?", r)
}

func endsWithTerminalPunctuation(text string) bool {
	text = strings.TrimSpace(text)
	if text == "" {
		return false
	}
	r, _ := utf8.DecodeLastRuneInString(text)
	return strings.ContainsRune("。！？.!?", r)
}

func isStructuredVoiceReply(content string) bool {
	content = strings.TrimSpace(content)
	if content == "" {
		return false
	}
	if strings.Contains(content, "```") || strings.Contains(content, "|") {
		return true
	}
	if strings.Contains(content, "http://") || strings.Contains(content, "https://") {
		return true
	}
	if containsInlineVoiceEnumeration(content) {
		return true
	}
	if strings.Count(content, "\n") >= 2 {
		return true
	}
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "- ") || strings.HasPrefix(line, "* ") || strings.HasPrefix(line, "•") {
			return true
		}
		if len(line) >= 3 && line[0] >= '0' && line[0] <= '9' && line[1] == '.' && line[2] == ' ' {
			return true
		}
	}
	return false
}

func containsInlineVoiceEnumeration(content string) bool {
	content = strings.TrimSpace(content)
	if _, ok := trimVoiceListPrefix(content); ok {
		return true
	}
	for idx, r := range content {
		if r < '0' || r > '9' {
			continue
		}
		if idx > 0 {
			prev, _ := utf8.DecodeLastRuneInString(content[:idx])
			if !strings.ContainsRune(" ：:，,；;。！？!?\n", prev) {
				continue
			}
		}
		if _, ok := trimVoiceListPrefix(content[idx:]); ok {
			return true
		}
	}
	return false
}

type session struct {
	conn         *websocket.Conn
	writeMu      sync.Mutex
	asr          asr.Provider
	tts          tts.Provider
	agentLoop    *agent.AgentLoop
	ownerStore   *ownerStore
	deviceStore  *voiceDeviceStore
	pendingStore *memory.VoicePendingStore
	registry     *deviceRegistry
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

func newSession(
	conn *websocket.Conn,
	asrProv asr.Provider,
	ttsProv tts.Provider,
	al *agent.AgentLoop,
	ownerStore *ownerStore,
	deviceStore *voiceDeviceStore,
	pendingStore *memory.VoicePendingStore,
	defaultOwnerID, sessionScope string,
	reg *deviceRegistry,
) *session {
	ctx, cancel := context.WithCancel(context.Background())
	return &session{
		conn:           conn,
		asr:            asrProv,
		tts:            ttsProv,
		agentLoop:      al,
		ownerStore:     ownerStore,
		deviceStore:    deviceStore,
		pendingStore:   pendingStore,
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

func normalizeUpstreamAudioSpec(formatName, codec string) (string, string) {
	formatName = strings.ToLower(strings.TrimSpace(formatName))
	codec = strings.ToLower(strings.TrimSpace(codec))
	if formatName == "pcm" && codec == "" {
		codec = "raw"
	}
	return formatName, codec
}

// validateNegotiatedUpstreamAudioFormat 校验当前 xiaozhi 通道是否真的支持该上行格式。
// 当前通道只做字节流透传，不做音频编解码；因此协商结果必须能被客户端直接生成、并能被 ASR provider 直接接收。
func validateNegotiatedUpstreamAudioFormat(format asr.AudioFormat) error {
	formatName, codec := normalizeUpstreamAudioSpec(format.Format, format.Codec)
	if formatName == "" {
		return fmt.Errorf("empty upstream audio format")
	}
	switch formatName {
	case "pcm":
		if codec != "raw" {
			return fmt.Errorf("upstream format %q requires codec raw, got %q", formatName, codec)
		}
	case "ogg":
		if codec != "opus" {
			return fmt.Errorf("upstream format %q requires codec opus, got %q", formatName, codec)
		}
	default:
		return fmt.Errorf("upstream format %q codec %q is not supported by xiaozhi passthrough", formatName, codec)
	}
	if format.SampleRate != 16000 {
		return fmt.Errorf("upstream sample rate %d is not supported, want 16000", format.SampleRate)
	}
	if format.Channels != 1 {
		return fmt.Errorf("upstream channels %d are not supported, want 1", format.Channels)
	}
	return nil
}

// validateAudioFrame 校验音频帧是否满足当前协商结果。
func (s *session) validateAudioFrame(data []byte) error {
	formatName, codec := normalizeUpstreamAudioSpec(s.audioFmt.Format, s.audioFmt.Codec)
	switch formatName {
	case "pcm":
		if codec != "raw" {
			return fmt.Errorf("negotiated upstream format %q codec %q is not supported", formatName, codec)
		}
		if len(data)%2 != 0 {
			return fmt.Errorf("pcm frame size %d is not 16-bit aligned", len(data))
		}
	case "ogg":
		if codec != "opus" {
			return fmt.Errorf("negotiated upstream format %q codec %q is not supported", formatName, codec)
		}
	default:
		return fmt.Errorf("negotiated upstream format %q codec %q is not supported", formatName, codec)
	}
	return nil
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

func (s *session) logNegotiatedAudioFormat() {
	logger.InfoCF("xiaozhi", "Negotiated upstream audio format", s.negotiatedAudioFormatFields())
}

func (s *session) negotiatedAudioFormatFields() map[string]any {
	serverFormat, serverCodec := normalizeUpstreamAudioSpec(s.audioFmt.Format, s.audioFmt.Codec)
	return map[string]any{
		"server_format":      serverFormat,
		"server_codec":       serverCodec,
		"server_sample_rate": s.audioFmt.SampleRate,
		"server_channels":    s.audioFmt.Channels,
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

func (s *session) prepareTurn(requestedTurnID, requestedMemoryID, inputType string) {
	s.audioBuf = s.audioBuf[:0]
	if requestedTurnID != "" {
		s.turnID = requestedTurnID
	} else {
		s.turnID = uuid.New().String()
	}

	resolvedOwnerID := resolveVoiceOwnerID(s.boundOwnerID, s.defaultOwnerID)
	ctxInfo := buildVoiceSessionContext(
		resolvedOwnerID,
		s.sessionScope,
		s.deviceID,
		s.connID,
		s.turnID,
		requestedMemoryID,
	)
	s.ownerID = ctxInfo.OwnerID
	s.requestedMemoryID = ctxInfo.RequestedMemoryID
	s.sessionKey = ctxInfo.SessionKey
	s.ownerMemoryKey = ctxInfo.OwnerMemoryKey
	if err := s.ensureOwnerStructuredFiles(); err != nil {
		logger.WarnCF("xiaozhi", "Ensure owner store failed", map[string]any{
			"owner_id": ctxInfo.OwnerID,
			"error":    err.Error(),
		})
	}
	logger.InfoCF("xiaozhi", "Conversation turn context resolved", map[string]any{
		"conn_id":             s.connID,
		"turn_id":             s.turnID,
		"owner_id":            s.ownerID,
		"device_id":           s.deviceID,
		"input_type":          strings.TrimSpace(inputType),
		"session_scope":       ctxInfo.SessionScope,
		"session_key":         s.sessionKey,
		"owner_memory_key":    s.ownerMemoryKey,
		"requested_memory_id": s.requestedMemoryID,
	})

	// 取消旧 pipeline 并等待其退出，确保新旧 turn 不交错。
	s.cancelMu.Lock()
	if s.cancel != nil {
		s.cancel()
		s.cancel = nil
	}
	s.cancelMu.Unlock()
	s.pipelineWg.Wait()

	// 清理旧 ASR session，避免文本 turn 复用到旧语音流状态。
	s.asrMu.Lock()
	if s.asrSess != nil {
		s.asrSess.Close()
		s.asrSess = nil
		s.asrFeedCh = nil
	}
	s.asrMu.Unlock()
}

// processSpeech: ASR → LLM → 文本/语音下行流水线。
// turnID 和 sessionKey 由调用方快照传入，避免与主 goroutine 的写入竞争。
func (s *session) processSpeech(frames [][]byte, turnID, sessionKey string, outputMode turnOutputMode) {
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
	if sessionKey == "" {
		sessionKey = buildVoiceSessionKey(s.ownerID, s.deviceID, s.connID, s.sessionScope)
	}
	s.runPipeline(ctx, finalText, reqStart, turnID, sessionKey, outputMode)
}

// processTextTurn 处理 xiaozhi 文本输入：默认只走文本输出，不触发 TTS。
func (s *session) processTextTurn(text, turnID, sessionKey string, outputMode turnOutputMode) {
	if strings.TrimSpace(text) == "" {
		return
	}
	reqStart := time.Now()
	logger.Infof("xiaozhi: text turn=%s session=%s owner=%s text=%q", turnID, sessionKey, s.ownerID, text)

	ctx, cancel := context.WithCancel(s.connCtx)
	s.cancelMu.Lock()
	s.cancel = cancel
	s.cancelMu.Unlock()
	defer cancel()

	if turnID == "" {
		turnID = s.connID
	}
	if sessionKey == "" {
		sessionKey = buildVoiceSessionKey(s.ownerID, s.deviceID, s.connID, s.sessionScope)
	}
	s.runPipeline(ctx, text, reqStart, turnID, sessionKey, outputMode)
}

// runPipeline 执行 LLM → 文本/语音下行流水线。
func (s *session) runPipeline(
	ctx context.Context,
	text string,
	reqStart time.Time,
	turnID, sessionKey string,
	outputMode turnOutputMode,
) {
	llmResult, err := s.streamLLM(ctx, turnID, sessionKey, text, reqStart)
	if err != nil {
		if ctx.Err() == nil && err != context.Canceled && err != context.DeadlineExceeded {
			logger.Errorf("xiaozhi: llm: %v", err)
		}
		return
	}
	if ctx.Err() != nil {
		return
	}

	if outputMode == turnOutputModeTextOnly {
		logger.InfoCF("xiaozhi", "Skipping TTS because current turn is text-only", map[string]any{
			"turn_id":        turnID,
			"session_key":    sessionKey,
			"content_len":    len(llmResult.finalContent),
			"sentence_count": len(llmResult.sentences),
		})
		s.writeText(newLlmStop())
		return
	}

	voicePlan := buildVoiceReplySpeechPlan(llmResult.finalContent, llmResult.sentences)
	if voicePlan.mode == turnOutputModeTextOnly {
		logger.InfoCF("xiaozhi", "Skipping TTS because voice reply is too long or structured", map[string]any{
			"turn_id":          turnID,
			"session_key":      sessionKey,
			"content_len":      len(llmResult.finalContent),
			"sentence_count":   len(llmResult.sentences),
			"structured_reply": isStructuredVoiceReply(llmResult.finalContent),
			"skip_reason":      voicePlan.skipReason,
		})
		s.finishVoiceTurnWithoutSpeech()
		return
	}

	if voicePlan.condensed {
		logger.InfoCF("xiaozhi", "Condensing voice reply for TTS", map[string]any{
			"turn_id":                 turnID,
			"session_key":             sessionKey,
			"content_len":             len(llmResult.finalContent),
			"original_sentence_count": len(llmResult.sentences),
			"speech_sentence_count":   len(voicePlan.items),
		})
	}

	items := make([]string, 0, len(voicePlan.items)+1)
	pendingAnnouncement := s.prepareOwnerPendingAnnouncement()
	if pendingAnnouncement.summary != "" {
		items = append(items, pendingAnnouncement.summary)
	}
	items = append(items, voicePlan.items...)
	if len(items) == 0 {
		if trimmed := strings.TrimSpace(llmResult.finalContent); trimmed != "" {
			items = append(items, trimmed)
		}
	}
	if len(items) == 0 {
		s.finishVoiceTurnWithoutSpeech()
		return
	}

	if s.playSpeechItems(ctx, items) && pendingAnnouncement.summary != "" {
		if err := s.confirmOwnerPendingAnnouncement(pendingAnnouncement); err != nil {
			logger.WarnCF("xiaozhi", "Confirm voice pending summary failed", map[string]any{
				"owner_id": s.ownerID,
				"error":    err.Error(),
			})
		}
	}
}

func (s *session) finishVoiceTurnWithoutSpeech() {
	if err := s.writeText(newLlmStop()); err != nil {
		logger.Errorf("xiaozhi: send llm.stop: %v", err)
		return
	}
	if err := s.writeText(newTts("stop", "")); err != nil {
		logger.Errorf("xiaozhi: send compatibility tts.stop: %v", err)
	}
}

// processStreamSpeech 处理实时 ASR 流：并发发送音频帧 + 等待最终识别结果，然后运行 LLM/TTS 流水线。
func (s *session) processStreamSpeech(
	ctx context.Context,
	sess asr.StreamingSession,
	feedCh <-chan asrChunk,
	reqStart time.Time,
	turnID, sessionKey string,
	outputMode turnOutputMode,
) {
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

	s.runPipeline(ctx, text, reqStart, turnID, sessionKey, outputMode)
}

// streamLLM 直接调用 AgentLoop 流式推理，并返回本轮整理后的句子与最终回复内容。
func (s *session) streamLLM(
	ctx context.Context,
	turnID, sessionKey, userText string,
	reqStart time.Time,
) (llmStreamResult, error) {
	result := llmStreamResult{}
	llmStart := time.Now()
	logger.Infof("xiaozhi: llm request: turn=%s session=%s owner=%s text=%q", turnID, sessionKey, s.ownerID, userText)
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
				result.sentences = append(result.sentences, trimmed)
				logger.Infof("xiaozhi: llm sentence: %q", trimmed)
				s.writeText(newLlmText(trimmed))
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

	if err := s.agentLoop.RunStreamAgentLoopWithKeys(llmCtx, agent.StreamConversationKeys{
		SessionKey: sessionKey,
		MemoryKey:  s.ownerMemoryKey,
	}, userText, "xiaozhi", turnID, onToken); err != nil {
		return result, err
	}
	result.finalContent = s.loadFinalAssistantContent(sessionKey)
	if err := s.persistOwnerVoiceSnapshot(turnID, sessionKey); err != nil {
		logger.WarnCF("xiaozhi", "Persist owner voice snapshot failed", map[string]any{
			"owner_id":   s.ownerID,
			"memory_key": s.ownerMemoryKey,
			"error":      err.Error(),
		})
	}

	if !inThinking && normalBuf.Len() > 0 {
		if trimmed := strings.TrimSpace(normalBuf.String()); trimmed != "" {
			result.sentences = append(result.sentences, trimmed)
			logger.Infof("xiaozhi: llm sentence: %q", trimmed)
			s.writeText(newLlmText(trimmed))
		}
	}
	if result.finalContent == "" && len(result.sentences) > 0 {
		result.finalContent = strings.Join(result.sentences, "")
	}

	logger.Infof("xiaozhi: llm done: turn=%s latency=%dms", turnID, time.Since(llmStart).Milliseconds())
	return result, nil
}

func (s *session) playSpeechItems(ctx context.Context, items []string) bool {
	started := false
	textStopSent := false
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if !textStopSent {
			if err := s.writeText(newLlmStop()); err != nil {
				logger.Errorf("xiaozhi: send llm.stop: %v", err)
				return false
			}
			textStopSent = true
		}
		if !started {
			if err := s.writeText(newTts("start", "")); err != nil {
				logger.Errorf("xiaozhi: send tts.start: %v", err)
				return false
			}
			started = true
		}
		if err := s.writeText(newTts("sentence_start", item)); err != nil {
			logger.Errorf("xiaozhi: send tts.sentence_start: %v", err)
			return false
		}
		frameWriteFailed := false
		synthErr := s.tts.SynthesizeFrames(ctx, item, "", func(frame []byte) {
			if err := s.writeBinary(frame); err != nil {
				logger.Errorf("xiaozhi: send audio frame: %v", err)
				frameWriteFailed = true
			}
		})
		if synthErr != nil && ctx.Err() == nil {
			logger.Errorf("xiaozhi: tts sentence %q: %v", item, synthErr)
			return false
		}
		if frameWriteFailed {
			return false
		}
		if err := s.writeText(newTts("sentence_end", "")); err != nil {
			logger.Errorf("xiaozhi: send tts.sentence_end: %v", err)
			return false
		}
		if ctx.Err() != nil {
			return false
		}
	}
	if started {
		if err := s.writeText(newTts("stop", "")); err != nil {
			logger.Errorf("xiaozhi: send tts.stop: %v", err)
			return false
		}
		return true
	}
	s.finishVoiceTurnWithoutSpeech()
	return true
}

func (s *session) loadFinalAssistantContent(sessionKey string) string {
	if s.agentLoop == nil {
		return ""
	}
	defaultAgent := s.agentLoop.GetRegistry().GetDefaultAgent()
	if defaultAgent == nil {
		return ""
	}
	history := defaultAgent.Sessions.GetHistory(sessionKey)
	for i := len(history) - 1; i >= 0; i-- {
		if history[i].Role != "assistant" {
			continue
		}
		content := strings.TrimSpace(history[i].Content)
		if content != "" {
			return content
		}
	}
	return ""
}

func (s *session) persistOwnerVoiceSnapshot(turnID, sessionKey string) error {
	if s.ownerStore == nil || s.agentLoop == nil || s.ownerID == "" || s.ownerMemoryKey == "" {
		return nil
	}

	defaultAgent := s.agentLoop.GetRegistry().GetDefaultAgent()
	if defaultAgent == nil {
		return nil
	}

	history := defaultAgent.Sessions.GetHistory(s.ownerMemoryKey)
	summary := defaultAgent.Sessions.GetSummary(s.ownerMemoryKey)
	snapshot := buildOwnerVoiceSnapshot(
		s.ownerID,
		s.ownerMemoryKey,
		sessionKey,
		"xiaozhi",
		s.deviceID,
		turnID,
		summary,
		history,
		time.Now().UTC(),
	)

	return s.ownerStore.WriteSnapshot(snapshot)
}

func (s *session) ensureOwnerStructuredFiles() error {
	if s.ownerStore == nil || s.ownerID == "" {
		return nil
	}
	if err := s.ownerStore.EnsureOwner(s.ownerID); err != nil {
		return err
	}
	if s.boundOwnerID != "" && s.deviceID != "" {
		return s.ownerStore.BindDevice(s.ownerID, s.deviceID, ownerBindingSourceExplicit)
	}
	return nil
}

func (s *session) bindDeviceOwner(ownerID string) error {
	ownerID = normalizeVoiceKeySegment(ownerID)
	if ownerID == "" {
		return nil
	}
	if s.deviceStore != nil && s.deviceID != "" {
		if err := s.deviceStore.BindOwner(s.deviceID, ownerID, s.connID); err != nil {
			return err
		}
	}
	if s.ownerStore != nil && s.deviceID != "" {
		if err := s.ownerStore.EnsureOwner(ownerID); err != nil {
			return err
		}
		if err := s.ownerStore.BindDevice(ownerID, s.deviceID, ownerBindingSourceExplicit); err != nil {
			return err
		}
	}
	return nil
}

func (s *session) loadBoundOwnerID() string {
	if s.deviceStore == nil || s.deviceID == "" {
		return ""
	}
	ownerID, err := s.deviceStore.ResolveOwner(s.deviceID)
	if err != nil {
		logger.WarnCF("xiaozhi", "Resolve device owner failed", map[string]any{
			"device_id": s.deviceID,
			"error":     err.Error(),
		})
		return ""
	}
	if ownerID != "" && s.ownerStore != nil {
		if err := s.ownerStore.EnsureOwner(ownerID); err != nil {
			logger.WarnCF("xiaozhi", "Ensure bound owner store failed", map[string]any{
				"owner_id": ownerID,
				"error":    err.Error(),
			})
		} else if s.deviceID != "" {
			if err := s.ownerStore.BindDevice(ownerID, s.deviceID, ownerBindingSourceExplicit); err != nil {
				logger.WarnCF("xiaozhi", "Mirror bound device into owner profile failed", map[string]any{
					"owner_id":  ownerID,
					"device_id": s.deviceID,
					"error":     err.Error(),
				})
			}
		}
	}
	return ownerID
}

func (s *session) touchBoundDevice() error {
	if s.deviceStore == nil || s.deviceID == "" {
		return nil
	}
	return s.deviceStore.Touch(s.deviceID, s.connID)
}

func (s *session) prepareOwnerPendingAnnouncement() pendingAnnouncementBatch {
	if s.pendingStore == nil || s.ownerID == "" {
		return pendingAnnouncementBatch{}
	}

	prepared, err := s.pendingStore.PrepareFirstDailySummary(s.ownerID, 3, 56)
	if err != nil {
		logger.WarnCF("xiaozhi", "Consume voice pending failed", map[string]any{
			"owner_id": s.ownerID,
			"error":    err.Error(),
		})
		return pendingAnnouncementBatch{}
	}
	if len(prepared.Items) == 0 || prepared.Summary == "" {
		return pendingAnnouncementBatch{}
	}

	logger.InfoCF("xiaozhi", "Voice pending summary loaded", map[string]any{
		"owner_id": s.ownerID,
		"count":    len(prepared.Items),
	})
	return pendingAnnouncementBatch{summary: prepared.Summary, items: prepared.Items}
}

func (s *session) confirmOwnerPendingAnnouncement(batch pendingAnnouncementBatch) error {
	if s.pendingStore == nil || s.ownerID == "" || len(batch.items) == 0 {
		return nil
	}

	itemIDs := make([]string, 0, len(batch.items))
	for _, item := range batch.items {
		if item.ID != "" {
			itemIDs = append(itemIDs, item.ID)
		}
	}
	return s.pendingStore.ConfirmFirstDailySummary(s.ownerID, itemIDs)
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
	err := s.conn.WriteMessage(websocket.TextMessage, data)
	if err != nil {
		logger.ErrorCF("xiaozhi", "Send text frame failed", map[string]any{
			"payload": summarizeOutboundTextFrame(data),
			"error":   err.Error(),
		})
	}
	return err
}

func (s *session) writeBinary(data []byte) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	err := s.conn.WriteMessage(websocket.BinaryMessage, data)
	if err != nil {
		logger.ErrorCF("xiaozhi", "Send binary frame failed", map[string]any{
			"size":  len(data),
			"error": err.Error(),
		})
	}
	return err
}

func summarizeOutboundTextFrame(data []byte) string {
	var payload struct {
		Type  string `json:"type"`
		State string `json:"state,omitempty"`
		Text  string `json:"text,omitempty"`
	}
	if err := json.Unmarshal(data, &payload); err == nil {
		text := strings.TrimSpace(payload.Text)
		if len([]rune(text)) > 48 {
			text = string([]rune(text)[:48]) + "..."
		}
		summary := payload.Type
		if payload.State != "" {
			summary += ":" + payload.State
		}
		if text != "" {
			summary += " text=" + text
		}
		if summary != "" {
			return summary
		}
	}
	raw := strings.TrimSpace(string(data))
	if len([]rune(raw)) > 96 {
		return string([]rune(raw)[:96]) + "..."
	}
	return raw
}
