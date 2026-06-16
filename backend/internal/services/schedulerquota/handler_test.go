package schedulerquota

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/linskybing/nexuspaas/backend/internal/contracts"
	"github.com/linskybing/nexuspaas/backend/internal/platform"
)

func TestSchedulerQuotaQueueAndPlanWorkflow(t *testing.T) {
	app := newSchedulerQuotaTestApp()

	code, data, _ := createQueue(app, schedulerRequest(http.MethodPost, "/api/v1/queues", `{"id":"q1","name":"gpu","priority":10}`), platform.RouteSpec{})
	assertSchedulerStatus(t, code, data, http.StatusCreated)
	code, data, _ = createQueue(app, schedulerRequest(http.MethodPost, "/api/v1/queues", `{"id":"q1","name":"duplicate"}`), platform.RouteSpec{})
	assertSchedulerStatus(t, code, data, http.StatusConflict)

	updateReq := schedulerRequest(http.MethodPut, "/api/v1/queues/q1", `{"priority":20}`)
	updateReq.SetPathValue("id", "q1")
	code, data, _ = updateQueue(app, updateReq, platform.RouteSpec{})
	assertSchedulerStatus(t, code, data, http.StatusOK)
	if data.(contracts.Record[map[string]any]).Data["priority"] != float64(20) {
		t.Fatalf("updated queue = %#v, want priority 20", data)
	}

	code, data, _ = createQueue(app, schedulerRequest(http.MethodPost, "/api/v1/queues", `{"id":"q2","name":"batch"}`), platform.RouteSpec{})
	assertSchedulerStatus(t, code, data, http.StatusCreated)
	code, data, _ = createPlan(app, schedulerRequest(http.MethodPost, "/api/v1/plans", `{"id":"p1","name":"default","gpu_limit":4,"queue_ids":["q1"]}`), platform.RouteSpec{})
	assertSchedulerStatus(t, code, data, http.StatusCreated)

	planQueuesReq := schedulerRequest(http.MethodGet, "/api/v1/plans/p1/queues", "")
	planQueuesReq.SetPathValue("id", "p1")
	code, data, _ = listPlanQueues(app, planQueuesReq, platform.RouteSpec{})
	assertSchedulerStatus(t, code, data, http.StatusOK)
	if queues := data.([]contracts.Record[map[string]any]); len(queues) != 1 || queues[0].ID != "q1" {
		t.Fatalf("plan queues = %#v, want q1", data)
	}

	bindReq := schedulerRequest(http.MethodPut, "/api/v1/plans/p1/queues", `{"queue_ids":["q1","q2"]}`)
	bindReq.SetPathValue("id", "p1")
	code, data, _ = bindPlanQueues(app, bindReq, platform.RouteSpec{})
	assertSchedulerStatus(t, code, data, http.StatusOK)

	deleteReq := schedulerRequest(http.MethodDelete, "/api/v1/queues/q2", "")
	deleteReq.SetPathValue("id", "q2")
	code, data, _ = deleteQueue(app, deleteReq, platform.RouteSpec{})
	assertSchedulerStatus(t, code, data, http.StatusOK)
	plan, _ := app.Store.Get(context.Background(), plansResource, "p1")
	if got := plan.Data["queue_ids"].([]string); len(got) != 1 || got[0] != "q1" {
		t.Fatalf("plan queue_ids after queue delete = %#v, want [q1]", got)
	}

	code, data, _ = batchDeleteQueues(app, schedulerRequest(http.MethodDelete, "/api/v1/queues/batch", `{"ids":["q1","missing"]}`), platform.RouteSpec{})
	assertSchedulerStatus(t, code, data, http.StatusOK)
	result := data.(map[string]any)
	if result["succeeded"] != 1 || result["failed"] != 1 {
		t.Fatalf("batch delete result = %#v, want one success and one failure", result)
	}
	if len(app.Events.Outbox()) == 0 {
		t.Fatal("scheduler workflow did not publish domain events")
	}
}

func TestSchedulerQuotaProjectBindingAndLiveQuota(t *testing.T) {
	// Co-hosted with org-project so the plan-binding owner contract is reachable
	// in-process; binding/unbinding go through org-project, not a scheduler write.
	app := newCoHostedBindingApp(t)
	createSchedulerRecord(t, app, queuesResource, map[string]any{"id": "q1", "name": "gpu"})
	createSchedulerRecord(t, app, plansResource, map[string]any{"id": "p1", "name": "default", "gpu_limit": 4.0, "queue_ids": []string{"q1"}})
	createSchedulerRecord(t, app, projectsResource, map[string]any{"id": "proj-1", "name": "science"})

	bindReq := schedulerRequest(http.MethodPut, "/api/v1/plans/bind/proj-1", `{"plan_id":"p1"}`)
	bindReq.SetPathValue("project_id", "proj-1")
	code, data, _ := bindPlanToProject(app, bindReq, platform.RouteSpec{})
	assertSchedulerStatus(t, code, data, http.StatusOK)

	projectQueuesReq := schedulerRequest(http.MethodGet, "/api/v1/queues/project/proj-1", "")
	projectQueuesReq.SetPathValue("project_id", "proj-1")
	code, data, _ = listQueuesByProject(app, projectQueuesReq, platform.RouteSpec{})
	assertSchedulerStatus(t, code, data, http.StatusOK)
	if queues := data.([]contracts.Record[map[string]any]); len(queues) != 1 || queues[0].ID != "q1" {
		t.Fatalf("project queues = %#v, want q1", data)
	}

	quotaReq := schedulerRequest(http.MethodGet, "/api/v1/projects/proj-1/quota/live", "")
	quotaReq.SetPathValue("id", "proj-1")
	code, data, _ = getProjectLiveQuota(app, quotaReq, platform.RouteSpec{})
	assertSchedulerStatus(t, code, data, http.StatusOK)
	quota := data.(contracts.Record[map[string]any]).Data
	if quota["source_resource"] != "plan" || quota["gpu_limit"] != float64(4) {
		t.Fatalf("derived quota = %#v, want plan quota", quota)
	}

	createSchedulerRecord(t, app, liveQuotasResource, map[string]any{"id": "proj-1", "project_id": "proj-1", "source_resource": "live", "gpu_limit": 8.0})
	code, data, _ = getProjectLiveQuota(app, quotaReq, platform.RouteSpec{})
	assertSchedulerStatus(t, code, data, http.StatusOK)
	live := data.(contracts.Record[map[string]any]).Data
	if live["source_resource"] != "live" || live["gpu_limit"] != float64(8) {
		t.Fatalf("live quota = %#v, want stored live quota", live)
	}

	deletePlanReq := schedulerRequest(http.MethodDelete, "/api/v1/plans/p1", "")
	deletePlanReq.SetPathValue("id", "p1")
	code, data, _ = deletePlan(app, deletePlanReq, platform.RouteSpec{})
	assertSchedulerStatus(t, code, data, http.StatusOK)
	project, _ := app.Store.Get(context.Background(), projectsResource, "proj-1")
	if project.Data["plan_id"] != "" || project.Data["resource_plan_id"] != "" {
		t.Fatalf("project plan binding after plan delete = %#v, want cleared", project.Data)
	}
}

// TestBindPlanToProjectGoesThroughOwnerContract proves scheduler-quota does not
// write the org-project project record directly: with org-project NOT co-hosted
// and no remote URL configured, the bind must fail closed (no local write),
// retiring the problem.md #2 ownership violation.
func TestBindPlanToProjectGoesThroughOwnerContract(t *testing.T) {
	app := platform.NewApp(platform.Config{ServiceName: serviceName, HTTPAddr: ":0"})
	Register(app)
	createSchedulerRecord(t, app, plansResource, map[string]any{"id": "p1", "name": "default"})
	createSchedulerRecord(t, app, projectsResource, map[string]any{"id": "proj-1", "name": "science"})

	bindReq := schedulerRequest(http.MethodPut, "/api/v1/plans/bind/proj-1", `{"plan_id":"p1"}`)
	bindReq.SetPathValue("project_id", "proj-1")
	code, _, _ := bindPlanToProject(app, bindReq, platform.RouteSpec{})
	if code != http.StatusServiceUnavailable {
		t.Fatalf("isolated bind without org-project contract status = %d, want 503", code)
	}
	project, _ := app.Store.Get(context.Background(), projectsResource, "proj-1")
	if got := project.Data["plan_id"]; got != nil && got != "" {
		t.Fatalf("project plan_id = %v, want no direct scheduler write", got)
	}
}

func TestSchedulerQuotaReadUpdateAndBatchPlanHandlers(t *testing.T) {
	app := newSchedulerQuotaTestApp()
	createSchedulerRecord(t, app, queuesResource, map[string]any{"id": "q2", "name": "zeta"})
	createSchedulerRecord(t, app, queuesResource, map[string]any{"id": "q1", "name": "alpha"})
	createSchedulerRecord(t, app, plansResource, map[string]any{"id": "p1", "name": "starter", "queue_ids": []string{"q1"}})
	createSchedulerRecord(t, app, plansResource, map[string]any{"id": "p2", "name": "pro", "queue_ids": []string{"q2"}})

	code, data, _ := listQueues(app, schedulerRequest(http.MethodGet, "/api/v1/queues", ""), platform.RouteSpec{})
	assertSchedulerStatus(t, code, data, http.StatusOK)
	if queues := data.([]contracts.Record[map[string]any]); len(queues) != 2 || queues[0].ID != "q1" {
		t.Fatalf("listQueues = %#v, want sorted q1 first", queues)
	}
	getQueueReq := schedulerRequest(http.MethodGet, "/api/v1/queues/q2", "")
	getQueueReq.SetPathValue("id", "q2")
	code, data, _ = getQueue(app, getQueueReq, platform.RouteSpec{})
	assertSchedulerStatus(t, code, data, http.StatusOK)
	if data.(contracts.Record[map[string]any]).Data["name"] != "zeta" {
		t.Fatalf("getQueue = %#v, want q2", data)
	}
	missingQueueReq := schedulerRequest(http.MethodGet, "/api/v1/queues/missing", "")
	missingQueueReq.SetPathValue("id", "missing")
	code, data, _ = getQueue(app, missingQueueReq, platform.RouteSpec{})
	assertSchedulerStatus(t, code, data, http.StatusNotFound)

	code, data, _ = listPlans(app, schedulerRequest(http.MethodGet, "/api/v1/plans", ""), platform.RouteSpec{})
	assertSchedulerStatus(t, code, data, http.StatusOK)
	if plans := data.([]contracts.Record[map[string]any]); len(plans) != 2 || plans[0].ID != "p2" {
		t.Fatalf("listPlans = %#v, want sorted by name", plans)
	}
	getPlanReq := schedulerRequest(http.MethodGet, "/api/v1/plans/p1", "")
	getPlanReq.SetPathValue("id", "p1")
	code, data, _ = getPlan(app, getPlanReq, platform.RouteSpec{})
	assertSchedulerStatus(t, code, data, http.StatusOK)

	updatePlanReq := schedulerRequest(http.MethodPatch, "/api/v1/plans/p1", `{"name":"starter-updated","queue_ids":["q1","q2"]}`)
	updatePlanReq.SetPathValue("id", "p1")
	code, data, _ = updatePlan(app, updatePlanReq, platform.RouteSpec{})
	assertSchedulerStatus(t, code, data, http.StatusOK)
	if data.(contracts.Record[map[string]any]).Data["name"] != "starter-updated" {
		t.Fatalf("updatePlan = %#v, want updated name", data)
	}

	code, data, _ = batchDeletePlans(app, schedulerRequest(http.MethodDelete, "/api/v1/plans/batch", `{"ids":["p1","missing"]}`), platform.RouteSpec{})
	assertSchedulerStatus(t, code, data, http.StatusOK)
	result := data.(map[string]any)
	if result["succeeded"] != 1 || result["failed"] != 1 {
		t.Fatalf("batchDeletePlans = %#v, want one success and one failure", result)
	}
	if _, found := app.Store.Get(context.Background(), plansResource, "p1"); found {
		t.Fatal("batchDeletePlans left p1 in store")
	}
}

func TestSchedulerQuotaValidationAndMalformedJSON(t *testing.T) {
	app := newSchedulerQuotaTestApp()
	code, data, _ := createQueue(app, schedulerRequest(http.MethodPost, "/api/v1/queues", `{`), platform.RouteSpec{})
	assertSchedulerStatus(t, code, data, http.StatusBadRequest)
	if got := len(app.Store.List(context.Background(), queuesResource)); got != 0 {
		t.Fatalf("queues after malformed create = %d, want 0", got)
	}

	createSchedulerRecord(t, app, plansResource, map[string]any{"id": "p1", "name": "default"})
	bindReq := schedulerRequest(http.MethodPut, "/api/v1/plans/p1/queues", `{"queue_ids":["missing"]}`)
	bindReq.SetPathValue("id", "p1")
	code, data, _ = bindPlanQueues(app, bindReq, platform.RouteSpec{})
	assertSchedulerStatus(t, code, data, http.StatusBadRequest)

	updateReq := schedulerRequest(http.MethodPut, "/api/v1/plans/missing", `{"name":"x"}`)
	updateReq.SetPathValue("id", "missing")
	code, data, _ = updatePlan(app, updateReq, platform.RouteSpec{})
	assertSchedulerStatus(t, code, data, http.StatusNotFound)
}

func newSchedulerQuotaTestApp() *platform.App {
	app := platform.NewApp(platform.Config{ServiceName: "all", HTTPAddr: ":0"})
	Register(app)
	return app
}

func schedulerRequest(method, target, body string) *http.Request {
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Idempotency-Key", "test-"+method+"-"+target)
	return req
}

func createSchedulerRecord(t *testing.T, app *platform.App, resource string, data map[string]any) {
	t.Helper()
	if _, err := app.Store.Create(context.Background(), resource, data); err != nil {
		t.Fatal(err)
	}
}

func assertSchedulerStatus(t *testing.T, code int, data any, want int) {
	t.Helper()
	if code != want {
		t.Fatalf("status=%d data=%#v, want %d", code, data, want)
	}
}

func intPtr(value int) *int {
	return &value
}
