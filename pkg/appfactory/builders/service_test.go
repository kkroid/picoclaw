package builders

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestFileStoresAcquireAndRelease(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	leases, err := NewFileLeaseStore(filepath.Join(root, "leases"))
	if err != nil {
		t.Fatalf("new lease store: %v", err)
	}
	now := time.Date(2026, 3, 26, 10, 0, 0, 0, time.UTC)
	lease, err := leases.Acquire(ctx, LeaseAcquireRequest{
		BuilderID: "builder-a",
		JobID:     "job-1",
		TTL:       3 * time.Minute,
		Now:       now,
	})
	if err != nil {
		t.Fatalf("acquire lease: %v", err)
	}
	if _, err := leases.Acquire(ctx, LeaseAcquireRequest{
		BuilderID: "builder-a",
		JobID:     "job-2",
		TTL:       3 * time.Minute,
		Now:       now,
	}); err != ErrBuilderAlreadyLeased {
		t.Fatalf("expected ErrBuilderAlreadyLeased, got %v", err)
	}
	if err := leases.BindRun(ctx, lease.LeaseID, "run-1"); err != nil {
		t.Fatalf("bind run: %v", err)
	}
	if err := leases.Release(ctx, lease.LeaseID, "done", now.Add(time.Minute)); err != nil {
		t.Fatalf("release lease: %v", err)
	}
	if _, err := leases.GetActiveLeaseByRun(ctx, "run-1", now.Add(2*time.Minute)); err != ErrLeaseNotFound {
		t.Fatalf("expected ErrLeaseNotFound after release, got %v", err)
	}
}

func TestServiceDispatchAndRecoverExpiredLease(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	registry, err := NewFileRegistryStore(filepath.Join(root, "nodes"))
	if err != nil {
		t.Fatalf("new registry: %v", err)
	}
	leases, err := NewFileLeaseStore(filepath.Join(root, "leases"))
	if err != nil {
		t.Fatalf("new leases: %v", err)
	}
	service := NewService(registry, leases, 2*time.Minute, 3*time.Minute)
	now := time.Date(2026, 3, 26, 10, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return now }
	if _, err := service.Register(ctx, RegisterRequest{
		BuilderID:      "builder-a",
		DisplayName:    "Builder A",
		CapabilityTags: []string{"flutter", "android"},
		WorkerProfile:  WorkerProfile{Image: "builder:latest"},
	}); err != nil {
		t.Fatalf("register builder: %v", err)
	}
	result, err := service.Dispatch(ctx, Requirement{
		JobID:                  "job-1",
		RequiredCapabilityTags: []string{"flutter"},
	})
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if result.Node.Status != StatusBusy {
		t.Fatalf("expected busy after dispatch, got %s", result.Node.Status)
	}
	now = now.Add(5 * time.Minute)
	if err := service.RecoverExpiredLeases(ctx, now); err != nil {
		t.Fatalf("recover expired leases: %v", err)
	}
	node, err := registry.GetNode(ctx, "builder-a")
	if err != nil {
		t.Fatalf("get builder: %v", err)
	}
	if node.Status != StatusOffline {
		t.Fatalf("expected offline after expired lease with stale heartbeat, got %s", node.Status)
	}
}

func TestServiceHeartbeatPreservesDisabledState(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	registry, err := NewFileRegistryStore(filepath.Join(root, "nodes"))
	if err != nil {
		t.Fatalf("new registry: %v", err)
	}
	leases, err := NewFileLeaseStore(filepath.Join(root, "leases"))
	if err != nil {
		t.Fatalf("new leases: %v", err)
	}
	service := NewService(registry, leases, 2*time.Minute, 3*time.Minute)
	if _, err := service.Register(ctx, RegisterRequest{
		BuilderID:     "builder-a",
		DisplayName:   "Builder A",
		WorkerProfile: WorkerProfile{Image: "builder:latest"},
	}); err != nil {
		t.Fatalf("register builder: %v", err)
	}
	if err := service.Disable(ctx, "builder-a", "manual maintenance"); err != nil {
		t.Fatalf("disable builder: %v", err)
	}
	node, err := service.Heartbeat(ctx, HeartbeatRequest{
		BuilderID:    "builder-a",
		Status:       StatusBusy,
		CurrentRunID: "run-1",
	})
	if err != nil {
		t.Fatalf("heartbeat: %v", err)
	}
	if node.Status != StatusDisabled {
		t.Fatalf("expected disabled status preserved, got %s", node.Status)
	}
}

func TestServiceReleaseByBuilder(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	registry, err := NewFileRegistryStore(filepath.Join(root, "nodes"))
	if err != nil {
		t.Fatalf("new registry: %v", err)
	}
	leases, err := NewFileLeaseStore(filepath.Join(root, "leases"))
	if err != nil {
		t.Fatalf("new leases: %v", err)
	}
	service := NewService(registry, leases, 2*time.Minute, 3*time.Minute)
	now := time.Date(2026, 3, 26, 10, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return now }
	if _, err := service.Register(ctx, RegisterRequest{
		BuilderID:      "builder-a",
		DisplayName:    "Builder A",
		CapabilityTags: []string{"flutter"},
		WorkerProfile:  WorkerProfile{Image: "builder:latest"},
	}); err != nil {
		t.Fatalf("register builder: %v", err)
	}
	result, err := service.Dispatch(ctx, Requirement{JobID: "job-1"})
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if err := service.ReleaseByBuilder(ctx, result.Node.BuilderID, "worker released"); err != nil {
		t.Fatalf("release by builder: %v", err)
	}
	node, err := registry.GetNode(ctx, result.Node.BuilderID)
	if err != nil {
		t.Fatalf("get node: %v", err)
	}
	if node.Status != StatusIdle {
		t.Fatalf("expected idle after builder release, got %s", node.Status)
	}
}

func TestServiceDispatchPrefersHigherPriorityAndPreferredCapabilities(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	registry, err := NewFileRegistryStore(filepath.Join(root, "nodes"))
	if err != nil {
		t.Fatalf("new registry: %v", err)
	}
	leases, err := NewFileLeaseStore(filepath.Join(root, "leases"))
	if err != nil {
		t.Fatalf("new leases: %v", err)
	}
	service := NewService(registry, leases, 2*time.Minute, 3*time.Minute)
	now := time.Date(2026, 3, 28, 10, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return now }
	for _, req := range []RegisterRequest{{
		BuilderID:      "builder-a",
		DisplayName:    "Builder A",
		CapabilityTags: []string{"flutter", "android"},
		Priority:       10,
		WorkerProfile:  WorkerProfile{Image: "builder-a:latest"},
	}, {
		BuilderID:      "builder-b",
		DisplayName:    "Builder B",
		CapabilityTags: []string{"flutter", "android", "dart"},
		Priority:       50,
		WorkerProfile:  WorkerProfile{Image: "builder-b:latest"},
	}} {
		if _, err := service.Register(ctx, req); err != nil {
			t.Fatalf("register builder %s: %v", req.BuilderID, err)
		}
	}
	result, err := service.Dispatch(ctx, Requirement{
		JobID:                   "job-priority",
		RequiredCapabilityTags:  []string{"flutter"},
		PreferredCapabilityTags: []string{"dart"},
	})
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if result.Node.BuilderID != "builder-b" {
		t.Fatalf("selected builder = %s, want builder-b", result.Node.BuilderID)
	}
	if len(result.Decision.CandidateBuilderIDs) != 2 || result.Decision.CandidateBuilderIDs[0] != "builder-b" {
		t.Fatalf("candidate order = %v, want builder-b first", result.Decision.CandidateBuilderIDs)
	}
	if result.Decision.FallbackCount != 1 {
		t.Fatalf("fallback_count = %d, want 1", result.Decision.FallbackCount)
	}
	if result.Decision.DecisionReason == "" || !strings.Contains(result.Decision.DecisionReason, "preferred_capability_matches=1") {
		t.Fatalf("decision_reason = %q, want preferred capability summary", result.Decision.DecisionReason)
	}
}

func TestServiceDispatchRejectsFlagshipBuilderForLowBudget(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	registry, err := NewFileRegistryStore(filepath.Join(root, "nodes"))
	if err != nil {
		t.Fatalf("new registry: %v", err)
	}
	leases, err := NewFileLeaseStore(filepath.Join(root, "leases"))
	if err != nil {
		t.Fatalf("new leases: %v", err)
	}
	service := NewService(registry, leases, 2*time.Minute, 3*time.Minute)
	for _, req := range []RegisterRequest{{
		BuilderID:      "builder-economy",
		DisplayName:    "Builder Economy",
		CapabilityTags: []string{"flutter"},
		ModelTags:      []string{"economy"},
		WorkerProfile:  WorkerProfile{Image: "economy:latest"},
	}, {
		BuilderID:      "builder-flagship",
		DisplayName:    "Builder Flagship",
		CapabilityTags: []string{"flutter"},
		ModelTags:      []string{"flagship"},
		Priority:       100,
		WorkerProfile:  WorkerProfile{Image: "flagship:latest"},
	}} {
		if _, err := service.Register(ctx, req); err != nil {
			t.Fatalf("register builder %s: %v", req.BuilderID, err)
		}
	}
	result, err := service.Dispatch(ctx, Requirement{
		JobID:                  "job-low-budget",
		RequiredCapabilityTags: []string{"flutter"},
		BudgetClass:            "low",
	})
	if err != nil {
		t.Fatalf("dispatch low budget: %v", err)
	}
	if result.Node.BuilderID != "builder-economy" {
		t.Fatalf("selected builder = %s, want builder-economy", result.Node.BuilderID)
	}
	noCandidateService := NewService(registry, leases, 2*time.Minute, 3*time.Minute)
	if _, err := noCandidateService.Dispatch(ctx, Requirement{
		JobID:                  "job-no-candidate",
		RequiredCapabilityTags: []string{"flutter"},
		BudgetClass:            "low",
		ModelTier:              "flagship",
	}); err != ErrNoBuilderCandidate {
		t.Fatalf("dispatch low budget flagship = %v, want ErrNoBuilderCandidate", err)
	}
}

func TestServiceDispatchSkipsHeartbeatExpiredIdleBuilders(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	registry, err := NewFileRegistryStore(filepath.Join(root, "nodes"))
	if err != nil {
		t.Fatalf("new registry: %v", err)
	}
	leases, err := NewFileLeaseStore(filepath.Join(root, "leases"))
	if err != nil {
		t.Fatalf("new leases: %v", err)
	}
	service := NewService(registry, leases, 2*time.Minute, 3*time.Minute)
	now := time.Date(2026, 4, 2, 14, 20, 0, 0, time.UTC)
	service.now = func() time.Time { return now }
	for _, req := range []RegisterRequest{ {
		BuilderID:      "builder-stale",
		DisplayName:    "Builder Stale",
		CapabilityTags: []string{"flutter"},
		WorkerProfile:  WorkerProfile{Image: "builder:latest"},
	}, {
		BuilderID:      "builder-fresh",
		DisplayName:    "Builder Fresh",
		CapabilityTags: []string{"flutter"},
		WorkerProfile:  WorkerProfile{Image: "builder:latest"},
	} } {
		if _, err := service.Register(ctx, req); err != nil {
			t.Fatalf("register builder %s: %v", req.BuilderID, err)
		}
	}
	if err := registry.UpdateNodeHeartbeat(ctx, "builder-stale", now.Add(-10*time.Minute), StatusIdle, "", ""); err != nil {
		t.Fatalf("mark stale heartbeat: %v", err)
	}
	result, err := service.Dispatch(ctx, Requirement{JobID: "job-1", RequiredCapabilityTags: []string{"flutter"}})
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if result.Node.BuilderID != "builder-fresh" {
		t.Fatalf("selected builder = %s, want builder-fresh", result.Node.BuilderID)
	}
	staleNode, err := registry.GetNode(ctx, "builder-stale")
	if err != nil {
		t.Fatalf("get stale builder: %v", err)
	}
	if staleNode.Status != StatusOffline {
		t.Fatalf("stale node status = %s, want offline", staleNode.Status)
	}
	if staleNode.LastFailureReason != "heartbeat timed out" {
		t.Fatalf("stale node last_failure_reason = %q, want heartbeat timed out", staleNode.LastFailureReason)
	}
}

func TestServiceDispatchMatchesModelTierAndWorkerProfile(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	registry, err := NewFileRegistryStore(filepath.Join(root, "nodes"))
	if err != nil {
		t.Fatalf("new registry: %v", err)
	}
	leases, err := NewFileLeaseStore(filepath.Join(root, "leases"))
	if err != nil {
		t.Fatalf("new leases: %v", err)
	}
	service := NewService(registry, leases, 2*time.Minute, 3*time.Minute)
	for _, req := range []RegisterRequest{{
		BuilderID:      "builder-balanced",
		DisplayName:    "Builder Balanced",
		CapabilityTags: []string{"flutter"},
		ModelTags:      []string{"balanced"},
		WorkerProfile:  WorkerProfile{Image: "balanced:latest"},
	}, {
		BuilderID:      "builder-flagship",
		DisplayName:    "Builder Flagship",
		CapabilityTags: []string{"flutter"},
		ModelTags:      []string{"flagship"},
		WorkerProfile:  WorkerProfile{Image: "flagship:latest"},
	}} {
		if _, err := service.Register(ctx, req); err != nil {
			t.Fatalf("register builder %s: %v", req.BuilderID, err)
		}
	}
	result, err := service.Dispatch(ctx, Requirement{
		JobID:             "job-flagship-profile",
		ModelTier:         "flagship",
		WorkerProfileName: "flagship:latest",
	})
	if err != nil {
		t.Fatalf("dispatch model tier/profile: %v", err)
	}
	if result.Node.BuilderID != "builder-flagship" {
		t.Fatalf("selected builder = %s, want builder-flagship", result.Node.BuilderID)
	}
}
