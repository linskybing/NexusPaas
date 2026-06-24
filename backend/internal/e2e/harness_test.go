//go:build e2e

package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"net/textproto"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/contracts"
	"github.com/linskybing/nexuspaas/backend/internal/platform"
	"github.com/linskybing/nexuspaas/backend/internal/services"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

const (
	identityService            = "identity-service"
	authorizationPolicyService = "authorization-policy-service"
	orgProjectService          = "org-project-service"
	schedulerQuotaService      = "scheduler-quota-service"
	workloadService            = "workload-service"
	k8sControlService          = "k8s-control-service"
	ideService                 = "ide-service"
	storageService             = "storage-service"
	imageRegistryService       = "image-registry-service"
	usageObservabilityService  = "usage-observability-service"
	auditComplianceService     = "audit-compliance-service"
	integrationProxyService    = "integration-proxy-service"
	mediaUploadService         = "media-upload-service"
	requestNotificationService = "request-notification-service"

	identityUsersResource       = "identity-service:users"
	identityRolesResource       = "identity-service:roles"
	identitySessionsResource    = "identity-service:sessions"
	identityAPITokensResource   = "identity-service:api_tokens"
	schedulerQueuesResource     = "scheduler-quota-service:queues"
	schedulerPlansResource      = "scheduler-quota-service:plans"
	schedulerAdmissionsResource = "scheduler-quota-service:submit_admissions"
	schedulerReservations       = "scheduler-quota-service:reservations"
	orgGroupsResource           = "org-project-service:groups"
	orgProjectsResource         = "org-project-service:projects"
	orgProjectMembersResource   = "org-project-service:project_members"
	orgUserGroupsResource       = "org-project-service:user_groups"
	workloadJobsResource        = "workload-service:jobs"
	gpuIdentityUsersResource    = "usage-observability-service:gpu_identity_users"
	gpuJobsResource             = "usage-observability-service:gpu_jobs"
	gpuProjectsResource         = "usage-observability-service:gpu_projects"
	gpuReadModelsResource       = "usage-observability-service:cluster_read_models"
	gpuSnapshotsResource        = "usage-observability-service:job_gpu_usage_snapshots"
	gpuSummariesResource        = "usage-observability-service:job_gpu_usage_summaries"
	mediaResource               = "media-upload-service:uploaded_media"
	formsResource               = "request-notification-service:forms"
)

type e2eHarness struct {
	t              *testing.T
	ctx            context.Context
	runID          string
	apiKey         string
	serviceKey     string
	databaseURL    string
	redisURL       string
	eventBusURL    string
	objectStoreURL string
	objectAccess   string
	objectSecret   string
	objectBucket   string
	pool           *pgxpool.Pool
	redis          *redis.Client
	store          platform.RecordStore
	objectStore    platform.ObjectStore
	services       map[string]*runningService
	closers        []func()
	// extraPeers preserves the call-site intent for tests that need to document a
	// specific owner dependency. The harness still exposes the full set of
	// preallocated service URLs to every service so the default topology matches
	// local compose/CI service-to-service wiring.
	extraPeers map[string][]string
}

type runningService struct {
	name     string
	url      string
	listener net.Listener
	server   *http.Server
	app      *platform.App
	backing  *platform.BackingResources
}

type testResponse struct {
	Status int
	Header http.Header
	Body   []byte
}

func newHarness(t *testing.T, serviceNames ...string) *e2eHarness {
	return newHarnessWithPeers(t, nil, serviceNames...)
}

// newHarnessWithPeers builds a harness and wires the given extra owner peers into
// each consumer process's SERVICE_URLS (consumer -> []owner). Use it for tests
// that exercise a cross-process owner contract while leaving the default
// shared-store read path intact for other tests.
func newHarnessWithPeers(t *testing.T, extraPeers map[string][]string, serviceNames ...string) *e2eHarness {
	t.Helper()
	ctx := context.Background()
	h := &e2eHarness{
		t:              t,
		ctx:            ctx,
		extraPeers:     extraPeers,
		runID:          sanitizedRunID(t),
		apiKey:         envDefault("E2E_API_KEY", "e2e-admin-key"),
		serviceKey:     envDefault("E2E_SERVICE_API_KEY", "e2e-service-key"),
		databaseURL:    requireEnv(t, "TEST_DATABASE_URL"),
		redisURL:       requireEnv(t, "TEST_REDIS_URL"),
		objectStoreURL: requireEnv(t, "TEST_OBJECT_STORE_URL"),
		objectAccess:   requireEnv(t, "TEST_OBJECT_STORE_ACCESS_KEY"),
		objectSecret:   requireEnv(t, "TEST_OBJECT_STORE_SECRET_KEY"),
		objectBucket:   envDefault("TEST_OBJECT_STORE_BUCKET", "media-e2e"),
		services:       map[string]*runningService{},
	}
	h.eventBusURL = envDefault("TEST_EVENT_BUS_URL", h.redisURL)

	if err := platform.ApplyMigrations(ctx, h.databaseURL); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}
	pool, err := pgxpool.New(ctx, h.databaseURL)
	if err != nil {
		t.Fatalf("connect postgres: %v", err)
	}
	h.pool = pool
	h.store = platform.NewPostgresStore(pool)

	redisOpts, err := redis.ParseURL(h.redisURL)
	if err != nil {
		t.Fatalf("parse TEST_REDIS_URL: %v", err)
	}
	h.redis = redis.NewClient(redisOpts)
	if err := h.redis.Ping(ctx).Err(); err != nil {
		t.Fatalf("ping redis: %v", err)
	}

	if _, err := platform.EnsureObjectStoreBucket(ctx, h.objectStoreURL, h.objectAccess, h.objectSecret, h.objectBucket); err != nil {
		t.Fatalf("ensure object store bucket: %v", err)
	}
	h.objectStore, err = platform.NewMinioObjectStore(ctx, h.objectStoreURL, h.objectAccess, h.objectSecret, h.objectBucket)
	if err != nil {
		t.Fatalf("connect object store: %v", err)
	}

	h.cleanup()
	for _, name := range serviceNames {
		h.preallocateService(name)
	}
	for _, name := range serviceNames {
		h.startService(name, h.serviceURLsFor(name))
	}
	h.assertOperationalEndpoints()
	t.Cleanup(func() {
		h.cleanup()
		h.close()
	})
	return h
}

func sanitizedRunID(t *testing.T) string {
	if id := strings.TrimSpace(os.Getenv("E2E_RUN_ID")); id != "" {
		return truncateID(sanitizeID(id), 32)
	}
	return "e2e" + fmt.Sprint(time.Now().UTC().UnixNano())
}

func sanitizeID(value string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(value) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	if b.Len() == 0 {
		return "e2e"
	}
	return b.String()
}

func truncateID(value string, max int) string {
	if len(value) <= max {
		return value
	}
	return value[:max]
}

func envDefault(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func requireEnv(t *testing.T, key string) string {
	t.Helper()
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		t.Fatalf("%s is required for cross-service e2e tests", key)
	}
	return value
}

func (h *e2eHarness) preallocateService(name string) {
	h.t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		h.t.Fatalf("listen %s: %v", name, err)
	}
	h.services[name] = &runningService{
		name:     name,
		url:      "http://" + ln.Addr().String(),
		listener: ln,
	}
}

func (h *e2eHarness) serviceURLsFor(name string) map[string]string {
	urls := make(map[string]string, len(h.services))
	for peer, rs := range h.services {
		if peer == name || rs == nil || rs.url == "" {
			continue
		}
		urls[peer] = rs.url
	}
	for _, peer := range h.extraPeers[name] {
		if rs := h.services[peer]; rs != nil && rs.url != "" {
			urls[peer] = rs.url
		}
	}
	if len(urls) == 0 {
		return nil
	}
	return urls
}

// peerURLs returns the service-URL map for the named peers that are running in
// this harness, skipping any that were not requested so isolated harnesses do
// not panic on an unallocated peer.
func (h *e2eHarness) peerURLs(peers ...string) map[string]string {
	urls := map[string]string{}
	for _, peer := range peers {
		if rs := h.services[peer]; rs != nil {
			urls[peer] = rs.url
		}
	}
	if len(urls) == 0 {
		return nil
	}
	return urls
}

func (h *e2eHarness) startService(name string, serviceURLs map[string]string) {
	h.t.Helper()
	rs := h.services[name]
	if rs == nil {
		h.t.Fatalf("service %s was not preallocated", name)
	}
	cfg := h.serviceConfig(name, serviceURLs)
	backing, err := platform.NewBackingResources(h.ctx, cfg)
	if err != nil {
		h.t.Fatalf("backing %s: %v", name, err)
	}
	app := platform.NewApp(cfg, backing.Options...)
	services.RegisterAll(app)
	rs.app = app
	rs.backing = backing
	rs.server = &http.Server{Handler: app}
	go func() {
		if err := rs.server.Serve(rs.listener); err != nil && err != http.ErrServerClosed {
			h.t.Errorf("serve %s: %v", name, err)
		}
	}()
	h.waitReady(rs)
}

func (h *e2eHarness) startExtraService(instanceName, serviceName string, serviceURLs map[string]string) *runningService {
	h.t.Helper()
	return h.startExtraServiceWithConfig(instanceName, serviceName, serviceURLs, nil)
}

func (h *e2eHarness) startExtraServiceWithConfig(instanceName, serviceName string, serviceURLs map[string]string, configure func(*platform.Config)) *runningService {
	h.t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		h.t.Fatalf("listen %s: %v", instanceName, err)
	}
	cfg := h.serviceConfig(serviceName, serviceURLs)
	if configure != nil {
		configure(&cfg)
	}
	backing, err := platform.NewBackingResources(h.ctx, cfg)
	if err != nil {
		_ = ln.Close()
		h.t.Fatalf("backing %s: %v", instanceName, err)
	}
	app := platform.NewApp(cfg, backing.Options...)
	services.RegisterAll(app)
	rs := &runningService{
		name:     instanceName,
		url:      "http://" + ln.Addr().String(),
		listener: ln,
		server:   &http.Server{Handler: app},
		app:      app,
		backing:  backing,
	}
	go func() {
		if err := rs.server.Serve(ln); err != nil && err != http.ErrServerClosed {
			h.t.Errorf("serve %s: %v", instanceName, err)
		}
	}()
	h.waitReady(rs)
	h.closers = append(h.closers, func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		_ = rs.server.Shutdown(ctx)
		cancel()
		backing.Close()
	})
	return rs
}

func (h *e2eHarness) serviceConfig(name string, serviceURLs map[string]string) platform.Config {
	cfg := platform.Config{
		ServiceName:            name,
		HTTPAddr:               ":0",
		RequireAuth:            true,
		APIKeys:                map[string]bool{h.apiKey: true, h.serviceKey: true},
		APIKeyPrincipals:       h.apiKeyPrincipals(),
		ServiceURLs:            serviceURLs,
		ServiceAPIKey:          h.serviceKey,
		DatabaseURL:            h.databaseURL,
		RedisURL:               h.redisURL,
		EventBusURL:            h.eventBusURL,
		AdapterTimeout:         500 * time.Millisecond,
		AdapterRetries:         1,
		AdapterThreshold:       1,
		AdapterOpenInterval:    time.Second,
		ShutdownTimeout:        time.Second,
		DefaultQueueName:       "default-batch",
		ExternalURLs:           map[string]string{},
		AuthorizationPolicyURL: "",
	}
	if cfg.RequiresObjectStore() {
		cfg.ObjectStoreURL = h.objectStoreURL
		cfg.ObjectStoreAccessKey = h.objectAccess
		cfg.ObjectStoreSecretKey = h.objectSecret
		cfg.ObjectStoreBucket = h.objectBucket
	}
	return cfg
}

func (h *e2eHarness) apiKeyPrincipals() map[string]platform.APIKeyPrincipal {
	return map[string]platform.APIKeyPrincipal{
		h.apiKey: {
			ID:       "admin-" + h.runID,
			Username: "admin-" + h.runID,
			Role:     "admin",
			Admin:    true,
		},
		h.serviceKey: {
			ID:       "service-" + h.runID,
			Username: "service-" + h.runID,
			Role:     "service",
		},
	}
}

func (h *e2eHarness) waitReady(rs *runningService) {
	h.t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		resp, err := http.Get(rs.url + "/healthz")
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return
			}
		}
		if time.Now().After(deadline) {
			h.t.Fatalf("service %s did not become ready", rs.name)
		}
		time.Sleep(25 * time.Millisecond)
	}
}

func (h *e2eHarness) assertOperationalEndpoints() {
	h.t.Helper()
	for name := range h.services {
		ready := h.do(h.newRequest(name, http.MethodGet, "/readyz", nil, h.apiKey), http.StatusOK)
		h.requireEnvelopeCorrelation(ready)
		readyData := ready.dataMap(h.t)
		if readyData["status"] != "ok" {
			h.t.Fatalf("%s /readyz data = %#v, want ok", name, readyData)
		}

		metrics := h.do(h.newRequest(name, http.MethodGet, "/metrics", nil, h.apiKey), http.StatusOK)
		if !strings.HasPrefix(metrics.Header.Get("Content-Type"), "text/plain") ||
			!bytes.Contains(metrics.Body, []byte("nexuspaas_http_requests_total")) {
			h.t.Fatalf("%s /metrics content-type=%q body=%q", name, metrics.Header.Get("Content-Type"), string(metrics.Body))
		}

		outbox := h.do(h.newRequest(name, http.MethodGet, "/outbox", nil, h.apiKey), http.StatusOK)
		env := h.requireEnvelopeCorrelation(outbox)
		if env["success"] != true {
			h.t.Fatalf("%s /outbox envelope = %#v", name, env)
		}
		if _, ok := env["data"].([]any); !ok {
			h.t.Fatalf("%s /outbox data = %#v, want array", name, env["data"])
		}
	}
}

func (h *e2eHarness) close() {
	for _, rs := range h.services {
		if rs.server != nil {
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			_ = rs.server.Shutdown(ctx)
			cancel()
		}
		if rs.backing != nil {
			rs.backing.Close()
		}
	}
	if h.redis != nil {
		_ = h.redis.Close()
	}
	if h.pool != nil {
		h.pool.Close()
	}
	for i := len(h.closers) - 1; i >= 0; i-- {
		h.closers[i]()
	}
}

func (h *e2eHarness) cleanup() {
	h.cleanupPostgres()
	h.cleanupRedis()
	h.cleanupObjects()
}

func (h *e2eHarness) cleanupPostgres() {
	if h.pool == nil {
		return
	}
	_, _ = h.pool.Exec(h.ctx, `
		DELETE FROM platform_records
		WHERE payload->>'e2e_run_id' = $1
		   OR id LIKE '%' || $1 || '%'
		   OR payload::text LIKE '%' || $1 || '%'`, h.runID)
}

func (h *e2eHarness) cleanupRedis() {
	if h.redis == nil {
		return
	}
	for _, pattern := range []string{"*" + h.runID + "*"} {
		keys, err := h.redis.Keys(h.ctx, pattern).Result()
		if err == nil && len(keys) > 0 {
			_ = h.redis.Del(h.ctx, keys...).Err()
		}
	}
	h.cleanupRedisEvents()
	h.cleanupRedisInboxMembers()
}

func (h *e2eHarness) cleanupRedisEvents() {
	messages, err := h.redis.XRange(h.ctx, "events", "-", "+").Result()
	if err != nil {
		return
	}
	var ids []string
	for _, msg := range messages {
		raw, ok := msg.Values["event"].(string)
		if ok && strings.Contains(raw, h.runID) {
			ids = append(ids, msg.ID)
		}
	}
	if len(ids) > 0 {
		_ = h.redis.XDel(h.ctx, "events", ids...).Err()
	}
}

func (h *e2eHarness) cleanupRedisInboxMembers() {
	keys, err := h.redis.Keys(h.ctx, "inbox:*").Result()
	if err != nil {
		return
	}
	for _, key := range keys {
		members, err := h.redis.SMembers(h.ctx, key).Result()
		if err != nil {
			continue
		}
		var runMembers []any
		for _, member := range members {
			if strings.Contains(member, h.runID) {
				runMembers = append(runMembers, member)
			}
		}
		if len(runMembers) > 0 {
			_ = h.redis.SRem(h.ctx, key, runMembers...).Err()
		}
	}
}

func (h *e2eHarness) cleanupObjects() {
	if h.objectStore == nil {
		return
	}
	infos, err := h.objectStore.List(h.ctx)
	if err != nil {
		return
	}
	for _, info := range infos {
		if strings.Contains(info.Key, h.runID) {
			_ = h.objectStore.Delete(h.ctx, info.Key)
		}
	}
}

func (h *e2eHarness) createRecord(resource, id string, data map[string]any) contracts.Record[map[string]any] {
	h.t.Helper()
	data = cloneMap(data)
	data["id"] = id
	data["e2e_run_id"] = h.runID
	rec, err := h.store.Create(h.ctx, resource, data)
	if err != nil {
		h.t.Fatalf("create %s/%s: %v", resource, id, err)
	}
	return rec
}

func (h *e2eHarness) updateRecord(resource, id string, data map[string]any) {
	h.t.Helper()
	data = cloneMap(data)
	data["e2e_run_id"] = h.runID
	if _, ok := h.store.Update(h.ctx, resource, id, data); !ok {
		h.t.Fatalf("update %s/%s failed", resource, id)
	}
}

func (h *e2eHarness) getRecord(resource, id string) contracts.Record[map[string]any] {
	h.t.Helper()
	rec, ok := h.store.Get(h.ctx, resource, id)
	if !ok {
		h.t.Fatalf("missing record %s/%s", resource, id)
	}
	return rec
}

func (h *e2eHarness) listRecords(resource string) []contracts.Record[map[string]any] {
	return h.store.List(h.ctx, resource)
}

func (h *e2eHarness) doJSON(serviceName, method, path string, payload any, apiKey string, want int) testResponse {
	h.t.Helper()
	var body io.Reader
	if payload != nil {
		raw, err := json.Marshal(payload)
		if err != nil {
			h.t.Fatalf("marshal request: %v", err)
		}
		body = bytes.NewReader(raw)
	}
	req := h.newRequest(serviceName, method, path, body, apiKey)
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return h.do(req, want)
}

func (h *e2eHarness) doInternalJSON(serviceName, method, path string, payload any, serviceKey string, want int) testResponse {
	h.t.Helper()
	var body io.Reader
	if payload != nil {
		raw, err := json.Marshal(payload)
		if err != nil {
			h.t.Fatalf("marshal request: %v", err)
		}
		body = bytes.NewReader(raw)
	}
	req := h.newRequest(serviceName, method, path, body, h.apiKey)
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if serviceKey != "" {
		req.Header.Set("X-Service-Key", serviceKey)
	}
	return h.do(req, want)
}

func (h *e2eHarness) doURLJSON(baseURL, method, path string, payload any, apiKey string, want int) testResponse {
	return h.doURLJSONWithIdempotencyKey(baseURL, method, path, payload, apiKey, "", want)
}

func (h *e2eHarness) doURLJSONWithIdempotencyKey(baseURL, method, path string, payload any, apiKey string, idempotencyKey string, want int) testResponse {
	h.t.Helper()
	var body io.Reader
	if payload != nil {
		raw, err := json.Marshal(payload)
		if err != nil {
			h.t.Fatalf("marshal request: %v", err)
		}
		body = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(h.ctx, method, baseURL+path, body)
	if err != nil {
		h.t.Fatalf("new request: %v", err)
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if apiKey != "" {
		req.Header.Set("X-API-Key", apiKey)
	}
	req.Header.Set("X-Request-ID", "req-"+h.runID)
	req.Header.Set("X-Trace-ID", "trace-"+h.runID)
	if idempotencyKey == "" {
		idempotencyKey = "idem-" + h.runID + "-" + sanitizeID(method+path)
	}
	req.Header.Set("Idempotency-Key", idempotencyKey)
	return h.do(req, want)
}

func (h *e2eHarness) doURLInternalJSON(baseURL, method, path string, payload any, serviceKey string, want int) testResponse {
	h.t.Helper()
	var body io.Reader
	if payload != nil {
		raw, err := json.Marshal(payload)
		if err != nil {
			h.t.Fatalf("marshal request: %v", err)
		}
		body = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(h.ctx, method, baseURL+path, body)
	if err != nil {
		h.t.Fatalf("new request: %v", err)
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if serviceKey != "" {
		req.Header.Set("X-Service-Key", serviceKey)
	}
	if h.apiKey != "" {
		req.Header.Set("X-API-Key", h.apiKey)
	}
	req.Header.Set("X-Request-ID", "req-"+h.runID)
	req.Header.Set("X-Trace-ID", "trace-"+h.runID)
	req.Header.Set("Idempotency-Key", "idem-"+h.runID+"-"+sanitizeID(method+path))
	return h.do(req, want)
}

func (h *e2eHarness) doMultipart(serviceName, path, field, filename, contentType string, body []byte, want int) testResponse {
	h.t.Helper()
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	part, err := writer.CreatePart(textprotoMIMEHeader(field, filename, contentType))
	if err != nil {
		h.t.Fatalf("create multipart part: %v", err)
	}
	if _, err := part.Write(body); err != nil {
		h.t.Fatalf("write multipart part: %v", err)
	}
	if err := writer.Close(); err != nil {
		h.t.Fatalf("close multipart: %v", err)
	}
	req := h.newRequest(serviceName, http.MethodPost, path, &buf, h.apiKey)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	return h.do(req, want)
}

func textprotoMIMEHeader(field, filename, contentType string) textproto.MIMEHeader {
	header := make(textproto.MIMEHeader)
	header.Set("Content-Disposition", fmt.Sprintf(`form-data; name="%s"; filename="%s"`, field, filename))
	header.Set("Content-Type", contentType)
	return header
}

func (h *e2eHarness) newRequest(serviceName, method, path string, body io.Reader, apiKey string) *http.Request {
	h.t.Helper()
	rs := h.services[serviceName]
	if rs == nil {
		h.t.Fatalf("unknown service %s", serviceName)
	}
	req, err := http.NewRequestWithContext(h.ctx, method, rs.url+path, body)
	if err != nil {
		h.t.Fatalf("new request: %v", err)
	}
	if apiKey != "" {
		req.Header.Set("X-API-Key", apiKey)
	}
	req.Header.Set("X-Request-ID", "req-"+h.runID)
	req.Header.Set("X-Trace-ID", "trace-"+h.runID)
	req.Header.Set("Idempotency-Key", "idem-"+h.runID+"-"+sanitizeID(method+path))
	return req
}

func (h *e2eHarness) do(req *http.Request, want int) testResponse {
	h.t.Helper()
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		h.t.Fatalf("%s %s: %v", req.Method, req.URL.String(), err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		h.t.Fatalf("read response: %v", err)
	}
	if resp.StatusCode != want {
		h.t.Fatalf("%s %s returned %d, want %d: %s", req.Method, req.URL.Path, resp.StatusCode, want, string(raw))
	}
	return testResponse{Status: resp.StatusCode, Header: resp.Header.Clone(), Body: raw}
}

func (r testResponse) envelope(t *testing.T) map[string]any {
	t.Helper()
	var env map[string]any
	if err := json.Unmarshal(r.Body, &env); err != nil {
		t.Fatalf("decode envelope: %v body=%s", err, string(r.Body))
	}
	return env
}

func (r testResponse) dataMap(t *testing.T) map[string]any {
	t.Helper()
	env := r.envelope(t)
	data, ok := env["data"].(map[string]any)
	if !ok {
		t.Fatalf("data = %#v, want object", env["data"])
	}
	return data
}

func (r testResponse) rawMap(t *testing.T) map[string]any {
	t.Helper()
	var data map[string]any
	if err := json.Unmarshal(r.Body, &data); err != nil {
		t.Fatalf("decode raw response: %v body=%s", err, string(r.Body))
	}
	return data
}

func (h *e2eHarness) requireEnvelopeCorrelation(r testResponse) map[string]any {
	h.t.Helper()
	env := r.envelope(h.t)
	if env["request_id"] != "req-"+h.runID || env["trace_id"] != "trace-"+h.runID {
		h.t.Fatalf("envelope correlation = request_id:%#v trace_id:%#v, want req/trace for %s", env["request_id"], env["trace_id"], h.runID)
	}
	return env
}

func (h *e2eHarness) requireEvent(name string, predicate func(contracts.Event) bool) contracts.Event {
	return h.requireEventMatching(name, predicate, false)
}

func (h *e2eHarness) requireCorrelatedEvent(name string, predicate func(contracts.Event) bool) contracts.Event {
	return h.requireEventMatching(name, predicate, true)
}

func (h *e2eHarness) requireEventMatching(name string, predicate func(contracts.Event) bool, correlated bool) contracts.Event {
	h.t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for {
		events := h.outbox()
		for i := len(events) - 1; i >= 0; i-- {
			event := events[i]
			if event.Name == name && (predicate == nil || predicate(event)) {
				requireEventMetadata(h.t, event)
				if correlated {
					h.requireEventCorrelation(event)
				}
				return event
			}
		}
		if time.Now().After(deadline) {
			h.t.Fatalf("missing event %s", name)
		}
		time.Sleep(25 * time.Millisecond)
	}
}

func (h *e2eHarness) countEvents(name string, predicate func(contracts.Event) bool) int {
	h.t.Helper()
	total := 0
	for _, event := range h.outbox() {
		if event.Name == name && (predicate == nil || predicate(event)) {
			total++
		}
	}
	return total
}

func (h *e2eHarness) outbox() []contracts.Event {
	for _, rs := range h.services {
		if rs.app != nil && rs.app.Events != nil {
			return rs.app.Events.Outbox()
		}
	}
	return nil
}

func requireEventMetadata(t *testing.T, event contracts.Event) {
	t.Helper()
	if event.EventID == "" || event.Source == "" || event.TraceID == "" || event.SchemaVersion == 0 || event.OccurredAt.IsZero() {
		t.Fatalf("event metadata incomplete: %#v", event)
	}
}

func (h *e2eHarness) requireEventCorrelation(event contracts.Event) {
	h.t.Helper()
	wantTrace := "trace-" + h.runID
	if event.TraceID != wantTrace {
		h.t.Fatalf("event %s trace_id = %q, want propagated %q", event.Name, event.TraceID, wantTrace)
	}
	wantIDPrefix := "idem-" + h.runID + "-"
	if !strings.HasPrefix(event.IdempotencyKey, wantIDPrefix) {
		h.t.Fatalf("event %s idempotency_key = %q, want prefix %q", event.Name, event.IdempotencyKey, wantIDPrefix)
	}
}

func cloneMap(in map[string]any) map[string]any {
	out := make(map[string]any, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}
