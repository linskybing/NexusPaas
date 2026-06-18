package schedulerquota

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/linskybing/nexuspaas/backend/internal/platform"
)

const workloadEvictJobPathTemplate = "/internal/workload/jobs/{id}/evict"

// workloadEvictRequest is the body sent to the workload eviction contract.
type workloadEvictRequest struct {
	Reason string `json:"reason"`
}

type workloadEvictFunc func(ctx context.Context, jobID string, req workloadEvictRequest) error

type internalWorkloadEvictionClient struct {
	client platform.InternalJSONClient
}

func newWorkloadEvictionClient(app *platform.App) (workloadEvictFunc, error) {
	if app == nil {
		return nil, fmt.Errorf("workload eviction client is not configured")
	}
	if !app.Config.AllowsService(workloadServiceName) {
		if strings.TrimSpace(app.Config.ServiceURLs[workloadServiceName]) == "" {
			return nil, fmt.Errorf("workload-service URL is not configured")
		}
		if strings.TrimSpace(app.Config.ServiceAPIKey) == "" {
			return nil, fmt.Errorf("service API key is not configured")
		}
	}
	client := internalWorkloadEvictionClient{client: platform.NewInternalJSONClient(app, workloadServiceName)}
	return client.Evict, nil
}

func (c internalWorkloadEvictionClient) Evict(ctx context.Context, jobID string, req workloadEvictRequest) error {
	resp, err := c.client.Do(ctx, platform.InternalJSONRequest{
		Method: http.MethodPost,
		Path:   strings.ReplaceAll(workloadEvictJobPathTemplate, "{id}", url.PathEscape(jobID)),
		Body:   req,
	})
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("workload evict status returned HTTP %d", resp.StatusCode)
	}
	return nil
}
