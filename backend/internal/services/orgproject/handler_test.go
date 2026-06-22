package orgproject

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
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

func TestMembershipRoleUpdateHelpersFailWhenTxUpdateMisses(t *testing.T) {
	app := platform.NewApp(platform.Config{ServiceName: serviceName})
	req := httptest.NewRequest(http.MethodPut, "/", nil)

	existingMembership := orgProjectMembershipRecord{
		ID: membershipID("U1", "G1"),
		Data: map[string]any{
			"id":       membershipID("U1", "G1"),
			"user_id":  "U1",
			"group_id": "G1",
			"role":     "user",
		},
	}
	if _, err := updateExistingMembership(app, req, existingMembership, "U1", "G1", "admin"); err == nil {
		t.Fatal("updateExistingMembership error = nil, want tx miss error")
	}

	existingProjectMember := platformRecord{
		ID: "P1/U1",
		Data: map[string]any{
			"id":         "P1/U1",
			"project_id": "P1",
			"user_id":    "U1",
			"role":       "user",
		},
	}
	if _, err := updateDirectProjectMemberRoleWithEvent(app, req, existingProjectMember, "P1", "U1", "admin"); err == nil {
		t.Fatal("updateDirectProjectMemberRoleWithEvent error = nil, want tx miss error")
	}
	if got := len(app.Events.Outbox()); got != 0 {
		t.Fatalf("tx miss emitted %d events, want 0", got)
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

func TestStaticAdminRoleHeaderRequiresPlatformAuth(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/groups", nil)
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
}

func TestSpoofedAdminRoleHeaderCannotCreateGroupThroughServeHTTP(t *testing.T) {
	app := newStaticAdminOrgProjectHTTPApp()

	noKey := serveOrgProjectGroupCreate(app, map[string]string{"X-User-Role": "admin"}, "spoof-no-key")
	if noKey.Code != http.StatusUnauthorized {
		t.Fatalf("spoofed admin header without API key status = %d, want 401: %s", noKey.Code, noKey.Body.String())
	}

	reader := serveOrgProjectGroupCreate(app, map[string]string{"X-API-Key": "reader-key", "X-User-Role": "admin"}, "spoof-reader")
	if reader.Code != http.StatusForbidden {
		t.Fatalf("spoofed admin header with reader key status = %d, want 403: %s", reader.Code, reader.Body.String())
	}

	if _, found := groupGPURepository(app).FindGroup(context.Background(), "spoof-no-key"); found {
		t.Fatal("spoofed no-key request created group")
	}
	if _, found := groupGPURepository(app).FindGroup(context.Background(), "spoof-reader"); found {
		t.Fatal("spoofed reader request created group")
	}
}

func TestStaticAdminAPIKeyPrincipalCanCreateGroupThroughServeHTTP(t *testing.T) {
	app := newStaticAdminOrgProjectHTTPApp()

	rec := serveOrgProjectGroupCreate(app, map[string]string{"X-API-Key": "admin-key"}, "static-admin")
	if rec.Code != http.StatusCreated {
		t.Fatalf("static admin group create status = %d, want 201: %s", rec.Code, rec.Body.String())
	}
	if _, found := groupGPURepository(app).FindGroup(context.Background(), "static-admin"); !found {
		t.Fatal("static admin request did not create group")
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

func newStaticAdminOrgProjectHTTPApp() *platform.App {
	app := platform.NewApp(platform.Config{
		ServiceName:  serviceName,
		HTTPAddr:     ":0",
		RequireAuth:  true,
		APIKeys:      map[string]bool{"admin-key": true, "reader-key": true},
		ExternalURLs: map[string]string{},
		APIKeyPrincipals: map[string]platform.APIKeyPrincipal{
			"admin-key":  {ID: "ops-admin", Username: "ops-admin", Admin: true},
			"reader-key": {ID: "ops-reader", Username: "ops-reader", Role: "user"},
		},
	})
	app.RegisterService(Spec())
	Register(app)
	return app
}

func serveOrgProjectGroupCreate(app *platform.App, headers map[string]string, id string) *httptest.ResponseRecorder {
	body := `{"id":"` + id + `","group_name":"` + id + `","name":"` + id + `"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/groups", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	return rec
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
