package workload

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/linskybing/nexuspaas/backend/internal/contracts"
	"github.com/linskybing/nexuspaas/backend/internal/platform"
)

func TestInternalJobsReadContractIsServiceKeyGatedListOnly(t *testing.T) {
	app := platform.NewApp(platform.Config{ServiceName: "all", HTTPAddr: ":0", ServiceAPIKey: "svc-key"})
	Register(app)
	if _, err := app.Store.Create(context.Background(), jobsResource, map[string]any{
		"id":           "job-read",
		"job_id":       "job-read",
		"project_id":   "project-read",
		"user_id":      "user-read",
		"status":       "running",
		"required_gpu": 1.5,
	}); err != nil {
		t.Fatalf("seed workload job: %v", err)
	}

	if rec := serveWorkloadReadContract(app, "/internal/workload/jobs", ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("list without service key status = %d, want 401", rec.Code)
	}
	if rec := serveWorkloadReadContract(app, "/internal/workload/jobs", "wrong"); rec.Code != http.StatusUnauthorized {
		t.Fatalf("list with wrong service key status = %d, want 401", rec.Code)
	}

	rec := serveWorkloadReadContract(app, "/internal/workload/jobs", "svc-key")
	if rec.Code != http.StatusOK {
		t.Fatalf("list status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var env struct {
		Data []contracts.Record[map[string]any] `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode list envelope: %v", err)
	}
	if len(env.Data) != 1 || env.Data[0].ID != "job-read" || env.Data[0].Data["status"] != "running" {
		t.Fatalf("jobs list = %#v, want seeded running job", env.Data)
	}

	if rec := serveWorkloadReadContract(app, "/internal/workload/jobs/job-read", "svc-key"); rec.Code != http.StatusNotFound && rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("job get status = %d, want 404/405 because the owner contract is list-only", rec.Code)
	}
}

func serveWorkloadReadContract(app *platform.App, path, serviceKey string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if serviceKey != "" {
		req.Header.Set("X-Service-Key", serviceKey)
	}
	app.ServeHTTP(rec, req)
	return rec
}
