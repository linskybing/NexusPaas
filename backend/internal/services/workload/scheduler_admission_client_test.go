package workload

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/platform"
	"github.com/linskybing/nexuspaas/backend/internal/services/schedulerquota"
)

func TestSubmitWorkflowDoesNotUseHttptest(t *testing.T) {
	raw, err := os.ReadFile("job_submit.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	for _, forbidden := range []string{"net/http/httptest", "httptest."} {
		if strings.Contains(source, forbidden) {
			t.Fatalf("job_submit.go contains %q", forbidden)
		}
	}
}

func TestSchedulerAdmissionClientLocalCopiesSafeHeadersAndReplacesServiceKeys(t *testing.T) {
	app := platform.NewApp(platform.Config{
		ServiceName:   "all",
		HTTPAddr:      ":0",
		ServiceAPIKey: "service-secret",
	})
	registerSchedulerAdmissionRoute(app)
	var received http.Header
	app.RegisterCustomHandler(http.MethodPost, schedulerAdmissionPath, func(_ *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
		received = r.Header.Clone()
		return http.StatusOK, map[string]any{"allowed": true}, nil
	})

	client, err := newSchedulerAdmissionClient(app)
	if err != nil {
		t.Fatal(err)
	}
	headers := http.Header{}
	headers.Set("Authorization", "Bearer caller-token")
	headers.Set("Cookie", "token=caller-cookie")
	headers.Set("Idempotency-Key", "submit-idempotency")
	headers.Set("X-API-Key", "caller-api-key")
	headers.Set("X-Request-ID", "req-submit")
	headers.Set("X-Service-Key", "caller-service-key")
	headers.Set("X-Trace-ID", "trace-submit")
	result, err := client.Review(context.Background(), headers, map[string]any{"project_id": "P1", "user_id": "U1"})
	if err != nil {
		t.Fatal(err)
	}

	if result.StatusCode != http.StatusOK || result.Data["allowed"] != true {
		t.Fatalf("local admission result = %#v, want allowed HTTP 200", result)
	}
	if received.Get("X-Service-Key") != "service-secret" || received.Get("X-API-Key") != "service-secret" {
		t.Fatalf("local scheduler service auth headers = X-Service-Key:%q X-API-Key:%q, want configured service key", received.Get("X-Service-Key"), received.Get("X-API-Key"))
	}
	if received.Get("X-Request-ID") != "req-submit" ||
		received.Get("X-Trace-ID") != "trace-submit" ||
		received.Get("Idempotency-Key") != "submit-idempotency" {
		t.Fatalf("local scheduler correlation headers = %#v, want safe caller context", received)
	}
	for _, key := range []string{"Authorization", "Cookie"} {
		if got := received.Get(key); got != "" {
			t.Fatalf("local scheduler received caller credential %s=%q", key, got)
		}
	}
}

func TestSchedulerAdmissionClientMissingURLDoesNotCreatePartialJob(t *testing.T) {
	app := newIsolatedWorkloadSubmitTestApp(platform.Config{
		ServiceAPIKey: "service-secret",
	})

	rec := serveSubmitJob(t, app, `{"project_id":"P1","user_id":"U1","queue_name":"default-batch","required_cpu":1,"required_memory":1024}`, "U1", http.StatusServiceUnavailable)
	data := responseEnvelopeData(t, rec)

	if data["message"] != "scheduler admission unavailable" {
		t.Fatalf("submit response = %#v, want scheduler unavailable message", data)
	}
	assertNoSubmittedJobs(t, app)
}

func TestSchedulerAdmissionClientUnavailableDoesNotCreatePartialJob(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("closed scheduler server should not handle requests")
	}))
	schedulerURL := server.URL
	server.Close()
	app := newIsolatedWorkloadSubmitTestApp(platform.Config{
		ServiceURLs:    map[string]string{schedulerServiceName: schedulerURL},
		ServiceAPIKey:  "service-secret",
		AdapterTimeout: 100 * time.Millisecond,
	})

	rec := serveSubmitJob(t, app, `{"project_id":"P1","user_id":"U1","queue_name":"default-batch","required_cpu":1,"required_memory":1024}`, "U1", http.StatusServiceUnavailable)
	data := responseEnvelopeData(t, rec)

	if data["message"] != "scheduler admission unavailable" {
		t.Fatalf("submit response = %#v, want scheduler unavailable message", data)
	}
	assertNoSubmittedJobs(t, app)
}

func TestSchedulerAdmissionClientBadServiceKeyDoesNotCreatePartialJob(t *testing.T) {
	schedulerKey := "scheduler-service-secret"
	schedulerApp := platform.NewApp(platform.Config{
		ServiceName:      schedulerServiceName,
		HTTPAddr:         ":0",
		RequireAuth:      true,
		APIKeys:          map[string]bool{schedulerKey: true},
		APIKeyPrincipals: map[string]platform.APIKeyPrincipal{schedulerKey: {ID: serviceName, Role: "service"}},
	})
	registerSchedulerAdmissionRoute(schedulerApp)
	schedulerquota.Register(schedulerApp)
	seedJobAdmissionProject(t, schedulerApp, map[string]any{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		schedulerApp.ServeHTTP(w, r)
	}))
	defer server.Close()
	app := newIsolatedWorkloadSubmitTestApp(platform.Config{
		ServiceURLs:    map[string]string{schedulerServiceName: server.URL},
		ServiceAPIKey:  "wrong-service-secret",
		AdapterTimeout: time.Second,
	})

	rec := serveSubmitJob(t, app, `{"project_id":"P1","user_id":"U1","queue_name":"default-batch","required_cpu":1,"required_memory":1024}`, "U1", http.StatusUnauthorized)
	data := responseEnvelopeData(t, rec)

	if data["allowed"] != false || data["reason"] != "authentication is required" {
		t.Fatalf("submit response = %#v, want scheduler auth denial", data)
	}
	assertNoSubmittedJobs(t, app)
	if got := len(schedulerApp.Store.List(context.Background(), testSchedulerAdmissionsResource)); got != 0 {
		t.Fatalf("remote scheduler admissions = %d, want 0 after bad service key", got)
	}
}

func newIsolatedWorkloadSubmitTestApp(cfg platform.Config) *platform.App {
	cfg.ServiceName = serviceName
	if cfg.HTTPAddr == "" {
		cfg.HTTPAddr = ":0"
	}
	app := platform.NewApp(cfg)
	registerWorkloadJobRoute(app)
	Register(app)
	return app
}

func assertNoSubmittedJobs(t *testing.T, app *platform.App) {
	t.Helper()
	if got := len(app.Store.List(context.Background(), jobsResource)); got != 0 {
		t.Fatalf("job count = %d, want 0", got)
	}
}
