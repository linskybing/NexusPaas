package workload

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/linskybing/nexuspaas/backend/internal/platform"
)

const (
	schedulerServiceName   = "scheduler-quota-service"
	schedulerAdmissionPath = "/api/v1/internal/scheduler/admission"
)

type schedulerAdmissionResult struct {
	StatusCode int
	Data       map[string]any
}

type internalSchedulerAdmissionClient struct {
	app    *platform.App
	client platform.InternalJSONClient
}

func newSchedulerAdmissionClient(app *platform.App) (internalSchedulerAdmissionClient, error) {
	if app == nil {
		return internalSchedulerAdmissionClient{}, fmt.Errorf("scheduler admission client is not configured")
	}
	if !app.Config.AllowsService(schedulerServiceName) {
		if strings.TrimSpace(app.Config.ServiceURLs[schedulerServiceName]) == "" {
			return internalSchedulerAdmissionClient{}, fmt.Errorf("scheduler service URL is not configured")
		}
		if strings.TrimSpace(app.Config.ServiceAPIKey) == "" {
			return internalSchedulerAdmissionClient{}, fmt.Errorf("service API key is not configured")
		}
	}
	return internalSchedulerAdmissionClient{
		app:    app,
		client: platform.NewInternalJSONClient(app, schedulerServiceName),
	}, nil
}

func (c internalSchedulerAdmissionClient) Review(ctx context.Context, headers http.Header, payload map[string]any) (schedulerAdmissionResult, error) {
	data := map[string]any{}
	resp, err := c.client.Do(ctx, platform.InternalJSONRequest{
		Method:        http.MethodPost,
		Path:          schedulerAdmissionPath,
		Headers:       schedulerAdmissionHeaders(c.app, headers),
		Body:          payload,
		Response:      &data,
		ResponseLimit: 1 << 20,
	})
	if err != nil {
		return schedulerAdmissionResult{}, err
	}
	if data == nil {
		data = map[string]any{}
	}
	if resp.EnvelopeError != nil && data["reason"] == nil {
		data["reason"] = resp.EnvelopeError.Message
	}
	return schedulerAdmissionResult{StatusCode: resp.StatusCode, Data: data}, nil
}

func schedulerAdmissionHeaders(app *platform.App, src http.Header) http.Header {
	dst := http.Header{}
	copySchedulerAdmissionContextHeaders(dst, src)
	if app != nil {
		if serviceKey := strings.TrimSpace(app.Config.ServiceAPIKey); serviceKey != "" {
			dst.Set("X-API-Key", serviceKey)
		}
	}
	return dst
}

func copySchedulerAdmissionContextHeaders(dst, src http.Header) {
	for _, key := range []string{"X-Request-ID", "X-Trace-ID", "Traceparent", "Idempotency-Key"} {
		if value := strings.TrimSpace(src.Get(key)); value != "" {
			dst.Set(key, value)
		}
	}
}
