package imageregistry

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/contracts"
	"github.com/linskybing/nexuspaas/backend/internal/platform"
	"github.com/linskybing/nexuspaas/backend/internal/services/shared"
)

const (
	harborCatalogSyncOperation = "harborCatalogSync"
	harborCatalogSyncTaskName  = "harbor-catalog-sync"
	harborArtifactPageSize     = 100
	harborArtifactMaxPages     = 5
)

type harborSyncTarget struct {
	tagID      string
	project    string
	repository string
	tag        string
	digest     string
	payload    map[string]any
}

func registerHarborCatalogSync(app *platform.App) {
	app.RegisterMaintenanceTaskForService(serviceName, harborCatalogSyncTaskName, func(ctx context.Context) error {
		return retryHarborCatalogSync(ctx, app, time.Now().UTC())
	})
}

func retryHarborCatalogSync(ctx context.Context, app *platform.App, now time.Time) error {
	if app == nil || app.Store == nil {
		return nil
	}
	for _, record := range app.Store.List(ctx, imageSyncResource) {
		status := shared.TextValue(record.Data, "status")
		if status != "sync_requested" && status != "degraded" {
			continue
		}
		if status == "degraded" && !shared.BoolValue(record.Data, "retryable") {
			continue
		}
		tagID := shared.FirstNonBlank(shared.TextValue(record.Data, "tag_id", "tagId"), record.ID)
		syncHarborCatalogTarget(ctx, app, tagID, record.Data, nil, now)
	}
	return nil
}

func syncHarborCatalogTarget(ctx context.Context, app *platform.App, tagID string, statusData, payload map[string]any, now time.Time) map[string]any {
	target, ok := resolveHarborSyncTarget(ctx, app, tagID, statusData, payload)
	if !ok {
		return upsertHarborSyncStatus(ctx, app, tagID, harborSyncDegraded(tagID, "missing_selector", "project and repository selectors are required", false, now))
	}
	adapter, ok := app.Adapters["harbor"].(contracts.ProxyAdapter)
	if !ok || adapter == nil {
		app.Metrics.Inc("harbor_degraded")
		return upsertHarborSyncStatus(ctx, app, tagID, harborSyncDegraded(tagID, "adapter_not_configured", "harbor proxy adapter is not configured", true, now))
	}

	artifact, found, degraded := findHarborArtifactThroughPages(ctx, adapter, target, now)
	if degraded != nil {
		app.Metrics.Inc("harbor_degraded")
		return upsertHarborSyncStatus(ctx, app, tagID, degraded)
	}
	if !found {
		if marked, err := markHarborCatalogMissing(ctx, app, target, now); err != nil {
			return upsertHarborSyncStatus(ctx, app, tagID, harborSyncDegraded(tagID, "catalog_persist_failed", err.Error(), true, now))
		} else if !marked {
			return upsertHarborSyncStatus(ctx, app, tagID, harborSyncDegraded(tagID, "artifact_not_found", "harbor artifact was not found", true, now))
		}
		return upsertHarborSyncStatus(ctx, app, tagID, harborSyncDegraded(tagID, "artifact_not_found", "harbor artifact was not found", true, now))
	}
	catalog := harborCatalogRecord(ctx, app, target, artifact, now)
	if err := upsertHarborCatalogRecord(ctx, app, target.tagID, catalog); err != nil {
		return upsertHarborSyncStatus(ctx, app, tagID, harborSyncDegraded(tagID, "catalog_persist_failed", err.Error(), true, now))
	}
	return upsertHarborSyncStatus(ctx, app, tagID, harborSyncSynced(tagID, target.tagID, now))
}

func resolveHarborSyncTarget(ctx context.Context, app *platform.App, tagID string, statusData, payload map[string]any) (harborSyncTarget, bool) {
	tagID = shared.FirstNonBlank(tagID, shared.TextValue(payload, "tag_id", "tagId", "id"), "catalog")
	merged := map[string]any{}
	if existing := catalogByIDFromStore(ctx, app, tagID); len(existing) > 0 {
		for key, value := range existing {
			merged[key] = value
		}
	}
	for _, source := range []map[string]any{statusData, payload} {
		for key, value := range source {
			merged[key] = value
		}
	}
	imageRef := shared.TextValue(merged, "image_reference", "imageReference", "image")
	repository := shared.FirstNonBlank(shared.TextValue(merged, "repository", "repository_name", "repositoryName", "image_name"), repositoryFromReference(imageRef))
	project := shared.TextValue(merged, "project", "project_name", "projectName", "harbor_project", "harborProject")
	if project == "" && strings.Contains(repository, "/") {
		project = strings.Split(repository, "/")[0]
	}
	tag := shared.TextValue(merged, "tag", "tag_name", "tagName")
	if tag == "" && imageRef != "" {
		tag = tagFromReference(imageRef)
	}
	target := harborSyncTarget{
		tagID:      tagID,
		project:    project,
		repository: repository,
		tag:        tag,
		digest:     shared.TextValue(merged, "digest", "image_digest", "imageDigest"),
		payload:    merged,
	}
	return target, target.project != "" && target.repository != "" && (target.tag != "" || target.digest != "")
}

func findHarborArtifactThroughPages(ctx context.Context, adapter contracts.ProxyAdapter, target harborSyncTarget, now time.Time) (map[string]any, bool, map[string]any) {
	for page := 1; page <= harborArtifactMaxPages; page++ {
		query := harborArtifactQuery(page)
		resp, result, err := adapter.Proxy(ctx, contracts.AdapterProxyRequest{
			Operation:  harborCatalogSyncOperation,
			Method:     http.MethodGet,
			Path:       harborArtifactListPath(target),
			RawQuery:   query.Encode(),
			Idempotent: true,
		})
		if err != nil {
			return nil, false, harborSyncDegraded(target.tagID, "adapter_unavailable", err.Error(), true, now)
		}
		if result.Degraded {
			return nil, false, harborSyncDegraded(target.tagID, shared.FirstNonBlank(result.Code, "adapter_unavailable"), result.Message, true, now)
		}
		if resp.StatusCode >= 400 {
			return nil, false, harborSyncDegraded(target.tagID, "harbor_http_error", fmt.Sprintf("harbor artifact list returned HTTP %d", resp.StatusCode), true, now)
		}
		artifacts := harborArtifactsFromBody(resp.Body)
		for _, artifact := range artifacts {
			if harborArtifactMatches(artifact, target) {
				return artifact, true, nil
			}
		}
		if len(artifacts) < harborArtifactPageSize {
			break
		}
	}
	return nil, false, nil
}

func catalogByIDFromStore(ctx context.Context, app *platform.App, id string) map[string]any {
	if app == nil || app.Store == nil || id == "" {
		return nil
	}
	if record, found := app.Store.Get(ctx, imageCatalogResource, id); found {
		return shared.CloneMap(record.Data)
	}
	return nil
}

func harborArtifactListPath(target harborSyncTarget) string {
	return "/projects/" + url.PathEscape(target.project) + "/artifacts"
}

func harborArtifactQuery(page int) url.Values {
	query := url.Values{}
	query.Set("with_tag", "true")
	query.Set("with_scan_overview", "true")
	query.Set("page_size", fmt.Sprintf("%d", harborArtifactPageSize))
	query.Set("page", fmt.Sprintf("%d", page))
	return query
}

func findHarborArtifact(body []byte, target harborSyncTarget) (map[string]any, bool) {
	for _, artifact := range harborArtifactsFromBody(body) {
		if harborArtifactMatches(artifact, target) {
			return artifact, true
		}
	}
	return nil, false
}

func harborArtifactsFromBody(body []byte) []map[string]any {
	var decoded any
	if err := json.Unmarshal(body, &decoded); err != nil {
		return nil
	}
	return harborArtifactList(decoded)
}

func harborArtifactList(decoded any) []map[string]any {
	switch value := decoded.(type) {
	case []any:
		return harborMaps(value)
	case map[string]any:
		for _, key := range []string{"artifacts", "items"} {
			if items, ok := value[key].([]any); ok {
				return harborMaps(items)
			}
		}
	}
	return nil
}

func harborMaps(items []any) []map[string]any {
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		if row, ok := item.(map[string]any); ok {
			out = append(out, row)
		}
	}
	return out
}

func harborArtifactMatches(artifact map[string]any, target harborSyncTarget) bool {
	if artifactRepo := shared.TextValue(artifact, "repository", "repository_name", "repositoryName"); artifactRepo != "" {
		artifactRepo = strings.TrimPrefix(artifactRepo, target.project+"/")
		targetRepo := strings.TrimPrefix(target.repository, target.project+"/")
		if artifactRepo != target.repository && artifactRepo != targetRepo {
			return false
		}
	}
	if target.digest != "" && shared.TextValue(artifact, "digest") == target.digest {
		return true
	}
	if target.tag == "" {
		return false
	}
	for _, tag := range harborMapsFromValue(artifact["tags"]) {
		if shared.TextValue(tag, "name") == target.tag {
			return true
		}
	}
	return false
}

func harborMapsFromValue(value any) []map[string]any {
	items, _ := value.([]any)
	return harborMaps(items)
}

func harborCatalogRecord(ctx context.Context, app *platform.App, target harborSyncTarget, artifact map[string]any, now time.Time) map[string]any {
	out := shared.CloneMap(target.payload)
	if existing := catalogByIDFromStore(ctx, app, target.tagID); len(existing) > 0 {
		out = existing
	}
	out["id"] = target.tagID
	out["tag_id"] = target.tagID
	out["repository"] = target.repository
	if target.tag != "" {
		out["tag"] = target.tag
	}
	if digest := shared.TextValue(artifact, "digest"); digest != "" {
		out["digest"] = digest
	}
	if scanStatus := harborScanStatus(artifact); scanStatus != "" {
		out["scan_status"] = scanStatus
	}
	out["deleted"] = harborBoolValue(artifact, false, "deleted", "is_deleted", "isDeleted")
	out["unavailable"] = harborBoolValue(artifact, false, "unavailable")
	out["status"] = shared.FirstNonBlank(shared.TextValue(artifact, "status"), "available")
	out["updated_at"] = now
	for _, key := range []string{"push_time", "pull_time"} {
		if value := shared.TextValue(artifact, key); value != "" {
			out[key] = value
		}
	}
	return out
}

func markHarborCatalogMissing(ctx context.Context, app *platform.App, target harborSyncTarget, now time.Time) (bool, error) {
	catalog := catalogByIDFromStore(ctx, app, target.tagID)
	if len(catalog) == 0 {
		return false, nil
	}
	catalog["id"] = target.tagID
	catalog["tag_id"] = target.tagID
	catalog["repository"] = target.repository
	if target.tag != "" {
		catalog["tag"] = target.tag
	}
	catalog["deleted"] = true
	catalog["unavailable"] = true
	catalog["status"] = "missing"
	catalog["updated_at"] = now
	if app == nil || app.Store == nil {
		return true, nil
	}
	if _, ok := app.Store.Update(ctx, imageCatalogResource, target.tagID, catalog); !ok {
		return true, fmt.Errorf("harbor catalog update failed")
	}
	return true, nil
}

func harborScanStatus(artifact map[string]any) string {
	if status := shared.TextValue(artifact, "scan_status", "scanStatus"); status != "" {
		return status
	}
	overview, _ := artifact["scan_overview"].(map[string]any)
	for _, value := range overview {
		if item, ok := value.(map[string]any); ok {
			if status := shared.TextValue(item, "scan_status", "scanStatus"); status != "" {
				return status
			}
		}
	}
	return ""
}

func harborBoolValue(data map[string]any, fallback bool, keys ...string) bool {
	for _, key := range keys {
		switch value := data[key].(type) {
		case bool:
			return value
		case string:
			text := strings.TrimSpace(value)
			if text != "" {
				return strings.EqualFold(text, "true")
			}
		}
	}
	return fallback
}

func upsertHarborCatalogRecord(ctx context.Context, app *platform.App, id string, data map[string]any) error {
	if app == nil || app.Store == nil {
		return nil
	}
	if _, ok := app.Store.Update(ctx, imageCatalogResource, id, data); ok {
		return nil
	}
	_, err := app.Store.Create(ctx, imageCatalogResource, data)
	return err
}

func harborSyncSynced(tagID, catalogID string, now time.Time) map[string]any {
	return map[string]any{
		"id":         tagID,
		"tag_id":     tagID,
		"status":     "synced",
		"catalog_id": catalogID,
		"synced_at":  now,
		"updated_at": now,
		"degraded":   false,
		"code":       "ok",
		"message":    "harbor artifact synced",
		"retryable":  false,
	}
}

func harborSyncDegraded(tagID, code, message string, retryable bool, now time.Time) map[string]any {
	return map[string]any{
		"id":         tagID,
		"tag_id":     tagID,
		"status":     "degraded",
		"updated_at": now,
		"degraded":   true,
		"code":       code,
		"message":    message,
		"retryable":  retryable,
	}
}

func upsertHarborSyncStatus(ctx context.Context, app *platform.App, tagID string, data map[string]any) map[string]any {
	if app == nil || app.Store == nil {
		return data
	}
	data["id"] = tagID
	data["tag_id"] = tagID
	if record, ok := app.Store.Update(ctx, imageSyncResource, tagID, data); ok {
		return record.Data
	}
	record, err := app.Store.Create(ctx, imageSyncResource, data)
	if err != nil {
		return data
	}
	return record.Data
}
