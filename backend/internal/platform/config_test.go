package platform

import (
	"net"
	"net/http"
	"strings"
	"testing"
	"time"
)

const (
	testAPIKey      = "key"
	testDatabaseURL = "postgres://db/app"
	testRedisURL    = "redis://redis:6379/0"
	testEventBusURL = "redis://events:6379/1"
	testPolicyURL   = "http://authorization-policy-service"
	testObjectURL   = "http://minio:9000"
	testPolicyKey   = "policy-key"
)

func TestConfigAllowedOriginsUsesFallbackInNonProduction(t *testing.T) {
	t.Setenv("PRODUCTION", "false")
	t.Setenv("ALLOWED_ORIGINS", "")

	cfg := ConfigFromEnv()
	want := defaultDevAllowedOrigins()
	if len(cfg.AllowedOrigins) != len(want) {
		t.Fatalf("allowed origins length = %d, want %d: %#v", len(cfg.AllowedOrigins), len(want), cfg.AllowedOrigins)
	}
	for origin := range want {
		if !cfg.AllowedOrigins[origin] {
			t.Fatalf("missing dev fallback origin %q in %#v", origin, cfg.AllowedOrigins)
		}
	}
}

func TestConfigAuthDefaultsFailClosed(t *testing.T) {
	t.Setenv("REQUIRE_AUTH", "")
	t.Setenv("DEV_HEADER_AUTH", "")

	cfg := ConfigFromEnv()
	if !cfg.RequireAuth {
		t.Fatal("RequireAuth default = false, want true")
	}
	if cfg.DevHeaderAuth {
		t.Fatal("DevHeaderAuth default = true, want false")
	}
}

func TestConfigDevHeaderAuthRequiresExplicitOptIn(t *testing.T) {
	t.Setenv("REQUIRE_AUTH", "false")
	t.Setenv("DEV_HEADER_AUTH", "true")

	cfg := ConfigFromEnv()
	if cfg.RequireAuth {
		t.Fatal("RequireAuth = true, want false when explicitly disabled")
	}
	if !cfg.DevHeaderAuth {
		t.Fatal("DevHeaderAuth = false, want true when explicitly enabled")
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() error = %v, want nil for explicit dev header-auth mode", err)
	}
}

func TestConfigAllowsDeployableUnitServiceAliases(t *testing.T) {
	cases := []struct {
		unit    string
		allows  []string
		denies  string
		needsOS bool
	}{
		{unit: "platform-gateway", allows: []string{"platform-gateway"}, denies: "identity-service"},
		{unit: "iam-unit", allows: []string{"identity-service", "authorization-policy-service"}, denies: "org-project-service"},
		{unit: "tenant-unit", allows: []string{"org-project-service"}, denies: "workload-service"},
		{unit: "collaboration-unit", allows: []string{"audit-compliance-service", "request-notification-service", "media-upload-service"}, denies: "storage-service", needsOS: true},
		{unit: "platform-io-unit", allows: []string{"storage-service", "image-registry-service", "integration-proxy-service"}, denies: "workload-service"},
		{unit: "usage-observability", allows: []string{"usage-observability-service"}, denies: "identity-service"},
		{unit: "compute-api", allows: []string{"workload-service", "ide-service"}, denies: "scheduler-quota-service"},
		{unit: "compute-control-plane", allows: []string{"scheduler-quota-service", "k8s-control-service"}, denies: "workload-service"},
	}
	for _, tc := range cases {
		t.Run(tc.unit, func(t *testing.T) {
			cfg := Config{ServiceName: tc.unit}
			for _, service := range tc.allows {
				if !cfg.AllowsService(service) {
					t.Fatalf("%s should allow %s", tc.unit, service)
				}
			}
			if cfg.AllowsService(tc.denies) {
				t.Fatalf("%s should not allow %s", tc.unit, tc.denies)
			}
			if cfg.RequiresObjectStore() != tc.needsOS {
				t.Fatalf("%s RequiresObjectStore = %v, want %v", tc.unit, cfg.RequiresObjectStore(), tc.needsOS)
			}
		})
	}
}

func TestConfigAllowedOriginsDeniedInProductionWhenUnset(t *testing.T) {
	t.Setenv("PRODUCTION", "true")
	t.Setenv("ALLOWED_ORIGINS", "")

	cfg := ConfigFromEnv()
	if len(cfg.AllowedOrigins) != 0 {
		t.Fatalf("production default allowed origins = %#v, want empty", cfg.AllowedOrigins)
	}
}

func TestConfigEnvironmentProfileDefaultsAndParsing(t *testing.T) {
	cfg := ConfigFromEnv()
	if cfg.EnvironmentProfile != "" {
		t.Fatalf("EnvironmentProfile default = %q, want empty compatibility profile", cfg.EnvironmentProfile)
	}
	if got := cfg.EnvironmentName(); got != runtimeProfileDev {
		t.Fatalf("EnvironmentName default = %q, want dev", got)
	}
	if cfg.Production || cfg.StrictRuntimeChecks() {
		t.Fatalf("default profile production=%v strict=%v, want false/false", cfg.Production, cfg.StrictRuntimeChecks())
	}

	t.Setenv(envAppEnv, runtimeProfileStaging)
	cfg = ConfigFromEnv()
	if cfg.EnvironmentProfile != runtimeProfileStaging || cfg.EnvironmentName() != runtimeProfileStaging {
		t.Fatalf("staging profile = %q env=%q", cfg.EnvironmentProfile, cfg.EnvironmentName())
	}
	if cfg.Production {
		t.Fatal("APP_ENV=staging should not set Production")
	}
	if !cfg.StrictRuntimeChecks() {
		t.Fatal("APP_ENV=staging should use strict startup checks")
	}

	t.Setenv(envAppEnv, runtimeProfileProduction)
	t.Setenv(envProduction, "")
	cfg = ConfigFromEnv()
	if !cfg.Production || cfg.EnvironmentName() != runtimeProfileProduction {
		t.Fatalf("APP_ENV=production production=%v env=%q, want true/production", cfg.Production, cfg.EnvironmentName())
	}
}

func TestConfigEnvironmentProfileRejectsInvalidAndConflictingSettings(t *testing.T) {
	cases := []struct {
		name       string
		appEnv     string
		production string
	}{
		{name: "invalid profile", appEnv: "qa"},
		{name: "production flag conflicts with dev profile", appEnv: runtimeProfileDev, production: "true"},
		{name: "false production flag conflicts with production profile", appEnv: runtimeProfileProduction, production: "false"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(envAppEnv, tc.appEnv)
			if tc.production != "" {
				t.Setenv(envProduction, tc.production)
			}
			cfg := ConfigFromEnv()
			err := cfg.Validate()
			if err == nil || !strings.Contains(err.Error(), envAppEnv) {
				t.Fatalf("Validate() error = %v, want containing %s", err, envAppEnv)
			}
		})
	}
}

func TestConfigAllowedOriginsParsing(t *testing.T) {
	t.Setenv("PRODUCTION", "true")
	t.Setenv("ALLOWED_ORIGINS", " http://a.test, http://b.test ,")

	cfg := ConfigFromEnv()
	if len(cfg.AllowedOrigins) != 2 || !cfg.AllowedOrigins["http://a.test"] || !cfg.AllowedOrigins["http://b.test"] {
		t.Fatalf("parsed allowed origins = %#v, want exact a/b origins", cfg.AllowedOrigins)
	}
}

func TestConfigAllowedOriginsSkipsEmptyTokens(t *testing.T) {
	t.Setenv("PRODUCTION", "true")
	t.Setenv("ALLOWED_ORIGINS", ",,http://a.test,,")

	cfg := ConfigFromEnv()
	if cfg.AllowedOrigins[""] {
		t.Fatalf("allowed origins contains empty key: %#v", cfg.AllowedOrigins)
	}
	if len(cfg.AllowedOrigins) != 1 || !cfg.AllowedOrigins["http://a.test"] {
		t.Fatalf("parsed allowed origins = %#v, want only http://a.test", cfg.AllowedOrigins)
	}
}

func TestConfigTracingAndLoggingEnv(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://collector:4318")
	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", "")
	t.Setenv("SERVICE_VERSION", "1.2.3")
	t.Setenv("LOG_LEVEL", "debug")
	t.Setenv("PRODUCTION", "true")

	cfg := ConfigFromEnv()
	if cfg.OTLPEndpoint != "http://collector:4318" {
		t.Fatalf("OTLPEndpoint = %q, want http://collector:4318", cfg.OTLPEndpoint)
	}
	if !cfg.TracingEnabled() {
		t.Fatal("TracingEnabled() = false, want true when endpoint set")
	}
	if cfg.ServiceVersion != "1.2.3" {
		t.Fatalf("ServiceVersion = %q, want 1.2.3", cfg.ServiceVersion)
	}
	if cfg.LogLevel != "debug" {
		t.Fatalf("LogLevel = %q, want debug", cfg.LogLevel)
	}
	if cfg.EnvironmentName() != "production" {
		t.Fatalf("EnvironmentName() = %q, want production", cfg.EnvironmentName())
	}
}

func TestConfigWorkloadMaintenanceEnv(t *testing.T) {
	t.Setenv("IDE_IDLE_REAPER_TIMEOUT", "45m")
	t.Setenv("AUTOMATED_POD_DELETION_ENABLED", "false")
	t.Setenv("PLAN_WINDOW_POD_DELETION_ENABLED", "false")
	t.Setenv("DEFAULT_QUEUE_NAME", "interactive")

	cfg := ConfigFromEnv()
	if cfg.WorkloadIdleTimeout != 45*time.Minute {
		t.Fatalf("WorkloadIdleTimeout = %v, want 45m", cfg.WorkloadIdleTimeout)
	}
	if cfg.AutomatedPodDeletion {
		t.Fatal("AutomatedPodDeletion = true, want false from env")
	}
	if cfg.PlanWindowPodDeletion {
		t.Fatal("PlanWindowPodDeletion = true, want false from env")
	}
	if cfg.DefaultQueueName != "interactive" {
		t.Fatalf("DefaultQueueName = %q, want interactive", cfg.DefaultQueueName)
	}
}

func TestConfigPlanWindowPodDeletionDefaultsTrue(t *testing.T) {
	if !ConfigFromEnv().PlanWindowPodDeletion {
		t.Fatal("PlanWindowPodDeletion should default to true")
	}
}

func TestConfigVPNUsageDefaultsAndParsing(t *testing.T) {
	cfg := ConfigFromEnv()
	if !cfg.VPNUsageEnabled {
		t.Fatal("VPNUsageEnabled should default to true")
	}
	if cfg.VPNUsageGrace != time.Minute {
		t.Fatalf("VPNUsageGrace default = %v, want 1m", cfg.VPNUsageGrace)
	}

	t.Setenv("VPN_USAGE_ENABLED", "false")
	t.Setenv("VPN_USAGE_GRACE", "90s")
	cfg = ConfigFromEnv()
	if cfg.VPNUsageEnabled {
		t.Fatal("VPNUsageEnabled = true, want false from env")
	}
	if cfg.VPNUsageGrace != 90*time.Second {
		t.Fatalf("VPNUsageGrace = %v, want 90s", cfg.VPNUsageGrace)
	}
}

func TestConfigServiceEnvDefaults(t *testing.T) {
	cfg := ConfigFromEnv()
	if cfg.WebUIDir != defaultWebUIDir {
		t.Fatalf("WebUIDir default = %q, want %q", cfg.WebUIDir, defaultWebUIDir)
	}
	if !strings.Contains(cfg.CLICACertPEM, "NEXUSPAAS-LOCAL-CLI-CA") {
		t.Fatalf("CLICACertPEM default = %q, want local placeholder", cfg.CLICACertPEM)
	}
	if cfg.VPNAPITimeout != 5*time.Second {
		t.Fatalf("VPNAPITimeout default = %v, want 5s", cfg.VPNAPITimeout)
	}
	if cfg.MinIOOperationTimeout != 10*time.Second {
		t.Fatalf("MinIOOperationTimeout default = %v, want 10s", cfg.MinIOOperationTimeout)
	}
	if cfg.PGAdminSSOHTTPTimeout != 10*time.Second {
		t.Fatalf("PGAdminSSOHTTPTimeout default = %v, want 10s", cfg.PGAdminSSOHTTPTimeout)
	}
	assertStringSlice(t, cfg.StorageClassOptions, []string{"standard", "fast"})
	assertStringSlice(t, cfg.GroupStorageClassOptions, nil)
	assertStringSlice(t, cfg.GroupRegistryProfileOptions, nil)

	t.Setenv(envWebUIDir, "/tmp/nexuspaas-web")
	cfg = ConfigFromEnv()
	if cfg.WebUIDir != "/tmp/nexuspaas-web" {
		t.Fatalf("WebUIDir env = %q, want /tmp/nexuspaas-web", cfg.WebUIDir)
	}
}

func TestConfigProductNameDefaultsAndParsing(t *testing.T) {
	cfg := ConfigFromEnv()
	if got := cfg.EffectiveProductName(); got != "NexusPaaS" {
		t.Fatalf("EffectiveProductName default = %q, want NexusPaaS", got)
	}
	if got := (Config{}).EffectiveProductName(); got != "NexusPaaS" {
		t.Fatalf("zero Config EffectiveProductName = %q, want NexusPaaS", got)
	}

	t.Setenv(envProductName, "  CSCC AI Platform  ")
	cfg = ConfigFromEnv()
	if got := cfg.EffectiveProductName(); got != "CSCC AI Platform" {
		t.Fatalf("EffectiveProductName env = %q, want trimmed CSCC AI Platform", got)
	}
}

func TestConfigInputLimitDefaultsAndParsing(t *testing.T) {
	cfg := ConfigFromEnv()
	if cfg.MaxAPIBodyBytes != defaultMaxAPIBodyBytes ||
		cfg.MaxConfigFileBytes != defaultMaxConfigFileBytes ||
		cfg.MaxConfigFileDocuments != defaultMaxConfigFileDocuments {
		t.Fatalf("input limit defaults = api:%d config:%d docs:%d", cfg.MaxAPIBodyBytes, cfg.MaxConfigFileBytes, cfg.MaxConfigFileDocuments)
	}

	t.Setenv(envMaxAPIBodyBytes, "2048")
	t.Setenv(envMaxConfigFileBytes, "1024")
	t.Setenv(envMaxConfigFileDocuments, "7")

	cfg = ConfigFromEnv()
	if cfg.MaxAPIBodyBytes != 2048 || cfg.MaxConfigFileBytes != 1024 || cfg.MaxConfigFileDocuments != 7 {
		t.Fatalf("input limits parsed incorrectly: api:%d config:%d docs:%d", cfg.MaxAPIBodyBytes, cfg.MaxConfigFileBytes, cfg.MaxConfigFileDocuments)
	}
}

func TestConfigInputLimitValidation(t *testing.T) {
	cases := []struct {
		name string
		cfg  Config
		want string
	}{
		{name: "valid defaults", cfg: streamValidationConfig()},
		{name: "negative api body", cfg: streamValidationConfig(func(cfg *Config) { cfg.MaxAPIBodyBytes = -1 }), want: envMaxAPIBodyBytes},
		{name: "negative config bytes", cfg: streamValidationConfig(func(cfg *Config) { cfg.MaxConfigFileBytes = -1 }), want: envMaxConfigFileBytes},
		{name: "negative config docs", cfg: streamValidationConfig(func(cfg *Config) { cfg.MaxConfigFileDocuments = -1 }), want: envMaxConfigFileDocuments},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.cfg.Validate()
			if tc.want == "" {
				if err != nil {
					t.Fatalf("Validate() error = %v, want nil", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Validate() error = %v, want containing %s", err, tc.want)
			}
		})
	}
}

func TestValidateManifestValueAllowsPlainTextContent(t *testing.T) {
	if err := ValidateManifestValue("v2", 16, 1); err != nil {
		t.Fatalf("ValidateManifestValue(plain text) error = %v, want nil", err)
	}

	if err := ValidateManifestValue(strings.Repeat("x", 9), 8, 1); InputLimitStatus(err, 0) != http.StatusRequestEntityTooLarge {
		t.Fatalf("ValidateManifestValue(oversized text) status = %d, want 413: %v", InputLimitStatus(err, 0), err)
	}

	if err := ValidateManifestValue("kind: Pod\n---\nkind: Service", 1024, 1); InputLimitStatus(err, 0) != http.StatusUnprocessableEntity {
		t.Fatalf("ValidateManifestValue(multi-doc manifest) status = %d, want 422: %v", InputLimitStatus(err, 0), err)
	}
}

func TestValidateManifestValueCoversTypedInputs(t *testing.T) {
	if err := ValidateManifestValue(nil, 8, 1); err != nil {
		t.Fatalf("ValidateManifestValue(nil) error = %v, want nil", err)
	}
	if err := ValidateManifestValue([]byte("kind: Pod"), 32, 1); err != nil {
		t.Fatalf("ValidateManifestValue([]byte) error = %v, want nil", err)
	}
	if err := ValidateManifestValue(map[string]any{"kind": "Pod"}, 64, 1); err != nil {
		t.Fatalf("ValidateManifestValue(map) error = %v, want nil", err)
	}
	if err := ValidateManifestValue(make(chan int), 64, 1); err == nil {
		t.Fatal("ValidateManifestValue(channel) error = nil, want marshal error")
	}

	plainErr := errTestSentinel{}
	if InputLimitStatus(plainErr, http.StatusTeapot) != http.StatusTeapot {
		t.Fatalf("InputLimitStatus fallback = %d, want 418", InputLimitStatus(plainErr, http.StatusTeapot))
	}
	if InputLimitMessage(plainErr, "fallback") != "fallback" {
		t.Fatalf("InputLimitMessage fallback = %q, want fallback", InputLimitMessage(plainErr, "fallback"))
	}
}

type errTestSentinel struct{}

func (errTestSentinel) Error() string { return "sentinel" }

func TestConfigServiceEnvParsing(t *testing.T) {
	t.Setenv(envCLICACertPEM, " test-ca ")
	t.Setenv(envVPNAPIURLs, " http://one.test, http://two.test ")
	t.Setenv(envVPNAPIURL, "http://fallback.test")
	t.Setenv(envVPNAPIKey, " vpn-key ")
	t.Setenv(envVPNAPITimeout, "250ms")
	t.Setenv(envMinIOConsoleAccessKey, " minio-access ")
	t.Setenv(envMinIOConsoleSecretKey, " minio-secret ")
	t.Setenv(envMinIOOperationTimeout, "2")
	t.Setenv(envPGAdminDefaultEmail, " admin@test.local ")
	t.Setenv(envPGAdminDefaultPassword, " pgadmin-secret ")
	t.Setenv(envPGAdminSSOHTTPTimeout, "3")
	t.Setenv(envStorageClassOptions, "standard,fast,")
	t.Setenv(envGroupStorageClassOptions, "archive,gold")
	t.Setenv(envGroupRegistryProfileOptions, "default,gpu")

	cfg := ConfigFromEnv()
	if cfg.CLICACertPEM != "test-ca" || cfg.VPNAPIKey != "vpn-key" {
		t.Fatal("service credential fields parsed incorrectly")
	}
	assertStringSlice(t, cfg.VPNAPIURLs, []string{"http://one.test", "http://two.test"})
	if cfg.VPNAPITimeout != 250*time.Millisecond || cfg.MinIOOperationTimeout != 2*time.Second || cfg.PGAdminSSOHTTPTimeout != 3*time.Second {
		t.Fatalf("service timeouts parsed incorrectly: vpn=%v minio=%v pgadmin=%v", cfg.VPNAPITimeout, cfg.MinIOOperationTimeout, cfg.PGAdminSSOHTTPTimeout)
	}
	if cfg.MinIOConsoleAccessKey != "minio-access" || cfg.MinIOConsoleSecretKey != "minio-secret" {
		t.Fatal("minio credentials parsed incorrectly")
	}
	if cfg.PGAdminDefaultEmail != "admin@test.local" || cfg.PGAdminDefaultPassword != "pgadmin-secret" {
		t.Fatal("pgadmin credentials parsed incorrectly")
	}
	assertStringSlice(t, cfg.StorageClassOptions, []string{"standard", "fast"})
	assertStringSlice(t, cfg.GroupStorageClassOptions, []string{"archive", "gold"})
	assertStringSlice(t, cfg.GroupRegistryProfileOptions, []string{"default", "gpu"})
}

func TestConfigStreamEnvDefaultsAndParsing(t *testing.T) {
	cfg := ConfigFromEnv()
	if cfg.StreamTURNCredentialTTL != 8*time.Hour ||
		cfg.StreamMaxBitrateKbps != 12000 ||
		cfg.StreamMaxConcurrentSessions != 64 ||
		cfg.StreamEgressBudgetKbps != 800000 {
		t.Fatalf("stream defaults = ttl:%v bitrate:%d sessions:%d budget:%d", cfg.StreamTURNCredentialTTL, cfg.StreamMaxBitrateKbps, cfg.StreamMaxConcurrentSessions, cfg.StreamEgressBudgetKbps)
	}
	if len(cfg.StreamTURNURIs) != 0 || cfg.StreamTURNSharedSecret != "" {
		t.Fatalf("stream TURN defaults = uris:%#v secret:%q, want empty", cfg.StreamTURNURIs, cfg.StreamTURNSharedSecret)
	}

	t.Setenv(envStreamTURNURIs, " turn:turn.example.com:3478?transport=udp, turns:turn.example.com:5349 ")
	t.Setenv(envStreamTURNSharedSecret, " stream-secret ")
	t.Setenv(envStreamTURNCredentialTTL, "2h")
	t.Setenv(envStreamMaxBitrateKbps, "9000")
	t.Setenv(envStreamMaxConcurrentSessions, "10")
	t.Setenv(envStreamEgressBudgetKbps, "100000")
	t.Setenv(envStreamSidecarImage, " registry.example.com/nexuspaas/selkies-gl-desktop:24.04 ")

	cfg = ConfigFromEnv()
	assertStringSlice(t, cfg.StreamTURNURIs, []string{"turn:turn.example.com:3478?transport=udp", "turns:turn.example.com:5349"})
	if cfg.StreamTURNSharedSecret != "stream-secret" || cfg.StreamTURNCredentialTTL != 2*time.Hour ||
		cfg.StreamMaxBitrateKbps != 9000 || cfg.StreamMaxConcurrentSessions != 10 || cfg.StreamEgressBudgetKbps != 100000 ||
		cfg.StreamSidecarImage != "registry.example.com/nexuspaas/selkies-gl-desktop:24.04" {
		t.Fatalf("stream env parsed incorrectly: %#v", cfg)
	}
}

func TestConfigStreamValidation(t *testing.T) {
	cases := []struct {
		name string
		cfg  Config
		want string
	}{
		{name: "valid defaults", cfg: streamValidationConfig()},
		{name: "negative ttl", cfg: streamValidationConfig(func(cfg *Config) { cfg.StreamTURNCredentialTTL = -time.Second }), want: envStreamTURNCredentialTTL},
		{name: "ttl too long", cfg: streamValidationConfig(func(cfg *Config) { cfg.StreamTURNCredentialTTL = 13 * time.Hour }), want: envStreamTURNCredentialTTL},
		{name: "negative bitrate", cfg: streamValidationConfig(func(cfg *Config) { cfg.StreamMaxBitrateKbps = -1 }), want: envStreamMaxBitrateKbps},
		{name: "negative sessions", cfg: streamValidationConfig(func(cfg *Config) { cfg.StreamMaxConcurrentSessions = -1 }), want: envStreamMaxConcurrentSessions},
		{name: "negative budget", cfg: streamValidationConfig(func(cfg *Config) { cfg.StreamEgressBudgetKbps = -1 }), want: envStreamEgressBudgetKbps},
		{name: "budget exceeded", cfg: streamValidationConfig(func(cfg *Config) {
			cfg.StreamMaxBitrateKbps = 12000
			cfg.StreamMaxConcurrentSessions = 100
			cfg.StreamEgressBudgetKbps = 800000
		}), want: envStreamEgressBudgetKbps},
		{name: "production turn secret required", cfg: streamProductionConfig(""), want: envStreamTURNSharedSecret},
		{name: "production turn secret set", cfg: streamProductionConfig("shared-secret")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.cfg.Validate()
			if tc.want == "" {
				if err != nil {
					t.Fatalf("Validate() error = %v, want nil", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Validate() error = %v, want containing %s", err, tc.want)
			}
		})
	}
}

func streamValidationConfig(opts ...func(*Config)) Config {
	cfg := Config{
		RequireAuth:               true,
		LonghornRWXHealthInterval: time.Minute,
		LonghornRWXRepairCooldown: time.Minute,
		PriorityClassSyncInterval: time.Minute,
	}
	for _, opt := range opts {
		opt(&cfg)
	}
	return cfg
}

func TestConfigServiceEnvFallbackParsing(t *testing.T) {
	t.Setenv(envVPNAPIURL, " http://single.test ")
	t.Setenv(envStorageClassOptions, "standard,fast")
	t.Setenv(envRegistryProfileOptions, "default,gpu")

	cfg := ConfigFromEnv()
	assertStringSlice(t, cfg.VPNAPIURLs, []string{"http://single.test"})
	assertStringSlice(t, cfg.GroupStorageClassOptions, []string{"standard", "fast"})
	assertStringSlice(t, cfg.GroupRegistryProfileOptions, []string{"default", "gpu"})
}

func TestConfigGPUUsageEnv(t *testing.T) {
	t.Setenv(envGPUSnapshotWindowMinutes, "15")
	t.Setenv(envGPUUsageRetentionDays, "45")

	cfg := ConfigFromEnv()
	if cfg.GPUUsageSnapshotWindowMin != 15 {
		t.Fatalf("GPUUsageSnapshotWindowMin = %d, want 15", cfg.GPUUsageSnapshotWindowMin)
	}
	if cfg.GPUUsageRetentionDays != 45 {
		t.Fatalf("GPUUsageRetentionDays = %d, want 45", cfg.GPUUsageRetentionDays)
	}
}

func TestConfigImageCheckEnv(t *testing.T) {
	cfg := ConfigFromEnv()
	if cfg.ImageCheckEnabled {
		t.Fatal("ImageCheckEnabled default = true, want false")
	}

	t.Setenv(envImageCheckEnabled, "true")
	cfg = ConfigFromEnv()
	if !cfg.ImageCheckEnabled {
		t.Fatal("ImageCheckEnabled = false, want true from env")
	}
}

func TestConfigDockerCleanupEnvAndDefaults(t *testing.T) {
	cfg := ConfigFromEnv()
	if cfg.DockerCleanupEnabled {
		t.Fatal("DockerCleanupEnabled default = true, want false")
	}
	if cfg.DockerCleanupNamespace != "default" || cfg.DockerCleanupImage != "docker:24-dind" {
		t.Fatalf("Docker cleanup defaults = namespace:%q image:%q", cfg.DockerCleanupNamespace, cfg.DockerCleanupImage)
	}

	t.Setenv(envDockerCleanupEnabled, "true")
	t.Setenv(envDockerCleanupNamespace, " cleanup-e2e ")
	t.Setenv(envDockerDindImage, " docker:25-dind ")
	cfg = ConfigFromEnv()
	if !cfg.DockerCleanupEnabled {
		t.Fatal("DockerCleanupEnabled = false, want true from env")
	}
	if cfg.DockerCleanupNamespace != "cleanup-e2e" || cfg.DockerCleanupImage != "docker:25-dind" {
		t.Fatalf("Docker cleanup env = namespace:%q image:%q", cfg.DockerCleanupNamespace, cfg.DockerCleanupImage)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() error = %v, want nil", err)
	}
}

func TestConfigDockerCleanupValidation(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(Config) Config
		want   string
	}{
		{
			name: "empty namespace",
			mutate: func(cfg Config) Config {
				cfg.DockerCleanupNamespace = ""
				return cfg
			},
			want: envDockerCleanupNamespace,
		},
		{
			name: "invalid namespace",
			mutate: func(cfg Config) Config {
				cfg.DockerCleanupNamespace = "Bad_Namespace"
				return cfg
			},
			want: envDockerCleanupNamespace,
		},
		{
			name: "empty image",
			mutate: func(cfg Config) Config {
				cfg.DockerCleanupImage = " "
				return cfg
			},
			want: envDockerDindImage,
		},
		{
			name: "disabled ignores unset fields",
			mutate: func(cfg Config) Config {
				cfg.DockerCleanupEnabled = false
				cfg.DockerCleanupNamespace = ""
				cfg.DockerCleanupImage = ""
				return cfg
			},
			want: "",
		},
	}
	base := Config{
		RequireAuth:               true,
		DockerCleanupEnabled:      true,
		DockerCleanupNamespace:    "cleanup-e2e",
		DockerCleanupImage:        "docker:24-dind",
		PriorityClassSyncInterval: time.Minute,
		LonghornRWXHealthInterval: time.Second,
		LonghornRWXRepairCooldown: time.Minute,
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.mutate(base).Validate()
			if tc.want == "" {
				if err != nil {
					t.Fatalf("Validate() error = %v, want nil", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Validate() error = %v, want containing %s", err, tc.want)
			}
		})
	}
}

func TestConfigDockerCleanupRuntimeDefaultsValidate(t *testing.T) {
	cfg := withRuntimeDefaults(Config{RequireAuth: true, DockerCleanupEnabled: true})
	if cfg.DockerCleanupNamespace != "default" || cfg.DockerCleanupImage != "docker:24-dind" {
		t.Fatalf("runtime Docker cleanup defaults = namespace:%q image:%q", cfg.DockerCleanupNamespace, cfg.DockerCleanupImage)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() error = %v, want nil after runtime defaults", err)
	}
}

func TestConfigLonghornRWXEnvParsing(t *testing.T) {
	t.Setenv(envLonghornNamespace, " longhorn-e2e ")
	t.Setenv(envLonghornRWXHealthInterval, "45s")
	t.Setenv(envLonghornRWXAutoRepair, "true")
	t.Setenv(envLonghornRWXRepairCooldown, "12m")
	t.Setenv(envLonghornRWXSnapshotWarn, "7")
	t.Setenv(envLonghornRWXSnapshotBlock, "11")

	cfg := ConfigFromEnv()
	if cfg.LonghornNamespace != "longhorn-e2e" {
		t.Fatalf("LonghornNamespace = %q, want trimmed longhorn-e2e", cfg.LonghornNamespace)
	}
	if cfg.LonghornRWXHealthInterval != 45*time.Second || !cfg.LonghornRWXAutoRepair || cfg.LonghornRWXRepairCooldown != 12*time.Minute {
		t.Fatalf("Longhorn RWX timing/repair config parsed incorrectly: %#v", cfg)
	}
	if cfg.LonghornRWXSnapshotWarn != 7 || cfg.LonghornRWXSnapshotBlock != 11 {
		t.Fatalf("Longhorn snapshot limits = %d/%d, want 7/11", cfg.LonghornRWXSnapshotWarn, cfg.LonghornRWXSnapshotBlock)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() Longhorn config error = %v, want nil", err)
	}
}

func TestConfigLonghornRWXDefaults(t *testing.T) {
	cfg := ConfigFromEnv()
	if cfg.LonghornNamespace != "longhorn-system" ||
		cfg.LonghornRWXHealthInterval != 30*time.Second ||
		cfg.LonghornRWXAutoRepair ||
		cfg.LonghornRWXRepairCooldown != 10*time.Minute ||
		cfg.LonghornRWXSnapshotWarn != 20 ||
		cfg.LonghornRWXSnapshotBlock != 50 {
		t.Fatalf("Longhorn RWX defaults = %#v", cfg)
	}
}

func TestConfigLonghornRWXValidation(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(Config) Config
		want   string
	}{
		{name: "zero interval", mutate: func(cfg Config) Config { cfg.LonghornRWXHealthInterval = 0; return cfg }, want: envLonghornRWXHealthInterval},
		{name: "zero cooldown", mutate: func(cfg Config) Config { cfg.LonghornRWXRepairCooldown = 0; return cfg }, want: envLonghornRWXRepairCooldown},
		{name: "negative interval", mutate: func(cfg Config) Config { cfg.LonghornRWXHealthInterval = -time.Second; return cfg }, want: envLonghornRWXHealthInterval},
		{name: "negative cooldown", mutate: func(cfg Config) Config { cfg.LonghornRWXRepairCooldown = -time.Minute; return cfg }, want: envLonghornRWXRepairCooldown},
		{name: "negative warn", mutate: func(cfg Config) Config { cfg.LonghornRWXSnapshotWarn = -1; return cfg }, want: envLonghornRWXSnapshotWarn},
		{name: "block below warn", mutate: func(cfg Config) Config {
			cfg.LonghornRWXSnapshotWarn = 10
			cfg.LonghornRWXSnapshotBlock = 5
			return cfg
		}, want: envLonghornRWXSnapshotBlock},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := tc.mutate(ConfigFromEnv())
			err := cfg.Validate()
			if tc.want == "" {
				if err != nil {
					t.Fatalf("Validate() error = %v, want nil", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Validate() error = %v, want containing %s", err, tc.want)
			}
		})
	}
}

func TestConfigLonghornRWXRuntimeDefaultsValidate(t *testing.T) {
	cfg := withRuntimeDefaults(Config{RequireAuth: true})
	if cfg.LonghornRWXHealthInterval != 30*time.Second || cfg.LonghornRWXRepairCooldown != 10*time.Minute {
		t.Fatalf("runtime defaults = interval:%v cooldown:%v", cfg.LonghornRWXHealthInterval, cfg.LonghornRWXRepairCooldown)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() error = %v, want nil after runtime defaults", err)
	}
}

func TestConfigPriorityClassSyncEnvAndValidation(t *testing.T) {
	t.Setenv(envPriorityClassSyncInterval, "45s")
	cfg := ConfigFromEnv()
	if cfg.PriorityClassSyncInterval != 45*time.Second {
		t.Fatalf("PriorityClassSyncInterval = %v, want 45s", cfg.PriorityClassSyncInterval)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() error = %v, want nil", err)
	}

	cfg.PriorityClassSyncInterval = 0
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), envPriorityClassSyncInterval) {
		t.Fatalf("Validate() error = %v, want %s", err, envPriorityClassSyncInterval)
	}
}

func TestConfigPriorityClassSyncRuntimeDefault(t *testing.T) {
	cfg := withRuntimeDefaults(Config{RequireAuth: true})
	if cfg.PriorityClassSyncInterval != time.Minute {
		t.Fatalf("PriorityClassSyncInterval = %v, want 1m", cfg.PriorityClassSyncInterval)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() error = %v, want nil after runtime defaults", err)
	}
}

func TestConfigGPUUsageEnvDefaultsAndBounds(t *testing.T) {
	cfg := ConfigFromEnv()
	if cfg.GPUUsageSnapshotWindowMin != 10 {
		t.Fatalf("GPUUsageSnapshotWindowMin default = %d, want 10", cfg.GPUUsageSnapshotWindowMin)
	}
	if cfg.GPUUsageRetentionDays != 30 {
		t.Fatalf("GPUUsageRetentionDays default = %d, want 30", cfg.GPUUsageRetentionDays)
	}

	t.Setenv(envGPUSnapshotWindowMinutes, "0")
	t.Setenv(envGPUUsageRetentionDays, "9999")
	cfg = ConfigFromEnv()
	if cfg.GPUUsageSnapshotWindowMin != 1 {
		t.Fatalf("GPUUsageSnapshotWindowMin lower bound = %d, want 1", cfg.GPUUsageSnapshotWindowMin)
	}
	if cfg.GPUUsageRetentionDays != 3650 {
		t.Fatalf("GPUUsageRetentionDays upper bound = %d, want 3650", cfg.GPUUsageRetentionDays)
	}
}

func TestConfigTracesEndpointTakesPrecedence(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://general:4318")
	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", "http://traces:4318")

	cfg := ConfigFromEnv()
	if cfg.OTLPEndpoint != "http://traces:4318" {
		t.Fatalf("OTLPEndpoint = %q, want traces-specific endpoint to win", cfg.OTLPEndpoint)
	}
}

func TestConfigBackingServiceEnvParsing(t *testing.T) {
	t.Setenv("SERVICE_NAME", "compute-api")
	t.Setenv(envAuthorizationPolicyURL, " "+testPolicyURL+" ")
	t.Setenv(envAuthorizationPolicyAPIKey, " "+testPolicyKey+" ")
	t.Setenv(envDexURL, " http://localhost:5556/dex/ ")
	t.Setenv(envServiceURLs, `{"identity-service":" http://identity-service/ ","":"http://ignored","empty":" "}`)
	t.Setenv(envServiceAPIKey, " service-key ")
	t.Setenv(envServiceIdentityKey, " scoped-key ")
	t.Setenv(envServiceTrustedIdentities, `{"iam-unit":{"key":" iam-key ","audiences":[" compute-api ","compute-api",""]}}`)
	t.Setenv(envDatabaseURL, " "+testDatabaseURL+" ")
	t.Setenv(envRedisURL, " "+testRedisURL+" ")
	t.Setenv(envEventBusURL, " "+testEventBusURL+" ")
	t.Setenv(envEventRelayBatchSize, "37")

	cfg := ConfigFromEnv()
	if cfg.AuthorizationPolicyURL != testPolicyURL {
		t.Fatalf("AuthorizationPolicyURL = %q, want trimmed policy URL", cfg.AuthorizationPolicyURL)
	}
	if cfg.AuthorizationPolicyAPIKey != testPolicyKey {
		t.Fatalf("AuthorizationPolicyAPIKey = %q, want trimmed policy API key", cfg.AuthorizationPolicyAPIKey)
	}
	if cfg.DexURL != "http://localhost:5556/dex" {
		t.Fatalf("DexURL = %q, want trimmed Dex URL", cfg.DexURL)
	}
	if cfg.ServiceURLs["identity-service"] != "http://identity-service" || len(cfg.ServiceURLs) != 1 {
		t.Fatalf("ServiceURLs = %#v, want trimmed identity-service URL only", cfg.ServiceURLs)
	}
	if cfg.ServiceAPIKey != "service-key" {
		t.Fatalf("ServiceAPIKey = %q, want trimmed service API key", cfg.ServiceAPIKey)
	}
	if cfg.ServiceIdentityName != "compute-api" || cfg.ServiceIdentityKey != "scoped-key" {
		t.Fatalf("service identity = %q/%q, want defaulted compute-api/scoped-key", cfg.ServiceIdentityName, cfg.ServiceIdentityKey)
	}
	if got := cfg.ServiceTrustedIdentities["iam-unit"]; got.Key != "iam-key" || len(got.Audiences) != 1 || got.Audiences[0] != "compute-api" {
		t.Fatalf("ServiceTrustedIdentities[iam-unit] = %#v, want trimmed key and deduped audience", got)
	}
	if cfg.DatabaseURL != testDatabaseURL {
		t.Fatalf("DatabaseURL = %q, want trimmed postgres URL", cfg.DatabaseURL)
	}
	if cfg.RedisURL != testRedisURL {
		t.Fatalf("RedisURL = %q, want trimmed redis URL", cfg.RedisURL)
	}
	if cfg.EventBusURL != testEventBusURL {
		t.Fatalf("EventBusURL = %q, want trimmed event bus URL", cfg.EventBusURL)
	}
	if cfg.EventRelayBatchSize != 37 {
		t.Fatalf("EventRelayBatchSize = %d, want 37", cfg.EventRelayBatchSize)
	}
}

func TestConfigLDAPEnvParsing(t *testing.T) {
	t.Setenv(envLDAPEnabled, "true")
	t.Setenv(envLDAPHost, " ldap.local ")
	t.Setenv(envLDAPPort, "1389")
	t.Setenv(envLDAPUseTLS, "true")
	t.Setenv(envLDAPBindDN, " cn=admin,dc=example,dc=org ")
	t.Setenv(envLDAPBindPassword, "secret-password")
	t.Setenv(envLDAPUserSearchBase, " ou=users,dc=example,dc=org ")
	t.Setenv(envLDAPUserFilter, " (mail=%s) ")
	t.Setenv(envLDAPMirrorSyncInterval, "7m")

	cfg := ConfigFromEnv()
	if !cfg.LDAPEnabled || cfg.LDAPHost != "ldap.local" || cfg.LDAPPort != 1389 || !cfg.LDAPUseTLS {
		t.Fatalf("LDAP connection config parsed incorrectly: %#v", cfg)
	}
	if cfg.LDAPBindDN != "cn=admin,dc=example,dc=org" || cfg.LDAPBindPassword != "secret-password" {
		t.Fatalf("LDAP bind config parsed incorrectly: bind_dn=%q password=%q", cfg.LDAPBindDN, cfg.LDAPBindPassword)
	}
	if cfg.LDAPUserSearchBase != "ou=users,dc=example,dc=org" || cfg.LDAPUserFilter != "(mail=%s)" {
		t.Fatalf("LDAP search config parsed incorrectly: base=%q filter=%q", cfg.LDAPUserSearchBase, cfg.LDAPUserFilter)
	}
	if cfg.LDAPMirrorSyncInterval != 7*time.Minute {
		t.Fatalf("LDAPMirrorSyncInterval = %v, want 7m", cfg.LDAPMirrorSyncInterval)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() LDAP config error = %v, want nil", err)
	}
}

func TestConfigLDAPDefaultsDisabled(t *testing.T) {
	cfg := ConfigFromEnv()
	if cfg.LDAPEnabled {
		t.Fatal("LDAPEnabled default = true, want false")
	}
	if cfg.LDAPPort != 389 || cfg.LDAPUseTLS || cfg.LDAPUserFilter != "(uid=%s)" || cfg.LDAPMirrorSyncInterval != 5*time.Minute {
		t.Fatalf("LDAP defaults = port:%d tls:%v filter:%q interval:%v", cfg.LDAPPort, cfg.LDAPUseTLS, cfg.LDAPUserFilter, cfg.LDAPMirrorSyncInterval)
	}
}

func TestConfigLDAPEnabledValidation(t *testing.T) {
	cases := []struct {
		name       string
		mutate     func(Config) Config
		want       string
		notContain string
	}{
		{name: "valid", mutate: func(cfg Config) Config { return cfg }},
		{name: "missing host", mutate: func(cfg Config) Config { cfg.LDAPHost = ""; return cfg }, want: envLDAPHost},
		{name: "invalid port low", mutate: func(cfg Config) Config { cfg.LDAPPort = 0; return cfg }, want: envLDAPPort},
		{name: "invalid port high", mutate: func(cfg Config) Config { cfg.LDAPPort = 70000; return cfg }, want: envLDAPPort},
		{name: "zero placeholder", mutate: func(cfg Config) Config { cfg.LDAPUserFilter = "(uid=alice-secret)"; return cfg }, want: envLDAPUserFilter, notContain: "alice-secret"},
		{name: "multiple placeholders", mutate: func(cfg Config) Config { cfg.LDAPUserFilter = "(&(uid=%s)(mail=%s))"; return cfg }, want: envLDAPUserFilter, notContain: "(&(uid=%s)(mail=%s))"},
		{name: "missing secret", mutate: func(cfg Config) Config { cfg.LDAPBindPassword = ""; return cfg }, want: envLDAPBindPassword, notContain: "bind-secret"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assertLDAPValidation(t, tc.mutate(validLDAPConfig()), tc.want, tc.notContain)
		})
	}
}

func assertLDAPValidation(t *testing.T, cfg Config, want, notContain string) {
	t.Helper()
	err := cfg.Validate()
	if want == "" {
		if err != nil {
			t.Fatalf("Validate() error = %v, want nil", err)
		}
		return
	}
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("Validate() error = %v, want containing %q", err, want)
	}
	if notContain != "" && strings.Contains(err.Error(), notContain) {
		t.Fatalf("Validate() leaked %q in error: %v", notContain, err)
	}
}

func TestConfigTracingDisabledByDefault(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")
	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", "")

	cfg := ConfigFromEnv()
	if cfg.TracingEnabled() {
		t.Fatal("TracingEnabled() = true, want false when no endpoint configured")
	}
	if cfg.ServiceVersion != "0.1.0" {
		t.Fatalf("ServiceVersion default = %q, want 0.1.0", cfg.ServiceVersion)
	}
	if cfg.EnvironmentName() != runtimeProfileDev {
		t.Fatalf("EnvironmentName() = %q, want dev", cfg.EnvironmentName())
	}
}

func TestConfigJWTEnvParsing(t *testing.T) {
	t.Setenv("JWT_JWKS_URL", " https://issuer.test/.well-known/jwks.json ")
	t.Setenv("JWT_ISSUER", " https://issuer.test ")
	t.Setenv("JWT_AUDIENCE", "platform-api,worker")
	t.Setenv("JWT_AUDIENCES", "admin-api")

	cfg := ConfigFromEnv()
	if cfg.JWKSURL != "https://issuer.test/.well-known/jwks.json" {
		t.Fatalf("JWKSURL = %q, want trimmed URL", cfg.JWKSURL)
	}
	if cfg.JWTIssuer != "https://issuer.test" {
		t.Fatalf("JWTIssuer = %q, want trimmed issuer", cfg.JWTIssuer)
	}
	for _, audience := range []string{"platform-api", "worker", "admin-api"} {
		if !cfg.JWTAudiences[audience] {
			t.Fatalf("JWTAudiences missing %q: %#v", audience, cfg.JWTAudiences)
		}
	}
}

func TestConfigAPIKeyPrincipalEnvParsing(t *testing.T) {
	t.Setenv("API_KEY_PRINCIPALS", `{
		"ops-key": {
			"id": "svc:ops",
			"username": "ops-bot",
			"role": "service",
			"scopes": ["catalog:read", "catalog:read", " admin "]
		},
		"admin-key": {
			"user_id": "svc:admin",
			"name": "admin-bot",
			"admin": true
		}
	}`)

	cfg := ConfigFromEnv()
	ops := cfg.APIKeyPrincipals["ops-key"].normalized()
	if ops.ID != "svc:ops" || ops.Username != "ops-bot" || ops.Role != "service" {
		t.Fatalf("ops principal parsed incorrectly: %#v", ops)
	}
	if len(ops.Scopes) != 2 || ops.Scopes[0] != "catalog:read" || ops.Scopes[1] != "admin" {
		t.Fatalf("ops scopes = %#v, want deduped and trimmed", ops.Scopes)
	}
	admin := cfg.APIKeyPrincipals["admin-key"].normalized()
	if admin.ID != "svc:admin" || admin.Username != "admin-bot" || admin.Role != "admin" || !admin.Admin {
		t.Fatalf("admin principal parsed incorrectly: %#v", admin)
	}
}

func TestConfigTrustedProxiesUnset(t *testing.T) {
	t.Setenv("TRUSTED_PROXY_CIDRS", "")

	cfg := ConfigFromEnv()
	if len(cfg.TrustedProxyCIDRs) != 0 {
		t.Fatalf("trusted proxies = %#v, want empty", cfg.TrustedProxyCIDRs)
	}
}

func TestConfigTrustedProxiesParsesCIDRsAndHosts(t *testing.T) {
	t.Setenv("TRUSTED_PROXY_CIDRS", "203.0.113.0/24, 203.0.114.1")

	cfg := ConfigFromEnv()
	if len(cfg.TrustedProxyCIDRs) != 2 {
		t.Fatalf("trusted proxies length = %d, want 2: %#v", len(cfg.TrustedProxyCIDRs), cfg.TrustedProxyCIDRs)
	}
	if !cfg.TrustedProxyCIDRs[0].Contains(parseTestIP(t, "203.0.113.99")) {
		t.Fatalf("first trusted proxy CIDR = %v, want 203.0.113.0/24", cfg.TrustedProxyCIDRs[0])
	}
	if !cfg.TrustedProxyCIDRs[1].Contains(parseTestIP(t, "203.0.114.1")) || cfg.TrustedProxyCIDRs[1].Contains(parseTestIP(t, "203.0.114.2")) {
		t.Fatalf("second trusted proxy CIDR = %v, want host-only 203.0.114.1", cfg.TrustedProxyCIDRs[1])
	}
}

func TestConfigTrustedProxiesSkipsInvalidEntries(t *testing.T) {
	t.Setenv("TRUSTED_PROXY_CIDRS", "bad, 203.0.113.10, also-bad/33")

	cfg := ConfigFromEnv()
	if len(cfg.TrustedProxyCIDRs) != 1 || !cfg.TrustedProxyCIDRs[0].Contains(parseTestIP(t, "203.0.113.10")) {
		t.Fatalf("trusted proxies = %#v, want only 203.0.113.10", cfg.TrustedProxyCIDRs)
	}
}

func TestConfigMalformedEnvFailsClosedInProduction(t *testing.T) {
	cases := []struct {
		name       string
		envName    string
		value      string
		notContain string
	}{
		{name: "production bool", envName: envProduction, value: "maybe"},
		{name: "require auth bool", envName: envRequireAuth, value: "maybe"},
		{name: "dev header auth bool", envName: envDevHeaderAuth, value: "maybe"},
		{name: "service fallback bool", envName: "DISABLE_SERVICE_FALLBACK", value: "maybe"},
		{name: "pod deletion bool", envName: "AUTOMATED_POD_DELETION_ENABLED", value: "maybe"},
		{name: "plan window pod deletion bool", envName: "PLAN_WINDOW_POD_DELETION_ENABLED", value: "maybe"},
		{name: "image check bool", envName: envImageCheckEnabled, value: "maybe"},
		{name: "docker cleanup bool", envName: envDockerCleanupEnabled, value: "maybe"},
		{name: "ldap enabled bool", envName: envLDAPEnabled, value: "maybe"},
		{name: "ldap tls bool", envName: envLDAPUseTLS, value: "maybe"},
		{name: "longhorn auto repair bool", envName: envLonghornRWXAutoRepair, value: "maybe"},
		{name: "shutdown duration", envName: "SHUTDOWN_TIMEOUT", value: "soon"},
		{name: "maintenance duration", envName: "MAINTENANCE_INTERVAL", value: "soon"},
		{name: "ldap mirror duration", envName: envLDAPMirrorSyncInterval, value: "soon"},
		{name: "longhorn health duration", envName: envLonghornRWXHealthInterval, value: "soon"},
		{name: "longhorn repair cooldown duration", envName: envLonghornRWXRepairCooldown, value: "soon"},
		{name: "priority class sync duration", envName: envPriorityClassSyncInterval, value: "soon"},
		{name: "vpn api timeout duration", envName: envVPNAPITimeout, value: "soon"},
		{name: "minio operation timeout duration", envName: envMinIOOperationTimeout, value: "soon"},
		{name: "pgadmin sso timeout duration", envName: envPGAdminSSOHTTPTimeout, value: "soon"},
		{name: "stream turn credential ttl duration", envName: envStreamTURNCredentialTTL, value: "soon"},
		{name: "vpn usage enabled bool", envName: "VPN_USAGE_ENABLED", value: "maybe"},
		{name: "vpn usage grace duration", envName: "VPN_USAGE_GRACE", value: "soon"},
		{name: "adapter timeout duration", envName: "ADAPTER_TIMEOUT", value: "soon"},
		{name: "adapter open duration", envName: "ADAPTER_CIRCUIT_OPEN_INTERVAL", value: "soon"},
		{name: "idle timeout duration", envName: "IDE_IDLE_REAPER_TIMEOUT", value: "soon"},
		{name: "adapter retries int", envName: "ADAPTER_RETRIES", value: "many"},
		{name: "adapter threshold int", envName: "ADAPTER_CIRCUIT_THRESHOLD", value: "many"},
		{name: "audit retention int", envName: "AUDIT_RETENTION_DAYS", value: "many"},
		{name: "gpu snapshot window int", envName: envGPUSnapshotWindowMinutes, value: "many"},
		{name: "gpu usage retention int", envName: envGPUUsageRetentionDays, value: "many"},
		{name: "ldap port int", envName: envLDAPPort, value: "many"},
		{name: "longhorn snapshot warn int", envName: envLonghornRWXSnapshotWarn, value: "many"},
		{name: "longhorn snapshot block int", envName: envLonghornRWXSnapshotBlock, value: "many"},
		{name: "stream max bitrate int", envName: envStreamMaxBitrateKbps, value: "many"},
		{name: "stream max sessions int", envName: envStreamMaxConcurrentSessions, value: "many"},
		{name: "stream egress budget int", envName: envStreamEgressBudgetKbps, value: "many"},
		{name: "service urls json", envName: envServiceURLs, value: "{bad-json"},
		{name: "trusted service identities json", envName: envServiceTrustedIdentities, value: "{bad-json"},
		{name: "adapter config json", envName: envAdapterConfig, value: `{"pgadmin":{"auth":{"token":"secret-token"`, notContain: "secret-token"},
		{name: "api key principals json", envName: envAPIKeyUsers, value: `{"key":{"id":"svc:test","token":"secret-token"`, notContain: "secret-token"},
		{name: "trusted proxy cidrs", envName: envTrustedProxyCIDRs, value: "bad,203.0.113.10"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			setValidProductionEnv(t)
			t.Setenv(tc.envName, tc.value)

			cfg := ConfigFromEnv()
			err := cfg.Validate()
			if err == nil || !strings.Contains(err.Error(), tc.envName) {
				t.Fatalf("Validate() error = %v, want containing %s", err, tc.envName)
			}
			if tc.notContain != "" && strings.Contains(err.Error(), tc.notContain) {
				t.Fatalf("Validate() error leaked raw value %q: %v", tc.notContain, err)
			}
		})
	}
}

func TestConfigMalformedProductionCannotDowngradeValidationMode(t *testing.T) {
	setValidProductionEnv(t)
	t.Setenv(envProduction, "maybe")
	t.Setenv(envDevHeaderAuth, "")

	cfg := ConfigFromEnv()
	if cfg.Production {
		t.Fatal("Production = true, want fallback false for malformed bool")
	}
	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), envProduction) {
		t.Fatalf("Validate() error = %v, want malformed PRODUCTION diagnostic", err)
	}
	if strings.Contains(err.Error(), "DEV_HEADER_AUTH") {
		t.Fatalf("Validate() reported non-production semantic error instead of parse diagnostic: %v", err)
	}
}

func TestConfigMalformedNonProductionRecordsDiagnosticsWithoutFailing(t *testing.T) {
	t.Setenv(envProduction, "false")
	t.Setenv(envAdapterConfig, "{bad-json")
	t.Setenv(envTrustedProxyCIDRs, "bad,203.0.113.10")

	cfg := ConfigFromEnv()
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() error = %v, want nil for non-production compatibility", err)
	}
	if len(cfg.parseDiagnostics) != 2 {
		t.Fatalf("parse diagnostics = %#v, want adapter config and trusted proxy diagnostics", cfg.parseDiagnostics)
	}
	if len(cfg.TrustedProxyCIDRs) != 1 || !cfg.TrustedProxyCIDRs[0].Contains(parseTestIP(t, "203.0.113.10")) {
		t.Fatalf("trusted proxies = %#v, want valid token retained", cfg.TrustedProxyCIDRs)
	}
}

func TestConfigMalformedStagingFailsClosed(t *testing.T) {
	t.Setenv(envAppEnv, runtimeProfileStaging)
	t.Setenv(envAdapterConfig, "{bad-json")

	cfg := ConfigFromEnv()
	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), envAdapterConfig) {
		t.Fatalf("Validate() error = %v, want containing %s", err, envAdapterConfig)
	}
}

func TestConfigValidateProductionGuards(t *testing.T) {
	cases := []struct {
		name    string
		cfg     Config
		wantErr string
	}{
		{name: "non production auth-on allows missing auth backend", cfg: Config{RequireAuth: true}},
		{name: "non production header mode requires explicit opt-in", cfg: Config{}, wantErr: "DEV_HEADER_AUTH"},
		{name: "non production accepts explicit header mode", cfg: Config{DevHeaderAuth: true}},
		{name: "production requires auth", cfg: Config{Production: true}, wantErr: "REQUIRE_AUTH"},
		{name: "production rejects dev header mode", cfg: Config{Production: true, RequireAuth: true, DevHeaderAuth: true}, wantErr: "DEV_HEADER_AUTH"},
		{name: "production requires auth backend", cfg: Config{Production: true, RequireAuth: true}, wantErr: "API_KEYS or JWT_JWKS_URL"},
		{name: "production ignores disabled api key", cfg: Config{Production: true, RequireAuth: true, APIKeys: map[string]bool{"disabled": false}}, wantErr: "API_KEYS or JWT_JWKS_URL"},
		{name: "production api key requires principal", cfg: Config{Production: true, RequireAuth: true, APIKeys: map[string]bool{"key": true}}, wantErr: "API_KEY_PRINCIPALS"},
		{name: "isolated production requires authorization policy url", cfg: isolatedProductionConfigWithout(envAuthorizationPolicyURL), wantErr: envAuthorizationPolicyURL},
		{name: "isolated production policy url requires api key", cfg: isolatedProductionConfigWithout(envAuthorizationPolicyAPIKey), wantErr: envAuthorizationPolicyAPIKey},
		{name: "production service urls reject legacy-only service key", cfg: productionConfigWithServiceURLs(false), wantErr: envServiceIdentityName},
		{name: "production accepts service urls with scoped service identity", cfg: productionConfigWithServiceURLs(true)},
		{name: "production requires database url", cfg: productionConfigWithout(envDatabaseURL), wantErr: envDatabaseURL},
		{name: "production requires redis url", cfg: productionConfigWithout(envRedisURL), wantErr: envRedisURL},
		{name: "production requires event bus url", cfg: productionConfigWithout(envEventBusURL), wantErr: envEventBusURL},
		{name: "production requires object store url", cfg: productionConfigWithout(envObjectStoreURL), wantErr: envObjectStoreURL},
		{name: "production rejects non-redis redis url", cfg: productionConfigWithURL(envRedisURL, "nats://redis:4222"), wantErr: envRedisURL},
		{name: "production rejects non-redis event bus url", cfg: productionConfigWithURL(envEventBusURL, "nats://events:4222"), wantErr: envEventBusURL},
		{name: "event relay batch size must be positive", cfg: Config{RequireAuth: true, EventRelayBatchSize: -1}, wantErr: envEventRelayBatchSize},
		{name: "production accepts api key principal", cfg: validProductionConfig()},
		{name: "production profile enables production guards", cfg: Config{EnvironmentProfile: runtimeProfileProduction, RequireAuth: true}, wantErr: "API_KEYS or JWT_JWKS_URL"},
		{name: "production flag conflicts with staging profile", cfg: Config{EnvironmentProfile: runtimeProfileStaging, Production: true, RequireAuth: true}, wantErr: envAppEnv},
		{name: "production rejects insecure jwks", cfg: Config{Production: true, RequireAuth: true, JWKSURL: "http://issuer.test/jwks", JWTIssuer: "https://issuer.test", JWTAudiences: map[string]bool{"api": true}}, wantErr: "https"},
		{name: "production jwks requires issuer", cfg: Config{Production: true, RequireAuth: true, JWKSURL: "https://issuer.test/.well-known/jwks.json", JWTAudiences: map[string]bool{"api": true}}, wantErr: "JWT_ISSUER"},
		{name: "production jwks requires audience", cfg: Config{Production: true, RequireAuth: true, JWKSURL: "https://issuer.test/.well-known/jwks.json", JWTIssuer: "https://issuer.test"}, wantErr: "JWT_AUDIENCE"},
		{name: "production accepts jwks", cfg: withProductionBacking(Config{Production: true, RequireAuth: true, JWKSURL: "https://issuer.test/.well-known/jwks.json", JWTIssuer: "https://issuer.test", JWTAudiences: map[string]bool{"api": true}})},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := withRuntimeDefaults(tc.cfg).Validate()
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("Validate() error = %v, want nil", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("Validate() error = %v, want containing %q", err, tc.wantErr)
			}
		})
	}
}

func setValidProductionEnv(t *testing.T) {
	t.Helper()
	values := map[string]string{
		envProduction:                    "true",
		envRequireAuth:                   "true",
		envDevHeaderAuth:                 "false",
		"DISABLE_SERVICE_FALLBACK":       "false",
		"AUTOMATED_POD_DELETION_ENABLED": "true",
		"SHUTDOWN_TIMEOUT":               "10s",
		"MAINTENANCE_INTERVAL":           "15m",
		"ADAPTER_TIMEOUT":                "2s",
		"ADAPTER_RETRIES":                "3",
		"ADAPTER_CIRCUIT_THRESHOLD":      "3",
		"ADAPTER_CIRCUIT_OPEN_INTERVAL":  "30s",
		"AUDIT_RETENTION_DAYS":           "30",
		"IDE_IDLE_REAPER_TIMEOUT":        "2h",
		"API_KEYS":                       testAPIKey,
		envAPIKeyUsers:                   `{"key":{"id":"svc:test"}}`,
		envJWTJWKSURL:                    "",
		envJWTIssuer:                     "",
		envJWTAudience:                   "",
		envJWTAudiences:                  "",
		envAuthorizationPolicyURL:        "",
		envAuthorizationPolicyAPIKey:     "",
		envServiceURLs:                   "",
		envServiceAPIKey:                 "",
		envServiceIdentityName:           "",
		envServiceIdentityKey:            "",
		envServiceTrustedIdentities:      "",
		envDexURL:                        "",
		envDatabaseURL:                   testDatabaseURL,
		envRedisURL:                      testRedisURL,
		envEventBusURL:                   testEventBusURL,
		envObjectStoreURL:                testObjectURL,
		envObjectStoreAccessKey:          "access",
		envObjectStoreSecretKey:          "secret",
		envObjectStoreBucket:             "media",
		envTrustedProxyCIDRs:             "",
		envAdapterConfig:                 "",
		envImageCheckEnabled:             "false",
		envDockerCleanupEnabled:          "false",
		envDockerCleanupNamespace:        "default",
		envDockerDindImage:               "docker:24-dind",
		envPriorityClassSyncInterval:     "1m",
	}
	for key, value := range values {
		t.Setenv(key, value)
	}
}

func validProductionConfig() Config {
	return withRuntimeDefaults(withProductionBacking(Config{
		Production:       true,
		RequireAuth:      true,
		APIKeys:          map[string]bool{testAPIKey: true},
		APIKeyPrincipals: map[string]APIKeyPrincipal{testAPIKey: {ID: "svc:test"}},
	}))
}

func validLDAPConfig() Config {
	return withRuntimeDefaults(Config{
		RequireAuth:            true,
		LDAPEnabled:            true,
		LDAPHost:               "ldap.local",
		LDAPPort:               1389,
		LDAPUseTLS:             false,
		LDAPBindDN:             "cn=admin,dc=example,dc=org",
		LDAPBindPassword:       "bind-secret",
		LDAPUserSearchBase:     "ou=users,dc=example,dc=org",
		LDAPUserFilter:         "(uid=%s)",
		LDAPMirrorSyncInterval: 5 * time.Minute,
	})
}

func productionConfigWithout(envName string) Config {
	cfg := validProductionConfig()
	switch envName {
	case envDatabaseURL:
		cfg.DatabaseURL = ""
	case envRedisURL:
		cfg.RedisURL = ""
	case envEventBusURL:
		cfg.EventBusURL = ""
	case envObjectStoreURL:
		cfg.ObjectStoreURL = ""
	}
	return cfg
}

func productionConfigWithURL(envName, value string) Config {
	cfg := validProductionConfig()
	switch envName {
	case envRedisURL:
		cfg.RedisURL = value
	case envEventBusURL:
		cfg.EventBusURL = value
	}
	return cfg
}

func productionConfigWithServiceURLs(includeKey bool) Config {
	cfg := validProductionConfig()
	cfg.ServiceURLs = map[string]string{"identity-service": "http://identity-service"}
	if includeKey {
		cfg.ServiceIdentityName = "compute-api"
		cfg.ServiceIdentityKey = "scoped-key"
		cfg.ServiceTrustedIdentities = map[string]ServiceTrustedIdentity{
			"iam-unit": {Key: "iam-key", Audiences: []string{"compute-api"}},
		}
	} else {
		cfg.ServiceAPIKey = "legacy-key"
	}
	return cfg
}

func streamProductionConfig(secret string) Config {
	cfg := validProductionConfig()
	cfg.StreamTURNURIs = []string{"turn:turn.example.com:3478?transport=udp"}
	cfg.StreamTURNSharedSecret = secret
	return cfg
}

func isolatedProductionConfigWithout(envName string) Config {
	cfg := validProductionConfig()
	cfg.ServiceName = "identity-service"
	cfg.AuthorizationPolicyURL = testPolicyURL
	cfg.AuthorizationPolicyAPIKey = testPolicyKey
	switch envName {
	case envAuthorizationPolicyURL:
		cfg.AuthorizationPolicyURL = ""
	case envAuthorizationPolicyAPIKey:
		cfg.AuthorizationPolicyAPIKey = ""
	}
	return cfg
}

func withProductionBacking(cfg Config) Config {
	cfg.DatabaseURL = testDatabaseURL
	cfg.RedisURL = testRedisURL
	cfg.EventBusURL = testEventBusURL
	cfg.ObjectStoreURL = testObjectURL
	cfg.ObjectStoreAccessKey = "access"
	cfg.ObjectStoreSecretKey = "secret"
	return cfg
}

func parseTestIP(t *testing.T, value string) net.IP {
	t.Helper()
	ip := net.ParseIP(value)
	if ip == nil {
		t.Fatalf("invalid test IP %q", value)
	}
	return ip
}

func assertStringSlice(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("slice = %#v, want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("slice = %#v, want %#v", got, want)
		}
	}
}
