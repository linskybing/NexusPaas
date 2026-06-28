# P2 Placement Scheduler Backend

## 1. Objective

Add a small scheduler-backend placement contract so jobs can request Kubernetes-native queueing backends through scheduler-owned `PlacementProfile` records. The first slice resolves placement metadata during admission and has workload dispatch inject the scheduler name, Kueue queue label, and Volcano gang hints without building a custom scheduler.

## 2. Background

`QUEUE-022` requires Kueue or an equivalent Kubernetes-native queueing path, while `CNCF-01` says NexusPaaS must not build a custom scheduler. The repo already has Plan/Queue admission, Volcano dispatch helpers, scheduler name injection, and network/data-plane dispatch injectors. This slice should reuse those paths and add only the missing profile contract.

Kueue documentation shows Kubernetes Jobs select a LocalQueue by setting the `kueue.x-k8s.io/queue-name` label on the Job metadata; LocalQueue maps to a ClusterQueue. Volcano support already exists in workload dispatch when `scheduler_name=volcano`.

## 3. Source References

- `docs/acceptance/plan-queue-quota.md`: QUEUE-022, queue/governance acceptance.
- `docs/acceptance/cncf-adoption.md`: do not build a custom scheduler; adopt Kueue/Kubernetes integrations.
- `backend/internal/services/schedulerquota/spec.go`: scheduler-quota service contract surface.
- `backend/internal/services/schedulerquota/network_profiles.go`: current profile CRUD/seed pattern.
- `backend/internal/services/schedulerquota/admission.go`: admission evaluation and response metadata.
- `backend/internal/services/workload/job_submit.go`: admission review propagation to job records.
- `backend/internal/services/workload/dispatcher.go`: scheduler name, queue, priority, and dispatch pipeline.
- `backend/internal/services/workload/dispatcher_volcano.go`: existing Volcano synthesis path.
- `backend/internal/services/workload/dispatcher_network.go`: current metadata injection pattern.
- Kueue LocalQueue docs: `https://kueue.sigs.k8s.io/docs/concepts/local_queue/`
- Kueue plain Job task docs: `https://kueue.sigs.k8s.io/docs/tasks/run/plain_jobs/`

## 4. Assumptions

- `scheduler-quota-service` owns placement policy metadata.
- `workload-service` consumes scheduler admission metadata and mutates manifests; it does not own placement profile records.
- Generic `platform_records` remains the durable store; no SQL DDL is needed.
- Existing jobs without `placement_profile` remain unchanged.
- Kueue and Volcano CRDs/operators are installed outside this slice.

## 5. Non-Goals

- No custom scheduler.
- No live Kueue controller integration tests.
- No Slurm/Slinky bridge.
- No topology-aware scheduling implementation.
- No DRA redesign.
- No queue admission rewrite.

## 6. Current Behavior

- Scheduler admission validates Project, Plan, Queue, quotas, streaming, resource floors, storage/network metadata, and preemption.
- Workload dispatch can set `schedulerName` and can synthesize Volcano resources only when `scheduler_name=volcano`.
- There is no admin-owned profile for choosing `kubernetes`, `kueue`, or `volcano` placement behavior.
- Kueue queue labels are not injected by workload dispatch.

## 7. Target Behavior

- Admins can CRUD placement profiles.
- Defaults are seeded idempotently:
  - `default-kubernetes`: `scheduler_backend=kubernetes`, `scheduler_name=default-scheduler`.
  - `kueue-batch`: `scheduler_backend=kueue`, `scheduler_name=default-scheduler`, `queue_label_key=kueue.x-k8s.io/queue-name`.
  - `volcano-gang`: `scheduler_backend=volcano`, `scheduler_name=volcano`, `gang_enabled=true`.
- A job may request `placement_profile`.
- Admission rejects missing or disabled requested profiles.
- Admission returns resolved placement metadata: `placement_profile`, `scheduler_backend`, `scheduler_name`, `gang_enabled`, `gang_min_available`, `placement_labels`, and `placement_annotations`.
- For Kueue, admission sets `placement_labels["kueue.x-k8s.io/queue-name"]` to the admitted queue name.
- Workload submit stores these resolved fields on the job record.
- Dispatch injects resolved placement labels/annotations onto workload object metadata and pod templates where appropriate, and relies on existing schedulerName injection.

## 8. Affected Domains

- `scheduler-quota-service`: owns placement profile records, seeds, admission resolution, and `PlacementProfileChanged`.
- `workload-service`: consumes resolved placement metadata and injects labels/annotations.
- `deploy/hpc/scheduler`: provides reference Kueue/Volcano manifests aligned to seeded profiles.

## 9. Affected Files

PR1:
- `backend/internal/services/schedulerquota/spec.go`
- `backend/internal/services/schedulerquota/handler.go`
- `backend/internal/services/schedulerquota/placement_profiles.go`
- `backend/internal/services/schedulerquota/placement_profiles_test.go`
- `backend/internal/contracts/fixtures/api/v1/*.json`
- `backend/internal/contracts/fixtures/events/v1/*.json`

PR2:
- `backend/internal/services/schedulerquota/admission.go`
- `backend/internal/services/schedulerquota/admission_decode.go`
- `backend/internal/services/schedulerquota/admission_placement.go`
- `backend/internal/services/schedulerquota/admission_test.go`
- `backend/internal/services/workload/job_submit.go`
- `backend/internal/services/workload/dispatcher_placement.go`
- `backend/internal/services/workload/dispatcher.go`
- `backend/internal/services/workload/dispatcher_placement_test.go`

PR3:
- `backend/deploy/hpc/scheduler/README.md`
- `backend/deploy/hpc/scheduler/kueue-localqueue-template.yaml`
- `backend/deploy/hpc/scheduler/kueue-clusterqueue-h100.yaml`
- `backend/deploy/hpc/scheduler/volcano-queue.yaml`

## 10. API / Contract Changes

PR1 adds admin CRUD:
- `GET /api/v1/placement-profiles`
- `POST /api/v1/placement-profiles`
- `GET /api/v1/placement-profiles/{id}`
- `PUT /api/v1/placement-profiles/{id}`
- `DELETE /api/v1/placement-profiles/{id}`

PR2 extends internal scheduler admission with optional request field:
- `placement_profile`

New response fields when a profile is resolved:
- `placement_profile`
- `scheduler_backend`
- `scheduler_name`
- `gang_enabled`
- `gang_min_available`
- `placement_labels`
- `placement_annotations`

Events:
- `PlacementProfileChanged`

## 11. Database / Migration Changes

No SQL DDL. New durable record resource:
- `scheduler-quota-service:placement_profiles`

## 12. Configuration Changes

No new required environment variables. Kueue/Volcano manifests are reference overlays only.

## 13. Observability Changes

- `PlacementProfileChanged` records profile mutations.
- `SubmitAdmissionReviewed` includes resolved placement metadata.
- Dispatch-created manifests carry backend labels/annotations for Kubernetes-native controller visibility.

## 14. Security Considerations

- Placement profile writes are admin-only.
- Jobs can request only existing enabled profiles.
- User-provided labels/annotations are not trusted directly; workload dispatch uses only scheduler-resolved metadata.
- No secrets are stored in placement profiles.

## 15. Implementation Steps

1. PR1: add `PlacementProfile` CRUD and seeded defaults.
   - Add `placement_profiles` table/event/routes to scheduler `Spec()`.
   - Register required fields: `name`, `scheduler_backend`.
   - Seed `default-kubernetes`, `kueue-batch`, and `volcano-gang`.
   - Add focused CRUD/seed/admin/event tests and contract fixtures.

2. PR2: resolve placement during admission and inject dispatch metadata.
   - Decode optional `placement_profile`.
   - Reject missing or disabled profiles.
   - Resolve scheduler backend/name, gang settings, labels, and annotations.
   - For Kueue profiles with a queue label key, set that label to admitted queue name.
   - Add resolved fields to `admissionReviewData`.
   - Apply resolved fields to submitted job records.
   - Inject `placement_labels` / `placement_annotations` onto object metadata and pod templates.
   - Keep absent placement profile as a no-op.

3. PR3: add reference scheduler manifests.
   - Add minimal Kueue ClusterQueue and LocalQueue template manifests.
   - Add minimal Volcano Queue manifest.
   - Document prerequisites and dry-run checks.

## 16. Verification Plan

Per PR:
- `cd backend && go test ./internal/services/schedulerquota/...`
- `cd backend && go test ./internal/services/workload/...` when workload files change
- `cd backend && go test ./internal/contracts/...`
- `cd backend && go test ./internal/services/...`
- `cd backend && go build ./...`
- Run SonarScanner per `CLAUDE.md` when the local scanner environment is available.

Additional checks:
- PR2: unit test proves Kueue labels and Volcano scheduler metadata are injected.
- PR2: unit test proves absent placement profile is a no-op.
- PR3: YAML parse check for `backend/deploy/hpc/scheduler/*.yaml`; `kubectl apply --dry-run=client -f backend/deploy/hpc/scheduler/` when a local API server is available.

## 17. Rollback Plan

- PR1: remove placement profile routes, table/event entries, seed helper, tests, and fixtures.
- PR2: remove admission fields and dispatch injector. Existing jobs remain compatible because all new fields are optional.
- PR3: delete `backend/deploy/hpc/scheduler`.

## 18. Risks and Tradeoffs

- This slice does not prove live Kueue or Volcano controller behavior.
- Kueue queue label injection is limited to metadata; LocalQueue/ClusterQueue installation remains a deployment prerequisite.
- Volcano synthesis is reused as-is and remains gated by the existing dynamic client and `scheduler_name=volcano`.
- Deeper topology and gang timeout semantics are deferred until scheduler backend adapters are validated.

## 19. Reviewer Checklist

- Requirement fit: moves QUEUE-022 forward without rebuilding scheduling.
- Scope control: profile metadata, admission resolution, dispatch labels, and reference manifests only.
- Architecture: scheduler owns policy; workload consumes scheduler-resolved metadata.
- API contract: routes/events/tables are declared in `Spec()` and fixtures.
- Data ownership: no cross-service writable records.
- Config: no new mandatory runtime config.
- Observability: profile changes and admission metadata are visible.
- Security: admin CRUD and scheduler-resolved labels/annotations only.
- Testing: focused scheduler/workload/contract tests cover behavior.
- Simplicity: no new controller, SDK, or custom scheduler.

## 20. Status

Status: Approved
