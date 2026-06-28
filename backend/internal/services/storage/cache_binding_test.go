package storage

import (
	"context"
	"net/http"
	"testing"

	"github.com/linskybing/nexuspaas/backend/internal/platform"
)

func TestCacheBindingCRUDRequiresProjectManager(t *testing.T) {
	app := newStorageTestApp(t)
	createProjectStorageFixtures(t, app)

	createReq := storageProjectRequest(http.MethodPost, "/api/v1/projects/P1/storage/cache-bindings", `{
		"storage_binding_id":"pvc1",
		"cache_key":"dataset-v1",
		"node_class":"gpu-hpc",
		"scratch_profile":"local-nvme-scratch"
	}`, "U3", "P1")
	code, data, _ := createCacheBinding(app, createReq, platform.RouteSpec{})
	assertStorageStatus(t, code, data, http.StatusOK)
	created := data.(map[string]any)
	id := created["id"].(string)
	if created["project_id"] != "P1" || created["cache_key"] != "dataset-v1" || created["node_class"] != "gpu-hpc" {
		t.Fatalf("created cache binding = %#v, want project cache mapping", created)
	}

	listReq := storageProjectRequest(http.MethodGet, "/api/v1/projects/P1/storage/cache-bindings", "", "U3", "P1")
	code, data, _ = listCacheBindings(app, listReq, platform.RouteSpec{})
	assertStorageStatus(t, code, data, http.StatusOK)
	if rows := data.([]map[string]any); len(rows) != 1 || rows[0]["id"] != id {
		t.Fatalf("cache binding list = %#v, want created binding", data)
	}

	getReq := storageProjectRequest(http.MethodGet, "/api/v1/projects/P1/storage/cache-bindings/"+id, "", "U3", "P1")
	getReq.SetPathValue("cacheBindingId", id)
	code, data, _ = getCacheBinding(app, getReq, platform.RouteSpec{})
	assertStorageStatus(t, code, data, http.StatusOK)

	updateReq := storageProjectRequest(http.MethodPut, "/api/v1/projects/P1/storage/cache-bindings/"+id, `{
		"storage_binding_id":"pvc1",
		"cache_key":"dataset-v1",
		"node_class":"gpu-hpc",
		"scratch_profile":"local-nvme-scratch",
		"last_staged_at":"2026-06-27T00:00:00Z"
	}`, "U3", "P1")
	updateReq.SetPathValue("cacheBindingId", id)
	code, data, _ = updateCacheBinding(app, updateReq, platform.RouteSpec{})
	assertStorageStatus(t, code, data, http.StatusOK)
	if data.(map[string]any)["last_staged_at"] != "2026-06-27T00:00:00Z" {
		t.Fatalf("updated cache binding = %#v, want supplied last_staged_at", data)
	}

	readerReq := storageProjectRequest(http.MethodPost, "/api/v1/projects/P1/storage/cache-bindings", `{
		"storage_binding_id":"pvc1",
		"cache_key":"dataset-v2",
		"scratch_profile":"local-nvme-scratch"
	}`, "U2", "P1")
	code, _, _ = createCacheBinding(app, readerReq, platform.RouteSpec{})
	assertStorageStatus(t, code, nil, http.StatusForbidden)

	deleteReq := storageProjectRequest(http.MethodDelete, "/api/v1/projects/P1/storage/cache-bindings/"+id, "", "U3", "P1")
	deleteReq.SetPathValue("cacheBindingId", id)
	code, data, _ = deleteCacheBinding(app, deleteReq, platform.RouteSpec{})
	assertStorageStatus(t, code, data, http.StatusOK)
	assertStorageRecordMissing(t, app, cacheBindingsResource, id)
}

func TestDataPlanePlanMarksCacheHitFromCacheBinding(t *testing.T) {
	app := newStorageMountPlanResolverApp(t)
	if err := seedDefaultStorageProfiles(app); err != nil {
		t.Fatal(err)
	}
	seedMountPlanBinding(t, app, "project-1", "group-1", "pvc-data", "project-data-claim")
	seedMountPlanGroupSource(t, app, "group-1", "pvc-data", "running")
	seedMountPlanProjectPermission(t, app, "project-1", "pvc-data", "user-1", "read_only")
	seedCacheBindingRecord(t, app, map[string]any{
		"id":                 "cache-project-1-pvc-data-dataset-v1",
		"project_id":         "project-1",
		"storage_binding_id": "pvc-data",
		"cache_key":          "dataset-v1",
		"node_class":         "gpu-hpc",
		"scratch_profile":    defaultScratchProfileID,
	})

	plan, status, err := resolveStorageDataPlanePlan(context.Background(), app, storageDataPlanePlanResolverRequest(), storageDataPlanePlanRequest{
		ProjectID:      "project-1",
		JobID:          "job-train-1",
		UserID:         "user-1",
		Namespace:      "project-one",
		NodeClass:      "gpu-hpc",
		ScratchProfile: defaultScratchProfileID,
		Checkpoint: storageDataPlaneCheckpointRequest{
			FlushTargetProfile: defaultCheckpointFlushProfileID,
			WritePolicy:        defaultCheckpointWritePolicy,
		},
		DatasetSources: []storageDataPlaneDatasetSource{{
			StorageBindingID: "pvc-data",
			CacheKey:         "dataset-v1",
		}},
	})
	if err != nil || status != http.StatusOK {
		t.Fatalf("status=%d err=%v plan=%#v, want data plane plan", status, err, plan)
	}
	if len(plan.StageInOperations) != 1 || !plan.StageInOperations[0].CacheHit ||
		plan.StageInOperations[0].CacheBindingID != "cache-project-1-pvc-data-dataset-v1" {
		t.Fatalf("stage ops = %#v, want cache-hit stage operation", plan.StageInOperations)
	}

	plan, status, err = resolveStorageDataPlanePlan(context.Background(), app, storageDataPlanePlanResolverRequest(), storageDataPlanePlanRequest{
		ProjectID:      "project-1",
		JobID:          "job-train-2",
		UserID:         "user-1",
		Namespace:      "project-one",
		NodeClass:      "cpu",
		ScratchProfile: defaultScratchProfileID,
		Checkpoint: storageDataPlaneCheckpointRequest{
			FlushTargetProfile: defaultCheckpointFlushProfileID,
			WritePolicy:        defaultCheckpointWritePolicy,
		},
		DatasetSources: []storageDataPlaneDatasetSource{{
			StorageBindingID: "pvc-data",
			CacheKey:         "dataset-v1",
		}},
	})
	if err != nil || status != http.StatusOK {
		t.Fatalf("status=%d err=%v plan=%#v, want data plane plan", status, err, plan)
	}
	if len(plan.StageInOperations) != 1 || plan.StageInOperations[0].CacheHit {
		t.Fatalf("stage ops = %#v, want cache miss for wrong node class", plan.StageInOperations)
	}
}

func seedCacheBindingRecord(t *testing.T, app *platform.App, row map[string]any) {
	t.Helper()
	if _, err := app.Store.Create(context.Background(), cacheBindingsResource, row); err != nil {
		t.Fatal(err)
	}
}
