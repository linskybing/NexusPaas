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

func TestFastTransferMoverProgressCallbackUsesScopedIdentity(t *testing.T) {
	app := newFastTransferMoverCallbackApp(t, platform.Config{
		ServiceURLs:         map[string]string{"storage-service": "http://storage.internal"},
		ServiceIdentityName: "k8s-control-service",
		ServiceIdentityKey:  "callback-key",
	})

	result := ensureFastTransferMoverJob(fastTransferMoverServiceRequest(fastTransferMoverBody()), app, fastTransferMoverRequest("/internal/storage/projects/P1/transfers/project-p1/copy1/progress"))
	if result.Action != cluster.FastTransferMoverActionCreated {
		t.Fatalf("result = %#v, want created", result)
	}
	env := fastTransferMoverJobEnv(t, app, result.Namespace, result.Name)
	if env["NEXUSPAAS_FAST_TRANSFER_PROGRESS_URL"] != "http://storage.internal/internal/storage/projects/P1/transfers/project-p1/copy1/progress" ||
		env["NEXUSPAAS_FAST_TRANSFER_PROGRESS_SERVICE_NAME"] != "k8s-control-service" ||
		env["NEXUSPAAS_FAST_TRANSFER_PROGRESS_KEY"] != "callback-key" {
		t.Fatalf("env = %#v, want scoped callback env", env)
	}
}

func TestFastTransferMoverProgressCallbackUsesLegacyKeyFallback(t *testing.T) {
	app := newFastTransferMoverCallbackApp(t, platform.Config{
		ServiceURLs:   map[string]string{"storage-service": "http://storage.internal/"},
		ServiceAPIKey: "legacy-key",
	})

	result := ensureFastTransferMoverJob(fastTransferMoverServiceRequest(fastTransferMoverBody()), app, fastTransferMoverRequest(""))
	if result.Action != cluster.FastTransferMoverActionCreated {
		t.Fatalf("result = %#v, want created", result)
	}
	env := fastTransferMoverJobEnv(t, app, result.Namespace, result.Name)
	if env["NEXUSPAAS_FAST_TRANSFER_PROGRESS_URL"] != "http://storage.internal/internal/storage/projects/P1/transfers/project-p1/copy1/progress" ||
		env["NEXUSPAAS_FAST_TRANSFER_PROGRESS_KEY"] != "legacy-key" ||
		env["NEXUSPAAS_FAST_TRANSFER_PROGRESS_SERVICE_NAME"] != "" {
		t.Fatalf("env = %#v, want legacy callback env", env)
	}
}

func TestFastTransferMoverProgressCallbackRejectsUntrustedPath(t *testing.T) {
	for _, path := range []string{
		"http://attacker/internal/storage/projects/P1/transfers/project-p1/copy1/progress",
		"/internal/storage/projects/P1/transfers/project-p1/other/progress",
	} {
		t.Run(path, func(t *testing.T) {
			app := newFastTransferMoverCallbackApp(t, platform.Config{
				ServiceURLs:         map[string]string{"storage-service": "http://storage.internal"},
				ServiceIdentityName: "k8s-control-service",
				ServiceIdentityKey:  "callback-key",
			})
			result := ensureFastTransferMoverJob(fastTransferMoverServiceRequest(fastTransferMoverBody()), app, fastTransferMoverRequest(path))
			if result.Action != cluster.FastTransferMoverActionCreated {
				t.Fatalf("result = %#v, want created", result)
			}
			if env := fastTransferMoverJobEnv(t, app, result.Namespace, result.Name); len(env) != 0 {
				t.Fatalf("env = %#v, want no callback env for untrusted path", env)
			}
		})
	}
}

func TestFastTransferMoverProgressCallbackRequiresBaseURLAndIdentity(t *testing.T) {
	for _, cfg := range []platform.Config{
		{ServiceIdentityName: "k8s-control-service", ServiceIdentityKey: "callback-key"},
		{ServiceURLs: map[string]string{"storage-service": "http://storage.internal"}},
	} {
		app := newFastTransferMoverCallbackApp(t, cfg)
		result := ensureFastTransferMoverJob(fastTransferMoverServiceRequest(fastTransferMoverBody()), app, fastTransferMoverRequest(""))
		if result.Action != cluster.FastTransferMoverActionCreated {
			t.Fatalf("result = %#v, want created", result)
		}
		if env := fastTransferMoverJobEnv(t, app, result.Namespace, result.Name); len(env) != 0 {
			t.Fatalf("env = %#v, want no callback env without base URL and identity", env)
		}
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

func newFastTransferMoverCallbackApp(t *testing.T, cfg platform.Config) *platform.App {
	t.Helper()
	cfg.ServiceName = "k8s-control-service"
	cfg.HTTPAddr = ":0"
	cfg.FastTransferMoverImage = "rsync:test"
	return platform.NewApp(cfg, platform.WithCluster(cluster.New(fake.NewSimpleClientset(), "proj")))
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

func fastTransferMoverRequest(callbackPath string) fastTransferMoverJobRequest {
	req := fastTransferMoverJobRequest{
		ProjectID:       "P1",
		TransferID:      "P1:project-p1:copy1",
		TargetNamespace: "project-p1",
		Name:            "copy1",
		Source:          fastTransferMoverEndpoint{Namespace: "project-p1", PVC: "dataset-pvc", Path: "/data/source"},
		Target:          fastTransferMoverEndpoint{Namespace: "project-p1", PVC: "scratch-pvc", Path: "/data/target"},
		Tool:            "rsync",
	}
	req.ProgressCallback.Path = callbackPath
	return req
}

func fastTransferMoverJobEnv(t *testing.T, app *platform.App, namespace, name string) map[string]string {
	t.Helper()
	job, err := app.Cluster.Clientset().BatchV1().Jobs(namespace).Get(context.Background(), name, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get mover job: %v", err)
	}
	env := map[string]string{}
	for _, entry := range job.Spec.Template.Spec.Containers[0].Env {
		env[entry.Name] = entry.Value
	}
	return env
}
