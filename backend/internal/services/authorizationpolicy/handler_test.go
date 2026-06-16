package authorizationpolicy

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
		t.Fatalf("authorization policy should use local event-fed identity read model, got isolation error: %v", err)
	}
}

func TestHasAdminPanelUsesProjectedIdentityWhenIsolated(t *testing.T) {
	app := platform.NewApp(platform.Config{ServiceName: serviceName})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/proxy-rbac/services", nil)

	if hasAdminPanel(app, req, "ADMIN") {
		t.Fatal("hasAdminPanel allowed admin before projected identity facts")
	}
	publishPolicyIdentityTestEvent(t, app, "UserCreated", map[string]any{"id": "ADMIN", "username": "admin", "role_id": "platform-admin"})
	if hasAdminPanel(app, req, "ADMIN") {
		t.Fatal("hasAdminPanel allowed admin before projected role grant")
	}
	publishPolicyIdentityTestEvent(t, app, "roleCreated", map[string]any{"id": "platform-admin", "name": "platform-admin", "capabilities": map[string]any{"adminPanel": true}})
	if !hasAdminPanel(app, req, "ADMIN") {
		t.Fatal("hasAdminPanel denied projected admin role grant")
	}
	if got := len(app.Store.List(context.Background(), usersResource)); got != 0 {
		t.Fatalf("source identity users = %d, want isolated authorization policy to avoid owner store", got)
	}
	if got := len(app.Store.List(context.Background(), rolesResource)); got != 0 {
		t.Fatalf("source identity roles = %d, want isolated authorization policy to avoid owner store", got)
	}
}

func TestPolicyIdentityProjectionUpdatesDeletesAndMergesCoHostedSource(t *testing.T) {
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

	projectPolicyIdentityEvent(app, req, contracts.Event{
		EventID:       platform.NewUUID(),
		Name:          "UserCreated",
		Source:        "identity-service",
		OccurredAt:    time.Now().UTC(),
		TraceID:       platform.NewUUID(),
		SchemaVersion: 1,
		Data:          map[string]any{"id": "local-admin", "role_id": "local-role"},
	})
	projectPolicyIdentityEvent(app, req, contracts.Event{
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
		t.Fatal("hasAdminPanel denied updated projected role grant")
	}
	projectPolicyIdentityEvent(app, req, contracts.Event{
		EventID:       platform.NewUUID(),
		Name:          "UserDeleted",
		Source:        "identity-service",
		OccurredAt:    time.Now().UTC(),
		TraceID:       platform.NewUUID(),
		SchemaVersion: 1,
		Data:          map[string]any{"id": "local-admin", "deleted": true},
	})
	if _, ok := app.Store.Get(context.Background(), policyIdentityUsers, "local-admin"); ok {
		t.Fatal("projected identity user was not deleted")
	}
	projectPolicyIdentityEvent(app, req, contracts.Event{Name: "Unrelated", Data: map[string]any{"id": "noop"}})
	deletePolicyIdentityReadModel(authorizationPolicyProjectionRepo(app), req, policyIdentityUsers, map[string]any{"id": "source-admin", "deleted": false})
	syncPolicyIdentityReadModels(nil, req)
}

func TestPolicyIdentityUpsertHandlesMissingIDConflictAndCreateError(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	conflictStore := &policyIdentityFakeStore{createErr: platform.CreateConflictError{Resource: policyIdentityRoles, ID: "role-1"}}
	app := platform.NewApp(platform.Config{ServiceName: serviceName}, platform.WithStore(conflictStore))

	upsertPolicyIdentityReadModel(authorizationPolicyProjectionRepo(app), req, policyIdentityRoles, map[string]any{})
	if conflictStore.createCalls != 0 || conflictStore.updateCalls != 0 {
		t.Fatalf("missing id should not touch store, create=%d update=%d", conflictStore.createCalls, conflictStore.updateCalls)
	}

	upsertPolicyIdentityReadModel(authorizationPolicyProjectionRepo(app), req, policyIdentityRoles, map[string]any{"id": "role-1", "name": "role-1"})
	if conflictStore.createCalls != 1 || conflictStore.updateCalls != 2 {
		t.Fatalf("conflict path calls create=%d update=%d, want 1/2", conflictStore.createCalls, conflictStore.updateCalls)
	}

	failingStore := &policyIdentityFakeStore{createErr: errors.New("store unavailable")}
	failingApp := platform.NewApp(platform.Config{ServiceName: serviceName}, platform.WithStore(failingStore))
	upsertPolicyIdentityReadModel(authorizationPolicyProjectionRepo(failingApp), req, policyIdentityRoles, map[string]any{"id": "role-2"})
	if failingStore.createCalls != 1 || failingStore.updateCalls != 1 {
		t.Fatalf("create-error path calls create=%d update=%d, want 1/1", failingStore.createCalls, failingStore.updateCalls)
	}
}

func publishPolicyIdentityTestEvent(t *testing.T, app *platform.App, name string, data map[string]any) {
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

type policyIdentityFakeStore struct {
	createErr   error
	createCalls int
	updateCalls int
}

func (s *policyIdentityFakeStore) Create(context.Context, string, map[string]any) (contracts.Record[map[string]any], error) {
	s.createCalls++
	return contracts.Record[map[string]any]{}, s.createErr
}

func (s *policyIdentityFakeStore) Get(context.Context, string, string) (contracts.Record[map[string]any], bool) {
	return contracts.Record[map[string]any]{}, false
}

func (s *policyIdentityFakeStore) List(context.Context, string) []contracts.Record[map[string]any] {
	return nil
}

func (s *policyIdentityFakeStore) Update(_ context.Context, resource, id string, data map[string]any) (contracts.Record[map[string]any], bool) {
	s.updateCalls++
	if s.updateCalls == 1 {
		return contracts.Record[map[string]any]{}, false
	}
	_ = resource
	return contracts.Record[map[string]any]{ID: id, Data: data}, true
}

func (s *policyIdentityFakeStore) Delete(context.Context, string, string) bool {
	return false
}

func (s *policyIdentityFakeStore) NextID(string, string, int, int) string {
	return ""
}

func TestRawPolicyPDPEnforcesStoredPolicy(t *testing.T) {
	store := platform.NewStore()
	repo := rawPermissionRepoFromStore(store)
	pdp := RawPolicyPDP{Policies: repo}
	subject, domain, object, action := "alice", "project-1", "model", "read"

	denied, err := pdp.Enforce(context.Background(), subject, domain, object, action)
	if err != nil {
		t.Fatal(err)
	}
	if denied.Allowed {
		t.Fatalf("empty policy store allowed request: %#v", denied)
	}

	policy := []string{subject, domain, object, action}
	if created, err := repo.CreateRawPermissionPolicy(context.Background(), policy); err != nil || !created {
		t.Fatalf("CreateRawPermissionPolicy created=%v err=%v, want created", created, err)
	}
	allowed, err := pdp.Enforce(context.Background(), subject, domain, object, action)
	if err != nil {
		t.Fatal(err)
	}
	if !allowed.Allowed {
		t.Fatalf("stored policy denied request: %#v", allowed)
	}
}
