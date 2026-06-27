package workload

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/linskybing/nexuspaas/backend/internal/contracts"
	"github.com/linskybing/nexuspaas/backend/internal/platform"
)

const schedulerReservationsPath = "/api/v1/internal/quota/reservations"

type schedulerReservationResult struct {
	StatusCode int
	Record     contracts.Record[map[string]any]
}

type internalSchedulerReservationClient struct {
	app    *platform.App
	client platform.InternalJSONClient
}

func newSchedulerReservationClient(app *platform.App) (internalSchedulerReservationClient, error) {
	if app == nil {
		return internalSchedulerReservationClient{}, fmt.Errorf("scheduler reservation client is not configured")
	}
	if !app.Config.AllowsService(schedulerServiceName) {
		if strings.TrimSpace(app.Config.ServiceURLs[schedulerServiceName]) == "" {
			return internalSchedulerReservationClient{}, fmt.Errorf("scheduler service URL is not configured")
		}
		if strings.TrimSpace(app.Config.ServiceAPIKey) == "" {
			return internalSchedulerReservationClient{}, fmt.Errorf("service API key is not configured")
		}
	}
	return internalSchedulerReservationClient{
		app:    app,
		client: platform.NewInternalJSONClient(app, schedulerServiceName),
	}, nil
}

func (c internalSchedulerReservationClient) Reserve(ctx context.Context, headers http.Header, payload map[string]any) (schedulerReservationResult, error) {
	return c.do(ctx, http.MethodPost, schedulerReservationsPath, headers, payload, "reserve")
}

func (c internalSchedulerReservationClient) Commit(ctx context.Context, headers http.Header, reservationID string) (schedulerReservationResult, error) {
	return c.do(ctx, http.MethodPost, schedulerReservationTransitionPath(reservationID, "commit"), headers, map[string]any{}, "commit")
}

func (c internalSchedulerReservationClient) Release(ctx context.Context, headers http.Header, reservationID string) (schedulerReservationResult, error) {
	return c.do(ctx, http.MethodPost, schedulerReservationTransitionPath(reservationID, "release"), headers, map[string]any{}, "release")
}

func (c internalSchedulerReservationClient) do(ctx context.Context, method, path string, headers http.Header, body map[string]any, action string) (schedulerReservationResult, error) {
	record := contracts.Record[map[string]any]{}
	resp, err := c.client.Do(ctx, platform.InternalJSONRequest{
		Method:        method,
		Path:          path,
		Headers:       schedulerAdmissionHeaders(c.app, headers),
		Body:          body,
		Response:      &record,
		ResponseLimit: 1 << 20,
	})
	if err != nil {
		return schedulerReservationResult{}, err
	}
	if resp.StatusCode >= http.StatusBadRequest {
		message := fmt.Sprintf("scheduler reservation %s failed with status %d", action, resp.StatusCode)
		if resp.EnvelopeError != nil && strings.TrimSpace(resp.EnvelopeError.Message) != "" {
			message = resp.EnvelopeError.Message
		}
		return schedulerReservationResult{StatusCode: resp.StatusCode, Record: record}, fmt.Errorf("%s", message)
	}
	return schedulerReservationResult{StatusCode: resp.StatusCode, Record: record}, nil
}

func schedulerReservationTransitionPath(reservationID, action string) string {
	return schedulerReservationsPath + "/" + url.PathEscape(strings.TrimSpace(reservationID)) + "/" + action
}
