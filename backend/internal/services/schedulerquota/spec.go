package schedulerquota

import (
	"net/http"

	"github.com/linskybing/nexuspaas/backend/internal/platform"
	"github.com/linskybing/nexuspaas/backend/internal/services/shared"
)

func Spec() platform.ServiceSpec {
	const (
		projectID = "/api/v1/projects/{id}"
		queueID   = "/api/v1/queues/{id}"
		planID    = "/api/v1/plans/{id}"
	)
	route, id, admin, serviceInternal := shared.Route, shared.ID, shared.Admin, shared.ServiceInternal
	return platform.ServiceSpec{
		Name:            "scheduler-quota-service",
		Category:        "compute",
		Phase:           "5",
		RequiresCluster: true,
		Description:     "Resource plans, queues, quota reservation, priority, preemption, and queue dispatch arbitration.",
		Tables:          []string{"plans", "queues", "resource_quotas", "submit_admissions", "priority_classes", "reservations", "preemption_records", "gpu_claim_snapshots", "outbox", "inbox"},
		Events:          []string{"PlanChanged", "QuotaReserved", "QuotaCommitted", "QuotaReleased", "SubmitAdmissionReviewed", "QueueDepthChanged", "JobPreempted", "PriorityClassSyncCompleted"},
		Routes: []platform.RouteSpec{
			route(http.MethodGet, "/api/v1/plans", "plans", "list"),
			route(http.MethodPost, "/api/v1/plans", "plans", "create", admin()),
			route(http.MethodGet, planID, "plans", "get", id("id"), admin()),
			route(http.MethodPut, planID, "plans", "update", id("id"), admin()),
			route(http.MethodPatch, planID, "plans", "update", id("id"), admin()),
			route(http.MethodDelete, planID, "plans", "delete", id("id"), admin()),
			route(http.MethodDelete, "/api/v1/plans/batch", "plans", "batch_delete", admin()),
			route(http.MethodPut, "/api/v1/plans/bind/{project_id}", "plans", "bind_project", id("project_id"), admin()),
			route(http.MethodGet, planID+"/queues", "plan_queues", "list", id("id"), admin()),
			route(http.MethodPut, planID+"/queues", "plan_queues", "update", id("id"), admin()),
			route(http.MethodGet, "/api/v1/queues", "queues", "list"),
			route(http.MethodPost, "/api/v1/queues", "queues", "create", admin()),
			route(http.MethodGet, queueID, "queues", "get", id("id"), admin()),
			route(http.MethodPut, queueID, "queues", "update", id("id"), admin()),
			route(http.MethodPatch, queueID, "queues", "update", id("id"), admin()),
			route(http.MethodDelete, queueID, "queues", "delete", id("id"), admin()),
			route(http.MethodDelete, "/api/v1/queues/batch", "queues", "batch_delete", admin()),
			route(http.MethodGet, "/api/v1/queues/project/{project_id}", "queues", "list_by_project", id("project_id")),
			route(http.MethodGet, projectID+"/quota/live", "live_quotas", "get", id("id")),
			route(http.MethodPost, "/api/v1/internal/quota/reservations", "reservations", "quota_reserve", serviceInternal()),
			route(http.MethodPost, "/api/v1/internal/quota/reservations/{reservationId}/commit", "reservations", "quota_commit", id("reservationId"), serviceInternal()),
			route(http.MethodPost, "/api/v1/internal/quota/reservations/{reservationId}/release", "reservations", "quota_release", id("reservationId"), serviceInternal()),
			route(http.MethodPost, "/api/v1/internal/scheduler/admission", "submit_admissions", "review", serviceInternal()),
			route(http.MethodPost, "/api/v1/internal/scheduler/preemptions", "preemptions", "command", serviceInternal()),
			route(http.MethodPost, "/api/v1/internal/workers/leases", "worker_leases", "worker_lease", serviceInternal()),
		},
	}
}
