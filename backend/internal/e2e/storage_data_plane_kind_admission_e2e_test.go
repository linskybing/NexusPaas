//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/platform"
	"github.com/linskybing/nexuspaas/backend/internal/platform/cluster"
	corev1 "k8s.io/api/core/v1"
	schedulingv1 "k8s.io/api/scheduling/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestStorageDataPlaneKindAdmissionE2E(t *testing.T) {
	if strings.TrimSpace(os.Getenv("TEST_LIVE_STORAGE_DATAPLANE_KIND_ADMISSION")) != "1" {
		t.Skip("set TEST_LIVE_STORAGE_DATAPLANE_KIND_ADMISSION=1 to run live storage DataPlane kind admission e2e")
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
	ensureLiveStorageDataPlanePriorityClass(t, ctx, cl)

	store := platform.NewStore()
	events := platform.NewEventBus()
	storageApp := newStorageDataPlaneDispatchStorageApp(store, events)
	storageServer := httptest.NewServer(storageApp)
	t.Cleanup(storageServer.Close)

	ids := seedLiveStorageDataPlaneKindAdmissionRecords(t, store)
	sourcePV := "pv-" + ids.jobID
	targetPV := fmt.Sprintf("share-juicefs-%s-%s", ids.namespace, ids.targetPVC)
	cleanupLiveStorageDataPlaneObjects(t, cl, ids, sourcePV, targetPV)
	seedLiveStorageDataPlaneSourcePVC(t, ctx, cl, ids, sourcePV)

	workloadApp := newStorageDataPlaneDispatchWorkloadApp(store, events, cl, storageServer.URL)
	createStorageDataPlaneDispatchJob(t, store, ids)

	waitForLiveStorageDataPlaneDispatch(t, ctx, workloadApp, cl, store, ids, targetPV)
	assertDataPlanePlanBuiltEvent(t, events, ids)
}

func ensureLiveStorageDataPlanePriorityClass(t *testing.T, ctx context.Context, cl *cluster.Client) {
	t.Helper()
	policy := corev1.PreemptLowerPriority
	pc := &schedulingv1.PriorityClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: "platform-batch-low",
			Labels: map[string]string{
				"nexuspaas.io/e2e": "storage-data-plane-kind-admission",
			},
		},
		Value:            0,
		PreemptionPolicy: &policy,
		GlobalDefault:    false,
		Description:      "NexusPaas storage DataPlane live admission E2E priority class",
	}
	if _, err := cl.Clientset().SchedulingV1().PriorityClasses().Create(ctx, pc, metav1.CreateOptions{}); err != nil {
		if apierrors.IsAlreadyExists(err) {
			return
		}
		t.Fatalf("create live admission PriorityClass %s: %v", pc.Name, err)
	}
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		err := cl.Clientset().SchedulingV1().PriorityClasses().Delete(cleanupCtx, pc.Name, metav1.DeleteOptions{})
		if err != nil && !apierrors.IsNotFound(err) {
			t.Logf("cleanup PriorityClass %s: %v", pc.Name, err)
		}
	})
}

func seedLiveStorageDataPlaneKindAdmissionRecords(t *testing.T, store *platform.Store) storageDataPlanePlanIDs {
	t.Helper()
	suffix := truncateID(sanitizeID(fmt.Sprint(time.Now().UTC().UnixNano())), 16)
	ids := storageDataPlanePlanIDs{
		projectID:       "p" + suffix,
		groupID:         "g" + suffix,
		userID:          "u" + suffix,
		jobID:           "j" + suffix,
		pvcID:           "datasets",
		namespace:       "proj-sdp-" + suffix,
		sourceNamespace: "src-sdp-" + suffix,
		sourcePVC:       "source-" + suffix,
		targetPVC:       "target-" + suffix,
	}
	createStorageDataPlaneRecord(t, store, e2eStorageBindingsResource, map[string]any{
		"id":         ids.projectID + ":" + ids.pvcID,
		"project_id": ids.projectID,
		"group_id":   ids.groupID,
		"pvc_id":     ids.pvcID,
		"target_pvc": ids.targetPVC,
	})
	createStorageDataPlaneRecord(t, store, e2eStorageGroupResource, map[string]any{
		"id":               ids.groupID + ":" + ids.pvcID,
		"group_id":         ids.groupID,
		"pvc_id":           ids.pvcID,
		"status":           "running",
		"namespace":        ids.sourceNamespace,
		"source_namespace": ids.sourceNamespace,
		"source_pvc":       ids.sourcePVC,
	})
	createStorageDataPlaneRecord(t, store, e2eStoragePermissionsResource, map[string]any{
		"id":         ids.projectID + ":" + ids.pvcID + ":" + ids.userID,
		"project_id": ids.projectID,
		"pvc_id":     ids.pvcID,
		"user_id":    ids.userID,
		"permission": "read_only",
	})
	return ids
}

func seedLiveStorageDataPlaneSourcePVC(t *testing.T, ctx context.Context, cl *cluster.Client, ids storageDataPlanePlanIDs, pvName string) {
	t.Helper()
	if err := cl.EnsureNamespace(ctx, ids.sourceNamespace); err != nil {
		t.Fatalf("create source namespace %s: %v", ids.sourceNamespace, err)
	}
	scName := ""
	volumeMode := corev1.PersistentVolumeFilesystem
	pv := e2eJuiceFSPV(pvName)
	pv.Spec.StorageClassName = scName
	pv.Spec.VolumeMode = &volumeMode
	pv.Spec.ClaimRef = &corev1.ObjectReference{
		Kind:       "PersistentVolumeClaim",
		APIVersion: "v1",
		Namespace:  ids.sourceNamespace,
		Name:       ids.sourcePVC,
	}
	if _, err := cl.Clientset().CoreV1().PersistentVolumes().Create(ctx, pv, metav1.CreateOptions{}); err != nil && !apierrors.IsAlreadyExists(err) {
		t.Fatalf("create source PV %s: %v", pvName, err)
	}
	pvc := e2eBoundPVC(ids.sourceNamespace, ids.sourcePVC, pvName)
	pvc.Spec.AccessModes = []corev1.PersistentVolumeAccessMode{corev1.ReadWriteMany}
	pvc.Spec.StorageClassName = &scName
	pvc.Spec.VolumeMode = &volumeMode
	pvc.Spec.Resources = corev1.VolumeResourceRequirements{Requests: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("10Gi")}}
	pvc.Status = corev1.PersistentVolumeClaimStatus{}
	if _, err := cl.Clientset().CoreV1().PersistentVolumeClaims(ids.sourceNamespace).Create(ctx, pvc, metav1.CreateOptions{}); err != nil && !apierrors.IsAlreadyExists(err) {
		t.Fatalf("create source PVC %s/%s: %v", ids.sourceNamespace, ids.sourcePVC, err)
	}
	waitForLiveStorageDataPlaneSourceBound(t, ctx, cl, ids)
}

func waitForLiveStorageDataPlaneSourceBound(t *testing.T, ctx context.Context, cl *cluster.Client, ids storageDataPlanePlanIDs) {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for {
		pvc, err := cl.Clientset().CoreV1().PersistentVolumeClaims(ids.sourceNamespace).Get(ctx, ids.sourcePVC, metav1.GetOptions{})
		if err == nil && pvc.Spec.VolumeName != "" && pvc.Status.Phase == corev1.ClaimBound {
			return
		}
		if err != nil && !apierrors.IsNotFound(err) {
			t.Fatalf("get source PVC %s/%s: %v", ids.sourceNamespace, ids.sourcePVC, err)
		}
		if time.Now().After(deadline) {
			if err == nil {
				t.Fatalf("source PVC %s/%s did not bind; phase=%s volume=%q", ids.sourceNamespace, ids.sourcePVC, pvc.Status.Phase, pvc.Spec.VolumeName)
			}
			t.Fatalf("source PVC %s/%s did not bind: %v", ids.sourceNamespace, ids.sourcePVC, err)
		}
		time.Sleep(250 * time.Millisecond)
	}
}

func waitForLiveStorageDataPlaneDispatch(
	t *testing.T,
	ctx context.Context,
	workloadApp *platform.App,
	cl *cluster.Client,
	store *platform.Store,
	ids storageDataPlanePlanIDs,
	targetPV string,
) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for {
		workloadApp.RunMaintenanceOnce(ctx, 200*time.Millisecond)
		pvc, pvcErr := cl.Clientset().CoreV1().PersistentVolumeClaims(ids.namespace).Get(ctx, ids.targetPVC, metav1.GetOptions{})
		pod, podErr := cl.Clientset().CoreV1().Pods(ids.namespace).Get(ctx, "data-plane-worker", metav1.GetOptions{})
		_, nsErr := cl.Clientset().CoreV1().Namespaces().Get(ctx, ids.namespace, metav1.GetOptions{})
		_, sourcePVErr := cl.Clientset().CoreV1().PersistentVolumes().Get(ctx, "pv-"+ids.jobID, metav1.GetOptions{})
		_, sourcePVCErr := cl.Clientset().CoreV1().PersistentVolumeClaims(ids.sourceNamespace).Get(ctx, ids.sourcePVC, metav1.GetOptions{})
		_, targetPVErr := cl.Clientset().CoreV1().PersistentVolumes().Get(ctx, targetPV, metav1.GetOptions{})
		if nsErr == nil && sourcePVErr == nil && sourcePVCErr == nil && targetPVErr == nil && pvcErr == nil && podErr == nil {
			if pvc.Spec.VolumeName != targetPV {
				t.Fatalf("target PVC volume = %q, want %q", pvc.Spec.VolumeName, targetPV)
			}
			assertDataPlaneDispatchPod(t, pod, ids)
			return
		}
		if fatalLiveStorageDataPlaneGetError(nsErr, sourcePVErr, sourcePVCErr, targetPVErr, pvcErr, podErr) {
			t.Fatalf("live storage DataPlane object lookup failed: ns=%v sourcePV=%v sourcePVC=%v targetPV=%v targetPVC=%v pod=%v", nsErr, sourcePVErr, sourcePVCErr, targetPVErr, pvcErr, podErr)
		}
		if time.Now().After(deadline) {
			record, _ := store.Get(ctx, workloadJobsResource, ids.jobID)
			t.Fatalf("storage DataPlane dispatch objects not visible before deadline; ns=%v sourcePV=%v sourcePVC=%v targetPV=%v targetPVC=%v pod=%v job=%#v", nsErr, sourcePVErr, sourcePVCErr, targetPVErr, pvcErr, podErr, record.Data)
		}
		time.Sleep(250 * time.Millisecond)
	}
}

func fatalLiveStorageDataPlaneGetError(errs ...error) bool {
	for _, err := range errs {
		if err != nil && !apierrors.IsNotFound(err) {
			return true
		}
	}
	return false
}

func cleanupLiveStorageDataPlaneObjects(t *testing.T, cl *cluster.Client, ids storageDataPlanePlanIDs, pvs ...string) {
	t.Helper()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		for _, ns := range []string{ids.namespace, ids.sourceNamespace} {
			if err := cl.Clientset().CoreV1().Namespaces().Delete(ctx, ns, metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
				t.Logf("cleanup namespace %s: %v", ns, err)
			}
		}
		for _, pv := range pvs {
			if err := cl.Clientset().CoreV1().PersistentVolumes().Delete(ctx, pv, metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
				t.Logf("cleanup PV %s: %v", pv, err)
			}
		}
	})
}
