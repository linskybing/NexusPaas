package workload

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"sort"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/contracts"
	"github.com/linskybing/nexuspaas/backend/internal/platform"
	"github.com/linskybing/nexuspaas/backend/internal/services/shared"
)

const (
	configsResource   = serviceName + ":configfiles"
	versionsResource  = serviceName + ":configfiles:versions"
	instancesResource = serviceName + ":instances"
	commandsResource  = serviceName + ":instances:commands"

	configIDPrefix  = "CFG"
	versionIDPrefix = "VER"
	commandIDPrefix = "CMD"
	configIDStart   = 2600001
	configIDWidth   = 7
)

var errWorkloadConfigRepositoryUnavailable = errors.New("workload config repository unavailable")

type recordStoreWorkloadConfigRepository struct {
	store platform.RecordStore
}

func configRepository(app *platform.App) *recordStoreWorkloadConfigRepository {
	if app == nil {
		return &recordStoreWorkloadConfigRepository{}
	}
	return configRepositoryFromStore(app.Store)
}

func configRepositoryFromStore(store platform.RecordStore) *recordStoreWorkloadConfigRepository {
	return &recordStoreWorkloadConfigRepository{store: store}
}

func (r recordStoreWorkloadConfigRepository) NextConfigID() string {
	return r.nextID(configsResource, configIDPrefix)
}

func (r recordStoreWorkloadConfigRepository) NextVersionID() string {
	return r.nextID(versionsResource, versionIDPrefix)
}

func (r recordStoreWorkloadConfigRepository) NextCommandID() string {
	return r.nextID(commandsResource, commandIDPrefix)
}

func (r recordStoreWorkloadConfigRepository) CreateConfig(
	ctx context.Context,
	data map[string]any,
) (contracts.Record[map[string]any], error) {
	return r.create(ctx, configsResource, data)
}

// CreateConfigWithEvent persists a config file and its domain event in one
// transaction (when the store supports it), keeping the config resource key
// owned by this repository. Prefer this over CreateConfig + a separate publish
// so a crash cannot leave a config without its event.
func (r recordStoreWorkloadConfigRepository) CreateConfigWithEvent(
	ctx context.Context,
	app *platform.App,
	data map[string]any,
	build func(contracts.Record[map[string]any]) contracts.Event,
) (contracts.Record[map[string]any], error) {
	return app.CreateRecordWithEvent(ctx, configsResource, shared.CloneMap(data), build)
}

// UpdateConfigWithEvent is the update counterpart to CreateConfigWithEvent.
func (r recordStoreWorkloadConfigRepository) UpdateConfigWithEvent(
	ctx context.Context,
	app *platform.App,
	id string,
	data map[string]any,
	build func(contracts.Record[map[string]any]) contracts.Event,
) (contracts.Record[map[string]any], bool, error) {
	return app.UpdateRecordWithEvent(ctx, configsResource, id, shared.CloneMap(data), build)
}

// DeleteConfigWithEvent is the delete counterpart to CreateConfigWithEvent.
func (r recordStoreWorkloadConfigRepository) DeleteConfigWithEvent(
	ctx context.Context,
	app *platform.App,
	id string,
	build func(deleted bool) contracts.Event,
) (bool, error) {
	return app.DeleteRecordWithEvent(ctx, configsResource, id, build)
}

func (r recordStoreWorkloadConfigRepository) GetConfig(
	ctx context.Context,
	id string,
) (contracts.Record[map[string]any], bool) {
	return r.get(ctx, configsResource, id)
}

func (r recordStoreWorkloadConfigRepository) UpdateConfig(
	ctx context.Context,
	id string,
	data map[string]any,
) (contracts.Record[map[string]any], bool) {
	return r.update(ctx, configsResource, id, data)
}

func (r recordStoreWorkloadConfigRepository) DeleteConfig(ctx context.Context, id string) bool {
	return r.delete(ctx, configsResource, id)
}

func (r recordStoreWorkloadConfigRepository) ListConfigs(ctx context.Context) []contracts.Record[map[string]any] {
	records := r.listMatching(ctx, configsResource, func(contracts.Record[map[string]any]) bool {
		return true
	})
	sort.SliceStable(records, func(i, j int) bool {
		left := configSortKey(records[i])
		right := configSortKey(records[j])
		return left < right
	})
	return records
}

func (r recordStoreWorkloadConfigRepository) ListConfigsByProject(
	ctx context.Context,
	projectID string,
) []contracts.Record[map[string]any] {
	records := r.listMatching(ctx, configsResource, func(record contracts.Record[map[string]any]) bool {
		return shared.TextValue(record.Data, "project_id", "projectId") == projectID
	})
	sort.SliceStable(records, func(i, j int) bool {
		left := configSortKey(records[i])
		right := configSortKey(records[j])
		return left < right
	})
	return records
}

func (r recordStoreWorkloadConfigRepository) CommitVersion(
	ctx context.Context,
	configID string,
	data map[string]any,
	now time.Time,
) (contracts.Record[map[string]any], error) {
	if r.store == nil {
		return contracts.Record[map[string]any]{}, errWorkloadConfigRepositoryUnavailable
	}
	return r.create(ctx, versionsResource, committedVersionPayload(configID, data, now))
}

func (r recordStoreWorkloadConfigRepository) CommitVersionWithEvent(
	ctx context.Context,
	app *platform.App,
	configID string,
	data map[string]any,
	now time.Time,
	build func(contracts.Record[map[string]any]) contracts.Event,
) (contracts.Record[map[string]any], error) {
	if r.store == nil {
		return contracts.Record[map[string]any]{}, errWorkloadConfigRepositoryUnavailable
	}
	return app.CreateRecordWithEvent(ctx, versionsResource, committedVersionPayload(configID, data, now), build)
}

func (r recordStoreWorkloadConfigRepository) CreateVersion(
	ctx context.Context,
	configID string,
	data map[string]any,
	reason string,
	now time.Time,
) (contracts.Record[map[string]any], error) {
	if r.store == nil {
		return contracts.Record[map[string]any]{}, errWorkloadConfigRepositoryUnavailable
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	sum := sha256.Sum256([]byte(shared.TextValue(data, "content")))
	version := map[string]any{
		"id":         r.NextVersionID(),
		"config_id":  configID,
		"sha256":     hex.EncodeToString(sum[:]),
		"message":    shared.FirstNonEmpty(shared.TextValue(data, "message"), reason),
		"created_at": now.UTC().Format(time.RFC3339),
	}
	return r.create(ctx, versionsResource, version)
}

func (r recordStoreWorkloadConfigRepository) GetVersion(
	ctx context.Context,
	id string,
) (contracts.Record[map[string]any], bool) {
	return r.get(ctx, versionsResource, id)
}

func (r recordStoreWorkloadConfigRepository) ListVersionsForConfigs(
	ctx context.Context,
	configIDs map[string]bool,
) []contracts.Record[map[string]any] {
	if len(configIDs) == 0 {
		return nil
	}
	return r.listMatching(ctx, versionsResource, func(record contracts.Record[map[string]any]) bool {
		return configIDs[shared.TextValue(record.Data, "config_id", "configId")]
	})
}

func (r recordStoreWorkloadConfigRepository) ListInstancesByConfig(
	ctx context.Context,
	configID string,
) []contracts.Record[map[string]any] {
	return r.listMatching(ctx, instancesResource, func(record contracts.Record[map[string]any]) bool {
		return shared.TextValue(record.Data, "config_id", "configId") == configID
	})
}

func (r recordStoreWorkloadConfigRepository) CreateInstanceCommand(
	ctx context.Context,
	configID string,
	action string,
	data map[string]any,
	now time.Time,
) (contracts.Record[map[string]any], error) {
	if r.store == nil {
		return contracts.Record[map[string]any]{}, errWorkloadConfigRepositoryUnavailable
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	payload := shared.CloneMap(data)
	payload["id"] = r.NextCommandID()
	payload["config_id"] = configID
	payload["action"] = action
	payload["status"] = "accepted"
	payload["requested_at"] = now.UTC().Format(time.RFC3339)
	return r.create(ctx, commandsResource, payload)
}

func (r recordStoreWorkloadConfigRepository) CreateInstanceCommandWithEvent(
	ctx context.Context,
	app *platform.App,
	configID string,
	action string,
	data map[string]any,
	now time.Time,
	build func(contracts.Record[map[string]any]) contracts.Event,
) (contracts.Record[map[string]any], error) {
	if r.store == nil {
		return contracts.Record[map[string]any]{}, errWorkloadConfigRepositoryUnavailable
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	payload := shared.CloneMap(data)
	payload["id"] = r.NextCommandID()
	payload["config_id"] = configID
	payload["action"] = action
	payload["status"] = "accepted"
	payload["requested_at"] = now.UTC().Format(time.RFC3339)
	return app.CreateRecordWithEvent(ctx, commandsResource, payload, build)
}

func committedVersionPayload(configID string, data map[string]any, now time.Time) map[string]any {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	version := shared.CloneMap(data)
	sum := sha256.Sum256([]byte(shared.TextValue(version, "content")))
	version["sha256"] = hex.EncodeToString(sum[:])
	version["immutable"] = true
	version["config_id"] = shared.FirstNonEmpty(shared.TextValue(version, "config_id", "configId"), configID)
	version["committed_at"] = now.UTC().Format(time.RFC3339)
	return version
}

func (r recordStoreWorkloadConfigRepository) nextID(resource, prefix string) string {
	if r.store == nil {
		return ""
	}
	return r.store.NextID(resource, prefix, configIDStart, configIDWidth)
}

func (r recordStoreWorkloadConfigRepository) create(
	ctx context.Context,
	resource string,
	data map[string]any,
) (contracts.Record[map[string]any], error) {
	if r.store == nil {
		return contracts.Record[map[string]any]{}, errWorkloadConfigRepositoryUnavailable
	}
	record, err := r.store.Create(ctx, resource, shared.CloneMap(data))
	if err != nil {
		return contracts.Record[map[string]any]{}, err
	}
	return cloneWorkloadConfigRecord(record), nil
}

func (r recordStoreWorkloadConfigRepository) get(
	ctx context.Context,
	resource string,
	id string,
) (contracts.Record[map[string]any], bool) {
	if r.store == nil {
		return contracts.Record[map[string]any]{}, false
	}
	record, found := r.store.Get(ctx, resource, id)
	if !found {
		return contracts.Record[map[string]any]{}, false
	}
	return cloneWorkloadConfigRecord(record), true
}

func (r recordStoreWorkloadConfigRepository) update(
	ctx context.Context,
	resource string,
	id string,
	data map[string]any,
) (contracts.Record[map[string]any], bool) {
	if r.store == nil {
		return contracts.Record[map[string]any]{}, false
	}
	record, ok := r.store.Update(ctx, resource, id, shared.CloneMap(data))
	if !ok {
		return contracts.Record[map[string]any]{}, false
	}
	return cloneWorkloadConfigRecord(record), true
}

func (r recordStoreWorkloadConfigRepository) delete(ctx context.Context, resource, id string) bool {
	if r.store == nil {
		return false
	}
	return r.store.Delete(ctx, resource, id)
}

func (r recordStoreWorkloadConfigRepository) listMatching(
	ctx context.Context,
	resource string,
	matches func(contracts.Record[map[string]any]) bool,
) []contracts.Record[map[string]any] {
	if r.store == nil {
		return nil
	}
	records := []contracts.Record[map[string]any]{}
	for _, record := range r.store.List(ctx, resource) {
		clone := cloneWorkloadConfigRecord(record)
		if matches(clone) {
			records = append(records, clone)
		}
	}
	return records
}

func cloneWorkloadConfigRecord(record contracts.Record[map[string]any]) contracts.Record[map[string]any] {
	record.Data = shared.CloneMap(record.Data)
	return record
}

func configSortKey(record contracts.Record[map[string]any]) string {
	return shared.FirstNonEmpty(shared.TextValue(record.Data, "path"), shared.TextValue(record.Data, "name"), record.ID)
}
