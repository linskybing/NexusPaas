package imageregistry

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
	imageAccelerationProfileAdminKey = "image-acceleration-profile-admin-key"
	imageAccelerationProfileUserKey  = "image-acceleration-profile-user-key"
)

func TestSeedDefaultImageAccelerationProfilesIdempotent(t *testing.T) {
	app := platform.NewApp(platform.Config{ServiceName: serviceName, HTTPAddr: ":0"})

	if err := seedDefaultImageAccelerationProfiles(app); err != nil {
		t.Fatalf("seed default image acceleration profiles: %v", err)
	}
	first := app.Store.List(context.Background(), imageAccelerationProfilesResource)
	if len(first) != 3 {
		t.Fatalf("seeded profiles = %d, want 3", len(first))
	}
	versions := map[string]int{}
	for _, record := range first {
		versions[record.ID] = record.Version
		assertSeededImageAccelerationProfile(t, record.Data)
	}

	if err := seedDefaultImageAccelerationProfiles(app); err != nil {
		t.Fatalf("seed default image acceleration profiles again: %v", err)
	}
	second := app.Store.List(context.Background(), imageAccelerationProfilesResource)
	if len(second) != 3 {
		t.Fatalf("seeded profiles after second seed = %d, want 3", len(second))
	}
	for _, record := range second {
		if record.Version != versions[record.ID] {
			t.Fatalf("%s version = %d, want unchanged %d", record.ID, record.Version, versions[record.ID])
		}
	}
}

func TestImageAccelerationProfileCRUDRejectsMissingRequiredFields(t *testing.T) {
	for _, missing := range []string{"name", "snapshotter", "prewarm_policy"} {
		t.Run(missing, func(t *testing.T) {
			app := newImageAccelerationProfileCRUDTestApp(t)
			payload := map[string]string{
				"id":             "profile-missing-" + missing,
				"name":           "GPU prewarm",
				"snapshotter":    "stargz",
				"prewarm_policy": "nodepool-based",
			}
			delete(payload, missing)

			status := serveImageAccelerationProfileRequest(t, app, http.MethodPost, "/api/v1/image-acceleration-profiles", payload, imageAccelerationProfileAdminKey)

			if status != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400", status)
			}
			if _, found := app.Store.Get(context.Background(), imageAccelerationProfilesResource, "profile-missing-"+missing); found {
				t.Fatalf("record for missing %s was created", missing)
			}
		})
	}
}

func TestImageAccelerationProfileUpdateRejectsMissingRequiredFields(t *testing.T) {
	app := newImageAccelerationProfileCRUDTestApp(t)

	status := serveImageAccelerationProfileRequest(t, app, http.MethodPut, "/api/v1/image-acceleration-profiles/profile-missing-name", map[string]string{
		"id":             "profile-missing-name",
		"snapshotter":    "stargz",
		"prewarm_policy": "nodepool-based",
	}, imageAccelerationProfileAdminKey)

	if status != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", status)
	}
	if _, found := app.Store.Get(context.Background(), imageAccelerationProfilesResource, "profile-missing-name"); found {
		t.Fatal("record with missing name was created")
	}
}

func TestImageAccelerationProfileAdminGuardEnforced(t *testing.T) {
	app := newImageAccelerationProfileCRUDTestApp(t)

	status := serveImageAccelerationProfileRequest(t, app, http.MethodGet, "/api/v1/image-acceleration-profiles", nil, imageAccelerationProfileUserKey)

	if status != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", status)
	}
}

func TestImageAccelerationProfileCreatePublishesChangedEvent(t *testing.T) {
	app := newImageAccelerationProfileCRUDTestApp(t)

	status := serveImageAccelerationProfileRequest(t, app, http.MethodPost, "/api/v1/image-acceleration-profiles", map[string]any{
		"id":                  "event-image-acceleration-profile",
		"name":                "Event image acceleration profile",
		"snapshotter":         "stargz",
		"prewarm_policy":      "nodepool-based",
		"conversion_required": true,
	}, imageAccelerationProfileAdminKey)

	if status != http.StatusCreated {
		t.Fatalf("status = %d, want 201", status)
	}
	for _, event := range app.Events.Outbox() {
		if event.Name != "ImageAccelerationProfileChanged" {
			continue
		}
		if event.Data["image_acceleration_profile_id"] != "event-image-acceleration-profile" || event.Data["action"] != "created" {
			t.Fatalf("event data = %#v, want created event-image-acceleration-profile", event.Data)
		}
		return
	}
	t.Fatalf("events = %#v, want ImageAccelerationProfileChanged", app.Events.Outbox())
}

func newImageAccelerationProfileCRUDTestApp(t *testing.T) *platform.App {
	t.Helper()
	app := platform.NewApp(platform.Config{
		ServiceName: serviceName,
		HTTPAddr:    ":0",
		RequireAuth: true,
		APIKeys: map[string]bool{
			imageAccelerationProfileAdminKey: true,
			imageAccelerationProfileUserKey:  true,
		},
		APIKeyPrincipals: map[string]platform.APIKeyPrincipal{
			imageAccelerationProfileAdminKey: {ID: "image-acceleration-profile-admin", Username: "admin", Admin: true},
			imageAccelerationProfileUserKey:  {ID: "image-acceleration-profile-user", Username: "user"},
		},
	})
	app.RegisterService(Spec())
	Register(app)
	return app
}

func serveImageAccelerationProfileRequest(t *testing.T, app *platform.App, method, target string, payload any, apiKey string) int {
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

func assertSeededImageAccelerationProfile(t *testing.T, data map[string]any) {
	t.Helper()
	for _, field := range []string{"id", "name", "snapshotter", "prewarm_policy"} {
		value, ok := data[field].(string)
		if !ok || strings.TrimSpace(value) == "" {
			t.Fatalf("seeded profile %v missing %s", data, field)
		}
	}
	if _, ok := data["conversion_required"].(bool); !ok {
		t.Fatalf("seeded profile %v missing conversion_required boolean", data)
	}
	if _, ok := data["enabled"].(bool); !ok {
		t.Fatalf("seeded profile %v missing enabled boolean", data)
	}
	if _, ok := data["allowed_for_projects"].([]any); !ok {
		t.Fatalf("seeded profile %v missing allowed_for_projects array", data)
	}
}
