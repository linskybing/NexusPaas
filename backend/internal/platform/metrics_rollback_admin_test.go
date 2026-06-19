package platform

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
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
	metrics.Observe("/api/v1/a", http.MethodGet, http.StatusOK, 200*time.Millisecond)
	metrics.Observe("/api/v1/a", http.MethodGet, http.StatusOK, 1500*time.Millisecond)
	metrics.Observe("/api/v1/a", http.MethodPost, http.StatusInternalServerError, 500*time.Millisecond)
	metrics.Inc("k8s-degraded")

	rec := httptest.NewRecorder()
	metrics.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	body := rec.Body.String()
	for _, want := range []string{
		"# TYPE nexuspaas_http_requests_total counter",
		`nexuspaas_http_requests_total{route="/api/v1/a",method="GET",status="200"} 2`,
		`nexuspaas_http_requests_total{route="/api/v1/a",method="POST",status="500"} 1`,
		"# TYPE nexuspaas_http_request_duration_seconds histogram",
		"nexuspaas_k8s_degraded_total 1",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("metrics body missing %q:\n%s", want, body)
		}
	}
	getLabels := `route="/api/v1/a",method="GET",status="200"`
	postLabels := `route="/api/v1/a",method="POST",status="500"`
	if got := metricSampleInt(t, body, "nexuspaas_http_request_duration_seconds_bucket", getLabels+`,le="0.1"`); got != 0 {
		t.Fatalf("GET le=0.1 bucket = %d, want 0", got)
	}
	if got := metricSampleInt(t, body, "nexuspaas_http_request_duration_seconds_bucket", getLabels+`,le="0.25"`); got != 1 {
		t.Fatalf("GET le=0.25 bucket = %d, want 1", got)
	}
	if got := metricSampleInt(t, body, "nexuspaas_http_request_duration_seconds_bucket", getLabels+`,le="0.5"`); got != 1 {
		t.Fatalf("GET le=0.5 bucket = %d, want 1", got)
	}
	if got := metricSampleInt(t, body, "nexuspaas_http_request_duration_seconds_bucket", getLabels+`,le="2"`); got != 2 {
		t.Fatalf("GET le=2 bucket = %d, want 2", got)
	}
	getInf := metricSampleInt(t, body, "nexuspaas_http_request_duration_seconds_bucket", getLabels+`,le="+Inf"`)
	getCount := metricSampleInt(t, body, "nexuspaas_http_request_duration_seconds_count", getLabels)
	if getInf != 2 || getCount != 2 || getInf != getCount {
		t.Fatalf("GET +Inf/count = %d/%d, want 2 and equal", getInf, getCount)
	}
	if got := metricSampleFloat(t, body, "nexuspaas_http_request_duration_seconds_sum", getLabels); got != 1.7 {
		t.Fatalf("GET duration sum = %f, want 1.7", got)
	}
	if got := metricSampleInt(t, body, "nexuspaas_http_request_duration_seconds_bucket", postLabels+`,le="0.3"`); got != 0 {
		t.Fatalf("POST le=0.3 bucket = %d, want 0", got)
	}
	if got := metricSampleInt(t, body, "nexuspaas_http_request_duration_seconds_bucket", postLabels+`,le="0.5"`); got != 1 {
		t.Fatalf("POST le=0.5 bucket = %d, want 1", got)
	}
	if got := metricSampleInt(t, body, "nexuspaas_http_request_duration_seconds_count", postLabels); got != 1 {
		t.Fatalf("POST duration count = %d, want 1", got)
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

func TestMetricsLabeledSamplesAreSnapshotsAndEscapeLabels(t *testing.T) {
	metrics := NewMetrics()
	labels := map[string]string{"consumer": "worker\"\\\n1"}
	metrics.SetGauge(metricEventConsumerLag, labels, 2)
	metrics.SetGauge(metricEventConsumerLag, labels, 5)
	metrics.SetCounter(metricProjectionApplied, labels, 3)
	metrics.SetCounter(metricProjectionApplied, labels, 3)

	rec := httptest.NewRecorder()
	metrics.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	body := rec.Body.String()
	escapedLabels := "consumer=\"worker\\\"\\\\\\n1\""
	if got := metricSampleInt(t, body, metricEventConsumerLag, escapedLabels); got != 5 {
		t.Fatalf("consumer lag gauge = %d, want latest snapshot 5", got)
	}
	if got := metricSampleInt(t, body, metricProjectionApplied, escapedLabels); got != 3 {
		t.Fatalf("projection applied counter = %d, want set snapshot 3", got)
	}
	if strings.Count(body, metricEventConsumerLag+"{") != 1 {
		t.Fatalf("consumer lag should have one escaped sample, body:\n%s", body)
	}
}

func metricSampleInt(t *testing.T, body, metric, labels string) int {
	t.Helper()
	value, err := strconv.Atoi(metricSampleValue(t, body, metric, labels))
	if err != nil {
		t.Fatalf("%s{%s} value is not an int: %v", metric, labels, err)
	}
	return value
}

func metricSampleFloat(t *testing.T, body, metric, labels string) float64 {
	t.Helper()
	value, err := strconv.ParseFloat(metricSampleValue(t, body, metric, labels), 64)
	if err != nil {
		t.Fatalf("%s{%s} value is not a float: %v", metric, labels, err)
	}
	return value
}

func metricSampleNoLabelsInt(t *testing.T, body, metric string) int {
	t.Helper()
	value, err := strconv.Atoi(metricSampleNoLabelsValue(t, body, metric))
	if err != nil {
		t.Fatalf("%s value is not an int: %v", metric, err)
	}
	return value
}

func metricSampleValue(t *testing.T, body, metric, labels string) string {
	t.Helper()
	prefix := metric + "{" + labels + "} "
	for _, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(line, prefix) {
			return strings.TrimSpace(strings.TrimPrefix(line, prefix))
		}
	}
	t.Fatalf("metrics body missing %s{%s}:\n%s", metric, labels, body)
	return ""
}

func metricSampleNoLabelsValue(t *testing.T, body, metric string) string {
	t.Helper()
	prefix := metric + " "
	for _, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(line, prefix) {
			return strings.TrimSpace(strings.TrimPrefix(line, prefix))
		}
	}
	t.Fatalf("metrics body missing %s:\n%s", metric, body)
	return ""
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
