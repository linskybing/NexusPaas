package platform

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/contracts"
)

func TestExternalAdapterRewritesPathAndInjectsUpstreamAuth(t *testing.T) {
	var seenPath, seenAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenPath = r.URL.Path
		seenAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	adapter := NewExternalAdapter("test", server.URL, time.Second, 1, 2, time.Second)
	adapter.configure(AdapterConfig{
		StripPrefix: "/api/v1/pgadmin",
		AddPrefix:   "/admin",
		Auth:        AdapterAuthConfig{Type: "bearer", Token: "upstream-secret"},
	})

	_, result, err := adapter.Proxy(context.Background(), contracts.AdapterProxyRequest{
		Operation: "proxy",
		Method:    http.MethodGet,
		Path:      "/api/v1/pgadmin/browser",
		Header:    http.Header{"Authorization": []string{"Bearer client-forged"}},
	})
	if err != nil || result.Degraded {
		t.Fatalf("proxy result = %+v err=%v", result, err)
	}
	if seenPath != "/admin/browser" {
		t.Fatalf("upstream path = %q, want /admin/browser (strip+add prefix)", seenPath)
	}
	if seenAuth != "Bearer upstream-secret" {
		t.Fatalf("upstream Authorization = %q, want injected upstream credential (client value overwritten)", seenAuth)
	}
}

func TestAdapterAuthConfigHeader(t *testing.T) {
	name, value := AdapterAuthConfig{Type: "bearer", Token: "tok"}.header()
	if name != "Authorization" || value != "Bearer tok" {
		t.Fatalf("bearer = %q/%q", name, value)
	}
	name, value = AdapterAuthConfig{Type: "basic", Username: "u", Password: "p"}.header()
	if name != "Authorization" || value != "Basic "+base64.StdEncoding.EncodeToString([]byte("u:p")) {
		t.Fatalf("basic = %q/%q", name, value)
	}
	name, value = AdapterAuthConfig{Type: "header", Header: "X-Api-Key", Value: "k"}.header()
	if name != "X-Api-Key" || value != "k" {
		t.Fatalf("header = %q/%q", name, value)
	}
	if n, v := (AdapterAuthConfig{Type: "bogus"}).header(); n != "" || v != "" {
		t.Fatalf("unknown type should yield empty header, got %q/%q", n, v)
	}
	if n, v := (AdapterAuthConfig{Type: "bearer"}).header(); n != "" || v != "" {
		t.Fatalf("bearer without token should be empty, got %q/%q", n, v)
	}
}

func TestParseAdapterConfigsAppliedToAdapter(t *testing.T) {
	cfg := Config{
		ExternalURLs:   map[string]string{"pgadmin": "http://pgadmin:80"},
		AdapterConfigs: parseAdapterConfigs(`{"pgadmin":{"strip_prefix":"/api/v1/pgadmin","auth":{"type":"header","header":"X-Token","value":"abc"}}}`),
	}
	app := NewApp(cfg)
	adapter, ok := app.Adapters["pgadmin"].(*ExternalAdapter)
	if !ok {
		t.Fatal("pgadmin adapter missing")
	}
	if adapter.stripPrefix != "/api/v1/pgadmin" || adapter.authHeaderName != "X-Token" || adapter.authHeaderValue != "abc" {
		t.Fatalf("adapter not configured from ADAPTER_CONFIG: strip=%q authName=%q authValue=%q",
			adapter.stripPrefix, adapter.authHeaderName, adapter.authHeaderValue)
	}
}
