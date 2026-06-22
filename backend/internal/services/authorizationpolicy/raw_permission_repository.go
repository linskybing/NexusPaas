package authorizationpolicy

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"

	"github.com/linskybing/nexuspaas/backend/internal/platform"
	"github.com/linskybing/nexuspaas/backend/internal/services/shared"
)

const (
	rawPoliciesResource            = serviceName + ":permission_policies"
	groupingResource               = serviceName + ":permission_grouping_policies"
	msgGroupingPolicyUpdateSkipped = "grouping policy update skipped"
)

var errRawPermissionRepositoryUnavailable = errors.New("raw permission repository unavailable")

type rawPermissionPolicyUpdateResult struct {
	Found    bool
	Conflict bool
	Updated  bool
}

type rawPermissionLookup interface {
	RawPermissionAllowed(context.Context, string, string, string, string) (bool, error)
}

type recordStoreRawPermissionRepository struct {
	store platform.RecordStore
}

func rawPermissionRepo(app *platform.App) *recordStoreRawPermissionRepository {
	if app == nil {
		return &recordStoreRawPermissionRepository{}
	}
	return &recordStoreRawPermissionRepository{store: app.Store}
}

func rawPermissionRepoFromStore(store platform.RecordStore) *recordStoreRawPermissionRepository {
	return &recordStoreRawPermissionRepository{store: store}
}

func (r recordStoreRawPermissionRepository) ListRawPermissionPolicies(ctx context.Context) [][]string {
	if r.store == nil {
		return nil
	}
	rows := [][]string{}
	for _, record := range r.store.List(ctx, rawPoliciesResource) {
		policy := policySlice(record.Data["policy"])
		if len(policy) == 0 {
			for i := 0; ; i++ {
				value := shared.TextValue(record.Data, fmt.Sprintf("v%d", i))
				if value == "" {
					break
				}
				policy = append(policy, value)
			}
		}
		if len(policy) > 0 {
			rows = append(rows, policy)
		}
	}
	sort.Slice(rows, func(i, j int) bool {
		return rawPolicyID(rows[i]) < rawPolicyID(rows[j])
	})
	return rows
}

func (r recordStoreRawPermissionRepository) RawPermissionPolicyExists(ctx context.Context, policy []string) bool {
	if r.store == nil {
		return false
	}
	_, found := r.store.Get(ctx, rawPoliciesResource, rawPolicyID(policy))
	return found
}

func (r recordStoreRawPermissionRepository) CreateRawPermissionPolicy(ctx context.Context, policy []string) (bool, error) {
	if r.store == nil {
		return false, errRawPermissionRepositoryUnavailable
	}
	if r.RawPermissionPolicyExists(ctx, policy) {
		return false, nil
	}
	if _, err := r.store.Create(ctx, rawPoliciesResource, rawPolicyRecord(policy)); err != nil {
		if platform.IsCreateConflict(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (r recordStoreRawPermissionRepository) CreateRawPermissionPolicyTx(ctx context.Context, tx platform.StoreTx, policy []string) (bool, error) {
	if r.store == nil {
		return false, errRawPermissionRepositoryUnavailable
	}
	if r.RawPermissionPolicyExists(ctx, policy) {
		return false, nil
	}
	if _, err := tx.Create(ctx, rawPoliciesResource, rawPolicyRecord(policy)); err != nil {
		if platform.IsCreateConflict(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (r recordStoreRawPermissionRepository) CreateRawPermissionPolicyRecord(ctx context.Context, policy []string, metadata map[string]any) (bool, error) {
	if r.store == nil {
		return false, errRawPermissionRepositoryUnavailable
	}
	if r.RawPermissionPolicyExists(ctx, policy) {
		return false, nil
	}
	record := rawPolicyRecord(policy)
	for key, value := range metadata {
		record[key] = value
	}
	if _, err := r.store.Create(ctx, rawPoliciesResource, record); err != nil {
		if platform.IsCreateConflict(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (r recordStoreRawPermissionRepository) ListRawPermissionPolicyRecords(ctx context.Context) []map[string]any {
	if r.store == nil {
		return nil
	}
	rows := []map[string]any{}
	for _, record := range r.store.List(ctx, rawPoliciesResource) {
		row := shared.CloneMap(record.Data)
		row["id"] = record.ID
		rows = append(rows, row)
	}
	return rows
}

func (r recordStoreRawPermissionRepository) DeleteRawPermissionPolicyRecord(ctx context.Context, id string) bool {
	if r.store == nil {
		return false
	}
	return r.store.Delete(ctx, rawPoliciesResource, id)
}

func (r recordStoreRawPermissionRepository) UpdateRawPermissionPolicy(ctx context.Context, oldPolicy, newPolicy []string) (rawPermissionPolicyUpdateResult, error) {
	if r.store == nil {
		return rawPermissionPolicyUpdateResult{}, errRawPermissionRepositoryUnavailable
	}
	oldID := rawPolicyID(oldPolicy)
	if _, found := r.store.Get(ctx, rawPoliciesResource, oldID); !found {
		return rawPermissionPolicyUpdateResult{}, nil
	}
	newID := rawPolicyID(newPolicy)
	result := rawPermissionPolicyUpdateResult{Found: true}
	if oldID != newID {
		if _, found := r.store.Get(ctx, rawPoliciesResource, newID); found {
			result.Conflict = true
			return result, nil
		}
		if _, err := r.store.Create(ctx, rawPoliciesResource, rawPolicyRecord(newPolicy)); err != nil {
			if platform.IsCreateConflict(err) {
				result.Conflict = true
				return result, nil
			}
			return result, err
		}
		r.store.Delete(ctx, rawPoliciesResource, oldID)
		result.Updated = true
		return result, nil
	}
	_, result.Updated = r.store.Update(ctx, rawPoliciesResource, oldID, rawPolicyRecord(newPolicy))
	if !result.Updated {
		slog.Warn("raw permission policy update skipped", "policy_id", oldID)
	}
	return result, nil
}

func (r recordStoreRawPermissionRepository) UpdateRawPermissionPolicyTx(ctx context.Context, tx platform.StoreTx, oldPolicy, newPolicy []string) (rawPermissionPolicyUpdateResult, error) {
	if r.store == nil {
		return rawPermissionPolicyUpdateResult{}, errRawPermissionRepositoryUnavailable
	}
	oldID := rawPolicyID(oldPolicy)
	if _, found := r.store.Get(ctx, rawPoliciesResource, oldID); !found {
		return rawPermissionPolicyUpdateResult{}, nil
	}
	newID := rawPolicyID(newPolicy)
	result := rawPermissionPolicyUpdateResult{Found: true}
	if oldID != newID {
		if _, found := r.store.Get(ctx, rawPoliciesResource, newID); found {
			result.Conflict = true
			return result, nil
		}
		if _, err := tx.Create(ctx, rawPoliciesResource, rawPolicyRecord(newPolicy)); err != nil {
			if platform.IsCreateConflict(err) {
				result.Conflict = true
				return result, nil
			}
			return result, err
		}
		if _, err := tx.Delete(ctx, rawPoliciesResource, oldID); err != nil {
			return result, err
		}
		result.Updated = true
		return result, nil
	}
	_, updated, err := tx.Update(ctx, rawPoliciesResource, oldID, rawPolicyRecord(newPolicy))
	if err != nil {
		return result, err
	}
	result.Updated = updated
	if !result.Updated {
		slog.Warn("raw permission policy update skipped", "policy_id", oldID)
	}
	return result, nil
}

func (r recordStoreRawPermissionRepository) DeleteRawPermissionPolicy(ctx context.Context, policy []string) bool {
	if r.store == nil {
		return false
	}
	return r.store.Delete(ctx, rawPoliciesResource, rawPolicyID(policy))
}

func (r recordStoreRawPermissionRepository) DeleteRawPermissionPolicyTx(ctx context.Context, tx platform.StoreTx, policy []string) (bool, error) {
	if r.store == nil {
		return false, nil
	}
	return tx.Delete(ctx, rawPoliciesResource, rawPolicyID(policy))
}

func (r recordStoreRawPermissionRepository) RawPermissionAllowed(ctx context.Context, subject, domain, object, action string) (bool, error) {
	if r.store == nil {
		return false, nil
	}
	policy := []string{subject, domain, object, action}
	_, found := r.store.Get(ctx, rawPoliciesResource, rawPolicyID(policy))
	return found, nil
}

func (r recordStoreRawPermissionRepository) ApplyPermissionOperation(ctx context.Context, op map[string]string) error {
	var domain string
	switch op["type"] {
	case "project_member":
		domain = op["project_id"]
	case "group_role":
		domain = op["group_id"]
	default:
		return fmt.Errorf("unsupported operation type: %s", op["type"])
	}
	switch op["action"] {
	case "add", "update":
		return r.UpsertGroupingPolicy(ctx, op["type"], op["user_id"], op["role"], domain)
	case "remove":
		r.DeleteGroupingPolicy(ctx, op["type"], op["user_id"], op["role"], domain)
	}
	return nil
}

func (r recordStoreRawPermissionRepository) ApplyPermissionOperationTx(ctx context.Context, tx platform.StoreTx, op map[string]string) error {
	var domain string
	switch op["type"] {
	case "project_member":
		domain = op["project_id"]
	case "group_role":
		domain = op["group_id"]
	default:
		return fmt.Errorf("unsupported operation type: %s", op["type"])
	}
	switch op["action"] {
	case "add", "update":
		return r.UpsertGroupingPolicyTx(ctx, tx, op["type"], op["user_id"], op["role"], domain)
	case "remove":
		_, err := r.DeleteGroupingPolicyTx(ctx, tx, op["type"], op["user_id"], op["role"], domain)
		return err
	}
	return nil
}

func (r recordStoreRawPermissionRepository) UpsertGroupingPolicy(ctx context.Context, opType, userID, role, domain string) error {
	if r.store == nil {
		return errRawPermissionRepositoryUnavailable
	}
	id := groupingPolicyID(opType, userID, role, domain)
	record := groupingPolicyRecord(opType, userID, role, domain)
	if _, found := r.store.Get(ctx, groupingResource, id); found {
		if _, ok := r.store.Update(ctx, groupingResource, id, record); !ok {
			slog.Warn(msgGroupingPolicyUpdateSkipped, "grouping_id", id)
		}
		return nil
	}
	if _, err := r.store.Create(ctx, groupingResource, record); err != nil {
		if platform.IsCreateConflict(err) {
			if _, ok := r.store.Update(ctx, groupingResource, id, record); !ok {
				slog.Warn(msgGroupingPolicyUpdateSkipped, "grouping_id", id)
			}
			return nil
		}
		return err
	}
	return nil
}

func (r recordStoreRawPermissionRepository) UpsertGroupingPolicyTx(ctx context.Context, tx platform.StoreTx, opType, userID, role, domain string) error {
	if r.store == nil {
		return errRawPermissionRepositoryUnavailable
	}
	id := groupingPolicyID(opType, userID, role, domain)
	record := groupingPolicyRecord(opType, userID, role, domain)
	if _, found := r.store.Get(ctx, groupingResource, id); found {
		if _, ok, err := tx.Update(ctx, groupingResource, id, record); err != nil {
			return err
		} else if !ok {
			slog.Warn(msgGroupingPolicyUpdateSkipped, "grouping_id", id)
		}
		return nil
	}
	if _, err := tx.Create(ctx, groupingResource, record); err != nil {
		if platform.IsCreateConflict(err) {
			if _, ok, err := tx.Update(ctx, groupingResource, id, record); err != nil {
				return err
			} else if !ok {
				slog.Warn(msgGroupingPolicyUpdateSkipped, "grouping_id", id)
			}
			return nil
		}
		return err
	}
	return nil
}

func (r recordStoreRawPermissionRepository) DeleteGroupingPolicy(ctx context.Context, opType, userID, role, domain string) bool {
	if r.store == nil {
		return false
	}
	return r.store.Delete(ctx, groupingResource, groupingPolicyID(opType, userID, role, domain))
}

func (r recordStoreRawPermissionRepository) DeleteGroupingPolicyTx(ctx context.Context, tx platform.StoreTx, opType, userID, role, domain string) (bool, error) {
	if r.store == nil {
		return false, errRawPermissionRepositoryUnavailable
	}
	return tx.Delete(ctx, groupingResource, groupingPolicyID(opType, userID, role, domain))
}

func (r recordStoreRawPermissionRepository) ListGroupingPolicies(ctx context.Context) []map[string]any {
	if r.store == nil {
		return nil
	}
	rows := make([]map[string]any, 0)
	for _, record := range r.store.List(ctx, groupingResource) {
		rows = append(rows, shared.CloneMap(record.Data))
	}
	sort.Slice(rows, func(i, j int) bool {
		return shared.TextValue(rows[i], "id") < shared.TextValue(rows[j], "id")
	})
	return rows
}

func groupingPolicyRecord(opType, userID, role, domain string) map[string]any {
	return map[string]any{
		"id":      groupingPolicyID(opType, userID, role, domain),
		"type":    opType,
		"user_id": userID,
		"role":    role,
		"domain":  domain,
	}
}

func groupingPolicyID(opType, userID, role, domain string) string {
	return strings.Join([]string{opType, userID, role, domain}, "\x1f")
}
