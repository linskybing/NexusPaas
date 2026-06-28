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
	networkProfilesResource = serviceName + ":network_profiles"
	msgNetworkProfileSave   = "network profile could not be saved"
)

func seedDefaultNetworkProfiles(app *platform.App) error {
	if app == nil || app.Store == nil || !app.Config.AllowsService(serviceName) {
		return nil
	}
	ctx := context.Background()
	for _, profile := range defaultNetworkProfiles() {
		id := shared.TextValue(profile, "id")
		if id == "" {
			continue
		}
		if _, found := app.Store.Get(ctx, networkProfilesResource, id); found {
			continue
		}
		if _, err := app.Store.Create(ctx, networkProfilesResource, profile); err != nil && !platform.IsCreateConflict(err) {
			return err
		}
	}
	return nil
}

func createNetworkProfile(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	payload, err := networkProfilePayload(r)
	if err != nil {
		return http.StatusBadRequest, shared.ErrorData(err.Error()), nil
	}
	if missing := missingNetworkProfileRequired(payload); missing != "" {
		return http.StatusBadRequest, shared.ErrorData("missing required field: " + missing), nil
	}
	record, err := app.CreateRecordWithEvent(r.Context(), networkProfilesResource, payload, func(record contracts.Record[map[string]any]) contracts.Event {
		return schedulerEvent(r, "NetworkProfileChanged", "created", networkProfileEventPayload(record.Data, "created"))
	})
	if err != nil {
		return networkProfileCreateError(err), shared.ErrorData(msgNetworkProfileSave), nil
	}
	return http.StatusCreated, record, nil
}

func updateNetworkProfile(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	id := strings.TrimSpace(r.PathValue("id"))
	payload, err := networkProfilePayload(r)
	if err != nil {
		return http.StatusBadRequest, shared.ErrorData(err.Error()), nil
	}
	if missing := missingNetworkProfileRequired(payload); missing != "" {
		return http.StatusBadRequest, shared.ErrorData("missing required field: " + missing), nil
	}
	if id == "" {
		id = shared.TextValue(payload, "id")
	}
	payload["id"] = id
	record, ok, err := app.UpdateRecordWithEvent(r.Context(), networkProfilesResource, id, payload, func(record contracts.Record[map[string]any]) contracts.Event {
		return schedulerEvent(r, "NetworkProfileChanged", "updated", networkProfileEventPayload(record.Data, "updated"))
	})
	if err != nil {
		return http.StatusInternalServerError, shared.ErrorData(msgNetworkProfileSave), nil
	}
	if !ok {
		record, err = app.CreateRecordWithEvent(r.Context(), networkProfilesResource, payload, func(record contracts.Record[map[string]any]) contracts.Event {
			return schedulerEvent(r, "NetworkProfileChanged", "updated", networkProfileEventPayload(record.Data, "updated"))
		})
		if err != nil {
			return networkProfileCreateError(err), shared.ErrorData(msgNetworkProfileSave), nil
		}
	}
	return http.StatusOK, record, nil
}

func deleteNetworkProfile(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	id := strings.TrimSpace(r.PathValue("id"))
	deleted, err := app.DeleteRecordWithEvent(r.Context(), networkProfilesResource, id, func(deleted bool) contracts.Event {
		return schedulerEvent(r, "NetworkProfileChanged", "deleted", map[string]any{
			"id":                 id,
			"profile_id":         id,
			"network_profile_id": id,
			"deleted":            deleted,
		})
	})
	if err != nil {
		return http.StatusInternalServerError, shared.ErrorData("network profile could not be deleted"), nil
	}
	return http.StatusOK, map[string]any{"id": id, "deleted": deleted}, nil
}

func networkProfilePayload(r *http.Request) (map[string]any, error) {
	payload, err := platform.DecodeMapWithError(r)
	if err != nil {
		return nil, err
	}
	return shared.CloneMap(payload), nil
}

func missingNetworkProfileRequired(payload map[string]any) string {
	for _, field := range []string{"name", "primary_cni"} {
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

func networkProfileEventPayload(data map[string]any, action string) map[string]any {
	payload := shared.CloneMap(data)
	id := shared.TextValue(data, "id")
	payload["profile_id"] = id
	payload["network_profile_id"] = id
	payload["action"] = action
	return payload
}

func networkProfileCreateError(err error) int {
	if platform.IsCreateConflict(err) {
		return http.StatusConflict
	}
	return http.StatusInternalServerError
}

func defaultNetworkProfiles() []map[string]any {
	// ponytail: two real profiles cover current clusters; hardware-specific profiles can be added by admin CRUD.
	return []map[string]any{
		{
			"id":                "default-cilium",
			"name":              "Default Cilium",
			"primary_cni":       "cilium",
			"secondary_network": "none",
			"rdma_enabled":      false,
			"roce_enabled":      false,
			"bandwidth_class":   "standard",
			"topology_policy":   "none",
			"enabled":           true,
			"annotations":       map[string]any{},
			"network_env":       map[string]any{},
		},
		{
			"id":                   "rdma-fast-lane",
			"name":                 "RDMA fast lane",
			"primary_cni":          "cilium",
			"secondary_network":    "nexuspaas-system/rdma-net",
			"rdma_enabled":         true,
			"roce_enabled":         true,
			"bandwidth_class":      "rdma",
			"mtu":                  9000,
			"required_nic_class":   "rdma",
			"topology_policy":      "same-rack",
			"enabled":              true,
			"node_selector":        map[string]any{"nexuspaas.io/rdma": "true"},
			"annotations":          map[string]any{"k8s.v1.cni.cncf.io/networks": "nexuspaas-system/rdma-net"},
			"network_env":          map[string]any{"NCCL_SOCKET_IFNAME": "net1", "NCCL_IB_DISABLE": "0"},
			"required_device_hint": "rdma/rdma_shared_device_a",
		},
	}
}
