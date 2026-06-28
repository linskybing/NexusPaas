package storage

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
	storageProfileAdminKey = "storage-profile-admin-key"
	storageProfileUserKey  = "storage-profile-user-key"
)

func TestSeedDefaultStorageProfilesIdempotent(t *testing.T) {
	app := platform.NewApp(platform.Config{ServiceName: serviceName, HTTPAddr: ":0"})

	if err := seedDefaultStorageProfiles(app); err != nil {
		t.Fatalf("seed default storage profiles: %v", err)
	}
	first := app.Store.List(context.Background(), storageProfilesResource)
	if len(first) != 4 {
		t.Fatalf("seeded profiles = %d, want 4", len(first))
	}
	versions := map[string]int{}
	for _, record := range first {
		versions[record.ID] = record.Version
		assertSeededStorageProfile(t, record.Data)
		if record.ID != "minio-artifact" && record.Data["storage_class_name"] != record.ID {
			t.Fatalf("%s storage_class_name = %v, want %s", record.ID, record.Data["storage_class_name"], record.ID)
		}
	}

	if err := seedDefaultStorageProfiles(app); err != nil {
		t.Fatalf("seed default storage profiles again: %v", err)
	}
	second := app.Store.List(context.Background(), storageProfilesResource)
	if len(second) != 4 {
		t.Fatalf("seeded profiles after second seed = %d, want 4", len(second))
	}
	for _, record := range second {
		if record.Version != versions[record.ID] {
			t.Fatalf("%s version = %d, want unchanged %d", record.ID, record.Version, versions[record.ID])
		}
	}
}

func TestStorageProfileCRUDRejectsMissingRequiredFields(t *testing.T) {
	required := []string{"name", "provider", "tier", "access_mode"}
	for _, missing := range required {
		t.Run(missing, func(t *testing.T) {
			app := newStorageProfileCRUDTestApp(t)
			payload := map[string]string{
				"id":          "profile-missing-" + missing,
				"name":        "Burst scratch",
				"provider":    "local-nvme",
				"tier":        "scratch",
				"access_mode": "rwo",
			}
			delete(payload, missing)

			status := serveStorageProfileRequest(t, app, http.MethodPost, "/api/v1/storage-profiles", payload, storageProfileAdminKey)

			if status != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400", status)
			}
			assertStorageRecordMissing(t, app, storageProfilesResource, "profile-missing-"+missing)
		})
	}
}

func TestStorageProfileUpdateRejectsMissingRequiredFields(t *testing.T) {
	app := newStorageProfileCRUDTestApp(t)

	status := serveStorageProfileRequest(t, app, http.MethodPut, "/api/v1/storage-profiles/profile-missing-name", map[string]string{
		"id":          "profile-missing-name",
		"provider":    "local-nvme",
		"tier":        "scratch",
		"access_mode": "rwo",
	}, storageProfileAdminKey)

	if status != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", status)
	}
	assertStorageRecordMissing(t, app, storageProfilesResource, "profile-missing-name")
}

func TestStorageProfileAdminGuardEnforced(t *testing.T) {
	app := newStorageProfileCRUDTestApp(t)

	status := serveStorageProfileRequest(t, app, http.MethodGet, "/api/v1/storage-profiles", nil, storageProfileUserKey)

	if status != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", status)
	}
}

func TestStorageProfileCreatePublishesChangedEvent(t *testing.T) {
	app := newStorageProfileCRUDTestApp(t)

	status := serveStorageProfileRequest(t, app, http.MethodPost, "/api/v1/storage-profiles", map[string]string{
		"id":                 "event-profile",
		"name":               "Event profile",
		"provider":           "local-nvme",
		"tier":               "scratch",
		"access_mode":        "rwo",
		"storage_class_name": "local-nvme-scratch",
	}, storageProfileAdminKey)

	if status != http.StatusCreated {
		t.Fatalf("status = %d, want 201", status)
	}
	events := app.Events.Outbox()
	for _, event := range events {
		if event.Name != "StorageProfileChanged" {
			continue
		}
		if event.Data["profile_id"] != "event-profile" || event.Data["action"] != "created" {
			t.Fatalf("event data = %#v, want created event-profile", event.Data)
		}
		return
	}
	t.Fatalf("events = %#v, want StorageProfileChanged", events)
}

func newStorageProfileCRUDTestApp(t *testing.T) *platform.App {
	t.Helper()
	app := platform.NewApp(platform.Config{
		ServiceName: "storage-service",
		HTTPAddr:    ":0",
		RequireAuth: true,
		APIKeys: map[string]bool{
			storageProfileAdminKey: true,
			storageProfileUserKey:  true,
		},
		APIKeyPrincipals: map[string]platform.APIKeyPrincipal{
			storageProfileAdminKey: {ID: "storage-profile-admin", Username: "admin", Admin: true},
			storageProfileUserKey:  {ID: "storage-profile-user", Username: "user"},
		},
	})
	app.RegisterService(Spec())
	Register(app)
	return app
}

func serveStorageProfileRequest(t *testing.T, app *platform.App, method, target string, payload any, apiKey string) int {
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

func assertSeededStorageProfile(t *testing.T, data map[string]any) {
	t.Helper()
	for _, field := range []string{"id", "name", "provider", "tier", "access_mode"} {
		value, ok := data[field].(string)
		if !ok || strings.TrimSpace(value) == "" {
			t.Fatalf("seeded profile %v missing %s", data, field)
		}
	}
	for _, field := range []string{"performance_class", "mount_mode", "topology_policy"} {
		value, ok := data[field].(string)
		if !ok || strings.TrimSpace(value) == "" {
			t.Fatalf("seeded profile %v missing %s", data, field)
		}
	}
	if _, ok := data["mount_options"].([]any); !ok {
		t.Fatalf("seeded profile %v missing mount_options array", data)
	}
	if _, ok := data["node_selector"].(map[string]any); !ok {
		t.Fatalf("seeded profile %v missing node_selector object", data)
	}
	if _, ok := data["allow_cross_namespace"].(bool); !ok {
		t.Fatalf("seeded profile %v missing allow_cross_namespace boolean", data)
	}
	if _, ok := data["allowed_project_scopes"].([]any); !ok {
		t.Fatalf("seeded profile %v missing allowed_project_scopes array", data)
	}
}
