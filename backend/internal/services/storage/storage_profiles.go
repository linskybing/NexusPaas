package storage

import (
	"context"
	"net/http"
	"strings"

	"github.com/linskybing/nexuspaas/backend/internal/contracts"
	"github.com/linskybing/nexuspaas/backend/internal/platform"
	"github.com/linskybing/nexuspaas/backend/internal/services/shared"
)

const storageProfilesResource = serviceName + ":storage_profiles"

func seedDefaultStorageProfiles(app *platform.App) error {
	if app == nil || app.Store == nil || !app.Config.AllowsService(serviceName) {
		return nil
	}
	ctx := context.Background()
	for _, profile := range defaultStorageProfiles() {
		id, _ := profile["id"].(string)
		if id == "" {
			continue
		}
		if _, ok := app.Store.Get(ctx, storageProfilesResource, id); ok {
			continue
		}
		if _, err := app.Store.Create(ctx, storageProfilesResource, profile); err != nil && !platform.IsCreateConflict(err) {
			return err
		}
	}
	return nil
}

func createStorageProfile(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	payload, err := storageProfilePayload(r)
	if err != nil {
		return http.StatusBadRequest, shared.ErrorData(err.Error()), nil
	}
	if missing := missingStorageProfileRequired(payload); missing != "" {
		return http.StatusBadRequest, shared.ErrorData("missing required field: " + missing), nil
	}
	record, err := app.CreateRecordWithEvent(r.Context(), storageProfilesResource, payload, func(record contracts.Record[map[string]any]) contracts.Event {
		return storageEvent(r, "StorageProfileChanged", storageProfileEventPayload(record.Data, "created"))
	})
	if err != nil {
		return storageProfileCreateError(err), shared.ErrorData("storage profile could not be saved"), nil
	}
	return http.StatusCreated, record, nil
}

func updateStorageProfile(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	id := strings.TrimSpace(r.PathValue("id"))
	payload, err := storageProfilePayload(r)
	if err != nil {
		return http.StatusBadRequest, shared.ErrorData(err.Error()), nil
	}
	if missing := missingStorageProfileRequired(payload); missing != "" {
		return http.StatusBadRequest, shared.ErrorData("missing required field: " + missing), nil
	}
	if id == "" {
		id = shared.TextValue(payload, "id")
	}
	payload["id"] = id
	record, ok, err := app.UpdateRecordWithEvent(r.Context(), storageProfilesResource, id, payload, func(record contracts.Record[map[string]any]) contracts.Event {
		return storageEvent(r, "StorageProfileChanged", storageProfileEventPayload(record.Data, "updated"))
	})
	if err != nil {
		return http.StatusInternalServerError, shared.ErrorData("storage profile could not be saved"), nil
	}
	if !ok {
		record, err = app.CreateRecordWithEvent(r.Context(), storageProfilesResource, payload, func(record contracts.Record[map[string]any]) contracts.Event {
			return storageEvent(r, "StorageProfileChanged", storageProfileEventPayload(record.Data, "updated"))
		})
		if err != nil {
			return storageProfileCreateError(err), shared.ErrorData("storage profile could not be saved"), nil
		}
	}
	return http.StatusOK, record, nil
}

func deleteStorageProfile(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	id := strings.TrimSpace(r.PathValue("id"))
	deleted, err := app.DeleteRecordWithEvent(r.Context(), storageProfilesResource, id, func(deleted bool) contracts.Event {
		return storageEvent(r, "StorageProfileChanged", map[string]any{"profile_id": id, "id": id, "action": "deleted", "deleted": deleted})
	})
	if err != nil {
		return http.StatusInternalServerError, shared.ErrorData("storage profile could not be deleted"), nil
	}
	return http.StatusOK, map[string]any{"id": id, "deleted": deleted}, nil
}

func storageProfilePayload(r *http.Request) (map[string]any, error) {
	payload, err := platform.DecodeMapWithError(r)
	if err != nil {
		return nil, err
	}
	return shared.CloneMap(payload), nil
}

func missingStorageProfileRequired(payload map[string]any) string {
	for _, field := range []string{"name", "provider", "tier", "access_mode"} {
		value, ok := payload[field]
		if !ok {
			return field
		}
		if text, ok := value.(string); ok && strings.TrimSpace(text) == "" {
			return field
		}
	}
	return ""
}

func storageProfileEventPayload(data map[string]any, action string) map[string]any {
	payload := shared.CloneMap(data)
	payload["profile_id"] = shared.TextValue(data, "id")
	payload["action"] = action
	return payload
}

func storageProfileCreateError(err error) int {
	if platform.IsCreateConflict(err) {
		return http.StatusConflict
	}
	return http.StatusInternalServerError
}

func defaultStorageProfiles() []map[string]any {
	// ponytail: four real startup profiles stay inline; add config when deployments need site-specific defaults.
	return []map[string]any{
		{
			"id":                     "longhorn-rwx-standard",
			"name":                   "Longhorn RWX standard",
			"provider":               "longhorn",
			"tier":                   "standard",
			"access_mode":            "rwx",
			"performance_class":      "standard",
			"storage_class_name":     "longhorn-rwx-standard",
			"mount_mode":             "nfs",
			"mount_options":          []any{},
			"node_selector":          map[string]any{},
			"topology_policy":        "none",
			"allow_cross_namespace":  false,
			"allowed_project_scopes": []any{},
		},
		{
			"id":                     "cephfs-rwx-authority",
			"name":                   "CephFS RWX authority",
			"provider":               "cephfs",
			"tier":                   "authority",
			"access_mode":            "rwx",
			"performance_class":      "high-throughput",
			"storage_class_name":     "cephfs-rwx-authority",
			"mount_mode":             "kernel",
			"mount_options":          []any{"noatime"},
			"node_selector":          map[string]any{},
			"topology_policy":        "none",
			"allow_cross_namespace":  false,
			"allowed_project_scopes": []any{},
		},
		{
			"id":                     "local-nvme-scratch",
			"name":                   "Local NVMe scratch",
			"provider":               "local-nvme",
			"tier":                   "hot-scratch",
			"access_mode":            "rwo",
			"performance_class":      "low-latency",
			"storage_class_name":     "local-nvme-scratch",
			"mount_mode":             "local-pv",
			"mount_options":          []any{},
			"node_selector":          map[string]any{"nexuspaas.io/local-nvme": "true"},
			"topology_policy":        "same-node",
			"allow_cross_namespace":  false,
			"allowed_project_scopes": []any{},
		},
		{
			"id":                     "minio-artifact",
			"name":                   "MinIO artifact",
			"provider":               "minio",
			"tier":                   "object-artifact",
			"access_mode":            "object",
			"performance_class":      "standard",
			"mount_mode":             "s3",
			"mount_options":          []any{},
			"node_selector":          map[string]any{},
			"topology_policy":        "none",
			"allow_cross_namespace":  false,
			"allowed_project_scopes": []any{},
		},
	}
}
