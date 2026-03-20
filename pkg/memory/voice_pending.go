package memory

import (
	"encoding/json"
	"fmt"
	"hash/fnv"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/sipeed/picoclaw/pkg/fileutil"
	"github.com/sipeed/picoclaw/pkg/routing"
)

const voicePendingFilename = "voice_pending.json"

type VoicePendingItem struct {
	ID        string    `json:"id"`
	Source    string    `json:"source,omitempty"`
	Title     string    `json:"title,omitempty"`
	Content   string    `json:"content"`
	Channel   string    `json:"channel,omitempty"`
	ChatID    string    `json:"chat_id,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

type VoicePendingQueue struct {
	OwnerID         string             `json:"owner_id"`
	Items           []VoicePendingItem `json:"items,omitempty"`
	LastAnnouncedAt time.Time          `json:"last_announced_at,omitempty"`
	UpdatedAt       time.Time          `json:"updated_at"`
}

type VoicePendingStore struct {
	workspace string
	now       func() time.Time
	locks     [numLockShards]sync.Mutex
}

type VoicePendingOwnerResolver func(channel, chatID string) string

type DefaultOwnerVoicePendingWriter struct {
	store          *VoicePendingStore
	defaultOwnerID string
	ownerResolver  VoicePendingOwnerResolver
}

type VoicePendingSummary struct {
	Summary string
	Items   []VoicePendingItem
}

func NewVoicePendingStore(workspace string) *VoicePendingStore {
	workspace = strings.TrimSpace(workspace)
	if workspace == "" {
		return nil
	}
	return &VoicePendingStore{
		workspace: workspace,
		now:       time.Now,
	}
}

func NewDefaultOwnerVoicePendingWriter(workspace, ownerID string) *DefaultOwnerVoicePendingWriter {
	return NewResolvedVoicePendingWriter(workspace, ownerID, nil)
}

func NewIdentityLinkedVoicePendingWriter(
	workspace, ownerID string,
	identityLinks map[string][]string,
) *DefaultOwnerVoicePendingWriter {
	return NewResolvedVoicePendingWriter(workspace, ownerID, func(channel, chatID string) string {
		return routing.ResolveLinkedPeerID(identityLinks, channel, chatID)
	})
}

func NewResolvedVoicePendingWriter(
	workspace, ownerID string,
	ownerResolver VoicePendingOwnerResolver,
) *DefaultOwnerVoicePendingWriter {
	store := NewVoicePendingStore(workspace)
	ownerID = normalizeVoicePendingOwnerID(ownerID)
	if store == nil || (ownerID == "" && ownerResolver == nil) {
		return nil
	}
	return &DefaultOwnerVoicePendingWriter{
		store:          store,
		defaultOwnerID: ownerID,
		ownerResolver:  ownerResolver,
	}
}

func (w *DefaultOwnerVoicePendingWriter) Enqueue(source, title, content, channel, chatID string) error {
	if w == nil || w.store == nil {
		return nil
	}

	ownerID := w.resolveOwnerID(channel, chatID)
	if ownerID == "" {
		return nil
	}

	return w.store.Enqueue(ownerID, VoicePendingItem{
		Source:  source,
		Title:   title,
		Content: content,
		Channel: channel,
		ChatID:  chatID,
	})
}

func (w *DefaultOwnerVoicePendingWriter) resolveOwnerID(channel, chatID string) string {
	if w == nil {
		return ""
	}
	channel = strings.TrimSpace(channel)
	chatID = strings.TrimSpace(chatID)

	if w.ownerResolver != nil {
		ownerID := normalizeVoicePendingOwnerID(w.ownerResolver(channel, chatID))
		if ownerID != "" {
			return ownerID
		}
		if channel != "" || chatID != "" {
			return ""
		}
	}

	return w.defaultOwnerID
}

func (s *VoicePendingStore) PrepareFirstDailySummary(
	ownerID string,
	maxItems, maxItemRunes int,
) (VoicePendingSummary, error) {
	if s == nil {
		return VoicePendingSummary{}, nil
	}

	ownerID = normalizeVoicePendingOwnerID(ownerID)
	if ownerID == "" {
		return VoicePendingSummary{}, nil
	}
	if maxItems <= 0 {
		maxItems = 3
	}
	if maxItemRunes <= 0 {
		maxItemRunes = 56
	}

	lock := s.ownerLock(ownerID)
	lock.Lock()
	defer lock.Unlock()

	queue, err := s.readQueue(ownerID)
	if err != nil {
		return VoicePendingSummary{}, err
	}
	if len(queue.Items) == 0 {
		return VoicePendingSummary{}, nil
	}

	now := s.now().UTC()
	if sameVoicePendingDay(queue.LastAnnouncedAt, now) {
		return VoicePendingSummary{}, nil
	}

	summary, prepareCount := buildVoicePendingSummary(queue.Items, maxItems, maxItemRunes)
	if summary == "" || prepareCount == 0 {
		return VoicePendingSummary{}, nil
	}

	items := append([]VoicePendingItem(nil), queue.Items[:prepareCount]...)
	return VoicePendingSummary{Summary: summary, Items: items}, nil
}

func (s *VoicePendingStore) ConfirmFirstDailySummary(ownerID string, itemIDs []string) error {
	if s == nil {
		return nil
	}

	ownerID = normalizeVoicePendingOwnerID(ownerID)
	if ownerID == "" || len(itemIDs) == 0 {
		return nil
	}

	lock := s.ownerLock(ownerID)
	lock.Lock()
	defer lock.Unlock()

	queue, err := s.readQueue(ownerID)
	if err != nil {
		return err
	}
	if len(queue.Items) == 0 {
		return nil
	}

	idSet := make(map[string]struct{}, len(itemIDs))
	for _, itemID := range itemIDs {
		itemID = strings.TrimSpace(itemID)
		if itemID != "" {
			idSet[itemID] = struct{}{}
		}
	}
	if len(idSet) == 0 {
		return nil
	}

	filtered := make([]VoicePendingItem, 0, len(queue.Items))
	removed := 0
	for _, item := range queue.Items {
		if _, ok := idSet[item.ID]; ok {
			removed++
			continue
		}
		filtered = append(filtered, item)
	}
	if removed == 0 {
		return nil
	}

	now := s.now().UTC()
	queue.Items = filtered
	queue.LastAnnouncedAt = now
	queue.UpdatedAt = now
	return s.writeQueue(ownerID, queue)
}

func (s *VoicePendingStore) Load(ownerID string) (VoicePendingQueue, error) {
	if s == nil {
		return VoicePendingQueue{}, nil
	}

	ownerID = normalizeVoicePendingOwnerID(ownerID)
	if ownerID == "" {
		return VoicePendingQueue{}, nil
	}

	lock := s.ownerLock(ownerID)
	lock.Lock()
	defer lock.Unlock()

	return s.readQueue(ownerID)
}

func (s *VoicePendingStore) Enqueue(ownerID string, item VoicePendingItem) error {
	if s == nil {
		return nil
	}

	ownerID = normalizeVoicePendingOwnerID(ownerID)
	if ownerID == "" {
		return nil
	}

	title := normalizeVoicePendingText(item.Title)
	content := normalizeVoicePendingText(item.Content)
	if title == "" && content == "" {
		return nil
	}

	lock := s.ownerLock(ownerID)
	lock.Lock()
	defer lock.Unlock()

	queue, err := s.readQueue(ownerID)
	if err != nil {
		return err
	}

	now := s.now().UTC()
	entry := VoicePendingItem{
		ID:        strings.TrimSpace(item.ID),
		Source:    normalizeVoicePendingText(item.Source),
		Title:     title,
		Content:   content,
		Channel:   strings.TrimSpace(item.Channel),
		ChatID:    strings.TrimSpace(item.ChatID),
		CreatedAt: item.CreatedAt.UTC(),
	}
	if entry.ID == "" {
		entry.ID = uuid.NewString()
	}
	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = now
	}

	queue.OwnerID = ownerID
	queue.Items = append(queue.Items, entry)
	queue.UpdatedAt = now

	return s.writeQueue(ownerID, queue)
}

func (s *VoicePendingStore) ConsumeFirstDailySummary(
	ownerID string,
	maxItems, maxItemRunes int,
) (string, []VoicePendingItem, error) {
	prepared, err := s.PrepareFirstDailySummary(ownerID, maxItems, maxItemRunes)
	if err != nil {
		return "", nil, err
	}
	if len(prepared.Items) == 0 || prepared.Summary == "" {
		return "", nil, nil
	}

	itemIDs := make([]string, 0, len(prepared.Items))
	for _, item := range prepared.Items {
		if item.ID != "" {
			itemIDs = append(itemIDs, item.ID)
		}
	}
	if err := s.ConfirmFirstDailySummary(ownerID, itemIDs); err != nil {
		return "", nil, err
	}

	return prepared.Summary, prepared.Items, nil
}

func (s *VoicePendingStore) ownerLock(ownerID string) *sync.Mutex {
	h := fnv.New32a()
	_, _ = h.Write([]byte(ownerID))
	return &s.locks[h.Sum32()%numLockShards]
}

func (s *VoicePendingStore) pathForOwner(ownerID string) string {
	ownerID = normalizeVoicePendingOwnerID(ownerID)
	if ownerID == "" {
		return ""
	}
	return filepath.Join(s.workspace, "memory", "queues", ownerID, voicePendingFilename)
}

func (s *VoicePendingStore) readQueue(ownerID string) (VoicePendingQueue, error) {
	ownerID = normalizeVoicePendingOwnerID(ownerID)
	if ownerID == "" {
		return VoicePendingQueue{}, nil
	}

	path := s.pathForOwner(ownerID)
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return VoicePendingQueue{OwnerID: ownerID}, nil
	}
	if err != nil {
		return VoicePendingQueue{}, fmt.Errorf("voice pending: read queue: %w", err)
	}

	var queue VoicePendingQueue
	if err := json.Unmarshal(data, &queue); err != nil {
		return VoicePendingQueue{}, fmt.Errorf("voice pending: decode queue: %w", err)
	}
	queue.OwnerID = ownerID
	if queue.Items == nil {
		queue.Items = []VoicePendingItem{}
	}
	return queue, nil
}

func (s *VoicePendingStore) writeQueue(ownerID string, queue VoicePendingQueue) error {
	ownerID = normalizeVoicePendingOwnerID(ownerID)
	if ownerID == "" {
		return nil
	}

	queue.OwnerID = ownerID
	if queue.Items == nil {
		queue.Items = []VoicePendingItem{}
	}
	if queue.UpdatedAt.IsZero() {
		queue.UpdatedAt = s.now().UTC()
	}

	data, err := json.MarshalIndent(queue, "", "  ")
	if err != nil {
		return fmt.Errorf("voice pending: encode queue: %w", err)
	}

	return fileutil.WriteFileAtomic(s.pathForOwner(ownerID), data, 0o600)
}

func buildVoicePendingSummary(items []VoicePendingItem, maxItems, maxItemRunes int) (string, int) {
	if len(items) == 0 {
		return "", 0
	}
	if maxItems <= 0 {
		maxItems = 3
	}
	if maxItemRunes <= 0 {
		maxItemRunes = 56
	}

	parts := make([]string, 0, maxItems+2)
	parts = append(parts, fmt.Sprintf("先同步一下，当前有 %d 条待播更新。", len(items)))

	consumed := 0
	for i := 0; i < len(items) && consumed < maxItems; i++ {
		text := summarizeVoicePendingItem(items[i], maxItemRunes)
		if text == "" {
			continue
		}
		consumed++
		parts = append(parts, fmt.Sprintf("第 %d 条，%s。", consumed, text))
	}
	if consumed == 0 {
		return "", 0
	}

	if remaining := len(items) - consumed; remaining > 0 {
		parts = append(parts, fmt.Sprintf("其余 %d 条我先帮你记着。", remaining))
	}

	return strings.Join(parts, ""), consumed
}

func summarizeVoicePendingItem(item VoicePendingItem, maxRunes int) string {
	base := normalizeVoicePendingText(item.Title)
	if base == "" {
		base = normalizeVoicePendingText(item.Content)
	}
	if source := normalizeVoicePendingText(item.Source); source != "" {
		if base != "" {
			base = source + "：" + base
		} else {
			base = source
		}
	}
	if base == "" {
		return ""
	}
	return truncateVoicePendingText(base, maxRunes)
}

func normalizeVoicePendingOwnerID(ownerID string) string {
	ownerID = strings.TrimSpace(strings.ToLower(ownerID))
	if ownerID == "" {
		return ""
	}
	replacer := strings.NewReplacer(" ", "-", "\t", "-", "\n", "-", "\r", "-", "/", "-", "\\", "-")
	return replacer.Replace(ownerID)
}

func normalizeVoicePendingText(value string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
}

func truncateVoicePendingText(value string, maxRunes int) string {
	value = strings.TrimSpace(value)
	if value == "" || maxRunes <= 0 {
		return value
	}
	if utf8.RuneCountInString(value) <= maxRunes {
		return value
	}

	runes := []rune(value)
	if maxRunes <= 3 {
		return string(runes[:maxRunes])
	}
	return string(runes[:maxRunes-3]) + "..."
}

func sameVoicePendingDay(left, right time.Time) bool {
	if left.IsZero() || right.IsZero() {
		return false
	}
	left = left.UTC()
	right = right.UTC()
	ly, lm, ld := left.Date()
	ry, rm, rd := right.Date()
	return ly == ry && lm == rm && ld == rd
}
