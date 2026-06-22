# Web GUI Job Submit Cancel Live E2E Slice

## 1. Objective

Close the remaining WEB/E2E evidence gap for live job submit/cancel by adding a
small first-party GUI job submit form that uses the existing workload REST API,
then proving a seeded live Project can submit a job through `/ui/`, display it,
and request cancel through the existing GUI cancel action.

## 2. Background

The current Web GUI can list active-Project jobs and request cancel for visible
jobs, but it only has a ConfigFile submit form. `gap.md` still marks live job
submit/cancel coverage as open.

Live direct probes on 2026-06-21 showed:

- A temporary Project without an active plan returns `403` from
  `POST /api/v1/jobs` with `project has no active resource plan`.
- A temporary Project with Queue/Plan/bind still returns `403` if the submitted
  `user_id` has no project access.
- A temporary Project created with `personal_user_id=e2e-admin`, plus a seeded
  Queue, Plan, and plan binding, returns `201` from `POST /api/v1/jobs` and
  `202` from `POST /api/v1/jobs/{id}/cancel`; Queue, Plan, Project, and Group
  cleanup returned `HTTP 200`.

The smallest useful slice is therefore frontend/API-client/E2E work using
existing backend routes and seed data. No backend route or scheduler behavior
change is needed for this evidence.

## 3. Source References

- `gap.md`
- `docs/acceptance/gap-analysis.md`
- `frontend/src/App.tsx`
- `frontend/src/api.ts`
- `frontend/src/types.ts`
- `frontend/src/App.test.tsx`
- `frontend/tests/e2e/dashboard.spec.ts`
- `backend/internal/services/workload/spec.go`
- `backend/internal/services/workload/job_submit.go`
- `backend/internal/services/workload/job_access_handlers.go`
- `backend/internal/services/schedulerquota/spec.go`
- `backend/internal/services/schedulerquota/admission_quota.go`

## 4. Assumptions

- GUI job submit should use `POST /api/v1/jobs`; cancel should keep using
  `POST /api/v1/jobs/{id}/cancel`.
- The live seeded user can be represented as `e2e-admin` by setting
  `personal_user_id` on the seeded Project, matching the direct probe.
- Queue/Plan/bind setup belongs only in live E2E seed data; production UI should
  not create plans or queues implicitly.
- The job submit form should be intentionally small: job id, user id, queue
  name, CPU, and memory.

## 5. Non-Goals

- No new backend route or API contract.
- No scheduler/admission semantics change.
- No Kubernetes workload execution or dispatcher proof.
- No WebRTC session launch.
- No nonzero GPU telemetry proof.
- No automatic Plan/Queue provisioning from the UI.

## 6. Current Behavior

- GUI can submit ConfigFiles.
- GUI lists jobs filtered to the active Project.
- GUI can request cancel for a visible cancelable job.
- GUI cannot submit a job, so live E2E cannot exercise submit/cancel through the
  browser.

## 7. Target Behavior

- GUI exposes a compact `Submit Job` form inside the Workloads panel.
- Submit sends a typed payload to `POST /api/v1/jobs` with the active Project.
- Submitted job appears in the active Project Jobs table after refresh.
- Existing cancel button can request cancel for the submitted job.
- Live seeded E2E creates Queue/Plan/binding with existing admin routes, submits
  the job through the GUI, cancels through the GUI, records route proof, and
  cleans up seeded resources.

## 8. Affected Domains

- Web GUI Workloads panel.
- Frontend API client/types/tests.
- Live seeded E2E setup/cleanup.
- Acceptance ledgers.

## 9. Affected Files

- `frontend/src/App.tsx`
- `frontend/src/api.ts`
- `frontend/src/types.ts`
- `frontend/src/App.test.tsx`
- `frontend/tests/e2e/dashboard.spec.ts`
- `docs/plan/2026-06-21-web-gui-job-submit-cancel-live-e2e.md`
- `gap.md`
- `docs/acceptance/gap-analysis.md`

## 10. API / Contract Changes

None. Existing REST/OpenAPI routes are consumed unchanged.

## 11. Database / Migration Changes

None.

## 12. Configuration Changes

No source config change. Live verification requires rebuilding and rolling
`platform-gateway` so it serves the updated Web GUI static assets.

## 13. Observability Changes

No runtime logging or metric change. E2E logs should record job submit/cancel
statuses and cleanup without logging API keys.

## 14. Security Considerations

- Do not store API keys in browser storage.
- Do not add admin-only Plan/Queue creation to the GUI.
- Keep job submit/cancel authorization entirely in backend workload and
  scheduler admission.
- Live E2E seed data must track all created IDs. Queue, Plan, Project, and
  Group must be cleaned through existing APIs; submitted Job and cancel command
  records must be recorded as explicit leftovers unless an existing cleanup route
  is proven.

## 15. Implementation Steps

- [x] Add `JobSubmitPayload` and `submitJob` to the frontend API client.
- [x] Add a compact `Submit Job` form to `WorkloadsPanel`, using the active
  Project and controlled inputs for job/user/queue/CPU/memory.
- [x] Update frontend unit tests to prove submit payload shape, active-Project
  scoping, refresh, and cancel behavior.
- [x] Extend seeded live E2E setup with optional Queue/Plan/bind seed and
  `personal_user_id=e2e-admin` on the seeded Project, tracking every created
  Queue, Plan, Project, Group, Job, and cancel command ID.
- [x] Extend live E2E to submit the job through the GUI, verify the job row,
  request cancel through the GUI, and record route proof.
- [x] Extend live cleanup to delete Plan, Queue, Project, and Group through
  existing APIs, rely on Plan deletion to clear plan binding, and record Job /
  cancel-command leftovers explicitly when no DELETE route exists.
- [x] Build and roll `platform-gateway` only, then run seeded live E2E.
- [x] Update ledgers honestly.

## 16. Verification Plan

Focused:

```sh
npm --prefix frontend run test
npm --prefix frontend run build
```

Regression:

```sh
go -C backend test ./... -count=1
go -C backend test ./... -coverprofile=coverage.out -count=1
bash backend/scripts/ci-security-gate.sh quick
bash backend/scripts/ci-security-gate.sh sonar
git diff --check
```

Live:

```sh
kubectl -n nexuspaas get deploy platform-gateway -o jsonpath='{.spec.template.spec.containers[0].image}'
docker build -t localhost:5000/nexuspaas-backend:<tag> -f backend/Dockerfile .
docker push localhost:5000/nexuspaas-backend:<tag>
kubectl -n nexuspaas set image deploy/platform-gateway app=localhost:5000/nexuspaas-backend:<tag>
kubectl -n nexuspaas rollout status deploy/platform-gateway --timeout=180s
NEXUSPAAS_E2E_API_KEY=<runtime-key> NEXUSPAAS_E2E_SEED_PROJECT=true NEXUSPAAS_E2E_APP_PATH=http://127.0.0.1:18080/ui/ npm --prefix frontend run e2e
```

If SonarScanner configuration or credentials are unavailable, record the exact
failure mode as `Not Run` in the plan and ledgers. Final GA completion remains
pending required Sonar Quality Gate evidence.

## 17. Rollback Plan

Roll `platform-gateway` back to the image recorded before this slice and verify
`kubectl -n nexuspaas rollout status deploy/platform-gateway --timeout=180s`.

## 18. Risks and Tradeoffs

- The UI form proves control-plane job submit/cancel, not Kubernetes execution.
- Live seed creates Plan/Queue resources only for E2E and must clean them.
- If live submit/cancel fails despite the direct probe, stop and record raw
  statuses before changing backend authorization/admission.

## 19. Reviewer Checklist

| Check | Status |
|---|---|
| Requirement fit: live GUI job submit/cancel | Ready for reviewer — live `/ui/` E2E submitted `e2e-job-mqne8xhu-2qx5i8`, listed it, and requested cancel |
| Existing REST/OpenAPI only | Ready for reviewer — frontend uses existing `POST /api/v1/jobs` and `POST /api/v1/jobs/{id}/cancel` |
| No DB/migration/config change | Ready for reviewer — no backend schema or runtime config source change in this slice |
| Backend auth/admission unchanged | Ready for reviewer — live seed satisfies existing Project access and Plan/Queue admission; backend behavior unchanged |
| API keys not persisted | Ready for reviewer — runtime key stays in E2E env/header only and logs omit it |
| Focused frontend tests/build | Ready for reviewer — `npm --prefix frontend run test`, `npm --prefix frontend run build`, and E2E build passed |
| Backend regression/quick/Sonar | Ready for reviewer — full Go tests, coverage run, quick gate, local Sonar Quality Gate, and `git diff --check` passed |
| Live seeded E2E | Ready for reviewer — seeded Playwright E2E passed against rolled gateway image |
| Seed cleanup and explicit leftovers | Ready for reviewer — ConfigFile, image build, Project image, Plan, Queue, Project, and Group cleaned; image request, Job, and cancel-command leftovers recorded because no DELETE route exists |
| Ledger accuracy | Ready for reviewer — ledgers updated as partial GA, not full WEB/E2E completion |
| Diff scope | Ready for reviewer — scoped to frontend API/UI/tests/E2E and evidence ledgers |

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
go -C backend test ./... -coverprofile=coverage.out -count=1
bash backend/scripts/ci-security-gate.sh quick
bash backend/scripts/ci-security-gate.sh sonar
git diff --check
```

- Rollback baseline before this slice:

```text
platform-gateway image: localhost:5000/nexuspaas-backend:ci-ga-gateway-proxy-adapter-20260621054757
platform-gateway imageID: localhost:5000/nexuspaas-backend@sha256:3cda2888dda836a1cd197c476c31342dd7e2f6f6befe5fa7e785ab46d13bc700
```

- Built, pushed, and rolled only `platform-gateway`:

```text
image: localhost:5000/nexuspaas-backend:ci-ga-web-job-submit-20260621141339
digest: sha256:aee156e2904e03d30dc3d671545a1b9e86e45e27092a03645427663d2544ccc4
rollout: deployment "platform-gateway" successfully rolled out
pod imageID: localhost:5000/nexuspaas-backend@sha256:aee156e2904e03d30dc3d671545a1b9e86e45e27092a03645427663d2544ccc4
```

- Live seeded Playwright E2E passed against `http://127.0.0.1:18080/ui/`.
  Route proof:

```json
{
  "project_id": "e2e-p-mqne8xhu-2qx5i8",
  "project_count": 1,
  "seeded_project_present": true,
  "config_file_count": 1,
  "seeded_config_id": "CFG2600006",
  "job_count": 2,
  "seeded_job_present": true,
  "job_cancel_requested": true,
  "job_cancel_command_id": "3a9c35be839dc1e5982346c9c7abaf33",
  "image_count": 1,
  "seeded_image_identifier": "e2e-p-mqne8xhu-2qx5i8:nexuspaas-e2e:mqne8xhu-2qx5i8",
  "build_count": 1,
  "gpu_status": 200,
  "gpu_ok": true
}
```

- Live cleanup:

```text
configfile=CFG2600006: HTTP 200
image_build=e2e-build-e2e-p-mqne8xhu-2qx5i8: HTTP 200
project_image=e2e-p-mqne8xhu-2qx5i8:nexuspaas-e2e:mqne8xhu-2qx5i8: HTTP 200
plan=e2e-plan-mqne8xhu-2qx5i8: HTTP 200
queue=e2e-q-mqne8xhu-2qx5i8: HTTP 200
project=e2e-p-mqne8xhu-2qx5i8: HTTP 200
group=e2e-g-mqne8xhu-2qx5i8: HTTP 200
leftovers: image_request=e2e-img-e2e-p-mqne8xhu-2qx5i8 has no DELETE route;
  job=e2e-job-mqne8xhu-2qx5i8 has no DELETE route;
  job_cancel_command=3a9c35be839dc1e5982346c9c7abaf33 has no DELETE route
```

Final implementation review: Approved by Reviewer Agent; no blocking findings.
Reviewer reran `npm --prefix frontend run test`, `npm --prefix frontend run
build`, `go -C backend test ./... -count=1`, and `git diff --check`.
Residual risks remain explicitly scoped to the broader GA backlog: minimal
control-plane job proof only, manual user/queue fields until OIDC/current-user
UX is complete, and no DELETE routes for Job, image request, or cancel-command
records.

Plan Agent checklist:

- [x] Requirement restated.
- [x] Live blocker and successful seed pattern captured.
- [x] Scope limited to frontend/API-client/E2E evidence.
- [x] Existing REST/OpenAPI contract preserved.
- [x] Reviewer Agent approval required before code changes.
- [x] Reviewer Agent approval received.
- [x] Code Agent implementation complete.
- [x] Reviewer Agent final implementation approval received.
