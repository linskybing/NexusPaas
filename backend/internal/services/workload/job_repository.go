package workload

import (
	"context"
	"sort"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/contracts"
	"github.com/linskybing/nexuspaas/backend/internal/platform"
	"github.com/linskybing/nexuspaas/backend/internal/platform/cluster"
	"github.com/linskybing/nexuspaas/backend/internal/services/shared"
)

type jobPreemptionUpdate struct {
	PreemptionID  string
	RequesterID   string
	Reason        string
	Cleanup       map[string]any
	PreemptedAt   time.Time
	CompletedAt   time.Time
	ErrorMessage  string
	StatusMessage string
}

type jobEvictionUpdate struct {
	Reason       string
	EvictedAt    time.Time
	CompletedAt  time.Time
	ErrorMessage string
}

type jobDispatchRunningUpdate struct {
	At               time.Time
	CreatedResources []map[string]any
}

type jobDispatchFailedUpdate struct {
	Reason      string
	CompletedAt time.Time
}

type jobInfrastructureRecoveryUpdate struct {
	RetryCount  int
	NextRetryAt time.Time
	Reason      string
}

type recordStoreWorkloadJobRepository struct {
	store platform.RecordStore
}

func jobRepository(app *platform.App) *recordStoreWorkloadJobRepository {
	if app == nil {
		return nil
	}
	return jobRepositoryFromStore(app.Store)
}

func jobRepositoryFromStore(store platform.RecordStore) *recordStoreWorkloadJobRepository {
	if store == nil {
		return nil
	}
	return &recordStoreWorkloadJobRepository{store: store}
}

func (r recordStoreWorkloadJobRepository) NextJobID() string {
	return r.store.NextID(jobsResource, defaultJobPrefix, defaultJobIDStart, defaultJobIDWidth)
}

func (r recordStoreWorkloadJobRepository) CreateSubmittedJob(ctx context.Context, job map[string]any) (contracts.Record[map[string]any], error) {
	return r.store.Create(ctx, jobsResource, shared.CloneMap(job))
}

func (r recordStoreWorkloadJobRepository) FindJob(ctx context.Context, idOrJobID string) (contracts.Record[map[string]any], bool) {
	if idOrJobID == "" {
		return contracts.Record[map[string]any]{}, false
	}
	if record, found := r.store.Get(ctx, jobsResource, idOrJobID); found {
		return record, true
	}
	for _, record := range r.store.List(ctx, jobsResource) {
		if shared.TextValue(record.Data, "job_id", "jobId") == idOrJobID {
			return record, true
		}
	}
	return contracts.Record[map[string]any]{}, false
}

func (r recordStoreWorkloadJobRepository) ListJobs(ctx context.Context) []contracts.Record[map[string]any] {
	return r.store.List(ctx, jobsResource)
}

func (r recordStoreWorkloadJobRepository) ListPreemptionCandidates(ctx context.Context) []contracts.Record[map[string]any] {
	return r.store.List(ctx, jobsResource)
}

func (r recordStoreWorkloadJobRepository) MarkPreempted(ctx context.Context, recordID string, update jobPreemptionUpdate) (contracts.Record[map[string]any], bool) {
	at := update.PreemptedAt
	if at.IsZero() {
		at = time.Now().UTC()
	}
	completedAt := update.CompletedAt
	if completedAt.IsZero() {
		completedAt = at
	}
	reason := shared.FirstNonEmpty(update.Reason, update.StatusMessage, "preempted by scheduler")
	data := map[string]any{
		"status":               jobStatusPreempted,
		"status_reason":        reason,
		"preemption_record_id": update.PreemptionID,
		"preempted_by_job_id":  update.RequesterID,
		"preempted_at":         at.UTC().Format(time.RFC3339),
		"completed_at":         completedAt.UTC().Format(time.RFC3339),
		"cleanup":              shared.CloneMap(update.Cleanup),
		"error_message":        update.ErrorMessage,
	}
	return r.store.Update(ctx, jobsResource, recordID, data)
}

func (r recordStoreWorkloadJobRepository) MarkEvicted(ctx context.Context, recordID string, update jobEvictionUpdate) (contracts.Record[map[string]any], bool) {
	at := update.EvictedAt
	if at.IsZero() {
		at = time.Now().UTC()
	}
	completedAt := update.CompletedAt
	if completedAt.IsZero() {
		completedAt = at
	}
	data := map[string]any{
		"status":        jobStatusEvicted,
		"status_reason": shared.FirstNonEmpty(update.Reason, "evicted by plan window reaper"),
		"evicted_at":    at.UTC().Format(time.RFC3339),
		"completed_at":  completedAt.UTC().Format(time.RFC3339),
		"error_message": update.ErrorMessage,
	}
	return r.store.Update(ctx, jobsResource, recordID, data)
}

func (r recordStoreWorkloadJobRepository) ListDispatchCandidates(ctx context.Context, now time.Time) []dispatchCandidate {
	candidates := []dispatchCandidate{}
	for _, record := range r.store.List(ctx, jobsResource) {
		status := currentJobStatus(record.Data)
		switch status {
		case jobStatusSubmitted:
			candidates = append(candidates, dispatchCandidate{record: record, dueAt: jobCreatedAt(record.Data, record.CreatedAt)})
		case jobStatusWaitingInfra:
			if due, ok := nextRetryDue(record.Data, now); ok {
				candidates = append(candidates, dispatchCandidate{record: record, dueAt: due})
			}
		}
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		leftPriority := jobPriority(candidates[i].record.Data)
		rightPriority := jobPriority(candidates[j].record.Data)
		if leftPriority != rightPriority {
			return leftPriority > rightPriority
		}
		if !candidates[i].dueAt.Equal(candidates[j].dueAt) {
			return candidates[i].dueAt.Before(candidates[j].dueAt)
		}
		return candidates[i].record.ID < candidates[j].record.ID
	})
	return candidates
}

func (r recordStoreWorkloadJobRepository) MarkDispatchRunning(ctx context.Context, id string, update jobDispatchRunningUpdate) bool {
	at := update.At
	if at.IsZero() {
		at = time.Now().UTC()
	}
	_, ok := r.store.Update(ctx, jobsResource, id, map[string]any{
		"status":            jobStatusRunning,
		"started_at":        at.UTC().Format(time.RFC3339),
		"dispatched_at":     at.UTC().Format(time.RFC3339),
		"created_resources": update.CreatedResources,
		"error_message":     "",
		"status_reason":     "",
		"next_retry_at":     nil,
	})
	return ok
}

func (r recordStoreWorkloadJobRepository) MarkDispatchFailed(ctx context.Context, id string, update jobDispatchFailedUpdate) bool {
	at := update.CompletedAt
	if at.IsZero() {
		at = time.Now().UTC()
	}
	_, ok := r.store.Update(ctx, jobsResource, id, map[string]any{
		"status":        jobStatusFailed,
		"error_message": update.Reason,
		"status_reason": update.Reason,
		"completed_at":  at.UTC().Format(time.RFC3339),
	})
	return ok
}

func (r recordStoreWorkloadJobRepository) DeferForInfrastructureRecovery(ctx context.Context, id string, update jobInfrastructureRecoveryUpdate) bool {
	_, ok := r.store.Update(ctx, jobsResource, id, map[string]any{
		"status":        jobStatusWaitingInfra,
		"retry_count":   update.RetryCount,
		"next_retry_at": update.NextRetryAt.UTC().Format(time.RFC3339),
		"error_message": update.Reason,
		"status_reason": update.Reason,
		"completed_at":  nil,
	})
	return ok
}

func (r recordStoreWorkloadJobRepository) MarkFailedIfActive(ctx context.Context, jobID, reason string) bool {
	record, found := r.FindJob(ctx, jobID)
	if !found {
		return false
	}
	if !activeJobStatuses[currentJobStatus(record.Data)] {
		return false
	}
	_, ok := r.store.Update(ctx, jobsResource, record.ID, map[string]any{"status": jobStatusFailed, "status_reason": reason})
	return ok
}

func (r recordStoreWorkloadJobRepository) ListStaleJobCandidates(ctx context.Context, now time.Time) []contracts.Record[map[string]any] {
	candidates := []contracts.Record[map[string]any]{}
	for _, record := range r.store.List(ctx, jobsResource) {
		status := currentJobStatus(record.Data)
		if !isStaleJobStatus(status) || now.Sub(jobCreatedAt(record.Data, record.CreatedAt)) < staleJobGracePeriod {
			continue
		}
		candidates = append(candidates, record)
	}
	return candidates
}

func (r recordStoreWorkloadJobRepository) ListLifecycleReconcileCandidates(ctx context.Context) []contracts.Record[map[string]any] {
	candidates := []contracts.Record[map[string]any]{}
	for _, record := range r.store.List(ctx, jobsResource) {
		if statusReconcilerLiveStatuses[currentJobStatus(record.Data)] {
			candidates = append(candidates, record)
		}
	}
	return candidates
}

func (r recordStoreWorkloadJobRepository) ApplyLifecycleObservation(ctx context.Context, record contracts.Record[map[string]any], lifecycle cluster.JobLifecycle, now time.Time) bool {
	update := statusReconcileUpdate(record, lifecycle, now)
	if len(update) == 0 {
		return true
	}
	_, ok := r.store.Update(ctx, jobsResource, record.ID, update)
	return ok
}
