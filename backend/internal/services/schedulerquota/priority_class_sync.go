package schedulerquota

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/contracts"
	"github.com/linskybing/nexuspaas/backend/internal/platform"
	"github.com/linskybing/nexuspaas/backend/internal/platform/cluster"
	"github.com/linskybing/nexuspaas/backend/internal/services/shared"
	corev1 "k8s.io/api/core/v1"
)

const (
	priorityClassSyncTaskName      = "priority-class-sync"
	priorityClassSyncCompletedName = "PriorityClassSyncCompleted"
)

func registerPriorityClassSync(app *platform.App) {
	app.RegisterMaintenanceTaskForService(serviceName, priorityClassSyncTaskName, func(ctx context.Context) error {
		return runPriorityClassSync(ctx, app, time.Now().UTC())
	})
}

func runPriorityClassSync(ctx context.Context, app *platform.App, checkedAt time.Time) error {
	repo := schedulerPreemptionPriorityRepositoryForApp(app)
	if app == nil || repo == nil {
		return nil
	}
	records := repo.ListPriorityClassRecords(ctx)
	summary := cluster.PriorityClassSyncSummary{SourceCount: len(records)}
	defs := make([]cluster.PriorityClassDefinition, 0, len(records))
	for _, record := range records {
		def, result := priorityClassDefinitionFromRecord(record)
		if result.Action != "" {
			addPriorityClassSyncResult(&summary, result)
			continue
		}
		defs = append(defs, def)
	}
	clusterSummary := app.Cluster.SyncPriorityClasses(ctx, defs)
	for _, result := range clusterSummary.Results {
		addPriorityClassSyncResult(&summary, result)
	}
	data := priorityClassSyncSummaryData(
		summary,
		checkedAt,
		app.Config.PriorityClassSyncInterval,
		repo.PriorityClassSourceResource(),
		repo.PriorityClassSyncSummaryResource(),
	)
	if err := persistPriorityClassSyncSummary(ctx, repo, data); err != nil {
		return err
	}
	if err := publishPriorityClassSyncCompleted(ctx, app.Events, data, checkedAt); err != nil {
		return err
	}
	slog.Info("priority class sync completed",
		"source_count", summary.SourceCount,
		"created", summary.Created,
		"updated", summary.Updated,
		"recreated", summary.Recreated,
		"adopted", summary.Adopted,
		"unchanged", summary.Unchanged,
		"invalid", summary.Invalid,
		"conflicts", summary.Conflict,
		"failed", summary.Failed,
		"degraded", summary.Degraded,
	)
	return nil
}

func priorityClassDefinitionFromRecord(record contracts.Record[map[string]any]) (cluster.PriorityClassDefinition, cluster.PriorityClassSyncResult) {
	name := shared.TextValue(record.Data, "name", "Name")
	value, ok := priorityClassValue(record.Data, "value", "Value", "priority", "Priority")
	if !ok {
		return cluster.PriorityClassDefinition{}, cluster.PriorityClassSyncResult{
			Name:   name,
			Action: cluster.PriorityClassActionInvalid,
			Reason: "priority class value required",
		}
	}
	policy, ok := priorityClassPreemptionPolicy(shared.TextValue(record.Data, "preemption_policy", "preemptionPolicy", "PreemptionPolicy"))
	if !ok {
		return cluster.PriorityClassDefinition{}, cluster.PriorityClassSyncResult{
			Name:   name,
			Action: cluster.PriorityClassActionInvalid,
			Reason: "invalid preemption policy",
		}
	}
	return cluster.PriorityClassDefinition{
		Name:             name,
		Value:            value,
		PreemptionPolicy: policy,
		Description:      shared.TextValue(record.Data, "description", "Description"),
		Labels:           priorityClassStringMap(shared.MapValue(record.Data, "labels", "Labels")),
		Annotations:      priorityClassStringMap(shared.MapValue(record.Data, "annotations", "Annotations")),
	}, cluster.PriorityClassSyncResult{}
}

func priorityClassStringMap(in map[string]any) map[string]string {
	out := map[string]string{}
	for key, value := range in {
		key = strings.TrimSpace(key)
		text := strings.TrimSpace(fmt.Sprint(value))
		if key != "" && text != "" {
			out[key] = text
		}
	}
	return out
}

func priorityClassValue(data map[string]any, keys ...string) (int32, bool) {
	for _, key := range keys {
		value, ok := data[key]
		if !ok {
			continue
		}
		n, ok := priorityClassInt64(value)
		if !ok || n < math.MinInt32 || n > math.MaxInt32 {
			return 0, false
		}
		return int32(n), true
	}
	return 0, false
}

func priorityClassInt64(value any) (int64, bool) {
	switch typed := value.(type) {
	case int:
		return int64(typed), true
	case int32:
		return int64(typed), true
	case int64:
		return typed, true
	case float64:
		if math.Trunc(typed) != typed {
			return 0, false
		}
		return int64(typed), true
	case json.Number:
		n, err := typed.Int64()
		return n, err == nil
	case string:
		n, err := strconv.ParseInt(strings.TrimSpace(typed), 10, 32)
		return n, err == nil
	default:
		return 0, false
	}
}

func priorityClassPreemptionPolicy(value string) (corev1.PreemptionPolicy, bool) {
	switch strings.TrimSpace(value) {
	case "", string(corev1.PreemptLowerPriority):
		return corev1.PreemptLowerPriority, true
	case string(corev1.PreemptNever), "PreemptNever":
		return corev1.PreemptNever, true
	default:
		return "", false
	}
}

func addPriorityClassSyncResult(summary *cluster.PriorityClassSyncSummary, result cluster.PriorityClassSyncResult) {
	summary.Results = append(summary.Results, result)
	switch result.Action {
	case cluster.PriorityClassActionCreated:
		summary.Created++
	case cluster.PriorityClassActionUpdated:
		summary.Updated++
	case cluster.PriorityClassActionRecreated:
		summary.Recreated++
	case cluster.PriorityClassActionAdopted:
		summary.Adopted++
	case cluster.PriorityClassActionUnchanged:
		summary.Unchanged++
	case cluster.PriorityClassActionInvalid:
		summary.Invalid++
	case cluster.PriorityClassActionConflict:
		summary.Conflict++
	case cluster.PriorityClassActionFailed:
		summary.Failed++
	case cluster.PriorityClassActionDegraded:
		summary.Degraded = true
	}
}

func priorityClassSyncSummaryData(
	summary cluster.PriorityClassSyncSummary,
	checkedAt time.Time,
	interval time.Duration,
	sourceResource string,
	summaryResource string,
) map[string]any {
	status := priorityClassSyncStatus(summary)
	results := make([]map[string]any, 0, len(summary.Results))
	for _, result := range summary.Results {
		entry := map[string]any{"name": result.Name, "action": result.Action}
		if result.Reason != "" {
			entry["reason"] = result.Reason
		}
		if result.Error != "" {
			entry["error"] = result.Error
		}
		results = append(results, entry)
	}
	return map[string]any{
		"id":               priorityClassSyncLatestRunID,
		"checked_at":       checkedAt.Format(time.RFC3339),
		"interval_seconds": int(interval.Seconds()),
		"status":           status,
		"source_count":     summary.SourceCount,
		"created_count":    summary.Created,
		"updated_count":    summary.Updated,
		"recreated_count":  summary.Recreated,
		"adopted_count":    summary.Adopted,
		"unchanged_count":  summary.Unchanged,
		"invalid_count":    summary.Invalid,
		"conflict_count":   summary.Conflict,
		"failed_count":     summary.Failed,
		"degraded":         summary.Degraded,
		"results":          results,
		"managed_by":       cluster.PriorityClassManagedByValue,
		"managed_owner":    cluster.PriorityClassOwnerValue,
		"schema_version":   1,
		"source_resource":  sourceResource,
		"summary_resource": summaryResource,
		"maintenance_task": priorityClassSyncTaskName,
		"event_name":       priorityClassSyncCompletedName,
		"cluster_resource": "priorityclasses.scheduling.k8s.io",
	}
}

func priorityClassSyncStatus(summary cluster.PriorityClassSyncSummary) string {
	switch {
	case summary.SourceCount == 0:
		return "no_records"
	case summary.Degraded:
		return "degraded"
	case summary.Failed > 0:
		return "failed"
	case summary.Conflict > 0:
		return "conflict"
	case summary.Invalid > 0 && summary.Created+summary.Updated+summary.Recreated+summary.Adopted+summary.Unchanged == 0:
		return "invalid"
	case summary.Created+summary.Updated+summary.Recreated+summary.Adopted > 0:
		return "synced"
	default:
		return "unchanged"
	}
}

func persistPriorityClassSyncSummary(
	ctx context.Context,
	repo schedulerPreemptionPriorityRepository,
	data map[string]any,
) error {
	if repo == nil {
		return nil
	}
	return repo.UpsertPriorityClassSyncSummary(ctx, data)
}

func publishPriorityClassSyncCompleted(ctx context.Context, events platform.EventStream, data map[string]any, checkedAt time.Time) error {
	if events == nil {
		return nil
	}
	event := contracts.Event{
		EventID:        platform.NewUUID(),
		Name:           priorityClassSyncCompletedName,
		Source:         serviceName,
		OccurredAt:     checkedAt,
		TraceID:        "priority-class-sync-" + checkedAt.Format("20060102150405"),
		SchemaVersion:  1,
		IdempotencyKey: priorityClassSyncTaskName + "-" + checkedAt.Format(time.RFC3339Nano),
		Data:           shared.CloneMap(data),
	}
	if err := events.Publish(ctx, event); err != nil {
		return fmt.Errorf("priority class sync event publish failed: %w", err)
	}
	return nil
}
