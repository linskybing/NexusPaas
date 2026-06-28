//go:build e2e

package e2e

import (
	"context"
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
	"k8s.io/apimachinery/pkg/util/intstr"
)

const (
	fastTransferProgressCallbackKindEnv = "TEST_LIVE_FAST_TRANSFER_PROGRESS_CALLBACK_KIND"
	fastTransferProgressCallbackSink    = "ft-callback-sink"
	fastTransferProgressCallbackKey     = "fast-transfer-e2e-callback-key"
)

func TestFastTransferProgressCallbackKindE2E(t *testing.T) {
	if strings.TrimSpace(os.Getenv(fastTransferProgressCallbackKindEnv)) != "1" {
		t.Skip("set " + fastTransferProgressCallbackKindEnv + "=1 to run live FastTransfer progress callback kind e2e")
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
	namespace := "ft-callback-" + suffix
	createFastTransferMoverAdmissionNamespace(t, ctx, cl, namespace)
	waitFastTransferProgressCallbackDefaultServiceAccount(t, ctx, cl, namespace)
	createFastTransferMoverExecutionPVC(t, ctx, cl, namespace, "source-pvc")
	createFastTransferMoverExecutionPVC(t, ctx, cl, namespace, "target-pvc")
	createFastTransferProgressCallbackSink(t, ctx, cl, namespace)
	waitFastTransferProgressCallbackSinkReady(t, ctx, cl, namespace)

	runFastTransferMoverExecutionPVCPod(t, ctx, cl, namespace, "seed-"+suffix, "source-pvc", fmt.Sprintf(`set -eu
mkdir -p /pvc/data/source
printf %%s %q > /pvc/data/source/hello.txt`, fastTransferMoverExecutionContent))
	waitFastTransferMoverExecutionPVCBound(t, ctx, cl, namespace, "source-pvc")

	callbackBaseURL := fmt.Sprintf("http://%s.%s.svc.cluster.local:8080", fastTransferProgressCallbackSink, namespace)
	app := newFastTransferProgressCallbackK8sControlApp(cl, callbackBaseURL)
	server := httptest.NewServer(app)
	t.Cleanup(server.Close)

	name := "copy-" + suffix
	projectID := "p-" + namespace
	transferID := projectID + ":" + namespace + ":" + name
	body := fastTransferMoverAdmissionRequest(namespace, name, transferID)

	first := postFastTransferMoverAdmission(t, server.URL, body, http.StatusCreated)
	if first.Action != "created" {
		t.Fatalf("first action = %q, want created", first.Action)
	}
	job := getFastTransferMoverAdmissionJob(t, ctx, cl, first.Namespace, first.Name)
	assertFastTransferMoverAdmissionJob(t, job, namespace, first.Name, transferID)
	assertFastTransferProgressCallbackEnv(t, job, callbackBaseURL+"/internal/storage/projects/"+projectID+"/transfers/"+namespace+"/"+name+"/progress")

	waitFastTransferMoverExecutionJobComplete(t, ctx, cl, namespace, first.Name)
	assertFastTransferMoverExecutionPodSucceeded(t, ctx, cl, namespace, first.Name)
	runFastTransferMoverExecutionPVCPod(t, ctx, cl, namespace, "verify-"+suffix, "target-pvc", fmt.Sprintf(`set -eu
got="$(cat /pvc/data/target/hello.txt)"
[ "$got" = %q ]`, fastTransferMoverExecutionContent))

	logs := waitFastTransferProgressCallbackSinkLogs(t, ctx, cl, namespace,
		`{"status":"running","progress_pct":1}`,
		`{"status":"succeeded","progress_pct":100}`,
	)
	if !strings.Contains(logs, "X-Service-Name: k8s-control-service") {
		t.Fatalf("callback logs missing service identity header:\n%s", logs)
	}
}

func newFastTransferProgressCallbackK8sControlApp(cl *cluster.Client, callbackBaseURL string) *platform.App {
	app := platform.NewApp(platform.Config{
		ServiceName:             "k8s-control-service",
		HTTPAddr:                ":0",
		RequireAuth:             true,
		ServiceAPIKey:           "fast-transfer-e2e-service-key",
		ServiceIdentityName:     "k8s-control-service",
		ServiceIdentityKey:      fastTransferProgressCallbackKey,
		ServiceURLs:             map[string]string{"storage-service": callbackBaseURL},
		FastTransferMoverImage:  cluster.FastTransferMoverDefaultImage,
		ServiceFallbackDisabled: true,
	}, platform.WithCluster(cl))
	app.RegisterService(k8scontrol.Spec())
	k8scontrol.Register(app)
	return app
}

func waitFastTransferProgressCallbackDefaultServiceAccount(t *testing.T, ctx context.Context, cl *cluster.Client, namespace string) {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for {
		if _, err := cl.Clientset().CoreV1().ServiceAccounts(namespace).Get(ctx, "default", metav1.GetOptions{}); err == nil {
			return
		} else if !apierrors.IsNotFound(err) {
			t.Fatalf("get default ServiceAccount in %s: %v", namespace, err)
		}
		if err := ctx.Err(); err != nil {
			t.Fatalf("wait for default ServiceAccount in %s: %v", namespace, err)
		}
		if time.Now().After(deadline) {
			t.Fatalf("default ServiceAccount in %s was not created before deadline", namespace)
		}
		time.Sleep(250 * time.Millisecond)
	}
}

func createFastTransferProgressCallbackSink(t *testing.T, ctx context.Context, cl *cluster.Client, namespace string) {
	t.Helper()
	labels := map[string]string{
		"app":              fastTransferProgressCallbackSink,
		"nexuspaas.io/e2e": "fast-transfer-progress-callback-kind",
	}
	_, err := cl.Clientset().CoreV1().Pods(namespace).Create(ctx, &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fastTransferProgressCallbackSink,
			Namespace: namespace,
			Labels:    labels,
		},
		Spec: corev1.PodSpec{
			RestartPolicy: corev1.RestartPolicyAlways,
			Containers: []corev1.Container{{
				Name:    "callback-sink",
				Image:   fastTransferMoverExecutionHelperImage,
				Command: []string{"/bin/sh", "-c"},
				Args: []string{`set -eu
cat > /tmp/callback-handler <<'EOF'
#!/bin/sh
content_length=0
while IFS= read -r line; do
  clean="$(printf '%s' "$line" | tr -d '\r')"
  printf 'CALLBACK_HEADER:%s\n' "$clean" >&2
  case "$clean" in
    Content-Length:*|content-length:*)
      content_length="$(printf '%s' "$clean" | tr -dc '0-9')"
      ;;
  esac
  [ -n "$clean" ] || break
done
if [ "${content_length:-0}" -gt 0 ]; then
  body="$(dd bs=1 count="$content_length" 2>/dev/null || true)"
  printf 'CALLBACK_BODY:%s\n' "$body" >&2
fi
printf 'HTTP/1.1 200 OK\r\nContent-Length: 2\r\nConnection: close\r\n\r\nOK'
EOF
chmod +x /tmp/callback-handler
touch /tmp/ready
exec nc -lk -p 8080 -e /tmp/callback-handler`},
				Ports: []corev1.ContainerPort{{ContainerPort: 8080}},
				ReadinessProbe: &corev1.Probe{
					ProbeHandler: corev1.ProbeHandler{
						Exec: &corev1.ExecAction{Command: []string{"/bin/sh", "-c", "test -f /tmp/ready"}},
					},
					PeriodSeconds:    1,
					FailureThreshold: 3,
				},
			}},
		},
	}, metav1.CreateOptions{})
	if err != nil && !apierrors.IsAlreadyExists(err) {
		t.Fatalf("create callback sink Pod: %v", err)
	}

	_, err = cl.Clientset().CoreV1().Services(namespace).Create(ctx, &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fastTransferProgressCallbackSink,
			Namespace: namespace,
			Labels:    labels,
		},
		Spec: corev1.ServiceSpec{
			Selector: labels,
			Ports: []corev1.ServicePort{{
				Name:       "http",
				Port:       8080,
				TargetPort: intstr.FromInt(8080),
			}},
		},
	}, metav1.CreateOptions{})
	if err != nil && !apierrors.IsAlreadyExists(err) {
		t.Fatalf("create callback sink Service: %v", err)
	}
}

func waitFastTransferProgressCallbackSinkReady(t *testing.T, ctx context.Context, cl *cluster.Client, namespace string) {
	t.Helper()
	deadline := time.Now().Add(45 * time.Second)
	for {
		pod, err := cl.Clientset().CoreV1().Pods(namespace).Get(ctx, fastTransferProgressCallbackSink, metav1.GetOptions{})
		if err == nil {
			if fastTransferProgressCallbackSinkReady(pod) {
				return
			}
			if pod.Status.Phase == corev1.PodFailed {
				logFastTransferMoverExecutionPodDiagnostics(t, ctx, cl, namespace, fastTransferProgressCallbackSink)
				t.Fatalf("callback sink Pod failed")
			}
		} else if !apierrors.IsNotFound(err) {
			t.Fatalf("get callback sink Pod: %v", err)
		}
		if err := ctx.Err(); err != nil {
			t.Fatalf("wait for callback sink Pod: %v", err)
		}
		if time.Now().After(deadline) {
			logFastTransferMoverExecutionPodDiagnostics(t, ctx, cl, namespace, fastTransferProgressCallbackSink)
			t.Fatalf("callback sink Pod did not become ready")
		}
		time.Sleep(250 * time.Millisecond)
	}
}

func fastTransferProgressCallbackSinkReady(pod *corev1.Pod) bool {
	if pod == nil || pod.Spec.NodeName == "" {
		return false
	}
	for _, condition := range pod.Status.Conditions {
		if condition.Type == corev1.PodReady && condition.Status == corev1.ConditionTrue {
			return true
		}
	}
	return false
}

func assertFastTransferProgressCallbackEnv(t *testing.T, job *batchv1.Job, wantURL string) {
	t.Helper()
	env := map[string]string{}
	for _, entry := range job.Spec.Template.Spec.Containers[0].Env {
		env[entry.Name] = entry.Value
	}
	if env["NEXUSPAAS_FAST_TRANSFER_PROGRESS_URL"] != wantURL ||
		env["NEXUSPAAS_FAST_TRANSFER_PROGRESS_SERVICE_NAME"] != "k8s-control-service" ||
		env["NEXUSPAAS_FAST_TRANSFER_PROGRESS_KEY"] != fastTransferProgressCallbackKey {
		t.Fatalf("callback env = %#v, want URL/service identity", env)
	}
}

func waitFastTransferProgressCallbackSinkLogs(t *testing.T, ctx context.Context, cl *cluster.Client, namespace string, wants ...string) string {
	t.Helper()
	deadline := time.Now().Add(45 * time.Second)
	for {
		raw, err := cl.Clientset().CoreV1().Pods(namespace).GetLogs(fastTransferProgressCallbackSink, &corev1.PodLogOptions{
			Container: "callback-sink",
		}).Do(ctx).Raw()
		logs := string(raw)
		if err == nil && containsAll(logs, wants...) {
			return logs
		}
		if err := ctx.Err(); err != nil {
			t.Fatalf("wait for callback sink logs: %v; last logs:\n%s", err, logs)
		}
		if time.Now().After(deadline) {
			logFastTransferMoverExecutionPodDiagnostics(t, ctx, cl, namespace, fastTransferProgressCallbackSink)
			t.Fatalf("callback sink logs missing %v; err=%v logs:\n%s", wants, err, logs)
		}
		time.Sleep(500 * time.Millisecond)
	}
}

func containsAll(value string, wants ...string) bool {
	for _, want := range wants {
		if !strings.Contains(value, want) {
			return false
		}
	}
	return true
}
