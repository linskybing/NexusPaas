# Production Beta Live Rehearsal Harness P0

## 1. Objective

Create a minimal, production-beta live staging rehearsal harness plan for
P0.2-P0.5 without running live staging actions in the current Docker Desktop
context.

The later Code Agent should add an opt-in script at
`backend/scripts/production-beta-live-rehearsal.sh`, add regression tests and
operator docs, and update `problem.md` / `gap.md` only for Remote Sonar
evidence. The harness must gather evidence for external image promotion,
registry scan status, production secret presence, migration apply/validate,
8-unit deploy/smoke, per-unit rollback, and per-unit redeploy while keeping live
P0.2-P0.5 open until actual staging evidence is reviewed.

## 2. Background

`gap.md` and `problem.md` both state that V1 external production launch remains
OPEN because external registry promotion/rollback, 8-unit staging
deploy/smoke/rollback, production secrets, live staging DB migration drills, and
remote CI/Sonar evidence are not all proven.

The existing `backend/scripts/ci-security-gate.sh beta-rc` path is intentionally
non-live. It renders `kubectl kustomize backend`, performs client dry-runs,
writes a rollback command plan, and proves local/runtime smoke behavior. It does
not mutate a real staging cluster.

This task should add the missing live rehearsal harness, but the Code Agent must
not execute live actions from the current Docker Desktop context.

## 3. Source References

- `docs/agents/workflow.md`
- `docs/agents/planning.md`
- `docs/agents/review-checklist.md`
- `problem.md`
- `gap.md`
- `backend/scripts/ci-security-gate.sh`
- `backend/internal/platform/deployment_test.go`
- `backend/docs/beta-launch-readiness.md`
- `backend/docs/e2e-testing.md`
- `backend/kustomization.yaml`
- `backend/deploy/k3s/production-beta/backend-units.yaml`
- `docs/architecture/testing-strategy.md`
- `docs/adr/0004-deployment-evidence-gates.md`
- `docs/architecture/service-boundaries.md`
- Kubernetes documentation, fetched through Context7 from `/kubernetes/website`:
  - `kubectl kustomize DIR` renders a kustomization directory before apply.
  - `kubectl wait --for=condition=complete --timeout=... job/<name>` waits for
    a Job to complete.
  - `kubectl set image deployment/<name> <container>=<image>` updates a
    Deployment container image.
  - `kubectl rollout status deployment/<name>` watches rollout completion.

## 4. Assumptions

- A real staging kube context will be provided later and is not the current
  Docker Desktop context.
- `KUBE_CONTEXT` is mandatory for live runs; the script must verify the active
  `kubectl config current-context` equals that exact value before any live
  mutation.
- The staging namespace is `nexuspaas` unless overridden by `NAMESPACE`.
- The production-beta backend units are exactly:
  `platform-gateway`, `iam-unit`, `tenant-unit`, `collaboration-unit`,
  `platform-io-unit`, `usage-observability`, `compute-api`, and
  `compute-control-plane`.
- Each backend unit Deployment has an `app` container.
- The candidate backend image is a single immutable image that can run all 8
  physical backend units.
- `crane` may already exist on an operator machine; the implementation must not
  vendor or install it.
- The external Sonar provider can publish PR comments/checks outside the backend
  GitHub Actions workflow.
- `problem.md` and `gap.md` may change only if there is reviewed Remote Sonar
  evidence; they must continue to show live P0.2-P0.5 as open.

## 5. Non-Goals

- Do not execute live staging actions now.
- Do not close live P0.2-P0.5 in `problem.md` or `gap.md`.
- Do not claim V1 external production launch is complete.
- Do not make public API, OpenAPI schema, frontend, or service route changes.
- Do not add database schema migrations.
- Do not add a new dependency, package manager entry, or vendored tool. The only
  allowed external tool is an optional pre-existing `crane` CLI.
- Do not print Kubernetes Secret values, encoded values, decoded values, or
  hashes.
- Do not introduce a new deployable unit or change service boundaries.

## 6. Current Behavior

- `ci-security-gate.sh beta-rc` performs non-live production-beta manifest
  rehearsal and local smoke checks.
- Production-beta kustomize output is expected to contain 8 backend units, omit
  the all-in-one `platform` Deployment, and contain no `-dev-` references.
- Existing docs require live staging deploy/smoke/rollback/redeploy evidence
  before external launch readiness can be claimed.
- `problem.md` and `gap.md` still list external production launch evidence as
  open.

## 7. Target Behavior

Add `backend/scripts/production-beta-live-rehearsal.sh` with these behaviors:

- Exit before any live mutation unless `LIVE_STAGING_REHEARSAL=1`.
- Require `KUBE_CONTEXT`, verify `kubectl config current-context` equals
  `KUBE_CONTEXT`, pass `--context "$KUBE_CONTEXT"` to every live `kubectl`
  command, and refuse Docker Desktop, local, localhost, loopback, kind, and
  minikube style contexts unless a reviewer-approved future exception is
  explicitly added.
- Require `CANDIDATE_IMAGE` to be immutable, non-localhost, and digest-pinned
  with `@sha256:<64 lowercase hex digest>`.
- If both `SOURCE_IMAGE` and `PROMOTED_IMAGE_TAG` are set, require an existing
  `crane` command and run a promotion copy. Otherwise require
  `PROMOTION_EVIDENCE`.
- Require either `REGISTRY_SCAN_STATUS` or `REGISTRY_SCAN_EVIDENCE`.
- Render `kubectl kustomize backend` and verify the render contains the 8
  backend unit Deployments, does not contain Deployment `platform`, and contains
  no `-dev-` references.
- Record previous `app` container image per backend unit before mutation.
- Check only required Secret names with `kubectl --context "$KUBE_CONTEXT" get
  secret <name> -o name`; record presence/absence only.
- Create migration apply and validate Jobs using the candidate image with
  `ADMIN_TASK=apply-migrations` and `ADMIN_TASK=validate-migrations`; wait with
  `kubectl --context "$KUBE_CONTEXT" wait --for=condition=complete
  --timeout=... job/<name>`.
- Apply candidate manifests and set each unit image with
  `kubectl --context "$KUBE_CONTEXT" set image deployment/<unit>
  app=<candidate-image>`.
- Wait for all 8 units with `kubectl --context "$KUBE_CONTEXT" rollout status
  deployment/<unit>`.
- Smoke `/healthz`, `/readyz`, and `/metrics` directly for every backend unit.
- Smoke gateway `/openapi.json` and `/service-registry`; verify
  `/service-registry` reports 15 logical services.
- For each unit, roll back to the recorded previous image, wait for rollout,
  smoke, redeploy the candidate image, wait for rollout, and smoke again.
- Write an artifact report covering image, digest, promotion evidence, scan
  evidence, secret presence, migration Jobs, deploy, smoke, rollback, and
  redeploy.

## 8. Affected Domains

- Release/staging operations for production-beta.
- External registry promotion evidence for P0.2.
- Production secret presence verification for P0.3.
- Migration runner admin Jobs for P0.4.
- 8-unit deploy/smoke/rollback/redeploy evidence for P0.5.
- CI/release documentation and text-level regression tests.
- Remote Sonar evidence wording in `problem.md` / `gap.md`, if evidence exists.

## 9. Affected Files

Code Agent may edit:

- `backend/scripts/production-beta-live-rehearsal.sh` (new)
- `backend/internal/platform/deployment_test.go`
- `backend/docs/beta-launch-readiness.md`
- `backend/docs/e2e-testing.md`
- `problem.md` only for Remote Sonar evidence wording
- `gap.md` only for Remote Sonar evidence wording

Plan Agent edits only:

- `docs/plan/2026-06-23-live-rehearsal-harness-p0.md`

Files explicitly protected from this task:

- Public API contract fixtures under `backend/internal/contracts/fixtures/`
- Frontend files under `frontend/`
- Service implementation files under `backend/internal/services/`
- Runtime platform implementation files except
  `backend/internal/platform/deployment_test.go`
- Kubernetes production-beta manifests under `backend/deploy/`
- Database migrations under `backend/**/migrations/`
- `.github/workflows/backend-quality-gate.yml`

## 10. API / Contract Changes

No public API, route, request/response, OpenAPI, frontend, event schema, or
internal service contract changes.

The only new operator contract is the script interface:

- Required:
  - `LIVE_STAGING_REHEARSAL=1`
  - `KUBE_CONTEXT=<expected real staging context>`
  - `CANDIDATE_IMAGE=<external-registry>/<repo>@sha256:<64-lowercase-hex-digest>`
  - `REGISTRY_SCAN_STATUS=<status>` or `REGISTRY_SCAN_EVIDENCE=<path-or-url>`
- Required unless promotion is performed by the script:
  - `PROMOTION_EVIDENCE=<path-or-url>`
- Optional:
  - `SOURCE_IMAGE=<source image>`
  - `PROMOTED_IMAGE_TAG=<external promoted tag>`
  - `NAMESPACE=nexuspaas`
  - `ARTIFACT_DIR=<artifact output directory>`
  - timeout variables for Jobs, rollout, and smoke checks

## 11. Database / Migration Changes

No schema changes and no new migration files.

The harness creates short-lived Kubernetes Jobs that run existing admin tasks:

- `ADMIN_TASK=apply-migrations`
- `ADMIN_TASK=validate-migrations`

These Jobs must use the same runtime config and Secret references as the
production-beta units. The script must wait for completion and record only Job
names, status, timestamps, and relevant non-secret output.

## 12. Configuration Changes

No application runtime configuration changes.

The new script reads operator-supplied environment variables only. It must fail
closed when required variables are missing, when `CANDIDATE_IMAGE` is mutable or
local, when `KUBE_CONTEXT` is missing, when `kubectl config current-context`
does not exactly equal `KUBE_CONTEXT`, when the kube context looks local, or
when registry scan/promotion evidence is missing.

## 13. Observability Changes

No service logging, metric, or trace implementation changes.

The script should create an evidence directory containing:

- rendered manifest metadata and validation result;
- previous image map per unit;
- candidate image and digest;
- registry promotion and scan evidence;
- Secret name presence matrix;
- migration Job names and completion status;
- rollout status per unit;
- smoke status per endpoint;
- rollback and redeploy status per unit;
- final markdown report.

## 14. Security Considerations

- The `LIVE_STAGING_REHEARSAL=1` guard must run before any command that mutates
  the cluster or external registry.
- `KUBE_CONTEXT` is mandatory for live runs. The script must verify
  `kubectl config current-context` equals `KUBE_CONTEXT` before any live
  mutation, reject local-style contexts, and pass `--context "$KUBE_CONTEXT"` to
  every live `kubectl` command.
- The script must reject `localhost`, `127.0.0.1`, `::1`, `host.docker.internal`,
  `localhost:<port>`, and local registry-style candidate images.
- The script must require `@sha256:<64 lowercase hex digest>` in
  `CANDIDATE_IMAGE`; tags, empty digests, short digests, and non-hex digests are
  not acceptable.
- Secret checks must use only names and existence. Do not run `kubectl get
  secret -o yaml`, do not read `.data`, and do not print hashes.
- Shell output must avoid `set -x`.
- Artifacts must not include credentials, bearer tokens, API keys, Secret data,
  kubeconfig contents, or registry auth material.
- Rollback must use the recorded previous image for each unit, not a guessed tag.

## 15. Implementation Steps

1. Confirm worktree state and preserve unrelated changes.
2. Add `backend/scripts/production-beta-live-rehearsal.sh` with
   `set -Eeuo pipefail`, no `set -x`, shared helpers for `die`, `log`,
   `need_cmd`, artifact writing, and cleanup.
3. Implement preflight guards before live mutation:
   - require `LIVE_STAGING_REHEARSAL=1`;
   - require `kubectl`, `curl`, `awk`, `sed`, `grep`, `sort`, and `wc`;
   - require `KUBE_CONTEXT`;
   - verify `kubectl config current-context` exactly equals `KUBE_CONTEXT`;
   - reject local kube contexts, especially Docker Desktop, kind, and minikube;
   - ensure every live cluster `kubectl` call includes
     `--context "$KUBE_CONTEXT"`;
   - require immutable non-local `CANDIDATE_IMAGE` with
     `@sha256:<64 lowercase hex digest>`;
   - require promotion evidence or perform optional `crane copy`;
   - require registry scan status/evidence.
4. Define the 8 backend unit list and 15 logical service map in the script,
   matching existing `ci-security-gate.sh` names.
5. Render `kubectl kustomize backend` to the artifact directory and validate:
   - exactly the 8 expected backend unit Deployments are present;
   - Deployment `platform` is absent;
   - `-dev-` is absent;
   - the render is client-applicable before live apply.
6. Discover required Secret names from the expected production-beta contract and
   rendered manifests. For each name, run only `kubectl --context
   "$KUBE_CONTEXT" -n "$NAMESPACE" get secret "$name" -o name` and record
   yes/no presence.
7. Record previous `app` images for each unit with `kubectl --context
   "$KUBE_CONTEXT" -n "$NAMESPACE" get deployment` jsonpath and store the map
   in artifacts.
8. Create and apply migration Job manifests for apply and validate using the
   candidate image and existing production-beta runtime config/Secret refs.
   Wait with `kubectl --context "$KUBE_CONTEXT" -n "$NAMESPACE" wait
   --for=condition=complete --timeout=... job/<name>`.
9. Apply the rendered production-beta manifests, then set candidate images with
   `kubectl --context "$KUBE_CONTEXT" -n "$NAMESPACE" apply -f ...` and
   `kubectl --context "$KUBE_CONTEXT" -n "$NAMESPACE" set image
   deployment/<unit> app="$CANDIDATE_IMAGE"`.
10. For each unit, run `kubectl --context "$KUBE_CONTEXT" -n "$NAMESPACE"
    rollout status deployment/<unit> --timeout=...`.
11. Implement direct unit smoke by port-forwarding each unit Service or another
    existing Kubernetes-native direct path, then checking `/healthz`, `/readyz`,
    and `/metrics` with `curl -fsS`.
12. Implement gateway smoke for `/openapi.json` and `/service-registry`; count
    15 logical service names without requiring `jq`.
13. For each unit, perform rollback/redeploy:
    - `kubectl --context "$KUBE_CONTEXT" -n "$NAMESPACE" set image deployment/<unit> app=<previous-image>`;
    - `kubectl --context "$KUBE_CONTEXT" -n "$NAMESPACE" rollout status deployment/<unit>`;
    - run unit smoke;
    - `kubectl --context "$KUBE_CONTEXT" -n "$NAMESPACE" set image deployment/<unit> app="$CANDIDATE_IMAGE"`;
    - `kubectl --context "$KUBE_CONTEXT" -n "$NAMESPACE" rollout status deployment/<unit>`;
    - run unit smoke again.
14. Write a final markdown report with every required evidence category and an
    explicit statement that successful harness execution still needs Reviewer
    acceptance before `problem.md` / `gap.md` can close live P0.2-P0.5.
15. Add regression assertions in `backend/internal/platform/deployment_test.go`
    verifying guard strings, immutable image checks, non-local image rejection,
    `crane copy` optionality, registry scan evidence requirement, kustomize
    render checks, secret-name-only checks, migration Job admin tasks, kubectl
    wait/set-image/rollout patterns, 8-unit rollout, 15-service registry check,
    rollback/redeploy loop, and artifact report fields.
16. Update `backend/docs/beta-launch-readiness.md` and
    `backend/docs/e2e-testing.md` to document the new script as an operator-only
    live staging rehearsal that must not be run from local Docker Desktop.
17. Update `problem.md` and `gap.md` only if PR comments/checks provide Remote
    Sonar evidence. Keep P0.2-P0.5 open and keep V1 external production launch
    OPEN.
18. Review the diff and remove any accidental edits outside the affected file
    list.

## 16. Verification Plan

Code Agent must run non-live verification only:

```sh
bash -n backend/scripts/production-beta-live-rehearsal.sh
LIVE_STAGING_REHEARSAL=0 bash backend/scripts/production-beta-live-rehearsal.sh
go -C backend test ./internal/platform -run 'ProductionBeta|ReleaseCandidate|LiveRehearsal|Sonar' -count=1
bash -n backend/scripts/ci-security-gate.sh
kubectl kustomize backend >/tmp/nexuspaas-production-beta-render.yaml
git diff --check
```

The `LIVE_STAGING_REHEARSAL=0` command must fail before any live mutation. If
the implementation requires other variables before reaching the guard, that is a
bug.

Protected-file checks:

```sh
git diff --name-only
git diff --name-only | rg '^(frontend/|backend/internal/contracts/fixtures/|backend/deploy/|backend/(.*/)?migrations/|\.github/workflows/backend-quality-gate\.yml$)'
git diff --name-only | rg '^backend/internal/(services|platform)/' | rg -v '^backend/internal/platform/deployment_test.go$'
rg -in 'P0\.[2-5].*(closed|complete|done|passed)|V1 external production launch.*passed' problem.md gap.md
rg -n 'kubectl get secret .*-(o yaml|o json|jsonpath=.*data)|base64|sha256sum|shasum|set -x' backend/scripts/production-beta-live-rehearsal.sh
```

Expected results:

- The protected-file `rg` commands return no matches.
- The P0 ledger `rg` command returns no matches.
- The secret-safety `rg` command returns no matches, except if `sha256` appears
  only inside the candidate image digest validation text and not as a hashing
  command.

Operator-only live command shape for a later approved staging run, not to be run
by the Code Agent in the current context:

```sh
LIVE_STAGING_REHEARSAL=1 \
KUBE_CONTEXT=<real-staging-context> \
NAMESPACE=nexuspaas \
CANDIDATE_IMAGE=registry.example.com/nexuspaas/backend@sha256:<64-lowercase-hex-digest> \
PROMOTION_EVIDENCE=<promotion-evidence-url-or-path> \
REGISTRY_SCAN_STATUS=Success \
bash backend/scripts/production-beta-live-rehearsal.sh
```

If the operator wants the script to perform promotion:

```sh
LIVE_STAGING_REHEARSAL=1 \
KUBE_CONTEXT=<real-staging-context> \
NAMESPACE=nexuspaas \
SOURCE_IMAGE=<source-image> \
PROMOTED_IMAGE_TAG=<external-registry>/<repo>:<tag> \
CANDIDATE_IMAGE=<external-registry>/<repo>@sha256:<64-lowercase-hex-digest> \
REGISTRY_SCAN_EVIDENCE=<scan-evidence-url-or-path> \
bash backend/scripts/production-beta-live-rehearsal.sh
```

PR checks/comments verification after push:

- Confirm backend workflow checks pass.
- Confirm the external SonarCloud/SonarQube PR check or comment is present.
- Confirm any `problem.md` / `gap.md` update references only Remote Sonar
  evidence.
- Confirm P0.2 external registry promotion, P0.3 production secrets, P0.4
  staging migration drill, and P0.5 8-unit staging deploy/smoke/rollback remain
  open until a real live harness artifact is reviewed.

## 17. Rollback Plan

If the new harness or docs are incorrect, revert:

- `backend/scripts/production-beta-live-rehearsal.sh`
- `backend/internal/platform/deployment_test.go`
- `backend/docs/beta-launch-readiness.md`
- `backend/docs/e2e-testing.md`
- any Remote Sonar-only edits in `problem.md` / `gap.md`

No runtime service rollback is needed because the Code Agent must not run live
cluster mutations in this task. For future live staging runs, the script itself
must preserve the previous image map and redeploy/rollback evidence so operators
can restore any unit with:

```sh
kubectl --context "$KUBE_CONTEXT" -n nexuspaas set image deployment/<unit> app=<recorded-previous-image>
kubectl --context "$KUBE_CONTEXT" -n nexuspaas rollout status deployment/<unit>
```

## 18. Risks and Tradeoffs

- A shell harness is intentionally simple, but shell parsing must stay strict
  because this script controls live staging mutation.
- Without a real staging run, tests can prove guardrails and command contracts,
  not actual external launch readiness.
- Optional `crane copy` avoids adding a dependency, but operators must install
  and authenticate `crane` themselves if they want the script to perform
  promotion.
- Counting service-registry entries without `jq` keeps dependencies minimal but
  is less expressive than structured JSON validation.
- Port-forward based smoke is pragmatic and direct, but it depends on local
  operator network access to the staging API server.
- Ledger updates for Remote Sonar can be confused with live P0 closure; the
  implementation must keep those concerns separate.

## 19. Reviewer Checklist

- [ ] Requirement fit: every required harness behavior appears in the script,
      tests, and docs.
- [ ] Scope control: no implementation files changed outside the approved list.
- [ ] Non-goals: no live execution now and no public API/schema/frontend changes.
- [ ] Kubernetes docs alignment: Job completion uses
      `kubectl wait --for=condition=complete job/...`, image changes use
      `kubectl set image deployment/<unit> app=<image>`, rollouts use
      `kubectl rollout status`, and render validation uses `kubectl kustomize`.
- [ ] Context safety: `KUBE_CONTEXT` is required, current context must match it,
      local-style contexts are rejected, and every live `kubectl` command passes
      `--context "$KUBE_CONTEXT"`.
- [ ] Image safety: `CANDIDATE_IMAGE` requires a non-local
      `@sha256:<64 lowercase hex digest>` digest.
- [ ] Registry evidence: promotion evidence or optional `crane copy` is required,
      and scan status/evidence is required.
- [ ] Secret hygiene: only Secret names are checked; no values, hashes, or
      encoded data can appear in output or artifacts.
- [ ] Migration evidence: apply and validate Jobs use
      `ADMIN_TASK=apply-migrations` and `ADMIN_TASK=validate-migrations`.
- [ ] Deploy evidence: all 8 backend units are rendered, deployed, and waited.
- [ ] Smoke evidence: `/healthz`, `/readyz`, `/metrics` per unit, plus gateway
      `/openapi.json` and `/service-registry` with 15 logical services.
- [ ] Rollback evidence: every unit rolls back to recorded previous image,
      smokes, redeploys candidate, and smokes again.
- [ ] Artifact evidence: final report includes image/digest/scan/secret
      presence/migration/deploy/smoke/rollback/redeploy.
- [ ] Ledger safety: `problem.md` / `gap.md` change only for Remote Sonar
      evidence and keep live P0.2-P0.5 open.
- [ ] Verification: non-live commands, protected-file checks, and PR
      comments/checks are recorded.
- [ ] SOLID / 12-Factor: script is cohesive, config is environment-driven,
      admin processes run as one-off Jobs, and no secrets are committed.

## 20. Status

Status: Draft
