package cluster

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

func TestLonghornRWXReconcileSummarizesHealthyAndUnhealthyVolumes(t *testing.T) {
	ctx := context.Background()
	namespace := "longhorn-system"
	dynamicClient := fakeLonghornDynamicClient(
		longhornVolumeObject(namespace, "vol-healthy", "rwx", "healthy", nil),
		longhornVolumeObject(namespace, "vol-unhealthy", "RWX", "faulted", nil),
		longhornVolumeObject(namespace, "vol-rwo", "rwo", "healthy", nil),
		longhornSnapshotObject(namespace, "snap-a", "vol-unhealthy"),
		longhornSnapshotObject(namespace, "snap-b", "vol-unhealthy"),
	)
	client := NewWithDynamic(k8sfake.NewSimpleClientset(
		healthyShareManagerService(namespace, "vol-healthy"),
		healthyShareManagerEndpoints(namespace, "vol-healthy"),
	), dynamicClient, "proj")

	summary := client.ReconcileLonghornRWXVolumes(ctx, LonghornRWXOptions{
		Namespace:          namespace,
		SnapshotWarnLimit:  2,
		SnapshotBlockLimit: 3,
	})

	if summary.Degraded {
		t.Fatalf("summary degraded = true, want false: %#v", summary)
	}
	if summary.Checked != 2 {
		t.Fatalf("checked = %d, want 2 RWX volumes only", summary.Checked)
	}
	if summary.Unavailable != 1 || summary.Unhealthy != 1 || summary.EndpointUnavailable != 1 {
		t.Fatalf("summary counts = %#v, want one unavailable/unhealthy/endpoint-unavailable", summary)
	}
	if summary.SnapshotWarning != 1 || summary.SnapshotBlocked != 0 {
		t.Fatalf("snapshot counts = warn:%d block:%d, want 1/0", summary.SnapshotWarning, summary.SnapshotBlocked)
	}
	healthy := findLonghornResult(t, summary, "vol-healthy")
	if !healthy.Available || !healthy.EndpointReady || healthy.Robustness != "healthy" {
		t.Fatalf("healthy result = %#v, want available healthy endpoint", healthy)
	}
	unhealthy := findLonghornResult(t, summary, "vol-unhealthy")
	if unhealthy.Available || unhealthy.Skipped != "auto_repair_disabled" || !unhealthy.SnapshotWarning {
		t.Fatalf("unhealthy result = %#v, want unavailable with repair disabled and snapshot warning", unhealthy)
	}
}

func TestLonghornRWXReconcileDegradesWhenDynamicClientUnavailable(t *testing.T) {
	summary := New(k8sfake.NewSimpleClientset(), "proj").ReconcileLonghornRWXVolumes(context.Background(), LonghornRWXOptions{})

	if !summary.Degraded || summary.Failed != 1 {
		t.Fatalf("summary = %#v, want degraded failure", summary)
	}
	if summary.Error != "dynamic client unavailable" {
		t.Fatalf("error = %q, want dynamic client unavailable", summary.Error)
	}
}

func TestLonghornRWXListErrorClassifiesCRDAndRBACFailures(t *testing.T) {
	notFound := longhornListError(apierrors.NewNotFound(schema.GroupResource{Group: "longhorn.io", Resource: "volumes"}, "volumes"))
	if !strings.Contains(notFound, "CRD unavailable") {
		t.Fatalf("not found error = %q, want CRD unavailable", notFound)
	}
	forbidden := longhornListError(apierrors.NewForbidden(schema.GroupResource{Group: "longhorn.io", Resource: "volumes"}, "volumes", errors.New("denied")))
	if !strings.Contains(forbidden, "access denied") {
		t.Fatalf("forbidden error = %q, want access denied", forbidden)
	}
	generic := longhornListError(errors.New("dial failed"))
	if !strings.Contains(generic, "list Longhorn volumes") {
		t.Fatalf("generic error = %q, want list Longhorn volumes", generic)
	}
}

func TestLonghornRWXDefaultOptionsNormalizeUnsetAndInvalidValues(t *testing.T) {
	opts := defaultLonghornRWXOptions(LonghornRWXOptions{
		SnapshotWarnLimit:  -1,
		SnapshotBlockLimit: 1,
	}, nil)

	if opts.Namespace != defaultLonghornNamespace {
		t.Fatalf("namespace = %q, want default", opts.Namespace)
	}
	if opts.RepairCooldown != 10*time.Minute {
		t.Fatalf("repair cooldown = %v, want 10m", opts.RepairCooldown)
	}
	if opts.SnapshotWarnLimit != defaultLonghornRWXSnapshotWarnLimit {
		t.Fatalf("warn limit = %d, want default", opts.SnapshotWarnLimit)
	}
	if opts.SnapshotBlockLimit != opts.SnapshotWarnLimit {
		t.Fatalf("block limit = %d, want promoted to warn %d", opts.SnapshotBlockLimit, opts.SnapshotWarnLimit)
	}
	if opts.Now == nil {
		t.Fatal("Now default was not installed")
	}
}

func TestLonghornRWXAutoRepairDeletesFailedShareManagerPodWhenSafe(t *testing.T) {
	ctx := context.Background()
	namespace := "longhorn-system"
	now := time.Date(2026, 6, 15, 10, 0, 0, 0, time.UTC)
	dynamicClient := fakeLonghornDynamicClient(longhornVolumeObject(namespace, "vol-repair", "rwx", "faulted", nil))
	client := NewWithDynamic(k8sfake.NewSimpleClientset(failedShareManagerPod(namespace, "vol-repair")), dynamicClient, "proj")

	summary := client.ReconcileLonghornRWXVolumes(ctx, LonghornRWXOptions{
		Namespace:          namespace,
		AutoRepairEnabled:  true,
		RepairCooldown:     time.Minute,
		SnapshotWarnLimit:  20,
		SnapshotBlockLimit: 50,
		Now:                func() time.Time { return now },
	})

	result := findLonghornResult(t, summary, "vol-repair")
	if !result.Repaired || result.RepairAction != "delete_failed_share_manager_pod" {
		t.Fatalf("repair result = %#v, want failed share-manager pod deletion", result)
	}
	if summary.RepairAttempted != 1 || summary.RepairSucceeded != 1 {
		t.Fatalf("repair counts = %#v, want attempted/succeeded", summary)
	}
	if _, err := client.Clientset().CoreV1().Pods(namespace).Get(ctx, "share-manager-vol-repair", metav1.GetOptions{}); !apierrors.IsNotFound(err) {
		t.Fatalf("failed share-manager pod still exists or unexpected error: %v", err)
	}
	volume, err := client.DynamicClient().Resource(longhornVolumeGVR).Namespace(namespace).Get(ctx, "vol-repair", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get repaired volume: %v", err)
	}
	if got := volume.GetAnnotations()[LonghornRWXRepairCooldownAnnotation]; got != now.Format(time.RFC3339) {
		t.Fatalf("repair annotation = %q, want %q", got, now.Format(time.RFC3339))
	}
}

func TestLonghornRWXAutoRepairGuardsSnapshotCooldownAndNoFailedPods(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	client := New(k8sfake.NewSimpleClientset(), "proj")

	blocked := LonghornRWXVolumeStatus{Volume: "vol-blocked", Namespace: "longhorn-system", SnapshotBlocked: true}
	client.maybeRepairLonghornRWXVolume(ctx, longhornVolumeObject("longhorn-system", "vol-blocked", "rwx", "faulted", nil), LonghornRWXOptions{
		AutoRepairEnabled: true,
		RepairCooldown:    time.Minute,
		Now:               func() time.Time { return now },
	}, &blocked)
	if blocked.Skipped != "snapshot_limit_guard" {
		t.Fatalf("snapshot guard result = %#v", blocked)
	}

	cooldownVolume := longhornVolumeObject("longhorn-system", "vol-cooldown", "rwx", "faulted", map[string]string{
		LonghornRWXRepairCooldownAnnotation: now.Add(-30 * time.Second).Format(time.RFC3339),
	})
	cooldown := LonghornRWXVolumeStatus{Volume: "vol-cooldown", Namespace: "longhorn-system"}
	client.maybeRepairLonghornRWXVolume(ctx, cooldownVolume, LonghornRWXOptions{
		AutoRepairEnabled: true,
		RepairCooldown:    time.Minute,
		Now:               func() time.Time { return now },
	}, &cooldown)
	if cooldown.Skipped != "cooldown" || !cooldown.InCooldown {
		t.Fatalf("cooldown result = %#v", cooldown)
	}

	noFailed := LonghornRWXVolumeStatus{Volume: "vol-empty", Namespace: "longhorn-system"}
	client.maybeRepairLonghornRWXVolume(ctx, longhornVolumeObject("longhorn-system", "vol-empty", "rwx", "faulted", nil), LonghornRWXOptions{
		AutoRepairEnabled: true,
		RepairCooldown:    time.Minute,
		Now:               func() time.Time { return now },
	}, &noFailed)
	if noFailed.Skipped != "no_failed_share_manager" {
		t.Fatalf("no failed pod result = %#v", noFailed)
	}
}

func TestLonghornRWXAutoRepairSkipsActiveConsumers(t *testing.T) {
	ctx := context.Background()
	namespace := "longhorn-system"
	dynamicClient := fakeLonghornDynamicClient(longhornVolumeObject(namespace, "vol-busy", "rwx", "faulted", nil))
	client := NewWithDynamic(k8sfake.NewSimpleClientset(
		failedShareManagerPod(namespace, "vol-busy"),
		&corev1.PersistentVolume{
			ObjectMeta: metav1.ObjectMeta{Name: "pv-busy"},
			Spec: corev1.PersistentVolumeSpec{
				PersistentVolumeSource: corev1.PersistentVolumeSource{CSI: &corev1.CSIPersistentVolumeSource{
					Driver:       csiDriverLonghorn,
					VolumeHandle: "vol-busy",
				}},
				ClaimRef: &corev1.ObjectReference{Namespace: "proj-p1-user", Name: "datasets"},
			},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "consumer", Namespace: "proj-p1-user"},
			Spec: corev1.PodSpec{Volumes: []corev1.Volume{{
				Name:         "data",
				VolumeSource: corev1.VolumeSource{PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: "datasets"}},
			}}},
			Status: corev1.PodStatus{Phase: corev1.PodRunning},
		},
	), dynamicClient, "proj")

	summary := client.ReconcileLonghornRWXVolumes(ctx, LonghornRWXOptions{
		Namespace:          namespace,
		AutoRepairEnabled:  true,
		RepairCooldown:     time.Minute,
		SnapshotWarnLimit:  20,
		SnapshotBlockLimit: 50,
	})

	result := findLonghornResult(t, summary, "vol-busy")
	if result.Repaired || result.Skipped != "active_consumers" || !result.ActiveConsumers {
		t.Fatalf("result = %#v, want active consumer guard", result)
	}
	if _, err := client.Clientset().CoreV1().Pods(namespace).Get(ctx, "share-manager-vol-busy", metav1.GetOptions{}); err != nil {
		t.Fatalf("failed share-manager pod should remain after active-consumer guard: %v", err)
	}
}

func TestLonghornShareManagerPodFailureClassifiers(t *testing.T) {
	cases := []struct {
		name string
		pod  corev1.Pod
		want bool
	}{
		{name: "failed phase", pod: corev1.Pod{Status: corev1.PodStatus{Phase: corev1.PodFailed}}, want: true},
		{name: "terminated container", pod: corev1.Pod{Status: corev1.PodStatus{ContainerStatuses: []corev1.ContainerStatus{{
			State: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{}},
		}}}}, want: true},
		{name: "crash waiting", pod: corev1.Pod{Status: corev1.PodStatus{ContainerStatuses: []corev1.ContainerStatus{{
			State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "CrashLoopBackOff"}},
		}}}}, want: true},
		{name: "benign waiting", pod: corev1.Pod{Status: corev1.PodStatus{ContainerStatuses: []corev1.ContainerStatus{{
			State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "PodInitializing"}},
		}}}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := longhornShareManagerPodFailed(tc.pod); got != tc.want {
				t.Fatalf("longhornShareManagerPodFailed() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestLonghornShareEndpointModeReportsClusterIPFailures(t *testing.T) {
	ctx := context.Background()
	namespace := "longhorn-system"
	client := New(k8sfake.NewSimpleClientset(&corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "share-manager-vol-no-ip",
			Namespace: namespace,
			Labels:    map[string]string{longhornShareManagerLabelKey: "vol-no-ip"},
		},
		Spec: corev1.ServiceSpec{ClusterIP: "None"},
	}), "proj")
	mode, err := client.longhornShareEndpointMode(ctx, namespace, "vol-no-ip")
	if mode != longhornEndpointModeClusterIP || err == nil || !strings.Contains(err.Error(), "no ClusterIP") {
		t.Fatalf("mode=%q err=%v, want ClusterIP failure", mode, err)
	}

	client = New(k8sfake.NewSimpleClientset(
		healthyShareManagerService(namespace, "vol-no-endpoint"),
		&corev1.Endpoints{ObjectMeta: metav1.ObjectMeta{Name: "share-manager-vol-no-endpoint", Namespace: namespace}},
	), "proj")
	mode, err = client.longhornShareEndpointMode(ctx, namespace, "vol-no-endpoint")
	if mode != longhornEndpointModeClusterIP || err == nil || !strings.Contains(err.Error(), "no ready NFS endpoint") {
		t.Fatalf("mode=%q err=%v, want no ready endpoint failure", mode, err)
	}
}

func fakeLonghornDynamicClient(objects ...runtime.Object) *dynamicfake.FakeDynamicClient {
	return dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
		runtime.NewScheme(),
		map[schema.GroupVersionResource]string{
			longhornVolumeGVR:   "VolumeList",
			longhornSnapshotGVR: "SnapshotList",
		},
		objects...,
	)
}

func longhornVolumeObject(namespace, name, accessMode, robustness string, annotations map[string]string) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "longhorn.io/v1beta2",
		"kind":       "Volume",
		"metadata": map[string]any{
			"name":      name,
			"namespace": namespace,
		},
		"spec": map[string]any{
			"accessMode": accessMode,
		},
		"status": map[string]any{
			"robustness": robustness,
		},
	}}
	obj.SetAnnotations(annotations)
	return obj
}

func longhornSnapshotObject(namespace, name, volume string) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "longhorn.io/v1beta2",
		"kind":       "Snapshot",
		"metadata": map[string]any{
			"name":      name,
			"namespace": namespace,
		},
		"spec": map[string]any{
			"volume": volume,
		},
	}}
}

func healthyShareManagerService(namespace, volume string) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "share-manager-" + volume,
			Namespace: namespace,
			Labels:    map[string]string{longhornShareManagerLabelKey: volume},
		},
		Spec: corev1.ServiceSpec{
			ClusterIP: "10.0.0.1",
			Ports:     []corev1.ServicePort{{Port: longhornShareManagerNFSPort}},
		},
	}
}

func healthyShareManagerEndpoints(namespace, volume string) *corev1.Endpoints {
	return &corev1.Endpoints{
		ObjectMeta: metav1.ObjectMeta{Name: "share-manager-" + volume, Namespace: namespace},
		Subsets: []corev1.EndpointSubset{{
			Addresses: []corev1.EndpointAddress{{IP: "10.0.0.2"}},
			Ports:     []corev1.EndpointPort{{Port: longhornShareManagerNFSPort}},
		}},
	}
}

func failedShareManagerPod(namespace, volume string) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "share-manager-" + volume,
			Namespace: namespace,
			Labels:    map[string]string{longhornShareManagerLabelKey: volume},
		},
		Status: corev1.PodStatus{Phase: corev1.PodFailed},
	}
}

func findLonghornResult(t *testing.T, summary LonghornRWXSummary, volume string) LonghornRWXVolumeStatus {
	t.Helper()
	for _, result := range summary.Results {
		if result.Volume == volume {
			return result
		}
	}
	t.Fatalf("missing result for %s in %#v", volume, summary.Results)
	return LonghornRWXVolumeStatus{}
}
