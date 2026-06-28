package storage

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/linskybing/nexuspaas/backend/internal/platform"
)

func TestFastTransferDispatchSubmitsConfiguredMoverJob(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path != "/internal/k8s-control/fast-transfers/mover-jobs" {
			t.Fatalf("path = %q, want mover job path", r.URL.Path)
		}
		if r.Header.Get("X-Service-Name") != serviceName || r.Header.Get("X-Service-Key") != "scoped-key" {
			t.Fatalf("service headers name=%q key=%q", r.Header.Get("X-Service-Name"), r.Header.Get("X-Service-Key"))
		}
		var req fastTransferMoverDispatchRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatal(err)
		}
		if req.TransferID == "" || req.Source.PVC != "dataset-pvc" || req.Target.PVC != "scratch-pvc" || req.Tool != "rsync" {
			t.Fatalf("dispatch request = %#v, want transfer/source/target/tool", req)
		}
		platform.WriteJSON(w, r, http.StatusCreated, map[string]any{
			"namespace": "project-p1",
			"name":      "fast-transfer-copy1",
			"action":    "created",
		})
	}))
	defer server.Close()

	app := newStorageTestApp(t)
	configureFastTransferDispatchApp(t, app)
	app.Config.ServiceURLs = map[string]string{k8sControlServiceName: server.URL}

	code, data, _ := startFastTransfer(app, storageProjectRequest(http.MethodPost, "/api/v1/projects/P1/storage/transfers/fast-stage", fastTransferDispatchBody("idem-1"), "U3", "P1"), platform.RouteSpec{})
	assertStorageStatus(t, code, data, http.StatusAccepted)
	row := data.(map[string]any)
	if row["status"] != fastTransferStatusQueued || row["dispatch_status"] != fastTransferDispatchSubmitted {
		t.Fatalf("transfer = %#v, want queued submitted", row)
	}
	if row["mover_job_namespace"] != "project-p1" || row["mover_job_name"] != "fast-transfer-copy1" {
		t.Fatalf("mover metadata = %#v", row)
	}
	if calls != 1 {
		t.Fatalf("dispatch calls = %d, want 1", calls)
	}
}

func TestFastTransferDispatchNotConfiguredStillAccepts(t *testing.T) {
	app := newStorageTestApp(t)
	configureFastTransferDispatchApp(t, app)

	code, data, _ := startFastTransfer(app, storageProjectRequest(http.MethodPost, "/api/v1/projects/P1/storage/transfers/fast-stage", fastTransferDispatchBody("idem-2"), "U3", "P1"), platform.RouteSpec{})
	assertStorageStatus(t, code, data, http.StatusAccepted)
	row := data.(map[string]any)
	if row["status"] != fastTransferStatusQueued || row["dispatch_status"] != fastTransferDispatchNotConfigured {
		t.Fatalf("transfer = %#v, want queued not_configured", row)
	}
}

func TestFastTransferDispatchDoesNotReplayForIdempotentRequest(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		platform.WriteJSON(w, r, http.StatusCreated, map[string]any{
			"namespace": "project-p1",
			"name":      "fast-transfer-copy1",
			"action":    "created",
		})
	}))
	defer server.Close()

	app := newStorageTestApp(t)
	configureFastTransferDispatchApp(t, app)
	app.Config.ServiceURLs = map[string]string{k8sControlServiceName: server.URL}
	body := fastTransferDispatchBody("idem-3")

	code, _, _ := startFastTransfer(app, storageProjectRequest(http.MethodPost, "/api/v1/projects/P1/storage/transfers/fast-stage", body, "U3", "P1"), platform.RouteSpec{})
	assertStorageStatus(t, code, nil, http.StatusAccepted)
	code, _, _ = startFastTransfer(app, storageProjectRequest(http.MethodPost, "/api/v1/projects/P1/storage/transfers/fast-stage", body, "U3", "P1"), platform.RouteSpec{})
	assertStorageStatus(t, code, nil, http.StatusAccepted)
	if calls != 1 {
		t.Fatalf("dispatch calls = %d, want one call for new record only", calls)
	}
}

func fastTransferDispatchBody(idempotencyKey string) string {
	return `{
		"name":"copy1",
		"target_namespace":"project-p1",
		"idempotency_key":"` + idempotencyKey + `",
		"source":{"namespace":"project-p1","pvc":"dataset-pvc","path":"/data/source"},
		"target":{"namespace":"project-p1","pvc":"scratch-pvc","path":"/data/target"}
	}`
}

func configureFastTransferDispatchApp(t *testing.T, app *platform.App) {
	t.Helper()
	app.Config.ServiceName = serviceName
	app.Config.ServiceIdentityName = serviceName
	app.Config.ServiceIdentityKey = "scoped-key"
	createStorageRecords(t, app, storageProjectsResource, []map[string]any{
		{"id": "P1", "project_name": "vision", "owner_id": "G1"},
	})
	createStorageRecords(t, app, storageProjectMembersResource, []map[string]any{
		{"id": "P1:U3", "project_id": "P1", "user_id": "U3", "role": "manager"},
	})
}
