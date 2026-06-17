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
