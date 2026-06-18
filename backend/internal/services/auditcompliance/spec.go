package auditcompliance

import (
	"net/http"

	"github.com/linskybing/nexuspaas/backend/internal/platform"
	"github.com/linskybing/nexuspaas/backend/internal/services/shared"
)

func Spec() platform.ServiceSpec {
	route, admin := shared.Route, shared.Admin
	return platform.ServiceSpec{
		Name:        "audit-compliance-service",
		Category:    "ops",
		Phase:       "1",
		Description: "Audit event ingestion, audit logs, project audit reports, security posture, retention, and cleanup.",
		Tables:      []string{"audit_logs", "security_findings", "security_reports", "event_ingestion_offsets", "project_report_members", "outbox", "inbox"},
		Events:      []string{"AuditEvent"},
		Routes: []platform.RouteSpec{
			route(http.MethodPost, "/api/v1/audit/events", "audit_events", "event_ingest"),
			route(http.MethodGet, "/api/v1/audit/logs", "audit_logs", "list", admin()),
			route(http.MethodGet, "/api/v1/audit/report", "audit_reports", "list"),
			route(http.MethodGet, "/api/v1/admin/security/posture", "security_posture", "list", admin()),
			route(http.MethodPost, "/api/v1/internal/audit/cleanup", "audit_retention", "command", admin()),
		},
	}
}
