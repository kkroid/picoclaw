package builders

import (
	"context"
	"time"
)

type RegistryStore interface {
	UpsertNode(ctx context.Context, node BuilderNode) error
	GetNode(ctx context.Context, builderID string) (BuilderNode, error)
	ListNodes(ctx context.Context) ([]BuilderNode, error)
	UpdateNodeStatus(ctx context.Context, builderID string, status Status, reason string) error
	UpdateNodeHeartbeat(ctx context.Context, builderID string, seenAt time.Time, status Status, currentRunID string, failureReason string) error
	DeleteNode(ctx context.Context, builderID string) error
	Close() error
}

type LeaseAcquireRequest struct {
	BuilderID string
	JobID     string
	TTL       time.Duration
	Now       time.Time
}

type LeaseStore interface {
	Acquire(ctx context.Context, req LeaseAcquireRequest) (Lease, error)
	BindRun(ctx context.Context, leaseID, runID string) error
	Renew(ctx context.Context, leaseID string, ttl time.Duration, now time.Time) (Lease, error)
	Release(ctx context.Context, leaseID, reason string, now time.Time) error
	GetActiveLeaseByBuilder(ctx context.Context, builderID string, now time.Time) (Lease, error)
	GetActiveLeaseByRun(ctx context.Context, runID string, now time.Time) (Lease, error)
	ListExpiredLeases(ctx context.Context, now time.Time) ([]Lease, error)
	Close() error
}
