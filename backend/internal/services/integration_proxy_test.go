package services

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/platform"
)

func TestIntegrationProxyVPNWorkflow(t *testing.T) {
	app := newTestApp()
	seedIntegrationProxyUsers(t, app)
	now := time.Date(2026, 5, 2, 12, 0, 0, 0, time.UTC)
	createRows(t, app, "integration-proxy-service:vpn_clients", []map[string]any{
		{"id": "C1", "commonName": "alice", "realAddress": "1.1.1.1", "virtualAddress": "10.8.0.2", "bytesReceived": int64(10), "bytesSent": int64(20), "connectedSince": now.Add(-time.Hour), "node": "node-a"},
		{"id": "C1DUP", "commonName": "alice", "realAddress": "1.1.1.1", "virtualAddress": "10.8.0.2", "bytesReceived": int64(10), "bytesSent": int64(20), "connectedSince": now.Add(-time.Hour), "node": "node-b"},
		{"id": "C2", "commonName": "bob", "realAddress": "2.2.2.2", "status": "disconnected"},
	})
	createRows(t, app, "integration-proxy-service:vpn_usage_sessions", []map[string]any{
		{"id": "S1", "username": "alice", "connected_since": "2026-05-01T10:00:00Z", "disconnected_at": "2026-05-01T11:00:00Z", "bytes_received": int64(10), "bytes_sent": int64(15)},
		{"id": "S2", "username": "alice", "connected_since": "2026-05-02T10:00:00Z", "last_seen_at": "2026-05-02T11:00:00Z", "bytes_received": int64(3), "bytes_sent": int64(7)},
		{"id": "S3", "username": "bob", "connected_since": "2026-04-01T10:00:00Z", "disconnected_at": "2026-04-01T11:00:00Z", "bytes_received": int64(100), "bytes_sent": int64(200)},
	})

	requestJSON(t, app, http.MethodGet, "/api/v1/admin/vpn/clients", "", nil, http.StatusUnauthorized)
	requestJSON(t, app, http.MethodGet, "/api/v1/admin/vpn/clients", "", adminHeaders("forged"), http.StatusForbidden)
	requestJSON(t, app, http.MethodGet, "/api/v1/admin/vpn/clients", "", userHeaders("U1"), http.StatusForbidden)

	clients := responseMap(t, requestJSON(t, app, http.MethodGet, "/api/v1/admin/vpn/clients", "", adminHeaders("ADMIN"), http.StatusOK))
	if clients["total"] != float64(1) || len(clients["clients"].([]any)) != 1 {
		t.Fatalf("clients = %#v, want one deduplicated active client", clients)
	}

	usage := responseMap(t, requestJSON(t, app, http.MethodGet, "/api/v1/admin/vpn/usage?since=2026-05-01&until=2026-05-02&username=alice", "", adminHeaders("ADMIN"), http.StatusOK))
	if usage["rowCount"] != float64(1) || usage["since"] != "2026-05-01" || usage["until"] != "2026-05-02" {
		t.Fatalf("usage metadata = %#v", usage)
	}
	summary := usage["summary"].(map[string]any)
	if summary["sessionCount"] != float64(2) || summary["totalBytes"] != float64(35) {
		t.Fatalf("summary = %#v, want two alice sessions and 35 total bytes", summary)
	}

	requestJSON(t, app, http.MethodDelete, "/api/v1/admin/vpn/clients/alice", "", adminHeaders("ADMIN"), http.StatusOK)
	clients = responseMap(t, requestJSON(t, app, http.MethodGet, "/api/v1/admin/vpn/clients", "", adminHeaders("ADMIN"), http.StatusOK))
	if clients["total"] != float64(0) {
		t.Fatalf("clients after disconnect = %#v, want no active clients", clients)
	}
}

func TestIntegrationProxySSOAndAuthCheck(t *testing.T) {
	app := newTestApp()
	seedIntegrationProxyUsers(t, app)

	assertPgAdminAuthCheck(t, app)
	assertMinIOConsoleSSO(t, app)
	assertPgAdminConsoleSSO(t, app)
}

func assertPgAdminAuthCheck(t *testing.T, app *platform.App) {
	t.Helper()

	requestJSON(t, app, http.MethodGet, "/api/v1/pgadmin-auth-check", "", userHeaders("U1"), http.StatusForbidden)
	rec := serveRaw(t, app, http.MethodGet, "/api/v1/pgadmin-auth-check", adminHeaders("ADMIN"))
	if rec.Code != http.StatusOK || strings.TrimSpace(rec.Body.String()) != "" {
		t.Fatalf("pgadmin auth-check = %d %q, want empty 200", rec.Code, rec.Body.String())
	}
}

func assertMinIOConsoleSSO(t *testing.T, app *platform.App) {
	t.Helper()

	minioUpstream := newMinIOConsoleSSOUpstream(t)
	defer minioUpstream.Close()
	app.Config.ExternalURLs["minio-console"] = minioUpstream.URL
	app.Config.MinIOConsoleAccessKey = "access"
	app.Config.MinIOConsoleSecretKey = "secret"

	rec := serveRaw(t, app, http.MethodGet, "/api/v1/minio-console-sso", adminHeaders("ADMIN"))
	if rec.Code != http.StatusFound || rec.Header().Get("Location") != "/api/v1/minio-console/" {
		t.Fatalf("minio sso = %d location=%q body=%s", rec.Code, rec.Header().Get("Location"), rec.Body.String())
	}
	if got := strings.Join(rec.Header().Values("Set-Cookie"), "\n"); !strings.Contains(got, "token=minio-session") || !strings.Contains(got, "Path=/api/v1/minio-console/") {
		t.Fatalf("minio Set-Cookie = %q", got)
	}
}

func newMinIOConsoleSSOUpstream(t *testing.T) *httptest.Server {
	t.Helper()

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/login" {
			t.Fatalf("unexpected MinIO request: %s %s", r.Method, r.URL.Path)
		}
		http.SetCookie(w, &http.Cookie{Name: "token", Value: "minio-session", Path: "/"})
		w.WriteHeader(http.StatusNoContent)
	}))
}

func assertPgAdminConsoleSSO(t *testing.T, app *platform.App) {
	t.Helper()

	pgadminUpstream := newPgAdminConsoleSSOUpstream(t)
	defer pgadminUpstream.Close()
	app.Config.ExternalURLs["pgadmin"] = pgadminUpstream.URL
	app.Config.PGAdminDefaultEmail = "admin@test.local"
	app.Config.PGAdminDefaultPassword = "pass"

	rec := serveRaw(t, app, http.MethodGet, "/api/v1/pgadmin-sso", adminHeaders("ADMIN"))
	if rec.Code != http.StatusFound || rec.Header().Get("Location") != "/api/v1/pgadmin/browser/" {
		t.Fatalf("pgadmin sso = %d location=%q body=%s", rec.Code, rec.Header().Get("Location"), rec.Body.String())
	}
	if got := strings.Join(rec.Header().Values("Set-Cookie"), "\n"); !strings.Contains(got, "pga4_session=session") || !strings.Contains(got, "Path=/api/v1/pgadmin/") {
		t.Fatalf("pgadmin Set-Cookie = %q", got)
	}
}

func newPgAdminConsoleSSOUpstream(t *testing.T) *httptest.Server {
	t.Helper()

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/pgadmin/login":
			http.SetCookie(w, &http.Cookie{Name: "anon", Value: "cookie", Path: "/"})
			_, _ = w.Write([]byte(`{"csrfToken": "csrf-123"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/pgadmin/authenticate/login":
			if err := r.ParseForm(); err != nil {
				t.Fatal(err)
			}
			if r.Form.Get("email") != "admin@test.local" || r.Form.Get("password") != "pass" || r.Header.Get("X-pgA-CSRFToken") != "csrf-123" {
				t.Fatalf("unexpected pgadmin auth request: form=%v csrf=%s", r.Form, r.Header.Get("X-pgA-CSRFToken"))
			}
			http.SetCookie(w, &http.Cookie{Name: "pga4_session", Value: "session", Path: "/"})
			http.Redirect(w, r, "/api/v1/pgadmin/browser/", http.StatusFound)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/pgadmin/browser/":
			w.WriteHeader(http.StatusOK)
		default:
			t.Fatalf("unexpected pgAdmin request: %s %s", r.Method, r.URL.Path)
		}
	}))
}

func TestIntegrationProxyPgAdminWildcardUsesConfiguredAdapter(t *testing.T) {
	var seenMethod, seenPath, seenQuery string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenMethod = r.Method
		seenPath = r.URL.Path
		seenQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "text/plain")
		w.Header().Set("X-Upstream", "pgadmin")
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte("pgadmin upstream"))
	}))
	defer upstream.Close()

	app := platform.NewApp(platform.Config{
		ServiceName:  "all",
		HTTPAddr:     ":0",
		ExternalURLs: map[string]string{"pgadmin": upstream.URL},
	})
	RegisterAll(app)

	rec := serveRaw(t, app, http.MethodGet, "/api/v1/pgadmin/browser/?tab=servers", nil)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("pgadmin proxy returned %d, want %d: %s", rec.Code, http.StatusAccepted, rec.Body.String())
	}
	if got := strings.TrimSpace(rec.Body.String()); got != "pgadmin upstream" {
		t.Fatalf("pgadmin proxy body = %q, want upstream body", got)
	}
	if rec.Header().Get("X-Upstream") != "pgadmin" {
		t.Fatalf("pgadmin proxy header = %q, want upstream header", rec.Header().Get("X-Upstream"))
	}
	if seenMethod != http.MethodGet || seenPath != "/api/v1/pgadmin/browser/" || seenQuery != "tab=servers" {
		t.Fatalf("upstream saw %s %s?%s, want GET /api/v1/pgadmin/browser/?tab=servers", seenMethod, seenPath, seenQuery)
	}
}

func TestIntegrationProxyVPNLiveAdapter(t *testing.T) {
	app := newTestApp()
	seedIntegrationProxyUsers(t, app)
	var sawListKey, sawDisconnect bool
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-API-Key") != "secret" {
			http.Error(w, "missing key", http.StatusUnauthorized)
			return
		}
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/vpn/clients":
			sawListKey = true
			_, _ = w.Write([]byte(`{"clients":[{"commonName":"alice","realAddress":"1.1.1.1"},{"commonName":"alice","realAddress":"1.1.1.1"}],"total":2}`))
		case r.Method == http.MethodDelete && r.URL.Path == "/api/v1/vpn/clients/alice":
			sawDisconnect = true
			w.WriteHeader(http.StatusOK)
		default:
			t.Fatalf("unexpected VPN API request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer upstream.Close()
	app.Config.VPNAPIURLs = []string{upstream.URL}
	app.Config.VPNAPIKey = "secret"

	clients := responseMap(t, requestJSON(t, app, http.MethodGet, "/api/v1/admin/vpn/clients", "", adminHeaders("ADMIN"), http.StatusOK))
	if !sawListKey || clients["total"] != float64(1) {
		t.Fatalf("live clients = %#v sawListKey=%v, want one deduplicated live client", clients, sawListKey)
	}
	requestJSON(t, app, http.MethodDelete, "/api/v1/admin/vpn/clients/alice", "", adminHeaders("ADMIN"), http.StatusOK)
	if !sawDisconnect {
		t.Fatal("live disconnect endpoint was not called")
	}
}

func seedIntegrationProxyUsers(t *testing.T, platformApp *platform.App) {
	t.Helper()
	createRows(t, platformApp, "identity-service:users", []map[string]any{
		{"id": "U1", "username": "alice", "capabilities": map[string]any{"adminPanel": false}},
		{"id": "ADMIN", "username": "admin", "capabilities": map[string]any{"adminPanel": true}},
	})
}

func serveRaw(t *testing.T, app http.Handler, method, path string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(method, path, nil)
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	app.ServeHTTP(rec, req)
	return rec
}
