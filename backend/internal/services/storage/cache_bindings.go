package storage

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/contracts"
	"github.com/linskybing/nexuspaas/backend/internal/platform"
	"github.com/linskybing/nexuspaas/backend/internal/services/shared"
)

const cacheBindingChangedEvent = "CacheBindingChanged"

func listCacheBindings(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	userID, status, data, ok := requireUser(r)
	if !ok {
		return status, data, nil
	}
	projectID := projectPathID(r)
	if _, status, data, ok := requireProjectManager(app, r, projectID, userID); !ok {
		return status, data, nil
	}
	return http.StatusOK, storageRepo(app).ListCacheBindings(r.Context(), projectID), nil
}

func createCacheBinding(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	return upsertCacheBinding(app, r)
}

func updateCacheBinding(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	return upsertCacheBinding(app, r)
}

func getCacheBinding(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	userID, status, data, ok := requireUser(r)
	if !ok {
		return status, data, nil
	}
	projectID := projectPathID(r)
	if _, status, data, ok := requireProjectManager(app, r, projectID, userID); !ok {
		return status, data, nil
	}
	record, found := storageRepo(app).GetCacheBinding(r.Context(), projectID, cacheBindingPathID(r))
	if !found {
		return http.StatusNotFound, shared.ErrorData("cache binding not found"), nil
	}
	return http.StatusOK, record, nil
}

func deleteCacheBinding(app *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
	userID, status, data, ok := requireUser(r)
	if !ok {
		return status, data, nil
	}
	projectID := projectPathID(r)
	if _, status, data, ok := requireProjectManager(app, r, projectID, userID); !ok {
		return status, data, nil
	}
	id := cacheBindingPathID(r)
	deleted, err := storageRepo(app).DeleteCacheBindingWithEvent(r.Context(), app, projectID, id, func(deleted bool) contracts.Event {
		return storageEvent(r, cacheBindingChangedEvent, map[string]any{"id": id, "cache_binding_id": id, "project_id": projectID, "action": "deleted", "deleted": deleted})
	})
	if err != nil {
		return http.StatusInternalServerError, shared.ErrorData("cache binding could not be deleted"), nil
	}
	if !deleted {
		return http.StatusNotFound, shared.ErrorData("cache binding not found"), nil
	}
	return http.StatusOK, map[string]any{"id": id, "deleted": true}, nil
}

func upsertCacheBinding(app *platform.App, r *http.Request) (int, any, *platform.Degraded) {
	userID, status, data, ok := requireUser(r)
	if !ok {
		return status, data, nil
	}
	projectID := projectPathID(r)
	if _, status, data, ok := requireProjectManager(app, r, projectID, userID); !ok {
		return status, data, nil
	}
	payload, err := platform.DecodeMapWithError(r)
	if err != nil {
		return http.StatusBadRequest, shared.ErrorData(msgInvalidRequestBody), nil
	}
	record, err := cacheBindingRecord(projectID, payload, cacheBindingPathID(r), time.Now().UTC())
	if err != nil {
		return http.StatusBadRequest, shared.ErrorData(err.Error()), nil
	}
	created, err := storageRepo(app).UpsertCacheBindingWithEvent(r.Context(), app, shared.TextValue(record, "id"), record, func(data map[string]any) contracts.Event {
		return storageEvent(r, cacheBindingChangedEvent, cacheBindingEventPayload(data, "upserted"))
	})
	if err != nil {
		return http.StatusConflict, shared.ErrorData("cache binding could not be saved"), nil
	}
	return http.StatusOK, created, nil
}

func cacheBindingRecord(projectID string, payload map[string]any, pathID string, now time.Time) (map[string]any, error) {
	storageBindingID := shared.TextValue(payload, "storage_binding_id", "storageBindingId")
	cacheKey := shared.TextValue(payload, "cache_key", "cacheKey")
	scratchProfile := shared.TextValue(payload, "scratch_profile", "scratchProfile")
	if storageBindingID == "" || cacheKey == "" || scratchProfile == "" {
		return nil, fmt.Errorf("storage_binding_id, cache_key, and scratch_profile are required")
	}
	nodeClass := shared.TextValue(payload, "node_class", "nodeClass")
	id := shared.FirstNonBlank(pathID, shared.TextValue(payload, "id"), cacheBindingID(projectID, storageBindingID, cacheKey, nodeClass))
	record := shared.CloneMap(payload)
	record["id"] = id
	record["project_id"] = projectID
	record["storage_binding_id"] = storageBindingID
	record["cache_key"] = cacheKey
	record["scratch_profile"] = scratchProfile
	record["node_class"] = nodeClass
	if _, ok := record["node_selector"]; !ok {
		if selector, ok := record["nodeSelector"]; ok {
			record["node_selector"] = selector
			delete(record, "nodeSelector")
		}
	}
	if _, ok := record["last_staged_at"]; !ok {
		record["last_staged_at"] = now
	}
	record["updated_at"] = now
	return record, nil
}

func cacheBindingEventPayload(data map[string]any, action string) map[string]any {
	payload := shared.CloneMap(data)
	payload["cache_binding_id"] = shared.TextValue(data, "id")
	payload["action"] = action
	return payload
}

func cacheBindingID(projectID, storageBindingID, cacheKey, nodeClass string) string {
	parts := []string{projectID, storageBindingID, cacheKey}
	if strings.TrimSpace(nodeClass) != "" {
		parts = append(parts, nodeClass)
	}
	return storagePlanName(strings.Join(parts, "-"), "cache-binding")
}

func cacheBindingPathID(r *http.Request) string {
	return strings.TrimSpace(shared.FirstNonBlank(r.PathValue("cacheBindingId"), r.PathValue("bindingId"), r.PathValue("id")))
}
