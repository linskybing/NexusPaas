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
	acceleratorProfileAdminKey = "accelerator-profile-admin-key"
	acceleratorProfileUserKey  = "accelerator-profile-user-key"
)

func TestSeedDefaultAcceleratorProfilesIdempotent(t *testing.T) {
	app := platform.NewApp(platform.Config{ServiceName: serviceName, HTTPAddr: ":0"})

	if err := seedDefaultAcceleratorProfiles(app); err != nil {
		t.Fatalf("seed default accelerator profiles: %v", err)
	}
	first := app.Store.List(context.Background(), acceleratorProfilesResource)
	if len(first) != 2 {
		t.Fatalf("seeded profiles = %d, want 2", len(first))
	}
	versions := map[string]int{}
	for _, record := range first {
		versions[record.ID] = record.Version
		assertSeededAcceleratorProfile(t, record.Data)
	}

	if err := seedDefaultAcceleratorProfiles(app); err != nil {
		t.Fatalf("seed default accelerator profiles again: %v", err)
	}
	second := app.Store.List(context.Background(), acceleratorProfilesResource)
	if len(second) != 2 {
		t.Fatalf("seeded profiles after second seed = %d, want 2", len(second))
	}
	for _, record := range second {
		if record.Version != versions[record.ID] {
			t.Fatalf("%s version = %d, want unchanged %d", record.ID, record.Version, versions[record.ID])
		}
	}
}

func TestAcceleratorProfileCRUDRejectsInvalidFields(t *testing.T) {
	tests := []struct {
		name    string
		payload map[string]any
	}{
		{name: "missing name", payload: map[string]any{"id": "missing-name"}},
		{name: "bad enabled", payload: map[string]any{"id": "bad-enabled", "name": "Bad enabled", "enabled": "true"}},
		{name: "bad selector", payload: map[string]any{"id": "bad-selector", "name": "Bad selector", "node_selector": map[string]any{"nexuspaas.io/gpu": true}}},
		{name: "bad sm", payload: map[string]any{"id": "bad-sm", "name": "Bad SM", "default_mps_sm_percentage": 101}},
		{name: "bad pinned", payload: map[string]any{"id": "bad-pinned", "name": "Bad pinned", "default_pinned_memory_limit": "not-a-quantity"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := newAcceleratorProfileCRUDTestApp(t)

			status := serveAcceleratorProfileRequest(t, app, http.MethodPost, "/api/v1/accelerator-profiles", tt.payload, acceleratorProfileAdminKey)

			if status != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400", status)
			}
		})
	}
}

func TestAcceleratorProfileAdminGuardEnforced(t *testing.T) {
	app := newAcceleratorProfileCRUDTestApp(t)

	status := serveAcceleratorProfileRequest(t, app, http.MethodGet, "/api/v1/accelerator-profiles", nil, acceleratorProfileUserKey)

	if status != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", status)
	}
}

func TestAcceleratorProfileCreatePublishesChangedEvent(t *testing.T) {
	app := newAcceleratorProfileCRUDTestApp(t)

	status := serveAcceleratorProfileRequest(t, app, http.MethodPost, "/api/v1/accelerator-profiles", map[string]any{
		"id":                        "event-accelerator-profile",
		"name":                      "Event accelerator profile",
		"allowed_device_class_name": defaultDeviceClassName,
		"node_selector":             map[string]any{"nexuspaas.io/gpu": "true"},
		"labels":                    map[string]any{"nexuspaas.io/accelerator-profile": "event-accelerator-profile"},
	}, acceleratorProfileAdminKey)

	if status != http.StatusCreated {
		t.Fatalf("status = %d, want 201", status)
	}
	for _, event := range app.Events.Outbox() {
		if event.Name != "AcceleratorProfileChanged" {
			continue
		}
		if event.Data["accelerator_profile_id"] != "event-accelerator-profile" || event.Data["action"] != "created" {
			t.Fatalf("event data = %#v, want created event-accelerator-profile", event.Data)
		}
		if event.Data["allowed_device_class_name"] != defaultDeviceClassName {
			t.Fatalf("event data = %#v, want device class", event.Data)
		}
		return
	}
	t.Fatalf("events = %#v, want AcceleratorProfileChanged", app.Events.Outbox())
}

func newAcceleratorProfileCRUDTestApp(t *testing.T) *platform.App {
	t.Helper()
	app := platform.NewApp(platform.Config{
		ServiceName: serviceName,
		HTTPAddr:    ":0",
		RequireAuth: true,
		APIKeys: map[string]bool{
			acceleratorProfileAdminKey: true,
			acceleratorProfileUserKey:  true,
		},
		APIKeyPrincipals: map[string]platform.APIKeyPrincipal{
			acceleratorProfileAdminKey: {ID: "accelerator-profile-admin", Username: "admin", Admin: true},
			acceleratorProfileUserKey:  {ID: "accelerator-profile-user", Username: "user"},
		},
	})
	app.RegisterService(Spec())
	Register(app)
	return app
}

func serveAcceleratorProfileRequest(t *testing.T, app *platform.App, method, target string, payload any, apiKey string) int {
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

func assertSeededAcceleratorProfile(t *testing.T, data map[string]any) {
	t.Helper()
	for _, field := range []string{"id", "name", "allowed_device_class_name"} {
		value, ok := data[field].(string)
		if !ok || strings.TrimSpace(value) == "" {
			t.Fatalf("seeded profile %v missing %s", data, field)
		}
	}
	if _, ok := data["enabled"].(bool); !ok {
		t.Fatalf("seeded profile %v missing enabled boolean", data)
	}
	if _, ok := data["node_selector"].(map[string]any); !ok {
		t.Fatalf("seeded profile %v missing node_selector object", data)
	}
	if _, ok := data["labels"].(map[string]any); !ok {
		t.Fatalf("seeded profile %v missing labels object", data)
	}
}
