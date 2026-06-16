package integrationproxy

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/platform"
)

func TestIntegrationProxyVPNHandlers(t *testing.T) {
	app := newIntegrationProxyTestApp(t)
	seedVPNData(t, app)

	code, data, _ := listClients(app, proxyRequest(http.MethodGet, "/api/v1/admin/vpn/clients", "ADMIN"), platform.RouteSpec{})
	assertProxyStatus(t, code, data, http.StatusOK)
	clients := data.(ClientsResponse).Clients
	if len(clients) != 2 || clients[0].CommonName != "alice" || clients[1].CommonName != "bob" {
		t.Fatalf("clients = %#v, want active local clients sorted by common name", clients)
	}

	disconnectReq := proxyRequest(http.MethodDelete, "/api/v1/admin/vpn/clients/alice", "ADMIN")
	disconnectReq.SetPathValue("cn", "alice")
	code, data, _ = disconnectClient(app, disconnectReq, platform.RouteSpec{})
	assertProxyStatus(t, code, data, http.StatusOK)
	if clients := activeVPNClients(app, proxyRequest(http.MethodGet, "/", "ADMIN")); len(clients) != 1 || clients[0].CommonName != "bob" {
		t.Fatalf("clients after disconnect = %#v, want only bob", clients)
	}

	usageReq := proxyRequest(http.MethodGet, "/api/v1/admin/vpn/usage?since=2026-04-01&until=2026-04-30&username=alice", "ADMIN")
	code, data, _ = getUsage(app, usageReq, platform.RouteSpec{})
	assertProxyStatus(t, code, data, http.StatusOK)
	usage := data.(usageResponse)
	if usage.RowCount != 1 || usage.Summary.TotalBytes != 300 || usage.Rows[0].Username != "alice" {
		t.Fatalf("usage = %#v, want filtered alice summary", usage)
	}

	code, data, _ = pgadminAuthCheck(app, proxyRequest(http.MethodGet, "/api/v1/pgadmin-auth-check", "ADMIN"), platform.RouteSpec{})
	assertProxyStatus(t, code, data, http.StatusOK)
}

func TestIntegrationProxySSOHandlersAndCookieHelpers(t *testing.T) {
	minio := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/login" {
			t.Fatalf("minio path = %s, want /api/v1/login", r.URL.Path)
		}
		http.SetCookie(w, &http.Cookie{Name: "minio", Value: "ok", Path: "/"})
		w.WriteHeader(http.StatusNoContent)
	}))
	defer minio.Close()

	app := newIntegrationProxyTestApp(t)
	app.Config.ExternalURLs["minio-console"] = minio.URL
	app.Config.MinIOConsoleAccessKey = "access"
	app.Config.MinIOConsoleSecretKey = "secret"
	code, data, _ := minioSSOLogin(app, proxyRequest(http.MethodGet, "/api/v1/minio-console-sso", "ADMIN"), platform.RouteSpec{})
	assertProxyStatus(t, code, data, http.StatusFound)
	raw := data.(platform.RawResponse)
	if raw.Headers["Location"] != minioProxyPrefix+"/" || !strings.Contains(raw.HeaderValues["Set-Cookie"][0], "Path=/api/v1/minio-console/") {
		t.Fatalf("minio redirect = %#v, want prefixed cookie path", raw)
	}

	pgadmin := newPgAdminTestServer(t)
	defer pgadmin.Close()
	app.Config.ExternalURLs["pgadmin"] = pgadmin.URL
	app.Config.PGAdminDefaultEmail = "admin@example.test"
	app.Config.PGAdminDefaultPassword = "secret"
	code, data, _ = pgadminSSOLogin(app, proxyRequest(http.MethodGet, "/api/v1/pgadmin-sso", "ADMIN"), platform.RouteSpec{})
	assertProxyStatus(t, code, data, http.StatusFound)
	raw = data.(platform.RawResponse)
	if raw.Headers["Location"] != pgadminProxyPrefix+"/browser/" || len(raw.HeaderValues["Set-Cookie"]) == 0 {
		t.Fatalf("pgadmin redirect = %#v, want browser redirect with cookies", raw)
	}

	cookies := mergeCookies([]*http.Cookie{{Name: "a", Value: "1", Path: "/"}}, []*http.Cookie{{Name: "a", Path: "/", MaxAge: -1}})
	if len(cookies) != 0 {
		t.Fatalf("merged cookies = %#v, want deletion to remove existing cookie", cookies)
	}
}

func TestIntegrationProxyHelperBranches(t *testing.T) {
	cfg := platform.Config{VPNAPIURLs: []string{" http://one.test", " http://two.test "}}
	if urls := vpnAPIURLs(cfg); len(urls) != 2 || urls[0] != "http://one.test" {
		t.Fatalf("vpn URLs = %#v, want parsed list", urls)
	}
	clients := mergeClients([][]Client{{{CommonName: "bob", RealAddress: "2"}, {CommonName: "alice", RealAddress: "1"}}, {{CommonName: "alice", RealAddress: "1"}}})
	if len(clients) != 2 || clients[0].CommonName != "alice" {
		t.Fatalf("merged clients = %#v, want deduped sorted clients", clients)
	}
	if _, err := parsePositiveInt("0"); err == nil {
		t.Fatal("parsePositiveInt accepted zero")
	}
	row := map[string]any{
		"t":    "2026-04-01",
		"n":    json.Number("7"),
		"flag": "true",
	}
	if timeValue(row, "t") == nil || int64Value(row, "n") != 7 || !boolValue(row, "flag") {
		t.Fatalf("helper conversions failed for %#v", row)
	}
	if externalURL(nil, "missing") != "" || resolveDate("bad", time.Unix(0, 0)).Unix() != 0 {
		t.Fatal("externalURL/resolveDate fallback failed")
	}
}

func newIntegrationProxyTestApp(t *testing.T) *platform.App {
	t.Helper()
	app := platform.NewApp(platform.Config{ServiceName: "all", HTTPAddr: ":0", ExternalURLs: map[string]string{}})
	Register(app)
	createProxyRecords(t, app, identityUsersResource, []map[string]any{
		{"id": "ADMIN", "username": "admin", "capabilities": map[string]any{"adminPanel": true}},
	})
	return app
}

func seedVPNData(t *testing.T, app *platform.App) {
	t.Helper()
	april := time.Date(2026, time.April, 5, 8, 0, 0, 0, time.UTC)
	createProxyRecords(t, app, vpnClientsResource, []map[string]any{
		{"id": "c1", "commonName": "alice", "realAddress": "10.0.0.1", "virtualAddress": "172.16.0.2", "bytesReceived": 100.0, "bytesSent": 200.0, "connectedSince": april, "node": "pod-a"},
		{"id": "c2", "commonName": "bob", "realAddress": "10.0.0.2", "bytesReceived": int64(50), "bytesSent": 70, "connected_since": april.Format(time.RFC3339), "node": "pod-b"},
		{"id": "c3", "commonName": "carol", "status": "disconnected"},
	})
	createProxyRecords(t, app, vpnUsageResource, []map[string]any{
		{"id": "u1", "username": "alice", "connectedSince": april, "disconnectedAt": april.Add(time.Hour), "uploadBytes": int64(100), "downloadBytes": int64(200)},
		{"id": "u2", "username": "bob", "connectedSince": april, "disconnectedAt": april.Add(time.Hour), "uploadBytes": int64(50), "downloadBytes": int64(70)},
	})
}

func createProxyRecords(t *testing.T, app *platform.App, resource string, rows []map[string]any) {
	t.Helper()
	for _, row := range rows {
		if _, err := app.Store.Create(context.Background(), resource, row); err != nil {
			t.Fatal(err)
		}
	}
}

func proxyRequest(method, target, userID string) *http.Request {
	req := httptest.NewRequest(method, target, nil)
	if userID != "" {
		req.Header.Set("X-User-ID", userID)
	}
	return req
}

func newPgAdminTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case pgadminProxyPrefix + "/login":
			http.SetCookie(w, &http.Cookie{Name: "pga", Value: "one", Path: "/"})
			_, _ = w.Write([]byte(`{"csrfToken": "csrf-1"}`))
		case pgadminProxyPrefix + "/authenticate/login":
			if r.Header.Get("X-pgA-CSRFToken") != "csrf-1" {
				t.Fatalf("csrf header = %q, want csrf-1", r.Header.Get("X-pgA-CSRFToken"))
			}
			http.SetCookie(w, &http.Cookie{Name: "pga", Value: "two", Path: "/"})
			_, _ = w.Write([]byte(`{"success":1}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

func assertProxyStatus(t *testing.T, code int, data any, want int) {
	t.Helper()
	if code != want {
		t.Fatalf("status=%d data=%#v, want %d", code, data, want)
	}
}
