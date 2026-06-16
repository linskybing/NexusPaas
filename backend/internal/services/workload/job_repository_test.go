package workload

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/contracts"
	"github.com/linskybing/nexuspaas/backend/internal/platform"
	"github.com/linskybing/nexuspaas/backend/internal/platform/cluster"
)

func TestJobRepositoryIDsCreateCloneAndFindAliases(t *testing.T) {
	app := platform.NewApp(platform.Config{ServiceName: serviceName})
	jobs := jobRepository(app)
	ctx := context.Background()

	if got := jobs.NextJobID(); got != "J2600001" {
		t.Fatalf("first job id = %q, want J2600001", got)
	}
	if got := jobs.NextJobID(); got != "J2600002" {
		t.Fatalf("second job id = %q, want J2600002", got)
	}

	job := map[string]any{"id": "store-id", "job_id": "logical-id", "status": jobStatusSubmitted}
	if _, err := jobs.CreateSubmittedJob(ctx, job); err != nil {
		t.Fatalf("create submitted job: %v", err)
	}
	job["status"] = "mutated-after-create"

	record, found := jobs.FindJob(ctx, "store-id")
	if !found || record.Data["status"] != jobStatusSubmitted {
		t.Fatalf("find by store id = %#v found=%v, want submitted", record.Data, found)
	}
	alias, found := jobs.FindJob(ctx, "logical-id")
	if !found || alias.ID != "store-id" {
		t.Fatalf("find by job_id alias = %#v found=%v, want store-id", alias, found)
	}
	if _, err := jobs.CreateSubmittedJob(ctx, map[string]any{"id": "store-id"}); !platform.IsCreateConflict(err) {
		t.Fatalf("duplicate create err = %v, want create conflict", err)
	}
}

func TestJobRepositoryDispatchCandidatesAndTransitions(t *testing.T) {
	app := platform.NewApp(platform.Config{ServiceName: serviceName})
	jobs := jobRepository(app)
	ctx := context.Background()
	now := time.Date(2026, 6, 15, 9, 0, 0, 0, time.UTC)
	old := now.Add(-time.Hour).Format(time.RFC3339)

	for _, row := range []map[string]any{
		{"id": "low", "status": jobStatusSubmitted, "priority_value": 10, "created_at": old},
		{"id": "high", "status": jobStatusSubmitted, "priority_value": 20, "created_at": old},
		{"id": "due-retry", "status": jobStatusWaitingInfra, "priority_value": 20, "next_retry_at": now.Add(-time.Minute).Format(time.RFC3339)},
		{"id": "future-retry", "status": jobStatusWaitingInfra, "priority_value": 30, "next_retry_at": now.Add(time.Hour).Format(time.RFC3339)},
		{"id": "done", "status": jobStatusCompleted, "priority_value": 100},
	} {
		createWorkloadRecord(t, app, jobsResource, row)
	}

	got := dispatchCandidateIDs(jobs.ListDispatchCandidates(ctx, now))
	want := []string{"high", "due-retry", "low"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("dispatch candidate order = %v, want %v", got, want)
	}

	if !jobs.MarkDispatchRunning(ctx, "low", jobDispatchRunningUpdate{
		At:               now,
		CreatedResources: []map[string]any{{"kind": "Pod", "name": "train"}},
	}) {
		t.Fatal("mark dispatch running returned false")
	}
	low := getJobRecord(t, app, "low")
	if low.Data["status"] != jobStatusRunning || low.Data["started_at"] == "" || low.Data["next_retry_at"] != nil {
		t.Fatalf("running record = %#v, want running with retry cleared", low.Data)
	}

	if !jobs.MarkDispatchFailed(ctx, "high", jobDispatchFailedUpdate{Reason: "bad manifest", CompletedAt: now}) {
		t.Fatal("mark dispatch failed returned false")
	}
	high := getJobRecord(t, app, "high")
	if high.Data["status"] != jobStatusFailed || high.Data["error_message"] != "bad manifest" {
		t.Fatalf("failed record = %#v, want failed reason", high.Data)
	}

	if !jobs.DeferForInfrastructureRecovery(ctx, "due-retry", jobInfrastructureRecoveryUpdate{
		RetryCount:  2,
		NextRetryAt: now.Add(time.Minute),
		Reason:      "cluster unavailable",
	}) {
		t.Fatal("defer for infrastructure returned false")
	}
	retry := getJobRecord(t, app, "due-retry")
	if retry.Data["status"] != jobStatusWaitingInfra || retry.Data["retry_count"] != 2 || retry.Data["completed_at"] != nil {
		t.Fatalf("retry record = %#v, want waiting_infra retry metadata", retry.Data)
	}
}

func TestJobRepositoryPreemptEvictAndActiveFailureGuards(t *testing.T) {
	app := platform.NewApp(platform.Config{ServiceName: serviceName})
	jobs := jobRepository(app)
	ctx := context.Background()
	now := time.Date(2026, 6, 15, 10, 0, 0, 0, time.UTC)

	createWorkloadRecord(t, app, jobsResource, map[string]any{"id": "running", "job_id": "run-logical", "status": jobStatusRunning})
	createWorkloadRecord(t, app, jobsResource, map[string]any{"id": "queued", "status": jobStatusQueued})
	createWorkloadRecord(t, app, jobsResource, map[string]any{"id": "completed", "status": jobStatusCompleted})
	createWorkloadRecord(t, app, jobsResource, map[string]any{"id": "active-fail", "job_id": "active-logical", "status": "active"})

	if _, ok := jobs.MarkPreempted(ctx, "running", jobPreemptionUpdate{
		PreemptionID: "pre-1",
		RequesterID:  "requester",
		Reason:       "higher priority",
		Cleanup:      map[string]any{"pods": 1},
		PreemptedAt:  now,
		CompletedAt:  now,
	}); !ok {
		t.Fatal("mark preempted returned false")
	}
	running := getJobRecord(t, app, "running")
	if running.Data["status"] != jobStatusPreempted ||
		running.Data["preemption_record_id"] != "pre-1" ||
		running.Data["preempted_by_job_id"] != "requester" {
		t.Fatalf("preempted record = %#v, want preemption metadata", running.Data)
	}

	if _, ok := jobs.MarkEvicted(ctx, "queued", jobEvictionUpdate{Reason: "plan expired", EvictedAt: now, CompletedAt: now}); !ok {
		t.Fatal("mark evicted returned false")
	}
	queued := getJobRecord(t, app, "queued")
	if queued.Data["status"] != jobStatusEvicted || queued.Data["status_reason"] != "plan expired" {
		t.Fatalf("evicted record = %#v, want eviction metadata", queued.Data)
	}

	if jobs.MarkFailedIfActive(ctx, "completed", "runtime exceeded") {
		t.Fatal("completed job should not be marked failed")
	}
	completed := getJobRecord(t, app, "completed")
	if completed.Data["status"] != jobStatusCompleted {
		t.Fatalf("completed record mutated = %#v", completed.Data)
	}

	if jobs.MarkFailedIfActive(ctx, "run-logical", "runtime exceeded") {
		t.Fatal("preempted alias should not be marked failed after terminal transition")
	}
	if !jobs.MarkFailedIfActive(ctx, "active-logical", "runtime exceeded") {
		t.Fatal("active job alias should be marked failed")
	}
	active := getJobRecord(t, app, "active-fail")
	if active.Data["status"] != jobStatusFailed || active.Data["status_reason"] != "runtime exceeded" {
		t.Fatalf("active-fail record = %#v, want failed runtime reason", active.Data)
	}
}

func TestJobRepositoryStaleAndLifecycleCandidates(t *testing.T) {
	app := platform.NewApp(platform.Config{ServiceName: serviceName})
	jobs := jobRepository(app)
	ctx := context.Background()
	now := time.Date(2026, 6, 15, 11, 0, 0, 0, time.UTC)
	old := now.Add(-10 * time.Minute).Format(time.RFC3339)

	for _, row := range []map[string]any{
		{"id": "stale-running", "status": jobStatusRunning, "created_at": old, "error_message": "old error"},
		{"id": "stale-queued", "status": jobStatusQueued, "created_at": old},
		{"id": "fresh-running", "status": jobStatusRunning, "created_at": now.Add(-time.Minute).Format(time.RFC3339)},
		{"id": "submitted", "status": jobStatusSubmitted},
		{"id": "evicted", "status": jobStatusEvicted},
		{"id": "preempted", "status": jobStatusPreempted},
	} {
		createWorkloadRecord(t, app, jobsResource, row)
	}

	if got := recordIDs(jobs.ListStaleJobCandidates(ctx, now)); strings.Join(got, ",") != "stale-queued,stale-running" {
		t.Fatalf("stale candidates = %v, want stale queued/running only", got)
	}

	lifecycleIDs := recordIDs(jobs.ListLifecycleReconcileCandidates(ctx))
	wantLifecycle := []string{"fresh-running", "stale-queued", "stale-running", "submitted"}
	if strings.Join(lifecycleIDs, ",") != strings.Join(wantLifecycle, ",") {
		t.Fatalf("lifecycle candidates = %v, want %v", lifecycleIDs, wantLifecycle)
	}

	record := getJobRecord(t, app, "stale-running")
	if !jobs.ApplyLifecycleObservation(ctx, record, cluster.JobLifecycle{Found: true, Status: jobStatusCompleted, Reason: "Complete"}, now) {
		t.Fatal("apply lifecycle observation returned false")
	}
	updated := getJobRecord(t, app, "stale-running")
	if updated.Data["status"] != jobStatusCompleted || updated.Data["completed_at"] == nil || updated.Data["error_message"] != "" {
		t.Fatalf("lifecycle-updated record = %#v, want completed with cleared error", updated.Data)
	}
}

func TestJobRepositorySourceGuardOwnsJobsResourceStoreAccess(t *testing.T) {
	pattern := regexp.MustCompile(`(?:\bStore\.|\bstore\.)(?:Get|List|Create|Update|Delete|NextID)\(.*jobsResource|jobsResource.*(?:\bStore\.|\bstore\.)(?:Get|List|Create|Update|Delete|NextID)\(`)
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	var violations []string
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || filepath.Ext(name) != ".go" || strings.HasSuffix(name, "_test.go") || name == "job_repository.go" {
			continue
		}
		raw, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		for i, line := range strings.Split(string(raw), "\n") {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "//") || !pattern.MatchString(trimmed) {
				continue
			}
			violations = append(violations, name+":"+strconv.Itoa(i+1)+": "+trimmed)
		}
	}
	if len(violations) > 0 {
		t.Fatalf("direct jobsResource store access outside job_repository.go:\n%s", strings.Join(violations, "\n"))
	}
}

func dispatchCandidateIDs(candidates []dispatchCandidate) []string {
	out := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		out = append(out, candidate.record.ID)
	}
	return out
}

func recordIDs(records []contracts.Record[map[string]any]) []string {
	out := make([]string, 0, len(records))
	for _, record := range records {
		out = append(out, record.ID)
	}
	sort.Strings(out)
	return out
}

func getJobRecord(t *testing.T, app *platform.App, id string) contracts.Record[map[string]any] {
	t.Helper()
	record, found := app.Store.Get(context.Background(), jobsResource, id)
	if !found {
		t.Fatalf("missing job %s", id)
	}
	return record
}
