package schedulerquota

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/linskybing/nexuspaas/backend/internal/contracts"
	"github.com/linskybing/nexuspaas/backend/internal/platform"
	"github.com/linskybing/nexuspaas/backend/internal/services/shared"
	"k8s.io/apimachinery/pkg/api/resource"
)

const acceleratorProfilesResource = serviceName + ":accelerator_profiles"

func seedDefaultAcceleratorProfiles(app *platform.App) error {
	if app == nil || app.Store == nil || !app.Config.AllowsService(serviceName) {
		return nil
	}
	ctx := context.Background()
	for _, profile := range defaultAcceleratorProfiles() {
		id := shared.TextValue(profile, "id")
		if id == "" {
			continue
		}
		if _, found := app.Store.Get(ctx, acceleratorProfilesResource, id); found {
			continue
		}
		if _, err := app.Store.Create(ctx, acceleratorProfilesResource, profile); err != nil && !platform.IsCreateConflict(err) {
			return err
		}
	}
	return nil
}

func createAcceleratorProfile(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	payload, err := acceleratorProfilePayload(r)
	if err != nil {
		return http.StatusBadRequest, shared.ErrorData(err.Error()), nil
	}
	record, err := app.CreateRecordWithEvent(r.Context(), acceleratorProfilesResource, payload, func(record contracts.Record[map[string]any]) contracts.Event {
		return schedulerEvent(r, "AcceleratorProfileChanged", "created", acceleratorProfileEventPayload(record.Data, "created"))
	})
	if err != nil {
		return acceleratorProfileCreateError(err), shared.ErrorData("accelerator profile could not be saved"), nil
	}
	return http.StatusCreated, record, nil
}

func updateAcceleratorProfile(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	id := strings.TrimSpace(r.PathValue("id"))
	payload, err := acceleratorProfilePayload(r)
	if err != nil {
		return http.StatusBadRequest, shared.ErrorData(err.Error()), nil
	}
	if id == "" {
		id = shared.TextValue(payload, "id")
	}
	payload["id"] = id
	record, ok, err := app.UpdateRecordWithEvent(r.Context(), acceleratorProfilesResource, id, payload, func(record contracts.Record[map[string]any]) contracts.Event {
		return schedulerEvent(r, "AcceleratorProfileChanged", "updated", acceleratorProfileEventPayload(record.Data, "updated"))
	})
	if err != nil {
		return http.StatusInternalServerError, shared.ErrorData("accelerator profile could not be saved"), nil
	}
	if !ok {
		record, err = app.CreateRecordWithEvent(r.Context(), acceleratorProfilesResource, payload, func(record contracts.Record[map[string]any]) contracts.Event {
			return schedulerEvent(r, "AcceleratorProfileChanged", "updated", acceleratorProfileEventPayload(record.Data, "updated"))
		})
		if err != nil {
			return acceleratorProfileCreateError(err), shared.ErrorData("accelerator profile could not be saved"), nil
		}
	}
	return http.StatusOK, record, nil
}

func deleteAcceleratorProfile(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	id := strings.TrimSpace(r.PathValue("id"))
	deleted, err := app.DeleteRecordWithEvent(r.Context(), acceleratorProfilesResource, id, func(deleted bool) contracts.Event {
		return schedulerEvent(r, "AcceleratorProfileChanged", "deleted", map[string]any{
			"id":                     id,
			"profile_id":             id,
			"accelerator_profile_id": id,
			"deleted":                deleted,
		})
	})
	if err != nil {
		return http.StatusInternalServerError, shared.ErrorData("accelerator profile could not be deleted"), nil
	}
	return http.StatusOK, map[string]any{"id": id, "deleted": deleted}, nil
}

func acceleratorProfilePayload(r *http.Request) (map[string]any, error) {
	payload, err := platform.DecodeMapWithError(r)
	if err != nil {
		return nil, err
	}
	return normalizeAcceleratorProfilePayload(shared.CloneMap(payload))
}

func normalizeAcceleratorProfilePayload(payload map[string]any) (map[string]any, error) {
	if strings.TrimSpace(shared.TextValue(payload, "name")) == "" {
		return nil, fmt.Errorf("missing required field: name")
	}
	if value, found := firstPresent(payload, "enabled", "Enabled"); found {
		if _, ok := value.(bool); !ok {
			return nil, fmt.Errorf("enabled must be a boolean")
		}
	} else {
		payload["enabled"] = true
	}
	nodeSelector, err := acceleratorStringMap(payload["node_selector"])
	if err != nil {
		return nil, fmt.Errorf("node_selector %w", err)
	}
	labels, err := acceleratorStringMap(payload["labels"])
	if err != nil {
		return nil, fmt.Errorf("labels %w", err)
	}
	payload["node_selector"] = nodeSelector
	payload["labels"] = labels
	if value, found := firstPresent(payload, "default_mps_sm_percentage", "defaultMpsSmPercentage"); found {
		sm := int(getInt64(value, 0))
		if sm < 1 || sm > 100 {
			return nil, fmt.Errorf("default_mps_sm_percentage must be between 1 and 100")
		}
		payload["default_mps_sm_percentage"] = sm
	}
	if pinned := shared.TextValue(payload, "default_pinned_memory_limit", "defaultPinnedMemoryLimit"); pinned != "" {
		if _, err := resource.ParseQuantity(pinned); err != nil {
			return nil, fmt.Errorf("invalid default_pinned_memory_limit %q: %w", pinned, err)
		}
		payload["default_pinned_memory_limit"] = pinned
	}
	if value, found := firstPresent(payload, "allowed_device_class_name", "allowedDeviceClassName"); found {
		text, ok := value.(string)
		if !ok || strings.TrimSpace(text) == "" {
			return nil, fmt.Errorf("allowed_device_class_name must be a non-empty string")
		}
		payload["allowed_device_class_name"] = strings.TrimSpace(text)
	}
	return payload, nil
}

func acceleratorStringMap(raw any) (map[string]any, error) {
	if raw == nil {
		return map[string]any{}, nil
	}
	switch typed := raw.(type) {
	case map[string]any:
		out := map[string]any{}
		for key, value := range typed {
			text, ok := value.(string)
			if !ok {
				return nil, fmt.Errorf("must contain string values")
			}
			key = strings.TrimSpace(key)
			text = strings.TrimSpace(text)
			if key == "" || text == "" {
				continue
			}
			out[key] = text
		}
		return out, nil
	case map[string]string:
		out := map[string]any{}
		for key, value := range typed {
			key = strings.TrimSpace(key)
			value = strings.TrimSpace(value)
			if key == "" || value == "" {
				continue
			}
			out[key] = value
		}
		return out, nil
	default:
		return nil, fmt.Errorf("must be an object")
	}
}

func acceleratorProfileEventPayload(data map[string]any, action string) map[string]any {
	id := shared.TextValue(data, "id")
	return map[string]any{
		"id":                          id,
		"profile_id":                  id,
		"accelerator_profile_id":      id,
		"name":                        shared.TextValue(data, "name"),
		"enabled":                     acceleratorProfileEnabled(data),
		"accelerator_node_selector":   admissionStringMap(data["node_selector"]),
		"accelerator_labels":          admissionStringMap(data["labels"]),
		"allowed_device_class_name":   shared.TextValue(data, "allowed_device_class_name", "allowedDeviceClassName"),
		"default_mps_sm_percentage":   shared.IntValue(data, "default_mps_sm_percentage", "defaultMpsSmPercentage"),
		"default_pinned_memory_limit": shared.TextValue(data, "default_pinned_memory_limit", "defaultPinnedMemoryLimit"),
		"action":                      action,
	}
}

func acceleratorProfileEnabled(data map[string]any) bool {
	value, found := firstPresent(data, "enabled", "Enabled")
	if !found {
		return true
	}
	enabled, ok := value.(bool)
	return !ok || enabled
}

func acceleratorProfileCreateError(err error) int {
	if platform.IsCreateConflict(err) {
		return http.StatusConflict
	}
	return http.StatusInternalServerError
}

func defaultAcceleratorProfiles() []map[string]any {
	// ponytail: two generic NVIDIA profiles cover current GPU labeling; admins add hardware-specific variants.
	return []map[string]any{
		{
			"id":                        "nvidia-gpu-default",
			"name":                      "NVIDIA GPU default",
			"enabled":                   true,
			"allowed_device_class_name": defaultDeviceClassName,
			"node_selector":             map[string]any{"nexuspaas.io/gpu": "true"},
			"labels":                    map[string]any{"nexuspaas.io/accelerator-profile": "nvidia-gpu-default"},
		},
		{
			"id":                        "nvidia-h100-sxm-rdma",
			"name":                      "NVIDIA H100 SXM RDMA",
			"enabled":                   true,
			"allowed_device_class_name": "h100-sxm.gpu.nvidia.com",
			"node_selector": map[string]any{
				"nexuspaas.io/gpu-class": "h100-sxm",
				"nexuspaas.io/rdma":      "true",
			},
			"labels": map[string]any{
				"nexuspaas.io/accelerator-class":   "h100-sxm",
				"nexuspaas.io/accelerator-profile": "nvidia-h100-sxm-rdma",
			},
		},
	}
}
