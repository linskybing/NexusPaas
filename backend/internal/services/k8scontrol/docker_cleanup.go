package k8scontrol

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/linskybing/nexuspaas/backend/internal/platform"
	"github.com/linskybing/nexuspaas/backend/internal/platform/cluster"
)

const dockerCleanupTaskName = cluster.DockerCleanupCronJobName

func registerDockerCleanup(app *platform.App) {
	if app == nil || !app.Config.DockerCleanupEnabled {
		return
	}
	app.RegisterMaintenanceTaskForService(serviceName, dockerCleanupTaskName, func(ctx context.Context) error {
		result := reconcileDockerCleanupCronJob(ctx, app)
		logDockerCleanupResult(result)
		switch result.Action {
		case cluster.DockerCleanupActionFailed, cluster.DockerCleanupActionInvalid, cluster.DockerCleanupActionConflict:
			return fmt.Errorf("docker cleanup CronJob reconciliation %s: %s", result.Action, firstNonEmpty(result.Error, result.Reason))
		default:
			return nil
		}
	})
}

func reconcileDockerCleanupCronJob(ctx context.Context, app *platform.App) cluster.DockerCleanupCronJobResult {
	if app == nil || app.Cluster == nil {
		return cluster.DockerCleanupCronJobResult{
			Namespace: firstNonEmpty(appDockerCleanupNamespace(app), cluster.DockerCleanupDefaultNamespace),
			Name:      cluster.DockerCleanupCronJobName,
			Action:    cluster.DockerCleanupActionDegraded,
			Reason:    "cluster client unavailable",
		}
	}
	return app.Cluster.EnsureDockerCleanupCronJob(ctx, cluster.DockerCleanupCronJobOptions{
		Namespace: app.Config.DockerCleanupNamespace,
		Image:     app.Config.DockerCleanupImage,
	})
}

func appDockerCleanupNamespace(app *platform.App) string {
	if app == nil {
		return ""
	}
	return app.Config.DockerCleanupNamespace
}

func logDockerCleanupResult(result cluster.DockerCleanupCronJobResult) {
	slog.Info("docker cleanup CronJob reconciliation completed",
		"namespace", result.Namespace,
		"name", result.Name,
		"action", result.Action,
		"reason", result.Reason,
		"error", result.Error,
	)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
