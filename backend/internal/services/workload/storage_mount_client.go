package workload

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/linskybing/nexuspaas/backend/internal/platform"
	"github.com/linskybing/nexuspaas/backend/internal/platform/cluster"
)

const (
	storageServiceName           = "storage-service"
	storageMountPlanPathTemplate = "/internal/storage/projects/{project_id}/mount-plan"
)

type storageMountPlanResolver func(context.Context, string, storageMountPlanRequest) (storageMountPlan, error)

type storageMountPlanRequest struct {
	UserID    string                     `json:"user_id"`
	Namespace string                     `json:"namespace"`
	Mounts    []storageMountPlanSelector `json:"mounts"`
}

type storageMountPlanSelector struct {
	PVCID     string `json:"pvc_id"`
	Name      string `json:"name,omitempty"`
	MountPath string `json:"mount_path,omitempty"`
	ReadOnly  bool   `json:"read_only,omitempty"`
	SubPath   string `json:"sub_path,omitempty"`
}

type storageMountPlan struct {
	ProjectID          string                    `json:"project_id"`
	UserID             string                    `json:"user_id"`
	Namespace          string                    `json:"namespace"`
	ManifestMounts     []storageMountPlanMount   `json:"manifest_mounts"`
	PVCShareOperations []storageMountPlanShareOp `json:"pvc_share_operations"`
}

type storageMountPlanMount struct {
	Name      string `json:"name"`
	ClaimName string `json:"claim_name"`
	MountPath string `json:"mount_path"`
	ReadOnly  bool   `json:"read_only,omitempty"`
	SubPath   string `json:"sub_path,omitempty"`
}

type storageMountPlanShareOp struct {
	SourceNamespace string `json:"source_namespace"`
	SourcePVC       string `json:"source_pvc"`
	TargetPVC       string `json:"target_pvc"`
}

type internalStorageMountPlanClient struct {
	client platform.InternalJSONClient
}

func newStorageMountPlanClient(app *platform.App) (storageMountPlanResolver, error) {
	if app == nil {
		return nil, fmt.Errorf("storage mount plan client is not configured")
	}
	if !app.Config.AllowsService(storageServiceName) {
		if strings.TrimSpace(app.Config.ServiceURLs[storageServiceName]) == "" {
			return nil, fmt.Errorf("storage-service URL is not configured")
		}
		if strings.TrimSpace(app.Config.ServiceAPIKey) == "" {
			return nil, fmt.Errorf("service API key is not configured")
		}
	}
	client := internalStorageMountPlanClient{client: platform.NewInternalJSONClient(app, storageServiceName)}
	return client.Resolve, nil
}

func (c internalStorageMountPlanClient) Resolve(ctx context.Context, projectID string, req storageMountPlanRequest) (storageMountPlan, error) {
	var plan storageMountPlan
	resp, err := c.client.Do(ctx, platform.InternalJSONRequest{
		Method:   http.MethodPost,
		Path:     storageMountPlanPath(projectID),
		Body:     req,
		Response: &plan,
	})
	if err != nil {
		return storageMountPlan{}, err
	}
	if resp.StatusCode != http.StatusOK {
		if resp.StatusCode >= http.StatusBadRequest && resp.StatusCode < http.StatusInternalServerError {
			return storageMountPlan{}, fmt.Errorf("%w: storage mount plan returned HTTP %d", cluster.ErrInvalidManifest, resp.StatusCode)
		}
		return storageMountPlan{}, fmt.Errorf("storage mount plan returned HTTP %d", resp.StatusCode)
	}
	if resp.EnvelopeError != nil {
		return storageMountPlan{}, errors.New(resp.EnvelopeError.Message)
	}
	return plan, nil
}

func storageMountPlanPath(projectID string) string {
	return strings.ReplaceAll(storageMountPlanPathTemplate, "{project_id}", url.PathEscape(projectID))
}
