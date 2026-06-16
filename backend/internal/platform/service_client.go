package platform

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/contracts"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

const internalRecordsPath = "/internal/records"

// CrossServiceReader resolves reads of resources owned by other services. The
// remote implementation calls explicit owning-service read contracts so an
// isolated deployment no longer reads another service's in-process store or a
// generic records API (finding 5).
type CrossServiceReader interface {
	List(ctx context.Context, resource string) ([]contracts.Record[map[string]any], error)
	Get(ctx context.Context, resource, id string) (contracts.Record[map[string]any], bool, error)
}

type readContract struct {
	listPath string
	getPath  string
}

var domainReadContracts = map[string]readContract{
	"identity-service:users": {listPath: "/internal/identity/users", getPath: "/internal/identity/users/{id}"},
	"identity-service:roles": {listPath: "/internal/identity/roles", getPath: "/internal/identity/roles/{id}"},

	// org-project / workload read contracts consumed by scheduler-quota submit
	// admission (problem.md #3). project_members and user_quotas are keyed by a
	// composite "<projectID>/<userID>", so their provider get routes use a trailing
	// wildcard; the get path here substitutes the raw key into that suffix. Workload
	// jobs are list-only because admission only needs aggregate active usage.
	"org-project-service:projects":        {listPath: "/internal/org-project/projects", getPath: "/internal/org-project/projects/{id}"},
	"org-project-service:project_members": {listPath: "/internal/org-project/project-members", getPath: "/internal/org-project/project-members/{id}"},
	"org-project-service:user_quotas":     {listPath: "/internal/org-project/user-quotas", getPath: "/internal/org-project/user-quotas/{id}"},
	"org-project-service:user_groups":     {listPath: "/internal/org-project/user-groups", getPath: "/internal/org-project/user-groups/{id}"},
	"workload-service:jobs":               {listPath: "/internal/workload/jobs"},
}

// crossServiceStore decorates a RecordStore so reads of resources owned by a
// non-co-hosted service are served over HTTP, while same-service reads and all
// writes stay local. In SERVICE_NAME=all every owner is co-hosted, so it is a
// transparent passthrough and handlers need no changes.
type crossServiceStore struct {
	local  RecordStore
	cfg    Config
	remote CrossServiceReader
}

var _ RecordStore = (*crossServiceStore)(nil)

func (s *crossServiceStore) List(ctx context.Context, resource string) []contracts.Record[map[string]any] {
	if s.cfg.AllowsService(resourceOwner(resource)) {
		return s.local.List(ctx, resource)
	}
	records, err := s.remote.List(ctx, resource)
	if err != nil {
		slog.Error("cross-service list failed", "resource", resource, "error", err)
		return nil
	}
	return records
}

func (s *crossServiceStore) Get(ctx context.Context, resource, id string) (contracts.Record[map[string]any], bool) {
	if s.cfg.AllowsService(resourceOwner(resource)) {
		return s.local.Get(ctx, resource, id)
	}
	record, ok, err := s.remote.Get(ctx, resource, id)
	if err != nil {
		slog.Error("cross-service get failed", "resource", resource, "id", id, "error", err)
		return contracts.Record[map[string]any]{}, false
	}
	return record, ok
}

func (s *crossServiceStore) Create(ctx context.Context, resource string, data map[string]any) (contracts.Record[map[string]any], error) {
	return s.local.Create(ctx, resource, data)
}

func (s *crossServiceStore) Update(ctx context.Context, resource, id string, data map[string]any) (contracts.Record[map[string]any], bool) {
	return s.local.Update(ctx, resource, id, data)
}

func (s *crossServiceStore) Delete(ctx context.Context, resource, id string) bool {
	return s.local.Delete(ctx, resource, id)
}

func (s *crossServiceStore) NextID(resource, prefix string, base, width int) string {
	return s.local.NextID(resource, prefix, base, width)
}

// RemoteServiceReader calls owning services through domain read contracts. It
// deliberately refuses resources without a contract instead of using the legacy
// generic internal-records fallback.
type RemoteServiceReader struct {
	urls   map[string]string
	apiKey string
	client *http.Client
}

var _ CrossServiceReader = (*RemoteServiceReader)(nil)

func NewRemoteServiceReader(cfg Config) *RemoteServiceReader {
	timeout := cfg.AdapterTimeout
	if timeout <= 0 {
		timeout = 2 * time.Second
	}
	return &RemoteServiceReader{
		urls:   cfg.ServiceURLs,
		apiKey: cfg.ServiceAPIKey,
		client: &http.Client{Timeout: timeout, Transport: otelhttp.NewTransport(http.DefaultTransport)},
	}
}

func (rr *RemoteServiceReader) List(ctx context.Context, resource string) ([]contracts.Record[map[string]any], error) {
	body, status, err := rr.fetch(ctx, resource, "")
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("cross-service list returned HTTP %d", status)
	}
	var records []contracts.Record[map[string]any]
	if err := decodeEnvelopeData(body, &records); err != nil {
		return nil, err
	}
	return records, nil
}

func (rr *RemoteServiceReader) Get(ctx context.Context, resource, id string) (contracts.Record[map[string]any], bool, error) {
	body, status, err := rr.fetch(ctx, resource, id)
	if err != nil {
		return contracts.Record[map[string]any]{}, false, err
	}
	if status == http.StatusNotFound {
		return contracts.Record[map[string]any]{}, false, nil
	}
	if status != http.StatusOK {
		return contracts.Record[map[string]any]{}, false, fmt.Errorf("cross-service get returned HTTP %d", status)
	}
	var record contracts.Record[map[string]any]
	if err := decodeEnvelopeData(body, &record); err != nil {
		return contracts.Record[map[string]any]{}, false, err
	}
	return record, true, nil
}

func (rr *RemoteServiceReader) fetch(ctx context.Context, resource, id string) ([]byte, int, error) {
	if contract, ok := domainReadContracts[resource]; ok {
		path := contract.listPath
		if id != "" {
			if contract.getPath == "" {
				return nil, 0, fmt.Errorf("no domain get contract for resource %q", resource)
			}
			path = strings.ReplaceAll(contract.getPath, "{id}", id)
		}
		return rr.fetchPath(ctx, resourceOwner(resource), path, "")
	}
	return nil, 0, fmt.Errorf("no domain read contract for resource %q", resource)
}

func (rr *RemoteServiceReader) fetchPath(ctx context.Context, owner, requestPath, rawQuery string) ([]byte, int, error) {
	base := rr.urls[owner]
	if base == "" {
		return nil, 0, fmt.Errorf("no SERVICE_URLS entry for owner %q", owner)
	}
	endpoint, err := serviceEndpoint(base, requestPath, rawQuery)
	if err != nil {
		return nil, 0, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, 0, err
	}
	if rr.apiKey != "" {
		req.Header.Set("X-Service-Key", rr.apiKey)
	}
	resp, err := rr.client.Do(req)
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

func serviceEndpoint(baseURL, requestPath, rawQuery string) (string, error) {
	base, err := url.Parse(baseURL)
	if err != nil {
		return "", err
	}
	if !base.IsAbs() || base.Host == "" {
		return "", fmt.Errorf("service URL must be absolute")
	}
	base.Path = joinProxyPath(base.Path, requestPath)
	base.RawQuery = rawQuery
	base.Fragment = ""
	return base.String(), nil
}

func decodeEnvelopeData(raw []byte, dest any) error {
	var envelope struct {
		Success bool            `json:"success"`
		Data    json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return err
	}
	if len(envelope.Data) == 0 {
		return nil
	}
	return json.Unmarshal(envelope.Data, dest)
}

// AuthorizeServiceRequest enforces the shared service-to-service API key used by
// internal read contracts. When no key is configured, internal contracts are not
// exposed at all.
func (a *App) AuthorizeServiceRequest(w http.ResponseWriter, r *http.Request) bool {
	if a.Config.ServiceAPIKey == "" {
		http.NotFound(w, r)
		return false
	}
	if !a.ServiceRequestAuthorized(r) {
		WriteError(w, r, http.StatusUnauthorized, "unauthorized", "service authentication is required")
		return false
	}
	return true
}

// ServiceRequestAuthorized reports whether r carries the configured
// service-to-service key. Custom handlers that return through the standard route
// envelope use this predicate; raw internal handlers use AuthorizeServiceRequest.
func (a *App) ServiceRequestAuthorized(r *http.Request) bool {
	if a.Config.ServiceAPIKey == "" {
		return false
	}
	key := r.Header.Get("X-Service-Key")
	return subtle.ConstantTimeCompare([]byte(key), []byte(a.Config.ServiceAPIKey)) == 1
}
