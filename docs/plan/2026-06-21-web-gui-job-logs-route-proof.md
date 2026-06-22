# Web GUI Job Logs Route Proof Slice

## 1. Objective

Add a minimal first-party GUI path for authorized users to request job logs from
the existing `GET /api/v1/jobs/{id}/logs` route, then prove the route is reached
from `/ui/` in seeded live E2E.

## 2. Background

`gap.md` still marks WEB-004 as partial because full job status/log workflow
evidence is missing. Backend `workload-service` already exposes:

- `GET /api/v1/jobs`
- `GET /api/v1/jobs/{id}`
- `POST /api/v1/jobs/{id}/cancel`
- `GET /api/v1/jobs/{id}/logs`

The current GUI Jobs table has a `Logs` column, but it displays the static text
`API ready`. That is weak evidence and should not stand in for a real route
call.

## 3. Source References

- `gap.md`
- `docs/acceptance/gap-analysis.md`
- `frontend/src/App.tsx`
- `frontend/src/api.ts`
- `frontend/src/types.ts`
- `frontend/src/App.test.tsx`
- `frontend/src/api.test.ts`
- `frontend/tests/e2e/dashboard.spec.ts`
- `backend/internal/services/workload/spec.go`
- `backend/internal/services/workload/job_access_handlers.go`

## 4. Assumptions

- The GUI should consume the existing same-origin REST/OpenAPI route. No new
  WebRPC/tRPC/gRPC transport is required for this slice.
- Live seeded E2E can prove the browser calls `GET /api/v1/jobs/{id}/logs`, but
  it may receive an empty list because there is no public API to create job log
  records.
- Empty-log rendering is useful route evidence, but it is not full real
  workload log tailing evidence.

## 5. Non-Goals

- No backend route, admission, dispatcher, or database change.
- No public log-write seed API.
- No Kubernetes workload execution proof.
- No WebRTC session launch.
- No claim that full WEB-004 is complete.

## 6. Current Behavior

- Jobs table shows job ID, status, static log text, and cancel action.
- The static log text does not call `GET /api/v1/jobs/{id}/logs`.
- Live E2E already submits and cancels a seeded job, but it does not prove the
  browser reaches the job logs route.

## 7. Target Behavior

- Jobs table exposes a small `View logs` action for each visible job.
- Clicking it calls `GET /api/v1/jobs/{id}/logs` through the frontend API
  client.
- GUI displays returned log rows or a clear empty state scoped to that job.
- Unit tests prove the API client, UI route call, active job selection, and
  empty/log row rendering.
- Live seeded E2E records `job_logs_status` and `job_logs_count` in route proof.

## 8. Affected Domains

- Web GUI Workloads panel.
- Frontend API client/types/tests.
- Live seeded E2E route proof.
- Acceptance ledgers.

## 9. Affected Files

- `frontend/src/types.ts`
- `frontend/src/api.ts`
- `frontend/src/App.tsx`
- `frontend/src/api.test.ts`
- `frontend/src/App.test.tsx`
- `frontend/tests/e2e/dashboard.spec.ts`
- `docs/plan/2026-06-21-web-gui-job-logs-route-proof.md`
- `gap.md`
- `docs/acceptance/gap-analysis.md`

## 10. API / Contract Changes

None. Existing `GET /api/v1/jobs/{id}/logs` is consumed unchanged.

## 11. Database / Migration Changes

None.

## 12. Configuration Changes

No source config change. Live verification requires rebuilding and rolling
`platform-gateway` so it serves the updated Web GUI static assets.

## 13. Observability Changes

No runtime logging or metric change. E2E logs should record only route status,
row count, and cleanup IDs; API keys must not be logged.

## 14. Security Considerations

- Keep authorization in backend `authorizedJobRecord`.
- Do not persist API keys in browser storage.
- Do not introduce a public log-write route just for tests.
- Do not list logs for jobs outside the active authorized Project.

## 15. Implementation Steps

- [x] Add `JobLogRecord` typing and `jobLogs(jobID)` to the frontend API client.
- [x] Replace the static Jobs-table log text with a `View logs` action.
- [x] Add a compact logs panel/empty state in `WorkloadsPanel`.
- [x] Update frontend unit tests for API call, UI state, and empty/log rows.
- [x] Extend seeded live E2E to click `View logs`, verify the browser `GET`
  response, and record `job_logs_status` / `job_logs_count`.
- [x] Build and roll `platform-gateway` only, then run seeded live E2E.
- [x] Update ledgers honestly: route consumption improved; real log tailing
  remains open until a workload produces logs or a supported log source exists.

## 16. Verification Plan

Focused:

```sh
npm --prefix frontend run test
npm --prefix frontend run build
```

Regression:

```sh
go -C backend test ./... -count=1
bash backend/scripts/ci-security-gate.sh quick
git diff --check
```

Live:

```sh
docker build -t localhost:5000/nexuspaas-backend:<tag> -f backend/Dockerfile .
docker push localhost:5000/nexuspaas-backend:<tag>
kubectl -n nexuspaas set image deploy/platform-gateway app=localhost:5000/nexuspaas-backend:<tag>
kubectl -n nexuspaas rollout status deploy/platform-gateway --timeout=180s
NEXUSPAAS_E2E_API_KEY=<runtime-key> NEXUSPAAS_E2E_SEED_PROJECT=true NEXUSPAAS_E2E_APP_PATH=http://127.0.0.1:18080/ui/ npm --prefix frontend run e2e
```

## 17. Rollback Plan

Roll `platform-gateway` back to the pre-slice image and verify rollout status.

## 18. Risks

- Live log list may be empty. That is acceptable for route proof but does not
  close real log tailing.
- Existing Job and cancel-command records still have no DELETE route and must
  remain explicit leftovers in live E2E cleanup.

## 19. Reviewer Checklist

| Check | Status |
|---|---|
| Requirement fit: GUI calls existing job logs route | Ready for reviewer — browser E2E clicked `View logs` and reached `GET /api/v1/jobs/{id}/logs` |
| Existing REST/OpenAPI only | Ready for reviewer — consumed existing workload route only |
| No backend route/DB/config change | Ready for reviewer — frontend/API-client/E2E only |
| API keys not persisted | Ready for reviewer — existing env/header flow unchanged |
| Empty-log evidence not overstated | Ready for reviewer — ledgers keep real log tailing open because live `job_logs_count=0` |
| Focused frontend tests/build | Ready for reviewer — `npm --prefix frontend run test` and build passed |
| Backend regression/quick gate | Ready for reviewer — full backend tests, quick gate, and `git diff --check` passed |
| Live seeded E2E | Ready for reviewer — seeded `/ui/` E2E passed with logs route proof |
| Ledger accuracy | Ready for reviewer — gap docs updated as partial WEB/E2E evidence |
| Diff scope | Ready for reviewer — scoped to frontend/API/E2E and evidence docs |

## 20. Status

Status: Approved

Implementation evidence captured on 2026-06-21:

- Focused frontend verification passed:

```sh
npm --prefix frontend run test
npm --prefix frontend run build
```

- Regression verification passed:

```sh
go -C backend test ./... -count=1
bash backend/scripts/ci-security-gate.sh quick
git diff --check
```

- Rollback baseline before this slice:

```text
platform-gateway image: localhost:5000/nexuspaas-backend:ci-ga-web-job-submit-20260621141339
```

- Built, pushed, and rolled only `platform-gateway`:

```text
image: localhost:5000/nexuspaas-backend:ci-ga-web-job-logs-20260621143553
digest: sha256:3111ba2be88c8b0cb4c344f172e468253c8b8c862930763d426117776ab1a824
rollout: deployment "platform-gateway" successfully rolled out
pod imageID: localhost:5000/nexuspaas-backend@sha256:3111ba2be88c8b0cb4c344f172e468253c8b8c862930763d426117776ab1a824
```

- Live seeded Playwright E2E passed against `http://127.0.0.1:18080/ui/`.
  Route proof:

```json
{
  "project_id": "e2e-p-mqneymza-1tqckn",
  "project_count": 1,
  "seeded_project_present": true,
  "config_file_count": 1,
  "seeded_config_id": "CFG2600007",
  "job_count": 3,
  "seeded_job_present": true,
  "job_cancel_requested": true,
  "job_cancel_command_id": "94925e294549528a2190b3dbafd09592",
  "job_logs_requested": true,
  "job_logs_status": 200,
  "job_logs_count": 0,
  "image_count": 1,
  "seeded_image_identifier": "e2e-p-mqneymza-1tqckn:nexuspaas-e2e:mqneymza-1tqckn",
  "build_count": 1,
  "gpu_status": 200,
  "gpu_ok": true
}
```

- Live cleanup:

```text
configfile=CFG2600007: HTTP 200
image_build=e2e-build-e2e-p-mqneymza-1tqckn: HTTP 200
project_image=e2e-p-mqneymza-1tqckn:nexuspaas-e2e:mqneymza-1tqckn: HTTP 200
plan=e2e-plan-mqneymza-1tqckn: HTTP 200
queue=e2e-q-mqneymza-1tqckn: HTTP 200
project=e2e-p-mqneymza-1tqckn: HTTP 200
group=e2e-g-mqneymza-1tqckn: HTTP 200
leftovers: image_request=e2e-img-e2e-p-mqneymza-1tqckn has no DELETE route;
  job=e2e-job-mqneymza-1tqckn has no DELETE route;
  job_cancel_command=94925e294549528a2190b3dbafd09592 has no DELETE route
```

The empty `job_logs_count=0` is intentional evidence of route consumption and
empty-state rendering only. It does not close real workload log tailing.

Final implementation review: Approved by Reviewer Agent; no blocking findings.
Reviewer reran `npm --prefix frontend run test`, `npm --prefix frontend run
build`, and `git diff --check`. Residual risks remain explicit in the ledgers:
`job_logs_count=0` proves route consumption/empty state only, and seeded image
request, Job, and cancel-command records still have no DELETE route.

Plan Agent checklist:

- [x] Requirement restated.
- [x] Existing backend route identified.
- [x] Scope limited to frontend/API-client/E2E evidence.
- [x] No new backend/API contract.
- [x] Empty-live-log caveat recorded.
- [x] Reviewer Agent approval received.
- [x] Code Agent implementation complete.
- [x] Reviewer Agent final implementation approval received.
