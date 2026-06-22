package clusterread

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/contracts"
	"github.com/linskybing/nexuspaas/backend/internal/platform"
)

func TestRegisterUsesEventFedClusterReadModels(t *testing.T) {
	app := platform.NewApp(platform.Config{ServiceName: serviceName})
	Register(app)

	if err := app.ValidateServiceIsolation(); err != nil {
		t.Fatalf("cluster read should use local event-fed read models, got isolation error: %v", err)
	}
}

func TestClusterReadUsesProjectedIdentityAndProjectAccessWhenIsolated(t *testing.T) {
	app := platform.NewApp(platform.Config{ServiceName: serviceName})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects/gpu-usage/by-user", nil)
	if hasAdminPanel(app, req, "ADMIN") {
		t.Fatal("hasAdminPanel allowed admin before projected identity facts")
	}
	publishClusterReadTestEvent(t, app, "UserCreated", "identity-service", map[string]any{"id": "ADMIN", "role_id": "platform-admin"})
	publishClusterReadTestEvent(t, app, "roleCreated", "identity-service", map[string]any{"id": "platform-admin", "name": "platform-admin", "capabilities": map[string]any{"adminPanel": true}})
	if !hasAdminPanel(app, req, "ADMIN") {
		t.Fatal("hasAdminPanel denied projected identity role")
	}
	publishClusterReadTestEvent(t, app, "ProjectCreated", "org-project-service", map[string]any{"id": "P1", "project_name": "vision"})
	publishClusterReadTestEvent(t, app, "project_memberCreated", "org-project-service", map[string]any{"project_id": "P1", "user_id": "U1"})
	publishClusterReadTestEvent(t, app, "GroupMembershipChanged", "org-project-service", map[string]any{"user_id": "U1", "group_id": "G1", "role": "member", "action": "create"})
	publishClusterReadTestEvent(t, app, "ProjectCreated", "org-project-service", map[string]any{"id": "P2", "owner_id": "G1"})

	visible := visibleProjectIDs(app, req, "U1")
	if _, ok := visible["P1"]; !ok {
		t.Fatalf("visible projects = %#v, want direct membership P1", visible)
	}
	if _, ok := visible["P2"]; !ok {
		t.Fatalf("visible projects = %#v, want group-owned P2", visible)
	}
	if !projectExists(app, req, "P1") {
		t.Fatal("projectExists denied projected project")
	}
	if got := len(app.Store.List(context.Background(), identityUsersResource)); got != 0 {
		t.Fatalf("source identity users = %d, want isolated cluster read to avoid owner store", got)
	}
	if got := len(app.Store.List(context.Background(), orgProjectsResource)); got != 0 {
		t.Fatalf("source org projects = %d, want isolated cluster read to avoid owner store", got)
	}
}

func TestStaticAdminRoleHeaderRequiresPlatformAuth(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects/P1/gpu-usage", nil)
	req.Header.Set("X-User-ID", "ops-admin")
	req.Header.Set("X-User-Role", "admin")

	unauthenticated := platform.NewApp(platform.Config{ServiceName: serviceName})
	if hasAdminPanel(unauthenticated, req, "ops-admin") {
		t.Fatal("hasAdminPanel trusted admin role header when RequireAuth=false")
	}

	authenticated := platform.NewApp(platform.Config{ServiceName: serviceName, RequireAuth: true})
	if !hasAdminPanel(authenticated, req, "ops-admin") {
		t.Fatal("hasAdminPanel denied platform-authenticated static admin role")
	}

	req.Header.Set("X-User-Role", "super-admin")
	if hasAdminPanel(authenticated, req, "ops-admin") {
		t.Fatal("hasAdminPanel accepted unplanned super-admin role header")
	}
}

func TestClusterProjectionUpdatesDeletesProxyRolesAndMergesCoHostedSource(t *testing.T) {
	app := platform.NewApp(platform.Config{ServiceName: "all"})
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	if _, err := app.Store.Create(context.Background(), identityUsersResource, map[string]any{"id": "source-admin", "capabilities": map[string]any{"adminPanel": true}}); err != nil {
		t.Fatal(err)
	}
	if !hasAdminPanel(app, req, "source-admin") {
		t.Fatal("hasAdminPanel should merge co-hosted source users")
	}

	projectClusterReadEvent(app, req, contracts.Event{
		EventID:       platform.NewUUID(),
		Name:          "UserCreated",
		Source:        "identity-service",
		OccurredAt:    time.Now().UTC(),
		TraceID:       platform.NewUUID(),
		SchemaVersion: 1,
		Data:          map[string]any{"id": "local-admin", "capabilities": map[string]any{"adminPanel": false}},
	})
	projectClusterReadEvent(app, req, contracts.Event{
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
	projectClusterReadEvent(app, req, contracts.Event{
		EventID:       platform.NewUUID(),
		Name:          "UserDeleted",
		Source:        "identity-service",
		OccurredAt:    time.Now().UTC(),
		TraceID:       platform.NewUUID(),
		SchemaVersion: 1,
		Data:          map[string]any{"id": "local-admin", "deleted": true},
	})
	if _, ok := app.Store.Get(context.Background(), clusterIdentityUsersResource, "local-admin"); ok {
		t.Fatal("projected cluster user was not deleted")
	}

	projectClusterReadEvent(app, req, contracts.Event{
		EventID:       platform.NewUUID(),
		Name:          "ProxyPolicyChanged",
		Source:        "authorization-policy-service",
		OccurredAt:    time.Now().UTC(),
		TraceID:       platform.NewUUID(),
		SchemaVersion: 1,
		Data:          map[string]any{"action": "role_create", "id": "ops-role", "capabilities": map[string]any{"adminPanel": true}},
	})
	projectClusterReadEvent(app, req, contracts.Event{
		EventID:       platform.NewUUID(),
		Name:          "ProxyPolicyChanged",
		Source:        "authorization-policy-service",
		OccurredAt:    time.Now().UTC(),
		TraceID:       platform.NewUUID(),
		SchemaVersion: 1,
		Data:          map[string]any{"action": "role_user_assign", "role_id": "ops-role", "user_id": "ops-admin"},
	})
	if !hasAdminPanel(app, req, "ops-admin") {
		t.Fatal("hasAdminPanel denied projected proxy role assignment")
	}
	projectClusterReadEvent(app, req, contracts.Event{
		EventID:       platform.NewUUID(),
		Name:          "ProxyPolicyChanged",
		Source:        "authorization-policy-service",
		OccurredAt:    time.Now().UTC(),
		TraceID:       platform.NewUUID(),
		SchemaVersion: 1,
		Data:          map[string]any{"action": "role_user_unassign", "role_id": "ops-role", "user_id": "ops-admin"},
	})
	if _, ok := app.Store.Get(context.Background(), clusterPolicyRoleAssignments, "ops-role:ops-admin"); ok {
		t.Fatal("projected proxy role assignment was not deleted")
	}
	projectClusterReadEvent(app, req, contracts.Event{Name: "Unrelated", Data: map[string]any{"id": "noop"}})
	deleteClusterReadModel(app, req, clusterProjectsResource, map[string]any{"id": "source-project", "deleted": false})
	syncClusterReadModels(nil, req)
}

func publishClusterReadTestEvent(t *testing.T, app *platform.App, name, source string, data map[string]any) {
	t.Helper()
	if err := app.Events.Publish(context.Background(), contracts.Event{
		EventID:       platform.NewUUID(),
		Name:          name,
		Source:        source,
		OccurredAt:    time.Now().UTC(),
		TraceID:       platform.NewUUID(),
		SchemaVersion: 1,
		Data:          data,
	}); err != nil {
		t.Fatal(err)
	}
}
