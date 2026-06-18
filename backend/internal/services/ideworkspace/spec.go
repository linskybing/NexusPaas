package ideworkspace

import (
	"net/http"

	"github.com/linskybing/nexuspaas/backend/internal/platform"
	"github.com/linskybing/nexuspaas/backend/internal/services/shared"
)

func Spec() platform.ServiceSpec {
	route, id, adapter := shared.Route, shared.ID, shared.Adapter
	return platform.ServiceSpec{
		Name:        "ide-service",
		Category:    "compute",
		Phase:       "5",
		Description: "Interactive workspace lifecycle, IDE proxy, activity tracking, idle reaping, and image listing.",
		Tables:      []string{"ide_sessions", "workspace_activity", "pod_mappings", "ide_identity_roles", "ide_identity_users", "ide_policy_roles", "ide_project_members", "ide_projects", "ide_user_groups", "outbox", "inbox"},
		Events:      []string{"IDEStarted", "IDEStopped", "IDEDeleted", "IDEIdleReaped"},
		Routes: []platform.RouteSpec{
			route(http.MethodGet, "/api/v1/ide", "ide_sessions", "list"),
			route(http.MethodPost, "/api/v1/ide", "ide_sessions", "command", adapter("k8s")),
			route(http.MethodGet, "/api/v1/ide/images", "ide_images", "list"),
			route(http.MethodPost, "/api/v1/ide/start", "ide_sessions", "command"),
			route(http.MethodPost, "/api/v1/ide/stop", "ide_sessions", "command"),
			route(http.MethodPost, "/api/v1/ide/delete", "ide_sessions", "command"),
			route(http.MethodPost, "/api/v1/ide/{id}/stop", "ide_sessions", "command", id("id"), adapter("k8s")),
			route(http.MethodDelete, "/api/v1/ide/{id}", "ide_sessions", "command", id("id"), adapter("k8s")),
			route(http.MethodPost, "/api/v1/ide/{id}/activity", "ide_activity", "create", id("id")),
			route(http.MethodPost, "/api/v1/ide/reap-idle", "ide_sessions", "command"),
			route(http.MethodGet, "/api/v1/ide/proxy/{podName}/{path...}", "ide_proxy", "proxy", id("podName"), adapter("k8s")),
		},
	}
}
