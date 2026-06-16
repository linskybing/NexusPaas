package workload

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/linskybing/nexuspaas/backend/internal/platform"
	"github.com/linskybing/nexuspaas/backend/internal/platform/cluster"
)

func TestStorageMountPlanClientRemoteSendsServiceKey(t *testing.T) {
	var gotKey, gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotKey = r.Header.Get(storageMountPlanServiceHeader)
		gotPath = r.URL.Path
		platform.WriteJSON(w, r, http.StatusOK, storageMountPlan{
			ProjectID: "P1",
			UserID:    "U1",
			Namespace: "proj-p1",
			ManifestMounts: []storageMountPlanMount{{
				Name: "datasets", ClaimName: "datasets-pvc", MountPath: "/mnt/datasets", ReadOnly: true,
			}},
			PVCShareOperations: []storageMountPlanShareOp{{
				SourceNamespace: "group-g1-storage", SourcePVC: "datasets-pvc", TargetPVC: "datasets-pvc",
			}},
		})
	}))
	defer server.Close()

	client, err := newStorageMountPlanClient(platform.NewApp(platform.Config{
		ServiceName:   serviceName,
		ServiceAPIKey: "service-key",
		ServiceURLs:   map[string]string{storageServiceName: server.URL + "/storage-root"},
	}))
	if err != nil {
		t.Fatal(err)
	}
	plan, err := client.Resolve(context.Background(), "P1", storageMountPlanRequest{
		UserID:    "U1",
		Namespace: "proj-p1",
		Mounts:    []storageMountPlanSelector{{PVCID: "datasets-pvc", MountPath: "/mnt/datasets", ReadOnly: true}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if gotKey != "service-key" {
		t.Fatalf("service key header = %q, want service-key", gotKey)
	}
	if gotPath != "/storage-root/internal/storage/projects/P1/mount-plan" {
		t.Fatalf("request path = %q, want storage-root mount-plan path", gotPath)
	}
	if len(plan.ManifestMounts) != 1 || plan.ManifestMounts[0].ClaimName != "datasets-pvc" {
		t.Fatalf("plan = %#v, want decoded manifest mount", plan)
	}
}

func TestStorageMountPlanClientWrongKeyIsPermanentDispatchFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get(storageMountPlanServiceHeader) != "service-key" {
			platform.WriteError(w, r, http.StatusUnauthorized, "unauthorized", "service authentication is required")
			return
		}
		platform.WriteJSON(w, r, http.StatusOK, storageMountPlan{})
	}))
	defer server.Close()

	client, err := newStorageMountPlanClient(platform.NewApp(platform.Config{
		ServiceName:   serviceName,
		ServiceAPIKey: "wrong-key",
		ServiceURLs:   map[string]string{storageServiceName: server.URL},
	}))
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Resolve(context.Background(), "P1", storageMountPlanRequest{
		UserID: "U1",
		Mounts: []storageMountPlanSelector{{
			PVCID: "datasets-pvc",
		}},
	})
	if !errors.Is(err, cluster.ErrInvalidManifest) || !strings.Contains(err.Error(), "HTTP 401") {
		t.Fatalf("err = %v, want permanent 401 dispatch failure", err)
	}
}

func TestStorageMountPlanClientCoHostedUsesLocalRouter(t *testing.T) {
	app := platform.NewApp(platform.Config{ServiceName: "all", ServiceAPIKey: "service-key"})
	app.RegisterService(platform.ServiceSpec{
		Name: storageServiceName,
		Routes: []platform.RouteSpec{{
			Method:       http.MethodPost,
			Pattern:      storageMountPlanPathTemplate,
			Resource:     "mount_plans",
			Action:       "resolve",
			PolicyBypass: true,
		}},
	})
	app.RegisterCustomHandler(http.MethodPost, storageMountPlanPathTemplate, func(_ *platform.App, r *http.Request, _ platform.RouteSpec) (int, any, *platform.Degraded) {
		if r.Header.Get(storageMountPlanServiceHeader) != "service-key" {
			return http.StatusUnauthorized, map[string]any{"message": "service key missing"}, nil
		}
		var body storageMountPlanRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			return http.StatusBadRequest, map[string]any{"message": err.Error()}, nil
		}
		return http.StatusOK, storageMountPlan{
			ProjectID: r.PathValue("project_id"),
			UserID:    body.UserID,
			Namespace: body.Namespace,
			ManifestMounts: []storageMountPlanMount{{
				Name: "scratch", ClaimName: body.Mounts[0].PVCID, MountPath: body.Mounts[0].MountPath,
			}},
		}, nil
	})

	client, err := newStorageMountPlanClient(app)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := client.Resolve(context.Background(), "P1", storageMountPlanRequest{
		UserID: "U1",
		Mounts: []storageMountPlanSelector{{
			PVCID: "scratch-pvc", MountPath: "/scratch",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if plan.ProjectID != "P1" || len(plan.ManifestMounts) != 1 || plan.ManifestMounts[0].ClaimName != "scratch-pvc" {
		t.Fatalf("plan = %#v, want local router response", plan)
	}
}

func TestStorageMountPlanClientRequiresRemoteConfig(t *testing.T) {
	_, err := newStorageMountPlanClient(platform.NewApp(platform.Config{ServiceName: serviceName}))
	if err == nil || !strings.Contains(err.Error(), "storage-service URL") {
		t.Fatalf("err = %v, want missing storage-service URL", err)
	}
}

func TestStorageMountSourceGuardNoDirectStorageResourceAccess(t *testing.T) {
	forbidden := regexp.MustCompile(`\b(groupStorageResource|projectBindingsResource|storagePermissionsResource|projectPermissionsResource|storagePoliciesResource|fastTransfersResource|userStorageResource)\b|storage-service:(group_storage|storage_permissions|storage_access_policies|storage_bindings|project_storage_permissions|fast_transfers|user_storage)`)
	var violations []string
	err := filepath.WalkDir(".", func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for i, line := range strings.Split(string(raw), "\n") {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "//") || !forbidden.MatchString(trimmed) {
				continue
			}
			violations = append(violations, path+":"+strings.TrimSpace(trimmed))
			_ = i
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(violations) > 0 {
		t.Fatalf("workload-service must use storage mount-plan contract, not storage-owned records:\n%s", strings.Join(violations, "\n"))
	}
}
