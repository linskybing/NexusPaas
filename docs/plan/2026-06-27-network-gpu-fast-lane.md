# P2 Network GPU Fast Lane

## 1. Objective

Add the first reviewable slice of the AI/HPC network fast lane: scheduler-owned `NetworkProfile` metadata, admission-time network/topology hints, workload dispatch injection of Multus/NCCL metadata, and matching HPC network manifests.

## 2. Background

The storage data path now has profile-driven planning. The next highest-impact gap is multi-node GPU training traffic: ordinary Pod networking remains the default, and jobs have no first-class way to request RDMA/secondary-network placement metadata. The platform should expose this as policy and manifest metadata while leaving actual scheduling and device allocation to Kubernetes-native components.

## 3. Source References

- `docs/acceptance/cncf-adoption.md`: use Kueue/Kubernetes integrations; do not build a custom scheduler.
- `docs/acceptance/iteration-plan.md`: M4/M6 call out queue governance, Kueue integration, GPU/DRA, and usage attribution.
- `backend/internal/services/schedulerquota/spec.go`: scheduler-quota service contract surface.
- `backend/internal/services/schedulerquota/handler.go`: current Plan/Queue custom handler and repository pattern.
- `backend/internal/services/schedulerquota/admission.go`: submit admission evaluation and response metadata.
- `backend/internal/services/schedulerquota/admission_decode.go`: submit admission payload decoding.
- `backend/internal/services/workload/job_submit.go`: workload submit payload preservation and admission review application.
- `backend/internal/services/workload/dispatcher.go`: dispatch manifest preparation pipeline.
- `backend/internal/services/workload/dispatcher_dataplane.go`: existing Pod/VCJob injection pattern.
- `backend/deploy/hpc/storage/`: current HPC overlay shape.
- `docs/agents/*.md`: required planning, review, and implementation workflow.

## 4. Assumptions

- `scheduler-quota-service` owns policy metadata for network profiles and admission-time network hints.
- `workload-service` consumes admission metadata and injects manifest annotations/env vars; it does not own network profile records.
- `k8s-control-service` remains the eventual owner for deeper adapter-specific manifest generation, but this slice can operate in workload dispatch because dispatch already injects storage, data-plane, DRA, scheduling, and runtime metadata.
- Generic `platform_records` storage remains the durable store; no per-entity SQL DDL is needed.
- Existing jobs without `network_profile`, `rdma_required`, `nic_class`, or `topology_requirement` must stay byte-compatible except for unrelated map ordering.

## 5. Non-Goals

- No custom scheduler.
- No Kueue, Volcano, Slurm, or DRA redesign in this slice.
- No live RDMA device discovery or node-residency validation.
- No GPU/NIC topology scoring.
- No image cold-start work.
- No production deployment of Multus/SR-IOV/RDMA operators; only manifests and contracts.

## 6. Current Behavior

- Scheduler admission validates project, queue, device class, resource floors, quotas, and streaming limits.
- Workload submit stores the original payload and applies selected admission review fields to the job record.
- Dispatch can mutate Pod/Deployment/Job/VCJob manifests for storage mounts, data-plane resources, streaming, DRA, scheduling, and runtime limits.
- Jobs cannot select a platform-approved network profile, and dispatch does not inject Multus annotations or NCCL network env vars.

## 7. Target Behavior

- Admins can CRUD network profiles through `scheduler-quota-service`.
- Defaults are seeded idempotently:
  - `default-cilium`: primary Cilium only, no RDMA.
  - `rdma-fast-lane`: Cilium primary plus `nexuspaas-system/rdma-net` secondary network, RDMA enabled, `NCCL_SOCKET_IFNAME=net1`, `NCCL_IB_DISABLE=0`.
- A job may request `network_profile`, `rdma_required`, `nic_class`, and `topology_requirement`.
- Admission rejects missing or disabled requested profiles.
- Admission returns the resolved profile name, RDMA/topology hints, Multus annotation value, and NCCL env map.
- Workload dispatch injects the resolved `k8s.v1.cni.cncf.io/networks` annotation and NCCL env vars into Pod templates and VCJob task templates.
- Absent network profile stays a no-op.
- HPC deploy manifests provide a minimal Multus RDMA `NetworkAttachmentDefinition` and README aligned with the seeded profile.

## 8. Affected Domains

- `scheduler-quota-service`: owns `NetworkProfile` records, profile seeds, admission profile resolution, and related events.
- `workload-service`: consumes admission metadata and injects dispatch manifest annotations/env.
- `deploy/hpc/network`: supplies reference manifests for the seeded RDMA profile.

## 9. Affected Files

PR1:
- `backend/internal/services/schedulerquota/spec.go`
- `backend/internal/services/schedulerquota/handler.go`
- `backend/internal/services/schedulerquota/network_profiles.go`
- `backend/internal/services/schedulerquota/scheduler_quota_repository.go`
- `backend/internal/services/schedulerquota/network_profiles_test.go`
- `backend/internal/contracts/fixtures/api/v1/*.json`
- `backend/internal/contracts/fixtures/events/v1/*.json`

PR2:
- `backend/internal/services/schedulerquota/admission.go`
- `backend/internal/services/schedulerquota/admission_decode.go`
- `backend/internal/services/schedulerquota/admission_test.go`
- `backend/internal/services/workload/job_submit.go`
- `backend/internal/services/workload/dispatcher_network.go`
- `backend/internal/services/workload/dispatcher.go`
- `backend/internal/services/workload/dispatcher_network_test.go`

PR3:
- `backend/deploy/hpc/network/README.md`
- `backend/deploy/hpc/network/multus-rdma-net.yaml`

## 10. API / Contract Changes

PR1 adds admin CRUD:
- `GET /api/v1/network-profiles`
- `POST /api/v1/network-profiles`
- `GET /api/v1/network-profiles/{id}`
- `PUT /api/v1/network-profiles/{id}`
- `DELETE /api/v1/network-profiles/{id}`

PR2 extends the existing internal scheduler admission request/response. New optional request fields:
- `network_profile`
- `rdma_required`
- `nic_class`
- `topology_requirement`

New response fields when a profile is resolved:
- `network_profile`
- `rdma_required`
- `nic_class`
- `topology_requirement`
- `network_annotations`
- `network_env`

Events:
- `NetworkProfileChanged`

## 11. Database / Migration Changes

No SQL DDL. New durable record resource:
- `scheduler-quota-service:network_profiles`

## 12. Configuration Changes

No new required environment variables. The seeded RDMA profile uses the manifest name `nexuspaas-system/rdma-net` so dev clusters without Multus are unaffected unless a job asks for that profile.

## 13. Observability Changes

- `NetworkProfileChanged` is emitted for profile CRUD.
- `SubmitAdmissionReviewed` carries resolved network fields as part of existing admission review data.
- Dispatch-created manifests make the selected network profile visible through annotations/env on the workload pod template.

## 14. Security Considerations

- Network profile writes are admin-only.
- Jobs can request only existing enabled profiles; user-provided annotation/env values are not trusted directly.
- Workload-service injects only scheduler-resolved `network_annotations` and `network_env`.
- No secret material is stored in profiles.

## 15. Implementation Steps

1. PR1: add `NetworkProfile` CRUD and seeded defaults.
   - Add `network_profiles` to scheduler `Spec().Tables`.
   - Add admin CRUD routes to `Spec().Routes`.
   - Register required fields for `scheduler-quota-service:network_profiles`.
   - Seed `default-cilium` and `rdma-fast-lane` idempotently from `schedulerquota.Register`.
   - Add focused tests for idempotent seeds, required-field rejection, disabled profile behavior, and admin guard.

2. PR2: resolve network profile during admission and inject dispatch metadata.
   - Decode optional network/topology request fields.
   - Resolve requested profile by id/name; reject missing or disabled profile.
   - Preserve explicit `rdma_required`, `nic_class`, and `topology_requirement` as stricter hints when provided.
   - Add resolved fields to `admissionReviewData`.
   - Apply resolved fields to workload job records.
   - Add `prepareNetworkDispatchResources` before streaming/DRA dispatch mutations.
   - Inject annotations onto object metadata and Pod templates; inject env vars into app containers.
   - Keep absent profile as a no-op.

3. PR3: add HPC network reference manifests.
   - Add `backend/deploy/hpc/network/multus-rdma-net.yaml`.
   - Add README describing prerequisites and dry-run verification.

## 16. Verification Plan

Per PR:
- `cd backend && go test ./internal/services/schedulerquota/...`
- `cd backend && go test ./internal/services/workload/...` when workload files change
- `cd backend && go test ./internal/contracts/...`
- `cd backend && go test ./internal/services/...`
- `cd backend && go build ./...`
- Run SonarScanner per `CLAUDE.md` before Reviewer sign-off when the local scanner environment is available.

Additional checks:
- PR2: unit test proves network annotation and NCCL env are injected into Pod and VCJob resources.
- PR2: unit test proves absent network profile is a no-op.
- PR3: YAML parse check for `backend/deploy/hpc/network/*.yaml`; `kubectl apply --dry-run=client -f backend/deploy/hpc/network/` when a local API server is available.

## 17. Rollback Plan

- PR1: remove network profile routes, table/event entries, seed helper, and tests. Existing generic records become inert.
- PR2: remove admission fields and dispatch injector. Existing jobs remain compatible because all new fields are optional.
- PR3: delete `backend/deploy/hpc/network`.

## 18. Risks and Tradeoffs

- This slice does not prove live RDMA performance. It only creates the platform contract and manifest injection path.
- `rdma-fast-lane` assumes the conventional `net1` Multus interface name; clusters that customize interface naming can update the profile payload.
- Workload dispatch owns the first injector because it already mutates manifests today. A later k8s-control adapter can absorb this when the lower scheduler adapter layer is ready.
- Profile validation is intentionally local to scheduler admission; deeper node topology checks are deferred until node inventory exists.

## 19. Reviewer Checklist

- Requirement fit: P2 first slice gives jobs a first-class network profile and RDMA metadata path.
- Scope control: no scheduler rewrite, no Slurm/Kueue/Volcano implementation.
- Architecture: scheduler owns policy; workload consumes admission metadata.
- API contract: routes/events/tables are declared in `Spec()` and fixtures.
- Data ownership: no cross-service writable records.
- Config: no new mandatory runtime config.
- Observability: profile changes and admission metadata are visible.
- Security: admin CRUD and scheduler-resolved annotations/env only.
- Testing: focused scheduler/workload/contract tests cover the behavior.
- Simplicity: manifests and metadata only; no custom networking controller.

## 20. Status

Status: Approved
