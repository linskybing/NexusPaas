//go:build e2e

package e2e

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/platform"
)

const (
	proxySensitiveAdapter = "proxy-sensitive-e2e"
	proxySensitiveHeader  = "X-Proxy-Test"
)

type proxyHeaderObservation struct {
	method string
	path   string
	query  string
	header http.Header
	body   string
}

func TestProxySensitiveHeaderStrippingE2E(t *testing.T) {
	t.Run("buffered proxy strips platform credentials", runBufferedProxySensitiveHeaderE2E)
	t.Run("streaming proxy strips platform credentials", runStreamingProxySensitiveHeaderE2E)
}

func runBufferedProxySensitiveHeaderE2E(t *testing.T) {
	observed := make(chan proxyHeaderObservation, 1)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read upstream body: %v", err)
		}
		observed <- proxyHeaderObservation{
			method: r.Method,
			path:   r.URL.Path,
			query:  r.URL.RawQuery,
			header: r.Header.Clone(),
			body:   string(body),
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Upstream", "buffered")
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"accepted":true}`))
	}))
	defer upstream.Close()

	server := httptest.NewServer(newProxySensitiveHeaderApp(upstream.URL))
	defer server.Close()

	req := newBufferedProxySensitiveHeaderRequest(t, server.URL)
	resp, respBody := doProxySensitiveHeaderRequest(t, server, req)
	assertBufferedProxyResponse(t, resp, respBody)
	assertBufferedProxyObservation(t, receiveProxyHeaderObservation(t, observed))
}

func runStreamingProxySensitiveHeaderE2E(t *testing.T) {
	observed := make(chan proxyHeaderObservation, 1)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		observed <- proxyHeaderObservation{
			method: r.Method,
			path:   r.URL.Path,
			query:  r.URL.RawQuery,
			header: r.Header.Clone(),
		}
		w.Header().Set("Content-Type", "text/plain")
		w.Header().Set("X-Upstream", "streaming")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("chunk-1\n"))
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
		_, _ = w.Write([]byte("chunk-2\n"))
	}))
	defer upstream.Close()

	server := httptest.NewServer(newProxySensitiveHeaderApp(upstream.URL))
	defer server.Close()

	req := newStreamingProxySensitiveHeaderRequest(t, server.URL)
	resp, respBody := doProxySensitiveHeaderRequest(t, server, req)
	assertStreamingProxyResponse(t, resp, respBody)
	assertStreamingProxyObservation(t, receiveProxyHeaderObservation(t, observed))
}

func newProxySensitiveHeaderApp(upstreamURL string) *platform.App {
	app := platform.NewApp(platform.Config{
		ServiceName:  "all",
		ExternalURLs: map[string]string{proxySensitiveAdapter: upstreamURL},
	})
	app.RegisterService(platform.ServiceSpec{
		Name: "platform-gateway",
		Routes: []platform.RouteSpec{
			{
				Method:          http.MethodPost,
				Pattern:         "/api/v1/e2e-proxy/{path...}",
				Resource:        "platform-gateway:e2e_proxy",
				Action:          "proxy",
				ExternalAdapter: proxySensitiveAdapter,
			},
			{
				Method:          http.MethodGet,
				Pattern:         "/api/v1/e2e-stream/{path...}",
				Resource:        "platform-gateway:e2e_stream_proxy",
				Action:          "proxy",
				ExternalAdapter: proxySensitiveAdapter,
			},
		},
	})
	return app
}

func newBufferedProxySensitiveHeaderRequest(t *testing.T, baseURL string) *http.Request {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, baseURL+"/api/v1/e2e-proxy/widgets?project_id=p1", strings.NewReader(`{"name":"w1"}`))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(proxySensitiveHeader, "buffered-ok")
	addSensitiveProxyHeaders(req)
	return req
}

func newStreamingProxySensitiveHeaderRequest(t *testing.T, baseURL string) *http.Request {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, baseURL+"/api/v1/e2e-stream/logs?stream=true", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set(proxySensitiveHeader, "streaming-ok")
	addSensitiveProxyHeaders(req)
	return req
}

func doProxySensitiveHeaderRequest(t *testing.T, server *httptest.Server, req *http.Request) (*http.Response, []byte) {
	t.Helper()
	resp, err := server.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return resp, body
}

func assertBufferedProxyResponse(t *testing.T, resp *http.Response, body []byte) {
	t.Helper()
	if resp.StatusCode != http.StatusAccepted || string(body) != `{"accepted":true}` {
		t.Fatalf("buffered response = %d %q", resp.StatusCode, body)
	}
	if resp.Header.Get("X-Upstream") != "buffered" {
		t.Fatalf("buffered response headers = %#v", resp.Header)
	}
}

func assertStreamingProxyResponse(t *testing.T, resp *http.Response, body []byte) {
	t.Helper()
	if resp.StatusCode != http.StatusOK || string(body) != "chunk-1\nchunk-2\n" {
		t.Fatalf("streaming response = %d %q", resp.StatusCode, body)
	}
	if resp.Header.Get("X-Upstream") != "streaming" {
		t.Fatalf("streaming response headers = %#v", resp.Header)
	}
}

func assertBufferedProxyObservation(t *testing.T, got proxyHeaderObservation) {
	t.Helper()
	if got.method != http.MethodPost || got.path != "/api/v1/e2e-proxy/widgets" || got.query != "project_id=p1" {
		t.Fatalf("upstream saw %s %s?%s", got.method, got.path, got.query)
	}
	assertSensitiveHeadersAbsentE2E(t, got.header)
	if got.header.Get(proxySensitiveHeader) != "buffered-ok" {
		t.Fatalf("upstream %s = %q", proxySensitiveHeader, got.header.Get(proxySensitiveHeader))
	}
	if got.header.Get("Content-Type") != "application/json" {
		t.Fatalf("upstream Content-Type = %q", got.header.Get("Content-Type"))
	}
	if got.body != `{"name":"w1"}` {
		t.Fatalf("upstream body = %q", got.body)
	}
}

func assertStreamingProxyObservation(t *testing.T, got proxyHeaderObservation) {
	t.Helper()
	if got.method != http.MethodGet || got.path != "/api/v1/e2e-stream/logs" || got.query != "stream=true" {
		t.Fatalf("upstream saw %s %s?%s", got.method, got.path, got.query)
	}
	assertSensitiveHeadersAbsentE2E(t, got.header)
	if got.header.Get(proxySensitiveHeader) != "streaming-ok" {
		t.Fatalf("upstream %s = %q", proxySensitiveHeader, got.header.Get(proxySensitiveHeader))
	}
	if got.header.Get("Accept") != "text/event-stream" {
		t.Fatalf("upstream Accept = %q", got.header.Get("Accept"))
	}
}

func addSensitiveProxyHeaders(req *http.Request) {
	req.Header.Set("Authorization", "Bearer caller-jwt")
	req.Header.Set("Proxy-Authorization", "Bearer proxy-jwt")
	req.Header.Set("X-API-Key", "platform-api-key")
	req.Header.Set("API-Key", "legacy-platform-api-key")
	req.Header.Set("X-Service-Key", "internal-service-key")
	req.AddCookie(&http.Cookie{Name: "token", Value: "platform-access"})
	req.AddCookie(&http.Cookie{Name: "refresh_token", Value: "platform-refresh"})
}

func receiveProxyHeaderObservation(t *testing.T, observed <-chan proxyHeaderObservation) proxyHeaderObservation {
	t.Helper()
	select {
	case got := <-observed:
		return got
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for upstream proxy request")
		return proxyHeaderObservation{}
	}
}

func assertSensitiveHeadersAbsentE2E(t *testing.T, header http.Header) {
	t.Helper()
	for _, key := range []string{"Authorization", "Proxy-Authorization", "Cookie", "X-API-Key", "API-Key", "X-Service-Key"} {
		if got := header.Get(key); got != "" {
			t.Fatalf("%s reached upstream: %q", key, got)
		}
	}
}
