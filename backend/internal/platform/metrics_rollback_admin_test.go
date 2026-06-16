package platform

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func TestRunAdminTaskValidateConfig(t *testing.T) {
	valid := validProductionConfig()
	if err := RunAdminTask("validate-config", valid); err != nil {
		t.Fatalf("validate-config valid error = %v, want nil", err)
	}
	invalid := withRuntimeDefaults(Config{Production: true, RequireAuth: false})
	if err := RunAdminTask("validate-config", invalid); err == nil || !strings.Contains(err.Error(), "REQUIRE_AUTH") {
		t.Fatalf("validate-config invalid error = %v, want REQUIRE_AUTH", err)
	}
	setValidProductionEnv(t)
	t.Setenv(envAdapterConfig, `{"pgadmin":{"auth":{"token":"secret-token"`)
	malformed := ConfigFromEnv()
	err := RunAdminTask("validate-config", malformed)
	if err == nil || !strings.Contains(err.Error(), envAdapterConfig) {
		t.Fatalf("validate-config malformed error = %v, want %s", err, envAdapterConfig)
	}
	if strings.Contains(err.Error(), "secret-token") {
		t.Fatalf("validate-config malformed error leaked raw config value: %v", err)
	}
}

func TestRunAdminTaskUnknownTask(t *testing.T) {
	err := RunAdminTask("not-a-task", Config{})
	if err == nil || !strings.Contains(err.Error(), "unknown admin task") {
		t.Fatalf("unknown task error = %v, want unknown admin task", err)
	}
}

func TestRunAdminTaskEnsureObjectStoreBucketGuards(t *testing.T) {
	nonBlob := Config{
		ServiceName:            "identity-service",
		ObjectStoreURL:         "http://minio:9000",
		ObjectStoreAccessKey:   "access",
		ObjectStoreSecretKey:   "secret",
		ObjectStoreBucket:      "media",
		AuthorizationPolicyURL: testPolicyURL,
	}
	err := RunAdminTask("ensure-object-store-bucket", nonBlob)
	if err == nil || !strings.Contains(err.Error(), "SERVICE_NAME=media-upload-service") {
		t.Fatalf("ensure-object-store-bucket non-blob error = %v, want service guard", err)
	}

	missingURL := Config{
		ServiceName:            mediaUploadServiceName,
		ObjectStoreAccessKey:   "access",
		ObjectStoreSecretKey:   "secret",
		ObjectStoreBucket:      "media",
		AuthorizationPolicyURL: testPolicyURL,
	}
	err = RunAdminTask("ensure-object-store-bucket", missingURL)
	if err == nil || !strings.Contains(err.Error(), envObjectStoreURL) {
		t.Fatalf("ensure-object-store-bucket missing URL error = %v, want %s", err, envObjectStoreURL)
	}

	invalidURL := missingURL
	invalidURL.ObjectStoreURL = "minio:9000"
	err = RunAdminTask("ensure-object-store-bucket", invalidURL)
	if err == nil || !strings.Contains(err.Error(), envObjectStoreURL) {
		t.Fatalf("ensure-object-store-bucket invalid URL error = %v, want %s", err, envObjectStoreURL)
	}
}

func TestRunAdminTaskValidateMigrations(t *testing.T) {
	t.Run("no files", func(t *testing.T) {
		withTempWD(t, t.TempDir())
		err := RunAdminTask("validate-migrations", Config{})
		if err == nil || !strings.Contains(err.Error(), "no service migration files found") {
			t.Fatalf("validate-migrations empty error = %v, want no files error", err)
		}
	})
	t.Run("found", func(t *testing.T) {
		dir := t.TempDir()
		writeAllServiceMigrations(t, dir)
		withTempWD(t, dir)
		if err := RunAdminTask("validate-migrations", Config{}); err != nil {
			t.Fatalf("validate-migrations found error = %v, want nil", err)
		}
	})
}

func TestMetricsObserveAndServeHTTP(t *testing.T) {
	metrics := NewMetrics()
	metrics.Observe("/api/v1/a", http.MethodGet, http.StatusOK, 1500*time.Millisecond)
	metrics.Observe("/api/v1/a", http.MethodPost, http.StatusInternalServerError, 500*time.Millisecond)
	metrics.Inc("k8s-degraded")

	rec := httptest.NewRecorder()
	metrics.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	body := rec.Body.String()
	for _, want := range []string{
		"# TYPE nexuspaas_http_requests_total counter",
		`nexuspaas_http_requests_total{route="/api/v1/a",method="GET",status="200"} 1`,
		`nexuspaas_http_requests_total{route="/api/v1/a",method="POST",status="500"} 1`,
		"# TYPE nexuspaas_http_request_duration_seconds_sum counter",
		`nexuspaas_http_request_duration_seconds_sum{route="/api/v1/a",method="GET",status="200"} 1.500000`,
		"nexuspaas_k8s_degraded_total 1",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("metrics body missing %q:\n%s", want, body)
		}
	}
}

func TestMetricsCountersAndErrorRatePercent(t *testing.T) {
	metrics := NewMetrics()
	if metrics.ErrorRatePercent() != 0 {
		t.Fatalf("empty error rate = %d, want 0", metrics.ErrorRatePercent())
	}
	metrics.Inc("k8s_degraded")
	metrics.Inc("harbor_degraded")
	metrics.Inc("other")
	if metrics.Counter("k8s_degraded") != 1 {
		t.Fatalf("k8s_degraded counter = %d, want 1", metrics.Counter("k8s_degraded"))
	}
	if metrics.CounterSuffix("_degraded") != 2 {
		t.Fatalf("degraded suffix counter = %d, want 2", metrics.CounterSuffix("_degraded"))
	}
	metrics.Observe("/ok", http.MethodGet, http.StatusOK, time.Millisecond)
	metrics.Observe("/error", http.MethodGet, http.StatusInternalServerError, time.Millisecond)
	if metrics.ErrorRatePercent() != 50 {
		t.Fatalf("error rate = %d, want 50", metrics.ErrorRatePercent())
	}
}

func TestRollbackGateAllowsAndBlocks(t *testing.T) {
	gate := RollbackGate{MaxOutboxLag: 10, MaxErrorRatePercent: 5, MaxDegradedAdapters: 0}
	if !gate.Allows(RollbackMetrics{OutboxLag: 10, ErrorRatePercent: 5, DegradedAdapters: 0}) {
		t.Fatal("gate rejected boundary-safe metrics")
	}
	for _, metrics := range []RollbackMetrics{
		{OutboxLag: 11, ErrorRatePercent: 5, DegradedAdapters: 0},
		{OutboxLag: 10, ErrorRatePercent: 6, DegradedAdapters: 0},
		{OutboxLag: 10, ErrorRatePercent: 5, DegradedAdapters: 1},
	} {
		if gate.Allows(metrics) {
			t.Fatalf("gate allowed unsafe metrics: %#v", metrics)
		}
	}
}

func TestRollbackMetricsFromApp(t *testing.T) {
	app := NewApp(Config{})
	app.Events.Checkpoint("rollback-gate")
	if err := app.Events.Publish(context.Background(), testEvent(1)); err != nil {
		t.Fatal(err)
	}
	if err := app.Events.Publish(context.Background(), testEvent(2)); err != nil {
		t.Fatal(err)
	}
	app.Metrics.Observe("/ok", http.MethodGet, http.StatusOK, time.Millisecond)
	app.Metrics.Observe("/error", http.MethodGet, http.StatusInternalServerError, time.Millisecond)
	app.Metrics.Inc("k8s_degraded")

	metrics := app.RollbackMetrics()
	if metrics.OutboxLag != 2 || metrics.ErrorRatePercent != 50 || metrics.DegradedAdapters != 1 {
		t.Fatalf("rollback metrics = %#v, want lag 2 error 50 degraded 1", metrics)
	}
	if app.CanRollback(DefaultRollbackGate()) {
		t.Fatal("CanRollback(default gate) = true, want false for error/degraded metrics")
	}
	if !app.CanRollback(RollbackGate{MaxOutboxLag: 2, MaxErrorRatePercent: 50, MaxDegradedAdapters: 1}) {
		t.Fatal("CanRollback(custom boundary gate) = false, want true")
	}
}

func TestRollbackTargetSwitches(t *testing.T) {
	app := NewApp(Config{})
	route := RouteSpec{Pattern: "/api/v1/workloads/{id}"}
	if got := app.RollbackTargetFor(route); got != "service" {
		t.Fatalf("default rollback target = %q, want service", got)
	}
	monolithRoute := RouteSpec{Pattern: "/api/v1/legacy/{path...}", ExternalAdapter: "monolith"}
	if got := app.RollbackTargetFor(monolithRoute); got != "monolith" {
		t.Fatalf("monolith route target = %q, want monolith", got)
	}
	app.Switches.Enable(route.Pattern, "workload-service")
	if got := app.RollbackTargetFor(route); got != "workload-service" {
		t.Fatalf("enabled route target = %q, want workload-service", got)
	}
	app.Switches.Rollback(route.Pattern)
	if got := app.RollbackTargetFor(route); got != "monolith" {
		t.Fatalf("rolled back route target = %q, want monolith", got)
	}
}

func withTempWD(t *testing.T, dir string) {
	t.Helper()
	original, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(original); err != nil {
			t.Fatalf("restore working directory: %v", err)
		}
	})
}
