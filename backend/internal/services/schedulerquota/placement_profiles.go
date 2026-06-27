package schedulerquota

import (
	"context"
	"net/http"
	"strings"

	"github.com/linskybing/nexuspaas/backend/internal/contracts"
	"github.com/linskybing/nexuspaas/backend/internal/platform"
	"github.com/linskybing/nexuspaas/backend/internal/services/shared"
)

const (
	placementProfilesResource     = serviceName + ":placement_profiles"
	placementDefaultSchedulerName = "default-scheduler"
	placementVolcanoSchedulerName = "volcano"
	kueueQueueNameLabel           = "kueue.x-k8s.io/queue-name"
	msgPlacementProfileSave       = "placement profile could not be saved"
)

func seedDefaultPlacementProfiles(app *platform.App) error {
	if app == nil || app.Store == nil || !app.Config.AllowsService(serviceName) {
		return nil
	}
	ctx := context.Background()
	for _, profile := range defaultPlacementProfiles() {
		id := shared.TextValue(profile, "id")
		if id == "" {
			continue
		}
		if _, found := app.Store.Get(ctx, placementProfilesResource, id); found {
			continue
		}
		if _, err := app.Store.Create(ctx, placementProfilesResource, profile); err != nil && !platform.IsCreateConflict(err) {
			return err
		}
	}
	return nil
}

func createPlacementProfile(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	payload, err := placementProfilePayload(r)
	if err != nil {
		return http.StatusBadRequest, shared.ErrorData(err.Error()), nil
	}
	if missing := missingPlacementProfileRequired(payload); missing != "" {
		return http.StatusBadRequest, shared.ErrorData("missing required field: " + missing), nil
	}
	record, err := app.CreateRecordWithEvent(r.Context(), placementProfilesResource, payload, func(record contracts.Record[map[string]any]) contracts.Event {
		return schedulerEvent(r, "PlacementProfileChanged", "created", placementProfileEventPayload(record.Data, "created"))
	})
	if err != nil {
		return placementProfileCreateError(err), shared.ErrorData(msgPlacementProfileSave), nil
	}
	return http.StatusCreated, record, nil
}

func updatePlacementProfile(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	id := strings.TrimSpace(r.PathValue("id"))
	payload, err := placementProfilePayload(r)
	if err != nil {
		return http.StatusBadRequest, shared.ErrorData(err.Error()), nil
	}
	if missing := missingPlacementProfileRequired(payload); missing != "" {
		return http.StatusBadRequest, shared.ErrorData("missing required field: " + missing), nil
	}
	if id == "" {
		id = shared.TextValue(payload, "id")
	}
	payload["id"] = id
	record, ok, err := app.UpdateRecordWithEvent(r.Context(), placementProfilesResource, id, payload, func(record contracts.Record[map[string]any]) contracts.Event {
		return schedulerEvent(r, "PlacementProfileChanged", "updated", placementProfileEventPayload(record.Data, "updated"))
	})
	if err != nil {
		return http.StatusInternalServerError, shared.ErrorData(msgPlacementProfileSave), nil
	}
	if !ok {
		record, err = app.CreateRecordWithEvent(r.Context(), placementProfilesResource, payload, func(record contracts.Record[map[string]any]) contracts.Event {
			return schedulerEvent(r, "PlacementProfileChanged", "updated", placementProfileEventPayload(record.Data, "updated"))
		})
		if err != nil {
			return placementProfileCreateError(err), shared.ErrorData(msgPlacementProfileSave), nil
		}
	}
	return http.StatusOK, record, nil
}

func deletePlacementProfile(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	id := strings.TrimSpace(r.PathValue("id"))
	deleted, err := app.DeleteRecordWithEvent(r.Context(), placementProfilesResource, id, func(deleted bool) contracts.Event {
		return schedulerEvent(r, "PlacementProfileChanged", "deleted", map[string]any{
			"id":                   id,
			"profile_id":           id,
			"placement_profile_id": id,
			"deleted":              deleted,
		})
	})
	if err != nil {
		return http.StatusInternalServerError, shared.ErrorData("placement profile could not be deleted"), nil
	}
	return http.StatusOK, map[string]any{"id": id, "deleted": deleted}, nil
}

func placementProfilePayload(r *http.Request) (map[string]any, error) {
	payload, err := platform.DecodeMapWithError(r)
	if err != nil {
		return nil, err
	}
	return shared.CloneMap(payload), nil
}

func missingPlacementProfileRequired(payload map[string]any) string {
	for _, field := range []string{"name", "scheduler_backend"} {
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

func placementProfileEventPayload(data map[string]any, action string) map[string]any {
	payload := shared.CloneMap(data)
	id := shared.TextValue(data, "id")
	payload["profile_id"] = id
	payload["placement_profile_id"] = id
	payload["action"] = action
	return payload
}

func placementProfileCreateError(err error) int {
	if platform.IsCreateConflict(err) {
		return http.StatusConflict
	}
	return http.StatusInternalServerError
}

func defaultPlacementProfiles() []map[string]any {
	// ponytail: profile metadata only; backend controllers remain cluster add-ons.
	return []map[string]any{
		{
			"id":                "default-kubernetes",
			"name":              "Default Kubernetes scheduler",
			"scheduler_backend": "kubernetes",
			"scheduler_name":    placementDefaultSchedulerName,
			"enabled":           true,
			"gang_enabled":      false,
			"labels":            map[string]any{},
			"annotations":       map[string]any{},
		},
		{
			"id":                "kueue-batch",
			"name":              "Kueue batch",
			"scheduler_backend": "kueue",
			"scheduler_name":    placementDefaultSchedulerName,
			"queue_label_key":   kueueQueueNameLabel,
			"enabled":           true,
			"gang_enabled":      false,
			"labels":            map[string]any{},
			"annotations":       map[string]any{},
		},
		{
			"id":                 "volcano-gang",
			"name":               "Volcano gang scheduling",
			"scheduler_backend":  "volcano",
			"scheduler_name":     placementVolcanoSchedulerName,
			"enabled":            true,
			"gang_enabled":       true,
			"gang_min_available": 1,
			"labels":             map[string]any{},
			"annotations":        map[string]any{},
		},
	}
}
