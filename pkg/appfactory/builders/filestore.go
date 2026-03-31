package builders

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/sipeed/picoclaw/pkg/fileutil"
)

type FileRegistryStore struct {
	root string
	mu   sync.Mutex
}

type FileLeaseStore struct {
	root string
	mu   sync.Mutex
}

func NewFileRegistryStore(root string) (*FileRegistryStore, error) {
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, fmt.Errorf("create builder registry dir: %w", err)
	}
	return &FileRegistryStore{root: root}, nil
}

func NewFileLeaseStore(root string) (*FileLeaseStore, error) {
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, fmt.Errorf("create builder lease dir: %w", err)
	}
	return &FileLeaseStore{root: root}, nil
}

func (store *FileRegistryStore) UpsertNode(_ context.Context, node BuilderNode) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	if node.BuilderID == "" {
		return fmt.Errorf("upsert node: %w", ErrInvalidRequest)
	}
	return writeJSON(nodePath(store.root, node.BuilderID), node)
}

func (store *FileRegistryStore) GetNode(_ context.Context, builderID string) (BuilderNode, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	var node BuilderNode
	if err := readJSON(nodePath(store.root, builderID), &node); err != nil {
		if os.IsNotExist(err) {
			return BuilderNode{}, ErrBuilderNotFound
		}
		return BuilderNode{}, err
	}
	return node, nil
}

func (store *FileRegistryStore) ListNodes(_ context.Context) ([]BuilderNode, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	entries, err := os.ReadDir(store.root)
	if err != nil {
		return nil, fmt.Errorf("list nodes: %w", err)
	}
	nodes := make([]BuilderNode, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		var node BuilderNode
		if err := readJSON(filepath.Join(store.root, entry.Name()), &node); err != nil {
			return nil, err
		}
		nodes = append(nodes, node)
	}
	sort.Slice(nodes, func(i, j int) bool {
		return nodes[i].BuilderID < nodes[j].BuilderID
	})
	return nodes, nil
}

func (store *FileRegistryStore) UpdateNodeStatus(ctx context.Context, builderID string, status Status, reason string) error {
	node, err := store.GetNode(ctx, builderID)
	if err != nil {
		return err
	}
	node.Status = status
	node.UpdatedAt = time.Now().UTC()
	if status == StatusDisabled {
		node.DisabledReason = reason
	} else if reason != "" {
		node.LastFailureReason = reason
	}
	return store.UpsertNode(ctx, node)
}

func (store *FileRegistryStore) UpdateNodeHeartbeat(ctx context.Context, builderID string, seenAt time.Time, status Status, currentRunID string, failureReason string) error {
	node, err := store.GetNode(ctx, builderID)
	if err != nil {
		return err
	}
	node.LastSeenAt = seenAt.UTC()
	node.UpdatedAt = seenAt.UTC()
	node.CurrentRunID = currentRunID
	if failureReason != "" {
		node.LastFailureReason = failureReason
	}
	if node.Status != StatusDisabled && node.Status != StatusDraining {
		node.Status = status
	}
	return store.UpsertNode(ctx, node)
}

func (store *FileRegistryStore) DeleteNode(_ context.Context, builderID string) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	if err := os.Remove(nodePath(store.root, builderID)); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("delete node: %w", err)
	}
	return nil
}

func (store *FileRegistryStore) Close() error {
	return nil
}

func (store *FileLeaseStore) Acquire(_ context.Context, req LeaseAcquireRequest) (Lease, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	if req.BuilderID == "" || req.JobID == "" || req.TTL <= 0 {
		return Lease{}, fmt.Errorf("acquire lease: %w", ErrInvalidRequest)
	}
	now := req.Now.UTC()
	leases, err := store.listLeasesLocked()
	if err != nil {
		return Lease{}, err
	}
	var nextFencing uint64 = 1
	for _, lease := range leases {
		if lease.FencingToken >= nextFencing {
			nextFencing = lease.FencingToken + 1
		}
		if lease.BuilderID == req.BuilderID && lease.IsActive(now) {
			return Lease{}, ErrBuilderAlreadyLeased
		}
	}
	lease := Lease{
		LeaseID:      uuid.NewString(),
		BuilderID:    req.BuilderID,
		JobID:        req.JobID,
		FencingToken: nextFencing,
		AcquiredAt:   now,
		RenewedAt:    now,
		ExpiresAt:    now.Add(req.TTL),
	}
	if err := writeJSON(leasePath(store.root, lease.LeaseID), lease); err != nil {
		return Lease{}, err
	}
	return lease, nil
}

func (store *FileLeaseStore) BindRun(_ context.Context, leaseID, runID string) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	lease, err := store.readLeaseLocked(leaseID)
	if err != nil {
		return err
	}
	lease.RunID = runID
	return writeJSON(leasePath(store.root, leaseID), lease)
}

func (store *FileLeaseStore) Renew(_ context.Context, leaseID string, ttl time.Duration, now time.Time) (Lease, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	lease, err := store.readLeaseLocked(leaseID)
	if err != nil {
		return Lease{}, err
	}
	if !lease.IsActive(now.UTC()) {
		return Lease{}, ErrLeaseExpired
	}
	lease.RenewedAt = now.UTC()
	lease.ExpiresAt = now.UTC().Add(ttl)
	if err := writeJSON(leasePath(store.root, leaseID), lease); err != nil {
		return Lease{}, err
	}
	return lease, nil
}

func (store *FileLeaseStore) Release(_ context.Context, leaseID, reason string, now time.Time) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	lease, err := store.readLeaseLocked(leaseID)
	if err != nil {
		return err
	}
	lease.ReleasedAt = now.UTC()
	lease.ReleaseReason = reason
	return writeJSON(leasePath(store.root, leaseID), lease)
}

func (store *FileLeaseStore) GetActiveLeaseByBuilder(_ context.Context, builderID string, now time.Time) (Lease, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	return store.findActiveLeaseLocked(func(lease Lease) bool {
		return lease.BuilderID == builderID
	}, now.UTC())
}

func (store *FileLeaseStore) GetActiveLeaseByRun(_ context.Context, runID string, now time.Time) (Lease, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	return store.findActiveLeaseLocked(func(lease Lease) bool {
		return lease.RunID == runID
	}, now.UTC())
}

func (store *FileLeaseStore) ListExpiredLeases(_ context.Context, now time.Time) ([]Lease, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	leases, err := store.listLeasesLocked()
	if err != nil {
		return nil, err
	}
	result := make([]Lease, 0)
	for _, lease := range leases {
		if lease.ReleasedAt.IsZero() && !lease.ExpiresAt.IsZero() && !now.UTC().Before(lease.ExpiresAt) {
			result = append(result, lease)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].LeaseID < result[j].LeaseID
	})
	return result, nil
}

func (store *FileLeaseStore) Close() error {
	return nil
}

func (store *FileLeaseStore) readLeaseLocked(leaseID string) (Lease, error) {
	var lease Lease
	if err := readJSON(leasePath(store.root, leaseID), &lease); err != nil {
		if os.IsNotExist(err) {
			return Lease{}, ErrLeaseNotFound
		}
		return Lease{}, err
	}
	return lease, nil
}

func (store *FileLeaseStore) listLeasesLocked() ([]Lease, error) {
	entries, err := os.ReadDir(store.root)
	if err != nil {
		return nil, fmt.Errorf("list leases: %w", err)
	}
	leases := make([]Lease, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		var lease Lease
		if err := readJSON(filepath.Join(store.root, entry.Name()), &lease); err != nil {
			return nil, err
		}
		leases = append(leases, lease)
	}
	return leases, nil
}

func (store *FileLeaseStore) findActiveLeaseLocked(match func(Lease) bool, now time.Time) (Lease, error) {
	leases, err := store.listLeasesLocked()
	if err != nil {
		return Lease{}, err
	}
	for _, lease := range leases {
		if match(lease) && lease.IsActive(now) {
			return lease, nil
		}
	}
	return Lease{}, ErrLeaseNotFound
}

func nodePath(root, builderID string) string {
	return filepath.Join(root, sanitizeID(builderID)+".json")
}

func leasePath(root, leaseID string) string {
	return filepath.Join(root, sanitizeID(leaseID)+".json")
}

func sanitizeID(value string) string {
	replacer := strings.NewReplacer(":", "_", "/", "_", "\\", "_")
	return replacer.Replace(value)
}

func writeJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal json: %w", err)
	}
	return fileutil.WriteFileAtomic(path, data, 0o600)
}

func readJSON(path string, target any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, target); err != nil {
		return fmt.Errorf("unmarshal json %s: %w", path, err)
	}
	return nil
}
