//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/platform"
	"github.com/linskybing/nexuspaas/backend/internal/platform/cluster"
	"github.com/linskybing/nexuspaas/backend/internal/services"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
)

const e2eDockerCleanupRunLabel = "nexuspaas.io/e2e-run"

const e2eDockerCleanupScript = `set -e
echo "Starting Docker cleanup..."
# Remove all unused images, containers, and networks older than 24 hours
docker system prune -af --filter "until=24h"
echo "Docker cleanup completed successfully"`

func TestDockerImageCleanupCronJobProvisionerE2E(t *testing.T) {
	ctx := context.Background()
	runID := "dockercleanup" + sanitizeID(time.Now().UTC().Format("150405.000000000"))

	t.Run("create", func(t *testing.T) {
		namespace := "cleanup-create-" + runID
		app, cl := newDockerCleanupE2EApp(namespace, "docker:e2e-create", runID)
		app.RunMaintenanceOnce(ctx, time.Second)

		cronJob := getE2EDockerCleanupCronJob(t, cl, namespace)
		assertE2EDockerCleanupCronJob(t, cronJob, namespace, "docker:e2e-create")
	})

	t.Run("managed_update", func(t *testing.T) {
		namespace := "cleanup-update-" + runID
		existing := e2eDockerCleanupCronJob(namespace, "docker:old", runID, true)
		existing.Spec.Schedule = "*/5 * * * *"
		token := true
		existing.Spec.JobTemplate.Spec.Template.Spec.AutomountServiceAccountToken = &token
		app, cl := newDockerCleanupE2EAppWithObjects(namespace, "docker:e2e-update", runID, existing)
		app.RunMaintenanceOnce(ctx, time.Second)

		cronJob := getE2EDockerCleanupCronJob(t, cl, namespace)
		assertE2EDockerCleanupCronJob(t, cronJob, namespace, "docker:e2e-update")
	})

	t.Run("unmanaged_conflict", func(t *testing.T) {
		namespace := "cleanup-conflict-" + runID
		existing := e2eDockerCleanupCronJob(namespace, "docker:unmanaged", runID, false)
		existing.Spec.JobTemplate.Spec.Template.Spec.Containers[0].Args = []string{"echo not-the-platform-cleanup"}
		app, cl := newDockerCleanupE2EAppWithObjects(namespace, "docker:e2e-conflict", runID, existing)
		app.RunMaintenanceOnce(ctx, time.Second)

		cronJob := getE2EDockerCleanupCronJob(t, cl, namespace)
		if cronJob.Labels[cluster.DockerCleanupManagedByLabel] != "" ||
			cronJob.Spec.JobTemplate.Spec.Template.Spec.Containers[0].Image != "docker:unmanaged" {
			t.Fatalf("unmanaged conflict CronJob mutated: labels=%#v image=%q", cronJob.Labels, cronJob.Spec.JobTemplate.Spec.Template.Spec.Containers[0].Image)
		}
	})
}

func TestDockerImageCleanupCronJobLiveK8sE2E(t *testing.T) {
	if strings.TrimSpace(os.Getenv("TEST_LIVE_K8S_DOCKER_CLEANUP")) != "1" {
		t.Skip("TEST_LIVE_K8S_DOCKER_CLEANUP=1 not set; skipping live Kubernetes docker cleanup e2e")
	}
	ensureDefaultKubeconfigNoSkip(t)
	ctx := context.Background()
	cl, err := cluster.NewFromEnv("proj")
	if err != nil {
		t.Fatalf("create cluster client: %v", err)
	}
	if cl == nil {
		t.Fatal("no Kubernetes client available")
	}

	runID := "dockercleanup" + sanitizeID(time.Now().UTC().Format("150405.000000000"))
	namespace := "nexuspaas-docker-cleanup-e2e-" + sanitizeID(time.Now().UTC().Format("150405000000"))
	cleanup := func() {
		if leftovers := cleanupE2EDockerCleanupLive(ctx, cl, namespace); len(leftovers) > 0 {
			t.Errorf("leftover Docker cleanup E2E resources: %s", strings.Join(leftovers, ","))
		}
	}
	t.Cleanup(cleanup)
	cleanup()

	if err := cl.EnsureNamespace(ctx, namespace); err != nil {
		t.Fatalf("create namespace %s: %v", namespace, err)
	}
	app := platform.NewApp(platform.Config{
		ServiceName:            k8sControlService,
		RequireAuth:            false,
		DockerCleanupEnabled:   true,
		DockerCleanupNamespace: namespace,
		DockerCleanupImage:     cluster.DockerCleanupDefaultImage,
	}, platform.WithCluster(cl))
	services.RegisterAll(app)
	if !containsE2ETask(app.MaintenanceTaskNames(), cluster.DockerCleanupCronJobName) {
		t.Fatalf("maintenance tasks = %v, want %s", app.MaintenanceTaskNames(), cluster.DockerCleanupCronJobName)
	}

	app.RunMaintenanceOnce(ctx, time.Second)

	cronJob := getE2EDockerCleanupCronJob(t, cl, namespace)
	labels := cronJob.GetLabels()
	if labels == nil {
		labels = map[string]string{}
	}
	labels[e2eDockerCleanupRunLabel] = runID
	cronJob.Labels = labels
	if _, err := cl.Clientset().BatchV1().CronJobs(namespace).Update(ctx, cronJob, metav1.UpdateOptions{}); err != nil {
		t.Fatalf("label live Docker cleanup CronJob for cleanup tracking: %v", err)
	}
	assertE2EDockerCleanupCronJob(t, cronJob, namespace, cluster.DockerCleanupDefaultImage)
	cleanup()
}

func newDockerCleanupE2EApp(namespace, image, runID string) (*platform.App, *cluster.Client) {
	return newDockerCleanupE2EAppWithObjects(namespace, image, runID)
}

func newDockerCleanupE2EAppWithObjects(namespace, image, _ string, objects ...runtime.Object) (*platform.App, *cluster.Client) {
	cl := cluster.New(fake.NewSimpleClientset(objects...), "proj")
	app := platform.NewApp(platform.Config{
		ServiceName:            k8sControlService,
		RequireAuth:            false,
		DockerCleanupEnabled:   true,
		DockerCleanupNamespace: namespace,
		DockerCleanupImage:     image,
	}, platform.WithCluster(cl))
	services.RegisterAll(app)
	return app, cl
}

func e2eDockerCleanupCronJob(namespace, image, runID string, managed bool) *batchv1.CronJob {
	automount := false
	privileged := true
	labels := map[string]string{e2eDockerCleanupRunLabel: runID}
	annotations := map[string]string{}
	if managed {
		labels[cluster.DockerCleanupManagedByLabel] = cluster.DockerCleanupManagedByValue
		labels[cluster.DockerCleanupPartOfLabel] = cluster.DockerCleanupPartOfValue
		labels[cluster.DockerCleanupComponentLabel] = cluster.DockerCleanupComponentValue
		labels[cluster.DockerCleanupOwnerLabel] = cluster.DockerCleanupOwnerValue
		annotations[cluster.DockerCleanupManagedAnnotation] = cluster.DockerCleanupManagedResource
	}
	return &batchv1.CronJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:        cluster.DockerCleanupCronJobName,
			Namespace:   namespace,
			Labels:      labels,
			Annotations: annotations,
		},
		Spec: batchv1.CronJobSpec{
			Schedule: cluster.DockerCleanupSchedule,
			JobTemplate: batchv1.JobTemplateSpec{
				Spec: batchv1.JobSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							RestartPolicy:                corev1.RestartPolicyOnFailure,
							AutomountServiceAccountToken: &automount,
							Containers: []corev1.Container{{
								Name:    cluster.DockerCleanupContainerName,
								Image:   image,
								Command: []string{"/bin/sh", "-c"},
								Args:    []string{e2eDockerCleanupScript},
								SecurityContext: &corev1.SecurityContext{
									Privileged: &privileged,
								},
								VolumeMounts: []corev1.VolumeMount{{
									Name:      cluster.DockerCleanupSocketVolumeName,
									MountPath: cluster.DockerCleanupSocketPath,
								}},
							}},
							Volumes: []corev1.Volume{{
								Name: cluster.DockerCleanupSocketVolumeName,
								VolumeSource: corev1.VolumeSource{
									HostPath: &corev1.HostPathVolumeSource{Path: cluster.DockerCleanupSocketPath},
								},
							}},
						},
					},
				},
			},
		},
	}
}

func getE2EDockerCleanupCronJob(t *testing.T, cl *cluster.Client, namespace string) *batchv1.CronJob {
	t.Helper()
	cronJob, err := cl.Clientset().BatchV1().CronJobs(namespace).Get(context.Background(), cluster.DockerCleanupCronJobName, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get Docker cleanup CronJob %s/%s: %v", namespace, cluster.DockerCleanupCronJobName, err)
	}
	return cronJob
}

func assertE2EDockerCleanupCronJob(t *testing.T, cronJob *batchv1.CronJob, namespace, image string) {
	t.Helper()
	assertE2EDockerCleanupIdentity(t, cronJob, namespace)
	podSpec := cronJob.Spec.JobTemplate.Spec.Template.Spec
	assertE2EDockerCleanupPodSpec(t, podSpec)
	assertE2EDockerCleanupContainer(t, podSpec.Containers[0], image)
	assertE2EDockerCleanupManaged(t, cronJob)
}

func assertE2EDockerCleanupIdentity(t *testing.T, cronJob *batchv1.CronJob, namespace string) {
	t.Helper()
	if cronJob.Namespace != namespace || cronJob.Name != cluster.DockerCleanupCronJobName {
		t.Fatalf("CronJob identity = %s/%s, want %s/%s", cronJob.Namespace, cronJob.Name, namespace, cluster.DockerCleanupCronJobName)
	}
	if cronJob.Spec.Schedule != cluster.DockerCleanupSchedule {
		t.Fatalf("schedule = %q, want %q", cronJob.Spec.Schedule, cluster.DockerCleanupSchedule)
	}
}

func assertE2EDockerCleanupPodSpec(t *testing.T, podSpec corev1.PodSpec) {
	t.Helper()
	if podSpec.AutomountServiceAccountToken == nil || *podSpec.AutomountServiceAccountToken {
		t.Fatalf("automountServiceAccountToken = %#v, want explicit false", podSpec.AutomountServiceAccountToken)
	}
	if podSpec.RestartPolicy != corev1.RestartPolicyOnFailure {
		t.Fatalf("restartPolicy = %q, want OnFailure", podSpec.RestartPolicy)
	}
	if len(podSpec.Containers) != 1 {
		t.Fatalf("containers = %#v, want one Docker cleanup container", podSpec.Containers)
	}
	if len(podSpec.Volumes) != 1 || podSpec.Volumes[0].HostPath == nil || podSpec.Volumes[0].HostPath.Path != cluster.DockerCleanupSocketPath {
		t.Fatalf("volumes = %#v, want docker socket hostPath", podSpec.Volumes)
	}
}

func assertE2EDockerCleanupContainer(t *testing.T, container corev1.Container, image string) {
	t.Helper()
	if container.Name != cluster.DockerCleanupContainerName || container.Image != image {
		t.Fatalf("container = %s/%s, want %s/%s", container.Name, container.Image, cluster.DockerCleanupContainerName, image)
	}
	if len(container.Command) != 2 || container.Command[0] != "/bin/sh" || container.Command[1] != "-c" {
		t.Fatalf("command = %#v, want /bin/sh -c", container.Command)
	}
	if len(container.Args) != 1 || !strings.Contains(container.Args[0], `docker system prune -af --filter "until=24h"`) {
		t.Fatalf("args = %#v, want Docker prune command", container.Args)
	}
	if container.SecurityContext == nil || container.SecurityContext.Privileged == nil || !*container.SecurityContext.Privileged {
		t.Fatalf("securityContext = %#v, want privileged true", container.SecurityContext)
	}
	if len(container.VolumeMounts) != 1 || container.VolumeMounts[0].MountPath != cluster.DockerCleanupSocketPath {
		t.Fatalf("volumeMounts = %#v, want docker socket mount", container.VolumeMounts)
	}
}

func assertE2EDockerCleanupManaged(t *testing.T, cronJob *batchv1.CronJob) {
	t.Helper()
	for key, want := range map[string]string{
		cluster.DockerCleanupManagedByLabel: cluster.DockerCleanupManagedByValue,
		cluster.DockerCleanupPartOfLabel:    cluster.DockerCleanupPartOfValue,
		cluster.DockerCleanupComponentLabel: cluster.DockerCleanupComponentValue,
		cluster.DockerCleanupOwnerLabel:     cluster.DockerCleanupOwnerValue,
	} {
		if got := cronJob.Labels[key]; got != want {
			t.Fatalf("managed label %s = %q, want %q on %#v", key, got, want, cronJob.Labels)
		}
	}
	if got := cronJob.Annotations[cluster.DockerCleanupManagedAnnotation]; got != cluster.DockerCleanupManagedResource {
		t.Fatalf("managed annotation = %q, want %q", got, cluster.DockerCleanupManagedResource)
	}
}

func cleanupE2EDockerCleanupLive(ctx context.Context, cl *cluster.Client, namespace string) []string {
	leftovers := []string{}
	if err := cl.Clientset().BatchV1().CronJobs(namespace).Delete(ctx, cluster.DockerCleanupCronJobName, metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
		leftovers = append(leftovers, fmt.Sprintf("%s/%s(delete:%v)", namespace, cluster.DockerCleanupCronJobName, err))
	}
	if err := cl.Clientset().CoreV1().Namespaces().Delete(ctx, namespace, metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
		leftovers = append(leftovers, fmt.Sprintf("%s(delete:%v)", namespace, err))
	}
	return leftovers
}

func containsE2ETask(tasks []string, want string) bool {
	for _, task := range tasks {
		if task == want {
			return true
		}
	}
	return false
}
