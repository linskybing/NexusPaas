package services

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/contracts"
	"github.com/linskybing/nexuspaas/backend/internal/platform"
)

type envelope struct {
	Success  bool            `json:"success"`
	Data     json.RawMessage `json:"data"`
	Error    any             `json:"error"`
	Degraded any             `json:"degraded"`
}

func newTestApp() *platform.App {
	app := platform.NewApp(platform.Config{ServiceName: "all", HTTPAddr: ":0", APIKeys: map[string]bool{"test-key": true}, ExternalURLs: map[string]string{}})
	RegisterAll(app)
	return app
}

func TestServiceCatalogCoversAllServices(t *testing.T) {
	app := newTestApp()
	want := []string{
		"platform-gateway",
		"identity-service",
		"authorization-policy-service",
		"org-project-service",
		"workload-service",
		"scheduler-quota-service",
		"k8s-control-service",
		"ide-service",
		"storage-service",
		"image-registry-service",
		"usage-observability-service",
		"audit-compliance-service",
		"request-notification-service",
		"integration-proxy-service",
		"media-upload-service",
	}
	if len(app.Services) != len(want) {
		t.Fatalf("service count = %d, want %d", len(app.Services), len(want))
	}
	for _, name := range want {
		if _, ok := app.Services[name]; !ok {
			t.Fatalf("missing service %s", name)
		}
	}
}

func TestRouteCoverage(t *testing.T) {
	app := newTestApp()
	patterns := map[string]bool{}
	for _, route := range app.Routes {
		patterns[route.Pattern] = true
	}
	required := []string{
		"/api/v1/login",
		"/api/v1/users",
		"/api/v1/oidc/jwks",
		"/api/v1/permissions/policies",
		"/api/v1/admin/proxy-rbac/services",
		"/api/v1/groups",
		"/api/v1/user-groups",
		"/api/v1/projects",
		"/api/v1/configfiles",
		"/api/v1/jobs",
		"/api/v1/plans",
		"/api/v1/queues",
		"/api/v1/internal/quota/reservations",
		"/api/v1/internal/scheduler/admission",
		"/api/v1/k8s/cluster",
		"/api/v1/resources",
		"/api/v1/ws/exec",
		"/api/v1/ide",
		"/api/v1/storage/permissions",
		"/api/v1/projects/{id}/storage/bindings",
		"/api/v1/image-requests",
		"/api/v1/images/build",
		"/api/v1/image-catalog",
		"/api/v1/harbor-status",
		"/api/v1/me/usage",
		"/api/v1/admin/usage",
		"/api/v1/dashboard/overview",
		"/api/v1/audit/events",
		"/api/v1/admin/security/posture",
		"/api/v1/forms",
		"/api/v1/notifications/read-all",
		"/api/v1/announcements/active",
		"/api/v1/admin/announcements",
		"/api/v1/grafana/{path...}",
		"/api/v1/minio-console/{path...}",
		"/api/v1/pgadmin/{path...}",
		"/api/v1/longhorn/{path...}",
		"/api/v1/admin/vpn",
		"/api/v1/uploads/images",
	}
	for _, pattern := range required {
		if !patterns[pattern] {
			t.Fatalf("missing route pattern %s", pattern)
		}
	}
}

func TestOpenAPICoversAllRegisteredServiceRoutes(t *testing.T) {
	app := newTestApp()
	doc := app.OpenAPI()
	paths, ok := doc["paths"].(map[string]map[string]any)
	if !ok {
		t.Fatalf("openapi paths type = %T, want map[string]map[string]any", doc["paths"])
	}

	for _, route := range app.Routes {
		pattern := openAPIRoutePattern(route.Pattern)
		operations, ok := paths[pattern]
		if !ok {
			t.Fatalf("openapi missing path for registered route %s %s (expected %s)", route.Method, route.Pattern, pattern)
		}
		if _, ok := operations[strings.ToLower(route.Method)]; !ok {
			t.Fatalf("openapi missing method for registered route %s %s", route.Method, route.Pattern)
		}
	}
}

func openAPIRoutePattern(pattern string) string {
	segments := strings.Split(pattern, "/")
	for i, segment := range segments {
		if !strings.HasPrefix(segment, "{") || !strings.HasSuffix(segment, "...}") {
			continue
		}
		name := strings.TrimSuffix(strings.TrimPrefix(segment, "{"), "...}")
		segments[i] = "{" + name + "}"
	}
	return strings.Join(segments, "/")
}

func TestProxyRoutesWithoutAdaptersAreServiceOwned(t *testing.T) {
	app := newTestApp()
	for _, route := range app.Routes {
		if route.Action != "proxy" || route.ExternalAdapter != "" {
			continue
		}
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(route.Method, sampleRoutePath(route.Pattern), nil)
		app.ServeHTTP(rec, req)
		body := rec.Body.String()
		if strings.Contains(body, `"adapter":"unbound_proxy"`) {
			t.Fatalf("%s %s fell through to unbound generic proxy: status=%d body=%s", route.Method, route.Pattern, rec.Code, body)
		}
	}
}

func TestCommonEndpoints(t *testing.T) {
	app := newTestApp()
	for _, path := range []string{"/healthz", "/readyz", "/metrics", "/openapi.json", "/swagger/", "/service-registry"} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		app.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s returned %d: %s", path, rec.Code, rec.Body.String())
		}
	}
	for _, path := range []string{"/healthz", "/readyz"} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		app.ServeHTTP(rec, req)
		var payload map[string]any
		decodeData(t, rec, &payload)
		data := payload["data"].(map[string]any)
		if data["status"] != "ok" {
			t.Fatalf("%s status = %v, want ok", path, data["status"])
		}
	}
}

func TestSwaggerEndpointServesEmbeddedSpec(t *testing.T) {
	app := newTestApp()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/swagger/", nil)
	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("/swagger/ returned %d: %s", rec.Code, rec.Body.String())
	}
	if contentType := rec.Header().Get("Content-Type"); !strings.HasPrefix(contentType, "text/html") {
		t.Fatalf("swagger content type = %q, want text/html", contentType)
	}
	body := rec.Body.String()
	for _, want := range []string{"SwaggerUIBundle", `"openapi":"3.1.0"`, `"paths":`} {
		if !strings.Contains(body, want) {
			t.Fatalf("swagger body missing %q: %s", want, body)
		}
	}
}

func TestCORSAllowlistPreflight(t *testing.T) {
	app := newCORSAllowlistTestApp(t, true, "http://allowed.test")

	rec := requestOptions(t, app, "/api/v1/login", "http://allowed.test")
	assertCORSOrigin(t, rec, "http://allowed.test")

	rec = requestOptions(t, app, "/api/v1/login", "http://blocked.test")
	assertCORSOrigin(t, rec, "")

	rec = requestOptions(t, app, "/api/v1/login", "")
	assertCORSOrigin(t, rec, "")
}

func TestCORSAllowlistWrappedRoutes(t *testing.T) {
	app := newCORSAllowlistTestApp(t, true, "http://allowed.test")

	rec := requestRaw(t, app, http.MethodGet, "/api/v1/captcha", map[string]string{"Origin": "http://allowed.test"}, http.StatusOK)
	assertCORSOrigin(t, rec, "http://allowed.test")

	rec = requestRaw(t, app, http.MethodGet, "/api/v1/captcha", map[string]string{"Origin": "http://blocked.test"}, http.StatusOK)
	assertCORSOrigin(t, rec, "")

	rec = requestRaw(t, app, http.MethodGet, "/api/v1/captcha", nil, http.StatusOK)
	assertCORSOrigin(t, rec, "")
}

func TestCORSAllowlistFallbackModes(t *testing.T) {
	t.Run("production denies local origin when unset", func(t *testing.T) {
		app := newCORSAllowlistTestApp(t, true, "")
		rec := requestOptions(t, app, "/api/v1/login", "http://localhost:3000")
		assertCORSOrigin(t, rec, "")
	})

	t.Run("non-production allows local fallback only", func(t *testing.T) {
		app := newCORSAllowlistTestApp(t, false, "")
		rec := requestOptions(t, app, "/api/v1/login", "http://localhost:3000")
		assertCORSOrigin(t, rec, "http://localhost:3000")

		rec = requestOptions(t, app, "/api/v1/login", "http://blocked.test")
		assertCORSOrigin(t, rec, "")
	})
}

func newCORSAllowlistTestApp(t *testing.T, production bool, origins string) *platform.App {
	t.Helper()
	if production {
		t.Setenv("PRODUCTION", "true")
	} else {
		t.Setenv("PRODUCTION", "false")
	}
	t.Setenv("ALLOWED_ORIGINS", origins)
	t.Setenv("SERVICE_NAME", "all")
	t.Setenv("HTTP_ADDR", ":0")
	t.Setenv("REQUIRE_AUTH", "false")

	cfg := platform.ConfigFromEnv()
	cfg.HTTPAddr = ":0"
	app := platform.NewApp(cfg)
	RegisterAll(app)
	return app
}

func requestOptions(t *testing.T, app http.Handler, path, origin string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodOptions, path, nil)
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	app.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("OPTIONS %s returned %d, want %d: %s", path, rec.Code, http.StatusNoContent, rec.Body.String())
	}
	return rec
}

func assertCORSOrigin(t *testing.T, rec *httptest.ResponseRecorder, want string) {
	t.Helper()
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != want {
		t.Fatalf("Access-Control-Allow-Origin = %q, want %q", got, want)
	}
	if got := rec.Header().Get("Access-Control-Allow-Credentials"); got != "" {
		t.Fatalf("Access-Control-Allow-Credentials = %q, want omitted", got)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got == "*" {
		t.Fatal("Access-Control-Allow-Origin must not be wildcard")
	}
}

func TestOperationalEndpointsRequireAdminWhenAuthEnabled(t *testing.T) {
	app := newAuthCommonEndpointTestApp(t)
	for _, path := range []string{"/healthz", "/readyz"} {
		requestRaw(t, app, http.MethodGet, path, nil, http.StatusOK)
	}

	opsPaths := []string{"/metrics", "/openapi.json", "/swagger/", "/service-registry", "/outbox"}
	apiKeyOnly := map[string]string{"X-API-Key": "ops-key"}
	userSession := map[string]string{"Authorization": "Bearer user-session"}
	adminSession := map[string]string{"Authorization": "Bearer admin-session"}
	for _, path := range opsPaths {
		assertOperationalAdminPath(t, app, path, apiKeyOnly, userSession, adminSession)
	}

	rec := requestRaw(t, app, http.MethodGet, "/outbox", userSession, http.StatusForbidden)
	assertOutboxForbiddenEnvelope(t, rec)
	adminOutbox := requestRaw(t, app, http.MethodGet, "/outbox", adminSession, http.StatusOK)
	assertAdminOutboxEnvelope(t, adminOutbox)
}

func assertOperationalAdminPath(t *testing.T, app *platform.App, path string, apiKeyOnly, userSession, adminSession map[string]string) {
	t.Helper()
	requestRaw(t, app, http.MethodGet, path, nil, http.StatusUnauthorized)
	requestRaw(t, app, http.MethodGet, path, apiKeyOnly, http.StatusForbidden)
	requestRaw(t, app, http.MethodGet, path, userSession, http.StatusForbidden)
	rec := requestRaw(t, app, http.MethodGet, path, adminSession, http.StatusOK)
	if path == "/metrics" && !strings.HasPrefix(rec.Header().Get("Content-Type"), "text/plain") {
		t.Fatalf("metrics content type = %q, want text/plain", rec.Header().Get("Content-Type"))
	}
	if path == "/swagger/" && !strings.HasPrefix(rec.Header().Get("Content-Type"), "text/html") {
		t.Fatalf("swagger content type = %q, want text/html", rec.Header().Get("Content-Type"))
	}
}

func assertOutboxForbiddenEnvelope(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()
	var denied struct {
		Success bool                `json:"success"`
		Error   *platform.ErrorBody `json:"error"`
		Data    json.RawMessage     `json:"data"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&denied); err != nil {
		t.Fatal(err)
	}
	if denied.Success || denied.Error == nil || denied.Error.Code != "forbidden" || denied.Error.Message != "administrator privileges are required" {
		t.Fatalf("outbox forbidden envelope = %#v", denied)
	}
	if len(denied.Data) != 0 && string(denied.Data) != "null" {
		t.Fatalf("outbox forbidden data = %s, want absent or null", string(denied.Data))
	}
}

func assertAdminOutboxEnvelope(t *testing.T, adminOutbox *httptest.ResponseRecorder) {
	t.Helper()
	var allowed struct {
		Success bool            `json:"success"`
		Data    json.RawMessage `json:"data"`
	}
	if err := json.NewDecoder(adminOutbox.Body).Decode(&allowed); err != nil {
		t.Fatal(err)
	}
	if !allowed.Success || len(allowed.Data) == 0 {
		t.Fatalf("admin outbox envelope = %#v", allowed)
	}
	var events []any
	if err := json.Unmarshal(allowed.Data, &events); err != nil {
		t.Fatalf("admin outbox data is not an array: %s", string(allowed.Data))
	}
}

func newAuthCommonEndpointTestApp(t *testing.T) *platform.App {
	t.Helper()
	app := platform.NewApp(platform.Config{
		ServiceName:  "all",
		HTTPAddr:     ":0",
		RequireAuth:  true,
		APIKeys:      map[string]bool{"ops-key": true},
		ExternalURLs: map[string]string{},
	})
	RegisterAll(app)
	now := time.Now().UTC()
	createRows(t, app, "identity-service:users", []map[string]any{
		{"id": "ADMIN", "username": "admin", "role": "admin", "system_role": 0, "status": "online"},
		{"id": "USER", "username": "user", "role": "user", "system_role": 2, "status": "online"},
	})
	createRows(t, app, "identity-service:sessions", []map[string]any{
		{"id": "admin-session", "user_id": "ADMIN", "expires_at": now.Add(time.Hour).Format(time.RFC3339)},
		{"id": "user-session", "user_id": "USER", "expires_at": now.Add(time.Hour).Format(time.RFC3339)},
	})
	allowRawPolicy(t, app, "ADMIN", "", "platform-runtime:metrics", "platform_runtime_metrics")
	allowRawPolicy(t, app, "ADMIN", "", "platform-runtime:openapi", "platform_runtime_openapi")
	allowRawPolicy(t, app, "ADMIN", "", "platform-runtime:swagger", "platform_runtime_swagger")
	allowRawPolicy(t, app, "ADMIN", "", "platform-runtime:service-registry", "platform_runtime_service_registry")
	allowRawPolicy(t, app, "ADMIN", "", "platform-runtime:outbox", "platform_runtime_outbox")
	return app
}

func TestAuthRequiredWhenConfigured(t *testing.T) {
	app := platform.NewApp(platform.Config{
		ServiceName:      "all",
		HTTPAddr:         ":0",
		RequireAuth:      true,
		APIKeys:          map[string]bool{"test-key": true},
		APIKeyPrincipals: map[string]platform.APIKeyPrincipal{"test-key": {ID: "test-user", Role: "user"}},
		ExternalURLs:     map[string]string{},
	})
	RegisterAll(app)
	allowPolicyForRoute(t, app, "test-user", "", http.MethodGet, "/api/v1/configfiles")

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/users", nil)
	app.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated users route returned %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/v1/users", nil)
	req.Header.Set("X-API-Key", "test-key")
	app.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("api-key admin users route returned %d, want 403: %s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/v1/configfiles", nil)
	req.Header.Set("X-API-Key", "test-key")
	app.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("authenticated configfiles route returned %d: %s", rec.Code, rec.Body.String())
	}

	noKeyApp := platform.NewApp(platform.Config{ServiceName: "all", HTTPAddr: ":0", RequireAuth: true, ExternalURLs: map[string]string{}})
	RegisterAll(noKeyApp)
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/v1/configfiles", nil)
	req.Header.Set("X-API-Key", "forged")
	noKeyApp.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("unconfigured api key returned %d, want 401: %s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/v1/login", bytes.NewBufferString(`{"username":"u"}`))
	app.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("public login without password returned %d: %s", rec.Code, rec.Body.String())
	}
}

func TestPolicyDecisionPointIsUsedForAuthorization(t *testing.T) {
	app := newTestApp()
	app.Config.RequireAuth = true
	app.Config.APIKeys = map[string]bool{"policy-key": true}
	app.Config.APIKeyPrincipals = map[string]platform.APIKeyPrincipal{
		"policy-key": {ID: "policy-user", Username: "policy-user", Role: "user"},
	}
	app.PDP = platform.StaticPDP{Allowed: false, Reason: "test deny"}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/users/policy-user", nil)
	req.Header.Set("X-API-Key", "policy-key")
	app.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("PDP deny returned %d, want 403", rec.Code)
	}
}

func TestEventContractsRequireMetadata(t *testing.T) {
	bus := platform.NewEventBus()
	if err := bus.Publish(context.Background(), contracts.Event{Name: "AuditEvent"}); err == nil {
		t.Fatal("Publish accepted event without required metadata")
	}

	app := newTestApp()
	union := map[string]bool{}
	for _, spec := range app.Services {
		for _, name := range spec.Events {
			union[name] = true
		}
	}
	for _, required := range []string{
		"UserCreated",
		"UserUpdated",
		"UserDisabled",
		"GroupCreated",
		"GroupMembershipChanged",
		"ProjectCreated",
		"ProjectUpdated",
		"ProjectDeleted",
		"PolicyChanged",
		"ProxyPolicyChanged",
		"PlanChanged",
		"ConfigCommitted",
		"JobSubmitted",
		"JobQueued",
		"JobRunning",
		"JobSucceeded",
		"JobFailed",
		"JobCancelled",
		"QuotaReserved",
		"QuotaCommitted",
		"QuotaReleased",
		"SubmitAdmissionReviewed",
		"QueueDepthChanged",
		"JobPreempted",
		"PriorityClassSyncCompleted",
		"ResourceSnapshotRecorded",
		"NamespaceCreated",
		"NamespaceDeleted",
		"IDEStarted",
		"IDEStopped",
		"IDEDeleted",
		"IDEIdleReaped",
		"PVCProvisioned",
		"StorageBound",
		"StoragePermissionChanged",
		"FastTransferCompleted",
		"LonghornRWXHealthChecked",
		"ImageRequested",
		"ImageApproved",
		"ImageBuildStarted",
		"ImageBuilt",
		"ImagePublished",
		"ImageSyncFailed",
		"UsageSnapshotRecorded",
		"ResourceHoursSummarized",
		"AuditEvent",
		"FormCreated",
		"FormUpdated",
		"NotificationRequested",
		"AnnouncementPublished",
		"ProxySessionStarted",
		"ProxySessionTerminated",
		"MediaUploaded",
		"MediaDeleted",
	} {
		if !union[required] {
			t.Fatalf("missing event contract %s", required)
		}
	}
}

func TestConfigFileImmutableCommit(t *testing.T) {
	app := newTestApp()
	first := postJSON(t, app, "/api/v1/configfiles/cfg-1/versions", `{"content":"print(1)","message":"initial"}`, http.StatusCreated)
	second := postJSON(t, app, "/api/v1/configfiles/cfg-1/versions", `{"content":"print(1)","message":"same content"}`, http.StatusCreated)

	var firstData, secondData map[string]any
	decodeData(t, first, &firstData)
	decodeData(t, second, &secondData)
	firstRecord := firstData["data"].(map[string]any)
	secondRecord := secondData["data"].(map[string]any)
	firstPayload := firstRecord["data"].(map[string]any)
	secondPayload := secondRecord["data"].(map[string]any)

	if firstRecord["id"] == secondRecord["id"] {
		t.Fatal("immutable commits reused the same version id")
	}
	if firstPayload["sha256"] != secondPayload["sha256"] {
		t.Fatal("same content produced different sha256")
	}
	if firstPayload["immutable"] != true || secondPayload["immutable"] != true {
		t.Fatal("config versions are not marked immutable")
	}
}

func TestQuotaReservationStateMachine(t *testing.T) {
	app := newTestApp()
	reservedEvents := eventCount(app, "QuotaReserved")
	reserved := postJSON(t, app, "/api/v1/internal/quota/reservations", `{"project_id":"p1","gpu":1}`, http.StatusCreated)
	assertEventCount(t, app, "QuotaReserved", reservedEvents+1)
	reservedEvent := lastEventByName(t, app, "QuotaReserved")
	var payload map[string]any
	decodeData(t, reserved, &payload)
	record := payload["data"].(map[string]any)
	id := record["id"].(string)
	if reservedEvent.Data["reservation_id"] != id || reservedEvent.Data["state"] != "reserved" {
		t.Fatalf("QuotaReserved data = %#v, want reservation_id %q state reserved", reservedEvent.Data, id)
	}

	committedEvents := eventCount(app, "QuotaCommitted")
	committed := postJSON(t, app, "/api/v1/internal/quota/reservations/"+id+"/commit", `{}`, http.StatusOK)
	assertEventCount(t, app, "QuotaCommitted", committedEvents+1)
	committedEvent := lastEventByName(t, app, "QuotaCommitted")
	if committedEvent.Data["reservation_id"] != id || committedEvent.Data["state"] != "committed" {
		t.Fatalf("QuotaCommitted data = %#v, want reservation_id %q state committed", committedEvent.Data, id)
	}
	var committedPayload map[string]any
	decodeData(t, committed, &committedPayload)
	committedRecord := committedPayload["data"].(map[string]any)
	if committedRecord["data"].(map[string]any)["state"] != "committed" {
		t.Fatalf("state after commit = %v", committedRecord["data"])
	}

	postJSON(t, app, "/api/v1/internal/quota/reservations/"+id+"/commit", `{}`, http.StatusOK)
	assertEventCount(t, app, "QuotaCommitted", committedEvents+1)

	releasedEvents := eventCount(app, "QuotaReleased")
	postJSON(t, app, "/api/v1/internal/quota/reservations/"+id+"/release", `{}`, http.StatusOK)
	assertEventCount(t, app, "QuotaReleased", releasedEvents+1)
	releasedEvent := lastEventByName(t, app, "QuotaReleased")
	if releasedEvent.Data["reservation_id"] != id || releasedEvent.Data["state"] != "released" {
		t.Fatalf("QuotaReleased data = %#v, want reservation_id %q state released", releasedEvent.Data, id)
	}
	postJSON(t, app, "/api/v1/internal/quota/reservations/"+id+"/release", `{}`, http.StatusOK)
	assertEventCount(t, app, "QuotaReleased", releasedEvents+1)
	postJSON(t, app, "/api/v1/internal/quota/reservations/"+id+"/commit", `{}`, http.StatusConflict)
	assertEventCount(t, app, "QuotaCommitted", committedEvents+1)

	_, _ = app.Store.Create(context.Background(), "scheduler-quota-service:reservations", map[string]any{"id": "corrupt-reservation", "state": "unknown"})
	postJSON(t, app, "/api/v1/internal/quota/reservations/corrupt-reservation/commit", `{}`, http.StatusConflict)
	assertEventCount(t, app, "QuotaCommitted", committedEvents+1)
}

func eventCount(app *platform.App, name string) int {
	count := 0
	for _, event := range app.Events.Outbox() {
		if event.Name == name {
			count++
		}
	}
	return count
}

func assertEventCount(t *testing.T, app *platform.App, name string, want int) {
	t.Helper()
	if got := eventCount(app, name); got != want {
		t.Fatalf("%s event count = %d, want %d", name, got, want)
	}
}

func lastEventByName(t *testing.T, app *platform.App, name string) contracts.Event {
	t.Helper()
	events := app.Events.Outbox()
	for i := len(events) - 1; i >= 0; i-- {
		if events[i].Name == name {
			return events[i]
		}
	}
	t.Fatalf("missing event %s in outbox %#v", name, events)
	return contracts.Event{}
}

func sampleRoutePath(pattern string) string {
	parts := strings.Split(strings.Trim(pattern, "/"), "/")
	for i, part := range parts {
		switch {
		case strings.HasPrefix(part, "{") && strings.HasSuffix(part, "...}"):
			parts[i] = "sample/path"
		case strings.HasPrefix(part, "{") && strings.HasSuffix(part, "}"):
			parts[i] = "sample"
		}
	}
	return "/" + strings.Join(parts, "/")
}

func TestAuditEventsAreEmittedForMutations(t *testing.T) {
	app := newTestApp()
	postJSON(t, app, "/api/v1/forms", `{"title":"Need GPU","description":"request quota review"}`, http.StatusCreated)
	for _, event := range app.Events.Outbox() {
		if event.Name == "AuditEvent" && event.Data["action"] != "" && event.Data["success"] == true {
			return
		}
	}
	t.Fatal("mutation did not emit AuditEvent")
}

func TestExternalAdapterDegradedResponse(t *testing.T) {
	app := newTestApp()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/harbor-status", nil)
	app.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("harbor status returned %d", rec.Code)
	}
	var env envelope
	if err := json.NewDecoder(rec.Body).Decode(&env); err != nil {
		t.Fatal(err)
	}
	if env.Degraded == nil {
		t.Fatal("expected degraded response for unconfigured Harbor adapter")
	}
}

func TestWorkerLeasePreventsDuplicateShardProcessing(t *testing.T) {
	app := newTestApp()
	first := postJSON(t, app, "/api/v1/internal/workers/leases", `{"worker":"a","shard":"audit-0"}`, http.StatusOK)
	second := postJSON(t, app, "/api/v1/internal/workers/leases", `{"worker":"b","shard":"audit-0"}`, http.StatusOK)
	var firstPayload, secondPayload map[string]any
	decodeData(t, first, &firstPayload)
	decodeData(t, second, &secondPayload)
	if firstPayload["data"].(map[string]any)["acquired"] != true {
		t.Fatal("first worker did not acquire lease")
	}
	if secondPayload["data"].(map[string]any)["acquired"] != false {
		t.Fatal("second worker acquired duplicate shard lease")
	}
}

func TestPlatformHandlersRejectMalformedJSONWithoutWrites(t *testing.T) {
	app := newTestApp()
	cases := []struct {
		name     string
		method   string
		path     string
		resource string
	}{
		{
			name:     "generic create",
			method:   http.MethodPost,
			path:     "/api/v1/queues",
			resource: "scheduler-quota-service:queues",
		},
		{
			name:     "generic update put",
			method:   http.MethodPut,
			path:     "/api/v1/queues/q-put",
			resource: "scheduler-quota-service:queues",
		},
		{
			name:     "generic update patch",
			method:   http.MethodPatch,
			path:     "/api/v1/queues/q-patch",
			resource: "scheduler-quota-service:queues",
		},
		{
			name:     "command",
			method:   http.MethodPost,
			path:     "/api/v1/jobs",
			resource: "workload-service:jobs",
		},
		{
			name:     "config commit",
			method:   http.MethodPost,
			path:     "/api/v1/configfiles/cfg-1/versions",
			resource: "workload-service:configfiles:versions",
		},
		{
			name:     "quota reservation",
			method:   http.MethodPost,
			path:     "/api/v1/internal/quota/reservations",
			resource: "scheduler-quota-service:reservations",
		},
		{
			name:     "scheduler admission",
			method:   http.MethodPost,
			path:     "/api/v1/internal/scheduler/admission",
			resource: "scheduler-quota-service:submit_admissions",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			before := len(app.Store.List(context.Background(), tc.resource))
			requestJSON(t, app, tc.method, tc.path, `{`, userHeaders("test-user"), http.StatusBadRequest)
			after := len(app.Store.List(context.Background(), tc.resource))
			if after != before {
				t.Fatalf("%s count = %d, want unchanged %d", tc.resource, after, before)
			}
		})
	}

	requestJSON(t, app, http.MethodPost, "/api/v1/internal/workers/leases", `{`, userHeaders("test-user"), http.StatusBadRequest)
	lease := responseMap(t, requestJSON(t, app, http.MethodPost, "/api/v1/internal/workers/leases", "", userHeaders("test-user"), http.StatusOK))
	if lease["worker"] != "worker" || lease["shard"] != "default" || lease["acquired"] != true {
		t.Fatalf("empty-body worker lease = %#v, want default acquired lease", lease)
	}
	outboxBefore := len(app.Events.Outbox())
	requestJSON(t, app, http.MethodPost, "/api/v1/audit/events", `{`, userHeaders("test-user"), http.StatusBadRequest)
	if outboxAfter := len(app.Events.Outbox()); outboxAfter != outboxBefore {
		t.Fatalf("outbox length after malformed audit event = %d, want unchanged %d", outboxAfter, outboxBefore)
	}
}

func TestGatewayOnlyAppDoesNotForwardRemovedCompatRoutes(t *testing.T) {
	app := platform.NewApp(platform.Config{ServiceName: "platform-gateway", HTTPAddr: ":0", ExternalURLs: map[string]string{}})
	RegisterAll(app)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/ws/job-status/job-1", nil)
	app.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound && rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("gateway-only removed compat route returned %d: %s", rec.Code, rec.Body.String())
	}
}

func TestDeploymentArtifactsExist(t *testing.T) {
	for _, spec := range Catalog() {
		for _, rel := range []string{"Dockerfile", "k8s/deployment.yaml", "migrations/0001_init.sql"} {
			path := filepath.Join("..", "..", spec.Name, rel)
			if _, err := os.Stat(path); err != nil {
				t.Fatalf("missing deployment artifact for %s: %s", spec.Name, rel)
			}
		}
	}
}

// TestRequiresClusterCapabilityMatchesClusterUsers guards against drift: exactly the
// services whose runtime workers/handlers use the cluster facade (App.Cluster) must
// declare RequiresCluster, so the readiness gate stays correct as services evolve.
func TestRequiresClusterCapabilityMatchesClusterUsers(t *testing.T) {
	want := map[string]bool{
		serviceSchedulerQuota:      true,
		serviceWorkload:            true,
		serviceK8sControl:          true,
		serviceAuthorizationPolicy: true,
		serviceUsageObservability:  true,
		serviceStorage:             true,
	}
	for _, spec := range Catalog() {
		if spec.RequiresCluster != want[spec.Name] {
			t.Errorf("%s RequiresCluster = %v, want %v", spec.Name, spec.RequiresCluster, want[spec.Name])
		}
	}
}

func postJSON(t *testing.T, app *platform.App, path, body string, want int) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", "test-"+path)
	req.Header.Set("X-User-ID", "test-user")
	req.Header.Set("X-Username", "test-user")
	app.ServeHTTP(rec, req)
	if rec.Code != want {
		t.Fatalf("POST %s returned %d, want %d: %s", path, rec.Code, want, rec.Body.String())
	}
	return rec
}

func decodeData(t *testing.T, rec *httptest.ResponseRecorder, out *map[string]any) {
	t.Helper()
	var env map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&env); err != nil {
		t.Fatal(err)
	}
	*out = env
}
