package xiaozhi

import (
	"encoding/json"
	"testing"

	"github.com/sipeed/picoclaw/pkg/asr"
	"github.com/sipeed/picoclaw/pkg/tts"
)

func TestHelloReply_EncodesASRAndTTSContainerAndCodec(t *testing.T) {
	raw := helloReply(
		"conn-1",
		"kkroid",
		asr.AudioFormat{Format: "ogg", Codec: "opus", SampleRate: 16000, Channels: 1},
		tts.AudioFormat{Format: "ogg", Codec: "opus", SampleRate: 16000, Channels: 1},
	)

	var msg helloReplyMsg
	if err := json.Unmarshal(raw, &msg); err != nil {
		t.Fatalf("Unmarshal hello reply: %v", err)
	}
	if msg.AsrParams.Format != "ogg" {
		t.Fatalf("asr_params.format = %q, want ogg", msg.AsrParams.Format)
	}
	if msg.AsrParams.Codec != "opus" {
		t.Fatalf("asr_params.codec = %q, want opus", msg.AsrParams.Codec)
	}
	if msg.TTSParams.Format != "ogg" {
		t.Fatalf("tts_params.format = %q, want ogg", msg.TTSParams.Format)
	}
	if msg.TTSParams.Codec != "opus" {
		t.Fatalf("tts_params.codec = %q, want opus", msg.TTSParams.Codec)
	}
}
