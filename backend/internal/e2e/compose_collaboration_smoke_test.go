//go:build e2e

package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/linskybing/nexuspaas/backend/internal/contracts"
	"github.com/linskybing/nexuspaas/backend/internal/platform"
	"github.com/linskybing/nexuspaas/backend/internal/services"
	"github.com/redis/go-redis/v9"
)

const (
	composeSmokeOptIn = "COMPOSE_COLLABORATION_SMOKE"

	composeRawPoliciesResource = "authorization-policy-service:permission_policies"
)

var composeSmokeServices = []string{
	auditComplianceService,
	authorizationPolicyService,
	ideService,
	identityService,
	imageRegistryService,
	integrationProxyService,
	k8sControlService,
	mediaUploadService,
	orgProjectService,
	"platform-gateway",
	requestNotificationService,
	schedulerQuotaService,
	storageService,
	usageObservabilityService,
	workloadService,
}

type composeSmoke struct {
	t                  *testing.T
	ctx                context.Context
	runID              string
	apiKey             string
	serviceKey         string
	adminPrincipalID   string
	servicePrincipalID string
	urls               map[string]string
	databaseURL        string
	redisURL           string
	eventBusURL        string
	objectStoreURL     string
	objectAccess       string
	objectSecret       string
	objectBucket       string
	summaryJSON        string
	summaryMarkdown    string
	composeProject     string
	composeFile        string
	pool               *pgxpool.Pool
	store              platform.RecordStore
	redis              *redis.Client
	eventRedis         *redis.Client
	objectStore        platform.ObjectStore
	client             *http.Client
	routes             []platform.RouteSpec
	summary            composeSmokeSummary
}

type composeSmokeSummary struct {
	RunID      string                 `json:"run_id"`
	StartedAt  time.Time              `json:"started_at"`
	FinishedAt time.Time              `json:"finished_at"`
	Services   []string               `json:"services"`
	Steps      []composeSmokeStep     `json:"steps"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
}

type composeSmokeStep struct {
	Name       string                 `json:"name"`
	Services   []string               `json:"services_touched"`
	Status     string                 `json:"status"`
	HTTP       []composeSmokeHTTPHop  `json:"http,omitempty"`
	Records    []string               `json:"records_checked,omitempty"`
	Events     []string               `json:"events_checked,omitempty"`
	Details    map[string]interface{} `json:"details,omitempty"`
	Failure    string                 `json:"failure_reason,omitempty"`
	DurationMS int64                  `json:"duration_ms"`
}

type composeSmokeHTTPHop struct {
	Service string `json:"service"`
	Method  string `json:"method"`
	Path    string `json:"path"`
	Status  int    `json:"status"`
	Hop     string `json:"hop,omitempty"`
}

type composeHTTPResponse struct {
	Status int
	Header http.Header
	Body   []byte
}

type composeRequest struct {
	service string
	method  string
	path    string
	body    io.Reader
	payload any
	headers map[string]string
	want    int
	step    *composeSmokeStep
	hop     string
}

func TestComposeCollaborationSmoke(t *testing.T) {
	if strings.TrimSpace(os.Getenv(composeSmokeOptIn)) != "1" {
		t.Skip(composeSmokeOptIn + "=1 is required for Docker compose collaboration smoke")
	}

	s := newComposeSmoke(t)
	t.Cleanup(s.close)
	t.Cleanup(s.writeSummary)

	s.runStep("authorization policy bootstrap", []string{authorizationPolicyService}, func(step *composeSmokeStep) error {
		return s.seedAuthorizationPolicies(step)
	})
	s.runStep("8 unit service registry", composeSmokeServices, func(step *composeSmokeStep) error {
		return s.assertIsolatedServiceRegistries(step)
	})

	ids := s.seedIdentityContracts()
	s.seedSchedulerAdmissionData(ids.userID)

	s.runStep("identity remote auth ignores forged user header", []string{requestNotificationService, identityService}, func(step *composeSmokeStep) error {
		return s.assertRemoteIdentityAuth(step, ids)
	})
	s.runStep("workload submit calls scheduler admission", []string{workloadService, schedulerQuotaService, orgProjectService}, func(step *composeSmokeStep) error {
		return s.assertWorkloadSchedulerSubmit(step, ids)
	})
	s.runStep("scheduler owner-read contracts and bad service credential", []string{schedulerQuotaService, orgProjectService, workloadService}, func(step *composeSmokeStep) error {
		return s.assertSchedulerOwnerReadContracts(step)
	})
	s.runStep("storage mount-plan internal contract", []string{workloadService, storageService}, func(step *composeSmokeStep) error {
		return s.assertStorageMountPlan(step)
	})
	s.runStep("media upload stores metadata and MinIO bytes", []string{mediaUploadService}, func(step *composeSmokeStep) error {
		return s.assertMediaUploadRoundTrip(step)
	})
	s.runStep("request notification emits form and audit events", []string{requestNotificationService, auditComplianceService}, func(step *composeSmokeStep) error {
		return s.assertRequestNotificationEvents(step)
	})
	s.runStep("scheduler outage fails workload submit closed", []string{workloadService, schedulerQuotaService}, func(step *composeSmokeStep) error {
		return s.assertSchedulerOutageFailsClosed(step, ids)
	})
}

func newComposeSmoke(t *testing.T) *composeSmoke {
	t.Helper()
	ctx := t.Context()
	runID := truncateID(sanitizeID(envDefault("COLLAB_SMOKE_RUN_ID", envDefault("CI_GATE_RUN_ID", fmt.Sprint(time.Now().UTC().UnixNano())))), 32)
	urls := requireComposeServiceURLs(t)
	databaseURL := requireEnv(t, "COLLAB_SMOKE_DATABASE_URL")
	redisURL := requireEnv(t, "COLLAB_SMOKE_REDIS_URL")
	eventBusURL := envDefault("COLLAB_SMOKE_EVENT_BUS_URL", redisURL)
	objectStoreURL := requireEnv(t, "COLLAB_SMOKE_OBJECT_STORE_URL")
	objectAccess := requireEnv(t, "TEST_OBJECT_STORE_ACCESS_KEY")
	objectSecret := requireEnv(t, "TEST_OBJECT_STORE_SECRET_KEY")
	objectBucket := envDefault("TEST_OBJECT_STORE_BUCKET", "media-e2e")

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect compose postgres: %v", err)
	}
	store := platform.NewPostgresStore(pool)

	redisOpts, err := redis.ParseURL(redisURL)
	if err != nil {
		t.Fatalf("parse COLLAB_SMOKE_REDIS_URL: %v", err)
	}
	redisClient := redis.NewClient(redisOpts)
	if err := redisClient.Ping(ctx).Err(); err != nil {
		t.Fatalf("ping compose redis: %v", err)
	}

	eventOpts, err := redis.ParseURL(eventBusURL)
	if err != nil {
		t.Fatalf("parse COLLAB_SMOKE_EVENT_BUS_URL: %v", err)
	}
	eventRedis := redis.NewClient(eventOpts)
	if err := eventRedis.Ping(ctx).Err(); err != nil {
		t.Fatalf("ping compose event redis: %v", err)
	}

	objectStore, err := platform.NewMinioObjectStore(ctx, objectStoreURL, objectAccess, objectSecret, objectBucket)
	if err != nil {
		t.Fatalf("connect compose object store: %v", err)
	}

	s := &composeSmoke{
		t:                  t,
		ctx:                ctx,
		runID:              runID,
		apiKey:             requireEnv(t, "TEST_RUNTIME_API_KEY"),
		serviceKey:         requireEnv(t, "TEST_RUNTIME_SERVICE_KEY"),
		adminPrincipalID:   envDefault("COLLAB_SMOKE_ADMIN_PRINCIPAL_ID", "smoke-admin"),
		servicePrincipalID: envDefault("COLLAB_SMOKE_SERVICE_PRINCIPAL_ID", "smoke-service"),
		urls:               urls,
		databaseURL:        databaseURL,
		redisURL:           redisURL,
		eventBusURL:        eventBusURL,
		objectStoreURL:     objectStoreURL,
		objectAccess:       objectAccess,
		objectSecret:       objectSecret,
		objectBucket:       objectBucket,
		summaryJSON:        envDefault("COLLAB_SMOKE_SUMMARY_JSON", filepath.Join(os.TempDir(), "nexuspaas-collaboration-smoke.json")),
		summaryMarkdown:    envDefault("COLLAB_SMOKE_SUMMARY_MD", filepath.Join(os.TempDir(), "nexuspaas-collaboration-smoke.md")),
		composeProject:     strings.TrimSpace(os.Getenv("COMPOSE_COLLABORATION_PROJECT")),
		composeFile:        strings.TrimSpace(os.Getenv("COMPOSE_COLLABORATION_FILE")),
		pool:               pool,
		store:              store,
		redis:              redisClient,
		eventRedis:         eventRedis,
		objectStore:        objectStore,
		client:             &http.Client{Timeout: 10 * time.Second},
		routes:             composeCatalogRoutes(),
		summary: composeSmokeSummary{
			RunID:     runID,
			StartedAt: time.Now().UTC(),
			Services:  append([]string(nil), composeSmokeServices...),
			Metadata: map[string]interface{}{
				"database_url_configured":     databaseURL != "",
				"redis_url_configured":        redisURL != "",
				"event_bus_url_configured":    eventBusURL != "",
				"object_store_url_configured": objectStoreURL != "",
				"compose_project":             strings.TrimSpace(os.Getenv("COMPOSE_COLLABORATION_PROJECT")),
			},
		},
	}
	s.cleanup()
	return s
}

func requireComposeServiceURLs(t *testing.T) map[string]string {
	t.Helper()
	raw := requireEnv(t, "COLLAB_SMOKE_SERVICE_URLS")
	var urls map[string]string
	if err := json.Unmarshal([]byte(raw), &urls); err != nil {
		t.Fatalf("COLLAB_SMOKE_SERVICE_URLS is not valid JSON: %v", err)
	}
	for _, service := range composeSmokeServices {
		if strings.TrimSpace(urls[service]) == "" {
			t.Fatalf("COLLAB_SMOKE_SERVICE_URLS missing %s", service)
		}
		urls[service] = strings.TrimRight(strings.TrimSpace(urls[service]), "/")
	}
	return urls
}

func composeCatalogRoutes() []platform.RouteSpec {
	app := platform.NewApp(platform.Config{ServiceName: "all", HTTPAddr: ":0"})
	services.RegisterAll(app)
	return append([]platform.RouteSpec(nil), app.CatalogRoutes...)
}

func (s *composeSmoke) close() {
	s.cleanup()
	if s.eventRedis != nil {
		_ = s.eventRedis.Close()
	}
	if s.redis != nil {
		_ = s.redis.Close()
	}
	if s.pool != nil {
		s.pool.Close()
	}
}

func (s *composeSmoke) cleanup() {
	s.cleanupPlatformRecords()
	s.cleanupRedisKeys()
	if s.eventRedis != nil {
		s.cleanupEventStream()
	}
	s.cleanupObjectStore()
}

func (s *composeSmoke) cleanupPlatformRecords() {
	if s.store == nil || s.pool == nil {
		return
	}
	_, _ = s.pool.Exec(s.ctx, `
		DELETE FROM platform_records
		WHERE payload->>'e2e_run_id' = $1
		   OR id LIKE '%' || $1 || '%'
		   OR payload::text LIKE '%' || $1 || '%'`, s.runID)
}

func (s *composeSmoke) cleanupRedisKeys() {
	if s.redis == nil {
		return
	}
	keys, err := s.redis.Keys(s.ctx, "*"+s.runID+"*").Result()
	if err == nil && len(keys) > 0 {
		_ = s.redis.Del(s.ctx, keys...).Err()
	}
}

func (s *composeSmoke) cleanupObjectStore() {
	if s.objectStore == nil {
		return
	}
	infos, err := s.objectStore.List(s.ctx)
	if err != nil {
		return
	}
	for _, info := range infos {
		if strings.Contains(info.Key, s.runID) {
			_ = s.objectStore.Delete(s.ctx, info.Key)
		}
	}
}

func (s *composeSmoke) cleanupEventStream() {
	messages, err := s.eventRedis.XRange(s.ctx, "events", "-", "+").Result()
	if err != nil {
		return
	}
	var ids []string
	for _, msg := range messages {
		raw, ok := msg.Values["event"].(string)
		if ok && strings.Contains(raw, s.runID) {
			ids = append(ids, msg.ID)
		}
	}
	if len(ids) > 0 {
		_ = s.eventRedis.XDel(s.ctx, "events", ids...).Err()
	}
}

func (s *composeSmoke) runStep(name string, services []string, fn func(*composeSmokeStep) error) {
	step := composeSmokeStep{
		Name:     name,
		Services: append([]string(nil), services...),
		Status:   "running",
		Details:  map[string]interface{}{},
	}
	s.summary.Steps = append(s.summary.Steps, step)
	idx := len(s.summary.Steps) - 1
	start := time.Now()
	defer func() {
		s.summary.Steps[idx].DurationMS = time.Since(start).Milliseconds()
		if s.summary.Steps[idx].Status == "running" {
			s.summary.Steps[idx].Status = "failed"
			s.summary.Steps[idx].Failure = "test aborted; see collaboration-smoke.log for the exact assertion"
		}
	}()
	if err := fn(&s.summary.Steps[idx]); err != nil {
		s.summary.Steps[idx].Status = "failed"
		s.summary.Steps[idx].Failure = err.Error()
		s.t.Fatalf("%s: %v", name, err)
	}
	s.summary.Steps[idx].Status = "passed"
}

func (s *composeSmoke) writeSummary() {
	s.summary.FinishedAt = time.Now().UTC()
	if s.summaryJSON != "" {
		_ = os.MkdirAll(filepath.Dir(s.summaryJSON), 0o755)
		raw, err := json.MarshalIndent(s.summary, "", "  ")
		if err == nil {
			_ = os.WriteFile(s.summaryJSON, append(raw, '\n'), 0o644)
		}
	}
	if s.summaryMarkdown != "" {
		_ = os.MkdirAll(filepath.Dir(s.summaryMarkdown), 0o755)
		_ = os.WriteFile(s.summaryMarkdown, []byte(s.summaryMarkdownBody()), 0o644)
	}
}

func (s *composeSmoke) summaryMarkdownBody() string {
	var b strings.Builder
	fmt.Fprintf(&b, "# 15-Service Collaboration Smoke\n\n")
	fmt.Fprintf(&b, "- Run ID: `%s`\n", s.runID)
	fmt.Fprintf(&b, "- Started: `%s`\n", s.summary.StartedAt.Format(time.RFC3339))
	fmt.Fprintf(&b, "- Finished: `%s`\n", s.summary.FinishedAt.Format(time.RFC3339))
	fmt.Fprintf(&b, "- Services: `%d`\n\n", len(s.summary.Services))
	fmt.Fprintf(&b, "| Workflow | Status | Services | HTTP | Records | Events | Failure |\n")
	fmt.Fprintf(&b, "| --- | --- | --- | --- | --- | --- | --- |\n")
	for _, step := range s.summary.Steps {
		httpParts := make([]string, 0, len(step.HTTP))
		for _, hop := range step.HTTP {
			httpParts = append(httpParts, fmt.Sprintf("%s %s %s -> %d", hop.Service, hop.Method, hop.Path, hop.Status))
		}
		fmt.Fprintf(&b, "| %s | %s | %s | %s | %s | %s | %s |\n",
			mdCell(step.Name),
			mdCell(step.Status),
			mdCell(strings.Join(step.Services, ", ")),
			mdCell(strings.Join(httpParts, "<br>")),
			mdCell(strings.Join(step.Records, "<br>")),
			mdCell(strings.Join(step.Events, "<br>")),
			mdCell(step.Failure),
		)
	}
	return b.String()
}

func mdCell(value string) string {
	value = strings.ReplaceAll(value, "|", "\\|")
	value = strings.ReplaceAll(value, "\n", "<br>")
	if value == "" {
		return "-"
	}
	return value
}

func (s *composeSmoke) seedAuthorizationPolicies(step *composeSmokeStep) error {
	subjects := []string{s.adminPrincipalID, s.servicePrincipalID}
	routePolicies := []struct {
		method  string
		pattern string
	}{
		{http.MethodPost, "/api/v1/jobs"},
		{http.MethodPost, "/api/v1/internal/scheduler/admission"},
		{http.MethodPost, "/api/v1/forms"},
		{http.MethodGet, "/api/v1/forms/my"},
		{http.MethodPost, "/api/v1/uploads/images"},
		{http.MethodGet, "/api/v1/uploads/images/{key...}"},
		{http.MethodPost, "/api/v1/permissions/enforce"},
	}
	for _, subject := range subjects {
		if err := s.createPolicy(subject, "", "platform-runtime:service-registry", "platform_runtime_service_registry"); err != nil {
			return err
		}
	}
	for _, routePolicy := range routePolicies {
		route, ok := s.route(routePolicy.method, routePolicy.pattern)
		if !ok {
			return fmt.Errorf("missing catalog route %s %s", routePolicy.method, routePolicy.pattern)
		}
		for _, subject := range subjects {
			if err := s.createPolicy(subject, "", route.Resource, route.OperationID); err != nil {
				return err
			}
		}
	}
	step.Records = append(step.Records, composeRawPoliciesResource)
	step.Details["subjects"] = subjects
	step.Details["route_policies"] = len(routePolicies)

	resp, err := s.doJSON(composeRequest{
		service: authorizationPolicyService,
		method:  http.MethodPost,
		path:    "/api/v1/permissions/enforce",
		payload: map[string]any{
			"sub": s.adminPrincipalID,
			"dom": "",
			"obj": "platform-runtime:service-registry",
			"act": "platform_runtime_service_registry",
		},
		headers: map[string]string{"X-API-Key": s.serviceKey, "X-Service-Name": "platform-gateway", "X-Service-Key": s.serviceKey},
		want:    http.StatusOK,
		step:    step,
		hop:     "policy service enforces seeded raw policy",
	})
	if err != nil {
		return err
	}
	data, err := envelopeDataMap(resp)
	if err != nil {
		return err
	}
	if allowed, _ := data["allowed"].(bool); !allowed {
		return fmt.Errorf("authorization policy enforce decision = %#v, want allowed", data)
	}
	return nil
}

func (s *composeSmoke) route(method, pattern string) (platform.RouteSpec, bool) {
	for _, route := range s.routes {
		if route.Method == method && route.Pattern == pattern {
			return route, true
		}
	}
	return platform.RouteSpec{}, false
}

func (s *composeSmoke) createPolicy(subject, domain, object, action string) error {
	policy := []string{subject, domain, object, action}
	id := strings.Join(policy, "\x1f")
	data := map[string]any{
		"id":         id,
		"policy":     policy,
		"v0":         subject,
		"v1":         domain,
		"v2":         object,
		"v3":         action,
		"e2e_run_id": s.runID,
	}
	if _, err := s.store.Create(s.ctx, composeRawPoliciesResource, data); err != nil && !platform.IsCreateConflict(err) {
		return fmt.Errorf("create raw permission policy %q: %w", id, err)
	}
	return nil
}

func (s *composeSmoke) createRoutePolicy(subject, method, pattern string) (platform.RouteSpec, error) {
	route, ok := s.route(method, pattern)
	if !ok {
		return platform.RouteSpec{}, fmt.Errorf("missing catalog route %s %s", method, pattern)
	}
	if err := s.createPolicy(subject, "", route.Resource, route.OperationID); err != nil {
		return platform.RouteSpec{}, err
	}
	return route, nil
}

func (s *composeSmoke) assertPolicyAllowed(step *composeSmokeStep, subject string, route platform.RouteSpec) error {
	resp, err := s.doJSON(composeRequest{
		service: authorizationPolicyService,
		method:  http.MethodPost,
		path:    "/api/v1/permissions/enforce",
		payload: map[string]any{
			"sub": subject,
			"dom": "",
			"obj": route.Resource,
			"act": route.OperationID,
		},
		headers: map[string]string{"X-API-Key": s.serviceKey, "X-Service-Name": "platform-gateway", "X-Service-Key": s.serviceKey},
		want:    http.StatusOK,
		step:    step,
		hop:     "policy service enforces workflow route policy",
	})
	if err != nil {
		return err
	}
	data, err := envelopeDataMap(resp)
	if err != nil {
		return err
	}
	if allowed, _ := data["allowed"].(bool); !allowed {
		return fmt.Errorf("authorization policy denied %s %s/%s", subject, route.Resource, route.OperationID)
	}
	return nil
}

func (s *composeSmoke) assertIsolatedServiceRegistries(step *composeSmokeStep) error {
	for _, service := range composeSmokeServices {
		resp, err := s.do(composeRequest{
			service: service,
			method:  http.MethodGet,
			path:    "/service-registry",
			headers: map[string]string{"X-API-Key": s.apiKey},
			want:    http.StatusOK,
			step:    step,
			hop:     "service must expose only its isolated registry",
		})
		if err != nil {
			return err
		}
		var env struct {
			Data []struct {
				Name string `json:"name"`
			} `json:"data"`
		}
		if err := json.Unmarshal(resp.Body, &env); err != nil {
			return fmt.Errorf("%s service registry decode: %w", service, err)
		}
		got := make([]string, 0, len(env.Data))
		for _, entry := range env.Data {
			got = append(got, entry.Name)
		}
		sort.Strings(got)
		want := expectedComposeRegistryServices(service)
		if strings.Join(got, ",") != strings.Join(want, ",") {
			return fmt.Errorf("%s service-registry = %v, want %v", service, got, want)
		}
	}
	step.Records = append(step.Records, "service-registry entries: 8 unit views covering 15 logical services")
	return nil
}

func expectedComposeRegistryServices(service string) []string {
	switch service {
	case "platform-gateway":
		return []string{"platform-gateway"}
	case identityService, authorizationPolicyService:
		return []string{authorizationPolicyService, identityService}
	case orgProjectService:
		return []string{orgProjectService}
	case auditComplianceService, requestNotificationService, mediaUploadService:
		return []string{auditComplianceService, mediaUploadService, requestNotificationService}
	case storageService, imageRegistryService, integrationProxyService:
		return []string{imageRegistryService, integrationProxyService, storageService}
	case usageObservabilityService:
		return []string{usageObservabilityService}
	case workloadService, ideService:
		return []string{ideService, workloadService}
	case schedulerQuotaService, k8sControlService:
		return []string{k8sControlService, schedulerQuotaService}
	default:
		return []string{service}
	}
}

func (s *composeSmoke) seedIdentityContracts() identityIDs {
	h := s.harness()
	return h.seedIdentityContracts()
}

func (s *composeSmoke) seedSchedulerAdmissionData(userID string) {
	h := s.harness()
	h.seedSchedulerAdmissionData(userID)
}

func (s *composeSmoke) harness() *e2eHarness {
	h := &e2eHarness{
		t:              s.t,
		ctx:            s.ctx,
		runID:          s.runID,
		apiKey:         s.apiKey,
		serviceKey:     s.serviceKey,
		databaseURL:    s.databaseURL,
		redisURL:       s.redisURL,
		eventBusURL:    s.eventBusURL,
		objectStoreURL: s.objectStoreURL,
		objectAccess:   s.objectAccess,
		objectSecret:   s.objectSecret,
		objectBucket:   s.objectBucket,
		pool:           s.pool,
		redis:          s.redis,
		store:          s.store,
		objectStore:    s.objectStore,
		services:       map[string]*runningService{},
	}
	for name, url := range s.urls {
		h.services[name] = &runningService{name: name, url: url}
	}
	return h
}

func (s *composeSmoke) assertRemoteIdentityAuth(step *composeSmokeStep, ids identityIDs) error {
	formsMyRoute, err := s.createRoutePolicy(ids.userID, http.MethodGet, "/api/v1/forms/my")
	if err != nil {
		return err
	}
	forgedUserID := "FORGED"
	if _, err := s.createRoutePolicy(forgedUserID, http.MethodGet, "/api/v1/forms/my"); err != nil {
		return err
	}
	if err := s.assertPolicyAllowed(step, ids.userID, formsMyRoute); err != nil {
		return err
	}
	if err := s.assertPolicyAllowed(step, forgedUserID, formsMyRoute); err != nil {
		return err
	}
	formID := "form" + s.runID
	otherID := "forged" + s.runID
	now := time.Now().UTC().Format(time.RFC3339)
	if err := s.createRecord(formsResource, formID, map[string]any{
		"user_id":     ids.userID,
		"title":       "mine",
		"description": "visible",
		"tag":         "",
		"status":      "Pending",
		"created_at":  now,
		"updated_at":  now,
	}); err != nil {
		return err
	}
	if err := s.createRecord(formsResource, otherID, map[string]any{
		"user_id":     forgedUserID,
		"title":       "not mine",
		"description": "must not be visible",
		"tag":         "",
		"status":      "Pending",
		"created_at":  now,
		"updated_at":  now,
	}); err != nil {
		return err
	}
	resp, err := s.do(composeRequest{
		service: requestNotificationService,
		method:  http.MethodGet,
		path:    "/api/v1/forms/my",
		headers: map[string]string{
			"Authorization": "Bearer " + ids.session,
			"X-User-ID":     forgedUserID,
		},
		want: http.StatusOK,
		step: step,
		hop:  "request-notification -> identity session auth",
	})
	if err != nil {
		return err
	}
	var env struct {
		Data []map[string]any `json:"data"`
	}
	if err := json.Unmarshal(resp.Body, &env); err != nil {
		return fmt.Errorf("decode forms/my response: %w", err)
	}
	if len(env.Data) != 1 || env.Data[0]["id"] != formID {
		return fmt.Errorf("forms/my data = %#v, want only real authenticated user form %s", env.Data, formID)
	}
	step.Records = append(step.Records, formsResource+"/"+formID, formsResource+"/"+otherID)
	return nil
}

func (s *composeSmoke) assertWorkloadSchedulerSubmit(step *composeSmokeStep, ids identityIDs) error {
	beforeJobs := s.countRunRecords(workloadJobsResource)
	beforeAdmissions := s.countRunRecords(schedulerAdmissionsResource)
	resp, err := s.submitJob(step, ids, "collabjob"+s.runID, http.StatusCreated)
	if err != nil {
		return err
	}
	data, err := envelopeDataMap(resp)
	if err != nil {
		return err
	}
	jobID, _ := data["id"].(string)
	jobData, _ := data["data"].(map[string]any)
	if jobID == "" || jobData["status"] != "submitted" || jobData["project_id"] != s.projectID() {
		return fmt.Errorf("submitted job = %#v, want submitted project job", data)
	}
	if after := s.countRunRecords(workloadJobsResource); after != beforeJobs+1 {
		return fmt.Errorf("workload job count = %d, want %d", after, beforeJobs+1)
	}
	if after := s.countRunRecords(schedulerAdmissionsResource); after != beforeAdmissions+1 {
		return fmt.Errorf("scheduler admission count = %d, want %d", after, beforeAdmissions+1)
	}
	admissionID := s.projectID() + "/" + ids.userID + "/" + s.queueName()
	admission, ok := s.store.Get(s.ctx, schedulerAdmissionsResource, admissionID)
	if !ok || admission.Data["allowed"] != true {
		return fmt.Errorf("admission %s = %#v found=%v, want allowed", admissionID, admission.Data, ok)
	}
	if err := s.requireEvent(step, "SubmitAdmissionReviewed", func(event contracts.Event) bool {
		return event.Source == schedulerQuotaService && event.Data["project_id"] == s.projectID()
	}); err != nil {
		return err
	}
	if err := s.requireEvent(step, "JobSubmitted", func(event contracts.Event) bool {
		return event.Source == workloadService && event.Data["job_id"] == jobID
	}); err != nil {
		return err
	}
	step.Records = append(step.Records, workloadJobsResource+"/"+jobID, schedulerAdmissionsResource+"/"+admissionID)
	return nil
}

func (s *composeSmoke) submitJob(step *composeSmokeStep, ids identityIDs, jobID string, want int) (composeHTTPResponse, error) {
	return s.doJSON(composeRequest{
		service: workloadService,
		method:  http.MethodPost,
		path:    "/api/v1/jobs",
		payload: map[string]any{
			"job_id":          jobID,
			"project_id":      s.projectID(),
			"user_id":         ids.userID,
			"queue_name":      s.queueName(),
			"required_cpu":    1,
			"required_memory": 1024,
			"e2e_run_id":      s.runID,
		},
		headers: map[string]string{"X-API-Key": s.apiKey, "Idempotency-Key": "idem-" + s.runID + "-" + sanitizeID(jobID)},
		want:    want,
		step:    step,
		hop:     "workload -> scheduler-quota admission",
	})
}

func (s *composeSmoke) assertSchedulerOwnerReadContracts(step *composeSmokeStep) error {
	userID := "owneruser" + s.runID
	badUserID := "badowneruser" + s.runID
	projectID, queueName := s.seedSchedulerOwnerReadAdmissionData(userID, badUserID)
	resp, err := s.doJSON(composeRequest{
		service: schedulerQuotaService,
		method:  http.MethodPost,
		path:    "/api/v1/internal/scheduler/admission",
		payload: map[string]any{
			"job_id":          "ownerjobsubmit" + s.runID,
			"project_id":      projectID,
			"user_id":         userID,
			"queue_name":      queueName,
			"required_cpu":    1,
			"required_memory": 1024,
			"e2e_run_id":      s.runID,
		},
		headers: map[string]string{"X-API-Key": s.serviceKey, "X-Service-Name": "platform-gateway", "X-Service-Key": s.serviceKey},
		want:    http.StatusOK,
		step:    step,
		hop:     "scheduler -> org-project/workload owner-read",
	})
	if err != nil {
		return err
	}
	data, err := envelopeDataMap(resp)
	if err != nil {
		return err
	}
	if data["allowed"] != true || data["project_id"] != projectID || data["user_id"] != userID {
		return fmt.Errorf("owner-read admission = %#v, want allowed owner snapshot", data)
	}
	usage, ok := data["usage"].(map[string]any)
	if !ok {
		return fmt.Errorf("owner-read usage = %#v, want object", data["usage"])
	}
	if usage["user_running_jobs"] != float64(1) {
		return fmt.Errorf("owner-read user_running_jobs = %#v, want 1", usage["user_running_jobs"])
	}
	beforeAdmissions := s.countRunRecords(schedulerAdmissionsResource)
	beforeJobs := s.countRunRecords(workloadJobsResource)
	if _, err := s.doJSON(composeRequest{
		service: schedulerQuotaService,
		method:  http.MethodPost,
		path:    "/api/v1/internal/scheduler/admission",
		payload: map[string]any{
			"job_id":          "ownerbadjob" + s.runID,
			"project_id":      projectID,
			"user_id":         badUserID,
			"queue_name":      queueName,
			"required_cpu":    1,
			"required_memory": 1024,
			"e2e_run_id":      s.runID,
		},
		headers: map[string]string{"X-API-Key": "wrong-" + s.serviceKey, "X-Service-Name": "platform-gateway", "X-Service-Key": "wrong-" + s.serviceKey},
		want:    http.StatusUnauthorized,
		step:    step,
		hop:     "bad service credential -> scheduler internal route",
	}); err != nil {
		return err
	}
	if after := s.countRunRecords(schedulerAdmissionsResource); after != beforeAdmissions {
		return fmt.Errorf("admissions after bad service credential = %d, want unchanged %d", after, beforeAdmissions)
	}
	if after := s.countRunRecords(workloadJobsResource); after != beforeJobs {
		return fmt.Errorf("workload jobs after bad service credential = %d, want unchanged %d", after, beforeJobs)
	}
	if _, err := s.do(composeRequest{
		service: orgProjectService,
		method:  http.MethodGet,
		path:    "/internal/org-project/projects/" + projectID,
		headers: map[string]string{"X-Service-Name": "platform-gateway", "X-Service-Key": "wrong-" + s.serviceKey},
		want:    http.StatusUnauthorized,
		step:    step,
		hop:     "bad service credential -> org-project owner-read",
	}); err != nil {
		return err
	}
	step.Records = append(step.Records, schedulerAdmissionsResource+"/"+projectID+"/"+userID+"/"+queueName, workloadJobsResource+"/ownerrunning"+s.runID)
	return nil
}

func (s *composeSmoke) seedSchedulerOwnerReadAdmissionData(userID, badUserID string) (string, string) {
	h := s.harness()
	return h.seedSchedulerOwnerReadAdmissionData(userID, badUserID)
}

func (s *composeSmoke) assertStorageMountPlan(step *composeSmokeStep) error {
	h := s.harness()
	ids := h.seedStorageMountPlanRecords()
	body, _ := json.Marshal(storageMountPlanE2ERequest(ids))
	resp, err := s.do(composeRequest{
		service: storageService,
		method:  http.MethodPost,
		path:    "/internal/storage/projects/" + ids.projectID + "/mount-plan",
		body:    bytes.NewReader(body),
		headers: map[string]string{
			"Content-Type":   "application/json",
			"X-Service-Name": "platform-gateway",
			"X-Service-Key":  s.serviceKey,
		},
		want: http.StatusOK,
		step: step,
		hop:  "workload -> storage mount-plan",
	})
	if err != nil {
		return err
	}
	var env struct {
		Data storageMountPlanE2EResponse `json:"data"`
	}
	if err := json.Unmarshal(resp.Body, &env); err != nil {
		return fmt.Errorf("decode storage mount plan: %w", err)
	}
	plan := env.Data
	if plan.ProjectID != ids.projectID || plan.UserID != ids.userID || plan.Namespace != ids.namespace {
		return fmt.Errorf("storage plan identity = %#v, want seeded project/user/namespace", plan)
	}
	if len(plan.PVCShareOperations) != 1 ||
		plan.PVCShareOperations[0].SourceNamespace != ids.sourceNamespace ||
		plan.PVCShareOperations[0].SourcePVC != ids.sourcePVC ||
		plan.PVCShareOperations[0].TargetPVC != ids.targetPVC {
		return fmt.Errorf("share operations = %#v, want storage-owned PVC refs", plan.PVCShareOperations)
	}
	if _, err := s.do(composeRequest{
		service: storageService,
		method:  http.MethodPost,
		path:    "/internal/storage/projects/" + ids.projectID + "/mount-plan",
		body:    bytes.NewReader(body),
		headers: map[string]string{
			"Content-Type":   "application/json",
			"X-Service-Name": "platform-gateway",
			"X-Service-Key":  "wrong-" + s.serviceKey,
		},
		want: http.StatusUnauthorized,
		step: step,
		hop:  "bad service credential -> storage mount-plan",
	}); err != nil {
		return err
	}
	step.Records = append(step.Records, e2eStorageBindingsResource+"/"+ids.projectID+":"+ids.pvcID, e2eStorageGroupResource+"/"+ids.groupID+":"+ids.pvcID, e2eStoragePermissionsResource+"/"+ids.projectID+":"+ids.pvcID+":"+ids.userID)
	return nil
}

func (s *composeSmoke) assertMediaUploadRoundTrip(step *composeSmokeStep) error {
	key, err := s.uploadMediaImage(step)
	if err != nil {
		return err
	}
	if err := s.assertMediaMetadataAndObject(key); err != nil {
		return err
	}
	if err := s.assertMediaPublicRead(step, key); err != nil {
		return err
	}
	step.Records = append(step.Records, mediaResource+"/"+key, "minio:"+key)
	return nil
}

func (s *composeSmoke) uploadMediaImage(step *composeSmokeStep) (string, error) {
	filename := s.runID + ".png"
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	part, err := writer.CreatePart(textprotoMIMEHeader("file", filename, "image/png"))
	if err != nil {
		return "", err
	}
	if _, err := part.Write(pngBytes()); err != nil {
		return "", err
	}
	if err := writer.Close(); err != nil {
		return "", err
	}
	resp, err := s.do(composeRequest{
		service: mediaUploadService,
		method:  http.MethodPost,
		path:    "/api/v1/uploads/images",
		body:    &buf,
		headers: map[string]string{
			"Content-Type": writer.FormDataContentType(),
			"X-API-Key":    s.apiKey,
		},
		want: http.StatusOK,
		step: step,
		hop:  "media-upload -> MinIO",
	})
	if err != nil {
		return "", err
	}
	uploaded, err := rawMap(resp)
	if err != nil {
		return "", err
	}
	key, _ := uploaded["key"].(string)
	if key == "" || !strings.Contains(key, s.runID) {
		return "", fmt.Errorf("upload key = %#v, want key containing run id", uploaded["key"])
	}
	return key, nil
}

func (s *composeSmoke) assertMediaMetadataAndObject(key string) error {
	record, ok := s.store.Get(s.ctx, mediaResource, key)
	if !ok {
		return fmt.Errorf("media metadata %s missing", key)
	}
	if _, inline := record.Data["body_base64"]; inline {
		return fmt.Errorf("media metadata contains inline body")
	}
	body, contentType, found, err := s.objectStore.Get(s.ctx, key)
	if err != nil || !found {
		return fmt.Errorf("object store get %s found=%v err=%v", key, found, err)
	}
	if contentType != "image/png" || !bytes.Equal(body, pngBytes()) {
		return fmt.Errorf("object %s = %q/%v, want uploaded png", key, contentType, body)
	}
	return nil
}

func (s *composeSmoke) assertMediaPublicRead(step *composeSmokeStep, key string) error {
	served, err := s.do(composeRequest{
		service: mediaUploadService,
		method:  http.MethodGet,
		path:    "/api/v1/uploads/images/" + key,
		headers: map[string]string{"X-API-Key": s.apiKey},
		want:    http.StatusOK,
		step:    step,
		hop:     "media-upload public read path",
	})
	if err != nil {
		return err
	}
	if !bytes.Equal(served.Body, pngBytes()) || !strings.HasPrefix(served.Header.Get("Content-Type"), "image/png") {
		return fmt.Errorf("served image content-type=%q len=%d, want uploaded png", served.Header.Get("Content-Type"), len(served.Body))
	}
	return nil
}

func (s *composeSmoke) assertRequestNotificationEvents(step *composeSmokeStep) error {
	resp, err := s.doJSON(composeRequest{
		service: requestNotificationService,
		method:  http.MethodPost,
		path:    "/api/v1/forms",
		payload: map[string]any{
			"title":       "E2E " + s.runID,
			"description": "cross-service event check",
			"e2e_run_id":  s.runID,
		},
		headers: map[string]string{"X-API-Key": s.apiKey},
		want:    http.StatusCreated,
		step:    step,
		hop:     "request-notification creates form",
	})
	if err != nil {
		return err
	}
	form, err := envelopeDataMap(resp)
	if err != nil {
		return err
	}
	formID, _ := form["id"].(string)
	if formID == "" {
		return fmt.Errorf("created form response = %#v, missing id", form)
	}
	if _, ok := s.store.Get(s.ctx, formsResource, formID); !ok {
		return fmt.Errorf("form record %s missing", formID)
	}
	if err := s.requireEvent(step, "FormCreated", func(event contracts.Event) bool {
		return event.Source == requestNotificationService && event.Data["id"] == formID
	}); err != nil {
		return err
	}
	if err := s.requireEvent(step, "AuditEvent", func(event contracts.Event) bool {
		return event.Data["resource"] == requestNotificationService+":forms" && event.Data["success"] == true
	}); err != nil {
		return err
	}
	step.Records = append(step.Records, formsResource+"/"+formID)
	return nil
}

func (s *composeSmoke) assertSchedulerOutageFailsClosed(step *composeSmokeStep, ids identityIDs) error {
	if s.composeProject == "" || s.composeFile == "" {
		return fmt.Errorf("COMPOSE_COLLABORATION_PROJECT and COMPOSE_COLLABORATION_FILE are required for scheduler outage scenario")
	}
	if err := s.stopComposeService(schedulerQuotaService); err != nil {
		return err
	}
	beforeJobs := s.countRunRecords(workloadJobsResource)
	beforeAdmissions := s.countRunRecords(schedulerAdmissionsResource)
	if _, err := s.submitJob(step, ids, "outagejob"+s.runID, http.StatusServiceUnavailable); err != nil {
		return err
	}
	if after := s.countRunRecords(workloadJobsResource); after != beforeJobs {
		return fmt.Errorf("workload jobs after scheduler outage = %d, want unchanged %d", after, beforeJobs)
	}
	if after := s.countRunRecords(schedulerAdmissionsResource); after != beforeAdmissions {
		return fmt.Errorf("scheduler admissions after scheduler outage = %d, want unchanged %d", after, beforeAdmissions)
	}
	step.Records = append(step.Records, "no new "+workloadJobsResource, "no new "+schedulerAdmissionsResource)
	return nil
}

func (s *composeSmoke) stopComposeService(service string) error {
	cmd := exec.CommandContext(s.ctx, "docker", "compose", "-p", s.composeProject, "-f", s.composeFile, "stop", composePhysicalServiceName(service))
	cmd.Env = os.Environ()
	raw, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("stop compose service %s: %w: %s", service, err, string(raw))
	}
	return nil
}

func composePhysicalServiceName(service string) string {
	switch service {
	case identityService, authorizationPolicyService:
		return "iam-unit"
	case orgProjectService:
		return "tenant-unit"
	case auditComplianceService, requestNotificationService, mediaUploadService:
		return "collaboration-unit"
	case storageService, imageRegistryService, integrationProxyService:
		return "platform-io-unit"
	case usageObservabilityService:
		return "usage-observability"
	case workloadService, ideService:
		return "compute-api"
	case schedulerQuotaService, k8sControlService:
		return "compute-control-plane"
	default:
		return service
	}
}

func (s *composeSmoke) doJSON(req composeRequest) (composeHTTPResponse, error) {
	if req.payload != nil {
		raw, err := json.Marshal(req.payload)
		if err != nil {
			return composeHTTPResponse{}, err
		}
		req.body = bytes.NewReader(raw)
		req.headers = cloneHeaders(req.headers)
		req.headers["Content-Type"] = "application/json"
	}
	return s.do(req)
}

func (s *composeSmoke) do(req composeRequest) (composeHTTPResponse, error) {
	baseURL := strings.TrimRight(s.urls[req.service], "/")
	if baseURL == "" {
		return composeHTTPResponse{}, fmt.Errorf("missing URL for service %s", req.service)
	}
	httpReq, err := http.NewRequestWithContext(s.ctx, req.method, baseURL+req.path, req.body)
	if err != nil {
		return composeHTTPResponse{}, err
	}
	for key, value := range req.headers {
		httpReq.Header.Set(key, value)
	}
	httpReq.Header.Set("X-Request-ID", "req-"+s.runID)
	httpReq.Header.Set("X-Trace-ID", "trace-"+s.runID)
	// Route-derived default only; steps that issue distinct logical requests on
	// the same route must supply their own Idempotency-Key or the server will
	// (correctly) reject the second body as a key conflict.
	if httpReq.Header.Get("Idempotency-Key") == "" {
		httpReq.Header.Set("Idempotency-Key", "idem-"+s.runID+"-"+sanitizeID(req.service+req.method+req.path))
	}
	resp, err := s.client.Do(httpReq)
	if err != nil {
		req.step.HTTP = append(req.step.HTTP, composeSmokeHTTPHop{Service: req.service, Method: req.method, Path: req.path, Status: 0, Hop: req.hop})
		return composeHTTPResponse{}, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return composeHTTPResponse{}, err
	}
	req.step.HTTP = append(req.step.HTTP, composeSmokeHTTPHop{Service: req.service, Method: req.method, Path: req.path, Status: resp.StatusCode, Hop: req.hop})
	if resp.StatusCode != req.want {
		return composeHTTPResponse{Status: resp.StatusCode, Header: resp.Header.Clone(), Body: raw}, fmt.Errorf("%s %s %s returned %d, want %d: %s", req.service, req.method, req.path, resp.StatusCode, req.want, string(raw))
	}
	return composeHTTPResponse{Status: resp.StatusCode, Header: resp.Header.Clone(), Body: raw}, nil
}

func cloneHeaders(headers map[string]string) map[string]string {
	cloned := make(map[string]string, len(headers)+1)
	for key, value := range headers {
		cloned[key] = value
	}
	return cloned
}

func (s *composeSmoke) createRecord(resource, id string, data map[string]any) error {
	data = cloneStringAny(data)
	data["id"] = id
	data["e2e_run_id"] = s.runID
	if _, err := s.store.Create(s.ctx, resource, data); err != nil && !platform.IsCreateConflict(err) {
		return fmt.Errorf("create %s/%s: %w", resource, id, err)
	}
	return nil
}

func (s *composeSmoke) countRunRecords(resource string) int {
	total := 0
	for _, record := range s.store.List(s.ctx, resource) {
		if strings.Contains(record.ID, s.runID) || strings.Contains(fmt.Sprint(record.Data), s.runID) {
			total++
		}
	}
	return total
}

func (s *composeSmoke) requireEvent(step *composeSmokeStep, name string, predicate func(contracts.Event) bool) error {
	deadline := time.Now().Add(10 * time.Second)
	for {
		event, found, err := s.findEvent(name, predicate)
		if err != nil {
			return err
		}
		if found {
			if err := validateEventMetadata(event); err != nil {
				return err
			}
			step.Events = append(step.Events, event.Source+"/"+name)
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("missing event %s containing run id %s", name, s.runID)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func (s *composeSmoke) findEvent(name string, predicate func(contracts.Event) bool) (contracts.Event, bool, error) {
	events, err := s.events()
	if err != nil {
		return contracts.Event{}, false, err
	}
	for i := len(events) - 1; i >= 0; i-- {
		event := events[i]
		if eventSatisfies(event, name, s.runID, predicate) {
			return event, true, nil
		}
	}
	return contracts.Event{}, false, nil
}

func eventSatisfies(event contracts.Event, name, runID string, predicate func(contracts.Event) bool) bool {
	return event.Name == name &&
		eventMatchesRun(event, runID) &&
		(predicate == nil || predicate(event))
}

func validateEventMetadata(event contracts.Event) error {
	if event.TraceID == "" || event.EventID == "" || event.Source == "" || event.SchemaVersion == 0 {
		return fmt.Errorf("event metadata incomplete: %#v", event)
	}
	return nil
}

func eventMatchesRun(event contracts.Event, runID string) bool {
	return strings.Contains(fmt.Sprint(event.Data), runID) ||
		strings.Contains(event.EventID, runID) ||
		strings.Contains(event.TraceID, runID) ||
		strings.Contains(event.IdempotencyKey, runID)
}

func (s *composeSmoke) events() ([]contracts.Event, error) {
	messages, err := s.eventRedis.XRange(s.ctx, "events", "-", "+").Result()
	if err != nil {
		return nil, err
	}
	events := make([]contracts.Event, 0, len(messages))
	for _, msg := range messages {
		raw, ok := msg.Values["event"].(string)
		if !ok {
			continue
		}
		var event contracts.Event
		if err := json.Unmarshal([]byte(raw), &event); err == nil {
			events = append(events, event)
		}
	}
	return events, nil
}

func (s *composeSmoke) projectID() string { return "project" + s.runID }
func (s *composeSmoke) queueName() string { return "queue-" + s.runID }

func envelopeDataMap(resp composeHTTPResponse) (map[string]any, error) {
	var env struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(resp.Body, &env); err != nil {
		return nil, fmt.Errorf("decode envelope: %w body=%s", err, string(resp.Body))
	}
	if env.Data == nil {
		return nil, fmt.Errorf("envelope data missing: %s", string(resp.Body))
	}
	return env.Data, nil
}

func rawMap(resp composeHTTPResponse) (map[string]any, error) {
	var data map[string]any
	if err := json.Unmarshal(resp.Body, &data); err != nil {
		return nil, fmt.Errorf("decode raw response: %w body=%s", err, string(resp.Body))
	}
	return data, nil
}

func cloneStringAny(in map[string]any) map[string]any {
	out := make(map[string]any, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func init() {
	sort.Strings(composeSmokeServices)
}
