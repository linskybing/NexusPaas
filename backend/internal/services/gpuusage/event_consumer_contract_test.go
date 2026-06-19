package gpuusage

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/linskybing/nexuspaas/backend/internal/contracts"
	"github.com/linskybing/nexuspaas/backend/internal/platform"
)

func TestGPUUsageConsumerMatchesJobSubmittedFixture(t *testing.T) {
	fixture := readGPUUsageEventFixture(t, "job-submitted.json")
	app := platform.NewApp(platform.Config{ServiceName: serviceName})
	req := httptest.NewRequest(http.MethodGet, "/", nil)

	if err := projectGPUUsageEvent(app, req, gpuUsageEventFromFixture(fixture)); err != nil {
		t.Fatalf("projectGPUUsageEvent: %v", err)
	}

	jobID := fixture.Payload["job_id"].(string)
	record, ok := app.Store.Get(context.Background(), gpuJobsResource, jobID)
	if !ok {
		t.Fatalf("missing projected GPU job %q", jobID)
	}
	if record.ID != jobID || record.Data["id"] != jobID {
		t.Fatalf("projected id = %q/%#v, want %q", record.ID, record.Data["id"], jobID)
	}
	if record.Data["status"] != "submitted" {
		t.Fatalf("projected status = %#v, want submitted", record.Data["status"])
	}
	for _, key := range []string{
		"job_id",
		"project_id",
		"user_id",
		"config_commit_id",
		"image_ref",
		"requested_resources",
	} {
		assertGPUUsagePayloadField(t, record.Data, fixture.Payload, key)
	}
	if got := len(app.Store.List(context.Background(), workloadJobsResource)); got != 0 {
		t.Fatalf("source workload jobs = %d, want isolated consumer to avoid owner store", got)
	}
}

func readGPUUsageEventFixture(t *testing.T, name string) contracts.EventEnvelope {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "contracts", "fixtures", "events", "v1", name))
	if err != nil {
		t.Fatalf("read event fixture %s: %v", name, err)
	}
	fixture, err := contracts.DecodeEventEnvelope(raw)
	if err != nil {
		t.Fatalf("decode event fixture %s: %v", name, err)
	}
	return fixture
}

func gpuUsageEventFromFixture(fixture contracts.EventEnvelope) contracts.Event {
	return contracts.Event{
		EventID:       fixture.EventID,
		Name:          fixture.EventType,
		Source:        fixture.Producer,
		OccurredAt:    fixture.OccurredAt,
		TraceID:       fixture.TraceID,
		SchemaVersion: fixture.SchemaVersion,
		Data:          cloneGPUUsagePayload(fixture.Payload),
	}
}

func cloneGPUUsagePayload(in map[string]any) map[string]any {
	out := make(map[string]any, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func assertGPUUsagePayloadField(t *testing.T, got, want map[string]any, key string) {
	t.Helper()
	if !reflect.DeepEqual(got[key], want[key]) {
		t.Fatalf("projected payload[%s] = %#v, want %#v", key, got[key], want[key])
	}
}
