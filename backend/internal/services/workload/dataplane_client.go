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

const dataPlanePlanPathTemplate = "/internal/storage/projects/{project_id}/data-plane-plan"

type dataPlanePlanResolver func(context.Context, string, dataPlanePlanRequest) (dataPlanePlan, error)

type dataPlanePlanRequest struct {
	JobID          string                   `json:"job_id,omitempty"`
	UserID         string                   `json:"user_id"`
	Namespace      string                   `json:"namespace"`
	DatasetSources []dataPlaneDatasetSource `json:"dataset_sources,omitempty"`
	ScratchProfile string                   `json:"scratch_profile,omitempty"`
	Checkpoint     dataPlaneCheckpointSpec  `json:"checkpoint,omitempty"`
}

type dataPlaneDatasetSource struct {
	StorageBindingID string `json:"storage_binding_id"`
	Mode             string `json:"mode,omitempty"`
	CacheKey         string `json:"cache_key,omitempty"`
}

type dataPlaneCheckpointSpec struct {
	FlushTargetProfile string `json:"flush_target_profile,omitempty"`
	WritePolicy        string `json:"write_policy,omitempty"`
	RetainLocalLastN   int    `json:"retain_local_last_n,omitempty"`
}

type dataPlanePlan struct {
	ProjectID         string                      `json:"project_id"`
	JobID             string                      `json:"job_id"`
	Namespace         string                      `json:"namespace"`
	Scratch           dataPlaneScratchPlan        `json:"scratch"`
	StageInOperations []dataPlaneStageInOperation `json:"stage_in_operations"`
	Checkpoint        dataPlaneCheckpointPlan     `json:"checkpoint"`
}

type dataPlaneScratchPlan struct {
	ProfileID        string `json:"profile_id"`
	StorageClassName string `json:"storage_class_name"`
	VolumeName       string `json:"volume_name"`
	ClaimName        string `json:"claim_name"`
	MountPath        string `json:"mount_path"`
	AccessMode       string `json:"access_mode"`
}

type dataPlaneStageInOperation struct {
	StorageBindingID string `json:"storage_binding_id"`
	CacheKey         string `json:"cache_key"`
	CacheHit         bool   `json:"cache_hit"`
	SourceNamespace  string `json:"source_namespace"`
	SourcePVC        string `json:"source_pvc"`
	TargetPVC        string `json:"target_pvc"`
	VolumeName       string `json:"volume_name"`
	SourcePath       string `json:"source_path"`
	ScratchPath      string `json:"scratch_path"`
}

type dataPlaneCheckpointPlan struct {
	FlushTargetProfileID string `json:"flush_target_profile_id"`
	StorageClassName     string `json:"storage_class_name"`
	WritePolicy          string `json:"write_policy"`
	LocalPath            string `json:"local_path"`
	FlushTargetPath      string `json:"flush_target_path"`
	RetainLocalLastN     int    `json:"retain_local_last_n"`
}

type internalDataPlanePlanClient struct {
	client platform.InternalJSONClient
}

func newDataPlanePlanClient(app *platform.App) (dataPlanePlanResolver, error) {
	if app == nil {
		return nil, fmt.Errorf("data plane plan client is not configured")
	}
	if !app.Config.AllowsService(storageServiceName) {
		if strings.TrimSpace(app.Config.ServiceURLs[storageServiceName]) == "" {
			return nil, fmt.Errorf("storage-service URL is not configured")
		}
		if strings.TrimSpace(app.Config.ServiceAPIKey) == "" {
			return nil, fmt.Errorf("service API key is not configured")
		}
	}
	client := internalDataPlanePlanClient{client: platform.NewInternalJSONClient(app, storageServiceName)}
	return client.Resolve, nil
}

func (c internalDataPlanePlanClient) Resolve(ctx context.Context, projectID string, req dataPlanePlanRequest) (dataPlanePlan, error) {
	var plan dataPlanePlan
	resp, err := c.client.Do(ctx, platform.InternalJSONRequest{
		Method:   http.MethodPost,
		Path:     dataPlanePlanPath(projectID),
		Body:     req,
		Response: &plan,
	})
	if err != nil {
		return dataPlanePlan{}, err
	}
	if resp.StatusCode != http.StatusOK {
		if resp.StatusCode >= http.StatusBadRequest && resp.StatusCode < http.StatusInternalServerError {
			return dataPlanePlan{}, fmt.Errorf("%w: data plane plan returned HTTP %d", cluster.ErrInvalidManifest, resp.StatusCode)
		}
		return dataPlanePlan{}, fmt.Errorf("data plane plan returned HTTP %d", resp.StatusCode)
	}
	if resp.EnvelopeError != nil {
		return dataPlanePlan{}, errors.New(resp.EnvelopeError.Message)
	}
	return plan, nil
}

func dataPlanePlanPath(projectID string) string {
	return strings.ReplaceAll(dataPlanePlanPathTemplate, "{project_id}", url.PathEscape(projectID))
}
