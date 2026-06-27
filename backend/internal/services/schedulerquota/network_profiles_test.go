package schedulerquota

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/linskybing/nexuspaas/backend/internal/platform"
)

const (
	networkProfileAdminKey = "network-profile-admin-key"
	networkProfileUserKey  = "network-profile-user-key"
)

func TestSeedDefaultNetworkProfilesIdempotent(t *testing.T) {
	app := platform.NewApp(platform.Config{ServiceName: serviceName, HTTPAddr: ":0"})

	if err := seedDefaultNetworkProfiles(app); err != nil {
		t.Fatalf("seed default network profiles: %v", err)
	}
	first := app.Store.List(context.Background(), networkProfilesResource)
	if len(first) != 2 {
		t.Fatalf("seeded profiles = %d, want 2", len(first))
	}
	versions := map[string]int{}
	for _, record := range first {
		versions[record.ID] = record.Version
		assertSeededNetworkProfile(t, record.Data)
	}

	if err := seedDefaultNetworkProfiles(app); err != nil {
		t.Fatalf("seed default network profiles again: %v", err)
	}
	second := app.Store.List(context.Background(), networkProfilesResource)
	if len(second) != 2 {
		t.Fatalf("seeded profiles after second seed = %d, want 2", len(second))
	}
	for _, record := range second {
		if record.Version != versions[record.ID] {
			t.Fatalf("%s version = %d, want unchanged %d", record.ID, record.Version, versions[record.ID])
		}
	}
}

func TestNetworkProfileCRUDRejectsMissingRequiredFields(t *testing.T) {
	for _, missing := range []string{"name", "primary_cni"} {
		t.Run(missing, func(t *testing.T) {
			app := newNetworkProfileCRUDTestApp(t)
			payload := map[string]string{
				"id":          "profile-missing-" + missing,
				"name":        "Burst RDMA",
				"primary_cni": "cilium",
			}
			delete(payload, missing)

			status := serveNetworkProfileRequest(t, app, http.MethodPost, "/api/v1/network-profiles", payload, networkProfileAdminKey)

			if status != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400", status)
			}
			if _, found := app.Store.Get(context.Background(), networkProfilesResource, "profile-missing-"+missing); found {
				t.Fatalf("record for missing %s was created", missing)
			}
		})
	}
}

func TestNetworkProfileUpdateRejectsMissingRequiredFields(t *testing.T) {
	app := newNetworkProfileCRUDTestApp(t)

	status := serveNetworkProfileRequest(t, app, http.MethodPut, "/api/v1/network-profiles/profile-missing-name", map[string]string{
		"id":          "profile-missing-name",
		"primary_cni": "cilium",
	}, networkProfileAdminKey)

	if status != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", status)
	}
	if _, found := app.Store.Get(context.Background(), networkProfilesResource, "profile-missing-name"); found {
		t.Fatal("record with missing name was created")
	}
}

func TestNetworkProfileAdminGuardEnforced(t *testing.T) {
	app := newNetworkProfileCRUDTestApp(t)

	status := serveNetworkProfileRequest(t, app, http.MethodGet, "/api/v1/network-profiles", nil, networkProfileUserKey)

	if status != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", status)
	}
}

func TestNetworkProfileCreatePublishesChangedEvent(t *testing.T) {
	app := newNetworkProfileCRUDTestApp(t)

	status := serveNetworkProfileRequest(t, app, http.MethodPost, "/api/v1/network-profiles", map[string]any{
		"id":                "event-network-profile",
		"name":              "Event network profile",
		"primary_cni":       "cilium",
		"secondary_network": "nexuspaas-system/rdma-net",
	}, networkProfileAdminKey)

	if status != http.StatusCreated {
		t.Fatalf("status = %d, want 201", status)
	}
	events := app.Events.Outbox()
	for _, event := range events {
		if event.Name != "NetworkProfileChanged" {
			continue
		}
		if event.Data["network_profile_id"] != "event-network-profile" || event.Data["action"] != "created" {
			t.Fatalf("event data = %#v, want created event-network-profile", event.Data)
		}
		return
	}
	t.Fatalf("events = %#v, want NetworkProfileChanged", events)
}

func newNetworkProfileCRUDTestApp(t *testing.T) *platform.App {
	t.Helper()
	app := platform.NewApp(platform.Config{
		ServiceName: serviceName,
		HTTPAddr:    ":0",
		RequireAuth: true,
		APIKeys: map[string]bool{
			networkProfileAdminKey: true,
			networkProfileUserKey:  true,
		},
		APIKeyPrincipals: map[string]platform.APIKeyPrincipal{
			networkProfileAdminKey: {ID: "network-profile-admin", Username: "admin", Admin: true},
			networkProfileUserKey:  {ID: "network-profile-user", Username: "user"},
		},
	})
	app.RegisterService(Spec())
	Register(app)
	return app
}

func serveNetworkProfileRequest(t *testing.T, app *platform.App, method, target string, payload any, apiKey string) int {
	t.Helper()
	body := ""
	if payload != nil {
		raw, err := json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
		body = string(raw)
	}
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if apiKey != "" {
		req.Header.Set("X-API-Key", apiKey)
	}
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	return rec.Code
}

func assertSeededNetworkProfile(t *testing.T, data map[string]any) {
	t.Helper()
	for _, field := range []string{"id", "name", "primary_cni", "secondary_network", "bandwidth_class", "topology_policy"} {
		value, ok := data[field].(string)
		if !ok || strings.TrimSpace(value) == "" {
			t.Fatalf("seeded profile %v missing %s", data, field)
		}
	}
	if _, ok := data["rdma_enabled"].(bool); !ok {
		t.Fatalf("seeded profile %v missing rdma_enabled boolean", data)
	}
	if _, ok := data["enabled"].(bool); !ok {
		t.Fatalf("seeded profile %v missing enabled boolean", data)
	}
	if _, ok := data["annotations"].(map[string]any); !ok {
		t.Fatalf("seeded profile %v missing annotations object", data)
	}
	if _, ok := data["network_env"].(map[string]any); !ok {
		t.Fatalf("seeded profile %v missing network_env object", data)
	}
}
