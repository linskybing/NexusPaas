package schedulerquota

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/contracts"
	"github.com/linskybing/nexuspaas/backend/internal/platform"
	"github.com/linskybing/nexuspaas/backend/internal/services/shared"
)

const (
	serviceName        = "scheduler-quota-service"
	queuesResource     = serviceName + ":queues"
	plansResource      = serviceName + ":plans"
	liveQuotasResource = serviceName + ":live_quotas"
	projectsResource   = "org-project-service:projects"
	pathQueueID        = "/api/v1/queues/{id}"
	pathPlanID         = "/api/v1/plans/{id}"
	msgInvalidBody     = "invalid request body"
	msgAdminOnly       = "admin access required"
	msgQueueNotFound   = "queue not found"
	msgPlanNotFound    = "plan not found"
	msgUnknownQueueIDs = "unknown queue ids: "
	msgQuotaNotFound   = "project quota not found"
	msgRepoUnavailable = "scheduler repository unavailable"
	defaultQueuePrefix = "Q"
	defaultPlanPrefix  = "PL"
	defaultIDStart     = 2600001
	defaultIDWidth     = 7
)

func Register(app *platform.App) {
	app.RegisterRequiredFields(networkProfilesResource, "name", "primary_cni")
	app.RegisterRequiredFields(placementProfilesResource, "name", "scheduler_backend")
	if err := seedDefaultNetworkProfiles(app); err != nil {
		slog.Error("network profile seed failed", "error", err)
	}
	if err := seedDefaultPlacementProfiles(app); err != nil {
		slog.Error("placement profile seed failed", "error", err)
	}
	app.RegisterCustomHandler(http.MethodPost, "/api/v1/network-profiles", createNetworkProfile)
	app.RegisterCustomHandler(http.MethodPut, "/api/v1/network-profiles/{id}", updateNetworkProfile)
	app.RegisterCustomHandler(http.MethodDelete, "/api/v1/network-profiles/{id}", deleteNetworkProfile)
	app.RegisterCustomHandler(http.MethodPost, "/api/v1/placement-profiles", createPlacementProfile)
	app.RegisterCustomHandler(http.MethodPut, "/api/v1/placement-profiles/{id}", updatePlacementProfile)
	app.RegisterCustomHandler(http.MethodDelete, "/api/v1/placement-profiles/{id}", deletePlacementProfile)
	app.RegisterCustomHandler(http.MethodGet, "/api/v1/queues", listQueues)
	app.RegisterCustomHandler(http.MethodPost, "/api/v1/queues", createQueue)
	app.RegisterCustomHandler(http.MethodGet, pathQueueID, getQueue)
	app.RegisterCustomHandler(http.MethodPut, pathQueueID, updateQueue)
	app.RegisterCustomHandler(http.MethodPatch, pathQueueID, updateQueue)
	app.RegisterCustomHandler(http.MethodDelete, pathQueueID, deleteQueue)
	app.RegisterCustomHandler(http.MethodDelete, "/api/v1/queues/batch", batchDeleteQueues)
	app.RegisterCustomHandler(http.MethodGet, "/api/v1/queues/project/{project_id}", listQueuesByProject)

	app.RegisterCustomHandler(http.MethodGet, "/api/v1/plans", listPlans)
	app.RegisterCustomHandler(http.MethodPost, "/api/v1/plans", createPlan)
	app.RegisterCustomHandler(http.MethodGet, pathPlanID, getPlan)
	app.RegisterCustomHandler(http.MethodPut, pathPlanID, updatePlan)
	app.RegisterCustomHandler(http.MethodPatch, pathPlanID, updatePlan)
	app.RegisterCustomHandler(http.MethodDelete, pathPlanID, deletePlan)
	app.RegisterCustomHandler(http.MethodDelete, "/api/v1/plans/batch", batchDeletePlans)
	app.RegisterCustomHandler(http.MethodPut, "/api/v1/plans/bind/{project_id}", bindPlanToProject)
	app.RegisterCustomHandler(http.MethodGet, pathPlanID+"/queues", listPlanQueues)
	app.RegisterCustomHandler(http.MethodPut, pathPlanID+"/queues", bindPlanQueues)
	app.RegisterCustomHandler(http.MethodGet, "/api/v1/projects/{id}/quota/live", getProjectLiveQuota)
	app.RegisterCustomHandler(http.MethodPost, "/api/v1/internal/scheduler/admission", reviewSubmitAdmission)
	app.RegisterCustomHandler(http.MethodPost, "/api/v1/internal/scheduler/preemptions", handlePreemption)
	registerResourceQuotaReconciler(app)
	registerPlanWindowReaper(app)
	registerPriorityClassSync(app)
}

func listQueues(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	repo := schedulerQuotaRepositoryForApp(app)
	if repo == nil {
		return http.StatusInternalServerError, shared.ErrorData(msgRepoUnavailable), nil
	}
	return http.StatusOK, sortedRecords(repo.ListQueues(r.Context()), "name"), nil
}

func createQueue(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	payload, err := decodePayload(r)
	if err != nil {
		return http.StatusBadRequest, shared.ErrorData(msgInvalidBody), nil
	}
	repo := schedulerQuotaRepositoryForApp(app)
	if repo == nil {
		return http.StatusInternalServerError, shared.ErrorData(msgRepoUnavailable), nil
	}
	queue, err := queuePayload(repo, payload)
	if err != nil {
		return http.StatusBadRequest, shared.ErrorData(err.Error()), nil
	}
	record, err := app.CreateRecordWithEvent(r.Context(), queuesResource, queue, func(rec contracts.Record[map[string]any]) contracts.Event {
		return schedulerEvent(r, "QueueChanged", "created", changePayload(nil, rec.Data))
	})
	if err != nil {
		return createError(err), nil, nil
	}
	return http.StatusCreated, record, nil
}

func getQueue(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	repo := schedulerQuotaRepositoryForApp(app)
	if repo == nil {
		return http.StatusInternalServerError, shared.ErrorData(msgRepoUnavailable), nil
	}
	record, found := repo.GetQueue(r.Context(), pathValue(r, "id"))
	if !found {
		return http.StatusNotFound, shared.ErrorData(msgQueueNotFound), nil
	}
	return http.StatusOK, record, nil
}

func updateQueue(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	payload, err := decodePayload(r)
	if err != nil {
		return http.StatusBadRequest, shared.ErrorData(msgInvalidBody), nil
	}
	id := pathValue(r, "id")
	repo := schedulerQuotaRepositoryForApp(app)
	if repo == nil {
		return http.StatusInternalServerError, shared.ErrorData(msgRepoUnavailable), nil
	}
	previous, found := repo.GetQueue(r.Context(), id)
	if !found {
		return http.StatusNotFound, shared.ErrorData(msgQueueNotFound), nil
	}
	update := normalizeQueueUpdate(payload)
	record, ok, err := app.UpdateRecordWithEvent(r.Context(), queuesResource, id, update, func(rec contracts.Record[map[string]any]) contracts.Event {
		return schedulerEvent(r, "QueueChanged", "updated", changePayload(previous.Data, rec.Data))
	})
	if err != nil || !ok {
		return http.StatusInternalServerError, shared.ErrorData("queue update failed"), nil
	}
	return http.StatusOK, record, nil
}

func deleteQueue(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	id := pathValue(r, "id")
	repo := schedulerQuotaRepositoryForApp(app)
	if repo == nil {
		return http.StatusInternalServerError, shared.ErrorData(msgRepoUnavailable), nil
	}
	deleted, removed, err := deleteQueueWithEvent(app, r, repo, id)
	if err != nil {
		return http.StatusInternalServerError, shared.ErrorData("queue could not be deleted"), nil
	}
	if !removed {
		return http.StatusNotFound, shared.ErrorData(msgQueueNotFound), nil
	}
	return http.StatusOK, deleted, nil
}

func deleteQueueWithEvent(
	app *platform.App,
	r *http.Request,
	repo *recordStoreSchedulerQuotaRepository,
	id string,
) (map[string]any, bool, error) {
	previous, found := repo.GetQueue(r.Context(), id)
	if !found {
		return nil, false, nil
	}
	deleted := map[string]any{"id": id, "deleted": true}
	removed := false
	err := app.WithTx(r.Context(), func(tx platform.StoreTx) error {
		ok, err := repo.DeleteQueueAndRemoveFromPlansTx(r.Context(), tx, id)
		if err != nil {
			return err
		}
		removed = ok
		if ok {
			tx.Emit(schedulerEvent(r, "QueueChanged", "deleted", changePayload(previous.Data, deleted)))
		}
		return nil
	})
	return deleted, removed, err
}

func batchDeleteQueues(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	ids, err := decodeIDs(r)
	if err != nil {
		return http.StatusBadRequest, shared.ErrorData(msgInvalidBody), nil
	}
	repo := schedulerQuotaRepositoryForApp(app)
	if repo == nil {
		return http.StatusInternalServerError, shared.ErrorData(msgRepoUnavailable), nil
	}
	result := schedulerDeleteResult{Errors: []string{}, Deleted: []string{}}
	for _, id := range ids {
		if _, removed, err := deleteQueueWithEvent(app, r, repo, id); err != nil || !removed {
			result.Failed++
			result.Errors = append(result.Errors, id)
			continue
		}
		result.Succeeded++
		result.Deleted = append(result.Deleted, id)
	}
	response := result.response()
	return http.StatusOK, response, nil
}

func listQueuesByProject(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	project, found := newAdmissionReaderForApp(app).Project(r.Context(), pathValue(r, "project_id"))
	if !found {
		return http.StatusNotFound, shared.ErrorData("project not found"), nil
	}
	planID := shared.TextValue(project.Data, "plan_id", "planId", "resource_plan_id", "resourcePlanId")
	if planID == "" {
		return http.StatusOK, []contracts.Record[map[string]any]{}, nil
	}
	return queuesForPlan(app, r, planID)
}

func listPlans(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	repo := schedulerQuotaRepositoryForApp(app)
	if repo == nil {
		return http.StatusInternalServerError, shared.ErrorData(msgRepoUnavailable), nil
	}
	return http.StatusOK, sortedRecords(repo.ListPlans(r.Context()), "name"), nil
}

func createPlan(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	payload, err := decodePayload(r)
	if err != nil {
		return http.StatusBadRequest, shared.ErrorData(msgInvalidBody), nil
	}
	repo := schedulerQuotaRepositoryForApp(app)
	if repo == nil {
		return http.StatusInternalServerError, shared.ErrorData(msgRepoUnavailable), nil
	}
	plan, err := planPayload(repo, payload)
	if err != nil {
		return http.StatusBadRequest, shared.ErrorData(err.Error()), nil
	}
	if missing := repo.MissingQueues(r.Context(), shared.StringSlice(plan["queue_ids"])); len(missing) > 0 {
		return http.StatusBadRequest, shared.ErrorData(msgUnknownQueueIDs + strings.Join(missing, ",")), nil
	}
	record, err := app.CreateRecordWithEvent(r.Context(), plansResource, plan, func(rec contracts.Record[map[string]any]) contracts.Event {
		return schedulerEvent(r, "PlanChanged", "created", changePayload(nil, rec.Data))
	})
	if err != nil {
		return createError(err), nil, nil
	}
	return http.StatusCreated, record, nil
}

func getPlan(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	repo := schedulerQuotaRepositoryForApp(app)
	if repo == nil {
		return http.StatusInternalServerError, shared.ErrorData(msgRepoUnavailable), nil
	}
	record, found := repo.GetPlan(r.Context(), pathValue(r, "id"))
	if !found {
		return http.StatusNotFound, shared.ErrorData(msgPlanNotFound), nil
	}
	return http.StatusOK, record, nil
}

func updatePlan(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	payload, err := decodePayload(r)
	if err != nil {
		return http.StatusBadRequest, shared.ErrorData(msgInvalidBody), nil
	}
	id := pathValue(r, "id")
	repo := schedulerQuotaRepositoryForApp(app)
	if repo == nil {
		return http.StatusInternalServerError, shared.ErrorData(msgRepoUnavailable), nil
	}
	previous, found := repo.GetPlan(r.Context(), id)
	if !found {
		return http.StatusNotFound, shared.ErrorData(msgPlanNotFound), nil
	}
	update := normalizePlanUpdate(payload)
	if missing := repo.MissingQueues(r.Context(), shared.StringSlice(update["queue_ids"])); len(missing) > 0 {
		return http.StatusBadRequest, shared.ErrorData(msgUnknownQueueIDs + strings.Join(missing, ",")), nil
	}
	record, ok, err := app.UpdateRecordWithEvent(r.Context(), plansResource, id, update, func(rec contracts.Record[map[string]any]) contracts.Event {
		return schedulerEvent(r, "PlanChanged", "updated", changePayload(previous.Data, rec.Data))
	})
	if err != nil || !ok {
		return http.StatusInternalServerError, shared.ErrorData("plan update failed"), nil
	}
	return http.StatusOK, record, nil
}

func deletePlan(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	id := pathValue(r, "id")
	repo := schedulerQuotaRepositoryForApp(app)
	if repo == nil {
		return http.StatusInternalServerError, shared.ErrorData(msgRepoUnavailable), nil
	}
	deleted, removed, err := deletePlanWithEvent(app, r, repo, id)
	if err != nil {
		return http.StatusInternalServerError, shared.ErrorData("plan could not be deleted"), nil
	}
	if !removed {
		return http.StatusNotFound, shared.ErrorData(msgPlanNotFound), nil
	}
	unbindPlanFromProjects(app, r, id)
	return http.StatusOK, deleted, nil
}

func deletePlanWithEvent(
	app *platform.App,
	r *http.Request,
	repo *recordStoreSchedulerQuotaRepository,
	id string,
) (map[string]any, bool, error) {
	previous, found := repo.GetPlan(r.Context(), id)
	if !found {
		return nil, false, nil
	}
	deleted := map[string]any{"id": id, "deleted": true}
	removed := false
	err := app.WithTx(r.Context(), func(tx platform.StoreTx) error {
		ok, err := repo.DeletePlanTx(r.Context(), tx, id)
		if err != nil {
			return err
		}
		removed = ok
		if ok {
			tx.Emit(schedulerEvent(r, "PlanChanged", "deleted", changePayload(previous.Data, deleted)))
		}
		return nil
	})
	return deleted, removed, err
}

func batchDeletePlans(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	ids, err := decodeIDs(r)
	if err != nil {
		return http.StatusBadRequest, shared.ErrorData(msgInvalidBody), nil
	}
	repo := schedulerQuotaRepositoryForApp(app)
	if repo == nil {
		return http.StatusInternalServerError, shared.ErrorData(msgRepoUnavailable), nil
	}
	result := schedulerDeleteResult{Errors: []string{}, Deleted: []string{}}
	for _, id := range ids {
		if _, removed, err := deletePlanWithEvent(app, r, repo, id); err != nil || !removed {
			result.Failed++
			result.Errors = append(result.Errors, id)
			continue
		}
		result.Succeeded++
		result.Deleted = append(result.Deleted, id)
	}
	for _, id := range result.Deleted {
		unbindPlanFromProjects(app, r, id)
	}
	response := result.response()
	return http.StatusOK, response, nil
}

func bindPlanToProject(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	payload, err := decodePayload(r)
	if err != nil {
		return http.StatusBadRequest, shared.ErrorData(msgInvalidBody), nil
	}
	projectID := pathValue(r, "project_id")
	planID := shared.FirstNonEmpty(shared.TextValue(payload, "plan_id", "planId"), shared.TextValue(payload, "id"))
	if planID == "" {
		return http.StatusBadRequest, shared.ErrorData("plan_id is required"), nil
	}
	repo := schedulerQuotaRepositoryForApp(app)
	if repo == nil {
		return http.StatusInternalServerError, shared.ErrorData(msgRepoUnavailable), nil
	}
	if _, found := repo.GetPlan(r.Context(), planID); !found {
		return http.StatusNotFound, shared.ErrorData(msgPlanNotFound), nil
	}
	// The project aggregate (incl. its plan binding) is owned by org-project; apply
	// the binding through the owner contract rather than writing it here (problem.md #2).
	client, err := newOrgProjectBindingClient(app)
	if err != nil {
		slog.Error("scheduler quota plan binding client unavailable", "error", err)
		return http.StatusServiceUnavailable, shared.ErrorData("project plan binding is unavailable"), nil
	}
	switch err := client.BindPlan(r.Context(), projectID, planID); {
	case err == nil:
	case errors.Is(err, errProjectNotFound):
		return http.StatusNotFound, shared.ErrorData("project not found"), nil
	default:
		slog.Error("scheduler quota plan binding failed", "project_id", projectID, "plan_id", planID, "error", err)
		return http.StatusBadGateway, shared.ErrorData("project plan binding failed"), nil
	}
	publish(app, r, "PlanChanged", "bound_project", changePayload(nil, map[string]any{"project_id": projectID, "plan_id": planID}))
	return http.StatusOK, map[string]any{"project_id": projectID, "plan_id": planID}, nil
}

func listPlanQueues(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	return queuesForPlan(app, r, pathValue(r, "id"))
}

func bindPlanQueues(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	payload, err := decodePayload(r)
	if err != nil {
		return http.StatusBadRequest, shared.ErrorData(msgInvalidBody), nil
	}
	id := pathValue(r, "id")
	repo := schedulerQuotaRepositoryForApp(app)
	if repo == nil {
		return http.StatusInternalServerError, shared.ErrorData(msgRepoUnavailable), nil
	}
	previous, found := repo.GetPlan(r.Context(), id)
	if !found {
		return http.StatusNotFound, shared.ErrorData(msgPlanNotFound), nil
	}
	queueIDs := firstStringSlice(payload, "queue_ids", "queueIds", "ids")
	if missing := repo.MissingQueues(r.Context(), queueIDs); len(missing) > 0 {
		return http.StatusBadRequest, shared.ErrorData(msgUnknownQueueIDs + strings.Join(missing, ",")), nil
	}
	var record contracts.Record[map[string]any]
	var updated bool
	if err := app.WithTx(r.Context(), func(tx platform.StoreTx) error {
		var e error
		record, updated, e = repo.BindPlanQueuesTx(r.Context(), tx, id, queueIDs)
		if e != nil || !updated {
			return e
		}
		tx.Emit(schedulerEvent(r, "PlanChanged", "bound_queues", changePayload(previous.Data, record.Data)))
		return nil
	}); err != nil || !updated {
		return http.StatusInternalServerError, shared.ErrorData("plan queue binding failed"), nil
	}
	return http.StatusOK, record, nil
}

func getProjectLiveQuota(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	projectID := pathValue(r, "id")
	repo := schedulerQuotaRepositoryForApp(app)
	if repo == nil {
		return http.StatusInternalServerError, shared.ErrorData(msgRepoUnavailable), nil
	}
	if record, found := repo.GetLiveQuota(r.Context(), projectID); found {
		return http.StatusOK, record, nil
	}
	project, found := newAdmissionReaderForApp(app).Project(r.Context(), projectID)
	if !found {
		return http.StatusNotFound, shared.ErrorData(msgQuotaNotFound), nil
	}
	planID := shared.TextValue(project.Data, "plan_id", "planId", "resource_plan_id", "resourcePlanId")
	if planID == "" {
		return http.StatusNotFound, shared.ErrorData(msgQuotaNotFound), nil
	}
	record, found := repo.DerivedQuotaFromPlan(r.Context(), projectID, planID, time.Now().UTC())
	if !found {
		return http.StatusNotFound, shared.ErrorData(msgQuotaNotFound), nil
	}
	return http.StatusOK, record, nil
}

func queuePayload(repo *recordStoreSchedulerQuotaRepository, payload map[string]any) (map[string]any, error) {
	name := shared.TextValue(payload, "name")
	if name == "" {
		return nil, fmt.Errorf("name is required")
	}
	queue := shared.CloneMap(payload)
	queue["name"] = name
	if shared.TextValue(queue, "id") == "" {
		queue["id"] = repo.NextQueueID()
	}
	queue["updated_at"] = time.Now().UTC()
	return queue, nil
}

func planPayload(repo *recordStoreSchedulerQuotaRepository, payload map[string]any) (map[string]any, error) {
	name := shared.TextValue(payload, "name")
	if name == "" {
		return nil, fmt.Errorf("name is required")
	}
	plan := normalizePlanUpdate(payload)
	plan["name"] = name
	if shared.TextValue(plan, "id") == "" {
		plan["id"] = repo.NextPlanID()
	}
	plan["updated_at"] = time.Now().UTC()
	return plan, nil
}

func normalizeQueueUpdate(payload map[string]any) map[string]any {
	update := shared.CloneMap(payload)
	if name := shared.TextValue(payload, "name"); name != "" {
		update["name"] = name
	}
	update["updated_at"] = time.Now().UTC()
	return update
}

func normalizePlanUpdate(payload map[string]any) map[string]any {
	update := shared.CloneMap(payload)
	if queueIDs := firstStringSlice(payload, "queue_ids", "queueIds", "queues"); queueIDs != nil {
		update["queue_ids"] = queueIDs
		update["queues"] = queueIDs
	}
	update["updated_at"] = time.Now().UTC()
	return update
}

func queuesForPlan(app *platform.App, r *http.Request, planID string) (int, any, *platform.Degraded) {
	repo := schedulerQuotaRepositoryForApp(app)
	if repo == nil {
		return http.StatusInternalServerError, shared.ErrorData(msgRepoUnavailable), nil
	}
	_, records, found := repo.QueuesForPlan(r.Context(), planID)
	if !found {
		return http.StatusNotFound, shared.ErrorData(msgPlanNotFound), nil
	}
	return http.StatusOK, sortedRecords(records, "name"), nil
}

// unbindPlanFromProjects clears a deleted plan from every project bound to it
// through the org-project owner contract, so scheduler-quota never writes project
// records directly (problem.md #2). Failures are logged but do not fail the plan
// deletion, preserving the prior fire-and-forget cleanup semantics.
func unbindPlanFromProjects(app *platform.App, r *http.Request, planID string) {
	client, err := newOrgProjectBindingClient(app)
	if err != nil {
		slog.Error("scheduler quota plan unbind client unavailable", "plan_id", planID, "error", err)
		return
	}
	if err := client.ClearPlanBindings(r.Context(), planID); err != nil {
		slog.Error("scheduler quota plan unbind failed", "plan_id", planID, "error", err)
	}
}

func quotaFromPlan(projectID string, plan contracts.Record[map[string]any], now time.Time) map[string]any {
	return map[string]any{
		"id":              projectID,
		"project_id":      projectID,
		"plan_id":         plan.ID,
		"gpu_limit":       shared.NumberValue(plan.Data, "gpu_limit", "gpuLimit"),
		"cpu_limit_cores": shared.NumberValue(plan.Data, "cpu_limit_cores", "cpuLimitCores"),
		"memory_limit_gb": shared.NumberValue(plan.Data, "memory_limit_gb", "memoryLimitGb"),
		"queue_ids":       shared.StringSlice(plan.Data["queue_ids"]),
		"source_resource": "plan",
		"generated_at":    now,
		"quota_contract":  "derived",
	}
}

func decodePayload(r *http.Request) (map[string]any, error) {
	return platform.DecodeMapWithError(r)
}

func decodeIDs(r *http.Request) ([]string, error) {
	payload, err := decodePayload(r)
	if err != nil {
		return nil, err
	}
	return firstStringSlice(payload, "ids", "queue_ids", "queueIds", "plan_ids", "planIds"), nil
}

func firstStringSlice(data map[string]any, keys ...string) []string {
	for _, key := range keys {
		if values := shared.StringSlice(data[key]); values != nil {
			return values
		}
	}
	return nil
}

func sortedRecords(records []contracts.Record[map[string]any], field string) []contracts.Record[map[string]any] {
	out := append([]contracts.Record[map[string]any]{}, records...)
	sort.SliceStable(out, func(i, j int) bool {
		left := shared.FirstNonEmpty(shared.TextValue(out[i].Data, field), out[i].ID)
		right := shared.FirstNonEmpty(shared.TextValue(out[j].Data, field), out[j].ID)
		return left < right
	})
	return out
}

func removeValue(values []string, remove string) []string {
	out := values[:0]
	for _, value := range values {
		if value != remove {
			out = append(out, value)
		}
	}
	return out
}

func pathValue(r *http.Request, name string) string {
	return strings.TrimSpace(r.PathValue(name))
}

func createError(err error) int {
	if platform.IsCreateConflict(err) {
		return http.StatusConflict
	}
	return http.StatusInternalServerError
}

func changePayload(oldValue, newValue map[string]any) map[string]any {
	payload := shared.CloneMap(newValue)
	payload["old"] = cloneEventMapOrNil(oldValue)
	payload["new"] = cloneEventMapOrNil(newValue)
	return payload
}

func cloneEventMapOrNil(value map[string]any) any {
	if value == nil {
		return nil
	}
	return shared.CloneMap(value)
}

func schedulerEvent(r *http.Request, name, action string, data map[string]any) contracts.Event {
	traceID := shared.FirstNonEmpty(r.Header.Get("X-Trace-ID"), r.Header.Get("X-Request-ID"), "scheduler-quota-local")
	event := contracts.Event{
		EventID:        platform.NewUUID(),
		Name:           name,
		Source:         serviceName,
		OccurredAt:     time.Now().UTC(),
		TraceID:        traceID,
		SchemaVersion:  1,
		IdempotencyKey: r.Header.Get("Idempotency-Key"),
		Data:           shared.CloneMap(data),
	}
	event.Data["action"] = action
	actor := shared.FirstNonEmpty(r.Header.Get("X-User-ID"), "anonymous")
	event.Data["actor_user_id"] = actor
	if shared.TextValue(event.Data, "user_id", "userId") == "" {
		event.Data["user_id"] = actor
	}
	return event
}

func publish(app *platform.App, r *http.Request, name, action string, data map[string]any) {
	if err := app.Events.Publish(r.Context(), schedulerEvent(r, name, action, data)); err != nil {
		slog.Error("scheduler quota event publish failed", "event", name, "error", err)
	}
}
