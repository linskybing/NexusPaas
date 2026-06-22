package authorizationpolicy

import (
	"context"
	"errors"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/contracts"
	"github.com/linskybing/nexuspaas/backend/internal/platform"
	"github.com/linskybing/nexuspaas/backend/internal/services/shared"
)

var errAuthorizationPolicyRepositoryUnavailable = errors.New("authorization policy repository unavailable")

type proxyPolicyRuleReplacement struct {
	Rules []map[string]any
}

type recordStoreAuthorizationPolicyRepository struct {
	store platform.RecordStore
}

func authorizationPolicyRepo(app *platform.App) *recordStoreAuthorizationPolicyRepository {
	if app == nil {
		return &recordStoreAuthorizationPolicyRepository{}
	}
	return &recordStoreAuthorizationPolicyRepository{store: app.Store}
}

func (r recordStoreAuthorizationPolicyRepository) EnsureDefaultProxyServices(ctx context.Context) error {
	if r.store == nil {
		return errAuthorizationPolicyRepositoryUnavailable
	}
	if r.seedMarked(ctx, seedProxyServices) {
		return nil
	}
	if len(r.store.List(ctx, servicesResource)) > 0 {
		_ = r.claimSeed(ctx, seedProxyServices)
		return nil
	}
	if !r.claimSeed(ctx, seedProxyServices) {
		return nil
	}
	now := time.Now().UTC()
	for _, row := range defaultServices {
		data := shared.CloneMap(row)
		data["created_at"] = now
		data["updated_at"] = now
		r.seedCreate(ctx, servicesResource, data)
	}
	return nil
}

func (r recordStoreAuthorizationPolicyRepository) ListProxyServices(ctx context.Context) []map[string]any {
	if r.store == nil {
		return nil
	}
	_ = r.EnsureDefaultProxyServices(ctx)
	rows := r.listMaps(ctx, servicesResource)
	sort.Slice(rows, func(i, j int) bool {
		left, right := shared.IntValue(rows[i], "sort_order", "sortOrder"), shared.IntValue(rows[j], "sort_order", "sortOrder")
		if left == right {
			return shared.TextValue(rows[i], "id") < shared.TextValue(rows[j], "id")
		}
		return left < right
	})
	return rows
}

func (r recordStoreAuthorizationPolicyRepository) GetProxyService(ctx context.Context, id string) (map[string]any, bool) {
	for _, service := range r.ListProxyServices(ctx) {
		if shared.TextValue(service, "id") == id {
			return service, true
		}
	}
	return nil, false
}

func (r recordStoreAuthorizationPolicyRepository) EnsureDefaultProxyPolicies(ctx context.Context) error {
	if r.store == nil {
		return errAuthorizationPolicyRepositoryUnavailable
	}
	if r.seedMarked(ctx, seedProxyPolicies) {
		return nil
	}
	if len(r.store.List(ctx, policiesResource)) > 0 {
		_ = r.claimSeed(ctx, seedProxyPolicies)
		return nil
	}
	if !r.claimSeed(ctx, seedProxyPolicies) {
		return nil
	}
	now := time.Now().UTC()
	for _, row := range defaultPolicies {
		data := shared.CloneMap(row)
		rules, _ := data["rules"].([]map[string]any)
		delete(data, "rules")
		data["created_at"] = now
		data["updated_at"] = now
		record := r.seedCreate(ctx, policiesResource, data)
		policyID := shared.FirstNonEmpty(record.ID, shared.TextValue(data, "id"))
		for _, rule := range rules {
			ruleData := shared.CloneMap(rule)
			ruleData["policy_id"] = policyID
			r.seedCreate(ctx, rulesResource, ruleData)
		}
	}
	return nil
}

func (r recordStoreAuthorizationPolicyRepository) NextProxyPolicyID(context.Context) string {
	return r.nextID(policiesResource, "PO", 2600001)
}

func (r recordStoreAuthorizationPolicyRepository) ListProxyPolicies(ctx context.Context) []map[string]any {
	if r.store == nil {
		return nil
	}
	_ = r.EnsureDefaultProxyPolicies(ctx)
	rows := r.listMaps(ctx, policiesResource)
	for i := range rows {
		rows[i] = r.composePolicy(ctx, rows[i])
	}
	sort.Slice(rows, func(i, j int) bool {
		return shared.TextValue(rows[i], "name") < shared.TextValue(rows[j], "name")
	})
	return rows
}

func (r recordStoreAuthorizationPolicyRepository) GetProxyPolicy(ctx context.Context, id string) (map[string]any, bool) {
	if r.store == nil {
		return nil, false
	}
	_ = r.EnsureDefaultProxyPolicies(ctx)
	if record, found := r.store.Get(ctx, policiesResource, id); found {
		return r.composePolicy(ctx, shared.CloneMap(record.Data)), true
	}
	return nil, false
}

func (r recordStoreAuthorizationPolicyRepository) PolicyNameExists(ctx context.Context, excludeID, name string) bool {
	normalized := strings.ToLower(strings.TrimSpace(name))
	for _, policy := range r.ListProxyPolicies(ctx) {
		if shared.TextValue(policy, "id") == excludeID {
			continue
		}
		if strings.ToLower(shared.TextValue(policy, "name")) == normalized {
			return true
		}
	}
	return false
}

func (r recordStoreAuthorizationPolicyRepository) CreateProxyPolicy(ctx context.Context, policy map[string]any, rules []map[string]any) (map[string]any, error) {
	if r.store == nil {
		return nil, errAuthorizationPolicyRepositoryUnavailable
	}
	record, err := r.store.Create(ctx, policiesResource, shared.CloneMap(policy))
	if err != nil {
		return nil, err
	}
	if err := r.replacePolicyRules(ctx, record.ID, rules); err != nil {
		r.store.Delete(ctx, policiesResource, record.ID)
		return nil, err
	}
	return r.composePolicy(ctx, shared.CloneMap(record.Data)), nil
}

func (r recordStoreAuthorizationPolicyRepository) UpdateProxyPolicy(ctx context.Context, id string, update map[string]any, replacement *proxyPolicyRuleReplacement) (map[string]any, bool, error) {
	if r.store == nil {
		return nil, false, errAuthorizationPolicyRepositoryUnavailable
	}
	updated, ok := r.store.Update(ctx, policiesResource, id, shared.CloneMap(update))
	if !ok {
		return nil, false, nil
	}
	if replacement != nil {
		if err := r.replacePolicyRules(ctx, id, replacement.Rules); err != nil {
			return nil, true, err
		}
	}
	return r.composePolicy(ctx, shared.CloneMap(updated.Data)), true, nil
}

// CreateProxyPolicyTx writes the policy and all its rule rows inside the caller's
// transaction, so the policy, its rules, and the emitted event commit together.
// Rollback replaces the hand-written delete-on-failure compensation. The composed
// result is built from the in-hand rules (they are not yet committed to read back).
func (r recordStoreAuthorizationPolicyRepository) CreateProxyPolicyTx(ctx context.Context, tx platform.StoreTx, policy map[string]any, rules []map[string]any) (map[string]any, error) {
	if r.store == nil {
		return nil, errAuthorizationPolicyRepositoryUnavailable
	}
	record, err := tx.Create(ctx, policiesResource, shared.CloneMap(policy))
	if err != nil {
		return nil, err
	}
	created := make([]map[string]any, 0, len(rules))
	for _, rule := range rules {
		ruleRecord := map[string]any{
			"id":         shared.FirstNonEmpty(shared.TextValue(rule, "id"), r.nextID(rulesResource, "PR", 2600001)),
			"policy_id":  record.ID,
			"service_id": shared.TextValue(rule, "service_id", "serviceId"),
			"actions":    shared.StringSlice(rule["actions"]),
		}
		if _, err := tx.Create(ctx, rulesResource, shared.CloneMap(ruleRecord)); err != nil {
			return nil, err
		}
		created = append(created, ruleRecord)
	}
	sort.Slice(created, func(i, j int) bool {
		return shared.TextValue(created[i], "service_id", "serviceId") < shared.TextValue(created[j], "service_id", "serviceId")
	})
	composed := shared.CloneMap(record.Data)
	composed["rules"] = created
	return composed, nil
}

// UpdateProxyPolicyTx updates the policy and, when requested, replaces its rule
// rows in the caller's transaction so the replacement and event commit together.
func (r recordStoreAuthorizationPolicyRepository) UpdateProxyPolicyTx(ctx context.Context, tx platform.StoreTx, id string, update map[string]any, replacement *proxyPolicyRuleReplacement) (map[string]any, bool, error) {
	if r.store == nil {
		return nil, false, errAuthorizationPolicyRepositoryUnavailable
	}
	updated, ok, err := tx.Update(ctx, policiesResource, id, shared.CloneMap(update))
	if err != nil || !ok {
		return nil, ok, err
	}
	if replacement == nil {
		return r.composePolicy(ctx, shared.CloneMap(updated.Data)), true, nil
	}
	for _, rule := range r.store.List(ctx, rulesResource) {
		if shared.TextValue(rule.Data, "policy_id", "policyId") == id {
			if _, err := tx.Delete(ctx, rulesResource, rule.ID); err != nil {
				return nil, true, err
			}
		}
	}
	created := make([]map[string]any, 0, len(replacement.Rules))
	for _, rule := range replacement.Rules {
		ruleRecord := map[string]any{
			"id":         shared.FirstNonEmpty(shared.TextValue(rule, "id"), r.nextID(rulesResource, "PR", 2600001)),
			"policy_id":  id,
			"service_id": shared.TextValue(rule, "service_id", "serviceId"),
			"actions":    shared.StringSlice(rule["actions"]),
		}
		if _, err := tx.Create(ctx, rulesResource, shared.CloneMap(ruleRecord)); err != nil {
			return nil, true, err
		}
		created = append(created, ruleRecord)
	}
	sort.Slice(created, func(i, j int) bool {
		return shared.TextValue(created[i], "service_id", "serviceId") < shared.TextValue(created[j], "service_id", "serviceId")
	})
	composed := shared.CloneMap(updated.Data)
	composed["rules"] = created
	return composed, true, nil
}

// DeleteProxyPolicyCascadeTx deletes the policy plus its rules and assignments in
// one transaction. Existing rows are read from the committed store; the deletes go
// through tx so they roll back together with the emitted event on any failure.
func (r recordStoreAuthorizationPolicyRepository) DeleteProxyPolicyCascadeTx(ctx context.Context, tx platform.StoreTx, id string) (map[string]any, bool, error) {
	current, found := r.GetProxyPolicy(ctx, id)
	if !found || r.store == nil {
		return nil, false, nil
	}
	for _, rule := range r.store.List(ctx, rulesResource) {
		if shared.TextValue(rule.Data, "policy_id", "policyId") == id {
			if _, err := tx.Delete(ctx, rulesResource, rule.ID); err != nil {
				return nil, false, err
			}
		}
	}
	for _, assignment := range r.store.List(ctx, assignmentsResource) {
		if shared.TextValue(assignment.Data, "policy_id", "policyId") == id {
			if _, err := tx.Delete(ctx, assignmentsResource, assignment.ID); err != nil {
				return nil, false, err
			}
		}
	}
	if _, err := tx.Delete(ctx, policiesResource, id); err != nil {
		return nil, false, err
	}
	return current, true, nil
}

// DeleteProxyRoleCascadeTx deletes the role plus its role-user memberships and
// role-targeted assignments in one transaction.
func (r recordStoreAuthorizationPolicyRepository) DeleteProxyRoleCascadeTx(ctx context.Context, tx platform.StoreTx, id string) (map[string]any, bool, error) {
	if r.store == nil {
		return nil, false, nil
	}
	current, _ := r.GetProxyRole(ctx, id)
	for _, member := range r.store.List(ctx, roleUsersResource) {
		if shared.TextValue(member.Data, "role_id", "roleId") == id {
			if _, err := tx.Delete(ctx, roleUsersResource, member.ID); err != nil {
				return nil, false, err
			}
		}
	}
	for _, assignment := range r.store.List(ctx, assignmentsResource) {
		if shared.TextValue(assignment.Data, "target_type", "targetType") == "role" &&
			shared.TextValue(assignment.Data, "target_id", "targetId") == id {
			if _, err := tx.Delete(ctx, assignmentsResource, assignment.ID); err != nil {
				return nil, false, err
			}
		}
	}
	if _, err := tx.Delete(ctx, platformRolesResource, id); err != nil {
		return nil, false, err
	}
	return current, true, nil
}

func (r recordStoreAuthorizationPolicyRepository) DeleteProxyPolicyCascade(ctx context.Context, id string) (map[string]any, bool) {
	current, found := r.GetProxyPolicy(ctx, id)
	if !found || r.store == nil {
		return nil, false
	}
	for _, rule := range r.store.List(ctx, rulesResource) {
		if shared.TextValue(rule.Data, "policy_id", "policyId") == id {
			r.store.Delete(ctx, rulesResource, rule.ID)
		}
	}
	for _, assignment := range r.store.List(ctx, assignmentsResource) {
		if shared.TextValue(assignment.Data, "policy_id", "policyId") == id {
			r.store.Delete(ctx, assignmentsResource, assignment.ID)
		}
	}
	r.store.Delete(ctx, policiesResource, id)
	return current, true
}

func (r recordStoreAuthorizationPolicyRepository) EnsureDefaultProxyAssignments(ctx context.Context) error {
	if r.store == nil {
		return errAuthorizationPolicyRepositoryUnavailable
	}
	_ = r.EnsureDefaultProxyPolicies(ctx)
	_ = r.EnsureDefaultProxyRoles(ctx)
	if r.seedMarked(ctx, seedProxyAssignments) {
		return nil
	}
	if len(r.store.List(ctx, assignmentsResource)) > 0 {
		_ = r.claimSeed(ctx, seedProxyAssignments)
		return nil
	}
	if !r.claimSeed(ctx, seedProxyAssignments) {
		return nil
	}
	now := time.Now().UTC()
	for _, row := range defaultAssignments {
		policyID := shared.TextValue(row, "policy_id", "policyId")
		if _, found := r.GetProxyPolicy(ctx, policyID); !found {
			continue
		}
		if shared.TextValue(row, "target_type", "targetType") == "role" {
			if _, found := r.GetProxyRole(ctx, shared.TextValue(row, "target_id", "targetId")); !found {
				continue
			}
		}
		data := shared.CloneMap(row)
		data["created_at"] = now
		r.seedCreate(ctx, assignmentsResource, data)
	}
	return nil
}

func (r recordStoreAuthorizationPolicyRepository) ListPolicyAssignments(ctx context.Context, policyID string) []map[string]any {
	if r.store == nil {
		return nil
	}
	rows := []map[string]any{}
	for _, assignment := range r.store.List(ctx, assignmentsResource) {
		if shared.TextValue(assignment.Data, "policy_id", "policyId") == policyID {
			rows = append(rows, r.composeAssignment(ctx, assignment.Data))
		}
	}
	sortAssignments(rows)
	return rows
}

func (r recordStoreAuthorizationPolicyRepository) CreatePolicyAssignment(ctx context.Context, policyID, targetType, targetID, assignedBy string) (map[string]any, bool, error) {
	if r.store == nil {
		return nil, false, errAuthorizationPolicyRepositoryUnavailable
	}
	if existing, found := r.findAssignment(ctx, policyID, targetType, targetID); found {
		return r.composeAssignment(ctx, existing.Data), false, nil
	}
	for attempt := 0; attempt < 3; attempt++ {
		record, err := r.store.Create(ctx, assignmentsResource, map[string]any{
			"id":          r.nextID(assignmentsResource, "PA", 2600001),
			"policy_id":   policyID,
			"target_type": targetType,
			"target_id":   targetID,
			"assigned_by": assignedBy,
			"created_at":  time.Now().UTC(),
		})
		if err == nil {
			return r.composeAssignment(ctx, record.Data), true, nil
		}
		if !platform.IsCreateConflict(err) {
			return nil, false, err
		}
		if existing, found := r.findAssignment(ctx, policyID, targetType, targetID); found {
			return r.composeAssignment(ctx, existing.Data), false, nil
		}
	}
	return nil, false, platform.CreateConflictError{Resource: assignmentsResource, ID: "assignment"}
}

func (r recordStoreAuthorizationPolicyRepository) CreatePolicyAssignmentTx(ctx context.Context, tx platform.StoreTx, policyID, targetType, targetID, assignedBy string) (map[string]any, bool, error) {
	if r.store == nil {
		return nil, false, errAuthorizationPolicyRepositoryUnavailable
	}
	if existing, found := r.findAssignment(ctx, policyID, targetType, targetID); found {
		return r.composeAssignment(ctx, existing.Data), false, nil
	}
	record, err := tx.Create(ctx, assignmentsResource, map[string]any{
		"id":          r.nextID(assignmentsResource, "PA", 2600001),
		"policy_id":   policyID,
		"target_type": targetType,
		"target_id":   targetID,
		"assigned_by": assignedBy,
		"created_at":  time.Now().UTC(),
	})
	if err != nil {
		if platform.IsCreateConflict(err) {
			if existing, found := r.findAssignment(ctx, policyID, targetType, targetID); found {
				return r.composeAssignment(ctx, existing.Data), false, nil
			}
		}
		return nil, false, err
	}
	return r.composeAssignment(ctx, record.Data), true, nil
}

func (r recordStoreAuthorizationPolicyRepository) UnassignPolicy(ctx context.Context, policyID, targetType, targetID string) (map[string]any, bool) {
	if r.store == nil {
		return nil, false
	}
	if assignment, found := r.findAssignment(ctx, policyID, targetType, targetID); found {
		row := shared.CloneMap(assignment.Data)
		r.store.Delete(ctx, assignmentsResource, assignment.ID)
		return row, true
	}
	return nil, false
}

func (r recordStoreAuthorizationPolicyRepository) UnassignPolicyTx(ctx context.Context, tx platform.StoreTx, policyID, targetType, targetID string) (map[string]any, bool, error) {
	if r.store == nil {
		return nil, false, nil
	}
	if assignment, found := r.findAssignment(ctx, policyID, targetType, targetID); found {
		row := shared.CloneMap(assignment.Data)
		if _, err := tx.Delete(ctx, assignmentsResource, assignment.ID); err != nil {
			return nil, false, err
		}
		return row, true, nil
	}
	return nil, false, nil
}

func (r recordStoreAuthorizationPolicyRepository) ListTargetAssignments(ctx context.Context, targetType, targetID string) []map[string]any {
	if r.store == nil {
		return nil
	}
	rows := []map[string]any{}
	for _, assignment := range r.store.List(ctx, assignmentsResource) {
		if shared.TextValue(assignment.Data, "target_type", "targetType") != targetType ||
			shared.TextValue(assignment.Data, "target_id", "targetId") != targetID {
			continue
		}
		rows = append(rows, r.composeAssignment(ctx, assignment.Data))
	}
	sortAssignments(rows)
	return rows
}

func (r recordStoreAuthorizationPolicyRepository) EnsureDefaultProxyRoles(ctx context.Context) error {
	if r.store == nil {
		return errAuthorizationPolicyRepositoryUnavailable
	}
	if r.seedMarked(ctx, seedProxyRoles) {
		return nil
	}
	if len(r.store.List(ctx, platformRolesResource)) > 0 {
		_ = r.claimSeed(ctx, seedProxyRoles)
		return nil
	}
	if !r.claimSeed(ctx, seedProxyRoles) {
		return nil
	}
	now := time.Now().UTC()
	for _, row := range defaultPlatformRoles {
		data := shared.CloneMap(row)
		data["created_at"] = now
		data["updated_at"] = now
		r.seedCreate(ctx, platformRolesResource, data)
	}
	return nil
}

func (r recordStoreAuthorizationPolicyRepository) NextProxyRoleID(context.Context) string {
	return r.nextID(platformRolesResource, "RL", 2600001)
}

func (r recordStoreAuthorizationPolicyRepository) ListProxyRoles(ctx context.Context) []map[string]any {
	if r.store == nil {
		return nil
	}
	_ = r.EnsureDefaultProxyRoles(ctx)
	rows := r.listMaps(ctx, platformRolesResource)
	sort.Slice(rows, func(i, j int) bool {
		return shared.TextValue(rows[i], "name") < shared.TextValue(rows[j], "name")
	})
	return rows
}

func (r recordStoreAuthorizationPolicyRepository) GetProxyRole(ctx context.Context, id string) (map[string]any, bool) {
	if r.store == nil {
		return nil, false
	}
	_ = r.EnsureDefaultProxyRoles(ctx)
	if record, found := r.store.Get(ctx, platformRolesResource, id); found {
		return shared.CloneMap(record.Data), true
	}
	return nil, false
}

func (r recordStoreAuthorizationPolicyRepository) RoleNameExists(ctx context.Context, excludeID, name string) bool {
	normalized := strings.ToLower(strings.TrimSpace(name))
	for _, role := range r.ListProxyRoles(ctx) {
		if shared.TextValue(role, "id") == excludeID {
			continue
		}
		if strings.ToLower(shared.TextValue(role, "name")) == normalized {
			return true
		}
	}
	return false
}

func (r recordStoreAuthorizationPolicyRepository) CreateProxyRole(ctx context.Context, role map[string]any) (map[string]any, error) {
	if r.store == nil {
		return nil, errAuthorizationPolicyRepositoryUnavailable
	}
	record, err := r.store.Create(ctx, platformRolesResource, shared.CloneMap(role))
	if err != nil {
		return nil, err
	}
	return shared.CloneMap(record.Data), nil
}

func (r recordStoreAuthorizationPolicyRepository) CreateProxyRoleTx(ctx context.Context, tx platform.StoreTx, role map[string]any) (map[string]any, error) {
	if r.store == nil {
		return nil, errAuthorizationPolicyRepositoryUnavailable
	}
	record, err := tx.Create(ctx, platformRolesResource, shared.CloneMap(role))
	if err != nil {
		return nil, err
	}
	return shared.CloneMap(record.Data), nil
}

func (r recordStoreAuthorizationPolicyRepository) UpdateProxyRole(ctx context.Context, id string, update map[string]any) (map[string]any, bool) {
	if r.store == nil {
		return nil, false
	}
	updated, ok := r.store.Update(ctx, platformRolesResource, id, shared.CloneMap(update))
	if !ok {
		return nil, false
	}
	return shared.CloneMap(updated.Data), true
}

func (r recordStoreAuthorizationPolicyRepository) UpdateProxyRoleTx(ctx context.Context, tx platform.StoreTx, id string, update map[string]any) (map[string]any, bool, error) {
	if r.store == nil {
		return nil, false, nil
	}
	updated, ok, err := tx.Update(ctx, platformRolesResource, id, shared.CloneMap(update))
	if err != nil || !ok {
		return nil, ok, err
	}
	return shared.CloneMap(updated.Data), true, nil
}

func (r recordStoreAuthorizationPolicyRepository) DeleteProxyRoleCascade(ctx context.Context, id string) (map[string]any, bool) {
	if r.store == nil {
		return nil, false
	}
	current, _ := r.GetProxyRole(ctx, id)
	for _, member := range r.store.List(ctx, roleUsersResource) {
		if shared.TextValue(member.Data, "role_id", "roleId") == id {
			r.store.Delete(ctx, roleUsersResource, member.ID)
		}
	}
	for _, assignment := range r.store.List(ctx, assignmentsResource) {
		if shared.TextValue(assignment.Data, "target_type", "targetType") == "role" &&
			shared.TextValue(assignment.Data, "target_id", "targetId") == id {
			r.store.Delete(ctx, assignmentsResource, assignment.ID)
		}
	}
	r.store.Delete(ctx, platformRolesResource, id)
	return current, true
}

func (r recordStoreAuthorizationPolicyRepository) ListRoleUsers(ctx context.Context, roleID string) []map[string]any {
	if r.store == nil {
		return nil
	}
	rows := []map[string]any{}
	for _, member := range r.store.List(ctx, roleUsersResource) {
		if shared.TextValue(member.Data, "role_id", "roleId") == roleID {
			rows = append(rows, r.composeRoleUser(ctx, member.Data))
		}
	}
	sort.Slice(rows, func(i, j int) bool {
		return shared.TextValue(rows[i], "user_id", "userId") < shared.TextValue(rows[j], "user_id", "userId")
	})
	return rows
}

func (r recordStoreAuthorizationPolicyRepository) CreateRoleUser(ctx context.Context, roleID, userID, assignedBy string) (map[string]any, bool, error) {
	if r.store == nil {
		return nil, false, errAuthorizationPolicyRepositoryUnavailable
	}
	if existing, found := r.findRoleUser(ctx, roleID, userID); found {
		return r.composeRoleUser(ctx, existing.Data), false, nil
	}
	record, err := r.store.Create(ctx, roleUsersResource, map[string]any{
		"id":          roleID + ":" + userID,
		"user_id":     userID,
		"role_id":     roleID,
		"assigned_by": assignedBy,
		"created_at":  time.Now().UTC(),
	})
	if err == nil {
		return r.composeRoleUser(ctx, record.Data), true, nil
	}
	if !platform.IsCreateConflict(err) {
		return nil, false, err
	}
	if existing, found := r.findRoleUser(ctx, roleID, userID); found {
		return r.composeRoleUser(ctx, existing.Data), false, nil
	}
	return nil, false, err
}

func (r recordStoreAuthorizationPolicyRepository) CreateRoleUserTx(ctx context.Context, tx platform.StoreTx, roleID, userID, assignedBy string) (map[string]any, bool, error) {
	if r.store == nil {
		return nil, false, errAuthorizationPolicyRepositoryUnavailable
	}
	if existing, found := r.findRoleUser(ctx, roleID, userID); found {
		return r.composeRoleUser(ctx, existing.Data), false, nil
	}
	record, err := tx.Create(ctx, roleUsersResource, map[string]any{
		"id":          roleID + ":" + userID,
		"user_id":     userID,
		"role_id":     roleID,
		"assigned_by": assignedBy,
		"created_at":  time.Now().UTC(),
	})
	if err != nil {
		if platform.IsCreateConflict(err) {
			if existing, found := r.findRoleUser(ctx, roleID, userID); found {
				return r.composeRoleUser(ctx, existing.Data), false, nil
			}
		}
		return nil, false, err
	}
	return r.composeRoleUser(ctx, record.Data), true, nil
}

func (r recordStoreAuthorizationPolicyRepository) UnassignRoleUser(ctx context.Context, roleID, userID string) (map[string]any, bool) {
	if r.store == nil {
		return nil, false
	}
	if member, found := r.findRoleUser(ctx, roleID, userID); found {
		row := shared.CloneMap(member.Data)
		r.store.Delete(ctx, roleUsersResource, member.ID)
		return row, true
	}
	return nil, false
}

func (r recordStoreAuthorizationPolicyRepository) UnassignRoleUserTx(ctx context.Context, tx platform.StoreTx, roleID, userID string) (map[string]any, bool, error) {
	if r.store == nil {
		return nil, false, nil
	}
	if member, found := r.findRoleUser(ctx, roleID, userID); found {
		row := shared.CloneMap(member.Data)
		if _, err := tx.Delete(ctx, roleUsersResource, member.ID); err != nil {
			return nil, false, err
		}
		return row, true, nil
	}
	return nil, false, nil
}

func (r recordStoreAuthorizationPolicyRepository) replacePolicyRules(ctx context.Context, policyID string, rules []map[string]any) error {
	if r.store == nil {
		return errAuthorizationPolicyRepositoryUnavailable
	}
	for _, rule := range r.store.List(ctx, rulesResource) {
		if shared.TextValue(rule.Data, "policy_id", "policyId") == policyID {
			r.store.Delete(ctx, rulesResource, rule.ID)
		}
	}
	for _, rule := range rules {
		serviceID := shared.TextValue(rule, "service_id", "serviceId")
		record := map[string]any{
			"id":         shared.FirstNonEmpty(shared.TextValue(rule, "id"), r.nextID(rulesResource, "PR", 2600001)),
			"policy_id":  policyID,
			"service_id": serviceID,
			"actions":    shared.StringSlice(rule["actions"]),
		}
		if _, err := r.store.Create(ctx, rulesResource, record); err != nil {
			return err
		}
	}
	return nil
}

func (r recordStoreAuthorizationPolicyRepository) composePolicy(ctx context.Context, policy map[string]any) map[string]any {
	id := shared.TextValue(policy, "id")
	rules := []map[string]any{}
	if r.store != nil {
		for _, record := range r.store.List(ctx, rulesResource) {
			if shared.TextValue(record.Data, "policy_id", "policyId") != id {
				continue
			}
			rule := shared.CloneMap(record.Data)
			if rule["id"] == nil {
				rule["id"] = record.ID
			}
			rules = append(rules, rule)
		}
	}
	sort.Slice(rules, func(i, j int) bool {
		return shared.TextValue(rules[i], "service_id", "serviceId") < shared.TextValue(rules[j], "service_id", "serviceId")
	})
	policy["rules"] = rules
	return policy
}

func (r recordStoreAuthorizationPolicyRepository) composeAssignment(ctx context.Context, data map[string]any) map[string]any {
	row := shared.CloneMap(data)
	if policy, found := r.GetProxyPolicy(ctx, shared.TextValue(row, "policy_id", "policyId")); found {
		row["policy"] = policy
	}
	return row
}

func (r recordStoreAuthorizationPolicyRepository) composeRoleUser(ctx context.Context, data map[string]any) map[string]any {
	row := shared.CloneMap(data)
	if role, found := r.GetProxyRole(ctx, shared.TextValue(row, "role_id", "roleId")); found {
		row["role"] = role
	}
	return row
}

func (r recordStoreAuthorizationPolicyRepository) findAssignment(ctx context.Context, policyID, targetType, targetID string) (contracts.Record[map[string]any], bool) {
	if r.store == nil {
		return contracts.Record[map[string]any]{}, false
	}
	for _, assignment := range r.store.List(ctx, assignmentsResource) {
		if shared.TextValue(assignment.Data, "policy_id", "policyId") == policyID &&
			shared.TextValue(assignment.Data, "target_type", "targetType") == targetType &&
			shared.TextValue(assignment.Data, "target_id", "targetId") == targetID {
			return assignment, true
		}
	}
	return contracts.Record[map[string]any]{}, false
}

func (r recordStoreAuthorizationPolicyRepository) findRoleUser(ctx context.Context, roleID, userID string) (contracts.Record[map[string]any], bool) {
	if r.store == nil {
		return contracts.Record[map[string]any]{}, false
	}
	for _, member := range r.store.List(ctx, roleUsersResource) {
		if shared.TextValue(member.Data, "role_id", "roleId") == roleID &&
			shared.TextValue(member.Data, "user_id", "userId") == userID {
			return member, true
		}
	}
	return contracts.Record[map[string]any]{}, false
}

func (r recordStoreAuthorizationPolicyRepository) listMaps(ctx context.Context, resource string) []map[string]any {
	if r.store == nil {
		return nil
	}
	records := r.store.List(ctx, resource)
	out := make([]map[string]any, 0, len(records))
	for _, record := range records {
		row := shared.CloneMap(record.Data)
		if row["id"] == nil {
			row["id"] = record.ID
		}
		out = append(out, row)
	}
	return out
}

func (r recordStoreAuthorizationPolicyRepository) seedMarked(ctx context.Context, name string) bool {
	if r.store == nil {
		return false
	}
	_, found := r.store.Get(ctx, seedMarkersResource, name)
	return found
}

func (r recordStoreAuthorizationPolicyRepository) claimSeed(ctx context.Context, name string) bool {
	if r.store == nil {
		return false
	}
	_, err := r.store.Create(ctx, seedMarkersResource, map[string]any{
		"id":         name,
		"created_at": time.Now().UTC(),
	})
	return err == nil
}

func (r recordStoreAuthorizationPolicyRepository) seedCreate(ctx context.Context, resource string, data map[string]any) contracts.Record[map[string]any] {
	record, err := r.store.Create(ctx, resource, data)
	if err != nil {
		slog.Warn("seed create skipped", "resource", resource, "error", err)
	}
	return record
}

func (r recordStoreAuthorizationPolicyRepository) nextID(resource, prefix string, base int) string {
	if r.store == nil {
		return ""
	}
	return r.store.NextID(resource, prefix, base, 7)
}
