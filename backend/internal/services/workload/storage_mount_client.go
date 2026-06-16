package workload

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
	"github.com/linskybing/nexuspaas/backend/internal/platform/cluster"
)

const (
	storageServiceName            = "storage-service"
	storageMountPlanPathTemplate  = "/internal/storage/projects/{project_id}/mount-plan"
	storageMountPlanServiceHeader = "X-Service-Key"
)

type storageMountPlanClient interface {
	Resolve(context.Context, string, storageMountPlanRequest) (storageMountPlan, error)
}

type storageMountPlanRequest struct {
	UserID    string                     `json:"user_id"`
	Namespace string                     `json:"namespace"`
	Mounts    []storageMountPlanSelector `json:"mounts"`
}

type storageMountPlanSelector struct {
	PVCID     string `json:"pvc_id"`
	Name      string `json:"name,omitempty"`
	MountPath string `json:"mount_path,omitempty"`
	ReadOnly  bool   `json:"read_only,omitempty"`
	SubPath   string `json:"sub_path,omitempty"`
}

type storageMountPlan struct {
	ProjectID          string                    `json:"project_id"`
	UserID             string                    `json:"user_id"`
	Namespace          string                    `json:"namespace"`
	ManifestMounts     []storageMountPlanMount   `json:"manifest_mounts"`
	PVCShareOperations []storageMountPlanShareOp `json:"pvc_share_operations"`
}

type storageMountPlanMount struct {
	Name      string `json:"name"`
	ClaimName string `json:"claim_name"`
	MountPath string `json:"mount_path"`
	ReadOnly  bool   `json:"read_only,omitempty"`
	SubPath   string `json:"sub_path,omitempty"`
}

type storageMountPlanShareOp struct {
	SourceNamespace string `json:"source_namespace"`
	SourcePVC       string `json:"source_pvc"`
	TargetPVC       string `json:"target_pvc"`
}

type appStorageMountPlanClient struct {
	app *platform.App
}

type httpStorageMountPlanClient struct {
	baseURL string
	apiKey  string
	client  *http.Client
}

type localStorageMountPlanClient struct {
	app *platform.App
}

func newAppStorageMountPlanClient(app *platform.App) storageMountPlanClient {
	return appStorageMountPlanClient{app: app}
}

func (c appStorageMountPlanClient) Resolve(ctx context.Context, projectID string, req storageMountPlanRequest) (storageMountPlan, error) {
	client, err := newStorageMountPlanClient(c.app)
	if err != nil {
		return storageMountPlan{}, err
	}
	return client.Resolve(ctx, projectID, req)
}

func newStorageMountPlanClient(app *platform.App) (storageMountPlanClient, error) {
	if app == nil {
		return nil, fmt.Errorf("storage mount plan client is not configured")
	}
	if app.Config.AllowsService(storageServiceName) {
		return localStorageMountPlanClient{app: app}, nil
	}
	baseURL := strings.TrimSpace(app.Config.ServiceURLs[storageServiceName])
	if baseURL == "" {
		return nil, fmt.Errorf("storage-service URL is not configured")
	}
	if strings.TrimSpace(app.Config.ServiceAPIKey) == "" {
		return nil, fmt.Errorf("service API key is not configured")
	}
	timeout := app.Config.AdapterTimeout
	if timeout <= 0 {
		timeout = 2 * time.Second
	}
	return httpStorageMountPlanClient{
		baseURL: baseURL,
		apiKey:  app.Config.ServiceAPIKey,
		client:  &http.Client{Timeout: timeout},
	}, nil
}

func (c httpStorageMountPlanClient) Resolve(ctx context.Context, projectID string, req storageMountPlanRequest) (storageMountPlan, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return storageMountPlan{}, err
	}
	endpoint, err := c.endpoint(storageMountPlanPath(projectID))
	if err != nil {
		return storageMountPlan{}, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return storageMountPlan{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set(storageMountPlanServiceHeader, c.apiKey)
	raw, status, err := c.do(httpReq)
	if err != nil {
		return storageMountPlan{}, err
	}
	if status != http.StatusOK {
		if status >= http.StatusBadRequest && status < http.StatusInternalServerError {
			return storageMountPlan{}, fmt.Errorf("%w: storage mount plan returned HTTP %d", cluster.ErrInvalidManifest, status)
		}
		return storageMountPlan{}, fmt.Errorf("storage mount plan returned HTTP %d", status)
	}
	return decodeStorageMountPlan(raw)
}

func (c httpStorageMountPlanClient) endpoint(requestPath string) (string, error) {
	parsed, err := url.Parse(c.baseURL)
	if err != nil {
		return "", err
	}
	if !parsed.IsAbs() || parsed.Host == "" {
		return "", fmt.Errorf("storage service URL must be absolute")
	}
	parsed.Path = path.Join(parsed.Path, requestPath)
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String(), nil
}

func (c httpStorageMountPlanClient) do(req *http.Request) ([]byte, int, error) {
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

func (c localStorageMountPlanClient) Resolve(ctx context.Context, projectID string, req storageMountPlanRequest) (storageMountPlan, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return storageMountPlan{}, err
	}
	httpReq := httptest.NewRequest(http.MethodPost, storageMountPlanPath(projectID), bytes.NewReader(body)).WithContext(ctx)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set(storageMountPlanServiceHeader, c.app.Config.ServiceAPIKey)
	rec := httptest.NewRecorder()
	c.app.ServeHTTP(rec, httpReq)
	if rec.Code != http.StatusOK {
		if rec.Code >= http.StatusBadRequest && rec.Code < http.StatusInternalServerError {
			return storageMountPlan{}, fmt.Errorf("%w: storage mount plan returned HTTP %d", cluster.ErrInvalidManifest, rec.Code)
		}
		return storageMountPlan{}, fmt.Errorf("storage mount plan returned HTTP %d", rec.Code)
	}
	return decodeStorageMountPlan(rec.Body.Bytes())
}

func storageMountPlanPath(projectID string) string {
	return strings.ReplaceAll(storageMountPlanPathTemplate, "{project_id}", url.PathEscape(projectID))
}

func decodeStorageMountPlan(raw []byte) (storageMountPlan, error) {
	var envelope struct {
		Data  json.RawMessage     `json:"data"`
		Error *platform.ErrorBody `json:"error"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return storageMountPlan{}, err
	}
	if envelope.Error != nil {
		return storageMountPlan{}, errors.New(envelope.Error.Message)
	}
	if len(envelope.Data) == 0 || string(envelope.Data) == "null" {
		return storageMountPlan{}, nil
	}
	var plan storageMountPlan
	if err := json.Unmarshal(envelope.Data, &plan); err != nil {
		return storageMountPlan{}, err
	}
	return plan, nil
}
