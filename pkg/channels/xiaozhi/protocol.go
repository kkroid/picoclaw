package xiaozhi

// protocol.go 定义 xiaozhi 语音通道与客户端之间的 WebSocket 消息协议。
//
// 基础协议：xiaozhi（https://github.com/78/xiaozhi-esp32）
// 扩展协议：picoclaw 在 xiaozhi 基础上新增若干字段和消息类型，统一标注为 [picoclaw 扩展]。
//
// ---- 客户端 → 服务端 ----
//
//   hello   握手，见 helloMsg
//   listen  VAD 控制，见 listenMsg；state = "start" | "end" | "stop"
//   text    文本输入，见 textMsg [picoclaw 扩展]
//   abort   打断当前播放，见 abortMsg
//   ping    心跳（当前实现忽略，不回 pong）
//
// ---- 服务端 → 客户端 ----
//
//   hello   握手响应，见 helloReplyMsg
//   stt     语音识别结果，见 sttMsg
//   tts     语音合成控制，见 ttsMsg；sentence_start/sentence_end 之间是对应句子的二进制音频帧，格式由 hello.tts_params 决定
//
// [picoclaw 扩展] 服务端 → 客户端：
//
//   llm     LLM 推理通知，见 llmMsg
//
// ---- picoclaw 扩展字段一览 ----
//
// 客户端 → 服务端：
//
//   hello.owner_id    string
//       显式设备 owner 绑定。首次提供后会把 device_id 持久化绑定到该 owner；后续同设备
//       即使不再传该字段，也会优先复用已绑定 owner，再回退 default_owner_id。
//
//   listen.memory_id   string
//       显式 session key 覆盖值。用于在默认 owner / device 解析之外，手动指定
//       一段共享对话上下文；不建议再将 device_id 直接当作长期记忆主键。
//
//   text.session_id / text.memory_id
//       语义与 listen.session_id / listen.memory_id 一致，但用于文本输入轮次。
//
// 服务端 → 客户端：
//
//   hello.session_id   string
//       连接级 ID，由服务端在每次 WebSocket 握手时生成（UUID）。
//       设备重连时刷新；与客户端 listen.session_id（turn_id，轮次标识）语义不同。
//       主要用途：服务端日志关联，客户端无需持久化。
//
//   hello.owner_id     string
//       服务端为当前连接解析出的有效 owner。优先级：hello.owner_id 显式绑定 > 设备已绑定 owner
//       > channels.xiaozhi.default_owner_id。
//
//   llm（新增消息类型）
//       LLM 推理过程通知，xiaozhi 标准协议中无此消息类型，见 llmMsg。
//       三种形态：
//         {"type":"llm","text":"..."}
//             LLM 断句后的回复文字。文本 turn 仅依赖 llm 文本下行；语音 turn 中它比对应的
//             tts.sentence_start 早约 200-500ms 发出，可用于在音频播放前在显示屏上呈现打字机效果。
//         {"type":"llm","state":"thinking_start"}
//             检测到 <think> 标记，模型进入思考阶段。
//         {"type":"llm","state":"thinking_end","duration_ms":N}
//             检测到 </think> 标记，思考结束，duration_ms 为本次 thinking 耗时（毫秒）。
//         {"type":"llm","state":"stop"}
//             当前文本输出轮次结束。用于 text 输入不再下发 tts.stop 时的客户端收尾信号。

import (
	"encoding/json"

	"github.com/sipeed/picoclaw/pkg/asr"
	"github.com/sipeed/picoclaw/pkg/tts"
)

// ---- 客户端 → 服务端 ----

// helloMsg 是客户端握手消息。picoclaw 协议仅使用 device_id 作为设备标识。
type helloMsg struct {
	Type      string `json:"type"`
	Version   int    `json:"version"`
	Transport string `json:"transport"`
	DeviceID  string `json:"device_id,omitempty"`
	OwnerID   string `json:"owner_id,omitempty"`
}

// listenMsg 是客户端语音控制消息。
// SessionID（session_id）由客户端在每轮问答开始时生成，服务端用作 turn_id，贯穿整轮 ASR→LLM→TTS。
// MemoryID（memory_id）是 picoclaw 协议扩展字段：显式覆盖本轮语音 session key。
type listenMsg struct {
	Type      string `json:"type"`
	State     string `json:"state"` // "start" | "end" | "stop"
	Mode      string `json:"mode,omitempty"`
	SessionID string `json:"session_id,omitempty"`
	MemoryID  string `json:"memory_id,omitempty"` // [picoclaw 扩展] 显式 session key 覆盖
}

// textMsg 是客户端文本输入消息。
// SessionID / MemoryID 语义与 listenMsg 一致：前者标识本轮 turn，后者显式覆盖会话 key。
type textMsg struct {
	Type      string `json:"type"`
	Text      string `json:"text"`
	SessionID string `json:"session_id,omitempty"`
	MemoryID  string `json:"memory_id,omitempty"`
}

type abortMsg struct {
	Type   string `json:"type"`
	Reason string `json:"reason,omitempty"`
}

// ---- 服务端 → 客户端 ----

// helloReplyAudioParams 是服务端 hello 响应中的 asr_params / tts_params 字段。
// frame_duration=60 为 xiaozhi 兼容固定值，用于设备端播放节拍控制。
type helloReplyAudioParams struct {
	Format        string `json:"format"`
	Codec         string `json:"codec,omitempty"`
	SampleRate    int    `json:"sample_rate"`
	Channels      int    `json:"channels"`
	FrameDuration int    `json:"frame_duration"`
}

// helloReplyMsg 是服务端握手响应。
// [picoclaw 扩展] session_id 承载服务端生成的连接级 ID，在设备重连时刷新；
// 与客户端 listen.session_id（turn_id）是不同语义的字段。
//
// asr_params：上行音频格式（客户端 → 服务端，用于 ASR），由 ASR provider 声明。
// tts_params：下行音频格式（服务端 → 客户端，用于 TTS 播放），由 TTS provider 声明。
type helloReplyMsg struct {
	Type      string                `json:"type"`
	Version   int                   `json:"version"`
	Transport string                `json:"transport"`
	SessionID string                `json:"session_id"`
	OwnerID   string                `json:"owner_id,omitempty"`
	AsrParams helloReplyAudioParams `json:"asr_params"` // 上行：ASR 期望格式
	TTSParams helloReplyAudioParams `json:"tts_params"` // 下行：TTS 输出格式
}

// sttMsg 是 ASR 识别结果通知。
type sttMsg struct {
	Type  string `json:"type"`
	Text  string `json:"text"`
	State string `json:"state"` // "recognizing"（中间结果）| "stop"（最终结果）
}

// ttsMsg 驱动客户端播放状态机。
type ttsMsg struct {
	Type  string `json:"type"`
	State string `json:"state"`          // "start" | "sentence_start" | "sentence_end" | "stop" | "abort"
	Text  string `json:"text,omitempty"` // 仅 sentence_start 携带
}

// llmMsg 是 picoclaw 扩展的 LLM 推理通知，xiaozhi 标准协议无此类型。
// State="thinking_start"：检测到 <think>；State="thinking_end"：检测到 </think>，携带 DurationMs；
// State="stop"：当前文本输出轮次结束。
// Text 字段：LLM 断句后的回复文字；在语音 turn 中通常早于对应 tts.sentence_start 发出。
type llmMsg struct {
	Type       string `json:"type"`
	Text       string `json:"text,omitempty"`
	State      string `json:"state,omitempty"`
	DurationMs int64  `json:"duration_ms,omitempty"`
}

// ---- 构造函数 ----

func newStt(text, state string) []byte {
	b, _ := json.Marshal(sttMsg{Type: "stt", Text: text, State: state})
	return b
}

func newTts(state, text string) []byte {
	b, _ := json.Marshal(ttsMsg{Type: "tts", State: state, Text: text})
	return b
}

func newLlmText(text string) []byte {
	b, _ := json.Marshal(llmMsg{Type: "llm", Text: text})
	return b
}

func newLlmThinkingStart() []byte {
	b, _ := json.Marshal(llmMsg{Type: "llm", State: "thinking_start"})
	return b
}

func newLlmThinkingEnd(ms int64) []byte {
	b, _ := json.Marshal(llmMsg{Type: "llm", State: "thinking_end", DurationMs: ms})
	return b
}

func newLlmStop() []byte {
	b, _ := json.Marshal(llmMsg{Type: "llm", State: "stop"})
	return b
}

// helloReply 构建服务端 hello 响应。
// asrFmt：ASR provider 声明的上行格式，客户端必须按此格式发送音频。
// ttsFmt：TTS provider 声明的下行格式，客户端按此初始化播放器/解封装器/解码器。
func helloReply(sessID, ownerID string, asrFmt asr.AudioFormat, ttsFmt tts.AudioFormat) []byte {
	b, _ := json.Marshal(helloReplyMsg{
		Type:      "hello",
		Version:   3,
		Transport: "websocket",
		SessionID: sessID,
		OwnerID:   normalizeVoiceKeySegment(ownerID),
		AsrParams: helloReplyAudioParams{
			Format:        asrFmt.Format,
			Codec:         asrFmt.Codec,
			SampleRate:    asrFmt.SampleRate,
			Channels:      asrFmt.Channels,
			FrameDuration: 60,
		},
		TTSParams: helloReplyAudioParams{
			Format:        ttsFmt.Format,
			Codec:         ttsFmt.Codec,
			SampleRate:    ttsFmt.SampleRate,
			Channels:      ttsFmt.Channels,
			FrameDuration: 60,
		},
	})
	return b
}
