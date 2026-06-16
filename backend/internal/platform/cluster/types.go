// Package cluster is the backend's real Kubernetes integration layer. It ports the
// cluster operations the reference backend exposes through pkg/k8s (namespace
// listing, pod listing/deletion, per-job resource cleanup, ResourceQuota
// reconciliation) onto an injectable client-go kubernetes.Interface, so it is
// exercised in tests with k8s.io/client-go/kubernetes/fake and runs against a real
// cluster in production. Unlike the reference's package-level globals, the client
// is dependency-injected (no globals), matching the backend's clean architecture.
package cluster

// Label keys platform workloads carry, mirrored from the reference backend so the
// same selectors match the same objects.
const (
	LabelJobID               = "platform-go/job-id"
	LabelProjectID           = "platform-go/project-id"
	LabelUserID              = "platform-go/user-id"
	LabelGPUCount            = "platform-go/gpu-count"
	RuntimeLimitSecondsKey   = "platform-go/runtime-limit-seconds"
	DefaultProjectNamespaceP = "proj"
)

// ContainerStatusInfo holds a container's runtime status.
type ContainerStatusInfo struct {
	Name         string `json:"name"`
	Ready        bool   `json:"ready"`
	RestartCount int32  `json:"restart_count"`
	State        string `json:"state"`
}

// PodInfo holds essential pod metadata and runtime status, mirroring the reference
// pkg/k8s.PodInfo shape so ported worker logic reads the same fields.
type PodInfo struct {
	Name        string                `json:"name"`
	Namespace   string                `json:"namespace"`
	Phase       string                `json:"phase"`
	PodIP       string                `json:"pod_ip"`
	NodeName    string                `json:"node_name"`
	StartTime   string                `json:"start_time,omitempty"`
	Containers  []ContainerStatusInfo `json:"containers,omitempty"`
	Annotations map[string]string     `json:"-"`
	Labels      map[string]string     `json:"labels,omitempty"`
}

// CleanupResult tracks how many resources were deleted per kind during a per-job
// cleanup, mirroring the reference CleanupResult.
type CleanupResult struct {
	Pods         int `json:"pods"`
	Deployments  int `json:"deployments"`
	StatefulSets int `json:"statefulsets"`
	Services     int `json:"services"`
	Jobs         int `json:"jobs"`
	VCJobs       int `json:"vcjobs"`
	PodGroups    int `json:"podgroups"`
	ConfigMaps   int `json:"configmaps"`
	Secrets      int `json:"secrets"`
	Ingresses    int `json:"ingresses"`
}

// Total returns the sum of all deleted resources.
func (r CleanupResult) Total() int {
	return r.Pods + r.Deployments + r.StatefulSets + r.Services + r.Jobs + r.VCJobs + r.PodGroups +
		r.ConfigMaps + r.Secrets + r.Ingresses
}
