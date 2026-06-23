package gpuusage

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/contracts"
	"github.com/linskybing/nexuspaas/backend/internal/platform"
)

func TestGPUProjectionDriftDetectsMissingOrphanStaleCleanAndSorts(t *testing.T) {
	ctx := context.Background()
	app := platform.NewApp(platform.Config{ServiceName: "all"})

	seedGPUProjectionRecord(t, ctx, app, gpuProjectsResource, map[string]any{"id": "project-orphan", "project_id": "project-orphan", "name": "Orphan"})
	seedGPUProjectionRecord(t, ctx, app, orgProjectsResource, map[string]any{"id": "project-stale", "project_id": "project-stale", "name": "Source"})
	seedGPUProjectionRecord(t, ctx, app, gpuProjectsResource, map[string]any{"id": "project-stale", "project_id": "project-stale", "name": "Local"})
	seedGPUProjectionRecord(t, ctx, app, orgProjectsResource, map[string]any{"id": "project-clean", "project_id": "project-clean", "name": "Clean"})
	seedGPUProjectionRecord(t, ctx, app, gpuProjectsResource, map[string]any{"id": "project-clean", "project_id": "project-clean", "name": "Clean"})

	seedGPUProjectionRecord(t, ctx, app, gpuJobsResource, map[string]any{"id": "job-orphan", "job_id": "job-orphan", "status": "running"})
	seedGPUProjectionRecord(t, ctx, app, workloadJobsResource, map[string]any{"id": "job-missing", "job_id": "job-missing", "status": "queued"})

	seedGPUProjectionRecord(t, ctx, app, authorizationRolesResource, map[string]any{"id": "role-clean", "role_id": "role-clean", "name": "viewer"})
	seedGPUProjectionRecord(t, ctx, app, gpuAuthorizationRolesResource, map[string]any{"id": "role-clean", "role_id": "role-clean", "name": "viewer"})
	seedGPUProjectionRecord(t, ctx, app, gpuAuthorizationRolesResource, map[string]any{"id": "role-stale", "role_id": "role-stale", "name": "local-admin"})
	seedGPUProjectionRecord(t, ctx, app, authorizationRolesResource, map[string]any{"id": "role-stale", "role_id": "role-stale", "name": "source-admin"})

	seedGPUProjectionRecord(t, ctx, app, gpuIdentityRolesResource, map[string]any{"id": "role-orphan", "role_id": "role-orphan", "name": "orphan"})
	seedGPUProjectionRecord(t, ctx, app, identityUsersResource, map[string]any{"id": "user-missing", "user_id": "user-missing", "username": "missing"})

	blankSourceID := seedGPUProjectionRecord(t, ctx, app, workloadJobsResource, map[string]any{"id": "blank-source-record", "status": "ignored"})
	updateGPUProjectionRecord(t, ctx, app, workloadJobsResource, blankSourceID, map[string]any{"id": ""})
	blankLocalID := seedGPUProjectionRecord(t, ctx, app, gpuJobsResource, map[string]any{"id": "blank-local-record", "status": "ignored"})
	updateGPUProjectionRecord(t, ctx, app, gpuJobsResource, blankLocalID, map[string]any{"id": ""})

	report, err := projectionDrift(ctx, app)
	if err != nil {
		t.Fatalf("projectionDrift returned error: %v", err)
	}

	wantMissing := []gpuProjectionDriftFinding{
		{SourceResource: identityUsersResource, LocalResource: gpuIdentityUsersResource, ID: "user-missing"},
		{SourceResource: workloadJobsResource, LocalResource: gpuJobsResource, ID: "job-missing"},
	}
	if !reflect.DeepEqual(report.Missing, wantMissing) {
		t.Fatalf("missing findings = %#v, want %#v", report.Missing, wantMissing)
	}

	wantOrphan := []gpuProjectionDriftFinding{
		{SourceResource: identityRolesResource, LocalResource: gpuIdentityRolesResource, ID: "role-orphan"},
		{SourceResource: workloadJobsResource, LocalResource: gpuJobsResource, ID: "job-orphan"},
		{SourceResource: orgProjectsResource, LocalResource: gpuProjectsResource, ID: "project-orphan"},
	}
	if !reflect.DeepEqual(report.Orphan, wantOrphan) {
		t.Fatalf("orphan findings = %#v, want %#v", report.Orphan, wantOrphan)
	}

	wantStale := []gpuProjectionDriftFinding{
		{SourceResource: authorizationRolesResource, LocalResource: gpuAuthorizationRolesResource, ID: "role-stale"},
		{SourceResource: orgProjectsResource, LocalResource: gpuProjectsResource, ID: "project-stale"},
	}
	if !reflect.DeepEqual(report.Stale, wantStale) {
		t.Fatalf("stale findings = %#v, want %#v", report.Stale, wantStale)
	}
}

func TestGPUProjectionDriftNormalizesCanonicalID(t *testing.T) {
	ctx := context.Background()
	app := platform.NewApp(platform.Config{})

	sourceJobRecordID := seedGPUProjectionRecord(t, ctx, app, workloadJobsResource, map[string]any{
		"id":     "source-record-job",
		"job_id": "job-normalized",
		"status": "running",
	})
	updateGPUProjectionRecord(t, ctx, app, workloadJobsResource, sourceJobRecordID, map[string]any{
		"id":     "",
		"job_id": "job-normalized",
		"status": "running",
	})
	seedGPUProjectionRecord(t, ctx, app, gpuJobsResource, map[string]any{
		"id":     "job-normalized",
		"job_id": "job-normalized",
		"status": "running",
	})

	sourceProjectRecordID := seedGPUProjectionRecord(t, ctx, app, orgProjectsResource, map[string]any{
		"id":         "source-record-project",
		"project_id": "project-normalized",
		"name":       "Normalized",
	})
	updateGPUProjectionRecord(t, ctx, app, orgProjectsResource, sourceProjectRecordID, map[string]any{
		"id":         "",
		"project_id": "project-normalized",
		"name":       "Normalized",
	})
	seedGPUProjectionRecord(t, ctx, app, gpuProjectsResource, map[string]any{
		"id":         "project-normalized",
		"project_id": "project-normalized",
		"name":       "Normalized",
	})

	report, err := projectionDrift(ctx, app)
	if err != nil {
		t.Fatalf("projectionDrift returned error: %v", err)
	}
	if len(report.Missing) != 0 || len(report.Orphan) != 0 || len(report.Stale) != 0 {
		t.Fatalf("projectionDrift with canonical ids = %#v, want no findings", report)
	}
}

func TestGPUProjectionDriftNilAppOrStoreFailsClosed(t *testing.T) {
	ctx := context.Background()
	for _, app := range []*platform.App{nil, {}} {
		if _, err := projectionDrift(ctx, app); !errors.Is(err, errGPUProjectionDriftUnavailable) {
			t.Fatalf("projectionDrift(%#v) error = %v, want %v", app, err, errGPUProjectionDriftUnavailable)
		}
	}
}

func TestGPUProjectionDriftPairsCoverExpectedResources(t *testing.T) {
	want := map[string]string{
		identityUsersResource:      gpuIdentityUsersResource,
		identityRolesResource:      gpuIdentityRolesResource,
		authorizationRolesResource: gpuAuthorizationRolesResource,
		orgProjectsResource:        gpuProjectsResource,
		workloadJobsResource:       gpuJobsResource,
	}
	if len(gpuProjectionDriftPairs) != len(want) {
		t.Fatalf("gpuProjectionDriftPairs length = %d, want %d", len(gpuProjectionDriftPairs), len(want))
	}

	got := map[string]string{}
	for _, pair := range gpuProjectionDriftPairs {
		if pair.idFn == nil {
			t.Fatalf("gpuProjectionDriftPairs contains nil idFn for %s -> %s", pair.sourceResource, pair.localResource)
		}
		if pair.sourceResource == snapshotsResource || pair.localResource == snapshotsResource {
			t.Fatalf("gpuProjectionDriftPairs includes snapshotsResource: %#v", pair)
		}
		if pair.sourceResource == summariesResource || pair.localResource == summariesResource {
			t.Fatalf("gpuProjectionDriftPairs includes summariesResource: %#v", pair)
		}
		got[pair.sourceResource] = pair.localResource
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("gpuProjectionDriftPairs = %#v, want %#v", got, want)
	}
}

func TestGPUProjectionStatusMergeAndHelperEdges(t *testing.T) {
	for _, tc := range []struct {
		event string
		want  string
	}{
		{event: "JobQueued", want: "queued"},
		{event: "JobRunning", want: "running"},
		{event: "JobSucceeded", want: "succeeded"},
		{event: "JobFailed", want: "failed"},
		{event: "JobCancelled", want: "cancelled"},
		{event: "JobSubmitted", want: "submitted"},
	} {
		if got := statusForJobEvent(tc.event); got != tc.want {
			t.Fatalf("statusForJobEvent(%s) = %q, want %q", tc.event, got, tc.want)
		}
	}

	local := []contracts.Record[map[string]any]{
		{ID: "local", Data: map[string]any{"job_id": "J1", "status": "running"}},
	}
	source := []contracts.Record[map[string]any]{
		{ID: "source-duplicate", Data: map[string]any{"job_id": "J1", "status": "queued"}},
		{ID: "source-new", Data: map[string]any{"job_id": "J2", "status": "succeeded"}},
		{ID: "source-no-id", Data: map[string]any{"status": "unknown"}},
	}
	merged := mergeGPURecords(gpuJobsResource, source, local)
	if len(merged) != 3 || merged[0].ID != "local" || merged[1].ID != "source-new" || merged[2].ID != "source-no-id" {
		t.Fatalf("merged GPU records = %#v, want local plus non-duplicate source records", merged)
	}

	if got := normalizedUtilizationPercent(0.42); got != 42 {
		t.Fatalf("normalizedUtilizationPercent fraction = %v, want 42", got)
	}
	if got := normalizedUtilizationPercent(75); got != 75 {
		t.Fatalf("normalizedUtilizationPercent percent = %v, want 75", got)
	}
	if !hasAnyKey(map[string]any{"gpu": true}, "missing", "gpu") {
		t.Fatal("hasAnyKey did not find present key")
	}
	if hasAnyKey(map[string]any{"gpu": true}, "missing") {
		t.Fatal("hasAnyKey found absent key")
	}
}

func TestGPUCollectorSummaryMathEdges(t *testing.T) {
	base := time.Date(2026, time.June, 17, 12, 0, 0, 0, time.UTC)
	if got := medianInterval([]time.Time{base, base, base.Add(5 * time.Second), base.Add(20 * time.Second)}); got != 15*time.Second {
		t.Fatalf("medianInterval with duplicate buckets = %s, want 15s", got)
	}
	if got := medianInterval([]time.Time{base}); got != time.Second {
		t.Fatalf("medianInterval single bucket = %s, want 1s", got)
	}
	if got := proportionalSeconds(100, 0, 0); got != 0 {
		t.Fatalf("proportionalSeconds zero samples = %v, want 0", got)
	}
	if got := averageFloat(10, 0); got != 0 {
		t.Fatalf("averageFloat zero count = %v, want 0", got)
	}
	if got := averageInt64(10, 0); got != 0 {
		t.Fatalf("averageInt64 zero count = %v, want 0", got)
	}
	if got := averageMemoryMB(2*1024*1024, 2); got != 1 {
		t.Fatalf("averageMemoryMB = %v, want 1", got)
	}
}

func seedGPUProjectionRecord(t *testing.T, ctx context.Context, app *platform.App, resource string, data map[string]any) string {
	t.Helper()
	record, err := app.Store.Create(ctx, resource, data)
	if err != nil {
		t.Fatalf("create %s record: %v", resource, err)
	}
	return record.ID
}

func updateGPUProjectionRecord(t *testing.T, ctx context.Context, app *platform.App, resource, id string, data map[string]any) {
	t.Helper()
	if _, ok := app.Store.Update(ctx, resource, id, data); !ok {
		t.Fatalf("update %s/%s missed", resource, id)
	}
}
