package runs

import "context"

type Service struct {
	store Store
}

func NewService(store Store) *Service {
	return &Service{store: store}
}

func (service *Service) Create(ctx context.Context, builderID, workerID, leaseID string, input BuildInput) (RunRecord, error) {
	return service.store.CreateRun(ctx, builderID, workerID, leaseID, input)
}

func (service *Service) Get(ctx context.Context, runID string) (RunRecord, error) {
	return service.store.GetRun(ctx, runID)
}

func (service *Service) Heartbeat(ctx context.Context, runID string, heartbeat Heartbeat) (RunRecord, error) {
	return service.store.UpdateHeartbeat(ctx, runID, heartbeat)
}

func (service *Service) Complete(ctx context.Context, runID string, output BuildOutput) (RunRecord, error) {
	return service.store.CompleteRun(ctx, runID, output)
}

func (service *Service) Fail(ctx context.Context, runID string, report FailureReport) (RunRecord, error) {
	return service.store.FailRun(ctx, runID, report)
}

func (service *Service) Cancel(ctx context.Context, runID, reason string) (RunRecord, error) {
	return service.store.CancelRun(ctx, runID, reason)
}

func (service *Service) IndexArtifacts(ctx context.Context, runID string, manifest ArtifactManifest) (RunRecord, error) {
	return service.store.IndexArtifacts(ctx, runID, manifest)
}

func (service *Service) IndexMetrics(ctx context.Context, runID string, metrics Metrics) (RunRecord, error) {
	return service.store.IndexMetrics(ctx, runID, metrics)
}
