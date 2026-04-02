package xiaozhi

import (
	"encoding/json"
	"strings"

	"github.com/gorilla/websocket"

	"github.com/sipeed/picoclaw/pkg/logger"
)

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
