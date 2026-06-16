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
	workloadServiceName            = "workload-service"
	workloadPreemptionContextPath  = "/internal/workload/preemption-context"
	workloadPreemptJobPathTemplate = "/internal/workload/jobs/{id}/preempt"
	headerServiceKey               = "X-Service-Key"
)

type workloadPreemptionClient interface {
	Context(context.Context, preemptionRequest) (workloadPreemptionContext, error)
	Preempt(context.Context, string, workloadPreemptRequest) (workloadJobSnapshot, error)
}

type workloadPreemptionContext struct {
	Requester  *workloadJobSnapshot  `json:"requester,omitempty"`
	Candidates []workloadJobSnapshot `json:"candidates"`
}

type workloadJobSnapshot struct {
	ID                 string  `json:"id"`
	JobID              string  `json:"job_id"`
	ProjectID          string  `json:"project_id"`
	UserID             string  `json:"user_id"`
	QueueName          string  `json:"queue_name"`
	Namespace          string  `json:"namespace"`
	Status             string  `json:"status"`
	PriorityValue      int     `json:"priority_value"`
	Preemptible        bool    `json:"preemptible"`
	RequiredGPU        float64 `json:"required_gpu"`
	RequiredCPU        float64 `json:"required_cpu"`
	RequiredMemory     int     `json:"required_memory"`
	DeviceClassName    string  `json:"device_class_name"`
	GPUModel           string  `json:"gpu_model"`
	CreatedAt          string  `json:"created_at"`
	PreemptionRecordID string  `json:"preemption_record_id"`
}

type workloadPreemptRequest struct {
	PreemptionID   string         `json:"preemption_id"`
	RequesterJobID string         `json:"requester_job_id,omitempty"`
	Reason         string         `json:"reason"`
	Cleanup        map[string]any `json:"cleanup"`
}

type httpWorkloadPreemptionClient struct {
	baseURL string
	apiKey  string
	client  *http.Client
}

type localWorkloadPreemptionClient struct {
	app *platform.App
}

func newWorkloadPreemptionClient(app *platform.App) (workloadPreemptionClient, error) {
	if app.Config.AllowsService(workloadServiceName) {
		return localWorkloadPreemptionClient{app: app}, nil
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
	return httpWorkloadPreemptionClient{
		baseURL: baseURL,
		apiKey:  app.Config.ServiceAPIKey,
		client:  &http.Client{Timeout: timeout},
	}, nil
}

func (c httpWorkloadPreemptionClient) Context(ctx context.Context, req preemptionRequest) (workloadPreemptionContext, error) {
	endpoint, err := c.endpoint(workloadPreemptionContextPath, req.contextQuery())
	if err != nil {
		return workloadPreemptionContext{}, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return workloadPreemptionContext{}, err
	}
	httpReq.Header.Set(headerServiceKey, c.apiKey)
	raw, status, err := c.do(httpReq)
	if err != nil {
		return workloadPreemptionContext{}, err
	}
	if status != http.StatusOK {
		return workloadPreemptionContext{}, fmt.Errorf("workload preemption context returned HTTP %d", status)
	}
	var out workloadPreemptionContext
	if err := decodePreemptionEnvelope(raw, &out); err != nil {
		return workloadPreemptionContext{}, err
	}
	return out, nil
}

func (c httpWorkloadPreemptionClient) Preempt(ctx context.Context, id string, req workloadPreemptRequest) (workloadJobSnapshot, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return workloadJobSnapshot{}, err
	}
	requestPath := strings.ReplaceAll(workloadPreemptJobPathTemplate, "{id}", url.PathEscape(id))
	endpoint, err := c.endpoint(requestPath, url.Values{})
	if err != nil {
		return workloadJobSnapshot{}, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return workloadJobSnapshot{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set(headerServiceKey, c.apiKey)
	raw, status, err := c.do(httpReq)
	if err != nil {
		return workloadJobSnapshot{}, err
	}
	if status != http.StatusOK {
		return workloadJobSnapshot{}, fmt.Errorf("workload preempt status returned HTTP %d", status)
	}
	var record struct {
		Data map[string]any `json:"data"`
	}
	if err := decodePreemptionEnvelope(raw, &record); err != nil {
		return workloadJobSnapshot{}, err
	}
	return workloadSnapshotFromData(id, record.Data), nil
}

func (c httpWorkloadPreemptionClient) endpoint(requestPath string, query url.Values) (string, error) {
	parsed, err := url.Parse(c.baseURL)
	if err != nil {
		return "", err
	}
	if !parsed.IsAbs() || parsed.Host == "" {
		return "", fmt.Errorf("workload service URL must be absolute")
	}
	parsed.Path = path.Join(parsed.Path, requestPath)
	parsed.RawQuery = query.Encode()
	parsed.Fragment = ""
	return parsed.String(), nil
}

func (c httpWorkloadPreemptionClient) do(req *http.Request) ([]byte, int, error) {
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, resp.StatusCode, err
	}
	return raw, resp.StatusCode, nil
}

func (c localWorkloadPreemptionClient) Context(ctx context.Context, req preemptionRequest) (workloadPreemptionContext, error) {
	target := workloadPreemptionContextPath
	if query := req.contextQuery().Encode(); query != "" {
		target += "?" + query
	}
	httpReq := httptest.NewRequest(http.MethodGet, target, nil).WithContext(ctx)
	httpReq.Header.Set(headerServiceKey, c.app.Config.ServiceAPIKey)
	rec := httptest.NewRecorder()
	c.app.ServeHTTP(rec, httpReq)
	if rec.Code != http.StatusOK {
		return workloadPreemptionContext{}, fmt.Errorf("workload preemption context returned HTTP %d", rec.Code)
	}
	var out workloadPreemptionContext
	if err := decodePreemptionEnvelope(rec.Body.Bytes(), &out); err != nil {
		return workloadPreemptionContext{}, err
	}
	return out, nil
}

func (c localWorkloadPreemptionClient) Preempt(ctx context.Context, id string, req workloadPreemptRequest) (workloadJobSnapshot, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return workloadJobSnapshot{}, err
	}
	target := strings.ReplaceAll(workloadPreemptJobPathTemplate, "{id}", url.PathEscape(id))
	httpReq := httptest.NewRequest(http.MethodPost, target, bytes.NewReader(body)).WithContext(ctx)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set(headerServiceKey, c.app.Config.ServiceAPIKey)
	rec := httptest.NewRecorder()
	c.app.ServeHTTP(rec, httpReq)
	if rec.Code != http.StatusOK {
		return workloadJobSnapshot{}, fmt.Errorf("workload preempt status returned HTTP %d", rec.Code)
	}
	var record struct {
		Data map[string]any `json:"data"`
	}
	if err := decodePreemptionEnvelope(rec.Body.Bytes(), &record); err != nil {
		return workloadJobSnapshot{}, err
	}
	return workloadSnapshotFromData(id, record.Data), nil
}

func decodePreemptionEnvelope(raw []byte, dest any) error {
	var envelope struct {
		Data  json.RawMessage     `json:"data"`
		Error *platform.ErrorBody `json:"error"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return err
	}
	if envelope.Error != nil {
		return errors.New(envelope.Error.Message)
	}
	if len(envelope.Data) == 0 || string(envelope.Data) == "null" {
		return nil
	}
	return json.Unmarshal(envelope.Data, dest)
}
