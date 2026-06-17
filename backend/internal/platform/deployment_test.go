package platform

import (
	"encoding/json"
	"fmt"
	"os"
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

	requireContains(t, "../../.dockerignore", readTextFile(t, "../../.dockerignore"), "*-service/")

	sharedDockerfile := readTextFile(t, "../../Dockerfile")
	requireContains(t, "../../Dockerfile", sharedDockerfile, "COPY go.mod go.sum ./")
	requireContains(t, "../../Dockerfile", sharedDockerfile, "RUN go mod download")
	requireContains(t, "../../Dockerfile", sharedDockerfile, "COPY cmd ./cmd")
	requireContains(t, "../../Dockerfile", sharedDockerfile, "COPY internal ./internal")
	requireContains(t, "../../Dockerfile", sharedDockerfile, "go build -trimpath -o /out/microservice ./cmd/microservice")
	requireContains(t, "../../Dockerfile", sharedDockerfile, "FROM alpine:3.22")
	requireContains(t, "../../Dockerfile", sharedDockerfile, "RUN apk add --no-cache --upgrade ca-certificates libcrypto3 libssl3 \\")
	requireContains(t, "../../Dockerfile", sharedDockerfile, "&& addgroup -S -g 10001 app \\")
	requireContains(t, "../../Dockerfile", sharedDockerfile, "&& adduser -S -D -H -u 10001 -G app app")
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

func TestProductionBetaKustomizationIncludesFifteenServices(t *testing.T) {
	path := "../../kustomization.yaml"
	body := readTextFile(t, path)
	requireContains(t, path, body, "deploy/k3s/production-beta/runtime-config.yaml")
	requireContains(t, path, body, "deploy/k3s/production-beta/runtime-secret-contract.yaml")
	requireContains(t, path, body, "deploy/k3s/production-beta/runtime-config-envfrom-patch.yaml")
	requireContains(t, path, body, "deploy/k3s/postgres.yaml")
	requireContains(t, path, body, "deploy/k3s/redis.yaml")
	requireContains(t, path, body, "deploy/k3s/minio.yaml")
	requireContains(t, path, body, "deploy/k3s/dex.yaml")
	requireContains(t, path, body, "deploy/k3s/production-beta/backing-secret-names.yaml")
	requireNotContains(t, path, body, "deploy/k3s/platform.yaml")

	deployments := serviceDeploymentManifests(t)
	for _, deployment := range deployments {
		service := filepath.Base(filepath.Dir(filepath.Dir(deployment)))
		resource := fmt.Sprintf("%s/k8s/deployment.yaml", service)
		requireContains(t, path, body, resource)
	}
	if got := strings.Count(body, "/k8s/deployment.yaml"); got != 15 {
		t.Fatalf("%s service deployment resource count = %d, want 15", path, got)
	}
}

func TestProductionBetaRuntimeConfigAndSecretContract(t *testing.T) {
	configPath := "../../deploy/k3s/production-beta/runtime-config.yaml"
	config := readTextFile(t, configPath)
	requireContains(t, configPath, config, "name: production-beta-runtime-config")
	requireContains(t, configPath, config, `REDIS_URL: "redis://redis:6379/0"`)
	requireContains(t, configPath, config, `EVENT_BUS_URL: "redis://redis:6379/1"`)
	requireContains(t, configPath, config, `JWT_AUDIENCE: "platform"`)
	serviceURLs := extractServiceURLs(t, configPath, config)

	contractPath := "../../deploy/k3s/production-beta/runtime-secret-contract.yaml"
	contract := readTextFile(t, contractPath)
	requireContains(t, contractPath, contract, "name: production-beta-runtime-secret-contract")
	requireContains(t, contractPath, contract, "`SERVICE_API_KEY`")
	requireContains(t, contractPath, contract, "`AUTHORIZATION_POLICY_URL`")
	requireContains(t, contractPath, contract, "`AUTHORIZATION_POLICY_API_KEY`")
	requireContains(t, contractPath, contract, "`postgres-password`")
	requireContains(t, contractPath, contract, "`dex-password`")
	requireContains(t, contractPath, contract, "`minio-credentials`")
	requireNotContains(t, contractPath, contract, "-dev-")

	backingPatchPath := "../../deploy/k3s/production-beta/backing-secret-names.yaml"
	backingPatch := readTextFile(t, backingPatchPath)
	requireContains(t, backingPatchPath, backingPatch, "name: postgres-password")
	requireContains(t, backingPatchPath, backingPatch, "name: dex-password")
	requireContains(t, backingPatchPath, backingPatch, "name: minio-credentials")
	requireNotContains(t, backingPatchPath, backingPatch, "-dev-")

	runtimePatchPath := "../../deploy/k3s/production-beta/runtime-config-envfrom-patch.yaml"
	runtimePatch := readTextFile(t, runtimePatchPath)
	requireContains(t, runtimePatchPath, runtimePatch, "path: /spec/template/spec/containers/0/envFrom/1")
	requireContains(t, runtimePatchPath, runtimePatch, "name: production-beta-runtime-config")
	requireContains(t, runtimePatchPath, runtimePatch, "optional: true")

	kustomizationPath := "../../kustomization.yaml"
	kustomization := readTextFile(t, kustomizationPath)
	requireContains(t, kustomizationPath, kustomization, "labelSelector: app in (")

	for _, deployment := range serviceDeploymentManifests(t) {
		service := filepath.Base(filepath.Dir(filepath.Dir(deployment)))
		if got := serviceURLs[service]; got != "http://"+service {
			t.Fatalf("%s SERVICE_URLS[%s] = %q, want http://%s", configPath, service, got, service)
		}
		requireContains(t, contractPath, contract, fmt.Sprintf("`%s-runtime-secret`", service))
		requireContains(t, kustomizationPath, kustomization, service)
	}
	if len(serviceURLs) != 15 {
		t.Fatalf("%s SERVICE_URLS count = %d, want 15", configPath, len(serviceURLs))
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
	requireContains(t, readinessPath, readiness, "all 15 services")
	requireContains(t, readinessPath, readiness, "kubectl rollout undo deployment/<service>")
	requireContains(t, nfrPath, nfrs, "docs/operational-readiness.md")
	requireContains(t, nfrPath, nfrs, "../../docs/architecture/observability-strategy.md")

	strategyPath := "../../../docs/architecture/observability-strategy.md"
	strategy := readTextFile(t, strategyPath)
	requireContains(t, strategyPath, strategy, "15 independently deployed backend services")
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
	requireContains(t, dashboardPath, dashboard, "nexuspaas_http_request_duration_seconds_sum")
	requireContains(t, dashboardPath, dashboard, "kube_deployment_status_replicas_available")

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
	requireContains(t, rulesPath, rules, "NexusPaasSyntheticSmokeFailed")

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

	for _, deployment := range serviceDeploymentManifests(t) {
		service := filepath.Base(filepath.Dir(filepath.Dir(deployment)))
		requireContains(t, dashboardPath, dashboard, service)
		requireContains(t, rulesPath, rules, service)
		requireContains(t, syntheticPath, synthetic, service)
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
	requireContains(t, scriptPath, script, "beta-rc   quick, production-beta render/dry-run/rollback rehearsal, docker/runtime smoke, security, sonar, RC report")
	requireContains(t, scriptPath, script, "beta-rc) run_beta_rc_gate")
	requireContains(t, scriptPath, script, "run_production_beta_manifest_rehearsal")
	requireContains(t, scriptPath, script, "run_runtime_smoke")
	requireContains(t, scriptPath, script, "kubectl kustomize backend")
	requireContains(t, scriptPath, script, "kubectl apply --dry-run=client --validate=false")
	requireContains(t, scriptPath, script, "production-beta-rollback-plan.sh")
	requireContains(t, scriptPath, script, "production-beta-redeploy-dry-run.txt")
	requireContains(t, scriptPath, script, "runtime-smoke.log")
	requireContains(t, scriptPath, script, "audit-compliance-service|/api/v1/audit/logs")
	requireContains(t, scriptPath, script, "platform-gateway|/api/v1/gateway/health")
	requireContains(t, scriptPath, script, "workload-service|/api/v1/jobs")
	requireContains(t, scriptPath, script, "run_quick")
	requireContains(t, scriptPath, script, "run_docker_gate")
	requireContains(t, scriptPath, script, "run_security_gate")
	requireContains(t, scriptPath, script, "run_sonar_gate")
	requireContains(t, scriptPath, script, "External Production Beta traffic still requires a live staging rehearsal")

	readinessPath := "../../docs/beta-launch-readiness.md"
	readiness := readTextFile(t, readinessPath)
	requireContains(t, readinessPath, readiness, "bash backend/scripts/ci-security-gate.sh beta-rc")
	requireContains(t, readinessPath, readiness, "production-beta manifest rehearsal")
	requireContains(t, readinessPath, readiness, "non-live runtime smoke")
	requireContains(t, readinessPath, readiness, "one read-only endpoint per service")
	requireContains(t, readinessPath, readiness, "rollback command plan for every service deployment")
	requireContains(t, readinessPath, readiness, "re-deploy client dry-run")
	requireContains(t, readinessPath, readiness, "Live Staging Rehearsal")
	requireContains(t, readinessPath, readiness, "All 15 services become ready.")
	requireContains(t, readinessPath, readiness, "no unaccepted launch blockers")

	e2eDocsPath := "../../docs/e2e-testing.md"
	e2eDocs := readTextFile(t, e2eDocsPath)
	requireContains(t, e2eDocsPath, e2eDocs, "bash backend/scripts/ci-security-gate.sh beta-rc")
	requireContains(t, e2eDocsPath, e2eDocs, "renders `kubectl kustomize backend`")
	requireContains(t, e2eDocsPath, e2eDocs, "writes rollback commands for every service")
	requireContains(t, e2eDocsPath, e2eDocs, "runtime smoke")
	requireContains(t, e2eDocsPath, e2eDocs, "re-deploy evidence")
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
