package schedulerquota

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/platform"
	"github.com/linskybing/nexuspaas/backend/internal/services/workload"
)

func TestWorkloadEvictionClientLocalAndRemote(t *testing.T) {
	ctx := context.Background()
	localOwner := newWorkloadEvictionOwnerTestApp(t, "all")
	seedEvictionJob(t, localOwner, "local-job")
	localClient, err := newWorkloadEvictionClient(localOwner)
	if err != nil {
		t.Fatalf("new local eviction client: %v", err)
	}
	if err := localClient(ctx, "local-job", workloadEvictRequest{Reason: "plan window closed"}); err != nil {
		t.Fatalf("local Evict: %v", err)
	}
	assertEvictedJob(t, localOwner, "local-job", "plan window closed")

	remoteOwner := newWorkloadEvictionOwnerTestApp(t, workloadServiceName)
	seedEvictionJob(t, remoteOwner, "remote-job")
	server := httptest.NewServer(remoteOwner)
	defer server.Close()
	remoteConsumer := platform.NewApp(platform.Config{
		ServiceName:    serviceName,
		ServiceAPIKey:  "svc-key",
		ServiceURLs:    map[string]string{workloadServiceName: server.URL},
		AdapterTimeout: time.Second,
	})
	remoteClient, err := newWorkloadEvictionClient(remoteConsumer)
	if err != nil {
		t.Fatalf("new remote eviction client: %v", err)
	}
	if err := remoteClient(ctx, "remote-job", workloadEvictRequest{Reason: "plan expired"}); err != nil {
		t.Fatalf("remote Evict: %v", err)
	}
	assertEvictedJob(t, remoteOwner, "remote-job", "plan expired")
	if err := remoteClient(ctx, "missing", workloadEvictRequest{Reason: "missing"}); err == nil || !strings.Contains(err.Error(), "HTTP 404") {
		t.Fatalf("remote missing Evict err = %v, want HTTP 404", err)
	}
}

func TestWorkloadEvictionClientConfigAndEndpointValidation(t *testing.T) {
	if _, err := newWorkloadEvictionClient(platform.NewApp(platform.Config{ServiceName: serviceName})); err == nil {
		t.Fatal("isolated eviction client without workload URL err = nil")
	}
	if _, err := newWorkloadEvictionClient(platform.NewApp(platform.Config{
		ServiceName: serviceName,
		ServiceURLs: map[string]string{workloadServiceName: "http://workload.local"},
	})); err == nil {
		t.Fatal("isolated eviction client without service key err = nil")
	}
}

func newWorkloadEvictionOwnerTestApp(t *testing.T, serviceName string) *platform.App {
	t.Helper()
	app := platform.NewApp(platform.Config{ServiceName: serviceName, HTTPAddr: ":0", ServiceAPIKey: "svc-key"})
	app.RegisterService(platform.ServiceSpec{Name: workloadServiceName, Routes: []platform.RouteSpec{
		{Method: http.MethodPost, Pattern: workloadEvictJobPathTemplate, Resource: "jobs", Action: "evict", AuthRequired: false},
	}})
	workload.Register(app)
	return app
}

func seedEvictionJob(t *testing.T, app *platform.App, id string) {
	t.Helper()
	if _, err := app.Store.Create(context.Background(), workloadJobsResource, map[string]any{
		"id":        id,
		"job_id":    id,
		"status":    "running",
		"namespace": "proj-p1",
	}); err != nil {
		t.Fatal(err)
	}
}

func assertEvictedJob(t *testing.T, app *platform.App, id, reason string) {
	t.Helper()
	record, found := app.Store.Get(context.Background(), workloadJobsResource, id)
	if !found {
		t.Fatalf("job %s not found", id)
	}
	if record.Data["status"] != "evicted" || record.Data["status_reason"] != reason {
		t.Fatalf("job %s = %#v, want evicted with reason %q", id, record.Data, reason)
	}
}
