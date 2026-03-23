package xiaozhi

import "strings"

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
