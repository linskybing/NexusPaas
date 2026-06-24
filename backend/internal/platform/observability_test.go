package platform

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/contracts"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

func TestWrapEmitsSpanAndCorrelatesTrace(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	prevTP := otel.GetTracerProvider()
	prevProp := otel.GetTextMapPropagator()
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.TraceContext{})
	t.Cleanup(func() {
		otel.SetTracerProvider(prevTP)
		otel.SetTextMapPropagator(prevProp)
	})

	app := NewApp(Config{ServiceName: "all", HTTPAddr: ":0", ExternalURLs: map[string]string{}})
	var handlerTrace string
	app.RegisterCustomHandler(http.MethodGet, "/api/v1/ping", func(_ *App, r *http.Request, _ RouteSpec) (int, any, *Degraded) {
		handlerTrace = trace.SpanContextFromContext(r.Context()).TraceID().String()
		return http.StatusOK, map[string]any{"ok": true}, nil
	})
	app.RegisterService(ServiceSpec{Name: "probe", Routes: []RouteSpec{{
		Method: http.MethodGet, Pattern: "/api/v1/ping", Resource: "ping", Action: "command",
	}}})

	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/ping", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}

	spans := recorder.Ended()
	if len(spans) != 1 {
		t.Fatalf("recorded %d spans, want 1", len(spans))
	}
	span := spans[0]
	if span.Name() != "GET /api/v1/ping" {
		t.Fatalf("span name = %q, want %q", span.Name(), "GET /api/v1/ping")
	}
	traceID := span.SpanContext().TraceID().String()
	if handlerTrace != traceID {
		t.Fatalf("handler trace %q != span trace %q", handlerTrace, traceID)
	}
	if !hasIntAttr(span.Attributes(), "http.response.status_code", 200) {
		t.Fatalf("span attributes missing status_code=200: %#v", span.Attributes())
	}

	var envelope struct {
		TraceID string `json:"trace_id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode envelope: %v", err)
	}
	if envelope.TraceID != traceID {
		t.Fatalf("envelope trace_id = %q, want %q (span trace)", envelope.TraceID, traceID)
	}
}

func TestReadyzFailsClosedWhenAuthUnconfigured(t *testing.T) {
	cases := []struct {
		name string
		cfg  Config
		want int
	}{
		{name: "auth not required is ready", cfg: Config{ServiceName: "all", ExternalURLs: map[string]string{}}, want: http.StatusOK},
		{name: "auth required without material is unavailable", cfg: Config{ServiceName: "all", RequireAuth: true, ExternalURLs: map[string]string{}}, want: http.StatusServiceUnavailable},
		{name: "auth required with api key is ready", cfg: Config{ServiceName: "all", RequireAuth: true, APIKeys: map[string]bool{"k": true}, ExternalURLs: map[string]string{}}, want: http.StatusOK},
		{name: "auth required with incomplete jwks is unavailable", cfg: Config{ServiceName: "all", RequireAuth: true, JWKSURL: "https://issuer.test/jwks", ExternalURLs: map[string]string{}}, want: http.StatusServiceUnavailable},
		{name: "auth required with jwks is ready", cfg: Config{ServiceName: "all", RequireAuth: true, JWKSURL: "https://issuer.test/jwks", JWTIssuer: "https://issuer.test", JWTAudiences: map[string]bool{"api": true}, ExternalURLs: map[string]string{}}, want: http.StatusOK},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			app := NewApp(tc.cfg)
			rec := httptest.NewRecorder()
			app.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))
			if rec.Code != tc.want {
				t.Fatalf("/readyz status = %d, want %d: %s", rec.Code, tc.want, rec.Body.String())
			}
		})
	}
}

func TestOutboxEndpointRedactsSensitivePayload(t *testing.T) {
	app := NewApp(Config{ServiceName: "all", HTTPAddr: ":0", ExternalURLs: map[string]string{}})
	if err := app.Events.Publish(context.Background(), contracts.Event{
		EventID:       "evt-1",
		Name:          "TokenIssued",
		Source:        "identity-service",
		OccurredAt:    time.Now().UTC(),
		TraceID:       "trace-1",
		SchemaVersion: 1,
		Data: map[string]any{
			"user_id":      "u1",
			"access_token": "secret-token",
		},
	}); err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/outbox", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("/outbox status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var envelope struct {
		Data []contracts.Event `json:"data"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&envelope); err != nil {
		t.Fatal(err)
	}
	if len(envelope.Data) != 1 {
		t.Fatalf("outbox events = %d, want 1", len(envelope.Data))
	}
	data := envelope.Data[0].Data
	if data["access_token"] != redactedValue || data["user_id"] != "u1" {
		t.Fatalf("outbox endpoint data was not redacted correctly: %#v", data)
	}
}

func TestOperationalEndpointsExposeOutboxInboxRuntimeEvidence(t *testing.T) {
	app := NewApp(Config{ServiceName: "all", HTTPAddr: ":0", ExternalURLs: map[string]string{}})
	ctx := context.Background()
	publishTestEvent(t, app, "e1", "ThingCreated")
	publishTestEvent(t, app, "e2", "ThingUpdated")

	app.RunProjection(ctx, "read-model", func(contracts.Event) error { return nil })
	publishTestEvent(t, app, "e3", "ThingFailed")
	app.RunProjection(ctx, "dead-letter-model", failEventID("e3"))
	app.ReplayProjection("dead-letter-model")
	app.RunProjection(ctx, "dead-letter-model", failEventID("e3"))

	statusByConsumer := projectionStatusByConsumer(t, app)
	if statusByConsumer["read-model"].Lag != 1 || statusByConsumer["read-model"].Applied != 2 {
		t.Fatalf("read-model projection status = %#v, want lag=1 applied=2", statusByConsumer["read-model"])
	}
	assertDeadLetterProjectionStatus(t, statusByConsumer["dead-letter-model"])

	body := metricsBody(t, app)
	if got := metricSampleNoLabelsInt(t, body, metricEventOutboxEvents); got != 3 {
		t.Fatalf("outbox events gauge = %d, want 3", got)
	}
	if got := metricSampleInt(t, body, metricEventConsumerLag, `consumer="read-model"`); got != 1 {
		t.Fatalf("read-model lag metric = %d, want 1", got)
	}
	if got := metricSampleInt(t, body, metricProjectionApplied, `consumer="read-model"`); got != 2 {
		t.Fatalf("read-model applied metric = %d, want 2", got)
	}
	if got := metricSampleInt(t, body, metricProjectionDeadLetters, `consumer="dead-letter-model"`); got != 2 {
		t.Fatalf("dead-letter metric = %d, want 2", got)
	}
	if got := metricSampleInt(t, body, metricProjectionRetries, `consumer="dead-letter-model"`); got != 1 {
		t.Fatalf("retry metric = %d, want 1", got)
	}
	if got := metricSampleInt(t, body, metricProjectionReplays, `consumer="dead-letter-model"`); got != 1 {
		t.Fatalf("replay metric = %d, want 1", got)
	}

	second := httptest.NewRecorder()
	app.ServeHTTP(second, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if got := metricSampleInt(t, second.Body.String(), metricProjectionApplied, `consumer="read-model"`); got != 2 {
		t.Fatalf("read-model applied metric after second scrape = %d, want unchanged 2", got)
	}
}

func TestOperationalEndpointExposesMonitoringAcceptanceMetrics(t *testing.T) {
	app := NewApp(Config{ServiceName: "all", HTTPAddr: ":0", ExternalURLs: map[string]string{}})
	ctx := context.Background()
	records := []struct {
		resource string
		data     map[string]any
	}{
		{monitoringWorkloadJobsResource, map[string]any{"id": "job-submitted", "status": "submitted", "project_id": "P1", "user_id": "U1"}},
		{monitoringWorkloadJobsResource, map[string]any{"id": "job-queued-stream", "status": "queued", "streaming_session": true, "stream_max_bitrate_kbps": 8000}},
		{monitoringWorkloadJobsResource, map[string]any{"id": "job-running-stream", "status": "running", "streaming_session": true, "stream_max_bitrate_kbps": 12000}},
		{monitoringWorkloadJobsResource, map[string]any{"id": "job-preempted", "status": "preempted"}},
		{monitoringWorkloadJobsResource, map[string]any{"id": "job-rejected", "status": "rejected"}},
		{monitoringWorkloadJobsResource, map[string]any{"id": "job-waiting-infra", "status": "waiting_infra", "status_reason": "waiting for workload infrastructure recovery"}},
		{monitoringWorkloadJobsResource, map[string]any{"id": "job-permanent-apply", "status": "failed", "error_message": "unsupported Kubernetes manifest kind: CronJob"}},
		{monitoringWorkloadJobsResource, map[string]any{"id": "job-runtime-failed", "status": "failed", "error_message": "runtime limit exceeded"}},
		{monitoringImageBuildJobsResource, map[string]any{"id": "build-running", "status": "running", "image_reference": "registry.local/team/app:run"}},
		{monitoringImageBuildJobsResource, map[string]any{"id": "build-building", "status": "building"}},
		{monitoringImageBuildJobsResource, map[string]any{"id": "build-failed", "status": "failed"}},
		{monitoringImageBuildJobsResource, map[string]any{"id": "build-succeeded", "status": "succeeded"}},
		{monitoringImageBuildJobsResource, map[string]any{"id": "build-completed", "status": "completed"}},
		{monitoringImageBuildJobsResource, map[string]any{"id": "build-timeout", "status": "timeout"}},
		{monitoringImageBuildJobsResource, map[string]any{"id": "build-timed-out", "status": "timed_out"}},
		{monitoringImageBuildJobsResource, map[string]any{"id": "build-cancelled", "status": "cancelled"}},
	}
	for _, record := range records {
		if _, err := app.Store.Create(ctx, record.resource, record.data); err != nil {
			t.Fatalf("seed %s/%s: %v", record.resource, record.data["id"], err)
		}
	}

	body := metricsBody(t, app)
	assertMonitoringMetricSamples(t, body, []monitoringMetricExpectation{
		{metric: metricWorkloadQueueJobs, labels: `status="pending"`, want: 3},
		{metric: metricWorkloadQueueJobs, labels: `status="running"`, want: 1},
		{metric: metricWorkloadQueueJobs, labels: `status="preempted"`, want: 1},
		{metric: metricWorkloadQueueJobs, labels: `status="rejected"`, want: 1},
		{metric: metricImageBuildJobs, labels: `status="running"`, want: 2},
		{metric: metricImageBuildJobs, labels: `status="failed"`, want: 1},
		{metric: metricImageBuildJobs, labels: `status="succeeded"`, want: 2},
		{metric: metricImageBuildJobs, labels: `status="timeout"`, want: 2},
		{metric: metricWebRTCActiveSessions, want: 2},
		{metric: metricWebRTCEgressBitrateKbps, want: 20000},
		{metric: metricKubernetesApplyFailures, labels: `reason="infrastructure_recovery"`, want: 1},
		{metric: metricKubernetesApplyFailures, labels: `reason="permanent_apply_failure"`, want: 1},
	})
	assertMonitoringMetricsDoNotContain(t, body, "job-running-stream", "P1", "U1", "registry.local/team/app:run", "CronJob")
}

func TestMonitoringAcceptanceMetricsStayWithinServiceOwnership(t *testing.T) {
	app := NewApp(Config{ServiceName: "workload-service", HTTPAddr: ":0", ExternalURLs: map[string]string{}})
	ctx := context.Background()
	for _, record := range []struct {
		resource string
		data     map[string]any
	}{
		{monitoringWorkloadJobsResource, map[string]any{"id": "job-running", "status": "running"}},
		{monitoringImageBuildJobsResource, map[string]any{"id": "build-running", "status": "running"}},
	} {
		if _, err := app.Store.Create(ctx, record.resource, record.data); err != nil {
			t.Fatalf("seed %s/%s: %v", record.resource, record.data["id"], err)
		}
	}

	body := metricsBody(t, app)
	if got := metricSampleInt(t, body, metricWorkloadQueueJobs, `status="running"`); got != 1 {
		t.Fatalf("workload running metric = %d, want 1", got)
	}
	if strings.Contains(body, metricImageBuildJobs) {
		t.Fatalf("workload service metrics included image build metric:\n%s", body)
	}
}

type monitoringMetricExpectation struct {
	metric string
	labels string
	want   int
}

func assertMonitoringMetricSamples(t *testing.T, body string, samples []monitoringMetricExpectation) {
	t.Helper()
	for _, sample := range samples {
		var got int
		if sample.labels != "" {
			got = metricSampleInt(t, body, sample.metric, sample.labels)
		} else {
			got = metricSampleNoLabelsInt(t, body, sample.metric)
		}
		if got != sample.want {
			t.Fatalf("%s{%s} = %d, want %d", sample.metric, sample.labels, got, sample.want)
		}
	}
}

func assertMonitoringMetricsDoNotContain(t *testing.T, body string, values ...string) {
	t.Helper()
	for _, value := range values {
		if strings.Contains(body, value) {
			t.Fatalf("metrics body leaked high-cardinality value %q:\n%s", value, body)
		}
	}
}

func failEventID(id string) func(contracts.Event) error {
	return func(event contracts.Event) error {
		if event.EventID == id {
			return errors.New("boom")
		}
		return nil
	}
}

func projectionStatusByConsumer(t *testing.T, app *App) map[string]ProjectionStatus {
	t.Helper()
	projections := httptest.NewRecorder()
	app.ServeHTTP(projections, httptest.NewRequest(http.MethodGet, "/projections", nil))
	if projections.Code != http.StatusOK {
		t.Fatalf("/projections status = %d, want 200: %s", projections.Code, projections.Body.String())
	}
	var projectionEnvelope struct {
		Data []ProjectionStatus `json:"data"`
	}
	if err := json.NewDecoder(projections.Body).Decode(&projectionEnvelope); err != nil {
		t.Fatal(err)
	}
	statusByConsumer := map[string]ProjectionStatus{}
	for _, status := range projectionEnvelope.Data {
		statusByConsumer[status.Consumer] = status
	}
	return statusByConsumer
}

func assertDeadLetterProjectionStatus(t *testing.T, status ProjectionStatus) {
	t.Helper()
	if status.Lag != 0 || status.DeadLettered != 2 || status.RetryCount != 1 ||
		status.ReplayCount != 1 || status.ReplayPending || status.LastReplayAt.IsZero() {
		t.Fatalf("dead-letter projection status = %#v, want lag=0 dead_lettered=2 retry=1 replay=1 pending=false", status)
	}
}

func metricsBody(t *testing.T, app *App) string {
	t.Helper()
	metrics := httptest.NewRecorder()
	app.ServeHTTP(metrics, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if metrics.Code != http.StatusOK {
		t.Fatalf("/metrics status = %d, want 200: %s", metrics.Code, metrics.Body.String())
	}
	return metrics.Body.String()
}

func TestNewAppInjectsPorts(t *testing.T) {
	fake := &fakeEventStream{}
	app := NewApp(Config{ServiceName: "all", ExternalURLs: map[string]string{}}, WithEventBus(fake))
	if app.Events != fake {
		t.Fatal("WithEventBus did not inject the provided event stream")
	}
	if app.Store == nil {
		t.Fatal("default store must not be nil when WithStore is omitted")
	}
	// A nil option value must not clobber the default.
	app2 := NewApp(Config{ServiceName: "all", ExternalURLs: map[string]string{}}, WithEventBus(nil))
	if app2.Events == nil {
		t.Fatal("WithEventBus(nil) must keep the default event stream")
	}
}

func hasIntAttr(attrs []attribute.KeyValue, key string, want int64) bool {
	for _, attr := range attrs {
		if string(attr.Key) == key && attr.Value.AsInt64() == want {
			return true
		}
	}
	return false
}

type fakeEventStream struct {
	published []contracts.Event
}

func (f *fakeEventStream) Publish(_ context.Context, event contracts.Event) error {
	f.published = append(f.published, event)
	return nil
}

func (f *fakeEventStream) Consume(context.Context, string, contracts.Event) (bool, error) {
	return true, nil
}

func (f *fakeEventStream) Outbox() []contracts.Event { return f.published }

func (f *fakeEventStream) Checkpoint(string) {}

func (f *fakeEventStream) Lag(string) int { return 0 }

func (f *fakeEventStream) ResetConsumer(string) {}

func (f *fakeEventStream) ResetConsumerEvents(string, []string) {}
