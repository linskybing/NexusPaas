//go:build e2e

package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/contracts"
	"github.com/linskybing/nexuspaas/backend/internal/platform"
	"github.com/linskybing/nexuspaas/backend/internal/platform/cluster"
	storageservice "github.com/linskybing/nexuspaas/backend/internal/services/storage"
)

const fastTransferStartMoverKindAdmissionEnv = "TEST_LIVE_FAST_TRANSFER_START_MOVER_KIND_ADMISSION"

func TestFastTransferStartMoverKindAdmissionE2E(t *testing.T) {
	if strings.TrimSpace(os.Getenv(fastTransferStartMoverKindAdmissionEnv)) != "1" {
		t.Skip("set " + fastTransferStartMoverKindAdmissionEnv + "=1 to run live FastTransfer start-to-mover kind admission e2e")
	}
	requireLiveKubeconfig(t)

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	cl, err := cluster.NewFromEnv("proj")
	if err != nil {
		t.Fatalf("create live Kubernetes client: %v", err)
	}
	if cl == nil {
		t.Fatal("live Kubernetes client is unavailable")
	}
	if err := cl.Ping(ctx); err != nil {
		t.Fatalf("ping live Kubernetes cluster: %v", err)
	}

	suffix := strconv.FormatInt(time.Now().UTC().UnixNano(), 36)
	namespace := "ft-start-" + suffix
	createFastTransferMoverAdmissionNamespace(t, ctx, cl, namespace)

	k8sServer := httptest.NewServer(newFastTransferMoverAdmissionApp(cl))
	t.Cleanup(k8sServer.Close)

	store := platform.NewStore()
	events := platform.NewEventBus()
	projectID, userID := "p-"+suffix, "u-"+suffix
	seedFastTransferStartAccess(t, store, projectID, userID)
	storageServer := httptest.NewServer(newFastTransferStartStorageApp(store, events, k8sServer.URL))
	t.Cleanup(storageServer.Close)

	name := "copy-" + suffix
	transferID := projectID + ":" + namespace + ":" + name
	payload := fastTransferStartRequest(namespace, name, "idem-"+suffix)

	first := postFastTransferStart(t, storageServer.URL, projectID, userID, payload)
	assertFastTransferStartRecord(t, first, transferID, namespace)
	assertFastTransferStartQueuedEvent(t, events, transferID)
	job := getFastTransferMoverAdmissionJob(t, ctx, cl, textE2E(first["mover_job_namespace"]), textE2E(first["mover_job_name"]))
	assertFastTransferMoverAdmissionJob(t, job, namespace, textE2E(first["mover_job_name"]), transferID)

	replay := postFastTransferStart(t, storageServer.URL, projectID, userID, payload)
	if textE2E(replay["id"]) != transferID || textE2E(replay["mover_job_name"]) != textE2E(first["mover_job_name"]) {
		t.Fatalf("replay = %#v, want same transfer and mover job as %#v", replay, first)
	}
	assertFastTransferMoverAdmissionJobCount(t, ctx, cl, namespace, transferID, 1)
}

func newFastTransferStartStorageApp(store *platform.Store, events *platform.EventBus, k8sControlURL string) *platform.App {
	app := platform.NewApp(platform.Config{
		ServiceName:             "storage-service",
		HTTPAddr:                ":0",
		RequireAuth:             false,
		ServiceFallbackDisabled: true,
		ServiceAPIKey:           "fast-transfer-e2e-service-key",
		ServiceURLs:             map[string]string{"k8s-control-service": k8sControlURL},
	}, platform.WithStore(store), platform.WithEventBus(events))
	app.RegisterService(storageservice.Spec())
	storageservice.Register(app)
	return app
}

func seedFastTransferStartAccess(t *testing.T, store *platform.Store, projectID, userID string) {
	t.Helper()
	createFastTransferStartRecord(t, store, "storage-service:storage_projects", map[string]any{
		"id":           projectID,
		"project_id":   projectID,
		"project_name": "fast-transfer-e2e",
	})
	createFastTransferStartRecord(t, store, "storage-service:storage_project_members", map[string]any{
		"id":         projectID + ":" + userID,
		"project_id": projectID,
		"user_id":    userID,
		"role":       "manager",
	})
}

func createFastTransferStartRecord(t *testing.T, store *platform.Store, resource string, data map[string]any) {
	t.Helper()
	if _, err := store.Create(context.Background(), resource, data); err != nil {
		t.Fatalf("seed %s: %v", resource, err)
	}
}

func fastTransferStartRequest(namespace, name, idempotencyKey string) map[string]any {
	return map[string]any{
		"name":              name,
		"target_namespace":  namespace,
		"idempotency_key":   idempotencyKey,
		"source":            map[string]any{"namespace": namespace, "pvc": "source-pvc", "path": "/data/source"},
		"target":            map[string]any{"namespace": namespace, "pvc": "target-pvc", "path": "/data/target"},
		"tool":              "rsync",
		"progress_callback": map[string]any{"path": "/unused-by-admission-e2e"},
	}
}

func postFastTransferStart(t *testing.T, baseURL, projectID, userID string, body map[string]any) map[string]any {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal storage request: %v", err)
	}
	req, err := http.NewRequest(http.MethodPost, baseURL+"/api/v1/projects/"+projectID+"/storage/transfers/fast-stage", bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("build storage request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-User-ID", userID)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("post storage fast-stage: %v", err)
	}
	defer resp.Body.Close()
	var envelope struct {
		Data  map[string]any      `json:"data"`
		Error *platform.ErrorBody `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		t.Fatalf("decode storage response: %v", err)
	}
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("status = %d data=%#v error=%#v, want 202", resp.StatusCode, envelope.Data, envelope.Error)
	}
	return envelope.Data
}

func assertFastTransferStartRecord(t *testing.T, record map[string]any, transferID, namespace string) {
	t.Helper()
	if textE2E(record["id"]) != transferID || textE2E(record["target_namespace"]) != namespace {
		t.Fatalf("transfer identity = %#v, want %s in %s", record, transferID, namespace)
	}
	if textE2E(record["status"]) != "queued" || textE2E(record["dispatch_status"]) != "submitted" {
		t.Fatalf("transfer status = %#v, want queued/submitted", record)
	}
	if textE2E(record["mover_job_namespace"]) != namespace || textE2E(record["mover_job_name"]) == "" {
		t.Fatalf("mover metadata = %#v, want namespace/name", record)
	}
}

func assertFastTransferStartQueuedEvent(t *testing.T, events *platform.EventBus, transferID string) {
	t.Helper()
	for _, event := range events.Outbox() {
		if event.Name == "FastTransferQueued" && event.Source == "storage-service" && textE2E(event.Data["transfer_id"]) == transferID {
			return
		}
	}
	t.Fatalf("outbox missing FastTransferQueued for %s: %#v", transferID, eventNames(events.Outbox()))
}

func eventNames(events []contracts.Event) []string {
	names := make([]string, 0, len(events))
	for _, event := range events {
		names = append(names, event.Name)
	}
	return names
}
