package schedulerquota

import (
	"net/http"
	"strings"
	"testing"

	"github.com/linskybing/nexuspaas/backend/internal/platform"
)

// GPU-010: a device class outside the plan's allowed_gpu_models is rejected.
func TestSubmitAdmissionRejectsDeviceClassNotAllowedByPlan(t *testing.T) {
	app := newSchedulerQuotaTestApp()
	seedAdmissionProject(t, app, admissionFixture{}) // allowed_gpu_models = [gpu.nvidia.com]

	code, data, _ := reviewSubmitAdmission(app, schedulerRequest(http.MethodPost, "/api/v1/internal/scheduler/admission", admissionBody(t, map[string]any{
		"project_id":        "P1",
		"user_id":           "U1",
		"queue_name":        "default-batch",
		"device_class_name": "gpu.example.com",
		"required_gpu":      1,
		"required_cpu":      1,
		"required_memory":   1024,
	})), platform.RouteSpec{})

	assertSchedulerStatus(t, code, data, http.StatusForbidden)
	if !strings.Contains(data.(map[string]any)["reason"].(string), "is not allowed by the plan") {
		t.Fatalf("device class denial = %#v, want plan model reason", data)
	}
}

// GPU-011: the plan gpu_limit restricts the aggregate SM percentage because the
// reserved GPU is accounted as gpu_count * sm_percentage / 100.
func TestSubmitAdmissionPlanGPULimitRestrictsAggregateSM(t *testing.T) {
	app := newSchedulerQuotaTestApp()
	seedAdmissionProject(t, app, admissionFixture{gpuLimit: 1})
	// An active sibling job already reserves 0.6 effective GPU (1 GPU @ 60% SM).
	createSchedulerRecord(t, app, workloadJobsResource, map[string]any{
		"id":           "J0",
		"project_id":   "P1",
		"user_id":      "U2",
		"status":       "running",
		"required_gpu": 0.6,
	})

	code, data, _ := reviewSubmitAdmission(app, schedulerRequest(http.MethodPost, "/api/v1/internal/scheduler/admission", admissionBody(t, map[string]any{
		"project_id":        "P1",
		"user_id":           "U1",
		"queue_name":        "default-batch",
		"device_class_name": "gpu.nvidia.com",
		"gpu_count":         1,
		"sm_percentage":     60,
		"required_gpu":      0.6,
		"required_cpu":      1,
		"required_memory":   1024,
	})), platform.RouteSpec{})

	// 0.6 (used) + 0.6 (requested) = 1.2 > 1.0 limit.
	assertSchedulerStatus(t, code, data, http.StatusConflict)
	if !strings.Contains(data.(map[string]any)["reason"].(string), "GPU quota exceeded") {
		t.Fatalf("aggregate SM denial = %#v, want GPU quota reason", data)
	}
}

// GPU-012: MPS sharing across projects is blocked unless the plan explicitly
// allows it (platform admin approval).
func TestSubmitAdmissionBlocksCrossProjectMPSUnlessPlanAllows(t *testing.T) {
	body := func() map[string]any {
		return map[string]any{
			"project_id":           "P1",
			"user_id":              "U1",
			"queue_name":           "default-batch",
			"device_class_name":    "gpu.nvidia.com",
			"gpu_count":            1,
			"sm_percentage":        50,
			"required_gpu":         0.5,
			"mps_share_project_id": "P2",
			"required_cpu":         1,
			"required_memory":      1024,
		}
	}

	blocked := newSchedulerQuotaTestApp()
	seedAdmissionProject(t, blocked, admissionFixture{})
	code, data, _ := reviewSubmitAdmission(blocked, schedulerRequest(http.MethodPost, "/api/v1/internal/scheduler/admission", admissionBody(t, body())), platform.RouteSpec{})
	assertSchedulerStatus(t, code, data, http.StatusForbidden)
	if !strings.Contains(data.(map[string]any)["reason"].(string), "across projects requires platform admin") {
		t.Fatalf("cross-project MPS denial = %#v, want platform admin reason", data)
	}

	allowed := newSchedulerQuotaTestApp()
	seedAdmissionProject(t, allowed, admissionFixture{planOverrides: map[string]any{"allow_cross_project_mps": true}})
	code, data, _ = reviewSubmitAdmission(allowed, schedulerRequest(http.MethodPost, "/api/v1/internal/scheduler/admission", admissionBody(t, body())), platform.RouteSpec{})
	assertSchedulerStatus(t, code, data, http.StatusOK)
	if data.(map[string]any)["allowed"] != true {
		t.Fatalf("cross-project MPS with plan allowance = %#v, want allowed", data)
	}
}

// GPU-014: a terminated job's GPU reservation is released and no longer counts
// toward the project quota.
func TestSubmitAdmissionReleasesTerminatedJobGPUReservation(t *testing.T) {
	app := newSchedulerQuotaTestApp()
	seedAdmissionProject(t, app, admissionFixture{gpuLimit: 2})
	// A finished job that used the full 2-GPU budget must not block new work.
	createSchedulerRecord(t, app, workloadJobsResource, map[string]any{
		"id":           "Jdone",
		"project_id":   "P1",
		"user_id":      "U2",
		"status":       "succeeded",
		"required_gpu": 2,
	})

	code, data, _ := reviewSubmitAdmission(app, schedulerRequest(http.MethodPost, "/api/v1/internal/scheduler/admission", admissionBody(t, map[string]any{
		"project_id":        "P1",
		"user_id":           "U1",
		"queue_name":        "default-batch",
		"device_class_name": "gpu.nvidia.com",
		"required_gpu":      2,
		"required_cpu":      1,
		"required_memory":   1024,
	})), platform.RouteSpec{})

	assertSchedulerStatus(t, code, data, http.StatusOK)
	if data.(map[string]any)["allowed"] != true {
		t.Fatalf("admission after terminated job = %#v, want allowed (reservation released)", data)
	}
}
