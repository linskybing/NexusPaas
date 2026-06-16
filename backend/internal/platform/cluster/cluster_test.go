package cluster

import (
	"context"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/fake"
)

func newFakeClient(objects ...runtime.Object) *Client {
	return New(fake.NewSimpleClientset(objects...), "proj")
}

func TestListProjectNamespacesMatchesPrefix(t *testing.T) {
	c := newFakeClient(
		ns("proj-p1-alice"),
		ns("proj-p1-bob"),
		ns("proj-p2-carol"),
		ns("kube-system"),
	)
	got, err := c.ListProjectNamespaces(context.Background(), "p1")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("namespaces = %v, want 2 for project p1", got)
	}
}

func TestListPodsByLabelAndDelete(t *testing.T) {
	c := newFakeClient(
		pod("proj-p1-alice", "job-pod", map[string]string{LabelJobID: "j1"}),
		pod("proj-p1-alice", "other-pod", map[string]string{"app": "x"}),
	)
	ctx := context.Background()
	pods, err := c.ListPodsByLabel(ctx, "proj-p1-alice", LabelJobID)
	if err != nil {
		t.Fatal(err)
	}
	if len(pods) != 1 || pods[0].Name != "job-pod" {
		t.Fatalf("pods = %+v, want only job-pod", pods)
	}
	if err := c.DeletePod(ctx, "proj-p1-alice", "job-pod"); err != nil {
		t.Fatal(err)
	}
	// Deleting an absent pod is a no-op (NotFound swallowed).
	if err := c.DeletePod(ctx, "proj-p1-alice", "job-pod"); err != nil {
		t.Fatalf("second delete should be no-op, got %v", err)
	}
}

func TestEnsureResourceQuotaCreatesThenUpdates(t *testing.T) {
	c := newFakeClient()
	ctx := context.Background()
	hard := BuildQuotaResources(0, 4, 8, 20)
	if err := c.EnsureResourceQuota(ctx, "proj-p1-alice", "proj-p1-alice-quota", hard); err != nil {
		t.Fatal(err)
	}
	got, err := c.Clientset().CoreV1().ResourceQuotas("proj-p1-alice").Get(ctx, "proj-p1-alice-quota", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if got.Spec.Hard.Cpu().String() != "4" {
		t.Fatalf("cpu hard = %s, want 4", got.Spec.Hard.Cpu())
	}
	// Second call updates in place rather than failing on AlreadyExists.
	if err := c.EnsureResourceQuota(ctx, "proj-p1-alice", "proj-p1-alice-quota", BuildQuotaResources(0, 2, 4, 10)); err != nil {
		t.Fatal(err)
	}
	got, _ = c.Clientset().CoreV1().ResourceQuotas("proj-p1-alice").Get(ctx, "proj-p1-alice-quota", metav1.GetOptions{})
	if got.Spec.Hard.Cpu().String() != "2" {
		t.Fatalf("cpu hard after update = %s, want 2", got.Spec.Hard.Cpu())
	}
}

func TestCollectNodeSummaryAggregatesActivePodRequests(t *testing.T) {
	c := newFakeClient(
		node("gpu-node", "8", "32Gi", "4"),
		node("cpu-node", "4", "16Gi", "0"),
		requestedPod("proj-p1-alice", "running-a", "gpu-node", corev1.PodRunning, "1500m", "2Gi", "1"),
		requestedPod("proj-p1-alice", "running-b", "gpu-node", corev1.PodRunning, "500m", "512Mi", ""),
		requestedPod("proj-p1-alice", "done", "gpu-node", corev1.PodSucceeded, "4", "8Gi", "2"),
		requestedPod("proj-p1-alice", "unscheduled", "", corev1.PodRunning, "1", "1Gi", ""),
	)

	summary, err := c.CollectNodeSummary(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if summary.NodeCount != 2 || summary.TotalCPUAllocatableMilli != 12000 || summary.TotalGPUAllocatable != 4 {
		t.Fatalf("summary capacity = %+v, want two nodes with cpu/gpu totals", summary)
	}
	if summary.TotalCPUUsedMilli != 2000 || summary.TotalGPUUsed != 1 {
		t.Fatalf("summary usage = %+v, want active scheduled pod requests only", summary)
	}
	gpuNode := findNodeResource(summary.Nodes, "gpu-node")
	if gpuNode == nil || gpuNode.CPUUsedMilli != 2000 || gpuNode.MemoryUsedBytes != 2684354560 {
		t.Fatalf("nodes = %+v, want gpu-node usage totals", summary.Nodes)
	}
}

func TestRuntimeLimitedResourcesListAndDelete(t *testing.T) {
	created := metav1.NewTime(metav1.Now().Add(-time.Hour))
	labels := map[string]string{RuntimeLimitSecondsKey: "60", LabelProjectID: "p1"}
	p := pod("proj-p1-alice", "runtime-pod", labels)
	p.CreationTimestamp = created
	dep := deployment("proj-p1-alice", "runtime-dep", labels)
	dep.CreationTimestamp = created
	job := batchJob("proj-p1-alice", "runtime-job", labels)
	job.CreationTimestamp = created
	c := newFakeClient(p, dep, job)
	ctx := context.Background()

	resources, err := c.ListRuntimeLimited(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(resources) != 3 || resources[0].CreatedAt.IsZero() {
		t.Fatalf("runtime resources = %+v, want pod/deployment/job with creation time", resources)
	}
	for _, resource := range resources {
		if err := c.DeleteResource(ctx, resource.Kind, resource.Namespace, resource.Name); err != nil {
			t.Fatalf("delete %s failed: %v", resource.Kind, err)
		}
	}
	if err := c.DeleteResource(ctx, "Unknown", "proj-p1-alice", "x"); err == nil {
		t.Fatal("unsupported runtime kind returned nil error")
	}
	remaining, err := c.ListRuntimeLimited(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(remaining) != 0 {
		t.Fatalf("remaining runtime resources = %+v, want none", remaining)
	}
}

func TestListJobPodResourceUsageExtractsBillableFields(t *testing.T) {
	now := time.Date(2026, 6, 14, 12, 0, 0, 0, time.UTC)
	runningAt := metav1.NewTime(now.Add(-30 * time.Minute))
	finishedAt := metav1.NewTime(now.Add(-5 * time.Minute))
	c := newFakeClient(
		resourceUsagePod("proj-p1-alice", "running", "uid-running", corev1.PodRunning, &runningAt, nil, map[string]string{
			LabelJobID: "j1", LabelProjectID: "p1", LabelUserID: "u1", LabelDRAEffectiveGPU: "0.5",
		}),
		resourceUsagePod("proj-p1-alice", "done", "uid-done", corev1.PodSucceeded, &runningAt, &finishedAt, map[string]string{
			LabelJobID: "j2", LabelProjectID: "p1", LabelUserID: "u2",
		}),
		resourceUsagePod("proj-p1-alice", "ignored", "uid-ignored", corev1.PodRunning, &runningAt, nil, map[string]string{"app": "no-job"}),
	)

	usages, err := c.ListJobPodResourceUsage(context.Background(), now)
	if err != nil {
		t.Fatal(err)
	}
	if len(usages) != 2 {
		t.Fatalf("usages = %+v, want two job-labeled pods", usages)
	}
	first := findPodUsage(usages, "j1")
	if first == nil {
		t.Fatalf("usages = %+v, missing j1", usages)
	}
	if first.JobID != "j1" || first.RequestedGPU != 0.5 || first.RequestedCPU != 2 || first.RequestedMemoryMB != 4096 {
		t.Fatalf("first usage = %+v, want DRA GPU and requested CPU/memory", *first)
	}
	second := findPodUsage(usages, "j2")
	if second == nil {
		t.Fatalf("usages = %+v, missing j2", usages)
	}
	if second.IsActive || second.TerminatedAt == nil || !second.TerminatedAt.Equal(finishedAt.Time) {
		t.Fatalf("terminal usage = %+v, want inactive with finished time", *second)
	}
}

func TestCleanupJobResourcesDeletesByJobLabel(t *testing.T) {
	c := newFakeClient(
		pod("proj-p1-alice", "p-1", map[string]string{LabelJobID: "j1"}),
		pod("proj-p1-alice", "p-2", map[string]string{LabelJobID: "j1"}),
		pod("proj-p1-alice", "p-other", map[string]string{LabelJobID: "j2"}),
		deployment("proj-p1-alice", "dep-1", map[string]string{LabelJobID: "j1"}),
		statefulSet("proj-p1-alice", "sts-1", map[string]string{LabelJobID: "j1"}),
		service("proj-p1-alice", "svc-1", map[string]string{LabelJobID: "j1"}),
		service("proj-p1-alice", "kubernetes", map[string]string{LabelJobID: "j1"}),
		batchJob("proj-p1-alice", "job-1", map[string]string{LabelJobID: "j1"}),
		configMap("proj-p1-alice", "cm-1", map[string]string{LabelJobID: "j1"}),
		secret("proj-p1-alice", "secret-1", map[string]string{LabelJobID: "j1"}),
		ingress("proj-p1-alice", "ing-1", map[string]string{LabelJobID: "j1"}),
	)
	res, err := c.CleanupJobResources(context.Background(), "proj-p1-alice", "j1")
	if err != nil {
		t.Fatal(err)
	}
	if res.Pods != 2 || res.Deployments != 1 || res.StatefulSets != 1 || res.Services != 1 ||
		res.Jobs != 1 || res.ConfigMaps != 1 || res.Secrets != 1 || res.Ingresses != 1 {
		t.Fatalf("cleanup result = %+v, want all labeled resources deleted once", res)
	}
	if res.Total() != 9 {
		t.Fatalf("cleanup total = %d, want 9", res.Total())
	}
	remaining, _ := c.ListPodsByLabel(context.Background(), "proj-p1-alice", "")
	if len(remaining) != 1 || remaining[0].Name != "p-other" {
		t.Fatalf("remaining = %+v, want only p-other", remaining)
	}
	if _, err := c.Clientset().CoreV1().Services("proj-p1-alice").Get(context.Background(), "kubernetes", metav1.GetOptions{}); err != nil {
		t.Fatalf("default kubernetes service should be retained: %v", err)
	}
}

// --- helpers ---

func ns(name string) *corev1.Namespace {
	return &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: name}}
}

func node(name, cpu, memory, gpu string) *corev1.Node {
	allocatable := corev1.ResourceList{
		corev1.ResourceCPU:    resource.MustParse(cpu),
		corev1.ResourceMemory: resource.MustParse(memory),
	}
	if gpu != "" {
		allocatable[corev1.ResourceName(gpuResourceName)] = resource.MustParse(gpu)
	}
	return &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: name}, Status: corev1.NodeStatus{Allocatable: allocatable}}
}

func pod(namespace, name string, labels map[string]string) *corev1.Pod {
	return &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: name, Labels: labels}}
}

func requestedPod(namespace, name, nodeName string, phase corev1.PodPhase, cpu, memory, gpu string) *corev1.Pod {
	requests := corev1.ResourceList{
		corev1.ResourceCPU:    resource.MustParse(cpu),
		corev1.ResourceMemory: resource.MustParse(memory),
	}
	if gpu != "" {
		requests[corev1.ResourceName(gpuResourceName)] = resource.MustParse(gpu)
	}
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: name},
		Spec: corev1.PodSpec{
			NodeName: nodeName,
			Containers: []corev1.Container{{
				Name:      "main",
				Resources: corev1.ResourceRequirements{Requests: requests},
			}},
		},
		Status: corev1.PodStatus{Phase: phase},
	}
}

func resourceUsagePod(namespace, name, uid string, phase corev1.PodPhase, runningAt, finishedAt *metav1.Time, labels map[string]string) *corev1.Pod {
	state := corev1.ContainerState{}
	if finishedAt != nil {
		state.Terminated = &corev1.ContainerStateTerminated{StartedAt: *runningAt, FinishedAt: *finishedAt}
	} else if runningAt != nil {
		state.Running = &corev1.ContainerStateRunning{StartedAt: *runningAt}
	}
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      name,
			UID:       types.UID(uid),
			Labels:    labels,
		},
		Spec: corev1.PodSpec{Containers: []corev1.Container{{
			Name: "main",
			Resources: corev1.ResourceRequirements{Requests: corev1.ResourceList{
				corev1.ResourceCPU:                   resource.MustParse("2"),
				corev1.ResourceMemory:                resource.MustParse("4Gi"),
				corev1.ResourceName(gpuResourceName): resource.MustParse("1"),
			}},
		}}},
		Status: corev1.PodStatus{
			Phase: phase,
			ContainerStatuses: []corev1.ContainerStatus{{
				Name:  "main",
				State: state,
			}},
		},
	}
}

func findNodeResource(nodes []NodeResource, name string) *NodeResource {
	for i := range nodes {
		if nodes[i].Name == name {
			return &nodes[i]
		}
	}
	return nil
}

func findPodUsage(usages []PodResourceUsage, jobID string) *PodResourceUsage {
	for i := range usages {
		if usages[i].JobID == jobID {
			return &usages[i]
		}
	}
	return nil
}

func deployment(namespace, name string, labels map[string]string) *appsv1.Deployment {
	return &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: name, Labels: labels}}
}

func statefulSet(namespace, name string, labels map[string]string) *appsv1.StatefulSet {
	return &appsv1.StatefulSet{ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: name, Labels: labels}}
}

func service(namespace, name string, labels map[string]string) *corev1.Service {
	return &corev1.Service{ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: name, Labels: labels}}
}

func batchJob(namespace, name string, labels map[string]string) *batchv1.Job {
	return &batchv1.Job{ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: name, Labels: labels}}
}

func configMap(namespace, name string, labels map[string]string) *corev1.ConfigMap {
	return &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: name, Labels: labels}}
}

func secret(namespace, name string, labels map[string]string) *corev1.Secret {
	return &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: name, Labels: labels}}
}

func ingress(namespace, name string, labels map[string]string) *networkingv1.Ingress {
	return &networkingv1.Ingress{ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: name, Labels: labels}}
}
