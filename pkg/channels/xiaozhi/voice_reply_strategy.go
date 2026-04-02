package xiaozhi

import (
	"strings"
	"unicode/utf8"
)

const (
	voiceReplyMaxRunesForSpeech               = 96
	voiceReplyMaxSentencesForSpeech           = 3
	voiceReplyMaxSingleSentenceRunesForSpeech = 48
	voiceReplySummaryMaxRunesForSpeech        = 72
	voiceReplySummaryMaxSentencesForSpeech    = 2
	voiceReplyStructuredMaxTitlesForSpeech    = 5
)

type voiceReplySpeechPlan struct {
	mode       turnOutputMode
	items      []string
	condensed  bool
	skipReason string
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
