package schedulerquota

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/platform"
)

const workloadEvictJobPathTemplate = "/internal/workload/jobs/{id}/evict"

// workloadEvictRequest is the body sent to the workload eviction contract.
type workloadEvictRequest struct {
	Reason string `json:"reason"`
}

// workloadEvictionClient marks a workload job evicted through the workload-owned
// internal contract. The plan-window reaper uses it instead of writing job records
// directly so job state stays owned by workload-service.
type workloadEvictionClient interface {
	Evict(ctx context.Context, jobID string, req workloadEvictRequest) error
}

type httpWorkloadEvictionClient struct {
	baseURL string
	apiKey  string
	client  *http.Client
}

type localWorkloadEvictionClient struct {
	app *platform.App
}

// newWorkloadEvictionClient returns a co-hosted (in-process) client when this
// process also hosts workload-service, otherwise a remote HTTP client targeting the
// configured workload-service URL. It mirrors newWorkloadPreemptionClient.
func newWorkloadEvictionClient(app *platform.App) (workloadEvictionClient, error) {
	if app.Config.AllowsService(workloadServiceName) {
		return localWorkloadEvictionClient{app: app}, nil
	}
	baseURL := strings.TrimSpace(app.Config.ServiceURLs[workloadServiceName])
	if baseURL == "" {
		return nil, fmt.Errorf("workload-service URL is not configured")
	}
	if strings.TrimSpace(app.Config.ServiceAPIKey) == "" {
		return nil, fmt.Errorf("service API key is not configured")
	}
	timeout := app.Config.AdapterTimeout
	if timeout <= 0 {
		timeout = 2 * time.Second
	}
	return httpWorkloadEvictionClient{
		baseURL: baseURL,
		apiKey:  app.Config.ServiceAPIKey,
		client:  &http.Client{Timeout: timeout},
	}, nil
}

func (c httpWorkloadEvictionClient) Evict(ctx context.Context, jobID string, req workloadEvictRequest) error {
	body, err := json.Marshal(req)
	if err != nil {
		return err
	}
	requestPath := strings.ReplaceAll(workloadEvictJobPathTemplate, "{id}", url.PathEscape(jobID))
	endpoint, err := c.endpoint(requestPath)
	if err != nil {
		return err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set(headerServiceKey, c.apiKey)
	resp, err := c.client.Do(httpReq)
	if err != nil {
		return err
	}
	defer drainAndClose(resp)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("workload evict status returned HTTP %d", resp.StatusCode)
	}
	return nil
}

func (c httpWorkloadEvictionClient) endpoint(requestPath string) (string, error) {
	parsed, err := url.Parse(c.baseURL)
	if err != nil {
		return "", err
	}
	if !parsed.IsAbs() || parsed.Host == "" {
		return "", fmt.Errorf("workload service URL must be absolute")
	}
	parsed.Path = path.Join(parsed.Path, requestPath)
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String(), nil
}

func (c localWorkloadEvictionClient) Evict(ctx context.Context, jobID string, req workloadEvictRequest) error {
	body, err := json.Marshal(req)
	if err != nil {
		return err
	}
	target := strings.ReplaceAll(workloadEvictJobPathTemplate, "{id}", url.PathEscape(jobID))
	httpReq := httptest.NewRequest(http.MethodPost, target, bytes.NewReader(body)).WithContext(ctx)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set(headerServiceKey, c.app.Config.ServiceAPIKey)
	rec := httptest.NewRecorder()
	c.app.ServeHTTP(rec, httpReq)
	if rec.Code != http.StatusOK {
		return fmt.Errorf("workload evict status returned HTTP %d", rec.Code)
	}
	return nil
}
