package cluster

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestEnsurePVCMountedCreatesJuiceFSStaticShare(t *testing.T) {
	ctx := context.Background()
	cl := New(fake.NewSimpleClientset(
		boundPVC("group-storage", "datasets", "pv-juicefs"),
		csiSourcePV("pv-juicefs", csiDriverJuiceFS, corev1.ReadWriteMany),
	), "proj")

	if err := cl.EnsurePVCMounted(ctx, "group-storage", "datasets", "proj-p1", "datasets"); err != nil {
		t.Fatal(err)
	}

	pv, err := cl.Clientset().CoreV1().PersistentVolumes().Get(ctx, "share-juicefs-proj-p1-datasets", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("JuiceFS share PV was not created: %v", err)
	}
	if pv.Spec.CSI == nil || pv.Spec.CSI.Driver != csiDriverJuiceFS || pv.Spec.CSI.VolumeHandle != "pv-juicefs" {
		t.Fatalf("share PV CSI source = %#v, want JuiceFS pv-juicefs", pv.Spec.CSI)
	}
	pvc, err := cl.Clientset().CoreV1().PersistentVolumeClaims("proj-p1").Get(ctx, "datasets", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("target PVC was not created: %v", err)
	}
	if pvc.Spec.VolumeName != "share-juicefs-proj-p1-datasets" || pvc.Spec.StorageClassName == nil || *pvc.Spec.StorageClassName != "" {
		t.Fatalf("target PVC spec = %#v, want static binding without storage class", pvc.Spec)
	}
}

func TestEnsurePVCMountedCreatesLonghornNFSShare(t *testing.T) {
	ctx := context.Background()
	cl := New(fake.NewSimpleClientset(
		boundPVC("group-storage", "datasets", "pv-longhorn"),
		csiSourcePV("pv-longhorn", csiDriverLonghorn, corev1.ReadWriteMany),
	), "proj")
	cl.shareConfig.RWXNFSMountOptions = []string{"vers=4.2", "hard"}
	cl.longhornShareEndpointResolver = func(context.Context, string) (string, error) { return "10.111.250.99", nil }

	if err := cl.EnsurePVCMounted(ctx, "group-storage", "datasets", "proj-p1", "datasets"); err != nil {
		t.Fatal(err)
	}

	pv, err := cl.Clientset().CoreV1().PersistentVolumes().Get(ctx, "share-nfs-proj-p1-datasets", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Longhorn NFS share PV was not created: %v", err)
	}
	if pv.Spec.NFS == nil || pv.Spec.NFS.Server != "10.111.250.99" || pv.Spec.NFS.Path != "/pv-longhorn" {
		t.Fatalf("share PV NFS source = %#v, want Longhorn NFS endpoint", pv.Spec.NFS)
	}
	if !reflect.DeepEqual(pv.Spec.MountOptions, []string{"vers=4.2", "hard"}) {
		t.Fatalf("mount options = %#v, want configured options", pv.Spec.MountOptions)
	}
}

func TestEnsurePVCMountedRepairsExistingLonghornNFSShare(t *testing.T) {
	ctx := context.Background()
	cl := New(fake.NewSimpleClientset(
		boundPVC("group-storage", "datasets", "pv-longhorn"),
		csiSourcePV("pv-longhorn", csiDriverLonghorn, corev1.ReadWriteMany),
		targetPVC("proj-p1", "datasets", "share-nfs-proj-p1-datasets"),
		targetNFSPV("share-nfs-proj-p1-datasets", "10.111.250.88", "/pv-longhorn"),
	), "proj")
	cl.shareConfig.RWXNFSMountOptions = []string{"vers=4.2", "hard"}
	cl.longhornShareEndpointResolver = func(context.Context, string) (string, error) { return "10.111.250.99", nil }

	if err := cl.EnsurePVCMounted(ctx, "group-storage", "datasets", "proj-p1", "datasets"); err != nil {
		t.Fatal(err)
	}

	pv, err := cl.Clientset().CoreV1().PersistentVolumes().Get(ctx, "share-nfs-proj-p1-datasets", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get repaired PV: %v", err)
	}
	if pv.Spec.NFS.Server != "10.111.250.99" || !reflect.DeepEqual(pv.Spec.MountOptions, []string{"vers=4.2", "hard"}) {
		t.Fatalf("repaired PV = %#v options %#v, want refreshed endpoint/options", pv.Spec.NFS, pv.Spec.MountOptions)
	}
}

func TestEnsurePVCMountedRejectsInvalidAndUnsupportedShares(t *testing.T) {
	ctx := context.Background()
	cl := New(fake.NewSimpleClientset(
		boundPVC("group-storage", "datasets", "pv-longhorn"),
		csiSourcePV("pv-longhorn", csiDriverLonghorn, corev1.ReadWriteOnce),
	), "proj")
	if err := cl.EnsurePVCMounted(ctx, "", "datasets", "proj-p1", "datasets"); !errors.Is(err, ErrInvalidManifest) {
		t.Fatalf("missing refs err = %v, want ErrInvalidManifest", err)
	}
	err := cl.EnsurePVCMounted(ctx, "group-storage", "datasets", "proj-p1", "datasets")
	if err == nil || !strings.Contains(err.Error(), "not RWX") {
		t.Fatalf("RWO Longhorn err = %v, want not RWX", err)
	}

	cl = New(fake.NewSimpleClientset(
		boundPVC("group-storage", "datasets", "pv-hostpath"),
		csiSourcePV("pv-hostpath", "example.com/hostpath", corev1.ReadWriteMany),
	), "proj")
	err = cl.EnsurePVCMounted(ctx, "group-storage", "datasets", "proj-p1", "datasets")
	if err == nil || !strings.Contains(err.Error(), "unsupported storage driver") {
		t.Fatalf("unsupported driver err = %v, want unsupported storage driver", err)
	}
}

func TestResolveLonghornShareEndpointFromService(t *testing.T) {
	ctx := context.Background()
	cl := New(fake.NewSimpleClientset(
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "share-manager-pv-longhorn",
				Namespace: defaultLonghornNamespace,
				Labels:    map[string]string{longhornShareManagerLabelKey: "pv-longhorn"},
			},
			Spec: corev1.ServiceSpec{ClusterIP: "10.111.250.99"},
		},
		&corev1.Endpoints{
			ObjectMeta: metav1.ObjectMeta{Name: "share-manager-pv-longhorn", Namespace: defaultLonghornNamespace},
			Subsets: []corev1.EndpointSubset{{
				Addresses: []corev1.EndpointAddress{{IP: "10.244.0.10"}},
				Ports:     []corev1.EndpointPort{{Port: longhornShareManagerNFSPort}},
			}},
		},
	), "proj")

	ip, err := cl.resolveLonghornShareEndpoint(ctx, "pv-longhorn")
	if err != nil {
		t.Fatal(err)
	}
	if ip != "10.111.250.99" {
		t.Fatalf("resolved endpoint = %q, want service ClusterIP", ip)
	}
}

func boundPVC(namespace, name, volumeName string) *corev1.PersistentVolumeClaim {
	return &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		Spec: corev1.PersistentVolumeClaimSpec{
			Resources:  corev1.VolumeResourceRequirements{Requests: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("10Gi")}},
			VolumeName: volumeName,
		},
		Status: corev1.PersistentVolumeClaimStatus{Phase: corev1.ClaimBound},
	}
}

func csiSourcePV(name, driver string, accessMode corev1.PersistentVolumeAccessMode) *corev1.PersistentVolume {
	return &corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: corev1.PersistentVolumeSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{accessMode},
			Capacity:    corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("10Gi")},
			PersistentVolumeSource: corev1.PersistentVolumeSource{
				CSI: &corev1.CSIPersistentVolumeSource{Driver: driver, VolumeHandle: name},
			},
		},
	}
}

func targetPVC(namespace, name, volumeName string) *corev1.PersistentVolumeClaim {
	return &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		Spec:       corev1.PersistentVolumeClaimSpec{VolumeName: volumeName},
	}
}

func targetNFSPV(name, server, path string) *corev1.PersistentVolume {
	return &corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: corev1.PersistentVolumeSpec{
			MountOptions: []string{"stale"},
			PersistentVolumeSource: corev1.PersistentVolumeSource{
				NFS: &corev1.NFSVolumeSource{Server: server, Path: path},
			},
		},
	}
}
