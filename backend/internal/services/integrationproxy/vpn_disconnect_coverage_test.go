package integrationproxy

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/contracts"
	"github.com/linskybing/nexuspaas/backend/internal/platform"
)

func TestDisconnectFromVPNAPIOK(t *testing.T) {
	found, err, gotKey, gotPath := callDisconnectFromVPNAPI(t, http.StatusOK)
	if !found || err != nil {
		t.Fatalf("found=%v err=%v, want found without error", found, err)
	}
	assertDisconnectRequest(t, gotKey, gotPath)
}

func TestDisconnectFromVPNAPINotFound(t *testing.T) {
	found, err, gotKey, gotPath := callDisconnectFromVPNAPI(t, http.StatusNotFound)
	if found || err != nil {
		t.Fatalf("found=%v err=%v, want not found without error", found, err)
	}
	assertDisconnectRequest(t, gotKey, gotPath)
}

func TestDisconnectFromVPNAPIGatewayError(t *testing.T) {
	found, err, gotKey, gotPath := callDisconnectFromVPNAPI(t, http.StatusServiceUnavailable)
	if found || err == nil || !strings.Contains(err.Error(), "downstream unavailable") {
		t.Fatalf("found=%v err=%v, want gateway error body", found, err)
	}
	assertDisconnectRequest(t, gotKey, gotPath)
}

func callDisconnectFromVPNAPI(t *testing.T, status int) (bool, error, string, string) {
	t.Helper()
	var gotKey, gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotKey = r.Header.Get("X-API-Key")
		gotPath = r.URL.EscapedPath()
		w.WriteHeader(status)
		_, _ = w.Write([]byte("downstream unavailable"))
	}))
	defer server.Close()

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/vpn/clients/alice", nil)
	found, err := disconnectFromVPNAPI(platform.Config{
		VPNAPIKey:     "vpn-key",
		VPNAPITimeout: time.Second,
	}, req, server.URL+"/", "alice/team")
	return found, err, gotKey, gotPath
}

func assertDisconnectRequest(t *testing.T, gotKey, gotPath string) {
	t.Helper()
	if gotKey != "vpn-key" {
		t.Fatalf("api key header = %q, want vpn-key", gotKey)
	}
	if gotPath != "/api/v1/vpn/clients/alice%2Fteam" {
		t.Fatalf("path = %q, want escaped common name", gotPath)
	}
}

func TestDisconnectLiveVPNClientHandlesConfiguredGateways(t *testing.T) {
	t.Run("no vpn APIs configured", func(t *testing.T) {
		app := platform.NewApp(platform.Config{})
		handled, err := disconnectLiveVPNClient(app, httptest.NewRequest(http.MethodDelete, "/", nil), "alice")
		if handled || err != nil {
			t.Fatalf("handled=%v err=%v, want unhandled nil", handled, err)
		}
	})

	t.Run("all gateways report not found", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
		defer server.Close()
		app := platform.NewApp(platform.Config{VPNAPIURLs: []string{server.URL}})
		handled, err := disconnectLiveVPNClient(app, httptest.NewRequest(http.MethodDelete, "/", nil), "alice")
		if !handled || err == nil || !strings.Contains(err.Error(), "not connected") {
			t.Fatalf("handled=%v err=%v, want handled not connected error", handled, err)
		}
	})

	t.Run("one gateway succeeds after one failure", func(t *testing.T) {
		failing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusBadGateway)
		}))
		defer failing.Close()
		success := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		defer success.Close()
		app := platform.NewApp(platform.Config{VPNAPIURLs: []string{failing.URL, success.URL}})
		handled, err := disconnectLiveVPNClient(app, httptest.NewRequest(http.MethodDelete, "/", nil), "alice")
		if !handled || err != nil {
			t.Fatalf("handled=%v err=%v, want handled success", handled, err)
		}
	})
}

func TestPgAdminHelpersRejectAndWarmBranches(t *testing.T) {
	if err := validatePgAdminLoginResponse(http.StatusInternalServerError, []byte("boom")); err == nil || !strings.Contains(err.Error(), "500") {
		t.Fatalf("err = %v, want HTTP rejection", err)
	}
	if err := validatePgAdminLoginResponse(http.StatusOK, []byte(`{"success":0,"errormsg":"bad login"}`)); err == nil || !strings.Contains(err.Error(), "bad login") {
		t.Fatalf("err = %v, want pgadmin login failure", err)
	}
	if err := validatePgAdminLoginResponse(http.StatusOK, []byte(`not-json`)); err != nil {
		t.Fatalf("err = %v, want malformed success body tolerated", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := r.Cookie("pga"); err != nil {
			t.Fatalf("warm request missing pga cookie: %v", err)
		}
		http.SetCookie(w, &http.Cookie{Name: "pga-warm", Value: "ok", Path: "/"})
		_, _ = w.Write([]byte("ok"))
	}))
	defer server.Close()
	base, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("User-Agent", "test-browser")
	cookies := warmPgAdminSession(req, server.Client(), base, "/browser/", []*http.Cookie{{Name: "pga", Value: "one", Path: "/"}})
	if len(cookies) != 2 || cookies[1].Name != "pga-warm" {
		t.Fatalf("cookies = %#v, want original plus warm cookie", cookies)
	}
	if got := len(warmPgAdminSession(req, server.Client(), base, "http://%", cookies)); got != 2 {
		t.Fatalf("cookies after invalid warm URL = %d, want unchanged", got)
	}
}

func TestIdentityReadModelIDAndDirectRoleBranches(t *testing.T) {
	if !directRoleGrantsAdminPanel(map[string]any{"user_id": "U1", "adminPanel": "true"}, "U1") {
		t.Fatal("direct role grant was not recognized")
	}
	if identityAdminReadModelID(proxyAdminUsersResource, map[string]any{"user_id": "U1"}) != "U1" {
		t.Fatal("user read-model ID should fall back to user_id")
	}
	if identityAdminReadModelID(proxyAdminRolesResource, map[string]any{"name": "platform-admin"}) != "platform-admin" {
		t.Fatal("role read-model ID should fall back to name")
	}
	if identityAdminReadModelID("custom", map[string]any{"Name": "custom-id"}) != "custom-id" {
		t.Fatal("default read-model ID should fall back to name")
	}
	if recordID(contracts.Record[map[string]any]{Data: map[string]any{"ID": "from-data"}}) != "from-data" {
		t.Fatal("recordID should fall back to record data")
	}
	if got := recordID(contracts.Record[map[string]any]{ID: "from-record", Data: map[string]any{"id": "from-data"}}); got != "from-record" {
		t.Fatalf("recordID = %q, want record ID first", got)
	}
}

func TestIdentityReadModelUpsertAndDeleteBranches(t *testing.T) {
	app := platform.NewApp(platform.Config{ServiceName: serviceName})
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	if err := upsertIdentityAdminReadModel(app, req, proxyAdminUsersResource, map[string]any{}); err != nil {
		t.Fatalf("empty identity upsert err = %v, want nil", err)
	}
	if err := upsertIdentityAdminReadModel(app, req, proxyAdminUsersResource, map[string]any{"id": "U1", "adminPanel": false}); err != nil {
		t.Fatal(err)
	}
	if err := upsertIdentityAdminReadModel(app, req, proxyAdminUsersResource, map[string]any{"id": "U1", "adminPanel": true}); err != nil {
		t.Fatal(err)
	}
	record, ok := app.Store.Get(context.Background(), proxyAdminUsersResource, "U1")
	if !ok || record.Data["adminPanel"] != true {
		t.Fatalf("record = %#v, want updated adminPanel", record)
	}
	deleteIdentityAdminReadModel(app, req, proxyAdminUsersResource, map[string]any{"id": "U1", "deleted": false})
	if _, ok := app.Store.Get(context.Background(), proxyAdminUsersResource, "U1"); !ok {
		t.Fatal("delete with deleted=false removed record")
	}
	deleteIdentityAdminReadModel(app, req, proxyAdminUsersResource, map[string]any{"id": "U1"})
	if _, ok := app.Store.Get(context.Background(), proxyAdminUsersResource, "U1"); ok {
		t.Fatal("delete without deleted=false did not remove record")
	}
}

func TestIdentityProjectionAndRequireAdminEdges(t *testing.T) {
	app := platform.NewApp(platform.Config{ServiceName: serviceName})
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	if err := projectIdentityAdminEvent(app, req, contracts.Event{Name: "RoleDeleted", Data: map[string]any{"id": "missing"}}); err != nil {
		t.Fatalf("role delete projection err = %v", err)
	}
	if _, _, _, ok := identityAdminProjection(contracts.Event{Name: "unrelated"}); ok {
		t.Fatal("unrelated identity event should not project")
	}
	if identitySourceCoHosted(nil, identityUsersResource) {
		t.Fatal("nil app should not report co-hosted identity source")
	}
	if _, _, ok := requireAdmin(nil, req); ok {
		t.Fatal("requireAdmin should reject request without user")
	}
	req.Header.Set("X-User-ID", "U1")
	if status, _, ok := requireAdmin(app, req); ok || status != http.StatusForbidden {
		t.Fatalf("requireAdmin = (%d,%v), want forbidden without admin grant", status, ok)
	}
}

func TestDisconnectFromVPNAPIInvalidInputs(t *testing.T) {
	req := httptest.NewRequest(http.MethodDelete, "/", nil)
	if _, err := disconnectFromVPNAPI(platform.Config{}, req, "http://%zz", "alice"); err == nil {
		t.Fatal("invalid URL was accepted")
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	req = httptest.NewRequest(http.MethodDelete, "/", nil).WithContext(cancelled)
	if _, err := disconnectFromVPNAPI(platform.Config{VPNAPITimeout: time.Second}, req, "http://127.0.0.1:1", "alice"); err == nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context canceled wrapped", err)
	}
}
