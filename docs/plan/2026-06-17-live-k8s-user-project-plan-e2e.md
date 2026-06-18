# Live K8s User Project Plan E2E

## 1. Objective

Implement an opt-in live E2E test that proves a user can register through the
LDAP-backed identity flow, be assigned by an admin to a project with a bound
scheduler plan, create a ConfigFile in that assigned project, and deploy a
minimal Kubernetes Job through workload submit on Docker Desktop Kubernetes.

## 2. Background

The backend already contains live opt-in E2E tests for LDAP, Kubernetes
readiness, policy ConfigMaps, priority classes, and plan-window cleanup. The
requested journey combines several service boundaries: identity, org-project,
scheduler-quota, workload, and the live Kubernetes facade.

The current workload deployment path is `POST /api/v1/jobs` with a resource
manifest payload. `POST /api/v1/configfiles/{id}/instance` records an instance
command and is not the live deployment path.

## 3. Source References

- `backend/internal/e2e/identity_ldap_e2e_test.go`
- `backend/internal/e2e/harness_test.go`
- `backend/internal/services/workload/handler.go`
- `backend/internal/services/workload/job_submit.go`
- `backend/internal/services/workload/dispatcher.go`
- `backend/internal/services/orgproject/project_handlers.go`
- `backend/internal/services/schedulerquota/handler.go`
- `backend/internal/platform/cluster/apply.go`
- `backend/docs/e2e-testing.md`

## 4. Assumptions

- Docker Desktop Kubernetes is the current context and may be mutated only by
  uniquely named E2E resources that the test cleans up.
- OpenLDAP, Postgres, Redis, and MinIO are supplied by the existing documented
  local E2E setup.
- Deployment uses workload job submission with `config_id`; ConfigFile instance
  commands are not expanded in this task.
- Project assignment is a security boundary: authenticated users outside a
  project must not read/write ConfigFiles or submit jobs for that project.
- Workload must not read identity or org-project tables directly in isolated
  service mode. It uses verified auth headers for the subject/admin role and the
  existing org-project owner-read contracts for project/project-member data.

## 5. Non-Goals

- Do not deploy all backend services into Kubernetes.
- Do not add a new deployment API or change route shapes.
- Do not implement `/api/v1/configfiles/{id}/instance` live deployment.
- Do not add long-running workloads, Services, Ingresses, or custom CRDs to the
  live E2E.
- Do not change LDAP schema or production LDAP configuration semantics.

## 6. Current Behavior

- LDAP registration/login is covered by a separate opt-in E2E.
- Scheduler plan binding is covered by a cross-service owner-contract E2E.
- Workload dispatcher can create native Kubernetes resources when a real
  cluster client is configured.
- Workload ConfigFile and job submit handlers do not currently enforce
  org-project membership before project-scoped reads or writes.

## 7. Target Behavior

- Project-scoped workload reads and writes require either platform admin access
  or membership in the target project.
- A live E2E proves the positive path: LDAP user registered, assigned to a
  project, plan bound, ConfigFile created, job submitted, dispatcher creates a
  Kubernetes Job, and workload job state becomes `running`.
- The same E2E proves the negative path: a non-member cannot create a ConfigFile
  or submit a job for another project.
- The live E2E is skipped unless `TEST_LIVE_K8S_USER_PROJECT_PLAN_DEPLOY=1` is
  set.

## 8. Affected Domains

- Identity: LDAP-backed user registration/login and identity projection.
- Org-project: project membership assignment and project ownership.
- Scheduler-quota: queue/plan creation and project plan binding.
- Workload: ConfigFile access control, job submit access control, dispatcher.
- Kubernetes: live namespace and minimal Job creation/cleanup.

## 9. Affected Files

- `docs/plan/2026-06-17-live-k8s-user-project-plan-e2e.md`
- `backend/internal/services/workload/handler.go`
- `backend/internal/services/workload/job_submit.go`
- `backend/internal/services/workload/project_access.go`
- `backend/internal/services/workload/job_access_handlers.go`
- `backend/internal/services/workload/handler_test.go`
- `backend/internal/services/workload/job_submit_test.go`
- `backend/internal/services/catalog.go`
- `backend/internal/services/service_isolation_test.go`
- `backend/internal/e2e/live_user_project_plan_deploy_e2e_test.go`
- `backend/docs/e2e-testing.md`

## 10. API / Contract Changes

No route shape changes.

Behavioral contract change: workload project-scoped read, write, instance
command, and deploy routes fail with `403` when the authenticated user is
neither an admin nor a member of the target project. Route shapes and response
envelope shape remain unchanged.

Protected public workload routes:

- `GET /api/v1/configfiles`
- `POST /api/v1/configfiles`
- `GET /api/v1/configfiles/{id}`
- `PUT /api/v1/configfiles/{id}`
- `PATCH /api/v1/configfiles/{id}`
- `DELETE /api/v1/configfiles/{id}`
- `POST /api/v1/configfiles/{id}/versions`
- `GET /api/v1/configfiles/{id}/versions`
- `GET /api/v1/configfiles/tree`
- `GET /api/v1/configfiles/project/{project_id}`
- `GET /api/v1/projects/{id}/config-files`
- `GET /api/v1/configfiles/project/{project_id}/tree`
- `GET /api/v1/configfiles/project/{project_id}/history`
- `POST /api/v1/configfiles/{id}/instance`
- `DELETE /api/v1/configfiles/{id}/instance`
- `GET /api/v1/configfiles/{id}/instance/pods`
- `GET /api/v1/jobs`
- `POST /api/v1/jobs`
- `GET /api/v1/jobs/{id}`
- `POST /api/v1/jobs/{id}/cancel`
- `GET /api/v1/jobs/{id}/logs`
- `GET /api/v1/jobs/{id}/gpu-summary`
- `GET /api/v1/jobs/{id}/gpu-timeline`
- `GET /api/v1/jobs/{id}/gpu-breakdown`

Internal workload routes remain governed by their existing service-key/internal
authorization.

Global non-project workload routes remain unchanged: `GET /api/v1/jobs/templates`
does not carry project ownership today and is not part of the project-scoped
security boundary.

List/tree semantics:

- Admin users receive the existing all-project list/tree result.
- Non-admin users receive only records whose `project_id` belongs to one of
  their org-project memberships.
- Single-record ConfigFile/job/log/GPU/cancel routes resolve the parent
  ConfigFile or job first, then authorize against that record's `project_id`.

ConfigFile project ownership is immutable in this task. `PUT`/`PATCH` may omit
`project_id` or repeat the existing value; attempts to change `project_id` /
`projectId` fail with `400`. Moving a ConfigFile to another project remains
outside this plan and can be modeled as create/delete in a future task.

## 11. Database / Migration Changes

No schema or migration changes.

Workload uses the existing platform owner-read model for foreign-owned
authorization data:

- Subject/admin status comes from verified request headers populated by platform
  authentication middleware. Workload does not read `identity-service` records
  for this check.
- Project existence and project membership are read from
  `org-project-service:projects` and `org-project-service:project_members`
  through existing org-project read contracts. In `SERVICE_NAME=all` this is a
  local owner read; in isolated `SERVICE_NAME=workload-service` it is resolved by
  `crossServiceStore` / `RemoteServiceReader` with `SERVICE_URLS` and
  `SERVICE_API_KEY`.
- `backend/internal/services/catalog.go` will register workload owner-read
  dependencies for those org-project resources so service isolation validation
  remains explicit.

## 12. Configuration Changes

- New test-only opt-in env var:
  `TEST_LIVE_K8S_USER_PROJECT_PLAN_DEPLOY=1`.
- The live E2E reuses existing `KUBECONFIG`, LDAP, Postgres, Redis, and MinIO
  test environment variables.

## 13. Observability Changes

No new metrics or tracing. The E2E relies on existing request correlation,
workload events, and job `created_resources` state for assertions.

## 14. Security Considerations

- LDAP credentials remain externalized through environment variables.
- The live E2E must not log passwords, LDAP bind credentials, session tokens, or
  API tokens.
- New workload authorization checks fail closed for missing project IDs,
  missing authenticated users, missing projects, owner-read errors, or
  non-member users.
- Admin access derives from verified platform auth headers. Since the platform
  strips inbound identity headers before auth when `RequireAuth=true`, callers
  cannot self-assert admin role on protected deployments.

## 15. Implementation Steps

1. Add focused workload authorization helpers that verify project access through
   verified auth headers plus org-project owner-read contracts without changing
   route shapes.
2. Register workload owner-read dependencies on
   `org-project-service:projects` and `org-project-service:project_members`,
   then update the isolation dependency test expectations.
3. Register custom workload handlers for every protected public workload route
   listed in section 10, including catalog fallback routes for global
   ConfigFile/job lists, ConfigFile versions, job get/cancel/log/GPU reads, and
   ConfigFile instance command/pod listing routes.
4. Preserve existing public response shapes while filtering list/tree outputs by
   authorized project membership for non-admin users.
5. Enforce immutable ConfigFile project ownership in `PUT`/`PATCH` by rejecting
   any target `project_id` / `projectId` that differs from the stored record.
6. Add unit tests covering member/admin allow, non-member deny, filtered
   list/tree output, job get/cancel/log/GPU route guards, isolated
   owner-read membership lookup, and immutable ConfigFile project behavior.
7. Add the opt-in live E2E using the existing harness, real LDAP config, and
   `cluster.NewFromEnv("proj")` through the workload backing resources.
8. Add E2E runbook instructions for the new live test, cleanup scope, and
   no-skip output assertions.
9. Run focused unit tests, the live E2E when local backing services are
   available, and the quick CI gate.

## 16. Verification Plan

Run:

```sh
cd /Users/sky/workspaces/backend
go test ./internal/services/workload ./internal/services/orgproject ./internal/services/schedulerquota ./internal/services/authorizationpolicy ./internal/platform/cluster -count=1
```

Run the live test after starting documented local backings and OpenLDAP:

```sh
cd /Users/sky/workspaces/backend
TEST_LIVE_K8S_USER_PROJECT_PLAN_DEPLOY=1 \
  go test -tags e2e ./internal/e2e -run '^TestLiveLDAPUserProjectPlanConfigDeployE2E$' -count=1 -v \
  | tee /tmp/live-k8s-user-project-plan-e2e.log
rg '^--- PASS: TestLiveLDAPUserProjectPlanConfigDeployE2E' /tmp/live-k8s-user-project-plan-e2e.log
! rg '^--- SKIP: TestLiveLDAPUserProjectPlanConfigDeployE2E' /tmp/live-k8s-user-project-plan-e2e.log
```

Run the local quick gate:

```sh
cd /Users/sky/workspaces
bash backend/scripts/ci-security-gate.sh quick
```

Run SonarScanner Quality Gate if `SONAR_HOST_URL` and `SONAR_TOKEN` are
available:

```sh
cd /Users/sky/workspaces
bash backend/scripts/ci-security-gate.sh sonar
```

If Sonar credentials or scanner binary are unavailable, record that explicitly
in the final implementation notes instead of treating it as passed. If it runs,
record the actual Quality Gate result.

## 17. Rollback Plan

Revert the workload access-control changes, the new E2E file, the E2E runbook
update, and this plan document. Delete any leftover live E2E namespace matching
the generated run ID if cleanup fails.

## 18. Risks and Tradeoffs

- Enforcing workload membership can reveal missing org-project owner-read
  configuration in isolated local setups; this is intentional fail-closed
  behavior for project-scoped reads/writes.
- The live E2E depends on local Docker Desktop Kubernetes, OpenLDAP, Postgres,
  Redis, and MinIO, so it remains opt-in and is not part of default unit gates.
- The E2E uses a minimal Kubernetes Job to reduce cluster mutation and cleanup
  risk, at the cost of not exercising Deployment/Service rollout behavior.

## 19. Reviewer Checklist

- Requirement fit: LDAP, project assignment, plan binding, ConfigFile, and live
  Kubernetes Job deployment are all verified.
- Scope control: no route shape, schema, or broad deployment changes.
- Security: non-members fail closed and secrets are not logged.
- Microservice ownership: workload reads org-project data only through owner-read
  contracts and does not write foreign-owned data.
- Testing: unit tests cover authorization behavior; live E2E covers the full
  requested journey.
- Rollback: changes are surgical and independently revertible.

## 20. Status

Status: Approved
