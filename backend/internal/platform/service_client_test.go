package platform

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/linskybing/nexuspaas/backend/internal/contracts"
)

// startOwnerService returns a running owner App that holds one user record and
// exposes identity-owned read contracts authenticated with serviceKey.
func startOwnerService(t *testing.T, serviceKey string) (*App, string) {
	t.Helper()
	owner := NewApp(Config{
		ServiceName: "identity-service",
		RequireAuth: false,
		ServiceTrustedIdentities: map[string]ServiceTrustedIdentity{
			"consumer-service": {Key: serviceKey, Audiences: []string{"identity-service"}},
		},
	})
	if _, err := owner.Store.Create(context.Background(), "identity-service:users", map[string]any{"id": "US1", "username": "alice"}); err != nil {
		t.Fatalf("seed owner: %v", err)
	}
	owner.Mux.HandleFunc("GET /internal/identity/users", func(w http.ResponseWriter, r *http.Request) {
		if !owner.AuthorizeServiceRequestForAudience(w, r, "identity-service") {
			return
		}
		WriteJSON(w, r, http.StatusOK, owner.Store.List(r.Context(), "identity-service:users"))
	})
	owner.Mux.HandleFunc("GET /internal/identity/users/{id}", func(w http.ResponseWriter, r *http.Request) {
		if !owner.AuthorizeServiceRequestForAudience(w, r, "identity-service") {
			return
		}
		record, ok := owner.Store.Get(r.Context(), "identity-service:users", r.PathValue("id"))
		if !ok {
			WriteJSON(w, r, http.StatusNotFound, nil)
			return
		}
		WriteJSON(w, r, http.StatusOK, record)
	})
	server := httptest.NewServer(owner)
	t.Cleanup(server.Close)
	return owner, server.URL
}

func TestRemoteServiceReaderListAndGet(t *testing.T) {
	serviceKey := testServiceKey(t)
	_, ownerURL := startOwnerService(t, serviceKey)
	reader := NewRemoteServiceReader(Config{
		ServiceURLs:         map[string]string{"identity-service": ownerURL},
		ServiceIdentityName: "consumer-service",
		ServiceIdentityKey:  serviceKey,
	})
	ctx := context.Background()

	list, err := reader.List(ctx, "identity-service:users")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 1 || list[0].Data["username"] != "alice" {
		t.Fatalf("list = %#v", list)
	}

	rec, ok, err := reader.Get(ctx, "identity-service:users", "US1")
	if err != nil || !ok || rec.Data["username"] != "alice" {
		t.Fatalf("get = %#v ok=%v err=%v", rec, ok, err)
	}

	_, ok, err = reader.Get(ctx, "identity-service:users", "missing")
	if err != nil || ok {
		t.Fatalf("get missing ok=%v err=%v, want false/nil", ok, err)
	}
}

func TestRemoteServiceReaderRejectsBadKey(t *testing.T) {
	serviceKey := testServiceKey(t)
	_, ownerURL := startOwnerService(t, serviceKey)
	reader := NewRemoteServiceReader(Config{
		ServiceURLs:         map[string]string{"identity-service": ownerURL},
		ServiceIdentityName: "consumer-service",
		ServiceIdentityKey:  serviceKey + "-wrong",
	})
	if _, err := reader.List(context.Background(), "identity-service:users"); err == nil {
		t.Fatal("expected unauthorized error with wrong service key")
	}
}

func TestRemoteServiceReaderStrictRuntimeDoesNotSendLegacyServiceKey(t *testing.T) {
	var gotName, gotKey string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotName = r.Header.Get(serviceNameHeader)
		gotKey = r.Header.Get(serviceKeyHeader)
		WriteError(w, r, http.StatusUnauthorized, "unauthorized", "service authentication is required")
	}))
	t.Cleanup(server.Close)

	reader := NewRemoteServiceReader(Config{
		EnvironmentProfile: runtimeProfileStaging,
		ServiceURLs:        map[string]string{"identity-service": server.URL},
		ServiceAPIKey:      "legacy-key",
	})
	if _, err := reader.List(context.Background(), "identity-service:users"); err == nil {
		t.Fatal("expected strict legacy-only remote reader request to fail")
	}
	if gotName != "" || gotKey != "" {
		t.Fatalf("strict legacy-only headers name=%q key=%q, want no service identity headers", gotName, gotKey)
	}
}

func TestRemoteServiceReaderPrefersIdentityReadContract(t *testing.T) {
	serviceKey := testServiceKey(t)
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		if r.Header.Get("X-Service-Key") != serviceKey {
			WriteError(w, r, http.StatusUnauthorized, "unauthorized", "service authentication is required")
			return
		}
		switch r.URL.Path {
		case "/internal/identity/users":
			WriteJSON(w, r, http.StatusOK, []contracts.Record[map[string]any]{
				{ID: "US1", Data: map[string]any{"id": "US1", "username": "alice"}},
			})
		case "/internal/identity/users/US1":
			WriteJSON(w, r, http.StatusOK, contracts.Record[map[string]any]{
				ID: "US1", Data: map[string]any{"id": "US1", "username": "alice"},
			})
		default:
			WriteError(w, r, http.StatusNotFound, "not_found", "unexpected path")
		}
	}))
	t.Cleanup(server.Close)

	reader := NewRemoteServiceReader(Config{
		ServiceURLs:   map[string]string{"identity-service": server.URL},
		ServiceAPIKey: serviceKey,
	})
	if _, err := reader.List(context.Background(), "identity-service:users"); err != nil {
		t.Fatalf("list: %v", err)
	}
	if _, ok, err := reader.Get(context.Background(), "identity-service:users", "US1"); err != nil || !ok {
		t.Fatalf("get ok=%v err=%v, want ok/nil", ok, err)
	}

	want := []string{"/internal/identity/users", "/internal/identity/users/US1"}
	if len(paths) != len(want) {
		t.Fatalf("paths = %#v, want %#v", paths, want)
	}
	for i := range want {
		if paths[i] != want[i] {
			t.Fatalf("paths = %#v, want %#v", paths, want)
		}
	}
}

func TestRemoteServiceReaderUsesOrgProjectCompositeReadContracts(t *testing.T) {
	serviceKey := testServiceKey(t)
	wantPath := "/internal/org-project/project-members/project-1/user-1"
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		if r.Header.Get("X-Service-Key") != serviceKey {
			WriteError(w, r, http.StatusUnauthorized, "unauthorized", "service authentication is required")
			return
		}
		if r.URL.Path != wantPath {
			WriteError(w, r, http.StatusNotFound, "not_found", "unexpected path")
			return
		}
		WriteJSON(w, r, http.StatusOK, contracts.Record[map[string]any]{
			ID: "project-1/user-1",
			Data: map[string]any{
				"id":         "project-1/user-1",
				"project_id": "project-1",
				"user_id":    "user-1",
				"role":       "user",
			},
		})
	}))
	t.Cleanup(server.Close)

	reader := NewRemoteServiceReader(Config{
		ServiceURLs:   map[string]string{"org-project-service": server.URL},
		ServiceAPIKey: serviceKey,
	})
	record, ok, err := reader.Get(context.Background(), "org-project-service:project_members", "project-1/user-1")
	if err != nil || !ok || record.ID != "project-1/user-1" {
		t.Fatalf("composite get = %#v ok=%v err=%v, want project-1/user-1", record, ok, err)
	}
	if len(paths) != 1 || paths[0] != wantPath {
		t.Fatalf("paths = %#v, want [%q]", paths, wantPath)
	}
}

func TestRemoteServiceReaderRejectsResourcesWithoutDomainContract(t *testing.T) {
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		WriteError(w, r, http.StatusInternalServerError, "unexpected", "generic records fallback should not be called")
	}))
	t.Cleanup(server.Close)

	reader := NewRemoteServiceReader(Config{
		ServiceURLs:   map[string]string{"widget-service": server.URL},
		ServiceAPIKey: "service-key",
	})
	if _, err := reader.List(context.Background(), "widget-service:widgets"); err == nil {
		t.Fatal("expected missing domain read contract to fail")
	}
	if called {
		t.Fatal("remote reader called HTTP server for resource without domain contract")
	}
}

func TestRemoteServiceReaderRejectsListOnlyGetContract(t *testing.T) {
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		WriteError(w, r, http.StatusInternalServerError, "unexpected", "list-only get should fail before HTTP")
	}))
	t.Cleanup(server.Close)

	reader := NewRemoteServiceReader(Config{
		ServiceURLs:   map[string]string{"workload-service": server.URL},
		ServiceAPIKey: "service-key",
	})
	if _, ok, err := reader.Get(context.Background(), "workload-service:jobs", "job-1"); err == nil || ok {
		t.Fatalf("get list-only workload jobs ok=%v err=%v, want fail-closed error", ok, err)
	}
	if called {
		t.Fatal("remote reader called HTTP server for workload jobs get despite list-only contract")
	}
}

func TestCrossServiceStoreRoutesRemoteReads(t *testing.T) {
	serviceKey := testServiceKey(t)
	_, ownerURL := startOwnerService(t, serviceKey)
	// An isolated service with SERVICE_URLS configured resolves contracted owner
	// resources remotely.
	consumer := NewApp(Config{
		ServiceName:         "usage-observability-service",
		RequireAuth:         false,
		ServiceURLs:         map[string]string{"identity-service": ownerURL},
		ServiceIdentityName: "consumer-service",
		ServiceIdentityKey:  serviceKey,
	})
	if _, ok := consumer.Store.(*crossServiceStore); !ok {
		t.Fatalf("store = %T, want *crossServiceStore", consumer.Store)
	}

	// Reading another service's resource transparently goes over HTTP.
	users := consumer.Store.List(context.Background(), "identity-service:users")
	if len(users) != 1 || users[0].Data["username"] != "alice" {
		t.Fatalf("cross-service list = %#v", users)
	}

	// Writing/reading its own resource stays local.
	if _, err := consumer.Store.Create(context.Background(), "usage-observability-service:dashboards", map[string]any{"id": "D1"}); err != nil {
		t.Fatalf("local create: %v", err)
	}
	if got := consumer.Store.List(context.Background(), "usage-observability-service:dashboards"); len(got) != 1 {
		t.Fatalf("local list = %#v", got)
	}
}

func TestCrossServiceStoreIsPassthroughWhenCoHosted(t *testing.T) {
	app := NewApp(Config{ServiceName: "all", RequireAuth: false, ServiceURLs: map[string]string{"identity-service": "http://unused"}})
	// all-mode hosts every owner, so the store is not decorated.
	if _, ok := app.Store.(*crossServiceStore); ok {
		t.Fatal("SERVICE_NAME=all should not wrap the store")
	}
}

func testServiceKey(t *testing.T) string {
	t.Helper()
	return "svc-" + t.Name()
}
