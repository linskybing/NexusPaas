//go:build e2e

package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/platform"
	"github.com/linskybing/nexuspaas/backend/internal/platform/cluster"
	"github.com/linskybing/nexuspaas/backend/internal/services/k8scontrol"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const fastTransferMoverKindAdmissionEnv = "TEST_LIVE_FAST_TRANSFER_MOVER_KIND_ADMISSION"

func TestFastTransferMoverKindAdmissionE2E(t *testing.T) {
	if strings.TrimSpace(os.Getenv(fastTransferMoverKindAdmissionEnv)) != "1" {
		t.Skip("set " + fastTransferMoverKindAdmissionEnv + "=1 to run live FastTransfer mover kind admission e2e")
	}
	requireLiveKubeconfig(t)

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	cl, err := cluster.NewFromEnv("proj")
	if err != nil {
		t.Fatalf("create live Kubernetes client: %v", err)
	}
	if cl == nil {
		t.Fatal("live Kubernetes client is unavailable")
	}
	if err := cl.Ping(ctx); err != nil {
		t.Fatalf("ping live Kubernetes cluster: %v", err)
	}

	suffix := strconv.FormatInt(time.Now().UTC().UnixNano(), 36)
	namespace := "ft-mover-" + suffix
	createFastTransferMoverAdmissionNamespace(t, ctx, cl, namespace)

	app := newFastTransferMoverAdmissionApp(cl)
	server := httptest.NewServer(app)
	t.Cleanup(server.Close)

	transferID := "p" + suffix + ":" + namespace + ":copy-" + suffix
	body := fastTransferMoverAdmissionRequest(namespace, "copy-"+suffix, transferID)

	first := postFastTransferMoverAdmission(t, server.URL, body, http.StatusCreated)
	if first.Action != "created" {
		t.Fatalf("first action = %q, want created", first.Action)
	}
	job := getFastTransferMoverAdmissionJob(t, ctx, cl, first.Namespace, first.Name)
	assertFastTransferMoverAdmissionJob(t, job, namespace, first.Name, transferID)

	replay := postFastTransferMoverAdmission(t, server.URL, body, http.StatusOK)
	if replay.Action != "already_exists" || replay.Namespace != first.Namespace || replay.Name != first.Name {
		t.Fatalf("replay = %#v, want already_exists for same job", replay)
	}
	assertFastTransferMoverAdmissionJobCount(t, ctx, cl, namespace, transferID, 1)
}

func newFastTransferMoverAdmissionApp(cl *cluster.Client) *platform.App {
	app := platform.NewApp(platform.Config{
		ServiceName:            "k8s-control-service",
		HTTPAddr:               ":0",
		RequireAuth:            true,
		ServiceAPIKey:          "fast-transfer-e2e-service-key",
		FastTransferMoverImage: cluster.FastTransferMoverDefaultImage,
	}, platform.WithCluster(cl))
	app.RegisterService(k8scontrol.Spec())
	k8scontrol.Register(app)
	return app
}

func createFastTransferMoverAdmissionNamespace(t *testing.T, ctx context.Context, cl *cluster.Client, namespace string) {
	t.Helper()
	_, err := cl.Clientset().CoreV1().Namespaces().Create(ctx, &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:   namespace,
			Labels: map[string]string{"nexuspaas.io/e2e": "fast-transfer-mover-kind-admission"},
		},
	}, metav1.CreateOptions{})
	if err != nil && !apierrors.IsAlreadyExists(err) {
		t.Fatalf("create namespace %s: %v", namespace, err)
	}
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := cl.Clientset().CoreV1().Namespaces().Delete(cleanupCtx, namespace, metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
			t.Logf("cleanup namespace %s: %v", namespace, err)
		}
	})
}

func fastTransferMoverAdmissionRequest(namespace, name, transferID string) map[string]any {
	return map[string]any{
		"project_id":        "p-" + namespace,
		"transfer_id":       transferID,
		"target_namespace":  namespace,
		"name":              name,
		"source":            map[string]any{"namespace": namespace, "pvc": "source-pvc", "path": "/data/source"},
		"target":            map[string]any{"namespace": namespace, "pvc": "target-pvc", "path": "/data/target"},
		"tool":              "rsync",
		"progress_callback": map[string]any{"path": fmt.Sprintf("/internal/storage/projects/%s/transfers/%s/%s/progress", "p-"+namespace, namespace, name)},
	}
}

type fastTransferMoverAdmissionResult struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	Action    string `json:"action"`
}

func postFastTransferMoverAdmission(t *testing.T, baseURL string, body map[string]any, wantStatus int) fastTransferMoverAdmissionResult {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	req, err := http.NewRequest(http.MethodPost, baseURL+"/internal/k8s-control/fast-transfers/mover-jobs", bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Service-Key", "fast-transfer-e2e-service-key")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("post mover job: %v", err)
	}
	defer resp.Body.Close()
	var envelope struct {
		Data  fastTransferMoverAdmissionResult `json:"data"`
		Error *platform.ErrorBody              `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.StatusCode != wantStatus {
		t.Fatalf("status = %d data=%#v error=%#v, want %d", resp.StatusCode, envelope.Data, envelope.Error, wantStatus)
	}
	return envelope.Data
}

func getFastTransferMoverAdmissionJob(t *testing.T, ctx context.Context, cl *cluster.Client, namespace, name string) *batchv1.Job {
	t.Helper()
	job, err := cl.Clientset().BatchV1().Jobs(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get mover Job %s/%s: %v", namespace, name, err)
	}
	return job
}

func assertFastTransferMoverAdmissionJob(t *testing.T, job *batchv1.Job, namespace, name, transferID string) {
	t.Helper()
	if job.Namespace != namespace || job.Name != name {
		t.Fatalf("Job identity = %s/%s, want %s/%s", job.Namespace, job.Name, namespace, name)
	}
	assertFastTransferMoverAdmissionMetadata(t, job, transferID)
	pod := job.Spec.Template.Spec
	if pod.RestartPolicy != corev1.RestartPolicyNever {
		t.Fatalf("restartPolicy = %q, want Never", pod.RestartPolicy)
	}
	if pod.AutomountServiceAccountToken == nil || *pod.AutomountServiceAccountToken {
		t.Fatalf("automountServiceAccountToken = %#v, want explicit false", pod.AutomountServiceAccountToken)
	}
	if len(pod.Containers) != 1 {
		t.Fatalf("containers = %#v, want exactly one", pod.Containers)
	}
	container := pod.Containers[0]
	if container.Name != cluster.FastTransferMoverContainerName {
		t.Fatalf("container name = %q, want %q", container.Name, cluster.FastTransferMoverContainerName)
	}
	if len(container.Command) != 2 || container.Command[0] != "/bin/sh" || container.Command[1] != "-c" {
		t.Fatalf("command = %#v, want /bin/sh -c", container.Command)
	}
	if len(container.Args) != 1 || !strings.Contains(container.Args[0], "set -eu") || !strings.Contains(container.Args[0], "rsync -a --delete --") {
		t.Fatalf("args = %#v, want restricted rsync script", container.Args)
	}
	if container.SecurityContext != nil && container.SecurityContext.Privileged != nil && *container.SecurityContext.Privileged {
		t.Fatalf("container is privileged: %#v", container.SecurityContext)
	}
	assertFastTransferMoverAdmissionVolumes(t, pod, container)
}

func assertFastTransferMoverAdmissionMetadata(t *testing.T, job *batchv1.Job, transferID string) {
	t.Helper()
	wantLabels := map[string]string{
		"app.kubernetes.io/managed-by": "platform-backend",
		"app.kubernetes.io/part-of":    "platform",
		"app.kubernetes.io/component":  "fast-transfer-mover",
		"nexuspaas.io/owner":           "k8s-control-service",
	}
	for key, want := range wantLabels {
		if got := job.Labels[key]; got != want {
			t.Fatalf("label %s = %q, want %q on %#v", key, got, want, job.Labels)
		}
	}
	if got := job.Annotations["nexuspaas.io/managed-resource"]; got != "fast-transfer-mover" {
		t.Fatalf("managed annotation = %q, want fast-transfer-mover", got)
	}
	if got := job.Annotations["nexuspaas.io/fast-transfer-id"]; got != transferID {
		t.Fatalf("transfer annotation = %q, want %q", got, transferID)
	}
}

func assertFastTransferMoverAdmissionVolumes(t *testing.T, pod corev1.PodSpec, container corev1.Container) {
	t.Helper()
	if len(pod.Volumes) != 2 {
		t.Fatalf("volumes = %#v, want source and target PVC volumes", pod.Volumes)
	}
	volumes := map[string]corev1.Volume{}
	for _, volume := range pod.Volumes {
		if volume.HostPath != nil {
			t.Fatalf("volume %s uses hostPath: %#v", volume.Name, volume.HostPath)
		}
		if volume.PersistentVolumeClaim == nil {
			t.Fatalf("volume %s is not PVC-backed: %#v", volume.Name, volume)
		}
		volumes[volume.Name] = volume
	}
	sourceVolume, ok := volumes["source-pvc"]
	if !ok || !sourceVolume.PersistentVolumeClaim.ReadOnly {
		t.Fatalf("source volume = %#v, want read-only source PVC", sourceVolume)
	}
	targetVolume, ok := volumes["target-pvc"]
	if !ok || targetVolume.PersistentVolumeClaim.ReadOnly {
		t.Fatalf("target volume = %#v, want writable target PVC", targetVolume)
	}
	mounts := map[string]corev1.VolumeMount{}
	for _, mount := range container.VolumeMounts {
		mounts[mount.Name] = mount
	}
	if mount, ok := mounts["source-pvc"]; !ok || !mount.ReadOnly {
		t.Fatalf("source mount = %#v, want read-only", mount)
	}
	if mount, ok := mounts["target-pvc"]; !ok || mount.ReadOnly {
		t.Fatalf("target mount = %#v, want writable", mount)
	}
}

func assertFastTransferMoverAdmissionJobCount(t *testing.T, ctx context.Context, cl *cluster.Client, namespace, transferID string, want int) {
	t.Helper()
	jobs, err := cl.Clientset().BatchV1().Jobs(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		t.Fatalf("list Jobs in %s: %v", namespace, err)
	}
	got := 0
	for _, job := range jobs.Items {
		if job.Annotations["nexuspaas.io/fast-transfer-id"] == transferID {
			got++
		}
	}
	if got != want {
		t.Fatalf("Jobs for transfer %s = %d, want %d", transferID, got, want)
	}
}
