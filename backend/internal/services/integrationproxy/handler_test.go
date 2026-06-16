package integrationproxy

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/contracts"
	"github.com/linskybing/nexuspaas/backend/internal/platform"
)

func TestRegisterUsesEventFedIdentityAdminReadModel(t *testing.T) {
	app := platform.NewApp(platform.Config{ServiceName: serviceName})
	Register(app)

	if err := app.ValidateServiceIsolation(); err != nil {
		t.Fatalf("integration proxy should use local event-fed identity admin read model, got isolation error: %v", err)
	}
}

func TestHasAdminPanelUsesEventFedIdentityReadModelWhenIsolated(t *testing.T) {
	app := platform.NewApp(platform.Config{ServiceName: serviceName})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/vpn/clients", nil)
	if hasAdminPanel(app, req, "ADMIN") {
		t.Fatal("hasAdminPanel allowed admin before projected identity facts")
	}
	publishIdentityAdminTestEvent(t, app, "userCreated", map[string]any{"id": "ADMIN", "username": "admin", "role_id": "platform-admin"})
	if hasAdminPanel(app, req, "ADMIN") {
		t.Fatal("hasAdminPanel allowed admin before projected role grants")
	}
	publishIdentityAdminTestEvent(t, app, "roleCreated", map[string]any{"id": "platform-admin", "name": "platform-admin", "capabilities": map[string]any{"adminPanel": true}})
	if !hasAdminPanel(app, req, "ADMIN") {
		t.Fatal("hasAdminPanel denied projected user role admin grant")
	}
	if got := len(app.Store.List(context.Background(), identityUsersResource)); got != 0 {
		t.Fatalf("source identity users = %d, want isolated integration proxy to avoid owner store", got)
	}
	if got := len(app.Store.List(context.Background(), identityRolesResource)); got != 0 {
		t.Fatalf("source identity roles = %d, want isolated integration proxy to avoid owner store", got)
	}
	if _, ok := app.Store.Get(context.Background(), proxyAdminUsersResource, "ADMIN"); !ok {
		t.Fatal("missing projected admin user")
	}
	if _, ok := app.Store.Get(context.Background(), proxyAdminRolesResource, "platform-admin"); !ok {
		t.Fatal("missing projected admin role")
	}
}

func TestIdentityAdminProjectionUpdatesDeletesAndMergesCoHostedSource(t *testing.T) {
	app := platform.NewApp(platform.Config{ServiceName: "all"})
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	if _, err := app.Store.Create(context.Background(), identityUsersResource, map[string]any{"id": "source-admin", "capabilities": map[string]any{"adminPanel": true}}); err != nil {
		t.Fatal(err)
	}
	if _, err := app.Store.Create(context.Background(), identityRolesResource, map[string]any{"id": "source-role", "user_id": "role-admin", "capabilities": map[string]any{"adminPanel": true}}); err != nil {
		t.Fatal(err)
	}
	if !hasAdminPanel(app, req, "source-admin") {
		t.Fatal("hasAdminPanel should merge co-hosted source users")
	}
	if !hasAdminPanel(app, req, "role-admin") {
		t.Fatal("hasAdminPanel should merge co-hosted source role assignments")
	}

	projectIdentityAdminEvent(app, req, contracts.Event{
		EventID:       platform.NewUUID(),
		Name:          "UserCreated",
		Source:        "identity-service",
		OccurredAt:    time.Now().UTC(),
		TraceID:       platform.NewUUID(),
		SchemaVersion: 1,
		Data:          map[string]any{"id": "local-admin", "capabilities": map[string]any{"adminPanel": false}},
	})
	projectIdentityAdminEvent(app, req, contracts.Event{
		EventID:       platform.NewUUID(),
		Name:          "UserUpdated",
		Source:        "identity-service",
		OccurredAt:    time.Now().UTC(),
		TraceID:       platform.NewUUID(),
		SchemaVersion: 1,
		Data: map[string]any{
			"new": map[string]any{"id": "local-admin", "capabilities": map[string]any{"adminPanel": true}},
		},
	})
	if !hasAdminPanel(app, req, "local-admin") {
		t.Fatal("hasAdminPanel denied updated projected admin grant")
	}
	projectIdentityAdminEvent(app, req, contracts.Event{
		EventID:       platform.NewUUID(),
		Name:          "UserDeleted",
		Source:        "identity-service",
		OccurredAt:    time.Now().UTC(),
		TraceID:       platform.NewUUID(),
		SchemaVersion: 1,
		Data:          map[string]any{"id": "local-admin", "deleted": true},
	})
	if _, ok := app.Store.Get(context.Background(), proxyAdminUsersResource, "local-admin"); ok {
		t.Fatal("projected admin user was not deleted")
	}
	projectIdentityAdminEvent(app, req, contracts.Event{Name: "Unrelated", Data: map[string]any{"id": "noop"}})
	deleteIdentityAdminReadModel(app, req, proxyAdminUsersResource, map[string]any{"id": "source-admin", "deleted": false})
	syncIdentityAdminReadModels(nil, req)
}

func publishIdentityAdminTestEvent(t *testing.T, app *platform.App, name string, data map[string]any) {
	t.Helper()
	if err := app.Events.Publish(context.Background(), contracts.Event{
		EventID:       platform.NewUUID(),
		Name:          name,
		Source:        "identity-service",
		OccurredAt:    time.Now().UTC(),
		TraceID:       platform.NewUUID(),
		SchemaVersion: 1,
		Data:          data,
	}); err != nil {
		t.Fatal(err)
	}
}
