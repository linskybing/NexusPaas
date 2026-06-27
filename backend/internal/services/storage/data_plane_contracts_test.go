package storage

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/linskybing/nexuspaas/backend/internal/platform"
)

func TestResolveStorageDataPlanePlanBuildsScratchStageInAndCheckpoint(t *testing.T) {
	app := newStorageMountPlanResolverApp(t)
	if err := seedDefaultStorageProfiles(app); err != nil {
		t.Fatal(err)
	}
	seedMountPlanBinding(t, app, "project-1", "group-1", "pvc-data", "project-data-claim")
	seedMountPlanGroupSource(t, app, "group-1", "pvc-data", "running")
	seedMountPlanProjectPermission(t, app, "project-1", "pvc-data", "user-1", "read_only")

	plan, status, err := resolveStorageDataPlanePlan(context.Background(), app, storageDataPlanePlanResolverRequest(), storageDataPlanePlanRequest{
		ProjectID:      "project-1",
		JobID:          "job-train-1",
		UserID:         "user-1",
		Namespace:      "project-one",
		ScratchProfile: defaultScratchProfileID,
		Checkpoint: storageDataPlaneCheckpointRequest{
			FlushTargetProfile: defaultCheckpointFlushProfileID,
			WritePolicy:        defaultCheckpointWritePolicy,
			RetainLocalLastN:   2,
		},
		DatasetSources: []storageDataPlaneDatasetSource{{
			StorageBindingID: "pvc-data",
			CacheKey:         "dataset-tokenized-v1",
		}},
	})
	if err != nil || status != http.StatusOK {
		t.Fatalf("status=%d err=%v plan=%#v, want resolved data plane plan", status, err, plan)
	}
	if plan.Scratch.ProfileID != defaultScratchProfileID || plan.Scratch.StorageClassName != "local-nvme-scratch" ||
		plan.Scratch.ClaimName != "scratch-job-train-1" || plan.Scratch.MountPath != defaultScratchMountPath {
		t.Fatalf("scratch = %#v, want local NVMe scratch claim", plan.Scratch)
	}
	if len(plan.StageInOperations) != 1 {
		t.Fatalf("stage ops = %#v, want one dataset stage-in", plan.StageInOperations)
	}
	stage := plan.StageInOperations[0]
	if stage.TargetPVC != "project-data-claim" || stage.SourceNamespace != "group-1-source-ns" ||
		stage.SourcePVC != "group-1-source-pvc-data" || stage.CacheHit {
		t.Fatalf("stage op = %#v, want storage-owned source and project target PVC", stage)
	}
	if stage.SourcePath != "/nexuspaas/stage-in/dataset-tokenized-v1" ||
		stage.ScratchPath != "/nexuspaas/scratch/datasets/dataset-tokenized-v1" {
		t.Fatalf("stage paths = %q -> %q, want staged dataset paths", stage.SourcePath, stage.ScratchPath)
	}
	if plan.Checkpoint.FlushTargetProfileID != defaultCheckpointFlushProfileID ||
		plan.Checkpoint.StorageClassName != "cephfs-rwx-authority" ||
		plan.Checkpoint.WritePolicy != defaultCheckpointWritePolicy ||
		plan.Checkpoint.RetainLocalLastN != 2 {
		t.Fatalf("checkpoint = %#v, want CephFS authority flush target", plan.Checkpoint)
	}
}

func TestResolveStorageDataPlanePlanMissingProfileReturnsUnprocessable(t *testing.T) {
	app := newStorageMountPlanResolverApp(t)
	_, status, err := resolveStorageDataPlanePlan(context.Background(), app, storageDataPlanePlanResolverRequest(), storageDataPlanePlanRequest{
		ProjectID:      "project-1",
		JobID:          "job-train-1",
		UserID:         "user-1",
		Namespace:      "project-one",
		ScratchProfile: "missing-scratch",
		Checkpoint: storageDataPlaneCheckpointRequest{
			FlushTargetProfile: defaultCheckpointFlushProfileID,
			WritePolicy:        defaultCheckpointWritePolicy,
		},
	})
	if status != http.StatusUnprocessableEntity || err == nil || !strings.Contains(err.Error(), "missing-scratch") {
		t.Fatalf("status=%d err=%v, want missing profile as 422", status, err)
	}
}

func TestStorageDataPlanePlanContractRequiresServiceKeyAndPublishesEvent(t *testing.T) {
	app := newStorageDataPlanePlanTestApp(t)
	createProjectStorageFixtures(t, app)

	code, data, errBody := postStorageDataPlanePlan(t, app, "service-key", `{
		"job_id":"job-train-1",
		"user_id":"U2",
		"namespace":"proj-p1",
		"dataset_sources":[{"storage_binding_id":"pvc1","cache_key":"dataset-v1"}],
		"checkpoint":{"retain_local_last_n":1}
	}`)
	if code != http.StatusOK || errBody != nil {
		t.Fatalf("status=%d error=%#v data=%#v, want data plane plan", code, errBody, data)
	}
	if scratch, _ := data["scratch"].(map[string]any); scratch["profile_id"] != defaultScratchProfileID {
		t.Fatalf("scratch payload = %#v, want default scratch profile", scratch)
	}
	foundEvent := false
	for _, event := range app.Events.Outbox() {
		if event.Name == dataPlanePlanBuiltEvent {
			foundEvent = true
			break
		}
	}
	if !foundEvent {
		t.Fatalf("outbox = %#v, want DataPlanePlanBuilt event", app.Events.Outbox())
	}

	code, _, errBody = postStorageDataPlanePlan(t, app, "wrong-key", `{"job_id":"job-train-1","user_id":"U2"}`)
	if code != http.StatusUnauthorized || errBody == nil {
		t.Fatalf("status=%d error=%#v, want unauthorized without service key", code, errBody)
	}
}

func newStorageDataPlanePlanTestApp(t *testing.T) *platform.App {
	t.Helper()
	app := newStorageTestApp(t)
	app.Config.ServiceAPIKey = "service-key"
	app.RegisterService(platform.ServiceSpec{
		Name: serviceName,
		Routes: []platform.RouteSpec{{
			Method:              http.MethodPost,
			Pattern:             pathInternalStorageDataPlanePlan,
			Resource:            "data_plane_plans",
			Action:              "resolve",
			IDParam:             "project_id",
			PolicyBypass:        true,
			ServiceAuthRequired: true,
			StateChanging:       true,
		}},
	})
	return app
}

func storageDataPlanePlanResolverRequest() *http.Request {
	return httptest.NewRequest(http.MethodPost, "/internal/storage/projects/project-1/data-plane-plan", nil)
}

func postStorageDataPlanePlan(t *testing.T, app *platform.App, key, body string) (int, map[string]any, *platform.ErrorBody) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/internal/storage/projects/P1/data-plane-plan", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if key != "" {
		req.Header.Set("X-Service-Key", key)
	}
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	var envelope struct {
		Data  json.RawMessage     `json:"data"`
		Error *platform.ErrorBody `json:"error"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&envelope); err != nil {
		t.Fatalf("decode response: %v; body=%s", err, rec.Body.String())
	}
	data := map[string]any{}
	if rec.Code < http.StatusBadRequest && len(envelope.Data) > 0 {
		if err := json.Unmarshal(envelope.Data, &data); err != nil {
			t.Fatalf("decode data plane plan data: %v", err)
		}
	}
	errBody := envelope.Error
	if rec.Code >= http.StatusBadRequest && errBody == nil && len(envelope.Data) > 0 {
		var payload map[string]any
		if err := json.Unmarshal(envelope.Data, &payload); err != nil {
			t.Fatalf("decode data plane plan error data: %v", err)
		}
		if message, _ := payload["message"].(string); message != "" {
			errBody = &platform.ErrorBody{Code: "error", Message: message}
		}
	}
	return rec.Code, data, errBody
}
