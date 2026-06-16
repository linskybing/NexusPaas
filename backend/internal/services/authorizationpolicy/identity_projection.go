package authorizationpolicy

import (
	"maps"
	"net/http"
	"strings"

	"github.com/linskybing/nexuspaas/backend/internal/contracts"
	"github.com/linskybing/nexuspaas/backend/internal/platform"
)

const (
	identityKeyID          = "id"
	identityKeyName        = "name"
	identityKeyNameTitle   = "Name"
	identityKeyIDTitle     = "ID"
	identityKeyUserID      = "user_id"
	identityKeyUserIDCamel = "userId"
	identityKeyUserIDTitle = "UserID"
)

func syncPolicyIdentityReadModels(app *platform.App, r *http.Request) {
	if app == nil || app.Store == nil || app.Events == nil {
		return
	}
	app.RunProjection(r.Context(), identityProjectionConsumer, func(event contracts.Event) error {
		return projectPolicyIdentityEvent(app, r, event)
	})
}

func projectPolicyIdentityEvent(app *platform.App, r *http.Request, event contracts.Event) error {
	resource, data, deleted, ok := policyIdentityProjection(event)
	if !ok {
		return nil
	}
	repo := authorizationPolicyProjectionRepo(app)
	if deleted {
		deletePolicyIdentityReadModel(repo, r, resource, data)
		return nil
	}
	return upsertPolicyIdentityReadModel(repo, r, resource, data)
}

func policyIdentityProjection(event contracts.Event) (string, map[string]any, bool, bool) {
	switch strings.ToLower(event.Name) {
	case "usercreated", "userupdated", "userdisabled":
		return policyIdentityUsers, policyIdentityEventData(event), false, true
	case "userdeleted":
		return policyIdentityUsers, policyIdentityEventData(event), true, true
	case "rolecreated", "roleupdated":
		return policyIdentityRoles, policyIdentityEventData(event), false, true
	case "roledeleted":
		return policyIdentityRoles, policyIdentityEventData(event), true, true
	default:
		return "", nil, false, false
	}
}

func policyIdentityEventData(event contracts.Event) map[string]any {
	if next, ok := event.Data["new"].(map[string]any); ok {
		return maps.Clone(next)
	}
	return maps.Clone(event.Data)
}

func upsertPolicyIdentityReadModel(repo authorizationPolicyProjectionRepository, r *http.Request, resource string, data map[string]any) error {
	switch resource {
	case policyIdentityUsers:
		return repo.UpsertIdentityUser(r.Context(), data)
	case policyIdentityRoles:
		return repo.UpsertIdentityRole(r.Context(), data)
	default:
		return nil
	}
}

func deletePolicyIdentityReadModel(repo authorizationPolicyProjectionRepository, r *http.Request, resource string, data map[string]any) bool {
	switch resource {
	case policyIdentityUsers:
		return repo.DeleteIdentityUser(r.Context(), data)
	case policyIdentityRoles:
		return repo.DeleteIdentityRole(r.Context(), data)
	default:
		return false
	}
}

func policyIdentityRecords(app *platform.App, r *http.Request, localResource, _ string) []map[string]any {
	syncPolicyIdentityReadModels(app, r)
	repo := authorizationPolicyProjectionRepo(app)
	switch localResource {
	case policyIdentityUsers:
		return repo.ListIdentityUsers(r.Context())
	case policyIdentityRoles:
		return repo.ListIdentityRoles(r.Context())
	default:
		return nil
	}
}
