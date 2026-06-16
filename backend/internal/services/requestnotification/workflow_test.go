package requestnotification

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/linskybing/nexuspaas/backend/internal/platform"
)

func TestRequestNotificationFormAndMessageHandlers(t *testing.T) {
	app := newRequestNotificationTestApp(t)
	service := NewService()

	code, data, _ := service.createForm(app, rnRequest(http.MethodPost, "/api/v1/forms", `{"project_id":"P1","title":"Need GPU","description":"training job","tag":"resource"}`, "U1", false), platform.RouteSpec{})
	assertRNStatus(t, code, data, http.StatusCreated)
	form := data.(Form)
	if form.ProjectID == nil || *form.ProjectID != "P1" || form.Status != "Pending" {
		t.Fatalf("created form = %#v, want project-linked pending form", form)
	}

	code, data, _ = service.listMyForms(app, rnRequest(http.MethodGet, "/api/v1/forms/my?page=1&limit=1", "", "U1", false), platform.RouteSpec{})
	assertRNStatus(t, code, data, http.StatusOK)
	if page := data.(map[string]any); page["total"] != int64(1) || len(page["list"].([]Form)) != 1 {
		t.Fatalf("my forms page = %#v, want one form", page)
	}
	code, data, _ = service.listAllForms(app, rnRequest(http.MethodGet, "/api/v1/forms", "", "ADMIN", true), platform.RouteSpec{})
	assertRNStatus(t, code, data, http.StatusOK)
	if len(data.([]Form)) != 1 {
		t.Fatalf("all forms = %#v, want one form", data)
	}

	getReq := rnRequest(http.MethodGet, "/api/v1/forms/"+form.ID, "", "U1", false)
	getReq.SetPathValue("id", form.ID)
	code, data, _ = service.getForm(app, getReq, platform.RouteSpec{})
	assertRNStatus(t, code, data, http.StatusOK)

	msgReq := rnRequest(http.MethodPost, "/api/v1/forms/"+form.ID+"/messages", `{"content":"any update?"}`, "U1", false)
	msgReq.SetPathValue("id", form.ID)
	code, data, _ = service.createMessage(app, msgReq, platform.RouteSpec{})
	assertRNStatus(t, code, data, http.StatusOK)
	if data.(FormMessage).Content != "any update?" {
		t.Fatalf("message = %#v, want content preserved", data)
	}
	listMsgReq := rnRequest(http.MethodGet, "/api/v1/forms/"+form.ID+"/messages", "", "U1", false)
	listMsgReq.SetPathValue("id", form.ID)
	code, data, _ = service.listMessages(app, listMsgReq, platform.RouteSpec{})
	assertRNStatus(t, code, data, http.StatusOK)
	if len(data.([]FormMessage)) != 1 {
		t.Fatalf("messages = %#v, want one message", data)
	}

	statusReq := rnRequest(http.MethodPut, "/api/v1/forms/"+form.ID+"/status", `{"status":"Processing"}`, "ADMIN", true)
	statusReq.SetPathValue("id", form.ID)
	code, data, _ = service.updateFormStatus(app, statusReq, platform.RouteSpec{})
	assertRNStatus(t, code, data, http.StatusOK)
	if data.(Form).Status != "Processing" {
		t.Fatalf("updated form = %#v, want Processing", data)
	}
	batchReq := rnRequest(http.MethodPut, "/api/v1/forms/batch/status", `{"ids":["`+form.ID+`"],"status":"Completed"}`, "ADMIN", true)
	code, data, _ = service.batchUpdateFormStatus(app, batchReq, platform.RouteSpec{})
	assertRNStatus(t, code, data, http.StatusOK)
	if result := data.(map[string]any); result["succeeded"] != 1 || result["failed"] != 0 {
		t.Fatalf("batch status result = %#v, want one success", result)
	}
}

func TestRequestNotificationAnnouncementHandlers(t *testing.T) {
	app := newRequestNotificationTestApp(t)
	service := NewService()

	first := createAnnouncementForTest(t, app, service, `{"title":"Policy","content":"body","priority":"unknown"}`)
	if first.Priority != "info" {
		t.Fatalf("priority = %q, want info default", first.Priority)
	}
	for _, title := range []string{"Pinned 1", "Pinned 2", "Pinned 3"} {
		createAnnouncementForTest(t, app, service, `{"title":"`+title+`","content":"body","is_pinned":true}`)
	}
	code, data, _ := service.createAnnouncement(app, rnRequest(http.MethodPost, "/api/v1/admin/announcements", `{"title":"Pinned 4","content":"body","is_pinned":true}`, "ADMIN", true), platform.RouteSpec{})
	assertRNStatus(t, code, data, http.StatusBadRequest)

	code, data, _ = service.unreadCount(app, rnRequest(http.MethodGet, "/api/v1/announcements/unread-count", "", "U1", false), platform.RouteSpec{})
	assertRNStatus(t, code, data, http.StatusOK)
	if data.(map[string]int64)["count"] != 4 {
		t.Fatalf("unread count = %#v, want four active announcements", data)
	}
	code, data, _ = service.listActiveAnnouncements(app, rnRequest(http.MethodGet, "/api/v1/announcements/active", "", "U1", false), platform.RouteSpec{})
	assertRNStatus(t, code, data, http.StatusOK)
	if len(data.([]ActiveAnnouncement)) != 4 {
		t.Fatalf("active announcements = %#v, want four", data)
	}

	readReq := rnRequest(http.MethodPut, "/api/v1/announcements/"+first.ID+"/read", "", "U1", false)
	readReq.SetPathValue("id", first.ID)
	code, data, _ = service.markAnnouncementRead(app, readReq, platform.RouteSpec{})
	assertRNStatus(t, code, data, http.StatusOK)
	code, data, _ = service.unreadCount(app, rnRequest(http.MethodGet, "/api/v1/announcements/unread-count", "", "U1", false), platform.RouteSpec{})
	assertRNStatus(t, code, data, http.StatusOK)
	if data.(map[string]int64)["count"] != 3 {
		t.Fatalf("unread count after read = %#v, want three", data)
	}

	getReq := rnRequest(http.MethodGet, "/api/v1/announcements/"+first.ID, "", "U1", false)
	getReq.SetPathValue("id", first.ID)
	code, data, _ = service.getAnnouncement(app, getReq, platform.RouteSpec{})
	assertRNStatus(t, code, data, http.StatusOK)

	adminReq := rnRequest(http.MethodGet, "/api/v1/admin/announcements?page=1&limit=500&search=policy", "", "ADMIN", true)
	code, data, _ = service.listAnnouncements(app, adminReq, platform.RouteSpec{})
	assertRNStatus(t, code, data, http.StatusOK)
	if page := data.(map[string]any); page["total"] != 1 || page["page_size"] != 20 {
		t.Fatalf("admin announcement page = %#v, want search hit with capped limit", page)
	}

	updateReq := rnRequest(http.MethodPut, "/api/v1/admin/announcements/"+first.ID, `{"title":"Policy updated","content":"new","priority":"critical"}`, "ADMIN", true)
	updateReq.SetPathValue("id", first.ID)
	code, data, _ = service.updateAnnouncement(app, updateReq, platform.RouteSpec{})
	assertRNStatus(t, code, data, http.StatusOK)
	if data.(Announcement).Priority != "critical" {
		t.Fatalf("updated announcement = %#v, want critical priority", data)
	}

	deleteReq := rnRequest(http.MethodDelete, "/api/v1/admin/announcements/"+first.ID, "", "ADMIN", true)
	deleteReq.SetPathValue("id", first.ID)
	code, data, _ = service.deleteAnnouncement(app, deleteReq, platform.RouteSpec{})
	assertRNStatus(t, code, data, http.StatusOK)
	code, data, _ = service.getAnnouncement(app, getReq, platform.RouteSpec{})
	assertRNStatus(t, code, data, http.StatusNotFound)
}

func TestRequestNotificationNotificationHandlers(t *testing.T) {
	app := newRequestNotificationTestApp(t)
	service := NewService()

	missingIDReq := rnRequest(http.MethodPut, "/api/v1/notifications//read", "", "U1", false)
	code, data, _ := service.markNotificationRead(app, missingIDReq, platform.RouteSpec{})
	assertRNStatus(t, code, data, http.StatusBadRequest)

	for _, id := range []string{"n1", "n2"} {
		req := rnRequest(http.MethodPut, "/api/v1/notifications/"+id+"/read", "", "U1", false)
		req.SetPathValue("id", id)
		code, data, _ = service.markNotificationRead(app, req, platform.RouteSpec{})
		assertRNStatus(t, code, data, http.StatusOK)
	}
	code, data, _ = service.markAllNotificationsRead(app, rnRequest(http.MethodPut, "/api/v1/notifications/read-all", "", "U1", false), platform.RouteSpec{})
	assertRNStatus(t, code, data, http.StatusOK)
	code, data, _ = service.clearNotifications(app, rnRequest(http.MethodDelete, "/api/v1/notifications/clear-all", "", "U1", false), platform.RouteSpec{})
	assertRNStatus(t, code, data, http.StatusOK)
	if got := len(app.Store.List(context.Background(), notificationsResource)); got != 0 {
		t.Fatalf("notifications after clear = %d, want none", got)
	}
}

func TestRequestNotificationGuardAndHelperBranches(t *testing.T) {
	app := newRequestNotificationTestApp(t)
	service := NewService()

	code, data, _ := service.createForm(app, rnRequest(http.MethodPost, "/api/v1/forms", `{`, "U1", false), platform.RouteSpec{})
	assertRNStatus(t, code, data, http.StatusBadRequest)
	code, data, _ = service.createForm(app, rnRequest(http.MethodPost, "/api/v1/forms", `{"title":"missing description"}`, "U1", false), platform.RouteSpec{})
	assertRNStatus(t, code, data, http.StatusBadRequest)
	code, data, _ = service.createAnnouncement(app, rnRequest(http.MethodPost, "/api/v1/admin/announcements", `{"title":"bad","content":"body","expires_at":"not-a-time"}`, "ADMIN", true), platform.RouteSpec{})
	assertRNStatus(t, code, data, http.StatusBadRequest)
	if _, ok := stringSlice([]any{"a", 2, "b"}); !ok {
		t.Fatal("stringSlice rejected mixed array")
	}
	if parsePositiveInt("bad", 7) != 7 || parsePositiveInt("0", 7) != 7 {
		t.Fatal("parsePositiveInt fallback failed")
	}
	if systemRole(rnRequest(http.MethodGet, "/", "", "ADMIN", true)) != 1 {
		t.Fatal("admin system role = 0, want 1")
	}
}

func newRequestNotificationTestApp(t *testing.T) *platform.App {
	t.Helper()
	app := platform.NewApp(platform.Config{ServiceName: "all", HTTPAddr: ":0"})
	Register(app)
	createRNRecords(t, app, orgProjectsResource, []map[string]any{
		{"id": "P1", "owner_id": "G1", "name": "vision"},
	})
	createRNRecords(t, app, orgUserGroupsResource, []map[string]any{
		{"id": "UG1", "group_id": "G1", "user_id": "U1", "role": "member"},
	})
	return app
}

func createRNRecords(t *testing.T, app *platform.App, resource string, rows []map[string]any) {
	t.Helper()
	for _, row := range rows {
		if _, err := app.Store.Create(context.Background(), resource, row); err != nil {
			t.Fatal(err)
		}
	}
}

func rnRequest(method, target, body, userID string, admin bool) *http.Request {
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if userID != "" {
		req.Header.Set("X-User-ID", userID)
		req.Header.Set("X-Username", strings.ToLower(userID))
	}
	if admin {
		req.Header.Set("X-User-Role", "admin")
	}
	return req
}

func createAnnouncementForTest(t *testing.T, app *platform.App, service *Service, body string) Announcement {
	t.Helper()
	code, data, _ := service.createAnnouncement(app, rnRequest(http.MethodPost, "/api/v1/admin/announcements", body, "ADMIN", true), platform.RouteSpec{})
	assertRNStatus(t, code, data, http.StatusCreated)
	return data.(Announcement)
}

func assertRNStatus(t *testing.T, code int, data any, want int) {
	t.Helper()
	if code != want {
		t.Fatalf("status=%d data=%#v, want %d", code, data, want)
	}
}
