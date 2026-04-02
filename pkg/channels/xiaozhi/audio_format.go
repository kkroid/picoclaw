package xiaozhi

import (
	"fmt"
	"strings"

	"github.com/sipeed/picoclaw/pkg/asr"
	"github.com/sipeed/picoclaw/pkg/logger"
)

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
