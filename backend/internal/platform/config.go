package platform

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	ServiceName                 string
	ProductName                 string
	EnvironmentProfile          string
	HTTPAddr                    string
	RequireAuth                 bool
	DevHeaderAuth               bool
	DevAuthSigningKey           string
	APIKeys                     map[string]bool
	APIKeyPrincipals            map[string]APIKeyPrincipal
	AllowedOrigins              map[string]bool
	TrustedProxyCIDRs           []*net.IPNet
	WebUIDir                    string
	JWKSURL                     string
	JWTIssuer                   string
	JWTAudiences                map[string]bool
	AuthorizationPolicyURL      string
	AuthorizationPolicyAPIKey   string
	ServiceURLs                 map[string]string
	ServiceAPIKey               string
	ServiceIdentityName         string
	ServiceIdentityKey          string
	ServiceTrustedIdentities    map[string]ServiceTrustedIdentity
	ServiceFallbackDisabled     bool
	DexURL                      string
	LDAPEnabled                 bool
	LDAPHost                    string
	LDAPPort                    int
	LDAPUseTLS                  bool
	LDAPBindDN                  string
	LDAPBindPassword            string
	LDAPUserSearchBase          string
	LDAPUserFilter              string
	LDAPMirrorSyncInterval      time.Duration
	DatabaseURL                 string
	RedisURL                    string
	EventBusURL                 string
	EventRelayBatchSize         int
	ObjectStoreURL              string
	ObjectStoreAccessKey        string
	ObjectStoreSecretKey        string
	ObjectStoreBucket           string
	OTLPEndpoint                string
	ServiceVersion              string
	LogLevel                    string
	Production                  bool
	ShutdownTimeout             time.Duration
	MaintenanceInterval         time.Duration
	AdapterTimeout              time.Duration
	AdapterRetries              int
	AdapterThreshold            int
	AdapterOpenInterval         time.Duration
	AuditRetentionDays          int
	MaxAPIBodyBytes             int
	MaxConfigFileBytes          int
	MaxConfigFileDocuments      int
	K8sNamespacePrefix          string
	ImageCheckEnabled           bool
	DockerCleanupEnabled        bool
	DockerCleanupNamespace      string
	DockerCleanupImage          string
	FastTransferMoverImage      string
	WorkloadIdleTimeout         time.Duration
	AutomatedPodDeletion        bool
	PlanWindowPodDeletion       bool
	DefaultQueueName            string
	GPUUsageSnapshotWindowMin   int
	GPUUsageRetentionDays       int
	LonghornNamespace           string
	LonghornRWXHealthInterval   time.Duration
	LonghornRWXAutoRepair       bool
	LonghornRWXRepairCooldown   time.Duration
	LonghornRWXSnapshotWarn     int
	LonghornRWXSnapshotBlock    int
	PriorityClassSyncInterval   time.Duration
	VPNUsageEnabled             bool
	VPNUsageGrace               time.Duration
	CLICACertPEM                string
	VPNAPIURLs                  []string
	VPNAPIKey                   string
	VPNAPITimeout               time.Duration
	MinIOConsoleAccessKey       string
	MinIOConsoleSecretKey       string
	MinIOOperationTimeout       time.Duration
	PGAdminDefaultEmail         string
	PGAdminDefaultPassword      string
	PGAdminSSOHTTPTimeout       time.Duration
	StreamTURNURIs              []string
	StreamTURNSharedSecret      string
	StreamTURNCredentialTTL     time.Duration
	StreamMaxBitrateKbps        int
	StreamMaxConcurrentSessions int
	StreamEgressBudgetKbps      int
	StreamSidecarImage          string
	StorageClassOptions         []string
	GroupStorageClassOptions    []string
	GroupRegistryProfileOptions []string
	ExternalURLs                map[string]string
	AdapterConfigs              map[string]AdapterConfig
	parseDiagnostics            []configParseDiagnostic
}

// AdapterConfig is the per-adapter upstream routing configuration (finding 8):
// an optional path rewrite plus an injected upstream auth credential.
type AdapterConfig struct {
	StripPrefix string            `json:"strip_prefix"`
	AddPrefix   string            `json:"add_prefix"`
	Auth        AdapterAuthConfig `json:"auth"`
}

type ServiceTrustedIdentity struct {
	Key       string   `json:"key"`
	Audiences []string `json:"audiences"`
}

// AdapterAuthConfig describes the credential to inject toward an upstream. Type is
// one of "bearer", "basic", or "header".
type AdapterAuthConfig struct {
	Type     string `json:"type"`
	Token    string `json:"token"`
	Username string `json:"username"`
	Password string `json:"password"`
	Header   string `json:"header"`
	Value    string `json:"value"`
}

const (
	configPositiveValidationSuffix = " must be positive"
	mediaUploadServiceName         = "media-upload-service"
	runtimeProfileLocal            = "local"
	runtimeProfileTest             = "test"
	runtimeProfileDev              = "dev"
	runtimeProfileStaging          = "staging"
	runtimeProfileProduction       = "production"

	envJWTJWKSURL                  = "JWT_JWKS_URL"
	envProductName                 = "PRODUCT_NAME"
	envAppEnv                      = "APP_ENV"
	envJWTIssuer                   = "JWT_ISSUER"
	envJWTAudience                 = "JWT_AUDIENCE"
	envJWTAudiences                = "JWT_AUDIENCES"
	envAPIKeyUsers                 = "API_KEY_PRINCIPALS"
	envDevHeaderAuth               = "DEV_HEADER_AUTH"
	envDevAuthSigningKey           = "DEV_AUTH_SIGNING_KEY"
	envAuthorizationPolicyURL      = "AUTHORIZATION_POLICY_URL"
	envAuthorizationPolicyAPIKey   = "AUTHORIZATION_POLICY_API_KEY"
	envDatabaseURL                 = "DATABASE_URL"
	envRedisURL                    = "REDIS_URL"
	envEventBusURL                 = "EVENT_BUS_URL"
	envEventRelayBatchSize         = "EVENT_RELAY_BATCH_SIZE"
	envObjectStoreURL              = "OBJECT_STORE_URL"
	envObjectStoreAccessKey        = "OBJECT_STORE_ACCESS_KEY"
	envObjectStoreSecretKey        = "OBJECT_STORE_SECRET_KEY"
	envObjectStoreBucket           = "OBJECT_STORE_BUCKET"
	envServiceURLs                 = "SERVICE_URLS"
	envServiceAPIKey               = "SERVICE_API_KEY"
	envServiceIdentityName         = "SERVICE_IDENTITY_NAME"
	envServiceIdentityKey          = "SERVICE_IDENTITY_KEY"
	envServiceTrustedIdentities    = "SERVICE_TRUSTED_IDENTITIES"
	envDexURL                      = "DEX_URL"
	envLDAPEnabled                 = "LDAP_ENABLED"
	envLDAPHost                    = "LDAP_HOST"
	envLDAPPort                    = "LDAP_PORT"
	envLDAPUseTLS                  = "LDAP_USE_TLS"
	envLDAPBindDN                  = "LDAP_BIND_DN"
	envLDAPBindPassword            = "LDAP_BIND_PASSWORD"
	envLDAPUserSearchBase          = "LDAP_USER_SEARCH_BASE"
	envLDAPUserFilter              = "LDAP_USER_FILTER"
	envLDAPMirrorSyncInterval      = "LDAP_MIRROR_SYNC_INTERVAL"
	envProduction                  = "PRODUCTION"
	envRequireAuth                 = "REQUIRE_AUTH"
	envTrustedProxyCIDRs           = "TRUSTED_PROXY_CIDRS"
	envWebUIDir                    = "WEB_UI_DIR"
	envMaxAPIBodyBytes             = "MAX_API_BODY_BYTES"
	envMaxConfigFileBytes          = "MAX_CONFIGFILE_BYTES"
	envMaxConfigFileDocuments      = "MAX_CONFIGFILE_DOCUMENTS"
	envAdapterConfig               = "ADAPTER_CONFIG"
	envImageCheckEnabled           = "K8S_IMAGE_CHECK_ENABLED"
	envDockerCleanupEnabled        = "DOCKER_CLEANUP_ENABLED"
	envDockerCleanupNamespace      = "DOCKER_CLEANUP_NAMESPACE"
	envFastTransferMoverImage      = "FAST_TRANSFER_MOVER_IMAGE"
	envDockerDindImage             = "IMAGE_DOCKER_DIND"
	envGPUUsageRetentionDays       = "GPU_USAGE_RETENTION_DAYS"
	envGPUSnapshotWindowMinutes    = "GPU_SNAPSHOT_WINDOW_MINUTES"
	envLonghornNamespace           = "LONGHORN_NAMESPACE"
	envLonghornRWXHealthInterval   = "LONGHORN_RWX_HEALTH_INTERVAL"
	envLonghornRWXAutoRepair       = "LONGHORN_RWX_AUTO_REPAIR_ENABLED"
	envLonghornRWXRepairCooldown   = "LONGHORN_RWX_REPAIR_COOLDOWN"
	envLonghornRWXSnapshotWarn     = "LONGHORN_RWX_SNAPSHOT_WARN_LIMIT"
	envLonghornRWXSnapshotBlock    = "LONGHORN_RWX_SNAPSHOT_BLOCK_LIMIT"
	envPriorityClassSyncInterval   = "PRIORITY_CLASS_SYNC_INTERVAL"
	envCLICACertPEM                = "CLI_CA_CERT_PEM"
	envVPNAPIKey                   = "VPN_API_KEY"
	envVPNAPIURLs                  = "VPN_API_URLS"
	envVPNAPIURL                   = "VPN_API_URL"
	envVPNAPITimeout               = "VPN_API_TIMEOUT_SEC"
	envMinIOConsoleAccessKey       = "MINIO_ACCESS_KEY"
	envMinIOConsoleSecretKey       = "MINIO_SECRET_KEY"
	envMinIOOperationTimeout       = "MINIO_OPERATION_TIMEOUT_SEC"
	envPGAdminDefaultEmail         = "PGADMIN_DEFAULT_EMAIL"
	envPGAdminDefaultPassword      = "PGADMIN_DEFAULT_PASSWORD"
	envPGAdminSSOHTTPTimeout       = "PGADMIN_SSO_HTTP_TIMEOUT_SEC"
	envStreamTURNURIs              = "STREAM_TURN_URIS"
	envStreamTURNSharedSecret      = "STREAM_TURN_SHARED_SECRET"
	envStreamTURNCredentialTTL     = "STREAM_TURN_CREDENTIAL_TTL"
	envStreamMaxBitrateKbps        = "STREAM_MAX_BITRATE_KBPS"
	envStreamMaxConcurrentSessions = "STREAM_MAX_CONCURRENT_SESSIONS"
	envStreamEgressBudgetKbps      = "STREAM_EGRESS_BUDGET_KBPS"
	envStreamSidecarImage          = "STREAM_SIDECAR_IMAGE"
	envStorageClassOptions         = "STORAGE_CLASS_OPTIONS"
	envGroupStorageClassOptions    = "GROUP_STORAGE_CLASS_OPTIONS"
	envGroupRegistryProfileOptions = "GROUP_REGISTRY_PROFILE_OPTIONS"
	envRegistryProfileOptions      = "REGISTRY_PROFILE_OPTIONS"
	configDiagnosticJSONObject     = "JSON object"
	defaultEventRelayBatchSize     = 100
)

var deployableUnitServices = map[string][]string{
	"platform-gateway":      {"platform-gateway"},
	"iam-unit":              {"identity-service", "authorization-policy-service"},
	"tenant-unit":           {"org-project-service"},
	"collaboration-unit":    {"audit-compliance-service", "request-notification-service", mediaUploadServiceName},
	"platform-io-unit":      {"storage-service", "image-registry-service", "integration-proxy-service"},
	"usage-observability":   {"usage-observability-service"},
	"compute-api":           {"workload-service", "ide-service"},
	"compute-control-plane": {"scheduler-quota-service", "k8s-control-service"},
}

type configParseDiagnostic struct {
	envName string
	kind    string
}

func (d configParseDiagnostic) String() string {
	return fmt.Sprintf("%s has invalid %s", d.envName, d.kind)
}

type configEnvParser struct {
	diagnostics []configParseDiagnostic
}

const (
	defaultProductName  = "NexusPaaS"
	defaultCLICACertPEM = "-----BEGIN CERTIFICATE-----\nNEXUSPAAS-LOCAL-CLI-CA\n-----END CERTIFICATE-----\n"
	defaultWebUIDir     = "/app/web"
)

func (p *configEnvParser) addDiagnostic(envName, kind string) {
	p.diagnostics = append(p.diagnostics, configParseDiagnostic{envName: envName, kind: kind})
}

func ConfigFromEnv() Config {
	parser := &configEnvParser{}
	productionRaw, productionSet := os.LookupEnv(envProduction)
	production := parser.envBool(envProduction, false)
	serviceName := env("SERVICE_NAME", "all")
	environmentProfile := normalizeEnvironmentProfile(os.Getenv(envAppEnv))
	if profileConflictsWithProductionFlag(environmentProfile, production, productionSet, productionRaw) {
		parser.addDiagnostic(envAppEnv, "profile compatible with PRODUCTION")
	}
	effectiveProduction := runtimeProfileIsProduction(environmentProfile, production)
	allowedOrigins := parseSet(os.Getenv("ALLOWED_ORIGINS"))
	if len(allowedOrigins) == 0 && !effectiveProduction {
		allowedOrigins = defaultDevAllowedOrigins()
	}
	trustedProxyCIDRs, trustedProxyErr := parseTrustedProxyCIDRsWithDiagnostics(os.Getenv(envTrustedProxyCIDRs))
	if trustedProxyErr != nil {
		parser.addDiagnostic(envTrustedProxyCIDRs, "CIDR/IP list")
	}
	cfg := Config{
		ServiceName:                 serviceName,
		ProductName:                 env(envProductName, defaultProductName),
		EnvironmentProfile:          environmentProfile,
		HTTPAddr:                    env("HTTP_ADDR", ":8080"),
		RequireAuth:                 parser.envBool(envRequireAuth, true),
		DevHeaderAuth:               parser.envBool(envDevHeaderAuth, false),
		DevAuthSigningKey:           strings.TrimSpace(os.Getenv(envDevAuthSigningKey)),
		APIKeys:                     parseSet(os.Getenv("API_KEYS")),
		APIKeyPrincipals:            parser.parseAPIKeyPrincipals(os.Getenv(envAPIKeyUsers)),
		AllowedOrigins:              allowedOrigins,
		TrustedProxyCIDRs:           trustedProxyCIDRs,
		WebUIDir:                    strings.TrimSpace(env(envWebUIDir, defaultWebUIDir)),
		JWKSURL:                     strings.TrimSpace(os.Getenv(envJWTJWKSURL)),
		JWTIssuer:                   strings.TrimSpace(os.Getenv(envJWTIssuer)),
		JWTAudiences:                parseJWTAudiences(),
		AuthorizationPolicyURL:      strings.TrimSpace(os.Getenv(envAuthorizationPolicyURL)),
		AuthorizationPolicyAPIKey:   strings.TrimSpace(os.Getenv(envAuthorizationPolicyAPIKey)),
		ServiceURLs:                 parser.parseServiceURLs(os.Getenv(envServiceURLs)),
		ServiceAPIKey:               strings.TrimSpace(os.Getenv(envServiceAPIKey)),
		ServiceIdentityName:         defaultServiceIdentityName(serviceName, os.Getenv(envServiceIdentityName)),
		ServiceIdentityKey:          strings.TrimSpace(os.Getenv(envServiceIdentityKey)),
		ServiceTrustedIdentities:    parser.parseServiceTrustedIdentities(os.Getenv(envServiceTrustedIdentities)),
		ServiceFallbackDisabled:     parser.envBool("DISABLE_SERVICE_FALLBACK", false),
		DexURL:                      strings.TrimRight(strings.TrimSpace(os.Getenv(envDexURL)), "/"),
		LDAPEnabled:                 parser.envBool(envLDAPEnabled, false),
		LDAPHost:                    strings.TrimSpace(os.Getenv(envLDAPHost)),
		LDAPPort:                    parser.envInt(envLDAPPort, 389),
		LDAPUseTLS:                  parser.envBool(envLDAPUseTLS, false),
		LDAPBindDN:                  strings.TrimSpace(os.Getenv(envLDAPBindDN)),
		LDAPBindPassword:            os.Getenv(envLDAPBindPassword),
		LDAPUserSearchBase:          strings.TrimSpace(os.Getenv(envLDAPUserSearchBase)),
		LDAPUserFilter:              strings.TrimSpace(env(envLDAPUserFilter, "(uid=%s)")),
		LDAPMirrorSyncInterval:      parser.envDuration(envLDAPMirrorSyncInterval, 5*time.Minute),
		DatabaseURL:                 strings.TrimSpace(os.Getenv(envDatabaseURL)),
		RedisURL:                    strings.TrimSpace(os.Getenv(envRedisURL)),
		EventBusURL:                 strings.TrimSpace(os.Getenv(envEventBusURL)),
		EventRelayBatchSize:         parser.envInt(envEventRelayBatchSize, defaultEventRelayBatchSize),
		ObjectStoreURL:              strings.TrimRight(strings.TrimSpace(os.Getenv(envObjectStoreURL)), "/"),
		ObjectStoreAccessKey:        strings.TrimSpace(os.Getenv(envObjectStoreAccessKey)),
		ObjectStoreSecretKey:        os.Getenv(envObjectStoreSecretKey),
		ObjectStoreBucket:           env(envObjectStoreBucket, "media"),
		OTLPEndpoint:                firstNonEmpty(os.Getenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT"), os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")),
		ServiceVersion:              env("SERVICE_VERSION", "0.1.0"),
		LogLevel:                    os.Getenv("LOG_LEVEL"),
		Production:                  effectiveProduction,
		ShutdownTimeout:             parser.envDuration("SHUTDOWN_TIMEOUT", 10*time.Second),
		MaintenanceInterval:         parser.envDuration("MAINTENANCE_INTERVAL", 15*time.Minute),
		AdapterTimeout:              parser.envDuration("ADAPTER_TIMEOUT", 2*time.Second),
		AdapterRetries:              parser.envInt("ADAPTER_RETRIES", 3),
		AdapterThreshold:            parser.envInt("ADAPTER_CIRCUIT_THRESHOLD", 3),
		AdapterOpenInterval:         parser.envDuration("ADAPTER_CIRCUIT_OPEN_INTERVAL", 30*time.Second),
		AuditRetentionDays:          parser.envInt("AUDIT_RETENTION_DAYS", 30),
		MaxAPIBodyBytes:             parser.envInt(envMaxAPIBodyBytes, defaultMaxAPIBodyBytes),
		MaxConfigFileBytes:          parser.envInt(envMaxConfigFileBytes, defaultMaxConfigFileBytes),
		MaxConfigFileDocuments:      parser.envInt(envMaxConfigFileDocuments, defaultMaxConfigFileDocuments),
		K8sNamespacePrefix:          env("K8S_PROJECT_NAMESPACE_PREFIX", "proj"),
		ImageCheckEnabled:           parser.envBool(envImageCheckEnabled, false),
		DockerCleanupEnabled:        parser.envBool(envDockerCleanupEnabled, false),
		DockerCleanupNamespace:      strings.TrimSpace(env(envDockerCleanupNamespace, "default")),
		DockerCleanupImage:          strings.TrimSpace(env(envDockerDindImage, "docker:24-dind")),
		FastTransferMoverImage:      strings.TrimSpace(env(envFastTransferMoverImage, "instrumentisto/rsync-ssh:alpine")),
		WorkloadIdleTimeout:         parser.envDuration("IDE_IDLE_REAPER_TIMEOUT", 2*time.Hour),
		AutomatedPodDeletion:        parser.envBool("AUTOMATED_POD_DELETION_ENABLED", true),
		PlanWindowPodDeletion:       parser.envBool("PLAN_WINDOW_POD_DELETION_ENABLED", true),
		DefaultQueueName:            env("DEFAULT_QUEUE_NAME", "default-batch"),
		GPUUsageSnapshotWindowMin:   clampInt(parser.envInt(envGPUSnapshotWindowMinutes, 10), 1, 1440),
		GPUUsageRetentionDays:       clampInt(parser.envInt(envGPUUsageRetentionDays, 30), 1, 3650),
		LonghornNamespace:           strings.TrimSpace(env(envLonghornNamespace, "longhorn-system")),
		LonghornRWXHealthInterval:   parser.envDuration(envLonghornRWXHealthInterval, 30*time.Second),
		LonghornRWXAutoRepair:       parser.envBool(envLonghornRWXAutoRepair, false),
		LonghornRWXRepairCooldown:   parser.envDuration(envLonghornRWXRepairCooldown, 10*time.Minute),
		LonghornRWXSnapshotWarn:     parser.envInt(envLonghornRWXSnapshotWarn, 20),
		LonghornRWXSnapshotBlock:    parser.envInt(envLonghornRWXSnapshotBlock, 50),
		PriorityClassSyncInterval:   parser.envDuration(envPriorityClassSyncInterval, time.Minute),
		VPNUsageEnabled:             parser.envBool("VPN_USAGE_ENABLED", true),
		VPNUsageGrace:               parser.envDuration("VPN_USAGE_GRACE", time.Minute),
		CLICACertPEM:                cliCACertPEM(os.Getenv(envCLICACertPEM)),
		VPNAPIURLs:                  firstNonEmptyList(parseList(os.Getenv(envVPNAPIURLs)), parseList(os.Getenv(envVPNAPIURL))),
		VPNAPIKey:                   strings.TrimSpace(os.Getenv(envVPNAPIKey)),
		VPNAPITimeout:               parser.envDurationOrSeconds(envVPNAPITimeout, 5*time.Second),
		MinIOConsoleAccessKey:       strings.TrimSpace(os.Getenv(envMinIOConsoleAccessKey)),
		MinIOConsoleSecretKey:       strings.TrimSpace(os.Getenv(envMinIOConsoleSecretKey)),
		MinIOOperationTimeout:       parser.envDurationOrSeconds(envMinIOOperationTimeout, 10*time.Second),
		PGAdminDefaultEmail:         strings.TrimSpace(os.Getenv(envPGAdminDefaultEmail)),
		PGAdminDefaultPassword:      strings.TrimSpace(os.Getenv(envPGAdminDefaultPassword)),
		PGAdminSSOHTTPTimeout:       parser.envDurationOrSeconds(envPGAdminSSOHTTPTimeout, 10*time.Second),
		StreamTURNURIs:              parseList(os.Getenv(envStreamTURNURIs)),
		StreamTURNSharedSecret:      strings.TrimSpace(os.Getenv(envStreamTURNSharedSecret)),
		StreamTURNCredentialTTL:     parser.envDuration(envStreamTURNCredentialTTL, 8*time.Hour),
		StreamMaxBitrateKbps:        parser.envInt(envStreamMaxBitrateKbps, 12000),
		StreamMaxConcurrentSessions: parser.envInt(envStreamMaxConcurrentSessions, 64),
		StreamEgressBudgetKbps:      parser.envInt(envStreamEgressBudgetKbps, 800000),
		StreamSidecarImage:          strings.TrimSpace(os.Getenv(envStreamSidecarImage)),
		StorageClassOptions:         parseList(env(envStorageClassOptions, "standard,fast")),
		GroupStorageClassOptions:    firstNonEmptyList(parseList(os.Getenv(envGroupStorageClassOptions)), parseList(os.Getenv(envStorageClassOptions))),
		GroupRegistryProfileOptions: firstNonEmptyList(parseList(os.Getenv(envGroupRegistryProfileOptions)), parseList(os.Getenv(envRegistryProfileOptions))),
		ExternalURLs: map[string]string{
			"k8s":           os.Getenv("K8S_CONTROL_URL"),
			"harbor":        os.Getenv("HARBOR_URL"),
			"minio":         os.Getenv("MINIO_URL"),
			"minio-console": os.Getenv("MINIO_CONSOLE_URL"),
			"pgadmin":       os.Getenv("PGADMIN_URL"),
			"longhorn":      os.Getenv("LONGHORN_URL"),
			"prometheus":    os.Getenv("PROMETHEUS_URL"),
		},
		AdapterConfigs: parser.parseAdapterConfigs(os.Getenv(envAdapterConfig)),
	}
	cfg.parseDiagnostics = parser.diagnostics
	return cfg
}

// parseAdapterConfigs reads ADAPTER_CONFIG, a JSON object mapping adapter name to
// its upstream routing/auth config. Invalid JSON yields an empty map; ConfigFromEnv
// records a sanitized diagnostic so production validation can fail closed.
func parseAdapterConfigs(value string) map[string]AdapterConfig {
	configs, _ := parseAdapterConfigsWithDiagnostics(value)
	return configs
}

func (p *configEnvParser) parseAdapterConfigs(value string) map[string]AdapterConfig {
	configs, err := parseAdapterConfigsWithDiagnostics(value)
	if err != nil {
		p.addDiagnostic(envAdapterConfig, configDiagnosticJSONObject)
	}
	return configs
}

func parseAdapterConfigsWithDiagnostics(value string) (map[string]AdapterConfig, error) {
	configs := map[string]AdapterConfig{}
	if strings.TrimSpace(value) == "" {
		return configs, nil
	}
	raw := map[string]AdapterConfig{}
	if err := json.Unmarshal([]byte(value), &raw); err != nil {
		return configs, err
	}
	for name, cfg := range raw {
		if name = strings.TrimSpace(name); name != "" {
			configs[name] = cfg
		}
	}
	return configs, nil
}

func defaultDevAllowedOrigins() map[string]bool {
	return map[string]bool{
		"http://localhost:3000": true,
		"http://localhost:5173": true,
		"http://127.0.0.1:3000": true,
		"http://127.0.0.1:5173": true,
	}
}

func normalizeEnvironmentProfile(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func validEnvironmentProfile(profile string) bool {
	switch profile {
	case runtimeProfileLocal, runtimeProfileTest, runtimeProfileDev, runtimeProfileStaging, runtimeProfileProduction:
		return true
	default:
		return false
	}
}

func runtimeProfileIsProduction(profile string, production bool) bool {
	if profile != "" {
		return profile == runtimeProfileProduction
	}
	return production
}

func effectiveEnvironmentProfile(profile string, production bool) string {
	if profile != "" {
		return profile
	}
	if production {
		return runtimeProfileProduction
	}
	return runtimeProfileDev
}

func profileConflictsWithProductionFlag(profile string, production bool, productionSet bool, productionRaw string) bool {
	if profile == "" || !productionSet || strings.TrimSpace(productionRaw) == "" {
		return false
	}
	if !validEnvironmentProfile(profile) {
		return false
	}
	return production != (profile == runtimeProfileProduction)
}

func (c Config) Validate() error {
	if err := c.validateParseDiagnostics(); err != nil {
		return err
	}
	if err := c.validateEnvironmentProfile(); err != nil {
		return err
	}
	if err := c.validateLDAP(); err != nil {
		return err
	}
	if err := c.validateLonghornRWX(); err != nil {
		return err
	}
	if err := c.validatePriorityClassSync(); err != nil {
		return err
	}
	if err := c.validateDockerCleanup(); err != nil {
		return err
	}
	if err := c.validateStreamConfig(); err != nil {
		return err
	}
	if err := c.validateInputLimits(); err != nil {
		return err
	}
	if err := c.validateEventRelay(); err != nil {
		return err
	}
	if c.IsProductionProfile() {
		if err := c.validateProduction(); err != nil {
			return err
		}
	} else if err := c.validateNonProduction(); err != nil {
		return err
	}
	return c.validateStrictServiceIdentity()
}

func (c Config) validateEnvironmentProfile() error {
	profile := strings.TrimSpace(c.EnvironmentProfile)
	if profile != "" && !validEnvironmentProfile(profile) {
		return fmt.Errorf("%s must be one of local, test, dev, staging, production", envAppEnv)
	}
	if c.Production && profile != "" && profile != runtimeProfileProduction {
		return fmt.Errorf("%s=%s conflicts with %s=true", envAppEnv, profile, envProduction)
	}
	return nil
}

func (c Config) validateProduction() error {
	if c.DevHeaderAuth {
		return errors.New("DEV_HEADER_AUTH must be false in production")
	}
	if c.DevAuthSigningKey != "" {
		return errors.New("DEV_AUTH_SIGNING_KEY must not be set in production")
	}
	if !c.RequireAuth {
		return errors.New("REQUIRE_AUTH must be true in production")
	}
	if err := c.validateProductionAuth(); err != nil {
		return err
	}
	return c.validateProductionBackingServices()
}

func (c Config) validateNonProduction() error {
	if !c.RequireAuth && !c.DevHeaderAuth {
		return errors.New("DEV_HEADER_AUTH must be true when REQUIRE_AUTH=false")
	}
	return nil
}

func (c Config) validateLDAP() error {
	if !c.LDAPEnabled {
		return nil
	}
	if missing := c.missingLDAPConfig(); len(missing) > 0 {
		return errors.New(strings.Join(missing, ", ") + " must be set when " + envLDAPEnabled + "=true")
	}
	if c.LDAPPort < 1 || c.LDAPPort > 65535 {
		return errors.New(envLDAPPort + " must be between 1 and 65535 when " + envLDAPEnabled + "=true")
	}
	if strings.Count(c.LDAPUserFilter, "%s") != 1 {
		return errors.New(envLDAPUserFilter + " must contain exactly one username placeholder")
	}
	return nil
}

func (c Config) validateLonghornRWX() error {
	if c.LonghornRWXHealthInterval <= 0 {
		return errors.New(envLonghornRWXHealthInterval + configPositiveValidationSuffix)
	}
	if c.LonghornRWXRepairCooldown <= 0 {
		return errors.New(envLonghornRWXRepairCooldown + configPositiveValidationSuffix)
	}
	if c.LonghornRWXSnapshotWarn < 0 {
		return errors.New(envLonghornRWXSnapshotWarn + " must be non-negative")
	}
	if c.LonghornRWXSnapshotBlock > 0 && c.LonghornRWXSnapshotBlock < c.LonghornRWXSnapshotWarn {
		return errors.New(envLonghornRWXSnapshotBlock + " must be greater than or equal to " + envLonghornRWXSnapshotWarn)
	}
	return nil
}

func (c Config) validatePriorityClassSync() error {
	if c.PriorityClassSyncInterval <= 0 {
		return errors.New(envPriorityClassSyncInterval + configPositiveValidationSuffix)
	}
	return nil
}

func (c Config) validateDockerCleanup() error {
	if !c.DockerCleanupEnabled {
		return nil
	}
	if !validKubernetesNamespaceName(c.DockerCleanupNamespace) {
		return errors.New(envDockerCleanupNamespace + " must be a valid Kubernetes namespace when " + envDockerCleanupEnabled + "=true")
	}
	if strings.TrimSpace(c.DockerCleanupImage) == "" {
		return errors.New(envDockerDindImage + " is required when " + envDockerCleanupEnabled + "=true")
	}
	return nil
}

func (c Config) validateStreamConfig() error {
	ttl := c.StreamTURNCredentialTTL
	if ttl == 0 {
		ttl = 8 * time.Hour
	}
	maxBitrate := c.StreamMaxBitrateKbps
	if maxBitrate == 0 {
		maxBitrate = 12000
	}
	maxSessions := c.StreamMaxConcurrentSessions
	if maxSessions == 0 {
		maxSessions = 64
	}
	budget := c.StreamEgressBudgetKbps
	if budget == 0 {
		budget = 800000
	}
	if ttl <= 0 {
		return errors.New(envStreamTURNCredentialTTL + configPositiveValidationSuffix)
	}
	if ttl > 12*time.Hour {
		return errors.New(envStreamTURNCredentialTTL + " must be no more than 12h")
	}
	if maxBitrate <= 0 {
		return errors.New(envStreamMaxBitrateKbps + configPositiveValidationSuffix)
	}
	if maxSessions <= 0 {
		return errors.New(envStreamMaxConcurrentSessions + configPositiveValidationSuffix)
	}
	if budget <= 0 {
		return errors.New(envStreamEgressBudgetKbps + configPositiveValidationSuffix)
	}
	if maxSessions*maxBitrate > budget {
		return errors.New(envStreamMaxConcurrentSessions + " * " + envStreamMaxBitrateKbps + " must not exceed " + envStreamEgressBudgetKbps)
	}
	if c.IsProductionProfile() && len(c.StreamTURNURIs) > 0 && strings.TrimSpace(c.StreamTURNSharedSecret) == "" {
		return errors.New(envStreamTURNSharedSecret + " is required when " + envStreamTURNURIs + " is set in production")
	}
	return nil
}

func (c Config) validateInputLimits() error {
	if c.MaxAPIBodyBytes < 0 {
		return errors.New(envMaxAPIBodyBytes + configPositiveValidationSuffix)
	}
	if c.MaxConfigFileBytes < 0 {
		return errors.New(envMaxConfigFileBytes + configPositiveValidationSuffix)
	}
	if c.MaxConfigFileDocuments < 0 {
		return errors.New(envMaxConfigFileDocuments + configPositiveValidationSuffix)
	}
	if c.EffectiveMaxAPIBodyBytes() <= 0 {
		return errors.New(envMaxAPIBodyBytes + configPositiveValidationSuffix)
	}
	if c.EffectiveMaxConfigFileBytes() <= 0 {
		return errors.New(envMaxConfigFileBytes + configPositiveValidationSuffix)
	}
	if c.EffectiveMaxConfigFileDocuments() <= 0 {
		return errors.New(envMaxConfigFileDocuments + configPositiveValidationSuffix)
	}
	return nil
}

func (c Config) validateEventRelay() error {
	if c.EventRelayBatchSize < 0 {
		return errors.New(envEventRelayBatchSize + configPositiveValidationSuffix)
	}
	if c.EffectiveEventRelayBatchSize() <= 0 {
		return errors.New(envEventRelayBatchSize + configPositiveValidationSuffix)
	}
	return nil
}

func validKubernetesNamespaceName(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 63 {
		return false
	}
	for i, r := range value {
		valid := r == '-' || (r >= '0' && r <= '9') || (r >= 'a' && r <= 'z')
		if !valid {
			return false
		}
		if (i == 0 || i == len(value)-1) && r == '-' {
			return false
		}
	}
	return true
}

func (c Config) missingLDAPConfig() []string {
	values := map[string]string{
		envLDAPHost:           c.LDAPHost,
		envLDAPBindDN:         c.LDAPBindDN,
		envLDAPBindPassword:   c.LDAPBindPassword,
		envLDAPUserSearchBase: c.LDAPUserSearchBase,
		envLDAPUserFilter:     c.LDAPUserFilter,
	}
	missing := []string{}
	for name, value := range values {
		if strings.TrimSpace(value) == "" {
			missing = append(missing, name)
		}
	}
	return missing
}

func (c Config) validateParseDiagnostics() error {
	if len(c.parseDiagnostics) == 0 {
		return nil
	}
	fatal := c.StrictRuntimeChecks()
	for _, diagnostic := range c.parseDiagnostics {
		if diagnostic.envName == envProduction || diagnostic.envName == envAppEnv {
			fatal = true
			break
		}
	}
	if !fatal {
		return nil
	}
	parts := make([]string, 0, len(c.parseDiagnostics))
	for _, diagnostic := range c.parseDiagnostics {
		parts = append(parts, diagnostic.String())
	}
	return fmt.Errorf("invalid configuration: %s", strings.Join(parts, "; "))
}

func (c Config) validateProductionAuth() error {
	if !hasEnabledAPIKey(c.APIKeys) && c.JWKSURL == "" {
		return errors.New("API_KEYS or JWT_JWKS_URL is required in production")
	}
	if hasEnabledAPIKey(c.APIKeys) && len(missingAPIKeyPrincipals(c.APIKeys, c.APIKeyPrincipals)) > 0 {
		return errors.New("API_KEY_PRINCIPALS must define a principal for every API_KEYS entry in production")
	}
	if c.JWKSURL != "" {
		if !strings.HasPrefix(c.JWKSURL, "https://") {
			return errors.New("JWT_JWKS_URL must use https in production")
		}
		if c.JWTIssuer == "" {
			return errors.New("JWT_ISSUER is required when JWT_JWKS_URL is set in production")
		}
		if len(c.JWTAudiences) == 0 {
			return errors.New("JWT_AUDIENCE or JWT_AUDIENCES is required when JWT_JWKS_URL is set in production")
		}
	}
	if !c.AllowsService("authorization-policy-service") && strings.TrimSpace(c.AuthorizationPolicyURL) == "" {
		return errors.New("AUTHORIZATION_POLICY_URL is required for isolated production services")
	}
	if strings.TrimSpace(c.AuthorizationPolicyURL) != "" && strings.TrimSpace(c.AuthorizationPolicyAPIKey) == "" {
		return errors.New("AUTHORIZATION_POLICY_API_KEY is required when AUTHORIZATION_POLICY_URL is set in production")
	}
	return nil
}

func (c Config) validateStrictServiceIdentity() error {
	if !c.StrictRuntimeChecks() || !c.requiresRemoteServiceAuth() {
		return nil
	}
	missing := []string{}
	if strings.TrimSpace(c.ServiceIdentityName) == "" {
		missing = append(missing, envServiceIdentityName)
	}
	if strings.TrimSpace(c.ServiceIdentityKey) == "" {
		missing = append(missing, envServiceIdentityKey)
	}
	if len(c.ServiceTrustedIdentities) == 0 {
		missing = append(missing, envServiceTrustedIdentities)
	}
	if len(missing) == 0 {
		return nil
	}
	return errors.New(strings.Join(missing, ", ") + " are required for scoped internal service identity in staging/production")
}

func (c Config) requiresRemoteServiceAuth() bool {
	return len(c.ServiceURLs) > 0 || strings.TrimSpace(c.AuthorizationPolicyURL) != ""
}

func (c Config) validateProductionBackingServices() error {
	missing := []string{}
	if strings.TrimSpace(c.DatabaseURL) == "" {
		missing = append(missing, envDatabaseURL)
	}
	if strings.TrimSpace(c.RedisURL) == "" {
		missing = append(missing, envRedisURL)
	}
	if strings.TrimSpace(c.EventBusURL) == "" {
		missing = append(missing, envEventBusURL)
	}
	if c.RequiresObjectStore() && strings.TrimSpace(c.ObjectStoreURL) == "" {
		missing = append(missing, envObjectStoreURL)
	}
	if len(missing) > 0 {
		return errors.New(strings.Join(missing, ", ") + " must be set in production")
	}
	if err := validateRedisBackingURL(envRedisURL, c.RedisURL); err != nil {
		return err
	}
	if err := validateRedisBackingURL(envEventBusURL, c.EventBusURL); err != nil {
		return err
	}
	if !c.RequiresObjectStore() {
		return nil
	}
	return c.validateProductionObjectStore()
}

func (c Config) validateProductionObjectStore() error {
	parsed, err := url.Parse(strings.TrimSpace(c.ObjectStoreURL))
	if err != nil || parsed.Host == "" {
		return fmt.Errorf("%s must be an http:// or https:// URL", envObjectStoreURL)
	}
	switch strings.ToLower(parsed.Scheme) {
	case "http", "https":
	default:
		return fmt.Errorf("%s must be an http:// or https:// URL", envObjectStoreURL)
	}
	if strings.TrimSpace(c.ObjectStoreAccessKey) == "" || strings.TrimSpace(c.ObjectStoreSecretKey) == "" {
		return fmt.Errorf("%s and %s are required when %s is set in production", envObjectStoreAccessKey, envObjectStoreSecretKey, envObjectStoreURL)
	}
	return nil
}

func validateRedisBackingURL(name, value string) error {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || parsed.Host == "" {
		return fmt.Errorf("%s must be a redis:// or rediss:// URL", name)
	}
	switch strings.ToLower(parsed.Scheme) {
	case "redis", "rediss":
		return nil
	default:
		return fmt.Errorf("%s must be a redis:// or rediss:// URL", name)
	}
}

func (c Config) AllowsService(name string) bool {
	serviceName := strings.TrimSpace(c.ServiceName)
	if serviceName == "" || serviceName == "all" || serviceName == name {
		return true
	}
	for _, hosted := range deployableUnitServices[serviceName] {
		if hosted == name {
			return true
		}
	}
	return false
}

// RequiresObjectStore reports whether this process hosts the blob-owning media
// upload capability and therefore needs OBJECT_STORE_* attached.
func (c Config) RequiresObjectStore() bool {
	return c.AllowsService(mediaUploadServiceName)
}

// TracingEnabled reports whether an OTLP trace endpoint is configured. When it is
// not, the runtime installs a no-op tracer provider so no spans are exported.
func (c Config) TracingEnabled() bool {
	return c.OTLPEndpoint != ""
}

func (c Config) EnvironmentName() string {
	return c.EffectiveEnvironmentProfile()
}

func (c Config) EffectiveEnvironmentProfile() string {
	return effectiveEnvironmentProfile(c.EnvironmentProfile, c.Production)
}

func (c Config) IsProductionProfile() bool {
	return c.EffectiveEnvironmentProfile() == runtimeProfileProduction
}

func (c Config) StrictRuntimeChecks() bool {
	switch c.EffectiveEnvironmentProfile() {
	case runtimeProfileStaging, runtimeProfileProduction:
		return true
	default:
		return false
	}
}

func (c Config) EffectiveMaxAPIBodyBytes() int {
	if c.MaxAPIBodyBytes > 0 {
		return c.MaxAPIBodyBytes
	}
	return defaultMaxAPIBodyBytes
}

func (c Config) EffectiveMaxConfigFileBytes() int {
	if c.MaxConfigFileBytes > 0 {
		return c.MaxConfigFileBytes
	}
	return defaultMaxConfigFileBytes
}

func (c Config) EffectiveMaxConfigFileDocuments() int {
	if c.MaxConfigFileDocuments > 0 {
		return c.MaxConfigFileDocuments
	}
	return defaultMaxConfigFileDocuments
}

func (c Config) EffectiveEventRelayBatchSize() int {
	if c.EventRelayBatchSize > 0 {
		return c.EventRelayBatchSize
	}
	return defaultEventRelayBatchSize
}

func (c Config) EffectiveProductName() string {
	if name := strings.TrimSpace(c.ProductName); name != "" {
		return name
	}
	return defaultProductName
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func clampInt(value, min, max int) int {
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}

func (p *configEnvParser) envBool(key string, fallback bool) bool {
	value, ok := os.LookupEnv(key)
	if !ok || value == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		p.addDiagnostic(key, "bool")
		return fallback
	}
	return parsed
}

func (p *configEnvParser) envDuration(key string, fallback time.Duration) time.Duration {
	value, ok := os.LookupEnv(key)
	if !ok || value == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		p.addDiagnostic(key, "duration")
		return fallback
	}
	return parsed
}

func (p *configEnvParser) envDurationOrSeconds(key string, fallback time.Duration) time.Duration {
	value, ok := os.LookupEnv(key)
	if !ok || strings.TrimSpace(value) == "" {
		return fallback
	}
	value = strings.TrimSpace(value)
	if parsed, err := time.ParseDuration(value); err == nil && parsed > 0 {
		return parsed
	}
	if seconds, err := strconv.Atoi(value); err == nil && seconds > 0 {
		return time.Duration(seconds) * time.Second
	}
	p.addDiagnostic(key, "duration or positive integer seconds")
	return fallback
}

func (p *configEnvParser) envInt(key string, fallback int) int {
	value, ok := os.LookupEnv(key)
	if !ok || value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		p.addDiagnostic(key, "integer")
		return fallback
	}
	return parsed
}

func parseSet(value string) map[string]bool {
	set := map[string]bool{}
	for _, item := range strings.Split(value, ",") {
		item = strings.TrimSpace(item)
		if item != "" {
			set[item] = true
		}
	}
	return set
}

func parseList(value string) []string {
	out := []string{}
	for _, item := range strings.Split(value, ",") {
		item = strings.TrimSpace(item)
		if item != "" {
			out = append(out, item)
		}
	}
	return out
}

func firstNonEmptyList(values ...[]string) []string {
	for _, value := range values {
		if len(value) > 0 {
			return value
		}
	}
	return []string{}
}

func cliCACertPEM(value string) string {
	if cert := strings.TrimSpace(value); cert != "" {
		return cert
	}
	return defaultCLICACertPEM
}

func parseJWTAudiences() map[string]bool {
	return parseSet(os.Getenv(envJWTAudience) + "," + os.Getenv(envJWTAudiences))
}

// parseServiceURLs reads SERVICE_URLS, a JSON object mapping owning service name
// to its base URL (e.g. {"identity-service":"http://identity-service"}). Isolated
// deployments use it to resolve cross-service reads over HTTP (finding 5).
func parseServiceURLs(value string) map[string]string {
	urls, _ := parseServiceURLsWithDiagnostics(value)
	return urls
}

func (p *configEnvParser) parseServiceURLs(value string) map[string]string {
	urls, err := parseServiceURLsWithDiagnostics(value)
	if err != nil {
		p.addDiagnostic(envServiceURLs, configDiagnosticJSONObject)
	}
	return urls
}

func parseServiceURLsWithDiagnostics(value string) (map[string]string, error) {
	urls := map[string]string{}
	if strings.TrimSpace(value) == "" {
		return urls, nil
	}
	raw := map[string]string{}
	if err := json.Unmarshal([]byte(value), &raw); err != nil {
		return urls, err
	}
	for name, url := range raw {
		name = strings.TrimSpace(name)
		url = strings.TrimRight(strings.TrimSpace(url), "/")
		if name != "" && url != "" {
			urls[name] = url
		}
	}
	return urls, nil
}

func defaultServiceIdentityName(serviceName, configured string) string {
	if name := strings.TrimSpace(configured); name != "" {
		return name
	}
	serviceName = strings.TrimSpace(serviceName)
	if serviceName == "" || serviceName == "all" {
		return ""
	}
	return serviceName
}

func (c Config) applyServiceIdentityHeaders(headers http.Header) bool {
	name := strings.TrimSpace(c.ServiceIdentityName)
	key := strings.TrimSpace(c.ServiceIdentityKey)
	if name != "" && key != "" {
		headers.Set(serviceNameHeader, name)
		headers.Set(serviceKeyHeader, key)
		return true
	}
	if c.StrictRuntimeChecks() {
		headers.Del(serviceNameHeader)
		headers.Del(serviceKeyHeader)
		return false
	}
	if key := strings.TrimSpace(c.ServiceAPIKey); key != "" {
		headers.Del(serviceNameHeader)
		headers.Set(serviceKeyHeader, key)
		return true
	}
	return false
}

func (c Config) canSendServiceIdentity() bool {
	return c.canSendScopedServiceIdentity() || strings.TrimSpace(c.ServiceAPIKey) != ""
}

func (c Config) canSendScopedServiceIdentity() bool {
	return strings.TrimSpace(c.ServiceIdentityName) != "" && strings.TrimSpace(c.ServiceIdentityKey) != ""
}

func (c Config) canUseRemoteServiceIdentity() bool {
	if c.StrictRuntimeChecks() {
		return c.canSendScopedServiceIdentity()
	}
	return c.canSendServiceIdentity()
}

func (c Config) acceptsServiceIdentity() bool {
	return len(c.ServiceTrustedIdentities) > 0 || strings.TrimSpace(c.ServiceAPIKey) != ""
}

func (p *configEnvParser) parseServiceTrustedIdentities(value string) map[string]ServiceTrustedIdentity {
	identities, err := parseServiceTrustedIdentitiesWithDiagnostics(value)
	if err != nil {
		p.addDiagnostic(envServiceTrustedIdentities, configDiagnosticJSONObject)
	}
	return identities
}

func parseServiceTrustedIdentitiesWithDiagnostics(value string) (map[string]ServiceTrustedIdentity, error) {
	identities := map[string]ServiceTrustedIdentity{}
	if strings.TrimSpace(value) == "" {
		return identities, nil
	}
	raw := map[string]ServiceTrustedIdentity{}
	if err := json.Unmarshal([]byte(value), &raw); err != nil {
		return identities, err
	}
	for name, identity := range raw {
		name = strings.TrimSpace(name)
		identity = identity.normalized()
		if name != "" && identity.Key != "" && len(identity.Audiences) > 0 {
			identities[name] = identity
		}
	}
	return identities, nil
}

func (i ServiceTrustedIdentity) normalized() ServiceTrustedIdentity {
	i.Key = strings.TrimSpace(i.Key)
	audiences := []string{}
	seen := map[string]bool{}
	for _, audience := range i.Audiences {
		audience = strings.TrimSpace(audience)
		if audience != "" && !seen[audience] {
			seen[audience] = true
			audiences = append(audiences, audience)
		}
	}
	i.Audiences = audiences
	return i
}

func parseAPIKeyPrincipals(value string) map[string]APIKeyPrincipal {
	principals, _ := parseAPIKeyPrincipalsWithDiagnostics(value)
	return principals
}

func (p *configEnvParser) parseAPIKeyPrincipals(value string) map[string]APIKeyPrincipal {
	principals, err := parseAPIKeyPrincipalsWithDiagnostics(value)
	if err != nil {
		p.addDiagnostic(envAPIKeyUsers, configDiagnosticJSONObject)
	}
	return principals
}

func parseAPIKeyPrincipalsWithDiagnostics(value string) (map[string]APIKeyPrincipal, error) {
	principals := map[string]APIKeyPrincipal{}
	if strings.TrimSpace(value) == "" {
		return principals, nil
	}
	raw := map[string]APIKeyPrincipal{}
	if err := json.Unmarshal([]byte(value), &raw); err != nil {
		return principals, err
	}
	for key, principal := range raw {
		key = strings.TrimSpace(key)
		principal = principal.normalized()
		if key != "" && principal.ID != "" {
			principals[key] = principal
		}
	}
	return principals, nil
}

func missingAPIKeyPrincipals(keys map[string]bool, principals map[string]APIKeyPrincipal) []string {
	missing := []string{}
	for key, enabled := range keys {
		if !enabled {
			continue
		}
		if principals[key].normalized().ID == "" {
			missing = append(missing, key)
		}
	}
	return missing
}

func parseTrustedProxyCIDRs(value string) []*net.IPNet {
	cidrs, _ := parseTrustedProxyCIDRsWithDiagnostics(value)
	return cidrs
}

func parseTrustedProxyCIDRsWithDiagnostics(value string) ([]*net.IPNet, error) {
	cidrs := []*net.IPNet{}
	invalid := false
	for _, item := range strings.Split(value, ",") {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if strings.Contains(item, "/") {
			if _, cidr, err := net.ParseCIDR(item); err == nil {
				cidrs = append(cidrs, cidr)
			} else {
				invalid = true
			}
			continue
		}
		ip := net.ParseIP(item)
		if ip == nil {
			invalid = true
			continue
		}
		bits := 128
		if v4 := ip.To4(); v4 != nil {
			ip = v4
			bits = 32
		}
		cidrs = append(cidrs, &net.IPNet{IP: ip, Mask: net.CIDRMask(bits, bits)})
	}
	if invalid {
		return cidrs, errors.New("invalid trusted proxy CIDR/IP entry")
	}
	return cidrs, nil
}
