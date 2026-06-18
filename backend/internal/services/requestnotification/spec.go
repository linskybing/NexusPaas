package requestnotification

import (
	"net/http"

	"github.com/linskybing/nexuspaas/backend/internal/platform"
	"github.com/linskybing/nexuspaas/backend/internal/services/shared"
)

func Spec() platform.ServiceSpec {
	route, id, admin := shared.Route, shared.ID, shared.Admin
	return platform.ServiceSpec{
		Name:        "request-notification-service",
		Category:    "collaboration",
		Phase:       "1",
		Description: "Forms, form messages, notifications, announcements, unread counts, and mark-read state.",
		Tables:      []string{"forms", "form_messages", "notifications", "announcements", "announcement_reads", "project_access_projects", "project_access_members", "project_access_user_groups", "outbox", "inbox"},
		Events:      []string{"FormCreated", "FormUpdated", "NotificationRequested", "AnnouncementPublished"},
		Routes: []platform.RouteSpec{
			route(http.MethodGet, "/api/v1/forms", "forms", "list"),
			route(http.MethodPost, "/api/v1/forms", "forms", "create"),
			route(http.MethodGet, "/api/v1/forms/my", "forms", "list_my"),
			route(http.MethodGet, "/api/v1/forms/{id}", "forms", "get", id("id")),
			route(http.MethodPut, "/api/v1/forms/{id}", "forms", "update", id("id"), admin()),
			route(http.MethodPut, "/api/v1/forms/{id}/status", "forms", "update", id("id"), admin()),
			route(http.MethodPut, "/api/v1/forms/batch/status", "forms", "batch_status", admin()),
			route(http.MethodGet, "/api/v1/forms/{id}/messages", "form_messages", "list", id("id")),
			route(http.MethodPost, "/api/v1/forms/{id}/messages", "form_messages", "create", id("id")),
			route(http.MethodPut, "/api/v1/notifications/{id}/read", "notifications", "update", id("id")),
			route(http.MethodPut, "/api/v1/notifications/read-all", "notifications", "update"),
			route(http.MethodDelete, "/api/v1/notifications/clear-all", "notifications", "delete"),
			route(http.MethodGet, "/api/v1/announcements/active", "announcements", "list"),
			route(http.MethodGet, "/api/v1/announcements/unread-count", "announcement_reads", "list"),
			route(http.MethodGet, "/api/v1/announcements/{id}", "announcements", "get", id("id")),
			route(http.MethodPut, "/api/v1/announcements/{id}/read", "announcement_reads", "create", id("id")),
			route(http.MethodGet, "/api/v1/admin/announcements", "announcements", "list", admin()),
			route(http.MethodPost, "/api/v1/admin/announcements", "announcements", "create", admin()),
			route(http.MethodPut, "/api/v1/admin/announcements/{id}", "announcements", "update", id("id"), admin()),
			route(http.MethodDelete, "/api/v1/admin/announcements/{id}", "announcements", "delete", id("id"), admin()),
		},
	}
}
