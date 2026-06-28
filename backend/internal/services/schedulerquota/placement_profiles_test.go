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
	placementProfileAdminKey = "placement-profile-admin-key"
	placementProfileUserKey  = "placement-profile-user-key"
)

func TestSeedDefaultPlacementProfilesIdempotent(t *testing.T) {
	app := platform.NewApp(platform.Config{ServiceName: serviceName, HTTPAddr: ":0"})

	if err := seedDefaultPlacementProfiles(app); err != nil {
		t.Fatalf("seed default placement profiles: %v", err)
	}
	first := app.Store.List(context.Background(), placementProfilesResource)
	if len(first) != 3 {
		t.Fatalf("seeded profiles = %d, want 3", len(first))
	}
	versions := map[string]int{}
	for _, record := range first {
		versions[record.ID] = record.Version
		assertSeededPlacementProfile(t, record.Data)
	}

	if err := seedDefaultPlacementProfiles(app); err != nil {
		t.Fatalf("seed default placement profiles again: %v", err)
	}
	second := app.Store.List(context.Background(), placementProfilesResource)
	if len(second) != 3 {
		t.Fatalf("seeded profiles after second seed = %d, want 3", len(second))
	}
	for _, record := range second {
		if record.Version != versions[record.ID] {
			t.Fatalf("%s version = %d, want unchanged %d", record.ID, record.Version, versions[record.ID])
		}
	}
}

func TestPlacementProfileCRUDRejectsMissingRequiredFields(t *testing.T) {
	for _, missing := range []string{"name", "scheduler_backend"} {
		t.Run(missing, func(t *testing.T) {
			app := newPlacementProfileCRUDTestApp(t)
			payload := map[string]string{
				"id":                "profile-missing-" + missing,
				"name":              "Kueue profile",
				"scheduler_backend": "kueue",
			}
			delete(payload, missing)

			status := servePlacementProfileRequest(t, app, http.MethodPost, "/api/v1/placement-profiles", payload, placementProfileAdminKey)

			if status != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400", status)
			}
			if _, found := app.Store.Get(context.Background(), placementProfilesResource, "profile-missing-"+missing); found {
				t.Fatalf("record for missing %s was created", missing)
			}
		})
	}
}

func TestPlacementProfileAdminGuardEnforced(t *testing.T) {
	app := newPlacementProfileCRUDTestApp(t)

	status := servePlacementProfileRequest(t, app, http.MethodGet, "/api/v1/placement-profiles", nil, placementProfileUserKey)

	if status != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", status)
	}
}

func TestPlacementProfileCreatePublishesChangedEvent(t *testing.T) {
	app := newPlacementProfileCRUDTestApp(t)

	status := servePlacementProfileRequest(t, app, http.MethodPost, "/api/v1/placement-profiles", map[string]any{
		"id":                "event-placement-profile",
		"name":              "Event placement profile",
		"scheduler_backend": "kueue",
	}, placementProfileAdminKey)

	if status != http.StatusCreated {
		t.Fatalf("status = %d, want 201", status)
	}
	for _, event := range app.Events.Outbox() {
		if event.Name != "PlacementProfileChanged" {
			continue
		}
		if event.Data["placement_profile_id"] != "event-placement-profile" || event.Data["action"] != "created" {
			t.Fatalf("event data = %#v, want created event-placement-profile", event.Data)
		}
		return
	}
	t.Fatalf("events = %#v, want PlacementProfileChanged", app.Events.Outbox())
}

func newPlacementProfileCRUDTestApp(t *testing.T) *platform.App {
	t.Helper()
	app := platform.NewApp(platform.Config{
		ServiceName: serviceName,
		HTTPAddr:    ":0",
		RequireAuth: true,
		APIKeys: map[string]bool{
			placementProfileAdminKey: true,
			placementProfileUserKey:  true,
		},
		APIKeyPrincipals: map[string]platform.APIKeyPrincipal{
			placementProfileAdminKey: {ID: "placement-profile-admin", Username: "admin", Admin: true},
			placementProfileUserKey:  {ID: "placement-profile-user", Username: "user"},
		},
	})
	app.RegisterService(Spec())
	Register(app)
	return app
}

func servePlacementProfileRequest(t *testing.T, app *platform.App, method, target string, payload any, apiKey string) int {
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

func assertSeededPlacementProfile(t *testing.T, data map[string]any) {
	t.Helper()
	for _, field := range []string{"id", "name", "scheduler_backend", "scheduler_name"} {
		value, ok := data[field].(string)
		if !ok || strings.TrimSpace(value) == "" {
			t.Fatalf("seeded profile %v missing %s", data, field)
		}
	}
	if _, ok := data["enabled"].(bool); !ok {
		t.Fatalf("seeded profile %v missing enabled boolean", data)
	}
	if _, ok := data["gang_enabled"].(bool); !ok {
		t.Fatalf("seeded profile %v missing gang_enabled boolean", data)
	}
	if _, ok := data["labels"].(map[string]any); !ok {
		t.Fatalf("seeded profile %v missing labels object", data)
	}
	if _, ok := data["annotations"].(map[string]any); !ok {
		t.Fatalf("seeded profile %v missing annotations object", data)
	}
}
