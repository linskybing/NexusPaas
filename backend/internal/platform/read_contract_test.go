package platform

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func readContractTestApp(t *testing.T) *App {
	t.Helper()
	app := NewApp(Config{ServiceName: "all", HTTPAddr: ":0", ServiceAPIKey: "svc-key"})
	app.RegisterReadContract("test-service:items", "/internal/test/items", "/internal/test/items/{id...}")
	return app
}

func getReadContract(t *testing.T, app *App, target, key string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, target, nil)
	if key != "" {
		req.Header.Set("X-Service-Key", key)
	}
	app.ServeHTTP(rec, req)
	return rec
}

func getScopedReadContract(t *testing.T, app *App, target, caller, key string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, target, nil)
	if caller != "" {
		req.Header.Set(serviceNameHeader, caller)
	}
	if key != "" {
		req.Header.Set(serviceKeyHeader, key)
	}
	app.ServeHTTP(rec, req)
	return rec
}

func TestRegisterReadContractRequiresServiceAuth(t *testing.T) {
	app := readContractTestApp(t)
	if rec := getReadContract(t, app, "/internal/test/items", ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("missing service key: got %d, want 401", rec.Code)
	}
	if rec := getReadContract(t, app, "/internal/test/items", "wrong"); rec.Code != http.StatusUnauthorized {
		t.Fatalf("bad key: got %d, want 401", rec.Code)
	}

	// When the server itself has no service key configured, the internal contract is
	// closed entirely (404), matching AuthorizeServiceRequest's fail-closed behavior.
	closed := NewApp(Config{ServiceName: "all", HTTPAddr: ":0"})
	closed.RegisterReadContract("test-service:items", "/internal/test/items", "/internal/test/items/{id...}")
	if rec := getReadContract(t, closed, "/internal/test/items", "anything"); rec.Code != http.StatusNotFound {
		t.Fatalf("no server key configured: got %d, want 404", rec.Code)
	}
}

func TestRegisterReadContractValidatesScopedCallerAudience(t *testing.T) {
	app := NewApp(Config{
		ServiceName: "all",
		HTTPAddr:    ":0",
		ServiceTrustedIdentities: map[string]ServiceTrustedIdentity{
			"allowed-caller": {Key: "allowed-key", Audiences: []string{"test-service"}},
			"wrong-caller":   {Key: "wrong-key", Audiences: []string{"other-service"}},
		},
	})
	app.RegisterReadContract("test-service:items", "/internal/test/items", "/internal/test/items/{id...}")

	for _, path := range []string{"/internal/test/items", "/internal/test/items/missing"} {
		if rec := getScopedReadContract(t, app, path, "wrong-caller", "wrong-key"); rec.Code != http.StatusUnauthorized {
			t.Fatalf("%s wrong audience status = %d, want 401: %s", path, rec.Code, rec.Body.String())
		}
	}
	if rec := getScopedReadContract(t, app, "/internal/test/items", "allowed-caller", "allowed-key"); rec.Code != http.StatusOK {
		t.Fatalf("allowed scoped caller status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
}

func TestRegisterReadContractListAndGet(t *testing.T) {
	app := readContractTestApp(t)
	ctx := context.Background()
	if _, err := app.Store.Create(ctx, "test-service:items", map[string]any{"id": "x1", "v": 1}); err != nil {
		t.Fatal(err)
	}

	rec := getReadContract(t, app, "/internal/test/items", "svc-key")
	if rec.Code != http.StatusOK {
		t.Fatalf("list status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var listEnv struct {
		Data []map[string]any `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &listEnv); err != nil {
		t.Fatal(err)
	}
	if len(listEnv.Data) != 1 {
		t.Fatalf("list returned %d records, want 1", len(listEnv.Data))
	}

	if rec := getReadContract(t, app, "/internal/test/items/x1", "svc-key"); rec.Code != http.StatusOK {
		t.Fatalf("get status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	if rec := getReadContract(t, app, "/internal/test/items/missing", "svc-key"); rec.Code != http.StatusNotFound {
		t.Fatalf("get missing status = %d, want 404", rec.Code)
	}
}

func TestRegisterReadContractGetHandlesCompositeSlashKey(t *testing.T) {
	app := readContractTestApp(t)
	ctx := context.Background()
	// Composite keys like "<projectID>/<userID>" contain a slash; the {id...} wildcard
	// must capture the whole suffix so cross-service Get resolves them.
	if _, err := app.Store.Create(ctx, "test-service:items", map[string]any{"id": "proj1/user2", "v": 9}); err != nil {
		t.Fatal(err)
	}
	rec := getReadContract(t, app, "/internal/test/items/proj1/user2", "svc-key")
	if rec.Code != http.StatusOK {
		t.Fatalf("composite-key get status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var env struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	if env.Data["id"] != "proj1/user2" {
		t.Fatalf("composite-key get returned %#v, want id=proj1/user2", env.Data)
	}
}

func TestRegisterReadContractListOnly(t *testing.T) {
	app := NewApp(Config{ServiceName: "all", HTTPAddr: ":0", ServiceAPIKey: "svc-key"})
	app.RegisterReadContract("test-service:jobs", "/internal/test/jobs", "")
	if rec := getReadContract(t, app, "/internal/test/jobs", "svc-key"); rec.Code != http.StatusOK {
		t.Fatalf("list-only contract list status = %d, want 200", rec.Code)
	}
}
