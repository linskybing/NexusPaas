# Image Build Supply-Chain Status Slice

## 1. Objective

Add a minimal supply-chain status slice for queued image-build records and
`ImageBuildStarted` events in `image-registry-service`, capturing the metadata needed
to progress IMG-025 and prepare IMG-016/017/018 without introducing live SBOM,
image signing, or runtime scan/promotion behavior.

## 2. Background

Queued image builds currently carry build intent and orchestration details, but do not
persist a structured supply-chain signal set aligned with downstream readiness checks.
To unblock traceability and later verification phases, this slice records immutable
status fields at queue time and emits them in `ImageBuildStarted` events.

## 3. Source References

- `backend/internal/services/imageregistry/handler.go`
- `backend/internal/services/imageregistry/spec.go`
- `backend/internal/services/imageregistry/handler_test.go`
- `backend/internal/services/imageregistry/api_fixtures_test.go`
- `backend/internal/contracts/fixtures/api/v1/image-registry-context-build.json`
- `backend/internal/contracts/fixtures/api/v1/image-registry-dockerfile-build.json`
- `backend/internal/contracts/fixtures/api/v1/image-registry-storage-build.json`
- `backend/internal/contracts/fixtures/events/v1/image-build-started.json`
- `docs/acceptance/image-build.md`
- `docs/acceptance/ga-acceptance-trace-matrix.md`

## 4. Assumptions

- `image-registry-service` owns image-build records and corresponding event emission
  paths.
- Adding new fields to records and existing event payloads is additive and
  backward-compatible for current tolerant JSON readers.
- `ImageBuildStarted` remains schema version `1`; older consumers may ignore
  the new optional keys, and newer consumers must tolerate missing keys from
  historical records/events.
- No external registry scanners, signing engines, or policy gates are required in this
  slice.
- Contract tests and focused service tests remain the primary validation path for this
  change set.

## 5. Non-Goals

- No live SBOM generation/extraction.
- No Cosign/Notary signing/signature verification integration.
- No Trivy or vulnerability scan execution.
- No Harbor/Tekton/BuildKit runtime behavior changes.
- No image promotion flow.
- No executor cancellation support.
- No frontend/API consumer UI changes.

## 6. Current Behavior

- Image builds can be queued and observed through existing handlers and contracts.
- `ImageBuildStarted` event payload reflects current runtime orchestration fields but lacks
  a dedicated supply-chain status envelope.
- Acceptance evidence currently focuses on queueing/build lifecycle basics and does not
  track `sbom_available`, `signature_ready`, or scan status markers.

## 7. Target Behavior

- Queued image-build record includes new supply-chain status fields (initially defaulted).
- `ImageBuildStarted` event payload includes the same status envelope so downstream trace
  steps can assert presence from the first lifecycle event.
- The queued response and stored record expose pending/unknown values without claiming
  a completed SBOM, signature, scan, digest, or allow-list decision.
- Existing workflows remain unchanged unless they read/write the new optional fields.
- Initial values indicate pending/unknown states suitable for non-blocking future
  completion by IMG-016/017/018 pipelines.

## 8. Affected Domains

- `image-registry-service` owns all code changes: queued image-build record
  persistence, public response shaping, and `ImageBuildStarted` event payloads.
- `internal/contracts` owns fixture validation only; it does not own
  image-registry behavior.
- `docs/acceptance` owns evidence wording only; it must keep live SBOM/signing,
  scan execution, image promotion, and full IMG GA open.
- No scheduler, workload, k8s-control, Harbor adapter, frontend, or deployment
  slice is owned by this plan.

## 9. Affected Files

- `backend/internal/services/imageregistry/handler.go` (record creation path + event emit path)
- `backend/internal/services/imageregistry/handler_test.go` (queue-level assertions)
- `backend/internal/services/imageregistry/api_fixtures_test.go` (contract fixture assertions)
- `backend/internal/contracts/fixtures/api/v1/image-registry-context-build.json`
- `backend/internal/contracts/fixtures/api/v1/image-registry-dockerfile-build.json`
- `backend/internal/contracts/fixtures/api/v1/image-registry-storage-build.json`
- `backend/internal/contracts/fixtures/events/v1/image-build-started.json` (event fixture)
- `backend/internal/contracts/event_envelope_test.go` (event fixture allowlist)
- `docs/acceptance/image-build.md` (evidence and sample payloads)
- `docs/acceptance/ga-acceptance-trace-matrix.md` (trace expectation row updates)

## 10. API / Contract Changes

- Extend queued-image-build schema fields with supply-chain status:
  - `image_digest` (empty until the push result is known)
  - `allow_list_decision` (initial `pending`)
  - `sbom_status` (enum/closed set string: `pending`/`required`/`available`)
  - `signature_status` (enum/closed set string: `pending`/`required`/`signed`/`not_applicable`)
  - `scan_status` (enum/closed set string: `pending`/`not_run`/`passed`/`failed`/`not_applicable`)
  - `supply_chain_checked_at` (RFC3339 timestamp or null)
- Extend `ImageBuildStarted` payload with the same keys, preserving existing top-level shape.
- Treat all new fields as optional/additive for readers. Missing fields on
  historical records or replayed old events mean `unknown` to consumers; they
  must not be interpreted as pass/fail.
- Keep `ImageBuildStarted` event envelope schema version at `1` because the
  change is additive and tolerant-reader compatible.
- Add explicit compatibility coverage for a historical `ImageBuildStarted`
  payload that omits all new supply-chain keys; decoding/validation must still
  pass.

## 11. Database / Migration Changes

- No SQL schema migration. Image builds remain generic
  `platform_records(resource='image-registry-service:image_build_jobs')` JSON
  payloads.
- New queued records get explicit defaults at create time.
- Existing rows are not backfilled in this slice. Read paths must tolerate
  missing values, and docs must describe missing historical fields as
  `unknown`, not as successful supply-chain proof.
- Optional operator audit query after rollout:
  `SELECT id FROM platform_records WHERE resource='image-registry-service:image_build_jobs' AND NOT (payload ? 'sbom_status');`
  Rows returned are historical/unbackfilled and are expected unless an operator
  chooses a separate backfill.

## 12. Configuration Changes

None. All defaults are explicit in service code to avoid environment-dependent status
population.

## 13. Observability Changes

- Add/extend assertions around event shape for:
  - presence of supply-chain status fields on `ImageBuildStarted`
  - defaults on queued build creation.
- Add a reviewer-visible audit note for detecting historical rows without the
  new metadata keys; do not add runtime metrics in this slice.
- No metric or tracing pipeline changes in this slice.

## 14. Security Considerations

- New status fields are non-sensitive operational metadata.
- No secret, token, credentials, or artifact digest material is introduced at this stage.
- Keep event payload size bounded to avoid expanding log payload footprint unnecessarily.

## 15. Implementation Steps

1. Add new record fields and default initialization in queued-build creation code path.
2. Propagate those fields into the in-memory event payload builder used by
   `ImageBuildStarted`.
3. Add focused imageregistry tests:
   - queued build response and stored record include exact defaults;
   - `ImageBuildStarted` event includes the same defaults;
   - existing listing/public response paths tolerate records missing the new
     fields.
4. Update the three existing image build API fixtures and add an
   `ImageBuildStarted` event fixture to lock contract shape.
5. Add a historical-event compatibility test or fixture path proving old
   `ImageBuildStarted` payloads without the new optional keys still decode and
   validate as schema version `1`.
6. Add acceptance doc entries for IMG-025 evidence and GA trace matrix hints for supply-chain
   completeness.

## 16. Verification Plan

- Required focused checks before broad suites:
  - `cd backend && go test ./internal/services/imageregistry -run "ImageBuildSupplyChain|ImageBuildStarted|ExternalAPI"`
  - `cd backend && go test ./internal/contracts/...` including an explicit
    historical `ImageBuildStarted` additive-compatibility test where the
    payload omits `image_digest`, `allow_list_decision`, `sbom_status`,
    `signature_status`, `scan_status`, and `supply_chain_checked_at`.
- `cd backend && go test ./internal/services/imageregistry -run "ImageBuild|ImageBuildStarted|ImageBuildQueued"`
- `cd backend && go test ./internal/services/...`
- `cd backend && go test ./...`
- `cd backend && go build ./...`
- `cd backend && make coverage`
- `cd backend && make ci-sonar`
- `git diff --check`

## 17. Rollback Plan

- Revert the record field additions and event payload field additions in
  `imageregistry`.
- Remove/restore fixture entries and acceptance doc changes.
- No data migration side effects, so rollback leaves historical records intact.
- Roll back if any queue/event consumer demonstrably fails on additive
  `ImageBuildStarted` payload fields; the restored event payload shape is the
  previous top-level image-build record only.

## 18. Risks and Tradeoffs

- Event consumers that strictly validate payloads may need fixture updates; this plan includes
  contract updates before merge.
- Optional/defaulted status fields avoid runtime failures but do not guarantee completion
  enforcement.
- This slice advances traceability but does not guarantee supply-chain integrity (intentionally).
- Existing rows remain unbackfilled, so dashboards must display missing fields
  as `unknown` or omit them until a separate backfill is approved.
- The operator audit query above is a diagnostic only; this slice must not fail
  service startup due to historical records.

## 19. Reviewer Checklist

- [ ] Changes limited to record schema + `ImageBuildStarted` contract surface.
- [ ] No SBOM/signing/scan runtime integrations are introduced.
- [ ] No Harbor/Tekton/BuildKit orchestration changes.
- [ ] No executor cancellation changes.
- [ ] No frontend/API external contract-breaking changes beyond additive fields.
- [ ] Tests include focused imageregistry defaults/event/listing coverage and
  contract fixture validation.
- [ ] Acceptance docs include executable criteria:
  - Given a valid image build request, when it is queued, then the response,
    stored record, and `ImageBuildStarted` event contain
    `allow_list_decision="pending"`, `sbom_status="pending"`,
    `signature_status="pending"`, `scan_status="pending"`,
    `image_digest=""`, and `supply_chain_checked_at=null`.
  - Given a historical image build record without those fields, when it is
    listed, then the service still returns the record without panic or failure.
  - Given a historical `ImageBuildStarted` event payload without those fields,
    when contracts decode/validate it, then schema version `1` remains valid
    and missing optional fields are treated as unknown.

## 20. Status

Status: Approved
