package schedulerquota

import (
	"context"
	"log/slog"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/platform"
	"github.com/linskybing/nexuspaas/backend/internal/platform/cluster"
	"github.com/linskybing/nexuspaas/backend/internal/services/shared"
)

const defaultQuotaPods = 20

// reconcileResourceQuotas syncs each project's bound resource plan into a
// Kubernetes ResourceQuota in every namespace of that project. It is the
// microservice port of reference cron.StartResourceQuotaReconciler's enforcement
// path: a project with a bound plan gets a "<ns>-quota" ResourceQuota built from
// the plan's CPU/memory/pod limits; GPU is enforced by admission, not by quota
// (see cluster.BuildQuotaResources).
//
// The reference's orphan-pod cleanup (cleanOrphans) requires the typed Job repo
// and lands with Phase 1b/3; it is intentionally not part of this enforcement
// pass. A nil cluster client (degraded mode) is a no-op.
func reconcileResourceQuotas(ctx context.Context, cl *cluster.Client, store platform.RecordStore, _ time.Time) error {
	return reconcileResourceQuotasWithReader(ctx, cl, store, newAdmissionReader(store))
}

func reconcileResourceQuotasWithReader(ctx context.Context, cl *cluster.Client, store platform.RecordStore, reader admissionReader) error {
	repo := schedulerQuotaRepositoryFromStore(store)
	if cl == nil || store == nil || repo == nil || reader == nil {
		return nil
	}
	for _, project := range reader.ListProjects(ctx) {
		planID := shared.TextValue(project.Data, "plan_id", "planId", "resource_plan_id", "resourcePlanId")
		if planID == "" {
			continue // no bound plan: unlimited at the K8s level, as in the reference.
		}
		plan, found := repo.GetPlan(ctx, planID)
		if !found {
			continue
		}
		gpu := shared.NumberValue(plan.Data, "gpu_limit", "gpuLimit")
		cpu := shared.NumberValue(plan.Data, "cpu_limit_cores", "cpuLimitCores")
		mem := shared.NumberValue(plan.Data, "memory_limit_gb", "memoryLimitGb")
		hard := cluster.BuildQuotaResources(gpu, cpu, mem, quotaPodLimit(project.Data))

		namespaces, err := cl.ListProjectNamespaces(ctx, project.ID)
		if err != nil {
			slog.Warn("resource quota reconciler: list namespaces failed", "project_id", project.ID, "error", err)
			continue
		}
		for _, ns := range namespaces {
			if err := cl.EnsureResourceQuota(ctx, ns, ns+"-quota", hard); err != nil {
				slog.Warn("resource quota reconciler: ensure quota failed", "project_id", project.ID, "namespace", ns, "error", err)
			}
		}
	}
	return nil
}

// quotaPodLimit mirrors the reference rule: max-concurrent-jobs-per-user × 2,
// defaulting to 20 when unset or non-positive.
func quotaPodLimit(project map[string]any) int {
	pods := int(shared.NumberValue(project, "max_concurrent_jobs_per_user", "maxConcurrentJobsPerUser")) * 2
	if pods <= 0 {
		return defaultQuotaPods
	}
	return pods
}

// registerResourceQuotaReconciler wires the reconciler as a lease-gated
// maintenance task. It runs only once StartMaintenance is called.
func registerResourceQuotaReconciler(app *platform.App) {
	app.RegisterMaintenanceTaskForService(serviceName, "resource-quota-reconciler", func(ctx context.Context) error {
		return reconcileResourceQuotasWithReader(ctx, app.Cluster, app.Store, newAdmissionReaderForApp(app))
	})
}
