package xiaozhi

import (
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/sipeed/picoclaw/pkg/logger"
	"github.com/sipeed/picoclaw/pkg/memory"
)

type pendingAnnouncementBatch struct {
	summary string
	items   []memory.VoicePendingItem
}

type voiceSessionContext struct {
	OwnerID           string
	DeviceID          string
	ConnectionID      string
	TurnID            string
	RequestedMemoryID string
	SessionKey        string
	OwnerMemoryKey    string
	SessionScope      string
}

func resolveVoiceOwnerID(boundOwnerID, defaultOwnerID string) string {
	if ownerID := normalizeVoiceKeySegment(boundOwnerID); ownerID != "" {
		return ownerID
	}
	return normalizeVoiceKeySegment(defaultOwnerID)
}

func buildVoiceSessionContext(
	defaultOwnerID, sessionScope, deviceID, connectionID, turnID, requestedMemoryID string,
) voiceSessionContext {
	ownerID := normalizeVoiceKeySegment(defaultOwnerID)
	deviceID = normalizeVoiceKeySegment(deviceID)
	connectionID = normalizeVoiceKeySegment(connectionID)
	turnID = strings.TrimSpace(turnID)
	requestedMemoryID = strings.TrimSpace(requestedMemoryID)
	sessionScope = normalizeVoiceSessionScope(sessionScope)

	ownerMemoryKey := ""
	if ownerID != "" {
		ownerMemoryKey = buildOwnerMemoryKey(ownerID)
	}

	sessionKey := requestedMemoryID
	if sessionKey == "" {
		sessionKey = buildVoiceSessionKey(ownerID, deviceID, connectionID, sessionScope)
	}

	return voiceSessionContext{
		OwnerID:           ownerID,
		DeviceID:          deviceID,
		ConnectionID:      connectionID,
		TurnID:            turnID,
		RequestedMemoryID: requestedMemoryID,
		SessionKey:        sessionKey,
		OwnerMemoryKey:    ownerMemoryKey,
		SessionScope:      sessionScope,
	}
}

func normalizeVoiceSessionScope(scope string) string {
	scope = strings.ToLower(strings.TrimSpace(scope))
	switch scope {
	case "per-owner":
		return "per-owner"
	case "", "per-owner-device":
		return "per-owner-device"
	default:
		return "per-owner-device"
	}
}

func buildOwnerMemoryKey(ownerID string) string {
	ownerID = normalizeVoiceKeySegment(ownerID)
	if ownerID == "" {
		return ""
	}
	return "xiaozhi:owner:" + ownerID
}

func buildVoiceSessionKey(ownerID, deviceID, connectionID, sessionScope string) string {
	ownerID = normalizeVoiceKeySegment(ownerID)
	deviceID = normalizeVoiceKeySegment(deviceID)
	connectionID = normalizeVoiceKeySegment(connectionID)
	sessionScope = normalizeVoiceSessionScope(sessionScope)

	switch sessionScope {
	case "per-owner":
		if ownerID != "" {
			return buildOwnerMemoryKey(ownerID)
		}
	default:
		if ownerID != "" && deviceID != "" {
			return "xiaozhi:owner:" + ownerID + ":device:" + deviceID
		}
		if ownerID != "" {
			return buildOwnerMemoryKey(ownerID)
		}
	}

	if deviceID != "" {
		return "xiaozhi:device:" + deviceID
	}
	if connectionID != "" {
		return "xiaozhi:conn:" + connectionID
	}
	return "xiaozhi:conn:unknown"
}

func normalizeVoiceKeySegment(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return ""
	}
	replacer := strings.NewReplacer(
		" ", "-",
		"\t", "-",
		"\n", "-",
		"\r", "-",
		"/", "-",
		"\\", "-",
		":", "-",
		"*", "-",
		"?", "-",
		"\"", "-",
		"<", "-",
		">", "-",
		"|", "-",
	)
	return replacer.Replace(value)
}

func (s *session) loadFinalAssistantContent(sessionKey string) string {
	if s.runtime == nil {
		return ""
	}
	history := s.runtime.GetHistory(sessionKey)
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
	if s.stores.owner == nil || s.runtime == nil || s.ownerID == "" || s.ownerMemoryKey == "" {
		return nil
	}
	history := s.runtime.GetHistory(s.ownerMemoryKey)
	summary := s.runtime.GetSummary(s.ownerMemoryKey)
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

	return s.stores.owner.WriteSnapshot(snapshot)
}

func (s *session) ensureOwnerStructuredFiles() error {
	if s.stores.owner == nil || s.ownerID == "" {
		return nil
	}
	if err := s.stores.owner.EnsureOwner(s.ownerID); err != nil {
		return err
	}
	if s.boundOwnerID != "" && s.deviceID != "" {
		return s.stores.owner.BindDevice(s.ownerID, s.deviceID, ownerBindingSourceExplicit)
	}
	return nil
}

func (s *session) bindDeviceOwner(ownerID string) error {
	ownerID = normalizeVoiceKeySegment(ownerID)
	if ownerID == "" {
		return nil
	}
	if s.stores.device != nil && s.deviceID != "" {
		if err := s.stores.device.BindOwner(s.deviceID, ownerID, s.connID); err != nil {
			return err
		}
	}
	if s.stores.owner != nil && s.deviceID != "" {
		if err := s.stores.owner.EnsureOwner(ownerID); err != nil {
			return err
		}
		if err := s.stores.owner.BindDevice(ownerID, s.deviceID, ownerBindingSourceExplicit); err != nil {
			return err
		}
	}
	return nil
}

func (s *session) loadBoundOwnerID() string {
	if s.stores.device == nil || s.deviceID == "" {
		return ""
	}
	ownerID, err := s.stores.device.ResolveOwner(s.deviceID)
	if err != nil {
		logger.WarnCF("xiaozhi", "Resolve device owner failed", map[string]any{
			"device_id": s.deviceID,
			"error":     err.Error(),
		})
		return ""
	}
	if ownerID != "" && s.stores.owner != nil {
		if err := s.stores.owner.EnsureOwner(ownerID); err != nil {
			logger.WarnCF("xiaozhi", "Ensure bound owner store failed", map[string]any{
				"owner_id": ownerID,
				"error":    err.Error(),
			})
		} else if s.deviceID != "" {
			if err := s.stores.owner.BindDevice(ownerID, s.deviceID, ownerBindingSourceExplicit); err != nil {
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
	if s.stores.device == nil || s.deviceID == "" {
		return nil
	}
	return s.stores.device.Touch(s.deviceID, s.connID)
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
	s.cancelCurrentTurn()
	s.pipelineWg.Wait()

	// 清理旧 ASR session，避免文本 turn 复用到旧语音流状态。
	s.closeRealtimeASRSession()
}

func (s *session) prepareOwnerPendingAnnouncement() pendingAnnouncementBatch {
	if s.stores.pending == nil || s.ownerID == "" {
		return pendingAnnouncementBatch{}
	}

	prepared, err := s.stores.pending.PrepareFirstDailySummary(s.ownerID, 3, 56)
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
	if s.stores.pending == nil || s.ownerID == "" || len(batch.items) == 0 {
		return nil
	}

	itemIDs := make([]string, 0, len(batch.items))
	for _, item := range batch.items {
		if item.ID != "" {
			itemIDs = append(itemIDs, item.ID)
		}
	}
	return s.stores.pending.ConfirmFirstDailySummary(s.ownerID, itemIDs)
}
