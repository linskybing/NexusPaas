//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/contracts"
	"github.com/linskybing/nexuspaas/backend/internal/platform/cluster"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var (
	liveDRAClaimTemplateGVR = schema.GroupVersionResource{Group: "resource.k8s.io", Version: "v1", Resource: "resourceclaimtemplates"}
	liveDRADeviceClassGVR   = schema.GroupVersionResource{Group: "resource.k8s.io", Version: "v1", Resource: "deviceclasses"}
)

func TestLiveK8sConfigFileDRADispatchE2E(t *testing.T) {
	if strings.TrimSpace(os.Getenv("TEST_LIVE_K8S_CONFIGFILE_DRA")) != "1" {
		t.Skip("set TEST_LIVE_K8S_CONFIGFILE_DRA=1 to run live ConfigFile DRA dispatch e2e")
	}
	requireLiveKubeconfig(t)
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	cl, err := cluster.NewFromEnv("proj")
	if err != nil {
		t.Fatalf("create live Kubernetes client: %v", err)
	}
	if cl == nil || cl.DynamicClient() == nil {
		t.Skip("live Kubernetes dynamic client is unavailable; DRA ResourceClaimTemplate dispatch cannot be verified")
	}
	if err := cl.Ping(ctx); err != nil {
		t.Fatalf("ping live Kubernetes cluster: %v", err)
	}
	deviceClassName := requireLiveDRASupport(t, ctx, cl)

	h := newHarnessWithPeers(t, map[string][]string{
		schedulerQuotaService: {orgProjectService, workloadService},
		workloadService:       {identityService, orgProjectService, schedulerQuotaService},
	}, identityService, orgProjectService, schedulerQuotaService, workloadService)
	h.services[workloadService].app.Cluster = cl
	ids := h.seedIdentityContracts()
	h.seedSchedulerAdmissionData(ids.userID)
	h.updateRecord(schedulerPlansResource, "plan"+h.runID, map[string]any{
		"gpu_limit":          4.0,
		"allowed_gpu_models": []string{deviceClassName},
	})

	suffix := e2eSuffix(h.runID)
	configID := "dra-cfg-" + suffix
	jobID := "dra-job-" + suffix
	podName := "dra-pod-" + suffix
	namespace := liveDeployNamespace(h.projectID(), ids.userID)
	t.Cleanup(func() {
		err := cl.Clientset().CoreV1().Namespaces().Delete(context.Background(), namespace, metav1.DeleteOptions{})
		if err != nil && !apierrors.IsNotFound(err) {
			t.Logf("cleanup namespace %s: %v", namespace, err)
		}
	})

	h.doJSONWithBearer(workloadService, http.MethodPost, "/api/v1/configfiles?project_id="+h.projectID(), map[string]any{
		"id":         configID,
		"name":       "dra-pod.yaml",
		"path":       "pods/dra-pod.yaml",
		"content":    "apiVersion: v1\nkind: Pod\nmetadata:\n  name: " + podName + "\n",
		"e2e_run_id": h.runID,
	}, ids.apiToken, http.StatusCreated)
	h.doJSONWithBearer(workloadService, http.MethodPost, "/api/v1/jobs", map[string]any{
		"job_id":              jobID,
		"config_id":           configID,
		"queue_name":          h.queueName(),
		"required_cpu":        0.1,
		"required_memory":     64,
		"required_gpu":        1,
		"gpu_count":           1,
		"sm_percentage":       50,
		"pinned_memory_limit": "8Gi",
		"device_class_name":   deviceClassName,
		"namespace":           namespace,
		"resources": []map[string]any{{
			"name":      podName,
			"kind":      "Pod",
			"json_data": liveDRAPodManifest(t, podName),
		}},
		"e2e_run_id": h.runID,
	}, ids.apiToken, http.StatusCreated)

	claimName := liveDRAClaimTemplateName(deviceClassName, 1, 50, "8Gi")
	waitForLiveDRADispatch(t, h, cl, liveDRAWaitTarget{
		namespace:       namespace,
		podName:         podName,
		claimName:       claimName,
		jobID:           jobID,
		deviceClassName: deviceClassName,
	})
}

func requireLiveDRASupport(t *testing.T, ctx context.Context, cl *cluster.Client) string {
	t.Helper()
	if _, err := cl.DynamicClient().Resource(liveDRAClaimTemplateGVR).Namespace("default").List(ctx, metav1.ListOptions{Limit: 1}); err != nil {
		if apierrors.IsNotFound(err) || apierrors.IsForbidden(err) {
			t.Skipf("live Kubernetes cluster does not expose %s: %v", liveDRAClaimTemplateGVR.String(), err)
		}
		t.Fatalf("list live DRA ResourceClaimTemplates: %v", err)
	}
	classes, err := cl.DynamicClient().Resource(liveDRADeviceClassGVR).List(ctx, metav1.ListOptions{Limit: 10})
	if err != nil {
		if apierrors.IsNotFound(err) || apierrors.IsForbidden(err) {
			t.Skipf("live Kubernetes cluster does not expose %s: %v", liveDRADeviceClassGVR.String(), err)
		}
		t.Fatalf("list live DRA DeviceClasses: %v", err)
	}
	if len(classes.Items) == 0 {
		t.Skip("live Kubernetes cluster exposes DRA APIs but no DeviceClass/resource driver is installed")
	}
	for i := range classes.Items {
		if classes.Items[i].GetName() == "gpu.nvidia.com" {
			return "gpu.nvidia.com"
		}
	}
	return classes.Items[0].GetName()
}

func liveDRAPodManifest(t *testing.T, name string) string {
	t.Helper()
	manifest := map[string]any{
		"apiVersion": "v1",
		"kind":       "Pod",
		"metadata": map[string]any{
			"name": name,
		},
		"spec": map[string]any{
			"restartPolicy": "Never",
			"containers": []map[string]any{{
				"name":  "main",
				"image": "registry.k8s.io/pause:3.9",
				"resources": map[string]any{
					"requests": map[string]any{"cpu": "10m", "memory": "16Mi", "nvidia.com/gpu": "1"},
					"limits":   map[string]any{"nvidia.com/gpu": "1"},
				},
			}},
		},
	}
	raw, err := json.Marshal(manifest)
	if err != nil {
		t.Fatalf("marshal DRA pod manifest: %v", err)
	}
	return string(raw)
}

type liveDRAWaitTarget struct {
	namespace       string
	podName         string
	claimName       string
	jobID           string
	deviceClassName string
}

func waitForLiveDRADispatch(t *testing.T, h *e2eHarness, cl *cluster.Client, target liveDRAWaitTarget) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for {
		h.services[workloadService].app.RunMaintenanceOnce(h.ctx, 200*time.Millisecond)
		result := readLiveDRADispatchState(t, h, cl, target)
		if result.ready() {
			assertLiveDRADispatchState(t, target, result)
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("DRA job %s did not dispatch before deadline; claimErr=%v podErr=%v record=%#v", target.jobID, result.claimErr, result.podErr, result.record.Data)
		}
		time.Sleep(250 * time.Millisecond)
	}
}

type liveDRADispatchState struct {
	claim    *unstructured.Unstructured
	pod      *corev1.Pod
	record   contracts.Record[map[string]any]
	found    bool
	claimErr error
	podErr   error
}

func (s liveDRADispatchState) ready() bool {
	return s.claimErr == nil && s.podErr == nil && s.found && textE2E(s.record.Data["status"]) == "running"
}

func readLiveDRADispatchState(t *testing.T, h *e2eHarness, cl *cluster.Client, target liveDRAWaitTarget) liveDRADispatchState {
	t.Helper()
	claim, claimErr := cl.DynamicClient().Resource(liveDRAClaimTemplateGVR).Namespace(target.namespace).Get(h.ctx, target.claimName, metav1.GetOptions{})
	pod, podErr := cl.Clientset().CoreV1().Pods(target.namespace).Get(h.ctx, target.podName, metav1.GetOptions{})
	record, found := h.store.Get(h.ctx, workloadJobsResource, target.jobID)
	if claimErr != nil && !apierrors.IsNotFound(claimErr) {
		t.Fatalf("get live DRA ResourceClaimTemplate %s/%s: %v", target.namespace, target.claimName, claimErr)
	}
	if podErr != nil && !apierrors.IsNotFound(podErr) {
		t.Fatalf("get live DRA Pod %s/%s: %v", target.namespace, target.podName, podErr)
	}
	return liveDRADispatchState{claim: claim, pod: pod, record: record, found: found, claimErr: claimErr, podErr: podErr}
}

func assertLiveDRADispatchState(t *testing.T, target liveDRAWaitTarget, state liveDRADispatchState) {
	t.Helper()
	assertLiveDRAClaimTemplate(t, state.claim, target.claimName, target.deviceClassName)
	assertLiveDRAPod(t, state.pod, target.claimName)
	resources := recordListE2E(state.record.Data["created_resources"])
	if !createdResourcesContain(resources, "ResourceClaimTemplate", target.namespace, target.claimName) ||
		!createdResourcesContain(resources, "Pod", target.namespace, target.podName) {
		t.Fatalf("DRA created_resources = %#v, want ResourceClaimTemplate and Pod", state.record.Data["created_resources"])
	}
}

func assertLiveDRAClaimTemplate(t *testing.T, claim *unstructured.Unstructured, claimName, deviceClassName string) {
	t.Helper()
	labels := claim.GetLabels()
	if labels["platform-go/dra-gpu-count"] != "1" ||
		labels["platform-go/dra-sm-pct"] != "50" ||
		labels["platform-go/dra-effective-gpu"] != "0.5" ||
		labels["platform-go/dra-pinned-mem"] != "8Gi" ||
		labels["platform-go/dra-claim-name"] != claimName {
		t.Fatalf("DRA claim labels = %#v, want GPU count/sm/effective/pinned/claim metadata", labels)
	}
	requests, found, _ := unstructured.NestedSlice(claim.Object, "spec", "spec", "devices", "requests")
	if !found || len(requests) != 1 {
		t.Fatalf("DRA claim requests = %#v, want one request", requests)
	}
	request, _ := requests[0].(map[string]any)
	exactly, _ := request["exactly"].(map[string]any)
	if textE2E(exactly["deviceClassName"]) != deviceClassName || liveIntValue(exactly["count"]) != 1 {
		t.Fatalf("DRA claim exactly = %#v, want deviceClassName=%s count=1", exactly, deviceClassName)
	}
	configs, _, _ := unstructured.NestedSlice(claim.Object, "spec", "spec", "devices", "config")
	if len(configs) != 1 {
		t.Fatalf("DRA claim config = %#v, want one MPS config", configs)
	}
}

func assertLiveDRAPod(t *testing.T, pod *corev1.Pod, claimName string) {
	t.Helper()
	if len(pod.Spec.ResourceClaims) != 1 ||
		pod.Spec.ResourceClaims[0].ResourceClaimTemplateName == nil ||
		*pod.Spec.ResourceClaims[0].ResourceClaimTemplateName != claimName {
		t.Fatalf("pod resourceClaims = %#v, want ResourceClaimTemplate %s", pod.Spec.ResourceClaims, claimName)
	}
	if len(pod.Spec.Containers) != 1 || len(pod.Spec.Containers[0].Resources.Claims) != 1 || pod.Spec.Containers[0].Resources.Claims[0].Name != "gpu" {
		t.Fatalf("pod container resource claims = %#v, want gpu claim", pod.Spec.Containers)
	}
	if _, ok := pod.Spec.Containers[0].Resources.Requests[corev1.ResourceName("nvidia.com/gpu")]; ok {
		t.Fatalf("pod requests still contain nvidia.com/gpu: %#v", pod.Spec.Containers[0].Resources.Requests)
	}
	if _, ok := pod.Spec.Containers[0].Resources.Limits[corev1.ResourceName("nvidia.com/gpu")]; ok {
		t.Fatalf("pod limits still contain nvidia.com/gpu: %#v", pod.Spec.Containers[0].Resources.Limits)
	}
	if pod.Labels["platform-go/dra-gpu-count"] != "1" ||
		pod.Labels["platform-go/dra-sm-pct"] != "50" ||
		pod.Labels["platform-go/dra-claim-name"] != claimName {
		t.Fatalf("pod DRA labels = %#v, want GPU DRA metadata", pod.Labels)
	}
	if got := pod.Annotations["volcano.sh/resource-request"]; !strings.Contains(got, ":1") {
		t.Fatalf("pod DRA resource annotation = %q, want resource request count", got)
	}
	if pod.Spec.SchedulerName != "default-scheduler" {
		t.Fatalf("pod schedulerName = %q, want default-scheduler for DRA pod", pod.Spec.SchedulerName)
	}
}

func liveDRAClaimTemplateName(deviceClassName string, count, sm int, pinnedMem string) string {
	base := "gpu-shared"
	if deviceClassName != "" && deviceClassName != "gpu.nvidia.com" {
		base = "gpu-" + strings.Split(deviceClassName, ".")[0]
	}
	mem := strings.ToLower(pinnedMem)
	if mem == "" {
		mem = "max"
	}
	return strings.ToLower(fmt.Sprintf("%s-cnt%d-sm%d-mem%s", base, count, sm, mem))
}

func liveIntValue(value any) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case int32:
		return int(typed)
	case float64:
		return int(typed)
	case string:
		parsed, _ := strconv.Atoi(typed)
		return parsed
	default:
		return 0
	}
}
