# Queue Duration Deployment Cleanup And Auto Preemption Live E2E

## 1. Objective

Implement queue-owned runtime duration limits and automatic quota-triggered preemption, then verify them with an opt-in live Docker Desktop Kubernetes E2E. The behavior must cover `Job`, `Pod`, and `Deployment` workloads. For `Deployment`, timeout means deleting the Deployment controller object, not only its Pods.

## 2. Background

Users can submit Kubernetes resources through workload job submit. Scheduler-quota already owns project plan and queue policy. Workload already owns dispatch, runtime cleanup, and job state transitions. Scheduler-quota already owns preemption decisions and calls workload owner contracts to mark victims preempted.

The new requirement adds two runtime policy paths:

- Queue `max_runtime_seconds` limits how long submitted resources may run.
- A high-priority submit can automatically preempt lower-priority eligible work when scheduler quota is insufficient.

## 3. Source References

- Kubernetes Deployments: https://kubernetes.io/docs/concepts/workloads/controllers/deployment/
- Kubernetes Jobs and `activeDeadlineSeconds`: https://kubernetes.io/docs/concepts/workloads/controllers/job/
- Kubernetes cascading deletion: https://kubernetes.io/docs/tasks/administer-cluster/use-cascading-deletion/
- Kubernetes Pod priority and preemption: https://kubernetes.io/docs/concepts/scheduling-eviction/pod-priority-preemption/
- Kueue preemption concepts: https://kueue.sigs.k8s.io/docs/concepts/preemption/
- Repo runtime behavior: `backend/internal/services/workload/dispatcher.go`, `backend/internal/services/workload/runtime_reaper.go`
- Repo scheduler behavior: `backend/internal/services/schedulerquota/admission.go`, `backend/internal/services/schedulerquota/preemption.go`

## 4. Assumptions

- “Resource insufficient” means scheduler-quota logical quota shortage from stored plan/queue/job data, not Kubernetes node pressure.
- Queue duration is an admin cap; if a user manifest already has a shorter native active deadline, keep the shorter value.
- Queue/plan/job payloads are JSON record data, so no SQL migration is required.
- Live E2E is independent from LDAP and uses seeded E2E users/projects.

## 5. Non-Goals

- Do not install Kueue, Volcano, or a custom Kubernetes scheduler.
- Do not implement real node-pressure preemption.
- Do not change public route paths.
- Do not make `Deployment` rely only on Pod failure for timeout cleanup.

## 6. Current Behavior

- Scheduler admission returns queue priority/preemptible metadata but workload submit does not persist all of it.
- Runtime reaper deletes expired resources carrying `platform-go/runtime-limit-seconds`.
- Dispatcher applies platform job/project/user/preemptible labels but not queue runtime duration labels or native active deadlines.
- Preemption is exposed through scheduler internal route, but user job submit does not automatically invoke it after quota denial.

## 7. Target Behavior

- Queue records can carry `max_runtime_seconds`.
- Scheduler admission returns `runtime_limit_seconds`, `priority_value`, and `preemptible` for the selected queue.
- Workload job submit persists admission metadata on the job record.
- Dispatcher applies duration by Kubernetes kind:
  - `batch/v1 Job`: set/cap `.spec.activeDeadlineSeconds`, plus runtime labels on object and pod template.
  - `v1 Pod`: set/cap `.spec.activeDeadlineSeconds`, plus runtime labels.
  - `apps/v1 Deployment`: label Deployment metadata and pod template; runtime reaper deletes the Deployment object on expiry so Kubernetes cascades dependents.
- Auto preemption runs only for scheduler admission `409` quota/resource shortage. If preemption completes, workload retries admission and persists the requester only after admission succeeds.

## 8. Affected Domains

- `scheduler-quota-service`: admission metadata, explicit preemption authorization, victim selection.
- `workload-service`: job submit flow, dispatch manifest mutation, runtime cleanup.
- `platform/cluster`: runtime-limited resource listing/deletion behavior for Deployments.
- E2E harness/docs: opt-in live Kubernetes verification.

## 9. Affected Files

- `backend/internal/services/schedulerquota/admission.go`
- `backend/internal/services/schedulerquota/admission_decode.go`
- `backend/internal/services/schedulerquota/preemption.go`
- `backend/internal/services/workload/job_submit.go`
- `backend/internal/services/workload/dispatcher.go`
- `backend/internal/services/workload/runtime_reaper.go`
- `backend/internal/platform/cluster/runtime.go`
- Focused tests under `backend/internal/services/workload`, `backend/internal/services/schedulerquota`, and `backend/internal/platform/cluster`
- New E2E under `backend/internal/e2e`
- `backend/docs/e2e-testing.md`

## 10. API / Contract Changes

- No route path changes.
- Queue payload accepts `max_runtime_seconds`.
- Scheduler admission response adds `runtime_limit_seconds`.
- Workload job records persist `priority_value`, `preemptible`, `is_preemptible`, and `runtime_limit_seconds`.
- Submit response can return `201 Created` after successful automatic preemption and retried admission where it previously returned quota `409`.

## 11. Database / Migration Changes

No SQL migration. All new fields live in existing JSON payload records.

## 12. Configuration Changes

Add opt-in test flag `TEST_LIVE_K8S_PLAN_WINDOW_DURATION_PREEMPTION=1`. No production configuration is required.

## 13. Observability Changes

Use existing event/log streams. Add structured fields to preemption attempt responses where useful: preemption status, preemption record id, victims, and original admission reason.

## 14. Security Considerations

- Explicit preemption override remains forbidden for normal users.
- Allow explicit preemption override only for admin/root/superadmin/system roles or verified service principals.
- Workload must call scheduler preemption using service auth only.
- Project membership enforcement from the previous workload access work remains in force.

## 15. Implementation Steps

1. Extend scheduler admission to read `max_runtime_seconds` from the selected queue and include `runtime_limit_seconds` in admission review data.
2. Extend workload submit to persist `priority_value`, `preemptible`, `is_preemptible`, and `runtime_limit_seconds` from successful admission.
3. Add workload auto-preemption helper used only when admission returns `409` and the denial reason is quota/resource shortage. The helper calls scheduler internal preemption with service auth, retries admission on completion, and does not persist the requester until retry admission succeeds.
4. Allow scheduler explicit preemption override for verified service principals in addition to admins/system roles.
5. Adjust preemption selection to minimize victims: lower priority first; for equal priority, prefer newer jobs first so older lower-priority work is preserved when either can satisfy demand.
6. Extend dispatcher manifest mutation to apply runtime labels and native active deadline for Pod/Job, and runtime labels for Deployment object and pod template.
7. Ensure runtime reaper deletes expired Deployments by deleting the Deployment controller object, relying on Kubernetes cascading deletion for dependents.
8. Add focused unit tests and an opt-in live E2E covering Job duration, Deployment duration, plan-window eviction, and auto preemption.
9. Update E2E runbook.

## 16. Verification Plan

- `go test ./internal/services/workload ./internal/services/schedulerquota ./internal/platform/cluster -count=1`
- `go test -tags e2e ./internal/e2e -run '^TestLiveK8sPlanWindowDurationPreemptionE2E$' -count=1 -v`
- Live: `TEST_LIVE_K8S_PLAN_WINDOW_DURATION_PREEMPTION=1 go test -tags e2e ./internal/e2e -run '^TestLiveK8sPlanWindowDurationPreemptionE2E$' -count=1 -v`
- `bash backend/scripts/ci-security-gate.sh quick`
- SonarScanner Quality Gate when credentials are available.

## 17. Rollback Plan

Revert the workload submit/dispatcher/runtime-reaper/scheduler admission/preemption changes and remove the opt-in E2E. Existing records with extra JSON fields are harmless because readers ignore unknown fields.

## 18. Risks and Tradeoffs

- Auto preemption adds a synchronous cross-service step on denied submit. It is bounded to the existing adapter timeout and only runs after quota `409`.
- Deployment timeout requires platform deletion rather than native `activeDeadlineSeconds`, because a Deployment controller maintains desired replica state.
- Live E2E mutates Docker Desktop Kubernetes, so it must use unique namespaces and cleanup.

## 19. Reviewer Checklist

- Requirement fit: queue duration, Deployment deletion, plan-window eviction, auto preemption.
- Boundary fit: scheduler-quota decides quota/preemption; workload owns dispatch and job state.
- Security: service-auth override only, normal user override denied.
- Tests: focused unit tests, E2E compile/default skip, live E2E instructions.
- Rollback: extra JSON fields are backward-compatible.

## 20. Status

Status: Approved
