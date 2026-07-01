package platform

import (
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/contracts"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

const internalRecordsPath = "/internal/records"

const (
	defaultInternalJSONResponseLimit int64 = 8 << 20
	serviceKeyHeader                       = "X-Service-Key"
	serviceNameHeader                      = "X-Service-Name"
)

// InternalJSONClient calls explicit service-to-service JSON contracts. It keeps
// service wrappers responsible for path construction, payloads, and status
// mapping; the shared client only owns local/remote transport and envelope
// decoding.
type InternalJSONClient struct {
	app    *App
	owner  string
	client *http.Client
}

type InternalJSONRequest struct {
	Method        string
	Path          string
	Query         url.Values
	Headers       http.Header
	Body          any
	Response      any
	ResponseLimit int64
}

type InternalJSONResponse struct {
	StatusCode    int
	Header        http.Header
	EnvelopeError *ErrorBody
}

func NewInternalJSONClient(app *App, owner string) InternalJSONClient {
	timeout := 2 * time.Second
	if app != nil && app.Config.AdapterTimeout > 0 {
		timeout = app.Config.AdapterTimeout
	}
	return InternalJSONClient{
		app:    app,
		owner:  owner,
		client: &http.Client{Timeout: timeout, Transport: otelhttp.NewTransport(http.DefaultTransport)},
	}
}

func (c InternalJSONClient) Do(ctx context.Context, req InternalJSONRequest) (InternalJSONResponse, error) {
	if c.app == nil {
		return InternalJSONResponse{}, fmt.Errorf("internal JSON client is not configured")
	}
	method := req.Method
	if method == "" {
		method = http.MethodGet
	}
	body, err := encodeInternalJSONBody(req.Body)
	if err != nil {
		return InternalJSONResponse{}, err
	}
	headers := cloneInternalJSONHeaders(req.Headers)
	if req.Body != nil && strings.TrimSpace(headers.Get("Content-Type")) == "" {
		headers.Set("Content-Type", "application/json")
	}
	c.app.Config.applyServiceIdentityHeaders(headers)
	if c.app.Config.AllowsService(c.owner) {
		return c.doLocal(ctx, method, req.Path, req.Query, headers, body, req)
	}
	return c.doRemote(ctx, method, req.Path, req.Query, headers, body, req)
}

func (c InternalJSONClient) doLocal(ctx context.Context, method, requestPath string, query url.Values, headers http.Header, body []byte, spec InternalJSONRequest) (InternalJSONResponse, error) {
	target := requestPath
	if encoded := query.Encode(); encoded != "" {
		target += "?" + encoded
	}
	httpReq := httptest.NewRequest(method, target, bytes.NewReader(body)).WithContext(ctx)
	copyInternalJSONHeaders(httpReq.Header, headers)
	rec := httptest.NewRecorder()
	c.app.ServeHTTP(rec, httpReq)
	return decodeInternalJSONResponse(rec.Code, rec.Header(), rec.Body.Bytes(), spec)
}

func (c InternalJSONClient) doRemote(ctx context.Context, method, requestPath string, query url.Values, headers http.Header, body []byte, spec InternalJSONRequest) (InternalJSONResponse, error) {
	baseURL := strings.TrimSpace(c.app.Config.ServiceURLs[c.owner])
	if baseURL == "" {
		return InternalJSONResponse{}, fmt.Errorf("no SERVICE_URLS entry for owner %q", c.owner)
	}
	endpoint, err := serviceEndpoint(baseURL, requestPath, query.Encode())
	if err != nil {
		return InternalJSONResponse{}, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, method, endpoint, bytes.NewReader(body))
	if err != nil {
		return InternalJSONResponse{}, err
	}
	copyInternalJSONHeaders(httpReq.Header, headers)
	client := c.client
	if client == nil {
		client = NewInternalJSONClient(c.app, c.owner).client
	}
	resp, err := client.Do(httpReq)
	if err != nil {
		return InternalJSONResponse{}, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, internalJSONLimit(spec)))
	if err != nil {
		return InternalJSONResponse{StatusCode: resp.StatusCode, Header: resp.Header.Clone()}, err
	}
	return decodeInternalJSONResponse(resp.StatusCode, resp.Header, raw, spec)
}

func encodeInternalJSONBody(body any) ([]byte, error) {
	if body == nil {
		return nil, nil
	}
	return json.Marshal(body)
}

func decodeInternalJSONResponse(status int, header http.Header, raw []byte, spec InternalJSONRequest) (InternalJSONResponse, error) {
	out := InternalJSONResponse{StatusCode: status, Header: header.Clone()}
	if len(bytes.TrimSpace(raw)) == 0 {
		return out, nil
	}
	var envelope struct {
		Data  json.RawMessage `json:"data"`
		Error *ErrorBody      `json:"error"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return out, err
	}
	out.EnvelopeError = envelope.Error
	if spec.Response != nil && len(envelope.Data) > 0 && string(envelope.Data) != "null" {
		if err := json.Unmarshal(envelope.Data, spec.Response); err != nil {
			return out, err
		}
	}
	return out, nil
}

func internalJSONLimit(spec InternalJSONRequest) int64 {
	if spec.ResponseLimit > 0 {
		return spec.ResponseLimit
	}
	return defaultInternalJSONResponseLimit
}

func cloneInternalJSONHeaders(src http.Header) http.Header {
	dst := http.Header{}
	copyInternalJSONHeaders(dst, src)
	return dst
}

func copyInternalJSONHeaders(dst, src http.Header) {
	for key, values := range src {
		for _, value := range values {
			dst.Add(key, value)
		}
	}
}

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

	// image-registry allow-list read consumed by scheduler-quota submit admission
	// (in-code image allow-list enforcement). List-only — admission needs the full
	// set of enabled rules for the project.
	"image-registry-service:image_allow_lists": {listPath: "/internal/image-registry/image-allow-lists"},
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
	cfg    Config
	client *http.Client
}

var _ CrossServiceReader = (*RemoteServiceReader)(nil)

func NewRemoteServiceReader(cfg Config) *RemoteServiceReader {
	timeout := cfg.AdapterTimeout
	if timeout <= 0 {
		timeout = 2 * time.Second
	}
	return &RemoteServiceReader{
		cfg:    cfg,
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
	base := rr.cfg.ServiceURLs[owner]
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
	rr.cfg.applyServiceIdentityHeaders(req.Header)
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

// AuthorizeServiceRequest is a compatibility wrapper for raw internal contracts
// that do not have an owner audience.
func (a *App) AuthorizeServiceRequest(w http.ResponseWriter, r *http.Request) bool {
	return a.AuthorizeServiceRequestForAudience(w, r, "")
}

// AuthorizeServiceRequestForAudience enforces service-to-service identity for
// raw internal contracts owned by audience. When no identity is configured,
// internal contracts are not exposed.
func (a *App) AuthorizeServiceRequestForAudience(w http.ResponseWriter, r *http.Request, audience string) bool {
	if !a.Config.acceptsServiceIdentity() {
		http.NotFound(w, r)
		return false
	}
	if !a.serviceRequestAuthorizedForAudience(r, audience) {
		WriteError(w, r, http.StatusUnauthorized, "unauthorized", "service authentication is required")
		return false
	}
	return true
}

// ServiceRequestAuthorized reports whether r carries a trusted service identity.
// Without a route audience it only validates the caller/key pair for legacy
// custom handlers.
func (a *App) ServiceRequestAuthorized(r *http.Request) bool {
	return a.serviceRequestAuthorizedForAudience(r, "")
}

// ServiceRequestAuthorizedForAudience reports whether r carries a trusted
// service identity allowed to call audience.
func (a *App) ServiceRequestAuthorizedForAudience(r *http.Request, audience string) bool {
	return a.serviceRequestAuthorizedForAudience(r, audience)
}

func (a *App) serviceRequestAuthorizedForAudience(r *http.Request, audience string) bool {
	if len(a.Config.ServiceTrustedIdentities) > 0 {
		return a.scopedServiceRequestAuthorized(r, audience)
	}
	if a.Config.StrictRuntimeChecks() || strings.TrimSpace(a.Config.ServiceAPIKey) == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(r.Header.Get(serviceKeyHeader)), []byte(a.Config.ServiceAPIKey)) == 1
}

func (a *App) scopedServiceRequestAuthorized(r *http.Request, audience string) bool {
	caller := strings.TrimSpace(r.Header.Get(serviceNameHeader))
	trusted, ok := a.Config.ServiceTrustedIdentities[caller]
	if !ok || strings.TrimSpace(trusted.Key) == "" {
		return false
	}
	if subtle.ConstantTimeCompare([]byte(r.Header.Get(serviceKeyHeader)), []byte(trusted.Key)) != 1 {
		return false
	}
	if strings.TrimSpace(audience) == "" {
		return true
	}
	return serviceAudienceAllowed(trusted.Audiences, audience)
}

func serviceAudienceAllowed(audiences []string, target string) bool {
	target = strings.TrimSpace(target)
	for _, audience := range audiences {
		audience = strings.TrimSpace(audience)
		if audience == target || deployableUnitHosts(audience, target) {
			return true
		}
	}
	return false
}

func deployableUnitHosts(unit, service string) bool {
	for _, hosted := range deployableUnitServices[unit] {
		if hosted == service {
			return true
		}
	}
	return false
}
