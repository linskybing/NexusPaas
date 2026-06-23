# V1 Launch Gap Gate

## 1. Objective

Turn `ac.md` and `docs/acceptance/gap-analysis.md` into an executable v1
checklist, then implement the smallest launch-blocking code slice first:
gateway/input abuse controls for `GATE-*` plus the K8S manifest size/document
cap called out in the gap analysis.

## 2. Background

`ac.md` points to `docs/acceptance/`. The accepted GA docs already cover the
core compute, GPU, RBAC, image, usage, security, operations, and performance
families. `gap-analysis.md` adds proposed launch-v1 blockers: `STORAGE-*`,
`SECRET-*`, `AUDIT-*`, `PLANADMIN-*`, and `GATE-*`; `WEB-*` is conditional on
advertising a NexusPaaS management Web UI and is not part of the API/CLI-only v1
scope.

The codebase already has the 8 deployable-unit Production Beta topology, route
auth hardening, rate-limit middleware, storage mount-plan contracts, ConfigFile
versioning, and quick gate coverage. First iteration should reuse those paths
instead of adding new platform products.

## 3. Source References

- `ac.md`
- `docs/acceptance/README.md`
- `docs/acceptance/gap-analysis.md`
- `docs/acceptance/cncf-adoption.md`
- `docs/acceptance/k8s-deployment.md`
- `docs/acceptance/ga-checklist.md`
- `docs/acceptance/iteration-plan.md`
- `docs/architecture/cncf-package-strategy.md`
- `docs/architecture/service-boundaries.md`
- `backend/docs/beta-launch-readiness.md`
- `backend/docs/operational-readiness.md`
- `problem.md`
- `backend/internal/platform/middleware.go`
- `backend/internal/platform/input_limits.go`
- `backend/internal/platform/response.go`
- `backend/internal/services/workload/handler.go`
- `backend/internal/services/schedulerquota/admission.go`

## 4. Assumptions

- Reviewer accepts the proposed `GATE-*` and K8S manifest-cap ACs as v1
  launch-blocking for this implementation slice.
- Existing CNCF direction stays: Kubernetes admission remains Kyverno /
  ValidatingAdmissionPolicy; queueing remains Kueue-oriented; metrics remain
  Prometheus/OpenTelemetry; no custom scheduler, registry, secret store, or
  metrics backend is added.
- This slice proves local/backend behavior. Live staging deploy, rollback, and
  beta-rc evidence remain separate release evidence gates.

## 5. Non-Goals

- No Web UI implementation in this slice.
- No new service, framework, gateway product, message broker, or service mesh.
- New Go dependencies are allowed only if they are mature, maintained, and
  smaller/safer than local code. This slice should first use existing
  Kubernetes/YAML dependencies already present in `backend/go.mod`.
- No replacement for Kyverno, Kueue, Harbor, External Secrets/Vault,
  Prometheus, or OpenTelemetry.
- No broad typed-domain migration, workload identity migration, or
  transactional outbox/inbox implementation.

## 6. Current Behavior

- Quick local gate passes.
- Request rate limiting exists, but 429 responses do not include retry guidance.
- JSON request bodies are read with `io.ReadAll` and have no global configured
  byte limit.
- ConfigFile content and admission resources do not have an explicit manifest
  byte/document-count guard before parsing/admission work.

## 7. Target Behavior

- Gateway returns 429 with retry guidance for rate-limited principals.
- API JSON payloads and ConfigFile/manifest submissions have configured byte
  caps and reject oversized input before expensive parsing.
- ConfigFile/manifest submissions reject too many YAML documents with
  machine-readable errors.
- Existing route auth, storage mount-plan, scheduler admission, and Kubernetes
  admission paths stay intact.

## 8. Affected Domains

- Platform gateway/runtime middleware.
- Workload ConfigFile lifecycle.
- Scheduler admission resource decoding.
- Backend tests and docs/plan checklist only.

## 9. Affected Files

- `docs/plan/2026-06-20-v1-launch-gap-gate.md`
- `backend/internal/platform/config.go`
- `backend/internal/platform/middleware.go`
- `backend/internal/platform/response.go`
- `backend/internal/platform/input_limits.go`
- `backend/internal/platform/ratelimit_test.go`
- `backend/internal/platform/config_test.go`
- `backend/internal/services/workload/handler.go`
- `backend/internal/services/workload/handler_test.go`
- `backend/internal/services/schedulerquota/admission.go`
- `backend/internal/services/schedulerquota/admission_test.go`

## 10. API / Contract Changes

- Error envelopes stay compatible.
- 429 responses add retry guidance, either in `Retry-After` and/or response
  data/error message.
- Oversize API payloads return `413`.
- Oversize or over-document ConfigFile/manifest content returns a 4xx
  machine-readable rejection reason.

## 11. Database / Migration Changes

None.

## 12. Configuration Changes

Add small config values with safe defaults:

- `MAX_API_BODY_BYTES`
- `MAX_CONFIGFILE_BYTES`
- `MAX_CONFIGFILE_DOCUMENTS`

Defaults must be local-friendly but bounded. Validation rejects negative
values; unset/zero values fall back to bounded defaults.

## 13. Observability Changes

Use existing request metrics. If low-cost, increment existing-style counters for
body-limit and manifest-limit rejections.

## 14. Security Considerations

This closes a basic DoS surface: public API callers cannot force unbounded body
reads or unbounded ConfigFile/admission manifest parsing. It does not replace
cluster admission policy; Kyverno / ValidatingAdmissionPolicy remains required
for bypass protection.

## 15. Implementation Steps

- [x] Read AC, gap analysis, workflow, architecture, and launch-readiness docs.
- [x] Run baseline quick gate.
- [x] Add config fields and validation for body and manifest limits.
- [x] Add platform body-limit handling using Go stdlib only.
- [x] Add ConfigFile content byte/doc-count validation.
- [x] Add scheduler admission resource byte/doc-count validation.
- [x] Add focused tests for config parsing, 429 retry guidance, 413 body limit,
  and ConfigFile/admission manifest caps.
- [x] Run focused tests and `bash backend/scripts/ci-security-gate.sh quick`.
- [x] Update this checklist with verification evidence.
- [x] Run the full release-candidate gate after all accepted v1 gap slices.
- [x] Deploy the RC image to the live local RKE2 namespace and run smoke.
- [x] Rehearse rollback and re-deploy on `platform-gateway`.

## 16. Verification Plan

```sh
go -C backend test ./internal/platform -run 'Config|Rate|Body|Middleware' -count=1
go -C backend test ./internal/services/workload -run 'ConfigFile|Manifest|Body' -count=1
go -C backend test ./internal/services/schedulerquota -run 'Admission|Manifest' -count=1
bash backend/scripts/ci-security-gate.sh quick
```

Executed on 2026-06-20:

```sh
go -C backend test ./internal/platform -run 'ConfigInputLimit|ValidateManifestValueAllowsPlainTextContent' -count=1 -v
go -C backend test ./internal/platform -run 'Rate|Body|Middleware|ConfigInputLimit|ValidateManifestValue' -count=1
go -C backend test ./internal/services/workload -run 'ConfigFileRejectsOversizedManifest|ConfigVersionRejectsTooManyManifestDocuments' -count=1
go -C backend test ./internal/services/schedulerquota -run 'SubmitAdmissionRejectsOversizedManifestResource|SubmitAdmissionRejectsTooManyManifestDocuments' -count=1
go -C backend test ./internal/services/workload ./internal/services/schedulerquota ./internal/platform -count=1
go -C backend test ./internal/services -count=1
go -C backend test ./... -coverprofile=coverage.out -count=1
bash backend/scripts/ci-security-gate.sh quick
bash backend/scripts/ci-security-gate.sh sonar
```

Result: all commands passed; SonarScanner Quality Gate passed.

Full release evidence:

```sh
TEST_MINIO_PORT=29000 TEST_MINIO_CONSOLE_PORT=29001 bash backend/scripts/ci-security-gate.sh beta-rc
```

Result: passed on 2026-06-20. The final RC report is
`/tmp/nexuspaas-quality-gate/local-1710124/beta-rc-report.md`. The gate covered
quick Go checks, production-beta kustomize and rollback/redeploy dry-runs,
migrations, integration coverage (`83.1%`, threshold `80%`), focused E2E,
non-live runtime smoke, 8-unit collaboration smoke, govulncheck, OSV, Trivy
image scan (`0` vulnerabilities), and SonarScanner Quality Gate.

Live local RKE2 staging evidence on 2026-06-20:

```sh
docker run -d --name nexuspaas-local-registry -p 5000:5000 registry:2
docker tag nexuspaas-backend:ci-local-1710124 localhost:5000/nexuspaas-backend:ci-local-1710124
docker push localhost:5000/nexuspaas-backend:ci-local-1710124
kubectl -n nexuspaas run nexuspaas-image-pull-test --image=localhost:5000/nexuspaas-backend:ci-local-1710124 --image-pull-policy=Always --restart=Never --command -- /bin/sh -c 'echo image-pull-ok'
kubectl -n nexuspaas set image deployment/<backend-service> app=localhost:5000/nexuspaas-backend:ci-local-1710124
kubectl -n nexuspaas rollout status deployment/<backend-service> --timeout=180s
kubectl -n nexuspaas rollout undo deployment/platform-gateway
kubectl -n nexuspaas set image deployment/platform-gateway app=localhost:5000/nexuspaas-backend:ci-local-1710124
```

Result: RKE2 pulled the RC image, all 15 existing backend deployments in
namespace `nexuspaas` rolled out to `localhost:5000/nexuspaas-backend:ci-local-1710124`
and reported `1/1` ready, `/healthz`, `/readyz`, authenticated `/metrics`, and
authenticated `/openapi.json` passed through `http://127.0.0.1:18081`, every
backend service returned `200` for its isolated `/service-registry`, the union
of registry views covered all 15 logical services, and `platform-gateway`
rollback/re-deploy completed successfully.

## 17. Rollback Plan

Revert this slice. No schema or persistent-data changes are involved.

## 18. Risks and Tradeoffs

The document-count guard is an early cost guard, not a full YAML security
validator. Full resource semantics still belong to platform preflight plus
Kubernetes admission. Use a small focused helper so handlers keep one reason to
change, and reuse existing YAML/Kubernetes packages where they reduce parser
risk.

## 19. Reviewer Checklist

| Check | Status |
|---|---|
| Requirement fit: implements `GATE-*` and K8S manifest-cap gap | Pass |
| Scope stays surgical and avoids Web UI/secret/audit/admin work | Pass |
| Uses mature existing dependencies or a justified new dependency | Pass |
| SOLID: limit validation is isolated from handlers and platform routing | Pass |
| Preserves external API envelope compatibility | Pass |
| 12-Factor: limits are config-driven with safe defaults | Pass |
| Tests cover limit acceptance and rejection paths | Pass |
| Quick gate passes after implementation | Pass |
| SonarScanner Quality Gate status | Pass |
| Risks and diff scope reviewed | Pass |

## 20. Status

Status: Implemented and reviewer-verified for this slice and rolled into the
V1 local RKE2 staging release evidence. V1 external production launch (real
registry, 8-unit topology, previous-image rollback, production secrets, remote
CI/Sonar) remains OPEN; see the First Version (V1) Status block in `problem.md`
/ `gap.md`.

Reviewer Agent: Approved and verified. The implementation directly addresses a
launch-blocking abuse-control gap, keeps the diff small, reuses existing
Kubernetes YAML parsing dependencies instead of adding a custom parser, keeps
limit validation out of route handlers, and passes quick plus Sonar Quality
Gate. The later storage, secret, audit, plan-admin, operational PDP, full RC,
and live local RKE2 staging checks are recorded in their slice plans and in the
V1 checklist below.

## V1 Acceptance Checklist

| Area | Status | Notes |
|---|---|---|
| `GATE-*` + K8S cap | RC/Live Pass | Config-driven API body caps, 429 retry guidance, ConfigFile byte/doc caps, and scheduler admission manifest caps implemented and verified. Included in final `beta-rc` and live RC rollout. |
| `STORAGE-*` | RC/Live Pass | Existing mount-plan contract enforces storage-owned PVC bindings/permissions; `StorageMountPlanResolved` and project-scoped AuditEvent now provide decision audit evidence. Included in final `beta-rc` and live RC rollout. |
| `SECRET-*` | RC/Live Pass | V1 policy uses External Secrets/Vault/runtime secret contracts, not a custom store. Raw Kubernetes `Secret` resources are rejected by scheduler admission and dispatcher defense-in-depth; rejection response/events omit plaintext values and publish safe `SecretAccessRejected`/`AuditEvent` metadata. Included in final `beta-rc` and live RC rollout. |
| `AUDIT-*` | RC/Live Pass | `AUDIT-001..004` covered: audit read/report routes are append-only through normal APIs, audit query/report RBAC is tested, retention cleanup is configured/enforced, hash-chain integrity fields are returned/exported, and `PRODUCT_NAME` drives report branding. Included in final `beta-rc` and live RC rollout. |
| `PLANADMIN-*` | RC/Live Pass | Plan/Queue routes are admin-only in service spec; Plan/Queue events now include actor plus old/new values, and Project plan binding stays owner-mediated through org-project. Included in final `beta-rc` and live RC rollout. |
| `WEB-*` | Not Applicable for V1 | V1 scope is API/CLI-first and does not advertise a NexusPaaS management Web UI. `WEB-001..007` remain required before any future Web UI launch; browser WebRTC sessions stay covered by RTC acceptance. |
| Accepted GA families | RC Pass | Final `beta-rc` passed with production-beta manifest rehearsal, integration coverage, focused E2E, 8-unit collaboration smoke, govulncheck, OSV, Trivy, and Sonar Quality Gate. |
| Live staging online | Local Live Pass — External launch OPEN | Local RKE2 namespace `nexuspaas` is running the RC image on all 15 existing backend deployments. Gateway is reachable at `http://127.0.0.1:18081` while the port-forward is active. Registry-union smoke covers all 15 logical services; gateway rollback/re-deploy passed. External beta still needs replacing the local `localhost:5000` registry with the real image registry and any planned 8-unit topology cutover window. |
