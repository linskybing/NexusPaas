//go:build e2e

package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/platform"
	"github.com/linskybing/nexuspaas/backend/internal/platform/cluster"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	orgIdentityUsersResource = "org-project-service:identity_users"
	workloadConfigsResource  = "workload-service:configfiles"
)

func TestLiveLDAPUserProjectPlanConfigDeployE2E(t *testing.T) {
	if strings.TrimSpace(os.Getenv("TEST_LIVE_K8S_USER_PROJECT_PLAN_DEPLOY")) != "1" {
		t.Skip("set TEST_LIVE_K8S_USER_PROJECT_PLAN_DEPLOY=1 to run live LDAP/project-plan/Kubernetes deploy e2e")
	}
	requireLiveKubeconfig(t)
	liveCtx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	cl, err := cluster.NewFromEnv("proj")
	if err != nil {
		t.Fatalf("create live Kubernetes client: %v", err)
	}
	if cl == nil {
		t.Fatal("live Kubernetes client is unavailable")
	}
	if err := cl.Ping(liveCtx); err != nil {
		t.Fatalf("ping live Kubernetes cluster: %v", err)
	}

	ldapCfg := liveLDAPConfigFromEnv()
	h := newHarnessWithPeers(t, map[string][]string{
		orgProjectService:     {identityService},
		schedulerQuotaService: {orgProjectService, workloadService},
		workloadService:       {identityService, orgProjectService, schedulerQuotaService},
	}, identityService, orgProjectService, schedulerQuotaService, workloadService)
	h.services[workloadService].app.Cluster = cl

	ldapIdentity := h.startExtraServiceWithConfig("identity-ldap-live", identityService, h.peerURLs(orgProjectService, schedulerQuotaService, workloadService), func(cfg *platform.Config) {
		cfg.LDAPEnabled = true
		cfg.LDAPHost = ldapCfg.host
		cfg.LDAPPort = ldapCfg.port
		cfg.LDAPUseTLS = ldapCfg.useTLS
		cfg.LDAPBindDN = ldapCfg.bindDN
		cfg.LDAPBindPassword = ldapCfg.bindPassword
		cfg.LDAPUserSearchBase = ldapCfg.searchBase
		cfg.LDAPUserFilter = ldapCfg.userFilter
		cfg.LDAPMirrorSyncInterval = 5 * time.Minute
	})

	adminID := "admin-" + h.runID
	adminIdentity := map[string]any{
		"username":      adminID,
		"name":          adminID,
		"password_hash": platform.HashSecret("admin-local-" + h.runID),
		"status":        "online",
		"role":          "admin",
		"role_id":       "RO2600001",
		"system_role":   0,
		"admin_panel":   true,
	}
	h.createRecord(identityUsersResource, adminID, adminIdentity)
	h.upsertE2ERecord(orgIdentityUsersResource, adminID, adminIdentity)

	createdUsername := "ldap" + h.runID
	createdPassword := "created-pass-" + h.runID
	h.doURLJSON(ldapIdentity.url, http.MethodPost, "/api/v1/register", map[string]any{
		"username":  createdUsername,
		"password":  createdPassword,
		"email":     createdUsername + "@example.org",
		"full_name": "LDAP Live E2E",
	}, "", http.StatusOK)
	requireLDAPEntry(t, ldapCfg, createdUsername, map[string]string{
		"cn":   "LDAP Live E2E",
		"mail": createdUsername + "@example.org",
	})

	login := h.doURLJSON(ldapIdentity.url, http.MethodPost, "/api/v1/login", map[string]any{
		"username": createdUsername,
		"password": createdPassword,
	}, "", http.StatusOK)
	token := fmt.Sprint(login.dataMap(t)["token"])
	if !strings.HasPrefix(token, "access.") {
		t.Fatalf("ldap login token = %q, want issued access token", token)
	}
	created := findE2EUserByUsername(t, h, createdUsername)
	requireOrgIdentityProjection(t, h, created.ID, createdUsername)

	suffix := truncateID(sanitizeID(h.runID), 20)
	groupID := "lg" + suffix
	projectID := "lp" + suffix
	deniedProjectID := "ld" + suffix
	queueID := "lq" + suffix
	planID := "lpl" + suffix
	configID := "cfg" + suffix
	jobID := "job" + suffix
	deniedJobID := "denyjob" + suffix

	h.doJSON(orgProjectService, http.MethodPost, "/api/v1/groups", map[string]any{
		"id":          groupID,
		"group_name":  "live group " + h.runID,
		"description": "live e2e group",
	}, h.apiKey, http.StatusCreated)
	h.doJSON(orgProjectService, http.MethodPost, "/api/v1/projects", map[string]any{
		"id":           projectID,
		"project_name": "live project " + h.runID,
		"g_id":         groupID,
		"description":  "live e2e project",
	}, h.apiKey, http.StatusCreated)
	h.doJSON(orgProjectService, http.MethodPost, "/api/v1/projects", map[string]any{
		"id":           deniedProjectID,
		"project_name": "denied live project " + h.runID,
		"g_id":         groupID,
		"description":  "live e2e non-member project",
	}, h.apiKey, http.StatusCreated)
	memberResult := h.doJSON(orgProjectService, http.MethodPost, "/api/v1/projects/"+projectID+"/members", map[string]any{
		"members": []map[string]any{{"user_id": created.ID, "role": "user"}},
	}, h.apiKey, http.StatusOK).dataMap(t)
	if got := fmt.Sprint(memberResult["succeeded"]); got != "1" {
		t.Fatalf("project member assignment result = %#v, want succeeded=1", memberResult)
	}

	h.doJSON(schedulerQuotaService, http.MethodPost, "/api/v1/queues", map[string]any{
		"id":             queueID,
		"name":           "default-batch",
		"priority_value": 1000,
	}, h.apiKey, http.StatusCreated)
	h.doJSON(schedulerQuotaService, http.MethodPost, "/api/v1/plans", map[string]any{
		"id":                 planID,
		"name":               "live plan " + h.runID,
		"gpu_limit":          0,
		"cpu_limit_cores":    2,
		"memory_limit_gb":    1,
		"queue_ids":          []string{queueID},
		"allowed_gpu_models": []string{},
	}, h.apiKey, http.StatusCreated)
	h.doJSON(schedulerQuotaService, http.MethodPut, "/api/v1/plans/bind/"+projectID, map[string]any{
		"plan_id": planID,
	}, h.apiKey, http.StatusOK)
	bound := h.getRecord(orgProjectsResource, projectID)
	if bound.Data["plan_id"] != planID || bound.Data["resource_plan_id"] != planID {
		t.Fatalf("bound project = %#v, want plan_id/resource_plan_id %s", bound.Data, planID)
	}

	h.doJSONBearer(workloadService, http.MethodPost, "/api/v1/configfiles?project_id="+projectID, map[string]any{
		"id":      configID,
		"name":    "job.yaml",
		"path":    "jobs/job.yaml",
		"content": "apiVersion: batch/v1\nkind: Job\n",
	}, token, http.StatusCreated)
	config := h.getRecord(workloadConfigsResource, configID)
	if config.Data["project_id"] != projectID {
		t.Fatalf("config project_id = %#v, want %s", config.Data["project_id"], projectID)
	}

	h.doJSONBearer(workloadService, http.MethodPost, "/api/v1/configfiles?project_id="+deniedProjectID, map[string]any{
		"id":   "denycfg" + suffix,
		"name": "denied.yaml",
	}, token, http.StatusForbidden)
	beforeDeniedAdmissions := len(h.listRecords(schedulerAdmissionsResource))
	h.doJSONBearer(workloadService, http.MethodPost, "/api/v1/jobs", map[string]any{
		"job_id":          deniedJobID,
		"project_id":      deniedProjectID,
		"user_id":         created.ID,
		"queue_name":      "default-batch",
		"required_cpu":    0.1,
		"required_memory": 32,
	}, token, http.StatusForbidden)
	if got := len(h.listRecords(schedulerAdmissionsResource)); got != beforeDeniedAdmissions {
		t.Fatalf("denied non-member submit created scheduler admissions: got %d want %d", got, beforeDeniedAdmissions)
	}
	if _, found := h.store.Get(h.ctx, workloadJobsResource, deniedJobID); found {
		t.Fatalf("denied non-member submit persisted job %s", deniedJobID)
	}

	namespace := liveDeployNamespace(projectID, created.ID)
	t.Cleanup(func() {
		err := cl.Clientset().CoreV1().Namespaces().Delete(context.Background(), namespace, metav1.DeleteOptions{})
		if err != nil && !apierrors.IsNotFound(err) {
			t.Logf("cleanup namespace %s: %v", namespace, err)
		}
	})
	jobName := "kjob-" + suffix
	h.doJSONBearer(workloadService, http.MethodPost, "/api/v1/jobs", map[string]any{
		"job_id":          jobID,
		"project_id":      projectID,
		"user_id":         created.ID,
		"queue_name":      "default-batch",
		"required_cpu":    0.1,
		"required_memory": 32,
		"config_id":       configID,
		"namespace":       namespace,
		"resources": []map[string]any{{
			"name":      jobName,
			"kind":      "Job",
			"json_data": livePauseJobManifest(t, jobName),
		}},
	}, token, http.StatusCreated)

	waitForLiveWorkloadDispatch(t, h, cl, namespace, jobName, jobID)
}

func requireLiveKubeconfig(t *testing.T) {
	t.Helper()
	if strings.TrimSpace(os.Getenv("KUBECONFIG")) != "" {
		return
	}
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("resolve home directory for kubeconfig: %v", err)
	}
	kubeconfig := filepath.Join(home, ".kube", "config")
	if _, err := os.Stat(kubeconfig); err != nil {
		t.Fatalf("default kubeconfig %s is unavailable: %v", kubeconfig, err)
	}
	t.Setenv("KUBECONFIG", kubeconfig)
}

func (h *e2eHarness) doJSONBearer(serviceName, method, path string, payload any, bearerToken string, want int) testResponse {
	h.t.Helper()
	var body bytes.Buffer
	if payload != nil {
		if err := json.NewEncoder(&body).Encode(payload); err != nil {
			h.t.Fatalf("marshal bearer request: %v", err)
		}
	}
	req := h.newRequest(serviceName, method, path, &body, "")
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Authorization", "Bearer "+bearerToken)
	req.Header.Set("Idempotency-Key", "idem-"+h.runID+"-"+sanitizeID(method+path+fmt.Sprint(time.Now().UnixNano())))
	return h.do(req, want)
}

func (h *e2eHarness) upsertE2ERecord(resource, id string, data map[string]any) {
	h.t.Helper()
	row := cloneMap(data)
	row["id"] = id
	row["e2e_run_id"] = h.runID
	if _, ok := h.store.Update(h.ctx, resource, id, row); ok {
		return
	}
	if _, err := h.store.Create(h.ctx, resource, row); err != nil {
		if platform.IsCreateConflict(err) {
			if _, ok := h.store.Update(h.ctx, resource, id, row); ok {
				return
			}
		}
		h.t.Fatalf("upsert %s/%s: %v", resource, id, err)
	}
}

func requireOrgIdentityProjection(t *testing.T, h *e2eHarness, userID, username string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		h.doJSON(orgProjectService, http.MethodGet, "/api/v1/groups", nil, h.apiKey, http.StatusOK)
		record, found := h.store.Get(h.ctx, orgIdentityUsersResource, userID)
		if found {
			if got := fmt.Sprint(record.Data["username"]); !strings.EqualFold(got, username) {
				t.Fatalf("org identity projection username = %q, want %q", got, username)
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("missing org identity projection for user %s", userID)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func liveDeployNamespace(projectID, userID string) string {
	project := truncateID(sanitizeID(projectID), 28)
	user := truncateID(sanitizeID(userID), 20)
	return truncateID("proj-"+project+"-"+user, 63)
}

func livePauseJobManifest(t *testing.T, name string) string {
	t.Helper()
	manifest := map[string]any{
		"apiVersion": "batch/v1",
		"kind":       "Job",
		"metadata": map[string]any{
			"name": name,
		},
		"spec": map[string]any{
			"backoffLimit": int64(0),
			"template": map[string]any{
				"spec": map[string]any{
					"restartPolicy": "Never",
					"containers": []map[string]any{{
						"name":  "pause",
						"image": "registry.k8s.io/pause:3.9",
						"resources": map[string]any{
							"requests": map[string]any{
								"cpu":    "10m",
								"memory": "16Mi",
							},
						},
					}},
				},
			},
		},
	}
	raw, err := json.Marshal(manifest)
	if err != nil {
		t.Fatalf("marshal live job manifest: %v", err)
	}
	return string(raw)
}

func waitForLiveWorkloadDispatch(t *testing.T, h *e2eHarness, cl *cluster.Client, namespace, jobName, jobID string) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for {
		h.services[workloadService].app.RunMaintenanceOnce(h.ctx, 200*time.Millisecond)
		if _, err := cl.Clientset().BatchV1().Jobs(namespace).Get(h.ctx, jobName, metav1.GetOptions{}); err != nil && !apierrors.IsNotFound(err) {
			t.Fatalf("get live Kubernetes Job %s/%s: %v", namespace, jobName, err)
		}
		record, found := h.store.Get(h.ctx, workloadJobsResource, jobID)
		if found && textE2E(record.Data["status"]) == "running" && len(recordListE2E(record.Data["created_resources"])) > 0 {
			if !createdResourcesContain(recordListE2E(record.Data["created_resources"]), "Job", namespace, jobName) {
				t.Fatalf("created_resources = %#v, want Job %s/%s", record.Data["created_resources"], namespace, jobName)
			}
			return
		}
		if time.Now().After(deadline) {
			record, _ := h.store.Get(h.ctx, workloadJobsResource, jobID)
			t.Fatalf("job %s did not dispatch to running with created resources before deadline; record=%#v", jobID, record.Data)
		}
		time.Sleep(250 * time.Millisecond)
	}
}

func recordListE2E(value any) []map[string]any {
	switch typed := value.(type) {
	case []map[string]any:
		return typed
	case []any:
		out := make([]map[string]any, 0, len(typed))
		for _, item := range typed {
			if row, ok := item.(map[string]any); ok {
				out = append(out, row)
			}
		}
		return out
	default:
		return nil
	}
}

func createdResourcesContain(resources []map[string]any, kind, namespace, name string) bool {
	for _, resource := range resources {
		if textE2E(resource["kind"]) == kind &&
			textE2E(resource["namespace"]) == namespace &&
			textE2E(resource["name"]) == name {
			return true
		}
	}
	return false
}
