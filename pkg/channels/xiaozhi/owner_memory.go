package xiaozhi

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/sipeed/picoclaw/pkg/fileutil"
	"github.com/sipeed/picoclaw/pkg/providers"
)

const (
	ownerProfileFilename       = "profile.json"
	ownerSubscriptionsFilename = "subscriptions.json"
	ownerRemindersFilename     = "reminders.json"
	ownerWatchlistFilename     = "watchlist.json"
	ownerVoiceContextFilename  = "voice_context.json"

	ownerBindingSourceExplicit = "explicit"
)

type ownerVoiceTurn struct {
	Role      string    `json:"role"`
	Content   string    `json:"content"`
	Timestamp time.Time `json:"timestamp"`
}

type ownerVoiceSnapshot struct {
	OwnerID       string           `json:"owner_id"`
	MemoryKey     string           `json:"memory_key,omitempty"`
	SessionKey    string           `json:"session_key,omitempty"`
	Channel       string           `json:"channel,omitempty"`
	DeviceID      string           `json:"device_id,omitempty"`
	LastTurnID    string           `json:"last_turn_id,omitempty"`
	Summary       string           `json:"summary,omitempty"`
	RecentTurns   []ownerVoiceTurn `json:"recent_turns,omitempty"`
	LastUpdatedAt time.Time        `json:"last_updated_at"`
}

type ownerBoundDevice struct {
	DeviceID      string    `json:"device_id"`
	BindingSource string    `json:"binding_source,omitempty"`
	BoundAt       time.Time `json:"bound_at,omitempty"`
	LastSeenAt    time.Time `json:"last_seen_at,omitempty"`
}

type ownerProfile struct {
	OwnerID           string             `json:"owner_id"`
	BoundDevices      []ownerBoundDevice `json:"bound_devices,omitempty"`
	LastVoiceDeviceID string             `json:"last_voice_device_id,omitempty"`
	LastVoiceChannel  string             `json:"last_voice_channel,omitempty"`
	LastVoiceTurnID   string             `json:"last_voice_turn_id,omitempty"`
	LastVoiceAt       time.Time          `json:"last_voice_at,omitempty"`
	UpdatedAt         time.Time          `json:"updated_at"`
}

type ownerSubscription struct {
	ID        string    `json:"id,omitempty"`
	Kind      string    `json:"kind,omitempty"`
	Target    string    `json:"target,omitempty"`
	Channel   string    `json:"channel,omitempty"`
	ChatID    string    `json:"chat_id,omitempty"`
	Enabled   bool      `json:"enabled"`
	CreatedAt time.Time `json:"created_at,omitempty"`
	UpdatedAt time.Time `json:"updated_at,omitempty"`
}

type ownerReminder struct {
	ID        string    `json:"id,omitempty"`
	Title     string    `json:"title,omitempty"`
	Schedule  string    `json:"schedule,omitempty"`
	Channel   string    `json:"channel,omitempty"`
	ChatID    string    `json:"chat_id,omitempty"`
	Enabled   bool      `json:"enabled"`
	CreatedAt time.Time `json:"created_at,omitempty"`
	UpdatedAt time.Time `json:"updated_at,omitempty"`
}

type ownerWatchItem struct {
	ID        string    `json:"id,omitempty"`
	Kind      string    `json:"kind,omitempty"`
	Target    string    `json:"target,omitempty"`
	Note      string    `json:"note,omitempty"`
	Enabled   bool      `json:"enabled"`
	CreatedAt time.Time `json:"created_at,omitempty"`
	UpdatedAt time.Time `json:"updated_at,omitempty"`
}

type ownerSubscriptions struct {
	OwnerID   string              `json:"owner_id"`
	Items     []ownerSubscription `json:"items,omitempty"`
	UpdatedAt time.Time           `json:"updated_at"`
}

type ownerReminders struct {
	OwnerID   string          `json:"owner_id"`
	Items     []ownerReminder `json:"items,omitempty"`
	UpdatedAt time.Time       `json:"updated_at"`
}

type ownerWatchlist struct {
	OwnerID   string           `json:"owner_id"`
	Items     []ownerWatchItem `json:"items,omitempty"`
	UpdatedAt time.Time        `json:"updated_at"`
}

type ownerStore struct {
	workspace string
	now       func() time.Time
	mu        sync.Mutex
}

func newOwnerStore(workspace string) *ownerStore {
	workspace = strings.TrimSpace(workspace)
	if workspace == "" {
		return nil
	}
	return &ownerStore{
		workspace: workspace,
		now:       time.Now,
	}
}

func (s *ownerStore) pathForOwnerFile(ownerID, filename string) string {
	ownerID = normalizeVoiceKeySegment(ownerID)
	filename = strings.TrimSpace(filename)
	if ownerID == "" || filename == "" {
		return ""
	}
	return filepath.Join(s.workspace, "memory", "owners", ownerID, filename)
}

func (s *ownerStore) EnsureOwner(ownerID string) error {
	if s == nil {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	return s.ensureOwnerLocked(ownerID)
}

func (s *ownerStore) BindDevice(ownerID, deviceID, bindingSource string) error {
	if s == nil {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	ownerID = normalizeVoiceKeySegment(ownerID)
	deviceID = normalizeVoiceKeySegment(deviceID)
	bindingSource = strings.TrimSpace(bindingSource)
	if ownerID == "" || deviceID == "" {
		return nil
	}
	if err := s.ensureOwnerLocked(ownerID); err != nil {
		return err
	}

	profile, err := s.loadProfileLocked(ownerID)
	if err != nil {
		return err
	}
	now := s.now().UTC()
	updated := false
	for idx := range profile.BoundDevices {
		if profile.BoundDevices[idx].DeviceID != deviceID {
			continue
		}
		if profile.BoundDevices[idx].BoundAt.IsZero() {
			profile.BoundDevices[idx].BoundAt = now
		}
		profile.BoundDevices[idx].LastSeenAt = now
		if bindingSource != "" {
			profile.BoundDevices[idx].BindingSource = bindingSource
		}
		updated = true
		break
	}
	if !updated {
		profile.BoundDevices = append(profile.BoundDevices, ownerBoundDevice{
			DeviceID:      deviceID,
			BindingSource: bindingSource,
			BoundAt:       now,
			LastSeenAt:    now,
		})
	}
	profile.UpdatedAt = now
	return s.writeProfileLocked(profile)
}

func (s *ownerStore) UpdateVoiceActivity(ownerID, deviceID, channel, turnID string) error {
	if s == nil {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	ownerID = normalizeVoiceKeySegment(ownerID)
	deviceID = normalizeVoiceKeySegment(deviceID)
	channel = strings.TrimSpace(channel)
	turnID = strings.TrimSpace(turnID)
	if ownerID == "" {
		return nil
	}
	if err := s.ensureOwnerLocked(ownerID); err != nil {
		return err
	}
	return s.updateVoiceActivityLocked(ownerID, deviceID, channel, turnID, s.now().UTC())
}

func (s *ownerStore) WriteSnapshot(snapshot ownerVoiceSnapshot) error {
	if s == nil {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	path := s.pathForOwnerFile(snapshot.OwnerID, ownerVoiceContextFilename)
	if path == "" {
		return nil
	}

	snapshot.OwnerID = normalizeVoiceKeySegment(snapshot.OwnerID)
	snapshot.DeviceID = normalizeVoiceKeySegment(snapshot.DeviceID)
	snapshot.Channel = strings.TrimSpace(snapshot.Channel)
	snapshot.MemoryKey = strings.TrimSpace(snapshot.MemoryKey)
	snapshot.SessionKey = strings.TrimSpace(snapshot.SessionKey)
	snapshot.LastTurnID = strings.TrimSpace(snapshot.LastTurnID)
	snapshot.Summary = strings.TrimSpace(snapshot.Summary)
	if snapshot.LastUpdatedAt.IsZero() {
		snapshot.LastUpdatedAt = s.now().UTC()
	}

	if err := s.ensureOwnerLocked(snapshot.OwnerID); err != nil {
		return err
	}
	if err := s.updateVoiceActivityLocked(
		snapshot.OwnerID,
		snapshot.DeviceID,
		snapshot.Channel,
		snapshot.LastTurnID,
		snapshot.LastUpdatedAt,
	); err != nil {
		return err
	}

	data, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return err
	}

	return fileutil.WriteFileAtomic(path, data, 0o600)
}

func (s *ownerStore) ensureOwnerLocked(ownerID string) error {
	ownerID = normalizeVoiceKeySegment(ownerID)
	if ownerID == "" {
		return nil
	}
	now := s.now().UTC()
	if err := s.writeJSONIfMissing(s.pathForOwnerFile(ownerID, ownerProfileFilename), ownerProfile{
		OwnerID:      ownerID,
		BoundDevices: []ownerBoundDevice{},
		UpdatedAt:    now,
	}); err != nil {
		return err
	}
	if err := s.writeJSONIfMissing(s.pathForOwnerFile(ownerID, ownerSubscriptionsFilename), ownerSubscriptions{
		OwnerID:   ownerID,
		Items:     []ownerSubscription{},
		UpdatedAt: now,
	}); err != nil {
		return err
	}
	if err := s.writeJSONIfMissing(s.pathForOwnerFile(ownerID, ownerRemindersFilename), ownerReminders{
		OwnerID:   ownerID,
		Items:     []ownerReminder{},
		UpdatedAt: now,
	}); err != nil {
		return err
	}
	return s.writeJSONIfMissing(s.pathForOwnerFile(ownerID, ownerWatchlistFilename), ownerWatchlist{
		OwnerID:   ownerID,
		Items:     []ownerWatchItem{},
		UpdatedAt: now,
	})
}

func (s *ownerStore) loadProfileLocked(ownerID string) (ownerProfile, error) {
	ownerID = normalizeVoiceKeySegment(ownerID)
	if ownerID == "" {
		return ownerProfile{}, nil
	}
	data, err := os.ReadFile(s.pathForOwnerFile(ownerID, ownerProfileFilename))
	if os.IsNotExist(err) {
		return ownerProfile{OwnerID: ownerID, BoundDevices: []ownerBoundDevice{}}, nil
	}
	if err != nil {
		return ownerProfile{}, fmt.Errorf("owner store: read profile: %w", err)
	}
	var profile ownerProfile
	if err := json.Unmarshal(data, &profile); err != nil {
		return ownerProfile{}, fmt.Errorf("owner store: decode profile: %w", err)
	}
	profile.OwnerID = ownerID
	if profile.BoundDevices == nil {
		profile.BoundDevices = []ownerBoundDevice{}
	}
	return profile, nil
}

func (s *ownerStore) writeProfileLocked(profile ownerProfile) error {
	profile.OwnerID = normalizeVoiceKeySegment(profile.OwnerID)
	if profile.OwnerID == "" {
		return nil
	}
	if profile.BoundDevices == nil {
		profile.BoundDevices = []ownerBoundDevice{}
	}
	if profile.UpdatedAt.IsZero() {
		profile.UpdatedAt = s.now().UTC()
	}
	data, err := json.MarshalIndent(profile, "", "  ")
	if err != nil {
		return fmt.Errorf("owner store: encode profile: %w", err)
	}
	return fileutil.WriteFileAtomic(s.pathForOwnerFile(profile.OwnerID, ownerProfileFilename), data, 0o600)
}

func (s *ownerStore) updateVoiceActivityLocked(ownerID, deviceID, channel, turnID string, now time.Time) error {
	profile, err := s.loadProfileLocked(ownerID)
	if err != nil {
		return err
	}
	profile.LastVoiceDeviceID = normalizeVoiceKeySegment(deviceID)
	profile.LastVoiceChannel = strings.TrimSpace(channel)
	profile.LastVoiceTurnID = strings.TrimSpace(turnID)
	profile.LastVoiceAt = now.UTC()
	profile.UpdatedAt = now.UTC()
	for idx := range profile.BoundDevices {
		if profile.BoundDevices[idx].DeviceID == profile.LastVoiceDeviceID {
			profile.BoundDevices[idx].LastSeenAt = now.UTC()
		}
	}
	return s.writeProfileLocked(profile)
}

func (s *ownerStore) writeJSONIfMissing(path string, payload any) error {
	if path == "" {
		return nil
	}
	if _, err := os.Stat(path); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	return fileutil.WriteFileAtomic(path, data, 0o600)
}

func buildOwnerVoiceSnapshot(
	ownerID, memoryKey, sessionKey, channel, deviceID, turnID, summary string,
	history []providers.Message,
	now time.Time,
) ownerVoiceSnapshot {
	if now.IsZero() {
		now = time.Now().UTC()
	}

	return ownerVoiceSnapshot{
		OwnerID:       normalizeVoiceKeySegment(ownerID),
		MemoryKey:     strings.TrimSpace(memoryKey),
		SessionKey:    strings.TrimSpace(sessionKey),
		Channel:       strings.TrimSpace(channel),
		DeviceID:      normalizeVoiceKeySegment(deviceID),
		LastTurnID:    strings.TrimSpace(turnID),
		Summary:       strings.TrimSpace(summary),
		RecentTurns:   buildRecentOwnerVoiceTurns(history, now),
		LastUpdatedAt: now.UTC(),
	}
}

func buildRecentOwnerVoiceTurns(history []providers.Message, now time.Time) []ownerVoiceTurn {
	const maxTurns = 6
	recent := make([]ownerVoiceTurn, 0, maxTurns)
	for i := len(history) - 1; i >= 0 && len(recent) < maxTurns; i-- {
		msg := history[i]
		if msg.Role != "user" && msg.Role != "assistant" {
			continue
		}
		content := strings.TrimSpace(msg.Content)
		if content == "" {
			continue
		}
		recent = append(recent, ownerVoiceTurn{
			Role:      msg.Role,
			Content:   content,
			Timestamp: now.UTC(),
		})
	}
	for left, right := 0, len(recent)-1; left < right; left, right = left+1, right-1 {
		recent[left], recent[right] = recent[right], recent[left]
	}
	return recent
}
