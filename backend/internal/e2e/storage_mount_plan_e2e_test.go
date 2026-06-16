//go:build e2e

package e2e

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/platform"
	"github.com/linskybing/nexuspaas/backend/internal/platform/cluster"
	"github.com/linskybing/nexuspaas/backend/internal/services"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

const (
	e2eStorageGroupResource       = "storage-service:group_storage"
	e2eStorageBindingsResource    = "storage-service:storage_bindings"
	e2eStoragePermissionsResource = "storage-service:project_storage_permissions"
)

func TestStorageMountPlanContractE2E(t *testing.T) {
	h := newHarness(t, storageService)
	ids := h.seedStorageMountPlanRecords()

	plan := h.assertStorageMountPlanContract(ids, h.serviceKey, http.StatusOK)
	if plan.ProjectID != ids.projectID || plan.UserID != ids.userID || plan.Namespace != ids.namespace {
		t.Fatalf("storage plan identity = %#v, want project/user/namespace", plan)
	}
	if len(plan.PVCShareOperations) != 1 ||
		plan.PVCShareOperations[0].SourceNamespace != ids.sourceNamespace ||
		plan.PVCShareOperations[0].SourcePVC != ids.sourcePVC ||
		plan.PVCShareOperations[0].TargetPVC != ids.targetPVC {
		t.Fatalf("share operations = %#v, want storage-owned PVC refs", plan.PVCShareOperations)
	}
	h.assertStorageMountPlanContract(ids, "wrong-"+h.serviceKey, http.StatusUnauthorized)

	cl := e2eStorageFakeCluster(ids)
	workloadApp := h.newStorageMountWorkloadApp(cl, h.serviceKey)
	goodJobID := "mountjob" + h.runID
	h.createStorageMountWorkloadJob(goodJobID, ids)
	workloadApp.RunMaintenanceOnce(h.ctx, time.Minute)
	h.assertStorageMountWorkloadDispatched(cl, goodJobID, ids)

	badKeyApp := h.newStorageMountWorkloadApp(e2eStorageFakeCluster(ids), "wrong-"+h.serviceKey)
	badJobID := "badmountjob" + h.runID
	h.createStorageMountWorkloadJob(badJobID, ids)
	badKeyApp.RunMaintenanceOnce(h.ctx, time.Minute)
	badJob := h.getRecord(workloadJobsResource, badJobID)
	if badJob.Data["status"] != "failed" || !strings.Contains(textE2E(badJob.Data["error_message"]), "HTTP 401") {
		t.Fatalf("bad-key job = %#v, want failed HTTP 401 without storage materialization", badJob.Data)
	}
}

type storageMountE2EIDs struct {
	projectID       string
	groupID         string
	userID          string
	pvcID           string
	sourceNamespace string
	sourcePVC       string
	sourcePV        string
	targetPVC       string
	namespace       string
}

func (h *e2eHarness) seedStorageMountPlanRecords() storageMountE2EIDs {
	ids := storageMountE2EIDs{
		projectID:       "project" + h.runID,
		groupID:         "group" + h.runID,
		userID:          "user" + h.runID,
		pvcID:           "datasets",
		sourceNamespace: "group-" + h.runID + "-storage",
		sourcePVC:       "source-" + h.runID,
		sourcePV:        "pv-" + h.runID,
		targetPVC:       "target-" + h.runID,
		namespace:       "proj-" + h.runID,
	}
	h.createRecord(e2eStorageBindingsResource, ids.projectID+":"+ids.pvcID, map[string]any{
		"project_id": ids.projectID,
		"group_id":   ids.groupID,
		"pvc_id":     ids.pvcID,
		"target_pvc": ids.targetPVC,
	})
	h.createRecord(e2eStorageGroupResource, ids.groupID+":"+ids.pvcID, map[string]any{
		"group_id":         ids.groupID,
		"pvc_id":           ids.pvcID,
		"status":           "running",
		"namespace":        ids.sourceNamespace,
		"source_pvc":       ids.sourcePVC,
		"source_namespace": ids.sourceNamespace,
	})
	h.createRecord(e2eStoragePermissionsResource, ids.projectID+":"+ids.pvcID+":"+ids.userID, map[string]any{
		"project_id": ids.projectID,
		"pvc_id":     ids.pvcID,
		"user_id":    ids.userID,
		"permission": "read_only",
	})
	return ids
}

func (h *e2eHarness) assertStorageMountPlanContract(ids storageMountE2EIDs, serviceKey string, want int) storageMountPlanE2EResponse {
	body, err := json.Marshal(storageMountPlanE2ERequest(ids))
	if err != nil {
		h.t.Fatalf("marshal mount plan request: %v", err)
	}
	req := h.newRequest(storageService, http.MethodPost, "/internal/storage/projects/"+ids.projectID+"/mount-plan", bytes.NewReader(body), "")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Service-Key", serviceKey)
	resp := h.do(req, want)
	h.requireEnvelopeCorrelation(resp)
	if want != http.StatusOK {
		return storageMountPlanE2EResponse{}
	}
	var envelope struct {
		Data storageMountPlanE2EResponse `json:"data"`
	}
	if err := json.Unmarshal(resp.Body, &envelope); err != nil {
		h.t.Fatalf("decode mount plan envelope: %v body=%s", err, string(resp.Body))
	}
	return envelope.Data
}

func storageMountPlanE2ERequest(ids storageMountE2EIDs) map[string]any {
	return map[string]any{
		"user_id":   ids.userID,
		"namespace": ids.namespace,
		"mounts": []map[string]any{{
			"pvc_id":           ids.pvcID,
			"name":             "datasets",
			"mount_path":       "/mnt/datasets",
			"read_only":        true,
			"source_namespace": "forged-storage",
			"source_pvc":       "forged-source",
			"target_pvc":       "forged-target",
		}},
	}
}

type storageMountPlanE2EResponse struct {
	ProjectID          string                 `json:"project_id"`
	UserID             string                 `json:"user_id"`
	Namespace          string                 `json:"namespace"`
	ManifestMounts     []storageMountE2EMount `json:"manifest_mounts"`
	PVCShareOperations []storageMountE2EShare `json:"pvc_share_operations"`
}

type storageMountE2EMount struct {
	Name      string `json:"name"`
	ClaimName string `json:"claim_name"`
	MountPath string `json:"mount_path"`
	ReadOnly  bool   `json:"read_only"`
}

type storageMountE2EShare struct {
	SourceNamespace string `json:"source_namespace"`
	SourcePVC       string `json:"source_pvc"`
	TargetPVC       string `json:"target_pvc"`
}

func (h *e2eHarness) newStorageMountWorkloadApp(cl *cluster.Client, serviceKey string) *platform.App {
	cfg := h.serviceConfig(workloadService, map[string]string{storageService: h.services[storageService].url})
	cfg.ServiceAPIKey = serviceKey
	app := platform.NewApp(cfg, platform.WithStore(h.store), platform.WithCluster(cl))
	services.RegisterAll(app)
	return app
}

func (h *e2eHarness) createStorageMountWorkloadJob(jobID string, ids storageMountE2EIDs) {
	h.createRecord(workloadJobsResource, jobID, map[string]any{
		"job_id":     jobID,
		"project_id": ids.projectID,
		"user_id":    ids.userID,
		"status":     "submitted",
		"namespace":  ids.namespace,
		"created_at": time.Now().Add(-time.Minute).UTC().Format(time.RFC3339),
		"storage_mounts": []any{map[string]any{
			"pvc_id":           ids.pvcID,
			"name":             "datasets",
			"mount_path":       "/mnt/datasets",
			"read_only":        true,
			"source_namespace": "forged-storage",
			"source_pvc":       "forged-source",
			"target_pvc":       "forged-target",
		}},
		"resources": []any{map[string]any{
			"name": "storage-worker",
			"kind": "Pod",
			"json_data": `{
				"apiVersion":"v1",
				"kind":"Pod",
				"metadata":{"name":"storage-worker"},
				"spec":{"containers":[{"name":"main","image":"busybox"}]}
			}`,
		}},
	})
}

func e2eStorageFakeCluster(ids storageMountE2EIDs) *cluster.Client {
	return cluster.New(fake.NewSimpleClientset(
		e2eBoundPVC(ids.sourceNamespace, ids.sourcePVC, ids.sourcePV),
		e2eJuiceFSPV(ids.sourcePV),
	), "proj")
}

func e2eBoundPVC(namespace, name, volumeName string) *corev1.PersistentVolumeClaim {
	return &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		Spec: corev1.PersistentVolumeClaimSpec{
			Resources:  corev1.VolumeResourceRequirements{Requests: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("10Gi")}},
			VolumeName: volumeName,
		},
		Status: corev1.PersistentVolumeClaimStatus{Phase: corev1.ClaimBound},
	}
}

func e2eJuiceFSPV(name string) *corev1.PersistentVolume {
	return &corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: corev1.PersistentVolumeSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteMany},
			Capacity:    corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("10Gi")},
			PersistentVolumeSource: corev1.PersistentVolumeSource{
				CSI: &corev1.CSIPersistentVolumeSource{Driver: "csi.juicefs.com", VolumeHandle: name},
			},
		},
	}
}

func (h *e2eHarness) assertStorageMountWorkloadDispatched(cl *cluster.Client, jobID string, ids storageMountE2EIDs) {
	job := h.getRecord(workloadJobsResource, jobID)
	if status := textE2E(job.Data["status"]); status != "running" && status != "queued" {
		h.t.Fatalf("job = %#v, want dispatched running/queued status", job.Data)
	}
	if _, err := cl.Clientset().CoreV1().PersistentVolumeClaims(ids.namespace).Get(h.ctx, ids.targetPVC, metav1.GetOptions{}); err != nil {
		h.t.Fatalf("storage target PVC was not materialized through storage-owned plan: %v", err)
	}
	if _, err := cl.Clientset().CoreV1().PersistentVolumeClaims(ids.namespace).Get(h.ctx, "forged-target", metav1.GetOptions{}); err == nil {
		h.t.Fatal("forged target PVC was materialized; workload trusted payload source details")
	}
	pod, err := cl.Clientset().CoreV1().Pods(ids.namespace).Get(h.ctx, "storage-worker", metav1.GetOptions{})
	if err != nil {
		h.t.Fatalf("storage workload pod was not created: %v", err)
	}
	if len(pod.Spec.Volumes) != 1 || pod.Spec.Volumes[0].PersistentVolumeClaim == nil ||
		pod.Spec.Volumes[0].PersistentVolumeClaim.ClaimName != ids.targetPVC {
		h.t.Fatalf("pod volumes = %#v, want storage-owned target PVC %s", pod.Spec.Volumes, ids.targetPVC)
	}
	mount := pod.Spec.Containers[0].VolumeMounts[0]
	if mount.Name != "datasets" || mount.MountPath != "/mnt/datasets" || !mount.ReadOnly {
		h.t.Fatalf("pod mount = %#v, want read-only datasets mount", mount)
	}
}

func textE2E(value any) string {
	text, _ := value.(string)
	return text
}
