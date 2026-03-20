package xiaozhi

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestDecideVoiceReplyOutputMode(t *testing.T) {
	if got := decideVoiceReplyOutputMode("好的，马上开始。", []string{"好的，马上开始。"}); got != turnOutputModeTextAndVoice {
		t.Fatalf("short reply mode = %q, want %q", got, turnOutputModeTextAndVoice)
	}

	structured := "1. 第一项\n2. 第二项\n3. 第三项\n4. 第四项"
	if got := decideVoiceReplyOutputMode(
		structured,
		[]string{"1. 第一项", "2. 第二项", "3. 第三项", "4. 第四项"},
	); got != turnOutputModeTextAndVoice {
		t.Fatalf("structured reply mode = %q, want %q", got, turnOutputModeTextAndVoice)
	}
	longReply := "我是 picoclaw，一个智能助手，专注于帮助您完成各种任务。我的功能包括信息查询、任务管理、文件处理和执行命令等。我可以提供股票公告、天气信息等内容，帮助您高效工作。如果您有需要的地方，请告诉我！"
	if got := decideVoiceReplyOutputMode(
		longReply,
		[]string{
			"我是 picoclaw，一个智能助手，专注于帮助您完成各种任务。",
			"我的功能包括信息查询、任务管理、文件处理和执行命令等。",
			"我可以提供股票公告、天气信息等内容，帮助您高效工作。",
			"如果您有需要的地方，请告诉我！",
		},
	); got != turnOutputModeTextAndVoice {
		t.Fatalf("long reply mode = %q, want %q", got, turnOutputModeTextAndVoice)
	}

	codeReply := "```go\nfmt.Println(\"hello\")\n```"
	if got := decideVoiceReplyOutputMode(codeReply, []string{codeReply}); got != turnOutputModeTextOnly {
		t.Fatalf("code reply mode = %q, want %q", got, turnOutputModeTextOnly)
	}
}

func TestBuildVoiceReplySpeechPlan_CondensesLongReply(t *testing.T) {
	finalContent := "我是 picoclaw，一个智能助手，专注于帮助您完成各种任务。我的功能包括信息查询、任务管理、文件处理和执行命令等。我可以提供股票公告、天气信息等内容，帮助您高效工作。如果您有需要的地方，请告诉我！"
	plan := buildVoiceReplySpeechPlan(finalContent, []string{
		"我是 picoclaw，一个智能助手，专注于帮助您完成各种任务。",
		"我的功能包括信息查询、任务管理、文件处理和执行命令等。",
		"我可以提供股票公告、天气信息等内容，帮助您高效工作。",
		"如果您有需要的地方，请告诉我！",
	})

	if plan.mode != turnOutputModeTextAndVoice {
		t.Fatalf("plan.mode = %q, want %q", plan.mode, turnOutputModeTextAndVoice)
	}
	if !plan.condensed {
		t.Fatal("expected long reply to be condensed for speech")
	}
	if len(plan.items) != 2 {
		t.Fatalf("len(plan.items) = %d, want 2", len(plan.items))
	}
	if plan.items[0] != "我是 picoclaw，一个智能助手，专注于帮助您完成各种任务。" {
		t.Fatalf("plan.items[0] = %q", plan.items[0])
	}
}

func TestBuildVoiceReplySpeechPlan_CondensesStructuredListReply(t *testing.T) {
	plan := buildVoiceReplySpeechPlan(
		"我可以做很多事情，包括：\n1. **信息查询**：获取新闻、天气、股票公告等信息。\n2. **任务管理**：设置提醒和管理日程。\n3. **文件处理**：读取、写入、编辑和发送文件。\n4. **执行命令**：在计算机上执行特定命令。",
		[]string{
			"我可以做很多事情，包括：",
			"1. **信息查询**：获取新闻、天气、股票公告等信息。",
			"2. **任务管理**：设置提醒和管理日程。",
			"3. **文件处理**：读取、写入、编辑和发送文件。",
			"4. **执行命令**：在计算机上执行特定命令。",
		},
	)

	if plan.mode != turnOutputModeTextAndVoice {
		t.Fatalf("plan.mode = %q, want %q", plan.mode, turnOutputModeTextAndVoice)
	}
	if !plan.condensed {
		t.Fatal("expected structured list reply to be condensed for speech")
	}
	if len(plan.items) != 2 {
		t.Fatalf("len(plan.items) = %d, want 2", len(plan.items))
	}
	if plan.items[0] != "我可以做很多事情，包括信息查询、任务管理和文件处理。" {
		t.Fatalf("plan.items[0] = %q", plan.items[0])
	}
	if plan.items[1] != "也能执行命令。" {
		t.Fatalf("plan.items[1] = %q", plan.items[1])
	}
}

func TestSessionRunPipeline_VoiceTurnSpeaksShortReply(t *testing.T) {
	workspace, err := os.MkdirTemp("", "xiaozhi-voice-short-*")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	defer os.RemoveAll(workspace)

	provider := &textTurnStreamingProvider{
		response:     "好的，马上开始。",
		streamChunks: []string{"好的，马上开始。"},
	}
	ttsProvider := &stubTTSProvider{}
	al := newTestAgentLoop(t, workspace, provider)
	serverConn, clientConn, cleanup := newTestWebsocketPair(t)
	defer cleanup()

	sess := newSession(
		serverConn,
		&noopASRProvider{},
		ttsProvider,
		al,
		newOwnerStore(workspace),
		newVoiceDeviceStore(workspace),
		nil,
		"kkroid",
		"per-owner-device",
		newDeviceRegistry(),
	)

	go sess.runPipeline(
		context.Background(),
		"开始执行",
		time.Now(),
		"turn-voice-1",
		"xiaozhi:owner:kkroid:device:desk-1",
		turnOutputModeTextAndVoice,
	)
	messages, binaryFrames := collectConversationOutput(t, clientConn)

	var gotTTSStart bool
	var gotTTSStop bool
	var gotLLMText string
	for _, msg := range messages {
		switch msg.Type {
		case "llm":
			gotLLMText += msg.Text
		case "tts":
			if msg.State == "start" {
				gotTTSStart = true
			}
			if msg.State == "stop" {
				gotTTSStop = true
			}
		}
	}

	if gotLLMText != "好的，马上开始。" {
		t.Fatalf("gotLLMText = %q, want %q", gotLLMText, "好的，马上开始。")
	}
	if !gotTTSStart || !gotTTSStop {
		t.Fatalf("expected tts start/stop, got start=%v stop=%v", gotTTSStart, gotTTSStop)
	}
	if binaryFrames == 0 {
		t.Fatal("expected binary audio frames for short voice reply")
	}
	if len(ttsProvider.texts) == 0 || ttsProvider.texts[0] != "好的，马上开始。" {
		t.Fatalf("tts texts = %#v, want first item 好的，马上开始。", ttsProvider.texts)
	}
}

func TestSessionRunPipeline_VoiceTurnKeepsCodeReplyTextOnly(t *testing.T) {
	workspace, err := os.MkdirTemp("", "xiaozhi-voice-code-*")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	defer os.RemoveAll(workspace)

	structured := "```go\nfmt.Println(\"hello\")\n```"
	provider := &textTurnStreamingProvider{
		response:     structured,
		streamChunks: []string{structured},
	}
	ttsProvider := &stubTTSProvider{}
	al := newTestAgentLoop(t, workspace, provider)
	serverConn, clientConn, cleanup := newTestWebsocketPair(t)
	defer cleanup()

	sess := newSession(
		serverConn,
		&noopASRProvider{},
		ttsProvider,
		al,
		newOwnerStore(workspace),
		newVoiceDeviceStore(workspace),
		nil,
		"kkroid",
		"per-owner-device",
		newDeviceRegistry(),
	)

	go sess.runPipeline(
		context.Background(),
		"给我一个清单",
		time.Now(),
		"turn-voice-2",
		"xiaozhi:owner:kkroid:device:desk-2",
		turnOutputModeTextAndVoice,
	)
	messages, binaryFrames := collectConversationOutput(t, clientConn)

	var gotLLMStop bool
	var gotTTSStop bool
	var gotTTSStart bool
	var gotLLMText string
	for _, msg := range messages {
		switch msg.Type {
		case "llm":
			if msg.State == "stop" {
				gotLLMStop = true
				continue
			}
			gotLLMText += msg.Text
		case "tts":
			if msg.State == "start" {
				gotTTSStart = true
			}
			if msg.State == "stop" {
				gotTTSStop = true
			}
		}
	}

	if gotLLMText == "" {
		t.Fatal("expected llm text for code voice reply")
	}
	if !gotLLMStop {
		t.Fatal("expected llm stop for text-only voice reply")
	}
	if gotTTSStart {
		t.Fatal("text-only voice reply should not emit tts.start")
	}
	if !gotTTSStop {
		t.Fatal("expected compatibility tts.stop for text-only voice reply")
	}
	if binaryFrames != 0 {
		t.Fatalf("binaryFrames = %d, want 0", binaryFrames)
	}
	if len(ttsProvider.texts) != 0 {
		t.Fatalf("tts texts = %#v, want empty", ttsProvider.texts)
	}
}

func TestSessionRunPipeline_VoiceTurnCondensesStructuredListForSpeech(t *testing.T) {
	workspace, err := os.MkdirTemp("", "xiaozhi-voice-structured-list-*")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	defer os.RemoveAll(workspace)

	response := "我可以做很多事情，包括：1. **信息查询**：获取新闻、天气、股票公告等信息。2. **任务管理**：设置提醒和管理日程。3. **文件处理**：读取、写入、编辑和发送文件。4. **执行命令**：在计算机上执行特定命令。5. **扩展技能**：使用特定技能来完成复杂任务。"
	provider := &textTurnStreamingProvider{
		response: response,
		streamChunks: []string{
			"我可以做很多事情，包括：",
			"1. **信息查询**：获取新闻、天气、股票公告等信息。",
			"2. **任务管理**：设置提醒和管理日程。",
			"3. **文件处理**：读取、写入、编辑和发送文件。",
			"4. **执行命令**：在计算机上执行特定命令。",
			"5. **扩展技能**：使用特定技能来完成复杂任务。",
		},
	}
	ttsProvider := &stubTTSProvider{}
	al := newTestAgentLoop(t, workspace, provider)
	serverConn, clientConn, cleanup := newTestWebsocketPair(t)
	defer cleanup()

	sess := newSession(
		serverConn,
		&noopASRProvider{},
		ttsProvider,
		al,
		newOwnerStore(workspace),
		newVoiceDeviceStore(workspace),
		nil,
		"kkroid",
		"per-owner-device",
		newDeviceRegistry(),
	)

	go sess.runPipeline(
		context.Background(),
		"你都会些什么",
		time.Now(),
		"turn-voice-structured-list-1",
		"xiaozhi:owner:kkroid:device:desk-5",
		turnOutputModeTextAndVoice,
	)
	messages, binaryFrames := collectConversationOutput(t, clientConn)

	var gotLLMStop bool
	var gotTTSStart bool
	var gotTTSStop bool
	var gotLLMText string
	for _, msg := range messages {
		switch msg.Type {
		case "llm":
			if msg.State == "stop" {
				gotLLMStop = true
				continue
			}
			gotLLMText += msg.Text
		case "tts":
			if msg.State == "start" {
				gotTTSStart = true
			}
			if msg.State == "stop" {
				gotTTSStop = true
			}
		}
	}

	if gotLLMText != response {
		t.Fatalf("gotLLMText = %q, want %q", gotLLMText, response)
	}
	if !gotLLMStop {
		t.Fatal("expected llm stop before structured speech playback")
	}
	if !gotTTSStart || !gotTTSStop {
		t.Fatalf("expected tts start/stop, got start=%v stop=%v", gotTTSStart, gotTTSStop)
	}
	if binaryFrames == 0 {
		t.Fatal("expected binary audio frames for structured list voice reply")
	}
	if len(ttsProvider.texts) != 2 {
		t.Fatalf("tts texts count = %d, want 2", len(ttsProvider.texts))
	}
	if ttsProvider.texts[0] != "我可以做很多事情，包括信息查询、任务管理和文件处理。" {
		t.Fatalf("tts texts[0] = %q", ttsProvider.texts[0])
	}
	if ttsProvider.texts[1] != "也能执行命令和扩展技能。" {
		t.Fatalf("tts texts[1] = %q", ttsProvider.texts[1])
	}
}

func TestSessionRunPipeline_VoiceTurnCondensesLongReplyForSpeech(t *testing.T) {
	workspace, err := os.MkdirTemp("", "xiaozhi-voice-condensed-*")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	defer os.RemoveAll(workspace)

	response := "我是 picoclaw，一个智能助手，专注于帮助您完成各种任务。我的功能包括信息查询、任务管理、文件处理和执行命令等。我可以提供股票公告、天气信息等内容，帮助您高效工作。如果您有需要的地方，请告诉我！"
	provider := &textTurnStreamingProvider{
		response: response,
		streamChunks: []string{
			"我是 picoclaw，一个智能助手，专注于帮助您完成各种任务。",
			"我的功能包括信息查询、任务管理、文件处理和执行命令等。",
			"我可以提供股票公告、天气信息等内容，帮助您高效工作。",
			"如果您有需要的地方，请告诉我！",
		},
	}
	ttsProvider := &stubTTSProvider{}
	al := newTestAgentLoop(t, workspace, provider)
	serverConn, clientConn, cleanup := newTestWebsocketPair(t)
	defer cleanup()

	sess := newSession(
		serverConn,
		&noopASRProvider{},
		ttsProvider,
		al,
		newOwnerStore(workspace),
		newVoiceDeviceStore(workspace),
		nil,
		"kkroid",
		"per-owner-device",
		newDeviceRegistry(),
	)

	go sess.runPipeline(
		context.Background(),
		"介绍一下你自己",
		time.Now(),
		"turn-voice-condensed-1",
		"xiaozhi:owner:kkroid:device:desk-4",
		turnOutputModeTextAndVoice,
	)
	messages, binaryFrames := collectConversationOutput(t, clientConn)

	var gotTTSStart bool
	var gotTTSStop bool
	var gotLLMText string
	for _, msg := range messages {
		switch msg.Type {
		case "llm":
			gotLLMText += msg.Text
		case "tts":
			if msg.State == "start" {
				gotTTSStart = true
			}
			if msg.State == "stop" {
				gotTTSStop = true
			}
		}
	}

	if gotLLMText != response {
		t.Fatalf("gotLLMText = %q, want %q", gotLLMText, response)
	}
	if !gotTTSStart || !gotTTSStop {
		t.Fatalf("expected tts start/stop, got start=%v stop=%v", gotTTSStart, gotTTSStop)
	}
	if binaryFrames == 0 {
		t.Fatal("expected binary audio frames for condensed voice reply")
	}
	if len(ttsProvider.texts) != 2 {
		t.Fatalf("tts texts count = %d, want 2", len(ttsProvider.texts))
	}
	if ttsProvider.texts[0] != "我是 picoclaw，一个智能助手，专注于帮助您完成各种任务。" {
		t.Fatalf("tts texts[0] = %q", ttsProvider.texts[0])
	}
	if ttsProvider.texts[1] != "我的功能包括信息查询、任务管理、文件处理和执行命令等。" {
		t.Fatalf("tts texts[1] = %q", ttsProvider.texts[1])
	}
}

func TestSessionRunPipeline_VoiceTurnFallsBackToNonStreamingProvider(t *testing.T) {
	workspace, err := os.MkdirTemp("", "xiaozhi-voice-fallback-*")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	defer os.RemoveAll(workspace)

	provider := &textTurnNonStreamingProvider{response: "非流式模型也应该播报。"}
	ttsProvider := &stubTTSProvider{}
	al := newTestAgentLoop(t, workspace, provider)
	serverConn, clientConn, cleanup := newTestWebsocketPair(t)
	defer cleanup()

	sess := newSession(
		serverConn,
		&noopASRProvider{},
		ttsProvider,
		al,
		newOwnerStore(workspace),
		newVoiceDeviceStore(workspace),
		nil,
		"kkroid",
		"per-owner-device",
		newDeviceRegistry(),
	)

	go sess.runPipeline(
		context.Background(),
		"开始执行",
		time.Now(),
		"turn-voice-fallback-1",
		"xiaozhi:owner:kkroid:device:desk-3",
		turnOutputModeTextAndVoice,
	)
	messages, binaryFrames := collectConversationOutput(t, clientConn)

	var gotTTSStart bool
	var gotTTSStop bool
	var gotLLMText string
	for _, msg := range messages {
		switch msg.Type {
		case "llm":
			gotLLMText += msg.Text
		case "tts":
			if msg.State == "start" {
				gotTTSStart = true
			}
			if msg.State == "stop" {
				gotTTSStop = true
			}
		}
	}

	if gotLLMText != "非流式模型也应该播报。" {
		t.Fatalf("gotLLMText = %q, want %q", gotLLMText, "非流式模型也应该播报。")
	}
	if !gotTTSStart || !gotTTSStop {
		t.Fatalf("expected tts start/stop, got start=%v stop=%v", gotTTSStart, gotTTSStop)
	}
	if binaryFrames == 0 {
		t.Fatal("expected binary audio frames for non-streaming provider voice reply")
	}
	if len(ttsProvider.texts) == 0 || ttsProvider.texts[0] != "非流式模型也应该播报。" {
		t.Fatalf("tts texts = %#v, want first item 非流式模型也应该播报。", ttsProvider.texts)
	}
}
