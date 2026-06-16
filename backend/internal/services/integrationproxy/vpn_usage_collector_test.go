package integrationproxy

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/platform"
)

func vpnFetcher(clients []Client, err error) vpnClientFetcher {
	return func(context.Context) ([]Client, error) { return clients, err }
}

func openVPNUsageRows(t *testing.T, app *platform.App) []map[string]any {
	t.Helper()
	out := []map[string]any{}
	for _, record := range app.Store.List(context.Background(), vpnUsageResource) {
		if !isDisconnected(record.Data) {
			out = append(out, record.Data)
		}
	}
	return out
}

func findVPNUsageRow(t *testing.T, app *platform.App, username string) map[string]any {
	t.Helper()
	for _, record := range app.Store.List(context.Background(), vpnUsageResource) {
		if textValue(record.Data, "username") == username {
			return record.Data
		}
	}
	t.Fatalf("no vpn usage row for %q", username)
	return nil
}

// AC1: a snapshot writes one open session per connected client, and /admin/vpn/usage
// aggregates the collected rows.
func TestVPNUsageCollectorSnapshotsActiveSessionsAndAggregates(t *testing.T) {
	app := newIntegrationProxyTestApp(t)
	now := time.Date(2026, time.June, 16, 12, 0, 0, 0, time.UTC)
	clients := []Client{
		{CommonName: "alice", ConnectedSince: now.Add(-time.Hour).Format(time.RFC3339), BytesReceived: 100, BytesSent: 200, Node: "pod-a"},
		{CommonName: "bob", ConnectedSince: now.Add(-2 * time.Hour).Format(time.RFC3339), BytesReceived: 50, BytesSent: 70, Node: "pod-b"},
	}
	if err := collectVPNUsage(context.Background(), app.Store, vpnFetcher(clients, nil), time.Minute, now); err != nil {
		t.Fatalf("collectVPNUsage: %v", err)
	}
	if rows := openVPNUsageRows(t, app); len(rows) != 2 {
		t.Fatalf("open rows = %d, want 2", len(rows))
	}

	req := proxyRequest(http.MethodGet, "/api/v1/admin/vpn/usage?since=2000-01-01", "ADMIN")
	code, data, _ := getUsage(app, req, platform.RouteSpec{})
	assertProxyStatus(t, code, data, http.StatusOK)
	usage := data.(usageResponse)
	if usage.RowCount != 2 || usage.Summary.TotalBytes != 420 {
		t.Fatalf("usage = %#v, want 2 rows totaling 420 bytes", usage)
	}
}

// AC2: re-observing a session updates byte counters in place (no duplicates); a
// session dropped past the grace window is closed and excluded from the active set.
func TestVPNUsageCollectorUpdatesInPlaceAndClosesStale(t *testing.T) {
	app := newIntegrationProxyTestApp(t)
	t0 := time.Date(2026, time.June, 16, 12, 0, 0, 0, time.UTC)
	aliceCS := t0.Add(-time.Hour).Format(time.RFC3339)
	first := []Client{
		{CommonName: "alice", ConnectedSince: aliceCS, BytesReceived: 100, BytesSent: 200},
		{CommonName: "bob", ConnectedSince: t0.Add(-time.Hour).Format(time.RFC3339), BytesReceived: 50, BytesSent: 70},
	}
	if err := collectVPNUsage(context.Background(), app.Store, vpnFetcher(first, nil), time.Minute, t0); err != nil {
		t.Fatalf("first collect: %v", err)
	}

	t1 := t0.Add(2 * time.Minute) // beyond the 1m grace
	second := []Client{{CommonName: "alice", ConnectedSince: aliceCS, BytesReceived: 500, BytesSent: 600}}
	if err := collectVPNUsage(context.Background(), app.Store, vpnFetcher(second, nil), time.Minute, t1); err != nil {
		t.Fatalf("second collect: %v", err)
	}

	if all := app.Store.List(context.Background(), vpnUsageResource); len(all) != 2 {
		t.Fatalf("total rows = %d, want 2 (no duplicate session rows)", len(all))
	}
	open := openVPNUsageRows(t, app)
	if len(open) != 1 || open[0]["username"] != "alice" {
		t.Fatalf("open rows = %#v, want only alice", open)
	}
	if int64Value(open[0], "uploadBytes") != 500 || int64Value(open[0], "downloadBytes") != 600 {
		t.Fatalf("alice row = %#v, want updated bytes 500/600", open[0])
	}
	bob := findVPNUsageRow(t, app, "bob")
	if !isDisconnected(bob) {
		t.Fatalf("bob row = %#v, want closed with disconnectedAt", bob)
	}
}

// AC2 (grace): a session dropped within the grace window stays open so a single
// missed scrape does not prematurely end an active session.
func TestVPNUsageCollectorKeepsRecentlyDroppedSessionOpen(t *testing.T) {
	app := newIntegrationProxyTestApp(t)
	t0 := time.Date(2026, time.June, 16, 12, 0, 0, 0, time.UTC)
	first := []Client{
		{CommonName: "alice", ConnectedSince: t0.Add(-time.Hour).Format(time.RFC3339), BytesReceived: 1, BytesSent: 2},
		{CommonName: "bob", ConnectedSince: t0.Add(-time.Hour).Format(time.RFC3339), BytesReceived: 3, BytesSent: 4},
	}
	if err := collectVPNUsage(context.Background(), app.Store, vpnFetcher(first, nil), time.Minute, t0); err != nil {
		t.Fatalf("first collect: %v", err)
	}
	t1 := t0.Add(30 * time.Second) // within the 1m grace
	if err := collectVPNUsage(context.Background(), app.Store, vpnFetcher(first[:1], nil), time.Minute, t1); err != nil {
		t.Fatalf("second collect: %v", err)
	}
	if open := openVPNUsageRows(t, app); len(open) != 2 {
		t.Fatalf("open rows = %d, want 2 (bob still within grace)", len(open))
	}
}

// AC3: with no VPN gateway configured the run writes nothing and leaves history intact.
func TestVPNUsageCollectorNoGatewayConfiguredIsNoop(t *testing.T) {
	app := newIntegrationProxyTestApp(t)
	now := time.Date(2026, time.June, 16, 12, 0, 0, 0, time.UTC)
	createProxyRecords(t, app, vpnUsageResource, []map[string]any{
		{"id": "u1", "username": "alice", "connectedSince": now.Add(-time.Hour).Format(time.RFC3339), "uploadBytes": int64(10), "downloadBytes": int64(20), "lastSeenAt": now.Add(-time.Hour).Format(time.RFC3339)},
	})
	if err := collectVPNUsage(context.Background(), app.Store, vpnFetcher(nil, nil), time.Minute, now); err != nil {
		t.Fatalf("collectVPNUsage: %v", err)
	}
	rows := app.Store.List(context.Background(), vpnUsageResource)
	if len(rows) != 1 || isDisconnected(rows[0].Data) {
		t.Fatalf("rows = %#v, want the single seeded session untouched and open", rows)
	}
}

// A fetch error must not close existing sessions (avoids false closure on a transient
// gateway outage).
func TestVPNUsageCollectorFetchErrorDoesNotCloseSessions(t *testing.T) {
	app := newIntegrationProxyTestApp(t)
	now := time.Date(2026, time.June, 16, 12, 0, 0, 0, time.UTC)
	createProxyRecords(t, app, vpnUsageResource, []map[string]any{
		{"id": "u1", "username": "alice", "connectedSince": now.Add(-time.Hour).Format(time.RFC3339), "lastSeenAt": now.Add(-time.Hour).Format(time.RFC3339)},
	})
	if err := collectVPNUsage(context.Background(), app.Store, vpnFetcher(nil, errors.New("all vpn APIs unreachable")), time.Minute, now); err != nil {
		t.Fatalf("collectVPNUsage: %v", err)
	}
	if open := openVPNUsageRows(t, app); len(open) != 1 {
		t.Fatalf("open rows = %d, want the session left open on fetch error", len(open))
	}
}

// AC4: the collector registers only where integration-proxy-service is hosted.
func TestVPNUsageCollectorRegistersOnlyForIntegrationProxyService(t *testing.T) {
	owner := platform.NewApp(platform.Config{ServiceName: serviceName, HTTPAddr: ":0"})
	Register(owner)
	if !containsTask(owner.MaintenanceTaskNames(), vpnUsageCollectorTask) {
		t.Fatalf("owner tasks = %v, want %q registered", owner.MaintenanceTaskNames(), vpnUsageCollectorTask)
	}

	other := platform.NewApp(platform.Config{ServiceName: "identity-service", HTTPAddr: ":0"})
	Register(other)
	if containsTask(other.MaintenanceTaskNames(), vpnUsageCollectorTask) {
		t.Fatalf("non-owner tasks = %v, want %q absent", other.MaintenanceTaskNames(), vpnUsageCollectorTask)
	}
}

// End-to-end through the configured-gateway fetcher and the lease-gated maintenance
// run, exercising the real HTTP path and the VPNUsageEnabled gate.
func TestVPNUsageCollectorViaConfiguredGatewayMaintenanceRun(t *testing.T) {
	now := time.Now().UTC()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/vpn/clients" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_ = json.NewEncoder(w).Encode(ClientsResponse{
			Clients: []Client{{CommonName: "alice", ConnectedSince: now.Add(-time.Hour).Format(time.RFC3339), BytesReceived: 10, BytesSent: 20}},
			Total:   1,
		})
	}))
	defer srv.Close()
	app := newIntegrationProxyTestApp(t)
	app.Config.VPNAPIURLs = []string{srv.URL}
	app.Config.VPNUsageEnabled = true
	app.Config.VPNUsageGrace = time.Minute
	app.RunMaintenanceOnce(context.Background(), time.Second)

	if open := openVPNUsageRows(t, app); len(open) != 1 || open[0]["username"] != "alice" {
		t.Fatalf("open rows = %#v, want one collected alice session", open)
	}
}

func TestVPNUsageCollectorDisabledIsNoop(t *testing.T) {
	app := newIntegrationProxyTestApp(t)
	app.Config.VPNUsageEnabled = false
	app.RunMaintenanceOnce(context.Background(), time.Second)
	if rows := app.Store.List(context.Background(), vpnUsageResource); len(rows) != 0 {
		t.Fatalf("rows = %d, want none written when collector disabled", len(rows))
	}
}

func TestConfiguredVPNFetcherNoGatewayReturnsNil(t *testing.T) {
	clients, err := configuredVPNFetcher(context.Background(), platform.Config{})
	if err != nil || clients != nil {
		t.Fatalf("configuredVPNFetcher = (%#v, %v), want (nil, nil) when unconfigured", clients, err)
	}
}

func TestConfiguredVPNFetcherAllGatewaysUnreachableErrors(t *testing.T) {
	cfg := platform.Config{VPNAPIURLs: []string{"http://127.0.0.1:0"}, VPNAPITimeout: time.Second}
	if _, err := configuredVPNFetcher(context.Background(), cfg); err == nil {
		t.Fatal("configuredVPNFetcher err = nil, want error when every gateway is unreachable")
	}
}

func TestStoredSessionKeyEmptyWithoutUsername(t *testing.T) {
	if key := storedSessionKey(map[string]any{"connectedSince": "2026-06-16T00:00:00Z"}); key != "" {
		t.Fatalf("storedSessionKey = %q, want empty when username missing", key)
	}
}

func containsTask(names []string, want string) bool {
	for _, name := range names {
		if name == want {
			return true
		}
	}
	return false
}
