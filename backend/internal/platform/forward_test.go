package platform

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const forwardProxyHeader = "X-Proxy-Test"

func TestProxyActionFailsClosedWithoutAdapter(t *testing.T) {
	app := NewApp(Config{ServiceName: "all", HTTPAddr: ":0"})
	app.RegisterService(ServiceSpec{
		Name: "test-service",
		Routes: []RouteSpec{{
			Method:   http.MethodGet,
			Pattern:  "/api/v1/unbound/{path...}",
			Resource: "unbound_proxy",
			Action:   "proxy",
		}},
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/unbound/path", nil)
	app.ServeHTTP(rec, req)
	body := rec.Body.String()

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502: %s", rec.Code, body)
	}
	var env Envelope
	decodeEnvelope(t, body, &env)
	if env.Success || env.Degraded == nil {
		t.Fatalf("envelope = %#v, want failed degraded response", env)
	}
	if env.Degraded.Adapter != unboundProxyAdapter || env.Degraded.Code != adapterNotConfiguredCode {
		t.Fatalf("degraded = %#v, want unbound proxy adapter-not-configured", env.Degraded)
	}
	if strings.Contains(body, "policy_checked") {
		t.Fatalf("unbound proxy returned legacy compatibility stub: %s", body)
	}
	if app.Metrics.Counter(unboundProxyAdapter+"_degraded") != 1 {
		t.Fatalf("unbound proxy degraded counter = %d, want 1", app.Metrics.Counter(unboundProxyAdapter+"_degraded"))
	}
}

func TestCustomHandlerCanOwnProxyRouteWithoutAdapter(t *testing.T) {
	app := NewApp(Config{ServiceName: "all", HTTPAddr: ":0"})
	app.RegisterService(ServiceSpec{
		Name: "test-service",
		Routes: []RouteSpec{{
			Method:   http.MethodGet,
			Pattern:  "/api/v1/owned/{path...}",
			Resource: "owned_proxy",
			Action:   "proxy",
		}},
	})
	app.RegisterCustomHandler(http.MethodGet, "/api/v1/owned/{path...}", func(_ *App, _ *http.Request, _ RouteSpec) (int, any, *Degraded) {
		return http.StatusOK, map[string]any{"owned": true}, nil
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/owned/path", nil)
	app.ServeHTTP(rec, req)
	body := rec.Body.String()

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, body)
	}
	var env Envelope
	decodeEnvelope(t, body, &env)
	data := env.Data.(map[string]any)
	if env.Degraded != nil || data["owned"] != true {
		t.Fatalf("envelope = %#v, want custom handler response", env)
	}
	if app.Metrics.Counter(unboundProxyAdapter+"_degraded") != 0 {
		t.Fatalf("custom handler incremented unbound degraded counter")
	}
}

func TestProxyActionPropagatesExternalAdapterResponse(t *testing.T) {
	var seenMethod, seenPath, seenQuery, seenHeader, seenBody string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenMethod = r.Method
		seenPath = r.URL.Path
		seenQuery = r.URL.RawQuery
		seenHeader = r.Header.Get(forwardProxyHeader)
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read upstream body: %v", err)
		}
		seenBody = string(body)
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Upstream", "ok")
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"accepted":true}`))
	}))
	defer upstream.Close()

	app := NewApp(Config{
		ServiceName:  "all",
		HTTPAddr:     ":0",
		ExternalURLs: map[string]string{"test-proxy": upstream.URL},
	})
	app.RegisterService(ServiceSpec{
		Name: "platform-gateway",
		Routes: []RouteSpec{{
			Method:          http.MethodPost,
			Pattern:         "/api/v1/{path...}",
			Resource:        "compat_proxy",
			Action:          "proxy",
			ExternalAdapter: "test-proxy",
		}},
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/widgets?project_id=p1", strings.NewReader(`{"name":"w1"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(forwardProxyHeader, "yes")
	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202, body=%s", rec.Code, rec.Body.String())
	}
	if rec.Body.String() != `{"accepted":true}` {
		t.Fatalf("body = %q, want raw upstream body", rec.Body.String())
	}
	if rec.Header().Get("Content-Type") != "application/json" || rec.Header().Get("X-Upstream") != "ok" {
		t.Fatalf("response headers were not propagated: %+v", rec.Header())
	}
	if seenMethod != http.MethodPost || seenPath != "/api/v1/widgets" || seenQuery != "project_id=p1" {
		t.Fatalf("upstream saw %s %s?%s", seenMethod, seenPath, seenQuery)
	}
	if seenHeader != "yes" || seenBody != `{"name":"w1"}` {
		t.Fatalf("upstream saw header=%q body=%q", seenHeader, seenBody)
	}
}

func decodeEnvelope(t *testing.T, body string, env *Envelope) {
	t.Helper()
	if err := json.NewDecoder(strings.NewReader(body)).Decode(env); err != nil {
		t.Fatal(err)
	}
}
