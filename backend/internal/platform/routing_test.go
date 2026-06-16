package platform

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

func TestRegisterServiceSkipsDuplicateCanonicalRoutePatterns(t *testing.T) {
	app := NewApp(Config{ServiceName: "all", HTTPAddr: ":0"})
	app.RegisterService(ServiceSpec{
		Name: "test-service",
		Routes: []RouteSpec{
			{Method: http.MethodGet, Pattern: "/api/v1/items/{id}", Resource: "items", Action: "get", IDParam: "id"},
			{Method: http.MethodGet, Pattern: "/api/v1/items/{item}", Resource: "duplicate_items", Action: "get", IDParam: "item"},
		},
	})

	if len(app.Routes) != 1 {
		t.Fatalf("route count = %d, want one canonical route", len(app.Routes))
	}
	if app.Routes[0].Resource != "test-service:items" {
		t.Fatalf("registered route = %#v, want first route to win", app.Routes[0])
	}
	if len(app.CatalogRoutes) != 2 {
		t.Fatalf("catalog route count = %d, want full catalog retained", len(app.CatalogRoutes))
	}
}

func TestRegisterServiceKeepsDistinctMethodPatternPairs(t *testing.T) {
	app := NewApp(Config{ServiceName: "all", HTTPAddr: ":0"})
	app.RegisterService(ServiceSpec{
		Name: "test-service",
		Routes: []RouteSpec{
			{Method: http.MethodGet, Pattern: "/api/v1/items/{id}", Resource: "items", Action: "get", IDParam: "id"},
			{Method: http.MethodPost, Pattern: "/api/v1/items/{item}", Resource: "items", Action: "create", IDParam: "item"},
		},
	})

	if len(app.Routes) != 2 {
		t.Fatalf("route count = %d, want GET and POST retained", len(app.Routes))
	}
}

func TestServeServiceRouteUsesBucketIndexAndSpecificity(t *testing.T) {
	app := NewApp(Config{ServiceName: "all", HTTPAddr: ":0"})
	app.RegisterService(ServiceSpec{
		Name: "test-service",
		Routes: []RouteSpec{
			{Method: http.MethodGet, Pattern: "/api/v1/groups/{id}", Resource: "groups", Action: "get", IDParam: "id"},
			{Method: http.MethodGet, Pattern: "/api/v1/projects/{id}", Resource: "projects", Action: "get", IDParam: "id"},
			{Method: http.MethodGet, Pattern: "/api/v1/{path...}", Resource: "compat_proxy", Action: "proxy"},
		},
	})
	app.RegisterCustomHandler(http.MethodGet, "/api/v1/groups/{id}", routeEchoHandler)
	app.RegisterCustomHandler(http.MethodGet, "/api/v1/{path...}", routeEchoHandler)

	candidates := app.routeCandidates(http.MethodGet, "/api/v1/groups/g1")
	if len(candidates) != 2 {
		t.Fatalf("candidate count = %d, want concrete group + wildcard: %#v", len(candidates), candidates)
	}
	for _, route := range candidates {
		if route.Resource == "test-service:projects" {
			t.Fatalf("candidate set included cross-bucket project route: %#v", candidates)
		}
	}

	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/groups/g1", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	data := responseData(t, rec)
	if data["resource"] != "test-service:groups" || data["id"] != "g1" {
		t.Fatalf("response data = %#v, want concrete group route", data)
	}
}

func TestServeServiceRouteFallsThroughForUnrelatedBucket(t *testing.T) {
	app := NewApp(Config{ServiceName: "all", HTTPAddr: ":0"})
	app.RegisterService(ServiceSpec{
		Name: "test-service",
		Routes: []RouteSpec{
			{Method: http.MethodGet, Pattern: "/api/v1/projects/{id}", Resource: "projects", Action: "get", IDParam: "id"},
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/groups/g1", nil)
	if app.serveServiceRoute(httptest.NewRecorder(), req) {
		t.Fatal("serveServiceRoute matched unrelated bucket, want false")
	}
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/groups/g1", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want mux 405 fallthrough", rec.Code)
	}
}

func TestServeServiceRouteRebuildsIndexForSameLengthRouteMutation(t *testing.T) {
	app := NewApp(Config{ServiceName: "all", HTTPAddr: ":0"})
	app.Routes = []RouteSpec{{Method: http.MethodGet, Pattern: "/api/v1/old/{id}", Resource: "test-service:old", Action: "get", IDParam: "id"}}
	app.rebuildRouteIndex()
	app.RegisterCustomHandler(http.MethodGet, "/api/v1/new/{id}", routeEchoHandler)
	app.Routes[0] = RouteSpec{Method: http.MethodGet, Pattern: "/api/v1/new/{id}", Resource: "test-service:new", Action: "get", IDParam: "id"}

	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/new/n1", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 after same-length route mutation: %s", rec.Code, rec.Body.String())
	}
	data := responseData(t, rec)
	if data["resource"] != "test-service:new" || data["id"] != "n1" {
		t.Fatalf("response data = %#v, want rebuilt new route", data)
	}
}

func TestHandleReservationTransitionStateMachine(t *testing.T) {
	app := NewApp(Config{ServiceName: "all", HTTPAddr: ":0"})
	route := RouteSpec{IDParam: "reservationId"}
	cases := []struct {
		name      string
		current   string
		requested string
		want      int
		wantState string
	}{
		{name: "reserved noop", current: "reserved", requested: "reserved", want: http.StatusOK, wantState: "reserved"},
		{name: "reserved commit", current: "reserved", requested: "committed", want: http.StatusOK, wantState: "committed"},
		{name: "reserved release", current: "reserved", requested: "released", want: http.StatusOK, wantState: "released"},
		{name: "committed release", current: "committed", requested: "released", want: http.StatusOK, wantState: "released"},
		{name: "released noop", current: "released", requested: "released", want: http.StatusOK, wantState: "released"},
		{name: "released commit conflict", current: "released", requested: "committed", want: http.StatusConflict, wantState: "released"},
		{name: "committed reserved conflict", current: "committed", requested: "reserved", want: http.StatusConflict, wantState: "committed"},
		{name: "unknown commit conflict", current: "unknown", requested: "committed", want: http.StatusConflict, wantState: "unknown"},
		{name: "unknown release conflict", current: "unknown", requested: "released", want: http.StatusConflict, wantState: "unknown"},
	}
	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			id := "res-" + strconv.Itoa(i)
			_, _ = app.Store.Create(context.Background(), "scheduler-quota-service:reservations", map[string]any{"id": id, "state": tc.current})
			status, _ := app.handleReservationTransition(testHTTPRequest(id), route, tc.requested)
			if status != tc.want {
				t.Fatalf("status = %d, want %d", status, tc.want)
			}
			record, ok := app.Store.Get(context.Background(), "scheduler-quota-service:reservations", id)
			if !ok {
				t.Fatalf("reservation %s missing", id)
			}
			if record.Data["state"] != tc.wantState {
				t.Fatalf("state = %v, want %s", record.Data["state"], tc.wantState)
			}
		})
	}

	status, _ := app.handleReservationTransition(testHTTPRequest("missing"), route, "committed")
	if status != http.StatusNotFound {
		t.Fatalf("missing reservation status = %d, want 404", status)
	}
}

func TestHandleReservationTransitionPublishesStateEvents(t *testing.T) {
	app := NewApp(Config{ServiceName: "all", HTTPAddr: ":0"})
	route := RouteSpec{IDParam: "reservationId"}
	_, _ = app.Store.Create(context.Background(), "scheduler-quota-service:reservations", map[string]any{"id": "commit-res", "state": "reserved"})
	status, _ := app.handleReservationTransition(testHTTPRequest("commit-res"), route, "committed")
	if status != http.StatusOK {
		t.Fatalf("commit status = %d, want 200", status)
	}
	assertPlatformEventCount(t, app, "QuotaCommitted", 1)
	status, _ = app.handleReservationTransition(testHTTPRequest("commit-res"), route, "committed")
	if status != http.StatusOK {
		t.Fatalf("same-state commit status = %d, want 200", status)
	}
	assertPlatformEventCount(t, app, "QuotaCommitted", 1)

	_, _ = app.Store.Create(context.Background(), "scheduler-quota-service:reservations", map[string]any{"id": "release-res", "state": "reserved"})
	status, _ = app.handleReservationTransition(testHTTPRequest("release-res"), route, "released")
	if status != http.StatusOK {
		t.Fatalf("release status = %d, want 200", status)
	}
	assertPlatformEventCount(t, app, "QuotaReleased", 1)
	status, _ = app.handleReservationTransition(testHTTPRequest("release-res"), route, "committed")
	if status != http.StatusConflict {
		t.Fatalf("released->committed status = %d, want 409", status)
	}
	assertPlatformEventCount(t, app, "QuotaCommitted", 1)
}

func TestHandleCRUDPostDuplicateIDConflictHasNoMutationOrEvent(t *testing.T) {
	app := NewApp(Config{ServiceName: "all", HTTPAddr: ":0"})
	route := RouteSpec{Resource: "test-service:records", Action: "create"}
	_, _ = app.Store.Create(context.Background(), route.Resource, map[string]any{"id": "r1", "name": "original"})
	outboxBefore := len(app.Events.Outbox())

	status, _ := app.handleCRUD(testJSONHTTPRequest(http.MethodPost, `{"id":"r1","name":"replacement"}`), route)
	if status != http.StatusConflict {
		t.Fatalf("duplicate POST status = %d, want 409", status)
	}
	record, ok := app.Store.Get(context.Background(), route.Resource, "r1")
	if !ok || record.Data["name"] != "original" {
		t.Fatalf("duplicate POST mutated record = %#v, found=%v", record.Data, ok)
	}
	if got := len(app.Events.Outbox()); got != outboxBefore {
		t.Fatalf("outbox length = %d, want unchanged %d", got, outboxBefore)
	}
}

func TestHandleCRUDUpsertFallbackDuplicateIDConflictHasNoMutationOrEvent(t *testing.T) {
	app := NewApp(Config{ServiceName: "all", HTTPAddr: ":0"})
	route := RouteSpec{Resource: "test-service:records", Action: "update"}
	originalHook := beforeCRUDFallbackCreate
	beforeCRUDFallbackCreate = func(app *App, r *httpRequest, route RouteSpec, payload map[string]any) {
		_, _ = app.Store.Create(r.Context(), route.Resource, map[string]any{"id": payload["id"], "name": "concurrent"})
	}
	defer func() { beforeCRUDFallbackCreate = originalHook }()
	outboxBefore := len(app.Events.Outbox())

	status, _ := app.handleCRUD(testJSONHTTPRequest(http.MethodPut, `{"id":"r-race","name":"replacement"}`), route)
	if status != http.StatusConflict {
		t.Fatalf("duplicate fallback PUT status = %d, want 409", status)
	}
	record, ok := app.Store.Get(context.Background(), route.Resource, "r-race")
	if !ok || record.Data["name"] != "concurrent" {
		t.Fatalf("fallback duplicate mutated record = %#v, found=%v", record.Data, ok)
	}
	if got := len(app.Events.Outbox()); got != outboxBefore {
		t.Fatalf("outbox length = %d, want unchanged %d", got, outboxBefore)
	}
}

func TestHandleCommandDuplicateIDConflictHasNoMutationOrEvent(t *testing.T) {
	app := NewApp(Config{ServiceName: "all", HTTPAddr: ":0"})
	route := RouteSpec{Resource: "workload-service:jobs", Action: "submit"}
	resource := route.Resource + ":commands"
	_, _ = app.Store.Create(context.Background(), resource, map[string]any{"id": "cmd-dup", "name": "original"})
	outboxBefore := len(app.Events.Outbox())

	status, _ := app.handleCommand(testJSONHTTPRequest(http.MethodPost, `{"id":"cmd-dup","name":"replacement"}`), route)
	if status != http.StatusConflict {
		t.Fatalf("duplicate command status = %d, want 409", status)
	}
	record, ok := app.Store.Get(context.Background(), resource, "cmd-dup")
	if !ok || record.Data["name"] != "original" {
		t.Fatalf("duplicate command mutated record = %#v, found=%v", record.Data, ok)
	}
	if got := len(app.Events.Outbox()); got != outboxBefore {
		t.Fatalf("outbox length = %d, want unchanged %d", got, outboxBefore)
	}
}

func TestHandleConfigCommitDuplicateIDConflictHasNoMutationOrEvent(t *testing.T) {
	app := NewApp(Config{ServiceName: "all", HTTPAddr: ":0"})
	route := RouteSpec{Resource: "workload-service:configfiles", Action: "commit"}
	resource := route.Resource + ":versions"
	_, _ = app.Store.Create(context.Background(), resource, map[string]any{"id": "ver-dup", "content": "original"})
	outboxBefore := len(app.Events.Outbox())

	status, _ := app.handleConfigCommit(testJSONHTTPRequest(http.MethodPost, `{"id":"ver-dup","content":"replacement"}`), route)
	if status != http.StatusConflict {
		t.Fatalf("duplicate config commit status = %d, want 409", status)
	}
	record, ok := app.Store.Get(context.Background(), resource, "ver-dup")
	if !ok || record.Data["content"] != "original" {
		t.Fatalf("duplicate config commit mutated record = %#v, found=%v", record.Data, ok)
	}
	if got := len(app.Events.Outbox()); got != outboxBefore {
		t.Fatalf("outbox length = %d, want unchanged %d", got, outboxBefore)
	}
}

func TestHandleReservationDuplicateIDConflictHasNoMutationOrEvent(t *testing.T) {
	app := NewApp(Config{ServiceName: "all", HTTPAddr: ":0"})
	route := RouteSpec{Resource: "scheduler-quota-service:reservations", Action: "reserve"}
	_, _ = app.Store.Create(context.Background(), route.Resource, map[string]any{"id": "res-dup", "state": "original"})
	outboxBefore := len(app.Events.Outbox())

	status, _ := app.handleReservation(testJSONHTTPRequest(http.MethodPost, `{"id":"res-dup"}`), route, "reserved")
	if status != http.StatusConflict {
		t.Fatalf("duplicate reservation status = %d, want 409", status)
	}
	record, ok := app.Store.Get(context.Background(), route.Resource, "res-dup")
	if !ok || record.Data["state"] != "original" {
		t.Fatalf("duplicate reservation mutated record = %#v, found=%v", record.Data, ok)
	}
	if got := len(app.Events.Outbox()); got != outboxBefore {
		t.Fatalf("outbox length = %d, want unchanged %d", got, outboxBefore)
	}
}

func testHTTPRequest(reservationID string) *httpRequest {
	req := httptest.NewRequest(http.MethodPost, "/reservation/"+reservationID, nil)
	req.SetPathValue("reservationId", reservationID)
	return &httpRequest{Request: req, Service: "test", TraceID: "trace"}
}

func testJSONHTTPRequest(method, body string) *httpRequest {
	req := httptest.NewRequest(method, "/", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	return &httpRequest{Request: req, Service: "test", TraceID: "trace", IdempotencyKey: "test-key"}
}

func assertPlatformEventCount(t *testing.T, app *App, name string, want int) {
	t.Helper()
	got := 0
	for _, event := range app.Events.Outbox() {
		if event.Name == name {
			got++
		}
	}
	if got != want {
		t.Fatalf("%s event count = %d, want %d", name, got, want)
	}
}

func routeEchoHandler(_ *App, r *http.Request, route RouteSpec) (int, any, *Degraded) {
	return http.StatusOK, map[string]any{
		"resource": route.Resource,
		"id":       r.PathValue("id"),
	}, nil
}

func responseData(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var env struct {
		Data map[string]any `json:"data"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&env); err != nil {
		t.Fatal(err)
	}
	return env.Data
}
