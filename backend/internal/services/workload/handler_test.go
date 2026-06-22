package workload

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/linskybing/nexuspaas/backend/internal/contracts"
	"github.com/linskybing/nexuspaas/backend/internal/platform"
	"github.com/linskybing/nexuspaas/backend/internal/platform/cluster"
	"github.com/linskybing/nexuspaas/backend/internal/services/orgproject"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

func TestWorkloadConfigFileWorkflow(t *testing.T) {
	app := newWorkloadTestApp()

	code, data, _ := createConfigFile(app, workloadRequest(http.MethodPost, "/api/v1/configfiles?project_id=P1", `{"id":"cfg1","name":"train.yaml","path":"jobs/train.yaml","content":"kind: Job"}`), platform.RouteSpec{})
	assertWorkloadStatus(t, code, data, http.StatusCreated)
	if data.(contracts.Record[map[string]any]).Data["project_id"] != "P1" {
		t.Fatalf("created config = %#v, want project P1", data)
	}

	getReq := workloadRequest(http.MethodGet, "/api/v1/configfiles/cfg1", "")
	getReq.SetPathValue("id", "cfg1")
	code, data, _ = getConfigFile(app, getReq, platform.RouteSpec{})
	assertWorkloadStatus(t, code, data, http.StatusOK)

	updateReq := workloadRequest(http.MethodPut, "/api/v1/configfiles/cfg1", `{"content":"kind: Pod","message":"second"}`)
	updateReq.SetPathValue("id", "cfg1")
	code, data, _ = updateConfigFile(app, updateReq, platform.RouteSpec{})
	assertWorkloadStatus(t, code, data, http.StatusOK)
	if versions := app.Store.List(context.Background(), versionsResource); len(versions) != 2 {
		t.Fatalf("versions = %d, want create and update versions", len(versions))
	}

	projectReq := workloadRequest(http.MethodGet, "/api/v1/configfiles/project/P1", "")
	projectReq.SetPathValue("project_id", "P1")
	code, data, _ = listConfigFilesByProject(app, projectReq, platform.RouteSpec{})
	assertWorkloadStatus(t, code, data, http.StatusOK)
	if records := data.([]contracts.Record[map[string]any]); len(records) != 1 || records[0].ID != "cfg1" {
		t.Fatalf("project configs = %#v, want cfg1", data)
	}

	treeReq := workloadRequest(http.MethodGet, "/api/v1/configfiles/project/P1/tree", "")
	treeReq.SetPathValue("project_id", "P1")
	code, data, _ = configFileTree(app, treeReq, platform.RouteSpec{})
	assertWorkloadStatus(t, code, data, http.StatusOK)
	if nodes := data.(map[string]any)["nodes"].([]map[string]any); len(nodes) != 1 || nodes[0]["path"] != "jobs/train.yaml" {
		t.Fatalf("tree = %#v, want config path", data)
	}

	historyReq := workloadRequest(http.MethodGet, "/api/v1/configfiles/project/P1/history", "")
	historyReq.SetPathValue("project_id", "P1")
	code, data, _ = projectConfigHistory(app, historyReq, platform.RouteSpec{})
	assertWorkloadStatus(t, code, data, http.StatusOK)
	if history := data.([]contracts.Record[map[string]any]); len(history) != 2 {
		t.Fatalf("history = %#v, want two versions", history)
	}
}

func TestWorkloadConfigInstanceCommands(t *testing.T) {
	app := newWorkloadTestApp()
	createWorkloadRecord(t, app, configsResource, map[string]any{"id": "cfg1", "project_id": "P1", "name": "train.yaml"})
	createWorkloadRecord(t, app, instancesResource, map[string]any{"id": "pod1", "config_id": "cfg1", "pod": "train-pod"})

	startReq := workloadRequest(http.MethodPost, "/api/v1/configfiles/cfg1/instance", `{"namespace":"proj-p1"}`)
	startReq.SetPathValue("id", "cfg1")
	code, data, _ := startConfigInstance(app, startReq, platform.RouteSpec{})
	assertWorkloadStatus(t, code, data, http.StatusAccepted)
	if data.(contracts.Record[map[string]any]).Data["action"] != "start" {
		t.Fatalf("start command = %#v, want action start", data)
	}

	stopReq := workloadRequest(http.MethodDelete, "/api/v1/configfiles/cfg1/instance", `{}`)
	stopReq.SetPathValue("id", "cfg1")
	code, data, _ = stopConfigInstance(app, stopReq, platform.RouteSpec{})
	assertWorkloadStatus(t, code, data, http.StatusAccepted)

	podsReq := workloadRequest(http.MethodGet, "/api/v1/configfiles/cfg1/instance/pods", "")
	podsReq.SetPathValue("id", "cfg1")
	code, data, _ = listConfigInstancePods(app, podsReq, platform.RouteSpec{})
	assertWorkloadStatus(t, code, data, http.StatusOK)
	if pods := data.([]contracts.Record[map[string]any]); len(pods) != 1 || pods[0].ID != "pod1" {
		t.Fatalf("pods = %#v, want pod1", data)
	}
}

func TestJobLogsReturnStoredRowsWithoutCluster(t *testing.T) {
	app := newAuthWorkloadTestApp()
	seedWorkloadProject(t, app, "P1")
	seedWorkloadProjectMember(t, app, "P1", "U1")
	createWorkloadRecord(t, app, jobsResource, map[string]any{"id": "j1", "job_id": "j1", "project_id": "P1", "namespace": "proj-p1"})
	createWorkloadRecord(t, app, jobLogsResource, map[string]any{"job_id": "j1", "message": "stored log"})

	records := serveJobLogs(t, app, "j1", "U1", http.StatusOK)
	if len(records) != 1 || records[0].Data["message"] != "stored log" {
		t.Fatalf("logs = %#v, want stored log only", records)
	}
}

func TestJobLogsAppendKubernetesLogsFromCreatedResources(t *testing.T) {
	app := newAuthWorkloadTestAppWithCluster(workloadLogPod("proj-p1", "created", "j1"))
	seedWorkloadProject(t, app, "P1")
	seedWorkloadProjectMember(t, app, "P1", "U1")
	createWorkloadRecord(t, app, jobsResource, map[string]any{
		"id": "j1", "job_id": "j1", "project_id": "P1", "namespace": "proj-p1",
		"created_resources": []map[string]any{{"kind": "Pod", "namespace": "proj-p1", "name": "created"}},
	})

	records := serveJobLogs(t, app, "j1", "U1", http.StatusOK)
	k8sLog := findLogRecordBySource(records, "kubernetes_pod_logs")
	if k8sLog == nil || k8sLog.Data["pod"] != "created" || k8sLog.Data["line"] != "fake logs" {
		t.Fatalf("logs = %#v, want kubernetes log from created pod", records)
	}
}

func TestJobLogsFallBackToJobLabelSelector(t *testing.T) {
	app := newAuthWorkloadTestAppWithCluster(
		workloadLogPod("proj-p1", "matching", "j1"),
		workloadLogPod("proj-p1", "other", "j2"),
	)
	seedWorkloadProject(t, app, "P1")
	seedWorkloadProjectMember(t, app, "P1", "U1")
	createWorkloadRecord(t, app, jobsResource, map[string]any{"id": "j1", "job_id": "j1", "project_id": "P1", "namespace": "proj-p1"})

	records := serveJobLogs(t, app, "j1", "U1", http.StatusOK)
	if len(records) != 1 || records[0].Data["pod"] != "matching" || records[0].Data["line"] != "fake logs" {
		t.Fatalf("logs = %#v, want label-selected kubernetes log", records)
	}
}

func TestJobLogsRejectUnauthorizedBeforeClusterRead(t *testing.T) {
	pod := workloadLogPod("proj-p2", "secret-job", "j2")
	clientset := k8sfake.NewSimpleClientset(pod)
	app := newAuthWorkloadTestAppWithClientset(clientset)
	seedWorkloadProject(t, app, "P1")
	seedWorkloadProject(t, app, "P2")
	seedWorkloadProjectMember(t, app, "P1", "U1")
	createWorkloadRecord(t, app, jobsResource, map[string]any{"id": "j2", "job_id": "j2", "project_id": "P2", "namespace": "proj-p2"})

	_ = serveJobLogs(t, app, "j2", "U1", http.StatusForbidden)
	if len(clientset.Actions()) != 0 {
		t.Fatalf("cluster actions = %#v, want none before authorization succeeds", clientset.Actions())
	}
}

func TestJobLogsIgnoreNamespaceOutsideProjectBoundary(t *testing.T) {
	pod := workloadLogPod("proj-p2", "spoofed", "j1")
	clientset := k8sfake.NewSimpleClientset(pod)
	app := newAuthWorkloadTestAppWithClientset(clientset)
	seedWorkloadProject(t, app, "P1")
	seedWorkloadProjectMember(t, app, "P1", "U1")
	createWorkloadRecord(t, app, jobsResource, map[string]any{"id": "j1", "job_id": "j1", "project_id": "P1", "namespace": "proj-p2"})

	records := serveJobLogs(t, app, "j1", "U1", http.StatusOK)
	if len(records) != 0 {
		t.Fatalf("logs = %#v, want no logs from namespace outside project boundary", records)
	}
	if len(clientset.Actions()) != 0 {
		t.Fatalf("cluster actions = %#v, want none for namespace outside project boundary", clientset.Actions())
	}
}

func TestJobNamespaceIgnoresPayloadNamespace(t *testing.T) {
	got := jobNamespace("proj", "P1", map[string]any{"namespace": "proj-p2"})
	if got != "proj-p1" {
		t.Fatalf("jobNamespace = %q, want server-derived project namespace", got)
	}
}

func TestWorkloadMalformedJSONDoesNotWrite(t *testing.T) {
	app := newWorkloadTestApp()
	code, data, _ := createConfigFile(app, workloadRequest(http.MethodPost, "/api/v1/configfiles", `{`), platform.RouteSpec{})
	assertWorkloadStatus(t, code, data, http.StatusBadRequest)
	if got := len(app.Store.List(context.Background(), configsResource)); got != 0 {
		t.Fatalf("config count = %d, want 0", got)
	}
}

func TestWorkloadConfigFileRejectsOversizedManifest(t *testing.T) {
	app := newWorkloadTestApp()
	app.Config.MaxConfigFileBytes = 8

	code, data, _ := createConfigFile(app, workloadRequest(http.MethodPost, "/api/v1/configfiles?project_id=P1", `{"id":"cfg-big","name":"big.yaml","content":"kind: Deployment"}`), platform.RouteSpec{})

	assertWorkloadStatus(t, code, data, http.StatusRequestEntityTooLarge)
	if got := len(app.Store.List(context.Background(), configsResource)); got != 0 {
		t.Fatalf("config count = %d, want 0", got)
	}
}

func TestWorkloadConfigVersionRejectsTooManyManifestDocuments(t *testing.T) {
	app := newWorkloadTestApp()
	app.Config.MaxConfigFileDocuments = 1
	createWorkloadRecord(t, app, configsResource, map[string]any{"id": "cfg1", "project_id": "P1", "name": "one.yaml"})

	req := workloadRequest(http.MethodPost, "/api/v1/configfiles/cfg1/versions", `{"content":"kind: Pod\n---\nkind: Service"}`)
	req.SetPathValue("id", "cfg1")
	code, data, _ := commitConfigFileVersion(app, req, platform.RouteSpec{})

	assertWorkloadStatus(t, code, data, http.StatusUnprocessableEntity)
	if got := len(app.Store.List(context.Background(), versionsResource)); got != 0 {
		t.Fatalf("version count = %d, want 0", got)
	}
}

func TestWorkloadConfigProjectAccessAllowsMemberAndAdmin(t *testing.T) {
	app := newAuthWorkloadTestApp()
	seedWorkloadProject(t, app, "P1")
	seedWorkloadProject(t, app, "P2")
	seedWorkloadProjectMember(t, app, "P1", "U1")

	memberReq := workloadAuthRequest(http.MethodPost, "/api/v1/configfiles?project_id=P1", `{"id":"cfg-member","name":"member.yaml"}`, "U1", "user")
	code, data, _ := createConfigFile(app, memberReq, platform.RouteSpec{})
	assertWorkloadStatus(t, code, data, http.StatusCreated)

	deniedReq := workloadAuthRequest(http.MethodPost, "/api/v1/configfiles?project_id=P2", `{"id":"cfg-denied","name":"denied.yaml"}`, "U1", "user")
	code, data, _ = createConfigFile(app, deniedReq, platform.RouteSpec{})
	assertWorkloadStatus(t, code, data, http.StatusForbidden)

	adminReq := workloadAuthRequest(http.MethodPost, "/api/v1/configfiles?project_id=P2", `{"id":"cfg-admin","name":"admin.yaml"}`, "ADMIN", "admin")
	code, data, _ = createConfigFile(app, adminReq, platform.RouteSpec{})
	assertWorkloadStatus(t, code, data, http.StatusCreated)
}

func TestWorkloadConfigReadsFilterByProjectMembership(t *testing.T) {
	app := newAuthWorkloadTestApp()
	seedWorkloadProject(t, app, "P1")
	seedWorkloadProject(t, app, "P2")
	seedWorkloadProjectMember(t, app, "P1", "U1")
	createWorkloadRecord(t, app, configsResource, map[string]any{"id": "cfg1", "project_id": "P1", "name": "one.yaml"})
	createWorkloadRecord(t, app, configsResource, map[string]any{"id": "cfg2", "project_id": "P2", "name": "two.yaml"})

	code, data, _ := listConfigFiles(app, workloadAuthRequest(http.MethodGet, "/api/v1/configfiles", "", "U1", "user"), platform.RouteSpec{})
	assertWorkloadStatus(t, code, data, http.StatusOK)
	records := data.([]contracts.Record[map[string]any])
	if len(records) != 1 || records[0].ID != "cfg1" {
		t.Fatalf("member config list = %#v, want only cfg1", records)
	}

	getDenied := workloadAuthRequest(http.MethodGet, "/api/v1/configfiles/cfg2", "", "U1", "user")
	getDenied.SetPathValue("id", "cfg2")
	code, data, _ = getConfigFile(app, getDenied, platform.RouteSpec{})
	assertWorkloadStatus(t, code, data, http.StatusForbidden)

	code, data, _ = listConfigFiles(app, workloadAuthRequest(http.MethodGet, "/api/v1/configfiles", "", "ADMIN", "admin"), platform.RouteSpec{})
	assertWorkloadStatus(t, code, data, http.StatusOK)
	if records := data.([]contracts.Record[map[string]any]); len(records) != 2 {
		t.Fatalf("admin config list = %#v, want both configs", records)
	}
}

func TestWorkloadConfigUpdateRejectsProjectMove(t *testing.T) {
	app := newAuthWorkloadTestApp()
	seedWorkloadProject(t, app, "P1")
	seedWorkloadProject(t, app, "P2")
	seedWorkloadProjectMember(t, app, "P1", "U1")
	createWorkloadRecord(t, app, configsResource, map[string]any{"id": "cfg1", "project_id": "P1", "name": "one.yaml"})

	req := workloadAuthRequest(http.MethodPatch, "/api/v1/configfiles/cfg1", `{"project_id":"P2","content":"moved"}`, "U1", "user")
	req.SetPathValue("id", "cfg1")
	code, data, _ := updateConfigFile(app, req, platform.RouteSpec{})
	assertWorkloadStatus(t, code, data, http.StatusBadRequest)

	record, _ := app.Store.Get(context.Background(), configsResource, "cfg1")
	if record.Data["project_id"] != "P1" || record.Data["content"] == "moved" {
		t.Fatalf("config after rejected move = %#v, want unchanged project/content", record.Data)
	}
}

func TestWorkloadConfigVersionTreeAndDeleteProjectAccess(t *testing.T) {
	app := newAuthWorkloadTestApp()
	seedWorkloadProject(t, app, "P1")
	seedWorkloadProject(t, app, "P2")
	seedWorkloadProjectMember(t, app, "P1", "U1")
	createWorkloadRecord(t, app, configsResource, map[string]any{"id": "cfg1", "project_id": "P1", "name": "one.yaml", "path": "jobs/one.yaml"})
	createWorkloadRecord(t, app, configsResource, map[string]any{"id": "cfg2", "project_id": "P2", "name": "two.yaml", "path": "jobs/two.yaml"})

	commitReq := workloadAuthRequest(http.MethodPost, "/api/v1/configfiles/cfg1/versions", `{"content":"kind: Job","message":"manual"}`, "U1", "user")
	commitReq.SetPathValue("id", "cfg1")
	code, data, _ := commitConfigFileVersion(app, commitReq, platform.RouteSpec{})
	assertWorkloadStatus(t, code, data, http.StatusCreated)

	versionReq := workloadAuthRequest(http.MethodGet, "/api/v1/configfiles/cfg1/versions", "", "U1", "user")
	versionReq.SetPathValue("id", "cfg1")
	code, data, _ = listConfigFileVersions(app, versionReq, platform.RouteSpec{})
	assertWorkloadStatus(t, code, data, http.StatusOK)
	if versions := data.([]contracts.Record[map[string]any]); len(versions) != 1 {
		t.Fatalf("versions = %#v, want committed version", versions)
	}

	treeReq := workloadAuthRequest(http.MethodGet, "/api/v1/configfiles/tree", "", "U1", "user")
	code, data, _ = listConfigFileTree(app, treeReq, platform.RouteSpec{})
	assertWorkloadStatus(t, code, data, http.StatusOK)
	nodes := data.(map[string]any)["nodes"].([]map[string]any)
	if len(nodes) != 1 || nodes[0]["id"] != "cfg1" {
		t.Fatalf("global tree nodes = %#v, want only cfg1", nodes)
	}

	projectAliasReq := workloadAuthRequest(http.MethodGet, "/api/v1/projects/P1/config-files", "", "U1", "user")
	projectAliasReq.SetPathValue("id", "P1")
	code, data, _ = listProjectConfigFiles(app, projectAliasReq, platform.RouteSpec{})
	assertWorkloadStatus(t, code, data, http.StatusOK)
	if records := data.([]contracts.Record[map[string]any]); len(records) != 1 || records[0].ID != "cfg1" {
		t.Fatalf("project alias configs = %#v, want cfg1", records)
	}

	commitDenied := workloadAuthRequest(http.MethodPost, "/api/v1/configfiles/cfg2/versions", `{"content":"denied"}`, "U1", "user")
	commitDenied.SetPathValue("id", "cfg2")
	code, data, _ = commitConfigFileVersion(app, commitDenied, platform.RouteSpec{})
	assertWorkloadStatus(t, code, data, http.StatusForbidden)

	deleteDenied := workloadAuthRequest(http.MethodDelete, "/api/v1/configfiles/cfg2", "", "U1", "user")
	deleteDenied.SetPathValue("id", "cfg2")
	code, data, _ = deleteConfigFile(app, deleteDenied, platform.RouteSpec{})
	assertWorkloadStatus(t, code, data, http.StatusForbidden)

	deleteAllowed := workloadAuthRequest(http.MethodDelete, "/api/v1/configfiles/cfg1", "", "U1", "user")
	deleteAllowed.SetPathValue("id", "cfg1")
	code, data, _ = deleteConfigFile(app, deleteAllowed, platform.RouteSpec{})
	assertWorkloadStatus(t, code, data, http.StatusOK)
	if _, found := app.Store.Get(context.Background(), configsResource, "cfg1"); found {
		t.Fatal("cfg1 still exists after authorized delete")
	}
}

func TestWorkloadConfigInstanceRoutesRequireProjectAccess(t *testing.T) {
	app := newAuthWorkloadTestApp()
	seedWorkloadProject(t, app, "P1")
	seedWorkloadProjectMember(t, app, "P1", "U1")
	createWorkloadRecord(t, app, configsResource, map[string]any{"id": "cfg1", "project_id": "P1", "name": "one.yaml"})
	createWorkloadRecord(t, app, instancesResource, map[string]any{"id": "pod1", "config_id": "cfg1", "pod": "pod1"})

	startDenied := workloadAuthRequest(http.MethodPost, "/api/v1/configfiles/cfg1/instance", `{}`, "U2", "user")
	startDenied.SetPathValue("id", "cfg1")
	code, data, _ := startConfigInstance(app, startDenied, platform.RouteSpec{})
	assertWorkloadStatus(t, code, data, http.StatusForbidden)

	podsAllowed := workloadAuthRequest(http.MethodGet, "/api/v1/configfiles/cfg1/instance/pods", "", "U1", "user")
	podsAllowed.SetPathValue("id", "cfg1")
	code, data, _ = listConfigInstancePods(app, podsAllowed, platform.RouteSpec{})
	assertWorkloadStatus(t, code, data, http.StatusOK)
	if pods := data.([]contracts.Record[map[string]any]); len(pods) != 1 || pods[0].ID != "pod1" {
		t.Fatalf("instance pods = %#v, want pod1", pods)
	}
}

func TestWorkloadProjectAccessUsesOrgProjectOwnerReadWhenIsolated(t *testing.T) {
	serviceKey := "service-secret"
	owner := platform.NewApp(platform.Config{ServiceName: "org-project-service", HTTPAddr: ":0", ServiceAPIKey: serviceKey})
	orgproject.Register(owner)
	createWorkloadRecord(t, owner, orgProjectsResource, map[string]any{"id": "P1", "project_name": "owner-read"})
	createWorkloadRecord(t, owner, orgProjectMembersResource, map[string]any{"id": "P1:U1", "project_id": "P1", "user_id": "U1", "role": "user"})
	server := httptest.NewServer(owner)
	defer server.Close()

	app := platform.NewApp(platform.Config{
		ServiceName:   serviceName,
		HTTPAddr:      ":0",
		RequireAuth:   true,
		ServiceURLs:   map[string]string{"org-project-service": server.URL},
		ServiceAPIKey: serviceKey,
	})
	Register(app)
	req := workloadAuthRequest(http.MethodPost, "/api/v1/configfiles?project_id=P1", `{"id":"cfg1","name":"remote.yaml"}`, "U1", "user")
	code, data, _ := createConfigFile(app, req, platform.RouteSpec{})
	assertWorkloadStatus(t, code, data, http.StatusCreated)
}

func newWorkloadTestApp() *platform.App {
	app := platform.NewApp(platform.Config{ServiceName: "all", HTTPAddr: ":0"})
	Register(app)
	return app
}

func newAuthWorkloadTestApp() *platform.App {
	app := platform.NewApp(platform.Config{ServiceName: "all", HTTPAddr: ":0", RequireAuth: true})
	Register(app)
	return app
}

func newAuthWorkloadTestAppWithCluster(objects ...runtime.Object) *platform.App {
	return newAuthWorkloadTestAppWithClientset(k8sfake.NewSimpleClientset(objects...))
}

func newAuthWorkloadTestAppWithClientset(clientset *k8sfake.Clientset) *platform.App {
	app := platform.NewApp(platform.Config{ServiceName: "all", HTTPAddr: ":0", RequireAuth: true}, platform.WithCluster(cluster.New(clientset, "proj")))
	Register(app)
	return app
}

func workloadRequest(method, target, body string) *http.Request {
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Idempotency-Key", "test-"+method+"-"+target)
	return req
}

func workloadAuthRequest(method, target, body, userID, role string) *http.Request {
	req := workloadRequest(method, target, body)
	req.Header.Set("X-User-ID", userID)
	req.Header.Set("X-Username", userID)
	req.Header.Set("X-User-Role", role)
	return req
}

func seedWorkloadProject(t *testing.T, app *platform.App, projectID string) {
	t.Helper()
	createWorkloadRecord(t, app, orgProjectsResource, map[string]any{"id": projectID, "project_name": projectID})
}

func seedWorkloadProjectMember(t *testing.T, app *platform.App, projectID, userID string) {
	t.Helper()
	createWorkloadRecord(t, app, orgProjectMembersResource, map[string]any{"id": projectID + ":" + userID, "project_id": projectID, "user_id": userID, "role": "user"})
}

func createWorkloadRecord(t *testing.T, app *platform.App, resource string, data map[string]any) {
	t.Helper()
	if _, err := app.Store.Create(context.Background(), resource, data); err != nil {
		t.Fatal(err)
	}
}

func assertWorkloadStatus(t *testing.T, code int, data any, want int) {
	t.Helper()
	if code != want {
		t.Fatalf("status=%d data=%#v, want %d", code, data, want)
	}
}

func serveJobLogs(t *testing.T, app *platform.App, jobID, userID string, want int) []contracts.Record[map[string]any] {
	t.Helper()
	req := workloadAuthRequest(http.MethodGet, "/api/v1/jobs/"+jobID+"/logs", "", userID, "user")
	req.SetPathValue("id", jobID)
	code, data, _ := listJobLogs(app, req, platform.RouteSpec{})
	assertWorkloadStatus(t, code, data, want)
	if want != http.StatusOK {
		return nil
	}
	records, ok := data.([]contracts.Record[map[string]any])
	if !ok {
		t.Fatalf("logs data = %T, want records", data)
	}
	return records
}

func workloadLogPod(namespace, name, jobID string) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: name, Labels: map[string]string{cluster.LabelJobID: jobID}},
		Spec: corev1.PodSpec{Containers: []corev1.Container{{
			Name: "main",
		}}},
	}
}

func findLogRecordBySource(records []contracts.Record[map[string]any], source string) *contracts.Record[map[string]any] {
	for i := range records {
		if records[i].Data["source"] == source {
			return &records[i]
		}
	}
	return nil
}
