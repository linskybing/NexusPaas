//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"io"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/platform"
	"github.com/linskybing/nexuspaas/backend/internal/platform/cluster"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	fastTransferMoverExecutionKindEnv     = "TEST_LIVE_FAST_TRANSFER_MOVER_EXECUTION_KIND"
	fastTransferMoverExecutionHelperImage = "busybox:1.36"
	fastTransferMoverExecutionContent     = "fast-transfer-mover-execution-kind"
)

func TestFastTransferMoverExecutionKindE2E(t *testing.T) {
	if strings.TrimSpace(os.Getenv(fastTransferMoverExecutionKindEnv)) != "1" {
		t.Skip("set " + fastTransferMoverExecutionKindEnv + "=1 to run live FastTransfer mover execution kind e2e")
	}
	requireLiveKubeconfig(t)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
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
	namespace := "ft-exec-" + suffix
	createFastTransferMoverAdmissionNamespace(t, ctx, cl, namespace)
	createFastTransferMoverExecutionPVC(t, ctx, cl, namespace, "source-pvc")
	createFastTransferMoverExecutionPVC(t, ctx, cl, namespace, "target-pvc")

	runFastTransferMoverExecutionPVCPod(t, ctx, cl, namespace, "seed-"+suffix, "source-pvc", fmt.Sprintf(`set -eu
mkdir -p /pvc/data/source
printf %%s %q > /pvc/data/source/hello.txt`, fastTransferMoverExecutionContent))
	waitFastTransferMoverExecutionPVCBound(t, ctx, cl, namespace, "source-pvc")

	k8sServer := httptest.NewServer(newFastTransferMoverAdmissionApp(cl))
	t.Cleanup(k8sServer.Close)

	store := platform.NewStore()
	events := platform.NewEventBus()
	projectID, userID := "p-"+suffix, "u-"+suffix
	seedFastTransferStartAccess(t, store, projectID, userID)
	storageServer := httptest.NewServer(newFastTransferStartStorageApp(store, events, k8sServer.URL))
	t.Cleanup(storageServer.Close)

	name := "copy-" + suffix
	transferID := projectID + ":" + namespace + ":" + name
	payload := fastTransferStartRequest(namespace, name, "idem-"+suffix)

	first := postFastTransferStart(t, storageServer.URL, projectID, userID, payload)
	assertFastTransferStartRecord(t, first, transferID, namespace)
	assertFastTransferStartQueuedEvent(t, events, transferID)

	jobNamespace := textE2E(first["mover_job_namespace"])
	jobName := textE2E(first["mover_job_name"])
	job := getFastTransferMoverAdmissionJob(t, ctx, cl, jobNamespace, jobName)
	assertFastTransferMoverAdmissionJob(t, job, namespace, jobName, transferID)

	waitFastTransferMoverExecutionJobComplete(t, ctx, cl, jobNamespace, jobName)
	waitFastTransferMoverExecutionPVCBound(t, ctx, cl, namespace, "source-pvc")
	waitFastTransferMoverExecutionPVCBound(t, ctx, cl, namespace, "target-pvc")
	assertFastTransferMoverExecutionPodSucceeded(t, ctx, cl, jobNamespace, jobName)

	runFastTransferMoverExecutionPVCPod(t, ctx, cl, namespace, "verify-"+suffix, "target-pvc", fmt.Sprintf(`set -eu
got="$(cat /pvc/data/target/hello.txt)"
[ "$got" = %q ]
set -- $(wc -c < /pvc/data/target/hello.txt)
[ "$1" = %q ]`, fastTransferMoverExecutionContent, strconv.Itoa(len(fastTransferMoverExecutionContent))))
}

func createFastTransferMoverExecutionPVC(t *testing.T, ctx context.Context, cl *cluster.Client, namespace, name string) {
	t.Helper()
	_, err := cl.Clientset().CoreV1().PersistentVolumeClaims(namespace).Create(ctx, &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    map[string]string{"nexuspaas.io/e2e": "fast-transfer-mover-execution-kind"},
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("64Mi")},
			},
		},
	}, metav1.CreateOptions{})
	if err != nil && !apierrors.IsAlreadyExists(err) {
		t.Fatalf("create PVC %s/%s: %v", namespace, name, err)
	}
}

func runFastTransferMoverExecutionPVCPod(t *testing.T, ctx context.Context, cl *cluster.Client, namespace, name, pvcName, script string) *corev1.Pod {
	t.Helper()
	_, err := cl.Clientset().CoreV1().Pods(namespace).Create(ctx, &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    map[string]string{"nexuspaas.io/e2e": "fast-transfer-mover-execution-kind"},
		},
		Spec: corev1.PodSpec{
			RestartPolicy: corev1.RestartPolicyNever,
			Containers: []corev1.Container{{
				Name:    "pvc-helper",
				Image:   fastTransferMoverExecutionHelperImage,
				Command: []string{"/bin/sh", "-c"},
				Args:    []string{script},
				VolumeMounts: []corev1.VolumeMount{{
					Name:      "pvc",
					MountPath: "/pvc",
				}},
			}},
			Volumes: []corev1.Volume{{
				Name: "pvc",
				VolumeSource: corev1.VolumeSource{PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
					ClaimName: pvcName,
				}},
			}},
		},
	}, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("create helper Pod %s/%s: %v", namespace, name, err)
	}
	return waitFastTransferMoverExecutionPodSucceeded(t, ctx, cl, namespace, name)
}

func waitFastTransferMoverExecutionPodSucceeded(t *testing.T, ctx context.Context, cl *cluster.Client, namespace, name string) *corev1.Pod {
	t.Helper()
	deadline := time.Now().Add(45 * time.Second)
	for {
		pod, err := cl.Clientset().CoreV1().Pods(namespace).Get(ctx, name, metav1.GetOptions{})
		if err == nil {
			if pod.Status.Phase == corev1.PodSucceeded && pod.Spec.NodeName != "" {
				return pod
			}
			if pod.Status.Phase == corev1.PodFailed {
				logFastTransferMoverExecutionPodDiagnostics(t, ctx, cl, namespace, name)
				t.Fatalf("Pod %s/%s failed", namespace, name)
			}
		} else if !apierrors.IsNotFound(err) {
			t.Fatalf("get Pod %s/%s: %v", namespace, name, err)
		}
		if err := ctx.Err(); err != nil {
			logFastTransferMoverExecutionPodDiagnostics(t, context.Background(), cl, namespace, name)
			t.Fatalf("wait for Pod %s/%s succeeded: %v", namespace, name, err)
		}
		if time.Now().After(deadline) {
			logFastTransferMoverExecutionPodDiagnostics(t, ctx, cl, namespace, name)
			t.Fatalf("Pod %s/%s did not succeed before deadline", namespace, name)
		}
		time.Sleep(250 * time.Millisecond)
	}
}

func waitFastTransferMoverExecutionPVCBound(t *testing.T, ctx context.Context, cl *cluster.Client, namespace, name string) *corev1.PersistentVolumeClaim {
	t.Helper()
	deadline := time.Now().Add(45 * time.Second)
	for {
		pvc, err := cl.Clientset().CoreV1().PersistentVolumeClaims(namespace).Get(ctx, name, metav1.GetOptions{})
		if err == nil {
			if pvc.Status.Phase == corev1.ClaimBound && pvc.Spec.VolumeName != "" {
				return pvc
			}
		} else if !apierrors.IsNotFound(err) {
			t.Fatalf("get PVC %s/%s: %v", namespace, name, err)
		}
		if err := ctx.Err(); err != nil {
			t.Fatalf("wait for PVC %s/%s Bound: %v", namespace, name, err)
		}
		if time.Now().After(deadline) {
			if err == nil {
				t.Fatalf("PVC %s/%s did not bind; phase=%s volume=%q", namespace, name, pvc.Status.Phase, pvc.Spec.VolumeName)
			}
			t.Fatalf("PVC %s/%s did not bind: %v", namespace, name, err)
		}
		time.Sleep(250 * time.Millisecond)
	}
}

func waitFastTransferMoverExecutionJobComplete(t *testing.T, ctx context.Context, cl *cluster.Client, namespace, name string) *batchv1.Job {
	t.Helper()
	deadline := time.Now().Add(90 * time.Second)
	for {
		job, err := cl.Clientset().BatchV1().Jobs(namespace).Get(ctx, name, metav1.GetOptions{})
		if err == nil {
			if fastTransferMoverExecutionJobComplete(job) {
				return job
			}
			if failed, message := fastTransferMoverExecutionJobFailed(job); failed {
				failFastTransferMoverExecutionJobWait(t, ctx, cl, namespace, name, "Job %s/%s failed: %s", namespace, name, message)
			}
		} else if !apierrors.IsNotFound(err) {
			t.Fatalf("get Job %s/%s: %v", namespace, name, err)
		}
		if err := ctx.Err(); err != nil {
			failFastTransferMoverExecutionJobWait(t, context.Background(), cl, namespace, name, "wait for Job %s/%s Complete: %v", namespace, name, err)
		}
		if time.Now().After(deadline) {
			failFastTransferMoverExecutionJobWait(t, ctx, cl, namespace, name, "Job %s/%s did not complete before deadline", namespace, name)
		}
		time.Sleep(500 * time.Millisecond)
	}
}

func fastTransferMoverExecutionJobComplete(job *batchv1.Job) bool {
	for _, condition := range job.Status.Conditions {
		if condition.Type == batchv1.JobComplete && condition.Status == corev1.ConditionTrue {
			return true
		}
	}
	return false
}

func fastTransferMoverExecutionJobFailed(job *batchv1.Job) (bool, string) {
	for _, condition := range job.Status.Conditions {
		if condition.Type == batchv1.JobFailed && condition.Status == corev1.ConditionTrue {
			return true, condition.Message
		}
	}
	return false, ""
}

func failFastTransferMoverExecutionJobWait(t *testing.T, ctx context.Context, cl *cluster.Client, namespace, name, format string, args ...any) {
	t.Helper()
	logFastTransferMoverExecutionJobPods(t, ctx, cl, namespace, name)
	t.Fatalf(format, args...)
}

func assertFastTransferMoverExecutionPodSucceeded(t *testing.T, ctx context.Context, cl *cluster.Client, namespace, jobName string) {
	t.Helper()
	pods := listFastTransferMoverExecutionJobPods(t, ctx, cl, namespace, jobName)
	for _, pod := range pods {
		if pod.Spec.NodeName != "" && pod.Status.Phase == corev1.PodSucceeded {
			return
		}
	}
	t.Fatalf("no scheduled succeeded mover Pod for Job %s/%s: %#v", namespace, jobName, pods)
}

func listFastTransferMoverExecutionJobPods(t *testing.T, ctx context.Context, cl *cluster.Client, namespace, jobName string) []corev1.Pod {
	t.Helper()
	pods, err := cl.Clientset().CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{LabelSelector: "job-name=" + jobName})
	if err != nil {
		t.Fatalf("list Pods for Job %s/%s: %v", namespace, jobName, err)
	}
	return pods.Items
}

func logFastTransferMoverExecutionJobPods(t *testing.T, ctx context.Context, cl *cluster.Client, namespace, jobName string) {
	t.Helper()
	pods := listFastTransferMoverExecutionJobPods(t, ctx, cl, namespace, jobName)
	for _, pod := range pods {
		logFastTransferMoverExecutionPodDiagnostics(t, ctx, cl, namespace, pod.Name)
	}
}

func logFastTransferMoverExecutionPodDiagnostics(t *testing.T, ctx context.Context, cl *cluster.Client, namespace, name string) {
	t.Helper()
	pod, err := cl.Clientset().CoreV1().Pods(namespace).Get(ctx, name, metav1.GetOptions{})
	if err == nil {
		t.Logf("Pod %s/%s phase=%s node=%q reason=%q message=%q statuses=%#v", namespace, name, pod.Status.Phase, pod.Spec.NodeName, pod.Status.Reason, pod.Status.Message, pod.Status.ContainerStatuses)
	} else if !apierrors.IsNotFound(err) {
		t.Logf("get Pod %s/%s diagnostics: %v", namespace, name, err)
	}
	if events, err := cl.Clientset().CoreV1().Events(namespace).List(ctx, metav1.ListOptions{FieldSelector: "involvedObject.name=" + name}); err == nil {
		for _, event := range events.Items {
			t.Logf("Pod event %s/%s: %s %s", namespace, name, event.Reason, event.Message)
		}
	}
	req := cl.Clientset().CoreV1().Pods(namespace).GetLogs(name, &corev1.PodLogOptions{})
	stream, err := req.Stream(ctx)
	if err != nil {
		t.Logf("get Pod %s/%s logs: %v", namespace, name, err)
		return
	}
	defer stream.Close()
	logs, err := io.ReadAll(io.LimitReader(stream, 64<<10))
	if err != nil {
		t.Logf("read Pod %s/%s logs: %v", namespace, name, err)
		return
	}
	if len(logs) > 0 {
		t.Logf("Pod %s/%s logs:\n%s", namespace, name, strings.TrimSpace(string(logs)))
	}
}
