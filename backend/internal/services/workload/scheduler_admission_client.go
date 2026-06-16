package workload

import (
	"bytes"
	"context"
	"encoding/json"
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
	schedulerServiceName   = "scheduler-quota-service"
	schedulerAdmissionPath = "/api/v1/internal/scheduler/admission"
)

type schedulerAdmissionClient interface {
	Review(context.Context, http.Header, map[string]any) (schedulerAdmissionResult, error)
}

type schedulerAdmissionResult struct {
	StatusCode int
	Data       map[string]any
}

type localSchedulerAdmissionClient struct {
	app *platform.App
}

type httpSchedulerAdmissionClient struct {
	baseURL string
	apiKey  string
	client  *http.Client
}

func newSchedulerAdmissionClient(app *platform.App) (schedulerAdmissionClient, error) {
	if app == nil {
		return nil, fmt.Errorf("scheduler admission client is not configured")
	}
	if app.Config.AllowsService(schedulerServiceName) {
		return localSchedulerAdmissionClient{app: app}, nil
	}
	baseURL := strings.TrimSpace(app.Config.ServiceURLs[schedulerServiceName])
	if baseURL == "" {
		return nil, fmt.Errorf("scheduler service URL is not configured")
	}
	apiKey := strings.TrimSpace(app.Config.ServiceAPIKey)
	if apiKey == "" {
		return nil, fmt.Errorf("service API key is not configured")
	}
	return httpSchedulerAdmissionClient{
		baseURL: baseURL,
		apiKey:  apiKey,
		client:  &http.Client{Timeout: schedulerAdmissionTimeout(app.Config.AdapterTimeout)},
	}, nil
}

func (c localSchedulerAdmissionClient) Review(ctx context.Context, headers http.Header, payload map[string]any) (schedulerAdmissionResult, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return schedulerAdmissionResult{}, err
	}
	req := httptest.NewRequest(http.MethodPost, schedulerAdmissionPath, bytes.NewReader(body)).WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	copySchedulerAdmissionContextHeaders(req.Header, headers)
	if serviceKey := strings.TrimSpace(c.app.Config.ServiceAPIKey); serviceKey != "" {
		setSchedulerAdmissionServiceAuth(req.Header, serviceKey)
	}
	rec := httptest.NewRecorder()
	c.app.ServeHTTP(rec, req)
	data, err := decodeAdmissionResponse(rec.Body.Bytes())
	return schedulerAdmissionResult{StatusCode: rec.Code, Data: data}, err
}

func (c httpSchedulerAdmissionClient) Review(ctx context.Context, headers http.Header, payload map[string]any) (schedulerAdmissionResult, error) {
	endpoint, err := schedulerAdmissionEndpoint(c.baseURL)
	if err != nil {
		return schedulerAdmissionResult{}, err
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return schedulerAdmissionResult{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return schedulerAdmissionResult{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	copySchedulerAdmissionContextHeaders(req.Header, headers)
	setSchedulerAdmissionServiceAuth(req.Header, c.apiKey)
	client := c.client
	if client == nil {
		client = &http.Client{Timeout: schedulerAdmissionTimeout(0)}
	}
	resp, err := client.Do(req)
	if err != nil {
		return schedulerAdmissionResult{}, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return schedulerAdmissionResult{StatusCode: resp.StatusCode}, err
	}
	data, err := decodeAdmissionResponse(raw)
	return schedulerAdmissionResult{StatusCode: resp.StatusCode, Data: data}, err
}

func setSchedulerAdmissionServiceAuth(header http.Header, apiKey string) {
	header.Set("X-Service-Key", apiKey)
	header.Set("X-API-Key", apiKey)
}

func copySchedulerAdmissionContextHeaders(dst, src http.Header) {
	for _, key := range []string{"X-Request-ID", "X-Trace-ID", "Traceparent", "Idempotency-Key"} {
		if value := strings.TrimSpace(src.Get(key)); value != "" {
			dst.Set(key, value)
		}
	}
}

func decodeAdmissionResponse(raw []byte) (map[string]any, error) {
	var envelope struct {
		Data  json.RawMessage     `json:"data"`
		Error *platform.ErrorBody `json:"error"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return nil, err
	}
	data := map[string]any{}
	if len(envelope.Data) > 0 && string(envelope.Data) != "null" {
		if err := json.Unmarshal(envelope.Data, &data); err != nil {
			return nil, err
		}
	}
	if envelope.Error != nil && data["reason"] == nil {
		data["reason"] = envelope.Error.Message
	}
	return data, nil
}

func schedulerAdmissionEndpoint(baseURL string) (string, error) {
	baseURL = strings.TrimSpace(baseURL)
	if baseURL == "" {
		return "", fmt.Errorf("scheduler service URL is not configured")
	}
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return "", err
	}
	if !parsed.IsAbs() || parsed.Host == "" {
		return "", fmt.Errorf("scheduler service URL must be absolute")
	}
	parsed.Path = path.Join(parsed.Path, schedulerAdmissionPath)
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String(), nil
}

func schedulerAdmissionTimeout(timeout time.Duration) time.Duration {
	if timeout <= 0 {
		return 2 * time.Second
	}
	return timeout
}
