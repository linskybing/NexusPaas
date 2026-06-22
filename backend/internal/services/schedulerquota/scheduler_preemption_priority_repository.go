package schedulerquota

import (
	"context"
	"fmt"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/contracts"
	"github.com/linskybing/nexuspaas/backend/internal/platform"
	"github.com/linskybing/nexuspaas/backend/internal/services/shared"
)

const (
	preemptionRecordsResource        = serviceName + ":preemption_records"
	priorityClassesResource          = serviceName + ":priority_classes"
	priorityClassSyncRunsResource    = serviceName + ":priority_class_sync_runs"
	priorityClassSyncLatestRunID     = "latest"
	errPriorityClassSummaryUpdateMsg = "priority class sync summary update failed"
)

type recordStoreSchedulerPreemptionPriorityRepository struct {
	store platform.RecordStore
}

func schedulerPreemptionPriorityRepositoryForApp(app *platform.App) *recordStoreSchedulerPreemptionPriorityRepository {
	if app == nil {
		return nil
	}
	return schedulerPreemptionPriorityRepositoryFromStore(app.Store)
}

func schedulerPreemptionPriorityRepositoryFromStore(store platform.RecordStore) *recordStoreSchedulerPreemptionPriorityRepository {
	if store == nil {
		return nil
	}
	return &recordStoreSchedulerPreemptionPriorityRepository{store: store}
}

func (repo recordStoreSchedulerPreemptionPriorityRepository) FindPreemptionRecord(
	ctx context.Context,
	id string,
) (contracts.Record[map[string]any], bool) {
	return repo.store.Get(ctx, preemptionRecordsResource, id)
}

func (repo recordStoreSchedulerPreemptionPriorityRepository) CreatePreemptionRecord(
	ctx context.Context,
	data map[string]any,
) (contracts.Record[map[string]any], error) {
	return repo.store.Create(ctx, preemptionRecordsResource, shared.CloneMap(data))
}

func (repo recordStoreSchedulerPreemptionPriorityRepository) FinishPreemptionRecord(
	ctx context.Context,
	id, status string,
	updates map[string]any,
	completedAt time.Time,
) map[string]any {
	update := shared.CloneMap(updates)
	update["status"] = status
	update["completed_at"] = completedAt.UTC().Format(time.RFC3339)
	record, ok := repo.store.Update(ctx, preemptionRecordsResource, id, update)
	if !ok {
		return update
	}
	return record.Data
}

func (repo recordStoreSchedulerPreemptionPriorityRepository) AppendPreemptionVictim(
	ctx context.Context,
	id string,
	victim map[string]any,
) {
	record, found := repo.FindPreemptionRecord(ctx, id)
	if !found {
		return
	}
	victims := preemptionListOfMaps(record.Data["victims"])
	victims = append(victims, shared.CloneMap(victim))
	repo.store.Update(ctx, preemptionRecordsResource, id, map[string]any{"victims": victims})
}

func (repo recordStoreSchedulerPreemptionPriorityRepository) AppendPreemptionVictimTx(
	ctx context.Context,
	tx platform.StoreTx,
	id string,
	victim map[string]any,
) error {
	record, found := repo.FindPreemptionRecord(ctx, id)
	if !found {
		return nil
	}
	victims := preemptionListOfMaps(record.Data["victims"])
	victims = append(victims, shared.CloneMap(victim))
	_, _, err := tx.Update(ctx, preemptionRecordsResource, id, map[string]any{"victims": victims})
	return err
}

func (repo recordStoreSchedulerPreemptionPriorityRepository) PreemptionRecordVictims(
	ctx context.Context,
	id string,
) []map[string]any {
	record, found := repo.FindPreemptionRecord(ctx, id)
	if !found {
		return nil
	}
	return preemptionListOfMaps(record.Data["victims"])
}

func (repo recordStoreSchedulerPreemptionPriorityRepository) ListPriorityClassRecords(
	ctx context.Context,
) []contracts.Record[map[string]any] {
	records := repo.store.List(ctx, priorityClassesResource)
	out := make([]contracts.Record[map[string]any], 0, len(records))
	for _, record := range records {
		record.Data = shared.CloneMap(record.Data)
		out = append(out, record)
	}
	return out
}

func (repo recordStoreSchedulerPreemptionPriorityRepository) PriorityClassSourceResource() string {
	return priorityClassesResource
}

func (repo recordStoreSchedulerPreemptionPriorityRepository) PriorityClassSyncSummaryResource() string {
	return priorityClassSyncRunsResource
}

func (repo recordStoreSchedulerPreemptionPriorityRepository) UpsertPriorityClassSyncSummary(
	ctx context.Context,
	data map[string]any,
) error {
	summary := shared.CloneMap(data)
	if _, found := repo.store.Get(ctx, priorityClassSyncRunsResource, priorityClassSyncLatestRunID); found {
		if _, ok := repo.store.Update(ctx, priorityClassSyncRunsResource, priorityClassSyncLatestRunID, summary); !ok {
			return fmt.Errorf(errPriorityClassSummaryUpdateMsg)
		}
		return nil
	}
	if _, err := repo.store.Create(ctx, priorityClassSyncRunsResource, summary); err != nil {
		if platform.IsCreateConflict(err) {
			if _, ok := repo.store.Update(ctx, priorityClassSyncRunsResource, priorityClassSyncLatestRunID, summary); ok {
				return nil
			}
		}
		return fmt.Errorf("priority class sync summary create failed: %w", err)
	}
	return nil
}
