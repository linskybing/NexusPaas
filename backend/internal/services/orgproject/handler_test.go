package orgproject

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/contracts"
	"github.com/linskybing/nexuspaas/backend/internal/platform"
)

func TestRegisterUsesEventFedIdentityReadModel(t *testing.T) {
	app := platform.NewApp(platform.Config{ServiceName: serviceName})
	Register(app)

	if err := app.ValidateServiceIsolation(); err != nil {
		t.Fatalf("org project should use local event-fed identity read model, got isolation error: %v", err)
	}
}

func TestOrgIdentityProjectionSupportsAdminAndUserLookupWhenIsolated(t *testing.T) {
	app := platform.NewApp(platform.Config{ServiceName: serviceName})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/groups", nil)
	publishOrgIdentityEvent(t, app, "UserCreated", map[string]any{"id": "ADMIN", "username": "admin", "role_id": "platform-admin"})
	publishOrgIdentityEvent(t, app, "UserCreated", map[string]any{"id": "U1", "username": "alice", "email": "alice@test.local"})
	publishOrgIdentityEvent(t, app, "roleCreated", map[string]any{"id": "platform-admin", "name": "platform-admin", "capabilities": map[string]any{"adminPanel": true}})

	if !hasAdminPanel(app, req, "ADMIN") {
		t.Fatal("hasAdminPanel denied projected admin role grant")
	}
	user, found := findUser(app, req, "alice")
	if !found || userIDFromMap(user) != "U1" {
		t.Fatalf("findUser projected lookup = %#v, found=%v", user, found)
	}
	if got := len(app.Store.List(context.Background(), usersResource)); got != 0 {
		t.Fatalf("source identity users = %d, want isolated org project to avoid owner store", got)
	}
	if got := len(app.Store.List(context.Background(), rolesResource)); got != 0 {
		t.Fatalf("source identity roles = %d, want isolated org project to avoid owner store", got)
	}
}

func TestOrgIdentityProjectionUpdatesDeletesAndMergesCoHostedSource(t *testing.T) {
	app := platform.NewApp(platform.Config{ServiceName: "all"})
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	if _, err := app.Store.Create(context.Background(), usersResource, map[string]any{"id": "source-admin", "role_id": "source-role"}); err != nil {
		t.Fatal(err)
	}
	if _, err := app.Store.Create(context.Background(), rolesResource, map[string]any{"id": "source-role", "name": "source-role", "capabilities": map[string]any{"adminPanel": true}}); err != nil {
		t.Fatal(err)
	}
	if !hasAdminPanel(app, req, "source-admin") {
		t.Fatal("hasAdminPanel should merge co-hosted identity source rows")
	}

	applyOrgIdentityEvent(app, req, contracts.Event{
		EventID:       platform.NewUUID(),
		Name:          "UserCreated",
		Source:        "identity-service",
		OccurredAt:    time.Now().UTC(),
		TraceID:       platform.NewUUID(),
		SchemaVersion: 1,
		Data:          map[string]any{"id": "local-admin", "role_id": "local-role"},
	})
	applyOrgIdentityEvent(app, req, contracts.Event{
		EventID:       platform.NewUUID(),
		Name:          "roleUpdated",
		Source:        "identity-service",
		OccurredAt:    time.Now().UTC(),
		TraceID:       platform.NewUUID(),
		SchemaVersion: 1,
		Data: map[string]any{
			"new": map[string]any{"id": "local-role", "name": "local-role", "capabilities": map[string]any{"adminPanel": true}},
		},
	})
	if !hasAdminPanel(app, req, "local-admin") {
		t.Fatal("hasAdminPanel denied updated projected identity grant")
	}
	applyOrgIdentityEvent(app, req, contracts.Event{
		EventID:       platform.NewUUID(),
		Name:          "UserDeleted",
		Source:        "identity-service",
		OccurredAt:    time.Now().UTC(),
		TraceID:       platform.NewUUID(),
		SchemaVersion: 1,
		Data:          map[string]any{"id": "local-admin", "deleted": true},
	})
	if _, ok := app.Store.Get(context.Background(), orgIdentityUsers, "local-admin"); ok {
		t.Fatal("projected identity user was not deleted")
	}
	applyOrgIdentityEvent(app, req, contracts.Event{Name: "Unrelated", Data: map[string]any{"id": "noop"}})
	syncOrgIdentity(nil, req)
}

func TestSaveOrgIdentityHandlesMissingIDConflictAndCreateError(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	conflictStore := &orgIdentityFakeStore{createErr: platform.CreateConflictError{Resource: orgIdentityUsers, ID: "U1"}}
	app := platform.NewApp(platform.Config{ServiceName: serviceName}, platform.WithStore(conflictStore))

	saveOrgIdentity(app, req, orgIdentityUsers, map[string]any{})
	if conflictStore.createCalls != 0 || conflictStore.updateCalls != 0 {
		t.Fatalf("missing id should not touch store, create=%d update=%d", conflictStore.createCalls, conflictStore.updateCalls)
	}

	saveOrgIdentity(app, req, orgIdentityUsers, map[string]any{"id": "U1"})
	if conflictStore.createCalls != 1 || conflictStore.updateCalls != 2 {
		t.Fatalf("conflict path calls create=%d update=%d, want 1/2", conflictStore.createCalls, conflictStore.updateCalls)
	}

	failingStore := &orgIdentityFakeStore{createErr: errors.New("store unavailable")}
	failingApp := platform.NewApp(platform.Config{ServiceName: serviceName}, platform.WithStore(failingStore))
	saveOrgIdentity(failingApp, req, orgIdentityUsers, map[string]any{"id": "U2"})
	if failingStore.createCalls != 1 || failingStore.updateCalls != 1 {
		t.Fatalf("create-error path calls create=%d update=%d, want 1/1", failingStore.createCalls, failingStore.updateCalls)
	}
}

func publishOrgIdentityEvent(t *testing.T, app *platform.App, name string, data map[string]any) {
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

type orgIdentityFakeStore struct {
	createErr   error
	createCalls int
	updateCalls int
}

func (s *orgIdentityFakeStore) Create(context.Context, string, map[string]any) (contracts.Record[map[string]any], error) {
	s.createCalls++
	return contracts.Record[map[string]any]{}, s.createErr
}

func (s *orgIdentityFakeStore) Get(context.Context, string, string) (contracts.Record[map[string]any], bool) {
	return contracts.Record[map[string]any]{}, false
}

func (s *orgIdentityFakeStore) List(context.Context, string) []contracts.Record[map[string]any] {
	return nil
}

func (s *orgIdentityFakeStore) Update(_ context.Context, _ string, id string, data map[string]any) (contracts.Record[map[string]any], bool) {
	s.updateCalls++
	if s.updateCalls == 1 {
		return contracts.Record[map[string]any]{}, false
	}
	return contracts.Record[map[string]any]{ID: id, Data: data}, true
}

func (s *orgIdentityFakeStore) Delete(context.Context, string, string) bool {
	return false
}

func (s *orgIdentityFakeStore) NextID(string, string, int, int) string {
	return ""
}
