package orgproject

import "github.com/linskybing/nexuspaas/backend/internal/platform"

// registerInternalReadContracts exposes org-project-owned aggregates that other
// services must read (today: scheduler-quota submit-admission — blocker-ledger.md #3) through
// service-key-gated, read-only HTTP contracts. Consumers read them transparently via
// the platform crossServiceStore when org-project is not co-hosted, so no bespoke
// client is needed. project_members and user_quotas are keyed by a composite
// "<projectID>/<userID>", so their get routes use a trailing wildcard.
func registerInternalReadContracts(app *platform.App) {
	app.RegisterReadContract(projectsResource, "/internal/org-project/projects", "/internal/org-project/projects/{id...}")
	app.RegisterReadContract(projectMembersResource, "/internal/org-project/project-members", "/internal/org-project/project-members/{id...}")
	app.RegisterReadContract(projectUserQuotasResource, "/internal/org-project/user-quotas", "/internal/org-project/user-quotas/{id...}")
	registerGroupGPUReadContracts(app)
}
