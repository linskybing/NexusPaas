package storage

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/linskybing/nexuspaas/backend/internal/platform"
	"k8s.io/apimachinery/pkg/util/yaml"
)

func TestStorageProfilesHPCStorageClassManifests(t *testing.T) {
	app := platform.NewApp(platform.Config{ServiceName: serviceName, HTTPAddr: ":0"})
	app.RegisterService(Spec())
	Register(app)

	profiles := app.Store.List(context.Background(), storageProfilesResource)
	if len(profiles) == 0 {
		t.Fatal("seeded storage profiles are empty")
	}
	manifests := loadHPCStorageClassManifests(t)
	minioArtifactChecked := false

	for _, profile := range profiles {
		minioArtifactChecked = assertStorageProfileManifest(t, profile.Data, manifests) || minioArtifactChecked
	}
	if !minioArtifactChecked {
		t.Fatal("minio-artifact seeded profile was not checked")
	}
}

func assertStorageProfileManifest(t *testing.T, profile map[string]any, manifests map[string]map[string]any) bool {
	t.Helper()
	id := profileText(profile, "id")
	storageClassName := profileText(profile, "storage_class_name")
	if id == "minio-artifact" {
		assertMinioArtifactProfile(t, storageClassName, manifests)
		return true
	}
	if isObjectStorageProfile(profile) || storageClassName == "" {
		return false
	}
	manifest, ok := manifests[storageClassName]
	if !ok {
		t.Fatalf("%s storage_class_name %q has no matching deploy/hpc/storage StorageClass manifest", id, storageClassName)
	}
	assertStorageClassManifest(t, id, storageClassName, manifest)
	return false
}

func assertMinioArtifactProfile(t *testing.T, storageClassName string, manifests map[string]map[string]any) {
	t.Helper()
	if storageClassName != "" {
		t.Fatalf("minio-artifact storage_class_name = %q, want empty object profile", storageClassName)
	}
	if _, ok := manifests["minio-artifact"]; ok {
		t.Fatalf("minio-artifact unexpectedly has a StorageClass manifest")
	}
}

func assertStorageClassManifest(t *testing.T, profileID, storageClassName string, manifest map[string]any) {
	t.Helper()
	if got := profileText(manifest, "apiVersion"); got != "storage.k8s.io/v1" {
		t.Fatalf("%s apiVersion = %q, want storage.k8s.io/v1", storageClassName, got)
	}
	if got := profileText(manifest, "kind"); got != "StorageClass" {
		t.Fatalf("%s kind = %q, want StorageClass", storageClassName, got)
	}
	metadata, _ := manifest["metadata"].(map[string]any)
	if got := profileText(metadata, "name"); got != storageClassName {
		t.Fatalf("%s metadata.name = %q, want %q", profileID, got, storageClassName)
	}
	labels, _ := metadata["labels"].(map[string]any)
	if got := profileText(labels, "nexuspaas.io/storage-profile"); got != profileID {
		t.Fatalf("%s nexuspaas.io/storage-profile = %q, want %q", storageClassName, got, profileID)
	}
}

func loadHPCStorageClassManifests(t *testing.T) map[string]map[string]any {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve test file path")
	}
	manifestDir := filepath.Join(filepath.Dir(file), "..", "..", "..", "deploy", "hpc", "storage")
	files, err := filepath.Glob(filepath.Join(manifestDir, "*.yaml"))
	if err != nil {
		t.Fatalf("glob hpc storage manifests: %v", err)
	}
	if len(files) == 0 {
		t.Fatalf("no hpc storage manifests found in %s", manifestDir)
	}

	manifests := map[string]map[string]any{}
	for _, path := range files {
		addStorageClassManifestsFromFile(t, manifests, path)
	}
	return manifests
}

func addStorageClassManifestsFromFile(t *testing.T, manifests map[string]map[string]any, path string) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	decoder := yaml.NewYAMLOrJSONDecoder(bytes.NewReader(raw), 4096)
	for {
		obj, done := decodeManifestObject(t, decoder, path)
		if done {
			return
		}
		if len(obj) == 0 || profileText(obj, "kind") != "StorageClass" {
			continue
		}
		addStorageClassManifest(t, manifests, path, obj)
	}
}

func decodeManifestObject(t *testing.T, decoder *yaml.YAMLOrJSONDecoder, path string) (map[string]any, bool) {
	t.Helper()
	var obj map[string]any
	if err := decoder.Decode(&obj); err != nil {
		if err == io.EOF {
			return nil, true
		}
		t.Fatalf("decode %s: %v", path, err)
	}
	return obj, false
}

func addStorageClassManifest(t *testing.T, manifests map[string]map[string]any, path string, obj map[string]any) {
	t.Helper()
	metadata, _ := obj["metadata"].(map[string]any)
	name := profileText(metadata, "name")
	if name == "" {
		t.Fatalf("%s StorageClass missing metadata.name", path)
	}
	if _, exists := manifests[name]; exists {
		t.Fatalf("duplicate StorageClass manifest for %s", name)
	}
	manifests[name] = obj
}

func isObjectStorageProfile(profile map[string]any) bool {
	return profileText(profile, "access_mode") == "object" ||
		profileText(profile, "tier") == "object-artifact" ||
		profileText(profile, "provider") == "minio"
}

func profileText(data map[string]any, field string) string {
	if data == nil {
		return ""
	}
	value, _ := data[field].(string)
	return strings.TrimSpace(value)
}
