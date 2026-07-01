package orgproject

import (
	"net/http"
	"strings"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/contracts"
	"github.com/linskybing/nexuspaas/backend/internal/platform"
	"github.com/linskybing/nexuspaas/backend/internal/services/shared"
)

// Internal command contract for project↔plan binding.
//
// org-project-service is the owner of the project aggregate, including its
// plan binding (plan_id / resource_plan_id). scheduler-quota-service owns plans
// and decides *which* plan binds to a project, but it must not write project
// records directly. These service-key-gated endpoints let scheduler-quota apply
// and clear bindings through the owner, exactly as the workload eviction
// contract keeps job-state writes inside workload-service (blocker-ledger.md #2).
const (
	pathBindProjectPlan    = "/internal/org-project/projects/{project_id}/plan"
	pathClearPlanBindings  = "/internal/org-project/plans/{plan_id}/project-bindings"
	eventProjectPlanUpdate = "ProjectUpdated"
)

// requireOrgProjectServiceAuth gates the internal contract on the shared service
// key. It mirrors workload's requireWorkloadServiceAuth: the route is closed
// (404) when no key is configured, and unauthorized (401) on a bad/absent key.
func requireOrgProjectServiceAuth(app *platform.App, r *http.Request) (int, map[string]any, bool) {
	if app.Config.ServiceAPIKey == "" {
		return http.StatusNotFound, shared.ErrorData("not found"), false
	}
	if !app.ServiceRequestAuthorized(r) {
		return http.StatusUnauthorized, shared.ErrorData("service authentication is required"), false
	}
	return 0, nil, true
}

// bindProjectPlan sets a project's plan binding. The owner is authoritative for
// project existence (404 if missing) and the write is idempotent: re-binding the
// same plan simply restamps updated_at.
func bindProjectPlan(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	if status, data, ok := requireOrgProjectServiceAuth(app, r); !ok {
		return status, data, nil
	}
	payload, err := platform.DecodeMapWithError(r)
	if err != nil {
		return http.StatusBadRequest, shared.ErrorData(msgInvalidRequestBody), nil
	}
	projectID := strings.TrimSpace(r.PathValue("project_id"))
	if projectID == "" {
		return http.StatusBadRequest, shared.ErrorData("project_id is required"), nil
	}
	planID := shared.TextValue(payload, "plan_id", "planId")
	if planID == "" {
		return http.StatusBadRequest, shared.ErrorData("plan_id is required"), nil
	}
	var updated orgProjectRecord
	var bound bool
	if err := app.WithTx(r.Context(), func(tx platform.StoreTx) error {
		old, next, ok, e := projectRepository(app).BindProjectPlanTx(r.Context(), tx, projectID, planID, time.Now().UTC())
		if e != nil || !ok {
			return e
		}
		bound = true
		updated = next
		tx.Emit(eventFor(r, eventProjectPlanUpdate, map[string]any{"old": old.Data, "new": next.Data}))
		return nil
	}); err != nil || !bound {
		if _, found := projectRepository(app).FindProject(r.Context(), projectID); !found {
			return http.StatusNotFound, shared.ErrorData(msgProjectNotFound), nil
		}
		return http.StatusInternalServerError, shared.ErrorData("project plan binding failed"), nil
	}
	return http.StatusOK, contracts.Record[map[string]any]{ID: updated.ID, Data: updated.Data}, nil
}

// clearProjectsPlan removes a plan binding from every project that references it.
// It is the owner-side counterpart of scheduler-quota's plan deletion and is
// idempotent: when no project references the plan it reports zero cleared.
func clearProjectsPlan(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	if status, data, ok := requireOrgProjectServiceAuth(app, r); !ok {
		return status, data, nil
	}
	planID := strings.TrimSpace(r.PathValue("plan_id"))
	if planID == "" {
		return http.StatusBadRequest, shared.ErrorData("plan_id is required"), nil
	}
	var cleared int
	if err := app.WithTx(r.Context(), func(tx platform.StoreTx) error {
		var e error
		cleared, e = projectRepository(app).ClearProjectsPlanTx(r.Context(), tx, planID, time.Now().UTC(), func(update orgProjectPlanUpdate) {
			tx.Emit(eventFor(r, eventProjectPlanUpdate, map[string]any{"old": update.Old.Data, "new": update.New.Data}))
		})
		return e
	}); err != nil {
		return http.StatusInternalServerError, shared.ErrorData("project plan bindings clear failed"), nil
	}
	return http.StatusOK, map[string]any{"plan_id": planID, "cleared": cleared}, nil
}
