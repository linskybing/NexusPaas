package identity

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/contracts"
	"github.com/linskybing/nexuspaas/backend/internal/platform"
	"github.com/linskybing/nexuspaas/backend/internal/services/shared"
)

const (
	usersResource = serviceName + ":users"
	rolesResource = serviceName + ":roles"
)

var errIdentityPrincipalStoreUnavailable = errors.New("identity principal repository unavailable")

type recordStoreIdentityPrincipalRepository struct {
	store platform.RecordStore
}

func principalRepository(app *platform.App) *recordStoreIdentityPrincipalRepository {
	if app == nil {
		return &recordStoreIdentityPrincipalRepository{}
	}
	return principalRepositoryFromStore(app.Store)
}

func principalRepositoryFromStore(store platform.RecordStore) *recordStoreIdentityPrincipalRepository {
	return &recordStoreIdentityPrincipalRepository{store: store}
}

func (r recordStoreIdentityPrincipalRepository) UserResourceName() string {
	return usersResource
}

func (r recordStoreIdentityPrincipalRepository) RoleResourceName() string {
	return rolesResource
}

func (r recordStoreIdentityPrincipalRepository) NextUserID() string {
	if r.store == nil {
		return ""
	}
	return r.store.NextID(usersResource, "US", 2600001, 0)
}

func (r recordStoreIdentityPrincipalRepository) ListUsers(ctx context.Context) []contracts.Record[map[string]any] {
	if r.store == nil {
		return nil
	}
	return cloneIdentityRecords(r.store.List(ctx, usersResource))
}

func (r recordStoreIdentityPrincipalRepository) GetUser(ctx context.Context, id string) (contracts.Record[map[string]any], bool) {
	if r.store == nil || strings.TrimSpace(id) == "" {
		return contracts.Record[map[string]any]{}, false
	}
	record, ok := r.store.Get(ctx, usersResource, id)
	if !ok {
		return contracts.Record[map[string]any]{}, false
	}
	return cloneIdentityRecord(record), true
}

func (r recordStoreIdentityPrincipalRepository) FindUserByUsername(ctx context.Context, username string) (contracts.Record[map[string]any], bool) {
	if r.store == nil {
		return contracts.Record[map[string]any]{}, false
	}
	username = strings.TrimSpace(username)
	if username == "" {
		return contracts.Record[map[string]any]{}, false
	}
	for _, record := range r.store.List(ctx, usersResource) {
		if strings.EqualFold(textValue(record.Data, "username"), username) {
			return cloneIdentityRecord(record), true
		}
	}
	return contracts.Record[map[string]any]{}, false
}

func (r recordStoreIdentityPrincipalRepository) FindUserByIdentifier(ctx context.Context, identifier string) (contracts.Record[map[string]any], bool) {
	identifier = strings.TrimSpace(identifier)
	if identifier == "" {
		return contracts.Record[map[string]any]{}, false
	}
	if user, ok := r.GetUser(ctx, identifier); ok {
		return user, true
	}
	if r.store == nil {
		return contracts.Record[map[string]any]{}, false
	}
	for _, user := range r.store.List(ctx, usersResource) {
		if textValue(user.Data, "username") == identifier || textValue(user.Data, "email") == identifier {
			return cloneIdentityRecord(user), true
		}
	}
	return contracts.Record[map[string]any]{}, false
}

func (r recordStoreIdentityPrincipalRepository) CreateUser(ctx context.Context, user map[string]any) (contracts.Record[map[string]any], error) {
	if r.store == nil {
		return contracts.Record[map[string]any]{}, errIdentityPrincipalStoreUnavailable
	}
	record, err := r.store.Create(ctx, usersResource, shared.CloneMap(user))
	if err != nil {
		return contracts.Record[map[string]any]{}, err
	}
	return cloneIdentityRecord(record), nil
}

func (r recordStoreIdentityPrincipalRepository) UpdateUser(ctx context.Context, id string, update map[string]any) (contracts.Record[map[string]any], bool) {
	if r.store == nil || strings.TrimSpace(id) == "" {
		return contracts.Record[map[string]any]{}, false
	}
	record, ok := r.store.Update(ctx, usersResource, id, shared.CloneMap(update))
	if !ok {
		return contracts.Record[map[string]any]{}, false
	}
	return cloneIdentityRecord(record), true
}

func (r recordStoreIdentityPrincipalRepository) DeleteUser(ctx context.Context, id string) bool {
	if r.store == nil || strings.TrimSpace(id) == "" {
		return false
	}
	return r.store.Delete(ctx, usersResource, id)
}

func (r recordStoreIdentityPrincipalRepository) SetUserStatus(ctx context.Context, id, status string) bool {
	_, ok := r.UpdateUser(ctx, id, map[string]any{"status": status})
	return ok
}

func (r recordStoreIdentityPrincipalRepository) GetUserSettings(ctx context.Context, id string) (map[string]any, bool) {
	record, ok := r.GetUser(ctx, id)
	if !ok {
		return nil, false
	}
	return shared.CloneMap(shared.MapValue(record.Data, "settings")), true
}

func (r recordStoreIdentityPrincipalRepository) UpdateUserSettings(ctx context.Context, id string, settings map[string]any, now time.Time) (map[string]any, bool) {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	record, ok := r.UpdateUser(ctx, id, map[string]any{
		"settings":   shared.CloneMap(settings),
		"updated_at": now.Format(time.RFC3339),
	})
	if !ok {
		return nil, false
	}
	return shared.CloneMap(shared.MapValue(record.Data, "settings")), true
}

func (r recordStoreIdentityPrincipalRepository) ListRoles(ctx context.Context) []contracts.Record[map[string]any] {
	if r.store == nil {
		return nil
	}
	return cloneIdentityRecords(r.store.List(ctx, rolesResource))
}

func (r recordStoreIdentityPrincipalRepository) GetRole(ctx context.Context, id string) (contracts.Record[map[string]any], bool) {
	if r.store == nil || strings.TrimSpace(id) == "" {
		return contracts.Record[map[string]any]{}, false
	}
	record, ok := r.store.Get(ctx, rolesResource, id)
	if !ok {
		return contracts.Record[map[string]any]{}, false
	}
	return cloneIdentityRecord(record), true
}

func cloneIdentityRecords(records []contracts.Record[map[string]any]) []contracts.Record[map[string]any] {
	out := make([]contracts.Record[map[string]any], 0, len(records))
	for _, record := range records {
		out = append(out, cloneIdentityRecord(record))
	}
	return out
}

func cloneIdentityRecord(record contracts.Record[map[string]any]) contracts.Record[map[string]any] {
	data := shared.CloneMap(record.Data)
	if data["id"] == nil && record.ID != "" {
		data["id"] = record.ID
	}
	return contracts.Record[map[string]any]{
		ID:        record.ID,
		Data:      data,
		CreatedAt: record.CreatedAt,
		UpdatedAt: record.UpdatedAt,
	}
}
