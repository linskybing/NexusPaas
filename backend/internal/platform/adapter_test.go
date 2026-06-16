package platform

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/contracts"
)

const (
	testAdapterName       = "test"
	testContentTypeHeader = "Content-Type"
	testProxyHeader       = "X-Proxy-Test"
	testHopHeader         = "X-Hop"
	testAdapterTimeout    = 2 * time.Second
)

func TestExternalAdapterCircuitBreakerHalfOpenProbe(t *testing.T) {
	var healthy atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if healthy.Load() {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	adapter := NewExternalAdapter(testAdapterName, server.URL, testAdapterTimeout, 1, 2, 20*time.Millisecond)
	first, err := adapter.Call(context.Background(), "probe", true)
	if err != nil {
		t.Fatal(err)
	}
	if !first.Degraded || first.Code != "adapter_unavailable" {
		t.Fatalf("first failure = %+v", first)
	}
	second, _ := adapter.Call(context.Background(), "probe", true)
	if !second.Degraded || second.Code != "adapter_unavailable" {
		t.Fatalf("second failure = %+v", second)
	}
	open, _ := adapter.Call(context.Background(), "probe", true)
	if !open.CircuitOpen || open.Code != "circuit_open" {
		t.Fatalf("circuit did not open: %+v", open)
	}

	healthy.Store(true)
	time.Sleep(25 * time.Millisecond)
	recovered, _ := adapter.Call(context.Background(), "probe", true)
	if recovered.Degraded || recovered.Code != "ok" {
		t.Fatalf("half-open probe did not recover circuit: %+v", recovered)
	}
}

func TestExternalAdapterProxyForwardsRequestAndReturnsUpstream(t *testing.T) {
	var seenMethod, seenPath, seenQuery, seenHeader, seenHopHeader, seenBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenMethod = r.Method
		seenPath = r.URL.Path
		seenQuery = r.URL.RawQuery
		seenHeader = r.Header.Get(testProxyHeader)
		seenHopHeader = r.Header.Get(testHopHeader)
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read upstream body: %v", err)
		}
		seenBody = string(body)
		w.Header().Set(testContentTypeHeader, "application/json")
		w.Header().Add("Set-Cookie", "session=upstream")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"proxied":true}`))
	}))
	defer server.Close()

	adapter := NewExternalAdapter(testAdapterName, server.URL+"/base", testAdapterTimeout, 1, 2, 20*time.Millisecond)
	response, result, err := adapter.Proxy(context.Background(), contracts.AdapterProxyRequest{
		Operation: "create_proxy",
		Method:    http.MethodPost,
		Path:      "/api/v1/widgets",
		RawQuery:  "project_id=p1",
		Header: http.Header{
			testProxyHeader: []string{"yes"},
			testHopHeader:   []string{"drop"},
			"Connection":    []string{testHopHeader},
		},
		Body: []byte(`{"name":"w1"}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Degraded {
		t.Fatalf("proxy degraded unexpectedly: %+v", result)
	}
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want 201", response.StatusCode)
	}
	if string(response.Body) != `{"proxied":true}` {
		t.Fatalf("body = %q", response.Body)
	}
	if response.Header.Get(testContentTypeHeader) != "application/json" || response.Header.Get("Set-Cookie") == "" {
		t.Fatalf("response headers were not preserved: %+v", response.Header)
	}
	if seenMethod != http.MethodPost || seenPath != "/base/api/v1/widgets" || seenQuery != "project_id=p1" {
		t.Fatalf("upstream saw %s %s?%s", seenMethod, seenPath, seenQuery)
	}
	if seenHeader != "yes" || seenBody != `{"name":"w1"}` {
		t.Fatalf("upstream saw header=%q body=%q", seenHeader, seenBody)
	}
	if seenHopHeader != "" {
		t.Fatalf("hop-by-hop header was forwarded: %q", seenHopHeader)
	}
}

func TestExternalAdapterProxyStripsSensitiveInboundHeaders(t *testing.T) {
	var seenHeader http.Header
	var seenBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenHeader = r.Header.Clone()
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read upstream body: %v", err)
		}
		seenBody = string(body)
		w.Header().Set(testContentTypeHeader, "application/json")
		w.Header().Set("X-Upstream", "ok")
		w.Header().Add("Set-Cookie", "session=upstream")
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"accepted":true}`))
	}))
	defer server.Close()

	adapter := NewExternalAdapter(testAdapterName, server.URL, testAdapterTimeout, 1, 2, 20*time.Millisecond)
	response, result, err := adapter.Proxy(context.Background(), contracts.AdapterProxyRequest{
		Operation: "secure_proxy",
		Method:    http.MethodPost,
		Path:      "/widgets",
		Header: http.Header{
			"Authorization":       []string{"Bearer caller-jwt"},
			"Proxy-Authorization": []string{"Bearer proxy-jwt"},
			"Cookie":              []string{"token=platform; refresh_token=platform"},
			"X-API-Key":           []string{"platform-api-key"},
			"x-api-key":           []string{"lower-platform-api-key"},
			"API-Key":             []string{"legacy-platform-api-key"},
			"X-Service-Key":       []string{"internal-service-key"},
			testContentTypeHeader: []string{"application/json"},
			testProxyHeader:       []string{"one", "two"},
		},
		Body: []byte(`{"name":"w1"}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Degraded {
		t.Fatalf("proxy degraded unexpectedly: %+v", result)
	}
	if response.StatusCode != http.StatusAccepted || string(response.Body) != `{"accepted":true}` {
		t.Fatalf("response = %d %q", response.StatusCode, response.Body)
	}
	if response.Header.Get("X-Upstream") != "ok" || response.Header.Get("Set-Cookie") == "" {
		t.Fatalf("response headers were not preserved: %+v", response.Header)
	}
	assertSensitiveProxyHeadersAbsent(t, seenHeader)
	if seenHeader.Get(testContentTypeHeader) != "application/json" {
		t.Fatalf("upstream content-type = %q", seenHeader.Get(testContentTypeHeader))
	}
	if got := seenHeader.Values(testProxyHeader); len(got) != 2 || got[0] != "one" || got[1] != "two" {
		t.Fatalf("upstream %s values = %v, want [one two]", testProxyHeader, got)
	}
	if seenBody != `{"name":"w1"}` {
		t.Fatalf("upstream body = %q", seenBody)
	}
}

func TestExternalAdapterProxyInjectsConfiguredAPIKeyAfterStripping(t *testing.T) {
	var seenHeader http.Header
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenHeader = r.Header.Clone()
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	adapter := NewExternalAdapter(testAdapterName, server.URL, testAdapterTimeout, 1, 2, 20*time.Millisecond)
	adapter.configure(AdapterConfig{Auth: AdapterAuthConfig{Type: "header", Header: "X-API-Key", Value: "upstream-api-key"}})

	_, result, err := adapter.Proxy(context.Background(), contracts.AdapterProxyRequest{
		Operation: "secure_proxy",
		Method:    http.MethodGet,
		Path:      "/widgets",
		Header: http.Header{
			"Authorization":       []string{"Bearer caller-jwt"},
			"Cookie":              []string{"token=platform"},
			"X-API-Key":           []string{"platform-api-key"},
			"API-Key":             []string{"legacy-platform-api-key"},
			testContentTypeHeader: []string{"application/json"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Degraded {
		t.Fatalf("proxy degraded unexpectedly: %+v", result)
	}
	if got := seenHeader.Get("X-API-Key"); got != "upstream-api-key" {
		t.Fatalf("upstream X-API-Key = %q, want configured upstream credential", got)
	}
	if got := seenHeader.Get("Authorization"); got != "" {
		t.Fatalf("caller Authorization reached upstream: %q", got)
	}
	if got := seenHeader.Get("Cookie"); got != "" {
		t.Fatalf("caller Cookie reached upstream: %q", got)
	}
	if got := seenHeader.Get("API-Key"); got != "" {
		t.Fatalf("caller API-Key reached upstream: %q", got)
	}
	if seenHeader.Get(testContentTypeHeader) != "application/json" {
		t.Fatalf("upstream content-type = %q", seenHeader.Get(testContentTypeHeader))
	}
}

func TestExternalAdapterProxyPropagatesUpstreamServerError(t *testing.T) {
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts.Add(1)
		w.Header().Set(testContentTypeHeader, "text/plain")
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte("upstream unavailable"))
	}))
	defer server.Close()

	adapter := NewExternalAdapter(testAdapterName, server.URL, testAdapterTimeout, 2, 2, 20*time.Millisecond)
	response, result, err := adapter.Proxy(context.Background(), contracts.AdapterProxyRequest{
		Operation:  "read_proxy",
		Method:     http.MethodGet,
		Path:       "/health",
		Idempotent: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Degraded {
		t.Fatalf("upstream HTTP response should be propagated, got degraded: %+v", result)
	}
	if response.StatusCode != http.StatusBadGateway || strings.TrimSpace(string(response.Body)) != "upstream unavailable" {
		t.Fatalf("response = %d %q", response.StatusCode, response.Body)
	}
	if attempts.Load() != 2 {
		t.Fatalf("attempts = %d, want retry for idempotent proxy", attempts.Load())
	}
}

func assertSensitiveProxyHeadersAbsent(t *testing.T, header http.Header) {
	t.Helper()
	for _, key := range []string{"Authorization", "Proxy-Authorization", "Cookie", "X-API-Key", "API-Key", "X-Service-Key"} {
		if got := header.Get(key); got != "" {
			t.Fatalf("%s reached upstream: %q", key, got)
		}
	}
}
