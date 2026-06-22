# Image Registry Harbor Degraded Build/List

## 1. Objective

Close or materially advance OPS-012 by making the live image-registry build and
Project image/build list APIs surface Harbor dependency outages clearly through
the existing degraded response envelope, then prove that behavior with focused
tests and a bounded live Harbor outage drill.

## 2. Background

The previous Harbor outage drill proved `/api/v1/harbor-status` reports a
retryable degraded Harbor state while `harbor-core` is unavailable. That remains
partial OPS-012 evidence because the image build and list custom handlers still
serve local records without invoking the route adapter path.

Source inspection and explorer review confirmed:

- image build submit/list/log/cancel are custom handlers;
- custom handlers run before the platform `ExternalAdapter` route path;
- `createBuild`, `listProjectImages`, and `listProjectBuilds` use local records;
- `callHarbor` already implements the platform Harbor degraded envelope.

This slice reconnects those custom handlers to the existing Harbor adapter
degraded signal. It does not build a new Harbor client or a new build executor.

## 3. Source References

- `docs/acceptance/operations.md`
- `docs/acceptance/gap-analysis.md`
- `gap.md`
- `problem.md`
- `docs/plan/2026-06-21-harbor-outage-failure-injection.md`
- `backend/internal/services/imageregistry/handler.go`
- `backend/internal/services/imageregistry/helpers.go`
- `backend/internal/services/imageregistry/handler_test.go`
- `backend/internal/services/imageregistry/helpers_coverage_test.go`
- `backend/internal/platform/app.go`
- `backend/internal/platform/response.go`
- `backend/internal/services/imageregistry/spec.go`

## 4. Assumptions

- The current Kubernetes context targets the same live RKE2 environment used for
  the earlier Harbor outage drill.
- `HARBOR_URL` is already configured in `production-beta-runtime-config`.
- `harbor-core` can be scaled from `1` to `0` and restored to `1` within the
  live drill window.
- Runtime API keys may be used inside local shell variables only; command output
  must not print key values, decoded values, hashes, auth headers, Docker
  configs, or credentials.
- The backend image is shared by backend services, so rolling
  `image-registry-service` is enough for the image-registry code path.

## 5. Non-Goals

- No new Harbor SDK/client, build executor, queue worker, sidecar, controller, or
  adapter abstraction.
- No change to Harbor deployment, credentials, backup/restore, Trivy setup, or
  registry storage.
- No new database table or migration.
- No rejection/rollback of existing local image build records.
- No frontend UI change in this slice; the degraded contract is verified at the
  product API level.
- No claim that full OPS-019 is closed; DB, K8s API, live Prometheus
  interruption, and node usage-agent failure remain separate.

## 6. Current Behavior

- `GET /api/v1/harbor-status` returns a degraded envelope when Harbor is down.
- `POST /api/v1/images/build*` creates a local `image_build_jobs` record and
  returns `202` without a Harbor degraded envelope.
- `GET /api/v1/projects/{id}/images` returns local allow-list rows without a
  Harbor degraded envelope.
- `GET /api/v1/projects/{id}/builds` and
  `GET /api/v1/projects/{id}/image-builds` return local build rows without a
  Harbor degraded envelope.

## 7. Target Behavior

- The image-registry custom handlers keep their existing auth, RBAC, local data,
  and response status behavior.
- When Harbor is healthy, the affected routes return their current data without
  a degraded envelope.
- When Harbor is unavailable or the adapter circuit is open:
  - `POST /api/v1/images/build`, `/from-storage`, and `/dockerfile` still return
    the existing build response but include `degraded.adapter="harbor"`;
  - `GET /api/v1/projects/{id}/images` returns local image rows with the same
    Harbor degraded envelope;
  - `GET /api/v1/projects/{id}/builds` and `/image-builds` return local build
    rows with the same Harbor degraded envelope.
- The degraded envelope uses existing `callHarbor` behavior and metrics.
- Ledgers record OPS-012 as build/list degraded behavior evidenced live, while
  keeping full OPS-019 open for other fault domains.

## 8. Affected Domains

- Image registry / supply-chain service.
- Harbor adapter degraded response contract.
- Live failure-injection evidence.
- Acceptance ledgers.

No microservice ownership boundary changes are introduced.

## 9. Affected Files

- `backend/internal/services/imageregistry/handler.go`
- `backend/internal/services/imageregistry/helpers.go`
- `backend/internal/services/imageregistry/handler_test.go`
- `docs/plan/2026-06-21-image-registry-harbor-degraded-build-list.md`
- `gap.md`
- `problem.md`
- `docs/acceptance/gap-analysis.md`

## 10. API / Contract Changes

The routes remain unchanged. Response bodies continue to use the existing
platform envelope.

New behavior is additive: successful HTTP responses may include the existing
top-level `degraded` object when the Harbor attached service is unavailable.
The `data` shape is unchanged.

## 11. Database / Migration Changes

None.

Existing resources remain owned by `image-registry-service`:

- `image-registry-service:image_allow_lists`
- `image-registry-service:image_build_jobs`

## 12. Configuration Changes

No source configuration change is planned. Live verification depends on the
already configured `HARBOR_URL` runtime value.

## 13. Observability Changes

No new telemetry backend is added. The slice reuses `callHarbor`, which already
increments `harbor_degraded` when a degraded adapter result is returned.

Live evidence will record:

- image tag and digest;
- rollout readiness;
- healthy baseline route responses with no degraded envelope;
- outage route responses with Harbor degraded envelope;
- restored Harbor readiness and recovery route responses;
- cleanup status for synthetic rows.

## 14. Security Considerations

- Keep existing route auth/RBAC checks before Harbor probes.
- Do not leak runtime API keys, service keys, auth headers, Kubernetes Secret
  data, decoded values, hashes, Docker configs, or credentials.
- Do not add client-controlled scan/deleted trust in this slice.
- Degraded responses must not include upstream credential material.

## 15. Implementation Steps

- [x] Add a small helper that calls the existing Harbor adapter and returns only
  the degraded pointer for image-registry build/list custom handlers.
- [x] Wire the helper into `listProjectImages` after auth/RBAC and row assembly.
- [x] Wire the helper into `listProjectBuilds` after auth/RBAC and row assembly.
- [x] Wire the helper into `createBuild` after the local build record is created,
  preserving existing status and data shape.
- [x] Add focused unit coverage for healthy and degraded Harbor behavior on
  Project image list, Project build list, and build submission. Tests must
  assert the existing `data` payloads/status codes stay unchanged in healthy and
  degraded cases, and that Harbor outage adds only the existing top-level
  `degraded` envelope metadata.
- [x] Run focused Go tests.
- [x] Run the local SonarScanner Quality Gate and record pass/fail status before
  marking the slice complete.
- [x] Build and push a timestamped backend image if tests pass.
- [x] Roll only `image-registry-service` to the new image and verify readiness.
- [x] Run a bounded live drill:
  - seed a synthetic Project/image/build row through existing APIs or exact
    test rows;
  - confirm healthy baseline for project images, project image-builds, and build
    submission;
  - scale `harbor-core` from `1` to `0`;
  - confirm those routes include Harbor degraded envelope;
  - restore `harbor-core` and confirm recovery;
  - clean exact synthetic rows.
- [x] Update ledgers with exact evidence and remaining gaps.
- [x] Submit implementation to Reviewer Agent.

## 15.1 Completed Execution Evidence

Plan review:

- Reviewer Agent requested Sonar Quality Gate coverage and explicit
  additive-degraded shape assertions.
- Revised plan was approved before implementation.

Code:

- Added `harborDegraded` in `helpers.go`, reusing the existing `callHarbor`
  adapter path.
- Wired `listProjectImages`, `listProjectBuilds`, and `createBuild` to return
  the Harbor degraded envelope after existing auth/RBAC and local data handling.
- Added `TestImageRegistryBuildAndListRoutesSurfaceHarborDegradedAdditively`.
- Expanded that test after reviewer feedback to explicitly cover the
  `/api/v1/projects/{id}/image-builds` alias and the
  `/api/v1/images/build/from-storage` build path under Harbor degraded state.

Tests:

```sh
go -C backend test ./internal/services/imageregistry -run 'Harbor|Build|CatalogRequestsAndBuildWorkflow' -count=1
go -C backend test ./internal/services/imageregistry -count=1
```

Both passed before and after the alias/from-storage test expansion.

Quality Gate:

```sh
bash backend/scripts/ci-security-gate.sh sonar
```

Initial run failed because two pre-existing Sonar hotspots were still
`TO_REVIEW`. The Harbor runtime URL hotspot was reviewed as `SAFE`; the existing
GPU desktop Dockerfile root-user hotspot was reviewed as `ACKNOWLEDGED`. The
rerun passed with `QUALITY GATE STATUS: PASSED`,
`new_coverage=81.4`, `new_violations=0`, and
`new_duplicated_lines_density=0.34458`.
After the test-only expansion, the Sonar Quality Gate was rerun and again
reported `QUALITY GATE STATUS: PASSED`.

Image and rollout:

- Built and pushed
  `localhost:5000/nexuspaas-backend:ci-ga-image-harbor-degraded-20260621211729`.
- Registry digest:
  `sha256:21345ac6ad43db05f489bfa2ee37122b0b79873daca6f04619506c8bcef9d319`.
- Rolled only `image-registry-service`.
- Ready pod:
  `image-registry-service-7f6c7df87-z2mhr` with the new digest.

Live drill:

- Trace: `ga-image-harbor-degraded-20260621212113`.
- Healthy baseline:
  - project images: HTTP `200`, success `true`, `degraded=null`, count `1`;
  - image build submit: HTTP `202`, success `true`, `degraded=null`, status
    `queued`;
  - project builds: HTTP `200`, success `true`, `degraded=null`, count `1`.
- Outage:
  - scaled `harbor-core` from `1` to `0`;
  - project images returned HTTP `200` with
    `degraded.adapter="harbor"`, `degraded.code="adapter_unavailable"`,
    `degraded.retryable=true`, count `1`;
  - project builds returned HTTP `200` with the same degraded envelope, count
    `1`;
  - image build submit returned HTTP `202` with the same degraded envelope and
    status `queued`.
- Recovery:
  - restored `harbor-core` to `1/1`;
  - after the existing circuit breaker naturally recovered, project images and
    project builds returned without a degraded envelope.
- Cleanup:
  - exact synthetic image-build, image-request, allow-list, image-project,
    project, and group rows were deleted;
  - `cleanup_leftovers=0`.

## 16. Verification Plan

Focused tests:

```sh
go -C backend test ./internal/services/imageregistry -run 'Harbor|Build|CatalogRequestsAndBuildWorkflow' -count=1
```

Backend package regression:

```sh
go -C backend test ./internal/services/imageregistry -count=1
```

Quality Gate:

```sh
bash backend/scripts/ci-security-gate.sh sonar
```

The SonarScanner result must include an explicit Quality Gate pass/fail
checkpoint before ledgers claim this OPS-012 slice is complete.

Pre-ledger hygiene:

```sh
git diff --check -- backend/internal/services/imageregistry/handler.go backend/internal/services/imageregistry/helpers.go backend/internal/services/imageregistry/handler_test.go gap.md problem.md docs/acceptance/gap-analysis.md docs/plan/2026-06-21-image-registry-harbor-degraded-build-list.md
```

Live drill:

```sh
kubectl -n nexuspaas rollout status deploy/image-registry-service --timeout=180s
kubectl -n harbor-system scale deploy/harbor-core --replicas=0
kubectl -n harbor-system rollout status deploy/harbor-core --timeout=180s
kubectl -n harbor-system scale deploy/harbor-core --replicas=1
kubectl -n harbor-system rollout status deploy/harbor-core --timeout=180s
```

The live API probes will use local shell variables for credentials and must
print only status/degraded fields, counts, trace IDs, image digest, and cleanup
counts.

## 17. Rollback Plan

- Roll `image-registry-service` back to the previously recorded image.
- Restore `harbor-core` to the original replica count if the drill is
  interrupted.
- Delete any exact synthetic rows inserted for route proof.
- Revert only this slice's source/ledger edits if Reviewer Agent rejects the
  implementation.

## 18. Risks and Tradeoffs

- Calling Harbor during list routes adds latency when Harbor is slow. The
  existing adapter timeout/retry/circuit-breaker controls bound this, and the
  route still returns local data with a degraded envelope.
- Build submission still creates the local queued record even when Harbor is
  down. This preserves current clients and avoids a larger contract break; the
  degraded envelope makes the outage explicit. A later build-executor slice can
  decide whether outage should reject or park work.
- This does not prove real image build execution in Harbor. It proves the product
  build/list API no longer hides Harbor dependency outage.

## 19. Reviewer Checklist

| Check | Status |
|---|---|
| Requirement fit: OPS-012 build/list degraded behavior | Pass |
| Scope limited to image-registry custom handlers | Pass |
| Existing REST routes/data shapes preserved | Pass |
| No new dependency or infrastructure | Pass |
| SOLID and microservice ownership preserved | Pass |
| 12-factor config preserved | Pass |
| Secrets not printed | Pass |
| Focused tests concrete | Pass |
| Live drill concrete and rollback-safe | Pass |
| Ledgers remain accurate | Pass |

## 20. Status

Status: Completed for OPS-012 build/list degraded-route proof. Reviewer Agent
implementation and ledger verification passed. Full GA remains open in
`gap.md` and `problem.md`.
