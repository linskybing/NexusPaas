package schedulerquota

import (
	"context"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/contracts"
	"github.com/linskybing/nexuspaas/backend/internal/platform"
	"github.com/linskybing/nexuspaas/backend/internal/services/shared"
)

type schedulerQuotaRepository interface {
	NextQueueID() string
	NextPlanID() string
	ListQueues(ctx context.Context) []contracts.Record[map[string]any]
	GetQueue(ctx context.Context, id string) (contracts.Record[map[string]any], bool)
	FindQueueByNameOrID(ctx context.Context, value string) (contracts.Record[map[string]any], bool)
	CreateQueue(ctx context.Context, queue map[string]any) (contracts.Record[map[string]any], error)
	UpdateQueue(ctx context.Context, id string, update map[string]any) (contracts.Record[map[string]any], bool)
	DeleteQueueAndRemoveFromPlans(ctx context.Context, id string) bool
	DeleteQueues(ctx context.Context, ids []string) schedulerDeleteResult

	ListPlans(ctx context.Context) []contracts.Record[map[string]any]
	GetPlan(ctx context.Context, id string) (contracts.Record[map[string]any], bool)
	CreatePlan(ctx context.Context, plan map[string]any) (contracts.Record[map[string]any], error)
	UpdatePlan(ctx context.Context, id string, update map[string]any) (contracts.Record[map[string]any], bool)
	DeletePlan(ctx context.Context, id string) bool
	DeletePlans(ctx context.Context, ids []string) schedulerDeleteResult
	MissingQueues(ctx context.Context, queueIDs []string) []string
	QueuesForPlan(ctx context.Context, planID string) (contracts.Record[map[string]any], []contracts.Record[map[string]any], bool)
	BindPlanQueues(ctx context.Context, planID string, queueIDs []string) (contracts.Record[map[string]any], bool)

	GetLiveQuota(ctx context.Context, projectID string) (contracts.Record[map[string]any], bool)
	DerivedQuotaFromPlan(ctx context.Context, projectID, planID string, now time.Time) (contracts.Record[map[string]any], bool)
	PersistSubmitAdmissionReview(ctx context.Context, review admissionReview) bool
}

type schedulerDeleteResult struct {
	Succeeded int
	Failed    int
	Errors    []string
	Deleted   []string
}

func (r schedulerDeleteResult) response() map[string]any {
	return map[string]any{
		"succeeded": r.Succeeded,
		"failed":    r.Failed,
		"errors":    append([]string{}, r.Errors...),
	}
}

type recordStoreSchedulerQuotaRepository struct {
	store platform.RecordStore
}

func schedulerQuotaRepositoryForApp(app *platform.App) schedulerQuotaRepository {
	if app == nil {
		return nil
	}
	return schedulerQuotaRepositoryFromStore(app.Store)
}

func schedulerQuotaRepositoryFromStore(store platform.RecordStore) schedulerQuotaRepository {
	if store == nil {
		return nil
	}
	return recordStoreSchedulerQuotaRepository{store: store}
}

func (repo recordStoreSchedulerQuotaRepository) NextQueueID() string {
	return repo.store.NextID(queuesResource, defaultQueuePrefix, defaultIDStart, defaultIDWidth)
}

func (repo recordStoreSchedulerQuotaRepository) NextPlanID() string {
	return repo.store.NextID(plansResource, defaultPlanPrefix, defaultIDStart, defaultIDWidth)
}

func (repo recordStoreSchedulerQuotaRepository) ListQueues(ctx context.Context) []contracts.Record[map[string]any] {
	return repo.store.List(ctx, queuesResource)
}

func (repo recordStoreSchedulerQuotaRepository) GetQueue(ctx context.Context, id string) (contracts.Record[map[string]any], bool) {
	return repo.store.Get(ctx, queuesResource, id)
}

func (repo recordStoreSchedulerQuotaRepository) FindQueueByNameOrID(ctx context.Context, value string) (contracts.Record[map[string]any], bool) {
	if value == "" {
		return contracts.Record[map[string]any]{}, false
	}
	if queue, found := repo.GetQueue(ctx, value); found {
		return queue, true
	}
	for _, queue := range repo.ListQueues(ctx) {
		if queue.ID == value || shared.TextValue(queue.Data, "name") == value {
			return queue, true
		}
	}
	return contracts.Record[map[string]any]{}, false
}

func (repo recordStoreSchedulerQuotaRepository) CreateQueue(ctx context.Context, queue map[string]any) (contracts.Record[map[string]any], error) {
	return repo.store.Create(ctx, queuesResource, shared.CloneMap(queue))
}

func (repo recordStoreSchedulerQuotaRepository) UpdateQueue(ctx context.Context, id string, update map[string]any) (contracts.Record[map[string]any], bool) {
	return repo.store.Update(ctx, queuesResource, id, shared.CloneMap(update))
}

func (repo recordStoreSchedulerQuotaRepository) DeleteQueueAndRemoveFromPlans(ctx context.Context, id string) bool {
	if !repo.store.Delete(ctx, queuesResource, id) {
		return false
	}
	repo.removeQueueFromPlans(ctx, id)
	return true
}

func (repo recordStoreSchedulerQuotaRepository) DeleteQueues(ctx context.Context, ids []string) schedulerDeleteResult {
	result := schedulerDeleteResult{Errors: []string{}, Deleted: []string{}}
	for _, id := range ids {
		if repo.DeleteQueueAndRemoveFromPlans(ctx, id) {
			result.Succeeded++
			result.Deleted = append(result.Deleted, id)
			continue
		}
		result.Failed++
		result.Errors = append(result.Errors, id)
	}
	return result
}

func (repo recordStoreSchedulerQuotaRepository) ListPlans(ctx context.Context) []contracts.Record[map[string]any] {
	return repo.store.List(ctx, plansResource)
}

func (repo recordStoreSchedulerQuotaRepository) GetPlan(ctx context.Context, id string) (contracts.Record[map[string]any], bool) {
	return repo.store.Get(ctx, plansResource, id)
}

func (repo recordStoreSchedulerQuotaRepository) CreatePlan(ctx context.Context, plan map[string]any) (contracts.Record[map[string]any], error) {
	return repo.store.Create(ctx, plansResource, shared.CloneMap(plan))
}

func (repo recordStoreSchedulerQuotaRepository) UpdatePlan(ctx context.Context, id string, update map[string]any) (contracts.Record[map[string]any], bool) {
	return repo.store.Update(ctx, plansResource, id, shared.CloneMap(update))
}

func (repo recordStoreSchedulerQuotaRepository) DeletePlan(ctx context.Context, id string) bool {
	return repo.store.Delete(ctx, plansResource, id)
}

func (repo recordStoreSchedulerQuotaRepository) DeletePlans(ctx context.Context, ids []string) schedulerDeleteResult {
	result := schedulerDeleteResult{Errors: []string{}, Deleted: []string{}}
	for _, id := range ids {
		if repo.DeletePlan(ctx, id) {
			result.Succeeded++
			result.Deleted = append(result.Deleted, id)
			continue
		}
		result.Failed++
		result.Errors = append(result.Errors, id)
	}
	return result
}

func (repo recordStoreSchedulerQuotaRepository) MissingQueues(ctx context.Context, queueIDs []string) []string {
	var missing []string
	for _, id := range queueIDs {
		if _, found := repo.GetQueue(ctx, id); !found {
			missing = append(missing, id)
		}
	}
	return missing
}

func (repo recordStoreSchedulerQuotaRepository) QueuesForPlan(
	ctx context.Context,
	planID string,
) (contracts.Record[map[string]any], []contracts.Record[map[string]any], bool) {
	plan, found := repo.GetPlan(ctx, planID)
	if !found {
		return contracts.Record[map[string]any]{}, nil, false
	}
	queueIDs := shared.StringSlice(plan.Data["queue_ids"])
	records := make([]contracts.Record[map[string]any], 0, len(queueIDs))
	for _, id := range queueIDs {
		if record, found := repo.GetQueue(ctx, id); found {
			records = append(records, record)
		}
	}
	return plan, records, true
}

func (repo recordStoreSchedulerQuotaRepository) BindPlanQueues(
	ctx context.Context,
	planID string,
	queueIDs []string,
) (contracts.Record[map[string]any], bool) {
	return repo.UpdatePlan(ctx, planID, map[string]any{"queue_ids": queueIDs, "queues": queueIDs})
}

func (repo recordStoreSchedulerQuotaRepository) GetLiveQuota(ctx context.Context, projectID string) (contracts.Record[map[string]any], bool) {
	return repo.store.Get(ctx, liveQuotasResource, projectID)
}

func (repo recordStoreSchedulerQuotaRepository) DerivedQuotaFromPlan(
	ctx context.Context,
	projectID, planID string,
	now time.Time,
) (contracts.Record[map[string]any], bool) {
	plan, found := repo.GetPlan(ctx, planID)
	if !found {
		return contracts.Record[map[string]any]{}, false
	}
	return contracts.Record[map[string]any]{
		ID:        projectID,
		Data:      quotaFromPlan(projectID, plan, now),
		Version:   1,
		CreatedAt: now,
		UpdatedAt: now,
	}, true
}

func (repo recordStoreSchedulerQuotaRepository) PersistSubmitAdmissionReview(ctx context.Context, review admissionReview) bool {
	data := admissionReviewData(review)
	data["id"] = shared.FirstNonEmpty(
		review.ProjectID+"/"+review.UserID+"/"+review.QueueName,
		repo.store.NextID(submitAdmissionsResource, "ADM", defaultIDStart, defaultIDWidth),
	)
	if _, err := repo.store.Create(ctx, submitAdmissionsResource, data); err != nil && !platform.IsCreateConflict(err) {
		return false
	}
	return true
}

func (repo recordStoreSchedulerQuotaRepository) removeQueueFromPlans(ctx context.Context, queueID string) {
	for _, plan := range repo.ListPlans(ctx) {
		queueIDs := removeValue(shared.StringSlice(plan.Data["queue_ids"]), queueID)
		if len(queueIDs) == len(shared.StringSlice(plan.Data["queue_ids"])) {
			continue
		}
		repo.UpdatePlan(ctx, plan.ID, map[string]any{"queue_ids": queueIDs, "queues": queueIDs})
	}
}
