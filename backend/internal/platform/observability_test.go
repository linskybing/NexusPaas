package platform

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
