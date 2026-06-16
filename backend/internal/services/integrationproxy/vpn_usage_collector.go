package integrationproxy

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/platform"
)

const vpnUsageCollectorTask = "vpn-usage-collector"

// vpnClientFetcher lists the currently-connected VPN clients across every
// configured gateway. A (nil, nil) result signals "no gateway configured" so the
// collector can degrade to a safe no-op instead of mistaking an absent source for
// "every session ended".
type vpnClientFetcher func(ctx context.Context) ([]Client, error)

// registerVPNUsageCollector wires the VPN usage snapshot loop as a lease-gated,
// owner-gated maintenance task. It is the microservice port of reference
// cron.StartVPNUsageCollector + application/vpn.Service.CollectUsage: the cron loop
// snapshotted active sessions and closed stale ones; here the same behavior runs as
// integration-proxy-service's maintenance task, persisting durable
// vpn_usage_sessions rows that the existing /api/v1/admin/vpn/usage report already
// aggregates. RegisterMaintenanceTaskForService owner-gates registration, so the
// task only runs where integration-proxy-service is hosted.
func registerVPNUsageCollector(app *platform.App) {
	app.RegisterMaintenanceTaskForService(serviceName, vpnUsageCollectorTask, func(ctx context.Context) error {
		if !app.Config.VPNUsageEnabled {
			return nil
		}
		fetch := func(ctx context.Context) ([]Client, error) {
			return configuredVPNFetcher(ctx, app.Config)
		}
		return collectVPNUsage(ctx, app.Store, fetch, app.Config.VPNUsageGrace, time.Now().UTC())
	})
}

// configuredVPNFetcher fetches connected clients from the configured VPN gateway
// URLs. It returns (nil, nil) when no gateway is configured (degraded no-op).
func configuredVPNFetcher(ctx context.Context, cfg platform.Config) ([]Client, error) {
	baseURLs := vpnAPIURLs(cfg)
	if len(baseURLs) == 0 {
		return nil, nil
	}
	perPod := make([][]Client, 0, len(baseURLs))
	failures := []string{}
	for _, baseURL := range baseURLs {
		clients, err := fetchVPNClients(ctx, cfg, baseURL)
		if err != nil {
			failures = append(failures, err.Error())
			continue
		}
		perPod = append(perPod, clients)
	}
	if len(perPod) == 0 && len(failures) > 0 {
		return nil, fmt.Errorf("all vpn APIs unreachable: %s", strings.Join(failures, "; "))
	}
	// mergeClients always returns a non-nil slice, so a reachable gateway with zero
	// connected clients is distinguishable from "no gateway configured".
	return mergeClients(perPod), nil
}

// collectVPNUsage snapshots active sessions and closes stale ones. It never mutates
// fetched records in place: each persisted change is a fresh field map handed to the
// copy-on-write store (immutability rule). A fetch error or unconfigured gateway is a
// no-op so a transient absence never overwrites the durable usage history.
func collectVPNUsage(ctx context.Context, store platform.RecordStore, fetch vpnClientFetcher, grace time.Duration, now time.Time) error {
	if store == nil || fetch == nil {
		return nil
	}
	collectCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	clients, err := fetch(collectCtx)
	if err != nil {
		slog.Warn("vpn usage collection failed", "error", err)
		return nil
	}
	if clients == nil {
		slog.Warn("vpn usage collector disabled: no vpn gateway configured")
		return nil
	}

	observed := upsertActiveSessions(collectCtx, store, clients, now)
	closeStaleSessions(collectCtx, store, observed, grace, now)
	return nil
}

// upsertActiveSessions records one open session per connected client, updating the
// existing open row in place when the session was already seen. It returns the set
// of session keys observed this tick so stale closure can skip them.
func upsertActiveSessions(ctx context.Context, store platform.RecordStore, clients []Client, now time.Time) map[string]bool {
	openByKey := openSessionsByKey(ctx, store)
	observed := make(map[string]bool, len(clients))
	for _, client := range clients {
		if strings.TrimSpace(client.CommonName) == "" {
			continue
		}
		key := clientSessionKey(client)
		if observed[key] {
			continue
		}
		observed[key] = true
		data := activeSessionData(client, now)
		if id, ok := openByKey[key]; ok {
			store.Update(ctx, vpnUsageResource, id, data)
			continue
		}
		if _, err := store.Create(ctx, vpnUsageResource, data); err != nil {
			slog.Warn("vpn usage session create failed", "username", client.CommonName, "error", err)
		}
	}
	return observed
}

// closeStaleSessions stamps disconnectedAt on open sessions no longer observed once
// they fall outside the grace window, preserving them for historical usage queries.
func closeStaleSessions(ctx context.Context, store platform.RecordStore, observed map[string]bool, grace time.Duration, now time.Time) {
	for _, record := range store.List(ctx, vpnUsageResource) {
		if isDisconnected(record.Data) {
			continue
		}
		key := storedSessionKey(record.Data)
		if key == "" || observed[key] {
			continue
		}
		if last := lastSeenValue(record.Data); last != nil && now.Sub(*last) < grace {
			continue
		}
		store.Update(ctx, vpnUsageResource, record.ID, map[string]any{
			"disconnectedAt": now.Format(time.RFC3339),
		})
	}
}

// openSessionsByKey indexes the currently-open usage rows by stable session key.
func openSessionsByKey(ctx context.Context, store platform.RecordStore) map[string]string {
	out := map[string]string{}
	for _, record := range store.List(ctx, vpnUsageResource) {
		if isDisconnected(record.Data) {
			continue
		}
		if key := storedSessionKey(record.Data); key != "" {
			out[key] = record.ID
		}
	}
	return out
}

// activeSessionData builds the field map for an open session. Field names match what
// aggregateUsage/accumulateUsageRecord already read so /admin/vpn/usage consumes them
// unchanged: BytesReceived is the client upload, BytesSent the download.
func activeSessionData(client Client, now time.Time) map[string]any {
	return map[string]any{
		"username":       client.CommonName,
		"connectedSince": client.ConnectedSince,
		"uploadBytes":    client.BytesReceived,
		"downloadBytes":  client.BytesSent,
		"node":           client.Node,
		"lastSeenAt":     now.Format(time.RFC3339),
	}
}

// clientSessionKey derives a stable identity for a live client; reconnects produce a
// distinct connectedSince and therefore a new session.
func clientSessionKey(client Client) string {
	return strings.TrimSpace(client.CommonName) + "|" + strings.TrimSpace(client.ConnectedSince)
}

// storedSessionKey derives the same identity from a persisted usage row.
func storedSessionKey(data map[string]any) string {
	username := textValue(data, "username", "Username")
	if username == "" {
		return ""
	}
	return username + "|" + textValue(data, "connectedSince", "connected_since", "ConnectedSince")
}

// lastSeenValue returns the most recent observation time for an open session.
func lastSeenValue(data map[string]any) *time.Time {
	return firstTimeValue(data, "lastSeenAt", "last_seen_at", "LastSeenAt", "connectedSince", "connected_since", "ConnectedSince")
}
