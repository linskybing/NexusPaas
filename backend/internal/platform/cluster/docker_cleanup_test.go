package cluster

import (
	"context"
	"errors"
	"strings"
	"testing"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

func TestDockerCleanupCreatesMissingCronJob(t *testing.T) {
	cl := New(fake.NewSimpleClientset(), "proj")

	result := cl.EnsureDockerCleanupCronJob(context.Background(), DockerCleanupCronJobOptions{
		Namespace: "cleanup-e2e",
		Image:     "docker:25-dind",
	})
	if result.Action != DockerCleanupActionCreated {
		t.Fatalf("action = %#v, want created", result)
	}
	cronJob := getDockerCleanupCronJob(t, cl, "cleanup-e2e")
	assertDockerCleanupCronJobSpec(t, cronJob, "cleanup-e2e", "docker:25-dind")
	assertDockerCleanupManaged(t, cronJob)
}

func TestDockerCleanupUpdatesManagedDriftIncludingTokenAutomount(t *testing.T) {
	cronJob := managedDockerCleanupCronJob("cleanup-e2e", "docker:old")
	cronJob.Spec.Schedule = "*/5 * * * *"
	cronJob.Spec.JobTemplate.Spec.Template.Spec.AutomountServiceAccountToken = nil
	cl := New(fake.NewSimpleClientset(cronJob), "proj")

	result := cl.EnsureDockerCleanupCronJob(context.Background(), DockerCleanupCronJobOptions{
		Namespace: "cleanup-e2e",
		Image:     "docker:25-dind",
	})
	if result.Action != DockerCleanupActionUpdated {
		t.Fatalf("action = %#v, want updated", result)
	}
	updated := getDockerCleanupCronJob(t, cl, "cleanup-e2e")
	assertDockerCleanupCronJobSpec(t, updated, "cleanup-e2e", "docker:25-dind")
}

func TestDockerCleanupLeavesManagedCronJobUnchanged(t *testing.T) {
	cl := New(fake.NewSimpleClientset(managedDockerCleanupCronJob("cleanup-e2e", "docker:24-dind")), "proj")

	result := cl.EnsureDockerCleanupCronJob(context.Background(), DockerCleanupCronJobOptions{Namespace: "cleanup-e2e"})
	if result.Action != DockerCleanupActionUnchanged {
		t.Fatalf("action = %#v, want unchanged", result)
	}
}

func TestDockerCleanupSafelyAdoptsCompatibleUnmanagedCronJob(t *testing.T) {
	cronJob := managedDockerCleanupCronJob("cleanup-e2e", "docker:20-dind")
	cronJob.Labels = nil
	cronJob.Annotations = nil
	token := true
	cronJob.Spec.JobTemplate.Spec.Template.Spec.AutomountServiceAccountToken = &token
	cl := New(fake.NewSimpleClientset(cronJob), "proj")

	result := cl.EnsureDockerCleanupCronJob(context.Background(), DockerCleanupCronJobOptions{
		Namespace: "cleanup-e2e",
		Image:     "docker:25-dind",
	})
	if result.Action != DockerCleanupActionAdopted {
		t.Fatalf("action = %#v, want adopted", result)
	}
	adopted := getDockerCleanupCronJob(t, cl, "cleanup-e2e")
	assertDockerCleanupManaged(t, adopted)
	assertDockerCleanupCronJobSpec(t, adopted, "cleanup-e2e", "docker:25-dind")
}

func TestDockerCleanupUnmanagedConflictDoesNotMutate(t *testing.T) {
	cronJob := managedDockerCleanupCronJob("cleanup-e2e", "docker:old")
	cronJob.Labels = nil
	cronJob.Annotations = nil
	cronJob.Spec.JobTemplate.Spec.Template.Spec.Containers[0].Args = []string{"echo no"}
	cl := New(fake.NewSimpleClientset(cronJob), "proj")

	result := cl.EnsureDockerCleanupCronJob(context.Background(), DockerCleanupCronJobOptions{
		Namespace: "cleanup-e2e",
		Image:     "docker:25-dind",
	})
	if result.Action != DockerCleanupActionConflict {
		t.Fatalf("action = %#v, want conflict", result)
	}
	unchanged := getDockerCleanupCronJob(t, cl, "cleanup-e2e")
	if unchanged.Labels[DockerCleanupManagedByLabel] != "" || unchanged.Spec.JobTemplate.Spec.Template.Spec.Containers[0].Image != "docker:old" {
		t.Fatalf("unmanaged conflict was mutated: labels=%#v image=%q", unchanged.Labels, unchanged.Spec.JobTemplate.Spec.Template.Spec.Containers[0].Image)
	}
	if !strings.Contains(result.Reason, "incompatible") {
		t.Fatalf("conflict reason = %q, want incompatible cleanup intent", result.Reason)
	}
}

func TestDockerCleanupConflictingOwnershipMarkerDoesNotMutate(t *testing.T) {
	cronJob := managedDockerCleanupCronJob("cleanup-e2e", "docker:old")
	cronJob.Labels[DockerCleanupOwnerLabel] = "other-service"
	cl := New(fake.NewSimpleClientset(cronJob), "proj")

	result := cl.EnsureDockerCleanupCronJob(context.Background(), DockerCleanupCronJobOptions{Namespace: "cleanup-e2e"})
	if result.Action != DockerCleanupActionConflict || !strings.Contains(result.Reason, DockerCleanupOwnerLabel) {
		t.Fatalf("result = %#v, want owner marker conflict", result)
	}
	unchanged := getDockerCleanupCronJob(t, cl, "cleanup-e2e")
	if unchanged.Labels[DockerCleanupOwnerLabel] != "other-service" {
		t.Fatalf("conflicting CronJob was mutated: %#v", unchanged.Labels)
	}
}

func TestDockerCleanupRejectsInvalidOptions(t *testing.T) {
	cl := New(fake.NewSimpleClientset(), "proj")
	cases := []DockerCleanupCronJobOptions{
		{Namespace: "Bad_Name", Image: "docker:24-dind"},
		{Namespace: strings.Repeat("a", 64), Image: "docker:24-dind"},
	}
	for _, opts := range cases {
		result := cl.EnsureDockerCleanupCronJob(context.Background(), opts)
		if result.Action != DockerCleanupActionInvalid {
			t.Fatalf("opts %#v result = %#v, want invalid", opts, result)
		}
	}
}

func TestDockerCleanupDegradesWhenClientUnavailable(t *testing.T) {
	result := New(nil, "proj").EnsureDockerCleanupCronJob(context.Background(), DockerCleanupCronJobOptions{})
	if result.Action != DockerCleanupActionDegraded || result.Namespace != DockerCleanupDefaultNamespace {
		t.Fatalf("result = %#v, want degraded default namespace", result)
	}
}

func TestDockerCleanupKubernetesOperationErrorIsReported(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	clientset.PrependReactor("create", "cronjobs", func(action k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, errors.New("api denied")
	})
	cl := New(clientset, "proj")

	result := cl.EnsureDockerCleanupCronJob(context.Background(), DockerCleanupCronJobOptions{Namespace: "cleanup-e2e"})
	if result.Action != DockerCleanupActionFailed || result.Error == "" {
		t.Fatalf("result = %#v, want failed with error", result)
	}
}

func managedDockerCleanupCronJob(namespace, image string) *batchv1.CronJob {
	return buildDockerCleanupCronJob(normalizeDockerCleanupOptions(DockerCleanupCronJobOptions{
		Namespace: namespace,
		Image:     image,
	}))
}

func getDockerCleanupCronJob(t *testing.T, cl *Client, namespace string) *batchv1.CronJob {
	t.Helper()
	cronJob, err := cl.Clientset().BatchV1().CronJobs(namespace).Get(context.Background(), DockerCleanupCronJobName, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get Docker cleanup CronJob: %v", err)
	}
	return cronJob
}

func assertDockerCleanupManaged(t *testing.T, cronJob *batchv1.CronJob) {
	t.Helper()
	for key, want := range dockerCleanupManagedLabels() {
		if got := cronJob.Labels[key]; got != want {
			t.Fatalf("managed label %s = %q, want %q on %#v", key, got, want, cronJob.Labels)
		}
	}
	if got := cronJob.Annotations[DockerCleanupManagedAnnotation]; got != DockerCleanupManagedResource {
		t.Fatalf("managed annotation = %q, want %q", got, DockerCleanupManagedResource)
	}
}

func assertDockerCleanupCronJobSpec(t *testing.T, cronJob *batchv1.CronJob, namespace, image string) {
	t.Helper()
	if cronJob.Name != DockerCleanupCronJobName || cronJob.Namespace != namespace {
		t.Fatalf("CronJob identity = %s/%s, want %s/%s", cronJob.Namespace, cronJob.Name, namespace, DockerCleanupCronJobName)
	}
	if cronJob.Spec.Schedule != DockerCleanupSchedule {
		t.Fatalf("schedule = %q, want %q", cronJob.Spec.Schedule, DockerCleanupSchedule)
	}
	podSpec := cronJob.Spec.JobTemplate.Spec.Template.Spec
	if podSpec.RestartPolicy != corev1.RestartPolicyOnFailure {
		t.Fatalf("restartPolicy = %q, want OnFailure", podSpec.RestartPolicy)
	}
	if podSpec.AutomountServiceAccountToken == nil || *podSpec.AutomountServiceAccountToken {
		t.Fatalf("automountServiceAccountToken = %#v, want explicit false", podSpec.AutomountServiceAccountToken)
	}
	if len(podSpec.Containers) != 1 {
		t.Fatalf("containers = %#v, want exactly one", podSpec.Containers)
	}
	container := podSpec.Containers[0]
	if container.Name != DockerCleanupContainerName || container.Image != image {
		t.Fatalf("container = %s/%s, want %s/%s", container.Name, container.Image, DockerCleanupContainerName, image)
	}
	if !reflectDockerCleanupContainer(container) {
		t.Fatalf("container does not match cleanup command/security/mounts: %#v", container)
	}
	if len(podSpec.Volumes) != 1 || podSpec.Volumes[0].HostPath == nil || podSpec.Volumes[0].HostPath.Path != DockerCleanupSocketPath {
		t.Fatalf("volumes = %#v, want docker socket hostPath", podSpec.Volumes)
	}
}

func reflectDockerCleanupContainer(container corev1.Container) bool {
	if len(container.Command) != 2 || container.Command[0] != "/bin/sh" || container.Command[1] != "-c" {
		return false
	}
	if len(container.Args) != 1 || !strings.Contains(container.Args[0], `docker system prune -af --filter "until=24h"`) {
		return false
	}
	if !containerPrivileged(container) {
		return false
	}
	return len(container.VolumeMounts) == 1 &&
		container.VolumeMounts[0].Name == DockerCleanupSocketVolumeName &&
		container.VolumeMounts[0].MountPath == DockerCleanupSocketPath
}
