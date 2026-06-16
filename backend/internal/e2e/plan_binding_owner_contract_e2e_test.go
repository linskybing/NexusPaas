//go:build e2e

package e2e

import (
	"net/http"
	"testing"
)

// TestPlanBindingOwnerContractE2E proves that, across separate scheduler-quota
// and org-project processes, scheduler-quota applies and clears a project's plan
// binding only through the org-project-owned internal contract (problem.md #2):
// the org-project project record reflects the binding even though scheduler-quota
// has no write access to org-project:projects.
func TestPlanBindingOwnerContractE2E(t *testing.T) {
	// scheduler-quota reaches org-project's binding contract over HTTP, so wire
	// org-project's URL into the scheduler-quota process for this harness only.
	h := newHarnessWithPeers(t,
		map[string][]string{schedulerQuotaService: {orgProjectService}},
		orgProjectService, schedulerQuotaService)

	projectID := "bindproj" + h.runID
	planID := "BINDPL" + h.runID

	h.createRecord(orgProjectsResource, projectID, map[string]any{"project_name": "binding-" + h.runID})
	h.createRecord(schedulerPlansResource, planID, map[string]any{"name": "plan-" + h.runID, "gpu_limit": 2.0})

	// Bind through scheduler-quota's public route; it must delegate to org-project.
	bindPath := "/api/v1/plans/bind/" + projectID
	h.doJSON(schedulerQuotaService, http.MethodPut, bindPath, map[string]any{"plan_id": planID}, h.apiKey, http.StatusOK)

	bound := h.getRecord(orgProjectsResource, projectID)
	if bound.Data["plan_id"] != planID || bound.Data["resource_plan_id"] != planID {
		t.Fatalf("project after bind = %#v, want plan_id/resource_plan_id %s", bound.Data, planID)
	}

	// Deleting the plan must clear the binding from the project via the owner contract.
	h.doJSON(schedulerQuotaService, http.MethodDelete, "/api/v1/plans/"+planID, nil, h.apiKey, http.StatusOK)

	cleared := h.getRecord(orgProjectsResource, projectID)
	if pid := cleared.Data["plan_id"]; pid != "" {
		t.Fatalf("project plan_id after plan delete = %#v, want cleared", pid)
	}
}
