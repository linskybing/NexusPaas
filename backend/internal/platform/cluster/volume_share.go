package cluster

import (
	"context"
	"fmt"
	"os"
	"reflect"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	csiDriverLonghorn            = "driver.longhorn.io"
	csiDriverJuiceFS             = "csi.juicefs.com"
	longhornShareManagerLabelKey = "longhorn.io/share-manager"
	longhornShareManagerNFSPort  = 2049
	volumeShareCreatedByLabel    = "created-by"
	volumeShareTypeLabel         = "share-type"
	volumeShareCreatedByValue    = "k8s-platform-share"
	defaultLonghornNamespace     = "longhorn-system"
	defaultRWXNFSMountOptions    = "vers=4.2,hard,nconnect=8,rsize=1048576,wsize=1048576,timeo=600,retrans=2,noatime"
	sourcePVCBindPollInterval    = 2 * time.Second
	sourcePVCBindTimeout         = 90 * time.Second
)

type volumeShareConfig struct {
	LonghornNamespace  string
	RWXNFSMountOptions []string
}

func volumeShareConfigFromEnv() volumeShareConfig {
	namespace := strings.TrimSpace(os.Getenv("LONGHORN_NAMESPACE"))
	if namespace == "" {
		namespace = defaultLonghornNamespace
	}
	options, err := parseRWXNFSMountOptions(firstNonEmptyEnv("RWX_NFS_MOUNT_OPTIONS", "LONGHORN_RWX_NFS_MOUNT_OPTIONS"))
	if err != nil {
		options, _ = parseRWXNFSMountOptions(defaultRWXNFSMountOptions)
	}
	return volumeShareConfig{LonghornNamespace: namespace, RWXNFSMountOptions: options}
}

func firstNonEmptyEnv(keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			return value
		}
	}
	return ""
}

// EnsurePVCMounted shares a bound CSI PVC into a workload namespace. It ports
// the reference backend's Longhorn RWX NFS and JuiceFS static-PV paths behind
// the injected cluster facade, so callers can stay service-owned and testable.
func (c *Client) EnsurePVCMounted(ctx context.Context, sourceNs, sourcePVC, targetNs, targetPVC string) error {
	if c == nil || c.clientset == nil {
		return ErrUnavailable
	}
	if err := validatePVCMountRefs(sourceNs, sourcePVC, targetNs, targetPVC); err != nil {
		return err
	}
	source, err := c.waitForPVCBound(ctx, sourceNs, sourcePVC)
	if err != nil {
		return fmt.Errorf("find source pvc %s/%s: %w", sourceNs, sourcePVC, err)
	}
	sourcePV, err := c.sourcePersistentVolume(ctx, source)
	if err != nil {
		return err
	}
	switch sourcePV.Spec.CSI.Driver {
	case csiDriverLonghorn:
		return c.mountLonghornVolume(ctx, source, sourcePV, targetNs, targetPVC)
	case csiDriverJuiceFS:
		return c.mountJuiceFSVolume(ctx, source, sourcePV, targetNs, targetPVC)
	default:
		return fmt.Errorf("source pv %s uses unsupported storage driver %s", sourcePV.Name, sourcePV.Spec.CSI.Driver)
	}
}

func validatePVCMountRefs(sourceNs, sourcePVC, targetNs, targetPVC string) error {
	if strings.TrimSpace(sourceNs) == "" || strings.TrimSpace(sourcePVC) == "" ||
		strings.TrimSpace(targetNs) == "" || strings.TrimSpace(targetPVC) == "" {
		return fmt.Errorf("%w: source and target PVC references are required", ErrInvalidManifest)
	}
	return nil
}

func (c *Client) waitForPVCBound(ctx context.Context, namespace, pvcName string) (*corev1.PersistentVolumeClaim, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	waitCtx, cancel := context.WithTimeout(ctx, sourcePVCBindTimeout)
	defer cancel()
	ticker := time.NewTicker(sourcePVCBindPollInterval)
	defer ticker.Stop()

	var lastPVC *corev1.PersistentVolumeClaim
	var lastErr error
	for {
		pvc, err := c.clientset.CoreV1().PersistentVolumeClaims(namespace).Get(waitCtx, pvcName, metav1.GetOptions{})
		if err == nil {
			lastPVC = pvc
			if pvc.Spec.VolumeName != "" && pvc.Status.Phase == corev1.ClaimBound {
				return pvc, nil
			}
			lastErr = fmt.Errorf("source pvc %s/%s is not bound yet (phase=%s, volume=%q)", namespace, pvcName, pvc.Status.Phase, pvc.Spec.VolumeName)
		} else {
			lastErr = err
		}
		select {
		case <-waitCtx.Done():
			return nil, pvcWaitError(namespace, pvcName, lastPVC, lastErr)
		case <-ticker.C:
		}
	}
}

func pvcWaitError(namespace, pvcName string, lastPVC *corev1.PersistentVolumeClaim, lastErr error) error {
	if lastPVC != nil {
		return fmt.Errorf("timed out waiting for source pvc %s/%s to bind (last phase=%s, volume=%q)", namespace, pvcName, lastPVC.Status.Phase, lastPVC.Spec.VolumeName)
	}
	if lastErr != nil {
		return lastErr
	}
	return fmt.Errorf("timed out waiting for source pvc %s/%s to bind", namespace, pvcName)
}

func (c *Client) sourcePersistentVolume(ctx context.Context, source *corev1.PersistentVolumeClaim) (*corev1.PersistentVolume, error) {
	pv, err := c.clientset.CoreV1().PersistentVolumes().Get(ctx, source.Spec.VolumeName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("get source pv %s: %w", source.Spec.VolumeName, err)
	}
	if pv.Spec.CSI == nil {
		return nil, fmt.Errorf("source pv %s is not a supported CSI volume", pv.Name)
	}
	return pv, nil
}

func (c *Client) mountJuiceFSVolume(
	ctx context.Context,
	sourcePVC *corev1.PersistentVolumeClaim,
	sourcePV *corev1.PersistentVolume,
	targetNs,
	targetPVCName string,
) error {
	volumeHandle := sourcePV.Spec.CSI.VolumeHandle
	if !hasAccessMode(sourcePV.Spec.AccessModes, corev1.ReadWriteMany) {
		return fmt.Errorf("JuiceFS source volume %s is not ReadWriteMany", volumeHandle)
	}
	targetPVC, err := c.clientset.CoreV1().PersistentVolumeClaims(targetNs).Get(ctx, targetPVCName, metav1.GetOptions{})
	if err == nil {
		return c.ensureExistingJuiceFSTarget(ctx, targetPVC, volumeHandle)
	}
	if !apierrors.IsNotFound(err) {
		return fmt.Errorf("check existing target pvc: %w", err)
	}
	pvName := fmt.Sprintf("share-juicefs-%s-%s", targetNs, targetPVCName)
	if err := c.createJuiceFSSharePV(ctx, sourcePVC, sourcePV, pvName, targetNs); err != nil {
		return err
	}
	return c.createStaticSharePVC(ctx, sourcePVQuantity(sourcePVC, sourcePV), "csi-juicefs", targetNs, targetPVCName, pvName)
}

func (c *Client) createJuiceFSSharePV(
	ctx context.Context,
	sourcePVC *corev1.PersistentVolumeClaim,
	sourcePV *corev1.PersistentVolume,
	pvName,
	targetNs string,
) error {
	volumeMode := volumeModeOrFilesystem(sourcePV)
	pv := &corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name: pvName,
			Labels: map[string]string{
				volumeShareCreatedByLabel: volumeShareCreatedByValue,
				volumeShareTypeLabel:      "csi-juicefs",
				"target-ns":               targetNs,
				"source-vol":              sourcePV.Spec.CSI.VolumeHandle,
				"source-driver":           csiDriverJuiceFS,
			},
		},
		Spec: corev1.PersistentVolumeSpec{
			Capacity:                      sourcePV.Spec.Capacity,
			VolumeMode:                    &volumeMode,
			AccessModes:                   []corev1.PersistentVolumeAccessMode{corev1.ReadWriteMany},
			MountOptions:                  sourcePV.Spec.MountOptions,
			PersistentVolumeReclaimPolicy: corev1.PersistentVolumeReclaimRetain,
			StorageClassName:              "",
			PersistentVolumeSource:        corev1.PersistentVolumeSource{CSI: sourcePV.Spec.CSI.DeepCopy()},
		},
	}
	pv.Spec.Capacity = corev1.ResourceList{corev1.ResourceStorage: sourcePVQuantity(sourcePVC, sourcePV)}
	if _, err := c.clientset.CoreV1().PersistentVolumes().Create(ctx, pv, metav1.CreateOptions{}); err != nil && !apierrors.IsAlreadyExists(err) {
		return fmt.Errorf("create JuiceFS share PV %s: %w", pvName, err)
	}
	return nil
}

func (c *Client) ensureExistingJuiceFSTarget(ctx context.Context, targetPVC *corev1.PersistentVolumeClaim, volumeHandle string) error {
	targetPV, err := c.boundTargetPV(ctx, targetPVC)
	if err != nil {
		return err
	}
	if targetPV.Spec.CSI == nil || targetPV.Spec.CSI.Driver != csiDriverJuiceFS {
		return fmt.Errorf("existing target pv %s is not JuiceFS-backed", targetPV.Name)
	}
	if targetPV.Spec.CSI.VolumeHandle != volumeHandle {
		return fmt.Errorf("existing target pv %s points at different JuiceFS volume", targetPV.Name)
	}
	return nil
}

func (c *Client) mountLonghornVolume(
	ctx context.Context,
	sourcePVC *corev1.PersistentVolumeClaim,
	sourcePV *corev1.PersistentVolume,
	targetNs,
	targetPVCName string,
) error {
	volumeHandle := sourcePV.Spec.CSI.VolumeHandle
	if !hasAccessMode(sourcePV.Spec.AccessModes, corev1.ReadWriteMany) {
		return fmt.Errorf("Longhorn volume %s is not RWX; use a shared RWX profile or fast-stage into a project-local RWO cache", volumeHandle)
	}
	targetPVC, err := c.clientset.CoreV1().PersistentVolumeClaims(targetNs).Get(ctx, targetPVCName, metav1.GetOptions{})
	if err == nil {
		return c.ensureExistingLonghornTarget(ctx, targetPVC, volumeHandle)
	}
	if !apierrors.IsNotFound(err) {
		return fmt.Errorf("check existing target pvc: %w", err)
	}
	nfsIP, err := c.longhornShareEndpointResolver(ctx, volumeHandle)
	if err != nil {
		return fmt.Errorf("RWX volume %s: share-manager NFS endpoint not available: %w", volumeHandle, err)
	}
	pvName := fmt.Sprintf("share-nfs-%s-%s", targetNs, targetPVCName)
	if err := c.createLonghornNFSPV(ctx, pvName, targetNs, volumeHandle, nfsIP, sourcePVQuantity(sourcePVC, sourcePV)); err != nil {
		return err
	}
	return c.createStaticSharePVC(ctx, sourcePVQuantity(sourcePVC, sourcePV), "nfs", targetNs, targetPVCName, pvName)
}

func (c *Client) ensureExistingLonghornTarget(ctx context.Context, targetPVC *corev1.PersistentVolumeClaim, volumeHandle string) error {
	targetPV, err := c.boundTargetPV(ctx, targetPVC)
	if err != nil {
		return err
	}
	if targetPV.Spec.NFS == nil {
		return fmt.Errorf("existing target pv %s for RWX volume %s is not NFS-backed", targetPV.Name, volumeHandle)
	}
	nfsIP, err := c.longhornShareEndpointResolver(ctx, volumeHandle)
	if err != nil {
		return fmt.Errorf("cannot resolve NFS endpoint for existing volume %s: %w", volumeHandle, err)
	}
	if c.longhornTargetPVHealthy(targetPV, volumeHandle, nfsIP) {
		return nil
	}
	updated := targetPV.DeepCopy()
	updated.Spec.NFS.Server = nfsIP
	updated.Spec.NFS.Path = "/" + volumeHandle
	updated.Spec.MountOptions = c.shareConfig.RWXNFSMountOptions
	if _, err := c.clientset.CoreV1().PersistentVolumes().Update(ctx, updated, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("update existing NFS share PV %s: %w", targetPV.Name, err)
	}
	return nil
}

func (c *Client) longhornTargetPVHealthy(pv *corev1.PersistentVolume, volumeHandle, nfsIP string) bool {
	return pv.Spec.NFS.Server == nfsIP &&
		pv.Spec.NFS.Path == "/"+volumeHandle &&
		reflect.DeepEqual(pv.Spec.MountOptions, c.shareConfig.RWXNFSMountOptions)
}

func (c *Client) createLonghornNFSPV(
	ctx context.Context,
	name,
	targetNs,
	volumeHandle,
	nfsIP string,
	size resource.Quantity,
) error {
	volumeMode := corev1.PersistentVolumeFilesystem
	pv := &corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			Labels: map[string]string{
				volumeShareCreatedByLabel: volumeShareCreatedByValue,
				volumeShareTypeLabel:      "nfs",
				"target-ns":               targetNs,
				"source-vol":              volumeHandle,
			},
		},
		Spec: corev1.PersistentVolumeSpec{
			Capacity:                      corev1.ResourceList{corev1.ResourceStorage: size},
			VolumeMode:                    &volumeMode,
			AccessModes:                   []corev1.PersistentVolumeAccessMode{corev1.ReadWriteMany},
			MountOptions:                  c.shareConfig.RWXNFSMountOptions,
			PersistentVolumeReclaimPolicy: corev1.PersistentVolumeReclaimRetain,
			StorageClassName:              "",
			PersistentVolumeSource: corev1.PersistentVolumeSource{
				NFS: &corev1.NFSVolumeSource{Server: nfsIP, Path: "/" + volumeHandle},
			},
		},
	}
	if _, err := c.clientset.CoreV1().PersistentVolumes().Create(ctx, pv, metav1.CreateOptions{}); err != nil && !apierrors.IsAlreadyExists(err) {
		return fmt.Errorf("create NFS share PV %s: %w", name, err)
	}
	return nil
}

func (c *Client) createStaticSharePVC(
	ctx context.Context,
	size resource.Quantity,
	shareType,
	targetNs,
	targetPVCName,
	pvName string,
) error {
	scName := ""
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      targetPVCName,
			Namespace: targetNs,
			Labels: map[string]string{
				volumeShareCreatedByLabel: volumeShareCreatedByValue,
				volumeShareTypeLabel:      shareType,
			},
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes:      []corev1.PersistentVolumeAccessMode{corev1.ReadWriteMany},
			Resources:        corev1.VolumeResourceRequirements{Requests: corev1.ResourceList{corev1.ResourceStorage: size}},
			StorageClassName: &scName,
			VolumeName:       pvName,
		},
	}
	if _, err := c.clientset.CoreV1().PersistentVolumeClaims(targetNs).Create(ctx, pvc, metav1.CreateOptions{}); err != nil && !apierrors.IsAlreadyExists(err) {
		return fmt.Errorf("create target pvc: %w", err)
	}
	return nil
}

func (c *Client) boundTargetPV(ctx context.Context, targetPVC *corev1.PersistentVolumeClaim) (*corev1.PersistentVolume, error) {
	if targetPVC.Spec.VolumeName == "" {
		return nil, fmt.Errorf("existing target pvc %s/%s is not bound yet (phase=%s)", targetPVC.Namespace, targetPVC.Name, targetPVC.Status.Phase)
	}
	targetPV, err := c.clientset.CoreV1().PersistentVolumes().Get(ctx, targetPVC.Spec.VolumeName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("get existing target pv %s: %w", targetPVC.Spec.VolumeName, err)
	}
	return targetPV, nil
}

func (c *Client) resolveLonghornShareEndpoint(ctx context.Context, volumeHandle string) (string, error) {
	services, err := c.clientset.CoreV1().Services(c.shareConfig.LonghornNamespace).List(ctx, metav1.ListOptions{
		LabelSelector: fmt.Sprintf("%s=%s", longhornShareManagerLabelKey, volumeHandle),
	})
	if err != nil {
		return "", fmt.Errorf("list share-manager services: %w", err)
	}
	if len(services.Items) == 0 {
		return "", fmt.Errorf("no share-manager service found for volume %s", volumeHandle)
	}
	service := services.Items[0]
	if service.Spec.ClusterIP == "" || service.Spec.ClusterIP == "None" {
		return "", fmt.Errorf("share-manager service %s has no ClusterIP", service.Name)
	}
	if err := c.requireReadyNFSEndpoint(ctx, service.Name); err != nil {
		return "", err
	}
	return service.Spec.ClusterIP, nil
}

func (c *Client) requireReadyNFSEndpoint(ctx context.Context, service string) error {
	endpoints, err := c.clientset.CoreV1().Endpoints(c.shareConfig.LonghornNamespace).Get(ctx, service, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("get share-manager endpoints %s: %w", service, err)
	}
	for _, subset := range endpoints.Subsets {
		if len(subset.Addresses) > 0 && endpointPortsInclude(subset.Ports, longhornShareManagerNFSPort) {
			return nil
		}
	}
	return fmt.Errorf("share-manager service %s has no ready NFS endpoint", service)
}

func endpointPortsInclude(ports []corev1.EndpointPort, target int32) bool {
	for _, port := range ports {
		if port.Port == target {
			return true
		}
	}
	return false
}

func sourcePVQuantity(sourcePVC *corev1.PersistentVolumeClaim, sourcePV *corev1.PersistentVolume) resource.Quantity {
	if sourcePVC != nil {
		if quantity, ok := sourcePVC.Spec.Resources.Requests[corev1.ResourceStorage]; ok {
			return quantity.DeepCopy()
		}
	}
	if sourcePV != nil {
		if quantity, ok := sourcePV.Spec.Capacity[corev1.ResourceStorage]; ok {
			return quantity.DeepCopy()
		}
	}
	return resource.MustParse("1Gi")
}

func volumeModeOrFilesystem(pv *corev1.PersistentVolume) corev1.PersistentVolumeMode {
	if pv != nil && pv.Spec.VolumeMode != nil {
		return *pv.Spec.VolumeMode
	}
	return corev1.PersistentVolumeFilesystem
}

func hasAccessMode(modes []corev1.PersistentVolumeAccessMode, target corev1.PersistentVolumeAccessMode) bool {
	for _, mode := range modes {
		if mode == target {
			return true
		}
	}
	return false
}

func parseRWXNFSMountOptions(raw string) ([]string, error) {
	if strings.TrimSpace(raw) == "" {
		raw = defaultRWXNFSMountOptions
	}
	seen := map[string]struct{}{}
	options := []string{}
	for _, item := range strings.Split(raw, ",") {
		option := strings.TrimSpace(item)
		if option == "" {
			return nil, fmt.Errorf("RWX_NFS_MOUNT_OPTIONS contains an empty option")
		}
		key := strings.ToLower(strings.SplitN(option, "=", 2)[0])
		if unsafeRWXNFSMountOption(key) {
			return nil, fmt.Errorf("RWX_NFS_MOUNT_OPTIONS contains unsafe option %q", key)
		}
		if _, duplicate := seen[key]; duplicate {
			return nil, fmt.Errorf("RWX_NFS_MOUNT_OPTIONS contains duplicate option %q", key)
		}
		seen[key] = struct{}{}
		options = append(options, option)
	}
	if len(options) == 0 || len(options) > 32 {
		return nil, fmt.Errorf("RWX_NFS_MOUNT_OPTIONS must contain 1-32 options")
	}
	return options, nil
}

func unsafeRWXNFSMountOption(key string) bool {
	switch key {
	case "async", "soft", "softerr":
		return true
	default:
		return false
	}
}
