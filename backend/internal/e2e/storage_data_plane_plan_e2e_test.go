//go:build e2e

package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/linskybing/nexuspaas/backend/internal/platform"
	"github.com/linskybing/nexuspaas/backend/internal/services"
)

const (
	e2eStorageDataPlanePlanKey = "storage-data-plane-e2e-key"
	e2eStorageProfilesResource = "storage-service:storage_profiles"
)

func TestStorageDataPlanePlanContractE2E(t *testing.T) {
	app, store, events := newStorageDataPlanePlanApp(t)
	ids := seedStorageDataPlanePlanRecords(t, store)
	requireSeededStorageProfile(t, store, "local-nvme-scratch")
	requireSeededStorageProfile(t, store, "cephfs-rwx-authority")

	postStorageDataPlanePlan(t, app, ids, "wrong-"+e2eStorageDataPlanePlanKey, http.StatusUnauthorized)

	plan := postStorageDataPlanePlan(t, app, ids, e2eStorageDataPlanePlanKey, http.StatusOK)
	if plan.ProjectID != ids.projectID || plan.JobID != ids.jobID || plan.Namespace != ids.namespace {
		t.Fatalf("plan identity = %#v, want project/job/namespace from request", plan)
	}
	if plan.Scratch.ProfileID != "local-nvme-scratch" ||
		plan.Scratch.StorageClassName != "local-nvme-scratch" ||
		plan.Scratch.ClaimName != "scratch-"+ids.jobID {
		t.Fatalf("scratch = %#v, want local NVMe scratch claim for job", plan.Scratch)
	}
	if len(plan.StageInOperations) != 1 {
		t.Fatalf("stage_in_operations = %#v, want one storage-owned stage-in", plan.StageInOperations)
	}
	stage := plan.StageInOperations[0]
	if stage.StorageBindingID != ids.pvcID || stage.SourceNamespace != ids.sourceNamespace ||
		stage.SourcePVC != ids.sourcePVC || stage.TargetPVC != ids.targetPVC {
		t.Fatalf("stage op = %#v, want storage-owned source/target PVCs", stage)
	}
	if stage.SourceNamespace == "forged-storage" || stage.SourcePVC == "forged-source" || stage.TargetPVC == "forged-target" {
		t.Fatalf("stage op = %#v, trusted forged request source details", stage)
	}
	if plan.Checkpoint.FlushTargetProfileID != "cephfs-rwx-authority" ||
		plan.Checkpoint.StorageClassName != "cephfs-rwx-authority" {
		t.Fatalf("checkpoint = %#v, want CephFS authority profile", plan.Checkpoint)
	}
	assertDataPlanePlanBuiltEvent(t, events, ids)
}

type storageDataPlanePlanIDs struct {
	projectID       string
	groupID         string
	userID          string
	jobID           string
	pvcID           string
	namespace       string
	sourceNamespace string
	sourcePVC       string
	targetPVC       string
}

type storageDataPlanePlanResponse struct {
	ProjectID         string                         `json:"project_id"`
	JobID             string                         `json:"job_id"`
	Namespace         string                         `json:"namespace"`
	Scratch           storageDataPlaneScratch        `json:"scratch"`
	StageInOperations []storageDataPlaneStageIn      `json:"stage_in_operations"`
	Checkpoint        storageDataPlanePlanCheckpoint `json:"checkpoint"`
}

type storageDataPlaneScratch struct {
	ProfileID        string `json:"profile_id"`
	StorageClassName string `json:"storage_class_name"`
	ClaimName        string `json:"claim_name"`
}

type storageDataPlaneStageIn struct {
	StorageBindingID string `json:"storage_binding_id"`
	SourceNamespace  string `json:"source_namespace"`
	SourcePVC        string `json:"source_pvc"`
	TargetPVC        string `json:"target_pvc"`
}

type storageDataPlanePlanCheckpoint struct {
	FlushTargetProfileID string `json:"flush_target_profile_id"`
	StorageClassName     string `json:"storage_class_name"`
}

func newStorageDataPlanePlanApp(t *testing.T) (*platform.App, *platform.Store, *platform.EventBus) {
	t.Helper()
	store := platform.NewStore()
	events := platform.NewEventBus()
	app := platform.NewApp(platform.Config{
		ServiceName:   storageService,
		HTTPAddr:      ":0",
		RequireAuth:   true,
		ServiceAPIKey: e2eStorageDataPlanePlanKey,
	}, platform.WithStore(store), platform.WithEventBus(events))
	services.RegisterAll(app)
	return app, store, events
}

func seedStorageDataPlanePlanRecords(t *testing.T, store *platform.Store) storageDataPlanePlanIDs {
	t.Helper()
	ids := storageDataPlanePlanIDs{
		projectID:       "project-data-plane-e2e",
		groupID:         "group-data-plane-e2e",
		userID:          "user-data-plane-e2e",
		jobID:           "job-data-plane-e2e",
		pvcID:           "datasets",
		namespace:       "proj-data-plane-e2e",
		sourceNamespace: "group-data-plane-e2e-storage",
		sourcePVC:       "source-data-plane-e2e",
		targetPVC:       "target-data-plane-e2e",
	}
	createStorageDataPlaneRecord(t, store, e2eStorageBindingsResource, map[string]any{
		"id":         ids.projectID + ":" + ids.pvcID,
		"project_id": ids.projectID,
		"group_id":   ids.groupID,
		"pvc_id":     ids.pvcID,
		"target_pvc": ids.targetPVC,
	})
	createStorageDataPlaneRecord(t, store, e2eStorageGroupResource, map[string]any{
		"id":          ids.groupID + ":" + ids.pvcID,
		"group_id":    ids.groupID,
		"pvc_id":      ids.pvcID,
		"status":      "running",
		"namespace":   ids.sourceNamespace,
		"source_pvc":  ids.sourcePVC,
		"target_pvc":  "unused-source-owned-target",
		"description": "storage-owned seed",
	})
	createStorageDataPlaneRecord(t, store, e2eStoragePermissionsResource, map[string]any{
		"id":         ids.projectID + ":" + ids.pvcID + ":" + ids.userID,
		"project_id": ids.projectID,
		"pvc_id":     ids.pvcID,
		"user_id":    ids.userID,
		"permission": "read_only",
	})
	return ids
}

func createStorageDataPlaneRecord(t *testing.T, store *platform.Store, resource string, data map[string]any) {
	t.Helper()
	if _, err := store.Create(context.Background(), resource, data); err != nil {
		t.Fatalf("seed %s: %v", resource, err)
	}
}

func requireSeededStorageProfile(t *testing.T, store *platform.Store, id string) {
	t.Helper()
	if _, ok := store.Get(context.Background(), e2eStorageProfilesResource, id); !ok {
		t.Fatalf("storage profile %q was not seeded by storage registration", id)
	}
}

func postStorageDataPlanePlan(t *testing.T, app *platform.App, ids storageDataPlanePlanIDs, serviceKey string, want int) storageDataPlanePlanResponse {
	t.Helper()
	body, err := json.Marshal(map[string]any{
		"job_id":    ids.jobID,
		"user_id":   ids.userID,
		"namespace": ids.namespace,
		"dataset_sources": []map[string]any{{
			"storage_binding_id": ids.pvcID,
			"cache_key":          "dataset-v1",
			"source_namespace":   "forged-storage",
			"source_pvc":         "forged-source",
			"target_pvc":         "forged-target",
		}},
		"checkpoint": map[string]any{"retain_local_last_n": 2},
	})
	if err != nil {
		t.Fatalf("marshal data-plane request: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/internal/storage/projects/"+ids.projectID+"/data-plane-plan", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Service-Key", serviceKey)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if rec.Code != want {
		t.Fatalf("data-plane status=%d body=%s, want %d", rec.Code, rec.Body.String(), want)
	}
	if want != http.StatusOK {
		return storageDataPlanePlanResponse{}
	}
	var envelope struct {
		Data storageDataPlanePlanResponse `json:"data"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&envelope); err != nil {
		t.Fatalf("decode data-plane envelope: %v body=%s", err, rec.Body.String())
	}
	return envelope.Data
}

func assertDataPlanePlanBuiltEvent(t *testing.T, events *platform.EventBus, ids storageDataPlanePlanIDs) {
	t.Helper()
	for _, event := range events.Outbox() {
		if event.Name != "DataPlanePlanBuilt" {
			continue
		}
		if event.Source != storageService || event.SchemaVersion != 1 {
			t.Fatalf("event metadata = %#v, want storage-service schema v1", event)
		}
		assertDataPlaneEventValue(t, event.Data, "project_id", ids.projectID)
		assertDataPlaneEventValue(t, event.Data, "job_id", ids.jobID)
		assertDataPlaneEventValue(t, event.Data, "namespace", ids.namespace)
		assertDataPlaneEventValue(t, event.Data, "scratch_profile", "local-nvme-scratch")
		assertDataPlaneEventValue(t, event.Data, "checkpoint_profile", "cephfs-rwx-authority")
		assertDataPlaneEventValue(t, event.Data, "dataset_source_count", 1)
		return
	}
	t.Fatalf("outbox = %#v, want DataPlanePlanBuilt", events.Outbox())
}

func assertDataPlaneEventValue(t *testing.T, data map[string]any, key string, want any) {
	t.Helper()
	if got := data[key]; got != want {
		t.Fatalf("event[%s] = %#v, want %#v in %#v", key, got, want, data)
	}
}
