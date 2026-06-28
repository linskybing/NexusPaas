package k8scontrol

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/linskybing/nexuspaas/backend/internal/platform"
	"github.com/linskybing/nexuspaas/backend/internal/platform/cluster"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestFastTransferMoverRouteRequiresServiceAuth(t *testing.T) {
	app := newFastTransferMoverApp(t)

	for _, tc := range []struct {
		name   string
		caller string
		key    string
		want   int
	}{
		{name: "missing", want: http.StatusUnauthorized},
		{name: "wrong key", caller: "storage-service", key: "wrong", want: http.StatusUnauthorized},
		{name: "wrong audience", caller: "other-service", key: "other-key", want: http.StatusUnauthorized},
		{name: "allowed", caller: "storage-service", key: "scoped-key", want: http.StatusCreated},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/internal/k8s-control/fast-transfers/mover-jobs", strings.NewReader(fastTransferMoverBody()))
			req.Header.Set("Content-Type", "application/json")
			if tc.caller != "" {
				req.Header.Set("X-Service-Name", tc.caller)
			}
			if tc.key != "" {
				req.Header.Set("X-Service-Key", tc.key)
			}
			rec := httptest.NewRecorder()
			app.ServeHTTP(rec, req)
			if rec.Code != tc.want {
				t.Fatalf("status = %d, want %d: %s", rec.Code, tc.want, rec.Body.String())
			}
		})
	}
}

func TestFastTransferMoverHandlerCreatesAndDedupesJob(t *testing.T) {
	app := newFastTransferMoverApp(t)
	req := fastTransferMoverServiceRequest(fastTransferMoverBody())

	code, data, _ := createFastTransferMoverJob(app, req, platform.RouteSpec{})
	if code != http.StatusCreated {
		t.Fatalf("status=%d data=%#v, want created", code, data)
	}
	code, data, _ = createFastTransferMoverJob(app, fastTransferMoverServiceRequest(fastTransferMoverBody()), platform.RouteSpec{})
	if code != http.StatusOK || data.(cluster.FastTransferMoverJobResult).Action != cluster.FastTransferMoverActionAlreadyExists {
		t.Fatalf("status=%d data=%#v, want already_exists", code, data)
	}
	if _, err := app.Cluster.Clientset().BatchV1().Jobs("project-p1").Get(context.Background(), "fast-transfer-copy1", metav1.GetOptions{}); err != nil {
		t.Fatalf("get mover job: %v", err)
	}
}

func TestFastTransferMoverHandlerValidatesPayloadAndCluster(t *testing.T) {
	app := newFastTransferMoverApp(t)
	code, data, _ := createFastTransferMoverJob(app, fastTransferMoverServiceRequest(`{"project_id":"P1","name":"copy1","tool":"sh"}`), platform.RouteSpec{})
	if code != http.StatusUnprocessableEntity || data.(cluster.FastTransferMoverJobResult).Action != cluster.FastTransferMoverActionInvalid {
		t.Fatalf("status=%d data=%#v, want invalid", code, data)
	}

	app.Cluster = nil
	code, data, _ = createFastTransferMoverJob(app, fastTransferMoverServiceRequest(fastTransferMoverBody()), platform.RouteSpec{})
	if code != http.StatusBadGateway || data.(cluster.FastTransferMoverJobResult).Action != cluster.FastTransferMoverActionDegraded {
		t.Fatalf("status=%d data=%#v, want degraded", code, data)
	}
}

func newFastTransferMoverApp(t *testing.T) *platform.App {
	t.Helper()
	app := platform.NewApp(platform.Config{
		ServiceName: "all",
		HTTPAddr:    ":0",
		ServiceTrustedIdentities: map[string]platform.ServiceTrustedIdentity{
			"storage-service": {Key: "scoped-key", Audiences: []string{serviceName}},
			"other-service":   {Key: "other-key", Audiences: []string{"other-service"}},
		},
		FastTransferMoverImage: "rsync:test",
	}, platform.WithCluster(cluster.New(fake.NewSimpleClientset(), "proj")))
	app.RegisterService(Spec())
	Register(app)
	return app
}

func fastTransferMoverServiceRequest(body string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/internal/k8s-control/fast-transfers/mover-jobs", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Service-Name", "storage-service")
	req.Header.Set("X-Service-Key", "scoped-key")
	return req
}

func fastTransferMoverBody() string {
	return `{
		"project_id":"P1",
		"transfer_id":"P1:project-p1:copy1",
		"target_namespace":"project-p1",
		"name":"copy1",
		"source":{"namespace":"project-p1","pvc":"dataset-pvc","path":"/data/source"},
		"target":{"namespace":"project-p1","pvc":"scratch-pvc","path":"/data/target"},
		"tool":"rsync",
		"progress_callback":{"path":"/internal/storage/projects/P1/transfers/project-p1/copy1/progress"}
	}`
}
