package integrationproxy

import (
	"net/http"

	"github.com/linskybing/nexuspaas/backend/internal/platform"
	"github.com/linskybing/nexuspaas/backend/internal/services/shared"
)

func Spec() platform.ServiceSpec {
	route, id, admin, adapter := shared.Route, shared.ID, shared.Admin, shared.Adapter
	return platform.ServiceSpec{
		Name:        "integration-proxy-service",
		Category:    "edge-tools",
		Phase:       "2",
		Description: "External tool UI proxies, SSO callbacks, proxy auth-check, and VPN administration.",
		Tables:      []string{"proxy_sessions", "proxy_cache", "vpn_clients", "vpn_usage_sessions", "admin_users", "admin_roles", "outbox", "inbox"},
		Events:      []string{"ProxySessionStarted", "ProxySessionTerminated"},
		Routes: []platform.RouteSpec{
			route(http.MethodGet, "/api/v1/grafana/{path...}", "grafana_proxy", "proxy", adapter("prometheus")),
			route(http.MethodGet, "/api/v1/minio-console-sso", "minio_sso", "proxy"),
			route(http.MethodGet, "/api/v1/minio-console/{path...}", "minio_proxy", "proxy", adapter("minio")),
			route(http.MethodGet, "/api/v1/pgadmin-sso", "pgadmin_sso", "proxy"),
			route(http.MethodGet, "/api/v1/pgadmin-auth-check", "pgadmin_auth_check", "proxy"),
			route(http.MethodGet, "/api/v1/pgadmin/{path...}", "pgadmin_proxy", "proxy", adapter("pgadmin")),
			route(http.MethodGet, "/api/v1/longhorn/{path...}", "longhorn_proxy", "proxy", adapter("longhorn")),
			route(http.MethodGet, "/api/v1/harbor/{path...}", "harbor_ui_proxy", "proxy", adapter("harbor")),
			route(http.MethodGet, "/api/v1/admin/vpn", "vpn_clients", "list", admin()),
			route(http.MethodGet, "/api/v1/admin/vpn/clients", "vpn_clients", "list", admin()),
			route(http.MethodGet, "/api/v1/admin/vpn/usage", "vpn_usage", "list", admin()),
			route(http.MethodDelete, "/api/v1/admin/vpn/clients/{cn}", "vpn_clients", "command", id("cn"), admin()),
			route(http.MethodPost, "/api/v1/admin/vpn/{id}/disconnect", "vpn_clients", "command", id("id"), admin()),
		},
	}
}
