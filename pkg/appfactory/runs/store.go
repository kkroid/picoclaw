package runs

import "context"

type Store interface {
	CreateRun(ctx context.Context, builderID, workerID, leaseID string, input BuildInput) (RunRecord, error)
	GetRun(ctx context.Context, runID string) (RunRecord, error)
	UpdateHeartbeat(ctx context.Context, runID string, heartbeat Heartbeat) (RunRecord, error)
	CompleteRun(ctx context.Context, runID string, output BuildOutput) (RunRecord, error)
	FailRun(ctx context.Context, runID string, report FailureReport) (RunRecord, error)
	CancelRun(ctx context.Context, runID, reason string) (RunRecord, error)
	IndexArtifacts(ctx context.Context, runID string, manifest ArtifactManifest) (RunRecord, error)
	IndexMetrics(ctx context.Context, runID string, metrics Metrics) (RunRecord, error)
	Close() error
}
