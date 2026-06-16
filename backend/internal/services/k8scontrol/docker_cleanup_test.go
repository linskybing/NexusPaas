package k8scontrol

import (
	"context"
	"testing"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/platform"
	"github.com/linskybing/nexuspaas/backend/internal/platform/cluster"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestDockerCleanupRegistersOnlyWhenEnabledForOwner(t *testing.T) {
	disabled := platform.NewApp(platform.Config{ServiceName: serviceName, RequireAuth: true})
	Register(disabled)
	if containsTask(disabled.MaintenanceTaskNames(), dockerCleanupTaskName) {
		t.Fatalf("disabled Docker cleanup registered task: %v", disabled.MaintenanceTaskNames())
	}

	enabled := platform.NewApp(platform.Config{
		ServiceName:            serviceName,
		RequireAuth:            true,
		DockerCleanupEnabled:   true,
		DockerCleanupNamespace: "cleanup-e2e",
		DockerCleanupImage:     "docker:25-dind",
	})
	Register(enabled)
	if !containsTask(enabled.MaintenanceTaskNames(), dockerCleanupTaskName) {
		t.Fatalf("enabled k8s-control tasks = %v, want %s", enabled.MaintenanceTaskNames(), dockerCleanupTaskName)
	}

	other := platform.NewApp(platform.Config{
		ServiceName:          "identity-service",
		RequireAuth:          true,
		DockerCleanupEnabled: true,
	})
	Register(other)
	if containsTask(other.MaintenanceTaskNames(), dockerCleanupTaskName) {
		t.Fatalf("unowned service registered %s: %v", dockerCleanupTaskName, other.MaintenanceTaskNames())
	}
}

func TestDockerCleanupMaintenanceCreatesCronJob(t *testing.T) {
	ctx := context.Background()
	cl := cluster.New(fake.NewSimpleClientset(), "proj")
	app := platform.NewApp(platform.Config{
		ServiceName:            serviceName,
		RequireAuth:            true,
		DockerCleanupEnabled:   true,
		DockerCleanupNamespace: "cleanup-e2e",
		DockerCleanupImage:     "docker:25-dind",
	}, platform.WithCluster(cl))
	Register(app)

	app.RunMaintenanceOnce(ctx, time.Second)

	cronJob, err := cl.Clientset().BatchV1().CronJobs("cleanup-e2e").Get(ctx, cluster.DockerCleanupCronJobName, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get Docker cleanup CronJob: %v", err)
	}
	if cronJob.Spec.Schedule != cluster.DockerCleanupSchedule {
		t.Fatalf("schedule = %q, want %q", cronJob.Spec.Schedule, cluster.DockerCleanupSchedule)
	}
	podSpec := cronJob.Spec.JobTemplate.Spec.Template.Spec
	if podSpec.AutomountServiceAccountToken == nil || *podSpec.AutomountServiceAccountToken {
		t.Fatalf("automountServiceAccountToken = %#v, want explicit false", podSpec.AutomountServiceAccountToken)
	}
	if len(podSpec.Containers) != 1 || podSpec.Containers[0].Image != "docker:25-dind" {
		t.Fatalf("containers = %#v, want configured Docker image", podSpec.Containers)
	}
}

func TestDockerCleanupReconcileDegradesWithoutCluster(t *testing.T) {
	app := platform.NewApp(platform.Config{
		ServiceName:            serviceName,
		RequireAuth:            true,
		DockerCleanupEnabled:   true,
		DockerCleanupNamespace: "cleanup-e2e",
	})

	result := reconcileDockerCleanupCronJob(context.Background(), app)
	if result.Action != cluster.DockerCleanupActionDegraded || result.Namespace != "cleanup-e2e" {
		t.Fatalf("result = %#v, want degraded cleanup-e2e", result)
	}
}

func containsTask(tasks []string, want string) bool {
	for _, task := range tasks {
		if task == want {
			return true
		}
	}
	return false
}
