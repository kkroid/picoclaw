package xiaozhi

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/sipeed/picoclaw/pkg/asr"
	"github.com/sipeed/picoclaw/pkg/logger"
)

// processSpeech: ASR -> LLM -> 文本/语音下行流水线。
// turnID 和 sessionKey 由调用方快照传入，避免与主 goroutine 的写入竞争。
func (s *session) processSpeech(frames [][]byte, turnID, sessionKey string, outputMode turnOutputMode) {
	reqStart := time.Now()
	logger.Infof("xiaozhi: processSpeech: frames=%d", len(frames))

	ctx, cancel := context.WithCancel(s.connCtx)
	s.setTurnCancel(cancel)
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
	s.setTurnCancel(cancel)
	defer cancel()

	if turnID == "" {
		turnID = s.connID
	}
	if sessionKey == "" {
		sessionKey = buildVoiceSessionKey(s.ownerID, s.deviceID, s.connID, s.sessionScope)
	}
	s.runPipeline(ctx, text, reqStart, turnID, sessionKey, outputMode)
}

// runPipeline 执行 LLM -> 文本/语音下行流水线。
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
				return
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
