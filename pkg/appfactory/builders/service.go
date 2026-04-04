package builders

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"
)

type Service struct {
	registry         RegistryStore
	leases           LeaseStore
	now              func() time.Time
	leaseTTL         time.Duration
	heartbeatTimeout time.Duration
}

func NewService(registry RegistryStore, leases LeaseStore, leaseTTL, heartbeatTimeout time.Duration) *Service {
	return &Service{
		registry:         registry,
		leases:           leases,
		now:              func() time.Time { return time.Now().UTC() },
		leaseTTL:         leaseTTL,
		heartbeatTimeout: heartbeatTimeout,
	}
}

func (service *Service) Register(ctx context.Context, req RegisterRequest) (BuilderNode, error) {
	if req.BuilderID == "" || req.DisplayName == "" {
		return BuilderNode{}, fmt.Errorf("register builder: %w", ErrInvalidRequest)
	}
	now := service.now()
	maxParallelRuns := req.MaxParallelRuns
	if maxParallelRuns <= 0 {
		maxParallelRuns = 1
	}
	node := BuilderNode{
		BuilderID:       req.BuilderID,
		DisplayName:     req.DisplayName,
		Endpoint:        req.Endpoint,
		Status:          StatusIdle,
		CapabilityTags:  slices.Clone(req.CapabilityTags),
		ModelTags:       slices.Clone(req.ModelTags),
		Priority:        req.Priority,
		MaxParallelRuns: maxParallelRuns,
		WorkerProfile:   req.WorkerProfile,
		LastSeenAt:      now,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	if existing, err := service.registry.GetNode(ctx, req.BuilderID); err == nil {
		node.CreatedAt = existing.CreatedAt
		node.LastAssignedAt = existing.LastAssignedAt
		node.DisabledReason = existing.DisabledReason
	}
	if err := service.registry.UpsertNode(ctx, node); err != nil {
		return BuilderNode{}, err
	}
	return node, nil
}

func (service *Service) Heartbeat(ctx context.Context, req HeartbeatRequest) (BuilderNode, error) {
	if req.BuilderID == "" {
		return BuilderNode{}, fmt.Errorf("heartbeat builder: %w", ErrInvalidRequest)
	}
	node, err := service.registry.GetNode(ctx, req.BuilderID)
	if err != nil {
		return BuilderNode{}, err
	}
	status := req.Status
	if status == "" {
		if req.CurrentRunID != "" || node.CurrentRunID != "" {
			status = StatusBusy
		} else {
			status = node.Status
			if status == "" {
				status = StatusIdle
			}
		}
	}
	now := service.now()
	if err := service.registry.UpdateNodeHeartbeat(ctx, req.BuilderID, now, status, req.CurrentRunID, req.LastFailureReason); err != nil {
		return BuilderNode{}, err
	}
	return service.registry.GetNode(ctx, req.BuilderID)
}

func (service *Service) Drain(ctx context.Context, builderID, reason string) error {
	return service.registry.UpdateNodeStatus(ctx, builderID, StatusDraining, reason)
}

func (service *Service) Resume(ctx context.Context, builderID string) error {
	return service.registry.UpdateNodeStatus(ctx, builderID, StatusIdle, "")
}

func (service *Service) Disable(ctx context.Context, builderID, reason string) error {
	return service.registry.UpdateNodeStatus(ctx, builderID, StatusDisabled, reason)
}

func (service *Service) Dispatch(ctx context.Context, req Requirement) (DispatchResult, error) {
	if strings.TrimSpace(req.JobID) == "" {
		return DispatchResult{}, fmt.Errorf("dispatch builder: %w", ErrInvalidRequest)
	}
	nodes, err := service.registry.ListNodes(ctx)
	if err != nil {
		return DispatchResult{}, err
	}
	now := service.now().UTC()
	candidates := make([]dispatchCandidate, 0)
	for _, node := range nodes {
		if service.isHeartbeatExpired(node, now) {
			if node.Status != StatusOffline {
				node.Status = StatusOffline
				node.CurrentRunID = ""
				node.LastFailureReason = "heartbeat timed out"
				node.UpdatedAt = now
				if err := service.registry.UpsertNode(ctx, node); err != nil {
					return DispatchResult{}, err
				}
			}
			continue
		}
		candidate, ok := buildDispatchCandidate(node, req)
		if !ok {
			continue
		}
		candidates = append(candidates, candidate)
	}
	if len(candidates) == 0 {
		return DispatchResult{}, ErrNoBuilderCandidate
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].PreferredMatches != candidates[j].PreferredMatches {
			return candidates[i].PreferredMatches > candidates[j].PreferredMatches
		}
		if candidates[i].Node.Priority != candidates[j].Node.Priority {
			return candidates[i].Node.Priority > candidates[j].Node.Priority
		}
		if !candidates[i].Node.LastAssignedAt.Equal(candidates[j].Node.LastAssignedAt) {
			return candidates[i].Node.LastAssignedAt.Before(candidates[j].Node.LastAssignedAt)
		}
		return candidates[i].Node.BuilderID < candidates[j].Node.BuilderID
	})
	selected := candidates[0].Node
	lease, err := service.leases.Acquire(ctx, LeaseAcquireRequest{
		BuilderID: selected.BuilderID,
		JobID:     req.JobID,
		TTL:       service.leaseTTL,
		Now:       now,
	})
	if err != nil {
		return DispatchResult{}, err
	}
	selected.Status = StatusBusy
	selected.LastAssignedAt = now
	selected.UpdatedAt = now
	if err := service.registry.UpsertNode(ctx, selected); err != nil {
		return DispatchResult{}, err
	}
	decision := DispatchDecision{
		JobID:               req.JobID,
		SelectedBuilderID:   selected.BuilderID,
		CandidateBuilderIDs: dispatchCandidateIDs(candidates),
		DecisionReason:      describeDispatchDecision(candidates[0], req),
		FallbackCount:       len(candidates) - 1,
	}
	return DispatchResult{Decision: decision, Lease: lease, Node: selected}, nil
}

func (service *Service) isHeartbeatExpired(node BuilderNode, now time.Time) bool {
	if service == nil || service.heartbeatTimeout <= 0 {
		return false
	}
	if node.Status == StatusDisabled || node.Status == StatusDraining {
		return false
	}
	if node.LastSeenAt.IsZero() {
		return false
	}
	return now.Sub(node.LastSeenAt.UTC()) > service.heartbeatTimeout
}

func (service *Service) BindRun(ctx context.Context, leaseID, runID string) error {
	if err := service.leases.BindRun(ctx, leaseID, runID); err != nil {
		return err
	}
	return nil
}

func (service *Service) ReleaseByRun(ctx context.Context, runID, reason string) error {
	lease, err := service.leases.GetActiveLeaseByRun(ctx, runID, service.now())
	if err != nil {
		return err
	}
	return service.releaseLease(ctx, lease, reason)
}

func (service *Service) ReleaseByBuilder(ctx context.Context, builderID, reason string) error {
	lease, err := service.leases.GetActiveLeaseByBuilder(ctx, builderID, service.now())
	if err != nil {
		return err
	}
	return service.releaseLease(ctx, lease, reason)
}

func (service *Service) RecoverExpiredLeases(ctx context.Context, now time.Time) error {
	expired, err := service.leases.ListExpiredLeases(ctx, now)
	if err != nil {
		return err
	}
	for _, lease := range expired {
		if err := service.leases.Release(ctx, lease.LeaseID, "expired", now); err != nil {
			return err
		}
		node, err := service.registry.GetNode(ctx, lease.BuilderID)
		if err != nil {
			return err
		}
		node.CurrentRunID = ""
		node.UpdatedAt = now.UTC()
		if node.Status != StatusDisabled && node.Status != StatusDraining {
			if !node.LastSeenAt.IsZero() && now.UTC().Sub(node.LastSeenAt) > service.heartbeatTimeout {
				node.Status = StatusOffline
			} else {
				node.Status = StatusIdle
			}
			node.LastFailureReason = "lease expired"
		}
		if err := service.registry.UpsertNode(ctx, node); err != nil {
			return err
		}
	}
	return nil
}

type dispatchCandidate struct {
	Node             BuilderNode
	PreferredMatches int
}

func buildDispatchCandidate(node BuilderNode, req Requirement) (dispatchCandidate, bool) {
	if node.Status != StatusIdle {
		return dispatchCandidate{}, false
	}
	if node.MaxParallelRuns <= 0 {
		return dispatchCandidate{}, false
	}
	if node.CurrentRunID != "" {
		return dispatchCandidate{}, false
	}
	if !matchesWorkerProfile(node, req.WorkerProfileName) {
		return dispatchCandidate{}, false
	}
	if !matchesModelTier(node, req.ModelTier) {
		return dispatchCandidate{}, false
	}
	if !matchesBudgetClass(node, req.BudgetClass) {
		return dispatchCandidate{}, false
	}
	for _, required := range req.RequiredCapabilityTags {
		if !slices.Contains(node.CapabilityTags, required) {
			return dispatchCandidate{}, false
		}
	}
	return dispatchCandidate{
		Node:             node,
		PreferredMatches: countPreferredCapabilityMatches(node, req.PreferredCapabilityTags),
	}, true
}

func matchesWorkerProfile(node BuilderNode, workerProfileName string) bool {
	trimmed := strings.TrimSpace(workerProfileName)
	if trimmed == "" {
		return true
	}
	return strings.EqualFold(strings.TrimSpace(node.WorkerProfile.Image), trimmed)
}

func matchesModelTier(node BuilderNode, modelTier string) bool {
	trimmed := normalizeModelTier(modelTier)
	if trimmed == "" {
		return true
	}
	return slices.Contains(normalizedTags(node.ModelTags), trimmed)
}

func matchesBudgetClass(node BuilderNode, budgetClass string) bool {
	switch normalizeBudgetClass(budgetClass) {
	case "", "standard", "high":
		return true
	case "low":
		for _, tier := range normalizedTags(node.ModelTags) {
			if tier == "flagship" {
				return false
			}
		}
		return true
	default:
		return true
	}
}

func normalizedTags(tags []string) []string {
	result := make([]string, 0, len(tags))
	for _, tag := range tags {
		normalized := strings.ToLower(strings.TrimSpace(tag))
		if normalized == "" {
			continue
		}
		result = append(result, normalized)
	}
	return result
}

func normalizeModelTier(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "economy", "balanced", "flagship":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return ""
	}
}

func normalizeBudgetClass(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "low", "standard", "high":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return ""
	}
}

func countPreferredCapabilityMatches(node BuilderNode, preferred []string) int {
	count := 0
	for _, tag := range preferred {
		if slices.Contains(node.CapabilityTags, tag) {
			count++
		}
	}
	return count
}

func dispatchCandidateIDs(candidates []dispatchCandidate) []string {
	ids := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		ids = append(ids, candidate.Node.BuilderID)
	}
	return ids
}

func describeDispatchDecision(candidate dispatchCandidate, req Requirement) string {
	reasons := []string{"matched idle builder"}
	if candidate.PreferredMatches > 0 {
		reasons = append(reasons, fmt.Sprintf("preferred_capability_matches=%d", candidate.PreferredMatches))
	}
	if candidate.Node.Priority != 0 {
		reasons = append(reasons, fmt.Sprintf("priority=%d", candidate.Node.Priority))
	}
	if tier := normalizeModelTier(req.ModelTier); tier != "" {
		reasons = append(reasons, "model_tier="+tier)
	}
	if budget := normalizeBudgetClass(req.BudgetClass); budget != "" {
		reasons = append(reasons, "budget_class="+budget)
	}
	return strings.Join(reasons, "; ")
}

func (service *Service) releaseLease(ctx context.Context, lease Lease, reason string) error {
	if err := service.leases.Release(ctx, lease.LeaseID, reason, service.now()); err != nil {
		return err
	}
	node, err := service.registry.GetNode(ctx, lease.BuilderID)
	if err != nil {
		return err
	}
	node.CurrentRunID = ""
	node.UpdatedAt = service.now()
	if node.Status != StatusDisabled && node.Status != StatusDraining {
		node.Status = StatusIdle
	}
	return service.registry.UpsertNode(ctx, node)
}
