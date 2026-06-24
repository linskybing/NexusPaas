package platform

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestDeploymentManifestsAreProductionHardened(t *testing.T) {
	deployments := serviceDeploymentManifests(t)
	if len(deployments) != 15 {
		t.Fatalf("deployment manifest count = %d, want 15: %#v", len(deployments), deployments)
	}
	for _, path := range deployments {
		requireProductionDeploymentManifest(t, path)
	}

	rootDockerignore := readTextFile(t, "../../../.dockerignore")
	requireContains(t, "../../../.dockerignore", rootDockerignore, "frontend/node_modules/")
	requireContains(t, "../../../.dockerignore", rootDockerignore, "frontend/dist/")
	requireContains(t, "../../../.dockerignore", rootDockerignore, "**/.env")

	sharedDockerfile := readTextFile(t, "../../Dockerfile")
	requireContains(t, "../../Dockerfile", sharedDockerfile, "FROM node:24.15.0-alpine AS web-build")
	requireContains(t, "../../Dockerfile", sharedDockerfile, "RUN npm ci")
	requireContains(t, "../../Dockerfile", sharedDockerfile, "RUN npm run build")
	requireContains(t, "../../Dockerfile", sharedDockerfile, "COPY backend/go.mod backend/go.sum ./")
	requireContains(t, "../../Dockerfile", sharedDockerfile, "RUN go mod download")
	requireContains(t, "../../Dockerfile", sharedDockerfile, "COPY backend/cmd ./cmd")
	requireContains(t, "../../Dockerfile", sharedDockerfile, "COPY backend/internal ./internal")
	requireContains(t, "../../Dockerfile", sharedDockerfile, "go build -trimpath -o /out/microservice ./cmd/microservice")
	requireContains(t, "../../Dockerfile", sharedDockerfile, "FROM alpine:3.22")
	requireContains(t, "../../Dockerfile", sharedDockerfile, "RUN apk add --no-cache --upgrade ca-certificates libcrypto3 libssl3 \\")
	requireContains(t, "../../Dockerfile", sharedDockerfile, "&& addgroup -S -g 10001 app \\")
	requireContains(t, "../../Dockerfile", sharedDockerfile, "&& adduser -S -D -H -u 10001 -G app app")
	requireContains(t, "../../Dockerfile", sharedDockerfile, "COPY --from=web-build --chown=app:app /src/frontend/dist ./web")
	requireContains(t, "../../Dockerfile", sharedDockerfile, "ENV WEB_UI_DIR=/app/web")
	requireContains(t, "../../Dockerfile", sharedDockerfile, "USER app:app")
	requireFileExists(t, "../../go.sum")

	dockerfiles, err := filepath.Glob("../../*/Dockerfile")
	if err != nil {
		t.Fatal(err)
	}
	if len(dockerfiles) != 15 {
		t.Fatalf("Dockerfile count = %d, want 15: %#v", len(dockerfiles), dockerfiles)
	}
	for _, path := range dockerfiles {
		body := readTextFile(t, path)
		requireContains(t, path, body, "ARG BASE_IMAGE=nexuspaas-backend:v0.1.0")
		requireContains(t, path, body, "FROM ${BASE_IMAGE}")
		requireNotContains(t, path, body, "go build")
		requireNotContains(t, path, body, "COPY . .")
		requireNotContains(t, path, body, "RUN apk add --no-cache ca-certificates")
	}
}

func TestProductionBetaKustomizationIncludesEightBackendUnits(t *testing.T) {
	path := "../../kustomization.yaml"
	body := readTextFile(t, path)
	requireContains(t, path, body, "deploy/k3s/production-beta/runtime-config.yaml")
	requireContains(t, path, body, "deploy/k3s/production-beta/runtime-secret-contract.yaml")
	requireContains(t, path, body, "deploy/k3s/production-beta/backend-units.yaml")
	requireContains(t, path, body, "deploy/k3s/postgres.yaml")
	requireContains(t, path, body, "deploy/k3s/redis.yaml")
	requireContains(t, path, body, "deploy/k3s/minio.yaml")
	requireContains(t, path, body, "deploy/k3s/dex.yaml")
	requireContains(t, path, body, "deploy/k3s/production-beta/backing-secret-postgres-patch.yaml")
	requireContains(t, path, body, "deploy/k3s/production-beta/backing-secret-dex-patch.yaml")
	requireContains(t, path, body, "deploy/k3s/production-beta/backing-secret-minio-patch.yaml")
	requireNotContains(t, path, body, "deploy/k3s/platform.yaml")
	requireNotContains(t, path, body, "/k8s/deployment.yaml")

	unitManifestPath := "../../deploy/k3s/production-beta/backend-units.yaml"
	unitManifest := readTextFile(t, unitManifestPath)
	for _, unit := range productionBackendUnits() {
		requireContains(t, unitManifestPath, unitManifest, fmt.Sprintf("name: %s", unit))
		requireContains(t, unitManifestPath, unitManifest, fmt.Sprintf("SERVICE_NAME: %q", unit))
		requireContains(t, unitManifestPath, unitManifest, fmt.Sprintf("OTEL_SERVICE_NAME: %q", unit))
		requireContains(t, unitManifestPath, unitManifest, fmt.Sprintf("name: %s-runtime-secret", unit))
	}
	if got := strings.Count(unitManifest, "\nkind: Deployment\n"); got != 8 {
		t.Fatalf("%s deployment count = %d, want 8", unitManifestPath, got)
	}
}

func TestProductionBetaRuntimeConfigAndSecretContract(t *testing.T) {
	configPath := "../../deploy/k3s/production-beta/runtime-config.yaml"
	config := readTextFile(t, configPath)
	requireContains(t, configPath, config, "name: production-beta-runtime-config")
	requireContains(t, configPath, config, `REDIS_URL: "redis://redis:6379/0"`)
	requireContains(t, configPath, config, `EVENT_BUS_URL: "redis://redis:6379/1"`)
	requireContains(t, configPath, config, `DEX_URL: ""`)
	requireContains(t, configPath, config, `HARBOR_URL: ""`)
	requireNotContains(t, configPath, config, `DEX_URL: "http://`)
	requireNotContains(t, configPath, config, `HARBOR_URL: "http://`)
	requireContains(t, configPath, config, `EVENT_RELAY_BATCH_SIZE: "100"`)
	requireContains(t, configPath, config, `JWT_AUDIENCE: "platform"`)
	serviceURLs := extractServiceURLs(t, configPath, config)

	contractPath := "../../deploy/k3s/production-beta/runtime-secret-contract.yaml"
	contract := readTextFile(t, contractPath)
	requireContains(t, contractPath, contract, "name: production-beta-runtime-secret-contract")
	requireContains(t, contractPath, contract, "`SERVICE_API_KEY`")
	requireContains(t, contractPath, contract, "`SERVICE_IDENTITY_KEY`")
	requireContains(t, contractPath, contract, "`SERVICE_TRUSTED_IDENTITIES`")
	requireContains(t, contractPath, contract, "`AUTHORIZATION_POLICY_URL`")
	requireContains(t, contractPath, contract, "`AUTHORIZATION_POLICY_API_KEY`")
	requireContains(t, contractPath, contract, "`postgres-password`")
	requireContains(t, contractPath, contract, "`dex-password`")
	requireContains(t, contractPath, contract, "`minio-credentials`")
	requireContains(t, contractPath, contract, "`coturn-runtime-secret`")
	requireContains(t, contractPath, contract, "`STREAM_TURN_SHARED_SECRET`")
	requireNotContains(t, contractPath, contract, "-dev-")

	for _, patch := range []struct {
		path string
		name string
	}{
		{"../../deploy/k3s/production-beta/backing-secret-postgres-patch.yaml", "name: postgres-password"},
		{"../../deploy/k3s/production-beta/backing-secret-dex-patch.yaml", "name: dex-password"},
		{"../../deploy/k3s/production-beta/backing-secret-minio-patch.yaml", "name: minio-credentials"},
	} {
		body := readTextFile(t, patch.path)
		requireContains(t, patch.path, body, patch.name)
		requireNotContains(t, patch.path, body, "-dev-")
	}

	runtimePatchPath := "../../deploy/k3s/production-beta/runtime-config-envfrom-patch.yaml"
	if _, err := os.Stat(runtimePatchPath); !os.IsNotExist(err) {
		t.Fatalf("%s should not exist after backend units own their envFrom wiring", runtimePatchPath)
	}

	kustomizationPath := "../../kustomization.yaml"
	kustomization := readTextFile(t, kustomizationPath)
	requireContains(t, kustomizationPath, kustomization, "deploy/k3s/production-beta/backend-units.yaml")

	for service, unit := range logicalServiceUnits() {
		wantURL := "http://" + unit
		if got := serviceURLs[service]; got != wantURL {
			t.Fatalf("%s SERVICE_URLS[%s] = %q, want %s", configPath, service, got, wantURL)
		}
	}
	if len(serviceURLs) != 15 {
		t.Fatalf("%s SERVICE_URLS count = %d, want 15", configPath, len(serviceURLs))
	}
	for _, unit := range productionBackendUnits() {
		requireContains(t, contractPath, contract, fmt.Sprintf("`%s-runtime-secret`", unit))
	}
}

func TestProductionBetaSecretDeployPathStaticEvidence(t *testing.T) {
	requiredSecrets := productionBetaRequiredSecretNames()

	for _, path := range productionBetaSecretSourcePaths(t) {
		body := readTextFile(t, path)
		if strings.HasSuffix(path, "production-beta-live-rehearsal.sh") {
			body = withoutShellFunction(body, "forbidden_secret_ref_pattern")
		}
		requireNoForbiddenProductionBetaSecretRefs(t, path, body)
	}

	kustomizationPath := "../../kustomization.yaml"
	kustomization := readTextFile(t, kustomizationPath)
	for _, required := range []string{
		"deploy/k3s/production-beta/runtime-config.yaml",
		"deploy/k3s/production-beta/runtime-secret-contract.yaml",
		"deploy/k3s/production-beta/backend-units.yaml",
		"deploy/k3s/production-beta/backing-secret-postgres-patch.yaml",
		"deploy/k3s/production-beta/backing-secret-dex-patch.yaml",
		"deploy/k3s/production-beta/backing-secret-minio-patch.yaml",
	} {
		requireContains(t, kustomizationPath, kustomization, required)
	}
	requireNotContains(t, kustomizationPath, kustomization, "deploy/k3s/platform.yaml")

	requireSecretKeyRef(t, "../../deploy/k3s/production-beta/backing-secret-postgres-patch.yaml", "postgres-password", "password")
	requireSecretKeyRef(t, "../../deploy/k3s/production-beta/backing-secret-dex-patch.yaml", "dex-password", "bcrypt-hash")
	requireSecretKeyRef(t, "../../deploy/k3s/production-beta/backing-secret-dex-patch.yaml", "postgres-password", "password")
	requireSecretKeyRef(t, "../../deploy/k3s/production-beta/backing-secret-minio-patch.yaml", "minio-credentials", "access-key")
	requireSecretKeyRef(t, "../../deploy/k3s/production-beta/backing-secret-minio-patch.yaml", "minio-credentials", "secret-key")

	unitManifestPath := "../../deploy/k3s/production-beta/backend-units.yaml"
	unitManifest := readTextFile(t, unitManifestPath)
	requireOnlyExpectedRuntimeSecretRefs(t, unitManifestPath, unitManifest, productionBetaRuntimeSecretNames())

	contractPath := "../../deploy/k3s/production-beta/runtime-secret-contract.yaml"
	contract := readTextFile(t, contractPath)
	for _, secret := range requiredSecrets {
		requireContains(t, contractPath, contract, "`"+secret+"`")
	}
	for _, key := range []string{
		"`password`",
		"`bcrypt-hash`",
		"`access-key`",
		"`secret-key`",
		"`TURN_STATIC_AUTH_SECRET`",
		"`DATABASE_URL`",
		"`SERVICE_IDENTITY_KEY`",
		"`SERVICE_TRUSTED_IDENTITIES`",
		"`AUTHORIZATION_POLICY_API_KEY`",
		"`STREAM_TURN_SHARED_SECRET`",
	} {
		requireContains(t, contractPath, contract, key)
	}

	scriptPath := "../../scripts/production-beta-live-rehearsal.sh"
	script := readTextFile(t, scriptPath)
	for _, secret := range requiredSecrets {
		requireContains(t, scriptPath, script, secret)
	}
	requireContains(t, scriptPath, script, "validate_render_secret_refs")
	requireContains(t, scriptPath, script, "forbidden_secret_ref_pattern")
	requireContains(t, scriptPath, script, "kctl get secret \"${secret}\" -o name")
	requireNotContains(t, scriptPath, script, "get secret \"${secret}\" -o yaml")
	requireNotContains(t, scriptPath, script, "get secret \"${secret}\" -o json")
	requireNotContains(t, scriptPath, script, "get secret \"${secret}\" -o jsonpath")
	requireNotContains(t, scriptPath, script, "jsonpath=.*data")
	requireNotContains(t, scriptPath, script, "base64")

	render := renderProductionBetaKustomization(t)
	requireNoForbiddenProductionBetaSecretRefs(t, "kubectl kustomize backend", render)
	requireNotContains(t, "kubectl kustomize backend", render, "\nkind: Secret\n")
	requireNotContains(t, "kubectl kustomize backend", render, "\nstringData:")
	for _, secret := range requiredSecrets {
		requireContains(t, "kubectl kustomize backend", render, secret)
	}
	for _, snippet := range []string{
		"key: password\n              name: postgres-password",
		"key: bcrypt-hash\n              name: dex-password",
		"key: access-key\n              name: minio-credentials",
		"key: secret-key\n              name: minio-credentials",
		"key: TURN_STATIC_AUTH_SECRET\n              name: coturn-runtime-secret",
	} {
		requireContains(t, "kubectl kustomize backend", render, snippet)
	}
}

func TestProductionOperationalReadinessDocsCoverAllServices(t *testing.T) {
	readinessPath := "../../docs/operational-readiness.md"
	readiness := readTextFile(t, readinessPath)
	nfrPath := "../../docs/non-functional-requirements.md"
	nfrs := readTextFile(t, nfrPath)
	requireContains(t, readinessPath, readiness, "Core API availability | >= 99.5%")
	requireContains(t, readinessPath, readiness, "Job submit synchronous phase | p95 < 2s")
	requireContains(t, readinessPath, readiness, "General read latency | p95 < 500ms")
	requireContains(t, readinessPath, readiness, "General write latency | p95 < 1s")
	requireContains(t, readinessPath, readiness, "`request_id`")
	requireContains(t, readinessPath, readiness, "`trace_id`")
	requireContains(t, readinessPath, readiness, "`user_id`")
	requireContains(t, readinessPath, readiness, "`project_id`")
	requireContains(t, readinessPath, readiness, "`traceparent`")
	requireContains(t, readinessPath, readiness, "`tracestate`")
	requireContains(t, readinessPath, readiness, "`OTEL_EXPORTER_OTLP_ENDPOINT`")
	requireContains(t, readinessPath, readiness, "Do not log secrets")
	requireContains(t, readinessPath, readiness, "Standard Runbook Template")
	requireContains(t, readinessPath, readiness, "Synthetic Smoke Checklist")
	requireContains(t, readinessPath, readiness, "GET /service-registry")
	requireContains(t, readinessPath, readiness, "all 15 logical services")
	requireContains(t, readinessPath, readiness, "kubectl rollout undo deployment/<unit>")
	requireContains(t, nfrPath, nfrs, "docs/operational-readiness.md")
	requireContains(t, nfrPath, nfrs, "../../docs/architecture/observability-strategy.md")

	strategyPath := "../../../docs/architecture/observability-strategy.md"
	strategy := readTextFile(t, strategyPath)
	requireContains(t, strategyPath, strategy, "8 physical backend units")
	requireContains(t, strategyPath, strategy, "../../backend/docs/operational-readiness.md")
	requireContains(t, strategyPath, strategy, "OpenTelemetry Collector")
	requireContains(t, strategyPath, strategy, "Prometheus")
	requireContains(t, strategyPath, strategy, "Grafana")
	requireContains(t, strategyPath, strategy, "W3C Trace Context")
	requireContains(t, strategyPath, strategy, "Production Beta Gaps")

	rows := serviceOperationRows(t, readinessPath, readiness)
	deployments := serviceDeploymentManifests(t)
	if len(rows) != len(deployments) {
		t.Fatalf("%s service operations row count = %d, want %d: %#v", readinessPath, len(rows), len(deployments), rows)
	}
	for _, deployment := range deployments {
		service := filepath.Base(filepath.Dir(filepath.Dir(deployment)))
		row, ok := rows[service]
		if !ok {
			t.Fatalf("%s does not contain an operations row for %s", readinessPath, service)
		}
		for _, marker := range []string{"SLO:", "Dashboard:", "Alerts:", "Runbook:", "Rollback:", "Synthetic:"} {
			requireContains(t, readinessPath+" "+service+" row", row, marker)
		}
	}
}

func TestProductionBetaObservabilityManifestsAreProvisioned(t *testing.T) {
	base := "../../deploy/observability/production-beta"
	kustomizationPath := base + "/kustomization.yaml"
	kustomization := readTextFile(t, kustomizationPath)
	requireContains(t, kustomizationPath, kustomization, "grafana-dashboard.yaml")
	requireContains(t, kustomizationPath, kustomization, "prometheus-rules.yaml")
	requireContains(t, kustomizationPath, kustomization, "synthetic-smoke.yaml")
	requireNotContains(t, kustomizationPath, kustomization, "../../kustomization.yaml")

	dashboardPath := base + "/grafana-dashboard.yaml"
	dashboard := readTextFile(t, dashboardPath)
	requireContains(t, dashboardPath, dashboard, "kind: ConfigMap")
	requireContains(t, dashboardPath, dashboard, `grafana_dashboard: "1"`)
	requireContains(t, dashboardPath, dashboard, "Core API availability")
	requireContains(t, dashboardPath, dashboard, "Deployment availability")
	requireContains(t, dashboardPath, dashboard, "Synthetic smoke failures")
	requireContains(t, dashboardPath, dashboard, "nexuspaas_http_requests_total")
	requireContains(t, dashboardPath, dashboard, "nexuspaas_http_request_duration_seconds_bucket")
	requireContains(t, dashboardPath, dashboard, "histogram_quantile(0.95")
	requireContains(t, dashboardPath, dashboard, "sum by (service, le)")
	requireContains(t, dashboardPath, dashboard, "kube_deployment_status_replicas_available")
	requireNotContains(t, dashboardPath, dashboard, "Mean request latency")
	requireNotContains(t, dashboardPath, dashboard, "mean latency")

	rulesPath := base + "/prometheus-rules.yaml"
	rules := readTextFile(t, rulesPath)
	requireContains(t, rulesPath, rules, "kind: PodMonitor")
	requireContains(t, rulesPath, rules, "kind: PrometheusRule")
	requireContains(t, rulesPath, rules, "path: /metrics")
	requireContains(t, rulesPath, rules, "portNumber: 8080")
	requireContains(t, rulesPath, rules, "authorization:")
	requireContains(t, rulesPath, rules, "type: Bearer")
	requireContains(t, rulesPath, rules, "credentials:")
	requireContains(t, rulesPath, rules, "name: nexuspaas-prometheus-scrape-secret")
	requireContains(t, rulesPath, rules, "key: bearer-token")
	requireNotContains(t, rulesPath, rules, "bearerTokenSecret")
	requireContains(t, rulesPath, rules, "NexusPaasCoreAvailabilityBurn")
	requireContains(t, rulesPath, rules, "NexusPaasHighReadP95Latency")
	requireContains(t, rulesPath, rules, "NexusPaasHighWriteP95Latency")
	requireContains(t, rulesPath, rules, "NexusPaasSyntheticSmokeFailed")
	requireContains(t, rulesPath, rules, "histogram_quantile(0.95")
	requireContains(t, rulesPath, rules, "sum by (service, le)")
	requireContains(t, rulesPath, rules, "nexuspaas_http_request_duration_seconds_bucket")
	requireNotContains(t, rulesPath, rules, "HighMean")
	requireNotContains(t, rulesPath, rules, "mean read latency")

	syntheticPath := base + "/synthetic-smoke.yaml"
	synthetic := readTextFile(t, syntheticPath)
	requireContains(t, syntheticPath, synthetic, "kind: CronJob")
	requireContains(t, syntheticPath, synthetic, "schedule: \"*/5 * * * *\"")
	requireContains(t, syntheticPath, synthetic, "automountServiceAccountToken: false")
	requireContains(t, syntheticPath, synthetic, "runAsNonRoot: true")
	requireContains(t, syntheticPath, synthetic, "runAsUser: 10001")
	requireContains(t, syntheticPath, synthetic, "allowPrivilegeEscalation: false")
	requireContains(t, syntheticPath, synthetic, "readOnlyRootFilesystem: true")
	requireContains(t, syntheticPath, synthetic, `drop: ["ALL"]`)
	requireContains(t, syntheticPath, synthetic, "name: nexuspaas-synthetic-smoke-secret")
	requireContains(t, syntheticPath, synthetic, "key: api-key")
	requireContains(t, syntheticPath, synthetic, "key: service-key")
	requireContains(t, syntheticPath, synthetic, "require_200 \"${service}\" /healthz")
	requireContains(t, syntheticPath, synthetic, "require_200 \"${service}\" /readyz")
	requireContains(t, syntheticPath, synthetic, "require_200 \"${service}\" /metrics")
	requireContains(t, syntheticPath, synthetic, "require_200 platform-gateway /openapi.json")
	requireContains(t, syntheticPath, synthetic, "require_200 platform-gateway /service-registry")

	readmePath := base + "/README.md"
	readme := readTextFile(t, readmePath)
	requireContains(t, readmePath, readme, "nexuspaas-prometheus-scrape-secret")
	requireContains(t, readmePath, readme, "nexuspaas-synthetic-smoke-secret")
	requireContains(t, readmePath, readme, "bearer-token")
	requireContains(t, readmePath, readme, "api-key")
	requireContains(t, readmePath, readme, "service-key")
	requireContains(t, readmePath, readme, "kubectl apply -k backend/deploy/observability/production-beta")
	requireContains(t, readmePath, readme, "expected 4xx")

	for _, unit := range productionBackendUnits() {
		requireContains(t, dashboardPath, dashboard, unit)
		requireContains(t, rulesPath, rules, unit)
		requireContains(t, syntheticPath, synthetic, unit)
	}
	for _, endpoint := range []string{
		"/api/v1/audit/logs",
		"/api/v1/permissions/policies",
		"/api/v1/ide",
		"/api/v1/users",
		"/api/v1/image-catalog",
		"/api/v1/admin/vpn",
		"/api/v1/resources",
		"/api/v1/uploads/images/nonexistent-smoke.png",
		"/api/v1/projects",
		"/api/v1/gateway/health",
		"/api/v1/forms",
		"/api/v1/plans",
		"/api/v1/storage/options",
		"/api/v1/admin/usage",
		"/api/v1/jobs",
	} {
		requireContains(t, syntheticPath, synthetic, endpoint)
	}

	readinessPath := "../../docs/operational-readiness.md"
	readiness := readTextFile(t, readinessPath)
	requireContains(t, readinessPath, readiness, "backend/deploy/observability/production-beta")
	requireContains(t, readinessPath, readiness, "nexuspaas-prometheus-scrape-secret")
	requireContains(t, readinessPath, readiness, "nexuspaas-synthetic-smoke-secret")

	strategyPath := "../../../docs/architecture/observability-strategy.md"
	strategy := readTextFile(t, strategyPath)
	requireContains(t, strategyPath, strategy, "backend/deploy/observability/production-beta")
	requireContains(t, strategyPath, strategy, "PodMonitor")
	requireContains(t, strategyPath, strategy, "PrometheusRule")
	requireContains(t, strategyPath, strategy, "CronJob")
}

func TestProductionBetaReleaseCandidateGateIsDocumented(t *testing.T) {
	scriptPath := "../../scripts/ci-security-gate.sh"
	script := readTextFile(t, scriptPath)
	requireContains(t, scriptPath, script, "sonar     SonarScanner Quality Gate")
	requireContains(t, scriptPath, script, "beta-rc   quick, production-beta render/dry-run/rollback rehearsal, docker/routing/collaboration smoke, security, Sonar, RC report")
	requireContains(t, scriptPath, script, "all       quick, docker, security, Sonar")
	requireContains(t, scriptPath, script, "beta-rc) run_beta_rc_gate")
	requireContains(t, scriptPath, script, "run_production_beta_manifest_rehearsal")
	requireContains(t, scriptPath, script, "run_runtime_smoke")
	requireContains(t, scriptPath, script, "run_compose_collaboration_smoke")
	requireContains(t, scriptPath, script, "kubectl kustomize backend")
	requireContains(t, scriptPath, script, "kubectl apply --dry-run=client --validate=false")
	requireContains(t, scriptPath, script, "production-beta-rollback-plan.sh")
	requireContains(t, scriptPath, script, "production-beta-redeploy-dry-run.txt")
	requireContains(t, scriptPath, script, "runtime-smoke.log")
	requireContains(t, scriptPath, script, "collaboration-smoke.log")
	requireContains(t, scriptPath, script, "collaboration-smoke-summary.md")
	requireContains(t, scriptPath, script, "audit-compliance-service|/api/v1/audit/logs")
	requireContains(t, scriptPath, script, "platform-gateway|/api/v1/gateway/health")
	requireContains(t, scriptPath, script, "workload-service|/api/v1/jobs")
	requireContains(t, scriptPath, script, "run_quick")
	requireContains(t, scriptPath, script, "run_docker_gate")
	requireContains(t, scriptPath, script, "run_security_gate")
	requireContains(t, scriptPath, script, "sonar) run_sonar_gate")
	requireContains(t, scriptPath, script, "sonar_scanner_install_complete")
	requireContains(t, scriptPath, script, "Discarding incomplete SonarScanner")
	requireContains(t, scriptPath, script, "FOCUSED_E2E_SKIP_PATTERN='^[[:space:]]*--- SKIP:|^SKIP[[:space:]]'")
	requireNotContains(t, scriptPath, script, "grep -Eiq 'SKIP|skipping'")
	requireContains(t, scriptPath, script, "CI_GATE_SONAR_REQUIRED")
	requireContains(t, scriptPath, script, "SONAR_TOKEN and SONAR_HOST_URL are required for this CI event")
	requireContains(t, scriptPath, script, "- Sonar Quality Gate: %s")
	requireContains(t, scriptPath, script, "External Production Beta traffic still requires a live staging rehearsal")

	workflowPath := "../../../.github/workflows/backend-quality-gate.yml"
	workflow := readTextFile(t, workflowPath)
	requireContains(t, workflowPath, workflow, "github.event_name != 'pull_request' || github.event.pull_request.head.repo.full_name == github.repository")
	requireContains(t, workflowPath, workflow, "FOCUSED_E2E_SKIP_PATTERN: '^[[:space:]]*--- SKIP:|^SKIP[[:space:]]'")
	requireContains(t, workflowPath, workflow, "go install \"golang.org/x/vuln/cmd/govulncheck@${GOVULNCHECK_VERSION}\"")
	requireNotContains(t, workflowPath, workflow, "\n  sonar:\n")
	requireNotContains(t, workflowPath, workflow, "Sonar Quality Gate")
	requireNotContains(t, workflowPath, workflow, "Require Sonar secrets")
	requireNotContains(t, workflowPath, workflow, "SONAR_TOKEN")
	requireNotContains(t, workflowPath, workflow, "SONAR_HOST_URL")
	requireNotContains(t, workflowPath, workflow, "needs.detect-changes.outputs.sonar")
	requireNotContains(t, workflowPath, workflow, "SonarSource/sonarqube-scan-action")
	requireNotContains(t, workflowPath, workflow, "-Dsonar.qualitygate.wait=true")
	requireNotContains(t, workflowPath, workflow, "\n      - sonar\n")
	requireNotContains(t, workflowPath, workflow, "echo \"sonar:")
	requireNotContains(t, workflowPath, workflow, "sonar_config")
	requireNotContains(t, workflowPath, workflow, "golang/govulncheck-action@v1")
	requireNotContains(t, workflowPath, workflow, "grep -Eiq 'SKIP|skipping'")
	requireNotContains(t, workflowPath, workflow, "Sonar skipped")

	readinessPath := "../../docs/beta-launch-readiness.md"
	readiness := readTextFile(t, readinessPath)
	requireContains(t, readinessPath, readiness, "bash backend/scripts/ci-security-gate.sh beta-rc")
	requireContains(t, readinessPath, readiness, "production-beta manifest rehearsal")
	requireContains(t, readinessPath, readiness, "non-live runtime smoke")
	requireContains(t, readinessPath, readiness, "8-unit collaboration smoke")
	requireContains(t, readinessPath, readiness, "one read-only endpoint per service")
	requireContains(t, readinessPath, readiness, "rollback command plan for every backend unit deployment")
	requireContains(t, readinessPath, readiness, "re-deploy client dry-run")
	requireContains(t, readinessPath, readiness, "Live Staging Rehearsal")
	requireContains(t, readinessPath, readiness, "All 8 backend units become ready.")
	requireContains(t, readinessPath, readiness, "GitHub Actions does not run SonarScanner")
	requireContains(t, readinessPath, readiness, "external required PR check")
	requireContains(t, readinessPath, readiness, "branch protection gate")
	requireContains(t, readinessPath, readiness, "workflow does not require `SONAR_TOKEN` or `SONAR_HOST_URL`")
	requireContains(t, readinessPath, readiness, "no unaccepted launch blockers")

	e2eDocsPath := "../../docs/e2e-testing.md"
	e2eDocs := readTextFile(t, e2eDocsPath)
	requireContains(t, e2eDocsPath, e2eDocs, "bash backend/scripts/ci-security-gate.sh beta-rc")
	requireContains(t, e2eDocsPath, e2eDocs, "renders `kubectl kustomize backend`")
	requireContains(t, e2eDocsPath, e2eDocs, "for every backend unit deployment")
	requireContains(t, e2eDocsPath, e2eDocs, "runtime smoke")
	requireContains(t, e2eDocsPath, e2eDocs, "re-deploy evidence")
	requireContains(t, e2eDocsPath, e2eDocs, "GitHub Actions does not run SonarScanner")
	requireContains(t, e2eDocsPath, e2eDocs, "external required PR check")
	requireContains(t, e2eDocsPath, e2eDocs, "branch protection gate")
	requireContains(t, e2eDocsPath, e2eDocs, "workflow does not require `SONAR_TOKEN` or `SONAR_HOST_URL`")
	requireContains(t, e2eDocsPath, e2eDocs, "local `sonar`")
	requireContains(t, e2eDocsPath, e2eDocs, "script subcommand remains available")
}

func TestProductionBetaLiveRehearsalHarnessIsGuarded(t *testing.T) {
	scriptPath := "../../scripts/production-beta-live-rehearsal.sh"
	script := readTextFile(t, scriptPath)
	requireContains(t, scriptPath, script, "set -Eeuo pipefail")
	requireNotContains(t, scriptPath, script, "set -x")
	requireContains(t, scriptPath, script, "LIVE_STAGING_REHEARSAL=1 is required before any live staging mutation")
	requireContains(t, scriptPath, script, "require_live_opt_in")
	requireContains(t, scriptPath, script, "require_env KUBE_CONTEXT")
	requireContains(t, scriptPath, script, "kubectl config current-context")
	requireContains(t, scriptPath, script, "kubectl --context \"${KUBE_CONTEXT}\" -n \"${NAMESPACE}\"")
	requireContains(t, scriptPath, script, "refusing local-style kube context")
	for _, marker := range []string{"docker-desktop", "localhost", "127.0.0.1", "loopback", "kind", "minikube"} {
		requireContains(t, scriptPath, script, marker)
	}

	requireContains(t, scriptPath, script, "require_candidate_image")
	requireContains(t, scriptPath, script, "CANDIDATE_IMAGE must be digest-pinned with @sha256:<64 lowercase hex digest>")
	requireContains(t, scriptPath, script, "grep -Eq '^[a-f0-9]{64}$'")
	requireContains(t, scriptPath, script, "reject_local_image_ref \"${CANDIDATE_IMAGE}\"")
	requireContains(t, scriptPath, script, "crane copy \"${SOURCE_IMAGE}\" \"${PROMOTED_IMAGE_TAG}\"")
	requireContains(t, scriptPath, script, "require_env PROMOTION_EVIDENCE")
	requireContains(t, scriptPath, script, "REGISTRY_SCAN_STATUS or REGISTRY_SCAN_EVIDENCE is required")

	requireContains(t, scriptPath, script, "kubectl kustomize backend")
	requireContains(t, scriptPath, script, "kubectl apply --dry-run=client --validate=false")
	requireContains(t, scriptPath, script, "production-beta render contains all-in-one platform Deployment")
	requireContains(t, scriptPath, script, "production-beta render contains dev references")
	requireContains(t, scriptPath, script, "rendered_deployment_names")
	requireContains(t, scriptPath, script, "rendered_has_deployment")
	for _, unit := range productionBackendUnits() {
		requireContains(t, scriptPath, script, unit)
	}

	requireContains(t, scriptPath, script, "required_secret_names")
	requireContains(t, scriptPath, script, "kctl get secret \"${secret}\" -o name")
	requireNotContains(t, scriptPath, script, "kubectl get secret")
	requireNotContains(t, scriptPath, script, "get secret \"${secret}\" -o yaml")
	requireNotContains(t, scriptPath, script, "get secret \"${secret}\" -o json")
	requireNotContains(t, scriptPath, script, "get secret \"${secret}\" -o jsonpath")
	requireNotContains(t, scriptPath, script, "jsonpath=.*data")
	requireNotContains(t, scriptPath, script, "base64")

	requireContains(t, scriptPath, script, "ADMIN_TASK")
	requireContains(t, scriptPath, script, "apply-migrations")
	requireContains(t, scriptPath, script, "validate-migrations")
	requireContains(t, scriptPath, script, "kctl wait --for=condition=complete --timeout=\"${JOB_TIMEOUT}\" \"job/${job_name}\"")
	requireContains(t, scriptPath, script, "kctl apply -f \"${RENDER_FILE}\"")
	requireContains(t, scriptPath, script, "kctl set image \"deployment/${unit}\" \"app=${CANDIDATE_IMAGE}\"")
	requireContains(t, scriptPath, script, "kctl rollout status \"deployment/${unit}\" --timeout=\"${ROLLOUT_TIMEOUT}\"")
	requireContains(t, scriptPath, script, "/healthz")
	requireContains(t, scriptPath, script, "/readyz")
	requireContains(t, scriptPath, script, "/metrics")
	requireContains(t, scriptPath, script, "/openapi.json")
	requireContains(t, scriptPath, script, "/service-registry")
	requireContains(t, scriptPath, script, "service-registry contains ${count} services, want 15")
	requireContains(t, scriptPath, script, "previous_image_for_unit")
	requireContains(t, scriptPath, script, "rollback_and_redeploy_each_unit")
	requireContains(t, scriptPath, script, "app=${previous_image}")
	requireContains(t, scriptPath, script, "app=${CANDIDATE_IMAGE}")

	requireContains(t, scriptPath, script, "Production Beta Live Rehearsal Report")
	for _, field := range []string{
		"Candidate image",
		"Candidate digest",
		"Promotion evidence",
		"Registry scan status",
		"Secret presence",
		"Migration Jobs",
		"Rollouts",
		"Smoke checks",
		"Rollback and redeploy",
		"close live P0.2-P0.5",
	} {
		requireContains(t, scriptPath, script, field)
	}

	readinessPath := "../../docs/beta-launch-readiness.md"
	readiness := readTextFile(t, readinessPath)
	requireContains(t, readinessPath, readiness, "operator-only harness")
	requireContains(t, readinessPath, readiness, "bash backend/scripts/production-beta-live-rehearsal.sh")
	requireContains(t, readinessPath, readiness, "LIVE_STAGING_REHEARSAL=1")
	requireContains(t, readinessPath, readiness, "KUBE_CONTEXT=<real-staging-context>")
	requireContains(t, readinessPath, readiness, "Docker Desktop, kind, minikube, localhost, loopback")
	requireContains(t, readinessPath, readiness, "Secret names only")
	requireContains(t, readinessPath, readiness, "rolls each unit back to its recorded previous image")
	requireContains(t, readinessPath, readiness, "production-beta-live-rehearsal-report.md")

	e2eDocsPath := "../../docs/e2e-testing.md"
	e2eDocs := readTextFile(t, e2eDocsPath)
	requireContains(t, e2eDocsPath, e2eDocs, "operator machine")
	requireContains(t, e2eDocsPath, e2eDocs, "staging context")
	requireContains(t, e2eDocsPath, e2eDocs, "bash backend/scripts/production-beta-live-rehearsal.sh")
	requireContains(t, e2eDocsPath, e2eDocs, "requires `kubectl config current-context` to match")
	requireContains(t, e2eDocsPath, e2eDocs, "records")
	requireContains(t, e2eDocsPath, e2eDocs, "only Secret name presence")
	requireContains(t, e2eDocsPath, e2eDocs, "per-unit")
	requireContains(t, e2eDocsPath, e2eDocs, "previous-image rollback plus candidate redeploy smoke")
}

func requireProductionDeploymentManifest(t *testing.T, path string) {
	t.Helper()
	service := filepath.Base(filepath.Dir(filepath.Dir(path)))
	body := readTextFile(t, path)
	requireDeploymentConfig(t, path, body, service)
	requireDeploymentScaling(t, path, body)
	requireDeploymentImages(t, path, body, service)
	requireDeploymentSecretContract(t, path, body, service)
	requireDeploymentSecurityContext(t, path, body)
	requireObjectStoreProvisioningContract(t, path, body, service)
}

func requireDeploymentConfig(t *testing.T, path, body, service string) {
	t.Helper()
	requireContains(t, path, body, "kind: ConfigMap")
	requireContains(t, path, body, fmt.Sprintf("name: %s-config", service))
	requireContains(t, path, body, fmt.Sprintf("SERVICE_NAME: %q", service))
	requireContains(t, path, body, `HTTP_ADDR: ":8080"`)
	requireContains(t, path, body, `REQUIRE_AUTH: "true"`)
	requireContains(t, path, body, `PRODUCTION: "true"`)
	requireContains(t, path, body, `APP_ENV: "production"`)
	requireContains(t, path, body, "LOG_LEVEL:")
	requireContains(t, path, body, "OTEL_EXPORTER_OTLP_ENDPOINT:")
	requireNotContains(t, path, body, "OTEL_EXPORTER_OTLP_ENDPOINT: \"http://")
	requireContains(t, path, body, fmt.Sprintf("OTEL_SERVICE_NAME: %q", service))
	requireContains(t, path, body, "OTEL_TRACES_SAMPLER:")
	requireContains(t, path, body, "OTEL_RESOURCE_ATTRIBUTES:")
	requireAuthorizationPolicySecretContract(t, path, body, service)
}

func requireDeploymentScaling(t *testing.T, path, body string) {
	t.Helper()
	requireContains(t, path, body, "replicas: 1")
	requireContains(t, path, body, "minReplicas: 1")
	requireContains(t, path, body, "maxReplicas: 4")
	requireNotContains(t, path, body, "replicas: 2")
	requireNotContains(t, path, body, "maxReplicas: 1")
	requireNotContains(t, path, body, "maxReplicas: 6")
}

func requireDeploymentImages(t *testing.T, path, body, service string) {
	t.Helper()
	images := imageValues(t, path, body)
	if service != mediaUploadServiceName && len(images) != 1 {
		t.Fatalf("%s image key count = %d, want 1: %#v", path, len(images), images)
	}
	for _, image := range images {
		if image != "nexuspaas-backend:v0.1.0" {
			t.Fatalf("%s image = %q, want nexuspaas-backend:v0.1.0", path, image)
		}
		if strings.Contains(image, ":latest") || image == fmt.Sprintf("%s:v0.1.0", service) {
			t.Fatalf("%s uses disallowed image value %q", path, image)
		}
	}
	requireContains(t, path, body, "imagePullPolicy: IfNotPresent")
}

func requireDeploymentSecretContract(t *testing.T, path, body, service string) {
	t.Helper()
	requireContains(t, path, body, "envFrom:")
	requireContains(t, path, body, "configMapRef:")
	requireContains(t, path, body, fmt.Sprintf("name: %s-config", service))
	requireContains(t, path, body, "secretRef:")
	requireContains(t, path, body, fmt.Sprintf("name: %s-runtime-secret", service))
	requireContains(t, path, body, "API_KEYS + API_KEY_PRINCIPALS or JWT_JWKS_URL + JWT_ISSUER + JWT_AUDIENCE")
	requireContains(t, path, body, "DATABASE_URL")
	requireContains(t, path, body, "REDIS_URL")
	requireContains(t, path, body, "EVENT_BUS_URL")
}

func requireDeploymentSecurityContext(t *testing.T, path, body string) {
	t.Helper()
	requireContains(t, path, body, "automountServiceAccountToken: false")
	requireContains(t, path, body, "runAsNonRoot: true")
	requireContains(t, path, body, "runAsUser: 10001")
	requireContains(t, path, body, "runAsGroup: 10001")
	requireContains(t, path, body, "fsGroup: 10001")
	requireContains(t, path, body, "seccompProfile:")
	requireContains(t, path, body, "type: RuntimeDefault")
	requireContains(t, path, body, "allowPrivilegeEscalation: false")
	requireContains(t, path, body, "readOnlyRootFilesystem: true")
	requireContains(t, path, body, "capabilities:")
	requireContains(t, path, body, `drop: ["ALL"]`)
	requireContains(t, path, body, "ephemeral-storage: 256Mi")
	requireContains(t, path, body, "ephemeral-storage: 1Gi")
}

func requireObjectStoreProvisioningContract(t *testing.T, path, body, service string) {
	t.Helper()
	if service == mediaUploadServiceName {
		requireContains(t, path, body, "name: ensure-object-store-bucket")
		requireContains(t, path, body, "ADMIN_TASK, value: ensure-object-store-bucket")
		requireContains(t, path, body, "OBJECT_STORE_SCHEME, value: \"http\"")
		requireContains(t, path, body, "OBJECT_STORE_URL, value: \"$(OBJECT_STORE_SCHEME)://minio:9000\"")
		requireNotContains(t, path, body, "http://minio:9000")
		return
	}
	requireNotContains(t, path, body, "\n          env:\n")
}

func requireAuthorizationPolicySecretContract(t *testing.T, path, body, service string) {
	t.Helper()
	requireNotContains(t, path, body, `AUTHORIZATION_POLICY_URL: "http://`)
	if service == "authorization-policy-service" {
		requireContains(t, path, body, "authorization-policy-service uses its local RawPolicyPDP")
		return
	}
	requireContains(t, path, body, "AUTHORIZATION_POLICY_URL + AUTHORIZATION_POLICY_API_KEY")
}

func readTextFile(t *testing.T, path string) string {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

func serviceDeploymentManifests(t *testing.T) []string {
	t.Helper()
	deployments, err := filepath.Glob("../../*/k8s/deployment.yaml")
	if err != nil {
		t.Fatal(err)
	}
	return deployments
}

func productionBackendUnits() []string {
	return []string{
		"platform-gateway",
		"iam-unit",
		"tenant-unit",
		"collaboration-unit",
		"platform-io-unit",
		"usage-observability",
		"compute-api",
		"compute-control-plane",
	}
}

func logicalServiceUnits() map[string]string {
	return map[string]string{
		"platform-gateway":             "platform-gateway",
		"identity-service":             "iam-unit",
		"authorization-policy-service": "iam-unit",
		"org-project-service":          "tenant-unit",
		"audit-compliance-service":     "collaboration-unit",
		"request-notification-service": "collaboration-unit",
		"media-upload-service":         "collaboration-unit",
		"storage-service":              "platform-io-unit",
		"image-registry-service":       "platform-io-unit",
		"integration-proxy-service":    "platform-io-unit",
		"usage-observability-service":  "usage-observability",
		"workload-service":             "compute-api",
		"ide-service":                  "compute-api",
		"scheduler-quota-service":      "compute-control-plane",
		"k8s-control-service":          "compute-control-plane",
	}
}

func productionBetaRuntimeSecretNames() []string {
	secrets := make([]string, 0, len(productionBackendUnits()))
	for _, unit := range productionBackendUnits() {
		secrets = append(secrets, unit+"-runtime-secret")
	}
	return secrets
}

func productionBetaRequiredSecretNames() []string {
	return append([]string{
		"postgres-password",
		"dex-password",
		"minio-credentials",
		"coturn-runtime-secret",
	}, productionBetaRuntimeSecretNames()...)
}

func productionBetaForbiddenSecretTerms() []string {
	return []string{
		"postgres-dev-password",
		"dex-dev-password",
		"minio-dev-credentials",
		"-dev-",
		"-test-",
		"-local-",
		"placeholder-secret",
		"sample-secret",
		"dummy-secret",
		"fake-secret",
		"test-secret",
		"local-secret",
		"change-me",
		"changeme",
	}
}

func productionBetaSecretSourcePaths(t *testing.T) []string {
	t.Helper()
	paths := []string{
		"../../kustomization.yaml",
		"../../scripts/production-beta-live-rehearsal.sh",
	}
	productionBetaFiles, err := filepath.Glob("../../deploy/k3s/production-beta/*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	paths = append(paths, productionBetaFiles...)
	return paths
}

func requireNoForbiddenProductionBetaSecretRefs(t *testing.T, path, body string) {
	t.Helper()
	for _, forbidden := range productionBetaForbiddenSecretTerms() {
		requireNotContains(t, path, body, forbidden)
	}
}

func requireSecretKeyRef(t *testing.T, path, name, key string) {
	t.Helper()
	body := readTextFile(t, path)
	requireContains(t, path, body, "secretKeyRef:")
	requireContains(t, path, body, "name: "+name)
	requireContains(t, path, body, "key: "+key)
}

func requireOnlyExpectedRuntimeSecretRefs(t *testing.T, path, body string, expected []string) {
	t.Helper()
	expectedSet := map[string]bool{}
	for _, secret := range expected {
		expectedSet[secret] = true
	}
	found := map[string]int{}
	for _, line := range strings.Split(body, "\n") {
		trimmed := strings.TrimSpace(line)
		const prefix = "- secretRef: {name: "
		if !strings.HasPrefix(trimmed, prefix) || !strings.HasSuffix(trimmed, "}") {
			continue
		}
		secret := strings.TrimSuffix(strings.TrimPrefix(trimmed, prefix), "}")
		if !expectedSet[secret] {
			t.Fatalf("%s contains unexpected runtime secretRef %q", path, secret)
		}
		found[secret]++
	}
	for _, secret := range expected {
		if found[secret] == 0 {
			t.Fatalf("%s missing runtime secretRef %q", path, secret)
		}
	}
	if len(found) != len(expected) {
		t.Fatalf("%s runtime secretRef unique count = %d, want %d: %#v", path, len(found), len(expected), found)
	}
}

func renderProductionBetaKustomization(t *testing.T) string {
	t.Helper()
	cmd := exec.Command("kubectl", "kustomize", "../..")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("kubectl kustomize backend failed: %v: %s", err, strings.TrimSpace(stderr.String()))
	}
	return string(output)
}

func withoutShellFunction(body, name string) string {
	lines := strings.Split(body, "\n")
	out := make([]string, 0, len(lines))
	skipping := false
	start := name + "() {"
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !skipping && trimmed == start {
			skipping = true
			continue
		}
		if skipping {
			if trimmed == "}" {
				skipping = false
			}
			continue
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}

func extractServiceURLs(t *testing.T, path, body string) map[string]string {
	t.Helper()
	const marker = "SERVICE_URLS: >-\n"
	start := strings.Index(body, marker)
	if start == -1 {
		t.Fatalf("%s does not contain SERVICE_URLS block", path)
	}
	line := body[start+len(marker):]
	line = strings.TrimSpace(strings.SplitN(line, "\n", 2)[0])
	urls := map[string]string{}
	if err := json.Unmarshal([]byte(line), &urls); err != nil {
		t.Fatalf("%s SERVICE_URLS is not valid JSON: %v", path, err)
	}
	return urls
}

func serviceOperationRows(t *testing.T, path, body string) map[string]string {
	t.Helper()
	rows := map[string]string{}
	for _, line := range strings.Split(body, "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "| ") || !strings.Contains(trimmed, " SLO:") || !strings.Contains(trimmed, " Synthetic:") {
			continue
		}
		parts := strings.Split(trimmed, "|")
		if len(parts) < 5 {
			t.Fatalf("%s malformed service operations row: %s", path, trimmed)
		}
		service := strings.TrimSpace(parts[1])
		if _, exists := rows[service]; exists {
			t.Fatalf("%s contains a duplicate service operations row for %s", path, service)
		}
		rows[service] = trimmed
	}
	return rows
}

func requireFileExists(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
}

func requireContains(t *testing.T, path, body, want string) {
	t.Helper()
	if !strings.Contains(body, want) {
		t.Fatalf("%s does not contain %q", path, want)
	}
}

func requireNotContains(t *testing.T, path, body, unwanted string) {
	t.Helper()
	if strings.Contains(body, unwanted) {
		t.Fatalf("%s contains %q", path, unwanted)
	}
}

func imageValues(t *testing.T, path, body string) []string {
	t.Helper()
	values := []string{}
	for _, line := range strings.Split(body, "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "image:") {
			continue
		}
		values = append(values, strings.TrimSpace(strings.TrimPrefix(trimmed, "image:")))
	}
	if len(values) == 0 {
		t.Fatalf("%s does not contain any image keys", path)
	}
	return values
}
