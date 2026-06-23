package authorizationpolicy

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sort"
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

type recordStoreAuthorizationPolicyProjectionRepository struct {
	store  platform.RecordStore
	config platform.Config
}

type authorizationPolicyProjectionDriftReport struct {
	Missing []authorizationPolicyProjectionDriftFinding
	Orphan  []authorizationPolicyProjectionDriftFinding
	Stale   []authorizationPolicyProjectionDriftFinding
}

type authorizationPolicyProjectionDriftFinding struct {
	SourceResource string
	LocalResource  string
	ID             string
}

type authorizationPolicyProjectionDriftPair struct {
	sourceResource string
	localResource  string
	idFn           func(map[string]any) string
}

var authorizationPolicyProjectionDriftPairs = []authorizationPolicyProjectionDriftPair{
	{sourceResource: usersResource, localResource: policyIdentityUsers, idFn: func(row map[string]any) string {
		return policyIdentityReadModelID(policyIdentityUsers, row)
	}},
	{sourceResource: rolesResource, localResource: policyIdentityRoles, idFn: func(row map[string]any) string {
		return policyIdentityReadModelID(policyIdentityRoles, row)
	}},
	{sourceResource: policySourceProjectsResource, localResource: policyDataProjectsResource, idFn: policyProjectID},
	{sourceResource: policySourcePlansResource, localResource: policyDataPlansResource, idFn: policyPlanID},
	{sourceResource: policySourceImageAllowListsResource, localResource: policyDataImageAllowListsResource, idFn: policyImageRuleID},
}

func authorizationPolicyProjectionRepo(app *platform.App) *recordStoreAuthorizationPolicyProjectionRepository {
	if app == nil {
		return &recordStoreAuthorizationPolicyProjectionRepository{}
	}
	return &recordStoreAuthorizationPolicyProjectionRepository{store: app.Store, config: app.Config}
}

func authorizationPolicyProjectionRepoFromStore(store platform.RecordStore, config platform.Config) *recordStoreAuthorizationPolicyProjectionRepository {
	return &recordStoreAuthorizationPolicyProjectionRepository{store: store, config: config}
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

func (r recordStoreAuthorizationPolicyProjectionRepository) projectionDrift(ctx context.Context) (authorizationPolicyProjectionDriftReport, error) {
	var report authorizationPolicyProjectionDriftReport
	if r.store == nil {
		return report, errAuthorizationPolicyProjectionRepositoryUnavailable
	}
	for _, pair := range authorizationPolicyProjectionDriftPairs {
		sourceRows := authorizationPolicyProjectionDriftIndex(r.listMaps(ctx, pair.sourceResource), pair.idFn)
		localRows := authorizationPolicyProjectionDriftIndex(r.listMaps(ctx, pair.localResource), pair.idFn)
		for id, sourceRow := range sourceRows {
			localRow, ok := localRows[id]
			finding := authorizationPolicyProjectionDriftFinding{
				SourceResource: pair.sourceResource,
				LocalResource:  pair.localResource,
				ID:             id,
			}
			if !ok {
				report.Missing = append(report.Missing, finding)
				continue
			}
			if !reflect.DeepEqual(sourceRow, localRow) {
				report.Stale = append(report.Stale, finding)
			}
		}
		for id := range localRows {
			if _, ok := sourceRows[id]; ok {
				continue
			}
			report.Orphan = append(report.Orphan, authorizationPolicyProjectionDriftFinding{
				SourceResource: pair.sourceResource,
				LocalResource:  pair.localResource,
				ID:             id,
			})
		}
	}
	report.sort()
	return report, nil
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

func authorizationPolicyProjectionDriftIndex(rows []map[string]any, idFn func(map[string]any) string) map[string]map[string]any {
	out := map[string]map[string]any{}
	for _, row := range rows {
		id, normalized := authorizationPolicyProjectionDriftNormalize(row, idFn)
		if id == "" {
			continue
		}
		out[id] = normalized
	}
	return out
}

func authorizationPolicyProjectionDriftNormalize(row map[string]any, idFn func(map[string]any) string) (string, map[string]any) {
	normalized := shared.CloneMap(row)
	id := idFn(normalized)
	if id == "" {
		return "", nil
	}
	normalized["id"] = id
	return id, normalized
}

func (r *authorizationPolicyProjectionDriftReport) sort() {
	sortAuthorizationPolicyProjectionDriftFindings(r.Missing)
	sortAuthorizationPolicyProjectionDriftFindings(r.Orphan)
	sortAuthorizationPolicyProjectionDriftFindings(r.Stale)
}

func sortAuthorizationPolicyProjectionDriftFindings(findings []authorizationPolicyProjectionDriftFinding) {
	sort.Slice(findings, func(i, j int) bool {
		if findings[i].LocalResource != findings[j].LocalResource {
			return findings[i].LocalResource < findings[j].LocalResource
		}
		if findings[i].ID != findings[j].ID {
			return findings[i].ID < findings[j].ID
		}
		return findings[i].SourceResource < findings[j].SourceResource
	})
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
