package doubao

import (
	"encoding/binary"
	"testing"

	"github.com/sipeed/picoclaw/pkg/tts"
)

func TestProviderAudioFormat_DefaultsToOggOpus(t *testing.T) {
	p := &provider{}
	got := p.AudioFormat()
	want := tts.AudioFormat{Format: "ogg", Codec: "opus", SampleRate: 16000, Channels: 1}
	if got != want {
		t.Fatalf("AudioFormat() = %#v, want %#v", got, want)
	}
}

func TestParseTTSFrame_ReturnsOggPayloadUnmodified(t *testing.T) {
	payload := []byte("OggS\x00\x02test-opus-page")
	frame := []byte{
		0x11,
		msgTypeServerAudio << 4,
		0x00,
		0x00,
	}
	frame = binary.BigEndian.AppendUint32(frame, 1)
	frame = binary.BigEndian.AppendUint32(frame, uint32(len(payload)))
	frame = append(frame, payload...)

	got, isLast, err := parseTTSFrame(0, frame)
	if err != nil {
		t.Fatalf("parseTTSFrame() error = %v", err)
	}
	if isLast {
		t.Fatal("parseTTSFrame() marked non-final audio frame as last")
	}
	if string(got) != string(payload) {
		t.Fatalf("parseTTSFrame() payload = %q, want %q", got, payload)
	}
}
