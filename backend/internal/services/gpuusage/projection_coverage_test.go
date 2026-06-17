package gpuusage

import (
	"testing"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/contracts"
)

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
