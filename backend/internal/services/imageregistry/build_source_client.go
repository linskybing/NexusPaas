package imageregistry

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/linskybing/nexuspaas/backend/internal/platform"
)

// buildSourceAccessPathTemplate is storage-service's owning-service contract
// answering whether a user may read image-build sources from a project's
// storage. Mirrors workload-service's mount-plan consumer wiring.
const (
	storageServiceName            = "storage-service"
	buildSourceAccessPathTemplate = "/internal/storage/projects/{project_id}/build-source-access"
)

type buildSourceAccessRequest struct {
	UserID      string `json:"user_id"`
	StoragePath string `json:"storage_path"`
}

type buildSourceAccessDecision struct {
	Allowed    bool   `json:"allowed"`
	Permission string `json:"permission"`
	PVCID      string `json:"pvc_id"`
}

type buildSourceAccessResolver func(ctx context.Context, projectID string, req buildSourceAccessRequest) (buildSourceAccessDecision, error)

func newBuildSourceAccessResolver(app *platform.App) buildSourceAccessResolver {
	client := platform.NewInternalJSONClient(app, storageServiceName)
	return func(ctx context.Context, projectID string, req buildSourceAccessRequest) (buildSourceAccessDecision, error) {
		var decision buildSourceAccessDecision
		resp, err := client.Do(ctx, platform.InternalJSONRequest{
			Method:   http.MethodPost,
			Path:     strings.ReplaceAll(buildSourceAccessPathTemplate, "{project_id}", url.PathEscape(projectID)),
			Body:     req,
			Response: &decision,
		})
		if err != nil {
			return buildSourceAccessDecision{}, err
		}
		if resp.StatusCode != http.StatusOK {
			if resp.EnvelopeError != nil {
				return buildSourceAccessDecision{}, errors.New(resp.EnvelopeError.Message)
			}
			return buildSourceAccessDecision{}, fmt.Errorf("build source access returned HTTP %d", resp.StatusCode)
		}
		return decision, nil
	}
}
