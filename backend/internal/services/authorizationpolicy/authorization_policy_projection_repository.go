package authorizationpolicy

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/linskybing/nexuspaas/backend/internal/platform"
	"github.com/linskybing/nexuspaas/backend/internal/services/shared"
)

const (
	policyIdentityUsers = serviceName + ":identity_users"
	policyIdentityRoles = serviceName + ":identity_roles"
	usersResource       = "identity-service:users"
	rolesResource       = "identity-service:roles"

	policyDataProjectsResource        = serviceName + ":policy_projects"
	policyDataPlansResource           = serviceName + ":policy_plans"
	policyDataImageAllowListsResource = serviceName + ":policy_image_allow_lists"

	policySourceProjectsResource        = "org-project-service:projects"
	policySourcePlansResource           = "scheduler-quota-service:plans"
	policySourceImageAllowListsResource = "image-registry-service:image_allow_lists"
)

var errAuthorizationPolicyProjectionRepositoryUnavailable = errors.New("authorization policy projection repository unavailable")

type authorizationPolicyProjectionRepository interface {
	UpsertIdentityUser(context.Context, map[string]any) error
	UpsertIdentityRole(context.Context, map[string]any) error
	DeleteIdentityUser(context.Context, map[string]any) bool
	DeleteIdentityRole(context.Context, map[string]any) bool
	ListIdentityUsers(context.Context) []map[string]any
	ListIdentityRoles(context.Context) []map[string]any

	UpsertPolicyProject(context.Context, map[string]any) error
	UpsertPolicyPlan(context.Context, map[string]any) error
	UpsertPolicyImageAllowList(context.Context, map[string]any) error
	DeletePolicyProject(context.Context, map[string]any) bool
	DeletePolicyPlan(context.Context, map[string]any) bool
	DeletePolicyImageAllowList(context.Context, map[string]any) bool
	ListPolicyProjects(context.Context) []map[string]any
	FindPolicyPlanForProject(context.Context, map[string]any) map[string]any
	ListPolicyImageRulesForProject(context.Context, string) []map[string]any
}

type recordStoreAuthorizationPolicyProjectionRepository struct {
	store  platform.RecordStore
	config platform.Config
}

func authorizationPolicyProjectionRepo(app *platform.App) authorizationPolicyProjectionRepository {
	if app == nil {
		return recordStoreAuthorizationPolicyProjectionRepository{}
	}
	return recordStoreAuthorizationPolicyProjectionRepository{store: app.Store, config: app.Config}
}

func authorizationPolicyProjectionRepoFromStore(store platform.RecordStore, config platform.Config) authorizationPolicyProjectionRepository {
	return recordStoreAuthorizationPolicyProjectionRepository{store: store, config: config}
}

func (r recordStoreAuthorizationPolicyProjectionRepository) UpsertIdentityUser(ctx context.Context, data map[string]any) error {
	return r.upsertReadModel(ctx, policyIdentityUsers, data, func(row map[string]any) string {
		return policyIdentityReadModelID(policyIdentityUsers, row)
	})
}

func (r recordStoreAuthorizationPolicyProjectionRepository) UpsertIdentityRole(ctx context.Context, data map[string]any) error {
	return r.upsertReadModel(ctx, policyIdentityRoles, data, func(row map[string]any) string {
		return policyIdentityReadModelID(policyIdentityRoles, row)
	})
}

func (r recordStoreAuthorizationPolicyProjectionRepository) DeleteIdentityUser(ctx context.Context, data map[string]any) bool {
	return r.deleteIdentityReadModel(ctx, policyIdentityUsers, data)
}

func (r recordStoreAuthorizationPolicyProjectionRepository) DeleteIdentityRole(ctx context.Context, data map[string]any) bool {
	return r.deleteIdentityReadModel(ctx, policyIdentityRoles, data)
}

func (r recordStoreAuthorizationPolicyProjectionRepository) ListIdentityUsers(ctx context.Context) []map[string]any {
	return r.identityRecords(ctx, policyIdentityUsers, usersResource)
}

func (r recordStoreAuthorizationPolicyProjectionRepository) ListIdentityRoles(ctx context.Context) []map[string]any {
	return r.identityRecords(ctx, policyIdentityRoles, rolesResource)
}

func (r recordStoreAuthorizationPolicyProjectionRepository) UpsertPolicyProject(ctx context.Context, data map[string]any) error {
	return r.upsertReadModel(ctx, policyDataProjectsResource, data, policyProjectID)
}

func (r recordStoreAuthorizationPolicyProjectionRepository) UpsertPolicyPlan(ctx context.Context, data map[string]any) error {
	return r.upsertReadModel(ctx, policyDataPlansResource, data, policyPlanID)
}

func (r recordStoreAuthorizationPolicyProjectionRepository) UpsertPolicyImageAllowList(ctx context.Context, data map[string]any) error {
	return r.upsertReadModel(ctx, policyDataImageAllowListsResource, data, policyImageRuleID)
}

func (r recordStoreAuthorizationPolicyProjectionRepository) DeletePolicyProject(ctx context.Context, data map[string]any) bool {
	return r.deleteReadModelByID(ctx, policyDataProjectsResource, policyProjectID(data))
}

func (r recordStoreAuthorizationPolicyProjectionRepository) DeletePolicyPlan(ctx context.Context, data map[string]any) bool {
	return r.deleteReadModelByID(ctx, policyDataPlansResource, policyPlanID(data))
}

func (r recordStoreAuthorizationPolicyProjectionRepository) DeletePolicyImageAllowList(ctx context.Context, data map[string]any) bool {
	if id := policyImageRuleID(data); id != "" {
		if r.deleteReadModelByID(ctx, policyDataImageAllowListsResource, id) {
			return true
		}
	}
	if r.store == nil {
		return false
	}
	tagID := shared.TextValue(data, "tag_id", "tagId")
	if tagID == "" {
		return false
	}
	deleted := false
	for _, record := range r.store.List(ctx, policyDataImageAllowListsResource) {
		if shared.TextValue(record.Data, "tag_id", "tagId") == tagID {
			deleted = r.store.Delete(ctx, policyDataImageAllowListsResource, record.ID) || deleted
		}
	}
	return deleted
}

func (r recordStoreAuthorizationPolicyProjectionRepository) ListPolicyProjects(ctx context.Context) []map[string]any {
	return r.policyDataRecords(ctx, policyDataProjectsResource, policySourceProjectsResource, policyProjectID)
}

func (r recordStoreAuthorizationPolicyProjectionRepository) FindPolicyPlanForProject(ctx context.Context, project map[string]any) map[string]any {
	planID := shared.TextValue(project, "plan_id", "planId", "resource_plan_id", "resourcePlanId")
	if planID == "" {
		return nil
	}
	for _, plan := range r.policyDataRecords(ctx, policyDataPlansResource, policySourcePlansResource, policyPlanID) {
		if policyPlanID(plan) == planID {
			return plan
		}
	}
	return nil
}

func (r recordStoreAuthorizationPolicyProjectionRepository) ListPolicyImageRulesForProject(ctx context.Context, projectID string) []map[string]any {
	if projectID == "" {
		return nil
	}
	records := r.policyDataRecords(ctx, policyDataImageAllowListsResource, policySourceImageAllowListsResource, policyImageRuleID)
	out := make([]map[string]any, 0, len(records))
	for _, record := range records {
		ruleProjectID := shared.TextValue(record, "project_id", "projectId")
		if ruleProjectID != projectID && ruleProjectID != "*" {
			continue
		}
		out = append(out, record)
	}
	return out
}

func (r recordStoreAuthorizationPolicyProjectionRepository) upsertReadModel(ctx context.Context, resource string, data map[string]any, idFn func(map[string]any) string) error {
	id := idFn(data)
	if id == "" {
		return nil
	}
	if r.store == nil {
		return errAuthorizationPolicyProjectionRepositoryUnavailable
	}
	data = shared.CloneMap(data)
	data["id"] = id
	if _, ok := r.store.Update(ctx, resource, id, data); ok {
		return nil
	}
	if _, err := r.store.Create(ctx, resource, data); err != nil {
		if platform.IsCreateConflict(err) {
			if _, ok := r.store.Update(ctx, resource, id, data); !ok {
				return fmt.Errorf("authorization policy projection conflict update missed for %s/%s", resource, id)
			}
			return nil
		}
		return fmt.Errorf("authorization policy projection create failed for %s/%s: %w", resource, id, err)
	}
	return nil
}

func (r recordStoreAuthorizationPolicyProjectionRepository) deleteIdentityReadModel(ctx context.Context, resource string, data map[string]any) bool {
	if deleted, ok := data["deleted"].(bool); ok && !deleted {
		return false
	}
	return r.deleteReadModelByID(ctx, resource, policyIdentityReadModelID(resource, data))
}

func (r recordStoreAuthorizationPolicyProjectionRepository) deleteReadModelByID(ctx context.Context, resource, id string) bool {
	if r.store == nil || id == "" {
		return false
	}
	return r.store.Delete(ctx, resource, id)
}

func (r recordStoreAuthorizationPolicyProjectionRepository) identityRecords(ctx context.Context, localResource, sourceResource string) []map[string]any {
	local := r.listMaps(ctx, localResource)
	if !r.identitySourceCoHosted(sourceResource) {
		return local
	}
	source := r.listMaps(ctx, sourceResource)
	if len(local) == 0 {
		return source
	}
	return mergePolicyIdentityRecords(localResource, source, local)
}

func (r recordStoreAuthorizationPolicyProjectionRepository) policyDataRecords(ctx context.Context, localResource, sourceResource string, idFn func(map[string]any) string) []map[string]any {
	local := r.listMaps(ctx, localResource)
	if !r.policyDataSourceCoHosted() {
		return local
	}
	source := r.listMaps(ctx, sourceResource)
	if len(local) == 0 {
		return source
	}
	return mergePolicyDataRecords(source, local, idFn)
}

func (r recordStoreAuthorizationPolicyProjectionRepository) listMaps(ctx context.Context, resource string) []map[string]any {
	if r.store == nil {
		return nil
	}
	records := r.store.List(ctx, resource)
	out := make([]map[string]any, 0, len(records))
	for _, record := range records {
		row := shared.CloneMap(record.Data)
		if shared.TextValue(row, "id", "ID") == "" {
			row["id"] = record.ID
		}
		out = append(out, row)
	}
	return out
}

func (r recordStoreAuthorizationPolicyProjectionRepository) identitySourceCoHosted(sourceResource string) bool {
	owner, _, ok := strings.Cut(sourceResource, ":")
	return ok && r.config.AllowsService(owner)
}

func (r recordStoreAuthorizationPolicyProjectionRepository) policyDataSourceCoHosted() bool {
	return r.config.ServiceName == "all"
}

func policyIdentityReadModelID(resource string, data map[string]any) string {
	id := shared.TextValue(data, identityKeyID, identityKeyIDTitle)
	userID := shared.TextValue(data, identityKeyUserID, identityKeyUserIDCamel, identityKeyUserIDTitle)
	name := shared.TextValue(data, identityKeyName, identityKeyNameTitle)
	switch resource {
	case policyIdentityUsers:
		return shared.FirstNonEmpty(id, userID)
	case policyIdentityRoles:
		return shared.FirstNonEmpty(id, name, userID)
	default:
		return shared.FirstNonEmpty(id, userID, name)
	}
}

func mergePolicyIdentityRecords(resource string, source, local []map[string]any) []map[string]any {
	out := make([]map[string]any, 0, len(source)+len(local))
	seen := map[string]bool{}
	for _, record := range local {
		if id := policyIdentityReadModelID(resource, record); id != "" {
			seen[id] = true
		}
		out = append(out, record)
	}
	for _, record := range source {
		id := policyIdentityReadModelID(resource, record)
		if id != "" && seen[id] {
			continue
		}
		out = append(out, record)
	}
	return out
}

func mergePolicyDataRecords(source, local []map[string]any, idFn func(map[string]any) string) []map[string]any {
	out := make([]map[string]any, 0, len(source)+len(local))
	seen := map[string]bool{}
	for _, record := range local {
		if id := idFn(record); id != "" {
			seen[id] = true
		}
		out = append(out, record)
	}
	for _, record := range source {
		id := idFn(record)
		if id != "" && seen[id] {
			continue
		}
		out = append(out, record)
	}
	return out
}
