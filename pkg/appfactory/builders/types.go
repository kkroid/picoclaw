package builders

import "time"

type Status string

const (
	StatusIdle      Status = "idle"
	StatusBusy      Status = "busy"
	StatusDraining  Status = "draining"
	StatusUnhealthy Status = "unhealthy"
	StatusOffline   Status = "offline"
	StatusDisabled  Status = "disabled"
)

type MaintenanceAction string

const (
	MaintenanceDrain   MaintenanceAction = "drain"
	MaintenanceResume  MaintenanceAction = "resume"
	MaintenanceDisable MaintenanceAction = "disable"
)

type WorkerProfile struct {
	Image              string            `json:"image"`
	AndroidAPILevel    int               `json:"android_api_level,omitempty"`
	EmulatorProfile    string            `json:"emulator_profile,omitempty"`
	MountTemplates     []string          `json:"mount_templates,omitempty"`
	Env                map[string]string `json:"env,omitempty"`
	NetworkPolicy      string            `json:"network_policy,omitempty"`
	WorkspaceRoot      string            `json:"workspace_root,omitempty"`
	ArtifactsRoot      string            `json:"artifacts_root,omitempty"`
	PreserveTTLMinutes int               `json:"preserve_ttl_minutes,omitempty"`
}

type BuilderNode struct {
	BuilderID         string        `json:"builder_id"`
	DisplayName       string        `json:"display_name"`
	Endpoint          string        `json:"endpoint,omitempty"`
	Status            Status        `json:"status"`
	CapabilityTags    []string      `json:"capability_tags,omitempty"`
	ModelTags         []string      `json:"model_tags,omitempty"`
	Priority          int           `json:"priority,omitempty"`
	MaxParallelRuns   int           `json:"max_parallel_runs"`
	CurrentRunID      string        `json:"current_run_id,omitempty"`
	WorkerProfile     WorkerProfile `json:"worker_profile"`
	LastSeenAt        time.Time     `json:"last_seen_at,omitempty"`
	LastAssignedAt    time.Time     `json:"last_assigned_at,omitempty"`
	LastFailureReason string        `json:"last_failure_reason,omitempty"`
	DisabledReason    string        `json:"disabled_reason,omitempty"`
	CreatedAt         time.Time     `json:"created_at"`
	UpdatedAt         time.Time     `json:"updated_at"`
}

type Lease struct {
	LeaseID       string    `json:"lease_id"`
	BuilderID     string    `json:"builder_id"`
	JobID         string    `json:"job_id"`
	RunID         string    `json:"run_id,omitempty"`
	FencingToken  uint64    `json:"fencing_token"`
	AcquiredAt    time.Time `json:"acquired_at"`
	RenewedAt     time.Time `json:"renewed_at"`
	ExpiresAt     time.Time `json:"expires_at"`
	ReleasedAt    time.Time `json:"released_at,omitempty"`
	ReleaseReason string    `json:"release_reason,omitempty"`
}

func (lease Lease) IsActive(now time.Time) bool {
	return lease.ReleasedAt.IsZero() && now.Before(lease.ExpiresAt)
}

type Requirement struct {
	JobID                   string   `json:"job_id"`
	RequiredCapabilityTags  []string `json:"required_capability_tags,omitempty"`
	PreferredCapabilityTags []string `json:"preferred_capability_tags,omitempty"`
	ModelTier               string   `json:"model_tier,omitempty"`
	BudgetClass             string   `json:"budget_class,omitempty"`
	WorkerProfileName       string   `json:"worker_profile_name,omitempty"`
}

type DispatchDecision struct {
	JobID               string   `json:"job_id"`
	SelectedBuilderID   string   `json:"selected_builder_id"`
	CandidateBuilderIDs []string `json:"candidate_builder_ids,omitempty"`
	DecisionReason      string   `json:"decision_reason"`
	FallbackCount       int      `json:"fallback_count"`
}

type DispatchResult struct {
	Decision DispatchDecision `json:"decision"`
	Lease    Lease            `json:"lease"`
	Node     BuilderNode      `json:"node"`
}

type RegisterRequest struct {
	BuilderID       string        `json:"builder_id"`
	DisplayName     string        `json:"display_name"`
	Endpoint        string        `json:"endpoint,omitempty"`
	CapabilityTags  []string      `json:"capability_tags,omitempty"`
	ModelTags       []string      `json:"model_tags,omitempty"`
	Priority        int           `json:"priority,omitempty"`
	MaxParallelRuns int           `json:"max_parallel_runs,omitempty"`
	WorkerProfile   WorkerProfile `json:"worker_profile"`
}

type HeartbeatRequest struct {
	BuilderID         string  `json:"builder_id"`
	Status            Status  `json:"status,omitempty"`
	CurrentRunID      string  `json:"current_run_id,omitempty"`
	LastFailureReason string  `json:"last_failure_reason,omitempty"`
	Load              float64 `json:"load,omitempty"`
	FreeDiskBytes     int64   `json:"free_disk_bytes,omitempty"`
}
