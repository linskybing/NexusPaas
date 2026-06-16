package clusterread

import (
	"context"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/platform"
	"github.com/linskybing/nexuspaas/backend/internal/platform/cluster"
)

// collectClusterResources snapshots cluster node capacity/usage into the
// cluster_read_models read model that the /cluster/* endpoints serve. It is the
// microservice port of reference cron.StartClusterResourceCollector +
// application/cluster.RefreshCache: the cron loop's RefreshCache wrote a cache the
// read API consumed; here the maintenance loop writes the same shape into the
// store-backed read model clusterSummary() already reads from.
//
// A nil cluster client (degraded mode, no Kubernetes config) is a no-op so the
// last good snapshot is retained rather than overwritten with zeros.
func collectClusterResources(ctx context.Context, cl *cluster.Client, store platform.RecordStore, now time.Time) error {
	if cl == nil || store == nil {
		return nil
	}
	summary, err := cl.CollectNodeSummary(ctx)
	if err != nil {
		return err
	}
	upsertReadModel(ctx, store, map[string]any{"summary": summaryMap(summary, now)})
	return nil
}

// summaryMap renders a cluster.NodeSummary into the camelCase map shape
// emptySummary()/publicSummary()/nodeList() expect. DRA device classes and
// Prometheus pod GPU usages are left empty until the DRA/GPU adapters land.
func summaryMap(s cluster.NodeSummary, now time.Time) map[string]any {
	nodes := make([]any, 0, len(s.Nodes))
	for _, n := range s.Nodes {
		nodes = append(nodes, map[string]any{
			"name":                   n.Name,
			"cpuAllocatableMilli":    n.CPUAllocatableMilli,
			"cpuUsedMilli":           n.CPUUsedMilli,
			"memoryAllocatableBytes": n.MemoryAllocatableBytes,
			"memoryUsedBytes":        n.MemoryUsedBytes,
			"gpuAllocatable":         n.GPUAllocatable,
			"gpuUsed":                n.GPUUsed,
		})
	}
	return map[string]any{
		"nodeCount":                   s.NodeCount,
		"totalCpuAllocatableMilli":    s.TotalCPUAllocatableMilli,
		"totalCpuUsedMilli":           s.TotalCPUUsedMilli,
		"totalMemoryAllocatableBytes": s.TotalMemoryAllocatableBytes,
		"totalMemoryUsedBytes":        s.TotalMemoryUsedBytes,
		"totalGpuAllocatable":         s.TotalGPUAllocatable,
		"totalGpuUsed":                s.TotalGPUUsed,
		"nodes":                       nodes,
		"podGpuUsages":                []any{},
		"deviceClasses":               []any{},
		"collectedAt":                 now,
	}
}

// upsertReadModel keeps a single cluster_read_models record: it updates the newest
// existing record in place (so the read path's "newest record" stays the live
// snapshot and the table does not grow unbounded) and creates one on first run.
func upsertReadModel(ctx context.Context, store platform.RecordStore, data map[string]any) {
	records := store.List(ctx, clusterReadModelResource)
	if len(records) > 0 {
		if _, ok := store.Update(ctx, clusterReadModelResource, records[len(records)-1].ID, data); ok {
			return
		}
	}
	_, _ = store.Create(ctx, clusterReadModelResource, data)
}

// registerClusterResourceCollector wires the collector as a lease-gated
// maintenance task. It runs only once StartMaintenance is called.
func registerClusterResourceCollector(app *platform.App) {
	app.RegisterMaintenanceTaskForService(serviceName, "cluster-resource-collector", func(ctx context.Context) error {
		return collectClusterResources(ctx, app.Cluster, app.Store, time.Now().UTC())
	})
}
