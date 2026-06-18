package authorizationpolicy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/contracts"
	"github.com/linskybing/nexuspaas/backend/internal/platform"
	"github.com/linskybing/nexuspaas/backend/internal/services/shared"
)

const (
	policyDataTaskName           = "policy-data-sync"
	policyDataProjectionConsumer = serviceName + ":policy_data_projection"
)

type policyDataBuildInput struct {
	project           map[string]any
	plan              map[string]any
	imageRules        []map[string]any
	now               time.Time
	imageCheckEnabled bool
	gpuUsage          float64
}

func registerPolicyDataSync(app *platform.App) {
	if app == nil {
		return
	}
	app.RegisterMaintenanceTaskForService(serviceName, policyDataTaskName, func(ctx context.Context) error {
		return syncPolicyData(ctx, app, time.Now().UTC())
	})
}

func syncPolicyData(ctx context.Context, app *platform.App, now time.Time) error {
	if app == nil || app.Store == nil {
		return nil
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	syncPolicyDataReadModels(ctx, app)
	repo := authorizationPolicyProjectionRepo(app)
	projects := repo.ListPolicyProjects(ctx)
	slog.Info("policy data sync started", "projects", len(projects))

	var errs []error
	var namespaceCount, updatedCount int
	for _, project := range projects {
		projectID := policyProjectID(project)
		if projectID == "" {
			continue
		}
		namespaces, err := policyProjectNamespaces(ctx, app, projectID)
		if err != nil {
			errs = append(errs, fmt.Errorf("list project namespaces %s: %w", projectID, err))
			continue
		}
		namespaceCount += len(namespaces)
		if len(namespaces) == 0 {
			continue
		}
		data := buildPolicyConfigMapData(policyDataBuildInput{
			project:           project,
			plan:              repo.FindPolicyPlanForProject(ctx, project),
			imageRules:        repo.ListPolicyImageRulesForProject(ctx, projectID),
			now:               now,
			imageCheckEnabled: app.Config.ImageCheckEnabled,
			gpuUsage:          policyGPUUsageForNamespaces(ctx, app, namespaces, now),
		})
		for _, namespace := range namespaces {
			if err := app.Cluster.EnsurePolicyDataConfigMap(ctx, namespace, data); err != nil {
				errs = append(errs, fmt.Errorf("ensure policy data configmap %s/%s: %w", namespace, projectID, err))
				continue
			}
			updatedCount++
		}
	}
	slog.Info("policy data sync completed", "projects", len(projects), "namespaces", namespaceCount, "configmaps", updatedCount, "errors", len(errs))
	return errors.Join(errs...)
}

func syncPolicyDataReadModels(ctx context.Context, app *platform.App) {
	if app == nil || app.Store == nil || app.Events == nil {
		return
	}
	app.RunProjection(ctx, policyDataProjectionConsumer, func(event contracts.Event) error {
		return projectPolicyDataEvent(ctx, app, event)
	})
}

func projectPolicyDataEvent(ctx context.Context, app *platform.App, event contracts.Event) error {
	resource, data, deleted, ok := policyDataProjection(event)
	if !ok {
		return nil
	}
	repo := authorizationPolicyProjectionRepo(app)
	if deleted {
		deletePolicyDataReadModel(ctx, repo, resource, data)
		return nil
	}
	return upsertPolicyDataReadModel(ctx, repo, resource, data)
}

func policyDataProjection(event contracts.Event) (string, map[string]any, bool, bool) {
	name := strings.ToLower(strings.TrimSpace(event.Name))
	switch name {
	case "projectcreated", "projectupdated":
		return policyDataProjectsResource, policyEventData(event), false, true
	case "projectdeleted":
		return policyDataProjectsResource, policyEventData(event), true, true
	case "planchanged":
		data := policyEventData(event)
		action := strings.ToLower(shared.TextValue(data, "action"))
		if action == "deleted" {
			return policyDataPlansResource, data, true, true
		}
		if !policyPlanPayloadHasRuntimeFields(data) {
			return "", nil, false, false
		}
		return policyDataPlansResource, data, false, true
	case "imageapproved", "imagepublished":
		return policyDataImageAllowListsResource, policyEventData(event), false, true
	case "imageunpublished", "projectimageremoved":
		return policyDataImageAllowListsResource, policyEventData(event), true, true
	default:
		return "", nil, false, false
	}
}

func policyEventData(event contracts.Event) map[string]any {
	for _, key := range []string{"new", "record", "data"} {
		if next, ok := event.Data[key].(map[string]any); ok {
			return maps.Clone(next)
		}
	}
	return maps.Clone(event.Data)
}

func upsertPolicyDataReadModel(ctx context.Context, repo *recordStoreAuthorizationPolicyProjectionRepository, resource string, data map[string]any) error {
	switch resource {
	case policyDataProjectsResource:
		return repo.UpsertPolicyProject(ctx, data)
	case policyDataPlansResource:
		return repo.UpsertPolicyPlan(ctx, data)
	case policyDataImageAllowListsResource:
		return repo.UpsertPolicyImageAllowList(ctx, data)
	default:
		return nil
	}
}

func deletePolicyDataReadModel(ctx context.Context, repo *recordStoreAuthorizationPolicyProjectionRepository, resource string, data map[string]any) bool {
	switch resource {
	case policyDataProjectsResource:
		return repo.DeletePolicyProject(ctx, data)
	case policyDataPlansResource:
		return repo.DeletePolicyPlan(ctx, data)
	case policyDataImageAllowListsResource:
		return repo.DeletePolicyImageAllowList(ctx, data)
	default:
		return false
	}
}

func buildPolicyConfigMapData(input policyDataBuildInput) map[string]string {
	if input.now.IsZero() {
		input.now = time.Now().UTC()
	}
	if policyProjectID(input.project) == "" {
		return restrictivePolicyConfigMapData(input.imageCheckEnabled)
	}
	maxRuntime := policyNonNegativeInt(input.project, "max_job_runtime_seconds", "maxJobRuntimeSeconds", "MaxJobRuntimeSeconds")
	gpuLimit := 0.0
	timeAllowed := false
	if policyPlanID(input.plan) != "" {
		gpuLimit = policyNumberValue(input.plan, "gpu_limit", "gpuLimit", "GPULimit")
		timeAllowed = policyPlanActive(input.plan, input.now)
	}
	images := policyImageLists(input.imageRules)
	return map[string]string{
		"maxJobRuntimeSeconds":  fmt.Sprintf("%d", maxRuntime),
		"gpuLimit":              fmt.Sprintf("%g", gpuLimit),
		"imageCheckEnabled":     fmt.Sprintf("%t", input.imageCheckEnabled),
		"timeAllowed":           fmt.Sprintf("%t", timeAllowed),
		"gpuNamespaceUsage":     fmt.Sprintf("%g", input.gpuUsage),
		"allowedProxyImages":    images["allowedProxyImages"],
		"allowedMirroredImages": images["allowedMirroredImages"],
		"syncedMirroredImages":  images["syncedMirroredImages"],
		"publishedBuiltImages":  images["publishedBuiltImages"],
	}
}

func restrictivePolicyConfigMapData(imageCheckEnabled bool) map[string]string {
	return map[string]string{
		"maxJobRuntimeSeconds":  "0",
		"gpuLimit":              "0",
		"imageCheckEnabled":     fmt.Sprintf("%t", imageCheckEnabled),
		"timeAllowed":           "false",
		"gpuNamespaceUsage":     "0",
		"allowedProxyImages":    ",",
		"allowedMirroredImages": ",",
		"syncedMirroredImages":  ",",
		"publishedBuiltImages":  ",",
	}
}

func policyPlanForProject(ctx context.Context, app *platform.App, project map[string]any) map[string]any {
	return authorizationPolicyProjectionRepo(app).FindPolicyPlanForProject(ctx, project)
}

func policyImageRulesForProject(ctx context.Context, app *platform.App, projectID string) []map[string]any {
	return authorizationPolicyProjectionRepo(app).ListPolicyImageRulesForProject(ctx, projectID)
}

func policyProjectNamespaces(ctx context.Context, app *platform.App, projectID string) ([]string, error) {
	if app == nil || app.Cluster == nil {
		return nil, nil
	}
	return app.Cluster.ListProjectNamespaces(ctx, projectID)
}

func policyGPUUsageForNamespaces(ctx context.Context, app *platform.App, namespaces []string, now time.Time) float64 {
	if app == nil || app.Cluster == nil || len(namespaces) == 0 {
		return 0
	}
	namespaceSet := map[string]bool{}
	for _, namespace := range namespaces {
		namespaceSet[namespace] = true
	}
	usages, err := app.Cluster.ListJobPodResourceUsage(ctx, now)
	if err != nil {
		slog.Warn("policy data sync: list pod usage failed", "error", err)
		return 0
	}
	var total float64
	for _, usage := range usages {
		if usage.IsActive && namespaceSet[usage.Namespace] {
			total += usage.RequestedGPU
		}
	}
	return total
}

func policyImageLists(rules []map[string]any) map[string]string {
	values := map[string][]string{
		"allowedProxyImages":    {},
		"allowedMirroredImages": {},
		"syncedMirroredImages":  {},
		"publishedBuiltImages":  {},
	}
	for _, rule := range rules {
		if !shared.BoolValue(rule, "enabled", "Enabled") {
			continue
		}
		ref := policyImageReference(rule)
		if ref == "" {
			continue
		}
		key := policyImageListKey(rule)
		if key == "" {
			continue
		}
		values[key] = append(values[key], ref)
	}
	out := map[string]string{}
	for key, entries := range values {
		out[key] = policyCommaList(entries)
	}
	return out
}

func policyImageReference(rule map[string]any) string {
	if ref := shared.TextValue(rule, "image_reference", "imageReference", "image", "full_name", "fullName"); ref != "" {
		return ref
	}
	repository := shared.TextValue(rule, "repository", "repo")
	tag := shared.TextValue(rule, "tag", "tag_name", "tagName")
	if repository == "" {
		return ""
	}
	if tag == "" {
		tag = "*"
	}
	return repository + ":" + tag
}

func policyImageListKey(rule map[string]any) string {
	mode := strings.ToLower(strings.TrimSpace(shared.TextValue(rule, "delivery_mode", "deliveryMode", "mode")))
	mode = strings.NewReplacer("-", "_", " ", "_").Replace(mode)
	if mode == "" || mode == "proxy" {
		return "allowedProxyImages"
	}
	switch mode {
	case "mirrored", "mirror":
		return "allowedMirroredImages"
	case "synced_mirrored", "synced", "mirrored_synced":
		return "syncedMirroredImages"
	case "built", "published", "published_built":
		return "publishedBuiltImages"
	default:
		return ""
	}
}

func policyCommaList(entries []string) string {
	if len(entries) == 0 {
		return ","
	}
	sort.Strings(entries)
	deduped := entries[:0]
	var last string
	for _, entry := range entries {
		if entry == "" || entry == last {
			continue
		}
		deduped = append(deduped, entry)
		last = entry
	}
	if len(deduped) == 0 {
		return ","
	}
	return "," + strings.Join(deduped, ",") + ","
}

func policyPlanActive(data map[string]any, now time.Time) bool {
	if policyPlanID(data) == "" {
		return false
	}
	if validFrom := policyTimeValue(data, "valid_from", "validFrom", "ValidFrom"); validFrom != nil && now.Before(*validFrom) {
		return false
	}
	if validUntil := policyTimeValue(data, "valid_until", "validUntil", "ValidUntil"); validUntil != nil && now.After(*validUntil) {
		return false
	}
	return policyWeekWindowsContain(policyWeekWindows(data), now)
}

func policyWeekWindows(data map[string]any) []map[string]any {
	raw, ok := firstPolicyValue(data, "week_windows", "weekWindows", "WeekWindows")
	if !ok || raw == nil {
		return nil
	}
	if windows, ok := raw.([]map[string]any); ok {
		return windows
	}
	if text, ok := raw.(string); ok && strings.TrimSpace(text) != "" {
		var decoded []map[string]any
		if json.Unmarshal([]byte(text), &decoded) == nil {
			return decoded
		}
	}
	items, ok := raw.([]any)
	if !ok {
		return nil
	}
	windows := make([]map[string]any, 0, len(items))
	for _, item := range items {
		if window, ok := item.(map[string]any); ok {
			windows = append(windows, window)
		}
	}
	return windows
}

func policyWeekWindowsContain(windows []map[string]any, now time.Time) bool {
	if len(windows) == 0 {
		return true
	}
	second := policyWeekSecond(now)
	for _, window := range windows {
		start := int(policyInt64Value(firstPolicyValueOrNil(window, "start", "Start"), -1))
		end := int(policyInt64Value(firstPolicyValueOrNil(window, "end", "End"), -1))
		if start >= 0 && end > start && end <= 604800 && second >= start && second < end {
			return true
		}
	}
	return false
}

func policyWeekSecond(t time.Time) int {
	utc := t.UTC()
	weekday := (int(utc.Weekday()) + 6) % 7
	return weekday*86400 + utc.Hour()*3600 + utc.Minute()*60 + utc.Second()
}

func policyTimeValue(data map[string]any, keys ...string) *time.Time {
	for _, key := range keys {
		switch value := data[key].(type) {
		case time.Time:
			t := value.UTC()
			return &t
		case string:
			if strings.TrimSpace(value) == "" {
				continue
			}
			if t, err := time.Parse(time.RFC3339, value); err == nil {
				utc := t.UTC()
				return &utc
			}
		}
	}
	return nil
}

func policyPlanPayloadHasRuntimeFields(data map[string]any) bool {
	if policyPlanID(data) == "" {
		return false
	}
	for _, key := range []string{"gpu_limit", "gpuLimit", "valid_from", "validFrom", "valid_until", "validUntil", "week_windows", "weekWindows", "name"} {
		if _, ok := data[key]; ok {
			return true
		}
	}
	return false
}

func policyProjectID(data map[string]any) string {
	return shared.FirstNonEmpty(
		shared.TextValue(data, "id", "ID"),
		shared.TextValue(data, "project_id", "projectId", "p_id", "pId", "P_ID"),
	)
}

func policyPlanID(data map[string]any) string {
	return shared.FirstNonEmpty(shared.TextValue(data, "id", "ID"), shared.TextValue(data, "plan_id", "planId"))
}

func policyImageRuleID(data map[string]any) string {
	if id := shared.TextValue(data, "id", "ID"); id != "" {
		return id
	}
	projectID := shared.TextValue(data, "project_id", "projectId")
	tagID := shared.FirstNonEmpty(shared.TextValue(data, "tag_id", "tagId"), shared.TextValue(data, "image_reference", "imageReference"))
	if projectID != "" && tagID != "" {
		return projectID + ":" + tagID
	}
	return tagID
}

func policyNonNegativeInt(data map[string]any, keys ...string) int {
	value := shared.IntValue(data, keys...)
	if value < 0 {
		return 0
	}
	return value
}

func policyNumberValue(data map[string]any, keys ...string) float64 {
	for _, key := range keys {
		switch value := data[key].(type) {
		case float64:
			return value
		case float32:
			return float64(value)
		case int:
			return float64(value)
		case int64:
			return float64(value)
		case json.Number:
			if n, err := value.Float64(); err == nil {
				return n
			}
		case string:
			if n, err := strconv.ParseFloat(strings.TrimSpace(value), 64); err == nil {
				return n
			}
		}
	}
	return 0
}

func firstPolicyValue(data map[string]any, keys ...string) (any, bool) {
	for _, key := range keys {
		value, ok := data[key]
		if ok {
			return value, true
		}
	}
	return nil, false
}

func firstPolicyValueOrNil(data map[string]any, keys ...string) any {
	value, _ := firstPolicyValue(data, keys...)
	return value
}

func policyInt64Value(value any, fallback int64) int64 {
	switch typed := value.(type) {
	case int:
		return int64(typed)
	case int64:
		return typed
	case int32:
		return int64(typed)
	case float64:
		return int64(typed)
	case json.Number:
		if n, err := typed.Int64(); err == nil {
			return n
		}
	case string:
		if n, err := strconv.ParseInt(strings.TrimSpace(typed), 10, 64); err == nil {
			return n
		}
	}
	return fallback
}
