package schedulerquota

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/platform"
)

func TestSubmitAdmissionAllowsPlanQueueAndResources(t *testing.T) {
	app := newSchedulerQuotaTestApp()
	seedAdmissionProject(t, app, admissionFixture{})

	code, data, _ := reviewSubmitAdmission(app, schedulerRequest(http.MethodPost, "/api/v1/internal/scheduler/admission", admissionBody(t, map[string]any{
		"job_id":          "J1",
		"project_id":      "P1",
		"user_id":         "U1",
		"queue_name":      "default-batch",
		"required_gpu":    1,
		"required_cpu":    1,
		"required_memory": 1024,
		"resources":       []any{podAdmissionResource(t, "train", "1", "1", "1Gi")},
	})), platform.RouteSpec{})

	assertSchedulerStatus(t, code, data, http.StatusOK)
	review := data.(map[string]any)
	if review["allowed"] != true || review["queue_name"] != "default-batch" {
		t.Fatalf("admission review = %#v, want allowed default-batch", review)
	}
	if review["priority_value"] != 1000 || review["preemptible"] != true || review["runtime_limit_seconds"] != 3600 {
		t.Fatalf("admission review = %#v, want trusted queue priority/preemptible/runtime metadata", review)
	}
	if _, found := app.Store.Get(context.Background(), submitAdmissionsResource, "P1/U1/default-batch"); !found {
		t.Fatal("allowed admission was not recorded")
	}
	if len(app.Events.Outbox()) == 0 {
		t.Fatal("allowed admission did not publish a domain event")
	}
}

func TestSubmitAdmissionResolvesNetworkProfileMetadata(t *testing.T) {
	app := newSchedulerQuotaTestApp()
	seedAdmissionProject(t, app, admissionFixture{})

	code, data, _ := reviewSubmitAdmission(app, schedulerRequest(http.MethodPost, "/api/v1/internal/scheduler/admission", admissionBody(t, map[string]any{
		"project_id":           "P1",
		"user_id":              "U1",
		"queue_name":           "default-batch",
		"required_cpu":         1,
		"required_memory":      1024,
		"network_profile":      "rdma-fast-lane",
		"rdma_required":        true,
		"topology_requirement": "same-rack",
		"resources":            []any{podAdmissionResource(t, "train", "0", "1", "1Gi")},
	})), platform.RouteSpec{})

	assertSchedulerStatus(t, code, data, http.StatusOK)
	review := data.(map[string]any)
	if review["network_profile"] != "rdma-fast-lane" || review["rdma_required"] != true {
		t.Fatalf("admission network review = %#v, want rdma-fast-lane required", review)
	}
	if review["nic_class"] != "rdma" || review["topology_requirement"] != "same-rack" {
		t.Fatalf("admission network hints = %#v, want rdma same-rack", review)
	}
	annotations := review["network_annotations"].(map[string]any)
	if annotations[multusNetworksAnnotation] != "nexuspaas-system/rdma-net" {
		t.Fatalf("network annotations = %#v, want rdma-net", annotations)
	}
	env := review["network_env"].(map[string]any)
	if env["NCCL_SOCKET_IFNAME"] != "net1" || env["NCCL_IB_DISABLE"] != "0" {
		t.Fatalf("network env = %#v, want NCCL defaults", env)
	}
}

func TestSubmitAdmissionRejectsMissingOrDisabledNetworkProfile(t *testing.T) {
	tests := []struct {
		name    string
		profile string
		seed    map[string]any
		want    string
	}{
		{name: "missing", profile: "missing-net", want: "network profile not found"},
		{name: "disabled", profile: "disabled-net", seed: map[string]any{
			"id":          "disabled-net",
			"name":        "Disabled net",
			"primary_cni": "cilium",
			"enabled":     false,
		}, want: "network profile is disabled"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := newSchedulerQuotaTestApp()
			seedAdmissionProject(t, app, admissionFixture{})
			if tt.seed != nil {
				createSchedulerRecord(t, app, networkProfilesResource, tt.seed)
			}

			code, data, _ := reviewSubmitAdmission(app, schedulerRequest(http.MethodPost, "/api/v1/internal/scheduler/admission", admissionBody(t, map[string]any{
				"project_id":      "P1",
				"user_id":         "U1",
				"queue_name":      "default-batch",
				"required_cpu":    1,
				"required_memory": 1024,
				"network_profile": tt.profile,
			})), platform.RouteSpec{})

			assertSchedulerStatus(t, code, data, http.StatusUnprocessableEntity)
			if !strings.Contains(data.(map[string]any)["reason"].(string), tt.want) {
				t.Fatalf("network denial = %#v, want %q", data, tt.want)
			}
		})
	}
}

func TestSubmitAdmissionRejectsQueueOutsideProjectPlan(t *testing.T) {
	app := newSchedulerQuotaTestApp()
	seedAdmissionProject(t, app, admissionFixture{})

	code, data, _ := reviewSubmitAdmission(app, schedulerRequest(http.MethodPost, "/api/v1/internal/scheduler/admission", admissionBody(t, map[string]any{
		"project_id":      "P1",
		"user_id":         "U1",
		"queue_name":      "blocked",
		"required_gpu":    0,
		"required_cpu":    1,
		"required_memory": 1024,
	})), platform.RouteSpec{})

	assertSchedulerStatus(t, code, data, http.StatusForbidden)
	if !strings.Contains(data.(map[string]any)["reason"].(string), "queue is not allowed") {
		t.Fatalf("queue denial = %#v, want queue reason", data)
	}
}

func TestSubmitAdmissionRejectsClosedPlanWindow(t *testing.T) {
	app := newSchedulerQuotaTestApp()
	now := time.Date(2026, 6, 15, 10, 0, 0, 0, time.UTC)
	seedAdmissionProject(t, app, admissionFixture{weekWindows: []map[string]any{{"start": 0, "end": 3600}}})
	req := submitAdmissionRequest{ProjectID: "P1", UserID: "U1", QueueName: "default-batch", RequiredCPU: 1, RequiredMemory: 1024}

	_, err := evaluateSubmitAdmission(context.Background(), newAdmissionReader(app.Store), req, now)

	if err == nil || !strings.Contains(err.Error(), "project has no active resource plan") {
		t.Fatalf("closed plan window err = %v, want inactive plan", err)
	}
}

func TestSubmitAdmissionRejectsProjectQuotaExceededByActiveUsage(t *testing.T) {
	app := newSchedulerQuotaTestApp()
	seedAdmissionProject(t, app, admissionFixture{gpuLimit: 2})
	createSchedulerRecord(t, app, workloadJobsResource, map[string]any{
		"id":           "J0",
		"project_id":   "P1",
		"user_id":      "U2",
		"status":       "running",
		"required_gpu": 1.5,
	})

	code, data, _ := reviewSubmitAdmission(app, schedulerRequest(http.MethodPost, "/api/v1/internal/scheduler/admission", admissionBody(t, map[string]any{
		"project_id":        "P1",
		"user_id":           "U1",
		"queue_name":        "default-batch",
		"device_class_name": "gpu.nvidia.com",
		"required_gpu":      1,
		"required_cpu":      1,
		"required_memory":   1024,
	})), platform.RouteSpec{})

	assertSchedulerStatus(t, code, data, http.StatusConflict)
	denial := data.(map[string]any)
	if !strings.Contains(denial["reason"].(string), "GPU quota exceeded") {
		t.Fatalf("quota denial = %#v, want GPU quota reason", data)
	}
	if denial["queue_name"] != "default-batch" || denial["priority_value"] != 1000 || denial["runtime_limit_seconds"] != 3600 {
		t.Fatalf("quota denial = %#v, want queue admission metadata preserved", denial)
	}
}

func TestSubmitAdmissionRejectsUserQuotaExceeded(t *testing.T) {
	app := newSchedulerQuotaTestApp()
	seedAdmissionProject(t, app, admissionFixture{})
	createSchedulerRecord(t, app, userQuotasResource, map[string]any{
		"id":              "P1/U1",
		"project_id":      "P1",
		"user_id":         "U1",
		"cpu_limit":       1.5,
		"memory_limit_gb": 8,
	})

	code, data, _ := reviewSubmitAdmission(app, schedulerRequest(http.MethodPost, "/api/v1/internal/scheduler/admission", admissionBody(t, map[string]any{
		"project_id":      "P1",
		"user_id":         "U1",
		"queue_name":      "default-batch",
		"required_gpu":    0,
		"required_cpu":    2,
		"required_memory": 1024,
	})), platform.RouteSpec{})

	assertSchedulerStatus(t, code, data, http.StatusConflict)
	if !strings.Contains(data.(map[string]any)["reason"].(string), "user CPU quota exceeded") {
		t.Fatalf("user quota denial = %#v, want user CPU quota reason", data)
	}
}

func TestSubmitAdmissionRejectsPayloadResourceUnderreporting(t *testing.T) {
	app := newSchedulerQuotaTestApp()
	seedAdmissionProject(t, app, admissionFixture{})

	code, data, _ := reviewSubmitAdmission(app, schedulerRequest(http.MethodPost, "/api/v1/internal/scheduler/admission", admissionBody(t, map[string]any{
		"project_id":      "P1",
		"user_id":         "U1",
		"queue_name":      "default-batch",
		"required_gpu":    1,
		"required_cpu":    1,
		"required_memory": 1024,
		"resources":       []any{podAdmissionResource(t, "train", "2", "1", "1Gi")},
	})), platform.RouteSpec{})

	assertSchedulerStatus(t, code, data, http.StatusUnprocessableEntity)
	if !strings.Contains(data.(map[string]any)["reason"].(string), "declared GPU") {
		t.Fatalf("floor denial = %#v, want declared GPU reason", data)
	}
}

func TestSubmitAdmissionRejectsOversizedManifestResource(t *testing.T) {
	app := newSchedulerQuotaTestApp()
	app.Config.MaxConfigFileBytes = 8

	code, data, _ := reviewSubmitAdmission(app, schedulerRequest(http.MethodPost, "/api/v1/internal/scheduler/admission", admissionBody(t, map[string]any{
		"project_id": "P1",
		"user_id":    "U1",
		"resources": []any{
			map[string]any{"name": "big", "manifest": "kind: Deployment"},
		},
	})), platform.RouteSpec{})

	assertSchedulerStatus(t, code, data, http.StatusRequestEntityTooLarge)
	if !strings.Contains(data.(map[string]any)["reason"].(string), "max byte size") {
		t.Fatalf("oversize denial = %#v, want byte-size reason", data)
	}
}

func TestSubmitAdmissionRejectsTooManyManifestDocuments(t *testing.T) {
	app := newSchedulerQuotaTestApp()
	app.Config.MaxConfigFileDocuments = 1

	code, data, _ := reviewSubmitAdmission(app, schedulerRequest(http.MethodPost, "/api/v1/internal/scheduler/admission", admissionBody(t, map[string]any{
		"project_id": "P1",
		"user_id":    "U1",
		"resources": []any{
			map[string]any{"name": "docs", "manifest": "kind: Pod\n---\nkind: Service"},
		},
	})), platform.RouteSpec{})

	assertSchedulerStatus(t, code, data, http.StatusUnprocessableEntity)
	if !strings.Contains(data.(map[string]any)["reason"].(string), "document count") {
		t.Fatalf("document denial = %#v, want document-count reason", data)
	}
}

func TestSubmitAdmissionRejectsRawSecretAndPublishesSafeAudit(t *testing.T) {
	app := newSchedulerQuotaTestApp()
	rawSecret := `{"apiVersion":"v1","kind":"Secret","metadata":{"name":"db-creds"},"stringData":{"password":"super-secret"}}`

	code, data, _ := reviewSubmitAdmission(app, schedulerRequest(http.MethodPost, "/api/v1/internal/scheduler/admission", admissionBody(t, map[string]any{
		"job_id":     "J-secret",
		"project_id": "P1",
		"user_id":    "U1",
		"resources": []any{
			map[string]any{"name": "db-creds", "manifest": rawSecret},
		},
	})), platform.RouteSpec{})

	assertSchedulerStatus(t, code, data, http.StatusForbidden)
	response := data.(map[string]any)
	if !strings.Contains(response["reason"].(string), "raw Kubernetes Secret resources are rejected") {
		t.Fatalf("secret denial = %#v, want raw Secret policy reason", response)
	}
	rawResponse, _ := json.Marshal(response)
	if strings.Contains(string(rawResponse), "super-secret") {
		t.Fatalf("secret denial leaked plaintext: %s", rawResponse)
	}

	domainEvent := requireSchedulerEvent(t, app, "SecretAccessRejected", "rejected")
	assertSchedulerEventValue(t, domainEvent.Data, "resource_type", "secret")
	assertSchedulerEventValue(t, domainEvent.Data, "resource_name", "db-creds")
	auditEvent := requireSchedulerEvent(t, app, "AuditEvent", "rejected")
	assertSchedulerEventValue(t, auditEvent.Data, "resource_type", "secret")
	assertSchedulerEventValue(t, auditEvent.Data, "success", false)
	allEvents, _ := json.Marshal(app.Events.Outbox())
	if strings.Contains(string(allEvents), "super-secret") {
		t.Fatalf("secret rejection event leaked plaintext: %s", allEvents)
	}

	if _, found := admissionSecretPolicyViolationFromRequest(submitAdmissionRequest{
		Resources: []admissionResourcePayload{{Name: "external", Kind: "ExternalSecret", Raw: []byte(`{"apiVersion":"external-secrets.io/v1","kind":"ExternalSecret"}`)}},
	}); found {
		t.Fatal("ExternalSecret profile should not be rejected as a raw Kubernetes Secret")
	}
}

func TestSubmitAdmissionAllowsOwnerGroupMemberAndDefaultQueue(t *testing.T) {
	t.Setenv("DEFAULT_QUEUE_NAME", "default-batch")
	app := newSchedulerQuotaTestApp()
	seedAdmissionProject(t, app, admissionFixture{withoutMember: true, ownerID: "G1"})
	createSchedulerRecord(t, app, userGroupsResource, map[string]any{
		"id":       "G1/U2",
		"group_id": "G1",
		"user_id":  "U2",
		"role":     "member",
	})

	review, err := evaluateSubmitAdmission(context.Background(), newAdmissionReader(app.Store), submitAdmissionRequest{
		ProjectID: "P1", UserID: "U2", RequiredCPU: 1, RequiredMemory: 1024,
	}, time.Now().UTC())

	if err != nil {
		t.Fatalf("group-member admission err = %v, want allowed", err)
	}
	if review.QueueName != "default-batch" || review.DeviceClassName != "" {
		t.Fatalf("normalized admission = %#v, want default queue and CPU-only device class cleared", review)
	}
}

func TestSubmitAdmissionRejectsPerUserRunningAndQueuedLimits(t *testing.T) {
	tests := []struct {
		name    string
		project map[string]any
		jobs    []map[string]any
		want    string
	}{
		{
			name:    "running",
			project: map[string]any{"max_concurrent_jobs_per_user": 1},
			jobs:    []map[string]any{{"id": "run-1", "status": "running"}},
			want:    "max concurrent jobs exceeded",
		},
		{
			name:    "queued",
			project: map[string]any{"max_queued_jobs_per_user": 2},
			jobs: []map[string]any{
				{"id": "queue-1", "status": "submitted"},
				{"id": "queue-2", "status": "waiting_infra"},
			},
			want: "max queued jobs exceeded",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := newSchedulerQuotaTestApp()
			seedAdmissionProject(t, app, admissionFixture{projectOverrides: tt.project})
			for _, job := range tt.jobs {
				job["project_id"] = "P1"
				job["user_id"] = "U1"
				createSchedulerRecord(t, app, workloadJobsResource, job)
			}

			_, err := evaluateSubmitAdmission(context.Background(), newAdmissionReader(app.Store), submitAdmissionRequest{
				ProjectID: "P1", UserID: "U1", QueueName: "default-batch", RequiredCPU: 1, RequiredMemory: 1024,
			}, time.Now().UTC())

			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("limit err = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestAdmissionResourceFloorCoversControllersAndDRA(t *testing.T) {
	req := submitAdmissionRequest{
		RequiredGPU:    5,
		RequiredCPU:    5,
		RequiredMemory: 4096,
		Resources: []admissionResourcePayload{
			{Name: "deploy", Raw: mustJSON(t, workloadAdmissionObject("Deployment", map[string]any{
				"replicas": 2,
				"template": map[string]any{
					"spec": podSpecAdmissionObject("1", "500m", "512Mi"),
				},
			}))},
			{Name: "job", Raw: mustJSON(t, workloadAdmissionObject("Job", map[string]any{
				"parallelism": 3,
				"template": map[string]any{
					"spec": podSpecAdmissionObject("0", "1", "1Gi"),
				},
			}))},
			{Name: "claim", Raw: mustJSON(t, draAdmissionClaim("ResourceClaim", 2, 50))},
			{Name: "template", Raw: mustJSON(t, draAdmissionClaim("ResourceClaimTemplate", 1, 100))},
		},
	}

	floor, err := admissionResourceFloorFromRequest(req)

	if err != nil {
		t.Fatalf("resource floor err = %v", err)
	}
	if floor.gpu != 4 || floor.cpu != 4 || floor.memoryMB != 4096 {
		t.Fatalf("resource floor = %#v, want gpu=4 cpu=4 memory=4096", floor)
	}
}

func TestAdmissionResourceAccountingRejectsInvalidInputs(t *testing.T) {
	smZero := 0
	tests := []struct {
		name string
		req  submitAdmissionRequest
		want string
	}{
		{name: "required gpu", req: submitAdmissionRequest{RequiredGPU: -1}, want: "required GPU must be non-negative"},
		{name: "required cpu", req: submitAdmissionRequest{RequiredCPU: -1}, want: "required CPU must be non-negative"},
		{name: "required memory", req: submitAdmissionRequest{RequiredMemory: -1}, want: "required memory must be non-negative"},
		{name: "dra gpu count", req: submitAdmissionRequest{GPUCount: -1}, want: "DRA GPU count must be non-negative"},
		{name: "dra sm percentage", req: submitAdmissionRequest{SMPercentage: &smZero}, want: "DRA SM percentage must be between 1 and 100"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := admissionResourceFloorFromRequest(tt.req)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("resource accounting err = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestAdmissionResourcePolicyRejectsRawSecrets(t *testing.T) {
	req := submitAdmissionRequest{Resources: []admissionResourcePayload{
		{Name: "raw-secret", Raw: mustJSON(t, map[string]any{
			"kind":     "Secret",
			"metadata": map[string]any{"name": "db-password"},
		})},
	}}

	_, err := admissionResourceFloorFromRequest(req)

	var violation admissionSecretPolicyViolation
	if err == nil || !strings.Contains(err.Error(), "raw Kubernetes Secret resources are rejected") {
		t.Fatalf("secret policy err = %v, want raw secret rejection", err)
	}
	if !strings.Contains(err.Error(), rawSecretPolicyReason()) {
		t.Fatalf("secret policy err = %v, want shared reason", err)
	}
	violation, _ = err.(admissionSecretPolicyViolation)
	if violation.ResourceName != "db-password" || violation.ResourceKind != "Secret" {
		t.Fatalf("secret violation = %#v, want named Secret", violation)
	}
}

func TestAdmissionSecretPolicyViolationFromRequestUsesExplicitAndRawMetadata(t *testing.T) {
	explicit, found := admissionSecretPolicyViolationFromRequest(submitAdmissionRequest{Resources: []admissionResourcePayload{
		{Name: "explicit-secret", Kind: "Secret"},
	}})
	if !found || explicit.ResourceName != "explicit-secret" || explicit.ResourceKind != "Secret" {
		t.Fatalf("explicit secret violation = %#v found=%v, want explicit metadata", explicit, found)
	}

	raw, found := admissionSecretPolicyViolationFromRequest(submitAdmissionRequest{Resources: []admissionResourcePayload{
		{Raw: mustJSON(t, map[string]any{"kind": "Secret", "metadata": map[string]any{"name": "raw-secret"}})},
	}})
	if !found || raw.ResourceName != "raw-secret" || raw.ResourceKind != "Secret" {
		t.Fatalf("raw secret violation = %#v found=%v, want raw metadata", raw, found)
	}

	_, found = admissionSecretPolicyViolationFromRequest(submitAdmissionRequest{Resources: []admissionResourcePayload{
		{Name: "config", Kind: "ConfigMap"},
	}})
	if found {
		t.Fatal("ConfigMap was reported as a secret policy violation")
	}
}

func TestSubmitAdmissionStreamingGuardrails(t *testing.T) {
	tests := []struct {
		name    string
		jobs    []map[string]any
		req     submitAdmissionRequest
		want    string
		allowed bool
	}{
		{
			name: "allowed",
			req: submitAdmissionRequest{
				ProjectID: "P1", UserID: "U1", QueueName: "default-batch", RequiredCPU: 1, RequiredMemory: 1024,
				StreamingSession: true, StreamMaxBitrateKbps: 12000, StreamBitrateCapKbps: 12000, StreamSessionCap: 2, StreamEgressBudgetKbps: 24000,
			},
			allowed: true,
		},
		{
			name: "session cap",
			jobs: []map[string]any{{"id": "stream-1", "status": "running", "streaming_session": true, "stream_max_bitrate_kbps": 12000}},
			req: submitAdmissionRequest{
				ProjectID: "P1", UserID: "U1", QueueName: "default-batch", RequiredCPU: 1, RequiredMemory: 1024,
				StreamingSession: true, StreamMaxBitrateKbps: 12000, StreamBitrateCapKbps: 12000, StreamSessionCap: 1, StreamEgressBudgetKbps: 24000,
			},
			want: "stream session cap exceeded",
		},
		{
			name: "per session bitrate",
			req: submitAdmissionRequest{
				ProjectID: "P1", UserID: "U1", QueueName: "default-batch", RequiredCPU: 1, RequiredMemory: 1024,
				StreamingSession: true, StreamMaxBitrateKbps: 13000, StreamBitrateCapKbps: 12000, StreamSessionCap: 2, StreamEgressBudgetKbps: 24000,
			},
			want: "stream bitrate cap exceeded",
		},
		{
			name: "egress budget",
			jobs: []map[string]any{{"id": "stream-1", "status": "running", "streaming_session": true, "stream_max_bitrate_kbps": 19000}},
			req: submitAdmissionRequest{
				ProjectID: "P1", UserID: "U1", QueueName: "default-batch", RequiredCPU: 1, RequiredMemory: 1024,
				StreamingSession: true, StreamMaxBitrateKbps: 12000, StreamBitrateCapKbps: 12000, StreamSessionCap: 3, StreamEgressBudgetKbps: 30000,
			},
			want: "stream egress budget exceeded",
		},
		{
			name: "active non-streaming jobs excluded from stream budgets",
			jobs: []map[string]any{
				{"id": "non-stream-1", "status": "running", "streaming_session": false, "stream_max_bitrate_kbps": 40000},
				{"id": "non-stream-2", "status": "queued", "streaming_session": false, "stream_max_bitrate_kbps": 40000},
				{"id": "non-stream-3", "status": "waiting_infra", "streaming_session": false, "stream_max_bitrate_kbps": 40000},
			},
			req: submitAdmissionRequest{
				ProjectID: "P1", UserID: "U1", QueueName: "default-batch", RequiredCPU: 1, RequiredMemory: 1024,
				StreamingSession: true, StreamMaxBitrateKbps: 12000, StreamBitrateCapKbps: 12000, StreamSessionCap: 1, StreamEgressBudgetKbps: 12000,
			},
			allowed: true,
		},
		{
			name: "terminal streaming jobs excluded from stream budgets",
			jobs: []map[string]any{
				{"id": "stream-succeeded", "status": "succeeded", "streaming_session": true, "stream_max_bitrate_kbps": 40000},
				{"id": "stream-failed", "status": "failed", "streaming_session": true, "stream_max_bitrate_kbps": 40000},
				{"id": "stream-cancelled", "status": "cancelled", "streaming_session": true, "stream_max_bitrate_kbps": 40000},
				{"id": "stream-canceled", "status": "canceled", "streaming_session": true, "stream_max_bitrate_kbps": 40000},
				{"id": "stream-completed", "status": "completed", "streaming_session": true, "stream_max_bitrate_kbps": 40000},
				{"id": "stream-timed-out", "status": "timed_out", "streaming_session": true, "stream_max_bitrate_kbps": 40000},
			},
			req: submitAdmissionRequest{
				ProjectID: "P1", UserID: "U1", QueueName: "default-batch", RequiredCPU: 1, RequiredMemory: 1024,
				StreamingSession: true, StreamMaxBitrateKbps: 12000, StreamBitrateCapKbps: 12000, StreamSessionCap: 1, StreamEgressBudgetKbps: 12000,
			},
			allowed: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := newSchedulerQuotaTestApp()
			seedAdmissionProject(t, app, admissionFixture{})
			seedStreamingAdmissionJobs(t, app, tt.jobs)

			review, err := evaluateSubmitAdmission(context.Background(), newAdmissionReader(app.Store), tt.req, time.Now().UTC())

			if tt.allowed {
				requireStreamingAdmissionAllowed(t, review, err, tt.req)
				return
			}
			requireStreamingAdmissionRejected(t, err, tt.want)
		})
	}
}

func seedStreamingAdmissionJobs(t *testing.T, app *platform.App, jobs []map[string]any) {
	t.Helper()
	for _, job := range jobs {
		job["project_id"] = "P1"
		job["user_id"] = "U2"
		createSchedulerRecord(t, app, workloadJobsResource, job)
	}
}

func requireStreamingAdmissionAllowed(t *testing.T, review admissionReview, err error, req submitAdmissionRequest) {
	t.Helper()
	if err != nil {
		t.Fatalf("stream admission err = %v, want allowed", err)
	}
	if !review.StreamingSession || review.StreamMaxBitrateKbps != req.StreamMaxBitrateKbps {
		t.Fatalf("stream review = %#v, want stream metadata preserved", review)
	}
}

func requireStreamingAdmissionRejected(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("stream admission err = %v, want %q", err, want)
	}
	denial, ok := err.(admissionDeny)
	if !ok || denial.status != http.StatusConflict {
		t.Fatalf("stream admission err = %T(%v), want admissionDeny status 409", err, err)
	}
}

func TestSubmitAdmissionRejectsUnsupportedWorkloadKind(t *testing.T) {
	app := newSchedulerQuotaTestApp()
	seedAdmissionProject(t, app, admissionFixture{})

	code, data, _ := reviewSubmitAdmission(app, schedulerRequest(http.MethodPost, "/api/v1/internal/scheduler/admission", admissionBody(t, map[string]any{
		"project_id":      "P1",
		"user_id":         "U1",
		"queue_name":      "default-batch",
		"required_gpu":    0,
		"required_cpu":    1,
		"required_memory": 1024,
		"resources": []any{map[string]any{
			"name":      "workers",
			"kind":      "StatefulSet",
			"json_data": string(mustJSON(t, workloadAdmissionObject("StatefulSet", map[string]any{}))),
		}},
	})), platform.RouteSpec{})

	assertSchedulerStatus(t, code, data, http.StatusUnprocessableEntity)
	if !strings.Contains(data.(map[string]any)["reason"].(string), "unsupported workload kind StatefulSet") {
		t.Fatalf("unsupported kind denial = %#v, want StatefulSet reason", data)
	}
}

func TestAdmissionPayloadDecodeAndQuantityHelpers(t *testing.T) {
	req, err := decodeSubmitAdmissionRequest(map[string]any{
		"jobId":                "J1",
		"projectId":            "P1",
		"userId":               "U1",
		"queueName":            "q",
		"requiredGpu":          1.5,
		"requiredCPU":          2,
		"requiredMemory":       "2Gi",
		"gpuCount":             2,
		"smPercentage":         50,
		"streamingSession":     true,
		"streamMaxBitrateKbps": 9000,
		"deviceClassName":      "gpu.nvidia.com",
		"resources": []any{
			map[string]any{"name": "raw", "json": map[string]any{"apiVersion": "v1", "kind": "Pod"}},
			map[string]any{"name": "direct", "apiVersion": "v1", "kind": "Pod"},
			map[string]any{"name": "empty", "json_data": ""},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if req.JobID != "J1" || req.RequiredMemory != 2048 || req.GPUCount != 2 || req.SMPercentage == nil || *req.SMPercentage != 50 ||
		!req.StreamingSession || req.StreamMaxBitrateKbps != 9000 {
		t.Fatalf("decoded request = %#v, want aliases, memory quantity, and SM percentage", req)
	}
	if len(req.Resources) != 2 || req.Resources[0].Kind != "Pod" || req.Resources[1].Kind != "Pod" {
		t.Fatalf("decoded resources = %#v, want two Pod payloads", req.Resources)
	}

	if got := parseAdmissionGPU("3"); got != 3 {
		t.Fatalf("parseAdmissionGPU string = %v, want 3", got)
	}
	if got := parseAdmissionGPU(int64(2)); got != 2 {
		t.Fatalf("parseAdmissionGPU int64 = %v, want 2", got)
	}
	if got := parseAdmissionGPU(false); got != 0 {
		t.Fatalf("parseAdmissionGPU bool = %v, want 0", got)
	}
	if got := parseAdmissionCPU("500m"); got < 0.49 || got > 0.51 {
		t.Fatalf("parseAdmissionCPU 500m = %v, want about 0.5", got)
	}
	if got := parseAdmissionCPU(int64(4)); got != 4 {
		t.Fatalf("parseAdmissionCPU int64 = %v, want 4", got)
	}
	if got := parseAdmissionCPU("bad"); got != 0 {
		t.Fatalf("parseAdmissionCPU bad = %v, want 0", got)
	}
	if got := parseAdmissionMemory("1536Mi"); got != 1536 {
		t.Fatalf("parseAdmissionMemory 1536Mi = %v, want 1536", got)
	}
	if got := admissionMemoryMB(map[string]any{"fallback": 512.0}, "missing", "fallback"); got != 512 {
		t.Fatalf("admissionMemoryMB fallback = %v, want 512", got)
	}
}

func TestAdmissionWindowAndListHelpers(t *testing.T) {
	now := time.Date(2026, 6, 15, 1, 0, 0, 0, time.UTC)
	active := map[string]any{
		"valid_from":   now.Add(-time.Hour).Format(time.RFC3339),
		"valid_until":  now.Add(time.Hour),
		"week_windows": `[{"start":0,"end":7200}]`,
	}
	if !admissionPlanActive(active, now) {
		t.Fatal("plan with active RFC3339/time windows was not active")
	}
	if admissionPlanActive(map[string]any{"valid_from": now.Add(time.Hour)}, now) {
		t.Fatal("future valid_from plan was active")
	}
	if admissionPlanActive(map[string]any{"valid_until": now.Add(-time.Hour).Format(time.RFC3339)}, now) {
		t.Fatal("expired plan was active")
	}
	if admissionPlanActive(map[string]any{"week_windows": []any{map[string]any{"start": 7200.0, "end": 7300.0}}}, now) {
		t.Fatal("closed week window was active")
	}
	if len(admissionWeekWindows(map[string]any{"weekWindows": []any{map[string]any{"start": 1.0, "end": 2.0}}})) != 1 {
		t.Fatal("weekWindows []any alias was not decoded")
	}
	if len(admissionWeekWindows(map[string]any{"WeekWindows": "not json"})) != 0 {
		t.Fatal("invalid week window string should decode to empty")
	}

	if got := admissionStringList(map[string]any{"models": []any{" a ", "", "b"}}, "models"); strings.Join(got, ",") != "a,b" {
		t.Fatalf("admissionStringList []any = %v, want a,b", got)
	}
	if got := admissionStringList(map[string]any{"models": `["x","y"]`}, "models"); strings.Join(got, ",") != "x,y" {
		t.Fatalf("admissionStringList JSON = %v, want x,y", got)
	}
	if got := admissionStringList(map[string]any{"models": `bad`}, "models"); len(got) != 0 {
		t.Fatalf("admissionStringList invalid JSON = %v, want empty", got)
	}
}

func TestAdmissionAccountingValidationAndRawResources(t *testing.T) {
	tests := []struct {
		name string
		req  submitAdmissionRequest
	}{
		{name: "negative gpu", req: submitAdmissionRequest{RequiredGPU: -1}},
		{name: "negative cpu", req: submitAdmissionRequest{RequiredCPU: -1}},
		{name: "negative memory", req: submitAdmissionRequest{RequiredMemory: -1}},
		{name: "negative dra", req: submitAdmissionRequest{GPUCount: -1}},
		{name: "bad sm", req: submitAdmissionRequest{SMPercentage: intPtr(0)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := validateAdmissionResourceAccounting(tt.req); err == nil {
				t.Fatal("expected accounting validation error")
			}
		})
	}
	if err := validateAdmissionResourceAccounting(submitAdmissionRequest{RequiredGPU: 1, GPUCount: 1, SMPercentage: intPtr(100)}); err != nil {
		t.Fatalf("valid accounting rejected: %v", err)
	}

	raw := resourceRawJSON(map[string]any{"json_data": `{"apiVersion":"v1","kind":"Pod"}`})
	if !strings.Contains(string(raw), `"Pod"`) {
		t.Fatalf("raw string resource = %s, want Pod JSON", raw)
	}
	raw = resourceRawJSON(map[string]any{"object": map[string]any{"apiVersion": "v1", "kind": "Pod"}})
	if !strings.Contains(string(raw), `"Pod"`) {
		t.Fatalf("object resource = %s, want Pod JSON", raw)
	}
	raw = resourceRawJSON(map[string]any{"apiVersion": "v1", "kind": "Pod"})
	if !strings.Contains(string(raw), `"Pod"`) {
		t.Fatalf("direct resource = %s, want Pod JSON", raw)
	}
	if raw := resourceRawJSON(map[string]any{"name": "empty"}); len(raw) != 0 {
		t.Fatalf("empty resource raw = %s, want empty", raw)
	}
	if got := kindFromRaw([]byte(`{`)); got != "" {
		t.Fatalf("kindFromRaw invalid = %q, want empty", got)
	}
}

func TestAdmissionUserQuotaMemoryAndDefaults(t *testing.T) {
	if got := schedulerDefaultQueueName(); got != defaultQueueName {
		t.Fatalf("schedulerDefaultQueueName = %q, want package default", got)
	}
	app := newSchedulerQuotaTestApp()
	createSchedulerRecord(t, app, userQuotasResource, map[string]any{
		"id":              "P1/U1",
		"project_id":      "P1",
		"user_id":         "U1",
		"memory_limit_gb": 1,
	})
	err := enforceAdmissionUserQuota(context.Background(), newAdmissionReader(app.Store), submitAdmissionRequest{
		ProjectID: "P1", UserID: "U1", RequiredMemory: 2048,
	}, admissionUsage{})
	if err == nil || !strings.Contains(err.Error(), "user Memory quota exceeded") {
		t.Fatalf("memory quota err = %v, want user memory quota exceeded", err)
	}
	if quota, found := admissionUserQuota(context.Background(), newAdmissionReader(app.Store), "P1", "U1"); !found || quota.ID != "P1/U1" {
		t.Fatalf("admissionUserQuota = %#v found=%v, want P1/U1", quota, found)
	}
	if _, found := admissionUserQuota(context.Background(), newAdmissionReader(app.Store), "P1", "missing"); found {
		t.Fatal("missing user quota unexpectedly found")
	}
	if gpuFraction(0, 100) != 0 || gpuFraction(2, 150) != 2 || gpuFraction(2, 25) != 0.5 {
		t.Fatal("gpuFraction did not normalize edge cases")
	}
}

func TestSubmitAdmissionUsesConfiguredDefaultQueue(t *testing.T) {
	app := platform.NewApp(platform.Config{ServiceName: "all", HTTPAddr: ":0", DefaultQueueName: "interactive"})
	Register(app)
	createSchedulerRecord(t, app, queuesResource, map[string]any{"id": "q-interactive", "name": "interactive"})
	createSchedulerRecord(t, app, plansResource, map[string]any{
		"id": "plan-1", "name": "default", "cpu_limit_cores": 8.0, "memory_limit_gb": 16.0, "queue_ids": []string{"q-interactive"},
	})
	createSchedulerRecord(t, app, projectsResource, map[string]any{"id": "P1", "plan_id": "plan-1"})
	createSchedulerRecord(t, app, projectMembersResource, map[string]any{"id": "P1/U1", "project_id": "P1", "user_id": "U1", "role": "user"})

	code, data, _ := reviewSubmitAdmission(app, schedulerRequest(http.MethodPost, "/api/v1/internal/scheduler/admission", admissionBody(t, map[string]any{
		"project_id": "P1", "user_id": "U1", "required_cpu": 1, "required_memory": 1024,
	})), platform.RouteSpec{})

	assertSchedulerStatus(t, code, data, http.StatusOK)
	if data.(map[string]any)["queue_name"] != "interactive" {
		t.Fatalf("admission queue = %#v, want configured default", data)
	}
}

type admissionFixture struct {
	gpuLimit         float64
	weekWindows      []map[string]any
	withoutMember    bool
	ownerID          string
	projectOverrides map[string]any
}

func seedAdmissionProject(t *testing.T, app *platform.App, fixture admissionFixture) {
	t.Helper()
	gpuLimit := fixture.gpuLimit
	if gpuLimit == 0 {
		gpuLimit = 4
	}
	plan := map[string]any{
		"id":                 "plan-1",
		"name":               "default",
		"gpu_limit":          gpuLimit,
		"cpu_limit_cores":    8.0,
		"memory_limit_gb":    16.0,
		"queue_ids":          []string{"q1"},
		"allowed_gpu_models": []string{"gpu.nvidia.com"},
	}
	if fixture.weekWindows != nil {
		plan["week_windows"] = fixture.weekWindows
	}
	createSchedulerRecord(t, app, queuesResource, map[string]any{"id": "q1", "name": "default-batch", "priority_value": 1000, "is_preemptible": true, "max_runtime_seconds": 3600})
	createSchedulerRecord(t, app, plansResource, plan)
	project := map[string]any{
		"id":                           "P1",
		"plan_id":                      "plan-1",
		"max_concurrent_jobs_per_user": 3,
		"max_queued_jobs_per_user":     5,
	}
	if fixture.ownerID != "" {
		project["owner_id"] = fixture.ownerID
	}
	for key, value := range fixture.projectOverrides {
		project[key] = value
	}
	createSchedulerRecord(t, app, projectsResource, project)
	if fixture.withoutMember {
		return
	}
	createSchedulerRecord(t, app, projectMembersResource, map[string]any{
		"id":         "P1/U1",
		"project_id": "P1",
		"user_id":    "U1",
		"role":       "user",
	})
}

func admissionBody(t *testing.T, body map[string]any) string {
	t.Helper()
	data, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func podAdmissionResource(t *testing.T, name, gpu, cpu, memory string) map[string]any {
	t.Helper()
	data := mustJSON(t, map[string]any{
		"apiVersion": "v1",
		"kind":       "Pod",
		"metadata": map[string]any{
			"name": name,
		},
		"spec": map[string]any{
			"containers": []any{
				map[string]any{
					"name":  "main",
					"image": "busybox:latest",
					"resources": map[string]any{
						"limits": map[string]any{
							"nvidia.com/gpu": gpu,
							"cpu":            cpu,
							"memory":         memory,
						},
					},
				},
			},
		},
	})
	return map[string]any{"name": name, "kind": "Pod", "json_data": string(data)}
}

func workloadAdmissionObject(kind string, spec map[string]any) map[string]any {
	apiVersion := "apps/v1"
	if kind == "Job" {
		apiVersion = "batch/v1"
	}
	return map[string]any{
		"apiVersion": apiVersion,
		"kind":       kind,
		"metadata":   map[string]any{"name": strings.ToLower(kind)},
		"spec":       spec,
	}
}

func podSpecAdmissionObject(gpu, cpu, memory string) map[string]any {
	return map[string]any{
		"containers": []any{
			map[string]any{
				"name": "main",
				"resources": map[string]any{
					"limits": map[string]any{
						"nvidia.com/gpu": gpu,
						"cpu":            cpu,
						"memory":         memory,
					},
				},
			},
		},
	}
}

func draAdmissionClaim(kind string, count, smPct int) map[string]any {
	devices := map[string]any{
		"requests": []any{map[string]any{"exactly": map[string]any{"count": count}}},
		"config": []any{map[string]any{
			"opaque": map[string]any{
				"parameters": map[string]any{
					"sharing": map[string]any{
						"mpsConfig": map[string]any{"defaultActiveThreadPercentage": smPct},
					},
				},
			},
		}},
	}
	spec := map[string]any{"devices": devices}
	if kind == "ResourceClaimTemplate" {
		spec = map[string]any{"spec": spec}
	}
	return map[string]any{"apiVersion": "resource.k8s.io/v1", "kind": kind, "spec": spec}
}
