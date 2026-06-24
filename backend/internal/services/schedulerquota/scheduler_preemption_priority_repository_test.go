package schedulerquota

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/contracts"
	"github.com/linskybing/nexuspaas/backend/internal/platform"
)

func TestSchedulerPreemptionPriorityRepositoryPreemptionLifecycle(t *testing.T) {
	ctx := context.Background()
	app := newSchedulerQuotaTestApp()
	repo := schedulerPreemptionPriorityRepositoryForApp(app)
	req := preemptionRequest{
		IdempotencyKey: "preempt-key",
		RequesterJobID: "requester",
		ProjectID:      "project-1",
		QueueName:      "batch",
		PriorityValue:  1000,
		RequiredGPU:    1,
		MaxPreemptions: 2,
		Fingerprint:    "fingerprint",
	}
	recordID := preemptionRecordID(req.IdempotencyKey)
	initial := initialPreemptionRecord(recordID, req)
	assertInitialPreemptionRecordPrivateMaterial(t, initial, recordID)

	created, err := repo.CreatePreemptionRecord(ctx, initial)
	if err != nil {
		t.Fatalf("CreatePreemptionRecord: %v", err)
	}
	initial["status"] = "mutated"
	found, ok := repo.FindPreemptionRecord(ctx, recordID)
	if !ok || found.ID != created.ID || found.Data["status"] != "in_progress" {
		t.Fatalf("FindPreemptionRecord = %#v found=%v, want in_progress record", found, ok)
	}
	if _, err := repo.CreatePreemptionRecord(ctx, initialPreemptionRecord(recordID, req)); !platform.IsCreateConflict(err) {
		t.Fatalf("duplicate CreatePreemptionRecord err = %v, want create conflict", err)
	}

	victim := map[string]any{"job_id": "victim-1", "status": "preempted"}
	repo.AppendPreemptionVictim(ctx, recordID, victim)
	victim["status"] = "mutated"
	victims := repo.PreemptionRecordVictims(ctx, recordID)
	if len(victims) != 1 || victims[0]["job_id"] != "victim-1" || victims[0]["status"] != "preempted" {
		t.Fatalf("PreemptionRecordVictims = %#v, want cloned victim", victims)
	}

	completedAt := time.Date(2026, 6, 16, 1, 2, 3, 0, time.UTC)
	finished := repo.FinishPreemptionRecord(ctx, recordID, "completed", map[string]any{
		"accepted": true,
		"victims":  victims,
	}, completedAt)
	if finished["status"] != "completed" || finished["accepted"] != true || finished["completed_at"] != completedAt.Format(time.RFC3339) {
		t.Fatalf("FinishPreemptionRecord = %#v, want completed accepted at fixed time", finished)
	}
	missing := repo.FinishPreemptionRecord(ctx, "missing", "failed", map[string]any{"reason": "gone"}, completedAt)
	if missing["status"] != "failed" || missing["reason"] != "gone" {
		t.Fatalf("FinishPreemptionRecord missing = %#v, want update payload", missing)
	}
}

func assertInitialPreemptionRecordPrivateMaterial(t *testing.T, initial map[string]any, recordID string) {
	t.Helper()
	if initial["idempotency_key"] != nil || initial["fingerprint"] != nil {
		t.Fatalf("initial preemption record stored raw idempotency material")
	}
	if initial[internalPreemptionIdempotencyKeyHash] == "" || initial[internalPreemptionFingerprintHash] == "" {
		t.Fatalf("initial preemption record missing private hashes")
	}
	if initial["preemption_id"] == "" || initial["preemption_id"] == recordID {
		t.Fatalf("initial preemption record must have generated public preemption_id")
	}
}

func TestSchedulerPreemptionPriorityRepositoryPriorityClassesAndSummary(t *testing.T) {
	ctx := context.Background()
	app := newSchedulerQuotaTestApp()
	repo := schedulerPreemptionPriorityRepositoryForApp(app)
	createSchedulerPriorityRecord(t, app, priorityClassesResource, map[string]any{
		"id":    "pc1",
		"name":  "platform-low",
		"value": 10,
	})
	createSchedulerPriorityRecord(t, app, priorityClassesResource, map[string]any{
		"id":    "pc2",
		"name":  "platform-high",
		"value": 100,
	})

	records := repo.ListPriorityClassRecords(ctx)
	if len(records) != 2 {
		t.Fatalf("ListPriorityClassRecords = %d, want 2", len(records))
	}
	records[0].Data["name"] = "mutated"
	again := repo.ListPriorityClassRecords(ctx)
	if again[0].Data["name"] == "mutated" || again[1].Data["name"] == "mutated" {
		t.Fatalf("ListPriorityClassRecords leaked caller mutation: %#v", again)
	}

	summary := map[string]any{"id": priorityClassSyncLatestRunID, "status": "synced", "source_count": 2}
	if err := repo.UpsertPriorityClassSyncSummary(ctx, summary); err != nil {
		t.Fatalf("UpsertPriorityClassSyncSummary create: %v", err)
	}
	summary["status"] = "mutated"
	stored, found := app.Store.Get(ctx, priorityClassSyncRunsResource, priorityClassSyncLatestRunID)
	if !found || stored.Data["status"] != "synced" {
		t.Fatalf("created summary = %#v found=%v, want synced", stored, found)
	}
	if err := repo.UpsertPriorityClassSyncSummary(ctx, map[string]any{"id": priorityClassSyncLatestRunID, "status": "updated"}); err != nil {
		t.Fatalf("UpsertPriorityClassSyncSummary update: %v", err)
	}
	updated, _ := app.Store.Get(ctx, priorityClassSyncRunsResource, priorityClassSyncLatestRunID)
	if updated.Data["status"] != "updated" {
		t.Fatalf("updated summary = %#v, want updated", updated.Data)
	}
}

func TestSchedulerPreemptionPriorityRepositorySummaryCreateConflictFallback(t *testing.T) {
	ctx := context.Background()
	base := platform.NewStore()
	if _, err := base.Create(ctx, priorityClassSyncRunsResource, map[string]any{
		"id":     priorityClassSyncLatestRunID,
		"status": "old",
	}); err != nil {
		t.Fatal(err)
	}
	store := summaryCreateConflictStore{RecordStore: base}
	repo := schedulerPreemptionPriorityRepositoryFromStore(store)

	if err := repo.UpsertPriorityClassSyncSummary(ctx, map[string]any{
		"id":     priorityClassSyncLatestRunID,
		"status": "fallback-updated",
	}); err != nil {
		t.Fatalf("UpsertPriorityClassSyncSummary conflict fallback: %v", err)
	}
	record, _ := base.Get(ctx, priorityClassSyncRunsResource, priorityClassSyncLatestRunID)
	if record.Data["status"] != "fallback-updated" {
		t.Fatalf("summary after conflict fallback = %#v, want fallback-updated", record.Data)
	}
}

func TestSchedulerPreemptionPriorityRepositoryNilStoreFailClosed(t *testing.T) {
	if repo := schedulerPreemptionPriorityRepositoryFromStore(nil); repo != nil {
		t.Fatalf("schedulerPreemptionPriorityRepositoryFromStore(nil) = %#v, want nil", repo)
	}
	if repo := schedulerPreemptionPriorityRepositoryForApp(nil); repo != nil {
		t.Fatalf("schedulerPreemptionPriorityRepositoryForApp(nil) = %#v, want nil", repo)
	}
}

func TestSchedulerPreemptionPriorityRepositorySourceGuardOwnsResources(t *testing.T) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	dir := filepath.Dir(currentFile)
	owned := `(preemptionRecordsResource|priorityClassesResource|priorityClassSyncRunsResource|scheduler-quota-service:preemption_records|scheduler-quota-service:priority_classes|scheduler-quota-service:priority_class_sync_runs|":preemption_records"|":priority_classes"|":priority_class_sync_runs")`
	storeCall := `(?:Store|store)\s*\.\s*(?:Get|List|Create|Update|Delete|NextID)`
	afterStore := regexp.MustCompile(storeCall + `(?s:[^\n;]*)\b` + owned + `\b`)
	beforeStore := regexp.MustCompile(`\b` + owned + `\b(?s:[^\n;]*)` + storeCall)
	literal := regexp.MustCompile(owned)

	err := filepath.WalkDir(dir, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}
		name := filepath.Base(path)
		if strings.HasSuffix(name, "_test.go") || name == "scheduler_preemption_priority_repository.go" {
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		text := string(content)
		if afterStore.MatchString(text) || beforeStore.MatchString(text) || literal.MatchString(text) {
			t.Errorf("%s directly accesses scheduler preemption/priority resources; use schedulerPreemptionPriorityRepository", path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

type summaryCreateConflictStore struct {
	platform.RecordStore
}

func (s summaryCreateConflictStore) Get(
	ctx context.Context,
	resource, id string,
) (contracts.Record[map[string]any], bool) {
	if resource == priorityClassSyncRunsResource && id == priorityClassSyncLatestRunID {
		return contracts.Record[map[string]any]{}, false
	}
	return s.RecordStore.Get(ctx, resource, id)
}

func (s summaryCreateConflictStore) Create(
	ctx context.Context,
	resource string,
	data map[string]any,
) (contracts.Record[map[string]any], error) {
	if resource == priorityClassSyncRunsResource && data["id"] == priorityClassSyncLatestRunID {
		return contracts.Record[map[string]any]{}, platform.CreateConflictError{Resource: resource, ID: priorityClassSyncLatestRunID}
	}
	return s.RecordStore.Create(ctx, resource, data)
}

func createSchedulerPriorityRecord(t *testing.T, app *platform.App, resource string, data map[string]any) {
	t.Helper()
	if _, err := app.Store.Create(context.Background(), resource, data); err != nil {
		t.Fatal(err)
	}
}
