package workload

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/linskybing/nexuspaas/backend/internal/platform"
)

const schedulerPreemptionsPath = "/api/v1/internal/scheduler/preemptions"

type schedulerPreemptionResult struct {
	StatusCode int
	Data       map[string]any
}

type internalSchedulerPreemptionClient struct {
	app    *platform.App
	client platform.InternalJSONClient
}

func newSchedulerPreemptionClient(app *platform.App) (internalSchedulerPreemptionClient, error) {
	if app == nil {
		return internalSchedulerPreemptionClient{}, fmt.Errorf("scheduler preemption client is not configured")
	}
	if !app.Config.AllowsService(schedulerServiceName) {
		if strings.TrimSpace(app.Config.ServiceURLs[schedulerServiceName]) == "" {
			return internalSchedulerPreemptionClient{}, fmt.Errorf("scheduler service URL is not configured")
		}
		if strings.TrimSpace(app.Config.ServiceAPIKey) == "" {
			return internalSchedulerPreemptionClient{}, fmt.Errorf("service API key is not configured")
		}
	}
	return internalSchedulerPreemptionClient{
		app:    app,
		client: platform.NewInternalJSONClient(app, schedulerServiceName),
	}, nil
}

func (c internalSchedulerPreemptionClient) Preempt(ctx context.Context, headers http.Header, payload map[string]any) (schedulerPreemptionResult, error) {
	data := map[string]any{}
	resp, err := c.client.Do(ctx, platform.InternalJSONRequest{
		Method:        http.MethodPost,
		Path:          schedulerPreemptionsPath,
		Headers:       schedulerAdmissionHeaders(c.app, headers),
		Body:          payload,
		Response:      &data,
		ResponseLimit: 1 << 20,
	})
	if err != nil {
		return schedulerPreemptionResult{}, err
	}
	if data == nil {
		data = map[string]any{}
	}
	if resp.EnvelopeError != nil && data["reason"] == nil {
		data["reason"] = resp.EnvelopeError.Message
	}
	return schedulerPreemptionResult{StatusCode: resp.StatusCode, Data: data}, nil
}
