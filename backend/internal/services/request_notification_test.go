package services

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/linskybing/nexuspaas/backend/internal/platform"
)

func TestRequestNotificationFormWorkflow(t *testing.T) {
	app := newTestApp()

	requestJSON(t, app, http.MethodPost, "/api/v1/forms", `{"title":"anon","description":"missing identity"}`, nil, http.StatusUnauthorized)
	requestJSON(t, app, http.MethodPost, "/api/v1/forms", `{"title":"missing description"}`, userHeaders("u1"), http.StatusBadRequest)
	requestJSON(t, app, http.MethodPost, "/api/v1/forms", `{"project_id":"p1","title":"Project","description":"no membership"}`, userHeaders("u1"), http.StatusForbidden)
	_, _ = app.Store.Create(httptest.NewRequest(http.MethodGet, "/", nil).Context(), "org-project-service:projects", map[string]any{"id": "p1", "owner_id": "g1"})
	_, _ = app.Store.Create(httptest.NewRequest(http.MethodGet, "/", nil).Context(), "org-project-service:user_groups", map[string]any{"id": "m1", "user_id": "u1", "group_id": "g1", "role": "member"})
	projectForm := responseMap(t, requestJSON(t, app, http.MethodPost, "/api/v1/forms", `{"project_id":"p1","title":"Project","description":"member access"}`, userHeaders("u1"), http.StatusCreated))
	if projectForm["project_id"] != "p1" {
		t.Fatalf("project form = %#v, want project_id p1", projectForm)
	}
	form1 := createTestForm(t, app, "u1", "Need GPU", "please review quota")
	form2 := createTestForm(t, app, "u2", "Question", "how do I access storage?")

	if form1["status"] != "Pending" || form1["user_id"] != "u1" {
		t.Fatalf("created form = %#v, want Pending form owned by u1", form1)
	}

	myForms := responseSlice(t, requestJSON(t, app, http.MethodGet, "/api/v1/forms/my", "", userHeaders("u1"), http.StatusOK))
	if len(myForms) != 2 || myForms[1].(map[string]any)["id"] != form1["id"] {
		t.Fatalf("my forms = %#v, want only first user's forms", myForms)
	}
	pagedMyForms := responseMap(t, requestJSON(t, app, http.MethodGet, "/api/v1/forms/my?page=1&limit=1", "", userHeaders("u1"), http.StatusOK))
	if pagedMyForms["total"] != float64(2) || pagedMyForms["page_size"] != float64(1) || len(pagedMyForms["list"].([]any)) != 1 {
		t.Fatalf("paged my forms = %#v, want one-item page with total 2", pagedMyForms)
	}

	requestJSON(t, app, http.MethodGet, "/api/v1/forms", "", userHeaders("u1"), http.StatusForbidden)
	allForms := responseSlice(t, requestJSON(t, app, http.MethodGet, "/api/v1/forms", "", adminHeaders("admin"), http.StatusOK))
	if len(allForms) != 3 {
		t.Fatalf("admin form list length = %d, want 3", len(allForms))
	}

	formPath := "/api/v1/forms/" + form1["id"].(string)
	requestJSON(t, app, http.MethodGet, formPath, "", userHeaders("u2"), http.StatusForbidden)
	requestJSON(t, app, http.MethodGet, "/api/v1/forms/"+form2["id"].(string), "", userHeaders("u2"), http.StatusOK)

	messagePath := formPath + "/messages"
	message := responseMap(t, requestJSON(t, app, http.MethodPost, messagePath, `{"content":"any update?"}`, userHeaders("u1"), http.StatusOK))
	if message["content"] != "any update?" {
		t.Fatalf("message = %#v, want created message content", message)
	}
	requestJSON(t, app, http.MethodPost, messagePath, `{"content":"not mine"}`, userHeaders("u2"), http.StatusForbidden)

	statusPath := formPath + "/status"
	requestJSON(t, app, http.MethodPut, statusPath, `{"status":"Completed"}`, adminHeaders("admin"), http.StatusBadRequest)
	processing := responseMap(t, requestJSON(t, app, http.MethodPut, statusPath, `{"status":"Processing"}`, adminHeaders("admin"), http.StatusOK))
	if processing["status"] != "Processing" {
		t.Fatalf("status update = %#v, want Processing", processing)
	}
	completed := responseMap(t, requestJSON(t, app, http.MethodPut, statusPath, `{"status":"Completed"}`, adminHeaders("admin"), http.StatusOK))
	if completed["status"] != "Completed" {
		t.Fatalf("status update = %#v, want Completed", completed)
	}
	requestJSON(t, app, http.MethodPost, messagePath, `{"content":"too late"}`, userHeaders("u1"), http.StatusBadRequest)
}

func TestRequestNotificationAnnouncementWorkflow(t *testing.T) {
	app := newTestApp()

	requestJSON(t, app, http.MethodGet, "/api/v1/announcements/active", "", nil, http.StatusUnauthorized)
	requestJSON(t, app, http.MethodPost, "/api/v1/admin/announcements", `{"title":"nope","content":"body"}`, userHeaders("u1"), http.StatusForbidden)
	requestJSON(t, app, http.MethodPost, "/api/v1/admin/announcements", `{"title":"forged","content":"body"}`, map[string]string{"X-User-ID": "u1", "X-Admin": "true"}, http.StatusForbidden)
	requestJSON(t, app, http.MethodPost, "/api/v1/admin/announcements", `{"title":"bad time","content":"body","expires_at":"not-a-time"}`, adminHeaders("admin"), http.StatusBadRequest)
	first := createAnnouncement(t, app, `{"title":"Policy","content":"body","priority":"unknown"}`)
	if first["priority"] != "info" {
		t.Fatalf("priority = %v, want default info", first["priority"])
	}

	for _, body := range []string{
		`{"title":"Pinned 1","content":"body","is_pinned":true}`,
		`{"title":"Pinned 2","content":"body","is_pinned":true}`,
		`{"title":"Pinned 3","content":"body","is_pinned":true}`,
	} {
		createAnnouncement(t, app, body)
	}
	requestJSON(t, app, http.MethodPost, "/api/v1/admin/announcements", `{"title":"Pinned 4","content":"body","is_pinned":true}`, adminHeaders("admin"), http.StatusBadRequest)

	count := responseMap(t, requestJSON(t, app, http.MethodGet, "/api/v1/announcements/unread-count", "", userHeaders("u1"), http.StatusOK))
	if count["count"] != float64(4) {
		t.Fatalf("unread count = %#v, want 4", count)
	}
	active := responseSlice(t, requestJSON(t, app, http.MethodGet, "/api/v1/announcements/active", "", userHeaders("u1"), http.StatusOK))
	if len(active) != 4 {
		t.Fatalf("active announcements length = %d, want 4", len(active))
	}

	id := first["id"].(string)
	requestJSON(t, app, http.MethodPut, "/api/v1/announcements/"+id+"/read", "", userHeaders("u1"), http.StatusOK)
	count = responseMap(t, requestJSON(t, app, http.MethodGet, "/api/v1/announcements/unread-count", "", userHeaders("u1"), http.StatusOK))
	if count["count"] != float64(3) {
		t.Fatalf("unread count after mark-read = %#v, want 3", count)
	}

	requestJSON(t, app, http.MethodGet, "/api/v1/announcements/"+id, "", userHeaders("u1"), http.StatusOK)
	adminPage := responseMap(t, requestJSON(t, app, http.MethodGet, "/api/v1/admin/announcements", "", adminHeaders("admin"), http.StatusOK))
	if adminPage["total"] != float64(4) || adminPage["page_size"] != float64(20) {
		t.Fatalf("admin announcement page = %#v, want total 4 page_size 20", adminPage)
	}

	updated := responseMap(t, requestJSON(t, app, http.MethodPut, "/api/v1/admin/announcements/"+id, `{"title":"Policy updated","content":"new","priority":"critical"}`, adminHeaders("admin"), http.StatusOK))
	if updated["title"] != "Policy updated" || updated["priority"] != "critical" {
		t.Fatalf("updated announcement = %#v", updated)
	}
	requestJSON(t, app, http.MethodDelete, "/api/v1/admin/announcements/"+id, "", adminHeaders("admin"), http.StatusOK)
	requestJSON(t, app, http.MethodGet, "/api/v1/announcements/"+id, "", userHeaders("u1"), http.StatusNotFound)
}

func TestRequestNotificationNotificationAcknowledgements(t *testing.T) {
	app := newTestApp()

	requestJSON(t, app, http.MethodPut, "/api/v1/notifications/n1/read", "", nil, http.StatusUnauthorized)
	assertNoData(t, requestJSON(t, app, http.MethodPut, "/api/v1/notifications/n1/read", "", userHeaders("u1"), http.StatusOK))
	requestJSON(t, app, http.MethodPut, "/api/v1/notifications/n2/read", "", userHeaders("u1"), http.StatusOK)
	assertNoData(t, requestJSON(t, app, http.MethodPut, "/api/v1/notifications/read-all", "", userHeaders("u1"), http.StatusOK))
	assertNoData(t, requestJSON(t, app, http.MethodDelete, "/api/v1/notifications/clear-all", "", userHeaders("u1"), http.StatusOK))
}

func TestRequestNotificationStatePersistsAcrossAppInstances(t *testing.T) {
	store := platform.NewStore()
	app1 := newTestAppWithStore(store)
	form := createTestForm(t, app1, "u1", "Durable form", "survive a restart")
	formPath := "/api/v1/forms/" + form["id"].(string)
	messagePath := formPath + "/messages"
	requestJSON(t, app1, http.MethodPost, messagePath, `{"content":"stored message"}`, userHeaders("u1"), http.StatusOK)
	announcement := createAnnouncement(t, app1, `{"title":"Durable announcement","content":"body"}`)
	announcementID := announcement["id"].(string)
	requestJSON(t, app1, http.MethodPut, "/api/v1/announcements/"+announcementID+"/read", "", userHeaders("u1"), http.StatusOK)
	requestJSON(t, app1, http.MethodPut, "/api/v1/notifications/n1/read", "", userHeaders("u1"), http.StatusOK)

	app2 := newTestAppWithStore(store)
	storedForm := responseMap(t, requestJSON(t, app2, http.MethodGet, formPath, "", userHeaders("u1"), http.StatusOK))
	if storedForm["title"] != "Durable form" {
		t.Fatalf("stored form = %#v, want persisted form", storedForm)
	}
	messages := responseSlice(t, requestJSON(t, app2, http.MethodGet, messagePath, "", userHeaders("u1"), http.StatusOK))
	if len(messages) != 1 || messages[0].(map[string]any)["content"] != "stored message" {
		t.Fatalf("stored messages = %#v, want persisted message", messages)
	}
	count := responseMap(t, requestJSON(t, app2, http.MethodGet, "/api/v1/announcements/unread-count", "", userHeaders("u1"), http.StatusOK))
	if count["count"] != float64(0) {
		t.Fatalf("unread count after app restart = %#v, want read state persisted", count)
	}
	assertNoData(t, requestJSON(t, app2, http.MethodDelete, "/api/v1/notifications/clear-all", "", userHeaders("u1"), http.StatusOK))
	if got := len(store.List(httptest.NewRequest(http.MethodGet, "/", nil).Context(), "request-notification-service:notifications")); got != 0 {
		t.Fatalf("notifications after clear through second app = %d, want 0", got)
	}
}

func TestRequestNotificationMalformedJSONDoesNotWrite(t *testing.T) {
	app := newTestApp()

	requestJSON(t, app, http.MethodPost, "/api/v1/forms", `{`, userHeaders("u1"), http.StatusBadRequest)
	if forms := responseSlice(t, requestJSON(t, app, http.MethodGet, "/api/v1/forms/my", "", userHeaders("u1"), http.StatusOK)); len(forms) != 0 {
		t.Fatalf("forms after malformed create = %#v, want none", forms)
	}

	form := createTestForm(t, app, "u1", "Need GPU", "please review quota")
	formPath := "/api/v1/forms/" + form["id"].(string)
	statusPath := formPath + "/status"
	requestJSON(t, app, http.MethodPut, statusPath, `{`, adminHeaders("admin"), http.StatusBadRequest)
	requestJSON(t, app, http.MethodPut, "/api/v1/forms/batch/status", `{`, adminHeaders("admin"), http.StatusBadRequest)
	current := responseMap(t, requestJSON(t, app, http.MethodGet, formPath, "", userHeaders("u1"), http.StatusOK))
	if current["status"] != "Pending" {
		t.Fatalf("form after malformed status update = %#v, want Pending", current)
	}

	messagePath := formPath + "/messages"
	requestJSON(t, app, http.MethodPost, messagePath, `{`, userHeaders("u1"), http.StatusBadRequest)
	if messages := responseSlice(t, requestJSON(t, app, http.MethodGet, messagePath, "", userHeaders("u1"), http.StatusOK)); len(messages) != 0 {
		t.Fatalf("messages after malformed create = %#v, want none", messages)
	}

	requestJSON(t, app, http.MethodPost, "/api/v1/admin/announcements", `{`, adminHeaders("admin"), http.StatusBadRequest)
	adminPage := responseMap(t, requestJSON(t, app, http.MethodGet, "/api/v1/admin/announcements", "", adminHeaders("admin"), http.StatusOK))
	if adminPage["total"] != float64(0) {
		t.Fatalf("announcements after malformed create = %#v, want none", adminPage)
	}

	announcement := createAnnouncement(t, app, `{"title":"Policy","content":"body"}`)
	id := announcement["id"].(string)
	requestJSON(t, app, http.MethodPut, "/api/v1/admin/announcements/"+id, `{`, adminHeaders("admin"), http.StatusBadRequest)
	currentAnnouncement := responseMap(t, requestJSON(t, app, http.MethodGet, "/api/v1/announcements/"+id, "", userHeaders("u1"), http.StatusOK))
	if currentAnnouncement["title"] != "Policy" || currentAnnouncement["content"] != "body" {
		t.Fatalf("announcement after malformed update = %#v, want original content", currentAnnouncement)
	}
}

func createTestForm(t *testing.T, app http.Handler, userID, title, description string) map[string]any {
	t.Helper()
	body := `{"title":` + quoteJSON(t, title) + `,"description":` + quoteJSON(t, description) + `,"tag":"feature"}`
	return responseMap(t, requestJSON(t, app, http.MethodPost, "/api/v1/forms", body, userHeaders(userID), http.StatusCreated))
}

func createAnnouncement(t *testing.T, app http.Handler, body string) map[string]any {
	t.Helper()
	return responseMap(t, requestJSON(t, app, http.MethodPost, "/api/v1/admin/announcements", body, adminHeaders("admin"), http.StatusCreated))
}

func newTestAppWithStore(store platform.RecordStore) *platform.App {
	app := platform.NewApp(platform.Config{ServiceName: "all", HTTPAddr: ":0", APIKeys: map[string]bool{"test-key": true}, ExternalURLs: map[string]string{}}, platform.WithStore(store))
	RegisterAll(app)
	return app
}

func requestJSON(t *testing.T, app http.Handler, method, path, body string, headers map[string]string, want int) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Idempotency-Key", "test-"+method+"-"+path)
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	app.ServeHTTP(rec, req)
	if rec.Code != want {
		t.Fatalf("%s %s returned %d, want %d: %s", method, path, rec.Code, want, rec.Body.String())
	}
	return rec
}

func responseMap(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var data map[string]any
	decodeResponseData(t, rec, &data)
	return data
}

func responseSlice(t *testing.T, rec *httptest.ResponseRecorder) []any {
	t.Helper()
	var data []any
	decodeResponseData(t, rec, &data)
	return data
}

func decodeResponseData(t *testing.T, rec *httptest.ResponseRecorder, out any) {
	t.Helper()
	var env struct {
		Success bool            `json:"success"`
		Data    json.RawMessage `json:"data"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&env); err != nil {
		t.Fatal(err)
	}
	if !env.Success {
		t.Fatalf("response was not successful: %s", string(env.Data))
	}
	if err := json.Unmarshal(env.Data, out); err != nil {
		t.Fatal(err)
	}
}

func assertNoData(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()
	var env struct {
		Success bool            `json:"success"`
		Data    json.RawMessage `json:"data"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&env); err != nil {
		t.Fatal(err)
	}
	if !env.Success {
		t.Fatal("response was not successful")
	}
	if len(env.Data) != 0 && string(env.Data) != "null" {
		t.Fatalf("data = %s, want omitted or null", string(env.Data))
	}
}

func userHeaders(userID string) map[string]string {
	return map[string]string{"X-User-ID": userID, "X-Username": userID}
}

func adminHeaders(userID string) map[string]string {
	headers := userHeaders(userID)
	headers["X-Admin"] = "true"
	headers["X-User-Role"] = "admin"
	return headers
}

func quoteJSON(t *testing.T, value string) string {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}
