package schedulerquota

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

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

type internalOrgProjectBindingClient struct {
	client platform.InternalJSONClient
}

// internalOrgProjectBindingClient applies and clears project-plan bindings
// through the org-project-owned internal contract.
func newOrgProjectBindingClient(app *platform.App) (internalOrgProjectBindingClient, error) {
	if app == nil {
		return internalOrgProjectBindingClient{}, fmt.Errorf("org-project binding client is not configured")
	}
	if !app.Config.AllowsService(orgProjectServiceName) {
		if strings.TrimSpace(app.Config.ServiceURLs[orgProjectServiceName]) == "" {
			return internalOrgProjectBindingClient{}, fmt.Errorf("org-project-service URL is not configured")
		}
		if strings.TrimSpace(app.Config.ServiceAPIKey) == "" {
			return internalOrgProjectBindingClient{}, fmt.Errorf("service API key is not configured")
		}
	}
	return internalOrgProjectBindingClient{client: platform.NewInternalJSONClient(app, orgProjectServiceName)}, nil
}

func bindPlanPath(projectID string) string {
	return strings.ReplaceAll(bindProjectPlanPathTemplate, "{project_id}", url.PathEscape(projectID))
}

func clearPlanBindingsPath(planID string) string {
	return strings.ReplaceAll(clearPlanBindingsPathTemplate, "{plan_id}", url.PathEscape(planID))
}

func (c internalOrgProjectBindingClient) BindPlan(ctx context.Context, projectID, planID string) error {
	resp, err := c.client.Do(ctx, platform.InternalJSONRequest{
		Method: http.MethodPut,
		Path:   bindPlanPath(projectID),
		Body:   bindPlanRequest{PlanID: planID},
	})
	if err != nil {
		return err
	}
	return bindStatusError("bind", resp.StatusCode)
}

func (c internalOrgProjectBindingClient) ClearPlanBindings(ctx context.Context, planID string) error {
	resp, err := c.client.Do(ctx, platform.InternalJSONRequest{
		Method: http.MethodDelete,
		Path:   clearPlanBindingsPath(planID),
	})
	if err != nil {
		return err
	}
	return clearStatusError(resp.StatusCode)
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
