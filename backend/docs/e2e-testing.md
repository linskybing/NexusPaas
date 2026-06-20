# Cross-Service E2E Testing

This runbook covers the `e2e` build-tagged tests in `internal/e2e`. The suite
starts isolated service instances in-process, gives each instance a local HTTP
listener, wires the full set of started peer URLs into each service's
`SERVICE_URLS`, and verifies cross-service data movement through real HTTP,
Postgres, Redis event streams, and MinIO.

## Scope

- Contract checks for service route isolation, the documented route ownership
  matrix, and bidirectional event catalog consistency with
  `docs/event-contracts.md`.
- Runtime-isolation checks for service-owned maintenance tasks and custom
  handler registration in isolated `SERVICE_NAME` instances.
- Critical journeys for identity internal read/auth, workload to scheduler
  admission, scheduler quota events, media upload object bytes, and
  request-notification audit events.
- A non-blob service isolation regression proves stale or broken
  `OBJECT_STORE_*` values do not make isolated services depend on MinIO.
- The workload to scheduler admission journey asserts real HTTP service-key
  authentication, scheduler response/event payload correctness, workload job
  persistence, and scheduler event metadata without requiring production
  internal-client behavior changes.
- The scheduler owner-read journey wires `scheduler-quota-service` to
  `org-project-service` and `workload-service` through `SERVICE_URLS`, proves
  workload usage is read over owner HTTP contracts, and proves a wrong outbound
  service key fails closed even when the shared local database contains records.
- The storage mount-plan journey runs a real isolated storage HTTP service and
  an isolated workload app with a fake Kubernetes client, proves workload
  dispatch consumes storage-owned PVC source/target data through
  `SERVICE_URLS["storage-service"]`, and proves a wrong service key fails closed
  without storage materialization.
- Core user-journey coverage includes ConfigFile lifecycle plus
  `config_commit_id` job submit/readback guards, image request/build governance,
  and IDE project-access lifecycle.
- Optional later-slice E2E tests, such as GPU telemetry and scheduler
  preemption, live in the same package but are not part of the required
  cross-service gate below.
- Dex/OIDC is not required for this suite.

## Local Kubernetes Context

Feature acceptance runs against an RKE2 cluster (Kubernetes >= 1.34). Export its
kubeconfig before running cluster tests; the cluster client reads `KUBECONFIG`,
then in-cluster config, then `~/.kube/config`:

```sh
sudo cp /etc/rancher/rke2/rke2.yaml ~/.kube/config
sudo chown "$(id -u):$(id -g)" ~/.kube/config   # or: export KUBECONFIG=/etc/rancher/rke2/rke2.yaml
```

The runtime-isolation E2E does not create, update, or delete Kubernetes objects,
but keep the cluster context healthy before running the gate:

```sh
kubectl config current-context
kubectl cluster-info
kubectl get nodes
```

### DRA readiness precondition

`TestLiveK8sConfigFileDRADispatchE2E` (below) and browser GPU streaming dispatch
require Dynamic Resource Allocation plus the NVIDIA DRA driver. The live test
**skips** (it never fails) when these are absent, so confirm the cluster — not
the code — is ready first:

```sh
kubectl api-resources | grep -E 'resourceclaimtemplates|deviceclasses'  # resource.k8s.io/v1 served
kubectl get deviceclasses                                               # must list gpu.nvidia.com
kubectl get pods -A | grep -i dra                                       # NVIDIA DRA driver running
```

The dispatch code expects driver `gpu.nvidia.com` and MPS opaque config
`resource.nvidia.com/v1beta1`
(`internal/services/workload/dispatcher_dra.go`). On Kubernetes >= 1.34 the
`resource.k8s.io/v1` APIs are GA and on by default; the DeviceClass appears only
after the NVIDIA DRA driver is installed.

## Start Local Backing Services

```sh
DEX_STATIC_PASSWORD_HASH='unused-for-e2e' \
  docker compose -f /home/lin/Desktop/NexusPaas/backend/deploy/local/docker-compose.yml up -d postgres redis minio
```

Use a dedicated local Redis DB/container and the dedicated MinIO bucket below
when possible. The harness removes only Redis keys, Redis stream entries, Redis
inbox members, Postgres records, and MinIO objects that contain the E2E run ID;
it does not clear shared Redis streams or reset broad Postgres service
sequences.

The dummy Dex variable is only needed because Compose interpolates every service
definition before selecting the requested services. Dex is not started by this
command and is not part of the E2E gate.

If `localhost:9000` is already in use, start a temporary MinIO on alternate
ports and point `TEST_OBJECT_STORE_URL` at it:

```sh
docker start nexuspaas-minio-e2e || docker run -d --name nexuspaas-minio-e2e \
  -p 19000:9000 -p 19001:9001 \
  -e MINIO_ROOT_USER=nexuspaas \
  -e MINIO_ROOT_PASSWORD=nexuspaas-secret \
  minio/minio:RELEASE.2025-04-08T15-41-24Z \
  server /data --console-address :9001

export TEST_OBJECT_STORE_URL='http://localhost:19000'
```

## Environment

```sh
export TEST_POSTGRES_PASSWORD='<local dev password>'
export TEST_DATABASE_URL="postgres://nexuspaas:${TEST_POSTGRES_PASSWORD}@localhost:5432/nexuspaas?sslmode=disable"
export TEST_REDIS_URL='redis://localhost:6379/0'
export TEST_EVENT_BUS_URL="$TEST_REDIS_URL"
export TEST_OBJECT_STORE_URL='http://localhost:9000' # or http://localhost:19000 when using the fallback container
export TEST_OBJECT_STORE_ACCESS_KEY='nexuspaas'
export TEST_OBJECT_STORE_SECRET_KEY='nexuspaas-secret'
export TEST_OBJECT_STORE_BUCKET='media-e2e'
```

Provision the media bucket explicitly before serving processes start. The E2E
harness also runs this idempotent provisioning path, but this command mirrors
the deployment admin process:

```sh
cd /home/lin/Desktop/NexusPaas/backend
SERVICE_NAME=media-upload-service \
PRODUCTION=false \
REQUIRE_AUTH=false \
DEV_HEADER_AUTH=true \
OBJECT_STORE_URL="$TEST_OBJECT_STORE_URL" \
OBJECT_STORE_ACCESS_KEY="$TEST_OBJECT_STORE_ACCESS_KEY" \
OBJECT_STORE_SECRET_KEY="$TEST_OBJECT_STORE_SECRET_KEY" \
OBJECT_STORE_BUCKET="$TEST_OBJECT_STORE_BUCKET" \
ADMIN_TASK=ensure-object-store-bucket \
go run ./cmd/microservice
```

Only `media-upload-service` and co-hosted `SERVICE_NAME=all` receive
object-store config. Isolated non-blob services must stay ready when their real
Postgres, Redis, and event-bus dependencies are healthy, even if stale
`OBJECT_STORE_*` values exist in shared developer shells.

Optional test-only overrides:

```sh
export E2E_API_KEY='e2e-admin-key'
export E2E_SERVICE_API_KEY='e2e-service-key'
export E2E_RUN_ID='e2e-local'
```

## Run

The required cross-service gate is the focused command below. It must show
explicit `PASS` lines for the required tests and must not pass by skipping due to
missing backing-service configuration:

```sh
cd /home/lin/Desktop/NexusPaas/backend
set -o pipefail
go test -tags e2e ./internal/e2e \
  -run 'TestServiceRouteIsolationContract|TestServiceIsolationValidationE2E|TestIsolatedRuntimeRegistrationE2E|TestProviderConsumerContractMatrix|TestCriticalCrossServiceJourneys|TestSchedulerAdmissionOwnerReadContractsE2E|TestNonBlobIsolatedServiceIgnoresObjectStoreConfigE2E|TestStorageMountPlanContractE2E|TestImageBuildGovernanceE2E|TestIDELifecycleProjectAccessE2E|TestWorkloadConfigFileLifecycleE2E' \
  -count=1 -v | tee /tmp/cross-service-e2e.log
rg '^--- PASS: Test(ServiceRouteIsolationContract|ServiceIsolationValidationE2E|IsolatedRuntimeRegistrationE2E|ProviderConsumerContractMatrix|CriticalCrossServiceJourneys|SchedulerAdmissionOwnerReadContractsE2E|NonBlobIsolatedServiceIgnoresObjectStoreConfigE2E|StorageMountPlanContractE2E|ImageBuildGovernanceE2E|IDELifecycleProjectAccessE2E|WorkloadConfigFileLifecycleE2E)' /tmp/cross-service-e2e.log
! rg 'SKIP|skipping|Skipping' /tmp/cross-service-e2e.log
```

Run the full E2E package after the focused gate when optional slice tests are
enabled or expected to skip:

```sh
cd /home/lin/Desktop/NexusPaas/backend
go test -tags e2e ./internal/e2e -count=1 -v
```

Run the focused runtime-isolation E2E while working on service ownership:

```sh
cd /home/lin/Desktop/NexusPaas/backend
go test -tags e2e ./internal/e2e -run 'TestIsolatedRuntimeRegistrationE2E' -count=1 -v
```

Run the focused scheduler admission owner-read E2E while working on
`scheduler-quota-service` admission dependencies:

```sh
cd /home/lin/Desktop/NexusPaas/backend
go test -tags e2e ./internal/e2e -run '^TestSchedulerAdmissionOwnerReadContractsE2E$' -count=1 -v
```

Run the focused storage mount-plan contract E2E while working on
workload-to-storage dispatch dependencies. This test requires local Postgres,
Redis, and MinIO envs, uses a fake Kubernetes client, and must not skip:

```sh
cd /home/lin/Desktop/NexusPaas/backend
set -o pipefail
go test -tags e2e ./internal/e2e -run '^TestStorageMountPlanContractE2E$' -count=1 -v \
  | tee /tmp/storage-mount-plan-e2e.log
rg '^--- PASS: TestStorageMountPlanContractE2E' /tmp/storage-mount-plan-e2e.log
! rg 'SKIP|skipping|Skipping' /tmp/storage-mount-plan-e2e.log
```

Run the focused core user-journey E2E while working on workload ConfigFiles,
image build governance, or IDE lifecycle. These tests require local Postgres,
Redis, and MinIO envs and must not skip:

```sh
cd /home/lin/Desktop/NexusPaas/backend
go test -tags e2e ./internal/e2e \
  -run 'TestImageBuildGovernanceE2E|TestIDELifecycleProjectAccessE2E|TestWorkloadConfigFileLifecycleE2E' \
  -count=1 -v
```

Run the focused GPU telemetry E2E while working on usage-observability workers:

```sh
cd /home/lin/Desktop/NexusPaas/backend
go test -tags e2e ./internal/e2e -run 'TestGPUUsageTelemetryCollectorE2E' -count=1 -v
```

Run the focused Longhorn RWX health worker E2E while working on storage worker
parity. This fake-client E2E is required and must not skip:

```sh
cd /home/lin/Desktop/NexusPaas/backend
go test -tags e2e ./internal/e2e -run '^TestLonghornRWXHealthWorkerE2E$' -count=1 -v
```

Run the focused scheduler priority-class sync worker E2E while working on
scheduler-quota worker parity. This fake-client E2E is required and must not
skip; it proves create, managed update, managed recreate, unmanaged conflict
reporting, summary persistence, and event publication through the maintenance
runtime:

```sh
cd /home/lin/Desktop/NexusPaas/backend
go test -tags e2e ./internal/e2e -run '^TestPriorityClassSyncWorkerE2E$' -count=1 -v
```

Run the live Kubernetes priority-class sync E2E only when the current local
RKE2 cluster may be mutated. The test creates unique
cluster-scoped `nexuspaas-e2e-priority-*` objects, labels them with an E2E run marker,
deletes only those uniquely named/labeled objects, and reports leftover names if
cleanup fails:

```sh
cd /home/lin/Desktop/NexusPaas/backend
set -o pipefail
TEST_LIVE_K8S_PRIORITY_CLASS_SYNC=1 \
  go test -tags e2e ./internal/e2e -run '^TestPriorityClassSyncWorkerLiveK8sE2E$' -count=1 -v \
  | tee /tmp/priority-class-sync-e2e.log
rg '^--- PASS: TestPriorityClassSyncWorkerLiveK8sE2E' /tmp/priority-class-sync-e2e.log
! rg 'SKIP|skipping|Skipping' /tmp/priority-class-sync-e2e.log
```

Run the focused Docker image cleanup CronJob provisioner E2E while working on
k8s-control worker parity. This fake-client E2E is required and must not skip;
it proves create, managed drift update, unmanaged conflict no-mutation, and
explicit `automountServiceAccountToken=false` without mutating a live cluster or
running Docker prune:

```sh
cd /home/lin/Desktop/NexusPaas/backend
go test -tags e2e ./internal/e2e -run '^TestDockerImageCleanupCronJobProvisionerE2E$' -count=1 -v
```

Run the live Kubernetes Docker cleanup CronJob E2E only when the current local
RKE2 cluster may be mutated. The test creates a temporary
namespace, creates only the `docker-image-cleanup` CronJob, verifies the pod
template, and deletes only that E2E namespace/CronJob. The CronJob is privileged
and mounts `/var/run/docker.sock`, but this test does not manually create a Job
or execute `docker system prune`:

```sh
cd /home/lin/Desktop/NexusPaas/backend
set -o pipefail
TEST_LIVE_K8S_DOCKER_CLEANUP=1 \
  go test -tags e2e ./internal/e2e -run '^TestDockerImageCleanupCronJobLiveK8sE2E$' -count=1 -v \
  | tee /tmp/docker-cleanup-e2e.log
rg '^--- PASS: TestDockerImageCleanupCronJobLiveK8sE2E' /tmp/docker-cleanup-e2e.log
! rg 'SKIP|skipping|Skipping' /tmp/docker-cleanup-e2e.log
```

Run the live Kubernetes Longhorn RWX smoke while working on storage worker
readiness. This opt-in smoke performs no mutations and keeps auto repair
disabled; if the current cluster lacks the Longhorn CRD or RBAC, the expected
result is a persisted degraded/error summary rather than a healthy empty
success:

```sh
cd /home/lin/Desktop/NexusPaas/backend
set -o pipefail
TEST_LIVE_K8S_LONGHORN_RWX_SMOKE=1 \
  go test -tags e2e ./internal/e2e -run '^TestLonghornRWXHealthWorkerLiveK8sSmokeE2E$' -count=1 -v \
  | tee /tmp/longhorn-rwx-smoke-e2e.log
rg '^--- PASS: TestLonghornRWXHealthWorkerLiveK8sSmokeE2E' /tmp/longhorn-rwx-smoke-e2e.log
! rg 'SKIP|skipping|Skipping' /tmp/longhorn-rwx-smoke-e2e.log
```

Run the optional real-Longhorn gate only when the current cluster is expected to
have Longhorn installed and accessible:

```sh
cd /home/lin/Desktop/NexusPaas/backend
TEST_LIVE_LONGHORN_RWX=1 \
  go test -tags e2e ./internal/e2e -run '^TestLonghornRWXHealthWorkerLiveLonghornE2E$' -count=1 -v
```

Run the live OpenLDAP identity strategy and mirror-sync E2E while working on
identity LDAP parity. This command is opt-in because it needs a dedicated local
LDAP container; for acceptance, the log must show the test passed and did not
skip:

```sh
docker rm -f nexuspaas-openldap-e2e 2>/dev/null || true
docker run -d --name nexuspaas-openldap-e2e \
  -p 1389:1389 -p 1636:1636 \
  -e LDAP_ROOT=dc=example,dc=org \
  -e LDAP_ADMIN_USERNAME=admin \
  -e LDAP_ADMIN_PASSWORD=adminpassword \
  -e LDAP_USERS=ldapalice \
  -e LDAP_PASSWORDS=ldappass \
  bitnamilegacy/openldap:2.6.10

export LDAP_ENABLED='true'
export LDAP_HOST='localhost'
export LDAP_PORT='1389'
export LDAP_USE_TLS='false'
export LDAP_BIND_DN='cn=admin,dc=example,dc=org'
export LDAP_BIND_PASSWORD='adminpassword'
export LDAP_USER_SEARCH_BASE='ou=users,dc=example,dc=org'
export LDAP_USER_FILTER='(uid=%s)'
export LDAP_MIRROR_SYNC_INTERVAL='5m'
export TEST_LDAP_USER='ldapalice'
export TEST_LDAP_PASSWORD='ldappass'

cd /home/lin/Desktop/NexusPaas/backend
set -o pipefail
TEST_LIVE_LDAP_IDENTITY=1 \
  go test -tags e2e ./internal/e2e -run '^TestIdentityLDAPStrategyMirrorSyncE2E$' -count=1 -v \
  | tee /tmp/identity-ldap-e2e.log
rg '^--- PASS: TestIdentityLDAPStrategyMirrorSyncE2E' /tmp/identity-ldap-e2e.log
! rg 'SKIP|skipping|Skipping' /tmp/identity-ldap-e2e.log
```

Run the live LDAP + project plan + Kubernetes deploy E2E while working on the
end-to-end user journey from identity registration through workload dispatch.
This opt-in test needs local Postgres, Redis, MinIO, the OpenLDAP container
above, and a mutable RKE2 Kubernetes context. When the flag is
set, missing LDAP, backing-service, or Kubernetes dependencies fail the test.
The test creates only E2E-marked records plus one unique `proj-<project>-<user>`
namespace, submits a minimal `batch/v1 Job`, verifies the Job object and
workload record, and deletes that namespace during cleanup:

```sh
cd /home/lin/Desktop/NexusPaas/backend
set -o pipefail
TEST_LIVE_K8S_USER_PROJECT_PLAN_DEPLOY=1 \
  go test -tags e2e ./internal/e2e -run '^TestLiveLDAPUserProjectPlanConfigDeployE2E$' -count=1 -v \
  | tee /tmp/live-user-project-plan-deploy-e2e.log
rg '^--- PASS: TestLiveLDAPUserProjectPlanConfigDeployE2E' /tmp/live-user-project-plan-deploy-e2e.log
! rg 'SKIP|skipping|Skipping' /tmp/live-user-project-plan-deploy-e2e.log
```

Run the live Kubernetes queue-duration, plan-window, and auto-preemption E2E
independently from LDAP. This opt-in test uses the current kubeconfig, creates
only unique `proj-<project>-<user>` namespaces/resources, verifies native
`activeDeadlineSeconds` for Jobs, Deployment runtime labels plus controller
deletion, plan-window eviction, and logical-quota auto-preemption:

```sh
cd /home/lin/Desktop/NexusPaas/backend
set -o pipefail
TEST_LIVE_K8S_PLAN_WINDOW_DURATION_PREEMPTION=1 \
  go test -tags e2e ./internal/e2e -run '^TestLiveK8sPlanWindowDurationPreemptionE2E$' -count=1 -v \
  | tee /tmp/live-plan-window-duration-preemption-e2e.log
rg '^--- PASS: TestLiveK8sPlanWindowDurationPreemptionE2E' /tmp/live-plan-window-duration-preemption-e2e.log
! rg 'SKIP|skipping|Skipping' /tmp/live-plan-window-duration-preemption-e2e.log
```

Run the live Kubernetes ConfigFile DRA dispatch E2E only when the current
cluster exposes `resource.k8s.io/v1` ResourceClaimTemplate APIs and has a DRA
DeviceClass/resource driver installed. This opt-in test creates one unique
project namespace, submits a ConfigFile-backed DRA Pod job with
`gpu_count`, `sm_percentage`, `pinned_memory_limit`, and `device_class_name`,
verifies ResourceClaimTemplate and Pod DRA wiring, then deletes only that
namespace:

```sh
cd /home/lin/Desktop/NexusPaas/backend
set -o pipefail
TEST_LIVE_K8S_CONFIGFILE_DRA=1 \
  go test -tags e2e ./internal/e2e -run '^TestLiveK8sConfigFileDRADispatchE2E$' -count=1 -v \
  | tee /tmp/live-configfile-dra-e2e.log
rg '^--- PASS: TestLiveK8sConfigFileDRADispatchE2E' /tmp/live-configfile-dra-e2e.log
```

Use that same live DRA test for browser GPU streaming dispatch acceptance: the
Selkies ConfigFile template still declares `nvidia.com/gpu: "1"`, so it must hit
the existing ResourceClaimTemplate + MPS injection path. Browser WebRTC, NVENC,
and forced-TURN relay validation are operator-run checks on a GPU cluster; see
`docs/browser-gpu-streaming.md`.

Run the optional live Harbor adapter boundary E2E only when `HARBOR_URL` points
at a local or staging Harbor endpoint. This test verifies the existing Harbor
adapter status path; image build queue/log/cancel behavior remains covered by
the non-live governance E2E because the production code is currently
record-backed:

```sh
cd /home/lin/Desktop/NexusPaas/backend
TEST_LIVE_HARBOR_IMAGE_BUILD=1 HARBOR_URL=http://localhost:8080 \
  go test -tags e2e ./internal/e2e -run '^TestLiveHarborImageBuildE2E$' -count=1 -v
```

Run the live Kubernetes policy-data ConfigMap E2E while working on the
authorization-policy sync worker. This command is opt-in because it creates and
deletes a temporary namespace in the current Kubernetes context; for acceptance,
the log must show the test passed and did not skip:

```sh
cd /home/lin/Desktop/NexusPaas/backend
set -o pipefail
TEST_LIVE_K8S_POLICY_DATA_SYNC=1 \
  go test -tags e2e ./internal/e2e -run '^TestPolicyDataSyncConfigMapE2E$' -count=1 -v \
  | tee /tmp/policy-data-sync-e2e.log
rg '^--- PASS: TestPolicyDataSyncConfigMapE2E' /tmp/policy-data-sync-e2e.log
! rg 'SKIP|skipping|Skipping' /tmp/policy-data-sync-e2e.log
```

## Production Beta Quality Gate

The CI workflow and local reviewers use the same gate script:

```sh
cd /home/lin/Desktop/NexusPaas
bash backend/scripts/ci-security-gate.sh all
```

Use focused subcommands while iterating:

```sh
bash backend/scripts/ci-security-gate.sh quick
bash backend/scripts/ci-security-gate.sh docker
bash backend/scripts/ci-security-gate.sh security
SONAR_HOST_URL=http://localhost:9000 SONAR_TOKEN=<token> \
  bash backend/scripts/ci-security-gate.sh sonar
```

The Docker-backed gate uses isolated ports by default:

- Postgres: `localhost:15432`
- Redis: `localhost:16379`
- MinIO API/console: `localhost:19000` / `localhost:19001`

The gate writes `backend/coverage.out` for Sonar and fails when integration
coverage is below `CI_GATE_COVERAGE_THRESHOLD`, which defaults to `80.0`.
Focused E2E must emit the required `PASS` lines and cannot pass by skipping.
Full non-live E2E runs after the focused gate. The Docker-backed gate then
starts `SERVICE_NAME=all` on `TEST_RUNTIME_PORT` (default `18080`) and checks
`/healthz`, `/readyz`, `/metrics`, `/openapi.json`, `/service-registry`, and
one read-only endpoint for each of the 15 registered services with no 5xx.
Live cluster tests remain guarded by their explicit opt-in environment
variables.

In GitHub Actions, SonarScanner is required for pushes, workflow dispatches, and
non-fork pull requests. Fork pull requests skip Sonar when secrets are
unavailable; branch protection should require the non-fork/default-branch gate
before merge.

Run the non-live Production Beta release-candidate rehearsal when preparing a
Beta RC:

```sh
bash backend/scripts/ci-security-gate.sh beta-rc
```

This command runs the quick gate, renders `kubectl kustomize backend`, verifies
the production-beta render contains the 15 NexusPaas service deployments and no
all-in-one `platform` deployment, rejects `-dev-` secret references, runs
client-side deploy dry-run, writes rollback commands for every service
deployment, runs re-deploy dry-run, then executes Docker-backed E2E, runtime smoke,
security, and Sonar gates. It writes `${ARTIFACT_DIR}/beta-rc-report.md` plus
render, dry-run, rollback, E2E, and runtime-smoke artifacts.

The `beta-rc` gate is non-live by default. External Production Beta traffic
still requires a live staging rehearsal with real staging secrets, ready pods,
15-service health/ready/metrics smoke, critical journey E2E, rollback, and
re-deploy evidence. See `docs/beta-launch-readiness.md`.

Existing manual gates remain:

```sh
cd /home/lin/Desktop/NexusPaas/backend
go test ./...
go vet ./...
go test -tags integration ./...
go test ./... -coverprofile=coverage.out

cd /home/lin/Desktop/NexusPaas
sonar-scanner -Dsonar.qualitygate.wait=true
```

## Notes

- `TEST_EVENT_BUS_URL` defaults to `TEST_REDIS_URL` when unset.
- `TEST_OBJECT_STORE_BUCKET` defaults to `media-e2e` when unset.
- The harness wires every started peer URL into each service's `SERVICE_URLS`.
  Individual journeys assert only the owner contracts they exercise, and failure
  paths may start extra service instances with a deliberately narrowed or bad
  peer map.
- Internal identity read/auth contracts require `X-Service-Key`; scheduler
  admission uses the current route auth contract with `X-API-Key` bound to a
  service principal.
