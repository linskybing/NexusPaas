package schedulerquota

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/platform"
)

const (
	orgProjectServiceName         = "org-project-service"
	bindProjectPlanPathTemplate   = "/internal/org-project/projects/{project_id}/plan"
	clearPlanBindingsPathTemplate = "/internal/org-project/plans/{plan_id}/project-bindings"
)

// errProjectNotFound is returned by BindPlan when the owner reports the project
// does not exist, so bindPlanToProject can surface a 404 to the caller.
var errProjectNotFound = errors.New("project not found")

// bindPlanRequest is the body sent to the org-project bind contract.
type bindPlanRequest struct {
	PlanID string `json:"plan_id"`
}

// orgProjectBindingClient applies and clears project↔plan bindings through the
// org-project-owned internal contract. scheduler-quota uses it instead of writing
// org-project:projects directly so the project aggregate stays owned by
// org-project-service (problem.md #2). It mirrors workloadEvictionClient.
type orgProjectBindingClient interface {
	BindPlan(ctx context.Context, projectID, planID string) error
	ClearPlanBindings(ctx context.Context, planID string) error
}

type httpOrgProjectBindingClient struct {
	baseURL string
	apiKey  string
	client  *http.Client
}

type localOrgProjectBindingClient struct {
	app *platform.App
}

// newOrgProjectBindingClient returns a co-hosted (in-process) client when this
// process also hosts org-project-service, otherwise a remote HTTP client targeting
// the configured org-project URL. It mirrors newWorkloadEvictionClient.
func newOrgProjectBindingClient(app *platform.App) (orgProjectBindingClient, error) {
	if app.Config.AllowsService(orgProjectServiceName) {
		return localOrgProjectBindingClient{app: app}, nil
	}
	baseURL := strings.TrimSpace(app.Config.ServiceURLs[orgProjectServiceName])
	if baseURL == "" {
		return nil, fmt.Errorf("org-project-service URL is not configured")
	}
	if strings.TrimSpace(app.Config.ServiceAPIKey) == "" {
		return nil, fmt.Errorf("service API key is not configured")
	}
	timeout := app.Config.AdapterTimeout
	if timeout <= 0 {
		timeout = 2 * time.Second
	}
	return httpOrgProjectBindingClient{
		baseURL: baseURL,
		apiKey:  app.Config.ServiceAPIKey,
		client:  &http.Client{Timeout: timeout},
	}, nil
}

func bindPlanPath(projectID string) string {
	return strings.ReplaceAll(bindProjectPlanPathTemplate, "{project_id}", url.PathEscape(projectID))
}

func clearPlanBindingsPath(planID string) string {
	return strings.ReplaceAll(clearPlanBindingsPathTemplate, "{plan_id}", url.PathEscape(planID))
}

func (c httpOrgProjectBindingClient) BindPlan(ctx context.Context, projectID, planID string) error {
	body, err := json.Marshal(bindPlanRequest{PlanID: planID})
	if err != nil {
		return err
	}
	endpoint, err := c.endpoint(bindPlanPath(projectID))
	if err != nil {
		return err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPut, endpoint, bytes.NewReader(body))
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
	return bindStatusError("bind", resp.StatusCode)
}

func (c httpOrgProjectBindingClient) ClearPlanBindings(ctx context.Context, planID string) error {
	endpoint, err := c.endpoint(clearPlanBindingsPath(planID))
	if err != nil {
		return err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodDelete, endpoint, nil)
	if err != nil {
		return err
	}
	httpReq.Header.Set(headerServiceKey, c.apiKey)
	resp, err := c.client.Do(httpReq)
	if err != nil {
		return err
	}
	defer drainAndClose(resp)
	return clearStatusError(resp.StatusCode)
}

func (c httpOrgProjectBindingClient) endpoint(requestPath string) (string, error) {
	parsed, err := url.Parse(c.baseURL)
	if err != nil {
		return "", err
	}
	if !parsed.IsAbs() || parsed.Host == "" {
		return "", fmt.Errorf("org-project service URL must be absolute")
	}
	parsed.Path = path.Join(parsed.Path, requestPath)
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String(), nil
}

func (c localOrgProjectBindingClient) BindPlan(ctx context.Context, projectID, planID string) error {
	body, err := json.Marshal(bindPlanRequest{PlanID: planID})
	if err != nil {
		return err
	}
	httpReq := httptest.NewRequest(http.MethodPut, bindPlanPath(projectID), bytes.NewReader(body)).WithContext(ctx)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set(headerServiceKey, c.app.Config.ServiceAPIKey)
	rec := httptest.NewRecorder()
	c.app.ServeHTTP(rec, httpReq)
	return bindStatusError("bind", rec.Code)
}

func (c localOrgProjectBindingClient) ClearPlanBindings(ctx context.Context, planID string) error {
	httpReq := httptest.NewRequest(http.MethodDelete, clearPlanBindingsPath(planID), nil).WithContext(ctx)
	httpReq.Header.Set(headerServiceKey, c.app.Config.ServiceAPIKey)
	rec := httptest.NewRecorder()
	c.app.ServeHTTP(rec, httpReq)
	return clearStatusError(rec.Code)
}

// drainAndClose discards any remaining response body before closing so the HTTP
// transport can reuse the keep-alive connection (review Finding 5). The contract
// responses are small; the bound caps a misbehaving upstream.
func drainAndClose(resp *http.Response) {
	if resp == nil || resp.Body == nil {
		return
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
	_ = resp.Body.Close()
}

// bindStatusError maps the bind contract status to an error. 404 means the
// project is unknown, surfaced as errProjectNotFound.
func bindStatusError(op string, status int) error {
	switch status {
	case http.StatusOK:
		return nil
	case http.StatusNotFound:
		return errProjectNotFound
	default:
		return fmt.Errorf("org-project %s contract returned HTTP %d", op, status)
	}
}

// clearStatusError maps the clear contract status to an error. Clearing never
// targets a single project, so a non-200 (incl. 404 when the contract is closed)
// is a plain failure, not errProjectNotFound.
func clearStatusError(status int) error {
	if status == http.StatusOK {
		return nil
	}
	return fmt.Errorf("org-project clear plan bindings contract returned HTTP %d", status)
}
