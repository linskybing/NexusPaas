package cluster

import (
	"context"
	"fmt"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

const (
	LonghornRWXRepairCooldownAnnotation = "platform.nexuspaas.io/last-rwx-repair-at"
	LonghornRWXRepairReasonAnnotation   = "platform.nexuspaas.io/last-rwx-repair-reason"

	longhornEndpointModeClusterIP = "cluster-ip"

	defaultLonghornRWXSnapshotWarnLimit  = 20
	defaultLonghornRWXSnapshotBlockLimit = 50
)

var (
	longhornVolumeGVR = schema.GroupVersionResource{
		Group:    "longhorn.io",
		Version:  "v1beta2",
		Resource: "volumes",
	}
	longhornSnapshotGVR = schema.GroupVersionResource{
		Group:    "longhorn.io",
		Version:  "v1beta2",
		Resource: "snapshots",
	}
)

// LonghornRWXOptions controls the production-safe RWX health reconciler. This
// first slice deliberately supports only guarded failed share-manager restarts;
// destructive fsck/remote exec repair remains a separate feature.
type LonghornRWXOptions struct {
	Namespace          string
	AutoRepairEnabled  bool
	RepairCooldown     time.Duration
	SnapshotWarnLimit  int
	SnapshotBlockLimit int
	Now                func() time.Time
}

type LonghornRWXVolumeStatus struct {
	Volume          string `json:"volume"`
	Namespace       string `json:"namespace"`
	Robustness      string `json:"robustness"`
	EndpointMode    string `json:"endpoint_mode"`
	EndpointReady   bool   `json:"endpoint_ready"`
	Available       bool   `json:"available"`
	ActiveSnapshots int    `json:"active_snapshots"`
	SnapshotWarning bool   `json:"snapshot_warning"`
	SnapshotBlocked bool   `json:"snapshot_blocked"`
	ActiveConsumers bool   `json:"active_consumers"`
	InCooldown      bool   `json:"in_cooldown"`
	Repaired        bool   `json:"repaired"`
	RepairAction    string `json:"repair_action,omitempty"`
	Skipped         string `json:"skipped,omitempty"`
	Error           string `json:"error,omitempty"`
}

type LonghornRWXSummary struct {
	Namespace           string                    `json:"longhorn_namespace"`
	Checked             int                       `json:"volumes_checked"`
	Unavailable         int                       `json:"unavailable_count"`
	Unhealthy           int                       `json:"unhealthy_count"`
	EndpointUnavailable int                       `json:"endpoint_unavailable_count"`
	SnapshotWarning     int                       `json:"snapshot_warn_count"`
	SnapshotBlocked     int                       `json:"snapshot_block_count"`
	RepairAttempted     int                       `json:"repair_attempted_count"`
	RepairSucceeded     int                       `json:"repair_succeeded_count"`
	RepairSkipped       int                       `json:"repair_skipped_count"`
	Failed              int                       `json:"failed_count"`
	Degraded            bool                      `json:"degraded"`
	Error               string                    `json:"error,omitempty"`
	Results             []LonghornRWXVolumeStatus `json:"results"`
}

func (c *Client) ReconcileLonghornRWXVolumes(ctx context.Context, opts LonghornRWXOptions) LonghornRWXSummary {
	opts = defaultLonghornRWXOptions(opts, c)
	summary := LonghornRWXSummary{Namespace: opts.Namespace}
	if c == nil || c.clientset == nil {
		return degradedLonghornSummary(summary, "cluster client unavailable")
	}
	if c.dynamicClient == nil {
		return degradedLonghornSummary(summary, "dynamic client unavailable")
	}
	volumes, err := c.dynamicClient.Resource(longhornVolumeGVR).Namespace(opts.Namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return degradedLonghornSummary(summary, longhornListError(err))
	}
	for i := range volumes.Items {
		vol := &volumes.Items[i]
		if !strings.EqualFold(longhornAccessMode(vol), "rwx") {
			continue
		}
		result := c.reconcileLonghornRWXVolume(ctx, vol, opts)
		summary.Checked++
		accumulateLonghornResult(&summary, result)
		summary.Results = append(summary.Results, result)
	}
	return summary
}

func defaultLonghornRWXOptions(opts LonghornRWXOptions, c *Client) LonghornRWXOptions {
	opts.Namespace = strings.TrimSpace(opts.Namespace)
	if opts.Namespace == "" && c != nil {
		opts.Namespace = strings.TrimSpace(c.shareConfig.LonghornNamespace)
	}
	if opts.Namespace == "" {
		opts.Namespace = defaultLonghornNamespace
	}
	if opts.RepairCooldown <= 0 {
		opts.RepairCooldown = 10 * time.Minute
	}
	if opts.SnapshotWarnLimit < 0 {
		opts.SnapshotWarnLimit = defaultLonghornRWXSnapshotWarnLimit
	}
	if opts.SnapshotBlockLimit <= 0 {
		opts.SnapshotBlockLimit = defaultLonghornRWXSnapshotBlockLimit
	}
	if opts.SnapshotBlockLimit < opts.SnapshotWarnLimit {
		opts.SnapshotBlockLimit = opts.SnapshotWarnLimit
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}
	return opts
}

func degradedLonghornSummary(summary LonghornRWXSummary, reason string) LonghornRWXSummary {
	summary.Degraded = true
	summary.Error = reason
	summary.Failed = 1
	summary.Results = append(summary.Results, LonghornRWXVolumeStatus{Namespace: summary.Namespace, Error: reason})
	return summary
}

func longhornListError(err error) string {
	switch {
	case apierrors.IsNotFound(err):
		return "longhorn volumes CRD unavailable: " + err.Error()
	case apierrors.IsForbidden(err), apierrors.IsUnauthorized(err):
		return "longhorn volumes access denied: " + err.Error()
	default:
		return "list Longhorn volumes: " + err.Error()
	}
}

func (c *Client) reconcileLonghornRWXVolume(ctx context.Context, vol *unstructured.Unstructured, opts LonghornRWXOptions) LonghornRWXVolumeStatus {
	name := vol.GetName()
	result := LonghornRWXVolumeStatus{
		Volume:     name,
		Namespace:  opts.Namespace,
		Robustness: longhornRobustness(vol),
	}
	if active, err := c.countLonghornActiveSnapshots(ctx, opts.Namespace, name); err != nil {
		result.Error = "count active snapshots: " + err.Error()
		return result
	} else {
		result.ActiveSnapshots = active
		result.SnapshotWarning = active >= opts.SnapshotWarnLimit && opts.SnapshotWarnLimit > 0
		result.SnapshotBlocked = active >= opts.SnapshotBlockLimit && opts.SnapshotBlockLimit > 0
	}
	endpointMode, err := c.longhornShareEndpointMode(ctx, opts.Namespace, name)
	result.EndpointMode = endpointMode
	result.EndpointReady = err == nil
	if err != nil {
		result.Error = err.Error()
	}
	result.Available = result.EndpointReady && result.Robustness == "healthy"
	if result.Available {
		return result
	}
	c.maybeRepairLonghornRWXVolume(ctx, vol, opts, &result)
	return result
}

func (c *Client) maybeRepairLonghornRWXVolume(ctx context.Context, vol *unstructured.Unstructured, opts LonghornRWXOptions, result *LonghornRWXVolumeStatus) {
	if !opts.AutoRepairEnabled {
		result.Skipped = "auto_repair_disabled"
		return
	}
	if result.SnapshotBlocked {
		result.Skipped = "snapshot_limit_guard"
		return
	}
	if longhornRepairInCooldown(vol, opts.RepairCooldown, opts.Now) {
		result.InCooldown = true
		result.Skipped = "cooldown"
		return
	}
	active, err := c.longhornVolumeHasActiveConsumers(ctx, result.Volume)
	if err != nil {
		result.Error = err.Error()
		return
	}
	result.ActiveConsumers = active
	if active {
		result.Skipped = "active_consumers"
		return
	}
	pods, err := c.longhornShareManagerPods(ctx, result.Namespace, result.Volume)
	if err != nil {
		result.Error = err.Error()
		return
	}
	failed := failedLonghornShareManagerPods(pods)
	if len(failed) == 0 {
		result.Skipped = "no_failed_share_manager"
		return
	}
	result.RepairAction = "delete_failed_share_manager_pod"
	if err := c.annotateLonghornRWXRepair(ctx, result.Namespace, result.Volume, "failed-share-manager-restart", opts.Now); err != nil {
		result.Error = err.Error()
		return
	}
	for _, pod := range failed {
		err := c.clientset.CoreV1().Pods(result.Namespace).Delete(ctx, pod.Name, metav1.DeleteOptions{})
		if err != nil && !apierrors.IsNotFound(err) {
			result.Error = fmt.Sprintf("delete failed share-manager pod %s: %v", pod.Name, err)
			return
		}
	}
	result.Repaired = true
}

func accumulateLonghornResult(summary *LonghornRWXSummary, result LonghornRWXVolumeStatus) {
	if !result.Available {
		summary.Unavailable++
	}
	if result.Robustness != "healthy" {
		summary.Unhealthy++
	}
	if !result.EndpointReady {
		summary.EndpointUnavailable++
	}
	if result.SnapshotWarning {
		summary.SnapshotWarning++
	}
	if result.SnapshotBlocked {
		summary.SnapshotBlocked++
	}
	if result.RepairAction != "" {
		summary.RepairAttempted++
	}
	if result.Repaired {
		summary.RepairSucceeded++
	}
	if result.Skipped != "" {
		summary.RepairSkipped++
	}
	if result.Error != "" {
		summary.Failed++
	}
}

func longhornAccessMode(vol *unstructured.Unstructured) string {
	mode, _, _ := unstructured.NestedString(vol.Object, "spec", "accessMode")
	return strings.TrimSpace(mode)
}

func longhornRobustness(vol *unstructured.Unstructured) string {
	robustness, _, _ := unstructured.NestedString(vol.Object, "status", "robustness")
	robustness = strings.TrimSpace(strings.ToLower(robustness))
	if robustness == "" {
		return "unknown"
	}
	return robustness
}

func (c *Client) countLonghornActiveSnapshots(ctx context.Context, namespace, volume string) (int, error) {
	snapshots, err := c.dynamicClient.Resource(longhornSnapshotGVR).Namespace(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return 0, err
	}
	count := 0
	for i := range snapshots.Items {
		if snapshots.Items[i].GetDeletionTimestamp() != nil {
			continue
		}
		got, _, _ := unstructured.NestedString(snapshots.Items[i].Object, "spec", "volume")
		if got == volume {
			count++
		}
	}
	return count, nil
}

func (c *Client) longhornShareEndpointMode(ctx context.Context, namespace, volume string) (string, error) {
	services, err := c.clientset.CoreV1().Services(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: fmt.Sprintf("%s=%s", longhornShareManagerLabelKey, volume),
	})
	if err != nil {
		return "unknown", fmt.Errorf("list share-manager services: %w", err)
	}
	if len(services.Items) == 0 {
		return "missing", fmt.Errorf("no share-manager service found for volume %s", volume)
	}
	service := services.Items[0]
	if service.Spec.ClusterIP == "" || service.Spec.ClusterIP == "None" {
		return longhornEndpointModeClusterIP, fmt.Errorf("share-manager service %s has no ClusterIP", service.Name)
	}
	endpoints, err := c.clientset.CoreV1().Endpoints(namespace).Get(ctx, service.Name, metav1.GetOptions{})
	if err != nil {
		return longhornEndpointModeClusterIP, fmt.Errorf("get share-manager endpoints %s: %w", service.Name, err)
	}
	for _, subset := range endpoints.Subsets {
		if len(subset.Addresses) > 0 && endpointPortsInclude(subset.Ports, longhornShareManagerNFSPort) {
			return longhornEndpointModeClusterIP, nil
		}
	}
	return longhornEndpointModeClusterIP, fmt.Errorf("share-manager service %s has no ready NFS endpoint", service.Name)
}

func longhornRepairInCooldown(vol *unstructured.Unstructured, cooldown time.Duration, now func() time.Time) bool {
	if cooldown <= 0 {
		return false
	}
	last := vol.GetAnnotations()[LonghornRWXRepairCooldownAnnotation]
	if last == "" {
		return false
	}
	ts, err := time.Parse(time.RFC3339, last)
	if err != nil {
		return false
	}
	return now().Sub(ts) < cooldown
}

func (c *Client) longhornShareManagerPods(ctx context.Context, namespace, volume string) ([]corev1.Pod, error) {
	pods, err := c.clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: fmt.Sprintf("%s=%s", longhornShareManagerLabelKey, volume),
	})
	if err != nil {
		return nil, fmt.Errorf("list share-manager pods for %s: %w", volume, err)
	}
	return pods.Items, nil
}

func failedLonghornShareManagerPods(pods []corev1.Pod) []corev1.Pod {
	failed := make([]corev1.Pod, 0)
	for _, pod := range pods {
		if longhornShareManagerPodFailed(pod) {
			failed = append(failed, pod)
		}
	}
	return failed
}

func longhornShareManagerPodFailed(pod corev1.Pod) bool {
	if pod.Status.Phase == corev1.PodFailed || pod.Status.Phase == corev1.PodSucceeded {
		return true
	}
	for _, status := range pod.Status.ContainerStatuses {
		if status.State.Terminated != nil {
			return true
		}
		if status.State.Waiting != nil && longhornShareManagerWaitingFailed(status.State.Waiting.Reason) {
			return true
		}
	}
	return false
}

func longhornShareManagerWaitingFailed(reason string) bool {
	switch reason {
	case "CrashLoopBackOff", "Error", "RunContainerError":
		return true
	default:
		return false
	}
}

func (c *Client) longhornVolumeHasActiveConsumers(ctx context.Context, volume string) (bool, error) {
	claims, err := c.claimsForLonghornVolume(ctx, volume)
	if err != nil || len(claims) == 0 {
		return false, err
	}
	pods, err := c.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return false, fmt.Errorf("list pods for Longhorn consumer check: %w", err)
	}
	for _, pod := range pods.Items {
		if pod.DeletionTimestamp != nil || pod.Status.Phase == corev1.PodSucceeded || pod.Status.Phase == corev1.PodFailed {
			continue
		}
		for _, vol := range pod.Spec.Volumes {
			if vol.PersistentVolumeClaim == nil {
				continue
			}
			if _, ok := claims[pod.Namespace+"/"+vol.PersistentVolumeClaim.ClaimName]; ok {
				return true, nil
			}
		}
	}
	return false, nil
}

func (c *Client) claimsForLonghornVolume(ctx context.Context, volume string) (map[string]struct{}, error) {
	pvs, err := c.clientset.CoreV1().PersistentVolumes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list PVs for Longhorn volume %s: %w", volume, err)
	}
	claims := map[string]struct{}{}
	for _, pv := range pvs.Items {
		if !pvPointsAtLonghornVolume(pv, volume) || pv.Spec.ClaimRef == nil {
			continue
		}
		claims[pv.Spec.ClaimRef.Namespace+"/"+pv.Spec.ClaimRef.Name] = struct{}{}
	}
	return claims, nil
}

func pvPointsAtLonghornVolume(pv corev1.PersistentVolume, volume string) bool {
	if pv.Spec.CSI != nil && pv.Spec.CSI.Driver == csiDriverLonghorn && pv.Spec.CSI.VolumeHandle == volume {
		return true
	}
	return pv.Spec.NFS != nil && pv.Spec.NFS.Path == "/"+volume
}

func (c *Client) annotateLonghornRWXRepair(ctx context.Context, namespace, volume, reason string, now func() time.Time) error {
	obj, err := c.dynamicClient.Resource(longhornVolumeGVR).Namespace(namespace).Get(ctx, volume, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("get Longhorn volume for repair annotation: %w", err)
	}
	annotations := obj.GetAnnotations()
	if annotations == nil {
		annotations = map[string]string{}
	}
	annotations[LonghornRWXRepairCooldownAnnotation] = now().UTC().Format(time.RFC3339)
	annotations[LonghornRWXRepairReasonAnnotation] = reason
	obj.SetAnnotations(annotations)
	if _, err := c.dynamicClient.Resource(longhornVolumeGVR).Namespace(namespace).Update(ctx, obj, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("update Longhorn repair annotation: %w", err)
	}
	return nil
}
