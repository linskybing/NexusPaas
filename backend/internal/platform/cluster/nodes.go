package cluster

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// gpuResourceName is the extended-resource key NVIDIA device plugins expose for
// whole-GPU allocation on non-DRA clusters. It is the legacy GPU source the
// reference falls back to when DRA ResourceSlices report nothing.
const gpuResourceName = "nvidia.com/gpu"

// NodeResource is the per-node capacity/usage snapshot the resource collector
// emits, mirroring the fields of reference domain.NodeResourceInfo that are
// derivable without DRA/Prometheus.
type NodeResource struct {
	Name                   string
	CPUAllocatableMilli    int64
	CPUUsedMilli           int64
	MemoryAllocatableBytes int64
	MemoryUsedBytes        int64
	GPUAllocatable         int64
	GPUUsed                int64
}

// NodeSummary aggregates per-node capacity/usage across the cluster, mirroring
// the totals of reference domain.ClusterSummary (excluding the DRA/Prometheus GPU
// device detail, which lands with the DRA adapter in a later phase).
type NodeSummary struct {
	Nodes                       []NodeResource
	NodeCount                   int
	TotalCPUAllocatableMilli    int64
	TotalCPUUsedMilli           int64
	TotalMemoryAllocatableBytes int64
	TotalMemoryUsedBytes        int64
	TotalGPUAllocatable         int64
	TotalGPUUsed                int64
}

// CollectNodeSummary lists cluster nodes and active pods and aggregates allocatable
// vs requested CPU (milli), memory (bytes) and whole-GPU counts per node and in
// total. It is the microservice port of the non-DRA/non-Prometheus core of the
// reference application/cluster.CollectClusterResources.
//
// DRA ResourceSlice GPU inventory and Prometheus GPU utilisation are intentionally
// out of scope here; whole-GPU allocatable falls back to the nvidia.com/gpu
// extended resource exactly as the reference does for non-DRA clusters. A nil
// clientset (degraded mode) yields a zero-valued summary, not an error.
func (c *Client) CollectNodeSummary(ctx context.Context) (NodeSummary, error) {
	if c == nil || c.clientset == nil {
		return NodeSummary{}, nil
	}
	nodes, err := c.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return NodeSummary{}, fmt.Errorf("list nodes: %w", err)
	}
	byNode, summary := summarizeNodeCapacity(nodes.Items)
	if err := c.addPodUsage(ctx, byNode, &summary); err != nil {
		return NodeSummary{}, err
	}
	summary.Nodes = orderedNodeResources(nodes.Items, byNode)
	summary.NodeCount = len(summary.Nodes)
	return summary, nil
}

func summarizeNodeCapacity(nodes []corev1.Node) (map[string]*NodeResource, NodeSummary) {
	byNode := make(map[string]*NodeResource, len(nodes))
	var summary NodeSummary
	for i := range nodes {
		res := nodeResource(&nodes[i])
		byNode[res.Name] = res
		summary.TotalCPUAllocatableMilli += res.CPUAllocatableMilli
		summary.TotalMemoryAllocatableBytes += res.MemoryAllocatableBytes
		summary.TotalGPUAllocatable += res.GPUAllocatable
	}
	return byNode, summary
}

func nodeResource(node *corev1.Node) *NodeResource {
	return &NodeResource{
		Name:                   node.Name,
		CPUAllocatableMilli:    milliValue(node.Status.Allocatable, corev1.ResourceCPU),
		MemoryAllocatableBytes: rawValue(node.Status.Allocatable, corev1.ResourceMemory),
		GPUAllocatable:         rawValue(node.Status.Allocatable, gpuResourceName),
	}
}

func (c *Client) addPodUsage(ctx context.Context, byNode map[string]*NodeResource, summary *NodeSummary) error {
	pods, err := c.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("list pods: %w", err)
	}
	for i := range pods.Items {
		addSinglePodUsage(&pods.Items[i], byNode, summary)
	}
	return nil
}

func addSinglePodUsage(pod *corev1.Pod, byNode map[string]*NodeResource, summary *NodeSummary) {
	if pod.Spec.NodeName == "" || isTerminalPhase(pod.Status.Phase) {
		return
	}
	res := byNode[pod.Spec.NodeName]
	if res == nil {
		return
	}
	for i := range pod.Spec.Containers {
		addContainerRequests(res, summary, pod.Spec.Containers[i].Resources.Requests)
	}
}

func addContainerRequests(res *NodeResource, summary *NodeSummary, req corev1.ResourceList) {
	cpu := milliValue(req, corev1.ResourceCPU)
	mem := rawValue(req, corev1.ResourceMemory)
	gpu := rawValue(req, gpuResourceName)
	res.CPUUsedMilli += cpu
	res.MemoryUsedBytes += mem
	res.GPUUsed += gpu
	summary.TotalCPUUsedMilli += cpu
	summary.TotalMemoryUsedBytes += mem
	summary.TotalGPUUsed += gpu
}

func orderedNodeResources(nodes []corev1.Node, byNode map[string]*NodeResource) []NodeResource {
	out := make([]NodeResource, 0, len(byNode))
	for i := range nodes {
		if res := byNode[nodes[i].Name]; res != nil {
			out = append(out, *res)
		}
	}
	return out
}

func isTerminalPhase(phase corev1.PodPhase) bool {
	return phase == corev1.PodSucceeded || phase == corev1.PodFailed
}

// milliValue returns the resource quantity in milli-units (e.g. CPU cores → milli),
// mirroring reference quantityMilli. Missing keys yield 0.
func milliValue(list corev1.ResourceList, name corev1.ResourceName) int64 {
	if q, ok := list[name]; ok {
		return q.MilliValue()
	}
	return 0
}

// rawValue returns the resource quantity in base units (bytes for memory, whole
// devices for GPU), mirroring reference quantityValue. Missing keys yield 0.
func rawValue(list corev1.ResourceList, name corev1.ResourceName) int64 {
	if q, ok := list[name]; ok {
		return q.Value()
	}
	return 0
}
