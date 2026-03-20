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
)

type voiceDeviceBinding struct {
	DeviceID         string    `json:"device_id"`
	OwnerID          string    `json:"owner_id,omitempty"`
	BindingSource    string    `json:"binding_source,omitempty"`
	LastConnectionID string    `json:"last_connection_id,omitempty"`
	BoundAt          time.Time `json:"bound_at,omitempty"`
	LastSeenAt       time.Time `json:"last_seen_at,omitempty"`
	UpdatedAt        time.Time `json:"updated_at"`
}

type voiceDeviceStore struct {
	workspace string
	now       func() time.Time
	mu        sync.Mutex
}

func newVoiceDeviceStore(workspace string) *voiceDeviceStore {
	workspace = strings.TrimSpace(workspace)
	if workspace == "" {
		return nil
	}
	return &voiceDeviceStore{
		workspace: workspace,
		now:       time.Now,
	}
}

func (s *voiceDeviceStore) pathForDevice(deviceID string) string {
	deviceID = normalizeVoiceKeySegment(deviceID)
	if deviceID == "" {
		return ""
	}
	return filepath.Join(s.workspace, "state", "devices", deviceID+".json")
}

func (s *voiceDeviceStore) ResolveOwner(deviceID string) (string, error) {
	if s == nil {
		return "", nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	binding, _, err := s.readBindingLocked(deviceID)
	if err != nil {
		return "", err
	}
	return binding.OwnerID, nil
}

func (s *voiceDeviceStore) BindOwner(deviceID, ownerID, connID string) error {
	if s == nil {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	deviceID = normalizeVoiceKeySegment(deviceID)
	ownerID = normalizeVoiceKeySegment(ownerID)
	connID = normalizeVoiceKeySegment(connID)
	if deviceID == "" || ownerID == "" {
		return nil
	}

	binding, _, err := s.readBindingLocked(deviceID)
	if err != nil {
		return err
	}
	now := s.now().UTC()
	if binding.BoundAt.IsZero() || binding.OwnerID != ownerID {
		binding.BoundAt = now
	}
	binding.DeviceID = deviceID
	binding.OwnerID = ownerID
	binding.BindingSource = ownerBindingSourceExplicit
	binding.LastConnectionID = connID
	binding.LastSeenAt = now
	binding.UpdatedAt = now
	return s.writeBindingLocked(binding)
}

func (s *voiceDeviceStore) Touch(deviceID, connID string) error {
	if s == nil {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	binding, exists, err := s.readBindingLocked(deviceID)
	if err != nil || !exists {
		return err
	}
	binding.LastConnectionID = normalizeVoiceKeySegment(connID)
	binding.LastSeenAt = s.now().UTC()
	binding.UpdatedAt = binding.LastSeenAt
	return s.writeBindingLocked(binding)
}

func (s *voiceDeviceStore) readBindingLocked(deviceID string) (voiceDeviceBinding, bool, error) {
	deviceID = normalizeVoiceKeySegment(deviceID)
	if deviceID == "" {
		return voiceDeviceBinding{}, false, nil
	}

	data, err := os.ReadFile(s.pathForDevice(deviceID))
	if os.IsNotExist(err) {
		return voiceDeviceBinding{DeviceID: deviceID}, false, nil
	}
	if err != nil {
		return voiceDeviceBinding{}, false, fmt.Errorf("voice device: read binding: %w", err)
	}

	var binding voiceDeviceBinding
	if err := json.Unmarshal(data, &binding); err != nil {
		return voiceDeviceBinding{}, false, fmt.Errorf("voice device: decode binding: %w", err)
	}
	binding.DeviceID = deviceID
	binding.OwnerID = normalizeVoiceKeySegment(binding.OwnerID)
	return binding, true, nil
}

func (s *voiceDeviceStore) writeBindingLocked(binding voiceDeviceBinding) error {
	binding.DeviceID = normalizeVoiceKeySegment(binding.DeviceID)
	binding.OwnerID = normalizeVoiceKeySegment(binding.OwnerID)
	binding.BindingSource = strings.TrimSpace(binding.BindingSource)
	binding.LastConnectionID = normalizeVoiceKeySegment(binding.LastConnectionID)
	if binding.DeviceID == "" {
		return nil
	}
	if binding.UpdatedAt.IsZero() {
		binding.UpdatedAt = s.now().UTC()
	}

	data, err := json.MarshalIndent(binding, "", "  ")
	if err != nil {
		return fmt.Errorf("voice device: encode binding: %w", err)
	}
	return fileutil.WriteFileAtomic(s.pathForDevice(binding.DeviceID), data, 0o600)
}
