# Web GUI Job Logs Live Nonempty Proof

## 1. Objective

Close the current Web GUI job-log evidence gap by making
`GET /api/v1/jobs/{id}/logs` return non-empty bounded log records for a real
Kubernetes pod that belongs to an authorized workload Job, then prove it through
the first-party GUI E2E harness.

This proves bounded non-empty log retrieval. It does not claim continuous
tailing, WebSocket streaming, full workload lifecycle status, log retention, or
multi-tenant log indexing.

## 2. Background

The GUI already requests `/api/v1/jobs/{id}/logs`. Current live route proof
shows `job_logs_status=200` but `job_logs_count=0`, so WEB workload evidence
still lacks visible non-empty logs.

The workload service currently lists stored `workload-service:job_logs` rows
related to the authorized Job. It does not read Kubernetes pod logs.

The repository already includes `client-go v0.33.3`, an injectable
`platform.Cluster *cluster.Client`, dispatcher-created resource metadata on Job
records, and platform pod labels such as `platform-go/job-id`.

## 3. Source References

- `docs/agents/planning.md` defines the required plan shape.
- `backend/internal/services/workload/job_access_handlers.go` owns
  `/api/v1/jobs/{id}/logs` and Project authorization.
- `backend/internal/platform/cluster/cluster.go` defines the injected cluster
  facade.
- `backend/internal/platform/cluster/resource_usage.go` already lists job pods
  by `platform-go/job-id`, proving the label convention and cluster facade are
  the right boundary.
- `backend/internal/services/workload/job_repository.go` stores
  `created_resources` after dispatch.
- `backend/internal/services/workload/dispatcher.go` records created Kubernetes
  objects as `{kind, namespace, name}`, making dispatched pod metadata the
  narrowest lookup path before falling back to a label selector.
- `frontend/tests/e2e/dashboard.spec.ts` records `job_logs_status`,
  `job_logs_count`, and `job_logs_visible`.
- Context7 lookup for `client-go` was attempted but unavailable because the
  configured API key is invalid. Fallback primary documentation:
  `pkg.go.dev/k8s.io/client-go/kubernetes/typed/core/v1`, where
  `PodInterface.GetLogs(name string, opts *v1.PodLogOptions) *restclient.Request`
  is documented.

## 4. Assumptions

- Platform-managed workload pods carry `platform-go/job-id=<job id>`; dispatcher
  paths already apply this label to native/fallback pods.
- A Job record has a namespace from submit time or dispatch metadata sufficient
  to search for its pods.
- A nil `app.Cluster` means degraded/local mode and must not break stored log
  reads.
- Live proof may use a short-lived Kubernetes pod fixture with a deterministic,
  non-secret log line and platform labels.

## 5. Non-Goals

- Do not add Loki, Fluent Bit, OpenSearch, Vector, or another log backend.
- Do not add follow-mode, WebSocket tailing, or browser media proof in this
  slice.
- Do not persist Kubernetes pod logs in Postgres.
- Do not widen RBAC beyond existing Job Project authorization.
- Do not modify scheduler admission, ConfigFile submit, or Job dispatch
  semantics except for tests/fixtures if needed.

## 6. Current Behavior

`listJobLogs` authorizes the Job, then returns only records from
`workload-service:job_logs` whose `job_id`, `jobId`, or `job_record_id` matches.
When no stored rows exist, the route returns HTTP `200` with an empty list.

The GUI renders the log table and live E2E records the empty state, but this is
not enough for GA Web evidence.

## 7. Target Behavior

After existing stored rows are collected, `listJobLogs` appends Kubernetes pod
log records when all conditions are met:

- `app.Cluster` is configured.
- The authorized Job has a namespace and job id.
- Matching pods exist in that namespace with `platform-go/job-id=<job id>`.

Candidate pod lookup is deterministic:

- Primary: Job `created_resources` entries where `kind == "Pod"` and namespace
  plus name are present.
- Fallback: namespace-scoped list of pods matching
  `platform-go/job-id=<job id>` when no created Pod resources are recorded.

Kubernetes reads are bounded with exact limits:

- `TailLines=200`
- `LimitBytes=65536`
- `Follow=false`
- `MaxPods=8`
- `MaxContainers=16`
- `MaxLines=200`

If Kubernetes is unavailable or no pods match, the route still returns existing
stored rows or an empty list. Successful log lines are converted into the same
list/envelope shape the GUI already consumes.

## 8. Affected Domains

- `workload-service`: owns Job authorization and the `/api/v1/jobs/{id}/logs`
  API response.
- `k8s-control` / cluster facade: Kubernetes API access remains behind the
  existing cluster client boundary. No new deployable service is introduced.
- `platform-gateway`: only proxies the existing route; no new gateway contract
  is required.

## 9. Affected Files

Planned write set:

- `backend/internal/platform/cluster/pod_logs.go` (new bounded pod-log helper)
- `backend/internal/platform/cluster/cluster_test.go` or focused
  `pod_logs_test.go`
- `backend/internal/services/workload/job_access_handlers.go`
- `backend/internal/services/workload/handler_test.go` or focused
  `job_logs_test.go`
- `frontend/tests/e2e/dashboard.spec.ts` only if route proof needs stronger
  log-text/visibility diagnostics
- `docs/acceptance/gap-analysis.md`, `gap.md`, and `problem.md` after live
  proof

## 10. API / Contract Changes

No route path, method, auth mode, or envelope contract changes.

The existing `/api/v1/jobs/{id}/logs` list gains additional Kubernetes-derived
rows with data fields:

- `job_id`
- `job_record_id`
- `project_id`
- `namespace`
- `pod`
- `container`
- `line`
- `message`
- `timestamp` when Kubernetes returns prefixed timestamps
- `source="kubernetes_pod_logs"`

The response remains a list of `contracts.Record[map[string]any]`-compatible
objects so existing GUI parsing still works.

## 11. Database / Migration Changes

None. Kubernetes pod logs are not persisted. Existing stored `job_logs` rows are
still returned unchanged.

## 12. Configuration Changes

No committed config or Secret changes.

Runtime behavior depends only on the existing injected `platform.Cluster`. Live
E2E may create and delete a short-lived Kubernetes pod fixture; that is test
data, not application configuration.

## 13. Observability Changes

No new metrics are required for this narrow slice.

Route proof will include:

- `job_logs_status=200`
- `job_logs_count>0`
- `job_logs_nonempty=true`
- `job_logs_visible=true`

If a Kubernetes container log read fails while other containers succeed, the
helper may include a non-secret diagnostic row with source/error metadata; it
must not include Kubernetes credentials, environment variables, or pod command
payloads.

## 14. Security Considerations

`authorizedJobRecord` remains the only entry point. Callers cannot supply
namespace, pod, or container names through the Job logs route. The service uses
the authorized Job's `project_id`, `namespace`, and `job_id` to find matching
pods.

Log rows must not print Secrets or service-account tokens in planned tests/live
fixtures. The live proof log line must be deterministic and non-secret.

## 15. Implementation Steps

1. Add `cluster.PodLogLine` and a helper such as
   `ListJobPodLogs(ctx, namespace, jobID, opts)` in `backend/internal/platform/cluster/`.
2. The helper accepts explicit pod names from Job `created_resources` and uses
   those first; if none are supplied, it lists pods in the namespace with label
   selector `platform-go/job-id=<job id>`.
3. For each candidate pod/container, call
   `CoreV1().Pods(namespace).GetLogs(pod.Name, &corev1.PodLogOptions{Container:
   container.Name, TailLines: &tail, LimitBytes: &limit, Timestamps: true,
   Follow: false})`.
4. Parse returned lines into bounded `PodLogLine` values. Preserve a line
   number per container. Do not retain raw buffers beyond `LimitBytes`.
5. In `workload`, extend `listJobRelatedRecords` or `listJobLogs` so only the
   logs route appends Kubernetes log rows after stored rows.
6. Map Kubernetes log lines into `contracts.Record[map[string]any]` with stable
   synthetic IDs such as `k8s:<namespace>:<pod>:<container>:<line>`.
7. Keep nil-cluster and no-pod behavior as HTTP `200` with existing rows.
8. Add focused tests for:
   - stored rows still returned
   - nil cluster does not fail
   - Kubernetes log rows append from `created_resources` pod metadata with exact cap options
   - namespace/job label fallback works when no Pod `created_resources` exist
   - unauthorized user cannot read another Project's Job logs
9. Run backend/frontend verification and live E2E.
10. Update trackers only after live evidence exists.

## 16. Verification Plan

Local:

- `go test ./internal/platform/cluster ./internal/services/workload`
- `make -C backend lint`
- `make -C backend build`
- `npm --prefix frontend test -- --run`
- `npm --prefix frontend run build`
- `make -C backend ci-sonar`

Live E2E:

- Build/push a new image, for example
  `localhost:5000/nexuspaas-backend:ci-ga-job-logs-nonempty-<timestamp>`.
- Roll only required deployments:
  - `workload-service`
  - `platform-gateway` only if frontend assets change.
- Start gateway port-forward.
- Run the existing seeded GUI harness with:

```bash
NEXUSPAAS_E2E_EXPECT_NONEMPTY_LOGS=true npm --prefix frontend run e2e
```

Expected proof values:

- `job_logs_status=200`
- `job_logs_count>0`
- `job_logs_nonempty=true`
- `job_logs_visible=true`

Cleanup proof:

- Seeded Project deleted or reads back `404`.
- Fixture pod/namespace deleted by exact label.
- No temporary runtime env remains.

## 17. Rollback Plan

- Revert the cluster pod-log helper and workload route changes.
- Roll `workload-service` back to its prior image if live rollout causes route
  errors.
- Roll `platform-gateway` back only if frontend assets changed.
- Delete any fixture pod/namespace by exact label.

## 18. Risks and Tradeoffs

- `client-go` fake log support may require a fake REST client/reactor seam in
  tests. Keep that seam local to the cluster helper if needed.
- Kubernetes logs can be large; this plan fixes `TailLines=200` and
  `LimitBytes=65536` and tests those values to avoid unbounded reads.
- Multi-container pods may have partial failures. The route should return
  successful lines and non-secret diagnostics rather than hiding all logs.
- This does not replace a production log aggregation stack; it only closes the
  current GUI evidence gap for bounded logs.

## 19. Reviewer Checklist

| Category | Plan Evidence | Status |
|---|---|---|
| Requirement fit | Sections 1, 2, 7, and 16 target non-empty GUI log evidence only. | Ready for review |
| Scope control | Sections 5 and 18 exclude log aggregation, WebSocket tailing, and media proof. | Ready for review |
| SOLID | Sections 8, 10, and 15 keep cluster reading separate from workload authorization. | Ready for review |
| 12-Factor | Section 12 uses injected cluster client and no committed live config. | Ready for review |
| Security | Sections 10 and 14 preserve RBAC and avoid secret exposure. | Ready for review |
| Tests | Section 16 lists focused tests, build, frontend, and Sonar. | Ready for review |
| Live evidence | Section 16 defines concrete E2E command, expected proof, and cleanup. | Ready for review |
| Diff scope | Section 9 lists the intended write set. | Ready for review |

## 20. Status

Status: Implemented and live verified

Reviewer Agent requested revisions to the first draft for plan-shape compliance,
concrete bounded-read limits, and complete verification gates. This revision
uses the required 20-section structure, fixes exact `TailLines=200` /
`LimitBytes=65536` behavior, and includes Sonar plus live E2E proof commands.
Reviewer Agent approved the revised plan.

Implementation evidence (2026-06-22):

- Added bounded Kubernetes pod-log reads behind the existing cluster facade with
  `TailLines=200`, `LimitBytes=65536`, `Follow=false`, `MaxPods=8`,
  `MaxContainers=16`, `MaxLines=200`, explicit created-pod lookup, and
  namespace-scoped `platform-go/job-id` fallback.
- Extended the authorized workload Job logs route to append Kubernetes-derived
  rows without changing the route, auth mode, envelope shape, or database
  schema.
- Reviewer-requested hardening is included: Kubernetes log reads use the
  server-derived Project namespace, reject stored namespace drift, ignore
  caller-supplied submit namespaces for new Jobs, and cap aggregate pod,
  container, and line reads per request.
- Verification passed:
  `go test ./internal/platform/cluster ./internal/services/workload`,
  `make -C backend lint`, `make -C backend build`,
  `npm --prefix frontend test -- --run`,
  `npm --prefix frontend run build`, `make -C backend ci-sonar`,
  and `git diff --check`.
- Live image:
  `localhost:5000/nexuspaas-backend:ci-ga-job-logs-nonempty-fix-20260622130645`
  (`sha256:fdb674beaf60e1ea052a7cbc974263b5c9fee4d39927c5980c12feb48ff2cc7e`).
- Live Playwright E2E with `NEXUSPAAS_E2E_EXPECT_NONEMPTY_LOGS=true` passed.
  Seeded Project `e2e-p-mqora84n-1y46vp` used fixture pod `log-proof` in
  namespace `proj-e2e-p-mqora84n-1y46vp`; the pod emitted
  `nexuspaas-log-proof-e2e-p-mqora84n-1y46vp`, and route proof recorded
  `job_logs_status=200`, `job_logs_count=1`, `job_logs_nonempty=true`, and
  `job_logs_visible=true`.
- Cleanup verified: seeded Project/Group/Plan/Queue/Config/Image rows were
  removed through existing APIs where delete routes exist, the exact proof
  namespace was deleted, build/tune pods were deleted, and temporary host
  inotify values were restored to `128` / `65536`.
