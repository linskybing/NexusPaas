package storage

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/platform"
)

func TestFastTransferStartQueuesRecordAndDedupesIdempotencyKey(t *testing.T) {
	app := newStorageTestApp(t)
	body := `{"target_namespace":"project-P1","idempotency_key":"idem-1","bytes_total":4096}`
	req := storageProjectRequest(http.MethodPost, "/api/v1/projects/P1/storage/transfers/fast-stage", body, "U3", "P1")

	code, data, _ := startFastTransfer(app, req, platform.RouteSpec{})
	assertStorageStatus(t, code, data, http.StatusAccepted)
	first := data.(map[string]any)
	if first["status"] != fastTransferStatusQueued || first["progress_pct"] != defaultFastTransferProgressPct || first["bytes_done"] != int64(0) {
		t.Fatalf("started transfer = %#v, want queued v2 state", first)
	}
	if first["idempotency_key"] != "idem-1" || first[internalFastTransferIdempotencyKeyHash] == "" {
		t.Fatalf("idempotency fields = %#v, want stored key and internal hash", first)
	}

	req = storageProjectRequest(http.MethodPost, "/api/v1/projects/P1/storage/transfers/fast-stage", body, "U3", "P1")
	code, data, _ = startFastTransfer(app, req, platform.RouteSpec{})
	assertStorageStatus(t, code, data, http.StatusAccepted)
	replay := data.(map[string]any)
	if replay["id"] != first["id"] {
		t.Fatalf("idempotent replay id = %v, want first id %v", replay["id"], first["id"])
	}
	if rows := app.Store.List(context.Background(), fastTransfersResource); len(rows) != 1 {
		t.Fatalf("fast transfer records = %#v, want one idempotent record", rows)
	}

	conflict := storageProjectRequest(http.MethodPost, "/api/v1/projects/P1/storage/transfers/fast-stage", `{"name":"different","target_namespace":"project-P1","idempotency_key":"idem-1"}`, "U3", "P1")
	code, _, _ = startFastTransfer(app, conflict, platform.RouteSpec{})
	assertStorageStatus(t, code, nil, http.StatusConflict)
}

func TestFastTransferProgressTransitionsAndEvents(t *testing.T) {
	app := newFastTransferProgressTestApp(t)
	seedFastTransferStateRecord(t, app, map[string]any{
		"id":               fastTransferID("P1", "project-P1", "copy1"),
		"project_id":       "P1",
		"target_namespace": "project-P1",
		"name":             "copy1",
		"status":           fastTransferStatusQueued,
		"progress_pct":     0,
		"bytes_done":       int64(0),
	})

	req := fastTransferProgressRequest(`{"status":"running","progress_pct":25,"bytes_done":1024,"bytes_total":4096}`, "service-key")
	code, data, _ := updateFastTransferProgress(app, req, platform.RouteSpec{})
	assertStorageStatus(t, code, data, http.StatusOK)
	running := data.(map[string]any)
	if running["status"] != fastTransferStatusRunning || running["progress_pct"] != 25 || running["bytes_done"] != int64(1024) {
		t.Fatalf("running transfer = %#v, want progress update", running)
	}
	assertFastTransferEventSeen(t, app, fastTransferProgressedEvent)

	req = fastTransferProgressRequest(`{"status":"succeeded","bytes_done":4096,"checksum":"sha256:abc"}`, "service-key")
	code, data, _ = updateFastTransferProgress(app, req, platform.RouteSpec{})
	assertStorageStatus(t, code, data, http.StatusOK)
	done := data.(map[string]any)
	if done["status"] != fastTransferStatusSucceeded || done["progress_pct"] != 100 || done["checksum"] != "sha256:abc" {
		t.Fatalf("completed transfer = %#v, want succeeded with checksum", done)
	}
	assertFastTransferEventSeen(t, app, fastTransferCompletedEvent)

	req = fastTransferProgressRequest(`{"status":"running","progress_pct":100,"bytes_done":4096}`, "service-key")
	code, _, _ = updateFastTransferProgress(app, req, platform.RouteSpec{})
	assertStorageStatus(t, code, nil, http.StatusConflict)
}

func TestFastTransferProgressRejectsDecreasingProgressAndBytes(t *testing.T) {
	current := map[string]any{"status": fastTransferStatusRunning, "progress_pct": 50, "bytes_done": int64(200)}
	_, _, status, err := fastTransferProgressPatchFromPayload(current, map[string]any{"progress_pct": 40, "bytes_done": int64(250)}, time.Now())
	if status != http.StatusConflict || err == nil {
		t.Fatalf("decreasing progress status=%d err=%v, want conflict", status, err)
	}
	_, _, status, err = fastTransferProgressPatchFromPayload(current, map[string]any{"progress_pct": 60, "bytes_done": int64(100)}, time.Now())
	if status != http.StatusConflict || err == nil {
		t.Fatalf("decreasing bytes status=%d err=%v, want conflict", status, err)
	}
}

func TestFastTransferCancelRejectsTerminalStatuses(t *testing.T) {
	app := newStorageTestApp(t)
	seedFastTransferStateRecord(t, app, map[string]any{
		"id":               fastTransferID("P1", "project-P1", "done"),
		"project_id":       "P1",
		"target_namespace": "project-P1",
		"name":             "done",
		"status":           fastTransferStatusSucceeded,
	})
	cancelReq := storageProjectRequest(http.MethodDelete, "/api/v1/projects/P1/storage/transfers/project-P1/done", "", "U3", "P1")
	cancelReq.SetPathValue("targetNamespace", "project-P1")
	cancelReq.SetPathValue("name", "done")

	code, _, _ := cancelFastTransfer(app, cancelReq, platform.RouteSpec{})
	assertStorageStatus(t, code, nil, http.StatusConflict)
}

func TestFastTransferProgressRequiresServiceKey(t *testing.T) {
	app := newFastTransferProgressTestApp(t)
	req := fastTransferProgressRequest(`{"status":"running"}`, "wrong-key")

	code, data, _ := updateFastTransferProgress(app, req, platform.RouteSpec{})
	if code != http.StatusUnauthorized || data == nil {
		t.Fatalf("status=%d data=%#v, want unauthorized", code, data)
	}
}

func TestFastTransferProgressAcceptsScopedServiceIdentity(t *testing.T) {
	app := newStorageTestApp(t)
	app.Config.ServiceTrustedIdentities = map[string]platform.ServiceTrustedIdentity{
		"k8s-control-service": {Key: "scoped-key", Audiences: []string{"storage-service"}},
	}
	seedFastTransferStateRecord(t, app, map[string]any{
		"id":               fastTransferID("P1", "project-P1", "copy1"),
		"project_id":       "P1",
		"target_namespace": "project-P1",
		"name":             "copy1",
		"status":           fastTransferStatusQueued,
		"progress_pct":     0,
	})

	req := fastTransferProgressRequest(`{"status":"running","progress_pct":1}`, "scoped-key")
	req.Header.Set("X-Service-Name", "k8s-control-service")
	code, data, _ := updateFastTransferProgress(app, req, platform.RouteSpec{})
	assertStorageStatus(t, code, data, http.StatusOK)
	if got := data.(map[string]any)["status"]; got != fastTransferStatusRunning {
		t.Fatalf("status = %v, want running", got)
	}

	req = fastTransferProgressRequest(`{"status":"running","progress_pct":2}`, "wrong-key")
	req.Header.Set("X-Service-Name", "k8s-control-service")
	code, data, _ = updateFastTransferProgress(app, req, platform.RouteSpec{})
	if code != http.StatusUnauthorized || data == nil {
		t.Fatalf("status=%d data=%#v, want unauthorized for wrong scoped key", code, data)
	}
}

func TestFastTransferProgressRejectsWrongScopedAudience(t *testing.T) {
	app := newStorageTestApp(t)
	app.Config.ServiceTrustedIdentities = map[string]platform.ServiceTrustedIdentity{
		"k8s-control-service": {Key: "scoped-key", Audiences: []string{"workload-service"}},
	}
	req := fastTransferProgressRequest(`{"status":"running","progress_pct":1}`, "scoped-key")
	req.Header.Set("X-Service-Name", "k8s-control-service")

	code, data, _ := updateFastTransferProgress(app, req, platform.RouteSpec{})
	if code != http.StatusUnauthorized || data == nil {
		t.Fatalf("status=%d data=%#v, want unauthorized for wrong scoped audience", code, data)
	}
}

func newFastTransferProgressTestApp(t *testing.T) *platform.App {
	t.Helper()
	app := newStorageTestApp(t)
	app.Config.ServiceAPIKey = "service-key"
	return app
}

func fastTransferProgressRequest(body, serviceKey string) *http.Request {
	req := storageProjectRequest(http.MethodPost, "/internal/storage/projects/P1/transfers/project-P1/copy1/progress", body, "", "P1")
	req.SetPathValue("project_id", "P1")
	req.SetPathValue("targetNamespace", "project-P1")
	req.SetPathValue("name", "copy1")
	if serviceKey != "" {
		req.Header.Set("X-Service-Key", serviceKey)
	}
	return req
}

func seedFastTransferStateRecord(t *testing.T, app *platform.App, row map[string]any) {
	t.Helper()
	if _, err := app.Store.Create(context.Background(), fastTransfersResource, row); err != nil {
		t.Fatal(err)
	}
}

func assertFastTransferEventSeen(t *testing.T, app *platform.App, name string) {
	t.Helper()
	for _, event := range app.Events.Outbox() {
		if event.Name == name {
			return
		}
	}
	t.Fatalf("events = %#v, want %s", app.Events.Outbox(), name)
}
