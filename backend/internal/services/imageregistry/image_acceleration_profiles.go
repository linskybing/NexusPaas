package imageregistry

import (
	"context"
	"net/http"
	"strings"

	"github.com/linskybing/nexuspaas/backend/internal/contracts"
	"github.com/linskybing/nexuspaas/backend/internal/platform"
	"github.com/linskybing/nexuspaas/backend/internal/services/shared"
)

const (
	imageAccelerationProfilesResource = serviceName + ":image_acceleration_profiles"
	msgImageAccelerationProfileSave   = "image acceleration profile could not be saved"
)

func seedDefaultImageAccelerationProfiles(app *platform.App) error {
	if app == nil || app.Store == nil || !app.Config.AllowsService(serviceName) {
		return nil
	}
	ctx := context.Background()
	for _, profile := range defaultImageAccelerationProfiles() {
		id := shared.TextValue(profile, "id")
		if id == "" {
			continue
		}
		if _, found := app.Store.Get(ctx, imageAccelerationProfilesResource, id); found {
			continue
		}
		if _, err := app.Store.Create(ctx, imageAccelerationProfilesResource, profile); err != nil && !platform.IsCreateConflict(err) {
			return err
		}
	}
	return nil
}

func createImageAccelerationProfile(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	payload, err := imageAccelerationProfilePayload(r)
	if err != nil {
		return http.StatusBadRequest, shared.ErrorData(err.Error()), nil
	}
	if missing := missingImageAccelerationProfileRequired(payload); missing != "" {
		return http.StatusBadRequest, shared.ErrorData("missing required field: " + missing), nil
	}
	record, err := app.CreateRecordWithEvent(r.Context(), imageAccelerationProfilesResource, payload, func(record contracts.Record[map[string]any]) contracts.Event {
		return registryEvent(r, "ImageAccelerationProfileChanged", imageAccelerationProfileEventPayload(record.Data, "created"))
	})
	if err != nil {
		return imageAccelerationProfileCreateError(err), shared.ErrorData(msgImageAccelerationProfileSave), nil
	}
	return http.StatusCreated, record, nil
}

func updateImageAccelerationProfile(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	id := strings.TrimSpace(r.PathValue("id"))
	payload, err := imageAccelerationProfilePayload(r)
	if err != nil {
		return http.StatusBadRequest, shared.ErrorData(err.Error()), nil
	}
	if missing := missingImageAccelerationProfileRequired(payload); missing != "" {
		return http.StatusBadRequest, shared.ErrorData("missing required field: " + missing), nil
	}
	if id == "" {
		id = shared.TextValue(payload, "id")
	}
	payload["id"] = id
	record, ok, err := app.UpdateRecordWithEvent(r.Context(), imageAccelerationProfilesResource, id, payload, func(record contracts.Record[map[string]any]) contracts.Event {
		return registryEvent(r, "ImageAccelerationProfileChanged", imageAccelerationProfileEventPayload(record.Data, "updated"))
	})
	if err != nil {
		return http.StatusInternalServerError, shared.ErrorData(msgImageAccelerationProfileSave), nil
	}
	if !ok {
		record, err = app.CreateRecordWithEvent(r.Context(), imageAccelerationProfilesResource, payload, func(record contracts.Record[map[string]any]) contracts.Event {
			return registryEvent(r, "ImageAccelerationProfileChanged", imageAccelerationProfileEventPayload(record.Data, "updated"))
		})
		if err != nil {
			return imageAccelerationProfileCreateError(err), shared.ErrorData(msgImageAccelerationProfileSave), nil
		}
	}
	return http.StatusOK, record, nil
}

func deleteImageAccelerationProfile(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	id := strings.TrimSpace(r.PathValue("id"))
	deleted, err := app.DeleteRecordWithEvent(r.Context(), imageAccelerationProfilesResource, id, func(deleted bool) contracts.Event {
		return registryEvent(r, "ImageAccelerationProfileChanged", map[string]any{
			"id":                            id,
			"profile_id":                    id,
			"image_acceleration_profile_id": id,
			"deleted":                       deleted,
			"action":                        "deleted",
		})
	})
	if err != nil {
		return http.StatusInternalServerError, shared.ErrorData("image acceleration profile could not be deleted"), nil
	}
	return http.StatusOK, map[string]any{"id": id, "deleted": deleted}, nil
}

func imageAccelerationProfilePayload(r *http.Request) (map[string]any, error) {
	payload, err := platform.DecodeMapWithError(r)
	if err != nil {
		return nil, err
	}
	return shared.CloneMap(payload), nil
}

func missingImageAccelerationProfileRequired(payload map[string]any) string {
	for _, field := range []string{"name", "snapshotter", "prewarm_policy"} {
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

func imageAccelerationProfileEventPayload(data map[string]any, action string) map[string]any {
	payload := shared.CloneMap(data)
	id := shared.TextValue(data, "id")
	payload["profile_id"] = id
	payload["image_acceleration_profile_id"] = id
	payload["action"] = action
	return payload
}

func imageAccelerationProfileCreateError(err error) int {
	if platform.IsCreateConflict(err) {
		return http.StatusConflict
	}
	return http.StatusInternalServerError
}

func defaultImageAccelerationProfiles() []map[string]any {
	// ponytail: these are policy hints only; real conversion/prewarm execution comes later.
	return []map[string]any{
		{
			"id":                   "standard-overlayfs",
			"name":                 "Standard overlayfs",
			"snapshotter":          "overlayfs",
			"prewarm_policy":       "none",
			"conversion_required":  false,
			"registry_mirror":      "",
			"allowed_for_projects": []any{},
			"enabled":              true,
		},
		{
			"id":                   "estargz-gpu-prewarm",
			"name":                 "eStargz GPU prewarm",
			"snapshotter":          "stargz",
			"prewarm_policy":       "nodepool-based",
			"conversion_required":  true,
			"registry_mirror":      "",
			"node_selector":        map[string]any{"nexuspaas.io/node-class": "gpu-hpc"},
			"allowed_for_projects": []any{},
			"enabled":              true,
		},
		{
			"id":                   "soci-inference-prewarm",
			"name":                 "SOCI inference prewarm",
			"snapshotter":          "soci",
			"prewarm_policy":       "queue-based",
			"conversion_required":  true,
			"registry_mirror":      "",
			"allowed_for_projects": []any{},
			"enabled":              true,
		},
	}
}
