package xiaozhi

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/sipeed/picoclaw/pkg/logger"
)

type llmStreamResult struct {
	finalContent string
	sentences    []string
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

	if s.runtime == nil {
		return result, fmt.Errorf("xiaozhi: runtime is not configured")
	}

	finalContent, err := s.runtime.RunVoiceTurn(
		llmCtx, sessionKey, s.ownerMemoryKey,
		userText, "xiaozhi", turnID, onToken,
	)
	if err != nil {
		return result, err
	}
	result.finalContent = finalContent
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
	if len(result.sentences) == 0 {
		for _, sentence := range splitVoiceReplyFallbackSentences(result.finalContent) {
			result.sentences = append(result.sentences, sentence)
			logger.Infof("xiaozhi: llm sentence: %q", sentence)
			s.writeText(newLlmText(sentence))
		}
	}
	if result.finalContent == "" && len(result.sentences) > 0 {
		result.finalContent = strings.Join(result.sentences, "")
	}

	logger.Infof("xiaozhi: llm done: turn=%s latency=%dms", turnID, time.Since(llmStart).Milliseconds())
	return result, nil
}

func splitVoiceReplyFallbackSentences(text string) []string {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	if structured := splitVoiceStructuredFallbackItems(text); len(structured) > 0 {
		return structured
	}
	return splitVoicePunctuationSentences(text)
}

func splitVoiceStructuredFallbackItems(text string) []string {
	starts := make([]int, 0, 4)
	for i := 0; i < len(text); i++ {
		if text[i] < '0' || text[i] > '9' {
			continue
		}
		if i > 0 {
			prefix := text[:i]
			if !strings.HasSuffix(prefix, " ") &&
				!strings.HasSuffix(prefix, "\t") &&
				!strings.HasSuffix(prefix, "\n") &&
				!strings.HasSuffix(prefix, ":") &&
				!strings.HasSuffix(prefix, "：") &&
				!strings.HasSuffix(prefix, "。") &&
				!strings.HasSuffix(prefix, "！") &&
				!strings.HasSuffix(prefix, "？") {
				continue
			}
		}
		j := i
		for j < len(text) && text[j] >= '0' && text[j] <= '9' {
			j++
		}
		if j >= len(text) {
			break
		}
		if text[j] != '.' && !strings.HasPrefix(text[j:], "、") && text[j] != ')' && !strings.HasPrefix(text[j:], "）") {
			continue
		}
		starts = append(starts, i)
		i = j
	}
	if len(starts) == 0 {
		return nil
	}
	items := make([]string, 0, len(starts)+1)
	if intro := strings.TrimSpace(text[:starts[0]]); intro != "" {
		items = append(items, intro)
	}
	for idx, start := range starts {
		end := len(text)
		if idx+1 < len(starts) {
			end = starts[idx+1]
		}
		if item := strings.TrimSpace(text[start:end]); item != "" {
			items = append(items, item)
		}
	}
	return items
}

func splitVoicePunctuationSentences(text string) []string {
	items := make([]string, 0, 4)
	start := 0
	for i, r := range text {
		if !strings.ContainsRune("。！？\n", r) {
			continue
		}
		end := i + utf8.RuneLen(r)
		if sentence := strings.TrimSpace(text[start:end]); sentence != "" {
			items = append(items, sentence)
		}
		start = end
	}
	if tail := strings.TrimSpace(text[start:]); tail != "" {
		items = append(items, tail)
	}
	if len(items) == 0 {
		return []string{text}
	}
	return items
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
