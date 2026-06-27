# P2 MPS Admission Policy

## 1. Objective

Create the next scheduler admission-only slice for `GPU-011`, `GPU-012`, and `GPU-013` from `docs/acceptance/gpu-dra-mps.md`: enforce MPS share policies at admission time with queue/plan/project policy evaluation and local active-workload guardrails, no hardware dependency.

## 2. Background

Admission already evaluates plans, queues, accelerator profiles, and fractional GPU accounting, but does not yet enforce explicit policy on MPS admission behavior. This slice closes the admission-policy gap before hardware validation (`GPU-016`) and avoids DRA scheduler redesign.

## 3. Source References

- `docs/acceptance/gpu-dra-mps.md` (GPU-011/012/013)
- `backend/internal/services/schedulerquota/admission.go`
- `backend/internal/services/schedulerquota/admission_accelerator.go`
- `backend/internal/services/schedulerquota/admission_resources.go`
- `backend/internal/services/schedulerquota/admission_quota.go`
- `backend/internal/services/schedulerquota/read_contracts.go`
- `backend/internal/services/schedulerquota/admission_test.go`
- `backend/internal/services/schedulerquota/accelerator_profiles.go`
- `docs/agents/planning.md`, `docs/agents/review-checklist.md`

## 4. Assumptions

- `scheduler-quota-service` owns policy resolution for accelerator admission decisions.
- No new data model service; all policy values live in existing `platform_records` on plans, queues, projects, and workload jobs.
- Existing `accelerator_profile` defaults (e.g., `default_mps_sm_percentage`) remain authoritative in admission request normalization.
- GPU cross-project policy is enforced as a control-plane approximation using active job records (same `device_class_name`, same `sm_percentage < 100`), not live node-scheduler proof.
- This slice updates only the admission path and acceptance docs.

## 5. Non-Goals

- No new DDL / SQL migration / custom tables.
- No new hardware/e2e validation.
- No new DRA implementation beyond existing `ResourceClaimTemplate`/MPS config wiring.
- No dispatch/injector work in this slice.
- No Kubernetes scheduler/placement-level proof of co-location.

## 6. Current Behavior

- Plan/queue admission currently enforces quota, queue membership, resource floor, device class, streaming, network/placement, and accelerator profile resolution.
- MPS semantics are inferred from `sm_percentage`, `pinned_memory_limit`, and accelerator profile defaults, but no explicit policy checks for cross-project safety or SM caps.
- Quota checks already use `required_gpu` and active workload usage (including fractional values) from existing workload job records.

## 7. Target Behavior

### 7.1 MPS request detection

A request is treated as MPS when any condition is true:

- `sm_percentage < 100`
- `pinned_memory_limit` is present
- an accelerator profile default sets `default_mps_sm_percentage < 100` and request does not supply `sm_percentage`

### 7.2 Plan-level policy (resource ownership)

Plan record fields (optional policy keys):

- `mps_allowed` (`mpsAllowed`) default `true`.
- `max_sm_percentage_per_gpu` (`maxMpsSmPercentage`) default unset (no extra cap).
- `allow_cross_project_mps` (`allowCrossProjectMps`) default `false`.

Validation at admission:

- `max_sm_percentage_per_gpu` must be `1..100` when present.
- booleans must be booleans.
- missing fields default to the policy fallback values above.

### 7.3 Queue-level policy override/tighten

Queue policy fields (optional policy keys):

- `mps_allowed` (`mpsAllowed`)
- `max_sm_percentage_per_gpu` (`maxMpsSmPercentage`)
- `allow_cross_project_mps` (`allowCrossProjectMps`)

Queue semantics are tighten-only on top of plan:

- effective `mps_allowed` = `plan_mps_allowed && (queue_mps_allowed unset ? true : queue_mps_allowed)`.
- effective max SM cap = tighter of plan and queue non-zero caps.
- effective cross-project allow = `plan_allow_cross_project_mps && (queue_allow_cross_project_mps unset ? true : queue_allow_cross_project_mps)`.

Queue fields can disable MPS or lower caps, but cannot re-enable a capability denied by the plan.

### 7.4 Project-level policy

Project policy fields:

- `high_security` (`highSecurity`) or `mps_forbidden` (`mpsForbidden`)

Either key set to true blocks all MPS requests unconditionally.

### 7.5 GPU-011 enforcement

- Reject admission with `sm_percentage` above effective plan/queue max.
- Continue using existing project GPU quota logic for total GPU capacity; this already consumes fractional `required_gpu` from active workload records.
- MPS sharing by itself is not a quota exception; it still consumes `required_gpu`.

### 7.6 GPU-012 enforcement (local approximation)

- If request is MPS, scan active workload records for other projects with same `device_class_name` and `sm_percentage < 100`.
- Active statuses for this scan are the same scheduler admission workload statuses used by quota accounting via `activeAdmissionStatus` (currently `submitted`, `waiting_infra`, `queued`, `running`).
- If any active match exists and effective cross-project allow is false, reject admission.
- This is explicit policy proof only, not live scheduler node proof; document as local conservative approximation.

Malformed policy fields on plan/queue/project records are denied deterministically instead of ignored. Boolean fields must decode as booleans, and SM cap fields must decode as integers in `1..100`.

## 8. Affected Domains

- `scheduler-quota-service`: admission resolver and policy validation.
- `docs/acceptance`: policy caveat visibility update for GPU-013.

## 9. Affected Files

- `backend/internal/services/schedulerquota/admission.go` (hook policy guard into evaluation flow)
- `backend/internal/services/schedulerquota/admission_resources.go` (any shared MPS helpers for `%`/string parsing if needed)
- `backend/internal/services/schedulerquota/admission_quota.go` (helper for fractional MPS policy checks against project usage if shared)
- `backend/internal/services/schedulerquota/admission_mps.go` (new helper module for MPS policy resolution + active-workload cross-project check)
- `backend/internal/services/schedulerquota/admission_test.go` (new and updated tests)
- `docs/acceptance/gpu-dra-mps.md` (GPU-013 caveat/update)
- `backend/docs/browser-gpu-streaming.md` (user-visible MPS caveat)
- `backend/docs/operational-readiness.md` (admin/operator MPS policy caveat)

## 10. API / Contract Changes

No new public API routes/events.

- Contract impact is internal-only: same `POST /api/v1/internal/scheduler/admission` request/response surface.
- New policy keys are stored in existing plan/queue/project payloads and read at admission:
  - plan fields
    - `mps_allowed`, `max_sm_percentage_per_gpu`, `allow_cross_project_mps`
  - queue fields
    - `mps_allowed`, `max_sm_percentage_per_gpu`, `allow_cross_project_mps`
  - project fields
    - `high_security` / `mps_forbidden`
- Admission output shape unchanged except for rejection reasons.

## 11. Database / Migration Changes

No SQL DDL or schema migration.

- Uses existing records:
  - `scheduler-quota-service:plans`
  - `scheduler-quota-service:queues`
  - `org-project-service:projects`
  - `workload-service:jobs` (read-only for active MPS conflict scan)

## 12. Configuration Changes

No new environment variables.

- Behavior is data-driven from existing plan/queue/project records.

## 13. Observability Changes

- Add/augment deny reasons in `SubmitAdmissionReviewed` responses for:
  - MPS request blocked by project policy
  - SM percentage exceeds effective cap
  - cross-project MPS denied by policy
- Continue relying on existing event stream; no new event type required.

## 14. Security Considerations

- Only scheduler-admission process makes enforcement decisions; workload submitter cannot override policy.
- Policy fields read from owner records only; malformed values result in deterministic denial.
- Cross-project deny uses explicit fields and active-workload status to avoid silent oversubscription assumptions.
- No dynamic code execution or string-templating paths are introduced.

## 15. Implementation Steps

1. Add `backend/internal/services/schedulerquota/admission_mps.go` with policy helpers:
   - extract MPS request intent from `req.SMPercentage`, `req.PinnedMemoryLimit`, and resolved accelerator-profile defaults.
   - resolve effective MPS policy from plan/queue/project values.
   - check project-level `high_security`/`mps_forbidden`.
   - check per-job SM cap against effective values.
   - scan active jobs in `ListWorkloadJobs` for same `device_class_name`, other project, and active `sm_percentage < 100`.
2. Wire enforcement in `evaluateSubmitAdmission` after plan/queue/project resolution and after accelerator profile resolution.
3. Add/update admission checks in `backend/internal/services/schedulerquota/admission_test.go`:
   - `TestSubmitAdmissionRejectsMPSAbovePlanOrQueueMax` (plan/queue max cap)
   - `TestSubmitAdmissionRejectsMPSWhenProjectHighSecurityOrForbidden`
   - `TestSubmitAdmissionRejectsCrossProjectMPSWithoutAllowCrossPolicy`
   - `TestSubmitAdmissionAllowsCrossProjectMPSWhenExplicitlyAllowed`
   - `TestSubmitAdmissionRejectsMalformedMPSPolicyFields`
   - `TestSubmitAdmissionUsesActiveStatusesForCrossProjectMPSScan`
   - `TestSubmitAdmissionRejectsProjectGPUQuotaWithActiveFractionalUsage` (explicitly cover fractional required GPU in existing quota path)
4. Update `docs/acceptance/gpu-dra-mps.md` with GPU-013 caveat:
   - highlight that cross-project MPS checks are local to scheduler admission records and may be relaxed before true placement-level enforcement.
5. Update user/admin docs:
   - user caveat in `backend/docs/browser-gpu-streaming.md`
   - operator policy caveat in `backend/docs/operational-readiness.md`

## 16. Verification Plan

- `cd backend && go test ./internal/services/schedulerquota -run "TestSubmitAdmission"`
- `cd backend && go test ./internal/services/schedulerquota -run "TestSubmitAdmissionRejectsMPS|TestSubmitAdmissionAllowsCrossProjectMPS|TestSubmitAdmissionRejectsProjectGPUQuota"`
- `cd backend && go test ./internal/services/workload/...` (only if no-op on workload behavior is verified after plan-level change)
- `cd backend && go test ./internal/contracts/...`
- `cd backend && go test ./...`
- Manual check: `grep -n "GPU-013\|MPS" docs/acceptance/gpu-dra-mps.md` confirms caveat text is present.
- Optional pipeline: run SonarScanner per project policy before reviewer handoff.

## 17. Rollback Plan

- Remove `admission_mps.go` and callsites in `evaluateSubmitAdmission`.
- Drop new admission tests and return to existing policy-only behavior.
- Revert docs caveat update.

## 18. Risks and Tradeoffs

- Cross-project check is approximate and can only use active workload metadata, not node-level placement.
- Queue field presence semantics must be explicit to avoid accidental policy broadening.
- Existing plan/queue records are schema-light; malformed admin input on new keys can cause silent policy no-op without strict record-level validation.
- Conservative cross-project blocking may reject some admissible hardware placements.

## 19. Reviewer Checklist

- Scope: admission-only slice aligned to `GPU-011`/`GPU-012`/`GPU-013`.
- No new DB schema or migrations.
- Plan/queue/project policy definitions are explicit and testable.
- Uses existing active workload records without introducing new reservation infrastructure.
- Test plan includes targeted coverage for plan/queue SM cap, project block, and cross-project denial/allow.
- Docs change covers caveat and states local-proof limitation.
- No API route/event bloat beyond internal admission behavior.

## 20. Status

Status: Approved
