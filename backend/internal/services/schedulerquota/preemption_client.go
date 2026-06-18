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
	workloadServiceName            = "workload-service"
	workloadPreemptionContextPath  = "/internal/workload/preemption-context"
	workloadPreemptJobPathTemplate = "/internal/workload/jobs/{id}/preempt"
)

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

type internalWorkloadPreemptionClient struct {
	client platform.InternalJSONClient
}

func newWorkloadPreemptionClient(app *platform.App) (internalWorkloadPreemptionClient, error) {
	if app == nil {
		return internalWorkloadPreemptionClient{}, fmt.Errorf("workload preemption client is not configured")
	}
	if !app.Config.AllowsService(workloadServiceName) {
		if strings.TrimSpace(app.Config.ServiceURLs[workloadServiceName]) == "" {
			return internalWorkloadPreemptionClient{}, fmt.Errorf("workload-service URL is not configured")
		}
		if strings.TrimSpace(app.Config.ServiceAPIKey) == "" {
			return internalWorkloadPreemptionClient{}, fmt.Errorf("service API key is not configured")
		}
	}
	return internalWorkloadPreemptionClient{client: platform.NewInternalJSONClient(app, workloadServiceName)}, nil
}

func (c internalWorkloadPreemptionClient) Context(ctx context.Context, req preemptionRequest) (workloadPreemptionContext, error) {
	var out workloadPreemptionContext
	resp, err := c.client.Do(ctx, platform.InternalJSONRequest{
		Method:   http.MethodGet,
		Path:     workloadPreemptionContextPath,
		Query:    req.contextQuery(),
		Response: &out,
	})
	if err != nil {
		return workloadPreemptionContext{}, err
	}
	if resp.StatusCode != http.StatusOK {
		return workloadPreemptionContext{}, fmt.Errorf("workload preemption context returned HTTP %d", resp.StatusCode)
	}
	if resp.EnvelopeError != nil {
		return workloadPreemptionContext{}, errors.New(resp.EnvelopeError.Message)
	}
	return out, nil
}

func (c internalWorkloadPreemptionClient) Preempt(ctx context.Context, id string, req workloadPreemptRequest) (workloadJobSnapshot, error) {
	var record struct {
		Data map[string]any `json:"data"`
	}
	resp, err := c.client.Do(ctx, platform.InternalJSONRequest{
		Method:   http.MethodPost,
		Path:     strings.ReplaceAll(workloadPreemptJobPathTemplate, "{id}", url.PathEscape(id)),
		Body:     req,
		Response: &record,
	})
	if err != nil {
		return workloadJobSnapshot{}, err
	}
	if resp.StatusCode != http.StatusOK {
		return workloadJobSnapshot{}, fmt.Errorf("workload preempt status returned HTTP %d", resp.StatusCode)
	}
	if resp.EnvelopeError != nil {
		return workloadJobSnapshot{}, errors.New(resp.EnvelopeError.Message)
	}
	return workloadSnapshotFromData(id, record.Data), nil
}
