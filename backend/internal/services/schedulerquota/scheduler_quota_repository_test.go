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

	"github.com/linskybing/nexuspaas/backend/internal/platform"
)

func TestSchedulerQuotaRepositoryIDsCreateCloneAndConflicts(t *testing.T) {
	ctx := context.Background()
	app := newSchedulerQuotaTestApp()
	repo := schedulerQuotaRepositoryForApp(app)

	if got := repo.NextQueueID(); got != "Q2600001" {
		t.Fatalf("NextQueueID = %q, want Q2600001", got)
	}
	if got := repo.NextPlanID(); got != "PL2600001" {
		t.Fatalf("NextPlanID = %q, want PL2600001", got)
	}

	queue := map[string]any{"id": "q1", "name": "gpu"}
	createdQueue, err := repo.CreateQueue(ctx, queue)
	if err != nil {
		t.Fatalf("CreateQueue: %v", err)
	}
	queue["name"] = "mutated"
	if createdQueue.Data["name"] != "gpu" {
		t.Fatalf("queue record mutated through caller map: %#v", createdQueue.Data)
	}
	if _, err := repo.CreateQueue(ctx, map[string]any{"id": "q1", "name": "dupe"}); !platform.IsCreateConflict(err) {
		t.Fatalf("duplicate queue err = %v, want create conflict", err)
	}

	plan := map[string]any{"id": "p1", "name": "starter", "queue_ids": []string{"q1"}}
	createdPlan, err := repo.CreatePlan(ctx, plan)
	if err != nil {
		t.Fatalf("CreatePlan: %v", err)
	}
	plan["name"] = "mutated"
	if createdPlan.Data["name"] != "starter" {
		t.Fatalf("plan record mutated through caller map: %#v", createdPlan.Data)
	}
	if _, err := repo.CreatePlan(ctx, map[string]any{"id": "p1", "name": "dupe"}); !platform.IsCreateConflict(err) {
		t.Fatalf("duplicate plan err = %v, want create conflict", err)
	}
}

func TestSchedulerQuotaRepositoryQueuePlanBindingsAndBatchDeletes(t *testing.T) {
	ctx := context.Background()
	app := newSchedulerQuotaTestApp()
	repo := schedulerQuotaRepositoryForApp(app)

	mustCreateQueue(t, repo, ctx, map[string]any{"id": "q1", "name": "alpha"})
	mustCreateQueue(t, repo, ctx, map[string]any{"id": "q2", "name": "zeta"})
	mustCreatePlan(t, repo, ctx, map[string]any{"id": "p1", "name": "starter", "queue_ids": []string{"q1", "q2"}})

	if missing := repo.MissingQueues(ctx, []string{"q1", "missing"}); len(missing) != 1 || missing[0] != "missing" {
		t.Fatalf("MissingQueues = %v, want [missing]", missing)
	}
	_, queues, found := repo.QueuesForPlan(ctx, "p1")
	if !found || len(queues) != 2 || queues[0].ID != "q1" || queues[1].ID != "q2" {
		t.Fatalf("QueuesForPlan found=%v queues=%#v, want q1/q2", found, queues)
	}
	if _, ok := repo.BindPlanQueues(ctx, "p1", []string{"q1"}); !ok {
		t.Fatal("BindPlanQueues returned false")
	}
	plan, _ := repo.GetPlan(ctx, "p1")
	if got := strings.Join(plan.Data["queue_ids"].([]string), ","); got != "q1" {
		t.Fatalf("bound queue_ids = %q, want q1", got)
	}

	mustCreatePlan(t, repo, ctx, map[string]any{"id": "p2", "name": "pro", "queue_ids": []string{"q1", "q2"}})
	result := repo.DeleteQueues(ctx, []string{"q2", "missing"})
	if result.Succeeded != 1 || result.Failed != 1 || len(result.Deleted) != 1 || result.Deleted[0] != "q2" {
		t.Fatalf("DeleteQueues = %#v, want q2 success and missing failure", result)
	}
	p2, _ := repo.GetPlan(ctx, "p2")
	if got := strings.Join(p2.Data["queue_ids"].([]string), ","); got != "q1" {
		t.Fatalf("p2 queue_ids after q2 delete = %q, want q1", got)
	}

	planResult := repo.DeletePlans(ctx, []string{"p1", "missing"})
	if planResult.Succeeded != 1 || planResult.Failed != 1 || len(planResult.Deleted) != 1 || planResult.Deleted[0] != "p1" {
		t.Fatalf("DeletePlans = %#v, want p1 success and missing failure", planResult)
	}
	if _, found := repo.GetPlan(ctx, "p1"); found {
		t.Fatal("DeletePlans left p1 in repository")
	}
}

func TestSchedulerQuotaRepositoryLiveQuotaDerivedQuotaAndAdmissionAudit(t *testing.T) {
	ctx := context.Background()
	app := newSchedulerQuotaTestApp()
	repo := schedulerQuotaRepositoryForApp(app)

	mustCreatePlan(t, repo, ctx, map[string]any{
		"id":              "plan-1",
		"name":            "default",
		"gpu_limit":       4.0,
		"cpu_limit_cores": 8.0,
		"memory_limit_gb": 16.0,
		"queue_ids":       []string{"q1"},
	})
	now := time.Date(2026, 6, 16, 1, 2, 3, 0, time.UTC)
	derived, found := repo.DerivedQuotaFromPlan(ctx, "project-1", "plan-1", now)
	if !found {
		t.Fatal("DerivedQuotaFromPlan did not find plan")
	}
	if derived.ID != "project-1" || derived.Data["source_resource"] != "plan" || derived.Data["generated_at"] != now {
		t.Fatalf("derived quota = %#v, want project plan quota at fixed time", derived)
	}
	if _, found := repo.GetLiveQuota(ctx, "project-1"); found {
		t.Fatal("live quota unexpectedly found before seed")
	}
	if _, err := app.Store.Create(ctx, liveQuotasResource, map[string]any{"id": "project-1", "source_resource": "live"}); err != nil {
		t.Fatal(err)
	}
	live, found := repo.GetLiveQuota(ctx, "project-1")
	if !found || live.Data["source_resource"] != "live" {
		t.Fatalf("GetLiveQuota = %#v found=%v, want live quota", live, found)
	}

	review := admissionReview{
		Allowed:        true,
		ProjectID:      "project-1",
		UserID:         "user-1",
		QueueName:      "default",
		RequiredGPU:    1,
		RequiredCPU:    2,
		RequiredMemory: 1024,
	}
	if !repo.PersistSubmitAdmissionReview(ctx, review) {
		t.Fatal("PersistSubmitAdmissionReview returned false")
	}
	if !repo.PersistSubmitAdmissionReview(ctx, review) {
		t.Fatal("PersistSubmitAdmissionReview conflict should stay audit-only success")
	}
	if _, found := app.Store.Get(ctx, submitAdmissionsResource, "project-1/user-1/default"); !found {
		t.Fatal("admission review was not persisted under project/user/queue id")
	}
}

func TestSchedulerQuotaRepositoryNilStoreFailClosed(t *testing.T) {
	if repo := schedulerQuotaRepositoryFromStore(nil); repo != nil {
		t.Fatalf("schedulerQuotaRepositoryFromStore(nil) = %#v, want nil", repo)
	}
}

func TestSchedulerQuotaRepositorySourceGuardOwnsSchedulerResources(t *testing.T) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	dir := filepath.Dir(currentFile)
	owned := `(queuesResource|plansResource|liveQuotasResource|submitAdmissionsResource)`
	storeCall := `(?:Store|store)\s*\.\s*(?:Get|List|Create|Update|Delete|NextID)`
	afterStore := regexp.MustCompile(storeCall + `(?s:[^\n;]*)\b` + owned + `\b`)
	beforeStore := regexp.MustCompile(`\b` + owned + `\b(?s:[^\n;]*)` + storeCall)

	err := filepath.WalkDir(dir, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}
		name := filepath.Base(path)
		if strings.HasSuffix(name, "_test.go") || name == "scheduler_quota_repository.go" {
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		text := string(content)
		if afterStore.MatchString(text) || beforeStore.MatchString(text) {
			t.Errorf("%s directly accesses scheduler-owned resources through RecordStore; use schedulerQuotaRepository", path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func mustCreateQueue(t *testing.T, repo *recordStoreSchedulerQuotaRepository, ctx context.Context, data map[string]any) {
	t.Helper()
	if _, err := repo.CreateQueue(ctx, data); err != nil {
		t.Fatal(err)
	}
}

func mustCreatePlan(t *testing.T, repo *recordStoreSchedulerQuotaRepository, ctx context.Context, data map[string]any) {
	t.Helper()
	if _, err := repo.CreatePlan(ctx, data); err != nil {
		t.Fatal(err)
	}
}
